package worktime

// Heute view rendering — pure (heute, now) → string transforms ohne
// Side-effects, plus die zwei Heute-spezifischen Render-Helfer
// (Status-Pille, Total-Threshold-Color). Split aus today.go (Skill
// §No-Monoliths): die ~250 Zeilen Render-Logik laufen sonst bei jedem
// Update- oder Action-Edit mit Diff.

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func (h heute) View() tea.View { return tea.NewView(h.viewContent()) }

func (h heute) viewContent() string {
	if h.width == 0 {
		return ""
	}
	// Projekt-Picker übernimmt den gesamten Bildschirm (eigene titlebox +
	// footer). FullScreen() gibt true zurück solange pp != nil, damit der
	// worktime-Root keinen zweiten titlebox-Wrapper aufbaut.
	if h.pp != nil {
		return h.pp.View()
	}
	if h.dialog == heuteDialogNoteView && h.noteView != nil {
		// Note-Viewer ist ein Vollbild-Sub-Screen via markdown_overlay
		// — eigenes frame + statusBar + footer, keine Dialog-Header-
		// Hülle drumherum.
		return h.noteView.View()
	}
	if h.dialog != heuteDialogNone {
		return h.renderDialog()
	}
	return h.renderBody()
}

func (h heute) renderBody() string {
	if !h.loaded {
		return stDim(h.pal, "  Heute lädt …")
	}
	if h.err != nil {
		return stErr(h.pal, h.err.Error())
	}

	inner := h.width - 4
	if inner <= 0 {
		inner = theme.WideBox
	}
	now := h.deps.Clock.Now()

	// Header pinned at the top (date / headline / progress / summary +
	// optional note + pause context); footer pinned at the bottom (toast
	// slot + hints). The sessions list is the scrollable middle that
	// fitHeight windows around the cursor when the terminal is too short.
	header := []string{h.renderDateLine(now), h.renderHeadline(now), "", h.renderProgressBar(inner, now), h.renderSummary(inner, now)}
	if line := h.renderActiveSessionsIndicator(now); line != "" {
		header = append(header, "", line)
	}
	if line := h.renderAttachedNotes(); line != "" {
		header = append(header, "", line)
	}
	if line := h.renderPauseHint(now); line != "" {
		header = append(header, "", line)
	}

	mid, focus := h.renderSessionsList(inner, now)

	// SlotRows kollabiert auf nichts, wenn kein Toast aktiv ist — der
	// dominante Idle-State zeigt nur eine Leerzeile vor dem Footer
	// statt drei. Transient toasts (2s default) drücken den Footer
	// kurz um 2 Zeilen runter; akzeptabler Trade-off für den Flash.
	footer := toast.SlotRows(h.toast, "  ")
	footer = append(footer, "", renderFooterHints(h.pal, h.footerHints(), inner))

	return fitHeight(header, mid, footer, focus, bodyBudget(h.height), h.pal)
}

// renderDateLine zeichnet "Mo · 01.05.2026" als kleine Anker-Zeile über
// der Headline. Die Heute-Surface zeigt sonst nirgends das Datum — wer
// History parallel offen hat, hat keinen Anhaltspunkt was „heute" bedeutet.
func (h heute) renderDateLine(now time.Time) string {
	return theme.Gap(theme.PadSM) + theme.Dim(domain.FmtDateDe(now, domain.DateShort), h.pal)
}

// pctOfTarget returns total as an integer percentage of target, clamped
// to [0,100]. target<=0 → 0 (no goal set). Shared by renderHeadline and
// renderProgressBar so the headline "Ziel N%", the bar fill, and the
// percentage label can never disagree — they previously duplicated this
// calc, and a Total() change on a frame boundary could desync them.
func pctOfTarget(total, target time.Duration) int {
	if target <= 0 {
		return 0
	}
	pct := int(total * 100 / target)
	if pct > 100 {
		pct = 100
	}
	return pct
}

func (h heute) renderHeadline(now time.Time) string {
	total := h.day.Total(now)
	target := h.day.Target
	// achieved nur mit echtem Ziel — sonst meldete ein target==0-Tag
	// „Ziel erreicht ✓" selbst bei 0:00 erfasst (target==0 erfüllte
	// total>=target immer). Ohne Ziel fällt der Badge auf den Timer-Status
	// (läuft / pausiert) zurück, wie jeder andere unfertige Idle-Tag.
	statusGlyph, statusLabel, statusColor := todayStatusBadge(h.pal, h.day.IsRunning(), target > 0 && total >= target)

	totalText := formatDur(total)
	if h.day.IsRunning() && h.day.Active != nil && now.Sub(*h.day.Active) < time.Minute {
		totalText = formatDurLive(total)
	}
	// Total ohne Bold — Skill §Color semantics: "Never combine bold +
	// accent on adjacent tokens". Die Status-Pille trägt das Bold (canonical
	// §Component vocabulary), die Threshold-Farbe trägt das Gewicht für
	// den Total-Wert.
	totalStr := fgStyle.Foreground(totalThresholdColor(h.pal, total, target, h.day.IsRunning())).Render(totalText)
	// Status-Badge bold (canonical Pill-Behavior).
	statusStr := boldStyle.Foreground(statusColor).Render(statusGlyph + " " + statusLabel)
	// Ohne Tagesziel ist „Ziel N%" bedeutungslos — würde „Ziel 0%" zeigen.
	pctStr := theme.Dim("kein Ziel", h.pal)
	if target > 0 {
		pctStr = theme.Dim(fmt.Sprintf("Ziel %d%%", pctOfTarget(total, target)), h.pal)
	}
	// Skill §Spacing: discrete scale {0,1,2,4} — 2-Cell-Indent links, 4-Cell-Gaps
	// zwischen den drei Status-Cells.
	gap4 := theme.Gap(theme.PadMD + theme.PadXS)
	return theme.Gap(theme.PadSM) + totalStr + gap4 + statusStr + gap4 + pctStr
}

// renderProgressBar nimmt `now` als Parameter, damit Headline,
// Progressbar und Summary innerhalb eines Frames denselben Zeitpunkt
// teilen. Vorher rief jede Render-Funktion h.deps.Clock.Now() einzeln,
// was bei einem Frame-Übergang an einer Sekundengrenze sichtbare
// Divergenz zwischen den drei Prozentwerten produzieren konnte.
func (h heute) renderProgressBar(inner int, now time.Time) string {
	target := h.day.Target
	total := h.day.Total(now)
	pct := pctOfTarget(total, target)
	barCells := inner - 4
	if barCells < 4 {
		barCells = 4
	}
	// BarColored mit totalThresholdColor: die Bar liest sich wie der
	// Status-Badge daneben (Cyan running · Green achieved · Red weit
	// drüber). Vorher Accent-Blau unabhängig vom Zustand — die Bar als
	// primäres Progress-Signal sagte nichts über Erfolg/Drüber-Status.
	barColor := totalThresholdColor(h.pal, total, target, h.day.IsRunning())
	return "  " + statusbar.BarColored(pct, barCells, barColor, h.pal)
}

// renderSummary teilt `now` mit Headline und Progressbar (siehe
// renderProgressBar-Comment).
func (h heute) renderSummary(inner int, now time.Time) string {
	target := h.day.Target
	if target <= 0 {
		// Ohne Tagesziel sind „Ziel/noch/ETA" alle bedeutungslos (zeigten
		// „Ziel 0:00  ·  noch 0:00"); ein Platzhalter liest ehrlicher.
		return renderFooterHints(h.pal, []string{"kein Tagesziel"}, inner)
	}
	total := h.day.Total(now)
	remaining := target - total
	if remaining < 0 {
		remaining = 0
	}
	parts := []string{
		fmt.Sprintf("Ziel %s", formatDur(target)),
		fmt.Sprintf("noch %s", formatDur(remaining)),
	}
	if h.day.Active != nil {
		eta := h.day.Active.Add(target - h.day.Logged)
		parts = append(parts, "ETA "+eta.Format("15:04"))
	}
	return renderFooterHints(h.pal, parts, inner)
}

// renderAttachedNotes renders the chip line that surfaces today's
// linked Kompendium notes. Empty result skips the row entirely so
// the layout doesn't grow a blank gap when nothing is attached.
func (h heute) renderAttachedNotes() string {
	if len(h.attachedNotes) == 0 {
		return ""
	}
	// Spec 2026-05-13-filled-dayoff-dots-supersede: vorher `● Highlight`
	// (= Purple, jetzt Vacation-Identität). Hier eine reine Info-Marker-
	// Stelle, kein Kind — also `›` (glyphs.Info) in Sem.Info, damit der
	// Marker semantisch klar von den Day-Off-Pace-Dots getrennt liest.
	label := theme.Info(glyphs.Info, h.pal)
	ids := stDim(h.pal, strings.Join(h.attachedNotes, "  ·  "))
	hint := stDim(h.pal, "  ·  o/O → ansehen/bearbeiten  ·  R → entfernen")
	return "  " + label + "  " + ids + hint
}

func (h heute) renderPauseHint(now time.Time) string {
	if !h.day.IsPaused() || h.day.PausedAt == nil {
		return ""
	}
	return "  " +
		theme.Warning(glyphs.Paused+" in Pause", h.pal) +
		stDim(h.pal, fmt.Sprintf("  seit %s  ·  %s — `s` setzt fort",
			h.day.PausedAt.Format("15:04"), formatDur(now.Sub(*h.day.PausedAt))))
}

// renderSessionsList returns the scrollable session rows plus the index
// (within those rows) of the row carrying the cursor, so fitHeight can
// keep the focused session visible when the list overflows the terminal
// height. focus is 0 when no session is focused (empty / running-only).
func (h heute) renderSessionsList(inner int, now time.Time) (rows []string, focus int) {
	totalRows := len(h.day.Sessions)
	if h.day.IsRunning() {
		totalRows++
	}
	if totalRows == 0 {
		if h.day.IsPaused() {
			return nil, 0
		}
		return []string{"", stDim(h.pal, "  Noch nichts erfasst — `s` startet")}, 0
	}

	rows = []string{"", picker.SectionHeader(
		fmt.Sprintf("sessions heute (%d)", totalRows), inner, h.pal,
	)}

	if h.day.IsRunning() && h.day.Active != nil {
		elapsed := now.Sub(*h.day.Active)
		// Trailing „läuft" weggelassen — Headline trägt den Status bereits
		// als ▶-Pille (renderHeadline → todayStatusBadge). Hier dupliziert
		// es nur Information und kostet 6+ Zeichen Platz für ein Tag-Slot.
		rows = append(rows, theme.Active(
			fmt.Sprintf("  %s %s → …   %s",
				glyphs.Active, h.day.Active.Format("15:04"), formatDur(elapsed)), h.pal,
		))
	}
	prevStop := time.Time{}
	for i, s := range h.day.Sessions {
		// Pause-Trenner zwischen aufeinanderfolgenden Sessions — spiegelt
		// das Format aus history.go renderDrill, damit Heute und Drill
		// dieselbe Lese-Erfahrung bieten.
		if !prevStop.IsZero() {
			pause := s.Start.Sub(prevStop)
			if pause > 0 {
				rows = append(rows, stDim(h.pal,
					fmt.Sprintf("%s%s Pause %s", theme.Gap(theme.PadMD*2+theme.PadXS), glyphs.BulletDot, formatDur(pause))))
			}
		}
		prevStop = s.Stop
		dur := durationWidth8Style.Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s",
			s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		if i == h.cursor {
			focus = len(rows)
		}
		rows = append(rows, picker.Row(i == h.cursor, label, hint, inner, h.pal))
		if s.Note != "" {
			rows = append(rows, stDim(h.pal, "       "+s.Note))
		}
	}
	return rows, focus
}

// footerHints liefert max 4 Hints, priorisiert nach Frequenz (Skill §Hint
// format: „Maximum 4 hints in a permanent footer; if more apply, the surplus
// belongs in the `?` overlay"). Reihenfolge:
//  1. s → start/stop/resume — globaler Default-State, immer relevant.
//  2. j/k → bewegen — Zeilennavigation. g/G (Sprung oben/unten) lebt im
//     `?`-Overlay (today_dialog.go HelpSections), konsistent mit den
//     Schwester-Screens history/frei, die ebenfalls nur `j/k → bewegen`
//     im Footer führen.
//  3. enter → bearbeiten (auf Session) oder : → aktionen (off-Session).
//  4. ? → Hilfe — Skill §Keybind grammar: `?` ist ein fixed-slot key und
//     muss aus jedem Footer auffindbar sein. Verdrängt den `:`-Hint auf
//     on-Session (Aktions-Menü bleibt via Palette + ?-Overlay erreichbar).
//
// D → löschen, Tag, Note und Pause leben im `?`-Overlay; das 4-Hint-
// Limit verdrängt die destructive Action in den Hilfetext.
func (h heute) footerHints() []string {
	var actions []string
	switch {
	case h.day.IsRunning():
		actions = append(actions, "s → stoppen")
	case h.day.IsPaused():
		actions = append(actions, "s → fortsetzen")
	default:
		actions = append(actions, "s → starten")
	}
	if h.onSession() {
		actions = append(actions, "j/k → bewegen", "enter → bearbeiten", uistrings.HintHelp)
	} else {
		actions = append(actions, "j/k → bewegen", ": → aktionen", uistrings.HintHelp)
	}
	if len(actions) > 4 {
		actions = actions[:4]
	}
	return actions
}

// todayStatusBadge wählt Glyph + Label + Foreground-Color für die Heute-
// Status-Pille. Glyphen aus components/glyphs (canonical Whitelist) statt
// als Magic-Strings inline — so kann ein Audit gegen den Whitelist greifen
// und die Bedeutung ist im Identifier dokumentiert.
//
// Color-Wahl spiegelt die kanonischen Pace-Dot-Semantiken (Skill
// §Color semantics, BuildPaceDots / week.renderPace):
//
//	running && !achieved → Sem.Active (Cyan) — live/läuft gerade
//	running &&  achieved → Sem.Success      — läuft & Ziel erreicht
//	         achieved    → Sem.Success      — Ziel erreicht (idle)
//	else                 → FgMuted          — pausiert / leer
//
// Damit identisches Active=Cyan auf Heute-Headline, Week-Pace-Strip
// und tmux-Pace-Dot — die gleiche laufende Session liest sich überall
// gleich.
func todayStatusBadge(p theme.Palette, running, achieved bool) (string, string, color.Color) {
	sem := p.Sem()
	switch {
	case running && achieved:
		return glyphs.Active, "läuft " + glyphs.Done, sem.Success
	case running:
		return glyphs.Active, "läuft", sem.Active
	case achieved:
		return glyphs.Done, "Ziel erreicht", sem.Success
	}
	return glyphs.Paused, "pausiert", p.FgMuted
}

// renderActiveSessionsIndicator renders the multi-project running-indicator
// line that appears in the header when the new ActiveSessions path is wired
// and at least one session is in flight.
//
// Format: `▶ Projektname 2h 30m  ·  Projektname2 0h 12m`
//
// The indicator is only visible in new-path mode (h.activeSessions != nil).
// In legacy mode (h.activeSessions == nil) this returns "" so the legacy
// header is unchanged.
//
// Project names come from h.activeSessions[i].ProjectID for now — the domain
// model carries the name only after the Projects.Get join that Task 21 will
// add to the sqlite store. Until then we display a short ID suffix.
func (h heute) renderActiveSessionsIndicator(now time.Time) string {
	if len(h.activeSessions) == 0 {
		return ""
	}
	var parts []string
	for _, as := range h.activeSessions {
		elapsed := now.Sub(as.StartedAt)
		// Use project name when available (non-empty), else fall back to the
		// last 8 chars of the ID for readability.
		name := as.ProjectID
		if len(name) > 8 {
			name = name[len(name)-8:]
		}
		parts = append(parts, fmt.Sprintf("%s %s  %s", glyphs.Active, name, formatDur(elapsed)))
	}
	line := strings.Join(parts, stDim(h.pal, "  ·  "))
	return "  " + theme.Active(line, h.pal)
}

// totalThresholdColor picks the today-total foreground based on running
// state and target progress. Red is reserved for "really a lot" so a
// normal hour of overtime doesn't look like an alarm.
func totalThresholdColor(p theme.Palette, total, target time.Duration, running bool) color.Color {
	sem := p.Sem()
	switch {
	case total >= target+4*time.Hour:
		return sem.Danger
	case total >= target:
		return sem.Success
	case running && total >= target-2*time.Hour:
		return sem.Warning
	case running:
		return sem.Active
	}
	return p.FgMuted
}
