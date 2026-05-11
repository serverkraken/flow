package lint_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCountLipglossNewStyle_MetaTest is the meta-test the reviewer
// asked for: countLipglossNewStyle is the heart of the per-file budget
// gate, and its counting logic was previously trusted on faith. A
// regression where the counter accidentally counts strings or comments
// would bump the baseline silently and let production drift unchecked.
//
// Each fixture below is a self-contained Go file with a known number of
// `lipgloss.NewStyle()` call sites. Comments, raw strings, qualified
// imports under different aliases, and chained calls must NOT inflate
// the count.
//
// The counter itself is a private helper — this test compiles in the
// same _test package and reaches it directly.
func TestCountLipglossNewStyle_MetaTest(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "zero calls",
			body: `package x
func F() string { return "" }
`,
			want: 0,
		},
		{
			name: "single call",
			body: `package x
import "github.com/charmbracelet/lipgloss"
var s = lipgloss.NewStyle()
`,
			want: 1,
		},
		{
			name: "three calls",
			body: `package x
import "github.com/charmbracelet/lipgloss"
var (
    a = lipgloss.NewStyle()
    b = lipgloss.NewStyle().Bold(true)
    c = lipgloss.NewStyle().Width(40)
)
`,
			want: 3,
		},
		{
			name: "comment with call text does not count",
			body: `package x
// lipgloss.NewStyle() — example, not a real call
func F() {}
`,
			want: 0,
		},
		{
			name: "string literal with call text does not count",
			body: `package x
var s = "lipgloss.NewStyle()"
`,
			want: 0,
		},
		{
			name: "different alias does not count",
			body: `package x
import lg "github.com/charmbracelet/lipgloss"
var s = lg.NewStyle()
`,
			want: 0,
		},
		{
			name: "chained calls count once",
			body: `package x
import "github.com/charmbracelet/lipgloss"
var s = lipgloss.NewStyle().Bold(true).Width(40).Padding(1, 2)
`,
			want: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.go")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := callCountLipglossNewStyle(t, path)
			if got != tc.want {
				t.Errorf("count: got %d, want %d (body:\n%s)", got, tc.want, tc.body)
			}
		})
	}
}

// callCountLipglossNewStyle is a thin shim to expose the package-private
// helper countLipglossNewStyle to this _test file. The shim lives in
// the same _test package so it inherits package visibility.
func callCountLipglossNewStyle(t *testing.T, path string) int {
	return countLipglossNewStyle(t, path)
}
