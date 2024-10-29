package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	gliderssh "github.com/gliderlabs/ssh"
	_ "github.com/go-sql-driver/mysql"
	"github.com/open-oni/oni-agent/internal/queue"
	"github.com/open-oni/oni-agent/internal/version"
	"golang.org/x/crypto/ssh"
)

// BABind is the address and port to bind this process
var BABind string

// ONILocation is where Open ONI lives on the server, for invoking the
// management commands
var ONILocation string

// BatchSource is where batches can be found, necessary for the "load" command
var BatchSource string

// HostKeySigner is used for the ssh key presented to clients
var HostKeySigner ssh.Signer

// JobRunner manages all the details needed for keeping a list of pending
// background jobs, providing status of existing jobs, etc.
var JobRunner *queue.Queue

// dbPool is our single DB connection shared app-wide
var dbPool *sql.DB

func getEnvironment() {
	var errList []error
	var err error

	BABind = os.Getenv("BA_BIND")
	if BABind == "" {
		errList = append(errList, errors.New("BA_BIND must be set"))
	}

	ONILocation = os.Getenv("ONI_LOCATION")
	if ONILocation == "" {
		errList = append(errList, errors.New("ONI_LOCATION must be set"))
	} else {
		var info, err = os.Stat(ONILocation)
		if err == nil {
			if !info.IsDir() {
				err = errors.New("not a valid directory")
			}
		}
		if err != nil {
			errList = append(errList, fmt.Errorf("Invalid setting for ONI_LOCATION: %w", err))
		}
	}

	BatchSource = os.Getenv("BATCH_SOURCE")
	if BatchSource == "" {
		errList = append(errList, errors.New("BATCH_SOURCE must be set"))
	} else {
		var info, err = os.Stat(BatchSource)
		if err == nil {
			if !info.IsDir() {
				err = errors.New("not a valid directory")
			}
		}
		if err != nil {
			errList = append(errList, fmt.Errorf("Invalid setting for BATCH_SOURCE: %w", err))
		}
	}

	var fname = os.Getenv("HOST_KEY_FILE")
	if fname == "" {
		errList = append(errList, errors.New("HOST_KEY_FILE must be set"))
	} else {
		HostKeySigner, err = readKey(fname)
		if err != nil {
			errList = append(errList, fmt.Errorf("HOST_KEY_FILE is invalid or cannot be read: %w", err))
		}
	}

	var connect = os.Getenv("DB_CONNECTION")
	if connect == "" {
		errList = append(errList, errors.New(`DB_CONNECTION must be set (e.g., "user:pass@tcp(127.0.0.1:3306)/dbname")`))
	} else {
		dbPool, err = sql.Open("mysql", connect)
		if err != nil {
			errList = append(errList, fmt.Errorf(`DB_CONNECTION is invalid: %w`, err))
		}
	}

	if len(errList) > 0 {
		for _, err := range errList {
			fmt.Fprintf(os.Stderr, " - %s\n", err)
		}
		os.Exit(1)
	}

	dbPool.SetConnMaxLifetime(0)
	dbPool.SetMaxIdleConns(3)
	dbPool.SetMaxOpenConns(3)
}

func readKey(keyfile string) (ssh.Signer, error) {
	var data, err = os.ReadFile(keyfile)
	if os.IsNotExist(err) {
		slog.Warn("HOST_KEY_FILE doesn't exist; creating it with a random key", "path", keyfile)
		return generateKey(keyfile)
	}
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return ssh.ParsePrivateKey(data)
}

func writeKeyFiles(key *rsa.PrivateKey, filename string) error {
	var priv = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)

	var pub = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
		},
	)

	var err = os.WriteFile(filename, priv, 0600)
	if err != nil {
		return fmt.Errorf("writing private key to %q: %w", filename, err)
	}

	filename += ".pub"
	err = os.WriteFile(filename, pub, 0644)
	if err != nil {
		return fmt.Errorf("writing public key to %q: %w", filename, err)
	}

	return nil
}

func generateKey(filename string) (ssh.Signer, error) {
	var key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	err = writeKeyFiles(key, filename)
	if err != nil {
		return nil, fmt.Errorf("writing key files: %w", err)
	}

	return ssh.NewSignerFromKey(key)
}

func main() {
	getEnvironment()
	JobRunner = queue.New(ONILocation)

	var srv = &gliderssh.Server{Addr: BABind}
	srv.AddHostKey(HostKeySigner)

	var sessionID atomic.Uint64
	srv.Handle(func(_s gliderssh.Session) {
		var s = session{Session: _s, id: sessionID.Add(1)}

		s.logInfo("Connection established", "source", s.RemoteAddr(), "command", s.RawCommand())
		s.handle()
		s.logInfo("Session closed", "source", s.RemoteAddr(), "command", s.RawCommand())
	})

	var ctx, cancel = context.WithCancel(context.Background())
	trapIntTerm(func() {
		cancel()
		srv.Close()
		dbPool.Close()
	})
	go JobRunner.Wait(ctx)

	slog.Info("starting ssh server", "port", BABind, "BATCH_SOURCE", BatchSource, "ONI_LOCATION", ONILocation, "version", version.Version)
	var err = srv.ListenAndServe()
	if err != nil && err != gliderssh.ErrServerClosed {
		slog.Error("Unable to serve SSH", "error", err)
	}

	slog.Info("Closing...")
}
