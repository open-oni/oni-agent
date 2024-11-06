package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestValidateBatch(t *testing.T) {
	var wd, err = os.Getwd()
	if err != nil {
		t.Fatalf("Unable to get working dir: %s", err)
	}
	var testpath = filepath.Join(wd, "testdata")

	var tests = map[string]struct {
		name        string
		expectError bool
		errorRegexp *regexp.Regexp
	}{
		"valid batch":        {name: "valid", expectError: false},
		"busted XML":         {name: "invalid-xml", expectError: true, errorRegexp: regexp.MustCompile(`^processing xml:`)},
		"bad issue":          {name: "missing-issues", expectError: true, errorRegexp: regexp.MustCompile(`no such file or directory`)},
		"invalid issue file": {name: "invalid-file", expectError: true, errorRegexp: regexp.MustCompile(`not a regular file`)},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var err = validateBatch(filepath.Join(testpath, tc.name))
			if tc.expectError {
				if err == nil {
					t.Fatalf("Loading %q: should have error, but no error was returned", tc.name)
				}
				if !tc.errorRegexp.MatchString(err.Error()) {
					t.Fatalf("Loading %q: error should match %q (got %q)", tc.name, tc.errorRegexp, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Loading %q: unexpected error: %s", tc.name, err)
				}
			}
		})
	}
}
