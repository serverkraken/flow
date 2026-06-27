---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · Baustein 1 — Hierarchie + Bindings — Detail-Spec

**Datum:** 2026-06-27 · **Branch:** `rebuild` (unmerged) · **Status:** approved (Brainstorm abgeschlossen), bereit für Implementation-Plan
**Übersichts-Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (Achsen, Mechanik, D1–D11)
**Vorgänger (Bindings):** `docs/superpowers/specs/2026-06-21-flow-project-resolution-design.md` · **Projects (M4):** `docs/superpowers/specs/2026-06-23-flow-project-management-design.md`

## Ziel

Das flache `projects`-Modell durch eine **rekursive Knoten-Hierarchie** ersetzen, die die reale
Struktur abbildet (Engagement → Vorhaben → Repo) und Kontext-Isolation zwischen Mandaten erzwingt.
B1 liefert das Datenmodell, die Resolution-Primitive (Ahnenkette), das Umhängen von Rate/Worktime auf
Engagement-Ebene, die Daten-Migration des Bestands **und** die strukturellen Hierarchie-Sichten in
TUI + WebUI + CLI. B1 ist die Grundlage für B3 (Kontext-Store walkt die Ahnenkette).

## Scope

**In:**
- `nodes`-Tabelle (rekursiv) durch Evolution von `projects`; Schema-Migration `0015` + idempotenter Daten-Fixup `0016`.
- Voll-Rename `Project` → `Node` quer durch domain/ports/pgstore/usecase/REST/apiclient/webui/tui/cli.
- FK-Rewire `project_id` → `node_id` auf `documents`, `work_sessions`, `project_bindings`.
- kind-Invarianten (CHECK + Usecase), `move`/reparent (zyklenfrei), Ahnenkette-Primitiv.
- Rate + Worktime + Export auf **Engagement**-Ebene (D3).
- Resolution: cwd → Repo (Rename), `Ancestors`, `ResolveEngagement`.
- Daten-Migration des Bestands (Engagements anlegen, Repos einsortieren, Docs per Kategorie, Rates auditen).
- **Strukturelle** Hierarchie-UI: WebUI-Baum + Node-Management, TUI-Node-Tab + Engagement-Picker, CLI `flow node`.
- SSE `project.*` → `node.*`; REST `/projects/*` → `/nodes/*`.

**Out (bewusst):**
- Doc-`path`-*Werte* umschreiben + „schlanker Name"-Konvention (`active-context`) → **B3**.
- Tag-System / Frontmatter-Abschaffung → **B2**.
- Kontext-Inspektor, Lifecycle/„veraltet"-Status, Tag-Chips an Docs, globale Cross-Scope-Suche, Aufräum-Ansicht → **Querschnitt A/B** (nach B2/B3).
- DocumentType-Redesign (agent → spec/plan/instruction/memory/activeContext sauber trennen) → **B3**. B1 lässt `DocumentType` unverändert, setzt nur `node_id`.
- Voller MCP-Redesign (`flow_get_context`) → **B3**. B1 hält die MCP-Tool-*Namen*, macht sie node-aware.
- Branch-/Feature-**Mechanik** (Branch-Resolution, auto-create, branch-scoped activeContext) → **B3**; Lifecycle „gemerged→veraltet" → **Querschnitt A**. B1 **reserviert nur** den kind `branch` im Schema (siehe §13), ohne Verhaltensänderung.
- Monorepo-Sub-Projekte (ein Origin, mehrere Repos via Subpfad) — wie schon in V0 out.

## Entscheidungen (B1-spezifisch, erweitern D1–D11 der Übersicht)

- **B1-1** Eine rekursive `nodes`-Tabelle (Evolution von `projects`), `node_id` ist **nie polymorph** — *ein* FK-Ziel über alle Ebenen. (Gegen zwei Tabellen / Polymorphie.)
- **B1-2** Voll-Rename auf `Node` (DB `nodes`, `domain.Node`, `NodeStore`, Spalte `node_id`, CLI `flow node`). Deckt sich mit der `node_id`/Ahnenkette-Vokabel der Übersicht.
- **B1-3** „global" = **NULL-Sentinel** (keine Knotenzeile). Engagements sind Wurzeln (`parent_id IS NULL`); globale Docs `node_id IS NULL`; Bootstrap = Ahnenkette ∪ global. Reuse der heutigen `project_id NULL`/`project:"none"`-Konvention.
- **B1-4** UI-Tiefe = **Full** (strukturelle Hierarchie-Sichten in TUI+WebUI+CLI). Grenze zu Querschnitt B: B1 = Struktur, Querschnitt B = Kontext/Lifecycle/Tag-Schichten.
- **B1-5** `type` ⊥ `node` ⊥ `tags` — drei unabhängige Achsen. Keine Kategorie fällt weg. Default-Scope je Kategorie (siehe §4).
- **B1-6** Migration: „Privat" (Default) + „RTL Extern" anlegen; Repos per Regel (`slug ~ 'gitlab'` → RTL Extern, sonst Privat); `daily` → RTL-Engagement, `free` → NULL; Rates fallenlassen (Audit in `extra.legacy_rate`), danach manuell am Engagement.
- **B1-7** `daily` ist **pro Engagement** (Strom je Engagement); `free` Default **global (NULL)**.
- **B1-8** Feature-Kontext-Einheit = **Branch**, nicht Worktree (Branch ist cross-device-stabil, git-ablesbar, deckt Worktree *und* Feature-Branch-im-Haupt-Checkout). B1 reserviert kind `branch` unter `repo`; Default-Branch → Repo-Kontext, Feature-Branch → Branch-Node (Mechanik B3). Amendiert D4 der Übersicht (activeContext wird branch-scoped, nicht repo-scoped).
- **B1-9** Nicht-Code-Projekte (kein git-Repo) = **Blatt-`vorhaben`** (kein eigenes kind, §14); Docs/activeContext hängen direkt am Vorhaben. **path-Bindung → `repo` oder Blatt-`vorhaben`**; **remote-Bindung → nur `repo`** (origin ist git-spezifisch).

---

## 1 · Datenmodell — `nodes`

`projects` (`0002_project_worksession.sql`, erweitert in `0005`/`0014`) wird zu `nodes`. Spalten der
bestehenden Tabelle bleiben; ergänzt werden `parent_id`, `kind`, `origin_slug`, `extra`.

```sql
-- Zielzustand nodes (kumuliert)
CREATE TABLE nodes (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL REFERENCES users(id),
    parent_id     TEXT     REFERENCES nodes(id) ON DELETE RESTRICT,   -- NULL = Wurzel
    kind          TEXT NOT NULL CHECK (kind IN ('engagement','vorhaben','repo','branch')),
    name          TEXT NOT NULL,
    slug          TEXT NOT NULL,
    color         TEXT NOT NULL DEFAULT '',
    glyph         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    rate_amount   BIGINT,            -- nur engagement (CHECK)
    rate_currency TEXT,
    origin_slug   TEXT,              -- nur repo (CHECK); git-origin-Slug am Blatt
    upstream_git  TEXT NOT NULL DEFAULT '',
    extra         JSONB NOT NULL DEFAULT '{}',   -- u.a. legacy_rate-Audit
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug),
    CONSTRAINT nodes_root_is_engagement CHECK (parent_id IS NOT NULL OR kind = 'engagement'),
    CONSTRAINT nodes_rate_only_engagement CHECK (rate_amount IS NULL OR kind = 'engagement'),
    CONSTRAINT nodes_origin_only_repo CHECK (origin_slug IS NULL OR kind = 'repo')
);
CREATE INDEX nodes_owner ON nodes (owner_id);
CREATE INDEX nodes_parent ON nodes (parent_id);
```

> `upstream_git` (aus `0014`) bleibt vorerst auf allen Knoten erlaubt (kein neuer CHECK), faktisch nur am Repo gesetzt. `origin_slug` ist neu und ist die *resolutionsrelevante* Spalte; `upstream_git` bleibt das anzeigbare Clone-URL-Feld aus M4.

**Statische Invarianten (CHECK, oben):** kind-Enum · Wurzel⇒engagement · rate nur engagement · origin nur repo.

**Cross-Row-Invarianten (Usecase, da nicht statisch in SQL):**
- `vorhaben`/`repo` brauchen `parent_id`; deren Parent-`kind` ∈ {engagement, vorhaben}.
- `branch` (in B1 reserviert, §13): `parent_id` Pflicht, Parent-`kind` = `repo`.
- Blatt-Regeln: `repo`-Kinder dürfen **nur** `kind='branch'` sein; `branch` ist immer Blatt (keine Kinder). Create/Move lehnt Verstöße ab.
- `move`/reparent ist **zyklenfrei**: Ziel-Parent darf nicht der Knoten selbst oder einer seiner Nachfahren sein (Ahnenkette des Ziels prüfen).
- Folge-Invariante (automatisch erfüllt): jeder Repo hat eine engagement-Wurzel als Vorfahr (D2 „nichts hängt frei").

**Delete-Semantik:** `parent_id … ON DELETE RESTRICT` ⇒ ein Knoten mit Kind-Knoten kann nicht gelöscht werden (erst leeren/umhängen). Passt zu „markieren statt löschen" (Querschnitt A); Hard-Delete bleibt selten + manuell.

## 2 · Domain — `Node`

`internal/domain/project.go` → `node.go`. `domain.Project` (Felder: `project.go:21–32`) → `domain.Node`:

```go
type NodeKind string
const (
    KindEngagement NodeKind = "engagement"
    KindVorhaben   NodeKind = "vorhaben"
    KindRepo       NodeKind = "repo"
    KindBranch     NodeKind = "branch"   // in B1 nur reserviert (§13); Mechanik B3
)

type Node struct {
    ID          string
    OwnerID     string    `json:"-"`
    ParentID    *string   // nil = Wurzel (Engagement)
    Kind        NodeKind
    Name        string
    Slug        string
    Color       string
    Glyph       string
    Description string
    Status      NodeStatus   // war ProjectStatus
    Rate        *Money       // nur engagement; nil = unset
    OriginSlug  string       // nur repo
    UpstreamGit string
    Extra       map[string]any
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

Pure Validierungs-Helfer (Domain-Tests, kein DB-Zugriff):
- `func ValidParentKind(child, parent NodeKind) bool` — die kind-Regeln aus §1.
- `func ResolveEngagement(chain []Node) (Node, bool)` — oberster Knoten einer (geordneten) Ahnenkette; `kind=engagement` erwartet.
- `func WouldCycle(targetParentID string, subtreeAncestorsOfTarget []Node, movingNodeID string) bool` — Zyklen-Check (oder äquivalent als Set-Check im Usecase).

`ProjectStatus` (`project.go`) → `NodeStatus` (Werte unverändert: active/paused/archived).

## 3 · FK-Rewire — drei Tabellen auf `node_id`

| Tabelle | heute | nachher | FK-Verhalten |
|---|---|---|---|
| `documents` | `project_id TEXT → projects(id)` (`0006:3`, FK `0012`) | `node_id TEXT → nodes(id)` | `ON DELETE SET NULL` bleibt |
| `work_sessions` | `project_id` (`0002`, FK `0012`) | `node_id` | `ON DELETE SET NULL` bleibt |
| `project_bindings` | `project_id → projects(id) ON DELETE CASCADE` (`0011:5`) | `node_id → nodes(id) ON DELETE CASCADE` | CASCADE bleibt |

- `documents` Unique-Index `documents_owner_project_path` (`0006:16–18`, `coalesce(project_id,'')`) → **`documents_owner_node_path`** auf `(owner_id, coalesce(node_id,''), path)`. **`path`-Werte unverändert** (lean-name = B3).
- `project_bindings`-Indizes (`0011`) referenzieren keine projects-Spalte direkt; nur die FK-Spalte wird umbenannt. Binding-Target je `BindingKind`: **remote-Bindung → nur `repo`** (git-origin), **path-Bindung → `repo` oder Blatt-`vorhaben`** (Nicht-Code-Projekte, §14). Usecase erzwingt das beim Bind.
- `domain.Document.ProjectID *string` (`document.go:61`) → `NodeID *string`; `"none"`-Sentinel-Konvention (Query = nur unzugeordnete) bleibt 1:1, jetzt „nur globale" (`node_id NULL`).
- `domain.WorkSession.ProjectID *string` (`worksession.go:13`) → `NodeID *string`; zeigt auf ein **Engagement**.

## 4 · Drei Achsen + Default-Scope je Kategorie

`type` (*was es ist*) ⊥ `node` (*wo es hängt*) ⊥ `tags` (*Quer-Thema*, B2). B1 fasst `DocumentType`
(`document.go:13–20`: daily/project/free/agent/memory/instruction/skill/plan) **nicht** an — es vergibt
nur `node_id`. Default-Scope (wo *neue* Docs landen; frei überschreibbar):

| `type` | Default-`node` | Begründung |
|---|---|---|
| `daily` | aktives **Engagement** | Arbeits-Journal; Buchung = Engagement (D3). Strom je Engagement. |
| `project` | **Repo** | handelt von konkretem Codebase |
| `agent` | **Repo** | Planungs-Docs (Specs/Plans) zu einem Codebase |
| `free` | **global (NULL)** | ungebunden, persönlicher Quer-Scratch |
| `instruction` / `memory` | **jede Ebene** | CLAUDE.md/Fakten auf global/engagement/vorhaben/repo |
| `skill` / `plan` | repo/global | wie heute, nur node-getaggt |

Default-Scope-Vergabe lebt in den Schreib-Usecases/Handlern, **nicht** in der DB (kein Trigger). Für B1
genügt: Create-Pfade setzen `node_id` aus dem aufgelösten Kontext (cwd→Repo, bzw. dessen Engagement für
`daily`), UI lässt den Scope wählen.

## 5 · Resolution + Ahnenkette

- `usecase.ResolveProject` (`resolve_project.go:17`) → `ResolveNode`; Signatur unverändert
  (`Execute(ctx, ownerID, remoteSlug, machineID, cwd) (Node, bool, error)`), liefert jetzt einen **Repo**-Node (oder ein Blatt-`vorhaben`, wenn eine path-Bindung auf ein Nicht-Code-Projekt zeigt, §14).
  `internal/projectresolve/resolve.go:34` zieht nach (`projectresolve` → `noderesolve`, oder Paketname behalten, Rückgabetyp `domain.Node`).
- **Neu** `NodeStore.Ancestors(ctx, ownerID, nodeID string) ([]Node, error)` — ein `WITH RECURSIVE`-CTE
  parent_id-Walk, geordnet **Blatt → Wurzel**. Das ist das Primitiv, das B3 (Bootstrap-Compose) konsumiert.
  Pures Pendant für Tests: Resolver bekommt die Liste, Domain ordnet/validiert.
- **Neu** `usecase.ResolveEngagement` — `ResolveNode` (cwd→Repo) → `Ancestors` → `domain.ResolveEngagement`.
  Worktime nutzt das, um die Buchungs-Ebene zu bestimmen.
- `NodeStore.Children(ctx, ownerID, parentID *string) ([]Node, error)` — für Baum-Rendering (parentID nil = Wurzeln/Engagements).

## 6 · Rate + Worktime auf Engagement (D3)

- `ProjectStore.SetRate` (`ports.go:84`) → `NodeStore.SetRate`; erzwingt `kind='engagement'` (sonst Fehler).
  `Node.Rate` ist nur am Engagement gesetzt; `Update` rührt Rate weiterhin nicht an (`ports.go:79–81`-Konvention bleibt).
- `usecase.StartSession` (`start_session.go:18`) + `AddSession` (`add_session.go:21`): Parameter `projectID *string`
  → `nodeID *string`; Validierung „node existiert + `kind='engagement'`" (statt beliebiges Projekt).
  Picker/Caller liefern künftig ein Engagement.
- `usecase.BuildExport` (`export.go:31`) aggregiert pro **Engagement** statt pro Projekt
  (`ProjectTotal` → `EngagementTotal`/`NodeTotal`; Σ time.Duration je Engagement, `Rate.Mul(total)` wie `export.go:88–99`).
- `SessionStore.Stop`/`Update` (`ports.go:97,104`): `projectID *string` → `nodeID *string`.

## 7 · API / Client / SSE / CLI

**REST** (`internal/adapter/httpserver/`): `/api/v1/projects/*` → `/api/v1/nodes/*`; bindings-Routen
(`server.go:122–127`, `projectbindings.go`) darunter umhängen. Handler/Server-Felder `Project*` → `Node*`.
Neue Routen: `GET /nodes?tree=…` (oder client-seitiger Baum aus flacher Liste), `POST /nodes/{id}/move`.

**apiclient** (`internal/adapter/apiclient/`): `projects.go`/`projectbindings.go` → node-Methoden;
`ResolveProject` → `ResolveNode`, neu `Ancestors`, `MoveNode`.

**SSE** (`internal/domain` Event + Bus): `project.created/updated/deleted` → `node.created/updated/moved/deleted`.
Worktime/Docs-Live-Sync (TUI shell `EventMsg`, WebUI `sse-swap`) konsumieren `node.*`.

**CLI** (`cmd/flow/`): `flow project` → `flow node`:
```
flow node create <name> --kind engagement|vorhaben|repo [--parent <slug> --upstream … --color … --glyph … --desc …]
flow node list   [--tree] [--kind …] [--status active|paused|archived|all]
flow node show   [<slug>]            # default: cwd-resolved repo + Ahnenkette
flow node move   <slug> --parent <slug>     # reparent, zyklenfrei
flow node rate   <engagement-slug> <amount> <currency>
flow node bind|unbind|bindings       # remote→repo; path→repo oder Blatt-vorhaben (§14)
flow node pause|resume|archive <slug>
flow node rm     <slug>              # RESTRICT: nur wenn kinderlos
```
(Verben am User-Review justierbar; kind-spezifische Aliase wie `flow engagement` optional later.)

**MCP** (`internal/adapter/mcp` o.ä.): `flow_list_projects`/`flow_project_context`/`flow_bind_project`
**Namen bleiben** (B1), werden node-aware (Rückgabe trägt `kind`, `parent`, Ahnenkette). Voller Redesign
(`flow_get_context`) = B3.

## 8 · UI (Full-Scope) — strukturelle Hierarchie-Sichten

**WebUI** (`internal/adapter/webui/`, templ + htmx, Studio-Tokens, `ui/badge`/`ui/chip`/`kindcolor`):
- Node-**Baum** (Engagement → Vorhaben → Repo) als Hauptansicht der Node-Verwaltung; kind-Badges via `kindcolor`.
- Create/Edit-Form (name·slug·kind·parent·color·glyph·desc·upstream); Engagement zusätzlich Rate; **Move**-Aktion (Parent-Picker).
- Repo-Detail: Bindings-Panel (read-only, wie M4), Ahnenkette als Breadcrumb.
- Worktime-Picker (`heute`/`historie`-Fragmente) listet **Engagements**; Doc-Listen zeigen Repo/Engagement-Name + kind statt flachem Projekt.

**TUI** (`internal/tui/`, shell-Routen, `theme`/`ui`):
- Neuer **Node-Tab** (oder Erweiterung des bestehenden „Projekte"): Baum-Navigation (fuzzy + kind-Filter via `ui/fuzzylist`), Detail-Cockpit, `move`/reparent-Dialog, kind-Badges (`kindcolor`/`ui/badge`).
- Worktime-Booking-Dialog: Engagement-Picker (MRU + fuzzy, `ui/fuzzylist`) statt Projekt-Picker.

**Glyph/Farbe:** kind über `kindcolor` (Whitelist, monospace). Keine Emoji ([[feedback_no_icons]]).

## 9 · Migration

**0015 — Schema (rein strukturell, idempotent via goose):**
1. `ALTER TABLE projects RENAME TO nodes;` + Index/Constraint-Namen mitziehen.
2. `ADD COLUMN parent_id`, `kind` (zunächst `DEFAULT 'repo'` zum Befüllen, danach Default droppen), `origin_slug`, `extra JSONB DEFAULT '{}'`.
3. CHECKs aus §1 (nach dem Daten-Fixup *gültig*; siehe Reihenfolge unten).
4. `documents`/`work_sessions`/`project_bindings`: Spalte `project_id` → `node_id` (RENAME COLUMN), FK-Namen/Targets auf `nodes` ziehen, Doc-Unique-Index → `documents_owner_node_path`.
5. `nodes_owner`/`nodes_parent`-Indizes.

> Goose-Annotations beachten ([[feedback_pgstore_goose_migrations]]): `-- +goose Up`/`Down`, sonst Apply-Fehler. Reihenfolge: Spalten+Rename in 0015, **CHECKs erst nachdem 0016 die Daten konsistent gemacht hat** — d.h. die CHECK-Constraints kommen ans **Ende von 0016** (oder ein `0017`), damit der Zwischenzustand sie nicht verletzt. (Plan-Detail; Default-Variante: 0015 ohne CHECKs, 0016 macht Daten konsistent + fügt CHECKs hinzu.)

**0016 — Daten-Fixup (idempotent, owner-scoped pro User):**
1. Engagement **„Privat"** (slug `privat`) + **„RTL Extern"** (slug `rtl-extern`) je Owner anlegen, falls fehlend (`kind=engagement`, `parent_id NULL`).
2. Bestehende Knoten (die Alt-Projekte) → `kind='repo'`; `parent_id` = (`slug ILIKE '%gitlab%'` → `rtl-extern`, sonst → `privat`); `origin_slug` aus vorhandenem Binding/`upstream_git` ableiten falls vorhanden.
3. Repo-Rate auditen + leeren: `extra = jsonb_set(extra,'{legacy_rate}', {amount,currency})` wenn gesetzt, dann `rate_amount=NULL, rate_currency=NULL` (CHECK `rate_only_engagement` wird damit erfüllbar).
4. Docs per Kategorie: `type='daily'` → `node_id` = RTL-Engagement; `type='free'` → `node_id NULL`. `type IN ('project','agent',…)` behalten ihr `node_id` (= Alt-`project_id`, jetzt Repo). Bereits NULL-globale (`instruction`/`memory`) bleiben NULL.
5. `work_sessions`: `node_id` (= Alt-Repo) → **Engagement**-Vorfahr umschreiben (`UPDATE … SET node_id = (SELECT engagement via Ahnenkette)`), da Buchung künftig Engagement-Ebene (D3).
6. Danach CHECK-Constraints aus §1 hinzufügen (`ADD CONSTRAINT … CHECK …`).

> Die Migration trägt nur die **Regel** (`%gitlab%` → RTL, sonst Privat), keine gepflegte Slug-Liste. Unsichere Zuordnungen landen sicher unter „Privat" und werden danach per `flow node move` / Baum-UI umgehängt. Engagement-Rates setzt Soenne manuell (`flow node rate "rtl-extern" 95 EUR`).

## 10 · Datei-Änderungs-Karte (für den Plan)

- **domain:** `project.go`→`node.go` (+ `NodeKind`/`NodeStatus`/Validatoren), `document.go` (`ProjectID`→`NodeID`), `worksession.go` (`ProjectID`→`NodeID`), `projectbinding.go` (`ProjectID`→`NodeID`, `ResolveBinding` Rückgabe-Doku), Events (`project.*`→`node.*`).
- **ports:** `ports.go` — `ProjectStore`→`NodeStore` (+ `Ancestors`/`Children`, `SetRate` kind-guard), `SessionStore`/`DocumentStore`-Signaturen (`projectID`→`nodeID`).
- **pgstore:** `projects.go`→`nodes.go` (+ rekursiver Walk), `documents.go`/`worksessions.go`/`projectbindings.go` Spalten-Rename; Migrationen `0015`,`0016`.
- **usecase:** `resolve_project.go`→`resolve_node.go`, neu `resolve_engagement.go`, `move_node.go`; `start_session.go`/`add_session.go`/`export.go`/`bind_project.go`/… Param-Rename + kind-Validierung.
- **adapter/httpserver:** `projects.go`/`projectbindings.go` Routen `/nodes/*` + `move`; `server.go` Wiring.
- **adapter/apiclient:** `projects.go`/`projectbindings.go` node-Methoden.
- **projectresolve:** Rückgabetyp `domain.Node`.
- **adapter/webui:** Node-Baum + Form + Move; Worktime-/Docs-Listen node-aware; `kindcolor` kind-Mapping.
- **internal/tui:** Node-Tab/Baum, Engagement-Picker im Booking, kind-Badges.
- **cmd/flow:** `project.go`→`node.go` (Verben §7), `worktime.go`/`export.go` Engagement-Wiring.
- **mcp:** node-aware Rückgaben, Tool-Namen unverändert.

## 11 · Testing-Strategie

TDD durchgängig ([[feedback_plan_main_wiring_task]] — finaler Wiring-Task mit curl-Smoke jeder Route):
- **Domain:** `ValidParentKind`, `ResolveEngagement`, Zyklen-Check, `ResolveBinding` (Ziel=Repo) — pure Tests.
- **pgstore (Docker):** Migration 0015+0016 (Bestand→Hierarchie, Rate-Audit, daily→RTL, sessions→Engagement), rekursiver `Ancestors`-Walk, kind-CHECKs greifen, RESTRICT-Delete.
- **usecase:** `ResolveNode`/`ResolveEngagement`/`MoveNode` (inkl. kind-Verstöße + Zyklen) via Fakes.
- **REST:** httptest für `/nodes/*` + `move` (owner-scope, 404 fremd, 400 bei kind-Verstoß).
- **apiclient/webui(templ)/tui(fake-apiclient)/cli(cmd):** je Layer.
- **Done-Gate:** `make ci` grün (Coverage-Gate halten) + Live-Smoke vs. Postgres+Dex (Resolution cwd→Repo→Engagement, Worktime bucht auf Engagement, Export Σh×Rate je Engagement, Baum-UI rendert).

## 12 · Abhängigkeiten / Reihenfolge

B1 ist Voraussetzung für **B3** (Ahnenkette). B2 (Tags) ist unabhängig/parallel. Innerhalb B1 empfiehlt
sich der Schnitt: **(a) Datenmodell+Migration+Resolution (Backend) → (b) Rate/Worktime/Export auf Engagement
→ (c) API/SSE/CLI → (d) UI (WebUI+TUI) → (e) Wiring-/Done-Gate** — der Implementation-Plan schneidet das in Tasks.

## 13 · Branch-Ebene (in B1 nur reserviert)

**Feature-Kontext-Einheit = Branch, nicht Worktree** (B1-8). Ein Worktree ist nur „ein Branch, ausgecheckt
an einem Pfad"; Branch-Keying deckt Worktree *und* Feature-Branch-im-Haupt-Checkout, ist cross-device-stabil
(gleicher Branch-Name in jedem Clone) und git-ablesbar — keine per-PC-Pfad-Bindung, keine Präzedenz-Umkehr.

**Was B1 tut (Modell zukunftssicher, billig):**
- kind `branch` im Enum + Invarianten (§1): `branch.parent = repo`; `repo`-Kinder nur `branch`; `branch` ist Blatt.
- Ahnenkette ist tiefen-agnostisch → `branch → repo → [vorhaben] → engagement → global` funktioniert ohne Zusatzcode.
- **Kein Verhaltenswechsel:** B1 legt keine Branch-Nodes an; Resolution bleibt origin→Repo. `node_id` auf Docs/Sessions zeigt in B1 nie auf einen Branch.

**Was später kommt (Mechanik):**
- **B3:** zweidimensionale Resolution — `origin-slug → Repo` **und** `git branch --show-current → Branch-Node` (neuer Reader `gitremote.CurrentBranch`, analog `OriginSlug`). Default-Branch (`main`) → Repo-Kontext (kein eigener Node); Feature-Branch → Branch-Node (auto-create beim ersten Kontext-Write, z.B. activeContext via Stop-Hook). **activeContext wird branch-scoped** statt repo-scoped — amendiert D4 der Übersicht.
- **Querschnitt A:** Lifecycle „Branch gemerged/gelöscht → Branch-Node + Feature-Kontext auf `veraltet`" (soft, reversibel), nicht hart löschen.

## 14 · Nicht-Code-Projekte (kein git-Repo)

Das Modell trägt Kontext für Arbeit **ohne** Sourcecode, weil `node_id` (Docs/Sessions) auf *jeden* Knoten
zeigt — nicht nur auf Repos. Ein Nicht-Code-Projekt ist ein **Blatt-`vorhaben`** (B1-9): ein `vorhaben`
ohne Repo-Kinder, an dem Docs/activeContext direkt hängen. Kein eigenes kind nötig („Vorhaben" = Projekt/Unterfangen).

```
Privat (engagement)
  ├─ flow (repo)                  ← Code
  └─ Buch schreiben (vorhaben)    ← Nicht-Code-Projekt: Docs/activeContext direkt am Vorhaben
RTL Extern (engagement)
  └─ Steuerkram 2026 (vorhaben)   ← Nicht-Code-Projekt
```

**Drei Zugangswege (Resolution):**
1. **Mit Arbeitsverzeichnis (kein git):** **path-Bindung → Blatt-`vorhaben`** (das path-Tier matcht jedes Verzeichnis per longest-prefix, nicht nur git-Repos). Dafür wird die Binding-Target-Regel geöffnet (§3): remote→repo, path→repo|Blatt-vorhaben.
2. **Ohne Verzeichnis (rein konzeptionell):** keine Bindung; **explizite Auswahl** via Picker / CLI (`flow node show <slug>`) / `FLOW_PROJECT`-Override. Kontext lebt am Knoten, wird durch Auswahl gezogen.
3. **activeContext** eines Nicht-Code-Projekts ist **vorhaben-scoped** (das Blatt) — analog repo-/branch-scoped beim Code (Mechanik in B3).

**B1-Umfang hier:** nur die Binding-Target-Lockerung (§3) + die Bestätigung, dass Docs an Blatt-vorhaben hängen
dürfen (gilt durch das `node_id`-FK-Modell ohnehin). Auto-create + vorhaben-scoped activeContext = B3.
