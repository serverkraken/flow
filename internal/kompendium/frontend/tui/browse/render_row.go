package browse

// Listen-Row-Render-Helper. Split aus model.go (Skill §No-Monoliths):
// Row-Komposition (Stripe + Caret + Date + Badge + Title + Excerpt +
// Tags), Soft-Wrap der Excerpts, Heatmap-Glyph, Style-Auswahl je nach
// Selected-Status, Match-Highlight, Today-Marker. Plus die Listen-
// Window-Berechnung (computeListWindow), die das sichtbare Stück um
// den Cursor herum innerhalb des Höhen-Budgets aufbaut.

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func (m Model) renderListRows(rowWidth int) string {
	rendered := make([]string, len(m.visible))
	heights := make([]int, len(m.visible))
	for i := range m.visible {
		rendered[i] = m.renderRow(i, m.visible[i], rowWidth)
		heights[i] = lipgloss.Height(rendered[i])
	}
	budget := m.listRows()
	if budget < 1 {
		budget = 1
	}
	start, end := computeListWindow(heights, m.cursor, budget)

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, rendered[i])
	}
	return strings.Join(rows, "\n")
}

// computeListWindow picks the [start, end) slice of entries to render so
// the cursor row is included and the total stacked height stays within
// budget. It grows forward from the cursor first, then backfills upward,
// so the cursor stays anchored at the top half of the view — predictable
// behavior when scrolling through a long list.
func computeListWindow(heights []int, cursor, budget int) (int, int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}
	used := heights[cursor]
	end := cursor + 1
	for end < n && used+heights[end] <= budget {
		used += heights[end]
		end++
	}
	start := cursor
	for start > 0 && used+heights[start-1] <= budget {
		start--
		used += heights[start]
	}
	return start, end
}

func (m Model) renderRow(idx int, e ports.NoteEntry, rowWidth int) string {
	selected := idx == m.cursor
	stripe, caret := rowStripeAndCaret(selected)
	dateRendered, todayMark := renderDateCell(e)
	badge := badgeFor(e.Meta.Type)
	title := m.titleForRow(e, stripe, caret, todayMark, dateRendered, badge, rowWidth, selected)

	mainLine := lipgloss.JoinHorizontal(lipgloss.Top,
		stripe, caret, todayMark, dateRendered, "  ", badge, "  ", title)

	hang := rowHangPrefix(selected)
	excerptLines := m.excerptParagraph(e, rowWidth-lipgloss.Width(hang)-1, 2)
	tagLine := m.renderTags(e.Meta.Tags)
	if len(excerptLines) == 0 && tagLine == "" {
		return mainLine
	}

	q := strings.ToLower(m.search.Value())
	lines := []string{mainLine}
	for _, el := range excerptLines {
		lines = append(lines, hang+styleExcerptLine(el, q))
	}
	if tagLine != "" {
		lines = append(lines, hang+tagLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// rowStripeAndCaret returns the two two-cell prefix columns at the row's
// left edge: a vertical stripe and a cursor caret. Both stay constant
// width so excerpt + tag lines hang under the title cleanly, and the
// test's `▶`-on-the-line check still holds.
func rowStripeAndCaret(selected bool) (string, string) {
	if selected {
		return cursorStripeStyle.Render(glyphs.AccentBar + " "), cursorStyle.Render(glyphs.Active + " ")
	}
	return "  ", "  "
}

// renderDateCell formats the row's date column and returns the colored
// date plus the today-marker glyph (● when today, blank otherwise).
func renderDateCell(e ports.NoteEntry) (string, string) {
	date := e.Meta.Date
	if date == "" {
		date = e.Mtime.Format("2006-01-02")
	}
	today := isToday(date)
	dateRendered := dateStyle.Render(date)
	if today {
		dateRendered = todayDateStyle.Render(date)
	}
	if today {
		// glyphs.Filled (●) ist der kanonische Today-Marker laut
		// Whitelist; ★ ist semantisch Holiday und wäre ein Drift.
		return dateRendered, todayMarkerStyle.Render(glyphs.Filled + " ")
	}
	return dateRendered, "  "
}

// titleForRow truncates the title to whatever fits in the row and renders
// it with the right base style (selected rows get bold), running the
// search-match highlight on top.
func (m Model) titleForRow(e ports.NoteEntry, stripe, caret, todayMark, dateRendered, badge string, rowWidth int, selected bool) string {
	title := smartTitle(e)
	if rowWidth > 0 {
		prefixW := lipgloss.Width(stripe) + lipgloss.Width(caret) + lipgloss.Width(todayMark) +
			lipgloss.Width(dateRendered) + lipgloss.Width(badge) + 4
		avail := rowWidth - prefixW - 1
		if avail < 8 {
			avail = 8
		}
		if lipgloss.Width(title) > avail {
			title = truncateText(title, avail)
		}
	}
	base := titleStyle
	if selected {
		base = selectedTitleStyle
	}
	return highlightMatch(title, strings.ToLower(m.search.Value()), base, matchStyle)
}

// rowHangPrefix is the soft indent under the title used for excerpt and
// tag lines. Selected rows get the stripe so the whole entry reads as
// one block; non-selected rows just indent.
func rowHangPrefix(selected bool) string {
	if selected {
		return cursorStripeStyle.Render(glyphs.AccentBar+" ") + "      "
	}
	return "        "
}

// styleExcerptLine renders one wrapped excerpt line with the muted
// excerpt style, layering the search-match highlight on top when the
// user has a query.
func styleExcerptLine(line, q string) string {
	if q == "" {
		return excerptStyle.Render(line)
	}
	return highlightMatch(line, q, excerptStyle, matchStyle)
}

// excerptParagraph builds a multi-line, soft-wrapped excerpt for the
// row. It collects up to ~220 chars of meaningful body lines (skipping
// the same redundant patterns excerptFor does), joins them into one
// paragraph, then word-wraps to width with maxLines as the cap. When
// width is too small to soft-wrap (e.g. tests that never sent a
// WindowSizeMsg), it falls back to the single-line excerptFor result so
// existing callers keep getting something useful.
func (m Model) excerptParagraph(e ports.NoteEntry, width, maxLines int) []string {
	if width < 8 || maxLines < 1 {
		if line := m.excerptFor(e); line != "" {
			return []string{line}
		}
		return nil
	}
	body, ok := m.bodies[e.ID]
	if !ok {
		return nil
	}
	var collected []string
	total := 0
	for _, line := range strings.Split(string(body), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		clean = strings.TrimLeft(clean, "#- *>`")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		if isDateString(clean) || clean == e.Meta.Date || clean == e.Meta.Project {
			continue
		}
		collected = append(collected, clean)
		total += len(clean) + 1
		if total >= 220 {
			break
		}
	}
	if len(collected) == 0 {
		return nil
	}
	return softWrap(strings.Join(collected, " "), width, maxLines)
}

// softWrap word-wraps s to width cells, capped at maxLines. Lines past
// the cap are dropped and the last kept line gets a "…" suffix. Words
// longer than width get hard-truncated. Empty input returns nil.
func softWrap(s string, width, maxLines int) []string {
	if width < 8 || maxLines < 1 {
		return nil
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range words {
		next, full := wrapAppend(cur, w, width)
		if !full {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
			if len(lines) >= maxLines {
				lines[maxLines-1] = appendEllipsis(lines[maxLines-1], width)
				return lines
			}
		}
		cur = wrapStart(w, width)
	}
	return finishWrap(lines, cur, width, maxLines)
}

// wrapAppend tries to append w to cur with a space separator. Returns
// (newLine, false) on success or (cur, true) when the result would
// exceed width.
func wrapAppend(cur, w string, width int) (string, bool) {
	candidate := w
	if cur != "" {
		candidate = cur + " " + w
	}
	if lipgloss.Width(candidate) <= width {
		return candidate, false
	}
	return cur, true
}

// wrapStart starts a new wrapped line with w, hard-truncating if w
// alone exceeds width.
func wrapStart(w string, width int) string {
	if lipgloss.Width(w) > width {
		return truncateText(w, width)
	}
	return w
}

// finishWrap flushes the remaining cur into lines and applies the
// ellipsis-on-overflow rule.
func finishWrap(lines []string, cur string, width, maxLines int) []string {
	if cur == "" {
		return lines
	}
	if len(lines) < maxLines {
		return append(lines, cur)
	}
	lines[maxLines-1] = appendEllipsis(lines[maxLines-1], width)
	return lines
}

func appendEllipsis(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return truncateText(s, width)
	}
	return s + "…"
}

// smartTitle picks a human-readable label for the row when the user
// hasn't set a frontmatter Title. Daily notes fall back to the raw ID
// (the date column already shows the date, but tests assert on the ID
// substring being present, and dropping the ID entirely would lose
// signal in older notebooks). Project notes get the canonical URL's
// last two segments — turning a 50-char `projects/github.com/.../date`
// into a clean `serverkraken/dotfiles`.
func smartTitle(e ports.NoteEntry) string {
	if e.Meta.Title != "" {
		return e.Meta.Title
	}
	if e.Meta.Type == domain.TypeProject && e.Meta.Project != "" {
		return shortProjectLabel(e.Meta.Project)
	}
	return e.ID.String()
}

func shortProjectLabel(canonicalURL string) string {
	parts := strings.Split(canonicalURL, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return canonicalURL
}

// truncateText clips s to at most `max` cells, appending an ellipsis. It
// is intentionally byte-naive — note titles are predominantly ASCII or
// latin-1 in practice, and a stricter rune-aware version would be
// overkill until a real-world note proves it wrong.
func truncateText(s string, maxCells int) string {
	if maxCells <= 1 {
		return "…"
	}
	if lipgloss.Width(s) <= maxCells {
		return s
	}
	if len(s) > maxCells-1 {
		return s[:maxCells-1] + "…"
	}
	return s + "…"
}

func (m Model) excerptFor(e ports.NoteEntry) string {
	body, ok := m.bodies[e.ID]
	if !ok {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		// Skip leading markdown clutter like "# heading" so the excerpt
		// reads as prose.
		clean = strings.TrimLeft(clean, "#- *>`")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		// Skip lines that are uninformative duplicates of the row's other
		// columns: a bare YYYY-MM-DD (often the project-template's only
		// body line), the row's own date, the canonical project URL.
		if isDateString(clean) || clean == e.Meta.Date || clean == e.Meta.Project {
			continue
		}
		if len(clean) > 80 {
			clean = clean[:79] + "…"
		}
		return clean
	}
	return ""
}

func isDateString(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i, c := range s {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func (m Model) renderTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var parts []string
	for _, t := range tags {
		parts = append(parts, tagChipStyle(t).Render(t))
	}
	return strings.Join(parts, " ")
}

func (m Model) renderEmptyState(width int) string {
	glyph := emptyGlyphStyle.Render("✺")
	title := emptyTitleStyle.Render("keine Treffer")
	newKey := keyLabel(m.keys.New)
	searchKey := keyLabel(m.keys.Search)
	filterKey := keyLabel(m.keys.Filter)
	hint := footerKeyStyle.Render(newKey) +
		emptyHintStyle.Render(" → neue Notiz anlegen")
	tail := footerKeyStyle.Render(filterKey) +
		emptyHintStyle.Render(" → Filter wechseln · ") + footerKeyStyle.Render(searchKey) +
		emptyHintStyle.Render(" → Suche zurücksetzen")
	stack := lipgloss.JoinVertical(lipgloss.Center, glyph, "", title, hint, tail)
	if width <= 0 {
		return stack
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, stack)
}

// keyLabel returns the first canonical label for a binding (e.g. "n", "/").
func keyLabel(b key.Binding) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func (m Model) renderPreviewPane() string {
	paneW := m.previewPaneWidth()
	if paneW <= 0 {
		return ""
	}
	titleLine := panelTitleStyle.Render("vorschau")
	body := m.preview.View()
	if body == "" {
		body = dimStyle.Render("(leer)")
	}
	inner := lipgloss.JoinVertical(lipgloss.Left, titleLine, body)
	// lipgloss Style.Width sets the *content* width; the NormalBorder
	// adds 1 cell on each side on top of that. Pass paneW-2 so the
	// rendered pane is exactly paneW wide overall — otherwise the body
	// row gets two cells too wide and lipgloss wraps every line in
	// half, doubling the visible height of a long Markdown preview.
	innerW := paneW - 2
	if innerW < 1 {
		innerW = 1
	}
	return panelStyle.Width(innerW).Render(inner)
}

// badgeFor returns the colored type pill shown in the list row.
// Skill §German UI: Badges sind user-facing — DE-Kurzformen statt EN.
// Width-Padding auf einheitliche 5 Zellen, damit die Type-Pille die
// Listen-Spalten nicht ungleichmäßig schiebt.
func badgeFor(t domain.NoteType) string {
	switch t {
	case domain.TypeDaily:
		return badgeDailyStyle.Render("TÄGL.")
	case domain.TypeProject:
		return badgeProjectStyle.Render("PROJ.")
	case domain.TypeFree:
		return badgeFreeStyle.Render("FREI ")
	}
	return badgeUnknownStyle.Render("  ?  ")
}

// highlightMatch wraps the substring `q` (case-insensitive) in `match`
// styling, leaving the surrounding text in `base`. q == "" returns the
// base-styled text untouched.
func highlightMatch(text, q string, base, match lipgloss.Style) string {
	if q == "" {
		return base.Render(text)
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, q)
	if idx < 0 {
		return base.Render(text)
	}
	end := idx + len(q)
	return base.Render(text[:idx]) + match.Render(text[idx:end]) + base.Render(text[end:])
}

// isToday reports whether date is today's YYYY-MM-DD. The clock here is
// the system clock — the browse view is a read-only surface, so the use
// case's testable Clock isn't threaded through.
func isToday(date string) bool {
	if date == "" {
		return false
	}
	return time.Now().UTC().Format("2006-01-02") == date
}
