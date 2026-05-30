// Package sidekick — sub-tab hosting (numeric-key routing only).
//
// A screen that owns a strip of sub-tabs (today only worktime, with
// Heute / Woche / Verlauf / Frei) implements subTabHost. The sidekick
// uses the contract to ROUTE numeric keys (1-N) to the host's
// SwitchSubTab — there is no pill rendering at the sidekick level;
// the host draws its own strip inside its own frame (worktime
// titlebox-title), so a second sidekick-level rendering would be
// redundant.
//
// Routing: numeric keys 1-9 reach the active host's SwitchSubTab when
// the host claims the slot; when the active screen is NOT a subTabHost
// (palette / projects / cheatsheet / notes), the keys fall through to
// the active screen so palette's 1-9 direct-pick keeps working.
package sidekick

import (
	tea "charm.land/bubbletea/v2"
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
