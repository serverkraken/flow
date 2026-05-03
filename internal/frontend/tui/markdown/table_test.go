package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

const minimalTable = "| Name | Status |\n|------|--------|\n| a    | ok     |\n| b    | fail   |\n"

// TestRender_Table_BoxDrawingFrame: a GFM table renders with full
// box-drawing border characters (top, header sep, bottom).
func TestRender_Table_BoxDrawingFrame(t *testing.T) {
	t.Parallel()
	out, err := Render(minimalTable, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, glyph := range []string{"┌", "┐", "└", "┘", "├", "┤", "┼", "┬", "┴", "│", "─"} {
		if !strings.Contains(plain, glyph) {
			t.Errorf("missing box-drawing glyph %q in table output:\n%s", glyph, plain)
		}
	}
}

// TestRender_Table_HeaderAndCellsPresent: header text + body cell
// content survive into the rendered output.
func TestRender_Table_HeaderAndCellsPresent(t *testing.T) {
	t.Parallel()
	out, err := Render(minimalTable, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"Name", "Status", "a", "b", "ok", "fail"} {
		if !strings.Contains(plain, want) {
			t.Errorf("table cell content %q missing:\n%s", want, plain)
		}
	}
}

// TestRender_Table_Alignment: `:--`, `:-:`, `--:` mark the columns as
// left/center/right aligned. We assert by counting padding spaces
// around the rendered content for each column.
func TestRender_Table_Alignment(t *testing.T) {
	t.Parallel()
	src := "| L | C | R |\n|:--|:-:|--:|\n| x | x | x |\n"
	out, err := Render(src, 40)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	// Find a row containing all three x's. Wider columns mean the
	// alignment shows: left = leading single space, right = trailing
	// single space, center = balanced padding.
	bodyLine := ""
	for _, line := range strings.Split(plain, "\n") {
		if strings.Count(line, "x") == 3 {
			bodyLine = line
			break
		}
	}
	if bodyLine == "" {
		t.Fatalf("no body row with 3 x's:\n%s", plain)
	}
	cells := strings.Split(bodyLine, "│")
	if len(cells) < 4 {
		t.Fatalf("expected at least 4 split parts (border|L|C|R|border), got %d: %q", len(cells), cells)
	}
	// cells[1]=L, cells[2]=C, cells[3]=R (cells[0] is leading
	// nothing, cells[4] trailing nothing).
	if !strings.HasPrefix(cells[1], " x") {
		t.Errorf("L column not left-aligned: %q", cells[1])
	}
	if !strings.HasSuffix(cells[3], "x ") {
		t.Errorf("R column not right-aligned: %q", cells[3])
	}
	// Centre: trim outer pad, expect roughly balanced inner whitespace.
	c := cells[2]
	if !strings.HasPrefix(c, " ") || !strings.HasSuffix(c, " ") {
		t.Errorf("C column missing outer pad: %q", c)
	}
}

// TestRender_Table_AllRowsShareWidth: every row of the rendered
// table (border + header + body) has the same visible width — a
// staggered table reads as broken.
func TestRender_Table_AllRowsShareWidth(t *testing.T) {
	t.Parallel()
	out, err := Render(minimalTable, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	rows := tableRows(out)
	if len(rows) < 5 {
		t.Fatalf("expected >=5 rows (top+hdr+sep+2 body+bottom): %v", rows)
	}
	want := lipgloss.Width(rows[0])
	for i, row := range rows {
		if w := lipgloss.Width(row); w != want {
			t.Errorf("row %d width = %d, want %d", i, w, want)
		}
	}
}

// TestRender_Table_WrapsLongCellsAcrossLines: a table whose natural
// width would overflow the budget shrinks the columns AND wraps the
// over-budget cell content vertically instead of truncating it. The
// truncation regression (only `…` shown for long Decisions/Notes
// cells, content lost off the right edge) is what this guards.
func TestRender_Table_WrapsLongCellsAcrossLines(t *testing.T) {
	t.Parallel()
	src := "| Decision | Notes |\n|---|---|\n" +
		"| tui-kit migrates into internal/frontend/tui/components | Eliminates the sibling replace directive — no more cross-module bouncing |\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	// Negative regression: the old truncation behaviour ended each
	// over-budget cell with `…`. Wrapping must NOT emit any `…` marker
	// for content that fits across multiple physical lines.
	if strings.Contains(plain, "…") {
		t.Errorf("wrap path must not surface `…` truncation markers; got:\n%s", plain)
	}
	// Positive regression: tokens at the START of each cell still
	// survive (they don't depend on wrap-point luck — wrap only ever
	// affects later text on a long line).
	for _, want := range []string{"tui-kit migrates into", "Eliminates the sibling"} {
		if !strings.Contains(plain, want) {
			t.Errorf("wrapped content should retain %q, got plain:\n%s", want, plain)
		}
	}
	// "bouncing" is the LAST token of the Notes cell — surviving the
	// wrap proves the second half of the content didn't fall off the
	// right edge as it did under the truncation regression.
	if !strings.Contains(plain, "bouncing") {
		t.Errorf("trailing word »bouncing« should survive the wrap, got plain:\n%s", plain)
	}
	// All physical lines of the framed table must share the same
	// width — a multi-line cell that breaks alignment reads broken.
	rows := tableRows(out)
	if len(rows) < 4 {
		t.Fatalf("expected >=4 rows (top + header + sep + ≥1 body wrapped), got %d:\n%s", len(rows), plain)
	}
	want := lipgloss.Width(rows[0])
	for i, row := range rows {
		if w := lipgloss.Width(row); w != want {
			t.Errorf("row %d width = %d, want %d", i, w, want)
		}
	}
}

// TestRender_Table_WrappedRowKeepsBackground: every physical line of
// a wrapped alt body row carries the alt-row background SGR — both
// the content lines AND the wrap-continuation / empty padding lines.
// Without this, the alt-row tint paints only the first line and the
// wrap continuations read as a transparent gap. Guards specifically
// against `cellStyle.Render` being skipped on continuation lines or
// against lipgloss-Render swallowing the bg on a multi-line input.
func TestRender_Table_WrappedRowKeepsBackground(t *testing.T) {
	// NO t.Parallel(): this test mutates the global lipgloss color
	// profile to force SGR emission. Other table tests rely on the
	// default Ascii profile (no SGRs) so they're racy with our flip.
	// Force truecolor so lipgloss doesn't strip SGR codes when run
	// outside a TTY (test runner has no terminal). Without this every
	// `;48;` would be elided by lipgloss's Ascii profile and the test
	// would silently always pass.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// Two body rows: first is row-index 0 (TableCell, no bg). Second
	// is row-index 1 (TableRowAlt, with bg). Both wrap because the
	// Decision column overflows; the Notes column has short content so
	// the empty-padding-line code path is exercised on the right cell.
	src := "| Decision | Notes |\n|---|---|\n" +
		"| this is the first body row with content that absolutely must wrap onto two lines | short |\n" +
		"| this is the second body row also wide enough to wrap onto two physical lines | tiny |\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	rows := tableRows(out)

	// Pull the alt body row's two physical lines: they're the two
	// rows that contain the bg SGR (`;48;`) — the first body row is
	// non-alt and has fg only.
	var altLines []string
	for _, row := range rows {
		if strings.Contains(row, ";48;") {
			altLines = append(altLines, row)
		}
	}
	if len(altLines) < 2 {
		t.Fatalf("expected ≥2 alt body lines (wrapped), got %d:\n%s",
			len(altLines), strings.Join(rows, "\n"))
	}
	// Every alt line — including the wrap continuation — must keep
	// the bg SGR. Pre-fix the second line lost it.
	for i, line := range altLines {
		if !strings.Contains(line, ";48;") {
			t.Errorf("alt-row physical line %d missing bg SGR (`;48;`):\n%q", i, line)
		}
	}
	// Stronger pin: every cell on each alt line must carry the bg.
	// The line stitches as │+cellA+│+cellB+│; cellA and cellB must
	// each contain the bg SGR. Test counts `;48;` per line and
	// expects ≥ number-of-cells (2 for this fixture).
	const wantCells = 2
	for i, line := range altLines {
		if got := strings.Count(line, ";48;"); got < wantCells {
			t.Errorf("alt-row physical line %d: %d bg-styled cells, want ≥%d:\n%q",
				i, got, wantCells, line)
		}
	}
}

// TestRender_Table_NeverExceedsWidth: a table whose natural width
// would overflow the budget gets shrunk so no row exceeds r.width.
func TestRender_Table_NeverExceedsWidth(t *testing.T) {
	t.Parallel()
	src := "| eine wirklich lange spalte | und noch eine ebenso lange |\n" +
		"|---|---|\n" +
		"| viel inhalt hier drin | nochmehr inhalt nochmehr |\n"
	const width = 40
	out, err := Render(src, width)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for i, row := range tableRows(out) {
		if w := lipgloss.Width(row); w > width {
			t.Errorf("row %d width = %d, exceeds budget %d:\n%s", i, w, width, row)
		}
	}
}

// TestRender_Table_AlternatingRowTint: every other body row carries
// the row-tint background SGR. We don't pin the colour value; we
// assert that two body rows produce different ANSI byte sequences,
// the alternation signal.
func TestRender_Table_AlternatingRowTint(t *testing.T) {
	t.Parallel()
	out, err := Render(minimalTable, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Find the two body rows by their content "a" and "b".
	var rowA, rowB string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(ansi.Strip(line), "│ a") {
			rowA = line
		}
		if strings.Contains(ansi.Strip(line), "│ b") {
			rowB = line
		}
	}
	if rowA == "" || rowB == "" {
		t.Fatalf("body rows not found: A=%q B=%q", rowA, rowB)
	}
	if rowA == rowB {
		t.Errorf("expected alternating tint: row A and row B render identically")
	}
}

// tableRows returns the consecutive lines that look like table rows
// (carry box-drawing chars).
func tableRows(out string) []string {
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		plain := ansi.Strip(line)
		if strings.ContainsAny(plain, "┌┐└┘├┤┼┬┴│─") {
			rows = append(rows, line)
		}
	}
	return rows
}
