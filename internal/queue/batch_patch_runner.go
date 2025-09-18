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
	fs      afero.Fs
	src     string
	dest    string
	bp      *batchpatch.BP
	running atomic.Bool
	err     error
}

// newBatchPatchRunner returns a runner for modifying a batch, creating a new
// one with an incremented version, then doing a purge and ingest to replace
// the old with the new.
func newBatchPatchRunner(fs afero.Fs, src, dest string, bp *batchpatch.BP) *batchPatchRunner {
	return &batchPatchRunner{fs: fs, src: src, dest: dest, bp: bp}
}

// Start sets up all the data and kicks off the batch patch
func (r *batchPatchRunner) Start(ctx context.Context) error {
	r.running.Store(true)
	go r.run(ctx)
	return nil
}

func (r *batchPatchRunner) run(_ context.Context) {
	defer r.running.Store(false)

	var f, err = batchfix.NewFixer(r.fs, r.src, r.dest)
	if err != nil {
		slog.Error("Unable to create batch fixer", "error", err)
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
		slog.Error("Unable to remove issues", "error", err)
		r.err = fmt.Errorf("removing issues: %w", err)
		return
	}
}

// Wait waits for the batch patch to complete
func (r *batchPatchRunner) Wait() error {
	for r.running.Load() {
		time.Sleep(time.Millisecond * 100)
	}

	return r.err
}

// Stdout is required for the internal "runner" interface, but always returns
// an empty log stream
func (r *batchPatchRunner) Stdout() logstream.Stream {
	return logstream.Stream{}
}

// Stderr is required for the internal "runner" interface, but always returns
// an empty log stream
func (r *batchPatchRunner) Stderr() logstream.Stream {
	return logstream.Stream{}
}
