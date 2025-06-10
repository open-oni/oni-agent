package queue

import (
	"context"
	"os/exec"

	"github.com/open-oni/oni-agent/internal/logstream"
	"github.com/open-oni/oni-agent/internal/venv"
)

// oniRunner is responsible for running `manage.py` commands against ONI
type oniRunner struct {
	cmd    *exec.Cmd
	args   []string
	stdout logstream.Stream
	stderr logstream.Stream
}

// newONIRunner returns a runner for calling `manage.py <args>`, using our venv
// package to get the right environment set up
func newONIRunner(args []string) *oniRunner {
	return &oniRunner{args: args}
}

// Start creates the command with the given context, starting the command and
// storing its start time. After calling start, wait must then be called to let
// the command finish and release resources.
func (r *oniRunner) Start(ctx context.Context) error {
	r.cmd = venv.Command(ctx, r.args)
	r.cmd.Stdout = &r.stdout
	r.cmd.Stderr = &r.stderr

	return r.cmd.Start()
}

// Wait wraps exec.Cmd.Wait, waiting for the command to exit and various stream
// copying to complete, setting the completed time if successful.
func (r *oniRunner) Wait() error {
	return r.cmd.Wait()
}

// Stdout returns the stdout stream captured from the ONI command
func (r *oniRunner) Stdout() logstream.Stream {
	return r.stdout
}

// Stderr returns the stderr stream captured from the ONI command
func (r *oniRunner) Stderr() logstream.Stream {
	return r.stderr
}
