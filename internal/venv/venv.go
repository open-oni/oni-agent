package venv

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildEnvironment uses the passed in path to Open ONI and the calling
// environment's PATH to emulate Python's virtual environment.
//
// The virtual environment created by `bin/activate` is very simple, and really
// just relies on three environment variables:
//
//   - export VIRTUAL_ENV=/opt/openoni/ENV
//   - export PATH="$VIRTUAL_ENV/bin:$PATH"
//   - unset PYTHONHOME
//
// `activate` does make other changes, such as altering the bash prompt,
// storing info for the `bin/deactivate` script, etc., but these are irrelevant
// when non-interactively calling `manage.py`.
func buildVirtualEnv(oniPath string) []string {
	var eVars = os.Environ()
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
	var envPath = filepath.Join(oniPath, "ENV")
	var binPath = filepath.Join(envPath, "bin")
	path = append([]string{binPath}, path...)

	return []string{"VIRTUAL_ENV=" + envPath, "PATH=" + strings.Join(path, pathListSeparator)}
}

// env is our virtual environment and must be used to run any ONI commands
type env struct {
	binpath string
	env     []string
}

var venv *env

// Activate acts like a virtualenv `bin/activate` script, preparing an environment
// usable for ONI's management commands. This must be used before other
// functions are called!
func Activate(pth string) {
	venv = &env{
		binpath: filepath.Join(pth, "manage.py"),
		env:     buildVirtualEnv(pth),
	}
}

// Command returns an [exec.Cmd] instance set up with the activated
// environment, pointing to `manage.py`, and ready for use. If the environment
// has not been activated, nil will be returned, and it's your own fault.
func Command(ctx context.Context, args []string) *exec.Cmd {
	if venv == nil {
		slog.Error("venv.Command called without activation")
		return nil
	}

	var cmd = exec.CommandContext(ctx, venv.binpath, args...)
	cmd.Env = venv.env
	return cmd
}
