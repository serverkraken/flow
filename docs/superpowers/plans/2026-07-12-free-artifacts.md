# Free Artifacts — owner-globale (node-lose) Artefakt-Bibliothek Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Additiver Folge-Slice auf **Lesesaal L6** (node-gebundene Artefakte). L6 hängt jedes Artefakt an genau einen `node`; freie Notizen (`Document.Type == "free"`, `NodeID == nil`) haben keinen Node → keine Ahnenkette → **keine Artefakt-Bibliothek**. Dieser Slice schließt die Lücke mit einer **owner-globalen „freien" Bibliothek**: Artefakte ohne Node (`domain.Artifact.NodeID == ""` ↔ DB `NULL`), owner-scoped, die freie Notizen referenzieren können — und die **jedes** Dokument als root-oberste (niedrigste) Auflösungs-Ebene sieht (node gewinnt bei Namensgleichheit). Volle L6-Parität: Web-Galerie `/wissen/artefakte` + Editor-Picker + Resolver-Frei-Ebene + REST + MCP + CLI + SSE.

**Architecture:** Server-rendered wie L6 (templ + htmx + Tailwind, kein SPA). Hexagonal, **additiv und DRY (Ansatz A)** — **ein** Modell, **ein** Store, **fünf** bestehende Usecases werden verlängert, nicht dupliziert. Die bestehende L6-Maschinerie (`domain.Artifact`, `ports.ArtifactStore`, `usecase.{Upload,Rename,List,Delete,Get}Artifact`, `pgstore.ArtifactStore`, die httpserver-Handler, `webui.ArtifactRef`/`ArtifactResolver`, `apiclient`, MCP-Tools, CLI) wird an ihren exakten bestehenden Signaturen erweitert. Kern-Hebel: (1) DB — `node_id` nullable + Partial-Unique-Index; (2) Store — NULL-sichere Einzelzeilen-Queries (`IS NOT DISTINCT FROM`) + verzweigtes `Put`-`ON CONFLICT` + neues `ListFree`; (3) Usecases — `UploadArtifact` überspringt den Node-Guard bei `nodeID==""`, `ListArtifacts` verzweigt (frei allein / Kette ++ frei); (4) Render — `buildArtifactResolver` gibt freien Artefakten Position `len(chain)` und Href `/artefakte/{slug}`, `artifactSrcRe` lässt beide Serve-Formen durch; (5) Oberflächen — neue Serve-Route `/artefakte/{slug}`, Galerie `/wissen/artefakte`, REST `/api/v1/artifacts`, MCP-Frei-Pfad, CLI `--free`.

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · goldmark (+ bluemonday-Sanitizer) · pgx/goose. **Eine** neue goose-Migration (`0032`, `ALTER`+Partial-Index). **Keine** neuen externen Abhängigkeiten, **kein** neues Vendoring, **kein** neuer Domain-Typ, **kein** neuer Usecase, **kein** neues Store-Struct, **kein** main.go-Usecase-Wiring (die fünf Usecases + der Store sind bereits verdrahtet — der Slice fügt nur Routen/Oberflächen hinzu).

**Spec:** `docs/superpowers/specs/2026-07-10-free-artifacts-design.md` (Stand 2026-07-10). Alle **5 Entscheidungen E1–E5** sind von Soenne bestätigt und **bindend** — nicht neu verhandeln: (E1) Frei-Ebene überall sichtbar, root-oberste Resolver-Stufe, node gewinnt; (E2) Galerie `/wissen/artefakte`; (E3) nullable `node_id` (Ansatz A, ein Modell); (E4) freie Artefakte im Node-Cockpit als geerbt „Frei" read-only; (E5) volle L6-Parität. Die Spec enthält Migration-DDL, Store-Methoden (`ListFree` + `IS NOT DISTINCT FROM`), Resolver-Regel, Serve-Route, Sanitizer-Doppel-Pfad, Pflicht-Tests und YAGNI-Abgrenzung (§12). Format-Vorbild: `docs/superpowers/plans/2026-07-09-lesesaal-l6-artefakte.md`.

**Basis:** Branch **`free-artifacts`** (bereits ausgecheckt), Worktree `/Users/msoent/SourceCode/serverkraken/flow-free-artifacts`, off `rebuild` @ `df0c9f1`. `df0c9f1` enthält **L6 komplett** (Migration `0031_artifacts.sql`) **plus** den flow-mcp-Auth-Fix. Das Slice-Gate umfasst `df0c9f1..HEAD` (bzw. `rebuild..HEAD`) des `free-artifacts`-Branches.

---

## Global Constraints

- Branch **`free-artifacts`** (bereits ausgecheckt); Worktree `/Users/msoent/SourceCode/serverkraken/flow-free-artifacts`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende mit der exakt vorgegebenen Message.
- **L4/L5-LEHREN — in JEDEN Task-Dispatch-Text aufnehmen:** (1) **Tests/CI SYNCHRON foreground**, **NIEMALS `run_in_background`** (Subagenten warten sonst auf nie kommende Notifications). (2) **Bash-Aufrufe mit `timeout: 600000`** (`make ci` läuft lange — Testcontainer-Postgres). (3) **Erst `git add -A` stagen, dann `make ci`** (verify-generate/verify-css diffen gegen den Index → uncommitted templ/css false-positiv). (4) **Nie zwei `make ci` parallel** (Podman-VM keilt bei parallelen Testcontainer-Läufen → Hard-Stop+Start). (5) **`make web` nach JEDER `.templ`-Änderung** (auch reine Klassennutzung ändert den Tailwind-Scan; verify-css ist ein Drift-Diff) und `internal/adapter/webui/static/app.css` mitcommitten. (6) **`make generate` nach JEDER `.templ`-Änderung**, die `*_templ.go` mitcommitten.
- **NIE `make fmt`** (Toolchain-Skew reformatiert das ganze Repo). **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen (Memory `feedback_subagent_git_commits_isolated`).
- `make ci` muss am Task-Ende grün sein (`lint verify-generate verify-css verify-no-popups cover build`; Coverage-Gate **75 %**, aktuell ~85 %, `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket, siehe AGENTS.md „Tailwind v4 + Docker"). **Task 1 fügt Migration 0032 hinzu → die pgstore-Docker-Tests laufen gegen das geänderte Schema; 0032 MUSS goose Up/Down-annotiert sein** (Memory `feedback_pgstore_goose_migrations`: nur die pgstore-Docker-Tests fangen fehlende Annotationen).
- **Migrations-Nummer 0032 vor dem Anlegen verifizieren:** `ls internal/adapter/pgstore/migrations/ | tail -3` — höchste ist **verifiziert `0031_artifacts.sql`**, also `0032` frei. Falls der Bestand inzwischen weiter ist, die **nächste freie Nummer** nehmen und im Ledger vermerken. **Bestand gewinnt.**
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced (`i18n_test.go` — prüft Key-**Existenz**, EN-Strings **explizit** ausformulieren). Keine hartkodierten Anzeige-Strings; `components.T(ctx, "key")` / `i18nT(r, "key")`. **Bestand-Wiederverwendung zuerst:** `cockpit.artifacts.err.tooLarge/badType/quota/generic`, `cockpit.artifacts.upload/empty/inherited/title` existieren bereits (rg-verifizieren) — für die Galerie/Cockpit-Frei-Fläche wiederverwenden statt duplizieren; nur genuin neue Keys (`wissen.artifacts.*`, `cockpit.artifacts.free`) anlegen.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ✚ ✗ ✓ ○ · + SVG erlaubt; der Datei-Chip nutzt bereits `■`), **keine Browser-Popups** (`verify-no-popups` — kein `alert/confirm/prompt`; Bestätigungen über `components.ConfirmDialog`/`data-dialog-open`).
- **owner-scoped überall** (jede Store-/Serve-/Mutation-/List-Query trägt `ownerID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze, Memory `feedback_flow_is_multi_tenant`). Jede neue Frei-Datenfläche bekommt einen **Owner-Scope-Negativtest**: fremder Owner sieht/lädt/löscht/referenziert **nichts** (pgstore-Store, `/artefakte/{slug}`-Serve, `/api/v1/artifacts` List/Delete, Web-Galerie, Editor-Picker, MCP, CLI).
- **Owner-Quota (Multi-Tenant „Limits per-user"):** `TotalBytes(ctx, owner)` summiert **alle** Owner-Artefakte inkl. freie → der 256-MiB-Cap greift automatisch, **kein** Code-Change. **Aber Pflicht-Testfall** (frei-Upload gegen einen quota-nahen Owner → `ErrArtifactQuotaExceeded`/413). Der Check-then-Act-Race bleibt der bewusst akzeptierte Soft-Cap aus L6 (Overshoot pro Tenant + pro Upload ≤ 8 MiB beschränkt — KEINE „nur ein User"-Begründung).
- **SSE-Regel (Mutation → Event → Konsument benannt):** jede Frei-Mutation emittiert **aus den Usecases** (nicht den Handlern, L6-Muster) genau ein `artifact.created`/`…updated`/`…deleted`; `EventData` trägt `{"id":slug, "name":name, "node":nodeID}` (bei frei `node==""`) — **bereits so in den Bestand-Usecases**, kein Change nötig, nur test-decken. Konsument der Galerie-Live-Updates: der **neue** `/wissen/artefakte`-SSE-Container (`hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`); der Node-Cockpit-Container (`#cockpit-artifacts`) bleibt Konsument für Node-Ops (unverändert). Der account-weite Puls-/Aktivitäts-Feed nutzt die bestehenden `artifact.*`-Verb-Keys — frei (`node==""`) rendert ohne Node-Referenz sauber (Pflicht-Verifikation, kein neuer Verb-Key).
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt, Memory `feedback_design_must_stay_easily_changeable`): die neue Galerie/Karte nutzt die **Bestand-Klassen** `.gallery`/`.artcard`/`.artcard-thumb`/`.artcard-chip`/`.artcard-name`/`.artcard-meta`/`.artcard-origin`/`.artcard-actions`/`.artupload`/`.sect`/`.sect-h`/`.eyebrow`/`.empty`/`.btn`/`.btn-q`/`.btn-s`/`.btn-pri` (aus `cockpit_artifacts.templ`/`web/tailwind.css` — rg-verifizieren), **keine** Arbitrary-`[#hex]`/`[px]`/`[.85rem]`, wo eine benannte Klasse existiert. Der Frei-Serve-Renderer/Resolver ist Backend (kein CSS). Neue Flächen ohne Mockup stehen als **Offene Entscheidungen** mit Empfehlung.
- Tailwind-v4-Fallen (Memory `feedback_tailwind_v4_templ_gotchas`): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen; `make web` mit `DOCKER_HOST=podman` für `make ci`.
- **htmx-GET-Picker brauchen `hx-include`** (nicht nur `hx-params`): htmx 2.x inkludiert das umschließende Form nur bei non-GET (L6-Live-Gate-Finding). Der Bestand-Editor-Picker hat `hx-include="[name=node], [name=projectId]"` **bereits** (`editor.templ:124,138` — rg-verifizieren, **nicht** anfassen); der Frei-Fall funktioniert dadurch automatisch: ein freies Doc sendet `node=""` (leeres hidden-Feld), und der Picker-Handler muss `nodeID==""` als **Frei-Kontext** behandeln (den `if nodeID != ""`-Guard entfernen, Task 3).
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht, jeder Task hat Step 0):** JEDES als „Bestand" referenzierte Symbol vor dem Tippen per `rg -n "<Name>" internal/ cmd/ -g '!*_templ.go'` prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden. Wörtliche Ankerliste u. a.: `domain.Artifact`, `NodeID`, `ArtifactStore`, `ErrArtifactNotFound`, `Put`, `Get`, `GetMeta`, `List`, `Rename`, `Delete`, `ExistingSlugs`, `TotalBytes`, `artifactCols`, `artifactMetaCols`, `nullableInt`, `derefInt`, `NewArtifactStore`, `FakeArtifactStore`, `artifactKey`, `UploadArtifact`, `ListArtifacts`, `RenameArtifact`, `DeleteArtifact`, `GetArtifact`, `ValidateArtifactBytes`, `nextArtifactSlug`, `MaxArtifactBytesPerOwner`, `handleUploadArtifact`, `handleListArtifacts`, `handleDeleteArtifact`, `handleServeArtifact`, `handleWebNodeArtifactUpload`, `handleWebNodeArtifactRename`, `handleWebNodeArtifactDelete`, `readArtifactUpload`, `artifactErrMsg`, `renderNodeArtifacts`, `handleWebEditorArtefaktePicker`, `handleWebEditorSeitenPicker`, `handleWebEditorPreview`, `renderEditorPreview`, `buildEditorArtifactResolver`, `buildDocumentVM`, `buildArtifactResolver`, `ArtifactRef`, `ArtifactResolver`, `artifactSrcRe`, `safeImageHTMLRenderer`, `RenderDocument`, `getDocPolicy`, `artifactEmbedHTMLRenderer`, `ArtifactCardVM`, `BuildArtifactCards`, `artifactTypeLabel`, `CockpitArtifacts`, `nodeCockpitData`, `BuildArtefaktInsertRows`, `InsertPickerRow`, `InsertPickerRows`, `userFrom`, `actor.FromContext`, `writeJSON`, `i18nT`, `c.do`, `UploadArtifact`(apiclient), `artifactNode`, `resolveScope`, `h.do`, `textResult`, `errorResult`, `resolveArtifactNode`, `resolveArtifactMime`, `artifactCmd`, `s.auth`, `s.webAuth`, `EventArtifactCreated/Updated/Deleted`, `Emitter`, `NodeAncestors`, `Ancestors`, `ListDocuments`.

---

## Frei-Artefakt-Semantik — Vorgabe (ENTSCHIEDEN aus Spec E1–E5, NICHT erneut konsultieren)

| Aspekt | Vorgabe (bindend) |
|---|---|
| **Modell (E3)** | **Ein Modell, Ansatz A:** nullable `node_id` auf der bestehenden `artifacts`-Tabelle. `domain.Artifact.NodeID == ""` ↔ DB `NULL` = **frei**. Kein `*string`-Umbau (der Domain-Typ bleibt `NodeID string`), kein separates `free_artifacts`, kein synthetischer Root-Node. pgstore mappt `"" ↔ NULL`. |
| **Reichweite (E1)** | Frei = **owner-global, überall sichtbar**. Ein **Node-Doc** löst `![[slug]]` gegen seine **Ahnenkette ++ die freie Bibliothek** auf; frei = **root-oberste (niedrigste) Priorität** (Resolver-Position `len(chain)`); **node-spezifisch gewinnt** bei Namensgleichheit (nearest-wins bleibt die einzige Regel, frei ist die letzte Stufe). Ein **freies Doc** (`NodeID==nil`) sieht **nur** die freie Bibliothek. |
| **Slug** | Frei-Slug **owner-global eindeutig** über Partial-Unique `(owner_id, slug) WHERE node_id IS NULL` (der Bestands-`unique(owner,node,slug)` greift bei NULL **nicht** — Postgres behandelt NULL als distinkt). Kollision beim Anlegen → `-1/-2`-Suffix via `ExistingSlugs(owner, "")`. Freies `"logo"` und node-`"logo"` **koexistieren**. |
| **Store (NULL-sicher)** | **Einzelzeilen** (`Get`/`GetMeta`/`Rename`/`Delete`/`ExistingSlugs`): `WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2`, `$2 = NULL` bei `node==""`. **`Put`** verzweigt am `ON CONFLICT`-Target (Node vs. `(owner_id,slug) WHERE node_id IS NULL`). **Neu `ListFree(owner)`** = `WHERE node_id IS NULL`. `List`/`TotalBytes` **unverändert**. `node_id`-Scan **nullable** (`NULL → ""`) in jeder Methode, die freie Zeilen liefern kann (`Get`/`GetMeta`/`ListFree`). |
| **Serve** | **`GET /artefakte/{slug}`** (`s.webAuth`) → `GetArtifact(owner, "", slug)`. **Identische** Header-Logik wie die Node-Serve-Route (ETag `"{ref}"`, Cache-Split nackt=`no-cache`/`?v=`=`immutable`, ETag+Cache-Control **vor** 304, `nosniff`, Bild `inline` / sonst `attachment; filename`) — **geteilter Helper** (kein Copy-Paste der sicherheitsrelevanten Header). |
| **Sanitizer (KRITISCH)** | `artifactSrcRe` lässt **beide** legitimen Formen durch: `^/nodes/[A-Za-z0-9_-]+/artifacts/[a-z0-9-]+…$` **UND** `^/artefakte/[a-z0-9-]+…$`, je nackt/`?v=[0-9a-f]{12}`. Extern/`data:`/`//host` → leere `src`. Gate = `safeImageHTMLRenderer`-Override auf `ast.KindImage` (die bluemonday-Regexp-Policy ist wegen OR-Semantik ein No-Op — L6-Befund). 3 Negativtests + Positivkontrolle **je Form**. |
| **Ref/Cache/Quota** | **unverändert aus L6:** `ref = sha256[:12]`, Bild-`src = "{Href}?v={ref}"` (immutable), Datei-Chip nacktes `{Href}` (no-cache). Owner-Quota `TotalBytes(owner)` summiert frei mit — Cap greift automatisch. |
| **Guard** | **Upload:** `Nodes.Get(owner,nodeID)` **NUR bei `nodeID != ""`** (frei hat keinen Node). **Rename/Delete/Get:** owner+slug-scoped, NULL-sicher über den Store (fremder Owner → `ErrArtifactNotFound`). `RenameArtifact`s `GetMeta`-Guard bestätigt weiterhin, dass das Artefakt an **dieser Ebene** (frei) hängt. |
| **SSE** | Dieselben `artifact.created/updated/deleted` **aus den Usecases** (L6-Muster, **kein** Change); `EventData` `{"id":slug,"name":name,"node":nodeID}`, frei `node==""`. Konsument: neuer `/wissen/artefakte`-Container (+ `#cockpit-artifacts` für Node-Ops). |
| **Cockpit (E4)** | Freie Artefakte erscheinen im **Node-Cockpit** als **geerbt (Quelle „Frei")**, **read-only** (`Inherited=true`, `FromNode = "Frei"`-Label, `Href = /artefakte/{slug}`). Verwaltet ausschließlich unter `/wissen/artefakte`. |
| **MCP/CLI (E5)** | **MCP:** `node` optional → frei (OE #1 — **Empfehlung: separater `free bool`-Parameter**, mutually exclusive mit `node`; kollidiert mit keinem Node-Namen [codex #5] und lässt omit→cwd intakt; Token `node:"free"` und Spec-Literal omit→frei als Alternativen). **CLI:** `--free` auf `add/ls/rm`, schließt `--node` aus. Actor-Kind `agent` (Server stempelt). |

---

## Agent-Besetzung & Dispatch-Protokoll

Es gibt **keine** `free-artifacts`-spezifischen Projekt-Agents. Rollen generisch besetzt (Modell im Dispatch **explizit** nennen — nie Fable erben, Memory `feedback_subagent_model_never_inherit_fable`). Orchestrator-Session `/effort high`.

| Task | Rolle (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 Store-Fundament (Migr 0032 · pgstore NULL-safe + `ListFree` · Port `ListFree` · Fake) | `general-purpose` (Implementer) | Sonnet · high |
| 2 Read/Serve frei-fähig (`buildArtifactResolver` Frei-Ebene + `/artefakte`-Href · `BuildArtifactCards` frei · `artifactSrcRe` beide Formen + Negativtests · Serve-Route `/artefakte/{slug}` + geteilter Header-Helper) | `general-purpose` (Implementer) | Sonnet · high |
| 3 Frei aktivieren (`UploadArtifact` skip-guard · `ListArtifacts` frei/Kette++frei · `buildDocumentVM` frei-Doc · `buildEditorArtifactResolver` frei · Editor-Picker-Guard · REST `/api/v1/artifacts` + apiclient) | `general-purpose` (Implementer) | Sonnet · high |
| 4 Web-Galerie `/wissen/artefakte` (Page-templ + Upload/Rename/Delete-Handler + SSE-Container + Wissen-Nav + i18n) | `general-purpose` (Implementer) | Sonnet · high |
| 5 MCP-Frei-Pfad + CLI `--free` | `general-purpose` (Implementer) | Sonnet · medium |
| 6 Wiring-Gate (Route-/Surface-Verify · Rest-Sweep · `make ci` · Live-Dogfood · Breakpoints) | `general-purpose` (Implementer) | Sonnet · medium |
| jedes Task-Review (2-stufig: Plan-Treue + Qualität) | `code-searcher` (Reviewer) | Haiku · high |
| Slice-Ende: Whole-Branch (`rebuild..HEAD`) | `code-searcher` (Final-Reviewer) | Opus · xhigh |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Frei-Artefakt-Semantik-Tabelle + „Branch `free-artifacts`, HEAD-basiert, Worktree `/Users/msoent/SourceCode/serverkraken/flow-free-artifacts`". Ein Task pro Dispatch. **Explizit im Dispatch:** „Tests/`make ci` SYNCHRON foreground, `timeout: 600000`, keine Hintergrund-Läufe; erst `git add -A`, dann `make ci`; nie zwei `make ci` parallel; `DOCKER_HOST` = Podman-Socket."
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` + `git diff --stat HEAD~1`.
3. Dispatch Reviewer mit Task-Text + Commit-Range (BASE = Task-Base). `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger (`.superpowers/sdd/progress.md`, gitignored) fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge, `rebuild..HEAD`):**
1. `make ci` grün.
2. **Rest-Sweep** (Dispatch-Text unten) über `git diff --name-only rebuild..HEAD`.
3. Final-Reviewer (Range `rebuild..HEAD`) → Findings fixen. **Fokus frei:** Owner-Scoping über JEDE Frei-Fläche (Store/Serve/List/Delete/Web/Editor-Picker/MCP/CLI, inkl. Cross-Tenant-`/artefakte/{slug}`-404); die **root-oberste** Resolver-Ebene (frei = Position `len(chain)`, node gewinnt) an ALLEN Auflösungsstellen (Doc-Resolver, Editor-Picker/-Preview, Cockpit-Karte); der Sanitizer lässt **beide** Serve-Formen durch und strippt extern/`data:`/`//host`; die **NULL-sichere Uniqueness** (frei `"logo"` + node-`"logo"` koexistieren, zweites frei-`Put` → `-1`); jede Frei-Mutation emittiert das passende `artifact.*`-Event mit `node==""`; die Owner-Quota summiert frei mit; die neuen Routen (`/artefakte/{slug}`, `/wissen/artefakte`+`/rename`+`/delete`, `/api/v1/artifacts`+`/{slug}`) sind in `server.go` registriert, die MCP-Frei-Pfade + CLI-`--free` verdrahtet — **kein „ship a usecase nothing calls" / keine tote Route**.
4. **Soenne-Live-Gate** (Browser, nicht delegierbar) — Dogfood-Skript Spec §13.
5. Nachlauf: Auto-Memory + flow-Mirror des Plans (`flow_create_doc`/`flow_update_doc`, `type: "agent"`, Pfad `plans/2026-07-12-free-artifacts`), **PROD-Deploy-Notiz** (Migration 0032 rollt via goose: `ALTER COLUMN … DROP NOT NULL` + Partial-Index; Down setzt NOT NULL wieder → nur rückrollbar, wenn keine freien Zeilen existieren).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **verwaiste i18n-Keys** (in beiden Katalogen definiert, nirgends per `T(`/`i18nT(` referenziert) — besonders `wissen.artifacts.*`/`cockpit.artifacts.free`; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `w-[`, `h-[`, `max-h-[`) auf der neuen Galerie-Fläche, wo eine benannte Bestand-Klasse (`.gallery`/`.artcard*`/`.artupload`/`.sect`/`.btn*`) existiert; (c) **verwaiste Symbole** mit null Konsumenten unter den Frei-Neubauten (`ListFree`, `handleServeFreeArtifact`, `handleUploadFreeArtifact`, `handleListFreeArtifacts`, `handleDeleteFreeArtifact`, `handleWebWissenArtifacts*`, `UploadFreeArtifact`/`ListFreeArtifacts`/`DeleteFreeArtifact`, `WissenArtifactsPage`); (d) **Querschnitt-Lücken:** löst JEDE Auflösungsstelle frei als **root-oberste** Ebene (Position `len(chain)`, node gewinnt)? zeigt JEDE `<img src>`/`<a href>` eines freien Artefakts auf `/artefakte/{slug}` (nicht `/nodes//artifacts/`)? emittiert JEDE Frei-Mutation (REST-Upload/Delete, Web-Upload/Rename/Delete, MCP, CLI) das `artifact.*`-Event mit `node==""`? sind die Sanitizer-Negativtests (extern/`data:`/`//host`) für BEIDE Formen vorhanden UND grün? ist die NULL-sichere Uniqueness test-gedeckt? greift die Owner-Quota für frei (413/i18n)? (e) **Route-/Surface-Wiring:** sind `/artefakte/{slug}`, `/wissen/artefakte`(+`/rename`+`/delete`), `/api/v1/artifacts`(+`/{slug}`) in `server.go` registriert, die MCP-Frei-Pfade in `cmd/flow-mcp/*`, das CLI-`--free` in `cmd/flow/artifact.go` verdrahtet? Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis main.go:** die fünf Usecases (`UploadArtifact`/`RenameArtifact`/`ListArtifacts`/`DeleteArtifact`/`GetArtifact`) + `pgstore.NewArtifactStore` sind **bereits** im Server-Literal verdrahtet (`cmd/flow-server/main.go:73,158-162` — rg-verifiziert). Der Slice fügt **keinen** Usecase/Store hinzu → **kein** main.go-Usecase-Wiring. Der Wiring-Gate (Task 6) verifiziert **Routen + MCP/CLI-Surface**, nicht das Composition-Root-Literal.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` übersprungen; Nachlauf ist Orchestrator-Arbeit.

---

### Task 1: Store-Fundament frei — Migration 0032 · pgstore NULL-safe (`Put`-ON-CONFLICT-Zweig · `IS NOT DISTINCT FROM` · nullable node_id-Scan) · `ListFree` · Port + Fake

**Files:**
- Create: `internal/adapter/pgstore/migrations/0032_artifacts_free.sql`
- Modify: `internal/adapter/pgstore/artifacts.go` (`Put`-Zweig, `Get`/`GetMeta`/`Rename`/`Delete`/`ExistingSlugs` NULL-sicher, neu `ListFree`, `nullableStr`-Helper, nullable node_id-Scan) + `internal/adapter/pgstore/artifacts_test.go`
- Modify: `internal/ports/ports.go` (`ArtifactStore`-Interface: `ListFree`)
- Modify: `internal/testutil/fakes.go` (`FakeArtifactStore.ListFree`)

**Interfaces / Produces (für Tasks 2–5):**
- **`ports.ArtifactStore.ListFree(ctx context.Context, ownerID string) ([]domain.Artifact, error)`** — neben `List`. Doku: `// ListFree returns owner-global (node-less) artifact META (no bytes), newest first.` **Interface-Ripple:** zwingt `FakeArtifactStore` (compile-time `var _ ports.ArtifactStore = (*FakeArtifactStore)(nil)`) zur Methode.
- **Store-Verhalten:** `node==""` ↔ `NULL` durchgängig NULL-sicher; freies und node-gleiches Slug koexistieren; `TotalBytes`/`List` unverändert.

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "func \(s \*ArtifactStore\)|artifactCols|artifactMetaCols|nullableInt|derefInt|ON CONFLICT|node_id=\$" internal/adapter/pgstore/artifacts.go
rg -n "type ArtifactStore interface|ListFree|List\(ctx|ErrArtifactNotFound" internal/ports/ports.go
rg -n "type FakeArtifactStore|func \(s \*FakeArtifactStore\)|artifactKey|var _ ports.ArtifactStore" internal/testutil/fakes.go
rg -n "func .*newArtifactStore|newDocStore|TestArtifactStore|func newTestPool" internal/adapter/pgstore/artifacts_test.go internal/adapter/pgstore/*_test.go | head
ls internal/adapter/pgstore/migrations/ | tail -3      # höchste = 0031 → neu 0032 (sonst nächste freie)
rg -n "nullableStr" internal/adapter/pgstore/          # existiert der Helper schon? sonst neu
```
- [ ] **Step 1: Failing Tests** (`artifacts_test.go`, testcontainer, Muster der bestehenden `ArtifactStore`-Tests per rg verifizieren):
  - **Frei-Roundtrip:** `Put` mit `NodeID==""` → `Get(owner,"",slug)` liefert bytes + `NodeID==""` zurück (nullable Scan, kein Scan-Fehler auf NULL); `GetMeta(owner,"",slug)` ohne bytes; `Rename(owner,"",slug,name)` ändert nur `name` (0 Rows → `ErrArtifactNotFound`); `Delete(owner,"",slug)` → 2. Delete → `ErrArtifactNotFound`.
  - **`ListFree`:** zwei freie + ein node-Artefakt desselben Owners → `ListFree(owner)` liefert **nur** die zwei freien, newest-first; **Owner-Scope-Negativ** (fremder Owner → leer).
  - **`ExistingSlugs(owner,"")`** liefert nur freie Slugs (nicht node-Slugs).
  - **Owner-Scope-Negativ je Einzelzeilen-Methode (codex #3):** ein freies Artefakt von Owner A → Owner B's `Get(B,"",slug)`/`GetMeta(B,"",slug)`/`Rename(B,"",slug,…)`/`Delete(B,"",slug)` → je `ErrArtifactNotFound`; `ExistingSlugs(B,"")` enthält A's Slug nicht (`IS NOT DISTINCT FROM NULL` bleibt owner-gefiltert über `owner_id=$1`).
  - **NULL-sichere Uniqueness:** freies `Put("logo")` **und** node-`Put(node,"logo")` koexistieren (kein Unique-Konflikt); **zweites** freies `Put("logo")` mit neuer ID → der Aufrufer würde via `ExistingSlugs` `-1` bilden, aber ein direkter zweiter `Put("logo")` frei **überschreibt** (ON CONFLICT `(owner,slug) WHERE node_id IS NULL`) statt zu duplizieren (Assert: eine Zeile, bytes/ref aktualisiert).
  - **`TotalBytes(owner)`** = Summe node + frei (Quota-Deckung).
  - **Node-Pfad-Regression:** die Bestand-Node-Tests (`Put`/`Get`/`List`/`Rename`/`Delete` mit echtem Node) bleiben grün (die `IS NOT DISTINCT FROM`-Umstellung ändert Node-Verhalten nicht).
- [ ] **Step 2: Laufen lassen** — FAIL (`ListFree` fehlt, Frei-Put bricht an NOT NULL/Unique). `DOCKER_HOST`=Podman-Socket.
- [ ] **Step 3: Migration 0032** (goose Up/Down PFLICHT):
```sql
-- +goose Up
ALTER TABLE artifacts ALTER COLUMN node_id DROP NOT NULL;
-- Der Bestands-unique(owner_id,node_id,slug) greift bei NULL-Zeilen NICHT
-- (Postgres behandelt NULL als distinkt) → Partial-Unique-Index für frei.
CREATE UNIQUE INDEX artifacts_owner_free_slug ON artifacts (owner_id, slug) WHERE node_id IS NULL;

-- +goose Down
-- Achtung: DROP INDEX zuerst, dann NOT NULL zurück. SET NOT NULL schlägt fehl,
-- wenn freie (NULL-)Zeilen existieren — Down ist ein Entwicklungs-Rollback,
-- kein PROD-Pfad; der Betreiber muss freie Artefakte vorher entfernen.
DROP INDEX artifacts_owner_free_slug;
ALTER TABLE artifacts ALTER COLUMN node_id SET NOT NULL;
```
- [ ] **Step 4: pgstore `artifacts.go`** — wörtlich:
  - **Helper** (falls nicht vorhanden): `func nullableStr(s string) *string { if s == "" { return nil }; return &s }` (neben `nullableInt`).
  - **`Put`** verzweigt am ON-CONFLICT-Target; beide Zweige binden `nullableStr(a.NodeID)` für `node_id`:
```go
func (s *ArtifactStore) Put(ctx context.Context, a domain.Artifact) error {
	const base = `
INSERT INTO artifacts (` + artifactCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT `
	const setClause = ` DO UPDATE SET
    name=$5, mime=$6, size_bytes=$7, ref=$8, bytes=$9, width=$10, height=$11, updated_at=$15`
	target := `(owner_id, node_id, slug)`
	if a.NodeID == "" {
		target = `(owner_id, slug) WHERE node_id IS NULL` // Partial-Index-Arbiter
	}
	_, err := s.pool.Exec(ctx, base+target+setClause,
		a.ID, a.OwnerID, nullableStr(a.NodeID), a.Slug, a.Name, a.Mime, a.SizeBytes, a.Ref, a.Bytes,
		nullableInt(a.Width), nullableInt(a.Height), a.CreatedByKind, a.CreatedByRef, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("pgstore: put artifact: %w", err)
	}
	return nil
}
```
  - **`Get`/`GetMeta`:** `WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2 AND slug=$3`, Argument `nullableStr(nodeID)`; **`node_id` in eine `*string`-Variable scannen** und via `derefStr` → `a.NodeID` (Muster `width/height`):
```go
// derefStr maps a NULL node_id back to "" (free artifact).
func derefStr(p *string) string { if p == nil { return "" }; return *p }
// … im Scan: var node *string; … Scan(&a.ID, &a.OwnerID, &node, …); a.NodeID = derefStr(node)
```
  - **`Rename`/`Delete`/`ExistingSlugs`:** `node_id IS NOT DISTINCT FROM $N`, Argument `nullableStr(nodeID)`. (Node-Verhalten unverändert — `IS NOT DISTINCT FROM 'x'` == `= 'x'` wenn beide non-NULL.)
  - **`ListFree`** (neu, nullable node_id-Scan wie `Get`):
```go
func (s *ArtifactStore) ListFree(ctx context.Context, ownerID string) ([]domain.Artifact, error) {
	const q = `SELECT ` + artifactMetaCols + ` FROM artifacts WHERE owner_id=$1 AND node_id IS NULL ORDER BY created_at DESC`
	// … rows-Loop wie List, aber node_id in *string scannen + derefStr …
}
```
  - **`List`/`TotalBytes`:** **unverändert** (List sieht nie NULL-Zeilen — `node_id = ANY($2)` mit echten IDs).
- [ ] **Step 5: Port + Fake** — `ListFree` ins `ArtifactStore`-Interface (Doku); `FakeArtifactStore.ListFree` (Filter `a.OwnerID==owner && a.NodeID==""`, newest-first sort — Muster `List`). `go build ./... ./internal/...` — der Compiler listet Fake-Lücken.
- [ ] **Step 6: Bauen + Tests + Commit**
```bash
git add -A
go build ./... && go test ./internal/adapter/pgstore/... ./internal/testutil/... -race 2>&1 | tail -20   # Docker-Socket
git commit -m "feat(pgstore): free (node-less) artifacts — Migr 0032 (node_id nullable + partial-unique) + NULL-safe store + ListFree"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig (keine templ/css-Änderung).

---

### Task 2: Read/Serve frei-fähig — `buildArtifactResolver` Frei-Ebene + `/artefakte`-Href · `BuildArtifactCards` frei · `artifactSrcRe` beide Formen + Negativtests · Serve-Route `/artefakte/{slug}` + geteilter Header-Helper

> **Konsumenten-zuerst (dormant bis Task 3):** dieser Task macht Resolver, Cockpit-Karte, Sanitizer und Serve **frei-tauglich**, BEVOR Task 3 die Listen tatsächlich freie Artefakte anhängen lässt. Kein transienter Bruch: die Frei-Zweige sind hier per Unit-Test gedeckt und werden in Task 3 integrativ aktiv.

**Files:**
- Modify: `internal/adapter/webui/wikilink.go` (`artifactSrcRe` beide Formen) + `internal/adapter/webui/wikilink_test.go` (**3 gemeinsame Negativtests** [formunabhängig] **+ Positivkontrolle JE Form**)
- Modify: `internal/adapter/httpserver/webui_document.go` (`buildArtifactResolver`: Frei-Position + `/artefakte`-Href) + `internal/adapter/httpserver/webui_document_artifact_test.go`
- Modify: `internal/adapter/webui/cockpit_artifacts_vm.go` (`BuildArtifactCards`: frei-Href + „Frei"-Herkunft) + `internal/adapter/webui/cockpit_artifacts_render_test.go`
- Create: `internal/adapter/httpserver/artifacts_serve.go` erweitern (geteilter Header-Helper + `handleServeFreeArtifact`) + `internal/adapter/httpserver/artifacts_test.go` (Serve-Frei-Tests)
- Modify: `internal/adapter/httpserver/server.go` (Route `GET /artefakte/{slug}`)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (`cockpit.artifacts.free`)

**Interfaces / Produces:**
- **`buildArtifactResolver(chain, arts)` (KRITISCH):** freien Artefakten (`a.NodeID==""`) Position `len(chain)` geben (root-oberste, niedrigste Priorität); `Href = "/artefakte/" + a.Slug` (NICHT `/nodes//artifacts/…`). Nearest-wins bleibt: ein Slug an einem Kettenknoten schlägt denselben freien Slug.
- **`BuildArtifactCards(arts, nodeID, names)`:** freie Karte → `Href = "/artefakte/" + a.Slug` (+`?v={ref}` bei Bild), `Inherited = a.NodeID != nodeID` (frei ist immer `Inherited`), `FromNode` = das „Frei"-Label. Umsetzung ohne Signaturänderung: der **Aufrufer** (`nodeCockpitData`, Task 3-Berührung) setzt `names[""] = i18nT(r,"cockpit.artifacts.free")`; die Href-Verzweigung sitzt in `BuildArtifactCards`.
- **`handleServeFreeArtifact` + geteilter Helper:** die ETag/Cache-Split/304/nosniff/Disposition-Logik aus `handleServeArtifact` in `(s *Server) writeArtifactResponse(w, r, a domain.Artifact)` extrahieren; `handleServeArtifact` UND `handleServeFreeArtifact` rufen ihn (kein dupliziertes sicherheitsrelevantes Header-Set).

- [ ] **Step 0: rg-Verifikation** — `rg -n "artifactSrcRe|safeImageHTMLRenderer|func .*render\(w util.BufWriter" internal/adapter/webui/wikilink.go`; `rg -n "func buildArtifactResolver|pos\[|bestPos|Href:" internal/adapter/httpserver/webui_document.go`; `rg -n "func BuildArtifactCards|href :?=|Inherited|FromNode|names\[" internal/adapter/webui/cockpit_artifacts_vm.go`; `rg -n "func \(s \*Server\) handleServeArtifact|ETag|Cache-Control|If-None-Match|nosniff|Content-Disposition|userFrom|GetArtifact.Execute" internal/adapter/httpserver/artifacts_serve.go`; `rg -n "mux.Handle\(\"GET /nodes/\{id\}/artifacts/\{slug\}\"|s.webAuth" internal/adapter/httpserver/server.go`.
- [ ] **Step 1: Failing Tests**
  - `wikilink_test.go` (Sanitizer/Renderer): **Positivkontrolle JE Form** (Spec §11) — `![](/artefakte/bild?v=aaaaaaaaaaaa)` **überlebt** (Frei-Form, neu) **und** `![](/nodes/n1/artifacts/bild?v=…)` überlebt weiter (Node-Form, Regression), je nackt + `?v=`; **3 gemeinsame Negativtests** — die Angriffsvektoren sind **formunabhängig** (matchen keine der beiden erlaubten Formen), daher genügt ein Satz statt Duplikat-je-Form: `![](https://evil/x.png)`, `![](data:image/png;base64,AAAA)`, `![](//host/x.png)` → `src=""` (gestrippt). *(Deckt „3 Negativtests + Positivkontrolle je Form" §11 strikt: Positive doppelt geführt, Negative form-agnostisch — der `safeImageHTMLRenderer`-Gate wertet `artifactSrcRe` form-unabhängig, ein Node-Negativtest wäre bit-identisch zum Frei-Negativtest. Nutzt den Bestand-`safeImageHTMLRenderer` — nur `artifactSrcRe` ändert sich.)*
  - `webui_document_artifact_test.go`: `buildArtifactResolver` mit (a) einem **freien** Artefakt (`NodeID==""`) → auflösbar, `Href=="/artefakte/slug"`, `Ref` gesetzt; (b) gleichem Slug an einem Kettenknoten **und** frei → **Node-Version gewinnt** (Href zeigt auf den Node); (c) Slug nur frei → frei gewinnt.
  - `cockpit_artifacts_render_test.go` (bzw. VM-Test): `BuildArtifactCards` mit einem freien Artefakt (`names[""]="Frei"`) → Karte `Inherited==true`, `FromNode=="Frei"`, `Href` beginnt `/artefakte/`.
  - `artifacts_test.go` (Serve): `GET /artefakte/{slug}` — Bild → `Content-Type`+`inline`+ETag; nackt → `no-cache`; `?v=ref` → `immutable`; `If-None-Match` → 304; Nicht-Bild → `attachment; filename`; `nosniff`; **Cross-Tenant-404** (fremder Owner → 404). Node-Serve-Regression (`GET /nodes/{id}/artifacts/{slug}`) bleibt grün nach der Helper-Extraktion.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Implementieren**
  - `artifactSrcRe` (Alternation, geteiltes optionales `?v=`):
```go
var artifactSrcRe = regexp.MustCompile(`^(/nodes/[A-Za-z0-9_-]+/artifacts/[a-z0-9-]+|/artefakte/[a-z0-9-]+)(\?v=[0-9a-f]{12})?$`)
```
  - `buildArtifactResolver` (Frei-Position + Href):
```go
func buildArtifactResolver(chain []domain.Node, arts []domain.Artifact) webui.ArtifactResolver {
	pos := make(map[string]int, len(chain))
	for i, n := range chain {
		pos[n.ID] = i
	}
	freePos := len(chain) // free artifacts rank below the root (lowest priority, E1)
	best := make(map[string]domain.Artifact, len(arts))
	bestPos := make(map[string]int, len(arts))
	for _, a := range arts {
		p, ok := pos[a.NodeID]
		if !ok {
			if a.NodeID != "" {
				continue // artifact on a non-ancestor node — not reachable here
			}
			p = freePos // free (node-less) artifact — lowest priority
		}
		if cur, seen := bestPos[a.Slug]; !seen || p < cur {
			best[a.Slug] = a
			bestPos[a.Slug] = p
		}
	}
	if len(best) == 0 {
		return nil
	}
	return func(slug string) (webui.ArtifactRef, bool) {
		a, ok := best[slug]
		if !ok {
			return webui.ArtifactRef{}, false
		}
		href := "/nodes/" + a.NodeID + "/artifacts/" + a.Slug
		if a.NodeID == "" {
			href = "/artefakte/" + a.Slug
		}
		return webui.ArtifactRef{
			Href: href, Ref: a.Ref, Name: a.Name, Mime: a.Mime,
			SizeStr: webui.FormatArtifactSize(a.SizeBytes), IsImage: a.IsImage(),
			Width: a.Width, Height: a.Height,
		}, true
	}
}
```
  - `BuildArtifactCards` (frei-Href):
```go
var href string
if a.NodeID == "" {
	href = "/artefakte/" + a.Slug
} else {
	href = "/nodes/" + a.NodeID + "/artifacts/" + a.Slug
}
if a.IsImage() {
	href += "?v=" + a.Ref
}
// … Inherited: a.NodeID != nodeID; if Inherited { card.FromNode = names[a.NodeID] } …
// (names[""] = "Frei"-Label wird vom Aufrufer gesetzt — Task 3)
```
  - `artifacts_serve.go`: `writeArtifactResponse(w, r, a)` (die gesamte Header-/304-/Write-Logik aus `handleServeArtifact` verbatim übernehmen); `handleServeArtifact` ruft ihn nach `GetArtifact.Execute(owner, PathValue("id"), PathValue("slug"))`; `handleServeFreeArtifact` ruft ihn nach `GetArtifact.Execute(owner, "", PathValue("slug"))`.
  - `server.go`: `mux.Handle("GET /artefakte/{slug}", s.webAuth(http.HandlerFunc(s.handleServeFreeArtifact)))`.
  - i18n: `cockpit.artifacts.free` = de `"Frei"` / en `"Free"` (beide Kataloge).
- [ ] **Step 4: Bauen + Suite + Commit**
```bash
make generate && make web && git add -A
go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git commit -m "feat(webui,httpserver): free-artifact read path — resolver root-level + /artefakte-Href, sanitizer both serve forms, GET /artefakte/{slug} serve"
```
Expected: PASS; `app.css` ggf. geändert (nur falls die templ-Nutzung neue Klassen zieht — hier i. d. R. keine).

---

### Task 3: Frei aktivieren — `UploadArtifact` skip-guard · `ListArtifacts` frei/Kette++frei · `buildDocumentVM` frei-Doc · `buildEditorArtifactResolver` frei · Editor-Picker-Guard · REST `/api/v1/artifacts` + apiclient

**Files:**
- Modify: `internal/usecase/upload_artifact.go` (Node-Guard nur bei `nodeID!=""`) + `upload_artifact_test.go`
- Modify: `internal/usecase/list_artifacts.go` (Verzweigung frei/Kette++frei) + `list_artifacts_test.go`
- Modify: `internal/adapter/httpserver/webui_document.go` (`buildDocumentVM`: Resolver auch für frei-Doc; `nodeCockpitData` `names[""]`-Set — falls hier; sonst `webui_cockpit.go`)
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData`: `names[""] = i18nT(r,"cockpit.artifacts.free")` vor `BuildArtifactCards`)
- Modify: `internal/adapter/httpserver/webui_editor.go` (`buildEditorArtifactResolver`: `nodeID==""` → `ListFree` via `ListArtifacts`)
- Modify: `internal/adapter/httpserver/webui_editor_pickers.go` (`handleWebEditorArtefaktePicker`: `if nodeID != ""`-Guard entfernen)
- Modify: `internal/adapter/webui/insertpicker_vm.go` (`BuildArtefaktInsertRows`: **Slug-Dedup**, codex #1 / OE #6)
- Create: `internal/adapter/httpserver/artifacts.go` erweitern (`handleUploadFreeArtifact`/`handleListFreeArtifacts`/`handleDeleteFreeArtifact` + geteilte Response-Helper) + `artifacts_test.go`
- Modify: `internal/adapter/httpserver/server.go` (REST-Routen `/api/v1/artifacts`, `s.auth`)
- Create: `internal/adapter/apiclient/artifacts.go` erweitern (`UploadFreeArtifact`/`ListFreeArtifacts`/`DeleteFreeArtifact`) + `artifacts_test.go`
- Modify: `internal/adapter/httpserver/webui_editor_test.go` / `webui_editor_pickers_test.go` (Frei-Preview + Frei-Picker)

**Interfaces / Produces (für Tasks 4/5):**
- **`UploadArtifact.Execute`** akzeptiert `nodeID==""` (frei); Rest identisch (Validate/Sniff, Quota, Kollision via `ExistingSlugs(owner,"")`, `ref`, `Put`, Emit — `EventData.node==""` automatisch).
- **`ListArtifacts.Execute`:** `nodeID==""` → `ListFree(owner)` **allein**; `nodeID!=""` → Ancestors → `List(owner,ids…)` **++ `ListFree(owner)`** (frei angehängt, root-oberste).
- **apiclient-Frei-Verben** (eigene Pfade, nicht Node-Verben mit leerem Node): `UploadFreeArtifact(ctx,name,mime,data)` → `POST /api/v1/artifacts`; `ListFreeArtifacts(ctx)` → `GET /api/v1/artifacts`; `DeleteFreeArtifact(ctx,slug)` → `DELETE /api/v1/artifacts/{slug}`. `c.do`-Muster.

- [ ] **Step 0: rg-Verifikation** — `rg -n "func \(uc UploadArtifact\) Execute|Nodes.Get\(ctx" internal/usecase/upload_artifact.go`; `rg -n "func \(uc ListArtifacts\) Execute|Ancestors|Artifacts.List" internal/usecase/list_artifacts.go`; `rg -n "func \(s \*Server\) buildDocumentVM|doc.NodeID != nil|s.ListArtifacts.Execute|buildArtifactResolver" internal/adapter/httpserver/webui_document.go`; `rg -n "func \(s \*Server\) buildEditorArtifactResolver|nodeID == \"\"|NodeAncestors.Nodes|ListArtifacts.Artifacts" internal/adapter/httpserver/webui_editor.go`; `rg -n "func \(s \*Server\) handleWebEditorArtefaktePicker|if nodeID != \"\"|BuildArtefaktInsertRows" internal/adapter/httpserver/webui_editor_pickers.go`; `rg -n "func \(s \*Server\) handleUploadArtifact|handleListArtifacts|handleDeleteArtifact|writeJSON|actor.FromContext|uploadArtifactReq|maxArtifactJSONBody" internal/adapter/httpserver/artifacts.go`; `rg -n "func \(c \*Client\) UploadArtifact|uploadArtifactBody|c.do\(ctx" internal/adapter/apiclient/artifacts.go internal/adapter/apiclient/client.go`; `rg -n "names\[|BuildArtifactCards|nodeCockpitData|s.ListArtifacts.Execute" internal/adapter/httpserver/webui_cockpit.go`.
- [ ] **Step 1: Failing Tests**
  - `upload_artifact_test.go`: `Execute(owner, "", name, …)` → frei-Artefakt (`NodeID==""`), **kein** `Nodes.Get`-Aufruf (Fake-NodeStore darf nicht getroffen werden / darf keinen Node haben) + genau ein `artifact.created` mit `EventData["node"]==""`; **Frei-Slug-Kollision (codex #4):** zweiter frei-Upload mit gleichem Namen → `-1`-Suffix (über `ExistingSlugs(owner,"")` + `nextArtifactSlug`, der eigentliche Usecase-Pfad — nicht nur der pgstore-Put-Overwrite aus Task 1) + `artifact.created`; **Replace** (`replaceSlug` gesetzt, frei) → Überschreiben + `artifact.updated`; Quota greift auch frei (quota-naher Owner → `ErrArtifactQuotaExceeded`).
  - `list_artifacts_test.go`: `Execute(owner, "")` → nur freie (Fake `ListFree`); `Execute(owner, node)` mit Artefakt am Node **und** einem freien → Liste enthält **beide**, das freie am Ende; ein Artefakt an einem Nicht-Vorfahr erscheint nicht; **`Execute(owner, "bogus-or-foreign-id")` → `[]` / nil (NICHT die freie Bibliothek — codex #2)** (Fake `Ancestors` liefert leeren Chain für unbekannten Node).
  - `artifacts_test.go` (httptest): `POST /api/v1/artifacts` JSON → 201 + Meta (`nodeId==""`) + `artifact.created`; `GET /api/v1/artifacts` → Frei-Liste; `DELETE /api/v1/artifacts/{slug}` → 204 + `artifact.deleted`; übergroß → 400; Quota → 413; **Owner-Scope-Negativ je Verb (codex #3):** User B `DELETE /api/v1/artifacts/{A-slug}` → 404 (kein stiller Erfolg) **UND** User B `GET /api/v1/artifacts` enthält **keines** von A's freien Artefakten (List-Isolation, nicht nur Delete).
  - `apiclient/artifacts_test.go` (Stub-Server, Muster Bestand): `UploadFreeArtifact`/`ListFreeArtifacts`/`DeleteFreeArtifact` round-trip gegen die richtigen Pfade.
  - `webui_editor_pickers_test.go`: `GET /ui/editor/artefakte` **ohne** `node` (leer) → listet die **freie** Bibliothek (nicht leer, wenn frei existiert); **Owner-Scope-Negativ** (fremdes freies Artefakt erscheint nicht); **Slug-Dedup (codex #1):** bei node-`logo` **und** frei-`logo` liefert der Picker **genau eine** `logo`-Row (die des Nodes), keine Doppel-Row (`BuildArtefaktInsertRows`-Unit-Test genügt).
  - `webui_editor_test.go`: `POST /wissen/preview` mit `body="![[bild]]"` + `node=""` (freies Bild `bild` existiert) → Vorschau enthält `<img src="/artefakte/bild?v=…">` (Frei-Preview, Spec §13); ohne passendes Artefakt → ungelöst-Chip.
  - `webui_document`-Ebene: ein freies Doc (`NodeID==nil`) mit `![[bild]]` (freies Bild) → `buildDocumentVM` löst es auf (Regression: bisher blieb es ungelöst — der eigentliche Fix).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Implementieren**
  - `upload_artifact.go`:
```go
if nodeID != "" {
	if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
		return domain.Artifact{}, err
	}
}
```
  - `list_artifacts.go` (**codex-Fund #2 — Bogus-Node-Guard, KRITISCH):** `Nodes.Ancestors` liefert für einen **unbekannten/fremden** Node `(nil, nil)` (kein Fehler — rg-verifiziert `pgstore/nodes.go` + `fakes.go`). Ohne Guard fiele ein leerer Chain durch auf `append([], ListFree(owner)…)` → `GET /api/v1/nodes/{bogus}/artifacts` gäbe die **gesamte freie Bibliothek** des Aufrufers zurück statt `[]` (heutiger Kontrakt: `[]`). Ein gültiger Node hat immer `len(chain) >= 1` (chain enthält den Node selbst) → `len(chain) == 0` ⟺ bogus/fremd → **kein Frei-Anhang**:
```go
func (uc ListArtifacts) Execute(ctx context.Context, ownerID, nodeID string) ([]domain.Artifact, error) {
	if nodeID == "" {
		return uc.Artifacts.ListFree(ctx, ownerID)
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, nodeID)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, nil // unknown/foreign node → [], NICHT die freie Bibliothek leaken (codex #2)
	}
	ids := make([]string, len(chain))
	for i, n := range chain {
		ids[i] = n.ID
	}
	nodeArts, err := uc.Artifacts.List(ctx, ownerID, ids...)
	if err != nil {
		return nil, err
	}
	free, err := uc.Artifacts.ListFree(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return append(nodeArts, free...), nil // free appended = root-oberste Ebene
}
```
  - `webui_document.go` `buildDocumentVM` — Resolver **immer** bauen (nodeID leer für frei-Doc):
```go
var chain []domain.Node
nodeID := ""
if doc.NodeID != nil {
	nodeID = *doc.NodeID
	chain, _ = s.NodeAncestors.Execute(r.Context(), ownerID, nodeID)
}
var resolveArtifact webui.ArtifactResolver
if arts, aerr := s.ListArtifacts.Execute(r.Context(), ownerID, nodeID); aerr == nil {
	resolveArtifact = buildArtifactResolver(chain, arts)
}
```
  - `webui_editor.go` `buildEditorArtifactResolver` — frei-Zweig:
```go
func (s *Server) buildEditorArtifactResolver(r *http.Request, ownerID, nodeID string) webui.ArtifactResolver {
	if s.ListArtifacts.Artifacts == nil {
		return nil
	}
	var chain []domain.Node
	if nodeID != "" {
		if s.NodeAncestors.Nodes == nil {
			return nil
		}
		c, err := s.NodeAncestors.Execute(r.Context(), ownerID, nodeID)
		if err != nil {
			return nil
		}
		chain = c
	}
	arts, err := s.ListArtifacts.Execute(r.Context(), ownerID, nodeID) // nodeID=="" → ListFree
	if err != nil {
		return nil
	}
	return buildArtifactResolver(chain, arts)
}
```
  - `insertpicker_vm.go` `BuildArtefaktInsertRows` — **Slug-Dedup (codex #1 / OE #6):** `ListArtifacts(node)` = Kette ++ frei kann denselben Slug **doppelt** liefern (node-`logo` + frei-`logo`). Beide Rows fügen identisch `![[logo]]` ein, das der Resolver zu **node-wins** auflöst → die frei-Row wäre eine stille Fehlleitung. `BuildArtefaktInsertRows` dedupt daher **je Slug, erster Treffer gewinnt**; `ListArtifacts` liefert **node-vor-frei** (`append(nodeArts, free…)`), also gewinnt der node-Eintrag (deckt sich mit dem Resolver auf der Frei-Achse). Kommentar dokumentiert die Ordnungs-Abhängigkeit. (Der pre-existing node-vs-Vorfahr-Gleichslug-Fall aus L6 ist **nicht** Scope — nur die durch frei häufig werdende node-vs-frei-Kollision.)
  - `webui_editor_pickers.go` `handleWebEditorArtefaktePicker` — Guard entfernen:
```go
q := r.URL.Query().Get("aq")
var rows []components.InsertPickerRow
if arts, err := s.ListArtifacts.Execute(r.Context(), u.ID, nodeID); err == nil { // nodeID=="" → frei
	rows = webui.BuildArtefaktInsertRows(arts, q)
}
```
  - **KEIN `editor.templ`-Change nötig (rg-verifiziert):** die Editor-Werkzeugleiste mit **beiden** Insert-Buttons rendert **unbedingt** (`editor.templ:119-162`, außerhalb des `if vm.Editing()`-Blocks) → der „Artefakt einfügen"-Button ist in **freien** Notizen bereits sichtbar. Der hidden `<input name="node">` steht nur im `Editing()`-Zweig; eine **neue** freie Notiz (`handleWebEditorNew`) sendet weder `node` noch `projectId` → der Picker-/Preview-Handler liest `nodeID==""` → Frei-Kontext (nach der Guard-Entfernung oben bzw. dem `buildEditorArtifactResolver`-Frei-Zweig). **Kein Sichtbarkeits-Change, kein templ-Anfassen** (Spec §8.3 „Artefakt-Einfüge-Button auch im Editor freier Notizen sichtbar" ist damit erfüllt).
  - `webui_cockpit.go` `nodeCockpitData` — vor `BuildArtifactCards`: `names[""] = i18nT(r, "cockpit.artifacts.free")` (freie Karten kriegen die „Frei"-Herkunft; E4).
  - `artifacts.go` REST — die JSON-Upload-Body-Parse + Usecase-Call + Fehler-Switch aus `handleUploadArtifact` in `(s *Server) uploadArtifactJSON(w, r, nodeID string)` extrahieren; `handleUploadArtifact` ruft ihn mit `PathValue("id")`, `handleUploadFreeArtifact` mit `""`. Analog Delete/List (frei-Handler rufen die Usecases mit `""`). `writeJSON`/`actor.FromContext`-Muster identisch.
  - `server.go` REST-Routen:
```go
mux.Handle("POST /api/v1/artifacts", s.auth(http.HandlerFunc(s.handleUploadFreeArtifact)))
mux.Handle("GET /api/v1/artifacts", s.auth(http.HandlerFunc(s.handleListFreeArtifacts)))
mux.Handle("DELETE /api/v1/artifacts/{slug}", s.auth(http.HandlerFunc(s.handleDeleteFreeArtifact)))
```
  - `apiclient/artifacts.go` — drei Frei-Verben (Pfade oben).
  - **Regressions-Wachsamkeit:** bestehende Node-List-Tests (`ListArtifacts(node)`) sehen jetzt zusätzlich **freie** Artefakte, sofern der Test-Store welche hat. Isolierte Fakes ohne freie Artefakte bleiben unverändert; einen etwaigen Längen-Assert anpassen. Volle `./internal/usecase/... ./internal/adapter/httpserver/...`-Suite laufen lassen.
- [ ] **Step 4: Bauen + Suite + Commit**
```bash
make generate && make web && git add -A
go test ./internal/usecase/... ./internal/adapter/httpserver/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(artifacts): activate free — upload skip-guard, ListArtifacts free/chain++free, doc+editor resolver frei, REST /api/v1/artifacts + apiclient"
```
Expected: PASS; `app.css` ggf. geändert.

---

### Task 4: Web-Galerie `/wissen/artefakte` — Page-templ + Upload/Rename/Delete-Handler + SSE-Container + Wissen-Nav + i18n

**Files:**
- Create: `internal/adapter/webui/wissen_artifacts.templ` (`WissenArtifactsPage(vm)` + `freeArtifactCard` + `freeArtifactUploadForm` + Rename-Dialog — benannte Bestand-Klassen, Frei-Action-URLs) + `_render_test.go`
- Create: `internal/adapter/webui/wissen_artifacts_vm.go` (`WissenArtifactsVM` + Builder aus `[]domain.Artifact`) — **oder** `BuildArtifactCards`-Wiederverwendung mit `nodeID=""` (dann sind alle Karten `Inherited==false` → eigene, mit Aktionen). **Empfehlung (OE #2):** eigene schlanke Frei-Karten-VM/-templ, die die **Bestand-CSS-Klassen** teilt, aber auf `/wissen/artefakte/...` postet.
- Create: `internal/adapter/httpserver/webui_wissen_artifacts.go` (`handleWebWissenArtifacts` = **volle Page** GET `/wissen/artefakte` · `handleWebWissenArtifactsFragment` = **Fragment-only** GET `/ui/wissen/artefakte` · `handleWebWissenArtifactUpload` POST multipart · `handleWebWissenArtifactRename` · `handleWebWissenArtifactDelete` · `renderFreeArtifacts`-Fragment-Helper) + `_test.go`
- Modify: `internal/adapter/httpserver/server.go` (Routen `s.webAuth`; die Fragment-Route unter `/ui/wissen/artefakte` neben den Bestand-`/ui/wissen/list*`-Routen; `/wissen/artefakte`-Page-Routen **VOR** `/wissen/{id}` — Go-1.22-Mux bevorzugt das spezifische Segment, kein Konflikt, aber sauber gruppieren)
- Modify: `internal/adapter/webui/wissen.templ` — **Einstiegs-Link „Artefakte"** in den Wissen-Pagehead (`WissenFragment`, neben dem Bestand-`common.new`-„Neu"-Button, `href="/wissen/artefakte"`, `hx-boost="false"`). **rg-verifiziert:** `WissenPage` nutzt `AppShell("docs", nil, nil, …)` → es gibt **keinen** Wissen-Subnav zum Einhängen; der auffindbare Ort ist der Pagehead (Muster des „Neu"-Buttons exakt übernehmen). Alternative (OE #3): eine Regal-artige Karte in `wissenShelvesSection`.
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (`wissen.artifacts.title`/`.empty`/`.upload`/`.uploadHint`/`.nav`; `cockpit.artifacts.err.*`/`cockpit.artifacts.upload` **wiederverwenden**)

**Interfaces / Produces:**
- **Web-Routen** (`s.webAuth`; **Handler emittieren NICHT — die Usecases tun es**, L6-Muster; Muster `handleWebNodeArtifactUpload`/`readArtifactUpload`/`artifactErrMsg`/`renderNodeArtifacts` mit `nodeID=""`):
  - `GET /wissen/artefakte` → **volle Page** (`AppShell` + Pagehead + der `#wissen-artefakte`-SSE-Container, der das Fragment via `hx-get` lädt) — Muster `WissenPage`/`wissenOuter`.
  - **`GET /ui/wissen/artefakte` → Fragment-only** (nur das Grid + Upload-Form + Fehler-Slot, KEIN `AppShell`) — Muster der Bestand-Fragment-Route `GET /ui/wissen/list` (`handleWebWissenList`, `server.go:272`) bzw. der Cockpit-Fragment-Route `GET /nodes/{id}/artifacts` (`handleWebNodeArtifacts`). **KRITISCH (gemini-Fund #2):** der SSE-Container zeigt mit `hx-get` auf **diese** Fragment-Route, NICHT auf `/wissen/artefakte` — sonst würde ein SSE-Trigger die **volle AppShell-Seite** in den Container-`div` swappen (verschachtelte Seite; genau das, wozu L6 die Route-Trennung `/nodes/{id}` vs. `/nodes/{id}/artifacts` hat).
  - `POST /wissen/artefakte` (multipart, optional `slug`-Feld = „Ersetzen") → `UploadArtifact.Execute(owner,"",name,declaredMime,data,replaceSlug,…)`; Fehler **inline i18n** (`artifactErrMsg`), kein Popup; rendert via `renderFreeArtifacts` das **Fragment** (dieselbe Templ wie die Fragment-Route).
  - `POST /wissen/artefakte/{slug}/rename` → `RenameArtifact.Execute(owner,"",slug,name)` → Fragment.
  - `POST /wissen/artefakte/{slug}/delete` (ConfirmDialog `data-dialog-open`, **kein `confirm()`**) → `DeleteArtifact.Execute(owner,"",slug)` → Fragment.
- **SSE-Container** `#wissen-artefakte`: `hx-get="/ui/wissen/artefakte"`, `hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`, `hx-swap="innerHTML"` (die freien Mutationen emittieren via die Usecases; der Container re-fetcht die **Fragment-Route**). **Coarse-Granularität akzeptiert (wie L6):** auch Node-Artefakt-Events triggern den Container — er rendert dieselbe Frei-Liste neu (harmlos, OE #5).
- **Nav-Eintrag** „Artefakte" in der Wissen-Sektion (neben den Bestand-Einträgen; genaue Nav-Komponente in Step 0 rg-verifizieren, **Bestand-Muster** exakt übernehmen).

**Zustände dieser Fläche:** leer (keine freien Artefakte → ruhiger Empty-State „Noch keine freien Artefakte" + Upload-Form); eigenes Bild (Thumbnail); eigenes Nicht-Bild (Datei-Chip mit Typ-Kürzel `PDF`/`CSV`/…); **lang** (langer Dateiname `truncate`/`min-w-0`); **mobil 375px** (Grid kollabiert auf 1–2 Spalten, kein horizontales Pannen); Fehlerpfad (Upload zu groß/falscher Typ/Quota → **inline** i18n, kein Popup, kein 500; Kollision → `-1`-Suffix serverseitig). **Kein** laufender Timer relevant.

- [ ] **Step 0: rg-Verifikation** — `rg -n "templ CockpitArtifacts|templ artifactCard|templ artifactUploadForm|class=\"gallery|class=\"artcard|class=\"artupload|data-dialog-open|ConfirmDialog|hx-trigger=\"sse:artifact" internal/adapter/webui/cockpit_artifacts.templ`; `rg -n "func \(s \*Server\) handleWebNodeArtifactUpload|readArtifactUpload|artifactErrMsg|renderNodeArtifacts|MaxBytesReader|actor.FromContext" internal/adapter/httpserver/webui_artifacts.go internal/adapter/httpserver/webui_cockpit.go`; `rg -n "mux.Handle\(\"GET /wissen|/wissen/typ|/wissen/neu|/wissen/\{id\}" internal/adapter/httpserver/server.go`; `rg -n "wissen.nav|/wissen/typ|Nav|subnav|WissenVM" internal/adapter/webui/wissen.templ`; `rg -n "cockpit.artifacts.err|cockpit.artifacts.upload|cockpit.artifacts.empty|cockpit.artifacts.title" internal/i18n/catalog_de.go`.
- [ ] **Step 1: Failing Tests** — `wissen_artifacts_render_test.go`: Grid rendert eigene freie Karten (mit Löschen/Umbenennen/Ersetzen); Bild → Thumb, PDF → Chip; leer → Empty-State + Upload-Form. `webui_wissen_artifacts_test.go`: `POST /wissen/artefakte` multipart → 200 + Fragment + `artifact.created` (echter Bus); Umbenennen → `name` geändert, `ref`/`slug` stabil, `artifact.updated`; Löschen → `artifact.deleted`; zu groß/Quota → **inline**-Fehler (kein 500, kein Popup); **Owner-Scope** (fremdes freies Slug → kein Effekt, `ErrArtifactNotFound` still). `GET /wissen/artefakte` → 200 + eigene freie Artefakte, **fremde erscheinen nicht**.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Implementieren** — VM/Builder; `wissen_artifacts.templ` (Bestand-Klassen, Frei-Action-URLs, ConfirmDialog fürs Löschen); GET/POST/rename/delete-Handler + `renderFreeArtifacts`-Fragment (Muster `renderNodeArtifacts`); SSE-Container; Routen; Nav-Eintrag; i18n beide Kataloge; `make generate` + `make web`.
- [ ] **Step 4: Bauen + Suite + Commit**
```bash
make generate && make web && git add -A
go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git commit -m "feat(wissen): freie Artefakt-Galerie /wissen/artefakte (Upload/Umbenennen/Löschen, SSE-Container, Wissen-Nav)"
```
Expected: PASS; `app.css` geändert.

---

### Task 5: MCP-Frei-Pfad + CLI `--free`

**Files:**
- Modify: `cmd/flow-mcp/tools_artifacts.go` (die drei Tool-Handler: `node`-Frei-Pfad) + `tools_artifacts_test.go`
- Modify: `cmd/flow/artifact.go` (`--free`-Flag auf `add/ls/rm`, Ausschluss `--node`) + `artifact_test.go`

**Interfaces / Produces:**
- **MCP (OE #1 — Empfehlung: `free bool`-Parameter):** die drei Input-Structs (`uploadArtifactIn`/`listArtifactsIn`/`deleteArtifactIn`) bekommen ein Feld `Free bool \`json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"\``. In `uploadArtifact`/`listArtifacts`/`deleteArtifact`: wenn `in.Free` → **`h.artifactNode` umgehen** (frei hat keinen Node) und die apiclient-Frei-Verben rufen (`c.UploadFreeArtifact`/`c.ListFreeArtifacts`/`c.DeleteFreeArtifact`), `label = "(free library)"`; wenn `in.Free && in.Node != ""` → `errorResult("free and node are mutually exclusive")`; sonst der Bestand-Node-Pfad (omit → cwd-Binding, **unverändert**). Actor-Kind `agent` (Server stempelt). **Falls Soenne die Token-/Spec-Literal-Variante will (OE #1):** statt des Bool das `in.Node == "free"`-Token bzw. `in.Node == ""` prüfen.
- **CLI:** `--free` (bool) auf `add/ls/rm`. `--free` && `--node != ""` → Fehler „`--free` and `--node` are mutually exclusive". Bei `--free`: Node-Auflösung überspringen, Frei-apiclient-Verben rufen. Sonst Bestand-Pfad (`resolveArtifactNode`).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func \(h \*handlers\) artifactNode|uploadArtifact|listArtifacts|deleteArtifact|h.do\(ctx|c.UploadArtifact|resolveScope|errorResult|textResult|jsonschema" cmd/flow-mcp/tools_artifacts.go`; `rg -n "func runArtifactAdd|runArtifactLs|runArtifactRm|resolveArtifactNode|StringVar|BoolVar|c.UploadArtifact|c.ListArtifacts|c.DeleteArtifact" cmd/flow/artifact.go`; `rg -n "func \(c \*Client\) UploadFreeArtifact|ListFreeArtifacts|DeleteFreeArtifact" internal/adapter/apiclient/artifacts.go` (Task 3 lieferte diese).
- [ ] **Step 1: Failing Tests** — `tools_artifacts_test.go` (Stub-apiclient/Server, Muster Bestand): `free:true` → ruft `UploadFreeArtifact`/`ListFreeArtifacts`/`DeleteFreeArtifact` (**`h.artifactNode`/`resolveScope` NICHT getroffen**); `free:true, node:"x"` → `errorResult` (Ausschluss); `node` gesetzt, `free:false` → Bestand-Node-Pfad (unverändert); **Owner-Scope-Negativ** (Stub 404 → Tool meldet Fehler, kein stiller Erfolg). `artifact_test.go` (Muster Bestand): `add --free`/`ls --free`/`rm <slug> --free` gegen Stub-Server → Frei-Verben; `--free --node x` → Fehler (Exit≠0); **Owner-Scope-Negativ** (Stub 404 → CLI-Fehler).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Implementieren** — MCP-`free`-Bool-Parameter (3 Structs) + Frei-Pfad + Ausschluss `free`+`node`; CLI-`--free`-Flag + Ausschluss + Frei-Verben-Aufrufe.
- [ ] **Step 4: Bauen + Tests + Commit**
```bash
git add -A && go test ./cmd/... ./internal/adapter/apiclient/... -race 2>&1 | tail -20
git commit -m "feat(mcp,cli): free-artifact path — MCP free param + CLI --free (Agenten + Menschen laden node-lose Artefakte)"
```
Expected: PASS; `make generate`/`make web` **nicht** nötig.

---

### Task 6: Wiring-Gate — Route-/Surface-Verify · Rest-Sweep · `make ci` · Live-Dogfood · Breakpoints

**Files:** i. d. R. keine neuen (Verifikation + evtl. Sweep-Fixes + Ledger).

- [ ] **Step 1: Route-/Surface-Verifikation** (`server.go` + `cmd/flow-mcp/*` + `cmd/flow/artifact.go`)
```bash
# Serve + Galerie + REST-Frei:
rg -n "GET /artefakte/\{slug\}|GET /wissen/artefakte\b|POST /wissen/artefakte\b|POST /wissen/artefakte/\{slug\}/rename|POST /wissen/artefakte/\{slug\}/delete|POST /api/v1/artifacts\b|GET /api/v1/artifacts\b|DELETE /api/v1/artifacts/\{slug\}" internal/adapter/httpserver/server.go
# main.go: KEIN neuer Usecase erwartet — nur bestätigen, dass die 5 + Store bereits verdrahtet sind:
rg -n "artifactStore :?=|UploadArtifact:|RenameArtifact:|ListArtifacts:|DeleteArtifact:|GetArtifact:" cmd/flow-server/main.go
# MCP-Frei-Pathway + CLI-Flag:
rg -n "Free bool|in.Free|UploadFreeArtifact|ListFreeArtifacts|DeleteFreeArtifact" cmd/flow-mcp/tools_artifacts.go
rg -n "free|--free|BoolVar" cmd/flow/artifact.go
# Nav-Eintrag:
rg -n "wissen/artefakte" internal/adapter/webui/
```
Erwartet: alle Frei-Routen registriert; die fünf Usecases + Store unverändert verdrahtet (kein „ship a usecase nothing calls"); MCP-Frei-Token + CLI-`--free` vorhanden; Galerie-Nav-Eintrag vorhanden.
- [ ] **Step 2: Rest-Sweep** — Dispatch-Text oben über `git diff --name-only rebuild..HEAD`. Gefundene tote Keys/Arbitrary-Values/verwaiste Symbole/Wiring-Lücken fixen.
- [ ] **Step 3: Tote i18n-Keys** — `wissen.artifacts.*`/`cockpit.artifacts.free` gegen `T(`/`i18nT(`-Nutzung; keine verwaisten; de+en-Parität. **Puls-Feed-Verifikation:** ein `artifact.*`-Event mit `node==""` rendert eine Aktivitäts-Zeile ohne Fehler (bestehende `artifact.*`-Verb-Keys, kein Node-Bezug — rg die Verb-Key-Nutzung, kein neuer Key).
- [ ] **Step 4: Volles CI**
```bash
git add -A
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST=Podman-Socket
```
(erst stagen, dann ci — L4-Lehre; nie zwei ci parallel; `timeout: 600000`.)
- [ ] **Step 5: Live-Dogfood** (Dev-Stack; Cookie-Flow) — Spec §13:
```bash
make dev-run &   # https://localhost:8080 (self-signed); danach stoppen
sleep 2
# Migration 0032 angewandt? (node_id nullable + partial-unique-index)
# /wissen/artefakte: Upload Bild (PNG) + PDF → Karten (SSE artifact.created)
# Freie Notiz (Wissen → neu, kein Projekt) mit ![[bild]] + ![[pdf]] → Figur/Chip + Abb.-Nummern; Preview löst LIVE auf
# Node-Doc referenziert dasselbe globale ![[bild]] → rendert es; ein node-lokaler Slug schlägt den gleichnamigen freien
# Deep-Link/Serve: /artefakte/bild?v=ref immutable; nackt no-cache; If-None-Match → 304
# MCP: flow_upload_artifact free=true → landet in /wissen/artefakte (CreatedByKind=agent)
# CLI: flow artifact add x.png --free ; flow artifact ls --free ; flow artifact rm <slug> --free
# Node-Cockpit: das freie Artefakt erscheint als geerbt „Frei", read-only
# Owner-fremd: anderer Owner GET/Serve/DELETE /artefakte/{slug} + /api/v1/artifacts → 404/nichts
```
Expected: Upload → SSE-Galerie-Reload; Frei-Embeds rendern als nummerierte Figuren in freien UND Node-Docs; node schlägt frei; Serve-Cache/304 korrekt; MCP/CLI-Frei sichtbar; Cockpit zeigt „Frei"-Herkunft; owner-fremder Zugriff scheitert. Danach Server stoppen.
- [ ] **Step 6: Breakpoint-Sichtprobe für Soenne notieren** — **≤960px** (Galerie-Grid stackt) und **375px** (Karte/Chip pannen NICHT horizontal; `truncate`; Picker-Dropdown im Viewport).
- [ ] **Step 7: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(free-artifacts): Gate — Route/Surface-Verify + Sweep + Live-Dogfood (frei/Galerie/Resolver/MCP/CLI)"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Alternativpfade. Entscheidung am Ausführungsstart.

1. **MCP-Frei-Opt-in: separater `free: true`-Bool-Parameter, reserviertes Token `node:"free"`, oder Spec-Literal „omit → frei"?** — *Empfehlung: separater `free: true`-Bool-Parameter* (mutually exclusive mit `node`, spiegelt die CLI-`--free`-Symmetrie). **Warum nicht Token `node:"free"`:** ein reserviertes Token **überschattet** dauerhaft einen realen Node, dessen Slug/Name buchstäblich „free" ist (codex-Fund #5 — Kollisionsgefahr für alle drei Artefakt-Tools). **Warum nicht Spec-Literal „omit → frei" (§8.5):** die Bestand-Tools behandeln ein **weggelassenes** `node` bereits als „cwd-gebundenes Projekt" (`jsonschema`: „omit to use the current directory's project" → `resolveScope`); „omit → frei" bräche das (ein Agent in einem gebundenen Repo landet ungewollt in der freien Bibliothek). Ein **eigener `free`-Bool** kollidiert mit **keinem** Node-Namen und lässt die omit→cwd-Semantik intakt. **Trade-off:** ein Parameter mehr pro Tool (drei jsonschema-Felder); `free:true` + `node:x` → Fehler (Ausschluss). **Alternativen:** Token `node:"free"` (Node-Shadow, codex #5) oder Spec-Literal omit→frei (bricht cwd-Binding). Empfehlung: `free`-Bool. *(Die E5-Entscheidung „node optional" bleibt in allen Varianten erfüllt.)*

2. **Web-Galerie `/wissen/artefakte`: dedizierte Frei-templ (Bestand-CSS-Klassen geteilt) oder die Cockpit-Sub-templs (`artifactCard`/`artifactUploadForm`) parametrisieren?** — *Empfehlung: dedizierte Frei-templ, die die **benannten** Bestand-Klassen (`.gallery`/`.artcard*`/`.artupload`) wiederverwendet, aber auf `/wissen/artefakte/...` postet.* Die Cockpit-Sub-templs backen node-scoped Action-URLs (`/nodes/{id}/artifacts/...`) ein; sie zu parametrisieren würde die **L6-templ anfassen** (Regressionsrisiko an der node-Galerie). Eine dedizierte Frei-templ ist rein additiv und hält das Design zentral (geteilte Klassen). **Trade-off:** etwas Markup-Duplikat (Karte/Upload-Form). **Alternative:** die Sub-templs um einen `baseURL`-Parameter erweitern (DRY-er, aber invasiv in L6). Empfehlung: dedizierte templ, geteilte Klassen.

3. **`/wissen/artefakte`-Einstieg: Pagehead-Link auf `/wissen`, oder eine Regal-Karte in der Regal-Sektion?** — *Empfehlung: Pagehead-Link „Artefakte"* neben dem Bestand-„Neu"-Button (rg-verifiziert: `WissenPage` = `AppShell("docs", nil, nil, …)` → **kein** Wissen-Subnav existiert; die Wissen-Seite hat einen Pagehead + 7 Typ-Regale + „Zuletzt aktualisiert"). Ein Pagehead-Link ist sofort sichtbar und mischt die Verwaltungsfläche nicht in die Regal-Liste. **Trade-off:** der Pagehead trägt dann zwei Aktionen (Neu + Artefakte). **Alternative:** eine Regal-artige Karte in `wissenShelvesSection` (konsistent mit den Typ-Regalen, aber ein Artefakt-„Regal" ist semantisch kein Doc-Typ und könnte verwirren). Empfehlung: Pagehead-Link (Navigation: Sichtbarkeit > Redundanz-Elimination, Memory). Kein AppShell-/Tabstrip-Change (die Galerie lebt unter dem Wissen-Tab).

4. **Datei-Chip-Typ-Kürzel auf der Frei-Galerie: `artifactTypeLabel` (Bestand, `PDF`/`CSV`/…) wiederverwenden?** — *Empfehlung: JA* (das Bestand-`artifactTypeLabel`/`artifactTypeLabels` aus `cockpit_artifacts_vm.go` teilen — monospace-Kürzel, keine Emoji, konsistent mit der Node-Galerie). **Trade-off:** keiner. **Alternative:** eigene Frei-Kürzel (unnötige Divergenz). Empfehlung: wiederverwenden.

5. **`/wissen/artefakte`-SSE-Granularität: der Frei-Container hört auf ALLE `artifact.*`-Events (auch Node-Ops) — feiner filtern?** — *Empfehlung: NEIN, coarse belassen* (wie L6-Cockpit): ein Node-Artefakt-Event triggert auch den Frei-Container, der dieselbe Frei-Liste neu rendert — ein harmloser Extra-Fetch. Ein event-payload-basierter Filter (`node==""`) wäre client-seitig htmx-fremd und L6-inkonsistent. **Trade-off:** gelegentlicher No-op-Refresh der Galerie. **Alternative:** dedizierte `artifact.free.*`-Event-Typen (mehr Event-Oberfläche, bricht die L6-Verb-Key-Wiederverwendung). Empfehlung: coarse.

6. **Gleichnamiger Slug an Node UND frei (`node-logo` + `frei-logo`): im Editor-Picker und/oder Cockpit dedupen?** (codex-Fund #1) — *Empfehlung: im Editor-**Picker** dedupen (node gewinnt), im **Cockpit** beide zeigen.* Der Picker fügt `![[slug]]` **slug-basiert** ein → beide Rows fügen identisch ein, was der Resolver zu node-wins auflöst; zwei Rows wären eine stille Fehlleitung, daher **eine** Row (die node-Version, da `ListArtifacts` node-vor-frei liefert). Das **Cockpit** ist eine **Verwaltungs**-Sicht (kein Insert) — dort beide Karten zu zeigen („logo (eigen)" + „logo (geerbt Frei)") ist **informativ** (macht die Überschattung sichtbar) und deckt sich mit E4 („frei als geerbt zeigen"). **Trade-off:** Picker- und Cockpit-Verhalten divergieren minimal (bewusst: Insert- vs. Verwaltungs-Semantik). **Alternative A:** auch das Cockpit dedupen (versteckt die geerbte Frei-Karte — widerspricht E4 leicht). **Alternative B:** gar nicht dedupen (Picker zeigt zwei identisch einfügende Rows — verwirrend, codex #1). **Hinweis:** der pre-existing L6-Fall „gleicher Slug an Node UND Vorfahr" ist **nicht** Scope dieses Slice (frei macht die Kollision nur häufig). Empfehlung: Picker-Dedup (Task 3), Cockpit unverändert.

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads (kanonisch).** Vollständig gelesen und für jede verwendete Signatur direkt am Code (Worktree `flow-free-artifacts`, Branch `free-artifacts`) verifiziert: der L6-Plan (Formatvorbild, alle 8 Tasks + OE + Self-Review), AGENTS.md, die Free-Artifacts-Spec (2026-07-10), sowie **`internal/domain/artifact.go`** (Struct `NodeID string`/`json:"nodeId"`, `MaxArtifactBytes`, Allowlists, `ArtifactSlug*`/`IsImage`/`Validate`), **`internal/adapter/pgstore/artifacts.go`** KOMPLETT (`Put` ON CONFLICT, `Get`/`GetMeta`/`List`/`Rename`/`Delete`/`ExistingSlugs`/`TotalBytes`, `artifactCols`/`artifactMetaCols`, `nullableInt`/`derefInt`), **`internal/adapter/pgstore/migrations/0031_artifacts.sql`** (NOT NULL + `UNIQUE(owner,node,slug)` + Index), **`internal/ports/ports.go`** (`ArtifactStore`-Interface, `ErrArtifactNotFound`, `Ancestors`), **`internal/usecase/{upload,list,rename,delete,get}_artifact.go`** KOMPLETT (Guards, `ValidateArtifactBytes`, Quota, `nextArtifactSlug`, `EventData {"id","name","node"}`, `MaxArtifactBytesPerOwner`, Sentinels), **`internal/adapter/httpserver/artifacts.go`+`artifacts_serve.go`** (REST-Handler, Serve-Header/Cache-Split/304/nosniff/Disposition, `userFrom`/`actor.FromContext`/`writeJSON`), **`internal/adapter/httpserver/webui_document.go`** (`buildDocumentVM` mit `doc.NodeID != nil`-Guard, `buildArtifactResolver` mit `pos`/`bestPos`/Href), **`internal/adapter/httpserver/webui_editor.go`+`webui_editor_pickers.go`** (`handleWebEditorPreview`/`renderEditorPreview`/`buildEditorArtifactResolver` mit `nodeID==""`-Guard; `handleWebEditorArtefaktePicker` mit `if nodeID != ""`-Guard), **`internal/adapter/httpserver/webui_artifacts.go`** (`readArtifactUpload`/`artifactErrMsg`/`renderNodeArtifacts`/Web-Handler), **`internal/adapter/httpserver/webui_cockpit.go`** (`nodeCockpitData`, `names`-Map, `BuildArtifactCards`-Aufruf), **`internal/adapter/httpserver/server.go`** (Server-Struct-Felder + alle Artefakt-Routen + `s.auth`/`s.webAuth`), **`internal/adapter/webui/wikilink.go`** KOMPLETT (`RenderDocument`-Signatur, `safeImageHTMLRenderer`+`artifactSrcRe`, `getDocPolicy`, goldmark-Verdrahtung), **`internal/adapter/webui/artifact_embed.go`** (`ArtifactRef`/`ArtifactResolver`/`artifactEmbedHTMLRenderer`, `?v={ref}`), **`internal/adapter/webui/cockpit_artifacts_vm.go`** (`ArtifactCardVM`/`BuildArtifactCards`/`artifactTypeLabel`), **`internal/adapter/webui/cockpit_artifacts.templ`** (benannte Klassen `.gallery`/`.artcard*`/`.artupload`, SSE-Container), **`internal/adapter/webui/editor.templ`** (Picker `hx-include`/`hx-params`, Preview `hx-post`/`hx-include`, hidden `node`), **`cmd/flow-mcp/tools_artifacts.go`** (`artifactNode`-Guard, 3 Tool-Handler, `h.do`/`resolveScope`), **`cmd/flow/artifact.go`** (`resolveArtifactNode`, cobra `add/ls/rm`, `--node`), **`internal/adapter/apiclient/artifacts.go`** (`UploadArtifact`/`ListArtifacts`/`DeleteArtifact`, `c.do`), **`internal/testutil/fakes.go`** (`FakeArtifactStore` KOMPLETT, `artifactKey`, `var _ ports.ArtifactStore`), **`internal/domain/event.go`** (`EventArtifact*`), **`cmd/flow-server/main.go`** (Store + 5 Usecases bereits verdrahtet). Migrations-Preflight (`ls … | tail`): höchste = `0031` → `0032` frei.
- **Dossier-Cross-Check (`gemini-bigcontext`-Agent):** ein paralleler Dossier-Auftrag über dieselben 18 Datei-Cluster lief; der Agent wählte **first-hand `Read`/`rg`** statt agy/Gemini-Relay (bewusst, um Transkriptions-/Paraphrasierungs-Risiko bei „wörtlich + Zeilennummern" zu vermeiden) und **bestätigte jeden strukturellen Befund unabhängig**: die harte `node_id NOT NULL`/`UNIQUE(owner,node,slug)`-Sperre in `0031`, `nodeID string` (nicht `*string`) durch das ganze Interface, den `artifactNode`-Guard (lehnt `nil`/„none" ab), die durchgängig chain-basierte Resolution, die bereits owner-scoped Quota, und die node-agnostische Serve-Route (nur `owner+node+slug`). **Kein CLI-Ausfall, kein Degradations-Modus** — First-Hand-Reads sind kanonisch, das Dossier ist Redundanz-Absicherung (Scratch: `free-artifacts-dossier.md`).
- **Flow-Recall:** Slice-Kontext aus dem Dispatch + Memory (`project_flow_rebuild_lesesaal_l6` — L6 DONE; Free-Artifacts = additiver Folge-Slice).

### Spec-Deckung — jeder Abschnitt auf einen Task gemappt
- **§2 E1 (Reichweite, root-oberste Frei-Ebene, node gewinnt)** → Task 2 (`buildArtifactResolver` Frei-Position + Href) + Task 3 (`ListArtifacts` Kette++frei, `buildDocumentVM`/`buildEditorArtifactResolver` frei).
- **§2 E2 (Galerie `/wissen/artefakte`)** → Task 4.
- **§2 E3 (nullable `node_id`, ein Modell)** → Task 1 (Migration + Store).
- **§2 E4 (Cockpit „Frei" geerbt read-only)** → Task 2 (`BuildArtifactCards` frei-Href/„Frei") + Task 3 (`names[""]`-Set, aktiviert durch `ListArtifacts` Kette++frei).
- **§2 E5 (volle L6-Parität)** → über alle Tasks (Web T4, Editor T3, Resolver T2/T3, REST T3, MCP+CLI T5, SSE bereits im Bestand).
- **§3 Datenmodell (Migration-DDL, `""↔NULL`, `Validate` ohne Node)** → Task 1.
- **§4 Store (`ListFree`, `IS NOT DISTINCT FROM`, `Put`-Zweig, Interface-Ripple)** → Task 1.
- **§5 Usecases (Upload-Guard-Skip, `ListArtifacts`-Verzweigung, Rename/Delete/Get NULL-sicher)** → Task 3 (Upload/List) + Task 1 (Store-NULL-Sicherheit deckt Rename/Delete/Get automatisch).
- **§6 Resolver (Frei-Position `len(chain)`, `/artefakte`-Href, Editor-Preview)** → Task 2 (Resolver) + Task 3 (Doc/Editor-Verdrahtung + Picker-Guard).
- **§7 Sanitizer (`artifactSrcRe` beide Formen, Negativtests + Positivkontrolle)** → Task 2.
- **§8.1 Serve (`/artefakte/{slug}`, identische Header-Logik)** → Task 2 (geteilter Helper).
- **§8.2 Web-Galerie (Grid/Upload/Rename/Delete, SSE-Container, Nav)** → Task 4.
- **§8.3 Editor (Picker `?node=` leer → frei, Preview-Frei-Kontext)** → Task 3.
- **§8.4 REST + apiclient (`/api/v1/artifacts`, Frei-Verben)** → Task 3.
- **§8.5 MCP (`node` optional → frei)** → Task 5 (+ OE #1).
- **§8.6 CLI (`--free`, Ausschluss `--node`)** → Task 5.
- **§9 SSE (`artifact.*` aus Usecases, `EventData.node==""`, Galerie-Konsument)** → bereits im Bestand (kein Change), test-gedeckt in Task 3/4; Puls-Feed-Verifikation Task 6.
- **§10 Fehlerbehandlung (Quota summiert frei, Bad-Type/404, Owner-Scope)** → Task 1/3 (Store/Usecase) + je Fläche Negativtest.
- **§11 Pflicht-Tests** → über alle Tasks verteilt (pgstore T1, Resolver/Serve/Sanitizer T2, Usecase/REST/Editor T3, Web-Galerie T4, MCP/CLI T5).
- **§12 YAGNI (kein Inline-Upload, kein Verschieben, keine Ordner, kein Per-Figur-Push, Quota-Race-Soft-Cap)** → bewusst NICHT geplant (im Constraints-Block als akzeptiert vermerkt).
- **§13 Done-Gate (Live-Dogfood)** → Task 6 Step 5.

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Anforderung ohne Task:** keine (Mapping oben vollständig; jeder §-Absatz auf ≥1 Task).
- **(b) Zustände je Task:** T4 benennt leer/lang/mobil-375/Fehler explizit; T2/T3 sind Backend/Render (Zustände = Testfälle: frei-Bild/Datei, node-schlägt-frei, nur-frei, extern/`data:`/`//host`-Reject, 304/Cache-Split, cross-owner); T1/T5 sind Store/CLI/MCP (Zustände = Testfälle: frei-Roundtrip, NULL-Uniqueness-Koexistenz, `--free`/`--node`-Ausschluss, cross-owner-404); T6 ist der Gate. **Kein „laufender Timer"** relevant (Artefakte sind keine Timer-Fläche) — bewusst n. a.
- **(c) Querschnitte:** main.go-Wiring → **kein neuer Usecase/Store** (5 + Store bereits verdrahtet; T6 verifiziert **Routen/Surface**); SSE je Mutation → `artifact.*` bereits aus den Usecases mit `node==""` (T3/T4 test-decken, T6 Puls-Verifikation) mit benanntem Konsument `#wissen-artefakte` (+ `#cockpit-artifacts`); i18n beide Kataloge → T2 (`cockpit.artifacts.free`) + T4 (`wissen.artifacts.*`) + T6-Parity; Responsive → T4 + Gate 960/375; Owner-Scoping → Negativtests T1 (Store/`ListFree`), T2 (`/artefakte`-Serve cross-tenant), T3 (REST/Editor-Picker), T4 (Web-Galerie), T5 (MCP/CLI).
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; Step 0 rg-Verifikation aller Bestandsnamen; „Bestand gewinnt". Die trickreichen Stellen (NULL-sichere Uniqueness/`IS NOT DISTINCT FROM`, nullable node_id-Scan, `Put`-ON-CONFLICT-Zweig, Frei-Position `len(chain)`, Sanitizer-Doppel-Form, Upload-Guard-Skip, cwd-vs-frei-MCP-Semantik) sind je mit eigenem Pflicht-Testfall + rg-Anker abgesichert.

### Adversariale Lückensuche — Berater-Findings + Verbleib

Beide Berater liefen gegen Spec + Plan-Entwurf + Dossier + realen Code mit dem wörtlichen Lücken-Auftrag (Raster a–d). **`codex exec`** (via `codex-second-opinion`-Agent, xhigh, `--sandbox read-only`, aus dem Ziel-Worktree ausgeführt) lief sauber (6 Findings + 8 Antworten, gegen den echten Code verifiziert). **`agy`/Gemini 3.1 Pro** (via `gemini-bigcontext`-Agent) lief sauber (2 verifizierte Gaps + 5 als bereits gedeckt zurückgewiesene Selbst-Falsch-Positive). **Kein `gemini`-CLI-Fallback nötig, kein Degradations-Modus** — beide Sichten vorhanden und komplementär. **Planner-Vorlauf:** VOR den Beratern hatte der Planner bereits zwei First-Hand-Korrekturen eingearbeitet (Editor-Toolbar rendert unbedingt → kein templ-Change für Frei-Button-Sichtbarkeit; Wissen hat keinen Subnav → Galerie-Einstieg als Pagehead-Link) — Gemini bestätigte beide unabhängig als „NOT a gap".

**codex exec — 6 Findings: 5 eingearbeitet, 1 begründet abgelehnt:**
1. **[eingearbeitet — Task 3 `list_artifacts.go`-Guard + Test]** (codex #2, KRITISCH, Codex' Eigenwert) `Nodes.Ancestors` liefert für einen unbekannten/fremden Node `(nil, nil)` statt Fehler (rg-verifiziert `pgstore/nodes.go` + `fakes.go`) → ein leerer Chain fiel durch auf `append([], ListFree(owner)…)`, d. h. `GET /api/v1/nodes/{bogus}/artifacts` hätte die **gesamte freie Bibliothek** des Aufrufers zurückgegeben statt `[]` (API-Kontrakt-Regression, owner-scoped aber falsch). → `if len(chain) == 0 { return nil, nil }`-Guard + Pflicht-Testfall (`Execute(owner,"bogus")` → `[]`).
2. **[eingearbeitet — Task 3 `insertpicker_vm.go`-Dedup + Test + OE #6]** (codex #1) Gleichnamiger Slug an Node UND frei liefert im Editor-Picker **zwei** Rows, die beide `![[slug]]` einfügen (Resolver löst node-wins auf) → stille Fehlleitung. → `BuildArtefaktInsertRows` dedupt je Slug (node-vor-frei-Ordnung gewinnt); Cockpit zeigt bewusst beide (Verwaltungssicht, E4); als OE #6 mit Empfehlung dokumentiert.
3. **[eingearbeitet — Task 1 + Task 3 Testlisten]** (codex #3) Owner-Scope-Negativabdeckung überzeichnet: nur REST-DELETE genannt, nicht `GET /api/v1/artifacts`-List-Isolation; Task 1 nannte `ListFree`-Fremdowner, aber nicht die freien Einzelzeilen-Methoden. → Task 1: Fremdowner-Fälle für frei `Get`/`GetMeta`/`Rename`/`Delete`/`ExistingSlugs`; Task 3: REST-List-Isolation (User B sieht keines von A's freien).
4. **[eingearbeitet — Task 3 Upload-Test]** (codex #4) Frei-Upload-Slug-Kollision nur auf pgstore-Ebene (Task 1 Put-Overwrite) getestet, nicht auf Usecase-Ebene (`ExistingSlugs`+`nextArtifactSlug`). → Task 3 Upload-Test: zweiter frei-Upload gleichen Namens → `-1`-Suffix; + Replace-Pfad (`replaceSlug` → `artifact.updated`).
5. **[eingearbeitet — OE #1 + Task 5 auf `free bool` umgestellt]** (codex #5, blocking) Das MCP-Token `node:"free"` überschattet dauerhaft einen realen Node, dessen Slug/Name „free" ist. → Empfehlung auf einen **separaten `free bool`-Parameter** (mutually exclusive mit `node`) umgestellt — kollidiert mit keinem Node-Namen und lässt omit→cwd-Binding intakt; Token + Spec-Literal als Alternativen in OE #1 belassen.
6. **[begründet ABGELEHNT]** (codex #6) „Flow-Mirror-`type` muss `plan` statt `agent` sein." → **Falsch, verworfen:** die globale `~/.claude/CLAUDE.md` (dem Codex-Prozess nicht sichtbar) schreibt explizit vor: „spec/plan under `docs/superpowers/**` → `type: "agent"`; omit `project`". Der Plan (Dispatch-Protokoll Slice-Ende, `flow_create_doc … type: "agent"`) ist konventionskonform. Der `gemini-bigcontext`-Berater (mein codex-Wrapper) identifizierte dies unabhängig als Fehl-Annahme und riet zum Verwerfen.

**codex — 8 Antworten (bestätigen das Kern-Design, kein Change nötig):** `IS NOT DISTINCT FROM` ≡ `=` für non-NULL (bricht keine Node-Tests); `Get`/`GetMeta`/`ListFree` brauchen nullable node_id-Scan, `List`/`TotalBytes` korrekt unangetastet, keine Methode übersehen; `ON CONFLICT (owner_id,slug) WHERE node_id IS NULL DO UPDATE` = gültige Partial-Index-Arbiter-Syntax; die Sanitizer-Regexp-Alternation (beide Formen, geteiltes `?v=`) korrekt; `freePos==0` beim frei-Doc (leerer Chain) kollidiert nie (ListFree ist owner-global slug-eindeutig). Diese vier trickreichen Stellen sind damit unabhängig bestätigt.

**agy/Gemini 3.1 Pro — 2 verifizierte Gaps (beide eingearbeitet) + 5 zurückgewiesene Selbst-Falsch-Positive:**
1. **[eingearbeitet — Task 2 Sanitizer-Testtext + Files-Zeile]** (gemini #1) Interner Widerspruch: Task 2 Files-Zeile versprach „Sanitizer-Negativ/Positiv **je Form**", der Step-1-Text sagte „je Form irrelevant". Spec §7 vs §11 phrasen die Anforderung uneinheitlich. → Vereinheitlicht: **Positivkontrolle je Form** (`/nodes/…` + `/artefakte/…`, Spec §11 strikt) **+ 3 gemeinsame form-agnostische Negativtests** (die Vektoren extern/`data:`/`//host` matchen keine der erlaubten Formen → ein Node-Negativtest wäre bit-identisch zum Frei-Negativtest); als bewusste, begründete Lesart im Text vermerkt (kein stilles Verengen).
2. **[eingearbeitet — Task 4 Fragment-Route + Container-`hx-get`]** (gemini #2, die höchst-konfidente Lücke) Der `/wissen/artefakte`-SSE-Container hatte keine dedizierte Fragment-only-GET-Route — ein SSE-Trigger hätte die **volle AppShell-Seite** in den Container-`div` geswappt (verschachtelte Seite). Das L6-Muster trennt Page (`/nodes/{id}`) von Fragment (`/nodes/{id}/artifacts`); der Wissen-Bereich nutzt bereits `/ui/wissen/list` dafür. → Task 4: neue Fragment-Route **`GET /ui/wissen/artefakte`** (`handleWebWissenArtifactsFragment`), Container `hx-get="/ui/wissen/artefakte"`, POST-Handler rendern dasselbe Fragment via `renderFreeArtifacts` (rg-verifiziert gegen `cockpit.templ:67` + `server.go:272,327`).
- **[minor, zur Kenntnis]** (gemini #3/#4) Die `wissen.artifacts.*`-Key-**Werte** (DE/EN-Strings) sind im Plantext nicht vorformuliert (nur die Key-Namen) und ein evtl. eigener `deleteConfirm`-Key fehlt — degradiert zu „Implementer wählt Wortlaut" (Parität CI-enforced); in Task 4 als i18n-Step abgedeckt, kein struktureller Gap. Kein zusätzlicher Change.
- **[von Gemini selbst als NOT-a-gap verbucht]** (gemini #5–9) Wissen-Nav (bereits OE #3 + Pagehead-Entscheidung), Editor-Toolbar-Sichtbarkeit für frei (bereits unbedingt gerendert, Task 3 entfernt nur den Daten-Guard), Responsive-375px (Task 4 Zustände + Task 6 Breakpoint), Owner-Quota-frei-Testabdeckung (Tasks 1–5 je Negativtest), §13-Cockpit-Frei-Sichtprobe (Task 6 Step 5) — alle bereits gedeckt, bestätigt Plan-Vollständigkeit.

**Von beiden bestätigt (kein Change nötig):** das eine Modell/Ansatz-A-NULL-Mapping, die root-oberste Frei-Auflösung (Position `len(chain)`, node gewinnt), der geteilte Serve-Header-Helper, die bereits-owner-scoped Quota, die additive main.go-Freiheit (kein neuer Usecase/Store).

**Dissens zwischen den Beratern:** einer — codex #6 (`type:"plan"`) vs. die globale Konvention (`type:"agent"`). **Entscheidung (Planner):** die `~/.claude/CLAUDE.md`-Konvention gewinnt (`type:"agent"` für `docs/superpowers/**`); codex hatte die Instruktionsdatei nicht im Blick. Der Gemini-Wrapper bestätigte diese Auflösung unabhängig.

**Netto aus der Lückensuche:** 1 KRITISCHE Kontrakt-Regression (Bogus-Node-Frei-Leak, codex #2) + 1 KRITISCHE SSE-Wiring-Lücke (Fragment-Route, gemini #2) + 1 blocking MCP-Semantik-Korrektur (`free`-Bool statt Token, codex #5) + 3 Test-/UX-Härtungen (Picker-Dedup, Owner-Scope-Negative, Upload-Kollisions-Test) + 1 Spec-Widerspruch-Auflösung (Sanitizer je-Form) — alle 7 eingearbeitet; 1 Finding begründet abgelehnt (mit CLAUDE.md-Stelle); 5 Gemini-Selbst-Falsch-Positive als bereits gedeckt bestätigt. Kein stilles Verwerfen.
