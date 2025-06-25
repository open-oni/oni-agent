package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Workers just poll the job queue until it's done
type Worker struct {
	sync.Mutex
	ID    int
	queue chan *Job
	wg    *sync.WaitGroup
	done  bool
}

// Start listens for jobs until the work queue is closed
func (w *Worker) Start() {
	log.Printf("DEBUG: worker %d starting jobs", w.ID)
	w.wg.Add(1)
	for {
		select {
		case j := <-w.queue:
			w.process(j)
		default:
			w.Lock()
			var isDone = w.done
			w.Unlock()
			if isDone {
				log.Printf("INFO: worker %d exiting", w.ID)
				w.wg.Done()
				return
			}
			log.Printf("DEBUG: worker %d has nothing to do; sleeping", w.ID)
			time.Sleep(time.Second)
		}
	}
}

func (w *Worker) Done() {
	log.Printf("DEBUG: worker %d draining pool and exiting", w.ID)
	w.Lock()
	w.done = true
	w.Unlock()
}

func (w *Worker) process(j *Job) {
	if j.Failures > 0 {
		log.Printf("DEBUG: worker %d Processing %s Job for %q (retry #%d)", w.ID, j.Type, j.DestPath, j.Failures)
	} else {
		log.Printf("DEBUG: worker %d Processing %s Job for %q", w.ID, j.Type, j.DestPath)
	}
	switch j.Type {
	case FileCopy:
		w.CopyFile(j)
	default:
		log.Fatalf("FATAL: worker %d attempted to process a job of unknown type for %q", w.ID, j.DestPath)
	}
}

// retry will put the job back into the main queue unless it has already failed
// too many times
func (w *Worker) retry(j *Job, reasonFormat string, reasonArgs ...interface{}) {
	var reason = fmt.Sprintf(reasonFormat, reasonArgs...)
	j.Failures++
	if j.Failures >= 5 {
		log.Printf("ERROR: %s", reason)
		return
	}

	log.Printf("WARN: %s; trying again (retry #%d)", reason, j.Failures)
	w.queue <- j
}

// CopyFile just opens source and copies the contents to the destination path
func (w *Worker) CopyFile(j *Job) {
	var err = copyfile(j.SourcePath, j.DestPath)
	if err != nil {
		w.retry(j, err.Error())
	}
}
