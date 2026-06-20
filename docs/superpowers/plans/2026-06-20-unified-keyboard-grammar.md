# Unified Keyboard Grammar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give flow's TUI one default-user keyboard grammar (↑/↓ move, q/Esc walk back then quit, Home/End, / search; no j/k), captured as reusable infrastructure: a `ui/grammar` binding registry that feeds both handlers and hints, a shared `ui/listnav` cursor, and one `ResolveBack` function for shell + host.

**Architecture:** A new `ui/grammar` package defines `Binding`s (keys + canonical German hint) as the single source for behaviour and advertised hints. `ui/listnav.Cursor` matches movement keys against those bindings (clamp, no wrap). Back/quit becomes one pure `ResolveBack` function consumed by both the Shell and the chrome-less RouteHost, driven by two small route capabilities: `CapturesInput` (forward non-back keys — text fields *and* the doc viewer) and a new `CapturesText` (forward back keys too — literal text fields only), plus a `Backer` for a route's internal back (doc-view → list, clear filter).

**Tech Stack:** Go, charm.land/bubbletea/v2 (`tea.KeyPressMsg{Code, Text, Mod}`, key constants `KeyUp/KeyDown/KeyHome/KeyEnd/KeyPgUp/KeyPgDown/KeyEnter/KeyEsc`), charm.land/lipgloss/v2, existing `internal/tui/ui/{keyhint,help,strings,theme}`.

## Global Constraints

- Branch: `rebuild` (unmerged). Do not merge to main.
- `make ci` must stay green (lint + templ + build + tests; coverage gate ~80%). Run it, not just `go test`.
- No emoji in the app body; glyphs only from `ui/glyphs`. Hints use the ` → ` connector and `  ·  ` separator (never `=`).
- German UI strings, proper umlauts (ä/ö/ü/ß). Code/comments English.
- No raw hex in components; use `theme.Sem()` / builders.
- Key matching goes through `grammar.Binding.Matches` — no new raw `k.Code`/`k.Text` checks for the structural keys (move/back/top/bottom/page/search/help).
- `j`, `k`, `g`, `G` must not appear in any advertised hint or as list-navigation after this plan.

---

### Task 1: `ui/grammar` registry (single source of truth)

**Files:**
- Create: `internal/tui/ui/grammar/grammar.go`
- Test: `internal/tui/ui/grammar/grammar_test.go`

**Interfaces:**
- Consumes: `tea.KeyPressMsg`, `keyhint.Hint`.
- Produces:
  - `type Key struct{}` with constructors `Special(tea.KeyCode) Key`, `Rune(string) Key`, `Ctrl(tea.KeyCode) Key`; method `Matches(tea.KeyPressMsg) bool`.
  - `type Binding struct { ID, KeyLabel, Desc string; Keys []Key }`; methods `Matches(tea.KeyPressMsg) bool` and `Hint() keyhint.Hint`.
  - Package vars (canonical bindings): `MoveUp, MoveDown, Top, Bottom, PageUp, PageDown, Open, Back, Quit, Search, Help, NextTab Binding`.

- [ ] **Step 1: Write the failing test**

```go
package grammar

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKey_Matches(t *testing.T) {
	if !Special(tea.KeyDown).Matches(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatal("Special(KeyDown) must match a KeyDown press")
	}
	if Special(tea.KeyDown).Matches(tea.KeyPressMsg{Code: tea.KeyUp}) {
		t.Fatal("Special(KeyDown) must not match KeyUp")
	}
	if !Rune("q").Matches(tea.KeyPressMsg{Text: "q"}) {
		t.Fatal("Rune(q) must match a 'q' text press")
	}
	if Rune("q").Matches(tea.KeyPressMsg{Text: "q", Mod: tea.ModCtrl}) {
		t.Fatal("Rune(q) must not match Ctrl+q")
	}
	if !Ctrl('c').Matches(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}) {
		t.Fatal("Ctrl(c) must match Ctrl+C")
	}
}

func TestBinding_Matches_Back(t *testing.T) {
	if !Back.Matches(tea.KeyPressMsg{Text: "q"}) {
		t.Fatal("Back must match q")
	}
	if !Back.Matches(tea.KeyPressMsg{Code: tea.KeyEsc}) {
		t.Fatal("Back must match Esc")
	}
}

func TestBinding_Hint(t *testing.T) {
	h := MoveDown.Hint()
	if h.Key != "↑/↓" || h.Desc != "bewegen" {
		t.Fatalf("MoveDown.Hint() = %+v, want {↑/↓ bewegen}", h)
	}
}

func TestNoVimKeysAdvertised(t *testing.T) {
	all := []Binding{MoveUp, MoveDown, Top, Bottom, PageUp, PageDown, Open, Back, Quit, Search, Help, NextTab}
	for _, b := range all {
		for _, bad := range []string{"j", "k", "g", "G"} {
			if b.KeyLabel == bad {
				t.Fatalf("binding %s advertises vim key %q", b.ID, bad)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/grammar/ -run Test -v`
Expected: FAIL — package/identifiers undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package grammar is the single source of truth for flow's keyboard grammar.
// Each Binding pairs the keys that trigger an action with the canonical German
// hint advertised for it, so behaviour (Matches) and footer/help text (Hint)
// are defined once and cannot drift. The grammar is deliberately default-user
// oriented (arrows, Home/End, q/Esc back) — not vim (no j/k/g/G).
package grammar

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Key is one triggering key: a special key (arrows, Home…), a printable rune,
// or a Ctrl-modified key.
type Key struct {
	code tea.KeyCode
	text string
	mod  tea.KeyMod
}

// Special matches a non-printable key by its code (e.g. tea.KeyUp).
func Special(c tea.KeyCode) Key { return Key{code: c} }

// Rune matches a printable key by the text it produces (e.g. "q", "/").
func Rune(s string) Key { return Key{text: s} }

// Ctrl matches a Ctrl-modified key by code (e.g. Ctrl('c')).
func Ctrl(c tea.KeyCode) Key { return Key{code: c, mod: tea.ModCtrl} }

// Matches reports whether k triggers this key.
func (key Key) Matches(k tea.KeyPressMsg) bool {
	if key.text != "" {
		return k.Mod == 0 && k.Text == key.text
	}
	return k.Code == key.code && k.Mod == key.mod
}

// Binding is one grammar entry. KeyLabel/Desc are the advertised hint halves.
type Binding struct {
	ID       string
	Keys     []Key
	KeyLabel string
	Desc     string
}

// Matches reports whether k triggers any of this binding's keys.
func (b Binding) Matches(k tea.KeyPressMsg) bool {
	for _, key := range b.Keys {
		if key.Matches(k) {
			return true
		}
	}
	return false
}

// Hint renders this binding as a footer key-hint.
func (b Binding) Hint() keyhint.Hint { return keyhint.Hint{Key: b.KeyLabel, Desc: b.Desc} }

// Canonical structural bindings — the contract, defined once.
var (
	MoveUp   = Binding{ID: "move.up", Keys: []Key{Special(tea.KeyUp)}, KeyLabel: "↑/↓", Desc: "bewegen"}
	MoveDown = Binding{ID: "move.down", Keys: []Key{Special(tea.KeyDown)}, KeyLabel: "↑/↓", Desc: "bewegen"}
	Top      = Binding{ID: "jump.top", Keys: []Key{Special(tea.KeyHome)}, KeyLabel: "pos1/ende", Desc: "sprung"}
	Bottom   = Binding{ID: "jump.bottom", Keys: []Key{Special(tea.KeyEnd)}, KeyLabel: "pos1/ende", Desc: "sprung"}
	PageUp   = Binding{ID: "page.up", Keys: []Key{Special(tea.KeyPgUp)}, KeyLabel: "bild↑/↓", Desc: "blättern"}
	PageDown = Binding{ID: "page.down", Keys: []Key{Special(tea.KeyPgDown)}, KeyLabel: "bild↑/↓", Desc: "blättern"}
	Open     = Binding{ID: "open", Keys: []Key{Special(tea.KeyEnter)}, KeyLabel: "enter", Desc: "öffnen"}
	Back     = Binding{ID: "back", Keys: []Key{Rune("q"), Special(tea.KeyEsc)}, KeyLabel: "q", Desc: "zurück"}
	Quit     = Binding{ID: "quit", Keys: []Key{Rune("q"), Special(tea.KeyEsc)}, KeyLabel: "q", Desc: "beenden"}
	Search   = Binding{ID: "search", Keys: []Key{Rune("/")}, KeyLabel: "/", Desc: "suchen"}
	Help     = Binding{ID: "help", Keys: []Key{Rune("?")}, KeyLabel: "?", Desc: "Hilfe"}
	NextTab  = Binding{ID: "tab.next", Keys: []Key{Special(tea.KeyTab)}, KeyLabel: "tab", Desc: "wechseln"}
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/grammar/ -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/grammar/
git commit -m "feat(tui): ui/grammar binding registry (single source for keys + hints)"
```

---

### Task 2: `ui/listnav` shared cursor

**Files:**
- Create: `internal/tui/ui/listnav/listnav.go`
- Test: `internal/tui/ui/listnav/listnav_test.go`

**Interfaces:**
- Consumes: `grammar.{MoveUp,MoveDown,Top,Bottom,PageUp,PageDown}`, `tea.KeyPressMsg`.
- Produces:
  - `type Cursor struct{}`; `func New() Cursor`; methods `Index() int`, `Clamp(count int) Cursor`, `Set(i, count int) Cursor`, `Handle(k tea.KeyPressMsg, count, pageSize int) (Cursor, bool)`.

- [ ] **Step 1: Write the failing test**

```go
package listnav

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(c tea.KeyCode) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }

func TestHandle_Movement(t *testing.T) {
	c := New() // idx 0
	c, ok := c.Handle(press(tea.KeyDown), 3, 2)
	if !ok || c.Index() != 1 {
		t.Fatalf("down: idx=%d ok=%v, want 1 true", c.Index(), ok)
	}
	c, _ = c.Handle(press(tea.KeyDown), 3, 2) // 2
	c, _ = c.Handle(press(tea.KeyDown), 3, 2) // clamp at 2
	if c.Index() != 2 {
		t.Fatalf("clamp bottom: idx=%d, want 2", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyUp), 3, 2)
	c, _ = c.Handle(press(tea.KeyUp), 3, 2)
	c, _ = c.Handle(press(tea.KeyUp), 3, 2) // clamp at 0
	if c.Index() != 0 {
		t.Fatalf("clamp top: idx=%d, want 0", c.Index())
	}
}

func TestHandle_HomeEndPage(t *testing.T) {
	c := New()
	c, _ = c.Handle(press(tea.KeyEnd), 10, 4)
	if c.Index() != 9 {
		t.Fatalf("End: idx=%d, want 9", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyHome), 10, 4)
	if c.Index() != 0 {
		t.Fatalf("Home: idx=%d, want 0", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyPgDown), 10, 4)
	if c.Index() != 4 {
		t.Fatalf("PgDown: idx=%d, want 4", c.Index())
	}
	c, _ = c.Handle(press(tea.KeyPgUp), 10, 4)
	if c.Index() != 0 {
		t.Fatalf("PgUp clamps: idx=%d, want 0", c.Index())
	}
}

func TestHandle_NotANavKey(t *testing.T) {
	c := New()
	_, ok := c.Handle(tea.KeyPressMsg{Text: "n"}, 3, 2)
	if ok {
		t.Fatal("'n' is not a nav key; ok must be false")
	}
}

func TestHandle_EmptyList(t *testing.T) {
	c := New()
	c, _ = c.Handle(press(tea.KeyDown), 0, 2)
	if c.Index() != 0 {
		t.Fatalf("empty: idx=%d, want 0", c.Index())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/listnav/ -v`
Expected: FAIL — undefined `New`/`Cursor`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package listnav is the shared list-cursor primitive. It owns a clamped index
// and maps keyboard keys to movement using the canonical grammar bindings
// (arrows, Home/End, PageUp/PageDown). Clamp, never wrap; no j/k. It is a pure
// value — screens embed a Cursor and call Handle in their Update.
package listnav

import (
	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/grammar"
)

// Cursor is a clamped selection index.
type Cursor struct{ idx int }

// New returns a Cursor at index 0.
func New() Cursor { return Cursor{} }

// Index returns the current index.
func (c Cursor) Index() int { return c.idx }

// Clamp returns c with its index bounded to [0, count-1] (0 when count<=0).
func (c Cursor) Clamp(count int) Cursor {
	if count <= 0 {
		c.idx = 0
		return c
	}
	if c.idx < 0 {
		c.idx = 0
	}
	if c.idx > count-1 {
		c.idx = count - 1
	}
	return c
}

// Set returns c moved to i, clamped to count.
func (c Cursor) Set(i, count int) Cursor { c.idx = i; return c.Clamp(count) }

// Handle maps k to a clamped movement against count. ok is false when k is not a
// navigation key (the caller keeps handling it). pageSize is the PageUp/Down step.
func (c Cursor) Handle(k tea.KeyPressMsg, count, pageSize int) (Cursor, bool) {
	switch {
	case grammar.MoveDown.Matches(k):
		return c.Set(c.idx+1, count), true
	case grammar.MoveUp.Matches(k):
		return c.Set(c.idx-1, count), true
	case grammar.Top.Matches(k):
		return c.Set(0, count), true
	case grammar.Bottom.Matches(k):
		return c.Set(count-1, count), true
	case grammar.PageDown.Matches(k):
		return c.Set(c.idx+pageSize, count), true
	case grammar.PageUp.Matches(k):
		return c.Set(c.idx-pageSize, count), true
	}
	return c, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ui/listnav/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/listnav/
git commit -m "feat(tui): ui/listnav shared clamped cursor over grammar bindings"
```

---

### Task 3: Migrate docs list + search cursor to listnav (drop j/k)

**Files:**
- Modify: `internal/tui/docs.go` (modeList nav at `:623-632`; search-results nav at `:900-905`)
- Test: `internal/tui/docs_test.go`

**Interfaces:**
- Consumes: `listnav.Cursor` (Task 2). docs keeps its own `m.sel int` and `m.searchSel int` fields; use a local `listnav.New().Set(m.sel, count)` per keypress (no struct change needed — the Cursor is a stateless mapper here).

- [ ] **Step 1: Write the failing test**

```go
func TestDocs_ListArrowsMoveNotJK(t *testing.T) {
	m := NewDocs(nil, nil, nil, theme.Default, "u")
	m.docs = []domain.Document{
		{ID: "a", Type: domain.DocFree, Path: "a", Title: "A"},
		{ID: "b", Type: domain.DocFree, Path: "b", Title: "B"},
	}
	// Arrow down moves the selection.
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m2.(DocsModel).sel != 1 {
		t.Fatalf("KeyDown: sel=%d, want 1", m2.(DocsModel).sel)
	}
	// 'j' no longer navigates (it is not a list-nav key).
	m3, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	if m3.(DocsModel).sel != 0 {
		t.Fatalf("'j' must not move the cursor: sel=%d, want 0", m3.(DocsModel).sel)
	}
	// End jumps to the last row.
	m4, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if m4.(DocsModel).sel != 1 {
		t.Fatalf("End: sel=%d, want 1", m4.(DocsModel).sel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestDocs_ListArrowsMoveNotJK -v`
Expected: FAIL — KeyDown does nothing today (docs has only j/k), sel stays 0.

- [ ] **Step 3: Replace the modeList j/k handler**

In `internal/tui/docs.go`, delete the two `j`/`k` cases at `:623-632`:

```go
	case k.Text == "j":
		if m.sel < len(m.visibleDocs())-1 {
			m.sel++
		}
		return m, nil
	case k.Text == "k":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
```

and add an early grammar-nav `if` at the **top** of the `// modeList` block (just before `switch {` at `:619-620`), so navigation is handled before the verb cases:

```go
	// modeList
	if cur, ok := listnav.New().Set(m.sel, len(m.visibleDocs())).Handle(k, len(m.visibleDocs()), m.docsPerPage()); ok {
		m.sel = cur.Index()
		return m, nil
	}
	switch {
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return m, tea.Quit
	// ... existing verb cases (Enter, n, e, d, f, /, p) unchanged ...
	}
```

Add the import `"github.com/serverkraken/flow/internal/tui/ui/listnav"` to `docs.go`. (`docsPerPage()` already exists — see `:1321`.)

- [ ] **Step 4: Do the same for search results**

In `handleSearchKey` (`:900-905`), replace:

```go
	case m.searching && k.Text == "j":
		...
	case m.searching && k.Text == "k":
		...
```

with an early nav `if` mirroring the list (using `m.searchSel` and `len(m.searchHits)`):

```go
	if !m.searching { // results focused, not typing
		if cur, ok := listnav.New().Set(m.searchSel, len(m.searchHits)).Handle(k, len(m.searchHits), m.docsPerPage()); ok {
			m.searchSel = cur.Index()
			return m, nil
		}
	}
```

Keep the existing typing/Enter/Esc handling for the search input. (Verify the exact `m.searching` gating against the current code; the nav block must only run when results — not the query input — are focused.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestDocs -v`
Expected: PASS, including the new test and existing docs tests.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/docs.go internal/tui/docs_test.go
git commit -m "feat(docs): list+search cursor via listnav (arrows/Home/End/Page, no j/k)"
```

---

### Task 4: Migrate worktime Today cursor to listnav (clamp, drop wrap + j/k + g/G)

**Files:**
- Modify: `internal/tui/screen/worktime/route.go` (key switch `:179-196`; `applyKey` `:204-211`)
- Test: `internal/tui/screen/worktime/route_test.go`

**Interfaces:**
- Consumes: `listnav.Cursor`. Replace `r.cursor int` usage; `r.cursor` stays an `int` field, moved via `listnav`.

- [ ] **Step 1: Write the failing test**

```go
func TestToday_ArrowsClampNoWrap(t *testing.T) {
	r := newTestTodayRoute(t, 3) // helper builds a route with 3 completed sessions, cursor 0
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if r2.(*TodayRoute).cursor != 0 {
		t.Fatalf("Up at top must clamp (no wrap): cursor=%d, want 0", r2.(*TodayRoute).cursor)
	}
	// 'j' no longer navigates.
	r3, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r3.(*TodayRoute).cursor != 0 {
		t.Fatalf("'j' must not move: cursor=%d, want 0", r3.(*TodayRoute).cursor)
	}
}
```

(If a `newTestTodayRoute` helper does not exist, build the route inline the way the existing `route_test.go` tests do — copy their construction, do not invent a helper signature.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/ -run TestToday_ArrowsClampNoWrap -v`
Expected: FAIL — Up currently wraps to the last item.

- [ ] **Step 3: Replace the nav cases and applyKey**

Replace the four cases at `:179-189`:

```go
	case k.Text == "j" || k.Code == tea.KeyDown:
		r.applyKey("j")
	case k.Text == "k" || k.Code == tea.KeyUp:
		r.applyKey("k")
	case k.Text == "g":
		r.cursor = 0
	case k.Text == "G":
		r.cursor = max(0, len(r.st.Completed)-1)
```

with a single grammar-driven nav branch at the top of the switch (before the dialog/verb handling already in `handleKey`):

```go
	if cur, ok := listnav.New().Set(r.cursor, len(r.st.Completed)).Handle(k, len(r.st.Completed), 5); ok {
		r.cursor = cur.Index()
		return r, nil
	}
```

Delete the now-unused `applyKey` method (`:204-211`) and its `% n` wrap. Add the `listnav` import.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/screen/worktime/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/route.go internal/tui/screen/worktime/route_test.go
git commit -m "feat(worktime): Today cursor via listnav (clamp, drop wrap/j/k/g/G)"
```

---

### Task 5: Migrate worktime dayoffs cursor to listnav

**Files:**
- Modify: `internal/tui/screen/worktime/dayoffs/route.go` (cursor wrap at `:130-135`)
- Test: `internal/tui/screen/worktime/dayoffs/route_test.go`

**Interfaces:**
- Consumes: `listnav.Cursor`. `r.cursor int` and `r.list` slice already exist.

- [ ] **Step 1: Write the failing test**

```go
func TestDayoffs_ArrowsClampNoWrap(t *testing.T) {
	r := buildDayoffsRouteWithItems(t, 2) // mirror existing dayoffs_test construction; cursor 0
	r2, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if r2.(*Route).cursor != 0 {
		t.Fatalf("Up at top must clamp: cursor=%d, want 0", r2.(*Route).cursor)
	}
	r3, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r3.(*Route).cursor != 0 {
		t.Fatalf("'j' must not move: cursor=%d, want 0", r3.(*Route).cursor)
	}
}
```

(Use the same route construction the existing `dayoffs/route_test.go` uses; do not invent helper names.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -run TestDayoffs_ArrowsClampNoWrap -v`
Expected: FAIL — Up wraps today.

- [ ] **Step 3: Replace the j/k wrap cases**

Replace the `case "j"` / `case "k"` wrap block at `:130-135` (and any `k.Code == tea.KeyDown/Up` aliases in the key switch) with a top-of-switch grammar nav branch:

```go
	if cur, ok := listnav.New().Set(r.cursor, len(r.list)).Handle(k, len(r.list), 5); ok {
		r.cursor = cur.Index()
		return r, nil
	}
```

Add the `listnav` import; remove the now-dead wrap helper if one exists.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/screen/worktime/dayoffs/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/worktime/dayoffs/
git commit -m "feat(worktime): dayoffs cursor via listnav (clamp, drop wrap/j/k)"
```

---

### Task 6: Align fuzzylist to grammar keys (Home/End/Page)

**Files:**
- Modify: `internal/tui/ui/fuzzylist/fuzzylist.go` (Update `:85-96`)
- Test: `internal/tui/ui/fuzzylist/fuzzylist_test.go`

**Interfaces:**
- Consumes: `grammar.{Top,Bottom,PageUp,PageDown}`. fuzzylist keeps its own `cursor int` and `rowCount()` (the create-row makes its count special, so it does **not** adopt `listnav.Cursor`), but matches the extra keys via grammar so the grammar stays the single source. Keeps `Ctrl+n/Ctrl+p` (live-filter context).

- [ ] **Step 1: Write the failing test**

```go
func TestFuzzyList_HomeEnd(t *testing.T) {
	m := fuzzylist.New(items(), theme.Default) // 3 items (see existing test helper)
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if it, _, ok := m.Selection(); !ok || it.ID != "oraya" {
		t.Fatalf("End must select last item, got ok=%v id=%q", ok, it.ID)
	}
	m = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if it, _, ok := m.Selection(); !ok || it.ID != items()[0].ID {
		t.Fatalf("Home must select first item, got id=%q", it.ID)
	}
}
```

(Reuse the existing `items()` helper and `Selection()` shape from `fuzzylist_test.go`; check the real last-item ID and adjust `"oraya"` to match.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/fuzzylist/ -run TestFuzzyList_HomeEnd -v`
Expected: FAIL — Home/End unhandled today.

- [ ] **Step 3: Extend the Update switch**

In `fuzzylist.go` Update, add cases alongside the existing `KeyUp`/`KeyDown` handling (`:89-96`):

```go
	case grammar.Top.Matches(k):
		m.cursor = 0
	case grammar.Bottom.Matches(k):
		m.cursor = m.rowCount() - 1
	case grammar.PageDown.Matches(k):
		m.cursor += 5
	case grammar.PageUp.Matches(k):
		m.cursor -= 5
```

Then keep the existing clamp block (`:140-145`) which already bounds `m.cursor` to `[0, rowCount()-1]`. Add the `grammar` import.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ui/fuzzylist/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ui/fuzzylist/
git commit -m "feat(tui): fuzzylist honours grammar Home/End/Page keys"
```

---

### Task 7: `Backer` interface + `ResolveBack` pure function

**Files:**
- Modify: `internal/tui/shell/route.go` (add `Backer` + `TextCapturer` interfaces)
- Create: `internal/tui/shell/back.go`
- Test: `internal/tui/shell/back_test.go`

**Interfaces:**
- Produces:
  - In `route.go`: `type Backer interface { Back() (Route, tea.Cmd, bool) }` and `type TextCapturer interface { CapturesText() bool }`.
  - In `back.go`: `type BackAction int` with `BackForward, BackOverlay, BackRoute, BackPop, BackQuit`; `func ResolveBack(top Route, stackDepth int, overlayOpen bool) BackAction`.

- [ ] **Step 1: Write the failing test**

```go
package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// stubRoute is a minimal Route for back tests; capabilities toggled per case.
type stubRoute struct {
	text    bool
	backOK  bool
}

func (s stubRoute) Title() string                          { return "stub" }
func (s stubRoute) Init() tea.Cmd                           { return nil }
func (s stubRoute) Update(tea.Msg) (Route, tea.Cmd)         { return s, nil }
func (s stubRoute) View(Frame) string                      { return "" }
func (s stubRoute) KeyHints() []keyhint.Hint                { return nil }
func (s stubRoute) CapturesText() bool                      { return s.text }
func (s stubRoute) Back() (Route, tea.Cmd, bool)            { return s, nil, s.backOK }

func TestResolveBack(t *testing.T) {
	cases := []struct {
		name    string
		top     Route
		depth   int
		overlay bool
		want    BackAction
	}{
		{"overlay closes first", stubRoute{}, 2, true, BackOverlay},
		{"text field forwards", stubRoute{text: true}, 2, false, BackForward},
		{"route internal back", stubRoute{backOK: true}, 2, false, BackRoute},
		{"pop when deep", stubRoute{}, 2, false, BackPop},
		{"quit at root", stubRoute{}, 1, false, BackQuit},
	}
	for _, tc := range cases {
		if got := ResolveBack(tc.top, tc.depth, tc.overlay); got != tc.want {
			t.Errorf("%s: ResolveBack = %v, want %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run TestResolveBack -v`
Expected: FAIL — `ResolveBack`/`BackAction`/`CapturesText`/`Back` undefined.

- [ ] **Step 3: Add interfaces + ResolveBack**

Append to `internal/tui/shell/route.go`:

```go
// TextCapturer lets a route signal it is in a *literal text-entry* field, where
// even the back keys (q/Esc) belong to the route (q is typed, the route's own
// Esc cancels the field). Narrower than InputCapturer, which also covers
// non-text key forwarding (e.g. the doc viewer's Tab/scroll).
type TextCapturer interface{ CapturesText() bool }

// Backer lets a route resolve one level of "back" within its own internal state
// (e.g. document view → list, clear an active filter) before the frame pops the
// nav-stack or quits. ok=false means "nothing internal — frame decides".
type Backer interface{ Back() (Route, tea.Cmd, bool) }
```

Create `internal/tui/shell/back.go`:

```go
package shell

// BackAction is what a back key (q/Esc) should do, decided by ResolveBack.
type BackAction int

const (
	BackForward BackAction = iota // route owns the key (literal text entry)
	BackOverlay                   // close the open help/palette overlay
	BackRoute                     // route handles it internally (Backer)
	BackPop                       // pop the nav-stack one level
	BackQuit                      // quit the program
)

// ResolveBack decides how a back key resolves for the active route, given the
// nav-stack depth and whether an overlay is open. Shell and RouteHost both call
// this; the host passes stackDepth=1 so it can only ever reach BackQuit.
func ResolveBack(top Route, stackDepth int, overlayOpen bool) BackAction {
	if overlayOpen {
		return BackOverlay
	}
	if tc, ok := top.(TextCapturer); ok && tc.CapturesText() {
		return BackForward
	}
	if b, ok := top.(Backer); ok {
		if _, _, handled := b.Back(); handled {
			return BackRoute
		}
	}
	if stackDepth > 1 {
		return BackPop
	}
	return BackQuit
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/shell/ -run TestResolveBack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/shell/route.go internal/tui/shell/back.go internal/tui/shell/back_test.go
git commit -m "feat(shell): Backer/TextCapturer interfaces + ResolveBack pure function"
```

---

### Task 8: docs implements Backer + CapturesText; wire ResolveBack into Shell

**Files:**
- Modify: `internal/tui/docs.go` (add `Back()` + `CapturesText()` on `DocsModel`; remove modeView Esc handler at `:592-601`)
- Modify: `internal/tui/screen/docs/route.go` (expose `Back()` + `CapturesText()` on the adapter)
- Modify: `internal/tui/shell/shell.go` (fold `q` + `Esc` into `ResolveBack`, `:204-212`)
- Test: `internal/tui/docs_test.go`, `internal/tui/shell/shell_test.go`

**Interfaces:**
- Consumes: `ResolveBack`, `BackAction` (Task 7).
- Produces: `func (m DocsModel) Back() (DocsModel, tea.Cmd, bool)`, `func (m DocsModel) CapturesText() bool`; adapter `func (r *Route) Back() (shell.Route, tea.Cmd, bool)`, `func (r *Route) CapturesText() bool`.

- [ ] **Step 1: Write the failing tests**

```go
// docs_test.go
func TestDocs_CapturesTextNarrow(t *testing.T) {
	m := NewDocs(nil, nil, nil, theme.Default, "u")
	m.viewing = &domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "A", Body: "x"}
	m.mode = modeView
	if m.CapturesText() {
		t.Fatal("modeView (no in-doc search) must NOT capture text — q/Esc go to the back chain")
	}
	m.mode = modeSearch
	if !m.CapturesText() {
		t.Fatal("modeSearch must capture text")
	}
}

func TestDocs_BackFromView(t *testing.T) {
	m := NewDocs(nil, nil, nil, theme.Default, "u")
	m.viewing = &domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "A", Body: "x"}
	m.mode = modeView
	nm, _, ok := m.Back()
	if !ok || nm.mode != modeList {
		t.Fatalf("Back from modeView → list: ok=%v mode=%v", ok, nm.mode)
	}
}

func TestDocs_BackFromListNothing(t *testing.T) {
	m := NewDocs(nil, nil, nil, theme.Default, "u") // no filter active
	if _, _, ok := m.Back(); ok {
		t.Fatal("Back from a clean list must report ok=false (frame pops/quits)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestDocs_CapturesText|TestDocs_Back' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Add `CapturesText` and `Back` to DocsModel**

In `docs.go`, add (keep the existing `CapturesInput()` unchanged — modeView still needs broad forwarding):

```go
// CapturesText reports a literal text-entry state, where back keys (q/Esc) are
// the route's own input. Narrower than CapturesInput: modeView forwards its
// non-back keys (Tab/scroll) but its q/Esc go to the frame back chain — unless
// the in-document search field is active.
func (m DocsModel) CapturesText() bool {
	switch m.mode {
	case modeCreating, modeFiltering, modeSearch, modeProjectFilter:
		return true
	case modeView:
		return m.overlayReady && m.overlay.CapturesInput()
	}
	return false
}

// Back resolves one internal level: pop the in-view wikilink stack, leave the
// viewer for the list, cancel a delete, or clear an applied filter. ok=false
// means the list is clean and the frame should pop/quit.
func (m DocsModel) Back() (DocsModel, tea.Cmd, bool) {
	switch m.mode {
	case modeView:
		if n := len(m.viewStack); n > 0 {
			prev := m.viewStack[n-1]
			m.viewStack = m.viewStack[:n-1]
			return m, m.loadDocNoPush(prev), true
		}
		m.viewing = nil
		m.mode = modeList
		m.viewLinks = nil
		m.linkFocus = -1
		m.overlayReady = false
		m.viewer = nil
		return m, nil, true
	case modeDeleting:
		m.mode = modeList
		return m, nil, true
	case modeList:
		if len(m.filterTags) > 0 {
			m.filterTags = nil
			m.sel = 0
			return m, nil, true
		}
	}
	return m, nil, false
}
```

Remove the now-superseded `modeView` `case k.Code == tea.KeyEsc:` block at `:592-601` (the frame drives that path now via `Back()`). Leave the in-overlay-search early-return (`:578-583`) intact — it runs while `CapturesText()` is true.

- [ ] **Step 4: Expose on the adapter**

In `internal/tui/screen/docs/route.go`, add:

```go
// CapturesText implements shell.TextCapturer.
func (r *Route) CapturesText() bool { return r.m.CapturesText() }

// Back implements shell.Backer, delegating to the wrapped DocsModel.
func (r *Route) Back() (shell.Route, tea.Cmd, bool) {
	nm, cmd, ok := r.m.Back()
	r.m = nm
	return r, cmd, ok
}
```

- [ ] **Step 5: Fold q + Esc into ResolveBack in shell.go**

Replace the `Esc` and `q` cases at `:204-212`:

```go
	case k.Code == tea.KeyEsc:
		if s.helpOpen {
			s.helpOpen = false
			return s, nil
		}
		s.tabs[s.activeTab].Pop()
		return s, nil
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return s, tea.Quit
```

with a single back-key branch (Ctrl+C stays an unconditional hard quit) placed before the other cases:

```go
	case k.Code == 'c' && k.Mod == tea.ModCtrl:
		return s, tea.Quit
	case grammar.Back.Matches(k):
		top := s.tabs[s.activeTab].Top()
		switch ResolveBack(top, s.tabs[s.activeTab].Len(), s.helpOpen || s.paletteOpen) {
		case BackOverlay:
			s.helpOpen = false
			s.paletteOpen = false
			return s, nil
		case BackForward:
			return s, s.tabs[s.activeTab].UpdateTop(k)
		case BackRoute:
			if b, ok := top.(Backer); ok {
				nr, cmd, _ := b.Back()
				s.tabs[s.activeTab].ReplaceTop(nr)
				return s, cmd
			}
			return s, nil
		case BackPop:
			s.tabs[s.activeTab].Pop()
			return s, nil
		case BackQuit:
			return s, tea.Quit
		}
		return s, nil
```

Note the input-capture early-return at `:200` still forwards non-back keys for `modeView`. Because `grammar.Back.Matches` is checked in the switch *after* that early-return, a text-capturing route never reaches here (its keys are already forwarded at `:200` when `CapturesInput()` is true). To keep back-key routing correct for `modeView` (which has `CapturesInput()==true` but `CapturesText()==false`), change the `:200` guard to **not** swallow back keys:

```go
	if ic, ok := s.tabs[s.activeTab].Top().(InputCapturer); ok && ic.CapturesInput() && !s.helpOpen && !grammar.Back.Matches(k) {
		return s, s.tabs[s.activeTab].UpdateTop(k)
	}
```

Add the `grammar` import to `shell.go`.

- [ ] **Step 6: Add a shell back test**

```go
// shell_test.go — q at depth 1 quits, Esc on a Backer route goes back internally.
func TestShell_BackKeyResolves(t *testing.T) {
	// Build a Shell with a single tab whose root is a stub Backer that reports
	// it went back; assert ReplaceTop happened and no Quit. Mirror existing
	// shell_test construction.
}
```

(Fill the body using the existing `shell_test.go` Shell construction helpers; assert the resolved action via observable state — top route swapped, or `tea.Quit` cmd present at root.)

- [ ] **Step 7: Run tests**

Run: `go test ./internal/tui/... -run 'Docs|Shell' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/docs.go internal/tui/screen/docs/route.go internal/tui/shell/shell.go internal/tui/docs_test.go internal/tui/shell/shell_test.go
git commit -m "feat(shell): q/Esc back-chain via ResolveBack; docs Backer+CapturesText"
```

---

### Task 9: Mirror ResolveBack into the standalone host

**Files:**
- Modify: `internal/tui/shell/host.go` (`Update` key branch `:32-35`)
- Test: `internal/tui/shell/host_test.go` (create if absent)

**Interfaces:**
- Consumes: `ResolveBack`, `grammar.Back`, `Backer`, `tea.Quit`.

- [ ] **Step 1: Write the failing test**

```go
func TestRouteHost_BackForwardsTextThenQuits(t *testing.T) {
	// A text-capturing route keeps 'q' (literal); a clean route quits on 'q'.
	h := NewRouteHost(stubRoute{text: true}, theme.Default)
	if _, cmd := h.Update(tea.KeyPressMsg{Text: "q"}); isQuit(cmd) {
		t.Fatal("q in a text field must NOT quit standalone host")
	}
	h2 := NewRouteHost(stubRoute{}, theme.Default)
	if _, cmd := h2.Update(tea.KeyPressMsg{Text: "q"}); !isQuit(cmd) {
		t.Fatal("q on a clean leaf must quit standalone host")
	}
}
```

(Add an `isQuit(tea.Cmd) bool` test helper that runs the cmd and checks for `tea.QuitMsg`, or reuse one if present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shell/ -run TestRouteHost_Back -v`
Expected: FAIL — host quits on q unconditionally today.

- [ ] **Step 3: Replace the host key branch**

Replace `:32-35`:

```go
	case tea.KeyPressMsg:
		if msg.Text == "q" || msg.Code == tea.KeyEsc || (msg.Code == 'c' && msg.Mod == tea.ModCtrl) {
			return h, tea.Quit
		}
```

with the ResolveBack-driven branch (stackDepth=1 — a host has no stack, so it can only forward, route-back, or quit):

```go
	case tea.KeyPressMsg:
		if msg.Code == 'c' && msg.Mod == tea.ModCtrl {
			return h, tea.Quit
		}
		if grammar.Back.Matches(msg) {
			switch ResolveBack(h.route, 1, false) {
			case BackForward:
				var cmd tea.Cmd
				h.route, cmd = h.route.Update(msg)
				return h, cmd
			case BackRoute:
				if b, ok := h.route.(Backer); ok {
					nr, cmd, _ := b.Back()
					h.route = nr
					return h, cmd
				}
				return h, nil
			default: // BackPop unreachable at depth 1, BackQuit
				return h, tea.Quit
			}
		}
```

Add the `grammar` import to `host.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/shell/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/shell/host.go internal/tui/shell/host_test.go
git commit -m "feat(shell): standalone host uses the same ResolveBack back-chain"
```

---

### Task 10: Hints & help render from grammar (drop j/k/g/G everywhere)

**Files:**
- Modify: `internal/tui/screen/docs/route.go` (`KeyHints` `:69-79`)
- Modify: `internal/tui/screen/worktime/route.go` (KeyHints — find via `rg -n 'KeyHints' internal/tui/screen/worktime/route.go`)
- Modify: `internal/tui/ui/strings/strings.go` (`HintNav` `:22`, `HintQuit` `:21`, `HintBack` `:30`)
- Modify: `internal/tui/shell/shell.go` `renderHelp()` (`:319`) — update the global nav section
- Test: a grammar-sweep test in `internal/tui/ui/strings/strings_test.go` (create if absent) + update existing hint/golden assertions

**Interfaces:**
- Consumes: `grammar.{MoveUp,Open,Back,Quit,Search,Help}.Hint()`.

- [ ] **Step 1: Write the failing test**

```go
// strings_test.go — no advertised hint constant mentions a vim nav key.
func TestHints_NoVimKeys(t *testing.T) {
	for name, s := range map[string]string{
		"HintNav": HintNav, "HintQuit": HintQuit, "HintBack": HintBack,
	} {
		for _, bad := range []string{"j/k", " j ", " k ", "g/G"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s = %q still advertises vim key %q", name, s, bad)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ui/strings/ -run TestHints_NoVimKeys -v`
Expected: FAIL — `HintNav = "j/k → navigieren  ·  …"`.

- [ ] **Step 3: Update the strings**

In `strings.go`:

```go
	HintQuit = "q → schließen"            // keep wording; q now also = back at depth>1
	HintNav  = "↑/↓ → bewegen  ·  Enter → öffnen"
	HintBack = "q → zurück"
```

- [ ] **Step 4: Update docs + worktime KeyHints to grammar**

In `screen/docs/route.go` `KeyHints`, replace the `{Key: "j/k", Desc: "wählen"}` entry with `grammar.MoveUp.Hint()` and the `enter`/`/` entries with `grammar.Open.Hint()` / `grammar.Search.Hint()`; keep the verb entries (`n`/`e`/`p`/`f`). Add `grammar.Back.Hint()` as the back hint. Add the `grammar` import.

In `screen/worktime/route.go` `KeyHints`, replace any `j/k` / `g/G` hint with `grammar.MoveUp.Hint()`; keep verb keys (`s`, `w/t/d/e`, `E`, `D`).

- [ ] **Step 5: Update the shell help overlay**

In `shell.go` `renderHelp()`, change the global "Navigation" section keys from `j/k`-based rows to grammar-based: `{"↑/↓", "bewegen"}`, `{"q / Esc", "zurück / beenden"}`, `{"Tab / 1–9", "Tab wechseln"}`, `{"/", "suchen"}`, `{"?", "Hilfe"}`. (Source these labels from the corresponding `grammar.*.KeyLabel/Desc` where they map cleanly.)

- [ ] **Step 6: Run the full suite + fix golden/hint assertions**

Run: `go test ./internal/tui/... -v`
Expected: PASS. Update any golden snapshot or hint-count test that asserted the old `j/k` footer/help wording (search them: `rg -n 'j/k' internal/tui`).

- [ ] **Step 7: Commit**

```bash
git add internal/tui/ui/strings/ internal/tui/screen/docs/route.go internal/tui/screen/worktime/route.go internal/tui/shell/shell.go
git commit -m "feat(tui): footer + help render the default-user grammar (no j/k/g/G)"
```

---

### Task 11: Wiring verification + manual key-walk

**Files:**
- None (verification task). Optionally update `docs/design-system.md` note that flow uses a default-user grammar (one line), pointing to the spec.

- [ ] **Step 1: Confirm no vim nav keys remain**

Run: `rg -n '"j"|"k"|"g"|"G"' internal/tui --glob '!*_test.go' | rg -i 'cursor|sel|nav|KeyDown|KeyUp|move'`
Expected: no list-navigation matches (verb keys like `D`, and unrelated `g` in identifiers, are fine — inspect each hit).

- [ ] **Step 2: `make ci` green**

Run: `make ci`
Expected: lint + templ + build + tests pass; coverage ≥ gate.

- [ ] **Step 3: Manual key-walk against the dev stack**

Start the dev stack and `flow ui` (see `reference_flow_dev_env`: `make dev-up`, `make dev-run`). Verify on every surface:
- `↑/↓` moves the selection; `Home/End` jump; `PageUp/PageDown` page; `j/k/g/G` do **nothing**.
- In docs: open a document (`Enter`) → `q` returns to the list; follow a wikilink, then `q` walks the wikilink stack back, then to the list, then (at the list root) `q` pops the tab / quits at the main screen.
- With a tag filter applied in docs: `q` clears the filter first, then leaves.
- In a text field (new document title, `/` search input, project picker): `q` types a literal `q`; `Esc` cancels the field.
- Standalone `flow docs`: the same back-walk and text-field behaviour; `q` only quits from the clean list.
- Footer + `?` overlay show the new grammar; no `j/k`/`g/G` anywhere.

- [ ] **Step 4: Commit any doc note**

```bash
git add -A
git commit -m "docs: note flow's default-user keyboard grammar; wiring verified"
```

---

## Self-Review notes

- **Spec coverage:** BB0→Task 1; BB1→Task 2; cursor migration→Tasks 3–6; BB2 (Backer+ResolveBack)→Tasks 7–9; BB3 (hints/help)→Task 10; wiring-verification→Task 11. The spec's "narrow CapturesInput" is refined to "keep CapturesInput broad, add CapturesText narrow" (Task 8) because modeView needs non-back-key forwarding — documented in Task 8's rationale.
- **Type consistency:** `Cursor.Handle(k, count, pageSize) (Cursor, bool)`, `ResolveBack(top, stackDepth, overlayOpen) BackAction`, `Backer.Back() (Route, tea.Cmd, bool)`, `TextCapturer.CapturesText() bool`, `Binding.Matches`/`.Hint()` are used identically across tasks.
- **Out of scope (unchanged):** updating the `tui-usability` skill (separate follow-up), `←/→` bindings, verb-key changes, mouse support.
```
