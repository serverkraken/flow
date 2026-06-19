# Docs Kompendium-Look + TUI Design Language Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the rich "kompendium" list look in `flow docs` (count header, colored kind badges, date cell, body excerpt, cursor stripe, paginator) plus a client-side project filter, migrate `docs.go` styling onto `theme.Sem()`, and extract reusable `ui/` atoms so other screens inherit the look.

**Architecture:** Add small domain-free `ui/` components (`badge`, `chip`, `countbar`) plus one domain-coupled mapping package (`internal/tui/kindcolor`). Rebuild `DocsModel`'s list rendering from these atoms + pure testable helpers + the native `bubbles/v2/paginator`. Project names are resolved client-side via `apiclient.ListProjects` (mirrors worktime Today); filtering is client-side and **inclusive** (daily/free always visible).

**Tech Stack:** Go, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2/paginator`, existing `internal/tui/theme` + `internal/tui/ui/*` design system.

## Global Constraints

- Module path: `github.com/serverkraken/flow`. Import `domain` as `github.com/serverkraken/flow/internal/domain` (pkg `domain`), `apiclient` as `github.com/serverkraken/flow/internal/adapter/apiclient` (pkg `apiclient`).
- Colors come **only** from `theme.Palette` / `theme.Sem()` — never new hardcoded hex. `theme.Color` is `type Color string` and implements `image/color.Color` (has `RGBA()`), so it is accepted by `lipgloss.NewStyle().Foreground(...)`/`.Background(...)`.
- `ui/*` packages stay **domain-free** (no `internal/domain` import). The `DocumentType → color/glyph/label` mapping lives in `internal/tui/kindcolor` (may import `domain` + `theme`).
- Tests: `package <pkg>_test`, `t.Parallel()`, plain `t.Errorf`/`t.Fatalf` (no testify). Build a palette in tests via `theme.Default`.
- Follow `make ci` (lint + templ + build + tests + coverage gate). Run it before declaring a milestone done.
- No emoji pictograms — only the monospace glyph whitelist in `internal/tui/ui/glyphs`.
- `styles.go` and the legacy `tui.New(...)` Model (`worktime.go`/`stats.go`/`dayoffs.go`/`export.go`) are **still wired** to standalone `flow worktime` (`cmd/flow/worktime.go:28`). Do **not** delete them in this plan. This plan only removes `docs.go`'s dependency on `styles.go`.

---

## File Structure

**New files:**
- `internal/tui/kindcolor/kindcolor.go` — `DocumentType → (badge label, count glyph, semantic color)`. Single source of truth.
- `internal/tui/kindcolor/kindcolor_test.go`
- `internal/tui/ui/badge/badge.go` — colored label pill (domain-free).
- `internal/tui/ui/badge/badge_test.go`
- `internal/tui/ui/chip/chip.go` — `⟨ label ⟩` filter/context chip (domain-free).
- `internal/tui/ui/chip/chip_test.go`
- `internal/tui/ui/countbar/countbar.go` — `n/m <noun> · ● x label …` counts line (domain-free).
- `internal/tui/ui/countbar/countbar_test.go`
- `internal/tui/docs_render.go` — pure render helpers for the docs list (same `package tui`).
- `internal/tui/docs_render_test.go`

**Modified files:**
- `internal/tui/docs.go` — add `pal`/`projects`/`projByID`/`projFilter`/project-picker state; new `reload`/projects load; rebuilt `renderList`; project-filter mode; full migration off `styles.go`.
- `internal/tui/screen/docs/route.go` — forward palette into `DocsModel`; add `p` keyhint.
- `cmd/flow/docs.go` — pass `theme.Load()` into `tui.NewDocs`.

---

### Task 1: `kindcolor` mapping package

**Files:**
- Create: `internal/tui/kindcolor/kindcolor.go`
- Test: `internal/tui/kindcolor/kindcolor_test.go`

**Interfaces:**
- Consumes: `domain.DocumentType`, `theme.Palette`, `internal/tui/ui/glyphs`.
- Produces:
  - `func Badge(t domain.DocumentType) string` — fixed-width (5-cell) label, e.g. `"TÄGL."`, `"PROJ."`, `"FREI "`, `"AGENT"`.
  - `func Glyph(t domain.DocumentType) string` — count glyph, e.g. `"●"`, `"◆"`, `"○"`, `"▪"`.
  - `func Color(t domain.DocumentType, p theme.Palette) theme.Color` — semantic color.

- [ ] **Step 1: Write the failing test**

```go
package kindcolor_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestBadge_FixedWidthFiveCells(t *testing.T) {
	t.Parallel()
	for _, tp := range []domain.DocumentType{domain.DocDaily, domain.DocProject, domain.DocFree, domain.DocAgent} {
		if w := lipgloss.Width(kindcolor.Badge(tp)); w != 5 {
			t.Errorf("Badge(%q) width = %d, want 5", tp, w)
		}
	}
}

func TestColorAndGlyph_PerType(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		t     domain.DocumentType
		color theme.Color
		glyph string
	}{
		{domain.DocDaily, sem.Accent, "●"},
		{domain.DocProject, sem.Success, "◆"},
		{domain.DocFree, sem.Highlight, "○"},
		{domain.DocAgent, sem.Warning, "▪"},
	}
	for _, c := range cases {
		if got := kindcolor.Color(c.t, p); got != c.color {
			t.Errorf("Color(%q) = %q, want %q", c.t, got, c.color)
		}
		if got := kindcolor.Glyph(c.t); got != c.glyph {
			t.Errorf("Glyph(%q) = %q, want %q", c.t, got, c.glyph)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/kindcolor/...`
Expected: FAIL (package/functions not defined).

- [ ] **Step 3: Write minimal implementation**

```go
// Package kindcolor maps a domain DocumentType to its visual identity (badge
// label, count glyph, semantic color). It is the single source of truth so a
// badge and its count glyph can never drift in color.
package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// Badge returns a fixed-width (5-cell) badge label for the document type.
func Badge(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return "TÄGL."
	case domain.DocProject:
		return "PROJ."
	case domain.DocFree:
		return "FREI "
	case domain.DocAgent:
		return "AGENT"
	default:
		return "  ?  "
	}
}

// Glyph returns the count/legend glyph for the document type.
func Glyph(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return glyphs.CountDaily
	case domain.DocProject:
		return glyphs.CountProject
	case domain.DocFree:
		return glyphs.CountFree
	case domain.DocAgent:
		return glyphs.Bullet4
	default:
		return glyphs.BulletDot
	}
}

// Color returns the semantic color for the document type from the palette.
func Color(t domain.DocumentType, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch t {
	case domain.DocDaily:
		return sem.Accent
	case domain.DocProject:
		return sem.Success
	case domain.DocFree:
		return sem.Highlight
	case domain.DocAgent:
		return sem.Warning
	default:
		return sem.Border
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/kindcolor/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/kindcolor/
git commit -m "feat(tui): kindcolor — DocumentType→badge/glyph/color single source"
```

---

### Task 2: `ui/badge` component

**Files:**
- Create: `internal/tui/ui/badge/badge.go`
- Test: `internal/tui/ui/badge/badge_test.go`

**Interfaces:**
- Consumes: `theme.Palette`, `theme.Color`.
- Produces: `func Render(label string, c theme.Color, p theme.Palette) string` — bold dark-on-color pill with horizontal padding (`Padding(0,1)`), so rendered width = `lipgloss.Width(label)+2`.

- [ ] **Step 1: Write the failing test**

```go
package badge_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/badge"
)

func TestRender_WidthAndContent(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := badge.Render("PROJ.", p.Sem().Success, p)
	if !strings.Contains(out, "PROJ.") {
		t.Errorf("badge missing label: %q", out)
	}
	if w := lipgloss.Width(out); w != lipgloss.Width("PROJ.")+2 {
		t.Errorf("badge width = %d, want label+2", w)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/badge/...`
Expected: FAIL (package not defined).

- [ ] **Step 3: Write minimal implementation**

```go
// Package badge renders a small colored label pill (dark text on a semantic
// color), e.g. kind badges in lists. Domain-free: the caller supplies label+color.
package badge

import (
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render returns a bold dark-on-color pill with single-column horizontal padding.
func Render(label string, c theme.Color, p theme.Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render(label)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/badge/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/badge/
git commit -m "feat(tui): ui/badge colored label pill"
```

---

### Task 3: `ui/chip` component

**Files:**
- Create: `internal/tui/ui/chip/chip.go`
- Test: `internal/tui/ui/chip/chip_test.go`

**Interfaces:**
- Consumes: `theme.Palette`, `theme.Color`.
- Produces: `func Render(label string, c theme.Color, p theme.Palette) string` — renders `⟨ label ⟩` as a dark-on-color chip (active filter/context indicator). Angle brackets are part of the output.

- [ ] **Step 1: Write the failing test**

```go
package chip_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/chip"
)

func TestRender_WrapsLabelInAngles(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := chip.Render("serverkraken/flow", p.Sem().Accent, p)
	if !strings.Contains(out, "serverkraken/flow") {
		t.Errorf("chip missing label: %q", out)
	}
	if !strings.Contains(out, "⟨") || !strings.Contains(out, "⟩") {
		t.Errorf("chip missing angle brackets: %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/chip/...`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
// Package chip renders a compact ⟨ label ⟩ chip used for an active filter or
// context indicator. Domain-free: the caller supplies label+color.
package chip

import (
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Render returns ⟨ label ⟩ as a bold dark-on-color chip.
func Render(label string, c theme.Color, p theme.Palette) string {
	return lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render("⟨ " + label + " ⟩")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/chip/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/chip/
git commit -m "feat(tui): ui/chip ⟨label⟩ context chip"
```

---

### Task 4: `ui/countbar` component

**Files:**
- Create: `internal/tui/ui/countbar/countbar.go`
- Test: `internal/tui/ui/countbar/countbar_test.go`

**Interfaces:**
- Consumes: `theme.Palette`, `theme.Color`, `theme` builders.
- Produces:
  - `type Seg struct { Glyph, Label string; N int; Color theme.Color }`
  - `func Render(visible, total int, noun string, segs []Seg, p theme.Palette) string` — e.g. `24/24 Notizen   ·   ● 10 täglich  ◆ 9 projekt  ○ 5 frei` (glyph+count colored per seg, label dim).

- [ ] **Step 1: Write the failing test**

```go
package countbar_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/countbar"
)

func TestRender_CountsAndNoun(t *testing.T) {
	t.Parallel()
	p := theme.Default
	out := countbar.Render(9, 24, "Notizen", []countbar.Seg{
		{Glyph: "●", Label: "täglich", N: 10, Color: p.Sem().Accent},
		{Glyph: "◆", Label: "projekt", N: 9, Color: p.Sem().Success},
	}, p)
	for _, want := range []string{"9/24 Notizen", "täglich", "projekt", "● 10", "◆ 9"} {
		if !strings.Contains(out, want) {
			t.Errorf("countbar missing %q in:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/countbar/...`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
// Package countbar renders a counts line: "n/m <noun> · <glyph> <N> <label> …".
// Domain-free: the caller supplies segments (glyph, label, count, color).
package countbar

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Seg is one colored count segment.
type Seg struct {
	Glyph string
	Label string
	N     int
	Color theme.Color
}

// Render returns the counts line. visible/total render as "visible/total noun".
func Render(visible, total int, noun string, segs []Seg, p theme.Palette) string {
	head := theme.Strong(fmt.Sprintf("%d/%d %s", visible, total, noun), p)
	parts := make([]string, 0, len(segs))
	for _, s := range segs {
		g := lipgloss.NewStyle().Foreground(s.Color).Render(fmt.Sprintf("%s %d", s.Glyph, s.N))
		parts = append(parts, g+theme.Dim(" "+s.Label, p))
	}
	return head + theme.Dim("   ·   ", p) + strings.Join(parts, "  ")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/countbar/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/countbar/
git commit -m "feat(tui): ui/countbar counts line"
```

---

### Task 5: docs pure render helpers

**Files:**
- Create: `internal/tui/docs_render.go` (package `tui`)
- Test: `internal/tui/docs_render_test.go`

**Interfaces:**
- Consumes: `domain.Document`, `domain.Project`, `internal/tui/ui/strings` (alias `tuistrings`), `charm.land/lipgloss/v2`.
- Produces:
  - `func docCounts(docs []domain.Document) map[domain.DocumentType]int`
  - `func dateCell(d domain.Document) string` — `Date` for daily (formatted `2006-01-02`), else `UpdatedAt`.
  - `func applyProjectFilter(docs []domain.Document, projID string) []domain.Document` — `projID==""` returns all; else keeps `ProjectID==nil` (daily/free) OR `*ProjectID==projID` (inclusive).
  - `func projRowLabel(d domain.Document, projByID map[string]domain.Project) string` — project docs → `slug · title`; else `d.Path`.
  - `func docExcerpt(body string, width, maxLines int) []string` — whitespace-collapsed, word-wrapped to `width`, capped at `maxLines` (last line ends with `…` when truncated).

- [ ] **Step 1: Write the failing test**

```go
package tui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func strptr(s string) *string { return &s }

func TestApplyProjectFilter_Inclusive(t *testing.T) {
	t.Parallel()
	docs := []domain.Document{
		{Type: domain.DocDaily},                                  // nil ProjectID
		{Type: domain.DocProject, ProjectID: strptr("p1")},
		{Type: domain.DocProject, ProjectID: strptr("p2")},
		{Type: domain.DocFree},                                   // nil ProjectID
	}
	got := applyProjectFilter(docs, "p1")
	if len(got) != 3 { // daily + p1 + free
		t.Fatalf("filtered len = %d, want 3 (daily+p1+free)", len(got))
	}
	if all := applyProjectFilter(docs, ""); len(all) != 4 {
		t.Fatalf("empty filter len = %d, want 4", len(all))
	}
}

func TestDateCell_DailyUsesDate_ElseUpdatedAt(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	daily := domain.Document{Type: domain.DocDaily, Date: &d, UpdatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	if got := dateCell(daily); got != "2026-05-18" {
		t.Errorf("daily dateCell = %q, want 2026-05-18", got)
	}
	free := domain.Document{Type: domain.DocFree, UpdatedAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}
	if got := dateCell(free); got != "2026-06-10" {
		t.Errorf("free dateCell = %q, want 2026-06-10", got)
	}
}

func TestProjRowLabel(t *testing.T) {
	t.Parallel()
	byID := map[string]domain.Project{"p1": {ID: "p1", Slug: "serverkraken/flow"}}
	proj := domain.Document{Type: domain.DocProject, ProjectID: strptr("p1"), Title: "demo", Path: "x/demo"}
	if got := projRowLabel(proj, byID); got != "serverkraken/flow · demo" {
		t.Errorf("projRowLabel = %q, want 'serverkraken/flow · demo'", got)
	}
	free := domain.Document{Type: domain.DocFree, Path: "notes/foo"}
	if got := projRowLabel(free, byID); got != "notes/foo" {
		t.Errorf("free projRowLabel = %q, want 'notes/foo'", got)
	}
}

func TestDocExcerpt_WrapAndCap(t *testing.T) {
	t.Parallel()
	lines := docExcerpt("alpha beta gamma delta epsilon zeta eta", 11, 2)
	if len(lines) != 2 {
		t.Fatalf("excerpt lines = %d, want 2", len(lines))
	}
	if got := docCounts([]domain.Document{{Type: domain.DocDaily}, {Type: domain.DocDaily}, {Type: domain.DocFree}}); got[domain.DocDaily] != 2 || got[domain.DocFree] != 1 {
		t.Errorf("docCounts = %+v, want daily:2 free:1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'ApplyProjectFilter|DateCell|ProjRowLabel|DocExcerpt'`
Expected: FAIL (helpers not defined).

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	tuistrings "github.com/serverkraken/flow/internal/tui/ui/strings"
)

// docCounts tallies documents per type.
func docCounts(docs []domain.Document) map[domain.DocumentType]int {
	m := make(map[domain.DocumentType]int, 4)
	for _, d := range docs {
		m[d.Type]++
	}
	return m
}

// dateCell is the date column for a row: the daily Date, else UpdatedAt.
func dateCell(d domain.Document) string {
	if d.Date != nil {
		return d.Date.Format("2006-01-02")
	}
	return d.UpdatedAt.Format("2006-01-02")
}

// applyProjectFilter keeps project-less docs (daily/free) plus docs of the
// selected project. projID == "" returns docs unchanged.
func applyProjectFilter(docs []domain.Document, projID string) []domain.Document {
	if projID == "" {
		return docs
	}
	out := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if d.ProjectID == nil || *d.ProjectID == projID {
			out = append(out, d)
		}
	}
	return out
}

// projRowLabel is the row's primary label: "slug · title" for project docs,
// else the document path.
func projRowLabel(d domain.Document, projByID map[string]domain.Project) string {
	if d.Type == domain.DocProject && d.ProjectID != nil {
		if p, ok := projByID[*d.ProjectID]; ok {
			return p.Slug + " · " + d.Title
		}
	}
	return d.Path
}

// docExcerpt collapses whitespace and word-wraps body to width, capped at
// maxLines (the last line ends with … when content is truncated).
func docExcerpt(body string, width, maxLines int) []string {
	if width < 1 || maxLines < 1 {
		return nil
	}
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range fields {
		cand := w
		if cur != "" {
			cand = cur + " " + w
		}
		if lipgloss.Width(cand) > width && cur != "" {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = cand
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = tuistrings.Truncate(lines[maxLines-1]+" …", width)
	}
	return lines
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'ApplyProjectFilter|DateCell|ProjRowLabel|DocExcerpt'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/docs_render.go internal/tui/docs_render_test.go
git commit -m "feat(tui): docs pure render helpers (counts/date/filter/label/excerpt)"
```

---

### Task 6: Plumb palette + load projects into DocsModel

**Files:**
- Modify: `internal/tui/docs.go` (struct fields ~56-104; `NewDocs` ~106; `Init` ~218; add `loadProjects`; `Update` projects handler near 399)
- Modify: `internal/tui/screen/docs/route.go:37` (`NewRoute` already receives `pal`)
- Modify: `cmd/flow/docs.go:30`
- Test: `internal/tui/docs_render_test.go` (extend)

**Interfaces:**
- Consumes: `apiclient.ListProjects`, `theme.Palette`, `domain.Project`.
- Produces (new `DocsModel` fields + funcs):
  - fields: `pal theme.Palette`, `projects []domain.Project`, `projByID map[string]domain.Project`, `projFilter string` (project ID; `""` = all), `projCursor int`.
  - `func NewDocs(client *apiclient.Client, ed docEditor, op urlOpener, pal theme.Palette, user string) DocsModel`
  - `type projectsLoadedMsg struct{ projects []domain.Project }`
  - `func (m DocsModel) loadProjects() tea.Cmd`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/docs_render_test.go`:

```go
func TestUpdate_ProjectsLoaded_BuildsIndex(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	nm, _ := m.Update(projectsLoadedMsg{projects: []domain.Project{
		{ID: "p1", Slug: "serverkraken/flow"},
		{ID: "p2", Slug: "other/repo"},
	}})
	dm := nm.(DocsModel)
	if len(dm.projByID) != 2 || dm.projByID["p1"].Slug != "serverkraken/flow" {
		t.Fatalf("projByID = %+v, want 2 entries indexed by id", dm.projByID)
	}
}
```

Add `"github.com/serverkraken/flow/internal/tui/theme"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run ProjectsLoaded`
Expected: FAIL (`NewDocs` arity / `projectsLoadedMsg` undefined).

- [ ] **Step 3: Implement — struct fields**

In `DocsModel` (after the existing `filterCursor int` block, ~line 98) add:

```go
	pal        theme.Palette
	projects   []domain.Project
	projByID   map[string]domain.Project
	projFilter string // selected project ID; "" = all projects
	projCursor int    // cursor in the project-filter picker
```

- [ ] **Step 4: Implement — `NewDocs` signature + standalone wiring**

Replace `NewDocs` (line 106) with:

```go
func NewDocs(client *apiclient.Client, ed docEditor, op urlOpener, pal theme.Palette, user string) DocsModel {
	return DocsModel{client: client, editor: ed, opener: op, pal: pal, user: user, newType: domain.DocFree, linkFocus: -1}
}
```

In `cmd/flow/docs.go`, change line 30 to pass the palette (add `"github.com/serverkraken/flow/internal/tui/theme"` to imports):

```go
			m := tui.NewDocs(client, editor.New(), opener.New(), theme.Load(), os.Getenv("USER"))
```

In `internal/tui/screen/docs/route.go`, change `NewRoute` (line 37) to forward `pal`:

```go
	return &Route{m: tui.NewDocs(client, ed, op, pal, user), pal: pal}
```

- [ ] **Step 5: Implement — projects load command + Update handler + Init**

Add near `loadTags` (after line 269):

```go
type projectsLoadedMsg struct{ projects []domain.Project }

func (m DocsModel) loadProjects() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, err := m.client.ListProjects(ctx)
		if err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg{projects: ps}
	}
}
```

Add to `Update` next to the `docsLoadedMsg` case (after line 404):

```go
	case projectsLoadedMsg:
		m.projects = msg.projects
		m.projByID = make(map[string]domain.Project, len(msg.projects))
		for _, p := range msg.projects {
			m.projByID[p.ID] = p
		}
		return m, nil
```

Change `Init` (line 218-220) to also load projects:

```go
func (m DocsModel) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.loadProjects(), m.subscribe())
}
```

- [ ] **Step 6: Run test + build**

Run: `go build ./... && go test ./internal/tui/ -run ProjectsLoaded`
Expected: PASS (and the whole module still builds — `screen/docs` + `cmd/flow` updated).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/docs.go internal/tui/screen/docs/route.go cmd/flow/docs.go internal/tui/docs_render_test.go
git commit -m "feat(tui): plumb palette + load projects into DocsModel"
```

---

### Task 7: Rebuild `renderList` — the kompendium look

**Files:**
- Modify: `internal/tui/docs.go` (`renderList` 1094-1116; add `writeDocRow`, `docsPerPage`, `badgeForType`; add imports)
- Test: `internal/tui/docs_render_test.go` (extend)

**Interfaces:**
- Consumes: `kindcolor`, `badge`, `chip`, `countbar`, `paginator`, the Task-5 helpers, `glyphs`, `theme`.
- Produces: rebuilt `renderList(b *strings.Builder)` rendering header (heading + countbar), active project chip, `notizen` section, paginated multi-line rows (stripe + date + badge + label + 2-line excerpt), and the paginator dots + `i/total`.

- [ ] **Step 1: Add imports to `internal/tui/docs.go`**

Add to the import block:

```go
	"charm.land/bubbles/v2/paginator"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/ui/badge"
	"github.com/serverkraken/flow/internal/tui/ui/chip"
	"github.com/serverkraken/flow/internal/tui/ui/countbar"
```

- [ ] **Step 2: Write the failing test**

Append to `internal/tui/docs_render_test.go`:

```go
func TestRenderList_KompendiumLook(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.width = 80
	m.height = 40
	m.docs = []domain.Document{
		{Type: domain.DocDaily, Path: "daily/2026-05-18", Date: &d, Body: "First on-call schedule note"},
		{Type: domain.DocFree, Path: "notes/foo", UpdatedAt: d},
	}
	out := m.View().Content
	for _, want := range []string{"kompendium", "Notizen", "TÄGL.", "FREI", "daily/2026-05-18", "2026-05-18", "1/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderList missing %q in:\n%s", want, out)
		}
	}
}
```

Add `"strings"` and `"time"` to test imports if not present.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/ -run KompendiumLook`
Expected: FAIL (old flat renderer; missing "kompendium"/badges).

- [ ] **Step 4: Replace `renderList` and add helpers**

Replace `renderList` (lines 1094-1116) with:

```go
func (m DocsModel) renderList(b *strings.Builder) {
	pal := m.pal
	visible := applyProjectFilter(m.docs, m.projFilter)
	counts := docCounts(visible)

	segs := []countbar.Seg{
		{Glyph: kindcolor.Glyph(domain.DocDaily), Label: "täglich", N: counts[domain.DocDaily], Color: kindcolor.Color(domain.DocDaily, pal)},
		{Glyph: kindcolor.Glyph(domain.DocProject), Label: "projekt", N: counts[domain.DocProject], Color: kindcolor.Color(domain.DocProject, pal)},
		{Glyph: kindcolor.Glyph(domain.DocFree), Label: "frei", N: counts[domain.DocFree], Color: kindcolor.Color(domain.DocFree, pal)},
	}
	if counts[domain.DocAgent] > 0 {
		segs = append(segs, countbar.Seg{Glyph: kindcolor.Glyph(domain.DocAgent), Label: "agent", N: counts[domain.DocAgent], Color: kindcolor.Color(domain.DocAgent, pal)})
	}
	b.WriteString(theme.Heading("kompendium", pal) + theme.Dim(" — ", pal) +
		countbar.Render(len(visible), len(m.docs), "Notizen", segs, pal) + "\n")

	if m.projFilter != "" {
		if p, ok := m.projByID[m.projFilter]; ok {
			c := pal.Sem().Accent
			if p.Color != "" {
				c = theme.Color(p.Color)
			}
			label := p.Slug
			if p.Glyph != "" {
				label = p.Glyph + " " + p.Slug
			}
			b.WriteString(chip.Render(label, c, pal) + "\n")
		}
	}
	if len(m.filterTags) > 0 {
		b.WriteString(theme.Dim("  filter: "+tagSuffix(m.filterTags), pal) + "\n")
	}

	b.WriteString("\n" + theme.Dim("notizen", pal) + "\n\n")

	if len(visible) == 0 {
		b.WriteString(theme.Dim("  keine Notizen — n für neu", pal) + "\n")
		return
	}

	width := m.width
	if width < 20 {
		width = 80
	}
	perPage := m.docsPerPage()
	if m.sel >= len(visible) {
		m.sel = len(visible) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	pager := paginator.New(paginator.WithPerPage(perPage))
	pager.Type = paginator.Dots
	pager.ActiveDot = lipgloss.NewStyle().Foreground(pal.Sem().Accent).Render(glyphs.Filled)
	pager.InactiveDot = theme.Dim(glyphs.Empty, pal)
	pager.SetTotalPages(len(visible))
	pager.Page = m.sel / perPage
	start, end := pager.GetSliceBounds(len(visible))
	for i := start; i < end; i++ {
		m.writeDocRow(b, visible[i], i == m.sel, width)
	}
	b.WriteString("\n" + pager.View() + theme.Dim(fmt.Sprintf("  %d/%d", m.sel+1, len(visible)), pal) + "\n")
}

func (m DocsModel) writeDocRow(b *strings.Builder, d domain.Document, selected bool, width int) {
	pal := m.pal
	stripe := "  "
	if selected {
		stripe = lipgloss.NewStyle().Foreground(pal.Sem().Active).Render(glyphs.AccentBar) + " "
	}
	labelStyle := lipgloss.NewStyle().Foreground(pal.Fg)
	if selected {
		labelStyle = labelStyle.Bold(true)
	}
	b.WriteString(stripe +
		theme.Dim(dateCell(d), pal) + "  " +
		badgeForType(d.Type, pal) + "  " +
		labelStyle.Render(projRowLabel(d, m.projByID)) + "\n")
	for _, line := range docExcerpt(d.Body, width-6, 2) {
		b.WriteString("   " + theme.Dim(line, pal) + "\n")
	}
	b.WriteString("\n")
}

func badgeForType(t domain.DocumentType, p theme.Palette) string {
	return badge.Render(kindcolor.Badge(t), kindcolor.Color(t, p), p)
}

// docsPerPage derives rows-per-page from the terminal height (each row is ~3
// lines: header + up to 2 excerpt lines + blank). Falls back to 5 when unknown.
func (m DocsModel) docsPerPage() int {
	if m.height < 12 {
		return 5
	}
	n := (m.height - 8) / 3
	if n < 1 {
		n = 1
	}
	return n
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/ -run KompendiumLook`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_render_test.go
git commit -m "feat(tui): rebuild docs list with kompendium look (badges/counts/excerpt/paginator)"
```

---

### Task 8: Project filter interaction

**Files:**
- Modify: `internal/tui/docs.go` (add `modeProjectFilter` const; list-key `p`; `handleProjectFilterKey`; `renderProjectFilter`; `View`/`handleKey`/`footer` switches; `CapturesInput`)
- Test: `internal/tui/docs_render_test.go` (extend)

**Interfaces:**
- Consumes: `picker.Row`, `theme`, the Task-5 `applyProjectFilter`.
- Produces:
  - new `docMode` value `modeProjectFilter`.
  - `func (m DocsModel) handleProjectFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd)`
  - `func (m DocsModel) renderProjectFilter(b *strings.Builder)`
  - project-picker entries: index 0 = "Alle Projekte" (clears filter), then `m.projects` in order.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/docs_render_test.go`:

```go
func TestProjectFilter_OpenSelectClear(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.projects = []domain.Project{{ID: "p1", Slug: "serverkraken/flow"}}
	m.projByID = map[string]domain.Project{"p1": m.projects[0]}

	// open picker with "p"
	nm, _ := m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	if m.mode != modeProjectFilter {
		t.Fatalf("after p: mode = %v, want modeProjectFilter", m.mode)
	}
	// move to the project (index 1) and select
	m.projCursor = 1
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "p1" {
		t.Fatalf("projFilter = %q, want p1", m.projFilter)
	}
	if m.mode != modeList {
		t.Fatalf("after select: mode = %v, want modeList", m.mode)
	}
	// re-open, choose index 0 ("Alle") to clear
	nm, _ = m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	m.projCursor = 0
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "" {
		t.Fatalf("after Alle: projFilter = %q, want empty", m.projFilter)
	}
}
```

Add `tea "charm.land/bubbletea/v2"` to test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run ProjectFilter_OpenSelectClear`
Expected: FAIL (`modeProjectFilter` undefined; `p` not handled).

- [ ] **Step 3: Add the mode constant**

In the `docMode` const block (lines 39-46) append after `modeSearch`:

```go
	modeProjectFilter // project-filter picker
```

- [ ] **Step 4: Handle `p` in modeList**

In the `modeList` key switch, add before the closing `}` (after the `/` case at line 619):

```go
	case k.Text == "p":
		m.mode = modeProjectFilter
		m.projCursor = 0
		return m, nil
```

- [ ] **Step 5: Dispatch + handler**

In `handleKey` (lines 520-531) add a case:

```go
	case modeProjectFilter:
		return m.handleProjectFilterKey(k)
```

Add the handler (near `handleFilterKey`, after line 797):

```go
func (m DocsModel) handleProjectFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// entries: 0 = "Alle Projekte", 1..N = m.projects
	last := len(m.projects)
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList
		return m, nil
	case k.Text == "j":
		if m.projCursor < last {
			m.projCursor++
		}
		return m, nil
	case k.Text == "k":
		if m.projCursor > 0 {
			m.projCursor--
		}
		return m, nil
	case k.Code == tea.KeyEnter:
		if m.projCursor == 0 {
			m.projFilter = ""
		} else if m.projCursor-1 < len(m.projects) {
			m.projFilter = m.projects[m.projCursor-1].ID
		}
		m.mode = modeList
		m.sel = 0
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 6: Render the picker + wire View/footer/CapturesInput**

Add `renderProjectFilter` (near `renderFilter`):

```go
func (m DocsModel) renderProjectFilter(b *strings.Builder) {
	pal := m.pal
	b.WriteString(theme.Heading("Projekt-Filter", pal) + "\n\n")
	width := m.width
	if width < 20 {
		width = 60
	}
	b.WriteString(picker.Row(m.projCursor == 0, "Alle Projekte", "", width-4, pal) + "\n")
	for i, p := range m.projects {
		b.WriteString(picker.Row(m.projCursor == i+1, p.Slug, "", width-4, pal) + "\n")
	}
}
```

Add the import `"github.com/serverkraken/flow/internal/tui/ui/picker"`.

In `View` (the mode switch, lines 999-1021) add a case:

```go
	case modeProjectFilter:
		m.renderProjectFilter(&b)
```

In `footer` (lines 1068-1083) add a case and extend the default:

```go
	case modeProjectFilter:
		return "j/k move · enter wählen · esc abbrechen"
```

Change the default footer line (line 1081) to include `p`:

```go
		return "j/k move · enter view · n new · e edit · d delete · p projekt · f filter · / suchen · q quit"
```

If `DocsModel` has a `CapturesInput()` method, ensure it returns `true` for `modeProjectFilter` (add `modeProjectFilter` wherever `modeFiltering` is listed so the shell forwards keys).

- [ ] **Step 7: Run test + build**

Run: `go build ./... && go test ./internal/tui/ -run ProjectFilter_OpenSelectClear`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_render_test.go
git commit -m "feat(tui): docs project filter (p) — picker, chip, inclusive scope"
```

---

### Task 9: Migrate remaining `docs.go` styling off `styles.go`

**Files:**
- Modify: `internal/tui/docs.go` (every reference to a `styles.go` var; remaining modes: `View` header, `renderView`, `renderCreate`, `renderFilter`, `renderSearch`, status/error/link lines)

**Interfaces:**
- Consumes: `theme` builders + `theme.Palette` (`pal := m.pal`), `lipgloss`.
- Produces: `docs.go` with **zero** references to `styles.go` vars.

This is a deterministic find-and-replace. For each function that renders, add `pal := m.pal` at the top (skip if already present), then apply this exact mapping to every occurrence:

| Old (`styles.go`) | New expression |
|---|---|
| `styleHeader.Render(s)` | `theme.Heading(s, pal)` |
| `styleMuted.Render(s)` | `theme.Dim(s, pal)` |
| `styleOk.Render(s)` | `theme.Success(s, pal)` |
| `styleErr.Render(s)` | `theme.Err(s, pal)` |
| `styleWarn.Render(s)` | `theme.Danger(s, pal)` |
| `styleSel.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Bg).Background(pal.Sem().Accent).Render(s)` |
| `styleLinkFocus.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Bg).Background(pal.Sem().Accent).Bold(true).Render(s)` |
| `styleSearchHit.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Bg).Background(pal.Sem().Highlight).Bold(true).Render(s)` |
| `styleWikiValid.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Sem().Accent).Underline(true).Render(s)` |
| `styleWikiBroken.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Sem().Danger).Strikethrough(true).Render(s)` |
| `styleWebLink.Render(s)` | `lipgloss.NewStyle().Foreground(pal.Sem().Info).Underline(true).Render(s)` |
| `lipgloss.NewStyle().Foreground(colAccent)...` (inline, renderSearch) | `lipgloss.NewStyle().Foreground(pal.Sem().Accent)...` |

Note: `View()`'s header at line 997 (`styleHeader.Render("flow · docs") + styleMuted.Render("  "+m.user)`) becomes `theme.Heading("flow · docs", pal) + theme.Dim("  "+m.user, pal)` — but since `renderList` now prints its own `kompendium` header, drop the `"flow · docs"` line in `modeList`/`modeDeleting`/`modeFiltering`/`modeProjectFilter` to avoid a double header. Keep a minimal header only for `modeView`/`modeCreating`/`modeSearch` if they relied on it. Concretely, replace lines 996-997 with:

```go
	var b strings.Builder
	pal := m.pal
	if m.mode == modeView || m.mode == modeCreating || m.mode == modeSearch {
		b.WriteString(theme.Heading("flow · docs", pal) + theme.Dim("  "+m.user, pal) + "\n\n")
	}
```

- [ ] **Step 1: Apply the mapping across `docs.go`**

Edit each occurrence per the table above. Ensure `pal := m.pal` is in scope in `renderView`, `renderCreate`, `renderFilter`, `renderSearch` (and any helper that renders).

- [ ] **Step 2: Verify no legacy style references remain**

Run: `rg -n 'style(Header|Muted|Sel|Ok|Err|Warn|Running|WikiValid|WikiBroken|WebLink|LinkFocus|SearchHit)|colAccent|colBg|colMuted|colGreen|colRed|colCyan|colPurple' internal/tui/docs.go`
Expected: **no output** (exit 1).

- [ ] **Step 3: Build + full docs tests**

Run: `go build ./... && go test ./internal/tui/...`
Expected: PASS (existing docs tests + new tests).

- [ ] **Step 4: Commit**

```bash
git add internal/tui/docs.go
git commit -m "refactor(tui): docs.go fully on theme.Sem(), off styles.go"
```

---

### Task 10: Cleanup verification + Phase-2 handoff note

**Files:**
- Modify: `docs/superpowers/specs/2026-06-19-flow-rebuild-docs-kompendium-look-design.md` (append a short status note) — or create a follow-up stub plan.

`styles.go` and the legacy `tui.New(...)` Model stay **for this plan only**: `cmd/flow/worktime.go:28` still wires them for standalone `flow worktime`. Removing the legacy worktime Model entirely (so the whole TUI is clean) is a **committed Phase 2** (own spec+plan). This task confirms docs no longer depends on `styles.go` and records the Phase-2 handoff.

- [ ] **Step 1: Confirm remaining `styles.go` consumers**

Run: `rg -l 'style[A-Z]|col[A-Z]' internal/tui/*.go`
Expected: `worktime.go`, `stats.go`, `dayoffs.go`, `export.go`, `styles.go` — **but not** `docs.go`.

- [ ] **Step 2: Record the follow-up**

Append to the spec file under "Out of scope / follow-ups":

```markdown
- **Phase 2 — Legacy `tui.New(...)` Model removal (committed):** `styles.go` +
  `worktime.go`/`stats.go`/`dayoffs.go`/`export.go` remain because `cmd/flow/worktime.go` still wires
  them for standalone `flow worktime`. Phase 2 migrates standalone `flow worktime` onto the modern
  `shell` + `screen/worktime` routes and then DELETES these files + `styles.go`, so the whole TUI is
  clean (no `col*`, no legacy Model). Own spec+plan.
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-06-19-flow-rebuild-docs-kompendium-look-design.md
git commit -m "docs(spec): record legacy tui.New Model removal as follow-up"
```

---

### Task 11: Wiring verification + done-gate

**Files:**
- Modify: `internal/tui/screen/docs/route.go` (`KeyHints` — add `p`)

- [ ] **Step 1: Add the `p` keyhint to the route**

In `KeyHints()` (route.go lines 69-77) add an entry:

```go
		{Key: "p", Desc: "Projekt"},
```

- [ ] **Step 2: Full CI**

Run: `make ci`
Expected: lint + templ + build + tests green; coverage gate satisfied.

- [ ] **Step 3: Manual done-gate (live dogfood vs dev stack)**

Start the dev stack and verify both surfaces render the kompendium look:

```bash
make dev-up
make dev-run   # in another shell, or per the dev README
# standalone:
flow docs
# shell tab:
flow ui docs
```

Confirm:
- Count header `kompendium — m/n Notizen · ● … ◆ … ○ …` with colored glyphs.
- Colored badges `TÄGL./PROJ./FREI/AGENT`, date cell, 2-line excerpt, selected-row stripe `▎`.
- Paginator dots + `i/total`; `j/k` crosses pages.
- `p` opens the project picker; selecting a project shows the colored `⟨ slug ⟩` chip and scopes the list while **daily/free stay visible**; "Alle Projekte" clears it.
- Existing behavior intact: `enter` opens the fullscreen markdown viewer (wikilink nav), `e` edits via `$EDITOR`, `n` creates, `D` deletes, `f` tag filter, `/` search, SSE live-reload updates the list.

- [ ] **Step 4: Commit (if any wiring changed)**

```bash
git add internal/tui/screen/docs/route.go
git commit -m "feat(tui): docs route keyhint for project filter"
```

---

## Self-Review

**1. Spec coverage:**
- Count header + badges + date + excerpt + stripe + paginator → Tasks 4,5,7. ✔
- Project chip + client-side inclusive filter → Tasks 6,8 + helper Task 5. ✔
- `docs.go` off `styles.go` onto `theme.Sem()` → Tasks 6,7,9. ✔
- Reusable design-language atoms (`badge`,`chip`,`countbar`) + central `kindcolor` → Tasks 1-4. ✔
- Native bubbles only for paginator → Task 7. ✔
- Preserve view/edit/new/delete/tag-filter/search/SSE/both surfaces → Tasks 7-9 (mode header guard), Task 11 done-gate. ✔
- Legacy cleanup verification-gated, no forced deletion → Task 10 (definitive: `styles.go` stays, follow-up recorded). ✔
- Testing strategy (component + pure-helper table tests) → Tasks 1-8. ✔

**2. Placeholder scan:** No "TBD/TODO". Task 9 is a fully-specified deterministic mapping (each style var → exact replacement) with a `rg` completion gate — not a placeholder.

**3. Type consistency:** `NewDocs(client, ed, op, pal, user)` used identically in Task 6 (def), `cmd/flow/docs.go`, `screen/docs/route.go`. `projFilter string` (ID) used consistently in Tasks 5/6/7/8. `projByID map[string]domain.Project` consistent. `countbar.Seg` fields (`Glyph/Label/N/Color`) match between Task 4 def and Task 7 use. `kindcolor.Badge/Glyph/Color` signatures match Task 1 def and Task 7 use. `applyProjectFilter(docs, projID)` signature consistent across Tasks 5/7/8. ✔
