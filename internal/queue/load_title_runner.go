package queue

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/open-oni/oni-agent/internal/logstream"
	"github.com/open-oni/oni-agent/internal/venv"
)

// loadTitleRunner loads a title into ONI from a blob of XML data
type loadTitleRunner struct {
	xml     []byte
	stdout  logstream.Stream
	stderr  logstream.Stream
	marcdir string
	xmlfile string
	running atomic.Bool
	err     error
}

// newLoadTitleRunner returns a runner for loading a title into ONI. It takes
// the blob of XML, writes it to disk, then runs the ONI load_titles command.
func newLoadTitleRunner(xml []byte) *loadTitleRunner {
	return &loadTitleRunner{xml: xml}
}

// Start sets up all the data and kicks off the processing of the XML file
// followed by the ONI title load
func (r *loadTitleRunner) Start(ctx context.Context) error {
	go r.run(ctx)
	return nil
}

func (r *loadTitleRunner) run(ctx context.Context) {
	r.running.Store(true)
	defer r.running.Store(false)

	var err error
	r.marcdir, err = os.MkdirTemp("", "*-oni-marc")
	if err != nil {
		slog.Error("Unable to create temp dir", "error", err)
		r.err = fmt.Errorf("creating temp dir for MARC XML: %w", err)
		return
	}

	// Write the MARC record out and tell ONI to ingest it
	r.xmlfile = filepath.Join(r.marcdir, "marc.xml")
	err = os.WriteFile(r.xmlfile, r.xml, 0600)
	if err != nil {
		slog.Error("Unable to write MARC XML", "path", r.xmlfile, "error", err)
		r.err = fmt.Errorf("writing MARC XML: %w", err)
		return
	}

	var cmd = venv.Command(ctx, []string{"load_titles", r.marcdir})
	cmd.Stdout = &r.stdout
	cmd.Stderr = &r.stderr
	err = cmd.Run()
	if err != nil {
		slog.Error("Error ingesting MARC XML", "path", r.marcdir, "error", err)
		r.err = fmt.Errorf("running load_titles: %w", err)
		return
	}
}

// Wait wraps exec.Cmd.Wait, waiting for the command to exit and various stream
// copying to complete, setting the completed time if successful.
func (r *loadTitleRunner) Wait() error {
	for r.running.Load() {
		time.Sleep(time.Millisecond * 100)
	}

	if r.err != nil {
		return r.err
	}

	// We only remove the temp XML file and the temp marc dir if there were no
	// load errors. This can potentially leave a mess, but it's very useful for
	// debugging if things go wrong.
	os.Remove(r.xmlfile)
	os.Remove(r.marcdir)

	return nil
}

// Stdout returns the stdout stream captured from the ONI load_title command
func (r *loadTitleRunner) Stdout() logstream.Stream {
	return r.stdout
}

// Stderr returns the stderr stream captured from the ONI load_title command
func (r *loadTitleRunner) Stderr() logstream.Stream {
	return r.stderr
}
