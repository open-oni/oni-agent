package main

import (
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/batch-agent/version"
)

func main() {
	var srv = &ssh.Server{Addr: ":2222"}

	var sessionID atomic.Uint64
	srv.Handle(func(_s ssh.Session) {
		var s = session{Session: _s, id: sessionID.Add(1)}

		slog.Info("Connection established", "source", s.RemoteAddr(), "command", s.RawCommand(), "user", s.User(), "id", s.id)
		s.handle()
		slog.Info("Connection closed", "source", s.RemoteAddr(), "command", s.RawCommand(), "user", s.User())
	})

	slog.Info("starting ssh server", "port", 2222, "version", version.Version)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("Unable to serve SSH", "error", err)
		os.Exit(1)
	}
}
