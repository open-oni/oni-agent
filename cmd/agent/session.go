package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/oni-agent/version"
)

var sessionID atomic.Uint64

type session struct {
	ssh.Session
	id uint64
}

// Status is a string type the handler's "status" JSON may return
type Status string

// All possible statuses
const (
	StatusError   Status = "error"
	StatusSuccess Status = "success"
)

// H is a simple type alias for more easily building JSON responses
type H map[string]any

func (s session) logInfo(msg string, args ...any) {
	var combined = append([]any{"sessionID", s.id}, args...)
	slog.Info(msg, combined...)
}

func (s session) logError(msg string, args ...any) {
	var combined = append([]any{"sessionID", s.id}, args...)
	slog.Error(msg, combined...)
}

func (s session) respond(st Status, msg string, data H) {
	if data == nil {
		data = H{}
	}
	data["status"] = st
	data["session"] = H{"id": s.id}
	if msg != "" {
		data["message"] = msg
	}
	var b, err = json.Marshal(data)
	if err != nil {
		s.logError("Cannot marshal response", "error", err, "data", data)
		return
	}

	s.Write(b)
}

func (s session) handle() {
	var cmds = s.Command()
	if len(cmds) == 0 {
		s.respond(StatusError, "no command specified", nil)
		return
	}

	var command = cmds[0]
	switch command {
	case "version":
		s.respond(StatusSuccess, "", H{"version": version.Version})

	case "load-batch":
		if len(cmds) != 2 {
			s.respond(StatusError, fmt.Sprintf("%q requires exactly one batch name", command), nil)
			return
		}
		s.loadBatch(cmds[1])

	case "purge-batch":
		if len(cmds) != 2 {
			s.respond(StatusError, fmt.Sprintf("%q requires exactly one batch name", command), nil)
			return
		}
		s.purgeBatch(cmds[1])

	default:
		s.respond(StatusError, fmt.Sprintf("%q is not a valid command name", command), nil)
		return
	}
}

func (s session) loadBatch(name string) {
	s.queueJob("batch load", "load_batch", filepath.Join(BatchSource, name))
}

func (s session) purgeBatch(name string) {
	s.queueJob("batch purge", "purge_batch", name)
}

func (s session) queueJob(name string, command string, args ...string) {
	var combined = append([]string{command}, args...)
	var id = JobRunner.NewJob(combined...)

	s.respond(StatusSuccess, "Job added to queue", H{"job": H{"id": id}})
	s.close()
}

func (s session) close() {
	s.logInfo("Closing connection...")
	var err = s.Session.Close()
	if err != nil {
		s.logError("Error closing connection", "error", err)
	}
}
