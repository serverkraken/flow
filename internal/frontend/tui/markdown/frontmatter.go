// Frontmatter card renderer. Produces a compact two-row header that
// summarises the note's frontmatter in chips: type badge + title + date
// on row 1; project chip + tag chips on row 2 (when set); a thin HR
// underneath. Rendered above the body when WithFrontmatter is passed.

package markdown

import (
	"hash/fnv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// renderFrontmatterCard returns the styled card for fm sized to width.
// Empty fm (zero-value) returns "" so the caller can omit the card
// without branching. Width <= 0 returns "" too.
func (r *nodeRenderer) renderFrontmatterCard(fm *Frontmatter) string {
	if fm == nil || r.width <= 0 || fm.IsEmpty() {
		return ""
	}
	row1 := r.cardRowOne(fm)
	row2 := r.cardRowTwo(fm)
	sep := r.roles.CardSeparator.Render(strings.Repeat("─", r.width))
	parts := []string{row1}
	if row2 != "" {
		parts = append(parts, row2)
	}
	parts = append(parts, sep)
	return strings.Join(parts, "\n") + "\n"
}

// cardRowOne builds the title row: type-badge + title (or ID if no
// title) on the left, ISO date right-aligned. The date and badge are
// optional — both can be missing without breaking the row layout.
func (r *nodeRenderer) cardRowOne(fm *Frontmatter) string {
	left := r.cardBadge(fm.Type)
	title := fm.Title
	if title == "" {
		title = fm.ID
	}
	if title != "" {
		if left != "" {
			left += " "
		}
		left += r.roles.CardTitle.Render(title)
	}
	right := ""
	if fm.Date != "" {
		right = r.roles.CardMeta.Render(fm.Date)
	}
	return joinRow(left, right, r.width)
}

// cardRowTwo builds the project + tags row. Returns "" when neither
// project nor tags are set, so cardRowOne stands alone in that case.
func (r *nodeRenderer) cardRowTwo(fm *Frontmatter) string {
	if fm.Project == "" && len(fm.Tags) == 0 {
		return ""
	}
	var parts []string
	if fm.Project != "" {
		parts = append(parts, r.roles.CardProjectChip.Render(" "+fm.Project+" "))
	}
	for _, tag := range fm.Tags {
		parts = append(parts, r.tagChip(tag))
	}
	return strings.Join(parts, " ")
}

// cardBadge picks the badge style for the note's type. AnyType /
// unknown falls back to a neutral chip in CardMeta colour so rows
// stay structured.
func (r *nodeRenderer) cardBadge(t NoteType) string {
	switch t {
	case TypeDaily:
		return r.roles.CardBadgeDaily.Render(strings.ToUpper(string(t)))
	case TypeProject:
		return r.roles.CardBadgeProject.Render(strings.ToUpper(string(t)))
	case TypeFree:
		return r.roles.CardBadgeFree.Render(strings.ToUpper(string(t)))
	}
	return ""
}

// tagChip renders one tag with a deterministic palette colour so
// `#go` always reads as the same colour across notes (matches the
// browse-list tag chip rotation in browse/styles.go).
//
// Geht über die vorgefertigten r.roles.TagChips, damit der Style durch
// denselben lipgloss.Renderer läuft wie der Rest und im NO_COLOR-Profil
// nicht die Hintergrundfarbe verloren geht.
func (r *nodeRenderer) tagChip(tag string) string {
	chips := r.roles.TagChips
	if len(chips) == 0 {
		return tag
	}
	idx := tagColorIdx(tag, len(chips))
	return chips[idx].Render(tag)
}

// tagColorIdx is a tiny FNV-1a hash mod paletteLen. Same shape as the
// helper in browse/styles.go so a tag picks the SAME palette slot in
// both surfaces — visual consistency for the user.
func tagColorIdx(tag string, paletteLen int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(tag))
	return int(h.Sum32() % uint32(paletteLen))
}

// joinRow places left at column 0 and right flush against width
// using a calculated gap of plain spaces. Falls back to left when
// the two together would overflow.
func joinRow(left, right string, width int) string {
	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	if leftW+rightW+1 > width {
		return left
	}
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
