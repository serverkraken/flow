# flow Rebuild M1e — Zeit-Export pro Projekt/Zeitraum · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Server-berechneter Worktime-Export über einen Datumsbereich — pro Projekt aggregiert + Detail-Zeilen, in CSV/JSON/Markdown, inkl. Σh×Satz (neues `Project.rate`) — konsumiert von CLI/TUI (Datei/Stdout) und WebUI (Download).

**Architecture:** Server-authoritative; reine Domain-Writer (CSV/JSON/Markdown) + ein `BuildExport`-Usecase, das gebuchte Sessions im Range nach Projekt aggregiert und `Project.rate` (Integer-Minor-Units) zu Σh×Satz verrechnet. Ein `GET /api/v1/export` streamt die Datei (`authAny` → Bearer **oder** Cookie, damit der WebUI-`<a download>` greift); `POST /api/v1/projects/{id}/rate` setzt den Satz. Clients rendern nur.

**Tech Stack:** Go, `pgx/v5`, goose (embedded migrations), `net/http`, `encoding/csv`+`encoding/json`, Cobra (CLI), templ+HTMX (WebUI), testcontainers (pgstore).

---

## Worktree & Branch

**Alle Code-Tasks im Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild` auf Branch `rebuild`** (HEAD aktuell `68342a9`, nach M1d). Plan-/Spec-Docs auf `main` — **nicht** ins `rebuild` committen. Modulpfad `github.com/serverkraken/flow`. Kleine fokussierte Commits pro Task; am Ende `make ci` grün inkl. Coverage-Gate **≥80%**.

## Datenquellen-Kontext (bestehend)

- `domain.Project{ID, OwnerID, Name, Slug, Color, Glyph, Status, CreatedAt, UpdatedAt}` (`internal/domain/project.go`). `pgstore.ProjectStore` (`Create`/`List`/`Get`) mit `scanProject` (9 Spalten) — Tabelle `projects` (Migration 0002).
- `ports.ProjectStore{Create, List, Get}`; `ports.ErrProjectNotFound`.
- `domain.WorkSession{ID, OwnerID, ProjectID *string, Tag, Note, Start time.Time, Stop *time.Time, CreatedAt}`, `Running()` = `Stop==nil`, `Elapsed(now)`.
- `ports.SessionStore.List(ctx, ownerID string, since time.Time) ([]WorkSession, error)` — Sessions mit `start_at >= since`, newest first.
- `httpserver.Server` (`internal/adapter/httpserver/server.go`): listet Usecases als Felder + `Routes()`; `writeJSON`, `userFrom`, `dayFmt="2006-01-02"`, `parseRange` (in dayoffs.go/worktime.go). Middleware: `s.auth` (Bearer/OIDC), `s.authAny` (Bearer **oder** Cookie, `webauth.go:118`), `s.webAuth` (Cookie-Page).
- CLI (`cmd/flow`, Cobra): `rootCmd()` in `main.go` mit `AddCommand`; Verben rufen `clientFromStore(cmd.Context())` → `*apiclient.Client`. Muster: `cmd/flow/dayoff.go`.
- `apiclient.Client` (`internal/adapter/apiclient/client.go`): `do(ctx, method, path, body, out)`; `CreateProject`/`ListProjects`; `domain.Project`-DTO.
- WebUI: templ-Pages mit `<X>Data`-Struct + `Page`/`Fragment`-Komponenten, Handler in `webui_<x>.go` (Muster `webui_stats.go`/`webui_dayoffs.go`). templ-Codegen: `go tool templ generate`; `make ci` prüft `verify-generate`.

---

## File Structure

**Neu (Domain):**
- `internal/domain/money.go` — `Money{Amount int64; Currency string}` + `Mul` + `String`.
- `internal/domain/export.go` — `ExportData`, `ProjectTotal`, `ExportRow` + `WriteCSV`/`WriteJSON`/`WriteMarkdown` + `fmtDur` Helper.

**Neu (Usecase):**
- `internal/usecase/export.go` — `BuildExport`.
- `internal/usecase/set_project_rate.go` — `SetProjectRate`.

**Neu (Adapter/HTTP/Client/CLI/WebUI):**
- `internal/adapter/httpserver/export.go` — `handleExport` + `handleSetProjectRate` + DTOs.
- `internal/adapter/apiclient/export.go` — `Export` + `SetProjectRate`.
- `cmd/flow/export.go` — `flow export`.
- `cmd/flow/project.go` — `flow project rate`.
- `internal/adapter/webui/export.templ` (+ generated `_templ.go`) — Export-Seite.
- `internal/adapter/httpserver/webui_export.go` — WebUI-Export-Handler.

**Geändert:**
- `internal/domain/project.go` — `Rate *Money`.
- `internal/ports/ports.go` — `ProjectStore.SetRate`.
- `internal/adapter/pgstore/migrations/0005_project_rate.sql` — neu.
- `internal/adapter/pgstore/projects.go` — `scanProject` + `Create`/`List`/`Get` um Rate; `SetRate`.
- `internal/adapter/apiclient/client.go` — `domain.Project` trägt `Rate` (DTO via domain) — **kein Change nötig**, `Project` ist DRY-DTO; nur sicherstellen Rate serialisiert.
- `internal/adapter/httpserver/server.go` — Felder + Routen + Nav.
- `internal/adapter/webui/*.templ` — Nav-Link „Export".
- `cmd/flow/main.go` — `AddCommand(exportCmd(), projectCmd())`.
- `cmd/flow-server/main.go` — Wiring.

---

## Task 1: Project.rate — Money domain + migration + store

**Files:**
- Create: `internal/domain/money.go`, `internal/adapter/pgstore/migrations/0005_project_rate.sql`
- Modify: `internal/domain/project.go`, `internal/ports/ports.go`, `internal/adapter/pgstore/projects.go`
- Test: `internal/domain/money_test.go`, `internal/adapter/pgstore/projects_test.go` (or the existing project test file)

- [ ] **Step 1: Money test (failing)**

Create `internal/domain/money_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestMoney_Mul(t *testing.T) {
	rate := domain.Money{Amount: 8000, Currency: "EUR"} // 80.00 EUR/h
	// 2h30m → 2.5h → 200.00 EUR → 20000 minor units
	got := rate.Mul(2*time.Hour + 30*time.Minute)
	if got.Amount != 20000 || got.Currency != "EUR" {
		t.Errorf("Mul: got %+v want {20000 EUR}", got)
	}
	// rounding: 1h20m = 1.3333h * 8000 = 10666.67 → 10667
	if g := rate.Mul(time.Hour + 20*time.Minute); g.Amount != 10667 {
		t.Errorf("rounding: got %d want 10667", g.Amount)
	}
}

func TestMoney_String(t *testing.T) {
	if s := (domain.Money{Amount: 480000, Currency: "EUR"}).String(); s != "4800.00 EUR" {
		t.Errorf("String: got %q want \"4800.00 EUR\"", s)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/domain/ -run TestMoney`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement Money**

Create `internal/domain/money.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// Money is an amount in integer minor units (e.g. cents) plus an ISO-4217
// currency. Used as a per-hour rate on Project and as the derived Σh×Satz
// amount in exports. Integer math avoids float rounding drift.
type Money struct {
	Amount   int64  `json:"amount"`   // minor units (cents)
	Currency string `json:"currency"` // ISO-4217, e.g. "EUR"
}

// Mul returns the cost of duration d at this per-hour rate, rounded to the
// nearest minor unit. (Amount is per-hour minor units.)
func (m Money) Mul(d time.Duration) Money {
	secs := int64(d / time.Second)
	total := (m.Amount*secs + 1800) / 3600 // round-half-up over 3600s/h
	return Money{Amount: total, Currency: m.Currency}
}

// String formats the amount as major.minor + currency, assuming a 2-decimal
// minor unit (the common case: EUR/USD/…). e.g. "4800.00 EUR".
func (m Money) String() string {
	a, sign := m.Amount, ""
	if a < 0 {
		sign, a = "-", -a
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, a/100, a%100, m.Currency)
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/domain/ -run TestMoney`
Expected: PASS.

- [ ] **Step 5: Project.Rate field**

In `internal/domain/project.go`, add to the `Project` struct after `Glyph`:
```go
	Rate *Money `json:"rate,omitempty"` // optional per-hour rate (nil = unset)
```

- [ ] **Step 6: Migration**

Create `internal/adapter/pgstore/migrations/0005_project_rate.sql`:
```sql
-- +goose Up
ALTER TABLE projects
    ADD COLUMN rate_amount   BIGINT,
    ADD COLUMN rate_currency TEXT;

-- +goose Down
ALTER TABLE projects
    DROP COLUMN rate_amount,
    DROP COLUMN rate_currency;
```

- [ ] **Step 7: Port — SetRate**

In `internal/ports/ports.go`, extend `ProjectStore`:
```go
type ProjectStore interface {
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	List(ctx context.Context, ownerID string) ([]domain.Project, error)
	Get(ctx context.Context, ownerID, id string) (domain.Project, error)
	// SetRate sets (rate != nil) or clears (rate == nil) the project's rate.
	SetRate(ctx context.Context, ownerID, id string, rate *domain.Money) error
}
```

- [ ] **Step 8: pgstore — read rate + SetRate**

In `internal/adapter/pgstore/projects.go`: extend every `SELECT`/`RETURNING` to add `rate_amount, rate_currency`, update `Create`'s INSERT to write them (NULL when `p.Rate==nil`), update `scanProject`, and add `SetRate`. Full replacement:

```go
func (s *ProjectStore) Create(ctx context.Context, p domain.Project) (domain.Project, error) {
	const q = `
INSERT INTO projects (id, owner_id, name, slug, color, glyph, status, created_at, updated_at, rate_amount, rate_currency)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id, owner_id, name, slug, color, glyph, status, created_at, updated_at, rate_amount, rate_currency`
	ra, rc := rateCols(p.Rate)
	return scanProject(s.pool.QueryRow(ctx, q,
		p.ID, p.OwnerID, p.Name, p.Slug, p.Color, p.Glyph, string(p.Status), p.CreatedAt, p.UpdatedAt, ra, rc))
}

func (s *ProjectStore) List(ctx context.Context, ownerID string) ([]domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at, rate_amount, rate_currency
FROM projects WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Get(ctx context.Context, ownerID, id string) (domain.Project, error) {
	const q = `
SELECT id, owner_id, name, slug, color, glyph, status, created_at, updated_at, rate_amount, rate_currency
FROM projects WHERE owner_id=$1 AND id=$2`
	p, err := scanProject(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, err
}

func (s *ProjectStore) SetRate(ctx context.Context, ownerID, id string, rate *domain.Money) error {
	ra, rc := rateCols(rate)
	const q = `UPDATE projects SET rate_amount=$1, rate_currency=$2, updated_at=now() WHERE owner_id=$3 AND id=$4`
	tag, err := s.pool.Exec(ctx, q, ra, rc, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set rate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrProjectNotFound
	}
	return nil
}

// rateCols maps an optional Money to the two nullable columns (both-or-neither).
func rateCols(m *domain.Money) (*int64, *string) {
	if m == nil {
		return nil, nil
	}
	a, c := m.Amount, m.Currency
	return &a, &c
}

func scanProject(r rowScanner) (domain.Project, error) {
	var p domain.Project
	var status string
	var ra *int64
	var rc *string
	if err := r.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.Color, &p.Glyph, &status, &p.CreatedAt, &p.UpdatedAt, &ra, &rc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, err
		}
		return domain.Project{}, fmt.Errorf("pgstore: scan project: %w", err)
	}
	p.Status = domain.ProjectStatus(status)
	if ra != nil && rc != nil {
		p.Rate = &domain.Money{Amount: *ra, Currency: *rc}
	}
	return p, nil
}
```
Keep the existing imports/`rowScanner` type. `Create` keeps its existing signature.

- [ ] **Step 9: pgstore rate round-trip test (failing → pass)**

Add to the existing project pgstore test file (find it: `rg -l "NewProjectStore" internal/adapter/pgstore/*_test.go`; if none, create `internal/adapter/pgstore/projects_test.go` using the same testcontainer pool helper as `user_settings_test.go`). Test: create a project (no rate → `Rate==nil` on Get), `SetRate(&Money{8000,"EUR"})` → Get shows it, `SetRate(nil)` → cleared, `SetRate` on unknown id → `ErrProjectNotFound`.

```go
func TestProjectStore_RateRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	st := pgstore.NewProjectStore(pool)
	ctx := context.Background()
	uid := seedUser(t, pool)
	p, err := st.Create(ctx, domain.Project{ID: "p1", OwnerID: uid, Name: "Acme", Slug: "acme", Status: domain.ProjectActive})
	if err != nil {
		t.Fatal(err)
	}
	if p.Rate != nil {
		t.Fatalf("fresh project should have nil rate")
	}
	if err := st.SetRate(ctx, uid, "p1", &domain.Money{Amount: 8000, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Get(ctx, uid, "p1")
	if got.Rate == nil || got.Rate.Amount != 8000 || got.Rate.Currency != "EUR" {
		t.Errorf("rate after set: got %+v", got.Rate)
	}
	if err := st.SetRate(ctx, uid, "p1", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(ctx, uid, "p1")
	if got.Rate != nil {
		t.Errorf("rate after clear should be nil, got %+v", got.Rate)
	}
	if err := st.SetRate(ctx, uid, "nope", &domain.Money{Amount: 1, Currency: "EUR"}); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("unknown id: want ErrProjectNotFound, got %v", err)
	}
}
```
Match the actual `newTestPool`/`seedUser` helper names in the package.

- [ ] **Step 10: Run + commit**

Run: `go test ./internal/domain/ ./internal/adapter/pgstore/ 2>&1 | tail -5` (pgstore needs Docker), `go build ./...`, `go vet ./internal/...`.
```bash
git add internal/domain/money.go internal/domain/money_test.go internal/domain/project.go internal/ports/ports.go internal/adapter/pgstore/migrations/0005_project_rate.sql internal/adapter/pgstore/projects.go internal/adapter/pgstore/projects_test.go
git commit -m "feat(export): Project.rate (Money minor-units) + pgstore round-trip"
```

---

## Task 2: Export domain — ExportData + CSV/JSON/Markdown writers

Pure, fully testable (high coverage value — M1d lesson: pure writers + handler happy-paths keep the gate green).

**Files:**
- Create: `internal/domain/export.go`
- Test: `internal/domain/export_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/export_test.go`:

```go
package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func sampleExport() domain.ExportData {
	d := func(h, m int) time.Time { return time.Date(2026, 6, 15, h, m, 0, 0, time.UTC) }
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	amt := rate.Mul(2 * time.Hour)
	return domain.ExportData{
		From: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		ByProject: []domain.ProjectTotal{
			{ProjectID: "p1", ProjectName: "Acme", Total: 2 * time.Hour, SessionCount: 1, Rate: &rate, Amount: &amt},
			{ProjectID: "p2", ProjectName: "Beta", Total: 30 * time.Minute, SessionCount: 1},
		},
		Sessions: []domain.ExportRow{
			{Date: d(9, 0), Start: d(9, 0), Stop: d(11, 0), Elapsed: 2 * time.Hour, ProjectName: "Acme", Tag: "deep", Note: "x"},
			{Date: d(13, 0), Start: d(13, 0), Stop: d(13, 30), Elapsed: 30 * time.Minute, ProjectName: "Beta"},
		},
	}
}

func TestWriteCSV(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteCSV(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "date,start,stop,duration_seconds,project,tag,note\n") {
		t.Errorf("header missing: %q", out)
	}
	if !strings.Contains(out, "2026-06-15,09:00,11:00,7200,Acme,deep,x") {
		t.Errorf("detail row missing: %q", out)
	}
}

func TestWriteJSON(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteJSON(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`"project": "Acme"`, `"totalSeconds": 7200`, `"amountMinor": 16000`, `"sessions"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json missing %q in %s", want, out)
		}
	}
}

func TestWriteMarkdown(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteMarkdown(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"# Worktime", "Acme", "2h 00m", "160.00 EUR", "## Sessions"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q in %s", want, out)
		}
	}
}

func TestWriteCSV_Empty(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteCSV(&b, domain.ExportData{}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "date,start,stop,duration_seconds,project,tag,note\n" {
		t.Errorf("empty CSV should be header only, got %q", b.String())
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/domain/ -run 'TestWrite'`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

Create `internal/domain/export.go`:

```go
package domain

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"
)

// ExportData is the rendered shape of a worktime export over [From, To]:
// a per-project aggregate plus the flat detail rows. Built by the export
// use case; serialised by the writers below.
type ExportData struct {
	From, To  time.Time
	ByProject []ProjectTotal
	Sessions  []ExportRow
}

// ProjectTotal is one project's aggregate. Amount = Rate.Mul(Total) when a
// rate is set, else nil.
type ProjectTotal struct {
	ProjectID    string
	ProjectName  string
	Total        time.Duration
	SessionCount int
	Rate         *Money
	Amount       *Money
}

// ExportRow is one booked session in the detail listing.
type ExportRow struct {
	Date    time.Time
	Start   time.Time
	Stop    time.Time
	Elapsed time.Duration
	ProjectName string
	Tag         string
	Note        string
}

// fmtDur renders a duration as "Hh MMm" (e.g. "2h 05m").
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// WriteCSV emits the detail session rows with a header. Pivot-friendly; the
// per-project aggregate is derivable in a spreadsheet (and lives in JSON/MD).
func WriteCSV(w io.Writer, d ExportData) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "start", "stop", "duration_seconds", "project", "tag", "note"})
	for _, r := range d.Sessions {
		_ = cw.Write([]string{
			r.Date.Format("2006-01-02"),
			r.Start.Format("15:04"),
			r.Stop.Format("15:04"),
			strconv.FormatInt(int64(r.Elapsed/time.Second), 10),
			r.ProjectName, r.Tag, r.Note,
		})
	}
	cw.Flush()
	return cw.Error()
}

// WriteJSON emits a structured object with the per-project aggregate and the
// detail rows.
func WriteJSON(w io.Writer, d ExportData) error {
	type projOut struct {
		Project      string `json:"project"`
		TotalSeconds int64  `json:"totalSeconds"`
		SessionCount int    `json:"sessionCount"`
		RateAmount   *int64 `json:"rateAmount,omitempty"`
		RateCurrency string `json:"rateCurrency,omitempty"`
		AmountMinor  *int64 `json:"amountMinor,omitempty"`
	}
	type rowOut struct {
		Date            string `json:"date"`
		Start           string `json:"start"`
		Stop            string `json:"stop"`
		DurationSeconds int64  `json:"durationSeconds"`
		Project         string `json:"project"`
		Tag             string `json:"tag"`
		Note            string `json:"note"`
	}
	out := struct {
		From      string    `json:"from"`
		To        string    `json:"to"`
		ByProject []projOut `json:"byProject"`
		Sessions  []rowOut  `json:"sessions"`
	}{From: d.From.Format("2006-01-02"), To: d.To.Format("2006-01-02")}
	for _, p := range d.ByProject {
		po := projOut{Project: p.ProjectName, TotalSeconds: int64(p.Total / time.Second), SessionCount: p.SessionCount}
		if p.Rate != nil {
			ra := p.Rate.Amount
			po.RateAmount = &ra
			po.RateCurrency = p.Rate.Currency
		}
		if p.Amount != nil {
			am := p.Amount.Amount
			po.AmountMinor = &am
		}
		out.ByProject = append(out.ByProject, po)
	}
	for _, r := range d.Sessions {
		out.Sessions = append(out.Sessions, rowOut{
			Date: r.Date.Format("2006-01-02"), Start: r.Start.Format("15:04"), Stop: r.Stop.Format("15:04"),
			DurationSeconds: int64(r.Elapsed / time.Second), Project: r.ProjectName, Tag: r.Tag, Note: r.Note,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteMarkdown emits a human-readable report: a per-project summary table
// (with Σh and amount), grand totals (hours always; amount per currency), and
// a detail session table.
func WriteMarkdown(w io.Writer, d ExportData) error {
	bw := &errWriter{w: w}
	bw.printf("# Worktime %s – %s\n\n", d.From.Format("2006-01-02"), d.To.Format("2006-01-02"))
	bw.printf("## Projekte\n\n| Projekt | Zeit | Betrag |\n|---|---|---|\n")
	var grandTotal time.Duration
	amountByCcy := map[string]int64{}
	for _, p := range d.ByProject {
		amt := "–"
		if p.Amount != nil {
			amt = p.Amount.String()
			amountByCcy[p.Amount.Currency] += p.Amount.Amount
		}
		bw.printf("| %s | %s | %s |\n", p.ProjectName, fmtDur(p.Total), amt)
		grandTotal += p.Total
	}
	bw.printf("\n**Summe:** %s", fmtDur(grandTotal))
	ccys := make([]string, 0, len(amountByCcy))
	for c := range amountByCcy {
		ccys = append(ccys, c)
	}
	sort.Strings(ccys)
	for _, c := range ccys {
		bw.printf(" · %s", Money{Amount: amountByCcy[c], Currency: c}.String())
	}
	bw.printf("\n\n## Sessions\n\n| Datum | Start | Stop | Dauer | Projekt | Tag | Notiz |\n|---|---|---|---|---|---|---|\n")
	for _, r := range d.Sessions {
		bw.printf("| %s | %s | %s | %s | %s | %s | %s |\n",
			r.Date.Format("2006-01-02"), r.Start.Format("15:04"), r.Stop.Format("15:04"),
			fmtDur(r.Elapsed), r.ProjectName, r.Tag, r.Note)
	}
	return bw.err
}

// errWriter swallows io errors until the end so the writer code stays flat.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/domain/ -run 'TestWrite'`
Expected: PASS. Also `go test ./internal/domain/` full + `go vet ./internal/domain/`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/export.go internal/domain/export_test.go
git commit -m "feat(export): ExportData + CSV/JSON/Markdown writers"
```

---

## Task 3: Export usecase — BuildExport + SetProjectRate

**Files:**
- Create: `internal/usecase/export.go`, `internal/usecase/set_project_rate.go`
- Test: `internal/usecase/export_test.go`

- [ ] **Step 1: Implement BuildExport + SetProjectRate**

Create `internal/usecase/export.go`:

```go
package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// BuildExport aggregates a user's booked (stopped) sessions in [from,to] by
// project into a domain.ExportData, resolving project names + rates. The
// running session is excluded. projectID "" means all projects.
type BuildExport struct {
	Sessions ports.SessionStore
	Projects ports.ProjectStore
	Clock    ports.Clock
	Loc      *time.Location
}

func (uc BuildExport) loc() *time.Location {
	if uc.Loc != nil {
		return uc.Loc
	}
	return time.Local
}

func (uc BuildExport) Execute(ctx context.Context, ownerID string, from, to time.Time, projectID string) (domain.ExportData, error) {
	loc := uc.loc()
	// inclusive day range [from 00:00, to+1d 00:00)
	lo := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	hi := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)

	sessions, err := uc.Sessions.List(ctx, ownerID, lo)
	if err != nil {
		return domain.ExportData{}, err
	}
	projects, err := uc.Projects.List(ctx, ownerID)
	if err != nil {
		return domain.ExportData{}, err
	}
	byID := make(map[string]domain.Project, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}

	data := domain.ExportData{From: lo, To: to}
	totals := map[string]*domain.ProjectTotal{}
	for _, s := range sessions {
		if s.Running() || s.ProjectID == nil { // exclude running + unbooked
			continue
		}
		start := s.Start.In(loc)
		if start.Before(lo) || !start.Before(hi) {
			continue
		}
		if projectID != "" && *s.ProjectID != projectID {
			continue
		}
		p := byID[*s.ProjectID]
		name := p.Name
		if name == "" {
			name = "(unbekannt)"
		}
		el := s.Stop.Sub(s.Start)
		data.Sessions = append(data.Sessions, domain.ExportRow{
			Date: start, Start: start, Stop: s.Stop.In(loc), Elapsed: el,
			ProjectName: name, Tag: s.Tag, Note: s.Note,
		})
		t, ok := totals[*s.ProjectID]
		if !ok {
			t = &domain.ProjectTotal{ProjectID: *s.ProjectID, ProjectName: name, Rate: p.Rate}
			totals[*s.ProjectID] = t
		}
		t.Total += el
		t.SessionCount++
	}
	for _, t := range totals {
		if t.Rate != nil {
			a := t.Rate.Mul(t.Total)
			t.Amount = &a
		}
		data.ByProject = append(data.ByProject, *t)
	}
	sort.Slice(data.ByProject, func(i, j int) bool { return data.ByProject[i].ProjectName < data.ByProject[j].ProjectName })
	sort.Slice(data.Sessions, func(i, j int) bool { return data.Sessions[i].Start.Before(data.Sessions[j].Start) })
	return data, nil
}
```

Create `internal/usecase/set_project_rate.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// SetProjectRate validates and stores (or clears) a project's per-hour rate.
type SetProjectRate struct {
	Projects ports.ProjectStore
}

func (uc SetProjectRate) Execute(ctx context.Context, ownerID, projectID string, rate *domain.Money) error {
	if rate != nil {
		if rate.Amount < 0 {
			return domain.ErrInvalidRate
		}
		if len(rate.Currency) != 3 {
			return domain.ErrInvalidRate
		}
	}
	return uc.Projects.SetRate(ctx, ownerID, projectID, rate)
}
```

Add to `internal/domain/errors.go`: `ErrInvalidRate = errors.New("invalid rate")`.

- [ ] **Step 2: Test (failing)**

Create `internal/usecase/export_test.go` (reuse the package's existing fakes — `rg -n "fakeSessionStore|FakeSessionStore|fakeProjectStore" internal/usecase internal/testutil` to find them; the StatsComputer test added a `fakeSessionStore` in `usecase_test`). If a project-store fake is missing, add a minimal one. Test BuildExport aggregates two sessions of one project, excludes the running one, computes amount from rate; and SetProjectRate rejects bad input.

```go
func TestBuildExport_AggregatesByProject(t *testing.T) {
	loc := time.UTC
	pid := "p1"
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	sessions := []domain.WorkSession{
		{ID: "a", ProjectID: &pid, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, loc))},
		{ID: "b", ProjectID: &pid, Start: time.Date(2026, 6, 15, 12, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 12, 30, 0, 0, loc))},
		{ID: "run", ProjectID: &pid, Start: time.Date(2026, 6, 15, 13, 0, 0, 0, loc), Stop: nil}, // excluded
	}
	uc := usecase.BuildExport{
		Sessions: fakeSessionStore{list: sessions},
		Projects: fakeProjectStore{list: []domain.Project{{ID: pid, Name: "Acme", Rate: &rate}}},
		Clock:    fixedClock{t: time.Date(2026, 6, 16, 0, 0, 0, 0, loc)},
		Loc:      loc,
	}
	data, err := uc.Execute(context.Background(), "u1", time.Date(2026, 6, 1, 0, 0, 0, 0, loc), time.Date(2026, 6, 30, 0, 0, 0, 0, loc), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sessions) != 2 {
		t.Fatalf("want 2 detail rows (running excluded), got %d", len(data.Sessions))
	}
	if len(data.ByProject) != 1 || data.ByProject[0].Total != 150*time.Minute {
		t.Fatalf("aggregate: got %+v", data.ByProject)
	}
	if data.ByProject[0].Amount == nil || data.ByProject[0].Amount.Amount != 20000 {
		t.Errorf("amount: got %+v want 20000 (2.5h*8000)", data.ByProject[0].Amount)
	}
}

func TestSetProjectRate_Validates(t *testing.T) {
	uc := usecase.SetProjectRate{Projects: fakeProjectStore{}}
	if err := uc.Execute(context.Background(), "u1", "p1", &domain.Money{Amount: -1, Currency: "EUR"}); err == nil {
		t.Error("negative amount should error")
	}
	if err := uc.Execute(context.Background(), "u1", "p1", &domain.Money{Amount: 1, Currency: "EU"}); err == nil {
		t.Error("bad currency should error")
	}
}
```
Define a `fakeProjectStore{list []domain.Project}` implementing `ports.ProjectStore` (Create/List/Get/SetRate) in the test file if absent. `ptr`/`fixedClock`/`fakeSessionStore` already exist in `usecase_test` (from M1d).

- [ ] **Step 3: Run — PASS**

Run: `go test ./internal/usecase/ -run 'TestBuildExport|TestSetProjectRate'`
Expected: PASS. Full `go test ./internal/usecase/ ./internal/domain/` + `go vet`.

- [ ] **Step 4: Commit**

```bash
git add internal/usecase/export.go internal/usecase/set_project_rate.go internal/usecase/export_test.go internal/domain/errors.go
git commit -m "feat(export): BuildExport (per-project aggregate) + SetProjectRate"
```

---

## Task 4: REST handlers + routes (export + set-rate)

**Files:**
- Create: `internal/adapter/httpserver/export.go`
- Modify: `internal/adapter/httpserver/server.go`
- Test: `internal/adapter/httpserver/export_test.go`

- [ ] **Step 1: Handlers**

Create `internal/adapter/httpserver/export.go`:

```go
package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// handleExport streams a worktime export in the requested format. authAny so a
// browser <a download> (cookie) and the CLI/TUI (bearer) both work.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		http.Error(w, "from/to required (yyyy-mm-dd)", http.StatusBadRequest)
		return
	}
	if to.Before(from) {
		http.Error(w, "to must be >= from", http.StatusBadRequest)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	data, err := s.BuildExport.Execute(r.Context(), u.ID, from, to, r.URL.Query().Get("project"))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	fname := fmt.Sprintf("flow-export-%s_%s", from.Format(dayFmt), to.Format(dayFmt))
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.csv"`)
		_ = domain.WriteCSV(w, data)
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.json"`)
		_ = domain.WriteJSON(w, data)
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`.md"`)
		_ = domain.WriteMarkdown(w, data)
	default:
		http.Error(w, "format must be csv, json or md", http.StatusBadRequest)
	}
}

type setRateReq struct {
	Amount   *int64 `json:"amount"`
	Currency string `json:"currency"`
}

func (s *Server) handleSetProjectRate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setRateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var rate *domain.Money
	if req.Amount != nil {
		rate = &domain.Money{Amount: *req.Amount, Currency: req.Currency}
	}
	err := s.SetProjectRate.Execute(r.Context(), u.ID, r.PathValue("id"), rate)
	switch {
	case errors.Is(err, domain.ErrInvalidRate):
		http.Error(w, "invalid rate", http.StatusBadRequest)
	case errors.Is(err, ports.ErrProjectNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 2: Server fields + routes**

In `internal/adapter/httpserver/server.go`, add to `Server`:
```go
	// m1e export
	BuildExport    usecase.BuildExport
	SetProjectRate usecase.SetProjectRate
```
In `Routes()`, after the stats routes:
```go
	mux.Handle("GET /api/v1/export", s.authAny(http.HandlerFunc(s.handleExport)))
	mux.Handle("POST /api/v1/projects/{id}/rate", s.auth(http.HandlerFunc(s.handleSetProjectRate)))
```

- [ ] **Step 3: Tests**

Create `internal/adapter/httpserver/export_test.go`, mirroring `stats_test.go`'s harness (`newStatsServer`-style with fakes — build a `usecase.BuildExport` with `testutil.NewFakeSessionStore()` + a fake/`testutil` project store seeded with one project + a session, and `usecase.SetProjectRate`). Tests:
- `TestHandleExport_CSV`: authed GET `/api/v1/export?from=2026-06-01&to=2026-06-30&format=csv` → 200, `Content-Type` text/csv, `Content-Disposition` contains `attachment`, body starts with the CSV header.
- `TestHandleExport_JSON` and `_MD`: 200 + right Content-Type + a body marker.
- `TestHandleExport_BadFormat`: `format=xml` → 400. `TestHandleExport_NoRange`: missing from/to → 400.
- `TestHandleSetProjectRate`: authed POST `/api/v1/projects/p1/rate` `{"amount":8000,"currency":"EUR"}` → 204; `{"amount":-1,...}` → 400; unknown id → 404.

If `testutil` lacks a project-store fake, add one (`testutil.NewFakeProjectStore()`) implementing `ports.ProjectStore` incl. `SetRate`, and seed it. Reuse the existing `newStatsServer` user/bearer/cookie wiring.

- [ ] **Step 4: Run + commit**

Run: `go test ./internal/adapter/httpserver/ ./internal/usecase/ ./internal/domain/ 2>&1 | tail -8`, `go vet ./internal/adapter/httpserver/`, `gofmt -w internal/adapter/httpserver/export.go`.
```bash
git add internal/adapter/httpserver/export.go internal/adapter/httpserver/server.go internal/adapter/httpserver/export_test.go internal/testutil/fakes.go
git commit -m "feat(export): REST GET /export (csv/json/md, authAny) + POST projects/{id}/rate"
```

---

## Task 5: apiclient — Export + SetProjectRate

**Files:**
- Create: `internal/adapter/apiclient/export.go`
- Test: `internal/adapter/apiclient/export_test.go`

- [ ] **Step 1: Client methods**

Create `internal/adapter/apiclient/export.go`:

```go
package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Export fetches a worktime export. format is "csv"|"json"|"md"; projectID ""
// means all projects. Returns the raw bytes of the chosen format.
func (c *Client) Export(ctx context.Context, from, to, format, projectID string) ([]byte, error) {
	q := url.Values{"from": {from}, "to": {to}, "format": {format}}
	if projectID != "" {
		q.Set("project", projectID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/export?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apiclient: export status %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// SetProjectRate sets (amount != nil) or clears (amount == nil) a project's
// per-hour rate in minor units.
func (c *Client) SetProjectRate(ctx context.Context, projectID string, amount *int64, currency string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/rate",
		map[string]any{"amount": amount, "currency": currency}, nil)
}
```

(`c.base`, `c.hc`, `c.do` exist in `client.go`. `domain.Project` already carries `Rate` from Task 1, so `ListProjects` decodes it — no client DTO change needed.)

- [ ] **Step 2: Test**

Create `internal/adapter/apiclient/export_test.go` mirroring `stats_test.go` (httptest server). Assert `Export` sends `from/to/format` query and returns the body bytes; `SetProjectRate` POSTs the right body to `/api/v1/projects/p1/rate`.

```go
func TestExport_FetchesBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/export" || r.URL.Query().Get("format") != "md" {
			t.Errorf("bad request: %s ?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("# Worktime\n"))
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tok")
	b, err := c.Export(context.Background(), "2026-06-01", "2026-06-30", "md", "")
	if err != nil || string(b) != "# Worktime\n" {
		t.Fatalf("got %q err %v", b, err)
	}
}
```

- [ ] **Step 3: Run + commit**

Run: `go test ./internal/adapter/apiclient/`, `go vet`, `gofmt -w`.
```bash
git add internal/adapter/apiclient/export.go internal/adapter/apiclient/export_test.go
git commit -m "feat(export): apiclient Export + SetProjectRate"
```

---

## Task 6: CLI verbs — flow export + flow project rate

**Files:**
- Create: `cmd/flow/export.go`, `cmd/flow/project.go`
- Modify: `cmd/flow/main.go`

- [ ] **Step 1: export verb**

Create `cmd/flow/export.go`:

```go
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func exportCmd() *cobra.Command {
	var from, to, format, project string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "export worktime (per project) for a range as csv|json|md to stdout",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// --project takes a slug for UX; the API filters by project id.
			projectID := ""
			if project != "" {
				projects, err := c.ListProjects(cmd.Context())
				if err != nil {
					return err
				}
				for _, p := range projects {
					if p.Slug == project {
						projectID = p.ID
					}
				}
				if projectID == "" {
					return fmt.Errorf("no project with slug %q", project)
				}
			}
			b, err := c.Export(cmd.Context(), from, to, format, projectID)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(b)
			return err
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&to, "to", "", "end date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&format, "format", "csv", "csv|json|md")
	cmd.Flags().StringVar(&project, "project", "", "optional project slug filter")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
```

(`fmt` must be imported in `cmd/flow/export.go` — add it alongside `os`.)

- [ ] **Step 2: project rate verb**

Create `cmd/flow/project.go`:

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "manage projects"}
	cmd.AddCommand(projectRateCmd())
	return cmd
}

func projectRateCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "rate <slug> [<amount-minor> <currency>]",
		Short: "set or clear a project's per-hour rate (amount in minor units, e.g. 8000 = 80.00)",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			var id string
			for _, p := range projects {
				if p.Slug == args[0] {
					id = p.ID
				}
			}
			if id == "" {
				return fmt.Errorf("no project with slug %q", args[0])
			}
			if clear {
				return c.SetProjectRate(cmd.Context(), id, nil, "")
			}
			if len(args) != 3 {
				return fmt.Errorf("usage: flow project rate <slug> <amount-minor> <currency>  (or --clear)")
			}
			amount, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("amount must be an integer (minor units): %w", err)
			}
			return c.SetProjectRate(cmd.Context(), id, &amount, args[2])
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "clear the rate instead of setting it")
	return cmd
}
```

- [ ] **Step 3: Wire into rootCmd**

In `cmd/flow/main.go` `rootCmd()`, add: `root.AddCommand(exportCmd())` and `root.AddCommand(projectCmd())`.

- [ ] **Step 4: Build + commit**

Run: `go build ./... && go vet ./cmd/flow/ && gofmt -w cmd/flow/export.go cmd/flow/project.go`. (CLI verbs are thin; the repo's `cmd/flow` tests are minimal — a build + vet is the gate here. If `cmd/flow` has a test harness for verbs, add a smoke test mirroring it.)
```bash
git add cmd/flow/export.go cmd/flow/project.go cmd/flow/main.go
git commit -m "feat(export): flow export + flow project rate CLI verbs"
```

---

## Task 7: WebUI — export page + download links + summary preview + nav

Mirror the M1d Stats WebUI (`stats.templ` + `webui_stats.go`). Download links are direct `<a download href="/api/v1/export?…&format=…">` (cookie auth via `authAny`).

**Files:**
- Create: `internal/adapter/webui/export.templ` (+ generated `_templ.go`)
- Create: `internal/adapter/httpserver/webui_export.go`
- Modify: `internal/adapter/httpserver/server.go` (routes), `internal/adapter/webui/{worktime,dayoffs,stats}.templ` (nav link)
- Test: `internal/adapter/httpserver/webui_export_test.go`

- [ ] **Step 1: templ page**

Create `internal/adapter/webui/export.templ` with an `ExportData`-style view struct (name it `ExportPageData` to avoid colliding with `domain.ExportData`) holding: `User string`, the current `From`/`To` strings, and the per-project summary rows (`[]ExportSummaryRow{Project, Time, Amount string}`) + grand totals. Components: `ExportPage(d ExportPageData)` (full page, same layout as `StatsPage`) and `ExportFragment(d ExportPageData)` (the summary preview, HTMX-swappable). The page has: a from/to form (presets KW/Monat/letzter Monat + free date inputs) that GETs `/ui/export/preview?from=&to=` to refresh the summary, three direct download links `<a download href={ "/api/v1/export?from=" + d.From + "&to=" + d.To + "&format=csv" }>CSV</a>` (+ json, md), and the summary table. Mirror `stats.templ`'s layout/nav/classes precisely. Add the nav header consistent with the other pages.

- [ ] **Step 2: nav link on the other pages**

In `worktime.templ`, `dayoffs.templ`, `stats.templ` nav headers, add an `<a href="/export">export</a>` link (same markup/casing as the existing nav links) so Export is reachable everywhere (M1d nav-symmetry precedent).

- [ ] **Step 3: generate templ**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go tool templ generate`. Commit the generated `*_templ.go`.

- [ ] **Step 4: WebUI handlers**

Create `internal/adapter/httpserver/webui_export.go` mirroring `webui_stats.go`: `exportPageData(ctx, u, from, to) (webui.ExportPageData, error)` calls `s.BuildExport.Execute` and formats the summary rows (`fmtDur`-style + `Money.String()`); default range = current month when from/to empty. Handlers `handleWebExportHome` (full page) + `handleWebExportPreview` (fragment). No mutation, no SSE.

- [ ] **Step 5: routes**

In `server.go` `Routes()`:
```go
	mux.Handle("GET /export", s.webAuth(http.HandlerFunc(s.handleWebExportHome)))
	mux.Handle("GET /ui/export/preview", s.webAuth(http.HandlerFunc(s.handleWebExportPreview)))
```

- [ ] **Step 6: test + generate + commit**

Create `internal/adapter/httpserver/webui_export_test.go` (mirror `webui_stats_test.go` cookie harness): GET `/export` → 200 + body contains "export"/"Projekt"; GET `/ui/export/preview?from=&to=` → 200.
Run: `go tool templ generate && go build ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/webui/ 2>&1 | tail -6 && go vet ./internal/adapter/httpserver/ && git diff --quiet -- ':*_templ.go' && echo "templ in sync"`.
```bash
git add internal/adapter/webui/export.templ internal/adapter/webui/export_templ.go internal/adapter/httpserver/webui_export.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_export_test.go
git add -A internal/adapter/webui/   # regenerated nav _templ.go for worktime/dayoffs/stats
git commit -m "feat(webui): export page (download links + summary preview) + nav link"
```

---

## Task 8: Wiring + curl smoke + make ci

**Files:**
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Composition root**

In `cmd/flow-server/main.go`, in the `srv := &httpserver.Server{...}`, add (after the M1d fields):
```go
		BuildExport: usecase.BuildExport{
			Sessions: sessionStore,
			Projects: projectStore,
			Clock:    clock,
			Loc:      time.Local,
		},
		SetProjectRate: usecase.SetProjectRate{Projects: projectStore},
```

- [ ] **Step 2: Build + vet + gofmt + templ sync**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
gofmt -w cmd/flow-server/main.go
go build ./... && go vet ./...
go tool templ generate && git diff --quiet -- ':*_templ.go' && echo "templ in sync"
```
Expected: clean.

- [ ] **Step 3: curl smoke (dev stack)**

Run (Muster `reference_flow_dev_env`):
```bash
make dev-up && make dev-run &
TOKEN=$(make dev-token); BASE=http://localhost:8080
# create a project + book a session so the export has data (or reuse existing)
PID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":"Acme","slug":"acme"}' "$BASE/api/v1/projects" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "{\"amount\":8000,\"currency\":\"EUR\"}" -o /dev/null -w 'set-rate → %{http_code}\n' "$BASE/api/v1/projects/$PID/rate"
for f in csv json md; do echo "=== export $f ==="; curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/export?from=2026-06-01&to=2026-06-30&format=$f" | head -c 300; echo; done
echo "bad format (expect 400):"; curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/export?from=2026-06-01&to=2026-06-30&format=xml"
```
Expected: set-rate 204; each format returns its body; xml → 400.

- [ ] **Step 4: make ci (coverage ≥80%)**

Run: `make ci`
Expected: green incl. coverage gate. The pure writers (Task 2) + usecase tests (Task 3) + handler happy-paths (Task 4) carry the coverage; if the gate dips below 80%, add happy-path tests for the export handlers / BuildExport branches (M1d lesson — do NOT lower the threshold).

- [ ] **Step 5: Done-Gate manuell**

- `flow project rate acme 8000 EUR` (CLI) → `flow export --from 2026-06-01 --to 2026-06-30 --format md` zeigt Acme mit Σh **und** Betrag.
- Dieselbe Range als CSV (`--format csv > /tmp/x.csv`) in Numbers/Excel → Detail-Zeilen.
- WebUI `/export`: Range wählen → Summary-Vorschau zeigt Beträge; CSV/JSON/MD-Download-Links laden die Dateien (Cookie-Auth via `authAny`).

- [ ] **Step 6: Commit + verify HEAD**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(export): wire BuildExport + SetProjectRate into composition root"
git log --oneline -10 && git status
```
Expected: alle M1e-Commits auf `rebuild`, Worktree clean. (Lesson [[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen, finales Wiring selbst fahren.)

---

## Self-Review-Notiz (vom Planautor)

**Spec-Coverage:** `Project.rate` Money+Migration+Store (T1) ✓, Export-Writer CSV/JSON/MD (T2) ✓, `BuildExport` Aggregat+running-exclude+Filter (T3) ✓, `SetProjectRate`-Validierung (T3) ✓, REST `/export` (authAny) + `/projects/{id}/rate` (T4) ✓, apiclient (T5) ✓, CLI `flow export` + `flow project rate` (T6) ✓, WebUI Export-Seite + Download + Preview + Nav (T7) ✓, Wiring + curl + make-ci-Coverage (T8) ✓.

**Bewusste Defaults (aus den offenen Spec-Punkten):** `flow project rate <amount>` nimmt **Minor-Units** (8000 = 80,00). Gemischte Währungen → **getrennte Summenzeilen je Währung** im Markdown. WebUI-Download = direkter `<a download>` (kein Proxy, `authAny`-Cookie). `SessionStore.List(since)` + Range-Filter im Usecase (kein neues `ListRange`).
