// Package wtfmt holds minute-based duration formatters shared by the Worktime
// sibling routes. It imports nothing from the worktime hub or its sibling
// packages, so leaves can use it without forming an import cycle.
package wtfmt

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
