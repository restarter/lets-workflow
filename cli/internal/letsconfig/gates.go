package letsconfig

// ShippedGates is the set of LETS_GATES levels, strictest first. strict keeps
// every pre-lets-wu2xl question; standard asks only for the hard tier of the
// core rules' gate table; minimal asks only for the floor.
var ShippedGates = []string{"strict", "standard", "minimal"}

// GatesDefault is the level an empty or unknown LETS_GATES counts as. Never
// minimal: a typo must not lower the gates.
const GatesDefault = "standard"

// ValidGates reports whether s is an accepted LETS_GATES value. The empty string
// is NOT valid here - callers treat "" as "not supplied".
func ValidGates(s string) bool {
	for _, g := range ShippedGates {
		if g == s {
			return true
		}
	}
	return false
}

// GatesLevel is the effective level of a raw LETS_GATES value: the value itself
// when valid, else GatesDefault. The SessionStart hook injects only this.
func GatesLevel(raw string) string {
	if ValidGates(raw) {
		return raw
	}
	return GatesDefault
}
