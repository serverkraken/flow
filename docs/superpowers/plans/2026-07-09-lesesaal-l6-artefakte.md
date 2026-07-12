# Lesesaal L6 — Artefakte + Inline-Figuren + Deep-Links + Editor-Einfügehelfer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** L6 macht **Artefakte** (Bilder, PDFs, Datei-Anhänge) zu erstklassigen, node-gebundenen Bausteinen des Lesesaals. Ein Artefakt hängt an einem `node` (nicht am Dokument), ist über die **Ahnenkette** (Node + Vorfahren) in allen Dokumenten darunter referenzierbar und wird per `![[slug]]` als **durchnummerierte Figur** (Bild-`<figure>` oder Datei-Chip) inline gerendert — mit demselben Zähler wie Mermaid. Storage ist ein Postgres-`bytea`-Blob (Präzedenz `node_logos`, aber **N pro Node**). Menschen laden in der **Cockpit-Galerie** hoch; Agenten über **flow-mcp** + **CLI** (E3, generisches Feature für alle Hosts). Zwei angrenzende „Verlinkung"-Anliegen kommen mit: server-seitige **Überschriften-Deep-Links** (GitHub-Slug-Anker + Hover-¶) und ein **Editor-Einfügehelfer** (Fuzzy-Picker für Artefakt-Referenz und Seiten-Wikilink).

**Architecture:** Server-rendered wie gehabt (templ + htmx + Tailwind, kein SPA, kein Node-Runtime). Hexagonal, additiv: `domain.Artifact` (reine Validierung) → `ports.ArtifactStore` (+ Sentinel `ErrArtifactNotFound` in `ports.go`) → fünf Usecases (`upload/rename/list/delete/get`, je Datei — „keine Monolithen"; die drei Mutations-Usecases emittieren `artifact.*` selbst) → Adapter (`pgstore.ArtifactStore`, REST-Handler + Serve-Route + Web-Galerie-Handler + Editor-Picker-Fragmente, `apiclient`-Verben, drei `flow-mcp`-Tools, `flow artifact`-Cobra-Verben). Die Render-Ebene erweitert **einen** goldmark-Inline-Parser (`wikiLinkParser` erkennt zusätzlich `![[…]]`) und **einen** gemeinsamen Figuren-Nummerierungspass (Mermaid + Artefakt in Dokumentreihenfolge). Deep-Links über `parser.WithHeadingAttribute()` + AST-Slugger. Alle Mutationen emittieren `artifact.*`-SSE-Events; Konsument ist ein dedizierter Cockpit-Galerie-Container.

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · goldmark (+ bluemonday-Sanitizer) · Schibsted Grotesk + JetBrains Mono. **Eine** neue goose-Migration (`0031`, `bytea`); ein neuer Image-Decoder-Import (`_ "image/gif"`); **keine** neuen externen Abhängigkeiten, **kein** neues Vendoring. Ein neues kleines Client-JS (`static/js/editor-insert.js`) + Erweiterung `static/toc.js` + `static/js/clipboard.js`-Nutzung für „Link kopiert".

**Spec:** `docs/superpowers/specs/2026-07-07-lesesaal-l6-artefakte-design.md` — Stand Rev. **2026-07-09** (Commit `fdddb8f`): die Spec-Review-Findings von Codex + Gemini sind bereits EINGEARBEITET (Parser-Trigger `!`+`[`, gemeinsamer Nummerierungspass, flacher Slug, Ahnenkette statt Subtree, Ownership-Guard statt `requireBookable`, no-cache/immutable-Cache-Split, Sanitizer-URL-Policy exakt aufs Route-Muster, JSON-only-REST-Upload, `FindWikilinks`-`!`-Ausschluss, Owner-Quota + Pflicht-Testfälle). Diese Vorgaben sind **bindend** — nicht neu verhandeln. Normative Optik-Referenz: `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4) + vorhandene Lesesaal-Primitives (benannte Klassen, keine Arbitrary-Werte). Formatvorbild: `docs/superpowers/plans/2026-07-07-lesesaal-l55-kontext-modus.md`.

**Basis:** Branch **`lesesaal-l6`** (bereits ausgecheckt), Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. HEAD enthält bereits den Merge von `rebuild` inkl. **L5.5** (Kontext-Modus, Migration 0030) und **tmux-status-Slice** (`internal/statusline/`, `GET /api/v1/nodes/mru`, `flow worktime status/stop`). Das Slice-Gate umfasst `rebuild..HEAD` des L6-Branches.

---

## Global Constraints

- Branch **`lesesaal-l6`** (bereits ausgecheckt); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende mit der exakt vorgegebenen Message.
- **L4/L5-LEHREN — in JEDEN Task-Dispatch-Text aufnehmen:** (1) **Tests/CI SYNCHRON foreground**, **NIEMALS `run_in_background`** (Subagenten warten sonst auf nie kommende Notifications). (2) **Bash-Aufrufe mit `timeout: 600000`** (L5-Lehre: `make ci` läuft lange — Testcontainer-Postgres). (3) **Erst `git add -A` stagen, dann `make ci`** (verify-generate/verify-css diffen gegen den Index → uncommitted templ/css false-positiv). (4) **Nie zwei `make ci` parallel** (Podman-VM keilt bei parallelen Testcontainer-Läufen → Hard-Stop+Start). (5) **`make web` nach JEDER `.templ`-Änderung** (auch reine Klassennutzung ändert den Tailwind-Scan; verify-css ist ein Drift-Diff) und `internal/adapter/webui/static/app.css` mitcommitten. (6) **`make generate` nach JEDER `.templ`-Änderung**, die `*_templ.go` mitcommitten.
- **NIE `make fmt`** (Toolchain-Skew reformatiert das ganze Repo). **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen (Memory `feedback_subagent_git_commits_isolated`).
- `make ci` muss am Task-Ende grün sein (`lint verify-generate verify-css verify-no-popups cover build`; Coverage-Gate **75 %**, aktuell ~85 %, `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket, siehe AGENTS.md „Tailwind v4 + Docker"). **Task 1 fügt Migration 0031 hinzu → die pgstore-Docker-Tests laufen gegen das neue Schema; 0031 MUSS goose Up/Down-annotiert sein** (Memory `feedback_pgstore_goose_migrations`: nur die pgstore-Docker-Tests fangen fehlende Annotationen).
- **Migrations-Nummer 0031 vor dem Anlegen verifizieren:** `ls internal/adapter/pgstore/migrations/ | tail -3` — aktuell höchste ist `0030_documents_context_mode.sql`, also `0031` frei. Falls der Bestand inzwischen weiter ist, die **nächste freie Nummer** nehmen und im Plan/Ledger vermerken. **Bestand gewinnt.**
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced (`TestCatalogsParity`, `internal/i18n/catalog_test.go` — prüft nur Key-**Existenz**, EN-Strings **explizit ausformulieren**, nicht „gleichwertig"). Keine hartkodierten Anzeige-Strings; `components.T(ctx, "key")`/`Tn`.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ✚ ✗ ✓ ○ · ↑ ↓ ¶ + SVG erlaubt), **keine Browser-Popups** (`verify-no-popups` — kein `alert/confirm/prompt`; Bestätigungen über `components.ConfirmDialog`/`data-dialog-open`; „Link kopiert" über die Bestand-`clipboard.js`, kein `alert`).
- **owner-scoped überall** (jede Store-/Serve-/Mutation-Query trägt `ownerID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze, Memory `feedback_flow_is_multi_tenant`). Jede neue Datenfläche bekommt einen **Owner-Scope-Negativtest**: fremder Owner sieht/lädt/löscht/referenziert **nichts** (pgstore-Store, REST-Serve, REST-List, REST-Delete, Web-Galerie, MCP, CLI).
- **Owner-Quota (Multi-Tenant „Limits per-user"):** Gesamt-Bytes pro Owner gedeckelt (Konstante `MaxArtifactBytesPerOwner = 256 << 20`), im Upload-Usecase über `ArtifactStore.TotalBytes(ctx, owner)` geprüft; Überschreitung → Fehler mit i18n-Meldung (Web) bzw. **413** (REST). Pflicht-Testfall.
- **SSE-Regel (Mutation → Event → Konsument benannt):** jede Artefakt-Mutation emittiert über `s.Emitter.Emit` genau ein `domain.EventArtifactCreated`/`…Updated`/`…Deleted` (`"artifact.created"`/`…updated`/`…deleted`). Konsument: der dedizierte Cockpit-Galerie-Container `#cockpit-artifacts` (`hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`). Dokument-Embeds bleiben an `document.updated` (Re-Upload bumpt den Content-Hash → `<img ?v=hash>` lädt beim nächsten Doc-Render neu; **kein** Live-Reload einzelner Figuren in offenen Tabs — bewusst YAGNI, Spec §12).
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt, Memory `feedback_design_must_stay_easily_changeable`): neue Flächen (Figur, Datei-Chip, Galerie-Grid, Picker) nutzen **benannte** Lesesaal-Klassen (`.frame`/`.blk`/`.krow`/`.btn`/`.seg`/`.panel`/`.narrow`/`.eyebrow` — Bestand `web/tailwind.css`) bzw. neue **benannte** Klassen (`.figure`, `.filechip`, `.gallery`, `.artpick` o. ä.), **keine** Arbitrary-`[#hex]`/`[px]`/`[.85rem]`, wo eine benannte Klasse existiert oder anlegbar ist. Farben über `rgb(var(--token))` (`--meta`/`--blue`/`--surface`/`--accent`/`--warn`/`--faint`/`--hair`). Der **Bestand** (`fuzzypicker.templ`, `editor.templ`) nutzt viele Arbitrary-Werte — die werden **nicht** in neue Flächen kopiert (der neue generische Picker in Task 6 ist named-class).
- Tailwind-v4-Fallen (Memory `feedback_tailwind_v4_templ_gotchas`): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen; `make web` mit `DOCKER_HOST=podman` für `make ci`.
- **Wo ein normatives Mockup existiert, gewinnt bei Zweifel das Mockup** (`lesesaal.html` v2.4). Für Flächen ohne Mockup (Galerie-Grid, Datei-Chip, Picker, Hover-¶) gilt Lesesaal-Doktrin: leise Bestand-Elemente, kein neues Farbsystem, Containment 375px. Diese Flächen stehen als **Offene Entscheidungen** mit Empfehlung.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht, jeder Task hat Step 0):** JEDES als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Usecase-Feld, Store-Methode, Test-Helper, i18n-Key, CSS-Klasse) vor dem Tippen per `rg -n "<Name>" internal/ cmd/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden. Wörtliche Ankerliste u. a.: `NodeLogo`, `ValidateNodeLogo`, `MaxNodeLogoBytes`, `NodeLogoStore`, `ErrNodeLogoNotFound`, `handleWebNodeLogo`, `readValidatedLogo`, `Slugify`, `SlugOK`, `FindWikilinks`, `WikilinkTargets`, `WikilinkSpan`, `getDocPolicy`, `RenderDocument`, `WikilinkResolver`, `wikiLinkParser`, `kindWikiLink`, `mermaidTransformer`, `mermaidNode`, `mermaidHTMLRenderer`, `DocMeta`, `NodeStore.Ancestors`, `NodeAncestors`, `Emitter`, `EventDocumentUpdated`, `buildDocumentVM`, `buildOutgoingRefs`, `ResolveWikilink`, `isContextType`, `handleWebEditorPreview`, `renderEditorPreview`, `editorVM`, `EditorVM`, `MarkdownPreview`, `ProjectFuzzyPicker`, `NodePickerVM`, `CockpitRailBlocks`, `NodeCockpit`, `nodeCockpitData`, `cockpit.templ`-SSE-Container, `c.do`, `CreateNodeFields`, `resolveNodeRef`, `mcp.AddTool`, `h.do`, `h.resolveScope`, `errorResult`/`textResult`, `TestCatalogsParity`, `.frame`, `.blk`, `.krow`, `.btn`, `.seg`, `.narrow`, `.prose`, `.field`, `.panel`, `.eyebrow`.

---

## Artefakt-Semantik — Vorgabe (ENTSCHIEDEN aus Spec Rev. 2026-07-09, NICHT erneut konsultieren)

| Aspekt | Vorgabe (bindend) |
|---|---|
| **Modell** | Ein Artefakt hängt an EINEM `node`. Reichweite = **Ahnenkette** (Node + Vorfahren, `NodeStore.Ancestors` leaf→root). Ein Artefakt am Engagement ist in allen Repos darunter referenzierbar. **NICHT** Subtree. |
| **Slug** | flaches **Ein-Segment-Token** (KEIN `/`) — eigenes `domain.ArtifactSlugOK` (Muster `Slugify`), **nicht** `domain.SlugOK` (erlaubt `/`). Kollision beim Anlegen → `-1/-2`-Suffix (Serverseite via `ExistingSlugs`). Node-eindeutig (`unique(owner_id,node_id,slug)`). |
| **MIME** | **Bilder** (inline-preview) `image/png,image/jpeg,image/webp,image/gif` · **Downloads** `application/pdf,text/csv,text/plain,application/json,application/zip,application/octet-stream`. **SVG verboten.** `IsImage()` (Präfix `image/`) entscheidet Figur vs. Chip; `width/height` nur für Bilder (via `image.DecodeConfig`; **GIF-Decoder explizit `import _ "image/gif"`**). |
| **Limit** | Einzeldatei `MaxArtifactBytes = 8 << 20`. Owner-Gesamt `MaxArtifactBytesPerOwner = 256 << 20`. |
| **Ref / Cache** | `ref` = 12-hex-Content-Hash (`sha256[:12]`, Muster `UploadNodeLogo`). ETag = `"{ref}"`. **Umbenennen bumpt `ref` NICHT** (Content unverändert; Referenzen + Embed-URLs stabil). Serve-Route: **nackt** → `Cache-Control: private, no-cache` (Revalidierung via ETag → 304); **`?v={ref}`** → `private, max-age=31536000, immutable`. `X-Content-Type-Options: nosniff`. Bilder `Content-Disposition: inline`, sonst `attachment; filename="{name}"`. |
| **Guard (REST)** | reiner **Ownership-Check** `Nodes.Get(ctx, owner, id)` (Existenz + Owner). **KEIN `IsBookable`-Gate** (Artefakte an JEDEM Node-Kind erlaubt — Ahnenkette umfasst nicht-bookable Knoten; `requireBookable` existiert nicht). |
| **Upload-Weg** | Web-Galerie = **multipart** (Muster Logo). REST/MCP/CLI = **JSON-only** `{name,mime,dataBase64}` (API-Konvention + `apiclient.do()` sind durchgehend JSON). Editor referenziert nur **Vorhandenes** (kein Upload im Editor). |
| **Referenz** | `![[slug]]` löst **ausschließlich Artefakte** auf (keine Doc-Transklusion). `[[…]]` bleibt Doc-Wikilink. Auflösung gegen die Artefakt-Bibliothek der **Ahnenkette des Dokuments**, erster Treffer gewinnt (nächster Vorfahr zuerst). Nicht gefunden / Doc ungebunden → sichtbarer „ungelöst"-Chip (Muster `.wikilink-broken`), bricht das Rendern nicht. |
| **Nummerierung** | Figuren (Bild + Datei-Chip) teilen **denselben** Zähler mit Mermaid — **EIN gemeinsamer dokumentordnungs-treuer Nummerierungspass** (ein AST-Transformer nummeriert Mermaid-Blöcke UND Artefakt-Embed-Knoten in Dokumentreihenfolge; beide Renderer lesen `n.N` nur ab). |

---

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1–L5.5)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. **Dispatches nennen das Modell NIE implizit** (Memory `feedback_subagent_model_never_inherit_fable`: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 Storage-Fundament (Migr 0031 · `domain.Artifact`/Slug/MIME · pgstore `ArtifactStore` · Port + `ErrArtifactNotFound` · Fakes · `FindWikilinks` `!`-Ausschluss) | `lesesaal-implementer-deep` | Sonnet · high |
| 2 Usecases (upload/rename/list/delete/get + Quota, SSE im Usecase) · REST (JSON POST/GET/DELETE + Serve-Route) · apiclient · SSE-Events (`event.go`) · Server-Felder + Teil-Wiring | `lesesaal-implementer-deep` | Sonnet · high |
| 3 Inline-Referenz (`![[…]]`-Parser-Trigger · gemeinsamer Nummerierungspass · Figur/Chip-Renderer · Resolver-Threading durch `RenderDocument` · Sanitizer-Policy) | `lesesaal-implementer-deep` | Sonnet · high |
| 4 Deep-Links (Heading-Slugger + `WithHeadingAttribute` · Sanitizer `id` auf h1–h6 · `toc.js` · Hover-¶ + „kopiert"-Feedback) | `lesesaal-implementer` | Sonnet · medium |
| 5 Cockpit-Galerie (`CockpitArtifacts` + SSE-Container · multipart-Upload · Umbenennen · Löschen (ConfirmDialog) · eigene+geerbte · i18n) | `lesesaal-implementer-deep` | Sonnet · high |
| 6 Editor-Einfügehelfer (generischer Picker · `/ui/editor/artefakte` + `/ui/editor/seiten` · `editor-insert.js` · Werkzeugleiste · Preview-Node-Kontext · i18n) | `lesesaal-implementer-deep` | Sonnet · high |
| 7 flow-mcp (3 Tools) + CLI (`flow artifact add/ls/rm`) | `lesesaal-implementer` | Sonnet · medium |
| 8 Wiring-Gate (Composition-Root · Sweep · `make ci` · Live-Dogfood · Breakpoints) | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch (`rebuild..HEAD`) | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Artefakt-Semantik-Vorgabe + „Branch `lesesaal-l6`, HEAD-basiert, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch. **Explizit im Dispatch:** „Tests/`make ci` SYNCHRON foreground, `timeout: 600000`, keine Hintergrund-Läufe; erst `git add -A`, dann `make ci`; nie zwei `make ci` parallel; `DOCKER_HOST` = Podman-Socket."
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range (BASE = Task-Base). `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge, `rebuild..HEAD`):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch, Dispatch-Text unten) über `git diff --name-only rebuild..HEAD`.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen. **Fokus L6:** Owner-Scoping über JEDE Artefakt-Fläche (Store/Serve/List/Delete/Web/MCP/CLI, inkl. Cross-Tenant-Serve-Negativtest); die **Ahnenkette** (nicht Subtree) an ALLEN Auflösungsstellen (REST-List, Galerie, Doc-Resolver, Editor-Picker); der `!`-Parser-Trigger bricht **keine** Bestandsbilder `![](url)` und keine kaputten `![[`-Formen; der **gemeinsame** Figuren-Zähler ist wirklich geteilt (gemischte Mermaid/Artefakt-Reihenfolge); Sanitizer lässt **nur** die Serve-Route-`img src` durch (extern/`data:`/`//host` gestrippt); Cache/ETag-Split (nackt=`no-cache`, `?v=`=immutable); `FindWikilinks` ignoriert `!`-Präfix (Backlinks sauber); Owner-Quota greift (413/i18n); jede Mutation emittiert das passende `artifact.*`-Event mit benanntem Konsument; main.go verdrahtet Store + 5 Usecases (3 mit `Emitter`) + Routen + MCP-Tools (kein „ship a usecase nothing calls").
4. `lesesaal-mockup-auditor` → Galerie/Figur/Chip/Picker/Hover-¶ haben **kein** Pixel-Mockup → Prüfung gegen Lesesaal-Doktrin (leise Bestand-Elemente, `.frame`/`.blk`/`.btn`/`.seg`-Konsistenz, kein neues Farbsystem, keine Emojis, Containment 375px), plus: die Figuren-Optik gegen die Bestand-`mermaid-figure`/`.frame`-Präzedenz.
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — Dogfood-Skript Spec §13 (Upload Bild+PDF → `![[slug]]` einbetten → Figur/Chip + Abb.-Nummer korrekt → Deep-Link `#slug` scrollt → Editor-Picker fügt beide Token-Arten ein → MCP-Upload eines Agenten landet in der Galerie → owner-fremder Zugriff scheitert; 960px/375px-Sichtprobe).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`), **PROD-Deploy-Notiz** (Migration 0031 rollt via goose; Blob-Storage wächst — Owner-Quota greift).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **verwaiste i18n-Keys** (in beiden Katalogen definiert, nirgends per `T(`/`Tn(` referenziert) — besonders `artifact.*`/`figure.*`/`editor.insert.*`-Keys; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `w-[`, `h-[`, `text-[.`, `max-h-[`) auf den L6-**Neubauten** (Galerie, Figur, Chip, Picker, Editor-Toolbar), wo eine benannte Lesesaal-Klasse existiert/anlegbar ist; (c) **verwaiste Symbole** mit null Konsumenten unter den L6-Neubauten (`Artifact`, `ArtifactStore`, `ArtifactSlug*`, `ListArtifacts`, `UploadArtifact`, `RenameArtifact`, `DeleteArtifact`, `GetArtifact`, `artifactEmbedNode`, `ArtifactResolver`, `ArtifactRef`, `CockpitArtifacts`, `EventArtifact*`); (d) **Querschnitt-Lücken:** löst JEDE Auflösungsstelle gegen die **Ahnenkette** (nicht Subtree) auf? emittiert JEDE Mutation (REST-Upload, REST-Delete, Web-Upload, Web-Rename, Web-Delete, MCP-Upload/Delete, CLI) das passende `artifact.*`-Event? ist der `!`-Trigger + der gemeinsame Zähler test-gedeckt (gemischte Reihenfolge, `![](url)`-Bestand, kaputte `![[`)? sind die Sanitizer-Negativtests (extern/`data:`/`//host`) vorhanden UND grün? ist der Cache-Split (nackt vs. `?v=`) test-gedeckt? greift die Owner-Quota (413/i18n)? (e) **main.go-Wiring:** sind `ArtifactStore` + `UploadArtifact` + `RenameArtifact` + `ListArtifacts` + `DeleteArtifact` + `GetArtifact` im Server-Literal (die 3 Mutations-Usecases mit `Emitter: emitter`), die REST/Serve/Web-Galerie/Editor-Picker-Routen in `server.go`, die MCP-Tools in `cmd/flow-mcp/server.go` registriert? Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` übersprungen; Nachlauf ist Orchestrator-Arbeit.

---

### Task 1: Storage-Fundament — Migration 0031 · `domain.Artifact` (Slug/MIME/Validate) · pgstore `ArtifactStore` · Port + `ErrArtifactNotFound` · Fakes · `FindWikilinks` `!`-Ausschluss

**Files:**
- Create: `internal/adapter/pgstore/migrations/0031_artifacts.sql`
- Create: `internal/domain/artifact.go` (+ `internal/domain/artifact_test.go`)
- Create: `internal/adapter/pgstore/artifacts.go` (+ `internal/adapter/pgstore/artifacts_test.go`)
- Modify: `internal/ports/ports.go` (`ArtifactStore`-Interface + Sentinel `ErrArtifactNotFound`)
- Modify: `internal/domain/wikilink.go` (`FindWikilinks`: `!`-Präfix-Ausschluss) + `internal/domain/wikilink_test.go`
- Modify: `internal/testutil/fakes.go` (`FakeArtifactStore`, Muster `FakeNodeLogoStore` :1439-Region)

**Interfaces / Produces (für Tasks 2–7):**
- **`domain.Artifact struct`** (Felder wie Spec §4.1, ohne DB-nur-Felder): `ID, OwnerID, NodeID, Slug, Name, Mime string; SizeBytes int64; Ref string; Bytes []byte; Width, Height int; CreatedByKind, CreatedByRef string; CreatedAt, UpdatedAt time.Time`. `json`-Tags für die REST-Meta-Antwort (`bytes` mit `json:"-"`).
- **`domain.ArtifactMimeAllowlist`** (map/set) mit `ArtifactImageMimes` (`image/png,image/jpeg,image/webp,image/gif`) + `ArtifactDownloadMimes` (`application/pdf,text/csv,text/plain,application/json,application/zip,application/octet-stream`). **`image/svg+xml` NICHT enthalten.**
- **`func ArtifactSlugOK(s string) bool`** — flaches Ein-Segment-Token: `^[a-z0-9]+(?:-[a-z0-9]+)*$` (KEIN `/`, kein `_`-Segment nötig — Muster `Slugify`-Output). **`func ArtifactSlug(name string) string`** — `Slugify(name)` ohne Endung-Sonderlogik (Basis für Kollisions-Suffix; Endung darf im Slug landen oder wird abgetrennt — Empfehlung: Dateiendung vor dem Slugify abtrennen, damit `bild.png` → `bild`).
- **`func (a Artifact) IsImage() bool`** — `strings.HasPrefix(a.Mime, "image/")`.
- **`func (a Artifact) Validate() error`** — reine Validierung (kein I/O): `ArtifactSlugOK(a.Slug)`, `a.Mime ∈ Allowlist`, `a.SizeBytes ≤ MaxArtifactBytes`, `a.Name != ""`; Fehler mit neuem Domain-Sentinel `ErrInvalidArtifact` (in `internal/domain/errors.go`).
- **Konstante** `MaxArtifactBytes int64 = 8 << 20` (in `domain/artifact.go`).
- **`ports.ArtifactStore`** (in `ports.go`, Doku wie Bestand):
```go
// ArtifactStore persists node-scoped artifacts as Postgres blobs (N per node,
// FK ON DELETE CASCADE). All reads are owner-scoped.
type ArtifactStore interface {
	// Put upserts on (owner_id, node_id, slug).
	Put(ctx context.Context, a domain.Artifact) error
	// Get returns one artifact incl. bytes. Owner-scoped; ErrArtifactNotFound when absent.
	Get(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error)
	// GetMeta returns one artifact WITHOUT bytes (rename/exists checks).
	GetMeta(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error)
	// List returns artifact META (no bytes) for the given nodeIDs (caller passes
	// the ancestor chain — Node + ancestors), newest first. Owner-scoped.
	List(ctx context.Context, ownerID string, nodeIDs ...string) ([]domain.Artifact, error)
	// Rename changes only the display name + updated_at (slug/ref/bytes untouched
	// — Referenzen + Embed-URLs bleiben stabil). Owner-scoped; ErrArtifactNotFound
	// when absent. (OE #6 — Empfehlung: eigene Methode statt Get(bytes)+Put.)
	Rename(ctx context.Context, ownerID, nodeID, slug, name string) error
	// Delete removes one artifact; ErrArtifactNotFound when absent.
	Delete(ctx context.Context, ownerID, nodeID, slug string) error
	// ExistingSlugs returns the slugs already used under (owner,node) — collision suffix.
	ExistingSlugs(ctx context.Context, ownerID, nodeID string) ([]string, error)
	// TotalBytes returns SUM(size_bytes) for the owner (per-user quota).
	TotalBytes(ctx context.Context, ownerID string) (int64, error)
}
```
- **`ports.ErrArtifactNotFound = errors.New("artifact not found")`** (in `ports.go`, neben `ErrNodeLogoNotFound` — Store-Sentinels leben in `ports.go`).

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "func Slugify|var nonSlug|var deUmlauts" internal/usecase/create_node.go
rg -n "func SlugOK|var slugRe" internal/domain/document.go
rg -n "type NodeLogoStore interface|ErrNodeLogoNotFound|type Emitter interface|Ancestors\(ctx" internal/ports/ports.go
rg -n "func .*Put\(ctx|ON CONFLICT|pgx.ErrNoRows|BYTEA|bytea" internal/adapter/pgstore/nodelogos.go
rg -n "func FindWikilinks|func WikilinkTargets|type WikilinkSpan" internal/domain/wikilink.go
rg -n "FakeNodeLogoStore|ErrNodeLogoNotFound" internal/testutil/fakes.go
rg -n "ErrInvalidDocument|ErrInvalid" internal/domain/errors.go
ls internal/adapter/pgstore/migrations/ | tail -3      # höchste = 0030 → neu 0031 (sonst nächste freie)
```
- [ ] **Step 1: Failing Tests**
  - `internal/domain/artifact_test.go`: `Validate` akzeptiert PNG-Bild + PDF-Download; lehnt `image/svg+xml`, Übergröße (`SizeBytes > MaxArtifactBytes`), leeren/`slug/mit/slash`-Slug ab. `ArtifactSlugOK("bild-1")`==true, `ArtifactSlugOK("a/b")`==false, `ArtifactSlugOK("A")`==false. `IsImage()` für image/* true, für application/pdf false. `ArtifactSlug("Mein Bild.PNG")`=="mein-bild".
  - `internal/domain/wikilink_test.go`: neuer Fall — `FindWikilinks("![[bild]] und [[doc]]")` liefert **nur** `doc` (das `![[bild]]` ist ausgeschlossen); `WikilinkTargets` dito.
  - `internal/adapter/pgstore/artifacts_test.go` (testcontainer, Muster `TestNodeStore_*`/`newDocStore`-Helper per rg verifizieren): `Put`→`Get` Roundtrip (bytes zurück); `GetMeta` ohne bytes; `List` über zwei Node-IDs (Ahnenkette) newest-first; `ExistingSlugs`; `TotalBytes` = Summe; `Delete`→`ErrArtifactNotFound`; **Owner-Scope-Negativtest** (`Get`/`Delete`/`List` mit fremdem Owner → nichts/`ErrArtifactNotFound`); Upsert (zweites `Put` auf gleichem `(owner,node,slug)` überschreibt bytes/ref, kein Duplikat).
- [ ] **Step 2: Laufen lassen** — FAIL (Typen/Store fehlen). `DOCKER_HOST`=Podman-Socket.
- [ ] **Step 3: Migration 0031** (goose Up/Down PFLICHT, Muster `node_logos`):
```sql
-- +goose Up
CREATE TABLE artifacts (
    id              TEXT PRIMARY KEY,
    owner_id        TEXT NOT NULL REFERENCES users(id),
    node_id         TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    mime            TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    ref             TEXT NOT NULL,
    bytes           BYTEA NOT NULL,
    width           INTEGER,
    height          INTEGER,
    created_by_kind TEXT NOT NULL DEFAULT '',
    created_by_ref  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    UNIQUE (owner_id, node_id, slug)
);
CREATE INDEX artifacts_owner_node ON artifacts (owner_id, node_id);

-- +goose Down
DROP TABLE artifacts;
```
- [ ] **Step 4: Domain** — `artifact.go` (Struct + Konstanten + Allowlist + `ArtifactSlugOK`/`ArtifactSlug`/`IsImage`/`Validate`); `errors.go` `ErrInvalidArtifact`; `wikilink.go` `FindWikilinks` `!`-Ausschluss:
```go
// In FindWikilinks, inside the loop after matching s[i]=='[' && s[i+1]=='[':
if i > 0 && s[i-1] == '!' {
    continue // artifact embed ![[slug]] — not a doc wikilink (L6)
}
```
- [ ] **Step 5: pgstore `ArtifactStore`** — `artifacts.go` analog `nodelogos.go` (`NewArtifactStore(pool)`; `Put` `INSERT … ON CONFLICT (owner_id,node_id,slug) DO UPDATE SET name=…, mime=…, size_bytes=…, ref=…, bytes=…, width=…, height=…, updated_at=…`; `Get`/`GetMeta` (GetMeta wählt alle Spalten außer `bytes`); `List` mit `WHERE owner_id=$1 AND node_id = ANY($2) ORDER BY created_at DESC` (`nodeIDs` als `[]string` via pgx-Array); `ExistingSlugs`; `TotalBytes` `SELECT coalesce(sum(size_bytes),0)`; **`Rename` `UPDATE artifacts SET name=$1, updated_at=$2 WHERE owner_id=$3 AND node_id=$4 AND slug=$5` → `ErrArtifactNotFound` bei 0 Rows**; `Delete` → `ErrArtifactNotFound` bei 0 Rows). SQL wörtlich, owner-scoped. **Interface-Ripple:** `Rename` zwingt jede `ArtifactStore`-Fake zur Methode (Step 6, Compiler-geführt).
- [ ] **Step 6: Port + Sentinel + Fakes** — `ArtifactStore`-Interface + `ErrArtifactNotFound` in `ports.go`; `FakeArtifactStore` in `internal/testutil/fakes.go` (In-Memory-Map keyed `(owner,node,slug)`; `List` filtert owner + node-in-Menge; `TotalBytes` summiert owner; Muster `FakeNodeLogoStore`). `go build ./... ./internal/...` — der Compiler listet Fake-Lücken.
- [ ] **Step 7: Bauen + Tests + Commit**
```bash
git add -A
go build ./... && go test ./internal/domain/... ./internal/adapter/pgstore/... -race   # Docker-Socket
git commit -m "feat(pgstore): artifacts store (Migr 0031, bytea) + domain.Artifact/Slug/MIME + FindWikilinks !-Ausschluss — Node-Asset-Fundament"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig (keine templ/css-Änderung).

---

### Task 2: Usecases (upload/rename/list/delete/get + Quota, SSE im Usecase) · REST (JSON POST/GET/DELETE + Serve-Route) · apiclient · SSE-Events · Server-Felder + Teil-Wiring

**Files:**
- Create: `internal/usecase/upload_artifact.go`, `rename_artifact.go`, `list_artifacts.go`, `delete_artifact.go`, `get_artifact.go` (je Datei — „keine Monolithen") + je `_test.go`
- Modify: `internal/domain/event.go` (drei `EventArtifact*`-Konstanten)
- Create: `internal/adapter/httpserver/artifacts.go` (REST-Handler: JSON-Upload, List, Delete) + `internal/adapter/httpserver/artifacts_serve.go` (Serve-Route) + `_test.go`
- Modify: `internal/adapter/httpserver/server.go` (Server-Felder + REST-Routen + Serve-Route)
- Modify: `cmd/flow-server/main.go` (Store + Usecases ins Server-Literal — Task 8 verifiziert die Composition-Root)
- Create: `internal/adapter/apiclient/artifacts.go` (+ `_test.go`)

**Interfaces / Produces (für Tasks 3/5/6/7):**
- **SSE in den Usecases (Codex-Fund #3, Spec §12 „emittiert von den Mutations-Usecases"):** die drei Mutations-Usecases (`UploadArtifact`/`RenameArtifact`/`DeleteArtifact`) halten ein **`Emitter ports.Emitter`**-Feld und **emittieren selbst** in `Execute` (Präzedenz `usecase.AddDayOffs{…, Emitter: emitter}` in main.go). Grund: Upload UND Delete haben **je zwei Einstiege** (REST + Web-Galerie) — Emit im Usecase ist DRY (kein doppelter Emit-Pfad) und spec-konform; die Handler emittieren NICHT mehr selbst. `EventData` trägt `{"nodeId":…, "slug":…}`.
- **`usecase.UploadArtifact{ Nodes ports.NodeStore; Artifacts ports.ArtifactStore; IDs ports.IDGen; Clock ports.Clock; Emitter ports.Emitter }`** mit `Execute(ctx, ownerID, nodeID, name, declaredMime string, data []byte, replaceSlug, actorKind, actorRef string) (domain.Artifact, error)`:
  1. Ownership-Guard `Nodes.Get(ctx, owner, nodeID)` → `ErrNodeNotFound`.
  2. Content-Sniff + Bild-Vermessung (`usecase.ValidateArtifactBytes` — Muster `ValidateNodeLogo`, **`import _ "image/gif"`** ergänzt): für Bilder muss `image.DecodeConfig` gelingen (liefert `w/h`); der gesniffte Bild-MIME ist autoritativ (ein als PDF deklariertes PNG wird als `image/png` gespeichert; ein als Bild deklariertes Nicht-Bild wird abgelehnt). Für Nicht-Bilder gilt der **deklarierte** MIME, wenn ∈ `ArtifactDownloadMimes`, sonst `http.DetectContentType`-Fallback; SVG (weder Sniff noch Allowlist) → `ErrArtifactBadType`. `SizeBytes > MaxArtifactBytes` → `ErrArtifactTooLarge`.
  3. **Owner-Quota:** `TotalBytes(owner) + SizeBytes > MaxArtifactBytesPerOwner` → `ErrArtifactQuotaExceeded`. **Bekannte Race (agy-Fund #5, bewusst akzeptiert als Soft-Cap):** `TotalBytes`-Read + `Put` sind nicht atomar → parallele Uploads eines Owners können den Cap um bis zu `MaxArtifactBytes × gleichzeitige Uploads` (≤ 8 MiB je Upload) überschreiten. Für einen 256-MiB-Soft-Cap ist der beschränkte Overshoot vertretbar (KEINE „nur ein User"-Begründung — der Overshoot ist pro Tenant beschränkt und pro Upload gedeckelt). **Deferred-Härtung** (Ledger-Notiz): transaktionaler Check (`SELECT sum(size_bytes) … FOR UPDATE` bzw. pg-advisory-lock owner-keyed) falls je nötig. Ein Concurrency-Test ist NICHT Teil von L6.
  4. **Slug + Neu-vs-Ersetzen (Codex-Fund #4, Spec §3/§12 Re-Upload überschreibt):** wenn `replaceSlug != ""` → **Überschreiben** dieses Slugs (`Put` upsert, neues `ref`, `name` aktualisiert), Event **`artifact.updated`**. Sonst **neu**: `base := ArtifactSlug(name)`; Kollision gegen `ExistingSlugs` → `-1/-2`-Suffix (nächster freier), Event **`artifact.created`**.
  5. `ref := sha256[:12]`; `Put`; **`Emitter.Emit`** (created bzw. updated); Rückgabe der Meta (mit finalem Slug). `CreatedByKind/Ref` aus den Argumenten (Actor-Kind human/agent).
  - `MaxArtifactBytesPerOwner int64 = 256 << 20` + `ErrArtifactTooLarge`/`ErrArtifactBadType`/`ErrArtifactQuotaExceeded` (usecase-Package, Muster `ErrLogoTooLarge`).
- **`usecase.RenameArtifact{ Nodes ports.NodeStore; Artifacts ports.ArtifactStore; Emitter ports.Emitter }`** (Codex-Fund #2 — eigener Usecase, NICHT Store-Direktaufruf aus dem Handler) mit `Execute(ctx, ownerID, nodeID, slug, name string) error` — `GetMeta` bestätigt, dass das Artefakt an DIESEM Node hängt (nicht geerbt) → `Artifacts.Rename` (Slug/ref/bytes stabil) → **Emit `artifact.updated`**. Owner-scoped; `ErrArtifactNotFound`.
- **`usecase.ListArtifacts{ Nodes ports.NodeStore; Artifacts ports.ArtifactStore }`** mit `Execute(ctx, ownerID, nodeID string) ([]domain.Artifact, error)` — intern `chain := Nodes.Ancestors(ctx,owner,nodeID)` → `nodeIDs` → `Artifacts.List(owner, nodeIDs...)`. **Ahnenkette** (nicht Subtree). Owner-scoped über beide Primitive.
- **`usecase.DeleteArtifact{ Artifacts ports.ArtifactStore; Emitter ports.Emitter }`** `Execute(ctx, owner, nodeID, slug) error` — owner+node+slug-scoped `Delete` (fremder Owner → `ErrArtifactNotFound`, **kein `Nodes`-Feld nötig**, Codex-Fund #1: die Store-Primitive sind owner+node+slug-scoped), dann **Emit `artifact.deleted`**.
- **`usecase.GetArtifact{ Artifacts ports.ArtifactStore }`** `Execute(ctx, owner, nodeID, slug) (domain.Artifact, error)` (inkl. bytes, für die Serve-Route; owner+node+slug-scoped → fremder Owner sieht nichts, kein `Nodes`-Feld nötig — Codex-Fund #1). Read-only, kein Event.
- **Events:** `EventArtifactCreated EventType = "artifact.created"`, `EventArtifactUpdated = "artifact.updated"`, `EventArtifactDeleted = "artifact.deleted"` in `event.go`.
- **REST** (owner-scoped, Guard = Ownership; **die Handler emittieren NICHT — der Usecase tut es**):
  - `POST /api/v1/nodes/{id}/artifacts` (`s.auth`) — **JSON-only** `{name,mime,dataBase64}` (+ optional `"slug"` für Re-Upload/Ersetzen, Codex-Fund #4); Body-Limit `http.MaxBytesReader` ~12 MiB (8 MiB × 4/3 + JSON-Overhead); `base64.StdEncoding.DecodeString`; → **201** mit Artefakt-Meta; `ErrArtifactTooLarge`/`ErrArtifactBadType` → **400**; `ErrArtifactQuotaExceeded` → **413**; `ErrNodeNotFound` → **404**. Actor-Kind aus dem Request-Actor (Bestand `actor.FromContext` — per rg verifizieren). `UploadArtifact.Execute` emittiert `artifact.created` bzw. (bei `slug`-Replace) `artifact.updated`.
  - `GET /api/v1/nodes/{id}/artifacts` (`s.auth`) — Liste (**Ahnenkette**, Meta) via `ListArtifacts`. 200.
  - `DELETE /api/v1/nodes/{id}/artifacts/{slug}` (`s.auth`) — 204; `ErrArtifactNotFound` → 404. `DeleteArtifact.Execute` emittiert `artifact.deleted`.
  - `GET /nodes/{id}/artifacts/{slug}` (`s.webAuth`, Serve — §5): `GetArtifact`; ETag `"{ref}"`; **Cache-Split**: `?v={ref}` gesetzt → `private, max-age=31536000, immutable`, sonst `private, no-cache`; `If-None-Match`==ETag → 304 (ETag+Cache-Control **vor** dem 304 setzen, Muster `handleWebNodeLogo`); `X-Content-Type-Options: nosniff`; `Content-Type: mime`; `Content-Disposition: inline` (Bild) bzw. `attachment; filename="{name}"` (sonst); `w.Write(bytes)`.
- **apiclient** (`artifacts.go`): `UploadArtifact(ctx, nodeID, name, mime string, data []byte) (domain.Artifact, error)` (POST `map[string]any{"name":…,"mime":…,"dataBase64":base64.StdEncoding.EncodeToString(data)}`); `ListArtifacts(ctx, nodeID) ([]domain.Artifact, error)` (GET); `DeleteArtifact(ctx, nodeID, slug) error` (DELETE). Muster `c.do(ctx, method, path, body, &out)`.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func .*UploadNodeLogo|ValidateNodeLogo|sha256.Sum256|MaxNodeLogoBytes" internal/usecase/upload_node_logo.go`; `rg -n "actor.FromContext|FromContext\(ctx" internal/ -g '!*_test.go' | head`; `rg -n "MaxBytesReader|json.NewDecoder|userFrom|s.auth|s.webAuth|mux.Handle\(\"GET /nodes/\{id\}/logo" internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_nodelogo.go`; `rg -n "func \(c \*Client\) do\(|c.do\(ctx" internal/adapter/apiclient/*.go | head`; `rg -n "EventDocumentUpdated|EventNodeCreated" internal/domain/event.go`; `rg -n "UploadNodeLogo:|GetNodeLogo:|nodeLogoStore :?=|NewNodeLogoStore|emitter :?=" cmd/flow-server/main.go`; `rg -n "UploadNodeLogo\s+usecase|GetNodeLogo\s+usecase|NodeAncestors\s+usecase|Emitter\s+ports" internal/adapter/httpserver/server.go`.
- [ ] **Step 1: Failing Tests**
  - `upload_artifact_test.go` (Fake-Stores + **Fake-Emitter**): PNG-Bytes → Bild-Artefakt (mime image/png, w/h gesetzt, Slug aus Name) **+ genau ein `artifact.created`**; PDF-Bytes+mime application/pdf → Download (kein w/h); SVG → `ErrArtifactBadType` (kein Event); Slug-Kollision (neu) → `-1`-Suffix + `artifact.created`; **Re-Upload/Replace** (`replaceSlug` gesetzt) → Überschreiben, neuer `ref`, **`artifact.updated`** (Codex-Fund #4); **Quota:** vorbelegter Owner nahe `MaxArtifactBytesPerOwner` → `ErrArtifactQuotaExceeded` (kein Event); unbekannter/fremder Node → `ErrNodeNotFound`; Actor-Kind wird durchgereicht.
  - `rename_artifact_test.go` (Fake-Stores + Fake-Emitter): Rename ändert `name`, lässt `slug`/`ref` stabil, **`artifact.updated`**; geerbtes/fremdes Artefakt (GetMeta an DIESEM Node schlägt fehl) → `ErrArtifactNotFound`, kein Event.
  - `list_artifacts_test.go`: Node mit Vorfahr, Artefakt am Vorfahr → erscheint in der Liste des Kindes (**Ahnenkette**); Artefakt an einem Nicht-Vorfahr → erscheint NICHT.
  - `delete_artifact_test.go` (Fake-Emitter): Roundtrip + **`artifact.deleted`**; `ErrArtifactNotFound` (kein Event). `get_artifact_test.go`: Roundtrip + `ErrArtifactNotFound`.
  - `internal/adapter/httpserver/artifacts_test.go` (httptest): POST JSON → 201 + Meta + genau ein `artifact.created` (der Usecase emittiert, via Fake-Emitter im Server); Re-Upload mit `slug` → `artifact.updated`; übergroß → 400; Quota → 413; GET Liste (Ahnenkette); DELETE → 204 + `artifact.deleted`; **Owner-Scope** (User B kann A's Node nicht bespielen → 404); **Serve** (`GET /nodes/{id}/artifacts/{slug}`): Bild → `Content-Type` + `inline` + ETag; nackt → `no-cache`; `?v=ref` → `immutable`; `If-None-Match` → 304; Nicht-Bild → `attachment; filename`; `nosniff` gesetzt; **Cross-Tenant-Serve-Negativtest** (fremder Owner → 404/nichts).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3–5: Implementieren** — Events; **fünf** Usecases (je Datei: `upload_artifact.go`, `rename_artifact.go`, `list_artifacts.go`, `delete_artifact.go`, `get_artifact.go`) + `ValidateArtifactBytes`; REST-Handler + Serve-Route (Handler emittieren nicht — Usecases tun es); apiclient; Server-Felder (`UploadArtifact`/`RenameArtifact`/`ListArtifacts`/`DeleteArtifact`/`GetArtifact` in die Server-Struct, Muster `UploadNodeLogo`-Block) + Routen (`s.auth` REST, `s.webAuth` Serve); main.go: `artifactStore := pgstore.NewArtifactStore(pool)` + die **fünf** Usecase-Felder ins Server-Literal, jeweils mit `Emitter: emitter` bei den drei Mutations-Usecases (`UploadArtifact: usecase.UploadArtifact{Nodes: nodeStore, Artifacts: artifactStore, IDs: ids, Clock: clock, Emitter: emitter}`, `RenameArtifact: usecase.RenameArtifact{Nodes: nodeStore, Artifacts: artifactStore, Emitter: emitter}`, `DeleteArtifact: usecase.DeleteArtifact{Artifacts: artifactStore, Emitter: emitter}`, `ListArtifacts: usecase.ListArtifacts{Nodes: nodeStore, Artifacts: artifactStore}`, `GetArtifact: usecase.GetArtifact{Artifacts: artifactStore}`).
- [ ] **Step 6: apiclient-Test** — gegen Stub-Server (Muster `documents_test.go`): Upload/List/Delete round-trip.
- [ ] **Step 7: Bauen + Tests + Commit**
```bash
git add -A && go test ./internal/usecase/... ./internal/adapter/httpserver/... ./internal/adapter/apiclient/... ./internal/domain/... -race 2>&1 | tail -20
git commit -m "feat(artifacts): upload/rename/list/delete/get usecases (Ahnenkette, Owner-Quota, artifact.* SSE im Usecase) + REST JSON + Serve-Route (ETag/Cache-Split/nosniff) + apiclient"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig.

---

### Task 3: Inline-Referenz — `![[…]]`-Parser-Trigger · gemeinsamer Nummerierungspass · Figur/Chip-Renderer · Resolver-Threading durch `RenderDocument` · Sanitizer-Policy

**Files:**
- Modify: `internal/adapter/webui/wikilink.go` (`wikiLinkParser.Trigger()`/`Parse()`, `artifactEmbedNode`, `RenderDocument`-Signatur + Renderer-Registrierung, `getDocPolicy`)
- Modify: `internal/adapter/webui/mermaid.go` (Nummerierung in den **gemeinsamen** Figuren-Zähl-Transformer überführen)
- Create: `internal/adapter/webui/artifact_embed.go` (`artifactEmbedNode` + `artifactEmbedHTMLRenderer` + `ArtifactResolver`/`ArtifactRef` — „keine Monolithen")
- Modify: `internal/adapter/httpserver/webui_document.go` (`buildDocumentVM`: Artefakt-Resolver aus der Ahnenkette bauen + durch `RenderDocument` reichen)
- Modify: `internal/adapter/httpserver/webui_editor.go` (beide `RenderDocument`-Aufrufe: Artefakt-Resolver — siehe Task 6 für den Node-Kontext der Preview; hier zunächst leerer Resolver, Task 6 verdrahtet den Node)
- Modify: `web/tailwind.css` (nur **eine** neue benannte Klasse `.filechip`; die Bild-Figur nutzt die **Bestand-Klassen `figure` + `figure .frame`** — `tailwind.css:520-521`, Containment `overflow-x:auto` + `min-width:0` bereits vorhanden, wie `mermaid-figure`; unaufgelöst = Bestand `.wikilink-broken` `:717`; **kein `.chip` im Bestand** → `.filechip` ist neu)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (`document.figure.download`-Größe/Chip-Labels; `figure.unresolved`)
- Test: `internal/adapter/webui/wikilink_test.go` (Parser-Konflikt + Numbering + Resolver + Sanitizer)

**Interfaces / Produces:**
- **`webui.ArtifactRef struct{ Href, Ref, Name, Mime, SizeStr string; IsImage bool; Width, Height int }`** + **`type ArtifactResolver func(slug string) (ArtifactRef, bool)`**. **`Ref` ist Pflicht (Codex-Fund #5):** der Bild-Renderer bildet `src="{Href}?v={Ref}"` (versioniert → immutable-cacheable); der Datei-Chip nutzt `{Href}` **nackt** (no-cache → frischer Download-Dateiname, Spec §5). Ohne `Ref` ließe sich `?v=` nicht bilden. `ListArtifacts` liefert je Artefakt `Ref` mit.
- **`RenderDocument(ctx, src string, resolve WikilinkResolver, resolveArtifact ArtifactResolver) (template.HTML, DocMeta)`** — neue Signatur (3 Bestand-Caller ziehen mit; ein `nil`-`resolveArtifact` behandelt jedes Embed als „ungelöst").
- **Serve-URL zeigt auf den EIGENEN Node des Artefakts (KRITISCH):** `ArtifactRef.Href = "/nodes/" + artifact.NodeID + "/artifacts/" + slug` — das aufgelöste Artefakt kann an einem **Vorfahren** des Dokument-Nodes hängen; die `<img src>`/`<a href>` muss auf `artifact.NodeID` zeigen, NICHT auf den Dokument-Node (sonst 404 auf der Serve-Route). `ListArtifacts` liefert je Artefakt seine `NodeID` mit.
- **Nearest-ancestor-wins-Ordnung (KRITISCH):** `ListArtifacts` liefert eine **flache** Liste (newest-first, nicht chain-geordnet). Der Resolver-Builder in `buildDocumentVM` iteriert die **Ahnenkette in `NodeStore.Ancestors`-Reihenfolge (leaf→root)** und trägt je `slug` NUR den ersten Treffer in die `map[slug]ArtifactRef` ein (nächster Vorfahr gewinnt, Spec §6.1). Umsetzung: `chain`-Index-Map (`nodeID → Position`) bilden, je slug das Artefakt mit der kleinsten Position wählen. Test: gleicher Slug an Node UND Vorfahr → Node-Version gewinnt.
- **`artifactEmbedNode struct{ ast.BaseInline; Slug string; N int }`** (`kindArtifactEmbed = ast.NewNodeKind("ArtifactEmbed")`).

**Parser-Trigger (KRITISCH — Bestand-verifiziert):** `wikiLinkParser` liegt bei `util.Prioritized(&wikiLinkParser{}, 100)`; goldmarks Core-`LinkParser` (besitzt `!`+`[`) liegt bei 200 → der `wikiLinkParser` gewinnt für beide Trigger-Bytes. `Trigger()` → `[]byte{'!', '['}`. In `Parse`:
```go
func (wikiLinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	// Embed form ![[slug]] — artifact figure.
	if len(line) >= 5 && line[0] == '!' && line[1] == '[' && line[2] == '[' {
		end := -1
		for i := 3; i+1 < len(line); i++ {
			if line[i] == '\n' { break }
			if line[i] == ']' && line[i+1] == ']' { end = i; break }
		}
		if end < 0 { return nil } // broken ![[ … — let core image/text handle
		slug := string(line[3:end])
		if slug == "" { return nil }
		block.Advance(end + 2)
		return &artifactEmbedNode{Slug: slug}
	}
	// A lone '!' that is not ![[ is not ours — return nil so goldmark's core
	// LinkParser renders ![](url) images / plain '!' text.
	if line[0] == '!' { return nil }
	// … existing [[target]] doc-wikilink logic UNCHANGED …
}
```
**Nummerierung (KRITISCH):** goldmark-AST-Transformer laufen **nach** dem Inline-Parsing → EIN Transformer kann Mermaid-Block-Knoten UND Artefakt-Inline-Knoten in Dokumentreihenfolge nummerieren. Der Transformer **hält den `resolveArtifact`-Resolver** (Codex-Fund #6 + OE #4): `mt := &figureTransformer{resolveArtifact: resolveArtifact}` (RenderDocument hat den Resolver in Scope, gibt ihn dem Transformer mit). `figureTransformer.Transform`: (1) Mermaid-`FencedCodeBlock` → `mermaidNode` swappen (N noch 0, `t.Count` weiter für `HasMermaid`); (2) **danach** ein `ast.Walk` über das ganze Dokument, das in Reihenfolge nummeriert — jeder `*mermaidNode` bekommt `N`; ein `*artifactEmbedNode` bekommt `N` **nur wenn `resolveArtifact(slug)` auflöst** (unaufgelöste Embeds werden NICHT gezählt → keine springenden Nummern bei fehlendem Artefakt, OE #4). Beide Renderer lesen `n.N` nur ab; der Artefakt-Renderer ruft `resolveArtifact` erneut (idempotent) für die eigentliche Figur/den Chip. Test mit **gemischter** Mermaid/Artefakt-Reihenfolge (Abb. 1 Mermaid, Abb. 2 Bild, Abb. 3 Mermaid) **UND** ein unaufgelöster Embed zwischendrin (verbraucht keine Nummer) Pflicht.
**Figur/Chip-Renderer** (`artifactEmbedHTMLRenderer`, priorität 100): `resolveArtifact(slug)` →
- gefunden + `IsImage` → `<figure class="figure"><div class="frame"><img loading="lazy" src="{Href}?v=…" alt="{Name}" width height></div><figcaption><b>{FigLabel} {N}</b> · {Name}</figcaption></figure>` (die Serve-URL mit `?v={ref}` gegen Stale; `FigLabel` = Bestand `document.figure.label`).
- gefunden + Nicht-Bild → `<figure class="figure"><a class="filechip" href="{Href}" download>{Typ-Icon} {Name} · {SizeStr}</a><figcaption><b>{FigLabel} {N}</b> · {Name}</figcaption></figure>` (Download-Link auf die nackte Serve-Route mit `attachment`).
- nicht gefunden → `<span class="wikilink-broken">{slug}</span>` (Muster unaufgelöster Wikilink; **kein** `N`-Verbrauch? — Empfehlung: unaufgelöste Embeds zählen NICHT als Figur; nur aufgelöste bekommen `N`. Der Nummerierungs-Walk braucht dazu die Auflösbarkeit — Alternative in Offene Entscheidung #4).
**Sanitizer (`getDocPolicy` erweitern):** `figure`/`figcaption` sind bereits erlaubt (Bestand :55). Ergänzen: `img` mit `alt`/`loading`/`width`/`height`; `a` mit `download`; `class` auf `img`/`a`; `id` auf `h1..h6` (Task 4 nutzt es mit). **`img src` NICHT über die generische Relativ-URL-Policy** — Custom-Prüfung exakt gegen `^/nodes/[A-Za-z0-9_-]+/artifacts/[a-z0-9-]+(\?v=[0-9a-f]{12})?$`. **Vektor-Klarstellung (agy-Fund #2):** roh eingetipptes `<img>`-HTML im Markdown-Quelltext ist bereits blockiert — `RenderDocument` setzt **kein** `html.WithUnsafe()`, goldmark reicht Roh-HTML nicht durch. Der EINZIGE Weg zu einer externen `img src` ist goldmarks **Core-Image-Parser** (`![alt](url)`). **ACHTUNG bluemonday-OR-Semantik:** `UGCPolicy` erlaubt `img src` bereits (nil-Regexp) — ein zusätzliches `AllowAttrs("src").OnElements("img").Matching(re)` **restringiert nicht** (Attr-Policies sind OR-verknüpft → die permissive UGCPolicy-Policy gewinnt). Deshalb ist der Mechanismus **definiert** (nicht „wahrscheinlich"): **Override von goldmarks Core-Image-Renderer** (`util.Prioritized`-Renderer auf `ast.KindImage`), der `<img>` **nur** für Serve-Route-`src` emittiert (extern/`data:`/`//host` → droppen/leeren), plus die Sanitizer-Regexp als zweite Verteidigungslinie. Die drei **Negativtests Pflicht** (extern `https://evil/x.png`, `data:`-URL, protokoll-relativ `//host/x`) — erst rot, dann grün. **Bestand gewinnt** — die tatsächliche bluemonday-Wirkung am roten Test verifizieren (OE #1).

**Zustände dieser Fläche (Lese-Ebene):** leer (Doc ohne Embeds → keine Figuren, Bestand-Render unverändert); `![[slug]]` aufgelöst-Bild (Figur + `?v=hash`); aufgelöst-Datei (Chip); **nicht gefunden / Doc ungebunden** (ungelöst-Chip, bricht Render nicht); gemischte Mermaid/Artefakt (gemeinsamer Zähler); **lang** (langer Dateiname bricht via `.filechip`-Containment/`truncate`); **mobil 375px** (Figur scrollt im `.frame`, Bild Naturgröße, Seite pannt nie horizontal — Spec §11); Fehlerpfad (kaputtes `![[` → sauberer Text-Fallback via LinkParser, keine Exception).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func \(wikiLinkParser\) Trigger|func \(wikiLinkParser\) Parse|type wikiLinkParser|RenderDocument\(|WikilinkResolver|util.Prioritized\(&wikiLinkParser|getDocPolicy|AllowAttrs|AllowElements\(\"figure" internal/adapter/webui/wikilink.go`; `rg -n "func .*Transform|type mermaidTransformer|type mermaidNode|mermaidHTMLRenderer|t.Count|mn.N|n.N" internal/adapter/webui/mermaid.go`; `rg -n "RenderDocument\(" internal/adapter/httpserver/webui_document.go internal/adapter/httpserver/webui_editor.go`; `rg -n "document.figure.label|document.figure.mermaid|document.figure.source|wikilink-broken|\.frame\b|\.mermaid-figure" internal/i18n/catalog_de.go web/tailwind.css`.
- [ ] **Step 1: Failing Tests** (`wikilink_test.go`): (a) `![](https://x/y.png)` bleibt Bild (Core-Parser, NICHT Embed); (b) `![[bild]]` → Figur mit `<img src="/nodes/.../artifacts/bild?v=...">` + `Abb. N`; (c) kaputte `![[`, `![[x]`, `!text` fallen sauber zurück (kein Panic, kein Embed); (d) **gemischte** Mermaid/Artefakt-Nummerierung (Zähler geteilt, Reihenfolge korrekt); (e) unaufgelöster Slug → `.wikilink-broken`; (f) **Sanitizer-Rejection**: externer Host, `data:`, `//host` als `img src` werden gestrippt/leeren die src (kein externes Bild überlebt); Serve-Route-`src` überlebt; (g) `FindWikilinks`/Backlinks unberührt (bereits Task 1, hier Regression sichern).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Note (agy-Fund #3):** die beiden Editor-Caller (`handleWebEditorPreview`, `renderEditorPreview`) bekommen in DIESEM Task einen `nil`/leeren Resolver (Verhalten: Embeds ungelöst) — der Build sichert die Signaturmigration; der **verhaltensvolle** Editor-Preview-Test (`![[slug]]` löst via Editor-Node auf) ist Pflicht-Testfall in **Task 6** (Spec §13). Hier kein doppelter Test nötig.
- [ ] **Step 3–5: Implementieren** — Parser-Trigger/Parse; `artifact_embed.go` (Node + Renderer + `ArtifactRef`/`ArtifactResolver`); Nummerierungs-Transformer (Mermaid + Artefakt); `RenderDocument`-Signatur + Registrierung `util.Prioritized(&artifactEmbedHTMLRenderer{resolve: resolveArtifact}, 100)`; die 3 Caller anpassen (`buildDocumentVM` baut den Resolver aus `s.ListArtifacts.Execute(owner, *doc.NodeID)` → `map[slug]ArtifactRef` → Closure; nil NodeID → nil-Resolver → alle Embeds ungelöst; die beiden Editor-Caller bekommen in diesem Task einen nil/leeren Resolver, Task 6 verdrahtet den Editor-Node); Sanitizer + benannte CSS-Klassen (`.figure`, `.filechip` — Containment `min-w-0`/`truncate`/`overflow-x:auto` im `.frame`); i18n beide Kataloge; `make generate` + `make web`.
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(webui): ![[slug]]-Artefakt-Embed (Figur/Chip) + gemeinsamer Mermaid/Artefakt-Figurenzähler + Sanitizer-URL-Policy (Serve-Route only)"
```
Expected: PASS; `app.css` geändert.

---

### Task 4: Deep-Links — Heading-Slugger (`WithHeadingAttribute` + AST-Transformer) · Sanitizer `id` auf h1–h6 · `toc.js` · Hover-¶ + „kopiert"-Feedback

**Files:**
- Modify: `internal/adapter/webui/wikilink.go` (in `RenderDocument`: `parser.WithHeadingAttribute()` + Heading-Slugger-Transformer registrieren; `getDocPolicy` `id` auf h1–h6 — falls Task 3 das nicht schon gesetzt hat)
- Create: `internal/adapter/webui/heading_slug.go` (`headingSlugTransformer` + GitHub-Slug-Funktion + Duplikat-Suffix) + `_test.go`
- Modify: `internal/adapter/webui/static/toc.js` (Server-`id` nutzen — bereits `if (!heading.id)`; ggf. Hover-¶-Markup)
- Modify: `internal/adapter/webui/document.templ` (Hover-¶-Anker an Überschriften — falls über CSS `.prose h2::after` nicht ausreichend, minimaler Markup/JS-Weg)
- Modify: `web/tailwind.css` (`.prose`-Heading-Hover-¶-Styling, benannte Klasse)
- Modify: `internal/adapter/webui/static/js/clipboard.js` **nutzen** (Bestand — kein `alert`) für „Link kopiert"
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (`heading.copyLink`/`heading.copied` A11y/Feedback)
- Test: `internal/adapter/webui/heading_slug_test.go`

**Interfaces / Produces:**
- **`headingSlugTransformer`** (AST-Transformer, in der `WithASTTransformers`-Liste): sluggt jede `*ast.Heading` GitHub-Stil (lowercase, Nicht-Alnum→`-`, Umlaute wie `Slugify`; **identisch** zum Slug in geteilten URLs) und hängt bei Duplikaten im selben Dokument `-1/-2/…` an; setzt das `id`-Attribut server-seitig an h1–h6.
- **Sanitizer:** `id` auf `h1..h6` (Task 3/4-koordiniert — genau einmal setzen).
- **`toc.js`:** nutzt die vorhandenen Server-`id`s (`if (!heading.id)` bleibt Fallback für ID-lose Headings). **Kein** Bruch der Bestand-TOC-Tests.
- **Hover-¶:** dezentes „¶" an Überschriften (`#slug`-Link, kopierbar). Rein CSS/Markup + winziges optionales JS für „Link kopiert"-Feedback über die Bestand-`clipboard.js` — **kein** `alert` (`verify-no-popups`).

**Zustände:** leer (Doc ohne Überschriften → keine Anker, TOC versteckt via `data-toc-block`-Bestand); Duplikat-Überschriften (`-1/-2`-Suffix, kollisionsfrei); lang (Überschrift bricht normal, ¶ bleibt am Zeilenende); mobil 375px (¶ tappbar, kein Overflow); Fehlerpfad (Sonderzeichen/leere Überschrift → stabiler Slug/kein leeres `id`).

- [ ] **Step 0: rg-Verifikation** — `rg -n "WithHeadingAttribute|WithParserOptions|WithASTTransformers|parser.With" internal/adapter/webui/wikilink.go`; `rg -n "heading.id|data-toc-nav|data-toc-block|querySelectorAll" internal/adapter/webui/static/toc.js`; `rg -n "clipboard|data-copy|navigator.clipboard" internal/adapter/webui/static/js/clipboard.js`; `rg -n "\.prose h2|\.prose h3|toc-|eyebrow" web/tailwind.css internal/adapter/webui/components/toc.templ`; `rg -n "AllowAttrs\(\"id\"\).OnElements" internal/adapter/webui/wikilink.go` (h1–h6 schon gesetzt?).
- [ ] **Step 1: Failing Test** (`heading_slug_test.go`): `## Mein Über-Titel` → `id="mein-ueber-titel"`; zwei gleiche Überschriften → `-1`; Sanitizer erhält `id` an h2; ein `RenderDocument`-Roundtrip zeigt die `id`s. **Hover-¶ (Codex-Fund #7):** die gerenderte Überschrift enthält den Anker `<a class="head-anchor" href="#mein-ueber-titel">` (bzw. das gewählte Markup aus OE #5) und der **Sanitizer behält ihn** (nach `SanitizeBytes` noch vorhanden); der Anker-Text ist `¶` (monospace-Glyph, keine Emoji). `toc.js`-Verhalten wird durch die Render-Tests indirekt gesichert (Server-`id` vorhanden). **Popup-Freiheit:** die „kopiert"-Rückmeldung nutzt die Bestand-`clipboard.js` — `make ci`/`verify-no-popups` erzwingt kein `alert/confirm/prompt` (kein neues Popup-JS).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3–5: Implementieren** — `heading_slug.go` (Slugger + Duplikat-Map); `WithHeadingAttribute()` + Transformer in `RenderDocument`; Sanitizer `id` auf h1–h6 (mit Task 3 koordiniert); Hover-¶ (`.prose`-Heading-CSS + optional `<a class="head-anchor" href="#slug">¶</a>` — Empfehlung: server-seitig im Slugger als Kind-Node ODER rein CSS `::after`; Offene Entscheidung #5); „kopiert"-Feedback via Bestand-`clipboard.js`; i18n beide Kataloge; `make generate` + `make web`.
- [ ] **Step 6: Bauen + Test + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(webui): Überschriften-Deep-Links (server-seitige GitHub-Slug-ids, Duplikat-Suffix) + Hover-¶ + toc.js nutzt Server-ids"
```
Expected: PASS; `app.css` geändert.

---

### Task 5: Cockpit-Galerie — `CockpitArtifacts` + SSE-Container · multipart-Upload · Umbenennen · Löschen (ConfirmDialog) · eigene + geerbte · i18n

**Files:**
- Create: `internal/adapter/webui/cockpit_artifacts.templ` (`CockpitArtifacts(d NodeCockpit)` — Grid + Upload-Form + Umbenennen/Löschen) + `_render_test.go`
- Create: `internal/adapter/webui/cockpit_artifacts_vm.go` (`ArtifactCardVM` + Builder aus `[]domain.Artifact`, „eigene vs. geerbt"-Markierung gegen die Cockpit-Node-ID)
- Modify: `internal/adapter/webui/cockpit_vm.go` (`NodeCockpit.Artifacts []ArtifactCardVM`)
- Modify: `internal/adapter/webui/cockpit.templ` (neuer SSE-Container `#cockpit-artifacts`, `hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`)
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData`: `d.Artifacts` via `s.ListArtifacts.Execute`; Fragment-Render-Handler für den SSE-Container)
- Create: `internal/adapter/httpserver/webui_artifacts.go` (Web-Handler: multipart-Upload, Umbenennen, Löschen — je eigene Route)
- Modify: `internal/adapter/httpserver/server.go` (Web-Routen `s.webAuth`)
- Modify: `web/tailwind.css` (`.gallery`/`.artcard`-Grid + Thumb + Chip, benannte Klassen)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/httpserver/webui_artifacts_test.go`, `internal/adapter/webui/cockpit_artifacts_render_test.go`

**Interfaces / Produces:**
- **`ArtifactCardVM struct{ Slug, Name, Href, SizeStr, TypeLabel string; IsImage, Inherited bool; FromNode string }`** — `Inherited` = Artefakt-`node_id` ≠ Cockpit-Node-ID (dann Herkunft `FromNode` markieren, read-only; Upload/Umbenennen/Löschen nur auf eigene). **Nachfahren-Artefakte gehören nicht hierher** (`ListArtifacts` liefert nur Ahnenkette — automatisch erfüllt).
- **Web-Routen** (`s.webAuth`; **die Handler emittieren NICHT — die Usecases tun es**, Codex-Fund #3):
  - `POST /nodes/{id}/artifacts` (multipart) → `readValidatedArtifact`-analog (`http.MaxBytesReader(w, r.Body, MaxArtifactBytes+64*1024)`, Muster `webui_nodes.go`) → `UploadArtifact.Execute` (leerer `replaceSlug` = neu); Fehler **inline i18n** (kein Popup); rendert `#cockpit-artifacts`-Fragment. `UploadArtifact` emittiert `artifact.created`.
  - **Re-Upload/Ersetzen (Codex-Fund #4, Spec §3/§12):** dieselbe Route mit einem `slug`-Formfeld (die Galerie-Karte hat eine „Ersetzen"-Affordanz, die den existierenden Slug mitschickt) → `UploadArtifact.Execute(replaceSlug=slug)` überschreibt (neuer `ref`, `bytes`), `UploadArtifact` emittiert `artifact.updated`. (Kleine, in-scope Affordanz; wenn Scope gekürzt werden muss, ist die „Ersetzen"-UI der cuttbare Teil — der Usecase-Pfad bleibt.)
  - `POST /nodes/{id}/artifacts/{slug}/rename` → **`RenameArtifact.Execute`** (Task 2 Usecase, NICHT Store-Direktaufruf — Codex-Fund #2; **Slug UND `ref`/`bytes` bleiben stabil**; nur auf **eigene** Artefakte, der Usecase bestätigt das via `GetMeta` an DIESEM Node). `RenameArtifact` emittiert `artifact.updated`. (OE #6.)
  - `POST /nodes/{id}/artifacts/{slug}/delete` (ConfirmDialog `data-dialog-open`, **kein `confirm()`**) → `DeleteArtifact.Execute`; rendert Fragment. `DeleteArtifact` emittiert `artifact.deleted`.
- **`CockpitArtifacts(d)`** als eigener SSE-Container im Cockpit (Platzierung = Offene Entscheidung #2 — Empfehlung: eigener voll­breiter Block unter dem `.cock`-Grid; Alternative Rail-Block).

**Zustände dieser Fläche:** leer (Node ohne Artefakte + keine geerbten → ruhiger Empty-State „Noch keine Artefakte", Upload-Form sichtbar); geerbt (Karte mit Herkunfts-Marke, read-only); eigenes Bild (Thumbnail); eigenes Nicht-Bild (Datei-Chip mit Typ-Icon); **lang** (langer Dateiname `truncate`/`min-w-0`); **mobil 375px** (Grid kollabiert auf 1–2 Spalten, kein horizontales Pannen — Spec §11); Fehlerpfad (Upload zu groß/falscher Typ/Quota → **inline** i18n-Meldung, kein Popup; Kollision → `-1`-Suffix serverseitig).

- [ ] **Step 0: rg-Verifikation** — `rg -n "templ CockpitRailBlocks|type NodeCockpit|nodeCockpitData|CockpitRailBlocks\(d\).Render|id=\"cockpit-|hx-trigger=\"sse:" internal/adapter/webui/cockpit.templ internal/adapter/webui/cockpit_rail.templ internal/adapter/webui/cockpit_vm.go internal/adapter/httpserver/webui_cockpit.go`; `rg -n "MaxBytesReader|readValidatedLogo|ParseMultipartForm|ConfirmDialog|data-dialog-open" internal/adapter/httpserver/webui_nodes.go internal/adapter/webui/components/*.templ`; `rg -n "\.blk|\.krow|\.btn|\.panel|\.eyebrow|\.frame" web/tailwind.css | head`; `rg -n "ListArtifacts\s+usecase|Emitter\s+ports|s.ListArtifacts" internal/adapter/httpserver/server.go`.
- [ ] **Step 1: Failing Tests** — `cockpit_artifacts_render_test.go`: Grid rendert eigene (mit Löschen/Umbenennen) + geerbte (read-only, Herkunft) Karten; Bild → Thumb, PDF → Chip; leer → Empty-State + Upload-Form. `webui_artifacts_test.go`: multipart-Upload → 200 + Fragment + `artifact.created`; Umbenennen → `name` geändert, `ref`/`slug` stabil, `artifact.updated`; Löschen → `artifact.deleted`; **Owner-Scope** (fremder Node → kein Effekt); zu groß/Quota → inline-Fehler (kein 500, kein Popup).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3–5: Implementieren** — VM + Builder; `cockpit_artifacts.templ` (benannte Klassen); SSE-Container in `cockpit.templ`; `nodeCockpitData` füllt `d.Artifacts` (via `s.ListArtifacts`) + Fragment-Handler; Web-Handler (Upload/Rename/Delete) + Routen; ConfirmDialog fürs Löschen; i18n beide Kataloge; `make generate` + `make web`.
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Cockpit-Artefakt-Galerie (Upload/Umbenennen/Löschen, eigene+geerbt) + #cockpit-artifacts SSE-Container (artifact.*)"
```
Expected: PASS; `app.css` geändert.

---

### Task 6: Editor-Einfügehelfer — generischer Picker · `/ui/editor/artefakte` + `/ui/editor/seiten` · `editor-insert.js` · Werkzeugleiste · Preview-Node-Kontext · i18n

**Files:**
- Create: `internal/adapter/webui/components/insertpicker.templ` (neue **generische** Picker-Variante ohne Glyph/Rate/„Neu anlegen", named-class; Muster-Mechanik `ProjectFuzzyPicker`, aber Lesesaal-Optik)
- Create: `internal/adapter/webui/static/js/editor-insert.js` (Einfügen am `selectionStart`, Cursor dahinter, Live-Vorschau triggern)
- Modify: `internal/adapter/webui/editor.templ` (Werkzeugleiste mit zwei Buttons **über** der `<textarea name="body">`; Node-Kontext in die Preview reichen — `hx-include`/hidden `node`)
- Modify: `internal/adapter/webui/editor_vm.go` (falls Picker-Daten/NodeID an die Toolbar müssen)
- Create: `internal/adapter/httpserver/webui_editor_pickers.go` (`GET /ui/editor/artefakte?node={id}` + `GET /ui/editor/seiten` Fragment-Handler)
- Modify: `internal/adapter/httpserver/webui_editor.go` (`handleWebEditorPreview` + `renderEditorPreview`: Artefakt-Resolver aus dem Editor-Node — Task 3 ließ ihn leer)
- Modify: `internal/adapter/httpserver/server.go` (zwei Picker-Routen `s.webAuth`)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/httpserver/webui_editor_pickers_test.go`, `internal/adapter/httpserver/webui_editor_test.go` (Preview rendert `![[slug]]`)

**Interfaces / Produces:**
- **„Artefakt einfügen ⋯"** (Button, `.btn .btn-q .btn-s`) → `GET /ui/editor/artefakte?node={id}` listet die Artefakte des Editor-Kontext-Nodes + Vorfahren (**Ahnenkette**, via `ListArtifacts`) im Picker → Auswahl fügt `![[slug]]` am `selectionStart` ein.
- **„Seite verlinken ⋯"** (Button) → `GET /ui/editor/seiten` listet Dokumente (fuzzy/MRU, Muster ⌘K-Palette) → Auswahl fügt `[[pfad]]` ein.
- **`editor-insert.js`:** am `selectionStart` einsetzen, Cursor dahinter, Live-Vorschau (`/wissen/preview`) neu triggern. Kein Popup.
- **Preview-Node-Kontext (KRITISCH):** `POST /wissen/preview` muss den Editor-Node kennen, damit `![[slug]]` in der Vorschau auflöst (Spec §13 Pflicht-Testfall). Konkret: die `<textarea>` bekommt `hx-include="[name=projectId], [name=node-ctx]"` bzw. ein hidden `node`-Feld (im Edit-Modus, wo `projectId` disabled ist, ein hidden `node`); `handleWebEditorPreview` liest `node`, baut den Artefakt-Resolver via `s.ListArtifacts.Execute(owner, node)` und reicht ihn in `RenderDocument`. Für ein Doc ohne Node → leerer Resolver (Embeds ungelöst).

**Zustände dieser Fläche:** leer (Node ohne Artefakte → Picker zeigt „keine Artefakte"; Seiten-Picker zeigt „keine Dokumente"); viele (Fuzzy-Filter greift, scrollbar); lang (langer Titel/Slug `truncate`); **mobil 375px** (Picker-Dropdown `max-width` an Viewport, kein Overflow); Fehlerpfad (Auswahl bei leerer Textarea → am Anfang einfügen; kein Node → leerer Artefakt-Picker, kein 500). **Kein** laufender Timer relevant.

- [ ] **Step 0: rg-Verifikation** — `rg -n "templ ProjectFuzzyPicker|type NodePickerVM|data-fuzzy-picker|data-fuzzy-filter|pick-row" internal/adapter/webui/components/fuzzypicker.templ`; `rg -n "textarea name=\"body\"|hx-post=\"/wissen/preview\"|hx-include|/wissen/preview" internal/adapter/webui/editor.templ`; `rg -n "handleWebEditorPreview|renderEditorPreview|RenderDocument\(|FormValue\(\"body\"\)|FormValue\(\"node\"\)" internal/adapter/httpserver/webui_editor.go`; `rg -n "palette|data-palette|MRU|NodeMRU|selectionStart" internal/adapter/webui/static/js/palette.js internal/adapter/webui`; `rg -n "type EditorVM|NodeID\b" internal/adapter/webui/editor_vm.go`.
- [ ] **Step 1: Failing Tests** — `webui_editor_pickers_test.go`: `GET /ui/editor/artefakte?node={id}` listet Ahnenketten-Artefakte (**Owner-Scope: fremder Node → leer/404**); `GET /ui/editor/seiten` listet Docs (**Owner-Scope-Negativtest (Codex-Fund #8): ein Doc eines fremden Owners erscheint NICHT** — `DocumentStore` ist owner-scoped). `webui_editor_test.go`: `POST /wissen/preview` mit `body="![[bild]]"` + `node={id}` (Artefakt am Node) → Vorschau enthält die Figur/`<img src="/nodes/.../artifacts/bild?v=...">` (Spec §13 Pflicht); ohne Node → ungelöst-Chip. **`editor-insert.js` (Codex-Fund #8):** das JS-Einfügen am `selectionStart`/Cursor/Preview-Trigger ist Client-JS (nicht Go-unit-testbar) → im **Live-Dogfood (Task 8)** verifiziert; hier wird nur asserted, dass die Toolbar-Buttons + `AssetURL("js/editor-insert.js")` im gerenderten Editor vorhanden sind und die Picker-Fragmente die erwartbaren `data-*`-Insert-Attribute (Slug/Pfad) tragen.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3–5: Implementieren** — generischer `insertpicker.templ` (named-class, Fuzzy-Mechanik wie `data-fuzzy-picker`); `editor-insert.js`; Toolbar-Buttons über der Textarea; Preview-Node-Kontext (hidden `node` + `hx-include` + Handler-Resolver); Picker-Fragment-Handler + Routen; i18n beide Kataloge; `make generate` + `make web`.
- [ ] **Step 6: Bauen + Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Editor-Einfügehelfer (Artefakt- + Seiten-Picker, editor-insert.js) + Preview löst ![[slug]] via Editor-Node"
```
Expected: PASS; `app.css` geändert.

---

### Task 7: flow-mcp (3 Tools) + CLI (`flow artifact add/ls/rm`)

**Files:**
- Create: `cmd/flow-mcp/tools_artifacts.go` (drei Tool-Handler) + `_test.go`
- Modify: `cmd/flow-mcp/server.go` (drei `mcp.AddTool`-Registrierungen)
- Create: `cmd/flow/artifact.go` (Cobra `artifact add/ls/rm`) + `_test.go`
- Modify: `cmd/flow/main.go` (Command registrieren — per rg das Registrierungsmuster verifizieren)

**Interfaces / Produces:**
- **flow-mcp:** `flow_upload_artifact(node, name, mime, base64)` · `flow_list_artifacts(node)` · `flow_delete_artifact(node, slug)`. Node-Auflösung wie Bestand-Tools (`h.resolveScope`/Slug/Name/Binding); `h.do(...)` → `c.UploadArtifact`/`c.ListArtifacts`/`c.DeleteArtifact` (apiclient aus Task 2); `textResult`/`errorResult`. Actor-Kind `agent` (der MCP-Client ist ein Agent — der Server stempelt `CreatedByKind` aus dem Request-Actor).
- **CLI:** `flow artifact add <datei> [--node] · ls [--node] · rm <slug> [--node]` (dünner apiclient; `--node` via `resolveNodeRef`-Muster / Projektauflösung; `add` liest die Datei, sniff-freier Upload — der Server validiert). Muster `cmd/flow/docs.go`/`node.go`.

**Zustände:** leer (`ls` ohne Artefakte → „keine Artefakte"); Fehlerpfad (`add` mit unlesbarer Datei / zu groß → klare Fehlermeldung; `rm` unbekannter Slug → Fehler); **Owner-Scope = verbindlicher Test (Gemini-Fund #6)** — ein Stub-Server, der für einen fremden/unbekannten Node **404** liefert, muss vom MCP-Tool UND vom CLI-Verb als Fehler durchgereicht werden (kein stiller Erfolg). **Kein** UI-Zustand (CLI/MCP sind textuell).

- [ ] **Step 0: rg-Verifikation** — `rg -n "mcp.AddTool|func \(h \*handlers\) createDoc|h.resolveScope|errorResult|textResult|h.do\(ctx" cmd/flow-mcp/server.go cmd/flow-mcp/tools_write.go cmd/flow-mcp/scope.go`; `rg -n "func newArtifactCmd|cobra.Command|resolveNodeRef|StringVar|c.CreateDocument|rootCmd.AddCommand|newDocsCmd" cmd/flow/main.go cmd/flow/docs.go cmd/flow/node.go cmd/flow/noderef.go`; `rg -n "func \(c \*Client\) UploadArtifact|ListArtifacts|DeleteArtifact" internal/adapter/apiclient/artifacts.go`.
- [ ] **Step 1: Failing Tests** — `cmd/flow-mcp/tools_artifacts_test.go` (Stub-apiclient/Server, Muster `write_test.go`): upload/list/delete rufen die richtigen apiclient-Verben, Node-Auflösung greift, Fehler → `errorResult`; **Owner-Scope-Negativtest** (Stub-Server 404 für fremden Node → Tool meldet Fehler, kein stiller Erfolg — Gemini-Fund #6). `cmd/flow/artifact_test.go` (Muster `node_subcommands_test.go`): `add`/`ls`/`rm` gegen Stub-Server; **Owner-Scope-Negativtest** (Stub 404 → CLI-Verb meldet Fehler mit Exit≠0).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3–4: Implementieren** — MCP-Tools + Registrierung; CLI-Command + Registrierung.
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
git add -A && go test ./cmd/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(mcp,cli): flow_upload/list/delete_artifact + flow artifact add/ls/rm (Agenten + Menschen laden Artefakte, generisch für alle Hosts)"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig.

---

### Task 8: Wiring-Gate — Composition-Root · Rest-Sweep · `make ci` · Live-Dogfood · Breakpoints

**Files:** i. d. R. keine neuen (Verifikation + evtl. Sweep-Fixes + Ledger).

- [ ] **Step 1: Composition-Root-Verifikation** (`cmd/flow-server/main.go` + `server.go` + `cmd/flow-mcp/server.go` + `cmd/flow/main.go`)
```bash
rg -n "artifactStore :?=|NewArtifactStore|UploadArtifact:|RenameArtifact:|ListArtifacts:|DeleteArtifact:|GetArtifact:|Emitter: emitter" cmd/flow-server/main.go
rg -n "UploadArtifact\s+usecase|RenameArtifact\s+usecase|ListArtifacts\s+usecase|DeleteArtifact\s+usecase|GetArtifact\s+usecase" internal/adapter/httpserver/server.go
# REST + Serve + Web-Galerie (Upload/Rename/Delete, Gemini-Fund #7) + Editor-Picker:
rg -n "POST /api/v1/nodes/\{id\}/artifacts|GET /api/v1/nodes/\{id\}/artifacts|DELETE /api/v1/nodes/\{id\}/artifacts/\{slug\}|GET /nodes/\{id\}/artifacts/\{slug\}|POST /nodes/\{id\}/artifacts\b|POST /nodes/\{id\}/artifacts/\{slug\}/rename|POST /nodes/\{id\}/artifacts/\{slug\}/delete|/ui/editor/artefakte|/ui/editor/seiten" internal/adapter/httpserver/server.go
rg -n "flow_upload_artifact|flow_list_artifacts|flow_delete_artifact" cmd/flow-mcp/server.go
rg -n "artifact" cmd/flow/main.go
```
Erwartet: `ArtifactStore` gebaut; die **fünf** Usecases im Server-Literal (drei mit `Emitter: emitter`); **alle** REST/Serve/Web-Galerie-Mutations-/Editor-Picker-Routen registriert (inkl. `POST /nodes/{id}/artifacts`, `/rename`, `/delete` — Gemini-Fund #7); drei MCP-Tools; CLI-Command verdrahtet. **Kein „ship a usecase nothing calls".**
- [ ] **Step 2: Rest-Sweep** — Dispatch-Text oben über `git diff --name-only rebuild..HEAD`. Gefundene tote Keys/Arbitrary-Values/verwaiste Symbole/Wiring-Lücken fixen.
- [ ] **Step 3: Tote i18n-Keys** — die neuen `artifact.*`/`figure.*`/`document.figure.*`/`editor.insert.*`/`heading.*`-Keys gegen `T(`/`Tn(`-Nutzung prüfen; keine verwaisten; de+en-Parität.
- [ ] **Step 4: Volles CI**
```bash
git add -A
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST=Podman-Socket
```
(erst stagen, dann ci — L4-Lehre; nie zwei ci parallel; `timeout: 600000`.)
- [ ] **Step 5: Live-Dogfood** (Dev-Stack; Cookie-Flow wie L1–L5.5-Gate) — Spec §13:
```bash
make dev-run &   # https://localhost:8080 (self-signed); danach stoppen
sleep 2
# Migration 0031 angewandt? (artifacts-Tabelle + unique + Index)
# Web-Galerie: Upload Bild (PNG) + Datei (PDF) an einem Node → Karten erscheinen (SSE artifact.created)
# In Doc am selben/Kind-Node: ![[bild]] + ![[pdf]] → Figur (img) + Datei-Chip, Abb.-Nummern korrekt & mit Mermaid geteilt
# Deep-Link: #<heading-slug> scrollt zur Sektion; Hover-¶ kopiert den Link (kein Popup)
# Editor: "Artefakt einfügen" fügt ![[slug]] ein; "Seite verlinken" fügt [[pfad]] ein; Live-Vorschau rendert die Figur
# REST: POST /api/v1/nodes/{id}/artifacts {name,mime,dataBase64} (Bearer) → 201; GET Liste (Ahnenkette); Serve ?v=ref → immutable, nackt → no-cache, If-None-Match → 304
# Quota: künstliche Grenze/kleiner MaxArtifactBytesPerOwner → 413/inline-Fehler
# MCP: flow_upload_artifact eines Agenten → landet in der Galerie (CreatedByKind=agent)
# Owner-fremd: anderer Owner GET/Serve/DELETE → 404/nichts
```
Expected: Upload → SSE-Galerie-Reload; Embeds rendern als nummerierte Figuren; Deep-Links scrollen; Editor-Picker fügen beide Token ein; MCP-Upload sichtbar; owner-fremder Zugriff scheitert. Danach Server stoppen.
- [ ] **Step 6: Breakpoint-Sichtprobe für Soenne notieren** — **≤960px** (Galerie-Grid stackt; Doc `.narrow`) und **375px** (Figur/Chip pannen NICHT horizontal; `.filechip`/`.artcard` `truncate`; Picker-Dropdown im Viewport; Hover-¶ tappbar).
- [ ] **Step 7: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L6-Gate — Composition-Root-Verify + Sweep + Live-Dogfood (Artefakte/Figuren/Deep-Links/Editor-Picker)"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Alternativpfade. Entscheidung am Ausführungsstart.

1. **Externe Markdown-Bilder `![](https://…)` weiter erlauben, oder alle `<img src>` auf die Artefakt-Serve-Route beschränken?** — *Empfehlung: beschränken* (Spec §8: Custom-URL-Prüfung exakt aufs Route-Muster, Negativtests extern/`data:`/`//host`). Artefakte sind der sanktionierte Bildweg; externe Hotlinks sind ein Privacy/Sicherheits-Smell und der Sanitizer kann Artefakt-`<img>` nicht von `![](url)`-`<img>` unterscheiden. **Trade-off:** Bestand-Docs mit externen Bild-URLs verlieren die Anzeige (werden zu leerer/gestrippter `src`). **Alternative:** zusätzlich http/https erlauben — schwächer, verwässert die Policy. Empfehlung: beschränken; der failing-Negativtest treibt den Mechanismus (Renderer-Override, da bluemonday-OR-Semantik die reine Regexp-Policy aushebelt — Task 3).

2. **Platzierung der Cockpit-Galerie: eigener vollbreiter SSE-Block unter dem `.cock`-Grid, oder Rail-Block in `CockpitRailBlocks`?** — *Empfehlung: eigener vollbreiter Block* (dedizierter `#cockpit-artifacts`-Container, Spec §10/§12) — ein Thumbnail-Grid + Upload-Form ist für die schmale Rail zu breit; ein eigener Block gibt der Galerie Raum und einen sauberen SSE-Consumer. **Trade-off:** ein weiterer Cockpit-Abschnitt. **Alternative:** Rail-Block (kompakter, aber Grid gequetscht, Upload-Form eng). Empfehlung: vollbreiter Block.

3. **Datei-Chip-Typ-Icon: monospace-Glyph oder SVG?** — *Empfehlung: monospace-Glyph/Kürzel* (z. B. `▤ PDF`, `▤ CSV` — Kürzel aus dem MIME) konsistent mit der No-Emoji-Regel und der Lesesaal-Ruhe. **Trade-off:** weniger schick als bunte Datei-Icons. **Alternative:** ein schlichtes monochromes SVG pro Typ (erlaubt lt. Regel „+ SVG"). Empfehlung: Glyph/Kürzel zuerst; SVG als spätere Politur.

4. **Zählen unaufgelöste `![[slug]]`-Embeds als Figur (`Abb. N`) mit?** — *Empfehlung: NEIN* — nur aufgelöste Artefakte bekommen eine Nummer; ein unaufgelöster Embed wird ein ruhiger `.wikilink-broken`-Chip ohne `Abb. N` (sonst „springt" die Nummerierung, wenn ein Artefakt fehlt/umbenannt wird). **Trade-off:** der Nummerierungs-Walk muss die Auflösbarkeit kennen (der Resolver muss vor/während der Nummerierung greifen — konkret: der Renderer bestimmt `resolved`; der Zähl-Transformer läuft aber vor dem Rendern). Umsetzung: der Zähl-Walk zählt nur Embeds, deren Slug der (dem Transformer bekannt gemachte) Resolver auflöst — d. h. der Resolver wird dem Transformer mitgegeben. **Alternative:** alle Embeds zählen (einfacher, aber springende Nummern bei fehlendem Artefakt). Empfehlung: nur aufgelöste zählen; wenn der Aufwand zu groß wird, ist „alle zählen" der akzeptable Fallback (dann im Ledger vermerken).

5. **Hover-¶-Anker: server-seitig als Kind-Node der Überschrift, rein CSS `::after`, oder client-JS?** — *Empfehlung: server-seitiger `<a class="head-anchor" href="#slug">` als letztes Kind der Überschrift* (im Slugger-Transformer erzeugt) + CSS `opacity:0` → `:hover opacity:1` + `clipboard.js` fürs Kopieren. Robust, kein Layout-Sprung, funktioniert ohne JS (Link), Feedback optional. **Trade-off:** Sanitizer muss `a.head-anchor` in Überschriften erlauben (bereits `a`+`class`+`href` erlaubt). **Alternative:** rein CSS `::after` (kein echter Link, nicht kopierbar) oder reines client-JS (baut die Anker nachträglich — fragiler). Empfehlung: server-seitiger Anker.

6. **Umbenennen: neue `ArtifactStore.Rename`-Methode oder `GetMeta`+`Put`?** — *Empfehlung: schlanke `Rename(ctx,owner,node,slug,name)`-Store-Methode* (`UPDATE artifacts SET name=$…, updated_at=$… WHERE …`) — vermeidet das Nachladen der bytes nur um den Namen zu ändern und lässt `ref`/`bytes`/`slug` garantiert unberührt. **Trade-off:** eine Interface-Methode mehr (alle Fakes ziehen mit). **Alternative:** `Get`(bytes)+`Put` — teurer (Blob-Roundtrip), riskiert versehentliche `ref`-Änderung. Empfehlung: `Rename`. (Falls gewählt: in Task 1/5 die Methode ergänzen; sonst Task 5 nutzt `GetMeta`+ein `Put`, das bytes NICHT überschreibt — dann braucht `Put` einen bytes-erhaltenden Pfad, unschöner.) → **Der Plan nimmt `Rename` an; wird sie abgelehnt, wandert die Store-Methode aus Task 5 nach Task 1.**

7. **REST-Body für Upload strikt JSON-only, oder auch multipart auf der REST-Route?** — *Empfehlung: JSON-only auf REST, multipart nur Web-Galerie* (Spec §11, `apiclient.do()` ist durchgehend JSON) — hält den Client dünn und konsistent. **Trade-off:** große Blobs als Base64 (~33 % Overhead, vom 12-MiB-`MaxBytesReader` gedeckt). **Alternative:** multipart auch auf REST (spart Base64) — bricht die JSON-Client-Konvention, doppelter Parse-Pfad. Abgelehnt (Spec-bindend).

8. **Slug-Bildung aus dem Dateinamen — Endung abtrennen?** — *Empfehlung: JA* (`bild.png` → Slug `bild`, nicht `bild-png`) — lesbarere `![[bild]]`-Referenzen; Kollisionen (`bild.png` + `bild.jpg`) lösen sich über den `-1`-Suffix. **Trade-off:** zwei Dateien mit gleichem Stamm kollidieren im Slug (dann `-1`). **Alternative:** Endung im Slug behalten (`bild-png`) — eindeutiger, aber hässliche Referenzen. Empfehlung: Endung abtrennen.

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads (kanonisch).** Vollständig gelesen und für jede verwendete Signatur direkt am Code verifiziert: der L5.5-Plan (Formatvorbild, alle 6 Tasks + OE + Self-Review), AGENTS.md, die L6-Spec (Rev. 2026-07-09), sowie **`internal/domain/nodelogo.go`** (NodeLogo-Struct), **`internal/usecase/upload_node_logo.go`** (`ValidateNodeLogo`/`MaxNodeLogoBytes`/`sha256[:12]`/Struct+Execute/Image-Decoder-Imports), **`internal/adapter/pgstore/nodelogos.go`** (`Put` ON CONFLICT/`Get`→Sentinel/bytea), **`internal/adapter/httpserver/webui_nodelogo.go`** (Serve-Header/ETag/304/`userFrom`), **`internal/ports/ports.go`** (`NodeLogoStore`, `NodeStore.Ancestors` leaf→root, `ListForContext`, `Emitter`, `Clock`/`IDGen`, `ErrNodeLogoNotFound`), Migration **0026/0027** (DDL-Muster), **`internal/domain/document.go`** (`SlugOK`/`slugRe`, Document-Struct), **`internal/usecase/create_node.go`** (`Slugify`/`nonSlug`/`deUmlauts`), **`internal/domain/wikilink.go`** (`FindWikilinks`/`WikilinkTargets`/`WikilinkSpan`/`ResolveWikilink`), **`internal/domain/errors.go`** (Sentinels), **`internal/domain/event.go`** (Event-Konstanten), **`internal/adapter/webui/wikilink.go`** KOMPLETT (`getDocPolicy`/`RenderDocument`/`wikiLinkParser`/`Trigger`/`Parse`/`wikiLinkHTMLRenderer`/goldmark-Verdrahtung/Prioritäten), **`internal/adapter/webui/mermaid.go`** KOMPLETT (`mermaidTransformer`/`mermaidNode`/`mermaidHTMLRenderer`/Nummerierung), **`internal/adapter/httpserver/webui_document.go`** (`buildDocumentVM`/`buildOutgoingRefs`/`isContextType`/`handleWebDocPin`/`handleWebDocMode`/Ancestors-Nutzung), **`internal/adapter/httpserver/webui_editor.go`** (Preview-Handler/`renderEditorPreview`/`editorVM`/3 `RenderDocument`-Caller), **`internal/adapter/webui/editor.templ`** (Textarea/Preview-htmx), **`internal/adapter/webui/components/fuzzypicker.templ`** (`ProjectFuzzyPicker`/Arbitrary-Werte-Befund), **`internal/adapter/webui/static/toc.js`** (Server-`id`-Fallback), **`internal/adapter/webui/cockpit.templ`**+**`cockpit_rail.templ`**+**`cockpit_vm.go`** (3 SSE-Container, `NodeCockpit.Ancestors`, `CockpitRailBlocks`), **`internal/adapter/apiclient/nodes.go`+`documents.go`** (`c.do`-Muster), **`cmd/flow-mcp/server.go`+`tools_write.go`+`resolve.go`** (`mcp.AddTool`/`h.do`/`h.resolveScope`/`textResult`), **`cmd/flow/noderef.go`+`node.go`** (`resolveNodeRef`/Cobra), **`cmd/flow-server/main.go`** (Store+Usecase-Wiring-Block, `nodeLogoStore`/`emitter`), **`internal/adapter/httpserver/server.go`** (Server-Struct-Felder + Routen-Muster `s.auth`/`s.webAuth`), **`internal/i18n/catalog_de.go`** (map-Struktur + `TestCatalogsParity`), **`web/tailwind.css`** (benannte Klassen `.seg`/`.field`/`.panel`/`.blk`/`.krow`/`.meter`/`.pin`/`.btn*`/`.more`/`.frame`/`.mermaid-figure`/`.narrow`/`.prose`/`.eyebrow`, Tokens). Dossier in Scratch (`l6-dossier.md`).
- **agy-Dossier (Phase-1-Vorstufe):** ein `gemini-bigcontext`/`agy`-Dossier-Auftrag über dieselben Präzedenz-Dateien lief parallel als Cross-Check; die First-Hand-Reads sind kanonisch (jede verwendete Signatur direkt am Code verifiziert). **Kein Abbruch, kein Degradations-Modus.**
- **Flow-Recall:** L6-Kontext aus dem Dispatch + Memory (`project_flow_rebuild_lesesaal_l6` — Spec APPROVED, Plan pending; L5.5 gemergt `db8b0a5`).

### Spec-Deckung — jeder Abschnitt auf einen Task gemappt
- **§4 Datenmodell (Migr 0031 bytea, `domain.Artifact`/Slug/MIME/`Validate`/`IsImage`, GIF-Decoder)** → Task 1 (Domain+Migration) + Task 2 (`ValidateArtifactBytes`+GIF-Import im Usecase, Muster `ValidateNodeLogo`).
- **§5 Storage+Serving (`ArtifactStore`-Interface, pgstore, Serve-Route, Cache-Split, ETag/304, `nosniff`, Disposition)** → Task 1 (Store+Port) + Task 2 (Serve-Route + Cache-Split).
- **§6 Referenz+Rendering (`![[…]]`-Trigger, Ahnenkette-Auflösung, Figur/Chip, gemeinsame Nummerierung, ungelöst-Chip, Sanitizer-Reihenfolge)** → Task 3.
- **§7 Deep-Links (`WithHeadingAttribute`+Slugger, `id` auf h1–h6, `toc.js`, Hover-¶)** → Task 4.
- **§8 Sicherheit/Sanitizer (SVG-Verbot, `getDocPolicy` figure/img/a-download, Custom-`img src`-Policy, Negativtests, `nosniff`)** → Task 1 (SVG-Verbot in `Validate`) + Task 3 (Sanitizer + Negativtests) + Task 2 (`nosniff` auf Serve).
- **§9 Editor-Einfügehelfer (zwei Picker, `/ui/editor/artefakte`+`/seiten`, `editor-insert.js`, Live-Vorschau)** → Task 6.
- **§10 Cockpit-Galerie (Grid, Upload, Umbenennen, Löschen, eigene+geerbt, Kollision `-1`)** → Task 5.
- **§11 REST·MCP·CLI (JSON-Upload, List/Delete, Ownership-Guard, Body-Limit, `flow-mcp` 3 Tools, `flow artifact` 3 Verben)** → Task 2 (REST+apiclient) + Task 7 (MCP+CLI).
- **§12 SSE (`artifact.created/updated/deleted`, Galerie-Consumer, Doc bleibt an `document.updated`)** → Task 2 (Events) + Task 5 (Consumer-Container).
- **§13 Querschnitt/Done-Gate (Multi-Tenant-Negativtests, Owner-Quota, Pflicht-Testfälle, goose, i18n, `verify-no-popups`, main.go-Wiring, TDD, Live-Dogfood)** → über alle Tasks verteilt (Negativtests je Task; Quota T2; Pflicht-Testfälle T1–T3; Wiring+Dogfood T8).
- **§14 Datei-Änderungs-Karte** → 1:1 auf die Task-`Files`-Blöcke gemappt.
- **§15 Reihenfolge (8 Schritte, Blätter zuerst, finaler Task = Wiring+Gate)** → Tasks 1–8 identisch geschnitten.

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Anforderung ohne Task:** keine (Mapping oben vollständig; jeder §-Absatz auf ≥1 Task).
- **(b) Zustände je Task:** T3/T5/T6 benennen leer/lang/mobil-375/Fehler explizit; T4 leer/Duplikat/lang/mobil; T1/T2/T7 sind Backend/CLI (Zustände = Testfälle: default, Bild/Download, SVG-Reject, Kollision, Quota, cross-owner, 304/Cache-Split); T8 ist der Gate. **Kein „laufender Timer"** relevant (Artefakte sind keine Timer-Fläche) — bewusst n. a.
- **(c) Querschnitte:** main.go-Wiring → T2 (Store+5 Usecases, 3 mit Emitter) + T8-Verify; SSE je Mutation → `artifact.*` (T2 REST-Upload/Delete, T5 Web-Upload/Rename/Delete, T7 MCP) mit benanntem Konsument `#cockpit-artifacts`; i18n beide Kataloge → T3/T4/T5/T6 Key-Steps + T8-Parity-Check; Responsive → T3/T5/T6 + Gate 960/375; Owner-Scoping → Negativtests T1 (Store), T2 (REST+Serve), T5 (Web), T6 (Picker), T7 (MCP/CLI via Server).
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; Step 0 rg-Verifikation aller Bestandsnamen; „Bestand gewinnt". Die trickreichen Stellen (Parser-Trigger-Priorität, gemeinsamer Zähler, bluemonday-OR-Semantik, Cache-Split, Ahnenkette-nicht-Subtree, CHECK-freie Migration) sind je mit eigenem Pflicht-Testfall + rg-Anker abgesichert.

### Adversariale Lückensuche — Berater-Findings + Verbleib

Beide Berater liefen SYNCHRON im Vordergrund gegen Spec + Plan-Entwurf + Dossier + realen Code mit dem wörtlichen Lücken-Auftrag. **`codex exec`** (`--sandbox read-only`, gpt-5-Klasse) lief sauber (8 Findings, gegen den echten Code verifiziert). **`agy`/Gemini 3.1 Pro** (`gemini-bigcontext`-Agent) lief sauber (7 Findings, mehrere gegen den Code gegengeprüft). **`gemini`-CLI-Fallback nicht nötig** (`agy` lieferte). **Degradations-Notiz:** keine — beide Sichten vorhanden und komplementär. **Planner-Vorlauf:** VOR den Beratern hatte der Planner bereits drei Selbst-Fund-Korrekturen eingearbeitet (Serve-URL zeigt auf den EIGENEN Node des Artefakts; nearest-ancestor-wins-Ordnung; `Rename`-Store-Methode + Interface-Ripple) — die Berater bestätigten diese teils unabhängig.

**codex exec — 8 Findings, ALLE eingearbeitet (7) bzw. als bereits abgedeckt eingeordnet (1):**
1. **[eingearbeitet — Task 2 `DeleteArtifact`/`GetArtifact`-Doku + Testliste]** (Codex #1) Ownership/Owner-Scope für Delete/Serve: die beiden Usecases brauchen **kein** `Nodes`-Feld — die Store-Primitive `Delete`/`Get` sind **owner+node+slug-scoped** (fremder Owner → `ErrArtifactNotFound`/nichts); die Negativtests (DELETE fremd, Serve fremd) sind in Task 2 Step 1 explizit. Klargestellt statt Usecase aufgebläht.
2. **[eingearbeitet — Task 2 Usecase `RenameArtifact` + Server-Feld + main-Wiring; Task 5 Handler; Task 8 Verify]** (Codex #2, deckt sich mit agy #1) `Rename` war nur Store-Methode + Handler-Direktaufruf. → Jetzt **eigener Usecase `RenameArtifact{Nodes,Artifacts,Emitter}`** (hexagonal), im Server-Literal + main.go verdrahtet, Task 8 verifiziert **fünf** Usecases.
3. **[eingearbeitet — Task 2 SSE-in-Usecases-Vorgabe + main-Wiring `Emitter: emitter`; Task 5 Handler emittieren nicht]** (Codex #3, KRITISCH) Spec §12 verlangt Emit **aus den Mutations-Usecases**; der Entwurf emittierte in den Handlern. → `UploadArtifact`/`RenameArtifact`/`DeleteArtifact` halten jetzt `Emitter` und emittieren selbst (DRY über REST+Web, Präzedenz `AddDayOffs`); die Handler emittieren nicht mehr.
4. **[eingearbeitet — Task 2 Upload `replaceSlug` + `artifact.updated`; Task 5 „Ersetzen"-Affordanz; Tests]** (Codex #4) Re-Upload/Überschreiben (Spec §3/§12) fehlte — nur der Kollisions-Suffix-Neu-Pfad war geplant. → `UploadArtifact.Execute(replaceSlug,…)`: leer = neu (`artifact.created` + Suffix), gesetzt = überschreiben (neuer `ref`, `artifact.updated`); Galerie-Karte bekommt eine „Ersetzen"-Affordanz; Testfall ergänzt.
5. **[eingearbeitet — Task 3 `ArtifactRef.Ref` + Renderer-Regel]** (Codex #5, KRITISCH) `ArtifactRef` hatte kein `Ref`, der Renderer bildete aber `{Href}?v={ref}`. → `Ref string` ergänzt; Bild-`src` = `{Href}?v={Ref}` (immutable), Datei-Chip = nacktes `{Href}` (no-cache, frischer Dateiname).
6. **[eingearbeitet — Task 3 Nummerierungs-Transformer hält `resolveArtifact`]** (Codex #6, deckt OE #4-Selbsteingeständnis) Der Zähl-Transformer zählte jeden `artifactEmbedNode` ohne Resolver — Widerspruch zur „nur aufgelöste zählen"-Empfehlung. → `figureTransformer{resolveArtifact}` bekommt den Resolver mit und überspringt unaufgelöste Embeds; Pflicht-Testfall (unaufgelöster Embed zwischendrin verbraucht keine Nummer).
7. **[eingearbeitet — Task 4 Step 1 Hover-¶-Test]** (Codex #7) Der Hover-¶-Anker (Spec §7) hatte keinen Task-Test. → Test: gerenderte Überschrift enthält `<a class="head-anchor" href="#slug">¶`, Sanitizer behält ihn, `verify-no-popups` erzwingt Popup-Freiheit (Bestand-`clipboard.js`).
8. **[eingearbeitet — Task 6 Step 1 Seiten-Picker-Owner-Scope + `editor-insert.js`-Note]** (Codex #8) `/ui/editor/seiten` ohne Owner-Scope-Negativtest; JS-Insert untergetestet. → Owner-Scope-Negativtest ergänzt (fremdes Doc erscheint nicht); JS-Insert ist Client-JS → Live-Dogfood-verifiziert (Task 8), plus Assertion auf Toolbar-Buttons + `editor-insert.js`-Asset + Picker-`data-*`-Attribute.

**agy/Gemini 3.1 Pro — 7 Findings: 4 eingearbeitet, 3 als bereits (teil-)gemildert verbucht:**
1. **[eingearbeitet — s. Codex #2]** (agy #1) `Rename` fehlte im Task-1-Interface + Fake → Task 5 kompiliert nicht. → `Rename` ins Interface + `pgstore` + `FakeArtifactStore` (Interface-Ripple-Notiz), und als Usecase (Codex #2).
2. **[eingearbeitet — Task 3 Sanitizer-Vektor-Klarstellung]** (agy #2) Framing-Korrektur: es ist **kein** Raw-HTML-Injection-Vektor (`RenderDocument` setzt kein `html.WithUnsafe()` → goldmark reicht Roh-`<img>` nicht durch); der EINZIGE externe-`img src`-Weg ist goldmarks **Core-Image-Parser** (`![](url)`). → Task 3 präzisiert: der Mechanismus ist **definiert** (Core-Image-Renderer-Override + Sanitizer-Regexp als zweite Linie), nicht mehr „wahrscheinlich"; die drei Negativtests bleiben Pflicht.
3. **[eingearbeitet — Task 3 Note zu Editor-Preview-Coverage]** (agy #3, schwächer) Kein expliziter Editor-Preview-Test mit leerem Resolver in Task 3. → Note: der verhaltensvolle Test liegt in Task 6 (Spec §13); Task 3 sichert die Signaturmigration über den Build. Kein Doppeltest.
4. **[eingearbeitet — Task 2 Quota-Race-Notiz + Deferred-Härtung]** (agy #5) `TotalBytes`+`Put` = Check-then-Act-Race. → Als **bewusst akzeptierter Soft-Cap** dokumentiert (Overshoot pro Tenant + pro Upload ≤ 8 MiB beschränkt, KEINE „nur ein User"-Begründung); transaktionale Härtung als Deferred-Ledger-Notiz; kein Concurrency-Test in L6.
5. **[eingearbeitet — Task 7 verbindlicher Owner-Scope-Test]** (agy #6) Task 7 sagte „Negativtest … möglich". → Jetzt **verbindlicher** Owner-Scope-Test (Stub-Server 404 für fremden Node → MCP-Tool UND CLI-Verb melden Fehler).
6. **[eingearbeitet — Task 8 Wiring-Grep um Web-Galerie-Routen erweitert]** (agy #7, deckt sich mit Codex #2) Task-8-Verify grep-te die beiden Web-Galerie-Mutationsrouten (`/rename`, `/delete`) + `POST /nodes/{id}/artifacts` nicht. → In die Composition-Root-Grep-Liste aufgenommen.
7. **[begründet als bereits gemildert — kein neuer Change]** (agy #4) Migrationsnummer 0031 Merge-Kollisionsrisiko. → Bereits durch den Preflight in den Global Constraints + Task 1 Step 0 („höchste Nummer prüfen, nächste freie nehmen, Bestand gewinnt") gedeckt. Der Merge-Zeitpunkt-Reconciliation-Aspekt ist eine Orchestrator-Verantwortung (Ledger), keine Plan-Task-Lücke — als Hinweis notiert, kein zusätzlicher Task.

**Von beiden explizit/implizit als sauber bestätigt (kein Plan-Change nötig):** die Ahnenketten-Auflösung (nicht Subtree) an allen Stellen; der `!`-Parser-Trigger-Vorrang (wikiLinkParser@100 vor Core-LinkParser@200); der gemeinsame Mermaid/Artefakt-Zähler nach dem Inline-Parsing; `FindWikilinks` `!`-Ausschluss; die goose-annotierte 0031 ohne CHECK-Falle; die Owner-Scope-Negativtests je Fläche.

**Dissens:** einer, vom Planner entschieden — agy #2 nannte die Sanitizer-Lücke „Raw-HTML-Injection", codex sah sie (korrekt) als Core-Image-Parser-Pfad. **Entscheidung:** codex/der Code hat recht (`html.WithUnsafe()` ist nirgends gesetzt → kein Raw-HTML-Durchlass); der reale Rest-Befund (Task 3 muss den Mechanismus festlegen) bleibt gültig und ist eingearbeitet. Ansonsten überschnitten sich die Berater nur bei Rename (agy #1 = codex #2), sonst komplementär.

**Netto aus der Lückensuche:** 3 KRITISCHE strukturelle Korrekturen (SSE-in-Usecases, `ArtifactRef.Ref`, `RenameArtifact`-Usecase) + 2 Spec-Pfad-Ergänzungen (Re-Upload/Overwrite, Nummerierungs-Resolver) + 6 Test-/Owner-Scope-/Wiring-Härtungen — alle verbucht; 1 Finding als bereits gemildert begründet, kein stilles Verwerfen.
