// Package taskid is the Go side of the detect-task id gate: the outer bound every
// tracker's task id must fit inside before it may reach a shell, a path or a
// tracker verb. The markdown gate lives in plugins/lets/skills/detect-task/SKILL.md;
// initcmd's TestTaskIDMatchesGate keeps the two in agreement.
//
// Leaf package: standard library only.
package taskid

import "regexp"

var validRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Valid reports whether id is a non-empty task id inside the gate's class
// [A-Za-z0-9._-] that does not start with a hyphen (a leading hyphen turns the id
// into an option at the verb).
func Valid(id string) bool { return id != "" && id[0] != '-' && validRe.MatchString(id) }
