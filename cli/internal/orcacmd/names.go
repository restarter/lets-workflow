//go:build unix

package orcacmd

import "regexp"

var handleRe = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)

// ValidHandle reports whether h is shaped like an Orca terminal handle
// (`term_<uuid>`), so a handle read from Orca output never reaches argv unchecked.
func ValidHandle(h string) bool { return handleRe.MatchString(h) }
