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
	for _, ref := range refs {
		b.WriteString(r.roles.Bullet1.Render("●"))
		b.WriteString(" ")
		b.WriteString(r.backlinkLine(ref))
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
		return r.roles.WikilinkBroken.Render("⌧ " + display)
	}
	r.osc8ID++
	return osc8Wrap(uri, r.osc8ID, r.roles.WikilinkValid.Render("⇲ "+display))
}
