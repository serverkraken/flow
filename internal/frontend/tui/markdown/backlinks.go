// Backlinks footer renderer. Appended below the body when
// WithBacklinks is passed and the slice is non-empty. Each backlink
// renders as a wikilink (so OSC 8 + valid/broken styling kicks in
// via the same resolver the body uses).

package markdown

import (
	"strings"
)

// renderBacklinksFooter returns the styled footer for refs sized to
// width. Empty refs returns "" so the caller can append it
// unconditionally. Width <= 0 returns "" too.
func (r *nodeRenderer) renderBacklinksFooter(refs []BacklinkRef) string {
	if len(refs) == 0 || r.width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(r.roles.CardSeparator.Render(strings.Repeat("─", r.width)))
	b.WriteString("\n\n")
	b.WriteString(r.roles.H3.Render("↩ Referenced by"))
	b.WriteString("\n\n")
	// Bullet "●" + space = 2 visible cells; eine schmal-aber-noch-lesbare
	// Floor-Breite verhindert, dass extrem schmale Viewer den Wrap auf
	// 0 zwingen (cellbuf.Wrap würde den String in Einzelzeichen splitten).
	bodyW := r.width - 2
	if bodyW < 8 {
		bodyW = 8
	}
	for _, ref := range refs {
		b.WriteString(r.roles.Bullet1.Render("●"))
		b.WriteString(" ")
		// Lange whitespace-freie Slugs (z. B. `2026-05-09-very-long-id`)
		// würden ohne explizites Wrap den Footer über die Viewport-Breite
		// hinausziehen und die OSC-8-Region mitziehen. wrapText respektiert
		// SGR-Spans, die backlinkLine emittiert.
		b.WriteString(wrapText(r.backlinkLine(ref), bodyW))
		b.WriteString("\n")
	}
	return b.String()
}

// backlinkLine renders one ref as a styled wikilink. Valid refs (the
// resolver knows the target) get an OSC 8 wrap; broken refs wear the
// WikilinkBroken style without a link. Title falls back to the ID
// when the target's frontmatter has no title.
func (r *nodeRenderer) backlinkLine(ref BacklinkRef) string {
	display := ref.Title
	if display == "" {
		display = ref.ID
	}
	uri, _, found := "", "", false
	if r.opts.resolver != nil {
		uri, _, found = r.opts.resolver.Resolve(ref.ID)
	}
	if !found {
		return r.roles.WikilinkBroken.Render("⊘ " + display)
	}
	r.osc8ID++
	return osc8Wrap(uri, r.osc8ID, r.roles.WikilinkValid.Render("→ "+display))
}
