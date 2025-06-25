package main

import (
	"fmt"
	"io"
	"os"
)

// copyfile is a general file-copying utility for ensuring that we gather (and
// return) all possible errors as well as we can
func copyfile(src, dest string) (err error) {
	// Allow for the possibility of src and dest being the same file, in which
	// case our job is already done
	if src == dest {
		return
	}

	var in, out *os.File

	in, err = os.Open(src)
	if err != nil {
		return fmt.Errorf("unable to read %q: %s", src, err)
	}
	defer in.Close()

	out, err = os.Create(dest)
	if err != nil {
		return fmt.Errorf("unable to create %q: %q", dest, err)
	}

	defer func() {
		var xerr = out.Close()
		if xerr != nil && err == nil {
			err = fmt.Errorf("unable to close %q: %s", dest, xerr)
			return
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("unable to write to %q: %s", dest, err)
	}

	err = out.Sync()
	if err != nil {
		return fmt.Errorf("unable to sync %q: %s", dest, err)
	}

	return
}
