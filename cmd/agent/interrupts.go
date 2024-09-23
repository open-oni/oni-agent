package main

import (
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

var isDone int32

// trapIntTerm catches interrupt and termination signals to let processes exit
// more cleanly.  A second signal of either type will immediately end the
// process, however.
func trapIntTerm(quit func()) {
	var sigInt = make(chan os.Signal, 1)
	signal.Notify(sigInt, syscall.SIGINT)
	signal.Notify(sigInt, syscall.SIGTERM)
	go func() {
		for range sigInt {
			if done() {
				slog.Warn("Force-interrupt detected; shutting down.")
				os.Exit(1)
			}

			slog.Info("Interrupt detected; attempting to clean up.  Another signal will immediately end the process.")
			atomic.StoreInt32(&isDone, 1)
			quit()
		}
	}()
}

func done() bool {
	return atomic.LoadInt32(&isDone) == 1
}
