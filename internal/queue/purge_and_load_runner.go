package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/open-oni/oni-agent/internal/logstream"
)

// purgeAndLoadRunner is sort of a two-command [oniRunner] with extra
// complexity to handle failure. We purge one batch and then load another,
// basically for batch fixes. If the load fails, we have to reload the
// original. Batch loads from a patched batch should almost never fail, but we
// can't rely on that assumption.
type purgeAndLoadRunner struct {
	originalBatchPath string
	newBatchPath      string
	stdout            logstream.Stream
	stderr            logstream.Stream
	running           atomic.Bool
	err               error
}

func newPurgeAndLoadRunner(originalBatchPath, newBatchPath string) *purgeAndLoadRunner {
	return &purgeAndLoadRunner{
		originalBatchPath: originalBatchPath,
		newBatchPath:      newBatchPath,
	}
}

// Start kicks off the batch purge and load process
func (r *purgeAndLoadRunner) Start(ctx context.Context) error {
	r.running.Store(true)
	go r.run(ctx)
	return nil
}

// runSub executes a single ONI command and merges logs into its "parent" (r)
func (r *purgeAndLoadRunner) runSub(ctx context.Context, args ...string) error {
	var sub = newONIRunner(args)
	var err = sub.Start(ctx)
	if err != nil {
		return fmt.Errorf("starting ONI sub-job for %q: %w", args, err)
	}

	err = sub.Wait()
	r.stdout = r.stdout.Merge(sub.Stdout())
	r.stderr = r.stderr.Merge(sub.Stderr())

	return err
}

func (r *purgeAndLoadRunner) run(ctx context.Context) {
	defer r.running.Store(false)

	var l = slog.With("source", "purgeAndLoadRunner", "originalBatch", r.originalBatchPath, "newBatch", r.newBatchPath)

	// Step 1: purge the original batch
	l.Info("Purging original batch")
	var err = r.runSub(ctx, "purge_batch", r.originalBatchPath)
	if err != nil {
		r.err = fmt.Errorf("purging original batch %q: %w", r.originalBatchPath, err)
		return
	}

	// Step 2: load the new batch
	l.Info("Loading new batch")
	err = r.runSub(ctx, "load_batch", r.newBatchPath)
	if err == nil {
		return
	}

	// Step 3: load failed: roll back and pray to the digital gods
	l.Error("Failed loading new batch. Reingesting original.", "error", err)
	err = r.runSub(ctx, "load_batch", r.originalBatchPath)
	if err != nil {
		l.Error("ROLLBACK FAILED. Manual intervention required!", "error", err)
		r.err = fmt.Errorf("reloading original batch %q: %w", r.originalBatchPath, err)
		return
	}

	r.err = fmt.Errorf("loading new batch %q: %w", r.newBatchPath, err)
}

// Wait waits for the runner to complete
func (r *purgeAndLoadRunner) Wait() error {
	for r.running.Load() {
		time.Sleep(time.Millisecond * 100)
	}
	return r.err
}

// Stdout returns the stdout stream captured from the command
func (r *purgeAndLoadRunner) Stdout() logstream.Stream {
	return r.stdout
}

// Stderr returns the stderr stream captured from the command
func (r *purgeAndLoadRunner) Stderr() logstream.Stream {
	return r.stderr
}
