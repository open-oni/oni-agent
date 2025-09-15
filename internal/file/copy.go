package file

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/afero"
)

// doCopy is a general file-copying utility for ensuring that we gather (and
// return) all possible errors as well as we can
func doCopy(fs afero.Fs, src, dest string) (err error) {
	// Allow for the possibility of src and dest being the same file, in which
	// case our job is already done
	if src == dest {
		return
	}

	var in, out afero.File

	in, err = fs.Open(src)
	if err != nil {
		return fmt.Errorf("unable to read %q: %s", src, err)
	}
	defer in.Close()

	out, err = fs.Create(dest)
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

// Copy copies a file on a custom afero filesystem, and retries up to maxRetry
// times to allow for cases where network storage issues are temporarily giving
// us problems. Each failure will just wait one second until it tries again,
// since many problems (permissions, disk full, etc.) are fatal, and we don't
// want to hold up a massive copy job for more than a few seconds if there's a
// "real" problem that a retry won't help.
func Copy(fs afero.Fs, src, dest string, maxRetry int) (err error) {
	for n := 0; n < maxRetry; n++ {
		if n > 0 {
			time.Sleep(time.Second)
		}
		err = doCopy(fs, src, dest)
		if err == nil {
			return err
		}
	}

	return err
}
