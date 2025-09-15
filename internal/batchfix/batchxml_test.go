package batchfix

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
)

func TestIssueXML_String(t *testing.T) {
	var i = &IssueXML{LCCN: "sn12345678", IssueDate: "1900-01-01", Edition: "01"}
	var expected = "sn12345678/1900-01-01"
	if i.String() != expected {
		t.Errorf(`Got %q, expected %q`, i.String(), expected)
	}
}

func Test_keyfix(t *testing.T) {
	var tests = map[string]struct {
		in       string
		expected string
	}{
		"Basic": {"sn12345678/1900-01-01_01", "sn12345678/1900010101"},
		"Weird": {"sn-1234-5678/1900_01_01-01", "sn12345678/1900010101"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got = keyfix(tt.in)
			if got != tt.expected {
				t.Errorf("Got %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestParseBatch(t *testing.T) {
	var batchXML = `
<ndnp:batch xmlns:ndnp="http://www.loc.gov/ndnp" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://www.loc.gov/ndnp" name="testbatch" awardee="awardee" awardYear="2023">
	<issue lccn="sn12345678" issueDate="1900-01-01" editionOrder="01">sn12345678/print/1900010101/1900010101.xml</issue>
	<issue lccn="sn12345678" issueDate="1900-01-02" editionOrder="01">sn12345678/print/1900010201/1900010201.xml</issue>
	<issue lccn="sn98765432" issueDate="1950-05-05" editionOrder="01">sn98765432/print/1950050501/1950050501.xml</issue>
	<reel reelNumber="1234">reels/1234.xml</reel>
</ndnp:batch>
`
	var pth = "batch.xml"
	var fs = afero.NewMemMapFs()
	afero.WriteFile(fs, pth, []byte(batchXML), 0644)

	var tests = map[string]struct {
		keys        []string
		expectError bool
		expected    *BatchXML
	}{
		"no skip keys": {
			expectError: false,
			expected: &BatchXML{
				Name:      "testbatch",
				Awardee:   "awardee",
				AwardYear: "2023",
				Issues: []*IssueXML{
					{LCCN: "sn12345678", IssueDate: "1900-01-01", Edition: "01", Path: "sn12345678/print/1900010101/1900010101.xml"},
					{LCCN: "sn12345678", IssueDate: "1900-01-02", Edition: "01", Path: "sn12345678/print/1900010201/1900010201.xml"},
					{LCCN: "sn98765432", IssueDate: "1950-05-05", Edition: "01", Path: "sn98765432/print/1950050501/1950050501.xml"},
				},
				Reels: []*reelXML{
					{ReelNum: "1234", Path: "reels/1234.xml"},
				},
			},
		},
		"one valid skip key": {
			keys:        []string{"sn12345678/1900-01-01_01"},
			expectError: false,
			expected: &BatchXML{
				Name:      "testbatch",
				Awardee:   "awardee",
				AwardYear: "2023",
				Issues: []*IssueXML{
					{LCCN: "sn12345678", IssueDate: "1900-01-01", Edition: "01", Path: "sn12345678/print/1900010101/1900010101.xml", Skip: true},
					{LCCN: "sn12345678", IssueDate: "1900-01-02", Edition: "01", Path: "sn12345678/print/1900010201/1900010201.xml"},
					{LCCN: "sn98765432", IssueDate: "1950-05-05", Edition: "01", Path: "sn98765432/print/1950050501/1950050501.xml"},
				},
				Reels: []*reelXML{
					{ReelNum: "1234", Path: "reels/1234.xml"},
				},
			},
		},
		"invalid skip key": {
			keys:        []string{"snbogus/1900-01-01_01"},
			expectError: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var b, err = ParseBatch(fs, pth, tt.keys)
			if err != nil {
				if tt.expectError {
					return
				}
				t.Fatalf("Expected success, got %#v", err)
			}
			if tt.expectError {
				t.Fatalf("Expected error, got nil")
			}

			var diff = cmp.Diff(tt.expected, b)
			if diff != "" {
				t.Error(diff)
			}
		})
	}

	t.Run("bad path", func(t *testing.T) {
		var _, err = ParseBatch(fs, "nonexistent-file.xml", nil)
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})

	t.Run("bad xml", func(t *testing.T) {
		afero.WriteFile(fs, pth, []byte("<xml>"), 0644)
		var _, err = ParseBatch(fs, pth, nil)
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})

	t.Run("no issues", func(t *testing.T) {
		var noIssuesXML = `<ndnp:batch xmlns:ndnp="http://www.loc.gov/ndnp"></ndnp:batch>`
		afero.WriteFile(fs, pth, []byte(noIssuesXML), 0644)
		var _, err = ParseBatch(fs, pth, nil)
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})
}

func TestWriteBatchXML(t *testing.T) {
	var batchxml = "testdata/batch.xml"
	var osfs = afero.NewOsFs()
	var originalData, err = afero.ReadFile(osfs, batchxml)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", batchxml, err)
	}
	var b *BatchXML
	b, err = ParseBatch(osfs, batchxml, nil)
	if err != nil {
		t.Fatalf("Unable to parse test batch.xml for write test: %s", err)
	}

	var fs = afero.NewMemMapFs()
	var outPath = "/new-batch.xml"
	err = b.WriteBatchXML(fs, outPath)
	if err != nil {
		t.Fatalf("unexpected error writing batch XML: %s", err)
	}

	var writtenData, readErr = afero.ReadFile(fs, outPath)
	if readErr != nil {
		t.Fatalf("unexpected error reading new batch XML: %s", readErr)
	}

	// Kill all indentations: the source file uses spaces while the output uses
	// tabs. I probably had a good reason for one of those choices.
	var re = regexp.MustCompile(`\n\s*`)
	var expected = re.ReplaceAllString(string(originalData), "")
	var got = re.ReplaceAllString(string(writtenData), "")
	var diff = cmp.Diff(expected, got)
	if diff != "" {
		t.Error(diff)
	}
}
