package main

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/open-oni/oni-agent/internal/batchfix"
	"github.com/spf13/afero"
)

func findAll(fs afero.Fs, dir string) (paths []string, err error) {
	var sub = afero.NewBasePathFs(fs, dir)
	err = afero.Walk(sub, "/", func(pth string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		paths = append(paths, pth)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking filesystem: %w", err)
	}

	sort.Strings(paths)
	return paths, nil
}

func assertFileExists(t *testing.T, fs afero.Fs, pth string, expected bool) {
	t.Helper()

	var _, err = fs.Stat(pth)
	if expected {
		if os.IsNotExist(err) {
			t.Errorf("Expected %q to exist", pth)
		} else if err != nil {
			t.Errorf("Error checking file existence for %q: %s", pth, err)
		}
	} else {
		if !os.IsNotExist(err) {
			t.Errorf("Expected %q not to exist", pth)
		}
	}
}

// extractBatchFiles creates a batch structure filled with empty files on our
// in-memory filesystem at `/batch`. `testdir` needs to point to a physical dir
// containing `manifest.txt` and `batch.xml`. For each line in the manifest, an
// empty file will be created on the in-memory filesystem.
func extractBatchFiles(fs afero.Fs, testdir string) error {
	var manifestPath = filepath.Join(testdir, "manifest.txt")
	var data, err = os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	// For each manifest line, create an empty file on the memory FS
	var lines = strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var fname = filepath.Join("/batch", "data", line)
		var parent = filepath.Dir(fname)
		var err = fs.MkdirAll(parent, 0755)
		if err != nil {
			return fmt.Errorf("creating parent dir %q (for file %q): %w", parent, fname, err)
		}
		err = afero.WriteFile(fs, fname, []byte{}, 0644)
		if err != nil {
			return fmt.Errorf("writing empty file %q: %w", fname, err)
		}
	}

	var batchXML = filepath.Join(testdir, "batch.xml")
	data, err = os.ReadFile(batchXML)
	if err != nil {
		return fmt.Errorf("reading %q: %w", batchXML, err)
	}

	var batchout = filepath.Join("/batch", "data", "batch.xml")
	err = afero.WriteFile(fs, batchout, data, 0644)
	if err != nil {
		return fmt.Errorf("writing %q: %w", batchout, err)
	}

	return nil
}

// TestBatch lets us store the test batch XML data for testing
var TestBatch *batchfix.BatchXML

// TestFS is our read-only filesystem containing the source batch
var TestFS afero.Fs

// TestMain does our pre-test setup and post-test teardown
func TestMain(m *testing.M) {
	// Read the original batch XML so we can base tests on what we're using in
	// case we change it
	var basedir = filepath.Join("testdata", "batch_oru_bravo_ver01")
	var data, err = os.ReadFile(filepath.Join(basedir, "batch.xml"))
	if err == nil {
		err = xml.Unmarshal(data, &TestBatch)
	}
	if err != nil {
		slog.Error("Unable to prep test batch", "error", err.Error())
		os.Exit(-1)
	}

	// While we're at it, may as well make sure the test batch has a good number
	// of issues
	if len(TestBatch.Issues) < 10 {
		slog.Error("Unable to prep test batch", "error", "batch.xml has suspiciously fewer issues than expected")
		os.Exit(-1)
	}

	// Create a writeable FS, extract batch files to it, then wrap it in a
	// read-only FS so other tests can't accidentally wreck it
	var writeFS = afero.NewMemMapFs()
	err = extractBatchFiles(writeFS, basedir)
	if err != nil {
		slog.Error("Error extracting batch files", "basedir", basedir, "error", err.Error())
		os.Exit(-1)
	}
	TestFS = afero.NewReadOnlyFs(writeFS)

	var code = m.Run()
	os.Exit(code)
}

// doRemove runs the remove operation with the given list of issue keys,
// returning a temp filesystem (layered on top of tfs so we can reuse tfs
// without re-extracting files) and any errors from the run operation.
//
// The corrected batch is written out to "/fixed" on the returned FS.
func doRemove(keys []string) (afero.Fs, error) {
	var rwfs = afero.NewCopyOnWriteFs(TestFS, afero.NewMemMapFs())
	var args = append([]string{"remove-issues (test)", "/batch", "/fixed"}, keys...)
	var err = run(rwfs, args...)

	return rwfs, err
}

func TestRemoveIssuesCommand(t *testing.T) {
	var removeIssues = []*batchfix.IssueXML{
		{LCCN: "sn84022658", IssueDate: "1856-01-05"},
		{LCCN: "sn84022658", IssueDate: "1856-11-29"},
		{LCCN: "sn98068707", IssueDate: "1855-10-03"},
	}
	var removeKeys = make([]string, len(removeIssues))
	for i, issue := range removeIssues {
		removeKeys[i] = issue.String() + "_01"
	}

	var fs, err = doRemove(removeKeys)
	if err != nil {
		t.Fatalf("Valid call should not have an error, but got %q", err)
	}

	// Verify batch.xml exists and has the right contents
	var dstBatchXML = filepath.Join("/fixed", "data", "batch.xml")
	assertFileExists(t, fs, dstBatchXML, true)

	var data []byte
	data, err = afero.ReadFile(fs, dstBatchXML)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", dstBatchXML, err)
	}

	var fixedBatch batchfix.BatchXML
	err = xml.Unmarshal(data, &fixedBatch)
	if err != nil {
		t.Fatalf("Error unmarshaling %q: %s", dstBatchXML, err)
	}

	// Check deserialized issue list with our expected list
	var expected = len(TestBatch.Issues) - len(removeIssues)
	var got = len(fixedBatch.Issues)
	t.Logf("Original batch had %d issues; new batch has %d", len(TestBatch.Issues), got)
	if expected != got {
		t.Errorf("Expected %d issues, but got %d", expected, got)
	}
	var has = make(map[string]bool)
	for _, i := range fixedBatch.Issues {
		has[i.String()] = true
	}
	for _, i := range removeIssues {
		if has[i.String()] {
			t.Errorf("Expected %s to be removed, but found it in the fixed batch", i)
		}
	}

	// Get source and destination file lists so we can compare them
	var srcList, dstList []string
	srcList, err = findAll(fs, "/batch")
	if err != nil {
		t.Fatalf("Unable to read test batch dir: %s", err)
	}
	dstList, err = findAll(fs, "/fixed")
	if err != nil {
		t.Fatalf("Unable to read fixed batch dir: %s", err)
	}

	// Generate our expected output: remove all tiffs, validated XMLs, and
	// removed issues' files from source
	var expectedList []string
	for _, pth := range srcList {
		if filepath.Ext(pth) == ".tif" {
			continue
		}
		if strings.HasSuffix(pth, "_1.xml") {
			continue
		}

		var removedKey = make(map[string]bool)
		for _, key := range removeKeys {
			key = strings.Replace(key, "-", "", -1)
			key = strings.Replace(key, "_", "", -1)
			removedKey[key] = true
		}
		var parts = strings.Split(pth, string(os.PathSeparator))
		if len(parts) >= 5 {
			var lccn, dted = parts[2], parts[4]
			var key = lccn + "/" + dted
			if removedKey[key] {
				continue
			}
		}

		expectedList = append(expectedList, pth)
	}

	var diff = cmp.Diff(expectedList, dstList)
	if diff != "" {
		t.Logf("Source list first 10 elements: %s", strings.Join(srcList[:10], ","))
		t.Fatalf("Expected lists to match: %s", diff)
	}
}

func TestRemoveIssues_InvalidIssueKeys(t *testing.T) {
	var _, err = doRemove([]string{"fakeyfake/1920-01-01_01"})
	if err == nil {
		t.Fatalf("Nonexistent issue keys should have failed in run(), but didn't")
	}
	if !strings.Contains(err.Error(), `issuekey "fakeyfake/1920010101" not in batch`) {
		t.Fatalf(`Nonexistent issue key error should include the text "foo"; got %q`, err)
	}
}
