package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	data["sessionID"] = s.id
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
		return

	case "load":
		if len(cmds) != 2 {
			s.respond(StatusError, fmt.Sprintf("%q requires exactly one batch name", command), nil)
			return
		}
		s.loadBatch(cmds[1])

	case "purge":
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
	s.respond(StatusSuccess, "starting "+name, H{"name": name, "command": command, "args": strings.Join(args, ",")})

	var combined = append([]string{command}, args...)
	var cmd = newManageCommand(combined...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	go func() {
		var err = cmd.Run()
		if err == nil {
			s.logInfo("Command completed", "name", name, "command", combined, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
		} else {
			s.logError("Command failed", "name", name, "command", combined, "error", err, "STDOUT", string(stdout.Bytes()), "STDERR", string(stderr.Bytes()))
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
