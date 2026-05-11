package lint_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// rawHues lists the Palette field names that screens must NOT read
// directly. The fix is to consume them via `theme.Palette.Sem()` so a
// palette swap shifts the whole UI in lockstep — see semantic.go for
// the canonical mapping.
//
// Dim, BgCode, BgChip, FgMuted, Fg, Bg etc. are deliberately NOT on
// this list: they are layout/scaffold colors with no semantic role yet
// (no Sem-Slot defined). The audit's §2.6 follow-up may add Border /
// BorderSubtle entries here when those Sem-Slot adopters land.
var rawHues = map[string]struct{}{
	"Red":     {},
	"Green":   {},
	"Yellow":  {},
	"Cyan":    {},
	"Blue":    {},
	"Purple":  {},
	"Magenta": {},
}

// TestScreenSemanticOnly walks the internal/frontend/tui/screen tree
// and fails when any screen file reads a raw Palette hue field
// directly (e.g. `pal.Green`, `h.pal.Cyan`, `w.pal.Red`). All
// screen-side color access must go through Sem() so a palette swap
// propagates uniformly — review finding "Theme-Compliance-Sweep".
//
// The check sits alongside screen_baseline_test.go (NewStyle count
// gate); both walk the same file set and use go/ast so they catch
// what golangci-lint's pattern matchers can't express cleanly.
func TestScreenSemanticOnly(t *testing.T) {
	t.Parallel()

	root := findScreensDir(t)
	files := walkScreenFiles(t, root)

	for relpath, fpath := range files {
		hits := findRawHueAccess(t, fpath)
		for _, h := range hits {
			t.Errorf("screen %s:%d: raw palette hue %q is forbidden — use theme.Palette.Sem() instead (semantic.go documents the mapping)",
				relpath, h.line, h.field)
		}
	}
}

type hueHit struct {
	line  int
	field string
}

// findRawHueAccess returns every SelectorExpr in fpath whose Sel.Name
// is in rawHues. That catches both `pal.Green` and `h.pal.Green` —
// the outer SelectorExpr's Sel is "Green" in both shapes — without
// needing to model the deeper expression shape.
//
// False-positive risk: a struct field happening to be named "Green"
// on a non-Palette type would also flag. Screen files have no such
// other types today; the cost of a future false positive (rename or
// add a narrow allowlist) is lower than the cost of letting drift
// re-establish itself.
func findRawHueAccess(t *testing.T, fpath string) []hueHit {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fpath, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", fpath, err)
	}

	var hits []hueHit
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, isHue := rawHues[sel.Sel.Name]; !isHue {
			return true
		}
		// Skip the case where the Sel sits on a literal Semantic
		// struct construction (theme.Semantic{Red: …}). Screens
		// never do that, but the parser sees the same Sel-Name
		// shape inside type literals as in field reads — guard via
		// the wrapping KeyValueExpr check below if it ever becomes
		// an issue. For now no screen file constructs Semantic.
		hits = append(hits, hueHit{
			line:  fset.Position(sel.Sel.Pos()).Line,
			field: sel.Sel.Name,
		})
		return true
	})
	return hits
}
