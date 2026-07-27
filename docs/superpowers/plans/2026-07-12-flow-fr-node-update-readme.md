# FR-Slice: Node-Update-Tooling + README im Cockpit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Node-Metadaten non-interaktiv pflegbar machen (CLI `flow node update` + MCP `flow_update_node`) auf Basis einer echten Partial-PATCH-Semantik, und ein `readme`-Dokument automatisch im Cockpit rendern.

**Architecture:** `UpdateNodeInput` wird von Full-Replace (String-Felder) auf Partial (Pointer-Felder, nur non-nil anwenden) umgestellt — das schließt einen Live-Bug (Icon-Nullung bei `pause`/`resume`/`archive`) und den Upstream-Unbind-Footgun an der Quelle. CLI/MCP senden nur gesetzte Felder; die WebUI-Formulare bleiben Full-Replace (senden alle Felder als Pointer). FR-A rendert ein knoteneigenes `readme`-Doc über die bestehende `webui.RenderDocument`-Pipeline in einer neuen Cockpit-Sektion.

**Tech Stack:** Go (hexagonal), stdlib `net/http` ServeMux, Cobra CLI, modelcontextprotocol/go-sdk MCP, templ, goldmark/bluemonday (Render).

## Global Constraints

- Hexagonal: `adapter → usecase → ports ← domain`; keine I/O in `domain`; Clock/IDs injiziert.
- Multi-Tenant: **jede** Store-Query owner-scoped (`u.ID`); „Single-User" ist keine Rechtfertigung.
- TDD: erst der fehlschlagende Test, dann Minimal-Impl. Häufige Commits (ein Commit pro Task-Ende).
- Migrations brauchen `-- +goose Up/Down` — **hier keine Migration** (reine Anzeige-Konvention).
- WebUI: nach templ-Änderung `make generate`, das erzeugte `*_templ.go` **committen**. Kein `window.alert/confirm/prompt`. htmx-Cockpit-Regel: Sektionen leben in `#cockpit-main`.
- **Nie `make fmt`** (Toolchain-Skew reformatiert das ganze Repo).
- SSE: mutierende Usecases emittieren über `Emitter`; `EventNodeUpdated` existiert bereits.
- Done-Gate: `make ci` GRÜN (lint + verify-generate + verify-css + cover 75% (`*_templ.go` excl.) + build).
- Out of scope: SVG→PNG-Hinweis; TUI-README-Anzeige; kein neues Feld/keine Migration.

---

## File Structure

**Slice 1 — Partial-PATCH-Kern**
- Modify `internal/usecase/update_node.go` — `UpdateNodeInput` → Pointer; `Execute` wendet nur non-nil an.
- Modify `internal/adapter/httpserver/worktime.go` — `updateProjReq` → Pointer; `handleUpdateNode`; Create-Followup-Caller (:228).
- Modify `internal/adapter/httpserver/webui_nodes.go` — die zwei Full-Replace-Caller (:361 Edit-Form, :419 Status) auf Pointer.
- Create `internal/adapter/httpserver/ptr.go` — kleine `sp`/`nsp` Pointer-Helfer (nur wenn nicht schon vorhanden).
- Modify `internal/adapter/apiclient/client.go` — `UpdateNodeFields` → Pointer **+ `Icon`**.
- Modify `cmd/flow/node_status.go` — nur noch `Status` senden (kein GetNode mehr).
- Modify `internal/tui/screen/nodetree/form.go` — Edit-Call auf Pointer-Felder.
- Modify betroffene Tests (usecase, httpserver, apiclient, tui).

**Slice 2 — CLI**
- Modify `cmd/flow/node_subcommands.go` — `runNodeUpdate`, `nodeUpdateCmd`, `runNodeShow` (+Description).
- Modify `cmd/flow/node.go` — `nodeCmd()` registriert `nodeUpdateCmd()`.
- Modify `cmd/flow/node_subcommands_test.go` (o. neue Datei) — Tests.

**Slice 3 — MCP**
- Modify `cmd/flow-mcp/tools_project.go` — `updateNodeIn` + `updateNode`-Handler.
- Modify `cmd/flow-mcp/server.go` — `flow_update_node` registrieren.
- Modify `cmd/flow-mcp/tools_project_test.go` (o. neue Datei) — Tests.

**Slice 4 — FR-A README**
- Modify `internal/adapter/webui/cockpit_vm.go` — `Readme template.HTML`, `HasReadme bool`, `ReadmeNewHref string`.
- Modify `internal/adapter/httpserver/webui_cockpit.go` — README-Build in `nodeCockpitData`.
- Modify `internal/adapter/webui/cockpit_main.templ` — `cockpitReadmeSection` + Einhängung.
- Modify `internal/i18n/catalog_de.go` + `catalog_en.go` — README-i18n-Keys.
- Modify Tests (`webui_cockpit_test.go`).

**Slice 5 — Gate:** keine neuen Dateien; Verifikation.

---

## Slice 1 — Partial-PATCH-Kern

### Task 1: `UpdateNodeInput` → Partial-Pointer (usecase)

**Files:**
- Modify: `internal/usecase/update_node.go`
- Test: `internal/usecase/update_node_test.go`

**Interfaces:**
- Produces: `usecase.UpdateNodeInput{ Name, Slug, Color, Glyph, Icon, Description, UpstreamGit *string; Status *domain.NodeStatus; CountsTowardTarget *bool }`. `Execute(ctx, ownerID, id, in) (domain.Node, error)` wendet nur non-nil Felder an.

- [ ] **Step 1: Failing tests schreiben** — in `update_node_test.go` (bestehende Fakes/Setup des Files wiederverwenden; das Muster für Node-Store-Fake und Bindings-Fake steht bereits im File):

```go
func TestUpdateNode_PartialLeavesOtherFieldsUntouched(t *testing.T) {
	uc, nodes, _ := newUpdateNodeFixture(t) // vorhandenes Setup-Helper des Files; sonst inline wie die bestehenden Tests
	seed := domain.Node{ID: "n1", OwnerID: "o1", Kind: domain.KindRepo,
		Name: "Alt", Slug: "alt", Icon: "hex-1", Description: "d", UpstreamGit: "https://github.com/a/b"}
	nodes.put(seed)

	desc := "neu"
	got, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Description: &desc})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Description != "neu" {
		t.Errorf("Description = %q, want neu", got.Description)
	}
	if got.Name != "Alt" || got.Slug != "alt" || got.Icon != "hex-1" || got.UpstreamGit != "https://github.com/a/b" {
		t.Errorf("partial update mutated untouched fields: %+v", got)
	}
}

func TestUpdateNode_NilUpstreamKeepsBinding(t *testing.T) {
	uc, nodes, bindings := newUpdateNodeFixture(t)
	nodes.put(domain.Node{ID: "n1", OwnerID: "o1", Kind: domain.KindRepo, Name: "R", Slug: "r",
		UpstreamGit: "https://github.com/a/b"})
	name := "R2"
	if _, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Name: &name}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bindings.deletedRemotes != 0 {
		t.Errorf("partial update without upstream deleted %d bindings, want 0", bindings.deletedRemotes)
	}
}

func TestUpdateNode_EmptyStringPointerClears(t *testing.T) {
	uc, nodes, _ := newUpdateNodeFixture(t)
	nodes.put(domain.Node{ID: "n1", OwnerID: "o1", Kind: domain.KindRepo, Name: "R", Slug: "r", Icon: "hex-1"})
	empty := ""
	got, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Icon: &empty})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Icon != "" {
		t.Errorf("Icon = %q, want cleared", got.Icon)
	}
}
```

> Falls kein `newUpdateNodeFixture`/`put`/`deletedRemotes` existiert: die bestehenden Tests dieses Files zeigen das exakte Fake-Setup — dort anlehnen und einen kleinen Helper extrahieren. Der `bindings`-Fake muss `DeleteRemote`-Aufrufe zählbar machen.

- [ ] **Step 2: Tests laufen lassen, Fehlschlag bestätigen** — `go test ./internal/usecase/ -run TestUpdateNode -v` → FAIL (Compile: `*string` vs `string`).

- [ ] **Step 3: `UpdateNodeInput` + `Execute` umstellen** — in `update_node.go`:

```go
// UpdateNodeInput is a PARTIAL update: a nil field is left untouched, a non-nil
// field is applied (a non-nil pointer to "" deliberately clears the field).
// Rate is excluded — see SetNodeRate. syncRemoteBinding only fires when
// UpstreamGit is provided (non-nil) and actually changes, so an update that
// omits UpstreamGit can never delete the node's remote binding.
type UpdateNodeInput struct {
	Name               *string
	Slug               *string
	Color              *string
	Glyph              *string
	Icon               *string
	Description        *string
	UpstreamGit        *string
	Status             *domain.NodeStatus
	CountsTowardTarget *bool
}
```

`Execute`, ersetze die zwei unbedingten Zuweisungszeilen (aktuell Zeilen 41–45) durch:

```go
	p := cur
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Slug != nil {
		p.Slug = *in.Slug
	}
	if in.Color != nil {
		p.Color = *in.Color
	}
	if in.Glyph != nil {
		p.Glyph = *in.Glyph
	}
	if in.Icon != nil {
		p.Icon = *in.Icon
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.UpstreamGit != nil {
		p.UpstreamGit = *in.UpstreamGit
	}
	if in.Status != nil {
		p.Status = *in.Status
	}
	if in.CountsTowardTarget != nil {
		p.CountsTowardTarget = in.CountsTowardTarget
	}
```

Der Rest (`p.UpdatedAt`, `Validate`, Upstream-Normalisierung, `if cur.UpstreamGit != p.UpstreamGit { syncRemoteBinding }`) bleibt unverändert — weil `p.UpstreamGit` bei nil-Input gleich `cur.UpstreamGit` bleibt, feuert `syncRemoteBinding` dann nicht.

- [ ] **Step 4: Tests laufen lassen, grün** — `go test ./internal/usecase/ -run TestUpdateNode -v` → PASS. (Compile bricht jetzt in httpserver/apiclient/tui — das reparieren Task 2/3; hier nur das usecase-Paket testen.)

- [ ] **Step 5: Commit** — `git add internal/usecase/update_node.go internal/usecase/update_node_test.go && git commit -m "feat(usecase): UpdateNodeInput partial pointer semantics"`

---

### Task 2: HTTP-Transport auf Pointer (httpserver)

**Files:**
- Create: `internal/adapter/httpserver/ptr.go` (nur falls `sp`/`nsp` nicht existieren — sonst vorhandene Helfer nutzen)
- Modify: `internal/adapter/httpserver/worktime.go` (`updateProjReq` :309, `handleUpdateNode` :321, Create-Followup :228)
- Modify: `internal/adapter/httpserver/webui_nodes.go` (:361 Edit-Form, :419 Status)
- Test: `internal/adapter/httpserver/worktime_test.go` (bzw. das Node-PATCH-Testfile)

**Interfaces:**
- Consumes: `usecase.UpdateNodeInput` (Task 1).
- Produces: PATCH `/api/v1/nodes/{id}` mit fehlenden JSON-Keys → nil → unangetastet.

- [ ] **Step 1: Failing test** — im PATCH-Handler-Testfile:

```go
func TestHandleUpdateNode_PartialBodyKeepsIcon(t *testing.T) {
	srv, seed := newNodePatchServer(t) // vorhandenes Server-Test-Setup dieses Pakets
	seed(domain.Node{ID: "n1", OwnerID: testUserID, Kind: domain.KindRepo, Name: "R", Slug: "r", Icon: "hex-1"})

	rec := doAuthedJSON(t, srv, http.MethodPatch, "/api/v1/nodes/n1", `{"description":"neu"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var got domain.Node
	mustDecode(t, rec, &got)
	if got.Icon != "hex-1" {
		t.Errorf("Icon = %q, want preserved hex-1", got.Icon)
	}
	if got.Description != "neu" {
		t.Errorf("Description = %q, want neu", got.Description)
	}
}
```

> Nutze das im Paket existierende Server-/Auth-Test-Harness (dieselben Helper, mit denen andere `worktime`-Handler getestet werden). Namen hier (`newNodePatchServer`, `doAuthedJSON`, `mustDecode`) an die vorhandenen Helfer anpassen.

- [ ] **Step 2: Fehlschlag bestätigen** — `go test ./internal/adapter/httpserver/ -run TestHandleUpdateNode -v` → FAIL (Compile: `updateProjReq` String-Felder vs Pointer; Task-1-Signaturbruch).

- [ ] **Step 3a: Pointer-Helfer** — falls das Paket noch keinen String-Pointer-Helfer hat, `ptr.go` anlegen:

```go
package httpserver

import "github.com/serverkraken/flow/internal/domain"

// sp / nsp build pointers for full-replace UpdateNode callers (WebUI forms,
// create-followup) that intend to set every field. The partial PATCH handler
// passes JSON pointers straight through and does not need these.
func sp(s string) *string { return &s }

func nsp(s domain.NodeStatus) *domain.NodeStatus { return &s }
```

- [ ] **Step 3b: `updateProjReq` + `handleUpdateNode`** — `worktime.go`:

```go
type updateProjReq struct {
	Name               *string `json:"name"`
	Slug               *string `json:"slug"`
	Color              *string `json:"color"`
	Glyph              *string `json:"glyph"`
	Icon               *string `json:"icon"`
	Description        *string `json:"description"`
	UpstreamGit        *string `json:"upstreamGit"`
	Status             *string `json:"status"`
	CountsTowardTarget *bool   `json:"countsTowardTarget"`
}
```

`handleUpdateNode`, den `Execute`-Aufruf ersetzen:

```go
	in := usecase.UpdateNodeInput{
		Name: req.Name, Slug: req.Slug, Color: req.Color, Glyph: req.Glyph, Icon: req.Icon,
		Description: req.Description, UpstreamGit: req.UpstreamGit,
		CountsTowardTarget: req.CountsTowardTarget,
	}
	if req.Status != nil {
		st := domain.NodeStatus(*req.Status)
		in.Status = &st
	}
	p, err := s.UpdateNode.Execute(r.Context(), u.ID, r.PathValue("id"), in)
```

- [ ] **Step 3c: Create-Followup (`worktime.go:228`)** — Full-Replace-Caller auf Pointer:

```go
	if req.Description != "" || req.UpstreamGit != "" {
		p, err = s.UpdateNode.Execute(r.Context(), u.ID, p.ID, usecase.UpdateNodeInput{
			Name: sp(p.Name), Slug: sp(p.Slug), Color: sp(p.Color), Glyph: sp(p.Glyph), Icon: sp(p.Icon),
			Description: sp(req.Description), UpstreamGit: sp(req.UpstreamGit), Status: nsp(p.Status),
		})
```

- [ ] **Step 3d: WebUI Edit-Form (`webui_nodes.go:361`)** — Full-Replace mit Formwerten:

```go
	n, err := s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        sp(vals.Name),
		Slug:        sp(vals.Slug),
		Color:       sp(vals.Color),
		Glyph:       sp(vals.Glyph),
		Icon:        sp(vals.Icon),
		Description: sp(vals.Description),
		UpstreamGit: sp(vals.UpstreamGit),
		Status:      nsp(domain.NodeStatus(orStatus(vals.Status))),
	})
```

- [ ] **Step 3e: WebUI Status (`webui_nodes.go:419`)** — Full-Replace preserve + neuer Status:

```go
	_, err = s.UpdateNode.Execute(r.Context(), u.ID, id, usecase.UpdateNodeInput{
		Name:        sp(cur.Name),
		Slug:        sp(cur.Slug),
		Color:       sp(cur.Color),
		Glyph:       sp(cur.Glyph),
		Icon:        sp(cur.Icon),
		Description: sp(cur.Description),
		UpstreamGit: sp(cur.UpstreamGit),
		Status:      nsp(domain.NodeStatus(r.FormValue("status"))),
	})
```

- [ ] **Step 4: Tests grün** — `go test ./internal/adapter/httpserver/ -run 'TestHandleUpdateNode|TestWebNode' -v` → PASS. Bestehende Node-Update-Tests, die JSON-Bodies bauen, ggf. auf die neuen (unveränderten) JSON-Keys prüfen — die Wire-Keys sind gleich geblieben, also sollten sie durchlaufen.

- [ ] **Step 5: Commit** — `git add internal/adapter/httpserver/ && git commit -m "feat(httpserver): partial PATCH node — pointer DTO + full-replace form callers"`

---

### Task 3: apiclient + CLI-Status + TUI auf Pointer

**Files:**
- Modify: `internal/adapter/apiclient/client.go` (`UpdateNodeFields` :259)
- Modify: `cmd/flow/node_status.go`
- Modify: `internal/tui/screen/nodetree/form.go` (:269)
- Test: `internal/adapter/apiclient/*_test.go`, `internal/tui/screen/nodetree/*_test.go`

**Interfaces:**
- Produces: `apiclient.UpdateNodeFields{ Name, Slug, Color, Glyph, Icon, Description, UpstreamGit, Status *string }` (`omitempty`). Alias `nodetree.UpdateFields = apiclient.UpdateNodeFields` bleibt.

- [ ] **Step 1: Failing test** — im apiclient-Testfile:

```go
func TestUpdateNode_OmitsUnsetFields(t *testing.T) {
	var body map[string]any
	c, srv := newClientCapturingBody(t, &body, `{"id":"n1"}`) // vorhandenes Test-Harness des Pakets
	defer srv.Close()

	desc := "neu"
	if _, err := c.UpdateNode(context.Background(), "n1", apiclient.UpdateNodeFields{Description: &desc}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if _, ok := body["icon"]; ok {
		t.Errorf("icon key present in partial body: %v", body)
	}
	if body["description"] != "neu" {
		t.Errorf("description = %v, want neu", body["description"])
	}
}
```

- [ ] **Step 2: Fehlschlag** — `go test ./internal/adapter/apiclient/ -run TestUpdateNode -v` → FAIL.

- [ ] **Step 3a: `UpdateNodeFields` → Pointer + Icon** — `client.go`:

```go
// UpdateNodeFields are the mutable project fields for a PARTIAL update (rate has
// its own endpoint). A nil field is omitted from the request body and left
// untouched server-side; a non-nil pointer to "" clears the field. JSON tags
// match the server's updateProjReq.
type UpdateNodeFields struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Color       *string `json:"color,omitempty"`
	Glyph       *string `json:"glyph,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Description *string `json:"description,omitempty"`
	UpstreamGit *string `json:"upstreamGit,omitempty"`
	Status      *string `json:"status,omitempty"`
}
```

- [ ] **Step 3b: `node_status.go`** — nur noch Status senden, GetNode entfällt:

```go
// runNodeSetStatus PATCHes only the status; the partial UpdateNode leaves every
// other field (name, icon, upstream, binding) untouched.
func runNodeSetStatus(ctx context.Context, c *apiclient.Client, w io.Writer, slug, status string) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	st := status
	if _, err := c.UpdateNode(ctx, id, apiclient.UpdateNodeFields{Status: &st}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "%s is now %s\n", slug, status)
	return nil
}
```

Den jetzt ungenutzten `io`-Import prüfen (bleibt via `io.Writer`), `c.GetNode`-Zeile ist weg.

- [ ] **Step 3c: TUI `form.go:269`** — Edit-Call auf Pointer-Felder (Icon bewusst nil = preserve):

```go
			if _, err := api.UpdateNode(ctx, id, UpdateFields{
				Name:        &v.Name,
				Slug:        &v.Slug,
				Color:       &v.Color,
				Glyph:       &v.Glyph,
				Description: &v.Description,
				UpstreamGit: &v.UpstreamGit,
				Status:      &v.Status,
			}); err != nil {
```

- [ ] **Step 3d: Betroffene Fakes/Tests** — `nodetree`-Fakes (`mount_test.go`, `form_test.go`) nehmen `UpdateFields` weiterhin (Alias unverändert); wo ein Test die gesetzten Felder prüft, jetzt Pointer-Dereferenzierung. `node_status`-Tests, die einen Full-Replace-Body erwarteten, auf „nur status" anpassen. Bestehende apiclient-`UpdateNode`-Tests (falls sie String-Felder bauen) auf Pointer umstellen.

- [ ] **Step 4: Tests grün** — `go test ./internal/adapter/apiclient/... ./cmd/flow/... ./internal/tui/screen/nodetree/... -v` → PASS.

- [ ] **Step 5: Commit** — `git add internal/adapter/apiclient/ cmd/flow/node_status.go internal/tui/screen/nodetree/ && git commit -m "feat(apiclient,cli,tui): partial UpdateNodeFields (pointers + Icon), status-only pause/resume/archive"`

---

## Slice 2 — CLI `flow node update` + `show`

### Task 4: `flow node update`

**Files:**
- Modify: `cmd/flow/node_subcommands.go` (`runNodeUpdate`, `nodeUpdateCmd`)
- Modify: `cmd/flow/node.go` (`nodeCmd()` registriert `nodeUpdateCmd()`)
- Test: `cmd/flow/node_subcommands_test.go`

**Interfaces:**
- Consumes: `apiclient.UpdateNodeFields` (Task 3), `c.SetNodeRate(ctx, id, *int64, currency)`.
- Produces: CLI `flow node update <slug> [--name --slug --desc --color --glyph --icon --upstream --status] [--rate N --currency EUR | --clear-rate]`.

- [ ] **Step 1: Failing tests** — `node_subcommands_test.go` (vorhandenes CLI-Test-Harness mit Fake-Client; das Muster steht bereits im File für `runNodeCreate`/`runNodeShow`):

```go
func TestRunNodeUpdate_SendsOnlySetFields(t *testing.T) {
	fc := newFakeNodeClient(t) // Fake, der UpdateNode + SetNodeRate aufzeichnet
	fc.nodeBySlug["r"] = "n1"
	desc := "neu"
	err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{Description: &desc}, nil)
	if err != nil {
		t.Fatalf("runNodeUpdate: %v", err)
	}
	if fc.lastUpdate == nil || fc.lastUpdate.Description == nil || *fc.lastUpdate.Description != "neu" {
		t.Fatalf("expected description update, got %+v", fc.lastUpdate)
	}
	if fc.lastUpdate.Name != nil || fc.lastUpdate.Icon != nil {
		t.Errorf("unset fields leaked as non-nil: %+v", fc.lastUpdate)
	}
	if fc.rateCalls != 0 {
		t.Errorf("rate touched without --rate/--clear-rate")
	}
}

func TestRunNodeUpdate_RateAndClearRate(t *testing.T) {
	fc := newFakeNodeClient(t)
	fc.nodeBySlug["r"] = "n1"
	amount := int64(8000)
	if err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{}, &rateChange{amount: &amount, currency: "EUR"}); err != nil {
		t.Fatalf("set rate: %v", err)
	}
	if fc.lastRateAmount == nil || *fc.lastRateAmount != 8000 || fc.lastRateCurrency != "EUR" {
		t.Errorf("rate set wrong: amt=%v cur=%q", fc.lastRateAmount, fc.lastRateCurrency)
	}
	if err := runNodeUpdate(context.Background(), fc.client(), io.Discard, "r",
		apiclient.UpdateNodeFields{}, &rateChange{clear: true}); err != nil {
		t.Fatalf("clear rate: %v", err)
	}
	if fc.lastRateAmount != nil {
		t.Errorf("clear-rate should send nil amount, got %v", fc.lastRateAmount)
	}
}
```

> Am vorhandenen Fake-Client des Files anlehnen (der `runNodeCreate`-Test nutzt bereits einen). `newFakeNodeClient` muss `UpdateNode` und `SetNodeRate` aufzeichnen und `resolveSlug` bedienen.

- [ ] **Step 2: Fehlschlag** — `go test ./cmd/flow/ -run TestRunNodeUpdate -v` → FAIL (undefined `runNodeUpdate`, `rateChange`).

- [ ] **Step 3a: `runNodeUpdate` + `rateChange`** — `node_subcommands.go`:

```go
// rateChange carries an optional rate mutation for `node update`: clear=true
// clears the rate, otherwise amount (minor units) + currency set it.
type rateChange struct {
	clear    bool
	amount   *int64
	currency string
}

// hasAnyField reports whether the metadata PATCH carries at least one field.
func (f apiclient.UpdateNodeFields) isEmpty() bool { return false } // placeholder — see note

// runNodeUpdate applies a partial metadata PATCH (only the set fields) and,
// when rc != nil, a separate rate mutation via the rate endpoint.
func runNodeUpdate(ctx context.Context, c *apiclient.Client, w io.Writer, slug string, f apiclient.UpdateNodeFields, rc *rateChange) error {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return err
	}
	if updateHasField(f) {
		if _, err := c.UpdateNode(ctx, id, f); err != nil {
			return err
		}
	}
	if rc != nil {
		if rc.clear {
			if err := c.SetNodeRate(ctx, id, nil, ""); err != nil {
				return err
			}
		} else {
			if err := c.SetNodeRate(ctx, id, rc.amount, rc.currency); err != nil {
				return err
			}
		}
	}
	_, _ = fmt.Fprintf(w, "updated %s\n", slug)
	return nil
}

// updateHasField reports whether at least one metadata field is set (so an
// invocation with only --rate skips the empty PATCH).
func updateHasField(f apiclient.UpdateNodeFields) bool {
	return f.Name != nil || f.Slug != nil || f.Color != nil || f.Glyph != nil ||
		f.Icon != nil || f.Description != nil || f.UpstreamGit != nil || f.Status != nil
}
```

> Die `isEmpty`-Zeile ist ein Fehler — **nicht übernehmen**; nur `updateHasField` (freie Funktion) verwenden. (Methoden auf Fremdtypen sind in Go ohnehin nicht erlaubt.)

- [ ] **Step 3b: `nodeUpdateCmd`** — Cobra-Wrapper mit `Changed`-Detektion:

```go
func nodeUpdateCmd() *cobra.Command {
	var name, slug, color, glyph, icon, desc, upstream, status, currency string
	var rate int64
	var clearRate bool
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "update a node's fields (only the flags you pass change)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			if clearRate && fl.Changed("rate") {
				return fmt.Errorf("--clear-rate and --rate are mutually exclusive")
			}
			if fl.Changed("status") && status != "active" && status != "paused" && status != "archived" {
				return fmt.Errorf("--status must be active, paused or archived")
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			var f apiclient.UpdateNodeFields
			if fl.Changed("name") {
				f.Name = &name
			}
			if fl.Changed("slug") {
				f.Slug = &slug
			}
			if fl.Changed("color") {
				f.Color = &color
			}
			if fl.Changed("glyph") {
				f.Glyph = &glyph
			}
			if fl.Changed("icon") {
				f.Icon = &icon
			}
			if fl.Changed("desc") {
				f.Description = &desc
			}
			if fl.Changed("upstream") {
				f.UpstreamGit = &upstream
			}
			if fl.Changed("status") {
				f.Status = &status
			}
			var rc *rateChange
			switch {
			case clearRate:
				rc = &rateChange{clear: true}
			case fl.Changed("rate"):
				cur := currency
				if cur == "" {
					cur = "EUR"
				}
				amt := rate
				rc = &rateChange{amount: &amt, currency: cur}
			}
			return runNodeUpdate(cmd.Context(), c, cmd.OutOrStdout(), args[0], f, rc)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&slug, "slug", "", "new slug (identity change)")
	cmd.Flags().StringVar(&color, "color", "", "identity color name")
	cmd.Flags().StringVar(&glyph, "glyph", "", "identity glyph")
	cmd.Flags().StringVar(&icon, "icon", "", "identity icon")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	cmd.Flags().StringVar(&upstream, "upstream", "", "git clone URL (repo only)")
	cmd.Flags().StringVar(&status, "status", "", "active|paused|archived")
	cmd.Flags().Int64Var(&rate, "rate", 0, "per-hour rate in minor units (e.g. 8000 = 80.00)")
	cmd.Flags().StringVar(&currency, "currency", "", "rate currency (default EUR)")
	cmd.Flags().BoolVar(&clearRate, "clear-rate", false, "clear the rate")
	return cmd
}
```

- [ ] **Step 3c: Registrieren** — `node.go` `nodeCmd()`: `cmd.AddCommand(nodeUpdateCmd())` (nach `nodeCreateCmd()`).

- [ ] **Step 4: Tests grün** — `go test ./cmd/flow/ -run 'TestRunNodeUpdate|TestNodeUpdateCmd' -v` → PASS. Optional ein Cobra-Level-Test, dass `--clear-rate --rate 1` einen Fehler gibt.

- [ ] **Step 5: Commit** — `git add cmd/flow/node.go cmd/flow/node_subcommands.go cmd/flow/node_subcommands_test.go && git commit -m "feat(cli): flow node update — partial metadata + status/slug/rate"`

---

### Task 5: `flow node show` zeigt Beschreibung

**Files:**
- Modify: `cmd/flow/node_subcommands.go` (`runNodeShow` :129)
- Test: `cmd/flow/node_subcommands_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestRunNodeShow_PrintsDescription(t *testing.T) {
	fc := newFakeNodeClient(t)
	fc.nodeBySlug["r"] = "n1"
	fc.nodes["n1"] = domain.Node{ID: "n1", Kind: domain.KindRepo, Name: "R", Slug: "r", Description: "eine Beschreibung"}
	var buf bytes.Buffer
	if err := runNodeShow(context.Background(), fc.client(), &buf, func(string) string { return "" }, "", "r"); err != nil {
		t.Fatalf("runNodeShow: %v", err)
	}
	if !strings.Contains(buf.String(), "eine Beschreibung") {
		t.Errorf("output missing description:\n%s", buf.String())
	}
}
```

- [ ] **Step 2: Fehlschlag** — `go test ./cmd/flow/ -run TestRunNodeShow_PrintsDescription -v` → FAIL.

- [ ] **Step 3: Description-Zeile** — in `runNodeShow`, nach der `status:`-Zeile (Zeile 130):

```go
	if node.Description != "" {
		_, _ = fmt.Fprintf(w, "description: %s\n", node.Description)
	}
```

- [ ] **Step 4: Grün** — `go test ./cmd/flow/ -run TestRunNodeShow -v` → PASS.

- [ ] **Step 5: Commit** — `git add cmd/flow/node_subcommands.go cmd/flow/node_subcommands_test.go && git commit -m "feat(cli): flow node show prints description"`

---

## Slice 3 — MCP `flow_update_node`

### Task 6: MCP-Tool

**Files:**
- Modify: `cmd/flow-mcp/tools_project.go` (`updateNodeIn`, `updateNode`)
- Modify: `cmd/flow-mcp/server.go` (Registrierung)
- Test: `cmd/flow-mcp/tools_project_test.go`

**Interfaces:**
- Consumes: `h.do(ctx, req, func(c *apiclient.Client) error)`, `h.artifactNode(ctx, node) (nodeID, label string, err error)` (Ziel-Resolution: slug/name/id oder repo-Binding, verlangt einen Knoten), `textResult`/`errorResult`/`h.resultErr`, `c.UpdateNode`, `c.SetNodeRate`.
- Produces: MCP-Tool `flow_update_node`.

- [ ] **Step 1: Failing test** — `tools_project_test.go` (Muster wie `bindProject`-Tests):

```go
func TestUpdateNode_PartialAndAddressing(t *testing.T) {
	h, rec := newHandlersWithFakeClient(t) // vorhandenes MCP-Test-Harness
	rec.nodeBySlug["r"] = "n1"

	res, _, err := h.updateNode(context.Background(), &mcp.CallToolRequest{}, updateNodeIn{
		Node: "r", Description: "neu",
	})
	if err != nil {
		t.Fatalf("updateNode: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %v", res)
	}
	if rec.lastUpdate == nil || rec.lastUpdate.Description == nil || *rec.lastUpdate.Description != "neu" {
		t.Fatalf("expected description update, got %+v", rec.lastUpdate)
	}
	if rec.lastUpdate.Name != nil {
		t.Errorf("unset field leaked: %+v", rec.lastUpdate)
	}
}
```

- [ ] **Step 2: Fehlschlag** — `go test ./cmd/flow-mcp/ -run TestUpdateNode -v` → FAIL.

- [ ] **Step 3a: `updateNodeIn` + Handler** — `tools_project.go`:

```go
type updateNodeIn struct {
	Node        string `json:"node,omitempty" jsonschema:"project slug, name, or id to update; omit to use the current directory's bound project"`
	Name        string `json:"name,omitempty" jsonschema:"new display name"`
	Description string `json:"description,omitempty" jsonschema:"new description (one-line subtitle)"`
	Color       string `json:"color,omitempty" jsonschema:"identity color name"`
	Glyph       string `json:"glyph,omitempty" jsonschema:"identity glyph"`
	Icon        string `json:"icon,omitempty" jsonschema:"identity icon"`
	Upstream    string `json:"upstream,omitempty" jsonschema:"git clone URL (repo only)"`
	Status      string `json:"status,omitempty" jsonschema:"active, paused or archived"`
	Slug        string `json:"slug,omitempty" jsonschema:"new slug — an identity change; rarely needed, changes how the node is addressed"`
	Rate        *int64 `json:"rate,omitempty" jsonschema:"per-hour rate in minor units (e.g. 8000 = 80.00)"`
	Currency    string `json:"currency,omitempty" jsonschema:"rate currency (default EUR)"`
	ClearRate   bool   `json:"clearRate,omitempty" jsonschema:"clear the rate instead of setting it"`
}

// updateNode applies a partial metadata update to a node (only the fields you
// pass change) and, optionally, a rate mutation. An empty string means "leave
// this field unchanged" — the MCP surface cannot clear a text field to empty
// (use the WebUI/TUI for that); rate can be cleared via clearRate.
func (h *handlers) updateNode(ctx context.Context, req *mcp.CallToolRequest, in updateNodeIn) (*mcp.CallToolResult, any, error) {
	if in.Status != "" && in.Status != "active" && in.Status != "paused" && in.Status != "archived" {
		return errorResult("status must be active, paused or archived"), nil, nil
	}
	if in.ClearRate && in.Rate != nil {
		return errorResult("rate and clearRate are mutually exclusive"), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		var f apiclient.UpdateNodeFields
		if in.Name != "" {
			f.Name = &in.Name
		}
		if in.Slug != "" {
			f.Slug = &in.Slug
		}
		if in.Color != "" {
			f.Color = &in.Color
		}
		if in.Glyph != "" {
			f.Glyph = &in.Glyph
		}
		if in.Icon != "" {
			f.Icon = &in.Icon
		}
		if in.Description != "" {
			f.Description = &in.Description
		}
		if in.Upstream != "" {
			f.UpstreamGit = &in.Upstream
		}
		if in.Status != "" {
			f.Status = &in.Status
		}
		if f.Name != nil || f.Slug != nil || f.Color != nil || f.Glyph != nil ||
			f.Icon != nil || f.Description != nil || f.UpstreamGit != nil || f.Status != nil {
			if _, err := c.UpdateNode(ctx, nodeID, f); err != nil {
				return err
			}
		}
		switch {
		case in.ClearRate:
			if err := c.SetNodeRate(ctx, nodeID, nil, ""); err != nil {
				return err
			}
		case in.Rate != nil:
			cur := in.Currency
			if cur == "" {
				cur = "EUR"
			}
			if err := c.SetNodeRate(ctx, nodeID, in.Rate, cur); err != nil {
				return err
			}
		}
		out = fmt.Sprintf("Updated node %s.", label)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 3b: Registrieren** — `server.go`, in `newServerH` (nach `flow_bind_project`):

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_node",
		Description: "Update a node's metadata (name, description, color, glyph, icon, upstream, status) and/or rate — only the fields you pass change. Scoped to the current project by default; pass node to target another. This is how an agent maintains a project's description without the TUI.",
	}, h.updateNode)
```

- [ ] **Step 4: Grün** — `go test ./cmd/flow-mcp/ -run TestUpdateNode -v` → PASS.

- [ ] **Step 5: Commit** — `git add cmd/flow-mcp/tools_project.go cmd/flow-mcp/server.go cmd/flow-mcp/tools_project_test.go && git commit -m "feat(mcp): flow_update_node — partial node metadata + rate"`

---

## Slice 4 — FR-A README im Cockpit

### Task 7: README-Build in `nodeCockpitData` (Backend)

**Files:**
- Modify: `internal/adapter/webui/cockpit_vm.go` (neue Felder auf `NodeCockpit`)
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData`)
- Test: `internal/adapter/httpserver/webui_cockpit_test.go`

**Interfaces:**
- Consumes: `s.ListDocuments.Execute(ctx, ownerID, *nodeID, tags)`, `s.NodeAncestors`, `s.ListArtifacts`, `buildArtifactResolver(chain, arts)`, `domain.ResolveWikilink`, `webui.RenderDocument(ctx, body, resolve, resolveArtifact)`.
- Produces: `NodeCockpit.Readme template.HTML`, `NodeCockpit.HasReadme bool`, `NodeCockpit.ReadmeNewHref string`.

- [ ] **Step 1: Failing tests** — `webui_cockpit_test.go` (vorhandenes Cockpit-Test-Harness mit Fake-Stores):

```go
func TestNodeCockpitData_RendersReadme(t *testing.T) {
	s, seed := newCockpitTestServer(t) // vorhandenes Setup dieses Files
	seed.node(domain.Node{ID: "n1", OwnerID: testUserID, Kind: domain.KindRepo, Name: "R", Slug: "r"})
	seed.doc(domain.Document{ID: "d1", OwnerID: testUserID, NodeID: sp("n1"), Path: "README.md",
		Title: "R", Body: "# Hallo\n\nWelt."})

	d, err := s.nodeCockpitData(newAuthedReq(t, "/nodes/n1"), testUser, "n1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if !d.HasReadme {
		t.Fatalf("HasReadme = false, want true")
	}
	if !strings.Contains(string(d.Readme), "Hallo") {
		t.Errorf("rendered README missing content: %q", d.Readme)
	}
}

func TestNodeCockpitData_NoReadmeEmptyState(t *testing.T) {
	s, seed := newCockpitTestServer(t)
	seed.node(domain.Node{ID: "n1", OwnerID: testUserID, Kind: domain.KindRepo, Name: "R", Slug: "r"})
	d, err := s.nodeCockpitData(newAuthedReq(t, "/nodes/n1"), testUser, "n1")
	if err != nil {
		t.Fatalf("nodeCockpitData: %v", err)
	}
	if d.HasReadme {
		t.Errorf("HasReadme = true without a readme doc")
	}
	if d.ReadmeNewHref == "" {
		t.Errorf("empty-state ReadmeNewHref not set")
	}
}
```

> `sp` = String-Pointer-Helfer des Test-Files (oder inline `func sp(s string) *string { return &s }`). Namen an das vorhandene Cockpit-Test-Harness anpassen.

- [ ] **Step 2: Fehlschlag** — `go test ./internal/adapter/httpserver/ -run TestNodeCockpitData_ -v` → FAIL (Felder fehlen).

- [ ] **Step 3a: VM-Felder** — `cockpit_vm.go`, im `NodeCockpit`-Struct ergänzen:

```go
	// README (FR-A): the node's own `readme` document, rendered to sanitized
	// HTML with inline ![[slug]] artifacts. HasReadme is false when the node has
	// no readme doc — then ReadmeNewHref points at the doc-create editor for the
	// empty-state link. No ancestor/subtree inheritance: only the node's own doc.
	Readme        template.HTML
	HasReadme     bool
	ReadmeNewHref string
```

Sicherstellen, dass `html/template` im File importiert ist (für `template.HTML`).

- [ ] **Step 3b: README-Build** — `webui_cockpit.go`, in `nodeCockpitData` **vor** dem `return d, nil` (die `chain`-Variable von Zeile 41 ist in Scope):

```go
	// README (FR-A): render the node's OWN `readme` document as the project
	// front page. Own-node only (no inheritance). Degrades silently — a missing
	// readme, an unwired store, or a render error never 500s the cockpit; it
	// falls back to the empty-state link.
	d.ReadmeNewHref = "/wissen/neu?node=" + n.ID
	if s.ListDocuments.Documents != nil {
		if docs, derr := s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil); derr == nil {
			if doc, ok := findReadme(docs); ok {
				all, _ := s.ListDocuments.Execute(ctx, u.ID, nil, nil)
				resolve := func(target string) (string, string, bool) {
					if t, ok := domain.ResolveWikilink(doc, target, all); ok {
						return "/wissen/" + t.ID, t.Title, true
					}
					return "", "", false
				}
				var resolveArtifact webui.ArtifactResolver
				if arts, aerr := s.ListArtifacts.Execute(ctx, u.ID, n.ID); aerr == nil {
					resolveArtifact = buildArtifactResolver(chain, arts)
				}
				if html, _ := webui.RenderDocument(ctx, doc.Body, resolve, resolveArtifact); html != "" {
					d.Readme = html
					d.HasReadme = true
				}
			}
		}
	}
```

Und den Finder-Helper (im selben File):

```go
// findReadme returns the node's own README document — the first doc whose path
// is "readme"/"README"/"readme.md" (case-insensitive, optional .md). Returns
// false when none matches.
func findReadme(docs []domain.Document) (domain.Document, bool) {
	for _, doc := range docs {
		p := strings.ToLower(strings.TrimSuffix(doc.Path, ".md"))
		if p == "readme" {
			return doc, true
		}
	}
	return domain.Document{}, false
}
```

> `s.ListDocuments.Documents` ist die Guard-Konvention dieses Pakets (vgl. `s.ListArtifacts.Artifacts != nil`, Zeile 140). Falls das Store-Feld auf `ListDocuments` anders heißt, an die tatsächliche Feldstruktur anpassen (der Guard verhindert ein nil-Store-Panic im Test/Dev-Server). `strings` ist bereits importiert (Zeile 7).

- [ ] **Step 4: Grün** — `go test ./internal/adapter/httpserver/ -run TestNodeCockpitData_ -v` → PASS.

- [ ] **Step 5: Commit** — `git add internal/adapter/webui/cockpit_vm.go internal/adapter/httpserver/webui_cockpit.go internal/adapter/httpserver/webui_cockpit_test.go && git commit -m "feat(cockpit): build node README (own readme doc → rendered HTML)"`

---

### Task 8: `cockpitReadmeSection` templ + i18n

**Files:**
- Modify: `internal/adapter/webui/cockpit_main.templ` (`cockpitReadmeSection` + Einhängung in `CockpitMain`)
- Modify: `internal/i18n/catalog_de.go`, `internal/i18n/catalog_en.go` (i18n-Keys)
- Generated: `internal/adapter/webui/cockpit_main_templ.go` (via `make generate`)

**Interfaces:**
- Consumes: `NodeCockpit.Readme/HasReadme/ReadmeNewHref` (Task 7).

- [ ] **Step 1: i18n-Keys** — in `catalog_de.go`:

```go
	"cockpit.readme.title":     "README",
	"cockpit.readme.empty":     "Kein README — Dokument mit Pfad readme anlegen",
	"cockpit.readme.emptyLink": "README anlegen",
```

und `catalog_en.go`:

```go
	"cockpit.readme.title":     "README",
	"cockpit.readme.empty":     "No README — create a document with path readme",
	"cockpit.readme.emptyLink": "Create README",
```

- [ ] **Step 2: Sektion in `cockpit_main.templ`** — neue templ-Funktion (Muster wie `cockpitWissenSection`, aber Inhalt = rendered HTML via `templ.Raw`):

```go
// cockpitReadmeSection renders the node's own README (FR-A) as the project's
// front page — full rendered HTML with inline artifacts — or a discreet
// empty-state with a create link when no readme doc exists.
templ cockpitReadmeSection(d NodeCockpit) {
	<div class="sect">
		<div class="sect-h">
			<span class="eyebrow">{ components.T(ctx, "cockpit.readme.title") }</span>
		</div>
		if d.HasReadme {
			<div class="prose">
				@templ.Raw(d.Readme)
			</div>
		} else {
			<p class="text-sm text-faint">
				{ components.T(ctx, "cockpit.readme.empty") }
				<a class="more ml-2" href={ templ.SafeURL(d.ReadmeNewHref) } hx-boost="false">{ components.T(ctx, "cockpit.readme.emptyLink") }</a>
			</p>
		}
	</div>
}
```

> `.prose` ist die im Dokument-Leseview verwendete Klasse für gerenderten Markdown; falls das Cockpit eine andere Prose-Klasse nutzt, dieselbe wie die `/wissen/{id}`-Seite verwenden, damit Typo/Spacing konsistent sind. `@templ.Raw` gibt das bereits sanitisierte (bluemonday) HTML aus.

- [ ] **Step 3: Einhängen in `CockpitMain`** — direkt nach `@cockpitInstr(d)` und dem Error-Banner, **vor** `cockpitEnthaeltSection`:

```go
templ CockpitMain(d NodeCockpit) {
	@cockpitInstr(d)
	if d.PanelErr != "" {
		<p class="text-warn text-sm mb-3" role="alert">{ d.PanelErr }</p>
	}
	@cockpitReadmeSection(d)
	if d.N.Kind != domain.KindRepo {
		@cockpitEnthaeltSection(d)
	}
	@cockpitWissenSection(d)
	if domain.IsBookable(d.N.Kind) {
		@cockpitBuchungenSection(d)
	}
	@cockpitPulseSection(d)
	if d.EditSession != nil {
		@components.SessionDialog(sessionDialogEditVM(d))
	}
}
```

- [ ] **Step 4: Generieren + bauen** — `make generate` (Bash-Timeout 600000), dann `go build ./...`. Das erzeugte `cockpit_main_templ.go` muss committed werden.

- [ ] **Step 5: Rendertest** (optional, wenn das Paket templ-Golden/Render-Tests hat): Sektion mit `HasReadme=true` enthält den HTML-Inhalt; mit `HasReadme=false` den Empty-State-Link. Sonst durch Task-7-Backend-Tests + Slice-5-Dogfood abgedeckt.

- [ ] **Step 6: Commit** — `git add internal/adapter/webui/cockpit_main.templ internal/adapter/webui/cockpit_main_templ.go internal/i18n/catalog_de.go internal/i18n/catalog_en.go && git commit -m "feat(cockpit): README section (rendered front page + empty-state)"`

---

## Slice 5 — Wiring-Verifikation + Gate

### Task 9: Verifikation, Smoke, Dogfood, `make ci`

**Files:** keine Produktionsänderung außer Fixes, die die Verifikation aufdeckt.

- [ ] **Step 1: Main-Wiring prüfen** — bestätigen, dass keine neue Usecase-Verdrahtung in `cmd/flow-server/main.go` nötig war (alle genutzten Usecases — `UpdateNode`, `SetNodeRate`, `ListDocuments`, `ListArtifacts`, `NodeAncestors`, `ComposeContext` — sind bereits Server-Felder). MCP-Tool in `newServerH` registriert (Task 6), CLI-Kommando in `nodeCmd()` (Task 4). `rg -n "flow_update_node" cmd/flow-mcp/server.go` und `rg -n "nodeUpdateCmd" cmd/flow/node.go` → je ein Treffer. [[feedback_plan_main_wiring_task]]

- [ ] **Step 2: `make ci`** — `make ci` (Bash-Timeout 600000) → GRÜN. Bei Coverage-Miss: die dünnen neuen Funktionen (`runNodeUpdate`, `updateNode`, `findReadme`) sind testabgedeckt; fehlende Zweige ergänzen.

- [ ] **Step 3: Dev-Stack hoch** — `make dev-up` (Bash-Timeout 600000), `make dev-run` (Hintergrund), `make dev-token`.

- [ ] **Step 4: REST-Smoke (Partial-PATCH)** — gegen einen Testknoten mit gesetztem Icon:

```bash
# nur description ändern → icon/upstream/binding intakt
curl -sk -X PATCH https://localhost:8080/api/v1/nodes/$NID \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"description":"smoke"}' | jq '{icon,description,upstreamGit}'
```
Erwartung: `icon` unverändert, `description` = "smoke", `upstreamGit` unverändert.

- [ ] **Step 5: CLI-Smoke**:

```bash
flow node update r --desc "cli smoke" --color teal
flow node show r                     # zeigt "description: cli smoke"
flow node update r --rate 8000 --currency EUR
flow node update r --clear-rate
flow node pause r && flow node resume r   # icon bleibt gesetzt (Live-Bug-Fix)
flow node show r                     # icon unverändert prüfen (WebUI/Cockpit)
```

- [ ] **Step 6: MCP-Smoke** — `flow_update_node {node:"r", description:"mcp smoke"}` → Node-Description aktualisiert; `flow node show r` bestätigt.

- [ ] **Step 7: Browser-Dogfood (FR-A)** — Cockpit des Jukebox-Projekts (`/nodes/{jukebox}`) öffnen: README oben gerendert, die drei PNG-Artefakte inline sichtbar. Ein Knoten ohne `readme`-Doc zeigt den dezenten Empty-State mit Anlege-Link. README-Edit → Cockpit (SSE `document.*`) aktualisiert die Sektion live (bzw. nach Reload, falls die Sektion an keinen SSE-Container hängt — dann prüfen, ob sie in den bestehenden Cockpit-Reload-Container gehört).

- [ ] **Step 8: Aufräumen + Abschluss** — temporäre Testknoten/-docs entfernen; `make ci` final GRÜN; Memory-Note [[project_flow_rebuild_free_artifacts]] Folge-Punkt „FR-Slice" auf DONE + Commit-Range aktualisieren.

---

## Self-Review (gegen die Spec)

- **Spec-Coverage:** FR-B Lücke 1 (CLI) → Task 4/5; Lücke 2 (MCP) → Task 6; Lücke 3 (Partial-PATCH) → Task 1–3 (inkl. Icon-Live-Bug-Fix Task 3). FR-A → Task 7/8. Wiring/Gate → Task 9. SVG-Hinweis bewusst out of scope (Spec §Out-of-scope). ✓
- **Typkonsistenz:** `UpdateNodeInput`/`UpdateNodeFields` durchgängig Pointer; `Status` als `*domain.NodeStatus` (usecase) bzw. `*string` (Wire/DTO), Konvertierung nur in `handleUpdateNode` + MCP/CLI. `rateChange{clear bool; amount *int64; currency string}` einheitlich in Task 4. `findReadme`/`updateHasField` einmal definiert. ✓
- **Placeholder:** die `isEmpty`-Zeile in Task 3a ist explizit als **nicht übernehmen** markiert (Lehrbeispiel); alle anderen Steps tragen echten Code. Zwei bewusste „falls-Helper-fehlt/Klassenname"-Hinweise verweisen auf vorhandene Muster im jeweiligen File, kein offener TODO. ✓
- **Empty-State-Prefill:** README-Anlege-Link nutzt das existierende `/wissen/neu?node={id}` (kein neuer `?path=`-Param nötig; bewusste Vereinfachung, Hinweistext nennt den Pfad `readme`). ✓
