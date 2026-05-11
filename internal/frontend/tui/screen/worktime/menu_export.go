// Export-Flow für das Worktime-Aktions-Menü. CSV / JSON via
// Reporter.WriteCSV / WriteJSON, in einen Buffer gerendert und durch
// dispatchToTarget zum gewählten Output-Ziel geleitet.

package worktime

import (
	"bytes"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
)

// exportFormat trennt CSV- und JSON-Pfad. Skill §Anti-patterns —
// «keine Magic Strings» — als Enum statt String-Compare.
type exportFormat int

const (
	exportFormatCSV exportFormat = iota
	exportFormatJSON
)

// ext liefert die Datei-Endung ohne Punkt. Wandert direkt in den Output-
// Adapter (Pager-Temp-File und SaveFile).
func (f exportFormat) ext() string {
	switch f {
	case exportFormatJSON:
		return "json"
	}
	return "csv"
}

// label liefert den User-facing-Format-Namen für Toasts und Fehlerwhich
// im Plain-Text der CLI.
func (f exportFormat) label() string {
	switch f {
	case exportFormatJSON:
		return "JSON"
	}
	return "CSV"
}

// exportPager liefert den Pager für `tmux-Split → glow/less`-Target.
// Beide Formate sind nicht-Markdown — `less -S` (kein word-wrap, side-
// scroll) zeigt CSV/JSON als raw Text in voller Zeilenbreite, was bei
// 6-Spalten-CSV mit langen Notes wichtig ist.
const exportPager = "less -S"

// exportCmd rendert Sessions im gewählten Range im gewählten Format
// und routet das Ergebnis über den Output-Port. Returns ein tea.Cmd,
// das ein menuActionDoneMsg liefert (Toast oder Fehler).
//
// rangeExpr darf leer sein → domain.ParseRange liefert Zero-Range,
// Reporter.WriteCSV/JSON dumpen alles. Mirrors `flow worktime export`
// ohne Argument.
func exportCmd(deps Deps, target outputTarget, rangeExpr string, format exportFormat) tea.Cmd {
	return func() tea.Msg {
		if deps.Reporter == nil {
			return menuActionDoneMsg{err: fmt.Errorf("reporter nicht verdrahtet")}
		}
		if deps.Output == nil {
			return menuActionDoneMsg{err: fmt.Errorf("output port nicht verdrahtet")}
		}
		rng, err := domain.ParseRange(deps.Clock.Now(), rangeExpr)
		if err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("range »%s«: %w", rangeExpr, err)}
		}
		var buf bytes.Buffer
		switch format {
		case exportFormatJSON:
			err = deps.Reporter.WriteJSON(&buf, rng)
		default:
			err = deps.Reporter.WriteCSV(&buf, rng)
		}
		if err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("export %s: %w", format.label(), err)}
		}
		basename := exportBasename(rangeExpr, format)
		return dispatchToTarget(deps.Output, target, buf.String(), basename, format.ext(), exportPager, deps.HomeDir)
	}
}

// exportBasename baut den SaveFile-Default ohne Datums-Suffix. Der
// Adapter hängt -<ts>.<ext> selber an; hier nur der inhaltliche Teil:
// worktime-export-csv-month, worktime-export-json-2026-04-01-to-2026-04-30.
//
// "all" wird substituiert wenn der Range leer ist, damit der Datei-
// name den Scope dokumentiert.
func exportBasename(rangeExpr string, format exportFormat) string {
	scope := strings.TrimSpace(rangeExpr)
	if scope == "" {
		scope = "all"
	}
	scope = sanitizeRangeForFilename(scope)
	return fmt.Sprintf("worktime-export-%s-%s", strings.ToLower(format.label()), scope)
}

// sanitizeRangeForFilename ersetzt Range-Tokens durch Datei-Sicherheits-
// freundliche Varianten: `..` → `-to-`, `/` → `-`. Keine vollständige
// Sanitisierung — wir vertrauen der Range-Input-Validation davor.
func sanitizeRangeForFilename(s string) string {
	s = strings.ReplaceAll(s, "..", "-to-")
	s = strings.ReplaceAll(s, "/", "-")
	return s
}
