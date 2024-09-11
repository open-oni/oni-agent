package main

import (
	"log/slog"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/batch-agent/version"
)

func main() {
	var srv = &ssh.Server{Addr: ":2222", Handler: nil}

	srv.Handle(func(s ssh.Session) {
		slog.Info("Connection established", "source", s.RemoteAddr(), "command", s.RawCommand(), "user", s.User())
	})

	slog.Info("starting ssh server", "port", 2222, "version", version.Version)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Unable to serve SSH", "error", err)
		os.Exit(1)
	}
}
