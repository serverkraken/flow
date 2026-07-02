package webui

import (
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
)

// TimerWidgetVM drives the global shell timer widget (desktop card + mobile
// chip). Exactly ONE session can run per owner; the widget is that session's
// single global home (IA rule: eine globale Aktion, ein globales Zuhause).
type TimerWidgetVM struct {
	Running     bool
	Unbound     bool // running without a node → stop requires choosing one
	SessionID   string
	NodeID      string
	NodeName    string
	NodeColor   string
	NodeKind    domain.NodeKind
	BaseSeconds int64
	Bookable    []domain.Node
	Err         string // i18n-resolved message rendered inline (never a popup)
}

// fmtClockHMS renders the initial "HH:MM:SS" clock text for the widget's
// [data-timer data-timer-fmt="clock"] span and the chip's [data-mini-timer]
// span — the two elements the live-timer script in components/base.templ
// re-renders via `p(h) + ':' + p(m) + ':' + p(s)` (its p() zero-pads EVERY
// component, hours included, not just minutes/seconds). Unlike fmtSecsClock
// (cockpit_vm.go, "0h 01m 30s" — the default/non-clock JS branch), this must
// match that exact padded-hours shape or the JS tick overwrites a
// differently-formatted initial paint with a visible flash/mismatch.
func fmtClockHMS(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
