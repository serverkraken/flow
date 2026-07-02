# Cockpit-Story Slice 2 — Node-Logos (Icon-Set + Bild-Upload + Render-Helper) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Nodes bekommen ein Logo: ein **kuratiertes Icon** aus einem 40er-Lucide-Set (gewhitelistet, im Knotenfarbton gerendert) **plus optionalen Bild-Upload** (DB-Blob, 1 Logo/Node, PNG/JPEG/WebP ≤ 512 KB). Render-Priorität überall: **Upload > Icon > Glyph** — sofort sichtbar im Cockpit-Head (`cockpitHex`) und der Projektliste (`nodeGlyphSwatch`).

**Architecture:** Hexagonal. `domain.Node` erhält `Icon string` (Whitelist-Key, validiert wie Glyph) + `LogoRef string` (Content-Hash des Uploads, ""=keiner). Blobs leben in einer neuen Tabelle `node_logos` (bytea) hinter einem neuen `ports.NodeLogoStore`; drei schmale Usecases (`UploadNodeLogo`/`GetNodeLogo`/`DeleteNodeLogo`). Die 40 Lucide-SVGs werden einmalig vendored und via `go:embed` ins Binary gebacken (Docker bleibt offline); ein Drift-Guard-Test pinnt Whitelist ↔ Assets. Upload/Entfernen läuft über das bestehende Node-Formular (multipart), Serving über `GET /nodes/{id}/logo` mit ETag/immutable-Caching (`?v={LogoRef}`-URLs).

**Tech Stack:** Go, pgx/pgstore + goose-Migrationen, templ + Tailwind v4 (WebUI), Lucide-SVGs (ISC, vendored), `make generate`/`make web`/`make ci`.

## Global Constraints
- Branch: `cockpit-story` im Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Spec: `docs/superpowers/specs/2026-07-01-cockpit-story-rework-design.md` §5.2/§7 (User-Entscheidungen 2026-07-02: **DB-Blob**, **Lucide ~40**, **sofort rendern**).
- **Migrationen** brauchen `-- +goose Up`/`-- +goose Down` (sonst „unexpected state 0" beim Apply; nur pgstore-Docker-Tests fangen das).
- Nächste freie Migration-Nummer: **0026** (0025 = counts_toward_target_nullable).
- `make ci` grün pro Task (Gate **75 %**, `*_templ.go` **ausgeschlossen** — echte output-asserting Tests, kein Padding). Nach `.templ`-Änderungen: `make generate` + `_templ.go` committen; nach neuen Tailwind-Utilities: `make web` + `web/static/css/app.css` committen. **Kein `make fmt`** (Toolchain-Skew formatiert das ganze Repo um). Keine Emojis/Popups. Events via `s.Emitter.Emit` (nicht `Bus.Publish`).
- **Icon-Whitelist == vendored Dateinamen-Set** — muss beim Vendoring ein Lucide-Name abweichen (404), wird die Whitelist in `internal/domain/nodestyle.go` angepasst; der Drift-Guard-Test (Task 2) erzwingt Deckungsgleichheit.
- Logo-Uploads: nur `image/png`, `image/jpeg`, `image/webp` (per `http.DetectContentType` **gesnifft**, nie dem Header vertraut), max **512 KiB** (`usecase.MaxNodeLogoBytes`). Kein SVG-Upload (XSS-Fläche).
- REST-Scope: `icon` wandert in die bestehenden JSON-DTOs (Parität); **keine** REST-Logo-Endpoints (WebUI-only, Spec §2 „WebUI + das nötige Backend").

---

## File Structure
**Create:**
- `internal/adapter/pgstore/migrations/0026_nodes_icon_logo.sql` — icon/logo_ref-Spalten + node_logos-Tabelle.
- `internal/domain/nodelogo.go` — `domain.NodeLogo` Blob-Typ.
- `internal/adapter/webui/icons/*.svg` (40 Stück) + `internal/adapter/webui/icons/LICENSE` — vendored Lucide.
- `internal/adapter/webui/icons.go` — embed + Key→SVG-Map + `NodeIconSVG`.
- `internal/adapter/webui/nodeicon.templ` — `NodeIcon`-Render-Component.
- `internal/adapter/pgstore/nodelogos.go` — `NodeLogoStore`-Adapter.
- `internal/usecase/upload_node_logo.go`, `get_node_logo.go`, `delete_node_logo.go` — je ein Usecase pro Datei (Keine Monolithen).
- `internal/adapter/httpserver/webui_nodelogo.go` — Serve-Handler + Upload-Lese-Helfer.

**Modify:**
- `internal/domain/node.go` — `Icon`/`LogoRef`-Felder + Validierung.
- `internal/domain/nodestyle.go` — `NodeIcons`-Whitelist + `ValidNodeIcon`.
- `internal/usecase/create_node.go` / `update_node.go` — `Icon` in den Inputs.
- `internal/adapter/pgstore/nodes.go` — nodeCols + Create/Update/scanNode + 6 explizite CTE-Spaltenlisten.
- `internal/ports/ports.go` — `NodeLogoStore`-Interface + `ErrNodeLogoNotFound`.
- `internal/testutil/fakes.go` — `FakeNodeLogoStore`.
- `internal/adapter/httpserver/server.go` — 3 Usecase-Felder + `GET /nodes/{id}/logo`-Route.
- `cmd/flow-server/main.go` — `NewNodeLogoStore` + Wiring.
- `internal/adapter/webui/node_tree_vm.go` — `NodeFormValues.Icon`.
- `internal/adapter/webui/nodes.templ` — multipart-Form, Icon-Picker, Logo-Feld, `nodeGlyphSwatch`-Priorität.
- `internal/adapter/webui/cockpit.templ` — `cockpitHex`-Priorität.
- `internal/adapter/httpserver/webui_nodes.go` — Form-Werte + Create/Update-Logo-Fluss.
- `internal/adapter/httpserver/worktime.go` — REST `icon`-Parität.
- `internal/i18n/catalog_de.go` / `catalog_en.go` — neue Keys.

---

## Task 1: Domain `Icon`/`LogoRef` + `NodeIcons`-Whitelist + `NodeLogo`-Typ + Migration 0026 + pgstore-Spalten

Atomarer Feld-Zuwachs end-to-end (Domain → Usecase-Inputs → SQL) — kompiliert nur als Einheit.

**Files:**
- Modify: `internal/domain/node.go` (Struct ~Z.36-50, `Validate` ~Z.78-82), `internal/domain/nodestyle.go`, `internal/usecase/create_node.go` (Input Z.15-23, Assign Z.56), `internal/usecase/update_node.go` (Input Z.14-23, Assign Z.40), `internal/adapter/pgstore/nodes.go` (nodeCols Z.19, Create Z.21-40, Update Z.79-97, scanNode Z.242-273, CTE-Listen Z.167/170/174/196/199/203).
- Create: `internal/adapter/pgstore/migrations/0026_nodes_icon_logo.sql`, `internal/domain/nodelogo.go`.
- Test: `internal/domain/node_test.go` (anhängen), `internal/domain/nodestyle_test.go` (anhängen), pgstore-Node-Testdatei (finden: `rg -l "CountsTowardTargetNullable" internal/adapter/pgstore`).

**Interfaces:**
- Produces: `domain.Node.Icon string` (json `icon`), `domain.Node.LogoRef string` (json `logoRef,omitempty`); `domain.NodeIcons []string` (40 Keys); `domain.ValidNodeIcon(string) bool`; `domain.NodeLogo{NodeID, OwnerID, Mime, Ref string; Bytes []byte; UpdatedAt time.Time}`; `usecase.CreateNodeInput.Icon string`; `usecase.UpdateNodeInput.Icon string`.

- [ ] **Step 1: Failing domain tests** — an `internal/domain/nodestyle_test.go` anhängen:

```go
func TestValidNodeIcon(t *testing.T) {
	if !ValidNodeIcon("") {
		t.Error("empty icon (unset) must be valid")
	}
	if !ValidNodeIcon("rocket") {
		t.Error("whitelisted icon must be valid")
	}
	if ValidNodeIcon("skull") {
		t.Error("non-whitelisted icon must be invalid")
	}
	if len(NodeIcons) != 40 {
		t.Errorf("NodeIcons has %d entries, want 40", len(NodeIcons))
	}
}
```

An `internal/domain/node_test.go` anhängen:

```go
func TestNodeValidate_Icon(t *testing.T) {
	n, err := NewNode("n1", "u1", "flow", "flow", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Icon = "rocket"
	if err := n.Validate(); err != nil {
		t.Errorf("whitelisted icon rejected: %v", err)
	}
	n.Icon = "skull"
	if err := n.Validate(); err == nil {
		t.Error("non-whitelisted icon accepted")
	}
}
```

- [ ] **Step 2: Run — fails to compile** (`ValidNodeIcon`/`NodeIcons`/`Icon` undefined)

Run: `go test ./internal/domain/ -run "TestValidNodeIcon|TestNodeValidate_Icon"`
Expected: FAIL (undefined).

- [ ] **Step 3: Domain-Felder + Whitelist**

`internal/domain/node.go` — nach `Glyph` (Z.36) einfügen:

```go
	// Icon is a whitelisted key into the curated Lucide identity-icon set
	// ("" = none); rendered in the node's color. LogoRef is the content hash
	// of the uploaded logo image ("" = none) — render priority is
	// upload > icon > glyph.
	Icon    string `json:"icon"`
	LogoRef string `json:"logoRef,omitempty"`
```

In `Validate()` nach dem Glyph-Case (Z.80-81) ergänzen:

```go
	case !ValidNodeIcon(p.Icon):
		return fmt.Errorf("%w: invalid icon %q", ErrInvalidNode, p.Icon)
```

`internal/domain/nodestyle.go` — anhängen:

```go
// NodeIcons is the whitelist of curated identity-icon keys (vendored Lucide
// SVGs, ISC — see internal/adapter/webui/icons/). The key set MUST equal the
// vendored asset filenames; a WebUI drift-guard test enforces both directions.
var NodeIcons = []string{
	"code", "terminal", "server", "database", "globe", "book-open", "briefcase", "home",
	"heart", "music", "camera", "gamepad-2", "chart-line", "rocket", "flask-conical",
	"graduation-cap", "plane", "leaf", "wrench", "users", "cpu", "cloud", "shield",
	"key-round", "lock", "mail", "message-square", "phone", "calendar", "clock",
	"map", "compass", "star", "zap", "flame", "coffee", "gift", "palette", "pen-tool",
	"lightbulb",
}

// ValidNodeIcon reports whether i is unset ("") or a whitelisted icon key.
func ValidNodeIcon(i string) bool { return i == "" || inList(NodeIcons, i) }
```

- [ ] **Step 4: `domain.NodeLogo`** — Create `internal/domain/nodelogo.go`:

```go
package domain

import "time"

// NodeLogo is a node's uploaded logo image (at most one per node,
// replace-on-upload; stored as a DB blob). Mime is sniffed server-side
// (image/png, image/jpeg or image/webp). Ref is the first 12 hex chars of
// the content's SHA-256 — mirrored onto Node.LogoRef for cache-busting URLs
// and used as the serving ETag.
type NodeLogo struct {
	NodeID    string
	OwnerID   string
	Mime      string
	Ref       string
	Bytes     []byte
	UpdatedAt time.Time
}
```

- [ ] **Step 5: Usecase-Inputs**

`internal/usecase/create_node.go` — `CreateNodeInput` Z.19 erweitern:

```go
	Color, Glyph, Icon, Description, UpstreamGit string
```

und im `Execute` (Z.56):

```go
	n.Color, n.Glyph, n.Icon = in.Color, in.Glyph, in.Icon
```

`internal/usecase/update_node.go` — `UpdateNodeInput` nach `Glyph` ergänzen:

```go
	Icon               string
```

und im `Execute` (Z.40):

```go
	p.Name, p.Slug, p.Color, p.Glyph, p.Icon = in.Name, in.Slug, in.Color, in.Glyph, in.Icon
```

(`p := cur` kopiert `LogoRef` unangetastet weiter — `UpdateNodeInput` bekommt **kein** LogoRef-Feld; das setzen nur die Logo-Usecases in Task 3.)

- [ ] **Step 6: Migration 0026** — Create `internal/adapter/pgstore/migrations/0026_nodes_icon_logo.sql`:

```sql
-- +goose Up
ALTER TABLE nodes ADD COLUMN icon text NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN logo_ref text NOT NULL DEFAULT '';
CREATE TABLE node_logos (
    node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    mime       TEXT NOT NULL,
    ref        TEXT NOT NULL,
    bytes      BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE node_logos;
ALTER TABLE nodes DROP COLUMN logo_ref;
ALTER TABLE nodes DROP COLUMN icon;
```

- [ ] **Step 7: pgstore-Spalten** — `internal/adapter/pgstore/nodes.go`:

1. `nodeCols` (Z.19): am Ende `, icon, logo_ref` anhängen:

```go
const nodeCols = `id, owner_id, parent_id, kind, name, slug, color, glyph, description, upstream_git, origin_slug, status, rate_amount, rate_currency, extra, created_at, updated_at, counts_toward_target, icon, logo_ref`
```

2. `Create` (Z.21-40): VALUES-Liste um `,$19,$20` erweitern und die Args um `, n.Icon, n.LogoRef` (nach `n.CountsTowardTarget`).

3. `Update` (Z.79-97): SET-Liste + Args werden:

```go
	const q = `
UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5,
                 upstream_git=$6, origin_slug=$7, status=$8, extra=$9, counts_toward_target=$10,
                 icon=$11, logo_ref=$12, updated_at=$13
WHERE owner_id=$14 AND id=$15
RETURNING ` + nodeCols
```

```go
	got, err := scanNode(s.pool.QueryRow(ctx, q,
		n.Name, n.Slug, n.Color, n.Glyph, n.Description, n.UpstreamGit, nullStr(n.OriginSlug),
		string(n.Status), ex, n.CountsTowardTarget, n.Icon, n.LogoRef, n.UpdatedAt, ownerID, n.ID))
```

4. `scanNode` (Z.242-273): Scan-Liste am Ende um `, &n.Icon, &n.LogoRef` erweitern (nach `&n.CountsTowardTarget` — Reihenfolge == nodeCols).

5. **Die 6 expliziten CTE-Spaltenlisten** (Ancestors Z.167/170/174, Subtree Z.196/199/203): in den beiden inneren CTE-SELECTs `icon, logo_ref` (bzw. `n.icon, n.logo_ref`) **vor** der depth-Spalte ergänzen, in den beiden finalen SELECTs am Ende. Verifikation danach:

```bash
rg -c "logo_ref" internal/adapter/pgstore/nodes.go
```

Expected: ≥ 8 Treffer (nodeCols + Update-SET + Update-Args-Zeile zählt nicht + 6 CTE-Listen).

- [ ] **Step 8: pgstore-Round-Trip-Test** — an die pgstore-Node-Testdatei anhängen (Harness der bestehenden Tests dort spiegeln, z. B. `TestNodeStore_CountsTowardTargetNullable`):

```go
func TestNodeStore_IconLogoRefRoundTrip(t *testing.T) {
	// (reuse the file's existing pgstore harness: pool + NodeStore + seeded owner)
	st := newTestNodeStore(t) // adapt to the constructor the file already uses
	ctx := context.Background()
	n, _ := domain.NewNode("n-icon", "u1", "n-icon", "n-icon", time.Now())
	n.Kind = domain.KindEngagement
	n.Icon = "rocket"
	got, err := st.Create(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icon != "rocket" || got.LogoRef != "" {
		t.Errorf("create round-trip: icon=%q logoRef=%q", got.Icon, got.LogoRef)
	}
	got.Icon, got.LogoRef = "leaf", "abc123def456"
	got2, err := st.Update(ctx, "u1", got)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Icon != "leaf" || got2.LogoRef != "abc123def456" {
		t.Errorf("update round-trip: icon=%q logoRef=%q", got2.Icon, got2.LogoRef)
	}
	// Subtree/Ancestors CTEs must carry the new columns too.
	sub, err := st.Subtree(ctx, "u1", got.ID)
	if err != nil || len(sub) == 0 {
		t.Fatalf("subtree: %v (len %d)", err, len(sub))
	}
	if sub[0].Icon != "leaf" {
		t.Errorf("subtree lost icon: %q", sub[0].Icon)
	}
}
```

- [ ] **Step 9: Build/Test/CI**

Run: `go build ./... && go test ./internal/domain/ ./internal/usecase/ && make ci`
Expected: grün (make ci fährt die pgstore-Docker-Tests → Migration 0026 applied; `testutil.FakeNodeStore` kopiert ganze Structs und braucht keine Änderung — kompiliert mit).

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat(node): Icon whitelist-key + LogoRef + node_logos blob table (migration 0026)"
```

---

## Task 2: Lucide-Assets vendoren + `NodeIcon`-Render + Drift-Guard

**Files:**
- Create: `internal/adapter/webui/icons/*.svg` (40), `internal/adapter/webui/icons/LICENSE`, `internal/adapter/webui/icons.go`, `internal/adapter/webui/nodeicon.templ`.
- Test: `internal/adapter/webui/icons_test.go`.

**Interfaces:**
- Consumes: `domain.NodeIcons` (Task 1).
- Produces: `webui.NodeIconSVG(key string) string` (render-fertiges Inline-SVG, "" für unbekannt); templ `webui.NodeIcon(key, class string)` (Span-Wrapper, tintet via currentColor).

- [ ] **Step 1: SVGs vendoren** (einmalig, dev-time; Binary bleibt offline via go:embed):

```bash
cd internal/adapter/webui && mkdir -p icons
for n in code terminal server database globe book-open briefcase home \
         heart music camera gamepad-2 chart-line rocket flask-conical \
         graduation-cap plane leaf wrench users cpu cloud shield \
         key-round lock mail message-square phone calendar clock \
         map compass star zap flame coffee gift palette pen-tool lightbulb; do
  curl -fsSL "https://unpkg.com/lucide-static@latest/icons/$n.svg" -o "icons/$n.svg" \
    || curl -fsSL "https://raw.githubusercontent.com/lucide-icons/lucide/main/icons/$n.svg" -o "icons/$n.svg" \
    || { echo "MISSING: $n"; }
done
curl -fsSL "https://unpkg.com/lucide-static@latest/LICENSE" -o icons/LICENSE
ls icons/*.svg | wc -l   # expected: 40
for f in icons/*.svg; do head -c 4 "$f" | rg -q "<svg" || echo "BAD: $f"; done
```

Meldet die Schleife `MISSING:`/`BAD:` (Lucide hat den Namen umbenannt): aktuellen Namen auf https://lucide.dev/icons nachschlagen, Datei unter dem **neuen** Namen vendoren und den Key in `domain.NodeIcons` (Task 1) identisch anpassen — Whitelist == Dateinamen-Set ist Pflicht.

- [ ] **Step 2: Failing Drift-Guard-Test** — Create `internal/adapter/webui/icons_test.go`:

```go
package webui_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// Drift guard: every whitelisted icon key MUST have a vendored SVG (else the
// form offers an icon that renders blank) and the SVG must be render-ready.
func TestNodeIconSVGCoversWholeWhitelist(t *testing.T) {
	for _, key := range domain.NodeIcons {
		svg := webui.NodeIconSVG(key)
		if !strings.Contains(svg, "<svg") {
			t.Errorf("icon %q → no SVG markup", key)
			continue
		}
		if !strings.Contains(svg, `width="100%"`) || !strings.Contains(svg, `height="100%"`) {
			t.Errorf("icon %q → fixed dimensions not rewritten to 100%%", key)
		}
		if !strings.Contains(svg, `stroke="currentColor"`) {
			t.Errorf("icon %q → not currentColor-tintable", key)
		}
	}
	if webui.NodeIconSVG("") != "" {
		t.Error("empty key → empty SVG")
	}
	if webui.NodeIconSVG("skull") != "" {
		t.Error("unknown key → empty SVG (not a guess)")
	}
}

// Reverse drift guard: no orphan assets beyond the whitelist.
func TestNodeIconAssetsMatchWhitelistCount(t *testing.T) {
	if got, want := webui.NodeIconCount(), len(domain.NodeIcons); got != want {
		t.Errorf("embedded icons = %d, whitelist = %d — keep them identical", got, want)
	}
}
```

- [ ] **Step 3: Run — fails** (`NodeIconSVG`/`NodeIconCount` undefined)

Run: `go test ./internal/adapter/webui/ -run TestNodeIcon`
Expected: FAIL (undefined).

- [ ] **Step 4: Embed + Map** — Create `internal/adapter/webui/icons.go`:

```go
package webui

import (
	"embed"
	"regexp"
	"strings"
)

//go:embed icons/*.svg
var iconFS embed.FS

// iconDim rewrites Lucide's fixed 24px dimensions so the SVG fills whatever
// sized box the caller renders it into (viewBox is preserved).
var iconDim = regexp.MustCompile(`(width|height)="24"`)

// nodeIconSVG maps icon keys (= vendored filenames, = domain.NodeIcons) to
// render-ready inline SVG markup. Lucide strokes with currentColor, so the
// wrapper's text color tints the icon. Assets are ISC-licensed (icons/LICENSE).
var nodeIconSVG = func() map[string]string {
	m := map[string]string{}
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		panic(err) // embedded dir is a compile-time constant; cannot fail at runtime
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		b, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			panic(err)
		}
		m[strings.TrimSuffix(e.Name(), ".svg")] = iconDim.ReplaceAllString(string(b), `$1="100%"`)
	}
	return m
}()

// NodeIconSVG returns the inline SVG for a whitelisted icon key ("" when unknown).
func NodeIconSVG(key string) string { return nodeIconSVG[key] }

// NodeIconCount reports how many icon assets are embedded (drift-guard seam).
func NodeIconCount() int { return len(nodeIconSVG) }
```

- [ ] **Step 5: templ-Component** — Create `internal/adapter/webui/nodeicon.templ`:

```templ
package webui

// NodeIcon renders a whitelisted identity icon as inline SVG. The SVG strokes
// with currentColor — callers tint it via text color class or a style attr on
// their wrapper. class sizes the box (e.g. "h-5 w-5"); unknown keys render
// nothing. The markup is trusted embedded repo content (templ.Raw is safe).
templ NodeIcon(key, class string) {
	if svg := NodeIconSVG(key); svg != "" {
		<span class={ "inline-grid shrink-0 place-items-center", class } aria-hidden="true">
			@templ.Raw(svg)
		</span>
	}
}
```

- [ ] **Step 6: Generate + Run — passes**

Run: `make generate && go test ./internal/adapter/webui/ -run TestNodeIcon -v`
Expected: PASS (beide Guards).

- [ ] **Step 7: CI + Commit**

```bash
make ci
git add -A
git commit -m "feat(webui): vendored Lucide icon set (40, ISC) + NodeIcon inline-SVG render + drift guards"
```

---

## Task 3: `NodeLogoStore`-Port + pgstore-Adapter + Upload/Get/Delete-Usecases + Fake

**Files:**
- Modify: `internal/ports/ports.go` (Interface + Sentinel, nahe `NodeStore` Z.81), `internal/testutil/fakes.go` (Fake anhängen).
- Create: `internal/adapter/pgstore/nodelogos.go`, `internal/usecase/upload_node_logo.go`, `internal/usecase/get_node_logo.go`, `internal/usecase/delete_node_logo.go`.
- Test: `internal/usecase/node_logo_test.go`, pgstore-Test (an die Datei aus Task 1 Step 8 anhängen).

**Interfaces:**
- Consumes: `domain.NodeLogo`, `ports.NodeStore.Get/Update` (Task 1).
- Produces: `ports.NodeLogoStore{Put(ctx, domain.NodeLogo) error; Get(ctx, ownerID, nodeID string) (domain.NodeLogo, error); Delete(ctx, ownerID, nodeID string) error}`; `ports.ErrNodeLogoNotFound`; `usecase.MaxNodeLogoBytes = 512*1024`; `usecase.ErrLogoTooLarge`, `usecase.ErrLogoBadType`; `usecase.ValidateNodeLogo(data []byte) (mime string, err error)`; `usecase.UploadNodeLogo{Nodes, Logos, Clock}.Execute(ctx, ownerID, nodeID string, data []byte) (domain.Node, error)`; `usecase.GetNodeLogo{Logos}.Execute(ctx, ownerID, nodeID string) (domain.NodeLogo, error)`; `usecase.DeleteNodeLogo{Nodes, Logos, Clock}.Execute(ctx, ownerID, nodeID string) (domain.Node, error)`; `testutil.NewFakeNodeLogoStore()`.

- [ ] **Step 1: Failing Usecase-Tests** — Create `internal/usecase/node_logo_test.go` (Paket/Imports wie die Nachbar-Tests dort, z. B. `set_counts_toward_target_test.go`):

```go
// pngPixel is a valid 1x1 PNG (sniffs as image/png).
func pngPixel(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUploadNodeLogo(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	uc := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk}

	got, err := uc.Execute(ctx, "u1", "n1", pngPixel(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.LogoRef) != 12 {
		t.Errorf("LogoRef = %q, want 12-hex content hash", got.LogoRef)
	}
	logo, err := ls.Get(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if logo.Mime != "image/png" || logo.Ref != got.LogoRef || len(logo.Bytes) == 0 {
		t.Errorf("stored logo mime=%q ref=%q len=%d", logo.Mime, logo.Ref, len(logo.Bytes))
	}

	// Rejections: wrong type, oversized, unknown node.
	if _, err := uc.Execute(ctx, "u1", "n1", []byte("plain text, not an image")); !errors.Is(err, usecase.ErrLogoBadType) {
		t.Errorf("text upload → %v, want ErrLogoBadType", err)
	}
	if _, err := uc.Execute(ctx, "u1", "n1", make([]byte, usecase.MaxNodeLogoBytes+1)); !errors.Is(err, usecase.ErrLogoTooLarge) {
		t.Errorf("oversized upload → %v, want ErrLogoTooLarge", err)
	}
	if _, err := uc.Execute(ctx, "u1", "ghost", pngPixel(t)); err == nil {
		t.Error("unknown node accepted")
	}
}

func TestDeleteNodeLogo(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	ls := testutil.NewFakeNodeLogoStore()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)}
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = ns.Create(ctx, n)
	up := usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk}
	if _, err := up.Execute(ctx, "u1", "n1", pngPixel(t)); err != nil {
		t.Fatal(err)
	}
	del := usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Clock: clk}
	got, err := del.Execute(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogoRef != "" {
		t.Errorf("LogoRef = %q after delete, want empty", got.LogoRef)
	}
	if _, err := ls.Get(ctx, "u1", "n1"); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("blob still present after delete: %v", err)
	}
	// Deleting again is a no-op, not an error.
	if _, err := del.Execute(ctx, "u1", "n1"); err != nil {
		t.Errorf("second delete errored: %v", err)
	}
}
```

- [ ] **Step 2: Run — fails to compile** (Port/Usecases/Fake undefined)

Run: `go test ./internal/usecase/ -run "NodeLogo"`
Expected: FAIL (undefined).

- [ ] **Step 3: Port + Sentinel** — `internal/ports/ports.go`, direkt nach dem `NodeStore`-Interface einfügen (Sentinel neben die bestehenden `ErrNode*`-Vars):

```go
// ErrNodeLogoNotFound signals a node without an uploaded logo.
var ErrNodeLogoNotFound = errors.New("node logo not found")

// NodeLogoStore persists at most one uploaded logo image per node.
type NodeLogoStore interface {
	// Put upserts the node's logo (replace-on-upload).
	Put(ctx context.Context, l domain.NodeLogo) error
	// Get returns the node's logo. Owner-scoped; ErrNodeLogoNotFound when absent.
	Get(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error)
	// Delete removes the node's logo; absent is a no-op, not an error.
	Delete(ctx context.Context, ownerID, nodeID string) error
}
```

(Falls `ports.go` die Sentinels in einem gemeinsamen `var (...)`-Block hält: dort einreihen statt einzeln.)

- [ ] **Step 4: pgstore-Adapter** — Create `internal/adapter/pgstore/nodelogos.go` (Konstruktor-/Pool-Muster von `nodes.go` spiegeln — gleiche pool-Typen/Imports wie dort):

```go
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeLogoStore persists uploaded node logos as Postgres blobs (1 per node,
// FK ON DELETE CASCADE cleans up when the node goes).
type NodeLogoStore struct{ pool pool }

// NewNodeLogoStore wires the store to a pgx pool.
func NewNodeLogoStore(p pool) *NodeLogoStore { return &NodeLogoStore{pool: p} }

func (s *NodeLogoStore) Put(ctx context.Context, l domain.NodeLogo) error {
	const q = `
INSERT INTO node_logos (node_id, owner_id, mime, ref, bytes, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (node_id) DO UPDATE SET mime=$3, ref=$4, bytes=$5, updated_at=$6`
	if _, err := s.pool.Exec(ctx, q, l.NodeID, l.OwnerID, l.Mime, l.Ref, l.Bytes, l.UpdatedAt); err != nil {
		return fmt.Errorf("pgstore: put node logo: %w", err)
	}
	return nil
}

func (s *NodeLogoStore) Get(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	const q = `SELECT node_id, owner_id, mime, ref, bytes, updated_at
FROM node_logos WHERE owner_id=$1 AND node_id=$2`
	var l domain.NodeLogo
	err := s.pool.QueryRow(ctx, q, ownerID, nodeID).
		Scan(&l.NodeID, &l.OwnerID, &l.Mime, &l.Ref, &l.Bytes, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeLogo{}, ports.ErrNodeLogoNotFound
	}
	if err != nil {
		return domain.NodeLogo{}, fmt.Errorf("pgstore: get node logo: %w", err)
	}
	return l, nil
}

func (s *NodeLogoStore) Delete(ctx context.Context, ownerID, nodeID string) error {
	const q = `DELETE FROM node_logos WHERE owner_id=$1 AND node_id=$2`
	if _, err := s.pool.Exec(ctx, q, ownerID, nodeID); err != nil {
		return fmt.Errorf("pgstore: delete node logo: %w", err)
	}
	return nil
}
```

> **VERIFY beim Implementieren:** wie `nodes.go` seinen Pool hält (`*pgxpool.Pool` direkt vs. ein lokales `pool`-Interface) — exakt dasselbe Muster verwenden, sonst kompiliert der Konstruktor-Aufruf in `main.go` nicht.

- [ ] **Step 5: Usecases** — Create `internal/usecase/upload_node_logo.go`:

```go
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MaxNodeLogoBytes caps uploaded node logos (DB-blob storage keeps rows small).
const MaxNodeLogoBytes = 512 * 1024

var (
	// ErrLogoTooLarge rejects uploads beyond MaxNodeLogoBytes.
	ErrLogoTooLarge = errors.New("logo exceeds 512 KiB")
	// ErrLogoBadType rejects anything that does not sniff as PNG/JPEG/WebP.
	ErrLogoBadType = errors.New("logo must be PNG, JPEG or WebP")
)

// ValidateNodeLogo size-checks and content-sniffs logo bytes. Handlers call it
// BEFORE creating a node so a bad file rejects the whole form; UploadNodeLogo
// calls it again as its own invariant.
func ValidateNodeLogo(data []byte) (string, error) {
	if len(data) > MaxNodeLogoBytes {
		return "", ErrLogoTooLarge
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
		return mime, nil
	default:
		return "", ErrLogoBadType
	}
}

// UploadNodeLogo stores a node's logo image (replace-on-upload) and stamps the
// node's LogoRef with the content hash for cache-busting URLs and ETags.
type UploadNodeLogo struct {
	Nodes ports.NodeStore
	Logos ports.NodeLogoStore
	Clock ports.Clock
}

func (uc UploadNodeLogo) Execute(ctx context.Context, ownerID, nodeID string, data []byte) (domain.Node, error) {
	mime, err := ValidateNodeLogo(data)
	if err != nil {
		return domain.Node{}, err
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	sum := sha256.Sum256(data)
	ref := hex.EncodeToString(sum[:])[:12]
	now := uc.Clock.Now()
	if err := uc.Logos.Put(ctx, domain.NodeLogo{
		NodeID: nodeID, OwnerID: ownerID, Mime: mime, Ref: ref, Bytes: data, UpdatedAt: now,
	}); err != nil {
		return domain.Node{}, err
	}
	n.LogoRef = ref
	n.UpdatedAt = now
	return uc.Nodes.Update(ctx, ownerID, n)
}
```

Create `internal/usecase/get_node_logo.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetNodeLogo returns a node's stored logo blob (the WebUI serving path).
type GetNodeLogo struct {
	Logos ports.NodeLogoStore
}

func (uc GetNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	return uc.Logos.Get(ctx, ownerID, nodeID)
}
```

Create `internal/usecase/delete_node_logo.go`:

```go
package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DeleteNodeLogo removes a node's uploaded logo and clears its LogoRef, so
// rendering falls back to icon/glyph. Absent logo is a no-op.
type DeleteNodeLogo struct {
	Nodes ports.NodeStore
	Logos ports.NodeLogoStore
	Clock ports.Clock
}

func (uc DeleteNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.Node, error) {
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if err := uc.Logos.Delete(ctx, ownerID, nodeID); err != nil {
		return domain.Node{}, err
	}
	if n.LogoRef == "" {
		return n, nil
	}
	n.LogoRef = ""
	n.UpdatedAt = uc.Clock.Now()
	return uc.Nodes.Update(ctx, ownerID, n)
}
```

- [ ] **Step 6: Fake** — an `internal/testutil/fakes.go` anhängen (Konventionen der Nachbar-Fakes spiegeln):

```go
// FakeNodeLogoStore is an in-memory ports.NodeLogoStore (keyed by node ID).
type FakeNodeLogoStore struct {
	logos map[string]domain.NodeLogo
}

// NewFakeNodeLogoStore builds an empty in-memory logo store.
func NewFakeNodeLogoStore() *FakeNodeLogoStore {
	return &FakeNodeLogoStore{logos: map[string]domain.NodeLogo{}}
}

func (s *FakeNodeLogoStore) Put(_ context.Context, l domain.NodeLogo) error {
	s.logos[l.NodeID] = l
	return nil
}

func (s *FakeNodeLogoStore) Get(_ context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	l, ok := s.logos[nodeID]
	if !ok || l.OwnerID != ownerID {
		return domain.NodeLogo{}, ports.ErrNodeLogoNotFound
	}
	return l, nil
}

func (s *FakeNodeLogoStore) Delete(_ context.Context, ownerID, nodeID string) error {
	if l, ok := s.logos[nodeID]; ok && l.OwnerID == ownerID {
		delete(s.logos, nodeID)
	}
	return nil
}
```

- [ ] **Step 7: Run — passes**

Run: `go test ./internal/usecase/ -run "NodeLogo" -v`
Expected: PASS (Upload happy + 3 Rejections, Delete + Doppel-Delete).

- [ ] **Step 8: pgstore-Blob-Test** — an die pgstore-Node-Testdatei anhängen:

```go
func TestNodeLogoStore_PutGetDeleteCascade(t *testing.T) {
	// (same harness as the node store tests; construct NewNodeLogoStore from the same pool)
	st := newTestNodeStore(t)          // adapt to the file's harness
	ls := NewNodeLogoStore(poolOf(st)) // adapt: however the harness exposes the pool
	ctx := context.Background()
	n, _ := domain.NewNode("n-logo", "u1", "n-logo", "n-logo", time.Now())
	n.Kind = domain.KindEngagement
	created, err := st.Create(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	l := domain.NodeLogo{NodeID: created.ID, OwnerID: "u1", Mime: "image/png",
		Ref: "aaaabbbbcccc", Bytes: []byte{1, 2, 3}, UpdatedAt: time.Now()}
	if err := ls.Put(ctx, l); err != nil {
		t.Fatal(err)
	}
	l.Bytes = []byte{9, 9, 9} // replace-on-put
	if err := ls.Put(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := ls.Get(ctx, "u1", created.ID)
	if err != nil || len(got.Bytes) != 3 || got.Bytes[0] != 9 {
		t.Fatalf("get after upsert: %v (bytes %v)", err, got.Bytes)
	}
	if _, err := ls.Get(ctx, "intruder", created.ID); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("foreign owner must not see the logo: %v", err)
	}
	// Node delete cascades the blob (FK ON DELETE CASCADE).
	if err := st.Delete(ctx, "u1", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.Get(ctx, "u1", created.ID); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("logo survived node delete: %v", err)
	}
}
```

> Harness-Namen (`newTestNodeStore`, Pool-Zugriff) an die Datei anpassen — sie hat funktionierende pgstore-Tests, deren Setup spiegeln. Läuft nur unter den Docker-gated pgstore-Tests (make ci).

- [ ] **Step 9: CI + Commit**

```bash
make ci
git add -A
git commit -m "feat(nodelogo): NodeLogoStore port + pgstore blob adapter + upload/get/delete usecases"
```

---

## Task 4: Serve-Endpoint `GET /nodes/{id}/logo` + Wiring

**Files:**
- Create: `internal/adapter/httpserver/webui_nodelogo.go`.
- Modify: `internal/adapter/httpserver/server.go` (Struct-Felder nahe Z.56, Route nahe Z.257), `cmd/flow-server/main.go` (Store + Wiring nahe Z.66/143).
- Test: `internal/adapter/httpserver/webui_nodelogo_test.go`.

**Interfaces:**
- Consumes: `usecase.GetNodeLogo` (Task 3).
- Produces: Route `GET /nodes/{id}/logo` (webAuth); Server-Felder `UploadNodeLogo usecase.UploadNodeLogo`, `DeleteNodeLogo usecase.DeleteNodeLogo`, `GetNodeLogo usecase.GetNodeLogo`; Handler `(*Server).handleWebNodeLogo`; Helfer `readLogoUpload(r *http.Request) ([]byte, error)` (von Task 5 konsumiert).

- [ ] **Step 1: Failing Handler-Test** — Create `internal/adapter/httpserver/webui_nodelogo_test.go` (Test-Harness der Nachbar-Tests spiegeln — wie `webui_nodes_test.go` seinen `*Server` + eingeloggte Session baut; dieselben Konstruktoren verwenden):

```go
func TestHandleWebNodeLogo_ServeETag304(t *testing.T) {
	// harness: server with FakeNodeStore + FakeNodeLogoStore, logged-in user "u1"
	// (mirror the setup used by the existing webui_nodes handler tests)
	srv, ts := newWebTestServer(t) // adapt to the file's harness helper
	defer ts.Close()

	// seed: node + logo
	ctx := context.Background()
	n, _ := domain.NewNode("n1", testUserID(t), "flow", "flow", time.Now())
	n.Kind = domain.KindEngagement
	_, _ = srv.nodeStoreForTest().Create(ctx, n) // adapt accessor to harness
	png := pngPixelBytes(t)                      // same 1x1 PNG helper as the usecase tests
	if _, err := srv.UploadNodeLogo.Execute(ctx, testUserID(t), "n1", png); err != nil {
		t.Fatal(err)
	}

	res := authedGet(t, ts, "/nodes/n1/logo") // adapt: harness's cookie-authed GET
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type %q", ct)
	}
	etag := res.Header.Get("ETag")
	if etag == "" || res.Header.Get("Cache-Control") == "" {
		t.Errorf("missing caching headers: etag=%q cc=%q", etag, res.Header.Get("Cache-Control"))
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Equal(body, png) {
		t.Error("served bytes differ from upload")
	}

	res2 := authedGetWithHeader(t, ts, "/nodes/n1/logo", "If-None-Match", etag)
	if res2.StatusCode != http.StatusNotModified {
		t.Errorf("conditional GET → %d, want 304", res2.StatusCode)
	}
	res2.Body.Close()

	res3 := authedGet(t, ts, "/nodes/ghost/logo")
	if res3.StatusCode != http.StatusNotFound {
		t.Errorf("missing logo → %d, want 404", res3.StatusCode)
	}
	res3.Body.Close()
}
```

> Die Helfer (`newWebTestServer`, `authedGet`, …) sind Platzhalter-NAMEN für das, was die bestehende webui-Handler-Testdatei bereits an Harness hat — deren Muster 1:1 übernehmen (inkl. wie sie den Server mit Fakes bestückt und die Session-Cookie-Auth macht). Der TEST-INHALT (Assertions, Status, Header, 304-Fluss) ist verbindlich.

- [ ] **Step 2: Run — fails** (Route/Handler/Felder fehlen)

Run: `go test ./internal/adapter/httpserver/ -run TestHandleWebNodeLogo`
Expected: FAIL (undefined / 404-statt-200).

- [ ] **Step 3: Handler + Upload-Helfer** — Create `internal/adapter/httpserver/webui_nodelogo.go`:

```go
package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/serverkraken/flow/internal/usecase"
)

// handleWebNodeLogo serves the node's uploaded logo blob. The <img> URLs carry
// ?v={LogoRef}, so each URL's content is immutable — served with a strong ETag
// (the content hash) plus long-lived private caching; If-None-Match short-
// circuits to 304.
func (s *Server) handleWebNodeLogo(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	logo, err := s.GetNodeLogo.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	etag := `"` + logo.Ref + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", logo.Mime)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(logo.Bytes)
}

// readLogoUpload pulls the optional multipart "logo" file field: nil bytes when
// the user picked no file. Reads at most MaxNodeLogoBytes+1 so an oversized
// upload fails validation instead of ballooning memory.
func readLogoUpload(r *http.Request) ([]byte, error) {
	f, hdr, err := r.FormFile("logo")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if hdr == nil || hdr.Filename == "" || hdr.Size == 0 {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(f, usecase.MaxNodeLogoBytes+1))
}
```

- [ ] **Step 4: Server-Felder + Route** — `internal/adapter/httpserver/server.go`: bei den Node-Usecases (nahe `SetCountsTowardTarget`, Z.56):

```go
	UploadNodeLogo usecase.UploadNodeLogo
	DeleteNodeLogo usecase.DeleteNodeLogo
	GetNodeLogo    usecase.GetNodeLogo
```

Route nach `GET /nodes/{id}/head` (Z.257):

```go
	mux.Handle("GET /nodes/{id}/logo", s.webAuth(http.HandlerFunc(s.handleWebNodeLogo)))
```

- [ ] **Step 5: main-Wiring** — `cmd/flow-server/main.go`: bei den Stores (nahe Z.66):

```go
	nodeLogoStore := pgstore.NewNodeLogoStore(pool)
```

Bei den Usecases (nahe `SetCountsTowardTarget:`, Z.144):

```go
	UploadNodeLogo: usecase.UploadNodeLogo{Nodes: nodeStore, Logos: nodeLogoStore, Clock: clock},
	DeleteNodeLogo: usecase.DeleteNodeLogo{Nodes: nodeStore, Logos: nodeLogoStore, Clock: clock},
	GetNodeLogo:    usecase.GetNodeLogo{Logos: nodeLogoStore},
```

(Identifier `pool`/`clock` an die dortigen Namen anpassen — dieselben wie bei `nodeStore`/`SetCountsTowardTarget`.)

- [ ] **Step 6: Run — passes**

Run: `go test ./internal/adapter/httpserver/ -run TestHandleWebNodeLogo -v`
Expected: PASS (200+Header, 304, 404).

- [ ] **Step 7: CI + Commit**

```bash
make ci
git add -A
git commit -m "feat(webui): GET /nodes/{id}/logo blob serving with ETag/immutable caching + wiring"
```

---

## Task 5: Node-Formular — Icon-Picker + Logo-Upload/Entfernen + i18n + REST-Parität

**Files:**
- Modify: `internal/adapter/webui/node_tree_vm.go` (Z.112-119), `internal/adapter/webui/nodes.templ` (Form Z.122, nach Glyph-Block Z.201-209), `internal/adapter/httpserver/webui_nodes.go` (Z.19-35, Create Z.178-230, Edit Z.232-268, Update Z.270-325), `internal/adapter/httpserver/worktime.go` (createNodeReq ~Z.176-183, updateProjReq Z.305-314 + beide Execute-Aufrufe), `internal/i18n/catalog_de.go` + `catalog_en.go`.
- Test: `internal/adapter/httpserver/webui_nodes_test.go` (anhängen).

**Interfaces:**
- Consumes: `usecase.ValidateNodeLogo`, `s.UploadNodeLogo`/`s.DeleteNodeLogo`, `readLogoUpload` (Tasks 3/4), `webui.NodeIcon` + `domain.NodeIcons` (Tasks 1/2).
- Produces: `webui.NodeFormValues.Icon string`; Formfelder `icon` (Radio), `logo` (file), `logoRemove` (Checkbox); i18n-Keys `node.icon`, `node.logo`, `node.logo.remove`, `node.logo.hint`, `node.err.logo`, `node.err.logoType`, `node.err.logoSize`; REST-JSON-Feld `icon` in create/update.

- [ ] **Step 1: Failing Handler-Test** — an `internal/adapter/httpserver/webui_nodes_test.go` anhängen (multipart-POST; Harness der Datei spiegeln):

```go
func TestWebNodeForm_IconAndLogo(t *testing.T) {
	// harness: same as the existing create/update form tests in this file
	srv, ts := newWebTestServer(t) // adapt to the file's harness helper

	// multipart create: name + icon + logo file
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Iconic")
	_ = mw.WriteField("kind", "engagement")
	_ = mw.WriteField("status", "active")
	_ = mw.WriteField("icon", "rocket")
	fw, _ := mw.CreateFormFile("logo", "logo.png")
	_, _ = fw.Write(pngPixelBytes(t))
	_ = mw.Close()
	res := authedPost(t, ts, "/nodes", mw.FormDataContentType(), &buf) // adapt
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create → %d", res.StatusCode)
	}

	n := findNodeByName(t, srv, "Iconic") // adapt: read back via store/usecase
	if n.Icon != "rocket" {
		t.Errorf("icon = %q, want rocket", n.Icon)
	}
	if len(n.LogoRef) != 12 {
		t.Errorf("logoRef = %q, want 12-hex hash (logo stored on create)", n.LogoRef)
	}

	// multipart update: remove the logo via checkbox
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	_ = mw2.WriteField("name", "Iconic")
	_ = mw2.WriteField("kind", "engagement")
	_ = mw2.WriteField("status", "active")
	_ = mw2.WriteField("icon", "rocket")
	_ = mw2.WriteField("logoRemove", "1")
	_ = mw2.Close()
	res2 := authedPost(t, ts, "/nodes/"+n.ID, mw2.FormDataContentType(), &buf2)
	if res2.StatusCode != http.StatusSeeOther {
		t.Fatalf("update → %d", res2.StatusCode)
	}
	n2 := getNode(t, srv, n.ID)
	if n2.LogoRef != "" {
		t.Errorf("logoRef = %q after remove, want empty", n2.LogoRef)
	}

	// bad logo type rejects the whole create (400 re-render, no node created)
	var buf3 bytes.Buffer
	mw3 := multipart.NewWriter(&buf3)
	_ = mw3.WriteField("name", "BadLogo")
	_ = mw3.WriteField("kind", "engagement")
	fw3, _ := mw3.CreateFormFile("logo", "evil.svg")
	_, _ = fw3.Write([]byte("<svg onload=alert(1)></svg>"))
	_ = mw3.Close()
	res3 := authedPost(t, ts, "/nodes", mw3.FormDataContentType(), &buf3)
	if res3.StatusCode != http.StatusBadRequest {
		t.Errorf("svg upload → %d, want 400", res3.StatusCode)
	}
}
```

> Wieder: Harness-Helfer an die Datei anpassen; Assertions verbindlich. Zusätzlich einen GET-Test auf `/nodes/{id}/edit` anhängen, der prüft, dass die Seite `name="icon"` und (bei gesetztem LogoRef) `logoRemove` enthält.

- [ ] **Step 2: Run — fails** (`icon`-Feld wird nicht gelesen, Logo-Fluss existiert nicht)

Run: `go test ./internal/adapter/httpserver/ -run TestWebNodeForm_IconAndLogo`
Expected: FAIL.

- [ ] **Step 3: Form-VM + Reader** — `internal/adapter/webui/node_tree_vm.go` Z.115:

```go
	Color, Glyph, Icon               string
```

`internal/adapter/httpserver/webui_nodes.go` `nodeFormValues` (Z.29): nach `Glyph:` ergänzen:

```go
		Icon:         r.FormValue("icon"),
```

`handleWebNodeEdit` vals (Z.247): nach `Glyph: n.Glyph,` ergänzen:

```go
		Icon:        n.Icon,
```

- [ ] **Step 4: i18n-Keys** (de+en Parität) — in beide Kataloge, beim `node.*`-Block:

```
"node.icon":         "Icon"                                         / "Icon"
"node.logo":         "Logo"                                         / "Logo"
"node.logo.remove":  "Logo entfernen"                               / "Remove logo"
"node.logo.hint":    "PNG, JPEG oder WebP · max. 512 KB · ersetzt Icon und Glyph" / "PNG, JPEG or WebP · max 512 KB · replaces icon and glyph"
"node.err.logo":     "Logo-Upload fehlgeschlagen"                   / "Logo upload failed"
"node.err.logoType": "Logo muss PNG, JPEG oder WebP sein"           / "Logo must be PNG, JPEG or WebP"
"node.err.logoSize": "Logo zu groß (max. 512 KB)"                   / "Logo too large (max 512 KB)"
```

- [ ] **Step 5: templ — multipart + Icon-Picker + Logo-Feld** — `internal/adapter/webui/nodes.templ`:

Form-Tag (Z.122) bekommt `enctype`:

```templ
	<form method="post" action={ nodeFormAction(editing) } enctype="multipart/form-data" hx-boost="false" class="max-w-2xl space-y-4 text-sm">
```

Nach dem Glyph-Block (Z.201-209) einfügen:

```templ
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.icon") }</label>
			<div class="flex flex-wrap gap-2">
				@nodeIconRadio("", d.Vals.Icon)
				for _, key := range domain.NodeIcons {
					@nodeIconRadio(key, d.Vals.Icon)
				}
			</div>
		</div>
		<div>
			<label class="block text-muted">{ components.T(ctx, "node.logo") }</label>
			if editing != nil && editing.LogoRef != "" {
				<div class="mb-2 flex items-center gap-3">
					<img src={ "/nodes/" + editing.ID + "/logo?v=" + editing.LogoRef } alt="" class="h-12 w-12 rounded-xl object-cover"/>
					<label class="flex items-center gap-2 text-[.82rem]">
						<input type="checkbox" name="logoRemove" value="1"/> { components.T(ctx, "node.logo.remove") }
					</label>
				</div>
			}
			<input type="file" name="logo" accept="image/png,image/jpeg,image/webp" class="w-full rounded-lg border border-line bg-surface px-3 py-2"/>
			<p class="mt-1 text-[.75rem] text-faint">{ components.T(ctx, "node.logo.hint") }</p>
		</div>
```

Am Datei-Ende die Radio-Component (Muster `nodeGlyphRadio` Z.280-295):

```templ
// nodeIconRadio renders one icon-picker cell; "" is the no-icon choice.
templ nodeIconRadio(key, current string) {
	<label class="cursor-pointer" title={ key }>
		if key == current {
			<input type="radio" name="icon" value={ key } checked class="peer sr-only"/>
		} else {
			<input type="radio" name="icon" value={ key } class="peer sr-only"/>
		}
		<span class="inline-flex h-8 w-8 items-center justify-center rounded border border-line text-muted peer-checked:ring-2 peer-checked:ring-blue peer-checked:text-ink">
			if key == "" {
				∅
			} else {
				@NodeIcon(key, "h-5 w-5")
			}
		</span>
	</label>
}
```

- [ ] **Step 6: Create-Handler** — `internal/adapter/httpserver/webui_nodes.go` `handleWebNodeCreate`: nach dem `rerr`-Check (Z.191-194) den Logo-Pre-Check einschieben (Datei-Fehler MUSS vor CreateNode rejecten, sonst entsteht ein halber Node):

```go
	logoData, lerr := readLogoUpload(r)
	if lerr != nil {
		reRender(i18nT(r, "node.err.logo"))
		return
	}
	if len(logoData) > 0 {
		if _, verr := usecase.ValidateNodeLogo(logoData); verr != nil {
			reRender(logoErrMsg(r, verr))
			return
		}
	}
```

Im `CreateNodeInput` (Z.210-215) `Icon` mitgeben:

```go
		Color: vals.Color, Glyph: vals.Glyph, Icon: vals.Icon,
```

Nach dem `SetTags`-Block (Z.223-227), VOR dem `Emit` (damit der SSE-Reload das Logo schon sieht):

```go
	if len(logoData) > 0 {
		if _, err := s.UploadNodeLogo.Execute(r.Context(), u.ID, n.ID, logoData); err != nil {
			slog.WarnContext(r.Context(), "webui: upload node logo failed", "nodeID", n.ID, "err", err)
		}
	}
```

Und den Fehler-Mapper (Datei-Ende):

```go
// logoErrMsg maps logo validation sentinels to i18n form errors.
func logoErrMsg(r *http.Request, err error) string {
	switch {
	case errors.Is(err, usecase.ErrLogoTooLarge):
		return i18nT(r, "node.err.logoSize")
	case errors.Is(err, usecase.ErrLogoBadType):
		return i18nT(r, "node.err.logoType")
	default:
		return i18nT(r, "node.err.logo")
	}
}
```

- [ ] **Step 7: Update-Handler** — `handleWebNodeUpdate`: denselben Pre-Check (readLogoUpload + ValidateNodeLogo → reRender) nach dem `rerr`-Check (Z.285-288) einschieben. Im `UpdateNodeInput` (Z.289-297) `Icon: vals.Icon,` ergänzen. Nach dem `SetCountsTowardTarget`-Block (Z.313-317), VOR `Emit`:

```go
	if r.FormValue("logoRemove") == "1" {
		if _, err := s.DeleteNodeLogo.Execute(r.Context(), u.ID, id); err != nil {
			slog.WarnContext(r.Context(), "webui: delete node logo failed", "nodeID", id, "err", err)
		}
	} else if len(logoData) > 0 {
		if _, err := s.UploadNodeLogo.Execute(r.Context(), u.ID, id, logoData); err != nil {
			slog.WarnContext(r.Context(), "webui: upload node logo failed", "nodeID", id, "err", err)
		}
	}
```

`handleWebNodeStatus` (Z.337-345): im `UpdateNodeInput` `Icon: cur.Icon,` ergänzen (full-replace darf das Icon nicht wegwischen!). Ebenso in `internal/adapter/httpserver/worktime.go` die beiden `UpdateNodeInput`-Aufrufe (Z.224-228: `Icon: p.Icon,`; Z.323-328: `Icon: req.Icon,`).

- [ ] **Step 8: REST-Parität** — `internal/adapter/httpserver/worktime.go`: `createNodeReq` + `updateProjReq` je um

```go
	Icon               string  `json:"icon"`
```

ergänzen und in den `CreateNodeInput`-/`UpdateNodeInput`-Aufrufen `Icon: req.Icon,` durchreichen.

- [ ] **Step 9: Generate + Web + Run — passes**

Run: `make generate && make web && go test ./internal/adapter/httpserver/ -run "TestWebNodeForm" -v`
Expected: PASS. (`make web`: neue Utility-Klassen wie `h-12`/`accept`-Input stammen aus `.templ` → app.css neu bauen und committen.)

- [ ] **Step 10: CI + Commit**

```bash
make ci
git add -A
git commit -m "feat(webui): node form icon picker + logo upload/remove (multipart) + REST icon parity"
```

---

## Task 6: Render-Priorität Upload > Icon > Glyph in Cockpit-Head + Projektliste

**Files:**
- Modify: `internal/adapter/webui/cockpit.templ` (`cockpitHex` Z.117-120), `internal/adapter/webui/nodes.templ` (`nodeGlyphSwatch` Z.86-94).
- Test: `internal/adapter/webui/cockpit_render_test.go` (anhängen; nutzt das dortige `renderToBuf`), `internal/adapter/webui/render_test.go` oder neue Assertions in `icons_test.go` für den Swatch.

**Interfaces:**
- Consumes: `NodeIcon` (Task 2), `domain.Node.Icon/LogoRef` (Task 1), bestehende `cockpitTileClass`/`glyphOrDefault`/`ColorHex`.
- Produces: keine neuen Symbole — geänderte Render-Semantik, durch Output-Tests gepinnt.

- [ ] **Step 1: Failing Render-Tests** — an `internal/adapter/webui/cockpit_render_test.go` anhängen:

```go
// Render priority: uploaded logo > icon > glyph — pinned for the cockpit head.
func TestCockpitHex_LogoIconGlyphPriority(t *testing.T) {
	ctx := context.Background()

	logo := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", LogoRef: "abc123def456", Icon: "rocket", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(logo, `/nodes/n1/logo?v=abc123def456`) {
		t.Errorf("logo-bearing node must render the <img> URL, got: %s", logo)
	}
	if strings.Contains(logo, "<svg") || strings.Contains(logo, "◈") {
		t.Error("logo must suppress icon and glyph")
	}
	if !strings.Contains(logo, "clip-path") {
		t.Error("uploaded logo must render with the hexagonal clip")
	}

	icon := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", Icon: "rocket", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(icon, "<svg") {
		t.Errorf("icon-bearing node must render inline SVG, got: %s", icon)
	}
	if strings.Contains(icon, "◈") {
		t.Error("icon must suppress the glyph")
	}

	glyph := renderToBuf(t, ctx, cockpitHex(domain.Node{ID: "n1", Glyph: "◈", Color: "cyan"}))
	if !strings.Contains(glyph, "◈") {
		t.Errorf("fallback must render the glyph, got: %s", glyph)
	}
}
```

Gleiches Muster für die Liste (in derselben Datei oder `icons_test.go`, je nach Paket — `nodeGlyphSwatch` ist unexportiert → Test muss ins `webui`-Paket-interne Testfile, Muster `node_tree_vm_internal_test.go`):

```go
func TestNodeGlyphSwatch_LogoIconGlyphPriority(t *testing.T) {
	ctx := context.Background()
	logo := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", LogoRef: "abc123def456", Icon: "rocket", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(logo, "/nodes/n1/logo?v=abc123def456") || strings.Contains(logo, "<svg") {
		t.Errorf("logo wins over icon in the tree row, got: %s", logo)
	}
	icon := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", Icon: "rocket", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(icon, "<svg") || strings.Contains(icon, "◆") {
		t.Errorf("icon wins over glyph in the tree row, got: %s", icon)
	}
	if !strings.Contains(icon, "#7dcfff") {
		t.Errorf("icon must be tinted in the node color, got: %s", icon)
	}
	glyph := renderToBuf(t, ctx, nodeGlyphSwatch(domain.Node{ID: "n1", Glyph: "◆", Color: "cyan"}))
	if !strings.Contains(glyph, "◆") {
		t.Errorf("glyph fallback broken, got: %s", glyph)
	}
}
```

> `renderToBuf` existiert in `cockpit_render_test.go`; wenn dessen Paket `webui` (intern) ist, beide Tests dort anhängen; ist es `webui_test`, den Swatch-Test in ein internes Testfile legen und `renderToBuf` dort minimal spiegeln.

- [ ] **Step 2: Run — fails**

Run: `go test ./internal/adapter/webui/ -run "Priority"`
Expected: FAIL (aktuell rendert immer Glyph).

- [ ] **Step 3: `cockpitHex` umbauen** — `internal/adapter/webui/cockpit.templ` Z.117-120 ersetzen:

```templ
// cockpitHex renders the node's identity tile — priority: uploaded logo
// (hex-cropped <img>) > whitelisted icon (tile-tinted SVG) > glyph.
templ cockpitHex(n domain.Node) {
	if n.LogoRef != "" {
		<img
			src={ "/nodes/" + n.ID + "/logo?v=" + n.LogoRef }
			alt=""
			class="h-11 w-11 shrink-0 object-cover"
			style="clip-path:polygon(25% 0,75% 0,100% 50%,75% 100%,25% 100%,0 50%)"
		/>
	} else if n.Icon != "" {
		<span class={ "grid place-items-center h-11 w-11 rounded-xl shrink-0", cockpitTileClass(n.Color) } aria-hidden="true">
			@NodeIcon(n.Icon, "h-6 w-6")
		</span>
	} else {
		<span class={ "grid place-items-center h-11 w-11 rounded-xl text-lg shrink-0", cockpitTileClass(n.Color) } aria-hidden="true">{ glyphOrDefault(n.Glyph) }</span>
	}
}
```

(Der hexagonale Crop kommt aus Spec §5.2; inline `style` statt Tailwind-Arbitrary, damit kein neuer CSS-Build-Pfad nötig ist. `cockpitTileClass` liefert Wash+Textfarbe — das SVG tintet via currentColor mit.)

- [ ] **Step 4: `nodeGlyphSwatch` umbauen** — `internal/adapter/webui/nodes.templ` Z.86-94 ersetzen:

```templ
// nodeGlyphSwatch renders the node's identity mark in list rows — priority:
// uploaded logo > icon (tinted in the node color) > color dot + glyph.
templ nodeGlyphSwatch(n domain.Node) {
	if n.LogoRef != "" {
		<img src={ "/nodes/" + n.ID + "/logo?v=" + n.LogoRef } alt="" class="h-5 w-5 shrink-0 rounded object-cover"/>
	} else if n.Icon != "" {
		if ColorHex(n.Color) != "" {
			<span class="shrink-0" style={ "color:" + ColorHex(n.Color) }>
				@NodeIcon(n.Icon, "h-4 w-4")
			</span>
		} else {
			<span class="shrink-0 text-faint">
				@NodeIcon(n.Icon, "h-4 w-4")
			</span>
		}
	} else {
		if ColorHex(n.Color) != "" {
			<span class="inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={ "background-color:" + ColorHex(n.Color) }></span>
		}
		if n.Glyph != "" {
			<span class="shrink-0 font-mono text-faint">{ n.Glyph }</span>
		}
	}
}
```

- [ ] **Step 5: Generate + Run — passes**

Run: `make generate && go test ./internal/adapter/webui/ -run "Priority|Cockpit" -v`
Expected: PASS (inkl. der bestehenden `TestCockpitHex_RendersGlyphAndClass` — falls der jetzt am geänderten Markup scheitert, seine Assertions auf den Glyph-Fallback-Zweig anpassen, Semantik bleibt: Glyph+Farbe rendern).

- [ ] **Step 6: Web + CI + Commit**

```bash
make web && make ci
git add -A
git commit -m "feat(webui): identity render priority upload>icon>glyph in cockpit head + node tree rows"
```

---

## Task 7: Wiring-Verifikation + Done-Gate + Holistic Review

- [ ] **Step 1: Wiring-Audit** — alles vom Composition-Root erreichbar:

```bash
rg -n "UploadNodeLogo|DeleteNodeLogo|GetNodeLogo" cmd/flow-server/main.go internal/adapter/httpserver/server.go internal/adapter/httpserver/webui_nodelogo.go internal/adapter/httpserver/webui_nodes.go
rg -n "nodes/\{id\}/logo" internal/adapter/httpserver/server.go
rg -n "NodeIcon\(" internal/adapter/webui/*.templ
```

Expected: main-Wiring (3 Usecases) + Server-Felder + Route + Handler-Aufrufe + beide templ-Render-Sites.

- [ ] **Step 2: Voll-CI** — `make generate && make web && make ci`. Expected: grün, Coverage ≥ 75 % (`*_templ.go` ausgeschlossen); Coverage-% notieren.

- [ ] **Step 3: Live-Done-Gate (Dev-Stack: Postgres + Dex)** — `make dev-up && make dev-run` (+ Login):
  1. Migration 0026 applied; bestehende Nodes rendern unverändert (Glyph-Fallback).
  2. Node-Form: Icon-Picker zeigt 40 SVGs + ∅; Icon wählen → Liste + Cockpit-Head zeigen das SVG im Knotenfarbton.
  3. Logo hochladen (PNG < 512 KB) → Cockpit-Head zeigt das Bild hex-gecroppt; Liste zeigt es rund; `curl -I` auf `/nodes/{id}/logo` liefert `ETag` + `Cache-Control: private, max-age=31536000, immutable`; zweiter Request mit `If-None-Match` → 304.
  4. Rejections: > 512 KB → Formfehler „Logo zu groß"; SVG/Textdatei → „Logo muss PNG, JPEG oder WebP sein"; Node wird dabei NICHT angelegt.
  5. „Logo entfernen"-Checkbox → zurück auf Icon; Icon auf ∅ → zurück auf Glyph.
  6. Node löschen → `node_logos`-Zeile weg (psql: `select count(*) from node_logos;`).
  7. REST: `PATCH/POST /api/v1/nodes` mit `"icon":"rocket"` round-tript; Node-JSON trägt `icon` + `logoRef`.
- [ ] **Step 4: Browser-Dogfood** (Soenne): Form-UX des Pickers (40er-Grid), Upload-Flow, Darstellung Dark+Light.
- [ ] **Step 5: Holistic Review (Opus)** — ganze Slice-Diff (BASE = `d27ded5`). Fokus: Upload-Validierungskette (sniff-vor-Vertrauen, Limit an JEDER Lesestelle), Owner-Scoping von Get/Serve (kein Cross-User-Logo-Leak), `UpdateNodeInput`-Full-Replace-Stellen (Icon darf nirgends weggewischt werden: webui update/status, REST update, worktime.go Z.224), CTE-Spaltenparität, ETag/304-Korrektheit, templ.Raw-Sicherheitsbegründung (nur embedded Assets).
- [ ] **Step 6: Follow-up-Commit nur bei Review-Findings** — `git commit -m "fix(logos): done-gate + holistic-review follow-ups"`.

---

## Self-Review (plan author)
**Spec-Coverage (§5.2 + Entscheidungen 2026-07-02):** `Icon`-Feld + kuratierte Whitelist → Task 1/2; `LogoRef` + Upload/Storage (DB-Blob, Migration, Limits, Format-Sniff) → Task 1/3; Render-Helper Upload>Icon>Glyph → Task 6 (Cockpit-Identität jetzt, Sidebar = Slice 5 per Spec §5.2); Node-Formular → Task 5; hexagonaler Crop → Task 6 Step 3; Wiring/Gate → Task 7. ✓
**Placeholder-Scan:** Die Test-Harness-Verweise (Tasks 4/5: `newWebTestServer` etc.) sind explizit als „Namen an die vorhandene Harness anpassen, Assertions verbindlich" markiert — kein stiller TODO; Task 2 Step 1 hat einen konkreten Fallback-Pfad für umbenannte Lucide-Icons; Task 3 Step 4 flaggt die Pool-Typ-Verifikation mit konkreter Alternative. Kein „TBD"/„add validation". ✓
**Typ-Konsistenz:** `Icon string`/`LogoRef string` (domain, beide Inputs, VM, DTOs); `domain.NodeLogo{NodeID,OwnerID,Mime,Ref,Bytes,UpdatedAt}` überall identisch; `ports.NodeLogoStore.Put/Get/Delete` == pgstore == Fake; `usecase.ValidateNodeLogo(data) (string, error)`; `UploadNodeLogo.Execute(ctx,owner,node,data) (domain.Node, error)`; `NodeIconSVG(key) string` + `NodeIcon(key, class)`; Formfelder `icon`/`logo`/`logoRemove`; Migration 0026. ✓
**Reuse:** Whitelist-Mechanik spiegelt `NodeGlyphs`/`ValidNodeGlyph`; Radio-Picker spiegelt `nodeGlyphRadio`; Drift-Guard spiegelt `TestColorHexCoversWholePalette`; Store-Fehler spiegeln `ErrNodeNotFound`-Muster; keine neuen NodeStore-Methoden (LogoRef fährt über das bestehende `Update`).
