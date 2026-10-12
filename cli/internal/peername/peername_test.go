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

func TestRepoSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"/x/lets-workflow", "lets-workflow"},
		{"/x/My Repo.git", "my-repo-git"},
		{"/", ""},
		{"/x/" + strings.Repeat("a", 70), strings.Repeat("a", 64)},
	} {
		if got := RepoSlug(c.in); got != c.want {
			t.Errorf("RepoSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOrchestrator(t *testing.T) {
	if got := Orchestrator("lets-workflow", ""); got != "lets-workflow-orc" {
		t.Errorf("Orchestrator plain = %q", got)
	}
	if got := Orchestrator("lets-workflow", "docs"); got != "lets-workflow-orc-docs" {
		t.Errorf("Orchestrator scoped = %q", got)
	}
}

func TestParseOrchestrator(t *testing.T) {
	for _, c := range []struct {
		name  string
		scope string
		ok    bool
	}{
		{"lets-workflow-orc", "", true},
		{"Lets-Workflow-orc-docs", "docs", true},
		{"lets-workflow-orchestra", "", false},
		{"lets-workflow-orc-", "", false},
		{"MAIN-LETS", "", false},
		{"other-orc", "", false},
	} {
		scope, ok := ParseOrchestrator("lets-workflow", c.name)
		if scope != c.scope || ok != c.ok {
			t.Errorf("ParseOrchestrator(%q) = (%q, %v), want (%q, %v)", c.name, scope, ok, c.scope, c.ok)
		}
	}
	if _, ok := ParseOrchestrator("", "lets-workflow-orc"); ok {
		t.Error("empty repo must not match")
	}
}

func TestWorker(t *testing.T) {
	w := Worker("lets-workflow", "lets-abc")
	if w != "lets-workflow-lets-abc" || !Valid(w) {
		t.Errorf("Worker = %q, valid=%v", w, Valid(w))
	}
}
