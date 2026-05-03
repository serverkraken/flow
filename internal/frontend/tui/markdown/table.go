// GFM table renderer. Walks the table once to compute per-column
// widths, then emits a box-drawing frame: top border, header row,
// header/body separator, body rows (with alternating tint), bottom
// border. Per-column alignment honours `:---`, `:---:`, `---:` from
// the markdown source.

package markdown

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// tableCellPad is the per-cell horizontal breathing room (one space
// each side) added on top of the cell's content width.
const tableCellPad = 2

// renderTable lays out a GFM table as a box-drawing panel and writes
// it to the buffer. Always returns WalkSkipChildren — children are
// walked manually so column widths can be computed before any cell
// is emitted.
func (r *nodeRenderer) renderTable(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	tbl, ok := n.(*extast.Table)
	if !ok {
		return ast.WalkContinue, nil
	}
	header, rows, err := r.collectTableRows(source, tbl)
	if err != nil {
		return ast.WalkStop, err
	}
	if header == nil && len(rows) == 0 {
		return ast.WalkSkipChildren, nil
	}
	cols := tableColumnCount(header, rows)
	widths := r.tableColumnWidths(header, rows, cols)
	out := r.renderTableFrame(header, rows, widths, tbl.Alignments)
	_, _ = w.WriteString("\n" + out + "\n\n")
	return ast.WalkSkipChildren, nil
}

// tableCell carries one cell's already-rendered inline content plus
// the cell-level alignment override goldmark records (the alignment
// is normally homogeneous per column, but the per-cell field is the
// authoritative source).
type tableCell struct {
	content string
	align   extast.Alignment
}

// collectTableRows walks the Table's children and returns the
// (header, body rows) pair with each cell's inline content already
// rendered. Tables emitted by goldmark always have one TableHeader
// (even when the markdown lacks an explicit header — GFM requires
// it).
func (r *nodeRenderer) collectTableRows(source []byte, tbl *extast.Table) ([]tableCell, [][]tableCell, error) {
	var (
		header []tableCell
		rows   [][]tableCell
	)
	for c := tbl.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *extast.TableHeader:
			cells, err := r.collectRowCells(source, row)
			if err != nil {
				return nil, nil, err
			}
			header = cells
		case *extast.TableRow:
			cells, err := r.collectRowCells(source, row)
			if err != nil {
				return nil, nil, err
			}
			rows = append(rows, cells)
		}
	}
	return header, rows, nil
}

// collectRowCells renders each TableCell child to its inline ANSI
// string. Returns the slice of (content, alignment) pairs in source
// order.
func (r *nodeRenderer) collectRowCells(source []byte, row ast.Node) ([]tableCell, error) {
	var cells []tableCell
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cell, ok := c.(*extast.TableCell)
		if !ok {
			continue
		}
		content, err := r.renderInlineToString(source, cell)
		if err != nil {
			return nil, err
		}
		cells = append(cells, tableCell{content: strings.TrimSpace(content), align: cell.Alignment})
	}
	return cells, nil
}

// tableColumnCount returns the maximum cell-count across header and
// body rows. Defensive — well-formed GFM tables already pad shorter
// rows, but goldmark's table parser doesn't enforce that universally.
func tableColumnCount(header []tableCell, rows [][]tableCell) int {
	cols := len(header)
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	return cols
}

// tableColumnWidths picks a width for each column. Starts from each
// column's natural max-content width plus tableCellPad. If the
// resulting table would exceed r.width, the budget is distributed
// proportionally so the wider columns shrink first.
func (r *nodeRenderer) tableColumnWidths(header []tableCell, rows [][]tableCell, cols int) []int {
	natural := make([]int, cols)
	measure := func(cells []tableCell) {
		for i, c := range cells {
			if i >= cols {
				continue
			}
			if w := visibleWidth(c.content) + tableCellPad; w > natural[i] {
				natural[i] = w
			}
		}
	}
	measure(header)
	for _, row := range rows {
		measure(row)
	}
	for i, w := range natural {
		if w < tableCellPad+1 {
			natural[i] = tableCellPad + 1
		}
	}
	// Frame: 1 leading │ + 1 trailing │ + (cols-1) inner │ separators.
	frame := cols + 1
	budget := r.effectiveWidth() - frame
	if budget < cols {
		budget = cols
	}
	total := 0
	for _, w := range natural {
		total += w
	}
	if total <= budget {
		return natural
	}
	return shrinkColumns(natural, budget)
}

// shrinkColumns proportionally reduces a slice of column widths so
// the sum equals budget. Each column keeps a minimum of (pad + 1)
// cells so a chopped cell still has room for `…`.
func shrinkColumns(widths []int, budget int) []int {
	out := make([]int, len(widths))
	copy(out, widths)
	for {
		total := 0
		for _, w := range out {
			total += w
		}
		if total <= budget {
			return out
		}
		// Drop one cell from the widest column.
		max, idx := 0, -1
		for i, w := range out {
			if w > max {
				max = w
				idx = i
			}
		}
		if idx < 0 || out[idx] <= tableCellPad+1 {
			return out
		}
		out[idx]--
	}
}

// renderTableFrame stitches header + body rows together with box-
// drawing borders. Layout:
//
//	┌─────┬─────┐
//	│ Hdr │ Hdr │
//	├─────┼─────┤
//	│ ... │ ... │
//	└─────┴─────┘
//
// Empty header is hidden — GFM technically requires a header but
// renderers in the wild are lenient, so we mirror that.
func (r *nodeRenderer) renderTableFrame(header []tableCell, rows [][]tableCell, widths []int, aligns []extast.Alignment) string {
	border := r.roles.TableBorder
	var b strings.Builder
	b.WriteString(border.Render(tableBorderRow("┌", "┬", "┐", widths)))
	if len(header) > 0 {
		b.WriteString("\n")
		b.WriteString(r.renderTableRow(header, widths, aligns, true, false))
		b.WriteString("\n")
		b.WriteString(border.Render(tableBorderRow("├", "┼", "┤", widths)))
	}
	for i, row := range rows {
		b.WriteString("\n")
		alt := i%2 == 1
		b.WriteString(r.renderTableRow(row, widths, aligns, false, alt))
	}
	b.WriteString("\n")
	b.WriteString(border.Render(tableBorderRow("└", "┴", "┘", widths)))
	return b.String()
}

// tableBorderRow builds a single border row from the corner glyphs:
// `left`, between-columns `mid`, `right` — joined by `─` filled to
// each column's width.
func tableBorderRow(left, mid, right string, widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("─", w)
	}
	return left + strings.Join(parts, mid) + right
}

// renderTableRow emits one content row: leading │, each cell padded
// to column width with alignment honoured, separated by │, trailing
// │. Multi-line cells (content wider than the column) emit multiple
// physical lines, each carrying the full border + per-cell row tint
// so the row reads as one visual block. Header rows wear TableHeader
// styling; body rows wear TableCell (with alternating-row tint when
// alt is true).
func (r *nodeRenderer) renderTableRow(cells []tableCell, widths []int, aligns []extast.Alignment, header, alt bool) string {
	border := r.roles.TableBorder
	cellStyle := r.roles.TableCell
	if header {
		cellStyle = r.roles.TableHeader
	} else if alt {
		cellStyle = r.roles.TableRowAlt
	}

	// Wrap each cell to its column width and find the row's height
	// (== max cell line count). Empty cells still produce one line of
	// pure padding so the border alignment stays intact.
	cellLines := make([][]string, len(widths))
	height := 1
	for i, w := range widths {
		var content string
		var align extast.Alignment
		if i < len(cells) {
			content = cells[i].content
			align = cells[i].align
		}
		if align == extast.AlignNone && i < len(aligns) {
			align = aligns[i]
		}
		cellLines[i] = formatTableCellLines(content, w, align)
		if n := len(cellLines[i]); n > height {
			height = n
		}
	}

	// Pad shorter cells with empty padded lines so every line of the
	// stitched row has the same number of columns.
	for i := range cellLines {
		for len(cellLines[i]) < height {
			cellLines[i] = append(cellLines[i], emptyCellLine(widths[i]))
		}
	}

	// Stitch line-by-line: │ + cell + │ + cell + │ … repeated `height`
	// times. The whole stitched block is the row.
	var b strings.Builder
	for ln := 0; ln < height; ln++ {
		if ln > 0 {
			b.WriteString("\n")
		}
		b.WriteString(border.Render("│"))
		for i := range widths {
			b.WriteString(cellStyle.Render(cellLines[i][ln]))
			b.WriteString(border.Render("│"))
		}
	}
	return b.String()
}

// formatTableCellLines wraps content into one or more lines, each
// shaped to width cells (including the per-cell pad) and aligned per
// the column's alignment. Wrapping uses the package's style-aware
// cellbuf wrapper, so wrap boundaries inside an inline-code or link
// span re-open the SGR on the next line.
//
// Truncation only fires as a fallback for unbreakable tokens wider
// than `inner` (e.g. a 60-char URL in a 20-cell column when wrapText
// can't find a break point). The caller can't reasonably do better.
func formatTableCellLines(content string, width int, align extast.Alignment) []string {
	inner := width - tableCellPad
	if inner < 1 {
		inner = 1
	}
	wrapped := wrapText(content, inner)
	rawLines := strings.Split(wrapped, "\n")
	out := make([]string, len(rawLines))
	for i, line := range rawLines {
		if visibleWidth(line) > inner {
			line = ansi.Truncate(line, inner, "…")
		}
		pad := inner - visibleWidth(line)
		if pad < 0 {
			pad = 0
		}
		switch align {
		case extast.AlignRight:
			out[i] = " " + strings.Repeat(" ", pad) + line + " "
		case extast.AlignCenter:
			left := pad / 2
			right := pad - left
			out[i] = " " + strings.Repeat(" ", left) + line + strings.Repeat(" ", right) + " "
		default:
			out[i] = " " + line + strings.Repeat(" ", pad) + " "
		}
	}
	return out
}

// emptyCellLine returns the padding-only string used to fill the
// shorter cells when a row's other cells wrap onto more lines.
func emptyCellLine(width int) string {
	inner := width - tableCellPad
	if inner < 1 {
		inner = 1
	}
	return " " + strings.Repeat(" ", inner) + " "
}
