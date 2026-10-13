//go:build unix

package ccregistry

import (
	"errors"
	"os"
	"testing"
)

// fakeTree makes ParentOf answer from parents (a pid missing from it is an error).
func fakeTree(t *testing.T, parents map[int]int) {
	t.Helper()
	old := ParentOf
	ParentOf = func(pid int) (int, error) {
		if p, ok := parents[pid]; ok {
			return p, nil
		}
		return 0, errors.New("no such process")
	}
	t.Cleanup(func() { ParentOf = old })
}

func TestCallerDescendsFrom(t *testing.T) {
	pp := os.Getppid()
	tree := map[int]int{pp: 900, 900: 800, 800: 1}
	for _, tc := range []struct {
		name    string
		parents map[int]int
		pid     int
		want    bool
		wantErr bool
	}{
		{"direct parent", tree, pp, true, false},
		{"grandparent", tree, 900, true, false},
		{"top of the chain", tree, 800, true, false},
		{"not on the chain", tree, 4242, false, false},
		{"pid zero", tree, 0, false, false},
		{"negative pid", tree, -5, false, false},
		{"unreadable hop", map[int]int{pp: 900}, 4242, false, true},
		{"self-parented stops", map[int]int{pp: 900, 900: 900}, 4242, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeTree(t, tc.parents)
			got, err := CallerDescendsFrom(tc.pid)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("CallerDescendsFrom(%d) = %v, %v; want %v, err=%v", tc.pid, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// A cycle never spins: the walk stops at MaxAncestry with an error.
func TestCallerDescendsFrom_Bounded(t *testing.T) {
	pp := os.Getppid()
	fakeTree(t, map[int]int{pp: 700, 700: 600, 600: 700})
	if got, err := CallerDescendsFrom(4242); got || err == nil {
		t.Errorf("a parent cycle: %v, %v; want false and an error", got, err)
	}
}

// The real platform walker finds this test's own parent and grandparent chain.
func TestCallerDescendsFrom_RealParent(t *testing.T) {
	if got, err := CallerDescendsFrom(os.Getppid()); !got || err != nil {
		t.Errorf("the direct parent: %v, %v", got, err)
	}
	if p, err := parentOf(os.Getpid()); err != nil || p != os.Getppid() {
		t.Errorf("parentOf(self) = %d, %v; want %d", p, err, os.Getppid())
	}
}
