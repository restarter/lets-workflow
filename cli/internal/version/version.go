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

// Format renders a version string for human display, eliding the "v" prefix
// for the dev sentinel and rendering the empty string as "legacy" (used by
// `lets init` when reporting regen of a .env file written before the
// LETS_ENV_VERSION marker was introduced).
//
// Examples: Format("0.5.1") → "v0.5.1"; Format("dev") → "dev";
// Format("") → "legacy".
func Format(v string) string {
	switch v {
	case "":
		return "legacy"
	case "dev":
		return "dev"
	}
	return "v" + v
}
