// Package main shouldn't need a comment, but revive is *really* strict. Bro,
// this is obviously a "main" package, which means it's a command. Come on.
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
	"strings"
	"sync/atomic"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	_ "github.com/go-sql-driver/mysql"
	"github.com/open-oni/oni-agent/internal/queue"
	"github.com/open-oni/oni-agent/internal/venv"
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

// HostKeyFile is the path to the ssh key
var HostKeyFile string

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

	var envDir = func(env string) string {
		var dir = os.Getenv(env)
		if dir == "" {
			errList = append(errList, fmt.Errorf("%s must be set", env))
		} else {
			var info, err = os.Stat(dir)
			if err == nil {
				if !info.IsDir() {
					err = errors.New("not a valid directory")
				}
			}
			if err != nil {
				errList = append(errList, fmt.Errorf("Invalid setting for %s: %w", env, err))
			}
		}
		return dir
	}

	ONILocation = envDir("ONI_LOCATION")
	BatchSource = envDir("BATCH_SOURCE")

	HostKeyFile = os.Getenv("HOST_KEY_FILE")
	if HostKeyFile == "" {
		errList = append(errList, errors.New("HOST_KEY_FILE must be set"))
	} else {
		HostKeySigner, err = readKey(HostKeyFile)
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
	venv.Activate(ONILocation)
	JobRunner = queue.New()

	var srv = &gliderssh.Server{Addr: BABind}
	srv.AddHostKey(HostKeySigner)
	srv.MaxTimeout = time.Duration(5 * time.Minute)

	var sessionID atomic.Int64
	srv.Handle(func(_s gliderssh.Session) {
		var s = session{io: _s, id: sessionID.Add(1)}

		s.logInfo("Connection established", "source", _s.RemoteAddr(), "command", _s.RawCommand())
		s.handle()
		s.logInfo("Session closed", "source", _s.RemoteAddr(), "command", _s.RawCommand())
	})

	var ctx, cancel = context.WithCancel(context.Background())
	trapIntTerm(func() {
		cancel()
		srv.Close()
		dbPool.Close()
	})
	go JobRunner.Wait(ctx)

	// This functions as an on-startup sanity check to verify that the agent can
	// in fact call ONI commands with its current configuration
	slog.Info("Checking ONI install")
	var j = JobRunner.NewJob("ONI Check", []string{"check"})
	j.Run(ctx)
	switch j.Status() {
	case queue.StatusSuccessful:
		slog.Info("ONI check successful")
	case queue.StatusFailStart, queue.StatusFailed:
		slog.Error("ONI check failed", "error", strings.Join(j.Stderr(), ", "))
	default:
		slog.Error("Unhandled job status for ONI check job, terminating", "status", j.Status())
		os.Exit(1)
	}

	slog.Info("starting ssh server",
		"port", BABind,
		"ONI_LOCATION", ONILocation,
		"BATCH_SOURCE", BatchSource,
		"HOST_KEY_FILE", HostKeyFile,
		"version", version.Version,
	)
	var err = srv.ListenAndServe()
	if err != nil && err != gliderssh.ErrServerClosed {
		slog.Error("Unable to serve SSH", "error", err)
	}

	slog.Info("Closing...")
}
