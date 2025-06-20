package queue

import (
	"context"
	"fmt"
	"log/slog"
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

// Job is the top-level "thing" that holds job data. Jobs are usually used for
// command-line tasks, but can also do more generalized logic.
type Job struct {
	id          int64
	status      JobStatus
	runner      runner
	name        string
	args        []string
	queuedAt    time.Time
	startedAt   time.Time
	completedAt time.Time
	purgeAt     time.Time
	err         error
}

// NoOpJob returns a job that does nothing and has a success status
func NoOpJob() *Job {
	return &Job{
		id:          -1,
		name:        "No-op job",
		status:      StatusSuccessful,
		args:        nil,
		queuedAt:    time.Now(),
		startedAt:   time.Now(),
		completedAt: time.Now(),
		err:         nil,
	}
}

// Start kicks off this job's runner
func (j *Job) Start(ctx context.Context) error {
	var logger = slog.With("id", j.id, "command", j.args)
	logger.Info("Starting job", "id", j.id, "command", j.args)

	if j.runner == nil {
		j.err = fmt.Errorf("job has no runner")
		logger.Error("Unable to start job", "error", j.err)
		j.status = StatusFailStart
		j.purgeAt = time.Now().Add(time.Hour * 24)
		return j.err
	}

	j.err = j.runner.Start(ctx)
	if j.err != nil {
		logger.Error("Unable to start job", "error", j.err)
		j.status = StatusFailStart
		j.purgeAt = time.Now().Add(time.Hour * 24)
		return j.err
	}

	j.status = StatusStarted
	logger.Info("Job started successfully", "id", j.id, "command", j.args)

	j.startedAt = time.Now()
	return nil
}

// Wait checks the runner's state, waiting for it to finish if it's in a valid
// state, and stores relevant data (logs, success/failure, etc.) on completion.
func (j *Job) Wait() error {
	var logger = slog.With("id", j.id, "command", j.args)

	if j.err != nil {
		logger.Error("Invalid job state in Job.Wait: job already has an error from a previous operation", "error", j.err)
		return fmt.Errorf("waiting for job completion: cannot start due to previous error: %w", j.err)
	}
	if j.startedAt.IsZero() {
		logger.Error("Invalid job state in Job.Wait: job has not been started", "error", j.err)
		return fmt.Errorf("waiting for job completion: Start must first be called")
	}

	j.err = j.runner.Wait()
	if j.err != nil {
		logger.Error("Job failed", "error", j.err)
		j.status = StatusFailed
		j.purgeAt = time.Now().Add(time.Hour * 24)
		return j.err
	}

	j.status = StatusSuccessful
	j.completedAt = time.Now()
	j.purgeAt = time.Now().Add(time.Hour * 24 * 7)
	logger.Info("Job complete")
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

// Name returns the human-friendly name of the job
func (j *Job) Name() string {
	return j.name
}

// QueuedAt returns when the job was created (sent to the job queue)
func (j *Job) QueuedAt() time.Time {
	return j.queuedAt
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

// Stdout returns the runner's output, if any, with timestamp prefixes
func (j *Job) Stdout() []string {
	return j.runner.Stdout().Timestamped()
}

// Stderr returns the runner's errors, if any, with timestamp prefixes
func (j *Job) Stderr() []string {
	return j.runner.Stderr().Timestamped()
}
