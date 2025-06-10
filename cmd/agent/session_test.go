package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		t.Fatalf("Failed to unmarshal response JSON: %v\nRaw output: %s", err, m.output.String())
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
		expectedResp *testResponse
	}{
		"read error": {
			name:       "read error",
			inputError: fmt.Errorf("simulated read error"),
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusError,
				Message: "Read error, connection terminating",
				Error:   "simulated read error",
			},
		},
		"invalid xml": {
			name:       "invalid xml",
			inputData:  "<root><invalid></root>\n\nEND\n",
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusError,
				Message: "Invalid data",
				Error:   "XML syntax error on line 1: element <invalid> closed by </root>",
			},
		},
		"successful load": {
			name:       "successful load",
			inputData:  "<root><title>Test Title</title></root>\n\nEND\n",
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusSuccess,
				Message: "MARC XML Received",
				Job: jobResponse{
					Stdout: []string{`[2024-09-25T00:00:01.987654321Z] Loading titles from XML: "<root><title>Test Title</title></root>"`},
				},
			},
		},
		"failed load": {
			name:       "failed load",
			inputData:  "<root>fail</root>\n\nEND\n",
			expectedResp: &testResponse{
				Session: sessionResponse{ID: 456},
				Status:  StatusError,
				Message: "Internal error, unable to ingest MARC",
				Error:   "exit status 1",
				Job: jobResponse{
					Stdout: []string{`[2024-09-25T00:00:01.987654321Z] You asked for failure, bruh!`},
				},
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
				t.Errorf("Exit() should have been called")
			}
			if mockIO.exitCode != 0 {
				t.Errorf("Expected exit code 0, got %d", mockIO.exitCode)
			}

			var got = mockIO.getResponseData(t)
			var diff = cmp.Diff(tc.expectedResp, got)
			if diff != "" {
				t.Errorf(diff)
			}
		})
	}
}
