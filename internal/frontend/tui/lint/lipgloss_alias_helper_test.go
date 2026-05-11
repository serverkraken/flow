package lint_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// lipglossImportPath is the canonical import path of the styling
// library. The NewStyle budget pins the package name "lipgloss" by
// literal identifier; any import that binds the package to a
// different name lets call sites slip past the counter.
const lipglossImportPath = "github.com/charmbracelet/lipgloss"

// lipglossAlias is a detector hit: the non-canonical name a file uses
// for the lipgloss package, plus the line of the import spec.
type lipglossAlias struct {
	alias string
	line  int
}

// findLipglossAliasImports parses fpath and returns every import of
// lipgloss whose binding name differs from "lipgloss". Canonical
// imports (no alias) and explicit redundant aliases (`lipgloss "…"`)
// are not reported. Dot (`.`) and blank (`_`) imports are reported —
// they break the NewStyle counter just as cleanly.
func findLipglossAliasImports(t *testing.T, fpath string) []lipglossAlias {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fpath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", fpath, err)
	}

	var hits []lipglossAlias
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != lipglossImportPath {
			continue
		}
		if imp.Name == nil || imp.Name.Name == "lipgloss" {
			continue
		}
		hits = append(hits, lipglossAlias{
			alias: imp.Name.Name,
			line:  fset.Position(imp.Pos()).Line,
		})
	}
	return hits
}
