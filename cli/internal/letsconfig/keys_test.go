package letsconfig_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

func TestKeys_NoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range letsconfig.Keys {
		if seen[k.Name] {
			t.Errorf("duplicate key: %s", k.Name)
		}
		seen[k.Name] = true
	}
}

func TestKeys_AllPrefixed(t *testing.T) {
	for _, k := range letsconfig.Keys {
		if !strings.HasPrefix(k.Name, "LETS_") {
			t.Errorf("key %q does not start with LETS_", k.Name)
		}
	}
}

func TestKeys_AllHaveComments(t *testing.T) {
	for _, k := range letsconfig.Keys {
		if k.Comment == "" {
			t.Errorf("key %s has empty comment", k.Name)
		}
	}
}

func TestKeys_AllHaveDefaults(t *testing.T) {
	for _, k := range letsconfig.Keys {
		if k.Default == "" {
			t.Errorf("key %s has empty default — required for .env.example generation", k.Name)
		}
	}
}

func TestDefaults_MatchesKeys(t *testing.T) {
	d := letsconfig.Defaults()
	if len(d) != len(letsconfig.Keys) {
		t.Fatalf("Defaults len: got %d want %d", len(d), len(letsconfig.Keys))
	}
	for _, k := range letsconfig.Keys {
		if d[k.Name] != k.Default {
			t.Errorf("Defaults[%s]: got %q want %q", k.Name, d[k.Name], k.Default)
		}
	}
}

// TestDefaults_HardcodedContract complements MatchesKeys — that test verifies
// the SHAPE of Defaults() matches Keys (catches caching/filtering regressions).
// This test verifies the VALUES are what we promise externally (catches drift
// when someone changes a default in Keys without realizing it ships to users).
func TestDefaults_HardcodedContract(t *testing.T) {
	want := map[string]string{
		"LETS_LANGUAGE":     "English",
		"LETS_MERGE_BRANCH": "main",
		"LETS_PR_FLOW":      "local",
		"LETS_TRACKER":      "beads",
		"LETS_LAUNCHER":     "terminal",
	}
	got := letsconfig.Defaults()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Defaults() value drift: got %v want %v", got, want)
	}
}

func TestNames_MatchesKeys(t *testing.T) {
	got := letsconfig.Names()
	if len(got) != len(letsconfig.Keys) {
		t.Fatalf("Names len: got %d want %d", len(got), len(letsconfig.Keys))
	}
	for i, name := range got {
		if name != letsconfig.Keys[i].Name {
			t.Errorf("Names[%d]: got %q want %q", i, name, letsconfig.Keys[i].Name)
		}
	}
}
