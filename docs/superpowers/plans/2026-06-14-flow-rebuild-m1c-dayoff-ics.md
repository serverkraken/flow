# flow Rebuild · M1c — DayOff + Feiertage + ICS · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Abwesenheits-Vertical für die `rebuild`-App: manuelle DayOffs (Urlaub/Krank) inkl. Halbtag + berechnete deutsche Feiertage + abonnierbarer ICS-Feed, in Server + TUI + WebUI, live-synced.

**Architecture:** Hexagonal, exakt nach M1a/M1b-Muster: Carry-over-Domain aus `main` → neue `ports` → `pgstore`-Adapter (eine Datei je Store) → dünne `usecase`s, die jede Mutation auf den `sse.Bus` publishen → REST unter `/api/v1/` hinter OIDC/Bearer + ein dritter Auth-Pfad (Token-by-URL) für `/ics/{token}.ics` → TUI (apiclient) + WebUI (templ/HTMX-SSE). Composition-Root: `cmd/flow-server/main.go`.

**Tech Stack:** Go, pgx/pgxpool, goose-Migrationen (embedded), `charm.land/bubbletea/v2`, `a-h/templ` + HTMX + Tailwind, testcontainers (Postgres).

**Branch:** Code auf dem Orphan-Branch `rebuild` (Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`). Planungs-Docs (dieser Plan) auf `main`.

**Spec:** `docs/superpowers/specs/2026-06-14-flow-rebuild-m1c-dayoff-ics-design.md`

---

## Conventions (vom bestehenden Code abgelesen — einhalten)

- REST-Routen: `/api/v1/...`. WebUI-Fragmente: `/ui/...`. ICS-Feed: `/ics/{token}.ics` (kein Versions-Prefix, eigener Auth-Zweig).
- pgstore-Konstruktor: `func NewXStore(pool *pgxpool.Pool) *XStore`. SQL als `const q = \`...\``. Owner-scoped Reads. `errors.Is(err, pgx.ErrNoRows)` → Port-Sentinel.
- Usecase: Struct mit Port-Feldern, `Execute(ctx, ...)`-Methode. Keine Konstruktoren.
- HTTP-Handler: `userFrom(r.Context())`, `writeJSON(w, status, v)`, `http.Error(w, msg, code)`, danach `s.Bus.Publish(domain.Event{...})`.
- Tests: pgstore via `startPG(t)` (testcontainers, schon vorhanden in `internal/adapter/pgstore/`). Usecase via Fakes (in-memory).
- Commit-Stil: `feat(m1c): …` / `test(m1c): …` / `chore(m1c): …`, jeweils mit Trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Gate: `make ci` muss am Ende grün sein.

## File Structure (was entsteht/sich ändert)

**Neu (Domain):**
- `internal/domain/dayoff.go` — Carry-over (Kind/ParseKind/DayOff).
- `internal/domain/holidays_de.go` — Carry-over (GermanHolidays).
- `internal/domain/ics.go` — Carry-over (WriteICS).
- `internal/domain/icalescape.go` — nur `IcalEscape` (aus v1 `format.go` herausgelöst).
- `internal/domain/dayoff_range.go` — neu: `ExpandRange`.
- `internal/domain/settings.go` — neu: `Settings`, `FeedToken`, `ValidBundesland`.

**Neu (Ports / Migration / Store):**
- `internal/ports/ports.go` — ergänzt: `DayOffStore`, `UserSettingsStore`, `FeedTokenStore` + Sentinels.
- `internal/adapter/pgstore/migrations/0003_dayoff_settings_feedtokens.sql`
- `internal/adapter/pgstore/dayoffs.go`, `user_settings.go`, `feed_tokens.go`

**Neu (Usecase):**
- `internal/usecase/add_dayoffs.go`, `delete_dayoff.go`, `list_dayoffs.go`, `ics_feed.go`, `settings.go`

**Neu/geändert (HTTP):**
- `internal/domain/event.go` — `EventDayOffChanged` ergänzt.
- `internal/adapter/httpserver/dayoffs.go` — DayOff + Settings + ICS-Token-Handler.
- `internal/adapter/httpserver/icsfeed.go` — Token-by-URL-Auth + Feed-Handler.
- `internal/adapter/httpserver/server.go` — Felder + Routen ergänzt.

**Neu/geändert (Clients):**
- `internal/adapter/apiclient/dayoffs.go` — Client-Methoden.
- `cmd/flow/dayoff.go` — CLI-Verben.
- `cmd/flow/main.go` — `dayoffCmd()` registrieren.
- `internal/tui/dayoffs.go` — DayOff-Sub-View; `internal/tui/worktime.go` — Tab-Umschaltung.
- `internal/adapter/webui/dayoffs.templ` — Page + Fragment.
- `internal/adapter/httpserver/webui_dayoffs.go` — Web-Handler.

**Geändert (Wiring):**
- `cmd/flow-server/main.go` — Stores + Usecases + Server-Felder verdrahten.

---

## Task 1: Domain carry-over — DayOff

**Files:**
- Create: `internal/domain/dayoff.go`
- Create: `internal/domain/dayoff_test.go`

- [ ] **Step 1: Copy the carry-over file + its test verbatim from `main`**

Im Rebuild-Worktree (`cd /Users/msoent/SourceCode/serverkraken/flow-rebuild`):

```bash
git show main:internal/domain/dayoff.go      > internal/domain/dayoff.go
git show main:internal/domain/dayoff_test.go > internal/domain/dayoff_test.go
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/domain/ -run 'TestKind_LabelDe|TestParseKind' -v`
Expected: PASS (`Kind`, `LabelDe`, `ParseKind`, `DayOff`, `AllKinds` now exist).

- [ ] **Step 3: Commit**

```bash
git add internal/domain/dayoff.go internal/domain/dayoff_test.go
git commit -m "feat(m1c): carry over DayOff domain from v1"
```

---

## Task 2: Domain carry-over — German holidays

**Files:**
- Create: `internal/domain/holidays_de.go`
- Create: `internal/domain/holidays_de_test.go`

- [ ] **Step 1: Copy from `main`**

```bash
git show main:internal/domain/holidays_de.go      > internal/domain/holidays_de.go
git show main:internal/domain/holidays_de_test.go > internal/domain/holidays_de_test.go
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/domain/ -run TestGermanHolidays -v`
Expected: PASS (NRW set, Bavaria Fronleichnam, Berlin excludes it, DE excludes land-specific).

- [ ] **Step 3: Commit**

```bash
git add internal/domain/holidays_de.go internal/domain/holidays_de_test.go
git commit -m "feat(m1c): carry over German-holidays computation from v1"
```

---

## Task 3: Domain carry-over — ICS writer + IcalEscape

**Files:**
- Create: `internal/domain/ics.go`
- Create: `internal/domain/ics_test.go`
- Create: `internal/domain/icalescape.go`

- [ ] **Step 1: Copy ICS writer + test from `main`**

```bash
git show main:internal/domain/ics.go      > internal/domain/ics.go
git show main:internal/domain/ics_test.go > internal/domain/ics_test.go
```

- [ ] **Step 2: Create `internal/domain/icalescape.go`** (nur `IcalEscape`, der Rest von v1 `format.go` ist Kompendium-Zeug → M2)

```go
package domain

import "strings"

// IcalEscape escapes the four characters RFC 5545 §3.3.11 requires escaped
// in TEXT-typed values: backslash, semicolon, comma, newline. Carriage
// returns are dropped — the ICS line ending is CRLF and a literal \r in
// content would corrupt it.
func IcalEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\n", `\n`,
		"\r", "",
	)
	return r.Replace(s)
}
```

- [ ] **Step 3: Run the test**

Run: `go test ./internal/domain/ -run TestWriteICS -v`
Expected: PASS. (If the v1 test name differs, run `go test ./internal/domain/ -v` and confirm the ICS test passes.)

- [ ] **Step 4: Commit**

```bash
git add internal/domain/ics.go internal/domain/ics_test.go internal/domain/icalescape.go
git commit -m "feat(m1c): carry over ICS writer + IcalEscape from v1"
```

---

## Task 4: Domain new — ExpandRange

**Files:**
- Create: `internal/domain/dayoff_range.go`
- Create: `internal/domain/dayoff_range_test.go`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestExpandRange_SkipsWeekends(t *testing.T) {
	// Mon 2026-06-15 .. Sun 2026-06-21, skip weekends → Mon..Fri = 5 days.
	got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 21), domain.KindVacation, "Sommer", 0, true)
	if len(got) != 5 {
		t.Fatalf("want 5 weekday entries, got %d", len(got))
	}
	for _, o := range got {
		if o.Kind != domain.KindVacation || o.Label != "Sommer" || o.Target != 0 {
			t.Fatalf("unexpected entry: %+v", o)
		}
		if wd := o.Date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("weekend leaked in: %s", o.Date)
		}
	}
}

func TestExpandRange_IncludesWeekendsAndHalfDay(t *testing.T) {
	got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 21), domain.KindSick, "", 4*time.Hour, false)
	if len(got) != 7 {
		t.Fatalf("want 7 entries incl. weekend, got %d", len(got))
	}
	if got[0].Target != 4*time.Hour {
		t.Fatalf("want half-day target carried, got %v", got[0].Target)
	}
}

func TestExpandRange_SingleDayAndReversedRange(t *testing.T) {
	if got := domain.ExpandRange(d(2026, 6, 15), d(2026, 6, 15), domain.KindVacation, "", 0, false); len(got) != 1 {
		t.Fatalf("single day: want 1, got %d", len(got))
	}
	if got := domain.ExpandRange(d(2026, 6, 21), d(2026, 6, 15), domain.KindVacation, "", 0, false); got != nil {
		t.Fatalf("reversed range: want nil, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestExpandRange -v`
Expected: FAIL (`undefined: domain.ExpandRange`).

- [ ] **Step 3: Write the implementation**

```go
package domain

import "time"

// ExpandRange expands an inclusive [from, to] date span into one DayOff per
// day. Dates are normalized to midnight in from's location. If to < from the
// result is nil. When skipWeekends is true, Saturdays and Sundays are
// omitted. Every produced entry carries the same kind, label and targetPerDay
// (0 = full day off, >0 = half-day override).
func ExpandRange(from, to time.Time, kind Kind, label string, targetPerDay time.Duration, skipWeekends bool) []DayOff {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, from.Location())
	if to.Before(from) {
		return nil
	}
	var out []DayOff
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if skipWeekends && (day.Weekday() == time.Saturday || day.Weekday() == time.Sunday) {
			continue
		}
		out = append(out, DayOff{Date: day, Kind: kind, Label: label, Target: targetPerDay})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/ -run TestExpandRange -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/dayoff_range.go internal/domain/dayoff_range_test.go
git commit -m "feat(m1c): ExpandRange expands a date span into per-day DayOffs"
```

---

## Task 5: Domain settings types + ports + event

**Files:**
- Create: `internal/domain/settings.go`
- Create: `internal/domain/settings_test.go`
- Modify: `internal/ports/ports.go`
- Modify: `internal/domain/event.go`

- [ ] **Step 1: Write the failing test for `ValidBundesland`**

```go
package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestValidBundesland(t *testing.T) {
	for _, ok := range []string{"NW", "nw", "BY", "DE", "NRW"} {
		if _, valid := domain.ValidBundesland(ok); !valid {
			t.Fatalf("%q should be valid", ok)
		}
	}
	if _, valid := domain.ValidBundesland("XX"); valid {
		t.Fatal("XX should be invalid")
	}
	if got, _ := domain.ValidBundesland("nrw"); got != "NW" {
		t.Fatalf("nrw should normalize to NW, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestValidBundesland -v`
Expected: FAIL (`undefined: domain.ValidBundesland`).

- [ ] **Step 3: Create `internal/domain/settings.go`**

```go
package domain

import (
	"strings"
	"time"
)

// Settings holds per-user preferences. Bundesland drives the computed
// German-holiday set. Future M1d/M1e prefs (daily target, …) extend this.
type Settings struct {
	UserID     string `json:"-"`
	Bundesland string `json:"bundesland"`
}

// FeedToken is a secret used to subscribe to a per-user calendar feed
// without interactive auth. Revoked tokens stop resolving.
type FeedToken struct {
	Token     string    `json:"token"`
	UserID    string    `json:"-"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}

// validLandCodes is the 16 Bundesländer plus "DE" (bundesweit only).
var validLandCodes = map[string]bool{
	"BW": true, "BY": true, "BE": true, "BB": true, "HB": true, "HH": true,
	"HE": true, "MV": true, "NI": true, "NW": true, "RP": true, "SL": true,
	"SN": true, "ST": true, "SH": true, "TH": true, "DE": true,
}

// ValidBundesland normalizes common aliases (NRW→NW, case) and reports
// whether the result is one of the 16 codes (or "DE"). Mirrors the aliasing
// that GermanHolidays applies internally so the API rejects garbage early.
func ValidBundesland(s string) (string, bool) {
	n := strings.ToUpper(strings.TrimSpace(s))
	switch n {
	case "NRW":
		n = "NW"
	case "BAYERN":
		n = "BY"
	case "BADEN-WÜRTTEMBERG", "BADEN-WUERTTEMBERG", "BAWÜ", "BAWUE":
		n = "BW"
	}
	return n, validLandCodes[n]
}
```

- [ ] **Step 4: Add the new ports to `internal/ports/ports.go`** (append to the existing `var (...)` sentinel block and add the three interfaces at the end of the file)

In the existing sentinel block, add:

```go
var (
	ErrProjectNotFound = errors.New("project not found")
	ErrSessionNotFound = errors.New("session not found")
	ErrFeedTokenNotFound = errors.New("feed token not found")
)
```

Append at end of file:

```go
// DayOffStore persists manual day-offs (vacation/sick). Holidays are computed,
// never stored. All reads are owner-scoped. Add upserts on (owner, day).
type DayOffStore interface {
	Add(ctx context.Context, ownerID string, d domain.DayOff) error
	Delete(ctx context.Context, ownerID string, day time.Time) error
	ListRange(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error)
}

// UserSettingsStore persists per-user preferences. Get lazily returns a
// default row (Bundesland "NW") for users that never saved settings.
type UserSettingsStore interface {
	Get(ctx context.Context, userID string) (domain.Settings, error)
	SetBundesland(ctx context.Context, userID, land string) error
}

// FeedTokenStore mints and resolves calendar-feed tokens. Resolve only
// returns owners for active (non-revoked) tokens. Create stores a token the
// caller already minted. Revoke is idempotent.
type FeedTokenStore interface {
	Create(ctx context.Context, ft domain.FeedToken) error
	Resolve(ctx context.Context, token string) (ownerID string, err error)
	ListByUser(ctx context.Context, userID string) ([]domain.FeedToken, error)
	Revoke(ctx context.Context, userID, token string) error
}
```

- [ ] **Step 5: Add the event type to `internal/domain/event.go`** (extend the `const (...)` block)

```go
const (
	EventSessionStarted EventType = "session.started"
	EventSessionStopped EventType = "session.stopped"
	EventSessionUpdated EventType = "session.updated"
	EventProjectCreated EventType = "project.created"
	EventDayOffChanged  EventType = "dayoff.changed"
)
```

- [ ] **Step 6: Run tests + build**

Run: `go build ./... && go test ./internal/domain/ -run TestValidBundesland -v`
Expected: build OK, test PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/settings.go internal/domain/settings_test.go internal/ports/ports.go internal/domain/event.go
git commit -m "feat(m1c): settings/feed-token types, ports, dayoff.changed event"
```

---

## Task 6: Migration 0003 + DayOffStore (pgstore)

**Files:**
- Create: `internal/adapter/pgstore/migrations/0003_dayoff_settings_feedtokens.sql`
- Create: `internal/adapter/pgstore/dayoffs.go`
- Create: `internal/adapter/pgstore/dayoffs_test.go`

- [ ] **Step 1: Create the migration**

```sql
-- +goose Up
CREATE TABLE day_offs (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    day        DATE NOT NULL,
    kind       TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    target_min INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, day)
);
CREATE INDEX day_offs_owner_day ON day_offs (owner_id, day);

CREATE TABLE user_settings (
    user_id    TEXT PRIMARY KEY REFERENCES users(id),
    bundesland TEXT NOT NULL DEFAULT 'NW',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE feed_tokens (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    kind       TEXT NOT NULL DEFAULT 'ics',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX feed_tokens_user ON feed_tokens (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE feed_tokens;
DROP TABLE user_settings;
DROP TABLE day_offs;
```

- [ ] **Step 2: Write the failing test** (`internal/adapter/pgstore/dayoffs_test.go`)

```go
package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func seedUser(t *testing.T, ctx context.Context, pool interface{ Close() }, id string) {}

func TestDayOffStore_AddListDelete(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	store := pgstore.NewDayOffStore(pool)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := store.Add(ctx, "u1", domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Re-add same day = upsert (no unique violation), updates kind/label/target.
	if err := store.Add(ctx, "u1", domain.DayOff{Date: day, Kind: domain.KindSick, Label: "", Target: 4 * time.Hour}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.ListRange(ctx, "u1", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1))
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %d err=%v", len(got), err)
	}
	if got[0].Kind != domain.KindSick || got[0].Target != 4*time.Hour {
		t.Fatalf("upsert did not overwrite: %+v", got[0])
	}
	// Owner isolation.
	if other, _ := store.ListRange(ctx, "u2", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)); len(other) != 0 {
		t.Fatalf("owner isolation broken: %d", len(other))
	}
	// Delete is idempotent.
	if err := store.Delete(ctx, "u1", day); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, "u1", day); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
	if got, _ := store.ListRange(ctx, "u1", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)); len(got) != 0 {
		t.Fatalf("want empty after delete, got %d", len(got))
	}
}
```

(The unused `seedUser` stub above is a leftover — delete it; the test seeds inline.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestDayOffStore -v`
Expected: FAIL (`undefined: pgstore.NewDayOffStore`).

- [ ] **Step 4: Write `internal/adapter/pgstore/dayoffs.go`**

```go
package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type DayOffStore struct{ pool *pgxpool.Pool }

func NewDayOffStore(pool *pgxpool.Pool) *DayOffStore { return &DayOffStore{pool: pool} }

// Add upserts on (owner_id, day): a second entry for the same day overwrites
// kind/label/target. id is derived from owner+day so re-adds are stable.
func (s *DayOffStore) Add(ctx context.Context, ownerID string, d domain.DayOff) error {
	const q = `
INSERT INTO day_offs (id, owner_id, day, kind, label, target_min)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (owner_id, day) DO UPDATE
SET kind = EXCLUDED.kind, label = EXCLUDED.label, target_min = EXCLUDED.target_min`
	id := ownerID + ":" + d.Date.Format("2006-01-02")
	_, err := s.pool.Exec(ctx, q, id, ownerID, d.Date, string(d.Kind), d.Label, int(d.Target/time.Minute))
	if err != nil {
		return fmt.Errorf("pgstore: add dayoff: %w", err)
	}
	return nil
}

func (s *DayOffStore) Delete(ctx context.Context, ownerID string, day time.Time) error {
	const q = `DELETE FROM day_offs WHERE owner_id=$1 AND day=$2`
	_, err := s.pool.Exec(ctx, q, ownerID, day)
	if err != nil {
		return fmt.Errorf("pgstore: delete dayoff: %w", err)
	}
	return nil
}

func (s *DayOffStore) ListRange(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error) {
	const q = `
SELECT day, kind, label, target_min FROM day_offs
WHERE owner_id=$1 AND day >= $2 AND day <= $3
ORDER BY day`
	rows, err := s.pool.Query(ctx, q, ownerID, from, to)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list dayoffs: %w", err)
	}
	defer rows.Close()
	var out []domain.DayOff
	for rows.Next() {
		var (
			day       time.Time
			kind      string
			label     string
			targetMin int
		)
		if err := rows.Scan(&day, &kind, &label, &targetMin); err != nil {
			return nil, fmt.Errorf("pgstore: scan dayoff: %w", err)
		}
		out = append(out, domain.DayOff{
			Date: day, Kind: domain.Kind(kind), Label: label,
			Target: time.Duration(targetMin) * time.Minute,
		})
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Remove the `seedUser` stub from the test, then run**

Run: `go test ./internal/adapter/pgstore/ -run TestDayOffStore -v`
Expected: PASS (migration applies cleanly, upsert/owner-isolation/idempotent-delete all green).

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/pgstore/migrations/0003_dayoff_settings_feedtokens.sql internal/adapter/pgstore/dayoffs.go internal/adapter/pgstore/dayoffs_test.go
git commit -m "feat(m1c): migration 0003 + DayOffStore (upsert per day)"
```

---

## Task 7: UserSettingsStore (pgstore)

**Files:**
- Create: `internal/adapter/pgstore/user_settings.go`
- Create: `internal/adapter/pgstore/user_settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package pgstore_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestUserSettingsStore_LazyDefaultAndSet(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	store := pgstore.NewUserSettingsStore(pool)
	// No row yet → lazy default NW.
	got, err := store.Get(ctx, "u1")
	if err != nil || got.Bundesland != "NW" {
		t.Fatalf("lazy default = %+v err=%v", got, err)
	}
	if err := store.SetBundesland(ctx, "u1", "BY"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ = store.Get(ctx, "u1")
	if got.Bundesland != "BY" {
		t.Fatalf("want BY after set, got %q", got.Bundesland)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestUserSettingsStore -v`
Expected: FAIL (`undefined: pgstore.NewUserSettingsStore`).

- [ ] **Step 3: Write `internal/adapter/pgstore/user_settings.go`**

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type UserSettingsStore struct{ pool *pgxpool.Pool }

func NewUserSettingsStore(pool *pgxpool.Pool) *UserSettingsStore {
	return &UserSettingsStore{pool: pool}
}

// Get returns the stored settings or a lazy default (Bundesland "NW") when the
// user never saved any. It does not write the default row — SetBundesland does.
func (s *UserSettingsStore) Get(ctx context.Context, userID string) (domain.Settings, error) {
	const q = `SELECT bundesland FROM user_settings WHERE user_id=$1`
	var land string
	err := s.pool.QueryRow(ctx, q, userID).Scan(&land)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settings{UserID: userID, Bundesland: "NW"}, nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("pgstore: get settings: %w", err)
	}
	return domain.Settings{UserID: userID, Bundesland: land}, nil
}

func (s *UserSettingsStore) SetBundesland(ctx context.Context, userID, land string) error {
	const q = `
INSERT INTO user_settings (user_id, bundesland, updated_at)
VALUES ($1,$2, now())
ON CONFLICT (user_id) DO UPDATE SET bundesland = EXCLUDED.bundesland, updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, userID, land); err != nil {
		return fmt.Errorf("pgstore: set bundesland: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/pgstore/ -run TestUserSettingsStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/pgstore/user_settings.go internal/adapter/pgstore/user_settings_test.go
git commit -m "feat(m1c): UserSettingsStore with lazy NW default"
```

---

## Task 8: FeedTokenStore (pgstore)

**Files:**
- Create: `internal/adapter/pgstore/feed_tokens.go`
- Create: `internal/adapter/pgstore/feed_tokens_test.go`

- [ ] **Step 1: Write the failing test**

```go
package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestFeedTokenStore_CreateResolveRevoke(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "Martin")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	store := pgstore.NewFeedTokenStore(pool)
	ft := domain.FeedToken{Token: "tok-abc", UserID: "u1", Kind: "ics", CreatedAt: time.Now()}
	if err := store.Create(ctx, ft); err != nil {
		t.Fatalf("create: %v", err)
	}
	owner, err := store.Resolve(ctx, "tok-abc")
	if err != nil || owner != "u1" {
		t.Fatalf("resolve = %q err=%v", owner, err)
	}
	if _, err := store.Resolve(ctx, "nope"); !errors.Is(err, ports.ErrFeedTokenNotFound) {
		t.Fatalf("unknown token: want ErrFeedTokenNotFound, got %v", err)
	}
	if err := store.Revoke(ctx, "u1", "tok-abc"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.Resolve(ctx, "tok-abc"); !errors.Is(err, ports.ErrFeedTokenNotFound) {
		t.Fatalf("revoked token must not resolve, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/pgstore/ -run TestFeedTokenStore -v`
Expected: FAIL (`undefined: pgstore.NewFeedTokenStore`).

- [ ] **Step 3: Write `internal/adapter/pgstore/feed_tokens.go`**

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type FeedTokenStore struct{ pool *pgxpool.Pool }

func NewFeedTokenStore(pool *pgxpool.Pool) *FeedTokenStore { return &FeedTokenStore{pool: pool} }

func (s *FeedTokenStore) Create(ctx context.Context, ft domain.FeedToken) error {
	const q = `INSERT INTO feed_tokens (token, user_id, kind, created_at) VALUES ($1,$2,$3,$4)`
	if _, err := s.pool.Exec(ctx, q, ft.Token, ft.UserID, ft.Kind, ft.CreatedAt); err != nil {
		return fmt.Errorf("pgstore: create feed token: %w", err)
	}
	return nil
}

// Resolve returns the owner of an active (non-revoked) token, or
// ErrFeedTokenNotFound for unknown/revoked tokens (no existence leak).
func (s *FeedTokenStore) Resolve(ctx context.Context, token string) (string, error) {
	const q = `SELECT user_id FROM feed_tokens WHERE token=$1 AND revoked_at IS NULL`
	var owner string
	err := s.pool.QueryRow(ctx, q, token).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ports.ErrFeedTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("pgstore: resolve feed token: %w", err)
	}
	return owner, nil
}

func (s *FeedTokenStore) ListByUser(ctx context.Context, userID string) ([]domain.FeedToken, error) {
	const q = `SELECT token, kind, created_at FROM feed_tokens WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list feed tokens: %w", err)
	}
	defer rows.Close()
	var out []domain.FeedToken
	for rows.Next() {
		ft := domain.FeedToken{UserID: userID}
		if err := rows.Scan(&ft.Token, &ft.Kind, &ft.CreatedAt); err != nil {
			return nil, fmt.Errorf("pgstore: scan feed token: %w", err)
		}
		out = append(out, ft)
	}
	return out, rows.Err()
}

// Revoke marks the token revoked. Idempotent and owner-scoped.
func (s *FeedTokenStore) Revoke(ctx context.Context, userID, token string) error {
	const q = `UPDATE feed_tokens SET revoked_at = now() WHERE user_id=$1 AND token=$2 AND revoked_at IS NULL`
	if _, err := s.pool.Exec(ctx, q, userID, token); err != nil {
		return fmt.Errorf("pgstore: revoke feed token: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/pgstore/ -run TestFeedTokenStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/pgstore/feed_tokens.go internal/adapter/pgstore/feed_tokens_test.go
git commit -m "feat(m1c): FeedTokenStore (resolve active, idempotent revoke)"
```

---

## Task 9: Usecases — Add / Delete / List DayOffs (merge)

**Files:**
- Create: `internal/usecase/add_dayoffs.go`
- Create: `internal/usecase/delete_dayoff.go`
- Create: `internal/usecase/list_dayoffs.go`
- Create: `internal/usecase/dayoffs_test.go`

- [ ] **Step 1: Write the failing test** (in-memory fakes for the stores)

```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeDayOffs is an in-memory DayOffStore keyed by (owner, yyyy-mm-dd).
type fakeDayOffs struct{ m map[string]domain.DayOff }

func newFakeDayOffs() *fakeDayOffs { return &fakeDayOffs{m: map[string]domain.DayOff{}} }
func key(owner string, day time.Time) string { return owner + ":" + day.Format("2006-01-02") }

func (f *fakeDayOffs) Add(_ context.Context, owner string, d domain.DayOff) error {
	f.m[key(owner, d.Date)] = d
	return nil
}
func (f *fakeDayOffs) Delete(_ context.Context, owner string, day time.Time) error {
	delete(f.m, key(owner, day))
	return nil
}
func (f *fakeDayOffs) ListRange(_ context.Context, owner string, from, to time.Time) ([]domain.DayOff, error) {
	var out []domain.DayOff
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if v, ok := f.m[key(owner, d)]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

type fakeSettings struct{ land string }

func (f fakeSettings) Get(context.Context, string) (domain.Settings, error) {
	return domain.Settings{Bundesland: f.land}, nil
}
func (f fakeSettings) SetBundesland(context.Context, string, string) error { return nil }

type recBus struct{ events []domain.Event }

func (b *recBus) Publish(ev domain.Event)                      { b.events = append(b.events, ev) }
func (b *recBus) Subscribe(string) (<-chan domain.Event, func()) { return nil, func() {} }

func TestAddDayOffs_ExpandsAndPublishesOnce(t *testing.T) {
	store := newFakeDayOffs()
	bus := &recBus{}
	uc := usecase.AddDayOffs{Store: store, Bus: bus}
	from := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // Mon
	to := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)   // Fri
	if err := uc.Execute(context.Background(), "u1", from, to, domain.KindVacation, "Sommer", 0, true); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(store.m) != 5 {
		t.Fatalf("want 5 stored days, got %d", len(store.m))
	}
	if len(bus.events) != 1 || bus.events[0].Type != domain.EventDayOffChanged {
		t.Fatalf("want exactly one dayoff.changed, got %+v", bus.events)
	}
}

func TestAddDayOffs_RejectsHolidayKind(t *testing.T) {
	uc := usecase.AddDayOffs{Store: newFakeDayOffs(), Bus: &recBus{}}
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := uc.Execute(context.Background(), "u1", d, d, domain.KindHoliday, "", 0, false); err == nil {
		t.Fatal("holiday kind must be rejected (holidays are computed)")
	}
}

func TestListDayOffs_MergesComputedHolidays(t *testing.T) {
	store := newFakeDayOffs()
	// Manual vacation on 2026-06-15.
	_ = store.Add(context.Background(), "u1", domain.DayOff{
		Date: time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local), Kind: domain.KindVacation,
	})
	uc := usecase.ListDayOffs{Store: store, Settings: fakeSettings{land: "NW"}, Loc: time.Local}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	got, err := uc.Execute(context.Background(), "u1", from, to)
	if err != nil {
		t.Fatal(err)
	}
	var vac, hol int
	for _, d := range got {
		switch d.Kind {
		case domain.KindVacation:
			vac++
		case domain.KindHoliday:
			hol++
		}
	}
	if vac != 1 || hol == 0 {
		t.Fatalf("want 1 vacation + NRW holidays merged, got vac=%d hol=%d", vac, hol)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/ -run 'TestAddDayOffs|TestListDayOffs' -v`
Expected: FAIL (`undefined: usecase.AddDayOffs` etc.).

- [ ] **Step 3: Write `internal/usecase/add_dayoffs.go`**

```go
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrHolidayNotManual rejects attempts to store a holiday — holidays are
// computed from the user's Bundesland, never persisted.
var ErrHolidayNotManual = errors.New("holiday kind is computed, not stored")

// AddDayOffs expands [from,to] into per-day rows (skipping weekends when
// asked), upserts each, and publishes a single dayoff.changed event.
type AddDayOffs struct {
	Store ports.DayOffStore
	Bus   ports.EventBus
}

func (uc AddDayOffs) Execute(ctx context.Context, ownerID string, from, to time.Time, kind domain.Kind, label string, targetPerDay time.Duration, skipWeekends bool) error {
	if kind == domain.KindHoliday {
		return ErrHolidayNotManual
	}
	if _, ok := domain.ParseKind(string(kind)); !ok {
		return domain.ErrInvalidDayOff
	}
	days := domain.ExpandRange(from, to, kind, label, targetPerDay, skipWeekends)
	if len(days) == 0 {
		return domain.ErrInvalidDayOff
	}
	for _, d := range days {
		if err := uc.Store.Add(ctx, ownerID, d); err != nil {
			return err
		}
	}
	uc.Bus.Publish(domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
```

- [ ] **Step 4: Add `domain.ErrInvalidDayOff`** to the domain error set.

Find where domain sentinel errors live (e.g. `internal/domain/errors.go` — grep `ErrInvalidUser`):

Run: `rg -n "ErrInvalidUser|ErrProjectRequired" internal/domain/`

Add alongside them:

```go
var ErrInvalidDayOff = errors.New("invalid day-off")
```

- [ ] **Step 5: Write `internal/usecase/delete_dayoff.go`**

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteDayOff removes one manual day-off and publishes dayoff.changed.
// Deleting a non-existent day is a no-op (still publishes — harmless).
type DeleteDayOff struct {
	Store ports.DayOffStore
	Bus   ports.EventBus
}

func (uc DeleteDayOff) Execute(ctx context.Context, ownerID string, day time.Time) error {
	if err := uc.Store.Delete(ctx, ownerID, day); err != nil {
		return err
	}
	uc.Bus.Publish(domain.Event{Type: domain.EventDayOffChanged, UserID: ownerID})
	return nil
}
```

- [ ] **Step 6: Write `internal/usecase/list_dayoffs.go`**

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ListDayOffs returns the merged set of manual day-offs (vacation/sick) and
// computed German holidays for the user's Bundesland over [from,to]. On a
// date collision the manual entry wins (holidays are dropped for that day).
// This is the single read source for TUI, WebUI and the ICS feed.
type ListDayOffs struct {
	Store    ports.DayOffStore
	Settings ports.UserSettingsStore
	Loc      *time.Location
}

func (uc ListDayOffs) Execute(ctx context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error) {
	manual, err := uc.Store.ListRange(ctx, ownerID, from, to)
	if err != nil {
		return nil, err
	}
	set, err := uc.Settings.Get(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]bool, len(manual))
	out := make([]domain.DayOff, 0, len(manual))
	for _, d := range manual {
		taken[d.Date.Format("2006-01-02")] = true
		out = append(out, d)
	}
	for y := from.Year(); y <= to.Year(); y++ {
		for _, h := range domain.GermanHolidays(y, set.Bundesland, uc.Loc) {
			if h.Date.Before(from) || h.Date.After(to) {
				continue
			}
			if taken[h.Date.Format("2006-01-02")] {
				continue
			}
			out = append(out, h)
		}
	}
	return out, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run 'TestAddDayOffs|TestListDayOffs' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/usecase/add_dayoffs.go internal/usecase/delete_dayoff.go internal/usecase/list_dayoffs.go internal/usecase/dayoffs_test.go internal/domain/errors.go
git commit -m "feat(m1c): AddDayOffs/DeleteDayOff/ListDayOffs (merge computed holidays)"
```

---

## Task 10: Usecases — ICS feed + settings/token

**Files:**
- Create: `internal/usecase/ics_feed.go`
- Create: `internal/usecase/settings.go`
- Create: `internal/usecase/ics_settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package usecase_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeFeedTokens struct {
	created []domain.FeedToken
	revoked []string
}

func (f *fakeFeedTokens) Create(_ context.Context, ft domain.FeedToken) error {
	f.created = append(f.created, ft)
	return nil
}
func (f *fakeFeedTokens) Resolve(_ context.Context, token string) (string, error) {
	for _, t := range f.created {
		if t.Token == token {
			return t.UserID, nil
		}
	}
	return "", nil
}
func (f *fakeFeedTokens) ListByUser(_ context.Context, _ string) ([]domain.FeedToken, error) {
	return f.created, nil
}
func (f *fakeFeedTokens) Revoke(_ context.Context, _ , token string) error {
	f.revoked = append(f.revoked, token)
	return nil
}

func TestRegenerateIcsToken_RevokesOldMintsNew(t *testing.T) {
	ft := &fakeFeedTokens{created: []domain.FeedToken{{Token: "old", UserID: "u1", Kind: "ics"}}}
	uc := usecase.RegenerateIcsToken{Tokens: ft, Clock: fixedClock{time.Now()}}
	tok, err := uc.Execute(context.Background(), "u1")
	if err != nil || tok == "" || tok == "old" {
		t.Fatalf("token = %q err=%v", tok, err)
	}
	if len(ft.revoked) != 1 || ft.revoked[0] != "old" {
		t.Fatalf("old token not revoked: %+v", ft.revoked)
	}
}

func TestIcsFeed_WritesManualOnly(t *testing.T) {
	store := newFakeDayOffs()
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	_ = store.Add(context.Background(), "u1", domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer"})
	ft := &fakeFeedTokens{created: []domain.FeedToken{{Token: "tok", UserID: "u1", Kind: "ics"}}}
	uc := usecase.IcsFeed{Tokens: ft, Store: store, Clock: fixedClock{day}}
	var buf bytes.Buffer
	if err := uc.Execute(context.Background(), "tok", &buf); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "BEGIN:VCALENDAR") || !strings.Contains(out, "Sommer") {
		t.Fatalf("ICS missing content:\n%s", out)
	}
	// Holidays must NOT be in the feed.
	if strings.Contains(out, "Neujahr") {
		t.Fatalf("holiday leaked into feed:\n%s", out)
	}
}

// fixedClock is a ports.Clock returning a constant time.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/ -run 'TestRegenerateIcsToken|TestIcsFeed' -v`
Expected: FAIL (`undefined: usecase.RegenerateIcsToken` / `usecase.IcsFeed`).

- [ ] **Step 3: Write `internal/usecase/ics_feed.go`**

```go
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// IcsFeed resolves a feed token to its owner and writes a VCALENDAR of that
// owner's manual day-offs (vacation/sick) for [now-1y, now+1y]. Computed
// holidays are intentionally excluded — they are public and already present
// in the subscriber's calendar.
type IcsFeed struct {
	Tokens ports.FeedTokenStore
	Store  ports.DayOffStore
	Clock  ports.Clock
}

func (uc IcsFeed) Execute(ctx context.Context, token string, w io.Writer) error {
	owner, err := uc.Tokens.Resolve(ctx, token)
	if err != nil {
		return err // ports.ErrFeedTokenNotFound bubbles to a 404
	}
	now := uc.Clock.Now()
	from := now.AddDate(-1, 0, 0)
	to := now.AddDate(1, 0, 0)
	dayoffs, err := uc.Store.ListRange(ctx, owner, from, to)
	if err != nil {
		return err
	}
	return domain.WriteICS(w, dayoffs, now)
}

// RegenerateIcsToken revokes the user's existing active tokens and mints a
// fresh one. The new token is returned for display (it is the secret).
type RegenerateIcsToken struct {
	Tokens ports.FeedTokenStore
	Clock  ports.Clock
}

func (uc RegenerateIcsToken) Execute(ctx context.Context, ownerID string) (string, error) {
	existing, err := uc.Tokens.ListByUser(ctx, ownerID)
	if err != nil {
		return "", err
	}
	for _, t := range existing {
		if err := uc.Tokens.Revoke(ctx, ownerID, t.Token); err != nil {
			return "", err
		}
	}
	tok, err := newFeedToken()
	if err != nil {
		return "", err
	}
	ft := domain.FeedToken{Token: tok, UserID: ownerID, Kind: "ics", CreatedAt: uc.Clock.Now()}
	if err := uc.Tokens.Create(ctx, ft); err != nil {
		return "", err
	}
	return tok, nil
}

// newFeedToken returns a 32-byte crypto-random, base64url-encoded secret.
func newFeedToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var _ = time.Now // keep time import if trimmed by goimports
```

(Remove the `var _ = time.Now` line if `time` is already referenced; it's only a guard so the file compiles standalone.)

- [ ] **Step 4: Write `internal/usecase/settings.go`**

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetSettings returns the user's settings plus their active feed tokens, for
// the settings screen.
type GetSettings struct {
	Settings ports.UserSettingsStore
	Tokens   ports.FeedTokenStore
}

func (uc GetSettings) Execute(ctx context.Context, ownerID string) (domain.Settings, []domain.FeedToken, error) {
	set, err := uc.Settings.Get(ctx, ownerID)
	if err != nil {
		return domain.Settings{}, nil, err
	}
	toks, err := uc.Tokens.ListByUser(ctx, ownerID)
	if err != nil {
		return domain.Settings{}, nil, err
	}
	return set, toks, nil
}

// SetBundesland validates and stores the user's Bundesland.
type SetBundesland struct {
	Settings ports.UserSettingsStore
}

func (uc SetBundesland) Execute(ctx context.Context, ownerID, land string) error {
	norm, ok := domain.ValidBundesland(land)
	if !ok {
		return domain.ErrInvalidDayOff // reuse 400-bubbling sentinel; see handler mapping
	}
	return uc.Settings.SetBundesland(ctx, ownerID, norm)
}
```

(Note: if you prefer a dedicated `domain.ErrInvalidBundesland`, add it next to `ErrInvalidDayOff` and map it to 400 in Task 12. Either is fine; the plan maps both to 400.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/usecase/ -run 'TestRegenerateIcsToken|TestIcsFeed' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/ics_feed.go internal/usecase/settings.go internal/usecase/ics_settings_test.go internal/domain/errors.go
git commit -m "feat(m1c): IcsFeed + token regenerate + settings usecases"
```

---

## Task 11: HTTP — DayOff REST handlers

**Files:**
- Create: `internal/adapter/httpserver/dayoffs.go`
- Modify: `internal/adapter/httpserver/server.go` (Server fields + routes)
- Create: `internal/adapter/httpserver/dayoffs_test.go`

- [ ] **Step 1: Add the usecase fields to `Server` in `server.go`** (inside the `// worktime usecases` block)

```go
	// m1c worktime extras
	AddDayOffs    usecase.AddDayOffs
	DeleteDayOff  usecase.DeleteDayOff
	ListDayOffs   usecase.ListDayOffs
	GetSettings   usecase.GetSettings
	SetBundesland usecase.SetBundesland
	IcsFeed       usecase.IcsFeed
	RegenIcsToken usecase.RegenerateIcsToken
```

- [ ] **Step 2: Add the routes to `Routes()` in `server.go`** (after the projects routes)

```go
	mux.Handle("GET /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleListDayOffs)))
	mux.Handle("POST /api/v1/dayoffs", s.auth(http.HandlerFunc(s.handleAddDayOffs)))
	mux.Handle("DELETE /api/v1/dayoffs/{day}", s.auth(http.HandlerFunc(s.handleDeleteDayOff)))
	mux.Handle("GET /api/v1/settings", s.auth(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("POST /api/v1/settings/bundesland", s.auth(http.HandlerFunc(s.handleSetBundesland)))
	mux.Handle("POST /api/v1/ics-token/regenerate", s.auth(http.HandlerFunc(s.handleRegenIcsToken)))
	mux.HandleFunc("GET /ics/{token}.ics", s.handleIcsFeed) // token-by-URL, Task 13
```

- [ ] **Step 3: Write the failing handler test**

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDayOffRoundTrip(t *testing.T) {
	srv := newTestServer(t) // shared helper in this package (see server_test.go)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	body := `{"from":"2026-06-15","to":"2026-06-19","kind":"vacation","label":"Sommer","targetMin":0,"skipWeekends":true}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/dayoffs", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("POST status=%v err=%v", res.StatusCode, err)
	}

	greq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/dayoffs?from=2026-06-01&to=2026-06-30", nil)
	greq.Header.Set("Authorization", "Bearer "+testToken)
	gres, _ := http.DefaultClient.Do(greq)
	if gres.StatusCode != http.StatusOK {
		t.Fatalf("GET status=%d", gres.StatusCode)
	}
}
```

> **Note on the test harness:** `server_test.go` already constructs a `Server` with a stub verifier + `testToken` for the M1a session tests. Reuse that helper (find it: `rg -n "func newTestServer|testToken" internal/adapter/httpserver/`). Extend the helper to wire the new usecases against in-memory fakes (or a testcontainers pool, matching the existing style). If the existing harness uses a real pool, point the new usecases at the same pool's stores.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestDayOffRoundTrip -v`
Expected: FAIL (handlers undefined / route 404).

- [ ] **Step 5: Write `internal/adapter/httpserver/dayoffs.go`**

```go
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

const dayFmt = "2006-01-02"

// dayOffDTO is the wire shape: target as minutes (not Duration-nanoseconds),
// date as yyyy-mm-dd, and an explicit holiday flag so the UI can style
// computed vs. manual entries.
type dayOffDTO struct {
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	TargetMin int    `json:"targetMin"`
	Holiday   bool   `json:"holiday"`
}

func toDayOffDTO(d domain.DayOff) dayOffDTO {
	return dayOffDTO{
		Day:       d.Date.Format(dayFmt),
		Kind:      string(d.Kind),
		Label:     d.Label,
		TargetMin: int(d.Target / time.Minute),
		Holiday:   d.Kind == domain.KindHoliday,
	}
}

func (s *Server) handleListDayOffs(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		http.Error(w, "from/to required (yyyy-mm-dd)", http.StatusBadRequest)
		return
	}
	list, err := s.ListDayOffs.Execute(r.Context(), u.ID, from, to)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	out := make([]dayOffDTO, 0, len(list))
	for _, d := range list {
		out = append(out, toDayOffDTO(d))
	}
	writeJSON(w, http.StatusOK, out)
}

type addDayOffReq struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Kind         string `json:"kind"`
	Label        string `json:"label"`
	TargetMin    int    `json:"targetMin"`
	SkipWeekends bool   `json:"skipWeekends"`
}

func (s *Server) handleAddDayOffs(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req addDayOffReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	from, err1 := time.ParseInLocation(dayFmt, req.From, time.Local)
	to, err2 := time.ParseInLocation(dayFmt, req.To, time.Local)
	if err1 != nil || err2 != nil {
		http.Error(w, "from/to must be yyyy-mm-dd", http.StatusBadRequest)
		return
	}
	kind, ok := domain.ParseKind(req.Kind)
	if !ok {
		http.Error(w, "invalid kind", http.StatusBadRequest)
		return
	}
	err := s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind,
		req.Label, time.Duration(req.TargetMin)*time.Minute, req.SkipWeekends)
	switch {
	case errors.Is(err, usecase.ErrHolidayNotManual) || errors.Is(err, domain.ErrInvalidDayOff):
		http.Error(w, "invalid day-off", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDeleteDayOff(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	day, err := time.ParseInLocation(dayFmt, r.PathValue("day"), time.Local)
	if err != nil {
		http.Error(w, "day must be yyyy-mm-dd", http.StatusBadRequest)
		return
	}
	if err := s.DeleteDayOff.Execute(r.Context(), u.ID, day); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseRange reads from/to query params (yyyy-mm-dd) in local time.
func parseRange(r *http.Request) (time.Time, time.Time, bool) {
	fs, ts := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	from, err1 := time.ParseInLocation(dayFmt, fs, time.Local)
	to, err2 := time.ParseInLocation(dayFmt, ts, time.Local)
	if err1 != nil || err2 != nil {
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapter/httpserver/ -run TestDayOffRoundTrip -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/dayoffs.go internal/adapter/httpserver/server.go internal/adapter/httpserver/dayoffs_test.go
git commit -m "feat(m1c): DayOff REST handlers + routes"
```

---

## Task 12: HTTP — settings + ICS-token handlers

**Files:**
- Modify: `internal/adapter/httpserver/dayoffs.go` (add settings handlers in the same file)
- Create: `internal/adapter/httpserver/settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsAndIcsToken(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// Set Bundesland.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/settings/bundesland",
		strings.NewReader(`{"bundesland":"BY"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusNoContent {
		t.Fatalf("set bundesland status=%d", res.StatusCode)
	}

	// Reject garbage.
	bad, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/settings/bundesland",
		strings.NewReader(`{"bundesland":"XX"}`))
	bad.Header.Set("Authorization", "Bearer "+testToken)
	if res, _ := http.DefaultClient.Do(bad); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage bundesland status=%d", res.StatusCode)
	}

	// Regenerate token returns a non-empty secret.
	rreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/ics-token/regenerate", nil)
	rreq.Header.Set("Authorization", "Bearer "+testToken)
	rres, _ := http.DefaultClient.Do(rreq)
	if rres.StatusCode != http.StatusOK {
		t.Fatalf("regen status=%d", rres.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestSettingsAndIcsToken -v`
Expected: FAIL (handlers undefined).

- [ ] **Step 3: Add the handlers to `dayoffs.go`**

```go
type settingsDTO struct {
	Bundesland string   `json:"bundesland"`
	FeedURLs   []string `json:"feedUrls"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	set, toks, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	urls := make([]string, 0, len(toks))
	for _, t := range toks {
		urls = append(urls, icsURL(r, t.Token))
	}
	writeJSON(w, http.StatusOK, settingsDTO{Bundesland: set.Bundesland, FeedURLs: urls})
}

type setBundeslandReq struct {
	Bundesland string `json:"bundesland"`
}

func (s *Server) handleSetBundesland(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setBundeslandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.SetBundesland.Execute(r.Context(), u.ID, req.Bundesland); err != nil {
		http.Error(w, "invalid bundesland", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type tokenDTO struct {
	Token   string `json:"token"`
	FeedURL string `json:"feedUrl"`
}

func (s *Server) handleRegenIcsToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tok, err := s.RegenIcsToken.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tokenDTO{Token: tok, FeedURL: icsURL(r, tok)})
}

// icsURL builds the absolute feed URL from the request host. Honors
// X-Forwarded-Proto behind the homelab ingress; defaults to https when set.
func icsURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/ics/" + token + ".ics"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/httpserver/ -run TestSettingsAndIcsToken -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver/dayoffs.go internal/adapter/httpserver/settings_test.go
git commit -m "feat(m1c): settings + ICS-token regenerate handlers"
```

---

## Task 13: HTTP — ICS feed route (token-by-URL auth)

**Files:**
- Create: `internal/adapter/httpserver/icsfeed.go`
- Create: `internal/adapter/httpserver/icsfeed_test.go`

- [ ] **Step 1: Write the failing test**

```go
package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIcsFeed_UnknownToken404(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/ics/does-not-exist.ics")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token: want 404, got %d", res.StatusCode)
	}
}

func TestIcsFeed_ValidTokenServesCalendar(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// Mint a token via the authenticated regenerate endpoint, then fetch the feed.
	rreq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/ics-token/regenerate", nil)
	rreq.Header.Set("Authorization", "Bearer "+testToken)
	rres, _ := http.DefaultClient.Do(rreq)
	if rres.StatusCode != http.StatusOK {
		t.Fatalf("regen status=%d", rres.StatusCode)
	}
	var body struct {
		Token   string `json:"token"`
		FeedURL string `json:"feedUrl"`
	}
	decodeJSON(t, rres, &body) // small helper in server_test.go; or inline json.NewDecoder

	res, err := http.Get(ts.URL + "/ics/" + body.Token + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("valid token: want 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("content-type = %q", ct)
	}
}
```

> If `decodeJSON` doesn't exist, inline `json.NewDecoder(rres.Body).Decode(&body)`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestIcsFeed -v`
Expected: FAIL (`handleIcsFeed` undefined or route missing — it was referenced in server.go Task 11; implement it now).

- [ ] **Step 3: Write `internal/adapter/httpserver/icsfeed.go`**

```go
package httpserver

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/ports"
)

// handleIcsFeed is the third auth path: a secret token in the URL path, no
// OIDC/cookie. Unknown or revoked tokens return 404 (no existence leak).
func (s *Server) handleIcsFeed(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var buf bytes.Buffer
	err := s.IcsFeed.Execute(r.Context(), token, &buf)
	if errors.Is(err, ports.ErrFeedTokenNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(buf.Bytes())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/httpserver/ -run TestIcsFeed -v`
Expected: PASS (404 for unknown, 200 `text/calendar` for valid).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpserver/icsfeed.go internal/adapter/httpserver/icsfeed_test.go
git commit -m "feat(m1c): ICS feed route with token-by-URL auth"
```

---

## Task 14: apiclient — DayOff/settings methods

**Files:**
- Create: `internal/adapter/apiclient/dayoffs.go`
- Create: `internal/adapter/apiclient/dayoffs_test.go`

- [ ] **Step 1: Write the failing test** (httptest server returning canned JSON, mirroring `client_test.go`)

```go
package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestClient_AddAndListDayOffs(t *testing.T) {
	var gotPost bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/dayoffs":
			gotPost = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/dayoffs":
			_, _ = w.Write([]byte(`[{"day":"2026-06-15","kind":"vacation","label":"Sommer","targetMin":0,"holiday":false}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	if err := c.AddDayOffs(context.Background(), "2026-06-15", "2026-06-19", "vacation", "Sommer", 0, true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !gotPost {
		t.Fatal("POST not issued")
	}
	list, err := c.ListDayOffs(context.Background(), "2026-06-01", "2026-06-30")
	if err != nil || len(list) != 1 || list[0].Label != "Sommer" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/apiclient/ -run TestClient_AddAndListDayOffs -v`
Expected: FAIL (methods + `DayOff` DTO type undefined).

- [ ] **Step 3: Write `internal/adapter/apiclient/dayoffs.go`**

```go
package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DayOff is the client-side view of a merged day-off (manual or computed
// holiday). Mirrors the server's dayOffDTO wire shape.
type DayOff struct {
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	TargetMin int    `json:"targetMin"`
	Holiday   bool   `json:"holiday"`
}

func (c *Client) AddDayOffs(ctx context.Context, from, to, kind, label string, targetMin int, skipWeekends bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/dayoffs", map[string]any{
		"from": from, "to": to, "kind": kind, "label": label,
		"targetMin": targetMin, "skipWeekends": skipWeekends,
	}, nil)
}

func (c *Client) ListDayOffs(ctx context.Context, from, to string) ([]DayOff, error) {
	q := url.Values{"from": {from}, "to": {to}}
	var out []DayOff
	err := c.do(ctx, http.MethodGet, "/api/v1/dayoffs?"+q.Encode(), nil, &out)
	return out, err
}

func (c *Client) DeleteDayOff(ctx context.Context, day string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/dayoffs/"+day, nil, nil)
}

// Settings mirrors the server settingsDTO.
type Settings struct {
	Bundesland string   `json:"bundesland"`
	FeedURLs   []string `json:"feedUrls"`
}

func (c *Client) GetSettings(ctx context.Context) (Settings, error) {
	var s Settings
	err := c.do(ctx, http.MethodGet, "/api/v1/settings", nil, &s)
	return s, err
}

func (c *Client) SetBundesland(ctx context.Context, land string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/settings/bundesland", map[string]any{"bundesland": land}, nil)
}

// RegenIcsToken mints a new feed token and returns its absolute URL.
func (c *Client) RegenIcsToken(ctx context.Context) (string, error) {
	var out struct {
		Token   string `json:"token"`
		FeedURL string `json:"feedUrl"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/ics-token/regenerate", nil, &out)
	return out.FeedURL, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/apiclient/ -run TestClient_AddAndListDayOffs -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/dayoffs.go internal/adapter/apiclient/dayoffs_test.go
git commit -m "feat(m1c): apiclient DayOff + settings methods"
```

---

## Task 15: CLI — `flow dayoff` verbs

**Files:**
- Create: `cmd/flow/dayoff.go`
- Modify: `cmd/flow/main.go` (register `dayoffCmd()`)

Look at `cmd/flow/worktime.go` for how a verb builds its client (`clientFromStore` / `clientconfig`). Mirror it.

- [ ] **Step 1: Read the existing CLI client wiring**

Run: `rg -n "clientFrom|apiclient.New|clientconfig" cmd/flow/worktime.go cmd/flow/whoami.go`

- [ ] **Step 2: Write `cmd/flow/dayoff.go`** (mirror the client-construction pattern you just read; the body below assumes a `mustClient(cmd)` helper like worktime.go uses — adapt to the actual helper name)

```go
package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func dayoffCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "dayoff", Short: "manage days off (vacation/sick) + holidays"}
	cmd.AddCommand(dayoffListCmd(), dayoffAddCmd(), dayoffRmCmd())
	return cmd
}

func dayoffListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list day-offs for the current year (manual + holidays)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromCmd(cmd) // same helper worktime.go uses
			if err != nil {
				return err
			}
			year := time.Now().Year()
			list, err := c.ListDayOffs(cmd.Context(),
				fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
			if err != nil {
				return err
			}
			for _, d := range list {
				tag := d.Kind
				if d.Holiday {
					tag = "holiday"
				}
				fmt.Printf("%s  %-8s %s\n", d.Day, tag, d.Label)
			}
			return nil
		},
	}
}

func dayoffAddCmd() *cobra.Command {
	var kind, label string
	var targetMin int
	var skipWeekends bool
	cmd := &cobra.Command{
		Use:   "add <from> <to>",
		Short: "add a day-off range (yyyy-mm-dd yyyy-mm-dd)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			return c.AddDayOffs(cmd.Context(), args[0], args[1], kind, label, targetMin, skipWeekends)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "vacation", "vacation|sick")
	cmd.Flags().StringVar(&label, "label", "", "optional label")
	cmd.Flags().IntVar(&targetMin, "target-min", 0, "half-day target in minutes (0 = full day off)")
	cmd.Flags().BoolVar(&skipWeekends, "skip-weekends", true, "skip Sat/Sun")
	return cmd
}

func dayoffRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <day>",
		Short: "remove a day-off (yyyy-mm-dd)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			return c.DeleteDayOff(cmd.Context(), args[0])
		},
	}
}
```

> **Adapt:** Replace `clientFromCmd(cmd)` with whatever helper `worktime.go` actually uses to build an `*apiclient.Client` from the stored token (Step 1 reveals the name). If no shared helper exists, extract one rather than copy-pasting client construction.

- [ ] **Step 3: Register in `cmd/flow/main.go`**

```go
	root.AddCommand(dayoffCmd())
```

- [ ] **Step 4: Build + smoke**

Run: `go build ./... && go run ./cmd/flow dayoff --help`
Expected: build OK, help lists `list`, `add`, `rm`.

- [ ] **Step 5: Commit**

```bash
git add cmd/flow/dayoff.go cmd/flow/main.go
git commit -m "feat(m1c): flow dayoff CLI verbs (list/add/rm)"
```

---

## Task 16: TUI — DayOff sub-view

**Files:**
- Create: `internal/tui/dayoffs.go`
- Modify: `internal/tui/worktime.go` (tab toggle + delegation)
- Create: `internal/tui/dayoffs_test.go`

The M1a `Model` (`internal/tui/worktime.go`) is the whole app shell with one screen. Add a second view (DayOffs) reachable with a key, sharing the same SSE stream. Keep `worktime.go` focused — put the DayOff state + rendering in `dayoffs.go`.

- [ ] **Step 1: Read the current model to find integration points**

Run: `rg -n "handleKey|reload|eventMsg|func .Model. View|m.booking" internal/tui/worktime.go`

- [ ] **Step 2: Write the failing test** (`internal/tui/dayoffs_test.go`)

```go
package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModel_TogglesDayOffView(t *testing.T) {
	m := New(nil, "msoent")
	// 'd' switches to the dayoff view.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "d"})
	if !updated.(Model).showDayOffs {
		t.Fatal("expected dayoff view active after 'd'")
	}
	// 'esc' returns to worktime.
	back, _ := updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(Model).showDayOffs {
		t.Fatal("expected worktime view after esc")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_TogglesDayOffView -v`
Expected: FAIL (`showDayOffs` field undefined).

- [ ] **Step 4: Add the field + key handling**

In `internal/tui/worktime.go`, add to the `Model` struct:

```go
	showDayOffs bool
	dayoffs     []apiclient.DayOff
```

In `handleKey` (when **not** booking), add a branch before the final `return`:

```go
	case k.Text == "d":
		m.showDayOffs = true
		return m, m.reloadDayOffs()
	case k.Code == tea.KeyEsc && m.showDayOffs:
		m.showDayOffs = false
		return m, nil
```

In `View()`, at the top of the body (after the header), delegate when active:

```go
	if m.showDayOffs {
		return m.dayOffView()
	}
```

In the `eventMsg` case of `Update`, also refresh dayoffs so the live-sync covers them:

```go
	case eventMsg:
		return m, tea.Batch(m.reload(), m.reloadDayOffs(), waitForEvent(m.events))
```

Add a `dayoffsLoadedMsg` case in `Update`:

```go
	case dayoffsLoadedMsg:
		m.dayoffs = msg.list
		return m, nil
```

- [ ] **Step 5: Write `internal/tui/dayoffs.go`**

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

type dayoffsLoadedMsg struct{ list []apiclient.DayOff }

func (m Model) reloadDayOffs() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		year := time.Now().Year()
		list, err := m.client.ListDayOffs(ctx,
			fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
		if err != nil {
			return errMsg{err}
		}
		return dayoffsLoadedMsg{list: list}
	}
}

func (m Model) dayOffView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · dayoffs") + styleMuted.Render("  "+m.user) + "\n\n")
	if len(m.dayoffs) == 0 {
		b.WriteString(styleMuted.Render("  no day-offs this year") + "\n")
	}
	for _, d := range m.dayoffs {
		glyph := dayOffGlyph(d.Kind, d.Holiday)
		label := d.Label
		if label == "" {
			label = d.Kind
		}
		fmt.Fprintf(&b, "  %s %s  %s\n", glyph, d.Day, label)
	}
	b.WriteString("\n" + styleMuted.Render("esc back · (add/remove via WebUI or `flow dayoff`)") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// dayOffGlyph mirrors the v1 Dayoff-Glyph-Unification: one ○ marker, kind via
// position. Holidays render dimmed.
func dayOffGlyph(kind string, holiday bool) string {
	if holiday {
		return styleMuted.Render("○")
	}
	return "○"
}
```

> **Scope note:** M1c TUI is **read + navigate** plus the existing worktime actions; DayOff *creation/deletion* in the TUI can be a thin follow-up if time allows, but the done-gate (WebUI→TUI live-sync) only requires the TUI to *display* live updates. Adding TUI add/rm dialogs here is optional and may be deferred to a follow-up commit — note it in the PR if skipped. Settings (Bundesland/token) likewise: read-only display is enough for the gate.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/tui/ -run TestModel_TogglesDayOffView -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/dayoffs.go internal/tui/worktime.go internal/tui/dayoffs_test.go
git commit -m "feat(m1c): TUI dayoff view, live-synced via SSE"
```

---

## Task 17: WebUI — DayOff page + handlers

**Files:**
- Create: `internal/adapter/webui/dayoffs.templ`
- Create: `internal/adapter/httpserver/webui_dayoffs.go`
- Modify: `internal/adapter/httpserver/server.go` (web routes)

Mirror `worktime.templ` + `webui.go`. The page subscribes to SSE and re-fetches its fragment on `sse:dayoff.changed`.

- [ ] **Step 1: Write `internal/adapter/webui/dayoffs.templ`**

```go
package webui

import "github.com/serverkraken/flow/internal/adapter/apiclient"

// DayOffData is the view model for the dayoff screen.
type DayOffData struct {
	User       string
	Bundesland string
	FeedURL    string
	DayOffs    []apiclient.DayOff
}

templ DayOffPage(d DayOffData) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>flow · dayoffs</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<script src="https://unpkg.com/htmx.org@2.0.4"></script>
			<script src="https://unpkg.com/htmx-ext-sse@2.2.3"></script>
		</head>
		<body class="bg-slate-50 text-slate-800" hx-ext="sse" sse-connect="/api/v1/events">
			<main class="mx-auto max-w-md p-4">
				<div id="do" hx-get="/ui/dayoffs" hx-trigger="sse:dayoff.changed" hx-swap="innerHTML">
					@DayOffFragment(d)
				</div>
			</main>
		</body>
	</html>
}

templ DayOffFragment(d DayOffData) {
	<header class="mb-4 flex items-center justify-between">
		<h1 class="text-lg font-semibold text-slate-900">flow · dayoffs</h1>
		<a href="/" class="text-sm text-slate-500">← worktime</a>
	</header>
	<form hx-post="/ui/dayoffs/add" hx-target="#do" hx-swap="innerHTML" class="mb-4 grid grid-cols-2 gap-2">
		<input type="date" name="from" required class="rounded border px-2 py-1 text-sm"/>
		<input type="date" name="to" required class="rounded border px-2 py-1 text-sm"/>
		<select name="kind" class="rounded border px-2 py-1 text-sm">
			<option value="vacation">Urlaub</option>
			<option value="sick">Krank</option>
		</select>
		<input name="label" placeholder="label (optional)" class="rounded border px-2 py-1 text-sm"/>
		<label class="col-span-2 flex items-center gap-2 text-sm text-slate-500">
			<input type="checkbox" name="skipWeekends" value="true" checked/> skip weekends
		</label>
		<button class="col-span-2 rounded bg-slate-900 px-3 py-1 text-sm text-white">add</button>
	</form>
	<section>
		<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">{ d.Bundesland } · this year</h2>
		<ul class="divide-y divide-slate-100">
			for _, o := range d.DayOffs {
				<li class="flex items-center justify-between py-2 text-sm">
					<span>
						{ o.Day }
						<span class="ml-2 text-slate-500">
							if o.Holiday {
								{ o.Label } (Feiertag)
							} else {
								{ o.Kind } { o.Label }
							}
						</span>
					</span>
					if !o.Holiday {
						<form hx-post="/ui/dayoffs/delete" hx-target="#do" hx-swap="innerHTML">
							<input type="hidden" name="day" value={ o.Day }/>
							<button class="text-slate-400 hover:text-red-500">✕</button>
						</form>
					}
				</li>
			}
		</ul>
	</section>
	<section class="mt-6 rounded bg-slate-100 p-3 text-sm">
		<div class="mb-1 font-medium text-slate-700">Calendar feed</div>
		<code class="block break-all text-xs text-slate-500">{ d.FeedURL }</code>
		<form hx-post="/ui/dayoffs/regen-token" hx-target="#do" hx-swap="innerHTML" class="mt-2">
			<button class="text-xs text-slate-500 underline">regenerate token</button>
		</form>
	</section>
}
```

- [ ] **Step 2: Generate templ Go** (the build uses generated `*_templ.go` — run the project's templ generator)

Run: `go tool templ generate ./internal/adapter/webui/` (or the command in the Makefile — check `rg -n templ Makefile`)
Expected: `internal/adapter/webui/dayoffs_templ.go` created.

- [ ] **Step 3: Write `internal/adapter/httpserver/webui_dayoffs.go`**

```go
package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) dayOffData(ctx context.Context, u domain.User) (webui.DayOffData, error) {
	year := s.Clock.Now().Year()
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(year, 12, 31, 0, 0, 0, 0, time.Local)
	list, err := s.ListDayOffs.Execute(ctx, u.ID, from, to)
	if err != nil {
		return webui.DayOffData{}, err
	}
	dtos := make([]apiclient.DayOff, 0, len(list))
	for _, d := range list {
		dtos = append(dtos, apiclient.DayOff{
			Day: d.Date.Format("2006-01-02"), Kind: string(d.Kind),
			Label: d.Label, TargetMin: int(d.Target / time.Minute),
			Holiday: d.Kind == domain.KindHoliday,
		})
	}
	set, toks, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.DayOffData{}, err
	}
	return webui.DayOffData{User: u.Username, Bundesland: set.Bundesland, FeedURL: firstFeedURL(toks), DayOffs: dtos}, nil
}

func firstFeedURL(toks []domain.FeedToken) string {
	if len(toks) == 0 {
		return "(none — regenerate below)"
	}
	return "/ics/" + toks[0].Token + ".ics"
}

func (s *Server) renderDayOffFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	d, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DayOffFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DayOffPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	kind, ok := domain.ParseKind(r.FormValue("kind"))
	if ok {
		from, err1 := time.ParseInLocation("2006-01-02", r.FormValue("from"), time.Local)
		to, err2 := time.ParseInLocation("2006-01-02", r.FormValue("to"), time.Local)
		if err1 == nil && err2 == nil {
			_ = s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind, r.FormValue("label"), 0, r.FormValue("skipWeekends") == "true")
		}
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	if day, err := time.ParseInLocation("2006-01-02", r.FormValue("day"), time.Local); err == nil {
		_ = s.DeleteDayOff.Execute(r.Context(), u.ID, day)
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebRegenToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_, _ = s.RegenIcsToken.Execute(r.Context(), u.ID)
	s.renderDayOffFragment(w, r, u)
}
```

- [ ] **Step 4: Add the web routes to `server.go`** (after the worktime web routes)

```go
	mux.Handle("GET /dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffHome)))
	mux.Handle("GET /ui/dayoffs", s.webAuth(http.HandlerFunc(s.handleWebDayOffFragment)))
	mux.Handle("POST /ui/dayoffs/add", s.webAuth(http.HandlerFunc(s.handleWebDayOffAdd)))
	mux.Handle("POST /ui/dayoffs/delete", s.webAuth(http.HandlerFunc(s.handleWebDayOffDelete)))
	mux.Handle("POST /ui/dayoffs/regen-token", s.webAuth(http.HandlerFunc(s.handleWebRegenToken)))
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/dayoffs.templ internal/adapter/webui/dayoffs_templ.go internal/adapter/httpserver/webui_dayoffs.go internal/adapter/httpserver/server.go
git commit -m "feat(m1c): WebUI dayoff page + handlers (HTMX-SSE live)"
```

---

## Task 18: Wiring + smoke (composition root)

**Files:**
- Modify: `cmd/flow-server/main.go`

This is the mandatory wiring-verification task (Lesson: "Plans need a main-wiring task"). Every new store/usecase/handler must be constructed in the composition root, and every new route must answer a curl.

- [ ] **Step 1: Wire the stores + usecases in `cmd/flow-server/main.go`**

After the existing store constructors, add:

```go
	dayOffStore := pgstore.NewDayOffStore(pool)
	settingsStore := pgstore.NewUserSettingsStore(pool)
	feedTokenStore := pgstore.NewFeedTokenStore(pool)
```

In the `srv := &httpserver.Server{...}` literal, add (alongside the worktime usecases):

```go
		AddDayOffs:    usecase.AddDayOffs{Store: dayOffStore, Bus: bus},
		DeleteDayOff:  usecase.DeleteDayOff{Store: dayOffStore, Bus: bus},
		ListDayOffs:   usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
		GetSettings:   usecase.GetSettings{Settings: settingsStore, Tokens: feedTokenStore},
		SetBundesland: usecase.SetBundesland{Settings: settingsStore},
		IcsFeed:       usecase.IcsFeed{Tokens: feedTokenStore, Store: dayOffStore, Clock: clock},
		RegenIcsToken: usecase.RegenerateIcsToken{Tokens: feedTokenStore, Clock: clock},
```

> `bus` is currently created inline as `Bus: sse.NewBus()`. Hoist it to a local first so the dayoff usecases share the same bus:
> ```go
> bus := sse.NewBus()
> ```
> then set `Bus: bus,` in the struct literal.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: build OK, no "declared and not used".

- [ ] **Step 3: Run the full test suite**

Run: `make ci`
Expected: green (lint + vet + all tests, incl. the new pgstore testcontainers tests).

- [ ] **Step 4: Manual curl smoke** (dev env from memory `reference_flow_dev_env`)

```bash
make dev-up                          # Postgres + Dex
TOKEN=$(make -s dev-token)           # bearer for the dev sub
make dev-run &                       # flow-server on its dev port (note the addr it logs)
BASE=http://localhost:8080           # adjust to the logged ListenAddr

# DayOffs
curl -s -XPOST $BASE/api/v1/dayoffs -H "Authorization: Bearer $TOKEN" \
  -d '{"from":"2026-06-15","to":"2026-06-19","kind":"vacation","label":"Sommer","skipWeekends":true}' -i | head -1
curl -s "$BASE/api/v1/dayoffs?from=2026-06-01&to=2026-06-30" -H "Authorization: Bearer $TOKEN" | head
curl -s -XDELETE $BASE/api/v1/dayoffs/2026-06-15 -H "Authorization: Bearer $TOKEN" -i | head -1

# Settings + token
curl -s -XPOST $BASE/api/v1/settings/bundesland -H "Authorization: Bearer $TOKEN" -d '{"bundesland":"NW"}' -i | head -1
curl -s $BASE/api/v1/settings -H "Authorization: Bearer $TOKEN"
FEED=$(curl -s -XPOST $BASE/api/v1/ics-token/regenerate -H "Authorization: Bearer $TOKEN" | python3 -c 'import sys,json;print(json.load(sys.stdin)["feedUrl"])')
echo "$FEED"

# ICS feed (no auth) — valid token serves text/calendar, garbage 404
curl -s "$FEED" -i | head -5
curl -s "$BASE/ics/garbage.ics" -i | head -1   # expect 404
```

Expected: `201`, JSON list, `204`, `204`, settings JSON, a feed URL, `200 text/calendar` for the real feed, `404` for garbage.

- [ ] **Step 5: Manual done-gate** (cross-surface live-sync)

1. `make dev-run`, open the WebUI (`/`), then open `/dayoffs`.
2. In the TUI (`go run ./cmd/flow` worktime → press `d`), confirm the dayoff view is visible.
3. Add a vacation week in the WebUI → confirm it appears in the TUI within ~1 s (SSE `dayoff.changed`).
4. Subscribe the printed feed URL in a real calendar (Apple/Google) → the vacation entries appear; holidays do **not** (by design).

- [ ] **Step 6: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "chore(m1c): wire DayOff/settings/ICS stores + usecases into composition root"
```

---

## Self-Review (gegen die Spec)

**Spec coverage:**
- DayOff manual (vacation/sick) + Range→Tageszeilen → Task 4, 6, 9, 11. ✓
- Halbtags `target_min` → Task 6 (DDL), 9/11 (carried through). ✓
- Feiertage berechnet + merge → Task 2, 9 (`ListDayOffs`). ✓
- Bundesland per-User, Default NW → Task 7, 10, 12. ✓
- ICS abonnierbarer Feed + Token-by-URL + revoke → Task 8, 10, 13. ✓ (404 für unbekannt/revoked.)
- ICS-Inhalt ohne Feiertage → Task 10 (`IcsFeed`), Task 13-Test asserts kein "Neujahr". ✓
- `user_settings` + `feed_tokens` (Variante C) → Task 6 DDL, 7, 8. ✓
- TUI + WebUI live-synced → Task 16, 17 (`dayoff.changed`). ✓
- Wiring-Verification-Pflichttask → Task 18. ✓
- CLI-Verben → Task 15. ✓

**Type consistency:** `dayOffDTO`/`apiclient.DayOff`/`webui` view-model use identical field names (Day/Kind/Label/TargetMin/Holiday). Ports `DayOffStore.Add(ctx, ownerID, DayOff)`, `Delete(ctx, ownerID, day)`, `ListRange(ctx, ownerID, from, to)` match pgstore + usecase callsites. `FeedTokenStore.Revoke(ctx, userID, token)` matches usecase + handler. Event const `EventDayOffChanged = "dayoff.changed"` matches templ `sse:dayoff.changed`. ✓

**Open adaptation points (flagged inline, not placeholders):**
- Task 11/13/16/15 reference existing test/CLI helpers (`newTestServer`/`testToken`/`clientFromCmd`) whose exact names must be read from the current code — each step says to `rg` for the real name and adapt. These are integration seams, not unspecified logic.
- Task 16 deliberately scopes the TUI to display + navigate (done-gate only needs live display); TUI add/rm dialogs are an explicit optional follow-up.
