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
	"github.com/open-oni/oni-agent/internal/version"
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
	s.close()
}

func (s session) handle() {
	var parts = s.Command()
	if len(parts) == 0 {
		s.respond(StatusError, "no command specified", nil)
		return
	}

	var command, args = parts[0], parts[1:]
	switch command {
	case "version":
		s.respond(StatusSuccess, "", H{"version": version.Version})

	case "job-status":
		if len(args) != 1 {
			s.respond(StatusError, "You must supply a job ID", nil)
			return
		}
		s.getJobStatus(args[0])

	case "job-logs":
		if len(args) != 1 {
			s.respond(StatusError, "You must supply a job ID", nil)
			return
		}
		s.getJobLogs(args[0])

	case "load-batch":
		if len(args) != 1 {
			s.respond(StatusError, fmt.Sprintf("%q requires exactly one batch name", command), nil)
			return
		}
		s.loadBatch(args[0])

	case "purge-batch":
		if len(args) != 1 {
			s.respond(StatusError, fmt.Sprintf("%q requires exactly one batch name", command), nil)
			return
		}
		s.purgeBatch(args[0])

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

func (s session) getJob(arg string) (job *queue.Job, found bool) {
	var id, _ = strconv.ParseUint(arg, 10, 64)
	if id == 0 {
		s.respond(StatusError, fmt.Sprintf("%q is not a valid job id", arg), nil)
		return nil, false
	}

	var j = JobRunner.GetJob(id)
	if j == nil {
		s.respond(StatusError, "Job not found", H{"job": H{"id": id}})
		return nil, false
	}

	return j, true
}

func (s session) getJobStatus(arg string) {
	var j, found = s.getJob(arg)
	if !found {
		return
	}

	var jobdata = H{"id": j.ID(), "status": j.Status()}
	var status = StatusSuccess
	var message string

	switch j.Status() {
	case queue.StatusPending:
		message = "Pending: this job is in the queue but hasn't been started yet."
	case queue.StatusStarted:
		message = "Started: this job is currently running."
	case queue.StatusFailStart:
		jobdata["error"] = j.Error()
		message = "Invalid: this job was not able to start."
	case queue.StatusSuccessful:
		message = "Success: this job is complete."
	case queue.StatusFailed:
		jobdata["error"] = j.Error()
		message = "Failed: this job started but returned a non-zero exit code."
	default:
		s.logError("Invalid job status", "jobID", j.ID(), "jobStatus", j.Status())
		status = StatusError
		message = "Internal error: unknown job status"
	}

	s.respond(status, message, H{"job": jobdata})
}

func (s session) getJobLogs(arg string) {
	var j, found = s.getJob(arg)
	if !found {
		return
	}

	var out = H{"job": H{
		"id":     j.ID(),
		"status": j.Status(),
		"stdout": j.Stdout(),
		"stderr": j.Stderr(),
	}}
	s.respond(StatusSuccess, "", out)
}

func (s session) queueJob(command string, args ...string) {
	var combined = append([]string{command}, args...)
	var id = JobRunner.NewJob(combined...)

	s.respond(StatusSuccess, "Job added to queue", H{"job": H{"id": id}})
}

// close terminates the session, always with a status of 0: Go ssh clients
// return an error if the request is anything but successful, so the caller has
// to parse the status instead.
func (s session) close() {
	s.logInfo("Closing connection...")
	var err = s.Session.Exit(0)
	if err != nil {
		s.logError("Error closing connection", "error", err)
	}
}
