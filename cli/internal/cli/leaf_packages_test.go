package cli

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// leafPackages import the standard library or another listed leaf only (the import
// direction in plan lets-ip06f section 4). A leaf must never import a package that
// imports it back, and the easiest way to guarantee that is to allow no non-leaf
// import at all. Append a directory when a leaf lands.
var leafPackages = []string{"trackeradapter", "taskid", "fsutil", "peername", "taskstate", "redact", "ccregistry", "agentrun", "teamfile", "memberscmd", "integratecmd", "gitutil"}

const modulePrefix = "github.com/restarter/lets-workflow/cli/internal/"

// deniedEdges are imports that compile today but break the plan's import direction
// (section 4): a forbidden edge fails here even before it forms a cycle. A package
// directory that does not exist yet is skipped.
var deniedEdges = map[string][]string{
	"worktreecmd": {"peerscmd", "orcacmd"},
	"orcacmd":     {"peerscmd"},
	"handoffcmd":  {"peerscmd"}, // the handoff lane never routes through the peer lane (lets-w5tm5)
}

func TestDeniedImports(t *testing.T) {
	fset := token.NewFileSet()
	for pkg, denied := range deniedEdges {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		for _, f := range files {
			af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
			if err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			for _, imp := range af.Imports {
				dep, ok := strings.CutPrefix(strings.Trim(imp.Path.Value, `"`), modulePrefix)
				if ok && slices.Contains(denied, dep) {
					t.Errorf("%s imports %s - %s must never import %v (plan section 4)", f, dep, pkg, denied)
				}
			}
		}
	}
}

func TestLeafPackages(t *testing.T) {
	leaves := map[string]bool{}
	for _, l := range leafPackages {
		leaves[l] = true
	}
	fset := token.NewFileSet()
	for _, pkg := range leafPackages {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go")) // cli/internal/cli -> cli/internal/<pkg>
		var prod int
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			prod++
			af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
			if err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			for _, imp := range af.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				if dep, ok := strings.CutPrefix(p, modulePrefix); ok && leaves[dep] {
					continue // leaf -> leaf is allowed
				}
				if first := strings.SplitN(p, "/", 2)[0]; strings.Contains(first, ".") {
					t.Errorf("%s imports %s - leaf packages import the standard library or another leaf only", f, p)
				}
			}
		}
		if prod == 0 {
			t.Errorf("leaf %s: no Go files - stale table entry", pkg)
		}
	}
}
