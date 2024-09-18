package queue

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Queue holds the list of ONI jobs we need to run
type Queue struct {
	m       sync.Mutex
	seq     uint64
	lookup  map[uint64]*Job
	binpath string
	env     []string
	queue   chan *Job
}

// New provides a new job queue
func New(oniPath string) *Queue {
	var binpath = filepath.Join(oniPath, "manage.py")
	var q = &Queue{lookup: make(map[uint64]*Job), queue: make(chan *Job, 1000), binpath: binpath}

	// We store the env vars needed to emulate Python's virtual environment,
	// which essentially operates by setting three env vars. There's other stuff
	// for changing the prompt, storing info for deactivation, etc., but this is
	// the only part that matters for executing the "manage.py" script:
	//
	//   - export VIRTUAL_ENV=/opt/openoni/ENV
	//   - export PATH="$VIRTUAL_ENV/bin:$PATH"
	//   - unset PYTHONHOME
	//
	// The last item is "free" because we just don't set anything to begin with

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
	path = append([]string{"/opt/openoni/ENV/bin"}, path...)
	q.env = append(q.env, "VIRTUAL_ENV=/opt/openoni/ENV")
	q.env = append(q.env, "PATH="+strings.Join(path, pathListSeparator))

	return q
}

func (q *Queue) NewJob(args ...string) uint64 {
	q.m.Lock()
	defer q.m.Unlock()

	q.seq++
	var j = &Job{
		args:     args,
		id:       q.seq,
		status:   StatusPending,
		queuedAt: time.Now(),
	}
	q.queue <- j

	return j.id
}

// Wait runs until ctx is canceled, watching for new jobs that need to be
// queued up
func (q *Queue) Wait(ctx context.Context) {
	for {
		select {
		case j := <-q.queue:
			q.run(ctx, j)
		case <-ctx.Done():
			return
		}
	}
}

func (q *Queue) run(ctx context.Context, j *Job) {
	var logger = slog.With("id", j.id, "command", j.args)
	var err = j.start(ctx, q.binpath, q.env)
	if err != nil {
		logger.Error("Unable to start job", "error", err)
		return
	}

	logger.Info("Started job")
	err = j.wait()
	if err != nil {
		logger.Error("Job failed", "error", err)
		return
	}
	logger.Info("Job complete")
}