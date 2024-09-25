// Package logstream provides an io.Writer which records a timestamp for when
// data was written so that raw output coming from a command-line app can be
// given more meaning
package logstream

import (
	"fmt"
	"strings"
	"time"
)

type Log struct {
	Timestamp time.Time
	Value     string
}

// String returns the entry with a prepended timestamp
func (l Log) String() string {
	return fmt.Sprintf("[%s] %s", l.Timestamp.Format(time.RFC3339Nano), l.Value)
}

type Stream struct {
	Logs        []Log
	lastWrite   time.Time
	unprocessed string
}

// New instantiates a new Stream ready for use as an io.Writer
func New() *Stream {
	return &Stream{}
}

type timeFunc func() time.Time

// timeNow gives us a way to mock time for testing; it is simply set to
// time.Now by default
var timeNow timeFunc = time.Now

// Write implements io.Writer's Write method, splitting up the written data by
// the OS line split character(s)
func (s *Stream) Write(data []byte) (n int, err error) {
	n = len(data)

	// This shouldn't happen, but who knows what crazy things might call a writer
	if n == 0 {
		return n, nil
	}

	var str = string(data)
	var lines = strings.Split(str, "\n")
	lines[0] = s.unprocessed + lines[0]

	// Grab anything after the final newline in the string. This works for all cases!
	//
	//   - If there are newlines and a partial line, it should be obvious why
	//     this works.
	//   - If there are no newlines, lines[0] is the only thing present, and it
	//     goes into the unprocessed list, leaving lines empty
	//   - If there isn't anything after the newline (the last character is a
	//     newline), the last "line" will actually be an empty string because of
	//     how strings.Split works.
	var end = len(lines) - 1
	lines, s.unprocessed = lines[:end], lines[end]

	s.lastWrite = timeNow()
	for _, line := range lines {
		s.Logs = append(s.Logs, Log{Timestamp: s.lastWrite, Value: line})
		s.lastWrite = s.lastWrite.Add(time.Nanosecond)
	}

	return n, nil
}

// Timestamped returns the captured output, prefixed with an RFC 3339-formatted
// timestamp per line. The final value, if present, is given the timestamp of
// when it was last written to.
func (s *Stream) Timestamped() []string {
	var out []string
	for _, log := range s.Logs {
		out = append(out, log.String())
	}
	if s.unprocessed != "" {
		var log = Log{Timestamp: s.lastWrite, Value: s.unprocessed}
		out = append(out, log.String())
	}

	return out
}
