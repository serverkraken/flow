# Unified Free-Day Glyphs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Vereinheitlichung der Free-Day-Glyphen über fünf Worktime-Surfaces (TUI Pace-Strip, Heatmap, Monatsraster, Frei-Tab, tmux Status-Segment): ein Glyph `○` für jeden freien Tag, das Kind ausschließlich über die Sem-Farbe codiert.

**Architecture:** Domain-Schicht (`internal/domain/status.go`) liefert neuen `kindStatusColor`-Helfer für tmux-Render. TUI-Schicht nutzt das schon vorhandene `kindColor()`/`kindStyle()`-Mapping. Die alten `dayOffGlyph`/`dayOffPaceGlyph`/`dayOffHeatmapGlyph`/`dotDayOffGlyph`/`bannerDayOffGlyph`-Helfer fallen weg. Whitelist-Glyphen `★/☼/✚` bleiben in `glyphs.go` für andere Konsumenten (Markdown-Renderer).

**Tech Stack:** Go 1.22, Bubbletea, Lipgloss, tmux status-right `#[fg=...]` Color-Codes.

**Spec:** `docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md`

---

## File Structure

**Modify (Domain):**
- `internal/domain/status.go` — `kindStatusColor` Helfer hinzu; `BuildStatusSegment` Banner-Stelle Zeile 67-69; `BuildPaceDots` Free-Day-Branch Zeile 211-216; Doc-Kommentar an `StatusPalette` Zeile 9-13; `bannerDayOffGlyph` Zeile 244-254 und `dotDayOffGlyph` Zeile 258-268 löschen.
- `internal/domain/status_test.go` — Erwartungen für Banner-Glyph + Pace-Dot-Glyph + per-Kind-Farben aktualisieren (Zeile 238, 374-403, 405-431).

**Modify (TUI):**
- `internal/frontend/tui/screen/worktime/week.go` — Zeile 374 (`renderPace` Free-Day-Branch); Zeile 474-479 (`dayOffPaceGlyph` löschen).
- `internal/frontend/tui/screen/worktime/history_month.go` — Zeile 160-162 (`renderMonthCell` Free-Day-Branch: Glyph + Farbe).
- `internal/frontend/tui/screen/worktime/history_heatmap.go` — Zeile 84-86 (`renderHeatmapCell` Free-Day-Branch: Farbe per Kind statt pauschal Info); Zeile 142 (`renderHeatmapLegend` 1-Chip → 3-Chip-Aufsplittung).
- `internal/frontend/tui/screen/worktime/helpers.go` — Zeile 51-69 (`dayOffGlyph` löschen).
- `internal/frontend/tui/screen/worktime/helpers_test.go` — Zeile 284-297 (`TestDayOffHeatmapGlyph` auf einheitliches `" ○ "` umstellen).
- `internal/frontend/tui/screen/worktime/dayoffs.go` — Zeile 517 (`renderKindSummary` Label in Kind-Farbe); Zeile 590-606 (`renderKindPicker` führender `○` Glyph).

**Create:**
- `internal/frontend/tui/screen/worktime/week_test.go` — `renderPace` Per-Kind-Farb-Assertions.
- `internal/frontend/tui/screen/worktime/history_month_test.go` — `renderMonthCell` Per-Kind-Farb-Assertions.
- `internal/frontend/tui/screen/worktime/history_heatmap_test.go` — `renderHeatmapCell` + `renderHeatmapLegend` Assertions.
- `internal/frontend/tui/screen/worktime/dayoffs_test.go` — `renderKindSummary` + `renderKindPicker` Assertions.

---

## Phase 1 — Domain Layer (tmux Status-Segment)

### Task 1: `kindStatusColor` Helfer hinzufügen

**Files:**
- Modify: `internal/domain/status.go` (neuer Helfer ans Datei-Ende, unterhalb der bestehenden `dotDayOffGlyph`)
- Modify: `internal/domain/status_test.go` (neuer Test)

- [ ] **Step 1: Failing Test schreiben**

In `internal/domain/status_test.go` ans Datei-Ende anhängen:

```go
func TestKindStatusColor_PerKind(t *testing.T) {
	p := pal()
	tests := []struct {
		kind domain.Kind
		want string
	}{
		{domain.KindHoliday, p.Cyan},
		{domain.KindVacation, p.Green},
		{domain.KindSick, p.Yellow},
		{domain.Kind("unknown"), p.Dim},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := domain.KindStatusColor(tc.kind, p); got != tc.want {
				t.Errorf("KindStatusColor(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
go test ./internal/domain/ -run TestKindStatusColor_PerKind -v
```

Expected: `FAIL` mit "undefined: domain.KindStatusColor".

- [ ] **Step 3: Helfer implementieren**

In `internal/domain/status.go` ans Datei-Ende anhängen (unterhalb `dotDayOffGlyph`):

```go
// KindStatusColor mappt Kind auf die tmux-StatusPalette. Holiday → Cyan
// (Info), Vacation → Green (Success), Sick → Yellow (der "Yellow"-Slot
// der StatusPalette wird via StatusPaletteFor mit Sem.Warning gefüttert
// und rendert damit denselben Orange-Hex wie die TUI). Unknown → Dim.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func KindStatusColor(k Kind, pal StatusPalette) string {
	switch k {
	case KindHoliday:
		return pal.Cyan
	case KindVacation:
		return pal.Green
	case KindSick:
		return pal.Yellow
	}
	return pal.Dim
}
```

- [ ] **Step 4: Run passing test**

```bash
go test ./internal/domain/ -run TestKindStatusColor_PerKind -v
```

Expected: `PASS` für alle vier Unter-Tests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/status.go internal/domain/status_test.go
git commit -m "feat(domain/status): kindStatusColor helper (Kind → tmux palette slot)

Holiday → Cyan, Vacation → Green, Sick → Yellow (Yellow-Slot trägt heute
schon Sem.Warning via StatusPaletteFor → rendert Orange-Hex). Spec:
docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md"
```

---

### Task 2: tmux Banner nutzt `○` + per-Kind-Farbe

**Files:**
- Modify: `internal/domain/status.go:67-69` (Banner-Stelle in `BuildStatusSegment`)
- Modify: `internal/domain/status.go:244-254` (`bannerDayOffGlyph` löschen)
- Modify: `internal/domain/status_test.go:228-241` (`TestBuildStatusSegment_DayOffBanner`)
- Modify: `internal/domain/status_test.go:405-431` (`TestBuildStatusSegment_DayOffBannerGlyphPerKind`)

- [ ] **Step 1: Bestehende Banner-Tests auf neues Verhalten umstellen**

In `internal/domain/status_test.go` ersetze `TestBuildStatusSegment_DayOffBanner` (ab Zeile 228) durch:

```go
func TestBuildStatusSegment_DayOffBanner(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	dayOff := &domain.DayOff{Date: now, Kind: domain.KindHoliday, Label: "Tag der Arbeit"}
	in := domain.StatusInputs{
		Now: now, Day: domain.Day{Target: 0}, Target: 0,
		DayOff:       dayOff,
		LookupDayOff: noLookup,
		Palette:      pal(),
	}
	got := domain.BuildStatusSegment(in)
	if !strings.Contains(got, "○ Tag der Arbeit") {
		t.Errorf("dayoff banner missing: %q", got)
	}
}
```

Und ersetze `TestBuildStatusSegment_DayOffBannerGlyphPerKind` (ab Zeile 405) durch:

```go
func TestBuildStatusSegment_DayOffBannerGlyphPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	p := pal()
	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, p.Cyan},
		{domain.KindVacation, p.Green},
		{domain.KindSick, p.Yellow},
		{domain.Kind("unknown"), p.Dim},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			d := &domain.DayOff{Date: now, Kind: tc.kind, Label: "Test"}
			in := domain.StatusInputs{
				Now: now, Day: domain.Day{}, Target: 0,
				DayOff:       d,
				LookupDayOff: noLookup,
				Palette:      p,
			}
			got := domain.BuildStatusSegment(in)
			want := tc.color + "]○ Test"
			if !strings.Contains(got, want) {
				t.Errorf("kind %q expected %q in banner: %q", tc.kind, want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/domain/ -run "TestBuildStatusSegment_DayOff" -v
```

Expected: `FAIL` — Banner zeigt noch `★ Tag der Arbeit` mit pauschal Cyan.

- [ ] **Step 3: BuildStatusSegment Banner-Stelle umstellen**

In `internal/domain/status.go` ersetze Zeile 67-69:

```go
	if in.DayOff != nil {
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s %s#[default]",
			in.Palette.Cyan, bannerDayOffGlyph(in.DayOff.Kind), in.DayOff.Label))
	}
```

durch:

```go
	if in.DayOff != nil {
		parts = append(parts, fmt.Sprintf("#[fg=%s]○ %s#[default]",
			KindStatusColor(in.DayOff.Kind, in.Palette), in.DayOff.Label))
	}
```

- [ ] **Step 4: `bannerDayOffGlyph` löschen**

In `internal/domain/status.go` Zeile 242-254 (ganzer Funktions-Block) löschen:

```go
// bannerDayOffGlyph picks a monospace TUI marker for each kind in the
// "[Frei: …]" banner. Default "·" when the kind is unknown.
func bannerDayOffGlyph(k Kind) string {
	switch k {
	case KindHoliday:
		return "★"
	case KindVacation:
		return "☼"
	case KindSick:
		return "✚"
	}
	return "·"
}
```

- [ ] **Step 5: Run tests, passing**

```bash
go test ./internal/domain/ -run "TestBuildStatusSegment_DayOff" -v
```

Expected: `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/status.go internal/domain/status_test.go
git commit -m "refactor(domain/status): banner uses ○ + kindStatusColor

[Frei: …] Banner: ★/☼/✚ + pauschal Cyan → ○ + per-Kind-Farbe
(Cyan/Green/Yellow). bannerDayOffGlyph entfernt."
```

---

### Task 3: tmux Pace-Dots nutzen `○` + per-Kind-Farbe

**Files:**
- Modify: `internal/domain/status.go:211-216` (Free-Day-Branch in `BuildPaceDots`)
- Modify: `internal/domain/status.go:256-268` (`dotDayOffGlyph` löschen)
- Modify: `internal/domain/status_test.go:374-403` (`TestBuildPaceDots_DayOffGlyphPerKind`)

- [ ] **Step 1: Test umstellen**

In `internal/domain/status_test.go` ersetze `TestBuildPaceDots_DayOffGlyphPerKind` (ab Zeile 374) durch:

```go
func TestBuildPaceDots_DayOffGlyphPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	fri := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	week := []domain.WeekDay{{Date: fri, Target: 8 * time.Hour}}
	p := pal()

	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, p.Cyan},
		{domain.KindVacation, p.Green},
		{domain.KindSick, p.Yellow},
		{domain.Kind("unknown"), p.Dim},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			lookup := func(d time.Time) (domain.DayOff, bool) {
				if d.Equal(fri) {
					return domain.DayOff{Date: d, Kind: tc.kind, Label: "T"}, true
				}
				return domain.DayOff{}, false
			}
			got := domain.BuildPaceDots(week, now, lookup, p)
			want := tc.color + "]○"
			if !strings.Contains(got, want) {
				t.Errorf("kind %q expected %q in %q", tc.kind, want, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/domain/ -run TestBuildPaceDots_DayOffGlyphPerKind -v
```

Expected: `FAIL` — alle Kinds rendern noch `★/☼/✚` in Cyan.

- [ ] **Step 3: BuildPaceDots umstellen**

In `internal/domain/status.go` ersetze Zeile 211-216 (Free-Day-Branch in `BuildPaceDots`):

```go
		if lookup != nil {
			if dayOff, isOff := lookup(d.Date); isOff {
				dots = append(dots, dot{dotDayOffGlyph(dayOff.Kind), pal.Cyan})
				any = true
				continue
			}
		}
```

durch:

```go
		if lookup != nil {
			if dayOff, isOff := lookup(d.Date); isOff {
				dots = append(dots, dot{"○", KindStatusColor(dayOff.Kind, pal)})
				any = true
				continue
			}
		}
```

- [ ] **Step 4: `dotDayOffGlyph` löschen**

In `internal/domain/status.go` Zeile 256-268 (ganzer Funktions-Block inkl. Doc-Kommentar) löschen:

```go
// dotDayOffGlyph mirrors bannerDayOffGlyph but with "○" as the unknown-
// kind fallback so the pace-dots row keeps its column rhythm.
func dotDayOffGlyph(k Kind) string {
	switch k {
	case KindHoliday:
		return "★"
	case KindVacation:
		return "☼"
	case KindSick:
		return "✚"
	}
	return "○"
}
```

- [ ] **Step 5: Run passing test**

```bash
go test ./internal/domain/ -run TestBuildPaceDots_DayOffGlyphPerKind -v
go test ./internal/domain/ -v
```

Expected: alles `PASS` (kein anderer Test verlässt sich auf `dotDayOffGlyph`).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/status.go internal/domain/status_test.go
git commit -m "refactor(domain/status): pace-dots use ○ + kindStatusColor

BuildPaceDots Free-Day-Branch: ★/☼/✚ + pauschal Cyan → ○ + per-Kind-Farbe.
dotDayOffGlyph entfernt."
```

---

### Task 4: Doc-Kommentar an `StatusPalette` aktualisieren

**Files:**
- Modify: `internal/domain/status.go:9-13` (Struct-Doc)

- [ ] **Step 1: Doc-Kommentar anpassen**

In `internal/domain/status.go` ersetze Zeile 9-13:

```go
// StatusPalette is the colour set used by tmux #[fg=...] markers in the
// status-right segment. Hex codes match the tokyonight defaults flow ships.
type StatusPalette struct {
	Green, Yellow, Red, Cyan, Dim string
}
```

durch:

```go
// StatusPalette is the colour set used by tmux #[fg=...] markers in the
// status-right segment. Hex codes match the tokyonight defaults flow ships.
//
// Slot-Semantik (Stand Spec 2026-05-12-unified-dayoff-glyphs):
//   Green  — Success: Werktag-Ziel erreicht, Urlaubs-Pace-Dot, Streak, ▲ Saldo
//   Yellow — Approaching/Warning (Doppelnutzung): Endspurt-Banner +
//            Krank-Pace-Dot. Wird via StatusPaletteFor mit Sem.Warning
//            gefüttert und rendert Orange-Hex (#ff9e64).
//   Red    — Danger: massive overtime im Banner
//   Cyan   — Info: laufende Session-Banner, Feiertag-Pace-Dot, Banner-Glyph
//   Dim    — idle/missed/unknown-kind
type StatusPalette struct {
	Green, Yellow, Red, Cyan, Dim string
}
```

- [ ] **Step 2: Build prüfen**

```bash
go build ./internal/domain/
```

Expected: keine Fehler.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/status.go
git commit -m "docs(domain/status): document StatusPalette slot semantics

Klarstellt die Doppelnutzung des Yellow-Slots (Endspurt-Approaching +
Krank-Pace-Dot) als bewusste Wahl nach dem Spec 2026-05-12-unified-
dayoff-glyphs."
```

---

## Phase 2 — TUI Render-Sites

### Task 5: `week.go renderPace` nutzt `glyphs.Empty`

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/week.go:374` (`renderPace` Free-Day-Branch)
- Modify: `internal/frontend/tui/screen/worktime/week.go:474-479` (`dayOffPaceGlyph` löschen)
- Create: `internal/frontend/tui/screen/worktime/week_test.go`

- [ ] **Step 1: Neuen Test-File anlegen**

Lege `internal/frontend/tui/screen/worktime/week_test.go` an. Wichtig: **`package worktime`** (internes Test-Package) — sonst sind `woche`, `newWoche` etc. nicht sichtbar.

```go
package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// TestRenderPace_FreeDayUsesEmptyGlyphPerKindColor pinnt fest, dass die
// Pace-Strip für jeden Free-Day-Kind den ○-Glyph (glyphs.Empty) emittiert
// und die Foreground-Farbe per Kind via Sem-Mapping unterscheidet.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func TestRenderPace_FreeDayUsesEmptyGlyphPerKindColor(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local) // Fri
	fri := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	pal := theme.TokyonightNight
	sem := pal.Sem()

	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, string(sem.Info)},
		{domain.KindVacation, string(sem.Success)},
		{domain.KindSick, string(sem.Warning)},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			dayoffs := testutil.NewFakeDayOffStore()
			if err := dayoffs.Add(domain.DayOff{Date: fri, Kind: tc.kind, Label: "T"}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			deps := Deps{
				DayOffStore: dayoffs,
				Clock:       &testutil.FixedClock{T: now},
			}
			w := newWoche(pal, deps)
			w.week = []domain.WeekDay{{Date: fri, Target: 8 * time.Hour}}
			w.loaded = true
			w.width = 80
			out := w.renderPace(now)
			if !strings.Contains(out, glyphs.Empty) {
				t.Errorf("renderPace should contain %q for free day, got: %q", glyphs.Empty, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("renderPace should colour kind %q with %q, got: %q", tc.kind, tc.color, out)
			}
		})
	}
}
```

(Achtung: `Deps` ist die unexportierte Struktur in `worktime`-Package. Felder `DayOffStore` und `Clock` reichen für `renderPace` — die anderen `Deps`-Felder sind in diesem Render-Pfad nicht angesprochen. Falls beim Compile-Lauf Felder fehlen, mit Zero-Werten ergänzen — der Test ruft kein `loadCmd` auf.)

- [ ] **Step 2: Run failing test**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderPace_FreeDayUsesEmptyGlyphPerKindColor -v
```

Expected: `FAIL` — emittiert noch `★/☼/✚`. Falls Compile-Error wegen `newRig`, schaue in den anderen `*_test.go` welche Helferin verfügbar ist und passe den Test an.

- [ ] **Step 3: `renderPace` umstellen**

In `internal/frontend/tui/screen/worktime/week.go` ersetze Zeile 373-374:

```go
		case isOff && !isWeekend:
			dots = append(dots, w.styles.kindStyle(dayOff.Kind).Render(dayOffPaceGlyph(dayOff.Kind)))
```

durch:

```go
		case isOff && !isWeekend:
			dots = append(dots, w.styles.kindStyle(dayOff.Kind).Render(glyphs.Empty))
```

- [ ] **Step 4: `dayOffPaceGlyph` löschen**

In `internal/frontend/tui/screen/worktime/week.go` Zeile 474-479 (ganzer Block inkl. Doc-Kommentar) löschen:

```go
// dayOffPaceGlyph delegiert an helpers.dayOffGlyph — selbe Whitelist-
// Mapping wie history_heatmap und history_month, eine zentrale
// Wahrheits-Quelle.
func dayOffPaceGlyph(k domain.Kind) string {
	return dayOffGlyph(k)
}
```

- [ ] **Step 5: Run passing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderPace_FreeDayUsesEmptyGlyphPerKindColor -v
```

Expected: `PASS` für alle drei Kinds.

- [ ] **Step 6: Commit**

```bash
git add internal/frontend/tui/screen/worktime/week.go internal/frontend/tui/screen/worktime/week_test.go
git commit -m "refactor(worktime/week): renderPace uses ○ + kindStyle for free days

★/☼/✚ + Sem-Farbe → ○ + Sem-Farbe. dayOffPaceGlyph entfernt. Spec:
docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md"
```

---

### Task 6: `history_month.go renderMonthCell` nutzt `○` + per-Kind-Farbe

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/history_month.go:160-162`
- Create: `internal/frontend/tui/screen/worktime/history_month_test.go`

- [ ] **Step 1: Test schreiben**

Lege `internal/frontend/tui/screen/worktime/history_month_test.go` an. **Package `worktime`** (internal). Vor dem Test-Schreiben kurz die echte Signatur von `renderMonthCell` in `history_month.go` lesen — die Parameter-Reihenfolge muss matchen, das Beispiel unten ist die *erwartete* Form.

```go
package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// TestRenderMonthCell_FreeDayColoredPerKind pinnt fest, dass die Monats-
// Grid-Zelle für freie Tage den ○-Glyph trägt und die Farbe per Kind
// (Info/Success/Warning) statt pauschal Info benutzt.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func TestRenderMonthCell_FreeDayColoredPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	pal := theme.TokyonightNight
	sem := pal.Sem()

	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, string(sem.Info)},
		{domain.KindVacation, string(sem.Success)},
		{domain.KindSick, string(sem.Warning)},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			dayoffs := testutil.NewFakeDayOffStore()
			if err := dayoffs.Add(domain.DayOff{Date: day, Kind: tc.kind, Label: "T"}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			deps := Deps{
				DayOffStore: dayoffs,
				Clock:       &testutil.FixedClock{T: now},
			}
			h := history{pal: pal, deps: deps}
			byKey := map[string]domain.DayRecord{}
			// renderMonthCell-Signatur ist (day time.Time, byKey map[string]domain.DayRecord,
			// monthRef time.Time, isCursor bool, now time.Time) string — vor dem
			// Schreiben prüfen und ggf. anpassen.
			out := h.renderMonthCell(day, byKey, day, false, now)
			if !strings.Contains(out, glyphs.Empty) {
				t.Errorf("cell missing %q for kind %q: %q", glyphs.Empty, tc.kind, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("cell should colour kind %q with %q, got: %q", tc.kind, tc.color, out)
			}
		})
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderMonthCell_FreeDayColoredPerKind -v
```

Expected: `FAIL` — Glyph ist noch `★/☼/✚` und Farbe ist `sem.Info` für alle Kinds.

- [ ] **Step 3: `renderMonthCell` umstellen**

In `internal/frontend/tui/screen/worktime/history_month.go` ersetze Zeile 160-162:

```go
	case isOff:
		glyph = dayOffGlyph(dayOff.Kind)
		color = sem.Info
```

durch:

```go
	case isOff:
		glyph = glyphs.Empty
		color = kindColor(h.pal, dayOff.Kind)
```

(`sem` ist die lokale Variable von Zeile 142, bleibt erhalten — nur die Free-Day-Branch ändert ihre Farbquelle.)

- [ ] **Step 4: Run passing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderMonthCell_FreeDayColoredPerKind -v
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/history_month.go internal/frontend/tui/screen/worktime/history_month_test.go
git commit -m "refactor(worktime/month): cell uses ○ + per-kind colour

renderMonthCell: ★/☼/✚ + pauschal Sem.Info → ○ + kindColor (Info/Success/
Warning). Macht Kind-Unterscheidung im Monatsraster zum ersten Mal
farblich sichtbar. Spec: 2026-05-12-unified-dayoff-glyphs."
```

---

### Task 7: `history_heatmap.go` — Cell + Legend umstellen

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/history_heatmap.go:84-86` (`renderHeatmapCell` Free-Day-Branch)
- Modify: `internal/frontend/tui/screen/worktime/history_heatmap.go:142` (`renderHeatmapLegend` Free-Day-Chip)
- Modify: `internal/frontend/tui/screen/worktime/history_heatmap.go:172-178` (`dayOffHeatmapGlyph` zu Konstante degradieren)
- Create: `internal/frontend/tui/screen/worktime/history_heatmap_test.go`

- [ ] **Step 1: Test schreiben**

Lege `internal/frontend/tui/screen/worktime/history_heatmap_test.go` an. **Package `worktime`** (internal). Auch hier vor dem Schreiben die genaue `renderHeatmapCell`-Signatur in `history_heatmap.go` prüfen — sie ist `renderHeatmapCell(day time.Time, byKey map[string]domain.DayRecord, w, d int, now time.Time) string` laut Read-Output.

```go
package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/testutil"
)

// TestRenderHeatmapCell_FreeDayColoredPerKind: Heatmap-Zelle für freie
// Tage trägt jetzt ○ und Farbe per Kind statt pauschal Sem.Info.
// Spec: docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md
func TestRenderHeatmapCell_FreeDayColoredPerKind(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	day := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	pal := theme.TokyonightNight
	sem := pal.Sem()

	tests := []struct {
		kind  domain.Kind
		color string
	}{
		{domain.KindHoliday, string(sem.Info)},
		{domain.KindVacation, string(sem.Success)},
		{domain.KindSick, string(sem.Warning)},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			dayoffs := testutil.NewFakeDayOffStore()
			if err := dayoffs.Add(domain.DayOff{Date: day, Kind: tc.kind, Label: "T"}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			deps := Deps{
				DayOffStore: dayoffs,
				Clock:       &testutil.FixedClock{T: now},
			}
			h := history{pal: pal, deps: deps}
			byKey := map[string]domain.DayRecord{}
			// Heatmap-Cursor (w=99, d=99) absichtlich weg vom Test-Day, damit
			// die Cursor-Style-Inversion das per-Kind-Farb-Erwartung nicht
			// überschreibt.
			out := h.renderHeatmapCell(day, byKey, 99, 99, now)
			if !strings.Contains(out, " "+glyphs.Empty+" ") {
				t.Errorf("cell missing ' %s ' for kind %q: %q", glyphs.Empty, tc.kind, out)
			}
			if !strings.Contains(out, tc.color) {
				t.Errorf("cell should colour kind %q with %q, got: %q", tc.kind, tc.color, out)
			}
		})
	}
}

// TestRenderHeatmapLegend_ThreeColoredKindChips: Legende zeigt drei
// separate ○-Chips in Cyan/Green/Orange für die drei Kinds.
func TestRenderHeatmapLegend_ThreeColoredKindChips(t *testing.T) {
	pal := theme.TokyonightNight
	sem := pal.Sem()
	h := history{pal: pal}
	out := h.renderHeatmapLegend(120)
	wants := []struct {
		label string
		color string
	}{
		{"○ Feiertag", string(sem.Info)},
		{"○ Urlaub", string(sem.Success)},
		{"○ Krank", string(sem.Warning)},
	}
	for _, w := range wants {
		if !strings.Contains(out, w.label) {
			t.Errorf("legend missing %q: %q", w.label, out)
		}
		if !strings.Contains(out, w.color) {
			t.Errorf("legend missing colour %q for %q: %q", w.color, w.label, out)
		}
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run "TestRenderHeatmap" -v
```

Expected: `FAIL` — Cell zeigt noch `★/☼/✚` mit pauschal Info-Farbe, Legend hat noch ein zusammengelegtes Chip.

- [ ] **Step 3: `renderHeatmapCell` Free-Day-Branch umstellen**

In `internal/frontend/tui/screen/worktime/history_heatmap.go` ersetze Zeile 82-87:

```go
	if dayOff, isOff := h.deps.DayOffStore.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			cell = dayOffHeatmapGlyph(dayOff.Kind)
		}
		color = h.pal.Sem().Info
	}
```

durch:

```go
	if dayOff, isOff := h.deps.DayOffStore.Lookup(day); isOff {
		if !hasRec || rec.Target == 0 {
			cell = " " + glyphs.Empty + " "
		}
		color = kindColor(h.pal, dayOff.Kind)
	}
```

- [ ] **Step 4: `renderHeatmapLegend` aufsplitten**

In `internal/frontend/tui/screen/worktime/history_heatmap.go` ersetze Zeile 142 (die `frei`-Chip-Zeile innerhalb `renderHeatmapLegend`):

```go
		lipgloss.NewStyle().Foreground(sem.Info).Render(glyphs.Holiday + "/" + glyphs.Vacation + "/" + glyphs.Extra + " frei"),
```

durch drei separate Einträge:

```go
		lipgloss.NewStyle().Foreground(sem.Info).Render(glyphs.Empty + " Feiertag"),
		lipgloss.NewStyle().Foreground(sem.Success).Render(glyphs.Empty + " Urlaub"),
		lipgloss.NewStyle().Foreground(sem.Warning).Render(glyphs.Empty + " Krank"),
```

- [ ] **Step 5: `dayOffHeatmapGlyph` zu Konstante degradieren oder inlinen**

In `internal/frontend/tui/screen/worktime/history_heatmap.go` Zeile 172-178 löschen:

```go
// dayOffHeatmapGlyph wraps dayOffGlyph mit den Heatmap-Cell-Spaces
// (jede Heat-Zelle ist 3 Char breit). Zentralisierter Single-Cell-
// Glyph kommt aus helpers.dayOffGlyph; die Spaces bleiben hier, weil
// nur die Heatmap pro Cell-Position drei Chars verlangt.
func dayOffHeatmapGlyph(k domain.Kind) string {
	return " " + dayOffGlyph(k) + " "
}
```

Der einzige Aufrufer (Step 3) wurde schon auf den Inline-Ausdruck umgestellt — die Funktion ist tot.

- [ ] **Step 6: Run passing tests**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run "TestRenderHeatmap" -v
```

Expected: `PASS` für Cell + Legend.

- [ ] **Step 7: Commit**

```bash
git add internal/frontend/tui/screen/worktime/history_heatmap.go internal/frontend/tui/screen/worktime/history_heatmap_test.go
git commit -m "refactor(worktime/heatmap): cell + legend use ○ + per-kind colour

renderHeatmapCell: ★/☼/✚ + pauschal Sem.Info → ○ + kindColor.
renderHeatmapLegend: 1 zusammengelegtes Chip → 3 farbige Chips
(Feiertag/Urlaub/Krank). dayOffHeatmapGlyph entfernt. Spec:
2026-05-12-unified-dayoff-glyphs."
```

---

### Task 8: `helpers.go` aufräumen + `helpers_test.go` updaten

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/helpers.go:51-69` (`dayOffGlyph` löschen)
- Modify: `internal/frontend/tui/screen/worktime/helpers_test.go:284-297` (`TestDayOffHeatmapGlyph` ersetzen oder löschen)

- [ ] **Step 1: `dayOffGlyph` löschen**

In `internal/frontend/tui/screen/worktime/helpers.go` Zeile 51-69 (ganzer Funktions-Block inkl. Doc-Kommentar) löschen:

```go
// dayOffGlyph mappt domain.Kind auf den kanonischen Single-Cell-Glyph
// aus der Whitelist. Zentral statt 3× dupliziert (week.dayOffPaceGlyph,
// history_heatmap.dayOffHeatmapGlyph, history_month-inline) — eine
// Whitelist-Änderung schlägt damit konsistent durch.
//
// Fallback ist BulletDot (·), nicht Empty (○): "unknown kind" ist eine
// schwache Aussage, kein "missed goal" wie Empty es semantisch trägt.
// Konsistent mit dem Heatmap-Pre-Refactor-Verhalten.
func dayOffGlyph(k domain.Kind) string {
	switch k {
	case domain.KindHoliday:
		return glyphs.Holiday
	case domain.KindVacation:
		return glyphs.Vacation
	case domain.KindSick:
		return glyphs.Extra
	}
	return glyphs.BulletDot
}
```

- [ ] **Step 2: `TestDayOffHeatmapGlyph` ersetzen**

In `internal/frontend/tui/screen/worktime/helpers_test.go` ersetze Zeile 284-297 (gesamter Test-Block) durch:

```go
// TestDayOffHeatmapGlyph wurde nach dem Glyph-Vereinheitlichungs-Spec
// 2026-05-12 entfernt — dayOffHeatmapGlyph ist gelöscht, die Heatmap
// rendert " ○ " inline (siehe TestRenderHeatmapCell_FreeDayColoredPerKind
// in history_heatmap_test.go).
```

(Den ganzen Test wegnehmen ist sauber — das Verhalten wird jetzt durch `TestRenderHeatmapCell_FreeDayColoredPerKind` aus Task 7 abgedeckt.)

- [ ] **Step 3: Build + Tests prüfen**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
go build ./internal/frontend/tui/screen/worktime/
go test ./internal/frontend/tui/screen/worktime/ -v -run "TestDayOff|TestRender"
```

Expected: Build clean, alle relevanten Tests `PASS`. Falls noch andere Aufrufer von `dayOffGlyph` aus früheren Tasks übrig sind, würde der Build hier mit "undefined" fehlschlagen — dann zurück zur jeweiligen Task und Aufrufer anpassen.

- [ ] **Step 4: Commit**

```bash
git add internal/frontend/tui/screen/worktime/helpers.go internal/frontend/tui/screen/worktime/helpers_test.go
git commit -m "chore(worktime/helpers): remove dayOffGlyph (unused)

Alle Worktime-Konsumenten (week/heatmap/month) gehen jetzt direkt gegen
glyphs.Empty + kindColor/kindStyle. dayOffGlyph hat keine Aufrufer mehr.
glyphs.Holiday/Vacation/Extra bleiben in der Whitelist (Markdown-Renderer
und kompendium nutzen sie noch)."
```

---

### Task 9: `dayoffs.go renderKindSummary` — Labels in Kind-Farbe

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/dayoffs.go:508-521`
- Create: `internal/frontend/tui/screen/worktime/dayoffs_test.go`

- [ ] **Step 1: Test schreiben**

Lege `internal/frontend/tui/screen/worktime/dayoffs_test.go` an. **Package `worktime`** (internal).

```go
package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// TestRenderKindSummary_LabelsInKindColor: Übersichts-Chips oben im
// Frei-Tab zeigen jedes Kind-Label in seiner Sem-Farbe (statt pauschal
// dim). Spec: 2026-05-12-unified-dayoff-glyphs.
func TestRenderKindSummary_LabelsInKindColor(t *testing.T) {
	pal := theme.TokyonightNight
	sem := pal.Sem()
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	f := frei{
		pal: pal,
		entries: []domain.DayOff{
			{Date: may, Kind: domain.KindHoliday, Label: "H1"},
			{Date: may.AddDate(0, 0, 1), Kind: domain.KindHoliday, Label: "H2"},
			{Date: may.AddDate(0, 0, 2), Kind: domain.KindVacation, Label: "U"},
			{Date: may.AddDate(0, 0, 3), Kind: domain.KindSick, Label: "K"},
		},
	}
	out := f.renderKindSummary()
	tests := []struct {
		label string
		color string
	}{
		{"Feiertag", string(sem.Info)},
		{"Urlaub", string(sem.Success)},
		{"Krank", string(sem.Warning)},
	}
	for _, tc := range tests {
		if !strings.Contains(out, tc.label) {
			t.Errorf("summary missing label %q: %q", tc.label, out)
		}
		if !strings.Contains(out, tc.color) {
			t.Errorf("summary missing colour %q for label %q: %q", tc.color, tc.label, out)
		}
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderKindSummary_LabelsInKindColor -v
```

Expected: `FAIL` — alle Labels noch dim, keine Sem-Farbe.

- [ ] **Step 3: `renderKindSummary` umstellen**

In `internal/frontend/tui/screen/worktime/dayoffs.go` ersetze Zeile 508-521:

```go
func (f frei) renderKindSummary() string {
	byKind := map[domain.Kind]int{}
	for _, d := range f.entries {
		byKind[d.Kind]++
	}
	parts := make([]string, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		if c := byKind[k]; c > 0 {
			parts = append(parts,
				stDim(f.pal, fmt.Sprintf("%s %d", k.LabelDe(), c)))
		}
	}
	return strings.Join(parts, stDim(f.pal, "  ·  "))
}
```

durch:

```go
func (f frei) renderKindSummary() string {
	byKind := map[domain.Kind]int{}
	for _, d := range f.entries {
		byKind[d.Kind]++
	}
	parts := make([]string, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		if c := byKind[k]; c > 0 {
			labelStyle := lipgloss.NewStyle().Foreground(kindColor(f.pal, k))
			parts = append(parts,
				labelStyle.Render(k.LabelDe())+" "+stDim(f.pal, fmt.Sprintf("%d", c)))
		}
	}
	return strings.Join(parts, stDim(f.pal, "  ·  "))
}
```

(Imports prüfen — `lipgloss` ist im File schon importiert, kein neuer Import nötig.)

- [ ] **Step 4: Run passing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderKindSummary_LabelsInKindColor -v
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/dayoffs.go internal/frontend/tui/screen/worktime/dayoffs_test.go
git commit -m "refactor(worktime/frei): Kind-Summary in Sem-Farben

renderKindSummary: Labels in Kind-Farbe (Info/Success/Warning), Count
bleibt dim für Hierarchie. Konsistent mit Pace-Strip/Heatmap/Monat.
Spec: 2026-05-12-unified-dayoff-glyphs."
```

---

### Task 10: `dayoffs.go renderKindPicker` — führender `○` in Kind-Farbe

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/dayoffs.go:590-606`
- Modify: `internal/frontend/tui/screen/worktime/dayoffs_test.go` (neuer Test)

- [ ] **Step 1: Test anhängen**

In `internal/frontend/tui/screen/worktime/dayoffs_test.go` ans Datei-Ende anhängen:

```go
// TestRenderKindPicker_LeadingColoredGlyph: jeder Kind-Chip im Add-Dialog
// trägt einen führenden ○ in der Kind-Farbe, auch wenn das Chip nicht
// selektiert ist. Selektierter Chip behält Accent (One-Accent-Per-Row).
// Spec: 2026-05-12-unified-dayoff-glyphs.
func TestRenderKindPicker_LeadingColoredGlyph(t *testing.T) {
	pal := theme.TokyonightNight
	sem := pal.Sem()
	f := frei{pal: pal, dialog: freiDialogAdd, kindCur: 0} // Holiday default-cursor
	out := f.renderKindPicker(80)

	tests := []struct {
		label string
		color string
	}{
		{"○ Feiertag", string(sem.Info)},
		{"○ Urlaub", string(sem.Success)},
		{"○ Krank", string(sem.Warning)},
	}
	for _, tc := range tests {
		if !strings.Contains(out, tc.label) {
			t.Errorf("picker missing %q: %q", tc.label, out)
		}
		if !strings.Contains(out, tc.color) {
			t.Errorf("picker missing colour %q for %q: %q", tc.color, tc.label, out)
		}
	}
}
```

(Achtung: prüfe ob `freiDialogAdd` exportiert/sichtbar im selben Package ist — `dayoffs_test.go` ist im selben Package, also unexported Identifier sind sichtbar. `kindIdx()` und Form-Struktur kann der Test ignorieren, da `renderKindPicker` nur `kindCur` und `formCur` benötigt.)

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderKindPicker_LeadingColoredGlyph -v
```

Expected: `FAIL` — Chips haben noch keinen führenden `○`.

- [ ] **Step 3: `renderKindPicker` umstellen**

In `internal/frontend/tui/screen/worktime/dayoffs.go` ersetze Zeile 590-606:

```go
func (f frei) renderKindPicker(inner int) string {
	header := picker.SectionHeader("kategorie  (h/l zum Wechseln)", inner, f.pal)
	chips := make([]string, 0, len(domain.AllKinds))
	kindFocused := f.formCur == f.kindIdx()
	for i, k := range domain.AllKinds {
		st := lipgloss.NewStyle().Foreground(f.pal.FgMuted)
		if i == f.kindCur {
			if kindFocused {
				st = lipgloss.NewStyle().Foreground(f.pal.Bg).Background(f.pal.Sem().Accent).Bold(true).Underline(true)
			} else {
				st = lipgloss.NewStyle().Foreground(f.pal.Sem().Accent).Bold(true).Underline(true)
			}
		}
		chips = append(chips, st.Render(" "+k.LabelDe()+" "))
	}
	return header + "\n  " + strings.Join(chips, "  ")
}
```

durch:

```go
func (f frei) renderKindPicker(inner int) string {
	header := picker.SectionHeader("kategorie  (h/l zum Wechseln)", inner, f.pal)
	chips := make([]string, 0, len(domain.AllKinds))
	kindFocused := f.formCur == f.kindIdx()
	for i, k := range domain.AllKinds {
		// Kind-Farbe trägt der führende ○ — auch im unselektierten
		// Zustand sichtbar. §Color semantics "one accent per row" bleibt
		// gewahrt: das Label-Style des selektierten Chips schreibt den
		// Glyph in der Selektions-Farbe (Accent) über die Kind-Farbe.
		glyphStyle := lipgloss.NewStyle().Foreground(kindColor(f.pal, k))
		labelStyle := lipgloss.NewStyle().Foreground(f.pal.FgMuted)
		if i == f.kindCur {
			if kindFocused {
				labelStyle = lipgloss.NewStyle().Foreground(f.pal.Bg).Background(f.pal.Sem().Accent).Bold(true).Underline(true)
				glyphStyle = labelStyle
			} else {
				labelStyle = lipgloss.NewStyle().Foreground(f.pal.Sem().Accent).Bold(true).Underline(true)
				glyphStyle = labelStyle
			}
		}
		chips = append(chips,
			glyphStyle.Render(" "+glyphs.Empty+" ")+labelStyle.Render(k.LabelDe()+" "))
	}
	return header + "\n  " + strings.Join(chips, "  ")
}
```

(`glyphs` Import prüfen — falls noch nicht importiert: `"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"` zum Import-Block hinzufügen.)

- [ ] **Step 4: Run passing test**

```bash
go test ./internal/frontend/tui/screen/worktime/ -run TestRenderKindPicker_LeadingColoredGlyph -v
```

Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/frontend/tui/screen/worktime/dayoffs.go internal/frontend/tui/screen/worktime/dayoffs_test.go
git commit -m "feat(worktime/frei): Kind-Picker mit führendem ○ in Sem-Farbe

renderKindPicker: jedes Chip bekommt einen führenden ○ in der Kind-Farbe.
Selektierter Chip schreibt Accent über Kind-Farbe (One-Accent-Per-Row).
Spec: 2026-05-12-unified-dayoff-glyphs."
```

---

## Phase 3 — CI Validation & Visual Smoke-Test

### Task 11: `make ci` grün, Golden-Snapshots regenerieren

**Files:** alle (cross-cutting Validation)

- [ ] **Step 1: Volles `make ci` laufen**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
make ci
```

Expected: kann an Golden-Snapshot-Tests fehlschlagen (`screen_baseline_test.go`, `render_repro_test.go`, ggf. `keymap_sync_test.go`). Pro Failure: Output anschauen, prüfen dass der Diff **nur** die erwarteten Glyph- und Farbänderungen enthält (kein `★/☼/✚`-Vorkommen mehr, neue Sem-Farben in Free-Day-Cells).

- [ ] **Step 2: Snapshots updaten falls vorhanden**

Wenn ein Test einen `UPDATE_GOLDEN=1`-Flag oder ähnliches hat, identifizieren und ausführen. Sonst manuell die `*.golden`/`*.snapshot`-Files anpassen.

Beispiel-Kommando (passe Pfad an):

```bash
UPDATE_GOLDEN=1 go test ./internal/frontend/tui/screen/worktime/ -run TestScreenBaseline -v
```

Wenn kein Update-Mechanismus existiert: das Test-Pattern lesen und die Erwartungs-Strings im Test-Source direkt anpassen.

- [ ] **Step 3: Re-Run `make ci`**

```bash
make ci
```

Expected: grün.

- [ ] **Step 4: Commit (Snapshot-Updates separat)**

```bash
git add internal/frontend/tui/
git commit -m "test(worktime): regenerate golden snapshots after glyph unification

Snapshots zeigen jetzt ○ statt ★/☼/✚ und Kind-Farben in Free-Day-Cells
über alle Surfaces. Spec: 2026-05-12-unified-dayoff-glyphs."
```

---

### Task 12: Visueller Smoke-Test in tmux

**Files:** keine — manuelle Verifikation.

- [ ] **Step 1: Status-Segment direkt aufrufen**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow
go run ./cmd/flow worktime status
```

Expected: Output enthält `[Frei: …]` Banner (falls heute ein DayOff existiert) als `○ <Label>` in der entsprechenden Kind-Farbe, plus Pace-Dots `● ● ○ ○ ● …` mit `○` für freie Tage in Cyan/Green/Yellow.

Falls heute kein DayOff existiert, kann via `flow worktime dayoff add` einer für heute angelegt werden (und nach dem Test wieder entfernt). Beispiel:

```bash
go run ./cmd/flow worktime dayoff add $(date +%Y-%m-%d) holiday "Smoke-Test"
go run ./cmd/flow worktime status
go run ./cmd/flow worktime dayoff rm $(date +%Y-%m-%d)
```

- [ ] **Step 2: tmux Status-Bar refreshen**

```bash
tmux refresh-client -S
```

Status-right anschauen — das Banner und die Pace-Dots sollten die erwartete Visualisierung zeigen.

- [ ] **Step 3: TUI-Worktime öffnen und durch alle Tabs navigieren**

```bash
go run ./cmd/flow worktime today
```

Per `2` zur Woche-Ansicht, `3` zur History (Heatmap + Monatsraster), `4` zum Frei-Tab. Pro Tab prüfen:

- **Woche-Tab:** Pace-Strip am Boden zeigt `○` für freie Tage in der jeweiligen Sem-Farbe — kein `★/☼/✚` mehr sichtbar.
- **History-Tab → Heatmap-View:** freie Tage in Cyan (Feiertag), Green (Urlaub), Yellow/Orange (Krank). Legende zeigt drei separate Chips `○ Feiertag · ○ Urlaub · ○ Krank` in ihren Farben.
- **History-Tab → Monats-View (`l` zum Switch falls vorhanden):** freie Tage in Kind-Farbe (nicht mehr pauschal cyan).
- **Frei-Tab:** Summary-Chips oben (`Feiertag 3 · Urlaub 7 · Krank 1`) — Labels in Kind-Farbe, Counts dim. Add-Dialog (`a` drücken) zeigt Kategorie-Chips mit führendem farbigem `○`.

- [ ] **Step 4: Visuellen Befund dokumentieren**

Falls alles ok: kein Commit nötig, der Smoke-Test ist ein Bestätigungs-Schritt.

Falls eine Visual-Regression auffällt, die `make ci` nicht abgefangen hat: einen Befund in `CLAUDE-activeContext.md` notieren oder einen Issue-/Task-Followup öffnen — typischerweise eine Style-Inheritance-Frage oder eine vergessene Cache-Invalidierung.

---

## Done-Criteria

- [ ] Alle 12 Tasks committed.
- [ ] `make ci` grün auf main.
- [ ] `go run ./cmd/flow worktime status` zeigt `○` + per-Kind-Farben in Banner und Pace-Dots.
- [ ] Alle vier TUI-Surfaces (Woche/Heatmap/Monat/Frei) rendern `○` + per-Kind-Farben.
- [ ] Whitelist-Glyphen `glyphs.Holiday/Vacation/Extra` sind in `glyphs.go` weiterhin definiert (Markdown-Konsumenten in kompendium unangetastet).
- [ ] Spec-Cross-Check: jeder Punkt aus `docs/superpowers/specs/2026-05-12-unified-dayoff-glyphs-design.md` "Implementierungs-Reihenfolge" Abschnitt ist abgehakt.

## Out of scope (separat tracken)

- Code-Review-Round-5-Fixes aus dem heutigen Review-Subagent (10 Befunde): separater Plan/Stack.
- StatusPalette-Slot-Renaming (`Yellow` → `Approaching` + `Warning` aufsplitten): wenn überhaupt, eigener Refactor.
- `glyphs.Holiday/Vacation/Extra` aus der Whitelist entfernen: nicht in diesem Spec — sie bleiben für Markdown-Renderer.
