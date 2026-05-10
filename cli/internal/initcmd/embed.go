// FROZEN SNAPSHOT - DO NOT EDIT.
//
// embedded_statusline_shim.sh in this directory is a verbatim copy of
// plugins/lets-workflow/scripts/lets/statusline.sh as of Phase 4a (the version users
// have on disk before they upgrade to Phase 4b). It is INTENTIONALLY decoupled
// from the live plugin source: changing the plugin's statusline.sh later must
// NOT change this snapshot, otherwise byte-equal detection of "this user has
// our shim" would silently break for already-installed users.
//
// If we ever ship another shim variant, add a SECOND embedded snapshot here
// (e.g. embedded_statusline_shim_phase4c.sh) and check both - never mutate
// the existing snapshots.
package initcmd

import _ "embed"

// embeddedStatuslineShim contains the canonical 12-line shim content of
// plugins/lets-workflow/scripts/lets/statusline.sh as of Phase 4a. Used for byte-equal
// detection: if a project's .lets/statusline.sh exactly matches this content
// (or matches the legacy 209-line bash via separate detection), it's safe to
// delete; anything else is "foreign" and gets a notice instead of deletion.
//
//go:embed embedded_statusline_shim.sh
var embeddedStatuslineShim []byte
