# flow WebUI Slice 1 — Worktime (Heute · Woche · Historie) — Design Spec

- **Datum:** 2026-06-24
- **Branch:** `rebuild`
- **Status:** Approved — bereit für writing-plans
- **Baut auf:** [[2026-06-23-flow-webui-overhaul-design]] (Parent-Spec; §5.1–5.3, §9, §12, §15 Slice 1) + Slice 0 (Fundament: `internal/adapter/webui/components`, i18n, AppShell, Dialog/ConfirmDialog/Pagination — fertig + gepusht auf `rebuild`).
- **Mockups (Referenz):** `docs/superpowers/specs/assets/2026-06-23-webui/` — `direction-b-studio.html` (Heute), `studio-worktime-week.html` (Woche), `studio-worktime-calendar.html` (Historie).

---

## 1. Ziel & Scope

Slice 1 baut die **Worktime-Triade** auf der neuen AppShell + Slice-0-Komponenten-Bibliothek:

- **`/` Heute** — vom alten pre-Slice-0-Worktime auf AppShell **portiert**.
- **`/woche` Woche** — neu.
- **`/historie` Historie** — neu; **Kern des Import-Cleanups** (Kalender + Liste + Bulk-Neuzuweisung).

**Außerhalb des Scopes** (bleibt vorerst altes Chrome bis Slice 4): `/stats`, `/frei`, `/export`. Die Worktime-Sub-Nav (`TabStrip`) umfasst alle fünf Bereiche; Links zu Stats/Frei führen während Slice 1 in altes Chrome (Parent-Spec §16: Alt/Neu koexistieren, Surface-für-Surface ersetzt).

### Erfolgskriterien
1. Heute/Woche/Historie sind eigene Routen auf AppShell, Dark/Light, responsive (Desktop dicht, Mobile echtes Layout), deutsch via i18n.
2. Historie zeigt importierte „ohne Projekt"-Sessions prominent und erlaubt **auswahl-basierte Bulk-Neuzuweisung** an ein (auch neu angelegtes) Projekt sowie **Bulk-Löschen** (mit Bestätigung).
3. Live: Heute tickt client-seitig; alle drei Flächen aktualisieren sich SSE-getrieben bei `session.*`/`project.*` (Woche zusätzlich `dayoff.changed`).
4. Sessions-Endpoint paginiert (`?limit&offset` + Gesamtanzahl); Historie hat eine paginierte flache „Alle Sitzungen"-Liste.
5. `make ci` grün (inkl. `verify-css`); Live-Done-Gate gegen Postgres+Dex.

### Non-Goals (YAGNI)
- Kein Drag-to-resize von Kalender-Blöcken (Zeiten via Edit-Dialog) — Parent-Spec.
- Keine Browser-Popups (`window.alert/confirm/prompt`) — alles über `Dialog`/`ConfirmDialog`.
- Keine Änderung an Stats/Frei/Export in diesem Slice.
- Kein neues Auth-Modell; WebUI nutzt Cookie-Session, REST Bearer-oder-Cookie.

---

## 2. Entscheidungen (in diesem Brainstorm getroffen)

| Entscheidung | Wahl | Begründung |
|---|---|---|
| Historie-Zeitraster | **Hybrid**: Default-Band (06–20), pro Woche auto-ausdehnen auf min(start)/max(stop), auf volle Stunde gesnapped | Zeigt immer alle Blöcke, meist kompakt; verzerrt keine Randfälle (frühe/späte Import-Sessions). |
| Bulk-Aktionen v1 | **Projekt zuweisen** + **inline „✚ neu"** im Picker + **Bulk-Löschen** | Alle drei im Mockup; mit Slice-0-`ConfirmDialog` + bestehenden Endpunkten günstig. |
| Sessions-Pagination | Endpoint `?limit&offset` **+** paginierte flache „Alle Sitzungen"-Liste in Historie | Nutzer-Wahl: Pagination end-to-end demonstriert; Kalender bleibt zeitfenster-begrenzt. |
| Heute | **auf AppShell portieren** (Index `/` nicht im alten Look lassen) | Konsistente Worktime-Triade; Einstiegsseite zuerst modern. |
| Selektions-Zustand | **client-seitiges Vanilla-JS** (ephemer), IDs erst beim Zuweisen/Löschen ge-POSTet | Per-Checkbox-Roundtrip wäre absurd; Parent-Spec erlaubt „kleines Vanilla-JS". |
| Antwort-Form Pagination | Body bleibt `[]WorkSession`, Gesamtanzahl via **`X-Total-Count`-Header** | Non-breaking für bestehende apiclient-`ListSessions`-Konsumenten. |
| Reassign-Pfad | Usecase + REST-Endpoint + apiclient-Methode; **WebUI-Handler ruft Usecase direkt** | Matcht bestehendes webui-Handler-Muster (kein Self-HTTP); REST+apiclient = testbarer Seam + CLI/TUI-Parität. |

---

## 3. Backend-Änderungen (additiv, hexagonal)

### 3.1 Bulk-Neuzuweisung
- **Usecase** `BulkAssignProject` (`internal/usecase/bulk_assign_project.go`):
  `Execute(ctx, ownerID string, sessionIDs []string, projectID string) (updated int, err error)`.
  - Validiert: `projectID` gehört `ownerID` (sonst `ErrProjectNotFound`/403-Mapping); jede Session gehört `ownerID` (fremde IDs werden übersprungen **oder** lösen Fehler aus — **Wahl: überspringen + im Count nur tatsächlich geänderte**, robust gegen veraltete Auswahl).
  - Setzt `ProjectID = &projectID`; **Start/Stop unverändert** → keine Overlap-Prüfung nötig.
  - Leere `sessionIDs` → `ErrNoSessions` (400). 
- **REST** `POST /api/v1/sessions/reassign` (authAny: Bearer **oder** Cookie): Body `{ "ids": ["…"], "projectId": "…" }` → `{ "updated": N }`. Publiziert **ein** `session.updated`-Event (UserID). 

### 3.2 Bulk-Löschen
- **Usecase** `BulkDeleteSessions` (`internal/usecase/bulk_delete_sessions.go`):
  `Execute(ctx, ownerID string, ids []string) (deleted int, err error)` — owner-scoped, überspringt Fremde.
- **REST** `POST /api/v1/sessions/bulk-delete` (authAny): Body `{ "ids": […] }` → `{ "deleted": N }`. Publiziert **ein** `session.deleted`-Event.

### 3.3 Pagination
- **Store-Methode** (`ports.SessionStore`): `ListPage(ctx, ownerID string, limit, offset int) (items []domain.WorkSession, total int, err error)` — sortiert `start DESC`. Implementiert in pgstore (`COUNT(*)` + `LIMIT/OFFSET`) und im In-Memory-/Test-Store.
- **Usecase** `ListSessionsPage` (`internal/usecase/list_sessions_page.go`): dünner Wrapper über `ListPage`.
- **REST** `GET /api/v1/sessions?limit&offset`: liefert all-time newest-first; Body bleibt `[]WorkSession`; Gesamtanzahl als Response-Header **`X-Total-Count`**.
  - Bestehendes Verhalten unverändert: `GET /sessions` (alle), `GET /sessions?since&until` (Range). `limit/offset` greift nur wenn gesetzt. Default-Seitengröße in der WebUI: 50 (Server validiert `limit ≤ 200`, `offset ≥ 0`).

### 3.4 apiclient (CLI/TUI-Parität + Test-Seam)
- `ReassignSessions(ctx, projectID string, ids []string) (int, error)`
- `BulkDeleteSessions(ctx, ids []string) (int, error)`
- `ListSessionsPage(ctx, limit, offset int) ([]WorkSession, total int, err error)` (liest `X-Total-Count`)

> Die WebUI selbst nutzt apiclient **nicht** — Handler rufen Usecases/Stores direkt (wie die bestehenden `webui_*.go`). apiclient ist für CLI/TUI + Tests.

---

## 4. Komponenten-Bibliothek (neu, `internal/adapter/webui/components/`)

„Keine Monolithen" — je Verantwortung eine Datei, klare Parameter, isoliert render-testbar. Wiederverwendet aus Slice 0: `Base`, `AppShell`, `SiteNav`, `TabStrip`, `Dialog`, `ConfirmDialog`, `Pagination`, `Button`/`IconButton`, `Card`, `Chip`(=ProjectChip), `Badge`, `Glyph`, `StatTile`, `EmptyState`, `Breadcrumb`, `ThemeToggle`.

| Komponente | Datei | Parameter (Skizze) | Zweck |
|---|---|---|---|
| `ProgressBar` | `progressbar.templ` | `ProgressBar(pct int, variant ProgressVariant)` (hit/over/under/running) | Tagesziel (Heute), Tagesbalken (Woche) |
| `PaceDots` | `pacedots.templ` | `PaceDots(dots []PaceDot)` (behind/ontrack/ahead/running/holiday/off) | Wochenstreifen (Heute, Woche) |
| `SessionRow` | `sessionrow.templ` | `SessionRow(vm SessionRowVM, selectable bool)` | Heute-Liste, Agenda (mobil), Liste-Ansicht |
| `SessionBlock` | `sessionblock.templ` | `SessionBlock(vm SessionBlockVM)` (top/height/hue/unassigned/running/selectable) | Kalender-Zeitraster-Block |
| `KennzahlenPanel` | `kennzahlen.templ` | `KennzahlenPanel(vm KennzahlenVM)` | Woche: Schnitt/Ziele/Saldo/pace/Status |
| `WeekTotalBanner` | `weektotal.templ` | `WeekTotalBanner(vm WeekTotalVM)` | Woche: WOCHE GESAMT (Mo–Fr-Ziel) |
| `ProjectFuzzyPicker` | `fuzzypicker.templ` | `ProjectFuzzyPicker(projects []ProjectOptionVM, newName string)` | MRU + Filter-Input + inline „✚ neu: …" |
| `SelectionActionBar` | `selectionbar.templ` | `SelectionActionBar(vm SelectionBarVM)` | Sticky Bulk-Leiste (Count, Zuweisen+Picker, Löschen, Abbrechen) |
| `SegToggle` | `segtoggle.templ` | `SegToggle(options []SegOption, active string)` | Woche\|Monat, Kalender\|Liste |

Single-Edit = Slice-0 `Dialog` + Session-Edit-Form-Body (Projekt-Picker, Start/Stop `type=time`, Tag, Notiz, Löschen→`ConfirmDialog`).

---

## 5. Surfaces

### 5.1 Heute (`/`) — `direction-b-studio.html`
Live-Session-Karte (▶ Projekt, große tickende Dauer client-seitig, Stopp), Start mit `ProjectFuzzyPicker`, Heute-gesamt vs Tagesziel (`ProgressBar`) + Saldo, heutige `SessionRow`s (inline Edit/Delete via `Dialog`/`ConfirmDialog`), Wochenstreifen mit `PaceDots`.
- **Leer:** „Noch keine Sitzung heute — starte oben."
- **Live:** `session.*`, `project.created` → `HeuteFragment`-Swap; Timer tickt client-seitig (Base-Script).

### 5.2 Woche (`/woche`) — `studio-worktime-week.html`
Tagesbalken Mo–So (`ProgressBar` mit Ziel-Marker + Overflow): erreicht/über=grün, unter=gelb, Wochenende dezent, Frei=Kind-Farbe+Glyph, heute/laufend=cyan. `WeekTotalBanner` (Mo–Fr-Ziel, **Wochenende exkludiert**). `KennzahlenPanel` (Schnitt · Ziele X/5 · Saldo · `PaceDots` · auf-Kurs/Rückstand). ‹ KW › Navigation.
- **Live:** `session.*`, `dayoff.changed`, `settings.changed`.

### 5.3 Historie (`/historie`) — `studio-worktime-calendar.html` — Kern
Toolbar: `SegToggle` **Kalender | Liste**, in Kalender zusätzlich **Woche | Monat**, Datums-/KW-Nav ‹ › + „Diese Woche", Filter (Alle / ohne Projekt / nach Projekt). Orange-Banner „N Sitzungen ohne Projekt" wenn vorhanden.

**Kalender / Woche (Desktop):** Zeitraster mit hybridem Fenster (Default 06–20, ausgedehnt auf Wochen-min/max, auf Stunde gesnapped; `--hour:48px`, Höhe `(ceil−floor)·48px`). `SessionBlock` zeit-positioniert in Projektfarbe (3px-Rail + Tint); **„ohne Projekt"** = grau/gestrichelt + Warn-Tint (fällt auf); laufende Session = cyan + now-line. Tag-Header zeigt Datum + Tagessumme.

**Kalender / Monat:** **nur navigierend** — Monatsgitter mit Tag-Zahl, gestapelten Mini-Projektbalken, ○-Flag bei vorhandenen „ohne Projekt"; Klick auf Tag → springt in die Wochen-Ansicht dieser Woche. **Kein** Bulk im Monat. Footer: Monatssumme + „N ohne Projekt im Monat".

**Mobil:** Agenda-Liste pro Tag (`SessionRow`), Checkboxen im Bulk-Modus, Sticky-Bulk-Leiste.

**Liste (`SegToggle`=Liste):** paginierte flache `SessionRow`-Liste (newest-first, `Pagination`, Seitengröße 50) über `ListSessionsPage`. Bulk-Modus auch hier (Row-Checkboxen).

**Bulk-Modus (Kalender-Woche, Agenda, Liste):** Button „Auswählen" → Checkboxen/Block-Selektion. Client-JS-Helfer: Block/Row toggeln, „Tag wählen" (Tag-Header), „ganze Woche", „alle ohne Projekt auswählen". `SelectionActionBar` (sticky): „N ausgewählt" → **Projekt zuweisen** (`ProjectFuzzyPicker`, inkl. inline-create) / **Löschen** (`ConfirmDialog`) / **Abbrechen**.
- Inline-create im Picker: `CreateProject(name)` (bestehend) → dann reassign mit neuer ID (Handler verkettet serverseitig).

**Einzel-Edit (Nicht-Select-Modus):** Block/Row-Klick → `Dialog` (Projekt-Picker, Start/Stop, Tag, Notiz, Löschen).
- **Live:** `session.*`, `project.*`.

---

## 6. Live-Updates (SSE) & Client-JS

Bestehend: `GET /api/v1/events` (per-User, Bearer **oder** Cookie); `<body hx-ext="sse" sse-connect="/api/v1/events">`, Fragmente mit `sse-swap`.

| Event | aktualisiert in Slice 1 |
|---|---|
| `session.started/stopped/updated/deleted` | Heute, Woche, Historie |
| `project.created/updated/deleted` | Heute (Picker), Historie (Picker/Farben) |
| `dayoff.changed`, `settings.changed` | Woche |

**Client-JS (minimal, eingebettet unter `static/`, kein CDN):**
- Live-Timer (Base-Script aus Slice 0, wiederverwendet).
- `historie-select.js`: ephemerer Selektions-Zustand (Set von Session-IDs), Toggle-Logik, „Tag/Woche/alle ohne Projekt", Anzeige `SelectionActionBar`-Count, beim Zuweisen/Löschen die IDs in ein verstecktes Form-Feld serialisieren und htmx-POST auslösen.
- `historie-view.js`: Kalender↔Liste, Woche↔Monat Umschaltung (oder via htmx-Fragmente — Plan-Detail).

---

## 7. Architektur & Datei-Organisation

```
internal/adapter/webui/
  heute.templ        # HeutePage + HeuteFragment
  woche.templ        # WochePage + WocheFragment
  historie.templ     # HistoriePage + HistorieCalendarFragment + HistorieListFragment + HistorieAgendaFragment
  components/         # neue Komponenten (siehe §4)
  static/            # historie-select.js, historie-view.js (go:embed)
internal/adapter/httpserver/
  webui_heute.go     # / (+ POST start/stop/add/edit/delete) — Viewmodel bauen
  webui_woche.go     # /woche (+ KW-Nav, Fragment)
  webui_historie.go  # /historie (+ Kalender/Liste/Monat-Nav, POST reassign/bulk-delete)
  sessions.go        # REST: + /sessions/reassign, /sessions/bulk-delete, ?limit&offset
internal/usecase/
  bulk_assign_project.go  bulk_delete_sessions.go  list_sessions_page.go
internal/ports/ports.go   # SessionStore += ListPage
internal/adapter/pgstore/ # ListPage-Impl
internal/adapter/apiclient/ # ReassignSessions, BulkDeleteSessions, ListSessionsPage
```

Handler-Muster (unverändert): Handler baut typisiertes Viewmodel → `webui.XPage(vm).Render(ctx, w)`; htmx-Refresh → `webui.XFragment(vm)`. Altes `worktime.templ` + alte `/`-/`/ui/worktime`-Routen werden auf Heute umgestellt; altes `worktime.templ` gelöscht, sobald ungenutzt.

---

## 8. Zustände, Dialoge, Pagination (Querschnitt)

- **Leer/Fehler/Lade** pro Surface definiert (deutsche UI-Stimme): Heute „Noch keine Sitzung heute"; Woche „Keine Sitzungen diese Woche"; Historie „Keine Sitzungen im Zeitraum"; Historie-Banner bei vielen „ohne Projekt".
- **Keine Browser-Popups**; jede destruktive Aktion (Einzel-Delete, Bulk-Delete) erst nach `ConfirmDialog` mit Konsequenz-Text + sicherem Default (Abbrechen fokussiert), destruktiver Button in `danger`.
- **Pagination** nur Liste-Ansicht (Slice 1); Kalender paginiert über das Zeitfenster (Woche/Monat).

---

## 9. Testing & Done-Gate

- **Unit:** `BulkAssignProject` (owner-scoping, fremde IDs übersprungen, leere Liste→Fehler, projektfremd→Fehler, Event publiziert); `BulkDeleteSessions`; `ListSessionsPage`/`ListPage` (limit/offset/total, Sortierung); Handler-`httptest` (reassign/bulk-delete/pagination-Header, auth); templ-Render-Tests neuer Komponenten.
- **Lint/Guard:** `make ci` grün (lint inkl. QF1002 — Integrations-Implementierer führen `make ci`, **nicht** nur `go test`); `verify-css`; Lint, dass kein `window.alert/confirm/prompt` im Code.
- **Live-Done-Gate** gegen Postgres+Dex (Dev-Stack): jede Route (`/`, `/woche`, `/historie`, Fragmente) 200; SSE-Live (Session start/stop spiegelt sich in allen drei); Reassign + inline-create + Bulk-Delete wirken und spiegeln via SSE; Pagination-Header korrekt; Dark/Light + Mobile geprüft; keine externen Requests (offline).
- Mockups in `assets/` = visuelle Referenz.

---

## 10. Slicing für writing-plans

Ein Plan mit TDD-Tasks, **expliziter Main-Wiring-/Verifikations-Task am Ende** ([[feedback_plan_main_wiring_task]]):
1. Backend: `ListPage` + `ListSessionsPage` + `GET /sessions?limit&offset` (X-Total-Count).
2. Backend: `BulkAssignProject` + `POST /sessions/reassign`.
3. Backend: `BulkDeleteSessions` + `POST /sessions/bulk-delete`.
4. apiclient: `ListSessionsPage`/`ReassignSessions`/`BulkDeleteSessions`.
5. Komponenten: ProgressBar, PaceDots, SessionRow, SessionBlock, KennzahlenPanel, WeekTotalBanner, ProjectFuzzyPicker, SelectionActionBar, SegToggle (+ Render-Tests).
6. Heute-Seite + Handler (Port, `/` umstellen).
7. Woche-Seite + Handler.
8. Historie-Seite + Handler (Kalender/Monat/Agenda/Liste) + Bulk-Client-JS.
9. **Main-Wiring + Verifikation:** Routen in `server.go` registriert, altes `worktime.templ` entfernt, `make ci` grün, curl-Smoke je Route + reassign/bulk-delete/pagination, Live-Done-Gate.

Ausführung subagent-getrieben; nach dem Slice Holistic-Review (Opus) + Dogfood-Gate.
