// Package version exposes the lets CLI version string.
package version

// Version is set at build time via -ldflags. Default is a sentinel string
// meaning "untagged dev build - see git log for actual content". Decoupled
// from the release minor so dev builds never need a manual bump.
//
// Tests that mutate this must snapshot and restore via t.Cleanup() to avoid
// corrupting other tests running in parallel within the same package.
var Version = "0.0.0-dev"
