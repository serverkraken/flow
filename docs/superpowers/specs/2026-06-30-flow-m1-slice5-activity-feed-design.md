# flow M1 Slice 5 — Activity-Feed + Actor — Design Spec

- **Datum:** 2026-06-30
- **Branch:** `m1-webui` (Worktree off `rebuild`)
- **Status:** Draft — zur Review
- **Parent-Spec:** `docs/superpowers/specs/2026-06-29-flow-m1-projekt-zentrik-webui-design.md` (M1-Übersicht, §5 Logstream + §9 Activity-Feed + Actor + §15 offene Punkte).
- **Vorgänger-Slices (fertig im Worktree, ungemerged):** Slice 1 (Worktime-Modell), Slice 2 (Kristall-Identität), Slice 3 (Shell & IA-Reframe), Slice 4 (Home: Timer-Hero + Stats + neueste Wissensartikel).
- **Methode:** Brainstorm → diese Spec → `writing-plans`.

---

## 1. Kontext & Ziel

flow ist ein **multi-tenant** Wissens- + Worktime-Produkt für **Menschen UND AI-Agents** (über den MCP-Server). Slice 4 hat das Home-Dashboard gebaut (Timer-Hero, Saldo-Kacheln, Burndown, neueste Wissensartikel). Was fehlt, ist der in §5/§9 der M1-Übersicht versprochene **Logstream**: ein persistierter Aktivitäts-Feed, der zeigt „**wer** hat **was** **wann** getan" — mit einer sichtbaren Unterscheidung zwischen **Mensch** und **AI-Agent**.

Heute ist jede Mutation flüchtig: Sie wird über den In-Memory-`EventBus` als SSE an offene Browser gepusht und ist danach weg. Es gibt **keine Historie** und **keine Actor-Information** — Mensch-TUI und MCP-Server treffen den REST-Server mit demselben Bearer-Token.

Slice 5 macht aus den flüchtigen Events ein **persistiertes, owner-scoped, paginiertes Aktivitätslog**, schreibt einen **Actor** an jeden Eintrag und rendert daraus den **Home-Logstream** mit Klassen- und Actor-Filter.

### Erfolgskriterien
1. Jede bedeutsame Mutation (Sessions, Dokumente, Nodes, Frei) erzeugt einen persistierten `activity`-Eintrag mit Actor, Zielangabe und lesbarem Label.
2. Der Eintrag trägt **Actor = Auth-Subjekt**: `actor_kind {human|agent}` (steuert den Glyph) + `actor_ref` (das Label — DisplayName bzw. konkreter Agent-Name wie `claude-code`).
3. Ein **MCP-Agent** wird als solcher erkannt und mit seinem Client-Namen attribuiert (`claude-code`, `gemini-cli`, `codex`, …) — ohne Authentik-Umbau.
4. Der **Home-Logstream** rendert die jüngsten Einträge: Actor-Glyph (Mensch = Kreis, Agent = Hexagon), Verb, verlinktes Ziel, relative Zeit — Kristall-Look, light+dark, deutsch, keine Emojis.
5. **Filter:** nach Ereignis-Klasse (Zeit/Wissen/Struktur/Frei) **und** nach Actor (ich · je Agent).
6. **Live:** der Logstream aktualisiert sich SSE-getrieben, ohne Page-Reload.
7. `make ci` grün (inkl. `verify-css`, `verify-no-popups`); Live-Done-Gate vs. Postgres+Dex.

---

## 2. Nicht-Ziele (Slice 5)

- **Kein Cockpit-Activity-Tab.** Slice 5 schreibt das `node_ref`-Feld mit, damit Slice 6 (Cockpit) per-Node filtern kann; die **UI** dafür ist Slice 6 (das Cockpit entsteht dort). (Beantwortet §15 „Cockpit-Activity-Tab ja/nein": Daten ja, UI nein.)
- **Kein Retention-Cap / kein Prune-Job.** Owner-scoped, single-user, indexiert, paginiert → YAGNI. Erst einführen, wenn das Log real groß wird.
- **Kein Backfill.** Der Feed startet leer und füllt sich ab Deploy; bestehende Sessions/Dokumente/Nodes erzeugen **keine** rückwirkenden Einträge. (Explizit vermerkt, damit niemand das als Bug liest.)
- **Kein TUI-Activity-Feed.** Die REST-API liegt bereit (für künftige Konsumenten), aber die einzige Surface in Slice 5 ist der WebUI-Home-Logstream.
- **Kein Authentik-/OIDC-Umbau.** Die Actor-Unterscheidung läuft über einen Request-Header (§5), nicht über einen separaten OIDC-Client oder Custom-Claim.
- **`settings.changed` wird nicht geloggt** (reiner Konfigurations-Lärm) — es wird weiterhin nur publiziert.

---

## 3. Datenmodell

Neue Tabelle, Migration **`0024_activity_log.sql`** (höchste bestehende ist `0023`). Goose-annotiert ([[feedback_pgstore_goose_migrations]]):

```sql
-- +goose Up
CREATE TABLE activity (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT        NOT NULL,
    actor_kind  TEXT        NOT NULL,        -- 'human' | 'agent'  → Glyph
    actor_ref   TEXT        NOT NULL,        -- 'Soenne' | 'claude-code' | …  → Label
    kind        TEXT        NOT NULL,        -- roher EventType: 'session.started', 'document.updated', …
    target_ref  TEXT,                        -- ID des betroffenen Objekts (nullable)
    label       TEXT,                        -- denormalisierter Snapshot (Doc-Titel / Node-Name)
    node_ref    TEXT,                        -- optionaler Scope-Node (für Slice-6 Cockpit-Filter)
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX activity_owner_at ON activity (owner_id, at DESC);
-- +goose Down
DROP TABLE activity;
```

Spiegelnd:
- `internal/domain/activity.go` — `ActivityEntry` (Felder 1:1, `at time.Time`, `target_ref`/`label`/`node_ref` als `*string`).
- `internal/ports/ports.go` — `ActivityStore` mit `Append(ctx, ActivityEntry) error` und `ListPage(ctx, ownerID string, classes []string, actorRef *string, limit, offset int) (items []ActivityEntry, total int, err error)`.

**Designnotizen:**
- `kind` ist der **rohe EventType-String** — die Filter-Klasse ist sein Prefix (`session`/`document`/`node`/`dayoff`), kein zusätzliches `target_kind`-Feld nötig.
- `label` ist ein **Snapshot zum Zeitpunkt der Aktion**. Begründung: Bei `*.deleted` existiert das Zielobjekt danach nicht mehr → ein nachträglicher Lookup wäre unmöglich. Der Snapshot hält den Feed auch nach Löschungen lesbar.
- `ListPage` filtert serverseitig nach `classes` (kind-Prefix-Match, leer = alle) und optional `actorRef` — beide Filter teilen denselben paginierten Query.

---

## 4. Capture — der DRY-`emit`-Choke-Point

Heute publizieren HTTP-Handler (und das `AddDayOffs`-Usecase) Mutationen via `Bus.Publish(ev)`. Slice 5 ersetzt diesen Aufruf durch **einen einzigen** Emit-Pfad, der publiziert **und** (selektiv) persistiert — DRY, kein zweiter `recordActivity`-Aufruf nebenher.

- **`Emitter`** — ein dünner Typ, der `EventBus` + `ActivityStore` + die Policy `activityFor` komponiert. Eine Methode: `Emit(ctx, ev domain.Event)`.
  - Ablauf: `bus.Publish(ev)` (SSE wie bisher) → `entry, ok := activityFor(ctx, ev)` → falls `ok`: `store.Append(ctx, entry)` **und** `bus.Publish(EventActivityLogged)` (für den Live-Refresh, §7).
  - Verortung: `internal/adapter/sse/` (neben `bus.go`) oder eigenes Paket; in `main.go` gewired und überall dort injiziert, wo heute der Bus injiziert ist.
- **`activityFor(ctx, ev) (domain.ActivityEntry, bool)`** — die **einzige** Stelle, die Einträge konstruiert:
  - mappt `ev.Type` → `kind`; liefert `ok=false` für `settings.changed` (nur Publish);
  - liest den Actor via `actor.FromContext(ctx)` (§5);
  - zieht `target_ref` / `label` / `node_ref` aus `ev.Data`.
- **Call-Site-Migration:** alle `Bus.Publish(ev)` → `Emit(ctx, ev)` (mechanisch). **Die einzige verteilte Zusatzarbeit:** wo `ev.Data` heute nur `{"id": …}` enthält, wird es um `title`/`name` (+ ggf. `node_ref`) angereichert, damit `activityFor` ein lesbares Label bauen kann. Die Eintrags-*Konstruktion* bleibt zentral — DRY gewahrt.

**Vollständigkeits-Sicherung:** Da `emit` der einzige Mutations-Broadcast-Pfad ist und alle Mutationen (auch CLI/TUI/MCP) über die REST-Handler laufen, deckt ein Test, der je bedeutsamer Mutation einen `Append` erwartet, das „vergessene Call-Site"-Risiko ab.

---

## 5. Actor-Ableitung

Es gibt heute **keine** Infrastruktur, Mensch von MCP-Agent zu unterscheiden — beide kommen mit demselben Device-Flow-Bearer-Token des Users (`SkipClientIDCheck: true`, kein `azp`, kein `actor_type`). Slice 5 führt die Unterscheidung über einen **Request-Header** ein (single-user, self-hosted → Spoofing-Risiko irrelevant; kein Authentik-Umbau):

- **Middleware** (direkt nach `EnsureUser`): liest `X-Flow-Actor`.
  - Header vorhanden → `actor_kind=agent`, `actor_ref=`Header-Wert.
  - Header fehlt → `actor_kind=human`, `actor_ref=`User-DisplayName.
  - Ergebnis als kleiner `actor.Actor` in den ctx (neuer `actorKey`); `actor.FromContext(ctx)` liest ihn (vom `Emitter` genutzt). Ein geteilter ctx-Helfer (`internal/actor` oder bei der Middleware), damit der `Emitter` (außerhalb httpserver) ihn ohne Import-Zyklus erreicht.
- **MCP-Client** (`cmd/flow-mcp`): erfährt seinen Aufrufer aus dem MCP-`initialize`-Handshake — `clientInfo.name` (Claude Code → `claude-code`, Gemini CLI / Codex → ihre Namen). `cmd/flow-mcp` setzt damit `X-Flow-Actor` beim Bau des `apiclient.Client`.
  - **Override/Fallback:** Env `FLOW_ACTOR` überschreibt, falls ein Client einen nichtssagenden `clientInfo.name` sendet.
  - **Verifikation im Plan:** der exakte Zugriff auf `clientInfo` im `modelcontextprotocol/go-sdk` v1.6.1 wird in der ersten Plan-Task verifiziert; der `FLOW_ACTOR`-Env-Pfad ist der robuste Notnagel, falls das SDK den Wert nicht zugänglich macht.

Damit differenziert der Feed sauber zwischen dir und **jedem** deiner Agents (Claude/Gemini/Codex), ohne pro Client etwas konfigurieren zu müssen.

---

## 6. Feed-API

Volle vertikale Scheibe, konsistent mit dem Rest des Systems:
- **Usecase** `internal/usecase/list_activity.go` — `ListActivity` (thin delegation → `ActivityStore.ListPage`, gibt `(items, total, err)` wie Sessions/Dokumente).
- **REST** `internal/adapter/httpserver/activity.go` — `GET /api/v1/activity?limit&offset&class&actor` (auth/bearer). Antwort: **flaches JSON-Array** (`[]ActivityEntry`), keine `{items,total}`-Hülle — folgt der bestehenden Konvention (Total nur WebUI-seitig via `components.PageNav`). Für künftige TUI/MCP-Konsumenten.
- **WebUI:** der Home-Handler ruft das `ListActivity`-Usecase **direkt** (wie `homeDataFor` die übrigen Usecases), nicht über HTTP.

---

## 7. Home-Logstream (UI)

Neue `<section id="logstream">` in `home.templ`, platziert zwischen Burndown-Banner und „Zuletzt im Wissen". Kompakt die jüngsten ~15 Einträge (volle Pagination liegt in der REST-API für eine spätere dedizierte Seite/TUI).

```
 Aktivität                          [Alle] [Zeit] [Wissen] [Struktur] [Frei]   [Alle ▾]
 ───────────────────────────────────────────────────────────────────────────────────
 ⬡ claude-code   bearbeitete   Quartalsreport                       vor 3 Min
 ○ Soenne        startete Timer                                     vor 12 Min
 ⬡ gemini-cli    legte Projekt an   serverkraken/flow               vor 1 Std
 ○ Soenne        buchte 2h 15m auf   template-apps-demo             vor 2 Std
```

- **Actor-Glyph** — neue Mini-Komponente (nichts Wiederverwendbares existiert; nur die `BrandMark`-Hexagon-SVG im AppShell): `○` Kreis = Mensch, `⬡` Hexagon = Agent (knüpft an die BrandMark-Hexagon-Sprache an). SVG/geometrisch, keine Emojis (Kristall-Regel), light+dark, WCAG-AA.
- **Verben** via i18n (DE primär, EN vollständig): `activity.session.started` → „startete Timer", `activity.session.stopped` → „buchte … auf", `activity.document.updated` → „bearbeitete", `activity.document.created` → „legte an", `activity.node.created` → „legte Projekt an", `*.deleted` → „löschte", etc. Mapping `kind` → i18n-Key.
- **Target** verlinkt: Dokument → `/wissen/{id}`; Node → Projekt (Cockpit-Link, in Slice 6 real); Session/Dayoff → ggf. Zeit-/Frei-Fläche oder unverlinkt.
- **Klassen-Filter-Chips** `[Alle][Zeit][Wissen][Struktur][Frei]` — htmx: `GET /ui/home/logstream?class=wissen` swappt nur die Section.
- **Actor-Filter** `[Alle ▾]` (ich · je Agent aus den vorhandenen `actor_ref`) — gleicher htmx-Mechanismus, zweiter Query-Param (`actor=`). Der eigentliche Payoff der Actor-Unterscheidung.

---

## 8. Realtime

Statt die bestehende Home-SSE-Trigger-Liste um `node.*`/`dayoff.*` zu erweitern, **ein** neuer Event:
- `internal/domain/event.go` — `EventActivityLogged = "activity.logged"`.
- `Emit` feuert ihn, sobald ein Eintrag persistiert wurde.
- Die Logstream-Section trägt `hx-trigger="sse:activity.logged"` und lädt **nur sich selbst** granular neu (nicht das ganze `#content`). Ein Trigger deckt alle geloggten Mutationen ab — DRY.

---

## 9. Testing & Done-Gate (TDD)

- **pgstore** (Docker-pgstore-Tests): `Append` + `ListPage` (Klassen-Prefix-Filter, Actor-Filter, Pagination/Total, `at DESC`-Ordnung, Owner-Isolation).
- **`activityFor`-Policy:** pro EventType → erwarteter `kind`/`target_ref`/`label`/`node_ref`; `settings.changed` → `ok=false`.
- **Middleware:** `X-Flow-Actor` vorhanden → `agent`/ref; fehlt → `human`/DisplayName; `actor.FromContext` round-trip.
- **`Emit`:** persistiert **und** publiziert (Fake-Bus + Fake-Store); `settings.changed` → nur Publish, kein Append, kein `activity.logged`.
- **REST:** `GET /api/v1/activity` paginiert, owner-scoped, Klassen-/Actor-Filter, 200/JSON-Array.
- **WebUI-home:** Logstream rendert Einträge; Glyph je `actor_kind`; **beide** Filter (Klasse + Actor) via htmx-Section-Swap; i18n DE+EN vollständig.
- **Build-Disziplin:** `.templ`→`make generate`+commit `_templ.go`; `tailwind.css`→`make web`+commit `app.css`; `verify-css`/`verify-no-popups` grün ([[feedback_tailwind_v4_templ_gotchas]]).
- **`make ci` grün** — aktuelles Coverage-Gate halten (Zahl in der ersten Plan-Task ablesen; nicht mit Fake-Tests pumpen).
- **Live-Done-Gate vs. Postgres+Dex** ([[reference_flow_dev_env]]):
  1. Mensch startet Timer (WebUI/TUI) → Eintrag `○ Soenne, startete Timer`.
  2. **MCP-Agent** (Claude — und idealerweise Gemini/Codex) schreibt ein Dokument → Eintrag `⬡ claude-code, bearbeitete …`.
  3. Klassen- **und** Actor-Filter greifen.
  4. SSE-Live-Append (`activity.logged`) ohne Page-Reload.
  5. Dokument löschen → Eintrag behält den Label-Snapshot („löschte *Titel*").
- **Holistic-Review (Opus)** je Slice; **Wiring-Verifikation** (main.go-Routen registriert + curl-Smoke je Route) als Abschluss-Task ([[feedback_plan_main_wiring_task]]).

---

## 10. Touchpoint-Karte (für `writing-plans`)

| Bereich | Dateien |
|---|---|
| Event | `internal/domain/event.go` (`EventActivityLogged`) |
| Domain | `internal/domain/activity.go` (`ActivityEntry`) |
| Actor-ctx | `internal/actor/` (oder bei der Middleware) — `Actor`, `WithContext`, `FromContext` |
| Port | `internal/ports/ports.go` (`ActivityStore`) |
| Store | `internal/adapter/pgstore/migrations/0024_activity_log.sql`, `internal/adapter/pgstore/activity.go` |
| Emitter | neuer `Emitter`+`activityFor` (`internal/adapter/sse/` o. eigenes Paket) + `main.go`-Wiring |
| Usecase | `internal/usecase/list_activity.go` |
| Middleware | `internal/adapter/httpserver/middleware.go` (Actor in ctx) |
| REST | `internal/adapter/httpserver/activity.go` + Route in `server.go` |
| Call-Sites | **alle `Bus.Publish`→`Emit(ctx, ev)`** + `ev.Data`-Label-Anreicherung (`worktime.go`, `documents.go`, `webui_editor.go`, `add_dayoffs.go`, node-Handler) |
| WebUI | `webui_home.go` (+ `/ui/home/logstream`-Handler), `webui/home.templ`, `webui/home_vm.go`, neue `actor_glyph`/`logstream`-Komponenten, i18n `catalog_de.go`/`catalog_en.go`, `static/app.css` |
| MCP | `cmd/flow-mcp/` (clientInfo→`X-Flow-Actor`, `FLOW_ACTOR`-Override) |

---

## 11. Offene Detail-Entscheidungen (in der Plan-Erstellung zu klären)

- **go-sdk `clientInfo`-Zugriff:** exakter API-Pfad in `modelcontextprotocol/go-sdk` v1.6.1 (sonst `FLOW_ACTOR`-Env-Pfad).
- **`Emitter`-Paket:** in `internal/adapter/sse/` neben `bus.go` vs. eigenes Paket — abhängig von Import-Zyklen mit dem Actor-ctx-Helfer.
- **Label-Quelle je Event:** welches `ev.Data`-Feld pro Mutation den Snapshot liefert (Doc-Titel, Node-Name, Session→Projekt-Name); inkl. der Stellen, die heute kein Label mitgeben.
- **`node_ref`-Befüllung:** welche Mutationen einen Scope-Node tragen (Session-Booking → gebuchter Node; Doc → `node_id`; Node-Mutation → der Node selbst).
- **Verb-Texte:** finaler DE/EN-Wortlaut je `kind`.
