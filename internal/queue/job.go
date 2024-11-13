package queue

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/open-oni/oni-agent/internal/logstream"
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
	id          int64
	status      JobStatus
	cmd         *exec.Cmd
	bin         string
	args        []string
	env         []string
	queuedAt    time.Time
	startedAt   time.Time
	completedAt time.Time
	err         error
	stdout      logstream.Stream
	stderr      logstream.Stream
	pid         int
}

// NoOpJob returns a job that does nothing and has a success status
func NoOpJob() *Job {
	return &Job{
		id:          -1,
		status:      StatusSuccessful,
		cmd:         nil,
		args:        nil,
		queuedAt:    time.Now(),
		startedAt:   time.Now(),
		completedAt: time.Now(),
		err:         nil,
		stdout:      logstream.Stream{},
		stderr:      logstream.Stream{},
		pid:         -1,
	}
}

// Start creates the command with the given context, starting the command and
// storing its pid and start time. After calling start, wait must then be
// called to let the command finish and release resources.
func (j *Job) Start(ctx context.Context) error {
	j.cmd = exec.CommandContext(ctx, j.bin, j.args...)
	j.cmd.Stdout = &j.stdout
	j.cmd.Stderr = &j.stderr
	j.cmd.Env = j.env

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
func (j *Job) Wait() error {
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

// Run starts the job and waits for it to complete
func (j *Job) Run(ctx context.Context) error {
	var err = j.Start(ctx)
	if err == nil {
		err = j.Wait()
	}

	return err
}

// ID returns the job's assigned ID number
func (j *Job) ID() int64 {
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

// Stdout returns the captured output to STDOUT
func (j *Job) Stdout() []string {
	return j.stdout.Timestamped()
}

// Stderr returns the captured output to STDERR
func (j *Job) Stderr() []string {
	return j.stderr.Timestamped()
}
