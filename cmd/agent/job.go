package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type job struct {
	command     *exec.Cmd
	err         error
	stdout      bytes.Buffer
	stderr      bytes.Buffer
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
	j.err = j.command.Run()
	return j.err
}
