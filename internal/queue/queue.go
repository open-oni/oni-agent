// Package queue manages a simple in-memory job queue for spawning, running,
// and storing logs from ONI management commands
package queue

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Queue holds the list of ONI jobs we need to run
type Queue struct {
	m      sync.RWMutex
	seq    int64
	lookup map[int64]*Job
	queue  chan *Job
}

// New provides a new job queue
func New() *Queue {
	return &Queue{lookup: make(map[int64]*Job), queue: make(chan *Job, 1000)}
}

func (q *Queue) newJob(name string, r runner) *Job {
	q.m.Lock()
	defer q.m.Unlock()

	// Since we don't know when a job will run, we give new jobs a wide berth for
	// purging: if the purge isn't set elsewhere for some odd reason, they'll
	// still get cleaned up after a month.
	var purgeTime = time.Now().Add(time.Hour * 24 * 30)
	q.seq++
	var j = &Job{
		id:      q.seq,
		name:    name,
		status:  StatusPending,
		runner:  r,
		purgeAt: purgeTime,
	}
	q.lookup[j.id] = j

	return j
}

// Push appends j to the job queue
func (q *Queue) Push(j *Job) {
	j.queuedAt = time.Now()
	q.queue <- j
}

// NewONIJob returns a Job set up to do a single ONI `manage.py` call with the
// given args
func (q *Queue) NewONIJob(name string, args []string) *Job {
	return q.newJob(name, newONIRunner(args))
}

// NewLoadTitleJob returns a job set up to load a title into ONI, as described
// in the raw MARC xml passed in
func (q *Queue) NewLoadTitleJob(xml []byte) *Job {
	return q.newJob("load title", newLoadTitleRunner(xml))
}

// GetJob returns a job by its id
func (q *Queue) GetJob(id int64) *Job {
	q.m.RLock()
	defer q.m.RUnlock()

	return q.lookup[id]
}

// AllJobs returns all jobs currently stored in memory
func (q *Queue) AllJobs() []*Job {
	q.m.RLock()
	defer q.m.RUnlock()

	var list []*Job
	for _, j := range q.lookup {
		list = append(list, j)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].queuedAt.Before(list[j].queuedAt)
	})

	return list
}

func (q *Queue) purgeOldJobs() {
	q.m.Lock()
	defer q.m.Unlock()

	var now = time.Now()
	for id, j := range q.lookup {
		if now.After(j.purgeAt) {
			delete(q.lookup, id)
		}
	}
}

// Wait runs until ctx is canceled, watching for new jobs that need to be
// queued up
func (q *Queue) Wait(ctx context.Context) {
	var lastPurgeCheck time.Time
	for {
		select {
		case j := <-q.queue:
			// We ignore errors here, as they're already logged by the job itself,
			// and nothing can be done about them anyway
			_ = j.Run(ctx)
		case <-ctx.Done():
			return
		default:
			if time.Since(lastPurgeCheck) > time.Hour {
				q.purgeOldJobs()
				lastPurgeCheck = time.Now()
			}
			time.Sleep(time.Second)
		}
	}
}
