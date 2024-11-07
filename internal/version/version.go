// Package version exposes a variable for the app to report its version, which
// is meant to be replaced at compile time with git metadata
package version

// Version holds the app version data. Do not change this, as it's meant to be
// replaced (see the Makefile)
var Version = "in-dev"
