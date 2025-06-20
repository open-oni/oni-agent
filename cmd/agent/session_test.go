package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/open-oni/oni-agent/internal/logstream"
	"github.com/open-oni/oni-agent/internal/queue"
	"github.com/open-oni/oni-agent/internal/venv"
	"github.com/open-oni/oni-agent/internal/version"
)

// mockSessionIO implements the sessionIO interface for testing purposes
type mockSessionIO struct {
	command    []string
	input      io.Reader
	output     *bytes.Buffer
	exitCode   int
	exitCalled bool
}

// newMockSessionIO creates a new mockSessionIO instance with the given
// command, but no input data
func newMockSessionIO(command []string) *mockSessionIO {
	return &mockSessionIO{
		command: command,
		input:   bytes.NewBuffer(nil),
		output:  bytes.NewBuffer(nil),
	}
}

// SetInputString sets the input buffer with a string
func (m *mockSessionIO) SetInputString(s string) {
	m.input = bytes.NewBufferString(s)
}

// errorReader is an io.Reader that always returns an error
type errorReader struct {
	err error
}

func (er *errorReader) Read(p []byte) (n int, err error) {
	return 0, er.err
}

// SetInputError sets the input to always return the given error
func (m *mockSessionIO) SetInputError(err error) {
	m.input = &errorReader{err: err}
}

// Command returns the predefined command for the mock.
func (m *mockSessionIO) Command() []string {
	return m.command
}

// Read reads data from the internal read buffer.
func (m *mockSessionIO) Read(data []byte) (int, error) {
	return m.input.Read(data)
}

// Write writes data to the internal write buffer, allowing it to be inspected later.
func (m *mockSessionIO) Write(data []byte) (int, error) {
	return m.output.Write(data)
}

// Exit records the exit code and marks that Exit was called.
func (m *mockSessionIO) Exit(code int) error {
	m.exitCode = code
	m.exitCalled = true
	return nil
}

type sessionResponse struct {
	ID int64
}

type jobResponse struct {
	ID     int64    `json:",omitempty"`
	Status string   `json:",omitempty"`
	Stdout []string `json:",omitempty"`
	Stderr []string `json:",omitempty"`
	Error  string
}

// testResponse holds response data that we receive so we can do a deep compare
type testResponse struct {
	Status  Status
	Message string
	Session sessionResponse
	Version string
	Error   string
	Job     jobResponse `json:",omitempty"`
}

// getResponseData unmarshals the mock session's output into a testResponse struct
func (m *mockSessionIO) getResponseData(t *testing.T) *testResponse {
	var resp testResponse
	var err = json.Unmarshal(m.output.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Failed to unmarshal response JSON: %v\nRaw output: %q", err, m.output.String())
	}
	return &resp
}

func setup(t *testing.T) {
	var wd, err = os.Getwd()
	if err != nil {
		t.Fatalf("Cannot get current working dir: %s", err)
	}

	ONILocation = filepath.Join(wd, "testdata", "session")
	venv.Activate(ONILocation)
	JobRunner = queue.New()
}

func TestSession_VersionCommand(t *testing.T) {
	setup(t)

	var mockIO = newMockSessionIO([]string{"version"})
	var s = session{io: mockIO, id: 123}

	s.handle()
	if !mockIO.exitCalled {
		t.Errorf("Exit() should have been called")
	}
	if mockIO.exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", mockIO.exitCode)
	}

	var got = mockIO.getResponseData(t)
	var expected = &testResponse{
		Status:  StatusSuccess,
		Version: version.Version,
		Session: sessionResponse{ID: 123},
		Job:     jobResponse{Stdout: nil, Stderr: nil, Error: ""},
	}

	var out = cmp.Diff(expected, got)
	if out != "" {
		t.Errorf(out)
	}
}

func TestSession_LoadTitleCommand(t *testing.T) {
	setup(t)

	var tests = map[string]struct {
		name         string
		inputData    string
		inputError   error
		expectJob    bool
		expectedResp *testResponse
		expectedLogs *testResponse
	}{
		"read error": {
			name:       "read error",
			inputError: fmt.Errorf("simulated read error"),
			expectJob:  false,
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusError,
				Message: "Read error, connection terminating",
				Error:   "simulated read error",
			},
		},
		"invalid xml": {
			name:      "invalid xml",
			inputData: "<root><invalid></root>\n\nEND\n",
			expectJob: false,
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusError,
				Message: "Invalid data",
				Error:   "XML syntax error on line 1: element <invalid> closed by </root>",
			},
		},
		"successful load": {
			name:      "successful load",
			inputData: "<root><title>Test Title</title></root>\n\nEND\n",
			expectJob: true,
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "Success: this job is complete.",
				Job: jobResponse{
					Status: "successful",
				},
			},
			expectedLogs: jobResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "foo",
				Stdout: []string{`[2024-09-25T00:00:01.987654321Z] Loading titles from XML: "<root><title>Test Title</title></root>"`},
			},
		},
		"failed load": {
			name:      "failed load",
			inputData: "<root>fail</root>\n\nEND\n",
			expectJob: true,
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "Failed: this job started but returned a non-zero exit code.",
				Job: jobResponse{
					Status: "failed",
					Error:  "running load_titles: exit status 1",
				},
			},
			expectedLogs: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "foo",
				Stdout: []string{`[2024-09-25T00:00:01.987654321Z] You asked for failure, bruh!`},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set time func for consistent logging
			var baseTime = time.Date(2024, 9, 25, 0, 0, 0, 987654321, time.UTC)
			var offset int64 = 0
			logstream.SetCustomNowFunction(func() time.Time {
				offset++
				return baseTime.Add(time.Second * time.Duration(offset))
			})

			var mockIO = newMockSessionIO([]string{"load-title"})
			if tc.inputError != nil {
				mockIO.SetInputError(tc.inputError)
			} else {
				mockIO.SetInputString(tc.inputData)
			}

			var s = session{io: mockIO, id: 456}
			s.handle()

			// Exit should always be called, and always with a zero status (see
			// [session.close] for details)
			if !mockIO.exitCalled {
				t.Fatal("Exit() should have been called")
			}
			if mockIO.exitCode != 0 {
				t.Fatalf("Expected exit code 0, got %d", mockIO.exitCode)
			}

			var got = mockIO.getResponseData(t)

			// If we don't expect a job, the test here is simple, so let's just do it
			// first and exit early
			if !tc.expectJob {
				var diff = cmp.Diff(tc.expectedResp, got)
				if diff != "" {
					t.Fatal(diff)
				}
				return
			}

			// We expect a job: we make sure we got one, poll until it's complete,
			// check its response, and then check its logs
			var jobid = got.Job.ID
			var expected = &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "Job added to queue",
				Job:     got.Job,
			}
			var diff = cmp.Diff(expected, got)
			if diff != "" {
				t.Fatal(diff)
			}

			// Wait for the job to finish and check the job-status response. We have
			// to hack in the job id since the expected response had no way to know
			// what it would be assigned
			var idstr = strconv.FormatInt(jobid, 10)
			got = awaitJob(t, idstr)
			tc.expectedResp.Job.ID = jobid
			diff = cmp.Diff(tc.expectedResp, got)
			if diff != "" {
				t.Fatal(diff)
			}

			// Request job logs and verify that response matches our expected job
			// logs response
			mockIO = newMockSessionIO([]string{"job-logs", idstr})
			s.handle()
			tc.expectedLogs.Job.ID = jobid
			diff = cmp.Diff(tc.expectedLogs, got)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

// awaitJob loops job-status calls until the response marks it as completed.
// The response is then returned.
func awaitJob(t *testing.T, id string) (resp *testResponse) {
	var st = queue.StatusPending
	var ctx, cancel = context.WithCancel(context.Background())
	go JobRunner.Wait(ctx)
	for st == queue.StatusPending || st == queue.StatusStarted {
		time.Sleep(time.Millisecond * 50)

		var mockIO = newMockSessionIO([]string{"job-status", id})
		var s = session{io: mockIO, id: 456}
		s.handle()
		resp = mockIO.getResponseData(t)
		slog.Info("Call complete", "response", resp)
		st = queue.JobStatus(resp.Job.Status)
	}

	cancel()
	return resp
}
