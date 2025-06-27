package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

var appName string

func printUsage(msg string, args ...interface{}) {
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
`, appName)
}

// config just holds the app's directory/lccn context so we don't have global
// variables puked out everywhere but we also don't pass a million args around
// to everything
type config struct {
	SourceDir string
	DestDir   string
	IssueKeys []string
	SkipDirs  []string
}

type usageError string

func newUsageError(format string, args ...any) usageError {
	return usageError(fmt.Sprintf(format, args...))
}

func (u usageError) Error() string {
	return string(u)
}

// getArgs does some sanity-checking and sets the source/dest args
func getArgs(args []string) (*config, error) {
	if len(args) < 1 {
		panic("missing args[0]!")
	}
	appName = args[0]
	if len(args) < 4 {
		return nil, newUsageError("missing one or more arguments")
	}

	var src = args[1]
	var dst = args[2]
	var conf = &config{
		SourceDir: src,
		DestDir:   dst,
		IssueKeys: args[3:],
	}
	var err error
	conf.SourceDir, err = filepath.Abs(conf.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}
	conf.DestDir, err = filepath.Abs(conf.DestDir)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	fmt.Printf("\033[1mInput dirs:\033[0m\n  - Source: %q\n  - Dest: %q\n", src, dst)
	fmt.Printf("\033[32;1mCleaned dirs:\033[0m\n  - Source %q\n  - Dest: %q\n\n", conf.SourceDir, conf.DestDir)

	var info os.FileInfo
	info, err = os.Stat(conf.SourceDir)
	if err != nil {
		return nil, newUsageError("invalid source (%q): %s", conf.SourceDir, err)
	}
	if !info.IsDir() {
		return nil, newUsageError("invalid source (%q): not a directory", conf.SourceDir)
	}

	_, err = os.Stat(conf.DestDir)
	if err == nil || !os.IsNotExist(err) {
		return nil, newUsageError("invalid destination (%q): already exists", conf.DestDir)
	}

	return conf, nil
}

// run contains the main logic of the application, allowing it to be called
// from tests without exiting the program
func run(args ...string) error {
	var conf, err = getArgs(args)
	if err != nil {
		return fmt.Errorf("getting options from args: %w", err)
	}

	// Read the batch XML to get a list of issue directories to skip
	var batchPath = filepath.Join(conf.SourceDir, "data", "batch.xml")
	var newBatchPath = filepath.Join(conf.DestDir, "data", "batch.xml")

	log.Printf("INFO: Reading source batch XML %q", batchPath)
	var batch *batchXML
	batch, err = ParseBatch(batchPath, conf.IssueKeys)
	if err != nil {
		return fmt.Errorf("parsing batch: %w", err)
	}

	log.Printf("INFO: Writing new batch XML to %q", newBatchPath)
	err = batch.WriteBatchXML(newBatchPath)
	if err != nil {
		return fmt.Errorf("writing batch: %w", err)
	}
	conf.SkipDirs = batch.SkipDirs

	// Crawl all files and determine the action necessary.  NOTE: this may not be
	// the ideal number of workers.  On an SSD, it seems to work much faster than
	// lower numbers.  One of the following must be true, but I dunno which:
	// - Go's IO is really bad when not parallelized
	// - My code is doing more CPU-intense logic than it seems like it should
	// - SSD write queuing is just super amazing
	var queue = NewWorkQueue(conf, 2*runtime.NumCPU())
	var walker = NewWalker(conf, queue)
	err = walker.Walk()
	if err != nil {
		return fmt.Errorf("walking batch files: %w", err)
	}

	// Wait for the queue to complete all actions/jobs
	queue.Wait()
	return nil
}

func main() {
	var err = run(os.Args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err)
		os.Exit(1)
	}
}
