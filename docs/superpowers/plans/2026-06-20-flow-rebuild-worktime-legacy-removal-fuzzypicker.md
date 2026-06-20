# Worktime Legacy Removal + Reusable Fuzzy/MRU Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the last legacy `tui.New(...)` worktime Model (and `styles.go`), make standalone `flow worktime` a worktime-only modern shell, and add a reusable `ui/fuzzylist` (fuzzy + MRU + inline-create) wired into both the worktime booking dialog and the docs project filter.

**Architecture:** Two new domain-free components — `ui/fuzzymatch` (subsequence matcher) and `ui/fuzzylist` (filterable list Model rendering via `picker.RowWithMatch`). MRU is derived client-side from `WorkSession` timestamps via a new `apiclient.ListSessionsSince` + a pure `mruProjects` helper. The legacy worktime Model files are deleted after relocating the four shared message symbols they own.

**Tech Stack:** Go, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, existing `internal/tui/theme` + `internal/tui/ui/*` + `internal/tui/shell` + `internal/tui/screen/worktime`.

## Global Constraints

- Module path `github.com/serverkraken/flow`. Imports: `domain` = `…/internal/domain`; `apiclient` = `…/internal/adapter/apiclient`; `theme` = `…/internal/tui/theme`; `picker` = `…/internal/tui/ui/picker`; `glyphs` = `…/internal/tui/ui/glyphs`; bubbletea = `tea "charm.land/bubbletea/v2"`.
- `ui/*` packages stay **domain-free** (no `internal/domain` import). `ui/fuzzymatch` and `ui/fuzzylist` are domain-free (operate on `Item{ID,Label string}`).
- Colors only from `theme.Palette`/`theme.Sem()`/builders — no hardcoded hex. No emoji outside `internal/tui/ui/glyphs`.
- Tests: `package <pkg>_test` for `ui/*` (external); internal `package <pkg>` only where unexported access is required (worktime/docs). `t.Parallel()`, plain `t.Errorf`/`t.Fatalf` (no testify). Palette via `theme.Default`.
- Fuzzy-mode navigation = `Up`/`Down` + `Ctrl+n`/`Ctrl+p` (a live text filter — typed runes incl. `j`/`k` go to the query). This matches the tui-usability filter-context exception.
- MRU window = 90 days. MRU applies to worktime only; the docs filter uses the project list order as-is.
- `make ci` (lint + templ + build + tests + coverage gate, currently 80%) must pass at each task boundary.
- German user-facing strings, proper umlauts.

---

## File Structure

**New files:**
- `internal/tui/events.go` — relocated shared message types (`errMsg`, `eventMsg`, `eventsReadyMsg`) + `waitForEvent`, used by the surviving `docs.go`.
- `internal/tui/ui/fuzzymatch/fuzzymatch.go` (+ `_test.go`) — subsequence matcher.
- `internal/tui/ui/fuzzylist/fuzzylist.go` (+ `_test.go`) — filterable list Model.
- `internal/tui/screen/worktime/mru.go` (+ `mru_test.go`) — `mruProjects` helper.

**Modified files:**
- `cmd/flow/worktime.go` — worktime-only shell.
- `internal/adapter/apiclient/client.go` (+ `worktime_test.go`) — `ListSessionsSince`.
- `internal/tui/screen/worktime/route.go` — `todayAPI` gains `ListSessionsSince`; booking load computes MRU; `projectsMsg` handler feeds the fuzzylist.
- `internal/tui/screen/worktime/dialogs.go` — `bookingState` → `fuzzylist.Model`.
- `internal/tui/docs.go` — `modeProjectFilter` → `fuzzylist.Model`.

**Deleted files:**
- `internal/tui/worktime.go`, `stats.go`, `dayoffs.go`, `export.go` and their `_test.go` files.
- `internal/tui/styles.go`.

---

### Task 1: Legacy removal + worktime-only shell

**Files:**
- Create: `internal/tui/events.go`
- Modify: `cmd/flow/worktime.go`
- Delete: `internal/tui/worktime.go`, `stats.go`, `dayoffs.go`, `export.go` (+ their `_test.go`), `internal/tui/styles.go`

**Interfaces:**
- Consumes: `shell.New`, `shell.Route`, `worktime.NewTodayRoute`, `worktime.BuildRegistry`, `theme.Load`.
- Produces: nothing new for later tasks (pure removal + rewire). `events.go` keeps `errMsg`, `eventMsg`, `eventsReadyMsg`, `waitForEvent` available to `docs.go` in package `tui`.

This task has no new unit test; its gate is `make ci` (existing `screen/worktime` + `docs` suites stay green) + a build + a `tui.New` absence check.

- [ ] **Step 1: Relocate shared symbols into `internal/tui/events.go`**

These four symbols are currently defined in `internal/tui/worktime.go` (lines ~76-79, ~140) but are used by the surviving `docs.go`. Create `internal/tui/events.go` with exactly:

```go
package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// errMsg carries an async error into Update.
type errMsg struct{ err error }

// eventMsg is one SSE client event delivered to a model.
type eventMsg struct{ ev apiclient.ClientEvent }

// eventsReadyMsg hands a model the live SSE channel after subscribe().
type eventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }

// waitForEvent blocks on the SSE channel and re-delivers the next event.
func waitForEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{ev: ev}
	}
}
```

Note: verify `waitForEvent`'s body matches the current one in `worktime.go` before deleting (open `internal/tui/worktime.go` around line 140 and copy its exact body if it differs from the above; the channel-closed guard must be preserved).

- [ ] **Step 2: Rewrite `cmd/flow/worktime.go`**

Replace the whole file with:

```go
package main

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			pal := theme.Load()
			m := shell.New(client, os.Getenv("USER"), pal).
				WithTabs([]shell.Route{
					worktime.NewTodayRoute(client, time.Now, pal, worktime.BuildRegistry(client, pal)),
				})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
}
```

- [ ] **Step 3: Delete the legacy Model files**

```bash
git rm internal/tui/worktime.go internal/tui/worktime_test.go \
       internal/tui/stats.go internal/tui/stats_test.go \
       internal/tui/dayoffs.go internal/tui/dayoffs_test.go \
       internal/tui/export.go internal/tui/export_test.go \
       internal/tui/styles.go
```

(If any of those `_test.go` names differ, `git rm` will error — list `internal/tui/*_test.go` first and remove the worktime/stats/dayoffs/export ones.)

- [ ] **Step 4: Build and resolve any remaining undefined symbols**

Run: `go build ./...`
Expected: clean. If the build reports `undefined: X` in `docs.go`/`docs_render.go`/`weblink.go`, that symbol was defined in a deleted file and is still needed — recover its definition from git (`git show HEAD:internal/tui/worktime.go`) and move it into `internal/tui/events.go` (for messages/helpers) or `docs.go`. The expected set was `errMsg`/`eventMsg`/`eventsReadyMsg`/`waitForEvent` (handled in Step 1); anything else, move it the same way. `glyphOr` and the legacy view/parse helpers are legacy-only and should NOT reappear.

- [ ] **Step 5: Verify legacy is gone + full CI**

Run: `rg -n 'tui\.New\b' --type go` → expect NO output (only `tui.NewDocs` remains, which won't match `tui\.New\b`… confirm: `rg -n 'tui\.New\b' cmd internal` returns nothing).
Run: `rg -n 'col[A-Z]|styleHeader|styleSel|styleRunning|styleWarn' internal/tui/` → expect NO output (styles.go gone; `styleBodyLine` is a function name, won't match these).
Run: `make ci`
Expected: green (the modern `screen/worktime` + `docs` suites cover behavior).

- [ ] **Step 6: Binary smoke**

Run: `go run ./cmd/flow worktime --help` and `go run ./cmd/flow ui --help`
Expected: both print help without panic.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/events.go cmd/flow/worktime.go
git commit -am "refactor(tui): remove legacy worktime Model + styles.go; flow worktime = worktime-only shell"
```

(`-am` also stages the `git rm` deletions.)

---

### Task 2: ui/fuzzymatch — subsequence matcher

**Files:**
- Create: `internal/tui/ui/fuzzymatch/fuzzymatch.go`
- Test: `internal/tui/ui/fuzzymatch/fuzzymatch_test.go`

**Interfaces:**
- Produces: `func Match(query, target string) (idx []int, score int, ok bool)` — case-insensitive subsequence; `idx` = rune indices in `target` that matched (for `picker.RowWithMatch`); higher `score` = better (contiguous + early); empty query → `ok=true, idx=nil, score=0`; no full match → `ok=false, idx=nil`.

- [ ] **Step 1: Write the failing test**

```go
package fuzzymatch_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

func TestMatch_SubsequenceAndIndices(t *testing.T) {
	t.Parallel()
	idx, _, ok := fuzzymatch.Match("fb", "foobar")
	if !ok {
		t.Fatal("fb should match foobar")
	}
	// f at 0, b at 3
	if len(idx) != 2 || idx[0] != 0 || idx[1] != 3 {
		t.Errorf("idx = %v, want [0 3]", idx)
	}
}

func TestMatch_CaseInsensitiveAndEmptyAndNoMatch(t *testing.T) {
	t.Parallel()
	if _, _, ok := fuzzymatch.Match("FOO", "foobar"); !ok {
		t.Error("FOO should match foobar case-insensitively")
	}
	if idx, score, ok := fuzzymatch.Match("", "anything"); !ok || idx != nil || score != 0 {
		t.Errorf("empty query: got idx=%v score=%d ok=%v, want nil/0/true", idx, score, ok)
	}
	if idx, _, ok := fuzzymatch.Match("zzz", "foobar"); ok || idx != nil {
		t.Errorf("zzz should not match foobar (idx=%v ok=%v)", idx, ok)
	}
}

func TestMatch_ScoreFavoursContiguousEarly(t *testing.T) {
	t.Parallel()
	_, contig, _ := fuzzymatch.Match("ab", "abc")  // contiguous + at start
	_, spread, _ := fuzzymatch.Match("ab", "axxb")  // spread + later
	if contig <= spread {
		t.Errorf("contiguous-early score %d should beat spread-late %d", contig, spread)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/fuzzymatch/...`
Expected: FAIL (package not defined).

- [ ] **Step 3: Write the implementation**

```go
// Package fuzzymatch is a tiny case-insensitive subsequence matcher with match
// indices and a quality score, for filterable lists. Domain-free, no deps.
package fuzzymatch

import "strings"

// Match reports whether query is a case-insensitive subsequence of target.
// idx are the rune indices in target that matched (for highlight); a higher
// score means a tighter match (contiguous and early runes score more). An empty
// query matches everything (ok=true, idx=nil, score=0). When not all query runes
// are found, ok=false and idx=nil.
func Match(query, target string) (idx []int, score int, ok bool) {
	if query == "" {
		return nil, 0, true
	}
	q := []rune(strings.ToLower(query))
	tl := []rune(strings.ToLower(target))
	qi := 0
	prev := -2
	for ti := 0; ti < len(tl) && qi < len(q); ti++ {
		if tl[ti] != q[qi] {
			continue
		}
		idx = append(idx, ti)
		score += 10
		if ti == prev+1 {
			score += 5 // contiguous with previous match
		}
		if ti == 0 {
			score += 5 // matches at the very start
		}
		prev = ti
		qi++
	}
	if qi < len(q) {
		return nil, 0, false
	}
	// Prefer tighter targets: subtract the unmatched-length slack.
	score -= len(tl) - len(idx)
	return idx, score, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/fuzzymatch/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/fuzzymatch/
git commit -m "feat(tui): ui/fuzzymatch subsequence matcher"
```

---

### Task 3: ui/fuzzylist — filterable list Model

**Files:**
- Create: `internal/tui/ui/fuzzylist/fuzzylist.go`
- Test: `internal/tui/ui/fuzzylist/fuzzylist_test.go`

**Interfaces:**
- Consumes: `fuzzymatch.Match`, `picker.RowWithMatch`/`picker.Row`, `glyphs.Extra`, `theme.Palette`.
- Produces:
  - `type Item struct{ ID, Label string }`
  - `func New(items []Item, pal theme.Palette) Model`
  - `func (m Model) WithCreateHint(hint string) Model`
  - `func (m Model) SetItems(items []Item) Model`
  - `func (m Model) Update(k tea.KeyPressMsg) Model`
  - `func (m Model) View(width int) string`
  - `func (m Model) Query() string`
  - `func (m Model) Selection() (it Item, isCreate, ok bool)`

- [ ] **Step 1: Write the failing test**

```go
package fuzzylist_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

func items() []fuzzylist.Item {
	return []fuzzylist.Item{{ID: "1", Label: "serverkraken/flow"}, {ID: "2", Label: "backstage"}, {ID: "3", Label: "oraya"}}
}

func TestFilterNarrowsAndSelects(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "o"}) // matches flow(o), backstage? no 'o'… 'oraya','flow' have o
	if m.Query() != "o" {
		t.Fatalf("query = %q, want o", m.Query())
	}
	it, isCreate, ok := m.Selection()
	if !ok || isCreate {
		t.Fatalf("expected a real selection, got ok=%v isCreate=%v", ok, isCreate)
	}
	_ = it
}

func TestTypingRoutesToQueryNotNav(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "j"}) // 'j' is a typed char, not navigation
	if m.Query() != "j" {
		t.Errorf("query = %q, want j (j must be typed, not navigation)", m.Query())
	}
}

func TestArrowAndCtrlNavigation(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default) // 3 items, cursor 0
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if _, _, ok := m.Selection(); !ok {
		t.Fatal("selection should exist after Down")
	}
	m = m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}) // ctrl+n = down
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})           // clamp at last
	it, _, _ := m.Selection()
	if it.ID != "3" {
		t.Errorf("cursor should clamp at last item (oraya), got %q", it.ID)
	}
	m = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) // ctrl+p = up
	if it, _, _ := m.Selection(); it.ID != "2" {
		t.Errorf("ctrl+p should move up to backstage, got %q", it.ID)
	}
}

func TestInlineCreate(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(items(), theme.Default).WithCreateHint("neu: %s")
	m = m.Update(tea.KeyPressMsg{Text: "z"}) // no item matches 'z' exactly → create row appears
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // move onto the create row (it's last)
	// move cursor to the create row regardless of how many matched
	for i := 0; i < 5; i++ {
		m = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	it, isCreate, ok := m.Selection()
	if !ok || !isCreate {
		t.Fatalf("expected create selection, got it=%+v isCreate=%v ok=%v", it, isCreate, ok)
	}
	if m.Query() != "z" {
		t.Errorf("query for create = %q, want z", m.Query())
	}
}

func TestSetItemsPreservesQuery(t *testing.T) {
	t.Parallel()
	m := fuzzylist.New(nil, theme.Default)
	m = m.Update(tea.KeyPressMsg{Text: "f"})
	m = m.SetItems(items())
	if m.Query() != "f" {
		t.Errorf("SetItems dropped query: %q", m.Query())
	}
	if it, _, ok := m.Selection(); !ok || it.ID != "1" {
		t.Errorf("after SetItems with query 'f', expected flow selected, got %+v ok=%v", it, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/fuzzylist/...`
Expected: FAIL (package not defined).

- [ ] **Step 3: Write the implementation**

```go
// Package fuzzylist is a reusable, domain-free filterable list Model: type to
// fuzzy-filter, Up/Down (or Ctrl+n/Ctrl+p) to move, optional inline-create row.
// The caller owns Enter/Esc and the meaning of a selection.
package fuzzylist

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/picker"
)

// Item is one selectable entry. ID is opaque to the component.
type Item struct{ ID, Label string }

type entry struct {
	item Item
	idx  []int
}

// Model is a value type; Update/SetItems/etc. return a new copy.
type Model struct {
	items      []Item
	query      string
	filtered   []entry
	cursor     int
	pal        theme.Palette
	createHint string // e.g. "neu: %s"; empty disables the inline-create row
}

// New builds a list over items (kept in the given order; the caller supplies
// e.g. MRU order). The empty query shows them all.
func New(items []Item, pal theme.Palette) Model {
	return (Model{items: items, pal: pal}).refilter()
}

// WithCreateHint enables an inline-create row using hint as a printf format over
// the current query (e.g. "neu: %s").
func (m Model) WithCreateHint(hint string) Model { m.createHint = hint; return m.refilter() }

// SetItems replaces the items and re-filters, preserving the current query.
func (m Model) SetItems(items []Item) Model { m.items = items; return m.refilter() }

// Query is the current filter text.
func (m Model) Query() string { return m.query }

func (m Model) createActive() bool {
	if m.createHint == "" || strings.TrimSpace(m.query) == "" {
		return false
	}
	for _, it := range m.items {
		if strings.EqualFold(it.Label, m.query) {
			return false
		}
	}
	return true
}

func (m Model) rowCount() int {
	n := len(m.filtered)
	if m.createActive() {
		n++
	}
	return n
}

// Selection returns the entry under the cursor. isCreate is true when the cursor
// is on the inline-create row (the caller should create using Query()). ok is
// false when there is nothing selectable.
func (m Model) Selection() (it Item, isCreate, ok bool) {
	if m.createActive() && m.cursor == len(m.filtered) {
		return Item{}, true, true
	}
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor].item, false, true
	}
	return Item{}, false, false
}

// Update handles text→query, Backspace, and cursor movement (Up/Down +
// Ctrl+n/Ctrl+p). Every other rune (including j/k) is typed into the query.
func (m Model) Update(k tea.KeyPressMsg) Model {
	switch {
	case k.Code == tea.KeyUp || (k.Code == 'p' && k.Mod == tea.ModCtrl):
		if m.cursor > 0 {
			m.cursor--
		}
		return m
	case k.Code == tea.KeyDown || (k.Code == 'n' && k.Mod == tea.ModCtrl):
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		return m
	case k.Code == tea.KeyBackspace:
		if rn := []rune(m.query); len(rn) > 0 {
			m.query = string(rn[:len(rn)-1])
		}
		return m.refilter()
	case k.Text != "":
		m.query += k.Text
		return m.refilter()
	}
	return m
}

func (m Model) refilter() Model {
	out := make([]entry, 0, len(m.items))
	if m.query == "" {
		for _, it := range m.items {
			out = append(out, entry{item: it})
		}
	} else {
		type scored struct {
			e     entry
			score int
			ord   int
		}
		var ss []scored
		for i, it := range m.items {
			if idx, score, ok := fuzzymatch.Match(m.query, it.Label); ok {
				ss = append(ss, scored{e: entry{item: it, idx: idx}, score: score, ord: i})
			}
		}
		sort.SliceStable(ss, func(a, b int) bool {
			if ss[a].score != ss[b].score {
				return ss[a].score > ss[b].score
			}
			return ss[a].ord < ss[b].ord
		})
		for _, s := range ss {
			out = append(out, s.e)
		}
	}
	m.filtered = out
	if m.cursor >= m.rowCount() {
		m.cursor = m.rowCount() - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

// View renders the filtered rows (with match highlight) plus the inline-create
// row when active. width is the available content width.
func (m Model) View(width int) string {
	var b strings.Builder
	for i, e := range m.filtered {
		b.WriteString(picker.RowWithMatch(picker.RowWithMatchOpts{
			Selected: i == m.cursor,
			Label:    e.item.Label,
			Width:    width,
			Match:    e.idx,
		}, m.pal) + "\n")
	}
	if m.createActive() {
		label := glyphs.Extra + " " + fmt.Sprintf(m.createHint, m.query)
		b.WriteString(picker.Row(m.cursor == len(m.filtered), label, "", width, m.pal) + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/fuzzylist/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/fuzzylist/
git commit -m "feat(tui): ui/fuzzylist filterable list with inline-create"
```

---

### Task 4: apiclient.ListSessionsSince

**Files:**
- Modify: `internal/adapter/apiclient/client.go`
- Test: `internal/adapter/apiclient/worktime_test.go`

**Interfaces:**
- Produces: `func (c *Client) ListSessionsSince(ctx context.Context, since time.Time) ([]domain.WorkSession, error)` — `GET /api/v1/sessions?since=<RFC3339>`.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapter/apiclient/worktime_test.go` (mirror the httptest setup already used in this package, e.g. `dayoffs_test.go`: `ts := httptest.NewServer(...)`, `c := apiclient.New(ts.URL, "tok")`):

```go
func TestClient_ListSessionsSince(t *testing.T) {
	var gotSince string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"s1","start":"2026-06-01T09:00:00Z"}]`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	since := time.Date(2026, 3, 22, 8, 0, 0, 0, time.UTC)
	out, err := c.ListSessionsSince(context.Background(), since)
	if err != nil {
		t.Fatalf("ListSessionsSince: %v", err)
	}
	if len(out) != 1 || out[0].ID != "s1" {
		t.Fatalf("decoded sessions = %+v, want one with id s1", out)
	}
	if got, want := gotSince, since.Format(time.RFC3339); got != want {
		t.Errorf("since query = %q, want %q", got, want)
	}
}
```

Ensure the test file imports `context`, `net/http`, `net/http/httptest`, `testing`, `time`, and the `apiclient` package (mirror sibling test imports).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run ListSessionsSince`
Expected: FAIL (`c.ListSessionsSince` undefined).

- [ ] **Step 3: Write the implementation**

Add to `internal/adapter/apiclient/client.go` (near `ListSessions`). Ensure `net/url` and `time` are imported:

```go
// ListSessionsSince returns sessions with start >= since.
func (c *Client) ListSessionsSince(ctx context.Context, since time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	path := "/api/v1/sessions?since=" + url.QueryEscape(since.Format(time.RFC3339))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/apiclient/ -run ListSessionsSince`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/worktime_test.go
git commit -m "feat(apiclient): ListSessionsSince(?since=) for MRU"
```

---

### Task 5: mruProjects helper

**Files:**
- Create: `internal/tui/screen/worktime/mru.go`
- Test: `internal/tui/screen/worktime/mru_test.go`

**Interfaces:**
- Produces: `func mruProjects(projects []domain.Project, sessions []domain.WorkSession) []domain.Project` — most-recently-used first (latest `Stop`, else `Start`); unused projects keep original relative order and trail used ones. Pure; does not mutate inputs.

- [ ] **Step 1: Write the failing test**

```go
package worktime

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestMruProjects_RecentFirstUnusedTrail(t *testing.T) {
	t.Parallel()
	projects := []domain.Project{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	t1 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	pa, pc := "a", "c"
	stop2 := t2.Add(time.Hour)
	sessions := []domain.WorkSession{
		{ProjectID: &pa, Start: t1, Stop: ptr(t1.Add(time.Hour))}, // a used at ~t1
		{ProjectID: &pc, Start: t2, Stop: &stop2},                 // c used at ~t2 (more recent)
	}
	got := mruProjects(projects, sessions)
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	// c (most recent) > a (older) > b (unused, original order)
	if ids[0] != "c" || ids[1] != "a" || ids[2] != "b" {
		t.Errorf("mru order = %v, want [c a b]", ids)
	}
}

func TestMruProjects_RunningSessionUsesStart(t *testing.T) {
	t.Parallel()
	projects := []domain.Project{{ID: "a"}, {ID: "b"}}
	pb := "b"
	recent := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	sessions := []domain.WorkSession{{ProjectID: &pb, Start: recent}} // running, no Stop
	got := mruProjects(projects, sessions)
	if got[0].ID != "b" {
		t.Errorf("running session should rank b first, got %v", got[0].ID)
	}
}

func ptr(t time.Time) *time.Time { return &t }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/ -run MruProjects`
Expected: FAIL (`mruProjects` undefined).

- [ ] **Step 3: Write the implementation**

```go
package worktime

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// mruProjects orders projects by most-recently-used. A project's recency is the
// latest session referencing it (Stop if set, else Start). Projects with no
// session keep their original relative order and trail the used ones. Pure.
func mruProjects(projects []domain.Project, sessions []domain.WorkSession) []domain.Project {
	last := make(map[string]time.Time, len(projects))
	for _, s := range sessions {
		if s.ProjectID == nil {
			continue
		}
		t := s.Start
		if s.Stop != nil {
			t = *s.Stop
		}
		if cur, ok := last[*s.ProjectID]; !ok || t.After(cur) {
			last[*s.ProjectID] = t
		}
	}
	idxOf := make(map[string]int, len(projects))
	for i, p := range projects {
		idxOf[p.ID] = i
	}
	out := append([]domain.Project(nil), projects...)
	sort.SliceStable(out, func(a, b int) bool {
		ta, oka := last[out[a].ID]
		tb, okb := last[out[b].ID]
		if oka != okb {
			return oka // used projects come first
		}
		if oka && okb && !ta.Equal(tb) {
			return ta.After(tb) // more recent first
		}
		return idxOf[out[a].ID] < idxOf[out[b].ID] // stable original order
	})
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/screen/worktime/ -run MruProjects`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/mru.go internal/tui/screen/worktime/mru_test.go
git commit -m "feat(worktime): mruProjects helper (recency-ordered)"
```

---

### Task 6: Worktime booking dialog → fuzzylist (MRU + fuzzy + inline-create)

**Files:**
- Modify: `internal/tui/screen/worktime/route.go` (`todayAPI` interface; `projectsMsg` handler)
- Modify: `internal/tui/screen/worktime/dialogs.go` (`bookingState`, `startOrStop`, `handleBookingKey`, `renderBooking`, `dialogHints`)
- Test: `internal/tui/screen/worktime/dialogs_test.go` (create or append)

**Interfaces:**
- Consumes: `fuzzylist.New/WithCreateHint/SetItems/Update/View/Selection/Query`, `mruProjects` (Task 5), `apiclient.ListSessionsSince` via the interface (Task 4).
- Produces: `bookingState{ list fuzzylist.Model }`; `projectItems(ps []domain.Project) []fuzzylist.Item`.

- [ ] **Step 1: Extend the `todayAPI` interface**

In `internal/tui/screen/worktime/route.go`, add to the `todayAPI` interface (after `ListSessions`):

```go
	ListSessionsSince(context.Context, time.Time) ([]domain.WorkSession, error)
```

(`*apiclient.Client` already satisfies it after Task 4.)

- [ ] **Step 2: Write the failing test**

Create `internal/tui/screen/worktime/dialogs_test.go` (internal `package worktime`). This test drives the booking key handler through a fuzzylist selection. Use a minimal fake `todayAPI`:

```go
package worktime

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

type fakeAPI struct {
	stopProjectID string
	created       string
}

func (f *fakeAPI) GetToday(context.Context) (apiclient.Today, error) { return apiclient.Today{}, nil }
func (f *fakeAPI) ListSessions(context.Context) ([]domain.WorkSession, error) { return nil, nil }
func (f *fakeAPI) ListSessionsSince(context.Context, time.Time) ([]domain.WorkSession, error) {
	return nil, nil
}
func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error) { return nil, nil }
func (f *fakeAPI) StartSession(context.Context, *string, string, string) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f *fakeAPI) StopSession(_ context.Context, _, projectID string) (domain.WorkSession, error) {
	f.stopProjectID = projectID
	return domain.WorkSession{}, nil
}
func (f *fakeAPI) EditSession(context.Context, string, *string, string, string, time.Time, *time.Time) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f *fakeAPI) DeleteSession(context.Context, string) error { return nil }
func (f *fakeAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	f.created = name
	return domain.Project{ID: "new-id", Name: name}, nil
}

func bookingRoute(api todayAPI) *TodayRoute {
	r := NewTodayRoute(api, time.Now, theme.Default, nil)
	r.st.Running = true
	r.st.ActiveID = "sess-1"
	r.dialog = dialogBooking
	r.booking = bookingState{list: fuzzylist.New([]fuzzylist.Item{{ID: "p1", Label: "flow"}}, theme.Default).WithCreateHint("neu: %s")}
	return r
}

func TestBooking_SelectExistingProject(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	r := bookingRoute(api)
	// cursor on the single project, Enter → StopSession with p1
	_, cmd := r.handleBookingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a stop command")
	}
	cmd() // run the async closure
	if api.stopProjectID != "p1" {
		t.Errorf("stopProjectID = %q, want p1", api.stopProjectID)
	}
}

func TestBooking_InlineCreate(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	r := bookingRoute(api)
	// type a new name, move onto the create row, Enter → CreateProject + Stop
	r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Text: "z"})
	for i := 0; i < 5; i++ {
		r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, cmd := r.handleBookingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a create+stop command")
	}
	cmd()
	if api.created != "z" {
		t.Errorf("created project = %q, want z", api.created)
	}
	if api.stopProjectID != "new-id" {
		t.Errorf("stopProjectID = %q, want new-id", api.stopProjectID)
	}
}
```

(If `TodayRoute` fields `st.Running`/`st.ActiveID` differ in name, open `today_state.go` and adjust the test setters to the real field names — the assertions stay the same.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/ -run Booking`
Expected: FAIL (booking still uses old `projects/sel/newName` shape; `list` field undefined).

- [ ] **Step 4: Rewrite the booking state + handlers in `dialogs.go`**

Replace `bookingState` and its functions:

```go
type bookingState struct {
	list fuzzylist.Model
}

func projectItems(ps []domain.Project) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(ps))
	for _, p := range ps {
		out = append(out, fuzzylist.Item{ID: p.ID, Label: p.Name})
	}
	return out
}

func (r *TodayRoute) startOrStop() (shell.Route, tea.Cmd) {
	if !r.st.Running {
		api := r.api
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StartSession(ctx, nil, "", ""); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	}
	r.dialog = dialogBooking
	r.booking = bookingState{list: fuzzylist.New(nil, r.pal).WithCreateHint("neu: %s")}
	api := r.api
	since := r.now().AddDate(0, 0, -90)
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, _ := api.ListProjects(ctx)
		ss, _ := api.ListSessionsSince(ctx, since)
		return projectsMsg{projects: mruProjects(ps, ss)}
	}
}

func (r *TodayRoute) handleBookingKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case k.Code == tea.KeyEnter:
		it, isCreate, ok := r.booking.list.Selection()
		if !ok {
			return r, nil
		}
		id := r.st.ActiveID
		api := r.api
		r.dialog = dialogNone
		if isCreate {
			name := strings.TrimSpace(r.booking.list.Query())
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				p, err := api.CreateProject(ctx, name)
				if err != nil {
					return loadedMsg{err: err}
				}
				if _, err := api.StopSession(ctx, id, p.ID); err != nil {
					return loadedMsg{err: err}
				}
				return reloadMsg{}
			}
		}
		pid := it.ID
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.StopSession(ctx, id, pid); err != nil {
				return loadedMsg{err: err}
			}
			return reloadMsg{}
		}
	default:
		r.booking.list = r.booking.list.Update(k)
		return r, nil
	}
}

func (r *TodayRoute) renderBooking(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  Projekt buchen  ")
	b.WriteString(theme.Dim("tippen → filtern  ·  ↑/↓ → wählen  ·  enter → buchen  ·  esc", f.Pal))
	b.WriteString("\n\n")
	b.WriteString(r.booking.list.View(f.Width - 4))
	return b.String()
}
```

Update the booking `dialogHints` case to the new nav:

```go
	case dialogBooking:
		return []keyhint.Hint{{Key: "↑/↓", Desc: "wählen"}, {Key: "enter", Desc: "buchen"}, {Key: "esc", Desc: "abbrechen"}}
```

Add imports to `dialogs.go`: `"github.com/serverkraken/flow/internal/tui/theme"` and `"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"`. Remove the now-unused `picker` import if nothing else in the file uses it (the edit dialog uses `form`, not `picker` — `go build` will flag it).

- [ ] **Step 5: Feed the fuzzylist from the `projectsMsg` handler in `route.go`**

Replace the `projectsMsg` case (route.go ~129-134) with:

```go
	case projectsMsg:
		r.booking.list = r.booking.list.SetItems(projectItems(m.projects))
		return r, nil
```

- [ ] **Step 6: Run tests + build**

Run: `go build ./... && go test ./internal/tui/screen/worktime/ -run Booking`
Expected: PASS. Then `go test ./internal/tui/screen/worktime/...` (no regression in existing worktime tests).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/worktime/
git commit -m "feat(worktime): booking dialog uses fuzzylist (MRU + fuzzy + inline-create)"
```

---

### Task 7: Docs project filter → fuzzylist

**Files:**
- Modify: `internal/tui/docs.go` (`DocsModel` field; `p` case; `handleProjectFilterKey`; `renderProjectFilter`; `footer`)
- Test: `internal/tui/docs_render_test.go` (append)

**Interfaces:**
- Consumes: `fuzzylist.New/SetItems/Update/View/Selection`.
- Produces: `projectFilterItems(ps []domain.Project) []fuzzylist.Item` (prepends an "Alle Projekte" entry with empty ID).

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/docs_render_test.go`:

```go
func TestProjectFilter_FuzzySelectAndClear(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.projects = []domain.Project{{ID: "p1", Slug: "serverkraken/flow"}, {ID: "p2", Slug: "other/repo"}}
	// open the filter
	nm, _ := m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	if m.mode != modeProjectFilter {
		t.Fatalf("mode = %v, want modeProjectFilter", m.mode)
	}
	// type to fuzzy-match "other/repo", then Enter selects it
	for _, r := range "repo" {
		nm, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = nm.(DocsModel)
	}
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "p2" {
		t.Fatalf("projFilter = %q, want p2", m.projFilter)
	}
	// reopen, Enter on the first row ("Alle Projekte") clears
	nm, _ = m.Update(tea.KeyPressMsg{Text: "p"})
	m = nm.(DocsModel)
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.projFilter != "" {
		t.Fatalf("after Alle: projFilter = %q, want empty", m.projFilter)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run ProjectFilter_FuzzySelectAndClear`
Expected: FAIL (still the `projCursor` j/k picker; fuzzy typing won't select).

- [ ] **Step 3: Swap the DocsModel field**

In the `DocsModel` struct, replace:

```go
	projCursor int    // cursor in the project-filter picker
```

with:

```go
	projList fuzzylist.Model // project-filter picker (fuzzy)
```

Add the import `"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"` to `docs.go`.

- [ ] **Step 4: Add the items helper + rewrite the `p` case, handler, render, footer**

Add the helper (near the other docs helpers):

```go
func projectFilterItems(ps []domain.Project) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(ps)+1)
	out = append(out, fuzzylist.Item{ID: "", Label: "Alle Projekte"})
	for _, p := range ps {
		out = append(out, fuzzylist.Item{ID: p.ID, Label: p.Slug})
	}
	return out
}
```

Replace the modeList `p` case:

```go
	case k.Text == "p":
		m.mode = modeProjectFilter
		m.projList = fuzzylist.New(projectFilterItems(m.projects), m.pal)
		return m, nil
```

Replace `handleProjectFilterKey`:

```go
func (m DocsModel) handleProjectFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList
		return m, nil
	case k.Code == tea.KeyEnter:
		if it, _, ok := m.projList.Selection(); ok {
			m.projFilter = it.ID // "" = Alle Projekte
		}
		m.mode = modeList
		m.sel = 0
		return m, nil
	default:
		m.projList = m.projList.Update(k)
		return m, nil
	}
}
```

Replace `renderProjectFilter`:

```go
func (m DocsModel) renderProjectFilter(b *strings.Builder) {
	pal := m.pal
	b.WriteString(theme.Heading("Projekt-Filter", pal) + "  ")
	b.WriteString(theme.Dim("tippen → filtern  ·  ↑/↓ → wählen  ·  enter → anwenden  ·  esc", pal))
	b.WriteString("\n\n")
	width := m.width
	if width < 20 {
		width = 60
	}
	b.WriteString(m.projList.View(width - 4))
}
```

Replace the `modeProjectFilter` footer case:

```go
	case modeProjectFilter:
		return "tippen → filtern · ↑/↓ wählen · enter anwenden · esc abbrechen"
```

- [ ] **Step 5: Run test + build**

Run: `go build ./... && go test ./internal/tui/ -run ProjectFilter`
Expected: PASS (both the new fuzzy test and the Phase-1 `TestProjectFilter_OpenSelectClear` — wait: that Phase-1 test drove `projCursor` directly and will now fail to compile). If `TestProjectFilter_OpenSelectClear` references `projCursor`, update it to drive via keys (type + Enter) like the new test, OR delete it as superseded by `TestProjectFilter_FuzzySelectAndClear`. Note which you did in the report.

Then: `go test ./internal/tui/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_render_test.go
git commit -m "feat(docs): project filter uses fuzzylist (fuzzy + Alle)"
```

---

### Task 8: Done-gate

- [ ] **Step 1: Full CI**

Run: `make ci`
Expected: lint + templ + build + tests green; coverage gate satisfied.

- [ ] **Step 2: Confirm legacy is fully gone**

Run: `rg -n 'tui\.New\b' cmd internal` → no output.
Run: `fd styles.go internal/tui` → no output.
Run: `fd -e go -d 1 . internal/tui | rg -v '_test.go'` → only `docs.go`, `docs_render.go`, `weblink.go`, `events.go`.

- [ ] **Step 3: Manual done-gate (live dogfood vs dev stack)**

```bash
make dev-up && make dev-run   # per dev README
go run ./cmd/flow worktime     # opens worktime-only shell (Heute; w/t/d/e → siblings)
go run ./cmd/flow ui           # full shell unaffected
```

Confirm:
- `flow worktime` opens directly into Heute; `w/t/d/e` reach Woche/Stats/Frei/Export.
- Stop a session → booking dialog: typing fuzzy-filters projects (match-highlight), MRU order when query empty, `↑/↓`/`Ctrl+n`/`Ctrl+p` navigate, a non-existing name shows `✚ neu: …` and Enter creates+books it, an existing project Enter books it.
- In `flow docs`, `p` opens the project filter: typing fuzzy-filters, Enter selects (chip + scoped list), "Alle Projekte" clears.

- [ ] **Step 4: Commit (if anything adjusted during the gate)**

```bash
git commit -am "test(tui): phase-2 done-gate adjustments"
```

---

## Self-Review

**1. Spec coverage:**
- Delete legacy Model + `styles.go` → Task 1. ✔
- `flow worktime` worktime-only shell → Task 1. ✔
- `ui/fuzzymatch` + `ui/fuzzylist` → Tasks 2, 3. ✔
- `apiclient.ListSessionsSince` + `mruProjects` (90d) → Tasks 4, 5. ✔
- Worktime booking wiring (MRU + fuzzy + inline-create) → Task 6. ✔
- Docs filter wiring (fuzzy + "Alle") → Task 7. ✔
- Fuzzy-nav `↑/↓`+`Ctrl+n/p`; `dayoff`/`export` CLI untouched; domain-free `ui/*` → enforced in Global Constraints + component code. ✔
- Done-gate + legacy-absence checks → Task 8. ✔

**2. Placeholder scan:** No "TBD/TODO". Task 1 Step 4's "resolve undefined symbols" is a fully-specified deterministic recovery (the expected set is named; the build is the gate). Task 4's test reuses the documented sibling httptest scaffold (`apiclient.New(ts.URL,"tok")`). Task 6/7 note the one pre-existing test each that may need updating, with the concrete action.

**3. Type consistency:** `fuzzylist.Item{ID,Label}`, `New/WithCreateHint/SetItems/Update/View/Selection/Query` used identically in Tasks 3, 6, 7. `Selection() (Item, bool, bool)` (it, isCreate, ok) consistent. `mruProjects(projects, sessions)` signature matches Task 5 def and Task 6 use. `ListSessionsSince(ctx, time.Time)` matches Task 4 def, the `todayAPI` interface addition (Task 6 Step 1), and `fakeAPI`. `projectItems`/`projectFilterItems` are distinct helpers (worktime vs docs) — intentional, different label fields (Name vs Slug) and the docs one prepends "Alle Projekte". ✔
