package logstream

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var baseTime = time.Date(2024, 9, 25, 0, 0, 0, 123456789, time.UTC)

func gettf(addSeconds int) timeFunc {
	return func() time.Time {
		return baseTime.Add(time.Second * time.Duration(addSeconds))
	}
}

func TestWrite(t *testing.T) {
	type tcase struct {
		inputs   []string
		expected []string
	}
	// For each test we start our fake time function at the baseTime above. Each
	// write adds one second before writing, so the first write acts like it
	// happens at 00:00:01.
	var tests = map[string]tcase{
		"Multiple writes, no newlines": {
			inputs:   []string{"foo", "bar", "baz"},
			expected: []string{"[2024-09-25T00:00:03.123456789Z - 0001] foobarbaz"},
		},
		"Multiple writes with newlines, trailing write": {
			inputs:   []string{"foo\n", "bar\n", "baz"},
			expected: []string{
				"[2024-09-25T00:00:01.123456789Z - 0001] foo",
				"[2024-09-25T00:00:02.123456789Z - 0002] bar",
				"[2024-09-25T00:00:03.123456789Z - 0003] baz",
			},
		},
	}

	for name, tc := range tests {
		var offset = 0
		t.Run(name, func(t *testing.T) {
			var s = New()
			for _, inp := range tc.inputs {
				offset++
				timeNow = gettf(offset)
				s.Write([]byte(inp))
			}

			var got = s.Timestamped()
			var diff = cmp.Diff(got, tc.expected)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
