// Package queue manages a simple in-memory job queue for spawning, running,
// and storing logs from ONI management commands
package queue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Queue holds the list of ONI jobs we need to run
type Queue struct {
	m       sync.Mutex
	seq     int64
	lookup  map[int64]*Job
	binpath string
	env     []string
	queue   chan *Job
}

// New provides a new job queue
func New(oniPath string) *Queue {
	var binpath = filepath.Join(oniPath, "manage.py")
	var q = &Queue{lookup: make(map[int64]*Job), queue: make(chan *Job, 1000), binpath: binpath}

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

// NewJob returns a Job set up to call ONI with the given args
func (q *Queue) NewJob(args ...string) *Job {
	q.m.Lock()
	defer q.m.Unlock()

	q.seq++
	var j = &Job{
		bin:    q.binpath,
		env:    q.env,
		args:   args,
		id:     q.seq,
		status: StatusPending,
	}
	q.lookup[j.id] = j

	return j
}

// QueueJob queues up a new ONI management command from the given args, and
// returns the queued job's id
func (q *Queue) QueueJob(args ...string) int64 {
	var j = q.NewJob(args...)
	j.queuedAt = time.Now()
	q.queue <- j

	return j.id
}

// GetJob returns a job by its id
func (q *Queue) GetJob(id int64) *Job {
	return q.lookup[id]
}

// Wait runs until ctx is canceled, watching for new jobs that need to be
// queued up
func (q *Queue) Wait(ctx context.Context) {
	for {
		select {
		case j := <-q.queue:
			// We ignore errors here, as they're already logged by the job itself,
			// and nothing can be done about them anyway
			_ = j.Run(ctx)
		case <-ctx.Done():
			return
		}
	}
}
