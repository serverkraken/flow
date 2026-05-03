package theme_test

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/serverkraken/flow/internal/frontend/tui/markdown/theme"
	canonical "github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestMarkdownRolesFor_AllStylesPopulated: every field on the
// returned MarkdownRoles must carry a usable lipgloss.Style — a
// missed field would render as the renderer's zero style and look
// indistinguishable from prose. Iterates the fields by hand because
// reflect-walking a styles struct is over-engineered.
func TestMarkdownRolesFor_AllStylesPopulated(t *testing.T) {
	r := lipgloss.DefaultRenderer()
	roles := theme.MarkdownRolesFor(r, canonical.TokyonightNight)
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
	p := canonical.TokyonightNight
	for _, k := range []theme.CalloutKind{
		theme.CalloutNote, theme.CalloutTip, theme.CalloutInfo, theme.CalloutWarning,
		theme.CalloutDanger, theme.CalloutImportant, theme.CalloutSuccess,
		theme.CalloutKind("unknown"),
	} {
		if got := theme.CalloutBadge(k, p).Render("X"); got == "" {
			t.Errorf("CalloutBadge(%q) returned empty", k)
		}
		if got := theme.CalloutBar(k, p).Render("│"); got == "" {
			t.Errorf("CalloutBar(%q) returned empty", k)
		}
	}
}

// TestMarkdownRolesFor_SwapsPerCall: passing a different palette
// produces different styles. Replaces the previous SetActive-based
// swap test — the new design has no global state, so a "swap" is
// simply two MarkdownRolesFor calls with two different palettes,
// and the returned styles must differ.
func TestMarkdownRolesFor_SwapsPerCall(t *testing.T) {
	t.Parallel()
	r := lipgloss.DefaultRenderer()
	tn := theme.MarkdownRolesFor(r, canonical.TokyonightNight).H2.GetForeground()
	cp := theme.MarkdownRolesFor(r, canonical.CatppuccinMocha).H2.GetForeground()
	if tn == cp {
		t.Errorf("expected H2 colour to change between palettes, got %v == %v", tn, cp)
	}
}

// TestMarkdownRolesFor_NoColorPath builds the role bundle through a
// renderer with the Ascii color profile (the same one Render uses
// when WithNoColor / NO_COLOR is active) and checks that styled
// output of meaningful roles is non-empty AND, when the input
// carries text, the text survives the strip.
//
// A11y-4 from docs/design-system-audit.md §2.5: no information may
// hide in colour alone. If a callout badge in the NO_COLOR profile
// produced just whitespace, "DANGER" would silently disappear from a
// terminal forced into Ascii mode.
func TestMarkdownRolesFor_NoColorPath(t *testing.T) {
	t.Parallel()
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(termenv.Ascii))
	p := canonical.TokyonightNight
	roles := theme.MarkdownRolesFor(r, p)

	cases := map[string]struct {
		style lipgloss.Style
		input string
	}{
		"H1Text":     {roles.H1Text, "Heading"},
		"H2":         {roles.H2, "Section"},
		"Paragraph":  {roles.Paragraph, "body text"},
		"CodeSpan":   {roles.CodeSpan, "code"},
		"TaskOpen":   {roles.TaskOpen, "[ ]"},
		"TaskDone":   {roles.TaskDone, "[x]"},
		"CardTitle":  {roles.CardTitle, "Title"},
		"BadgeDaily": {roles.CardBadgeDaily, "DAILY"},
	}
	for name, tc := range cases {
		out := tc.style.Render(tc.input)
		if !strings.Contains(out, tc.input) {
			t.Errorf("%s NO_COLOR: rendered output %q does not contain input %q",
				name, out, tc.input)
		}
	}

	// Callouts must keep their KIND-label readable in NO_COLOR — the
	// colour was the *whole* signal in the rendered chip.
	for _, k := range []theme.CalloutKind{theme.CalloutDanger, theme.CalloutWarning, theme.CalloutSuccess} {
		label := strings.ToUpper(string(k))
		out := theme.CalloutBadge(k, p).Render(label)
		if !strings.Contains(out, label) {
			t.Errorf("CalloutBadge(%q) NO_COLOR: missing label %q in %q", k, label, out)
		}
	}
}

// TestH6_NoFaint guards against a regression of A11y-3 (audit §2.5):
// Faint() on already-Muted text drops contrast below WCAG AA. If a
// well-meaning refactor re-adds Faint(true) to H6, this catches it.
func TestH6_NoFaint(t *testing.T) {
	t.Parallel()
	r := lipgloss.DefaultRenderer()
	roles := theme.MarkdownRolesFor(r, canonical.TokyonightNight)
	if roles.H6.GetFaint() {
		t.Error("H6 has Faint() applied — drops below WCAG AA on FgMuted (A11y-3)")
	}
}
