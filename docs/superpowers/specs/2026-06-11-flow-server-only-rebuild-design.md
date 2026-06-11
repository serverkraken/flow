# flow Server-Only Rebuild — Design (Phase-1-Reset)

**Datum:** 2026-06-11
**Status:** approved (Brainstorm-Sektionen 1–3 abgenommen 2026-06-11)
**Scope:** Konzeptueller Reset des `next`-Branches: Server wird einzige Wahrheit, Clients werden dünn, Postgres ersetzt SQLite server-seitig, Kompendium-Dokumente werden erstklassige Server-Resource. Ersetzt die Sync-Architektur aus `2026-06-02-flow-client-server-phase1-design.md`; alles dort nicht Widersprochene (OIDC, Hosting, WebUI-Stack, Hexagonal-Layout) gilt weiter.

## 1. Ausgangslage & Diagnose

Der `next`-Branch (M1–M9 implementiert, homelab-deployed) ist in Soennes Urteil nicht nutzbar — alle vier Schmerzfelder gleichzeitig: unzuverlässiger Worktime-Sync, fehlende Markdown-Dokumente, Login/Identity-Gefrickel, kaputtes TUI/WebUI-Erlebnis. Die Review-Historie (12 PoC-Blocker + Identity-Adoption-Arc + 17 Fix-Tasks) zeigt fünf strukturelle Wurzelursachen:

1. **Hybrides Wahrheitsmodell.** Offline-first-Local-Truth (Sessions/Projects/Notes) + Server-Truth (ActiveSessions) + flockstate (Pause). Jeder Write-Pfad muss wissen, in welchem Modell er lebt — die Blocker (toggle-Split, Version-Writeback, started_at-Drift, Queue-Halt) waren direkte Folgen.
2. **Identity rückwärts gebootstrapt.** `local`-User vor Login, dann Adoption + pull-remap; reihenfolgeabhängige Logins (logout-zuerst-Ritual), Footgun in `buildDeps` offen.
3. **Kernziel nie im Datenmodell.** Kompendium-Markdown blieb lokales FS; M4 wurde still auf RepoNotes verengt; MCP-Doc-Tools deferred; WebUI-`/notes` las das Container-FS (`FLOW_NOTEBOOK_ROOT`).
4. **Verteiltheit ohne Sichtbarkeit.** Kein Sync-Status irgendwo; WebUI-SyncState hardcoded `"ok"`.
5. **Operative Fragilität.** Mutable `:next`-Tag, stale-mirror, kein Client/Server-Versions-Handshake.

**Entscheidung:** Nicht härten, sondern vereinfachen. Offline-Schreiben wird aufgegeben (Soennes reale Nutzung: Geräte im LAN/WLAN, Server im Homelab).

## 2. Ziele

Soennes acht Ziele, unverändert:

1. Worktime auf mehreren Geräten, überall derselbe Status
2. Markdown-Dokumente auf allen Geräten verfügbar
3. flow kompendium auch für Claude-erstellte Dokumente (MCP)
4. WebUI mit Einblick in Worktime + Dokumente, inkl. Worktime-Bedienung, alle Geräte in Sync
5. WebUI responsive
6. Multi-User-fähig später (Authentik bleibt)
7. WebUI übersichtlich und gut nutzbar
8. TUI übersichtlich und gut nutzbar

## 3. Nicht-Ziele

- **Offline-Schreiben.** Server nicht erreichbar ⇒ read-only-Anzeige (letzter Snapshot) + sichtbares Offline-Banner. Ein „Offline-light-Journal" (append-only Replay für start/stop) ist ein sauber nachrüstbares Add-on, wird aber **nicht** vorsorglich gebaut.
- CRDT / Konflikt-Auflösungs-UI. Es gibt keine zwei Wahrheiten mehr; `If-Match`/412 + Neuladen genügt.
- Standalone-Betrieb ohne Server als eigener Code-Modus. Für Dritte gilt: flow-server via compose auf localhost ist der Standalone-Modus.
- Multi-User-**Funktionalität** (Phase 2; Datenmodell bleibt user-gescoped und Phase-2-ready).
- Mobile-App/PWA. Responsive WebUI reicht.

## 4. Kernentscheidungen

| Frage | Entscheidung |
| --- | --- |
| Wahrheitsmodell | **Server-only.** Eine Wahrheit (Server-DB) für Worktime, Projekte, Dokumente, Day-Offs, Settings. Clients sind dünne API-Konsumenten. |
| Server-DB | **PostgreSQL via CNPG** (Operator läuft im Homelab). pgx/v5 + goose (PG-Dialekt), frische Baseline-Migration. SQLite fliegt server-seitig komplett raus; kein Dual-Dialekt. |
| Dokumente | **`documents`-Tabelle in der Server-DB** mit Postgres-FTS. RepoNotes werden ein Namespace darin (`repos/<key>.md` + `repo_key`-Spalte). Lokale `~/notes`-Dateien werden einmalig importiert. |
| Pause | **Server-Zustand** der ActiveSession (`paused_at`, `pause_total`). start/stop/pause/resume sind Server-Endpoints — eine Statemachine für alle Geräte. |
| Identity | **Kein local-User mehr.** Login (Device-Flow) ist Voraussetzung für Datenzugriff; Adoption/pull-remap entfallen ersatzlos. |
| Liveness | **SSE-Events für alle Clients** (Browser, TUI, MCP) + Fallback-Poll. Ziel: Gerät B sieht Änderungen von Gerät A in < 2 s. |
| Umsetzungsweg | **In-place-Rückbau auf `next`** (Weg A): Adapter-Swap hinter bestehenden Ports, „delete first". `next` bleibt Integrationsbranch, Squash auf `main` am Ende. |
| Auth | Unverändert: Authentik-OIDC, multi-issuer-Verifier, Device-Flow + Keychain (per-Feld-Split), Browser-Cookie-Flow, `FLOW_ALLOWED_SUBS`-Allowlist. |

## 5. Architektur

```
flow-server (K8s, stateless, 1 Replica)          flow (TUI/CLI)    flow-mcp     Browser
  ├─ httpserver: REST /api/v1/* + SSE /events      └─ httpapi ─────┴─ httpapi      │
  ├─ webui: Templ/HTMX (nutzt dieselben Use-Cases)      (Port-Adapter mit          │
  ├─ usecase: Worktime-Statemachine, Documents           In-Memory-Cache +         │
  ├─ pgstore: pgx/v5-Adapter (ersetzt sqliteserver)      SSE-Invalidierung +    OIDC (Authentik)
  └─ oidcserver: multi-issuer (unverändert)              Offline-Read-Snapshot)
            │
       CNPG-Cluster „flow-pg" (Backups/PITR via Operator; Litestream + PVC entfallen)
```

- **flow-server** bleibt der Kern. Neu: pgstore, documents, Statemachine-Endpoints, `/api/v1/meta`, generalisierte SSE. WebUI rendert aus denselben Use-Cases wie die REST-API.
- **flow (TUI/CLI)** und **flow-mcp** teilen den neuen `internal/adapter/httpapi`, der die **bestehenden Ports** implementiert (SessionStore inkl. Edit-Upserts, ActiveSessionStore, ProjectStore, DayOffStore, neu DocumentStore). Use-Cases und Screens bleiben unangetastet — der Umbau ist ein Adapter-Swap.
- Kein Hintergrund-Sync-Worker mehr. Es gibt nichts zu syncen.

## 6. Datenmodell (PG-Baseline)

```sql
CREATE TABLE users (
    id           uuid PRIMARY KEY,
    oidc_sub     text NOT NULL UNIQUE,
    display_name text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id),
    name       text NOT NULL,
    slug       text NOT NULL,
    archived   boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
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
    user_id     uuid NOT NULL REFERENCES users(id),
    project_id  uuid NOT NULL REFERENCES projects(id),
    started_at  timestamptz NOT NULL,   -- Server-Zeit, nie Client-Zeit
    paused_at   timestamptz,            -- NULL = läuft
    pause_total interval NOT NULL DEFAULT '0',
    started_on_device text NOT NULL DEFAULT '',
    tag         text NOT NULL DEFAULT '',
    note        text NOT NULL DEFAULT '',
    version     bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE documents (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id),
    path       text NOT NULL,           -- relativ, '/'-separiert, z. B. "projects/serverkraken/flow/ideen.md"
    body       text NOT NULL DEFAULT '',
    repo_key   text,                    -- gesetzt für Repo-Notes ("git:github.com/foo/bar" | "path:sha256…")
    version    bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    search     tsvector GENERATED ALWAYS AS (to_tsvector('simple', path || ' ' || body)) STORED,
    UNIQUE (user_id, path)
);
CREATE UNIQUE INDEX documents_repo_key ON documents (user_id, repo_key) WHERE repo_key IS NOT NULL;
CREATE INDEX documents_search ON documents USING gin (search);

CREATE TABLE day_offs (
    user_id uuid NOT NULL REFERENCES users(id),
    day     date NOT NULL,
    kind    text NOT NULL,              -- Kinds wie heutige TUI-Domain (zur Plan-Zeit übernehmen)
    PRIMARY KEY (user_id, day)
);

CREATE TABLE user_settings (
    user_id uuid NOT NULL REFERENCES users(id),
    key     text NOT NULL,              -- "daily_target", "timezone", …
    value   text NOT NULL,
    PRIMARY KEY (user_id, key)
);
```

Festlegungen:

- **Zeit:** durchgängig `timestamptz`. Buchungstag (`day`) berechnet der Server aus `started_at` in der User-Zeitzone (`user_settings.timezone`, Default `Europe/Berlin`). Clients rendern lokal.
- **Repo-Notes = Dokumente.** Konvention: `path = "repos/" + urlescape(canonical_key) + ".md"`, `repo_key` gesetzt. Lookup wahlweise über Pfad oder `repo_key`.
- **FTS:** `'simple'`-Konfiguration (DE/EN-Mix), Query via `websearch_to_tsquery`, Ranking `ts_rank`. Kein `pg_trgm` in Phase 1.
- **Versionierung:** `version` pro Row, Increment server-seitig bei jedem Write; `If-Match` auf PUT (412 bei Mismatch).
- Die `lamport`-Tabelle und alle Sync-Watermarks entfallen.

## 7. API

Alle Routen unter `/api/v1`, Auth via Bearer (Device-Flow-Token) oder Browser-Session-Cookie. Statuscodes: 401 (kein/abgelaufenes Token → Client zeigt Login-Hinweis), 403 (sub nicht in Allowlist), 404, 409 (ActiveSession existiert bereits), 412 (If-Match-Mismatch), 422 (Validierung).

| Route | Methoden | Zweck |
| --- | --- | --- |
| `/worktime/sessions?from=&to=` | GET | Sessions im Zeitraum |
| `/worktime/sessions` | POST | Manuelle Session anlegen (Correct/Nachtrag) |
| `/worktime/sessions:bulk` | POST | Idempotenter Bulk-Import (TSV-Migration; Client-UUIDv5-IDs) |
| `/worktime/sessions/{id}` | PUT, DELETE | Edit/Delete mit `If-Match` |
| `/worktime/active` | GET | Alle aktiven Sessions des Users |
| `/worktime/active/start` | POST | `{project_id, tag?, note?}` → 409 wenn für Projekt schon aktiv |
| `/worktime/active/stop` | POST | `{project_id}` → schreibt Session (elapsed = now − started_at − pause_total; eine laufende Pause endet mit dem Stop) |
| `/worktime/active/pause` · `/resume` | POST | Pause-Statemachine, idempotent |
| `/projects` · `/projects/{id}` | GET, POST, PUT | CRUD inkl. Archivieren |
| `/documents?prefix=&q=&limit=` | GET | Liste (Tree via prefix) oder FTS-Suche |
| `/documents/{path…}` | GET, PUT, DELETE | Einzeldokument; PUT mit `If-Match` (Create: `If-Match: 0`) |
| `/repos/{canonical-key}/note` | GET, PUT | Alias auf das Dokument mit diesem `repo_key` |
| `/day-offs?year=` | GET | Day-Offs |
| `/day-offs/{date}` | PUT, DELETE | Setzen/Entfernen |
| `/settings` | GET, PUT | User-Settings (Tagesziel, Zeitzone) |
| `/me` | GET | Identität (bleibt) |
| `/meta` | GET | `{server_version, min_client_version}` — Versions-Handshake |
| `/events` | GET (SSE) | `changed`-Events `{resource: worktime\|documents\|projects\|dayoffs}`; Heartbeat-Kommentar alle 25 s |

- Clients senden `X-Flow-Client-Version`. TUI/WebUI warnen sichtbar, wenn `min_client_version` nicht erfüllt ist.
- SSE ersetzt Poll nicht vollständig: Clients refetchen bei (Re-)Connect und pollen als Fallback alle 30–60 s, falls der Stream reißt.
- Die bisherigen Sync-Endpoints (paginierte pull/push-Routen, Drain-Semantik) entfallen ersatzlos.

## 8. Client: flow (TUI/CLI)

- **`internal/adapter/httpapi`** implementiert die bestehenden Ports. Intern: typisierter REST-Client (Kern aus dem heutigen `httpsync`-Client wiederverwendbar), In-Memory-Cache pro Resource, Invalidierung durch SSE-`changed` und eigene Writes (danach Refetch), Fallback-Poll.
- **Writes synchron.** `s`/Start/Stop/Pause/Toggle = API-Call aus dem bubbletea-Cmd; Erfolg oder sichtbarer Fehler. Kein Queuen. Timer tickt lokal ab Server-`started_at`/`pause_total` — Client-Uhr schreibt nie Zeiten.
- **Statuszeile überall** (Worktime-Spine + Standalone-Subcommands): still wenn ok, laut wenn nicht. Online: `●` + Host in `FgMuted`; offline: `○ offline · Stand 14:32 (read-only)` in `Sem().Warning`; ohne Login: `○ nicht angemeldet · flow login`; veraltet: `▲ Client veraltet` (Glyph-Whitelist: kein `⚠`). Glyph + Farbe, nie Farbe allein.
- **Offline-Read-Snapshot:** letzte erfolgreiche Read-Antworten als JSON unter `$XDG_STATE_HOME/flow/snapshot.json` (mit `fetched_at`). Bei Server-Unerreichbarkeit speisen sie die Read-Ports; Writes liefern eine klare Fehlermeldung.
- **Ohne Login:** Screens zeigen Login-Hinweis, CLI-Verben brechen mit Anleitung ab. `FLOW_LOCAL_USER_SUB`, local-User, Adoption: gestrichen.
- **Kompendium-Screen:** Tree/Read über documents-API, Markdown-Rendering + Wikilink-Auflösung bleiben client-seitig (Pfadliste aus dem Cache), Suche = Server-FTS, `e` = temp-File → `$EDITOR` → PUT mit `If-Match`; bei 412 neu laden + Hinweis.
- **Lokal bleiben:** Cheatsheet, Palette, Sidekick-State (`jsonflowstate`) — UI-Zustand, keine User-Daten.
- `FLOW_SERVER_URL` ist Pflicht-Konfiguration für Worktime/Docs-Funktionen.

## 9. Client: flow-mcp

- Gleicher `httpapi`-Adapter, kein eigener Worker.
- **Tools (10):** bisherige 7 (`flow_get_repo_note`, `flow_save_repo_note`, `flow_list_repo_notes`, `flow_search_notes`, `flow_worktime_status`, `flow_start_session`, `flow_stop_session`) — Repo-Note-Tools arbeiten jetzt auf dem documents-Namespace — **plus** `flow_get_note(path)`, `flow_save_note(path, content)`, `flow_list_notes(prefix?)`. `flow_search_notes` wird Server-FTS über alle Dokumente inkl. Repo-Notes.
- **Resources:** `flow://docs/<path>` zusätzlich zu `flow://repos/<key>/note`.
- Boot ohne Token: unverändert „Login required: run `flow login`".

Damit ist Ziel 3 vollständig: Claude kann Kompendium-Dokumente lesen **und schreiben**, von jedem Gerät, gegen dieselbe Wahrheit.

## 10. WebUI

- **Notes-Sektion auf documents-Use-Cases** (derselbe Code wie die REST-API). Der `FLOW_NOTEBOOK_ROOT`/fsstore-Pfad im Server entfällt. Liste = Tree nach Pfad-Prefix + FTS-Suche; View = server-gerendertes Markdown (Renderer bleibt); Edit = CodeMirror mit verstecktem `If-Match`.
- **Worktime:** bestehende vier Tabs + Start/Stop bleiben; **Pause/Resume-Buttons** kommen dazu (dieselbe Statemachine wie TUI).
- **Statusleiste ehrlich:** eingeloggt als + Server-Version; der hardcodete `SyncState: "ok"` wird entfernt — es gibt keinen Sync mehr, nur Erreichbarkeit.
- **Responsive-Pass als eigene Task:** Navigation collapsible unter `md`, Session-Tabellen → Cards unter `sm`, documents-Tree full-width; Verifikation real am Telefon, nicht nur im DevTools-Emulator.
- SSE-Live-Banner konsumiert dieselben `changed`-Events wie TUI/MCP.

## 11. Migration der Bestandsdaten

1. **Server-PoC-Daten:** verwerfbar; PG startet leer.
2. **Worktime-TSV** (Hauptlaptop): `flow worktime migrate-from-tsv` liest wie bisher, schreibt via `POST /worktime/sessions:bulk`. Idempotent über UUIDv5. **Zeitzonen explizit:** TSV-Zeiten sind lokale Zeiten → Import mappt mit `Europe/Berlin` auf `timestamptz`; eigene Plan-Task + Stichproben-Verifikation.
3. **Kompendium:** neues einmaliges `flow docs import <dir>` — rekursiv, `PUT /documents/{relpath}` pro Datei, idempotent (Re-Run überschreibt mit `If-Match`-Disziplin).
4. **Day-Offs + Tagesziel:** vom Import-Verb mit abgedeckt bzw. einmalig manuell (wenige Einträge).

## 12. Lösch-Liste

Ersatzlos entfernt (exakte Paketpfade werden zur Plan-Zeit mechanisch verifiziert):

| Was | Warum obsolet |
| --- | --- |
| `internal/adapter/httpsync` (Worker, Write-Queue, Drains, Conflict-Channel) | kein Sync mehr |
| `internal/adapter/sqliteclient` (kompletter lokaler Cache inkl. `sync_state`, `write_queue`) | Clients halten keine Wahrheit mehr |
| `internal/adapter/sqliteserver` + goose-SQLite-Migrationen + `lamport` | ersetzt durch pgstore |
| flockstate-Adapter (ActiveStore/PauseMarker) | Pause ist Server-Zustand |
| conflict_overlay-Component + TUI-Konflikt-Verkabelung | keine zwei Wahrheiten, keine Konflikte |
| local-User/Adoption/pull-remap (`tryAdoptLocalProfile`, Remap im Worker, `FLOW_LOCAL_USER_SUB`) | Login-first |
| kompendium fsstore-Anbindung im Server (`FLOW_NOTEBOOK_ROOT`) + `kompsqliteindex`-FTS-Index | documents-API + PG-FTS |
| WebUI `SyncState`-Fake | echter Status |
| Litestream-Sidecar + PVC im Helm-Chart | CNPG-Backups |

Bleiben: `jsonflowstate` (Sidekick-UI-State), Markdown-Renderer + Wikilinks, oidcclient/oidcserver, mcpstdio, WebUI-Design-System.

## 13. Rückbau-Reihenfolge (Milestones R1–R6 auf `next`)

Jeder Schritt endet `make ci`-grün; Details schneidet writing-plans.

1. **R1 Server:** pgstore + PG-Baseline, documents + FTS, Pause-Statemachine, `/meta`, SSE generalisiert; compose + Helm auf CNPG/PG; sqliteserver + Sync-Endpoints löschen; WebUI auf documents + echten Status.
2. **R2 Client:** httpsync/sqliteclient/flockstate/Adoption/conflict_overlay löschen; `httpapi`-Adapter; Statuszeile; Login-Pflicht-UX; CLI-Verben auf API.
3. **R3 flow-mcp:** auf httpapi; Doc-Tools + Resources.
4. **R4 Importe:** TSV-Bulk-Migration, `flow docs import`.
5. **R5 WebUI-Polish + Responsive-Pass.**
6. **R6 Wiring-Verification + E2E:** Composition-Root-Audit + curl-Smoke jeder Route (gemäß Wiring-Task-Regel); Multi-Device-Smoke: A `start` → B sieht es < 2 s (SSE), Browser parallel; MCP `flow_save_note` → WebUI zeigt das Dokument; Offline-Verhalten (Server gestoppt → Banner + read-only).

Abschluss: PR #48 wird auf diesen Stand aktualisiert; Squash auf `main`, sobald Soenne produktiv nutzt und vertraut.

## 14. Testing

- **pgstore:** testcontainers-Postgres (Pattern im Repo durch testcontainers-Dependency vorhanden); Store-Tests analog heutigem sqliteserver-Niveau.
- **httpapi:** gegen `httptest`-Server mit echten Handlern (Router-Level-Konvention wie WebUI), inkl. 401→Login-Hinweis, 412→Refetch, Offline→Snapshot.
- **Statemachine:** Tabellen-Tests (start/stop/pause/resume/Doppel-Start/Stop-ohne-Start) im Server-usecase.
- **SSE:** Handler-Test (Events kommen, Heartbeat), Client-Test (Invalidierung + Reconnect-Refetch).
- **E2E-Smoke-Script:** compose-Stack (PG + dex + flow-server) + zwei Client-Homes + curl; ersetzt `smoke-m2-m3.sh`.
- **Coverage-Gate:** nach R2 neu vermessen und ehrlich setzen (Löschungen verschieben die Basis; templ-Drag bleibt).

## 15. Risiken & offene Punkte

1. **Server down = kein Tracking.** Bewusster Kauf. Nachrüstoption Offline-light-Journal dokumentiert (Nicht-Ziel, bis es real schmerzt).
2. **Zeitzonen-Mapping beim TSV-Import** — eigene Task + Stichproben gegen die heutige TUI-Anzeige.
3. **SSE durch Ingress/Proxy-Idle-Timeouts** — Heartbeat 25 s + Reconnect-mit-Refetch; Fallback-Poll deckt Totalausfall ab.
4. **PG-Betrieb:** CNPG-Cluster + Secret-Wiring in homelab-study (eigene PRs, render-then-commit-Hygiene); einmalige Restore-Drill ersetzt die Litestream-Drill.
5. **Remote-Latenz:** Writes unterwegs ~100–300 ms — akzeptiert; Reads kommen aus dem Cache.
6. **Push-Stand `next`:** verifiziert 2026-06-11 — HEAD = origin/next = `c175c97`, Working-Tree clean. Nach den PoC-Fixes (`aacd794`) liegen dort 7 weitere TUI/WebUI-Fix-Commits (FastTick, PullDone-Reload, Picker-Switch u. a.).
7. **Lösch-Liste-Pfade:** Paketnamen zur Plan-Zeit mechanisch verifizieren (`fd`/`rg`), nicht aus diesem Doc abschreiben.

## 16. Phase-2-Ausblick (unverändert, jetzt natürlicher)

- Frau als zweiter User: Authentik + Allowlist; alle Tabellen sind user-gescoped.
- Sharing wird ein Feature-Add auf `documents` (`document_shares`-Tabelle) statt auf zwei Note-Systemen.
- Concurrent-Editing-Schmerz ⇒ erst dann über Locking/CRDT reden (Phase 3, unverändert).
