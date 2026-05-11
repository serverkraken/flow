package lint_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFindLipglossAliasImports_MetaTest pins the detector that closes
// the Q3 gap in TestScreenInlineNewStyleBudget: countLipglossNewStyle
// matches `lipgloss.NewStyle()` by literal package identifier, so a
// file importing `lg "github.com/charmbracelet/lipgloss"` would bypass
// the budget. The walker test below catches that drift on real files;
// this meta-test pins the detector itself so the walker can be trusted.
func TestFindLipglossAliasImports_MetaTest(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantAliases []string
	}{
		{
			name: "no lipgloss import",
			body: `package x
import "fmt"
func F() { fmt.Println("hi") }
`,
			wantAliases: nil,
		},
		{
			name: "canonical import — no alias drift",
			body: `package x
import "github.com/charmbracelet/lipgloss"
var s = lipgloss.NewStyle()
`,
			wantAliases: nil,
		},
		{
			name: "explicit canonical alias — still no drift",
			body: `package x
import lipgloss "github.com/charmbracelet/lipgloss"
var s = lipgloss.NewStyle()
`,
			wantAliases: nil,
		},
		{
			name: "short alias drifts",
			body: `package x
import lg "github.com/charmbracelet/lipgloss"
var s = lg.NewStyle()
`,
			wantAliases: []string{"lg"},
		},
		{
			name: "dot import drifts",
			body: `package x
import . "github.com/charmbracelet/lipgloss"
var s = NewStyle()
`,
			wantAliases: []string{"."},
		},
		{
			name: "blank import drifts",
			body: `package x
import _ "github.com/charmbracelet/lipgloss"
`,
			wantAliases: []string{"_"},
		},
		{
			name: "lipgloss is one of several imports — only that one matters",
			body: `package x
import (
    "fmt"
    lg "github.com/charmbracelet/lipgloss"
    "strings"
)
var _ = fmt.Sprintf("%s", strings.ToLower("x"))
var s = lg.NewStyle()
`,
			wantAliases: []string{"lg"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.go")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := findLipglossAliasImports(t, path)
			gotAliases := make([]string, 0, len(got))
			for _, a := range got {
				gotAliases = append(gotAliases, a.alias)
			}
			if !equalStringSlices(gotAliases, tc.wantAliases) {
				t.Errorf("aliases: got %v, want %v (body:\n%s)", gotAliases, tc.wantAliases, tc.body)
			}
		})
	}
}

// TestNoLipglossAliasImportsInRepo walks the entire internal/ tree
// (production code + tests) and asserts no file imports lipgloss under
// an alias. The screen-baseline counter matches `lipgloss.NewStyle()`
// by package identifier; any aliased import would silently bypass the
// budget. Catching it at the import site is simpler than rewriting the
// counter as an AST-resolver, and rejects the gap categorically.
func TestNoLipglossAliasImportsInRepo(t *testing.T) {
	t.Parallel()

	root := findInternalDir(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		aliases := findLipglossAliasImports(t, path)
		for _, a := range aliases {
			rel, _ := filepath.Rel(root, path)
			violations = append(violations,
				rel+":"+strconv.Itoa(a.line)+" imports lipgloss as "+a.alias)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	if len(violations) > 0 {
		t.Errorf("lipgloss must be imported without an alias so the "+
			"NewStyle budget (TestScreenInlineNewStyleBudget) catches all "+
			"call sites. Violations:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func findInternalDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, "internal")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	t.Fatalf("could not find internal/ above %q", wd)
	return ""
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
