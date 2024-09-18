package queue

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// JobStatus is a way to tell callers what's going on with any job in the queue
type JobStatus string

// All valid job statuses
const (
	StatusPending    JobStatus = "pending"
	StatusStarted    JobStatus = "started"
	StatusFailStart  JobStatus = "couldn't start"
	StatusSuccessful JobStatus = "successful"
	StatusFailed     JobStatus = "failed"
)

// Job represents a single ONI management job to be run
type Job struct {
	id          uint64
	status      JobStatus
	cmd         *exec.Cmd
	args        []string
	queuedAt    time.Time
	startedAt   time.Time
	completedAt time.Time
	err         error
	stdout      bytes.Buffer
	stderr      bytes.Buffer
	pid         int
}

// start creates the command with the given context, starting the command and
// storing its pid and start time. After calling start, wait must then be
// called to let the command finish and release resources.
func (j *Job) start(ctx context.Context, binpath string, env []string) error {
	j.cmd = exec.CommandContext(ctx, binpath, j.args...)
	j.cmd.Stdout = &j.stdout
	j.cmd.Stderr = &j.stderr
	j.cmd.Env = env

	j.err = j.cmd.Start()
	if j.err != nil {
		j.status = StatusFailStart
		return j.err
	}
	j.status = StatusStarted

	j.startedAt = time.Now()
	j.pid = j.cmd.Process.Pid
	return nil
}

// wait wraps exec.Cmd.Wait, waiting for the command to exit and various stream
// copying to complete, setting the completed time if successful.
func (j *Job) wait() error {
	if j.err != nil {
		return fmt.Errorf("waiting for job completion: cannot start due to previous error: %w", j.err)
	}
	if j.startedAt.IsZero() {
		return fmt.Errorf("waiting for job completion: Start must first be called")
	}

	j.err = j.cmd.Wait()
	if j.err != nil {
		j.status = StatusFailed
		return j.err
	}

	j.status = StatusSuccessful
	j.completedAt = time.Now()
	return nil
}

// ID returns the job's assigned ID number
func (j *Job) ID() uint64 {
	return j.id
}

// Status returns the job's status value
func (j *Job) Status() JobStatus {
	return j.status
}

// Error returns the first error which occurred when queueing, starting, or
// running the job
func (j *Job) Error() error {
	return j.err
}

func getOutput(buf bytes.Buffer) []string {
	var lines = strings.Replace(string(buf.Bytes()), "\r\n", "\n", -1)
	return strings.Split(lines, "\n")
}

// Stdout returns the captured output to STDOUT
func (j *Job) Stdout() []string {
	return getOutput(j.stdout)
}

// Stderr returns the captured output to STDERR
func (j *Job) Stderr() []string {
	return getOutput(j.stderr)
}
