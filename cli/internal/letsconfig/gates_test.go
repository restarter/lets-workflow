package letsconfig_test

import (
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

func TestValidGates(t *testing.T) {
	cases := map[string]bool{
		"strict":   true,
		"standard": true,
		"minimal":  true,
		"":         false,
		"STANDARD": false,
		"bypass":   false,
	}
	for in, want := range cases {
		if got := letsconfig.ValidGates(in); got != want {
			t.Errorf("ValidGates(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGatesLevel(t *testing.T) {
	cases := map[string]string{
		"":         "standard",
		"minimal":  "minimal",
		"strict":   "strict",
		"standard": "standard",
		"x":        "standard",
		"MINIMAL":  "standard",
	}
	for in, want := range cases {
		if got := letsconfig.GatesLevel(in); got != want {
			t.Errorf("GatesLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// The default must never be the lowest level: an unknown value falls back to
// it, and a typo must not lower the gates.
func TestGatesDefault_NotMinimal(t *testing.T) {
	if letsconfig.GatesDefault == "minimal" || !letsconfig.ValidGates(letsconfig.GatesDefault) {
		t.Errorf("GatesDefault = %q, want a valid level other than minimal", letsconfig.GatesDefault)
	}
	if letsconfig.Defaults()["LETS_GATES"] != letsconfig.GatesDefault {
		t.Errorf("Keys default for LETS_GATES = %q, want GatesDefault %q", letsconfig.Defaults()["LETS_GATES"], letsconfig.GatesDefault)
	}
}
