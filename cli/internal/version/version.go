// Package version exposes the lets CLI version string.
package version

import "strings"

// Version is set at build time via -ldflags. Default sentinel "dev" means
// "untagged build - see git log for actual content". `dev-<metadata>` patterns
// (e.g. "dev-feat-4fd62e0-dirty") are also treated as dev for renderer
// purposes via IsDev(). Decoupled from the release minor so dev builds never
// need a manual bump.
//
// Renderers that prefix `v` (statusline, cobra --version) check IsDev() and
// omit the prefix to avoid awkward `vdev` rendering.
//
// Tests that mutate this must snapshot and restore via t.Cleanup() to avoid
// corrupting other tests running in parallel within the same package.
var Version = "dev"

// IsDev reports whether the build is an untagged dev build. Covers:
//   - "dev" exactly: Go default sentinel (no ldflags set).
//   - "dev-<metadata>": rich dev stamping (e.g. "dev-mybranch-4fd62e0[-dirty]"
//     from scripts/dev/run.sh). Requires non-empty suffix to avoid blessing
//     a literal "dev-" as valid metadata.
//
// updatecmd consumes this to skip env-version regeneration on dev binaries;
// statusline consumes it to elide the `v` prefix.
func IsDev() bool {
	if Version == "dev" {
		return true
	}
	return strings.HasPrefix(Version, "dev-") && len(Version) > 4
}

// Format renders a version string for human display, eliding the "v" prefix
// for dev sentinels (incl. dev-<metadata> form) and rendering the empty
// string as "legacy" (used by `lets init` when reporting regen of a .env
// file written before the LETS_ENV_VERSION marker was introduced).
//
// Examples:
//
//	Format("0.5.1")              → "v0.5.1"
//	Format("dev")                → "dev"
//	Format("dev-feat-abc1234")   → "dev-feat-abc1234"
//	Format("")                   → "legacy"
//	Format("dev-")               → "vdev-" (not a valid dev suffix; falls through)
func Format(v string) string {
	if v == "" {
		return "legacy"
	}
	if v == "dev" || (strings.HasPrefix(v, "dev-") && len(v) > 4) {
		return v
	}
	return "v" + v
}
