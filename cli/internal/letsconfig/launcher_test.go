package letsconfig

import "testing"

func TestValidLauncher(t *testing.T) {
	for _, ok := range ShippedLaunchers {
		if !ValidLauncher(ok) {
			t.Errorf("ValidLauncher(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "tmix", "TMUX", "zellij", "../etc"} {
		if ValidLauncher(bad) {
			t.Errorf("ValidLauncher(%q) = true, want false", bad)
		}
	}
}

func TestShippedLaunchers_ContainsDefault(t *testing.T) {
	def := Defaults()["LETS_LAUNCHER"]
	if !ValidLauncher(def) {
		t.Fatalf("default launcher %q is not in ShippedLaunchers %v", def, ShippedLaunchers)
	}
}
