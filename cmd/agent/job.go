package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type job struct {
	command     *exec.Cmd
	startedAt   time.Time
	completedAt time.Time
	err         error
	stdout      bytes.Buffer
	stderr      bytes.Buffer
	pid         int
}

func newJob(args ...string) *job {
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

	var j = &job{command: cmd}
	cmd.Stdout = &j.stdout
	cmd.Stderr = &j.stderr

	return j
}

// Run wraps exec.Cmd.Run, starting the job and waiting for it to finish, while
// populating data within the job as needed
func (j *job) Run() error {
	j.Start()
	if j.err == nil {
		j.Wait()
	}

	return j.err
}

// Run wraps exec.Cmd.Start, setting the start time and kicking off the command
// without waiting for it to finish. After calling Start, Wait must then be
// called to let the command finish and release resources.
func (j *job) Start() error {
	j.err = j.command.Start()
	if j.err != nil {
		return j.err
	}

	j.startedAt = time.Now()
	j.pid = j.command.Process.Pid
	return nil
}

// Wait wraps exec.Cmd.Wait, waiting for the command to exit and various stream
// copying to complete, setting the completed time if successful.
func (j *job) Wait() error {
	if j.err != nil {
		return fmt.Errorf("waiting for job completion: cannot start due to previous error: %w", j.err)
	}
	if j.startedAt.IsZero() {
		return fmt.Errorf("waiting for job completion: Start must first be called")
	}

	j.err = j.command.Wait()
	if j.err != nil {
		return j.err
	}

	j.completedAt = time.Now()
	return nil
}
