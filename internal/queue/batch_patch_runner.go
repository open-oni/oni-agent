package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/open-oni/oni-agent/internal/batchfix"
	"github.com/open-oni/oni-agent/internal/batchpatch"
	"github.com/open-oni/oni-agent/internal/logstream"
	"github.com/spf13/afero"
)

// batchPatchRunner removes one or more issues from a batch by creating a new
// batch with the issues removed
type batchPatchRunner struct {
	q       *Queue
	fs      afero.Fs
	src     string
	dest    string
	bp      *batchpatch.BP
	running atomic.Bool
	stdout  logstream.Stream
	stderr  logstream.Stream
	err     error
}

// newBatchPatchRunner returns a runner for modifying a batch, creating a new
// one with an incremented version, then doing a purge and ingest to replace
// the old with the new.
func newBatchPatchRunner(q *Queue, fs afero.Fs, src, dest string, bp *batchpatch.BP) *batchPatchRunner {
	return &batchPatchRunner{q: q, fs: fs, src: src, dest: dest, bp: bp}
}

// Start sets up all the data and kicks off the batch patch
func (r *batchPatchRunner) Start(ctx context.Context) error {
	r.running.Store(true)
	go r.run(ctx)
	return nil
}

// runSub executes a single ONI command and merges logs into its "parent" (r)
func (r *batchPatchRunner) runSub(ctx context.Context, args ...string) error {
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

func (r *batchPatchRunner) run(ctx context.Context) {
	defer r.running.Store(false)

	var l = slog.With("source", "batchPatchRunner", "src", r.src, "dest", r.dest)
	l.Info("Runner starting...")
	var f, err = batchfix.NewFixer(r.fs, r.src, r.dest)
	if err != nil {
		l.Error("Unable to create batch fixer", "error", err)
		r.err = fmt.Errorf("creating batch fixer: %w", err)
		return
	}

	var removeKeys []string
	for op, item := range r.bp.Instructions() {
		if op == batchpatch.OpRemoveIssue {
			removeKeys = append(removeKeys, item)
		}
	}
	err = f.RemoveIssues(removeKeys)
	if err != nil {
		l.Error("Unable to remove issues", "error", err)
		r.err = fmt.Errorf("removing issues: %w", err)
		return
	}

	// Step 1: purge the original batch
	l.Info("Patch succeeded: purging original batch")
	err = r.runSub(ctx, "purge_batch", r.src)
	if err != nil {
		r.err = fmt.Errorf("purging original batch %q: %w", r.src, err)
		return
	}

	// Step 2: load the new batch: if there's no error we can exit here
	l.Info("Purge complete: loading new batch")
	err = r.runSub(ctx, "load_batch", r.dest)
	if err == nil {
		return
	}

	// Step 3: load failed: roll back and pray to the digital gods
	l.Error("Failed loading new batch. Reingesting original.", "error", err)
	var err2 = r.runSub(ctx, "load_batch", r.src)
	if err2 != nil {
		l.Error("ROLLBACK FAILED. Manual intervention required!", "load error", err, "rollback error", err2)
		r.err = fmt.Errorf("loading new batch %q: %w; rolling back to original batch %q: %w", r.dest, err, r.src, err2)
		return
	}

	r.err = fmt.Errorf("loading new batch %q: %w", r.dest, err)
}

// Wait waits for the batch patch to complete
func (r *batchPatchRunner) Wait() error {
	for r.running.Load() {
		time.Sleep(time.Millisecond * 100)
	}

	return r.err
}

// Returns a combined STDOUT log for the load and purge operations
func (r *batchPatchRunner) Stdout() logstream.Stream {
	return r.stdout
}

// Returns a combined STDERR log for the load and purge operations
func (r *batchPatchRunner) Stderr() logstream.Stream {
	return r.stderr
}
