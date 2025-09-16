package queue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-oni/oni-agent/internal/venv"
)

var wd, testdir string

func getQ(t *testing.T) *Queue {
	var err error
	wd, err = os.Getwd()
	if err != nil {
		t.Fatalf("Unable to get working dir: %s", err)
	}
	testdir = filepath.Join(wd, "testdata")
	venv.Activate(testdir)
	return New()
}

func TestJobLifecycle(t *testing.T) {
	var q = getQ(t)
	var j = q.NewONIJob("test job", []string{"succeed"})

	if j.Status() != StatusPending {
		t.Errorf("expected status %s, got %s", StatusPending, j.Status())
	}

	if j.ID() <= 0 {
		t.Error("expected positive job ID")
	}

	if j.Name() != "test job" {
		t.Errorf("expected name %s, got %s", "test job", j.Name())
	}

	var fetchedJob = q.GetJob(j.ID())
	if fetchedJob != j {
		t.Error("GetJob returned wrong job")
	}

	// Run the job to test the command's environment
	var err = j.Run(context.Background())
	if err != nil {
		t.Errorf("Unable to run job: %s", err)
	}
	var hasVirtualEnv, hasPath bool
	for _, env := range j.runner.(*oniRunner).cmd.Env {
		var parts = strings.Split(env, "=")
		if len(parts) != 2 {
			t.Errorf("Unexpected ENV setting: %q", env)
		}

		var envdir = filepath.Join(testdir, "ENV")
		switch parts[0] {
		case "VIRTUAL_ENV":
			hasVirtualEnv = true
			if parts[1] != envdir {
				t.Errorf("Invalid VIRTUAL_ENV setting: expected %q but got %q", envdir, parts[1])
			}
		case "PATH":
			hasPath = true
			var bindir = filepath.Join(envdir, "bin")
			if !strings.Contains(parts[1], bindir) {
				t.Errorf("Invalid PATH setting: bin path %q to be included, but got %q", bindir, parts[1])
			}
		}
	}
	if !hasVirtualEnv {
		t.Error("VIRTUAL_ENV not set")
	}
	if !hasPath {
		t.Error("PATH not set")
	}

	// Validate post-job status
	if j.Status() != StatusSuccessful {
		t.Error("Job failed to run")
	}
}

func TestPush(t *testing.T) {
	var q = getQ(t)
	var j = q.NewONIJob("test job", []string{"arg1"})

	// Ensure the queue actually has the job
	j = q.GetJob(j.ID())
	if j == nil {
		t.Fatal("queued job not found")
	}

	if !j.queuedAt.IsZero() {
		t.Error("queuedAt shouldn't be set yet")
	}

	q.Push(j)

	if j.queuedAt.IsZero() {
		t.Error("queuedAt not set")
	}
}

func TestJobExecution_Success(t *testing.T) {
	var q = getQ(t)
	var j = q.NewONIJob("Test success", []string{"succeed"})
	var err = j.Run(context.Background())

	if err != nil {
		t.Errorf("job execution failed: %v", err)
	}

	if j.Status() != StatusSuccessful {
		t.Errorf("expected status %s, got %s", StatusSuccessful, j.Status())
	}

	var stdout = j.Stdout()
	if len(stdout) != 1 || !strings.Contains(stdout[0], "Yes!") {
		t.Error("unexpected stdout content")
	}
}

func TestJobExecution_Fail(t *testing.T) {
	var q = getQ(t)
	var j = q.NewONIJob("Test failure", []string{"fail"})
	var err = j.Run(context.Background())

	if err == nil {
		t.Error("expected error from failing job")
	}

	if j.Status() != StatusFailed {
		t.Errorf("expected status %s, got %s", StatusFailed, j.Status())
	}
}

func TestPurgeOldJobs(t *testing.T) {
	var q = New()
	var j = q.NewONIJob("test purge", []string{"arg1"})

	var id = j.ID()
	if q.GetJob(id) != j {
		t.Error("job should be in the queue, but wasn't found")
	}

	q.purgeOldJobs()
	if q.GetJob(id) != j {
		t.Error("job shouldn't have been purged yet")
	}

	j.purgeAt = time.Now().Add(-time.Hour)
	q.purgeOldJobs()

	if q.GetJob(id) != nil {
		t.Error("job not purged")
	}
}

func TestAllJobs(t *testing.T) {
	var q = New()
	var j1 = q.NewONIJob("job1", []string{"arg1"})
	j1.queuedAt = time.Now()
	var j2 = q.NewONIJob("job2", []string{"arg2"})
	j2.queuedAt = j1.queuedAt.Add(-1 * time.Hour)

	var jobs = q.AllJobs()

	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}

	if jobs[0] != j2 || jobs[1] != j1 {
		t.Error("jobs not returned in correct order")
	}
}

func TestWaitInvalidState(t *testing.T) {
	var j = &Job{status: StatusPending}

	var err = j.Wait()
	if err == nil {
		t.Error("expected error when waiting without starting")
	}
}
