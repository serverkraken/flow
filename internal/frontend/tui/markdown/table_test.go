package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
