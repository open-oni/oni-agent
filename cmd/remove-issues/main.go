package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

func usageError(msg string, args ...interface{}) {
	var fmsg = fmt.Sprintf(msg, args...)
	fmt.Printf("\033[31;1mERROR: %s\033[0m\n", fmsg)
	fmt.Printf(`
Usage: %s <source directory> <destination directory> <issue key>...

The source directory should either be the pristine dark archive, or a copy
thereof (though the TIFF files won't matter, as they aren't copied to the
destination).  Once complete, the destination will contain an ONI-ingestable
batch.

One or more issue keys must be present.  If any key is given but isn't in the
source batch, this tool will report it and exit without processing any other
keys, even if they're valid.
`, os.Args[0])
	os.Exit(1)
}

// FixContext is just the app's directory/lccn context so we don't have global
// variables puked out everywhere but we also don't pass a million args around
// to everything
type FixContext struct {
	SourceDir string
	DestDir   string
	IssueKeys []string
	SkipDirs  []string
}

// getArgs does some sanity-checking and sets the source/dest args
func getArgs() *FixContext {
	if len(os.Args) < 4 {
		usageError("Missing one or more arguments")
	}

	var src = os.Args[1]
	var dst = os.Args[2]
	var fc = &FixContext{
		SourceDir: src,
		DestDir:   dst,
		IssueKeys: os.Args[3:],
	}
	var err error
	fc.SourceDir, err = filepath.Abs(fc.SourceDir)
	if err != nil {
		usageError("Source (%s) is invalid: %s", src, err)
	}
	fc.DestDir, err = filepath.Abs(fc.DestDir)
	if err != nil {
		usageError("Destination (%s) is invalid: %s", dst, err)
	}

	fmt.Printf("\033[1mInput dirs:\033[0m\n  - Source: %q\n  - Dest: %q\n", src, dst)
	fmt.Printf("\033[32;1mCleaned dirs:\033[0m\n  - Source %q\n  - Dest: %q\n\n", fc.SourceDir, fc.DestDir)

	var info os.FileInfo
	info, err = os.Stat(fc.SourceDir)
	if err != nil {
		usageError("Source (%s) is invalid: %s", fc.SourceDir, err)
	}
	if !info.IsDir() {
		usageError("Source (%s) is invalid: not a directory", fc.SourceDir)
	}

	_, err = os.Stat(fc.DestDir)
	if err == nil || !os.IsNotExist(err) {
		usageError("Destination (%s) already exists", fc.DestDir)
	}

	return fc
}

func main() {
	var fixContext = getArgs()

	// Read the batch XML to get a list of issue directories to skip
	var batchPath = filepath.Join(fixContext.SourceDir, "data", "batch.xml")
	var newBatchPath = filepath.Join(fixContext.DestDir, "data", "batch.xml")

	log.Printf("INFO: Reading source batch XML %q", batchPath)
	var batch, err = ParseBatch(batchPath, fixContext.IssueKeys)
	if err != nil {
		log.Fatalf("ERROR: Unable to process batch XML file %q: %s", batchPath, err)
	}

	log.Printf("INFO: Writing new batch XML to %q", newBatchPath)
	err = batch.WriteBatchXML(newBatchPath)
	fixContext.SkipDirs = batch.SkipDirs

	// Crawl all files and determine the action necessary.  NOTE: this may not be
	// the ideal number of workers.  On an SSD, it seems to work much faster than
	// lower numbers.  One of the following must be true, but I dunno which:
	// - Go's IO is really bad when not parallelized
	// - My code is doing more CPU-intense logic than it seems like it should
	// - SSD write queuing is just super amazing
	var queue = NewWorkQueue(fixContext, 2*runtime.NumCPU())
	var walker = NewWalker(fixContext, queue)
	err = walker.Walk()
	if err != nil {
		log.Fatalf("Error trying to copy/fix the batch: %s\n", err)
	}

	// Wait for the queue to complete all actions/jobs
	queue.Wait()
}
