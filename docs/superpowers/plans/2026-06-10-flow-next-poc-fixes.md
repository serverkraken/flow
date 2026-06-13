# flow next-Branch PoC-Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Den `next`-Branch in einen nutzbaren Single-User-PoC-Zustand bringen: TUI-Korruption durch Auth-/Sync-Logs beheben, Worktime-Lifecycle (start/stop/toggle/pause/resume/correct) auf dem neuen sqlite-ActiveSessions-Pfad vereinheitlichen, Sync-Logikfehler (Version-Writeback, Tag/Note-Verlust, started_at) fixen, Auth-Lifecycle (Refresh, leise Offline-Behandlung) komplettieren, WebUI-SSE-Auth reparieren.

**Architecture:** Hexagonal (domain/ports/usecase/adapter/frontend). Der neue sqlite-Pfad (`usecase.ActiveSessions` + `httpsync.Worker` + write_queue) wird die einzige Lifecycle-Autorität; `flockstate` behält nur den Pause-Marker (per-device, nie gesynct). `SessionWriter` behält manuelle Edits/Tag/Note. Alle Hintergrund-Logs gehen via `slog.SetDefault` in eine Datei statt auf stderr.

**Tech Stack:** Go 1.24+, bubbletea v2 (charm.land), cobra, SQLite (modernc), chi (Server), templ (WebUI), OIDC Device-Flow (Authentik).

**Arbeitsverzeichnis:** `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1` (Branch `next`). Alle Pfade relativ dazu. Vor Beginn: `git status` muss clean sein.

**Werkzeug-Regeln:** `find`/`grep`/`tree` sind per Hook geblockt — `fd`/`rg` benutzen. Nach jedem Task: betroffene Packages mit `go test ./<pkg>/...` testen, dann committen. `gofumpt -w <files>` vor jedem Commit (golangci enforced).

**Review-Findings → Tasks:** F1 slog→Task 1 · F12 Reader-Scoping→Task 2 · F8 Tag/Note→Task 3 · started_at-Drift→Task 4 · F7 Version/409→Tasks 5+6 · F6 stop→Task 7 · F5 toggle→Task 8 · pause/resume→Task 9 · correct→Task 10 · F4 TUI→Task 11 · F3 Migration-Guard→Task 12 · F9/F11 Identity→Task 13 · F2 Refresh→Tasks 14+15 · F10 SSE→Task 16 · Wiring-Verification→Task 17.

---

### Task 1: slog auf Datei umleiten (TUI-Korruption stoppen)

Der httpsync-Worker loggt via Default-slog auf stderr, während bubbletea das Terminal besitzt (`internal/adapter/httpsync/worker.go:179ff`). `cmd/flow/main.go` setzt nie `slog.SetDefault`.

**Files:**
- Create: `cmd/flow/logging.go`
- Create: `cmd/flow/logging_test.go`
- Modify: `cmd/flow/main.go` (in `main()`, vor `buildDeps`)

- [ ] **Step 1: Failing Test schreiben**

```go
// cmd/flow/logging_test.go
package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupLoggingWritesToFile(t *testing.T) {
	dir := t.TempDir()
	closeFn := setupLogging(dir, "warn")
	defer closeFn()

	slog.Warn("test-marker-warn")
	slog.Debug("test-marker-debug")
	closeFn()

	data, err := os.ReadFile(filepath.Join(dir, "flow.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "test-marker-warn") {
		t.Errorf("warn line missing in log file: %q", data)
	}
	if strings.Contains(string(data), "test-marker-debug") {
		t.Errorf("debug line should be filtered at warn level: %q", data)
	}
}

func TestSetupLoggingDebugLevel(t *testing.T) {
	dir := t.TempDir()
	closeFn := setupLogging(dir, "debug")
	defer closeFn()
	slog.Debug("dbg-marker")
	closeFn()
	data, _ := os.ReadFile(filepath.Join(dir, "flow.log"))
	if !strings.Contains(string(data), "dbg-marker") {
		t.Errorf("debug line missing with FLOW_LOG_LEVEL=debug: %q", data)
	}
}
```

- [ ] **Step 2: Test laufen lassen — muss fehlschlagen**

Run: `go test ./cmd/flow/ -run TestSetupLogging -v`
Expected: FAIL `undefined: setupLogging`

- [ ] **Step 3: Implementierung**

```go
// cmd/flow/logging.go
package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// setupLogging routes the process-global slog default handler into
// <stateDir>/flow.log. The flow TUI owns the terminal via bubbletea —
// any stderr write from a background goroutine (httpsync worker, keyring
// warnings) would corrupt the alternate-screen render, so NOTHING may log
// to stderr after this call. Falls back to io.Discard when the state dir
// is not writable. Returns a close func for the log file (call via defer).
func setupLogging(stateDir, level string) func() {
	var w io.Writer = io.Discard
	closeFn := func() {}
	if err := os.MkdirAll(stateDir, 0o755); err == nil {
		f, ferr := os.OpenFile(filepath.Join(stateDir, "flow.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr == nil {
			w = f
			closeFn = func() { _ = f.Close() }
		}
	}
	lvl := slog.LevelWarn
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})))
	return closeFn
}
```

- [ ] **Step 4: Test laufen lassen — muss passen**

Run: `go test ./cmd/flow/ -run TestSetupLogging -v`
Expected: PASS

- [ ] **Step 5: In `main()` verdrahten**

In `cmd/flow/main.go`, Funktion `main()`, direkt NACH der `xdgDataHome`-Auflösung (Zeile ~618) und VOR dem `env := Env{...}`-Block einfügen:

```go
	logClose := setupLogging(
		filepath.Join(home, ".local", "state", "flow"),
		os.Getenv("FLOW_LOG_LEVEL"),
	)
	defer logClose()
```

- [ ] **Step 6: Build + bestehende Tests**

Run: `go build ./cmd/flow/ && go test ./cmd/flow/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/flow/logging.go cmd/flow/logging_test.go cmd/flow/main.go
git commit -m "fix(flow): route default slog to state-dir file — stderr writes corrupted the bubbletea TUI"
```

---

### Task 2: WorktimeReader user-scopen + ActiveSessions als Running-Quelle

Zwei Bugs in `internal/usecase/worktime_reader.go`: (a) alle `Sessions.Load("")`-Aufrufe übergeben leere userID gegen den user-gescopten SQLite-Store → Today/Week/History sind IMMER leer; (b) `State.GetActive()` liest den flockstate-Marker, den der neue Start-Pfad nie setzt → Running-Indikator zeigt nie etwas an.

**Files:**
- Modify: `internal/usecase/worktime_reader.go`
- Modify: `internal/usecase/stats_computer.go` (gleiche GetActive-Quelle)
- Modify: `cmd/flow/main.go:270` (Reader-Wiring)
- Test: `internal/usecase/worktime_reader_user_scope_test.go` (neu)

- [ ] **Step 1: Failing Test schreiben**

Vorher Fakes prüfen: `rg -l "ActiveSessionStore" internal/testutil/` (Fake-Store existiert) und bestehende Reader-Tests als Konstruktions-Vorlage: `rg -l "WorktimeReader{" internal/usecase/`. Test:

```go
// internal/usecase/worktime_reader_user_scope_test.go
package usecase

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// userScopedSessions fails the test when Load is called with the wrong userID.
type userScopedSessions struct {
	wantUser string
	rows     []domain.Session
	t        *testing.T
}

func (s *userScopedSessions) Load(userID string) ([]domain.Session, error) {
	if userID != s.wantUser {
		s.t.Errorf("Load called with userID %q, want %q", userID, s.wantUser)
	}
	return s.rows, nil
}
func (s *userScopedSessions) LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, _ := s.Load(userID)
	var out []domain.Session
	for _, r := range all {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *userScopedSessions) Upsert(domain.Session) error        { return nil }
func (s *userScopedSessions) UpsertBatch([]domain.Session) error { return nil }
func (s *userScopedSessions) Delete(userID, id string) error     { return nil }

type fakeActiveList struct{ rows []domain.ActiveSession }

func (f *fakeActiveList) ListByUser(string) ([]domain.ActiveSession, error) { return f.rows, nil }
func (f *fakeActiveList) Get(string, string) (domain.ActiveSession, error) {
	return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
}
func (f *fakeActiveList) Upsert(domain.ActiveSession) error { return nil }
func (f *fakeActiveList) Delete(string, string) error       { return nil }

type fakeIdleState struct{}

func (fakeIdleState) GetActive() (*time.Time, error) { return nil, nil }
func (fakeIdleState) SetActive(time.Time) error      { return nil }
func (fakeIdleState) ClearActive() error             { return nil }
func (fakeIdleState) GetPause() (*time.Time, error)  { return nil, nil }
func (fakeIdleState) SetPause(time.Time) error       { return nil }
func (fakeIdleState) ClearPause() error              { return nil }

func TestReaderTodayIsUserScopedAndReadsActiveStore(t *testing.T) {
	now := time.Date(2026, 6, 10, 14, 0, 0, 0, time.Local)
	started := now.Add(-25 * time.Minute).UTC()
	sessions := &userScopedSessions{
		wantUser: "user-1",
		t:        t,
		rows: []domain.Session{{
			ID: "s1", UserID: "user-1", ProjectID: "p1",
			Date:    time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local),
			Elapsed: time.Hour,
		}},
	}
	r := &WorktimeReader{
		Sessions: sessions,
		State:    fakeIdleState{},
		Active:   &fakeActiveList{rows: []domain.ActiveSession{{UserID: "user-1", ProjectID: "p1", StartedAt: started}}},
		UserID:   "user-1",
		Targets:  &TargetResolver{DefaultTarget: 8 * time.Hour},
		Clock:    fixedClockAt(now),
	}
	day, err := r.Today()
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if len(day.Sessions) != 1 {
		t.Errorf("want 1 session, got %d (userID scoping broken)", len(day.Sessions))
	}
	if day.Active == nil {
		t.Fatal("day.Active is nil — Active store not consulted")
	}
	if !day.Active.Equal(started) {
		t.Errorf("day.Active = %v, want %v", *day.Active, started)
	}
}
```

`fixedClockAt`: vorhandenen Fake-Clock-Helper im Package suchen (`rg -n "Now\(\) time.Time" internal/usecase/*_test.go internal/testutil/*.go | head`) und verwenden bzw. lokal definieren. Falls `TargetResolver` mit nil-Feldern panict: minimalen Fake-Config-Reader aus bestehenden Reader-Tests übernehmen.

- [ ] **Step 2: Test laufen lassen — muss fehlschlagen**

Run: `go test ./internal/usecase/ -run TestReaderTodayIsUserScoped -v`
Expected: FAIL `unknown field Active` / `unknown field UserID`

- [ ] **Step 3: Reader umbauen**

In `internal/usecase/worktime_reader.go` Struct erweitern + neue Methode:

```go
type WorktimeReader struct {
	Sessions ports.SessionStore
	State    ports.LegacyActiveStore
	Targets  *TargetResolver
	Clock    ports.Clock

	// Active is the sqlite-backed multi-device active-session store. When
	// non-nil (production wiring) it is the source of truth for the
	// running indicator; State then only serves the pause marker. Nil in
	// legacy tests → State.GetActive() keeps working.
	Active ports.ActiveSessionStore
	// UserID scopes every Sessions/Active call. The sqlite stores filter
	// WHERE user_id = ? — an empty UserID returns zero rows.
	UserID string

	ShowWeekend bool
}

// ActiveStart returns the start time of the running session: earliest
// ActiveSession row when the sqlite store is wired, else the legacy
// flockstate marker. Exported so StatsComputer shares the same source.
func (r *WorktimeReader) ActiveStart() (*time.Time, error) {
	if r.Active == nil {
		return r.State.GetActive()
	}
	list, err := r.Active.ListByUser(r.UserID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	earliest := list[0].StartedAt
	for _, a := range list[1:] {
		if a.StartedAt.Before(earliest) {
			earliest = a.StartedAt
		}
	}
	// Stored UTC; downstream formats wall-clock times (15:04) → Local.
	earliest = earliest.Local()
	return &earliest, nil
}
```

Dann ALLE Aufrufstellen umstellen (`rg -n 'State\.GetActive|Load\(""\)|LoadFiltered\(""' internal/usecase/worktime_reader.go`):
- `Today()`: `r.State.GetActive()` → `r.ActiveStart()`; `LoadFiltered("", ...)` → `LoadFiltered(r.UserID, ...)`
- `Week()`: `r.State.GetActive()` → `r.ActiveStart()`; `LoadFiltered("", ...)` → `LoadFiltered(r.UserID, ...)`
- `History()`: `Load("")` → `Load(r.UserID)`
- `Range()`: beide Stellen `""` → `r.UserID`
- `SessionsOverlap()`: `LoadFiltered("", ...)` → `LoadFiltered(r.UserID, ...)`

- [ ] **Step 4: StatsComputer auf dieselbe Quelle**

`rg -n 'State\.' internal/usecase/stats_computer.go` — jede `GetActive()`-Stelle durch `<receiver>.Reader.ActiveStart()` ersetzen (`GetPause`-Aufrufe bleiben auf State). Wenn stats_computer kein `GetActive` nutzt: nichts tun.

- [ ] **Step 5: Wiring in main.go**

`cmd/flow/main.go:270` ändern zu:

```go
	reader := &usecase.WorktimeReader{
		Sessions: sessionStore,
		State:    activeStore,
		Active:   cacheActiveSessions,
		UserID:   localUser.ID,
		Targets:  targets,
		Clock:    clock,
	}
```

(`cacheActiveSessions` ist Zeile ~178 definiert, vor dem Reader — Reihenfolge passt.)

- [ ] **Step 6: Tests laufen lassen**

Run: `go test ./internal/usecase/ ./cmd/flow/`
Expected: PASS. Bestehende Reader-Tests ohne `Active`/`UserID` bleiben auf dem Legacy-Pfad (nil Active) und müssen grün bleiben.

- [ ] **Step 7: Commit**

```bash
git add internal/usecase/worktime_reader.go internal/usecase/stats_computer.go internal/usecase/worktime_reader_user_scope_test.go cmd/flow/main.go
git commit -m "fix(usecase): scope WorktimeReader by UserID and read running state from sqlite ActiveSessions — views were empty and the running indicator dead on the new path"
```

---

### Task 3: Stop-Payload mit Tag/Note (+Version) statt handgebautem JSON

`internal/usecase/active_sessions.go:161` baut `{"action":"stop","version":N}` per String-Konkatenation — `drainActiveStop` (worker.go:491) liest aber `body.Tag`/`body.Note` und schickt damit immer Leerstrings zum Server; der Server schreibt die finale Session ohne Tag/Note, der nächste Pull überschreibt die korrekten lokalen Werte mit leer.

**Files:**
- Modify: `internal/usecase/active_sessions.go`
- Test: `internal/usecase/active_sessions_test.go` (erweitern)

- [ ] **Step 1: Failing Test schreiben**

Bestehende Stop-Tests in `internal/usecase/active_sessions_test.go` nutzen Fake-Stores + Fake-WriteQueue (`internal/testutil/write_queue.go`), die enqueued Entries exponieren. Nach deren Muster: User+Projekt anlegen, `Start(user, project, "deep", "n1")`, dann `Stop(user, project, "", "")` (leere Args → erben vom Start-Row). Den `active_sessions_stop`-Entry aus der Fake-Queue holen und asserten:

```go
	var body struct {
		Action  string `json:"action"`
		Version int64  `json:"version"`
		Tag     string `json:"tag"`
		Note    string `json:"note"`
	}
	if err := json.Unmarshal(stopEntry.Payload, &body); err != nil {
		t.Fatalf("unmarshal stop payload: %v", err)
	}
	if body.Action != "stop" {
		t.Errorf("action = %q, want stop", body.Action)
	}
	if body.Tag != "deep" || body.Note != "n1" {
		t.Errorf("stop payload lost tag/note: tag=%q note=%q", body.Tag, body.Note)
	}
```

- [ ] **Step 2: Test laufen lassen — muss fehlschlagen**

Run: `go test ./internal/usecase/ -run TestStopPayload -v`
Expected: FAIL (Tag/Note fehlen im Payload)

- [ ] **Step 3: Implementierung**

In `internal/usecase/active_sessions.go` den Stop-Payload-Block (Zeile ~160-162) ersetzen:

```go
	// Queue active-stop signal with the known server version for If-Match.
	// Tag/Note travel in the payload — drainActiveStop forwards them so the
	// server's canonical finished Session keeps them even when the stopping
	// device differs (and so the next pull can't blank them out locally).
	stopPayload, encErr := json.Marshal(struct {
		Action  string `json:"action"`
		Version int64  `json:"version"`
		Tag     string `json:"tag"`
		Note    string `json:"note"`
	}{"stop", cur.Version, tag, note})
	if encErr == nil {
		_, _ = a.queue.Enqueue("active_sessions_stop", projectID, stopPayload, cur.Version)
	}
	a.signalPush()
```

`strconv`-Import entfernen, falls dadurch ungenutzt (`go build ./internal/usecase/` zeigt es).

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/usecase/ ./internal/adapter/httpsync/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/active_sessions.go internal/usecase/active_sessions_test.go
git commit -m "fix(usecase): carry tag/note in the active-stop queue payload — server stop blanked them and the next pull erased local values"
```

---

### Task 4: started_at end-to-end im Active-Start-Push

Der Start-Push überträgt kein `started_at` — der Server stempelt `time.Now()` beim Drain (`internal/adapter/sqliteserver/active_sessions.go:65`). Bei verzögertem Sync (offline, Backoff) springt der lokale Timer nach dem nächsten Pull auf die Drain-Zeit. Fix über alle 4 Schichten; macht außerdem Task 10 (CorrectStart) serverseitig wirksam.

**Files:**
- Modify: `internal/usecase/active_sessions.go` (`encodeActiveStart`)
- Modify: `internal/adapter/httpsync/worker.go` (`activeStartBody`, `drainActiveStart`-Aufruf)
- Modify: `internal/adapter/httpsync/client.go` (`StartActive`-Signatur: `rg -n "func \(c \*Client\) StartActive" internal/adapter/httpsync/client.go`)
- Modify: `internal/adapter/httpserver/active_sessions_handlers.go` (`ActiveServer`-Interface + `NewActiveStartHandler`)
- Modify: `internal/adapter/sqliteserver/active_sessions.go` (`Start`)
- Tests: bestehende in allen vier Packages anpassen + neue Assertion in sqliteserver

- [ ] **Step 1: usecase-Payload erweitern**

`encodeActiveStart` in `internal/usecase/active_sessions.go`:

```go
// encodeActiveStart produces the queue payload for an active-session start.
// The shape matches httpsync.Worker's activeStartBody (snake_case JSON).
func encodeActiveStart(row domain.ActiveSession) ([]byte, error) {
	return json.Marshal(struct {
		Action          string    `json:"action"`
		ProjectID       string    `json:"project_id"`
		StartedAt       time.Time `json:"started_at"`
		StartedOnDevice string    `json:"started_on_device"`
		Tag             string    `json:"tag"`
		Note            string    `json:"note"`
	}{"start", row.ProjectID, row.StartedAt, row.StartedOnDevice, row.Tag, row.Note})
}
```

- [ ] **Step 2: Worker-Body + Drain-Aufruf erweitern**

`internal/adapter/httpsync/worker.go`:

```go
// activeStartBody is the JSON shape written by usecase.encodeActiveStart.
type activeStartBody struct {
	Action          string    `json:"action"`
	ProjectID       string    `json:"project_id"`
	StartedAt       time.Time `json:"started_at"`
	StartedOnDevice string    `json:"started_on_device"`
	Tag             string    `json:"tag"`
	Note            string    `json:"note"`
}
```

In `drainActiveStart` den Client-Aufruf erweitern auf `w.client.StartActive(ctx, e.RowID, body.StartedAt, body.StartedOnDevice, e.ExpectedVersion, body.Tag, body.Note)`.

- [ ] **Step 3: Client-Methode erweitern**

`StartActive` in `client.go` lesen; Parameter `startedAt time.Time` (direkt nach der rowID/projectID) ergänzen und im Request-Body-Struct `StartedAt time.Time` mit Tag `json:"started_at"` mitschicken (das Body-Struct dort hat bereits started_on_device/tag/note).

- [ ] **Step 4: Server-Handler + Store**

`internal/adapter/httpserver/active_sessions_handlers.go`:
- Interface: `Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error)` (Import `time` ergänzen).
- `NewActiveStartHandler`-Body-Struct um `StartedAt time.Time` (Tag `json:"started_at"`) erweitern und durchreichen: `store.Start(user.ID, projectID, body.StartedAt, body.StartedOnDevice, expected, body.Tag, body.Note)`.

`internal/adapter/sqliteserver/active_sessions.go` `Start`: Parameter `startedAt time.Time` nach `projectID` einfügen; statt `now := time.Now().UTC()`:

```go
	if startedAt.IsZero() {
		startedAt = time.Now().UTC() // legacy clients send no started_at
	}
	startedAt = startedAt.UTC()
```

und `startedAt` überall verwenden, wo bisher `now` für die `started_at`-Spalte und das Rückgabe-Struct stand.

- [ ] **Step 5: Compiler-geführte Anpassung aller Aufrufer + neue Assertion**

`go build ./...` — alle Fehler fixen (Tests in den vier Packages). In `internal/adapter/sqliteserver/active_sessions_test.go` Assertion ergänzen: `Start` mit `startedAt := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)` → zurückgegebene und per `Get` gelesene Row hat exakt diese `StartedAt`.

Run: `go build ./... && go test ./internal/usecase/ ./internal/adapter/httpsync/ ./internal/adapter/httpserver/ ./internal/adapter/sqliteserver/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A internal/usecase internal/adapter/httpsync internal/adapter/httpserver internal/adapter/sqliteserver
git commit -m "fix(sync): carry started_at through the active-start push — server stamped drain time, so delayed syncs shifted the running timer on the next pull"
```

---

### Task 5: drainActiveStart schreibt Server-Version zurück

`worker.go:471` verwirft die Server-Antwort (`_, err :=`). Lokal bleibt `Version: 0`; der spätere Stop schickt `If-Match: 0` → garantierter 409 → `DrainHalt` (Queue dauerhaft blockiert). Vergleiche `drainSession` (worker.go:394-395), das den Writeback korrekt macht.

**Files:**
- Modify: `internal/adapter/httpsync/worker.go` (`drainActiveStart`)
- Test: `internal/adapter/httpsync/worker_test.go` (erweitern, nach Muster der bestehenden Drain-Tests mit httptest-Fake-Server)

- [ ] **Step 1: Failing Test**

Szenario nach Muster der bestehenden active-start-Drain-Tests: Start enqueuen, lokalen ActiveStore vorher mit der Version-0-Row befüllen, Fake-Server antwortet 200 mit vollem ActiveSession-JSON inkl. `Version: 7` (Feldnamen am Wire-Format der bestehenden Tests ausrichten), drainen. Kern-Assertion:

```go
	got, err := activeStore.Get(userID, projectID)
	if err != nil {
		t.Fatalf("local row gone after drain: %v", err)
	}
	if got.Version != 7 {
		t.Errorf("local version = %d, want 7 (server write-back missing)", got.Version)
	}
```

Run: `go test ./internal/adapter/httpsync/ -run <TestName> -v` → FAIL (Version bleibt 0)

- [ ] **Step 2: Implementierung**

`drainActiveStart` in `worker.go` — Block nach dem Unmarshal ersetzen:

```go
	srv, err := w.client.StartActive(ctx, e.RowID, body.StartedAt, body.StartedOnDevice, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		w.emitConflictFromError(ctx, "active_sessions", e.RowID, e.Seq, body, err)
		return DrainHalt, nil
	}
	if err != nil {
		return w.classifyPushError("active_sessions", e.RowID, e.Seq, err)
	}
	// Write the server-assigned version back to the local row so the later
	// Stop's If-Match matches. Skip when the row is already gone (user
	// stopped while offline — the queued stop reconciles via the 409-retry
	// in drainActiveStop); upserting here would resurrect a finished session.
	if _, gerr := w.active.Get(w.userID, e.RowID); gerr == nil {
		srv.UserID = w.userID
		_ = w.active.Upsert(srv)
	}
	return DrainAck, nil
```

- [ ] **Step 3: Tests**

Run: `go test ./internal/adapter/httpsync/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/httpsync/worker.go internal/adapter/httpsync/worker_test.go
git commit -m "fix(httpsync): write server-assigned version back after active-start push — stop pushed If-Match:0 and 409-halted the queue forever"
```

---

### Task 6: drainActiveStop — Conflict einmal mit Server-Version retryen

Auch mit Task 5 bleibt der Offline-Fall (Start+Stop beide enqueued, bevor der Start gepusht war): der Stop-Entry trägt `ExpectedVersion=0`, der Server hat nach dem Start-Drain Version N. Statt `DrainHalt`: aus dem 409-Body (`ConflictError.Current`) die aktuelle Version lesen und den Stop EINMAL damit wiederholen (Single-User: den eigenen Stop gewinnen lassen).

**Files:**
- Modify: `internal/adapter/httpsync/worker.go` (`drainActiveStop` + Helper)
- Test: `internal/adapter/httpsync/worker_test.go`

- [ ] **Step 1: Failing Test**

Fake-Server: DELETE mit `If-Match: 0` → 409 + Body `{"current": {... "Version": 3 ...}}` (Wire-Format wie in den bestehenden Conflict-Tests); DELETE mit `If-Match: 3` → 200. Assertions: Entry wird geackt (Queue leer), der zweite Request kam mit If-Match 3, KEINE ConflictMsg auf `worker.Conflicts()`.

Run → FAIL (heute: DrainHalt + ConflictMsg)

- [ ] **Step 2: Implementierung**

```go
func (w *Worker) drainActiveStop(ctx context.Context, e ports.WriteQueueEntry) (DrainAction, error) {
	var body activeStopBody
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return DrainAck, err
	}
	_, err := w.client.StopActive(ctx, e.RowID, e.ExpectedVersion, body.Tag, body.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		// Stale local version (start drained after the stop was enqueued).
		// The 409 body carries the server's current row — retry once with
		// that version. Stopping our own session is last-writer-wins for
		// the single-user PoC; a failing retry still halts for the overlay.
		if cur, ok := conflictCurrentActive(err); ok {
			_, rerr := w.client.StopActive(ctx, e.RowID, cur.Version, body.Tag, body.Note)
			if rerr == nil || errors.Is(rerr, ports.ErrActiveSessionNotFound) {
				return DrainAck, nil
			}
		}
		w.emitConflictFromError(ctx, "active_sessions_stop", e.RowID, e.Seq, body, err)
		return DrainHalt, nil
	}
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		// Another device already stopped this session — nothing left to stop.
		return DrainAck, nil
	}
	if err != nil {
		return w.classifyPushError("active_sessions_stop", e.RowID, e.Seq, err)
	}
	return DrainAck, nil
}

// conflictCurrentActive extracts the server's current ActiveSession from a
// 409 ConflictError, when present.
func conflictCurrentActive(err error) (domain.ActiveSession, bool) {
	var ce *ConflictError
	if !errors.As(err, &ce) || len(ce.Current) == 0 {
		return domain.ActiveSession{}, false
	}
	var cur domain.ActiveSession
	if jerr := json.Unmarshal(ce.Current, &cur); jerr != nil {
		return domain.ActiveSession{}, false
	}
	return cur, true
}
```

- [ ] **Step 3: Tests**

Run: `go test ./internal/adapter/httpsync/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/httpsync/worker.go internal/adapter/httpsync/worker_test.go
git commit -m "fix(httpsync): retry active-stop once with the server's current version on 409 — offline start+stop pairs halted the queue"
```

---

### Task 7: CLI `stop` stoppt DIE aktive Session (nicht das cwd-Projekt)

`runStopNew` (`internal/frontend/cli/worktime.go:467`) löst das Projekt via cwd-Kaskade auf und stoppt dann `(user, falsches Projekt)`; `ErrActiveSessionNotFound` wird als silent no-op geschluckt — Sessions laufen ewig, exit 0, keine Ausgabe.

**Files:**
- Modify: `internal/frontend/cli/worktime.go` (`WorktimeDeps` + `runStopNew` + Guard in `newStopCmd`)
- Modify: `cmd/flow/main.go` (Wiring `ListActiveSessions`)
- Test: `internal/frontend/cli/worktime_start_stop_test.go` (erweitern)

- [ ] **Step 1: Failing Test**

Bestehende new-path-Tests in `worktime_start_stop_test.go` lesen (`rg -n "runStopNew\|StopActiveSession" internal/frontend/cli/worktime_start_stop_test.go | head`) und nach deren Fixture-Muster zwei Tests ergänzen:

1. `TestStopNewStopsTheActiveSessionRegardlessOfCwd`: Fake-Deps mit `ListActiveSessions` → eine ActiveSession auf Projekt "p-A"; `ResolveProject` würde "p-B" liefern (cwd-divergenz). Nach `flow worktime stop`: `StopActiveSession` wurde mit `"p-A"` aufgerufen.
2. `TestStopNewNothingRunningPrintsHint`: `ListActiveSessions` → leer; stderr enthält `"Keine laufende Session"`, exit 0, `StopActiveSession` wurde NICHT aufgerufen.

Run: `go test ./internal/frontend/cli/ -run TestStopNew -v` → FAIL (`unknown field ListActiveSessions`)

- [ ] **Step 2: Deps-Feld ergänzen**

In `WorktimeDeps` (worktime.go, nach `StopActiveSession`):

```go
	// ListActiveSessions returns the currently running sessions for the user.
	// Stop/toggle/pause operate on THE active session — not on whatever
	// project the cwd happens to resolve to.
	ListActiveSessions func(userID string) ([]domain.ActiveSession, error)
```

- [ ] **Step 3: runStopNew ersetzen**

```go
// runStopNew handles the new sqlite-backed stop path: it stops the user's
// running session. --project disambiguates when several run in parallel.
func runStopNew(cmd *cobra.Command, deps WorktimeDeps, projectFlag, tag, note string) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fprintln(cmd.ErrOrStderr(), "Keine laufende Session")
		return nil
	}
	var projectID string
	switch {
	case projectFlag != "":
		pwd, _ := os.Getwd()
		pr, rerr := deps.ResolveProject(deps.UserID, projectFlag, pwd)
		if rerr != nil {
			return rerr
		}
		projectID = pr.ID
	case len(list) == 1:
		projectID = list[0].ProjectID
	default:
		return fmt.Errorf("%d Sessions laufen parallel — mit --project wählen", len(list))
	}
	sess, err := deps.StopActiveSession(deps.UserID, projectID, tag, note)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		fprintln(cmd.ErrOrStderr(), "Keine laufende Session für dieses Projekt")
		return nil
	}
	if err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	h := int(sess.Elapsed.Hours())
	m := int(sess.Elapsed.Minutes()) % 60
	fprintf(cmd.ErrOrStderr(), "Gestoppt nach %dh %02dm\n", h, m)
	return nil
}
```

Guard in `newStopCmd` erweitern: `if deps.ResolveProject != nil && deps.StopActiveSession != nil && deps.ListActiveSessions != nil {`.

- [ ] **Step 4: Wiring in main.go**

Im `Worktime: cli.WorktimeDeps{...}`-Block (nach `StopActiveSession`):

```go
				ListActiveSessions: func(userID string) ([]domain.ActiveSession, error) {
					return activeSessionsUC.ListActive(userID)
				},
```

- [ ] **Step 5: Tests + Commit**

Run: `go test ./internal/frontend/cli/ ./cmd/flow/` → PASS. Bestehende Tests, die das alte cwd-Stop-Verhalten asserten, an das neue Verhalten anpassen (nicht löschen — Erwartung umdrehen).

```bash
git add internal/frontend/cli/worktime.go internal/frontend/cli/worktime_start_stop_test.go cmd/flow/main.go
git commit -m "fix(cli): stop targets the running session instead of the cwd-resolved project — sessions were unstoppable after a cd"
```

---

### Task 8: CLI `toggle` auf den neuen Pfad

`newToggleCmd` (worktime.go:494) ruft unconditional `SessionWriter.Toggle()` (flockstate) — start/stop nutzen aber sqlite. Zwei Wahrheitsquellen; toggle ist Soennes primäres tmux-Binding.

**Files:**
- Modify: `internal/frontend/cli/worktime.go`
- Test: `internal/frontend/cli/worktime_start_stop_test.go`

- [ ] **Step 1: Failing Tests**

Nach bestehendem Fixture-Muster:
1. `TestToggleNewStopsWhenRunning`: eine ActiveSession läuft → toggle ruft `StopActiveSession` mit deren ProjectID; stderr enthält `"Gestoppt"`.
2. `TestToggleNewStartsWhenIdle`: keine ActiveSession → toggle ruft `ResolveProject` + `StartActiveSession`; stderr enthält `"läuft seit"`.

Run → FAIL (legacy SessionWriter.Toggle wird gerufen)

- [ ] **Step 2: Implementierung**

`newToggleCmd` ersetzen:

```go
func newToggleCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "toggle",
		Aliases:      []string{"s"},
		Short:        "Start wenn idle, stopp wenn läuft (alias: s)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StopActiveSession != nil &&
				deps.ResolveProject != nil && deps.StartActiveSession != nil {
				return runToggleNew(cmd, deps)
			}
			// Legacy TSV/flockstate path (tests without new-path deps).
			msg, err := deps.SessionWriter.Toggle()
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintln(cmd.ErrOrStderr(), msg)
			return nil
		},
	}
}

// runToggleNew: stop the earliest running session when one exists, else
// resolve a project (pwd → MRU → Allgemein) and start.
func runToggleNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		target := list[0]
		for _, a := range list[1:] {
			if a.StartedAt.Before(target.StartedAt) {
				target = a
			}
		}
		sess, serr := deps.StopActiveSession(deps.UserID, target.ProjectID, "", "")
		if serr != nil && !errors.Is(serr, ports.ErrActiveSessionNotFound) {
			return serr
		}
		_ = deps.Tmux.RefreshClient()
		fprintf(cmd.ErrOrStderr(), "Gestoppt nach %dh %02dm\n",
			int(sess.Elapsed.Hours()), int(sess.Elapsed.Minutes())%60)
		return nil
	}
	pwd, _ := os.Getwd()
	pr, err := deps.ResolveProject(deps.UserID, "", pwd)
	if err != nil {
		return err
	}
	if _, err := deps.StartActiveSession(deps.UserID, pr.ID, "", ""); err != nil {
		if errors.Is(err, usecase.ErrActiveSessionExists) {
			fprintf(cmd.ErrOrStderr(), "Session auf '%s' läuft bereits\n", pr.Name)
			return nil
		}
		return err
	}
	_ = deps.Tmux.RefreshClient()
	fprintf(cmd.ErrOrStderr(), "Worktime läuft seit %s auf '%s'\n",
		deps.Clock.Now().Format("15:04"), pr.Name)
	return nil
}
```

- [ ] **Step 3: Tests + Commit**

Run: `go test ./internal/frontend/cli/` → PASS

```bash
git add internal/frontend/cli/worktime.go internal/frontend/cli/worktime_start_stop_test.go
git commit -m "fix(cli): toggle uses the sqlite ActiveSessions path — it wrote phantom flockstate markers while start/stop used sqlite"
```

---

### Task 9: CLI `pause`/`resume` auf den neuen Pfad

`pause`/`resume` laufen über `SessionWriter` (flockstate-Active-Marker) und sehen neue-Pfad-Sessions nicht. Neu: pause = Stop der aktiven Session + Pause-Marker (flockstate, per-device); resume = Marker löschen + Start auf MRU-Projekt.

**Files:**
- Modify: `internal/frontend/cli/worktime.go` (`WorktimeDeps` + `newPauseCmd` + `newResumeCmd`)
- Modify: `cmd/flow/main.go` (Wiring `PauseMarker`)
- Test: `internal/frontend/cli/worktime_start_stop_test.go`

- [ ] **Step 1: Failing Tests**

1. `TestPauseNewStopsAndSetsMarker`: ActiveSession läuft → pause ruft `StopActiveSession` und `PauseMarker.SetPause`; stderr enthält `"Pausiert"`.
2. `TestPauseNewIdleIsNoop`: nichts läuft → kein Stop-Aufruf, exit 0.
3. `TestResumeNewStartsMRUAndClearsMarker`: nichts läuft, Pause-Marker gesetzt → resume ruft `ResolveProject(userID, "", "")` (leere pwd → MRU-Kaskade) + `StartActiveSession` + `ClearPause`.
4. `TestResumeNewAlreadyRunningClearsMarkerOnly`: Session läuft → nur `ClearPause`, kein Start.

Fake-PauseStore (3 Methoden) lokal im Test definieren. Run → FAIL

- [ ] **Step 2: Deps-Feld**

```go
	// PauseMarker is the per-device pause flag (flockstate worktime.pause).
	// Never synced; pause = stop the active session + set this marker so
	// resume knows to restart.
	PauseMarker ports.PauseStore
```

- [ ] **Step 3: newPauseCmd ersetzen**

```go
func newPauseCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "pause",
		Short:        "Aktive Session pausieren (resume mit `start`/`toggle`)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StopActiveSession != nil && deps.PauseMarker != nil {
				return runPauseNew(cmd, deps)
			}
			s, err := deps.SessionWriter.Pause()
			if err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			if s.Elapsed > 0 {
				h := int(s.Elapsed.Hours())
				m := int(s.Elapsed.Minutes()) % 60
				fprintf(cmd.ErrOrStderr(), "Pausiert nach %dh %02dm — `flow worktime resume` setzt fort\n", h, m)
			}
			return nil
		},
	}
}

// runPauseNew: stop the running session (idempotent no-op when idle) and
// set the per-device pause marker so resume restarts on the same project (MRU).
func runPauseNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil // idempotent like the legacy path: nothing running is fine
	}
	target := list[0]
	for _, a := range list[1:] {
		if a.StartedAt.Before(target.StartedAt) {
			target = a
		}
	}
	sess, err := deps.StopActiveSession(deps.UserID, target.ProjectID, "", "")
	if err != nil && !errors.Is(err, ports.ErrActiveSessionNotFound) {
		return err
	}
	if err := deps.PauseMarker.SetPause(deps.Clock.Now()); err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	h := int(sess.Elapsed.Hours())
	m := int(sess.Elapsed.Minutes()) % 60
	fprintf(cmd.ErrOrStderr(), "Pausiert nach %dh %02dm — `flow worktime resume` setzt fort\n", h, m)
	return nil
}
```

- [ ] **Step 4: newResumeCmd ersetzen**

```go
func newResumeCmd(deps WorktimeDeps) *cobra.Command {
	return &cobra.Command{
		Use:          "resume",
		Short:        "Nach Pause weiterarbeiten",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.ListActiveSessions != nil && deps.StartActiveSession != nil &&
				deps.ResolveProject != nil && deps.PauseMarker != nil {
				return runResumeNew(cmd, deps)
			}
			if err := deps.SessionWriter.Resume(); err != nil {
				return err
			}
			_ = deps.Tmux.RefreshClient()
			fprintln(cmd.ErrOrStderr(), "Resume — Worktime läuft weiter")
			return nil
		},
	}
}

// runResumeNew: clear the pause marker; when idle, restart on the MRU
// project (empty pwd skips the cwd step of the resolve cascade, so the
// paused project — last touched at its start — wins).
func runResumeNew(cmd *cobra.Command, deps WorktimeDeps) error {
	list, err := deps.ListActiveSessions(deps.UserID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		pr, rerr := deps.ResolveProject(deps.UserID, "", "")
		if rerr != nil {
			return rerr
		}
		if _, serr := deps.StartActiveSession(deps.UserID, pr.ID, "", ""); serr != nil &&
			!errors.Is(serr, usecase.ErrActiveSessionExists) {
			return serr
		}
	}
	if err := deps.PauseMarker.ClearPause(); err != nil {
		return err
	}
	_ = deps.Tmux.RefreshClient()
	fprintln(cmd.ErrOrStderr(), "Resume — Worktime läuft weiter")
	return nil
}
```

Hinweis: Falls `ResolveProject(userID, "", "")` mit leerer pwd in `usecase.ResolveProject` fehlschlägt (Implementierung in `internal/usecase/sessions.go` prüfen!), dort den pwd-Schritt bei leerem String sauber überspringen lassen.

- [ ] **Step 5: Wiring + Tests + Commit**

main.go Worktime-Block: `PauseMarker: activeStore,` (flockstate.State implementiert ports.PauseStore).

Run: `go test ./internal/frontend/cli/ ./cmd/flow/` → PASS

```bash
git add internal/frontend/cli/worktime.go internal/frontend/cli/worktime_start_stop_test.go cmd/flow/main.go
git commit -m "fix(cli): pause/resume operate on sqlite ActiveSessions — they read the dead flockstate marker on the new path"
```

---

### Task 10: `correct` auf den neuen Pfad (ActiveSessions.CorrectStart)

`correct` schreibt den flockstate-Marker, den der neue Pfad nicht liest. Neu: Startzeit der laufenden sqlite-Session ändern + Start mit If-Match requeuen (Server-Update via Takeover-Semantik; funktioniert dank Task 4 started_at).

**Files:**
- Modify: `internal/usecase/active_sessions.go` (neue Methode)
- Modify: `internal/frontend/cli/worktime.go` (`WorktimeDeps` + `newCorrectCmd`)
- Modify: `cmd/flow/main.go` (Wiring)
- Tests: `internal/usecase/active_sessions_test.go`, `internal/frontend/cli/worktime_start_stop_test.go`

- [ ] **Step 1: Failing usecase-Test**

`TestCorrectStartMovesStartedAtAndRequeues`: Start, dann `CorrectStart(user, ts)`. Assertions: lokale Row hat `StartedAt == ts.UTC()`; die Queue enthält einen zweiten `active_sessions`-Entry mit `ExpectedVersion == cur.Version` und Payload-`started_at == ts.UTC()`. Außerdem `TestCorrectStartNothingRunning`: leerer Store → `ports.ErrActiveSessionNotFound`.

- [ ] **Step 2: Usecase-Methode**

In `internal/usecase/active_sessions.go`:

```go
// CorrectStart moves the start time of the user's (earliest) running session
// and re-queues the start with If-Match so the server row follows. Returns
// ports.ErrActiveSessionNotFound when nothing is running.
func (a *ActiveSessions) CorrectStart(userID string, ts time.Time) error {
	list, err := a.active.ListByUser(userID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return ports.ErrActiveSessionNotFound
	}
	cur := list[0]
	for _, c := range list[1:] {
		if c.StartedAt.Before(cur.StartedAt) {
			cur = c
		}
	}
	cur.StartedAt = ts.UTC()
	if err := a.active.Upsert(cur); err != nil {
		return err
	}
	payload, err := encodeActiveStart(cur)
	if err != nil {
		return err
	}
	if _, err := a.queue.Enqueue("active_sessions", cur.ProjectID, payload, cur.Version); err != nil {
		return err
	}
	a.signalPush()
	return nil
}
```

(`time`-Import existiert bereits.)

- [ ] **Step 3: CLI-Branch**

`WorktimeDeps`-Feld:

```go
	// CorrectActiveStart moves the running session's start time (new path).
	CorrectActiveStart func(userID string, ts time.Time) error
```

`newCorrectCmd`-RunE: nach dem `domain.ParseStartArg`-Block:

```go
			if deps.CorrectActiveStart != nil {
				if err := deps.CorrectActiveStart(deps.UserID, ts); err != nil {
					if errors.Is(err, ports.ErrActiveSessionNotFound) {
						fprintln(cmd.ErrOrStderr(), "Keine laufende Session")
						return nil
					}
					return err
				}
				_ = deps.Tmux.RefreshClient()
				fprintf(cmd.ErrOrStderr(), "Startzeit korrigiert auf %s\n", ts.Format("15:04"))
				return nil
			}
			// Legacy path unverändert darunter.
```

main.go-Wiring: `CorrectActiveStart: activeSessionsUC.CorrectStart,`

- [ ] **Step 4: Tests + Commit**

Run: `go test ./internal/usecase/ ./internal/frontend/cli/ ./cmd/flow/` → PASS

```bash
git add internal/usecase/active_sessions.go internal/usecase/active_sessions_test.go internal/frontend/cli/worktime.go internal/frontend/cli/worktime_start_stop_test.go cmd/flow/main.go
git commit -m "feat(usecase): ActiveSessions.CorrectStart + wire worktime correct to the sqlite path"
```

---

### Task 11: TUI — `s` stoppt laufende Session; `p` pausiert auf dem neuen Pfad

`handleSKey` (`internal/frontend/tui/screen/worktime/today_project_picker.go:66`) öffnet im neuen Pfad IMMER den Picker; laufende Projekte sind rausgefiltert, der Picker hat kein Stop-Affordance → Session ist im TUI unstoppbar. Der `p`-Pause-Pfad ruft `SessionWriter.Pause()` (flockstate) — gleiche Krankheit.

**Files:**
- Modify: `internal/frontend/tui/screen/worktime/today_project_picker.go` (`handleSKey` + neuer Cmd)
- Modify: `internal/frontend/tui/screen/worktime/today_actions.go` (Pause-Branch; Datei vorher lesen: `rg -n "pauseCmd|toggleStartStopCmd" internal/frontend/tui/screen/worktime/`)
- Modify: `internal/frontend/tui/screen/worktime/model.go` (Deps-Kommentar aktualisieren — kein Code nötig, `Deps.ActiveSessions` ist bereits `*usecase.ActiveSessions` mit Stop/ListActive)
- Test: `internal/frontend/tui/screen/worktime/today_project_picker_test.go` (erweitern)

- [ ] **Step 1: Failing Test**

Bestehende Picker-Tests lesen (`today_project_picker_test.go` hat ein Rig mit gefaktem ActiveSessions-Setup). Neuer Test `TestSKeyStopsRunningSession`: heute-Model mit `h.activeSessions = []domain.ActiveSession{{ProjectID: "p1", StartedAt: ...}}` (wie die bestehenden Tests das Feld setzen), `s` drücken → zurückgegebener Cmd ausführen → die emittierte Msg ist `heuteActionDoneMsg` mit `toast` enthält `"gestoppt"`, und der Fake-Store hat keine aktive Session mehr. Außerdem `TestSKeyOpensPickerWhenIdle` (Bestand absichern — existiert vermutlich schon).

WICHTIG: `Deps.ActiveSessions` ist der konkrete `*usecase.ActiveSessions` — die Tests konstruieren ihn real über Fake-Stores (so machen es die bestehenden Picker-Tests; Muster übernehmen).

Run → FAIL (Picker öffnet statt zu stoppen)

- [ ] **Step 2: handleSKey + Stop-Cmd**

In `today_project_picker.go`:

```go
// handleSKey is the dispatcher for the `s` key in normal (no-dialog) mode.
// New path: stop the running session when one exists, else open the picker.
// Legacy path: toggleStartStopCmd (unchanged).
func (h heute) handleSKey() (tea.Model, tea.Cmd) {
	if h.deps.ActiveSessions != nil && h.deps.UserID != "" {
		if len(h.activeSessions) > 0 {
			h.actionInFlight = true
			return h, h.activeSessionsStopCmd(h.activeSessions[0])
		}
		return h.openProjectPicker()
	}
	return h, h.toggleStartStopCmd()
}

// activeSessionsStopCmd stops the given running session and emits
// heuteActionDoneMsg; emitWorktimeChanged reloads all sub-tabs.
func (h heute) activeSessionsStopCmd(target domain.ActiveSession) tea.Cmd {
	as := h.deps.ActiveSessions
	userID := h.deps.UserID
	now := h.deps.Clock.Now()
	mut := func() tea.Msg {
		sess, err := as.Stop(userID, target.ProjectID, "", "")
		if err != nil {
			if errors.Is(err, ports.ErrActiveSessionNotFound) {
				return heuteActionDoneMsg{toast: "Nichts läuft", info: true}
			}
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{
			toast: fmt.Sprintf("Gestoppt nach %dh %02dm — %s",
				int(sess.Elapsed.Hours()), int(sess.Elapsed.Minutes())%60, now.Format("15:04")),
		}
	}
	return tea.Batch(mut, emitWorktimeChanged(now))
}
```

Import `"github.com/serverkraken/flow/internal/ports"` ergänzen. Bei mehreren parallelen Sessions stoppt `s` die erste (h.activeSessions[0]) — für den Single-User-PoC ausreichend; Mehrfachauswahl ist Phase 2.

- [ ] **Step 3: TUI-Pause-Branch**

`rg -n "SessionWriter.Pause|pauseCmd" internal/frontend/tui/screen/worktime/` — den Cmd finden, der `deps.SessionWriter.Pause()` ruft (vermutlich `today_actions.go`). Dort am Anfang des Cmd-Closures den neuen Pfad einschieben (gleiches Muster wie `activeSessionsStopCmd`, nur zusätzlich Pause-Marker):

```go
		if h.deps.ActiveSessions != nil && h.deps.UserID != "" && len(h.activeSessions) > 0 {
			target := h.activeSessions[0]
			sess, err := h.deps.ActiveSessions.Stop(h.deps.UserID, target.ProjectID, "", "")
			if err != nil && !errors.Is(err, ports.ErrActiveSessionNotFound) {
				return heuteActionDoneMsg{err: err}
			}
			_ = h.deps.SessionWriter.State.SetPause(now)
			return heuteActionDoneMsg{
				toast: fmt.Sprintf("Pausiert nach %dh %02dm", int(sess.Elapsed.Hours()), int(sess.Elapsed.Minutes())%60),
			}
		}
```

(`SessionWriter.State` ist der flockstate-Store mit dem Pause-Marker; exportiertes Feld, direkt nutzbar. Resume im TUI läuft über `s`/Picker → Start, kein eigener Branch nötig; der Pause-Marker wird beim nächsten Start vom Reader ignoriert, weil `ActiveStart()` dann eine Session liefert. Falls die heutige TUI gar keinen expliziten Resume-Key hat: nichts weiter tun.)

- [ ] **Step 4: Tests + Commit**

Run: `go test ./internal/frontend/tui/screen/worktime/` → PASS (Golden-/Baseline-Tests bei Bedarf via dokumentiertem UPDATE-Flag der Suite aktualisieren — `rg -n "UPDATE" internal/frontend/tui/lint/ | head` zeigt die Konvention).

```bash
git add internal/frontend/tui/screen/worktime/
git commit -m "fix(tui): s stops the running session on the sqlite path (picker only when idle); pause works on ActiveSessions"
```

---

### Task 12: TSV-Auto-Migration in buildDeps statt Block-Guard

Der Guard (`internal/frontend/cli/worktime.go:119-144`) blockt ALLE worktime-Verben hart, bis manuell migriert wurde — jeder main-Upgrader hat `~/.tmux/worktime.log` und steht vor einer kaputten CLI. `MigrateTSV.Run` ist idempotent (UUIDv5) und archiviert die TSV per Rename → einfach automatisch beim Start migrieren; der Sidekick (umgeht den cobra-Tree) profitiert dann auch.

**Files:**
- Modify: `cmd/flow/main.go` (`buildDeps`: Auto-Migration nach Identity-Resolution)
- Modify: `internal/frontend/cli/worktime.go` (Guard-Block + `SessionCount`/`TSVPath`/`CacheDBPath`-Felder entfernen, sofern nur vom Guard genutzt — `rg -n "SessionCount|TSVPath|CacheDBPath" internal/frontend/cli/ cmd/flow/` prüfen; `migrate-from-tsv`-Subcommand BLEIBT)
- Test: `internal/frontend/cli/worktime_test.go` (Guard-Tests entfernen/anpassen)

- [ ] **Step 1: Auto-Migration in buildDeps**

In `cmd/flow/main.go` nach der Konstruktion von `cacheSessions` (Zeile ~172) und `localUser` — den `migrateTSVUC`-Konstruktor (aktuell Zeile ~235) VOR diesen Block hochziehen:

```go
	migrateTSVUC := usecase.NewMigrateTSV(cacheUsers, cacheProjects, cacheSessions)
	// Auto-migrate the legacy TSV on first run: worktime.log present + empty
	// sqlite cache means an upgrade from the single-user main branch. Run()
	// is idempotent (UUIDv5 row IDs) and archives the TSV via rename, so this
	// fires exactly once. Replaces the old hard-blocking cobra guard.
	if _, statErr := os.Stat(p.WorktimeLog); statErr == nil {
		if rows, lerr := cacheSessions.Load(localUser.ID); lerr == nil && len(rows) == 0 {
			if res, merr := migrateTSVUC.Run(localUser.ID, p.WorktimeLog, "Allgemein"); merr != nil {
				slog.Warn("flow: tsv auto-migration failed — run `flow worktime migrate-from-tsv`", slog.Any("err", merr))
			} else if res.Inserted > 0 {
				slog.Info("flow: tsv auto-migration done",
					slog.Int("inserted", res.Inserted), slog.String("archived", res.ArchivedTo))
			}
		}
	}
```

ACHTUNG Reihenfolge: `cacheProjects` (Zeile ~171) muss vor diesem Block stehen — passt. `slog`-Import in main.go ergänzen. Die alte `migrateTSVUC`-Zeile weiter unten löschen.

- [ ] **Step 2: Guard entfernen**

In `internal/frontend/cli/worktime.go` den kompletten `worktimeCmd.PersistentPreRunE`-Guard-Block (Zeile 110-144) löschen. Felder `SessionCount`, `TSVPath`, `CacheDBPath` aus `WorktimeDeps` + main.go-Wiring entfernen, WENN `rg -n "SessionCount|deps.TSVPath|CacheDBPath" internal/ cmd/` keine weiteren Nutzer zeigt (der `migrate-from-tsv`-Subcommand bekommt seinen Pfad separat — prüfen via `rg -n "MigrateTSVDeps" internal/frontend/cli/migrate_tsv.go`; falls er TSVPath aus WorktimeDeps zieht, Feld behalten).

- [ ] **Step 3: Tests anpassen**

`rg -ln "migrate-from-tsv first|TSV detected" internal/frontend/cli/` — Guard-Tests entfernen. Run: `go test ./internal/frontend/cli/ ./cmd/flow/` → PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/main.go internal/frontend/cli/worktime.go internal/frontend/cli/worktime_test.go
git commit -m "fix(flow): auto-migrate legacy worktime.log on first run — the hard guard blocked every worktime verb for upgraders"
```

---

### Task 13: Identity härten — Keyring-Fehler sichtbar, kein Daten-Fork, CountOwnedRows komplett

Drei verwandte Bugs: (a) `cmd/flow/main.go:156` verschluckt JEDEN Keyring-Fehler (auch locked Keychain) → läuft still als `local`; (b) nach der Adoption existiert kein `local`-Row mehr — `EnsureBySub("local")` legt einen FRISCHEN User an und forkt die Daten; (c) `CountOwnedRows` (`internal/adapter/sqliteclient/users.go:92`) zählt nur projects+sessions — User mit nur Repos/Notes/aktiver Session überspringen die Adoption.

**Files:**
- Modify: `internal/usecase/identity.go` (`IdentityStore` + `ResolveActiveUser`)
- Modify: `internal/adapter/sqliteclient/users.go` (`SoleUser` + `CountOwnedRows`)
- Modify: `cmd/flow/main.go` (Keyring-Fehlerbehandlung)
- Tests: `internal/usecase/identity_test.go`, `internal/adapter/sqliteclient/users_test.go`

- [ ] **Step 1: Failing Tests**

1. `users_test.go` — `TestCountOwnedRowsCountsAllUserTables`: User anlegen, je eine Row in `active_sessions`, `repos`, `repo_notes` (Insert-Helfer der bestehenden Tests nutzen; FK-Abhängigkeiten beachten: repo_notes braucht repos, active_sessions braucht projects) → Count > 0 obwohl projects/sessions leer.
2. `users_test.go` — `TestSoleUser`: leere DB → ok=false; ein User → ok=true + dessen Row; zwei User → ok=false.
3. `identity_test.go` — `TestResolveActiveUserEmptySubFallsBackToSoleUser`: Store enthält NUR den (bereits adoptierten) OIDC-User, kein `local` → `ResolveActiveUser("")` liefert den OIDC-User und legt KEINEN neuen `local` an.

Run → FAIL

- [ ] **Step 2: sqliteclient.Users erweitern**

`CountOwnedRows`-SQL ersetzen:

```go
// CountOwnedRows returns how many rows across all user_id-keyed tables
// reference the given user. Drives the first-login adoption gate — counting
// only projects+sessions skipped users whose data is repos/notes/active only.
func (u *Users) CountOwnedRows(userID string) (int, error) {
	var n int
	err := u.store.DB().QueryRow(
		`SELECT (SELECT COUNT(*) FROM projects        WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM sessions        WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM active_sessions WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM repos           WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM repo_notes      WHERE user_id = ?)`,
		userID, userID, userID, userID, userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.Users.CountOwnedRows: %w", err)
	}
	return n, nil
}

// SoleUser returns the only user row when exactly one exists. Identity uses
// it as the logged-out fallback so a transient keyring failure after adoption
// cannot fork the data under a freshly-created `local` user.
func (u *Users) SoleUser() (domain.User, bool, error) {
	rows, err := u.store.DB().Query(
		`SELECT id, oidc_sub, email, display_name, created_at FROM users LIMIT 2`)
	if err != nil {
		return domain.User{}, false, fmt.Errorf("sqliteclient.Users.SoleUser: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var users []domain.User
	for rows.Next() {
		var user domain.User
		var createdAt string
		if err := rows.Scan(&user.ID, &user.OIDCSub, &user.Email, &user.DisplayName, &createdAt); err != nil {
			return domain.User{}, false, fmt.Errorf("sqliteclient.Users.SoleUser: scan: %w", err)
		}
		if t, perr := time.Parse(time.RFC3339, createdAt); perr == nil {
			user.CreatedAt = t
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return domain.User{}, false, err
	}
	if len(users) != 1 {
		return domain.User{}, false, nil
	}
	return users[0], true, nil
}
```

- [ ] **Step 3: Identity erweitern**

`internal/usecase/identity.go`:

```go
// IdentityStore is the subset of the user store the Identity use case needs.
type IdentityStore interface {
	EnsureBySub(sub, email, displayName string) (domain.User, error)
	GetBySub(sub string) (domain.User, error)
	CountOwnedRows(userID string) (int, error)
	RelabelBySub(fromSub, toSub, email, displayName string) error
	SoleUser() (domain.User, bool, error)
}
```

`ResolveActiveUser` — den `tokenSub == ""`-Zweig ersetzen:

```go
	if tokenSub == "" {
		if u, err := i.store.GetBySub(i.localSub); err == nil {
			return u, nil
		}
		// No local profile: after first-login adoption the local row was
		// relabeled to the OIDC sub. A keyring hiccup (locked keychain)
		// must NOT mint a fresh `local` user — that forks the data. With
		// exactly one user in the DB (single-user PoC) run as that user.
		if u, ok, err := i.store.SoleUser(); err == nil && ok {
			return u, nil
		}
		return i.store.EnsureBySub(i.localSub, "", "")
	}
```

Fakes in `identity_test.go` um `SoleUser` erweitern (Compiler führt).

- [ ] **Step 4: main.go Keyring-Fehler loggen**

`cmd/flow/main.go:156`-Block ersetzen:

```go
	tokenSub := ""
	switch toks, terr := keyringadapter.New().Get("tokens:" + env.ServerURL); {
	case terr == nil:
		src := toks.IDToken
		if src == "" {
			src = toks.AccessToken
		}
		if c, cerr := oidcclient.ClaimsFromToken(src); cerr == nil {
			tokenSub = c.Sub
		} else {
			slog.Warn("flow: stored token undecodable — running logged-out", slog.Any("err", cerr))
		}
	case errors.Is(terr, ports.ErrTokenNotFound):
		// Logged out — the normal offline state, no log noise.
	default:
		slog.Warn("flow: keyring unavailable — running logged-out", slog.Any("err", terr))
	}
```

Imports `errors`, `log/slog`, `github.com/serverkraken/flow/internal/ports` in main.go ergänzen.

- [ ] **Step 5: Tests + Commit**

Run: `go test ./internal/usecase/ ./internal/adapter/sqliteclient/ ./cmd/flow/` → PASS

```bash
git add internal/usecase/identity.go internal/usecase/identity_test.go internal/adapter/sqliteclient/users.go internal/adapter/sqliteclient/users_test.go cmd/flow/main.go
git commit -m "fix(identity): sole-user fallback + full CountOwnedRows + visible keyring errors — a locked keychain forked data under a fresh local user"
```

---

### Task 14: Token-Refresh in den Sync-Client verdrahten

`oidcclient.Refresh` existiert, hat aber NULL Produktions-Caller — abgelaufene Access-Tokens erzeugen chronische 401s. Fix: Refresher-Adapter (Keyring + Token-Endpoint via Server-Discovery) + 401-Retry-Once im httpsync-Client.

**Files:**
- Create: `internal/adapter/oidcclient/endpoints.go` (+ Test)
- Create: `internal/adapter/oidcclient/refresher.go` (+ Test)
- Modify: `internal/adapter/httpsync/client.go` (`SetRefresher` + `do()`)
- Modify: `cmd/flow/login.go` (`resolveOIDCEndpoints` durch `oidcclient.ResolveEndpoints` ersetzen)
- Modify: `cmd/flow/main.go` (Env.OIDCClientID + Wiring)
- Test: `internal/adapter/httpsync/client_test.go`

- [ ] **Step 1: ResolveEndpoints nach oidcclient heben**

```go
// internal/adapter/oidcclient/endpoints.go
package oidcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ResolveEndpoints asks flow-server's /api/v1/oidc/config for the IdP's
// device + token endpoints, so clients never need the IdP URL directly.
// deviceURL may be empty (not all flows need it); tokenURL is required.
func ResolveEndpoints(ctx context.Context, serverURL string, httpc *http.Client) (deviceURL, tokenURL string, err error) {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/v1/oidc/config", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oidc/config: status %s", resp.Status)
	}
	var cfg struct {
		DeviceURL string `json:"device_authorization_endpoint"`
		TokenURL  string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", "", err
	}
	if cfg.TokenURL == "" {
		return "", "", fmt.Errorf("oidc/config: no token_endpoint")
	}
	return cfg.DeviceURL, cfg.TokenURL, nil
}
```

In `cmd/flow/login.go`: `resolveOIDCEndpoints` löschen, Aufruf ersetzen durch `oidcclient.ResolveEndpoints(cmd.Context(), serverURL, http.DefaultClient)` + dort die bestehende Device-URL-Pflicht prüfen (`if deviceURL == "" { return fmt.Errorf("oidc/config: IdP exposes no device_authorization_endpoint") }`). Test mit httptest-Server für beide Fälle (ok / fehlender token_endpoint).

- [ ] **Step 2: StoreRefresher**

```go
// internal/adapter/oidcclient/refresher.go
package oidcclient

import (
	"context"
	"net/http"
	"sync"

	"github.com/serverkraken/flow/internal/ports"
)

// StoreRefresher exchanges the keyring's refresh token for fresh tokens and
// persists the result. The token endpoint is resolved lazily from the
// flow-server's /api/v1/oidc/config and cached for the process lifetime.
type StoreRefresher struct {
	ServerURL  string
	ClientID   string
	Store      ports.TokenStore
	Slot       string
	HTTPClient *http.Client

	mu       sync.Mutex
	tokenURL string
}

// RefreshTokens performs one refresh-token exchange and stores the result.
// Returns ports.ErrTokenNotFound when no refresh token is available.
func (r *StoreRefresher) RefreshTokens(ctx context.Context) (ports.Tokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := r.Store.Get(r.Slot)
	if err != nil {
		return ports.Tokens{}, err
	}
	if cur.RefreshToken == "" {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	if r.tokenURL == "" {
		_, tokenURL, rerr := ResolveEndpoints(ctx, r.ServerURL, r.HTTPClient)
		if rerr != nil {
			return ports.Tokens{}, rerr
		}
		r.tokenURL = tokenURL
	}
	fresh, err := Refresh(ctx, RefreshConfig{
		ClientID:     r.ClientID,
		TokenURL:     r.tokenURL,
		HTTPClient:   r.HTTPClient,
		RefreshToken: cur.RefreshToken,
	})
	if err != nil {
		return ports.Tokens{}, err
	}
	if err := r.Store.Put(r.Slot, fresh); err != nil {
		return ports.Tokens{}, err
	}
	return fresh, nil
}
```

Test mit `keyringadapter.Fake` (existiert: `internal/adapter/keyringadapter/fake.go`) + httptest: Refresh-Roundtrip persistiert neue Tokens.

- [ ] **Step 3: httpsync.Client — 401 → Refresh → Retry once**

In `client.go`:

```go
// TokenRefresher renews the stored token bundle after a 401.
type TokenRefresher interface {
	RefreshTokens(ctx context.Context) (ports.Tokens, error)
}
```

Client-Struct: Feld `refresher TokenRefresher` + Setter `func (c *Client) SetRefresher(r TokenRefresher) { c.refresher = r }`. `do()` ersetzen:

```go
// do executes req with the bearer token; on 401 it refreshes once and
// retries. GetBody is set by http.NewRequest for bytes.Reader bodies, so
// the retry can replay POST/PUT payloads.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	token, err := c.bearer()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req = req.WithContext(ctx)
	resp, err := c.httpc.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || c.refresher == nil {
		return resp, err
	}
	fresh, rerr := c.refresher.RefreshTokens(ctx)
	if rerr != nil {
		return resp, nil // keep the original 401 → ErrUnauthorized upstream
	}
	_ = resp.Body.Close()
	retry := req.Clone(ctx)
	if req.GetBody != nil {
		b, gerr := req.GetBody()
		if gerr != nil {
			return nil, gerr
		}
		retry.Body = b
	}
	retry.Header.Set("Authorization", "Bearer "+fresh.AccessToken)
	return c.httpc.Do(retry)
}
```

Test in `client_test.go`: httptest-Server liefert beim ersten Call 401, beim zweiten 200; Fake-Refresher zählt Aufrufe; Assertion: genau 1 Refresh, Ergebnis ok. Plus Negativ-Test: ohne Refresher bleibt 401 → `ErrUnauthorized`.

- [ ] **Step 4: Wiring in main.go**

`Env`-Struct: Feld `OIDCClientID string` + in `main()`: `OIDCClientID: envOrDefault("FLOW_OIDC_CLIENT_ID", "flow-cli"),`. In `buildDeps` nach `syncClient := httpsync.NewClient(...)`:

```go
	syncClient.SetRefresher(&oidcclient.StoreRefresher{
		ServerURL: serverURL,
		ClientID:  env.OIDCClientID,
		Store:     keyring,
		Slot:      keyringSlot,
	})
```

ACHTUNG: `keyring`/`keyringSlot` werden aktuell NACH dem Identity-Block definiert (Zeile ~189) — der Refresher-Aufruf gehört direkt dahinter. (`flow-mcp` bleibt out of scope — eigener Tokens-Pfad.)

- [ ] **Step 5: Tests + Commit**

Run: `go build ./... && go test ./internal/adapter/oidcclient/ ./internal/adapter/httpsync/ ./cmd/flow/` → PASS

```bash
git add internal/adapter/oidcclient/ internal/adapter/httpsync/client.go internal/adapter/httpsync/client_test.go cmd/flow/login.go cmd/flow/main.go
git commit -m "feat(auth): wire token refresh into the sync client — expired tokens caused chronic 401 retry storms; refresh existed but had no caller"
```

---

### Task 15: Worker — Unauthorized leise behandeln, nicht spammen

Ohne Login feuert jeder Pull-Zyklus 5 `slog.Warn` (eine pro Ressource) und jeder Drain-Versuch `slog.Info` — Offline/ausgeloggt ist aber der NORMALE Zustand des Single-User-PoC. Nach Task 1 landet das nur noch in der Datei, müllt sie aber zu.

**Files:**
- Modify: `internal/adapter/httpsync/worker.go` (`runPull`, `classifyPushError`)
- Test: `internal/adapter/httpsync/worker_test.go`

- [ ] **Step 1: Failing Test**

`TestRunPullShortCircuitsWhenUnauthorized`: Client ohne Token (Fake-TokenStore liefert `ports.ErrTokenNotFound`) → `runPull` macht genau EINEN HTTP-/bearer-Versuch (Request-Counter am Fake), nicht fünf. (Zugriff: Test im Package `httpsync` kann `runPull` direkt rufen, wie bestehende Worker-Tests.)

Run → FAIL (5 Versuche)

- [ ] **Step 2: runPull ersetzen**

```go
func (w *Worker) runPull(ctx context.Context) {
	resources := []string{"projects", "sessions", "active_sessions"}
	if w.repos != nil {
		resources = append(resources, "repos")
	}
	if w.notes != nil {
		resources = append(resources, "repo_notes")
	}
	for _, res := range resources {
		if err := w.pullResource(ctx, res); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				// Logged-out is the normal offline state — one debug line,
				// skip the remaining resources (they fail identically).
				slog.Debug("sync: pull skipped — not logged in")
				return
			}
			slog.Warn("sync: pull "+res, slog.Any("err", err))
		}
	}
}
```

(Die fünf einzelnen if-Blöcke ersetzen; Verhalten für nicht-auth-Fehler unverändert.)

- [ ] **Step 3: classifyPushError — 401 auf Debug**

Den `slog.Info`-Block am Ende ersetzen:

```go
	level := slog.LevelInfo
	if errors.Is(err, ErrUnauthorized) {
		level = slog.LevelDebug // logged-out retries are routine, not noteworthy
	}
	slog.Log(context.Background(), level,
		"sync: transient push failure — scheduling retry",
		slog.String("resource", resource),
		slog.String("row_id", rowID),
		slog.Int64("seq", seq),
		slog.Any("err", err),
	)
	return DrainRetry, err
```

(`context`-Import existiert.) 401 bleibt DrainRetry — nach Task 14 heißt ein echtes 401 "Refresh-Token auch tot" → der User muss `flow login` ausführen; die Queue bleibt erhalten und drained nach dem Login.

- [ ] **Step 4: Tests + Commit**

Run: `go test ./internal/adapter/httpsync/` → PASS

```bash
git add internal/adapter/httpsync/worker.go internal/adapter/httpsync/worker_test.go
git commit -m "fix(httpsync): treat logged-out as a quiet normal state — pull short-circuits and 401 retries log at debug"
```

---

### Task 16: WebUI — SSE/HTMX bekommen 401 statt Landing-Redirect

`NewBrowserAuthMiddleware` (`internal/adapter/httpserver/middleware_browser.go:38-44`) 302-redirected JEDE unauthentifizierte Anfrage — auch `/api/v1/events` (EventSource folgt dem Redirect auf die HTML-Landing, htmx-sse verliert den Stream still; Dashboard friert nach Cookie-Ablauf (8h) ein).

**Files:**
- Modify: `internal/adapter/httpserver/middleware_browser.go`
- Modify: `internal/webui/templates/layout/base.templ` (+ `templ generate`)
- Test: `internal/adapter/httpserver/middleware_test.go` oder neue `middleware_browser_test.go`

- [ ] **Step 1: Failing Test**

```go
func TestBrowserMiddlewareReturns401ForSSEAndHTMX(t *testing.T) {
	// Setup wie der bestehende Browser-Middleware-Test (Fake BrowserSessionStore).
	cases := []struct {
		name   string
		header http.Header
	}{
		{"sse", http.Header{"Accept": []string{"text/event-stream"}}},
		{"htmx", http.Header{"Hx-Request": []string{"true"}}},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		req.Header = tc.header
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req) // handler = middleware(next) ohne Cookie
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", tc.name, rec.Code)
		}
	}
	// Browser-HTML-Request ohne Header bleibt 302 → LandingPath (Bestand).
}
```

Run → FAIL (302)

- [ ] **Step 2: Middleware**

In `middleware_browser.go`:

```go
// rejectUnauthenticated answers an unauthenticated request: machine-style
// clients (EventSource, HTMX partial swaps) get a plain 401 — a 302 to the
// HTML landing silently kills an SSE stream; humans get the redirect.
func rejectUnauthenticated(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") ||
		r.Header.Get("HX-Request") == "true" {
		http.Error(w, "session expired", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, LandingPath, http.StatusFound)
}
```

Beide Redirect-Stellen im Handler ersetzen durch `rejectUnauthenticated(w, r); return`. Import `strings` ergänzen.

- [ ] **Step 3: Client-seitiger Reload-Hook**

In `internal/webui/templates/layout/base.templ` vor `</body>` (genaue Stelle lesen):

```html
<script>
  // A dead session yields 401 on SSE/HTMX — reload so the browser-auth
  // middleware redirects to the landing page instead of freezing silently.
  document.body.addEventListener('htmx:sseError', function () { window.location.reload(); });
  document.body.addEventListener('htmx:responseError', function (e) {
    if (e.detail && e.detail.xhr && e.detail.xhr.status === 401) { window.location.reload(); }
  });
</script>
```

Dann `templ generate ./internal/webui/...` (bzw. das Makefile-Target: `rg -n "templ" Makefile | head`) — `base_templ.go` muss sich ändern.

- [ ] **Step 4: Tests + Commit**

Run: `go test ./internal/adapter/httpserver/ ./internal/webui/...` → PASS

```bash
git add internal/adapter/httpserver/middleware_browser.go internal/adapter/httpserver/*_test.go internal/webui/templates/layout/
git commit -m "fix(webui): 401 instead of landing-redirect for SSE/HTMX requests + client-side reload on dead session — dashboard froze silently after cookie expiry"
```

---

### Task 17: Wiring-Verification + End-to-End-Smoke (PFLICHT, nicht kürzen)

Per-Task-Tests fangen nicht "der Composition-Root ruft den neuen Constructor nie auf". Dieser Task verifiziert das Gesamtsystem.

**Files:** keine neuen — Verifikation.

- [ ] **Step 1: Volle CI lokal**

Run: `make ci`
Expected: grün (Lint + Tests + Coverage-Gate 55%). Fehler fixen, bevor es weitergeht.

- [ ] **Step 2: Composition-Root-Audit**

Checkliste gegen `cmd/flow/main.go` (jeden Punkt mit `rg` belegen):
- [ ] `setupLogging` wird in `main()` vor `buildDeps` gerufen
- [ ] `reader` hat `Active: cacheActiveSessions` + `UserID: localUser.ID`
- [ ] Worktime-Deps enthalten `ListActiveSessions`, `PauseMarker`, `CorrectActiveStart`
- [ ] `syncClient.SetRefresher(...)` ist verdrahtet
- [ ] Auto-Migration-Block existiert in `buildDeps`; der cobra-Guard ist weg
- [ ] `rg -n 'fmt\.Print|println\(' internal/adapter/httpsync/ internal/usecase/` → keine Treffer (nichts schreibt am slog vorbei auf stdout/stderr in Hintergrund-Pfaden)

- [ ] **Step 3: Offline-Smoke (kein Server, kein Login)**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
go build -o /tmp/flow-poc ./cmd/flow/
export FLOW_CACHE_DB=/tmp/flow-poc-test/cache.db
export FLOW_SERVER_URL=http://localhost:59999   # absichtlich tot
mkdir -p /tmp/flow-poc-test
/tmp/flow-poc worktime start --project testprojekt   # → "Worktime läuft seit HH:MM auf 'testprojekt'"
/tmp/flow-poc worktime status                          # → Segment zeigt laufende Session (Running-Indikator!)
cd /tmp && /tmp/flow-poc worktime stop                 # → "Gestoppt nach 0h 00m" TROTZ anderem cwd
/tmp/flow-poc worktime toggle                          # → "Worktime läuft seit ..." (Start)
/tmp/flow-poc worktime toggle                          # → "Gestoppt nach ..."
/tmp/flow-poc worktime pause                           # → kein Fehler (idle no-op)
/tmp/flow-poc worktime stats today                     # → zeigt die zwei Sessions
```

Kernchecks: (a) KEINE sync-Warnungen auf dem Terminal — `cat ~/.local/state/flow/flow.log` enthält sie stattdessen (bzw. mit FLOW_LOG_LEVEL unset nur Warn-Level); (b) Running-Indikator in `status` funktioniert; (c) stop ohne cwd-Abhängigkeit.

- [ ] **Step 4: Server-Roundtrip-Smoke**

flow-server lokal starten (Pattern aus `scripts/run-flow-server.sh` bzw. `scripts/smoke-m2-m3.sh` übernehmen — dort ist der OIDC-freie/Dex-Testmodus dokumentiert). Dann mit gesetztem FLOW_SERVER_URL: start → `flow sync status` (Queue drained, kein Halt) → stop → Queue wieder leer; auf der Server-DB prüfen, dass die finale Session Tag/Note + korrekte started_at trägt. Falls der Server lokal nur mit Dex-Container startet (`docker-compose` in `deploy/podman/`), diesen Schritt als Checkliste für Soenne dokumentieren statt blind zu skippen.

- [ ] **Step 5: TSV-Upgrade-Smoke**

```bash
rm -rf /tmp/flow-poc-test2 && mkdir -p /tmp/flow-poc-test2
printf '2026-06-01\t09:00\t10:30\t5400\tdeep\n' > /tmp/flow-poc-tsv.log
# worktime.log-Pfad ist ~/.tmux/worktime.log — Smoke daher mit echtem HOME-Sandbox:
HOME=/tmp/flow-poc-test2 sh -c 'mkdir -p $HOME/.tmux && cp /tmp/flow-poc-tsv.log $HOME/.tmux/worktime.log && FLOW_CACHE_DB=$HOME/cache.db FLOW_SERVER_URL=http://localhost:59999 /tmp/flow-poc worktime stats 2026'
```

Expected: Befehl läuft durch (kein Guard-Block), zeigt die migrierte Session, `$HOME/.tmux/worktime.log` wurde zu `worktime.log.migrated-*` umbenannt.

- [ ] **Step 6: TUI-Sichtprüfung (Soenne)**

Manuell (nicht automatisierbar): `flow worktime today` mit totem FLOW_SERVER_URL ≥2 Minuten offen lassen → keine Fremdzeichen/Korruption; `s` startet (Picker) und stoppt; Timer zählt sichtbar.

- [ ] **Step 7: Aufräumen + Commit**

```bash
rm -rf /tmp/flow-poc /tmp/flow-poc-test /tmp/flow-poc-test2 /tmp/flow-poc-tsv.log
git status   # muss clean sein (Verification ändert keinen Code; Fixes aus Step 1-5 wurden als eigene fix-Commits gemacht)
```

---

## Bewusst NICHT in diesem Plan (Phase 2)

- Sync-Status-Zeile in der TUI (tea.Msg-Kanal vom Worker statt nur File-Log) — Log-File reicht für PoC.
- `sqliteclient`/`sqliteserver`-Dedup, `conflict_overlay`→`modal.Render`, `viewContent`-Dreifachkopie, `mondayOf`-Duplikate — Cleanup-Findings ohne PoC-Impact.
- Multi-Session-Auswahl beim TUI-Stop (`s` stoppt die früheste).
- flow-mcp Token-Refresh.
- WebUI Sliding-Session-Cookie (Reload-Hook reicht für PoC).

