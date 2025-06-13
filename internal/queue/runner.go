package queue

import (
	"context"

	"github.com/open-oni/oni-agent/internal/logstream"
)

// runner is an internal interface strictly for letting us have more
// flexibility in our job types. Stdout and Stderr are primarily for
// command-line tasks, but could be used for other tasks if there's a need to
// expose more information to callers.
type runner interface {
	Start(ctx context.Context) error
	Wait() error
	Stdout() logstream.Stream
	Stderr() logstream.Stream
}
