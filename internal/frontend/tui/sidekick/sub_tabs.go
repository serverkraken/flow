// Package sidekick — sub-tab hosting.
//
// A screen that owns a strip of sub-tabs (today only worktime, with
// Heute / Woche / History / Frei) implements subTabHost. The sidekick
// consumes that interface to render the sub-tab pills right-aligned in
// the global tab strip — saving one permanent row of vertical budget
// over the prior stacked layout (sidekick strip + worktime strip).
//
// Routing: numeric keys 1-9 reach the active host's SwitchSubTab when
// the host claims the slot; when the active screen is NOT a subTabHost
// (palette / projects / cheatsheet / notes), the keys fall through to
// the active screen so palette's 1-9 direct-pick keeps working.
package sidekick

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// subTabHost is the contract a screen implements when it owns its own
// strip of sub-tabs. SubTabs returns the labels in display order;
// SubTabIndex returns the currently active sub-tab; SwitchSubTab(i) is
// invoked when the sidekick routes a numeric key (1-9) to the host.
//
// The returned tea.Model from SwitchSubTab REPLACES the host in the
// sidekick's screens array — the same pattern as stateRestorer's
// WithState — so the host can hand back an updated value cleanly.
type subTabHost interface {
	SubTabs() []string
	SubTabIndex() int
	SwitchSubTab(i int) tea.Model
}

// renderSubTabPills renders the host's sub-tab labels as pills in the
// same visual grammar as the main strip: the active one bracketed and
// styled via activeTabStyle, inactive ones dimmed and parenthesised.
// Each pill is prefixed with its numeric shortcut (1-N) so the strip
// self-documents the keybinds.
//
// Returns an empty string when the host advertises no sub-tabs (defensive
// — a host would have no reason to do that, but the caller is allowed
// to assume any non-empty return is renderable as-is).
func (m Model) renderSubTabPills(host subTabHost) string {
	labels := host.SubTabs()
	if len(labels) == 0 {
		return ""
	}
	active := host.SubTabIndex()
	style := activeTabStyle(m.pal)
	parts := make([]string, len(labels))
	for i, label := range labels {
		shortcut := fmt.Sprintf("%d %s", i+1, label)
		if i == active {
			parts[i] = style.Render("[" + shortcut + "]")
		} else {
			parts[i] = theme.Dim("("+shortcut+")", m.pal)
		}
	}
	return strings.Join(parts, " ")
}

// composeStripWithSubTabs concatenates the main strip with the host's
// sub-tab pills, right-aligned via theme.Gap. When the combined width
// fits, the gap fills the slack; when it doesn't, the pills snap into
// the strip with a minimum theme.PadSM separator so they stay legible
// on narrow panes instead of overlapping.
func (m Model) composeStripWithSubTabs(mainStrip string, host subTabHost) string {
	pills := m.renderSubTabPills(host)
	if pills == "" {
		return mainStrip
	}
	used := lipgloss.Width(mainStrip) + lipgloss.Width(pills)
	if m.width > 0 && used < m.width {
		// Trailing space (1 col) keeps the right edge of the last pill
		// off the screen edge — same convention as the main strip's
		// leading space — so the active-tab underline doesn't bleed
		// into column 0 of the next row in narrow tmux panes.
		gap := m.width - used - 1
		if gap < theme.PadSM {
			gap = theme.PadSM
		}
		return mainStrip + theme.Gap(gap) + pills
	}
	return mainStrip + theme.Gap(theme.PadSM) + pills
}
