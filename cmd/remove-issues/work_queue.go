package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// JobType is an enumeration of the different types of jobs we handle
type JobType int

const (
	Unknown JobType = iota
	FileSkip
	FileCopy
)

func (jt JobType) String() string {
	switch jt {
	case FileSkip:
		return "skip file"
	case FileCopy:
		return "file copy"
	}
	return "Unknown"
}

// Job holds the source and destination paths for a file which needs to be
// processed, and the type of processing it needs
type Job struct {
	SourcePath string
	DestPath   string
	Type       JobType
	Failures   int
}

// The WorkQueue holds the workers and allows adding jobs and stopping the job
// collection process
type WorkQueue struct {
	workers  []*Worker
	queue    chan *Job
	wg       *sync.WaitGroup
	skipDirs []string
}

// NewWorkQueue creates n workers and starts them listening for jobs
func NewWorkQueue(ctx *FixContext, n int) *WorkQueue {
	var q = &WorkQueue{
		workers:  make([]*Worker, n),
		queue:    make(chan *Job, 100000),
		wg:       new(sync.WaitGroup),
		skipDirs: ctx.SkipDirs,
	}

	for i := 0; i < n; i++ {
		q.workers[i] = &Worker{
			ID:    i,
			queue: q.queue,
			wg:    q.wg,
		}
		go q.workers[i].Start()
	}

	return q
}

func (q *WorkQueue) Add(sourcePath, destDir, baseName string) {
	// Check if the directory is in the list of skipDirs
	for _, skipDir := range q.skipDirs {
		if strings.Contains(sourcePath, skipDir) {
			log.Printf("INFO: skipping file %q: matches skipDir %q", sourcePath, skipDir)
			return
		}
	}

	if baseName == "batch.xml" {
		log.Printf("INFO: skipping file %q: batch.XML", sourcePath)
		return
	}

	// Create the destination directory if it doesn't exist
	var err = os.MkdirAll(destDir, 0755)
	if err != nil {
		log.Printf("ERROR: could not create %q: %s", destDir, err)
		return
	}

	var ext = strings.ToLower(filepath.Ext(baseName)[1:])
	var destFile = filepath.Join(destDir, baseName)
	var job = &Job{SourcePath: sourcePath, DestPath: destFile}

	switch ext {
	case "xml":
		if baseName[len(baseName)-6:] == "_1.xml" {
			log.Printf("INFO: skipping file %q: validated XML", sourcePath)
			job.Type = FileSkip
		} else {
			job.Type = FileCopy
		}
	case "tif", "tiff":
		log.Printf("INFO: skipping file %q: TIFF", sourcePath)
		job.Type = FileSkip
	default:
		job.Type = FileCopy
	}

	if job.Type == FileSkip {
		return
	}

	log.Printf("INFO: queueing job for %q (destination %q, type %s)", sourcePath, destDir, job.Type)
	q.queue <- job
}

// Wait blocks until the queue is empty and all workers have quit
func (q *WorkQueue) Wait() {
	for _, w := range q.workers {
		w.Done()
	}
	q.wg.Wait()
}
