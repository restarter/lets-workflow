package peername

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	for _, n := range []string{"MAIN-PWA", "MAIN PWA", "ГОЛОВНИЙ", "main-lic.fable", "a_b", strings.Repeat("a", 64)} {
		if !Valid(n) {
			t.Errorf("%q must be valid", n)
		}
	}
	for _, n := range []string{"", "x --auto", "a  b", " a", "a ", "-x", `a"b`, "a$b", "a`b", "a\nb", strings.Repeat("a", 65), "a/b"} {
		if Valid(n) {
			t.Errorf("%q must be invalid", n)
		}
	}
}
