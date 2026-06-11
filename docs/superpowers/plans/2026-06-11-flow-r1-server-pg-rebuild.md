# R1 — Server-Rebuild auf Postgres (Server-only-Wahrheit) — Implementation Plan

> **Für den Executor (agy CLI):** Dieser Plan ist selbsttragend — er setzt KEINE
> Claude-Code-Skills voraus. Abarbeitung streng Task-für-Task nach dem
> Executor-Protokoll unten. Checkboxen (`- [ ]`) in DIESER Datei abhaken
> (Datei editieren), sie sind der persistente Fortschrittszustand zwischen
> agy-Sessions.

**Spec:** `docs/superpowers/specs/2026-06-11-flow-server-only-rebuild-design.md` (R1 = §13 Punkt 1)

**Goal:** flow-server wird die einzige Wahrheit: Postgres (pgx/v5 + goose-PG-Baseline) ersetzt
SQLite server-seitig, `documents`-Tabelle mit PG-FTS ersetzt fsstore-Notes und repo_notes,
Pause-Statemachine auf active_sessions, neue REST-API nach Spec §7 (inkl. `/meta`),
SSE generalisiert (`changed`-Events + Heartbeat, Bearer-fähig), alte Sync-Endpoints +
sqliteserver gelöscht, WebUI auf documents + ehrlichen Status, compose + Helm auf PG/CNPG.

**Architecture:** Hexagonal. Neuer Adapter `internal/adapter/pgstore` (pgx/v5-Pool, goose
`DialectPostgres`, embedded Baseline-Migration) mit Sub-Adaptern Users/Projects/Sessions/
ActiveSessions/Documents/DayOffs/Settings — Methoden-Signaturen drop-in-kompatibel zu
`sqliteserver`, plus neu Pause/Resume. Die WebUI-Handler-Deps werden vorher von konkreten
`*sqliteserver.X`-Typen auf schmale lokale Interfaces gehoben (bestehende Layer-Verletzung),
so dass der Swap in `cmd/flow-server/main.go` ein reiner Wiring-Tausch ist. Die neue
Bearer-API lebt in neuen Handler-Dateien unter `internal/adapter/httpserver/` und wird im
Swap-Task gemountet, während die alten pull/push-Routen entfallen.

**Tech Stack:** Go 1.25, pgx/v5 + pgxpool, pressly/goose v3 (PG-Dialekt), testcontainers-go
(+ modules/postgres), chi v5, templ 0.3.x, HTMX (+sse-Extension), Postgres 16 (lokal compose,
im Homelab CNPG).

**Arbeitsverzeichnis:** `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1` (Branch `next`).
Alle Pfade relativ dazu.

---

## Executor-Protokoll (agy)

Pro Task EINE frische agy-Session starten (hält den Kontext klein; der Plan trägt den Zustand):

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
agy -i "Öffne docs/superpowers/plans/2026-06-11-flow-r1-server-pg-rebuild.md. Suche den ersten Task, der noch unerledigte Checkboxen hat. Führe NUR diesen einen Task aus: jeden Step exakt wie beschrieben, Kommandos ausführen und Output gegen 'Expected' prüfen. Nach jedem erledigten Step die Checkbox in der Plan-Datei auf [x] setzen. Am Task-Ende committen (Commit-Message steht im Task). Danach STOPPEN und kurz berichten, was ggf. vom Plan abgewichen ist."
```

Regeln für den Executor:

1. **Reihenfolge ist bindend.** Tasks bauen aufeinander auf; nie vorgreifen.
2. **Expected-Mismatch = Stopp.** Wenn ein Kommando nicht das erwartete Ergebnis liefert:
   Ursache untersuchen und beheben, solange es im Scope des Tasks bleibt. Bei allem
   darüber hinaus: Abweichung unter dem Task im Plan-File dokumentieren
   (`> **Abweichung:** …`) und die Session mit Bericht beenden — nicht improvisieren.
3. **Code-Blöcke sind die Wahrheit.** Den gezeigten Code übernehmen; nur mechanische
   Anpassungen (Imports sortieren, vom Compiler geforderte Trivial-Fixes) sind erlaubt
   und werden als Abweichung notiert, wenn sie über Formatierung hinausgehen.
4. **Vor jedem Commit:** `gofumpt -w <geänderte .go-Dateien>` (golangci-lint erzwingt das).
   Bei `.templ`-Änderungen vorher `make webui-templ` (generiert `*_templ.go` — mit committen).
5. **Niemals:** `git push`, Branch wechseln, `CLAUDE-*.md` committen oder löschen,
   Force-Operationen. Commits bleiben lokal auf `next`.
6. **Commit-Messages** exakt wie im Task angegeben, jeweils mit Abschluss-Zeile
   `Co-Authored-By: agy <noreply@google.com>` (im `git commit -m "$(cat <<'EOF' … EOF)"`-Muster
   der Tasks bereits enthalten).
7. **`make ci`-Checkpoints** stehen nur in den Tasks 0, 8, 13, 18, 19 und 22 — zwischendurch
   reichen die im Task genannten paketlokalen Builds/Tests.
8. **testcontainers:** Die pgstore-/Handler-Tests starten Postgres-Container. Auf dieser
   Maschine läuft podman — die env-Variablen aus Task 0 Step 3 müssen in jeder Session
   gesetzt sein, in der Tests laufen.

---

## File-Map (Endzustand R1)

**Neu:**

| Datei | Verantwortung |
|---|---|
| `internal/testutil/pgtest/pg.go` | Wegwerf-PG-Container für Tests (ein Container pro Test-Package via TestMain) |
| `internal/adapter/pgstore/store.go` | pgxpool + goose-Migrationen + Ping/Close |
| `internal/adapter/pgstore/migrations/embed.go` + `0001_baseline.sql` | PG-Baseline (Spec §6, Abweichungen siehe Task 2) |
| `internal/adapter/pgstore/users.go` / `projects.go` / `sessions.go` / `active_sessions.go` / `documents.go` / `dayoffs.go` / `settings.go` | Sub-Adapter (+ je `*_test.go`) |
| `internal/ports/documents.go` | `DocumentStore`-Port + Sentinel-Errors |
| `internal/adapter/httpserver/worktime_api.go` | Bearer-API: sessions CRUD+bulk, active start/stop/pause/resume |
| `internal/adapter/httpserver/documents_api.go` | Bearer-API: documents + `/repos/{key}/note`-Alias |
| `internal/adapter/httpserver/dayoffs_settings_api.go` | Bearer-API: day-offs + settings |
| `internal/adapter/httpserver/meta.go` | `GET /api/v1/meta` (public) |
| `internal/adapter/httpserver/middleware_dual.go` | Bearer-ODER-Cookie-Middleware für `/api/v1/events` |
| `internal/webui/handlers/documents.go` / `documents_vm.go` / `document_actions.go` | WebUI-Notes auf DocumentStore |
| `scripts/smoke-r1-routes.sh` | Route-Smoke für Task 22 |

**Gelöscht (Task 19/20/21):** `internal/adapter/sqliteserver/` (komplett),
`internal/adapter/httpserver/{sessions,active_sessions,projects,repos,repo_notes}_handlers.go`
(+ Tests), `internal/webui/handlers/{notes.go,notes_vm.go,note_actions.go}` (+ Tests),
`deploy/podman/litestream.yml`, `scripts/litestream-restore-drill.sh`, `scripts/smoke-m2-m3.sh`,
Litestream/MinIO aus compose + Helm, `FLOW_SERVER_DB`/`FLOW_NOTEBOOK_ROOT` aus Config.

**Bleibt unangetastet (R2!):** `internal/adapter/sqliteclient`, `internal/adapter/httpsync`,
`internal/adapter/flockstate`, alles unter `cmd/flow`, `cmd/flow-mcp`, `internal/frontend`,
`internal/kompendium` (Client-Teile). Der Client ist nach R1 gegen den neuen Server
funktional tot — das ist der geplante Zustand auf dem Integrationsbranch bis R2.

**Bewusste Abweichungen von der Spec (zur Plan-Zeit entschieden, im Code kommentieren):**

1. `users.email`-Spalte zusätzlich (Spec-DDL §6 hat sie nicht, `domain.User`/`EnsureBySub`
   brauchen sie).
2. `projects`: `archived_at timestamptz NULL` statt `archived boolean` — `domain.Project`
   trägt `ArchivedAt *time.Time`; plus `last_used_at`, `created_at`, `version`, `updated_at`
   (vom heutigen Verhalten benötigt).
3. `active_sessions.pause_total_ns bigint` statt `interval` — pgx-v5-Mapping
   interval↔`time.Duration` ist fragil; bigint-Nanosekunden sind verlustfrei und trivial.
4. `day_offs`: zusätzlich `label text` + `target_ns bigint` (Felder von `domain.DayOff`).
5. Versionierung pro Row via `version = version + 1` — der globale Lamport entfällt
   ersatzlos (keine Sync-Watermarks mehr, Spec §6).

---

### Task 0: Preflight — Baseline + testcontainers/podman

**Files:** keine Änderungen, nur Verifikation.

- [x] **Step 1: Worktree-Zustand prüfen**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
git status --short && git branch --show-current
```

Expected: keine Ausgabe von `--short` (clean) und Branch `next`. Wenn nicht clean: STOPP, Bericht.

- [x] **Step 2: CI-Baseline**

```bash
make ci 2>&1 | tail -5
```

Expected: grün (Coverage-Gate 77 % besteht). Wenn rot: STOPP, Bericht — die Baseline muss
in Ordnung sein, bevor der Umbau beginnt.

- [x] **Step 3: podman-Socket für testcontainers verifizieren**

```bash
podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
docker_host_ok=$(curl -s --unix-socket "${DOCKER_HOST#unix://}" http://d/_ping || true)
echo "ping: ${docker_host_ok}"
```

Expected: `ping: OK`. Diese beiden `export`-Zeilen braucht JEDE Session, die `go test` auf
pgstore/httpserver/webui ausführt (Task 2 ff.). Wenn die podman-Machine nicht läuft:
`podman machine start`, dann wiederholen.

- [x] **Step 4: Notiz im Plan**

Trage hier die gemessene Coverage-Baseline ein (Zahl aus Step 2):

> Coverage-Baseline vor R1: 77.4 %

Kein Commit in diesem Task.

---

### Task 1: Dependencies — pgx/v5 + testcontainers-postgres

**Files:**
- Modify: `go.mod`, `go.sum`

- [x] **Step 1: Pakete holen**

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@v0.42.0
go mod tidy
```

Expected: `go: added github.com/jackc/pgx/v5 v5.x.y` (+ pgpassfile/pgservicefile/puddle
indirekt). Das postgres-Modul MUSS dieselbe Version wie testcontainers-go (v0.42.0) haben.

- [x] **Step 2: Build-Check**

```bash
go build ./...
```

Expected: Exit 0.

- [x] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore(deps): pgx/v5 + testcontainers postgres module (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 2: pgtest-Helper + pgstore.Store + PG-Baseline-Migration

**Files:**
- Create: `internal/testutil/pgtest/pg.go`
- Create: `internal/adapter/pgstore/store.go`
- Create: `internal/adapter/pgstore/migrations/embed.go`
- Create: `internal/adapter/pgstore/migrations/0001_baseline.sql`
- Create: `internal/adapter/pgstore/main_test.go`
- Create: `internal/adapter/pgstore/store_test.go`

- [x] **Step 1: pgtest-Container-Helper**

```go
// internal/testutil/pgtest/pg.go
//
// Package pgtest starts a throwaway PostgreSQL container for store and
// handler tests. Mirrors the oidctest dex pattern, but fails loud instead
// of skipping: pgstore tests ARE the core test surface after R1, a silent
// skip would hollow out the coverage gate.
//
// Usage (one container per test package, shared store, isolation via
// per-test users — all tables are user-scoped):
//
//	var testStore *pgstore.Store   // package-level, set in TestMain
//
//	func TestMain(m *testing.M) { os.Exit(pgtest.RunWithStore(m, &testStore)) }
package pgtest

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

// StartContainer boots a postgres:16-alpine container and returns its DSN
// plus a terminate func. The caller owns termination.
func StartContainer(ctx context.Context) (dsn string, terminate func(), err error) {
	ctr, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("flow_test"),
		postgres.WithUsername("flow"),
		postgres.WithPassword("flow"),
		testcontainers.WithWaitStrategy(
			tcwait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("pgtest: start container (DOCKER_HOST gesetzt? podman machine läuft?): %w", err)
	}
	dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return "", nil, fmt.Errorf("pgtest: connection string: %w", err)
	}
	return dsn, func() { _ = ctr.Terminate(context.Background()) }, nil
}
```

- [x] **Step 2: Migrations-Embed**

```go
// internal/adapter/pgstore/migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [x] **Step 3: Baseline-Migration**

`internal/adapter/pgstore/migrations/0001_baseline.sql` — Spec §6 mit den im File-Map
dokumentierten Abweichungen (email, archived_at, pause_total_ns, day_offs-Zusatzfelder):

```sql
-- +goose Up
CREATE TABLE users (
    id           uuid PRIMARY KEY,
    oidc_sub     text NOT NULL UNIQUE,
    email        text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id),
    name         text NOT NULL,
    slug         text NOT NULL,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    version      bigint NOT NULL DEFAULT 1,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, slug)
);

CREATE TABLE sessions (
    id         uuid PRIMARY KEY,        -- Client darf UUIDv5 liefern (Import-Idempotenz)
    user_id    uuid NOT NULL REFERENCES users(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    day        date NOT NULL,           -- Buchungstag in User-Zeitzone
    started_at timestamptz NOT NULL,
    stopped_at timestamptz NOT NULL,
    tag        text NOT NULL DEFAULT '',
    note       text NOT NULL DEFAULT '',
    version    bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_day ON sessions (user_id, day);

CREATE TABLE active_sessions (
    user_id           uuid NOT NULL REFERENCES users(id),
    project_id        uuid NOT NULL REFERENCES projects(id),
    started_at        timestamptz NOT NULL, -- Server-Zeit, nie Client-Zeit
    paused_at         timestamptz,          -- NULL = läuft
    pause_total_ns    bigint NOT NULL DEFAULT 0,
    started_on_device text NOT NULL DEFAULT '',
    tag               text NOT NULL DEFAULT '',
    note              text NOT NULL DEFAULT '',
    version           bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE documents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id),
    path       text NOT NULL,
    body       text NOT NULL DEFAULT '',
    repo_key   text,
    version    bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    search     tsvector GENERATED ALWAYS AS (to_tsvector('simple', path || ' ' || body)) STORED,
    UNIQUE (user_id, path)
);
CREATE UNIQUE INDEX documents_repo_key ON documents (user_id, repo_key) WHERE repo_key IS NOT NULL;
CREATE INDEX documents_search ON documents USING gin (search);

CREATE TABLE day_offs (
    user_id   uuid NOT NULL REFERENCES users(id),
    day       date NOT NULL,
    kind      text NOT NULL,
    label     text NOT NULL DEFAULT '',
    target_ns bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day)
);

CREATE TABLE user_settings (
    user_id uuid NOT NULL REFERENCES users(id),
    key     text NOT NULL,
    value   text NOT NULL,
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS day_offs;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS active_sessions;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
```

- [x] **Step 4: Store-Konstruktor**

```go
// internal/adapter/pgstore/store.go
//
// Package pgstore implements flow-server's persistence on PostgreSQL
// (pgx/v5 + goose PG migrations). It replaces internal/adapter/sqliteserver
// in the R1 server-only rebuild. Sub-adapters (Users, Projects, Sessions,
// ActiveSessions, Documents, DayOffs, Settings) share the pool via *Store.
//
// Versioning: per-row `version` incremented on every server-side write.
// The old global lamport counter is gone — without sync watermarks there
// is nothing that needs cross-row monotonicity.
package pgstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/serverkraken/flow/internal/adapter/pgstore/migrations"
)

// Store owns the pgx connection pool. Open runs pending goose migrations.
type Store struct{ pool *pgxpool.Pool }

// Open connects to dsn, pings, migrates, and returns a ready Store.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgstore: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	// goose braucht *sql.DB; stdlib wrappt den Pool nur für die Migration.
	sqlDB := stdlib.OpenDBFromPool(pool)
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: migrate: %w", err)
	}
	_ = sqlDB.Close()
	return &Store{pool: pool}, nil
}

// Pool exposes the pgx pool for the sub-adapters in this package.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping reports connectivity; wired into /readyz.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Close shuts the pool down.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}
```

- [x] **Step 5: TestMain + Store-Test (failing first ist hier nicht möglich — der Test
  verifiziert die Migration; er schlägt fehl, solange das SQL fehlerhaft ist)**

```go
// internal/adapter/pgstore/main_test.go
package pgstore_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// testStore is shared across the package's tests. Isolation comes from
// per-test users — every table is user-scoped.
var testStore *pgstore.Store

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx := context.Background()
		dsn, terminate, err := pgtest.StartContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer terminate()
		s, err := pgstore.Open(ctx, dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgstore open:", err)
			return 1
		}
		defer s.Close()
		testStore = s
		return m.Run()
	}())
}
```

```go
// internal/adapter/pgstore/store_test.go
package pgstore_test

import (
	"context"
	"testing"
)

func TestOpen_MigrationsCreateAllTables(t *testing.T) {
	want := []string{
		"users", "projects", "sessions", "active_sessions",
		"documents", "day_offs", "user_settings",
	}
	for _, name := range want {
		var got string
		err := testStore.Pool().QueryRow(context.Background(),
			`SELECT table_name FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, name).Scan(&got)
		if err != nil {
			t.Errorf("table %q missing: %v", name, err)
		}
	}
	// Kein lamport mehr — bewusst gelöscht (Spec §6).
	var n int
	_ = testStore.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = 'lamport'`).Scan(&n)
	if n != 0 {
		t.Errorf("lamport table must not exist, found %d", n)
	}
}
```

- [x] **Step 6: Tests laufen lassen**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./internal/adapter/pgstore/... -run TestOpen -v -timeout 180s
```

Expected: PASS. (Erster Lauf zieht das postgres:16-alpine-Image — kann ~30 s dauern.)

- [x] **Step 7: Commit**

```bash
gofumpt -w internal/testutil/pgtest/ internal/adapter/pgstore/
git add internal/testutil/pgtest/ internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore): Store + PG-Baseline-Migration + pgtest-Container-Helper (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 3: pgstore.Users

**Files:**
- Create: `internal/adapter/pgstore/users.go`
- Create: `internal/adapter/pgstore/users_test.go`

Hinweis uuid↔string: pgx v5 kodiert Go-`string` ↔ PG-`uuid` in beide Richtungen. Falls ein
Scan-Fehler `cannot scan uuid` auftritt: betroffene SELECT-Spalte auf `id::text` umstellen
und als Abweichung notieren.

- [ ] **Step 1: Failing Test**

```go
// internal/adapter/pgstore/users_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestUsers_EnsureBySub_CreateThenUpdate(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)

	created, err := u.EnsureBySub("sub-users-1", "a@b.de", "Alice")
	if err != nil {
		t.Fatalf("EnsureBySub create: %v", err)
	}
	if created.ID == "" || created.OIDCSub != "sub-users-1" || created.Email != "a@b.de" {
		t.Fatalf("unexpected user: %+v", created)
	}

	updated, err := u.EnsureBySub("sub-users-1", "neu@b.de", "Alice Neu")
	if err != nil {
		t.Fatalf("EnsureBySub update: %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("ID changed on upsert: %s != %s", updated.ID, created.ID)
	}
	if updated.Email != "neu@b.de" || updated.DisplayName != "Alice Neu" {
		t.Errorf("fields not updated: %+v", updated)
	}
}

func TestUsers_Get_NotFound(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)
	if _, err := u.GetByID("00000000-0000-0000-0000-000000000000"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("GetByID: want ErrUserNotFound, got %v", err)
	}
	if _, err := u.GetBySub("does-not-exist"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Errorf("GetBySub: want ErrUserNotFound, got %v", err)
	}
}

func TestUsers_GetBySub_RoundTrip(t *testing.T) {
	t.Parallel()
	u := pgstore.NewUsers(testStore)
	created, _ := u.EnsureBySub("sub-users-2", "x@y.de", "X")
	got, err := u.GetBySub("sub-users-2")
	if err != nil {
		t.Fatalf("GetBySub: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: %s != %s", got.ID, created.ID)
	}
}
```

- [ ] **Step 2: Test laufen lassen — muss fehlschlagen**

```bash
go test ./internal/adapter/pgstore/... -run TestUsers -timeout 180s 2>&1 | tail -3
```

Expected: Compile-FAIL `undefined: pgstore.NewUsers`.

- [ ] **Step 3: Implementierung**

```go
// internal/adapter/pgstore/users.go
package pgstore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Users implements ports.UserStore on PG.
type Users struct{ store *Store }

func NewUsers(s *Store) *Users { return &Users{store: s} }

var _ ports.UserStore = (*Users)(nil)

const userCols = `id, oidc_sub, email, display_name, created_at`

// EnsureBySub upserts by OIDC sub and returns the canonical row.
func (u *Users) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(), `
		INSERT INTO users (id, oidc_sub, email, display_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (oidc_sub) DO UPDATE
			SET email = EXCLUDED.email, display_name = EXCLUDED.display_name
		RETURNING `+userCols,
		uuid.NewString(), sub, email, displayName)
	return scanUser(row)
}

func (u *Users) GetByID(id string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(),
		`SELECT `+userCols+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (u *Users) GetBySub(sub string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(),
		`SELECT `+userCols+` FROM users WHERE oidc_sub = $1`, sub)
	return scanUser(row)
}

// rowScanner abstracts pgx.Row and pgx.Rows for the scan helpers in this package.
type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var out domain.User
	err := r.Scan(&out.ID, &out.OIDCSub, &out.Email, &out.DisplayName, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}
```

- [ ] **Step 4: Tests grün**

```bash
go test ./internal/adapter/pgstore/... -run TestUsers -v -timeout 180s 2>&1 | tail -8
```

Expected: 3× PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/adapter/pgstore/
git add internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore): Users-Adapter (ports.UserStore) + Tests (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 4: pgstore.Projects

**Files:**
- Create: `internal/adapter/pgstore/projects.go`
- Create: `internal/adapter/pgstore/projects_test.go`

Signaturen drop-in-kompatibel zu `sqliteserver.Projects` (insb.
`Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)` — OCC).
`PullSince` entfällt ersatzlos (Sync ist tot).

- [ ] **Step 1: Failing Test**

```go
// internal/adapter/pgstore/projects_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// mustUser legt einen frischen User für Test-Isolation an (alle Tabellen
// sind user-gescoped; der geteilte Container braucht keine Truncates).
func mustUser(t *testing.T, sub string) string {
	t.Helper()
	u, err := pgstore.NewUsers(testStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("mustUser(%s): %v", sub, err)
	}
	return u.ID
}

func TestProjects_EnsureListArchive(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-1")

	proj, err := p.EnsureBySlug(uid, "Mein Projekt", "mein-projekt")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	if proj.Name != "Mein Projekt" || proj.Version != 1 {
		t.Fatalf("unexpected project: %+v", proj)
	}

	// idempotent: zweiter Ensure liefert dieselbe Row, legt nichts Neues an
	again, err := p.EnsureBySlug(uid, "ignoriert", "mein-projekt")
	if err != nil {
		t.Fatalf("EnsureBySlug again: %v", err)
	}
	if again.ID != proj.ID || again.Name != "Mein Projekt" {
		t.Errorf("EnsureBySlug not idempotent: %+v", again)
	}

	active, err := p.ListActive(uid)
	if err != nil || len(active) != 1 {
		t.Fatalf("ListActive: %v len=%d", err, len(active))
	}

	if err := p.Archive(uid, proj.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	active, _ = p.ListActive(uid)
	if len(active) != 0 {
		t.Errorf("after Archive ListActive should be empty, got %d", len(active))
	}
	all, _ := p.ListAll(uid)
	if len(all) != 1 || all[0].ArchivedAt == nil {
		t.Errorf("ListAll should contain archived project with ArchivedAt set: %+v", all)
	}
}

func TestProjects_UpsertOCC(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-2")
	proj, _ := p.EnsureBySlug(uid, "A", "a")

	proj.Name = "A umbenannt"
	saved, err := p.Upsert(proj, proj.Version)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if saved.Version != proj.Version+1 || saved.Name != "A umbenannt" {
		t.Errorf("version/name after upsert: %+v", saved)
	}

	// stale write → Konflikt
	if _, err := p.Upsert(proj, proj.Version); !errors.Is(err, ports.ErrProjectVersionConflict) {
		t.Errorf("stale upsert: want ErrProjectVersionConflict, got %v", err)
	}
}

func TestProjects_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-3")
	if _, err := p.GetByID(uid, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("want ErrProjectNotFound, got %v", err)
	}
	var _ domain.Project // keep import obvious
}
```

- [ ] **Step 2: Test — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/pgstore/... -run TestProjects -timeout 180s 2>&1 | tail -3
```

Expected: `undefined: pgstore.NewProjects`.

- [ ] **Step 3: Implementierung**

```go
// internal/adapter/pgstore/projects.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects mirrors the sqliteserver.Projects surface (minus PullSince) on PG.
type Projects struct{ store *Store }

func NewProjects(s *Store) *Projects { return &Projects{store: s} }

const projectCols = `id, user_id, name, slug, archived_at, created_at, last_used_at, version, updated_at`

func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	return p.list(userID, `AND archived_at IS NULL`)
}

func (p *Projects) ListAll(userID string) ([]domain.Project, error) {
	return p.list(userID, ``)
}

func (p *Projects) list(userID, extraCond string) ([]domain.Project, error) {
	rows, err := p.store.Pool().Query(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 `+extraCond+` ORDER BY name ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		proj, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, proj)
	}
	return out, rows.Err()
}

func (p *Projects) GetByID(userID, id string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 AND id = $2`, userID, id)
	return scanProjectNotFound(row)
}

func (p *Projects) GetBySlug(userID, slug string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(),
		`SELECT `+projectCols+` FROM projects WHERE user_id = $1 AND slug = $2`, userID, slug)
	return scanProjectNotFound(row)
}

// EnsureBySlug creates the project if missing and returns the existing row
// otherwise — it never renames (matches sqliteserver semantics).
func (p *Projects) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	row := p.store.Pool().QueryRow(context.Background(), `
		INSERT INTO projects (id, user_id, name, slug)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, slug) DO UPDATE SET slug = EXCLUDED.slug -- no-op, forces RETURNING
		RETURNING `+projectCols,
		uuid.NewString(), userID, name, slug)
	return scanProject(row)
}

// Upsert writes with OCC: the stored version must equal expectedVersion
// (0 = "must not exist yet"). Returns the saved row with bumped version.
func (p *Projects) Upsert(in domain.Project, expectedVersion int64) (domain.Project, error) {
	ctx := context.Background()
	if expectedVersion == 0 {
		if in.ID == "" {
			in.ID = uuid.NewString()
		}
		row := p.store.Pool().QueryRow(ctx, `
			INSERT INTO projects (id, user_id, name, slug, archived_at, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, 1, now())
			ON CONFLICT (id) DO NOTHING
			RETURNING `+projectCols,
			in.ID, in.UserID, in.Name, in.Slug, in.ArchivedAt)
		out, err := scanProject(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, ports.ErrProjectVersionConflict
		}
		return out, err
	}
	row := p.store.Pool().QueryRow(ctx, `
		UPDATE projects
		SET name = $3, slug = $4, archived_at = $5, version = version + 1, updated_at = now()
		WHERE user_id = $1 AND id = $2 AND version = $6
		RETURNING `+projectCols,
		in.UserID, in.ID, in.Name, in.Slug, in.ArchivedAt, expectedVersion)
	out, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectVersionConflict
	}
	return out, err
}

func (p *Projects) TouchLastUsed(userID, id string) error {
	_, err := p.store.Pool().Exec(context.Background(),
		`UPDATE projects SET last_used_at = now() WHERE user_id = $1 AND id = $2`, userID, id)
	return err
}

func (p *Projects) Archive(userID, id string) error {
	_, err := p.store.Pool().Exec(context.Background(),
		`UPDATE projects SET archived_at = now(), version = version + 1, updated_at = now()
		 WHERE user_id = $1 AND id = $2 AND archived_at IS NULL`, userID, id)
	return err
}

func scanProject(r rowScanner) (domain.Project, error) {
	var out domain.Project
	var archivedAt, lastUsedAt *time.Time
	err := r.Scan(&out.ID, &out.UserID, &out.Name, &out.Slug,
		&archivedAt, &out.CreatedAt, &lastUsedAt, &out.Version, new(time.Time))
	if err != nil {
		return domain.Project{}, err
	}
	out.ArchivedAt = archivedAt
	if lastUsedAt != nil {
		out.LastUsedAt = *lastUsedAt
	}
	return out, nil
}

func scanProjectNotFound(r rowScanner) (domain.Project, error) {
	out, err := scanProject(r)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return out, err
}
```

- [ ] **Step 4: Tests grün**

```bash
go test ./internal/adapter/pgstore/... -run TestProjects -v -timeout 180s 2>&1 | tail -8
```

Expected: 3× PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/adapter/pgstore/
git add internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore): Projects-Adapter mit OCC-Upsert + Tests (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 5: pgstore.Settings + pgstore.DayOffs

**Files:**
- Create: `internal/adapter/pgstore/settings.go`
- Create: `internal/adapter/pgstore/dayoffs.go`
- Create: `internal/adapter/pgstore/settings_test.go`
- Create: `internal/adapter/pgstore/dayoffs_test.go`

Settings liefert auch den Timezone-Helper, den Sessions (Task 6) und die
Active-Statemachine (Task 7) für den Buchungstag brauchen (Spec §6: `day` = `started_at`
in User-Zeitzone, Default `Europe/Berlin`).

- [ ] **Step 1: Failing Tests**

```go
// internal/adapter/pgstore/settings_test.go
package pgstore_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func TestSettings_GetSetRoundTrip(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSettings(testStore)
	uid := mustUser(t, "settings-1")

	// fehlender Key → leer, kein Fehler
	v, err := s.Get(uid, "daily_target")
	if err != nil || v != "" {
		t.Fatalf("Get missing: v=%q err=%v", v, err)
	}

	if err := s.Set(uid, "daily_target", "8h"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(uid, "daily_target", "7h"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	v, _ = s.Get(uid, "daily_target")
	if v != "7h" {
		t.Errorf("Get after Set: got %q want 7h", v)
	}

	all, err := s.All(uid)
	if err != nil || all["daily_target"] != "7h" {
		t.Errorf("All: %v %v", all, err)
	}
}

func TestSettings_Location_DefaultBerlin(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSettings(testStore)
	uid := mustUser(t, "settings-2")

	loc := s.Location(uid)
	if loc.String() != "Europe/Berlin" {
		t.Errorf("default location: got %s want Europe/Berlin", loc)
	}

	_ = s.Set(uid, "timezone", "America/New_York")
	if got := s.Location(uid); got.String() != "America/New_York" {
		t.Errorf("custom location: got %s", got)
	}

	// kaputte Zeitzone → Fallback Berlin, kein Panic
	_ = s.Set(uid, "timezone", "Nicht/Existent")
	if got := s.Location(uid); got.String() != "Europe/Berlin" {
		t.Errorf("broken tz fallback: got %s", got)
	}
}
```

```go
// internal/adapter/pgstore/dayoffs_test.go
package pgstore_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestDayOffs_PutListDelete(t *testing.T) {
	t.Parallel()
	d := pgstore.NewDayOffs(testStore)
	uid := mustUser(t, "dayoff-1")

	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer", Target: 0}
	if err := d.Put(uid, off); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Put ist Upsert: Kind ändern
	off.Kind = domain.KindSick
	if err := d.Put(uid, off); err != nil {
		t.Fatalf("Put upsert: %v", err)
	}

	list, err := d.List(uid, 2026)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: err=%v len=%d", err, len(list))
	}
	if list[0].Kind != domain.KindSick || list[0].Label != "Sommer" {
		t.Errorf("roundtrip: %+v", list[0])
	}

	other, _ := d.List(uid, 2025)
	if len(other) != 0 {
		t.Errorf("List 2025 should be empty, got %d", len(other))
	}

	if err := d.Delete(uid, day); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = d.List(uid, 2026)
	if len(list) != 0 {
		t.Errorf("after Delete: %d entries", len(list))
	}
	// Delete ist idempotent
	if err := d.Delete(uid, day); err != nil {
		t.Errorf("Delete idempotent: %v", err)
	}
}
```

- [ ] **Step 2: Tests — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/pgstore/... -run 'TestSettings|TestDayOffs' -timeout 180s 2>&1 | tail -3
```

Expected: `undefined: pgstore.NewSettings`.

- [ ] **Step 3: Implementierung Settings**

```go
// internal/adapter/pgstore/settings.go
package pgstore

import (
	"context"
	"time"
)

// Settings stores per-user key/value settings ("daily_target", "timezone").
type Settings struct{ store *Store }

func NewSettings(s *Store) *Settings { return &Settings{store: s} }

// Get returns the value or "" when the key is unset.
func (s *Settings) Get(userID, key string) (string, error) {
	var v string
	err := s.store.Pool().QueryRow(context.Background(),
		`SELECT value FROM user_settings WHERE user_id = $1 AND key = $2`, userID, key).Scan(&v)
	if err != nil {
		if err.Error() == "no rows in result set" { // pgx.ErrNoRows
			return "", nil
		}
		return "", err
	}
	return v, nil
}

func (s *Settings) Set(userID, key, value string) error {
	_, err := s.store.Pool().Exec(context.Background(), `
		INSERT INTO user_settings (user_id, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value`,
		userID, key, value)
	return err
}

// All returns every setting for the user as a map.
func (s *Settings) All(userID string) (map[string]string, error) {
	rows, err := s.store.Pool().Query(context.Background(),
		`SELECT key, value FROM user_settings WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Location resolves the user's booking timezone (Spec §6: day is computed
// in the user's timezone). Unset or unparsable values fall back to
// Europe/Berlin — booking a session must never fail on a bad setting.
func (s *Settings) Location(userID string) *time.Location {
	tz, err := s.Get(userID, "timezone")
	if err != nil || tz == "" {
		return mustBerlin()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return mustBerlin()
	}
	return loc
}

func mustBerlin() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC // tzdata fehlt im Container? UTC ist der letzte Halt.
	}
	return loc
}
```

WICHTIG: ersetze den String-Vergleich in `Get` durch `errors.Is(err, pgx.ErrNoRows)` mit
Imports `errors` + `github.com/jackc/pgx/v5` (sauberer; der Block oben zeigt die Stelle).

- [ ] **Step 4: Implementierung DayOffs**

```go
// internal/adapter/pgstore/dayoffs.go
package pgstore

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// DayOffs is the server-side day-off store. The client-side
// ports.DayOffStore has no userID in its signatures, so this adapter gets
// its own server shape; the httpserver handlers depend on it directly.
type DayOffs struct{ store *Store }

func NewDayOffs(s *Store) *DayOffs { return &DayOffs{store: s} }

// List returns the user's day-offs within the given year, ordered by day.
func (d *DayOffs) List(userID string, year int) ([]domain.DayOff, error) {
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	rows, err := d.store.Pool().Query(context.Background(), `
		SELECT day, kind, label, target_ns FROM day_offs
		WHERE user_id = $1 AND day >= $2 AND day < $3 ORDER BY day ASC`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DayOff
	for rows.Next() {
		var off domain.DayOff
		var kind string
		var targetNS int64
		if err := rows.Scan(&off.Date, &kind, &off.Label, &targetNS); err != nil {
			return nil, err
		}
		off.Kind = domain.Kind(kind)
		off.Target = time.Duration(targetNS)
		out = append(out, off)
	}
	return out, rows.Err()
}

// Put upserts the day-off for off.Date (date precision).
func (d *DayOffs) Put(userID string, off domain.DayOff) error {
	_, err := d.store.Pool().Exec(context.Background(), `
		INSERT INTO day_offs (user_id, day, kind, label, target_ns)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, day) DO UPDATE
			SET kind = EXCLUDED.kind, label = EXCLUDED.label, target_ns = EXCLUDED.target_ns`,
		userID, off.Date, string(off.Kind), off.Label, int64(off.Target))
	return err
}

// Delete removes the day-off; idempotent.
func (d *DayOffs) Delete(userID string, day time.Time) error {
	_, err := d.store.Pool().Exec(context.Background(),
		`DELETE FROM day_offs WHERE user_id = $1 AND day = $2`, userID, day)
	return err
}
```

- [ ] **Step 5: Tests grün**

```bash
go test ./internal/adapter/pgstore/... -run 'TestSettings|TestDayOffs' -v -timeout 180s 2>&1 | tail -8
```

Expected: alle PASS.

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/adapter/pgstore/
git add internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore): Settings (inkl. Timezone-Resolver) + DayOffs + Tests (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 6: pgstore.Sessions (inkl. Buchungstag in User-Zeitzone + Bulk-Import)

**Files:**
- Create: `internal/adapter/pgstore/sessions.go`
- Create: `internal/adapter/pgstore/sessions_test.go`

Signaturen drop-in zu `sqliteserver.Sessions` (`Upsert(in, expectedVersion) (Session, error)`,
`Delete(userID, id, expectedVersion) error`, `GetByID`, `ListByUserDateRange` **inklusive**
beider Grenzen — die WebUI ruft `ListByUserDateRange(uid, dayStart, dayStart)` für einen
einzelnen Tag auf!). Neu: `BulkUpsert` (idempotenter Import, Spec §7 `:bulk`) und
`bookingDay`-Helper.

- [ ] **Step 1: Failing Tests**

```go
// internal/adapter/pgstore/sessions_test.go
package pgstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func mustProject(t *testing.T, userID, slug string) string {
	t.Helper()
	p, err := pgstore.NewProjects(testStore).EnsureBySlug(userID, slug, slug)
	if err != nil {
		t.Fatalf("mustProject: %v", err)
	}
	return p.ID
}

func mkSession(uid, pid string, start time.Time, dur time.Duration) domain.Session {
	return domain.Session{
		ID: uuid.NewString(), UserID: uid, ProjectID: pid,
		Date:  time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
		Start: start, Stop: start.Add(dur), Elapsed: dur, Version: 0,
	}
}

func TestSessions_UpsertListSingleDay(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-1")
	pid := mustProject(t, uid, "work")

	start := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	in := mkSession(uid, pid, start, time.Hour)
	in.Tag, in.Note = "deep", "fokus"
	saved, err := s.Upsert(in, 0)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if saved.Version != 1 {
		t.Errorf("version: got %d want 1", saved.Version)
	}

	// inklusives Einzeltages-Fenster — exakt der WebUI-Aufruf
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	list, err := s.ListByUserDateRange(uid, day, day)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByUserDateRange single day: err=%v len=%d", err, len(list))
	}
	if list[0].Tag != "deep" || list[0].Elapsed != time.Hour {
		t.Errorf("roundtrip: %+v", list[0])
	}
}

func TestSessions_UpsertOCCAndDelete(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-2")
	pid := mustProject(t, uid, "occ")

	in := mkSession(uid, pid, time.Date(2026, 6, 11, 8, 0, 0, 0, time.UTC), time.Hour)
	saved, _ := s.Upsert(in, 0)

	saved.Note = "edit"
	edited, err := s.Upsert(saved, saved.Version)
	if err != nil || edited.Version != 2 {
		t.Fatalf("edit: err=%v version=%d", err, edited.Version)
	}

	if _, err := s.Upsert(saved, 1); !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("stale upsert: want conflict, got %v", err)
	}

	if err := s.Delete(uid, saved.ID, 1); !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("stale delete: want conflict, got %v", err)
	}
	if err := s.Delete(uid, saved.ID, 2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetByID(uid, saved.ID); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Errorf("after delete: want not found, got %v", err)
	}
}

func TestSessions_BulkUpsertIdempotent(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSessions(testStore)
	uid := mustUser(t, "sess-3")
	pid := mustProject(t, uid, "bulk")

	batch := []domain.Session{
		mkSession(uid, pid, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), time.Hour),
		mkSession(uid, pid, time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC), 2*time.Hour),
	}
	if err := s.BulkUpsert(batch); err != nil {
		t.Fatalf("BulkUpsert: %v", err)
	}
	// Re-Run mit denselben IDs ist ein No-op, kein Fehler, keine Duplikate
	if err := s.BulkUpsert(batch); err != nil {
		t.Fatalf("BulkUpsert rerun: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	list, _ := s.ListByUserDateRange(uid, from, to)
	if len(list) != 2 {
		t.Errorf("after bulk rerun: want 2 sessions, got %d", len(list))
	}
}

func TestBookingDay_UserTimezone(t *testing.T) {
	t.Parallel()
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// 22:30 UTC am 11.6. = 00:30 am 12.6. in Berlin (CEST) → Buchungstag 12.6.
	started := time.Date(2026, 6, 11, 22, 30, 0, 0, time.UTC)
	day := pgstore.BookingDay(started, berlin)
	if day.Format("2006-01-02") != "2026-06-12" {
		t.Errorf("BookingDay: got %s want 2026-06-12", day.Format("2006-01-02"))
	}
}
```

- [ ] **Step 2: Tests — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/pgstore/... -run 'TestSessions|TestBookingDay' -timeout 180s 2>&1 | tail -3
```

Expected: `undefined: pgstore.NewSessions`.

- [ ] **Step 3: Implementierung**

```go
// internal/adapter/pgstore/sessions.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Sessions mirrors the sqliteserver.Sessions surface (minus PullSince) on PG.
type Sessions struct{ store *Store }

func NewSessions(s *Store) *Sessions { return &Sessions{store: s} }

const sessionCols = `id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at`

// BookingDay maps a wall-clock instant to the user's booking day
// (Spec §6: day is computed from started_at in the user's timezone).
func BookingDay(startedAt time.Time, loc *time.Location) time.Time {
	local := startedAt.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// ListByUserDateRange returns sessions whose day lies in [from, to]
// (both inclusive — the WebUI queries single days as from==to).
func (s *Sessions) ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error) {
	rows, err := s.store.Pool().Query(context.Background(),
		`SELECT `+sessionCols+` FROM sessions
		 WHERE user_id = $1 AND day >= $2 AND day <= $3
		 ORDER BY day ASC, started_at ASC`,
		userID, dateOnly(from), dateOnly(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Sessions) GetByID(userID, id string) (domain.Session, error) {
	row := s.store.Pool().QueryRow(context.Background(),
		`SELECT `+sessionCols+` FROM sessions WHERE user_id = $1 AND id = $2`, userID, id)
	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrSessionNotFound
	}
	return sess, err
}

// Upsert writes with OCC (expectedVersion 0 = insert-only) and returns the
// saved row. The day column comes from in.Date — callers (stop handler,
// manual create) have already computed the booking day via BookingDay.
func (s *Sessions) Upsert(in domain.Session, expectedVersion int64) (domain.Session, error) {
	ctx := context.Background()
	if expectedVersion == 0 {
		row := s.store.Pool().QueryRow(ctx, `
			INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
			ON CONFLICT (id) DO NOTHING
			RETURNING `+sessionCols,
			in.ID, in.UserID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop, in.Tag, in.Note)
		out, err := scanSession(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Session{}, ports.ErrSessionVersionConflict
		}
		return out, err
	}
	row := s.store.Pool().QueryRow(ctx, `
		UPDATE sessions
		SET project_id = $3, day = $4, started_at = $5, stopped_at = $6,
		    tag = $7, note = $8, version = version + 1, updated_at = now()
		WHERE user_id = $1 AND id = $2 AND version = $9
		RETURNING `+sessionCols,
		in.UserID, in.ID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop,
		in.Tag, in.Note, expectedVersion)
	out, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrSessionVersionConflict
	}
	return out, err
}

// BulkUpsert is the idempotent import path (Spec §7 sessions:bulk): rows
// whose ID already exists are skipped, never overwritten — re-running an
// import must not clobber server-side edits.
func (s *Sessions) BulkUpsert(sessions []domain.Session) error {
	ctx := context.Background()
	tx, err := s.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, in := range sessions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
			ON CONFLICT (id) DO NOTHING`,
			in.ID, in.UserID, in.ProjectID, dateOnly(in.Date), in.Start, in.Stop, in.Tag, in.Note); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Delete removes a session with OCC. Matches sqliteserver semantics:
// missing row → ErrSessionNotFound, version mismatch → conflict.
func (s *Sessions) Delete(userID, id string, expectedVersion int64) error {
	ctx := context.Background()
	tag, err := s.store.Pool().Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND id = $2 AND version = $3`,
		userID, id, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, gerr := s.GetByID(userID, id); errors.Is(gerr, ports.ErrSessionNotFound) {
			return ports.ErrSessionNotFound
		}
		return ports.ErrSessionVersionConflict
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func scanSession(r rowScanner) (domain.Session, error) {
	var out domain.Session
	if err := r.Scan(&out.ID, &out.UserID, &out.ProjectID, &out.Date,
		&out.Start, &out.Stop, &out.Tag, &out.Note, &out.Version, &out.UpdatedAt); err != nil {
		return domain.Session{}, err
	}
	out.Elapsed = out.Stop.Sub(out.Start)
	return out, nil
}
```

- [ ] **Step 4: Tests grün**

```bash
go test ./internal/adapter/pgstore/... -run 'TestSessions|TestBookingDay' -v -timeout 180s 2>&1 | tail -10
```

Expected: alle PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/adapter/pgstore/
git add internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore): Sessions mit OCC, BulkUpsert + BookingDay in User-TZ (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 7: Pause-Felder in domain.ActiveSession + pgstore.ActiveSessions-Statemachine

**Files:**
- Modify: `internal/domain/active_session.go`
- Create: `internal/adapter/pgstore/active_sessions.go`
- Create: `internal/adapter/pgstore/active_sessions_test.go`

Start/Stop behalten die sqliteserver-Signaturen (drop-in für WebUI-Handler); Pause/Resume
sind neu und **idempotent ohne expectedVersion** (Spec §7: „Pause-Statemachine, idempotent").
Stop bucht `elapsed = now − started_at − pause_total` (eine offene Pause endet mit dem Stop)
und berechnet den Buchungstag in der User-Zeitzone.

- [ ] **Step 1: Domain-Felder + Elapsed-Methode**

In `internal/domain/active_session.go` den Struct erweitern (nach `StartedAt`):

```go
	// PausedAt is non-nil while the session is paused (server-set on
	// Pause, cleared on Resume). Nil = running.
	PausedAt *time.Time
	// PauseTotal accumulates completed pause intervals. It does NOT
	// include a currently-open pause — Elapsed() handles that.
	PauseTotal time.Duration
```

Und am Datei-Ende die Methode ergänzen:

```go
// Elapsed returns the worked duration at instant now: wall time since
// start minus completed pauses minus a currently-open pause. This is THE
// canonical elapsed formula (Spec §7 stop semantics) — every surface
// (API stop, WebUI banner, später TUI) MUST use it instead of computing
// now.Sub(StartedAt) by hand.
func (a ActiveSession) Elapsed(now time.Time) time.Duration {
	e := now.Sub(a.StartedAt) - a.PauseTotal
	if a.PausedAt != nil {
		e -= now.Sub(*a.PausedAt)
	}
	if e < 0 {
		return 0
	}
	return e
}
```

- [ ] **Step 2: Failing Tests**

```go
// internal/adapter/pgstore/active_sessions_test.go
package pgstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestActive_StartStopCycle(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-1")
	pid := mustProject(t, uid, "active-work")

	as, err := a.Start(uid, pid, time.Time{}, "laptop", 0, "deep", "fokus")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if as.Version != 1 || as.Tag != "deep" || as.PausedAt != nil {
		t.Fatalf("unexpected active: %+v", as)
	}

	// Doppel-Start auf dasselbe Projekt → Konflikt (Spec §7: 409)
	if _, err := a.Start(uid, pid, time.Time{}, "phone", 0, "", ""); !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("double start: want conflict, got %v", err)
	}

	sess, err := a.Stop(uid, pid, as.Version, "", "")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if sess.Elapsed <= 0 || sess.Tag != "deep" {
		t.Errorf("stopped session: %+v", sess)
	}
	if sess.Version != 1 {
		t.Errorf("session version after stop: got %d want 1", sess.Version)
	}

	// Stop ohne aktive Session → NotFound
	if _, err := a.Stop(uid, pid, 1, "", ""); !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("double stop: want not found, got %v", err)
	}
}

func TestActive_PauseResumeIdempotent(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-2")
	pid := mustProject(t, uid, "pause-work")

	started, _ := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")

	paused, err := a.Pause(uid, pid)
	if err != nil || paused.PausedAt == nil {
		t.Fatalf("Pause: err=%v PausedAt=%v", err, paused.PausedAt)
	}
	if paused.Version <= started.Version {
		t.Errorf("Pause must bump version: %d <= %d", paused.Version, started.Version)
	}

	// Pause auf pausierter Session → idempotent, gleicher Zustand
	paused2, err := a.Pause(uid, pid)
	if err != nil || paused2.PausedAt == nil || paused2.Version != paused.Version {
		t.Errorf("Pause idempotent: err=%v %+v", err, paused2)
	}

	resumed, err := a.Resume(uid, pid)
	if err != nil || resumed.PausedAt != nil {
		t.Fatalf("Resume: err=%v PausedAt=%v", err, resumed.PausedAt)
	}
	if resumed.PauseTotal <= 0 {
		t.Errorf("PauseTotal after resume: %v", resumed.PauseTotal)
	}

	// Resume ohne Pause → idempotent
	resumed2, err := a.Resume(uid, pid)
	if err != nil || resumed2.Version != resumed.Version {
		t.Errorf("Resume idempotent: err=%v %+v", err, resumed2)
	}

	// Pause/Resume ohne aktive Session → NotFound
	if _, err := a.Pause(uid, "00000000-0000-0000-0000-000000000009"); !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("pause w/o active: want not found, got %v", err)
	}
}

func TestActive_StopDuringPauseEndsPause(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-3")
	pid := mustProject(t, uid, "pausestop")

	started, _ := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")
	paused, _ := a.Pause(uid, pid)

	sess, err := a.Stop(uid, pid, paused.Version, "", "")
	if err != nil {
		t.Fatalf("Stop while paused: %v", err)
	}
	// elapsed = Wandzeit − Pausen; bei sofortigem Pause→Stop nahe 0, nie negativ
	if sess.Elapsed < 0 {
		t.Errorf("elapsed negative: %v", sess.Elapsed)
	}
	wall := sess.Stop.Sub(started.StartedAt)
	if sess.Elapsed > wall {
		t.Errorf("elapsed %v exceeds wall time %v", sess.Elapsed, wall)
	}
}
```

- [ ] **Step 3: Tests — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/pgstore/... -run TestActive_ -timeout 180s 2>&1 | tail -3
```

Expected: `undefined: pgstore.NewActiveSessions`.

- [ ] **Step 4: Implementierung**

```go
// internal/adapter/pgstore/active_sessions.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions is the server-side worktime statemachine
// (start/stop/pause/resume — Spec §7). Start/Stop keep the sqliteserver
// signatures so WebUI handlers swap without churn; Pause/Resume are new
// and idempotent (no expectedVersion — the server is the only writer of
// pause state and the endpoints are defined idempotent).
type ActiveSessions struct {
	store    *Store
	sessions *Sessions
	settings *Settings
}

func NewActiveSessions(s *Store, sessions *Sessions, settings *Settings) *ActiveSessions {
	return &ActiveSessions{store: s, sessions: sessions, settings: settings}
}

const activeCols = `user_id, project_id, started_at, paused_at, pause_total_ns, started_on_device, tag, note, version`

// Start creates the active row. expectedVersion 0 = "must not exist";
// any existing row for (user, project) is a conflict (Spec §7 → 409).
// startedAt zero value → server time now (Server-Zeit, nie Client-Zeit).
func (a *ActiveSessions) Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error) {
	ctx := context.Background()
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	startedAt = startedAt.UTC()
	if expectedVersion != 0 {
		return domain.ActiveSession{}, ports.ErrActiveSessionConflict
	}
	row := a.store.Pool().QueryRow(ctx, `
		INSERT INTO active_sessions (user_id, project_id, started_at, started_on_device, tag, note, version)
		VALUES ($1, $2, $3, $4, $5, $6, 1)
		ON CONFLICT (user_id, project_id) DO NOTHING
		RETURNING `+activeCols,
		userID, projectID, startedAt, device, tag, note)
	out, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ActiveSession{}, ports.ErrActiveSessionConflict
	}
	return out, err
}

// Stop atomically converts the active row into a finished session.
// elapsed = now − started_at − pause_total (eine offene Pause endet mit
// dem Stop, Spec §7). Booking day: started_at in the user's timezone.
// Empty tag/note keep the stored values.
func (a *ActiveSessions) Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error) {
	ctx := context.Background()
	tx, err := a.store.Pool().Begin(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`SELECT `+activeCols+` FROM active_sessions
		 WHERE user_id = $1 AND project_id = $2 FOR UPDATE`, userID, projectID)
	cur, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ports.ErrActiveSessionNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	if cur.Version != expectedVersion {
		return domain.Session{}, ports.ErrActiveSessionConflict
	}
	if tag == "" {
		tag = cur.Tag
	}
	if note == "" {
		note = cur.Note
	}

	now := time.Now().UTC()
	elapsed := cur.Elapsed(now)
	stop := cur.StartedAt.Add(cur.PauseTotal).Add(elapsed)
	if cur.PausedAt != nil {
		// Offene Pause zählt als Pause bis zum Stop: stop = jetzt.
		stop = now
	}
	day := BookingDay(cur.StartedAt, a.settings.Location(userID))

	sess := domain.Session{
		ID: uuid.NewString(), UserID: userID, ProjectID: projectID,
		Date: day, Start: cur.StartedAt, Stop: stop, Elapsed: elapsed,
		Tag: tag, Note: note,
	}
	insRow := tx.QueryRow(ctx, `
		INSERT INTO sessions (id, user_id, project_id, day, started_at, stopped_at, tag, note, version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, now())
		RETURNING `+sessionCols, sess.ID, sess.UserID, sess.ProjectID, sess.Date, sess.Start, sess.Stop, sess.Tag, sess.Note)
	saved, err := scanSession(insRow)
	if err != nil {
		return domain.Session{}, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM active_sessions WHERE user_id = $1 AND project_id = $2`,
		userID, projectID); err != nil {
		return domain.Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Session{}, err
	}
	saved.Elapsed = elapsed // Wahrheit der Statemachine, nicht stop−start
	return saved, nil
}

// Pause sets paused_at if running; already-paused is a no-op returning the
// current row. Missing row → ErrActiveSessionNotFound.
func (a *ActiveSessions) Pause(userID, projectID string) (domain.ActiveSession, error) {
	ctx := context.Background()
	tag, err := a.store.Pool().Exec(ctx, `
		UPDATE active_sessions SET paused_at = now(), version = version + 1
		WHERE user_id = $1 AND project_id = $2 AND paused_at IS NULL`,
		userID, projectID)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	_ = tag
	return a.Get(userID, projectID)
}

// Resume folds the open pause into pause_total_ns and clears paused_at;
// not-paused is a no-op returning the current row.
func (a *ActiveSessions) Resume(userID, projectID string) (domain.ActiveSession, error) {
	ctx := context.Background()
	_, err := a.store.Pool().Exec(ctx, `
		UPDATE active_sessions
		SET pause_total_ns = pause_total_ns
		      + (EXTRACT(EPOCH FROM (now() - paused_at)) * 1e9)::bigint,
		    paused_at = NULL,
		    version = version + 1
		WHERE user_id = $1 AND project_id = $2 AND paused_at IS NOT NULL`,
		userID, projectID)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	return a.Get(userID, projectID)
}

func (a *ActiveSessions) ListByUser(userID string) ([]domain.ActiveSession, error) {
	rows, err := a.store.Pool().Query(context.Background(),
		`SELECT `+activeCols+` FROM active_sessions WHERE user_id = $1 ORDER BY started_at ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ActiveSession
	for rows.Next() {
		as, err := scanActive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, as)
	}
	return out, rows.Err()
}

func (a *ActiveSessions) Get(userID, projectID string) (domain.ActiveSession, error) {
	row := a.store.Pool().QueryRow(context.Background(),
		`SELECT `+activeCols+` FROM active_sessions WHERE user_id = $1 AND project_id = $2`,
		userID, projectID)
	out, err := scanActive(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ActiveSession{}, ports.ErrActiveSessionNotFound
	}
	return out, err
}

func scanActive(r rowScanner) (domain.ActiveSession, error) {
	var out domain.ActiveSession
	var pausedAt *time.Time
	var pauseNS int64
	if err := r.Scan(&out.UserID, &out.ProjectID, &out.StartedAt, &pausedAt, &pauseNS,
		&out.StartedOnDevice, &out.Tag, &out.Note, &out.Version); err != nil {
		return domain.ActiveSession{}, err
	}
	out.PausedAt = pausedAt
	out.PauseTotal = time.Duration(pauseNS)
	return out, nil
}
```

- [ ] **Step 5: Alle pgstore-Tests grün + bestehende Domain-Tests**

```bash
go build ./... && go test ./internal/domain/... ./internal/adapter/pgstore/... -timeout 300s 2>&1 | tail -5
```

Expected: PASS (das Hinzufügen der Domain-Felder darf nichts brechen — Felder sind additiv).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/domain/active_session.go internal/adapter/pgstore/
git add internal/domain/active_session.go internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore,domain): ActiveSessions-Statemachine mit Pause/Resume (R1)

domain.ActiveSession trägt PausedAt/PauseTotal + die kanonische
Elapsed(now)-Formel; Stop bucht elapsed = now − started − pausen und den
Buchungstag in der User-Zeitzone.

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 8: ports.DocumentStore + pgstore.Documents (PG-FTS)

**Files:**
- Create: `internal/ports/documents.go`
- Create: `internal/adapter/pgstore/documents.go`
- Create: `internal/adapter/pgstore/documents_test.go`

- [ ] **Step 1: Port definieren**

```go
// internal/ports/documents.go
package ports

import "time"

// Document is a server-side markdown document (Spec §6). Path is the
// '/-separated relative location ("projects/serverkraken/flow/ideen.md");
// RepoKey is non-empty only for repo notes ("git:github.com/foo/bar").
type Document struct {
	ID        string
	UserID    string
	Path      string
	Body      string
	RepoKey   string
	Version   int64
	UpdatedAt time.Time
}

// DocumentEntry is the body-less list/search row.
type DocumentEntry struct {
	Path      string
	RepoKey   string
	Version   int64
	UpdatedAt time.Time
	Snippet   string // FTS-Headline bei Suche, sonst leer
}

// DocumentStore persists markdown documents in the server DB (Spec §6/§7).
type DocumentStore interface {
	// Get returns the document at path or ErrDocumentNotFound.
	Get(userID, path string) (Document, error)
	// GetByRepoKey returns the repo-note alias target or ErrDocumentNotFound.
	GetByRepoKey(userID, repoKey string) (Document, error)
	// List returns entries under prefix (both may be empty), optionally
	// FTS-filtered by query, ordered by path. limit <= 0 → default 200.
	List(userID, prefix, query string, limit int) ([]DocumentEntry, error)
	// Put upserts with If-Match semantics: ifMatch 0 = create-only
	// (conflict when the path exists), N = update-only-if-version-is-N.
	Put(userID, path, body, repoKey string, ifMatch int64) (Document, error)
	// Delete removes the document; idempotent.
	Delete(userID, path string) error
}

// ErrDocumentNotFound is returned when no document exists at (user, path/key).
var ErrDocumentNotFound = errSentinel("flow: document not found")

// ErrDocumentVersionConflict is returned by Put on If-Match mismatch.
var ErrDocumentVersionConflict = errSentinel("flow: document version conflict")
```

- [ ] **Step 2: Failing Tests**

```go
// internal/adapter/pgstore/documents_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestDocuments_PutGetUpdateDelete(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-1")

	created, err := docs.Put(uid, "projects/flow/ideen.md", "# Ideen", "", 0)
	if err != nil || created.Version != 1 {
		t.Fatalf("create: err=%v %+v", err, created)
	}

	// create-only auf existierenden Pfad → Konflikt
	if _, err := docs.Put(uid, "projects/flow/ideen.md", "x", "", 0); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("create on existing: want conflict, got %v", err)
	}

	updated, err := docs.Put(uid, "projects/flow/ideen.md", "# Ideen v2", "", 1)
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: err=%v %+v", err, updated)
	}

	// stale If-Match → Konflikt
	if _, err := docs.Put(uid, "projects/flow/ideen.md", "stale", "", 1); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("stale update: want conflict, got %v", err)
	}

	got, err := docs.Get(uid, "projects/flow/ideen.md")
	if err != nil || got.Body != "# Ideen v2" {
		t.Fatalf("get: err=%v body=%q", err, got.Body)
	}

	if err := docs.Delete(uid, "projects/flow/ideen.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := docs.Get(uid, "projects/flow/ideen.md"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("after delete: want not found, got %v", err)
	}
	if err := docs.Delete(uid, "projects/flow/ideen.md"); err != nil {
		t.Errorf("delete idempotent: %v", err)
	}
}

func TestDocuments_RepoKeyAlias(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-2")

	key := "git:github.com/serverkraken/flow"
	path := "repos/git%3Agithub.com%2Fserverkraken%2Fflow.md"
	if _, err := docs.Put(uid, path, "repo note", key, 0); err != nil {
		t.Fatalf("put repo note: %v", err)
	}
	got, err := docs.GetByRepoKey(uid, key)
	if err != nil || got.Path != path || got.RepoKey != key {
		t.Fatalf("GetByRepoKey: err=%v %+v", err, got)
	}
	if _, err := docs.GetByRepoKey(uid, "git:github.com/nope/nope"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("missing key: want not found, got %v", err)
	}
}

func TestDocuments_ListPrefixAndFTS(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-3")

	seed := map[string]string{
		"daily/2026-06-10.md":     "standup kubernetes cluster",
		"daily/2026-06-11.md":     "postgres migration notes",
		"projects/flow/arch.md":   "kubernetes deployment der webui",
		"projects/flow/random.md": "nichts besonderes",
	}
	for p, body := range seed {
		if _, err := docs.Put(uid, p, body, "", 0); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	byPrefix, err := docs.List(uid, "daily/", "", 0)
	if err != nil || len(byPrefix) != 2 {
		t.Fatalf("prefix list: err=%v len=%d", err, len(byPrefix))
	}

	byQuery, err := docs.List(uid, "", "kubernetes", 0)
	if err != nil || len(byQuery) != 2 {
		t.Fatalf("fts list: err=%v len=%d", err, len(byQuery))
	}

	both, err := docs.List(uid, "projects/", "kubernetes", 0)
	if err != nil || len(both) != 1 {
		t.Fatalf("prefix+fts: err=%v len=%d", err, len(both))
	}

	limited, _ := docs.List(uid, "", "", 2)
	if len(limited) != 2 {
		t.Errorf("limit: want 2, got %d", len(limited))
	}
}
```

- [ ] **Step 3: Tests — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/pgstore/... -run TestDocuments -timeout 180s 2>&1 | tail -3
```

Expected: `undefined: pgstore.NewDocuments`.

- [ ] **Step 4: Implementierung**

```go
// internal/adapter/pgstore/documents.go
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/serverkraken/flow/internal/ports"
)

// Documents implements ports.DocumentStore with Postgres-FTS
// ('simple'-Konfiguration, websearch_to_tsquery — Spec §6).
type Documents struct{ store *Store }

func NewDocuments(s *Store) *Documents { return &Documents{store: s} }

var _ ports.DocumentStore = (*Documents)(nil)

const documentCols = `id, user_id, path, body, COALESCE(repo_key, ''), version, updated_at`

const defaultListLimit = 200

func (d *Documents) Get(userID, path string) (ports.Document, error) {
	row := d.store.Pool().QueryRow(context.Background(),
		`SELECT `+documentCols+` FROM documents WHERE user_id = $1 AND path = $2`,
		userID, path)
	return scanDocument(row)
}

func (d *Documents) GetByRepoKey(userID, repoKey string) (ports.Document, error) {
	row := d.store.Pool().QueryRow(context.Background(),
		`SELECT `+documentCols+` FROM documents WHERE user_id = $1 AND repo_key = $2`,
		userID, repoKey)
	return scanDocument(row)
}

func (d *Documents) List(userID, prefix, query string, limit int) ([]ports.DocumentEntry, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	args := []any{userID}
	conds := []string{"user_id = $1"}
	order := "path ASC"
	snippet := "''"
	if prefix != "" {
		args = append(args, prefix+"%")
		conds = append(conds, fmt.Sprintf("path LIKE $%d", len(args)))
	}
	if query != "" {
		args = append(args, query)
		conds = append(conds, fmt.Sprintf("search @@ websearch_to_tsquery('simple', $%d)", len(args)))
		snippet = fmt.Sprintf("ts_headline('simple', body, websearch_to_tsquery('simple', $%d), 'MaxWords=18,MinWords=8')", len(args))
		order = fmt.Sprintf("ts_rank(search, websearch_to_tsquery('simple', $%d)) DESC, path ASC", len(args))
	}
	args = append(args, limit)
	q := fmt.Sprintf(
		`SELECT path, COALESCE(repo_key, ''), version, updated_at, %s
		 FROM documents WHERE %s ORDER BY %s LIMIT $%d`,
		snippet, strings.Join(conds, " AND "), order, len(args))
	rows, err := d.store.Pool().Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.DocumentEntry
	for rows.Next() {
		var e ports.DocumentEntry
		if err := rows.Scan(&e.Path, &e.RepoKey, &e.Version, &e.UpdatedAt, &e.Snippet); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *Documents) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	ctx := context.Background()
	var repoKeyArg *string
	if repoKey != "" {
		repoKeyArg = &repoKey
	}
	if ifMatch == 0 {
		row := d.store.Pool().QueryRow(ctx, `
			INSERT INTO documents (user_id, path, body, repo_key)
			VALUES ($1, $2, $3, $4)
			RETURNING `+documentCols,
			userID, path, body, repoKeyArg)
		doc, err := scanDocument(row)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ports.Document{}, ports.ErrDocumentVersionConflict
		}
		return doc, err
	}
	row := d.store.Pool().QueryRow(ctx, `
		UPDATE documents
		SET body = $3, repo_key = COALESCE($4, repo_key), version = version + 1, updated_at = now()
		WHERE user_id = $1 AND path = $2 AND version = $5
		RETURNING `+documentCols,
		userID, path, body, repoKeyArg, ifMatch)
	doc, err := scanDocument(row)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	return doc, err
}

func (d *Documents) Delete(userID, path string) error {
	_, err := d.store.Pool().Exec(context.Background(),
		`DELETE FROM documents WHERE user_id = $1 AND path = $2`, userID, path)
	return err
}

func scanDocument(r rowScanner) (ports.Document, error) {
	var out ports.Document
	err := r.Scan(&out.ID, &out.UserID, &out.Path, &out.Body, &out.RepoKey, &out.Version, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	if err != nil {
		return ports.Document{}, err
	}
	return out, nil
}
```

- [ ] **Step 5: Tests grün + ci-Checkpoint**

```bash
go test ./internal/adapter/pgstore/... -v -timeout 300s 2>&1 | tail -10
make ci 2>&1 | tail -3
```

Expected: alle pgstore-Tests PASS, `make ci` grün (neuer, gut getesteter Code hebt die
Coverage eher).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/ports/documents.go internal/adapter/pgstore/
git add internal/ports/documents.go internal/adapter/pgstore/
git commit -m "$(cat <<'EOF'
feat(pgstore,ports): DocumentStore-Port + PG-FTS-Implementierung (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 9: Bearer-API — Worktime (sessions CRUD + bulk, active start/stop/pause/resume)

**Files:**
- Modify: `internal/webui/sse/broadcaster.go` (Changed-Helper)
- Create: `internal/adapter/httpserver/worktime_api.go`
- Create: `internal/adapter/httpserver/main_pg_test.go`
- Create: `internal/adapter/httpserver/worktime_api_test.go`

Die neuen Handler werden hier NUR gebaut + getestet; gemountet werden sie in Task 18
(gleichzeitig mit dem Entfernen der alten pull/push-Routen). API-Konventionen (Spec §7):
JSON mit snake_case-DTOs, `If-Match`-Header als nackte Integer-Version, Statuscodes
401/403/404/**409** (ActiveSession existiert)/**412** (If-Match-Mismatch)/422 (Validierung).

- [ ] **Step 1: sse.Changed-Helper**

Ans Ende von `internal/webui/sse/broadcaster.go` anfügen:

```go
// Changed publishes the generalized cross-client invalidation event
// (Spec §7 /events): every successful write fans out
// `changed {resource}` so TUI/MCP/Browser refetch the resource. The
// legacy fine-grained session.*/project.*/note.* events stay for the
// WebUI's own HTMX swaps — Changed is the cross-device contract.
func (b *Broadcaster) Changed(userID, resource string) {
	b.Publish(userID, Event{Type: "changed", Data: map[string]string{"resource": resource}})
}
```

- [ ] **Step 2: TestMain für das httpserver-Paket (PG-Container)**

```go
// internal/adapter/httpserver/main_pg_test.go
package httpserver

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// pgTestStore backs the new R1 API handler tests. One container per
// package; isolation via per-test users.
var pgTestStore *pgstore.Store

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx := context.Background()
		dsn, terminate, err := pgtest.StartContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer terminate()
		s, err := pgstore.Open(ctx, dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgstore open:", err)
			return 1
		}
		defer s.Close()
		pgTestStore = s
		return m.Run()
	}())
}
```

- [ ] **Step 3: Failing Tests**

```go
// internal/adapter/httpserver/worktime_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

type worktimeAPIEnv struct {
	user     domain.User
	projID   string
	router   chi.Router
}

// newWorktimeAPIEnv wires the new API handlers onto a bare chi router with
// the test user pre-injected — mirrors what Task 18 mounts in production.
func newWorktimeAPIEnv(t *testing.T, sub string) worktimeAPIEnv {
	t.Helper()
	users := pgstore.NewUsers(pgTestStore)
	u, err := users.EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	proj, err := pgstore.NewProjects(pgTestStore).EnsureBySlug(u.ID, "Work", "work")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	deps := WorktimeAPIDeps{
		Sessions: pgstore.NewSessions(pgTestStore),
		Active:   pgstore.NewActiveSessions(pgTestStore, pgstore.NewSessions(pgTestStore), pgstore.NewSettings(pgTestStore)),
		Settings: pgstore.NewSettings(pgTestStore),
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountWorktimeAPI(r, deps)
	return worktimeAPIEnv{user: u, projID: proj.ID, router: r}
}

func (e worktimeAPIEnv) do(t *testing.T, method, path string, body any, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func TestWorktimeAPI_ActiveLifecycle(t *testing.T) {
	e := newWorktimeAPIEnv(t, "api-active-1")

	// Start
	rec := e.do(t, "POST", "/worktime/active/start",
		map[string]string{"project_id": e.projID, "tag": "deep"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	// Doppel-Start → 409 (Spec §7)
	rec = e.do(t, "POST", "/worktime/active/start",
		map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("double start: want 409, got %d", rec.Code)
	}
	// GET active
	rec = e.do(t, "GET", "/worktime/active", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("active list: %d", rec.Code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("active items: %d", len(list.Items))
	}
	// Pause → paused_at gesetzt; idempotent
	rec = e.do(t, "POST", "/worktime/active/pause", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(t, "POST", "/worktime/active/pause", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause idempotent: %d", rec.Code)
	}
	// Resume
	rec = e.do(t, "POST", "/worktime/active/resume", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume: %d", rec.Code)
	}
	// Stop → Session-DTO
	rec = e.do(t, "POST", "/worktime/active/stop", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body)
	}
	var sess map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess["id"] == "" || sess["project_id"] != e.projID {
		t.Errorf("stop payload: %v", sess)
	}
	// Stop ohne aktive → 404
	rec = e.do(t, "POST", "/worktime/active/stop", map[string]string{"project_id": e.projID}, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("double stop: want 404, got %d", rec.Code)
	}
}

func TestWorktimeAPI_SessionsCRUDAndBulk(t *testing.T) {
	e := newWorktimeAPIEnv(t, "api-sess-1")

	// Manuelle Session (Nachtrag)
	create := map[string]any{
		"project_id": e.projID,
		"started_at": "2026-06-10T09:00:00Z",
		"stopped_at": "2026-06-10T10:30:00Z",
		"tag":        "deep",
	}
	rec := e.do(t, "POST", "/worktime/sessions", create, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" || created["day"] != "2026-06-10" {
		t.Fatalf("create payload: %v", created)
	}

	// Liste im Zeitraum
	rec = e.do(t, "GET", "/worktime/sessions?from=2026-06-10&to=2026-06-10", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	// Validierung: kaputtes from → 422
	rec = e.do(t, "GET", "/worktime/sessions?from=gestern&to=2026-06-10", nil, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad from: want 422, got %d", rec.Code)
	}

	// PUT mit If-Match
	update := map[string]any{
		"project_id": e.projID,
		"started_at": "2026-06-10T09:00:00Z",
		"stopped_at": "2026-06-10T11:00:00Z",
		"note":       "korrigiert",
	}
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// Stale If-Match → 412 + current
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale put: want 412, got %d", rec.Code)
	}
	// Fehlender If-Match → 422
	rec = e.do(t, "PUT", "/worktime/sessions/"+id, update, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing if-match: want 422, got %d", rec.Code)
	}

	// DELETE mit If-Match
	rec = e.do(t, "DELETE", "/worktime/sessions/"+id, nil, map[string]string{"If-Match": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}

	// Bulk idempotent (Client-UUIDv5)
	bulk := map[string]any{"sessions": []map[string]any{
		{"id": "8c5e9b7e-0000-5000-8000-000000000001", "project_id": e.projID,
			"started_at": "2026-01-05T09:00:00Z", "stopped_at": "2026-01-05T10:00:00Z"},
	}}
	for i := 0; i < 2; i++ {
		rec = e.do(t, "POST", "/worktime/sessions:bulk", bulk, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("bulk run %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	rec = e.do(t, "GET", "/worktime/sessions?from=2026-01-01&to=2026-01-31", nil, nil)
	var page struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 {
		t.Errorf("bulk idempotency: want 1 session, got %d", len(page.Items))
	}
	fmt.Sprint() // keep fmt import
}
```

- [ ] **Step 4: Test — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/httpserver/ -run TestWorktimeAPI -timeout 300s 2>&1 | tail -3
```

Expected: `undefined: WorktimeAPIDeps` / `undefined: MountWorktimeAPI`.

- [ ] **Step 5: Implementierung**

```go
// internal/adapter/httpserver/worktime_api.go
//
// R1 Bearer-API für Worktime (Spec §7). Ersetzt die alten pull/push-Sync-
// Routen. DTOs sind snake_case-JSON; If-Match trägt die nackte Versions-
// zahl; 412 = Version-Mismatch, 409 = ActiveSession existiert bereits.
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// WorktimeSessionsStore is the narrow store surface the worktime API needs.
// Satisfied by *pgstore.Sessions.
type WorktimeSessionsStore interface {
	ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error)
	GetByID(userID, id string) (domain.Session, error)
	Upsert(in domain.Session, expectedVersion int64) (domain.Session, error)
	BulkUpsert(sessions []domain.Session) error
	Delete(userID, id string, expectedVersion int64) error
}

// WorktimeActiveStore is the statemachine surface. Satisfied by
// *pgstore.ActiveSessions.
type WorktimeActiveStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error)
	Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
	Pause(userID, projectID string) (domain.ActiveSession, error)
	Resume(userID, projectID string) (domain.ActiveSession, error)
}

// TimezoneResolver liefert die Buchungs-Zeitzone des Users (pgstore.Settings).
type TimezoneResolver interface {
	Location(userID string) *time.Location
}

// WorktimeAPIDeps bundles the worktime API dependencies. Bus is optional
// (nil = no SSE fan-out, e.g. in focused handler tests).
type WorktimeAPIDeps struct {
	Sessions WorktimeSessionsStore
	Active   WorktimeActiveStore
	Settings TimezoneResolver
	Bus      *sse.Broadcaster
}

func (d WorktimeAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "worktime")
	}
}

// MountWorktimeAPI registers the §7 worktime routes on r. The caller wraps
// r in the bearer middleware (Task 18).
func MountWorktimeAPI(r chi.Router, d WorktimeAPIDeps) {
	r.Get("/worktime/sessions", d.handleSessionsList)
	r.Post("/worktime/sessions", d.handleSessionCreate)
	r.Post("/worktime/sessions:bulk", d.handleSessionsBulk)
	r.Put("/worktime/sessions/{id}", d.handleSessionPut)
	r.Delete("/worktime/sessions/{id}", d.handleSessionDelete)
	r.Get("/worktime/active", d.handleActiveList)
	r.Post("/worktime/active/start", d.handleActiveStart)
	r.Post("/worktime/active/stop", d.handleActiveStop)
	r.Post("/worktime/active/pause", d.handleActivePause)
	r.Post("/worktime/active/resume", d.handleActiveResume)
}

// — DTOs ---------------------------------------------------------------------

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

func toSessionDTO(s domain.Session) sessionDTO {
	return sessionDTO{
		ID: s.ID, ProjectID: s.ProjectID, Day: s.Date.Format("2006-01-02"),
		StartedAt: s.Start, StoppedAt: s.Stop, Tag: s.Tag, Note: s.Note,
		Version: s.Version, UpdatedAt: s.UpdatedAt,
	}
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

func toActiveDTO(a domain.ActiveSession) activeDTO {
	return activeDTO{
		ProjectID: a.ProjectID, StartedAt: a.StartedAt, PausedAt: a.PausedAt,
		PauseTotalMS: a.PauseTotal.Milliseconds(), StartedOnDevice: a.StartedOnDevice,
		Tag: a.Tag, Note: a.Note, Version: a.Version,
	}
}

type sessionWriteDTO struct {
	ID        string    `json:"id"` // nur bulk: Client-UUIDv5; sonst ignoriert
	ProjectID string    `json:"project_id"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
}

type projectIDBody struct {
	ProjectID string `json:"project_id"`
	Tag       string `json:"tag"`
	Note      string `json:"note"`
}

// — Helpers ------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ifMatchVersion parses the bare-integer If-Match header; ok=false when
// the header is absent or not a number (caller answers 422).
func ifMatchVersion(r *http.Request) (int64, bool) {
	v := r.Header.Get("If-Match")
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (d WorktimeAPIDeps) sessionFromWrite(userID string, in sessionWriteDTO) (domain.Session, string) {
	if in.ProjectID == "" {
		return domain.Session{}, "project_id fehlt"
	}
	if in.StartedAt.IsZero() || in.StoppedAt.IsZero() || !in.StoppedAt.After(in.StartedAt) {
		return domain.Session{}, "started_at/stopped_at ungültig (stop muss nach start liegen)"
	}
	loc := time.UTC
	if d.Settings != nil {
		loc = d.Settings.Location(userID)
	}
	return domain.Session{
		ID: in.ID, UserID: userID, ProjectID: in.ProjectID,
		Date:  bookingDayOf(in.StartedAt, loc),
		Start: in.StartedAt.UTC(), Stop: in.StoppedAt.UTC(),
		Tag: in.Tag, Note: in.Note,
	}, ""
}

// bookingDayOf duplicates pgstore.BookingDay's tiny formula to keep the
// adapter dependency direction clean (httpserver kennt pgstore nicht).
func bookingDayOf(startedAt time.Time, loc *time.Location) time.Time {
	l := startedAt.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// — Sessions -----------------------------------------------------------------

func (d WorktimeAPIDeps) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	from, err1 := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	to, err2 := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || to.Before(from) {
		apiError(w, http.StatusUnprocessableEntity, "from/to müssen YYYY-MM-DD sein, to >= from")
		return
	}
	items, err := d.Sessions.ListByUserDateRange(user.ID, from, to)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]sessionDTO, 0, len(items))
	for _, s := range items {
		dtos = append(dtos, toSessionDTO(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d WorktimeAPIDeps) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in sessionWriteDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	sess, problem := d.sessionFromWrite(user.ID, in)
	if problem != "" {
		apiError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	sess.ID = uuid.NewString() // manuelle Session: Server vergibt die ID
	saved, err := d.Sessions.Upsert(sess, 0)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(saved))
}

func (d WorktimeAPIDeps) handleSessionsBulk(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in struct {
		Sessions []sessionWriteDTO `json:"sessions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	batch := make([]domain.Session, 0, len(in.Sessions))
	for i, row := range in.Sessions {
		sess, problem := d.sessionFromWrite(user.ID, row)
		if problem != "" {
			apiError(w, http.StatusUnprocessableEntity,
				"sessions["+strconv.Itoa(i)+"]: "+problem)
			return
		}
		if sess.ID == "" {
			apiError(w, http.StatusUnprocessableEntity,
				"sessions["+strconv.Itoa(i)+"]: id (Client-UUIDv5) fehlt — Import-Idempotenz braucht stabile IDs")
			return
		}
		batch = append(batch, sess)
	}
	if err := d.Sessions.BulkUpsert(batch); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"received": len(batch)})
}

func (d WorktimeAPIDeps) handleSessionPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	var in sessionWriteDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	sess, problem := d.sessionFromWrite(user.ID, in)
	if problem != "" {
		apiError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	sess.ID = id
	saved, err := d.Sessions.Upsert(sess, expected)
	if errors.Is(err, ports.ErrSessionVersionConflict) {
		current, gerr := d.Sessions.GetByID(user.ID, id)
		if errors.Is(gerr, ports.ErrSessionNotFound) {
			apiError(w, http.StatusNotFound, "session existiert nicht")
			return
		}
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toSessionDTO(current)})
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(saved))
}

func (d WorktimeAPIDeps) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	err := d.Sessions.Delete(user.ID, id, expected)
	switch {
	case errors.Is(err, ports.ErrSessionNotFound):
		apiError(w, http.StatusNotFound, "session existiert nicht")
	case errors.Is(err, ports.ErrSessionVersionConflict):
		current, _ := d.Sessions.GetByID(user.ID, id)
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toSessionDTO(current)})
	case err != nil:
		apiError(w, http.StatusInternalServerError, err.Error())
	default:
		d.changed(user.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// — Active -------------------------------------------------------------------

func (d WorktimeAPIDeps) handleActiveList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	items, err := d.Active.ListByUser(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]activeDTO, 0, len(items))
	for _, a := range items {
		dtos = append(dtos, toActiveDTO(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d WorktimeAPIDeps) handleActiveStart(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	device := r.Header.Get("X-Flow-Device")
	a, err := d.Active.Start(user.ID, in.ProjectID, time.Time{}, device, 0, in.Tag, in.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		apiError(w, http.StatusConflict, "für dieses Projekt läuft bereits eine Session")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toActiveDTO(a))
}

func (d WorktimeAPIDeps) handleActiveStop(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	cur, err := d.Active.Get(user.ID, in.ProjectID)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess, err := d.Active.Stop(user.ID, in.ProjectID, cur.Version, in.Tag, in.Note)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		apiError(w, http.StatusConflict, "Zustand hat sich parallel geändert — neu laden")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(sess))
}

func (d WorktimeAPIDeps) handleActivePause(w http.ResponseWriter, r *http.Request) {
	d.pauseResume(w, r, d.Active.Pause)
}

func (d WorktimeAPIDeps) handleActiveResume(w http.ResponseWriter, r *http.Request) {
	d.pauseResume(w, r, d.Active.Resume)
}

func (d WorktimeAPIDeps) pauseResume(w http.ResponseWriter, r *http.Request, op func(userID, projectID string) (domain.ActiveSession, error)) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	a, err := op(user.ID, in.ProjectID)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toActiveDTO(a))
}
```

- [ ] **Step 6: Tests grün**

```bash
go test ./internal/adapter/httpserver/ -run TestWorktimeAPI -v -timeout 300s 2>&1 | tail -8
```

Expected: PASS. Achtung: schlägt der `sessions:bulk`-Test mit 404 fehl, kommt chi mit dem
Doppelpunkt im statischen Segment nicht klar — dann Route + Test + Spec-Kommentar auf
`/worktime/sessions/bulk` umstellen und als Abweichung dokumentieren.

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/adapter/httpserver/ internal/webui/sse/
git add internal/adapter/httpserver/ internal/webui/sse/
git commit -m "$(cat <<'EOF'
feat(api): Worktime-Bearer-API nach Spec §7 (sessions CRUD+bulk, Statemachine) (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 10: Bearer-API — Documents + Repo-Note-Alias

**Files:**
- Create: `internal/adapter/httpserver/documents_api.go`
- Create: `internal/adapter/httpserver/documents_api_test.go`

- [ ] **Step 1: Failing Tests**

```go
// internal/adapter/httpserver/documents_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func newDocsAPIEnv(t *testing.T, sub string) (domain.User, chi.Router) {
	t.Helper()
	u, err := pgstore.NewUsers(pgTestStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountDocumentsAPI(r, DocumentsAPIDeps{Store: pgstore.NewDocuments(pgTestStore)})
	return u, r
}

func docReq(t *testing.T, r chi.Router, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDocumentsAPI_CRUDWithIfMatch(t *testing.T) {
	_, r := newDocsAPIEnv(t, "api-docs-1")

	// Create: If-Match: 0
	body := `{"body":"# Ideen"}`
	rec := docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", body, map[string]string{"If-Match": "0"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}

	// GET liefert body + version
	rec = docReq(t, r, "GET", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	if doc["body"] != "# Ideen" || doc["version"].(float64) != 1 {
		t.Fatalf("get payload: %v", doc)
	}

	// Update mit If-Match
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"v2"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	// Stale → 412
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"stale"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale: want 412, got %d", rec.Code)
	}
	// Ohne If-Match → 422
	rec = docReq(t, r, "PUT", "/documents/projects/flow/ideen.md", `{"body":"x"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("no if-match: want 422, got %d", rec.Code)
	}

	// Liste + Suche
	rec = docReq(t, r, "GET", "/documents?prefix=projects/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	// DELETE idempotent
	rec = docReq(t, r, "DELETE", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = docReq(t, r, "GET", "/documents/projects/flow/ideen.md", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("after delete: want 404, got %d", rec.Code)
	}
}

func TestDocumentsAPI_RepoNoteAlias(t *testing.T) {
	_, r := newDocsAPIEnv(t, "api-docs-2")

	key := "git:github.com/serverkraken/flow"
	escaped := url.PathEscape(key)

	// PUT über den Alias legt das Dokument unter dem Konventions-Pfad an
	rec := docReq(t, r, "PUT", "/repos/"+escaped+"/note", `{"body":"repo wisdom"}`, map[string]string{"If-Match": "0"})
	if rec.Code != http.StatusOK {
		t.Fatalf("alias put: %d %s", rec.Code, rec.Body)
	}
	// GET über den Alias
	rec = docReq(t, r, "GET", "/repos/"+escaped+"/note", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("alias get: %d", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	if doc["repo_key"] != key {
		t.Errorf("repo_key roundtrip: %v", doc["repo_key"])
	}
	// und über den documents-Pfad (Spec: Lookup wahlweise)
	rec = docReq(t, r, "GET", "/documents/repos/"+url.PathEscape(escaped)+".md", "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("path lookup of repo note: %d", rec.Code)
	}
	// fehlender Key → 404
	rec = docReq(t, r, "GET", "/repos/"+url.PathEscape("git:github.com/x/y")+"/note", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing repo note: want 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Test — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/httpserver/ -run TestDocumentsAPI -timeout 300s 2>&1 | tail -3
```

Expected: `undefined: MountDocumentsAPI`.

- [ ] **Step 3: Implementierung**

```go
// internal/adapter/httpserver/documents_api.go
//
// R1 Bearer-API für documents (Spec §7) inkl. /repos/{key}/note-Alias.
// Pfad-Konvention für Repo-Notes (Spec §6):
//   path = "repos/" + url.PathEscape(canonicalKey) + ".md", repo_key = canonicalKey
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// DocumentsAPIDeps bundles the documents API dependencies.
type DocumentsAPIDeps struct {
	Store ports.DocumentStore
	Bus   *sse.Broadcaster
}

func (d DocumentsAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "documents")
	}
}

// MountDocumentsAPI registers the documents + repo-note-alias routes on r.
func MountDocumentsAPI(r chi.Router, d DocumentsAPIDeps) {
	r.Get("/documents", d.handleList)
	r.Get("/documents/*", d.handleGet)
	r.Put("/documents/*", d.handlePut)
	r.Delete("/documents/*", d.handleDelete)
	r.Get("/repos/{key}/note", d.handleRepoNoteGet)
	r.Put("/repos/{key}/note", d.handleRepoNotePut)
}

type documentDTO struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

func toDocumentDTO(doc ports.Document) documentDTO {
	return documentDTO{
		Path: doc.Path, Body: doc.Body, RepoKey: doc.RepoKey,
		Version: doc.Version, UpdatedAt: doc.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// docPath extracts the wildcard document path. Multi-segment, URL-unescaped.
func docPath(r *http.Request) string {
	raw := chi.URLParam(r, "*")
	p, err := url.PathUnescape(raw)
	if err != nil {
		p = raw
	}
	return strings.TrimPrefix(p, "/")
}

// repoNotePath is THE path convention for repo notes (Spec §6).
func repoNotePath(canonicalKey string) string {
	return "repos/" + url.PathEscape(canonicalKey) + ".md"
}

func (d DocumentsAPIDeps) handleList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			apiError(w, http.StatusUnprocessableEntity, "limit muss eine positive Zahl sein")
			return
		}
		limit = n
	}
	entries, err := d.Store.List(user.ID, q.Get("prefix"), q.Get("q"), limit)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type entryDTO struct {
		Path      string `json:"path"`
		RepoKey   string `json:"repo_key"`
		Version   int64  `json:"version"`
		UpdatedAt string `json:"updated_at"`
		Snippet   string `json:"snippet,omitempty"`
	}
	dtos := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryDTO{
			Path: e.Path, RepoKey: e.RepoKey, Version: e.Version,
			UpdatedAt: e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), Snippet: e.Snippet,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d DocumentsAPIDeps) handleGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	doc, err := d.Store.Get(user.ID, docPath(r))
	if errors.Is(err, ports.ErrDocumentNotFound) {
		apiError(w, http.StatusNotFound, "document existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}

func (d DocumentsAPIDeps) handlePut(w http.ResponseWriter, r *http.Request) {
	d.putDocument(w, r, docPath(r), "")
}

func (d DocumentsAPIDeps) handleDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	if err := d.Store.Delete(user.ID, docPath(r)); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (d DocumentsAPIDeps) handleRepoNoteGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		apiError(w, http.StatusUnprocessableEntity, "canonical-key ungültig")
		return
	}
	doc, err := d.Store.GetByRepoKey(user.ID, key)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		apiError(w, http.StatusNotFound, "keine Note für dieses Repo")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}

func (d DocumentsAPIDeps) handleRepoNotePut(w http.ResponseWriter, r *http.Request) {
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		apiError(w, http.StatusUnprocessableEntity, "canonical-key ungültig")
		return
	}
	d.putDocument(w, r, repoNotePath(key), key)
}

// putDocument is the shared If-Match write path for both surfaces.
func (d DocumentsAPIDeps) putDocument(w http.ResponseWriter, r *http.Request, path, repoKey string) {
	user, _ := UserFromContext(r.Context())
	if path == "" {
		apiError(w, http.StatusUnprocessableEntity, "pfad fehlt")
		return
	}
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header fehlt (0 = neu anlegen)")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	doc, err := d.Store.Put(user.ID, path, in.Body, repoKey, expected)
	if errors.Is(err, ports.ErrDocumentVersionConflict) {
		current, gerr := d.Store.Get(user.ID, path)
		if gerr == nil {
			writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toDocumentDTO(current)})
			return
		}
		apiError(w, http.StatusPreconditionFailed, "version conflict")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toDocumentDTO(doc))
}
```

- [ ] **Step 4: Tests grün**

```bash
go test ./internal/adapter/httpserver/ -run TestDocumentsAPI -v -timeout 300s 2>&1 | tail -8
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/adapter/httpserver/
git add internal/adapter/httpserver/
git commit -m "$(cat <<'EOF'
feat(api): Documents-Bearer-API + /repos/{key}/note-Alias (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 11: Bearer-API — Day-Offs + Settings

**Files:**
- Create: `internal/adapter/httpserver/dayoffs_settings_api.go`
- Create: `internal/adapter/httpserver/dayoffs_settings_api_test.go`

- [ ] **Step 1: Failing Tests**

```go
// internal/adapter/httpserver/dayoffs_settings_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func newMiscAPIEnv(t *testing.T, sub string) chi.Router {
	t.Helper()
	u, err := pgstore.NewUsers(pgTestStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountDayOffsSettingsAPI(r, DayOffsSettingsAPIDeps{
		DayOffs:  pgstore.NewDayOffs(pgTestStore),
		Settings: pgstore.NewSettings(pgTestStore),
	})
	return r
}

func miscReq(t *testing.T, r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDayOffsAPI_PutListDelete(t *testing.T) {
	r := newMiscAPIEnv(t, "api-dayoff-1")

	rec := miscReq(t, r, "PUT", "/day-offs/2026-06-15", `{"kind":"vacation","label":"Sommer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// ungültiger Kind → 422
	rec = miscReq(t, r, "PUT", "/day-offs/2026-06-16", `{"kind":"feiertag?"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad kind: want 422, got %d", rec.Code)
	}
	// ungültiges Datum → 422
	rec = miscReq(t, r, "PUT", "/day-offs/morgen", `{"kind":"sick"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad date: want 422, got %d", rec.Code)
	}

	rec = miscReq(t, r, "GET", "/day-offs?year=2026", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0]["kind"] != "vacation" {
		t.Fatalf("list payload: %v", page.Items)
	}

	rec = miscReq(t, r, "DELETE", "/day-offs/2026-06-15", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestSettingsAPI_GetPut(t *testing.T) {
	r := newMiscAPIEnv(t, "api-settings-1")

	rec := miscReq(t, r, "GET", "/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get empty: %d", rec.Code)
	}

	rec = miscReq(t, r, "PUT", "/settings", `{"daily_target":"7h30m","timezone":"Europe/Berlin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// kaputte Zeitzone → 422
	rec = miscReq(t, r, "PUT", "/settings", `{"timezone":"Nicht/Existent"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad tz: want 422, got %d", rec.Code)
	}

	rec = miscReq(t, r, "GET", "/settings", "")
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["daily_target"] != "7h30m" || got["timezone"] != "Europe/Berlin" {
		t.Errorf("roundtrip: %v", got)
	}
}
```

- [ ] **Step 2: Test — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/httpserver/ -run 'TestDayOffsAPI|TestSettingsAPI' -timeout 300s 2>&1 | tail -3
```

Expected: `undefined: MountDayOffsSettingsAPI`.

- [ ] **Step 3: Implementierung**

```go
// internal/adapter/httpserver/dayoffs_settings_api.go
//
// R1 Bearer-API für Day-Offs + User-Settings (Spec §7). Settings sind ein
// flaches key/value-Objekt; nur bekannte Keys werden akzeptiert und
// validiert (Zeitzone muss ladbar, daily_target eine Go-Duration sein).
package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// DayOffsServerStore is the server-side day-off surface (pgstore.DayOffs).
type DayOffsServerStore interface {
	List(userID string, year int) ([]domain.DayOff, error)
	Put(userID string, off domain.DayOff) error
	Delete(userID string, day time.Time) error
}

// SettingsServerStore is the user-settings surface (pgstore.Settings).
type SettingsServerStore interface {
	All(userID string) (map[string]string, error)
	Set(userID, key, value string) error
}

// DayOffsSettingsAPIDeps bundles both small APIs — they share validation
// helpers and always ship together.
type DayOffsSettingsAPIDeps struct {
	DayOffs  DayOffsServerStore
	Settings SettingsServerStore
	Bus      *sse.Broadcaster
}

// MountDayOffsSettingsAPI registers /day-offs and /settings on r.
func MountDayOffsSettingsAPI(r chi.Router, d DayOffsSettingsAPIDeps) {
	r.Get("/day-offs", d.handleDayOffsList)
	r.Put("/day-offs/{date}", d.handleDayOffPut)
	r.Delete("/day-offs/{date}", d.handleDayOffDelete)
	r.Get("/settings", d.handleSettingsGet)
	r.Put("/settings", d.handleSettingsPut)
}

type dayOffDTO struct {
	Day    string `json:"day"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Target string `json:"target,omitempty"` // Go-Duration, z.B. "4h"
}

func validKind(raw string) bool {
	for _, k := range domain.AllKinds {
		if string(k) == raw {
			return true
		}
	}
	return false
}

func (d DayOffsSettingsAPIDeps) handleDayOffsList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil || year < 2000 || year > 2200 {
		apiError(w, http.StatusUnprocessableEntity, "year=YYYY erforderlich")
		return
	}
	items, err := d.DayOffs.List(user.ID, year)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]dayOffDTO, 0, len(items))
	for _, off := range items {
		dto := dayOffDTO{Day: off.Date.Format("2006-01-02"), Kind: string(off.Kind), Label: off.Label}
		if off.Target > 0 {
			dto.Target = off.Target.String()
		}
		dtos = append(dtos, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d DayOffsSettingsAPIDeps) handleDayOffPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	day, err := time.Parse("2006-01-02", chi.URLParam(r, "date"))
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "Datum muss YYYY-MM-DD sein")
		return
	}
	var in dayOffDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	if !validKind(in.Kind) {
		apiError(w, http.StatusUnprocessableEntity, "kind muss holiday|vacation|sick sein")
		return
	}
	var target time.Duration
	if in.Target != "" {
		target, err = time.ParseDuration(in.Target)
		if err != nil || target < 0 {
			apiError(w, http.StatusUnprocessableEntity, "target muss eine Go-Duration sein (z.B. 4h)")
			return
		}
	}
	off := domain.DayOff{Date: day, Kind: domain.Kind(in.Kind), Label: in.Label, Target: target}
	if err := d.DayOffs.Put(user.ID, off); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Bus != nil {
		d.Bus.Changed(user.ID, "dayoffs")
	}
	writeJSON(w, http.StatusOK, in)
}

func (d DayOffsSettingsAPIDeps) handleDayOffDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	day, err := time.Parse("2006-01-02", chi.URLParam(r, "date"))
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "Datum muss YYYY-MM-DD sein")
		return
	}
	if err := d.DayOffs.Delete(user.ID, day); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Bus != nil {
		d.Bus.Changed(user.ID, "dayoffs")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// settingsValidators gates the accepted keys. Unbekannte Keys → 422, damit
// sich keine Tippfehler-Settings ansammeln.
var settingsValidators = map[string]func(string) bool{
	"daily_target": func(v string) bool { d, err := time.ParseDuration(v); return err == nil && d >= 0 },
	"timezone":     func(v string) bool { _, err := time.LoadLocation(v); return err == nil && v != "" },
}

func (d DayOffsSettingsAPIDeps) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	all, err := d.Settings.All(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}

func (d DayOffsSettingsAPIDeps) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in map[string]string
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	for k, v := range in {
		validate, known := settingsValidators[k]
		if !known {
			apiError(w, http.StatusUnprocessableEntity, "unbekannter Settings-Key: "+k)
			return
		}
		if !validate(v) {
			apiError(w, http.StatusUnprocessableEntity, "ungültiger Wert für "+k)
			return
		}
	}
	for k, v := range in {
		if err := d.Settings.Set(user.ID, k, v); err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	all, err := d.Settings.All(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}
```

- [ ] **Step 4: Tests grün**

```bash
go test ./internal/adapter/httpserver/ -run 'TestDayOffsAPI|TestSettingsAPI' -v -timeout 300s 2>&1 | tail -8
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/adapter/httpserver/
git add internal/adapter/httpserver/
git commit -m "$(cat <<'EOF'
feat(api): Day-Offs- + Settings-Bearer-API (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 12: `/api/v1/meta` + Versions-Plumbing

**Files:**
- Create: `internal/adapter/httpserver/meta.go`
- Create: `internal/adapter/httpserver/meta_test.go`
- Modify: `Makefile` (build-server mit ldflags)
- Modify: `deploy/podman/Dockerfile.server` (ARG VERSION)
- Modify: `.github/workflows/build-server-image.yml` (build-arg)

`/meta` ist PUBLIC (kein Auth): der Versions-Handshake muss vor dem Login funktionieren
und enthält keine Geheimnisse. `min_client_version` bleibt in R1 `"0.0.0"` — R2 hebt sie an,
sobald der neue httpapi-Client existiert.

- [ ] **Step 1: Failing Test**

```go
// internal/adapter/httpserver/meta_test.go
package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetaHandler(t *testing.T) {
	t.Parallel()
	h := NewMetaHandler(MetaResponse{ServerVersion: "1.2.3-test", MinClientVersion: "0.0.0"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var got MetaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.ServerVersion != "1.2.3-test" || got.MinClientVersion != "0.0.0" {
		t.Errorf("payload: %+v", got)
	}
}
```

- [ ] **Step 2: Test — Compile-FAIL erwartet**

```bash
go test ./internal/adapter/httpserver/ -run TestMetaHandler -timeout 60s 2>&1 | tail -3
```

Expected: `undefined: NewMetaHandler`.

- [ ] **Step 3: Handler**

```go
// internal/adapter/httpserver/meta.go
package httpserver

import "net/http"

// MetaResponse is the §7 version handshake. Public — no secrets, must be
// reachable before login so clients can warn about version skew.
type MetaResponse struct {
	ServerVersion    string `json:"server_version"`
	MinClientVersion string `json:"min_client_version"`
}

// NewMetaHandler returns the GET /api/v1/meta handler.
func NewMetaHandler(meta MetaResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meta)
	})
}
```

- [ ] **Step 4: Test grün**

```bash
go test ./internal/adapter/httpserver/ -run TestMetaHandler -v -timeout 60s 2>&1 | tail -4
```

Expected: PASS.

- [ ] **Step 5: Makefile — Version in build-server**

Im `Makefile` oben bei den Variablen ergänzen:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
```

und das `build-server`-Target so ändern, dass es die Version einbrennt:

```makefile
build-server:
	@mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION)" -o bin/flow-server ./cmd/flow-server
```

(Die Variable `main.version` entsteht in Task 18 in `cmd/flow-server/main.go`; bis dahin
ist das ldflags-Flag wirkungslos, aber harmlos — `go build` warnt nicht über fehlende
Symbole... doch: neuere Go-Versionen loggen "unknown symbol". Falls `make build-server`
deswegen Output zeigt: ignorieren, Task 18 löst es auf.)

- [ ] **Step 6: Dockerfile + GHA**

In `deploy/podman/Dockerfile.server` direkt nach `FROM golang:1.25-alpine AS build`:

```dockerfile
ARG VERSION=dev
```

und die go-build-Zeile erweitern:

```dockerfile
RUN CGO_ENABLED=0 GOFLAGS="-trimpath" go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/flow-server ./cmd/flow-server
```

In `.github/workflows/build-server-image.yml` beim docker/build-push-Step ergänzen
(gleiche Einrückung wie vorhandene Keys des Steps):

```yaml
          build-args: |
            VERSION=${{ github.sha }}
```

- [ ] **Step 7: Build-Check + Commit**

```bash
go build ./... && make build-server >/dev/null && echo OK
gofumpt -w internal/adapter/httpserver/meta.go internal/adapter/httpserver/meta_test.go
git add internal/adapter/httpserver/meta.go internal/adapter/httpserver/meta_test.go Makefile deploy/podman/Dockerfile.server .github/workflows/build-server-image.yml
git commit -m "$(cat <<'EOF'
feat(api): /api/v1/meta Versions-Handshake + Version-ldflags-Plumbing (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 13: SSE generalisieren — Heartbeat, changed-Konsum, Bearer-oder-Cookie

**Files:**
- Modify: `internal/webui/handlers/events.go` (25-s-Heartbeat)
- Create: `internal/adapter/httpserver/middleware_dual.go`
- Create: `internal/adapter/httpserver/middleware_dual_test.go`
- Modify: `internal/webui/templates/worktime/today.templ` (changed-Konsum)
- Run: `make webui-templ`

- [ ] **Step 1: Heartbeat in den Events-Handler**

In `internal/webui/handlers/events.go`: über `func NewEvents` eine Paket-Variable einführen
(Test-Seam) und die Event-Loop um den Ticker-Fall erweitern.

Vor `func NewEvents`:

```go
// heartbeatInterval keeps idle SSE connections alive through ingress
// proxies (Spec §7: Heartbeat-Kommentar alle 25 s; nginx-Idle-Timeouts
// liegen typisch bei 60 s). Paket-Variable als Test-Seam.
var heartbeatInterval = 25 * time.Second
```

In der Handler-Funktion nach `flusher.Flush()` (dem ": connected"-Flush) einfügen:

```go
		hb := time.NewTicker(heartbeatInterval)
		defer hb.Stop()
```

und die `select`-Schleife um diesen Fall erweitern (vor `case ev, open := <-ch:`):

```go
			case <-hb.C:
				if _, err := fmt.Fprint(w, ": hb\n\n"); err != nil {
					return
				}
				flusher.Flush()
```

Import `"time"` ergänzen.

- [ ] **Step 2: Heartbeat-Test**

Ans Ende von `internal/webui/handlers/events_test.go` (Paket `handlers_test` — der Seam
muss deshalb über eine kleine exportierte Test-Hilfe gesetzt werden; einfacher Weg: den
Test in eine NEUE Datei `internal/webui/handlers/events_heartbeat_test.go` mit
`package handlers` legen):

```go
// internal/webui/handlers/events_heartbeat_test.go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/sse"
)

func TestEvents_HeartbeatComment(t *testing.T) {
	old := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	defer func() { heartbeatInterval = old }()

	b := sse.New()
	h := NewEvents(b)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	ctx, cancel := contextWithTimeout(req, 120*time.Millisecond)
	defer cancel()
	req = req.WithContext(httpserver.WithUser(ctx, domain.User{ID: "hb-user"}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), ": hb") {
		t.Errorf("heartbeat comment missing in stream: %q", rec.Body.String())
	}
}
```

Dazu oben in derselben Datei den kleinen Helper:

```go
func contextWithTimeout(r *http.Request, d time.Duration) (ctx interface {
	Done() <-chan struct{}
	Err() error
	Value(any) any
	Deadline() (time.Time, bool)
}, cancel func(),
) {
	c, cancelFn := context.WithTimeout(r.Context(), d)
	return c, cancelFn
}
```

— SIMPLER und vorzuziehen: statt des Interface-Gymnastik-Helpers direkt
`ctx, cancel := context.WithTimeout(req.Context(), 120*time.Millisecond)` schreiben und
`"context"` importieren. Den obigen Helper dann weglassen.

```bash
go test ./internal/webui/handlers/ -run TestEvents_Heartbeat -v -timeout 60s 2>&1 | tail -4
```

Expected: PASS.

- [ ] **Step 3: Bearer-oder-Cookie-Middleware**

```go
// internal/adapter/httpserver/middleware_dual.go
package httpserver

import "net/http"

// NewBearerOrCookieMiddleware lets ONE route serve both client classes
// (Spec §5: SSE für Browser UND TUI/MCP): requests carrying an
// Authorization header take the bearer path, everything else the
// browser-cookie path. Both wrapped middlewares already put the resolved
// domain.User into the context, which is all /api/v1/events needs.
func NewBearerOrCookieMiddleware(bearer, cookie func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		bearerChain := bearer(next)
		cookieChain := cookie(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				bearerChain.ServeHTTP(w, r)
				return
			}
			cookieChain.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Middleware-Test**

```go
// internal/adapter/httpserver/middleware_dual_test.go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerOrCookie_RoutesByAuthorizationHeader(t *testing.T) {
	t.Parallel()
	mark := func(label string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Path", label)
				next.ServeHTTP(w, r)
			})
		}
	}
	mw := NewBearerOrCookieMiddleware(mark("bearer"), mark("cookie"))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer x")
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Path") != "bearer" {
		t.Errorf("with Authorization: want bearer path, got %q", rec.Header().Get("X-Path"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Header().Get("X-Path") != "cookie" {
		t.Errorf("without Authorization: want cookie path, got %q", rec.Header().Get("X-Path"))
	}
}
```

```bash
go test ./internal/adapter/httpserver/ -run TestBearerOrCookie -v -timeout 60s 2>&1 | tail -4
```

Expected: PASS.

- [ ] **Step 5: WebUI konsumiert `changed`**

In `internal/webui/templates/worktime/today.templ`:

1. Im `sse-swap`-Attribut `changed` ergänzen:

```
<div sse-swap="tick,changed,session.started,session.stopped,session.updated,session.deleted" hx-swap="none" hidden></div>
```

2. Im Script-Block (`sseTodayScript`) den Reload-Trigger erweitern — die bestehende
Bedingung

```js
			if (t === 'session.started' || t === 'session.stopped' ||
				t === 'session.updated' || t === 'session.deleted') {
```

ersetzen durch

```js
			if (t === 'changed' || t === 'session.started' || t === 'session.stopped' ||
				t === 'session.updated' || t === 'session.deleted') {
```

Dann:

```bash
make webui-templ && go build ./... && go test ./internal/webui/... -timeout 300s 2>&1 | tail -4
make ci 2>&1 | tail -3
```

Expected: Build + Tests + ci grün.

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/adapter/httpserver/ internal/webui/handlers/
git add internal/adapter/httpserver/ internal/webui/
git commit -m "$(cat <<'EOF'
feat(sse): 25s-Heartbeat, changed-Konsum im WebUI, Bearer-oder-Cookie-MW (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 14: WebUI-Handler-Deps auf Interfaces heben (Layer-Fix vor dem Swap)

**Files:**
- Create: `internal/webui/handlers/stores.go`
- Modify: `internal/webui/handlers/dashboard.go`, `worktime.go`, `projects.go`,
  `session_actions.go`, `project_actions.go`

Heute importieren acht WebUI-Handler-Dateien `sqliteserver` direkt in ihren Deps-Structs —
eine bestehende Layer-Verletzung, die den pgstore-Swap blockieren würde. Dieser Task führt
schmale Interfaces ein, die `sqliteserver` (heute) und `pgstore` (ab Task 18) strukturell
erfüllen. `repos.go` und `note_actions.go` bleiben hier unangetastet (sie werden in
Task 15/16 ersetzt bzw. in Task 19 gelöscht).

- [ ] **Step 1: Interfaces definieren**

```go
// internal/webui/handlers/stores.go
//
// Narrow store interfaces for the WebUI handler Deps. Both server store
// adapters (sqliteserver until R1's swap, pgstore after) satisfy them
// structurally — the handlers must not know which one is wired.
package handlers

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// SessionsStore is the session surface the WebUI write/read handlers use.
type SessionsStore interface {
	ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error)
	GetByID(userID, id string) (domain.Session, error)
	Upsert(in domain.Session, expectedVersion int64) (domain.Session, error)
	Delete(userID, id string, expectedVersion int64) error
}

// ActiveStore is the active-session lifecycle surface.
type ActiveStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error)
	Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
}

// PauseResumeStore is the R1 pause statemachine surface. Separate from
// ActiveStore because sqliteserver never implements it — the field is
// only wired once pgstore is in (Task 18); handlers nil-guard it.
type PauseResumeStore interface {
	Pause(userID, projectID string) (domain.ActiveSession, error)
	Resume(userID, projectID string) (domain.ActiveSession, error)
}

// ProjectsStore is the project surface.
type ProjectsStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
	GetBySlug(userID, slug string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)
	TouchLastUsed(userID, id string) error
	Archive(userID, id string) error
}
```

- [ ] **Step 2: Deps-Structs mechanisch umstellen**

In jeder der fünf Dateien die `*sqliteserver.X`-Typen ersetzen — Feldnamen bleiben:

| Datei | Feld | neu |
|---|---|---|
| `dashboard.go` (`DashboardDeps`) | `Active` / `Sessions` / `Projects` | `ActiveStore` / `SessionsStore` / `ProjectsStore` |
| `worktime.go` (`WorktimeDeps`) | dito | dito |
| `projects.go` (`ProjectsDeps`) | `Projects` / `Sessions` / `Active` | `ProjectsStore` / `SessionsStore` / `ActiveStore` |
| `session_actions.go` (`SessionActionsDeps`) | `Sessions` / `Active` / `Projects` | `SessionsStore` / `ActiveStore` / `ProjectsStore` |
| `project_actions.go` (`ProjectActionsDeps`) | `Projects` | `ProjectsStore` |

Zusätzlich in `session_actions.go` den Helper umtypen:

```go
func projectNameFor(projects ProjectsStore, userID, projectID string) string {
```

Danach in allen fünf Dateien den nun unbenutzten Import
`"github.com/serverkraken/flow/internal/adapter/sqliteserver"` entfernen.

- [ ] **Step 3: Build + bestehende Tests**

```bash
go build ./... && go test ./internal/webui/... ./cmd/flow-server/... -timeout 300s 2>&1 | tail -4
```

Expected: grün — die Tests übergeben weiterhin konkrete sqliteserver-Adapter, die die
Interfaces strukturell erfüllen. Compile-Fehler hier heißen fast immer: eine
Interface-Signatur weicht vom konkreten Adapter ab — Signatur im Interface an
`sqliteserver` anpassen (NICHT den Adapter ändern) und als Abweichung notieren.

- [ ] **Step 4: Commit**

```bash
gofumpt -w internal/webui/handlers/
git add internal/webui/handlers/
git commit -m "$(cat <<'EOF'
refactor(webui): Handler-Deps auf Store-Interfaces — Layer-Fix vor pgstore-Swap (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 15: WebUI Notes → Documents (Handler-Trio + Edit mit If-Match)

**Files:**
- Create: `internal/webui/handlers/documents.go`
- Create: `internal/webui/handlers/documents_vm.go`
- Create: `internal/webui/handlers/document_actions.go`
- Modify: `internal/webui/templates/notes/edit.templ` (Version-Feld)
- Run: `make webui-templ`

Die bestehenden notes-Templates (`index.templ`, `view.templ`, `edit.templ`) werden
WEITERVERWENDET — nur die Datenquelle wechselt von fsstore/kompendium auf
`ports.DocumentStore`. Die Typ-Sub-Tabs (`?type=`) verlieren in R1 ihre Filterwirkung
(Dokumente haben keine kompendium-NoteTypes); der Strip bleibt sichtbar, ActiveTab ist
immer `TabAlle` — Feinschliff ist R5. Gemountet wird das Trio in Task 18; die alten
notes-Handler bleiben bis Task 19 bestehen, damit deren Tests weiterlaufen.

- [ ] **Step 1: edit.templ um Version erweitern**

In `internal/webui/templates/notes/edit.templ` im `EditVM`-Struct nach `Content` ergänzen:

```go
	// Version is the If-Match expected version for the documents-backed
	// save (R1). 0 = create. Rendered as hidden input "version".
	Version int64
```

Im `templ Edit(vm EditVM)`-Body innerhalb des `<form …>`-Elements (direkt neben dem
versteckten content-Textarea) ein Hidden-Field ergänzen:

```html
<input type="hidden" name="version" value={ fmt.Sprintf("%d", vm.Version) }/>
```

(Falls `fmt` im Template noch nicht importiert ist: `import "fmt"` im templ-Header ergänzen.
Falls das Template bereits einen Konvertierungs-Helper wie `strconv.FormatInt` nutzt, dem
lokalen Stil folgen.)

```bash
make webui-templ && go build ./internal/webui/... 
```

Expected: Exit 0.

- [ ] **Step 2: Documents-Read-Handler**

```go
// internal/webui/handlers/documents.go
//
// R1: /notes wird von der documents-Tabelle bedient (Spec §10) — gleiche
// Templates, neue Wahrheit. Drei Handler: Index (Liste + Server-FTS via
// ?q=), View (gerendertes Markdown), Edit (CodeMirror-Form mit Version).
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/markdown"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// DocumentsDeps bundles the documents-backed /notes surface dependencies.
type DocumentsDeps struct {
	Store    flowports.DocumentStore
	Markdown *markdown.Renderer
	Clock    flowports.Clock
}

// docWildcardPath extracts the multi-segment document path from /notes/*.
func docWildcardPath(r *http.Request, suffixToStrip string) string {
	raw := chi.URLParam(r, "*")
	if raw == "" {
		raw = strings.TrimPrefix(r.URL.Path, "/notes/")
	}
	p, err := url.PathUnescape(raw)
	if err != nil {
		p = raw
	}
	return strings.TrimSuffix(strings.TrimPrefix(p, "/"), suffixToStrip)
}

// NewDocumentsIndex handles GET /notes (+ ?q= Server-FTS).
func NewDocumentsIndex(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		entries, err := d.Store.List(u.ID, "", query, 200)
		if err != nil {
			slog.Error("documents index: list failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm := buildDocumentsIndexVM(entries, query, d.Clock)
		meta := layout.PageMeta{
			Title:       "Notes",
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.Index(vm)).Render(r.Context(), w); err != nil {
			slog.Error("documents index: render failed", slog.String("err", err.Error()))
		}
	})
}

// NewDocumentView handles GET /notes/* (single document).
func NewDocumentView(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		docPath := docWildcardPath(r, "")
		if docPath == "" {
			http.NotFound(w, r)
			return
		}
		doc, err := d.Store.Get(u.ID, docPath)
		if errors.Is(err, flowports.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("document view: get failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm, err := buildDocumentViewVM(d, doc)
		if err != nil {
			slog.Error("document view: build failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		meta := layout.PageMeta{
			Title:       "Notes · " + vm.Title,
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.View(vm)).Render(r.Context(), w); err != nil {
			slog.Error("document view: render failed", slog.String("err", err.Error()))
		}
	})
}

// NewDocumentEdit handles GET /notes/*/edit — the CodeMirror form,
// pre-filled with the current body + version (If-Match seed).
func NewDocumentEdit(d DocumentsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		docPath := docWildcardPath(r, "/edit")
		if docPath == "" {
			http.NotFound(w, r)
			return
		}
		doc, err := d.Store.Get(u.ID, docPath)
		if errors.Is(err, flowports.ErrDocumentNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("document edit: get failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		vm := notestmpl.EditVM{
			ID:      doc.Path,
			Title:   docTitle(doc.Path, doc.Body),
			Content: doc.Body,
			Version: doc.Version,
		}
		meta := layout.PageMeta{
			Title:       "Notes · " + vm.Title + " bearbeiten",
			CurrentPath: "/notes",
			UserLabel:   userLabel(u),
			Spine:       layout.SpineState{},
		}
		if err := layout.Base(meta, notestmpl.Edit(vm)).Render(r.Context(), w); err != nil {
			slog.Error("document edit: render failed", slog.String("err", err.Error()))
		}
	})
}

// docTitle derives a display title: first H1 wins, else the file stem.
func docTitle(docPath, body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return strings.TrimSuffix(path.Base(docPath), ".md")
}
```

- [ ] **Step 3: VM-Builder**

```go
// internal/webui/handlers/documents_vm.go
package handlers

import (
	"fmt"
	"path"
	"strings"
	"time"

	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/format"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// buildDocumentsIndexVM maps DocumentEntry rows onto the existing notes
// IndexVM. Typ-Sub-Tabs filtern in R1 nicht (Dokumente haben keine
// kompendium-Typen) — ActiveTab bleibt TabAlle, der Strip bleibt sichtbar.
func buildDocumentsIndexVM(entries []flowports.DocumentEntry, query string, clock flowports.Clock) notestmpl.IndexVM {
	now := time.Now()
	if clock != nil {
		now = clock.Now()
	}
	rows := make([]notestmpl.IndexRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, notestmpl.IndexRow{
			ID:      e.Path,
			Title:   strings.TrimSuffix(path.Base(e.Path), ".md"),
			Type:    docTypeLabel(e.Path),
			When:    format.HumanRelativeTime(e.UpdatedAt, now),
			Preview: e.Snippet, // FTS-Headline bei Suche, sonst leer
			Href:    "/notes/" + e.Path,
		})
	}
	vm := notestmpl.IndexVM{
		ActiveTab:  notestmpl.TabAlle,
		Query:      query,
		Configured: true, // documents-API ist immer da — kein NOTEBOOK_ROOT-Placeholder mehr
		Rows:       rows,
		TotalLabel: documentsTotalLabel(len(rows)),
	}
	if len(rows) == 0 {
		if query != "" {
			vm.EmptyReason = "search"
		} else {
			vm.EmptyReason = "empty"
		}
	}
	return vm
}

func documentsTotalLabel(n int) string {
	if n == 1 {
		return "1 Note"
	}
	return fmt.Sprintf("%d Notes", n)
}

// docTypeLabel derives the badge from the path root — the directory
// layout survives the import 1:1 (daily/…, projects/…, repos/…).
func docTypeLabel(docPath string) string {
	switch {
	case strings.HasPrefix(docPath, "daily/"):
		return "Daily"
	case strings.HasPrefix(docPath, "projects/"):
		return "Project"
	case strings.HasPrefix(docPath, "repos/"):
		return "Repo"
	default:
		return "Frei"
	}
}

// buildDocumentViewVM renders the markdown body into the notes ViewVM.
func buildDocumentViewVM(d DocumentsDeps, doc flowports.Document) (notestmpl.ViewVM, error) {
	html, err := d.Markdown.Render([]byte(doc.Body))
	if err != nil {
		html = "" // degrade wie bei den alten notes: Rail bleibt nutzbar
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}
	vm := notestmpl.ViewVM{
		ID:            doc.Path,
		Title:         docTitle(doc.Path, doc.Body),
		Path:          doc.Path,
		TypeLabel:     docTypeLabel(doc.Path),
		HTML:          html,
		CreatedLabel:  "—", // documents tragen kein created-Datum; ehrlich statt geraten
		ModifiedLabel: format.HumanRelativeTime(doc.UpdatedAt, now),
		SyncLabel:     fmt.Sprintf("server · v%d", doc.Version),
		BreadcrumbHrefs: notestmpl.Breadcrumb{
			NotesHref: "/notes",
			TypeHref:  "/notes",
		},
	}
	for _, h := range d.Markdown.Headings([]byte(doc.Body)) {
		vm.Headings = append(vm.Headings, notestmpl.HeadingItem{
			Level: h.Level, Text: h.Text, Anchor: h.Anchor,
		})
	}
	return vm, nil
}
```

- [ ] **Step 4: Write-Handler (PUT mit If-Match aus dem Formular)**

```go
// internal/webui/handlers/document_actions.go
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
	"github.com/serverkraken/flow/internal/webui/templates/layout"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// DocumentActionsDeps bundles the write path for documents-backed notes.
type DocumentActionsDeps struct {
	Store flowports.DocumentStore
	Bus   *sse.Broadcaster
}

// NewDocumentPut handles PUT /notes/* — CodeMirror save with If-Match.
// Conflict (412-Semantik) re-renders the edit form with the FRESH server
// body + version and a hint in the title (Spec §8-Analogie: neu laden +
// Hinweis; eine Diff-UI ist bewusst nicht R1).
func NewDocumentPut(d DocumentActionsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		docPath := docWildcardPath(r, "/edit")
		if docPath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		content := r.PostForm.Get("content")
		version, _ := strconv.ParseInt(strings.TrimSpace(r.PostForm.Get("version")), 10, 64)

		_, err := d.Store.Put(u.ID, docPath, content, "", version)
		if errors.Is(err, flowports.ErrDocumentVersionConflict) {
			current, gerr := d.Store.Get(u.ID, docPath)
			if gerr != nil {
				slog.Error("document put: conflict re-read failed", slog.String("err", gerr.Error()))
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusConflict)
			vm := notestmpl.EditVM{
				ID:      current.Path,
				Title:   docTitle(current.Path, current.Body) + " — Konflikt: Server-Stand neu geladen",
				Content: current.Body,
				Version: current.Version,
			}
			meta := layout.PageMeta{
				Title:       "Notes · Konflikt",
				CurrentPath: "/notes",
				UserLabel:   userLabel(u),
				Spine:       layout.SpineState{},
			}
			_ = layout.Base(meta, notestmpl.Edit(vm)).Render(r.Context(), w)
			return
		}
		if err != nil {
			slog.Error("document put: failed", slog.String("path", docPath), slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if d.Bus != nil {
			d.Bus.Publish(u.ID, sse.Event{Type: "note.updated", Data: map[string]any{"path": docPath}})
			d.Bus.Changed(u.ID, "documents")
		}
		http.Redirect(w, r, "/notes/"+docPath, http.StatusSeeOther)
	})
}
```

- [ ] **Step 5: Build (Mounting kommt in Task 18)**

```bash
make webui-templ && go build ./... && go test ./internal/webui/... -timeout 300s 2>&1 | tail -4
```

Expected: grün. Häufigste Stolperstelle: `notestmpl.Breadcrumb`-Feldnamen oder
`markdown.Renderer.Headings`-Signatur — bei Compile-Fehlern die tatsächliche Definition
nachschlagen (`rg "type Breadcrumb" internal/webui/templates/notes/`,
`rg "func .*Headings" internal/webui/markdown/`), minimal anpassen, Abweichung notieren.

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/webui/handlers/
git add internal/webui/
git commit -m "$(cat <<'EOF'
feat(webui): documents-backed Notes-Handler (Index/View/Edit/Put mit If-Match) (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 16: WebUI Repos → Documents-Alias

**Files:**
- Modify: `internal/webui/handlers/repos.go`
- Modify: `internal/webui/handlers/note_actions.go` (nur die RepoNote-Handler)

Die `/repos`-Seiten zeigen künftig die documents mit `repo_key` (Pfad-Konvention
`repos/<urlescape(key)>.md`). Die Repo-METADATEN (DisplayName, RemoteURL) entfallen mit der
repos-Tabelle — DisplayName wird aus dem Key abgeleitet, Repos OHNE Note erscheinen nicht
mehr in der Liste (documents sind die einzige Wahrheit). Das ist der Spec-§12-Trade-off.

Dieser Task ist ein GEFÜHRTER UMBAU der zwei Dateien — die Templates
(`templates/repos/*.templ`) bleiben unverändert, nur die Datenbeschaffung wechselt:

- [ ] **Step 1: ReposDeps + Index-Handler in `repos.go` umbauen**

1. `ReposDeps` ersetzen durch:

```go
// ReposDeps bundles the documents-backed /repos surface (R1: repo notes
// ARE documents with repo_key set — the repos table is gone).
type ReposDeps struct {
	Documents flowports.DocumentStore
	Markdown  *markdown.Renderer
	Clock     flowports.Clock
}
```

2. Den `sqliteserver`-Import entfernen; `flowports`-Import
   (`flowports "github.com/serverkraken/flow/internal/ports"`) sicherstellen.
3. Im Index-Handler die bisherige Datenbeschaffung (`Repos.PullSince`/`ListByUser`-artige
   Aufrufe + `RepoNotes.GetByRepo` pro Zeile) ersetzen durch EINEN Aufruf:

```go
		entries, err := d.Documents.List(u.ID, "repos/", "", 200)
```

4. Die bestehende Row-Bau-Schleife auf die Entries umstellen — pro `e := range entries`:

```go
		key := repoKeyOfEntry(e) // Helper unten
		rows = append(rows, repostmpl.IndexRow{
			DisplayName: repoDisplayName(key),
			Subtitle:    key + " · note ✓",
			HasNote:     true,
			MetaLeft:    fmt.Sprintf("version %d", e.Version),
			// … übrige Felder (Href/MetaRight/…): bestehende Zuweisungen
			// beibehalten und aus key bzw. e ableiten — Feldliste NICHT
			// verändern, das Template bleibt wie es ist.
		})
```

5. Diese zwei Helper ans Datei-Ende:

```go
// repoKeyOfEntry prefers the stored repo_key and falls back to decoding
// the path convention repos/<urlescape(key)>.md.
func repoKeyOfEntry(e flowports.DocumentEntry) string {
	if e.RepoKey != "" {
		return e.RepoKey
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(e.Path, "repos/"), ".md")
	if key, err := url.PathUnescape(raw); err == nil {
		return key
	}
	return raw
}

// repoDisplayName shortens "git:github.com/foo/bar" to "foo/bar".
func repoDisplayName(key string) string {
	k := strings.TrimPrefix(key, "git:")
	if i := strings.Index(k, "/"); i > 0 && strings.Contains(k[:i], ".") {
		k = k[i+1:] // Host-Anteil (enthält einen Punkt) abwerfen
	}
	return k
}
```

6. Den Note-View-Handler (`RepoNote`) auf `d.Documents.GetByRepoKey(u.ID, key)` umstellen;
   Body-Rendering über den vorhandenen Markdown-Renderer-Aufruf beibehalten. Felder ohne
   Datenquelle ehrlich füllen: `RemoteURL` → `"(kein remote — R1: notes only)"`,
   `ShortHash` → erste 7 Zeichen der Document-ID, `ModifiedLabel` →
   `format.HumanRelativeTime(doc.UpdatedAt, now)`. Existiert keine Note für den Key
   (`ErrDocumentNotFound`): den bestehenden Empty-State-Zweig der Seite rendern (heute der
   "keine Note"-Branch — Logik beibehalten, nur Trigger ist jetzt der NotFound-Fehler).

- [ ] **Step 2: RepoNote-Edit/Put in `note_actions.go` umbauen**

`NewRepoNoteEdit` (Zeile ~235) und `NewRepoNotePut` (Zeile ~325) auf den DocumentStore
umstellen — `NoteActionsDeps` bekommt dafür das Feld `Documents flowports.DocumentStore`,
die Felder `Repos`/`RepoNotes` werden aus dem Struct entfernt (der `NoteStore`-Teil für die
alten fsstore-Handler `NewNoteEdit`/`NewNotePut` bleibt bis Task 19 unangetastet):

- **Edit:** `doc, err := d.Documents.GetByRepoKey(u.ID, key)`. NotFound → leeres
  `NoteEditVM{CanonicalKey: key, Version: 0}` (erster Save legt an); sonst
  `Content: doc.Body, Version: doc.Version`. `DisplayName: repoDisplayName(key)`.
- **Put:** Version aus dem Formular (bestehende Logik) → 
  `d.Documents.Put(u.ID, repoNotePathWeb(key), content, key, version)` mit

```go
// repoNotePathWeb mirrors the API-side path convention (Spec §6).
func repoNotePathWeb(canonicalKey string) string {
	return "repos/" + url.PathEscape(canonicalKey) + ".md"
}
```

  `ErrDocumentVersionConflict` → der bestehende Konflikt-Zweig (`NoteConflictVM`), gespeist
  aus `d.Documents.GetByRepoKey` statt `RepoNotes.GetByRepo`. Publish-Aufrufe beibehalten
  und um `d.Bus.Changed(u.ID, "documents")` ergänzen.

- [ ] **Step 3: Build + Tests**

```bash
go build ./... 2>&1 | head -20
```

Die Tests `repos_test.go` + die RepoNote-Teile von `note_actions_test.go` kompilieren jetzt
NICHT mehr (sie konstruieren sqliteserver-Repos). Sie werden in Task 19 auf pgstore
umgezogen — für diesen Task die betroffenen Test-FUNKTIONEN (nicht die Dateien) mit
`t.Skip("R1: wird in Task 19 auf pgstore/documents umgezogen")` als erste Zeile stilllegen
bzw., wo der Compile-Fehler im Test-Setup selbst liegt, die Setup-Helper minimal auf
`pgstore` umstellen, falls das in <20 Zeilen geht — sonst skippen und Abweichung notieren.

```bash
go build ./... && go test ./internal/webui/... -timeout 300s 2>&1 | tail -4
```

Expected: Build grün; Tests grün (mit Skips).

- [ ] **Step 4: Commit**

```bash
gofumpt -w internal/webui/handlers/
git add internal/webui/
git commit -m "$(cat <<'EOF'
feat(webui): /repos liest und schreibt documents via repo_key-Alias (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 17: Pause/Resume im WebUI + Statusleiste ehrlich (SyncState stirbt)

**Files:**
- Modify: `internal/webui/templates/shared/live_banner.templ`
- Modify: `internal/webui/templates/worktime/today.templ` (Tick-JS pausenfest)
- Modify: `internal/webui/handlers/session_actions.go` (Pause/Resume-Handler + Banner-VM)
- Modify: `internal/webui/handlers/worktime_vm.go` (Banner mit Pause)
- Modify: `internal/webui/templates/layout/spine.templ` (SyncState-Feld + Dot raus)
- Modify: alle Handler mit `SyncState: "ok"`-Literalen
- Create: `internal/webui/handlers/session_actions_pause_test.go`
- Run: `make webui-templ`

- [ ] **Step 1: LiveBanner um Pause erweitern**

In `internal/webui/templates/shared/live_banner.templ` dem `LiveBanner`-Struct nach
`StopHref` hinzufügen:

```go
	// PauseHref/ResumeHref are the POST targets for the R1 pause
	// statemachine buttons. Empty → button not rendered (pre-R1 callers).
	PauseHref  string
	ResumeHref string
	// IsPaused switches the banner into its frozen state: Resume statt
	// Pause-Button, Tick-JS hält den Zähler an (data-paused).
	IsPaused bool
```

Im `templ LiveBannerCard(b LiveBanner)`:

1. Den `data-started`-Zweig der elapsed-Anzeige um das Paused-Attribut erweitern —
   aus

```
<div class="live-elapsed live-pulse" data-started={ liveBannerStartedAttr(b.StartedUnix) }>{ b.ElapsedLabel }</div>
```

   wird

```
if b.IsPaused {
	<div class="live-elapsed" data-started={ liveBannerStartedAttr(b.StartedUnix) } data-paused="1">{ b.ElapsedLabel }</div>
} else {
	<div class="live-elapsed live-pulse" data-started={ liveBannerStartedAttr(b.StartedUnix) }>{ b.ElapsedLabel }</div>
}
```

2. Vor dem Stop-Button-Block die beiden neuen Buttons einfügen (gleiche
   Button-Konventionen wie der Stop-Button, SVG statt Emoji):

```
if b.PauseHref != "" && !b.IsPaused {
	<button
		class="btn btn-sm"
		hx-post={ b.PauseHref }
		hx-target="#live-banner-container"
		hx-swap="outerHTML"
		aria-label="Session pausieren"
		title="Pause"
	>
		<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">
			<line x1="9" y1="5" x2="9" y2="19"></line>
			<line x1="15" y1="5" x2="15" y2="19"></line>
		</svg>
		<span>Pause</span>
	</button>
}
if b.ResumeHref != "" && b.IsPaused {
	<button
		class="btn btn-sm"
		hx-post={ b.ResumeHref }
		hx-target="#live-banner-container"
		hx-swap="outerHTML"
		aria-label="Session fortsetzen"
		title="Weiter"
	>
		<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
			<polygon points="6 4 20 12 6 20 6 4"></polygon>
		</svg>
		<span>Weiter</span>
	</button>
}
```

- [ ] **Step 2: Tick-JS pausenfest machen**

Im `sseTodayScript()`-Block von `internal/webui/templates/worktime/today.templ` den
tick-Zweig erweitern — nach `if (!el) return;`:

```js
				if (el.getAttribute('data-paused') === '1') return;
```

**Semantik-Fix dazu:** `data-started` trägt ab jetzt `StartedUnix + PauseTotal-Sekunden`
(der Zähler rechnet `now − data-started` — Pausen müssen den Anker nach vorn schieben).
Das passiert server-seitig im VM-Bau (Step 3), das Template bleibt dumm.

- [ ] **Step 3: Banner-VM-Bau mit Pause (zwei Stellen, identische Formel)**

`internal/webui/handlers/session_actions.go`, `buildBannerContainerVM` — den Banner-Teil
ersetzen durch:

```go
	now0 := now
	return partials.LiveBannerContainerVM{
		HasActive: true,
		Banner: shared.LiveBanner{
			ProjectLabel: label,
			Tag:          active.Tag,
			ElapsedLabel: formatElapsedHumane(active.Elapsed(now0)),
			StartedAt:    active.StartedAt.In(now0.Location()).Format("15:04"),
			SinceLabel:   bannerSinceLabel(*active),
			StopHref:     "/worktime/active/stop",
			PauseHref:    "/worktime/active/pause",
			ResumeHref:   "/worktime/active/resume",
			IsPaused:     active.PausedAt != nil,
			// data-started-Anker: Start + bisherige Pausen, damit
			// now − Anker == Elapsed (Tick-JS bleibt eine Subtraktion).
			StartedUnix: active.StartedAt.Add(active.PauseTotal).Unix(),
		},
	}
```

mit dem kleinen Helper am Datei-Ende:

```go
// bannerSinceLabel keeps the running label, but says so when frozen.
func bannerSinceLabel(a domain.ActiveSession) string {
	if a.PausedAt != nil {
		return "▮▮ pausiert"
	}
	return "→ läuft"
}
```

In `internal/webui/handlers/worktime_vm.go`, `renderToday`: den `vm.Live = shared.LiveBanner{…}`-
Block durch exakt dieselben Feldzuweisungen ersetzen (ElapsedLabel via
`worktime.FormatElapsedHumane(active.Elapsed(now))`, StartedUnix mit PauseTotal-Anker,
PauseHref/ResumeHref/IsPaused wie oben).

- [ ] **Step 4: Pause/Resume-HTMX-Handler**

`SessionActionsDeps` in `session_actions.go` um das nil-tolerante Feld erweitern:

```go
	// PauseResume is the R1 statemachine surface (pgstore only — nil until
	// Task 18 wires pgstore; the routes are nil-guarded in server.go).
	PauseResume PauseResumeStore
```

Und die zwei Handler ans Datei-Ende:

```go
// — POST /worktime/active/pause + /resume ------------------------------------

// NewActivePause handles POST /worktime/active/pause. Idempotent wie die
// API: pausiert die (einzige) aktive Session des Users, rendert den
// Banner-Container neu.
func NewActivePause(d SessionActionsDeps) http.Handler {
	return newPauseResumeHandler(d, func(userID, projectID string) (domain.ActiveSession, error) {
		return d.PauseResume.Pause(userID, projectID)
	})
}

// NewActiveResume handles POST /worktime/active/resume.
func NewActiveResume(d SessionActionsDeps) http.Handler {
	return newPauseResumeHandler(d, func(userID, projectID string) (domain.ActiveSession, error) {
		return d.PauseResume.Resume(userID, projectID)
	})
}

func newPauseResumeHandler(d SessionActionsDeps, op func(userID, projectID string) (domain.ActiveSession, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := httpserver.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if d.PauseResume == nil {
			http.Error(w, "pause not available", http.StatusNotFound)
			return
		}
		rows, err := d.Active.ListByUser(u.ID)
		if err != nil {
			slog.Error("pause/resume: list failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if len(rows) == 0 {
			_ = partials.LiveBannerContainer(partials.LiveBannerContainerVM{}).Render(r.Context(), w)
			return
		}
		a, err := op(u.ID, rows[0].ProjectID)
		if err != nil {
			slog.Error("pause/resume: op failed", slog.String("err", err.Error()))
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		d.publish(u.ID, "session.updated", map[string]any{
			"project_id": a.ProjectID, "paused": a.PausedAt != nil,
		})
		vm := buildBannerContainerVM(d, u.ID, &a, d.Clock.Now())
		_ = partials.LiveBannerContainer(vm).Render(r.Context(), w)
	})
}
```

- [ ] **Step 5: Handler-Test mit Fake (kein Container nötig)**

```go
// internal/webui/handlers/session_actions_pause_test.go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type fakePauseActive struct {
	active domain.ActiveSession
	paused bool
}

func (f *fakePauseActive) ListByUser(string) ([]domain.ActiveSession, error) {
	return []domain.ActiveSession{f.current()}, nil
}
func (f *fakePauseActive) Get(string, string) (domain.ActiveSession, error) { return f.current(), nil }
func (f *fakePauseActive) Start(string, string, time.Time, string, int64, string, string) (domain.ActiveSession, error) {
	return domain.ActiveSession{}, ports.ErrActiveSessionConflict
}
func (f *fakePauseActive) Stop(string, string, int64, string, string) (domain.Session, error) {
	return domain.Session{}, ports.ErrActiveSessionNotFound
}
func (f *fakePauseActive) Pause(string, string) (domain.ActiveSession, error) {
	f.paused = true
	return f.current(), nil
}
func (f *fakePauseActive) Resume(string, string) (domain.ActiveSession, error) {
	f.paused = false
	return f.current(), nil
}
func (f *fakePauseActive) current() domain.ActiveSession {
	a := f.active
	if f.paused {
		now := time.Now()
		a.PausedAt = &now
	}
	return a
}

type fakeProjects struct{}

func (fakeProjects) ListActive(string) ([]domain.Project, error)      { return nil, nil }
func (fakeProjects) ListAll(string) ([]domain.Project, error)         { return nil, nil }
func (fakeProjects) GetBySlug(string, string) (domain.Project, error) { return domain.Project{}, ports.ErrProjectNotFound }
func (fakeProjects) GetByID(string, string) (domain.Project, error) {
	return domain.Project{ID: "p1", Name: "Demo"}, nil
}
func (fakeProjects) EnsureBySlug(string, string, string) (domain.Project, error) {
	return domain.Project{}, nil
}
func (fakeProjects) Upsert(domain.Project, int64) (domain.Project, error) {
	return domain.Project{}, nil
}
func (fakeProjects) TouchLastUsed(string, string) error { return nil }
func (fakeProjects) Archive(string, string) error       { return nil }

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestActivePauseResume_RendersBannerStates(t *testing.T) {
	t.Parallel()
	fake := &fakePauseActive{active: domain.ActiveSession{
		UserID: "u1", ProjectID: "p1", StartedAt: time.Now().Add(-30 * time.Minute), Version: 1,
	}}
	deps := SessionActionsDeps{
		Active:      fake,
		PauseResume: fake,
		Projects:    fakeProjects{},
		Clock:       fixedClock{t: time.Now()},
	}

	do := func(h http.Handler) string {
		req := httptest.NewRequest(http.MethodPost, "/worktime/active/pause", nil)
		req = req.WithContext(httpserver.WithUser(req.Context(), domain.User{ID: "u1"}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: %d", rec.Code)
		}
		return rec.Body.String()
	}

	body := do(NewActivePause(deps))
	if !strings.Contains(body, "data-paused") || !strings.Contains(body, "Weiter") {
		t.Errorf("paused banner: erwartet data-paused + Weiter-Button, got: %.300s", body)
	}

	body = do(NewActiveResume(deps))
	if strings.Contains(body, "data-paused") || !strings.Contains(body, "Pause") {
		t.Errorf("resumed banner: kein data-paused, Pause-Button erwartet, got: %.300s", body)
	}
}
```

- [ ] **Step 6: SyncState entfernen — ehrlicher Status**

Der Spine-Sync-Dot hat immer `"ok"` gelogen (Spec §10/§12: Fake fliegt raus). Der ehrliche
Status (eingeloggt-als = `UserLabel` in der Nav; Server-Version = Settings-Seite über
BuildInfo) existiert bereits — es ist NUR der Fake zu entfernen:

1. `internal/webui/templates/layout/spine.templ`: Feld `SyncState string` aus `SpineState`
   löschen, die Doc-Zeile dazu löschen, und den kompletten
   `if s.SyncState == "ok" { … } else if … } else { … }`-Block am Ende ersatzlos entfernen
   (der Dot verschwindet — kein neuer Fake-„Verbunden"-Dot).
2. Alle Literale entfernen:

```bash
rg -l 'SyncState:' internal/webui/handlers/
```

   In jeder gefundenen Datei die Zeile `SyncState: "ok",` aus den
   `layout.SpineState{…}`-Literalen löschen (betrifft u. a. `projects.go`,
   `worktime_vm.go`, `repos.go`, `dashboard.go`, `worktime.go`, `settings.go`,
   `notes.go`, `note_actions.go`).
3. `make webui-templ`, dann prüfen, dass NICHTS mehr matcht:

```bash
make webui-templ
rg 'SyncState' internal/ cmd/flow-server/ --type go --type-add 'templ:*.templ' --type templ | rg -v sqliteclient | rg -v sync_state
```

Expected: keine Treffer (die sqliteclient-`SyncState`-Watermark-Tabelle ist Client-Code —
R2-Thema, hier ausgefiltert).

- [ ] **Step 7: Build + Tests + Commit**

```bash
make webui-templ && go build ./... && go test ./internal/webui/... -timeout 300s 2>&1 | tail -4
gofumpt -w internal/webui/handlers/
git add internal/webui/
git commit -m "$(cat <<'EOF'
feat(webui): Pause/Resume-Buttons + pausenfester Tick; SyncState-Fake entfernt (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 18: Projects-API + DER SWAP — config, main.go, server.go auf pgstore

**Files:**
- Create: `internal/adapter/httpserver/projects_api.go`
- Create: `internal/adapter/httpserver/projects_api_test.go`
- Modify: `internal/adapter/httpserver/config.go` + `config_test.go`
- Modify: `internal/adapter/httpserver/auth_browser.go` (AuthDeps)
- Modify: `internal/adapter/httpserver/server.go`
- Modify: `internal/adapter/httpserver/webui.go`
- Modify: `cmd/flow-server/main.go`

Nach diesem Task läuft flow-server NUR noch gegen Postgres; die alten Sync-Routen sind
abgeklemmt (Dateien löscht Task 19). `make ci` muss am Ende grün sein.

- [ ] **Step 1: Projects-API — Failing Test**

```go
// internal/adapter/httpserver/projects_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func newProjectsAPIEnv(t *testing.T, sub string) chi.Router {
	t.Helper()
	u, err := pgstore.NewUsers(pgTestStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountProjectsAPI(r, ProjectsAPIDeps{Projects: pgstore.NewProjects(pgTestStore)})
	return r
}

func TestProjectsAPI_CreateListRenameArchive(t *testing.T) {
	r := newProjectsAPIEnv(t, "api-proj-1")
	do := func(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// Create
	rec := do("POST", "/projects", `{"name":"Mein Projekt","slug":"mein-projekt"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var proj map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &proj)
	id, _ := proj["id"].(string)
	if id == "" || proj["version"].(float64) != 1 {
		t.Fatalf("create payload: %v", proj)
	}
	// Create ohne Name → 422
	rec = do("POST", "/projects", `{"slug":"x"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("create w/o name: want 422, got %d", rec.Code)
	}

	// List (default: nur aktive)
	rec = do("GET", "/projects", "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("list: %d len=%d", rec.Code, len(page.Items))
	}

	// Rename via PUT mit If-Match
	rec = do("PUT", "/projects/"+id, `{"name":"Umbenannt","slug":"mein-projekt"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	// Stale → 412
	rec = do("PUT", "/projects/"+id, `{"name":"x","slug":"mein-projekt"}`, map[string]string{"If-Match": "1"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("stale rename: want 412, got %d", rec.Code)
	}

	// Archivieren via PUT archived=true
	rec = do("PUT", "/projects/"+id, `{"name":"Umbenannt","slug":"mein-projekt","archived":true}`, map[string]string{"If-Match": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	rec = do("GET", "/projects", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 0 {
		t.Errorf("archived project still listed: %v", page.Items)
	}
	rec = do("GET", "/projects?all=1", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 {
		t.Errorf("?all=1 should include archived: %v", page.Items)
	}
}
```

```bash
go test ./internal/adapter/httpserver/ -run TestProjectsAPI -timeout 300s 2>&1 | tail -3
```

Expected: `undefined: MountProjectsAPI`.

- [ ] **Step 2: Projects-API — Implementierung**

```go
// internal/adapter/httpserver/projects_api.go
//
// R1 Bearer-API für Projekte (Spec §7: GET/POST /projects, PUT
// /projects/{id} inkl. Archivieren via archived=true).
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// ProjectsAPIStore is the surface MountProjectsAPI needs (pgstore.Projects).
type ProjectsAPIStore interface {
	ListActive(userID string) ([]domain.Project, error)
	ListAll(userID string) ([]domain.Project, error)
	GetByID(userID, id string) (domain.Project, error)
	EnsureBySlug(userID, name, slug string) (domain.Project, error)
	Upsert(in domain.Project, expectedVersion int64) (domain.Project, error)
}

// ProjectsAPIDeps bundles the projects API dependencies.
type ProjectsAPIDeps struct {
	Projects ProjectsAPIStore
	Bus      *sse.Broadcaster
}

// MountProjectsAPI registers the §7 project routes on r.
func MountProjectsAPI(r chi.Router, d ProjectsAPIDeps) {
	r.Get("/projects", d.handleList)
	r.Post("/projects", d.handleCreate)
	r.Get("/projects/{id}", d.handleGet)
	r.Put("/projects/{id}", d.handlePut)
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

func toProjectDTO(p domain.Project) projectDTO {
	return projectDTO{
		ID: p.ID, Name: p.Name, Slug: p.Slug, ArchivedAt: p.ArchivedAt,
		CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt, Version: p.Version,
	}
}

func (d ProjectsAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "projects")
	}
}

func (d ProjectsAPIDeps) handleList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	list := d.Projects.ListActive
	if r.URL.Query().Get("all") == "1" {
		list = d.Projects.ListAll
	}
	items, err := list(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]projectDTO, 0, len(items))
	for _, p := range items {
		dtos = append(dtos, toProjectDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d ProjectsAPIDeps) handleGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	p, err := d.Projects.GetByID(user.ID, chi.URLParam(r, "id"))
	if errors.Is(err, ports.ErrProjectNotFound) {
		apiError(w, http.StatusNotFound, "projekt existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toProjectDTO(p))
}

func (d ProjectsAPIDeps) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		apiError(w, http.StatusUnprocessableEntity, "name fehlt")
		return
	}
	if in.Slug == "" {
		in.Slug = slugify(in.Name)
	}
	p, err := d.Projects.EnsureBySlug(user.ID, in.Name, in.Slug)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toProjectDTO(p))
}

func (d ProjectsAPIDeps) handlePut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Archived bool   `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		apiError(w, http.StatusUnprocessableEntity, "name/slug fehlen")
		return
	}
	cur, err := d.Projects.GetByID(user.ID, id)
	if errors.Is(err, ports.ErrProjectNotFound) {
		apiError(w, http.StatusNotFound, "projekt existiert nicht")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	next := cur
	next.Name, next.Slug = in.Name, in.Slug
	if in.Archived && next.ArchivedAt == nil {
		now := time.Now().UTC()
		next.ArchivedAt = &now
	}
	if !in.Archived {
		next.ArchivedAt = nil
	}
	saved, err := d.Projects.Upsert(next, expected)
	if errors.Is(err, ports.ErrProjectVersionConflict) {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toProjectDTO(cur)})
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toProjectDTO(saved))
}

// slugify is intentionally minimal: lowercase, spaces→dashes; alles
// Weitere regelt die UNIQUE(user_id, slug)-Constraint.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
```

```bash
go test ./internal/adapter/httpserver/ -run TestProjectsAPI -v -timeout 300s 2>&1 | tail -6
```

Expected: PASS.

- [ ] **Step 3: Config — FLOW_PG_DSN ersetzt FLOW_SERVER_DB + FLOW_NOTEBOOK_ROOT**

`internal/adapter/httpserver/config.go`:

1. Felder `ServerDBPath` + `NotebookRoot` (inkl. Doc-Kommentare) löschen; stattdessen:

```go
	// PgDSN is the PostgreSQL connection string — the server's ONLY
	// truth store after the R1 rebuild (Spec §4). Required.
	// Beispiel: postgres://flow:secret@flow-pg-rw:5432/flow?sslmode=disable
	PgDSN string // FLOW_PG_DSN (Pflicht)
```

2. In `LoadConfig()` die zwei Zeilen ersetzen durch `PgDSN: os.Getenv("FLOW_PG_DSN"),`.
3. In `internal/adapter/httpserver/config_test.go` die Erwartungen anpassen
   (`rg "ServerDBPath|NotebookRoot|FLOW_SERVER_DB|FLOW_NOTEBOOK_ROOT" internal/adapter/httpserver/config_test.go`
   — betroffene Assertions auf `PgDSN`/`FLOW_PG_DSN` umschreiben, Default-Erwartungen für
   die gelöschten Felder entfernen).

- [ ] **Step 4: AuthDeps + server.go — neue Routen rein, Sync-Routen raus**

`internal/adapter/httpserver/auth_browser.go`, `AuthDeps`: die fünf Felder
`ProjectsServer`, `SessionsServer`, `ActiveServer`, `ReposServer`, `RepoNotesServer`
löschen und ersetzen durch:

```go
	// R1 Bearer-API-Deps (Spec §7). Nil-Pointer = Routen nicht gemountet
	// (z. B. in alten Tests, die nur die Auth-Surface brauchen).
	WorktimeAPI  *WorktimeAPIDeps
	ProjectsAPI  *ProjectsAPIDeps
	DocumentsAPI *DocumentsAPIDeps
	MiscAPI      *DayOffsSettingsAPIDeps
	Meta         MetaResponse
```

`internal/adapter/httpserver/server.go`, in `NewWithAuth`:

1. Nach der `/api/v1/oidc/config`-Zeile mounten:

```go
	r.Handle("/api/v1/meta", NewMetaHandler(d.Meta))
```

2. Die KOMPLETTE Bearer-Gruppe (`r.Group`-Block mit `me-bearer` + allen
   pull/push-Routen, heute Zeilen ~69–93) ersetzen durch:

```go
	// Bearer-protected API surface (Spec §7). me-bearer bleibt als
	// CLI-Identitäts-Probe erhalten.
	r.Group(func(rr chi.Router) {
		rr.Use(NewBearerMiddleware(d.Provider, d.Access, d.Users))
		rr.Get("/api/v1/me-bearer", NewMeHandler().ServeHTTP)
		rr.Route("/api/v1", func(api chi.Router) {
			if d.WorktimeAPI != nil {
				MountWorktimeAPI(api, *d.WorktimeAPI)
			}
			if d.ProjectsAPI != nil {
				MountProjectsAPI(api, *d.ProjectsAPI)
			}
			if d.DocumentsAPI != nil {
				MountDocumentsAPI(api, *d.DocumentsAPI)
			}
			if d.MiscAPI != nil {
				MountDayOffsSettingsAPI(api, *d.MiscAPI)
			}
		})
	})
```

3. SSE für ALLE Clients (Spec §5): in `mountWebUI` den `w.Events`-Block AUS der
   Browser-Cookie-Gruppe entfernen und stattdessen (ebenfalls in `mountWebUI`, nach der
   Cookie-Gruppe) mounten:

```go
	// /api/v1/events bedient Browser (Cookie) UND TUI/MCP (Bearer) über
	// dieselbe Route (Spec §5/§7).
	if w.Events != nil {
		r.Group(func(rr chi.Router) {
			rr.Use(NewBearerOrCookieMiddleware(
				NewBearerMiddleware(d.Provider, d.Access, d.Users),
				NewBrowserAuthMiddleware(d.Session, d.Cookie.Name, d.Users),
			))
			rr.Method(http.MethodGet, "/api/v1/events", w.Events)
		})
	}
```

4. In `mountWebUIRead` den notes-Block umstellen:

```go
	if w.DocumentsIndex != nil {
		rr.Method(http.MethodGet, "/notes", w.DocumentsIndex)
	}
	if w.DocumentView != nil {
		rr.Method(http.MethodGet, "/notes/*", notesGetDispatch(w.DocumentView, w.DocumentEdit))
	}
	if w.DocumentPut != nil {
		rr.Method(http.MethodPut, "/notes/*", w.DocumentPut)
	}
```

   (Die Felder `NotesIndex`/`NotesView`/`NoteEdit`/`NotePut` verschwinden in Step 5.)

5. In `mountWebUISessionWrites` nach dem `ActiveStop`-Block:

```go
	if w.ActivePause != nil {
		rr.Method(http.MethodPost, "/worktime/active/pause", w.ActivePause)
	}
	if w.ActiveResume != nil {
		rr.Method(http.MethodPost, "/worktime/active/resume", w.ActiveResume)
	}
```

- [ ] **Step 5: webui.go-Felder**

In `internal/adapter/httpserver/webui.go` im `WebUIHandlers`-Struct:
`NotesIndex`, `NotesView`, `NoteEdit`, `NotePut` löschen; ergänzen:

```go
	// R1 — documents-backed notes surface.
	DocumentsIndex http.Handler
	DocumentView   http.Handler
	DocumentEdit   http.Handler
	DocumentPut    http.Handler

	// R1 — Pause-Statemachine im Today-Banner.
	ActivePause  http.Handler
	ActiveResume http.Handler
```

- [ ] **Step 6: cmd/flow-server/main.go umverkabeln**

1. Imports: `sqliteserver`, `kompfsstore`, `kompports`, `kompusecase`, `path/filepath`
   raus; rein:

```go
	"github.com/serverkraken/flow/internal/adapter/pgstore"
```

2. Direkt nach den Imports die Versions-Variable:

```go
// version is stamped via -ldflags "-X main.version=…" (Makefile,
// Dockerfile); "dev" für ungestempelte Builds.
var version = "dev"
```

3. `requireConfig`: die Prüfung um PG ergänzen —

```go
	if c.PgDSN == "" {
		missing = append(missing, "FLOW_PG_DSN")
	}
```

4. Den „SQLite server store"-Block (MkdirAll + sqliteserver.Open + defer + die sechs
   `sqliteserver.New*`-Zeilen) ersetzen durch:

```go
	// --- Postgres store (R1: die einzige Wahrheit) ---------------------------

	pg, err := pgstore.Open(ctx, cfg.PgDSN)
	if err != nil {
		return errors.New("open postgres: " + err.Error())
	}
	defer pg.Close()

	users := pgstore.NewUsers(pg)
	projects := pgstore.NewProjects(pg)
	sessions := pgstore.NewSessions(pg)
	settings := pgstore.NewSettings(pg)
	activeStore := pgstore.NewActiveSessions(pg, sessions, settings)
	documents := pgstore.NewDocuments(pg)
	dayOffs := pgstore.NewDayOffs(pg)
```

5. `buildWebUIHandlers`-Aufruf: `repos, repoNotes` durch `documents` ersetzen.
6. `httpserver.NewWithAuth(AuthDeps{…})`: die fünf alten `…Server`-Felder ersetzen durch

```go
		WorktimeAPI: &httpserver.WorktimeAPIDeps{
			Sessions: sessions, Active: activeStore, Settings: settings, Bus: broadcaster,
		},
		ProjectsAPI:  &httpserver.ProjectsAPIDeps{Projects: projects, Bus: broadcaster},
		DocumentsAPI: &httpserver.DocumentsAPIDeps{Store: documents, Bus: broadcaster},
		MiscAPI: &httpserver.DayOffsSettingsAPIDeps{
			DayOffs: dayOffs, Settings: settings, Bus: broadcaster,
		},
		Meta: httpserver.MetaResponse{ServerVersion: version, MinClientVersion: "0.0.0"},
```

   und `Ready:` umstellen auf

```go
		Ready: func() error { return pg.Ping(context.Background()) },
```

7. `buildWebUIHandlers` umbauen — Signatur:

```go
func buildWebUIHandlers(
	logger *slog.Logger,
	cfg httpserver.Config,
	sessions *pgstore.Sessions,
	activeStore *pgstore.ActiveSessions,
	projects *pgstore.Projects,
	documents *pgstore.Documents,
	broadcaster *sse.Broadcaster,
) *httpserver.WebUIHandlers {
```

   Im Body: den kompletten Notebook/fsstore-Block (inkl. der beiden `logger.Warn`-Zweige)
   löschen. `notesDeps`/`noteActionsDeps`/`reposDeps` ersetzen durch:

```go
	docDeps := handlers.DocumentsDeps{Store: documents, Markdown: mdRenderer, Clock: clock}
	docActionsDeps := handlers.DocumentActionsDeps{Store: documents, Bus: broadcaster}
	reposDeps := handlers.ReposDeps{Documents: documents, Markdown: mdRenderer, Clock: clock}
	noteActionsDeps := handlers.NoteActionsDeps{Documents: documents, Clock: clock, Bus: broadcaster}
```

   `sessionActionsDeps` um `PauseResume: activeStore,` ergänzen. Im zurückgegebenen
   `WebUIHandlers{…}`-Literal:

```go
		// R1 — documents-backed notes.
		DocumentsIndex: handlers.NewDocumentsIndex(docDeps),
		DocumentView:   handlers.NewDocumentView(docDeps),
		DocumentEdit:   handlers.NewDocumentEdit(docDeps),
		DocumentPut:    handlers.NewDocumentPut(docActionsDeps),

		// R1 — Pause-Statemachine.
		ActivePause:  handlers.NewActivePause(sessionActionsDeps),
		ActiveResume: handlers.NewActiveResume(sessionActionsDeps),
```

   statt der vier `Notes*`/`Note*`-fsstore-Einträge (`RepoNoteEdit`/`RepoNotePut`/
   `ReposIndex`/`RepoNote` bleiben — sie laufen seit Task 16 auf documents).
   Im `Settings`-Block `ServerDBPath: cfg.ServerDBPath` ersetzen durch
   `ServerDBPath: "PostgreSQL (FLOW_PG_DSN)"` — die DSN enthält ein Passwort und hat in
   der UI nichts verloren; das VM-Feld wird in R5 umbenannt.

- [ ] **Step 7: Komplett-Build + Test-Reparatur + ci**

```bash
go build ./... 2>&1 | head -30
```

Übliche Nacharbeiten (alle mechanisch, als Abweichung nur Größeres notieren):

- `server_test.go` / `auth_browser_test.go` / `me_test.go`: Stellen, die die gelöschten
  `…Server`-Felder setzen oder alte Routen asserten →
  `rg "SessionsServer|ActiveServer|ReposServer|RepoNotesServer|ProjectsServer" internal/adapter/httpserver/`
  — Felder aus Test-Wirings entfernen; Route-Assertions auf die neue Tabelle anpassen.
- `integration_e2e_test.go` (`//go:build integration`): kompiliert gegen sqliteserver →
  in diesem Task NUR den Build sichern: `go vet -tags integration ./...`; die inhaltliche
  Umstellung auf pgtest macht Task 19.
- Alte Handler-Tests (`sessions_handlers_test.go` etc.) testen ihre Handler direkt und
  bleiben grün — sie sterben erst in Task 19 mit ihren Handlern.

```bash
go test ./... -timeout 600s 2>&1 | tail -6
make ci 2>&1 | tail -3
```

Expected: grün. Wenn das Coverage-Gate knapp reißt (neue main.go-Pfade sind ungetestet):
NICHT das Gate anfassen — Task 19 vermisst neu; hier nur als Abweichung notieren.

- [ ] **Step 8: Commit**

```bash
gofumpt -w cmd/flow-server/ internal/adapter/httpserver/
git add cmd/flow-server/ internal/adapter/httpserver/ internal/webui/
git commit -m "$(cat <<'EOF'
feat(server)!: Swap auf pgstore — PG ist die einzige Wahrheit (R1)

FLOW_PG_DSN ersetzt FLOW_SERVER_DB + FLOW_NOTEBOOK_ROOT; neue §7-API
(worktime/projects/documents/day-offs/settings/meta) gemountet, alte
pull/push-Sync-Routen abgeklemmt; /api/v1/events bedient Bearer UND
Cookie; WebUI-Notes laufen auf documents, Pause/Resume verdrahtet.

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 19: Löschen — sqliteserver + alte Handler; Tests auf pgstore; Coverage ehrlich

**Files:**
- Delete: `internal/adapter/sqliteserver/` (komplett, inkl. `migrations/`)
- Delete: `internal/adapter/httpserver/{sessions_handlers,active_sessions_handlers,projects_handlers,repos_handlers,repo_notes_handlers}.go` + zugehörige `_test.go`
- Delete: `internal/webui/handlers/{notes.go,notes_vm.go,notes_test.go}`
- Modify: `internal/webui/handlers/note_actions.go` (+ Test), `documents.go`
- Create: `internal/webui/handlers/main_pg_test.go`
- Modify: `internal/usecase/server_worktime_view_test.go`
- Modify: `internal/adapter/httpserver/integration_e2e_test.go`
- Modify: `Makefile` (`COVER_THRESHOLD` ehrlich)

- [ ] **Step 1: userLabelFromContext retten**

`userLabelFromContext` lebt in `notes.go`, wird aber von `settings.go` u. a. genutzt.
Die Funktion (samt Doc-Kommentar) ans Ende von
`internal/webui/handlers/documents.go` verschieben.

- [ ] **Step 2: Alte fsstore-Notes-Handler entfernen**

```bash
git rm internal/webui/handlers/notes.go internal/webui/handlers/notes_vm.go internal/webui/handlers/notes_test.go
```

In `note_actions.go`: `NewNoteEdit` + `NewNotePut` (die fsstore-Pfade) inkl. ihrer Helfer
löschen; das Feld `NoteStore` aus `NoteActionsDeps` und die kompendium-Imports entfernen.
In `note_actions_test.go` die zugehörigen Testfunktionen löschen. Übrig bleiben die
RepoNote-Handler aus Task 16.

- [ ] **Step 3: Alte Server-Handler + sqliteserver löschen**

```bash
git rm internal/adapter/httpserver/sessions_handlers.go internal/adapter/httpserver/sessions_handlers_test.go \
       internal/adapter/httpserver/active_sessions_handlers.go internal/adapter/httpserver/active_sessions_handlers_test.go \
       internal/adapter/httpserver/projects_handlers.go internal/adapter/httpserver/projects_handlers_test.go \
       internal/adapter/httpserver/repos_handlers.go internal/adapter/httpserver/repo_notes_handlers.go
git rm -r internal/adapter/sqliteserver
```

Verifikation, dass nichts mehr darauf zeigt:

```bash
rg "adapter/sqliteserver" --type go -l && echo "NOCH REFERENZEN" || echo "sauber"
```

Expected: `sauber`. Jede verbleibende Referenz ist ein Testfile → nächste Steps.

- [ ] **Step 4: WebUI-Handler-Tests auf pgstore umziehen**

Ein TestMain für das gesamte handlers-Testbinary (gilt für `package handlers` UND
`package handlers_test` — es darf nur EINES geben):

```go
// internal/webui/handlers/main_pg_test.go
package handlers

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// pgWebUIStore backs every store-driven handler test in this package.
var pgWebUIStore *pgstore.Store

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx := context.Background()
		dsn, terminate, err := pgtest.StartContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer terminate()
		s, err := pgstore.Open(ctx, dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgstore open:", err)
			return 1
		}
		defer s.Close()
		pgWebUIStore = s
		return m.Run()
	}())
}

// pgStores is the per-test fixture: fresh user + the four store adapters.
type pgStores struct {
	User      domain.User
	Sessions  *pgstore.Sessions
	Active    *pgstore.ActiveSessions
	Projects  *pgstore.Projects
	Documents *pgstore.Documents
}

func newPGStores(t *testing.T, sub string) pgStores {
	t.Helper()
	u, err := pgstore.NewUsers(pgWebUIStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("newPGStores user: %v", err)
	}
	sessions := pgstore.NewSessions(pgWebUIStore)
	settings := pgstore.NewSettings(pgWebUIStore)
	return pgStores{
		User:      u,
		Sessions:  sessions,
		Active:    pgstore.NewActiveSessions(pgWebUIStore, sessions, settings),
		Projects:  pgstore.NewProjects(pgWebUIStore),
		Documents: pgstore.NewDocuments(pgWebUIStore),
	}
}
```

Dann pro Testdatei mit sqliteserver-Setup
(`rg -l "sqliteserver" internal/webui/handlers/`) den Setup-Teil mechanisch umstellen —
Muster:

- `sqliteserver.Open(filepath.Join(t.TempDir(), "server.db"))` + `NewUsers/...`-Kaskade
  → ein `s := newPGStores(t, "<testname>-user")`-Aufruf; Deps-Felder aus `s.…` befüllen.
- Achtung Eindeutigkeit: jeder Test braucht einen EIGENEN sub-String (geteilter Container).
- Die in Task 16 geskippten Repo-Tests (`t.Skip("R1: …")`) jetzt auf
  `s.Documents.Put(uid, "repos/<urlescape(key)>.md", body, key, 0)` als Seed umschreiben
  und entskippen.
- `events_test.go` (package handlers_test) braucht keinen Store — unverändert lassen.

```bash
go test ./internal/webui/... -timeout 600s 2>&1 | tail -5
```

Expected: PASS, keine Skips mit "R1:"-Marker mehr:

```bash
rg "R1: wird in Task 19" internal/webui/ && echo "NOCH SKIPS" || echo "sauber"
```

- [ ] **Step 5: server_worktime_view_test auf lokale Fakes**

`internal/usecase/server_worktime_view_test.go` nutzt sqliteserver. Die View braucht nur
zwei winzige Interfaces — Setup ersetzen durch In-Memory-Fakes (KEIN Container im
usecase-Paket):

```go
type fakeSessionsReader struct{ sessions []domain.Session }

func (f fakeSessionsReader) ListByUserDateRange(_ string, from, to time.Time) ([]domain.Session, error) {
	var out []domain.Session
	for _, s := range f.sessions {
		if !s.Date.Before(from) && !s.Date.After(to) {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeActiveReader struct{ rows []domain.ActiveSession }

func (f fakeActiveReader) ListByUser(string) ([]domain.ActiveSession, error) {
	return f.rows, nil
}
```

Die bestehenden Testfälle behalten ihre Assertions; nur die Daten kommen jetzt aus den
Fake-Slices statt aus SQLite-Inserts. (Datums-Hinweis: `Date` der Fixtures auf
UTC-Mitternacht setzen, exakt wie `pgstore.scanSession` es liefert.)

- [ ] **Step 6: integration_e2e_test auf pgtest**

In `internal/adapter/httpserver/integration_e2e_test.go` (Build-Tag `integration`):
`sqliteserver.Open(...)`-Setup durch `pgtest.StartContainer` + `pgstore.Open` ersetzen
(Pattern aus `main_pg_test.go`; der Test startet ohnehin schon dex-Container, der
PG-Container fügt sich ein). Die Store-Konstruktionen auf `pgstore.New*` umstellen und die
AuthDeps-Felder auf die neuen API-Deps-Pointer. Kompilier- und Lauf-Check:

```bash
go vet -tags integration ./internal/adapter/httpserver/
go test -tags integration ./internal/adapter/httpserver/ -run TestIntegration -timeout 300s 2>&1 | tail -5
```

Expected: grün (oder dokumentierter Skip, falls der Test Docker-gated ist und übersprungen
wird — dann reicht das `go vet`).

- [ ] **Step 7: Coverage neu vermessen + Gate ehrlich setzen**

```bash
make cover 2>&1 | tail -5
```

Die Löschungen verschieben die Basis (Spec §14). Das Gate in `Makefile`
(`COVER_THRESHOLD := 77`) auf `gemessener Wert − 1` setzen, NUR wenn der Wert unter 77
liegt; liegt er drüber, Gate anheben. Den neuen Wert hier eintragen:

> Coverage nach Task 19: ____ % → COVER_THRESHOLD = ____

- [ ] **Step 8: ci + Commit**

```bash
make ci 2>&1 | tail -3
git add -A
git commit -m "$(cat <<'EOF'
refactor(server)!: sqliteserver + Sync-Handler gelöscht, Tests auf pgstore (R1)

Lösch-Liste Spec §12 server-seitig vollzogen: sqliteserver (inkl.
goose-SQLite-Migrationen + lamport), pull/push-Handler, fsstore-Notes-
Handler. WebUI-Handler-Tests laufen gegen testcontainers-PG; Coverage-
Gate neu vermessen.

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 20: compose-Stack auf Postgres (Litestream/MinIO sterben)

**Files:**
- Modify: `deploy/podman/docker-compose.yml`
- Delete: `deploy/podman/litestream.yml`, `scripts/litestream-restore-drill.sh`, `scripts/smoke-m2-m3.sh`
- Modify: `deploy/podman/README.md`, `Makefile` (drill-Target raus)

- [ ] **Step 1: docker-compose.yml ersetzen**

Kompletter neuer Inhalt von `deploy/podman/docker-compose.yml`:

```yaml
services:
  flow-server:
    build:
      context: ../..
      dockerfile: deploy/podman/Dockerfile.server
    image: flow-server:dev
    env_file: .env
    depends_on:
      postgres:
        condition: service_healthy
      dex:
        condition: service_started
    ports:
      - "8080:8080"

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: flow
      POSTGRES_USER: flow
      POSTGRES_PASSWORD: flow-dev
    ports:
      - "5432:5432"
    volumes:
      - pg-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U flow -d flow"]
      interval: 2s
      timeout: 3s
      retries: 15

  dex:
    image: ghcr.io/dexidp/dex:v2.41.1
    command: ["dex", "serve", "/etc/dex/config.yaml"]
    volumes:
      - ./dex-config.yaml:/etc/dex/config.yaml:Z
    ports:
      - "5556:5556"

volumes:
  pg-data: {}
```

- [ ] **Step 2: Leichen entsorgen**

```bash
git rm deploy/podman/litestream.yml scripts/litestream-restore-drill.sh scripts/smoke-m2-m3.sh
rg "litestream|minio|smoke-m2-m3" Makefile deploy/ scripts/ .github/ --ignore-case -l
```

Jede verbleibende Fundstelle bereinigen: das `drill-restore`-Target im `Makefile` löschen;
README-Abschnitte zu Litestream/MinIO/Backups ersetzen (Step 3); GHA-Workflows referenzieren
die Dateien laut Recon nicht — falls doch, Pfade entfernen.

- [ ] **Step 3: README + .env-Dokumentation**

In `deploy/podman/README.md`: die Litestream/MinIO-Abschnitte ersetzen durch einen
PG-Abschnitt. Dokumentierter `.env`-Block (die Datei selbst ist gitignored):

```
FLOW_SERVER_ADDR=:8080
FLOW_SERVER_BASE_URL=http://localhost:8080
FLOW_PG_DSN=postgres://flow:flow-dev@postgres:5432/flow?sslmode=disable
FLOW_OIDC_ISSUER=http://localhost:5556
FLOW_OIDC_CLIENT_ID=flow-server-dev
FLOW_OIDC_CLIENT_SECRET=dev-secret
FLOW_ALLOWED_SUBS=Cghsb2NhbGRldhIFbG9jYWw
FLOW_COOKIE_HASH_KEY=<openssl rand -hex 32>
FLOW_COOKIE_BLOCK_KEY=<openssl rand -hex 32>
```

Plus ein Satz Betrieb: „Backups übernimmt im Homelab CNPG (Operator-Snapshots + PITR);
der lokale compose-Stack ist Wegwerf-Dev — `podman volume rm` setzt ihn zurück."
`FLOW_SERVER_DB`/`FLOW_NOTEBOOK_ROOT`/`LITESTREAM_*` aus allen Beispielen tilgen:

```bash
rg "FLOW_SERVER_DB|FLOW_NOTEBOOK_ROOT|LITESTREAM" deploy/ docs/ scripts/ -l
```

Gefundene Doku-Stellen (auch `docs/runbook/*.md`) anpassen — Runbook-Schritte, die auf den
SQLite-Pfad zeigen, auf `FLOW_PG_DSN` umschreiben oder als „(R1: ersetzt durch PG)"
markieren.

- [ ] **Step 4: Stack-Probe + Commit**

```bash
cd deploy/podman && podman-compose up -d postgres dex && podman-compose ps && cd ../..
```

Expected: beide Services laufen (flow-server-Probe macht Task 22 mit echter .env).

```bash
cd deploy/podman && podman-compose down && cd ../..
git add -A
git commit -m "$(cat <<'EOF'
chore(deploy): compose auf Postgres 16 — Litestream/MinIO + alte Smokes raus (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 21: Helm-Chart auf PG/CNPG

**Files:**
- Modify: `deploy/helm/flow-server/Chart.yaml`, `values.yaml`,
  `templates/deployment.yaml`, `templates/configmap.yaml`, `templates/secret.yaml`
- Delete: `deploy/helm/flow-server/templates/pvc.yaml`

Das Chart deployt weiterhin NUR flow-server; der CNPG-`Cluster` selbst entsteht als
separates Manifest in homelab-study (Spec §15 Punkt 4 — eigene PRs, nicht dieses Repo).
Das Chart konsumiert das von CNPG generierte App-Secret (`<cluster>-app`, Key `uri`).

- [ ] **Step 1: Chart.yaml**

`description` ersetzen durch:

```yaml
description: |
  flow-server — server-only truth HTTP API + WebUI for flow (R1).
  Stateless; persistence lives in a CloudNativePG cluster whose app
  secret (key `uri`) is referenced via postgres.existingSecret.
version: 0.2.0
```

(`version: 0.1.0` → `0.2.0`.)

- [ ] **Step 2: values.yaml**

Die Blöcke `persistence:` und `litestream:` KOMPLETT löschen; im `flow:`-Block
`serverDBPath` + `notebookRoot` löschen; neu auf Top-Level:

```yaml
postgres:
  # Name des Secrets mit der DSN (CNPG: "<cluster-name>-app") und der Key
  # darin. Das Secret muss im Release-Namespace existieren.
  existingSecret: "flow-pg-app"
  secretKey: "uri"
```

Im `secret:`-Block `litestreamAccessKeyID` + `litestreamSecretAccessKey` löschen. Den
`resources.litestream`-Block löschen.

- [ ] **Step 3: deployment.yaml**

1. Den Kommentar + `strategy: Recreate`-Block ersetzen durch:

```yaml
  # Stateless seit R1 (Postgres via CNPG) — Recreate war ein SQLite-
  # Single-Writer-Zwang und ist Geschichte.
  strategy:
    type: RollingUpdate
```

2. Den kompletten `{{- if .Values.litestream.enabled }}…{{- end }}`-Sidecar-Block löschen.
3. Im flow-server-Container nach `envFrom:` ergänzen:

```yaml
          env:
            - name: FLOW_PG_DSN
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.postgres.existingSecret }}
                  key: {{ .Values.postgres.secretKey }}
```

4. Im `volumes:`-Abschnitt den `flow-data`-Eintrag (inkl. PVC-Verzweigung) und den
   zugehörigen `volumeMounts:`-Eintrag `flow-data` löschen; `tmp` bleibt.
5. `git rm deploy/helm/flow-server/templates/pvc.yaml`.

- [ ] **Step 4: configmap.yaml + secret.yaml**

```bash
rg "serverDBPath|notebookRoot|FLOW_SERVER_DB|FLOW_NOTEBOOK_ROOT|LITESTREAM" deploy/helm/ -l
```

In `configmap.yaml` die Zeilen für `FLOW_SERVER_DB`/`FLOW_NOTEBOOK_ROOT` löschen; in
`secret.yaml` die Litestream-Keys löschen.

- [ ] **Step 5: Render-Probe + Commit**

```bash
helm lint deploy/helm/flow-server
helm template smoke deploy/helm/flow-server \
  --set flow.serverBaseURL=https://flow.example.com \
  --set flow.oidcIssuer=https://auth.example.com/application/o/flow/ \
  --set flow.oidcClientID=flow --set flow.allowedSubs=msoent \
  --set secret.oidcClientSecret=x --set secret.cookieHashKey=$(openssl rand -hex 32) \
  --set secret.cookieBlockKey=$(openssl rand -hex 32) \
  | rg "FLOW_PG_DSN|litestream|persistentVolumeClaim|strategy" -A 2
```

Expected: `FLOW_PG_DSN` mit secretKeyRef erscheint; KEIN litestream, KEIN
persistentVolumeClaim; `RollingUpdate`. (Schlägt `helm template` wegen der zweistufigen
helm-lint-CI-Konvention fehl, die Required-Fields-Prüfung des Workflows lokal nachstellen:
`.github/workflows/helm-lint.yml` lesen und dieselben `--set`-Werte verwenden.)

```bash
git add -A
git commit -m "$(cat <<'EOF'
chore(helm): Chart 0.2.0 — CNPG-Secret-DSN, Litestream-Sidecar + PVC raus (R1)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

### Task 22: Wiring-Verification + Route-Smoke (Pflicht-Abschlusstask)

**Files:**
- Create: `scripts/smoke-r1-routes.sh`

Composition-Root-Audit + curl-Smoke JEDER Route — per-Task-Reviews fangen nicht „der
Composition-Root ruft den neuen Constructor nie auf". 401 statt 404 ist hier der Beweis,
dass eine Bearer-Route GEMOUNTET ist.

- [ ] **Step 1: Constructor-Audit**

```bash
for sym in pgstore.Open NewUsers NewProjects NewSessions NewActiveSessions NewDocuments NewDayOffs NewSettings \
           WorktimeAPIDeps ProjectsAPIDeps DocumentsAPIDeps DayOffsSettingsAPIDeps MetaResponse \
           NewDocumentsIndex NewDocumentView NewDocumentEdit NewDocumentPut NewActivePause NewActiveResume; do
  rg -q "$sym" cmd/flow-server/main.go && echo "OK   $sym" || echo "FEHLT $sym"
done
```

Expected: 18× `OK`. Jedes `FEHLT` ist ein Wiring-Loch → in main.go nachverdrahten.

- [ ] **Step 2: Smoke-Script anlegen**

```bash
#!/usr/bin/env bash
# scripts/smoke-r1-routes.sh — R1 Route-Smoke: beweist pro Route, dass sie
# GEMOUNTET ist (Status-Codes, kein Login nötig: 401/302 sind Beweise).
# Voraussetzungen: podman, openssl, gebautes Repo. Startet PG-Container +
# dex (compose) + flow-server, räumt via trap auf.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_CTR="flow-r1-smoke-pg"
SRV_PID=""
cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
  podman rm -f "$PG_CTR" >/dev/null 2>&1 || true
  (cd deploy/podman && podman-compose down dex >/dev/null 2>&1) || true
}
trap cleanup EXIT

podman run -d --rm --name "$PG_CTR" -p 15432:5432 \
  -e POSTGRES_DB=flow -e POSTGRES_USER=flow -e POSTGRES_PASSWORD=flow \
  postgres:16-alpine >/dev/null
(cd deploy/podman && podman-compose up -d dex >/dev/null)

for i in $(seq 1 30); do
  podman exec "$PG_CTR" pg_isready -U flow -d flow >/dev/null 2>&1 && break
  sleep 1
done

make build-server >/dev/null

FLOW_PG_DSN="postgres://flow:flow@localhost:15432/flow?sslmode=disable" \
FLOW_OIDC_ISSUER="http://localhost:5556" \
FLOW_OIDC_CLIENT_ID="flow-server-dev" \
FLOW_OIDC_CLIENT_SECRET="dev-secret" \
FLOW_ALLOWED_SUBS="Cghsb2NhbGRldhIFbG9jYWw" \
FLOW_COOKIE_HASH_KEY="$(openssl rand -hex 32)" \
FLOW_COOKIE_BLOCK_KEY="$(openssl rand -hex 32)" \
./bin/flow-server &
SRV_PID=$!
sleep 2

BASE="http://localhost:8080"
fail=0
check() { # check METHOD PATH EXPECTED [HEADER]
  local method=$1 path=$2 want=$3 hdr=${4:-}
  local got
  if [ -n "$hdr" ]; then
    got=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" -H "$hdr" "$BASE$path")
  else
    got=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$BASE$path")
  fi
  if [ "$got" = "$want" ]; then
    echo "OK   $method $path → $got"
  else
    echo "FAIL $method $path → $got (want $want)"
    fail=1
  fi
}

echo "— public —"
check GET /healthz 200
check GET /readyz 200
check GET /metrics 200
check GET /api/v1/meta 200
check GET /api/v1/oidc/config 200
check GET /auth/landing 200
check GET /login 302

echo "— Bearer-API: 401 beweist gemountet, 404 wäre ein Wiring-Loch —"
AUTH="Authorization: Bearer kaputt"
check GET  "/api/v1/worktime/sessions?from=2026-01-01&to=2026-01-02" 401 "$AUTH"
check POST /api/v1/worktime/sessions 401 "$AUTH"
check POST /api/v1/worktime/sessions:bulk 401 "$AUTH"
check PUT  /api/v1/worktime/sessions/x 401 "$AUTH"
check DELETE /api/v1/worktime/sessions/x 401 "$AUTH"
check GET  /api/v1/worktime/active 401 "$AUTH"
check POST /api/v1/worktime/active/start 401 "$AUTH"
check POST /api/v1/worktime/active/stop 401 "$AUTH"
check POST /api/v1/worktime/active/pause 401 "$AUTH"
check POST /api/v1/worktime/active/resume 401 "$AUTH"
check GET  /api/v1/projects 401 "$AUTH"
check POST /api/v1/projects 401 "$AUTH"
check PUT  /api/v1/projects/x 401 "$AUTH"
check GET  /api/v1/documents 401 "$AUTH"
check GET  /api/v1/documents/foo.md 401 "$AUTH"
check PUT  /api/v1/documents/foo.md 401 "$AUTH"
check DELETE /api/v1/documents/foo.md 401 "$AUTH"
check GET  /api/v1/repos/key/note 401 "$AUTH"
check PUT  /api/v1/repos/key/note 401 "$AUTH"
check GET  "/api/v1/day-offs?year=2026" 401 "$AUTH"
check PUT  /api/v1/day-offs/2026-01-01 401 "$AUTH"
check DELETE /api/v1/day-offs/2026-01-01 401 "$AUTH"
check GET  /api/v1/settings 401 "$AUTH"
check PUT  /api/v1/settings 401 "$AUTH"
check GET  /api/v1/me-bearer 401 "$AUTH"
check GET  /api/v1/events 401 "$AUTH"

echo "— alte Sync-Routen müssen TOT sein (404/405) —"
check GET /api/v1/sessions 404 "$AUTH"
check GET /api/v1/active 404 "$AUTH"
check GET /api/v1/repos 404 "$AUTH"
check GET /api/v1/repo-notes 404 "$AUTH"

echo "— WebUI (ohne Cookie → 302 auf /auth/landing) —"
for p in / /worktime /notes /repos /projects /settings; do
  check GET "$p" 302
done
check GET /api/v1/events 401 "Accept: text/event-stream"

exit $fail
```

```bash
chmod +x scripts/smoke-r1-routes.sh
```

- [ ] **Step 3: Smoke laufen lassen**

```bash
./scripts/smoke-r1-routes.sh
```

Expected: durchgehend `OK …`, Exit-Code 0. Jedes `FAIL … → 404 (want 401)` ist ein
Mounting-Loch in server.go/main.go — fixen, Smoke wiederholen. (Hinweis: bei den alten
Sync-Routen ist auch 405 akzeptabel — dann das Expected im Script auf den realen Wert
setzen und als Abweichung notieren.)

- [ ] **Step 4: Finale ci + Bestandsaufnahme**

```bash
make ci 2>&1 | tail -3
git log --oneline next ^origin/next | head -30
git status --short
```

Expected: ci grün; die R1-Commits (~20) lokal auf `next`; Working-Tree clean.
**NICHT pushen** — Soenne reviewt und pusht selbst.

- [ ] **Step 5: Commit**

```bash
git add scripts/smoke-r1-routes.sh
git commit -m "$(cat <<'EOF'
test(r1): Route-Smoke-Script — Wiring-Beweis für jede §7-Route (R1 komplett)

Co-Authored-By: agy <noreply@google.com>
EOF
)"
```

---

## Self-Review (gegen Spec §13 R1)

| Spec-Anforderung (R1) | Task |
|---|---|
| pgstore + PG-Baseline (pgx/v5 + goose-PG, frische Baseline) §4/§6 | 1–8 |
| documents-Tabelle + PG-FTS (`simple`, websearch, ts_rank) §6 | 2, 8 |
| Pause-Statemachine (`paused_at`, `pause_total`, idempotent) §6/§7 | 7, 9, 17 |
| Buchungstag in User-Zeitzone (Default Europe/Berlin) §6 | 5, 6, 7 |
| `/meta` (server_version, min_client_version) §7 | 12, 18 |
| §7-API komplett: worktime/projects/documents/repos-alias/day-offs/settings | 9, 10, 11, 18 |
| If-Match/412 + 409-Semantik §7 | 9, 10, 18 |
| SSE generalisiert: `changed {resource}` + Heartbeat 25 s + alle Clients §5/§7 | 9, 13, 18 |
| Sync-Endpoints + sqliteserver + lamport löschen §12/§13 | 18 (abklemmen), 19 (löschen) |
| WebUI auf documents (fsstore/`FLOW_NOTEBOOK_ROOT` raus) §10/§12 | 15, 16, 18, 19 |
| WebUI Pause/Resume-Buttons §10 | 17, 18 |
| WebUI-Status ehrlich (SyncState-Fake raus) §10/§12 | 17 |
| compose + Helm auf PG/CNPG (Litestream + PVC raus) §12/§13 | 20, 21 |
| Testing: testcontainers-PG, Statemachine-Tabellen-Tests, Router-Tests §14 | 2–11, 19 |
| Coverage-Gate ehrlich neu vermessen §14 | 19 |
| Wiring-Verification + curl-Smoke jeder Route (Plan-Regel) | 22 |

**Explizit NICHT R1** (kommt in R2–R6): Client-Umbau (httpapi-Adapter, Statuszeile,
Login-Pflicht-UX, Offline-Snapshot), flow-mcp, Import-Verben (`migrate-from-tsv`,
`flow docs import` — die Server-Seite `:bulk`/`PUT /documents` steht bereit),
WebUI-Responsive-Pass, Multi-Device-E2E, homelab-study-PRs (CNPG-Cluster + Secret-Wiring +
`FLOW_PG_DSN`-Env im Deployment — eigene PRs im anderen Repo, NACH Soennes Review).

**Bekannte Folge auf `next`:** Nach R1 ist der bestehende Client (TUI/CLI/MCP) gegen den
neuen Server funktional tot (Sync-Routen weg). Das ist der geplante Zustand des
Integrationsbranches bis R2 — kein Bug, nicht „reparieren".

## Abweichungs-Protokoll

(Der Executor trägt hier ein, was vom Plan abweichen musste — Task-Nummer + 1–3 Sätze.)

