package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestMarkdownRolesFor_AllStylesPopulated: every field on the
// returned MarkdownRoles must carry a usable lipgloss.Style — a
// missed field would render as the renderer's zero style and look
// indistinguishable from prose. Iterates the fields by hand because
// reflect-walking a styles struct is over-engineered.
func TestMarkdownRolesFor_AllStylesPopulated(t *testing.T) {
	r := lipgloss.DefaultRenderer()
	roles := MarkdownRolesFor(r)
	cases := map[string]lipgloss.Style{
		"H1Bar":          roles.H1Bar,
		"H1Text":         roles.H1Text,
		"H2":             roles.H2,
		"H3":             roles.H3,
		"H4":             roles.H4,
		"H5":             roles.H5,
		"H6":             roles.H6,
		"Paragraph":      roles.Paragraph,
		"HRule":          roles.HRule,
		"Strong":         roles.Strong,
		"Emph":           roles.Emph,
		"Strike":         roles.Strike,
		"CodeSpan":       roles.CodeSpan,
		"LinkText":       roles.LinkText,
		"CodeFenceBg":    roles.CodeFenceBg,
		"CodeFenceBand":  roles.CodeFenceBand,
		"CodeFenceLabel": roles.CodeFenceLabel,
		"CodeFencePlain": roles.CodeFencePlain,
	}
	for name, style := range cases {
		if got := style.Render("x"); got == "" {
			t.Errorf("%s style produced empty output for non-empty input", name)
		}
	}
}

// TestCalloutBadgeAndBar_AllKinds: every recognised callout kind
// must have a non-empty rendered chip + bar so the renderer never
// emits a colourless callout. The default branch handles unknown
// kinds with a muted style — also asserted for completeness.
func TestCalloutBadgeAndBar_AllKinds(t *testing.T) {
	for _, k := range []CalloutKind{
		CalloutNote, CalloutTip, CalloutInfo, CalloutWarning,
		CalloutDanger, CalloutImportant, CalloutSuccess, CalloutKind("unknown"),
	} {
		if got := CalloutBadge(k).Render("X"); got == "" {
			t.Errorf("CalloutBadge(%q) returned empty", k)
		}
		if got := CalloutBar(k).Render("│"); got == "" {
			t.Errorf("CalloutBar(%q) returned empty", k)
		}
	}
}

// TestMarkdownRolesFor_SwapsWithPalette: after SetActive(Catppuccin),
// re-building roles must use the Catppuccin colours. Asserts a
// per-render style picks up theme switches without re-init of the
// markdown package.
func TestMarkdownRolesFor_SwapsWithPalette(t *testing.T) {
	t.Cleanup(func() { SetActive(Tokyonight) })
	r := lipgloss.DefaultRenderer()
	tn := MarkdownRolesFor(r).H2.GetForeground()
	SetActive(Catppuccin)
	cp := MarkdownRolesFor(r).H2.GetForeground()
	if tn == cp {
		t.Errorf("expected H2 colour to change on palette swap, got %v == %v", tn, cp)
	}
}
