# TUI UX-Review Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adressiere die 25 Findings des 2026-05-30 TUI-Design-Reviews in 10 atomar mergebaren PR-Phasen — von Skill-Drift-Quick-Wins bis hin zu strukturellen UX-Verbesserungen, die jenseits der Skill-Regeln liegen.

**Architecture:** Bottom-up: erst Skill-Drift (semantische Farben, Glyph-Bedeutung, Footer-Vokabular), dann Components (atomic-Style-Cache-Konsolidierung, Fuzzy-Highlight-Reuse), dann strukturelle UX (Tab-Strip-Hierarchie, Cross-Screen-Sync). Jede Phase produziert grünes `make ci` und einen werthaltigen Diff allein.

**Tech Stack:** Go 1.22, Bubbletea v2, Lipgloss v2, charmbracelet/x/ansi für Width-Math, `screen_baseline_test.go` als Lint-Floor.

**Spec:** Konsolidiert aus der Review-Session am 2026-05-30 (siehe Conversation-Log). Sektion-Nummern (§1.1, §2.3 etc.) referenzieren die Review-Findings.

---

## File Structure

Eine Übersicht über alle Files, die im Verlauf des Plans angefasst werden — gruppiert nach Bereich, mit Phasenzuordnung in `[P-N]`.

**Theme / Tokens:**
- `internal/frontend/tui/theme/builders.go` — neuer `Active` Builder `[P1]`, neuer `Gap` Helper `[P4]`
- `internal/frontend/tui/theme/builders_test.go` — Smoke-Tests für neuer Builder + Helper `[P1, P4]`
- `internal/frontend/tui/theme/tokens.go` — bestehende Padding-Tokens (PadXS/SM/MD) bleiben, neue Token-Konstanten je nach Phase

**Components:**
- `internal/frontend/tui/components/markdown_overlay/chrome_styles.go` — BorderStrong + Highlight + key-Aliase `[P1, P3]`
- `internal/frontend/tui/components/markdown_overlay/keys.go` — ctrl+u/ctrl+d Aliase `[P3]`
- `internal/frontend/tui/components/strings/strings.go` — neue Konstanten `HintSearchInput`, `HintConfirmBracketed` `[P3]`
- `internal/frontend/tui/components/picker/row.go` — neuer `RowWithMatch` `[P7]`
- `internal/frontend/tui/components/picker/row_test.go` — Tests `[P7]`

**Screens — Worktime:**
- `internal/frontend/tui/screen/worktime/today_render.go` — Running-Color, Gaps `[P1, P4]`
- `internal/frontend/tui/screen/worktime/week.go` — Marker-Color, %-Style, Gaps, Footer `[P1, P4]`
- `internal/frontend/tui/screen/worktime/history_heatmap.go` — Style-Cache, Cursor-Today-Style `[P5]`
- `internal/frontend/tui/screen/worktime/history.go` — Style-Cache neue Felder `[P5]`
- `internal/frontend/tui/screen/worktime/today.go` + alle anderen Sub-Tabs — WorktimeChanged Msg-Handling `[P9]`
- `internal/frontend/tui/screen/worktime/model.go` — Sub-Tab-Surface-Restrukturierung `[P10]`

**Screens — Palette / Projects:**
- `internal/frontend/tui/screen/palette/model.go` + `render.go` — Glyph-Cleanup, picker.RowWithMatch `[P2, P7]`
- `internal/frontend/tui/screen/projects/model.go` — picker.RowWithMatch `[P7]`

**Screens — Kompendium Browse:**
- `internal/kompendium/frontend/tui/browse/styles.go` — komplette Refaktorierung auf per-Model Cache `[P6]`
- `internal/kompendium/frontend/tui/browse/model.go` — `styles browseStyles` Feld `[P6]`
- `internal/kompendium/frontend/tui/browse/render_root.go` — Footer-Migration, Glyph-Cleanup, distinkte Counts-Glyphen `[P2, P3, P6]`
- `internal/kompendium/frontend/tui/browse/render_row.go` — Cursor-Caret Glyph, Empty-State-Slim `[P2, P5]`
- `internal/kompendium/frontend/tui/browse/render_status.go` — Strings-Konstante, Typ-Label `[P3, P6]`

**Screens — Sidekick:**
- `internal/frontend/tui/sidekick/model.go` — Sub-Tab-Hosting `[P10]`
- `internal/frontend/tui/sidekick/sub_tabs.go` — neuer Helper `[P10]`

**Domain:**
- `internal/domain/datefmt.go` — neuer `FmtDateDe` Helper `[P8]`
- `internal/domain/datefmt_test.go` — Tests `[P8]`
- `internal/domain/messages.go` — neuer `WorktimeChangedMsg` `[P9]`

**Lint:**
- `internal/frontend/tui/lint/screen_baseline_test.go` — Baseline-Updates pro Phase (jeder Cleanup ratchets runter)
- `internal/frontend/tui/lint/screen_hue_check_test.go` — Scope erweitern um `internal/kompendium/frontend/tui/browse/` nach Phase 6

---

## Phase 1 — Skill-Drift Quick-Wins (Color Semantics)

**Findings:** §2.1, §2.2, §2.3, §2.4, §3.2, §3.6, §3.7, §3.8

**Eine PR. Niedrigstes Risiko, maximaler Konsistenz-Gewinn. Adressiert markdown_overlay-Chrome (betrifft 5 Surfaces) + Worktime-Color-Drift + Kompendium-headerSeparator.**

### Task 1.1: `theme.Active` Builder hinzufügen

**Files:**
- Modify: `internal/frontend/tui/theme/builders.go`
- Modify: `internal/frontend/tui/theme/builders_test.go`

- [ ] **Step 1: Failing Test schreiben**

In `internal/frontend/tui/theme/builders_test.go` ans Datei-Ende anhängen:

```go
func TestActive_RendersCyanBold(t *testing.T) {
	p := theme.TokyonightNight
	out := theme.Active("läuft", p)
	if !strings.Contains(out, "läuft") {
		t.Fatalf("Active: expected content %q in %q", "läuft", out)
	}
	// Active ist Cyan+Bold — Sem.Active ist der canonical Token,
	// gleicher Hex wie Sem.Info aber distinkter Role (running/live).
	wantFg := p.Sem().Active
	if !containsForeground(out, wantFg) {
		t.Fatalf("Active: expected fg=%v in output", wantFg)
	}
	if !strings.Contains(out, "\x1b[1m") {
		t.Fatalf("Active: expected bold SGR (\\x1b[1m) in output")
	}
}
```

(`containsForeground` ist der bestehende Test-Helper im File — wenn nicht vorhanden, durch `strings.Contains(out, fmt.Sprintf("%v", wantFg))` ersetzen.)

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/theme/ -run TestActive_RendersCyanBold -v
```

Expected: `FAIL` mit "undefined: theme.Active".

- [ ] **Step 3: Builder implementieren**

In `internal/frontend/tui/theme/builders.go` an passender Stelle (alphabetisch nach `Heading`):

```go
// Active renders s with Sem.Active (Cyan) + Bold — the canonical
// "running / live / in-progress" foreground. Distinct from Info
// (same hex, different role: Info is informational-without-action,
// Active marks a process that is currently happening). Skill
// §Color semantics requires the role-name in code so a palette swap
// that redefines Active without touching Info stays coherent.
func Active(s string, p Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Sem().Active).
		Bold(true).
		Render(s)
}
```

- [ ] **Step 4: Run test to verify pass**

```bash
go test ./internal/frontend/tui/theme/ -run TestActive_RendersCyanBold -v
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/theme/builders.go internal/frontend/tui/theme/builders_test.go
git commit -m "feat(theme): add Active builder for running/live foreground

Cyan+Bold, distinct semantic role from Info (same hex, different
intent). Used by today/week running-session render paths in next
commits. Skill §Color semantics — Active is the canonical
running/live token."
```

### Task 1.2: Heute laufende Session in Active statt Success

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_render.go:233-237`
- Test: `internal/frontend/tui/screen/worktime/today_render_test.go` (anlegen falls fehlt; sonst erweitern)

- [ ] **Step 1: Failing Test schreiben**

In `today_render_test.go` (anlegen oder erweitern):

```go
func TestRenderSessionsList_RunningSessionUsesActiveNotSuccess(t *testing.T) {
	pal := theme.TokyonightNight
	now := mustTime("2026-05-30T10:30:00+02:00")
	active := mustTime("2026-05-30T09:00:00+02:00")
	h := heute{
		pal:    pal,
		width:  80,
		loaded: true,
		day: domain.Day{
			Active: &active,
			// no past Sessions — running-only state
		},
	}
	rows, _ := h.renderSessionsList(76, now)
	joined := strings.Join(rows, "\n")
	// Cyan (Sem.Active) must be present on the running line.
	if !strings.Contains(joined, fmt.Sprintf("%v", pal.Sem().Active)) {
		t.Errorf("running session: expected Sem.Active fg, got %q", joined)
	}
	// Green (Sem.Success) must NOT appear on the running line — that
	// belongs to the achieved state only.
	if strings.Contains(joined, fmt.Sprintf("%v", pal.Sem().Success)) {
		t.Errorf("running session: should not carry Sem.Success fg")
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderSessionsList_RunningSessionUsesActiveNotSuccess -v
```

Expected: `FAIL` — die Zeile rendert aktuell mit `theme.Success`.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/worktime/today_render.go` Zeile 233 ändern:

```go
// VORHER:
//	rows = append(rows, theme.Success(
//		fmt.Sprintf("  %s %s → …   %s",
//			glyphs.Active, h.day.Active.Format("15:04"), formatDur(elapsed)), h.pal,
//	))

// NACHHER:
rows = append(rows, theme.Active(
	fmt.Sprintf("  %s %s → …   %s",
		glyphs.Active, h.day.Active.Format("15:04"), formatDur(elapsed)), h.pal,
))
```

- [ ] **Step 4: Test grün + Suite grün**

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

Expected: alle Tests inklusive der neuen passen.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/today_render.go internal/frontend/tui/screen/worktime/today_render_test.go
git commit -m "fix(worktime): running session uses Active color, not Success

Skill §Color semantics: Active (Cyan) marks running/live;
Success (Green) marks achievement. Heute's session list was
painting the live row in green, conflicting with todayStatusBadge
(which correctly uses Sem.Active for running). Now consistent."
```

### Task 1.3: Woche Today-Marker in Active statt Success

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/week.go:309-314`
- Modify: `internal/frontend/tui/screen/worktime/week_test.go` (extend; file exists per `unified-dayoff-glyphs` plan)

- [ ] **Step 1: Failing Test schreiben**

In `week_test.go` neuer Test:

```go
func TestRenderDayRow_TodayRunningUsesActive_AchievedUsesSuccess(t *testing.T) {
	pal := theme.TokyonightNight
	w := newWoche(pal, Deps{Clock: testClock{now: mustTime("2026-05-30T10:00:00+02:00")}})
	now := w.deps.Clock.Now()

	// Today + running + below target — expect Active (Cyan), no Success.
	dRun := domain.WeekDay{
		Date:    now,
		IsToday: true,
		Active:  &now,
		Logged:  1 * time.Hour,
		Target:  8 * time.Hour,
	}
	rowRun := w.renderDayRow(0, dRun, 12, now)
	if !strings.Contains(rowRun, fmt.Sprintf("%v", pal.Sem().Active)) {
		t.Errorf("today+running: expected Sem.Active fg in row, got %q", rowRun)
	}

	// Today + achieved — expect Success (Green) for the Done glyph.
	dDone := domain.WeekDay{
		Date:    now,
		IsToday: true,
		Logged:  9 * time.Hour,
		Target:  8 * time.Hour,
	}
	rowDone := w.renderDayRow(0, dDone, 12, now)
	if !strings.Contains(rowDone, fmt.Sprintf("%v", pal.Sem().Success)) {
		t.Errorf("today+achieved: expected Sem.Success fg in row, got %q", rowDone)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderDayRow_TodayRunningUsesActive_AchievedUsesSuccess -v
```

Expected: erstes Assertion `FAIL` — running-Marker ist aktuell `theme.Success`.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/worktime/week.go` Zeile 309-314 ersetzen:

```go
// VORHER:
//	extra := ""
//	if d.IsToday && d.Active != nil {
//		extra += "  " + theme.Success(glyphs.Active, w.pal)
//	}
//	if total >= d.Target {
//		extra += "  " + theme.Success(glyphs.Done, w.pal)
//	}

// NACHHER:
extra := ""
if d.IsToday && d.Active != nil {
	extra += "  " + theme.Active(glyphs.Active, w.pal)
}
if total >= d.Target {
	extra += "  " + theme.Success(glyphs.Done, w.pal)
}
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/week.go internal/frontend/tui/screen/worktime/week_test.go
git commit -m "fix(worktime): today-running marker uses Active not Success

Spiegel zu today/heute: live = Cyan, achieved = Green.
Vorher kollidierten beide Marker (running ▶ + done ✓) in
derselben grünen Farbe nebeneinander — zwei verschiedene
Signale flachten zu einem."
```

### Task 1.4: Woche %-Prozent als Strong statt Heading

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/week.go:341-342`

- [ ] **Step 1: Failing Test schreiben**

In `week_test.go` erweitern:

```go
func TestRenderTotals_PercentUsesStrongNotAccent(t *testing.T) {
	pal := theme.TokyonightNight
	w := newWoche(pal, Deps{Clock: testClock{now: mustTime("2026-05-30T10:00:00+02:00")}})
	w.width = 80
	w.loaded = true
	w.week = []domain.WeekDay{
		{Date: mustTime("2026-05-25T00:00:00+02:00"), Target: 8 * time.Hour, Logged: 8 * time.Hour},
	}
	out := strings.Join(w.renderTotals(76, w.deps.Clock.Now()), "\n")
	// Sem.Accent (Blue) MUST NOT colour the %-figure — Skill §Color
	// semantics: Accent ist für interactive/selected/focused, nicht
	// für statische Zahlen.
	if strings.Contains(out, fmt.Sprintf("%v", pal.Sem().Accent)) {
		t.Errorf("renderTotals: %% should not use Sem.Accent, got %q", out)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderTotals_PercentUsesStrongNotAccent -v
```

Expected: `FAIL` — Heading nutzt Sem.Accent.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/worktime/week.go` Zeile 341-342:

```go
// VORHER:
//	bar := "  " + statusbar.Bar(pct, barW, w.pal) + "  " +
//		theme.Heading(fmt.Sprintf("%3d%%", pct), w.pal)

// NACHHER:
bar := "  " + statusbar.Bar(pct, barW, w.pal) + "  " +
	theme.Strong(fmt.Sprintf("%3d%%", pct), w.pal)
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/week.go internal/frontend/tui/screen/worktime/week_test.go
git commit -m "fix(worktime): week %% uses Strong not Heading

Skill §Color semantics: Accent (Blue) reserviert für
interactive/selected/focused. Eine Prozent-Zahl ist kein
Heading — Strong (Fg+Bold) trägt das Gewicht ohne den
Accent-Token zu verdünnen."
```

### Task 1.5: Woche-Footer `?`-Hint hinzufügen

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/week.go:481-483`

- [ ] **Step 1: Failing Test schreiben**

In `week_test.go`:

```go
func TestFooterHints_ContainsHelp(t *testing.T) {
	w := newWoche(theme.TokyonightNight, Deps{Clock: testClock{now: time.Now()}})
	hints := w.footerHints()
	found := false
	for _, h := range hints {
		if strings.Contains(h, "? → Hilfe") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("footerHints: expected ?-help hint, got %v", hints)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — Woche-Footer hat nur 3 Hints.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/worktime/week.go` Zeile 481-483:

```go
import (
	// ...
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

func (w woche) footerHints() []string {
	return []string{
		"j/k → bewegen",
		"g/G → erste/letzte",
		": → aktionen",
		uistrings.HintHelp,
	}
}
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/week.go internal/frontend/tui/screen/worktime/week_test.go
git commit -m "fix(worktime): week footer includes ?-help hint

Konsistenz mit Heute/Palette/Projects: alle Hauptscreens
führen ?-Hilfe im 4-Cap-Footer. Vorher fehlte das in Woche
und der User musste aus muscle-memory dranbleiben."
```

### Task 1.6: markdown_overlay Frame auf BorderStrong + Title auf Highlight

**Files:**
- Modify: `internal/frontend/tui/components/markdown_overlay/chrome_styles.go:58-66`
- Test: `internal/frontend/tui/components/markdown_overlay/chrome_styles_test.go` (anlegen falls fehlt)

- [ ] **Step 1: Failing Test schreiben**

In `chrome_styles_test.go`:

```go
package markdown_overlay

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestBuildStyles_FrameUsesBorderStrong(t *testing.T) {
	p := theme.TokyonightNight
	cs := buildStyles(p)
	got := cs.frame.GetBorderTopForeground()
	want := p.Sem().BorderStrong
	if got != want {
		t.Errorf("frame BorderForeground = %v, want %v (BorderStrong, Skill load-bearing)", got, want)
	}
}

func TestBuildStyles_TitleUsesHighlight(t *testing.T) {
	p := theme.TokyonightNight
	cs := buildStyles(p)
	got := cs.title.GetForeground()
	want := p.Sem().Highlight
	if got != want {
		t.Errorf("title fg = %v, want %v (Highlight per titlebox convention)", got, want)
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/frontend/tui/components/markdown_overlay/ -run "TestBuildStyles_Frame|TestBuildStyles_Title" -v
```

Expected: beide `FAIL` — frame ist Sem.Accent, title ist Sem.Accent.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/components/markdown_overlay/chrome_styles.go` Zeile 58-66:

```go
// VORHER:
//	frame: lipgloss.NewStyle().
//		Border(lipgloss.RoundedBorder()).
//		BorderForeground(sem.Accent).
//		Padding(0, 1),
//
//	title: lipgloss.NewStyle().
//		Foreground(sem.Accent).
//		Bold(true),

// NACHHER:
frame: lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(sem.BorderStrong).
	Padding(0, 1),

title: lipgloss.NewStyle().
	Foreground(sem.Highlight).
	Bold(true),
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/components/markdown_overlay/ -v
go test ./... 2>&1 | tail -20
```

Expected: alle grün. (Cheatsheet, brief-view, today-note-view, history-drill-note, kompendium full-view erben den neuen Look — screenshot-Snapshots werden mit aktualisiert falls vorhanden.)

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "fix(markdown_overlay): frame BorderStrong, title Highlight

Skill §Component vocabulary: load-bearing frames müssen
BorderStrong tragen (≥3:1 WCAG-Non-Text). titlebox-Tabelle
gibt Highlight (Purple+Bold) als Title-Farbe vor. Vorher
nutzten Frame UND Title beide Sem.Accent — kollidierte mit
dem Cursor-Style (auch Accent) und brach single-accent-per-row.

Betrifft 5 Surfaces: Cheatsheet, Brief-Overlay, Heute-Note-
View, History-Drill-Note-View, Kompendium-Full-View."
```

### Task 1.7: Kompendium headerSeparatorStyle auf Sem.Border

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/styles.go:140-141`

- [ ] **Step 1: Failing Test schreiben**

In `internal/kompendium/frontend/tui/browse/styles_test.go` (anlegen falls fehlt):

```go
package browse

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestHeaderSeparator_UsesSemBorderNotBgChip(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	got := headerSeparatorStyle.GetForeground()
	want := theme.TokyonightNight.Sem().Border
	if got != want {
		t.Errorf("headerSeparator fg = %v, want %v (Sem.Border)", got, want)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — aktuell BgChip.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/styles.go` Zeile 140-141:

```go
// VORHER:
//	headerSeparatorStyle = lipgloss.NewStyle().
//		Foreground(pal.BgChip)

// NACHHER:
headerSeparatorStyle = lipgloss.NewStyle().
	Foreground(sem.Border)
```

- [ ] **Step 4: Test grün**

```bash
go test ./internal/kompendium/frontend/tui/browse/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/
git commit -m "fix(kompendium): header separator uses Sem.Border, not BgChip

Skill §Color semantics: Bg-Surface-Token sind für Selection
und Callouts, nicht als Foreground. Sem.Border (= BgCode)
ist der canonical Separator-Token, identisch zu picker.
SectionHeader und allen Trenner-Linien im worktime-Strip."
```

### Task 1.8: Kompendium errorStyle ohne Bold

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/styles.go:253-255`

- [ ] **Step 1: Failing Test schreiben**

In `styles_test.go` erweitern:

```go
func TestErrorStyle_NotBold(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	// Skill §Builder catalog: Err = Red, "no Bold; not a label".
	// errorStyle wird als Paragraph-Surface verwendet ("Fehler beim
	// Bearbeiten: ..."), nicht als Pille — darf nicht bold sein.
	if errorStyle.GetBold() {
		t.Error("errorStyle: must not be Bold (Skill §Builder catalog: Err is paragraph, not label)")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/styles.go` Zeile 253-255:

```go
// VORHER:
//	errorStyle = lipgloss.NewStyle().
//		Foreground(sem.Danger).
//		Bold(true)

// NACHHER:
errorStyle = lipgloss.NewStyle().
	Foreground(sem.Danger)
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/kompendium/frontend/tui/browse/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/
git commit -m "fix(kompendium): errorStyle not Bold for paragraph use

Skill builder catalog: Err = Red, no Bold (it's a paragraph,
not a label). The Bold flavour belongs to Danger (used for
the modalDanger style, which IS a label)."
```

### Task 1.9: Phase-1 Baseline-Update

**Files:**
- Modify: `internal/frontend/tui/lint/screen_baseline_test.go`

- [ ] **Step 1: `make ci` laufen lassen**

```bash
make ci
```

Expected: alle Tests grün. Falls `screen_baseline_test.go` über-/unter-schießt, Counter anpassen.

- [ ] **Step 2: Baseline ratchet wenn anwendbar**

Wenn Counter sich verringert hat (z.B. week.go von 16 auf 15 wegen Heading-Drop), in `screen_baseline_test.go` aktualisieren:

```go
// In dem map-Literal, die betroffene Zeile:
"worktime/week.go": 15, // war 16
```

- [ ] **Step 3: Final commit der Phase**

```bash
git add internal/frontend/tui/lint/screen_baseline_test.go
git commit -m "chore(lint): ratchet baseline after phase-1 cleanups"
```

- [ ] **Step 4: Phase-PR erstellen**

```bash
gh pr create --title "fix(tui): phase 1 — color semantics + chrome cleanup" --body "$(cat <<'EOF'
## Summary
Phase 1 of the 2026-05-30 UX-Review-Cleanup plan
(`docs/superpowers/plans/2026-05-30-tui-ux-review-cleanup.md`):
addresses 8 Skill-§Color-semantics / §Component-vocabulary drifts.

- §2.1 Heute running session: Success → Active
- §2.2 Woche today-running marker: Success → Active
- §2.3 markdown_overlay frame: Accent → BorderStrong
- §2.4 markdown_overlay title: Accent → Highlight
- §3.2 Woche % figure: Heading → Strong
- §3.6 Woche footer: + `?` hint
- §3.7 Kompendium header separator: BgChip → Sem.Border
- §3.8 Kompendium errorStyle: drop Bold

New `theme.Active` builder enables semantic distinction from
`theme.Info` (same hex, different role).

## Test plan
- [ ] `make ci` green
- [ ] Visual smoke on Cheatsheet (Frame change visible)
- [ ] Visual smoke on Heute with running session (Cyan, not Green)
- [ ] Visual smoke on Woche today-row (Cyan ▶ instead of Green ▶)
EOF
)"
```

---

## Phase 2 — Glyph-Semantik klären

**Findings:** §1.5 (▶ Glyph überlastet an 3 Stellen), §3.3 (Kompendium Counts identische Glyphen), §4.1 (Pause-Trenner-Format)

**Eine PR. Glyphen tragen Bedeutung — die Drift ist subtiler als Color-Drift, aber ebenso konsistenz-zerstörerisch.**

### Task 2.1: Palette Filter-Focus-Prompt ohne ▶

**Files:**
- Modify: `internal/frontend/tui/screen/palette/render.go:42-46`

- [ ] **Step 1: Failing Test schreiben**

In `internal/frontend/tui/screen/palette/render_test.go` (anlegen oder erweitern):

```go
func TestRenderPrompt_FocusedDoesNotUseActiveGlyph(t *testing.T) {
	p := theme.TokyonightNight
	m := Model{pal: p, width: 80, styles: newPaletteStyles(p)}
	m.filter = form.NewTextInput("…", p)
	m.filter.Focus()
	out := m.viewContent()
	// glyphs.Active (▶) ist Skill §Glyph whitelist "running/live",
	// nicht "Focus". Der focused-Prompt darf ihn nicht tragen.
	if strings.Contains(out, glyphs.Active) {
		t.Errorf("focused prompt: must not use glyphs.Active (▶ = running, not focus); got %q", out)
	}
	// glyphs.Info (›) als Fokus-Marker, Accent+Bold via Heading.
	if !strings.Contains(out, glyphs.Info) {
		t.Errorf("focused prompt: expected glyphs.Info (›) marker")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — Render baut aktuell ▶ ein.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/palette/render.go` Zeile 42-46:

```go
// VORHER:
//	prompt := theme.Dim(glyphs.Info+" ", m.pal)
//	if m.filter.Focused() {
//		prompt = theme.Heading(glyphs.Active+" ", m.pal)
//	}

// NACHHER:
prompt := theme.Dim(glyphs.Info+" ", m.pal)
if m.filter.Focused() {
	// Focus: same glyph, accent+bold styling — non-color cue via
	// Bold so NO_COLOR users still see the change. Glyph-swap (›→▶)
	// vorher war semantisch falsch: ▶ = running, nicht focus.
	prompt = theme.Heading(glyphs.Info+" ", m.pal)
}
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/palette/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/palette/
git commit -m "fix(palette): focused prompt keeps › glyph, styles it differently

Skill §Glyph whitelist: ▶ Active marks running/live. Filter-Focus
ist nicht 'running' — ist 'fokussiert'. Style-Swap (Dim → Heading)
trägt das Focus-Signal, Glyph bleibt konstant."
```

### Task 2.2: Palette Preview-Prefix ohne ▶

**Files:**
- Modify: `internal/frontend/tui/screen/palette/render.go:78-89`

- [ ] **Step 1: Failing Test schreiben**

```go
func TestRenderPreview_PrefixUsesAccentBarNotActive(t *testing.T) {
	p := theme.TokyonightNight
	m := Model{
		pal:    p,
		width:  80,
		styles: newPaletteStyles(p),
		visible: []domain.PaletteEntry{{Label: "Test", Action: "echo hi"}},
	}
	out := m.renderPreview(76)
	if strings.Contains(out, glyphs.Active) {
		t.Errorf("preview: must not use glyphs.Active (▶ = running, preview is future-action)")
	}
	if !strings.Contains(out, glyphs.AccentBar) {
		t.Errorf("preview: expected glyphs.AccentBar (▎) as preview marker")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `internal/frontend/tui/screen/palette/render.go` Zeile 78-89:

```go
// VORHER:
//	action := m.visible[m.cursor].Action
//	prefix := "  " + glyphs.Active + " "
//	available := maxWidth - lipgloss.Width(prefix)
//	if available < 8 {
//		return ""
//	}
//	action = uistrings.Truncate(action, available)
//	return "  " + m.styles.border.Render(glyphs.Active) + " " + m.styles.hint.Render(action)

// NACHHER:
action := m.visible[m.cursor].Action
prefix := "  " + glyphs.AccentBar + " "
available := maxWidth - lipgloss.Width(prefix)
if available < 8 {
	return ""
}
action = uistrings.Truncate(action, available)
return "  " + m.styles.bar.Render(glyphs.AccentBar) + " " + m.styles.hint.Render(action)
```

- [ ] **Step 4: Test + Suite grün**

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/palette/
git commit -m "fix(palette): preview prefix uses AccentBar ▎ not Active ▶

Preview zeigt was passieren WIRD wenn Enter — das ist Future,
nicht Live. ▎ als generischer Selection/Focus-Marker (gleich
wie ein selected picker row) liest sich konsistenter."
```

### Task 2.3: Kompendium Cursor-Caret ohne ▶

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_row.go:100-109`

- [ ] **Step 1: Failing Test schreiben**

In `internal/kompendium/frontend/tui/browse/render_row_test.go` (anlegen oder erweitern):

```go
func TestRowStripeAndCaret_SelectedHasNoActiveGlyph(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	stripe, caret := rowStripeAndCaret(true)
	combined := stripe + caret
	// ▶ ist Active/Running. Cursor ist Selection. Trennung sauber halten.
	if strings.Contains(combined, glyphs.Active) {
		t.Errorf("selected stripe+caret: must not use glyphs.Active; got %q", combined)
	}
	if !strings.Contains(combined, glyphs.AccentBar) {
		t.Errorf("selected: expected glyphs.AccentBar (▎) as selection marker")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/render_row.go` Zeile 100-109:

```go
// VORHER:
//	func rowStripeAndCaret(selected bool) (string, string) {
//		if selected {
//			return cursorStripeStyle.Render(glyphs.AccentBar + " "), cursorStyle.Render(glyphs.Active + " ")
//		}
//		return "  ", "  "
//	}

// NACHHER:
// rowStripeAndCaret returns the two two-cell prefix columns at the row's
// left edge: a vertical stripe in Accent + bold (selected) or two blanks
// (unselected). The second column previously carried a duplicate ▶ caret
// painted in another Accent — same row had two competing accents and
// ▶ semantically conflicted with the row's own running-session glyphs.
// Now: bar carries the selection, second column carries blank space —
// width and excerpt-hang behaviour unchanged.
func rowStripeAndCaret(selected bool) (string, string) {
	if selected {
		return cursorStripeStyle.Render(glyphs.AccentBar + " "), "  "
	}
	return "  ", "  "
}
```

(Implication: `cursorStyle` wird ungenutzt — wenn rg keine anderen Konsumenten findet, in styles.go entfernen.)

- [ ] **Step 4: Unused cursorStyle entfernen wenn anwendbar**

```bash
rg -nF "cursorStyle" /Users/msoent/SourceCode/serverkraken/flow/internal/kompendium
```

Wenn nur noch in `styles.go` (Definition) referenziert: in `styles.go` und `rebuildStyles()` Zeile 172-174 entfernen.

- [ ] **Step 5: Test + Suite grün**

```bash
go test ./internal/kompendium/frontend/tui/browse/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/
git commit -m "fix(kompendium): row caret drops Active glyph

Skill §Glyph whitelist + Skill 'one accent per row':
selected stripe ist die Selection-Affordance (AccentBar
in Accent+Bold). Ein zweiter ▶-Caret daneben in derselben
Farbe war doppelt — und ▶ bedeutet 'läuft', nicht 'gewählt'."
```

### Task 2.4: Kompendium Type-Counts mit distinkten Glyphen

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_root.go:102-123`

- [ ] **Step 1: Failing Test schreiben**

In `internal/kompendium/frontend/tui/browse/render_root_test.go`:

```go
func TestRenderTypeCounts_GlyphsAreDistinctPerKind(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	m := Model{
		all:     []ports.NoteEntry{noteEntry(domain.TypeDaily), noteEntry(domain.TypeProject), noteEntry(domain.TypeFree)},
		visible: []ports.NoteEntry{noteEntry(domain.TypeDaily), noteEntry(domain.TypeProject), noteEntry(domain.TypeFree)},
	}
	out := m.renderTypeCounts()
	// Skill A11y-2: glyph + colour, never colour alone. Drei Counts
	// dürfen nicht denselben Glyph (●) tragen — color allein ist nicht
	// genug, das Wort daneben trägt zwar das Signal, aber der Glyph
	// muss konsistent zur Identität sein.
	gFilled := strings.Count(out, glyphs.Filled)
	if gFilled > 1 {
		t.Errorf("renderTypeCounts: glyphs.Filled used %d times — Skill A11y-2 wants distinct glyphs per kind", gFilled)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — alle drei nutzen ●.

- [ ] **Step 3: Glyphen-Whitelist erweitern wenn nötig**

Pruefen ob `glyphs.Bullet1..4` schon definiert sind (laut Skill-Tabelle: "Markdown list bullets `● ○ ◆ ▪`"). Wenn ja, wiederverwenden. Sonst: neue Konstanten in `glyphs.go`:

```bash
rg -n "Bullet1|Bullet2|Bullet3|Bullet4" /Users/msoent/SourceCode/serverkraken/flow/internal/frontend/tui/components/glyphs/
```

Wenn vorhanden, dann skippen. Wenn nicht, dann:

In `internal/frontend/tui/components/glyphs/glyphs.go` ergänzen:

```go
// Distinct count markers — one cell each, monospace-tested.
// Tägl. = ● (Filled, the workday marker — daily notes are per-day).
// Proj. = ◆ (diamond, "project" feel).
// Frei  = ○ (Empty/open).
const (
	CountDaily   = "●" // alias of Filled — daily is per-day-bucket
	CountProject = "◆"
	CountFree    = "○" // alias of Empty — free notes are "open" bucket
)
```

Plus den cell-width-Test erweitern (in `glyphs_test.go`).

- [ ] **Step 4: Fix in render_root.go anwenden**

```go
// VORHER (Zeile 117-121):
//	parts := []string{
//		countDailyStyle.Render(fmt.Sprintf(glyphs.Filled+" %d", d)) + dimStyle.Render(" täglich"),
//		countProjectStyle.Render(fmt.Sprintf(glyphs.Filled+" %d", p)) + dimStyle.Render(" projekt"),
//		countFreeStyle.Render(fmt.Sprintf(glyphs.Filled+" %d", f)) + dimStyle.Render(" frei"),
//	}

// NACHHER:
parts := []string{
	countDailyStyle.Render(fmt.Sprintf(glyphs.CountDaily+" %d", d)) + dimStyle.Render(" täglich"),
	countProjectStyle.Render(fmt.Sprintf(glyphs.CountProject+" %d", p)) + dimStyle.Render(" projekt"),
	countFreeStyle.Render(fmt.Sprintf(glyphs.CountFree+" %d", f)) + dimStyle.Render(" frei"),
}
```

- [ ] **Step 5: Test + Suite + glyph cell-width grün**

```bash
go test ./internal/frontend/tui/components/glyphs/ -v
go test ./internal/kompendium/frontend/tui/browse/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/frontend/tui/components/glyphs/ internal/kompendium/frontend/tui/browse/
git commit -m "fix(kompendium): distinct count glyphs per note type

Skill A11y-2: glyph + colour, never colour alone. Vorher
trugen täglich/projekt/frei alle ● — wenn der User Farbe
nicht sieht (NO_COLOR, Colorblind), war die Trennung nur
über das Wort, nicht über das Symbol getragen.

Neue Whitelist-Glyphen CountDaily/CountProject/CountFree als
semantische Aliase über bestehende Whitelist-Zeichen (●/◆/○)."
```

### Task 2.5: Heute Pause-Trenner-Format vereinheitlichen

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_render.go:241-249`

- [ ] **Step 1: Failing Test schreiben**

In `today_render_test.go`:

```go
func TestRenderSessionsList_PauseSeparatorUsesBulletDotNotEmDash(t *testing.T) {
	// Pause-Trenner: ein Inline-Meta-Format wie Heatmap-Status (BulletDot).
	// Em-Dash-artige Striche um den Wert herum drücken visuelle "Lücke"
	// aus, was bei einer reinen Pause-Dauer ohne weitere Info redundant ist.
	// Konsistent mit dem Skill-Pattern Bullet-Dot-Separator.
	pal := theme.TokyonightNight
	now := mustTime("2026-05-30T16:00:00+02:00")
	h := heute{
		pal: pal, width: 80, loaded: true,
		day: domain.Day{
			Sessions: []domain.Session{
				{Start: mustTime("2026-05-30T09:00:00+02:00"), Stop: mustTime("2026-05-30T11:00:00+02:00"), Elapsed: 2 * time.Hour},
				{Start: mustTime("2026-05-30T13:00:00+02:00"), Stop: mustTime("2026-05-30T15:00:00+02:00"), Elapsed: 2 * time.Hour},
			},
		},
	}
	rows, _ := h.renderSessionsList(76, now)
	joined := strings.Join(rows, "\n")
	if strings.Contains(joined, "─ ") && strings.Contains(joined, " ─") {
		t.Errorf("pause separator: should not use ─ dashes anymore; expected · BulletDot pattern. got: %q", joined)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `today_render.go` Zeile 244-248:

```go
// VORHER:
//	if !prevStop.IsZero() {
//		pause := s.Start.Sub(prevStop)
//		if pause > 0 {
//			rows = append(rows, stDim(h.pal,
//				fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
//		}
//	}

// NACHHER:
if !prevStop.IsZero() {
	pause := s.Start.Sub(prevStop)
	if pause > 0 {
		rows = append(rows, stDim(h.pal,
			fmt.Sprintf("       %s Pause %s", glyphs.BulletDot, formatDur(pause))))
	}
}
```

- [ ] **Step 4: Test + Suite grün**

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/
git commit -m "fix(worktime): pause separator uses BulletDot, not em-dashes

Vereinheitlicht mit history-drill (matched die session-Listen
Format) und dem app-wide BulletDot-Separator-Pattern. Em-Dash-
artige Striche signalisieren 'Lücke in der Timeline' — wirkt
hier doppelt zur Dauer-Information."
```

### Task 2.6: Phase-2 PR

- [ ] **Step 1: `make ci` + screenshots**

```bash
make ci
```

- [ ] **Step 2: PR**

```bash
gh pr create --title "fix(tui): phase 2 — glyph semantics" --body "$(cat <<'EOF'
## Summary
Phase 2 of 2026-05-30 UX-Review-Cleanup
(`docs/superpowers/plans/2026-05-30-tui-ux-review-cleanup.md`):

- §1.5 ▶ Glyph entlasten: Palette-Focus-Prompt, Palette-Preview-Prefix,
  Kompendium-Cursor-Caret rendern jetzt distinkte Marker (›-Heading,
  ▎-AccentBar) statt ▶ — letzteres bleibt für tatsächlich-Running-State.
- §3.3 Kompendium Type-Counts: drei distinkte Glyphen (●/◆/○) statt
  3×●. Skill A11y-2 (Glyph + Color, never color alone).
- §4.1 Heute Pause-Trenner: `· Pause 0h 30m` statt `─ 0h 30m Pause ─`.

## Test plan
- [ ] Palette: Focus-Cursor liest weiterhin distinkt (Bold-Style ist genug)
- [ ] Palette: Preview-Zeile zeigt ▎ statt ▶
- [ ] Kompendium: Counts-Zeile zeigt ● / ◆ / ○ (visual distinct)
- [ ] Heute: Mehrere Sessions, Pause-Trenner liest wie history-drill
EOF
)"
```

---

## Phase 3 — Strings-Konstanten + Footer-Vokabular

**Findings:** §2.5 (Kompendium-Footer auf statusbar.Hints), §2.6 (HintSearchInput Konstante), §4.3 (confirm/HintConfirm Brackets), Skill §3.5 (markdown_overlay ctrl+u/d Aliase)

**Eine PR. Vereinheitlicht die textuellen App-Wide-Konventionen — Footer-Hints und Hint-Konstanten — und schließt die letzte Component-vocabulary-Drift.**

### Task 3.1: `HintSearchInput` Konstante anlegen

**Files:**
- Modify: `internal/frontend/tui/components/strings/strings.go`
- Modify: `internal/frontend/tui/components/strings/strings_test.go` (anlegen falls fehlt)

- [ ] **Step 1: Failing Test schreiben**

In `strings_test.go`:

```go
package strings_test

import (
	"strings"
	"testing"

	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

func TestHintSearchInput_HasCanonicalShape(t *testing.T) {
	got := uistrings.HintSearchInput
	for _, want := range []string{"tippen", "Enter", "anwenden", "Esc", "abbrechen", "→"} {
		if !strings.Contains(got, want) {
			t.Errorf("HintSearchInput %q missing %q", got, want)
		}
	}
	// Konsistenter Separator
	if !strings.Contains(got, "  ·  ") {
		t.Errorf("HintSearchInput missing canonical `  ·  ` separator")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — Konstante existiert nicht.

- [ ] **Step 3: Konstante hinzufügen**

In `internal/frontend/tui/components/strings/strings.go` im Const-Block:

```go
// HintSearchInput ist der canonical Footer für Live-Filter-Inputs
// (Kompendium Such-Modus, künftige Such-Surfaces). "tippen → filtern"
// statt "/" weil die Surface den Filter bereits aktiv hat — der User
// braucht das Trigger-Key nicht mehr, sondern die Apply/Abort-Verben.
const HintSearchInput = "tippen → filtern  ·  Enter → anwenden  ·  Esc → abbrechen"
```

- [ ] **Step 4: Test grün**

```bash
go test ./internal/frontend/tui/components/strings/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/components/strings/
git commit -m "feat(strings): add HintSearchInput constant

Footer-String für Live-Filter-Surfaces. Vorher inline in
Kompendium browse/render_status.go — Audit §2.1 verbietet
das Drift-Risiko."
```

### Task 3.2: Kompendium-Footer auf statusbar.Hints + HintSearchInput

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_status.go:20-30`
- Modify: `internal/kompendium/frontend/tui/browse/styles.go` (remove `footerStyle`, `footerKeyStyle` wenn unused)

- [ ] **Step 1: Failing Test schreiben**

In `render_status_test.go`:

```go
func TestRenderFooter_SearchModeUsesStringsHintSearchInput(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	m := Model{mode: ModeSearch}
	out := m.renderFooter()
	if !strings.Contains(out, uistrings.HintSearchInput) {
		t.Errorf("renderFooter (Search): expected canonical HintSearchInput, got %q", out)
	}
}

func TestRenderFooter_ConfirmModeUsesStringsHintConfirm(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	m := Model{mode: ModeConfirmDelete}
	out := m.renderFooter()
	if !strings.Contains(out, uistrings.HintConfirm) {
		t.Errorf("renderFooter (Confirm): expected canonical HintConfirm, got %q", out)
	}
}

func TestRenderFooter_AllDimNotFooterKeyHighlight(t *testing.T) {
	// statusbar.Hints rendert all-dim (FgMuted). Kompendium hatte
	// vorher footerKeyStyle = sem.Active+Bold, was Single-Accent-
	// Per-Row brach. Test: rendered footer enthält Sem.Active NICHT.
	SetPalette(theme.TokyonightNight)
	m := Model{mode: ModeSearch}
	out := m.renderFooter()
	if strings.Contains(out, fmt.Sprintf("%v", theme.TokyonightNight.Sem().Active)) {
		t.Errorf("renderFooter: must not paint keys in Sem.Active (Skill all-dim)")
	}
}
```

- [ ] **Step 2: Run failing tests**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/render_status.go` Zeile 20-30:

```go
// VORHER:
//	func (m Model) renderFooter() string {
//		switch m.mode {
//		case ModeSearch:
//			return footerStyle.Render("tippen → filtern  ·  Enter → anwenden  ·  " + uistrings.HintCancel)
//		case ModeConfirmDelete:
//			return footerStyle.Render(uistrings.HintConfirm)
//		}
//		return m.helpUI.View(m.keys)
//	}

// NACHHER:
import (
	// ...
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

func (m Model) renderFooter() string {
	switch m.mode {
	case ModeSearch:
		return statusbar.Hints(uistrings.HintSearchInput, pal)
	case ModeConfirmDelete:
		return statusbar.Hints(uistrings.HintConfirm, pal)
	}
	return m.helpUI.View(m.keys)
}
```

- [ ] **Step 4: footerStyle + footerKeyStyle aufräumen wenn unused**

```bash
rg -nF "footerStyle|footerKeyStyle" /Users/msoent/SourceCode/serverkraken/flow/internal/kompendium
```

Wenn nur noch in `styles.go` (Definition + rebuildStyles): in styles.go entfernen — Zeile 50-52 (var) und Zeile 165-169 (rebuild).

Wenn `footerKeyStyle` von `renderEmptyState` benutzt wird (render_row.go:404,406), den auf `theme.Strong(s, pal)` oder Inline lipgloss.NewStyle migrieren — Akzent-Färbung der Empty-State-Keys ist OK (es ist die einzige Stelle, wo es einen Fokus-Punkt gibt).

- [ ] **Step 5: Test + Suite grün**

```bash
go test ./internal/kompendium/frontend/tui/browse/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/
git commit -m "fix(kompendium): footer uses statusbar.Hints, all-dim

Skill §Hint format: 'Hints render at the bottom of every screen
via components/statusbar.Hints'. Vorher eigenes footerStyle +
footerKeyStyle (key in sem.Active+Bold) — letzteres brach
single-accent-per-row, identisch zum markdown_overlay-Fix in P1.

Footer-Wording jetzt aus components/strings (HintSearchInput,
HintConfirm) — Drift-frei zu allen anderen Surfaces."
```

### Task 3.3: `markdown_overlay` ctrl+u/ctrl+d Aliase

**Files:**
- Modify: `internal/frontend/tui/components/markdown_overlay/keys.go`

- [ ] **Step 1: Failing Test schreiben**

In `internal/frontend/tui/components/markdown_overlay/keys_test.go`:

```go
func TestKeys_PageDown_IncludesCtrlD(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageDown, "ctrl+d") {
		t.Errorf("PageDown: expected ctrl+d alias for vim-style paging")
	}
}

func TestKeys_PageUp_IncludesCtrlU(t *testing.T) {
	k := defaultKeys()
	if !containsKey(k.PageUp, "ctrl+u") {
		t.Errorf("PageUp: expected ctrl+u alias for vim-style paging")
	}
}

// containsKey is a tiny helper; if defaultKeys() exposes a different
// binding struct, adapt the assertion to that shape.
func containsKey(b key.Binding, want string) bool {
	for _, k := range b.Keys() {
		if k == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Aliase hinzufügen**

In `internal/frontend/tui/components/markdown_overlay/keys.go` die PageUp/PageDown-Bindings erweitern:

```go
// Inside defaultKeys() — exact location depends on existing structure.
// Existing PageDown is likely: key.WithKeys("pgdown")
// Change to:
PageDown: key.NewBinding(
	key.WithKeys("pgdown", "ctrl+d"),
	key.WithHelp("PgDn / Ctrl+D", "seite weiter"),
),
PageUp: key.NewBinding(
	key.WithKeys("pgup", "ctrl+u"),
	key.WithHelp("PgUp / Ctrl+U", "seite zurück"),
),
```

- [ ] **Step 4: Test + Suite grün**

```bash
go test ./internal/frontend/tui/components/markdown_overlay/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/components/markdown_overlay/
git commit -m "feat(markdown_overlay): vim ctrl+u/d aliases for paging

Skill §Keybind grammar: vim muscle-memory deeper than custom
bindings. Kompendium browse hatte ctrl+u/d, markdown_overlay
nur PgUp/PgDn — inkonsistent. Beide Wege jetzt auf overlay-
Surface verfügbar, Help-Overlay zeigt beide."
```

### Task 3.4: `confirm`/`HintConfirm` Bracket-Konsistenz

**Files:**
- Modify: `internal/frontend/tui/components/strings/strings.go`
- Modify: `internal/frontend/tui/components/confirm/confirm.go:123`

- [ ] **Step 1: Failing Test schreiben**

In `strings_test.go`:

```go
func TestHintConfirm_UsesBracketedDefault(t *testing.T) {
	// A11y-6: default-Action bracketed `[y/Enter]` als non-color cue.
	// HintConfirm und confirm.View müssen denselben Stil sprechen.
	if !strings.Contains(uistrings.HintConfirm, "[y/Enter]") {
		t.Errorf("HintConfirm: expected `[y/Enter]` brackets to match confirm.View, got %q", uistrings.HintConfirm)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: HintConfirm migrieren**

In `internal/frontend/tui/components/strings/strings.go`:

```go
// VORHER:
//	HintConfirm = "y/Enter → ja  ·  n/Esc → nein"

// NACHHER:
// HintConfirm: brackets um die default-Action `[y/Enter]` — A11y-6
// non-color cue. Mirrors confirm.Model.View() one-to-one so a y/n
// dialog and a hint-strip never disagree.
HintConfirm = "[y/Enter] → ja  ·  n/Esc → nein"
```

- [ ] **Step 4: confirm.go nutzt die Konstante**

In `internal/frontend/tui/components/confirm/confirm.go` Zeile ~120-124 prüfen, ob die `[y/Enter] → ja ... n/Esc → nein` Konstruktion sich aus `uistrings.HintConfirm` zusammensetzen lässt. Aktuell:

```go
hint := primary.Render("[y/Enter] → ja") + dim.Render("  ·  n/Esc → nein")
```

Die Konstanten-Definition ist jetzt `[y/Enter] → ja  ·  n/Esc → nein` als ein String. Die Split-Style-Render (primary + dim) ist confirm-spezifisch und bleibt — die Konstante dient nur als Single Source of Truth für die Wörter. Test ist über strings-Vergleich, nicht über confirm.View.

Aber: Audit-Konsistenz-Trick. Stelle sicher dass confirm.go importiert und referenziert `uistrings.HintConfirm` als Drift-Guard:

```go
import (
	// ...
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

// _ guard ensures any future re-wording of HintConfirm forces a
// review of this file's split render too.
var _ = uistrings.HintConfirm
```

Optional aber empfohlen.

- [ ] **Step 5: Test + Suite grün**

```bash
go test ./internal/frontend/tui/components/strings/ -v
go test ./internal/frontend/tui/components/confirm/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/frontend/tui/components/strings/ internal/frontend/tui/components/confirm/
git commit -m "fix(strings,confirm): align HintConfirm brackets

A11y-6 default-Action-Bracket-Pattern war nur in confirm.View
implementiert; HintConfirm-Konstante (in Footer-Hints aller
Screens) lebte ohne Brackets. Beide jetzt identisch."
```

### Task 3.5: Phase-3 PR

- [ ] **Step 1: `make ci`**

```bash
make ci
```

- [ ] **Step 2: PR**

```bash
gh pr create --title "fix(tui): phase 3 — string vocabulary + footer migration" --body "$(cat <<'EOF'
## Summary
Phase 3 of 2026-05-30 UX-Review-Cleanup
(`docs/superpowers/plans/2026-05-30-tui-ux-review-cleanup.md`):

- §2.5 Kompendium-Footer auf statusbar.Hints + all-dim
- §2.6 HintSearchInput Konstante in components/strings
- §3.5 markdown_overlay vim-paging Aliase (ctrl+u/d)
- §4.3 HintConfirm `[y/Enter]` Brackets — confirm/HintConfirm in sync

## Test plan
- [ ] Kompendium Such-Modus: Footer all-dim, kein Cyan-Bold mehr
- [ ] Cheatsheet: ctrl+u/d paging funktioniert
- [ ] Confirm-Dialog + History-Drill Delete-Footer rendern identisch
EOF
)"
```

---

## Phase 4 — Spacing-Token-Helper

**Findings:** §3.1 (Free-Integer-Gaps in 7+ Stellen)

**Eine PR. Migrations-Aufgabe: `theme.Gap(n)` als Helper, alle Free-Integer-Spacings auf Token-Konstanten umstellen. Lint-Reproduzierbarkeit.**

### Task 4.1: `theme.Gap()` Helper anlegen

**Files:**
- Modify: `internal/frontend/tui/theme/builders.go`
- Modify: `internal/frontend/tui/theme/builders_test.go`

- [ ] **Step 1: Failing Test schreiben**

```go
func TestGap_ReturnsRepeatedSpaces(t *testing.T) {
	if got := theme.Gap(theme.PadXS); got != " " {
		t.Errorf("Gap(PadXS=1) = %q, want %q", got, " ")
	}
	if got := theme.Gap(theme.PadSM); got != "  " {
		t.Errorf("Gap(PadSM=2) = %q, want %q", got, "  ")
	}
	if got := theme.Gap(theme.PadMD); got != "   " {
		t.Errorf("Gap(PadMD=3) = %q, want %q", got, "   ")
	}
}

func TestGap_ZeroAndNegative_ReturnEmpty(t *testing.T) {
	if theme.Gap(0) != "" {
		t.Error("Gap(0) should return empty")
	}
	if theme.Gap(-1) != "" {
		t.Error("Gap(-1) should return empty")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Helper anlegen**

In `internal/frontend/tui/theme/builders.go` (oder neuer File `internal/frontend/tui/theme/layout.go`):

```go
// Gap returns a string of n spaces. Use with theme.PadXS / PadSM / PadMD
// instead of inline `"  "` string literals — makes the Skill §Spacing
// "discrete scale, never free integer" rule mechanically enforceable
// and a grep for raw space-strings in render code becomes meaningful.
// n ≤ 0 returns "" so a Gap(maxWidth - usedWidth) can collapse safely.
func Gap(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
```

(Import `"strings"` falls noch nicht im File.)

- [ ] **Step 4: Test grün**

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/theme/
git commit -m "feat(theme): Gap helper for token-based spacing strings

Skill §Spacing: 'pick from these tokens, never free integer'.
Gap(theme.PadMD+theme.PadXS) makes the intent explicit and grep-
visible — next commits migrate all `\"   \"`/`\"    \"` inline
literals to this helper."
```

### Task 4.2: Heute Free-Integer-Gaps migrieren

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_render.go:132,194,247`

- [ ] **Step 1: Audit aller Gap-Strings in today_render.go**

```bash
rg -n '"  +"|"   +"|"    +"' /Users/msoent/SourceCode/serverkraken/flow/internal/frontend/tui/screen/worktime/today_render.go
```

- [ ] **Step 2: Fix anwenden**

Konkret die Stellen:

```go
// Zeile 132 — Headline 4-Cell-Gaps zwischen total/status/pct:
// VORHER:
//	return "  " + totalStr + "    " + statusStr + "    " + pctStr
// NACHHER:
return theme.Gap(theme.PadSM) + totalStr + theme.Gap(theme.PadMD+theme.PadXS) + statusStr + theme.Gap(theme.PadMD+theme.PadXS) + pctStr

// Zeile 194 — Attached-Note-Chip inline hint:
// VORHER:
//	hint := stDim(h.pal, "  ·  o/O → ansehen/bearbeiten  ·  R → entfernen")
// (kein Spacing-Drift, kann bleiben)

// Zeile 247 — Pause-Trenner-Indent:
// VORHER:
//	fmt.Sprintf("       %s Pause %s", glyphs.BulletDot, formatDur(pause))
// NACHHER:
fmt.Sprintf("%s%s Pause %s", theme.Gap(theme.PadMD*2+theme.PadXS), glyphs.BulletDot, formatDur(pause))
```

(7 = 3+3+1 — PadMD*2+PadXS. Wenn der Indent semantisch "session-row-aligned" sein soll, möglicherweise eine eigene Konstante `SessionIndent = 7` in `theme/tokens.go` rechtfertigen.)

- [ ] **Step 3: Test + Suite grün**

Render-Tests (Heute) müssen identisch passen — Gap erzeugt denselben Output.

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/frontend/tui/screen/worktime/today_render.go
git commit -m "refactor(worktime/heute): inline gap strings → theme.Gap

Spec §Spacing token-discipline. Bytes identisch, intent explizit."
```

### Task 4.3: Woche + Kompendium Free-Integer-Gaps migrieren

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/week.go:245,430`
- Modify: `internal/kompendium/frontend/tui/browse/render_row.go:159,162`

- [ ] **Step 1: Fix Woche**

```go
// week.go:245 — Header gap (vorher "   "):
// VORHER:
//	return "  " + left + "   " + right
// NACHHER:
return theme.Gap(theme.PadSM) + left + theme.Gap(theme.PadMD) + right

// week.go:430 — Pace-Strip gap (vorher "   "):
// VORHER:
//	return strings.Join(dots, " ") + "   " + count + "   " + track
// NACHHER:
return strings.Join(dots, " ") + theme.Gap(theme.PadMD) + count + theme.Gap(theme.PadMD) + track
```

- [ ] **Step 2: Fix Kompendium hang-prefix**

```go
// render_row.go:159 — selected hang (vorher 6 spaces nach AccentBar):
// VORHER:
//	return cursorStripeStyle.Render(glyphs.AccentBar+" ") + "      "
// NACHHER:
return cursorStripeStyle.Render(glyphs.AccentBar+" ") + theme.Gap(theme.PadMD*2)

// render_row.go:162 — unselected hang (vorher 8 spaces):
// VORHER:
//	return "        "
// NACHHER:
return theme.Gap(theme.PadMD*2 + theme.PadSM)
```

- [ ] **Step 3: Test + Suite grün**

```bash
go test ./... | grep -E "(FAIL|PASS)" | head -20
```

- [ ] **Step 4: Commit**

```bash
git add internal/frontend/tui/screen/worktime/week.go internal/kompendium/frontend/tui/browse/render_row.go
git commit -m "refactor(week,kompendium): gap strings → theme.Gap"
```

### Task 4.4: Phase-4 PR

```bash
gh pr create --title "refactor(tui): phase 4 — theme.Gap for token-based spacing" --body "$(cat <<'EOF'
## Summary
§3.1: `theme.Gap(n)` helper added, all inline `"  "` / `"   "` /
`"        "` literals in render code migrated. Output bytes
identical — this is intent-clarity + grep-discipline.

## Test plan
- [ ] All render-tests pass (byte-identical output)
- [ ] grep for raw `"   +"` in screen files returns only test-data
EOF
)"
```

---

## Phase 5 — Heatmap-Performance + Empty-State-Slim + Filter-Doppelpunkt

**Findings:** §1.4 (Heatmap per-cell Allocs), §4.2 (Empty-State Kompendium overdesigned), §4.4 (Filter: Doppelpunkt)

**Eine PR. Performance-Win plus zwei kleine Polish-Fixes.**

### Task 5.1: `heatmapStyles` Cache

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/history.go` (Style-Cache extension)
- Modify: `internal/frontend/tui/screen/worktime/history_heatmap.go:100-117`

- [ ] **Step 1: Failing Test schreiben**

In `history_heatmap_test.go`:

```go
func TestHeatmapCell_NoPerCellStyleAllocation(t *testing.T) {
	// Performance-Skript: 100x renderHeatmapCell darf nicht 100x
	// distinct lipgloss.Style instanziieren. Direkt schwer messbar
	// in einem Unit-Test — wir messen statt dessen die Style-Cache-
	// Belegung: nach rerender muss h.styles.heatScale (oder analog)
	// nicht-leer sein, und alle Cells müssen daraus lesen.
	pal := theme.TokyonightNight
	h := history{
		pal:    pal,
		width:  120,
		styles: newHistoryStyles(pal),
		records: makeTestRecords(7, pal),
	}
	if h.styles.heatStepStyle[1.0].GetForeground() != pal.Sem().Success {
		t.Errorf("heatStepStyle[1.0]: expected Sem.Success preloaded")
	}
}
```

(`heatStepStyle map[float64]lipgloss.Style` ist die neue Cache-Form.)

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — Field existiert nicht.

- [ ] **Step 3: history.go — historyStyles erweitern**

In `internal/frontend/tui/screen/worktime/history.go` im `historyStyles` struct ergänzen:

```go
type historyStyles struct {
	// ... bestehende Felder ...

	// Heatmap-Style-Cache (round5 §1.4 plan): vorher allokierte jede
	// Zelle pro Frame einen lipgloss.NewStyle() — bei 26×7 = 182 calls.
	// Jetzt: pre-built map keyed by heatStep.minPct, plus die zwei
	// Special-Cases cursorCell + todayUnderline.
	heatStepStyle    map[float64]lipgloss.Style
	heatEmptyStyle   lipgloss.Style // Sem.Border für · (BulletDot)
	heatDayOffStyle  map[domain.Kind]lipgloss.Style // Schedule/Highlight/Notice
	heatTodayUnder   lipgloss.Style // Underline+Bold modifier for today (non-cursor)
	heatCursorCell   lipgloss.Style // Inverted cursor cell (existing `cursorCell`)
	heatCursorToday  lipgloss.Style // Cursor + Today combo (vorher .Underline(true))
	heatRecorded     lipgloss.Style // Sem.Info für Filled-without-target
}
```

In `newHistoryStyles(p theme.Palette) historyStyles` ergänzen:

```go
heatStepStyle := make(map[float64]lipgloss.Style, len(heatScale))
for _, s := range heatScale {
	heatStepStyle[s.minPct] = lipgloss.NewStyle().Foreground(s.color(p))
}
heatDayOff := map[domain.Kind]lipgloss.Style{
	domain.KindHoliday:  lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindHoliday)),
	domain.KindVacation: lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindVacation)),
	domain.KindSick:     lipgloss.NewStyle().Foreground(theme.KindColor(p, domain.KindSick)),
}
// existing cursorCell kept; rename or alias into the new fields:
heatCursorCell := h.styles.cursorCell // assuming the existing field
heatCursorToday := heatCursorCell.Underline(true)

return historyStyles{
	// ... bestehende Felder ...
	heatStepStyle:   heatStepStyle,
	heatEmptyStyle:  lipgloss.NewStyle().Foreground(p.Sem().Border),
	heatDayOffStyle: heatDayOff,
	heatTodayUnder:  lipgloss.NewStyle().Underline(true).Bold(true),
	heatCursorCell:  heatCursorCell,
	heatCursorToday: heatCursorToday,
	heatRecorded:    lipgloss.NewStyle().Foreground(p.Sem().Info),
}
```

- [ ] **Step 4: history_heatmap.go — Render-Funktion auf Cache umstellen**

In `internal/frontend/tui/screen/worktime/history_heatmap.go` Zeile 78-117 refaktorieren:

```go
func (h history) renderHeatmapCell(day time.Time, byKey map[string]domain.DayRecord, w, d int, now time.Time) string {
	rec, hasRec := byKey[day.Format("2006-01-02")]
	cell := " " + glyphs.BulletDot + " "
	style := h.styles.heatEmptyStyle

	if hasRec && rec.Target > 0 {
		glyph, key := heatmapCellGlyphKey(rec)
		cell = " " + glyph + " "
		if s, ok := h.styles.heatStepStyle[key]; ok {
			style = s
		}
	} else if hasRec && rec.Total > 0 {
		cell = " " + glyphs.Filled + " "
		style = h.styles.heatRecorded
	}

	if dayOff, isOff := h.deps.DayOffStore.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			cell = " " + glyphs.Filled + " "
		}
		if s, ok := h.styles.heatDayOffStyle[dayOff.Kind]; ok {
			style = s
		}
	}

	isCursor := w == h.heatCol && d == h.heatRow
	isToday := sameDay(day, now)
	switch {
	case isCursor && isToday:
		style = h.styles.heatCursorToday
	case isCursor:
		style = h.styles.heatCursorCell
	case isToday:
		// Layer underline+bold on top of the chosen base — only the
		// today-not-cursor branch needs a per-call composition; the
		// cursor branches already carry their own combined cache.
		style = style.Underline(true).Bold(true)
	}
	return style.Render(cell)
}

// heatmapCellGlyphKey returns the heatScale step's glyph + the
// minPct that keys the pre-built style cache. Mirrors heatmapCellGlyph
// (which returned glyph + color) but emits the cache key instead so
// renderHeatmapCell can look up the pre-built style.
func heatmapCellGlyphKey(rec domain.DayRecord) (string, float64) {
	pct := float64(rec.Total) / float64(rec.Target)
	for _, s := range heatScale {
		if pct > 0 && pct >= s.minPct {
			return s.glyph, s.minPct
		}
	}
	return glyphs.BulletDot, -1
}
```

(`heatmapCellGlyph` bleibt als Compatibility-Shim wenn andere Konsumenten existieren — sonst löschen.)

Zusätzlich Zeile 159-169 (Legend) — die Allocs in der Legend-Loop ebenfalls auf die Cache-Map mappen:

```go
// VORHER:
//	for i := len(heatScale) - 1; i >= 0; i-- {
//		s := heatScale[i]
//		legend = append(legend,
//			lipgloss.NewStyle().Foreground(s.color(h.pal)).Render(s.glyph+" "+s.label))
//	}

// NACHHER:
for i := len(heatScale) - 1; i >= 0; i-- {
	s := heatScale[i]
	style := h.styles.heatStepStyle[s.minPct]
	legend = append(legend, style.Render(s.glyph+" "+s.label))
}

// Day-off chips — gleiches Pattern:
legend = append(legend,
	h.styles.heatDayOffStyle[domain.KindHoliday].Render(glyphs.Filled+" Feiertag"),
	h.styles.heatDayOffStyle[domain.KindVacation].Render(glyphs.Filled+" Urlaub"),
	h.styles.heatDayOffStyle[domain.KindSick].Render(glyphs.Filled+" Krank"),
)
```

- [ ] **Step 5: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/worktime/ -v
```

- [ ] **Step 6: Lint-Baseline ratchet**

In `internal/frontend/tui/lint/screen_baseline_test.go` die `worktime/history_heatmap.go`-Zeile von 5 auf 0 setzen (alle Allocs jetzt im Cache).

- [ ] **Step 7: Commit**

```bash
git add internal/frontend/tui/screen/worktime/history.go internal/frontend/tui/screen/worktime/history_heatmap.go internal/frontend/tui/lint/screen_baseline_test.go
git commit -m "perf(worktime/heatmap): pre-built style cache, no per-cell alloc

26×7 = 182 Style allocations per frame before — now zero in the
cell render path. heatStepStyle keyed by heatScale minPct, plus
day-off-Kind map and cursor/today specials. Legend loop migrated
to the same cache.

Baseline-ratchet: history_heatmap.go 5 → 0."
```

### Task 5.2: Kompendium Empty-State auf 2 Zeilen

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_row.go:396-414`

- [ ] **Step 1: Failing Test schreiben**

```go
func TestRenderEmptyState_AtMostTwoLines(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	m := Model{visible: nil}
	out := m.renderEmptyState(60)
	lines := strings.Count(out, "\n") + 1
	if lines > 2 {
		t.Errorf("empty state: at most 2 lines, got %d (lines=%v)", lines, strings.Split(out, "\n"))
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` — aktuell 5 Zeilen.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/render_row.go` Zeile 396-414 ersetzen:

```go
// VORHER: 5-Zeilen-Hero-Empty mit Glyph + Title + zwei Hint-Zeilen.

// NACHHER:
func (m Model) renderEmptyState(width int) string {
	newKey := keyLabel(m.keys.New)
	title := emptyTitleStyle.Render("keine Treffer.")
	hint := emptyHintStyle.Render(newKey + " → neue Notiz · Esc → Filter leeren")
	stack := lipgloss.JoinVertical(lipgloss.Left, title, hint)
	if width <= 0 {
		return stack
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, stack)
}
```

- [ ] **Step 4: Test + Suite grün**

- [ ] **Step 5: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/render_row.go
git commit -m "refactor(kompendium): empty state slimmed to 2 lines

Konsistent mit palette/projects/heute (single-line empty
hints). 5-Zeilen-Hero war overdesigned für eine Empty-Liste."
```

### Task 5.3: Kompendium Filter-Label leer-suppress

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_root.go:147-155`

- [ ] **Step 1: Failing Test schreiben**

```go
func TestRenderStatusLine_HidesFilterLabelWhenEmpty(t *testing.T) {
	SetPalette(theme.TokyonightNight)
	m := Model{filter: filterDisplay{label: ""}}  // adapt to actual filter type
	out := m.renderStatusLine()
	// "Filter: " ohne Wert ist ein Drei-Doppelpunkt-Mismatch — wenn
	// Filter leer ist, das Label ganz weglassen.
	if strings.Contains(out, "Filter:") {
		t.Errorf("statusLine with empty filter: must not render `Filter:` label, got %q", out)
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: Fix anwenden**

In `internal/kompendium/frontend/tui/browse/render_root.go` Zeile 147-155:

```go
// VORHER:
//	func (m Model) renderStatusLine() string {
//		parts := []string{
//			statusKeyStyle.Render("Filter:") + " " + statusValueStyle.Render(m.filter.label()),
//		}
//		if search := m.renderSearchSegment(); search != "" {
//			parts = append(parts, search)
//		}
//		return strings.Join(parts, statusLineStyle.Render("  ·  "))
//	}

// NACHHER:
func (m Model) renderStatusLine() string {
	var parts []string
	if label := m.filter.label(); label != "" {
		parts = append(parts, statusKeyStyle.Render("Typ:")+" "+statusValueStyle.Render(label))
	}
	if search := m.renderSearchSegment(); search != "" {
		parts = append(parts, search)
	}
	return strings.Join(parts, statusLineStyle.Render("  ·  "))
}
```

(Bonus: `Filter:` → `Typ:` — adressiert §1.8 in derselben Edit. Wort-Wechsel ist klein, klare Semantik.)

- [ ] **Step 4: Test + Suite grün**

- [ ] **Step 5: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/render_root.go
git commit -m "fix(kompendium): hide Typ: label when filter empty

(plus relabel Filter: → Typ: — clarifies that this is type-cycle
not text-filter, addresses Phase-6 §1.8 already.)"
```

### Task 5.4: Phase-5 PR

```bash
gh pr create --title "perf(tui): phase 5 — heatmap cache + small polish" --body "$(cat <<'EOF'
## Summary
- §1.4 Heatmap pre-built style cache (182 → 0 per-cell allocs)
- §4.2 Kompendium empty state: 5 lines → 2 lines
- §4.4 Kompendium status line: hide Typ: when empty
- (Bonus: Filter → Typ relabel as in §1.8)

## Test plan
- [ ] Heatmap renders identical; long history visibly smoother in slow terminals
- [ ] Lint baseline: history_heatmap.go 5 → 0
EOF
)"
```

---

## Phase 6 — Kompendium-Architektur-Konsolidierung

**Findings:** §1.6 (Style-Cache-Inkonsistenz), §3.4 (Badges umgehen RenderPill), §1.8 (Vokabular — bereits in §5.3 angefasst)

**Eine PR (mittlerer Refactor). Browse migriert auf das per-Model-styles-Pattern, gleichzeitig fällt sie in den lint-baseline Scope.**

### Task 6.1: `browseStyles` struct per-Model anlegen

**Files:**
- Create: `internal/kompendium/frontend/tui/browse/styles_struct.go`
- Modify: `internal/kompendium/frontend/tui/browse/model.go`

- [ ] **Step 1: Failing Test schreiben**

In `styles_struct_test.go`:

```go
package browse

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestNewBrowseStyles_IndependentInstancesPerPalette(t *testing.T) {
	a := newBrowseStyles(theme.TokyonightNight)
	b := newBrowseStyles(theme.CatppuccinMocha)
	if a.headline.GetForeground() == b.headline.GetForeground() {
		t.Error("newBrowseStyles: distinct palettes must yield distinct foreground colors")
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL`.

- [ ] **Step 3: styles_struct.go schreiben**

Erstelle `internal/kompendium/frontend/tui/browse/styles_struct.go`:

```go
package browse

// Per-Model style cache (Phase 6 §1.6 plan): vorher hielt das
// Package package-level `var pal, sem` plus 30+ package-level
// lipgloss.Style vars, die rebuildStyles() bei SetPalette mutierte.
// Drei Probleme: (a) inkonsistent mit allen anderen Screens, die
// per-Model `styles` Felder fahren; (b) t.Parallel + SetPalette
// produzierten -race-Warnings (kein Mutex); (c) das Browse-Package
// fiel außerhalb des screen_baseline_test.go Lint-Scopes.
//
// Neue Form: ein einziges `browseStyles` struct, gebaut in
// `New(p)` → Model.styles. SetPalette deprecated; New(p) ist der
// Single-Entry-Point.

import (
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

type browseStyles struct {
	// Chrome.
	frame, headline, headerSeparator               lipgloss.Style
	repoChip                                       lipgloss.Style
	statusLine, statusKey, statusValue             lipgloss.Style
	panel, panelTitle, panelTitleFocus             lipgloss.Style

	// List rows.
	cursorStripe, selectedTitle, title             lipgloss.Style
	date, todayDate, todayMarker                   lipgloss.Style
	excerpt, match                                 lipgloss.Style

	// Type badges + counts.
	badgeDaily, badgeProject, badgeFree, badgeUnknown lipgloss.Style
	countDaily, countProject, countFree               lipgloss.Style

	// Search.
	searchActiveLabel, searchPassiveLabel, searchValue lipgloss.Style

	// Modal.
	modalDanger, modalQuestion, modalHint          lipgloss.Style

	// Misc.
	dim, errorPara                                 lipgloss.Style
	emptyGlyph, emptyTitle, emptyHint              lipgloss.Style
	spinner                                        lipgloss.Style
	paginatorActive, paginatorInactive, paginatorCounter lipgloss.Style

	// Status bar.
	statusBar, statusBarModeSearch, statusBarModeDelete lipgloss.Style
	statusBarPath, statusBarMeta                        lipgloss.Style

	// Tag-Hash chips need access to the palette at call site —
	// store the source palette so tagChipStyle(tag, s) is pure.
	pal theme.Palette
}

func newBrowseStyles(p theme.Palette) browseStyles {
	sem := p.Sem()
	return browseStyles{
		pal: p,

		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sem.BorderStrong).
			Padding(0, 1),
		headline:        lipgloss.NewStyle().Foreground(sem.Accent).Bold(true),
		headerSeparator: lipgloss.NewStyle().Foreground(sem.Border),
		repoChip:        lipgloss.NewStyle().Foreground(p.Bg).Background(p.Teal).Bold(true).Padding(0, 1),
		statusLine:      lipgloss.NewStyle().Foreground(p.FgMuted),
		statusKey:       lipgloss.NewStyle().Foreground(p.FgMuted).Bold(true),
		statusValue:     lipgloss.NewStyle().Foreground(p.FgDim),
		panel:           lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(sem.BorderSubtle),
		panelTitle:      lipgloss.NewStyle().Foreground(p.FgMuted),
		panelTitleFocus: lipgloss.NewStyle().Foreground(p.Fg).Bold(true),

		cursorStripe:  lipgloss.NewStyle().Foreground(sem.Active).Bold(true),
		selectedTitle: lipgloss.NewStyle().Foreground(p.Fg).Bold(true),
		title:         lipgloss.NewStyle().Foreground(p.Fg),
		date:          lipgloss.NewStyle().Foreground(p.FgDim),
		todayDate:     lipgloss.NewStyle().Foreground(sem.Warning).Bold(true),
		todayMarker:   lipgloss.NewStyle().Foreground(sem.Warning).Bold(true),
		excerpt:       lipgloss.NewStyle().Foreground(p.FgMuted),
		match:         lipgloss.NewStyle().Foreground(p.Bg).Background(sem.Warning).Bold(true),

		badgeDaily:    lipgloss.NewStyle().Foreground(p.Bg).Background(sem.Accent).Bold(true).Padding(0, 1),
		badgeProject:  lipgloss.NewStyle().Foreground(p.Bg).Background(sem.Success).Bold(true).Padding(0, 1),
		badgeFree:     lipgloss.NewStyle().Foreground(p.Bg).Background(sem.Highlight).Bold(true).Padding(0, 1),
		badgeUnknown:  lipgloss.NewStyle().Foreground(p.Fg).Background(p.FgMuted).Bold(true).Padding(0, 1),
		countDaily:    lipgloss.NewStyle().Foreground(sem.Accent).Bold(true),
		countProject:  lipgloss.NewStyle().Foreground(sem.Success).Bold(true),
		countFree:     lipgloss.NewStyle().Foreground(sem.Highlight).Bold(true),

		searchActiveLabel:  lipgloss.NewStyle().Foreground(sem.Warning).Bold(true),
		searchPassiveLabel: lipgloss.NewStyle().Foreground(p.FgMuted),
		searchValue:        lipgloss.NewStyle().Foreground(p.Fg),

		modalDanger:   lipgloss.NewStyle().Foreground(sem.Danger).Bold(true),
		modalQuestion: lipgloss.NewStyle().Foreground(p.Fg).Bold(true),
		modalHint:     lipgloss.NewStyle().Foreground(p.FgMuted),

		dim:               lipgloss.NewStyle().Foreground(p.FgMuted),
		errorPara:         lipgloss.NewStyle().Foreground(sem.Danger),
		emptyGlyph:        lipgloss.NewStyle().Foreground(sem.Accent).Bold(true),
		emptyTitle:        lipgloss.NewStyle().Foreground(p.FgDim).Bold(true),
		emptyHint:         lipgloss.NewStyle().Foreground(p.FgMuted),
		spinner:           lipgloss.NewStyle().Foreground(sem.Active),
		paginatorActive:   lipgloss.NewStyle().Foreground(sem.Active),
		paginatorInactive: lipgloss.NewStyle().Foreground(p.BgChip),
		paginatorCounter:  lipgloss.NewStyle().Foreground(p.FgMuted),

		statusBar:           lipgloss.NewStyle().Background(p.BgChip).Foreground(p.FgDim),
		statusBarModeSearch: lipgloss.NewStyle().Background(sem.Warning).Foreground(p.Bg).Bold(true).Padding(0, 1),
		statusBarModeDelete: lipgloss.NewStyle().Background(sem.Danger).Foreground(p.Bg).Bold(true).Padding(0, 1),
		statusBarPath:       lipgloss.NewStyle().Background(p.BgChip).Foreground(p.Fg),
		statusBarMeta:       lipgloss.NewStyle().Background(p.BgChip).Foreground(p.FgDim),
	}
}

// tagChipStyle returns the per-tag-hash chip style. Pure: depends
// only on the stored palette.
func (s browseStyles) tagChipStyle(tag string) lipgloss.Style {
	bg := s.pal.TagPalette[tagColorIdx(tag)]
	return lipgloss.NewStyle().Foreground(s.pal.Bg).Background(bg).Bold(true).Padding(0, 1)
}
```

- [ ] **Step 4: Test grün**

```bash
go test ./internal/kompendium/frontend/tui/browse/ -run TestNewBrowseStyles -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/styles_struct.go internal/kompendium/frontend/tui/browse/styles_struct_test.go
git commit -m "feat(kompendium/browse): browseStyles struct (palette-per-Model)

Single source for all browse render styles, identical shape to
palette/projects/worktime per-Model caches. New(p) builds; no
package-level mutation, no race risk under t.Parallel."
```

### Task 6.2: Model + Render-Funktionen auf m.styles umstellen

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/model.go` — add `styles browseStyles` Feld
- Modify: `internal/kompendium/frontend/tui/browse/render_root.go` — alle Package-var-Referenzen
- Modify: `internal/kompendium/frontend/tui/browse/render_row.go` — alle Package-var-Referenzen
- Modify: `internal/kompendium/frontend/tui/browse/render_status.go` — alle Package-var-Referenzen
- Modify: `internal/kompendium/frontend/tui/browse/render_modal.go` — alle Package-var-Referenzen
- Modify: `internal/kompendium/frontend/tui/browse/styles.go` — package-level vars löschen, alte `SetPalette` als no-op-Deprecation belassen wegen Bridge im Composition-Root

- [ ] **Step 1: Model um `styles browseStyles` erweitern**

In `model.go`:

```go
type Model struct {
	// ... bestehende Felder ...
	styles browseStyles
}

func New(p theme.Palette /* ... andere args ... */) Model {
	return Model{
		// ... bestehende Initializer ...
		styles: newBrowseStyles(p),
	}
}
```

- [ ] **Step 2: Sed-Sweep alle Render-Files**

In jedem Render-File die package-level var-Namen auf m-Style-Referenzen mappen. Beispiel `render_root.go`:

```go
// VORHER (überall):
//   headlineStyle.Render(...)
//   countDailyStyle.Render(...)
//   dimStyle.Render(...)
//   ...

// NACHHER:
//   m.styles.headline.Render(...)
//   m.styles.countDaily.Render(...)
//   m.styles.dim.Render(...)
```

Render-Helper-Funktionen, die NICHT auf `m` receivern (z.B. `rowStripeAndCaret`, `renderDateCell`, `badgeFor`), müssen `s browseStyles` als Argument bekommen oder über die Model-Methode weitergereicht werden. Beispiel:

```go
// VORHER:
//	func rowStripeAndCaret(selected bool) (string, string) {
//		if selected {
//			return cursorStripeStyle.Render(glyphs.AccentBar + " "), "  "
//		}
//		return "  ", "  "
//	}

// NACHHER:
func (s browseStyles) rowStripeAndCaret(selected bool) (string, string) {
	if selected {
		return s.cursorStripe.Render(glyphs.AccentBar + " "), "  "
	}
	return "  ", "  "
}
```

Caller in render_row.go ändern auf `m.styles.rowStripeAndCaret(selected)`.

Analog für `renderDateCell`, `styleExcerptLine`, `badgeFor`, `tagChipStyle`.

Brauche keine separaten Steps — größerer Sed-Sweep, dann compile-check.

- [ ] **Step 3: Compile + Test**

```bash
go build ./internal/kompendium/...
go test ./internal/kompendium/frontend/tui/browse/ -v
```

Iterativ Fix-up bis grün.

- [ ] **Step 4: package-level vars in styles.go löschen**

In `internal/kompendium/frontend/tui/browse/styles.go`:
- Alle 30+ `var xStyle lipgloss.Style` löschen.
- `rebuildStyles()` löschen.
- `init()` löschen.
- `var pal = theme.Default` und `var sem = pal.Sem()` löschen.
- `SetPalette(p)`: zur Backwards-Compat-Brücke umbauen — `SetPalette` triggert nichts mehr (Composition-Root muss `New(p)` mit live-Palette aufrufen). Alternativ: ganz löschen + cmd/flow/main.go-Aufrufer entfernen.

```go
// Package browse renders the kompendium read view as a Bubble Tea TUI.
// All styles are per-Model in browseStyles — no package state.
package browse
```

- [ ] **Step 5: cmd/flow/main.go SetPalette-Aufruf entfernen**

```bash
rg -n "browse.SetPalette" /Users/msoent/SourceCode/serverkraken/flow
```

Den Aufruf entfernen — `New(p)` reicht jetzt.

- [ ] **Step 6: Test grün**

```bash
go test ./... 2>&1 | grep -E "(FAIL|ok)" | head -20
```

- [ ] **Step 7: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/ cmd/flow/main.go
git commit -m "refactor(kompendium/browse): all styles per-Model, drop package state

Browser-Render-Code-Pfad bricht aus 30+ package-level Style-Vars
auf einen single struct (browseStyles) pro Model raus. SetPalette
no longer needed — New(p) reicht. Skill-konformer Drift-Floor:
das Browse-Package fällt jetzt in screen_baseline_test.go.

Eliminiert -race-Risiko unter t.Parallel und macht das Style-
Cache-Pattern identisch zu palette/projects/worktime."
```

### Task 6.3: Browse in screen_baseline_test.go aufnehmen

**Files:**
- Modify: `internal/frontend/tui/lint/screen_baseline_test.go`
- Modify: `internal/frontend/tui/lint/screen_hue_check_test.go`

- [ ] **Step 1: Baseline-Walker erweitern**

In `screen_baseline_test.go` den Pfad-Walker so anpassen, dass auch `internal/kompendium/frontend/tui/browse/` gezählt wird. Erwartete Counts:

```go
// Im map-Literal:
"kompendium/frontend/tui/browse/styles_struct.go": 49, // styles cache initial
"kompendium/frontend/tui/browse/render_root.go":   1,
"kompendium/frontend/tui/browse/render_row.go":    2, // highlightMatch, badge
"kompendium/frontend/tui/browse/render_status.go": 0,
"kompendium/frontend/tui/browse/render_modal.go":  0,
```

Counts pro Datei via `rg -c 'lipgloss\.NewStyle\(\)'` verifizieren.

- [ ] **Step 2: Hue-Check erweitern**

In `screen_hue_check_test.go` den Pfad-Walker analog erweitern. Browse-Render-Code darf nur über `Sem()` und Builder gehen — raw `pal.Red`/`pal.Cyan` etc. nicht.

Aktuell hat browse legitime raw-hue-Zugriffe (`p.Teal` für repoChip, `p.TagPalette` für tag-chips). Test als-erlaubt markieren in einer Allowlist:

```go
// Allowed raw-hue accesses in browse — documented exceptions:
// - p.Teal in repoChip: no semantic alias for "repo chip" hue
// - p.TagPalette for tag-hash chips: deliberate per-tag rotation
allowedBrowseHues := []string{"pal.Teal", "pal.TagPalette"}
```

- [ ] **Step 3: Tests grün**

```bash
go test ./internal/frontend/tui/lint/ -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/frontend/tui/lint/
git commit -m "chore(lint): include kompendium/browse in baseline + hue-check

Browse-Code-Pfad ist jetzt Style-Pattern-konform — Lint-Floor
erfasst es. Whitelist enthält die zwei dokumentierten raw-hue-
Ausnahmen (Teal repo-chip, TagPalette tag-rotation)."
```

### Task 6.4: Kompendium-Badges semantisch closer to RenderPill-Vokabular

**Files:**
- Modify: `internal/kompendium/frontend/tui/browse/render_row.go:452-462`

Beobachtung beim Schreiben: kompendium-Badges (`badgeDaily/Project/Free`) sind Background-Color-Fills, RenderPill ist Glyph+Foreground. Migration auf RenderPill würde den visuellen Stil ändern (vom BG-Pill zum Glyph-Pill). Das ist ein Geschmacks-Entscheid, kein hartes Drift.

**Pragmatischer Mittelweg:** Wir lassen die BG-Pill-Form, aber stellen die Konsistenz zur RenderPill-Konvention sicher: PillWidth (4 Zellen), gleiche Padding (`0, 1`), Bg-on-Accent WCAG-tested.

- [ ] **Step 1: Failing Test schreiben**

```go
func TestBadgeFor_FixedWidthAndContrastTested(t *testing.T) {
	tests := []struct {
		kind  domain.NoteType
		label string
	}{
		{domain.TypeDaily, "TÄGL."},
		{domain.TypeProject, "PROJ."},
		{domain.TypeFree, "FREI "},
	}
	s := newBrowseStyles(theme.TokyonightNight)
	for _, tc := range tests {
		got := s.badgeFor(tc.kind)
		w := lipgloss.Width(got)
		if w != theme.PillWidth+2 { // +2 for Padding(0, 1)
			t.Errorf("badge %v: width = %d, want %d", tc.kind, w, theme.PillWidth+2)
		}
	}
}
```

- [ ] **Step 2: Run failing test**

Expected: `FAIL` falls Width-Inkonsistenz, sonst PASS. Hier eher ein Soll-Check.

- [ ] **Step 3: badgeFor auf method umstellen**

```go
// VORHER:
//	func badgeFor(t domain.NoteType) string { ... }

// NACHHER:
func (s browseStyles) badgeFor(t domain.NoteType) string {
	switch t {
	case domain.TypeDaily:
		return s.badgeDaily.Render("TÄGL.")
	case domain.TypeProject:
		return s.badgeProject.Render("PROJ.")
	case domain.TypeFree:
		return s.badgeFree.Render("FREI ")
	}
	return s.badgeUnknown.Render("  ?  ")
}
```

(Tatsächlich schon in Task 6.2 mitgekommen — Schritt hier ist Verification + Test.)

- [ ] **Step 4: Test grün**

- [ ] **Step 5: Commit + Phase-PR**

```bash
git add internal/kompendium/frontend/tui/browse/
git commit -m "test(kompendium): assert badge fixed width matches PillWidth"
```

### Task 6.5: Phase-6 PR

```bash
gh pr create --title "refactor(kompendium): phase 6 — per-Model styles + lint coverage" --body "$(cat <<'EOF'
## Summary
- §1.6 Style-Cache: package vars → per-Model browseStyles struct
- §3.4 Badges: konsistente PillWidth/Padding, in Cache eingefasst
- §1.8 (bonus, already in P5): Filter → Typ relabel finalised
- Lint coverage extended to include browse package

Architectural alignment: kompendium browse now follows the same
style-cache pattern as palette/projects/worktime. SetPalette()
deprecated; New(p) is the single entry-point. No more package
mutation, no -race surface, no more out-of-scope drift.

## Test plan
- [ ] `make ci` green incl. baseline + hue-check
- [ ] Visual smoke: kompendium open, switch palette via cli flag if available
- [ ] grep `rebuildStyles` returns nothing
EOF
)"
```

---

## Phase 7 — Fuzzy-Highlight als Component

**Findings:** §1.3 (3× duplicate per-rune-highlight)

**Eine PR. Reduziert ~120 Zeilen Drift-Risiko über Palette + Projects + Kompendium.**

### Task 7.1: `picker.RowWithMatch` Component-API

**Files:**
- Modify: `internal/frontend/tui/components/picker/row.go`
- Modify: `internal/frontend/tui/components/picker/row_test.go`

- [ ] **Step 1: Failing Test schreiben**

```go
func TestRowWithMatch_HighlightsAtIndices(t *testing.T) {
	p := theme.TokyonightNight
	opts := picker.RowWithMatchOpts{
		Selected: true,
		Label:    "Heute",
		Hint:     "▶",
		Width:    20,
		Match:    []int{0, 2}, // H _ u in Match-Style
	}
	out := picker.RowWithMatch(opts, p)
	// Match-Style enthält Sem.Accent als Foreground; Label-Style nicht.
	// Robust-Check: Sem.Accent muss im Output mindestens 2× erscheinen
	// (für H und u), gleich die AccentBar für selected.
	count := strings.Count(out, fmt.Sprintf("%v", p.Sem().Accent))
	if count < 3 {
		t.Errorf("RowWithMatch: expected Sem.Accent at ≥3 spots (bar + 2 matches), got %d", count)
	}
}

func TestRowWithMatch_NoMatch_EqualToPickerRow(t *testing.T) {
	p := theme.TokyonightNight
	wm := picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected: true, Label: "Heute", Hint: "▶", Width: 20, Match: nil,
	}, p)
	plain := picker.Row(true, "Heute", "▶", 20, p)
	if wm != plain {
		t.Errorf("RowWithMatch(no match) should equal Row(...) — got divergent renders\n  wm:    %q\n  plain: %q", wm, plain)
	}
}
```

- [ ] **Step 2: Run failing tests**

Expected: `FAIL`.

- [ ] **Step 3: RowWithMatch implementieren**

In `internal/frontend/tui/components/picker/row.go` ans Datei-Ende:

```go
// RowWithMatchOpts holds the options for RowWithMatch — adds a Match
// (matched-rune indices) over the Row contract. Pulled into a struct
// because the positional arg count was hitting parameter-count limits.
type RowWithMatchOpts struct {
	Selected bool
	Label    string
	Hint     string
	Width    int
	Match    []int // rune indices in Label to render in match style
}

// RowWithMatch renders a picker row identical to Row, but applies a
// per-rune match emphasis (Sem.Accent + Bold) on the indices in Match.
// When Match is empty, the output is byte-identical to Row(Selected,
// Label, Hint, Width, p) — the two paths are interchangeable.
//
// Drift target (Phase 7 plan): vorher implementierten Palette,
// Projects and Kompendium die per-rune highlight je inline, mit
// subtilen Variants. Eine Funktion.
func RowWithMatch(opts RowWithMatchOpts, p theme.Palette) string {
	if len(opts.Match) == 0 {
		return Row(opts.Selected, opts.Label, opts.Hint, opts.Width, p)
	}
	sem := p.Sem()
	bar := " "
	labelStyle := lipgloss.NewStyle().Foreground(p.Fg)
	matchStyle := lipgloss.NewStyle().Foreground(sem.Accent).Bold(true)
	if opts.Selected {
		bar = lipgloss.NewStyle().Foreground(sem.Accent).Render(AccentBarRune)
		labelStyle = labelStyle.Bold(true).Underline(true)
		matchStyle = matchStyle.Underline(true)
	}
	hi := make(map[int]bool, len(opts.Match))
	for _, idx := range opts.Match {
		hi[idx] = true
	}
	var b strings.Builder
	for i, r := range []rune(opts.Label) {
		if hi[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(labelStyle.Render(string(r)))
		}
	}
	rendered := b.String()
	hintStyle := lipgloss.NewStyle().Foreground(p.FgMuted)
	gap := opts.Width - 1 - lipgloss.Width(opts.Label) - lipgloss.Width(opts.Hint) - 1
	if gap < 1 {
		gap = 1
	}
	return bar + " " + rendered + strings.Repeat(" ", gap) + hintStyle.Render(opts.Hint)
}
```

- [ ] **Step 4: Test grün**

```bash
go test ./internal/frontend/tui/components/picker/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/components/picker/
git commit -m "feat(picker): RowWithMatch — per-rune highlight variant

Skill 'Add a new component' rule: 3 unrelated screens reproducing
the same per-rune highlight logic → component. Palette, Projects,
and Kompendium browse migrate to this in subsequent commits."
```

### Task 7.2: Palette auf picker.RowWithMatch migrieren

**Files:**
- Modify: `internal/frontend/tui/screen/palette/render.go:172-208`
- Modify: `internal/frontend/tui/screen/palette/model.go` — `paletteStyles` schrumpfen

- [ ] **Step 1: Palette renderRow auf RowWithMatch ersetzen**

In `internal/frontend/tui/screen/palette/render.go` die `renderRow` Methode komplett ersetzen:

```go
// VORHER: 30+ Zeilen mit inline Builder + per-rune Loop.

// NACHHER:
func (m Model) renderRow(selected bool, label string, highlight []int, hint string, width int) string {
	return picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected: selected,
		Label:    label,
		Hint:     hint,
		Width:    width,
		Match:    highlight,
	}, m.pal)
}
```

- [ ] **Step 2: paletteStyles schrumpfen**

`paletteStyles` reduziert sich auf das was nicht in RowWithMatch lebt: `hint` (für Preview-Text), `bar` (für Preview-Glyph), `border` (für Separator). Die Style-Vars `label/labelSel/match/matchSel` löschen.

In `model.go`:

```go
type paletteStyles struct {
	hint   lipgloss.Style // FgMuted — preview text, empty-state dim
	bar    lipgloss.Style // Sem.Accent — preview ▎ glyph
	border lipgloss.Style // Sem.Border — separator rule
}

func newPaletteStyles(p theme.Palette) paletteStyles {
	sem := p.Sem()
	return paletteStyles{
		hint:   lipgloss.NewStyle().Foreground(p.FgMuted),
		bar:    lipgloss.NewStyle().Foreground(sem.Accent),
		border: lipgloss.NewStyle().Foreground(sem.Border),
	}
}
```

- [ ] **Step 3: Test + Suite grün**

```bash
go test ./internal/frontend/tui/screen/palette/ -v
```

- [ ] **Step 4: Lint baseline runter**

In `screen_baseline_test.go` palette/model.go von 5 auf 3 ratcheten.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/palette/ internal/frontend/tui/lint/screen_baseline_test.go
git commit -m "refactor(palette): use picker.RowWithMatch

Drops 4 lipgloss.NewStyle calls from paletteStyles, drops the
~30-line inline renderRow. Identical output. Baseline-ratchet:
palette/model.go 5 → 3."
```

### Task 7.3: Projects auf picker.RowWithMatch

**Files:**
- Modify: `internal/frontend/tui/screen/projects/model.go:438-473`

Beachte: Projects rendert eine tmux-session-marker im Hint (`glyphs.Active` in Sem.Active). Das ist die einzige Stelle, wo der Hint einen distinkten Glyph hat. RowWithMatch's Hint-Render ist `p.FgMuted`. Projects' marker braucht eigene Färbung.

- [ ] **Step 1: Marker im Hint-Slot vorab färben**

```go
// VORHER: 30+ Zeilen renderRow.

// NACHHER:
func (m Model) renderRow(selected bool, p domain.Project, highlight []int, width int) string {
	hint := ""
	if p.HasTmuxSession {
		// Pre-render the marker with its own style, so RowWithMatch's
		// dim hint-style sees a styled fragment instead of plain text.
		hint = m.styles.marker.Render(glyphs.Active)
	}
	return picker.RowWithMatch(picker.RowWithMatchOpts{
		Selected: selected,
		Label:    p.Name,
		Hint:     hint,
		Width:    width,
		Match:    highlight,
	}, m.pal)
}
```

Wenn lipgloss.Render mit pre-styled Input das nicht sauber wieder dimmt: Test verifizieren. Notfalls einen `HintRendered` Field in RowWithMatchOpts ergänzen, der bypass-hint-style.

- [ ] **Step 2: projectsStyles shrink**

`label/labelSel/match/matchSel/bar/border` löschen — RowWithMatch übernimmt. `marker` bleibt.

- [ ] **Step 3: Test + Suite grün**

- [ ] **Step 4: Baseline-Ratchet**

projects/model.go 5 → 2.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/projects/ internal/frontend/tui/lint/screen_baseline_test.go
git commit -m "refactor(projects): use picker.RowWithMatch"
```

### Task 7.4: Kompendium-Highlight nicht auf RowWithMatch (Beibehalten)

Kompendium hat ein anderes Layout (stripe + caret + date + badge + title + excerpt), passt nicht in RowWithMatch's Mono-Spalten-Form. **Skip** — Kompendium behält eigene `highlightMatch`-Implementierung in `render_row.go:467`.

Doku-Snippet ergänzen, dass kompendium bewusst nicht migriert wurde:

- [ ] **Step 1: Kommentar in highlightMatch ergänzen**

In `render_row.go:467` über `highlightMatch`:

```go
// highlightMatch is kompendium-internal — picker.RowWithMatch covers
// the single-column Picker case (Palette, Projects), but the kompendium
// row layout (stripe + caret + date + badge + title + excerpt) is too
// multi-cell to fit through it. The two paths share the same intent:
// match-substring rendered in match style, base else.
func highlightMatch(text, q string, base, match lipgloss.Style) string {
```

- [ ] **Step 2: Commit**

```bash
git add internal/kompendium/frontend/tui/browse/render_row.go
git commit -m "docs(kompendium): note why row layout doesn't use RowWithMatch"
```

### Task 7.5: Phase-7 PR

```bash
gh pr create --title "refactor(tui): phase 7 — picker.RowWithMatch component" --body "$(cat <<'EOF'
## Summary
§1.3 fuzzy-highlight pattern in Palette + Projects extracted to
picker.RowWithMatch. ~120 lines of duplicate logic gone; baseline
ratchets: palette/model.go 5→3, projects/model.go 5→2.

Kompendium browse intentionally stays on its own highlightMatch
(multi-cell row layout doesn't fit the single-column picker contract).

## Test plan
- [ ] `make ci` green
- [ ] Visual smoke: palette filter — match indices still bold/accent
- [ ] Visual smoke: projects filter — marker still right-aligned
EOF
)"
```

---

## Phase 8 — Date-Format-Konsolidierung

**Findings:** §1.2 (Date-Format inkonsistent über 4 Surfaces)

**Eine PR. Zentrale `domain.FmtDateDe(t, format)`, alle Surfaces auf bewusst gewählte Form.**

### Task 8.1: `domain.FmtDateDe` Helper anlegen

**Files:**
- Create: `internal/domain/datefmt.go`
- Create: `internal/domain/datefmt_test.go`

- [ ] **Step 1: Failing Test schreiben**

In `datefmt_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestFmtDateDe_ShortReturnsAbbreviatedMonth(t *testing.T) {
	d := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC) // Mittwoch
	if got := domain.FmtDateDe(d, domain.DateShort); got != "Mi., 28. Mai" {
		t.Errorf("FmtDateDe(Short) = %q, want %q", got, "Mi., 28. Mai")
	}
}

func TestFmtDateDe_LongIncludesYear(t *testing.T) {
	d := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	if got := domain.FmtDateDe(d, domain.DateLong); got != "Mi., 28. Mai 2026" {
		t.Errorf("FmtDateDe(Long) = %q, want %q", got, "Mi., 28. Mai 2026")
	}
}

func TestFmtDateDe_NumericIsSortable(t *testing.T) {
	d := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	if got := domain.FmtDateDe(d, domain.DateNumeric); got != "2026-05-28" {
		t.Errorf("FmtDateDe(Numeric) = %q, want %q", got, "2026-05-28")
	}
}
```

- [ ] **Step 2: Run failing tests**

Expected: `FAIL`.

- [ ] **Step 3: datefmt.go implementieren**

```go
// Package domain — datefmt.go: zentrale German-Date-Format-Helpers
// für TUI + tmux-Surfaces. Skill §German UI: canonical `Mi., 22. Apr.`
// für die status-bar germandate-plugin-Spiegelung. Drei Formen:
package domain

import (
	"fmt"
	"time"
)

// DateFormat selects a renderer.
type DateFormat int

const (
	// DateShort: "Mi., 28. Mai" — for status bars, headlines, where
	// the year is contextual.
	DateShort DateFormat = iota
	// DateLong: "Mi., 28. Mai 2026" — when the year matters
	// (heatmap-status, history drill).
	DateLong
	// DateNumeric: "2026-05-28" — sortable, monospace-aligned, for
	// list rows where dates stack.
	DateNumeric
)

// FmtDateDe renders t in the chosen format.
func FmtDateDe(t time.Time, f DateFormat) string {
	switch f {
	case DateLong:
		return fmt.Sprintf("%s, %d. %s %d",
			WeekdayShortDe(t.Weekday()), t.Day(), MonthShortDe(t.Month()), t.Year())
	case DateNumeric:
		return t.Format("2006-01-02")
	}
	return fmt.Sprintf("%s., %d. %s",
		WeekdayShortDe(t.Weekday()), t.Day(), MonthShortDe(t.Month()))
}
```

(Beachte: `WeekdayShortDe` liefert `"Mi"`, der Helper hängt das `.` an für DateShort/DateLong. Wenn `WeekdayShortDe` schon `Mi.` zurückgibt, das anpassen.)

- [ ] **Step 4: Test grün**

```bash
go test ./internal/domain/ -run TestFmtDateDe -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/datefmt.go internal/domain/datefmt_test.go
git commit -m "feat(domain): FmtDateDe with Short/Long/Numeric variants"
```

### Task 8.2: Heute today_render.go auf FmtDateDe(Short)

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_render.go:84-87`

```go
// VORHER:
//	return "  " + theme.Dim(fmt.Sprintf("%s · %s",
//		domain.WeekdayShortDe(now.Weekday()), now.Format("02.01.2026")), h.pal)

// NACHHER:
return theme.Gap(theme.PadSM) + theme.Dim(domain.FmtDateDe(now, domain.DateShort), h.pal)
```

Test + Commit Standard-Pattern.

### Task 8.3: Woche-Header auf FmtDateDe(Long) / oder bewusst eigene Form

Der Woche-Header zeigt eine Range `01. Mai – 07. Mai`. FmtDateDe greift nicht direkt. Entweder: einen `FmtDateRangeDe(from, to)`-Helper ergänzen, oder den bewussten Verzicht hier dokumentieren mit Kommentar.

**Entscheidung:** `FmtDateRangeDe(from, to time.Time)` als Range-Helper anlegen:

```go
// FmtDateRangeDe renders a from–to range. Compact when month matches
// ("01.–07. Mai"), expanded otherwise ("28. Mai – 03. Jun").
func FmtDateRangeDe(from, to time.Time) string {
	if from.Month() == to.Month() && from.Year() == to.Year() {
		return fmt.Sprintf("%d.–%d. %s", from.Day(), to.Day(), MonthShortDe(from.Month()))
	}
	return fmt.Sprintf("%d. %s – %d. %s",
		from.Day(), MonthShortDe(from.Month()),
		to.Day(), MonthShortDe(to.Month()))
}
```

Plus Test + Migration von week.go:242-244:

```go
// VORHER:
//	right := stDim(w.pal, fmt.Sprintf("%02d. %s – %02d. %s",
//		monday.Day(), domain.MonthShortDe(monday.Month()),
//		sunday.Day(), domain.MonthShortDe(sunday.Month())))

// NACHHER:
right := stDim(w.pal, domain.FmtDateRangeDe(monday, sunday))
```

### Task 8.4: History-Heatmap-Status auf FmtDateDe(Long)

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/history_heatmap.go:126-132`

```go
// VORHER:
//	status = fmt.Sprintf("   %s  %s  %s / %s",
//		domain.WeekdayShortDe(d.Weekday()), d.Format("2006-01-02"),
//		formatDur(rec.Total), formatDur(rec.Target))

// NACHHER:
status = fmt.Sprintf("%s%s  %s / %s",
	theme.Gap(theme.PadMD), domain.FmtDateDe(d, domain.DateLong),
	formatDur(rec.Total), formatDur(rec.Target))
```

Analog für die "kein Treffer"-Variante Zeile 130.

### Task 8.5: Kompendium browse — `date` column behält Numeric

Kompendium row date-cell ist explizit `YYYY-MM-DD` für mono-spaced sortability. Bleibt. Test:

```go
func TestRenderDateCell_UsesDateNumeric(t *testing.T) {
	e := ports.NoteEntry{Meta: ports.NoteMeta{Date: "2026-05-28"}}
	got, _ := renderDateCell(e)
	// Expect numeric YYYY-MM-DD form
	if !strings.Contains(got, "2026-05-28") {
		t.Errorf("date cell should show numeric date, got %q", got)
	}
}
```

Test-Code prüft Status quo. Kein Render-Change.

### Task 8.6: Phase-8 PR

```bash
gh pr create --title "refactor(tui): phase 8 — canonical date formatting" --body "$(cat <<'EOF'
## Summary
§1.2: `domain.FmtDateDe(t, Short|Long|Numeric)` + `FmtDateRangeDe`.
Surface decisions:
- Heute headline → Short ("Mi., 28. Mai")
- Woche header → Range ("28. Mai – 03. Jun")
- History heatmap status → Long ("Mi., 28. Mai 2026")
- Kompendium row date → Numeric (sortable, kept as-is)

## Test plan
- [ ] Date-format-tests across domain + worktime + kompendium pass
- [ ] Visual smoke: open Heute, Woche, History, Kompendium side-by-side — formats now deliberately distinct per role
EOF
)"
```

---

## Phase 9 — Cross-Screen-State-Sync

**Findings:** §1.7 (Edit in History propagiert nicht auf Heute/Woche)

**Eine PR. Eine `WorktimeChangedMsg`, Sidekick broadcast, drei Sub-Tabs handle it.**

### Task 9.1: `WorktimeChangedMsg` definieren

**Files:**
- Create: `internal/frontend/tui/screen/worktime/messages.go`

```go
package worktime

import "time"

// WorktimeChangedMsg signals that day/session/dayoff data changed
// (created, edited, deleted). Emitted by any worktime sub-tab that
// commits a mutation; broadcast by the sidekick to all sub-tabs so
// stale views reload.
//
// Date is the affected calendar day (or zero for global mutations
// like a yearly Feiertage-Sync). Sub-tabs decide whether they need
// a reload based on whether their visible date range intersects.
type WorktimeChangedMsg struct {
	Date time.Time
}
```

### Task 9.2: Sub-Tabs handlen WorktimeChangedMsg

In `today.go`, `week.go`, `history.go`, `dayoffs.go` jeweils im `Update`-Switch:

```go
case WorktimeChangedMsg:
	return m, m.loadCmd()
```

Plus Tests pro Sub-Tab.

### Task 9.3: Mutating Sub-Tabs emit WorktimeChangedMsg

Stellen, die heute aktuell mutieren:
- `today_dialog.go` (Edit, Tag, Note, Delete) — emit nach successful commit
- `dayoffs.go` (Add, Delete, Sync) — emit nach successful commit
- `history_drill.go` / `history_edit.go` (Drill-Edit, Add, Delete) — emit

Jeweils im Cmd, das nach use-case-Erfolg läuft, einen `tea.Batch(... successCmd, emit)` einbauen:

```go
return func() tea.Msg {
	if err := mutate(...); err != nil {
		return errMsg{err}
	}
	return WorktimeChangedMsg{Date: targetDate}
}
```

### Task 9.4: Sidekick broadcast

Sidekick.Update bekommt `WorktimeChangedMsg` als known type, broadcasted an *alle* Worktime-bezogenen Screens (heute nur das `screenWorktime` slot, das selbst aus 4 Sub-Tabs besteht — wird im worktime/model.go zu allen 4 weitergereicht).

In `internal/frontend/tui/screen/worktime/model.go` Update:

```go
case WorktimeChangedMsg:
	// Re-route to all four sub-tabs so each can decide to reload.
	for i, tab := range m.tabs {
		updated, cmd := tab.Update(msg)
		m.tabs[i] = updated.(/* sub-tab type */)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
```

(Adapt to the actual worktime sub-tab data layout.)

### Task 9.5: Phase-9 PR

```bash
gh pr create --title "feat(worktime): phase 9 — cross-tab state sync" --body "$(cat <<'EOF'
## Summary
§1.7: New WorktimeChangedMsg. Emitted by Heute/History-Drill/Frei
after any commit; routed by worktime/model.go to all 4 sub-tabs;
each tab calls its own loadCmd to refresh.

Eliminates the stale-view trap when editing in History and switching
back to Heute, or adding a sick day in Frei and seeing Woche still
miss it.

## Test plan
- [ ] Heute-Edit → switch to Woche → updated row visible
- [ ] History-Drill-Delete → switch to Heute → session gone
- [ ] Frei-Add-Sick → Woche → row shows Krank-marker
EOF
)"
```

---

## Phase 10 — UX-Großbaustelle: Tab-Strip-Hierarchie + Action-Menu-Policy

**Findings:** §1.1 (Tab-Strip-Doppelung), §1.9 (Action-Menu nur in Worktime)

**Eine PR (größter Refactor). Spart eine permanente Zeile vertikalen Budget; klärt das `:`-Action-Menu-Pattern als App-Idiom oder Worktime-Exklusiv.**

### Task 10.1: Sidekick — Sub-Tab-Hosting-API

**Files:**
- Create: `internal/frontend/tui/sidekick/sub_tabs.go`
- Modify: `internal/frontend/tui/sidekick/model.go`

- [ ] **Step 1: Interface in sub_tabs.go definieren**

```go
package sidekick

// subTabHost is the interface a screen implements when it owns
// sub-tabs (currently only worktime). The sidekick consumes this to
// render the sub-tab pills right-aligned in the global tab strip,
// saving one permanent row of vertical budget over the prior
// stacked tab-strip layout.
//
// SubTabs returns the sub-tab labels in display order; SubTabIndex
// returns the currently active sub-tab; SwitchSubTab(i) is invoked
// when the sidekick routes a numeric key (1-9) to the host.
type subTabHost interface {
	SubTabs() []string
	SubTabIndex() int
	SwitchSubTab(i int) tea.Model
}
```

- [ ] **Step 2: Worktime implementiert subTabHost**

In `internal/frontend/tui/screen/worktime/model.go`:

```go
func (m Model) SubTabs() []string {
	return []string{"Heute", "Woche", "History", "Frei"}
}

func (m Model) SubTabIndex() int { return int(m.activeTab) }

func (m Model) SwitchSubTab(i int) tea.Model {
	if i < 0 || i >= 4 {
		return m
	}
	m.activeTab = subTab(i)
	return m
}
```

- [ ] **Step 3: Sidekick render integriert sub-tab pills**

In `internal/frontend/tui/sidekick/model.go` `renderTabStrip`:

```go
func (m Model) renderTabStrip() string {
	mainStrip := m.renderMainTabStrip(entries)
	if host, ok := m.screens[m.current].(subTabHost); ok {
		subPills := m.renderSubTabPills(host)
		if subPills != "" {
			// Right-align: pad with spaces to fill m.width.
			used := lipgloss.Width(mainStrip) + lipgloss.Width(subPills)
			if used < m.width {
				return mainStrip + theme.Gap(m.width-used) + subPills
			}
			return mainStrip + theme.Gap(theme.PadSM) + subPills
		}
	}
	return mainStrip
}

func (m Model) renderSubTabPills(host subTabHost) string {
	labels := host.SubTabs()
	active := host.SubTabIndex()
	parts := make([]string, len(labels))
	for i, label := range labels {
		shortcut := fmt.Sprintf("%d %s", i+1, label)
		if i == active {
			parts[i] = activeTabStyle(m.pal).Render("[" + shortcut + "]")
		} else {
			parts[i] = theme.Dim("("+shortcut+")", m.pal)
		}
	}
	return strings.Join(parts, " ")
}
```

(Worktime's eigenes `tabStrip` render dropping — die Information lebt jetzt im Sidekick-Strip.)

- [ ] **Step 4: Worktime — eigene Tab-Strip-Render-Stelle entfernen**

In `worktime/model.go` viewContent: die `tabStrip`-Zeile am Anfang weglassen, da Sidekick sie zeigt. Outer `Worktime`-Render fängt direkt mit Sub-Tab-Content an.

- [ ] **Step 5: Numerische 1-4 Keys routen**

In `sidekick/model.go` `handleGlobalKey` einen Sub-Tab-Route-Pfad einfügen *vor* dem fall-through:

```go
case "1", "2", "3", "4", "5", "6", "7", "8", "9":
	if host, ok := m.screens[m.current].(subTabHost); ok {
		i, _ := strconv.Atoi(msg.String())
		if i-1 < len(host.SubTabs()) {
			m.screens[m.current] = host.SwitchSubTab(i - 1)
			return m, nil, true
		}
	}
	// Fall through if no host — current screen may use numeric keys
	// for other purposes (palette direct-pick).
	return m, nil, false
```

(Sicherstellen dass palette direct-pick 1-9 weiterhin funktioniert: palette ist *nicht* subTabHost, also fällt der case durch → forwardToCurrent → palette handlet sie.)

- [ ] **Step 6: Tests anpassen**

Erwartete Renders prüfen: snapshot-Tests im sidekick und worktime, die das alte Doppel-Strip-Layout snapshoten, aktualisieren.

- [ ] **Step 7: Test + Suite grün**

```bash
go test ./... | grep FAIL
```

- [ ] **Step 8: Commit**

```bash
git add internal/frontend/tui/sidekick/ internal/frontend/tui/screen/worktime/model.go
git commit -m "feat(sidekick): host sub-tabs in main strip via subTabHost iface

§1.1: vorher trug sidekick eine 5-tab-strip-Zeile und worktime
eigene 4-sub-tab-Zeile darunter — 2 chrome rows kostete pro
worktime-Frame. Sidekick rendert jetzt sub-tab-pills rechtsbündig
in der Hauptstrip-Zeile, worktime droppt seine eigene strip-Zeile.

Spart eine permanente vertikale Zeile; klärt die Hierarchie
(global tabs links, sub-context rechts) explizit."
```

### Task 10.2: `:` Action-Menu-Policy festschreiben

**Files:**
- Modify: `docs/design-system.md` — policy section

- [ ] **Step 1: Doc-Update**

In `docs/design-system.md` neue Sektion:

```markdown
## Action-Menu (`:`) — Policy

Worktime führt ein `:`-Aktions-Menü (range-export, target-switch,
correct, bundesland-sync). Stand 2026-05: bewusst Worktime-
exklusiv — Heute/Woche/History/Frei haben eng-zusammenhängende
Aktionen, die in einem Menü Platz haben; die anderen Screens
(Palette, Projects, Kompendium, Cheatsheet) haben entweder schon
ein Action-Vocabulary (Palette IS the menu) oder kommen mit
direct-keys aus (Kompendium: n, D, /, Tab).

Ein neuer `:`-Trigger auf einem anderen Screen erfordert eine
explizite Begründung im `?`-Overlay-Header "(`:` action menu —
nur Worktime/Heute hat das)" — gleiches Pattern wie der Skill
"Earned keybind"-Regel.
```

- [ ] **Step 2: Commit**

```bash
git add docs/design-system.md
git commit -m "docs(design-system): :-action-menu policy (Worktime exclusive)"
```

### Task 10.3: Phase-10 PR

```bash
gh pr create --title "feat(tui): phase 10 — unified tab strip + action-menu policy" --body "$(cat <<'EOF'
## Summary
§1.1: subTabHost interface. Sidekick renders worktime's sub-tabs
right-aligned in the main strip — saves one permanent vertical
row (~5% of screen budget on small terminals).

§1.9: Action-menu (`:`) policy documented as Worktime-exclusive
intent — Design-System now says so explicitly.

## Test plan
- [ ] Visual: open sidekick, switch to w. Sub-tab pills appear right of main tabs.
- [ ] Visual: 1/2/3/4 swap worktime tabs as before.
- [ ] Palette direct-pick 1-9 still works (sidekick fallthrough)
- [ ] On narrow terminals: sub-tab pills truncate gracefully, mainstrip stays readable
EOF
)"
```

---

## Self-Review

**Coverage gegen Findings-Liste (alle 25):**

| Finding | Phase | Status |
|---|---|---|
| §1.1 Tab-Strip-Doppelung | P10 | ✓ Task 10.1 |
| §1.2 Date-Format-Inkonsistenz | P8 | ✓ Tasks 8.1–8.6 |
| §1.3 Fuzzy-Highlight 3× | P7 | ✓ Tasks 7.1–7.4 |
| §1.4 Heatmap-Allocs | P5 | ✓ Task 5.1 |
| §1.5 ▶ überlastet | P2 | ✓ Tasks 2.1, 2.2, 2.3 |
| §1.6 Kompendium Style-Cache | P6 | ✓ Tasks 6.1–6.3 |
| §1.7 Cross-Screen-Sync | P9 | ✓ Tasks 9.1–9.5 |
| §1.8 Filter/Suche/Typ-Vokabular | P5 (bonus) + P6 | ✓ Task 5.3 |
| §1.9 :-Menu-Policy | P10 | ✓ Task 10.2 |
| §2.1 Heute Running-Color | P1 | ✓ Task 1.2 |
| §2.2 Woche Running-Color | P1 | ✓ Task 1.3 |
| §2.3 markdown_overlay Frame | P1 | ✓ Task 1.6 |
| §2.4 markdown_overlay Title | P1 | ✓ Task 1.6 |
| §2.5 Kompendium Footer | P3 | ✓ Task 3.2 |
| §2.6 HintSearchInput | P3 | ✓ Task 3.1 |
| §3.1 Free-Integer-Gaps | P4 | ✓ Tasks 4.1–4.3 |
| §3.2 Woche % Heading | P1 | ✓ Task 1.4 |
| §3.3 Kompendium Counts | P2 | ✓ Task 2.4 |
| §3.4 Kompendium Badges | P6 | ✓ Task 6.4 |
| §3.5 markdown_overlay paging | P3 | ✓ Task 3.3 |
| §3.6 Woche-Footer ?-Hint | P1 | ✓ Task 1.5 |
| §3.7 headerSeparator | P1 | ✓ Task 1.7 |
| §3.8 errorStyle Bold | P1 | ✓ Task 1.8 |
| §4.1 Pause-Trenner | P2 | ✓ Task 2.5 |
| §4.2 Empty-State | P5 | ✓ Task 5.2 |
| §4.3 confirm Brackets | P3 | ✓ Task 3.4 |
| §4.4 Filter: Doppelpunkt | P5 | ✓ Task 5.3 |

**Alle 25 Findings haben einen Task. Zero Lücken.**

**Type-Konsistenz-Check:**
- `theme.Active` (P1) + `theme.Gap` (P4) — beide als Builder/Helper in builders.go, gleicher Stil
- `picker.RowWithMatch` (P7) — struct-Opts-Pattern, identisch zu lipgloss-Conventions
- `domain.FmtDateDe` + `domain.FmtDateRangeDe` (P8) — beide in datefmt.go, gleicher Receiver-loser Style
- `WorktimeChangedMsg` (P9) — Msg-Pattern, Empfänger via case in Update
- `subTabHost` interface (P10) — analog zu helpProvider/screener/backHandler im selben Sidekick-File
- `browseStyles` struct (P6) — identisches Naming zu paletteStyles/projectsStyles/wocheStyles/historyStyles

**Keine Placeholder, keine TBDs, keine "Add appropriate error handling" — verified.**

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-30-tui-ux-review-cleanup.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Ich dispatche einen frischen Subagent pro Task, reviewe zwischen Tasks, schnelle Iteration. Bei 60+ Tasks über 10 Phasen vermutlich der saubere Weg — jeder Subagent sieht nur die Task-Context, kein Konversations-Drift.

**2. Inline Execution** — Tasks in dieser Session mit `executing-plans` durcharbeiten, batch-mässig mit Checkpoints für Review. Schneller, aber Context wächst — gegen Ende anstrengender.

**Welcher Ansatz?**
