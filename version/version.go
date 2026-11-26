// Package version holds the program version.
package version

// Version is the current program version.
// It is overridden at build time via -ldflags "-X xteve/version.Version=vX.Y.Z", see build.sh.
var Version = "dev"
