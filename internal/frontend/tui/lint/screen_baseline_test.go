package lint_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// screenBaseline pins the maximum number of `lipgloss.NewStyle()` call
// sites allowed per screen file. The numbers reflect the legitimate
// long-tail after P4c (layout-chain Width/Padding/Border, dynamic
// color resolvers, pre-built styles in tight render loops).
//
// Adding to a screen: if a new NewStyle is needed, first ask whether
// a components/theme builder (Dim, Strong, Heading, …) covers the
// case. Only if the call genuinely needs a layout-chain bump the
// baseline here in the same commit. Reviewers see the bump; the
// commit message documents why.
//
// Reducing: when a refactor removes NewStyle from a file, run the
// suite — the test logs the new lower count. Update the baseline in
// the same commit so the next PR can't regress past the new floor.
var screenBaseline = map[string]int{
	"cheatsheet/model.go": 1,
	"palette/model.go":    8,
	"projects/model.go":   2,
	"worktime/dayoffs.go": 5,
	"worktime/history.go": 29,
	"worktime/model.go":   2,
	"worktime/today.go":   3,
	"worktime/week.go":    13,
}

// TestScreenInlineNewStyleBudget walks the internal/frontend/tui/screen
// tree and asserts each screen file's lipgloss.NewStyle() call count
// stays at or below screenBaseline.
//
// docs/design-system-audit.md §2.6 — "depguard erweitern"; intent is
// that screens consume components/theme builders for body styles and
// reach for raw lipgloss only when the layout API is actually needed.
func TestScreenInlineNewStyleBudget(t *testing.T) {
	t.Parallel()

	root := findScreensDir(t)
	files := walkScreenFiles(t, root)

	for relpath, fpath := range files {
		got := countLipglossNewStyle(t, fpath)
		want, pinned := screenBaseline[relpath]
		switch {
		case !pinned && got == 0:
			// New screen file with no NewStyle — fine, no entry needed.
		case !pinned && got > 0:
			t.Errorf("screen %s has %d lipgloss.NewStyle() but no baseline entry. "+
				"If the calls are layout-chain (Width/Padding/Border/Place*), add "+
				"the file to screenBaseline with the current count.", relpath, got)
		case got > want:
			t.Errorf("screen %s: %d lipgloss.NewStyle() calls, baseline is %d "+
				"(audit §2.6: prefer components/theme builders for non-layout styles)",
				relpath, got, want)
		case got < want:
			t.Logf("screen %s: %d lipgloss.NewStyle() calls, baseline is %d — "+
				"lower the baseline in this commit", relpath, got, want)
		}
	}

	// Also check: every entry in the baseline still corresponds to a
	// real file. Stale entries indicate someone deleted a screen but
	// forgot to clean up the budget map.
	for relpath := range screenBaseline {
		if _, ok := files[relpath]; !ok {
			t.Errorf("screenBaseline has stale entry for %q (file no longer exists)", relpath)
		}
	}
}

// findScreensDir walks up from the test's working directory until it
// finds the repo's screen package root. Avoids hard-coding an absolute
// path so the test runs whether invoked via `go test ./...` from the
// repo root or from inside the package.
func findScreensDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, "internal", "frontend", "tui", "screen")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	t.Fatalf("could not find internal/frontend/tui/screen above %q", wd)
	return ""
}

// walkScreenFiles returns a map of "<screen>/<file>.go" → absolute
// path, skipping _test.go files (only production code is budgeted).
func walkScreenFiles(t *testing.T, root string) map[string]string {
	t.Helper()

	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files[rel] = path
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	return files
}

// countLipglossNewStyle parses fpath and counts call expressions that
// match `lipgloss.NewStyle`. It deliberately does NOT match qualified
// imports under a different alias — screens consistently import
// `lipgloss` by its package name.
func countLipglossNewStyle(t *testing.T, fpath string) int {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fpath, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", fpath, err)
	}

	count := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "lipgloss" && sel.Sel.Name == "NewStyle" {
			count++
		}
		return true
	})
	return count
}
