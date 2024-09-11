package main

import (
	"fmt"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/batch-agent/version"
)

type session struct {
	ssh.Session
	id uint64
}

func (s session) Printf(msg string, args ...any) (n int, err error) {
	return fmt.Fprintf(s, msg, args...)
}

func (s session) handle() {
	var cmds = s.Command()
	if len(cmds) == 0 {
		s.Printf("Missing command; terminating session\n")
		return
	}

	var command = cmds[0]
	switch command {
	case "version":
		s.Printf("Batch Agent version %s\n", version.Version)
		return

	case "load-batch":
		if len(cmds) < 2 {
			s.Printf("%q requires at least one batch name; terminating session\n", command)
			return
		}
		for _, batch := range cmds[1:] {
			s.Printf("Loading batch %q...\n", batch)
		}

	default:
		s.Printf("Unknown command %q\n", command)
	}
}
