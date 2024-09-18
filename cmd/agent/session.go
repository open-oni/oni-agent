package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/oni-agent/internal/queue"
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

	case "job-status":
		if len(cmds) != 2 {
			s.respond(StatusError, "You must supply a job ID", nil)
			return
		}
		s.getJobStatus(cmds[1])

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
	s.queueJob("load_batch", filepath.Join(BatchSource, name))
}

func (s session) purgeBatch(name string) {
	s.queueJob("purge_batch", name)
}

func (s session) getJobStatus(arg string) {
	var id, _ = strconv.ParseUint(arg, 10, 64)
	if id == 0 {
		s.respond(StatusError, fmt.Sprintf("%q is not a valid job id", arg), nil)
		return
	}

	var j = JobRunner.GetJob(id)
	if j == nil {
		s.respond(StatusError, "Job not found", H{"job": H{"id": id}})
		return
	}

	switch j.Status() {
	case queue.StatusPending:
		s.respond(StatusSuccess, "Pending: this job is in the queue but hasn't been started yet.", nil)
	case queue.StatusStarted:
		s.respond(StatusSuccess, "Started: this job is currently running.", nil)
	case queue.StatusFailStart:
		s.respond(StatusSuccess, "Invalid: this job was not able to start.", H{"error": j.Error()})
	case queue.StatusSuccessful:
		s.respond(StatusSuccess, "Success: this job is complete.", nil)
	case queue.StatusFailed:
		s.respond(StatusSuccess, "Failed: this job started but returned a non-zero exit code.", H{"error": j.Error()})
	default:
		s.logError("Invalid job status", "jobID", j.ID(), "jobStatus", j.Status())
		s.respond(StatusError, "Internal error: unknown job status", nil)
	}
}

func (s session) queueJob(command string, args ...string) {
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
