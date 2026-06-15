# flow Rebuild · M1e — Zeit-Export pro Projekt/Zeitraum · Design

**Datum:** 2026-06-15
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Scope:** Dritter und letzter M1c-Folgeslice. Server-berechneter Worktime-Export über einen Datumsbereich, **pro Projekt aggregiert + Detail-Zeilen**, in **CSV / JSON / Markdown**, inklusive **Σh×Satz-Insight** je Projekt (neues `Project.rate`). Konsumiert von CLI/TUI (Datei) + WebUI (Download). Hält die slice-spezifischen Entscheidungen fest; Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md`, Worktime-Spine siehe `2026-06-14-flow-rebuild-m1a-worktime-design.md`, Stats/Tagesziel siehe `2026-06-15-flow-rebuild-m1d-stats-design.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Milestone); Planungs-Docs auf `main` (M0/M1a/M1c/M1d-Präzedenz).

## Warum dieser Schnitt (Slicing)

Die M1c-Spec teilte das ursprüngliche M1c-Bündel in drei Schnitte: M1c (DayOff+Feiertage+ICS, erledigt), M1d (Stats+Burndown+Tagesziel, erledigt), **M1e (dieses Dokument: Zeit-Export pro Projekt/Zeitraum)**. M1e konsumiert, was M1a/M1d liefern: gebuchte Sessions (M1a) + die first-class Projekte (M0). Der neue Winkel gegenüber v1 (`main`): v1 exportierte einen **flachen Session-Dump nach Tag** (kein Projekt, nur Tags) — der Rebuild nutzt `projectId` auf Sessions und aggregiert **pro Projekt**, plus optional Σh×Satz über einen neuen `Project.rate`.

## Done-Gate (Akzeptanztest)

> Stundensatz für ein Projekt via CLI setzen (`flow project rate <slug> 8000 EUR`) → `GET /api/v1/export?format=md&from=…&to=…` zeigt das Projekt mit Σh **und** Σh×Satz; dieselbe Range als CSV in Excel/Numbers geöffnet zeigt die Detail-Session-Zeilen; die WebUI-Export-Seite lädt alle drei Formate herunter und zeigt die Projekt-Summary-Tabelle mit Beträgen.

## Scope / Non-Goals

**In Scope:**
- **`Project.rate`** (optionaler Stundensatz) — Migration + Domain + Store + Setzen via REST + CLI-Verb. Read-only Insight, kein Billing.
- **Export-Usecase**: gebuchte Sessions in `[from,to]` → pro-Projekt-Aggregat (Σh, Session-Count, Σh×Satz) + Detail-Zeilen.
- **Drei Format-Writer** (rein, je idiomatisch): CSV (Detail), JSON (Aggregat+Detail strukturiert), Markdown (menschenlesbarer Report).
- **REST**: `GET /api/v1/export` (streamt Datei) + `POST /api/v1/projects/{id}/rate`.
- **CLI/TUI**: `flow export` (Datei/Stdout) + `flow project rate` (Satz setzen); TUI-Export-Affordance.
- **WebUI**: Export-Seite (Datumsbereich + Download-Buttons + Summary-Vorschau).

**Non-Goals (vertagt / bewusst draußen):**
- **Kein Billing/Rechnungswesen** — `rate` ist reines Insight (Σh×Satz), keine Rechnungen/Steuern/Positionen.
- **Keine vollständige Projekt-Management-UI** — Satz wird via CLI/API gesetzt (Rebuild hat keine Projekt-Edit-Seite; eine baut M1e *nicht*). Export zeigt den Betrag, das Setzen läuft über CLI/API.
- **Kein User-Default-Satz** — nur per-Projekt-`rate`. (Master-Design nannte `User.defaultRate`; vertagt — Projekte ohne Satz zeigen nur Stunden.)
- **Laufende Session ausgeschlossen** — Export deckt nur gestoppte (gebuchte) Sessions; eine laufende ist unvollständig.
- **Kein SSE/Live** — Export ist Pull/on-demand.
- **Kein Tag-basiertes Aggregat** als zweite Dimension — `tag` erscheint nur als Spalte in den Detail-Zeilen (kein Tag-Erfassungs-Flow; siehe [[project_flow_rebuild_m1d]]). Aggregiert wird nach Projekt.
- Pixelgenaue UI-Politur → später.

## Kern-Entscheidungen (Brainstorm 2026-06-15)

| Frage | Entscheidung |
|---|---|
| Export-Inhalt | **Aggregat (pro Projekt) + Detail-Zeilen.** |
| Formate | **CSV + JSON + Markdown.** |
| Format-Aufteilung | **CSV = nur Detail** (Pivot-tauglich); **JSON + Markdown = Aggregat + Detail** (je idiomatisch). |
| Rate-Insight | **Drin** — neues optionales `Project.rate` {amount, currency}; Σh×Satz im Aggregat. |
| Rate-Setzen | **CLI/API-first** (`flow project rate` + `POST …/rate`); keine neue Projekt-UI. |
| Geld-Repräsentation | **Integer-Minor-Units** (z.B. Cent pro Stunde) + ISO-4217-Currency; kein Float. |
| Laufende Session | **Ausgeschlossen** (nur gebuchte/gestoppte Sessions). |
| Range-Semantik | Sessions, deren `Start` (in lokaler Zone) in `[from, to]` fällt — konsistent mit dem M1d-TZ-Fix (`now.Location()`). |
| Live-Sync | **Keiner** (on-demand Pull). |

## Datenmodell-Delta — `Project.rate`

Neue Migration `migrations/0005_project_rate.sql` (inkrementell, embedded — goose wie M0/M1a/M1c/M1d):

```sql
-- +goose Up
ALTER TABLE projects
    ADD COLUMN rate_amount   BIGINT,  -- Minor-Units pro Stunde (z.B. Cent), NULL = kein Satz
    ADD COLUMN rate_currency TEXT;    -- ISO-4217 (z.B. 'EUR'), NULL = kein Satz

-- +goose Down
ALTER TABLE projects
    DROP COLUMN rate_amount,
    DROP COLUMN rate_currency;
```

**Invariante:** both-or-neither — entweder beide `rate_*` gesetzt (Satz aktiv) oder beide NULL (kein Satz). Erzwungen im Usecase/Store, nicht per CHECK (einfachheitshalber).

Domain (`internal/domain/`):
- `money.go` (neu): `Money{Amount int64 /* Minor-Units */, Currency string}`; `Mul(d time.Duration) Money` → `round(Amount × Stunden)` als Minor-Units via Integer-Mathematik `(Amount*seconds + 1800) / 3600` (rund auf nächste Minor-Unit); `String()`/Formatierung (z.B. `4.800,00 EUR`).
- `project.go` (erweitern): `Rate *Money json:"rate,omitempty"`. `nil` = kein Satz.

## Domain & Usecase

**Neu — `internal/domain/export.go`** (an v1 `main:internal/domain/export.go` angelehnt, aber **projekt-aware** — v1s `Session`-Shape kennt kein Projekt, also Neubau statt 1:1):
- `ExportData{From, To time.Time; ByProject []ProjectTotal; Sessions []ExportRow}`.
- `ProjectTotal{ProjectID, ProjectName string; Total time.Duration; SessionCount int; Rate *Money; Amount *Money}` — `Amount = Rate.Mul(Total)` wenn `Rate != nil`, sonst `nil`.
- `ExportRow{Date, Start, Stop time.Time; Elapsed time.Duration; ProjectName, Tag, Note string}`.
- Reine Writer: `WriteCSV(w io.Writer, d ExportData) error` (Detail-Zeilen, Header `date,start,stop,duration_seconds,project,tag,note`), `WriteJSON(w, d)` (`{from,to,byProject:[…],sessions:[…]}`, Beträge als `amountMinor`+`currency`), `WriteMarkdown(w, d)` (`# Worktime <range>`, Projekt-Summary-Tabelle `Projekt · Σh · Betrag`, Gesamtsumme(n), Detail-Tabelle).
- `ByProject` nach `ProjectName` sortiert. Grand-Total Stunden immer; Grand-Total Betrag **pro Währung** (Map Currency→Σ), da gemischte Sätze möglich.

**Neu — `internal/usecase/export.go`**: `BuildExport`
- Felder: `Sessions ports.SessionStore`, `Projects ports.ProjectStore`, `Clock ports.Clock`, `Loc *time.Location`.
- `Execute(ctx, ownerID string, from, to time.Time, projectID string /* "" = alle */) (domain.ExportData, error)`:
  1. Sessions im Range laden (gestoppt; laufende ausschließen). Filter nach lokalem `Start`-Tag in `[from,to]`.
  2. Projekte des Users laden → Map `id → (name, rate)`.
  3. Optionaler `projectID`-Filter.
  4. Pro Projekt aggregieren (Σ Elapsed, Count, `Amount = rate.Mul(Σ)`), Detail-Zeilen mit aufgelöstem Projektnamen bauen.

**Neu — `usecase/set_project_rate.go`**: `SetProjectRate{Projects ports.ProjectStore}` — `Execute(ctx, owner, projectID string, rate *domain.Money) error`; validiert `amount >= 0` + 3-Letter-Currency wenn gesetzt; `nil` löscht den Satz.

**Port-Erweiterung** (`internal/ports/ports.go`): `ProjectStore.SetRate(ctx, ownerID, id string, rate *domain.Money) error`. (`Get`/`List` liefern `Rate` mit, sobald die Spalten gelesen werden.) `SessionStore.List(owner, since)` existiert (M1a) — für den Range ggf. eine `ListRange`-Variante oder seit `from` laden + clientseitig auf `to` filtern (Plan-Detail; für Einzeluser unkritisch).

**pgstore** (`internal/adapter/pgstore/projects.go`): `Create`/`Get`/`List` um `rate_amount`/`rate_currency` erweitern (NULL → `Rate=nil`); neuer `SetRate` (both-or-neither, NULL bei Clear).

## HTTP-Routen

| Methode | Pfad | Auth | Zweck |
|---|---|---|---|
| `GET` | `/api/v1/export?from=&to=&format=csv\|json\|md&project=<id?>` | **`authAny`** (Bearer **oder** Cookie) | streamt die Export-Datei |
| `POST` | `/api/v1/projects/{id}/rate` | OIDC/Bearer | Satz setzen `{amount, currency}` / `{amount:null}` (clear) |

**Auth-Hinweis:** Der Export läuft über `s.authAny` (akzeptiert Bearer **und** Session-Cookie, wie die bestehende `/api/v1/events`-Route), damit der WebUI-Download per `<a download href="/api/v1/export?…">` mit dem Session-Cookie funktioniert und CLI/TUI per Bearer. So braucht es keinen zweiten WebUI-Proxy-Endpoint.

Export-Handler: `Content-Type` je Format (`text/csv`, `application/json`, `text/markdown; charset=utf-8`) + `Content-Disposition: attachment; filename="flow-export-<from>_<to>.<ext>"`. `format` default `csv`. 400 bei ungültigem `format`/`from`/`to` (`from`/`to` Pflicht, yyyy-mm-dd, `to >= from`). Owner-scoped; `project` optional.

Rate-Handler: `{amount int64|null, currency string}`; 400 bei negativem Amount oder fehlender/ungültiger Currency wenn Amount gesetzt; 404 wenn Projekt nicht dem User gehört. Kein neues SSE-Event (Export ist Pull; Rate-Anzeige ist nicht live-synced).

## CLI / TUI

- **CLI** (`cmd/flow`, Cobra, dünner apiclient-Aufrufer):
  - `flow export --from <d> --to <d> [--format csv|json|md] [--project <slug>]` → streamt nach **Stdout** (pipe-freundlich: `flow export … > report.csv`).
  - `flow project rate <slug> <amount> <currency>` setzt den Satz (`amount` als Minor-Units oder als Dezimal — Plan-Detail, Default Minor-Units); `flow project rate <slug> --clear` löscht. Slug→ID via `ListProjects`.
- **TUI**: Export-Affordance (Key/Command) im Worktime/Stats-Screen → Range (Default aktuelle KW/Monat) + Format wählen → `apiclient.Export` → in eine Datei schreiben (z.B. `~/Downloads/flow-export-<range>.<ext>`) + Pfad anzeigen. Bewusst schlicht; Rate-Setzen bleibt CLI/API.
- **apiclient**: `Export(ctx, from, to, format, projectID) (io.ReadCloser /* oder []byte */, error)`, `SetProjectRate(ctx, id string, rate *Money) error`, und `Project`-DTO um `Rate` erweitern.

## WebUI

- `export.templ` Seite: Datumsbereich (Presets KW/Monat/letzter Monat + freie `from`/`to`-Felder) + drei Download-Links/Buttons (CSV/JSON/MD, die je `format` auf `/api/v1/export` zeigen — `hx-boost="false"` bzw. echte `<a download>` für den Datei-Download) + eine **Vorschau-Tabelle** der Projekt-Summary (Projekt · Σh · Betrag) per HTMX-Fragment. Nav-Link „Export" konsistent zu Worktime/DayOffs/Stats (Symmetrie wie M1d-Fix).
- Rendering bewusst schlicht (Politur vertagt). Kein SSE.

## Error-Handling

- `from`/`to` fehlt/kaputt oder `to < from` → 400.
- `format` ∉ {csv,json,md} → 400.
- `project` unbekannt/fremd → leeres Aggregat (oder 404 — Plan-Detail; Default: leeres Ergebnis, kein Leak).
- Rate `amount < 0` → 400; Currency fehlt bei gesetztem Amount → 400; ungültiger Currency-Code → 400.
- Leerer Range (keine Sessions) → valider Export mit leerem `byProject`/`sessions` (CSV nur Header, MD „keine Einträge").
- Gemischte Währungen → Grand-Total je Währung getrennt; kein Misch-Summen-Fehler.

## Testing

- **Domain:** `Money.Mul` (Rundung, Null-Rate), `WriteCSV`/`WriteJSON`/`WriteMarkdown` (Format, Header, Aggregat-Zeilen, gemischte Währungen, leerer Range), Aggregations-Korrektheit.
- **Usecase:** `BuildExport` (Aggregat je Projekt, laufende Session ausgeschlossen, Projekt-Filter, Owner-Scoping, Rate-Resolve/Σh×Satz), `SetProjectRate` (Validierung, Clear, both-or-neither).
- **pgstore:** `projects` Rate round-trip (both-or-neither, NULL), `SetRate`, Clear.
- **httpserver:** Export-Handler je Format (Content-Type + Content-Disposition + Body-Marker), 400-Fälle, Rate-Handler (200/400/404), Owner-Isolation.
- **CLI:** `flow export` schreibt das richtige Format nach Stdout; `flow project rate` setzt/löscht.
- **Done-Gate manuell:** wie oben (CLI-Satz → MD-Export mit Betrag → CSV in Excel → WebUI-Downloads + Summary).

## Wiring-Verification (Pflicht-Abschlusstask)

Letzter Plan-Task (Lesson [[feedback_plan_main_wiring_task]]): Composition-Root (`cmd/flow-server/main.go`) verdrahtet `BuildExport`/`SetProjectRate` + Handler; `cmd/flow` verdrahtet die neuen Verben; curl-Smoke trifft **jede** neue Route (`/api/v1/export` je Format + `/api/v1/projects/{id}/rate`); `make ci` grün inkl. Coverage-Gate **≥80%** (M1d-Lehre: neue Handler brauchen Happy-Path-Tests, sonst drückt der Gate).

## Offene Punkte (für die Plan-Phase)

- `flow project rate <amount>`: Minor-Units vs. Dezimal-Eingabe (z.B. `80` vs `8000`) — Default Minor-Units, Dezimal-Komfort optional.
- Grand-Total-Darstellung bei gemischten Währungen im Markdown (mehrere Summenzeilen vs. nur per-Projekt).
- `SessionStore.ListRange` ergänzen vs. `List(since)`+Filter im Usecase.
- Exakter Default-Dateiname/-Pfad beim TUI-Export.
- Vorschau-Tabelle der WebUI: eigenes HTMX-Fragment (`/ui/export/preview`) vs. direkt in die Export-Seite gerendert (Download-Links sind ohnehin direkte `<a download>` auf `/api/v1/export`, Auth via `authAny`-Cookie — siehe Auth-Hinweis).
