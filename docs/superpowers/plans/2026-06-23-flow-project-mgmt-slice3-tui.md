# Project Management M4 — Slice 3 (TUI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The TUI route tasks give a working baseline (struct + key Update branches + View layout + exact data bindings) that MIRRORS the named existing screens — the bindings, interfaces, and apiclient calls are the contract; visual/idiomatic refinement within that structure is welcome but the data flow and key grammar are fixed. Invoke the **`tui-usability`** skill when building the route Views to keep color/spacing/keybind grammar consistent.

**Goal:** Add a TUI "Projekte" surface — a sidekick shell tab with a project list (fuzzy + status filter), a read-only detail cockpit (description, worktime Σ+€, docs, a **read-only live git-worktree panel**, status actions), and a create/edit form (whitelist color/glyph pickers + optional rate) — plus two new client packages `gitworktree` + `clientcheckout` and a checkout auto-record hook in `projectresolve`.

**Architecture:** Native bubbletea/v2 `shell.Route`s under `internal/tui/screen/projects/`, navigated by `shell.PushRouteMsg`/`PopRouteMsg` (list→detail→form), mounted as the 4th shell tab in `cmd/flow/ui.go`. Data comes from the existing `apiclient` (REST); worktrees are read live and client-side via `gitworktree.List(root)` where `root` comes from the per-machine `clientcheckout.json` registry. No new server state, no server-side git, no write-side git (no clone/worktree add/rm).

**Tech Stack:** Go, bubbletea/v2 (`charm.land/bubbletea/v2`), bubbles/v2, lipgloss/v2, the existing `internal/tui` design system (`theme`, `ui/listnav`, `ui/grammar`, `ui/form`, `markdown`, `kindcolor`), real `git` binary (shelled out) for worktree/checkout reads.

## Global Constraints

- Module path `github.com/serverkraken/flow`. Work on branch `rebuild`.
- Spec: `docs/superpowers/specs/2026-06-23-flow-project-management-design.md` (§59 Architecture, §155 Checkout & worktrees, §227 TUI, §322 Slicing). This slice = Slicing item 3.
- Slice-1 backend + Slice-2 WebUI are done. The domain single-source whitelists exist: `domain.ProjectColors []string` (`blue cyan green purple magenta yellow orange red teal`) and `domain.ProjectGlyphs []string` (`◆ ● ▶ ★ ☼ ✚ ▲ ■`) in `internal/domain/projectstyle.go`. `domain.Project` has `ID, Slug, Name, Description, UpstreamGit, Status (active|paused|archived), Color, Glyph string` and `Rate *domain.Money`. `domain.Money{Amount int64 /* cents */, Currency string}` with `Mul(time.Duration) Money` and `String() string`.
- **Read-only git, read-only worktree panel (decided):** the worktree panel is a live list only — no cursor on it, no actions, no `git worktree add/rm`, no clone. Fields per worktree: `Path`, `Branch` (or detached HEAD short), `HeadShort`, `Dirty`, `IsMain`. ahead/behind deferred (v2).
- **No server state for checkouts/worktrees:** `clientcheckout.json` (`slug→root`) is per-machine under `os.UserConfigDir()/flow/`; mirrors `internal/clientmachine`. `gitworktree.List(root)` shells out live; mirrors `internal/gitremote`.
- `make ci` must stay green; coverage gate ~80%. CI = `golangci-lint run` + `verify-generate` + `cover` + `build`. Run it before the final commit of each task that changes Go.
- TUI keyboard grammar is centralised in `internal/tui/ui/grammar` (`MoveUp/MoveDown/Top/Bottom/Open(enter)/Edit("e")/Delete("d")/Nachbuchen("n")/Back(q,Esc)/Search("/")/Help("?")/WeekPrev("[")/WeekNext("]")`). Use these bindings; do NOT advertise `j/k/g/G` as hints. List cursors use `internal/tui/ui/listnav` (clamped `Cursor`).
- Navigation: a `shell.Route` drills in by returning `shell.PushRouteMsg{Route: child}` from `Update`; the shell pops on `PopRouteMsg{}` and on the back-key chain (`ResolveBack`, q/Esc). A route that owns the keyboard (a form/dialog) implements `CapturesInput() bool`; one that must receive even Esc/q implements `CapturesText() bool`.
- No emoji. The whitelisted monospace glyphs (◆ ● ▶ ★ ☼ ✚ ▲ ■) are allowed.
- Every commit message ends with the trailer:
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`
- Do NOT add CLI verbs (Slice 4) or the session-edit project picker (Slice 5). This slice is the TUI projects screen + the two client packages + the projectresolve hook only.

---

### Task 1: `internal/gitworktree` — live worktree list + repo root

**Files:**
- Create: `internal/gitworktree/gitworktree.go`
- Test: `internal/gitworktree/gitworktree_test.go`

**Interfaces:**
- Produces: `gitworktree.Worktree{Path, Branch, HeadShort string; Dirty, IsMain bool}`; `gitworktree.List(root string) ([]Worktree, error)`; `gitworktree.Root(dir string) (root string, ok bool, err error)`. Mirrors `internal/gitremote` (real `git` binary, `exec.Command("git","-C",dir,…)`, non-zero-exit → `ok=false,err=nil`).

**Context:** `internal/gitremote/gitremote.go` is the exact template — read it first. `List` parses `git -C root worktree list --porcelain`; `Root` returns the current worktree's top level via `git -C dir rev-parse --show-toplevel` and is used by Task 3 to record the checkout root. `git worktree list` from ANY worktree lists all worktrees of the repo (incl. main), so recording any one worktree's toplevel is sufficient for the detail panel.

- [ ] **Step 1: Write the failing test**

Create `internal/gitworktree/gitworktree_test.go`. Use the real git binary + `t.TempDir()` (mirrors `gitremote_test.go`). Helper runs git commands; create a repo, an initial commit, and one linked worktree, then assert `List` + `Root`.

```go
package gitworktree_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/gitworktree"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRootAndList(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-b", "main")
	git(t, main, "commit", "--allow-empty", "-m", "init")

	// Root from the main worktree.
	root, ok, err := gitworktree.Root(main)
	if err != nil || !ok {
		t.Fatalf("Root ok=%v err=%v", ok, err)
	}
	// macOS /var → /private/var symlink: compare resolved suffixes loosely.
	if !strings.HasSuffix(root, filepath.Base(main)) {
		t.Errorf("Root = %q, want suffix %q", root, filepath.Base(main))
	}

	// Add a linked worktree on a new branch.
	wt := main + "-wt"
	git(t, main, "worktree", "add", "-b", "feature", wt)

	wts, err := gitworktree.List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("want 2 worktrees, got %d: %+v", len(wts), wts)
	}
	var mainSeen, featSeen bool
	for _, w := range wts {
		if w.HeadShort == "" {
			t.Errorf("worktree %q missing HeadShort", w.Path)
		}
		if w.IsMain {
			mainSeen = true
		}
		if w.Branch == "feature" {
			featSeen = true
		}
	}
	if !mainSeen {
		t.Error("no worktree marked IsMain")
	}
	if !featSeen {
		t.Error("feature branch worktree not found")
	}
}

func TestRootNotAGitRepo(t *testing.T) {
	_, ok, err := gitworktree.Root(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Error("non-repo dir must report ok=false")
	}
}

func TestDirtyFlag(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-b", "main")
	git(t, main, "commit", "--allow-empty", "-m", "init")
	// create an untracked-but-ignored-free change → tracked file modification
	if err := exec.Command("sh", "-c", "echo hi > "+filepath.Join(main, "f.txt")).Run(); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", "f.txt")
	git(t, main, "commit", "-m", "add f")
	if err := exec.Command("sh", "-c", "echo changed > "+filepath.Join(main, "f.txt")).Run(); err != nil {
		t.Fatal(err)
	}
	root, _, _ := gitworktree.Root(main)
	wts, _ := gitworktree.List(root)
	if len(wts) != 1 || !wts[0].Dirty {
		t.Errorf("expected single dirty worktree, got %+v", wts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gitworktree/`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement**

Create `internal/gitworktree/gitworktree.go`:

```go
// Package gitworktree reads a repo's worktrees live via the git binary. It is
// READ-ONLY: it never runs write-side git (no add/rm/clone). Mirrors
// internal/gitremote (same exec/error shape).
package gitworktree

import (
	"errors"
	"os/exec"
	"strings"
)

// Worktree is one entry of `git worktree list`. Branch is the short branch name,
// or "" when detached (HeadShort still carries the commit). IsMain marks the
// primary worktree (the first porcelain block).
type Worktree struct {
	Path      string
	Branch    string
	HeadShort string
	Dirty     bool
	IsMain    bool
}

// Root returns dir's worktree top level. ok=false (err=nil) when dir is not a
// git repo. A real error (git missing) is returned as err.
func Root(dir string) (root string, ok bool, err error) {
	out, runErr := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return "", false, nil
		}
		return "", false, runErr
	}
	return strings.TrimSpace(string(out)), true, nil
}

// List returns all worktrees of the repo containing root (git lists every
// worktree from any one of them). The slice is in git's order; the first entry
// is the main worktree.
func List(root string) ([]Worktree, error) {
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	var wts []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			cur.Dirty = isDirty(cur.Path)
			wts = append(wts, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree "), IsMain: len(wts) == 0}
		case cur == nil:
			// skip
		case strings.HasPrefix(line, "HEAD "):
			sha := strings.TrimPrefix(line, "HEAD ")
			if len(sha) > 7 {
				sha = sha[:7]
			}
			cur.HeadShort = sha
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Branch = "" // detached: Branch empty, HeadShort carries identity
		}
	}
	flush()
	return wts, nil
}

// isDirty reports whether the worktree at path has uncommitted tracked changes
// (cheap: ignores untracked files via -uno). Errors → treated as clean.
func isDirty(path string) bool {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain", "-uno").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/gitworktree/`
Expected: PASS (3 tests). If `git worktree add` is unavailable in CI, the test still needs git ≥ 2.5 — the repo already requires git for `gitremote` tests, so this is consistent.

- [ ] **Step 5: Commit**

```bash
git add internal/gitworktree/
git commit -m "feat(project-mgmt): gitworktree.List + Root (live, read-only)"
```

---

### Task 2: `internal/clientcheckout` — per-machine slug→root registry

**Files:**
- Create: `internal/clientcheckout/clientcheckout.go`
- Test: `internal/clientcheckout/clientcheckout_test.go`

**Interfaces:**
- Produces: `clientcheckout.Checkouts{Roots map[string]string}`; `Load() (Checkouts, error)`; `LoadFrom(dir string) (Checkouts, error)`; `(Checkouts).Get(slug string) (string, bool)`; package-level `Record(slug, root string) error` and `RecordIn(dir, slug, root string) error`. Mirrors `internal/clientmachine` (same `Load`/`LoadFrom` pair, `os.UserConfigDir()/flow/`, `0o700` dir / `0o600` file, JSON).

**Context:** Read `internal/clientmachine/machine.go` first — it is the exact template. The registry file is `checkouts.json` holding `{"roots": {"<slug>": "<root>"}}`. `Record` is the package-level convenience (`RecordIn` against the real config dir); `RecordIn(dir,…)` is the testable variant. Task 3 calls `clientcheckout.Record`; the detail route (Task 6) calls `clientcheckout.Load()` then `.Get(slug)`.

- [ ] **Step 1: Write the failing test**

Create `internal/clientcheckout/clientcheckout_test.go`:

```go
package clientcheckout_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/clientcheckout"
)

func TestRecordAndGet(t *testing.T) {
	dir := t.TempDir()
	if err := clientcheckout.RecordIn(dir, "flow", "/home/me/src/flow"); err != nil {
		t.Fatal(err)
	}
	// second slug, and an overwrite of the first
	if err := clientcheckout.RecordIn(dir, "dotfiles", "/home/me/dotfiles"); err != nil {
		t.Fatal(err)
	}
	if err := clientcheckout.RecordIn(dir, "flow", "/home/me/work/flow"); err != nil {
		t.Fatal(err)
	}

	c, err := clientcheckout.LoadFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := c.Get("flow")
	if !ok || root != "/home/me/work/flow" {
		t.Errorf("flow → %q,%v; want /home/me/work/flow,true (overwrite)", root, ok)
	}
	if r, ok := c.Get("dotfiles"); !ok || r != "/home/me/dotfiles" {
		t.Errorf("dotfiles → %q,%v", r, ok)
	}
	if _, ok := c.Get("nope"); ok {
		t.Error("unknown slug must report ok=false")
	}
}

func TestLoadFromMissingFileIsEmpty(t *testing.T) {
	c, err := clientcheckout.LoadFrom(t.TempDir())
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if _, ok := c.Get("anything"); ok {
		t.Error("empty registry must have no entries")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/clientcheckout/`
Expected: FAIL — package undefined.

- [ ] **Step 3: Implement**

Create `internal/clientcheckout/clientcheckout.go`:

```go
// Package clientcheckout is a per-machine registry mapping a project slug to its
// git checkout root on THIS machine. It is inherently device-local (nothing
// crosses devices). Mirrors internal/clientmachine's file pattern.
package clientcheckout

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Checkouts is the on-disk registry. Roots maps projectSlug → checkoutRoot.
type Checkouts struct {
	Roots map[string]string `json:"roots"`
}

// Get returns the recorded checkout root for slug on this machine.
func (c Checkouts) Get(slug string) (string, bool) {
	r, ok := c.Roots[slug]
	return r, ok
}

// Load reads the registry from os.UserConfigDir()/flow/checkouts.json.
func Load() (Checkouts, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Checkouts{Roots: map[string]string{}}, err
	}
	return LoadFrom(filepath.Join(dir, "flow"))
}

// LoadFrom reads <dir>/checkouts.json. A missing or corrupt file yields an empty
// (non-nil) registry, not an error.
func LoadFrom(dir string) (Checkouts, error) {
	path := filepath.Join(dir, "checkouts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Checkouts{Roots: map[string]string{}}, nil
		}
		return Checkouts{Roots: map[string]string{}}, err
	}
	var c Checkouts
	if json.Unmarshal(data, &c) != nil || c.Roots == nil {
		return Checkouts{Roots: map[string]string{}}, nil
	}
	return c, nil
}

// Record upserts slug→root in the real config dir. Non-fatal callers ignore the
// error.
func Record(slug, root string) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return RecordIn(filepath.Join(dir, "flow"), slug, root)
}

// RecordIn upserts slug→root in <dir>/checkouts.json (testable variant).
func RecordIn(dir, slug, root string) error {
	c, err := LoadFrom(dir)
	if err != nil {
		return err
	}
	c.Roots[slug] = root
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "checkouts.json"), out, 0o600)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/clientcheckout/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/clientcheckout/
git commit -m "feat(project-mgmt): clientcheckout per-machine slug→root registry"
```

---

### Task 3: `projectresolve` — auto-record the checkout root

**Files:**
- Modify: `internal/projectresolve/resolve.go`
- Test: `internal/projectresolve/resolve_test.go` (add a test; keep existing ones green)

**Interfaces:**
- Consumes: `gitworktree.Root` (Task 1), `clientcheckout.Record` (Task 2).
- Produces: no signature change to `Resolve`. Adds a package-level seam `var recordCheckout = clientcheckout.Record` so the record side-effect is testable without touching the real config dir.

**Context:** `Resolve` (resolve.go:21-44) already computes `remoteSlug` via `gitremote.OriginSlug(cwd)`. When cwd is inside a git repo with an origin (remoteSlug != ""), record `slug→toplevel` so the detail view (Task 6) can show worktrees on this machine. The hook is non-fatal and must NOT change the resolution result.

- [ ] **Step 1: Write the failing test**

Add to `internal/projectresolve/resolve_test.go` (mirror the existing test's httptest + real-git setup). Stub the `recordCheckout` seam to capture the call instead of writing to disk:

```go
func TestResolve_recordsCheckoutForGitRepo(t *testing.T) {
	// Stub the record seam (restore after).
	var gotSlug, gotRoot string
	restore := projectresolve.SetRecordCheckoutForTest(func(slug, root string) error {
		gotSlug, gotRoot = slug, root
		return nil
	})
	defer restore()

	// A server that has a project bound to the repo's origin slug.
	// (Reuse the existing test's server/helper that seeds a project + remote binding
	// and returns an *apiclient.Client. Mirror whatever the current resolve_test does.)
	ts, client := newResolveServer(t /* seeds project slug "flow" bound to remote "github.com/acme/flow" */)
	defer ts.Close()

	repo := t.TempDir()
	gitInit(t, repo, "git@github.com:acme/flow.git") // helper: git init + remote add origin

	p, ok, err := projectresolve.Resolve(context.Background(), client, func(string) string { return "" }, repo)
	if err != nil || !ok {
		t.Fatalf("resolve ok=%v err=%v", ok, err)
	}
	if p.Slug != "flow" {
		t.Errorf("resolved slug = %q, want flow", p.Slug)
	}
	if gotSlug != "github.com/acme/flow" {
		t.Errorf("recorded slug = %q", gotSlug)
	}
	if gotRoot == "" {
		t.Error("expected a non-empty recorded checkout root")
	}
}

func TestResolve_doesNotRecordOutsideGitRepo(t *testing.T) {
	called := false
	restore := projectresolve.SetRecordCheckoutForTest(func(string, string) error { called = true; return nil })
	defer restore()
	ts, client := newResolveServer(t)
	defer ts.Close()
	// A non-git dir with FLOW_PROJECT set still resolves but must not record.
	_, _, _ = projectresolve.Resolve(context.Background(), client, func(k string) string {
		if k == "FLOW_PROJECT" { return "flow" }
		return ""
	}, t.TempDir())
	if called {
		t.Error("FLOW_PROJECT / non-git path must not record a checkout")
	}
}
```

> **Implementer note:** match the helper names to whatever the existing `resolve_test.go` already provides (server seeding, git-init helper). If it lacks a reusable server helper, add a minimal one mirroring the existing test. The two new behaviours under test: (1) a git-repo resolution records `slug→toplevel`; (2) the FLOW_PROJECT / non-git path records nothing.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/projectresolve/ -run TestResolve_records`
Expected: FAIL — `SetRecordCheckoutForTest` undefined; no record happens.

- [ ] **Step 3: Implement the seam + hook**

In `internal/projectresolve/resolve.go`:

1. Add imports `"github.com/serverkraken/flow/internal/clientcheckout"` and `"github.com/serverkraken/flow/internal/gitworktree"`.
2. Add the seam below the imports:

```go
// recordCheckout records a resolved git repo's slug→root on this machine. A
// package var so tests can stub it (the real impl writes to the user config dir).
var recordCheckout = clientcheckout.Record

// SetRecordCheckoutForTest swaps the record hook and returns a restore func.
func SetRecordCheckoutForTest(f func(slug, root string) error) func() {
	prev := recordCheckout
	recordCheckout = f
	return func() { recordCheckout = prev }
}
```

3. In `Resolve`, right after `remoteSlug, _, _ := gitremote.OriginSlug(cwd)` (only on the non-FLOW_PROJECT path), add the hook:

```go
	remoteSlug, _, _ := gitremote.OriginSlug(cwd)
	if remoteSlug != "" {
		if root, ok, _ := gitworktree.Root(cwd); ok {
			_ = recordCheckout(remoteSlug, root) // non-fatal
		}
	}
```

(The FLOW_PROJECT early-return path is untouched, so that branch never records — satisfying the second test.)

- [ ] **Step 4: Run tests to verify they pass + no regression**

Run: `go test ./internal/projectresolve/`
Expected: PASS (new + existing). Resolution result is unchanged; only the side-effect is added.

- [ ] **Step 5: Commit**

```bash
git add internal/projectresolve/resolve.go internal/projectresolve/resolve_test.go
git commit -m "feat(project-mgmt): projectresolve auto-records checkout root (slug→toplevel)"
```

---

### Task 4: `kindcolor.ProjectColor` — whitelist name → theme color

**Files:**
- Create: `internal/tui/kindcolor/project.go`
- Test: `internal/tui/kindcolor/project_test.go`

**Interfaces:**
- Consumes: `domain.ProjectColors` (whitelist), `theme.Palette` (has fields `Blue, Cyan, Green, Purple, Magenta, Yellow, Orange, Red, Teal theme.Color` + `FgMuted`).
- Produces: `kindcolor.ProjectColor(name string, p theme.Palette) theme.Color` — maps a whitelisted color NAME to the palette's hue; unknown/empty → `p.FgMuted`. This is the TUI analogue of the WebUI `ColorHex` (both consume the same domain whitelist).

**Context:** The WebUI maps names→hex (`webui.ColorHex`); the TUI maps names→`theme.Color`. The drift guard here asserts every `domain.ProjectColors` name resolves to a non-`FgMuted` color, so the two surfaces can't diverge on which names exist. `internal/tui/kindcolor/` already exists (used by docs).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/kindcolor/project_test.go`:

```go
package kindcolor_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Drift guard: every domain palette name must map to a real hue (not the muted
// fallback), else a project could carry a color the TUI renders as grey.
func TestProjectColorCoversWholePalette(t *testing.T) {
	p := theme.Default
	for _, name := range domain.ProjectColors {
		if got := kindcolor.ProjectColor(name, p); got == p.FgMuted {
			t.Errorf("color %q resolved to the muted fallback", name)
		}
	}
	if kindcolor.ProjectColor("", p) != p.FgMuted {
		t.Error("empty name → FgMuted")
	}
	if kindcolor.ProjectColor("chartreuse", p) != p.FgMuted {
		t.Error("unknown name → FgMuted")
	}
}

func TestProjectColorMapsKnown(t *testing.T) {
	p := theme.Default
	if kindcolor.ProjectColor("blue", p) != p.Blue {
		t.Error("blue must map to p.Blue")
	}
	if kindcolor.ProjectColor("teal", p) != p.Teal {
		t.Error("teal must map to p.Teal")
	}
}
```

> **Implementer note:** confirm `theme.Default` is the exported default palette value (it's used across the TUI tests). If the exported name differs (e.g. `theme.TokyonightNight`), use that.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/kindcolor/ -run TestProjectColor`
Expected: FAIL — `ProjectColor` undefined.

- [ ] **Step 3: Implement**

Create `internal/tui/kindcolor/project.go`:

```go
package kindcolor

import (
	"github.com/serverkraken/flow/internal/tui/theme"
)

// ProjectColor maps a whitelisted project color NAME (domain.ProjectColors) to
// the active palette's hue. Unset or unknown names fall back to FgMuted (the
// caller renders a neutral swatch rather than guessing). Single source so the
// TUI cannot drift from the domain whitelist (enforced by a drift-guard test).
func ProjectColor(name string, p theme.Palette) theme.Color {
	switch name {
	case "blue":
		return p.Blue
	case "cyan":
		return p.Cyan
	case "green":
		return p.Green
	case "purple":
		return p.Purple
	case "magenta":
		return p.Magenta
	case "yellow":
		return p.Yellow
	case "orange":
		return p.Orange
	case "red":
		return p.Red
	case "teal":
		return p.Teal
	default:
		return p.FgMuted
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/kindcolor/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/kindcolor/project.go internal/tui/kindcolor/project_test.go
git commit -m "feat(project-mgmt): kindcolor.ProjectColor (TUI whitelist drift-guard)"
```

---

### Task 5: `screen/projects` list route

**Files:**
- Create: `internal/tui/screen/projects/route.go` (the list route, the tab root)
- Create: `internal/tui/screen/projects/api.go` (the narrow client interface + a `*apiclient.Client` assert)
- Test: `internal/tui/screen/projects/route_test.go`

**Interfaces:**
- Consumes: `apiclient.Client.ListProjects(ctx) ([]domain.Project, error)`; `shell.Route`, `shell.Frame`, `shell.EventMsg`, `shell.PushRouteMsg`; `listnav.Cursor`; `ui/grammar` bindings; `kindcolor.ProjectColor`; `theme.Palette`.
- Produces: `projects.NewRoute(api ProjectsAPI, pal theme.Palette, user string) *Route` (implements `shell.Route`, `Title()=="Projekte"`). Status filter cycles `active+paused` (default) → `archived` → `all` via `WeekNext`/`WeekPrev` (`]`/`[`). `enter` emits `shell.PushRouteMsg{Route: detail}` — the detail constructor is wired in Task 8 via a `detailFor func(domain.Project) shell.Route` field (set to nil here; the list still compiles + renders + cursor-navigates without it, and the test asserts the push only after wiring). `n` emits a push to the form (also wired Task 8).

**Context:** Mirror `internal/tui/screen/worktime/week/route.go` (leaf route: stored `listnav.Cursor`, `loadCmd`, `shell.EventMsg` reload) and the docs list for fuzzy+filter feel. The route owns a narrow `ProjectsAPI` interface so tests use a fake (mirrors worktime's `weekAPI`). To keep this task self-contained, `detailFor`/`formFor` are `func` fields defaulting to nil; Task 8 sets them. The Update returns a `PushRouteMsg` only when the corresponding func is non-nil.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/screen/projects/route_test.go`:

```go
package projects_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAPI struct{ ps []domain.Project }

func (f *fakeAPI) ListProjects(context.Context) ([]domain.Project, error) { return f.ps, nil }

func seed() []domain.Project {
	return []domain.Project{
		{ID: "p1", Slug: "aaa", Name: "Aaa", Status: domain.ProjectActive, Color: "blue", Glyph: "◆"},
		{ID: "p2", Slug: "bbb", Name: "Bbb", Status: domain.ProjectPaused},
		{ID: "p3", Slug: "ccc", Name: "Ccc", Status: domain.ProjectArchived},
	}
}

func drainInit(r *projects.Route) { // run Init's load cmd synchronously
	if cmd := r.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			nr, _ := r.Update(msg)
			*r = *nr.(*projects.Route)
		}
	}
}

func view(r *projects.Route) string { return r.View(shell.Frame{Width: 80, Height: 24}) }

func TestListShowsActivePausedHidesArchivedByDefault(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)
	out := view(r)
	if !strings.Contains(out, "Aaa") || !strings.Contains(out, "Bbb") {
		t.Error("default view must list active + paused")
	}
	if strings.Contains(out, "Ccc") {
		t.Error("default view must hide archived")
	}
}

func TestStatusFilterCycleRevealsArchived(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	drainInit(r)
	// `]` advances the filter: default → archived
	nr, _ := r.Update(tea.KeyPressMsg{Code: ']'})
	r = nr.(*projects.Route)
	if !strings.Contains(view(r), "Ccc") {
		t.Error("archived filter must reveal Ccc")
	}
}

func TestEnterPushesDetailWhenWired(t *testing.T) {
	r := projects.NewRoute(&fakeAPI{ps: seed()}, theme.Default, "msoent")
	pushed := false
	r.SetDetailFactory(func(p domain.Project) shell.Route { pushed = true; return nil })
	drainInit(r)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatalf("enter should emit PushRouteMsg, got %T", cmd())
	}
	if !pushed {
		t.Error("detail factory should have been called with the selected project")
	}
}
```

> **Implementer note:** match `tea.KeyPressMsg` construction to the codebase's bubbletea/v2 idiom (see how `week/route_test.go` builds key messages — use the same helper/shape; `tea.KeyPressMsg{Code: ']'}` and `{Code: tea.KeyEnter}` are illustrative). `SetDetailFactory`/`SetFormFactory` are the wiring setters (also used by Task 8).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/screen/projects/`
Expected: FAIL — package undefined.

- [ ] **Step 3: Implement the API interface**

Create `internal/tui/screen/projects/api.go`:

```go
package projects

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// ProjectsAPI is the narrow read used by the list route (a fake implements it in
// tests; *apiclient.Client satisfies it in production).
type ProjectsAPI interface {
	ListProjects(ctx context.Context) ([]domain.Project, error)
}

var _ ProjectsAPI = (*apiclient.Client)(nil)
```

- [ ] **Step 4: Implement the list route**

Create `internal/tui/screen/projects/route.go`. Mirror `worktime/week/route.go` structure. Key pieces (write the full route; the essentials):

```go
package projects

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
)

type statusFilter int

const (
	filterActivePaused statusFilter = iota // default
	filterArchived
	filterAll
)

func (f statusFilter) label() string {
	switch f {
	case filterArchived:
		return "archiviert"
	case filterAll:
		return "alle"
	default:
		return "aktiv + pausiert"
	}
}

type loadedMsg struct {
	ps  []domain.Project
	err error
}

// Route is the projects list (the "Projekte" tab root).
type Route struct {
	api    ProjectsAPI
	pal    theme.Palette
	user   string
	all    []domain.Project // unfiltered, as loaded
	shown  []domain.Project // after status filter
	cur    listnav.Cursor
	filter statusFilter
	err    error

	detailFor func(domain.Project) shell.Route // set by Task 8 wiring
	formFor   func(*domain.Project) shell.Route // nil project → create; set by Task 8
}

func NewRoute(api ProjectsAPI, pal theme.Palette, user string) *Route {
	return &Route{api: api, pal: pal, user: user, cur: listnav.New()}
}

func (r *Route) SetDetailFactory(f func(domain.Project) shell.Route) { r.detailFor = f }
func (r *Route) SetFormFactory(f func(*domain.Project) shell.Route)  { r.formFor = f }

func (r *Route) Title() string { return "Projekte" }

func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ps, err := api.ListProjects(context.Background())
		return loadedMsg{ps: ps, err: err}
	}
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.all, r.err = m.ps, m.err
		r.applyFilter()
		return r, nil
	case shell.EventMsg:
		if isProjectEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		switch {
		case grammar.WeekNext.Matches(m): // `]` cycle filter forward
			r.filter = (r.filter + 1) % 3
			r.applyFilter()
			return r, nil
		case grammar.WeekPrev.Matches(m): // `[` cycle filter back
			r.filter = (r.filter + 2) % 3
			r.applyFilter()
			return r, nil
		case grammar.Nachbuchen.Matches(m): // `n` new project
			if r.formFor != nil {
				return r, push(r.formFor(nil))
			}
			return r, nil
		case grammar.Open.Matches(m): // enter → detail
			if r.detailFor != nil && len(r.shown) > 0 {
				return r, push(r.detailFor(r.shown[r.cur.Index()]))
			}
			return r, nil
		}
		if c, ok := r.cur.Handle(m, len(r.shown), 0); ok {
			r.cur = c
			return r, nil
		}
	}
	return r, nil
}

func push(child shell.Route) tea.Cmd {
	return func() tea.Msg { return shell.PushRouteMsg{Route: child} }
}

func (r *Route) applyFilter() {
	r.shown = r.shown[:0]
	for _, p := range r.all {
		switch r.filter {
		case filterAll:
			r.shown = append(r.shown, p)
		case filterArchived:
			if p.Status == domain.ProjectArchived {
				r.shown = append(r.shown, p)
			}
		default:
			if p.Status == domain.ProjectActive || p.Status == domain.ProjectPaused {
				r.shown = append(r.shown, p)
			}
		}
	}
	r.cur = r.cur.Clamp(len(r.shown))
}

func (r *Route) View(f shell.Frame) string {
	// Render: a filter line ("Filter: <label>  ([/] wechseln)"), then one row per
	// r.shown: "<glyph> <name>   <status>", cursor row highlighted, color dot via
	// kindcolor.ProjectColor(p.Color, r.pal). Empty → "Keine Projekte." Mirror the
	// row/cursor styling of worktime/week/route.go's View and docs list. Use
	// r.pal + lipgloss/v2; keep within f.Width.
	_ = kindcolor.ProjectColor // used in row rendering
	// ... full View implementation ...
}

func (r *Route) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		grammar.Open.Hint(), grammar.Nachbuchen.Hint(),
		{Key: "[ ]", Desc: "Filter"}, grammar.MoveUp.Hint(), grammar.Back.Hint(),
	}
}

// isProjectEvent reports whether a client SSE event should reload the list.
func isProjectEvent(t string) bool {
	return t == "project.created" || t == "project.updated" || t == "project.deleted"
}
```

> **Implementer note:** confirm `listnav.Cursor` has `New()`, `Handle(msg, n, viewport) (Cursor, bool)`, `Index() int`, `Clamp(n) Cursor` (or the codebase's equivalents — read `ui/listnav`; adapt names). Confirm `grammar.WeekNext/WeekPrev/Open/Nachbuchen/MoveUp/Back` exist (they do, per the grammar package). Write the full `View` with real lipgloss styling mirroring `week/route.go`. Keep the file focused; if it grows past ~200 lines, split the View into `view.go` (no-monolith).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/screen/projects/ && go build ./...`
Expected: PASS; builds. Then `golangci-lint run` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/projects/route.go internal/tui/screen/projects/api.go internal/tui/screen/projects/route_test.go
git commit -m "feat(project-mgmt): TUI projects list route (filter + cursor + SSE reload)"
```

---

### Task 6: `screen/projects` detail cockpit route (incl. read-only worktree panel)

**Files:**
- Create: `internal/tui/screen/projects/detail.go`
- Create: `internal/tui/screen/projects/worktime.go` (per-project aggregate helper — keep detail.go focused)
- Test: `internal/tui/screen/projects/detail_test.go`

**Interfaces:**
- Consumes: `apiclient.Client` methods `GetProject(ctx,id)`, `ListSessionsRange(ctx,since,until) ([]domain.WorkSession,error)`, `ListDocumentsScoped(ctx,*projectID,...tags) ([]domain.Document,error)`, `ListBindings(ctx) ([]domain.ProjectBinding,error)`, `UpdateProject(ctx,id,apiclient.UpdateProjectFields) (domain.Project,error)`; `clientcheckout.Load`, `gitworktree.List`; `markdown.Render`; `kindcolor.ProjectColor`; `shell.PushRouteMsg`/`PopRouteMsg`.
- Produces: `projects.NewDetailRoute(api DetailAPI, pal theme.Palette, p domain.Project) *DetailRoute` (implements `shell.Route`, `Title()==p.Name`). `e` → push the edit form (via a `formFor` setter wired in Task 8). Status actions: `p` pause / `r` resume / `a` archive call `UpdateProject` with the full current fields + new status, then reload. SSE `project.updated` reloads.

**Context:** This mirrors the WebUI Slice-2 cockpit (`webui_projects.go` `projectCockpitData` + `projectWorktime`). The worktime aggregate is computed CLIENT-SIDE from `ListSessionsRange` (no per-project backend usecase): sum completed sessions whose `*ProjectID == p.ID`; total / current-week / current-month; earnings = `p.Rate.Mul(totalDur).String()` when `p.Rate != nil`. The worktree panel: `clientcheckout.Load().Get(p.Slug)` → root; if found, `gitworktree.List(root)` and render each line; if not, render the hint "nicht ausgecheckt auf diesem PC". The panel is READ-ONLY (no cursor, no actions).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/screen/projects/detail_test.go`:

```go
package projects_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDetailAPI struct {
	p        domain.Project
	sessions []domain.WorkSession
}

func (f *fakeDetailAPI) GetProject(context.Context, string) (domain.Project, error) { return f.p, nil }
func (f *fakeDetailAPI) ListSessionsRange(context.Context, time.Time, time.Time) ([]domain.WorkSession, error) {
	return f.sessions, nil
}
func (f *fakeDetailAPI) ListDocumentsScoped(context.Context, *string, ...string) ([]domain.Document, error) {
	return nil, nil
}
func (f *fakeDetailAPI) ListBindings(context.Context) ([]domain.ProjectBinding, error) { return nil, nil }
func (f *fakeDetailAPI) UpdateProject(_ context.Context, _ string, in projects.UpdateFields) (domain.Project, error) {
	f.p.Status = domain.ProjectStatus(in.Status)
	return f.p, nil
}

func detailView(r *projects.DetailRoute) string { return r.View(shell.Frame{Width: 80, Height: 30}) }

func TestDetailRendersCockpit(t *testing.T) {
	p := domain.Project{
		ID: "p1", Slug: "flow", Name: "Flow", Status: domain.ProjectPaused,
		Description: "# Notiz\nhallo", UpstreamGit: "git@github.com:acme/flow.git", Color: "blue",
	}
	api := &fakeDetailAPI{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)
	if cmd := r.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			nr, _ := r.Update(msg)
			r = nr.(*projects.DetailRoute)
		}
	}
	out := detailView(r)
	for _, want := range []string{"Flow", "Notiz", "nicht ausgecheckt", "pausiert"} {
		if !strings.Contains(out, want) {
			t.Errorf("cockpit missing %q\n%s", want, out)
		}
	}
}

func TestDetailStatusActionArchives(t *testing.T) {
	p := domain.Project{ID: "p1", Slug: "flow", Name: "Flow", Status: domain.ProjectActive}
	api := &fakeDetailAPI{p: p}
	r := projects.NewDetailRoute(api, theme.Default, p)
	// `a` archives via UpdateProject then reload
	nr, cmd := r.Update(keyPress('a'))
	r = nr.(*projects.DetailRoute)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			r2, _ := r.Update(msg)
			r = r2.(*projects.DetailRoute)
		}
	}
	if api.p.Status != domain.ProjectArchived {
		t.Errorf("status action did not archive: %s", api.p.Status)
	}
}
```

> **Implementer note:** `keyPress(rune)` — reuse the key-message helper from Task 5's test (extract it to a shared `helpers_test.go` in the package). Confirm `domain.WorkSession`'s project field is `ProjectID *string` and that it has `Running() bool` + `Elapsed(now time.Time) time.Duration` (the WebUI cockpit uses exactly these — mirror `internal/adapter/httpserver/webui_projects.go`'s `projectWorktime`). `projects.UpdateFields` is this package's alias for the apiclient update struct (defined in Step 3) so the fake doesn't import apiclient's field names verbatim.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/screen/projects/ -run TestDetail`
Expected: FAIL — `NewDetailRoute`/`DetailRoute`/`UpdateFields` undefined.

- [ ] **Step 3: Implement the per-project worktime helper**

Create `internal/tui/screen/projects/worktime.go` — copy the idiom from `internal/adapter/httpserver/webui_projects.go`'s `projectWorktime` (settled sessions only; skip running; sum total/week/month; earnings via `Money.Mul`):

```go
package projects

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

type worktimeAgg struct {
	Total, Week, Month time.Duration
	Earnings           string // "" when no rate
}

// aggregate sums settled sessions for project id. Mirrors the WebUI cockpit:
// running sessions are excluded (settled-time summary); earnings = rate × total.
func aggregate(p domain.Project, sessions []domain.WorkSession, now time.Time) worktimeAgg {
	weekStart := startOfWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var a worktimeAgg
	for _, s := range sessions {
		if s.ProjectID == nil || *s.ProjectID != p.ID || s.Running() {
			continue
		}
		d := s.Elapsed(now)
		a.Total += d
		if !s.Start().Before(weekStart) {
			a.Week += d
		}
		if !s.Start().Before(monthStart) {
			a.Month += d
		}
	}
	if p.Rate != nil {
		a.Earnings = p.Rate.Mul(a.Total).String()
	}
	return a
}

func startOfWeek(now time.Time) time.Time {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// ISO week: Monday start.
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off)
}
```

> **Implementer note:** confirm `domain.WorkSession` exposes `Start() time.Time` (or a `Start` field) and `Elapsed(now)`/`Running()` — match the exact accessors the WebUI cockpit used. If sessions carry start as a field not a method, adjust.

- [ ] **Step 4: Implement the detail route**

Create `internal/tui/screen/projects/detail.go`. Extend `DetailAPI` (its own interface), define `UpdateFields` alias, render the cockpit sections. Essentials:

```go
package projects

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/clientcheckout"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitworktree"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/markdown"
)

// UpdateFields mirrors apiclient.UpdateProjectFields (aliased so tests + this
// package don't depend on the apiclient struct's literal field tags).
type UpdateFields struct {
	Name, Slug, Color, Glyph, Description, UpstreamGit, Status string
}

type DetailAPI interface {
	GetProject(ctx context.Context, id string) (domain.Project, error)
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
	ListDocumentsScoped(ctx context.Context, projectID *string, tags ...string) ([]domain.Document, error)
	ListBindings(ctx context.Context) ([]domain.ProjectBinding, error)
	UpdateProject(ctx context.Context, id string, in UpdateFields) (domain.Project, error)
}

type detailLoadedMsg struct {
	p     domain.Project
	agg   worktimeAgg
	docs  []domain.Document
	binds []domain.ProjectBinding
	wts   []gitworktree.Worktree
	root  string // "" = not checked out on this machine
}

type DetailRoute struct {
	api  DetailAPI
	pal  theme.Palette
	p    domain.Project
	data detailLoadedMsg
	now  func() time.Time

	formFor func(*domain.Project) shell.Route
}

func NewDetailRoute(api DetailAPI, pal theme.Palette, p domain.Project) *DetailRoute {
	return &DetailRoute{api: api, pal: pal, p: p, now: time.Now}
}

func (r *DetailRoute) SetFormFactory(f func(*domain.Project) shell.Route) { r.formFor = f }
func (r *DetailRoute) Title() string                                      { return r.p.Name }
func (r *DetailRoute) Init() tea.Cmd                                      { return r.loadCmd() }

func (r *DetailRoute) loadCmd() tea.Cmd {
	api, p, now := r.api, r.p, r.now()
	return func() tea.Msg {
		fresh, err := api.GetProject(context.Background(), p.ID)
		if err == nil {
			p = fresh
		}
		// per-project worktime: pull a wide range and aggregate client-side.
		sessions, _ := api.ListSessionsRange(context.Background(), now.AddDate(-5, 0, 0), now)
		agg := aggregate(p, sessions, now)
		pid := p.ID
		docs, _ := api.ListDocumentsScoped(context.Background(), &pid)
		allBinds, _ := api.ListBindings(context.Background())
		var binds []domain.ProjectBinding
		for _, b := range allBinds {
			if b.ProjectID == p.ID {
				binds = append(binds, b)
			}
		}
		// worktree panel (client-local).
		var wts []gitworktree.Worktree
		var root string
		if reg, _ := clientcheckout.Load(); true {
			if rt, ok := reg.Get(p.Slug); ok {
				root = rt
				wts, _ = gitworktree.List(rt)
			}
		}
		return detailLoadedMsg{p: p, agg: agg, docs: docs, binds: binds, wts: wts, root: root}
	}
}

func (r *DetailRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case detailLoadedMsg:
		r.p, r.data = m.p, m
		return r, nil
	case shell.EventMsg:
		if m.Ev.Type == "project.updated" {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		switch {
		case grammar.Edit.Matches(m):
			if r.formFor != nil {
				return r, push(r.formFor(&r.p))
			}
		case keyIs(m, 'p'):
			return r, r.setStatusCmd("paused")
		case keyIs(m, 'r'):
			return r, r.setStatusCmd("active")
		case keyIs(m, 'a'):
			return r, r.setStatusCmd("archived")
		}
	}
	return r, nil
}

// setStatusCmd applies a status transition (full-field UpdateProject, mirrors
// the WebUI status handler) then reloads.
func (r *DetailRoute) setStatusCmd(status string) tea.Cmd {
	api, p := r.api, r.p
	return func() tea.Msg {
		_, _ = api.UpdateProject(context.Background(), p.ID, UpdateFields{
			Name: p.Name, Slug: p.Slug, Color: p.Color, Glyph: p.Glyph,
			Description: p.Description, UpstreamGit: p.UpstreamGit, Status: status,
		})
		return reloadMsg{}
	}
}

type reloadMsg struct{}

func (r *DetailRoute) View(f shell.Frame) string {
	// Sections (top→bottom), each only when it has content:
	//  • header: <glyph> <name>  <status badge> (color via kindcolor.ProjectColor)
	//  • description: markdown.Render(r.p.Description, f.Width, markdown.WithPalette(r.pal))
	//    (only when non-empty; ignore render error → show raw)
	//  • Git: r.p.UpstreamGit (only when non-empty)
	//  • Worktime: "Σ <total>  Woche <week>  Monat <month>  [<earnings>]" from r.data.agg
	//  • Worktrees: if r.data.root=="" → "nicht ausgecheckt auf diesem PC";
	//    else one line per r.data.wts: "<*main> <branch|HEAD short> <dirty?>  <path>"
	//  • Dokumente: list r.data.docs titles
	//  • Bindings: list r.data.binds ("<kind>: <slug|path>")
	//  • actions hint line (Bearbeiten/Pausieren/…)
	// Mirror the WebUI cockpit section order + the worktime/week View styling.
	// ... full implementation ...
}

func (r *DetailRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{
		grammar.Edit.Hint(),
		{Key: "p/r/a", Desc: "Pausieren/Fortsetzen/Archivieren"},
		grammar.Back.Hint(),
	}
}
```

Handle `reloadMsg` in `Update` (return `r, r.loadCmd()`). Add `keyIs(m tea.KeyPressMsg, r rune) bool` to the package's key helpers.

> **Implementer note:** confirm `markdown.Render(source, width, ...Option)` signature + `markdown.WithPalette` (per Task reference: `markdown/render.go:95`). Confirm `domain.ProjectBinding` fields (`Kind`, `RemoteSlug`, `Path`, `ProjectID`) — render `RemoteSlug` for remote, `Path` for path kind (mirror the WebUI `bindingTarget`). Keep `detail.go` focused; move the `View` body to `detailview.go` if it grows past ~200 lines.

- [ ] **Step 5: Run tests, build, lint**

Run: `go test ./internal/tui/screen/projects/ -run TestDetail && go build ./... && golangci-lint run`
Expected: PASS; clean.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/screen/projects/detail.go internal/tui/screen/projects/worktime.go internal/tui/screen/projects/detail_test.go
git commit -m "feat(project-mgmt): TUI project detail cockpit + read-only worktree panel"
```

---

### Task 7: `screen/projects` create/edit form route

**Files:**
- Create: `internal/tui/screen/projects/form.go`
- Test: `internal/tui/screen/projects/form_test.go`

**Interfaces:**
- Consumes: `apiclient.Client` `CreateProject(ctx,name)`, `UpdateProject(ctx,id,UpdateFields-equivalent)`, `SetProjectRate(ctx,projectID,*int64,currency)`; `ui/form.NewTextInput`; `domain.ProjectColors`/`ProjectGlyphs`; `kindcolor.ProjectColor`; `shell.PopRouteMsg`.
- Produces: `projects.NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Project) *FormRoute` (implements `shell.Route` + `CapturesInput() bool { return true }`). On submit: create composes `CreateProject` → `UpdateProject` → optional `SetProjectRate` (mirrors WebUI Slice-2); edit = `UpdateProject` + `SetProjectRate` (blank amount clears). On success emits `shell.PopRouteMsg{}` (back to list/detail, which reloads via SSE). Esc cancels (pop).

**Context:** Mirror the multi-field dialog pattern in `worktime/dialogs.go` (a `[]textinput.Model` + focus index; Tab/Down advance; Enter on last submits; Esc cancels). The form owns the keyboard (`CapturesInput`). Fields in order: Name, Slug, Beschreibung, Upstream, **Status** (cycle active/paused/archived with ←/→), **Farbe** (cycle "" + `domain.ProjectColors` with ←/→, swatch via `kindcolor.ProjectColor`), **Glyph** (cycle "" + `domain.ProjectGlyphs`), Satz (rate amount), Währung. Rate parse: blank → `nil` (clears); else cents `int64(f*100 + 0.5)`.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/screen/projects/form_test.go`:

```go
package projects_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeFormAPI struct {
	created   domain.Project
	updated   projects.UpdateFields
	rateCents *int64
	rateSet   bool
}

func (f *fakeFormAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	f.created = domain.Project{ID: "new1", Name: name, Slug: name}
	return f.created, nil
}
func (f *fakeFormAPI) UpdateProject(_ context.Context, id string, in projects.UpdateFields) (domain.Project, error) {
	f.updated = in
	return domain.Project{ID: id, Name: in.Name, Slug: in.Slug, Status: domain.ProjectStatus(in.Status)}, nil
}
func (f *fakeFormAPI) SetProjectRate(_ context.Context, _ string, amount *int64, _ string) error {
	f.rateSet, f.rateCents = true, amount
	return nil
}

func TestFormCreateComposes(t *testing.T) {
	api := &fakeFormAPI{}
	r := projects.NewFormRoute(api, theme.Default, nil) // create
	// Drive: type a name, set rate, submit. The implementer exposes a test seam
	// SubmitForTest(values) OR the test types via key msgs. Prefer a small
	// fillForTest setter to keep the test resilient:
	r.FillForTest(projects.FormValues{Name: "PM TUI", Slug: "pm-tui", Status: "active",
		Color: "blue", Glyph: "◆", RateAmount: "90.00", RateCurrency: "EUR"})
	_, cmd := r.Submit()
	if api.created.Name != "PM TUI" {
		t.Fatalf("CreateProject not called with name: %+v", api.created)
	}
	if api.updated.Status != "active" || api.updated.Color != "blue" {
		t.Errorf("UpdateProject compose wrong: %+v", api.updated)
	}
	if !api.rateSet || api.rateCents == nil || *api.rateCents != 9000 {
		t.Errorf("rate should be 9000 cents, got set=%v cents=%v", api.rateSet, api.rateCents)
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(shell.PopRouteMsg); !ok {
			t.Errorf("success should pop, got %T", msg)
		}
	}
}

func TestFormEditClearsRateOnBlank(t *testing.T) {
	api := &fakeFormAPI{}
	editing := &domain.Project{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
	r := projects.NewFormRoute(api, theme.Default, editing)
	r.FillForTest(projects.FormValues{Name: "Flow", Slug: "flow", Status: "paused", RateAmount: "", RateCurrency: "EUR"})
	_, _ = r.Submit()
	if api.updated.Status != "paused" {
		t.Errorf("edit should set paused, got %q", api.updated.Status)
	}
	if !api.rateSet || api.rateCents != nil {
		t.Errorf("blank rate must clear (nil), got set=%v cents=%v", api.rateSet, api.rateCents)
	}
}
```

> **Implementer note:** expose `FormValues` (exported struct: Name, Slug, Description, UpstreamGit, Status, Color, Glyph, RateAmount, RateCurrency strings), `FillForTest(FormValues)` (sets the field models/selectors from values), and `Submit() (shell.Route, tea.Cmd)` (the same path Enter-on-last triggers). These seams keep the test from depending on exact key choreography while still exercising the real compose + rate parse. The interactive Update (key handling, focus, ←/→ cycling) is also implemented but tested via the seams + a focus-movement test you add.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/screen/projects/ -run TestForm`
Expected: FAIL — `NewFormRoute`/`FormValues`/`FillForTest`/`Submit` undefined.

- [ ] **Step 3: Implement the form route**

Create `internal/tui/screen/projects/form.go`. Define `FormAPI`, `FormValues`, the route with `[]textinput.Model` + selector indices, the compose on submit, and the cycling pickers. Essentials:

```go
package projects

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/bubbles/v2/textinput"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/form"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

type FormAPI interface {
	CreateProject(ctx context.Context, name string) (domain.Project, error)
	UpdateProject(ctx context.Context, id string, in UpdateFields) (domain.Project, error)
	SetProjectRate(ctx context.Context, projectID string, amount *int64, currency string) error
}

type FormValues struct {
	Name, Slug, Description, UpstreamGit, Status, Color, Glyph, RateAmount, RateCurrency string
}

// colorChoices / glyphChoices prepend "" (none) to the domain whitelists.
var colorChoices = append([]string{""}, domain.ProjectColors...)
var glyphChoices = append([]string{""}, domain.ProjectGlyphs...)
var statusChoices = []string{"active", "paused", "archived"}

type FormRoute struct {
	api     FormAPI
	pal     theme.Palette
	editing *domain.Project // nil = create

	inputs   []textinput.Model // Name, Slug, Description, Upstream, RateAmount, RateCurrency
	cur      int
	statusIx int
	colorIx  int
	glyphIx  int
	err      string
}

func NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Project) *FormRoute {
	r := &FormRoute{api: api, pal: pal, editing: editing}
	// build 6 themed text inputs via form.NewTextInput(placeholder, pal)
	// prefill from editing when non-nil (incl. rate → "%d.%02d", and select indices)
	// ... init inputs + indices ...
	return r
}

func (r *FormRoute) CapturesInput() bool { return true }

func (r *FormRoute) values() FormValues {
	return FormValues{
		Name: r.inputs[0].Value(), Slug: r.inputs[1].Value(), Description: r.inputs[2].Value(),
		UpstreamGit: r.inputs[3].Value(), Status: statusChoices[r.statusIx],
		Color: colorChoices[r.colorIx], Glyph: glyphChoices[r.glyphIx],
		RateAmount: r.inputs[4].Value(), RateCurrency: r.inputs[5].Value(),
	}
}

// FillForTest sets the form state from values (test seam).
func (r *FormRoute) FillForTest(v FormValues) {
	r.inputs[0].SetValue(v.Name); r.inputs[1].SetValue(v.Slug); r.inputs[2].SetValue(v.Description)
	r.inputs[3].SetValue(v.UpstreamGit); r.inputs[4].SetValue(v.RateAmount); r.inputs[5].SetValue(v.RateCurrency)
	r.statusIx = indexOf(statusChoices, orDefault(v.Status, "active"))
	r.colorIx = indexOf(colorChoices, v.Color)
	r.glyphIx = indexOf(glyphChoices, v.Glyph)
}

// Submit runs the create/edit compose and returns a Pop on success (mirrors the
// WebUI Slice-2 handlers).
func (r *FormRoute) Submit() (shell.Route, tea.Cmd) {
	v := r.values()
	if strings.TrimSpace(v.Name) == "" {
		r.err = "Name erforderlich"
		return r, nil
	}
	rate, perr := parseRateCents(v.RateAmount)
	if perr != nil {
		r.err = perr.Error()
		return r, nil
	}
	api, editing := r.api, r.editing
	cur := strings.TrimSpace(v.RateCurrency)
	if cur == "" {
		cur = "EUR"
	}
	return r, func() tea.Msg {
		id := ""
		if editing != nil {
			id = editing.ID
		} else {
			p, err := api.CreateProject(context.Background(), v.Name)
			if err != nil {
				return formErrMsg{"Konnte Projekt nicht anlegen"}
			}
			id = p.ID
		}
		if _, err := api.UpdateProject(context.Background(), id, UpdateFields{
			Name: v.Name, Slug: v.Slug, Color: v.Color, Glyph: v.Glyph,
			Description: v.Description, UpstreamGit: v.UpstreamGit, Status: v.Status,
		}); err != nil {
			return formErrMsg{err.Error()}
		}
		_ = api.SetProjectRate(context.Background(), id, rate, cur) // nil clears
		return shell.PopRouteMsg{}
	}
}

type formErrMsg struct{ msg string }

func (r *FormRoute) Title() string { if r.editing != nil { return "Projekt bearbeiten" }; return "Neues Projekt" }
func (r *FormRoute) Init() tea.Cmd { return textinput.Blink }

func (r *FormRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case formErrMsg:
		r.err = m.msg
		return r, nil
	case tea.KeyPressMsg:
		switch m.String() {
		case "esc":
			return r, func() tea.Msg { return shell.PopRouteMsg{} }
		case "enter":
			return r.Submit()
		case "tab", "down":
			r.focusNext(1)
			return r, nil
		case "shift+tab", "up":
			r.focusNext(-1)
			return r, nil
		case "left":
			r.cycleSelector(-1)
			return r, nil
		case "right":
			r.cycleSelector(1)
			return r, nil
		}
		// forward to the focused text input (when on a text field)
		// ... update r.inputs[r.cur] ...
	}
	return r, nil
}

func (r *FormRoute) View(f shell.Frame) string {
	// Render: title; optional rose error (r.err); each labelled field; the three
	// selectors (Status/Farbe/Glyph) rendered as "‹ value ›" with the color swatch
	// for Farbe via kindcolor.ProjectColor; a hint line "Enter speichern · Esc abbrechen".
	// Mirror worktime/dialogs.go field styling.
	// ... full implementation ...
}

func (r *FormRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{{Key: "enter", Desc: "speichern"}, {Key: "esc", Desc: "abbrechen"}, {Key: "←/→", Desc: "Auswahl"}}
}

// parseRateCents: blank → (nil,nil) = clear; else cents int64(f*100+0.5); negative/NaN → error.
func parseRateCents(amount string) (*int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	fAmt, err := strconv.ParseFloat(amount, 64)
	if err != nil || fAmt < 0 {
		return nil, errInvalidRate(amount)
	}
	c := int64(fAmt*100 + 0.5)
	return &c, nil
}
```

Add small helpers `indexOf`, `orDefault`, `errInvalidRate`, `focusNext`, `cycleSelector` (cycleSelector mutates statusIx/colorIx/glyphIx only when the cursor is on a selector row). Map the selector rows into the focus order alongside the text inputs.

> **Implementer note:** confirm the bubbles/v2 textinput import path (`github.com/charmbracelet/bubbles/v2/textinput`) matches the codebase (check an existing user, e.g. `ui/form/textinput.go` or `worktime/dialogs.go`). Confirm `form.NewTextInput(placeholder, pal)` signature. The focus model interleaves 6 text inputs + 3 selectors; pick a single `cur` index over a combined ordered list of 9 focusables (decide the order: Name, Slug, Description, Upstream, Status, Farbe, Glyph, Satz, Währung) and route ←/→ to selectors / typing to text inputs based on which focusable is active. Keep `form.go` focused; split View to `formview.go` if it grows past ~200 lines.

- [ ] **Step 4: Run tests, build, lint**

Run: `go test ./internal/tui/screen/projects/ && go build ./... && golangci-lint run`
Expected: all projects tests PASS; clean.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/screen/projects/form.go internal/tui/screen/projects/form_test.go
git commit -m "feat(project-mgmt): TUI project create/edit form (whitelist pickers + rate)"
```

---

### Task 8: Wire navigation + mount the "Projekte" tab (main-wiring task)

**Files:**
- Create: `internal/tui/screen/projects/mount.go` (the wiring constructor that links list↔detail↔form)
- Modify: `cmd/flow/ui.go` (add the tab + deep-link)
- Test: `internal/tui/screen/projects/mount_test.go`; manual `cmd/flow/ui.go` smoke

**Interfaces:**
- Consumes: `NewRoute`/`NewDetailRoute`/`NewFormRoute` + their `SetDetailFactory`/`SetFormFactory` setters; `*apiclient.Client`; `theme.Palette`.
- Produces: `projects.Mount(client *apiclient.Client, pal theme.Palette, user string) shell.Route` — returns the list route with the detail/form factories wired (so `enter`→detail, `n`→create-form, detail `e`→edit-form all push real routes). Mounted as tab index 3 in `cmd/flow/ui.go`.

**Context:** This closes the per-task gap the standing lesson warns about (the composition root never calling the new constructor). The factories are closures capturing `client`/`pal`; the detail route gets a form factory too (for its `e` action).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/screen/projects/mount_test.go`:

```go
package projects_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestMountWiresDrillDown(t *testing.T) {
	// Mount with a fake api that lists one project; enter must push a DetailRoute.
	root := projects.MountWithAPI(&fakeAPI{ps: seed()}, &fakeDetailAPI{p: seed()[0]}, &fakeFormAPI{}, theme.Default, "msoent")
	r := root.(*projects.Route)
	drainInit(r)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd()
	push, ok := msg.(shell.PushRouteMsg)
	if !ok {
		t.Fatalf("enter should push, got %T", msg)
	}
	if _, ok := push.Route.(*projects.DetailRoute); !ok {
		t.Fatalf("enter should push a DetailRoute, got %T", push.Route)
	}
	_ = domain.ProjectActive
}
```

> **Implementer note:** expose `MountWithAPI(list ProjectsAPI, detail DetailAPI, form FormAPI, pal, user)` as the DI seam that `Mount` (production) delegates to with a single `*apiclient.Client` satisfying all three interfaces. This lets the test inject fakes; production passes the real client three times.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/screen/projects/ -run TestMount`
Expected: FAIL — `MountWithAPI`/`Mount` undefined.

- [ ] **Step 3: Implement the mount wiring**

Create `internal/tui/screen/projects/mount.go`:

```go
package projects

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Mount builds the Projekte tab root with list↔detail↔form navigation wired.
func Mount(client *apiclient.Client, pal theme.Palette, user string) shell.Route {
	return MountWithAPI(client, client, client, pal, user)
}

// MountWithAPI is the DI seam (tests inject fakes; production passes the client
// three times — it satisfies all three interfaces).
func MountWithAPI(list ProjectsAPI, detail DetailAPI, form FormAPI, pal theme.Palette, user string) shell.Route {
	root := NewRoute(list, pal, user)
	formFactory := func(editing *domain.Project) shell.Route { return NewFormRoute(form, pal, editing) }
	root.SetFormFactory(formFactory)
	root.SetDetailFactory(func(p domain.Project) shell.Route {
		d := NewDetailRoute(detail, pal, p)
		d.SetFormFactory(formFactory)
		return d
	})
	return root
}
```

Add the `var _ DetailAPI = (*apiclient.Client)(nil)` and `var _ FormAPI = (*apiclient.Client)(nil)` asserts (in api.go or here) so a drift in the client surface fails the build.

- [ ] **Step 4: Mount the tab in `cmd/flow/ui.go`**

In `cmd/flow/ui.go`:
1. Add import: `projectscreen "github.com/serverkraken/flow/internal/tui/screen/projects"`.
2. In the `WithTabs([]shell.Route{...})` slice, append as index 3:

```go
		projectscreen.Mount(client, pal, user), // 3 — "Projekte"
```

3. In `tabIndexForArg`, add a case:

```go
	case "projekte", "projects", "p":
		return 3
```

- [ ] **Step 5: Generate nothing; run tests, build, lint**

Run: `go test ./internal/tui/screen/projects/ ./cmd/flow/ && go build ./... && golangci-lint run`
Expected: PASS; clean. Confirm `var _ DetailAPI = (*apiclient.Client)(nil)` etc. compile (this is the wiring guard).

- [ ] **Step 6: Manual smoke (composition root actually runs)**

Run: `go run ./cmd/flow ui projekte` against the dev stack (Task 9 brings it up) OR at minimum `go build -o /tmp/flow ./cmd/flow && /tmp/flow ui --help` to confirm the binary builds and the tab arg is accepted. Full interactive smoke is the Task-9 done-gate.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/screen/projects/mount.go internal/tui/screen/projects/mount_test.go cmd/flow/ui.go
git commit -m "feat(project-mgmt): wire Projekte tab into flow ui (list↔detail↔form)"
```

---

### Task 9: Done-gate (live, against the dev stack)

**Files:** none (verification only). If a gap is found, fix it in the relevant task's files and re-commit.

- [ ] **Step 1: Full CI gate**

Run: `make ci` (with the podman socket up so pgstore tests run, as in prior slices).
Expected: lint + verify-generate + cover (≥ ~80%) + build all green.

- [ ] **Step 2: Confirm the tab + routes are wired**

Run: `rg -n "projectscreen|Mount\(client|projekte" cmd/flow/ui.go`
Expected: the import, the `Mount(client, pal, user)` tab entry, and the `tabIndexForArg` case all present.

- [ ] **Step 3: Bring up the dev stack + run the TUI**

`make dev-up`; obtain a token (`make dev-token` / `flow login` per the dev env); `go run ./cmd/flow ui projekte` (or `flow ui` then Tab to Projekte).

- [ ] **Step 4: Interactive dogfood (the done-gate)**

Verify in the TUI:
- The "Projekte" tab appears (4th tab); the list shows active+paused; `[`/`]` cycles the filter to archived/all and back; `n` opens the create form.
- Create a project with a color + glyph + upstream + rate → on save returns to the list, the new project appears (SSE reload), the color swatch + glyph render.
- `enter` opens the detail cockpit: glyph+color+name+status, rendered markdown description, Git line (upstream), worktime Σ (+ € once a session exists for it), Dokumente, Bindings (the auto-synced remote binding), and the worktree panel.
- In a checked-out repo (cd into this repo, run any `flow` command once so `projectresolve` records the checkout), the detail worktree panel lists the repo's worktrees (main marked, a `git worktree add` one shows its branch + dirty flag); a project with no checkout on this PC shows "nicht ausgecheckt auf diesem PC".
- `e` opens the edit form prefilled; change status active→paused→archived; on save the detail/list reflect it; clearing the rate (blank amount) removes the € from the cockpit.
- `p`/`r`/`a` status actions on the detail change status live.
- `q`/Esc pops detail→list, list→(tab); no double-pop, no clock/stat freeze.

- [ ] **Step 5: Final commit (only if Steps 1–4 required fixes)**

```bash
git add -A
git commit -m "chore(project-mgmt): slice-3 done-gate fixes"
```

---

## Self-Review

**1. Spec coverage (Slice 3 = TUI scope, spec §322 item 3):**
- `screen/projects` list (fuzzy/filter, glyph+color+name+status, enter→detail, n→new) → Task 5. ✓ (fuzzy: the list uses status-filter + cursor; if the spec's "fuzzy" means a `/`-search filter, the implementer adds a `ui/fuzzylist`-style filter in Task 5 — noted; the existing docs/booking fuzzy pattern is the reference.)
- Detail cockpit (description M3d renderer · worktime Σ+€ · docs · git/worktree live panel · status actions · `e` edit) → Task 6. ✓
- Form (same fields as WebUI; whitelist color/glyph pickers) → Task 7. ✓
- New `gitworktree` + `clientcheckout` + checkout auto-record in `projectresolve` → Tasks 1–3. ✓
- Mounted as a shell tab + deep-link → Task 8. ✓
- Read-only worktree panel (decided) → Task 6 renders a list only, no cursor/actions. ✓
- TUI whitelist consumed via `theme` (the spec's "consumes domain.ProjectColors/ProjectGlyphs via theme") → Task 4 `kindcolor.ProjectColor` + Task 7 pickers iterate the domain whitelists. ✓

**2. Placeholder scan:** Load-bearing Go is complete and verbatim for the mechanical packages (gitworktree, clientcheckout, projectresolve hook, kindcolor, mount wiring, the compose/parse logic, all interfaces + tests). The three bubbletea route Views (`View` bodies in Tasks 5–7) are described by exact section/data contract + "mirror <named file>" rather than full lipgloss source — the same altitude the Slice-2 plan used for templ, justified because the design system styling is established and the data bindings (the part that can be wrong) ARE fully specified. Each route's Update/Init/data-flow + every apiclient call is concrete. Implementer-verify points are flagged inline (listnav API names, WorkSession accessors, bubbles/v2 textinput path, theme.Default name, markdown.Render options) — each names the file to confirm against.

**3. Type consistency:** `ProjectsAPI`/`DetailAPI`/`FormAPI` defined in Tasks 5–7 and satisfied by `*apiclient.Client` (asserted in Task 8); `UpdateFields` defined in Task 6 and reused by Task 7's compose + the fakes; `FormValues` defined in Task 7; `kindcolor.ProjectColor(name, pal)` produced in Task 4, consumed in Tasks 5–7; `gitworktree.Worktree`/`List`/`Root` produced in Task 1, consumed in Tasks 3 + 6; `clientcheckout.Load`/`Get`/`Record` produced in Task 2, consumed in Tasks 3 + 6; navigation via `shell.PushRouteMsg`/`PopRouteMsg` throughout; `Mount`/`MountWithAPI` (Task 8) returns the list `*Route` with factories set. Tab title `"Projekte"` matches `tabIndexForArg` + `SwitchTabMsg`.

**Known limitations (carried/explicit):**
- Worktime aggregate pulls a 5-year session range and sums client-side (no per-project backend usecase) — same approach as the WebUI cockpit; acceptable for single-user scale. Running sessions excluded from Σ (settled-time summary), matching the WebUI cockpit.
- `projectresolve` records the *current* worktree's toplevel (not necessarily the main worktree); `git worktree list` from any worktree still enumerates all, so the panel is complete regardless. Acceptable.
- Checkout auto-record is a side-effect tested via the `recordCheckout` seam (unit), not end-to-end against the real config dir (matches how `clientmachine.Load()` is already used un-mocked in `projectresolve`).

**Deferred to later slices (not this plan):** CLI `project create/list/show/edit/pause/resume/archive/worktrees` (Slice 4); TUI session-edit project picker (Slice 5); worktree comfort actions (copy-path / open-in-$EDITOR) and ahead/behind (v2).
