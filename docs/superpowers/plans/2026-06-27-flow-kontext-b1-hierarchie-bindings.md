---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · B1 — Hierarchie + Bindings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `projects` model with a recursive `nodes` hierarchy (Engagement → Vorhaben → Repo [→ Branch reserved]), move rate/worktime/export to the engagement level, migrate the existing data, and ship the structural hierarchy UI across TUI + WebUI + CLI.

**Architecture:** Single recursive `nodes` table (evolution of `projects`), `node_id` FK on documents/work_sessions/project_bindings (never polymorphic), `WITH RECURSIVE` ancestor walk as the B3-ready primitive. Full mechanical `Project→Node` rename first (behavior-identical, CI green), then hierarchy features layered on. Hexagonal: domain → ports → usecase → adapters (pgstore/httpserver/apiclient/webui/tui) → cmd wiring.

**Tech Stack:** Go, Postgres + pgx/v5 + goose migrations, templ + htmx + Tailwind (WebUI), charm.land/bubbletea-v2 + lipgloss-v2 + internal/tui design system (TUI), cobra (CLI).

**Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md` (read it; this plan implements §1–§14). **Übersicht:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (D1–D11 + B1-1…B1-9).

## Global Constraints

- Module path `github.com/serverkraken/flow`. Hexagonal — one responsibility per file, no monoliths.
- **Canonical names (obey verbatim across all slices):** `domain.Node`, `NodeKind` (`KindEngagement|KindVorhaben|KindRepo|KindBranch`), `NodeStatus` (`NodeActive|NodePaused|NodeArchived`), `ports.NodeStore`, `ports.ErrNodeNotFound`, `ports.ErrNodeHasChildren`, `node_id` FK column, `domain.Document.NodeID`, `domain.WorkSession.NodeID`, events `node.created|updated|moved|deleted`, REST `/api/v1/nodes/*`, CLI `flow node`, `domain.ProjectBinding.NodeID` (the *binding store* keeps the name `ProjectBindingStore`/`NewProjectBindingStore` — only the field renames).
- **Hierarchy invariants:** root (`parent_id IS NULL`) ⇒ `kind=engagement`; `vorhaben`/`repo` parent ∈ {engagement, vorhaben}; `branch.parent = repo`; repo's children may only be `branch`; `branch` is a leaf. Static CHECKs: kind-enum, root-is-engagement, rate-only-engagement, origin-only-repo. Cross-row rules (leaf/parent-kind, cycle-free move) enforced in usecases.
- **global = NULL:** global docs have `node_id IS NULL`; unique `documents(owner_id, coalesce(node_id,''), path)`. Doc `path` *values* are NOT changed in B1 (B3).
- **Worktime = engagement (D3):** sessions store an engagement `node_id`; StartSession/AddSession/StopSession reject non-engagement; rate only on engagement; export aggregates per engagement.
- **`branch` kind is reserved only** in B1 (no behavior; mechanics → B3). **Non-code projects = leaf `vorhaben`**; path-binding may target repo *or* leaf-vorhaben, remote-binding only repo.
- **Migrations:** goose-annotated (`-- +goose Up`/`Down`); applied on server boot via `pgstore.Migrate` (no standalone goose target — "goose up" == `make dev-run`); verified by Docker pgstore tests ([[feedback_pgstore_goose_migrations]]).
- **Tests:** `package <pkg>_test`, `t.Parallel()`, table-driven where natural; httptest for handlers; fake-apiclient for TUI routes; Docker Postgres for pgstore.
- **CI:** `make ci` (gofumpt + staticcheck incl. QF1002, verify-generate/css/no-popups, coverage gate, build) must stay green and the coverage gate must not regress. Run `templ generate` after any `.templ` change and commit the generated `_templ.go`.
- **i18n:** WebUI strings via `T(ctx,"key")`/`Tn`; add keys to BOTH `internal/i18n/catalog_de.go` (full) + `catalog_en.go` (stub). German primary.
- **Glyphs:** monospace whitelist only, no emoji ([[feedback_no_icons]]). Colors via `theme.Sem()`/`kindcolor` only.
- **Owner-scoping** on every store/usecase call. Foreign ids never leak or mutate.
- Every `git commit` message ends with the trailer `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

## Slice map (execution order; each slice is independently testable + `make ci` green)

| Slice | Deliverable |
|---|---|
| **A — Foundation** | mechanical `Project→Node` rename (CI green) → hierarchy schema + domain Node + validators + NodeStore (Ancestors/Children/Reparent) + migrations 0015/0016 + resolution (resolve_node/resolve_engagement/move_node/create_node/bind_node) |
| **B — Rate/Worktime/Export** | StartSession/AddSession/StopSession require engagement; SetNodeRate kind-guard; BuildExport per engagement (`NodeTotal`/`ByEngagement`); composition wiring |
| **C — API/SSE/CLI** | `node.moved` SSE; `POST /nodes/{id}/move` + node REST surface; apiclient MoveNode/Ancestors; `flow node` (create/list --tree/show/move/rate/bind/…) |
| **D — WebUI** | node tree + create/edit/move form + node-aware lists + engagement booking selector + WebUI kind badges |
| **E — TUI** | `kindcolor` NodeKind mapping + node tree screen (move/detail) + engagement picker in worktime booking |
| **F — Wiring/Done-Gate** | composition-root audit + migration apply/idempotency + curl-smoke every route + live dogfood + `make ci` + memory/spec status |

> **Cross-slice note:** Slice A owns the *atomic* rename (Task A1) so the repo compiles; B–E assume renamed code + the NodeStore contract and only ADD their new behavior. Slice F's wiring audit is the backstop.
>
> **Cross-slice reconciliation (these PIN the genuinely ambiguous shared shapes — Slice A is authoritative; obey these over any divergent "Consumes" annotation in a later slice):**
> 1. **CreateNode / UpdateNode take struct inputs, not positional args:** `usecase.CreateNode.Execute(ctx, ownerID string, in usecase.CreateNodeInput) (domain.Node, error)` with `CreateNodeInput{Name, Slug string; Kind domain.NodeKind; ParentID *string; Color, Glyph, Description, UpstreamGit string}`; `usecase.UpdateNode.Execute(ctx, ownerID, id string, in usecase.UpdateNodeInput) (domain.Node, error)`. Any slice that wrote a positional `CreateNode.Execute(…,name,slug,kind,parentID,color,glyph)` means the struct form. (Slice A defines these; Slice C's create handler/CLI and Slice D's form consume the struct.)
> 2. **Migration numbering is three files** (Slice A split the contract's 0015/0016 to keep every step `make ci`-green and the data-fixup testable against pre-CHECK rows): `0015_rename_projects_to_nodes.sql` (table+column rename only), `0016_nodes_hierarchy_columns.sql` (parent_id/kind[temp DEFAULT 'repo']/origin_slug/extra, no CHECKs), `0017_nodes_data_fixup.sql` (Privat/RTL Extern, gitlab→RTL, rate audit, daily→RTL, free→NULL, sessions→engagement, drop kind default, ADD CHECKs). Slice F's assertions are file-number-agnostic; the contract's "0015/0016" labels map onto this 3-file scheme.
> 3. **Export JSON wire key:** Slice B4 renames the Go field to `ByEngagement` but **keeps the serialized JSON key `byProject`** for wire-stability within B. If a later slice relabels it to `byEngagement`, the Slice-F smoke (F3 Step 12) must assert that key — F3 already hedges. The done-gate asserts whichever key the committed code emits.

---

## Slice A — Foundation

> **Migration-numbering note (read first):** the parent split the contract's `0015_nodes_hierarchy.sql` into a *rename-only* `0015` (Task A1). Because (a) every task must leave `make ci` green and (b) the data-fixup must be **testable against pre-CHECK legacy rows**, the hierarchy columns ship in their own `0016_nodes_hierarchy_columns.sql` (Task A3, no CHECKs, `kind DEFAULT 'repo'`) and the contract's `0016` data-fixup becomes **`0017_nodes_data_fixup.sql`** (Task A6, data + drop-default + CHECKs). Content is exactly the contract's `0016` outline; only the file number shifts by one because A1 consumed `0015`.

---

### Task A1: Mechanical rename `Project` → `Node` (zero behavior change)

Pure rename across the whole repo (Go + `.templ`) plus a rename-only migration so the renamed code matches the DB. The model stays **FLAT** (no `parent_id`/`kind` yet). This is allowed to be an exhaustive symbol/file mapping + verification (per the parent's instruction).

**Files (modify):** every Go/templ file returned by the `rg` queries in Step 9 below. **Do NOT touch `docs/**`** (historical plans/specs keep old names). Key file *renames* (use `git mv`):
- `internal/domain/project.go` → `internal/domain/node.go`
- `internal/domain/projectstyle.go` → `internal/domain/nodestyle.go`; `internal/domain/projectstyle_test.go` → `internal/domain/nodestyle_test.go`
- `internal/adapter/pgstore/projects.go` → `internal/adapter/pgstore/nodes.go`; `internal/adapter/pgstore/projects_test.go` → `internal/adapter/pgstore/nodes_test.go`
- `internal/usecase/resolve_project.go` → `resolve_node.go`; `bind_project.go` → `bind_node.go`; `unbind_project.go` → `unbind_node.go`; `list_project_bindings.go` → `list_node_bindings.go`; `create_project.go` → `create_node.go`; `get_project.go` → `get_node.go`; `list_projects.go` → `list_nodes.go`; `update_project.go` → `update_node.go`; `delete_project.go` → `delete_node.go`; `set_project_rate.go` → `set_node_rate.go`; `bulk_assign_project.go` → `bulk_assign_node.go` (+ matching `*_test.go`)
- `cmd/flow/project.go` → `cmd/flow/node.go`; `cmd/flow/project_test.go` → `cmd/flow/node_test.go` (keep `projectbind*.go` filenames — bindings type names are *preserved*, see below)
- **Migration (create):** `internal/adapter/pgstore/migrations/0015_rename_projects_to_nodes.sql`

**Interfaces — exact rename map (the contract is authoritative):**

| Old symbol | New symbol |
|---|---|
| `domain.Project` (struct) | `domain.Node` |
| `domain.ProjectStatus` | `domain.NodeStatus` |
| `domain.ProjectActive/Paused/Archived` | `domain.NodeActive/NodePaused/NodeArchived` (values `active/paused/archived` unchanged) |
| `domain.NewProject` | `domain.NewNode` |
| `(Project).Validate` | `(Node).Validate` |
| `domain.ValidProjectColor/ValidProjectGlyph` | `domain.ValidNodeColor/ValidNodeGlyph` |
| `domain.ProjectColors/ProjectGlyphs` | `domain.NodeColors/NodeGlyphs` |
| `domain.ErrInvalidProject` | `domain.ErrInvalidNode` |
| `domain.Document.ProjectID` | `.NodeID` |
| `domain.WorkSession.ProjectID` | `.NodeID` (param `projectID *string` in `NewWorkSession` → `nodeID *string`) |
| `domain.ProjectBinding.ProjectID` | `.NodeID` (**type name `ProjectBinding` stays**; `BindKey` stays) |
| `domain.ProjectTotal` | `domain.NodeTotal` (fields `ProjectID`→`NodeID`, `ProjectName`→`NodeName`) |
| `domain.ExportData.ByProject` | `.ByEngagement` (type `[]NodeTotal`) |
| `domain.ExportRow.ProjectName` | `.NodeName` |
| `domain.EventProjectCreated/Updated/Deleted` | `domain.EventNodeCreated/Updated/Deleted` (string values `node.created/updated/deleted`) |
| `ports.ProjectStore` | `ports.NodeStore` |
| `ports.ErrProjectNotFound` | `ports.ErrNodeNotFound` |
| `ports.SessionStore.Stop/Update` param `projectID *string` | `nodeID *string` |
| `ports.DocumentStore.List/ListPage/ListPageByTypes/Search/SearchByTypes/SemanticSearch*` param `projectID *string` | `nodeID *string` |
| `pgstore.ProjectStore`/`NewProjectStore` | `pgstore.NodeStore`/`NewNodeStore` |
| `pgstore.scanProject` | `scanNode` |
| `usecase.ResolveProject` | `ResolveNode` (field `Projects`→`Nodes ports.NodeStore`) |
| `usecase.BindProject` | `BindNode` (field `Projects`→`Nodes`) |
| `usecase.UnbindProject` | `UnbindNode` |
| `usecase.ListProjectBindings` | `ListNodeBindings` |
| `usecase.CreateProject` | `CreateNode` |
| `usecase.GetProject` | `GetNode` |
| `usecase.ListProjects` | `ListNodes` |
| `usecase.UpdateProject`/`UpdateProjectInput` | `UpdateNode`/`UpdateNodeInput` |
| `usecase.DeleteProject` | `DeleteNode` |
| `usecase.SetProjectRate` | `SetNodeRate` |
| `usecase.BulkAssignProject` | `BulkAssignNode` |
| usecase struct field `Projects ports.ProjectStore` (StopSession, etc.) | `Nodes ports.NodeStore` |
| `testutil.FakeProjectStore`/`NewFakeProjectStore` | `FakeNodeStore`/`NewFakeNodeStore` |
| apiclient `CreateProject/ListProjects/GetProject/UpdateProject/DeleteProject/UpdateProjectFields/ResolveProject` | `CreateNode/ListNodes/GetNode/UpdateNode/DeleteNode/UpdateNodeFields/ResolveNode` |
| httpserver `Server` fields + handlers `*Project*` | `*Node*` (e.g. `handleCreateProject`→`handleCreateNode`, `CreateProject`→`CreateNode`) |
| REST `/api/v1/projects*`, webui `/projects*` | `/api/v1/nodes*`, `/nodes*` |
| CLI `flow project` (cobra `Use: "project"`) | `flow node` (`Use: "node"`); `projectCmd()`→`nodeCmd()` etc. |
| `internal/projectresolve` return type `domain.Project` | `domain.Node` (keep package name `projectresolve`; `Resolve` returns `(domain.Node, bool, error)`, calls `c.ResolveNode`) |

**PRESERVED (do NOT rename):** `domain.ProjectBinding` (only field renamed), `domain.BindKey`, `ports.ProjectBindingStore`, `pgstore.ProjectBindingStore`/`NewProjectBindingStore`, `ProjectBindingStore.ListByProject`, `testutil.FakeProjectBindingStore`, `domain.BindingRemote/BindingPath`, `domain.ResolveBinding`. The binding *concept* keeps its name; only its `NodeID` field and the bind/unbind/list *use-cases* rename. (Confirmed by contract line 98: `ResolveNode struct { Bindings ports.ProjectBindingStore; Nodes ports.NodeStore }`.)

Steps:

- [ ] **Step 1: Write `0015_rename_projects_to_nodes.sql`** (rename only — table, three columns, doc unique index):
```sql
-- +goose Up
ALTER TABLE projects RENAME TO nodes;
ALTER TABLE documents RENAME COLUMN project_id TO node_id;
ALTER TABLE work_sessions RENAME COLUMN project_id TO node_id;
ALTER TABLE project_bindings RENAME COLUMN project_id TO node_id;
DROP INDEX documents_owner_project_path;
CREATE UNIQUE INDEX documents_owner_node_path
    ON documents (owner_id, coalesce(node_id, ''), path);

-- +goose Down
ALTER TABLE project_bindings RENAME COLUMN node_id TO project_id;
ALTER TABLE work_sessions RENAME COLUMN node_id TO project_id;
ALTER TABLE documents RENAME COLUMN node_id TO project_id;
DROP INDEX documents_owner_node_path;
CREATE UNIQUE INDEX documents_owner_project_path
    ON documents (owner_id, coalesce(project_id, ''), path);
ALTER TABLE nodes RENAME TO projects;
```
(FK + partial-unique-index names auto-follow the table/column rename; the `documents` unique index is the only one whose *definition* references the renamed column directly, so it is dropped + recreated under the new name.)

- [ ] **Step 2: Rename the domain layer.** Apply the map to `internal/domain/{node.go,nodestyle.go,document.go,worksession.go,projectbinding.go,export.go,event.go}` and their `_test.go`. In `node.go` the struct/consts/`NewNode`/`Validate`/`Validate`'s `ValidNodeColor/Glyph` references; in `event.go` the three `EventNode*` consts and string values `"node.created"/"node.updated"/"node.deleted"`; in `export.go` `NodeTotal`/`ByEngagement`/`NodeName` (and JSON keys may stay `byProject`/`project` if you want zero wire change — but per contract rename the Go field to `ByEngagement`; the JSON tags are cosmetic, keep them stable to avoid client breakage in this slice).

- [ ] **Step 3: Rename `ports.go`.** `ProjectStore`→`NodeStore`, `ErrProjectNotFound`→`ErrNodeNotFound`, and the `Stop/Update` (SessionStore) + all `DocumentStore` `projectID *string`→`nodeID *string` params + doc comments ("project"→"node"). Leave `ProjectBindingStore` untouched.

- [ ] **Step 4: Rename `pgstore`** (`nodes.go`, `sessions.go`, `documents.go`, `documents_embed.go`, `projectbindings.go`): table `projects`→`nodes`, columns `project_id`→`node_id` in every SQL string, `scanProject`→`scanNode`, `NewProjectStore`→`NewNodeStore`, `ports.ErrProjectNotFound`→`ports.ErrNodeNotFound`, struct field types. `projectbindings.go`: only the SQL column `project_id`→`node_id` and `b.ProjectID`→`b.NodeID` (keep the store/type names).

- [ ] **Step 5: Rename usecases + struct fields + main.go wiring.** Apply the file renames + symbol map. Update `cmd/flow-server/main.go`: `projectStore := pgstore.NewNodeStore(pool)` → name var `nodeStore`; every `Projects: projectStore` → `Nodes: nodeStore`; usecase constructors `usecase.CreateNode{Nodes: nodeStore, …}` etc.; `ResolveNode/BindNode/UnbindNode/ListNodeBindings/SetNodeRate/DeleteNode/GetNode/ListNodes/UpdateNode/BulkAssignNode`.

- [ ] **Step 6: Rename adapters + cmd.** `internal/adapter/httpserver/*` (Server fields, handlers, route literals `/api/v1/nodes*` + webui `/nodes*`, `EventNode*` publishes); `internal/adapter/apiclient/*` (methods + paths); `internal/adapter/webui/*` incl. `.templ` files (`projects.templ`, nav, etc.) — then run **`templ generate`** and commit the regenerated `_templ.go`; `internal/tui/**` (screen/projects, shell/home, docs, worktime) `domain.EventNode*` + `.NodeID`; `cmd/flow/node.go` (`Use: "node"`, `nodeCmd()`), `cmd/flow/ui.go`, `cmd/flow-mcp/*` (keep MCP tool *names* `flow_*_project*` unchanged per spec §7, but their Go calls to `domain.Node`/`.NodeID`/`ResolveNode` rename).

- [ ] **Step 7: Rename `testutil/fakes.go`.** `FakeProjectStore`→`FakeNodeStore`, `NewFakeProjectStore`→`NewFakeNodeStore`, all `domain.Project`→`domain.Node`, `ports.ErrProjectNotFound`→`ports.ErrNodeNotFound`, session/document fakes' `projectID`→`nodeID` params + `.ProjectID`→`.NodeID` (incl. `matchesProject`→keep helper name or rename `matchesNode`; `docCollisionKey(... projectID *string ...)`→`nodeID`). Keep `FakeProjectBindingStore` name; rename its `b.ProjectID`→`b.NodeID`, `ListByProject` stays.

- [ ] **Step 8: Run the compile gate (expect FAIL until complete, then PASS).** `go build ./... && go vet ./...`; then `make ci`. Iterate until green. There are no *new* tests — the existing suite is the regression net; behavior is unchanged.

- [ ] **Step 9: Verification greps (must all be empty over non-docs Go/templ).**
```
rg "domain\.Project\b|domain\.ProjectStatus|domain\.NewProject|domain\.ValidProjectColor|domain\.ValidProjectGlyph|domain\.ErrInvalidProject" -g '!docs/**'
rg "\bProjectStore\b" -g '!docs/**'            # ProjectBindingStore is a different token; must not appear
rg "ErrProjectNotFound|EventProject" -g '!docs/**'
rg "\.ProjectID\b|ProjectTotal\b|\.ProjectName\b|\.ByProject\b" -g '!docs/**'
rg "/api/v1/projects|\"/projects" -g '!docs/**' -g '*.go'
rg "usecase\.(Create|Get|List|Update|Delete|Resolve|Bind|Unbind|SetProjectRate|BulkAssignProject)Project" -g '!docs/**'
```
Then confirm `make ci` is green (coverage gate held).

- [ ] **Step 10: Commit.** `refactor(domain): rename Project→Node across repo (flat, no behavior change) + 0015 rename migration`

---

### Task A2: `Node` hierarchy fields + pure domain validators (TDD)

Add the hierarchy fields to `domain.Node` and the three pure helpers from the contract. No DB, no I/O.

**Files:**
- modify `internal/domain/node.go`
- test `internal/domain/node_test.go` (new; `package domain_test`)

**Interfaces (Consumes/Produces — exact):**
- Produces `const ( KindEngagement, KindVorhaben, KindRepo, KindBranch NodeKind )`
- Produces `Node` fields `ParentID *string`, `Kind NodeKind`, `OriginSlug string`, `Extra map[string]any`
- Produces `func ValidParentKind(child, parent NodeKind) bool`
- Produces `func AllowedChildKind(parent, child NodeKind) bool` ( `= ValidParentKind(child, parent)` )
- Produces `func ResolveEngagement(chain []Node) (Node, bool)`

Steps:

- [ ] **Step 1: Write failing test `node_test.go`.**
```go
package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestValidParentKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		child, parent domain.NodeKind
		want         bool
	}{
		{"engagement never has a parent", domain.KindEngagement, domain.KindEngagement, false},
		{"engagement not under repo", domain.KindEngagement, domain.KindRepo, false},
		{"vorhaben under engagement", domain.KindVorhaben, domain.KindEngagement, true},
		{"vorhaben under vorhaben", domain.KindVorhaben, domain.KindVorhaben, true},
		{"vorhaben not under repo", domain.KindVorhaben, domain.KindRepo, false},
		{"repo under engagement", domain.KindRepo, domain.KindEngagement, true},
		{"repo under vorhaben", domain.KindRepo, domain.KindVorhaben, true},
		{"repo not under repo", domain.KindRepo, domain.KindRepo, false},
		{"branch under repo", domain.KindBranch, domain.KindRepo, true},
		{"branch not under engagement", domain.KindBranch, domain.KindEngagement, false},
		{"branch not under vorhaben", domain.KindBranch, domain.KindVorhaben, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ValidParentKind(c.child, c.parent); got != c.want {
				t.Errorf("ValidParentKind(%s,%s)=%v want %v", c.child, c.parent, got, c.want)
			}
			if got := domain.AllowedChildKind(c.parent, c.child); got != c.want {
				t.Errorf("AllowedChildKind(%s,%s)=%v want %v", c.parent, c.child, got, c.want)
			}
		})
	}
}

func TestResolveEngagement(t *testing.T) {
	t.Parallel()
	p := "p"
	chain := []domain.Node{
		{ID: "repo", Kind: domain.KindRepo, ParentID: &p},
		{ID: "p", Kind: domain.KindEngagement, Name: "Privat"},
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok || eng.ID != "p" {
		t.Fatalf("want engagement p, got %+v ok=%v", eng, ok)
	}
	if _, ok := domain.ResolveEngagement(nil); ok {
		t.Error("empty chain must be ok=false")
	}
	noEng := []domain.Node{{ID: "repo", Kind: domain.KindRepo}}
	if _, ok := domain.ResolveEngagement(noEng); ok {
		t.Error("chain whose root is not an engagement must be ok=false")
	}
}
```

- [ ] **Step 2: Run (expect FAIL — symbols undefined).** `go test ./internal/domain/`

- [ ] **Step 3: Implement.** Add to `node.go`:
```go
// NodeKind is the level of a node in the engagement→vorhaben→repo→branch tree.
type NodeKind string

const (
	KindEngagement NodeKind = "engagement"
	KindVorhaben   NodeKind = "vorhaben"
	KindRepo       NodeKind = "repo"
	KindBranch     NodeKind = "branch" // B1: reserved only; no behavior
)
```
Extend the `Node` struct (add the four fields; keep JSON tags from the contract):
```go
	ParentID    *string        `json:"parentId,omitempty"`
	Kind        NodeKind       `json:"kind"`
	OriginSlug  string         `json:"originSlug,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
```
Add the helpers:
```go
// ValidParentKind reports whether a node of childKind may hang under parentKind.
// Root placement (nil parent) is handled by the caller (root must be engagement).
func ValidParentKind(child, parent NodeKind) bool {
	switch child {
	case KindVorhaben, KindRepo:
		return parent == KindEngagement || parent == KindVorhaben
	case KindBranch:
		return parent == KindRepo
	default: // engagement (or unknown) may never have a parent
		return false
	}
}

// AllowedChildKind reports whether parentKind may have a child of childKind.
func AllowedChildKind(parent, child NodeKind) bool { return ValidParentKind(child, parent) }

// ResolveEngagement returns the engagement from an ancestor chain ordered
// leaf→root (as NodeStore.Ancestors returns). The engagement is the last
// element (root); ok=false if the chain is empty or its root is not an engagement.
func ResolveEngagement(chain []Node) (Node, bool) {
	if len(chain) == 0 {
		return Node{}, false
	}
	root := chain[len(chain)-1]
	if root.Kind != KindEngagement {
		return Node{}, false
	}
	return root, true
}
```

- [ ] **Step 4: Run (expect PASS).** `go test ./internal/domain/`

- [ ] **Step 5: Commit.** `feat(domain): add Node hierarchy fields + ValidParentKind/AllowedChildKind/ResolveEngagement`

---

### Task A3: pgstore hierarchy columns migration + `Create/List/Get/Update/scanNode` (TDD)

Add the `0016` columns migration and rewrite `nodes.go`'s store methods to read/write `parent_id`/`kind`/`origin_slug`/`extra`. Update `FakeNodeStore.Update` to mirror the new mutable set.

**Files:**
- create `internal/adapter/pgstore/migrations/0016_nodes_hierarchy_columns.sql`
- modify `internal/adapter/pgstore/nodes.go`
- modify `internal/testutil/fakes.go` (`FakeNodeStore.Update`)
- test `internal/adapter/pgstore/nodes_test.go` (add a round-trip test; Docker)

**Interfaces (unchanged signatures from A1 `ports.NodeStore`):** `Create/List/Get/Update/SetRate/Delete`. This task only widens the persisted column set; the FK `parent_id … ON DELETE RESTRICT` and indexes are added here.

Steps:

- [ ] **Step 1: Write `0016_nodes_hierarchy_columns.sql`** (no CHECKs yet; `kind` keeps temp default so the data-fixup can label pre-existing rows as repos):
```sql
-- +goose Up
ALTER TABLE nodes ADD COLUMN parent_id   TEXT REFERENCES nodes(id) ON DELETE RESTRICT;
ALTER TABLE nodes ADD COLUMN kind        TEXT NOT NULL DEFAULT 'repo';
ALTER TABLE nodes ADD COLUMN origin_slug TEXT;
ALTER TABLE nodes ADD COLUMN extra       JSONB NOT NULL DEFAULT '{}';
CREATE INDEX nodes_owner  ON nodes (owner_id);
CREATE INDEX nodes_parent ON nodes (parent_id);

-- +goose Down
DROP INDEX IF EXISTS nodes_parent;
DROP INDEX IF EXISTS nodes_owner;
ALTER TABLE nodes DROP COLUMN extra;
ALTER TABLE nodes DROP COLUMN origin_slug;
ALTER TABLE nodes DROP COLUMN kind;
ALTER TABLE nodes DROP COLUMN parent_id;
```

- [ ] **Step 2: Write failing round-trip test in `nodes_test.go`** (Docker; full Migrate, then create an engagement + a repo child, assert the new fields survive):
```go
func TestNodeStore_HierarchyRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-h", "sub-h", "huser", "h@x.de", "H User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	eng, _ := domain.NewNode("eng", "u-h", "Privat", "privat", now)
	eng.Kind = domain.KindEngagement
	eng.Extra = map[string]any{"legacy_rate": map[string]any{"amount": float64(9000), "currency": "EUR"}}
	if _, err := st.Create(ctx, eng); err != nil {
		t.Fatalf("create engagement: %v", err)
	}

	repo, _ := domain.NewNode("repo", "u-h", "flow", "flow", now)
	repo.Kind = domain.KindRepo
	repo.ParentID = strptr("eng")
	repo.OriginSlug = "github.com/serverkraken/flow"
	got, err := st.Create(ctx, repo)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if got.Kind != domain.KindRepo || got.ParentID == nil || *got.ParentID != "eng" || got.OriginSlug != "github.com/serverkraken/flow" {
		t.Fatalf("create returned %+v", got)
	}

	re, err := st.Get(ctx, "u-h", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if re.Kind != domain.KindRepo || re.ParentID == nil || *re.ParentID != "eng" {
		t.Errorf("get repo: %+v", re)
	}
	reEng, _ := st.Get(ctx, "u-h", "eng")
	if reEng.Extra["legacy_rate"] == nil {
		t.Errorf("engagement extra lost: %+v", reEng.Extra)
	}

	// Update persists origin_slug + extra, leaves parent_id + rate untouched.
	upd := re
	upd.OriginSlug = "github.com/serverkraken/flow2"
	upd.Extra = map[string]any{"note": "x"}
	upd.UpdatedAt = now.Add(time.Hour)
	if _, err := st.Update(ctx, "u-h", upd); err != nil {
		t.Fatalf("update: %v", err)
	}
	re2, _ := st.Get(ctx, "u-h", "repo")
	if re2.OriginSlug != "github.com/serverkraken/flow2" || re2.Extra["note"] != "x" {
		t.Errorf("update did not persist origin/extra: %+v", re2)
	}
	if re2.ParentID == nil || *re2.ParentID != "eng" {
		t.Errorf("update must not touch parent_id: %+v", re2.ParentID)
	}
}

func strptr(s string) *string { return &s }
```
(If a `strptr` helper already exists in `pgstore_test`, reuse it and drop the local one.)

- [ ] **Step 3: Run (expect FAIL — `Create` doesn't yet write the new columns; scan mismatch).** `go test ./internal/adapter/pgstore/ -run HierarchyRoundTrip`

- [ ] **Step 4: Implement — rewrite `nodes.go` store methods.** Replace the `Create/List/Get/Update/scanNode` bodies (keep `rateCols` + `rowScanner`):
```go
const nodeCols = `id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at`

func (s *NodeStore) Create(ctx context.Context, n domain.Node) (domain.Node, error) {
	const q = `
INSERT INTO nodes (` + nodeCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
RETURNING ` + nodeCols
	ra, rc := rateCols(n.Rate)
	os := nullStr(n.OriginSlug)
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	return scanNode(s.pool.QueryRow(ctx, q,
		n.ID, n.OwnerID, n.ParentID, string(n.Kind), n.Name, n.Slug, n.Color, n.Glyph,
		n.Description, n.UpstreamGit, os, string(n.Status), ra, rc, ex, n.CreatedAt, n.UpdatedAt))
}

func (s *NodeStore) List(ctx context.Context, ownerID string) ([]domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list nodes: %w", err)
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *NodeStore) Get(ctx context.Context, ownerID, id string) (domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 AND id=$2`
	n, err := scanNode(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return n, err
}

// Update overwrites mutable metadata (name, slug, color, glyph, description,
// upstream_git, origin_slug, status, extra). It does NOT touch rate or parent_id.
func (s *NodeStore) Update(ctx context.Context, ownerID string, n domain.Node) (domain.Node, error) {
	const q = `
UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5,
                 upstream_git=$6, origin_slug=$7, status=$8, extra=$9, updated_at=$10
WHERE owner_id=$11 AND id=$12
RETURNING ` + nodeCols
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		n.Name, n.Slug, n.Color, n.Glyph, n.Description, n.UpstreamGit, nullStr(n.OriginSlug),
		string(n.Status), ex, n.UpdatedAt, ownerID, n.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return got, err
}

// nullStr maps "" → SQL NULL so partial CHECKs (origin_slug only on repo) hold.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanNode(r rowScanner) (domain.Node, error) {
	var n domain.Node
	var kind, status string
	var parentID, originSlug *string
	var ra *int64
	var rc *string
	var extra map[string]any
	if err := r.Scan(
		&n.ID, &n.OwnerID, &parentID, &kind, &n.Name, &n.Slug, &n.Color, &n.Glyph,
		&n.Description, &n.UpstreamGit, &originSlug, &status, &ra, &rc, &extra,
		&n.CreatedAt, &n.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Node{}, err
		}
		return domain.Node{}, fmt.Errorf("pgstore: scan node: %w", err)
	}
	n.ParentID = parentID
	n.Kind = domain.NodeKind(kind)
	if originSlug != nil {
		n.OriginSlug = *originSlug
	}
	n.Status = domain.NodeStatus(status)
	if (ra == nil) != (rc == nil) {
		return domain.Node{}, fmt.Errorf("pgstore: scan node: inconsistent rate columns (amount set=%v currency set=%v)", ra != nil, rc != nil)
	}
	if ra != nil && rc != nil {
		n.Rate = &domain.Money{Amount: *ra, Currency: *rc}
	}
	n.Extra = extra
	return n, nil
}
```
Keep `SetRate` and `Delete` as renamed in A1 (A4 enhances `Delete`).

- [ ] **Step 5: Update `FakeNodeStore.Update`** to mirror the new mutable set (it already stores `Create` wholesale, so `Kind/ParentID/OriginSlug` round-trip on create; `Update` must now also copy `OriginSlug` + `Extra`, and must NOT touch `ParentID`/`Rate`):
```go
	existing.Description = p.Description
	existing.UpstreamGit = p.UpstreamGit
	existing.OriginSlug = p.OriginSlug
	existing.Status = p.Status
	existing.Extra = p.Extra
	existing.UpdatedAt = p.UpdatedAt
```

- [ ] **Step 6: Run (expect PASS).** `go test ./internal/adapter/pgstore/ -run HierarchyRoundTrip` then `make ci`.

- [ ] **Step 7: Commit.** `feat(pgstore): node hierarchy columns (0016) + Create/List/Get/Update/scanNode read parent_id/kind/origin_slug/extra`

---

### Task A4: `ports.NodeStore` additions + pgstore `Children`/`Ancestors`/`Reparent` + RESTRICT `Delete` (TDD)

Grow `ports.NodeStore` to its final contract shape and implement the recursive walk + RESTRICT delete in pgstore and the fakes.

**Files:**
- modify `internal/ports/ports.go` (`NodeStore` additions + `ErrNodeHasChildren`)
- modify `internal/adapter/pgstore/nodes.go` (`Children`, `Ancestors`, `Reparent`; rework `Delete`)
- modify `internal/testutil/fakes.go` (`FakeNodeStore`: add the three methods; rework `Delete`)
- test `internal/adapter/pgstore/nodes_test.go` (Docker)

**Interfaces (Produces — exact from contract):**
- `Children(ctx, ownerID string, parentID *string) ([]domain.Node, error)` (nil = roots, name-ordered)
- `Ancestors(ctx, ownerID, nodeID string) ([]domain.Node, error)` (leaf→root)
- `Reparent(ctx, ownerID, id string, parentID *string) (domain.Node, error)`
- `Delete(...)` → `ports.ErrNodeHasChildren` on FK RESTRICT
- `var ErrNodeHasChildren = errors.New("node has children")`

Steps:

- [ ] **Step 1: Write failing Docker test `TestNodeStore_TreeWalk`.**
```go
func TestNodeStore_TreeWalk(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-t", "sub-t", "tuser", "t@x.de", "T")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	mk := func(id, name, slug string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "u-t", name, slug, now)
		n.Kind = kind
		n.ParentID = parent
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("eng", "Privat", "privat", domain.KindEngagement, nil)
	mk("vor", "Sub", "sub", domain.KindVorhaben, strptr("eng"))
	mk("repo", "flow", "flow", domain.KindRepo, strptr("vor"))

	// Children of root (nil) = the engagement.
	roots, err := st.Children(ctx, "u-t", nil)
	if err != nil || len(roots) != 1 || roots[0].ID != "eng" {
		t.Fatalf("children(nil)=%v err=%v", roots, err)
	}
	// Children of eng = vor.
	kids, _ := st.Children(ctx, "u-t", strptr("eng"))
	if len(kids) != 1 || kids[0].ID != "vor" {
		t.Fatalf("children(eng)=%v", kids)
	}
	// Ancestors of repo, leaf→root: repo, vor, eng.
	chain, err := st.Ancestors(ctx, "u-t", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 || chain[0].ID != "repo" || chain[1].ID != "vor" || chain[2].ID != "eng" {
		t.Fatalf("ancestors=%v", chain)
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok || eng.ID != "eng" {
		t.Fatalf("resolveEngagement=%v ok=%v", eng, ok)
	}
	// Reparent repo onto eng directly.
	if _, err := st.Reparent(ctx, "u-t", "repo", strptr("eng")); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	chain2, _ := st.Ancestors(ctx, "u-t", "repo")
	if len(chain2) != 2 || chain2[1].ID != "eng" {
		t.Fatalf("after reparent ancestors=%v", chain2)
	}
	// Delete with children → ErrNodeHasChildren; leaf delete → ok.
	if err := st.Delete(ctx, "u-t", "eng"); !errors.Is(err, ports.ErrNodeHasChildren) {
		t.Fatalf("delete eng with children: want ErrNodeHasChildren, got %v", err)
	}
	if err := st.Delete(ctx, "u-t", "repo"); err != nil {
		t.Fatalf("delete leaf repo: %v", err)
	}
	if err := st.Delete(ctx, "u-t", "missing"); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("delete missing: want ErrNodeNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run (expect FAIL — methods undefined / `Delete` returns wrong error).** `go test ./internal/adapter/pgstore/ -run TreeWalk`

- [ ] **Step 3: Implement ports.** In `ports.go` add to `NodeStore` (full final interface = the contract block at lines 71–90) the methods `SetRate`, `Reparent`, `Children`, `Ancestors` and the RESTRICT semantics doc on `Delete`, and add `var ErrNodeHasChildren = errors.New("node has children")` to the sentinel block.

- [ ] **Step 4: Implement pgstore.** Add to `nodes.go` (import `github.com/jackc/pgx/v5/pgconn`):
```go
func (s *NodeStore) Children(ctx context.Context, ownerID string, parentID *string) ([]domain.Node, error) {
	var rows pgx.Rows
	var err error
	if parentID == nil {
		rows, err = s.pool.Query(ctx, `SELECT `+nodeCols+` FROM nodes WHERE owner_id=$1 AND parent_id IS NULL ORDER BY name`, ownerID)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT `+nodeCols+` FROM nodes WHERE owner_id=$1 AND parent_id=$2 ORDER BY name`, ownerID, *parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: children: %w", err)
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Ancestors returns the node and its ancestors ordered leaf→root.
func (s *NodeStore) Ancestors(ctx context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	const q = `
WITH RECURSIVE chain AS (
  SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, 0 AS depth
  FROM nodes WHERE owner_id=$1 AND id=$2
  UNION ALL
  SELECT n.id, n.owner_id, n.parent_id, n.kind, n.name, n.slug, n.color, n.glyph, n.description, n.upstream_git, n.origin_slug, n.status, n.rate_amount, n.rate_currency, n.extra, n.created_at, n.updated_at, c.depth+1
  FROM nodes n JOIN chain c ON n.id = c.parent_id
  WHERE n.owner_id=$1
)
SELECT id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at
FROM chain ORDER BY depth`
	rows, err := s.pool.Query(ctx, q, ownerID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: ancestors: %w", err)
	}
	defer rows.Close()
	var out []domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *NodeStore) Reparent(ctx context.Context, ownerID, id string, parentID *string) (domain.Node, error) {
	const q = `UPDATE nodes SET parent_id=$1, updated_at=now() WHERE owner_id=$2 AND id=$3 RETURNING ` + nodeCols
	n, err := scanNode(s.pool.QueryRow(ctx, q, parentID, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return n, err
}
```
Rework `Delete` to map FK RESTRICT (`23503`) → `ErrNodeHasChildren`:
```go
func (s *NodeStore) Delete(ctx context.Context, ownerID, id string) error {
	const q = `DELETE FROM nodes WHERE owner_id=$1 AND id=$2`
	tag, err := s.pool.Exec(ctx, q, ownerID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ports.ErrNodeHasChildren
		}
		return fmt.Errorf("pgstore: delete node: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNodeNotFound
	}
	return nil
}
```

- [ ] **Step 5: Implement fakes.** Add to `FakeNodeStore` in `fakes.go` (`sort` already imported):
```go
func (s *FakeNodeStore) Children(_ context.Context, ownerID string, parentID *string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Node
	for _, n := range s.m {
		if n.OwnerID != ownerID {
			continue
		}
		if parentID == nil {
			if n.ParentID == nil {
				out = append(out, n)
			}
		} else if n.ParentID != nil && *n.ParentID == *parentID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *FakeNodeStore) Ancestors(_ context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Node
	cur := nodeID
	for {
		n, ok := s.m[cur]
		if !ok || n.OwnerID != ownerID {
			break
		}
		out = append(out, n)
		if n.ParentID == nil {
			break
		}
		cur = *n.ParentID
	}
	return out, nil
}

func (s *FakeNodeStore) Reparent(_ context.Context, ownerID, id string, parentID *string) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.m[id]
	if !ok || n.OwnerID != ownerID {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	n.ParentID = parentID
	s.m[id] = n
	return n, nil
}
```
Rework `FakeNodeStore.Delete` to reject parents:
```go
func (s *FakeNodeStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.m[id]
	if !ok || n.OwnerID != ownerID {
		return ports.ErrNodeNotFound
	}
	for _, other := range s.m {
		if other.ParentID != nil && *other.ParentID == id {
			return ports.ErrNodeHasChildren
		}
	}
	delete(s.m, id)
	return nil
}
```

- [ ] **Step 6: Run (expect PASS).** `go test ./internal/adapter/pgstore/ -run TreeWalk` then `make ci`.

- [ ] **Step 7: Commit.** `feat(ports,pgstore): NodeStore Children/Ancestors(WITH RECURSIVE)/Reparent + RESTRICT Delete→ErrNodeHasChildren`

---

### Task A5: usecases — `ResolveEngagement` + `MoveNode` + `CreateNode` kind/parent validation + `BindNode` target-kind (TDD)

Add the new/extended use cases. `ResolveNode`, `BindNode`, `UnbindNode`, `ListNodeBindings` already exist (renamed in A1); this task adds `ResolveEngagement`/`MoveNode`, rewrites `CreateNode` to validate kind+parent, and adds binding-target validation to `BindNode`. It also threads the new `CreateNode` signature through its direct callers so `make ci` stays green (the full `--kind/--parent` UI is Slice C).

**Files:**
- create `internal/usecase/resolve_engagement.go` + `resolve_engagement_test.go`
- create `internal/usecase/move_node.go` + `move_node_test.go`
- modify `internal/usecase/create_node.go` + `create_node_test.go`
- modify `internal/usecase/bind_node.go` + `bind_node_test.go` (or existing `project_bindings_test.go`)
- modify callers: `internal/adapter/httpserver/worktime.go` (`handleCreateNode`), `cmd/flow-server/main.go` if constructor fields change, plus any other `CreateNode.Execute(` caller surfaced by grep
- modify `cmd/flow-server/main.go` wiring: add `ResolveEngagement`/`MoveNode` constructors and `Nodes` field to `BindNode`

**Interfaces (Produces — exact):**
- `type ResolveEngagement struct { Resolve ResolveNode; Nodes ports.NodeStore }` ; `Execute(ctx, ownerID, remoteSlug, machineID, cwd string) (domain.Node, bool, error)`
- `type MoveNode struct { Nodes ports.NodeStore }` ; `Execute(ctx, ownerID, id string, newParentID *string) (domain.Node, error)` ; `var ErrNodeCycle = errors.New("usecase: move would create a cycle")`
- `type CreateNodeInput struct { Name, Slug string; Kind domain.NodeKind; ParentID *string; Color, Glyph, Description, UpstreamGit string }` ; `CreateNode.Execute(ctx, ownerID string, in CreateNodeInput) (domain.Node, error)`
- `BindNode.Execute(ctx, ownerID, nodeID string, k BindKey) (domain.ProjectBinding, error)` with `Nodes ports.NodeStore`; `var ErrInvalidBindTarget = errors.New("usecase: invalid binding target kind")`

Steps:

- [ ] **Step 1: Write failing tests.**

`resolve_engagement_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestResolveEngagement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	now := time.Now()
	eng, _ := domain.NewNode("eng", "o", "Privat", "privat", now)
	eng.Kind = domain.KindEngagement
	_, _ = nodes.Create(ctx, eng)
	repo, _ := domain.NewNode("repo", "o", "flow", "flow", now)
	repo.Kind = domain.KindRepo
	repo.ParentID = sp("eng")
	_, _ = nodes.Create(ctx, repo)
	_, _ = binds.Upsert(ctx, domain.ProjectBinding{ID: "b", OwnerID: "o", NodeID: "repo", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow", CreatedAt: now, UpdatedAt: now})

	uc := usecase.ResolveEngagement{
		Resolve: usecase.ResolveNode{Bindings: binds, Nodes: nodes},
		Nodes:   nodes,
	}
	got, ok, err := uc.Execute(ctx, "o", "github.com/serverkraken/flow", "m", "/x")
	if err != nil || !ok || got.ID != "eng" {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	// Unresolved context → ok=false, no error.
	if _, ok, err := uc.Execute(ctx, "o", "github.com/none/none", "m", "/x"); ok || err != nil {
		t.Fatalf("unresolved: ok=%v err=%v", ok, err)
	}
}

func sp(s string) *string { return &s }
```

`move_node_test.go`:
```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestMoveNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	now := time.Now()
	mk := func(id string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "o", id, id, now)
		n.Kind = kind
		n.ParentID = parent
		_, _ = nodes.Create(ctx, n)
	}
	mk("eng", domain.KindEngagement, nil)
	mk("eng2", domain.KindEngagement, nil)
	mk("vor", domain.KindVorhaben, sp("eng"))
	mk("repo", domain.KindRepo, sp("vor"))

	uc := usecase.MoveNode{Nodes: nodes}

	// valid: repo → eng2
	if got, err := uc.Execute(ctx, "o", "repo", sp("eng2")); err != nil || got.ParentID == nil || *got.ParentID != "eng2" {
		t.Fatalf("valid move: %+v err=%v", got, err)
	}
	// kind violation: vor under repo
	if _, err := uc.Execute(ctx, "o", "vor", sp("repo")); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("kind violation: want ErrInvalidNode, got %v", err)
	}
	// cycle: eng under its own descendant vor
	if _, err := uc.Execute(ctx, "o", "eng", sp("vor")); !errors.Is(err, usecase.ErrNodeCycle) {
		t.Fatalf("cycle: want ErrNodeCycle, got %v", err)
	}
	// self-parent is a cycle
	if _, err := uc.Execute(ctx, "o", "vor", sp("vor")); !errors.Is(err, usecase.ErrNodeCycle) {
		t.Fatalf("self-parent: want ErrNodeCycle, got %v", err)
	}
	// move repo to root → only engagements may be roots
	if _, err := uc.Execute(ctx, "o", "repo", nil); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("repo to root: want ErrInvalidNode, got %v", err)
	}
	// engagement to root is fine
	if _, err := uc.Execute(ctx, "o", "eng", nil); err != nil {
		t.Fatalf("eng to root: %v", err)
	}
}
```

`create_node_test.go`:
```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	uc := usecase.CreateNode{Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}

	// root must be engagement
	eng, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "Privat", Kind: domain.KindEngagement})
	if err != nil || eng.Kind != domain.KindEngagement || eng.Slug != "privat" {
		t.Fatalf("engagement: %+v err=%v", eng, err)
	}
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "X", Kind: domain.KindRepo}); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("rootless repo: want ErrInvalidNode, got %v", err)
	}
	// repo under engagement ok
	repo, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "flow", Kind: domain.KindRepo, ParentID: &eng.ID})
	if err != nil || repo.ParentID == nil || *repo.ParentID != eng.ID {
		t.Fatalf("repo: %+v err=%v", repo, err)
	}
	// repo under repo rejected
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "b", Kind: domain.KindRepo, ParentID: &repo.ID}); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("repo under repo: want ErrInvalidNode, got %v", err)
	}
	// unknown parent → ErrNodeNotFound
	bad := "nope"
	if _, err := uc.Execute(ctx, "o", usecase.CreateNodeInput{Name: "x", Kind: domain.KindRepo, ParentID: &bad}); err == nil {
		t.Fatal("unknown parent must error")
	}
}
```

`bind_node_test.go`:
```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBindNode_TargetKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	binds := testutil.NewFakeProjectBindingStore()
	now := time.Now()
	mk := func(id string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "o", id, id, now)
		n.Kind = kind
		n.ParentID = parent
		_, _ = nodes.Create(ctx, n)
	}
	mk("eng", domain.KindEngagement, nil)
	mk("repo", domain.KindRepo, sp("eng"))
	mk("leafvor", domain.KindVorhaben, sp("eng"))
	mk("parentvor", domain.KindVorhaben, sp("eng"))
	mk("child", domain.KindRepo, sp("parentvor"))

	uc := usecase.BindNode{Bindings: binds, Nodes: nodes, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
	remote := usecase.BindKey{Kind: domain.BindingRemote, RemoteSlug: "github.com/o/r"}
	path := usecase.BindKey{Kind: domain.BindingPath, MachineID: "m", Path: "/p"}

	if _, err := uc.Execute(ctx, "o", "repo", remote); err != nil {
		t.Fatalf("remote→repo ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "eng", remote); !errors.Is(err, usecase.ErrInvalidBindTarget) {
		t.Fatalf("remote→engagement: want ErrInvalidBindTarget, got %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "repo", path); err != nil {
		t.Fatalf("path→repo ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "leafvor", path); err != nil {
		t.Fatalf("path→leaf vorhaben ok: %v", err)
	}
	if _, err := uc.Execute(ctx, "o", "parentvor", path); !errors.Is(err, usecase.ErrInvalidBindTarget) {
		t.Fatalf("path→non-leaf vorhaben: want ErrInvalidBindTarget, got %v", err)
	}
}
```

- [ ] **Step 2: Run (expect FAIL — undefined symbols / old `CreateNode` signature).** `go test ./internal/usecase/`

- [ ] **Step 3: Implement `resolve_engagement.go`.**
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ResolveEngagement resolves the cwd/remote context to a node, walks its ancestor
// chain, and returns the engagement at its root. Worktime books against this.
type ResolveEngagement struct {
	Resolve ResolveNode
	Nodes   ports.NodeStore
}

func (uc ResolveEngagement) Execute(ctx context.Context, ownerID, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	node, ok, err := uc.Resolve.Execute(ctx, ownerID, remoteSlug, machineID, cwd)
	if err != nil || !ok {
		return domain.Node{}, ok, err
	}
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, node.ID)
	if err != nil {
		return domain.Node{}, false, err
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok {
		return domain.Node{}, false, nil
	}
	return eng, true, nil
}
```

- [ ] **Step 4: Implement `move_node.go`.**
```go
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ErrNodeCycle is returned when a move would make a node its own ancestor.
var ErrNodeCycle = errors.New("usecase: move would create a cycle")

// MoveNode reparents a node, enforcing kind rules and acyclicity.
type MoveNode struct {
	Nodes ports.NodeStore
}

func (uc MoveNode) Execute(ctx context.Context, ownerID, id string, newParentID *string) (domain.Node, error) {
	node, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err
	}
	if newParentID == nil {
		if node.Kind != domain.KindEngagement {
			return domain.Node{}, fmt.Errorf("%w: only engagements may be roots", domain.ErrInvalidNode)
		}
		return uc.Nodes.Reparent(ctx, ownerID, id, nil)
	}
	parent, err := uc.Nodes.Get(ctx, ownerID, *newParentID)
	if err != nil {
		return domain.Node{}, err
	}
	if !domain.AllowedChildKind(parent.Kind, node.Kind) {
		return domain.Node{}, fmt.Errorf("%w: %s cannot be a child of %s", domain.ErrInvalidNode, node.Kind, parent.Kind)
	}
	// Cycle: the new parent's ancestor chain (which includes the new parent
	// itself) must not contain the node being moved.
	chain, err := uc.Nodes.Ancestors(ctx, ownerID, *newParentID)
	if err != nil {
		return domain.Node{}, err
	}
	for _, a := range chain {
		if a.ID == id {
			return domain.Node{}, ErrNodeCycle
		}
	}
	return uc.Nodes.Reparent(ctx, ownerID, id, newParentID)
}
```

- [ ] **Step 5: Rewrite `create_node.go`** (was the renamed flat `CreateNode`; keep `Slugify`/`nonSlug` here if they currently live in this file — otherwise leave them where A1 put them):
```go
package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateNodeInput is the field set for creating a node. Kind+ParentID drive the
// hierarchy validation; a nil ParentID means a root, which must be an engagement.
type CreateNodeInput struct {
	Name, Slug                             string
	Kind                                   domain.NodeKind
	ParentID                               *string
	Color, Glyph, Description, UpstreamGit string
}

// CreateNode creates an owner-scoped node, validating kind+parent placement.
type CreateNode struct {
	Nodes ports.NodeStore
	IDs   ports.IDGen
	Clock ports.Clock
}

func (uc CreateNode) Execute(ctx context.Context, ownerID string, in CreateNodeInput) (domain.Node, error) {
	slug := in.Slug
	if slug == "" {
		slug = Slugify(in.Name)
	}
	if in.ParentID == nil {
		if in.Kind != domain.KindEngagement {
			return domain.Node{}, fmt.Errorf("%w: root node must be an engagement", domain.ErrInvalidNode)
		}
	} else {
		parent, err := uc.Nodes.Get(ctx, ownerID, *in.ParentID)
		if err != nil {
			return domain.Node{}, err
		}
		if !domain.AllowedChildKind(parent.Kind, in.Kind) {
			return domain.Node{}, fmt.Errorf("%w: %s cannot be a child of %s", domain.ErrInvalidNode, in.Kind, parent.Kind)
		}
	}
	n, err := domain.NewNode(uc.IDs.NewID(), ownerID, in.Name, slug, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	n.Kind = in.Kind
	n.ParentID = in.ParentID
	n.Color, n.Glyph = in.Color, in.Glyph
	n.Description, n.UpstreamGit = in.Description, in.UpstreamGit
	if err := n.Validate(); err != nil {
		return domain.Node{}, err
	}
	return uc.Nodes.Create(ctx, n)
}
```

- [ ] **Step 6: Add target-kind validation to `bind_node.go`.** Add `Nodes ports.NodeStore` to the `BindNode` struct (A1 renamed the field `Projects`→`Nodes` already; keep it), set `NodeID: nodeID` on the binding, and validate:
```go
// ErrInvalidBindTarget is returned when a binding points at a node whose kind is
// not permitted for that binding kind (remote→repo; path→repo|leaf-vorhaben).
var ErrInvalidBindTarget = errors.New("usecase: invalid binding target kind")

func (uc BindNode) Execute(ctx context.Context, ownerID, nodeID string, k BindKey) (domain.ProjectBinding, error) {
	node, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.ProjectBinding{}, err
	}
	if err := uc.validateTarget(ctx, ownerID, node, k.Kind); err != nil {
		return domain.ProjectBinding{}, err
	}
	now := uc.Clock.Now()
	b := domain.ProjectBinding{
		ID:           uc.IDs.NewID(),
		OwnerID:      ownerID,
		NodeID:       nodeID,
		Kind:         k.Kind,
		RemoteSlug:   k.RemoteSlug,
		MachineID:    k.MachineID,
		MachineLabel: k.MachineLabel,
		Path:         k.Path,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return uc.Bindings.Upsert(ctx, b)
}

// validateTarget enforces: remote→repo; path→repo or a leaf (childless) vorhaben.
func (uc BindNode) validateTarget(ctx context.Context, ownerID string, node domain.Node, kind domain.BindingKind) error {
	switch kind {
	case domain.BindingRemote:
		if node.Kind != domain.KindRepo {
			return ErrInvalidBindTarget
		}
	case domain.BindingPath:
		if node.Kind == domain.KindRepo {
			return nil
		}
		if node.Kind == domain.KindVorhaben {
			children, err := uc.Nodes.Children(ctx, ownerID, &node.ID)
			if err != nil {
				return err
			}
			if len(children) == 0 {
				return nil
			}
		}
		return ErrInvalidBindTarget
	}
	return nil
}
```
(Add `"errors"` to the imports of `bind_node.go`.)

- [ ] **Step 7: Thread the new `CreateNode` signature through callers (keep `make ci` green).** In `internal/adapter/httpserver/worktime.go` `handleCreateNode`, build the input — for Slice A keep behavior minimal and CHECK-valid by defaulting to an engagement; Slice C adds `kind`/`parentId` to the request and a tree UI:
```go
	p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
		Name: req.Name, Slug: req.Slug, Color: req.Color, Glyph: req.Glyph,
		Kind: domain.KindEngagement, // TODO(Slice C): read kind/parentId from request
	})
```
Update `cmd/flow-server/main.go`: `CreateNode: usecase.CreateNode{Nodes: nodeStore, IDs: ids, Clock: clock}`, add `BindNode: usecase.BindNode{Bindings: bindingStore, Nodes: nodeStore, IDs: ids, Clock: clock}`, and add new constructors `ResolveEngagement: usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore}, Nodes: nodeStore}` and `MoveNode: usecase.MoveNode{Nodes: nodeStore}` (and the matching `Server` fields, even if not yet routed — Slice C wires the routes). Grep `rg "CreateNode\.Execute\(|\.CreateProject\.Execute\(" -g '*.go'` and fix every caller (CLI/apiclient handlers as needed).

- [ ] **Step 8: Run (expect PASS).** `go test ./internal/usecase/ ./internal/adapter/httpserver/` then `make ci`.

- [ ] **Step 9: Commit.** `feat(usecase): ResolveEngagement + MoveNode(cycle/kind) + CreateNode(kind/parent) + BindNode target-kind validation`

---

### Task A6: Data-fixup migration `0017_nodes_data_fixup.sql` + Docker integration test

The heavy hierarchy migration (contract's `0016` outline, now numbered `0017`): create the two default engagements per owner, parent legacy repos by rule, audit+clear repo rates, re-scope docs, re-point sessions to the engagement, drop the temp `kind` default, and add the four CHECK constraints. Plus a `MigrateUpTo` helper and a Docker integration test that stages legacy rows *before* the CHECKs land and asserts the post-migration shape.

**Files:**
- create `internal/adapter/pgstore/migrations/0017_nodes_data_fixup.sql`
- modify `internal/adapter/pgstore/pool.go` (add `MigrateUpTo`)
- test `internal/adapter/pgstore/nodes_fixup_test.go` (new; Docker)

**Interfaces (Produces):** `func MigrateUpTo(ctx context.Context, pool *pgxpool.Pool, version int64) error` (goose `UpToContext`).

Steps:

- [ ] **Step 1: Add `MigrateUpTo` to `pool.go`** (consumed by the test to stage migrations through a specific version):
```go
// MigrateUpTo applies up migrations through the given version (inclusive). Used
// by data-migration tests to stage rows before later migrations (e.g. CHECKs).
func MigrateUpTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("pgstore: dialect: %w", err)
	}
	if err := goose.UpToContext(ctx, db, "migrations", version); err != nil {
		return fmt.Errorf("pgstore: migrate up to %d: %w", version, err)
	}
	return nil
}
```

- [ ] **Step 2: Write `0017_nodes_data_fixup.sql`** (idempotent owner-scoped data ops, then drop default + add CHECKs):
```sql
-- +goose Up

-- 1. Ensure the two default engagements exist for every owner that has >=1 node.
INSERT INTO nodes (id, owner_id, parent_id, kind, name, slug, status, extra, created_at, updated_at)
SELECT 'eng-privat-' || o.owner_id, o.owner_id, NULL, 'engagement', 'Privat', 'privat', 'active', '{}', now(), now()
FROM (SELECT DISTINCT owner_id FROM nodes) o
WHERE NOT EXISTS (SELECT 1 FROM nodes n WHERE n.owner_id = o.owner_id AND n.slug = 'privat');

INSERT INTO nodes (id, owner_id, parent_id, kind, name, slug, status, extra, created_at, updated_at)
SELECT 'eng-rtl-extern-' || o.owner_id, o.owner_id, NULL, 'engagement', 'RTL Extern', 'rtl-extern', 'active', '{}', now(), now()
FROM (SELECT DISTINCT owner_id FROM nodes) o
WHERE NOT EXISTS (SELECT 1 FROM nodes n WHERE n.owner_id = o.owner_id AND n.slug = 'rtl-extern');

-- 2. Parent the legacy repos under an engagement (slug ~ gitlab → RTL Extern, else Privat).
--    Idempotent: only rows still at the root (parent_id IS NULL) are touched.
UPDATE nodes r
SET parent_id = e.id
FROM nodes e
WHERE r.kind = 'repo'
  AND r.parent_id IS NULL
  AND r.slug NOT IN ('privat','rtl-extern')
  AND e.owner_id = r.owner_id
  AND e.kind = 'engagement'
  AND e.slug = CASE WHEN r.slug ILIKE '%gitlab%' THEN 'rtl-extern' ELSE 'privat' END;

-- 3. Audit + clear rate on repos (rate belongs to the engagement now).
UPDATE nodes
SET extra = jsonb_set(extra, '{legacy_rate}',
        jsonb_build_object('amount', rate_amount, 'currency', rate_currency)),
    rate_amount = NULL,
    rate_currency = NULL
WHERE kind = 'repo' AND rate_amount IS NOT NULL;

-- 4. Re-scope documents by category. daily → RTL engagement; free → global (NULL).
--    project/agent keep their node_id (now a repo); instruction/memory stay NULL.
UPDATE documents d
SET node_id = e.id
FROM nodes e
WHERE d.type = 'daily'
  AND e.owner_id = d.owner_id
  AND e.kind = 'engagement'
  AND e.slug = 'rtl-extern';

UPDATE documents SET node_id = NULL WHERE type = 'free';

-- 5. Re-point work sessions from the repo to its engagement parent (booking = engagement).
--    Idempotent: once node_id points at the engagement the join no longer matches a repo.
UPDATE work_sessions ws
SET node_id = r.parent_id
FROM nodes r
WHERE ws.node_id = r.id
  AND r.kind = 'repo'
  AND r.parent_id IS NOT NULL;

-- 6. Drop the temporary kind default and lock in the static invariants.
ALTER TABLE nodes ALTER COLUMN kind DROP DEFAULT;

ALTER TABLE nodes ADD CONSTRAINT nodes_kind_enum
    CHECK (kind IN ('engagement','vorhaben','repo','branch'));
ALTER TABLE nodes ADD CONSTRAINT nodes_root_is_engagement
    CHECK (parent_id IS NOT NULL OR kind = 'engagement');
ALTER TABLE nodes ADD CONSTRAINT nodes_rate_only_engagement
    CHECK (rate_amount IS NULL OR kind = 'engagement');
ALTER TABLE nodes ADD CONSTRAINT nodes_origin_only_repo
    CHECK (origin_slug IS NULL OR kind = 'repo');

-- +goose Down
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_origin_only_repo;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_rate_only_engagement;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_root_is_engagement;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_kind_enum;
ALTER TABLE nodes ALTER COLUMN kind SET DEFAULT 'repo';
-- Data transformations are not reversed (audit lives in extra.legacy_rate).
```

- [ ] **Step 3: Write the failing Docker test `nodes_fixup_test.go`.**
```go
package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestMigration0017_DataFixup(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// Stage the schema through 0016 (columns present, NO CHECKs yet) so we can
	// insert legacy-shaped rows (repos at the root) that the fixup will repair.
	if err := pgstore.MigrateUpTo(ctx, pool, 16); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-fix", "sub-fix", "fixuser", "fix@x.de", "Fix")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	// Legacy repos at the root: 'flow' (rate set), 'gitlab-acme' (rate set).
	mustExec(t, pool, `INSERT INTO nodes (id, owner_id, name, slug, kind, rate_amount, rate_currency, created_at, updated_at)
		VALUES ('n-flow','u-fix','flow','flow','repo', 9500,'EUR',$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO nodes (id, owner_id, name, slug, kind, rate_amount, rate_currency, created_at, updated_at)
		VALUES ('n-gl','u-fix','acme on gitlab','gitlab-acme','repo', 8000,'EUR',$1,$1)`, now)

	// Docs: daily, free, project — node_id starts pointing at the legacy repo.
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, doc_date, created_at, updated_at)
		VALUES ('d-daily','u-fix','n-flow','daily','daily/2026-06-27',$1::date,$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, created_at, updated_at)
		VALUES ('d-free','u-fix','n-flow','free','scratch/idea',$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, created_at, updated_at)
		VALUES ('d-proj','u-fix','n-flow','project','flow/arch',$1,$1)`, now)

	// A session booked against the legacy repo.
	mustExec(t, pool, `INSERT INTO work_sessions (id, owner_id, node_id, start_at, stop_at, created_at)
		VALUES ('ws-1','u-fix','n-flow',$1,$2,$1)`, now, now.Add(time.Hour))

	// Run the data fixup + CHECKs.
	if err := pgstore.MigrateUpTo(ctx, pool, 17); err != nil {
		t.Fatalf("apply 0017: %v", err)
	}

	// --- engagements exist, are roots ---
	assertScalar(t, pool, `SELECT count(*) FROM nodes WHERE owner_id='u-fix' AND kind='engagement' AND parent_id IS NULL AND slug IN ('privat','rtl-extern')`, int64(2))

	// --- repos parented by rule: flow→privat, gitlab-acme→rtl-extern ---
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-flow'`, "eng-privat-u-fix")
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-gl'`, "eng-rtl-extern-u-fix")

	// --- rate audited into extra.legacy_rate and cleared ---
	assertScalar(t, pool, `SELECT count(*) FROM nodes WHERE id='n-flow' AND rate_amount IS NULL`, int64(1))
	assertText(t, pool, `SELECT extra->'legacy_rate'->>'amount' FROM nodes WHERE id='n-flow'`, "9500")

	// --- docs re-scoped ---
	assertText(t, pool, `SELECT node_id FROM documents WHERE id='d-daily'`, "eng-rtl-extern-u-fix")
	assertScalar(t, pool, `SELECT count(*) FROM documents WHERE id='d-free' AND node_id IS NULL`, int64(1))
	assertText(t, pool, `SELECT node_id FROM documents WHERE id='d-proj'`, "n-flow") // unchanged

	// --- session re-pointed to the engagement (flow's parent = privat) ---
	assertText(t, pool, `SELECT node_id FROM work_sessions WHERE id='ws-1'`, "eng-privat-u-fix")

	// --- CHECKs are live: a rootless repo is rejected ---
	if _, err := pool.Exec(ctx, `INSERT INTO nodes (id, owner_id, name, slug, kind, created_at, updated_at)
		VALUES ('bad','u-fix','bad','bad','repo',now(),now())`); err == nil {
		t.Error("nodes_root_is_engagement CHECK should reject a rootless repo")
	}
	// --- idempotency: re-running the data ops is a no-op (apply 0017 again is a goose no-op;
	//     re-running the UPDATEs by hand must not change parents) ---
	mustExec(t, pool, `UPDATE nodes r SET parent_id = e.id FROM nodes e
		WHERE r.kind='repo' AND r.parent_id IS NULL AND r.slug NOT IN ('privat','rtl-extern')
		  AND e.owner_id=r.owner_id AND e.kind='engagement'
		  AND e.slug = CASE WHEN r.slug ILIKE '%gitlab%' THEN 'rtl-extern' ELSE 'privat' END`)
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-flow'`, "eng-privat-u-fix")
}

func mustExec(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconnCommandTag, error)
}, q string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
```
> **Note for the implementer:** `*pgxpool.Pool.Exec` returns `(pgconn.CommandTag, error)`. Drop the `pgconnCommandTag` shim above and just take `*pgxpool.Pool` directly:
```go
func mustExec(t *testing.T, pool *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func assertScalar(t *testing.T, pool *pgxpool.Pool, q string, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(context.Background(), q).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if got != want {
		t.Errorf("%q = %d, want %d", q, got, want)
	}
}

func assertText(t *testing.T, pool *pgxpool.Pool, q, want string) {
	t.Helper()
	var got *string
	if err := pool.QueryRow(context.Background(), q).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if got == nil || *got != want {
		t.Errorf("%q = %v, want %q", q, got, want)
	}
}
```
(Import `"github.com/jackc/pgx/v5/pgxpool"` in the test. Use the clean `*pgxpool.Pool` versions of the helpers; the interface shim in the first block is illustrative only.)

- [ ] **Step 4: Run (expect FAIL — `MigrateUpTo` undefined / `0017` missing).** `go test ./internal/adapter/pgstore/ -run 0017`

- [ ] **Step 5: Implement** (Steps 1–2 already wrote `MigrateUpTo` + the SQL). Run again (expect PASS): `go test ./internal/adapter/pgstore/ -run 0017`.

- [ ] **Step 6: Full gate.** `make ci` (full migrate path runs `0015→0016→0017`; on a fresh empty DB the `0017` data ops are no-ops, so every other pgstore test still seeds valid hierarchy and passes). Confirm coverage gate held.

- [ ] **Step 7: Commit.** `feat(pgstore): 0017 nodes data fixup (default engagements, repo parenting, rate audit, doc/session re-scope, CHECKs) + MigrateUpTo + Docker test`

---

**Slice A done-state:** `domain.Node` is the canonical hub with a recursive hierarchy; pgstore persists `parent_id/kind/origin_slug/extra` with `WITH RECURSIVE` ancestor/child walks, RESTRICT deletes, and the four static CHECK invariants; the usecase layer can resolve cwd→repo→engagement, move nodes acyclically, create kind-valid nodes, and bind only valid targets; the legacy `projects` data is migrated into the new shape. `make ci` is green after every task. Slices B (worktime/export on engagement) and C (REST/SSE/CLI/UI) build on this foundation.

### Key cross-references for the parent / other slices
- **Preserved binding names** (do not rename): `domain.ProjectBinding` (field → `NodeID`), `domain.BindKey`, `ports.ProjectBindingStore`, `pgstore.ProjectBindingStore`, `testutil.FakeProjectBindingStore`, `ProjectBindingStore.ListByProject`. Confirmed by contract line 98.
- **`CreateNode` signature changed** to `Execute(ctx, ownerID, CreateNodeInput)` — the REST/CLI create paths are left compiling with a `KindEngagement` default in A5; **Slice C must add `kind`/`parentId` to the request + a tree UI** (flagged with a `TODO(Slice C)`).
- **New `Server` use-case fields** `ResolveEngagement` and `MoveNode` are wired in `main.go` in A5 but not yet routed — **Slice C adds `POST /api/v1/nodes/{id}/move` and the engagement-aware worktime/export wiring**.
- **Migration renumber:** contract's `0016_nodes_data_fixup.sql` ships as **`0017`** (A1 took `0015`, columns took `0016`). Highest migration after Slice A = `0017`.
- **`MigrateUpTo(ctx, pool, version int64)`** is a new reusable pgstore test helper for staging data migrations.


---

## Slice B — Rate / Worktime / Export on Engagement (D3)

**Baseline assumption (post-Slice-A):** the mechanical `Project→Node` rename is done. Concretely this slice relies on: `domain.WorkSession.NodeID *string`; `domain.Node` with `Kind NodeKind` + consts `KindEngagement`/`KindRepo`; `domain.ErrInvalidNode` (renamed from `ErrInvalidProject`); `ports.NodeStore` (with `Get(ctx, ownerID, id) (domain.Node, error)` → `ports.ErrNodeNotFound`, plus `List`, `SetRate`); `ports.SessionStore.Stop/Update` params named `nodeID`; `domain.NewWorkSession(id, ownerID string, nodeID *string, start)`; `testutil.FakeNodeStore` (`NewFakeNodeStore()`, `Create(ctx, domain.Node)`, `Get`, `List`, `SetRate`); composition-root var `nodeStore`; the rate usecase/handler/field/route renamed to `SetNodeRate` / `handleSetNodeRate` / `s.SetNodeRate` / `/api/v1/nodes/{id}/rate`. Where Slice A might not have touched something (e.g. the export domain types, the `BuildExport.Projects` field), the task below is its authoritative owner — apply the rename there if not already present.

**Test helpers available in `package usecase_test`:** `fakeSessionStore` + `ptr` (stats_computer_test.go), `fixedClock` (ics_settings_test.go), and the shared `testutil.*` fakes.

**Conventions for every step:** `package <pkg>_test`, `t.Parallel()`, gofumpt + `make ci` (lint, not just `go test`), owner-scoping. Every `git commit` message ends with the repo's `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` trailer.

---

### Task B1: StartSession + AddSession require an engagement node

Add a `Nodes ports.NodeStore` dependency to both worktime-create use cases and reject any `nodeID` that is not an existing engagement (worktime books to engagements, D3). A nil/empty `nodeID` still starts unbooked.

**Files**
- `internal/usecase/engagement_guard.go` (new — shared helper)
- `internal/usecase/start_session.go` (modify)
- `internal/usecase/add_session.go` (modify)
- `internal/usecase/start_session_test.go` (new)
- `internal/usecase/add_session_test.go` (rewrite)
- `internal/adapter/httpserver/worktime.go` (modify — map new errors in `handleStartSession`)

**Interfaces**
- *Consumes (contract / Slice A):* `ports.NodeStore.Get`, `ports.ErrNodeNotFound`, `domain.Node.Kind`, `domain.KindEngagement`, `domain.ErrInvalidNode`, `testutil.FakeNodeStore`.
- *Produces:* `usecase.requireEngagement(ctx, ports.NodeStore, ownerID, nodeID *string) error`; `StartSession{Sessions, Nodes, IDs, Clock}` with `Execute(ctx, ownerID string, nodeID *string, tag, note string)`; `AddSession{Sessions, Nodes, IDs, Clock}` with `Execute(ctx, ownerID string, nodeID *string, start, stop time.Time, tag, note string)`.

**Steps**

- [ ] **Step 1 — Failing tests for StartSession.** Create `internal/usecase/start_session_test.go` with the shared seed helpers (also used by B1/B2):
  ```go
  package usecase_test

  import (
  	"context"
  	"errors"
  	"testing"
  	"time"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/ports"
  	"github.com/serverkraken/flow/internal/testutil"
  	"github.com/serverkraken/flow/internal/usecase"
  )

  func newStartSession(ss *testutil.FakeSessionStore, ns *testutil.FakeNodeStore, now time.Time) usecase.StartSession {
  	return usecase.StartSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
  }

  // seedEngagement / seedRepo are shared by the worktime use-case tests.
  func seedEngagement(t *testing.T, ns *testutil.FakeNodeStore, ownerID, id string) {
  	t.Helper()
  	if _, err := ns.Create(context.Background(), domain.Node{
  		ID: id, OwnerID: ownerID, Kind: domain.KindEngagement,
  		Name: id, Slug: id, Status: domain.NodeActive,
  	}); err != nil {
  		t.Fatalf("seed engagement: %v", err)
  	}
  }

  func seedRepo(t *testing.T, ns *testutil.FakeNodeStore, ownerID, id string) {
  	t.Helper()
  	parent := "eng-root"
  	if _, err := ns.Create(context.Background(), domain.Node{
  		ID: id, OwnerID: ownerID, ParentID: &parent, Kind: domain.KindRepo,
  		Name: id, Slug: id, Status: domain.NodeActive,
  	}); err != nil {
  		t.Fatalf("seed repo: %v", err)
  	}
  }

  func TestStartSession_NilNodeStartsUnbooked(t *testing.T) {
  	t.Parallel()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
  	got, err := uc.Execute(context.Background(), "u1", nil, "deep", "n")
  	if err != nil {
  		t.Fatalf("start: %v", err)
  	}
  	if got.NodeID != nil {
  		t.Errorf("want nil NodeID, got %v", *got.NodeID)
  	}
  }

  func TestStartSession_EngagementAccepted(t *testing.T) {
  	t.Parallel()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	seedEngagement(t, ns, "u1", "eng1")
  	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
  	eng := "eng1"
  	got, err := uc.Execute(context.Background(), "u1", &eng, "", "")
  	if err != nil {
  		t.Fatalf("start: %v", err)
  	}
  	if got.NodeID == nil || *got.NodeID != "eng1" {
  		t.Errorf("want NodeID eng1, got %v", got.NodeID)
  	}
  }

  func TestStartSession_RepoRejected(t *testing.T) {
  	t.Parallel()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	seedRepo(t, ns, "u1", "repo1")
  	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
  	repo := "repo1"
  	if _, err := uc.Execute(context.Background(), "u1", &repo, "", ""); !errors.Is(err, domain.ErrInvalidNode) {
  		t.Fatalf("want ErrInvalidNode for repo node, got %v", err)
  	}
  }

  func TestStartSession_MissingNodeRejected(t *testing.T) {
  	t.Parallel()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	uc := newStartSession(ss, ns, time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC))
  	ghost := "ghost"
  	if _, err := uc.Execute(context.Background(), "u1", &ghost, "", ""); !errors.Is(err, ports.ErrNodeNotFound) {
  		t.Fatalf("want ErrNodeNotFound, got %v", err)
  	}
  }
  ```

- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/usecase/ -run TestStartSession` → fails to compile (`StartSession` has no `Nodes` field / `requireEngagement` undefined). Good.

- [ ] **Step 3 — Add the shared guard helper.** Create `internal/usecase/engagement_guard.go`:
  ```go
  package usecase

  import (
  	"context"
  	"fmt"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/ports"
  )

  // requireEngagement verifies that nodeID (when non-nil and non-empty) names an
  // existing node of kind engagement owned by ownerID. Worktime is booked at the
  // engagement level (D3); a nil/empty nodeID is allowed (unbooked). A missing or
  // foreign node surfaces the store's ErrNodeNotFound; a non-engagement kind
  // yields ErrInvalidNode.
  func requireEngagement(ctx context.Context, nodes ports.NodeStore, ownerID string, nodeID *string) error {
  	if nodeID == nil || *nodeID == "" {
  		return nil
  	}
  	n, err := nodes.Get(ctx, ownerID, *nodeID)
  	if err != nil {
  		return err
  	}
  	if n.Kind != domain.KindEngagement {
  		return fmt.Errorf("%w: worktime books to an engagement, got %s", domain.ErrInvalidNode, n.Kind)
  	}
  	return nil
  }
  ```

- [ ] **Step 4 — Wire the guard into StartSession.** Rewrite `internal/usecase/start_session.go`:
  ```go
  package usecase

  import (
  	"context"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/ports"
  )

  // StartSession begins the user's single running timer. nodeID is optional at
  // start; when set it must name an engagement (worktime books to engagements,
  // D3). tag/note are optional annotations.
  type StartSession struct {
  	Sessions ports.SessionStore
  	Nodes    ports.NodeStore
  	IDs      ports.IDGen
  	Clock    ports.Clock
  }

  func (uc StartSession) Execute(ctx context.Context, ownerID string, nodeID *string, tag, note string) (domain.WorkSession, error) {
  	if err := requireEngagement(ctx, uc.Nodes, ownerID, nodeID); err != nil {
  		return domain.WorkSession{}, err
  	}
  	if _, running, err := uc.Sessions.Running(ctx, ownerID); err != nil {
  		return domain.WorkSession{}, err
  	} else if running {
  		return domain.WorkSession{}, domain.ErrAlreadyRunning
  	}
  	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, nodeID, uc.Clock.Now())
  	if err != nil {
  		return domain.WorkSession{}, err
  	}
  	s.Tag, s.Note = tag, note
  	return uc.Sessions.Create(ctx, s)
  }
  ```

- [ ] **Step 5 — Run, expect PASS.** `go test ./internal/usecase/ -run TestStartSession` → all four pass.

- [ ] **Step 6 — Rewrite AddSession tests (failing).** Replace `internal/usecase/add_session_test.go` so the helper carries a `Nodes` store and the happy path seeds an engagement; add a repo-rejected case. Keep all existing invariant cases:
  ```go
  package usecase_test

  import (
  	"context"
  	"errors"
  	"testing"
  	"time"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/testutil"
  	"github.com/serverkraken/flow/internal/usecase"
  )

  func newAddSession(ss *testutil.FakeSessionStore, ns *testutil.FakeNodeStore, now time.Time) usecase.AddSession {
  	return usecase.AddSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: now}}
  }

  func TestAddSession_HappyPath(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	seedEngagement(t, ns, "u1", "p1")
  	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
  	uc := newAddSession(ss, ns, now)
  	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
  	pid := "p1"
  	got, err := uc.Execute(ctx, "u1", &pid, start, stop, "deep", "n")
  	if err != nil {
  		t.Fatalf("AddSession: %v", err)
  	}
  	if got.ID == "" || got.Stop == nil || !got.Stop.Equal(stop) || got.Tag != "deep" {
  		t.Fatalf("AddSession result wrong: %+v", got)
  	}
  	if got.CreatedAt != start {
  		t.Errorf("CreatedAt = %v, want start %v", got.CreatedAt, start)
  	}
  }

  func TestAddSession_RepoRejected(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	seedRepo(t, ns, "u1", "repo1")
  	uc := newAddSession(ss, ns, time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
  	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
  	repo := "repo1"
  	if _, err := uc.Execute(ctx, "u1", &repo, start, stop, "", ""); !errors.Is(err, domain.ErrInvalidNode) {
  		t.Fatalf("want ErrInvalidNode for repo node, got %v", err)
  	}
  }

  func TestAddSession_StopBeforeStart(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC))
  	start := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
  	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrStopBeforeStart) {
  		t.Fatalf("want ErrStopBeforeStart, got %v", err)
  	}
  }

  func TestAddSession_Future(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
  	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), now)
  	start := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
  	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrFutureSession) {
  		t.Fatalf("want ErrFutureSession, got %v", err)
  	}
  }

  func TestAddSession_CrossMidnight(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
  	uc := newAddSession(testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore(), now)
  	start := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC)
  	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrInvalidSession) {
  		t.Fatalf("want ErrInvalidSession (cross-midnight), got %v", err)
  	}
  }

  func TestAddSession_Overlap(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
  	existingStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
  	if _, err := ss.Create(ctx, domain.WorkSession{
  		ID: "x", OwnerID: "u1",
  		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &existingStop,
  	}); err != nil {
  		t.Fatalf("seed: %v", err)
  	}
  	uc := newAddSession(ss, ns, now)
  	start := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
  	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrOverlap) {
  		t.Fatalf("want ErrOverlap, got %v", err)
  	}
  }

  func TestAddSession_OverlapWithRunningOutsideWindow(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	if _, err := ss.Create(ctx, domain.WorkSession{
  		ID: "running", OwnerID: "u1",
  		Start: time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC), Stop: nil,
  	}); err != nil {
  		t.Fatalf("seed running session: %v", err)
  	}
  	now := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
  	uc := newAddSession(ss, ns, now)
  	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
  	stop := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
  	if _, err := uc.Execute(ctx, "u1", nil, start, stop, "", ""); !errors.Is(err, domain.ErrOverlap) {
  		t.Fatalf("want ErrOverlap (running session spans candidate), got %v", err)
  	}
  }
  ```

- [ ] **Step 7 — Run, expect FAIL.** `go test ./internal/usecase/ -run TestAddSession` → compile failure (`AddSession` has no `Nodes` field). Good.

- [ ] **Step 8 — Wire the guard into AddSession.** In `internal/usecase/add_session.go` add the field and call the guard first; rename the param `projectID`→`nodeID` and pass it to `NewWorkSession`:
  ```go
  type AddSession struct {
  	Sessions ports.SessionStore
  	Nodes    ports.NodeStore
  	IDs      ports.IDGen
  	Clock    ports.Clock
  }

  func (uc AddSession) Execute(ctx context.Context, ownerID string, nodeID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error) {
  	if err := requireEngagement(ctx, uc.Nodes, ownerID, nodeID); err != nil {
  		return domain.WorkSession{}, err
  	}
  	if !stop.After(start) {
  		return domain.WorkSession{}, domain.ErrStopBeforeStart
  	}
  	// ... (existing now/future, same-day, overlap checks unchanged) ...
  	s, err := domain.NewWorkSession(uc.IDs.NewID(), ownerID, nodeID, start)
  	if err != nil {
  		return domain.WorkSession{}, err
  	}
  	s.Stop = &stop
  	s.Tag, s.Note = tag, note
  	return uc.Sessions.Create(ctx, s)
  }
  ```
  (Only the struct field, the guard call, the param name, and the `NewWorkSession` arg change; the body of the time/overlap checks is untouched.)

- [ ] **Step 9 — Run, expect PASS.** `go test ./internal/usecase/ -run 'TestAddSession|TestStartSession'` → green.

- [ ] **Step 10 — Map the new errors in the REST handler.** In `internal/adapter/httpserver/worktime.go`, extend the error switches in `handleStartSession` so a kind violation is `400` and a missing node is `404`. Add to the `AddSession` branch:
  ```go
  case errors.Is(err, domain.ErrInvalidNode):
  	http.Error(w, "worktime can only be booked to an engagement", http.StatusBadRequest)
  	return
  case errors.Is(err, ports.ErrNodeNotFound):
  	http.Error(w, "not found", http.StatusNotFound)
  	return
  ```
  and convert the live-start path's `if errors.Is(...)` chain into a `switch` with the same two cases plus the existing `ErrAlreadyRunning → 409` and `default → 500`. (`ports` is already imported.)

- [ ] **Step 11 — Build + commit.** `go build ./... && go test ./internal/usecase/ ./internal/adapter/httpserver/` → green. `git commit -am "feat(worktime): StartSession/AddSession require an engagement node (D3)"`.

---

### Task B2: StopSession books only to an engagement (booking-path completion)

**Scope note:** the literal contract list names Start/Add; this task extends it to `StopSession` because Stop is the path where booking is mandatory (`ErrProjectRequired`), so leaving it unguarded would let a non-engagement node be booked, defeating D3. Drop this task only if the human deliberately wants Stop to accept any node.

**Files**
- `internal/usecase/stop_session.go` (modify)
- `internal/usecase/stop_session_split_test.go` (modify — seed `KindEngagement`, add negative case)

**Interfaces**
- *Consumes:* `ports.NodeStore.Get`, `domain.KindEngagement`, `domain.ErrInvalidNode`, `domain.ErrProjectRequired` (existing).
- *Produces:* unchanged signature `StopSession.Execute(ctx, ownerID, sessionID string, nodeID *string)`; struct field is the post-A `Nodes ports.NodeStore`.

**Steps**

- [ ] **Step 1 — Add a failing negative test + fix seeds.** In `stop_session_split_test.go`, make the two existing seeds engagements and add a repo-rejected case. Change the `ps`/`Projects` usage to the post-A `Nodes`/`FakeNodeStore` form and seed kind:
  ```go
  ns := testutil.NewFakeNodeStore()
  if _, err := ns.Create(ctx, domain.Node{ID: "p1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "RTL Extern", Slug: "rtl-extern", Status: domain.NodeActive}); err != nil {
  	t.Fatalf("seed engagement: %v", err)
  }
  // ...
  uc := usecase.StopSession{Sessions: ss, Nodes: ns, IDs: ids, Clock: clk, Loc: loc}
  ```
  Add:
  ```go
  func TestStopSession_RepoRejected(t *testing.T) {
  	t.Parallel()
  	ctx := context.Background()
  	loc := time.UTC
  	ss, ns := testutil.NewFakeSessionStore(), testutil.NewFakeNodeStore()
  	seedRepo(t, ns, "u1", "repo1")
  	_, _ = ss.Create(ctx, domain.WorkSession{ID: "run", OwnerID: "u1", Start: time.Date(2026, 6, 24, 9, 0, 0, 0, loc)})
  	uc := usecase.StopSession{Sessions: ss, Nodes: ns, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Date(2026, 6, 24, 12, 0, 0, 0, loc)}, Loc: loc}
  	repo := "repo1"
  	if _, err := uc.Execute(ctx, "u1", "run", &repo); !errors.Is(err, domain.ErrInvalidNode) {
  		t.Fatalf("want ErrInvalidNode booking a repo, got %v", err)
  	}
  }
  ```
  Also add `s.NodeID` (not `s.ProjectID`) in the split assertions and ensure the two split/same-day tests still check `*s.NodeID == "p1"`.

- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/usecase/ -run TestStopSession` → `TestStopSession_RepoRejected` fails (repo currently accepted).

- [ ] **Step 3 — Add the kind check.** In `internal/usecase/stop_session.go`, capture the `Get` result and reject non-engagement (keep the mandatory `ErrProjectRequired` guard and the daily-split logic intact):
  ```go
  func (uc StopSession) Execute(ctx context.Context, ownerID, sessionID string, nodeID *string) (domain.WorkSession, error) {
  	if nodeID == nil || *nodeID == "" {
  		return domain.WorkSession{}, domain.ErrProjectRequired
  	}
  	n, err := uc.Nodes.Get(ctx, ownerID, *nodeID)
  	if err != nil {
  		return domain.WorkSession{}, err // ErrNodeNotFound bubbles to a 404
  	}
  	if n.Kind != domain.KindEngagement {
  		return domain.WorkSession{}, fmt.Errorf("%w: worktime books to an engagement, got %s", domain.ErrInvalidNode, n.Kind)
  	}
  	// ... existing Get(session) + SplitDaily + per-day Stop/Create unchanged ...
  }
  ```
  Add `"fmt"` to imports. The struct comment should say "books it to an engagement".

- [ ] **Step 4 — Map the error in the handler.** In `worktime.go` `handleStopSession`, add to the switch:
  ```go
  case errors.Is(err, domain.ErrInvalidNode):
  	http.Error(w, "worktime can only be booked to an engagement", http.StatusBadRequest)
  	return
  case errors.Is(err, ports.ErrNodeNotFound) || errors.Is(err, ports.ErrSessionNotFound):
  	http.Error(w, "not found", http.StatusNotFound)
  	return
  ```
  (replace the existing `ports.ErrProjectNotFound` case with `ports.ErrNodeNotFound`).

- [ ] **Step 5 — Run, expect PASS + commit.** `go test ./internal/usecase/ ./internal/adapter/httpserver/` → green. `git commit -am "feat(worktime): StopSession books only to an engagement node"`.

---

### Task B3: SetNodeRate guards kind==engagement

The store's `SetRate` does not re-check kind (per `ports.NodeStore` contract). Add the guard in the use case and map the new error in the REST handler.

**Files**
- `internal/usecase/set_node_rate.go` (modify — post-A rename of `set_project_rate.go`)
- `internal/usecase/set_node_rate_test.go` (new)
- `internal/usecase/export_test.go` (remove the old `TestSetProjectRate_Validates` to avoid a duplicate / stale-fake test; the export aggregation tests are rewritten in B4)
- `internal/adapter/httpserver/export.go` (modify `handleSetNodeRate` error switch)

**Interfaces**
- *Consumes:* `ports.NodeStore.Get` + `SetRate`, `ports.ErrNodeNotFound`, `domain.KindEngagement`, `domain.ErrInvalidNode`, `domain.ErrInvalidRate`.
- *Produces:* `SetNodeRate{Nodes ports.NodeStore}` with `Execute(ctx, ownerID, nodeID string, rate *domain.Money) error` that rejects non-engagement targets.

**Steps**

- [ ] **Step 1 — Failing test.** Create `internal/usecase/set_node_rate_test.go`:
  ```go
  package usecase_test

  import (
  	"context"
  	"errors"
  	"testing"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/testutil"
  	"github.com/serverkraken/flow/internal/usecase"
  )

  func TestSetNodeRate_ValidatesRate(t *testing.T) {
  	t.Parallel()
  	ns := testutil.NewFakeNodeStore()
  	seedEngagement(t, ns, "u1", "eng1")
  	uc := usecase.SetNodeRate{Nodes: ns}
  	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: -1, Currency: "EUR"}); !errors.Is(err, domain.ErrInvalidRate) {
  		t.Errorf("negative amount: want ErrInvalidRate, got %v", err)
  	}
  	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: 1, Currency: "EU"}); !errors.Is(err, domain.ErrInvalidRate) {
  		t.Errorf("bad currency: want ErrInvalidRate, got %v", err)
  	}
  	if err := uc.Execute(context.Background(), "u1", "eng1", nil); err != nil {
  		t.Errorf("nil rate (clear) on engagement should succeed: %v", err)
  	}
  	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: 5000, Currency: "EUR"}); err != nil {
  		t.Errorf("valid rate on engagement should succeed: %v", err)
  	}
  }

  func TestSetNodeRate_RepoRejected(t *testing.T) {
  	t.Parallel()
  	ns := testutil.NewFakeNodeStore()
  	seedRepo(t, ns, "u1", "repo1")
  	uc := usecase.SetNodeRate{Nodes: ns}
  	if err := uc.Execute(context.Background(), "u1", "repo1", &domain.Money{Amount: 5000, Currency: "EUR"}); !errors.Is(err, domain.ErrInvalidNode) {
  		t.Fatalf("want ErrInvalidNode setting rate on a repo, got %v", err)
  	}
  }

  func TestSetNodeRate_MissingNode(t *testing.T) {
  	t.Parallel()
  	uc := usecase.SetNodeRate{Nodes: testutil.NewFakeNodeStore()}
  	if err := uc.Execute(context.Background(), "u1", "ghost", &domain.Money{Amount: 5000, Currency: "EUR"}); err == nil {
  		t.Fatal("want error for missing node")
  	}
  }
  ```
  Delete `TestSetProjectRate_Validates` from `internal/usecase/export_test.go`.

- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/usecase/ -run TestSetNodeRate` → fails (Execute has no kind guard / old name). Good.

- [ ] **Step 3 — Add the guard.** Rewrite `internal/usecase/set_node_rate.go`:
  ```go
  package usecase

  import (
  	"context"
  	"fmt"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/ports"
  )

  // SetNodeRate validates and stores (or clears) an engagement's per-hour rate.
  // Only engagement nodes may carry a rate (D3); the store does not re-check kind,
  // so the guard lives here.
  type SetNodeRate struct {
  	Nodes ports.NodeStore
  }

  // Execute validates rate (when non-nil), then verifies the target is an
  // engagement before delegating to the store. A nil rate clears any existing rate.
  func (uc SetNodeRate) Execute(ctx context.Context, ownerID, nodeID string, rate *domain.Money) error {
  	if rate != nil {
  		if rate.Amount < 0 || len(rate.Currency) != 3 {
  			return domain.ErrInvalidRate
  		}
  	}
  	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
  	if err != nil {
  		return err // ErrNodeNotFound bubbles to a 404
  	}
  	if n.Kind != domain.KindEngagement {
  		return fmt.Errorf("%w: only an engagement may carry a rate, got %s", domain.ErrInvalidNode, n.Kind)
  	}
  	return uc.Nodes.SetRate(ctx, ownerID, nodeID, rate)
  }
  ```

- [ ] **Step 4 — Map the handler error.** In `internal/adapter/httpserver/export.go` `handleSetNodeRate`, change `s.SetNodeRate.Execute(...)` switch to:
  ```go
  switch {
  case errors.Is(err, domain.ErrInvalidRate):
  	http.Error(w, "invalid rate", http.StatusBadRequest)
  case errors.Is(err, domain.ErrInvalidNode):
  	http.Error(w, "only an engagement may carry a rate", http.StatusBadRequest)
  case errors.Is(err, ports.ErrNodeNotFound):
  	http.Error(w, "not found", http.StatusNotFound)
  case err != nil:
  	http.Error(w, "server error", http.StatusInternalServerError)
  default:
  	w.WriteHeader(http.StatusNoContent)
  }
  ```

- [ ] **Step 5 — Run, expect PASS + commit.** `go test ./internal/usecase/ ./internal/adapter/httpserver/` → green. `git commit -am "feat(node): SetNodeRate rejects rates on non-engagement nodes"`.

---

### Task B4: Export aggregates per engagement (`NodeTotal` / `ByEngagement`)

Repoint `BuildExport` onto `ports.NodeStore`, rename the export value types, and aggregate the (already engagement-keyed, post-migration) sessions per engagement. Money rounding via `Money.Mul` is preserved.

**Files**
- `internal/domain/export.go` (rename `ProjectTotal`→`NodeTotal`, `ByProject`→`ByEngagement`, `ExportRow.ProjectName`→`NodeName`; update writers)
- `internal/domain/export_test.go` (update `sampleExport()` + pipe-escape test to new field names; serialized-output assertions stay)
- `internal/usecase/export.go` (rewrite `Execute`)
- `internal/usecase/export_test.go` (rewrite aggregation tests with a node store)
- `internal/adapter/httpserver/webui_export.go` (consumer: `data.ByEngagement`, `pt.NodeName`)

**Interfaces**
- *Consumes:* `ports.NodeStore.List`, `domain.Node{Name, Rate}`, `domain.Money.Mul`, `domain.WorkSession.NodeID`.
- *Produces:* `domain.NodeTotal{NodeID, NodeName string; Total time.Duration; SessionCount int; Rate, Amount *Money}`; `domain.ExportData.ByEngagement []NodeTotal`; `domain.ExportRow.NodeName`; `BuildExport{Sessions, Nodes ports.NodeStore, Clock, Loc}` with `Execute(ctx, ownerID string, from, to time.Time, engagementID string)`.

**Steps**

- [ ] **Step 1 — Rename the domain types + writers.** In `internal/domain/export.go`:
  - `ExportData`: `ByProject []ProjectTotal` → `ByEngagement []NodeTotal`.
  - Rename `ProjectTotal` → `NodeTotal`, fields `ProjectID`→`NodeID`, `ProjectName`→`NodeName` (keep `Total`, `SessionCount`, `Rate`, `Amount`).
  - `ExportRow.ProjectName` → `NodeName`.
  - In `WriteCSV`/`WriteJSON`/`WriteMarkdown`: replace `d.ByProject`→`d.ByEngagement`, `p.ProjectName`→`p.NodeName`, `r.ProjectName`→`r.NodeName`. **Keep the serialized keys/labels unchanged** (`projOut.Project json:"project"`, the `byProject` JSON key, the `| Projekt |` markdown header) so the export wire format is stable in this slice — relabeling to "Engagement" is a Slice-E/C polish (see handoff).

- [ ] **Step 2 — Update the domain export tests.** In `internal/domain/export_test.go`, change `sampleExport()` and the pipe-escape literal to `domain.NodeTotal{NodeID:..., NodeName:...}`, `ByEngagement:`, and `ExportRow{... NodeName: ...}`. Leave every `strings.Contains` assertion as-is (they assert serialized output, which is unchanged). Run `go test ./internal/domain/` → PASS.

- [ ] **Step 3 — Failing usecase tests.** Rewrite `internal/usecase/export_test.go` to seed engagement nodes via a node store and assert per-engagement aggregation (drop the old `fakeProjectStore`; use `testutil.FakeNodeStore`):
  ```go
  package usecase_test

  import (
  	"context"
  	"testing"
  	"time"

  	"github.com/serverkraken/flow/internal/domain"
  	"github.com/serverkraken/flow/internal/testutil"
  	"github.com/serverkraken/flow/internal/usecase"
  )

  func TestBuildExport_AggregatesByEngagement(t *testing.T) {
  	t.Parallel()
  	loc := time.UTC
  	ns := testutil.NewFakeNodeStore()
  	rate := domain.Money{Amount: 8000, Currency: "EUR"}
  	if _, err := ns.Create(context.Background(), domain.Node{ID: "eng1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "RTL Extern", Slug: "rtl-extern", Status: domain.NodeActive, Rate: &rate}); err != nil {
  		t.Fatalf("seed: %v", err)
  	}
  	eng := "eng1"
  	sessions := []domain.WorkSession{
  		{ID: "a", NodeID: &eng, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 11, 0, 0, 0, loc))},   // 2h
  		{ID: "b", NodeID: &eng, Start: time.Date(2026, 6, 15, 12, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 12, 30, 0, 0, loc))}, // 30m
  		{ID: "run", NodeID: &eng, Start: time.Date(2026, 6, 15, 13, 0, 0, 0, loc), Stop: nil},                                          // running → excluded
  	}
  	uc := usecase.BuildExport{Sessions: fakeSessionStore{list: sessions}, Nodes: ns, Clock: fixedClock{t: time.Date(2026, 6, 16, 0, 0, 0, 0, loc)}, Loc: loc}
  	data, err := uc.Execute(context.Background(), "u1", time.Date(2026, 6, 1, 0, 0, 0, 0, loc), time.Date(2026, 6, 30, 0, 0, 0, 0, loc), "")
  	if err != nil {
  		t.Fatal(err)
  	}
  	if len(data.Sessions) != 2 {
  		t.Fatalf("want 2 detail rows (running excluded), got %d", len(data.Sessions))
  	}
  	if len(data.ByEngagement) != 1 || data.ByEngagement[0].Total != 150*time.Minute {
  		t.Fatalf("aggregate: got %+v", data.ByEngagement)
  	}
  	if data.ByEngagement[0].NodeName != "RTL Extern" {
  		t.Errorf("name: got %q", data.ByEngagement[0].NodeName)
  	}
  	if data.ByEngagement[0].Amount == nil || data.ByEngagement[0].Amount.Amount != 20000 {
  		t.Errorf("amount: got %+v want 20000 (2.5h*8000)", data.ByEngagement[0].Amount)
  	}
  }

  func TestBuildExport_ExcludesOutOfRangeAndUnbooked(t *testing.T) {
  	t.Parallel()
  	loc := time.UTC
  	ns := testutil.NewFakeNodeStore()
  	_, _ = ns.Create(context.Background(), domain.Node{ID: "eng1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "X", Slug: "x", Status: domain.NodeActive})
  	eng := "eng1"
  	sessions := []domain.WorkSession{
  		{ID: "before", NodeID: &eng, Start: time.Date(2026, 5, 31, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 5, 31, 10, 0, 0, 0, loc))},
  		{ID: "in", NodeID: &eng, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc))},
  		{ID: "after", NodeID: &eng, Start: time.Date(2026, 7, 1, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 7, 1, 10, 0, 0, 0, loc))},
  		{ID: "unbooked", NodeID: nil, Start: time.Date(2026, 6, 16, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 16, 10, 0, 0, 0, loc))},
  	}
  	uc := usecase.BuildExport{Sessions: fakeSessionStore{list: sessions}, Nodes: ns, Clock: fixedClock{t: time.Date(2026, 6, 20, 0, 0, 0, 0, loc)}, Loc: loc}
  	data, err := uc.Execute(context.Background(), "u1", time.Date(2026, 6, 1, 0, 0, 0, 0, loc), time.Date(2026, 6, 30, 0, 0, 0, 0, loc), "")
  	if err != nil {
  		t.Fatal(err)
  	}
  	if len(data.Sessions) != 1 {
  		t.Fatalf("want 1 in-range booked session, got %d", len(data.Sessions))
  	}
  }

  func TestBuildExport_FilterByEngagement(t *testing.T) {
  	t.Parallel()
  	loc := time.UTC
  	ns := testutil.NewFakeNodeStore()
  	_, _ = ns.Create(context.Background(), domain.Node{ID: "e1", OwnerID: "u1", Kind: domain.KindEngagement, Name: "A", Slug: "a", Status: domain.NodeActive})
  	_, _ = ns.Create(context.Background(), domain.Node{ID: "e2", OwnerID: "u1", Kind: domain.KindEngagement, Name: "B", Slug: "b", Status: domain.NodeActive})
  	e1, e2 := "e1", "e2"
  	sessions := []domain.WorkSession{
  		{ID: "a", NodeID: &e1, Start: time.Date(2026, 6, 15, 9, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 10, 0, 0, 0, loc))},
  		{ID: "b", NodeID: &e2, Start: time.Date(2026, 6, 15, 11, 0, 0, 0, loc), Stop: ptr(time.Date(2026, 6, 15, 12, 0, 0, 0, loc))},
  	}
  	uc := usecase.BuildExport{Sessions: fakeSessionStore{list: sessions}, Nodes: ns, Clock: fixedClock{t: time.Date(2026, 6, 16, 0, 0, 0, 0, loc)}, Loc: loc}
  	data, err := uc.Execute(context.Background(), "u1", time.Date(2026, 6, 1, 0, 0, 0, 0, loc), time.Date(2026, 6, 30, 0, 0, 0, 0, loc), "e1")
  	if err != nil {
  		t.Fatal(err)
  	}
  	if len(data.ByEngagement) != 1 || data.ByEngagement[0].NodeID != "e1" {
  		t.Fatalf("filter: got %+v", data.ByEngagement)
  	}
  }

  func TestBuildExport_NilLoc(t *testing.T) {
  	t.Parallel()
  	uc := usecase.BuildExport{Sessions: fakeSessionStore{}, Nodes: testutil.NewFakeNodeStore(), Clock: fixedClock{t: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)}, Loc: nil}
  	if _, err := uc.Execute(context.Background(), "u1", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), ""); err != nil {
  		t.Fatalf("Execute with nil loc: %v", err)
  	}
  }
  ```

- [ ] **Step 4 — Run, expect FAIL.** `go test ./internal/usecase/ -run TestBuildExport` → compile failure (`BuildExport` has no `Nodes` field, `ByEngagement` unknown). Good.

- [ ] **Step 5 — Rewrite `BuildExport.Execute`.** Replace the struct + Execute in `internal/usecase/export.go`:
  ```go
  // BuildExport aggregates a user's booked (stopped) sessions in [from,to] by
  // engagement into a domain.ExportData, resolving engagement names + rates. The
  // running session is excluded. engagementID "" means all engagements.
  type BuildExport struct {
  	Sessions ports.SessionStore
  	Nodes    ports.NodeStore
  	Clock    ports.Clock
  	Loc      *time.Location
  }

  func (uc BuildExport) loc() *time.Location {
  	if uc.Loc != nil {
  		return uc.Loc
  	}
  	return time.Local
  }

  // Execute aggregates stopped sessions between from and to (inclusive day
  // boundaries). engagementID filters to a single engagement when non-empty.
  // Sessions store an engagement node_id post-migration, so grouping by NodeID
  // already groups per engagement; the name/rate are resolved via NodeStore.
  func (uc BuildExport) Execute(ctx context.Context, ownerID string, from, to time.Time, engagementID string) (domain.ExportData, error) {
  	loc := uc.loc()
  	lo := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
  	toNorm := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)
  	hi := toNorm.AddDate(0, 0, 1)

  	sessions, err := uc.Sessions.List(ctx, ownerID, lo)
  	if err != nil {
  		return domain.ExportData{}, err
  	}
  	nodes, err := uc.Nodes.List(ctx, ownerID)
  	if err != nil {
  		return domain.ExportData{}, err
  	}
  	byID := make(map[string]domain.Node, len(nodes))
  	for _, n := range nodes {
  		byID[n.ID] = n
  	}

  	data := domain.ExportData{From: lo, To: toNorm}
  	totals := map[string]*domain.NodeTotal{}

  	for _, s := range sessions {
  		if s.Running() || s.NodeID == nil {
  			continue
  		}
  		start := s.Start.In(loc)
  		if start.Before(lo) || !start.Before(hi) {
  			continue
  		}
  		if engagementID != "" && *s.NodeID != engagementID {
  			continue
  		}
  		n := byID[*s.NodeID]
  		name := n.Name
  		if name == "" {
  			name = "(unbekannt)"
  		}
  		stop := s.Stop.In(loc)
  		el := stop.Sub(start)
  		data.Sessions = append(data.Sessions, domain.ExportRow{
  			Date: start, Start: start, Stop: stop, Elapsed: el,
  			NodeName: name, Tag: s.Tag, Note: s.Note,
  		})
  		t, ok := totals[*s.NodeID]
  		if !ok {
  			t = &domain.NodeTotal{NodeID: *s.NodeID, NodeName: name, Rate: n.Rate}
  			totals[*s.NodeID] = t
  		}
  		t.Total += el
  		t.SessionCount++
  	}

  	for _, t := range totals {
  		if t.Rate != nil {
  			a := t.Rate.Mul(t.Total)
  			t.Amount = &a
  		}
  		data.ByEngagement = append(data.ByEngagement, *t)
  	}

  	sort.Slice(data.ByEngagement, func(i, j int) bool {
  		return data.ByEngagement[i].NodeName < data.ByEngagement[j].NodeName
  	})
  	sort.Slice(data.Sessions, func(i, j int) bool {
  		return data.Sessions[i].Start.Before(data.Sessions[j].Start)
  	})
  	return data, nil
  }
  ```

- [ ] **Step 6 — Fix the WebUI consumer.** In `internal/adapter/httpserver/webui_export.go`: `len(data.ByProject)`→`len(data.ByEngagement)`, `for _, pt := range data.ByProject`→`data.ByEngagement`, `Project: pt.ProjectName`→`Project: pt.NodeName`. (The `webui.ExportSummaryRow.Project` field name stays.)

- [ ] **Step 7 — Run, expect PASS + commit.** `go build ./... && go test ./internal/domain/ ./internal/usecase/ ./internal/adapter/httpserver/` → green. `git commit -am "feat(export): aggregate worktime per engagement (NodeTotal/ByEngagement)"`.

---

### Task B5: Composition-root wiring + non-UI call sites + `make ci` gate

**Files**
- `cmd/flow-server/main.go` (add `Nodes` to `StartSession`/`AddSession`; ensure `BuildExport`/`SetNodeRate` use `nodeStore`)
- `internal/adapter/httpserver/server.go` (confirm `SetNodeRate`/`BuildExport` field types compile)
- (verify only) `cmd/flow/session.go`, `cmd/flow/worktime_import.go`, `cmd/flow/project.go`

**Interfaces**
- *Consumes:* the post-A `nodeStore` (`ports.NodeStore`).
- *Produces:* a buildable composition root where every worktime/rate/export use case has its `Nodes` dependency injected.

**Steps**

- [ ] **Step 1 — Inject `Nodes` into the worktime constructors.** In `cmd/flow-server/main.go`:
  ```go
  StartSession: usecase.StartSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock},
  AddSession:   usecase.AddSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock},
  ```
  Confirm `BuildExport{Sessions: sessionStore, Nodes: nodeStore, Clock: clock, Loc: time.Local}` and `SetNodeRate{Nodes: nodeStore}` (rename the field key from `Projects:`/`projectStore` if Slice A left it). `StopSession` already carries `Nodes: nodeStore` from B2's expectations.

- [ ] **Step 2 — Build.** `go build ./...` → green. Fix any remaining `Projects:` field keys or `s.SetProjectRate`/`s.BuildExport.Projects` references the rename touched.

- [ ] **Step 3 — Verify non-UI CLI call sites compile unchanged.** `cmd/flow/session.go`, `cmd/flow/worktime_import.go`, and `cmd/flow/project.go` call the apiclient (`c.AddSession`, the rate setter), whose Go signatures are unchanged (they pass a node id string/pointer). No code change needed; just confirm `go build ./cmd/...`. **Hand-off:** these CLI worktime paths must resolve their node id to an *engagement* — that resolution lands in Slice C (`ResolveEngagement` + `flow node` wiring); until then a repo id will be rejected at the API with `ErrInvalidNode` (HTTP 400).

- [ ] **Step 4 — Full gate.** `make ci` (lint + tests + coverage) → green; do not regress the coverage gate. Then a live curl smoke vs Postgres+Dex: start with an engagement id (`201`), start with a repo id (`400`), `PUT /api/v1/nodes/{repo}/rate` (`400`), `GET /api/v1/export?from&to&format=md` shows per-engagement rows with Σh×rate.

- [ ] **Step 5 — Commit.** `git commit -am "feat(wiring): inject NodeStore into worktime/export use cases (B-slice)"`.

**Hand-off to UI slices D (WebUI) / E (TUI) — worktime picker must list ENGAGEMENTS, not repos:**
- TUI booking dialogs `internal/tui/screen/worktime/dialogs.go` (start) and `internal/tui/screen/worktime/daydetail/dialogs.go` (Nachbuchen) currently feed a project picker; they must offer **engagements** (MRU + fuzzy via `ui/fuzzylist`) and surface `ErrInvalidNode` as a user-facing error. `route.go`/`api.go` interface signatures are unchanged.
- WebUI booking: `internal/adapter/httpserver/webui_worktime.go` (`handleAddSession`) and `internal/adapter/httpserver/webui.go` (live start) — the booking form's selector must list engagements; the engagement is the `nodeId` posted.
- Export UI relabeling (CSV/JSON/MD column headers and the `byProject`/`project` wire keys → "engagement") is intentionally deferred from this slice; pick it up in Slice C/E if the redesign wants engagement-labeled output.


---

## Slice C — API / SSE / CLI (hierarchy surface)

> **Preconditions (Slices A+B, assumed done):** code uses `domain.Node`/`ports.NodeStore`/`node_id`; REST mechanically renamed to `/api/v1/nodes/*` with handlers `handleCreateNode/handleListNodes/handleGetNode/handleUpdateNode/handleDeleteNode` (the old project handlers in `worktime.go`, renamed); `domain.EventNodeCreated/Updated/Deleted` (`node.created/updated/deleted`) exist in `internal/domain/event.go`; usecases `ResolveNode`, `ResolveEngagement`, `MoveNode` (with `usecase.ErrNodeCycle`), `CreateNode` (kind+parent validation, `domain.ErrInvalidNode`), `BindNode` (target-kind validation, `usecase.ErrInvalidBindTarget`) and `ports.NodeStore.{Children,Ancestors,Reparent,Delete}` (`ports.ErrNodeNotFound`, `ports.ErrNodeHasChildren`) exist; `testutil.NewFakeNodeStore()` exists; apiclient renamed (`CreateNode(ctx,name)`, `ListNodes`, `GetNode`, `UpdateNode`, `DeleteNode`, `ResolveNode`, `BindRemote`/`BindPath`/`UnbindRemote`/`UnbindPath`/`ListBindings`, `SetNodeRate`); CLI command renamed `nodeCmd()` with `nodeRateCmd/nodeBindCmd/nodeUnbindCmd/nodeBindingsCmd/nodeRmCmd`; `cmd/flow` helpers `resolveSlug(ctx,c,slug)(string,error)` (now over `ListNodes`), `clientFromStore`, `projectresolve.Resolve(ctx,c,getenv,cwd)(domain.Node,bool,error)`.
> **Composition root:** `cmd/flow-server/main.go` — `nodeStore := pgstore.NewNodeStore(pool)` (≈L66), `bindingStore` (≈L67), `srv := &httpserver.Server{…}` (≈L91).
> **CreateNode usecase signature consumed by this slice** (natural extension of the old `CreateProject.Execute(ctx,ownerID,name,slug,color,glyph)`): `Execute(ctx, ownerID, name, slug string, kind domain.NodeKind, parentID *string, color, glyph string) (domain.Node, error)`. The wiring task (C8) verifies this against the real A/B signature; reconcile there if A chose a struct-input form.

---

### Task C1: SSE `node.moved` event + bus carry

**Files**
- `internal/domain/event.go` (add const)
- `internal/domain/event_test.go` (new; package `domain_test`)
- `internal/adapter/sse/bus_moved_test.go` (new; package `sse_test`)

**Interfaces**
- Consumes (A): `domain.EventNodeCreated/Updated/Deleted`, `domain.Event`, `ports.EventBus`/`sse.NewBus()`.
- Produces: `domain.EventNodeMoved EventType = "node.moved"` (carried by the existing type-agnostic bus; no bus code change).

**Steps**
- [ ] **Step 1 — Failing test (const value + bus delivery).** Create `internal/domain/event_test.go`:
```go
package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestEventNodeMovedValue(t *testing.T) {
	t.Parallel()
	if got := domain.EventNodeMoved; got != "node.moved" {
		t.Fatalf("EventNodeMoved = %q, want %q", got, "node.moved")
	}
}
```
Create `internal/adapter/sse/bus_moved_test.go`:
```go
package sse_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
)

func TestBusCarriesNodeMoved(t *testing.T) {
	t.Parallel()
	b := sse.NewBus()
	ch, cancel := b.Subscribe("u1")
	defer cancel()

	b.Publish(domain.Event{Type: domain.EventNodeMoved, UserID: "u1", Data: map[string]any{"id": "n1", "parentId": "p1"}})
	select {
	case ev := <-ch:
		if ev.Type != domain.EventNodeMoved || ev.Data["id"] != "n1" {
			t.Fatalf("got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/domain/ ./internal/adapter/sse/` → undefined `domain.EventNodeMoved`.
- [ ] **Step 3 — Implement.** In `internal/domain/event.go`, add to the const block next to the other node events:
```go
	EventNodeMoved EventType = "node.moved"
```
- [ ] **Step 4 — Run, expect PASS.** `go test ./internal/domain/ ./internal/adapter/sse/`.
- [ ] **Step 5 — Commit.** `feat(domain): add node.moved SSE event type`.

---

### Task C2: REST `POST /nodes/{id}/move` + delete-with-children → 409

**Files**
- `internal/adapter/httpserver/nodemove.go` (new — `handleMoveNode`)
- `internal/adapter/httpserver/worktime.go` (edit `handleDeleteNode` error mapping)
- `internal/adapter/httpserver/server.go` (add `MoveNode` field + route)
- `internal/adapter/httpserver/nodemove_test.go` (new; package `httpserver_test`)

**Interfaces**
- Consumes: `usecase.MoveNode.Execute(ctx, ownerID, id string, newParentID *string)(domain.Node,error)`, `usecase.ErrNodeCycle`, `domain.ErrInvalidNode`, `ports.ErrNodeNotFound`, `ports.ErrNodeHasChildren`; helpers `writeJSON`, `userFrom`, `s.auth`, `s.Bus.Publish`; `domain.EventNodeMoved` (C1); `testutil.NewFakeNodeStore()`.
- Produces: route `POST /api/v1/nodes/{id}/move` (body `{"parentId": string|null}`); `Server.MoveNode usecase.MoveNode`; `handleDeleteNode` now maps `ErrNodeHasChildren`→409.

**Steps**
- [ ] **Step 1 — Failing test.** Create `internal/adapter/httpserver/nodemove_test.go`. The helper seeds the fake `NodeStore` directly using the owner id learned from `GET /me`, so it is independent of the create route:
```go
package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newNodesSrv builds a Server over fake stores and returns the do() helper, the
// fake NodeStore (for direct seeding) and the authenticated owner id.
func newNodesSrv(t *testing.T) (do func(method, path, body string) *http.Response, ns *testutil.FakeNodeStore, ownerID string) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns = testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	users := testutil.NewFakeUserStore()

	srv := &httpserver.Server{
		Verifier:   testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:     usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:        sse.NewBus(),
		Clock:      clk,
		ListNodes:  usecase.ListNodes{Nodes: ns},
		GetNode:    usecase.GetNode{Nodes: ns},
		DeleteNode: usecase.DeleteNode{Nodes: ns},
		MoveNode:   usecase.MoveNode{Nodes: ns},
		NodeAncestors:     usecase.NodeAncestors{Nodes: ns},
		ResolveNode:       usecase.ResolveNode{Bindings: bs, Nodes: ns},
		ResolveEngagement: usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bs, Nodes: ns}, Nodes: ns},
		ListNodeBindings:  usecase.ListNodeBindings{Bindings: bs},
		BindNode:          usecase.BindNode{Bindings: bs, Nodes: ns, IDs: ids, Clock: clk},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	do = func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := do("GET", "/api/v1/me", "")
	var u domain.User
	_ = json.NewDecoder(res.Body).Decode(&u)
	_ = res.Body.Close()
	return do, ns, u.ID
}

// seed inserts a node owned by ownerID straight into the fake store.
func seedNode(t *testing.T, ns *testutil.FakeNodeStore, owner, id string, kind domain.NodeKind, parent *string) {
	t.Helper()
	if _, err := ns.Create(context.Background(), domain.Node{
		ID: id, OwnerID: owner, Kind: kind, ParentID: parent,
		Name: id, Slug: id, Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func ptr(s string) *string { return &s }

func TestMoveNode_OK(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "eng2", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("POST", "/api/v1/nodes/repo1/move", `{"parentId":"eng2"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("move status %d, want 200", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	if n.ParentID == nil || *n.ParentID != "eng2" {
		t.Fatalf("parent = %v, want eng2", n.ParentID)
	}
}

func TestMoveNode_Cycle409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "vorA", domain.KindVorhaben, ptr("eng1"))
	seedNode(t, ns, owner, "vorB", domain.KindVorhaben, ptr("vorA")) // vorB descends from vorA

	// Moving vorA under its own descendant vorB → cycle. (kind vorhaben→vorhaben is allowed.)
	res := do("POST", "/api/v1/nodes/vorA/move", `{"parentId":"vorB"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("cycle status %d, want 409", res.StatusCode)
	}
}

func TestMoveNode_InvalidKind400(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "eng2", domain.KindEngagement, nil)

	// An engagement may never have a parent → ErrInvalidNode.
	res := do("POST", "/api/v1/nodes/eng1/move", `{"parentId":"eng2"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid-kind status %d, want 400", res.StatusCode)
	}
}

func TestMoveNode_NotFound404(t *testing.T) {
	do, _, _ := newNodesSrv(t)
	res := do("POST", "/api/v1/nodes/ghost/move", `{"parentId":null}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("not-found status %d, want 404", res.StatusCode)
	}
}

func TestDeleteNode_WithChildren409(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("DELETE", "/api/v1/nodes/eng1", "") // has a child → RESTRICT
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("delete-with-children status %d, want 409", res.StatusCode)
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/adapter/httpserver/ -run 'MoveNode|DeleteNode_WithChildren'` → `Server` has no field `MoveNode`/`NodeAncestors`/`ResolveEngagement`, route missing. (If `ResolveEngagement`/`NodeAncestors`/`ListNodeBindings`/`BindNode` field names differ, this also pins them for C3/C7.)
- [ ] **Step 3 — Implement handler.** Create `internal/adapter/httpserver/nodemove.go`:
```go
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type moveNodeReq struct {
	ParentID *string `json:"parentId"` // null/absent = make root
}

// handleMoveNode handles POST /api/v1/nodes/{id}/move.
// Body: {"parentId": string|null}. Reparents (cycle-free, kind-checked).
func (s *Server) handleMoveNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req moveNodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n, err := s.MoveNode.Execute(r.Context(), u.ID, r.PathValue("id"), req.ParentID)
	switch {
	case errors.Is(err, usecase.ErrNodeCycle):
		http.Error(w, "move would create a cycle", http.StatusConflict)
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, "invalid parent for this node kind", http.StatusBadRequest)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Bus.Publish(domain.Event{Type: domain.EventNodeMoved, UserID: u.ID, Data: map[string]any{"id": n.ID, "parentId": n.ParentID}})
		writeJSON(w, http.StatusOK, n)
	}
}
```
- [ ] **Step 4 — Implement delete-409 mapping.** In `internal/adapter/httpserver/worktime.go`, replace the `handleDeleteNode` switch (the renamed `handleDeleteProject`) to add the children branch:
```go
	switch err := s.DeleteNode.Execute(r.Context(), u.ID, id); {
	case errors.Is(err, ports.ErrNodeHasChildren):
		http.Error(w, "node has children; move or remove them first", http.StatusConflict)
		return
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
```
- [ ] **Step 5 — Wire field + route.** In `server.go`, add to the struct (next to the other node usecases):
```go
	MoveNode usecase.MoveNode
```
and register the route (place with the node routes, before/after the bindings block; Go 1.22 mux resolves by specificity so order is irrelevant):
```go
	mux.Handle("POST /api/v1/nodes/{id}/move", s.auth(http.HandlerFunc(s.handleMoveNode)))
```
- [ ] **Step 6 — Run, expect PASS.** `go test ./internal/adapter/httpserver/ -run 'MoveNode|DeleteNode_WithChildren'`.
- [ ] **Step 7 — Commit.** `feat(httpserver): POST /nodes/{id}/move + delete-with-children 409`.

---

### Task C3: REST create kind/parent + `GET /nodes/{id}/ancestors` + `GET /nodes/resolve-engagement` + list shape

**Files**
- `internal/usecase/node_ancestors.go` (new — thin read usecase)
- `internal/usecase/node_ancestors_test.go` (new; package `usecase_test`)
- `internal/adapter/httpserver/nodes_extra.go` (new — `handleNodeAncestors`, `handleResolveEngagement`)
- `internal/adapter/httpserver/worktime.go` (edit `handleCreateNode` to read `kind`/`parentId`)
- `internal/adapter/httpserver/server.go` (add `NodeAncestors`, `ResolveEngagement` fields + 2 routes)
- `internal/adapter/httpserver/nodes_extra_test.go` (new; package `httpserver_test`)

**Interfaces**
- Consumes: `ports.NodeStore.Ancestors`, `usecase.CreateNode.Execute(ctx, ownerID, in usecase.CreateNodeInput)` (struct input — see reconciliation note #1), `usecase.UpdateNode`+`usecase.UpdateNodeInput`, `usecase.ResolveEngagement.Execute(ctx,ownerID,remoteSlug,machineID,cwd)(domain.Node,bool,error)`, `domain.NormalizeRemoteSlug`, `domain.ErrInvalidNode`, `ports.ErrNodeNotFound`, `domain.EventNodeCreated`.
- Produces: `usecase.NodeAncestors{Nodes ports.NodeStore}` with `Execute(ctx,ownerID,id)([]domain.Node,error)`; routes `GET /api/v1/nodes/{id}/ancestors`, `GET /api/v1/nodes/resolve-engagement`; `Server.NodeAncestors`, `Server.ResolveEngagement`; `createNodeReq` now carries `Kind`/`ParentID`.

**Steps**
- [ ] **Step 1 — Failing test (usecase).** `internal/usecase/node_ancestors_test.go`:
```go
package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNodeAncestors(t *testing.T) {
	t.Parallel()
	ns := testutil.NewFakeNodeStore()
	ctx := context.Background()
	p := func(s string) *string { return &s }
	_, _ = ns.Create(ctx, domain.Node{ID: "eng", OwnerID: "u", Kind: domain.KindEngagement})
	_, _ = ns.Create(ctx, domain.Node{ID: "repo", OwnerID: "u", Kind: domain.KindRepo, ParentID: p("eng")})

	chain, err := usecase.NodeAncestors{Nodes: ns}.Execute(ctx, "u", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != "repo" || chain[1].ID != "eng" {
		t.Fatalf("chain = %+v, want [repo eng] (leaf→root)", chain)
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/usecase/ -run NodeAncestors`.
- [ ] **Step 3 — Implement usecase.** `internal/usecase/node_ancestors.go`:
```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeAncestors returns a node and its ancestors ordered leaf→root.
type NodeAncestors struct {
	Nodes ports.NodeStore
}

func (uc NodeAncestors) Execute(ctx context.Context, ownerID, id string) ([]domain.Node, error) {
	return uc.Nodes.Ancestors(ctx, ownerID, id)
}
```
- [ ] **Step 4 — Run, expect PASS.** `go test ./internal/usecase/ -run NodeAncestors`.
- [ ] **Step 5 — Failing test (REST).** `internal/adapter/httpserver/nodes_extra_test.go` (reuses `newNodesSrv`/`seedNode`/`ptr` from C2):
```go
package httpserver_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestListNodes_ReturnsParentAndKind(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("GET", "/api/v1/nodes", "")
	defer res.Body.Close()
	var raw []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&raw)
	var repo map[string]any
	for _, n := range raw {
		if n["id"] == "repo1" {
			repo = n
		}
	}
	if repo == nil || repo["kind"] != "repo" || repo["parentId"] != "eng1" {
		t.Fatalf("flat list missing kind/parentId: %+v", repo)
	}
}

func TestNodeAncestorsRoute(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))

	res := do("GET", "/api/v1/nodes/repo1/ancestors", "")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ancestors status %d, want 200", res.StatusCode)
	}
	var chain []domain.Node
	_ = json.NewDecoder(res.Body).Decode(&chain)
	if len(chain) != 2 || chain[0].ID != "repo1" || chain[1].ID != "eng1" {
		t.Fatalf("ancestors = %+v, want [repo1 eng1]", chain)
	}
}

func TestCreateNode_WithKindAndParent(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)

	res := do("POST", "/api/v1/nodes", `{"name":"flow","kind":"repo","parentId":"eng1"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	if n.Kind != domain.KindRepo || n.ParentID == nil || *n.ParentID != "eng1" {
		t.Fatalf("created node = %+v", n)
	}
}

func TestCreateNode_InvalidKind400(t *testing.T) {
	do, _, _ := newNodesSrv(t)
	// repo with no parent → root-is-engagement violation → ErrInvalidNode.
	res := do("POST", "/api/v1/nodes", `{"name":"x","kind":"repo"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestResolveEngagement_Route(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil)
	seedNode(t, ns, owner, "repo1", domain.KindRepo, ptr("eng1"))
	// bind remote → repo, then resolve-engagement by that slug → eng1.
	_ = do("PUT", "/api/v1/nodes/repo1/bindings", `{"kind":"remote","remoteSlug":"github.com/sk/flow"}`).Body.Close()

	res := do("GET", "/api/v1/nodes/resolve-engagement?slug=github.com%2Fsk%2Fflow", "")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve-engagement status %d, want 200", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	if n.ID != "eng1" {
		t.Fatalf("resolved engagement = %q, want eng1", n.ID)
	}
}
```
- [ ] **Step 6 — Run, expect FAIL.** `go test ./internal/adapter/httpserver/ -run 'ListNodes_Return|NodeAncestorsRoute|CreateNode_With|CreateNode_Invalid|ResolveEngagement_Route'`.
- [ ] **Step 7 — Implement extra handlers.** `internal/adapter/httpserver/nodes_extra.go`:
```go
package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// handleNodeAncestors handles GET /api/v1/nodes/{id}/ancestors (leaf→root).
func (s *Server) handleNodeAncestors(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	chain, err := s.NodeAncestors.Execute(r.Context(), u.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		if chain == nil {
			chain = []domain.Node{}
		}
		writeJSON(w, http.StatusOK, chain)
	}
}

// handleResolveEngagement handles GET /api/v1/nodes/resolve-engagement.
// Query: ?slug=&machine=&path= (same as /resolve). Returns 200 engagement | 404.
func (s *Server) handleResolveEngagement(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	n, ok, err := s.ResolveEngagement.Execute(r.Context(), u.ID, q.Get("slug"), q.Get("machine"), q.Get("path"))
	switch {
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	case !ok:
		http.Error(w, "not found", http.StatusNotFound)
	default:
		writeJSON(w, http.StatusOK, n)
	}
}
```
- [ ] **Step 8 — Extend create handler.** In `worktime.go`, replace `createNodeReq` (the renamed `createProjReq`) and the kind/parent part of `handleCreateNode`:
```go
type createNodeReq struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Kind        string  `json:"kind"`
	ParentID    *string `json:"parentId"`
	Color       string  `json:"color"`
	Glyph       string  `json:"glyph"`
	Description string  `json:"description"`
	UpstreamGit string  `json:"upstreamGit"`
}
```
and the handler body (keep the existing upstream pre-check + description/upstream UpdateNode follow-up; only the create call and error mapping change):
```go
func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req createNodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Kind == "" {
		http.Error(w, "kind required", http.StatusBadRequest)
		return
	}
	if req.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(req.UpstreamGit); !ok {
			http.Error(w, "invalid upstream git url", http.StatusBadRequest)
			return
		}
	}
	p, err := s.CreateNode.Execute(r.Context(), u.ID, req.Name, req.Slug, domain.NodeKind(req.Kind), req.ParentID, req.Color, req.Glyph)
	switch {
	case errors.Is(err, domain.ErrInvalidNode):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, ports.ErrNodeNotFound): // parent referenced but absent
		http.Error(w, "parent not found", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if req.Description != "" || req.UpstreamGit != "" {
		p, err = s.UpdateNode.Execute(r.Context(), u.ID, p.ID, usecase.UpdateNodeInput{
			Name: p.Name, Slug: p.Slug, Color: p.Color, Glyph: p.Glyph,
			Description: req.Description, UpstreamGit: req.UpstreamGit, Status: p.Status,
		})
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID}})
	writeJSON(w, http.StatusCreated, p)
}
```
- [ ] **Step 9 — Wire fields + routes.** In `server.go` add to the struct:
```go
	NodeAncestors     usecase.NodeAncestors
	ResolveEngagement usecase.ResolveEngagement
```
and routes (with the node routes):
```go
	mux.Handle("GET /api/v1/nodes/{id}/ancestors", s.auth(http.HandlerFunc(s.handleNodeAncestors)))
	mux.Handle("GET /api/v1/nodes/resolve-engagement", s.auth(http.HandlerFunc(s.handleResolveEngagement)))
```
- [ ] **Step 10 — Run, expect PASS.** `go test ./internal/usecase/ ./internal/adapter/httpserver/`.
- [ ] **Step 11 — Commit.** `feat(httpserver): node create kind/parent, ancestors + resolve-engagement routes`.

---

### Task C4: apiclient — `MoveNode`, `Ancestors`, `ResolveEngagement`, rich `CreateNode`

**Files**
- `internal/adapter/apiclient/nodes.go` (new — `MoveNode`, `Ancestors`, `CreateNodeFields`+`CreateNode`)
- `internal/adapter/apiclient/nodebindings.go` (edit — add `ResolveEngagement`; this is the renamed `projectbindings.go` holding `ResolveNode`)
- `internal/adapter/apiclient/nodes_test.go` (new; package `apiclient_test` — match existing httptest style)
- `cmd/flow/nodebind.go` (edit the 2 picker create call-sites for the new `CreateNode` shape)

**Interfaces**
- Consumes: `c.do`, `APIError`, routes from C2/C3, `domain.Node`.
- Produces: `Client.MoveNode(ctx, id string, parentID *string)(domain.Node,error)`; `Client.Ancestors(ctx, id string)([]domain.Node,error)`; `Client.ResolveEngagement(ctx, remoteSlug, machineID, cwd string)(domain.Node,bool,error)`; `apiclient.CreateNodeFields` + `Client.CreateNode(ctx, in CreateNodeFields)(domain.Node,error)` (replaces the A `CreateNode(ctx,name)`).

**Steps**
- [ ] **Step 1 — Failing test.** `internal/adapter/apiclient/nodes_test.go`:
```go
package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestMoveNode_PostsParent(t *testing.T) {
	t.Parallel()
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/n1/move" {
			b, _ := json.Marshal(map[string]any{})
			var raw map[string]any
			_ = json.NewDecoder(r.Body).Decode(&raw)
			gotBody, _ = raw["parentId"].(string)
			_ = b
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", ParentID: func() *string { s := "p2"; return &s }()})
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	p := "p2"
	n, err := c.MoveNode(context.Background(), "n1", &p)
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != "p2" || n.ParentID == nil || *n.ParentID != "p2" {
		t.Fatalf("MoveNode body=%q result=%+v", gotBody, n)
	}
}

func TestAncestors_Decodes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/nodes/repo1/ancestors" {
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "repo1"}, {ID: "eng1"}})
			return
		}
		t.Errorf("unexpected %s", r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	chain, err := c.Ancestors(context.Background(), "repo1")
	if err != nil || len(chain) != 2 || chain[1].ID != "eng1" {
		t.Fatalf("Ancestors = %+v err=%v", chain, err)
	}
}

func TestResolveEngagement_404IsNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	_, ok, err := c.ResolveEngagement(context.Background(), "github.com/x/y", "m1", "/tmp")
	if err != nil || ok {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestCreateNode_PostsFields(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes" {
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Kind: domain.KindRepo})
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	p := "eng1"
	n, err := c.CreateNode(context.Background(), apiclient.CreateNodeFields{Name: "flow", Kind: "repo", ParentID: &p})
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "repo" || got["parentId"] != "eng1" || n.Kind != domain.KindRepo {
		t.Fatalf("CreateNode body=%v result=%+v", got, n)
	}
	_ = strings.TrimSpace
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/adapter/apiclient/ -run 'MoveNode|Ancestors|ResolveEngagement|CreateNode_PostsFields'`.
- [ ] **Step 3 — Implement `nodes.go`.** `internal/adapter/apiclient/nodes.go`:
```go
package apiclient

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

// CreateNodeFields are the inputs for creating a node. JSON tags match the
// server's createNodeReq.
type CreateNodeFields struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Kind        string  `json:"kind"`
	ParentID    *string `json:"parentId"`
	Color       string  `json:"color"`
	Glyph       string  `json:"glyph"`
	Description string  `json:"description"`
	UpstreamGit string  `json:"upstreamGit"`
}

// CreateNode creates a node (engagement, vorhaben or repo).
func (c *Client) CreateNode(ctx context.Context, in CreateNodeFields) (domain.Node, error) {
	var n domain.Node
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes", in, &n)
	return n, err
}

// MoveNode reparents a node. parentID nil → make it a root (engagement).
func (c *Client) MoveNode(ctx context.Context, id string, parentID *string) (domain.Node, error) {
	var n domain.Node
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/"+id+"/move",
		map[string]any{"parentId": parentID}, &n)
	return n, err
}

// Ancestors returns the node and its ancestors ordered leaf→root.
func (c *Client) Ancestors(ctx context.Context, id string) ([]domain.Node, error) {
	var out []domain.Node
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id+"/ancestors", nil, &out)
	return out, err
}
```
- [ ] **Step 4 — Implement `ResolveEngagement`.** In `internal/adapter/apiclient/nodebindings.go`, mirror `ResolveNode`:
```go
// ResolveEngagement calls GET /api/v1/nodes/resolve-engagement and returns the
// engagement for the resolved repo. 404 → ok=false, err=nil.
func (c *Client) ResolveEngagement(ctx context.Context, remoteSlug, machineID, cwd string) (domain.Node, bool, error) {
	path := "/api/v1/nodes/resolve-engagement?slug=" + url.QueryEscape(remoteSlug) +
		"&machine=" + url.QueryEscape(machineID) +
		"&path=" + url.QueryEscape(cwd)
	var n domain.Node
	err := c.do(ctx, http.MethodGet, path, nil, &n)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound {
			return domain.Node{}, false, nil
		}
		return domain.Node{}, false, err
	}
	return n, true, nil
}
```
- [ ] **Step 5 — Fix the old simple `CreateNode` callers.** In `cmd/flow/nodebind.go` (renamed `projectbind.go`), the picker create calls `c.CreateNode(ctx, picked.Label)` (2 sites: `bindSelection`, `runBindPathInteractive`). Change each to the struct form, creating a repo:
```go
		p, err := c.CreateNode(ctx, apiclient.CreateNodeFields{Name: picked.Label, Kind: string(domain.KindRepo)})
```
> NOTE: the bind-picker creating a parent-less repo is a Slice-B/UI semantic (engagement-parent selection); this edit only keeps it compiling. If Slice B already reshaped the picker’s create call, keep B’s version and skip this edit. The C8 wiring gate (`make ci`) catches any leftover `CreateNode(ctx, string)` callers.
- [ ] **Step 6 — Run, expect PASS.** `go test ./internal/adapter/apiclient/ ./cmd/flow/`.
- [ ] **Step 7 — Commit.** `feat(apiclient): MoveNode, Ancestors, ResolveEngagement, rich CreateNode`.

---

### Task C5: CLI pure tree builder + indented renderer

**Files**
- `cmd/flow/nodetree.go` (new)
- `cmd/flow/nodetree_test.go` (new; package `main`)

**Interfaces**
- Consumes: `domain.Node` (`ID`, `ParentID`, `Name`, `Slug`, `Kind`).
- Produces: `type nodeTree struct{ Node domain.Node; Children []*nodeTree }`; `func buildTree(nodes []domain.Node) []*nodeTree`; `func renderTree(roots []*nodeTree, w io.Writer)`.

**Steps**
- [ ] **Step 1 — Failing test.** `cmd/flow/nodetree_test.go`:
```go
package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func p(s string) *string { return &s }

func TestBuildTree_ForestAndOrder(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "eng1", Name: "Privat", Kind: domain.KindEngagement},
		{ID: "repoB", Name: "beta", Kind: domain.KindRepo, ParentID: p("eng1")},
		{ID: "repoA", Name: "alpha", Kind: domain.KindRepo, ParentID: p("eng1")},
		{ID: "eng0", Name: "AAA", Kind: domain.KindEngagement},
		{ID: "orphan", Name: "ghost", Kind: domain.KindRepo, ParentID: p("missing")}, // dangling → treated as root
	}
	roots := buildTree(nodes)
	// Roots are name-sorted: AAA, Privat, then the dangling orphan (parent absent).
	if len(roots) != 3 {
		t.Fatalf("roots = %d, want 3", len(roots))
	}
	if roots[0].Node.Name != "AAA" || roots[1].Node.Name != "Privat" {
		t.Fatalf("root order = %q,%q", roots[0].Node.Name, roots[1].Node.Name)
	}
	// Children name-sorted under Privat: alpha, beta.
	priv := roots[1]
	if len(priv.Children) != 2 || priv.Children[0].Node.Name != "alpha" || priv.Children[1].Node.Name != "beta" {
		t.Fatalf("Privat children = %+v", priv.Children)
	}
}

func TestRenderTree_Indents(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement},
		{ID: "repo1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: p("eng1")},
	}
	var sb strings.Builder
	renderTree(buildTree(nodes), &sb)
	out := sb.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d:\n%s", len(lines), out)
	}
	if strings.HasPrefix(lines[0], " ") {
		t.Errorf("root line should not be indented: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") || !strings.Contains(lines[1], "flow") {
		t.Errorf("child line should be indented and name the repo: %q", lines[1])
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./cmd/flow/ -run 'BuildTree|RenderTree'`.
- [ ] **Step 3 — Implement.** `cmd/flow/nodetree.go`:
```go
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// nodeTree is a node plus its (name-sorted) children.
type nodeTree struct {
	Node     domain.Node
	Children []*nodeTree
}

// buildTree groups a flat node list into a parent→children forest. A node whose
// ParentID is nil or points to an absent node is treated as a root. Roots and
// each child level are sorted by name (case-insensitive).
func buildTree(nodes []domain.Node) []*nodeTree {
	byID := make(map[string]*nodeTree, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodeTree{Node: nodes[i]}
	}
	var roots []*nodeTree
	for _, t := range byID {
		pid := t.Node.ParentID
		if pid == nil {
			roots = append(roots, t)
			continue
		}
		if parent, ok := byID[*pid]; ok {
			parent.Children = append(parent.Children, t)
		} else {
			roots = append(roots, t) // dangling parent → surface as root
		}
	}
	var sortRec func(ts []*nodeTree)
	sortRec = func(ts []*nodeTree) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(ts[i].Node.Name) < strings.ToLower(ts[j].Node.Name)
		})
		for _, t := range ts {
			sortRec(t.Children)
		}
	}
	sortRec(roots)
	return roots
}

// renderTree writes the forest indented two spaces per depth level.
func renderTree(roots []*nodeTree, w io.Writer) {
	var walk func(ts []*nodeTree, depth int)
	walk = func(ts []*nodeTree, depth int) {
		for _, t := range ts {
			fmt.Fprintf(w, "%s%s  %s (%s)\n", strings.Repeat("  ", depth), t.Node.Kind, t.Node.Name, t.Node.Slug)
			walk(t.Children, depth+1)
		}
	}
	walk(roots, 0)
}
```
- [ ] **Step 4 — Run, expect PASS.** `go test ./cmd/flow/ -run 'BuildTree|RenderTree'`.
- [ ] **Step 5 — Commit.** `feat(cli): pure node tree builder + indented renderer`.

---

### Task C6: CLI `flow node create | list | show | move`

**Files**
- `cmd/flow/node.go` (edit — register new subcommands on `nodeCmd()`)
- `cmd/flow/node_subcommands.go` (new — `nodeCreateCmd/nodeListCmd/nodeShowCmd/nodeMoveCmd`)
- `cmd/flow/node_subcommands_test.go` (new; package `main`)

**Interfaces**
- Consumes: `clientFromStore`, `resolveSlug`, `projectresolve.Resolve(ctx,c,getenv,cwd)(domain.Node,bool,error)`, `apiclient.{CreateNodeFields, CreateNode, ListNodes, MoveNode, Ancestors, IsConflict}`, `buildTree`/`renderTree` (C5), `os.Getwd`, `os.Getenv`, cobra.
- Produces: `flow node create <name> --kind --parent --color --glyph --desc --upstream`; `flow node list [--tree] [--kind] [--status]`; `flow node show [<slug>]`; `flow node move <slug> --parent <slug>`.

**Steps**
- [ ] **Step 1 — Failing test.** `cmd/flow/node_subcommands_test.go` (cobra cmd-level happy-path + validation surfacing; httptest backend, `SetArgs`+`SetOut`):
```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// runNodeList/Create/Move are the testable cores (no clientFromStore/env).

func TestRunNodeCreate_PostsKindAndParent(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "eng1", Slug: "privat", Kind: domain.KindEngagement}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Name: "flow", Slug: "flow", Kind: domain.KindRepo})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeCreate(context.Background(), c, &out, "flow", "repo", "privat", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "repo" || got["parentId"] != "eng1" {
		t.Fatalf("posted = %v", got)
	}
	if !strings.Contains(out.String(), "flow") {
		t.Errorf("output missing node name: %q", out.String())
	}
}

func TestRunNodeList_Tree(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, true, "", "all"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 || strings.HasPrefix(lines[0], " ") || !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("tree output:\n%s", out.String())
	}
}

func TestRunNodeList_KindFilter(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := "eng1"
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "eng1", Name: "Privat", Slug: "privat", Kind: domain.KindEngagement, Status: domain.NodeActive},
			{ID: "r1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, ParentID: &pid, Status: domain.NodeActive},
		})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeList(context.Background(), c, &out, false, "engagement", "all"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "flow") || !strings.Contains(out.String(), "privat") {
		t.Fatalf("kind filter wrong:\n%s", out.String())
	}
}

func TestRunNodeMove_CycleSurfaced(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{
				{ID: "a", Slug: "a"}, {ID: "b", Slug: "b"},
			})
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/move"):
			http.Error(w, "move would create a cycle", http.StatusConflict)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	err := runNodeMove(context.Background(), c, &out, "a", "b")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./cmd/flow/ -run 'RunNode(Create|List|Move)'`.
- [ ] **Step 3 — Implement cores + cobra.** `cmd/flow/node_subcommands.go`:
```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/projectresolve"
	"github.com/spf13/cobra"
)

// --- testable cores ---

func runNodeCreate(ctx context.Context, c *apiclient.Client, w io.Writer, name, kind, parentSlug, color, glyph, desc, upstream string) error {
	var parentID *string
	if parentSlug != "" {
		id, err := resolveSlug(ctx, c, parentSlug)
		if err != nil {
			return err
		}
		parentID = &id
	}
	n, err := c.CreateNode(ctx, apiclient.CreateNodeFields{
		Name: name, Kind: kind, ParentID: parentID,
		Color: color, Glyph: glyph, Description: desc, UpstreamGit: upstream,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "created %s %s (%s)\n", n.Kind, n.Name, n.Slug)
	return nil
}

// statusWanted reports whether n passes the --status filter (""/"all" → any).
func statusWanted(n domain.Node, status string) bool {
	if status == "" || status == "all" {
		return true
	}
	return string(n.Status) == status
}

func runNodeList(ctx context.Context, c *apiclient.Client, w io.Writer, tree bool, kind, status string) error {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return err
	}
	filtered := make([]domain.Node, 0, len(nodes))
	for _, n := range nodes {
		if kind != "" && string(n.Kind) != kind {
			continue
		}
		if !statusWanted(n, status) {
			continue
		}
		filtered = append(filtered, n)
	}
	if tree {
		renderTree(buildTree(filtered), w)
		return nil
	}
	for _, n := range filtered {
		_, _ = fmt.Fprintf(w, "%-11s %-24s %s\n", n.Kind, n.Slug, n.Name)
	}
	return nil
}

func runNodeMove(ctx context.Context, c *apiclient.Client, w io.Writer, slug, parentSlug string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	var parentID *string
	if parentSlug != "" {
		pid, err := resolveSlug(ctx, c, parentSlug)
		if err != nil {
			return err
		}
		parentID = &pid
	}
	if _, err := c.MoveNode(ctx, id, parentID); err != nil {
		if apiclient.IsConflict(err) {
			return fmt.Errorf("cannot move %s: it would create a cycle", slug)
		}
		return fmt.Errorf("move %s: %w", slug, err)
	}
	dest := "root"
	if parentSlug != "" {
		dest = parentSlug
	}
	_, _ = fmt.Fprintf(w, "moved %s → %s\n", slug, dest)
	return nil
}

// runNodeShow renders a node's detail + leaf→root breadcrumb. If slug is empty,
// the node is resolved from cwd (git origin → repo).
func runNodeShow(ctx context.Context, c *apiclient.Client, w io.Writer, getenv func(string) string, cwd, slug string) error {
	var node domain.Node
	if slug != "" {
		id, err := resolveSlug(ctx, c, slug)
		if err != nil {
			return err
		}
		if node, err = c.GetNode(ctx, id); err != nil {
			return err
		}
	} else {
		n, ok, err := projectresolve.Resolve(ctx, c, getenv, cwd)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no node bound to %s (pass a slug or bind this directory)", cwd)
		}
		node = n
	}
	chain, err := c.Ancestors(ctx, node.ID)
	if err != nil {
		return err
	}
	// chain is leaf→root; print breadcrumb root→leaf.
	crumbs := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		crumbs = append(crumbs, chain[i].Name)
	}
	_, _ = fmt.Fprintf(w, "%s %s (%s)\n", node.Kind, node.Name, node.Slug)
	_, _ = fmt.Fprintf(w, "status: %s\n", node.Status)
	if node.UpstreamGit != "" {
		_, _ = fmt.Fprintf(w, "upstream: %s\n", node.UpstreamGit)
	}
	if len(crumbs) > 0 {
		_, _ = fmt.Fprintf(w, "path: %s\n", join(crumbs, " / "))
	}
	return nil
}

func join(ss []string, sep string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}

// --- cobra wrappers ---

func nodeCreateCmd() *cobra.Command {
	var kind, parent, color, glyph, desc, upstream string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "create a node (engagement, vorhaben or repo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeCreate(cmd.Context(), c, cmd.OutOrStdout(), args[0], kind, parent, color, glyph, desc, upstream)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "engagement|vorhaben|repo (required)")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node slug (omit for an engagement root)")
	cmd.Flags().StringVar(&color, "color", "", "identity color name")
	cmd.Flags().StringVar(&glyph, "glyph", "", "identity glyph")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	cmd.Flags().StringVar(&upstream, "upstream", "", "git clone URL (repo only)")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func nodeListCmd() *cobra.Command {
	var tree bool
	var kind, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list nodes (flat, or --tree for the hierarchy)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeList(cmd.Context(), c, cmd.OutOrStdout(), tree, kind, status)
		},
	}
	cmd.Flags().BoolVar(&tree, "tree", false, "render the hierarchy indented")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind (engagement|vorhaben|repo)")
	cmd.Flags().StringVar(&status, "status", "all", "active|paused|archived|all")
	return cmd
}

func nodeShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [<slug>]",
		Short: "show a node and its ancestor path (default: cwd-resolved repo)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			cwd, _ := os.Getwd()
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			return runNodeShow(cmd.Context(), c, cmd.OutOrStdout(), os.Getenv, cwd, slug)
		},
	}
	return cmd
}

func nodeMoveCmd() *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "move <slug>",
		Short: "reparent a node (cycle-free); --parent \"\" moves it to a root",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeMove(cmd.Context(), c, cmd.OutOrStdout(), args[0], parent)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "new parent node slug (empty = root)")
	return cmd
}
```
- [ ] **Step 4 — Register.** In `cmd/flow/node.go` (renamed `project.go`), add to `nodeCmd()`:
```go
	cmd.AddCommand(nodeCreateCmd())
	cmd.AddCommand(nodeListCmd())
	cmd.AddCommand(nodeShowCmd())
	cmd.AddCommand(nodeMoveCmd())
```
- [ ] **Step 5 — Run, expect PASS.** `go test ./cmd/flow/ -run 'RunNode(Create|List|Move)'`.
- [ ] **Step 6 — Commit.** `feat(cli): flow node create/list/show/move`.

---

### Task C7: CLI bind/unbind/bindings target-kind UX + pause/resume/archive + friendly `rm`

**Files**
- `internal/adapter/httpserver/nodebindings.go` (edit `handleBindNode` — map `usecase.ErrInvalidBindTarget`→400; this is the renamed `projectbindings.go`)
- `internal/adapter/httpserver/nodebindings_test.go` (add a 400 case)
- `cmd/flow/node.go` (register `nodePauseCmd/nodeResumeCmd/nodeArchiveCmd`; friendly `rm`)
- `cmd/flow/node_status.go` (new — status helpers + cobra)
- `cmd/flow/node_status_test.go` (new; package `main`)

**Interfaces**
- Consumes: `usecase.ErrInvalidBindTarget`, `ports.ErrNodeNotFound`, `apiclient.{GetNode, UpdateNode, UpdateNodeFields, DeleteNode, IsConflict}`, `domain.NodeStatus`, `resolveSlug`, `clientFromStore`.
- Produces: bind handler 400 on invalid target kind; `flow node pause|resume|archive <slug>`; `flow node rm` surfaces `ErrNodeHasChildren` as a friendly message.

**Steps**
- [ ] **Step 1 — Failing tests.** Add to `internal/adapter/httpserver/nodebindings_test.go` (uses C2 `newNodesSrv`/`seedNode`/`ptr`):
```go
func TestBindNode_InvalidTargetKind400(t *testing.T) {
	do, ns, owner := newNodesSrv(t)
	seedNode(t, ns, owner, "eng1", domain.KindEngagement, nil) // remote bind onto an engagement → invalid (must be repo)
	res := do("PUT", "/api/v1/nodes/eng1/bindings", `{"kind":"remote","remoteSlug":"github.com/x/y"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid bind target status %d, want 400", res.StatusCode)
	}
}
```
And `cmd/flow/node_status_test.go`:
```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestRunNodeSetStatus(t *testing.T) {
	t.Parallel()
	var patched map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "n1", Slug: "flow"}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes/n1":
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Name: "flow", Slug: "flow", Status: domain.NodeActive})
		case r.Method == "PATCH" && r.URL.Path == "/api/v1/nodes/n1":
			_ = json.NewDecoder(r.Body).Decode(&patched)
			_ = json.NewEncoder(w).Encode(domain.Node{ID: "n1", Slug: "flow", Status: domain.NodePaused})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	var out bytes.Buffer
	if err := runNodeSetStatus(context.Background(), c, &out, "flow", "paused"); err != nil {
		t.Fatal(err)
	}
	if patched["status"] != "paused" {
		t.Fatalf("patched status = %v", patched["status"])
	}
}

func TestRunNodeRm_HasChildrenFriendly(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "n1", Slug: "eng"}})
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/nodes/n1":
			http.Error(w, "node has children", http.StatusConflict)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	err := runNodeRm(context.Background(), c, "eng")
	if err == nil || !strings.Contains(err.Error(), "children") {
		t.Fatalf("want friendly children error, got %v", err)
	}
}
```
- [ ] **Step 2 — Run, expect FAIL.** `go test ./internal/adapter/httpserver/ -run BindNode_InvalidTargetKind && go test ./cmd/flow/ -run 'RunNodeSetStatus|RunNodeRm_HasChildren'`.
- [ ] **Step 3 — Implement bind 400.** In `nodebindings.go`’s `handleBindNode` switch, add a branch (before the generic `err != nil`):
```go
	case errors.Is(err, usecase.ErrInvalidBindTarget):
		http.Error(w, "binding target has the wrong kind (remote→repo, path→repo or leaf vorhaben)", http.StatusBadRequest)
```
- [ ] **Step 4 — Implement status + friendly rm cores.** `cmd/flow/node_status.go`:
```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

// runNodeSetStatus reads the node, then PATCHes it with the new status (full
// replace; rate is untouched by UpdateNode).
func runNodeSetStatus(ctx context.Context, c *apiclient.Client, w io.Writer, slug, status string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	n, err := c.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if _, err := c.UpdateNode(ctx, id, apiclient.UpdateNodeFields{
		Name: n.Name, Slug: n.Slug, Color: n.Color, Glyph: n.Glyph,
		Description: n.Description, UpstreamGit: n.UpstreamGit, Status: status,
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "%s is now %s\n", slug, status)
	return nil
}

func nodeStatusCmd(use, short, status string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <slug>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runNodeSetStatus(cmd.Context(), c, cmd.OutOrStdout(), args[0], status)
		},
	}
}

func nodePauseCmd() *cobra.Command   { return nodeStatusCmd("pause", "pause a node", "paused") }
func nodeResumeCmd() *cobra.Command  { return nodeStatusCmd("resume", "resume a paused node", "active") }
func nodeArchiveCmd() *cobra.Command { return nodeStatusCmd("archive", "archive a node", "archived") }
```
- [ ] **Step 5 — Friendly rm.** In `cmd/flow/node.go`, update `runNodeRm` (renamed `runProjectRm`) to surface 409:
```go
func runNodeRm(ctx context.Context, c *apiclient.Client, slug string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	if err := c.DeleteNode(ctx, id); err != nil {
		if apiclient.IsConflict(err) {
			return fmt.Errorf("cannot delete %s: it has children; move or remove them first", slug)
		}
		return err
	}
	return nil
}
```
and register the new status commands in `nodeCmd()`:
```go
	cmd.AddCommand(nodePauseCmd())
	cmd.AddCommand(nodeResumeCmd())
	cmd.AddCommand(nodeArchiveCmd())
```
(Ensure `nodeRmCmd`’s `RunE` calls `runNodeRm`; keep its confirmation prompt.)
- [ ] **Step 6 — Run, expect PASS.** `go test ./internal/adapter/httpserver/ ./cmd/flow/`.
- [ ] **Step 7 — Commit.** `feat(cli): node pause/resume/archive + friendly rm; bind invalid-target 400`.

---

### Task C8: Compose-root wiring + full-slice verification (curl smoke)

**Files**
- `cmd/flow-server/main.go` (wire `MoveNode`, `NodeAncestors`, `ResolveEngagement` on the `&httpserver.Server{…}` literal)

**Interfaces**
- Consumes: `nodeStore` (≈L66), `bindingStore` (≈L67), the new usecases.
- Produces: a fully-wired server; every new route reachable.

**Steps**
- [ ] **Step 1 — Wire the composition root.** In `cmd/flow-server/main.go`, in the `srv := &httpserver.Server{…}` literal, beside the existing node usecases (where `ResolveNode`/`CreateNode` are assigned), add:
```go
		MoveNode:          usecase.MoveNode{Nodes: nodeStore},
		NodeAncestors:     usecase.NodeAncestors{Nodes: nodeStore},
		ResolveEngagement: usecase.ResolveEngagement{Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore}, Nodes: nodeStore},
```
> Confirm the `CreateNode` field’s usecase value matches the signature this slice consumes (`Execute(ctx,ownerID,name,slug,kind,parentID,color,glyph)`). If A built `CreateNode` with a different shape, reconcile the C3 handler call here.
- [ ] **Step 2 — Verify no other `Server{}`/`ln{` literal drifted.** `rg -n 'httpserver\.Server\{|&ln\{' cmd/ internal/ | rg -v _test` — every non-test constructor must compile with the new required fields (only the compose root is non-test).
- [ ] **Step 3 — Full gate.** Run `make ci` (gofumpt + staticcheck + build + tests + coverage gate). Expect green; fix any QF1002/lint before proceeding (lint, not just `go test`).
- [ ] **Step 4 — Live curl smoke (against dev Postgres+Dex; `make dev-up`, token via `make dev-token`).** Verify each new route end-to-end:
```sh
BASE=https://localhost:8443; TOK=$(make -s dev-token); H="-ksH"
# create an engagement, a repo under it, list as tree, ancestors, move, resolve-engagement
ENG=$(curl $H "Authorization: Bearer $TOK" -X POST $BASE/api/v1/nodes -d '{"name":"Privat","kind":"engagement"}' | jq -r .id)
REPO=$(curl $H "Authorization: Bearer $TOK" -X POST $BASE/api/v1/nodes -d "{\"name\":\"flow\",\"kind\":\"repo\",\"parentId\":\"$ENG\"}" | jq -r .id)
curl $H "Authorization: Bearer $TOK" "$BASE/api/v1/nodes/$REPO/ancestors"          # [repo, engagement]
ENG2=$(curl $H "Authorization: Bearer $TOK" -X POST $BASE/api/v1/nodes -d '{"name":"RTL Extern","kind":"engagement"}' | jq -r .id)
curl $H "Authorization: Bearer $TOK" -X POST $BASE/api/v1/nodes/$REPO/move -d "{\"parentId\":\"$ENG2\"}"   # 200
curl $H "Authorization: Bearer $TOK" -X POST $BASE/api/v1/nodes/$ENG2/move -d "{\"parentId\":\"$REPO\"}"   # 409 cycle
curl -sw '%{http_code}\n' $H "Authorization: Bearer $TOK" -X DELETE $BASE/api/v1/nodes/$ENG2 -o /dev/null   # 409 has children
```
And the CLI surface: `flow node create … --kind …`, `flow node list --tree`, `flow node show flow`, `flow node move flow --parent rtl-extern`, `flow node pause flow`, `flow node rm privat` (friendly children error).
- [ ] **Step 5 — Commit.** `feat(server): wire MoveNode/NodeAncestors/ResolveEngagement; B1 Slice C done`.


---

## Slice D — WebUI (hierarchy tree + node management)

This slice rebuilds the flat project WebUI (`projects.templ` / `webui_projects.go`) into a node-hierarchy UI: an indented Engagement → Vorhaben → Repo tree, a kind-aware create/edit form, a move (reparent) action, a repo cockpit with ancestor breadcrumb + read-only bindings, node-aware doc/worktime lists, and an **engagement** worktime booking selector. It also adds the WebUI `NodeKind → badge` mapping (none exists today; only the TUI `kindcolor` maps `DocumentType`).

**Consumed from Slices A/C (do not redefine — wire these on `*httpserver.Server`):**
- `Nodes ports.NodeStore` — `List / Get / Children(ctx,owner,*parentID) / Ancestors(ctx,owner,nodeID) []domain.Node` (leaf→root).
- `ListNodes usecase.ListNodes` `.Execute(ctx,owner) ([]domain.Node,error)`; `GetNode usecase.GetNode` `.Execute(ctx,owner,id) (domain.Node,error)`.
- `CreateNode usecase.CreateNode` `.Execute(ctx,owner, in usecase.CreateNodeInput) (domain.Node,error)` with `CreateNodeInput{Name,Slug string; Kind domain.NodeKind; ParentID *string; Color,Glyph,Description,UpstreamGit string}` (validates `AllowedChildKind`/root=engagement → `domain.ErrInvalidNode`).
- `UpdateNode usecase.UpdateNode` `.Execute(ctx,owner,id, in usecase.UpdateNodeInput) (domain.Node,error)` with `UpdateNodeInput{Name,Slug,Color,Glyph,Description,UpstreamGit string; Status domain.NodeStatus}` (does not touch rate/parent, per `ports.NodeStore.Update` doc).
- `DeleteNode usecase.DeleteNode` `.Execute(ctx,owner,id) error` (surfaces `ports.ErrNodeHasChildren`).
- `SetNodeRate usecase.SetNodeRate` `.Execute(ctx,owner,id, *domain.Money) error` (engagement-only).
- `MoveNode usecase.MoveNode` `.Execute(ctx,owner,id, newParentID *string) (domain.Node,error)` (→ `domain.ErrInvalidNode`, `usecase.ErrNodeCycle`).
- `ListNodeBindings usecase.ListNodeBindings` `.ExecuteByNode(ctx,owner,nodeID) ([]domain.NodeBinding,error)`.
- Domain: `domain.Node`, `domain.NodeKind` (`KindEngagement/KindVorhaben/KindRepo/KindBranch`), `domain.NodeStatus` (`NodeActive/NodePaused/NodeArchived`), `domain.NodeColors`, `domain.NodeGlyphs`, `domain.ValidParentKind`, `domain.AllowedChildKind`, `domain.NodeBinding{Kind,RemoteSlug,Path}`, `domain.Document.NodeID`, `domain.WorkSession.NodeID`, events `EventNodeCreated/Updated/Moved/Deleted` (`node.created/updated/moved/deleted`).
- testutil (post-A): `testutil.NewFakeNodeStore()`, `testutil.NewFakeNodeBindingStore()`, `domain.NewNode(...)`.

> If Slice C named `CreateNodeInput`/`UpdateNodeInput` fields differently, the only adaptation is at the four call-sites in `webui_nodes.go`; everything else (templ, VM builders) is decoupled.

---

### Task D1: WebUI NodeKind→badge mapping + node style helpers

**Files**
- `internal/adapter/webui/nodekind.go` (new)
- `internal/adapter/webui/nodekind_test.go` (new)
- `internal/adapter/webui/nodestyle.go` (new; replaces `projectstyle.go`)
- `internal/adapter/webui/nodestyle_test.go` (new; replaces `projectstyle_test.go`)
- delete `internal/adapter/webui/projectstyle.go` + `projectstyle_test.go`
- `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go`

**Interfaces**
- `webui.NodeKindStyle(k domain.NodeKind) NodeKindBadge` where `NodeKindBadge{LabelKey, Glyph, Tone string}`.
- `webui.StatusBadge(s domain.NodeStatus) (label, classes string)` (signature change: `ProjectStatus`→`NodeStatus`).
- `webui.ColorHex(name string) string` (unchanged — color names are strings).

Steps:

- [ ] **Step 1** Add i18n keys (DE full, EN stub) for kind labels. In `catalog_de.go` (German map) add:
```go
"node.kind.engagement": "Engagement",
"node.kind.vorhaben":   "Vorhaben",
"node.kind.repo":       "Repo",
"node.kind.branch":     "Branch",
```
In `catalog_en.go` (English map) add the stubs:
```go
"node.kind.engagement": "Engagement",
"node.kind.vorhaben":   "Initiative",
"node.kind.repo":       "Repo",
"node.kind.branch":     "Branch",
```

- [ ] **Step 2** Write the failing test `internal/adapter/webui/nodekind_test.go`:
```go
package webui_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func TestNodeKindStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind       domain.NodeKind
		wantLabel  string
		wantGlyph  string
		wantTone   string
	}{
		{domain.KindEngagement, "node.kind.engagement", "◆", "accent"},
		{domain.KindVorhaben, "node.kind.vorhaben", "▲", "highlight"},
		{domain.KindRepo, "node.kind.repo", "●", "success"},
		{domain.KindBranch, "node.kind.branch", "·", "muted"},
	}
	for _, c := range cases {
		got := webui.NodeKindStyle(c.kind)
		if got.LabelKey != c.wantLabel || got.Glyph != c.wantGlyph || got.Tone != c.wantTone {
			t.Errorf("NodeKindStyle(%q) = %+v", c.kind, got)
		}
	}
}
```
Run `go test ./internal/adapter/webui/ -run TestNodeKindStyle` → FAIL (undefined).

- [ ] **Step 3** Implement `internal/adapter/webui/nodekind.go`:
```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// NodeKindBadge is the WebUI presentation of a domain.NodeKind: an i18n label
// key, a whitelisted monospace glyph and a tone name that maps to the shared
// kindToneClass utility (accent|highlight|success|muted). This is the single
// source of truth for node-kind coloring in the WebUI (the TUI kindcolor pkg
// only maps DocumentType).
type NodeKindBadge struct {
	LabelKey string
	Glyph    string
	Tone     string
}

// NodeKindStyle maps a node kind to its badge treatment.
func NodeKindStyle(k domain.NodeKind) NodeKindBadge {
	switch k {
	case domain.KindEngagement:
		return NodeKindBadge{LabelKey: "node.kind.engagement", Glyph: "◆", Tone: "accent"}
	case domain.KindVorhaben:
		return NodeKindBadge{LabelKey: "node.kind.vorhaben", Glyph: "▲", Tone: "highlight"}
	case domain.KindRepo:
		return NodeKindBadge{LabelKey: "node.kind.repo", Glyph: "●", Tone: "success"}
	case domain.KindBranch:
		return NodeKindBadge{LabelKey: "node.kind.branch", Glyph: "·", Tone: "muted"}
	default:
		return NodeKindBadge{LabelKey: "node.kind.repo", Glyph: "·", Tone: "muted"}
	}
}
```
Run the test → PASS. (`kindToneClass` in `wissen_vm.go` already returns the neutral class for `"muted"`.)

- [ ] **Step 4** Create `internal/adapter/webui/nodestyle.go` (carry `colorHex`/`ColorHex` verbatim from `projectstyle.go`, change `StatusBadge` to `domain.NodeStatus`):
```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// colorHex maps each domain.NodeColors name to its Tokyonight-Night hex.
// MUST cover every name in domain.NodeColors (enforced by a drift-guard test).
var colorHex = map[string]string{
	"blue": "#7aa2f7", "cyan": "#7dcfff", "green": "#9ece6a",
	"purple": "#bb9af7", "magenta": "#ff007c", "yellow": "#e0af68",
	"orange": "#ff9e64", "red": "#f7768e", "teal": "#73daca",
}

// ColorHex returns the swatch hex for a whitelisted color name, or "" for unset
// or unknown (the caller renders no swatch rather than guessing).
func ColorHex(name string) string { return colorHex[name] }

// StatusBadge returns a German label and Tailwind chip classes for a node status.
func StatusBadge(s domain.NodeStatus) (label, classes string) {
	switch s {
	case domain.NodePaused:
		return "pausiert", "rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 opacity-70"
	case domain.NodeArchived:
		return "archiviert", "rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-400"
	default:
		return "aktiv", "rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700"
	}
}
```

- [ ] **Step 5** Recreate the drift-guard `internal/adapter/webui/nodestyle_test.go` (port `projectstyle_test.go`, swapping `domain.ProjectColors`→`domain.NodeColors` and `domain.ProjectStatus`→`domain.NodeStatus`). Delete `projectstyle.go` + `projectstyle_test.go`. Run `go test ./internal/adapter/webui/ -run 'TestNodeKindStyle|TestColor|TestStatusBadge'` → PASS.

- [ ] **Step 6** Commit: `feat(webui): NodeKind badge mapping + node style helpers`.

---

### Task D2: Pure view-model builders (`buildNodeTree`, parent constraints)

**Files**
- `internal/adapter/webui/node_tree_vm.go` (new)
- `internal/adapter/webui/node_tree_vm_internal_test.go` (new, `package webui`)

**Interfaces**
- `type TreeRow struct { Level int; Node domain.Node }`
- `func buildNodeTree(nodes []domain.Node) []TreeRow` — DFS, roots first, siblings name-ordered; cycle/orphan safe.
- `func ValidParentsFor(kind domain.NodeKind, all []domain.Node) []domain.Node` — nodes that may host `kind` (uses `domain.AllowedChildKind`); empty for engagement.
- `func descendantIDs(all []domain.Node, id string) map[string]bool` — `id` + its subtree (move-target exclusion).
- VM structs `NodesPageData`, `NodeFormData`, `NodeFormValues`, `NodeCockpit`, `NodeMoveData` (used by later tasks).

Steps:

- [ ] **Step 1** Failing test `node_tree_vm_internal_test.go`:
```go
package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func ptr(s string) *string { return &s }

func nodesFixture() []domain.Node {
	return []domain.Node{
		{ID: "eng1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "repoB", Kind: domain.KindRepo, Name: "beta", ParentID: ptr("eng1")},
		{ID: "repoA", Kind: domain.KindRepo, Name: "alpha", ParentID: ptr("vor1")},
		{ID: "vor1", Kind: domain.KindVorhaben, Name: "Buch", ParentID: ptr("eng1")},
		{ID: "eng2", Kind: domain.KindEngagement, Name: "RTL Extern"},
	}
}

func TestBuildNodeTree_DFSIndentAndOrder(t *testing.T) {
	t.Parallel()
	rows := buildNodeTree(nodesFixture())
	type lr struct {
		id    string
		level int
	}
	got := make([]lr, len(rows))
	for i, r := range rows {
		got[i] = lr{r.Node.ID, r.Level}
	}
	want := []lr{
		{"eng1", 0}, {"vor1", 1}, {"repoA", 2}, {"repoB", 1}, {"eng2", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("rows=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildNodeTree_OrphanFallback(t *testing.T) {
	t.Parallel()
	rows := buildNodeTree([]domain.Node{{ID: "x", Kind: domain.KindRepo, Name: "x", ParentID: ptr("ghost")}})
	if len(rows) != 1 || rows[0].Level != 0 || rows[0].Node.ID != "x" {
		t.Fatalf("orphan not surfaced: %+v", rows)
	}
}

func TestValidParentsFor(t *testing.T) {
	t.Parallel()
	all := nodesFixture()
	if got := ValidParentsFor(domain.KindEngagement, all); len(got) != 0 {
		t.Errorf("engagement must be root-only, got %d", len(got))
	}
	// repo/vorhaben may hang under engagement or vorhaben.
	repoParents := ValidParentsFor(domain.KindRepo, all)
	ids := map[string]bool{}
	for _, n := range repoParents {
		ids[n.ID] = true
	}
	if !ids["eng1"] || !ids["eng2"] || !ids["vor1"] {
		t.Errorf("repo parents missing engagement/vorhaben: %v", ids)
	}
	if ids["repoA"] || ids["repoB"] {
		t.Errorf("repo may not parent a repo: %v", ids)
	}
}

func TestDescendantIDs(t *testing.T) {
	t.Parallel()
	d := descendantIDs(nodesFixture(), "eng1")
	for _, id := range []string{"eng1", "vor1", "repoA", "repoB"} {
		if !d[id] {
			t.Errorf("missing %q in subtree", id)
		}
	}
	if d["eng2"] {
		t.Errorf("eng2 must not be in eng1 subtree")
	}
}
```
Run `go test ./internal/adapter/webui/ -run 'BuildNodeTree|ValidParentsFor|DescendantIDs'` → FAIL.

- [ ] **Step 2** Implement `internal/adapter/webui/node_tree_vm.go`:
```go
package webui

import (
	"html/template"
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

// TreeRow is one rendered line of the node tree: the node plus its depth (0 =
// engagement root) so the template can indent it.
type TreeRow struct {
	Level int
	Node  domain.Node
}

// buildNodeTree turns a flat node slice into a depth-first, indented row list:
// engagement roots first, each followed by its subtree; siblings are ordered by
// name. It is cycle- and orphan-safe — any node whose parent is absent (or that
// would re-enter a visited subtree) is surfaced at level 0 so nothing is hidden.
func buildNodeTree(nodes []domain.Node) []TreeRow {
	children := map[string][]domain.Node{}
	for _, n := range nodes {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	for k := range children {
		sort.SliceStable(children[k], func(i, j int) bool {
			return children[k][i].Name < children[k][j].Name
		})
	}
	var out []TreeRow
	seen := map[string]bool{}
	var walk func(parentKey string, level int)
	walk = func(parentKey string, level int) {
		for _, n := range children[parentKey] {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			out = append(out, TreeRow{Level: level, Node: n})
			walk(n.ID, level+1)
		}
	}
	walk("", 0)
	// Orphans (parent not in the set) — defensive: never drop a node.
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, TreeRow{Level: 0, Node: n})
		}
	}
	return out
}

// ValidParentsFor returns the nodes that may host a child of the given kind,
// name-ordered. Engagement is always a root → empty result.
func ValidParentsFor(kind domain.NodeKind, all []domain.Node) []domain.Node {
	if kind == domain.KindEngagement {
		return nil
	}
	var out []domain.Node
	for _, n := range all {
		if domain.AllowedChildKind(n.Kind, kind) {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// descendantIDs returns id plus every node in its subtree. Move targets must be
// excluded from this set to keep reparenting acyclic.
func descendantIDs(all []domain.Node, id string) map[string]bool {
	children := map[string][]string{}
	for _, n := range all {
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	out := map[string]bool{id: true}
	var walk func(string)
	walk = func(cur string) {
		for _, c := range children[cur] {
			if !out[c] {
				out[c] = true
				walk(c)
			}
		}
	}
	walk(id)
	return out
}

// ---- page/form/cockpit view models (rendered by D3–D6) ----

// NodesPageData is the tree view model (node-management home).
type NodesPageData struct {
	User   string
	Status string // "" (active+paused) | "archived" | "all"
	Rows   []TreeRow
}

// NodeFormValues holds raw create/edit form fields (re-rendered on error).
type NodeFormValues struct {
	Name, Slug, Kind, ParentID            string
	Description, UpstreamGit, Status       string
	Color, Glyph                           string
	RateAmount, RateCurrency               string
}

// NodeFormData drives the create (editing==nil) / edit form.
type NodeFormData struct {
	User    string
	Error   string
	Vals    NodeFormValues
	Parents []domain.Node // candidate parents (engagements + vorhaben)
}

// NodeCockpit is the read-only repo/vorhaben/engagement detail view model.
type NodeCockpit struct {
	User            string
	N               domain.Node
	Ancestors       []domain.Node // leaf→root (as NodeStore.Ancestors returns)
	DescriptionHTML template.HTML
	TotalHours      float64
	WeekHours       float64
	MonthHours      float64
	Earnings        string
	Docs            []domain.Document
	Bindings        []domain.NodeBinding
	MoveTargets     []domain.Node // valid new parents (for the inline move form)
}
```
Run the tests → PASS.

- [ ] **Step 3** Commit: `feat(webui): pure node-tree + parent-constraint view-model builders`.

---

### Task D3: Node tree page + fragment + handlers (replaces flat project list)

**Files**
- `internal/adapter/webui/nodes.templ` (new) → `nodes_templ.go` via `templ generate`
- `internal/adapter/httpserver/webui_nodes.go` (new; replaces `webui_projects.go` list/home parts)
- `internal/adapter/httpserver/server.go` (route edits)
- `internal/adapter/httpserver/webui_nodes_test.go` (new; replaces `webui_projects_test.go`)
- `internal/i18n/catalog_de.go`, `catalog_en.go`
- delete `internal/adapter/webui/projects.templ` + `projects_templ.go` and `internal/adapter/httpserver/webui_projects.go` + `webui_projects_test.go` (cockpit/form/move parts are re-created in D4–D6 of this slice — do the deletion at the end of D6 once all replacements exist; for D3 only *add* `webui_nodes.go` + tree handlers and leave the old file's cockpit/form handlers in place until D4–D6 replace them. To keep the build green, copy the still-needed `fmtHours/fmtCount/gitDisplay/bindingTarget/orDefault/formAction/parseRate/formValues/orStatus/startOfWeek/startOfMonth` helpers into `node_tree_vm.go`/`webui_nodes.go` now and delete `projects.templ`/`webui_projects.go` only after D6).

> Sequencing note for the executor: to avoid a half-renamed build, do the full swap atomically across D3–D6 in one branch — write all new files, then delete the two `project*` webui files in D6’s final step, then `templ generate` + `make ci`.

**Interfaces**
- Handlers: `handleWebNodesHome` (GET `/projects`), `handleWebNodeTree` (GET `/ui/nodes/tree`, SSE-swap target).
- `nodesListData(r, u) webui.NodesPageData` — `ListNodes` + status filter + `buildNodeTree`.

Steps:

- [ ] **Step 1** Add i18n keys. DE:
```go
"nodes.title":      "Projekte",
"nodes.subtitle":   "Engagements, Vorhaben und Repos",
"nodes.new":        "Neuer Knoten",
"nodes.empty":      "Keine Knoten.",
"nodes.emptyHint":  "Lege ein Engagement an, um zu starten.",
"nodes.filterActive":   "aktiv + pausiert",
"nodes.filterArchived": "archiviert",
"nodes.filterAll":      "alle",
```
EN stubs (same keys): `"Projects" / "Engagements, initiatives and repos" / "New node" / "No nodes." / "Create an engagement to get started." / "active + paused" / "archived" / "all"`.

- [ ] **Step 2** Failing httptest `webui_nodes_test.go` (test harness mirrors `webui_projects_test.go` but with node fakes):
```go
package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func newWebNodesServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeNodeBindingStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	ss := testutil.NewFakeSessionStore()
	docs := testutil.NewFakeDocumentStore()
	srv := &httpserver.Server{
		Users: users, Session: codec, Bus: sse.NewBus(), Clock: clk,
		Ensure: usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Nodes:            ns,
		CreateNode:       usecase.CreateNode{Nodes: ns, IDs: ids, Clock: clk},
		ListNodes:        usecase.ListNodes{Nodes: ns},
		GetNode:          usecase.GetNode{Nodes: ns},
		UpdateNode:       usecase.UpdateNode{Nodes: ns, Bindings: bs, IDs: ids, Clock: clk},
		DeleteNode:       usecase.DeleteNode{Nodes: ns},
		SetNodeRate:      usecase.SetNodeRate{Nodes: ns},
		MoveNode:         usecase.MoveNode{Nodes: ns},
		ListNodeBindings: usecase.ListNodeBindings{Bindings: bs},
		ListSessionsRange: usecase.ListSessionsRange{Sessions: ss},
		ListDocuments:    usecase.ListDocuments{Docs: docs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns
}

func seedNode(t *testing.T, ns *testutil.FakeNodeStore, id, name string, kind domain.NodeKind, parent *string) domain.Node {
	t.Helper()
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	n, err := domain.NewNode(id, "u1", name, name, now)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	n.Kind = kind
	n.ParentID = parent
	n.Status = domain.NodeActive
	_, _ = ns.Create(context.Background(), n)
	return n
}

func getN(t *testing.T, ts *httptest.Server, c *http.Cookie, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.AddCookie(c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body) // add "io" import
	return res.StatusCode, string(b)
}

func postN(t *testing.T, ts *httptest.Server, c *http.Cookie, path string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(form.Encode()))
	req.AddCookie(c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebNodeTree_IndentAndFilter(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	eng := seedNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	seedNode(t, ns, "repo1", "flow", domain.KindRepo, &eng.ID)
	arch := seedNode(t, ns, "eng2", "Alt", domain.KindEngagement, nil)
	arch.Status = domain.NodeArchived
	_, _ = ns.Update(context.Background(), "u1", arch)

	code, body := getN(t, ts, c, "/projects")
	if code != 200 {
		t.Fatalf("GET /projects = %d", code)
	}
	// engagement + child repo render, with the kind labels.
	for _, want := range []string{"Privat", "flow", "Engagement", "Repo"} {
		if !strings.Contains(body, want) {
			t.Errorf("tree missing %q; body=%.500s", want, body)
		}
	}
	// archived hidden by default, shown with ?status=archived.
	if strings.Contains(body, "Alt") {
		t.Errorf("default view must hide archived")
	}
	_, arr := getN(t, ts, c, "/projects?status=archived")
	if !strings.Contains(arr, "Alt") {
		t.Errorf("archived filter should show Alt")
	}
	// SSE fragment route.
	if code, _ := getN(t, ts, c, "/ui/nodes/tree"); code != 200 {
		t.Errorf("GET /ui/nodes/tree = %d", code)
	}
}
```
Run `go test ./internal/adapter/httpserver/ -run TestWebNodeTree` → FAIL.

- [ ] **Step 3** Add the tree templ to `internal/adapter/webui/nodes.templ`:
```go
package webui

import (
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

templ NodesPage(d NodesPageData) {
	@components.Base("projekte", nodesBody(d))
}

templ nodesBody(d NodesPageData) {
	@components.AppShell("projekte", nil, nil, nodesOuter(d))
}

templ nodesOuter(d NodesPageData) {
	<div id="content"
		hx-get={ "/ui/nodes/tree?status=" + d.Status }
		hx-trigger="sse:node.created, sse:node.updated, sse:node.moved, sse:node.deleted"
		hx-swap="innerHTML">
		@NodesFragment(d)
	</div>
}

templ NodesFragment(d NodesPageData) {
	<header class="mb-5 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
		<div>
			<p class="eyebrow mb-1 text-[.72rem] font-semibold uppercase text-blue">{ components.T(ctx, "nav.projects") }</p>
			<h1 class="font-display text-[2rem] font-semibold leading-tight">{ components.T(ctx, "nodes.title") }</h1>
			<p class="mt-1 text-[.9rem] text-muted">{ components.T(ctx, "nodes.subtitle") }</p>
		</div>
		<a href="/projects/new"
			class="inline-flex items-center justify-center gap-2 rounded-2xl bg-ink px-5 py-2.5 text-[.92rem] font-semibold text-canvas shadow-soft transition hover:bg-ink/90 active:scale-[.99]">
			<span aria-hidden="true">✚</span> { components.T(ctx, "nodes.new") }
		</a>
	</header>
	<div class="mb-4 flex flex-wrap gap-2 text-sm">
		<a href="/projects" class={ nodeFilterChip(d.Status == "") }>{ components.T(ctx, "nodes.filterActive") }</a>
		<a href="/projects?status=archived" class={ nodeFilterChip(d.Status == "archived") }>{ components.T(ctx, "nodes.filterArchived") }</a>
		<a href="/projects?status=all" class={ nodeFilterChip(d.Status == "all") }>{ components.T(ctx, "nodes.filterAll") }</a>
	</div>
	if len(d.Rows) == 0 {
		@components.EmptyState("◆", "nodes.empty", "nodes.emptyHint")
	} else {
		<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface shadow-soft">
			for _, row := range d.Rows {
				@nodeTreeRow(row)
			}
		</ul>
	}
}

templ nodeTreeRow(row TreeRow) {
	{{ k := NodeKindStyle(row.Node.Kind) }}
	<li class="p-3 transition-colors hover:bg-sunken/40">
		<a href={ templ.SafeURL("/projects/" + row.Node.ID) } class="group flex items-center gap-3" style={ nodeIndentStyle(row.Level) }>
			@nodeKindBadge(row.Node.Kind)
			@nodeGlyphSwatch(row.Node)
			<span class="min-w-0 flex-1 truncate font-medium text-ink group-hover:text-blue">{ row.Node.Name }</span>
			if row.Node.UpstreamGit != "" {
				<span class="hidden sm:block truncate font-mono text-[.75rem] text-faint">{ gitDisplay(row.Node.UpstreamGit) }</span>
			}
			@nodeStatusBadge(row.Node.Status)
		</a>
	</li>
}

// nodeKindBadge renders the kind pill (glyph + i18n label, kind-toned).
templ nodeKindBadge(kind domain.NodeKind) {
	{{ k := NodeKindStyle(kind) }}
	<span class={ "inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-[.72rem] font-medium", kindToneClass(k.Tone) }>
		<span aria-hidden="true">{ k.Glyph }</span> { components.T(ctx, k.LabelKey) }
	</span>
}

templ nodeGlyphSwatch(n domain.Node) {
	if ColorHex(n.Color) != "" {
		<span class="inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={ "background-color:" + ColorHex(n.Color) }></span>
	}
	if n.Glyph != "" {
		<span class="shrink-0 font-mono text-faint">{ n.Glyph }</span>
	}
}

templ nodeStatusBadge(s domain.NodeStatus) {
	{{ label, classes := StatusBadge(s) }}
	<span class={ "shrink-0", classes }>{ label }</span>
}
```
Add the Go helpers to `node_tree_vm.go` (append):
```go
import "fmt" // ensure present

func nodeFilterChip(active bool) string {
	if active {
		return "rounded-full bg-ink px-3 py-1 text-xs font-medium text-canvas"
	}
	return "rounded-full border border-line bg-surface px-3 py-1 text-xs font-medium text-muted hover:border-blue/40 hover:text-blue"
}

// nodeIndentStyle indents a tree row by depth (1rem per level).
func nodeIndentStyle(level int) string { return fmt.Sprintf("padding-left:%drem", level) }
```
Also move `gitDisplay` into `node_tree_vm.go` (copy from `projects.templ` verbatim).

- [ ] **Step 4** Implement `internal/adapter/httpserver/webui_nodes.go` (tree handlers only for now):
```go
package httpserver

import (
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// nodesListData loads the owner's nodes, applies the status filter and builds
// the indented tree. "" → active+paused; "archived" → archived only; "all".
func (s *Server) nodesListData(r *http.Request, u domain.User) webui.NodesPageData {
	status := r.URL.Query().Get("status")
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	filtered := make([]domain.Node, 0, len(all))
	for _, n := range all {
		switch status {
		case "all":
			filtered = append(filtered, n)
		case "archived":
			if n.Status == domain.NodeArchived {
				filtered = append(filtered, n)
			}
		default:
			if n.Status == domain.NodeActive || n.Status == domain.NodePaused {
				filtered = append(filtered, n)
			}
		}
	}
	return webui.NodesPageData{User: u.Username, Status: status, Rows: webui.BuildTree(filtered)}
}

func (s *Server) handleWebNodesHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodesPage(s.nodesListData(r, u)).Render(r.Context(), w)
}

func (s *Server) handleWebNodeTree(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodesFragment(s.nodesListData(r, u)).Render(r.Context(), w)
}
```
Export a thin `BuildTree` wrapper in `node_tree_vm.go` so the httpserver package can call it (keep `buildNodeTree` unexported for the internal unit tests):
```go
// BuildTree is the exported entry point used by the httpserver adapter.
func BuildTree(nodes []domain.Node) []TreeRow { return buildNodeTree(nodes) }
```

- [ ] **Step 5** In `server.go`: change the project home/list routes to node handlers (keep the user-facing `/projects` prefix; rename the fragment route):
```go
mux.Handle("GET /projects", s.webAuth(http.HandlerFunc(s.handleWebNodesHome)))
mux.Handle("GET /ui/nodes/tree", s.webAuth(http.HandlerFunc(s.handleWebNodeTree)))
```
(Remove the old `GET /ui/projects/list → handleWebProjectsList` line.) Leave the remaining `/projects/...` cockpit/form/status/delete routes pointing at the old handlers for now; D4–D6 replace them.

- [ ] **Step 6** `templ generate`; run `go test ./internal/adapter/httpserver/ -run TestWebNodeTree` → PASS. Commit generated `nodes_templ.go`. Commit: `feat(webui): node hierarchy tree page + SSE node.* fragment`.

---

### Task D4: Create/Edit form (kind, constrained parent, color, glyph, desc, upstream, rate)

**Files**
- `internal/adapter/webui/nodes.templ` (append form templ) → regen
- `internal/adapter/httpserver/webui_nodes.go` (append form handlers)
- `internal/adapter/httpserver/server.go` (route edits)
- `internal/adapter/httpserver/webui_nodes_test.go` (append tests)
- `internal/i18n/catalog_de.go`, `catalog_en.go`

**Interfaces**
- Handlers: `handleWebNodeNew` (GET `/projects/new`), `handleWebNodeCreate` (POST `/projects`), `handleWebNodeEdit` (GET `/projects/{id}/edit`), `handleWebNodeUpdate` (POST `/projects/{id}`), `handleWebNodeStatus` (POST `/projects/{id}/status`), `handleWebNodeDelete` (POST `/projects/{id}/delete`).
- `nodeFormValues(r) webui.NodeFormValues`, `parseRate(amount,currency)` (reuse from old projects helper — move to `webui_nodes.go`).

Steps:

- [ ] **Step 1** Add i18n keys. DE:
```go
"node.create.title": "Neuer Knoten",
"node.edit.title":   "Knoten bearbeiten",
"node.name":         "Name",
"node.slug":         "Slug",
"node.kind":         "Art",
"node.parent":       "Übergeordnet",
"node.parentRoot":   "— (Wurzel / Engagement)",
"node.description":  "Beschreibung (Markdown)",
"node.upstream":     "Upstream Git",
"node.status":       "Status",
"node.color":        "Farbe",
"node.glyph":        "Glyph",
"node.rate":         "Satz (optional)",
"node.rateCurrency": "Währung",
"node.rateHint":     "nur Engagement",
"node.err.nameRequired": "Name erforderlich",
"node.err.badUpstream":  "Ungültige Upstream-Git-URL",
"node.err.create":       "Konnte Knoten nicht anlegen",
"node.status.active":    "aktiv",
"node.status.paused":    "pausiert",
"node.status.archived":  "archiviert",
```
EN stubs with English text (`"New node" / "Edit node" / "Name" / "Slug" / "Kind" / "Parent" / "— (root / engagement)" / "Description (Markdown)" / "Upstream Git" / "Status" / "Color" / "Glyph" / "Rate (optional)" / "Currency" / "engagement only" / "Name required" / "Invalid upstream Git URL" / "Could not create node" / "active" / "paused" / "archived"`).

- [ ] **Step 2** Failing test (append to `webui_nodes_test.go`):
```go
func TestWebNodeCreateEditStatusDelete(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	eng := seedNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)

	// form lists the engagement as a candidate parent.
	_, form := getN(t, ts, c, "/projects/new")
	if !strings.Contains(form, "RTL Extern") || !strings.Contains(form, "node.kind") == false {
		// (label assertions below; this just ensures the parent shows up)
	}
	if !strings.Contains(form, "RTL Extern") {
		t.Errorf("new-form should list parent engagement")
	}

	// CREATE a repo under the engagement.
	res := postN(t, ts, c, "/projects", url.Values{
		"name": {"flow"}, "slug": {"flow"}, "kind": {"repo"}, "parentId": {eng.ID},
		"upstreamGit": {"git@github.com:serverkraken/flow.git"}, "status": {"active"},
		"color": {domain.NodeColors[0]}, "glyph": {domain.NodeGlyphs[0]},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create = %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	id := strings.TrimPrefix(loc, "/projects/")
	got, _ := ns.Get(context.Background(), "u1", id)
	if got.Kind != domain.KindRepo || got.ParentID == nil || *got.ParentID != eng.ID {
		t.Fatalf("created node wrong kind/parent: %+v", got)
	}

	// CREATE engagement with a rate; rate must persist (engagement-only).
	res = postN(t, ts, c, "/projects", url.Values{
		"name": {"Beratung"}, "kind": {"engagement"}, "parentId": {""},
		"status": {"active"}, "rateAmount": {"95.00"}, "rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create engagement = %d", res.StatusCode)
	}
	eid := strings.TrimPrefix(res.Header.Get("Location"), "/projects/")
	_ = res.Body.Close()
	e2, _ := ns.Get(context.Background(), "u1", eid)
	if e2.Rate == nil || e2.Rate.Amount != 9500 {
		t.Errorf("engagement rate not set: %+v", e2.Rate)
	}

	// name required → 400 re-render.
	res = postN(t, ts, c, "/projects", url.Values{"kind": {"repo"}, "parentId": {eng.ID}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing name = %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// STATUS → archive.
	res = postN(t, ts, c, "/projects/"+id+"/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", res.StatusCode)
	}
	_ = res.Body.Close()
	got, _ = ns.Get(context.Background(), "u1", id)
	if got.Status != domain.NodeArchived {
		t.Errorf("not archived: %s", got.Status)
	}

	// DELETE (leaf, no children).
	res = postN(t, ts, c, "/projects/"+id+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, err := ns.Get(context.Background(), "u1", id); err == nil {
		t.Errorf("node should be deleted")
	}
}
```
Run → FAIL.

- [ ] **Step 3** Append the form templ to `nodes.templ`:
```go
templ NodeForm(d NodeFormData, editing *domain.Node) {
	@components.Base("projekte", nodeFormBody(d, editing))
}

templ nodeFormBody(d NodeFormData, editing *domain.Node) {
	@components.AppShell("projekte", nil, nil, nodeFormInner(d, editing))
}

templ nodeFormInner(d NodeFormData, editing *domain.Node) {
	<h1 class="mb-4 font-display text-2xl font-semibold">
		if editing != nil {
			{ components.T(ctx, "node.edit.title") }
		} else {
			{ components.T(ctx, "node.create.title") }
		}
	</h1>
	if d.Error != "" {
		<div class="mb-4 rounded-2xl bg-red/10 px-4 py-2 text-sm font-medium text-red" role="alert">{ d.Error }</div>
	}
	<form method="post" action={ nodeFormAction(editing) } hx-boost="false" class="max-w-2xl space-y-4 text-sm">
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.name") }</label>
			<input name="name" value={ d.Vals.Name } required class="w-full rounded-lg border border-line bg-surface px-3 py-2"/>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.slug") }</label>
			<input name="slug" value={ d.Vals.Slug } class="w-full rounded-lg border border-line bg-surface px-3 py-2 font-mono"/>
		</div>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<label class="block text-muted">{ components.T(ctx, "node.kind") }</label>
				<select name="kind" id="node-kind" class="w-full rounded-lg border border-line bg-surface px-3 py-2">
					@nodeKindOption("engagement", "node.kind.engagement", d.Vals.Kind)
					@nodeKindOption("vorhaben", "node.kind.vorhaben", d.Vals.Kind)
					@nodeKindOption("repo", "node.kind.repo", d.Vals.Kind)
				</select>
			</div>
			<div>
				<label class="block text-muted">{ components.T(ctx, "node.parent") }</label>
				<select name="parentId" id="node-parent" class="w-full rounded-lg border border-line bg-surface px-3 py-2">
					<option value="">{ components.T(ctx, "node.parentRoot") }</option>
					for _, p := range d.Parents {
						if d.Vals.ParentID == p.ID {
							<option value={ p.ID } data-parent-kind={ string(p.Kind) } selected>{ nodeParentLabel(p) }</option>
						} else {
							<option value={ p.ID } data-parent-kind={ string(p.Kind) }>{ nodeParentLabel(p) }</option>
						}
					}
				</select>
			</div>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.description") }</label>
			<textarea name="description" rows="6" class="w-full rounded-lg border border-line bg-surface px-3 py-2 font-mono">{ d.Vals.Description }</textarea>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.upstream") }</label>
			<input name="upstreamGit" value={ d.Vals.UpstreamGit } placeholder="git@github.com:org/repo.git" class="w-full rounded-lg border border-line bg-surface px-3 py-2 font-mono"/>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.status") }</label>
			<select name="status" class="rounded-lg border border-line bg-surface px-3 py-2">
				@nodeStatusOption("active", "node.status.active", d.Vals.Status)
				@nodeStatusOption("paused", "node.status.paused", d.Vals.Status)
				@nodeStatusOption("archived", "node.status.archived", d.Vals.Status)
			</select>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.color") }</label>
			<div class="flex flex-wrap gap-2">
				@nodeColorRadio("", d.Vals.Color)
				for _, name := range domain.NodeColors {
					@nodeColorRadio(name, d.Vals.Color)
				}
			</div>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.glyph") }</label>
			<div class="flex flex-wrap gap-2">
				@nodeGlyphRadio("", d.Vals.Glyph)
				for _, g := range domain.NodeGlyphs {
					@nodeGlyphRadio(g, d.Vals.Glyph)
				}
			</div>
		</div>
		<div id="node-rate" class="flex flex-wrap items-end gap-3">
			<div>
				<label class="block text-muted">{ components.T(ctx, "node.rate") }</label>
				<input name="rateAmount" value={ d.Vals.RateAmount } placeholder="95.00" class="w-32 rounded-lg border border-line bg-surface px-3 py-2"/>
			</div>
			<div>
				<label class="block text-muted">{ components.T(ctx, "node.rateCurrency") }</label>
				<input name="rateCurrency" value={ orDefault(d.Vals.RateCurrency, "EUR") } class="w-20 rounded-lg border border-line bg-surface px-3 py-2"/>
			</div>
			<span class="text-[.75rem] text-faint">{ components.T(ctx, "node.rateHint") }</span>
		</div>
		<div class="flex gap-2">
			@components.Button(components.BtnPrimary, components.T(ctx, "common.save"), "✓", templ.Attributes{"type": "submit"})
			<a href="/projects" class="rounded-2xl border border-line px-4 py-2 hover:bg-sunken">{ components.T(ctx, "common.cancel") }</a>
		</div>
	</form>
	if editing != nil {
		<form method="post" action={ templ.SafeURL("/projects/" + editing.ID + "/delete") } hx-boost="false" class="mt-3 max-w-2xl">
			@components.Button(components.BtnDanger, components.T(ctx, "common.delete"), "✗", templ.Attributes{"type": "submit"})
		</form>
	}
	// Progressive enhancement: engagement = root (disable parent); the create
	// usecase is authoritative regardless of this script.
	<script src="/static/js/nodeform.js" defer></script>
}

templ nodeKindOption(val, labelKey, current string) {
	if val == current || (current == "" && val == "engagement") {
		<option value={ val } selected>{ components.T(ctx, labelKey) }</option>
	} else {
		<option value={ val }>{ components.T(ctx, labelKey) }</option>
	}
}

templ nodeStatusOption(val, labelKey, current string) {
	if val == current || (current == "" && val == "active") {
		<option value={ val } selected>{ components.T(ctx, labelKey) }</option>
	} else {
		<option value={ val }>{ components.T(ctx, labelKey) }</option>
	}
}

templ nodeColorRadio(name, current string) {
	<label class="cursor-pointer">
		if name == current {
			<input type="radio" name="color" value={ name } checked class="peer sr-only"/>
		} else {
			<input type="radio" name="color" value={ name } class="peer sr-only"/>
		}
		if name == "" {
			<span class="inline-flex h-6 w-6 items-center justify-center rounded-full border border-line text-xs text-faint peer-checked:ring-2 peer-checked:ring-blue">∅</span>
		} else {
			<span class="inline-block h-6 w-6 rounded-full ring-offset-1 peer-checked:ring-2 peer-checked:ring-blue" style={ "background-color:" + ColorHex(name) }></span>
		}
	</label>
}

templ nodeGlyphRadio(g, current string) {
	<label class="cursor-pointer">
		if g == current {
			<input type="radio" name="glyph" value={ g } checked class="peer sr-only"/>
		} else {
			<input type="radio" name="glyph" value={ g } class="peer sr-only"/>
		}
		<span class="inline-flex h-6 w-6 items-center justify-center rounded border border-line font-mono peer-checked:ring-2 peer-checked:ring-blue">
			if g == "" {
				∅
			} else {
				{ g }
			}
		</span>
	</label>
}
```
Add Go helpers to `node_tree_vm.go`:
```go
func nodeFormAction(editing *domain.Node) templ.SafeURL {
	if editing != nil {
		return templ.SafeURL("/projects/" + editing.ID)
	}
	return templ.SafeURL("/projects")
}

func nodeParentLabel(p domain.Node) string { return p.Name }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```
(`templ` import: add `"github.com/a-h/templ"` to `node_tree_vm.go`.)

- [ ] **Step 4** Add `internal/adapter/webui/static/js/nodeform.js` (minimal, no popups):
```js
(function () {
	var kind = document.getElementById('node-kind');
	var parent = document.getElementById('node-parent');
	var rate = document.getElementById('node-rate');
	if (!kind || !parent) return;
	function sync() {
		var isEng = kind.value === 'engagement';
		parent.disabled = isEng;
		if (isEng) parent.value = '';
		if (rate) rate.style.display = isEng ? '' : 'none';
		Array.prototype.forEach.call(parent.options, function (o) {
			if (!o.value) return;
			var pk = o.getAttribute('data-parent-kind');
			o.hidden = !(pk === 'engagement' || pk === 'vorhaben');
		});
	}
	kind.addEventListener('change', sync);
	sync();
})();
```

- [ ] **Step 5** Append form handlers to `webui_nodes.go`:
```go
import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func nodeFormValues(r *http.Request) webui.NodeFormValues {
	return webui.NodeFormValues{
		Name: r.FormValue("name"), Slug: r.FormValue("slug"),
		Kind: r.FormValue("kind"), ParentID: r.FormValue("parentId"),
		Description: r.FormValue("description"), UpstreamGit: r.FormValue("upstreamGit"),
		Status: r.FormValue("status"), Color: r.FormValue("color"), Glyph: r.FormValue("glyph"),
		RateAmount: r.FormValue("rateAmount"), RateCurrency: r.FormValue("rateCurrency"),
	}
}

func parseRate(amount, currency string) (*domain.Money, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 {
		return nil, fmt.Errorf("ungültiger Satz %q", amount)
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "EUR"
	}
	return &domain.Money{Amount: int64(f*100 + 0.5), Currency: cur}, nil
}

func parentPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func orStatus(s string) string {
	if s == "" {
		return "active"
	}
	return s
}

// nodeParents returns the candidate parents (engagements + vorhaben) for the
// form, name-ordered.
func (s *Server) nodeParents(r *http.Request, u domain.User) []domain.Node {
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	var out []domain.Node
	for _, n := range all {
		if n.Kind == domain.KindEngagement || n.Kind == domain.KindVorhaben {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) handleWebNodeNew(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeForm(webui.NodeFormData{
		User:    u.Username,
		Vals:    webui.NodeFormValues{Kind: "engagement", Status: "active"},
		Parents: s.nodeParents(r, u),
	}, nil).Render(r.Context(), w)
}

func (s *Server) handleWebNodeCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vals := nodeFormValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Error: msg, Vals: vals, Parents: s.nodeParents(r, u)}, nil).Render(r.Context(), w)
	}
	if vals.Name == "" {
		reRender(i18nT(r, "node.err.nameRequired"))
		return
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	if vals.UpstreamGit != "" {
		if _, ok := domain.NormalizeRemoteSlug(vals.UpstreamGit); !ok {
			reRender(i18nT(r, "node.err.badUpstream"))
			return
		}
	}
	kind := domain.NodeKind(vals.Kind)
	parent := parentPtr(vals.ParentID)
	if kind == domain.KindEngagement {
		parent = nil
	}
	n, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
		Name: vals.Name, Slug: vals.Slug, Kind: kind, ParentID: parent,
		Color: vals.Color, Glyph: vals.Glyph,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
	})
	if err != nil {
		reRender(i18nT(r, "node.err.create") + ": " + err.Error())
		return
	}
	if kind == domain.KindEngagement && rate != nil {
		_ = s.SetNodeRate.Execute(r.Context(), u.ID, n.ID, rate)
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": n.ID}})
	http.Redirect(w, r, "/projects/"+n.ID, http.StatusSeeOther)
}

func (s *Server) handleWebNodeEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	n, err := s.GetNode.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	vals := webui.NodeFormValues{
		Name: n.Name, Slug: n.Slug, Kind: string(n.Kind), Status: string(n.Status),
		Description: n.Description, UpstreamGit: n.UpstreamGit, Color: n.Color, Glyph: n.Glyph,
	}
	if n.ParentID != nil {
		vals.ParentID = *n.ParentID
	}
	if n.Rate != nil {
		vals.RateAmount = fmt.Sprintf("%d.%02d", n.Rate.Amount/100, n.Rate.Amount%100)
		vals.RateCurrency = n.Rate.Currency
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Vals: vals, Parents: s.nodeParents(r, u)}, &n).Render(r.Context(), w)
}

func (s *Server) handleWebNodeUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	vals := nodeFormValues(r)
	rate, rerr := parseRate(vals.RateAmount, vals.RateCurrency)
	cur, gerr := s.GetNode.Execute(r.Context(), u.ID, id)
	if gerr != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	reRender := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = webui.NodeForm(webui.NodeFormData{User: u.Username, Error: msg, Vals: vals, Parents: s.nodeParents(r, u)}, &cur).Render(r.Context(), w)
	}
	if rerr != nil {
		reRender(rerr.Error())
		return
	}
	n, err := s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name: vals.Name, Slug: vals.Slug, Color: vals.Color, Glyph: vals.Glyph,
		Description: vals.Description, UpstreamGit: vals.UpstreamGit,
		Status: domain.NodeStatus(orStatus(vals.Status)),
	})
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case errors.Is(err, domain.ErrInvalidNode):
		reRender(err.Error())
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Rate only applies to engagements (store enforces kind); skip otherwise.
	if n.Kind == domain.KindEngagement {
		_ = s.SetNodeRate.Execute(r.Context(), u.ID, id, rate)
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": n.ID}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebNodeStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	cur, err := s.GetNode.Execute(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, err = s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name: cur.Name, Slug: cur.Slug, Color: cur.Color, Glyph: cur.Glyph,
		Description: cur.Description, UpstreamGit: cur.UpstreamGit,
		Status: domain.NodeStatus(r.FormValue("status")),
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}

func (s *Server) handleWebNodeDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.DeleteNode.Execute(r.Context(), u.ID, id); err != nil {
		// RESTRICT on parent_id → has children.
		if errors.Is(err, ports.ErrNodeHasChildren) {
			http.Redirect(w, r, "/projects/"+id+"?err=children", http.StatusSeeOther)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeDeleted, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// i18nT resolves an i18n key in the request's language for handler-side messages.
func i18nT(r *http.Request, key string) string { return i18n.T(r.Context(), key) }
```
(Add `"github.com/serverkraken/flow/internal/i18n"` import; if a request-scoped translator already exists in httpserver, use that instead of `i18nT`.)

- [ ] **Step 6** In `server.go`, repoint the form routes to the new handlers:
```go
mux.Handle("GET /projects/new", s.webAuth(http.HandlerFunc(s.handleWebNodeNew)))
mux.Handle("POST /projects", s.webAuth(http.HandlerFunc(s.handleWebNodeCreate)))
mux.Handle("GET /projects/{id}/edit", s.webAuth(http.HandlerFunc(s.handleWebNodeEdit)))
mux.Handle("POST /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeUpdate)))
mux.Handle("POST /projects/{id}/status", s.webAuth(http.HandlerFunc(s.handleWebNodeStatus)))
mux.Handle("POST /projects/{id}/delete", s.webAuth(http.HandlerFunc(s.handleWebNodeDelete)))
```

- [ ] **Step 7** `templ generate`; run `go test ./internal/adapter/httpserver/ -run TestWebNodeCreateEditStatusDelete` → PASS. Ensure `nodeform.js` is served (it lives under `static/js/`, already embedded by `static.go`’s `embed`). Commit generated `nodes_templ.go` + `nodeform.js`. Commit: `feat(webui): node create/edit form with kind + constrained parent + engagement rate`.

---

### Task D5: Move (reparent) action

**Files**
- `internal/adapter/webui/nodes.templ` (append move form templ, used in the cockpit) → regen
- `internal/adapter/httpserver/webui_nodes.go` (append `handleWebNodeMove`)
- `internal/adapter/httpserver/server.go` (route)
- `internal/adapter/httpserver/webui_nodes_test.go` (append test)
- `internal/i18n/catalog_de.go`, `catalog_en.go`

**Interfaces**
- Handler `handleWebNodeMove` (POST `/projects/{id}/move`), body `parentId` (`""` = root). Uses `MoveNode` (validates kind + cycle).
- `moveTargetsFor(all []domain.Node, n domain.Node) []domain.Node` — `ValidParentsFor(n.Kind, all)` minus `descendantIDs(all, n.ID)`.

Steps:

- [ ] **Step 1** Add i18n keys. DE:
```go
"node.move":        "Verschieben",
"node.moveTitle":   "Knoten verschieben",
"node.moveTarget":  "Neues übergeordnetes Element",
"node.err.move":    "Verschieben fehlgeschlagen",
"node.err.cycle":   "Ziel liegt im eigenen Teilbaum",
"node.err.children":"Knoten hat untergeordnete Elemente und kann nicht gelöscht werden",
```
EN stubs: `"Move" / "Move node" / "New parent" / "Move failed" / "Target is inside its own subtree" / "Node has children and cannot be deleted"`.

- [ ] **Step 2** Failing test (append):
```go
func TestWebNodeMove(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	e1 := seedNode(t, ns, "e1", "Privat", domain.KindEngagement, nil)
	e2 := seedNode(t, ns, "e2", "RTL", domain.KindEngagement, nil)
	repo := seedNode(t, ns, "r1", "flow", domain.KindRepo, &e1.ID)

	// move repo from e1 to e2.
	res := postN(t, ts, c, "/projects/"+repo.ID+"/move", url.Values{"parentId": {e2.ID}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("move = %d", res.StatusCode)
	}
	_ = res.Body.Close()
	got, _ := ns.Get(context.Background(), "u1", repo.ID)
	if got.ParentID == nil || *got.ParentID != e2.ID {
		t.Fatalf("reparent failed: %+v", got.ParentID)
	}

	// cycle: move e1 under repo (its descendant) → handler redirects back, no change.
	res = postN(t, ts, c, "/projects/"+e1.ID+"/move", url.Values{"parentId": {repo.ID}})
	_ = res.Body.Close()
	e1got, _ := ns.Get(context.Background(), "u1", e1.ID)
	if e1got.ParentID != nil {
		t.Errorf("cycle move must be rejected, parent=%v", e1got.ParentID)
	}
}
```
Run → FAIL.

- [ ] **Step 3** Append the move form templ to `nodes.templ` (rendered inside the cockpit in D6):
```go
templ nodeMoveForm(d NodeCockpit) {
	if len(d.MoveTargets) > 0 || d.N.ParentID != nil {
		<form method="post" action={ templ.SafeURL("/projects/" + d.N.ID + "/move") } hx-boost="false" class="flex flex-wrap items-end gap-2">
			<div>
				<label class="block text-[.75rem] text-muted">{ components.T(ctx, "node.moveTarget") }</label>
				<select name="parentId" class="rounded-lg border border-line bg-surface px-3 py-2 text-sm">
					<option value="">{ components.T(ctx, "node.parentRoot") }</option>
					for _, t := range d.MoveTargets {
						if d.N.ParentID != nil && *d.N.ParentID == t.ID {
							<option value={ t.ID } selected>{ t.Name }</option>
						} else {
							<option value={ t.ID }>{ t.Name }</option>
						}
					}
				</select>
			</div>
			@components.Button(components.BtnSecondary, components.T(ctx, "node.move"), "→", templ.Attributes{"type": "submit"})
		</form>
	}
}
```
Add Go helper to `node_tree_vm.go`:
```go
// moveTargetsFor returns valid new parents for n: parents allowed by kind,
// excluding n and its subtree (keeps reparenting acyclic).
func moveTargetsFor(all []domain.Node, n domain.Node) []domain.Node {
	sub := descendantIDs(all, n.ID)
	var out []domain.Node
	for _, p := range ValidParentsFor(n.Kind, all) {
		if !sub[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// MoveTargetsFor is the exported entry point for the httpserver adapter.
func MoveTargetsFor(all []domain.Node, n domain.Node) []domain.Node { return moveTargetsFor(all, n) }
```

- [ ] **Step 4** Append handler to `webui_nodes.go`:
```go
func (s *Server) handleWebNodeMove(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	parent := parentPtr(r.FormValue("parentId"))
	_, err := s.MoveNode.Execute(r.Context(), u.ID, id, parent)
	switch {
	case errors.Is(err, usecase.ErrNodeCycle):
		http.Redirect(w, r, "/projects/"+id+"?err=cycle", http.StatusSeeOther)
		return
	case errors.Is(err, domain.ErrInvalidNode):
		http.Redirect(w, r, "/projects/"+id+"?err=move", http.StatusSeeOther)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventNodeMoved, UserID: u.ID, Data: map[string]any{"id": id}})
	http.Redirect(w, r, "/projects/"+id, http.StatusSeeOther)
}
```

- [ ] **Step 5** Route in `server.go`:
```go
mux.Handle("POST /projects/{id}/move", s.webAuth(http.HandlerFunc(s.handleWebNodeMove)))
```

- [ ] **Step 6** `templ generate`; run `go test ./internal/adapter/httpserver/ -run TestWebNodeMove` → PASS. Commit: `feat(webui): node move (reparent) action with cycle guard`.

---

### Task D6: Node cockpit — ancestor breadcrumb + bindings panel + node-aware worktime aggregate

**Files**
- `internal/adapter/webui/nodes.templ` (append cockpit templ) → regen
- `internal/adapter/httpserver/webui_nodes.go` (append `nodeCockpitData`, `nodeWorktime`, `handleWebNodeView`)
- `internal/adapter/httpserver/server.go` (route)
- `internal/adapter/httpserver/webui_nodes_test.go` (append test)
- `internal/i18n/catalog_de.go`, `catalog_en.go`
- **end of D6:** delete `internal/adapter/webui/projects.templ` + `projects_templ.go`, `internal/adapter/httpserver/webui_projects.go` + `webui_projects_test.go`; remove leftover `/api/v1/projects/*` only if Slice C already moved them (otherwise leave REST alone — this slice is WebUI).

**Interfaces**
- `handleWebNodeView` (GET `/projects/{id}`) → `webui.NodeView(NodeCockpit)`.
- `nodeCockpitData(r,u,id) (webui.NodeCockpit, error)`: `GetNode` + `Nodes.Ancestors` + rendered description + `nodeWorktime` aggregate (sessions filtered by `NodeID==id`; for an engagement this is its booked time) + node-scoped docs (`ListDocuments` with `&id`) + `ListNodeBindings.ExecuteByNode` + `MoveTargetsFor`.

Steps:

- [ ] **Step 1** Add i18n keys. DE:
```go
"node.section.worktime": "Worktime",
"node.section.docs":     "Dokumente",
"node.section.bindings": "Bindings",
"node.section.git":      "Git",
"node.ancestors":        "Hierarchie",
"node.worktime.total":   "Σ",
"node.worktime.week":    "Woche",
"node.worktime.month":   "Monat",
"node.none":             "—",
```
EN stubs: `"Worktime" / "Documents" / "Bindings" / "Git" / "Hierarchy" / "Σ" / "Week" / "Month" / "—"`.

- [ ] **Step 2** Failing test (append):
```go
func TestWebNodeCockpit(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	eng := seedNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)
	repo := seedNode(t, ns, "r1", "flow", domain.KindRepo, &eng.ID)
	repo.Description = "# Notiz\nhallo"
	repo.UpstreamGit = "git@github.com:serverkraken/flow.git"
	_, _ = ns.Update(context.Background(), "u1", repo)

	code, body := getN(t, ts, c, "/projects/r1")
	if code != 200 {
		t.Fatalf("cockpit = %d", code)
	}
	for _, want := range []string{
		"flow", "RTL Extern", // ancestor breadcrumb (engagement parent)
		"github.com/serverkraken/flow", // git display
		"Notiz",                        // rendered markdown
		"Repo",                         // kind badge label
		"node.move" == "" && "" || "Verschieben", // move form present
	} {
		if want == "" {
			continue
		}
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q; body=%.700s", want, body)
		}
	}
	if code, _ := getN(t, ts, c, "/projects/nope"); code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", code)
	}
}
```
Run → FAIL.

- [ ] **Step 3** Append cockpit templ to `nodes.templ`:
```go
import "github.com/serverkraken/flow/internal/adapter/webui/components"

templ NodeView(d NodeCockpit) {
	@components.Base("projekte", nodeViewBody(d))
}

templ nodeViewBody(d NodeCockpit) {
	@components.AppShell("projekte", nodeBreadcrumb(d), nil, nodeViewOuter(d))
}

templ nodeViewOuter(d NodeCockpit) {
	<div id="content"
		hx-get={ "/projects/" + d.N.ID }
		hx-trigger="sse:node.updated, sse:node.moved"
		hx-swap="outerHTML">
		@nodeCockpitBody(d)
	</div>
}

// nodeBreadcrumb renders the ancestor chain root→leaf. d.Ancestors is leaf→root
// (NodeStore.Ancestors order), so iterate in reverse.
templ nodeBreadcrumb(d NodeCockpit) {
	{{ crumbs := nodeCrumbs(d) }}
	@components.Breadcrumb(crumbs)
}

templ nodeCockpitBody(d NodeCockpit) {
	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<h1 class="flex items-center gap-3 font-display text-2xl font-semibold">
			@nodeKindBadge(d.N.Kind)
			@nodeGlyphSwatch(d.N)
			{ d.N.Name }
			@nodeStatusBadge(d.N.Status)
		</h1>
		<div class="flex gap-2 text-sm">
			<a href={ templ.SafeURL("/projects/" + d.N.ID + "/edit") } class="rounded-2xl bg-ink px-4 py-2 text-canvas hover:bg-ink/90">{ components.T(ctx, "common.edit") }</a>
		</div>
	</div>
	if d.DescriptionHTML != "" {
		<div class="prose prose-invert mb-6 max-w-none text-sm">@templ.Raw(string(d.DescriptionHTML))</div>
	}
	if d.N.UpstreamGit != "" {
		<section class="mb-6">
			<h2 class="mb-1 text-sm font-medium text-body">{ components.T(ctx, "node.section.git") }</h2>
			<code class="rounded-lg bg-sunken px-2 py-1 text-xs">{ gitDisplay(d.N.UpstreamGit) }</code>
		</section>
	}
	<section class="mb-6">
		<h2 class="mb-1 text-sm font-medium text-body">{ components.T(ctx, "node.section.worktime") }</h2>
		<div class="flex flex-wrap gap-6 text-sm text-muted">
			<span>{ components.T(ctx, "node.worktime.total") } { fmtHours(d.TotalHours) }</span>
			<span>{ components.T(ctx, "node.worktime.week") } { fmtHours(d.WeekHours) }</span>
			<span>{ components.T(ctx, "node.worktime.month") } { fmtHours(d.MonthHours) }</span>
			if d.Earnings != "" {
				<span class="font-medium text-ink">{ d.Earnings }</span>
			}
		</div>
	</section>
	<section class="mb-6">
		<h2 class="mb-1 text-sm font-medium text-body">{ components.T(ctx, "node.section.docs") } ({ fmtCount(len(d.Docs)) })</h2>
		if len(d.Docs) == 0 {
			<p class="text-sm text-faint">{ components.T(ctx, "node.none") }</p>
		} else {
			<ul class="divide-y divide-line2 rounded-2xl border border-line bg-surface">
				for _, doc := range d.Docs {
					<li class="px-3 py-2 text-sm"><a href={ templ.SafeURL("/wissen/" + doc.ID) } class="hover:text-blue">{ doc.Title }</a></li>
				}
			</ul>
		}
	</section>
	<section class="mb-6">
		<h2 class="mb-1 text-sm font-medium text-body">{ components.T(ctx, "node.section.bindings") }</h2>
		if len(d.Bindings) == 0 {
			<p class="text-sm text-faint">{ components.T(ctx, "node.none") }</p>
		} else {
			<ul class="text-xs text-muted">
				for _, b := range d.Bindings {
					<li class="font-mono">{ string(b.Kind) }: { bindingTarget(b) }</li>
				}
			</ul>
		}
	</section>
	<section>
		<h2 class="mb-1 text-sm font-medium text-body">{ components.T(ctx, "node.moveTitle") }</h2>
		@nodeMoveForm(d)
	</section>
}
```
Add Go helpers to `node_tree_vm.go` (carry `fmtHours`, `fmtCount`, `bindingTarget` from old `projects.templ`, adjusting `bindingTarget` to `domain.NodeBinding`):
```go
func fmtHours(h float64) string {
	total := int(h * 60)
	if total < 0 {
		total = 0
	}
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func fmtCount(n int) string { return fmt.Sprintf("%d", n) }

func bindingTarget(b domain.NodeBinding) string {
	if b.RemoteSlug != "" {
		return b.RemoteSlug
	}
	return b.Path
}

// nodeCrumbs builds breadcrumb segments root→leaf from the leaf→root Ancestors
// chain; the leaf (current node) has no Href.
func nodeCrumbs(d NodeCockpit) []components.Crumb {
	var crumbs []components.Crumb
	for i := len(d.Ancestors) - 1; i >= 0; i-- {
		a := d.Ancestors[i]
		if a.ID == d.N.ID {
			crumbs = append(crumbs, components.Crumb{Label: a.Name})
		} else {
			crumbs = append(crumbs, components.Crumb{Href: "/projects/" + a.ID, Label: a.Name})
		}
	}
	if len(crumbs) == 0 { // Ancestors empty (defensive)
		crumbs = append(crumbs, components.Crumb{Label: d.N.Name})
	}
	return crumbs
}
```
(Add `"github.com/serverkraken/flow/internal/adapter/webui/components"` import to `node_tree_vm.go`.)

- [ ] **Step 4** Append cockpit data + handler to `webui_nodes.go` (port `projectWorktime`/`projectCockpitData`, node-aware). Reuse `startOfWeek`/`startOfMonth` (still defined in `webui_projects.go` until deleted; once deleted, move them into `webui_nodes.go`):
```go
import "time"

func (s *Server) nodeWorktime(r *http.Request, u domain.User, n domain.Node) (totalH, weekH, monthH float64, earnings string) {
	ctx := r.Context()
	now := s.Clock.Now()
	since := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := s.ListSessionsRange.Execute(ctx, u.ID, since, now.AddDate(0, 0, 1))
	if err != nil {
		return
	}
	weekStart := startOfWeek(now.Local())
	monthStart := startOfMonth(now.Local())
	var totalDur, weekDur, monthDur time.Duration
	for _, sess := range sessions {
		if sess.NodeID == nil || *sess.NodeID != n.ID || sess.Running() {
			continue
		}
		d := sess.Elapsed(now)
		totalDur += d
		if !sess.Start.Before(weekStart) {
			weekDur += d
		}
		if !sess.Start.Before(monthStart) {
			monthDur += d
		}
	}
	totalH, weekH, monthH = totalDur.Hours(), weekDur.Hours(), monthDur.Hours()
	if n.Rate != nil {
		earnings = n.Rate.Mul(totalDur).String()
	}
	return
}

func (s *Server) nodeCockpitData(r *http.Request, u domain.User, id string) (webui.NodeCockpit, error) {
	n, err := s.GetNode.Execute(r.Context(), u.ID, id)
	if err != nil {
		return webui.NodeCockpit{}, err
	}
	d := webui.NodeCockpit{User: u.Username, N: n}
	d.Ancestors, _ = s.Nodes.Ancestors(r.Context(), u.ID, n.ID)
	if n.Description != "" {
		d.DescriptionHTML = webui.RenderDocument(n.Description, func(string) (string, string, bool) { return "", "", false })
	}
	d.TotalHours, d.WeekHours, d.MonthHours, d.Earnings = s.nodeWorktime(r, u, n)
	nid := n.ID
	d.Docs, _ = s.ListDocuments.Execute(r.Context(), u.ID, &nid, nil)
	d.Bindings, _ = s.ListNodeBindings.ExecuteByNode(r.Context(), u.ID, n.ID)
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	d.MoveTargets = webui.MoveTargetsFor(all, n)
	return d, nil
}

func (s *Server) handleWebNodeView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.nodeCockpitData(r, u, r.PathValue("id"))
	if errors.Is(err, ports.ErrNodeNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.NodeView(d).Render(r.Context(), w)
}
```

- [ ] **Step 5** Route in `server.go`:
```go
mux.Handle("GET /projects/{id}", s.webAuth(http.HandlerFunc(s.handleWebNodeView)))
```

- [ ] **Step 6** Delete `internal/adapter/webui/projects.templ`, `projects_templ.go`, `internal/adapter/httpserver/webui_projects.go`, `webui_projects_test.go`. Confirm `startOfWeek`/`startOfMonth`/`RenderDocument` still resolve (move `startOfWeek`/`startOfMonth` into `webui_nodes.go` if they only lived in `webui_projects.go`). `templ generate`; run `go test ./internal/adapter/httpserver/ -run TestWebNode` → PASS. Commit: `feat(webui): node cockpit (ancestor breadcrumb + read-only bindings + worktime aggregate)`.

---

### Task D7: Node-aware lists — engagement worktime selector + session/doc kind labels

**Files**
- `internal/adapter/httpserver/webui_heute.go` (booking selector → engagements; session row name+kind)
- `internal/adapter/webui/heute.templ` (selector labels) → regen
- `internal/adapter/httpserver/webui_worktime.go` (`resolveWebProject` → engagement-aware)
- `internal/adapter/webui/wissen_vm.go` + `wissen.templ` (project group → node name + kind badge) → regen
- `internal/adapter/httpserver/webui_wissen.go` (`projectNameColorMaps` → node maps incl. kind)
- `internal/adapter/httpserver/webui_heute_test.go` / `webui_wissen_test.go` (assertions)
- `internal/i18n/catalog_de.go`, `catalog_en.go`

**Interfaces**
- `nodeIdentity(nodes []domain.Node, id *string) (name, hue string)` and `nodeKindOf(nodes, id) domain.NodeKind` (replace `projectIdentity`/`projectGlyph`).
- Booking picker `vm.Engagements []components.FuzzyProjectVM` filtered to `Kind==engagement`.

Steps:

- [ ] **Step 1** Add i18n keys. DE:
```go
"heute.bookEngagement": "Engagement buchen",
"heute.orNewEngagement": "oder neues Engagement…",
"sessions.engagement":   "Engagement",
```
EN stubs: `"Book engagement" / "or new engagement…" / "Engagement"`.

- [ ] **Step 2** Failing assertions: in `webui_heute_test.go`, seed an engagement + a repo, start a session, assert the stop selector lists the **engagement** name and **not** the repo; in `webui_wissen_test.go`, seed a `project`-type doc under a repo node and assert the group header shows the node name + the `node.kind.repo` label. Run → FAIL.

- [ ] **Step 3** In `webui_heute.go`:
  - Replace `projects, err := s.ListProjects.Execute(...)` with `nodes, err := s.ListNodes.Execute(...)`.
  - Build `vm.Projects` (keep the field name to avoid templ churn, or rename to `vm.Engagements`) from `nodes` **filtered to `n.Kind == domain.KindEngagement`** — only engagements are bookable per Slice B.
  - Replace `projectIdentity`/`projectGlyph` with `nodeIdentity`/`nodeKindOf` operating on `sess.NodeID` (sessions now carry an engagement id):
```go
func nodeIdentity(nodes []domain.Node, id *string) (string, string) {
	if id == nil {
		return "ohne Engagement", ""
	}
	for _, n := range nodes {
		if n.ID == *id {
			return n.Name, n.Color
		}
	}
	return "ohne Engagement", ""
}
```
  - `sessionRowVM` resolves name/hue via `nodeIdentity`; set `SessionRowVM.Glyph` from the engagement glyph (or `NodeKindStyle(KindEngagement).Glyph`).

- [ ] **Step 4** In `heute.templ`: change the stop/add `<select name="projectId">` label keys to `heute.bookEngagement` / the new-input placeholder to `heute.orNewEngagement`; keep the `name="projectId"` (it is the posted node id = engagement — consistent with REST). Iterate `vm.Projects` (the engagement list).

- [ ] **Step 5** In `webui_worktime.go`, `resolveWebProject` → create a **new engagement** when `newProject` is filled:
```go
func (s *Server) resolveWebNode(r *http.Request, u domain.User) *string {
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if n, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{
			Name: name, Kind: domain.KindEngagement,
		}); err == nil {
			nodeID = n.ID
			s.Bus.Publish(domain.Event{Type: domain.EventNodeCreated, UserID: u.ID})
		}
	}
	if nodeID == "" {
		return nil
	}
	return &nodeID
}
```
Repoint `handleWebAdd`/`handleWebEdit`/stop to `resolveWebNode`; the start/stop usecases (`StartSession`/`AddSession`/`EditSession`) validate `Kind==engagement` (Slice B).

- [ ] **Step 6** In `webui_wissen.go`, rename `projectNameColorMaps` → `nodeMaps` returning `names, colors map[string]string` **plus** `kinds map[string]domain.NodeKind`, sourced from `s.ListNodes`. In `wissen_vm.go`, add `Kind domain.NodeKind` to `ProjectGroup` and thread it through `GroupDocsByCategory` (param `nodeKinds map[string]domain.NodeKind`); in `wissen.templ` `wissenCategoryProjectGroups`/`wissenNotesSection` headers, render `@nodeKindBadge(group.Kind)` next to the name. (Field rename `d.ProjectID`→`d.NodeID` is already done by Slice A's mechanical pass; this step adds the kind badge only.)

- [ ] **Step 7** `templ generate`; run `go test ./internal/adapter/httpserver/ ./internal/adapter/webui/...` → PASS. Commit: `feat(webui): node-aware worktime booking (engagements) + doc-group kind badges`.

---

### Task D8: Final wiring + i18n parity + done-gate

**Files**
- `internal/adapter/httpserver/server.go` (struct fields verification)
- `internal/i18n/catalog_de.go`, `catalog_en.go`
- `web/tailwind.css` / regenerate `app.css` if any new utility classes were introduced (run `make web`)

Steps:

- [ ] **Step 1** Verify the `Server` struct carries every consumed field (D-Interfaces list): `Nodes`, `ListNodes`, `GetNode`, `CreateNode`, `UpdateNode`, `DeleteNode`, `SetNodeRate`, `MoveNode`, `ListNodeBindings` — added by Slices A/C. If any is missing, that is a Slice A/C gap; flag, do not stub.

- [ ] **Step 2** i18n parity: confirm every new `node.*` / `nodes.*` / `heute.bookEngagement` key exists in **both** `catalog_de.go` (full German) and `catalog_en.go` (English stub). If the repo has an i18n drift-guard test, run it; otherwise add a quick check that `len(DE) == len(EN)` for the added block.

- [ ] **Step 3** `make web` (rebuild `app.css`) so any new Tailwind utilities used in `nodes.templ` (e.g. `kindToneClass` tones, indent) are compiled — the `verify-css` guard fails on uncompiled classes. Confirm the tones `border-accent/30 bg-accent/10 text-accent` etc. already exist (used by `wissen.templ`) — they do, so no new utilities beyond standard spacing.

- [ ] **Step 4** Run `make ci` (gofumpt + staticcheck + templ-generated-committed check + tests + coverage gate). Fix any QF1002/lint. Confirm coverage gate stays green.

- [ ] **Step 5** Live curl-smoke vs Postgres+Dex (note `make dev-up`/`dev-token`): `GET /projects` (tree renders engagement→repo indented), `GET /ui/nodes/tree` (fragment 200), `POST /projects` (create engagement, then repo under it), `GET /projects/{id}` (breadcrumb + bindings + worktime), `POST /projects/{id}/move` (reparent), `GET /` (Heute stop-selector lists engagements), `GET /wissen/projekte` (group header shows node + kind badge). Confirm a `node.created`/`node.moved` SSE event re-swaps the tree fragment.

- [ ] **Step 6** Commit: `chore(webui): wire node hierarchy UI + i18n parity + css`. Final `make ci` green.

---

**Notes for the plan integrator**
- The `project*` → node swap (D3–D6) must land as one atomic branch: `projects.templ`/`webui_projects.go` reference `domain.Project` which no longer exists post-Slice-A, so the deletions in D6 and the new files in D3–D6 are interdependent — do not split across PRs.
- User-facing paths stay under `/projects` (nav label `nav.projects` = "Projekte") to avoid nav churn; only the SSE fragment route is renamed (`/ui/projects/list` → `/ui/nodes/tree`) and now triggers on `node.*`.
- WebUI handlers call usecases in-process (not apiclient) — matching the existing project handlers; `apiclient.MoveNode/Ancestors/ResolveEngagement` (Slice C) are for TUI/CLI only.
- Glyph whitelist respected (`◆ ▲ ● ·` for kinds; `→ ✓ ✗ ✚` for actions); no emoji; colors via `kindToneClass`/`ColorHex` only.


---

## Slice E — TUI (node tab tree + engagement picker)

**Assumes Slices A+B+C merged:** `domain.Node`/`NodeKind`/`NodeStatus`, `domain.ValidParentKind`, `domain.ResolveEngagement`; SSE `domain.EventNodeCreated/Updated/Moved/Deleted` (`"node.created/updated/moved/deleted"`); apiclient `ListNodes/GetNode/CreateNode/UpdateNode/MoveNode/DeleteNode/Ancestors/SetNodeRate` plus the field structs `apiclient.CreateNodeFields` and `apiclient.UpdateNodeFields`; `domain.ProjectBinding.NodeID`; session calls already `nodeID`-typed (Slice B). The legacy flat "Projekte" TUI screen still exists (Slice A's mechanical rename of `internal/tui/screen/projects`, still a flat list) and is replaced by this slice.

**Design reconciliation (read before E2/E3):** the tree route renders **kind-colored, indented rows** that `ui/fuzzylist`'s generic renderer cannot draw, so the *tree* fuzzy/kind filtering reuses the same matcher `ui/fuzzymatch` (which `fuzzylist` is itself built on); `ui/fuzzylist` is used **verbatim** for the move parent-picker (E3) and the worktime engagement-picker (E6). Glyph color is driven by `kindcolor.NodeKindColor` (kind is the tree's organizing axis); the per-node `Color`/`Glyph` attributes stay editable in the form and surface in the WebUI.

**New package:** `internal/tui/screen/nodetree` (package `nodetree`) — a fresh directory to avoid any collision with however Slice A renamed `screen/projects`. E7 wires the tab to it and deletes the legacy flat screen.

---

### Task E1: `kindcolor` NodeKind → color/glyph/label mapping

**Files**
- `internal/tui/kindcolor/nodekind.go` (new)
- `internal/tui/kindcolor/nodekind_test.go` (new)

**Interfaces**
- *Consumes (A):* `domain.NodeKind`, `domain.KindEngagement/KindVorhaben/KindRepo/KindBranch`; `theme.Palette`/`theme.Color`/`Sem()`; `glyphs.*`.
- *Produces:* `kindcolor.NodeKindColor(kind domain.NodeKind, p theme.Palette) theme.Color`, `kindcolor.NodeKindGlyph(kind domain.NodeKind) string`, `kindcolor.NodeKindLabel(kind domain.NodeKind) string`.

**Steps**

- [ ] **Step 1** — Write the failing test `internal/tui/kindcolor/nodekind_test.go`:
```go
package kindcolor_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestNodeKind_ColorGlyphLabel(t *testing.T) {
	t.Parallel()
	p := theme.Default
	sem := p.Sem()
	cases := []struct {
		kind  domain.NodeKind
		color theme.Color
		glyph string
		label string
	}{
		{domain.KindEngagement, sem.Accent, "▌", "ENGAGEMENT"},
		{domain.KindVorhaben, sem.Highlight, "◆", "VORHABEN"},
		{domain.KindRepo, sem.Success, "●", "REPO"},
		{domain.KindBranch, sem.Warning, "▪", "BRANCH"},
	}
	for _, c := range cases {
		if got := kindcolor.NodeKindColor(c.kind, p); got != c.color {
			t.Errorf("NodeKindColor(%q) = %q, want %q", c.kind, got, c.color)
		}
		if got := kindcolor.NodeKindGlyph(c.kind); got != c.glyph {
			t.Errorf("NodeKindGlyph(%q) = %q, want %q", c.kind, got, c.glyph)
		}
		if got := kindcolor.NodeKindLabel(c.kind); got != c.label {
			t.Errorf("NodeKindLabel(%q) = %q, want %q", c.kind, got, c.label)
		}
	}
}

func TestNodeKindGlyph_SingleCell(t *testing.T) {
	t.Parallel()
	for _, k := range []domain.NodeKind{
		domain.KindEngagement, domain.KindVorhaben, domain.KindRepo, domain.KindBranch,
	} {
		if w := lipgloss.Width(kindcolor.NodeKindGlyph(k)); w != 1 {
			t.Errorf("NodeKindGlyph(%q) width = %d, want 1 (monospace whitelist)", k, w)
		}
	}
}

func TestNodeKind_UnknownFallback(t *testing.T) {
	t.Parallel()
	p := theme.Default
	if got := kindcolor.NodeKindColor(domain.NodeKind("bogus"), p); got != p.Sem().Border {
		t.Errorf("unknown kind color = %q, want Border", got)
	}
	if got := kindcolor.NodeKindGlyph(domain.NodeKind("bogus")); got != "·" {
		t.Errorf("unknown kind glyph = %q, want ·", got)
	}
}
```
- [ ] **Step 2** — `go test ./internal/tui/kindcolor/` → FAIL (undefined `NodeKindColor/Glyph/Label`).
- [ ] **Step 3** — Implement `internal/tui/kindcolor/nodekind.go`:
```go
package kindcolor

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

// NodeKindColor maps a domain.NodeKind to its semantic hierarchy color. The
// kind is the organizing axis of the node tree, so the color is read from the
// palette's Semantic aliases (never a raw hue). Unknown kinds → neutral border.
func NodeKindColor(kind domain.NodeKind, p theme.Palette) theme.Color {
	sem := p.Sem()
	switch kind {
	case domain.KindEngagement:
		return sem.Accent
	case domain.KindVorhaben:
		return sem.Highlight
	case domain.KindRepo:
		return sem.Success
	case domain.KindBranch:
		return sem.Warning
	default:
		return sem.Border
	}
}

// NodeKindGlyph returns the single-cell identity glyph for a node kind, from the
// monospace whitelist. Distinct shape per kind so the tree is legible without
// color (A11y: shape + color, never color alone).
func NodeKindGlyph(kind domain.NodeKind) string {
	switch kind {
	case domain.KindEngagement:
		return glyphs.BarThick // ▌
	case domain.KindVorhaben:
		return glyphs.Bullet3 // ◆
	case domain.KindRepo:
		return glyphs.Filled // ●
	case domain.KindBranch:
		return glyphs.Bullet4 // ▪
	default:
		return glyphs.BulletDot // ·
	}
}

// NodeKindLabel returns the uppercase badge label for a node kind.
func NodeKindLabel(kind domain.NodeKind) string {
	switch kind {
	case domain.KindEngagement:
		return "ENGAGEMENT"
	case domain.KindVorhaben:
		return "VORHABEN"
	case domain.KindRepo:
		return "REPO"
	case domain.KindBranch:
		return "BRANCH"
	default:
		return "?"
	}
}
```
- [ ] **Step 4** — `go test ./internal/tui/kindcolor/` → PASS; `make ci` green.
- [ ] **Step 5** — Commit: `feat(tui): kindcolor NodeKind→color/glyph/label mapping`.

---

### Task E2: pure tree builders + filters + move candidates

**Files**
- `internal/tui/screen/nodetree/tree.go` (new)
- `internal/tui/screen/nodetree/tree_test.go` (new)

**Interfaces**
- *Consumes (A):* `domain.Node`, `domain.NodeKind`, `domain.ValidParentKind`; `ui/fuzzymatch.Match`.
- *Produces:* `nodetree.Row{Node domain.Node; Depth int}`, `BuildTree([]domain.Node) []Row`, `FilterKind([]Row, domain.NodeKind) []Row`, `FuzzyFilter([]Row, string) []Row`, `MoveCandidates(all []domain.Node, node domain.Node) []domain.Node`.

**Steps**

- [ ] **Step 1** — Write failing `internal/tui/screen/nodetree/tree_test.go`:
```go
package nodetree

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func sp(s string) *string { return &s }

func sample() []domain.Node {
	return []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "e2", Kind: domain.KindEngagement, Name: "RTL Extern"},
		{ID: "r1", ParentID: sp("e1"), Kind: domain.KindRepo, Name: "flow"},
		{ID: "v1", ParentID: sp("e1"), Kind: domain.KindVorhaben, Name: "Buch"},
		{ID: "r2", ParentID: sp("e2"), Kind: domain.KindRepo, Name: "gitlab-x"},
	}
}

func TestBuildTree_PreOrderDepthSorted(t *testing.T) {
	t.Parallel()
	rows := BuildTree(sample())
	// roots name-sorted (Privat < RTL Extern); children name-sorted (Buch < flow).
	want := []struct {
		id    string
		depth int
	}{
		{"e1", 0}, {"v1", 1}, {"r1", 1},
		{"e2", 0}, {"r2", 1},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i].Node.ID != w.id || rows[i].Depth != w.depth {
			t.Errorf("row %d = (%s,%d), want (%s,%d)", i, rows[i].Node.ID, rows[i].Depth, w.id, w.depth)
		}
	}
}

func TestBuildTree_OrphanTreatedAsRoot(t *testing.T) {
	t.Parallel()
	rows := BuildTree([]domain.Node{{ID: "x", ParentID: sp("ghost"), Kind: domain.KindRepo, Name: "x"}})
	if len(rows) != 1 || rows[0].Depth != 0 {
		t.Fatalf("orphan not surfaced as root: %+v", rows)
	}
}

func TestFilterKind_FlattensToZeroDepth(t *testing.T) {
	t.Parallel()
	rows := FilterKind(BuildTree(sample()), domain.KindRepo)
	if len(rows) != 2 {
		t.Fatalf("repos = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Node.Kind != domain.KindRepo || r.Depth != 0 {
			t.Errorf("bad filtered row %+v", r)
		}
	}
	if got := FilterKind(BuildTree(sample()), ""); len(got) != 5 {
		t.Errorf("empty kind must keep all, got %d", len(got))
	}
}

func TestFuzzyFilter_KeepsMatchPlusAncestors(t *testing.T) {
	t.Parallel()
	rows := FuzzyFilter(BuildTree(sample()), "flow")
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.Node.ID] = true
	}
	if !ids["r1"] || !ids["e1"] {
		t.Errorf("fuzzy 'flow' must keep r1 + ancestor e1, got %v", ids)
	}
	if ids["e2"] || ids["r2"] {
		t.Errorf("unrelated subtree must be dropped, got %v", ids)
	}
	if got := FuzzyFilter(rows, "   "); len(got) != len(rows) {
		t.Errorf("blank query must be a no-op")
	}
}

func TestMoveCandidates_KindValidNoCycle(t *testing.T) {
	t.Parallel()
	all := sample()
	var r1 domain.Node
	for _, n := range all {
		if n.ID == "r1" {
			r1 = n
		}
	}
	cands := MoveCandidates(all, r1) // repo: parent ∈ {engagement, vorhaben}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.ID] = true
	}
	if !got["e1"] || !got["e2"] || !got["v1"] {
		t.Errorf("repo candidates must include engagements + vorhaben, got %v", got)
	}
	if got["r1"] || got["r2"] {
		t.Errorf("repos are not valid parents of a repo, got %v", got)
	}
}

func TestMoveCandidates_ExcludesOwnSubtree(t *testing.T) {
	t.Parallel()
	all := []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "E"},
		{ID: "v1", ParentID: sp("e1"), Kind: domain.KindVorhaben, Name: "V"},
		{ID: "v2", ParentID: sp("v1"), Kind: domain.KindVorhaben, Name: "V2"},
	}
	var v1 domain.Node
	for _, n := range all {
		if n.ID == "v1" {
			v1 = n
		}
	}
	for _, c := range MoveCandidates(all, v1) {
		if c.ID == "v2" || c.ID == "v1" {
			t.Errorf("candidate %s is inside v1's subtree (cycle)", c.ID)
		}
	}
}
```
- [ ] **Step 2** — `go test ./internal/tui/screen/nodetree/` → FAIL (undefined builders).
- [ ] **Step 3** — Implement `internal/tui/screen/nodetree/tree.go`:
```go
// Package nodetree is the "Knoten" tab: an indented Engagement→Vorhaben→Repo
// hierarchy tree with kind + fuzzy filters, cursor nav, SSE live reload, and
// in-route move/delete dialogs. detail and form are pushed child routes.
package nodetree

import (
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

// Row is one rendered tree line: a node plus its depth (0 = engagement root).
type Row struct {
	Node  domain.Node
	Depth int
}

// BuildTree flattens nodes in DFS pre-order: each root (parent_id nil, or a
// parent absent from the set) followed by its descendants, every sibling group
// name-sorted (then ID for stability). Pure.
func BuildTree(nodes []domain.Node) []Row {
	present := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		present[n.ID] = true
	}
	byParent := map[string][]domain.Node{}
	for _, n := range nodes {
		key := ""
		if n.ParentID != nil && present[*n.ParentID] {
			key = *n.ParentID
		}
		byParent[key] = append(byParent[key], n)
	}
	for k := range byParent {
		kids := byParent[k]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].Name != kids[j].Name {
				return kids[i].Name < kids[j].Name
			}
			return kids[i].ID < kids[j].ID
		})
		byParent[k] = kids
	}
	var rows []Row
	var walk func(parentKey string, depth int)
	walk = func(parentKey string, depth int) {
		for _, n := range byParent[parentKey] {
			rows = append(rows, Row{Node: n, Depth: depth})
			walk(n.ID, depth+1)
		}
	}
	walk("", 0)
	return rows
}

// FilterKind keeps rows whose node kind == kind; the zero kind keeps all. A
// non-empty kind flattens the result (Depth reset to 0) since ancestors are
// dropped. Pure.
func FilterKind(rows []Row, kind domain.NodeKind) []Row {
	if kind == "" {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Node.Kind == kind {
			out = append(out, Row{Node: r.Node, Depth: 0})
		}
	}
	return out
}

// FuzzyFilter keeps rows whose node name matches query (subsequence, via
// ui/fuzzymatch) PLUS every ancestor of a match, so the tree stays legible.
// Blank query is a no-op. Pure.
func FuzzyFilter(rows []Row, query string) []Row {
	if strings.TrimSpace(query) == "" {
		return rows
	}
	idx := make(map[string]int, len(rows))
	for i, r := range rows {
		idx[r.Node.ID] = i
	}
	keep := map[string]bool{}
	for _, r := range rows {
		if _, _, ok := fuzzymatch.Match(query, r.Node.Name); ok {
			keep[r.Node.ID] = true
			cur := r.Node
			for cur.ParentID != nil {
				j, ok := idx[*cur.ParentID]
				if !ok {
					break
				}
				keep[*cur.ParentID] = true
				cur = rows[j].Node
			}
		}
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if keep[r.Node.ID] {
			out = append(out, r)
		}
	}
	return out
}

// MoveCandidates returns the nodes a node may be reparented under: kind-valid
// (domain.ValidParentKind) and outside the node's own subtree (no cycle). The
// node itself is excluded; result name-sorted. Pure.
func MoveCandidates(all []domain.Node, node domain.Node) []domain.Node {
	inSubtree := map[string]bool{node.ID: true}
	for changed := true; changed; {
		changed = false
		for _, n := range all {
			if n.ParentID != nil && inSubtree[*n.ParentID] && !inSubtree[n.ID] {
				inSubtree[n.ID] = true
				changed = true
			}
		}
	}
	var out []domain.Node
	for _, n := range all {
		if inSubtree[n.ID] {
			continue
		}
		if domain.ValidParentKind(node.Kind, n.Kind) {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
```
- [ ] **Step 4** — `go test ./internal/tui/screen/nodetree/` → PASS; `make ci` green.
- [ ] **Step 5** — Commit: `feat(tui): nodetree pure builders (tree/kind/fuzzy/move-candidates)`.

---

### Task E3: node tree Route — list/filters/cursor + move & delete dialogs

**Files**
- `internal/tui/screen/nodetree/api.go` (new)
- `internal/tui/screen/nodetree/route.go` (new)
- `internal/tui/screen/nodetree/view.go` (new)
- `internal/tui/screen/nodetree/dialogs.go` (new)
- `internal/tui/screen/nodetree/route_test.go` (new)

**Interfaces**
- *Consumes:* `nodetree.{BuildTree,FilterKind,FuzzyFilter,MoveCandidates,Row}` (E2); `kindcolor.NodeKind*` (E1); apiclient `ListNodes/DeleteNode/MoveNode`; `domain.EventNode*`; `shell.{Route,Frame,EventMsg,PushRouteMsg,InputCapturer}`; `ui/{listnav,fuzzylist,confirm,toast,grammar,keyhint,badge,glyphs}`; `theme`.
- *Produces:* `nodetree.TreeAPI`; `nodetree.Route` (implements `shell.Route` + `shell.InputCapturer`); factory setters `SetDetailFactory(func(domain.Node) shell.Route)`, `SetFormFactory(func(*domain.Node) shell.Route)`; `NewRoute(api TreeAPI, pal theme.Palette, user string) *Route`; package-internal `push(shell.Route) tea.Cmd`.

**Steps**

- [ ] **Step 1** — Write failing `internal/tui/screen/nodetree/route_test.go`:
```go
package nodetree

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
)

type fakeTreeAPI struct {
	nodes   []domain.Node
	deleted []string
	moved   []struct{ id, parent string }
	delErr  error
}

func (f *fakeTreeAPI) ListNodes(context.Context) ([]domain.Node, error) { return f.nodes, nil }
func (f *fakeTreeAPI) DeleteNode(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.delErr
}
func (f *fakeTreeAPI) MoveNode(_ context.Context, id string, parentID *string) (domain.Node, error) {
	p := ""
	if parentID != nil {
		p = *parentID
	}
	f.moved = append(f.moved, struct{ id, parent string }{id, p})
	return domain.Node{}, nil
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func loaded(r *Route) {
	r.Update(loadedMsg{nodes: []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "r1", ParentID: sp("e1"), Kind: domain.KindRepo, Name: "flow"},
		{ID: "e2", Kind: domain.KindEngagement, Name: "RTL Extern"},
	}})
}

func TestRoute_LoadBuildsTree(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	if len(r.rows) != 3 || r.rows[0].Node.ID != "e1" || r.rows[1].Node.ID != "r1" {
		t.Fatalf("tree not built pre-order: %+v", r.rows)
	}
}

func TestRoute_CursorJK(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(key('j'))
	if r.cur.Index() != 1 {
		t.Fatalf("j → cursor 1, got %d", r.cur.Index())
	}
	r.Update(key('k'))
	if r.cur.Index() != 0 {
		t.Fatalf("k → cursor 0, got %d", r.cur.Index())
	}
}

func TestRoute_KindFilterCycle(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(key(']')) // alle → Engagements
	if r.kind != domain.KindEngagement || len(r.rows) != 2 {
		t.Fatalf("] → Engagements (2 rows), got kind=%q rows=%d", r.kind, len(r.rows))
	}
}

func TestRoute_FuzzyFilterMode(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	loaded(r)
	r.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !r.filtering || !r.CapturesInput() {
		t.Fatal("/ must enter filtering mode + capture input")
	}
	for _, c := range "flow" {
		r.Update(key(c))
	}
	if r.query != "flow" {
		t.Fatalf("query = %q, want flow", r.query)
	}
	ids := map[string]bool{}
	for _, row := range r.rows {
		ids[row.Node.ID] = true
	}
	if !ids["r1"] || !ids["e1"] || ids["e2"] {
		t.Fatalf("fuzzy rows wrong: %v", ids)
	}
	r.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if r.filtering || r.query != "" {
		t.Fatal("esc must clear + exit filter")
	}
}

func TestRoute_DeleteConfirmCallsAPI(t *testing.T) {
	t.Parallel()
	f := &fakeTreeAPI{}
	r := NewRoute(f, theme.Default, "u")
	loaded(r)
	r.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	if r.dialog != dialogDelete {
		t.Fatal("D must open delete confirm")
	}
	_, cmd := r.Update(confirm.ResultMsg{Confirmed: true})
	if cmd == nil {
		t.Fatal("confirmed delete must return a cmd")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("delete cmd produced no msg")
	}
	if len(f.deleted) != 1 || f.deleted[0] != "e1" {
		t.Fatalf("DeleteNode not called for e1: %v", f.deleted)
	}
}

func TestRoute_MoveDialogCallsAPI(t *testing.T) {
	t.Parallel()
	f := &fakeTreeAPI{}
	r := NewRoute(f, theme.Default, "u")
	loaded(r)
	r.Update(key('j')) // cursor → r1 (repo)
	r.Update(key('m'))
	if r.dialog != dialogMove {
		t.Fatal("m must open move dialog")
	}
	// candidates for repo r1 = engagements e1,e2 (sorted: Privat, RTL Extern)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("move enter must return a cmd")
	}
	cmd()
	if len(f.moved) != 1 || f.moved[0].id != "r1" {
		t.Fatalf("MoveNode not called for r1: %v", f.moved)
	}
}

func TestRoute_EnterPushesDetail(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	var got domain.Node
	r.SetDetailFactory(func(n domain.Node) shell.Route { got = n; return nil })
	loaded(r)
	_, cmd := r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must push detail")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatal("enter cmd must emit PushRouteMsg")
	}
	if got.ID != "e1" {
		t.Fatalf("detail factory got %q, want e1", got.ID)
	}
}

func TestRoute_SSEReload(t *testing.T) {
	t.Parallel()
	r := NewRoute(&fakeTreeAPI{}, theme.Default, "u")
	_, cmd := r.Update(shell.EventMsg{Ev: clientEvent("node.moved")})
	if cmd == nil {
		t.Fatal("node.moved must trigger reload")
	}
}
```
  Add a tiny test helper `clientEvent` (in the same test file) building `apiclient.ClientEvent{Type: t}` — mirror `internal/tui/shell/event_test.go`.
- [ ] **Step 2** — `go test ./internal/tui/screen/nodetree/` → FAIL (undefined `Route`, `loadedMsg`, `dialogDelete`, …).
- [ ] **Step 3** — Implement the four production files.

`api.go`:
```go
package nodetree

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// TreeAPI is the read+mutate surface the tree root needs: list for the tree,
// delete + move for the in-route dialogs. *apiclient.Client satisfies it.
type TreeAPI interface {
	ListNodes(ctx context.Context) ([]domain.Node, error)
	DeleteNode(ctx context.Context, id string) error
	MoveNode(ctx context.Context, id string, parentID *string) (domain.Node, error)
}

var _ TreeAPI = (*apiclient.Client)(nil)
```

`route.go`:
```go
package nodetree

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/listnav"
	"github.com/serverkraken/flow/internal/tui/ui/toast"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogDelete
	dialogMove
)

type loadedMsg struct {
	nodes []domain.Node
	err   error
}
type reloadMsg struct{}

// Route is the "Knoten" tab root. Implements shell.Route + shell.InputCapturer.
type Route struct {
	api  TreeAPI
	pal  theme.Palette
	user string

	all       []domain.Node
	rows      []Row
	cur       listnav.Cursor
	kind      domain.NodeKind // "" = all
	query     string
	filtering bool

	loaded bool
	err    error
	toast  toast.Model

	dialog  dialogKind
	confirm confirm.Model
	delID   string
	move    moveState

	detailFor func(domain.Node) shell.Route
	formFor   func(*domain.Node) shell.Route // nil ptr → create; non-nil → edit
}

func NewRoute(api TreeAPI, pal theme.Palette, user string) *Route {
	return &Route{api: api, pal: pal, user: user, cur: listnav.New()}
}

func (r *Route) SetDetailFactory(f func(domain.Node) shell.Route) { r.detailFor = f }
func (r *Route) SetFormFactory(f func(*domain.Node) shell.Route)  { r.formFor = f }

func (r *Route) Title() string { return "Knoten" }
func (r *Route) Init() tea.Cmd { return r.loadCmd() }

func (r *Route) loadCmd() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ns, err := api.ListNodes(ctx)
		return loadedMsg{nodes: ns, err: err}
	}
}

func (r *Route) rebuild() {
	rows := BuildTree(r.all)
	rows = FilterKind(rows, r.kind)
	rows = FuzzyFilter(rows, r.query)
	r.rows = rows
	r.cur = r.cur.Clamp(len(r.rows))
}

func (r *Route) selected() (domain.Node, bool) {
	i := r.cur.Index()
	if i >= 0 && i < len(r.rows) {
		return r.rows[i].Node, true
	}
	return domain.Node{}, false
}

func (r *Route) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case loadedMsg:
		r.loaded = true
		r.all, r.err = m.nodes, m.err
		r.rebuild()
		return r, nil
	case reloadMsg:
		return r, r.loadCmd()
	case shell.EventMsg:
		if isNodeEvent(m.Ev.Type) {
			return r, r.loadCmd()
		}
		return r, nil
	case toast.DismissedMsg:
		r.toast, _ = r.toast.Update(m)
		return r, nil
	case confirm.ResultMsg:
		open := r.dialog == dialogDelete
		r.dialog = dialogNone
		if open && m.Confirmed && r.delID != "" {
			id := r.delID
			api := r.api
			return r, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := api.DeleteNode(ctx, id); err != nil {
					return deleteErrMsg{err}
				}
				return reloadMsg{}
			}
		}
		return r, nil
	case deleteErrMsg:
		r.toast = toast.NewDanger(deleteErrText(m.err), r.pal)
		return r, r.toast.Init()
	case moveDoneMsg:
		if m.err != nil {
			r.toast = toast.NewDanger("Verschieben: "+m.err.Error(), r.pal)
			return r, r.toast.Init()
		}
		return r, r.loadCmd()
	case tea.KeyPressMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *Route) handleKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	if r.dialog != dialogNone {
		return r.handleDialogKey(k)
	}
	if r.filtering {
		return r.handleFilterKey(k)
	}
	if cur, ok := r.cur.Handle(k, len(r.rows), 5); ok { // arrows / Home / End / PgUp-Dn
		r.cur = cur
		return r, nil
	}
	switch {
	case k.Text == "j":
		r.cur = r.cur.Set(r.cur.Index()+1, len(r.rows))
		return r, nil
	case k.Text == "k":
		r.cur = r.cur.Set(r.cur.Index()-1, len(r.rows))
		return r, nil
	case grammar.WeekNext.Matches(k): // ]
		r.kind = nextKind(r.kind, +1)
		r.rebuild()
		return r, nil
	case grammar.WeekPrev.Matches(k): // [
		r.kind = nextKind(r.kind, -1)
		r.rebuild()
		return r, nil
	case grammar.Search.Matches(k): // /
		r.filtering = true
		return r, nil
	case grammar.New.Matches(k): // n
		if r.formFor != nil {
			return r, push(r.formFor(nil))
		}
		return r, nil
	case grammar.Edit.Matches(k): // e
		if n, ok := r.selected(); ok && r.formFor != nil {
			cp := n
			return r, push(r.formFor(&cp))
		}
		return r, nil
	case k.Text == "m":
		return r.openMove()
	case k.Text == "D":
		return r.openDelete()
	case grammar.Open.Matches(k): // enter
		if n, ok := r.selected(); ok && r.detailFor != nil {
			return r, push(r.detailFor(n))
		}
		return r, nil
	}
	return r, nil
}

func (r *Route) handleFilterKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.filtering = false
		r.query = ""
		r.rebuild()
		return r, nil
	case tea.KeyEnter:
		r.filtering = false
		if n, ok := r.selected(); ok && r.detailFor != nil {
			return r, push(r.detailFor(n))
		}
		return r, nil
	case tea.KeyBackspace:
		if rn := []rune(r.query); len(rn) > 0 {
			r.query = string(rn[:len(rn)-1])
		}
		r.rebuild()
		return r, nil
	}
	if cur, ok := r.cur.Handle(k, len(r.rows), 5); ok { // arrows only (no printable runes)
		r.cur = cur
		return r, nil
	}
	if k.Text != "" {
		r.query += k.Text
		r.rebuild()
	}
	return r, nil
}

// CapturesInput implements shell.InputCapturer — own the keyboard while a dialog
// is open or while typing a fuzzy filter.
func (r *Route) CapturesInput() bool { return r.dialog != dialogNone || r.filtering }

func (r *Route) View(f shell.Frame) string { return renderView(r, f) }

func (r *Route) KeyHints() []keyhint.Hint {
	if r.dialog == dialogDelete {
		return []keyhint.Hint{{Key: "y", Desc: "löschen"}, {Key: "n", Desc: "abbrechen"}}
	}
	if r.dialog == dialogMove {
		return []keyhint.Hint{{Key: "↑/↓", Desc: "Ziel"}, {Key: "enter", Desc: "verschieben"}, {Key: "esc", Desc: "abbrechen"}}
	}
	if r.filtering {
		return []keyhint.Hint{{Key: "tippen", Desc: "filtern"}, {Key: "enter", Desc: "öffnen"}, {Key: "esc", Desc: "abbrechen"}}
	}
	return []keyhint.Hint{
		grammar.Open.Hint(),
		grammar.New.Hint(),
		grammar.Edit.Hint(),
		{Key: "m", Desc: "verschieben"},
		{Key: "D", Desc: "löschen"},
		grammar.Search.Hint(),
		{Key: "[ ]", Desc: "Filter"},
		grammar.MoveUp.Hint(),
	}
}

func push(child shell.Route) tea.Cmd {
	return func() tea.Msg { return shell.PushRouteMsg{Route: child} }
}

func isNodeEvent(t string) bool {
	switch domain.EventType(t) {
	case domain.EventNodeCreated, domain.EventNodeUpdated, domain.EventNodeMoved, domain.EventNodeDeleted:
		return true
	}
	return false
}

var kindCycle = []domain.NodeKind{"", domain.KindEngagement, domain.KindVorhaben, domain.KindRepo}

func nextKind(cur domain.NodeKind, dir int) domain.NodeKind {
	i := 0
	for j, k := range kindCycle {
		if k == cur {
			i = j
			break
		}
	}
	return kindCycle[(i+dir+len(kindCycle))%len(kindCycle)]
}

func kindFilterLabel(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "Engagements"
	case domain.KindVorhaben:
		return "Vorhaben"
	case domain.KindRepo:
		return "Repos"
	default:
		return "alle"
	}
}
```

`dialogs.go`:
```go
package nodetree

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/confirm"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

// ---- delete -----------------------------------------------------------------

type deleteErrMsg struct{ err error }

func (r *Route) openDelete() (shell.Route, tea.Cmd) {
	n, ok := r.selected()
	if !ok {
		return r, nil
	}
	r.delID = n.ID
	r.confirm = confirm.NewDanger("Knoten löschen?", n.Name, r.pal)
	r.dialog = dialogDelete
	return r, nil
}

// deleteErrText surfaces ports.ErrNodeHasChildren (RESTRICT) as a clear hint.
func deleteErrText(err error) string {
	if err != nil && strings.Contains(err.Error(), "children") {
		return "Knoten hat Unterknoten — erst leeren oder umhängen"
	}
	if err != nil {
		return "Löschen fehlgeschlagen: " + err.Error()
	}
	return "Löschen fehlgeschlagen"
}

// ---- move (reparent) --------------------------------------------------------

type moveState struct {
	node  domain.Node
	list  fuzzylist.Model
	cands []domain.Node
}

type moveDoneMsg struct{ err error }

func (r *Route) openMove() (shell.Route, tea.Cmd) {
	n, ok := r.selected()
	if !ok {
		return r, nil
	}
	cands := MoveCandidates(r.all, n)
	r.move = moveState{node: n, cands: cands, list: fuzzylist.New(candItems(cands), r.pal)}
	r.dialog = dialogMove
	return r, nil
}

func candItems(ns []domain.Node) []fuzzylist.Item {
	out := make([]fuzzylist.Item, 0, len(ns))
	for _, n := range ns {
		out = append(out, fuzzylist.Item{ID: n.ID, Label: n.Name + " (" + string(n.Kind) + ")"})
	}
	return out
}

func (r *Route) handleDialogKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch r.dialog {
	case dialogDelete:
		m, cmd := r.confirm.Update(k)
		r.confirm = m
		return r, cmd
	case dialogMove:
		return r.handleMoveKey(k)
	}
	return r, nil
}

func (r *Route) handleMoveKey(k tea.KeyPressMsg) (shell.Route, tea.Cmd) {
	switch k.Code {
	case tea.KeyEsc:
		r.dialog = dialogNone
		return r, nil
	case tea.KeyEnter:
		it, _, ok := r.move.list.Selection()
		if !ok {
			r.dialog = dialogNone
			return r, nil
		}
		id, parent, api := r.move.node.ID, it.ID, r.api
		r.dialog = dialogNone
		return r, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := api.MoveNode(ctx, id, &parent); err != nil {
				return moveDoneMsg{err}
			}
			return moveDoneMsg{}
		}
	default:
		r.move.list = r.move.list.Update(k)
		return r, nil
	}
}

func (r *Route) renderMove(f shell.Frame) string {
	var b strings.Builder
	b.WriteString("\n  „" + r.move.node.Name + "" verschieben unter …  ")
	b.WriteString(theme.Dim("tippen → filtern · ↑/↓ → wählen · enter → verschieben · esc", f.Pal))
	b.WriteString("\n\n")
	if len(r.move.cands) == 0 {
		b.WriteString(theme.Dim("  Keine gültigen Ziele für diesen Knotentyp.", f.Pal))
		return b.String()
	}
	b.WriteString(r.move.list.View(f.Width - 4))
	return b.String()
}
```

`view.go`:
```go
package nodetree

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

func renderView(r *Route, f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	if !r.loaded {
		return theme.Dim("  Knoten lädt …", pal)
	}
	if r.err != nil {
		return theme.Err("  Fehler: "+r.err.Error(), pal)
	}
	if r.dialog == dialogDelete {
		return r.confirm.View()
	}
	if r.dialog == dialogMove {
		return r.renderMove(f)
	}

	var b strings.Builder
	// header: kind filter + (optional) fuzzy query.
	head := "  Filter: " + kindFilterLabel(r.kind) + "  " +
		lipgloss.NewStyle().Foreground(pal.FgMuted).Render("([ ] Typ · / suchen)")
	b.WriteString(lipgloss.NewStyle().Foreground(pal.FgMuted).Render(head) + "\n")
	if r.filtering || r.query != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.Sem().Accent).Render("/"+r.query) + "\n")
	}
	b.WriteString("\n")

	if len(r.rows) == 0 {
		b.WriteString(theme.Dim("  Keine Knoten.", pal))
		b.WriteString(r.toast.View())
		return b.String()
	}

	selBar := lipgloss.NewStyle().Foreground(pal.Sem().Accent)
	for i, row := range r.rows {
		selected := i == r.cur.Index()
		bar := " "
		if selected {
			bar = selBar.Render(glyphs.AccentBar)
		}
		indent := strings.Repeat("  ", row.Depth)
		glyph := row.Node.Glyph
		if glyph == "" {
			glyph = kindcolor.NodeKindGlyph(row.Node.Kind)
		}
		gStr := lipgloss.NewStyle().Foreground(kindcolor.NodeKindColor(row.Node.Kind, pal)).Render(glyph)
		nameStyle := lipgloss.NewStyle().Foreground(pal.Fg)
		if selected {
			nameStyle = nameStyle.Bold(true)
		}
		kindTag := theme.Dim(kindcolor.NodeKindLabel(row.Node.Kind), pal)
		status := ""
		if row.Node.Status == domain.NodeArchived {
			status = "  " + theme.Dim("[archiviert]", pal)
		} else if row.Node.Status == domain.NodePaused {
			status = "  " + theme.Dim("[pausiert]", pal)
		}
		b.WriteString("  " + bar + " " + indent + gStr + " " + nameStyle.Render(row.Node.Name) +
			"  " + kindTag + status + "\n")
	}
	b.WriteString(r.toast.View())
	return b.String()
}
```
- [ ] **Step 4** — `go test ./internal/tui/screen/nodetree/` → PASS; `make ci` green (run lint — watch QF1002).
- [ ] **Step 5** — Commit: `feat(tui): nodetree Route (tree/filters/cursor + move & delete dialogs)`.

---

### Task E4: node detail cockpit Route

**Files**
- `internal/tui/screen/nodetree/detail.go` (new)
- `internal/tui/screen/nodetree/detailview.go` (new)
- `internal/tui/screen/nodetree/detail_test.go` (new)

**Interfaces**
- *Consumes:* apiclient `GetNode`, `Ancestors`, `ListDocumentsScoped(ctx, nodeID *string, tags ...string)`, `ListBindings`; `domain.{Node,Money,ProjectBinding,BindingRemote,BindingPath,EventNodeUpdated}`; `kindcolor.NodeKind*`; `ui/badge`; `theme`.
- *Produces:* `nodetree.DetailAPI`; `nodetree.DetailRoute` (implements `shell.Route`); `NewDetailRoute(api DetailAPI, pal theme.Palette, n domain.Node) *DetailRoute`; `SetFormFactory(func(*domain.Node) shell.Route)`.

**Steps**

- [ ] **Step 1** — Write failing `detail_test.go`:
```go
package nodetree

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDetailAPI struct {
	node  domain.Node
	chain []domain.Node
	docs  []domain.Document
	binds []domain.ProjectBinding
}

func (f *fakeDetailAPI) GetNode(_ context.Context, _ string) (domain.Node, error) { return f.node, nil }
func (f *fakeDetailAPI) Ancestors(_ context.Context, _ string) ([]domain.Node, error) {
	return f.chain, nil
}
func (f *fakeDetailAPI) ListDocumentsScoped(_ context.Context, _ *string, _ ...string) ([]domain.Document, error) {
	return f.docs, nil
}
func (f *fakeDetailAPI) ListBindings(_ context.Context) ([]domain.ProjectBinding, error) {
	return f.binds, nil
}

func TestDetail_RendersCockpit(t *testing.T) {
	t.Parallel()
	rate := domain.Money{Amount: 9500, Currency: "EUR"}
	f := &fakeDetailAPI{
		node:  domain.Node{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern", Rate: &rate},
		chain: []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern"}},
		docs:  []domain.Document{{ID: "d1", Title: "Spec"}},
	}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.Update(detailLoadedMsg{node: f.node, chain: f.chain, docs: f.docs})
	out := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	for _, want := range []string{"RTL Extern", "ENGAGEMENT", "95.00 EUR", "Dokumente (1)"} {
		if !contains(out, want) {
			t.Errorf("cockpit missing %q in:\n%s", want, out)
		}
	}
}

func TestDetail_BreadcrumbRootToLeaf(t *testing.T) {
	t.Parallel()
	f := &fakeDetailAPI{
		node: domain.Node{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
		// Ancestors returns leaf→root; view must render root→leaf.
		chain: []domain.Node{
			{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
			{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		},
	}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.Update(detailLoadedMsg{node: f.node, chain: f.chain})
	out := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !contains(out, "Privat › flow") {
		t.Errorf("breadcrumb wrong:\n%s", out)
	}
}

func TestDetail_EditPushesForm(t *testing.T) {
	t.Parallel()
	f := &fakeDetailAPI{node: domain.Node{ID: "e1", Kind: domain.KindEngagement, Name: "E"}}
	r := NewDetailRoute(f, theme.Default, f.node)
	r.SetFormFactory(func(*domain.Node) shell.Route { return nil })
	_, cmd := r.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("e must push edit form")
	}
	if _, ok := cmd().(shell.PushRouteMsg); !ok {
		t.Fatal("e cmd must emit PushRouteMsg")
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf2(s, sub) >= 0) }
func indexOf2(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```
- [ ] **Step 2** — `go test ./internal/tui/screen/nodetree/` → FAIL.
- [ ] **Step 3** — Implement `detail.go`:
```go
package nodetree

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// DetailAPI is the read surface the cockpit needs. *apiclient.Client satisfies it.
type DetailAPI interface {
	GetNode(ctx context.Context, id string) (domain.Node, error)
	Ancestors(ctx context.Context, id string) ([]domain.Node, error)
	ListDocumentsScoped(ctx context.Context, nodeID *string, tags ...string) ([]domain.Document, error)
	ListBindings(ctx context.Context) ([]domain.ProjectBinding, error)
}

var _ DetailAPI = (*apiclient.Client)(nil)

type detailLoadedMsg struct {
	node  domain.Node
	chain []domain.Node // leaf→root, as Ancestors returns
	docs  []domain.Document
	binds []domain.ProjectBinding
}

// DetailRoute is the node cockpit: name, kind badge, ancestor breadcrumb, rate
// (engagement), read-only bindings, assigned-docs count. Implements shell.Route.
type DetailRoute struct {
	api     DetailAPI
	pal     theme.Palette
	n       domain.Node
	data    detailLoadedMsg
	formFor func(*domain.Node) shell.Route
}

func NewDetailRoute(api DetailAPI, pal theme.Palette, n domain.Node) *DetailRoute {
	return &DetailRoute{api: api, pal: pal, n: n}
}

func (r *DetailRoute) SetFormFactory(f func(*domain.Node) shell.Route) { r.formFor = f }

func (r *DetailRoute) Title() string { return r.n.Name }
func (r *DetailRoute) Init() tea.Cmd { return r.loadCmd() }

func (r *DetailRoute) loadCmd() tea.Cmd {
	api, n := r.api, r.n
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		fresh, err := api.GetNode(ctx, n.ID)
		if err == nil {
			n = fresh
		}
		chain, _ := api.Ancestors(ctx, n.ID)
		id := n.ID
		docs, _ := api.ListDocumentsScoped(ctx, &id)
		allBinds, _ := api.ListBindings(ctx)
		var binds []domain.ProjectBinding
		for _, b := range allBinds {
			if b.NodeID == n.ID {
				binds = append(binds, b)
			}
		}
		return detailLoadedMsg{node: n, chain: chain, docs: docs, binds: binds}
	}
}

func (r *DetailRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case detailLoadedMsg:
		r.n, r.data = m.node, m
		return r, nil
	case shell.EventMsg:
		if m.Ev.Type == string(domain.EventNodeUpdated) || m.Ev.Type == string(domain.EventNodeMoved) {
			return r, r.loadCmd()
		}
		return r, nil
	case tea.KeyPressMsg:
		if grammar.Edit.Matches(m) && r.formFor != nil {
			cp := r.n
			return r, push(r.formFor(&cp))
		}
	}
	return r, nil
}

func (r *DetailRoute) View(f shell.Frame) string { return renderDetailView(r, f) }

func (r *DetailRoute) KeyHints() []keyhint.Hint {
	return []keyhint.Hint{grammar.Edit.Hint(), grammar.Back.Hint()}
}
```
  Implement `detailview.go`:
```go
package nodetree

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/kindcolor"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/badge"
)

func renderDetailView(r *DetailRoute, f shell.Frame) string {
	pal := f.Pal
	if pal.Bg == "" {
		pal = r.pal
	}
	var b strings.Builder

	// header: kind glyph + name + kind badge.
	glyph := lipgloss.NewStyle().
		Foreground(kindcolor.NodeKindColor(r.n.Kind, pal)).
		Render(kindcolor.NodeKindGlyph(r.n.Kind))
	name := lipgloss.NewStyle().Foreground(pal.Fg).Bold(true).Render(r.n.Name)
	kb := badge.Render(kindcolor.NodeKindLabel(r.n.Kind), kindcolor.NodeKindColor(r.n.Kind, pal), pal)
	b.WriteString("  " + glyph + " " + name + "  " + kb + "\n\n")

	// breadcrumb (root→leaf) from the leaf→root chain.
	if len(r.data.chain) > 0 {
		names := make([]string, 0, len(r.data.chain))
		for i := len(r.data.chain) - 1; i >= 0; i-- {
			names = append(names, r.data.chain[i].Name)
		}
		b.WriteString("  " + theme.Dim(strings.Join(names, " › "), pal) + "\n\n")
	}

	// description.
	if r.n.Description != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.Fg).Render(r.n.Description) + "\n\n")
	}

	// rate — engagement only.
	if r.n.Kind == domain.KindEngagement {
		lbl := lipgloss.NewStyle().Foreground(pal.FgMuted).Render("Satz: ")
		if r.n.Rate != nil {
			b.WriteString("  " + lbl + lipgloss.NewStyle().Foreground(pal.Sem().Success).Render(r.n.Rate.String()) + "\n\n")
		} else {
			b.WriteString("  " + lbl + theme.Dim("kein Satz", pal) + "\n\n")
		}
	}

	// upstream git.
	if r.n.UpstreamGit != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).Render("Git: ") +
			lipgloss.NewStyle().Foreground(pal.Fg).Render(r.n.UpstreamGit) + "\n\n")
	}

	// bindings (read-only).
	if len(r.data.binds) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).
			Render(fmt.Sprintf("Bindings (%d):", len(r.data.binds))) + "\n")
		for _, bd := range r.data.binds {
			b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).
				Render(string(bd.Kind)+": "+bindingTarget(bd)) + "\n")
		}
		b.WriteString("\n")
	}

	// assigned docs count.
	b.WriteString("  " + lipgloss.NewStyle().Foreground(pal.FgMuted).
		Render(fmt.Sprintf("Dokumente (%d):", len(r.data.docs))) + "\n")
	for _, d := range r.data.docs {
		title := d.Title
		if title == "" {
			title = d.Path
		}
		b.WriteString("    " + lipgloss.NewStyle().Foreground(pal.Fg).Render(title) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("  " + theme.Dim("e Bearbeiten · q Zurück", pal) + "\n")
	return b.String()
}

func bindingTarget(b domain.ProjectBinding) string {
	switch b.Kind {
	case domain.BindingRemote:
		return b.RemoteSlug
	case domain.BindingPath:
		return b.Path
	default:
		return string(b.Kind)
	}
}
```
- [ ] **Step 4** — `go test ./internal/tui/screen/nodetree/` → PASS; `make ci` green.
- [ ] **Step 5** — Commit: `feat(tui): nodetree detail cockpit (breadcrumb, rate, bindings, docs)`.

---

### Task E5: node create/edit form Route

**Files**
- `internal/tui/screen/nodetree/form.go` (new)
- `internal/tui/screen/nodetree/formview.go` (new)
- `internal/tui/screen/nodetree/form_test.go` (new)

**Interfaces**
- *Consumes:* apiclient `ListNodes`, `CreateNode(ctx, apiclient.CreateNodeFields)`, `UpdateNode(ctx, id, apiclient.UpdateNodeFields)`, `SetNodeRate(ctx, id, *int64, currency)`; `domain.{Node,NodeKind,NodeColors,NodeGlyphs,ValidParentKind,KindEngagement}`; `ui/form.NewTextInput`; `shell.{Route,TextCapturer,InputCapturer,PopRouteMsg}`.
  - Expected Slice-C field structs: `apiclient.CreateNodeFields{Name, Kind, Color, Glyph, Description, UpstreamGit string; ParentID *string}`; `apiclient.UpdateNodeFields{Name, Slug, Color, Glyph, Description, UpstreamGit, Status string}` (no Kind/Parent/Rate — kind immutable, parent via move, rate via SetNodeRate).
- *Produces:* `nodetree.FormAPI`; `nodetree.FormRoute` (implements `shell.Route` + `shell.InputCapturer` + `shell.TextCapturer`); `NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Node) *FormRoute`; test seams `FormValues`, `Values()`, `FillForTest(FormValues)`, `Submit()`; type aliases `CreateFields = apiclient.CreateNodeFields`, `UpdateFields = apiclient.UpdateNodeFields`.

**Steps**

- [ ] **Step 1** — Write failing `form_test.go`:
```go
package nodetree

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeFormAPI struct {
	nodes   []domain.Node
	created CreateFields
	updated UpdateFields
	updID   string
	rateAmt *int64
	rateCur string
	createN domain.Node
}

func (f *fakeFormAPI) ListNodes(context.Context) ([]domain.Node, error) { return f.nodes, nil }
func (f *fakeFormAPI) CreateNode(_ context.Context, in CreateFields) (domain.Node, error) {
	f.created = in
	if f.createN.ID == "" {
		f.createN = domain.Node{ID: "new1", Name: in.Name, Kind: domain.NodeKind(in.Kind)}
	}
	return f.createN, nil
}
func (f *fakeFormAPI) UpdateNode(_ context.Context, id string, in UpdateFields) (domain.Node, error) {
	f.updID, f.updated = id, in
	return domain.Node{ID: id}, nil
}
func (f *fakeFormAPI) SetNodeRate(_ context.Context, _ string, amount *int64, cur string) error {
	f.rateAmt, f.rateCur = amount, cur
	return nil
}

func TestForm_CreateRepoUnderParent(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{nodes: []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"}}}
	r := NewFormRoute(f, theme.Default, nil)
	r.Update(nodesLoadedMsg{nodes: f.nodes})
	r.FillForTest(FormValues{Name: "flow", Kind: string(domain.KindRepo), ParentID: "e1"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("valid create must return a cmd")
	}
	cmd()
	if f.created.Name != "flow" || f.created.Kind != string(domain.KindRepo) {
		t.Fatalf("CreateNode fields wrong: %+v", f.created)
	}
	if f.created.ParentID == nil || *f.created.ParentID != "e1" {
		t.Fatalf("ParentID = %v, want e1", f.created.ParentID)
	}
}

func TestForm_CreateEngagementWithRate(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{}
	r := NewFormRoute(f, theme.Default, nil)
	r.Update(nodesLoadedMsg{})
	r.FillForTest(FormValues{Name: "RTL", Kind: string(domain.KindEngagement), RateAmount: "95", RateCurrency: "EUR"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("valid create must return cmd")
	}
	cmd()
	if f.created.ParentID != nil {
		t.Fatalf("engagement must be root, ParentID=%v", f.created.ParentID)
	}
	if f.rateAmt == nil || *f.rateAmt != 9500 || f.rateCur != "EUR" {
		t.Fatalf("rate not set: amt=%v cur=%q", f.rateAmt, f.rateCur)
	}
}

func TestForm_NonEngagementNeedsParent(t *testing.T) {
	t.Parallel()
	r := NewFormRoute(&fakeFormAPI{}, theme.Default, nil)
	r.FillForTest(FormValues{Name: "x", Kind: string(domain.KindRepo)}) // no parent
	if _, cmd := r.Submit(); cmd != nil {
		t.Fatal("repo without parent must fail validation")
	}
	if r.err == "" {
		t.Fatal("expected validation error")
	}
}

func TestForm_EditUpdatesMetadata(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{}
	editing := domain.Node{ID: "r1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", Status: domain.NodeActive}
	r := NewFormRoute(f, theme.Default, &editing)
	r.FillForTest(FormValues{Name: "flow2", Slug: "flow", Status: "active"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("edit must return cmd")
	}
	cmd()
	if f.updID != "r1" || f.updated.Name != "flow2" {
		t.Fatalf("UpdateNode wrong: id=%q %+v", f.updID, f.updated)
	}
	if f.created.Name != "" {
		t.Fatal("edit must not call CreateNode")
	}
}
```
- [ ] **Step 2** — `go test ./internal/tui/screen/nodetree/` → FAIL.
- [ ] **Step 3** — Implement `form.go`:
```go
package nodetree

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/form"
)

// CreateFields / UpdateFields alias the Slice-C apiclient field structs so this
// package and its tests reference them without importing apiclient directly.
type (
	CreateFields = apiclient.CreateNodeFields
	UpdateFields = apiclient.UpdateNodeFields
)

// FormAPI is the write surface the form needs. *apiclient.Client satisfies it.
type FormAPI interface {
	ListNodes(ctx context.Context) ([]domain.Node, error)
	CreateNode(ctx context.Context, in CreateFields) (domain.Node, error)
	UpdateNode(ctx context.Context, id string, in UpdateFields) (domain.Node, error)
	SetNodeRate(ctx context.Context, id string, amount *int64, currency string) error
}

var (
	_ FormAPI            = (*apiclient.Client)(nil)
	_ shell.TextCapturer = (*FormRoute)(nil)
)

type (
	formErrMsg     struct{ err string }
	nodesLoadedMsg struct {
		nodes []domain.Node
		err   error
	}
)

// FormValues is the exported snapshot of form fields (test seam).
type FormValues struct {
	Name, Slug, Description, UpstreamGit string
	Kind, ParentID, Status, Color, Glyph string
	RateAmount, RateCurrency             string
}

// focus order. Kind+Parent only matter in create mode; Rate only for engagement.
const (
	focusName int = iota
	focusSlug
	focusDescription
	focusUpstream
	focusKind
	focusParent
	focusStatus
	focusColor
	focusGlyph
	focusRateAmount
	focusRateCurrency
	focusCount
)

var textInputIdx = map[int]int{
	focusName: 0, focusSlug: 1, focusDescription: 2, focusUpstream: 3,
	focusRateAmount: 4, focusRateCurrency: 5,
}

var (
	kindChoices   = []string{string(domain.KindEngagement), string(domain.KindVorhaben), string(domain.KindRepo)}
	statusChoices = []string{"active", "paused", "archived"}
	colorChoices  = append([]string{""}, domain.NodeColors...)
	glyphChoices  = append([]string{""}, domain.NodeGlyphs...)
)

// FormRoute is the node create/edit form. Owns all keys while active.
type FormRoute struct {
	api     FormAPI
	pal     theme.Palette
	editing *domain.Node // nil = create

	inputs   []textinput.Model // Name, Slug, Desc, Upstream, RateAmount, RateCurrency
	focusIdx int
	kindIx   int
	statusIx int
	colorIx  int
	glyphIx  int

	allNodes    []domain.Node
	parentIDs   []string // [""] + valid parent ids for current kind
	parentLabel []string
	parentIx    int

	err string
}

func NewFormRoute(api FormAPI, pal theme.Palette, editing *domain.Node) *FormRoute {
	r := &FormRoute{api: api, pal: pal, editing: editing}
	ph := []string{"Name", "slug", "Beschreibung", "https://…", "0.00", "EUR"}
	r.inputs = make([]textinput.Model, len(ph))
	for i, p := range ph {
		r.inputs[i] = form.NewTextInput(p, pal)
	}
	_ = r.inputs[0].Focus()
	r.statusIx = indexOf(statusChoices, "active")
	if editing != nil {
		r.inputs[0].SetValue(editing.Name)
		r.inputs[1].SetValue(editing.Slug)
		r.inputs[2].SetValue(editing.Description)
		r.inputs[3].SetValue(editing.UpstreamGit)
		r.kindIx = indexOf(kindChoices, string(editing.Kind))
		r.statusIx = indexOf(statusChoices, string(editing.Status))
		r.colorIx = indexOf(colorChoices, editing.Color)
		r.glyphIx = indexOf(glyphChoices, editing.Glyph)
		if editing.Rate != nil {
			r.inputs[4].SetValue(fmt.Sprintf("%d.%02d", editing.Rate.Amount/100, editing.Rate.Amount%100))
			r.inputs[5].SetValue(editing.Rate.Currency)
		}
	} else {
		r.inputs[5].SetValue("EUR")
	}
	r.recomputeParents()
	return r
}

func (r *FormRoute) currentKind() domain.NodeKind {
	if r.editing != nil {
		return r.editing.Kind
	}
	return domain.NodeKind(kindChoices[r.kindIx])
}

// recomputeParents rebuilds the parent selector from allNodes for the current
// kind. Engagements are always roots ("(Wurzel)" only).
func (r *FormRoute) recomputeParents() {
	r.parentIDs = []string{""}
	r.parentLabel = []string{"(Wurzel / keine)"}
	if r.currentKind() == domain.KindEngagement {
		r.parentIx = 0
		return
	}
	kind := r.currentKind()
	for _, n := range r.allNodes {
		if domain.ValidParentKind(kind, n.Kind) {
			r.parentIDs = append(r.parentIDs, n.ID)
			r.parentLabel = append(r.parentLabel, n.Name+" ("+string(n.Kind)+")")
		}
	}
	if r.parentIx >= len(r.parentIDs) {
		r.parentIx = 0
	}
}

func (r *FormRoute) CapturesInput() bool { return true }
func (r *FormRoute) CapturesText() bool  { return true }

func (r *FormRoute) Title() string {
	if r.editing != nil {
		return "Knoten bearbeiten"
	}
	return "Neuer Knoten"
}

func (r *FormRoute) Init() tea.Cmd {
	api := r.api
	return func() tea.Msg {
		ns, err := api.ListNodes(context.Background())
		return nodesLoadedMsg{nodes: ns, err: err}
	}
}

func (r *FormRoute) Values() FormValues {
	pid := ""
	if r.parentIx >= 0 && r.parentIx < len(r.parentIDs) {
		pid = r.parentIDs[r.parentIx]
	}
	return FormValues{
		Name: r.inputs[0].Value(), Slug: r.inputs[1].Value(),
		Description: r.inputs[2].Value(), UpstreamGit: r.inputs[3].Value(),
		Kind: kindChoices[r.kindIx], ParentID: pid, Status: statusChoices[r.statusIx],
		Color: colorChoices[r.colorIx], Glyph: glyphChoices[r.glyphIx],
		RateAmount: r.inputs[4].Value(), RateCurrency: r.inputs[5].Value(),
	}
}

func (r *FormRoute) FillForTest(v FormValues) {
	r.inputs[0].SetValue(v.Name)
	r.inputs[1].SetValue(v.Slug)
	r.inputs[2].SetValue(v.Description)
	r.inputs[3].SetValue(v.UpstreamGit)
	r.inputs[4].SetValue(v.RateAmount)
	r.inputs[5].SetValue(v.RateCurrency)
	if v.Kind != "" {
		r.kindIx = indexOf(kindChoices, v.Kind)
	}
	r.statusIx = indexOf(statusChoices, orDefault(v.Status, "active"))
	r.colorIx = indexOf(colorChoices, v.Color)
	r.glyphIx = indexOf(glyphChoices, v.Glyph)
	r.recomputeParents()
	r.parentIx = indexOf(r.parentIDs, v.ParentID)
}

// Submit validates synchronously then returns an async cmd.
func (r *FormRoute) Submit() (shell.Route, tea.Cmd) {
	v := r.Values()
	if strings.TrimSpace(v.Name) == "" {
		r.err = "Name erforderlich"
		return r, nil
	}
	kind := r.currentKind()
	var parentPtr *string
	if kind != domain.KindEngagement {
		if strings.TrimSpace(v.ParentID) == "" {
			r.err = "Übergeordneter Knoten erforderlich"
			return r, nil
		}
		p := v.ParentID
		parentPtr = &p
	}
	rate, perr := parseRateCents(v.RateAmount)
	if perr != nil {
		r.err = perr.Error()
		return r, nil
	}
	cur := strings.TrimSpace(v.RateCurrency)
	if cur == "" {
		cur = "EUR"
	}
	api, editing := r.api, r.editing
	return r, func() tea.Msg {
		ctx := context.Background()
		var id string
		if editing != nil {
			id = editing.ID
			if _, err := api.UpdateNode(ctx, id, UpdateFields{
				Name: v.Name, Slug: v.Slug, Color: v.Color, Glyph: v.Glyph,
				Description: v.Description, UpstreamGit: v.UpstreamGit, Status: v.Status,
			}); err != nil {
				return formErrMsg{fmt.Sprintf("Speichern: %v", err)}
			}
		} else {
			n, err := api.CreateNode(ctx, CreateFields{
				Name: v.Name, Kind: string(kind), ParentID: parentPtr,
				Color: v.Color, Glyph: v.Glyph, Description: v.Description, UpstreamGit: v.UpstreamGit,
			})
			if err != nil {
				return formErrMsg{fmt.Sprintf("Anlegen: %v", err)}
			}
			id = n.ID
		}
		if kind == domain.KindEngagement {
			if err := api.SetNodeRate(ctx, id, rate, cur); err != nil {
				return formErrMsg{fmt.Sprintf("Satz: %v", err)}
			}
		}
		return shell.PopRouteMsg{}
	}
}

func (r *FormRoute) Init2() {} // (placeholder removed — none)

func (r *FormRoute) View(f shell.Frame) string { return renderFormView(r, f) }

func (r *FormRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	switch m := msg.(type) {
	case nodesLoadedMsg:
		r.allNodes = m.nodes
		r.recomputeParents()
		return r, nil
	case formErrMsg:
		r.err = m.err
		return r, nil
	case tea.KeyPressMsg:
		switch {
		case m.Code == tea.KeyEsc:
			return r, func() tea.Msg { return shell.PopRouteMsg{} }
		case m.Code == tea.KeyEnter:
			return r.Submit()
		case m.Code == tea.KeyTab && m.Mod.Contains(tea.ModShift):
			r.focusBy(-1)
			return r, nil
		case m.Code == tea.KeyTab || m.Code == tea.KeyDown:
			r.focusBy(1)
			return r, nil
		case m.Code == tea.KeyUp:
			r.focusBy(-1)
			return r, nil
		case m.Code == tea.KeyRight:
			r.cycle(1)
			return r, nil
		case m.Code == tea.KeyLeft:
			r.cycle(-1)
			return r, nil
		default:
			if ti, ok := textInputIdx[r.focusIdx]; ok {
				var cmd tea.Cmd
				r.inputs[ti], cmd = r.inputs[ti].Update(m)
				return r, cmd
			}
		}
	}
	return r, nil
}

func (r *FormRoute) focusBy(d int) {
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		r.inputs[ti].Blur()
	}
	r.focusIdx = (r.focusIdx + d + focusCount) % focusCount
	// skip Kind/Parent when editing (immutable); skip Rate for non-engagement.
	for r.skip(r.focusIdx) {
		r.focusIdx = (r.focusIdx + sign(d) + focusCount) % focusCount
	}
	if ti, ok := textInputIdx[r.focusIdx]; ok {
		_ = r.inputs[ti].Focus()
	}
}

func (r *FormRoute) skip(idx int) bool {
	if r.editing != nil && (idx == focusKind || idx == focusParent) {
		return true
	}
	if r.currentKind() != domain.KindEngagement && (idx == focusRateAmount || idx == focusRateCurrency) {
		return true
	}
	return false
}

func (r *FormRoute) cycle(d int) {
	switch r.focusIdx {
	case focusKind:
		if r.editing == nil {
			r.kindIx = (r.kindIx + d + len(kindChoices)) % len(kindChoices)
			r.recomputeParents()
		}
	case focusParent:
		if n := len(r.parentIDs); n > 0 {
			r.parentIx = (r.parentIx + d + n) % n
		}
	case focusStatus:
		r.statusIx = (r.statusIx + d + len(statusChoices)) % len(statusChoices)
	case focusColor:
		r.colorIx = (r.colorIx + d + len(colorChoices)) % len(colorChoices)
	case focusGlyph:
		r.glyphIx = (r.glyphIx + d + len(glyphChoices)) % len(glyphChoices)
	}
}

func sign(d int) int {
	if d < 0 {
		return -1
	}
	return 1
}

func parseRateCents(amount string) (*int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 {
		return nil, fmt.Errorf("ungültiger Satz %q", amount)
	}
	c := int64(f*100 + 0.5)
	return &c, nil
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
```
  (Remove the accidental `Init2` stub before committing — included here only as a reminder not to leave dead methods; delete it.) Implement `formview.go` mirroring `projects/formview.go`'s layout: labelled text inputs for Name/Slug/Description/Upstream; selector lines for Kind (create-only), Parent (create-only), Status, Color (glyph swatch via `kindcolor`/raw color name), Glyph; Rate amount/currency only when `currentKind()==engagement`; `‹ ›` arrows on the focused selector; render `r.err` in `theme.Err` when set; footer hint "tab Feld · ← → Auswahl · enter speichern · esc abbrechen".
- [ ] **Step 4** — `go test ./internal/tui/screen/nodetree/` → PASS; `make ci` green.
- [ ] **Step 5** — Commit: `feat(tui): nodetree create/edit form (kind+parent+rate)`.

---

### Task E6: worktime booking → ENGAGEMENT picker

**Files** (modify)
- `internal/tui/screen/worktime/route.go` (todayAPI: `CreateNode`; add `domain` engagement filter helper usage)
- `internal/tui/screen/worktime/mru.go` (`mruProjects` → `mruEngagements`)
- `internal/tui/screen/worktime/dialogs.go` (booking picker → engagements)
- `internal/tui/screen/worktime/daydetail/api.go` + `route.go` + `dialogs.go` (Nachbuchen picker → engagements)
- `internal/tui/screen/worktime/dialogs_test.go`, `route_test.go`, `daydetail/route_test.go` (update fakes)

**Interfaces**
- *Consumes:* apiclient `ListNodes`, `CreateNode(ctx, CreateFields)` (Slice C), `StartSession(ctx, *string, …)` / `StopSession(ctx, id, nodeID)` / `AddSession` / `EditSession` (Slice B, `nodeID`-typed + engagement kind-guard, returns `ErrInvalidNode` if not engagement); `domain.KindEngagement`; `ui/fuzzylist`.
- *Produces:* engagement-scoped booking; `mruEngagements([]domain.Node, []domain.WorkSession) []domain.Node`; toast on `ErrInvalidNode`.

**Steps**

- [ ] **Step 1** — Update tests to drive engagements. In `worktime/route_test.go` change the fake's `ListProjects`→`ListNodes` returning `[]domain.Node` with mixed kinds, and assert the booking list only contains engagements; add a `CreateNode(ctx, CreateFields)` stub. In `dialogs_test.go` assert inline-create sends `Kind == string(domain.KindEngagement)`. Example new assertion block in `dialogs_test.go`:
```go
func TestBooking_OnlyEngagementsListed(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern"},
		{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
	}
	got := engagementItems(mruEngagements(nodes, nil))
	if len(got) != 1 || got[0].ID != "e1" {
		t.Fatalf("booking list must contain only engagements, got %+v", got)
	}
}
```
- [ ] **Step 2** — `go test ./internal/tui/screen/worktime/...` → FAIL (compile: `ListNodes`/`CreateNode`/`mruEngagements`/`engagementItems` mismatch).
- [ ] **Step 3** — Apply changes:
  - `worktime/route.go` `todayAPI`: replace `CreateProject(ctx, name) (domain.Project, error)` with `CreateNode(ctx context.Context, in apiclient.CreateNodeFields) (domain.Node, error)` (keep `ListNodes`, the rest already `nodeID`-typed by Slice B). Add import alias if needed.
  - `worktime/mru.go`: rename `mruProjects`→`mruEngagements`, change input to `[]domain.Node`, and **filter to engagements first**:
```go
func mruEngagements(nodes []domain.Node, sessions []domain.WorkSession) []domain.Node {
	var engs []domain.Node
	for _, n := range nodes {
		if n.Kind == domain.KindEngagement {
			engs = append(engs, n)
		}
	}
	// …existing MRU body, over engs, using s.NodeID instead of s.ProjectID…
	return engs // (after MRU sort)
}
```
  - `worktime/dialogs.go`: rename `projectItems`→`engagementItems` over `[]domain.Node`; in `startOrStop` load `api.ListNodes(ctx)` → `mruEngagements(ns, ss)`; relabel `renderBooking` to "Engagement buchen / wählen"; in `handleBookingKey` inline-create:
```go
if isCreate {
	name := strings.TrimSpace(r.booking.list.Query())
	return r, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n, err := api.CreateNode(ctx, apiclient.CreateNodeFields{
			Name: name, Kind: string(domain.KindEngagement),
		})
		if err != nil {
			return loadedMsg{err: err}
		}
		if _, err := api.StopSession(ctx, id, n.ID); err != nil {
			return loadedMsg{err: err}
		}
		return reloadMsg{}
	}
}
```
   and the existing-selection branch keeps `StopSession(ctx, id, it.ID)` but on `ErrInvalidNode` surface a toast (the `loadedMsg{err}` path already renders the error; optionally map to `toast.NewDanger("Kein gültiges Engagement", r.pal)`).
  - `daydetail/api.go`: `n(ctx) ([]domain.Project…)`→`ListNodes(ctx) ([]domain.Node, error)` + `CreateNode(ctx, apiclient.CreateNodeFields) (domain.Node, error)`.
  - `daydetail/route.go` + `dialogs.go`: `projectItems`→`engagementItems` over engagements (`mruEngagements`/filter), inline-create → `CreateNode` kind=engagement, labels "Engagement wählen", `projID`/`projName` now hold the engagement.
- [ ] **Step 4** — `go test ./internal/tui/screen/worktime/...` → PASS; `make ci` green.
- [ ] **Step 5** — Commit: `feat(tui): worktime booking uses the engagement picker`.

---

### Task E7: mount DI seam + tab wiring + deep-link + remove legacy flat screen

**Files**
- `internal/tui/screen/nodetree/mount.go` (new)
- `internal/tui/screen/nodetree/mount_test.go` (new)
- `cmd/flow/ui.go` (modify)
- delete the legacy flat screen package (Slice A's renamed `internal/tui/screen/projects`) and its import

**Interfaces**
- *Consumes:* `*apiclient.Client` (satisfies `TreeAPI`+`DetailAPI`+`FormAPI`); `shell.{Route}`; `theme.Palette`.
- *Produces:* `nodetree.Mount(client *apiclient.Client, pal theme.Palette, user string) shell.Route`; `nodetree.MountWithAPI(tree TreeAPI, detail DetailAPI, form FormAPI, pal theme.Palette, user string) shell.Route`.

**Steps**

- [ ] **Step 1** — Write failing `mount_test.go`:
```go
package nodetree

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeAll struct{ fakeTreeAPI }

func (f *fakeAll) GetNode(context.Context, string) (domain.Node, error)  { return domain.Node{}, nil }
func (f *fakeAll) Ancestors(context.Context, string) ([]domain.Node, error) { return nil, nil }
func (f *fakeAll) ListDocumentsScoped(context.Context, *string, ...string) ([]domain.Document, error) {
	return nil, nil
}
func (f *fakeAll) ListBindings(context.Context) ([]domain.ProjectBinding, error) { return nil, nil }
func (f *fakeAll) ListNodes(context.Context) ([]domain.Node, error)             { return nil, nil }
func (f *fakeAll) CreateNode(context.Context, CreateFields) (domain.Node, error) { return domain.Node{}, nil }
func (f *fakeAll) UpdateNode(context.Context, string, UpdateFields) (domain.Node, error) {
	return domain.Node{}, nil
}
func (f *fakeAll) SetNodeRate(context.Context, string, *int64, string) error { return nil }

func TestMountWithAPI_WiresFactories(t *testing.T) {
	t.Parallel()
	f := &fakeAll{}
	root := MountWithAPI(f, f, f, theme.Default, "u").(*Route)
	if root.detailFor == nil || root.formFor == nil {
		t.Fatal("Mount must wire detail + form factories")
	}
	// enter→detail and n→form must produce real child routes.
	root.all = []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "E"}}
	root.rebuild()
	if d := root.detailFor(root.rows[0].Node); d == nil {
		t.Fatal("detail factory returned nil")
	}
	if fm := root.formFor(nil); fm == nil {
		t.Fatal("form factory returned nil")
	}
	var _ shell.Route = root
}
```
- [ ] **Step 2** — `go test ./internal/tui/screen/nodetree/` → FAIL (no `Mount`).
- [ ] **Step 3** — Implement `mount.go`:
```go
package nodetree

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Mount builds the "Knoten" tab root with tree↔detail↔form navigation wired.
// The production composition root passes the shared *apiclient.Client, which
// satisfies all three narrow surfaces.
func Mount(client *apiclient.Client, pal theme.Palette, user string) shell.Route {
	return MountWithAPI(client, client, client, pal, user)
}

// MountWithAPI is the DI seam: tests inject fakes; production passes one client
// three times.
func MountWithAPI(tree TreeAPI, detail DetailAPI, form FormAPI, pal theme.Palette, user string) shell.Route {
	root := NewRoute(tree, pal, user)
	formFactory := func(editing *domain.Node) shell.Route {
		return NewFormRoute(form, pal, editing)
	}
	root.SetFormFactory(formFactory)
	root.SetDetailFactory(func(n domain.Node) shell.Route {
		d := NewDetailRoute(detail, pal, n)
		d.SetFormFactory(formFactory)
		return d
	})
	return root
}
```
  Modify `cmd/flow/ui.go`: swap the import `projectscreen "…/screen/projects"` for `nodetree "…/screen/nodetree"`; change the 4th tab to `nodetree.Mount(client, pal, user)`; update `tabIndexForArg` to map `"node","nodes","knoten","p","projekte","projects"` → 3; update the cobra `Short` to "Home · Worktime · Docs · Knoten". Then delete the legacy flat screen package directory (Slice A's renamed `internal/tui/screen/projects`) and remove its import. (Note: if Slice A renamed that dir to `screen/node`, delete that one — the dir name is not load-bearing; `nodetree` is the live tree screen.)
- [ ] **Step 4** — `go build ./... && go test ./...` → PASS; `make ci` green; manual deep-link check `go run ./cmd/flow ui node` opens on the Knoten tab.
- [ ] **Step 5** — Commit: `feat(tui): wire Knoten tree tab + flow ui node deep-link; drop legacy flat screen`.

---

### Task E8: live done-gate (wiring verification)

**Files** (none — verification only)

**Steps**

- [ ] **Step 1** — Start the dev stack (`make dev-up`, `make dev-run`, `make dev-token`) against real Postgres+Dex with the B1 migrations applied.
- [ ] **Step 2** — `flow ui node`: verify the indented Engagement→Vorhaben→Repo tree renders with kind-colored glyphs + kind badges; `j/k`+arrows move; `[`/`]` cycle the kind filter; `/` fuzzy-filters and keeps ancestors; `enter` opens the cockpit (breadcrumb root→leaf, engagement rate, read-only bindings, docs count).
- [ ] **Step 3** — `n` create a repo under an engagement; `e` edit metadata; `m` reparent it (parent picker, no cycle offered); `D` delete — confirm `ErrNodeHasChildren` surfaces as the "erst leeren oder umhängen" toast for a non-empty node and a leaf deletes cleanly. Confirm the tree live-reloads on each `node.*` SSE event without manual refresh.
- [ ] **Step 4** — Worktime: stop a running timer → the booking picker lists **only engagements** (MRU-sorted), inline-create makes an engagement, and the session books onto it; a stale/non-engagement id surfaces the `ErrInvalidNode` toast. Repeat for daydetail Nachbuchen.
- [ ] **Step 5** — `make ci` green (coverage gate held); record HEAD + outcome in the memory bank.

---

**Slice summary for the plan author:** E1 = `kindcolor.NodeKind{Color,Glyph,Label}`; E2 = pure tree/filter/move-candidate builders; E3–E5 = the `nodetree` shell.Route (tree+filters+move/delete, detail cockpit, create/edit form) following the existing `screen/projects` Route/factory/mount conventions verbatim; E6 = worktime booking → engagement picker via `ui/fuzzylist`; E7 = mount DI seam + tab wiring + `flow ui node` + legacy-screen removal; E8 = live done-gate. All glyphs come from the whitelist, all colors via `theme.Sem()`/`kindcolor`, no emoji, bubbletea/v2 API throughout.


---

## Slice F — Wiring + Done-Gate

> Verification-only slice (per [[feedback_plan_main_wiring_task]]): every step is a run-command + assertion. The only code edits permitted here are **wiring fixes** uncovered by Task F1 (a constructor the composition root forgot to call, a route never registered, an event still publishing the old type). Dev stack throughout: Postgres + Dex OIDC via `make dev-up`, server via `make dev-run`, bearer via `make dev-token`. All curls use `-k` (dev serves a self-signed cert for HTTP/2).

### Task F1: Composition-root + CLI wiring audit (cmd/flow-server/main.go, cmd/flow/main.go)

**Files:** `cmd/flow-server/main.go`, `internal/adapter/httpserver/server.go`, `cmd/flow/main.go`, `internal/adapter/apiclient/*.go`

This is the heart of the slice: prove that every store/usecase/route renamed or added in Slices A–E is actually constructed and injected. Per-task reviews pass while `run()` never calls the new constructor — these greps catch exactly that.

- [ ] **Step 1 — No stale `Project` symbols survive in the wiring layer.** Run:
  ```
  cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
  rg -n "Project" cmd/flow-server/main.go internal/adapter/httpserver/server.go cmd/flow/main.go
  ```
  Expected output: **only** `ProjectBindingStore` / `NewProjectBindingStore` (the binding *store* type keeps its name per contract — `domain.ProjectBinding.ProjectID→NodeID` is a field rename, not a store rename). Every other hit (`NewProjectStore`, `usecase.CreateProject`, `usecase.SetProjectRate`, `ResolveProject`, `/api/v1/projects`, `projectCmd`) is a FAIL → fix before proceeding.

- [ ] **Step 2 — Store constructor renamed + injected.** Confirm `main.go` line 66 reads:
  ```go
  nodeStore := pgstore.NewNodeStore(pool)
  ```
  and that `pgstore.NewNodeStore` exists: `rg -n "func NewNodeStore" internal/adapter/pgstore`. Expected: one match. (`bindingStore := pgstore.NewProjectBindingStore(pool)` stays.)

- [ ] **Step 3 — Every usecase field on the `Server` literal now takes `Nodes: nodeStore`.** Verify each of these in `main.go`'s `srv := &httpserver.Server{…}` block is present and wired (renames of the lines audited at 102–143):
  - `StartSession: usecase.StartSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock}` — **new** `Nodes` field (engagement-kind guard, contract §128)
  - `AddSession:   usecase.AddSession{Sessions: sessionStore, Nodes: nodeStore, IDs: ids, Clock: clock}` — **new** `Nodes` field
  - `StopSession:  usecase.StopSession{Sessions: sessionStore, Nodes: nodeStore, …}` (was `Projects:`)
  - `CreateNode:   usecase.CreateNode{Nodes: nodeStore, IDs: ids, Clock: clock}` (was `CreateProject`)
  - `ListNodes:    usecase.ListNodes{Nodes: nodeStore}`
  - `UpdateNode:   usecase.UpdateNode{Nodes: nodeStore, Bindings: bindingStore, IDs: ids, Clock: clock}`
  - `GetNode:      usecase.GetNode{Nodes: nodeStore}`
  - `DeleteNode:   usecase.DeleteNode{Nodes: nodeStore}`
  - `BulkAssignProject: …{Sessions: sessionStore, Nodes: nodeStore}` (field still named per Slice B; arg renamed)
  - `BuildExport:  usecase.BuildExport{Sessions: sessionStore, Nodes: nodeStore, Clock: clock, Loc: time.Local}` (was `Projects:`)
  - `SetNodeRate:  usecase.SetNodeRate{Nodes: nodeStore}` (was `SetProjectRate`)
  - `BindNode:     usecase.BindNode{Bindings: bindingStore, Nodes: nodeStore, IDs: ids, Clock: clock}`
  - `UnbindNode:   usecase.UnbindNode{Bindings: bindingStore}`
  - `ResolveNode:  usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore}`
  - `ListNodeBindings: usecase.ListNodeBindings{Bindings: bindingStore}`

  Command: `rg -n "Projects: projectStore|projectStore" cmd/flow-server/main.go` → expected **no matches** (variable renamed to `nodeStore`).

- [ ] **Step 4 — NEW usecases that did not exist pre-B1 are constructed.** Confirm these two literals are present in the `Server{}` block (the most likely "constructor never called" omissions):
  ```go
  ResolveEngagement: usecase.ResolveEngagement{
      Resolve: usecase.ResolveNode{Bindings: bindingStore, Nodes: nodeStore},
      Nodes:   nodeStore,
  },
  MoveNode: usecase.MoveNode{Nodes: nodeStore},
  ```
  Command: `rg -n "ResolveEngagement|MoveNode" cmd/flow-server/main.go internal/adapter/httpserver/server.go` → expected: present in both the struct field list (server.go) **and** the literal (main.go). Any field declared on `Server` but absent from the `main.go` literal is the classic wiring bug → fix.

- [ ] **Step 5 — `Server` struct fields renamed/added.** In `server.go`, confirm the field block (lines ~25–62) now declares `CreateNode`, `ListNodes`, `DeleteNode`, `UpdateNode`, `GetNode`, `MoveNode`, `SetNodeRate`, `BindNode`, `UnbindNode`, `ResolveNode`, `ResolveEngagement`, `ListNodeBindings` and that `Nodes ports.NodeStore` is reachable where StartSession/AddSession need it. Build is the real gate: `go build ./cmd/... ` must compile (Step 9).

- [ ] **Step 6 — Routes: all `/api/v1/nodes/*` registered, `/projects` gone, `move` present.** Run:
  ```
  rg -n "/api/v1/projects|/api/v1/nodes" internal/adapter/httpserver/server.go
  ```
  Expected `/api/v1/nodes/*` route set, **zero** `/api/v1/projects`, and these specific lines present:
  - `POST /api/v1/nodes`, `GET /api/v1/nodes`, `GET /api/v1/nodes/{id}`, `PATCH /api/v1/nodes/{id}`, `DELETE /api/v1/nodes/{id}`
  - `POST /api/v1/nodes/{id}/move` ← new, must exist
  - `PUT /api/v1/nodes/{id}/rate` ← contract §137 (legacy was `POST /projects/{id}/rate`; confirm method switched)
  - `GET /api/v1/nodes/{id}/ancestors` ← backs `apiclient.Ancestors`
  - static-before-wildcard ordering preserved: `GET /api/v1/nodes/resolve` and `…/nodes/bindings` registered **before** `…/nodes/{id}` (same ordering rule as today's lines 122–127). Visually confirm the resolve/bindings handles precede the `{id}` handles.

- [ ] **Step 7 — `node.*` event publishing replaced `project.*` everywhere.** Run:
  ```
  rg -n "EventProject|project\.(created|updated|deleted)" internal/
  ```
  Expected: **zero matches.** Then confirm the new constructors exist and are published from the handlers that mutate nodes (worktime.go ~197/248/299, webui_projects.go ~265/334/361/372, webui.go ~30, webui_worktime.go ~53) and that `move` publishes `node.moved` carrying the new parent:
  ```
  rg -n "EventNode(Created|Updated|Deleted|Moved)" internal/domain/event.go internal/adapter/httpserver
  ```
  Expected: all four constants defined in `event.go`; `EventNodeMoved` published from the move handler with `Data: {"id": …, "parentId": …}`.

- [ ] **Step 8 — CLI: `flow node` registered, `flow project` gone.** In `cmd/flow/main.go` confirm `root.AddCommand(nodeCmd())` replaces `projectCmd()`. Run:
  ```
  rg -n "projectCmd|nodeCmd" cmd/flow/main.go
  go run ./cmd/flow node --help
  ```
  Expected: `nodeCmd()` registered, no `projectCmd`; `--help` lists subcommands `create list show move rate bind unbind bindings pause resume archive rm` (contract §139). Also confirm apiclient surface: `rg -n "func \(c \*Client\) (Create|List|Get|Update|Delete|Move|Resolve)Node|Ancestors|ResolveEngagement" internal/adapter/apiclient` → `*Node*`, `MoveNode`, `Ancestors`, `ResolveNode` present; `rg -n "Project" internal/adapter/apiclient` → only `ProjectBinding*` residue allowed.

- [ ] **Step 9 — Compile both binaries.** This is the definitive "every reference resolves" check:
  ```
  go build ./cmd/flow-server ./cmd/flow
  ```
  Expected: exit 0, no output. If F1 found a missing wiring line, fix it now (smallest possible edit in main.go/server.go), rebuild, then commit:
  ```
  git add -A && git commit -m "fix(b1): wire NodeStore/MoveNode/ResolveEngagement + /api/v1/nodes routes into composition root

  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
  ```

### Task F2: Apply migrations 0015 + 0016 against dev Postgres and assert the data fix-up

**Files:** `internal/adapter/pgstore/migrations/0015_nodes_hierarchy.sql`, `0016_nodes_data_fixup.sql`, `internal/adapter/pgstore/pool.go` (apply path)

Migrations run automatically on server boot (`pgstore.Migrate` → `goose.UpContext`). The done-gate proves they apply cleanly, are idempotent (goose-tracked + the 0016 body itself), and produced the expected hierarchy. **Note on numbering:** Slice A may split into 0015 (rename) / 0016 (hierarchy columns) / 0017 (data fix-up + CHECKs). Use the actual file numbers Slice A produced; the assertions below are number-agnostic.

- [ ] **Step 1 — Bring up Postgres + Dex.** `make dev-up`. Expected tail: `postgres ... ready` and `dex ... ready`.

- [ ] **Step 2 — Apply migrations by booting the server once.** In a background terminal: `make dev-run`. Expected log lines include the goose apply for the nodes-hierarchy + data-fixup migrations, then `listening addr=:8080 dev=true`, and **no** `pgstore: migrate:` error. (If you have pre-existing dev data from earlier milestones, this is the realistic fix-up; if the DB is empty, the data-fixup is a no-op except creating Privat/RTL when ≥1 node exists.)

- [ ] **Step 3 — Idempotency at the goose level (re-run = no-op).** Stop the server (Ctrl-C) and run `make dev-run` again. Expected: **no** nodes-migration apply lines on the second boot (goose sees them in `goose_db_version`), server reaches `listening`. Confirm the version table:
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -tAc \
    "SELECT version_id FROM goose_db_version ORDER BY version_id DESC LIMIT 4"
  ```
  Expected: the B1 migration ids on top (e.g. `17`, `16`, `15`, `14`).

- [ ] **Step 4 — Privat + RTL Extern engagements exist as roots.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -c \
    "SELECT slug, kind, parent_id FROM nodes WHERE slug IN ('privat','rtl-extern')"
  ```
  Expected: both rows `kind=engagement`, `parent_id` NULL. (Count = 2 per owner-with-nodes; not duplicated on re-boot.)

- [ ] **Step 5 — Repos parented by the gitlab rule.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -c \
    "SELECT n.slug, n.kind, p.slug AS parent FROM nodes n LEFT JOIN nodes p ON p.id=n.parent_id WHERE n.kind='repo' ORDER BY parent, n.slug"
  ```
  Expected: every repo has a non-null parent; `slug ILIKE '%gitlab%'` repos → `rtl-extern`, all others → `privat`. No repo with `parent IS NULL`.

- [ ] **Step 6 — Rate audited off repos into `extra.legacy_rate`.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -tAc \
    "SELECT count(*) FROM nodes WHERE kind='repo' AND rate_amount IS NOT NULL"
  ```
  Expected: `0`. And `SELECT count(*) FROM nodes WHERE kind='repo' AND extra ? 'legacy_rate'` ≥ the number of repos that previously carried a rate.

- [ ] **Step 7 — Daily docs → RTL engagement; free docs → NULL.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -tAc \
    "SELECT count(*) FROM documents d JOIN nodes n ON n.id=d.node_id WHERE d.type='daily' AND n.kind<>'engagement'"
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -tAc \
    "SELECT count(*) FROM documents WHERE type='free' AND node_id IS NOT NULL"
  ```
  Expected: both `0`. Also `SELECT count(*) FROM documents d JOIN nodes n ON n.id=d.node_id WHERE d.type='daily' AND n.slug<>'rtl-extern'` → `0`.

- [ ] **Step 8 — Sessions repointed to their engagement ancestor.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -tAc \
    "SELECT count(*) FROM work_sessions ws JOIN nodes n ON n.id=ws.node_id WHERE ws.node_id IS NOT NULL AND n.kind<>'engagement'"
  ```
  Expected: `0` (no session points at a repo/vorhaben anymore).

- [ ] **Step 9 — CHECK constraints active.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -c \
    "SELECT conname FROM pg_constraint WHERE conrelid='nodes'::regclass AND contype='c' ORDER BY conname"
  ```
  Expected: `nodes_kind_enum`, `nodes_origin_only_repo`, `nodes_rate_only_engagement`, `nodes_root_is_engagement`. Spot-check one fires (insert a repo as root):
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -c \
    "INSERT INTO nodes (id,owner_id,parent_id,kind,name,slug) VALUES ('chk1',(SELECT owner_id FROM nodes LIMIT 1),NULL,'repo','bad','bad-root')"
  ```
  Expected: errors with `nodes_root_is_engagement`.

- [ ] **Step 10 — Parent FK is ON DELETE RESTRICT.**
  ```
  podman compose -f deploy/dev/compose.yml exec -T db psql -U flow -d flow -c \
    "SELECT confdeltype FROM pg_constraint WHERE conrelid='nodes'::regclass AND contype='f' AND conname LIKE '%parent%'"
  ```
  Expected: `confdeltype='r'` (RESTRICT).

### Task F3: Scripted curl-smoke of every `/api/v1/nodes/*` route + worktime/export (dev stack)

**Files:** optionally persist as `scripts/smoke-b1.sh` (mirrors the `scripts/smoke-m1a.sh` pattern); otherwise run inline.

Server from F2 still running on `https://localhost:8080`. Each step asserts an exact HTTP status; bodies parsed with `jq`.

- [ ] **Step 0 — Token + helpers.**
  ```
  cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
  export TOKEN=$(make dev-token 2>/dev/null)
  export BASE=https://localhost:8080
  H=(-k -sS -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")
  code() { curl -k -s -o /tmp/b1.json -w '%{http_code}' -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" "$@"; }
  ```
  Sanity: `curl "${H[@]}" "$BASE/api/v1/me" | jq -r .id` prints the dev user id.

- [ ] **Step 1 — Create engagement → vorhaben → repo (each 201, capture ids).**
  ```
  ENG=$(curl "${H[@]}" -X POST "$BASE/api/v1/nodes" -d '{"name":"Smoke Eng","slug":"smoke-eng","kind":"engagement"}' | jq -r .id)
  VOR=$(curl "${H[@]}" -X POST "$BASE/api/v1/nodes" -d "{\"name\":\"Smoke Vorhaben\",\"slug\":\"smoke-vor\",\"kind\":\"vorhaben\",\"parentId\":\"$ENG\"}" | jq -r .id)
  REPO=$(curl "${H[@]}" -X POST "$BASE/api/v1/nodes" -d "{\"name\":\"Smoke Repo\",\"slug\":\"smoke-repo\",\"kind\":\"repo\",\"parentId\":\"$VOR\"}" | jq -r .id)
  echo "$ENG $VOR $REPO"
  ```
  Expected: three non-empty ids. Bad-root guard: `code -X POST "$BASE/api/v1/nodes" -d '{"name":"x","slug":"smoke-badroot","kind":"repo"}'` → **400**.

- [ ] **Step 2 — List is flat; parent_id + kind correct.**
  ```
  curl "${H[@]}" "$BASE/api/v1/nodes" | jq '[.[] | select(.slug|startswith("smoke-")) | {slug,kind,parentId}]'
  ```
  Expected: `smoke-eng kind=engagement parentId=null`; `smoke-vor kind=vorhaben parentId=$ENG`; `smoke-repo kind=repo parentId=$VOR`.

- [ ] **Step 3 — Move repo directly under the engagement → 200.**
  ```
  code -X POST "$BASE/api/v1/nodes/$REPO/move" -d "{\"parentId\":\"$ENG\"}"   # expect 200
  ```
  Verify: `curl "${H[@]}" "$BASE/api/v1/nodes/$REPO" | jq -r .parentId` == `$ENG`.

- [ ] **Step 4 — Move that would create a cycle → 409.**
  ```
  code -X POST "$BASE/api/v1/nodes/$ENG/move" -d "{\"parentId\":\"$REPO\"}"   # expect 409 (ErrNodeCycle)
  ```

- [ ] **Step 5 — Move with an illegal kind pairing → 400.**
  ```
  code -X POST "$BASE/api/v1/nodes/$VOR/move" -d "{\"parentId\":\"$REPO\"}"   # expect 400 (ErrInvalidNode)
  ```

- [ ] **Step 6 — Rate on engagement → 204; rate on repo → 400.**
  ```
  code -X PUT "$BASE/api/v1/nodes/$ENG/rate"  -d '{"amount":9500,"currency":"EUR"}'   # expect 204
  code -X PUT "$BASE/api/v1/nodes/$REPO/rate" -d '{"amount":9500,"currency":"EUR"}'   # expect 400
  ```
  Verify: `curl "${H[@]}" "$BASE/api/v1/nodes/$ENG" | jq '.rate'` → `{amount:9500,currency:"EUR"}`.

- [ ] **Step 7 — Bind remote→repo (ok), path→leaf-vorhaben (ok), remote→vorhaben rejected.**
  ```
  code -X PUT "$BASE/api/v1/nodes/$REPO/bindings" -d '{"kind":"remote","remoteSlug":"github.com/acme/smoke"}'              # 200
  code -X PUT "$BASE/api/v1/nodes/$VOR/bindings"  -d '{"kind":"path","machineId":"smoke-machine","path":"/tmp/smoke-vor"}' # 200
  code -X PUT "$BASE/api/v1/nodes/$VOR/bindings"  -d '{"kind":"remote","remoteSlug":"github.com/acme/nope"}'              # 400 (ErrInvalidBindTarget)
  ```

- [ ] **Step 8 — Resolve cwd/remote → repo.**
  ```
  curl "${H[@]}" "$BASE/api/v1/nodes/resolve?slug=github.com/acme/smoke" | jq -r .slug   # smoke-repo
  curl "${H[@]}" "$BASE/api/v1/nodes/resolve?machine=smoke-machine&path=/tmp/smoke-vor" | jq -r .slug  # smoke-vor
  ```

- [ ] **Step 9 — Ancestors (leaf→root order).**
  ```
  curl "${H[@]}" "$BASE/api/v1/nodes/$REPO/ancestors" | jq -r '.[].slug'
  ```
  Expected, in order: `smoke-repo` then `smoke-eng`.

- [ ] **Step 10 — Delete a node with children → 409.**
  ```
  code -X DELETE "$BASE/api/v1/nodes/$ENG"   # expect 409 (ErrNodeHasChildren / RESTRICT)
  ```

- [ ] **Step 11 — Worktime start: engagement 201, repo 400** (request body key is `nodeId`).
  ```
  SID=$(curl "${H[@]}" -X POST "$BASE/api/v1/sessions" -d "{\"nodeId\":\"$ENG\"}" | jq -r .id)   # 201
  code -X POST "$BASE/api/v1/sessions" -d "{\"nodeId\":\"$REPO\"}"                               # 400
  curl "${H[@]}" -X POST "$BASE/api/v1/sessions/$SID/stop" -d "{\"nodeId\":\"$ENG\"}" >/dev/null
  ```

- [ ] **Step 12 — Export aggregates per engagement (Σh × rate).**
  ```
  FROM=$(date -u -v-1d +%Y-%m-%d 2>/dev/null || date -u -d 'yesterday' +%Y-%m-%d)
  TO=$(date -u -v+1d +%Y-%m-%d 2>/dev/null || date -u -d 'tomorrow' +%Y-%m-%d)
  S=$(date -u -v-2H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '2 hours ago' +%Y-%m-%dT%H:%M:%SZ)
  E=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  curl "${H[@]}" -X POST "$BASE/api/v1/sessions" -d "{\"nodeId\":\"$ENG\",\"start\":\"$S\",\"stop\":\"$E\"}" >/dev/null
  curl "${H[@]}" "$BASE/api/v1/export?from=$FROM&to=$TO&format=json" | jq '.byEngagement[] | select(.nodeName=="Smoke Eng") | {nodeName, total, amount}'
  ```
  Expected: one `byEngagement` entry `nodeName="Smoke Eng"`, total ≈ 2h, `amount.amount=19000 currency=EUR`. (Confirm the JSON key — if Slice B kept `byProject` for wire-stability, assert that key instead.)

- [ ] **Step 13 — Cleanup (children first, RESTRICT-safe).**
  ```
  curl "${H[@]}" -X DELETE "$BASE/api/v1/nodes/$REPO" -o /dev/null -w '%{http_code}\n'  # 204
  curl "${H[@]}" -X DELETE "$BASE/api/v1/nodes/$VOR"  -o /dev/null -w '%{http_code}\n'  # 204
  curl "${H[@]}" -X DELETE "$BASE/api/v1/nodes/$ENG"  -o /dev/null -w '%{http_code}\n'  # 204 (now childless)
  ```
  Expected: all `204`; `curl "${H[@]}" "$BASE/api/v1/nodes" | jq '[.[]|select(.slug|startswith("smoke-"))]|length'` → `0`.

- [ ] **Step 14 — Record the result.** Any status mismatch is a Slice C wiring/handler bug → fix in the owning slice's file, rebuild, re-run F3 from Step 0.

### Task F4: Live dogfood (TUI + WebUI + SSE) and `make ci` green

**Files:** none (observational); fixes land in the owning slice's files if a surface is broken.

- [ ] **Step 1 — CLI login for the TUI.**
  ```
  set -a; . deploy/dev/flow-cli.env; set +a
  go run ./cmd/flow login          # device-flow against Dex; complete in browser
  go run ./cmd/flow node list --tree
  ```
  Expected: a tree printing `Privat` / `RTL Extern` engagements with repos nested beneath, kind badges via `kindcolor`. `go run ./cmd/flow node show <repo-slug>` prints the leaf→root ancestor chain.

- [ ] **Step 2 — TUI node tree + move.** `go run ./cmd/flow ui` → open the Node/Projekte tab. Confirm: the engagement→vorhaben→repo tree renders; fuzzy + kind-filter work; the move/reparent dialog changes a node's parent and the tree re-renders. In the Worktime booking dialog, the picker now lists **engagements** (MRU + fuzzy), not repos.

- [ ] **Step 3 — WebUI tree + form + move.** Open `https://localhost:8080/` → log in via Dex → navigate to the node-management page (Slice D route). Confirm: hierarchical tree (Engagement → Vorhaben → Repo) with kind badges; create/edit form exposes name·slug·kind·parent·color·glyph·desc·upstream (+ rate field only for engagement); the Move action (parent picker) reparents and the tree updates. Repo detail shows the bindings panel + ancestor breadcrumb.

- [ ] **Step 4 — SSE live-reload across surfaces.** With a `flow ui` TUI and the WebUI open side by side: create/move/delete a node in the browser; the TUI tree and any open node list refresh within ~1s (consuming `node.created/moved/deleted`). Start a timer on an engagement in one surface; it appears in the other.

- [ ] **Step 5 — Full CI gate.** `make ci`. Expected: `lint` (gofumpt + staticcheck, incl. QF1002) clean, `verify-generate` OK (templ committed), `verify-css` OK, `verify-no-popups` OK, `cover` passes the gate (confirm coverage ≥ the Makefile threshold and **not** regressed vs the pre-B1 ~83%), `build` succeeds.

### Task F5: Update project memory + flip spec status to done

**Files:** `docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md`, flow MCP docs, MEMORY topic file.

Only after F2–F4 are all green.

- [ ] **Step 1 — Mark the spec done.** Edit the status line of the design spec: `**Status:** approved …` → `**Status:** DONE — B1 implemented + done-gate green (<date>, branch rebuild, unmerged)`.

- [ ] **Step 2 — Mirror the spec to flow (MCP).** `type: agent`, project = current dir's project, path `specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design`. `flow_search_docs` → if it exists `flow_update_doc` the body; else `flow_create_doc`.

- [ ] **Step 3 — Add a MEMORY topic file + one-line index entry.** Topic file `project_flow_rebuild_b1_done.md`: B1 (Project→Node hierarchy + bindings + Engagement-rate/worktime/export + migration) done <date>, branch `rebuild` (unmerged), commit range, `make ci` green (<cov>%), done-gate verified vs Postgres+Dex. Add the ≤200-char one-liner to the MEMORY index (it is over budget — keep it short).

- [ ] **Step 4 — Offer `memory-bank-synchronizer`** to reconcile `CLAUDE-*.md` (patterns/decisions) with the Node model, then commit the spec status:
  ```
  git add docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md
  git commit -m "docs(b1): mark hierarchie+bindings spec DONE after done-gate

  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
  ```
  (Do **not** commit `CLAUDE.md`/`CLAUDE-*.md`/`MEMORY.md`.)

- [ ] **Step 5 — Tear down dev stack.** `make dev-down`. Done-gate complete: wiring audited, migration verified, every node route curl-smoked, TUI/WebUI/SSE dogfooded, `make ci` green, memory + spec status updated.

---

**Key findings from the real code (for the implementer):**
- `cmd/flow-server/main.go:66` is the single store-constructor swap (`NewProjectStore`→`NewNodeStore`); `bindingStore := pgstore.NewProjectBindingStore(pool)` is **unchanged** by the contract — don't flag it.
- `StartSession`/`AddSession` literals (`main.go:102/111`) currently have **no** node store; B1 adds a `Nodes` field — the most likely "forgot to wire" omission (F1 Step 3/4).
- The rate route is `POST /api/v1/projects/{id}/rate` today (`server.go:120`); contract §137 specifies `PUT /api/v1/nodes/{id}/rate` — F1/F3 assume PUT; confirm Slice C switched the method.
- Migrations apply via `pgstore.Migrate` (goose `UpContext`) on server boot — "run goose up" == `make dev-run` (F2 Step 2).
- Resolve query keys are `slug`/`machine`/`path` (`projectbindings.go:28-30`); start-session body key `projectId` → `nodeId`.
- `make dev-token` prints the id_token to stdout (claims to stderr) → `TOKEN=$(make dev-token 2>/dev/null)`.
