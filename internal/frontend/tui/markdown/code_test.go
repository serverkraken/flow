package markdown

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestRender_FencedCodeBlock_PanelWithLanguageLabel: a ` ```go ` block
// renders as a top band carrying ` go `, content rows and a bottom
// band. The language label must be present on the strip-ANSI form so
// the user can identify the fence at a glance.
func TestRender_FencedCodeBlock_PanelWithLanguageLabel(t *testing.T) {
	t.Parallel()
	src := "```go\nfunc main() { fmt.Println(\"hi\") }\n```\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, " go ") {
		t.Errorf("language label 'go' missing from band:\n%s", plain)
	}
	if !strings.Contains(plain, "func main") {
		t.Errorf("code body missing from output:\n%s", plain)
	}
}

// TestRender_FencedCodeBlock_AllRowsShareWidth: every row of the
// rendered panel (top band + content rows + bottom band) must share
// the same visible width — that's what makes the block read as a
// rectangular panel rather than a ragged region.
func TestRender_FencedCodeBlock_AllRowsShareWidth(t *testing.T) {
	t.Parallel()
	src := "```go\nx := 1\nlongerLineHereWithMore()\nshort()\n```\n"
	out, err := Render(src, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	rows := codePanelRows(out)
	if len(rows) < 5 {
		t.Fatalf("expected >=5 panel rows (top band + 3 code + bottom band), got %d:\n%s", len(rows), out)
	}
	want := lipgloss.Width(rows[0])
	for i, row := range rows {
		if w := lipgloss.Width(row); w != want {
			t.Errorf("row %d width = %d, want %d (panel rows must share width)\n%q", i, w, want, row)
		}
	}
}

// TestRender_FencedCodeBlock_NeverExceedsWidth: when the longest line
// would push the panel past the width budget, the panel caps at the
// budget. Verifies the shrink-to-fit clamp.
func TestRender_FencedCodeBlock_NeverExceedsWidth(t *testing.T) {
	t.Parallel()
	src := "```go\n" + strings.Repeat("x", 200) + "\n```\n"
	const width = 60
	out, err := Render(src, width)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for i, row := range codePanelRows(out) {
		if w := lipgloss.Width(row); w > width {
			t.Errorf("panel row %d width = %d, exceeds budget %d", i, w, width)
		}
	}
}

// TestRender_FencedCodeBlock_UnknownLanguageStillPanelled: a fence
// with an unknown info string ('mylang') falls back to plain styling
// inside the panel — but the panel frame, language label, and
// content survival all still hold.
func TestRender_FencedCodeBlock_UnknownLanguageStillPanelled(t *testing.T) {
	t.Parallel()
	src := "```mylang\nplain content here\n```\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, " mylang ") {
		t.Errorf("language label still expected even for unknown lexer:\n%s", plain)
	}
	if !strings.Contains(plain, "plain content here") {
		t.Errorf("content missing for unknown-language fence:\n%s", plain)
	}
}

// TestRender_FencedCodeBlock_NoLanguageNoLabel: a fence without an
// info string yields a band without a language label — so prose-only
// code samples don't show an empty `  ` chip.
func TestRender_FencedCodeBlock_NoLanguageNoLabel(t *testing.T) {
	t.Parallel()
	src := "```\nbare\n```\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "  go ") || strings.Contains(plain, "mylang") {
		t.Errorf("unexpected language chip on label-less fence:\n%s", plain)
	}
	if !strings.Contains(plain, "bare") {
		t.Errorf("body missing:\n%s", plain)
	}
}

// TestRender_IndentedCodeBlock_RendersAsPanel: a 4-space-indented
// block uses the same panel as a fenced block — without a language
// label, since indented blocks have no info string.
func TestRender_IndentedCodeBlock_RendersAsPanel(t *testing.T) {
	t.Parallel()
	src := "    indented line one\n    indented line two\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"indented line one", "indented line two"} {
		if !strings.Contains(plain, want) {
			t.Errorf("indented block lost %q:\n%s", want, plain)
		}
	}
	rows := codePanelRows(out)
	if len(rows) < 4 {
		t.Errorf("expected >=4 rows (top band + 2 content + bottom band), got %d", len(rows))
	}
}

// TestRender_FencedCodeBlock_TerraformHighlights: terraform-aliased
// lexer paints HCL keywords. We don't pin the exact colour SGR — we
// assert that the output contains *some* SGR around the resource
// keyword so the lexer was actually consulted (vs. falling through
// to plain).
func TestRender_FencedCodeBlock_TerraformHighlights(t *testing.T) {
	t.Parallel()
	src := "```terraform\nresource \"null_resource\" \"x\" {}\n```\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("expected SGR sequences in syntax-highlighted output")
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"resource", "null_resource", " terraform "} {
		if !strings.Contains(plain, want) {
			t.Errorf("terraform fence missing %q:\n%s", want, plain)
		}
	}
}

// TestChromaTokenColor_CoversCommonCategories drives the token →
// colour mapper directly. Goldmark + chroma round-trips don't reach
// every branch (a single fixture rarely triggers e.g. NameDecorator
// + Generic + Punctuation in one go), so this test pins the per-
// category contracts without growing the integration fixtures.
func TestChromaTokenColor_CoversCommonCategories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		tok      chroma.TokenType
		nonEmpty bool
	}{
		{"KeywordType", chroma.KeywordType, true},
		{"NameFunction", chroma.NameFunction, true},
		{"NameDecorator", chroma.NameDecorator, true},
		{"NameAttribute", chroma.NameAttribute, true},
		{"NameConstant", chroma.NameConstant, true},
		{"NameBuiltin", chroma.NameBuiltin, true},
		{"NameClass", chroma.NameClass, true},
		{"NameVariable", chroma.NameVariable, true},
		{"NameTag", chroma.NameTag, true},
		{"NameNamespace", chroma.NameNamespace, true},
		{"Comment", chroma.Comment, true},
		{"CommentSingle", chroma.CommentSingle, true},
		{"Keyword", chroma.Keyword, true},
		{"KeywordReserved", chroma.KeywordReserved, true},
		{"LiteralString", chroma.LiteralString, true},
		{"LiteralStringDouble", chroma.LiteralStringDouble, true},
		{"LiteralNumber", chroma.LiteralNumber, true},
		{"LiteralNumberInteger", chroma.LiteralNumberInteger, true},
		{"Operator", chroma.Operator, true},
		{"Punctuation", chroma.Punctuation, true},
		{"GenericInserted", chroma.GenericInserted, true},
		{"Background", chroma.Background, false},
	}
	p := canonical.TokyonightNight
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chromaTokenColor(tc.tok, p)
			if tc.nonEmpty && got == "" {
				t.Errorf("expected non-empty colour for %s", tc.name)
			}
			if !tc.nonEmpty && got != "" {
				t.Errorf("expected empty colour for %s, got %q", tc.name, got)
			}
		})
	}
}

// codePanelRows returns the slice of consecutive lines in out that
// look like code-panel rows (carry a SGR sequence and aren't empty).
// Used by the panel-shape assertions to scope width / count checks
// to the panel and ignore surrounding prose.
func codePanelRows(out string) []string {
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, "\x1b[") && lipgloss.Width(line) > 0 {
			rows = append(rows, line)
		}
	}
	return rows
}
