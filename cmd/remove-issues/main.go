package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/open-oni/oni-agent/internal/batchfix"
	"github.com/spf13/afero"
)

var appName string

func printUsage(msg string, args ...interface{}) {
	var fmsg = fmt.Sprintf(msg, args...)
	fmt.Printf("\033[91;1mERROR:\033[97m %s\033[m\n", fmsg)
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
	FS        afero.Fs
}

type usageError string

func newUsageError(format string, args ...any) usageError {
	return usageError(fmt.Sprintf(format, args...))
}

func (u usageError) Error() string {
	return string(u)
}

// getArgs does some sanity-checking and sets the source/dest args
func getArgs(fs afero.Fs, args []string) (*config, error) {
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
		FS:        fs,
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

	var info os.FileInfo
	info, err = conf.FS.Stat(conf.SourceDir)
	if err != nil {
		return nil, newUsageError("invalid source (%q): %s", conf.SourceDir, err)
	}
	if !info.IsDir() {
		return nil, newUsageError("invalid source (%q): not a directory", conf.SourceDir)
	}

	_, err = conf.FS.Stat(conf.DestDir)
	if err == nil || !os.IsNotExist(err) {
		return nil, newUsageError("invalid destination (%q): already exists", conf.DestDir)
	}

	return conf, nil
}

// run contains the main logic of the application, allowing it to be called
// from tests without exiting the program
func run(fs afero.Fs, args ...string) error {
	var conf, err = getArgs(fs, args)
	if err != nil {
		return fmt.Errorf("getting options from args: %w", err)
	}

	var fixer = batchfix.NewFixer(conf.FS, conf.SourceDir, conf.DestDir)
	err = fixer.RemoveIssues(conf.IssueKeys)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var err = run(afero.NewOsFs(), os.Args...)
	if err != nil {
		printUsage(err.Error())
		os.Exit(1)
	}

	log.Printf("INFO: All files processed.")
}
