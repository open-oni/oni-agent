package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gliderlabs/ssh"
	"github.com/open-oni/oni-agent/version"
)

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

func (s session) respond(st Status, data H) {
	data["status"] = st
	var b, err = json.Marshal(data)
	if err != nil {
		slog.Error("marshaling data", "error", err, "data", data)
	}
	slog.Info("marshaling data", "error", err, "data", data)
	s.Write(b)
}

func (s session) handle() {
	var cmds = s.Command()
	if len(cmds) == 0 {
		s.respond(StatusError, H{"message": " no command specified"})
		return
	}

	var command = cmds[0]
	switch command {
	case "version":
		s.respond(StatusSuccess, H{"version": version.Version})
		return

	case "load":
		if len(cmds) != 2 {
			s.respond(StatusError, H{"command": command, "message": "requires exactly one batch name"})
			return
		}
		s.loadBatch(cmds[1])

	case "purge":
		if len(cmds) != 2 {
			s.respond(StatusError, H{"command": command, "message": "requires exactly one batch name"})
			return
		}
		s.purgeBatch(cmds[1])

	default:
		s.respond(StatusError, H{"command": command, "message": "unknown command"})
		return
	}
}

func (s session) loadBatch(name string) {
	s.respond(StatusSuccess, H{"message": "starting batch load", "batch": name})
	var cmd = newManageCommand("load_batch", filepath.Join(BatchSource, name))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	go func() {
		var err = cmd.Run()
		if err == nil {
			slog.Info("Successfully loaded batch", "name", name, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
		} else {
			slog.Info("Error running command", "error", err, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
		}
	}()
}

func (s session) purgeBatch(name string) {
	s.respond(StatusSuccess, H{"message": "starting batch purge", "batch": name})
	var cmd = newManageCommand("purge_batch", name)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	go func() {
		var err = cmd.Run()
		if err == nil {
			slog.Info("Successfully purged batch", "batch", name, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
		} else {
			slog.Info("Error purging batch", "batch", name, "error", err, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
		}
	}()
}

func newManageCommand(args ...string) *exec.Cmd {
	var manage = filepath.Join(ONILocation, "manage.py")
	var cmd = exec.Command(manage, args...)

	// Apply Python's virtual environment, which essentially operates by setting
	// three env vars. There's other stuff for changing the prompt, storing info
	// for deactivation, etc., but this is the only part that matters for
	// executing the "manage.py" script:
	//
	//   - export VIRTUAL_ENV=/opt/openoni/ENV
	//   - export PATH="$VIRTUAL_ENV/bin:$PATH"
	//   - unset PYTHONHOME
	var eVars = cmd.Environ()
	var path []string
	var pathListSeparator = string(os.PathListSeparator)
	for _, val := range eVars {
		var parts = strings.SplitN(val, "=", 2)
		if len(parts) < 2 {
			continue
		}
		if parts[0] == "PATH" {
			path = strings.Split(parts[1], pathListSeparator)
		}
	}
	path = append([]string{"/opt/openoni/ENV/bin"}, path...)
	cmd.Env = append(cmd.Env, "VIRTUAL_ENV=/opt/openoni/ENV")
	cmd.Env = append(cmd.Env, "PATH="+strings.Join(path, pathListSeparator))

	return cmd
}
