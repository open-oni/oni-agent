package batchpatch

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFromStream(t *testing.T) {
	var tests = map[string]struct {
		input       string
		expected    *BP
		expectError bool
	}{
		"valid stream": {
			input: "RemoveIssue 123\nRemoveIssue 456",
			expected: &BP{
				instructions: []*instruction{
					{operation: OpRemoveIssue, operand: "123"},
					{operation: OpRemoveIssue, operand: "456"},
				},
			},
			expectError: false,
		},
		"empty stream": {
			input:       "",
			expected:    &BP{},
			expectError: false,
		},
		"stream with blank lines": {
			input: "RemoveIssue 123\n\nRemoveIssue 456",
			expected: &BP{
				instructions: []*instruction{
					{operation: OpRemoveIssue, operand: "123"},
					{operation: OpRemoveIssue, operand: "456"},
				},
			},
			expectError: false,
		},
		"stream with whitespace": {
			input: "  RemoveIssue 123  \n\tRemoveIssue 456\t",
			expected: &BP{
				instructions: []*instruction{
					{operation: OpRemoveIssue, operand: "123"},
					{operation: OpRemoveIssue, operand: "456"},
				},
			},
			expectError: false,
		},
		"malformed instruction": {
			input:       "RemoveIssue",
			expected:    nil,
			expectError: true,
		},
		"invalid operation": {
			input:       "AddIssue 123",
			expected:    nil,
			expectError: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var r = strings.NewReader(tt.input)
			var got, err = FromStream(r)
			if (err != nil) != tt.expectError {
				t.Errorf("FromStream() error = %v, expectError %v", err, tt.expectError)
				return
			}
			var diff = cmp.Diff(got.String(), tt.expected.String())
			if diff != "" {
				t.Errorf("FromStream():\n%s", diff)
			}
		})
	}
}

func TestInstructions(t *testing.T) {
	var bp = &BP{
		instructions: []*instruction{
			{operation: OpRemoveIssue, operand: "123"},
			{operation: OpRemoveIssue, operand: "456"},
			{operation: OpRemoveIssue, operand: "789"},
		},
	}

	var expected = map[Operation][]string{
		OpRemoveIssue: {"123", "456", "789"},
	}

	var got = make(map[Operation][]string)
	for op, operand := range bp.Instructions() {
		got[op] = append(got[op], operand)
	}

	var diff = cmp.Diff(got, expected)
	if diff != "" {
		t.Errorf("Instructions():\n%s", diff)
	}
}
