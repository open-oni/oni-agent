package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
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

// testResponse holds response data that we receive so we can do a deep compare
type testResponse struct {
	Status  Status
	Session struct {
		ID int64
	}
	Version string `json:"version"`
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

func TestSession_VersionCommand(t *testing.T) {
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
		Session: struct{ ID int64 }{ID: 123},
	}

	var out = cmp.Diff(expected, got)
	if out != "" {
		t.Errorf(out)
	}
}
