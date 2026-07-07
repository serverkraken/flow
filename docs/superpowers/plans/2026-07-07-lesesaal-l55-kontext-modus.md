# Lesesaal L5.5 — Kontext-Modus pro Dokument (`auto` · `immer` · `nie`) + Cockpit-Kurzbeschreibung Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** L5 (Kontext-Kuratierung, 15 Commits auf `lesesaal-l5`, alle reviewt) machte das Kontext-Backend sichtbar — offenbarte aber eine **Design-Falle**: bei GLOBALEN memory-Docs (`node_id NULL`) ist der **Pin die einzige Mitgliedschafts-Eintrittskarte** in fremde Ketten (`compose_context.go:215` `globalAllowed[d.ID] || d.Pinned`). Der Pin-Toggle auf der Kuratieren-Seite macht Entpinnen damit zur **Einbahnstraße**: Doc fliegt aus dem Compose, Zeile verschwindet MITSAMT dem Rückweg. Soenne hat so **zweimal** seine 8 globalen Feedback-Memories aus dem Kontext geworfen (Ledger-Eintrag „PIN-DATENVERLUST-Analyse"). L5.5 **entkoppelt Mitgliedschaft von Pin** durch einen expliziten **Kontext-Modus pro Dokument** — `auto` (Bestandsverhalten), `immer` (garantiert enthalten, uncapped, tag-gate-frei), `nie` (nie komponiert, aber voll sichtbar in Wissen/Suche). Damit bestimmt Soenne selbst, WAS „immer enthalten" ist (heute hart am Typ instruction/activecontext), und die 8 Globals werden per einmaligem Daten-Nachlauf auf `immer` gesetzt — ihr Pin-Zustand wird irrelevant. **Zweiter Slice-Inhalt (Soenne-Scope-Add):** die **Projekt-Kurzbeschreibung** wieder anzeigen — `webui_cockpit.go:42` berechnet `d.DescriptionHTML`, aber KEIN Template konsumiert es (toter Code seit dem L2-Cockpit-Rebuild); Soenne dampft die Node-Descriptions gerade zu Kurz-Einzeilern ein und will sie als schlichte Textzeile in der Cockpit-Identität sehen.

**Architecture:** Server-rendered wie gehabt (templ + htmx + Tailwind, kein SPA, kein Node). **Additiv** — Default `auto` ist **verhaltensneutral zum Bestand** (alle heutigen Docs bekommen per DB-Default `auto` → identisches Compose, alle Bestand-Compose-Tests grün). Sechs Schichten:
1. **Feld:** `documents.context_mode TEXT NOT NULL DEFAULT 'auto'` (Migration `0030`, CHECK auf die drei Werte), `domain.ContextMode`-Typ + `Document.ContextMode`, pgstore-Spaltenlisten + **vier** Reader + `Create`-Arity ($19) + `SetContextMode`-Storemethode, Port-Erweiterung, alle Fakes.
2. **Compose-Semantik:** die reine `Compose` verzweigt pro Doc auf den Modus — `nie` → gesammelt in neuer `Hidden`-Liste (0 Extra-Queries), nie in Used/Ranked/Memories/Always; `immer` → neuer **Always-Tier-Topf `AlwaysMemories`** (uncapped, zählt in Used, bypasst Tag-Gate/Pin); `auto` → exakt heutige Logik. `StandingOf` kennt `AlwaysMemories` (→ "always"); `ContextItem.ContextMode` trägt den Modus in jede VM.
3. **Mutation:** `usecase.SetContextMode` + REST `POST /api/v1/documents/{id}/context-mode` + apiclient-Methode + Wiring; emittiert `document.updated`. (CLI/MCP-Verben Deferred.)
4. **UI-Umschalter:** ein `.seg .seg-sm`-Dreisegment (auto/immer/nie) an JEDER Kuratieren-Zeile (Rang-Liste, Immer-enthalten-Abschnitt, neuer Ausgeblendet-Abschnitt) UND im Kontext-Block der Dokument-Seite; Web-Handler `POST /kontext/{id}/mode` + `POST /wissen/{id}/mode`; i18n de+en; SSE `document.updated`.
5. **Cockpit-Kurzbeschreibung:** der tote `DescriptionHTML`-Pfad wird zu einer schlichten Plaintext-Identitätszeile im Cockpit-Head (kein RenderDocument mehr); optional Untertitel im Projekte-Baum.
6. **Wiring-Gate:** Composition-Root-Verify, Rest-Sweep, `make ci`, Live-Smoke, Breakpoints, **Daten-Nachlauf-Dokumentation** (8 Globals → `immer`, dev+PROD, owner-scoped SQL — kein goose).

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · Schibsted Grotesk + JetBrains Mono. **Eine** neue goose-Migration (`0030`); keine neuen Abhängigkeiten, kein neues Vendoring, kein Client-JS.

**Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (Kontext-Inspektor/Lifecycle: Docs treten in den Kontext ein/aus — genau die Mitgliedschafts-Kontrolle, die L5.5 explizit macht) + Lesesaal-Spec `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` §10 (Kontext-Instrument, „enthalten/verworfen/immer"). **Kein Mockup** für den Modus-Umschalter → Layout ist Empfehlungs-Entscheid (Offene Entscheidungen), nach Lesesaal-Doktrin geschnitten. Formatvorbild: `docs/superpowers/plans/2026-07-07-lesesaal-l5-kontext-kuratierung.md`.

**Basis:** L5.5 baut BEWUSST auf dem **ungemergten** L5-Branch `lesesaal-l5` (HEAD `e471fba`) weiter — ein gemeinsames Gate+Merge nach L5.5 (Ledger `.superpowers/sdd/progress.md`).

---

## Global Constraints

- Branch **`lesesaal-l5`** (HEAD `e471fba`, bereits ausgecheckt); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende. Kein separater L5.5-Branch — L5 ist noch nicht gemergt, das Slice-Gate umfasst L5+L5.5 gemeinsam (`rebuild..HEAD`).
- **L4/L5-LEHREN — in JEDEN Task-Dispatch-Text aufnehmen:** (1) **Tests/CI SYNCHRON foreground**, **NIEMALS `run_in_background`** (Subagenten warten sonst auf nie kommende Notifications). (2) **Erst `git add -A` stagen, dann `make ci`** (verify-generate/verify-css diffen gegen den Index → uncommitted templ/css false-positiv). (3) **Nie zwei `make ci` parallel** (Podman-VM keilt bei parallelen Testcontainer-Läufen → Hard-Stop+Start). (4) **`make web` nach JEDER `.templ`-Änderung** (auch reine Klassennutzung ändert den Tailwind-Scan; verify-css ist ein Drift-Diff) und `internal/adapter/webui/static/app.css` mitcommitten. (5) **`make generate` nach JEDER `.templ`-Änderung**, die `*_templ.go` mitcommitten.
- **Seiten-Hülle-Lehre (L5 Hull-Fix, LIVE-Befund):** jede neue/geänderte Vollseite = `components.Base(aktiv, body)` → AppShell-Body (Zweischicht wie `document.templ`/`kontext.templ`). Ein Render-Test MUSS `<!DOCTYPE` **und** den `app.css`-Link asserten, nicht nur Fragment-Inhalte (L5.5 baut keine neue Vollseite, aber Task 4/5 ändern Bestands-Seiten — die bestehenden Hüllen-Asserts nicht brechen).
- **NIE `make fmt`**. **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen (Memory).
- `make ci` muss am Task-Ende grün sein (Gate 75 %, aktuell ~85,7 %; `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket). **Task 1 fügt Migration 0030 hinzu → die pgstore-Docker-Tests laufen gegen das neue Schema; Migration 0030 muss goose Up/Down-annotiert sein** (Memory: nur die pgstore-Docker-Tests fangen fehlende Annotationen ODER eine CHECK-Verletzung).
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced (`TestCatalogsParity` prüft nur Key-**Existenz** — EN-Strings **explizit ausformulieren**, nicht „gleichwertig"). Keine hartkodierten Anzeige-Strings; `components.T(ctx, "key")`/`Tn`.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ✚ ✗ ✓ ○ · ↑ ↓ + SVG erlaubt), **keine Browser-Popups** (`verify-no-popups`). Die Modus-Segmente sind `.seg .seg-sm`-Links/Buttons, kein `confirm()`.
- **owner-scoped überall** (jede Store-/Compose-/Mutation-Query trägt `u.ID`/`ownerID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze). Jede neue Datenfläche bekommt einen **Owner-Scope-Negativtest**: `SetContextMode` auf ein Fremd-Dokument → `ErrDocumentNotFound`; die REST-/Web-Handler eines Fremd-Docs sehen keinen Effekt; `ExecuteForNode`/Compose liefern nie fremde Docs.
- **SSE-Regel (Mutation → Event → Konsument benannt):** jede Modus-Mutation emittiert `domain.EventDocumentUpdated` (`"document.updated"`) über `s.Emitter.Emit`. Konsumenten (alle Bestand nach L5): `#document-fragment` (`sse:document.updated`), `#cockpit-rail` (hat `document.*` seit L5-T5), `#cockpit-main` (`document.*`), das Kuratieren-`#content` (`sse:document.updated`). **Kein neuer Event-Typ** (Offene Entscheidung #3). Die Cockpit-Kurzbeschreibung ist read-only Anzeige — ihre Mutation (Node-Edit) emittiert `node.updated` (Bestand, `webui_nodes.go:405/433`), worauf der Cockpit-Head bereits reloadet.
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt): der Modus-Umschalter nutzt Bestand `.seg`/`.seg-sm` (tailwind.css:335-337/486, `aria-pressed="true"` = aktives Segment via `--surface`/`--blue`). Keine Arbitrary-`[#hex]`/`[px]`, wo eine benannte Klasse existiert. Farben über `rgb(var(--token))`. Neue Kurzbeschreibungs-Zeile = benannte Klasse (`.spine-desc` o. ä.) + Containment (`truncate`/`min-w-0` — Spec §11), kein Arbitrary.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht, jeder Task hat Step 0):** JEDES als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Usecase-Feld, Store-Methode, Test-Helper, i18n-Key, CSS-Klasse — u. a. `docCols`, `prefixedDocCols`, `scanDocument`, `scanSearchHit`, `scanSemanticHit`, `StaleDocuments`, `Create`, `UpsertByPath`, `ListForContext`, `SetPinned`, `SetPriority`, `ErrDocumentNotFound`, `Compose`, `ComposedContext`, `ContextItem`, `itemOf`, `StandingOf`, `RankedItem`, `ExecuteForNode`, `composeForChain`, `globalAllowed`, `BuildCockpitContext`, `CockpitContextVM`, `AlwaysN`, `BuildKontextVM`, `KontextVM`, `KontextRowVM`, `KontextAlwaysVM`, `kontextRow`, `kontextAlwaysRow`, `KontextFragment`, `handleWebKontextPin`, `kontextDataFor`, `renderKontext`, `BuildDocContext`, `DocContextVM`, `isContextType`, `buildDocumentVM`, `DocumentVM`, `DocumentFragment`, `handleWebDocPin`, `handlePinDocument`, `nodeCockpitData`, `NodeCockpit`, `DescriptionHTML`, `CockpitHead`, `cockpitIdentity`, `NodeAvatar`, `ShortName`, `RenderDocument`, `EventDocumentUpdated`, `EventNodeUpdated`, `.seg`, `.seg-sm`, `.blk`, `.krow`, `.row`, `.narrow`, `.spine`, `.spine-main`, `.spine-meta`, `document.pin`, `context.rail.always`) vor dem Tippen per `rg -n "<Name>" internal/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden.
- **Spaltenlisten-Lehre — VERSCHÄRFT für context_mode (KRITISCH, Task 1):** eine Änderung an `docCols`/`prefixedDocCols` bricht **VIER Reader** — `scanDocument` (:557), `scanSearchHit` (:400), `scanSemanticHit` (:513) UND den **Inline-Scan `StaleDocuments`** (`documents_embed.go:41-43`, der 4. Leser aus der L5-Lehre) — plus die **arity-gekoppelte INSERT-Klausel von `Create`** (:92-108: `VALUES ($1..$18)` + 18 Args nach L5 → context_mode macht `$19` + ein 19. Arg). `Update` (:174-190) nutzt docCols nur im RETURNING (kein Arity-Change). `UpsertByPath` (:248) hat eine EIGENE Spaltenliste + `RETURNING id, updated_at` → DB-Default sicher. **NEU GEGENÜBER priority (Zero-Value-Falle):** priority-Zero-Value `0` ist gültig; context_mode-Zero-Value `""` **verletzt den CHECK-Constraint** (`'' NOT IN ('auto','immer','nie')`). Deshalb MUSS `Create` den Wert **koaleszieren** (`d.ContextMode.OrAuto()` → nie `""` binden); der Domain-Helfer `OrAuto()` liefert `auto` bei leerem Wert. `go build ./...` + der pgstore-Test fangen jede Fehlstellung (Scan-Arity, Placeholder-Zahl, CHECK-Verletzung).
- **Interface-Ripple:** `SetContextMode` an `ports.DocumentStore` zwingt **jede** `ports.DocumentStore`-Fake zur Methode (verifiziert: `internal/testutil/fakes.go`; Compiler listet weitere Inline-Fakes). Task 1 fügt sie überall hinzu (`rg -rn "func.*SetPriority\(ctx" internal --glob '*_test.go'` + `go build ./... ./internal/...`).

## Kontext-Modus-Semantik — Vorgabe (ENTSCHIEDEN, NICHT erneut konsultieren)

Die drei Werte (canonical DB/Domain-Werte **`auto` / `immer` / `nie`** — siehe Offene Entscheidung #1):

| Modus | Compose-Wirkung | Sichtbarkeit sonst |
|---|---|---|
| **`auto`** (Default) | **exakt heutige Logik**: instruction/activecontext → Always-Tier; memory node-in-chain → Pool; memory global → Pool nur wenn `globalAllowed[d.ID] \|\| d.Pinned` (D7-Tag-Gate mit Pin-Bypass). Verhaltensneutral zum Bestand. | normal |
| **`immer`** | **Always-Tier `AlwaysMemories`**: uncapped, zählt in `Budget.Used`, **bypasst Tag-Gate UND Pin** (garantiert enthalten — **stärker als Pin**: ein Pin kann bei Budget-Überlauf droppen, `immer` NIE). Auch für global memory. Erscheint im „Immer enthalten"-Abschnitt, NICHT in `Ranked`. | normal |
| **`nie`** | **NIE komponiert** (weder Pool noch Always noch Used); gesammelt in `Hidden` (nur für die Kuratieren-Wiederherstellung, 0 Extra-Queries). **Bewusst anders als `archived`**: archived = überall weg; `nie` = nur aus dem Agenten-Kontext, in Wissen/Listen/Suche **voll sichtbar**. | **voll sichtbar** |

**Disposition pro Doc (der eine Compose-Switch):**
```
mode := d.ContextMode.OrAuto()
if mode == nie:            → Hidden (sammeln), continue   // nie in Used/Ranked/Memories/Always
alwaysByType := d.Type==instruction || (d.Type==activecontext && tier[node]=="leaf")
if mode == immer || alwaysByType:  → Always-Tier
    instruction         → Instructions
    activecontext(leaf) → ActiveContext
    sonst (immer memory, immer non-leaf activecontext) → AlwaysMemories
else:                     → Pool (memory, auto — Tag-Gate/Pin wie Bestand)
```
`immer` global memory umgeht `globalAllowed`/Pin **komplett** (das ist der Fix für Soennes 8 Globals). **Default `auto` = verhaltensneutral** (jeder Doc ohne expliziten Modus verhält sich wie vor L5.5; die Bestand-Compose-Table-Tests bleiben grün). Ein sehr großes `immer`-Doc kann das Budget füllen und alle `auto`-Docs droppen — das ist die bewusste Kurator-Entscheidung (dokumentiert, wie instructions es längst tun).

---

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1–L5)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. Dispatches nennen das Modell NIE implizit (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 `documents.context_mode` (Migr 0030 CHECK · Domain ContextMode/OrAuto · pgstore 4 Reader + Create-Arity + SetContextMode · Port · Fakes) | `lesesaal-implementer-deep` | Sonnet · high |
| 2 Compose-Semantik (nie→Hidden · immer→AlwaysMemories · ContextItem.ContextMode · StandingOf · default-auto-neutral) | `lesesaal-implementer-deep` | Sonnet · high |
| 3 SetContextMode-Usecase + REST `/api/v1/documents/{id}/context-mode` + apiclient + Wiring | `lesesaal-implementer` | Sonnet · medium |
| 4 UI-Modus-Umschalter (Kuratieren-Zeilen/Always/Hidden + Doc-Seite-Block + Web-Handler + Cockpit-AlwaysN + i18n + SSE) | `lesesaal-implementer-deep` | Sonnet · high |
| 5 Cockpit-Kurzbeschreibung (toter DescriptionHTML → Plaintext-Identitätszeile + optional Baum-Untertitel) | `lesesaal-implementer` | Sonnet · medium |
| 6 Wiring-Gate (main.go · Sweep · make ci · Live-Smoke · Breakpoints · Daten-Nachlauf-Doku) | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch (L5+L5.5, `rebuild..HEAD`) | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Semantik-Vorgabe + „Branch `lesesaal-l5`, HEAD-basiert, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch. **Explizit im Dispatch:** „Tests/`make ci` SYNCHRON foreground, keine Hintergrund-Läufe; erst `git add -A`, dann `make ci`; nie zwei `make ci` parallel."
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range (BASE = Task-Base). `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge, umfasst L5+L5.5):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch, Dispatch-Text unten) über `git diff --name-only rebuild..HEAD`.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen. **Fokus L5.5:** Owner-Scoping über SetContextMode/alle Handler; `auto`-Default-Neutralität (Bestand-Compose-Tests unverändert grün?); die VIER Reader + Create-Arity + CHECK; **`AlwaysMemories` wird in JEDEM `ComposedContext`-Konsumenten gerendert — WebUI (Cockpit-AlwaysN, Kuratieren-Always-Abschnitt, StandingOf) UND der `cmd/flow/context.go`-Markdown-Renderer + etwaige MCP-Pendants** (agy-Fund #1 — sonst erreichen `immer`-Docs den Agenten nie); `immer`-uncapped-Verbuchung in Used konsistent; `nie`-Wiederherstellbarkeit real (Doc-Seite + Hidden-Abschnitt); SSE-Trigger je Modus-Mutation; Interface-Ripple (alle Fakes `SetContextMode`).
4. `lesesaal-mockup-auditor` → der Modus-Umschalter hat **kein Mockup** → Prüfung gegen Lesesaal-Doktrin (leise Bestand-Elemente, kein neues Farbsystem, keine Emojis, `.seg`-Konsistenz, Containment 375px), nicht gegen Pixel.
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — inkl.: Doc-Seite zeigt Modus-Umschalter (auto/immer/nie), Umschalten togglet + Cockpit-Meter/Rang ziehen live (SSE) nach; ein `immer`-Doc erscheint im „Immer enthalten"-Abschnitt (Cockpit + Kuratieren) und **droppt nie** (auch bei vollem Budget); ein `nie`-Doc verschwindet aus Compose, bleibt in Wissen/Suche sichtbar und ist per Doc-Seite UND Kuratieren-Ausgeblendet-Abschnitt wiederherstellbar; die 8 Globals nach Daten-Nachlauf auf `immer` (Pin-Zustand egal); Cockpit-Head zeigt die Kurzbeschreibung; 960px/375px-Sichtprobe (Umschalter bricht nicht, kein horizontales Pannen).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **verwaiste i18n-Keys** (in beiden Katalogen definiert, nirgends per `T(`/`Tn(` referenziert) — besonders `context.mode.*`/`document.context.mode`-Keys; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `w-[`, `h-[`) auf den L5.5-Flächen (kontext/document/cockpit_rail/cockpit_head), wo eine benannte Lesesaal-Klasse existiert; (c) **verwaiste Symbole** mit null Konsumenten unter den L5.5-Neubauten (`SetContextMode`, `ContextMode`, `OrAuto`, `AlwaysMemories`, `Hidden`, `DocContextVM.Mode`, `KontextHiddenVM`); (d) **Semantik-Regressionen:** ist der `auto`-Pfad in Compose unverändert (jeder Bestand-Doc = auto = altes Verhalten)? zählt `immer` in `Budget.Used` und droppt NIE? ist `nie` aus Used/Ranked/Memories/Always komplett draußen? (e) **SSE-Lücken:** emittiert jede Modus-Mutation (REST, Web-Kontext, Web-Doc) `document.updated`? (f) **toter DescriptionHTML-Rest:** ist der `RenderDocument`-Pfad in `nodeCockpitData` entfernt/umgewidmet und nirgends ein verwaistes `DescriptionHTML`-Feld übrig? Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` übersprungen; Nachlauf ist Orchestrator-Arbeit.

---

### Task 1: `documents.context_mode` — Migration 0030 (CHECK) · Domain ContextMode/OrAuto · pgstore (4 Reader + Create-Arity + SetContextMode) · Port · Fakes

**Files:**
- Create: `internal/adapter/pgstore/migrations/0030_documents_context_mode.sql`
- Modify: `internal/domain/document.go` (ContextMode-Typ + Konstanten + `OrAuto`/`Valid` + `Document.ContextMode`-Feld)
- Modify: `internal/adapter/pgstore/documents.go` (`docCols`, `prefixedDocCols`, **`Create`s VALUES/Args (Arity $19 + `OrAuto()`!)**, `scanDocument`, `scanSearchHit`, `scanSemanticHit`, neue `SetContextMode`-Methode)
- Modify: `internal/adapter/pgstore/documents_embed.go` (`StaleDocuments`-Inline-Scan — der 4. Reader)
- Modify: `internal/ports/ports.go` (DocumentStore-Interface)
- Modify: **jede** `ports.DocumentStore`-Fake (`internal/testutil/fakes.go` + Compiler-geführt)
- Test: `internal/adapter/pgstore/documents_test.go` (SetContextMode-Roundtrip + Owner-Scope-Negativtest + Create-Default-Roundtrip, Muster `TestDocumentStore_SetPriority`)

**Interfaces / Produces (für Tasks 2/3/4):**
- **`domain.ContextMode string`** mit `ContextModeAuto="auto"`, `ContextModeImmer="immer"`, `ContextModeNie="nie"`; `func (m ContextMode) OrAuto() ContextMode` (leer → auto); `func (m ContextMode) Valid() bool` (∈ die drei Werte). **`domain.Document.ContextMode ContextMode`** (`json:"contextMode"`) — nach `Priority`, vor `CreatedAt`; Kommentar: „per-document agent-context membership mode (auto/immer/nie; default auto). Set by SetContextMode. Create binds it via OrAuto() (empty→'auto', since the CHECK forbids ''); UpsertByPath omits it (own column list → DB default 'auto')."
- **`ports.DocumentStore.SetContextMode(ctx, ownerID, id string, mode domain.ContextMode) error`** — mirror `SetPriority`; **bumpt `updated_at` NICHT** (Modus ist Kuration, orthogonal zur Content-Aktualität — Offene Entscheidung #2); Owner-scoped; `ErrDocumentNotFound` bei 0 Rows.

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "const docCols|const prefixedDocCols" internal/adapter/pgstore/documents.go
rg -n "func scanDocument|func scanSearchHit|func scanSemanticHit|VALUES \(\\\$|func .*Create\(ctx|func .*SetPriority\(ctx" internal/adapter/pgstore/documents.go
rg -n "func .*StaleDocuments|rows.Scan\(|&d.Priority" internal/adapter/pgstore/documents_embed.go
rg -n "SetPriority|ErrDocumentNotFound|DocumentStore interface" internal/ports/ports.go
rg -rn "func.*SetPriority\(ctx" internal --glob '*_test.go' internal/testutil/fakes.go   # jede Fake
ls internal/adapter/pgstore/migrations/ | tail -3          # höchste = 0029 → neu 0030
```
- [ ] **Step 1: Failing Test** — in `documents_test.go` (testcontainer; Muster `TestDocumentStore_SetPriority`):
```go
func TestDocumentStore_SetContextMode(t *testing.T) {
	ctx, ds := newDocStore(t) // Bestand-Helper — echten Namen per rg verifizieren
	d, _ := ds.Create(ctx, domain.Document{OwnerID: "u1", Type: domain.DocMemory, Path: "m/p", Title: "T", Body: "b"})
	if d.ContextMode != domain.ContextModeAuto {
		t.Fatalf("new doc ContextMode = %q, want auto (DB default via Create OrAuto)", d.ContextMode)
	}
	if err := ds.SetContextMode(ctx, "u1", d.ID, domain.ContextModeImmer); err != nil {
		t.Fatalf("SetContextMode: %v", err)
	}
	got, _ := ds.Get(ctx, "u1", d.ID)
	if got.ContextMode != domain.ContextModeImmer {
		t.Fatalf("ContextMode = %q, want immer", got.ContextMode)
	}
	// Owner-Scope: fremder Owner darf nicht schreiben.
	if err := ds.SetContextMode(ctx, "u2", d.ID, domain.ContextModeNie); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("cross-owner SetContextMode err = %v, want ErrDocumentNotFound", err)
	}
}
```
- [ ] **Step 2: Test laufen lassen** — Expected: FAIL (Feld/Methode fehlen; ggf. Compile-Fehler in Fakes). `DOCKER_HOST` auf Podman-Socket.
- [ ] **Step 3: Migration 0030** (goose Up/Down PFLICHT, CHECK):
```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN context_mode TEXT NOT NULL DEFAULT 'auto'
    CHECK (context_mode IN ('auto','immer','nie'));

-- +goose Down
ALTER TABLE documents DROP COLUMN context_mode;
```
- [ ] **Step 4: Domain** — `ContextMode`-Typ + Konstanten + Helfer in `domain/document.go`:
```go
// ContextMode is a document's agent-context membership mode: auto (type-driven,
// the pre-L5.5 behavior), immer (always composed, uncapped, bypasses tag-gate
// and pin), or nie (never composed but fully visible in Wissen/search).
type ContextMode string

const (
	ContextModeAuto  ContextMode = "auto"
	ContextModeImmer ContextMode = "immer"
	ContextModeNie   ContextMode = "nie"
)

// OrAuto returns the mode, defaulting an empty (zero-value) mode to auto — the
// pgstore Create binding uses it so a Document built without an explicit mode
// never binds '' (which the CHECK constraint forbids).
func (m ContextMode) OrAuto() ContextMode {
	if m == "" {
		return ContextModeAuto
	}
	return m
}

// Valid reports whether m is one of the three known modes (used by the write
// use case to reject bad API input before it hits the CHECK constraint).
func (m ContextMode) Valid() bool {
	switch m {
	case ContextModeAuto, ContextModeImmer, ContextModeNie:
		return true
	default:
		return false
	}
}
```
Feld im `Document`-Struct nach `Priority` (Doku-Kommentar wie oben). `Validate()` **unangetastet** (Modus wird nicht dort validiert — Create koalesziert, REST-Handler prüft `Valid()`).
- [ ] **Step 5: pgstore-Spaltenlisten + `Create`-Arity + vier Reader + SetContextMode**
  - `docCols`: `context_mode` **am Ende** anhängen (`…, updated_by_kind, updated_by_ref, priority, context_mode`).
  - `prefixedDocCols`: `d.context_mode` am Ende.
  - **`Create` (:92-108) — ARITY + CHECK-KOALESZENZ:** `VALUES ($1..$18)` → **`$19`**; **`d.ContextMode.OrAuto()`** als 19. Arg (nach `d.Priority`). `OrAuto()` ist Pflicht — ein leeres `d.ContextMode` würde sonst `''` binden → **CHECK-Verletzung**. Das RETURNING (docCols) zieht über `scanDocument` (19 Ziele) mit.
  - Die **vier Reader** — je `var mode string` + `&mode` **direkt nach `&d.Priority`** (und vor den Extra-Spalten), dann `d.ContextMode = domain.ContextMode(mode)` (Muster wie `typ`/`d.Type`):
    - `scanDocument` (:562-570): `…&d.Priority, &mode)` → `d.ContextMode = domain.ContextMode(mode)`
    - `scanSearchHit` (:406-411): `…&d.Priority, &mode, &snippet)` → dito
    - `scanSemanticHit` (:520-525): `…&d.Priority, &mode, &content, &dist)` → dito
    - `StaleDocuments` (`documents_embed.go:41-43`): `…&d.Priority, &mode, &attempts)` → dito
  - `Update` (:174-190): nur RETURNING (docCols) → **kein** Arity-Change, Scan zieht via `scanDocument` mit.
  - `UpsertByPath` (:248): **unangetastet** (eigene INSERT-Liste, DB-Default 'auto').
  - Neue Methode:
```go
// SetContextMode sets a document's agent-context membership mode. Owner-scoped;
// deliberately does NOT bump updated_at (mode is curation, orthogonal to content
// recency — mirrors SetPriority; see domain.Document.ContextMode / Offene Entsch. #2).
func (s *DocumentStore) SetContextMode(ctx context.Context, ownerID, id string, mode domain.ContextMode) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET context_mode=$1 WHERE owner_id=$2 AND id=$3`, string(mode), ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set context mode: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}
```
- [ ] **Step 6: Port-Interface + alle Fakes** — `SetContextMode` an `ports.DocumentStore` (Doku wie oben); dann `go build ./... ./internal/...` — der Compiler listet jede Fake ohne `SetContextMode`; überall trivial ergänzen (In-Memory-Fakes: `d.ContextMode = mode`; spiegele die Fake-`SetPriority`-Struktur). **Wichtig:** die Fake-`Create`/`Get` müssen `ContextMode` mitführen und leere Modi als `auto` behandeln (`OrAuto()` beim Create-Fake), sonst schlägt der Compose-Default-Test in Task 2 fehl.
- [ ] **Step 7: Bauen + Tests + Commit** — **das VOLLE pgstore-Paket** laufen lassen: der neue `SetContextMode`/`Create`-Test übt nur `scanDocument`; die **Bestand-Tests für `Search`, `SemanticSearch` und `StaleDocuments`** üben die anderen drei Reader und fangen deren Arity-Regression (Codex-Fund #1). Falls kein Bestand-`StaleDocuments`-Test existiert (rg prüfen), einen Minimal-Roundtrip ergänzen. **Codex-Fund #4:** ein `Search`-Test, der ein `nie`-Doc anlegt und bestätigt, dass es **weiter gefunden** wird (Search filtert NICHT nach context_mode — die Fachanforderung „`nie` bleibt in Suche/Wissen sichtbar" muss test-gedeckt sein).
```bash
git add -A
go build ./... && go test ./internal/adapter/pgstore/... ./internal/usecase/... -race   # Docker-Socket; deckt Search/SemanticSearch/StaleDocuments mit ab
git commit -m "feat(pgstore): documents.context_mode (Migr 0030, CHECK) + SetContextMode — Kontext-Modus-Feld"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig (keine templ/css-Änderung).

---

### Task 2: Compose-Semantik — `nie`→`Hidden` · `immer`→`AlwaysMemories` · `ContextItem.ContextMode` · `StandingOf` · default-auto-neutral

**Files:**
- Modify: `internal/usecase/compose_context.go` (`ContextItem`, `ComposedContext`, `itemOf`, `Compose`-Switch + Used-Verbuchung, `StandingOf`)
- Modify: `cmd/flow/context.go` (der **reine `ComposedContext` → Markdown`-Renderer**, den der SessionStart-Hook injiziert — muss `AlwaysMemories` mit ausgeben, sonst erreichen `immer`-Docs den Agenten NIE; agy-Fund #1)
- Modify: **jeder weitere `ComposedContext` → Markdown/Text-Renderer** (rg-Verifikation Step 0: MCP `flow-mcp`/`internal/adapter/…` — falls vorhanden, gleich mitziehen; sonst begründet als „reused CLI renderer" notieren)
- Test: `internal/usecase/compose_context_test.go` (Bestand-Table-Tests MÜSSEN grün bleiben + neue Fälle) + `cmd/flow/context_test.go` (AlwaysMemories im Markdown gerendert; leer → keine Sektion)

**Interfaces / Produces (für Tasks 3/4):**
- **`ContextItem.ContextMode domain.ContextMode`** (`json:"contextMode,omitempty"`) — in `itemOf` aus `d.ContextMode.OrAuto()` gesetzt (nie leer, damit jede VM den aktiven Modus rendern kann).
- **`ComposedContext.AlwaysMemories []ContextItem`** (`json:"alwaysMemories,omitempty"`) — `immer`-Docs, die NICHT schon per Typ Always sind (immer memory + immer non-leaf activecontext); uncapped, zählen in `Budget.Used`.
- **`ComposedContext.Hidden []ContextItem`** (`json:"hidden,omitempty"`) — `nie`-Docs dieser Kette (node-in-chain ODER global), NUR für die Kuratieren-Wiederherstellung; nie in Used/Ranked/Memories/Always (Offene Entscheidung #4 = recommend include; cuttable).
- `StandingOf` erweitert um `AlwaysMemories` → `"always"`.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func Compose\(|type ComposedContext|type ContextItem|func itemOf|func StandingOf|out.Instructions|out.ActiveContext|out.Budget.Used|globalAllowed\[|d.Pinned" internal/usecase/compose_context.go`; `rg -n "func .*ListForContext" internal/adapter/pgstore/documents.go` (context_mode wird NICHT gefiltert → nie-Docs kommen an). **Renderer-Inventar (agy-Fund #1):** `rg -rn "\.Instructions|\.ActiveContext|\.Memories|ComposedContext" cmd/ internal/ -g '!*_test.go' -g '!internal/adapter/webui/*'` — den `cmd/flow/context.go`-Markdown-Renderer (iteriert Instructions/ActiveContext/Memories-Gruppen, den der SessionStart-Hook injiziert) UND jeden MCP-Pendant finden; ALLE müssen `AlwaysMemories` mitrendern.
- [ ] **Step 1: Failing Tests** — in `compose_context_test.go`:
```go
// (a) immer memory landet in AlwaysMemories, zählt in Used, droppt NIE (auch bei winzigem cap).
// (b) immer GLOBAL memory ist enthalten OHNE globalAllowed und OHNE Pin (Fix der 8 Globals).
// (c) nie-Doc (memory/instruction) ist in Hidden, NICHT in Used/Ranked/Memories/Always.
// (d) nie kann eine instruction demoten (instruction+nie → Hidden, nicht Instructions).
// (e) Backward-compat: alle Docs auto/leer → Memories+Ranked+Used IDENTISCH zum Bestand-Table-Test
//     (AlwaysMemories/Hidden leer). Default-Neutralität.
// (f) StandingOf: immer memory → "always"; nie-Doc → "absent" (Handler upgraded via doc.ContextMode).
// (g) ContextItem.ContextMode ist gesetzt (auto für Bestand, immer/nie wo gesetzt).
func TestCompose_ImmerAlwaysUncapped(t *testing.T)      { /* … */ }
func TestCompose_ImmerGlobalBypassesGate(t *testing.T)  { /* … */ }
func TestCompose_NieHiddenNotComposed(t *testing.T)     { /* … */ }
func TestCompose_NieDemotesInstruction(t *testing.T)    { /* … */ }
func TestCompose_AutoIsBestandNeutral(t *testing.T)     { /* … */ }
func TestStandingOf_ImmerMemoryAlways(t *testing.T)     { /* … */ }
```
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `ContextItem`/`ComposedContext`-Felder + `itemOf`**
```go
// ContextItem: + ContextMode domain.ContextMode `json:"contextMode,omitempty"`
// ComposedContext: + AlwaysMemories []ContextItem `json:"alwaysMemories,omitempty"`
//                  + Hidden         []ContextItem `json:"hidden,omitempty"`
// itemOf: it.ContextMode = d.ContextMode.OrAuto()
```
- [ ] **Step 4: `Compose`-Switch (der EINE Klassifikations-Loop, :200)** — pro Doc VOR der Typ-Routing-Logik den Modus prüfen; die Bestand-`auto`-Pfade **unverändert** lassen:
```go
for _, d := range docs {
	mode := d.ContextMode.OrAuto()
	if mode == domain.ContextModeNie {
		// Never composed. Collect for the Kuratieren restore affordance only
		// (node-in-chain OR global). Not in Used/Ranked/Memories/Always.
		lbl := "global"
		if d.NodeID != nil {
			if l, ok := label[*d.NodeID]; ok {
				lbl = l
			} else {
				continue // node not in chain — not this chain's concern
			}
		}
		out.Hidden = append(out.Hidden, itemOf(d, lbl))
		continue
	}
	if mode == domain.ContextModeImmer {
		// Forced always-tier regardless of type/tag-gate/pin. Uncapped; counted below.
		lbl := "global"
		if d.NodeID != nil {
			if l, ok := label[*d.NodeID]; ok {
				lbl = l
			} else {
				continue // node not in chain
			}
		}
		it := itemOf(d, lbl)
		switch d.Type {
		case domain.DocInstruction:
			out.Instructions = append(out.Instructions, it)
		case domain.DocActiveContext:
			if d.NodeID != nil && tier[*d.NodeID] == "leaf" {
				out.ActiveContext = &it
			} else {
				out.AlwaysMemories = append(out.AlwaysMemories, it)
			}
		default: // memory
			out.AlwaysMemories = append(out.AlwaysMemories, it)
		}
		continue
	}
	// mode == auto: EXACT Bestand logic (unverändert übernehmen — der bestehende switch d.Type { … }).
	switch d.Type {
	case domain.DocInstruction: /* … Bestand … */
	case domain.DocActiveContext: /* … Bestand … */
	case domain.DocMemory: /* … Bestand (global: globalAllowed||Pinned; node-in-chain: pool) … */
	}
}
```
  Used-Verbuchung (:231) ergänzen: nach Instructions + ActiveContext auch **`AlwaysMemories` in `out.Budget.Used`** einrechnen (uncapped Always-Tier):
```go
for _, it := range out.AlwaysMemories {
	out.Budget.Used += it.EstTokens
}
```
  **Der Sort + Füll-Loop (:240-275) bleibt UNVERÄNDERT** (nur `auto`-Memory-Docs sind im Pool; `immer`/`nie` sind nie im Pool). `Ranked` enthält weiterhin nur Pool-Items.
- [ ] **Step 5: `StandingOf`** (:412) — nach der ActiveContext-Prüfung eine Schleife über `AlwaysMemories`:
```go
for _, it := range cc.AlwaysMemories {
	if it.ID == docID {
		return ContextStanding{State: "always", ScopeLabel: it.ScopeLabel}
	}
}
```
  (`nie`-Docs sind in keiner cc-Liste außer `Hidden` → `StandingOf` liefert `"absent"`; die Doc-Seite upgraded das anhand `doc.ContextMode==nie` selbst — Task 4. `StandingOf` bleibt bewusst unabhängig von `Hidden`, damit OE #4 es cutten kann.)
- [ ] **Step 5b: CLI/MCP-Markdown-Renderer — `AlwaysMemories` ausgeben (agy-Fund #1, KRITISCH)** — der reine `ComposedContext` → Markdown-Renderer in `cmd/flow/context.go` (den der SessionStart-Hook injiziert) iteriert `Instructions`/`ActiveContext`/`Memories`, aber NICHT `AlwaysMemories` → ohne diesen Schritt landen `immer`-Docs im Server-Compose (Used/JSON), aber **nie im tatsächlichen Agenten-Prompt** — das Kernversprechen von `immer` wäre still gebrochen. `AlwaysMemories` in der Always-Sektion des Markdown mit ausgeben (neben Instructions/ActiveContext, uncapped, Sektionsstil des Bestands treffen). Erst **failing test** in `cmd/flow/context_test.go` (eine `ComposedContext{AlwaysMemories: […]}` erscheint im Markdown; leere Liste → keine Sektion), dann implementieren. Jeden weiteren gefundenen Renderer (Step 0 Inventar, MCP) gleich mitziehen.
- [ ] **Step 6: Tests + Commit**
```bash
git add -A && go test ./internal/usecase/... ./cmd/flow/... -race 2>&1 | tail -20
git commit -m "feat(usecase): Kontext-Modus-Semantik — immer→AlwaysMemories (uncapped) · nie→Hidden · ContextItem.ContextMode · StandingOf + CLI-Markdown-Renderer; auto verhaltensneutral"
```
Expected: PASS; **Bestand-Compose-Table-Tests grün** (auto-Pfad unverändert); der CLI-Markdown-Renderer gibt `AlwaysMemories` aus.

---

### Task 3: `SetContextMode`-Usecase + REST `POST /api/v1/documents/{id}/context-mode` + apiclient + Wiring

**Files:**
- Create: `internal/usecase/set_context_mode.go` + `internal/usecase/set_context_mode_test.go`
- Modify: `internal/adapter/httpserver/documents.go` (`handleSetContextMode`)
- Modify: `internal/adapter/httpserver/server.go` (Server-Feld `SetContextMode` + Route)
- Modify: `cmd/flow-server/main.go` (Wiring — T6 verifiziert die Composition-Root)
- Modify: `internal/adapter/apiclient/context.go` (+ ggf. `_test.go`) — `SetContextMode`-Client-Methode (Muster `SetPinned` :76)
- Test: `internal/adapter/httpserver/documents_test.go` (REST-Roundtrip + Owner-Scope + invalider Modus 400 + Emit-Capture)

**Interfaces / Produces:**
- **`usecase.SetContextMode{ Docs ports.DocumentStore }`** mit `Execute(ctx, ownerID, id string, mode domain.ContextMode) error` — validiert `mode.Valid()` (sonst `domain.ErrInvalidDocument` — Bestand-Sentinel aus `internal/domain/errors.go`, Codex-Fund #2), dann `Docs.SetContextMode`. Owner-scoped über die Store-Primitive.
- **REST `POST /api/v1/documents/{id}/context-mode`** Body `{"mode":"immer"}` → 204; invalider Modus → 400; fremd/unbekannt → 404; emittiert **einen** `document.updated` (`Data{"id":id}`). Muster: `handlePinDocument` (:295).

- [ ] **Step 0: rg-Verifikation** — `rg -n "type SetPinned|func .*SetPinned.*Execute" internal/usecase/set_pinned.go`; `rg -n "ErrInvalidDocument" internal/domain/errors.go internal/ports/ports.go` (**Codex-Fund #2: `ErrInvalidDocument` lebt in `domain` (errors.go), NICHT in `ports` — `domain.ErrInvalidDocument` verwenden**); `rg -n "func .*handlePinDocument|type pinReq|s.Emitter.Emit|EventDocumentUpdated" internal/adapter/httpserver/documents.go`; `rg -n "SetPinned\s+usecase|SetContextMode|mux.Handle\(\"POST /api/v1/documents/\{id\}/pin" internal/adapter/httpserver/server.go`; `rg -n "SetPinned:|ReorderContextDocs:" cmd/flow-server/main.go`; `rg -n "func \(c \*Client\) SetPinned|func \(c \*Client\) ReorderContext" internal/adapter/apiclient/context.go`.
- [ ] **Step 1: Failing Usecase-Test** — `set_context_mode_test.go` mit Fake-DocumentStore: `Execute(owner, id, immer)` ruft `SetContextMode(owner,id,immer)`; invalider Modus (`"bogus"`) → Fehler **ohne** Store-Aufruf; owner-fremd → propagierter `ErrDocumentNotFound`.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Usecase**
```go
package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type SetContextMode struct{ Docs ports.DocumentStore }

// Execute validates the mode and sets it on the document (owner-scoped). An
// unknown mode is rejected before any store write (belt-and-suspenders with the
// DB CHECK constraint).
func (uc SetContextMode) Execute(ctx context.Context, ownerID, id string, mode domain.ContextMode) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: bad context mode %q", domain.ErrInvalidDocument, mode)
	}
	return uc.Docs.SetContextMode(ctx, ownerID, id, mode)
}
```
  (`domain.ErrInvalidDocument` ist der Bestand-Sentinel aus `internal/domain/errors.go` — Codex-Fund #2. `ports` bleibt importiert für `ports.DocumentStore` im Struct-Feld.)
- [ ] **Step 4: REST-Handler** (`documents.go`), Muster `handlePinDocument`:
```go
type contextModeReq struct {
	Mode string `json:"mode"`
}

func (s *Server) handleSetContextMode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req contextModeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	switch err := s.SetContextMode.Execute(r.Context(), u.ID, id, domain.ContextMode(req.Mode)); {
	case errors.Is(err, domain.ErrInvalidDocument):
		http.Error(w, "bad mode", http.StatusBadRequest)
	case errors.Is(err, ports.ErrDocumentNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
		w.WriteHeader(http.StatusNoContent)
	}
}
```
- [ ] **Step 5: Server-Feld + Route + main.go** — `SetContextMode usecase.SetContextMode` in die Server-Struct (neben `SetPinned`); `mux.Handle("POST /api/v1/documents/{id}/context-mode", s.auth(http.HandlerFunc(s.handleSetContextMode)))` (neben der Pin-Route :192); in `main.go` `SetContextMode: usecase.SetContextMode{Docs: documentStore}` ins Server-Literal (neben `SetPinned:`).
- [ ] **Step 6: Handler-Test** (`documents_test.go`, httptest): 204 + Fake-Store-Assertion (Modus gesetzt) + genau ein `document.updated` (Emitter-Capture); invalider Modus → 400; User B kann A's Doc nicht ändern (404).
- [ ] **Step 7: apiclient `SetContextMode`** — Methode nach Muster `SetPinned` (`apiclient/context.go`; `POST /api/v1/documents/{id}/context-mode` mit `{"mode":…}`); + Client-Test gegen Stub-Server. **CLI-Verb + MCP-Tool bleiben Deferred** (Self-Review; die Fähigkeit ist über apiclient erreichbar — generisches Feature für alle Hosts, Memory).
- [ ] **Step 8: Tests + Commit**
```bash
git add -A && go test ./internal/usecase/... ./internal/adapter/httpserver/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(context): SetContextMode + POST /api/v1/documents/{id}/context-mode + apiclient (owner-scoped, emits document.updated)"
```
Expected: PASS.

---

### Task 4: UI-Modus-Umschalter — Kuratieren (Rang/Always/Hidden) + Doc-Seite + Cockpit-AlwaysN + Web-Handler + i18n + SSE

**Files:**
- Modify: `internal/adapter/webui/kontext_vm.go` (`KontextRowVM.Mode`, `KontextAlwaysVM.Mode`, neue `KontextHiddenVM`, `KontextVM.Hidden`, `BuildKontextVM` iteriert `AlwaysMemories`+`Hidden`)
- Modify: `internal/adapter/webui/cockpit_context_vm.go` (`AlwaysN += len(cc.AlwaysMemories)`)
- Modify: `internal/adapter/webui/doc_context_vm.go` (`DocContextVM.Mode`, `BuildDocContext` nimmt Modus, rendert immer für Kontext-Typen)
- Modify: `internal/adapter/webui/document_vm.go` (falls `DocumentVM.Context`-Bau die Signatur berührt)
- Modify: `internal/adapter/webui/kontext.templ` (Modus-Segment in `kontextRow`/`kontextAlwaysRow`, neuer `kontextHiddenRow` + Hidden-Sektion)
- Modify: `internal/adapter/webui/document.templ` (Modus-Segment im Kontext-`.blk`, Block auch für `nie`/`absent` rendern)
- Modify: `internal/adapter/httpserver/webui_kontext.go` (`handleWebKontextMode` + Route-Nutzung)
- Modify: `internal/adapter/httpserver/webui_document.go` (`buildDocumentVM`: `BuildDocContext` mit Modus für alle Kontext-Typen; `handleWebDocMode`)
- Modify: `internal/adapter/httpserver/server.go` (zwei Web-Routen)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/kontext_vm_test.go`, `internal/adapter/webui/doc_context_vm_test.go`, `internal/adapter/httpserver/webui_kontext_test.go`, `internal/adapter/httpserver/webui_document_test.go`

**Widget (ENTSCHIEDEN, Offene Entscheidung #5):** ein `.seg .seg-sm`-Dreisegment (Bestand, tailwind.css:335-337/486; Muster `cockpit_main.templ:100` Wissen-Scope). Drei Segmente Auto/Immer/Nie; das aktive trägt `aria-pressed="true"`; jedes `hx-post`et den Modus, `hx-target` = das jeweilige Fragment (`#kontext-fragment` bzw. `#document-fragment`), `hx-swap="outerHTML"`, `hx-vals={ {"doc":id,"mode":"immer"} }`. Ein reines A11y-`aria-label`/`title` je Segment (i18n). Keine Emojis, kein neues Farbsystem.

**Containment `.right`-Aktionsleiste (KONKRET, Codex-Fund #6, Spec §11):** die Rang-Zeile trägt jetzt Pin + ↑ + ↓ + Edit **und** das Dreisegment — auf 375px pannt das sonst horizontal. Konkrete Maßnahme (kein bloßer Zustands-Vermerk): `.right` bekommt `flex-wrap: wrap` (die `.seg` rutscht bei Enge in eine zweite Zeile statt zu überlaufen) ODER das Segment steht in einer eigenen Zeile unter den Icon-Buttons. Eine **benannte** Regel in `web/tailwind.css` (z. B. `.kontext-actions` oder Erweiterung der Bestand-`.right`-Regel), kein Arbitrary. Der Render-Test/das Gate assertet: bei schmaler Breite kein horizontaler Overflow der Zeile.

**Interfaces:**
- **`KontextRowVM.Mode domain.ContextMode`** + **`KontextAlwaysVM.Mode`** — aus `r.Item.ContextMode`. Rang-Zeilen sind konstruktiv immer `auto` (immer→Always, nie→Hidden), aber das Segment zeigt den Ist-Modus und erlaubt Promotion/Demotion.
- **`KontextHiddenVM{ DocID, Title, ChipClass, TypeLabel, ScopeLabel, TokensStr string; Mode domain.ContextMode }`** + **`KontextVM.Hidden []KontextHiddenVM`** — aus `cc.Hidden`. Eigene ruhige Sektion („Ausgeblendet (nie)"), pro Zeile nur der Modus-Umschalter + Editor-Link (kein Pin/↑/↓). Wiederherstellung in-place.
- **`BuildKontextVM`** (:58): zusätzlich `for _, it := range cc.AlwaysMemories { vm.Always = append(vm.Always, kontextAlwaysOf(it)) }` (immer-Memories in den Always-Abschnitt) und `for _, it := range cc.Hidden { vm.Hidden = append(vm.Hidden, kontextHiddenOf(it)) }`. `kontextAlwaysOf`/`kontextHiddenOf` setzen `Mode`.
- **`BuildCockpitContext`** (:64): `vm.AlwaysN = len(cc.Instructions) + boolToInt(cc.ActiveContext != nil) + len(cc.AlwaysMemories)` (immer-Memories zählen als „immer enthalten").
- **`DocContextVM.Mode domain.ContextMode`** + **`BuildDocContext(st usecase.ContextStanding, nodeName string, mode domain.ContextMode) *DocContextVM`** — gibt für Kontext-Typen **immer** einen Block zurück (nie `nil`), damit der Umschalter erreichbar ist; `State` bleibt `included/dropped/always/absent`, PLUS neuer Anzeige-Zweig `nie` wenn `mode==nie` (der Handler setzt `State="hidden"` bzw. der Templ-Zweig liest `vm.Mode==nie`).
- **`buildDocumentVM`** (Kontext-Teil): für Kontext-Typen (`isContextType`) IMMER `vm.Context` bauen (auch bei `absent`/`nie`), Modus = `doc.ContextMode.OrAuto()`; für Nicht-Kontext-Typen kein Block (wie Bestand).
- **`handleWebKontextMode`** (`webui_kontext.go`): `_ = r.ParseForm()`; `doc := r.FormValue("doc")`, `mode := r.FormValue("mode")`; `err := s.SetContextMode.Execute(ctx, u.ID, doc, domain.ContextMode(mode))` — `ErrInvalidDocument`/`ErrDocumentNotFound` → sauberer No-op-Re-Render (kein 500), sonst `Emit(document.updated)`; `renderKontext` (Muster `handleWebKontextPin` :138).
- **`handleWebDocMode`** (`webui_document.go`): analog `handleWebDocPin` — `SetContextMode` + `Emit` + rendert das `#document-fragment` (buildDocumentVM/DocumentFragment).
- **Routen** (server.go, `webAuth`, bei den Bestand-Routen): `POST /kontext/{id}/mode` → `handleWebKontextMode`; `POST /wissen/{id}/mode` → `handleWebDocMode` (Pin-Route-Namen per rg verifizieren: `/wissen/{id}/pin`).
- i18n (beide Kataloge):
```go
"context.mode.label": "Kontext-Modus",   // en: "Context mode"
"context.mode.auto":  "Auto",            // en: "Auto"
"context.mode.immer": "Immer",           // en: "Always"
"context.mode.nie":   "Nie",             // en: "Never"
"context.mode.autoHint":  "Automatisch nach Typ/Regeln",      // en: "Automatic by type/rules"
"context.mode.immerHint": "Immer im Agenten-Kontext",          // en: "Always in agent context"
"context.mode.nieHint":   "Nie im Agenten-Kontext (in Wissen sichtbar)", // en: "Never in agent context (still in Wissen)"
"context.curate.hidden":  "Ausgeblendet (nie)",                // en: "Hidden (never)"
"document.context.hidden": "ausgeblendet (nie)",               // en: "hidden (never)"
```

**Zustände dieser Fläche:** leer (Knoten ohne Docs → keine Rows/Always/Hidden, Umschalter nur auf Doc-Seite); `immer` (Doc im Always-Abschnitt, Cockpit-AlwaysN+1, droppt nie); `nie` (Doc raus aus Rang-Liste, taucht in Hidden-Abschnitt auf, Doc-Seite zeigt „ausgeblendet (nie)" + Umschalter → wiederherstellbar); lang (Doc-Titel bricht via `.row .t`/`title`; Umschalter fixe Breite); **mobil 375px** (`.right`-Aktionsleiste + `.seg`-Umschalter dürfen NICHT horizontal pannen — `.seg` schrumpft/`flex-wrap` in `.right`; **Containment-Check Pflicht**, Spec §11); laufender Timer (n. a.); Fehlerpfad (invalider/gelöschter Doc → No-op-Re-Render, kein 500).

- [ ] **Step 0: rg-Verifikation** — `rg -n "type KontextRowVM|type KontextAlwaysVM|type KontextVM|func BuildKontextVM|func kontextAlwaysOf" internal/adapter/webui/kontext_vm.go`; `rg -n "templ kontextRow|templ kontextAlwaysRow|templ KontextFragment|pinLabelKey" internal/adapter/webui/kontext.templ`; `rg -n "func BuildDocContext|type DocContextVM|func .*isContextType|func .*buildDocumentVM|vm.Context =" internal/adapter/webui/doc_context_vm.go internal/adapter/httpserver/webui_document.go`; `rg -n "func .*handleWebDocPin|/wissen/\{id\}/pin|DocumentFragment" internal/adapter/httpserver/webui_document.go internal/adapter/webui/document.templ internal/adapter/httpserver/server.go`; `rg -n "\.seg\b|seg-sm|aria-pressed" internal/adapter/webui/cockpit_main.templ web/tailwind.css`; `rg -n "AlwaysN =|len\(cc.Instructions\)" internal/adapter/webui/cockpit_context_vm.go`.
- [ ] **Step 1: Failing Builder-Tests** — `kontext_vm_test.go` (BuildKontextVM: Always enthält immer-Memories, Hidden enthält nie-Docs, Row.Mode gesetzt), `doc_context_vm_test.go` (BuildDocContext liefert Block auch bei `nie`/`absent`, `Mode` durchgereicht).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: VMs** — `KontextRowVM.Mode`/`KontextAlwaysVM.Mode`/`KontextHiddenVM`/`KontextVM.Hidden`; `BuildKontextVM` iteriert AlwaysMemories+Hidden; `BuildCockpitContext.AlwaysN += len(AlwaysMemories)`; `DocContextVM.Mode` + `BuildDocContext(st, nodeName, mode)`.
- [ ] **Step 4: templ + Handler + Routen + i18n** — Modus-`.seg`-Segment als geteiltes templ-Snippet (z. B. `modeSeg(postURL, target, docID string, cur domain.ContextMode)`), in `kontextRow`/`kontextAlwaysRow`/`kontextHiddenRow` (post `/kontext/{node}/mode`, target `#kontext-fragment`) und im Doc-`.blk` (post `/wissen/{id}/mode`, target `#document-fragment`); Hidden-Sektion in `KontextFragment` (nach der Rang-Liste, nur wenn `len(vm.Hidden)>0`); Doc-`.blk` rendert jetzt auch bei `nie`/`absent` (Umschalter immer); Web-Handler + Routen + i18n beide Kataloge.
- [ ] **Step 5: Handler-/Render-Tests** — `webui_kontext_test.go`: Modus-Segment mit `aria-pressed`/`aria-label` in Rang-/Always-/Hidden-Zeilen; `POST /kontext/{id}/mode doc=X mode=immer` → Doc wandert in Always (Re-Render), emittiert `document.updated`, nur Fragment zurück; `mode=nie` → Doc in Hidden; **Owner-Scope** (Fremd-Doc → No-op, kein Effekt); invalider Modus → No-op-Re-Render (kein 500). `webui_document_test.go`: ein Memory-Doc zeigt den Umschalter mit aktivem Ist-Modus; `nie`-Doc zeigt „ausgeblendet (nie)" + Umschalter; Nicht-Kontext-Doc (project) zeigt **keinen** Block; Anpinnen-Button (Bestand) unverändert. **Codex-Fund #3 — der Doc-Seiten-Rückweg braucht seinen EIGENEN Mutationstest:** `POST /wissen/{id}/mode doc=X mode=immer` → togglet den Modus (Store-Assertion), emittiert genau ein `document.updated`, gibt **nur** das `#document-fragment` zurück (keine AppShell), Owner-Scope (Fremd-Doc → No-op), invalider Modus → sauberer Re-Render (kein 500).
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Kontext-Modus-Umschalter (auto/immer/nie) — Kuratieren Rang/Immer/Ausgeblendet + Doc-Seite + Cockpit-AlwaysN (owner-scoped, SSE document.updated)"
```
Expected: PASS; `app.css` ggf. geändert (neue Klassennutzung).

---

### Task 5: Cockpit-Kurzbeschreibung — toter `DescriptionHTML`-Pfad → Plaintext-Identitätszeile

**Files:**
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData`: den `RenderDocument`-Block :41-43 entfernen/umwidmen)
- Modify: `internal/adapter/webui/cockpit_vm.go` (`NodeCockpit.DescriptionHTML template.HTML` → **entfernen**; die Anzeige liest `d.N.Description` direkt)
- Modify: `internal/adapter/webui/cockpit_head.templ` (Plaintext-Zeile in `.spine-main`, nach dem `<h1>`)
- Modify: `web/tailwind.css` (`.spine-desc`-Klasse — Plaintext, `--meta`-Farbe, Containment)
- Optional: `internal/adapter/webui/node_tree*.templ`/`_vm.go` (Untertitel im Projekte-Baum — Offene Entscheidung #7)
- Test: `internal/adapter/webui/cockpit_head_render_test.go` (bzw. Bestand-Cockpit-Render-Test): Description-Zeile erscheint; leere Description → keine Zeile

**Design (Offene Entscheidung #6 = ENTSCHIEDEN „Head-Spine-Identität, Plaintext"):** Die Kurzbeschreibung ist **Plaintext** (Kurz-Einzeiler brauchen kein Markdown → `RenderDocument` entfällt, der tote Pfad wird ersatzlos entfernt). Sie steht als ruhige Untertitel-Zeile in der Cockpit-Identität (`.spine-main`, direkt unter dem `<h1>`), `.spine-desc`-Klasse (`color:rgb(var(--meta))`, `font-size` klein), **Containment**: einzeilig mit `truncate`/`min-w-0` (Spec §11) — eine versehentlich lange/mehrzeilige Bestand-Description bricht die Identität nicht. Kein i18n-Label nötig (reiner Inhalt). Kein SSE-Neuwerk: der Head reloadet bereits auf `node.updated` (Bestand, `webui_nodes.go:405/433` beim Node-Edit).

- [ ] **Step 0: rg-Verifikation** — `rg -n "DescriptionHTML|n.Description|RenderDocument|d.N.Description" internal/adapter/webui internal/adapter/httpserver -g '!*_templ.go'`; `rg -n "template CockpitHead|spine-main|\.spine-desc|<h1>" internal/adapter/webui/cockpit_head.templ web/tailwind.css`; `rg -n "Description" internal/domain/node.go` (Feldname bestätigen: `domain.Node.Description string`). **Codex-Fund #7 — Layout prüfen:** `rg -n "\.spine-main" web/tailwind.css` — ist `.spine-main` eine **flex-row** (Avatar neben Titel)? Dann wird ein `<p>` NACH dem `<h1>` zum dritten Flex-**Geschwister** (neben dem h1), NICHT darunter. Der Titel-Block (h1 + Beschreibung) muss in eine **eigene Textspalte** (`flex-direction:column`, `min-w-0`), sonst rendert die Beschreibung falsch.
- [ ] **Step 1: Failing Render-Test** — Cockpit-Head eines Knotens mit `Description="Kurz-Einzeiler"` rendert den Text (in der Titelspalte unter dem `<h1>`); leere Description → die Zeile fehlt (kein leeres `<div>`); eine sehr lange Description bricht nicht (Ellipsis/`.spine-desc`) — Zustand lang + 375px im Gate.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: toten Pfad entfernen** — in `nodeCockpitData` den `if n.Description != "" { d.DescriptionHTML, _ = webui.RenderDocument(...) }`-Block **löschen**; das `DescriptionHTML template.HTML`-Feld aus `NodeCockpit` entfernen (verwaist). Prüfen, dass `webui.RenderDocument` **anderswo** noch genutzt wird (Doc-Seite/Editor — ja, Bestand) → Import bleibt gültig; nur der Cockpit-Aufruf fällt.
- [ ] **Step 4: templ + CSS** — in `cockpit_head.templ` die Beschreibung als eigene Zeile UNTER dem `<h1>`. **Weil `.spine-main` eine Flex-Row ist (Avatar + Titel, Codex-Fund #7), muss der Titel-Teil in eine Textspalte** — das `<h1>` + die Beschreibung in ein `div.spine-title` (Spalte) wickeln, damit die Beschreibung darunter (nicht daneben) sitzt:
```html
<div class="spine-title">
	<h1>{ ShortName(d.N.Name) }</h1>
	if d.N.Description != "" {
		<p class="spine-desc" title={ d.N.Description }>{ d.N.Description }</p>
	}
</div>
```
  (Falls das Bestand-`<h1>` nicht umschließbar ist, alternativ die Beschreibung in `.spine-meta` platzieren — der Render-Test entscheidet über die Zeilenlage.) `.spine-title` + `.spine-desc` in `web/tailwind.css` (benannte Klassen, Containment):
```css
.spine-title { display: flex; flex-direction: column; min-width: 0; }
.spine-desc { color: rgb(var(--meta)); font-size: .9rem; margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
```
- [ ] **Step 5: (optional) Projekte-Baum-Untertitel** — falls Offene Entscheidung #7 „ja": in der Baum-Zeile die Kurzbeschreibung als gedämpften Untertitel unter dem Node-Namen (Bestand-`node_tree`-VM/templ, Plaintext, `truncate`). Sonst Deferred-Notiz.
- [ ] **Step 6: Bauen + Test + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Cockpit-Kurzbeschreibung als Plaintext-Identitätszeile (toter DescriptionHTML/RenderDocument-Pfad entfernt)"
```
Expected: PASS; `app.css` geändert.

---

### Task 6: Wiring-Gate (Composition-Root · Sweep · make ci · Live-Smoke · Breakpoints · Daten-Nachlauf-Doku)

**Files:** i. d. R. keine neuen (Verifikation + evtl. Sweep-Fixes + Ledger).

- [ ] **Step 1: Composition-Root-Verifikation** (`cmd/flow-server/main.go` + `server.go`)
```bash
rg -n "SetContextMode|ComposeContext|SetPinned|ReorderContextDocs|ContextBudget|GetDocument|GetNode" cmd/flow-server/main.go internal/adapter/httpserver/server.go
rg -n "mux.Handle\(\"POST /api/v1/documents/\{id\}/context-mode|mux.Handle\(\"POST /kontext/\{id\}/mode|mux.Handle\(\"POST /wissen/\{id\}/mode" internal/adapter/httpserver/server.go
```
Erwartet: `SetContextMode` im Server-Literal verdrahtet; die drei Modus-Routen (REST + zwei Web) registriert. **Kein weiterer main.go-Change** — T4/T5 nutzen ausschließlich Bestands-Server-Felder (`ComposeContext`, `SetPinned`, `GetNode`, `GetDocument`, `ContextBudget`, `NodeAncestors`, `Stats`).
- [ ] **Step 2: Rest-Sweep** — Dispatch-Text oben über `git diff --name-only rebuild..HEAD`. Gefundene tote Keys/Arbitrary-Values/verwaiste Symbole/DescriptionHTML-Reste fixen.
- [ ] **Step 3: Tote i18n-Keys** — die neuen `context.mode.*`/`context.curate.hidden`/`document.context.hidden`-Keys gegen `T(`/`Tn(`-Nutzung prüfen; keine verwaisten; de+en-Parität.
- [ ] **Step 4: Volles CI**
```bash
git add -A
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST=Podman-Socket
```
(erst stagen, dann ci — L4-Lehre; nie zwei ci parallel.)
- [ ] **Step 5: Live-Smoke** (Dev-Stack; Cookie-Flow wie L1–L5-Gate)
```bash
make dev-run &   # https://localhost:8080 (self-signed); danach stoppen
sleep 2
# Migration 0030 angewandt? (context_mode-Spalte + CHECK)
# POST /api/v1/documents/{id}/context-mode {"mode":"immer"} (Bearer) → 204; GET /context zeigt Doc in alwaysMemories
# POST .../context-mode {"mode":"nie"} → 204; GET /context: Doc weg aus memories/ranked/alwaysMemories, in hidden; GET /api/v1/documents/{id} zeigt es weiter
# nie-Doc SICHTBAR IN SUCHE (Codex-Fund #4): GET Wissen-Suche nach dem nie-Doc-Titel → Treffer vorhanden
# GET /context (nach immer): der CLI-Markdown (flow context) enthält das immer-Doc im Prompt (agy-Fund #1 — nicht nur JSON)
# GET /kontext/{id}: Umschalter in Rang/Immer/Ausgeblendet; immer-Doc droppt nie; nie-Doc in Ausgeblendet, wiederherstellbar
# GET /wissen/{memory-id}: Umschalter mit Ist-Modus; nie → "ausgeblendet (nie)"
# GET /nodes/{id}: Cockpit zeigt Kurzbeschreibung (Node mit Description)
```
Expected: Modus-Umschalten togglet + Cockpit-Meter/AlwaysN/Dokument-Rang ziehen per SSE (`document.updated`) nach; `immer`-Doc garantiert enthalten (auch bei vollem Budget); `nie`-Doc aus Compose, in Wissen sichtbar, per Doc-Seite UND Ausgeblendet-Abschnitt wiederherstellbar; Cockpit-Head zeigt die Kurzbeschreibung. Danach Server stoppen.
- [ ] **Step 6: Breakpoint-Sichtprobe für Soenne notieren** — **≤960px** (Kuratieren `.narrow` volle Breite; Cockpit-Rail stackt) und **375px** (Modus-`.seg`-Umschalter + `.right`-Aktionen pannen NICHT horizontal; `.spine-desc` Ellipsis greift; Umschalter tappbar).
- [ ] **Step 7: Daten-Nachlauf dokumentieren (kein goose) — PRÄZISE, nicht pauschal (Codex-Fund #5)** — die 8 globalen Feedback-Memories auf `immer` setzen, **einmalig, owner-scoped, dev+PROD** (Ledger + Deferred-Notiz, NICHT in eine Migration). **NICHT blind alle globalen Memories updaten** — Soenne kann weitere global-memory-Docs haben. Runbook = erst **selektieren + zählen**, Soenne bestätigt die genaue Menge, dann **per expliziter ID-Liste** updaten, danach gegenzählen:
```sql
-- 1) Kandidaten sichten (Titel/Pfad prüfen — sind es genau die 8 Feedback-Docs?):
SELECT id, title, path, pinned, context_mode
  FROM documents
 WHERE owner_id = '<SOENNE_OWNER_ID>' AND type='memory' AND node_id IS NULL
 ORDER BY updated_at DESC;
-- 2) Vorher-Count:
SELECT count(*) FROM documents
 WHERE owner_id='<SOENNE_OWNER_ID>' AND context_mode='immer';
-- 3) Gezieltes UPDATE per bestätigter ID-Liste (KEIN pauschales WHERE node_id IS NULL):
UPDATE documents SET context_mode='immer'
 WHERE owner_id='<SOENNE_OWNER_ID>' AND id IN ('<id1>', …, '<id8>');
-- 4) Nachher-Count (= Vorher + Anzahl der bestätigten Docs):
SELECT count(*) FROM documents
 WHERE owner_id='<SOENNE_OWNER_ID>' AND context_mode='immer';
```
  Danach ist ihr Pin-Zustand egal (Fix der Einbahnstraße). **Owner-ID/IDs nie hartkodiert committen** — im Ledger als Runbook-Schritt, im Live-Gate von Soenne (dev zuerst, dann PROD) ausgeführt.
- [ ] **Step 8: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L5.5-Gate — Composition-Root-Verify + Sweep + Live-Smoke (Kontext-Modus + Kurzbeschreibung)"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Alternativpfade. Entscheidung am Ausführungsstart.

1. **Canonical-Werte des `context_mode`: `auto`/`immer`/`nie` (deutsch) oder `auto`/`always`/`never` (englisch)?** — *Empfehlung: `auto`/`immer`/`nie`* — genau die Werte aus Soennes Spec, und die Codebase hat mit `NodeKind = "vorhaben"` bereits einen deutschen Domain-Wert (kein reines Englisch-Dogma). Der Feldname `context_mode` bleibt englisch, die Werte deutsch — kein Übersetzungs-Layer zwischen DB und i18n-Label (Label „Auto/Immer/Nie" fällt direkt). **Alternative:** englische Werte (`always`/`never`) konsistent mit `DocMemory="memory"` — sauberer, aber ein Mapping DB↔Anzeige, und weicht von Soennes Spec ab. Abgelehnt.
2. **Bumpt `SetContextMode` `updated_at`?** — *Empfehlung: NEIN* (wie `SetPriority`). Ein Modus-Wechsel ist Kuration, kein Content-Edit; ein Bump würde das Doc fälschlich als „gerade bearbeitet" in Wissen-Listen/Recency-Tiebreaker hochspülen und die Provenance-„aktualisiert"-Zeile verfälschen. Trade-off: Abweichung vom Bestand-`SetPinned` (das bumpt). **Alternative:** bumpen (konsistent mit Pin), dafür Recency-Rauschen. Abgelehnt.
3. **SSE-Event für Modus-Mutationen.** — *Empfehlung: `document.updated` wiederverwenden* (alle Konsumenten führen es seit L5). Minimal, kein neuer Typ. Trade-off: die Rail lädt auch bei fremden Doc-Änderungen neu — harmlos (Bestand). **Alternative:** `context.changed` — sauberere Semantik, aber neuer Typ + alle Handler/Konsumenten. Abgelehnt.
4. **`nie`-Wiederherstellung: Kuratieren-„Ausgeblendet"-Abschnitt (via `cc.Hidden`) oder nur Doc-Seite?** — *Empfehlung: BEIDE* — die Doc-Seite ist der universelle, immer erreichbare Weg (jedes Doc ist in Wissen sichtbar → Umschalter dort), UND ein ruhiger „Ausgeblendet (nie)"-Abschnitt auf der Kuratieren-Seite (aus `cc.Hidden`, **0 Extra-Queries** — die nie-Docs sind bereits von `ListForContext` geladen) macht die Wiederherstellung auch in-place möglich. Das ist die eigentliche Trap-Vermeidung: anders als beim Entpinnen (das die einzige Zeile mitnahm) hat `nie` **zwei** sichtbare Rückwege. Trade-off: ein zusätzlicher UI-Abschnitt + `Hidden`-Feld. **Alternative:** nur Doc-Seite (leaner, `cc.Hidden` entfällt) — akzeptabel, weil die Doc-Seite universell ist, aber der Kuratieren-in-place-Komfort fehlt und es näher an der alten Falle liegt. Empfehlung: BEIDE bauen; wenn Scope gekürzt werden muss, ist der Hidden-Abschnitt (nicht die Doc-Seite) der cuttbare Teil.
5. **Widget des Modus-Umschalters.** — *Empfehlung: `.seg .seg-sm`-Dreisegment* (Bestand; Präzedenz Wissen-Scope-Toggle) mit `aria-pressed="true"` am aktiven Segment — leise, konsistent, kein neues Farbsystem, kein JS. **Alternativen:** (a) drei `.btn.btn-q.btn-s`-Toggles (mehr Chrome, kein aktiv-Zustand von Haus aus); (b) `<select>` (braucht Form/JS, gegen Lesesaal-Ruhe); (c) Icon-only (A11y-Verstoß). Abgelehnt.
6. **Platzierung der Kurzbeschreibung: Head-Spine-Identität oder eigener Rail-Block?** — *Empfehlung: Head-Spine* (`.spine-main`, unter dem `<h1>`) als Plaintext-Untertitel — dort sitzt die Identität (Avatar + Name + Kind/Status), die Beschreibung gehört als Untertitel dazu; der Head reloadet bereits auf `node.updated`. Soennes Wortlaut „(Rail)" meint konzeptuell die Cockpit-Meta/Identität; die schärfste Umsetzung ist der Identitäts-Untertitel. **Alternative:** ein eigener „Über"-`.blk` in der Rail (`CockpitRailBlocks`) — passt Soennes Klammer wörtlich, ist aber ein zweiter Ort für Identitäts-Info und weniger ruhig. Empfehlung: Head-Spine; falls Soenne den Rail-Block will, ist der Umbau trivial.
7. **Kurzbeschreibung zusätzlich als Untertitel im Projekte-Baum?** — *Empfehlung: JA* (generisches Feature in alle Hosts, Memory) — ein gedämpfter, einzeiliger Untertitel unter dem Node-Namen im Baum hilft der Orientierung. Trade-off: minimal mehr Baum-Chrome. **Alternative:** nur Cockpit (Baum bleibt schlank) — Deferred-Notiz. Empfehlung: JA, aber als Task-5-Optionalschritt klar abtrennbar, falls der Baum zu dicht wirkt.
8. **`immer` ist stärker als Pin (uncapped, droppt nie).** — *Empfehlung/Bestätigung: JA* (Vorgabe-Block) — `immer` = Always-Tier = garantiert, wie instructions; ein Pin kann bei Budget-Überlauf droppen, `immer` nie. Das ist genau der Fix für Soennes 8 Globals (garantiert statt pin-gated). Trade-off: ein sehr großes `immer`-Doc kann das Budget füllen und `auto`-Docs verdrängen — bewusste Kurator-Entscheidung, sichtbar am Meter (Bernstein/„fast voll"). **Alternative:** `immer` respektiert den Cap (kann droppen) — widerspräche „garantiert enthalten", nimmt der Semantik den Sinn. Abgelehnt.
9. **REST-API + apiclient jetzt, CLI/MCP-Verben deferred?** — *Empfehlung: JA* — REST `POST …/context-mode` + apiclient-Methode jetzt (generisches Feature für alle Hosts; Agenten sind erstklassige Kontext-Konsumenten und dürfen ihre eigenen Memories auf `immer`/`nie` setzen). CLI-Verb (`flow docs mode …`) + MCP-Tool bleiben Deferred (Self-Review). Trade-off: ein Handler + Client-Methode mehr jetzt. **Alternative:** nur Web-Handler (kein REST) — spart Task 3, lässt aber Agenten ohne Modus-Weg. Abgelehnt.

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads (kanonisch, Degradations-Modus wie L4/L5).** Vollständig gelesen: der L5-Plan (Formatvorbild, alle 8 Tasks + OE + Self-Review), AGENTS.md, das L5-Ledger, **`compose_context.go` komplett** (alle Typen + exakter Compose-Switch :200 + Sort :240 + Used-Verbuchung :231 + `StandingOf` :412 + `ExecuteForNode`/`composeForChain`/`globalAllowed`), `domain/document.go` (Document-Struct + Priority-Zielstelle + Doc-Typen + Validate), `pgstore/documents.go` (`docCols`/`prefixedDocCols` :31/33 + `Create`-Arity `$18`+18-Args :92-108 + **drei** Scanner :400/513/557 + `Update`/`UpsertByPath`/`ListForContext` + `SetPinned`/`SetPriority`-Muster), **`documents_embed.go` `StaleDocuments`** (der 4. Reader :41-43, `&d.Priority, &attempts`), `ports.go` (DocumentStore-Interface + SetPinned/SetPriority :213/219 + ErrDocumentNotFound), `set_pinned.go` (Usecase-Muster), `documents.go` httpserver (`handlePinDocument` :295 — REST-Muster), `webui_kontext.go` (kontextDataFor/renderKontext/handleWebKontextReorder/Pin), `kontext.templ`+`kontext_vm.go` (Row/Always-VMs + BuildKontextVM + KontextFragment/kontextRow/kontextAlwaysRow), `cockpit_context_vm.go` (BuildCockpitContext + AlwaysN + fmtThousandsDE), `doc_context_vm.go` (DocContextVM/BuildDocContext), `document.templ` (docrail-Kontext-`.blk` :120 + Provenance-Pin :80), `cockpit_rail.templ` (CockpitRailBlocks + Kontext-`.blk` :39), `cockpit_head.templ` (Spine/cockpitIdentity :17-45), `cockpit_vm.go` (NodeCockpit + DescriptionHTML :70), `webui_cockpit.go` (nodeCockpitData + **toter RenderDocument-Pfad :41-43** + node.updated-Emit), `server.go` (Server-Felder + Routen), `main.go` (Wiring), `apiclient/context.go` (SetPinned/ReorderContext :71/76), `tailwind.css` (`.seg`/`.seg-sm`/`.meter`/`.blk`/`.krow`/`.row` :335/486), i18n-Katalogstruktur (`context.*` :133-151, `document.context.*` :257, `document.pin*` :247). Dossier in Scratch (`l55-dossier.md`).
- **Degradations-Notiz:** agy/gemini-Dossier NICHT als Vorstufe genutzt — die Lückensuche (Phase 3) lief per `codex exec` + `agy` synchron im Vordergrund (unten). Das Grounding ist **first-hand kanonisch** (jede verwendete Signatur direkt am Code verifiziert). Kein Abbruch.
- **Flow-Recall:** L5.5-Kontext stammt aus dem Dispatch + L5-Ledger („PIN-DATENVERLUST … PRODUKT-LEHRE für L5.5: globale Pool-Mitgliedschaft darf nicht am Pin hängen (context_mode)"). Lokale Dateien kanonisch.

### Spec-Deckung — jeder Absatz auf einen Task gemappt
- **Fachlicher Kern „Feld" (Migr 0030, Domain, Store, Port, Fakes)** → Task 1. **CHECK auf die drei Werte** → Task 1 Migration + der Empty-String-Trap-Constraint.
- **„Compose-Semantik auto/immer/nie"** → Task 2 (nie→Hidden, immer→AlwaysMemories uncapped+tag-gate-frei, auto=Bestand, StandingOf, ContextItem.ContextMode). Default-auto-Neutralität = Task-2-Test (f/e).
- **„UI: Modus-Umschalter an jeder Kuratieren-Zeile (Rang UND Immer) + Doc-Seite; Instruction auf nie degradierbar"** → Task 4 (Rang/Always/Hidden + Doc-Block; `nie` demotet auch instruction — Task-2-Test d + Task-4-Rendering).
- **„REST POST /api/v1/documents/{id}/context-mode; CLI/MCP deferred"** → Task 3 (+ apiclient; CLI/MCP Deferred, OE #9).
- **„Daten-Nachlauf: 8 Globals → immer, dev+PROD, kein goose"** → Task 6 Step 7 (owner-scoped SQL-Runbook).
- **„Kuratieren-Pin-Falle: Guard/Empfehlung"** → OE #4 + #8: `immer` löst die Falle für die 8 (garantiert, pin-frei); für global-`auto`-Docs bleibt der L5-Pin-Toggle, aber der neue Ausgeblendet-Abschnitt + die Doc-Seite geben `nie`/`auto`/`immer` zwei sichtbare Rückwege → keine Einbahnstraße mehr. (Ein zusätzlicher Inline-Hinweis beim Entpinnen eines global-`auto`-Docs ist bewusst NICHT gebaut — `immer` ist der vorgesehene Weg, OE #8.)
- **Soenne-Scope-Add „Projekt-Description wieder anzeigen (toter DescriptionHTML-Pfad)"** → Task 5 (Plaintext-Identitätszeile, RenderDocument entfernt; Baum-Untertitel OE #7).

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Absatz ohne Task:** keiner (Mapping oben vollständig; CLI/MCP-Verben bewusst Deferred per Dispatch-Erlaubnis).
- **(b) Zustände je Task:** T4 benennt leer/immer/nie/lang/mobil-375/Fehler explizit; T5 leer(keine Description)/lang(Ellipsis)/mobil; T1–T3 sind Backend (Zustände = Testfälle: default-auto, immer-uncapped, nie-hidden, cross-owner, invalider Modus); T6 ist der Gate.
- **(c) Querschnitte:** main.go-Wiring → T3 (SetContextMode) + T6-Verify (T4/T5 nutzen nur Bestands-Server-Felder); SSE je Mutation → `document.updated` (T3 REST, T4 Web-Kontext/Web-Doc) + Konsumenten benannt; Description-Mutation = `node.updated` (Bestand, Head reloadet); i18n beide Kataloge → T4 Key-Step; Responsive → T4/T5 + Gate 960/375; Owner-Scoping → Negativtests T1 (SetContextMode cross-owner), T3 (REST cross-owner + invalider Modus), T4 (Web-Handler Fremd-Doc No-op), Compose owner-scoped (Bestand).
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; Step 0 rg-Verifikation aller Bestandsnamen; „Bestand gewinnt". **Spaltenlisten-Lehre VERSCHÄRFT** (VIER Reader inkl. StaleDocuments + Create-Arity `$19` + **CHECK-Empty-String-Koaleszenz** via `OrAuto`) als Compiler-/CHECK-geführte Pflicht in T1; Interface-Ripple (alle Fakes `SetContextMode`) in T1.

### Adversariale Lückensuche — Berater-Findings + Verbleib

Beide Berater liefen SYNCHRON im Vordergrund gegen Spec + Plan-Entwurf + Dossier + realen Code mit dem wörtlichen Lücken-Auftrag. **`codex exec`** (gpt-5.5, `--sandbox read-only`, `model_reasoning_effort=high`) lief sauber (7 Findings). **`agy`/Gemini 3** (`--print`) lief nach Flag-Korrektur (`--print` statt bare-TTY) sauber (1 Finding, kritisch). **`gemini`-CLI ist tot** (`IneligibleTierError` — der bekannte OAuth-Ausfall, Memory `reference_gemini_cli_oauth_dead`); `agy` deckt die Gemini-Sicht ab. **Degradations-Notiz:** keine — beide Berater-Sichten vorhanden und komplementär.

**agy/Gemini 3 — 1 CRITICAL, EINGEARBEITET:**
1. **[eingearbeitet — Task 2 Files + Step 0 Renderer-Inventar + Step 5b + Task 6 Live-Smoke]** (agy #1, KRITISCH) Der Plan aktualisierte alle `ComposedContext`-Konsumenten im WebUI, vergaß aber den **`cmd/flow/context.go`-Markdown-Renderer** (die reine `ComposedContext` → Markdown-Funktion, die der SessionStart-Hook in Claudes Startkontext injiziert). Der iteriert `Instructions`/`ActiveContext`/`Memories`, NICHT das neue `AlwaysMemories` → `immer`-Docs lägen im Server-Compose (Used/JSON), erreichten aber **nie den tatsächlichen Agenten-Prompt** — das Kernversprechen von `immer` wäre still gebrochen (am Code verifiziert: `cmd/flow/context.go` iteriert Memories-Gruppen `ccn[g.key]`). → Task 2 zieht jetzt `cmd/flow/context.go` + `context_test.go` mit; Step 0 hat ein Renderer-Inventar (`rg` über cmd/ + internal/) um jeden weiteren (MCP-)Renderer zu finden; das Live-Gate prüft den `flow context`-Markdown, nicht nur das JSON. agy bestätigte explizit als sauber: die 4 pgstore-Reader + Create-Arity + CHECK/`OrAuto`, die `auto`-Neutralität, den Tag-Gate-Bypass globaler `immer`-Docs, `UpsertByPath`-Unabhängigkeit und die Zwei-Wege-`nie`-Wiederherstellung.

**codex exec — 7 Findings, ALLE eingearbeitet:**
2. **[eingearbeitet — Task 3 Step 0 + Usecase + Handler]** (Codex #2) Der Plan nutzte `ports.ErrInvalidDocument` — dieser Sentinel liegt aber in **`internal/domain/errors.go`** (`domain.ErrInvalidDocument`), nicht in `ports` (am Code verifiziert). → Usecase + Handler + rg-Step auf `domain.ErrInvalidDocument` korrigiert; der Fallback-`ErrInvalidContextMode`-Halbsatz entfernt (der echte Sentinel existiert und wird durchgängig verdrahtet).
3. **[eingearbeitet — Task 1 Step 7]** (Codex #1) Der failing-Test deckte nur `Create`/`Get`/`SetContextMode` ab (die alle nur `scanDocument` üben); die anderen drei Reader (`scanSearchHit`/`scanSemanticHit`/`StaleDocuments`) blieben ungetestet-erwähnt. → Step 7 verlangt jetzt das **volle pgstore-Paket** (die Bestand-`Search`/`SemanticSearch`/`StaleDocuments`-Tests üben die drei Reader; fehlt ein `StaleDocuments`-Test, Minimal-Roundtrip ergänzen).
4. **[eingearbeitet — Task 1 Step 7 + Task 6 Live-Smoke]** (Codex #4) Die Fachanforderung „`nie` bleibt in Suche/Wissen sichtbar" hatte keinen Test/Smoke; `scanSearchHit`/`scanSemanticHit` sind durch die Spaltenänderung regressionsanfällig. → pgstore-`Search`-Test (nie-Doc wird weiter gefunden) + Live-Smoke-Zeile ergänzt.
5. **[eingearbeitet — Task 4 Step 5]** (Codex #3) Für den Doc-Seiten-Rückweg `POST /wissen/{id}/mode` fehlte ein eigener Mutationstest (Fragment, Owner-Scope, invalider Modus, `document.updated`-Emit). → expliziter Doc-Mode-Mutationstest ergänzt.
6. **[eingearbeitet — Task 4 Widget-Block + Step 5]** (Codex #6) Die 375px-Containment-Anforderung für den neuen Segmented-Control neben der `.right`-Icon-Gruppe war nur als Zustands-Label genannt, ohne konkreten CSS/Template-Schritt + Overflow-Assertion. → konkreter `.right`-Containment-Schritt (`flex-wrap`/eigene Zeile, benannte Klasse) + Overflow-Gate-Check.
7. **[eingearbeitet — Task 5 Step 0/Step 4/Step 1]** (Codex #7) Die Kurzbeschreibung sollte unter das `<h1>`, aber `.spine-main` ist eine **Flex-Row** — ein `<p>` nach dem `<h1>` würde zum Geschwister-Flex-Item (daneben, nicht darunter). → Titelblock (h1 + Beschreibung) in eine `.spine-title`-Spalte gewickelt + Layout-Test (lang/375px).
8. **[eingearbeitet — Task 6 Step 7]** (Codex #5) Der Daten-Nachlauf `UPDATE … WHERE type='memory' AND node_id IS NULL` träfe **alle** globalen Memories, nicht nur die 8 Feedback-Docs; ohne Preselect + ID-Liste + Vorher/Nachher-Count riskiert er Kollateral-Änderungen. → präzises Runbook: selektieren → zählen → Soenne bestätigt → gezieltes `UPDATE … WHERE id IN (…)` → gegenzählen.

**Von beiden explizit als sauber bestätigt (kein Plan-Change):** die VIER-Reader-Topologie inkl. `StaleDocuments` + Create-Arity `$19` + **CHECK-Empty-String-Koaleszenz via `OrAuto`** (Task 1, der eigentliche Datenintegritäts-Kern — von beiden bestätigt); `auto`-Verhaltensneutralität; `immer`-Tag-Gate-Bypass für globale Docs; `nie` komplett aus Used/Ranked/Memories/Always; `UpsertByPath`-Unabhängigkeit (DB-Default); die Zwei-Wege-`nie`-Wiederherstellung (Doc-Seite + Hidden-Abschnitt) — **keine neue Einbahnstraße** (die eigentliche Sorge des Slices).

**Dissens:** keiner — die Berater überschnitten sich nicht (agy fand den CLI-Renderer-Gap, codex sieben orthogonale Test-/Sentinel-/Layout-Lücken); alle Sichten komplementär und eingearbeitet. **Netto aus der Lückensuche: 1 CRITICAL (CLI-Markdown-Renderer für `immer`) + 1 CRITICAL-Korrektur (falscher Sentinel) + 6 substanzielle Test-/Layout-/Runbook-Ergänzungen — alle verbucht.**
