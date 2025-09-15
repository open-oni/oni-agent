package batchfix

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
)

const (
	testSrcDir = "/src"
	testDstDir = "/dst"
)

var testIssues = []struct {
	LCCN        string
	IssueDate   string
	Edition     string
	RelativeDir string
	Files       []string
}{
	{
		LCCN:        "sn12345678",
		IssueDate:   "1900-01-01",
		Edition:     "01",
		RelativeDir: "sn12345678/print/1900010101",
		Files:       []string{"001.jp2", "001.xml", "001.pdf", "002.jp2", "002.pdf", "002.xml"},
	},
	{
		LCCN:        "sn12345678",
		IssueDate:   "1900-01-02",
		Edition:     "01",
		RelativeDir: "sn12345678/print/1900010201",
		Files:       []string{"001.jp2", "001.xml", "001.pdf", "001.tiff", "002.jp2", "002.pdf", "002.xml", "002.tif"},
	},
	{
		LCCN:        "sn98765432",
		IssueDate:   "1950-05-05",
		Edition:     "01",
		RelativeDir: "sn98765432/print/1950050501",
		Files:       []string{"001.jp2", "001.xml"},
	},
}

// createTestBatch creates a dummy batch structure on the given filesystem
func createTestBatch(fs afero.Fs, dir string) error {
	var err = fs.MkdirAll(filepath.Join(dir, "data"), 0755)
	if err != nil {
		return fmt.Errorf("creating test batch data dir: %w", err)
	}

	for _, issue := range testIssues {
		var issuePath = filepath.Join(dir, "data", issue.RelativeDir)
		err = fs.MkdirAll(issuePath, 0755)
		if err != nil {
			return fmt.Errorf("creating issue dir %q: %w", issuePath, err)
		}
		for _, fname := range issue.Files {
			var filePath = filepath.Join(issuePath, fname)
			err = afero.WriteFile(fs, filePath, []byte(fmt.Sprintf("Hello, my name is: %s", fname)), 0644)
			if err != nil {
				return fmt.Errorf("writing issue %q file %q: %w", issue.RelativeDir, fname, err)
			}
		}
	}

	var batchXMLContent = `
<ndnp:batch xmlns:ndnp="http://www.loc.gov/ndnp" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://www.loc.gov/ndnp" name="testbatch" awardee="test" awardYear="2023">
	<issue lccn="sn12345678" issueDate="1900-01-01" editionOrder="01">sn12345678/print/1900010101/1900010101.xml</issue>
	<issue lccn="sn12345678" issueDate="1900-01-02" editionOrder="01">sn12345678/print/1900010201/1900010201.xml</issue>
	<issue lccn="sn98765432" issueDate="1950-05-05" editionOrder="01">sn98765432/print/1950050501/1950050501.xml</issue>
</ndnp:batch>
`
	var batchXMLPath = filepath.Join(dir, "data", "batch.xml")
	err = afero.WriteFile(fs, batchXMLPath, []byte(batchXMLContent), 0644)
	if err != nil {
		return fmt.Errorf("writing test batch XML manifest: %w", err)
	}

	return nil
}

// findAll returns a sorted list of all file paths relative to the given directory
func findAll(fs afero.Fs, dir string) (paths []string, err error) {
	var sub = afero.NewBasePathFs(fs, dir)
	err = afero.Walk(sub, "/", func(pth string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, pth)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking filesystem: %w", err)
	}

	sort.Strings(paths)
	return paths, nil
}

func TestNewFixer(t *testing.T) {
	var tests = map[string]struct {
		src     string
		dst     string
		setup   func(fs afero.Fs, src, dst string) error
		wantErr bool
	}{
		"Valid": {
			src: testSrcDir,
			dst: testDstDir,
			setup: func(fs afero.Fs, src, dst string) error {
				return fs.MkdirAll(src, 0755)
			},
			wantErr: false,
		},
		"Source does not exist": {
			src:     "/nonexistent",
			dst:     testDstDir,
			wantErr: true,
		},
		"Source is a file": {
			src: testSrcDir,
			dst: testDstDir,
			setup: func(fs afero.Fs, src, dst string) error {
				_, err := fs.Create(src)
				return err
			},
			wantErr: true,
		},
		"Destination exists": {
			src: testSrcDir,
			dst: testDstDir,
			setup: func(fs afero.Fs, src, dst string) error {
				if err := fs.MkdirAll(src, 0755); err != nil {
					return err
				}
				// Doesn't matter if it's a file or a dir, just that it exists
				_, err := fs.Create(dst)
				return err
			},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var fs = afero.NewMemMapFs()
			if tt.setup != nil {
				var err = tt.setup(fs, tt.src, tt.dst)
				if err != nil {
					t.Fatalf("setup failed: %s", err)
				}
			}

			fixer, err := NewFixer(fs, tt.src, tt.dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFixer() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && fixer == nil {
				t.Errorf("Expected a non-nil fixer on success")
			}
		})
	}
}

func TestFixer_readSourceBatch(t *testing.T) {
	var tests = map[string]struct {
		skipKeys         []string
		wantErr          bool
		expectedSkipDirs []string
	}{
		"No skip keys": {
			skipKeys:         []string{},
			wantErr:          false,
			expectedSkipDirs: nil,
		},
		"One valid skip key": {
			skipKeys:         []string{"sn12345678/1900-01-01_01"},
			wantErr:          false,
			expectedSkipDirs: []string{"sn12345678/print/1900010101/"},
		},
		"Multiple valid skip keys": {
			skipKeys:         []string{"sn12345678/1900-01-01_01", "sn98765432/1950-05-05_01"},
			wantErr:          false,
			expectedSkipDirs: []string{"sn12345678/print/1900010101/", "sn98765432/print/1950050501/"},
		},
		"Invalid skip key": {
			skipKeys:         []string{"fakeyfake/1920-01-01_01"},
			wantErr:          true,
			expectedSkipDirs: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var fs = afero.NewMemMapFs()
			err := createTestBatch(fs, testSrcDir)
			if err != nil {
				t.Fatalf("Failed to create test batch: %v", err)
			}
			fixer, err := NewFixer(fs, testSrcDir, testDstDir)
			if err != nil {
				t.Fatalf("Error calling NewFixer: %s", err)
			}
			err = fixer.readSourceBatch(tt.skipKeys)

			// Verify error state. If we expect an error, we also don't test further.
			if tt.wantErr {
				if err == nil {
					t.Errorf("readSourceBatch should have an error (got nil)")
				}
				return
			}
			if err != nil {
				t.Errorf("readSourceBatch should not have an error (got %s)", err)
			}

			// Basic batch checks
			if fixer.batch == nil {
				t.Fatalf("Expected batch to be parsed, got nil")
			}
			var expectedIssues = len(testIssues)
			if len(fixer.batch.Issues) != expectedIssues {
				t.Errorf("Expected %d issues in batch, got %d", expectedIssues, len(fixer.batch.Issues))
			}

			// Make sure skipdirs match up
			sort.Strings(fixer.skipDirs)
			sort.Strings(tt.expectedSkipDirs)
			var diff = cmp.Diff(tt.expectedSkipDirs, fixer.skipDirs)
			if diff != "" {
				t.Errorf("skipdirs mismatch:\n%s", diff)
			}

			// Verify skipped issues are flagged as such
			for _, issue := range fixer.batch.Issues {
				var issueKey = issue.String() + issue.Edition
				var shouldSkip = false
				for _, sk := range tt.skipKeys {
					if keyfix(sk) == keyfix(issueKey) {
						shouldSkip = true
						break
					}
				}
				if issue.Skip != shouldSkip {
					t.Errorf("Issue %q: expected Skip=%t, got %t", issueKey, shouldSkip, issue.Skip)
				}
			}
		})
	}
}

func TestFixer_RemoveIssues(t *testing.T) {
	var tests = map[string]struct {
		skipKeys            []string
		wantErr             bool
		expectedDstFiles    []string
		expectedBatchIssues int
	}{
		"Remove one issue": {
			skipKeys: []string{"sn12345678/1900-01-01_01"},
			wantErr:  false,
			expectedDstFiles: []string{
				"/data/batch.xml",
				"/data/sn12345678/print/1900010201/001.jp2",
				"/data/sn12345678/print/1900010201/001.xml",
				"/data/sn12345678/print/1900010201/001.pdf",
				"/data/sn12345678/print/1900010201/002.jp2",
				"/data/sn12345678/print/1900010201/002.pdf",
				"/data/sn12345678/print/1900010201/002.xml",
				"/data/sn98765432/print/1950050501/001.jp2",
				"/data/sn98765432/print/1950050501/001.xml",
			},
			expectedBatchIssues: len(testIssues) - 1,
		},
		"Remove multiple issues": {
			skipKeys: []string{"sn12345678/1900-01-01_01", "sn98765432/1950-05-05_01"},
			wantErr:  false,
			expectedDstFiles: []string{
				"/data/batch.xml",
				"/data/sn12345678/print/1900010201/001.jp2",
				"/data/sn12345678/print/1900010201/001.xml",
				"/data/sn12345678/print/1900010201/001.pdf",
				"/data/sn12345678/print/1900010201/002.jp2",
				"/data/sn12345678/print/1900010201/002.pdf",
				"/data/sn12345678/print/1900010201/002.xml",
			},
			expectedBatchIssues: len(testIssues) - 2,
		},
		"Remove no issues": {
			skipKeys: []string{},
			wantErr:  false,
			expectedDstFiles: []string{
				"/data/batch.xml",
				"/data/sn12345678/print/1900010101/001.jp2",
				"/data/sn12345678/print/1900010101/001.xml",
				"/data/sn12345678/print/1900010101/001.pdf",
				"/data/sn12345678/print/1900010101/002.jp2",
				"/data/sn12345678/print/1900010101/002.pdf",
				"/data/sn12345678/print/1900010101/002.xml",
				"/data/sn12345678/print/1900010201/001.jp2",
				"/data/sn12345678/print/1900010201/001.xml",
				"/data/sn12345678/print/1900010201/001.pdf",
				"/data/sn12345678/print/1900010201/002.jp2",
				"/data/sn12345678/print/1900010201/002.pdf",
				"/data/sn12345678/print/1900010201/002.xml",
				"/data/sn98765432/print/1950050501/001.jp2",
				"/data/sn98765432/print/1950050501/001.xml",
			},
			expectedBatchIssues: len(testIssues),
		},
		"Remove all issues": {
			skipKeys: []string{"sn12345678/1900-01-01_01", "sn12345678/1900-01-02_01", "sn98765432/1950-05-05_01"},
			wantErr:  false,
			expectedDstFiles: []string{
				"/data/batch.xml",
			},
			expectedBatchIssues: 0,
		},
		"Invalid issue key": {
			skipKeys:            []string{"fakeyfake/1920-01-01_01"},
			wantErr:             true,
			expectedDstFiles:    []string{},
			expectedBatchIssues: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var fs = afero.NewMemMapFs()
			var err = createTestBatch(fs, testSrcDir)
			if err != nil {
				t.Fatalf("Failed to create test batch: %s", err)
			}

			fixer, err := NewFixer(fs, testSrcDir, testDstDir)
			if err != nil {
				t.Fatalf("Error calling NewFixer: %s", err)
			}
			err = fixer.RemoveIssues(tt.skipKeys)

			var dstFiles []string
			if err == nil {
				dstFiles, err = findAll(fs, testDstDir)
				if err != nil {
					t.Fatalf("Error calling findAll: %s", err)
				}
			}
			sort.Strings(dstFiles)

			// If we expect an error, we return after some checks, as further tests
			// won't make sense
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected RemoveIssues to have an error, got nil")
					return
				}

				// Is destination empty?
				if len(dstFiles) > 0 {
					t.Errorf("Expected no files in destination on error, but found: %v", dstFiles)
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected RemoveIssues to succeed, got error %s", err)
			}

			sort.Strings(tt.expectedDstFiles)
			var diff = cmp.Diff(tt.expectedDstFiles, dstFiles)
			if diff != "" {
				t.Errorf("Destination files mismatch:\n%s", diff)
			}

			// Verify batch.xml content in destination
			var dstBatchXMLPath = filepath.Join(testDstDir, "data", "batch.xml")
			var data []byte
			data, err = afero.ReadFile(fs, dstBatchXMLPath)
			if err != nil {
				t.Fatalf("Failed to read destination batch.xml: %v", err)
			}

			var fixedBatch BatchXML
			err = xml.Unmarshal(data, &fixedBatch)
			if err != nil {
				t.Fatalf("Failed to unmarshal destination batch.xml: %v", err)
			}

			if len(fixedBatch.Issues) != tt.expectedBatchIssues {
				t.Errorf("Expected %d issues in destination batch.xml, got %d", tt.expectedBatchIssues, len(fixedBatch.Issues))
			}

			// Verify that removed issues are indeed gone from the XML
			for _, removedKey := range tt.skipKeys {
				var found = false
				for _, issue := range fixedBatch.Issues {
					if keyfix(issue.String()+issue.Edition) == keyfix(removedKey) {
						found = true
						break
					}
				}
				if found {
					t.Errorf("Issue %q was expected to be removed but found in destination batch.xml", removedKey)
				}
			}
		})
	}
}
