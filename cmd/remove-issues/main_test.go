package main

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func findAll(dir fs.FS) (paths []string, err error) {
	err = fs.WalkDir(dir, ".", func(pth string, _ fs.DirEntry, err error) error {
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

// TestBatchDir holds the path to the test source directory - we build a
// source from our bravo manifest which all tests can then use
var TestBatchDir string

// TestBatch lets us read what our test batch actually has
var TestBatch *batchXML

func assertFileExists(t *testing.T, pth string, expected bool) {
	t.Helper()

	var _, err = os.Stat(pth)
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

// extractBatchFiles creates empty files in the destination based on the
// source's manifest file. `src` should point to a test batch location where
// `manifest.txt` and `batch.xml` files live. For each line in the manifest, an
// empty file will be created in `dst/data`. The XML file is copied to
// `dst/data/batch.xml`.
func extractBatchFiles(src, dst string) error {
	var manifestPath = filepath.Join(src, "manifest.txt")
	var data, err = os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	// For each line, create an empty file in dst/data/...
	var lines = strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var fname = filepath.Join(dst, "data", line)
		var parent = filepath.Dir(fname)
		var err = os.MkdirAll(parent, 0755)
		if err != nil {
			return fmt.Errorf("creating parent dir %q (for file %q): %w", parent, fname, err)
		}
		err = os.WriteFile(fname, []byte{}, 0644)
		if err != nil {
			return fmt.Errorf("writing empty file %q: %w", fname, err)
		}
	}

	var batchXML = filepath.Join(src, "batch.xml")
	data, err = os.ReadFile(batchXML)
	if err != nil {
		return fmt.Errorf("reading %q: %w", batchXML, err)
	}

	var batchout = filepath.Join(dst, "data", "batch.xml")
	err = os.WriteFile(batchout, data, 0644)
	if err != nil {
		return fmt.Errorf("writing %q: %w", batchout, err)
	}

	return nil
}

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

	TestBatchDir, err = os.MkdirTemp("", "remove-issues-src-")
	if err != nil {
		slog.Error("Cannot create temp dir for dummy batch source", "error", err)
		os.Exit(-1)
	}

	err = extractBatchFiles(basedir, TestBatchDir)
	if err != nil {
		slog.Error("Error extracting batch files", "basedir", basedir, "TestBatchDir", TestBatchDir, "error", err.Error())
		os.RemoveAll(TestBatchDir)
		os.Exit(-1)
	}

	var code = m.Run()
	os.RemoveAll(TestBatchDir)
	os.Exit(code)
}

// doRemove sets up a tempdir for the batch copy, then calls run() on the list
// of keys given, returning the created destination directory and any error
// from the run command.
//
// It is the caller's responsibility to remove the destination directory.
func doRemove(t *testing.T, keys ...string) (dstdir string, err error) {
	// run requires the destination not exist, so we only create it to make sure
	// it's a usable tempdir, then remove it immediately.
	dstdir, err = os.MkdirTemp("", "remove-issues-dest-")
	if err != nil {
		t.Fatalf("Failed to create temp dest dir: %v", err)
	}
	os.Remove(dstdir)

	var args = append([]string{"remove-issues (test)", TestBatchDir, dstdir}, keys...)
	return dstdir, run(args...)
}

func TestRemoveIssuesCommand(t *testing.T) {
	var removeIssues = []*issueXML{
		{LCCN: "sn84022658", IssueDate: "1856-01-05"},
		{LCCN: "sn84022658", IssueDate: "1856-11-29"},
		{LCCN: "sn98068707", IssueDate: "1855-10-03"},
	}
	var removeKeys = make([]string, len(removeIssues))
	for i, issue := range removeIssues {
		removeKeys[i] = issue.String() + "_01"
	}
	var dstdir, err = doRemove(t, removeKeys...)
	defer os.RemoveAll(dstdir)
	if err != nil {
		t.Fatalf("Valid call should not have an error, but got %q", err)
	}

	// Verify batch.xml exists and has the right contents
	var dstBatchXML = filepath.Join(dstdir, "data", "batch.xml")
	assertFileExists(t, dstBatchXML, true)

	var data []byte
	data, err = os.ReadFile(dstBatchXML)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", dstBatchXML, err)
	}

	var fixedBatch batchXML
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
	srcList, err = findAll(os.DirFS(TestBatchDir))
	if err != nil {
		t.Fatalf("Unable to read test batch dir: %s", err)
	}
	dstList, err = findAll(os.DirFS(dstdir))
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
		if len(parts) >= 4 {
			var lccn, dted = parts[1], parts[3]
			var key = lccn + "/" + dted
			if removedKey[key] {
				continue
			}
		}

		expectedList = append(expectedList, pth)
	}

	var diff = cmp.Diff(expectedList, dstList)
	if diff != "" {
		t.Fatalf("Expected lists to match: %s", diff)
	}
}

func TestRemoveIssues_InvalidIssueKeys(t *testing.T) {
	var dstdir, err = doRemove(t, "fakeyfake/1920-01-01_01")
	defer os.RemoveAll(dstdir)

	if err == nil {
		t.Fatalf("Nonexistent issue keys should have failed in run(), but didn't")
	}
	if !strings.Contains(err.Error(), `issuekey "fakeyfake/1920010101" not in batch`) {
		t.Fatalf(`Nonexistent issue key error should include the text "foo"; got %q`, err)
	}
}
