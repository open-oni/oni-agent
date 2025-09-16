package main

import (
	"fmt"
	"log"
	"os"

	"github.com/open-oni/oni-agent/internal/batchfix"
	"github.com/spf13/afero"
)

var appName string

func printUsage(msg string, args ...any) {
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
		return nil, fmt.Errorf("missing one or more arguments")
	}

	var conf = &config{
		SourceDir: args[1],
		DestDir:   args[2],
		IssueKeys: args[3:],
		FS:        fs,
	}
	return conf, nil
}

// run contains the main logic of the application, allowing it to be called
// from tests without exiting the program
func run(fs afero.Fs, args ...string) error {
	var conf, err = getArgs(fs, args)
	if err != nil {
		printUsage(err.Error())
		os.Exit(1)
	}

	var fixer *batchfix.Fixer
	fixer, err = batchfix.NewFixer(conf.FS, conf.SourceDir, conf.DestDir)
	if err == nil {
		err = fixer.RemoveIssues(conf.IssueKeys)
	}
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var err = run(afero.NewOsFs(), os.Args...)
	if err != nil {
		os.Exit(1)
	}

	log.Print("INFO: All files processed.")
}
