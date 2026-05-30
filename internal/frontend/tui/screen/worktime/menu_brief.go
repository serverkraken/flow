// Brief-Flow für das Worktime-Aktions-Menü. Zwei Aktionen:
// menuActionBriefWeek (Range = aktuelle ISO-Woche) und
// menuActionBriefMonth (Range = aktueller Monat). Beide rendern
// Reporter.WriteBrief in einen Buffer und routen durch den vom User
// gewählten Output-Target — Markdown landet im integrierten Overlay
// (tmux-Split-Target), pbcopy (Clipboard) oder
// ~/Downloads/worktime-brief-<scope>-<ts>.md (Datei). Kein externer
// Pager mehr: der Worktime-Root hostet view.Model + viewport für den
// Brief, sodass alles im gleichen Pane bleibt.

package worktime

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// briefCmd liefert das tea.Cmd, das Reporter.WriteBrief gegen den
// gegebenen Scope laufen lässt und das Ergebnis ans User-gewählte
// Target dispatcht. Für outputTargetSplit liefert das Cmd einen
// briefViewMsg an den Worktime-Root, der den integrierten Overlay
// öffnet; für die anderen Targets bleibt der Output-Port (Clipboard /
// SaveFile) der richtige Weg.
func briefCmd(deps Deps, target outputTarget, scope domain.ReportRange) tea.Cmd {
	return func() tea.Msg {
		if deps.Reporter == nil {
			return menuActionDoneMsg{err: fmt.Errorf("reporter nicht verdrahtet")}
		}
		ref := deps.Clock.Now()
		var buf bytes.Buffer
		if err := deps.Reporter.WriteBrief(&buf, ref, scope); err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("brief: %w", err)}
		}
		content := buf.String()
		basename := briefBasename(scope, ref)
		// Markdown-Brief ins Split-Target: kein externer Pager mehr,
		// stattdessen ein briefViewMsg an den Worktime-Root für den
		// integrierten Overlay-Viewer.
		if target == outputTargetSplit {
			return briefViewMsg{title: briefOverlayTitle(scope, ref), body: content}
		}
		if deps.Output == nil {
			return menuActionDoneMsg{err: fmt.Errorf("output port nicht verdrahtet")}
		}
		// Brief reicht für Clipboard/SaveFile-Targets weiter durch den
		// generischen Dispatcher. outputTargetSplit ist oben bereits
		// abgefangen, viewer ist hier unused.
		return dispatchToTarget(deps.Output, target, content, basename, "md", "", deps.HomeDir)
	}
}

// briefOverlayTitle formatiert den Titel des Brief-Overlays — Scope +
// Periode, damit der User auch ohne Footer-Subtext weiß, welcher Brief
// vor ihm liegt.
func briefOverlayTitle(scope domain.ReportRange, ref time.Time) string {
	if scope == domain.ReportMonth {
		return fmt.Sprintf("Brief · %s %04d", domain.MonthShortDe(ref.Month()), ref.Year())
	}
	_, w := ref.ISOWeek()
	return fmt.Sprintf("Brief · KW %02d %04d", w, ref.Year())
}

// briefBasename erzeugt den Datei-Default für SaveFile. Format:
// worktime-brief-week-2026-W18 / worktime-brief-month-2026-05.
// Der Adapter hängt -<ts>.md noch an, also bleibt die Basis kollisions-
// frei pro Tag.
func briefBasename(scope domain.ReportRange, ref time.Time) string {
	switch scope {
	case domain.ReportMonth:
		return fmt.Sprintf("worktime-brief-month-%04d-%02d", ref.Year(), int(ref.Month()))
	default:
		_, w := ref.ISOWeek()
		return fmt.Sprintf("worktime-brief-week-%04d-W%02d", ref.Year(), w)
	}
}

// dispatchToTarget routet content durch den Output-Port. Synchron
// aufgerufen aus einer tea.Cmd-Closure (siehe briefCmd / Slice D
// exportCmd / statsCmd) — der Adapter spawnt selbst keine Goroutinen.
//
// Für SaveFile wird der zurückgegebene Pfad als ~/-shortened für den
// Toast aufbereitet, damit ein langer Downloads-Pfad die Modal-Footer
// nicht überlaufen lässt.
func dispatchToTarget(out ports.Output, target outputTarget, content, basename, ext, viewer, home string) tea.Msg {
	switch target {
	case outputTargetClipboard:
		if err := out.Copy(content); err != nil {
			return menuActionDoneMsg{err: err}
		}
		return menuActionDoneMsg{toast: "in Zwischenablage"}
	case outputTargetSplit:
		if err := out.Pager(content, viewer, ext); err != nil {
			return menuActionDoneMsg{err: err}
		}
		return menuActionDoneMsg{toast: "in tmux-Split geöffnet"}
	case outputTargetFile:
		path, err := out.SaveFile(basename, ext, []byte(content))
		if err != nil {
			return menuActionDoneMsg{err: err}
		}
		return menuActionDoneMsg{toast: "gespeichert: " + tildePath(path, home)}
	}
	return menuActionDoneMsg{err: fmt.Errorf("unbekanntes output-target: %d", target)}
}

// tildePath kürzt einen absoluten Pfad mit `~` wenn er unter `home`
// liegt. Toast-Text-Cosmetic — die Output-Adapter geben absolute Pfade
// zurück, aber `~/Downloads/foo.md` ist im Modal-Footer angenehmer.
//
// Pre-A1 las diese Funktion os.Getenv("HOME") direkt; jetzt kommt der
// Wert via Deps.HomeDir vom Composition Root durchgereicht.
func tildePath(p, home string) string {
	if home == "" {
		return p
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// briefScopeFor reverses the menuAction-kind → ReportRange mapping.
// Used by dispatchPending to decide which scope briefCmd takes for a
// given action. menuActionBriefMonth → ReportMonth; everything else
// (specifically BriefWeek) defaults to ReportWeek.
func briefScopeFor(kind menuActionKind) domain.ReportRange {
	if kind == menuActionBriefMonth {
		return domain.ReportMonth
	}
	return domain.ReportWeek
}

// Compile-time assertions that the Brief flow's dependencies remain
// the public Reporter type — refactoring Reporter into a port would
// land here loudly.
var _ = (*usecase.Reporter)(nil)
