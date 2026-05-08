// Package version exposes the lets CLI version string.
package version

// Version is set at build time via -ldflags. Default sentinel "dev" means
// "untagged build - see git log for actual content". Decoupled from the
// release minor so dev builds never need a manual bump.
//
// Renderers that prefix `v` (statusline, cobra --version) check for this
// sentinel and omit the prefix to avoid awkward `vdev` rendering.
//
// Tests that mutate this must snapshot and restore via t.Cleanup() to avoid
// corrupting other tests running in parallel within the same package.
var Version = "dev"

// IsDev reports whether the build is an untagged dev build (no -ldflags set
// at compile time). Renderers use this to elide the `v` prefix.
func IsDev() bool {
	return Version == "dev"
}
