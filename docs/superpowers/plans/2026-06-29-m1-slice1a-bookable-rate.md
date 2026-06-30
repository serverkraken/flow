# M1 Slice 1a — Buchbare Nodes + Rate-Vererbung — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Worktime kann auf *jeden* buchbaren Node (Engagement/Vorhaben/Repo) gebucht werden, und der Stundensatz wird über die Ahnenkette vererbt (nächster Vorfahre mit Rate gewinnt).

**Architecture:** Reine Backend-/Domain-Änderung, keine neuen Konstruktoren. Zwei neue pure Domain-Helfer (`IsBookable`, `ResolveRate`); der Buchungs-Guard und der Rate-Guard werden von „nur Engagement" auf „jeder buchbare Node" relaxt; der Export löst den Satz über die bereits existierende `NodeStore.Ancestors`-Kette auf. Hosts (WebUI/TUI-Picker) bleiben unangetastet — sie adoptieren die Fähigkeit in späteren Slices.

**Tech Stack:** Go; hexagonale Architektur (domain / usecase / ports / adapter); Tests mit `internal/testutil`-Fakes; `errors.Is` für Domain-Fehler; goose-Migrationen (hier keine nötig).

## Global Constraints
- Sprache Go; bestehende Patterns folgen (kleine fokussierte Dateien, Memory `feedback_no_monoliths`).
- **Buchbar = Engagement, Vorhaben, Repo. Branch ist NICHT buchbar** (B1: reserviert).
- Domain-Fehler über `errors.Is` prüfen; Sentinel `domain.ErrInvalidNode` für falsche Kind.
- Keine Host-Picker-Änderungen in dieser Slice (WebUI/TUI gating = spätere Slices).
- Alle Befehle vom Repo-Root `/Users/msoent/SourceCode/serverkraken/flow-m1`.
- Abschluss: `make ci` grün; Live-Smoke gegen Dev-Stack (Postgres+Dex).

---

## File Structure
- `internal/domain/node.go` — **modify**: + `IsBookable`, + `ResolveRate`.
- `internal/domain/node_test.go` — **modify**: + `TestIsBookable`, + `TestResolveRate`.
- `internal/usecase/engagement_guard.go` — **modify**: `requireEngagement` → `requireBookable`.
- `internal/usecase/start_session.go` — **modify** (line 24): Aufruf umbenennen.
- `internal/usecase/add_session.go` — **modify** (line 28): Aufruf umbenennen.
- `internal/usecase/stop_session.go` — **modify** (line 44): Inline-Guard relaxen.
- `internal/usecase/set_node_rate.go` — **modify** (line 30): Rate-Guard relaxen.
- `internal/usecase/export.go` — **modify** (line 85-93): Rate über Ahnenkette auflösen.
- `internal/usecase/start_session_test.go` — **modify**: Repo-Test flippen + Branch-Test.
- `internal/usecase/add_session_test.go` — **modify**: Repo-Test flippen.
- `internal/usecase/stop_session_test.go` — **create**: Repo-Accept-Test.
- `internal/usecase/set_node_rate_test.go` — **modify**: Repo-Accept-Test (+ ggf. alten Reject-Test flippen).
- `internal/usecase/export_test.go` — **modify**: Rate-Vererbungs-Test.
- `internal/adapter/httpserver/worktime.go` — **modify** (lines 49,69,100): Fehlertexte.

---

### Task 1: `domain.IsBookable`

**Files:**
- Modify: `internal/domain/node.go` (nach `AllowedChildKind`, ~Zeile 102)
- Test: `internal/domain/node_test.go`

**Interfaces:**
- Produces: `func IsBookable(k NodeKind) bool`

- [ ] **Step 1: Write the failing test** — in `internal/domain/node_test.go` anhängen:

```go
func TestIsBookable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind domain.NodeKind
		want bool
	}{
		{domain.KindEngagement, true},
		{domain.KindVorhaben, true},
		{domain.KindRepo, true},
		{domain.KindBranch, false},
		{domain.NodeKind("unknown"), false},
	}
	for _, c := range cases {
		if got := domain.IsBookable(c.kind); got != c.want {
			t.Errorf("IsBookable(%s)=%v want %v", c.kind, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestIsBookable`
Expected: FAIL — `undefined: domain.IsBookable`

- [ ] **Step 3: Implement** — in `internal/domain/node.go` nach `AllowedChildKind` einfügen:

```go
// IsBookable reports whether worktime may be booked to a node of this kind.
// Engagement, Vorhaben and Repo are bookable; Branch is reserved (B1) and not.
func IsBookable(k NodeKind) bool {
	return k == KindEngagement || k == KindVorhaben || k == KindRepo
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/ -run TestIsBookable`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/node.go internal/domain/node_test.go
git commit -m "feat(worktime): domain.IsBookable (engagement/vorhaben/repo)"
```

---

### Task 2: `requireBookable` (Start/Add) + REST-Fehlertexte

**Files:**
- Modify: `internal/usecase/engagement_guard.go`
- Modify: `internal/usecase/start_session.go:24`, `internal/usecase/add_session.go:28`
- Modify: `internal/usecase/start_session_test.go`, `internal/usecase/add_session_test.go`
- Modify: `internal/adapter/httpserver/worktime.go:49,69,100`

**Interfaces:**
- Consumes: `domain.IsBookable` (Task 1)
- Produces: `func requireBookable(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error`

- [ ] **Step 1: Flip the failing tests.** In `internal/usecase/start_session_test.go` `TestStartSession_RepoRejected` (Z.69-78) ersetzen durch:

```go
func TestStartSession_RepoAccepted(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	repo := "repo1"
	got, err := uc.Execute(context.Background(), "u1", &repo, nil, "")
	if err != nil {
		t.Fatalf("start on repo: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "repo1" {
		t.Errorf("want NodeID repo1, got %v", got.NodeID)
	}
}

func TestStartSession_BranchRejected(t *testing.T) {
	t.Parallel()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	repoParent := "repo1"
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: "br1", OwnerID: "u1", ParentID: &repoParent, Kind: domain.KindBranch,
		Name: "br1", Slug: "br1", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
	br := "br1"
	if _, err := uc.Execute(context.Background(), "u1", &br, nil, ""); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("want ErrInvalidNode for branch node, got %v", err)
	}
}
```

In `internal/usecase/add_session_test.go` `TestAddSession_RepoRejected` (Z.40-52) ersetzen durch:

```go
func TestAddSession_RepoAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	uc := newAddSession(ss, ns, time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	repo := "repo1"
	got, err := uc.Execute(ctx, "u1", &repo, start, stop, nil, "")
	if err != nil {
		t.Fatalf("add on repo: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "repo1" {
		t.Errorf("want NodeID repo1, got %v", got.NodeID)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/usecase/ -run 'TestStartSession_RepoAccepted|TestStartSession_BranchRejected|TestAddSession_RepoAccepted'`
Expected: FAIL — `TestStartSession_RepoAccepted` / `TestAddSession_RepoAccepted` bekommen `ErrInvalidNode` statt Erfolg.

- [ ] **Step 3: Implement.** In `internal/usecase/engagement_guard.go` die Funktion ersetzen:

```go
// requireBookable verifies that nodeID (when set) names a bookable node
// (engagement, vorhaben or repo). A nil/empty nodeID is allowed (unbooked start).
func requireBookable(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error {
	if nodeID == nil || *nodeID == "" {
		return nil
	}
	n, err := nodes.Get(ctx, ownerID, *nodeID)
	if err != nil {
		return err
	}
	if !domain.IsBookable(n.Kind) {
		return fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
	}
	return nil
}
```

In `internal/usecase/start_session.go:24` und `internal/usecase/add_session.go:28` jeweils `requireEngagement(` → `requireBookable(`.

In `internal/adapter/httpserver/worktime.go` die drei Literale (Z.49, 69, 100)
`"worktime can only be booked to an engagement"` →
`"worktime can only be booked to a bookable node (engagement, vorhaben or repo)"`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/usecase/ ./internal/adapter/httpserver/`
Expected: PASS (alle).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/engagement_guard.go internal/usecase/start_session.go internal/usecase/add_session.go internal/usecase/start_session_test.go internal/usecase/add_session_test.go internal/adapter/httpserver/worktime.go
git commit -m "feat(worktime): start/add book to any bookable node (requireBookable)"
```

---

### Task 3: Stop-Session bucht auf jeden buchbaren Node

**Files:**
- Modify: `internal/usecase/stop_session.go:44`
- Create: `internal/usecase/stop_session_test.go`

**Interfaces:**
- Consumes: `domain.IsBookable` (Task 1)

- [ ] **Step 1: Write the failing test** — neue Datei `internal/usecase/stop_session_test.go`:

```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStopSession_RepoAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	start := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start}); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	uc := usecase.StopSession{
		Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: start.Add(time.Hour)}, Loc: time.UTC,
	}
	repo := "repo1"
	got, err := uc.Execute(ctx, "u1", "s1", &repo)
	if err != nil {
		t.Fatalf("stop on repo: %v", err)
	}
	if got.NodeID == nil || *got.NodeID != "repo1" {
		t.Errorf("want NodeID repo1, got %v", got.NodeID)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestStopSession_RepoAccepted`
Expected: FAIL — `ErrInvalidNode` (Stop verlangt heute Engagement).

- [ ] **Step 3: Implement** — `internal/usecase/stop_session.go:44` ersetzen:

```go
	if !domain.IsBookable(n.Kind) {
		return domain.WorkSession{}, fmt.Errorf("%w: worktime books to a bookable node, got %s", domain.ErrInvalidNode, n.Kind)
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run TestStopSession`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/stop_session.go internal/usecase/stop_session_test.go
git commit -m "feat(worktime): stop books to any bookable node"
```

---

### Task 4: `domain.ResolveRate` (Ahnenketten-Vererbung)

**Files:**
- Modify: `internal/domain/node.go` (nach `ResolveEngagement`)
- Test: `internal/domain/node_test.go`

**Interfaces:**
- Consumes: `NodeStore.Ancestors` liefert die Kette leaf→root.
- Produces: `func ResolveRate(chain []Node) *Money`

- [ ] **Step 1: Write the failing test** — in `internal/domain/node_test.go` anhängen:

```go
func TestResolveRate(t *testing.T) {
	t.Parallel()
	eur := func(a int64) *domain.Money { m := domain.Money{Amount: a, Currency: "EUR"}; return &m }
	p := "eng"
	chain := []domain.Node{
		{ID: "repo", Kind: domain.KindRepo, ParentID: &p},
		{ID: "eng", Kind: domain.KindEngagement, Rate: eur(9500)},
	}
	if got := domain.ResolveRate(chain); got == nil || got.Amount != 9500 {
		t.Fatalf("want inherited 9500, got %+v", got)
	}
	chain[0].Rate = eur(12000) // nearer wins
	if got := domain.ResolveRate(chain); got == nil || got.Amount != 12000 {
		t.Fatalf("want nearer 12000, got %+v", got)
	}
	if got := domain.ResolveRate([]domain.Node{{ID: "repo", Kind: domain.KindRepo}}); got != nil {
		t.Fatalf("want nil (no rate in chain), got %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/domain/ -run TestResolveRate`
Expected: FAIL — `undefined: domain.ResolveRate`

- [ ] **Step 3: Implement** — in `internal/domain/node.go` nach `ResolveEngagement` einfügen:

```go
// ResolveRate returns the effective rate for a node by walking its ancestor
// chain (leaf→root, as returned by NodeStore.Ancestors): the nearest node that
// carries a rate wins. Returns nil when no node in the chain has a rate.
func ResolveRate(chain []Node) *Money {
	for _, n := range chain {
		if n.Rate != nil {
			return n.Rate
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/domain/ -run TestResolveRate`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/node.go internal/domain/node_test.go
git commit -m "feat(worktime): domain.ResolveRate walks ancestor chain"
```

---

### Task 5: Rate auf jedem buchbaren Node setzbar

**Files:**
- Modify: `internal/usecase/set_node_rate.go:11-13,18-19,30`
- Modify: `internal/usecase/set_node_rate_test.go`

**Interfaces:**
- Consumes: `domain.IsBookable` (Task 1)

- [ ] **Step 1: Write the failing test** — in `internal/usecase/set_node_rate_test.go` anhängen:

```go
func TestSetNodeRate_RepoAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	parent := "eng"
	if _, err := ns.Create(ctx, domain.Node{
		ID: "repo1", OwnerID: "u1", ParentID: &parent, Kind: domain.KindRepo,
		Name: "repo1", Slug: "repo1", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	uc := usecase.SetNodeRate{Nodes: ns}
	rate := domain.Money{Amount: 12000, Currency: "EUR"}
	if err := uc.Execute(ctx, "u1", "repo1", &rate); err != nil {
		t.Fatalf("set rate on repo: %v", err)
	}
	got, _ := ns.Get(ctx, "u1", "repo1")
	if got.Rate == nil || got.Rate.Amount != 12000 {
		t.Errorf("want rate 12000 on repo, got %+v", got.Rate)
	}
}
```

Außerdem `internal/usecase/set_node_rate_test.go` öffnen und einen evtl. vorhandenen Test, der einen Nicht-Engagement-Node beim Rate-Setzen mit `ErrInvalidNode` ablehnt, **entfernen oder** auf Erfolg umstellen (er widerspricht sonst dem neuen Verhalten).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestSetNodeRate_RepoAccepted`
Expected: FAIL — `ErrInvalidNode` (heute nur Engagement).

- [ ] **Step 3: Implement** — `internal/usecase/set_node_rate.go:30` ersetzen:

```go
	if !domain.IsBookable(n.Kind) {
		return fmt.Errorf("%w: only a bookable node may carry a rate, got %s", domain.ErrInvalidNode, n.Kind)
	}
```

Und den Doc-Kommentar (Z.11-13 + 18-19) von „Only engagement nodes may carry a rate" auf „Only bookable nodes (engagement/vorhaben/repo) may carry a rate" anpassen.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run TestSetNodeRate`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/set_node_rate.go internal/usecase/set_node_rate_test.go
git commit -m "feat(worktime): any bookable node may carry a rate"
```

---

### Task 6: Export löst den Satz über die Ahnenkette auf

**Files:**
- Modify: `internal/usecase/export.go:85-93`
- Modify: `internal/usecase/export_test.go`

**Interfaces:**
- Consumes: `NodeStore.Ancestors`, `domain.ResolveRate` (Task 4)

- [ ] **Step 1: Write the failing test** — in `internal/usecase/export_test.go` anhängen:

```go
func TestBuildExport_RateInheritedFromAncestor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
	rate := domain.Money{Amount: 9500, Currency: "EUR"}
	if _, err := ns.Create(ctx, domain.Node{
		ID: "eng", OwnerID: "u1", Kind: domain.KindEngagement,
		Name: "Kunde A", Slug: "kunde-a", Status: domain.NodeActive, Rate: &rate,
	}); err != nil {
		t.Fatalf("seed eng: %v", err)
	}
	parent := "eng"
	if _, err := ns.Create(ctx, domain.Node{
		ID: "repo", OwnerID: "u1", ParentID: &parent, Kind: domain.KindRepo,
		Name: "repo-y", Slug: "repo-y", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC) // 2h on the repo
	repo := "repo"
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", NodeID: &repo, Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	uc := usecase.BuildExport{Sessions: ss, Nodes: ns, Clock: testutil.FakeClock{T: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)}, Loc: time.UTC}
	data, err := uc.Execute(ctx, "u1", start, stop, "")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data.ByEngagement) != 1 {
		t.Fatalf("want 1 node total, got %d", len(data.ByEngagement))
	}
	nt := data.ByEngagement[0]
	if nt.Rate == nil || nt.Rate.Amount != 9500 {
		t.Fatalf("want inherited rate 9500, got %+v", nt.Rate)
	}
	if nt.Amount == nil || nt.Amount.Amount != 19000 { // 9500/h * 2h
		t.Fatalf("want amount 19000, got %+v", nt.Amount)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/usecase/ -run TestBuildExport_RateInheritedFromAncestor`
Expected: FAIL — `nt.Rate` ist nil (Repo hat keinen eigenen Satz; heute kein Walk).

- [ ] **Step 3: Implement** — in `internal/usecase/export.go` den `if !ok`-Block (Z.86-93) ersetzen:

```go
		if !ok {
			chain, aerr := uc.Nodes.Ancestors(ctx, ownerID, *s.NodeID)
			if aerr != nil {
				return domain.ExportData{}, aerr
			}
			t = &domain.NodeTotal{
				NodeID:   *s.NodeID,
				NodeName: name,
				Rate:     domain.ResolveRate(chain),
			}
			totals[*s.NodeID] = t
		}
```

(`name` bleibt aus `byID[*s.NodeID]`; nur die `Rate` kommt jetzt aus `ResolveRate(chain)`.)

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/usecase/ -run TestBuildExport`
Expected: PASS (alle Export-Tests).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/export.go internal/usecase/export_test.go
git commit -m "feat(export): resolve hourly rate via ancestor chain"
```

---

### Task 7: Verifikation (Build, CI, Live-Smoke)

**Files:** keine.

- [ ] **Step 1: Full build + vet**

Run: `go build ./... && go vet ./...`
Expected: kein Fehler.

- [ ] **Step 2: `make ci`**

Run: `make ci`
Expected: grün (Lint + Tests + Coverage-Gate). Bei `NO_COLOR`-Konflikt mit TUI-Markdown-Tests: `env -u NO_COLOR make ci`.

- [ ] **Step 3: Live-Smoke gegen Dev-Stack** (Postgres+Dex). Stack hoch (`make dev-up`), Token holen (`make dev-token`), Server (`make dev-run`). Dann:

```bash
# Annahme: $TOKEN gesetzt (make dev-token), Engagement "Kunde A" mit Repo-Kind-Kind "repo-y" existiert oder via API anlegen.
# 1) Start (ohne Projekt) → 2) Stop auf einen REPO-Node muss jetzt 200 liefern (vorher 400).
curl -fsS -X POST localhost:8080/api/v1/sessions/start -H "Authorization: Bearer $TOKEN"
curl -fsS -X POST localhost:8080/api/v1/sessions/stop  -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' -d '{"projectId":"<REPO_NODE_ID>"}'
# erwartet: 200 + Session mit nodeId=<REPO_NODE_ID>
```

Verifizieren: Stop auf einen **Repo**-Node liefert **200** (nicht „can only be booked to an engagement"). Optional: Repo ohne eigenen Satz, Engagement mit Satz → `GET /api/v1/export?...` zeigt den **vererbten** Satz in der Repo-Zeile.

- [ ] **Step 4: Commit (nur falls Smoke-Fixups nötig)** — sonst überspringen.

---

## Self-Review (durchgeführt)

**1. Spec-Coverage (Slice-1 Parts a+b):** (a) buchbar = Tasks 1-3 (Domain + Start/Add/Stop + REST-Texte). (b) Rate-Vererbung = Tasks 4-6 (ResolveRate + Rate-Guard relax + Export-Walk). Parts c (Subtree-Rollup) + d (Job/Privat-Soll) sind **bewusst** in Plan 1b. ✔
**2. Placeholder-Scan:** Jeder Code-Step zeigt echten Code; Test-Flips zeigen den exakten Ziel-Code; einziger „lies & prüfe"-Schritt (Task 5, evtl. alter Reject-Test) nennt Datei + exakte Assertion. ✔
**3. Typ-Konsistenz:** `IsBookable`/`ResolveRate`-Signaturen identisch über alle Consumer; `requireBookable` ersetzt `requireEngagement` an beiden Aufrufstellen; `NodeTotal.Rate/Amount`, `Money.Amount` wie in `export.go` verwendet. ✔

---

## Execution Handoff

Nach Plan-Abnahme folgt **Plan 1b** (Subtree-Rollup `NodeStore.Subtree` + `StatsComputer.NodeStats` + `countsTowardTarget` + Soll-Scoping) — er braucht zusätzlich `internal/domain/records.go`, `aggregate.go`, `stats.go`, `stats_computer.go` (lese ich beim Schreiben von 1b).
