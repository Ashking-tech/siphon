// Package version carries build identity.
package version

import "fmt"

// Version is overridden at build time with -ldflags.
var Version = "0.1.0-dev"

// Commit is the git revision, when injected.
var Commit = ""

// String renders the user-facing version.
func String() string {
	if Commit != "" {
		return fmt.Sprintf("%s (%s)", Version, Commit)
	}
	return Version
}
