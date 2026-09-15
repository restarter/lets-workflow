package taskid

import "testing"

func TestValid(t *testing.T) {
	for _, id := range []string{"lets-abc", "48647", "lets-abc.1", "PWA-45122", "a_b"} {
		if !Valid(id) {
			t.Errorf("%q must be valid", id)
		}
	}
	for _, id := range []string{"", "-x", "--help", "a b", "a;b", "$(x)", "a/b", "x\n", "`x`"} {
		if Valid(id) {
			t.Errorf("%q must be invalid", id)
		}
	}
}
