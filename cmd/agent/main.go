package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/open-oni/oni-agent/version"
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

func getEnvironment() {
	BABind = os.Getenv("BA_BIND")
	if BABind == "" {
		slog.Error("BA_BIND must be set")
		os.Exit(1)
	}

	ONILocation = os.Getenv("ONI_LOCATION")

	var info, err = os.Stat(ONILocation)
	if err == nil {
		if !info.IsDir() {
			err = errors.New("not a valid directory")
		}
	}
	if err != nil {
		slog.Error("Invalid setting for ONI_LOCATION", "error", err)
		os.Exit(1)
	}

	BatchSource = os.Getenv("BATCH_SOURCE")
	info, err = os.Stat(BatchSource)
	if err == nil {
		if !info.IsDir() {
			err = errors.New("not a valid directory")
		}
	}
	if err != nil {
		slog.Error("Invalid setting for BATCH_SOURCE", "error", err)
		os.Exit(1)
	}

	var fname = os.Getenv("HOST_KEY_FILE")
	if fname == "" {
		slog.Error("HOST_KEY_FILE must be set")
		os.Exit(1)
	}
	HostKeySigner, err = readKey(fname)
	if err != nil {
		slog.Error("HOST_KEY_FILE is invalid or cannot be read", "error", err)
		os.Exit(1)
	}
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

	var srv = &gliderssh.Server{Addr: BABind}
	srv.AddHostKey(HostKeySigner)

	var sessionID atomic.Uint64
	srv.Handle(func(_s gliderssh.Session) {
		var s = session{Session: _s, id: sessionID.Add(1)}

		s.logInfo("Connection established", "source", s.RemoteAddr(), "command", s.RawCommand())
		s.handle()
		s.logInfo("Process complete", "source", s.RemoteAddr(), "command", s.RawCommand())
	})

	slog.Info("starting ssh server", "port", BABind, "BATCH_SOURCE", BatchSource, "ONI_LOCATION", ONILocation, "version", version.Version)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Unable to serve SSH", "error", err)
		os.Exit(1)
	}
}
