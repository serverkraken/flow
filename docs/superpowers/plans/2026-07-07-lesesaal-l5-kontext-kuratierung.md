# Lesesaal L5 — Kontext-Kuratierung (Budget-Meter · Rang · Anpinnen · Kuratieren-Fläche) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die **WebUI-Fläche für das fertige Kontext-Backend**. Das Kontext-System (Compose + cap+rank + Pins, B1–B4, LIVE) hat bisher **null** WebUI-Sichtbarkeit (`rg "kontext" internal/adapter/webui/` = 0). L5 gibt ihm drei Instrumente in Lesesaal-Sprache: (1) das **Kontext-Instrument** in der Cockpit-Meta-Spalte (Budget-Meter `11.891 tk / 12.000` bernstein ab ~95 %, Zeilen Enthalten/Verworfen/Angepinnt, nummerierte Top-Pins, Link „Kuratieren — sortieren & pinnen ›"); (2) am Dokument den **Kontext-Rang** in der docrail („Im Agenten-Kontext · enthalten ✓ · Rang 04/24" — das *Anpinnen* selbst existiert seit L3 in der Provenance-Zeile); (3) die **Kuratieren-Fläche** `/kontext/{nodeID}` — die neue, im Mockup nur als `#`-Link angedeutete Reorder-/Pin-Oberfläche. Dafür ist genau **eine neue Backend-Fähigkeit** nötig: ein **Prioritäts-Feld** an Dokumenten, das in den bestehenden cap+rank-Sortierschlüssel einsortiert wird, plus ein **Reorder-Usecase + REST-API** (die Pin-API existiert seit B3). Nach L5 folgen L6 Artefakte und L7 Dunkel-Zwilling + Politur (L5/L6 tauschbar, Spec §17).

**Architecture:** Server-rendered wie gehabt (templ + htmx + Tailwind, kein SPA, kein Node). Der Kern ist **additiv** — es fällt nichts, es wird nichts entkristallisiert (die sichtbare App ist seit L1–L4 vollständig Lesesaal). Fünf Schichten:
1. **Feld:** `documents.priority INTEGER NOT NULL DEFAULT 0` (Migration `0029`), `domain.Document.Priority`, pgstore-Spaltenlisten + drei Scanner + `SetPriority`-Storemethode, Port-Erweiterung (`ports.DocumentStore`). Der Default 0 macht L5 **verhaltensneutral zum Bestand**: alle heutigen Docs haben Priorität 0 → identische cap+rank-Reihenfolge wie vor L5.
2. **Rang-Kern:** die **reine** `usecase.Compose` bekommt `priority desc` in ihren Sortierschlüssel (nach `pinned`, vor `tierRank`) und exponiert eine **flache, geordnete `Ranked []RankedItem`-Liste** (mit `Included bool` + 1-basiertem `Rank`), die alle drei Instrumente füttert (Meter-Zähler, Dokument-Rang „04/24", Kuratieren-Liste). Neu: `ComposeContext.ExecuteForNode(ctx, owner, nodeID, cap)` — ein **ID-basierter** Compose-Einstieg (der bestehende `Execute` löst per Slug/Binding auf; Slugs sind aber nur **geschwister-eindeutig** [Migr 0018], ein Slug-Override träfe im WebUI womöglich einen Fremd-Knoten — deshalb geht die WebUI über die Knoten-**ID**). Plus der reine Helfer `StandingOf(cc, docID)` für den Dokument-Rang.
3. **Mutation:** `usecase.ReorderContextDocs` (stempelt dichte absteigende Prioritäten auf eine geordnete ID-Liste, owner-scoped, über die Einzel-Primitive `SetPriority`) + REST `POST /api/v1/context/reorder` + Web-Handler. **SSE:** jede Kuratierungs-Mutation (Pin-Toggle, Reorder) emittiert `document.updated` (Bestand: der Pin-Handler tut das bereits) — der Konsument ist je Fläche benannt; die Cockpit-Rail bekommt dafür `document.*` in ihre `hx-trigger`-Liste ergänzt.
4. **Instrumente (templ):** ein `.blk`-Block in `CockpitRailBlocks` (Mockup Z.647–660) + ein `.blk`-Block in der `DocumentFragment`-docrail (Mockup Z.794–798). Beide bauen aus fertigen, unit-getesteten Go-VMs (`BuildCockpitContext`, `BuildDocContext`); die templ nimmt fertige VMs.
5. **Kuratieren-Seite:** `/kontext/{nodeID}` — Lesesaal-Doktrin (Inhalt auf Papier, das **eine** Instrument = Budget-Meter auf `--panel`, Haarlinien-Zeilen statt Karten, **kein Drag-Drop** — schlichte Höher/Tiefer-Reorder-Mechanik + Pin-Toggle pro Zeile, eine `.cutline` markiert die Budget-Grenze). Node-scoped Seite, **dokument-globale** Prioritätswirkung (Priorität ist eine Eigenschaft des Dokuments, kein (Dokument,Knoten)-Paar).

**Design-Realität (verifiziert, Bestand gewinnt):** Die Kontext-Instrument-CSS (`.meter`/`.meter i`/`.meter-l`/`.ctxrows`/`.pin`/`.pin .g`/`.pin .n`) ist **schon da** (tailwind.css:451–457, in L2/L3 aus dem Mockup übernommen). L5 fixt nur den Meter-Füllton (heute hart `--warn` aus dem 99 %-Demo → default `--accent`, `--warn` erst per `.meter.full`-Modifier) und ergänzt **eine** neue Klasse `.cutline`. Der **Anpinnen-Button** ist Bestand (L3, `DocumentFragment` Provenance-Zeile, `hx-post="/wissen/{id}/pin"`, i18n `document.pin`/`document.pinned`/`document.pin.hint`) — L5 fügt ihn **nicht** neu hinzu. Die Meta-Rails tragen die L5-Blöcke an den vom Bestand explizit dafür freigehaltenen Stellen (cockpit_rail.templ-Kommentar: „Kontext … L5 … does NOT belong here yet"; document.templ-Kommentar: „Im Agenten-Kontext … deliberately L5").

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · Schibsted Grotesk + JetBrains Mono (L1). **Eine** neue goose-Migration (`0029`); keine neuen Abhängigkeiten, kein neues Vendoring, kein Client-JS.

**Spec:** `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` (§9 Cockpit-Meta-Spalte = Kette·**Kontext-Instrument**·Bindings + Dokument-Meta-Spalte enthält Kontext-Rang · §10 **Kontext-Instrument** erstklassig, Budget-Meter/Enthalten/Verworfen/Pins/nummerierte Top-Pins/„Kuratieren ›", am Dokument „enthalten ✓ · Rang 04/24" + Anpinnen · §16 Punkt 5 „Prioritäts-Feld + Reorder/Pin-API + Kuratieren-UI (neu; cap+rank/Pins existieren) — eigener Slice" · §17 L5-Definition · §11 Eindämmung soweit relevant · §13 A11y `role="img"`+Label am Meter). Hintergrund-Spec Kontext-System: `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md`. **Normatives Mockup:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4 — bei Zweifel gewinnt das Mockup; **Kontext-Instrument-Panel = Z.647–660**, **Dokument-Kontext-Block = Z.794–798**, **Anpinnen-Button = Z.694**, **Kontext-CSS = Z.171–177**). **Mockup-Lücke:** die **Kuratieren-Fläche selbst hat KEIN Mockup** (der Link geht auf `#`) → das Layout ist ein Empfehlungs-Entscheid (Offene Entscheidung #1), nach Lesesaal-Doktrin geschnitten.

---

## Global Constraints

- Branch **`lesesaal-l5`** (frisch off `rebuild` `73b66d9`, bereits ausgecheckt); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende.
- **L4-LEHREN — in JEDEN Task-Dispatch-Text aufnehmen:** (1) **Tests/CI SYNCHRON foreground** ausführen, **KEINE Hintergrund-Läufe** (Subagenten verfallen sonst in Warte-Posen auf nie kommende Notifications). (2) **verify-generate/verify-css diffen gegen den Index** → bei uncommitted templ/css false-positiv; **erst `git add -A` stagen, dann `make ci`**. (3) **Nie zwei `make ci` parallel** (Podman-VM 2 GiB keilt bei parallelen Testcontainer-Läufen ein → Hard-Stop+Start). (4) **`make web` nach JEDER `.templ`-Änderung** (auch reine Klassen-Löschung ändert den Tailwind-Scan; verify-css ist ein Drift-Diff) und `internal/adapter/webui/static/app.css` mitcommitten. (5) **Live-Tick nur mit absoluten `data-since`-Ankern** (Epoche), nie relativ — irrelevant für L5 (keine neuen Timer-Flächen), aber wenn eine L5-Fläche einen laufenden Timer zeigt, gilt der Bestand-Anker.
- **NIE `make fmt`** ausführen. **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen (Memory).
- `make ci` muss am Task-Ende grün sein (Gate 75 %, aktuell ~85,6 %; `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket setzen). **Task 1 fügt Migration 0029 hinzu → die pgstore-Docker-Tests laufen gegen das neue Schema; Migration 0029 muss goose Up/Down-annotiert sein** (Memory: nur die pgstore-Docker-Tests fangen fehlende Annotationen).
- Nach JEDER `.templ`-Änderung: `make generate` und die `*_templ.go` mitcommitten. Nach jeder `web/tailwind.css`- ODER templ-Klassen-Änderung: `make web` + `app.css` mitcommitten.
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced (`TestCatalogsParity` prüft nur Key-**Existenz** — EN-Strings explizit ausformulieren, nicht „gleichwertig"). Keine hartkodierten Anzeige-Strings; `components.T(ctx, "key")`/`Tn`.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ✚ ✗ ✓ ○ · ↑ ↓ + SVG erlaubt), **keine Browser-Popups** (`verify-no-popups`). Die Kuratieren-Reorder-Buttons sind Haarlinien-`.btn.btn-q.btn-s`, kein `confirm()`.
- **owner-scoped überall** (jede Store-/Compose-/Reorder-Query trägt `u.ID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze). **Priorität ist owner-scoped** (`SetPriority` WHERE owner_id=…), **Compose ist owner-scoped** (Bestand), **Reorder ist owner-scoped**. Jede neue Datenfläche bekommt einen **Owner-Scope-Negativtest**: `SetPriority`/Reorder auf ein Fremd-Dokument → `ErrDocumentNotFound`/kein Effekt; `ExecuteForNode` auf einen Fremd-Knoten liefert keine fremden Docs; die Kuratieren-Seite eines Fremd-Knotens ist 404.
- **SSE-Regel (Mutation → Event → Konsument benannt):** Jede Kuratierungs-Mutation emittiert `domain.EventDocumentUpdated` (`"document.updated"`) über `s.Emitter.Emit` (Bestand: `handleWebDocPin` tut das schon). Konsumenten: `#document-fragment` (Bestand-Trigger `sse:document.updated`), `#cockpit-rail` (**Trigger in T5 um `document.created/updated/deleted` ergänzt**), `#cockpit-main` (Bestand hat `document.*` schon), das Kuratieren-`#content` (T7 neuer Trigger `sse:document.updated`). **Kein neues Event-Typ** (Offene Entscheidung #4 = reuse; Alternative `context.changed` dort begründet abgelehnt).
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt): der Meter-Fortschritt ist eine **erlaubte Datenbindung** (`style="width:{Pct}%"` — Präzedenz `wocheDayBarStyle`/`.day .bar i`), kein Design-Token. Alle Farben über `rgb(var(--token))` (`--accent`/`--warn`/`--live`/`--panel`/`--hair`/`--hairp`/`--meta`/`--faint`). Keine Arbitrary-`[#hex]`/`[px]`, wo eine benannte Klasse existiert (`.meter`/`.pin`/`.krow`/`.row`/`.blk`/`.typechip`/`.cutline` decken L5 ab). Bestand-`.meter` etc. sind da (tailwind.css:451–457) — **Bestand gewinnt**, T4 fixt nur den Füllton + ergänzt `.cutline`.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht):** JEDES als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Komponente, Usecase-Feld, Store-Methode, Test-Helper, i18n-Key, CSS-Klasse — z. B. `ComposeContext`, `Compose`, `ComposedContext`, `ContextItem`, `ContextBudget`, `DroppedCount`, `ContextResolveInput`, `bootstrapTypes`, `itemOf`, `estTokens`, `globalAllowed`, `SetPinned`, `handleWebDocPin`, `handlePinDocument`, `buildDocumentVM`, `DocumentVM`, `DocumentFragment`, `nodeCockpitData`, `NodeCockpit`, `CockpitRailBlocks`, `cockpitBody`, `handleWebNodeRail`, `renderNodeRail`, `NodeStore.Get`, `NodeStore.Ancestors`, `NodeAncestors`, `GetNode`, `ListForContext`, `docCols`, `prefixedDocCols`, `scanDocument`, `scanSearchHit`, `scanSemanticHit`, `EventDocumentUpdated`, `.meter`, `.pin`, `.krow`, `.blk`, `.narrow`, `.pagehead`, `.sect`, `.row`, `.typechip`, `DocTypeChipClass`, `DocTypeLabel`, `Avatar`, `AgentAvatar`, `Initials`, `AvatarTone`, `ShortName`, `FmtRelTime`, `ReadingTime`, `testCtx`, `renderToBuf`, `i18n.WithLocale`, `document.pin`, `cockpit.rail.chain`) vor dem Tippen per `rg -n "<Name>" internal/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden. Wo das Dossier eine Stelle nicht deckt: expliziter rg-Verifikationsstep im Task-Text (jeder Task hat Step 0).
- **Spaltenlisten-Lehre (B3-A1, verifiziert am Code — GEMINI-CRITICAL #13):** eine Änderung an `docCols`/`prefixedDocCols` **bricht JEDEN Leser** — nicht nur die Scanner, sondern auch die **arity-gekoppelte INSERT-Klausel von `Create`**. Verifiziert: (1) **drei** Scanner (`scanDocument` :548, `scanSearchHit` :392, `scanSemanticHit` :506) brauchen `&d.Priority`; (2) **`Create` (:92–108)** nutzt `docCols` **doppelt** — als INSERT-Spaltenliste **und** als RETURNING —, mit **hartkodiertem `VALUES ($1..$17)` + 17 gebundenen Args**: `priority` an `docCols` anzuhängen macht daraus eine 18-Spalten-INSERT mit nur 17 Werten → **Laufzeit-SQL-Fehler**. `Create` braucht deshalb ein **`$18`-Platzhalter + ein `d.Priority`-Arg** (neue Docs binden ihre — per Zero-Value 0 — Priorität explizit). (3) `Update` (:174–189) nutzt `docCols` **nur** im RETURNING (SET-Klausel explizit) → **keine** Arity-Kopplung, nur der Scan (via `scanDocument`) zieht mit. (4) `UpsertByPath` (:237) hat eine **eigene** INSERT-Spaltenliste + `RETURNING id, updated_at` (kein docCols-Scan) → **priority-frei sicher** (DB-Default 0). T1 fasst die drei Scanner **und** `Create`s VALUES/Args an; `go build ./...` + der pgstore-Test fangen jede Fehlstellung (Scan-Arity ≠ Spaltenzahl, Placeholder-Zahl ≠ Args).
- **Interface-Ripple:** `SetPriority` an `ports.DocumentStore` zwingt **jede** `ports.DocumentStore`-Fake (usecase-/httpserver-Tests) zur Methode. Der Compiler listet jede fehlende Implementierung — T1 fügt sie überall hinzu (`rg -rn "ports.DocumentStore" internal --glob '*_test.go'` + `go build ./... ./internal/...`).

## Priorität & cap+rank — Vorgabe (ENTSCHIEDEN, Spec §10/§16.5; NICHT erneut konsultieren)

Der **Bestand-Sortierschlüssel** der reinen `Compose` ist exakt (verifiziert compose_context.go:209–217, bestätigt vom cap+rank-Plan): `(pinned desc, tierRank asc, updatedAt desc)`, `tierRank{global:0, engagement:1, vorhaben:2, leaf:3}`. **Pins bypassen NICHT den Token-Cap**, sondern (a) den D7-Tag-Gate und (b) füllen zuerst; ein zu großer Pin wird trotzdem gedroppt (`Dropped.Pinned++`). Der **neue Schlüssel** ist:

```
(pinned desc, priority desc, tierRank asc, updatedAt desc)
```

**Begründung (Offene Entscheidung #2 = ENTSCHIEDEN „A"):** Priorität ist eine **manuelle Kurations-Übersteuerung** — ein Kurator (Mensch **oder** Agent) hebt ein Dokument über die Tier-Automatik, ohne die Pin-Garantie zu brauchen. `priority` liegt **nach** `pinned` (Pins bleiben oben/garantiert) und **vor** `tierRank` (Priorität darf ein Leaf-Memory über ein Engagement-Memory heben — das ist der Sinn von „sortieren"). **Default 0 = verhaltensneutral** (alle Bestandsdocs 0 → identische Reihenfolge wie vor L5; die Compose-Table-Tests des Bestands bleiben grün). Priorität **bypassed den Cap NICHT** — sie ändert nur die Füll-Reihenfolge (was zuerst ins Budget kommt). Garantie = Pin; Präferenz = Priorität. Die konservative Alternative (Priorität nur **innerhalb** eines Tiers) ist in OE #2 begründet abgelehnt.

**Flache Rang-Liste:** `Compose` exponiert zusätzlich eine geordnete `Ranked []RankedItem` (`{Item, Group, Included bool, Rank int}`), im **selben** Füll-Loop befüllt (kein zweiter Sort): jedes Pool-Item in globaler Rang-Reihenfolge, `Included`+1-basierter `Rank` wenn es ins Budget passt, sonst `Included=false, Rank=0`. Das ist die **eine** Quelle für Meter-Zähler, Dokument-Rang „04/24" und die Kuratieren-Liste. Die Bestand-Felder `Memories map[tier][]ContextItem` + `Dropped` bleiben unverändert (JSON-API-Parität für CLI/MCP; das neue `Ranked` ist additiv, `omitempty`). Instructions + ActiveContext bleiben **always-tier** (nie im Pool, nie im `Ranked`; im Dokument-Rang als „immer enthalten").

**ID-basierter Einstieg:** Die WebUI komponiert **per Knoten-ID** (`ExecuteForNode`), nie per Slug — Slugs sind geschwister-eindeutig (Migr 0018), ein Slug-Override (`ContextResolveInput.NodeOverride`, für CLI/MCP gedacht) könnte einen Fremd-Sibling treffen. `ExecuteForNode` spiegelt die Cockpit-Ketten-Montage (`Get` + `Ancestors`, self als `chain[0]` sicherstellen — verifiziert webui_cockpit.go:44–47).

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1–L4)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. Dispatches nennen das Modell NIE implizit (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 `documents.priority` (Migr 0029 · Domain · pgstore 3 Scanner + SetPriority · Port · Fakes) | `lesesaal-implementer-deep` | Sonnet · high |
| 2 Compose-Kern (priority-Sort · `Ranked` · `ExecuteForNode` · `StandingOf`) | `lesesaal-implementer-deep` | Sonnet · high |
| 3 Reorder-Usecase + REST `/api/v1/context/reorder` + SSE + Wiring | `lesesaal-implementer` | Sonnet · medium |
| 4 L5-CSS (`.meter`-Zustände accent/full-warn · `.cutline`) | `lesesaal-implementer` | Sonnet · medium |
| 5 Cockpit-Kontext-Instrument-Panel (Rail-`.blk` · `ExecuteForNode` in nodeCockpitData · Rail-SSE += document.*) | `lesesaal-implementer-deep` | Sonnet · high |
| 6 Dokument-docrail „Im Agenten-Kontext"-Block (Rang N/M · enthalten ✓) | `lesesaal-implementer` | Sonnet · medium |
| 7 Kuratieren-Seite `/kontext/{nodeID}` + Web-Reorder/Pin-Handler | `lesesaal-implementer-deep` | Sonnet · high |
| 8 Wiring-Gate (main.go · Sweep · make ci · Live-Smoke · Breakpoints) | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Priorität-cap+rank-Vorgabe + „Branch `lesesaal-l5`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch. **Explizit im Dispatch:** „Tests/`make ci` SYNCHRON foreground, keine Hintergrund-Läufe; erst `git add -A`, dann `make ci`; nie zwei `make ci` parallel."
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range (BASE = Task-Base, nie HEAD~1). `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch): `gemini-bigcontext` (agy) über `git diff --name-only rebuild..HEAD`; Fallback `code-searcher`/rg. Dispatch-Text unten.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen. **Fokus:** Owner-Scoping über alle drei Compose-Call-Sites; `Ranked`-Rang-Konsistenz zwischen Cockpit-Zähler, Dokument-Rang und Kuratieren-Liste (dieselbe Quelle?); SSE-Trigger-Vollständigkeit (Rail bekommt document.*?); Interface-Ripple (alle Fakes haben SetPriority?); Multi-Tenant-Negativtests real vorhanden.
4. `lesesaal-mockup-auditor` → Abweichungen fixen (Referenzzeilen: Kontext-Panel Z.647–660, Dokument-Block Z.794–798, Kontext-CSS Z.171–177). **Die Kuratieren-Fläche hat kein Mockup** → der Auditor prüft sie gegen die Lesesaal-Doktrin (Zwei-Flächen-Regel, keine Karten-in-Karten, Haarlinien-Zeilen), nicht gegen Pixel.
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — inkl.: Cockpit zeigt Kontext-Instrument (Meter füllt korrekt, Bernstein ab 95 %, Enthalten/Verworfen/Angepinnt stimmen gegen `flow context`/`GET /context`, Top-Pins nummeriert); „Kuratieren ›" führt auf `/kontext/{id}`; dort Höher/Tiefer verschiebt einen Doc, das Cockpit-Meter + der Dokument-Rang ziehen **live** (SSE) nach; ein Doc-Page zeigt „enthalten ✓ · Rang N/M"; Anpinnen (Bestand) togglet und der Pin taucht im Cockpit-Top-Pins auf; 960px- und 375px-Sichtprobe (Rail stackt, Kuratieren-Liste scrollt nicht horizontal, Meter-Panel bleibt Panel).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **verwaiste i18n-Keys** (in beiden Katalogen definiert, nirgends per `T(`/`Tn(` referenziert) — besonders `context.*`/`document.context.*`-Keys, die eine Umbenennung hinterlassen hat; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `w-[`, `h-[`, `text-[1`, `text-[.`) auf den L5-Flächen (cockpit_rail/document/kontext), wo eine benannte Lesesaal-Klasse existiert (Ausnahme: `style="width:…%"`-Datenbindung am Meter); (c) **verwaiste Symbole** mit **null** verbleibenden Konsumenten (`rg`-Zähler) unter den L5-Neubauten (`BuildCockpitContext`, `BuildDocContext`, `CockpitContextVM`, `DocContextVM`, `StandingOf`, `RankedItem`, `ReorderContextDocs`); (d) **cap+rank-Regressionen:** ist der Bestand-Sortierschlüssel `(pinned, tierRank, updatedAt)` in genau **einer** Stelle um `priority` erweitert (nicht dupliziert)? bleibt `Memories`/`Dropped` unverändert? (e) **SSE-Lücken:** emittiert jede Kuratierungs-Mutation (Reorder-API, Web-Reorder, Web-Pin) `document.updated`, und trägt `#cockpit-rail` den `document.*`-Trigger? Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` übersprungen; Nachlauf ist Orchestrator-Arbeit (Auto-Memory + flow-Mirror).

---

### Task 1: `documents.priority` — Migration 0029 · Domain · pgstore (3 Scanner + SetPriority) · Port · Fakes

**Files:**
- Create: `internal/adapter/pgstore/migrations/0029_documents_priority.sql`
- Modify: `internal/domain/document.go` (Document-Struct)
- Modify: `internal/adapter/pgstore/documents.go` (`docCols`, `prefixedDocCols`, **`Create`s VALUES/Args (Arity!)**, `scanDocument`, `scanSearchHit`, `scanSemanticHit`, neue `SetPriority`-Methode)
- Modify: `internal/ports/ports.go` (DocumentStore-Interface)
- Modify: **jede** `ports.DocumentStore`-Fake (Compiler-geführt: `rg -rn "func.*SetPinned\(ctx" internal --glob '*_test.go'` + `go build ./...`)
- Test: `internal/adapter/pgstore/documents_test.go` (SetPriority-Roundtrip + Owner-Scope-Negativtest, Muster `TestDocumentStore_SetPinned` :Bestand)

**Interfaces / Produces (für Tasks 2/3):**
- **`domain.Document.Priority int`** (`json:"priority"`) — nach `Pinned`/`Archived` einsortiert; Kommentar: „manual context-ranking priority (higher = ranked earlier within the memory pool; default 0). Set by ReorderContextDocs. Create binds it explicitly (zero-value 0 for new docs, since docCols is the shared INSERT column list); UpsertByPath omits it (own column list → DB default 0)."
- **`ports.DocumentStore.SetPriority(ctx, ownerID, id string, priority int) error`** — mirror `SetPinned`; **bumpt `updated_at` NICHT** (Priorität ist orthogonal zur Aktualität; ein Reorder-Batch würde sonst die `updatedAt desc`-Tiebreaker-Reihenfolge zerschießen — Offene Entscheidung #3); Owner-scoped; `ErrDocumentNotFound` bei 0 Rows.

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "const docCols|const prefixedDocCols" internal/adapter/pgstore/documents.go
rg -n "func scanDocument|func scanSearchHit|func scanSemanticHit|RETURNING .*docCols|INSERT INTO documents" internal/adapter/pgstore/documents.go
rg -n "func .*SetPinned|ErrDocumentNotFound" internal/adapter/pgstore/documents.go internal/ports/ports.go
rg -rn "func.*SetPinned\(ctx" internal --glob '*_test.go'   # jede Fake, die SetPriority auch braucht
ls internal/adapter/pgstore/migrations/ | tail -3          # höchste Nummer = 0028 → neu 0029
```
- [ ] **Step 1: Failing Test** — in `documents_test.go` (testcontainer; Muster des Bestand-`SetPinned`-Tests):
```go
func TestDocumentStore_SetPriority(t *testing.T) {
	ctx, ds := newDocStore(t) // Bestand-Helper — per rg den echten Namen verifizieren
	d, _ := ds.Create(ctx, domain.Document{OwnerID: "u1", Type: domain.DocMemory, Path: "m/p", Title: "T", Body: "b"})
	if err := ds.SetPriority(ctx, "u1", d.ID, 7); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	got, _ := ds.Get(ctx, "u1", d.ID)
	if got.Priority != 7 {
		t.Fatalf("Priority = %d, want 7", got.Priority)
	}
	// Owner-Scope: fremder Owner darf nicht schreiben.
	if err := ds.SetPriority(ctx, "u2", d.ID, 3); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("cross-owner SetPriority err = %v, want ErrDocumentNotFound", err)
	}
}
```
- [ ] **Step 2: Test laufen lassen** — Expected: FAIL (Feld/Methode fehlen; ggf. Compile-Fehler in Fakes). `DOCKER_HOST` auf Podman-Socket.
- [ ] **Step 3: Migration 0029** (goose Up/Down PFLICHT):
```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE documents DROP COLUMN priority;
```
- [ ] **Step 4: Domain-Feld** — `Priority int \`json:"priority"\`` in `domain.Document` (nach `Archived`/`ArchivedAt`, vor `CreatedAt`).
- [ ] **Step 5: pgstore-Spaltenlisten + `Create`-Arity + drei Scanner + SetPriority**
  - `docCols`: `priority` **am Ende** anhängen (`…, updated_by_kind, updated_by_ref, priority`).
  - `prefixedDocCols`: `d.priority` am Ende anhängen.
  - **`Create` (:92–108) — ARITY (Gemini-Critical #13):** `VALUES ($1,…,$17)` → **`$18`** ergänzen **und** `d.Priority` als 18. Arg in den `QueryRow(...)`-Aufruf (nach `nullIfEmpty(d.UpdatedByRef)`). Sonst 18-Spalten-INSERT mit 17 Werten → Laufzeitfehler. Das RETURNING (auch `docCols`) zieht über `scanDocument` (18 Scan-Ziele) mit.
  - `scanDocument` (:548), `scanSearchHit` (:392), `scanSemanticHit` (:506): `&d.Priority` einfügen **direkt nach `&updatedByRef`** und **vor** etwaigen Extra-Spalten (`&snippet` bzw. `&content, &dist`). (Reihenfolge = Spaltenreihenfolge; priority steht in docCols/prefixedDocCols am Ende, aber die Extra-Spalten der Such-Scanner kommen erst NACH prefixedDocCols → `&d.Priority` liegt vor ihnen.)
  - `Update` (:174–189): nur RETURNING (docCols) → **kein** Arity-Change, der Scan zieht via `scanDocument` automatisch mit.
  - `UpsertByPath` (:237): **unangetastet** (eigene INSERT-Liste, DB-Default 0, `RETURNING id, updated_at`).
  - **Test-Absicherung:** ein `Create`-Roundtrip-Test (neu angelegtes Doc hat `Priority == 0`) fängt die Arity-Regression zusätzlich zum bestehenden `SetPriority`-Test.
  - Neue Methode:
```go
// SetPriority sets the manual context-ranking priority (higher = ranked earlier
// within the memory pool). Owner-scoped; deliberately does NOT bump updated_at
// (priority is orthogonal to recency — see domain.Document.Priority).
func (s *DocumentStore) SetPriority(ctx context.Context, ownerID, id string, priority int) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET priority=$1 WHERE owner_id=$2 AND id=$3`, priority, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set priority: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}
```
- [ ] **Step 6: Port-Interface + alle Fakes** — `SetPriority` an `ports.DocumentStore` (Doku-Kommentar wie oben); dann `go build ./... ./internal/...` — der Compiler listet jede Fake ohne `SetPriority`; überall die triviale Implementierung ergänzen (In-Memory-Fakes: `d.Priority = priority`; erfinde-nichts, spiegele die Fake-`SetPinned`-Struktur).
- [ ] **Step 7: Bauen + Tests + Commit**
```bash
git add -A
go build ./... && go test ./internal/adapter/pgstore/... ./internal/usecase/... -race   # Docker-Socket
git commit -m "feat(pgstore): documents.priority (Migr 0029) + SetPriority — Kontext-Kurations-Feld"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig (keine templ/css-Änderung).

---

### Task 2: Compose-Kern — Priorität in cap+rank · flache `Ranked`-Liste · `ExecuteForNode(id)` · `StandingOf`

**Files:**
- Modify: `internal/usecase/compose_context.go` (`ContextItem`, `ComposedContext`, `Compose`, `ComposeContext.Execute`, neu `composeForChain`/`ExecuteForNode`/`StandingOf`, neuer Typ `RankedItem`/`ContextStanding`)
- Test: `internal/usecase/compose_context_test.go` (Bestand — die erschöpfenden Table-Tests MÜSSEN grün bleiben, plus neue Fälle)

**Interfaces / Produces (für Tasks 5/6/7):**
- **`ContextItem.Priority int`** (`json:"priority,omitempty"`) — aus `d.Priority` in `itemOf` gesetzt.
- **`RankedItem{ Item ContextItem; Group string; Included bool; Rank int }`** + **`ComposedContext.Ranked []RankedItem`** (`json:"ranked,omitempty"`, additiv — `Memories`/`Dropped`/`Instructions`/`ActiveContext` unverändert).
- **`ComposeContext.ExecuteForNode(ctx, ownerID, nodeID string, cap int) (ComposedContext, error)`** — ID-basiert (kein Slug-Override).
- **`ContextStanding{ State string; Rank, Total int; ScopeLabel string }`** + **`func StandingOf(cc ComposedContext, docID string) ContextStanding`** — `State ∈ {"included","dropped","always","absent"}`; `Rank`/`Total` nur bei `included` (Total = Anzahl included Memories); `always` für Instructions/ActiveContext; `absent` sonst.

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "func Compose\(|type ComposedContext|type ContextItem|func itemOf|sort.SliceStable|type ranked struct|out.Budget.Dropped" internal/usecase/compose_context.go
rg -n "func .*ComposeContext.*Execute|resolveLeaf|composeForChain|Nodes.Ancestors|Nodes.Get|ListForContext|bootstrapTypes|globalAllowed" internal/usecase/compose_context.go
rg -n "func .*Ancestors|func .*Get\(ctx" internal/ports/ports.go   # NodeStore.Ancestors/Get bestätigen
```
- [ ] **Step 1: Failing Tests** — in `compose_context_test.go`:
```go
// (a) Priorität hebt ein Leaf-Memory über ein Engagement-Memory (bei gleichem Pin-Status).
// (b) Ranked ist geordnet, Included/Rank korrekt, Rank läuft 1..N über die INCLUDED.
// (c) Backward-compat: alle Priority==0 → Memories-Reihenfolge identisch zum Bestand-Table-Test.
// (d) StandingOf: included(rank/total), dropped, always (instruction/activecontext), absent.
// (e) ExecuteForNode über Fake-NodeStore/DocStore liefert dieselbe Compose wie Execute
//     für einen aufgelösten Knoten; Owner-Scope wird durchgereicht (Fake prüft ownerID).
// (f) ExecuteForNode auf einen FREMDEN Knoten (Codex-Fund #2): ein owner-scoped
//     Fake-NodeStore.Get liefert für einen Knoten fremden Owners ErrNodeNotFound
//     → ExecuteForNode propagiert den Fehler, liefert KEINE fremden Docs. Muster:
//     NodeStore.Get(ctx,owner,id) ist owner-scoped (ports.go:86-89).
func TestCompose_PriorityLiftsAcrossTier(t *testing.T) { /* … */ }
func TestCompose_RankedFlatOrder(t *testing.T)        { /* … */ }
func TestCompose_ZeroPriorityIsBestandOrder(t *testing.T) { /* … */ }
func TestStandingOf_States(t *testing.T)              { /* … */ }
func TestComposeContext_ExecuteForNode(t *testing.T)  { /* … */ }
func TestComposeContext_ExecuteForNode_ForeignNode(t *testing.T) { /* Fake.Get→ErrNodeNotFound; want err, keine Docs */ }
```
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `Compose` — priority + Ranked** (die einzige Sort-Stelle erweitern, nicht duplizieren):
```go
// itemOf: it.Priority = d.Priority

type ranked struct {
	item   ContextItem
	group  string
	pinned bool
	prio   int    // NEW: d.Priority
	rank   int
	upd    string
}
// beim Anlegen: ranked{it, grp, d.Pinned, it.Priority, rankOf[grp], it.UpdatedAt}

sort.SliceStable(pool, func(i, j int) bool {
	if pool[i].pinned != pool[j].pinned {
		return pool[i].pinned
	}
	if pool[i].prio != pool[j].prio {
		return pool[i].prio > pool[j].prio // NEW: higher priority fills first
	}
	if pool[i].rank != pool[j].rank {
		return pool[i].rank < pool[j].rank
	}
	return pool[i].upd > pool[j].upd
})

// Füll-Loop: incl-Zähler + Ranked befüllen (KEIN zweiter Sort):
incl := 0
for _, r := range pool {
	if out.Budget.Used+r.item.EstTokens <= cap {
		out.Budget.Used += r.item.EstTokens
		out.Memories[r.group] = append(out.Memories[r.group], r.item) // Bestand
		incl++
		out.Ranked = append(out.Ranked, RankedItem{Item: r.item, Group: r.group, Included: true, Rank: incl})
		continue
	}
	// Bestand-Dropped-Zählung unverändert …
	out.Ranked = append(out.Ranked, RankedItem{Item: r.item, Group: r.group, Included: false, Rank: 0})
}
```
- [ ] **Step 4: `ExecuteForNode` + `composeForChain`-Extraktion** — den Post-Resolve-Schwanz von `Execute` (chain→ListForContext→globalAllowed→Compose) in `composeForChain(ctx, owner, chain, cap)` ziehen; `Execute` ruft ihn nach `resolveLeaf`; neu:
```go
// ExecuteForNode composes the context of a node addressed by ID (not slug —
// slugs are only sibling-unique). Mirrors the cockpit chain assembly.
func (uc ComposeContext) ExecuteForNode(ctx context.Context, ownerID, nodeID string, cap int) (ComposedContext, error) {
	leaf, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return ComposedContext{}, err
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, leaf.ID)
	if err != nil {
		return ComposedContext{}, err
	}
	if len(chain) == 0 || chain[0].ID != leaf.ID {
		chain = append([]domain.Node{leaf}, chain...)
	}
	return uc.composeForChain(ctx, ownerID, chain, cap)
}
```
- [ ] **Step 5: `StandingOf`** (rein):
```go
func StandingOf(cc ComposedContext, docID string) ContextStanding {
	for _, it := range cc.Instructions {
		if it.ID == docID {
			return ContextStanding{State: "always", ScopeLabel: it.ScopeLabel}
		}
	}
	if cc.ActiveContext != nil && cc.ActiveContext.ID == docID {
		return ContextStanding{State: "always", ScopeLabel: cc.ActiveContext.ScopeLabel}
	}
	total := 0
	for _, r := range cc.Ranked {
		if r.Included {
			total++
		}
	}
	for _, r := range cc.Ranked {
		if r.Item.ID == docID {
			if r.Included {
				return ContextStanding{State: "included", Rank: r.Rank, Total: total, ScopeLabel: r.Item.ScopeLabel}
			}
			return ContextStanding{State: "dropped", ScopeLabel: r.Item.ScopeLabel}
		}
	}
	return ContextStanding{State: "absent"}
}
```
- [ ] **Step 6: Tests + Commit**
```bash
git add -A && go test ./internal/usecase/... -race 2>&1 | tail -20
git commit -m "feat(usecase): Kontext-Priorität in cap+rank (pinned>priority>tier>recency) + flache Ranked-Liste + ExecuteForNode/StandingOf"
```
Expected: PASS; Bestand-Compose-Table-Tests grün (priority=0-Pfad unverändert).

---

### Task 3: Reorder-Usecase + REST `POST /api/v1/context/reorder` + SSE + Wiring

**Files:**
- Create: `internal/usecase/reorder_context.go` + `internal/usecase/reorder_context_test.go`
- Modify: `internal/adapter/httpserver/context.go` (`handleReorderContext`) + `internal/adapter/httpserver/server.go` (Server-Feld `ReorderContextDocs` + Route)
- Modify: `internal/adapter/apiclient/context.go` (+ `_test.go`) — **`ReorderContext`-Client-Methode** (Codex-Fund #1: Spec §16.5 „Reorder/**Pin**-API"; `SetPinned` hat schon einen Client [`context.go:37-71`], die Reorder-API braucht ihn auch, sonst bleibt sie für CLI/MCP unerreichbar — widerspräche OE #7)
- Modify: `cmd/flow-server/main.go` (Wiring — T8 verifiziert die Composition-Root)
- Test: `internal/adapter/httpserver/context_test.go` (Reorder-Roundtrip + Owner-Scope-Negativtest + Emit-Capture)

**Interfaces / Produces:**
- **`usecase.ReorderContextDocs{ Docs ports.DocumentStore }`** mit `Execute(ctx, ownerID string, orderedIDs []string) error` — stempelt dichte absteigende Prioritäten (`SetPriority(id, len-i)`); owner-scoped über die Einzel-Primitive; ein Fehler bricht ab (partielle Writes möglich — der Client re-submittet die volle Ordnung, idempotent).
- **REST `POST /api/v1/context/reorder`** Body `{"ids":[…]}` → 200; emittiert **einen** `document.updated` (Data `{"reordered": n}`), damit alle SSE-Konsumenten (`#cockpit-rail`, `#cockpit-main`, `#document-fragment`, Kuratieren-`#content`) neu laden.

- [ ] **Step 0: rg-Verifikation**
```bash
rg -n "func .*handleGetContext|func .*handlePutContextActive|s.Emitter.Emit|EventDocumentUpdated" internal/adapter/httpserver/context.go
rg -n "ComposeContext|SetPinned|ContextBudget|ReorderContextDocs" internal/adapter/httpserver/server.go cmd/flow-server/main.go
rg -n "mux.Handle\(\"POST /api/v1/context|mux.Handle\(\"POST /api/v1/documents/\{id\}/pin" internal/adapter/httpserver/server.go
```
- [ ] **Step 1: Failing Usecase-Test** — `reorder_context_test.go` mit einem Fake-DocumentStore: `Execute(owner, [c,a,b])` setzt Prioritäten 3,2,1 auf c,a,b; owner-fremder Aufruf (Fake gibt `ErrDocumentNotFound`) propagiert den Fehler.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Usecase**
```go
package usecase

import (
	"context"
	"github.com/serverkraken/flow/internal/ports"
)

type ReorderContextDocs struct{ Docs ports.DocumentStore }

// Execute stamps a dense descending priority (first = highest) on the given
// documents in the given order. Owner-scoped; a write failure aborts (the
// client re-submits the full order — idempotent).
func (uc ReorderContextDocs) Execute(ctx context.Context, ownerID string, orderedIDs []string) error {
	n := len(orderedIDs)
	for i, id := range orderedIDs {
		if err := uc.Docs.SetPriority(ctx, ownerID, id, n-i); err != nil {
			return err
		}
	}
	return nil
}
```
- [ ] **Step 4: REST-Handler** (`context.go`), owner-scoped, Emit:
```go
type reorderReq struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleReorderContext(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req reorderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.ReorderContextDocs.Execute(r.Context(), u.ID, req.IDs); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"reordered": len(req.IDs)}})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "n": len(req.IDs)})
}
```
- [ ] **Step 5: Server-Feld + Route + main.go** — `ReorderContextDocs usecase.ReorderContextDocs` in die Server-Struct (bei den Doc-Usecases); `mux.Handle("POST /api/v1/context/reorder", s.auth(http.HandlerFunc(s.handleReorderContext)))`; in `cmd/flow-server/main.go` `ReorderContextDocs: usecase.ReorderContextDocs{Docs: documentStore}` ins Server-Literal (neben `SetPinned`).
- [ ] **Step 6: Handler-Test** (`context_test.go`, httptest): Reorder setzt Prioritäten (Fake-Store-Assertion) + emittiert **genau ein** `document.updated` (Emitter-Capture) + User B kann A's Doc nicht umsortieren (404/ErrDocumentNotFound).
- [ ] **Step 7: apiclient `ReorderContext`** (Codex-Fund #1) — Methode nach dem Muster von `SetPinned` (`internal/adapter/apiclient/context.go`; erst `rg -n "func .*SetPinned|func .*ComposeContext|type Client" internal/adapter/apiclient/context.go`): `POST /api/v1/context/reorder` mit `{"ids":[…]}`; + httptest-Client-Test (Roundtrip gegen einen Stub-Server). Kein CLI-Command in L5 nötig (die Fähigkeit ist über MCP/apiclient erreichbar — CLI-Verb bleibt optional/L6, Self-Review).
- [ ] **Step 8: Tests + Commit**
```bash
git add -A && go test ./internal/usecase/... ./internal/adapter/httpserver/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(context): ReorderContextDocs + POST /api/v1/context/reorder + apiclient (owner-scoped, emits document.updated)"
```
Expected: PASS.

---

### Task 4: L5-CSS — `.meter`-Zustände (accent-default / full-warn) + `.cutline` (Kuratieren)

**Files:**
- Modify: `web/tailwind.css` (`.meter i` Füllton fixen; `.meter.full i` + `.meter-l .warn` + `.cutline` ergänzen)
- Modify: `internal/adapter/webui/components/styleguide.templ` (L5-Sektion)
- Test: `internal/adapter/webui/components/styleguide_test.go` (Render-Smoke, Muster L4-Sektion)

**Interfaces / Produces:** benannte Klassen. **Bestand gewinnt** — `.meter`/`.meter i`/`.meter-l`/`.ctxrows`/`.pin`/`.pin .g`/`.pin .n` existieren (tailwind.css:451–457). Nur:
- **`.meter i` Füllton:** heute hart `background:rgb(var(--warn));width:99.1%` (99 %-Demo-Zustand aus dem Mockup). → `background:rgb(var(--accent))` als Default, `width` **entfernen** (kommt per `style="width:{Pct}%"`-Datenbindung). Neu **`.meter.full i{background:rgb(var(--warn))}`** (≥95 %, per VM `Full`-Flag → Klasse `meter full`).
- **`.meter-l .warn`** (die „fast voll · 12.000"-Notiz, Mockup Z.650): `color:rgb(var(--warn));font-weight:600`.
- **`.cutline`** (NEU, Kuratieren-Budget-Grenze — keine Mockup-Vorlage, Doktrin-konform): eine Haarlinie mit zentrierter Versalien-Notiz:
```css
.cutline { display:flex; align-items:center; gap:12px; margin:14px 0 6px; font-size:11px; letter-spacing:.09em; text-transform:uppercase; color:rgb(var(--warn)); }
.cutline::before, .cutline::after { content:""; flex:1; height:1px; background:rgb(var(--hairp)); }
```
- **Prüfen** (`rg -n "\.meter|\.cutline|\.pin\b|\.ctxrows" web/tailwind.css`): nichts doppeln; `.meter i` NUR modifizieren, nicht neu anlegen.

**Zustände dieser Fläche:** Styleguide zeigt (a) einen `.meter` bei ~40 % (accent), (b) einen `.meter.full` bei 98 % (warn) + `.meter-l` mit `.warn`-Notiz, (c) drei `.pin`-Zeilen (nummeriert), (d) eine `.cutline`. Sichtprobe 375px im Gate.

- [ ] **Step 1: Bestand + Mockup prüfen** — `rg -n "\.meter|\.pin\b|\.ctxrows|\.cutline|\.meter-l" web/tailwind.css`; `sed -n '171,177p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html`.
- [ ] **Step 2: Failing Test** — `TestStyleguide_HasLesesaalL5Section`: gerendertes `StyleguidePage()` enthält `"meter full"` und `"cutline"`.
- [ ] **Step 3: Laufen lassen** — FAIL.
- [ ] **Step 4: CSS** — `.meter i` accent-default + width-Zeile raus; `.meter.full i`, `.meter-l .warn`, `.cutline` ergänzen. `@source not`-Zeilen nicht anfassen.
- [ ] **Step 5: Styleguide-Sektion „Lesesaal L5"** — die vier Demos (`role="img"` + aria-label am Meter zeigen, damit T5/T7 die Struktur kopieren).
- [ ] **Step 6: Bauen + Test + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): L5-CSS — Meter-Zustände (accent/full-warn) + cutline (Kuratieren-Budgetgrenze)"
```
Expected: PASS; `app.css` geändert.

---

### Task 5: Cockpit-Kontext-Instrument-Panel (Rail-`.blk` · `ExecuteForNode` in nodeCockpitData · Rail-SSE += document.*)

**Files:**
- Create: `internal/adapter/webui/cockpit_context_vm.go` + `internal/adapter/webui/cockpit_context_vm_test.go` (reiner Builder `BuildCockpitContext`)
- Modify: `internal/adapter/webui/cockpit_vm.go` (`NodeCockpit` += `Context *CockpitContextVM`)
- Modify: `internal/adapter/webui/cockpit_rail.templ` (neuer `.blk` zwischen Kette/Contributors und Bindings)
- Modify: `internal/adapter/webui/cockpit.templ` (`#cockpit-rail` `hx-trigger` += `document.*`)
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData`: `ExecuteForNode` → `d.Context`)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/cockpit_rail_render_test.go` (Bestand), `internal/adapter/httpserver/webui_cockpit_test.go` (Owner-Scope + Panel-Präsenz)

**Interfaces:**
- **`webui.CockpitContextVM`** + **`func BuildCockpitContext(cc usecase.ComposedContext, nodeID string) *CockpitContextVM`** (rein, domain-/store-frei):
```go
type ContextPinVM struct{ Num, Title string } // "01", Doc-Titel
type CockpitContextVM struct {
	NodeID           string        // Kuratieren-Link-Ziel
	UsedStr, CapStr  string        // "11.891", "12.000" (Tausender-Punkt, de)
	Pct              int           // 0..100 (Meter-Breite), clamped
	Full             bool          // Pct >= 95 → Klasse "meter full" + warn-Notiz
	IncludedN        int           // included Memories (len Ranked.Included) — Mockup "24 Docs"
	DroppedN         int           // Σ Budget.Dropped (Leaf+Vorhaben+Engagement+Global)
	PinnedN          int           // context-scoped: Ranked mit Item.Pinned (Abweichung dokumentiert, s. u.)
	TopPins          []ContextPinVM // included && Pinned, top 3, nummeriert
}
```
  Ableitung aus `cc.Budget` (Used/Cap/Dropped) + `cc.Ranked`. **PinnedN ist context-scoped** (Pins in DIESEM Ketten-Kontext), nicht global — das Mockup-„12" war die globale Korpus-Zahl; das node-scoped Panel zeigt die Kontext-Pins (dokumentierte, bewusste Abweichung; Offene Entscheidung #5). Tausender-Formatierung: erst `rg -n "func Fmt.*int|thousand|de-DE|Sprintf.*%d" internal/adapter/webui` — existiert kein Helfer, einen winzigen `fmtThousandsDE(n int) string` (Punkt als Tausendertrennzeichen) im selben File anlegen (unit-getestet).
- **`nodeCockpitData`** (nach dem Bindings-Block, owner-scoped, guarded):
```go
// Kontext-Instrument (L5): the composed agent-context budget for THIS node's
// chain. ExecuteForNode uses the node ID directly (slugs are only sibling-
// unique). Guarded — an unwired/failed compose degrades to no panel (the page
// still renders). Owner-scoped (u.ID).
if s.ComposeContext.Nodes != nil {
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 12000
	}
	if cc, cerr := s.ComposeContext.ExecuteForNode(ctx, u.ID, n.ID, budget); cerr == nil {
		d.Context = webui.BuildCockpitContext(cc, n.ID)
	} else {
		slog.WarnContext(ctx, "cockpit: compose context failed", "nodeID", n.ID, "err", cerr)
	}
}
```
- **`CockpitRailBlocks`** — neuer `.blk` **zwischen** Kette (bzw. Contributors) und Bindings (Mockup-Reihenfolge Kette·Kontext·Bindings), nur wenn `d.Context != nil`:
```html
<div class="blk">
  <span class="eyebrow">{ T(ctx, "context.rail.title") }</span>
  <div class={ "meter", templ.KV("full", d.Context.Full) } role="img" aria-label={ fmt.Sprintf(T(ctx, "context.meterAria"), d.Context.Pct) }>
    <i style={ "width:" + strconv.Itoa(d.Context.Pct) + "%" }></i>
  </div>
  <div class="meter-l">
    <span>{ d.Context.UsedStr } tk</span>
    if d.Context.Full {
      <span class="warn">{ T(ctx,"context.budgetFull") } · { d.Context.CapStr }</span>
    } else {
      <span>{ d.Context.CapStr }</span>
    }
  </div>
  <div class="ctxrows">
    <div class="krow"><span class="n">{ T(ctx,"context.included") }</span><span class="v">{ fmt.Sprintf(T(ctx,"context.docsN"), d.Context.IncludedN) }</span></div>
    <div class="krow"><span class="n">{ T(ctx,"context.dropped") }</span><span class="v">{ strconv.Itoa(d.Context.DroppedN) }</span></div>
    <div class="krow"><span class="n">{ T(ctx,"context.pinned") }</span><span class="v">{ strconv.Itoa(d.Context.PinnedN) }</span></div>
  </div>
  for i, p := range d.Context.TopPins {
    <div class="pin" if i == len(d.Context.TopPins)-1 { style="border-bottom:none" }>
      <span class="g">{ p.Num }</span><span class="n" title={ p.Title }>{ p.Title }</span>
    </div>
  }
  <div class="mt-3">
    <a class="more" href={ templ.SafeURL("/kontext/" + d.Context.NodeID) }>{ T(ctx,"context.curate") }</a>
  </div>
</div>
```
  (Die `.pin`-`style="border-bottom:none"`-Zeile spiegelt Mockup Z.658 — erlaubte Struktur-Ausnahme wie im Kette-Block Bestand.)
- **`cockpit.templ`** — `#cockpit-rail` `hx-trigger` um `sse:document.created, sse:document.updated, sse:document.deleted` erweitern (Bestand: `session.* , node.updated, node.moved`), damit Pin/Reorder das Meter live nachziehen.
- i18n (beide Kataloge):
```go
"context.rail.title": "Kontext für Agenten",   // en: "Agent context"
"context.included":   "Enthalten",              // en: "Included"
"context.dropped":    "Verworfen (Budget)",     // en: "Dropped (budget)"
"context.pinned":     "Angepinnt",              // en: "Pinned"
"context.docsN":      "%d Docs",                // en: "%d docs"
"context.budgetFull": "fast voll",              // en: "nearly full"
"context.curate":     "Kuratieren — sortieren & pinnen ›", // en: "Curate — sort & pin ›"
"context.meterAria":  "Kontext-Budget zu %d Prozent belegt", // en: "Context budget %d percent used"
```

**Zustände dieser Fläche:** leer (Knoten ohne Kontext-Docs → Meter 0 %, „0 Docs"-Zeilen, keine Pins → keine `.pin`-Zeilen; Kuratieren-Link bleibt), voll (≥95 % → `meter full` warn + „fast voll"-Notiz), lang (Pin-Titel bricht via `.pin .n` Ellipsis + `title`), mobil 375px (Rail stackt unter die Bühne, Panel bleibt Panel — Bestand `.rail`-Responsive), laufender Timer (irrelevant — Panel ist statisch), Fehlerpfad (`ExecuteForNode`-Fehler → `d.Context == nil` → kein Panel, Cockpit rendert; Guard wie Bestand).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func nodeCockpitData|ComposeContext|ContextBudget|s.ComposeContext" internal/adapter/httpserver/webui_cockpit.go server.go`; `rg -n "templ CockpitRailBlocks|hx-trigger.*cockpit-rail|id=\"cockpit-rail\"" internal/adapter/webui/cockpit_rail.templ cockpit.templ`; `rg -n "type NodeCockpit struct" internal/adapter/webui/cockpit_vm.go`; `rg -n "func Fmt|Sprintf.*%d" internal/adapter/webui | rg -i "thousand|dur"`.
- [ ] **Step 1: Failing Builder-Test** — `cockpit_context_vm_test.go`: `BuildCockpitContext` liefert Pct/Full/IncludedN/DroppedN/PinnedN/TopPins korrekt aus einer handgebauten `ComposedContext` (inkl. Full ab 95 %, TopPins-Cap 3, Tausender-Format „11.891").
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Builder + `fmtThousandsDE` + NodeCockpit-Feld** implementieren.
- [ ] **Step 4: `nodeCockpitData`-Compose + `CockpitRailBlocks`-`.blk` + `cockpit.templ`-Trigger** + i18n (beide Kataloge).
- [ ] **Step 5: Handler-/Render-Test** — `webui_cockpit_test.go`: Cockpit eines Knotens mit Kontext-Docs zeigt `meter` + Kuratieren-Link (`/kontext/{id}`); **Owner-Scope-Negativtest** (User A's Cockpit zeigt keine Docs von User B im Meter-Zähler); Cockpit ohne Kontext-Docs rendert ohne Panel-Crash.
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Cockpit-Kontext-Instrument — Budget-Meter/Enthalten/Verworfen/Pins + Kuratieren-Link (ExecuteForNode, node-scoped, SSE document.*)"
```
Expected: PASS; Cockpit-Render-Test grün.

---

### Task 6: Dokument-docrail „Im Agenten-Kontext"-Block (enthalten ✓ · Rang N/M)

**Files:**
- Create: `internal/adapter/webui/doc_context_vm.go` + `_test.go` (reiner Builder `BuildDocContext`)
- Modify: `internal/adapter/webui/document_vm.go` (`DocumentVM` += `Context *DocContextVM`)
- Modify: `internal/adapter/webui/document.templ` (`.blk` in der docrail, nach „Verweise")
- Modify: `internal/adapter/httpserver/webui_document.go` (`buildDocumentVM`: Compose für den Doc-Knoten → `StandingOf` → `vm.Context`)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/document_layout_test.go` (Bestand), `internal/adapter/httpserver/webui_document_test.go`

**Interfaces:**
- **`webui.DocContextVM{ State string; NodeName string; RankStr string; Included bool }`** + **`func BuildDocContext(st usecase.ContextStanding, nodeName string) *DocContextVM`** (rein): `State` durchreichen; `RankStr = fmt.Sprintf("%02d / %02d", st.Rank, st.Total)` nur bei `included`; `Included = st.State=="included"`; `NodeName` = Scope-Name (aus `st.ScopeLabel` = `kind:name` → Name-Teil, oder der übergebene Knotenname). `absent` → Builder gibt `nil` (kein Block).
- **`isContextType(t domain.DocumentType) bool`** (in webui_document.go oder doc_context_vm.go): `t ∈ {DocMemory, DocInstruction, DocActiveContext}` — nur diese nehmen an Compose teil; für alles andere kein Block (spart den Compose-Aufruf komplett).
- **`buildDocumentVM`** (nach den Backlinks, owner-scoped, guarded):
```go
// Kontext-Rang (L5): only context-eligible docs participate in Compose. Compose
// the OWNING node's context by ID (nil node → global/unresolved context via
// Execute). Guarded/owner-scoped; a compose error just omits the block.
if isContextType(doc.Type) && s.ComposeContext.Nodes != nil {
	budget := s.ContextBudget
	if budget <= 0 {
		budget = 12000
	}
	var cc usecase.ComposedContext
	var cerr error
	if doc.NodeID != nil {
		cc, cerr = s.ComposeContext.ExecuteForNode(r.Context(), ownerID, *doc.NodeID, budget)
	} else {
		cc, cerr = s.ComposeContext.Execute(r.Context(), ownerID, usecase.ContextResolveInput{}, budget)
	}
	if cerr == nil {
		nodeName := ""
		if len(vm.Crumbs) > 0 {
			nodeName = vm.Crumbs[len(vm.Crumbs)-1].Label // leaf crumb = doc's node
		}
		vm.Context = webui.BuildDocContext(usecase.StandingOf(cc, doc.ID), nodeName)
	}
}
```
- **`DocumentFragment`** — neuer `.blk` in der docrail (nach „Verweise"), nur wenn `vm.Context != nil`:
```html
<div class="blk">
  <span class="eyebrow">{ T(ctx,"document.context.title") }</span>
  if vm.Context.State == "included" {
    <div class="krow"><span class="n">{ vm.Context.NodeName } · { T(ctx,"document.context.in") }</span><span class="v" style="color:rgb(var(--live))">✓</span></div>
    <div class="krow" style="border-bottom:none"><span class="n">{ T(ctx,"document.context.rank") }</span><span class="v">{ vm.Context.RankStr }</span></div>
  } else if vm.Context.State == "dropped" {
    <div class="krow" style="border-bottom:none"><span class="n">{ T(ctx,"document.context.dropped") }</span><span class="v">✗</span></div>
  } else if vm.Context.State == "always" {
    <div class="krow" style="border-bottom:none"><span class="n">{ T(ctx,"document.context.always") }</span><span class="v" style="color:rgb(var(--live))">✓</span></div>
  }
</div>
```
  **Der Anpinnen-Button bleibt UNANGETASTET** (Bestand, Provenance-Zeile, L3) — hier NICHT neu bauen.
- i18n (beide Kataloge):
```go
"document.context.title":   "Im Agenten-Kontext",   // en: "In agent context"
"document.context.in":      "enthalten",             // en: "included"
"document.context.rank":    "Rang",                  // en: "Rank"
"document.context.dropped": "verworfen (Budget)",    // en: "dropped (budget)"
"document.context.always":  "immer enthalten",       // en: "always included"
```

**Zustände dieser Fläche:** included (`{node} · enthalten ✓` + „Rang 04 / 24"), dropped (`verworfen ✗`), always (Instruction/ActiveContext → „immer enthalten ✓"), absent (Nicht-Kontext-Typ ODER Doc nicht in der Kette → **kein Block**, docrail rendert nur ToC+Verweise), lang (langer Knotenname bricht via `.krow .n` Ellipsis + kein Overflow), mobil (docrail stackt — Bestand), Fehler (Compose-Fehler → kein Block).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func .*buildDocumentVM|DocumentVM|vm.Crumbs|s.ComposeContext|ContextBudget" internal/adapter/httpserver/webui_document.go`; `rg -n "templ DocumentFragment|docrail|document.refs" internal/adapter/webui/document.templ`; `rg -n "type DocumentVM struct" internal/adapter/webui/document_vm.go`; `rg -n "DocMemory|DocInstruction|DocActiveContext" internal/domain/document.go`.
- [ ] **Step 1: Failing Builder-Test** — `doc_context_vm_test.go`: `BuildDocContext` liefert RankStr „04 / 24" bei included, nil bei absent, „always"/„dropped"-States korrekt.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Builder + `isContextType` + DocumentVM-Feld.**
- [ ] **Step 4: `buildDocumentVM`-Compose + docrail-`.blk`** + i18n (beide Kataloge).
- [ ] **Step 5: Handler-/Render-Test** — ein Memory-Doc in einer Kette zeigt „enthalten ✓ · Rang N/M"; ein Nicht-Kontext-Doc (z. B. project) zeigt **keinen** Block; **Owner-Scope** (das Compose sieht nur eigene Docs); der Anpinnen-Button (Bestand) unverändert vorhanden.
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Dokument-Kontext-Rang in der docrail (enthalten ✓ · Rang N/M; nur Kontext-Typen; Anpinnen bleibt Bestand)"
```
Expected: PASS.

---

### Task 7: Kuratieren-Seite `/kontext/{nodeID}` + Web-Reorder/Pin-Handler

**Files:**
- Create: `internal/adapter/webui/kontext.templ` + `internal/adapter/webui/kontext_vm.go` + `internal/adapter/webui/kontext_vm_test.go`
- Create: `internal/adapter/httpserver/webui_kontext.go` (`handleWebKontextView`, `handleWebKontextReorder`, `handleWebKontextPin`, gemeinsamer `kontextDataFor`/`renderKontext`)
- Modify: `internal/adapter/httpserver/server.go` (drei Routen)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/httpserver/webui_kontext_test.go` (Render + Reorder-Roundtrip + Pin-Toggle + Owner-Scope-404 + SSE-Emit)

**Layout (Offene Entscheidung #1 = ENTSCHIEDEN „schlichte Reorder-Liste", kein Mockup):** `.narrow`-Seite (860px), Lesesaal-Doktrin (Inhalt auf Papier, das **eine** Instrument = Meter auf `--panel`, Haarlinien-`.row`-Zeilen, **kein Drag-Drop**):
1. **pagehead:** `.eyebrow`=„Kontext kuratieren", `h1`=Knoten-Kurzname (`ShortName`), `.sub`=„{node} · {IncludedN} enthalten · {DroppedN} verworfen".
2. **Budget-Meter-Panel** (dasselbe `.meter`-Instrument wie im Cockpit, `role="img"`+aria).
3. **Rang-Liste** (`.sect`): pro Memory-`RankedItem` eine `.row`:
   - Rang-Badge (`.pin .g`-Stil / `.num`-Mono), `.typechip {DocTypeChipClass}` mit `{DocTypeLabel}`, `.grow`(`.t`=Titel · `.s`=Scope-Label + Tokens), rechts die **Aktionen** (alle `hx-target="#kontext-fragment" hx-swap="outerHTML"`): Anpinnen-Toggle (`.btn.btn-q.btn-s`, `hx-post="/kontext/{node}/pin"` mit `name="doc"`) + **Höher/Tiefer** (`.btn.btn-q.btn-s`, Glyphen `↑`/`↓`, `hx-post="/kontext/{node}/reorder"` mit `doc`+`dir`, an den Enden `disabled`). **A11y (Codex-Fund #5, Spec §13): jeder Icon-Button trägt `aria-label` + `title` aus den i18n-Keys** (`context.moveUp`/`context.moveDown`/`document.pin`|`document.pinned`) — Glyph-only-Buttons ohne Label sind ein §13-Verstoß.
   - Eine **`.cutline`** („Budget voll — ab hier verworfen") genau dort, wo `Included` von true auf false kippt; die verworfenen Zeilen darunter gedimmt (`.dim`/`opacity`).
4. **Leerzustand:** keine Memories → ruhige Zeile „Kein kuratierbarer Kontext für diesen Knoten."

**Interfaces:**
- **`kontextDataFor(r, u, nodeID) (webui.KontextVM, error)`**: `n, err := s.GetNode.Execute(ctx, u.ID, nodeID)` (404 bei `ErrNodeNotFound` — **Owner-Scope**: fremder/unbekannter Knoten = 404); `budget := s.ContextBudget|12000`; `cc, err := s.ComposeContext.ExecuteForNode(ctx, u.ID, n.ID, budget)`; `webui.BuildKontextVM(n, cc)`.
- **`webui.KontextVM`** + **`BuildKontextVM(n domain.Node, cc usecase.ComposedContext) KontextVM`** (rein): `NodeID`, `Title`(ShortName), `Sub`, das Meter (Pct/Full/UsedStr/CapStr wie Cockpit — **`fmtThousandsDE` aus T5 wiederverwenden**, nicht duplizieren), und `Rows []KontextRowVM` aus `cc.Ranked` (alle Pool-Items in Rang-Reihenfolge). `KontextRowVM{ DocID, Num, Title, ChipClass, TypeLabel, ScopeLabel, TokensStr, Pinned, Included, FirstDropped bool }` (`FirstDropped` markiert die `.cutline`-Position; `IsFirst/IsLast` für die Move-Button-Disable-Zustände).
- **`handleWebKontextReorder`** (Fehlerpfade explizit — Codex-Fund #4): `_ = r.ParseForm()`; `docID := r.FormValue("doc")`, `dir := r.FormValue("dir")`; `cc, err := s.ComposeContext.ExecuteForNode(ctx, u.ID, nodeID, budget)` — **Compose-Fehler NICHT verschlucken**: `err != nil` → `renderKontext` mit Fehlerzeile (bzw. 404 bei `ErrNodeNotFound`). `ids := [it.Item.ID for it in cc.Ranked]`; Index `k` von `docID`; **`k < 0` (Doc nebenläufig gelöscht / nicht mehr im Kontext) → No-op-Re-Render** (kein 500, kein Panic); sonst bei `up`&`k>0` swap `ids[k],ids[k-1]`, bei `down`&`k<len-1` swap `ids[k],ids[k+1]`; `s.ReorderContextDocs.Execute(ctx, u.ID, ids)` (dessen `ErrDocumentNotFound` — Doc zwischen Compose und Write gelöscht — sauber abfangen → No-op-Re-Render); `s.Emitter.Emit(EventDocumentUpdated{reordered})`; `renderKontext` (nur `#kontext-fragment`).
- **`handleWebKontextPin`**: `doc := r.FormValue("doc")`; `d, _ := s.GetDocument.Execute(ctx, u.ID, doc)`; `s.SetPinned.Execute(ctx, u.ID, doc, !d.Pinned)`; `s.Emitter.Emit(EventDocumentUpdated{id})`; `renderKontext`. (Reuse der Bestand-`SetPinned`; das Kuratieren-Fragment ist das Ziel, nicht `#document-fragment`.)
- **Routen** (server.go, `webAuth`, bei den `/wissen`-Routen):
```go
mux.Handle("GET /kontext/{id}", s.webAuth(http.HandlerFunc(s.handleWebKontextView)))
mux.Handle("POST /kontext/{id}/reorder", s.webAuth(http.HandlerFunc(s.handleWebKontextReorder)))
mux.Handle("POST /kontext/{id}/pin", s.webAuth(http.HandlerFunc(s.handleWebKontextPin)))
```
- **SSE — fragment-sicher (Codex-Fund #3):** `GET /kontext/{id}` ist eine **Full-Page-Route** (liefert AppShell). Ein `hx-swap="innerHTML"` auf `hx-get="/kontext/{id}"` würde die **volle AppShell** in `#content` verschachteln. Deshalb **exakt das Bestand-Dokument-Pattern** (`document.templ:22-33`) spiegeln: ein `kontextOuter` wickelt das `#kontext-fragment` in `<div id="content" hx-get="/kontext/{id}" hx-trigger="sse:document.updated, sse:document.created, sse:document.deleted" hx-select="#kontext-fragment" hx-target="#kontext-fragment" hx-swap="outerHTML">`. Der SSE-Reload extrahiert per `hx-select` **nur** das Fragment aus der zurückgegebenen Vollseite — keine AppShell-Verschachtelung. Die Reorder-/Pin-Handler geben ihrerseits **nur** das `#kontext-fragment` zurück (nicht die Vollseite), Ziel `#kontext-fragment`. Das Seiten-Gerüst nutzt `components.AppShell("projekte", …)` (Kuratieren gehört zum Projekt-Kontext) — den aktiven Bereich per rg am Bestand prüfen (`rg -n "AppShell\(\"" internal/adapter/webui/document.templ`).
- i18n (beide Kataloge):
```go
"context.curate.eyebrow": "Kontext kuratieren",        // en: "Curate context"
"context.curate.sub":     "%s · %d enthalten · %d verworfen", // en: "%s · %d included · %d dropped"
"context.moveUp":         "Höher",                      // en: "Move up"
"context.moveDown":       "Tiefer",                     // en: "Move down"
"context.cutline":        "Budget voll — ab hier verworfen", // en: "Budget full — dropped below"
"context.estTokens":      "%s tk",                      // en: "%s tk"
"context.curate.empty":   "Kein kuratierbarer Kontext für diesen Knoten.", // en: "No curatable context for this node."
```
  (`context.pin`/`context.pinned` — prüfen ob `document.pin`/`document.pinned` (Bestand) wiederverwendbar; sonst eigene Keys.)

**Zustände dieser Fläche:** leer (keine Memories → ruhige Zeile), lang (86-Zeichen-Doc-Titel bricht via `.row .t`/`title`; viele Zeilen → Seite scrollt vertikal), mobil 375px (`.narrow` volle Breite; Aktions-Buttons bleiben tappbar; `.row .right .k` weg <620px Bestand; **kein horizontales Pannen**), laufender Timer (irrelevant), Fehlerpfad (Knoten 404; Compose-Fehler → Fehlerzeile; Reorder eines gelöschten Docs → `ErrDocumentNotFound` sauber abgefangen).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func .*GetNode|func .*SetPinned|func .*ReorderContextDocs|ComposeContext|ContextBudget|ErrNodeNotFound" internal/adapter/httpserver/server.go`; `rg -n "AppShell\(\"|components.AppShell|webAuth|mux.Handle\(\"GET /wissen" internal/adapter/httpserver/server.go internal/adapter/webui`; `rg -n "ShortName|DocTypeChipClass|DocTypeLabel|\.narrow|\.pagehead|\.sect|\.row\b|\.typechip" internal/adapter/webui -g '!*_templ.go'`; `rg -n "fmtThousandsDE|BuildCockpitContext" internal/adapter/webui` (T5-Helfer wiederverwenden).
- [ ] **Step 1: Failing Builder-Test** — `kontext_vm_test.go`: `BuildKontextVM` liefert Rows in Rang-Reihenfolge, markiert `FirstDropped` an der Budget-Grenze, `IsFirst/IsLast` an den Enden, Meter korrekt.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: VM-Builder + templ (`kontext.templ`) + Handler (`webui_kontext.go`) + Routen.**
- [ ] **Step 4: Handler-Test** — `webui_kontext_test.go`: `GET /kontext/{id}` rendert Meter + Zeilen + Kuratieren-Aktionen **mit `aria-label`** an den ↑/↓/Pin-Buttons (Codex-Fund #5) + das `#kontext-fragment` (Codex-Fund #3, für den SSE-`hx-select`); **Reorder** (`POST …/reorder doc=X dir=up`) ändert die Prioritäten (Fake-Store-Assertion) + emittiert `document.updated` + gibt nur das Fragment zurück (keine `<html>`/AppShell); **Reorder eines nicht mehr im Kontext befindlichen/gelöschten Docs** (Codex-Fund #4: `doc=<unbekannt>`) → sauberer No-op-Re-Render, kein 500; **Pin** togglet + emittiert; **Owner-Scope-404** (`/kontext/{fremder-id}` → 404, kein Fremd-Doc sichtbar); leerer Knoten rendert Leerzustand.
- [ ] **Step 5: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Kuratieren-Seite /kontext/{id} — Budget-Meter + Rang-Liste mit Höher/Tiefer + Anpinnen (owner-scoped, SSE document.updated)"
```
Expected: PASS.

---

### Task 8: Wiring-Gate (Composition-Root · Sweep · make ci · Live-Smoke · Breakpoints)

**Files:** i. d. R. keine neuen (Verifikation + evtl. Sweep-Fixes).

- [ ] **Step 1: Composition-Root-Verifikation** (`cmd/flow-server/main.go` + `server.go`)
```bash
rg -n "ReorderContextDocs|ComposeContext|SetPinned|ContextBudget|ListDocuments|GetDocument|GetNode|NodeAncestors" cmd/flow-server/main.go internal/adapter/httpserver/server.go
rg -n "mux.Handle\(\"POST /api/v1/context/reorder|mux.Handle\(\"GET /kontext|mux.Handle\(\"POST /kontext" internal/adapter/httpserver/server.go
```
Erwartet: `ReorderContextDocs` im Server-Literal verdrahtet; die drei `/kontext`-Routen + die Reorder-API registriert; `ComposeContext`/`SetPinned`/`ContextBudget` (Bestand) unverändert wired. **Kein weiterer main.go-Change** — T5/T6/T7 nutzen ausschließlich Bestands-Server-Felder (`ComposeContext`, `SetPinned`, `GetNode`, `GetDocument`, `ContextBudget`, `NodeAncestors`).
- [ ] **Step 2: Rest-Sweep** — `gemini-bigcontext` (agy) / Fallback rg über `git diff --name-only rebuild..HEAD`; Dispatch-Text oben. Gefundene tote Keys/Arbitrary-Values/verwaiste Symbole fixen.
- [ ] **Step 3: Tote i18n-Keys** — die neuen `context.*`/`document.context.*`-Keys gegen `T(`/`Tn(`-Nutzung prüfen (`rg -n "\"context\\.|\"document.context\\." internal --glob '!catalog_*.go'`); keine verwaisten; de+en-Parität.
- [ ] **Step 4: Volles CI**
```bash
git add -A
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST=Podman-Socket
```
(erst stagen, dann ci — L4-Lehre; nie zwei ci parallel.)
- [ ] **Step 5: Live-Smoke** (Dev-Stack; Cookie-Flow wie L1–L4-Gate)
```bash
make dev-run &   # https://localhost:8080 (self-signed); danach stoppen
sleep 2
# Migration 0029 angewandt?
# GET /nodes/{id}         → Kontext-Instrument (Meter + Enthalten/Verworfen/Angepinnt + Top-Pins + "Kuratieren ›")
# GET /kontext/{id}       → Meter + Rang-Liste + Höher/Tiefer + Anpinnen + cutline
# POST /api/v1/context/reorder {"ids":[...]} (Bearer)  → 200; danach GET /context zeigt neue Reihenfolge
# GET /wissen/{memory-id} → docrail "Im Agenten-Kontext · enthalten ✓ · Rang N/M"
```
Expected: `/nodes/{id}` zeigt das Meter (Bernstein ab 95 %, Zähler decken sich mit `GET /context`); „Kuratieren ›" → `/kontext/{id}`; Höher/Tiefer verschiebt einen Doc → Cockpit-Meter **und** Dokument-Rang ziehen per SSE (`document.updated`) nach; Anpinnen (Bestand) togglet → Pin erscheint in den Cockpit-Top-Pins; ein Memory-Doc zeigt „Rang N/M"; ein Nicht-Kontext-Doc zeigt **keinen** Kontext-Block. Danach Server stoppen.
- [ ] **Step 6: Breakpoint-Sichtprobe für Soenne notieren** — **≤960px** (Cockpit-Rail stackt unter die Bühne, Meter-Panel bleibt Panel; Kuratieren `.narrow` volle Breite) und **375px** (kein horizontales Pannen; Kuratieren-Aktions-Buttons tappbar; Meter/Pins lesbar; `.krow .n`/`.pin .n` Ellipsis greift).
- [ ] **Step 7: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L5-Gate — Composition-Root-Verify + Sweep + Live-Smoke (Kontext-Kuratierung)"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Alternativpfade. Entscheidung am Ausführungsstart.

1. **Layout der Kuratieren-Fläche (Mockup-Lücke — der Link geht auf `#`).** — *Empfehlung: eine `.narrow`-Seite mit Budget-Meter-Panel oben + einer einzigen Haarlinien-Rang-Liste (`.row`-Zeilen) in Compose-Reihenfolge; pro Zeile Anpinnen-Toggle + Höher/Tiefer-Buttons; eine `.cutline` markiert die Budget-Grenze, verworfene Zeilen darunter gedimmt.* Lesesaal-Doktrin: Inhalt auf Papier, **ein** Instrument (Meter) auf `--panel`, keine Karten-in-Karten, **kein Drag-Drop** (Spec-Vorgabe „schlichte Reorder-Mechanik"). Trade-off: Höher/Tiefer ist bei sehr langen Listen mühsam — akzeptabel, weil Kuration selten und die Liste durch die Kette begrenzt ist. **Reconcile mit dem Hintergrund-Spec (Gemini-Fund #9):** `2026-06-27-flow-kontext-redesign-design.md` nennt den Kontext-Inspektor „gruppiert nach Ebene/Typ". Die **flache** Rang-Liste erfüllt das **weich**: der Rang kodiert bereits die Tier-Ordnung (`pinned > priority > tierRank`), sodass Zeilen bei gleicher Priorität nach Ebene clustern, und **jede Zeile trägt ihr Scope-Label** (`kind:name` aus `ScopeLabel`) + `.typechip` — Ebene und Typ sind also pro Zeile sichtbar. Eine **harte** Sektions-Gruppierung würde die **eine** Gesamtordnung zerschneiden, die der Höher/Tiefer-Reorder braucht (man kann nicht über eine Sektionsgrenze reordern). Empfehlung: flach + Scope-Label je Zeile (weiche Gruppierung); optionale visuelle Ebenen-Trenner sind ein L7-Politur-Nice-to-have. **Alternativen:** (a) Zwei-Spalten „enthalten | verworfen" (mehr Chrome, gegen die Ein-Meter-Ruhe); (b) absolute Prioritäts-Zahl pro Zeile als Eingabefeld (präziser, aber fummelig/hässlich); (c) Drag-Drop via vendored JS (CSP-/Mobil-/Doktrin-Konflikt — abgelehnt); (d) harte Sektions-Gruppierung nach Ebene (erfüllt den Spec-Wortlaut streng, bricht aber die Reorder-Gesamtordnung — abgelehnt).
2. **Wo sitzt Priorität im cap+rank-Schlüssel?** — **ENTSCHIEDEN „A" (Vorgabe-Block):** `(pinned, priority, tierRank, updatedAt)` — Priorität nach Pin (Garantie bleibt Pin), vor Tier (Priorität darf über Tiers heben). Default 0 = verhaltensneutral. **Alternative B** (Priorität nur *innerhalb* eines Tiers: `pinned, tierRank, priority, updatedAt`) ist konservativer (Tier-Semantik strikt), nimmt dem Kurator aber die Kraft, ein Leaf-Memory über ein Engagement-Memory zu heben — das ist aber genau der Sinn von „sortieren". Abgelehnt, im Plan dokumentiert.
3. **Bumpt `SetPriority` `updated_at`?** — *Empfehlung: NEIN.* Ein Reorder-Batch schreibt viele Docs; würde jedes `updated_at` neu stempeln, zerschösse das die `updatedAt desc`-Tiebreaker-Reihenfolge (und die „aktualisiert"-Provenance). Priorität ist orthogonal zur Aktualität. Trade-off: Abweichung vom Bestand-`SetPinned` (das `updated_at=now()` setzt). **Alternative:** mitstempeln (konsistent mit SetPinned), dafür Recency-Scramble beim Reorder — abgelehnt.
4. **SSE-Event für Kuratierungs-Mutationen.** — *Empfehlung: `document.updated` wiederverwenden* (der Pin-Handler tut das schon; die Rail bekommt `document.*` in ihre Trigger). Minimal, kein neuer Event-Typ. Trade-off: die Rail lädt jetzt auch bei jeder fremden Doc-Änderung neu — harmlos (Kette/Bindings stabil, `#cockpit-main` tut das längst). **Alternative:** neuer `context.changed`-Event (sauberere Semantik, aber neuer Typ + alle Kurations-Handler + alle Konsumenten müssten ihn führen — mehr Fläche). Abgelehnt.
5. **Cockpit-„Angepinnt"-Zahl: global oder kontext-scoped?** — *Empfehlung: kontext-scoped* (Pins in DIESER Kette, aus `cc.Ranked`). Das Panel ist node-scoped; eine globale „12" (Mockup-Zahl aus dem Korpus) neben node-scoped Enthalten/Verworfen wäre inkonsistent. Trade-off: die Zahl weicht vom Mockup-Wert ab (dort war „12" die globale Pin-Zahl). **Alternative:** globale Pin-Zahl per Extra-Query (`ListArchived`-Muster invers) — ein zusätzlicher owner-scoped Scan pro Rail-Render, für eine Zahl, die neben den anderen (node-scoped) semantisch bricht. Abgelehnt.
6. **Kuratieren-Reichweite: node-scoped Seite, dokument-globale Wirkung.** — *Empfehlung: die Seite `/kontext/{nodeID}` zeigt die Memories DIESER Kette, aber Priorität ist eine Eigenschaft des **Dokuments** — ein Reorder wirkt auf das Doc in JEDEM Kontext, in dem es erscheint.* Das entspricht der Kurator-Mentalität („ich arbeite in backstage, ich kuratiere, was backstage's Agent sieht") und ist das einfachste Modell (kein (Doc,Node)-Prioritäts-Paar, keine neue Tabelle). Trade-off: eine Umsortierung in Kette A verschiebt das Doc auch in Kette B (wenn es dort vorkommt). **Alternative:** per-(Doc,Node)-Priorität (Join-Tabelle) — deutlich mehr Schema/Code, nicht L5-nötig. Abgelehnt.
7. **Reorder als eigene REST-API (`/api/v1/context/reorder`) oder nur Web?** — *Empfehlung: eigene REST-API* (Task 3), damit die Reorder-Fähigkeit auch MCP/CLI offensteht (Memory „generische Features in alle Hosts"; die Pin-API existiert schon owner-scoped). Trade-off: ein Handler + Test mehr. **Alternative:** nur der Web-Handler in Task 7 — spart Task 3, lässt aber Agenten (die den Kontext ja konsumieren) ohne Reorder-Weg. Abgelehnt.
8. **Kontext-Instrument auf allen Knoten-Kinds oder nur Repo/Leaf?** — *Empfehlung: alle Kinds* (`ExecuteForNode` liefert für jeden Knoten eine gültige Ketten-Compose). Konsistente Rail, keine Sonderfälle. Trade-off: für ein Engagement zeigt das Meter dessen (kurze) Kette — semantisch „der Kontext, den ein Agent an diesem Knoten bekäme", was am Leaf am schärfsten ist. **Alternative:** nur auf Repo-Knoten (schärfere Semantik, inkonsistente Rail auf Engagement/Vorhaben). Abgelehnt.
9. **Compose-Kosten pro Rail-Render.** — *Hinweis (keine Blocker-Entscheidung): `nodeCockpitData` ist single-pass (L2-Flatten) und läuft für head/main/rail; die neue `ExecuteForNode`-Compose (Get + Ancestors + ListForContext + Tag-Gate, ~3–4 Queries) läuft damit auf jedem Fragment-Reload mit.* Für ein persönliches Tool akzeptabel. **Falls im Live-Gate spürbar:** die Compose nur berechnen, wenn die Rail gerendert wird (nicht in head/main), oder einen kurzen per-request-Memo. Deferred bis Messung (nicht spekulativ optimieren).

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads (kanonisch):** Spec §9/§10/§16.5/§17 + Mockup Kontext-CSS Z.171–177, Kontext-Panel Z.647–660, Dokument-Block Z.794–798, Anpinnen Z.694; L4-Formatvorbild + L4-Ledger vollständig; und der echte Code: `compose_context.go` (ALLE Typen + exakter Sort `pinned>tier>updatedAt`, `Execute/resolveLeaf/globalAllowed/itemOf/estTokens/bootstrapTypes`), `domain/document.go` (Pinned/Archived/Priority-Zielstelle, Kontext-Typen), `domain/node.go` (Slug **geschwister-eindeutig** → ID-Compose), `domain/event.go` (alle EventTypes wörtlich), `ports.go` (DocumentStore inkl. SetPinned/ListForContext, NodeStore Get/Ancestors), `set_pinned.go`, `pgstore/documents.go` (`docCols`/`prefixedDocCols` + **drei** Scanner :392/:506/:548 + Create/Update-RETURNING + UpsertByPath eigene Liste + SetPinned-Muster), Migrations (höchste = 0028), `context.go` (handleGet/PutContext, Budget-Default 12000), `documents.go` (handlePinDocument emittiert document.updated), `webui_document.go` (buildDocumentVM alle Felder, handleWebDocPin emittiert + rendert #document-fragment), `webui_cockpit.go` (nodeCockpitData single-pass, vier Fragment-Handler, Ketten-Montage :44–47), `cockpit_vm.go` (NodeCockpit), `document_vm.go` (DocumentVM), `cockpit_rail.templ` (blk/krow + „Kontext … L5"-Kommentar), `document.templ` (Provenance-Anpinnen Bestand + „Im Agenten-Kontext … L5"-Kommentar), `cockpit.templ` (die drei Fragment-hx-trigger-Listen — Rail hat KEIN document.*), `server.go` (Routen + Server-Felder), `main.go` (ComposeContext/SetPinned/ContextBudget wired), `tailwind.css` (**`.meter`/`.meter-l`/`.ctxrows`/`.pin` existieren :451–457** → CSS-Task schrumpft), i18n-Katalogstruktur (`cockpit.rail.*`/`document.pin*`).
- **Sekundär: agy-Dossier** (gemini-bigcontext) über die L5-Flächen asynchron dispatcht (Auftrag: wörtliche Extraktion Compose-Typen/Sort, Scanner-Topologie, VM-Structs, Rail-/docrail-Markup, i18n-Bestand, tailwind-Klassen-Präsenz, Mockup-CSS). **Degradations-Notiz:** wie in L4 kann das agy-Dossier am Session-Limit sterben; das Grounding ist deshalb **first-hand kanonisch** (jede im Plan verwendete Signatur direkt am Code verifiziert — u. a. `.meter`-Bestand, 3-Scanner-Topologie, Slug-Geschwister-Eindeutigkeit, Ancestors-Ketten-Montage, `document.updated`-Emit-Bestand). Kein Abbruch.
- **Flow-Recall:** `flow_search_docs` (project-scope, type plan) für „Lesesaal L5 Kontext-Kuratierung" — kein L5-Plan remote (ich bin der erste); der cap+rank-Plan bestätigt den exakten Sort-Schlüssel + Cap 12000; der L4-Plan nennt „Next: L5 Kontext-Kuratierung". Lokale Dateien kanonisch.

### Spec-Deckung L5 (§17-Scope) — jeder Spec-Absatz auf einen Task gemappt
- §10 **Kontext-Instrument** (Budget-Meter `11.891/12.000` Bernstein ab 95 %, Enthalten/Verworfen/Pins-Zeilen, nummerierte Top-Pins, „Kuratieren ›") → T4 (Meter-Zustände) + T5 (Panel + Compose + SSE). **Am Dokument** „enthalten ✓ · Rang 04/24" → T6. **„Prioritäts-/Reorder-API neu"** → T1 (Feld) + T2 (Rang) + T3 (API).
- §9 **Cockpit-Meta = Kette · Kontext-Instrument · Bindings** → T5 (Panel zwischen Kette und Bindings, Mockup-Reihenfolge). **Dokument-Meta enthält Kontext-Rang** → T6. **Provenance-„Anpinnen"** → **Bestand (L3), unverändert** (in T6 explizit als Nicht-Ziel markiert).
- §16.5 **„Prioritäts-Feld + Reorder/Pin-API + Kuratieren-UI (neu; cap+rank/Pins existieren) — eigener Slice"** → Feld T1, Rang T2, Reorder-API T3, Kuratieren-UI T7 (Pin-API = Bestand B3, in T7 wiederverwendet).
- §11 **Eindämmung** (Kuratieren-Liste pannt nicht horizontal; `.pin .n`/`.krow .n` Ellipsis; lange Doc-Titel brechen) → T4/T5/T7 + Gate 375px.
- §13 **A11y/i18n**: Meter `role="img"`+aria-label (Mockup Z.649) → T4-Styleguide + T5/T7-Markup; i18n de/en Parität → jeder Key-Step.
- §17 **L5-Definition** (Kontext-Kuratierung) → alle Tasks. **NICHT in L5 (bewusst, Spec §17):** Artefakte (L6), Dunkel-Zwilling (L7).

### Carry-forwards / Deferred — Verbleib
1. **L4-Deferred (L7-Rollup):** Kolophon, Micro-Type-Sammelklasse, AllTime-Σ-Scan, @font-face-URLs unversioniert, readTimeLabel-Duplikat, Projekte-Summary — **alle weiter deferred** (nicht L5-Scope; keine berührt Kontext).
2. **Compose-Kosten pro Rail-Render** (OE #9) → bewusst deferred bis Live-Gate-Messung (nicht spekulativ optimieren).
3. **per-(Doc,Node)-Priorität** (OE #6-Alternative) → bewusst nicht gebaut (dokument-globale Priorität genügt; Join-Tabelle wäre Über-Engineering).
4. **Globaler Kontext ohne Knoten** (`/kontext` ohne ID, für unresolved/global-only) → bewusst deferred; L5 ist cockpit-getrieben node-scoped. Der Dokument-Block deckt den `doc.NodeID == nil`-Fall via `Execute` (global) bereits ab.

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Absatz ohne Task:** keiner im L5-Scope (Mapping oben); L6/L7-Absätze bewusst außerhalb.
- **(b) Zustände je Task:** leer/lang/mobil-375/laufender-Timer(n. a.)/Fehler in T4–T7 explizit benannt; T1–T3 sind Backend (Zustände = Test-Fälle: default-0, cross-owner, dropped, empty-chain); T8 ist der Gate.
- **(c) Querschnitte:** main.go-Wiring → T3 (ReorderContextDocs) + T8-Verify (T5/T6/T7 nutzen nur Bestands-Server-Felder); SSE je Mutation → `document.updated` (T3 API, T7 Web-Reorder/Web-Pin) + Konsumenten benannt (Rail-Trigger += document.* in T5; Kuratieren-`#content` in T7); i18n beide Kataloge → jeder Key-Step; Responsive → T4 + Gate 960/375; Owner-Scoping → Negativtests in T1 (SetPriority cross-owner), T3 (Reorder cross-owner), T5 (Meter-Zähler), T6 (Compose), T7 (`/kontext/{fremd}`=404), `u.ID` in jedem Handler.
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; Step 0/Step 1 rg-Verifikation aller Bestandsnamen; „Bestand gewinnt". Spaltenlisten-Lehre (3 Scanner) + Interface-Ripple (alle Fakes) in T1 als Compiler-geführte Pflicht.

### Adversariale Lückensuche — Berater-Findings + Verbleib

Beide Berater liefen gegen Spec + Mockup + Plan-Entwurf + Dossier mit dem wörtlichen Lücken-Auftrag. **`agy`/Gemini 3.1 Pro** lief sauber durch (13 Findings, davon einer hart code-verifiziert). **`codex exec`** wurde parallel dispatcht.

**CRITICAL — von Gemini hart am Code verifiziert, EINGEARBEITET:**
1. **[eingearbeitet — Task 1 Step 5 + Files + Spaltenlisten-Constraint]** (Gemini #13) Der Plan sicherte die **drei Scanner** ab, übersah aber, dass **`Create` (documents.go:92–108)** `docCols` **auch** als INSERT-Spaltenliste nutzt — mit hartkodiertem `VALUES ($1..$17)` + 17 Args. `priority` an `docCols` anhängen → 18-Spalten-INSERT mit 17 Werten → **Laufzeit-SQL-Fehler**. Zusätzlich widersprach der Plan-Kommentar „Create/UpsertByPath rely on the DB default" der eigenen „am Ende anhängen"-Prämisse. → **Fix:** Task 1 ergänzt jetzt explizit `Create`s `VALUES ($18)` + `d.Priority`-Arg (verifiziert documents.go:92–108: `$1..$17` + 17 gebundene Args); der Domain-Kommentar präzisiert „Create binds it explicitly (zero-value 0); UpsertByPath omits it"; die Global-Constraint „Spaltenlisten-Lehre" nennt jetzt die **Arity-Kopplung** von Create (nicht nur die Scanner); ein `Create`-Roundtrip-Test (`Priority == 0`) sichert die Regression. `Update` (:174–189) ist verifiziert **nicht** betroffen (docCols nur im RETURNING, keine Arity-Kopplung).

**Sekundär legitim, EINGEARBEITET:**
2. **[eingearbeitet — Offene Entscheidung #1]** (Gemini #9) Der Hintergrund-Spec (`2026-06-27-flow-kontext-redesign-design.md`) fordert den Kontext-Inspektor „gruppiert nach Ebene/Typ"; die Kuratieren-Fläche des Plans ist bewusst flach (OE #1). → OE #1 reconciled das jetzt explizit: die flache Rang-Liste erfüllt „gruppiert" **weich** (Rang kodiert Tier-Ordnung; jede Zeile trägt Scope-Label + Typ-Chip), eine harte Sektions-Gruppierung würde die für den Höher/Tiefer-Reorder nötige Gesamtordnung zerschneiden (abgelehnte Alternative dokumentiert; harte Ebenen-Trenner = L7-Politur-Option).

**Offene Frage, als Deferred verbucht:**
3. **[begründet abgelehnt / deferred — Self-Review Carry-forward]** (Gemini #8) Ob die `flow context`-CLI-Ausgabe den neuen Rang/Priorität separat darstellen muss. → Die CLI rendert die von `Compose` gelieferte Reihenfolge; da `Compose` jetzt `priority` mitsortiert, **zieht die CLI die neue Reihenfolge implizit mit** (kein CLI-Change nötig für die Ordnung). Ein **expliziter** Rang/Priorität-Ausweis in der CLI ist ein optionales Nice-to-have (generisches Feature für den Agent-Host, Memory), **nicht L5-blockierend** — als L6/L7-Kandidat notiert. `Ranked` liegt additiv im `GET /context`-JSON bereit, falls die CLI es später zeigen will.

**Begründet abgelehnt — außerhalb L5-Scope (Slice-Historie L1–L4 gemerged):**
4. **[abgelehnt]** (Gemini #1–#7) Gemini verlangte pauschal jede §9-IA-Zeile (Schreibtisch/Projekte/Wissen/Zeit/Dokument-Provenance) + §10-Timer + §10-Provenance als eigenen L5-Task. §9 ist die IA-Tabelle der **gesamten** Lesesaal-Serie; Schreibtisch/Zeit sind **L4 DONE**, Projekte/Cockpit **L2 DONE**, Dokument-Provenance/Wissen **L3 DONE**, Timer-Pill **L1/L4 DONE**. Der Plan zitiert (Header) korrekt **nur** die L5-relevanten §9/§10-Teilsätze (Cockpit-Meta = Kette·Kontext·Bindings, Dokument-Meta = Kontext-Rang) — exakt der §16.5-Scope „Kontext-Kuratierung — eigener Slice". Gemini hat den L5-Scope nicht gegen die Slice-Historie abgeglichen (im Caveat selbst eingeräumt). Das **Anpinnen** (das einzige Provenance-relevante L5-Element) ist Bestand (L3), in Task 6 als Nicht-Ziel markiert. Kein Gap.

**Von Gemini explizit als sauber bestätigt (kein Plan-Change):** Zustände leer/lang/mobil/Fehler in T4–T7 + Gate benannt (Kat. b); main.go-Wiring (T3), SSE `document.updated` je Mutation + Rail-Trigger-Ergänzung, i18n beide Kataloge, Owner-Scope-Negativtests (T1/T3/T5/T6/T7) vorhanden (Kat. c); jeder Task hat Test-Step + Step-0-`rg`-Verifikation (Kat. d); der **Reorder-Swap-Algorithmus** operiert korrekt auf der **vollen** `cc.Ranked`-Liste (inkl. gedroppter Items), kein Bug.

**codex exec (synchron im Vordergrund, `--sandbox read-only`, `model_reasoning_effort=high`) — 5 Findings, ALLE eingearbeitet:**
5. **[eingearbeitet — Task 3 Files + Step 7]** (Codex #1) Spec §16.5 fordert „Reorder/**Pin**-API"; Task 3 baute nur Server-Handler/Route/Wiring, aber **keine `apiclient.ReorderContext`-Methode** (verifiziert apiclient/context.go:37-71 hat `ComposeContext`+`SetPinned`, keinen Reorder-Client). Das widersprach OE #7 („für CLI/MCP offen"). → apiclient-Methode + httptest-Client-Test in Task 3 ergänzt (CLI-Verb bleibt optional/L6).
6. **[eingearbeitet — Task 2 Step 1]** (Codex #2) Der Owner-Scope-Anspruch an `ExecuteForNode` (Global Constraint) war in Task 2 nur als „ownerID durchgereicht" getestet, nicht als echter **Fremd-Knoten → `ErrNodeNotFound`** (Port-Semantik `NodeStore.Get(ctx,owner,id)` owner-scoped, ports.go:86-89). → Zusatztest `TestComposeContext_ExecuteForNode_ForeignNode`.
7. **[eingearbeitet — Task 7 SSE-Bullet + Step 4]** (Codex #3, load-bearing) `GET /kontext/{id}` ist eine Full-Page-Route; `hx-swap="innerHTML"` auf `hx-get="/kontext/{id}"` hätte die **volle AppShell in `#content` verschachtelt**. → das Bestand-Dokument-Pattern (`document.templ:22-33`) gespiegelt: `kontextOuter` + `#kontext-fragment` + `hx-select="#kontext-fragment"` + `hx-target=#kontext-fragment` + `hx-swap="outerHTML"`; die Handler geben nur das Fragment zurück; Render-Test prüft `#kontext-fragment` + kein `<html>`.
8. **[eingearbeitet — Task 7 Handler-Bullet + Step 4]** (Codex #4) Der Reorder-Handler-Sketch verschluckte den Compose-Fehler (`cc, _ :=`) und deckte den „docID nicht in `cc.Ranked`" (nebenläufig gelöschtes Doc) nicht ab; Step 4 testete nur Happy-Path/Pin/Owner-404/leer. → Handler behandelt jetzt Compose-Fehler (Fehlerzeile/404) + `k<0`-No-op + `ReorderContextDocs`-`ErrDocumentNotFound`-No-op; Test „Reorder eines gelöschten Docs → sauberer No-op" ergänzt.
9. **[eingearbeitet — Task 7 Layout-Bullet + Step 4]** (Codex #5, Spec §13) Die ↑/↓-Glyph-Buttons hatten i18n-Keys, aber der Plan wies nicht an, sie als `aria-label`/`title` zu setzen. → jeder Icon-Button (↑/↓/Pin) trägt jetzt `aria-label`+`title` aus `context.moveUp`/`context.moveDown`/`document.pin`; Render-Test assertet das.

**Von codex explizit als sauber bestätigt (kein Plan-Change):** L5-relevantes §9/§10/§16.5-Grundmapping; `document.updated` als Mutationsevent; `#cockpit-rail`-Trigger-Ergänzung; i18n beide Kataloge; `main.go`-Wiring für `ReorderContextDocs`; die pgstore-Spaltenlisten/Scanner/**Create-Arity** (der Gemini-Critical war bei codex' Lauf bereits eingearbeitet); `cc.Ranked` über enthaltene **und** verworfene Items; apiclient-JSON-Decode durch additive Felder (`ContextItem.Priority`/`ComposedContext.Ranked`) **nicht** gebrochen.

**Dissens:** keiner — die Berater überschnitten sich nicht (Gemini fand die Create-Arity #13, codex fünf orthogonale UI-/API-/Test-Lücken); alle Sichten komplementär und eingearbeitet. **Netto aus der Lückensuche: 1 CRITICAL (Create-Arity) + 5 substanzielle Ergänzungen + 2 Spec-Reconciles (Kontext-Inspektor-Grouping, CLI-Rang) — alle verbucht.**
