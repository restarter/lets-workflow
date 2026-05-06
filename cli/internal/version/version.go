// Package version exposes the lets CLI version string.
package version

// Version is set at build time via -ldflags. Default is the dev placeholder
// used when building from a non-tagged commit.
//
// Tests that mutate this must snapshot and restore via t.Cleanup() to avoid
// corrupting other tests running in parallel within the same package.
var Version = "0.4.0-dev"
