package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const pathSeparator = string(os.PathSeparator)

// Walker is not a Texas ranger, just a structure for walking files with the
// context we need to know how to start jobs and translate a source file to
// where we'll want it to end up
type Walker struct {
	conf  *config
}

// NewWalker returns a new Walker instance with the given context. Why did I
// name it that way? TODO: need to pass in an actual [context.Context] so this
// can be an interruptable process. TODO 2: need to have a post-failure cleanup
// somehow. Maybe that's the caller's responsibility?
func NewWalker(conf *config) *Walker {
	return &Walker{conf}
}

// Walk starts the file system traversal and copies all files except batch.xml
// (which needs to be rewritten outside the file copy), validated XMLs (the
// various *_1.xml files), and TIFFs.
func (w *Walker) Walk() error {
	return filepath.Walk(w.conf.SourceDir, w.walkFunc)
}

func (w *Walker) walkFunc(path string, info os.FileInfo, err error) error {
	// Stop on any error
	if err != nil {
		return err
	}

	// We don't do anything with directories
	if info.IsDir() {
		return nil
	}

	// Gather info
	var parts = strings.Split(path, pathSeparator)
	var baseName = parts[len(parts)-1]
	var localDir = strings.Replace(strings.Replace(path, w.conf.SourceDir, "", 1), baseName, "", 1)
	var destPath = filepath.Join(w.conf.DestDir, localDir)

	// Check if the directory is in the list of skipDirs
	for _, skipDir := range w.conf.SkipDirs {
		if strings.Contains(path, skipDir) {
			log.Printf("INFO: skipping file %q: matches skipDir %q", path, skipDir)
			return nil
		}
	}

	// Skip batch.xml: it needs to be rewritten, not just copied, and XML
	// rewrites aren't in Walker's job description
	if baseName == "batch.xml" {
		log.Printf("INFO: skipping file %q: batch.XML", path)
		return nil
	}

	// Create the destination directory if it doesn't exist
	err = os.MkdirAll(destPath, 0755)
	if err != nil {
		log.Printf("ERROR: could not create %q: %s", destPath, err)
		return err
	}

	var ext = strings.ToLower(filepath.Ext(baseName)[1:])
	var destFile = filepath.Join(destPath, baseName)

	switch ext {
	case "xml":
		if baseName[len(baseName)-6:] == "_1.xml" {
			log.Printf("INFO: skipping file %q: validated XML", path)
			return nil
		}
	case "tif", "tiff":
		log.Printf("INFO: skipping file %q: TIFF", path)
		return nil
	}

	log.Printf("INFO: copying file %q to %q", path, destFile)
	err = copyWithRetry(path, destFile, 5)
	if err != nil {
		return fmt.Errorf("copying %q to %q: %w", path, destFile, err)
	}

	return nil
}
