# flow Rebuild M1d — Stats + Burndown + Tagesziel · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Server-berechnete Worktime-Auswertung — Tagesziel-Config (Default + per-Weekday-Overrides), Monats-Burndown, Today-Saldo, Wochen-Sicht und Range-Stats (KW/Monat) — in Server + TUI + WebUI, live-synced.

**Architecture:** Server-authoritative; die reinen Auswertungs-Funktionen werden aus `main` übernommen (`Aggregate`/`AggregateRange`/`MonthBurndownCompute`/`Stats`), die Datenquelle ist Postgres statt File. Ein neuer `StatsComputer`-Usecase baut pro Request einen reinen `TargetResolver` (aus `user_settings` + gemergten DayOffs/Feiertagen aus M1c) und liefert `Stats`/`MonthBurndownReport`/Today/Week als JSON-DTO. Clients rendern nur, keine Aggregations-Logik clientseitig. Stats sind abgeleitet → Re-Fetch auf bestehende `session.*`/`dayoff.changed` plus neues `settings.changed`-Event.

**Tech Stack:** Go, `pgx/v5`, goose (embedded migrations), `net/http` (std mux), Bubbletea v2 / Lipgloss v2 (`charm.land/v2`), templ + HTMX (WebUI), testcontainers (pgstore-Tests).

---

## Worktree & Branch

**Alle Code-Tasks laufen im bestehenden Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild` auf dem Orphan-Branch `rebuild`** (HEAD aktuell `9b076f6`). Der Plan-/Spec-Doc lebt auf `main` (M0/M1a/M1c-Präzedenz) — **nicht** mit ins `rebuild` committen. Modulpfad: `github.com/serverkraken/flow`. Carry-over-Quelldateien liegen im selben Repo unter `main:` und sind via `git show main:<pfad>` lesbar (rebuild ist ein Orphan-Branch desselben Repos).

**Commit-Konvention:** kleine, fokussierte Commits pro Task; Message-Stil wie M1c (`feat(stats): …`, `feat(settings): …`). Kein `make ci` zwischen jedem Step, aber am Ende jedes Tasks `go test ./...` der betroffenen Pakete grün; finaler Task fährt `make ci`.

## Datenquellen-Kontext (aus M1a/M1c, unverändert)

- `ports.SessionStore.List(ctx, ownerID, since) ([]domain.WorkSession, error)` — Sessions mit `start_at >= since`, newest first. `domain.WorkSession{ID,OwnerID,ProjectID,Tag,Note,Start time.Time,Stop *time.Time,CreatedAt}`; `Stop == nil` = laufend; `Elapsed(now) time.Duration` ist eine **Methode**.
- `ports.SessionStore.Running(ctx, ownerID) (domain.WorkSession, bool, error)` — der laufende Timer.
- `usecase.ListDayOffs{Store, Settings, Loc}.Execute(ctx, ownerID, from, to) ([]domain.DayOff, error)` — **gemergte** manuelle DayOffs ∪ berechnete Feiertage (Bundesland aus Settings). `domain.DayOff{Date time.Time, Kind, Label, Target time.Duration}`; `Kind == domain.KindHoliday` für Feiertage.
- `ports.UserSettingsStore.Get(ctx, userID) (domain.Settings, error)` — lazy Default (`Bundesland "NW"`) ohne Row.
- `ports.EventBus.Publish(domain.Event{Type, UserID, Data})` — Events werden in den **HTTP-Handlern** publiziert (Muster: `internal/adapter/httpserver/worktime.go` `handleStartSession`).
- Events: `domain.EventType` in `internal/domain/event.go` (`session.started/stopped/updated`, `project.created`, `dayoff.changed`).
- Migrationen: `internal/adapter/pgstore/migrations/NNNN_*.sql`, goose-Format (`-- +goose Up` / `-- +goose Down`), embedded; `pgstore.Migrate(ctx, pool)` läuft sie.

---

## File Structure

**Neu (Domain, rein):**
- `internal/domain/stats.go` — `Stats`, `TagDur`, `TopTags` (Carry-over aus `main:internal/domain/stats.go`, verbatim).
- `internal/domain/burndown.go` — `MonthBurndownReport` (Carry-over `main:internal/domain/burndown.go`, verbatim).
- `internal/domain/dayrecord.go` — `RecordSession`, `DayRecord`, `WeekDay` (aus `main:internal/domain/day.go`, **Session→RecordSession** adaptiert, Pause/`Day` weggelassen).
- `internal/domain/aggregate.go` — `Aggregate`, `AggregateRange`, `PlannedTarget`, `MonthBurndownCompute`, `FilterRecords` + Helfer (Carry-over `main:internal/domain/aggregate.go`, **ohne** `ReportRange`/`BriefBounds`, `DayRecord.Sessions`→`[]RecordSession`).
- `internal/domain/workcalendar.go` — `IsWorkday`, `isWeekend`, `isoMonday` (aus `main:internal/domain/calendar.go`, nur diese drei).
- `internal/domain/records.go` — `BuildDayRecords(sessions []WorkSession, now time.Time, targetFor func(time.Time) time.Duration) []DayRecord` (neu, rein).

**Neu (Usecase):**
- `internal/usecase/target_resolver.go` — `TargetResolver` (reines Value-Object: `Default`, `Weekday [7]*time.Duration`, `DayOffs map[string]domain.DayOff`).
- `internal/usecase/stats_computer.go` — `StatsComputer` mit `Today`/`Week`/`RangeStats`/`Burndown`.
- `internal/usecase/set_target.go` — `SetTargetConfig` (Validierung + Store).

**Geändert:**
- `internal/domain/settings.go` — `Settings` um `DefaultTargetMin int` + `WeekdayTargetMin map[time.Weekday]int` erweitern.
- `internal/domain/event.go` — `EventSettingsChanged EventType = "settings.changed"`.
- `internal/ports/ports.go` — `UserSettingsStore` um `SetTargetConfig` erweitern; `Get` liefert die neuen Felder mit.
- `internal/adapter/pgstore/migrations/0004_target_config.sql` — neu.
- `internal/adapter/pgstore/user_settings.go` — `Get` + neuer `SetTargetConfig`.
- `internal/adapter/httpserver/stats.go` — neu (DTOs + Handler Today/Week/Stats/Burndown).
- `internal/adapter/httpserver/dayoffs.go` — `settingsDTO` um Target-Felder erweitern; `handleSetTarget`.
- `internal/adapter/httpserver/server.go` — `Server`-Felder + Routen.
- `internal/adapter/apiclient/stats.go` — neu (Client-Methoden + DTOs).
- `internal/adapter/apiclient/dayoffs.go` — `Settings`-DTO erweitern, `SetTargetConfig`.
- `internal/tui/worktime.go` — Today/Burndown-Header, `reloadStats`, Keys `w`/`t`.
- `internal/tui/stats.go` — neu (Week- + Stats-Sicht + Settings-Target-Editor-Render).
- `internal/adapter/webui/stats.templ` (+ generated `_templ.go`) — neu.
- `internal/adapter/httpserver/webui_stats.go` — neu (WebUI-Fragmente).
- `cmd/flow-server/main.go` — Wiring.

---

## Task 1: Settings-Migration + Domain-Felder + Store

**Files:**
- Create: `internal/adapter/pgstore/migrations/0004_target_config.sql`
- Modify: `internal/domain/settings.go`
- Modify: `internal/ports/ports.go:90-93` (`UserSettingsStore`)
- Modify: `internal/adapter/pgstore/user_settings.go`
- Test: `internal/adapter/pgstore/user_settings_test.go`

- [ ] **Step 1: Migration schreiben**

Create `internal/adapter/pgstore/migrations/0004_target_config.sql`:

```sql
-- +goose Up
ALTER TABLE user_settings
    ADD COLUMN default_target_min INT NOT NULL DEFAULT 480,
    ADD COLUMN target_sun_min INT,
    ADD COLUMN target_mon_min INT,
    ADD COLUMN target_tue_min INT,
    ADD COLUMN target_wed_min INT,
    ADD COLUMN target_thu_min INT,
    ADD COLUMN target_fri_min INT,
    ADD COLUMN target_sat_min INT;

-- +goose Down
ALTER TABLE user_settings
    DROP COLUMN default_target_min,
    DROP COLUMN target_sun_min,
    DROP COLUMN target_mon_min,
    DROP COLUMN target_tue_min,
    DROP COLUMN target_wed_min,
    DROP COLUMN target_thu_min,
    DROP COLUMN target_fri_min,
    DROP COLUMN target_sat_min;
```

Spalten-Reihenfolge Sun..Sat entspricht `int(time.Weekday)` (Sunday=0 … Saturday=6) — kein Mapping-Drift.

- [ ] **Step 2: `domain.Settings` erweitern**

In `internal/domain/settings.go`, `Settings` ersetzen durch:

```go
// Settings holds per-user preferences. Bundesland drives the computed
// German-holiday set; DefaultTargetMin + WeekdayTargetMin drive the daily
// work target (M1d). WeekdayTargetMin keys are time.Weekday; a present
// entry (incl. 0) is an explicit override, absence means "use default".
type Settings struct {
	UserID           string               `json:"-"`
	Bundesland       string               `json:"bundesland"`
	DefaultTargetMin int                  `json:"defaultTargetMin"`
	WeekdayTargetMin map[time.Weekday]int `json:"-"`
}

// DefaultDailyTargetMin is the fallback daily target (8h) when a user never
// configured one.
const DefaultDailyTargetMin = 480
```

`time` ist bereits importiert.

- [ ] **Step 3: Port erweitern**

In `internal/ports/ports.go`, `UserSettingsStore` ersetzen durch:

```go
// UserSettingsStore persists per-user preferences. Get lazily returns a
// default row (Bundesland "NW", DefaultTargetMin 480, no weekday overrides)
// for users that never saved settings. SetTargetConfig replaces the daily
// target config wholesale (default + the full override set).
type UserSettingsStore interface {
	Get(ctx context.Context, userID string) (domain.Settings, error)
	SetBundesland(ctx context.Context, userID, land string) error
	SetTargetConfig(ctx context.Context, userID string, defaultMin int, weekday map[time.Weekday]int) error
}
```

- [ ] **Step 4: Store-Test schreiben (failing)**

In `internal/adapter/pgstore/user_settings_test.go` ergänzen (Muster wie bestehende Tests in der Datei — `newTestPool(t)` o.ä.; vorhandenes Helper-Setup wiederverwenden):

```go
func TestUserSettings_TargetConfigRoundTrip(t *testing.T) {
	pool := newTestPool(t) // existing helper
	st := pgstore.NewUserSettingsStore(pool)
	ctx := context.Background()
	uid := seedUser(t, pool) // existing helper that inserts a users row

	// lazy default before any write
	got, err := st.Get(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTargetMin != 480 || len(got.WeekdayTargetMin) != 0 {
		t.Fatalf("lazy default: got default=%d weekday=%v", got.DefaultTargetMin, got.WeekdayTargetMin)
	}

	// explicit 0 on Saturday must survive (presence semantics)
	if err := st.SetTargetConfig(ctx, uid, 420, map[time.Weekday]int{time.Friday: 360, time.Saturday: 0}); err != nil {
		t.Fatal(err)
	}
	got, err = st.Get(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTargetMin != 420 {
		t.Errorf("default: got %d want 420", got.DefaultTargetMin)
	}
	if v, ok := got.WeekdayTargetMin[time.Friday]; !ok || v != 360 {
		t.Errorf("friday override: got %d ok=%v want 360", v, ok)
	}
	if v, ok := got.WeekdayTargetMin[time.Saturday]; !ok || v != 0 {
		t.Errorf("saturday explicit-0 override lost: got %d ok=%v", v, ok)
	}
	if _, ok := got.WeekdayTargetMin[time.Monday]; ok {
		t.Errorf("monday should have no override")
	}

	// SetTargetConfig must not clobber bundesland
	if err := st.SetBundesland(ctx, uid, "BY"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTargetConfig(ctx, uid, 480, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(ctx, uid)
	if got.Bundesland != "BY" {
		t.Errorf("bundesland clobbered: got %q", got.Bundesland)
	}
}
```

Falls keine `seedUser`/`newTestPool`-Helfer existieren, die in `internal/adapter/pgstore/worktime_test.go` / `user_settings_test.go` bereits genutzten Setup-Helfer verwenden (dort nachsehen, gleiches Muster).

- [ ] **Step 5: Test laufen lassen — erwartet FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/pgstore/ -run TestUserSettings_TargetConfigRoundTrip`
Expected: FAIL (`SetTargetConfig` undefined / Spalten fehlen).

- [ ] **Step 6: Store implementieren**

`internal/adapter/pgstore/user_settings.go` ersetzen:

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

type UserSettingsStore struct{ pool *pgxpool.Pool }

func NewUserSettingsStore(pool *pgxpool.Pool) *UserSettingsStore {
	return &UserSettingsStore{pool: pool}
}

// weekdayCols lists the 7 nullable override columns in time.Weekday order
// (Sunday=0 … Saturday=6) so scan/insert stay aligned with the enum.
var weekdayCols = [7]string{
	"target_sun_min", "target_mon_min", "target_tue_min", "target_wed_min",
	"target_thu_min", "target_fri_min", "target_sat_min",
}

func (s *UserSettingsStore) Get(ctx context.Context, userID string) (domain.Settings, error) {
	const q = `
SELECT bundesland, default_target_min,
       target_sun_min, target_mon_min, target_tue_min, target_wed_min,
       target_thu_min, target_fri_min, target_sat_min
FROM user_settings WHERE user_id=$1`
	var land string
	var def int
	var wd [7]*int
	err := s.pool.QueryRow(ctx, q, userID).Scan(&land, &def,
		&wd[0], &wd[1], &wd[2], &wd[3], &wd[4], &wd[5], &wd[6])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Settings{UserID: userID, Bundesland: "NW",
			DefaultTargetMin: domain.DefaultDailyTargetMin,
			WeekdayTargetMin: map[time.Weekday]int{}}, nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("pgstore: get settings: %w", err)
	}
	overrides := map[time.Weekday]int{}
	for i, p := range wd {
		if p != nil {
			overrides[time.Weekday(i)] = *p
		}
	}
	return domain.Settings{UserID: userID, Bundesland: land,
		DefaultTargetMin: def, WeekdayTargetMin: overrides}, nil
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

func (s *UserSettingsStore) SetTargetConfig(ctx context.Context, userID string, defaultMin int, weekday map[time.Weekday]int) error {
	var wd [7]*int
	for d, v := range weekday {
		if d > time.Saturday {
			continue
		}
		vv := v
		wd[int(d)] = &vv
	}
	const q = `
INSERT INTO user_settings
  (user_id, default_target_min,
   target_sun_min, target_mon_min, target_tue_min, target_wed_min,
   target_thu_min, target_fri_min, target_sat_min, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, now())
ON CONFLICT (user_id) DO UPDATE SET
  default_target_min = EXCLUDED.default_target_min,
  target_sun_min = EXCLUDED.target_sun_min,
  target_mon_min = EXCLUDED.target_mon_min,
  target_tue_min = EXCLUDED.target_tue_min,
  target_wed_min = EXCLUDED.target_wed_min,
  target_thu_min = EXCLUDED.target_thu_min,
  target_fri_min = EXCLUDED.target_fri_min,
  target_sat_min = EXCLUDED.target_sat_min,
  updated_at = now()`
	if _, err := s.pool.Exec(ctx, q, userID, defaultMin,
		wd[0], wd[1], wd[2], wd[3], wd[4], wd[5], wd[6]); err != nil {
		return fmt.Errorf("pgstore: set target config: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Test laufen lassen — erwartet PASS**

Run: `go test ./internal/adapter/pgstore/ -run TestUserSettings`
Expected: PASS. (testcontainers braucht Docker; falls die pgstore-Tests im CI über ein Build-Tag laufen, dasselbe Tag verwenden wie M1c.)

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/pgstore/migrations/0004_target_config.sql internal/domain/settings.go internal/ports/ports.go internal/adapter/pgstore/user_settings.go internal/adapter/pgstore/user_settings_test.go
git commit -m "feat(settings): per-weekday daily target config in user_settings"
```

---

## Task 2: Carry-over der Auswertungs-Domain

Reine Funktionen + Typen aus `main` übernehmen. Quelle via `git show main:<pfad>` (gleiches Repo). Anpassung: `DayRecord.Sessions` wird `[]RecordSession` (statt `[]Session`), `Brief`/`ReportRange`/`SplitAtMidnight`/`Day`/Pause **nicht** mitnehmen.

**Files:**
- Create: `internal/domain/stats.go`, `internal/domain/burndown.go`, `internal/domain/dayrecord.go`, `internal/domain/aggregate.go`, `internal/domain/workcalendar.go`
- Test: `internal/domain/stats_test.go`, `internal/domain/aggregate_test.go`

- [ ] **Step 1: `stats.go` + `burndown.go` verbatim übernehmen**

Inhalt 1:1 aus `main`:

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git show main:internal/domain/stats.go > internal/domain/stats.go
git show main:internal/domain/burndown.go > internal/domain/burndown.go
```
Beide referenzieren nur `time`/`sort` und den (neuen) `DayOff`-Typ (in `Stats.DaysOff []DayOff`) — `domain.DayOff` existiert im Rebuild bereits.

- [ ] **Step 2: `dayrecord.go` neu (RecordSession + DayRecord + WeekDay)**

Create `internal/domain/dayrecord.go`:

```go
package domain

import "time"

// RecordSession is the per-session view stats aggregation needs: the tag
// (for the by-tag tally) and the already-computed elapsed. Built from a
// WorkSession at the use-case boundary so the pure aggregators stay I/O-free
// and independent of the live/stopped distinction.
type RecordSession struct {
	Tag     string
	Elapsed time.Duration
}

// DayRecord is one calendar day's history entry used by stats/burndown.
type DayRecord struct {
	Date     time.Time
	Sessions []RecordSession
	Total    time.Duration
	Target   time.Duration
}

// WeekDay is one day in the week view.
type WeekDay struct {
	Date    time.Time
	Logged  time.Duration
	Active  *time.Time
	Target  time.Duration
	IsToday bool
}

// Total returns logged + active elapsed for this day. The active tail is only
// added when this is today's row — past days never have a live counter.
func (w WeekDay) Total(now time.Time) time.Duration {
	if !w.IsToday || w.Active == nil {
		return w.Logged
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := *w.Active
	if start.Before(midnight) {
		start = midnight
	}
	return w.Logged + now.Sub(start)
}
```

- [ ] **Step 3: `aggregate.go` übernehmen + adaptieren**

Run: `git show main:internal/domain/aggregate.go > internal/domain/aggregate.go`

Dann **entfernen** (gehören zu Kompendium/M1e, nicht hier):
- `ReportRange`-Typ + Konstanten `ReportWeek`/`ReportMonth`,
- `BriefBounds(...)` (referenziert `MonthShortDe`/`isoMonday`-Title — nicht gebraucht),
- den Import `fmt`, falls nach dem Löschen ungenutzt.

`DayRecord.Sessions` ist jetzt `[]RecordSession` — die zwei Schleifen `for _, s := range r.Sessions { st.ByTag[s.Tag] += s.Elapsed; st.CountByTag[s.Tag]++ }` funktionieren unverändert (RecordSession hat `Tag`+`Elapsed`). Behalten: `Aggregate`, `AggregateRange`, `PlannedTarget`, `MonthBurndownCompute`, `FilterRecords`, `isHit`, `bestStreak`, `currentStreak`, `filterAndIndexRange`, `tallyRecordsInto`, `walkWorkdaysForSaldo`, `truncDay`.

- [ ] **Step 4: `workcalendar.go` neu (nur IsWorkday + isWeekend)**

Create `internal/domain/workcalendar.go`:

```go
package domain

import "time"

// IsWorkday reports whether t is neither a weekend nor a configured day-off.
// The isDayOff predicate is injected so the domain stays I/O-free.
func IsWorkday(t time.Time, isDayOff func(time.Time) bool) bool {
	if isWeekend(t) {
		return false
	}
	return !isDayOff(t)
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}
```

`isoMonday` aus `main:calendar.go` wird **nicht** mit-portiert: sein einziger Caller (`BriefBounds`) entfällt, und der Usecase nutzt sein eigenes `isoMondayLocal` (Task 5) — ein ungenutztes `domain.isoMonday` würde `staticcheck U1000` unter `make ci` triggern.

Falls `truncDay`/`isWeekend`/`IsWorkday`-Namenskonflikte mit bestehenden Rebuild-Dateien auftreten: `truncDay` kommt aus `aggregate.go` (Carry-over); prüfen, dass es nicht schon existiert (`rg -n "func truncDay|func isWeekend|func IsWorkday" internal/domain`). Bei Konflikt den bestehenden behalten und den Carry-over-Duplikat löschen.

- [ ] **Step 5: Carry-over-Tests übernehmen + adaptieren**

Erst die relevanten Testdateien in `main` auflisten:
```bash
git ls-tree main internal/domain/ | grep -E 'stats_test|aggregate_test|burndown_test'
```
Vorhandene davon übernehmen, z.B.:
```bash
git show main:internal/domain/stats_test.go > internal/domain/stats_test.go
# falls aggregate_test.go existiert:
git show main:internal/domain/aggregate_test.go > internal/domain/aggregate_test.go
```
In den übernommenen Tests **alle** `Sessions: []Session{{...Elapsed: X, Tag: "y"...}}` zu `Sessions: []RecordSession{{Elapsed: X, Tag: "y"}}` umschreiben (nur `Tag`+`Elapsed` bleiben relevant; `Date`/`Start`/`Stop`/`Note` streichen). Tests, die `BriefBounds`/`ReportRange`/`SplitAtMidnight`/`Day`/Pause prüfen, **entfernen** (nicht im M1d-Scope). Tests für `stats_text.go` (CLI-Text-Rendering) **nicht** übernehmen — `stats_text.go` ist kein M1d-Carry-over.

- [ ] **Step 6: Tests laufen lassen**

Run: `go test ./internal/domain/`
Expected: PASS (alle portierten Aggregate/Stats/Burndown-Tests grün).

- [ ] **Step 7: Commit**

```bash
git add internal/domain/stats.go internal/domain/burndown.go internal/domain/dayrecord.go internal/domain/aggregate.go internal/domain/workcalendar.go internal/domain/stats_test.go internal/domain/aggregate_test.go
git commit -m "feat(stats): carry over aggregate/stats/burndown domain from main"
```

---

## Task 3: `BuildDayRecords` (rein)

WorkSessions → DayRecords: gruppiert nach lokalem Kalendertag von `Start`, summiert Elapsed (gestoppt = `Stop-Start`, laufend = `now-Start`), setzt `Target` je Tag via injizierter `targetFor`. Der laufende Timer wird in den Tagesrekord seines Starttags eingerechnet (Live-Tail) — dadurch enthalten Today/Week/Range/Burndown automatisch die aktuelle Zeit ohne separaten `active`-Parameter.

**Files:**
- Create: `internal/domain/records.go`
- Test: `internal/domain/records_test.go`

- [ ] **Step 1: Test schreiben (failing)**

Create `internal/domain/records_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func ptr(t time.Time) *time.Time { return &t }

func TestBuildDayRecords_GroupsAndSumsPerDay(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, loc) // Monday
	d1 := time.Date(2026, 6, 14, 0, 0, 0, 0, loc)   // Sunday
	target := func(time.Time) time.Duration { return 8 * time.Hour }

	sessions := []domain.WorkSession{
		// two stopped sessions same day (Sunday): 1h + 30m
		{ID: "a", Start: time.Date(2026, 6, 14, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 14, 10, 0, 0, 0, loc)), Tag: "deep"},
		{ID: "b", Start: time.Date(2026, 6, 14, 11, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 14, 11, 30, 0, 0, loc)), Tag: ""},
		// running session today, started 13:00 → 1h live tail at now=14:00
		{ID: "c", Start: time.Date(2026, 6, 15, 13, 0, 0, 0, loc), Stop: nil, Tag: "meeting"},
	}

	recs := domain.BuildDayRecords(sessions, now, target)
	if len(recs) != 2 {
		t.Fatalf("want 2 day records, got %d", len(recs))
	}
	byDay := map[string]domain.DayRecord{}
	for _, r := range recs {
		byDay[r.Date.Format("2006-01-02")] = r
	}
	sun := byDay[d1.Format("2006-01-02")]
	if sun.Total != 90*time.Minute {
		t.Errorf("sunday total: got %v want 1h30m", sun.Total)
	}
	if sun.Target != 8*time.Hour {
		t.Errorf("sunday target: got %v", sun.Target)
	}
	if len(sun.Sessions) != 2 {
		t.Errorf("sunday sessions: got %d want 2", len(sun.Sessions))
	}
	mon := byDay[now.Format("2006-01-02")]
	if mon.Total != time.Hour {
		t.Errorf("monday live tail: got %v want 1h", mon.Total)
	}
}

func TestBuildDayRecords_Empty(t *testing.T) {
	if r := domain.BuildDayRecords(nil, time.Now(), func(time.Time) time.Duration { return 0 }); len(r) != 0 {
		t.Errorf("want empty, got %d", len(r))
	}
}
```

- [ ] **Step 2: Test laufen — FAIL**

Run: `go test ./internal/domain/ -run TestBuildDayRecords`
Expected: FAIL (`BuildDayRecords` undefined).

- [ ] **Step 3: Implementieren**

Create `internal/domain/records.go`:

```go
package domain

import (
	"sort"
	"time"
)

// BuildDayRecords groups sessions by the local calendar day of their Start and
// produces one DayRecord per day with sessions present. Elapsed per session is
// Stop-Start for finished sessions and now-Start for the running one (its live
// tail thus lands in today's Total). Target is filled per day via targetFor.
// Records are returned chronologically.
func BuildDayRecords(sessions []WorkSession, now time.Time, targetFor func(time.Time) time.Duration) []DayRecord {
	byDay := map[string]*DayRecord{}
	for _, s := range sessions {
		day := time.Date(s.Start.Year(), s.Start.Month(), s.Start.Day(), 0, 0, 0, 0, s.Start.Location())
		key := day.Format("2006-01-02")
		el := s.Elapsed(now)
		if el < 0 {
			el = 0
		}
		rec, ok := byDay[key]
		if !ok {
			rec = &DayRecord{Date: day, Target: targetFor(day)}
			byDay[key] = rec
		}
		rec.Total += el
		rec.Sessions = append(rec.Sessions, RecordSession{Tag: s.Tag, Elapsed: el})
	}
	out := make([]DayRecord, 0, len(byDay))
	for _, r := range byDay {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}
```

- [ ] **Step 4: Test laufen — PASS**

Run: `go test ./internal/domain/ -run TestBuildDayRecords`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/records.go internal/domain/records_test.go
git commit -m "feat(stats): BuildDayRecords folds sessions + live tail into day records"
```

---

## Task 4: `TargetResolver` (reines Value-Object)

Pro Request gebaut aus Settings (Default + Weekday-Overrides) + gemergten DayOffs (manuell + Feiertage). Priorität: DayOff-Target > Weekday-Override > Default.

**Files:**
- Create: `internal/usecase/target_resolver.go`
- Test: `internal/usecase/target_resolver_test.go`

- [ ] **Step 1: Test schreiben (failing)**

Create `internal/usecase/target_resolver_test.go`:

```go
package usecase_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestTargetResolver_Priority(t *testing.T) {
	loc := time.UTC
	fri := time.Date(2026, 6, 19, 0, 0, 0, 0, loc) // Friday
	sat := time.Date(2026, 6, 20, 0, 0, 0, 0, loc) // Saturday
	vacDay := time.Date(2026, 6, 18, 0, 0, 0, 0, loc) // Thursday, vacation half-day

	r := usecase.TargetResolver{
		Default: 8 * time.Hour,
		Weekday: [7]*time.Duration{
			time.Friday:   durPtr(6 * time.Hour),
			time.Saturday: durPtr(0), // explicit "no work Saturday"
		},
		DayOffs: map[string]domain.DayOff{
			vacDay.Format("2006-01-02"): {Date: vacDay, Kind: domain.KindVacation, Target: 4 * time.Hour},
		},
	}

	if got := r.For(fri); got != 6*time.Hour {
		t.Errorf("friday override: got %v want 6h", got)
	}
	if got := r.For(sat); got != 0 {
		t.Errorf("saturday explicit-0: got %v want 0", got)
	}
	if got := r.For(vacDay); got != 4*time.Hour {
		t.Errorf("dayoff override wins: got %v want 4h", got)
	}
	if !r.IsDayOff(vacDay) {
		t.Errorf("vacDay should be a day-off")
	}
	if r.IsWorkday(sat) {
		t.Errorf("saturday is weekend, not a workday")
	}
	if !r.IsWorkday(fri) {
		t.Errorf("friday should be a workday")
	}
	if r.IsWorkday(vacDay) {
		t.Errorf("vacation day is not a workday")
	}
}

func durPtr(d time.Duration) *time.Duration { return &d }
```

- [ ] **Step 2: Test laufen — FAIL**

Run: `go test ./internal/usecase/ -run TestTargetResolver_Priority`
Expected: FAIL (undefined).

- [ ] **Step 3: Implementieren**

Create `internal/usecase/target_resolver.go`:

```go
package usecase

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TargetResolver is a pure, per-request value object computing the daily work
// target for a date. Priority: day-off override > per-weekday override >
// default. Build it from a user's Settings and the merged day-off set
// (ListDayOffs) at the use-case boundary so it stays I/O-free.
type TargetResolver struct {
	Default time.Duration
	// Weekday is indexed by int(time.Weekday) (Sunday=0). A nil entry means
	// "no override → use Default"; a non-nil entry (incl. a 0-duration) is an
	// explicit override.
	Weekday [7]*time.Duration
	// DayOffs is keyed by "2006-01-02"; presence marks the date as a day-off
	// (manual or computed holiday) and supplies its target override.
	DayOffs map[string]domain.DayOff
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// For returns the target work duration for date.
func (r TargetResolver) For(date time.Time) time.Duration {
	if d, ok := r.DayOffs[dayKey(date)]; ok {
		return d.Target // 0 = full day off; >0 = half-day override
	}
	if o := r.Weekday[int(date.Weekday())]; o != nil {
		return *o
	}
	return r.Default
}

// IsDayOff reports whether date carries a day-off (manual or holiday).
func (r TargetResolver) IsDayOff(date time.Time) bool {
	_, ok := r.DayOffs[dayKey(date)]
	return ok
}

// IsWorkday reports whether date is neither weekend nor day-off.
func (r TargetResolver) IsWorkday(date time.Time) bool {
	return domain.IsWorkday(date, r.IsDayOff)
}
```

- [ ] **Step 4: Test laufen — PASS**

Run: `go test ./internal/usecase/ -run TestTargetResolver_Priority`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/target_resolver.go internal/usecase/target_resolver_test.go
git commit -m "feat(stats): pure TargetResolver (dayoff > weekday > default)"
```

---

## Task 5: `StatsComputer` Usecase

Baut pro Request den Resolver (Settings + gemergte DayOffs), liest Sessions, baut Records, ruft die Domain-Aggregatoren. Liefert vier Ergebnis-Structs.

**Files:**
- Create: `internal/usecase/stats_computer.go`
- Test: `internal/usecase/stats_computer_test.go`

- [ ] **Step 1: Ergebnis-Structs + Computer-Gerüst + Test (failing)**

Create `internal/usecase/stats_computer.go`:

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TodaySummary is the worktime header: logged-so-far, the day's target and the
// resulting saldo, plus whether a timer is running.
type TodaySummary struct {
	Date    time.Time
	Logged  time.Duration
	Target  time.Duration
	Saldo   time.Duration // Logged - Target
	Running bool
}

// StatsComputer turns stored sessions + day-offs + target config into the
// derived stats/burndown/today/week shapes. All reads are owner-scoped.
type StatsComputer struct {
	Sessions ports.SessionStore
	Settings ports.UserSettingsStore
	DayOffs  ListDayOffs // merged manual + holidays
	Clock    ports.Clock
	Loc      *time.Location
}

// resolver builds the per-request TargetResolver over [from,to] (inclusive
// bounds passed to the merged day-off list).
func (c StatsComputer) resolver(ctx context.Context, ownerID string, from, to time.Time) (TargetResolver, []domain.DayOff, error) {
	set, err := c.Settings.Get(ctx, ownerID)
	if err != nil {
		return TargetResolver{}, nil, err
	}
	offs, err := c.DayOffs.Execute(ctx, ownerID, from, to)
	if err != nil {
		return TargetResolver{}, nil, err
	}
	r := TargetResolver{
		Default: time.Duration(set.DefaultTargetMin) * time.Minute,
		DayOffs: make(map[string]domain.DayOff, len(offs)),
	}
	for d, v := range set.WeekdayTargetMin {
		if d > time.Saturday {
			continue
		}
		dur := time.Duration(v) * time.Minute
		r.Weekday[int(d)] = &dur
	}
	for _, o := range offs {
		r.DayOffs[o.Date.Format("2006-01-02")] = o
	}
	return r, offs, nil
}

func (c StatsComputer) loc() *time.Location {
	if c.Loc != nil {
		return c.Loc
	}
	return time.Local
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
```

> Falls `startOfDay` bereits im Paket `usecase` existiert, die lokale Definition hier weglassen und die bestehende nutzen (`rg -n "func startOfDay" internal/usecase`).

- [ ] **Step 2: Today/Week/RangeStats/Burndown ergänzen**

In `internal/usecase/stats_computer.go` anhängen:

```go
// Today returns the summary for the calendar day containing now.
func (c StatsComputer) Today(ctx context.Context, ownerID string) (TodaySummary, error) {
	now := c.Clock.Now().In(c.loc())
	from := startOfDay(now)
	to := from.AddDate(0, 0, 1)
	res, _, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return TodaySummary{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return TodaySummary{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	sum := TodaySummary{Date: from, Target: res.For(from)}
	for _, r := range recs {
		if r.Date.Equal(from) {
			sum.Logged = r.Total
		}
	}
	for _, s := range sessions {
		if s.Running() {
			sum.Running = true
		}
	}
	sum.Saldo = sum.Logged - sum.Target
	return sum, nil
}

// Week returns the 7 WeekDay rows of the ISO week containing ref (zero ref =
// today).
func (c StatsComputer) Week(ctx context.Context, ownerID string, ref time.Time) ([]domain.WeekDay, error) {
	now := c.Clock.Now().In(c.loc())
	if ref.IsZero() {
		ref = now
	}
	mon := isoMondayLocal(ref)
	to := mon.AddDate(0, 0, 7)
	res, _, err := c.resolver(ctx, ownerID, mon, to.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, mon)
	if err != nil {
		return nil, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	byDay := map[string]domain.DayRecord{}
	for _, r := range recs {
		byDay[r.Date.Format("2006-01-02")] = r
	}
	today := startOfDay(now)
	var running *time.Time
	for i := range sessions {
		if sessions[i].Running() {
			s := sessions[i].Start
			running = &s
		}
	}
	out := make([]domain.WeekDay, 0, 7)
	for d := mon; d.Before(to); d = d.AddDate(0, 0, 1) {
		wd := domain.WeekDay{Date: d, Target: res.For(d), IsToday: d.Equal(today)}
		if r, ok := byDay[d.Format("2006-01-02")]; ok {
			wd.Logged = r.Total // already includes the live tail from BuildDayRecords
		}
		// Active is informational for the UI marker; Total already carries it.
		if wd.IsToday && running != nil {
			wd.Active = nil // tail already folded; avoid double-count in WeekDay.Total
		}
		out = append(out, wd)
	}
	return out, nil
}

// RangeStats aggregates the ISO week ("week") or calendar month ("month")
// containing now.
func (c StatsComputer) RangeStats(ctx context.Context, ownerID, rng string) (domain.Stats, error) {
	now := c.Clock.Now().In(c.loc())
	var from, to time.Time
	switch rng {
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, c.loc())
		to = from.AddDate(0, 1, 0)
	case "week", "":
		from = isoMondayLocal(now)
		to = from.AddDate(0, 0, 7)
	default:
		return domain.Stats{}, domain.ErrInvalidRange
	}
	res, offs, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return domain.Stats{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return domain.Stats{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	listOffs := func(f, t time.Time) []domain.DayOff {
		var in []domain.DayOff
		for _, o := range offs {
			if !o.Date.Before(f) && !o.Date.After(t) {
				in = append(in, o)
			}
		}
		return in
	}
	return domain.AggregateRange(recs, from, to, res.IsWorkday, res.For, listOffs), nil
}

// Burndown reports monthly progress for the month containing now. The live
// tail is already folded into recs, so MonthBurndownCompute is called with a
// nil active marker to avoid double-counting.
func (c StatsComputer) Burndown(ctx context.Context, ownerID string) (domain.MonthBurndownReport, error) {
	now := c.Clock.Now().In(c.loc())
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, c.loc())
	to := from.AddDate(0, 1, 0)
	res, _, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return domain.MonthBurndownReport{}, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return domain.MonthBurndownReport{}, err
	}
	recs := domain.BuildDayRecords(sessions, now, res.For)
	return domain.MonthBurndownCompute(now, recs, nil, res.IsWorkday, res.For), nil
}

// isoMondayLocal mirrors domain.isoMonday (unexported there) for the use case.
func isoMondayLocal(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	d := t.AddDate(0, 0, -(wd - 1))
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}
```

Add to `internal/domain/errors.go`: `ErrInvalidRange = errors.New("invalid range")` (prüfen ob `errors.go` schon ein Var-Block hat; dort einreihen).

- [ ] **Step 3: Test schreiben (failing)** — mit Fakes

Create `internal/usecase/stats_computer_test.go`:

```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeSessionStore returns a fixed list regardless of since (test controls input).
type fakeSessionStore struct{ list []domain.WorkSession }

func (f fakeSessionStore) Create(context.Context, domain.WorkSession) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f fakeSessionStore) Running(context.Context, string) (domain.WorkSession, bool, error) {
	return domain.WorkSession{}, false, nil
}
func (f fakeSessionStore) Stop(context.Context, string, string, *string, time.Time) (domain.WorkSession, error) {
	return domain.WorkSession{}, nil
}
func (f fakeSessionStore) List(_ context.Context, _ string, since time.Time) ([]domain.WorkSession, error) {
	var out []domain.WorkSession
	for _, s := range f.list {
		if !s.Start.Before(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeSettings struct{ s domain.Settings }

func (f fakeSettings) Get(context.Context, string) (domain.Settings, error) { return f.s, nil }
func (f fakeSettings) SetBundesland(context.Context, string, string) error  { return nil }
func (f fakeSettings) SetTargetConfig(context.Context, string, int, map[time.Weekday]int) error {
	return nil
}

// fakeDayOffStore feeds ListDayOffs; no manual offs here (holidays computed).
type fakeDayOffStore struct{}

func (fakeDayOffStore) Add(context.Context, string, domain.DayOff) error { return nil }
func (fakeDayOffStore) Delete(context.Context, string, time.Time) error  { return nil }
func (fakeDayOffStore) ListRange(context.Context, string, time.Time, time.Time) ([]domain.DayOff, error) {
	return nil, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newComputer(sessions []domain.WorkSession, set domain.Settings) usecase.StatsComputer {
	return usecase.StatsComputer{
		Sessions: fakeSessionStore{list: sessions},
		Settings: fakeSettings{s: set},
		DayOffs:  usecase.ListDayOffs{Store: fakeDayOffStore{}, Settings: fakeSettings{s: set}, Loc: time.UTC},
		Clock:    fixedClock{t: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)}, // Monday
		Loc:      time.UTC,
	}
}

func TestStatsComputer_TodaySaldo(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	sessions := []domain.WorkSession{
		{ID: "a", Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC))},
	}
	c := newComputer(sessions, set)
	sum, err := c.Today(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Logged != 2*time.Hour {
		t.Errorf("logged: got %v want 2h", sum.Logged)
	}
	if sum.Target != 8*time.Hour {
		t.Errorf("target: got %v want 8h", sum.Target)
	}
	if sum.Saldo != -6*time.Hour {
		t.Errorf("saldo: got %v want -6h", sum.Saldo)
	}
}

func TestStatsComputer_WeekHasSevenDays(t *testing.T) {
	set := domain.Settings{Bundesland: "NW", DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}}
	c := newComputer(nil, set)
	wk, err := c.Week(context.Background(), "u1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(wk) != 7 {
		t.Fatalf("want 7 days, got %d", len(wk))
	}
	// Saturday/Sunday are weekends → target stays default but they are not workdays.
	if wk[0].Date.Weekday() != time.Monday {
		t.Errorf("week starts Monday, got %v", wk[0].Date.Weekday())
	}
}

func TestStatsComputer_RangeInvalid(t *testing.T) {
	c := newComputer(nil, domain.Settings{DefaultTargetMin: 480, WeekdayTargetMin: map[time.Weekday]int{}})
	if _, err := c.RangeStats(context.Background(), "u1", "year"); err == nil {
		t.Errorf("want error for invalid range")
	}
}
```

- [ ] **Step 4: Tests laufen — PASS**

Run: `go test ./internal/usecase/ -run TestStatsComputer`
Expected: PASS. (Falls `ListDayOffs` `Loc` braucht und der Fake-Holiday-Merge in der Testwoche Feiertage einstreut: KW 25/2026 hat keine NW-Feiertage, also bleibt die Woche sauber.)

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/stats_computer.go internal/usecase/stats_computer_test.go internal/domain/errors.go
git commit -m "feat(stats): StatsComputer (today/week/range/burndown) over sessions+dayoffs"
```

---

## Task 6: HTTP-Handler, DTOs, Routen, settings.changed-Event

**Files:**
- Create: `internal/adapter/httpserver/stats.go`
- Modify: `internal/adapter/httpserver/dayoffs.go` (settingsDTO + handleSetTarget)
- Modify: `internal/adapter/httpserver/server.go` (Felder + Routen)
- Modify: `internal/domain/event.go` (EventSettingsChanged)
- Test: `internal/adapter/httpserver/stats_test.go`

- [ ] **Step 1: Event-Typ ergänzen**

In `internal/domain/event.go` im const-Block: `EventSettingsChanged EventType = "settings.changed"`.

- [ ] **Step 2: Stats-DTOs + Handler**

Create `internal/adapter/httpserver/stats.go`:

```go
package httpserver

import (
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// minutes converts a Duration to whole minutes for the wire.
func minutes(d time.Duration) int { return int(d / time.Minute) }

type todayDTO struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	SaldoMin  int    `json:"saldoMin"`
	Running   bool   `json:"running"`
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	sum, err := s.Stats.Today(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, todayDTO{
		Date: sum.Date.Format(dayFmt), LoggedMin: minutes(sum.Logged),
		TargetMin: minutes(sum.Target), SaldoMin: minutes(sum.Saldo), Running: sum.Running,
	})
}

type weekDayDTO struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	IsToday   bool   `json:"isToday"`
	Workday   bool   `json:"workday"`
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var ref time.Time
	if q := r.URL.Query().Get("ref"); q != "" {
		t, err := time.ParseInLocation(dayFmt, q, time.Local)
		if err != nil {
			http.Error(w, "ref must be yyyy-mm-dd", http.StatusBadRequest)
			return
		}
		ref = t
	}
	week, err := s.Stats.Week(r.Context(), u.ID, ref)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	out := make([]weekDayDTO, 0, len(week))
	now := s.Clock.Now()
	for _, d := range week {
		out = append(out, weekDayDTO{
			Date: d.Date.Format(dayFmt), LoggedMin: minutes(d.Total(now)),
			TargetMin: minutes(d.Target), IsToday: d.IsToday,
			Workday: d.Date.Weekday() != time.Saturday && d.Date.Weekday() != time.Sunday,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type statsDTO struct {
	From          string `json:"from"`
	To            string `json:"to"`
	TotalMin      int    `json:"totalMin"`
	AvgMin        int    `json:"avgMin"`
	MaxMin        int    `json:"maxMin"`
	MinMin        int    `json:"minMin"`
	Workdays      int    `json:"workdays"`
	DaysWithWork  int    `json:"daysWithWork"`
	Hits          int    `json:"hits"`
	Streak        int    `json:"streak"`
	BestStreak    int    `json:"bestStreak"`
	OvertimeMin   int    `json:"overtimeMin"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rng := r.URL.Query().Get("range")
	st, err := s.Stats.RangeStats(r.Context(), u.ID, rng)
	if err == domain.ErrInvalidRange {
		http.Error(w, "range must be week or month", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, statsDTO{
		TotalMin: minutes(st.Total), AvgMin: minutes(st.Avg), MaxMin: minutes(st.Max),
		MinMin: minutes(st.Min), Workdays: st.Workdays, DaysWithWork: st.DaysWithSessions,
		Hits: st.Hits, Streak: st.Streak, BestStreak: st.BestStreak, OvertimeMin: minutes(st.Overtime),
	})
}

type burndownDTO struct {
	TotalMin    int  `json:"totalMin"`
	TargetMin   int  `json:"targetMin"`
	SaldoMin    int  `json:"saldoMin"`
	OnTrack     bool `json:"onTrack"`
	WorkdaysAll int  `json:"workdaysAll"`
	WorkdaysDue int  `json:"workdaysDue"`
}

func (s *Server) handleBurndown(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	b, err := s.Stats.Burndown(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, burndownDTO{
		TotalMin: minutes(b.Total), TargetMin: minutes(b.Target), SaldoMin: minutes(b.Saldo),
		OnTrack: b.OnTrack, WorkdaysAll: b.WorkdaysAll, WorkdaysDue: b.WorkdaysDue,
	})
}
```

(Hinweis: `gofmt` richtet die ausgerichteten Struct-Tags neu aus — das ist erwartet.)

- [ ] **Step 3: settingsDTO erweitern + handleSetTarget**

In `internal/adapter/httpserver/dayoffs.go` den `settingsDTO` + `handleGetSettings` ersetzen und `handleSetTarget` ergänzen:

```go
type settingsDTO struct {
	Bundesland       string         `json:"bundesland"`
	FeedURLs         []string       `json:"feedUrls"`
	DefaultTargetMin int            `json:"defaultTargetMin"`
	WeekdayTargetMin map[string]int `json:"weekdayTargetMin"` // key = weekday number "0".."6"
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
	wd := make(map[string]int, len(set.WeekdayTargetMin))
	for d, v := range set.WeekdayTargetMin {
		wd[strconv.Itoa(int(d))] = v
	}
	writeJSON(w, http.StatusOK, settingsDTO{
		Bundesland: set.Bundesland, FeedURLs: urls,
		DefaultTargetMin: set.DefaultTargetMin, WeekdayTargetMin: wd,
	})
}

type setTargetReq struct {
	DefaultTargetMin int            `json:"defaultTargetMin"`
	WeekdayTargetMin map[string]int `json:"weekdayTargetMin"`
}

func (s *Server) handleSetTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setTargetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	wd := make(map[time.Weekday]int, len(req.WeekdayTargetMin))
	for k, v := range req.WeekdayTargetMin {
		n, err := strconv.Atoi(k)
		if err != nil || n < 0 || n > 6 {
			http.Error(w, "weekday key must be 0..6", http.StatusBadRequest)
			return
		}
		wd[time.Weekday(n)] = v
	}
	if err := s.SetTarget.Execute(r.Context(), u.ID, req.DefaultTargetMin, wd); err != nil {
		if errors.Is(err, domain.ErrInvalidTarget) {
			http.Error(w, "target minutes must be >= 0", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	w.WriteHeader(http.StatusNoContent)
}
```

`strconv` zum Importblock von `dayoffs.go` hinzufügen.

- [ ] **Step 4: SetTargetConfig-Usecase + ErrInvalidTarget**

Create `internal/usecase/set_target.go`:

```go
package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetTargetConfig validates and stores the daily target config (default +
// per-weekday overrides). All minute values must be non-negative.
type SetTargetConfig struct {
	Settings ports.UserSettingsStore
}

func (uc SetTargetConfig) Execute(ctx context.Context, ownerID string, defaultMin int, weekday map[time.Weekday]int) error {
	if defaultMin < 0 {
		return domain.ErrInvalidTarget
	}
	for _, v := range weekday {
		if v < 0 {
			return domain.ErrInvalidTarget
		}
	}
	return uc.Settings.SetTargetConfig(ctx, ownerID, defaultMin, weekday)
}
```

Add to `internal/domain/errors.go`: `ErrInvalidTarget = errors.New("invalid target")`.

- [ ] **Step 5: Server-Struct + Routen erweitern**

In `internal/adapter/httpserver/server.go`, `Server` ergänzen (im `// m1c worktime extras`-Block oder neuem `// m1d stats`-Block):

```go
	// m1d stats
	Stats     usecase.StatsComputer
	SetTarget usecase.SetTargetConfig
```

In `Routes()` nach den dayoff-Routen einfügen:

```go
	mux.Handle("GET /api/v1/today", s.auth(http.HandlerFunc(s.handleToday)))
	mux.Handle("GET /api/v1/week", s.auth(http.HandlerFunc(s.handleWeek)))
	mux.Handle("GET /api/v1/stats", s.auth(http.HandlerFunc(s.handleStats)))
	mux.Handle("GET /api/v1/burndown", s.auth(http.HandlerFunc(s.handleBurndown)))
	mux.Handle("POST /api/v1/settings/target", s.auth(http.HandlerFunc(s.handleSetTarget)))
```

- [ ] **Step 6: Handler-Test schreiben (failing)**

Create `internal/adapter/httpserver/stats_test.go` nach dem Muster von `dayoffs_test.go`/`settings_test.go` in diesem Paket (gleiches Test-Harness: ein `Server` mit Fake-Stores + Bearer-Stub). Mindestens:

```go
func TestHandleStats_InvalidRange(t *testing.T) {
	srv := newTestServer(t) // existing harness in this package
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats?range=year", nil)
	req = withTestUser(req) // existing helper that injects an authed user
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d want 400", rr.Code)
	}
}

func TestHandleSetTarget_PublishesEvent(t *testing.T) {
	srv := newTestServer(t)
	body := strings.NewReader(`{"defaultTargetMin":480,"weekdayTargetMin":{"5":360}}`)
	req := withTestUser(httptest.NewRequest(http.MethodPost, "/api/v1/settings/target", body))
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("got %d want 204", rr.Code)
	}
	// assert a settings.changed event was published (inspect the fake bus)
}
```

> Den genauen Aufbau (`newTestServer`, `withTestUser`, Fake-Bus) aus `internal/adapter/httpserver/dayoffs_test.go` / `settings_test.go` 1:1 übernehmen — dort steht das etablierte Harness inkl. Fake-`Stats`/`SetTarget`-Verdrahtung (Fakes für `usecase.StatsComputer` gibt es nicht direkt, da es ein Struct ist; den Computer mit Fake-Stores wie in Task 5 bauen oder die Route über einen leichten Fake umgehen — Harness-Stil des Pakets folgen).

- [ ] **Step 7: Tests laufen — PASS**

Run: `go test ./internal/adapter/httpserver/ ./internal/usecase/ ./internal/domain/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/httpserver/stats.go internal/adapter/httpserver/dayoffs.go internal/adapter/httpserver/server.go internal/adapter/httpserver/stats_test.go internal/usecase/set_target.go internal/domain/event.go internal/domain/errors.go
git commit -m "feat(stats): REST today/week/stats/burndown + target config + settings.changed event"
```

---

## Task 7: apiclient-Methoden + DTOs

**Files:**
- Create: `internal/adapter/apiclient/stats.go`
- Modify: `internal/adapter/apiclient/dayoffs.go` (Settings-DTO + SetTargetConfig)
- Test: `internal/adapter/apiclient/stats_test.go`

- [ ] **Step 1: Client-DTOs + Methoden**

Create `internal/adapter/apiclient/stats.go`:

```go
package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

type Today struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	SaldoMin  int    `json:"saldoMin"`
	Running   bool   `json:"running"`
}

type WeekDay struct {
	Date      string `json:"date"`
	LoggedMin int    `json:"loggedMin"`
	TargetMin int    `json:"targetMin"`
	IsToday   bool   `json:"isToday"`
	Workday   bool   `json:"workday"`
}

type Stats struct {
	TotalMin     int `json:"totalMin"`
	AvgMin       int `json:"avgMin"`
	MaxMin       int `json:"maxMin"`
	MinMin       int `json:"minMin"`
	Workdays     int `json:"workdays"`
	DaysWithWork int `json:"daysWithWork"`
	Hits         int `json:"hits"`
	Streak       int `json:"streak"`
	BestStreak   int `json:"bestStreak"`
	OvertimeMin  int `json:"overtimeMin"`
}

type Burndown struct {
	TotalMin    int  `json:"totalMin"`
	TargetMin   int  `json:"targetMin"`
	SaldoMin    int  `json:"saldoMin"`
	OnTrack     bool `json:"onTrack"`
	WorkdaysAll int  `json:"workdaysAll"`
	WorkdaysDue int  `json:"workdaysDue"`
}

func (c *Client) GetToday(ctx context.Context) (Today, error) {
	var t Today
	err := c.do(ctx, http.MethodGet, "/api/v1/today", nil, &t)
	return t, err
}

func (c *Client) GetWeek(ctx context.Context, ref string) ([]WeekDay, error) {
	path := "/api/v1/week"
	if ref != "" {
		path += "?" + url.Values{"ref": {ref}}.Encode()
	}
	var out []WeekDay
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) GetStats(ctx context.Context, rng string) (Stats, error) {
	var s Stats
	err := c.do(ctx, http.MethodGet, "/api/v1/stats?"+url.Values{"range": {rng}}.Encode(), nil, &s)
	return s, err
}

func (c *Client) GetBurndown(ctx context.Context) (Burndown, error) {
	var b Burndown
	err := c.do(ctx, http.MethodGet, "/api/v1/burndown", nil, &b)
	return b, err
}
```

- [ ] **Step 2: Settings-DTO erweitern + SetTargetConfig**

In `internal/adapter/apiclient/dayoffs.go` den `Settings`-Typ erweitern und Methode ergänzen:

```go
// Settings mirrors the server settingsDTO.
type Settings struct {
	Bundesland       string         `json:"bundesland"`
	FeedURLs         []string       `json:"feedUrls"`
	DefaultTargetMin int            `json:"defaultTargetMin"`
	WeekdayTargetMin map[string]int `json:"weekdayTargetMin"`
}

func (c *Client) SetTargetConfig(ctx context.Context, defaultMin int, weekday map[string]int) error {
	return c.do(ctx, http.MethodPost, "/api/v1/settings/target", map[string]any{
		"defaultTargetMin": defaultMin, "weekdayTargetMin": weekday,
	}, nil)
}
```

- [ ] **Step 3: Test schreiben (failing)** — `httptest`-Server wie in `client_test.go`

Create `internal/adapter/apiclient/stats_test.go`:

```go
package apiclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestGetBurndown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/burndown" {
			t.Errorf("path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"totalMin":120,"targetMin":480,"saldoMin":-360,"onTrack":false,"workdaysAll":22,"workdaysDue":10}`))
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tok")
	b, err := c.GetBurndown(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b.TotalMin != 120 || b.OnTrack {
		t.Errorf("got %+v", b)
	}
}
```

> `t.Context()` ist Go 1.24+; falls die Codebase älter ist, `context.Background()` nutzen (an bestehenden `client_test.go` orientieren).

- [ ] **Step 4: Tests laufen — PASS**

Run: `go test ./internal/adapter/apiclient/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/apiclient/stats.go internal/adapter/apiclient/dayoffs.go internal/adapter/apiclient/stats_test.go
git commit -m "feat(stats): apiclient today/week/stats/burndown + SetTargetConfig"
```

---

## Task 8: TUI — Today/Burndown-Header + reloadStats-Wiring

**Files:**
- Modify: `internal/tui/worktime.go`
- Test: `internal/tui/worktime_test.go`

- [ ] **Step 1: Model-Felder + Lade-Command**

In `internal/tui/worktime.go`, `Model` um Felder ergänzen:

```go
	today    apiclient.Today
	burndown apiclient.Burndown
```

Und einen Lade-Command + Msg ergänzen:

```go
type statsLoadedMsg struct {
	today    apiclient.Today
	burndown apiclient.Burndown
}

func (m Model) reloadStats() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		today, err := m.client.GetToday(ctx)
		if err != nil {
			return errMsg{err}
		}
		bd, err := m.client.GetBurndown(ctx)
		if err != nil {
			return errMsg{err}
		}
		return statsLoadedMsg{today: today, burndown: bd}
	}
}
```

- [ ] **Step 2: Init + Update verdrahten**

In `Init()` `m.reloadStats()` in den `tea.Batch` aufnehmen. In `Update`:
- `statsLoadedMsg` behandeln: `m.today = msg.today; m.burndown = msg.burndown; return m, nil`.
- im `eventMsg`-Zweig den Batch erweitern: `return m, tea.Batch(m.reload(), m.reloadDayOffs(), m.reloadStats(), waitForEvent(m.events))`.

- [ ] **Step 3: View-Header rendern**

In `View()` nach der Running/Idle-Zeile eine Saldo-Zeile einfügen (vor `Today`-Header):

```go
	{
		logged := fmtMin(m.today.LoggedMin)
		target := fmtMin(m.today.TargetMin)
		saldo := m.today.SaldoMin
		saldoStr := fmtSaldo(saldo)
		line := fmt.Sprintf("  heute %s / %s · %s", logged, target, saldoStr)
		if saldo >= 0 {
			b.WriteString(styleOk.Render(line) + "\n")
		} else {
			b.WriteString(styleWarn.Render(line) + "\n")
		}
		if m.burndown.TargetMin > 0 {
			bd := fmt.Sprintf("  Monat %s / %s · %s", fmtMin(m.burndown.TotalMin), fmtMin(m.burndown.TargetMin), fmtSaldo(m.burndown.SaldoMin))
			b.WriteString(styleMuted.Render(bd) + "\n")
		}
	}
```

Helper unten in der Datei ergänzen:

```go
func fmtMin(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%dh %02dm", min/60, min%60)
}

func fmtSaldo(min int) string {
	sign := "+"
	if min < 0 {
		sign = "-"
		min = -min
	}
	return fmt.Sprintf("%s%dh %02dm", sign, min/60, min%60)
}
```

`styleOk`/`styleWarn` in `internal/tui/styles.go` ergänzen, falls nicht vorhanden (Tokyonight green/red — bestehende Sem-Farben aus `styles.go` wiederverwenden; `rg -n "styleOk|styleWarn|styleRunning" internal/tui/styles.go`).

- [ ] **Step 4: Test ergänzen + laufen**

In `internal/tui/worktime_test.go` einen Test ergänzen, der ein `statsLoadedMsg` durch `Update` schickt und prüft, dass `View()` „heute" + den Saldo enthält. Muster wie bestehende Update/View-Tests in der Datei.

Run: `go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/worktime.go internal/tui/worktime_test.go internal/tui/styles.go
git commit -m "feat(tui): today saldo + monthly burndown header, reload stats on events"
```

---

## Task 9: TUI — Wochen-Sicht (`w`) + Stats-Sicht (`t`) + Target-Editor

**Files:**
- Create: `internal/tui/stats.go`
- Modify: `internal/tui/worktime.go` (Keys + View-Routing + Model-Felder)
- Test: `internal/tui/stats_test.go`

- [ ] **Step 1: Model-Felder + Keys**

In `worktime.go` `Model` ergänzen:

```go
	showWeek  bool
	showStats bool
	week      []apiclient.WeekDay
	stats     apiclient.Stats
	statsRng  string // "week" | "month"
```

In `handleKey` (im nicht-booking-Zweig) ergänzen:

```go
	case k.Text == "w":
		m.showWeek = true
		return m, m.reloadWeek()
	case k.Text == "t":
		m.showStats = true
		if m.statsRng == "" {
			m.statsRng = "week"
		}
		return m, m.reloadRange()
	case k.Text == "m" && m.showStats:
		m.statsRng = "month"
		return m, m.reloadRange()
	case k.Text == "W" && m.showStats:
		m.statsRng = "week"
		return m, m.reloadRange()
	case k.Code == tea.KeyEsc && (m.showWeek || m.showStats):
		m.showWeek = false
		m.showStats = false
		return m, nil
```

(Den bestehenden `case k.Code == tea.KeyEsc && m.showDayOffs` belassen; die neue Esc-Klausel ergänzt um Week/Stats.)

- [ ] **Step 2: Lade-Commands + Msgs (in stats.go)**

Create `internal/tui/stats.go`:

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

type weekLoadedMsg struct{ week []apiclient.WeekDay }
type rangeLoadedMsg struct {
	rng   string
	stats apiclient.Stats
}

func (m Model) reloadWeek() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		wk, err := m.client.GetWeek(ctx, "")
		if err != nil {
			return errMsg{err}
		}
		return weekLoadedMsg{week: wk}
	}
}

func (m Model) reloadRange() tea.Cmd {
	if m.client == nil {
		return nil
	}
	rng := m.statsRng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := m.client.GetStats(ctx, rng)
		if err != nil {
			return errMsg{err}
		}
		return rangeLoadedMsg{rng: rng, stats: st}
	}
}

func (m Model) weekView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · Woche") + "\n\n")
	for _, d := range m.week {
		day := d.Date // yyyy-mm-dd
		bar := bar(d.LoggedMin, d.TargetMin, 20)
		marker := "  "
		if d.IsToday {
			marker = styleRunning.Render("▶ ")
		}
		line := fmt.Sprintf("%s%s  %s %s/%s", marker, day, bar, fmtMin(d.LoggedMin), fmtMin(d.TargetMin))
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + styleMuted.Render("esc back · q quit") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m Model) statsView() tea.View {
	var b strings.Builder
	label := "KW"
	if m.statsRng == "month" {
		label = "Monat"
	}
	b.WriteString(styleHeader.Render("flow · Stats · "+label) + "\n\n")
	s := m.stats
	rows := [][2]string{
		{"Total", fmtMin(s.TotalMin)},
		{"⌀/Tag", fmtMin(s.AvgMin)},
		{"Max", fmtMin(s.MaxMin)},
		{"Min", fmtMin(s.MinMin)},
		{"Arbeitstage", fmt.Sprintf("%d", s.Workdays)},
		{"Treffer", fmt.Sprintf("%d/%d", s.Hits, s.Workdays)},
		{"Streak", fmt.Sprintf("%d (best %d)", s.Streak, s.BestStreak)},
		{"Saldo", fmtSaldo(s.OvertimeMin)},
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", r[0], r[1]))
	}
	b.WriteString("\n" + styleMuted.Render("W Woche · m Monat · esc back · q quit") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// bar renders a fixed-width horizontal progress bar (filled vs target).
func bar(logged, target, width int) string {
	if target <= 0 {
		target = 1
	}
	filled := logged * width / target
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
```

- [ ] **Step 3: Update + View-Routing**

In `worktime.go` `Update` die neuen Msgs behandeln:

```go
	case weekLoadedMsg:
		m.week = msg.week
		return m, nil
	case rangeLoadedMsg:
		m.statsRng = msg.rng
		m.stats = msg.stats
		return m, nil
```

Im `eventMsg`-Zweig zusätzlich Week/Range nachladen, wenn deren Sicht offen ist:

```go
	case eventMsg:
		cmds := []tea.Cmd{m.reload(), m.reloadDayOffs(), m.reloadStats(), waitForEvent(m.events)}
		if m.showWeek {
			cmds = append(cmds, m.reloadWeek())
		}
		if m.showStats {
			cmds = append(cmds, m.reloadRange())
		}
		return m, tea.Batch(cmds...)
```

In `View()` ganz oben das Routing erweitern:

```go
	if m.showWeek {
		return m.weekView()
	}
	if m.showStats {
		return m.statsView()
	}
	if m.showDayOffs {
		return m.dayOffView()
	}
```

Und die Footer-Hinweiszeile um `w Woche · t Stats` ergänzen.

- [ ] **Step 4: Test schreiben + laufen**

In `internal/tui/stats_test.go` Tests: `weekLoadedMsg` → `weekView()` enthält die Tage; `rangeLoadedMsg` → `statsView()` enthält „Total". `bar(60,120,10)` == `"[█████░░░░░]"`.

Run: `go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 5: Settings-Target-Editor (Mini)**

Den bestehenden Settings-/DayOff-Bereich der TUI (M1c) um eine Anzeige des aktuellen Tagesziels erweitern: in `dayOffView()` oder der Settings-Mini eine Zeile „Tagesziel: 8h 00m (Fr 6h)“ aus `m.client.GetSettings`. Eingabe: eine einfache Prompt-Zeile zum Setzen des Default-Ziels (`SetTargetConfig(defaultMin, currentWeekdayMap)`), Weekday-Overrides editierbar analog. Falls der M1c-Settings-Screen noch minimal ist, hier nur die **Anzeige** + Default-Ziel-Edit umsetzen (Weekday-Edit kann eine schlanke Folgezeile sein) — Eingabe-UX bewusst schlicht, Politur vertagt. Test: Update-Pfad für das Setzen des Default-Ziels.

Run: `go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/stats.go internal/tui/worktime.go internal/tui/stats_test.go
git commit -m "feat(tui): week view + range stats + target config editor"
```

---

## Task 10: WebUI — Stats-Fragmente + Target-Form + SSE

Spiegelt die TUI-Sichten in der WebUI nach dem M1c-Muster (`dayoffs.templ` + `webui_dayoffs.go`). Bewusst schlicht; horizontale Balken (Design-Sprache).

**Files:**
- Create: `internal/adapter/webui/stats.templ` (+ generiertes `stats_templ.go`)
- Create: `internal/adapter/httpserver/webui_stats.go`
- Modify: `internal/adapter/httpserver/server.go` (WebUI-Routen)
- Test: `internal/adapter/httpserver/webui_stats_test.go`

- [ ] **Step 1: templ-Komponenten**

Create `internal/adapter/webui/stats.templ` mit Komponenten: `StatsPage(today, burndown, week, stats)`, `TodaySaldo(today)`, `BurndownBar(burndown)`, `WeekBars(week)`, `RangeStats(stats)`, `TargetForm(settings)`. Muster + Imports/Style aus `dayoffs.templ` übernehmen. Horizontale Balken via inline-width-`div` (Tailwind/inline-style), Saldo-Farbe grün/rot. Die SSE-lauschenden Wrapper bekommen `hx-ext="sse"` + `sse-swap="session.stopped,dayoff.changed,settings.changed"` und `hx-get="/ui/stats/fragment"` (Muster: M1c `webui_dayoffs.go` Fragment-Re-render).

> Den exakten templ-Stil (Layout-Wrapper, `templ.Component`-Signaturen, Class-Konventionen) aus `internal/adapter/webui/dayoffs.templ` 1:1 spiegeln, damit Look & Tooling konsistent bleiben.

- [ ] **Step 2: templ generieren**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && templ generate ./internal/adapter/webui/`
Expected: erzeugt `internal/adapter/webui/stats_templ.go`. (Falls `templ` nicht im PATH: `go tool templ generate` bzw. das Makefile-Target nutzen, das M1c verwendet — `rg -n "templ generate" Makefile`.)

- [ ] **Step 3: WebUI-Handler**

Create `internal/adapter/httpserver/webui_stats.go` mit `handleWebStatsHome` (volle Seite) + `handleWebStatsFragment` (HTMX-Re-render) + `handleWebSetTarget` (Form-POST → `s.SetTarget.Execute` → `s.Bus.Publish(settings.changed)` → Fragment zurück). Muster: `internal/adapter/httpserver/webui_dayoffs.go`. Die Handler holen Daten direkt über die Usecases (`s.Stats.Today/Week/RangeStats/Burndown`, `s.GetSettings`).

- [ ] **Step 4: Routen**

In `server.go` `Routes()` ergänzen:

```go
	mux.Handle("GET /stats", s.webAuth(http.HandlerFunc(s.handleWebStatsHome)))
	mux.Handle("GET /ui/stats/fragment", s.webAuth(http.HandlerFunc(s.handleWebStatsFragment)))
	mux.Handle("POST /ui/stats/target", s.webAuth(http.HandlerFunc(s.handleWebSetTarget)))
```

Und einen Nav-Link „Stats“ in der bestehenden WebUI-Navigation ergänzen (wo M1c „DayOffs“ verlinkt — `rg -n "dayoffs" internal/adapter/webui/*.templ`).

- [ ] **Step 5: Test + render**

`internal/adapter/httpserver/webui_stats_test.go`: GET `/stats` (authed) → 200 + enthält „Woche“/„Monat“; POST `/ui/stats/target` mit Form → 204/200 + Event publiziert. Muster: `webui_dayoffs_test.go`.

Run: `go test ./internal/adapter/httpserver/ ./internal/adapter/webui/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/stats.templ internal/adapter/webui/stats_templ.go internal/adapter/httpserver/webui_stats.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_stats_test.go
git commit -m "feat(webui): stats page (today/burndown/week/range) + target form, SSE-live"
```

---

## Task 11: Wiring-Verification + curl-Smoke + make ci

**Files:**
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Composition-Root verdrahten**

In `cmd/flow-server/main.go` im `srv := &httpserver.Server{...}` ergänzen (nach den M1c-Feldern):

```go
		Stats: usecase.StatsComputer{
			Sessions: sessionStore,
			Settings: settingsStore,
			DayOffs:  usecase.ListDayOffs{Store: dayOffStore, Settings: settingsStore, Loc: time.Local},
			Clock:    clock,
			Loc:      time.Local,
		},
		SetTarget: usecase.SetTargetConfig{Settings: settingsStore},
```

- [ ] **Step 2: Build + vet**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go vet ./...`
Expected: keine Fehler.

- [ ] **Step 3: Lokalen Dev-Stack hochfahren + curl-Smoke**

Run (Muster aus `reference_flow_dev_env`):
```bash
make dev-up          # Postgres + Dex
make dev-run &       # flow-server mit FLOW_DEV=1
TOKEN=$(make dev-token)   # bzw. der etablierte Token-Pfad
BASE=http://localhost:8080
for path in "/api/v1/today" "/api/v1/week" "/api/v1/stats?range=week" "/api/v1/stats?range=month" "/api/v1/burndown" "/api/v1/settings"; do
  echo "GET $path"; curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE$path" | head -c 300; echo
done
echo "POST /api/v1/settings/target"
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"defaultTargetMin":480,"weekdayTargetMin":{"5":360}}' -o /dev/null -w '%{http_code}\n' "$BASE/api/v1/settings/target"
echo "GET /api/v1/stats?range=year (expect 400)"
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/stats?range=year"
```
Expected: alle GET → 200 mit JSON; `settings/target` → 204; `range=year` → 400. Genaue `make`-Targets/Token-Beschaffung an `reference_flow_dev_env` / dem Makefile ausrichten.

- [ ] **Step 4: make ci**

Run: `make ci`
Expected: grün (Coverage-Gate wie M1c ~81%; falls neue Pakete das Gate drücken, gezielt Tests ergänzen, nicht das Gate senken).

- [ ] **Step 5: Done-Gate manuell (Cross-Surface-Live-Loop)**

- TUI (`flow` mit Device-Flow-Token) offen; WebUI (`/stats`) offen.
- In Settings (WebUI) Tagesziel auf Fr 6h setzen → Wochen-Sicht/Burndown-Target passen sich in beiden Surfaces ~1 s an.
- Timer in der TUI starten → WebUI Today-Saldo & Monats-Gauge aktualisieren ~1 s (und Stop umgekehrt).
- Urlaubstag (M1c) eintragen → geplantes Monats-Target sinkt cross-surface.

Dokumentiere das Ergebnis (kurzer Vermerk), Screenshots optional.

- [ ] **Step 6: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(stats): wire StatsComputer + target config into composition root"
```

- [ ] **Step 7: HEAD verifizieren**

Run: `git log --oneline -12 && git status`
Expected: alle M1d-Commits auf `rebuild`, Worktree clean. (Lesson „subagent commits können isoliert sein“ — HEAD-Ref nach jedem Subagent prüfen, Orphans via reflog bergen, finales Wiring selbst fahren.)

---

## Self-Review-Notiz (vom Planautor)

**Spec-Coverage:** Tagesziel-Config (T1/T4/T6) ✓, server-`StatsComputer` (T5) ✓, Burndown-Gauge (T5/T6/T8/T10) ✓, Today-Saldo (T5/T6/T8/T10) ✓, Wochen-Sicht (T5/T6/T9/T10) ✓, Range-Stats (T5/T6/T9/T10) ✓, Live-Sync ohne neues Domain-Event für Stats + `settings.changed` für Target (T6/T8/T9/T10) ✓, by-tag getragen aber nicht gezeigt (Stats-DTO lässt `ByTag` weg, Domain trägt es — T2/T6) ✓, `WorkSession`→`RecordSession`-Adapter (T2/T3) ✓, Wiring-Task (T11) ✓.

**Bewusste Abweichungen von der Spec:** `TargetResolver` ist ein reines Value-Object (nicht der `ConfigReader`-Port aus `main`), weil die Quelle Settings+gemergte-DayOffs ist — klarer testbar. Today nutzt eine schlanke `TodaySummary` statt `main`s `Day` (kein Pause-Konzept im Rebuild).
