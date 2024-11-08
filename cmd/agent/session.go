package main

import (
	"database/sql"
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

var sessionID atomic.Int64

type session struct {
	ssh.Session
	id int64
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
	case "send-marc":
		if len(args) != 1 {
			s.respond(StatusError, "You must supply an LCCN", nil)
			return
		}
		s.sendMARC(args[0])

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

	case "ensure-awardee":
		if len(args) < 1 || len(args) > 2 {
			s.respond(StatusError, fmt.Sprintf("%q requires one or two args: MARC org code and awardee name. Name is required if the awardee is to be auto-created.", command), nil)
			return
		}

		if len(args) == 1 {
			args = []string{args[0], ""}
		}
		s.ensureAwardee(args[0], args[1])

	default:
		s.respond(StatusError, fmt.Sprintf("%q is not a valid command name", command), nil)
		return
	}
}

func (s session) sendMARC(lccn string) {
	// Create a ~100k data-receiving buffer
	var data = make([]byte, 100_000)

	var marcData []byte
	for {
		var n, err = s.Read(data)
		if err != nil {
			slog.Error("Unable to read from client", "error", err)
			s.respond(StatusError, "Read error, connection terminating", H{"error": err.Error()})
			return
		}
		var got = data[:n]
		slog.Info("Got data", "size", n, "data", string(data[:n]))

		marcData = append(marcData, got...)
		var l = len(marcData)
		if l > 6 && string(marcData[l-6:]) == "\n\nEND\n" {
			marcData = marcData[:l-6]
			break
		}
	}

	// TODO: Parse the data to get the title and LCCN

	// TODO: Save XML file then tell ONI to try loading it

	slog.Info("Received data", "marc", string(marcData))
	s.respond(StatusSuccess, "MARC XML Received", nil)
}

func (s session) loadBatch(name string) {
	// ONI currently succeeds if a batch is already loaded and we try to load it
	// again, but this could change, so we explicitly ensure success here
	var exists, err = checkBatch(name)
	if err != nil {
		s.respond(StatusError, fmt.Sprintf("%q cannot be loaded", name), H{"error": err.Error()})
		return
	}
	if exists {
		s.respondNoJob()
		return
	}

	var batchPath = filepath.Join(BatchSource, name)
	err = validateBatch(batchPath)
	if err != nil {
		s.respond(StatusError, fmt.Sprintf("%q cannot be loaded", name), H{"error": err.Error()})
		return
	}
	s.queueJob("load_batch", batchPath)
}

func (s session) purgeBatch(name string) {
	// ONI will fail if you try to purge a batch which doesn't exist, but we want
	// to return success for idempotence of NCA jobs
	var exists, err = checkBatch(name)
	if err != nil {
		s.respond(StatusError, fmt.Sprintf("%q cannot be purged", name), H{"error": err.Error()})
		return
	}
	if !exists {
		s.respondNoJob()
		return
	}
	s.queueJob("purge_batch", name)
}

func (s session) getJob(arg string) (job *queue.Job, found bool) {
	var id, _ = strconv.ParseInt(arg, 10, 64)
	if id == 0 {
		s.respond(StatusError, fmt.Sprintf("%q is not a valid job id", arg), nil)
		return nil, false
	}

	// Allow fake jobs to get a response instead of an error so that automations
	// that haven't accounted for "no job needed" responses don't fail
	var noop = queue.NoOpJob()
	if id == noop.ID() {
		return noop, true
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

func (s session) respondNoJob() {
	s.respond(StatusSuccess, "No-op: job is redundant or already completed", H{"job": H{"id": queue.NoOpJob().ID()}})
}

func (s session) queueJob(command string, args ...string) {
	var combined = append([]string{command}, args...)
	var id = JobRunner.NewJob(combined...)

	s.respond(StatusSuccess, "Job added to queue", H{"job": H{"id": id}})
}

func (s session) ensureAwardee(code string, name string) {
	var rows, err = dbPool.Query("SELECT COUNT(*) FROM core_awardee WHERE org_code = ?", code)
	if err != nil {
		s.respond(StatusError, "Unable to query database", H{"error": err.Error()})
		return
	}
	defer rows.Close()

	// What does it mean if there's no error reported, but no count returned?
	if !rows.Next() {
		s.respond(StatusError, "Unable to count awardees in database", H{"error": "no rows returned by SQL COUNT()"})
		return
	}

	var count int
	err = rows.Scan(&count)
	if err != nil {
		s.respond(StatusError, "Unable to count awardees in database", H{"error": err.Error()})
		return
	}

	// We really only care that there's at least one row. If there are dupes,
	// that's out of scope to deal with, and technically not an error in terms of
	// what we need.
	if count > 0 {
		s.respond(StatusSuccess, "Awardee already exists", nil)
		return
	}

	// No rows, no error: if a name was given, create the awardee, otherwise abort
	if name == "" {
		s.respond(StatusError, "Unable to create awardee", H{"error": "awardee name must be given to auto-create awardees", "org_code": code, "name": name})
		return
	}

	var result sql.Result
	result, err = dbPool.Exec("INSERT INTO core_awardee (`org_code`, `name`, `created`) VALUES(?, ?, NOW())", code, name)
	if err != nil {
		s.respond(StatusError, "Unable to create awardee", H{"error": err.Error(), "org_code": code, "name": name})
		return
	}
	var n int64
	n, err = result.RowsAffected()
	if err != nil {
		s.respond(StatusError, "Unable to read result of INSERT", H{"error": err.Error(), "org_code": code, "name": name})
		return
	}
	if n != 1 {
		s.respond(StatusError, "Unable to create awardee", H{"error": "No rows created", "org_code": code, "name": name})
		return
	}

	s.respond(StatusSuccess, "Awardee created", nil)
	return
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
