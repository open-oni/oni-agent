package batchfix

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/open-oni/oni-agent/internal/file"
	"github.com/spf13/afero"
)

const pathSeparator = string(os.PathSeparator)

// Fixer is not a lawyer who will clean up your legal messes for you. It is a
// batch fixer: it copies a batch from one location to another, doing some kind
// of fix to the files copied.
//
// Currently Fixer's only fix is removing issues, which means not copying any
// files that match a list of directories to skip.
type Fixer struct {
	fs       afero.Fs
	src      string
	dst      string
	skipDirs []string
	batch    *BatchXML
}

// NewFixer returns a new Fixer instance that will copy (and fix) files on the
// give filesystem from source to destination.
//
// TODO: we need a [context.Context] passed in so this can be interrupted. TODO
// 2: need to have a post-failure cleanup somehow. Maybe that's the caller's
// responsibility?
func NewFixer(filesystem afero.Fs, source, destination string) (f *Fixer, err error) {
	f = &Fixer{
		fs:  filesystem,
		src: source,
		dst: destination,
	}
	f.src, err = filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path from source %q: %w", source, err)
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	var info os.FileInfo
	info, err = filesystem.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("invalid source (%q): %s", source, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("invalid source (%q): not a directory", source)
	}

	_, err = filesystem.Stat(destination)
	if err == nil || !os.IsNotExist(err) {
		return nil, fmt.Errorf("invalid destination (%q): already exists", destination)
	}

	return f, nil
}

func (f *Fixer) readSourceBatch(skipKeys []string) error {
	var err error
	var srcBatchPath = filepath.Join(f.src, "data", "batch.xml")
	f.batch, err = ParseBatch(f.fs, srcBatchPath, skipKeys)
	if err != nil {
		return fmt.Errorf("parsing source batch: %w", err)
	}

	// Gather dropped issues to figure out the dirs we'll skip
	for _, i := range f.batch.Issues {
		if i.Skip {
			var dir, _ = filepath.Split(i.Path)
			f.skipDirs = append(f.skipDirs, dir)
		}
	}

	return nil
}

// RemoveIssues starts the file system traversal, copying all files except the
// source batch.xml (it's modified on write to remove the given issue keys),
// validated XMLs (the various *_1.xml files), TIFFs, and anything in a skipped
// issue's location.
func (f *Fixer) RemoveIssues(keys []string) error {
	// Read the source batch file to set up our skip dirs
	var err = f.readSourceBatch(keys)
	if err != nil {
		return err
	}

	// We use a base-path FS for the walk so we don't have to strip off the path
	// for building source/dest mappings
	err = afero.Walk(afero.NewBasePathFs(f.fs, f.src), "/", f.walkFunc)
	if err != nil {
		return err
	}

	var dstBatchPath = filepath.Join(f.dst, "data", "batch.xml")
	err = f.batch.WriteBatchXML(f.fs, dstBatchPath)
	if err != nil {
		return fmt.Errorf("writing destination batch: %w", err)
	}

	return nil
}

func (f *Fixer) walkFunc(pth string, info os.FileInfo, err error) error {
	// Stop on any error
	if err != nil {
		return err
	}

	// We don't do anything with directories
	if info.IsDir() {
		return nil
	}

	// We need dir and file split up to do the batch.xml check, skip-dirs checks,
	// and then create destination structure
	var dir, filename = path.Split(pth)
	filename = strings.ToLower(filename)

	// Skip batch.xml: it needs to be rewritten, not just copied, and XML
	// rewrites aren't in the Fixer's job description
	if filename == "batch.xml" {
		return nil
	}

	// Skip all validated XML files
	var l = len(filename)
	if l > 6 && filename[l-6:] == "_1.xml" {
		return nil
	}

	// Skip all tiff files
	var ext = filepath.Ext(filename)
	if ext == ".tif" || ext == ".tiff" {
		return nil
	}

	// Check if the directory is in the list of skipDirs
	for _, skipDir := range f.skipDirs {
		if strings.Contains(dir, skipDir) {
			return nil
		}
	}

	// Create the destination directory if it doesn't exist
	err = f.fs.MkdirAll(filepath.Join(f.dst, dir), 0755)
	if err != nil {
		return fmt.Errorf("creating dir %q in destination filesystem: %w", dir, err)
	}

	var src = filepath.Join(f.src, pth)
	var dst = filepath.Join(f.dst, pth)
	err = file.Copy(f.fs, src, dst, 5)
	if err != nil {
		return err
	}

	return nil
}
