# R2a — Worktime-Client auf httpapi (Server-only) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development
> (ein frischer Subagent pro Task, Model **Sonnet oder kleiner**). Steps nutzen
> Checkbox-Syntax (`- [x]`) und werden in DIESER Datei abgehakt.

**Spec:** `docs/superpowers/specs/2026-06-11-flow-server-only-rebuild-design.md` (§5, §8, §13 R2; A1 abgenommen). R2 ist in zwei Pläne geschnitten: **R2a (dieser)** = Worktime/Identity/Statuszeile/Sync-Stack-Löschung/flow-mcp-Minimal-Swap. **R2b** (eigene Datei) = Kompendium-Client. Das Dogfood-Gate (§14) liegt NACH R2b.

**Goal:** Der flow-Client (TUI/CLI) und flow-mcp sprechen für Worktime, Projekte, Day-Offs,
Settings, Repo-Notes und Identität ausschließlich die Server-API (`internal/adapter/httpapi`).
sqliteclient, httpsync, flockstate, conflict_overlay und das local-User/Adoption-Konzept sind
ersatzlos gelöscht. Eine Statuszeile zeigt überall Online/Offline/Login-Zustand.

**Architecture:** Hexagonal, Adapter-Swap in `cmd/flow/main.go` + `cmd/flow-mcp/main.go`.
Neuer Adapter `internal/adapter/httpapi`: typisierter REST-Client (Kern aus `httpsync/client.go`
übernommen: Bearer aus Keyring-Slot `tokens:<serverURL>` + 401-Refresh via
`oidcclient.StoreRefresher`), pro Resource In-Memory-Cache mit SSE-Invalidierung
(`/api/v1/events`, `changed`-Events) + Fallback-Poll, Offline-Read-Snapshot unter
`$XDG_STATE_HOME/flow/snapshot.json`. Writes sind synchron (kein Queuing). Die
Statemachine-Writes (start/stop/pause/resume/correct) laufen über einen NEUEN schmalen Port
`ports.WorktimeMachine` — das alte `ActiveSessionStore.Upsert/Delete` passt semantisch nicht
auf Server-Endpoints (bewusste Plan-Zeit-Entscheidung, Spec §5 nennt die Ports nur grob).

**Tech Stack:** Go 1.25, stdlib net/http (kein SSE-Framework — Zeilen-Parser), bubbletea v2,
testcontainers-PG + echte httpserver-Handler für Contract-Tests (Spec §14).

**Arbeitsverzeichnis:** `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1` (Branch `next`).

---

## Executor-Protokoll (Sonnet-Subagents)

Identisch zu R1b (`2026-06-11-flow-r1b-document-revisions.md`), Kurzform:

1. Pro Task EIN frischer Subagent; Tasks strikt in Reihenfolge; Checkboxen in DIESER Datei pflegen.
2. Expected-Mismatch = beheben wenn im Task-Scope, sonst Abweichung notieren + STOPP.
3. Code-Blöcke sind die Wahrheit; nur mechanische Compiler-Fixes erlaubt (als Abweichung notieren).
4. Vor jedem Commit `gofumpt -w` auf geänderte .go-Dateien.
5. NIEMALS pushen, Branch wechseln, CLAUDE-*.md anfassen. Commit-Trailer:
   `Co-Authored-By: Claude <noreply@anthropic.com>`.
6. Vor JEDEM `go test`-Lauf in einer Session:
   ```bash
   export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
   export TESTCONTAINERS_RYUK_DISABLED=true
   ```
7. `make ci` nur wo der Task es sagt — in detached tmux, Exit aus `ci.status`-Datei
   (Muster siehe R1b Task 4 Step 1), NIE aus einer Pipe.

---

## File-Map (Endzustand R2a)

**Neu — `internal/adapter/httpapi/`:**

| Datei | Verantwortung |
|---|---|
| `client.go` | REST-Kern: do() mit Bearer + 401-Refresh-Retry, X-Flow-Client-Version/-Device, Error-Mapping, ErrNotConfigured/ErrUnavailable/ErrUnauthorized |
| `retry.go` | Backoff (kopiert aus httpsync/retry.go — Quelle wird in Task 12 gelöscht) |
| `dto.go` | Wire-DTOs (sessionDTO, activeDTO, projectDTO, dayOffDTO, documentDTO/entryDTO, metaDTO) + domain-Konverter |
| `cache.go` | Per-Resource-Cache + Snapshot-Persistenz (`$XDG_STATE_HOME/flow/snapshot.json`) |
| `status.go` | ConnState (Online/Offline/LoggedOut/NotConfigured/Outdated) + Statusschnappschuss für die UI + Changed-Kanal |
| `meta.go` | GET /meta + Versionsvergleich (dotted numeric) |
| `identity.go` | GET /me-bearer → domain.User (gecacht); LoggedOut-Erkennung |
| `sessions.go` | ports.SessionStore (Load/LoadFiltered/Upsert/UpsertBatch/Delete) |
| `active_sessions.go` | ports.ActiveSessionStore — NUR Reads (ListByUser/Get); Upsert/Delete returnen Fehler „use WorktimeMachine" |
| `machine.go` | ports.WorktimeMachine: Start/Stop/Pause/Resume/CorrectStart via POST-Endpoints |
| `projects.go` | ports.ProjectStore (EnsureBySlug = List+POST; TouchLastUsed = No-op, Server macht es bei start) |
| `dayoffs.go` | ports.DayOffStore (Jahres-GETs, fehlertolerant per Snapshot) |
| `settings.go` | Settings-GET/PUT + ConfigReader-Compose über iniconfig (daily_target/timezone vom Server, Rest lokal) |
| `documents.go` | ports.DocumentStore inkl. Repo-Note-Alias (PUT /repos/{key}/note) |
| `events.go` | SSE-Client: changed→Invalidate+Changed-Kanal, Reconnect mit Backoff + Refetch, Fallback-Poll 45 s |
| `main_test.go` + `*_test.go` | Contract-Tests gegen ECHTE httpserver-Handler (pgtest-Container + oidctest-Issuer) |

**Neu — sonstige:**

| Datei | Verantwortung |
|---|---|
| `internal/ports/worktime_machine.go` | Port WorktimeMachine + Doku |
| `internal/frontend/tui/components/statusbar/connstate.go` (+`_test.go`) | Statuszeilen-Primitiv: `● host` / `○ offline · Stand 14:32 (read-only)` / `○ nicht angemeldet · flow login` / `▲ Client veraltet` |
| `internal/adapter/httpserver/worktime_api_correct.go` (Erweiterung in worktime_api.go erlaubt) | POST /worktime/active/correct (Server-Ergänzung, siehe Entscheidungen) |

**Modifiziert (zentral):** `cmd/flow/main.go` (buildDeps-Rewrite), `cmd/flow-mcp/main.go`,
`Makefile` (ldflags-Version für flow + flow-mcp), `internal/usecase/active_sessions.go`,
`internal/usecase/worktime_reader.go`, `internal/usecase/identity.go`,
`internal/frontend/tui/screen/worktime/{model.go,today_actions.go,today_project_picker.go}`,
`internal/frontend/tui/sidekick/model.go`, `internal/frontend/cli/worktime.go`,
`cmd/flow/login.go`, `cmd/flow/whoami.go`.

**Gelöscht (Task 12):** `internal/adapter/httpsync/`, `internal/adapter/sqliteclient/`,
`internal/adapter/flockstate/`, `internal/frontend/tui/components/conflict_overlay/`,
`internal/frontend/tui/screen/worktime/conflicts.go` (+`conflicts_test.go`),
`internal/frontend/cli/sync.go` (+ Tests), `internal/frontend/cli/brief_conflict_test.go`,
`internal/usecase/sync_status.go`, `internal/ports/sync.go`, `ports.LegacyActiveStore` +
`ports.PauseStore` + `ports.Lock` (aus `internal/ports/sessions.go`/`pause.go`),
Adoption: `tryAdoptLocalProfile` in `cmd/flow/login.go:87–140`,
`usecase.Identity.AdoptLocalDataIfFirstLogin`/`localSub`, Env `FLOW_LOCAL_USER_SUB` +
`FLOW_CACHE_DB`.

**Bleibt:** `iniconfig` (Wochentags-/Tag-Targets lokal, Phase 1), `linkstsv`
(worktime↔notes-Links sind lokales TSV, kein Server-Modell in Phase 1 — bewusst),
`dayoffstsv` NUR als Lese-Quelle für `flow worktime dayoff sync` (Feiertags-Import) und
R4-TSV-Migration; `jsonflowstate`, `keyringadapter`, `oidcclient`, `tmuxbridge`,
`cheatsheetfs`, `fspaletteentries`, `jsonpalettestats`, `fssourcedirs`, kompendium-Teile (R2b).

---

## Plan-Zeit-Entscheidungen (im Code kommentieren, Spec §15.7-Geist)

1. **`ports.WorktimeMachine` statt ActiveSessionStore-Writes.** Server-Statemachine
   (start/stop/pause/resume) ist nicht Upsert-förmig; ein Upsert-Mapping würde die
   409/404-Semantik verstecken. ActiveSessionStore bleibt als Read-Port.
2. **Server-Ergänzung `POST /api/v1/worktime/active/correct`** `{project_id, started_at}`:
   `flow worktime correct` (CorrectStart) hat sonst keinen Server-Pfad. Statemachine-Methode
   + Handler + Tests gehören zum Server-Teil dieses Plans (Task 4).
3. **TouchLastUsed beim Start ist Server-Sache:** der Start-Handler ruft
   `projects.TouchLastUsed` (Task 4 verifiziert das und ergänzt es, falls es fehlt) —
   Client-`TouchLastUsed` wird No-op. MRU im Picker bleibt damit geräteübergreifend korrekt.
4. **Session-Delete braucht Version (If-Match):** `ports.SessionStore.Delete(userID, id)` hat
   keine. Der Adapter löst über den Sessions-Cache auf (Load merkt sich Versionen); Cache-Miss
   ⇒ gezielter GET. Dokumentiert in `sessions.go`.
5. **Settings-Compose:** Server-`/settings` kennt nur `daily_target` + `timezone`
   (Server-Validatoren!). Wochentags-/Tag-Targets + max_streak bleiben in `worktime.conf`
   (lokal, Phase 1). `ConfigReader` liefert die Server-Werte mit ini-Fallback; beim ersten
   erfolgreichen Load mit leeren Server-Settings werden daily_target/timezone EINMALIG aus der
   ini gesät (idempotent, geloggt) — sonst stimmt das WebUI-Saldo bis R4 nicht.
6. **`flow worktime pause/resume`** wechseln von „Stop+Marker" auf die Server-Pause
   (eine Statemachine für alle Geräte — der eigentliche Sinn von R2).
7. **Fallback-Poll 45 s** (Spec sagt 30–60): fix verdrahtet, kein Config-Knopf (YAGNI).

---

### Task 0: Preflight

**Files:** keine.

- [x] **Step 1: Worktree clean auf `next`, R1b ist drauf**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
git status --short && git log --oneline -6 | rg "R1b|97f7fab|Revision" | head -3
```

Expected: clean; mindestens ein R1b-Commit sichtbar (`feat(pgstore): Revision bei jedem
documents-Write` — R1b MUSS vor R2a abgeschlossen sein). Wenn nicht: STOPP.

- [x] **Step 2: podman-Exports + Baseline**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go build ./... && go test ./internal/adapter/httpserver/ -count=1 2>&1 | tail -1
```

Expected: Build ok, `ok …/httpserver`.

- [x] **Step 3: httpserver-Test-Bootstrap kartieren (für Task 1 Step 1)**

```bash
rg -n "func TestMain|func newTestServer|oidctest|bearerToken|mintToken" internal/adapter/httpserver/*_test.go | head -15
```

Expected: Treffer, die zeigen, wie httpserver-Tests PG-Container + Test-Issuer + Token-Minting
aufsetzen. Die GENAUEN Helper-Namen hier als Notiz unter diesem Step eintragen — Task 1
kopiert dieses Muster in `httpapi/main_test.go`.

Kein Commit.

---

### Task 1: httpapi-Kern — client.go, retry.go, dto.go + Test-Bootstrap

**Files:**
- Create: `internal/adapter/httpapi/client.go`, `retry.go`, `dto.go`, `doc.go`, `main_test.go`, `client_test.go`

- [x] **Step 1: Test-Bootstrap `main_test.go`**

Muster aus `internal/adapter/httpserver`-Tests übernehmen (Task 0 Step 3): TestMain startet
EINEN pgtest-Container; ein Helper baut pro Test einen `httptest.Server` mit
`httpserver.NewWithAuth(...)` (echte Handler, echter pgstore, oidctest-Issuer) und liefert
`(*httpapi.Client, baseURL, mintToken func(sub string) string)`. Tokens landen in einem
In-Memory-TokenStore-Fake:

```go
// internal/adapter/httpapi/main_test.go (Struktur — Helper-Namen exakt aus dem
// httpserver-Testpaket übernehmen, siehe Task-0-Notiz)
package httpapi_test

type memTokens struct{ t ports.Tokens; ok bool }

func (m *memTokens) Get(string) (ports.Tokens, error) {
	if !m.ok {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	return m.t, nil
}
func (m *memTokens) Put(_ string, t ports.Tokens) error { m.t, m.ok = t, true; return nil }
func (m *memTokens) Delete(string) error                { m.ok = false; return nil }
```

Jeder Test: `srv := newTestAPI(t)` → echte Routen; `cli := httpapi.New(httpapi.Config{BaseURL: srv.URL, Tokens: &memTokens{...gültiges Token...}, Version: "9.9.9"})`.

- [x] **Step 2: `client.go` — der Kern**

```go
// Package httpapi implements the client-side ports against flow-server's
// /api/v1 REST surface. One truth (the server), thin clients (Spec §5/§8):
// reads go through a cache fed by SSE invalidation, writes are synchronous.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// Sentinel errors the UI layers translate into banners/hints.
var (
	ErrNotConfigured = errors.New("httpapi: FLOW_SERVER_URL nicht gesetzt")
	ErrLoggedOut     = errors.New("httpapi: nicht angemeldet — flow login")
	ErrUnavailable   = errors.New("httpapi: server nicht erreichbar")
)

// TokenRefresher matches oidcclient.StoreRefresher.
type TokenRefresher interface {
	RefreshTokens(ctx context.Context) (ports.Tokens, error)
}

type Config struct {
	BaseURL   string           // "" => ErrNotConfigured auf jedem Call
	Tokens    ports.TokenStore // keyringadapter; Slot "tokens:"+BaseURL
	Slot      string
	Refresher TokenRefresher // optional (flow-mcp: nil)
	Version   string         // ldflags-Client-Version, "dev" wenn leer
	Device    string         // Hostname für X-Flow-Device
	HTTPC     *http.Client   // optional, default 15s Timeout
}

type Client struct {
	base      string
	tokens    ports.TokenStore
	slot      string
	refresher TokenRefresher
	version   string
	device    string
	httpc     *http.Client
	status    *Status // Task 2
}

func New(c Config) *Client {
	httpc := c.HTTPC
	if httpc == nil {
		httpc = &http.Client{Timeout: 15 * time.Second}
	}
	version := c.Version
	if version == "" {
		version = "dev"
	}
	device := c.Device
	if device == "" {
		device, _ = os.Hostname()
	}
	return &Client{
		base: c.BaseURL, tokens: c.Tokens, slot: c.Slot,
		refresher: c.Refresher, version: version, device: device,
		httpc: httpc, status: newStatus(),
	}
}

func (c *Client) bearer() (string, error) {
	t, err := c.tokens.Get(c.slot)
	if errors.Is(err, ports.ErrTokenNotFound) {
		return "", ErrLoggedOut
	}
	if err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

// doJSON führt einen Request aus: Bearer + Pflicht-Header, EIN transparenter
// Retry nach Token-Refresh bei 401 (Muster aus httpsync), Status-Mapping.
// out == nil ⇒ Body wird verworfen. ifMatch >= 0 ⇒ If-Match-Header.
func (c *Client) doJSON(ctx context.Context, method, path string, body any, ifMatch int64, out any) error {
	if c.base == "" {
		return ErrNotConfigured
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	mk := func() (*http.Request, error) {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if ifMatch >= 0 {
			req.Header.Set("If-Match", fmt.Sprintf("%d", ifMatch))
		}
		req.Header.Set("X-Flow-Client-Version", c.version)
		req.Header.Set("X-Flow-Device", c.device)
		return req, nil
	}
	tok, err := c.bearer()
	if err != nil {
		c.status.setLoggedOut()
		return err
	}
	req, err := mk()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.httpc.Do(req)
	if err != nil {
		c.status.setOffline()
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode == http.StatusUnauthorized && c.refresher != nil {
		_ = resp.Body.Close()
		fresh, rerr := c.refresher.RefreshTokens(ctx)
		if rerr != nil {
			c.status.setLoggedOut()
			return ErrLoggedOut
		}
		if req, err = mk(); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+fresh.AccessToken)
		if resp, err = c.httpc.Do(req); err != nil {
			c.status.setOffline()
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		c.status.setLoggedOut()
		return ErrLoggedOut
	case resp.StatusCode >= 500:
		c.status.setOffline()
		return fmt.Errorf("%w: server %d", ErrUnavailable, resp.StatusCode)
	}
	c.status.setOnline(c.base)
	if resp.StatusCode >= 400 {
		return &StatusError{Code: resp.StatusCode, Body: raw}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// StatusError transportiert 4xx-Antworten zu den Resource-Adaptern, die sie
// in ports-Sentinels übersetzen (404→NotFound, 409/412→Conflict, 403→Fehlertext).
type StatusError struct {
	Code int
	Body []byte
}

func (e *StatusError) Error() string { return fmt.Sprintf("httpapi: status %d: %s", e.Code, e.Body) }

func statusCode(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return 0
}
```

- [x] **Step 3: `retry.go`** — `Backoff`-Struct 1:1 aus `internal/adapter/httpsync/retry.go`
  kopieren (Paketname auf `httpapi` ändern, Kommentar „kopiert aus httpsync (R2a), Quelle
  gelöscht" ergänzen). Den zugehörigen Test aus `retry_test.go` mitkopieren.

- [x] **Step 4: `dto.go`** — Wire-Typen EXAKT wie der Server (Quelle: `worktime_api.go:81/101/120/129`, `projects_api.go:44`, `documents_api.go:45/98`, `dayoffs_settings_api.go:50`, `meta.go:8`):

```go
package httpapi

import "time"

type sessionDTO struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Day       string    `json:"day"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type sessionWriteDTO struct {
	ID        string    `json:"id,omitempty"`
	ProjectID string    `json:"project_id"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
}

type activeDTO struct {
	ProjectID       string     `json:"project_id"`
	StartedAt       time.Time  `json:"started_at"`
	PausedAt        *time.Time `json:"paused_at"`
	PauseTotalMS    int64      `json:"pause_total_ms"`
	StartedOnDevice string     `json:"started_on_device"`
	Tag             string     `json:"tag"`
	Note            string     `json:"note"`
	Version         int64      `json:"version"`
}

type projectDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Slug       string     `json:"slug"`
	ArchivedAt *time.Time `json:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
	Version    int64      `json:"version"`
}

type dayOffDTO struct {
	Day    string `json:"day"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Target string `json:"target,omitempty"`
}

type documentDTO struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

type entryDTO struct {
	Path      string `json:"path"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Snippet   string `json:"snippet,omitempty"`
}

type metaDTO struct {
	ServerVersion    string `json:"server_version"`
	MinClientVersion string `json:"min_client_version"`
}

type itemsEnvelope[T any] struct {
	Items []T `json:"items"`
}
```

Dazu in `dto.go` die Konverter `sessionFromDTO(d sessionDTO) domain.Session`,
`activeFromDTO(d activeDTO, userID string) domain.ActiveSession`,
`projectFromDTO(d projectDTO, userID string) domain.Project`,
`dayOffFromDTO(d dayOffDTO) (domain.DayOff, error)` — Feldzuordnung anhand der
domain-Structs (vor dem Schreiben `rg -n "type Session struct|type ActiveSession struct|type Project struct|type DayOff struct" internal/domain/` und Felder exakt mappen; `Day` parsen mit `time.DateOnly`, `PauseTotalMS` → `time.Duration(ms)*time.Millisecond`).

- [x] **Step 5: `client_test.go`** — drei Contract-Tests gegen den echten Server-Stack:
  (a) gültiges Token → GET /api/v1/me-bearer liefert 200 (über `doJSON` mit out-map);
  (b) leerer TokenStore → `ErrLoggedOut`; (c) BaseURL "" → `ErrNotConfigured`;
  (d) Server gestoppt (httptest.Server.Close vor Call) → `ErrUnavailable`.

- [x] **Step 6: Build + Tests + Commit**

```bash
go build ./... && go test ./internal/adapter/httpapi/ -count=1 2>&1 | tail -2
```

Expected: `ok`. Commit:

```bash
git add internal/adapter/httpapi/ && git commit -m "$(cat <<'EOF'
feat(httpapi): REST-Kern — Bearer+Refresh, Pflicht-Header, Error-Mapping (R2a)

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Status, Meta-Handshake, Cache + Offline-Snapshot

**Files:**
- Create: `internal/adapter/httpapi/status.go`, `meta.go`, `cache.go` (+ `status_test.go`, `cache_test.go`)

- [x] **Step 1: `status.go`**

```go
package httpapi

import (
	"sync"
	"time"
)

type ConnState int

const (
	StateUnknown ConnState = iota
	StateOnline
	StateOffline
	StateLoggedOut
	StateNotConfigured
	StateOutdated // Client < min_client_version
)

type StatusSnapshot struct {
	State         ConnState
	Host          string    // Server-Host für die Statuszeile
	LastFetched   time.Time // jüngster erfolgreicher Read (für „Stand 14:32")
	ServerVersion string
}

// Status ist von allen Resource-Adaptern geteilt; UI liest Snapshot(),
// Änderungen wecken den Changed-Kanal (coalesced, cap 1).
type Status struct {
	mu      sync.Mutex
	snap    StatusSnapshot
	changed chan struct{}
}

func newStatus() *Status { return &Status{changed: make(chan struct{}, 1)} }

func (s *Status) Snapshot() StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

func (s *Status) Changed() <-chan struct{} { return s.changed }

func (s *Status) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Status) set(mut func(*StatusSnapshot)) {
	s.mu.Lock()
	before := s.snap
	mut(&s.snap)
	after := s.snap
	s.mu.Unlock()
	if before != after {
		s.notify()
	}
}

func (s *Status) setOnline(host string) {
	s.set(func(sn *StatusSnapshot) {
		if sn.State != StateOutdated { // Outdated bleibt kleben bis Neustart
			sn.State = StateOnline
		}
		sn.Host = host
		sn.LastFetched = time.Now()
	})
}
func (s *Status) setOffline()   { s.set(func(sn *StatusSnapshot) { sn.State = StateOffline }) }
func (s *Status) setLoggedOut() { s.set(func(sn *StatusSnapshot) { sn.State = StateLoggedOut }) }

// StatusOf exponiert den Tracker für main.go/TUI.
func (c *Client) StatusOf() *Status { return c.status }
```

- [x] **Step 2: `meta.go`** — `(c *Client) CheckMeta(ctx)` ruft GET `/api/v1/meta` OHNE
  Bearer (Route ist public — eigener kleiner Request, nicht doJSON), füllt
  `status.snap.ServerVersion`, vergleicht `c.version` gegen `min_client_version` mit
  dotted-numeric-Vergleich (eigene 15-Zeilen-Funktion `versionLess(a, b string) bool`,
  "dev" gilt immer als aktuell) und setzt ggf. `StateOutdated`. Test: Tabelle für
  versionLess (1.2.3 < 1.10.0, dev nie outdated, gleiche Version nicht outdated).

- [x] **Step 3: `cache.go`** — generischer Resource-Cache + JSON-Snapshot:

```go
package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// resourceCache hält die letzte gute Server-Antwort einer Resource und
// persistiert sie als Teil des Offline-Snapshots (Spec §8). Invalidate()
// erzwingt beim nächsten Read einen Refetch.
type resourceCache[T any] struct {
	mu    sync.Mutex
	val   T
	ok    bool
	stale bool
}

func (r *resourceCache[T]) get() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ok || r.stale {
		var zero T
		return zero, false
	}
	return r.val, true
}

func (r *resourceCache[T]) put(v T) {
	r.mu.Lock()
	r.val, r.ok, r.stale = v, true, false
	r.mu.Unlock()
}

func (r *resourceCache[T]) invalidate() { r.mu.Lock(); r.stale = true; r.mu.Unlock() }

// fallback liefert den letzten bekannten Wert auch wenn stale — für
// Offline-Reads (Server weg ⇒ lieber alt als nichts, UI zeigt das Banner).
func (r *resourceCache[T]) fallback() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.val, r.ok
}

// Snapshot ist die Platte-Repräsentation unter $XDG_STATE_HOME/flow/snapshot.json.
type Snapshot struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Sessions  []sessionDTO      `json:"sessions"`
	Active    []activeDTO       `json:"active"`
	Projects  []projectDTO      `json:"projects"`
	DayOffs   []dayOffDTO       `json:"day_offs"`
	Settings  map[string]string `json:"settings"`
}

func snapshotPath() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "flow", "snapshot.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "flow", "snapshot.json")
}

func saveSnapshot(s Snapshot) error {
	p := snapshotPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func loadSnapshot() (Snapshot, bool) {
	b, err := os.ReadFile(snapshotPath())
	if err != nil {
		return Snapshot{}, false
	}
	var s Snapshot
	if json.Unmarshal(b, &s) != nil {
		return Snapshot{}, false
	}
	return s, true
}
```

Die Resource-Adapter (Tasks 3/5) schreiben nach jedem erfolgreichen Voll-Read ihren Teil in
den Snapshot (load→mutate→save, best effort, Fehler nur loggen) und lesen ihn als
Offline-Fallback, wenn `ErrUnavailable` UND der In-Memory-Cache leer ist. `status.LastFetched`
speist „Stand 14:32". Tests: put/get/invalidate/fallback; save/load-Roundtrip mit
`t.Setenv("XDG_STATE_HOME", t.TempDir())`.

- [x] **Step 4: Build + Tests + Commit** (`feat(httpapi): Status/Meta/Cache + Offline-Snapshot (R2a)`)

---

### Task 3: Read-Adapter — Sessions, Active, Projects, DayOffs, Settings, Identity

**Files:**
- Create: `internal/adapter/httpapi/{sessions.go,active_sessions.go,projects.go,dayoffs.go,settings.go,identity.go}` + je `_test.go`

Gemeinsames Muster pro Adapter: Konstruktor `NewX(c *Client) *X`, Compile-Assertion
`var _ ports.X = (*X)(nil)`, Reads über Cache (Cache-Hit ⇒ kein Request; SSE/Writes
invalidieren), bei `ErrUnavailable` Fallback Cache→Snapshot, ALLE userID-Parameter werden
ignoriert (der Server scoped über das Token — Kommentar an jedem Adapter-Kopf).

- [x] **Step 1: `sessions.go`** — `Load(userID)`: GET
  `/api/v1/worktime/sessions?from=2000-01-01&to=2100-12-31` → `itemsEnvelope[sessionDTO]` →
  Konverter; merkt sich `map[id]version` für Delete/Upsert (Entscheidung 4). `LoadFiltered`:
  Load + Filter im Client. `Upsert(s)`: Version aus Map; `>0` ⇒ PUT `/sessions/{id}` mit
  If-Match, sonst POST `/sessions` (Server vergibt ID — Rückgabe-ID in die Map). 412 ⇒
  `ports.ErrSessionVersionConflict`, 404 ⇒ `ports.ErrSessionNotFound` (via `statusCode(err)`).
  `UpsertBatch` ⇒ POST `/sessions:bulk` `{"sessions":[…]}` (IDs mitsenden). `Delete` ⇒ DELETE
  mit If-Match aus Map; Miss ⇒ vorher Load. Jeder Write: Cache invalidieren.
- [x] **Step 2: `active_sessions.go`** — `ListByUser` ⇒ GET `/worktime/active` (Cache).
  `Get(userID, projectID)` ⇒ ListByUser + Filter, sonst `ports.ErrActiveSessionNotFound`.
  `Upsert`/`Delete` ⇒ `errors.New("httpapi: ActiveSessions sind read-only — WorktimeMachine benutzen")`.
- [x] **Step 3: `projects.go`** — ListActive ⇒ GET `/projects`; ListAll ⇒ `?all=1`;
  GetByID/GetBySlug aus ListAll-Cache (404-Fallback: Refetch einmal); EnsureBySlug ⇒
  GetBySlug, bei NotFound POST `{name, slug}`; Upsert ⇒ PUT `{name, slug, archived}` +
  If-Match(p.Version), 412 ⇒ `ports.ErrProjectVersionConflict`; Archive ⇒ GetByID + PUT
  archived=true; TouchLastUsed ⇒ No-op mit Kommentar (Entscheidung 3).
- [x] **Step 4: `dayoffs.go`** — interner Jahres-Cache `map[int][]domain.DayOff`. `List(from,to)`
  lädt fehlende Jahre via GET `/day-offs?year=`; Fehler ⇒ slog.Warn + Snapshot-Fallback
  (Port hat keinen error-Rückgabewert — Kommentar!). Lookup ⇒ List(date,date). Add ⇒ PUT
  `/day-offs/{date}` mit dayOffDTO (Target als Go-Duration-String wenn >0); AddBatch ⇒
  Schleife; Remove ⇒ DELETE. Writes invalidieren das Jahr.
- [x] **Step 5: `settings.go`** — `Settings`-Typ mit `Get(ctx) (map[string]string, error)` /
  `Put(ctx, map[string]string) error` gegen `/settings`. Dazu `NewConfigReader(c *Client, ini ports.ConfigReader) ports.ConfigReader`:
  Load() = ini.Load(); wenn Server liefert: `daily_target` → Config-Tagesziel,
  `timezone` mitnehmen falls das domain.Config-Feld existiert (vorher prüfen:
  `rg -n "type Config struct" internal/domain/ -A 12`); wenn Server-Map LEER:
  einmaliges Seed-PUT aus ini-Werten (Entscheidung 5), slog.Info dazu.
- [x] **Step 6: `identity.go`** — `Identity`-Typ: `Me(ctx) (domain.User, error)` ⇒ GET
  `/me-bearer` (gecacht bis Logout/401); mappt auf domain.User (Felder vorher prüfen).
- [x] **Step 7: Contract-Tests** — pro Adapter gegen den echten Test-Stack aus Task 1:
  Sessions CRUD-Roundtrip inkl. 412-Mapping; Active read-only-Fehler; Projects
  EnsureBySlug-Idempotenz + Archive; DayOffs Add/List/Remove-Roundtrip; Settings
  Seed-bei-leer + Overlay; Identity Me. Offline-Pfad je einmal: Server.Close() ⇒ Read liefert
  Snapshot-Stand (vorher einen erfolgreichen Read machen).
- [x] **Step 8: Build + Tests + Commit** (`feat(httpapi): Read/Write-Adapter für Worktime-Ports (R2a)`)

---

### Task 4: WorktimeMachine + Server-Ergänzungen (correct, TouchLastUsed)

**Files:**
- Create: `internal/ports/worktime_machine.go`, `internal/adapter/httpapi/machine.go` (+Test)
- Modify: `internal/adapter/pgstore/active_sessions.go` (+Test), `internal/adapter/httpserver/worktime_api.go` (+Test), `scripts/smoke-r1-routes.sh`

- [x] **Step 1: Port**

```go
// internal/ports/worktime_machine.go
package ports

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// WorktimeMachine ist die Client-Sicht auf die Server-Statemachine
// (Spec §7): synchrone Writes, Konflikte kommen als Sentinels zurück.
// Reads laufen weiter über ActiveSessionStore.
type WorktimeMachine interface {
	Start(projectID, tag, note string) (domain.ActiveSession, error) // 409 ⇒ ErrActiveSessionConflict
	Stop(projectID string) (domain.Session, error)                   // 404 ⇒ ErrActiveSessionNotFound
	Pause(projectID string) (domain.ActiveSession, error)
	Resume(projectID string) (domain.ActiveSession, error)
	CorrectStart(projectID string, startedAt time.Time) (domain.ActiveSession, error)
}
```

- [x] **Step 2: Server — Statemachine-Methode CorrectStart in pgstore** (`active_sessions.go`):
  `CorrectStart(userID, projectID string, startedAt time.Time)` — UPDATE von `started_at`
  (+ version+1) WHERE user+project, Validierung `startedAt` nicht in der Zukunft und nicht
  nach now; Fehlen ⇒ `ports.ErrActiveSessionNotFound`. Failing-Test zuerst (Tabellen-Test im
  Stil der bestehenden Statemachine-Tests in `active_sessions_test.go`).
- [x] **Step 3: Server — Handler `POST /api/v1/worktime/active/correct`** in
  `worktime_api.go`: Body `{project_id, started_at}`; 404/422-Semantik wie die Nachbarn;
  Route im selben Mount registrieren; `sse.Changed(userID, "worktime")` wie start/stop.
  Router-Test im Stil der bestehenden (`worktime_api`-Tests). Smoke-Script: eine
  `check POST /api/v1/worktime/active/correct 401`-Zeile in `scripts/smoke-r1-routes.sh`
  ergänzen (Abschnitt der active-Routen).
- [x] **Step 4: Server — TouchLastUsed beim Start verifizieren/ergänzen**

```bash
rg -n "TouchLastUsed" internal/adapter/httpserver/ internal/adapter/pgstore/
```

Wenn der Start-Handler (`handleActiveStart`) es NICHT ruft: nach erfolgreichem Start
`_ = deps.Projects.TouchLastUsed(userID, projectID)` ergänzen (Fehler nur loggen) + Test,
dass `last_used_at` nach Start gesetzt ist. Wenn es schon da ist: Checkbox abhaken + Notiz.

- [x] **Step 5: Client — `machine.go`**: POSTs auf `/worktime/active/{start,stop,pause,resume,correct}`
  mit `projectIDBody`/correct-Body; Mapping 409 ⇒ `ports.ErrActiveSessionConflict`,
  404 ⇒ `ports.ErrActiveSessionNotFound`; Start sendet tag/note; Stop liefert die
  geschriebene `domain.Session` zurück; jeder Erfolg invalidiert Active- + Sessions-Cache.
  Contract-Tests: Start→Pause→Resume→Stop-Roundtrip, Doppel-Start ⇒ Conflict,
  Stop-ohne-Start ⇒ NotFound, CorrectStart verschiebt.
- [x] **Step 6: Build + ALLE Server-Tests + Commit**

```bash
go build ./... && go test ./internal/adapter/pgstore/ ./internal/adapter/httpserver/ ./internal/adapter/httpapi/ -count=1 2>&1 | tail -3
```

Commit: `feat(api,httpapi): WorktimeMachine — Statemachine-Port + active/correct (R2a)`

---

### Task 5: Documents-Adapter (Repo-Notes + MCP-Basis)

**Files:**
- Create: `internal/adapter/httpapi/documents.go` (+Test)

- [x] **Step 1:** `ports.DocumentStore`-Implementierung: Get ⇒ GET `/documents/{path}`;
  GetByRepoKey ⇒ GET `/repos/{url.PathEscape(key)}/note`; List ⇒ GET `/documents?prefix=&q=&limit=`;
  Put ⇒ repoKey == "" ? PUT `/documents/{path}` : PUT `/repos/{key}/note` (Body `{"body": …}`,
  If-Match); Delete ⇒ DELETE `/documents/{path}` + If-Match aus vorherigem Get (Server
  verlangt If-Match auf documents-DELETE — prüfen via `rg -n "ifMatchVersion" internal/adapter/httpserver/documents_api.go`;
  wenn DELETE keinen If-Match prüft, ohne Header senden + Notiz). Mapping 404/412 auf die
  ports-Sentinels. UpdatedAt-Strings mit `time.RFC3339` parsen.
- [x] **Step 2:** Contract-Tests: Put-create(If-Match 0)→Get→Put-update→412-stale→List(q)→Delete;
  RepoKey-Alias-Roundtrip.
- [x] **Step 3: Commit** (`feat(httpapi): DocumentStore-Adapter inkl. Repo-Note-Alias (R2a)`)

---

### Task 6: SSE-Events-Client + Fallback-Poll

**Files:**
- Create: `internal/adapter/httpapi/events.go` (+ `events_test.go`)

- [x] **Step 1: `events.go`**

```go
package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Events konsumiert /api/v1/events (SSE): "changed"-Events invalidieren die
// Resource-Caches und wecken Changed() — die TUI refetcht dann. Reißt der
// Stream, gilt Reconnect-mit-Refetch (Spec §7): nach jedem (Re-)Connect wird
// einmal pauschal invalidiert. Zusätzlich weckt ein Fallback-Poll alle 45 s.
type Events struct {
	c          *Client
	invalidate func(resource string) // verdrahtet in main.go über alle Adapter
	changed    chan struct{}
	stop       context.CancelFunc
	done       chan struct{}
}

func NewEvents(c *Client, invalidate func(string)) *Events {
	return &Events{c: c, invalidate: invalidate, changed: make(chan struct{}, 1), done: make(chan struct{})}
}

// Changed weckt die UI (coalesced); nach jedem Wecken: neu laden.
func (e *Events) Changed() <-chan struct{} { return e.changed }

func (e *Events) notify() {
	select {
	case e.changed <- struct{}{}:
	default:
	}
}

func (e *Events) invalidateAll() {
	for _, r := range []string{"worktime", "projects", "documents", "dayoffs"} {
		e.invalidate(r)
	}
}

func (e *Events) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	e.stop = cancel
	go e.loop(ctx)
}

func (e *Events) Stop() {
	if e.stop != nil {
		e.stop()
	}
	<-e.done
}

func (e *Events) loop(ctx context.Context) {
	defer close(e.done)
	bo := Backoff{}
	attempt := 0
	poll := time.NewTicker(45 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			e.invalidateAll()
			e.notify()
			continue
		default:
		}
		err := e.stream(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Debug("sse: stream ended", "err", err)
		}
		attempt++
		select {
		case <-ctx.Done():
			return
		case <-time.After(bo.For(attempt)):
		}
	}
}

// stream verbindet einmal und liest bis zum Fehler. Erfolgreicher Connect
// (HTTP 200) setzt attempt-Reset beim Aufrufer voraus — bewusst simpel:
// invalidateAll nach Connect deckt verpasste Events ab.
func (e *Events) stream(ctx context.Context) error {
	tok, err := e.c.bearer()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.c.base+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Flow-Client-Version", e.c.version)
	httpc := &http.Client{Timeout: 0} // Stream: kein Gesamt-Timeout
	resp, err := httpc.Do(req)
	if err != nil {
		e.c.status.setOffline()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode}
	}
	e.c.status.setOnline(e.c.base)
	e.invalidateAll() // Reconnect-Refetch
	e.notify()
	sc := bufio.NewScanner(resp.Body)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if event == "changed" {
				var d struct {
					Resource string `json:"resource"`
				}
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &d) == nil {
					e.invalidate(d.Resource)
					e.notify()
				}
			}
			event = ""
		}
	}
	return sc.Err()
}
```

- [x] **Step 2: Test** — httptest-Handler, der einen SSE-Stream schreibt (`: connected`,
  dann `event: changed\ndata: {"resource":"worktime"}\n\n`, dann Verbindung schließt):
  asserten, dass invalidate("worktime") gerufen und Changed gefeuert hat; zweiter Test:
  Reconnect nach Close (zweiter Connect am Test-Handler zählt Connects ≥ 2).
- [x] **Step 3: Commit** (`feat(httpapi): SSE-Events-Client — Invalidierung + Reconnect + Fallback-Poll (R2a)`)

---

### Task 7: Use-Cases auf die neuen Ports

**Files:**
- Modify: `internal/usecase/active_sessions.go` (+Tests), `internal/usecase/worktime_reader.go` (+Tests), `internal/usecase/identity.go` (+Tests)

- [x] **Step 1: Ist-Stand kartieren**

```bash
rg -n "func \(.*ActiveSessions\)" internal/usecase/active_sessions.go
rg -n "GetActive|flockstate|PauseStore|SetPushSignal|pushSignal" internal/usecase/*.go
```

Notiere die Treffer unter diesem Step.

- [x] **Step 2: `usecase.ActiveSessions`** bekommt das Feld `Machine ports.WorktimeMachine`.
  `Start/Stop/Pause/Resume/CorrectStart` delegieren an die Machine (lokale
  Statemachine-Logik + Store-Upserts + `SetPushSignal`-Hook ENTFERNEN); List bleibt auf
  `ports.ActiveSessionStore`. Methodensignaturen nach außen UNVERÄNDERT lassen (Aufrufer:
  TUI today_actions/picker, CLI worktime.go, MCP) — nur Innenleben tauschen. Wo heute beim
  Stop die Session lokal geschrieben wird: entfällt — `Machine.Stop` liefert die
  Server-Session; Rückgabewerte beibehalten. Pause/Resume: Marker-Logik raus.
- [x] **Step 3: `worktime_reader.go:64`** — `GetActive()`-Aufruf (flockstate) ersetzen durch
  `ActiveSessionStore.ListByUser(userID)`-basierte Ermittlung (running = Eintrag mit
  `PausedAt == nil`; Elapsed über die domain.ActiveSession-Methode aus R1). Tests anpassen
  (Fakes statt flockstate).
- [x] **Step 4: `identity.go`** — `localSub`-Feld, `ResolveActiveUser`-local-Fallback und
  `AdoptLocalDataIfFirstLogin` LÖSCHEN. Übrig bleibt eine schlanke Identität:
  Token-Claims → User via `httpapi.Identity.Me`. Wer `ResolveActiveUser` ruft
  (`rg -n "ResolveActiveUser" --type go`), bekommt die neue Signatur; ohne Token ⇒
  `ports.ErrTokenNotFound` durchreichen (UI/CLI zeigen Login-Hinweis, Task 8/9).
- [x] **Step 5: Unit-Tests grün + Commit**

```bash
go test ./internal/usecase/ -count=1 2>&1 | tail -2
```

Commit: `refactor(usecase): ActiveSessions auf WorktimeMachine, Identity ohne local-User (R2a)`

---

### Task 8: TUI — Changed-Kanal, Statuszeile, Konflikt-Code raus

**Files:**
- Create: `internal/frontend/tui/components/statusbar/connstate.go` (+`_test.go`)
- Modify: `internal/frontend/tui/screen/worktime/{model.go,today_actions.go,today_project_picker.go}`, `internal/frontend/tui/sidekick/model.go`
- Delete: `internal/frontend/tui/screen/worktime/conflicts.go`, `conflicts_test.go`, `internal/frontend/tui/components/conflict_overlay/` (komplett), `internal/frontend/cli/brief_conflict_test.go`

- [x] **Step 1: `connstate.go`** — pure Render-Funktion im statusbar-Stil
  (`Hints`-Vorbild, `hints.go:10`):

```go
package statusbar

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// ConnState rendert die Server-Statuszeile (Spec §8): still wenn ok, laut
// wenn nicht. Glyph + Farbe, nie Farbe allein (A11y-Regel).
func ConnState(s httpapi.StatusSnapshot, pal theme.Palette) string {
	sem := pal.Sem()
	switch s.State {
	case httpapi.StateOnline:
		return lipgloss.NewStyle().Foreground(sem.FgMuted).
			Render(fmt.Sprintf("● %s", hostOnly(s.Host)))
	case httpapi.StateOffline:
		stand := "—"
		if !s.LastFetched.IsZero() {
			stand = s.LastFetched.Local().Format("15:04")
		}
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render(fmt.Sprintf("○ offline · Stand %s (read-only)", stand))
	case httpapi.StateLoggedOut:
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render("○ nicht angemeldet · flow login")
	case httpapi.StateNotConfigured:
		return lipgloss.NewStyle().Foreground(sem.Warning).
			Render("○ kein Server · FLOW_SERVER_URL setzen")
	case httpapi.StateOutdated:
		return lipgloss.NewStyle().Foreground(sem.Danger).
			Render("▲ Client veraltet · Update nötig")
	default:
		return ""
	}
}

func hostOnly(base string) string {
	s := base
	for _, pre := range []string{"https://", "http://"} {
		if len(s) > len(pre) && s[:len(pre)] == pre {
			s = s[len(pre):]
		}
	}
	if i := indexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

var _ = time.Now // silence bei Format-only-Nutzung, entfernen wenn unnötig
```

(Falls `theme.Palette.Sem()` kein `FgMuted` hat: `rg -n "FgMuted|Muted" internal/frontend/tui/theme/semantic.go` und den vorhandenen Muted-Alias nehmen; Abweichung notieren.)
Test: Tabelle über alle fünf Zustände, asserten dass Glyph + Kerntext enthalten sind
(`x/exp/teatest`-frei, reiner String-Test mit fest verdrahteter Palette).

- [x] **Step 2: Worktime-Deps + Model** — in `screen/worktime/model.go`:
  Deps-Felder `Conflicts <-chan ports.ConflictMsg` und `PullDone <-chan struct{}` ersetzen
  durch `Changed <-chan struct{}` und `Status func() httpapi.StatusSnapshot`;
  conflictOverlay-Feld + Import + alle Branches (Zeilen lt. Lösch-Checkliste: 18, 163, 214,
  365, 380–382, 466–468, 549–551 — vorher mit `rg -n "conflict" internal/frontend/tui/screen/worktime/model.go`
  verifizieren) entfernen; `pullDoneMsg` durch `changedMsg struct{}` ersetzen:
  `listenForChanged(ch)`-Cmd analog `listenForPullDone` (conflicts.go stirbt — die
  listen-Helper für changed in `model.go` neu anlegen), Verhalten bei `changedMsg`:
  bestehende Reload-Logik von `pullDoneMsg` (model.go:366) übernehmen. View: Statuszeile
  über `statusbar.ConnState(deps.Status(), pal)` ans Footer-Ende der Host-View hängen —
  NUR wenn State != Online (still wenn ok) sonst der ●-Kurzform.
- [x] **Step 3: `today_actions.go` + Picker** — Legacy-Zweige (SessionWriter.Start/Stop/Pause
  via flockstate, `sw.State.SetPause`) LÖSCHEN; alle Pfade laufen über
  `deps.ActiveSessions` (der jetzt die Machine nutzt). `pauseCmd` ⇒ `ActiveSessions.Pause`,
  Resume analog. Fehlerpfade: `httpapi.ErrLoggedOut` ⇒ Statusmeldung „flow login",
  `ErrUnavailable` ⇒ „offline — Write nicht möglich" (bestehender Fehleranzeige-Mechanismus
  der Screens, `rg -n "errMsg|setError" internal/frontend/tui/screen/worktime/ | head`).
- [x] **Step 4: Sidekick** — `sidekick/model.go`: Statuszeile in der Root-View (unter den
  Hints, `model.go:441`-Bereich) aus einem neuen Deps-Feld `Status func() httpapi.StatusSnapshot`
  + `Changed`-Listener für Re-Render.
- [x] **Step 5: Löschen + Bauen** — conflicts.go/-_test.go, conflict_overlay/, brief_conflict_test.go
  löschen; `go build ./...`; TUI-Tests:

```bash
tmux kill-session -t flow_t 2>/dev/null || true
tmux new-session -d -s flow_t "export DOCKER_HOST=\"unix://\$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')\" && export TESTCONTAINERS_RYUK_DISABLED=true && go test ./internal/frontend/... -count=1 > t.log 2>&1; echo \$? > t.status"
while [ ! -f t.status ]; do sleep 5; done; cat t.status; tail -5 t.log; rm -f t.log t.status
```

Expected: `0`.

- [x] **Step 6: Commit** (`feat(tui): Statuszeile + Changed-Reload; Konflikt-UI + Legacy-Pfade raus (R2a)`)

---

### Task 9: CLI-Verben — sync raus, login ohne Adoption, Status-Hinweise

**Files:**
- Modify: `internal/frontend/cli/worktime.go`, `cmd/flow/login.go`, `cmd/flow/whoami.go`
- Delete: `internal/frontend/cli/sync.go` (+ zugehörige Tests, `rg -l "SyncStatus|sync_test" internal/frontend/cli/`)

- [x] **Step 1:** `flow sync*` entfernen: Datei + Registrierung in main.go
  (`rg -n "sync" cmd/flow/main.go`) + `usecase/sync_status.go` bleibt bis Task 12 (Compile-Kette).
- [x] **Step 2:** `worktime.go`: Legacy-Fallback-Zweige (`SessionWriter.Start/StartForce/Stop`,
  flockstate-Branches Zeilen ~552/643) löschen — ohne Server keine Writes; Fehlertexte:
  `ErrLoggedOut` ⇒ „Nicht angemeldet — `flow login`", `ErrUnavailable` ⇒
  „Server nicht erreichbar — read-only (Stand HH:MM)". `flow worktime status` ergänzt die
  ConnState-Zeile (gleicher Renderer) unter der Ausgabe wenn State != Online.
- [x] **Step 3:** `login.go`: `tryAdoptLocalProfile` + `LocalSub`-Felder + Aufruf (Zeile 78)
  löschen; nach erfolgreichem Login: `httpapi.Identity.Me` einmal rufen und Begrüßung
  ausgeben („eingeloggt als <name>"). `whoami.go`: auf `httpapi`-Client umstellen
  (gleiche Ausgabe, weniger Handarbeit).
- [x] **Step 4:** Build + CLI-Tests (tmux-Muster wie Task 8 Step 5) + Commit
  (`refactor(cli): Login-first — sync-Verben raus, Adoption raus, Status-Hinweise (R2a)`)

---

### Task 10: flow-mcp Minimal-Swap

**Files:**
- Modify: `cmd/flow-mcp/main.go`, ggf. `internal/usecase/mcp_tools.go`

- [x] **Step 1:** Wiring ersetzen: sqliteclient/httpsync/Worker raus; rein:
  `httpapi.New` (ohne Refresher wie bisher — Kommentar lassen), `httpapi.NewDocuments`,
  `httpapi.NewActiveSessions` + `httpapi.NewMachine`, `httpapi.NewProjects`,
  `httpapi.NewSessions`, Identity. `usecase.MCPTools`-Konstruktion auf die neuen Stores
  umstellen (`rg -n "NewMCPTools|MCPTools{" internal/usecase/mcp_tools.go cmd/flow-mcp/`).
  Repo-Note-Tools laufen über `ports.DocumentStore.GetByRepoKey/Put` (RepoNotes-UC-Umweg
  entfernen, wenn er nur sqliteclient kapselte — Innenleben von `mcp_tools.go` prüfen und
  minimal-invasiv umhängen). `flow_search_notes` ⇒ `DocumentStore.List(userID, "", q, 20)`.
  `flow_list_repo_notes` ⇒ `DocumentStore.List` mit `prefix="repos/"`.
  Boot ohne Token: unverändert „Login required: run `flow login`" (Stelle:
  `rg -n "Login required" cmd/flow-mcp/`).
- [x] **Step 2:** `make build`-Äquivalent für mcp (`go build ./cmd/flow-mcp/`) + bestehende
  mcp-Tests (`go test ./cmd/flow-mcp/... ./internal/usecase/ -run MCP -count=1`) grün;
  wo Tests sqliteclient-Fixtures bauten: auf httpapi-Teststack bzw. Port-Fakes umziehen.
- [x] **Step 3: Commit** (`refactor(mcp): flow-mcp auf httpapi — gleiche 7 Tools, Server-Wahrheit (R2a)`)

---

### Task 11: Composition Root — buildDeps-Rewrite + Makefile-Version

**Files:**
- Modify: `cmd/flow/main.go`, `Makefile`

- [x] **Step 1:** `buildDeps` umbauen:
  - RAUS: sqliteclient.Open + alle 8 Sub-Adapter, httpsync.{NewClient,NewQueue,NewWorker} +
    Start/Stop-Verkabelung (cleanup-Closure!), flockstate.NewLock/NewState, localSub/localUser,
    `FLOW_CACHE_DB`/`FLOW_LOCAL_USER_SUB`-Env-Reads.
  - REIN (Reihenfolge): `serverURL := os.Getenv("FLOW_SERVER_URL")` — LEER ist erlaubt
    (Client liefert dann ErrNotConfigured; Statuszeile zeigt es); keyring + Slot wie gehabt;
    `httpapi.New(httpapi.Config{BaseURL: serverURL, Tokens: keyring, Slot: slot, Refresher: &oidcclient.StoreRefresher{…}, Version: version, …})`;
    Resource-Adapter; `events := httpapi.NewEvents(client, invalidateFn)`; `events.Start(ctx)`
    + Stop in cleanup; `go func(){ _ = client.CheckMeta(ctx) }()` einmalig beim Start.
  - `var version = "dev"` Top-Level in `cmd/flow/main.go` einführen (ldflags-Ziel).
  - Deps-Structs der Screens mit `Changed: events.Changed()`, `Status: client.StatusOf().Snapshot`
    befüllen; UserID-Felder: aus `identity.Me` wenn eingeloggt, sonst "" (Screens zeigen
    Login-Hinweis — Reads liefern ErrLoggedOut).
  - dayoffstsv: Konstruktion NUR noch dort, wo `dayoff sync`/Migration sie als Quelle
    braucht (`rg -n "dayoffstsv" cmd/ internal/frontend/cli/`) — DayOffStore-Port zeigt auf httpapi.
- [x] **Step 2:** Makefile: ldflags `-X main.version=$(VERSION)` analog flow-server-Eintrag
  (Makefile:116) für `cmd/flow` und `cmd/flow-mcp` ergänzen (`rg -n "ldflags" Makefile`).
- [x] **Step 3:** `go build ./... && go vet ./...` grün; `./bin/flow --help` läuft (Build via
  bestehendes make-Target, `rg -n "^build" Makefile`). Commit
  (`feat(flow): Composition Root auf httpapi — Server-only-Client (R2a)`)

---

### Task 12: Löschungen + Port-Hygiene

**Files:**
- Delete: `internal/adapter/httpsync/`, `internal/adapter/sqliteclient/`, `internal/adapter/flockstate/`, `internal/usecase/sync_status.go` (+Tests), `internal/ports/sync.go`
- Modify: `internal/ports/sessions.go` (LegacyActiveStore + Lock raus), `internal/ports/pause.go` (Datei löschen wenn PauseStore der einzige Inhalt ist), Konsumenten-Reste

- [x] **Step 1:** Pakete löschen (`git rm -r`), dann Compile-Kette abarbeiten:

```bash
go build ./... 2>&1 | head -30
```

Jeden Fehler einzeln fixen — erwartete Reste: Imports in Tests, `ports.ConflictMsg`-Nutzung
(conflicts-Reste), `WriteQueue`-Referenzen in usecase-Tests. Lösch-Checkliste aus der
Inventur abgleichen: `rg -n "httpsync|sqliteclient|flockstate|WriteQueue|SyncWatermark|ConflictMsg|LegacyActiveStore|PauseStore|FLOW_CACHE_DB|FLOW_LOCAL_USER_SUB" --type go | rg -v "_test.go:.*//"` — am Ende NULL Treffer außerhalb von docs/.

- [x] **Step 2:** Betroffene Tests: löschen wenn sie GELÖSCHTES Verhalten testeten, umziehen
  auf Fakes/httpapi wenn sie BLEIBENDES Verhalten testeten (Entscheidung je Datei als
  Abweichungsnotiz, wenn unklar).
- [x] **Step 3:** `go build ./... && go vet ./...` + voller Testlauf (tmux-Muster). Commit
  (`refactor(client)!: Sync-Stack gelöscht — httpsync, sqliteclient, flockstate, Konflikt-Ports (R2a)`)

---

### Task 13: Coverage-Gate neu vermessen

- [x] **Step 1:** `make ci` (tmux-Muster); aus `ci.log` die Gesamt-Coverage ablesen.
- [x] **Step 2:** Gate in der CI-Konfiguration (`rg -n "73|coverage" Makefile .github/ ci/ 2>/dev/null | rg -i "threshold|gate|cover"`) ehrlich auf gemessenen Wert −2 pp setzen (Spec §14), Begründung als Kommentar.
- [x] **Step 3:** Commit (`ci: Coverage-Gate nach R2a-Löschungen neu vermessen (R2a)`)

---

### Task 14: Wiring-Verification + Abschluss (Pflicht-DoD)

**Files:**
- Create: `scripts/smoke-r2a-client.sh`
- Modify: diese Plan-Datei (Protokoll)

- [x] **Step 1: Constructor-Audit** — jede in Task 1–6 neue Konstruktor-Funktion wird in
  `cmd/flow/main.go` ODER `cmd/flow-mcp/main.go` gerufen:

```bash
for f in New NewEvents NewSessions NewActiveSessions NewMachine NewProjects NewDayOffs NewConfigReader NewDocuments NewIdentity; do
  rg -l "httpapi.$f\b" cmd/ >/dev/null || echo "UNVERKABELT: httpapi.$f"
done
```

Expected: keine Ausgabe. (Namen vorher gegen die tatsächlich angelegten Konstruktoren
prüfen, Liste ggf. korrigieren.)

- [x] **Step 2: Client-Smoke-Script** `scripts/smoke-r2a-client.sh` (Muster von
  `smoke-r1-routes.sh`): startet PG + dex + flow-server wie der R1-Smoke, baut `./bin/flow`,
  dann OHNE Login: `FLOW_SERVER_URL=http://localhost:8080 ./bin/flow worktime status` ⇒
  Output enthält „nicht angemeldet" und Exit-Code 0 (Status ist read-only-Anzeige);
  `…/flow worktime start -p x` ⇒ Exit ≠ 0 + „flow login"-Hinweis; OHNE FLOW_SERVER_URL:
  Output enthält „kein Server". Script mit `set -euo pipefail` + trap-Cleanup.
- [x] **Step 3:** Smoke laufen lassen (`./scripts/smoke-r2a-client.sh; echo EXIT=$?`) ⇒ EXIT=0.
- [x] **Step 4:** Finale `make ci` (tmux-Muster) ⇒ `0`; `git status --short` clean.
- [x] **Step 5:** Buchhaltung: alle Checkboxen dieses Plans gepflegt, Abweichungs-Protokoll
  gefüllt; Commit (`docs(plan): R2a abgeschlossen — Smoke + Buchhaltung`). NICHT pushen.

> **Hinweis:** Das Dogfood-Gate (§14) kommt NACH R2b — R2a alleine lässt das
> Kompendium noch auf lokalem fsstore (separater Plan).

---

## Self-Review (gegen Spec §8/§13 R2 + A1)

| Spec-Anforderung | Task |
|---|---|
| httpapi implementiert bestehende Ports; Cache + SSE-Invalidierung + Fallback-Poll §5/§8 | 1–3, 6 |
| Writes synchron aus dem bubbletea-Cmd; Timer ab Server-started_at/pause_total §8 | 4, 7, 8 |
| Statuszeile überall: ●/○/▲, Glyph+Farbe, still wenn ok §8 | 8, 9 |
| Offline-Read-Snapshot `$XDG_STATE_HOME/flow/snapshot.json` §8 | 2, 3 |
| Ohne Login: Hinweis statt Daten; Adoption/local-User gestrichen §8 | 7, 8, 9, 11 |
| `FLOW_SERVER_URL` Pflicht-Konfiguration (NotConfigured-Zustand) §8 | 1, 11, 14 |
| Versions-Handshake: X-Flow-Client-Version + /meta-Warnung §7 | 1, 2, 11 (Makefile) |
| Lösch-Liste §12: httpsync, sqliteclient, flockstate, conflict_overlay, Adoption | 8, 9, 12 |
| flow-mcp gegen dieselbe Wahrheit (Minimal-Swap; neue Doc-Tools erst R3) §9 | 10 |
| Coverage ehrlich neu §14 | 13 |
| Wiring-Task am Ende (Memory-Regel) + DoD (A1) | 14 |

**Bewusste Abweichungen von der Spec:** WorktimeMachine-Port (statt Upsert-Mapping),
Server-Ergänzungen active/correct + TouchLastUsed-on-start, R2-Split in R2a/R2b mit
Dogfood-Gate nach R2b, `flow_search_notes` zeigt ab R2a auf Server-FTS (Spec §9 nennt das
für R3 — hier nötig, weil der lokale Suchpfad mit sqliteclient stirbt).

## Abweichungs-Protokoll

(Der Executor trägt hier ein, was vom Plan abweichen musste — Task-Nummer + 1–3 Sätze.)
