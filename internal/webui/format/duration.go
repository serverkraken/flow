// Package format holds the pure-Go locale/format helpers shared across
// every WebUI surface (dashboard, worktime, notes, repos, …). Each
// function is template-callable, has no I/O, and never imports a
// handler-specific package — that keeps the templ codegen + the unit
// tests trivially decoupled from the handler boundary.
//
// All helpers render German output by design; i18n is out of scope for
// Phase 1.
package format

import (
	"fmt"
	"time"
)

// FormatHHMM renders a duration as "H:MM" (with no leading sign).
// Negative durations are clamped to 0 — callers that want a signed
// readout should call FormatSignedHHMM instead.
func FormatHHMM(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%d:%02d", h, m)
}

// FormatSignedHHMM renders a duration as "+H:MM" / "-H:MM" / "0:00".
// Used for saldo readouts where the sign carries meaning.
func FormatSignedHHMM(d time.Duration) string {
	if d == 0 {
		return "0:00"
	}
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%s%d:%02d", sign, h, m)
}
