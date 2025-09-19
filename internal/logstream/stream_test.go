package logstream

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var baseTime = time.Date(2024, 9, 25, 0, 0, 0, 987654321, time.UTC)

func gettf(addSeconds int) NowFunc {
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
			expected: []string{"[2024-09-25T00:00:03.987654321Z] foobarbaz"},
		},
		"Multiple writes with newlines, trailing write": {
			inputs: []string{"foo\n", "bar\n", "baz"},
			expected: []string{
				"[2024-09-25T00:00:01.987654321Z] foo",
				"[2024-09-25T00:00:02.987654321Z] bar",
				"[2024-09-25T00:00:03.987654321Z] baz",
			},
		},
		"Multiple logs in a single write, yielding fake nanosecond increments": {
			inputs: []string{"write 1 log 1\nwrite 1 log 2\nwrite 1 log 3\n", "write 2 log 1\nwrite 2 log 2\n"},
			expected: []string{
				"[2024-09-25T00:00:01.987654321Z] write 1 log 1",
				"[2024-09-25T00:00:01.987654322Z] write 1 log 2",
				"[2024-09-25T00:00:01.987654323Z] write 1 log 3",
				"[2024-09-25T00:00:02.987654321Z] write 2 log 1",
				"[2024-09-25T00:00:02.987654322Z] write 2 log 2",
			},
		},
		"Nanosecond log prefix always shows all nine digits": {
			inputs: []string{"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"},
			expected: []string{
				"[2024-09-25T00:00:01.987654321Z] 1",
				"[2024-09-25T00:00:01.987654322Z] 2",
				"[2024-09-25T00:00:01.987654323Z] 3",
				"[2024-09-25T00:00:01.987654324Z] 4",
				"[2024-09-25T00:00:01.987654325Z] 5",
				"[2024-09-25T00:00:01.987654326Z] 6",
				"[2024-09-25T00:00:01.987654327Z] 7",
				"[2024-09-25T00:00:01.987654328Z] 8",
				"[2024-09-25T00:00:01.987654329Z] 9",
				"[2024-09-25T00:00:01.987654330Z] 10",
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

func TestMerge(t *testing.T) {
	type tcase struct {
		s1       *Stream
		s2       *Stream
		expected []string
	}

	var s1 = New()
	timeNow = gettf(1)
	s1.Write([]byte("s1 log 1\n"))
	timeNow = gettf(2)
	s1.Write([]byte("s1 log 2\n"))
	timeNow = gettf(3)
	s1.Write([]byte("s1 log 3 - unprocessed"))

	var s2 = New()
	timeNow = gettf(4)
	s2.Write([]byte("s2 log 1\n"))
	timeNow = gettf(5)
	s2.Write([]byte("s2 log 2\n"))
	timeNow = gettf(6)
	s2.Write([]byte("s2 log 3 - unprocessed"))

	var s3 = New()
	timeNow = gettf(10)
	s3.Write([]byte("s3 log 1\n"))

	var tests = map[string]tcase{
		"Simple merge": {
			s1: s1,
			s2: s2,
			expected: []string{
				"[2024-09-25T00:00:01.987654321Z] s1 log 1",
				"[2024-09-25T00:00:02.987654321Z] s1 log 2",
				"[2024-09-25T00:00:03.987654321Z] s1 log 3 - unprocessed",
				"[2024-09-25T00:00:04.987654321Z] s2 log 1",
				"[2024-09-25T00:00:05.987654321Z] s2 log 2",
				"[2024-09-25T00:00:06.987654321Z] s2 log 3 - unprocessed",
			},
		},
		"Merge into empty": {
			s1: New(),
			s2: s3,
			expected: []string{
				"[2024-09-25T00:00:10.987654321Z] s3 log 1",
			},
		},
		"Merge with empty": {
			s1: s3,
			s2: New(),
			expected: []string{
				"[2024-09-25T00:00:10.987654321Z] s3 log 1",
			},
		},
		"Merge empty streams": {
			s1:       New(),
			s2:       New(),
			expected: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var merged = tc.s1.Merge(*tc.s2)
			var got = merged.Timestamped()
			var diff = cmp.Diff(got, tc.expected)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
