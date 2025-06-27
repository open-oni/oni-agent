package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func readfile(t *testing.T, pth string) []byte {
	t.Helper()

	var data, err = os.ReadFile(pth)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", pth, err)
	}
	return data
}

func extractDummyFiles(t *testing.T, src, dst string) {
	t.Helper()

	var manifest = filepath.Join(src, "manifest.txt")
	var data = readfile(t, manifest)

	// For each line, create an empty file in dst/data/...
	var lines = strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var dummyfile = filepath.Join(dst, "data", line)
		var parent = filepath.Dir(dummyfile)
		var err = os.MkdirAll(parent, 0755)
		if err != nil {
			t.Fatalf("Can't create parent dir %q (for file %q): %s", dst, dummyfile, err)
		}
		err = os.WriteFile(dummyfile, []byte{}, 0644)
		if err != nil {
			t.Fatalf("Error writing dummy file %q: %s", dummyfile, err)
		}
	}
}

func copyBatchXML(t *testing.T, src, dst string) {
	t.Helper()

	var batchXML = filepath.Join(src, "batch.xml")
	var data, err = os.ReadFile(batchXML)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", batchXML, err)
	}

	var batchout = filepath.Join(dst, "batch.xml")
	err = os.WriteFile(batchout, data, 0644)
	if err != nil {
		t.Fatalf("Error writing %q: %s", batchout, err)
	}
}

func TestRemoveIssuesCommand(t *testing.T) {
	// TODO: Move the fake batch creation to a setup / init function so we can
	// run other tests. Add a test for an issue key that's not in the batch. Add
	// a "teardown" function that removes the dummy batch source dir.

	// Create temporary dirs
	var srcdir, err = os.MkdirTemp("", "remove-issues-src-")
	if err != nil {
		t.Fatalf("Failed to create temp src dir: %v", err)
	}
	defer os.RemoveAll(srcdir)

	var dstdir string
	dstdir, err = os.MkdirTemp("", "remove-issues-dest-")
	if err != nil {
		t.Fatalf("Failed to create temp dest dir: %v", err)
	}
	defer os.RemoveAll(dstdir)

	// Create test data in source dir based on our manifest and the batch.xml
	// file: all files are just empty dummy files except the batch.xml
	var basedir = filepath.Join("testdata", "batch_oru_bravo_ver01")
	extractDummyFiles(t, basedir, srcdir)
	copyBatchXML(t, basedir, filepath.Join(srcdir, "data"))

	// Remove some issues. run() requires the dest dir not already exist, so we
	// pre-remove it here.
	os.Remove(dstdir)
	err = run("remove-issues (test)", srcdir, dstdir, "sn84022657/1871-02-11_01", "sn84022658/1856-08-16_01", "1856-08-16/1854-01-13_01")
	if err != nil {
		t.Fatalf("Error running remove-issues: %s", err)
	}

	// Verify batch.xml exists and has the right contents
	var dstBatchXML = filepath.Join(dstdir, "data", "batch.xml")
	assertFileExists(t, dstBatchXML, true)

	var data []byte
	data, err = os.ReadFile(dstBatchXML)
	if err != nil {
		t.Fatalf("Unable to read %q: %s", dstBatchXML, err)
	}

	var b batchXML
	err = xml.Unmarshal(data, &b)
	if err != nil {
		t.Fatalf("Error unmarshaling %q: %s", dstBatchXML, err)
	}

	// TODO: Check deserialized issue list with our expected list
	// TODO: Make sure all files in manifest are copied except tiffs and removed issues' files
}
