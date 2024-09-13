package main

import (
	"errors"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/oni-agent/version"
)

// BABind is the address and port to bind this process
var BABind string

// ONILocation is where Open ONI lives on the server, for invoking the
// management commands
var ONILocation string

// BatchSource is where batches can be found, necessary for the "load" command
var BatchSource string

func main() {
	BABind = os.Getenv("BA_BIND")
	if BABind == "" {
		slog.Error("BA_BIND must be set")
		os.Exit(1)
	}

	var srv = &ssh.Server{Addr: BABind}

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

	var sessionID atomic.Uint64
	srv.Handle(func(_s ssh.Session) {
		var s = session{Session: _s, id: sessionID.Add(1)}

		slog.Info("Connection established", "source", s.RemoteAddr(), "command", s.RawCommand(), "user", s.User(), "id", s.id)
		s.handle()
		slog.Info("Connection closed", "source", s.RemoteAddr(), "command", s.RawCommand(), "user", s.User())
	})

	slog.Info("starting ssh server", "port", BABind, "BATCH_SOURCE", BatchSource, "ONI_LOCATION", ONILocation, "version", version.Version)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Unable to serve SSH", "error", err)
		os.Exit(1)
	}
}
