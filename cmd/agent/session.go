package main

import (
	"bytes"
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

	case "load":
		if len(cmds) != 2 {
			s.Printf("%q requires exactly one batch name; terminating session\n", command)
			return
		}
		s.loadBatch(cmds[1])

	case "purge":
		if len(cmds) != 2 {
			s.Printf("%q requires exactly one batch name; terminating session\n", command)
			return
		}
		s.purgeBatch(cmds[1])

	default:
		s.Printf("Unknown command %q\n", command)
	}
}

func (s session) loadBatch(name string) {
	s.Printf("Loading batch %q...\n", name)
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
	s.Printf("Purging batch %q...\n", name)
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
