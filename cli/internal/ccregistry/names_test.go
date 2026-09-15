package ccregistry

import (
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	ok := []string{"MAIN", "MAIN PWA", "MAIN-FABLE", "bitbucket-prod", "Оркестратор", "a.b_c"}
	bad := []string{"", `a"b`, "a$b", "a`b", `a\b`, "a\nb", "-x", "x --auto", "a  b", " a", strings.Repeat("я", 65)}
	for _, n := range ok {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false", n)
		}
	}
	for _, n := range bad {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true", n)
		}
	}
}

func TestValidSession(t *testing.T) {
	if !ValidSession("0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d") {
		t.Error("lowercase uuid refused")
	}
	for _, s := range []string{"", "0A1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D", "0a1b2c3d4e5f4a6b8c7d9e0f1a2b3c4d", "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d ", "../x"} {
		if ValidSession(s) {
			t.Errorf("ValidSession(%q) = true", s)
		}
	}
}
