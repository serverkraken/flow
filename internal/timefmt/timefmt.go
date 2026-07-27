// Package timefmt holds minute-based duration formatting and HH:MM/duration
// parsing. It imports only the standard library, so any layer may use it —
// the TUI worktime routes and the MCP node tools both do.
package timefmt

import "fmt"

// FormatMin renders a non-negative minute count as "Xh YYm".
func FormatMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

// FormatSaldo renders a signed minute count as "+Xh YYm" / "-Xh YYm".
func FormatSaldo(min int) string {
	sign := "+"
	if min < 0 {
		sign = "-"
		min = -min
	}
	return fmt.Sprintf("%s%dh %02dm", sign, min/60, min%60)
}
