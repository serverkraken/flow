// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"fmt"
	"time"
)

// fmtDur renders a duration as HH:MM (clamped at zero).
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}
