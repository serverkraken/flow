// Package projects renders the WebUI projects surface at `/projects`.
// All data resolution happens in the handler; templates only render
// formatted strings off a flat view-model.
//
// M6 is read-only — create/rename/archive land in M7 (Task 13). The
// "Neues Projekt" button is rendered disabled with a Phase-2 hint.
package projects

import (
	"strconv"
	"strings"
)

// FormatTotalsLabel renders the page-header eyebrow:
//
//	"42 Projekte · 4 aktiv letzte 7 Tage"
//	"1 Projekt · 0 aktiv letzte 7 Tage"
//
// German uses "Projekt" for singular and "Projekte" for plural. Pre-
// formatted here so the templ surface stays markup-only.
func FormatTotalsLabel(total, active int) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(total))
	if total == 1 {
		b.WriteString(" Projekt")
	} else {
		b.WriteString(" Projekte")
	}
	b.WriteString(" · ")
	b.WriteString(strconv.Itoa(active))
	b.WriteString(" aktiv letzte 7 Tage")
	return b.String()
}
