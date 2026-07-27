# flow WebUI — Wissen-Fläche + Markdown-Parität Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Tool-agnostic:** This plan is written to be executed by Claude Code, Gemini CLI, or OpenAI Codex. It assumes **no** prior conversation, no Claude-only memory, and no Claude-only skills. Read `AGENTS.md` (created in Task 0) at the repo root first — it holds the repo conventions every task depends on.

**Goal:** Move the whole WebUI "Wissen" surface (category-specific document list + read page + editor with live preview) onto the existing `AppShell`, and upgrade the web Markdown renderer to feature-parity (GFM, footnotes, callouts, Chroma syntax-highlighting with two themes, wikilinks, frontmatter).

**Architecture:** Server-rendered Go (templ + htmx + Tailwind v4), hexagonal layers. Handlers in `internal/adapter/httpserver/webui_*.go` build typed viewmodels and render templ components from `internal/adapter/webui/`. One shared web Markdown renderer (`webui.RenderDocument`) feeds doc-view, project descriptions, and editor preview. Additive backend changes only: documents pagination + a web-only preview endpoint. No REST/TUI/MCP changes.

**Tech Stack:** Go 1.x, [templ](https://templ.guide) (`.templ` → generated `_templ.go`), htmx + htmx-sse, Tailwind v4 (standalone CLI), goldmark (+ `extension.GFM`, `extension.Footnote`), `github.com/yuin/goldmark-highlighting/v2` (new dep) bridging `github.com/alecthomas/chroma/v2`, bluemonday, Postgres via pgstore.

## Global Constraints

> These apply to **every** task. Values copied verbatim from the design spec `docs/superpowers/specs/2026-06-25-flow-webui-wissen-markdown-design.md` and the repo.

- **Branch:** all work on `rebuild` (worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`). Do **not** merge to `main`/`next`.
- **Routing is id-based:** `/wissen`, `/wissen/{id}`, `/wissen/neu`, `/wissen/{id}/bearbeiten`, `POST /wissen/preview`. NOT slug-based. No new slug-lookup usecase.
- **No browser popups:** never use `window.alert/confirm/prompt` or native form-validation bubbles. All confirms via `components.ConfirmDialog`. The `make verify-no-popups` guard (`scripts/verify-no-popups.sh`) must stay green.
- **i18n:** no hardcoded German display strings in `.templ` files. Add keys to the Go-map catalogs `internal/i18n/catalog_de.go` (primary, complete) and `internal/i18n/catalog_en.go` (stub). Call via `components.T(ctx, "key")` inside templ. (Catalogs are Go maps, NOT TOML.)
- **templ codegen:** after editing any `.templ`, run `templ generate` (or `make generate`); the generated `*_templ.go` must be committed. `make verify-generate` enforces no drift.
- **CSS build:** after Tailwind-affecting changes run `make web` (`tailwindcss --input web/tailwind.css --output internal/adapter/webui/static/app.css --minify`) and commit `internal/adapter/webui/static/app.css`. `make verify-css` (`scripts/verify-css.sh`) enforces no drift.
- **"Keine Monolithen":** one responsibility per file; new components/pages/handlers as their own files (see spec §3 file map).
- **Done gate per task:** `make ci` must be green = `lint verify-generate verify-css verify-no-popups cover build`. Coverage gate is **75 %** (`scripts/coverage-gate.sh`). Run **`make ci`** (lint included) — not just `go test`.
- **Commit style:** small, frequent commits; conventional-commit subject (`feat(webui): …`, `feat(pgstore): …`, `docs: …`). One coherent deliverable per commit.
- **SSE events:** the bus emits `document.created`, `document.updated`, `document.deleted`, `project.created`, `project.updated`. Live fragments use `hx-trigger="sse:document.created, sse:document.updated, sse:document.deleted"`.

### Verified interface reference (from the existing codebase)

Use these exact signatures; they already exist unless marked **NEW**.

```go
// internal/domain/document.go
type Document struct {
    ID        string
    OwnerID   string
    ProjectID *string
    Type      DocumentType
    Path      string
    Title     string
    Body      string
    Tags      []string
    Date      *time.Time
    Role      *string
    Extra     map[string]any
    CreatedAt time.Time
    UpdatedAt time.Time
}
const ( // DocumentType values
    DocDaily="daily"; DocProject="project"; DocFree="free"; DocAgent="agent"
    DocMemory="memory"; DocInstruction="instruction"; DocSkill="skill"; DocPlan="plan"
)
func ParseFrontmatter(body string) (tags []string, bodyStart int)            // domain/frontmatter.go
func ResolveWikilink(doc Document, target string, all []Document) (Document, bool) // exists; used in webui_docs.go:220

// internal/adapter/webui (existing)
type WikilinkResolver func(target string) (href, title string, ok bool)       // wikilink.go:22
func RenderDocument(src string, resolve WikilinkResolver) template.HTML        // wikilink.go:50
func RenderMarkdown(src string) template.HTML                                  // markdown.go (plain, legacy)
func ColorHex(name string) string                                             // projectstyle.go:21
func StaticHandler() http.Handler                                            // static.go ; mounted at /static/

// internal/adapter/webui/components (existing templ)
templ Base(active string, content templ.Component)                            // base.templ
templ AppShell(active string, breadcrumb, subnav, content templ.Component)    // appshell.templ:6
templ Pagination(p PageNav)                                                   // pagination.templ:42
type PageNav struct { Page, Total, PageSize int; BaseHref string }            // pagination.templ:7
templ Dialog(id, titleKey string, body templ.Component)                       // dialog.templ:6
templ ConfirmDialog(spec ConfirmSpec)                                         // dialog.templ:46
type ConfirmSpec struct { ID, TitleKey, BodyKey, ConfirmLabelKey string; ConfirmAttrs templ.Attributes }
func T(ctx context.Context, key string) string                                // components/i18nhelper.go
func Tn(ctx context.Context, key string, n int) string

// internal/usecase (existing; injected on *httpserver.Server)
ListDocuments.Execute(ctx, ownerID string, projectID *string, tags []string) ([]domain.Document, error)
SearchDocuments.Execute(ctx, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error)
ListTags.Execute(ctx, ownerID string) ([]domain.TagCount, error)
GetDocument.Execute(ctx, ownerID, id string) (domain.Document, error)
BacklinksDocument.Execute(ctx, ownerID, docID string) ([]domain.BacklinkRef, error)
CreateDocument.Execute(ctx, ownerID string, in CreateDocumentInput) (domain.Document, error)
UpdateDocument.Execute(ctx, ownerID, id string, in UpdateDocumentInput) (domain.Document, error)
DeleteDocument.Execute(ctx, ownerID, id string) error
type CreateDocumentInput struct { Type domain.DocumentType; ProjectID *string; Path, Title, Body string }
type UpdateDocumentInput struct { Title, Body string }

// internal/ports/ports.go : DocumentStore (existing)
List(ctx, ownerID string, projectID *string, tags ...string) ([]domain.Document, error)
// pgstore impl: internal/adapter/pgstore/documents.go:69

// Web routes are registered in internal/adapter/httpserver/server.go:176 with s.webAuth(...)
// Existing AppShell page pattern (mirror this): internal/adapter/webui/frei.templ + frei_vm.go
//   + handler internal/adapter/httpserver/webui_dayoffs.go:114
```

---

## Task mapping to spec §13

Spec listed 7 conceptual slices; this plan right-sizes them into 10 tasks:
spec-1 → Tasks 1+2+3 (renderer core, callouts, chroma); spec-2 → folded into Task 3;
new Task 0 (AGENTS.md) + Task 4 (kind-style + prose component, supports pages);
spec-3 → Task 5; spec-4 → Task 6; spec-5 → Task 7; spec-6 → Task 8; spec-7 → Task 9.

---

## Task 0: AGENTS.md — repo conventions for any executor

**Files:**
- Create: `AGENTS.md` (repo root)

**Interfaces:**
- Consumes: nothing.
- Produces: a conventions file every later task references. (Codex auto-reads `AGENTS.md`; Gemini reads it when pointed at it; Claude reads it on request.)

- [ ] **Step 1: Write `AGENTS.md`**

```markdown
# AGENTS.md — flow (rebuild branch)

Conventions for any coding agent (Claude Code / Gemini CLI / Codex) working in this repo.

## Build / test / lint
- `make ci` = `lint verify-generate verify-css verify-no-popups cover build`. Must be green before any task is "done".
- `make test` runs Go tests; `make cover` enforces the 75% coverage gate (`scripts/coverage-gate.sh`).
- `make generate` runs `templ generate`; commit generated `*_templ.go`. `make verify-generate` checks for drift.
- `make web` rebuilds Tailwind CSS into `internal/adapter/webui/static/app.css`; commit it. `make verify-css` checks for drift.
- `make verify-no-popups` fails if `window.alert/confirm/prompt` appears in templ/JS.

## Architecture (hexagonal)
- `internal/domain` — entities + pure logic, no I/O.
- `internal/ports` — interfaces (stores, buses).
- `internal/usecase` — application services (`Execute(...)`).
- `internal/adapter/...` — drivers: `httpserver` (HTTP + web handlers), `webui` (templ components), `pgstore` (Postgres), `tui`.
- Wiring is in `cmd/flow` + `internal/adapter/httpserver/server.go`.

## WebUI conventions
- templ + htmx + Tailwind, server-rendered. No SPA, no Node runtime.
- Pages: `XPage(vm)` wraps `components.Base(active, body)` → `components.AppShell(active, breadcrumb, subnav, content)`.
- htmx fragments: `XFragment(vm)`; SSE live via `hx-ext="sse"` (body) + `hx-trigger="sse:<event>"` on containers.
- i18n: NO hardcoded display strings; add keys to `internal/i18n/catalog_de.go` + `catalog_en.go`; use `components.T(ctx, "key")`.
- NO browser popups; confirms via `components.ConfirmDialog`.
- One responsibility per file ("keine Monolithen").

## Dev stack (live verification)
- `make dev-up` starts Postgres + Dex (OIDC). `make dev-run` runs the server. `make dev-token` mints a bearer token. (Cookie auth for the browser.)
- Live done-gate: each new route returns expected status; SSE reflects create/update/delete.

## TDD
- Write a failing test, run it (see it fail), implement minimal code, run it (see it pass), commit. Small commits.
```

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: add AGENTS.md with repo conventions for coding agents"
```

---

## Task 1: Markdown renderer core — GFM + Footnotes + extended sanitize

**Files:**
- Modify: `internal/adapter/webui/wikilink.go` (the `RenderDocument` goldmark builder + `getDocPolicy`)
- Test: `internal/adapter/webui/markdown_test.go` (add cases)

**Interfaces:**
- Consumes: `RenderDocument(src string, resolve WikilinkResolver) template.HTML`, `domain.ParseFrontmatter`.
- Produces: same `RenderDocument` signature, now emitting GFM + footnotes. Later tasks (callouts, chroma) extend the **same** goldmark builder.

- [ ] **Step 1: Write failing tests for GFM + footnotes**

Add to `internal/adapter/webui/markdown_test.go`:

```go
func resolveNone(target string) (string, string, bool) { return "", "", false }

func TestRenderDocument_GFMTable(t *testing.T) {
    out := string(RenderDocument("| A | B |\n|---|---|\n| 1 | 2 |\n", resolveNone))
    if !strings.Contains(out, "<table") || !strings.Contains(out, "<td") {
        t.Fatalf("expected GFM table, got: %s", out)
    }
}

func TestRenderDocument_Tasklist(t *testing.T) {
    out := string(RenderDocument("- [x] done\n- [ ] todo\n", resolveNone))
    if !strings.Contains(out, `type="checkbox"`) {
        t.Fatalf("expected task checkboxes, got: %s", out)
    }
}

func TestRenderDocument_Strikethrough(t *testing.T) {
    out := string(RenderDocument("~~gone~~\n", resolveNone))
    if !strings.Contains(out, "<del>") {
        t.Fatalf("expected <del>, got: %s", out)
    }
}

func TestRenderDocument_Footnote(t *testing.T) {
    out := string(RenderDocument("Text[^1]\n\n[^1]: note\n", resolveNone))
    if !strings.Contains(out, `class="footnotes"`) && !strings.Contains(out, "footnote-ref") {
        t.Fatalf("expected footnote markup, got: %s", out)
    }
}

func TestRenderDocument_XSSStripped(t *testing.T) {
    out := string(RenderDocument("<script>alert(1)</script>\n\n[ok](javascript:alert(1))\n", resolveNone))
    if strings.Contains(out, "<script") || strings.Contains(out, "javascript:") {
        t.Fatalf("XSS not stripped: %s", out)
    }
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/adapter/webui/ -run TestRenderDocument -v`
Expected: FAIL (no `<table>`, no checkbox, no `<del>`, no footnote markup).

- [ ] **Step 3: Add GFM + Footnote extensions to the goldmark builder**

In `internal/adapter/webui/wikilink.go`, in `RenderDocument`, extend the `goldmark.New(...)` call. Replace the current construction with:

```go
import (
    // add:
    "github.com/yuin/goldmark/extension"
)

gm := goldmark.New(
    goldmark.WithExtensions(
        extension.GFM,       // tables, strikethrough, tasklists, autolinks
        extension.Footnote,  // footnotes
    ),
    goldmark.WithParserOptions(
        parser.WithInlineParsers(
            util.Prioritized(&wikiLinkParser{}, 100),
        ),
    ),
)
```

- [ ] **Step 4: Extend the bluemonday policy for GFM + footnote markup**

In `getDocPolicy()` (same file), after the existing `AllowAttrs` lines and before assigning `docPolicy = p`, add:

```go
// GFM tables
p.AllowAttrs("align").OnElements("td", "th")
p.AllowElements("table", "thead", "tbody", "tr", "th", "td")
// GFM task list checkboxes (rendered disabled)
p.AllowAttrs("type", "checked", "disabled").OnElements("input")
p.AllowAttrs("class").OnElements("li", "ul")
// Footnotes
p.AllowElements("sup", "section")
p.AllowAttrs("id").OnElements("li", "sup", "a", "section")
p.AllowAttrs("class").OnElements("section", "ol", "li", "sup")
p.AllowAttrs("href").OnElements("a") // already present; harmless
p.AllowAttrs("role", "aria-label").OnElements("a", "section")
```

> Note: `bluemonday.UGCPolicy()` already allows `<del>`, `<table>` children may need the explicit `AllowElements` above to survive. Re-run tests after this step.

- [ ] **Step 5: Run tests, verify they pass**

Run: `go test ./internal/adapter/webui/ -run TestRenderDocument -v`
Expected: PASS (all 5).

- [ ] **Step 6: Run the full package + lint**

Run: `go test ./internal/adapter/webui/... && go vet ./internal/adapter/webui/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add internal/adapter/webui/wikilink.go internal/adapter/webui/markdown_test.go go.mod go.sum
git commit -m "feat(webui): GFM tables/tasklists/strikethrough + footnotes in RenderDocument"
```

---

## Task 2: Callout extension (GitHub-alert syntax)

**Files:**
- Create: `internal/adapter/webui/markdown_callout.go`
- Test: `internal/adapter/webui/markdown_callout_test.go`
- Modify: `internal/adapter/webui/wikilink.go` (register the callout AST transformer + node renderer)

**Interfaces:**
- Consumes: goldmark AST (`ast.Blockquote`), the `RenderDocument` builder from Task 1.
- Produces: blockquotes starting with `[!NOTE]` / `[!TIP]` / `[!WARNING]` / `[!IMPORTANT]` / `[!DANGER]` render as `<div class="callout callout-<kind>">` with a titled header. Consumed visually by Task 3/4 CSS.

- [ ] **Step 1: Write failing tests**

`internal/adapter/webui/markdown_callout_test.go`:

```go
package webui

import (
    "strings"
    "testing"
)

func TestCallout_Note(t *testing.T) {
    out := string(RenderDocument("> [!NOTE]\n> hello\n", resolveNone))
    if !strings.Contains(out, `class="callout callout-note"`) {
        t.Fatalf("expected callout-note div, got: %s", out)
    }
    if !strings.Contains(out, "hello") {
        t.Fatalf("callout body lost: %s", out)
    }
}

func TestCallout_AllKinds(t *testing.T) {
    for _, k := range []string{"note", "tip", "warning", "important", "danger"} {
        src := "> [!" + strings.ToUpper(k) + "]\n> body\n"
        out := string(RenderDocument(src, resolveNone))
        if !strings.Contains(out, "callout-"+k) {
            t.Fatalf("kind %s not rendered: %s", k, out)
        }
    }
}

func TestCallout_PlainBlockquoteUnchanged(t *testing.T) {
    out := string(RenderDocument("> just a quote\n", resolveNone))
    if strings.Contains(out, "callout") {
        t.Fatalf("plain blockquote should not be a callout: %s", out)
    }
    if !strings.Contains(out, "<blockquote") {
        t.Fatalf("expected blockquote, got: %s", out)
    }
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/adapter/webui/ -run TestCallout -v`
Expected: FAIL (no callout markup).

- [ ] **Step 3: Implement the callout AST transformer + renderer**

`internal/adapter/webui/markdown_callout.go`:

```go
package webui

import (
    "regexp"
    "strings"

    "github.com/yuin/goldmark/ast"
    "github.com/yuin/goldmark/parser"
    "github.com/yuin/goldmark/renderer"
    "github.com/yuin/goldmark/text"
    "github.com/yuin/goldmark/util"
)

var calloutKinds = map[string]bool{
    "note": true, "tip": true, "warning": true, "important": true, "danger": true,
}

var calloutRe = regexp.MustCompile(`^\[!([A-Za-z]+)\]\s*(.*)$`)

// kindCalloutNode marks a blockquote that opened with [!KIND].
var kindCallout = ast.NewNodeKind("Callout")

type calloutNode struct {
    ast.BaseBlock
    Kind  string
    Title string
}

func (n *calloutNode) Kind() ast.NodeKind          { return kindCallout }
func (n *calloutNode) Dump(src []byte, lvl int)    { ast.DumpHelper(n, src, lvl, nil, nil) }

// calloutTransformer rewrites qualifying blockquotes into calloutNodes.
type calloutTransformer struct{}

func (calloutTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
    src := reader.Source()
    var targets []*ast.Blockquote
    _ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
        if entering {
            if bq, ok := n.(*ast.Blockquote); ok {
                targets = append(targets, bq)
            }
        }
        return ast.WalkContinue, nil
    })
    for _, bq := range targets {
        first := bq.FirstChild()
        if first == nil || first.Type() != ast.TypeBlock {
            continue
        }
        // The first line text of the first paragraph.
        line := firstLineText(first, src)
        m := calloutRe.FindStringSubmatch(strings.TrimSpace(line))
        if m == nil {
            continue
        }
        kind := strings.ToLower(m[1])
        if !calloutKinds[kind] {
            continue
        }
        cn := &calloutNode{Kind: kind, Title: strings.TrimSpace(m[2])}
        // Move blockquote children (minus the marker line) under the callout.
        stripFirstLine(first, src)
        for c := bq.FirstChild(); c != nil; {
            next := c.NextSibling()
            bq.RemoveChild(bq, c)
            cn.AppendChild(cn, c)
            c = next
        }
        bq.Parent().ReplaceChild(bq.Parent(), bq, cn)
    }
}

// firstLineText returns the raw text of the first text segment of a node.
func firstLineText(n ast.Node, src []byte) string {
    if n.Lines() != nil && n.Lines().Len() > 0 {
        seg := n.Lines().At(0)
        return string(seg.Value(src))
    }
    return ""
}

// stripFirstLine drops the first source line of a block's text segments.
func stripFirstLine(n ast.Node, _ []byte) {
    if n.Lines() == nil || n.Lines().Len() == 0 {
        return
    }
    lines := n.Lines()
    // Rebuild without the first segment.
    rebuilt := text.NewSegments()
    for i := 1; i < lines.Len(); i++ {
        rebuilt.Append(lines.At(i))
    }
    n.SetLines(rebuilt)
}

// --- renderer ---

type calloutHTMLRenderer struct{}

func (r *calloutHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
    reg.Register(kindCallout, r.render)
}

var calloutGlyph = map[string]string{
    "note": "●", "tip": "✓", "warning": "▲", "important": "★", "danger": "✗",
}

func (r *calloutHTMLRenderer) render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*calloutNode)
    if entering {
        _, _ = w.WriteString(`<div class="callout callout-` + n.Kind + `">`)
        _, _ = w.WriteString(`<p class="callout-title"><span class="callout-glyph" aria-hidden="true">`)
        _, _ = w.WriteString(calloutGlyph[n.Kind])
        _, _ = w.WriteString(`</span> `)
        title := n.Title
        if title == "" {
            title = strings.ToUpper(n.Kind[:1]) + n.Kind[1:]
        }
        _, _ = w.WriteString(util.EscapeHTML([]byte(title)) |> string) // see note below
        _, _ = w.WriteString(`</p>`)
    } else {
        _, _ = w.WriteString(`</div>`)
    }
    return ast.WalkContinue, nil
}
```

> **Implementation note:** Go has no `|>` operator — write the title line as
> `w.Write(util.EscapeHTML([]byte(title)))`. (Shown above only to flag the escape call; use `util.EscapeHTML`.) Keep child rendering by returning `ast.WalkContinue` (not `WalkSkipChildren`) so the inner paragraphs render normally.

- [ ] **Step 4: Register the transformer + renderer in `RenderDocument`**

In `internal/adapter/webui/wikilink.go`, extend the builder from Task 1:

```go
gm := goldmark.New(
    goldmark.WithExtensions(extension.GFM, extension.Footnote),
    goldmark.WithParserOptions(
        parser.WithInlineParsers(util.Prioritized(&wikiLinkParser{}, 100)),
        parser.WithASTTransformers(util.Prioritized(calloutTransformer{}, 0)),
    ),
)
gm.Renderer().AddOptions(
    renderer.WithNodeRenderers(
        util.Prioritized(&wikiLinkHTMLRenderer{resolve: resolve}, 100),
        util.Prioritized(&calloutHTMLRenderer{}, 100),
    ),
)
```

- [ ] **Step 5: Allow callout markup in the sanitizer**

In `getDocPolicy()` add (callout containers are `div`/`p`/`span` with `class`):

```go
p.AllowElements("div")
p.AllowAttrs("class").OnElements("div", "p")
p.AllowAttrs("aria-hidden").OnElements("span")
```

- [ ] **Step 6: Run tests, verify they pass**

Run: `go test ./internal/adapter/webui/ -run 'TestCallout|TestRenderDocument' -v`
Expected: PASS (callouts + Task 1 still green).

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/webui/markdown_callout.go internal/adapter/webui/markdown_callout_test.go internal/adapter/webui/wikilink.go
git commit -m "feat(webui): GitHub-alert callouts (NOTE/TIP/WARNING/IMPORTANT/DANGER) in markdown"
```

---

## Task 3: Chroma syntax-highlighting + two theme stylesheets

**Files:**
- Create: `internal/adapter/webui/markdown_chroma.go` (wires goldmark-highlighting with classes)
- Create: `internal/adapter/webui/chromacss.go` (generates the combined scoped `chroma.css`)
- Create: `internal/adapter/webui/chromacss_test.go` (up-to-date guard)
- Create: `internal/adapter/webui/static/chroma.css` (generated, committed, embedded)
- Modify: `internal/adapter/webui/wikilink.go` (add highlighting extension)
- Modify: `internal/adapter/webui/components/base.templ` (link `chroma.css`)
- Modify: `getDocPolicy()` for chroma spans/pre/code classes

**Interfaces:**
- Consumes: goldmark builder (Tasks 1–2), chroma styles `github` (light) + `github-dark` (dark).
- Produces: fenced code blocks render as `<pre class="chroma">…<span class="…">` (class-based, **no inline styles**); `static/chroma.css` scopes light vs dark under `:root` / `:root[data-theme="dark"]`.

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/yuin/goldmark-highlighting/v2@latest
go mod tidy
```
Expected: `go.mod` gains `goldmark-highlighting/v2` (chroma/v2 is already present).

- [ ] **Step 2: Write the chroma CSS generator + its guard test**

`internal/adapter/webui/chromacss.go`:

```go
package webui

import (
    "bytes"
    "strings"

    "github.com/alecthomas/chroma/v2"
    chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
    "github.com/alecthomas/chroma/v2/styles"
)

// chromaLightStyle / chromaDarkStyle are the two committed themes. Change here
// to retune; regenerate chroma.css (go generate) afterwards.
const (
    chromaLightStyle = "github"
    chromaDarkStyle  = "github-dark"
)

// GenerateChromaCSS returns the combined, theme-scoped chroma stylesheet:
// light rules under :root (default), dark rules under :root[data-theme="dark"].
func GenerateChromaCSS() string {
    var b strings.Builder
    b.WriteString("/* generated by chromacss.go — run `go generate ./internal/adapter/webui/...` */\n")
    b.WriteString(scopedCSS(chromaLightStyle, ":root"))
    b.WriteString(scopedCSS(chromaDarkStyle, `:root[data-theme="dark"]`))
    return b.String()
}

func scopedCSS(styleName, scope string) string {
    s := styles.Get(styleName)
    f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.ClassPrefix(""))
    var buf bytes.Buffer
    // WriteCSS emits ".chroma .k { ... }" rules; prefix each with the scope.
    _ = f.WriteCSS(&buf, s)
    out := buf.String()
    // Prefix every rule selector with the theme scope.
    var scoped strings.Builder
    for _, line := range strings.Split(out, "\n") {
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "/*") || trimmed == "" {
            scoped.WriteString(line + "\n")
            continue
        }
        if strings.Contains(line, "{") {
            scoped.WriteString(scope + " " + line + "\n")
        } else {
            scoped.WriteString(line + "\n")
        }
    }
    _ = chroma.Coalesce // keep import stable if unused elsewhere
    return scoped.String()
}
```

> If `chroma.Coalesce` triggers an "imported and not used" issue, drop that import and the no-op line.

`internal/adapter/webui/chromacss_test.go`:

```go
package webui

import (
    "os"
    "testing"
)

// TestChromaCSSUpToDate fails if static/chroma.css drifts from the generator.
// Regenerate with: go generate ./internal/adapter/webui/...
func TestChromaCSSUpToDate(t *testing.T) {
    want := GenerateChromaCSS()
    got, err := os.ReadFile("static/chroma.css")
    if err != nil {
        t.Fatalf("read chroma.css: %v (run go generate)", err)
    }
    if string(got) != want {
        t.Fatalf("static/chroma.css is stale — run: go generate ./internal/adapter/webui/...")
    }
}
```

- [ ] **Step 3: Add a generator entrypoint + go:generate directive**

Append to `internal/adapter/webui/chromacss.go`:

```go
//go:generate go run ./gen/chromacss
```

Create `internal/adapter/webui/gen/chromacss/main.go`:

```go
// Command chromacss writes the committed static/chroma.css.
package main

import (
    "os"

    "github.com/serverkraken/flow/internal/adapter/webui"
)

func main() {
    if err := os.WriteFile("internal/adapter/webui/static/chroma.css", []byte(webui.GenerateChromaCSS()), 0o644); err != nil {
        panic(err)
    }
}
```

> The `//go:generate` line lives in `chromacss.go` but the command writes a repo-root-relative path; run generation from the repo root: `go run ./internal/adapter/webui/gen/chromacss`.

- [ ] **Step 4: Generate and inspect the CSS**

Run:
```bash
go run ./internal/adapter/webui/gen/chromacss
head -20 internal/adapter/webui/static/chroma.css
```
Expected: a file beginning with the generated comment, then `:root .chroma .k { ... }` light rules followed by `:root[data-theme="dark"] .chroma .k { ... }` dark rules.

- [ ] **Step 5: Run the guard test, verify it passes**

Run: `go test ./internal/adapter/webui/ -run TestChromaCSSUpToDate -v`
Expected: PASS.

- [ ] **Step 6: Write a failing test for class-based highlighting in RenderDocument**

Add to `internal/adapter/webui/markdown_test.go`:

```go
func TestRenderDocument_CodeHighlightUsesClasses(t *testing.T) {
    out := string(RenderDocument("```go\nfunc main() {}\n```\n", resolveNone))
    if !strings.Contains(out, `class="chroma"`) {
        t.Fatalf("expected chroma container, got: %s", out)
    }
    if strings.Contains(out, "style=") {
        t.Fatalf("highlighting must be class-based, found inline style: %s", out)
    }
}
```

Run: `go test ./internal/adapter/webui/ -run TestRenderDocument_CodeHighlight -v`
Expected: FAIL (no chroma container yet).

- [ ] **Step 7: Wire highlighting into the goldmark builder**

`internal/adapter/webui/markdown_chroma.go`:

```go
package webui

import (
    highlighting "github.com/yuin/goldmark-highlighting/v2"
    chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
)

// highlightingExtension returns the goldmark extension that renders fenced code
// with chroma using CSS classes (no inline styles). Theme is chosen by the
// committed chroma.css (data-theme scoping), so we don't pin a style here.
func highlightingExtension() interface{ /* goldmark.Extender */ } {
    return highlighting.NewHighlighting(
        highlighting.WithFormatOptions(
            chromahtml.WithClasses(true),
            chromahtml.ClassPrefix(""),
        ),
    )
}
```

> If the empty-interface return is awkward, type it as `goldmark.Extender` (import `github.com/yuin/goldmark`). Then in `wikilink.go` add it to `WithExtensions`:

```go
goldmark.WithExtensions(
    extension.GFM,
    extension.Footnote,
    highlightingExtension(),
),
```

- [ ] **Step 8: Allow chroma markup in the sanitizer**

In `getDocPolicy()` add:

```go
p.AllowAttrs("class").OnElements("pre", "code", "span")
```

(`style` must remain disallowed — highlighting is class-based.)

- [ ] **Step 9: Run highlighting test, verify it passes**

Run: `go test ./internal/adapter/webui/ -run TestRenderDocument_CodeHighlight -v`
Expected: PASS.

- [ ] **Step 10: Link chroma.css in the page head**

In `internal/adapter/webui/components/base.templ`, in `<head>` right after the `app.css` link, add:

```html
<link rel="stylesheet" href="/static/chroma.css"/>
```

Run: `make generate` (regenerate base_templ.go).

- [ ] **Step 11: Full package tests + lint**

Run: `go test ./internal/adapter/webui/... && go vet ./internal/adapter/webui/...`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/adapter/webui/markdown_chroma.go internal/adapter/webui/chromacss.go \
        internal/adapter/webui/chromacss_test.go internal/adapter/webui/gen/chromacss/main.go \
        internal/adapter/webui/static/chroma.css internal/adapter/webui/wikilink.go \
        internal/adapter/webui/markdown_test.go internal/adapter/webui/components/base.templ \
        internal/adapter/webui/components/base_templ.go go.mod go.sum
git commit -m "feat(webui): chroma syntax-highlighting with light/dark theme-scoped stylesheet"
```

---

## Task 4: Doc-kind style helper + MarkdownProse component

**Files:**
- Create: `internal/adapter/webui/dockindstyle.go`
- Create: `internal/adapter/webui/dockindstyle_test.go`
- Create: `internal/adapter/webui/components/markdownprose.templ`
- Modify: `web/tailwind.css` (prose + callout token layer)

**Interfaces:**
- Consumes: `domain.DocumentType`.
- Produces:
  - `func DocKindStyle(t domain.DocumentType) DocKind` returning `DocKind{Label, Glyph, Tone string}` where `Tone` is a semantic CSS class fragment (`accent|success|highlight|warning`).
  - `templ MarkdownProse(html template.HTML)` — wraps rendered markdown in `<div class="prose">…</div>`.

- [ ] **Step 1: Write failing test for DocKindStyle**

`internal/adapter/webui/dockindstyle_test.go`:

```go
package webui

import (
    "testing"

    "github.com/serverkraken/flow/internal/domain"
)

func TestDocKindStyle(t *testing.T) {
    cases := map[domain.DocumentType]struct{ label, tone string }{
        domain.DocDaily:       {"Täglich", "accent"},
        domain.DocProject:     {"Projekt", "success"},
        domain.DocFree:        {"Frei", "highlight"},
        domain.DocAgent:       {"Agent", "warning"},
        domain.DocMemory:      {"Memory", "warning"},
        domain.DocInstruction: {"Instruction", "warning"},
        domain.DocSkill:       {"Skill", "warning"},
        domain.DocPlan:        {"Plan", "warning"},
    }
    for typ, want := range cases {
        got := DocKindStyle(typ)
        if got.Label != want.label || got.Tone != want.tone {
            t.Errorf("%s: got {%s,%s} want {%s,%s}", typ, got.Label, got.Tone, want.label, want.tone)
        }
        if got.Glyph == "" {
            t.Errorf("%s: empty glyph", typ)
        }
    }
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/adapter/webui/ -run TestDocKindStyle -v`
Expected: FAIL (DocKindStyle undefined).

- [ ] **Step 3: Implement DocKindStyle**

`internal/adapter/webui/dockindstyle.go`:

```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// DocKind is the web presentation of a document type: a German label, a
// meaning-bearing glyph, and a semantic tone class fragment used by Tailwind
// utility classes (text-<tone>, bg-<tone>/10, etc.).
type DocKind struct {
    Label string
    Glyph string
    Tone  string // accent | success | highlight | warning
}

// DocKindStyle maps a DocumentType to its web styling. The four agent/system
// kinds share the "warning" tone and are grouped in their own list section.
func DocKindStyle(t domain.DocumentType) DocKind {
    switch t {
    case domain.DocDaily:
        return DocKind{Label: "Täglich", Glyph: "●", Tone: "accent"}
    case domain.DocProject:
        return DocKind{Label: "Projekt", Glyph: "◆", Tone: "success"}
    case domain.DocFree:
        return DocKind{Label: "Frei", Glyph: "○", Tone: "highlight"}
    case domain.DocAgent:
        return DocKind{Label: "Agent", Glyph: "▪", Tone: "warning"}
    case domain.DocMemory:
        return DocKind{Label: "Memory", Glyph: "▪", Tone: "warning"}
    case domain.DocInstruction:
        return DocKind{Label: "Instruction", Glyph: "▪", Tone: "warning"}
    case domain.DocSkill:
        return DocKind{Label: "Skill", Glyph: "▪", Tone: "warning"}
    case domain.DocPlan:
        return DocKind{Label: "Plan", Glyph: "▪", Tone: "warning"}
    default:
        return DocKind{Label: string(t), Glyph: "▪", Tone: "warning"}
    }
}

// IsSystemKind reports whether a type belongs to the agent/system group
// (rendered in its own denser section, separate from human notes).
func IsSystemKind(t domain.DocumentType) bool {
    switch t {
    case domain.DocAgent, domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocPlan:
        return true
    }
    return false
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `go test ./internal/adapter/webui/ -run TestDocKindStyle -v`
Expected: PASS.

- [ ] **Step 5: Create the MarkdownProse component**

`internal/adapter/webui/components/markdownprose.templ`:

```templ
package components

import "html/template"

// MarkdownProse wraps pre-rendered, sanitized markdown HTML in the prose
// container. The html argument MUST already be sanitized (webui.RenderDocument).
templ MarkdownProse(html template.HTML) {
    <div class="prose max-w-[70ch]">
        @templ.Raw(html)
    </div>
}
```

Run: `make generate`

- [ ] **Step 6: Add the prose + callout token layer to Tailwind**

In `web/tailwind.css`, append a layer (after existing component layers):

```css
@layer components {
  .prose { @apply text-body leading-relaxed; }
  .prose h1 { @apply font-display text-2xl mt-8 mb-3 text-ink; }
  .prose h2 { @apply font-display text-xl mt-6 mb-2 text-ink; }
  .prose h3 { @apply font-semibold text-lg mt-5 mb-2 text-ink; }
  .prose p { @apply my-3; }
  .prose ul { @apply list-disc pl-6 my-3; }
  .prose ol { @apply list-decimal pl-6 my-3; }
  .prose a { @apply text-accent underline underline-offset-2; }
  .prose code { @apply font-mono text-sm px-1 py-0.5 rounded bg-sunken; }
  .prose pre.chroma { @apply font-mono text-sm p-4 my-4 rounded-lg overflow-x-auto bg-[#0d1117]; }
  .prose pre.chroma code { @apply bg-transparent p-0; }
  .prose table { @apply my-4 border-collapse w-full; }
  .prose th, .prose td { @apply border border-line px-3 py-1.5 text-left; }
  .prose .wikilink { @apply text-accent; }
  .prose .wikilink-broken { @apply text-danger line-through; }

  .callout { @apply my-4 rounded-lg border-l-4 pl-4 pr-3 py-2; }
  .callout-title { @apply font-semibold mb-1 flex items-center gap-2; }
  .callout-note { @apply border-accent bg-accent/5; }
  .callout-tip { @apply border-success bg-success/5; }
  .callout-warning { @apply border-warning bg-warning/5; }
  .callout-important { @apply border-highlight bg-highlight/5; }
  .callout-danger { @apply border-danger bg-danger/5; }
}
```

> Use the actual semantic color names defined in the project's `@theme` block
> (foundation/accent/etc. per spec §3.1). If a utility like `bg-accent/5`
> doesn't compile, check the token name in `web/tailwind.css`'s `@theme`/`:root`
> and adjust. The code-block background `#0d1117` matches the dark chroma style;
> keep it constant in both themes (spec §4.2 keeps code dark — wait, spec chose
> two stylesheets: keep the `pre.chroma` background theme-scoped instead — set it
> via chroma.css `.chroma { background: … }` rules, which `WriteCSS` already
> emits, and drop the `bg-[#0d1117]` utility here). Verify visually in Task 9.

- [ ] **Step 7: Rebuild CSS + run tests**

Run: `make web && go test ./internal/adapter/webui/...`
Expected: PASS; `app.css` updated.

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/webui/dockindstyle.go internal/adapter/webui/dockindstyle_test.go \
        internal/adapter/webui/components/markdownprose.templ internal/adapter/webui/components/markdownprose_templ.go \
        web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "feat(webui): doc-kind style helper + MarkdownProse component + prose/callout CSS"
```

---

## Task 5: Documents pagination backend (port + pgstore + usecase)

**Files:**
- Modify: `internal/ports/ports.go` (add `ListPage` to `DocumentStore`)
- Modify: `internal/adapter/pgstore/documents.go` (implement `ListPage`)
- Create: `internal/usecase/list_documents_page.go`
- Modify: every in-memory/fake `DocumentStore` used in tests (search the repo for types implementing `DocumentStore`) to satisfy the new method.
- Test: `internal/usecase/list_documents_page_test.go` and a pgstore test if the package has Postgres-backed tests.

**Interfaces:**
- Consumes: `domain.Document`.
- Produces:
  - Port: `ListPage(ctx context.Context, ownerID string, projectID *string, limit, offset int, tags ...string) ([]domain.Document, int, error)` — returns the page plus the total count.
  - Usecase: `ListDocumentsPage.Execute(ctx, ownerID string, projectID *string, tags []string, limit, offset int) ([]domain.Document, int, error)`.

- [ ] **Step 1: Add the port method**

In `internal/ports/ports.go`, inside `type DocumentStore interface { … }`, add:

```go
// ListPage returns one page of documents (ordered updated_at DESC) plus the
// total count matching the filter, for server-side pagination.
ListPage(ctx context.Context, ownerID string, projectID *string, limit, offset int, tags ...string) ([]domain.Document, int, error)
```

- [ ] **Step 2: Write a failing usecase test (with a fake store)**

`internal/usecase/list_documents_page_test.go` — mirror the fake-store pattern already used in `list_documents_test.go` (open that file first to copy the fake's shape). Test body:

```go
func TestListDocumentsPage(t *testing.T) {
    store := &fakeDocStore{ // reuse/extend the existing fake in this package
        page:  []domain.Document{{ID: "a"}, {ID: "b"}},
        total: 5,
    }
    uc := NewListDocumentsPage(store)
    docs, total, err := uc.Execute(context.Background(), "owner", nil, nil, 2, 0)
    if err != nil { t.Fatal(err) }
    if total != 5 { t.Fatalf("total=%d want 5", total) }
    if len(docs) != 2 { t.Fatalf("len=%d want 2", len(docs)) }
}
```

> Extend the package's existing fake `DocumentStore` with a `ListPage` method
> returning `f.page, f.total, nil`. If no shared fake exists, define a minimal
> one in this test file implementing the full `ports.DocumentStore` interface
> (stub the other methods with `panic("unused")`).

- [ ] **Step 3: Run test, verify it fails**

Run: `go test ./internal/usecase/ -run TestListDocumentsPage -v`
Expected: FAIL (NewListDocumentsPage undefined).

- [ ] **Step 4: Implement the usecase**

`internal/usecase/list_documents_page.go`:

```go
package usecase

import (
    "context"

    "github.com/serverkraken/flow/internal/domain"
    "github.com/serverkraken/flow/internal/ports"
)

// ListDocumentsPage returns one page of a user's documents plus the total.
type ListDocumentsPage struct{ store ports.DocumentStore }

func NewListDocumentsPage(store ports.DocumentStore) *ListDocumentsPage {
    return &ListDocumentsPage{store: store}
}

func (uc *ListDocumentsPage) Execute(ctx context.Context, ownerID string, projectID *string, tags []string, limit, offset int) ([]domain.Document, int, error) {
    if limit <= 0 {
        limit = 50
    }
    if offset < 0 {
        offset = 0
    }
    return uc.store.ListPage(ctx, ownerID, projectID, limit, offset, tags...)
}
```

- [ ] **Step 5: Implement ListPage in pgstore**

In `internal/adapter/pgstore/documents.go`, add (mirror the existing `List` at :69 for filter assembly; `docCols` already exists):

```go
func (s *DocumentStore) ListPage(ctx context.Context, ownerID string, projectID *string, limit, offset int, tags ...string) ([]domain.Document, int, error) {
    where := ` WHERE owner_id=$1`
    args := []any{ownerID}
    where = appendProjectFilter(where, "project_id", &args, projectID)
    if len(tags) > 0 {
        args = append(args, tags)
        where += fmt.Sprintf(` AND tags @> $%d`, len(args))
    }

    // total count
    var total int
    if err := s.db.QueryRow(ctx, `SELECT count(*) FROM documents`+where, args...).Scan(&total); err != nil {
        return nil, 0, err
    }

    // page
    args = append(args, limit, offset)
    q := `SELECT ` + docCols + ` FROM documents` + where +
        fmt.Sprintf(` ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
    rows, err := s.db.Query(ctx, q, args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()
    docs, err := scanDocuments(rows) // reuse the existing scan helper used by List
    if err != nil {
        return nil, 0, err
    }
    return docs, total, nil
}
```

> Open `documents.go` and match the real names: the DB handle field (`s.db`),
> the row-scan helper used by `List` (it may be inline — if so, factor the scan
> loop into `scanDocuments(rows)` and call it from both `List` and `ListPage` to
> stay DRY), and `appendProjectFilter`. Adjust if the helpers differ.

- [ ] **Step 6: Satisfy the new interface method in all fakes**

Run a repo-wide search for other `DocumentStore` implementations:
```bash
grep -rl "func.*List(ctx context.Context, ownerID string, projectID \*string, tags ...string)" internal | sort -u
```
For each fake/mock/in-memory store (commonly under `internal/adapter/memstore` or test files), add a `ListPage` method. For an in-memory store, implement real slicing:

```go
func (m *MemDocumentStore) ListPage(ctx context.Context, ownerID string, projectID *string, limit, offset int, tags ...string) ([]domain.Document, int, error) {
    all, err := m.List(ctx, ownerID, projectID, tags...)
    if err != nil { return nil, 0, err }
    total := len(all)
    if offset > total { offset = total }
    end := offset + limit
    if end > total { end = total }
    return all[offset:end], total, nil
}
```

- [ ] **Step 7: Run usecase test + build, verify pass**

Run: `go test ./internal/usecase/ -run TestListDocumentsPage -v && go build ./...`
Expected: PASS + clean build (all DocumentStore implementers satisfy `ListPage`).

- [ ] **Step 8: Commit**

```bash
git add internal/ports/ports.go internal/adapter/pgstore/documents.go \
        internal/usecase/list_documents_page.go internal/usecase/list_documents_page_test.go \
        internal/adapter/memstore # adjust to the real fake path(s)
git commit -m "feat(pgstore): paginated DocumentStore.ListPage + ListDocumentsPage usecase"
```

---

## Task 6: Wissen list page (`/wissen`) — category-specific, search, tag-filter, pagination, SSE

**Files:**
- Create: `internal/adapter/webui/wissen.templ` (`WissenPage` + `WissenFragment` + section sub-templates)
- Create: `internal/adapter/webui/wissen_vm.go` (`WissenVM` + builder helpers)
- Create: `internal/adapter/webui/components/categorystrip.templ`
- Create: `internal/adapter/httpserver/webui_wissen.go` (handlers)
- Create: `internal/adapter/webui/wissen_vm_test.go`, `internal/adapter/httpserver/webui_wissen_test.go`
- Create: `internal/adapter/webui/static/scrollspy.js` (vendored, IntersectionObserver) + link in base
- Modify: `internal/i18n/catalog_de.go`, `catalog_en.go` (new keys)
- Inject: `ListDocumentsPage` usecase onto `*httpserver.Server` (wired in Task 9)

**Interfaces:**
- Consumes: `ListDocumentsPage`, `SearchDocuments`, `ListTags`, `DocKindStyle`, `IsSystemKind`, `ColorHex`, `components.Pagination`, `components.T`.
- Produces:
  - `WissenVM` (see Step 1).
  - Handlers `handleWebWissenHome(w,r)` (full page) and `handleWebWissenList(w,r)` (fragment).
  - Routes (registered in Task 9): `GET /wissen`, `GET /ui/wissen/list`.

> **Pattern to mirror:** `internal/adapter/webui/frei.templ` (Page→Base→AppShell→Fragment) and `internal/adapter/httpserver/webui_docs.go` (`docsListData`, search vs list branch, `toggleTagHref`, `renderSnippet`). Reuse `renderSnippet`/`encodeListQuery`/`toggleTagHref`/`singleTagHref` verbatim — move them from `webui_docs.go` into `webui_wissen.go` (they're deleted with the old file in Task 9), updating their base path from `/docs` to `/wissen`.

- [ ] **Step 1: Define the viewmodel**

`internal/adapter/webui/wissen_vm.go`:

```go
package webui

import "github.com/serverkraken/flow/internal/domain"

type DocRow struct {
    ID, Type, Path, Title string
    Tags                  []string
    ProjectID             string
    ProjectColor          string // hex via ColorHex, "" if none
}

type SearchRow struct {
    DocRow
    Snippet string // pre-rendered <mark> HTML (renderSnippet)
}

type TagChip struct {
    Tag    string
    Count  int
    Active bool
    Href   string
}

// ProjectGroup groups project-notes under one project header.
type ProjectGroup struct {
    ProjectID, Name, Color, Glyph string
    Docs                          []DocRow
}

type WissenVM struct {
    User       string
    AllTags    []TagChip
    ActiveTags []string
    SearchQ    string
    Query      string // "?tag=…&q=…" preserved for the SSE fragment hx-get

    // Category sections (empty when searching):
    Daily   []DocRow       // DocDaily, chronological
    Notes   []ProjectGroup // DocProject grouped by project
    Free    []DocRow       // DocFree
    System  []DocRow       // agent/memory/instruction/skill/plan

    // Search mode:
    Results []SearchRow

    Page components.PageNav // pagination for the active list
}
```

> If `components.PageNav` causes an import cycle (webui → components is fine;
> components → webui is not), keep `PageNav` usage only inside templ where
> `components` is already imported, and store the raw `Page, Total, PageSize int`
> on the VM instead. Prefer storing `Page int`, `Total int`, `PageSize int` on
> `WissenVM` and constructing `components.PageNav{...}` in the templ.

- [ ] **Step 2: Write a failing VM-builder test**

`internal/adapter/webui/wissen_vm_test.go`:

```go
package webui

import (
    "testing"

    "github.com/serverkraken/flow/internal/domain"
)

func TestGroupDocsByCategory(t *testing.T) {
    docs := []domain.Document{
        {ID: "1", Type: domain.DocDaily, Title: "Mon"},
        {ID: "2", Type: domain.DocProject, Title: "Note", ProjectID: strptr("p1")},
        {ID: "3", Type: domain.DocFree, Title: "Urlaub"},
        {ID: "4", Type: domain.DocMemory, Title: "Mem"},
    }
    names := map[string]string{"p1": "Alpha"}
    colors := map[string]string{"p1": "blue"}
    vm := groupDocsByCategory(docs, names, colors)
    if len(vm.Daily) != 1 || len(vm.Free) != 1 || len(vm.System) != 1 {
        t.Fatalf("category split wrong: %+v", vm)
    }
    if len(vm.Notes) != 1 || vm.Notes[0].Name != "Alpha" || len(vm.Notes[0].Docs) != 1 {
        t.Fatalf("project grouping wrong: %+v", vm.Notes)
    }
}

func strptr(s string) *string { return &s }
```

- [ ] **Step 3: Run test, verify it fails**

Run: `go test ./internal/adapter/webui/ -run TestGroupDocsByCategory -v`
Expected: FAIL (groupDocsByCategory undefined).

- [ ] **Step 4: Implement the grouping helper**

Add to `wissen_vm.go`:

```go
// groupDocsByCategory splits docs into the four list sections. projectNames /
// projectColors map project IDs to display name + color token name.
func groupDocsByCategory(docs []domain.Document, projectNames, projectColors map[string]string) WissenVM {
    var vm WissenVM
    groups := map[string]*ProjectGroup{}
    for _, d := range docs {
        row := DocRow{ID: d.ID, Type: string(d.Type), Path: d.Path, Title: d.Title, Tags: d.Tags}
        if d.ProjectID != nil {
            row.ProjectID = *d.ProjectID
            row.ProjectColor = ColorHex(projectColors[*d.ProjectID])
        }
        switch {
        case d.Type == domain.DocDaily:
            vm.Daily = append(vm.Daily, row)
        case d.Type == domain.DocFree:
            vm.Free = append(vm.Free, row)
        case d.Type == domain.DocProject:
            pid := ""
            if d.ProjectID != nil { pid = *d.ProjectID }
            g := groups[pid]
            if g == nil {
                ks := DocKindStyle(domain.DocProject)
                g = &ProjectGroup{ProjectID: pid, Name: projectNames[pid], Color: ColorHex(projectColors[pid]), Glyph: ks.Glyph}
                groups[pid] = g
                vm.Notes = append(vm.Notes, ProjectGroup{}) // placeholder, fixed below
            }
            g.Docs = append(g.Docs, row)
        default: // agent/memory/instruction/skill/plan
            vm.System = append(vm.System, row)
        }
    }
    // Rebuild Notes from the map in stable insertion order.
    vm.Notes = vm.Notes[:0]
    seen := map[string]bool{}
    for _, d := range docs {
        if d.Type != domain.DocProject { continue }
        pid := ""
        if d.ProjectID != nil { pid = *d.ProjectID }
        if seen[pid] { continue }
        seen[pid] = true
        vm.Notes = append(vm.Notes, *groups[pid])
    }
    return vm
}
```

- [ ] **Step 5: Run test, verify it passes**

Run: `go test ./internal/adapter/webui/ -run TestGroupDocsByCategory -v`
Expected: PASS.

- [ ] **Step 6: Write the category strip + page templates**

`internal/adapter/webui/components/categorystrip.templ`:

```templ
package components

import "context"

type CatTab struct{ ID, LabelKey string; Count int }

templ CategoryStrip(ctx context.Context, tabs []CatTab) {
    <nav class="flex gap-2 overflow-x-auto border-b border-line mb-4" aria-label={ T(ctx, "wissen.categories") }>
        for _, tab := range tabs {
            <a href={ templ.SafeURL("#" + tab.ID) } class="catstrip-link whitespace-nowrap px-3 py-2 text-muted hover:text-ink" data-target={ tab.ID }>
                { T(ctx, tab.LabelKey) }
                <span class="text-faint">({ itoa(tab.Count) })</span>
            </a>
        }
    </nav>
}
```

> `itoa` helper: add a tiny `func itoa(n int) string { return strconv.Itoa(n) }`
> in a `components/format.go` if none exists, or use an existing one.

`internal/adapter/webui/wissen.templ` — structure (mirror `frei.templ` for the Base/AppShell wrapper; fill section rendering):

```templ
package webui

import (
    "github.com/serverkraken/flow/internal/adapter/webui/components"
)

templ WissenPage(vm WissenVM) {
    @components.Base("docs", wissenBody(vm))
}

templ wissenBody(vm WissenVM) {
    @components.AppShell("docs", nil, nil, wissenOuter(vm))
}

templ wissenOuter(vm WissenVM) {
    <div id="content"
         hx-get={ "/ui/wissen/list" + vm.Query }
         hx-trigger="sse:document.created, sse:document.updated, sse:document.deleted"
         hx-swap="innerHTML">
        @WissenFragment(vm)
    </div>
}

templ WissenFragment(vm WissenVM) {
    <header class="flex items-center justify-between mb-4">
        <h1 class="font-display text-2xl">{ components.T(ctx, "wissen.title") }</h1>
        <a href="/wissen/neu" class="btn-primary">{ components.T(ctx, "common.new") }</a>
    </header>
    @wissenSearchBar(vm)
    @wissenTagChips(vm)
    if vm.SearchQ != "" {
        @wissenResults(vm)
    } else {
        @components.CategoryStrip(ctx, wissenTabs(vm))
        @wissenDailySection(vm)
        @wissenNotesSection(vm)
        @wissenFreeSection(vm)
        @wissenSystemSection(vm)
        @components.Pagination(components.PageNav{Page: vm.Page.Page, Total: vm.Page.Total, PageSize: vm.Page.PageSize, BaseHref: "/wissen"})
    }
}
```

> Implement each `wissen*Section` sub-template rendering its `DocRow`s as
> accessible list items: kind glyph (`DocKindStyle`), title link to
> `/wissen/{ID}`, tag chips. The project-notes section iterates `vm.Notes`,
> emitting a project-colored header (`style="--swatch: <Color>"`) then the docs.
> The system section gets `class="docs-system"` (denser, separated). Keep each
> section in its own templ block. Empty state: when all sections are empty and
> not searching, render `components.EmptyState` with key `wissen.empty`.

- [ ] **Step 7: Write the handlers**

`internal/adapter/httpserver/webui_wissen.go` — move `renderSnippet`, `stripStraySentinels`, `encodeListQuery`, `toggleTagHref`, `singleTagHref`, `encodeTagQuery` here from `webui_docs.go` (change `/docs` → `/wissen`), then:

```go
func (s *Server) wissenData(r *http.Request, u domain.User) (webui.WissenVM, error) {
    active := r.URL.Query()["tag"]
    q := strings.TrimSpace(r.URL.Query().Get("q"))
    page := atoiDefault(r.URL.Query().Get("page"), 1)
    const pageSize = 50

    allTags, err := s.ListTags.Execute(r.Context(), u.ID)
    if err != nil { return webui.WissenVM{}, err }
    // build tag chips (copy from docsListData)
    // ...
    vm := webui.WissenVM{User: u.Username, ActiveTags: active, SearchQ: q, Query: encodeListQuery(active, q)}
    // tag chips assignment omitted for brevity — copy from docsListData

    if q != "" {
        hits, err := s.SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)
        if err != nil { return webui.WissenVM{}, err }
        for _, h := range hits {
            vm.Results = append(vm.Results, webui.SearchRow{
                DocRow:  webui.DocRow{ID: h.ID, Type: string(h.Type), Path: h.Path, Title: h.Title, Tags: h.Tags},
                Snippet: renderSnippet(h.Snippet),
            })
        }
        return vm, nil
    }

    docs, total, err := s.ListDocumentsPage.Execute(r.Context(), u.ID, nil, active, pageSize, (page-1)*pageSize)
    if err != nil { return webui.WissenVM{}, err }
    names, colors := s.projectNameColorMaps(r.Context(), u.ID) // helper: id→name, id→colortoken
    cat := webui.GroupDocsByCategoryExported(docs, names, colors) // export the helper or build inline
    cat.User, cat.ActiveTags, cat.SearchQ, cat.Query, cat.AllTags = vm.User, active, q, vm.Query, vm.AllTags
    cat.Page = components.PageNav{Page: page, Total: total, PageSize: pageSize}
    return cat, nil
}

func (s *Server) handleWebWissenHome(w http.ResponseWriter, r *http.Request) {
    u, _ := userFrom(r.Context())
    vm, err := s.wissenData(r, u)
    if err != nil { http.Error(w, "server error", http.StatusInternalServerError); return }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = webui.WissenPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebWissenList(w http.ResponseWriter, r *http.Request) {
    u, _ := userFrom(r.Context())
    vm, err := s.wissenData(r, u)
    if err != nil { http.Error(w, "server error", http.StatusInternalServerError); return }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = webui.WissenFragment(vm).Render(r.Context(), w)
}
```

> Add `atoiDefault(s string, def int) int` if not present. Export
> `groupDocsByCategory` as `GroupDocsByCategory` (capital) so the handler can
> call it, OR keep it unexported and add a thin exported builder. Implement
> `projectNameColorMaps` using the existing project list usecase (find how
> `webui_projects.go` lists projects) returning `map[id]name`, `map[id]colorToken`.

- [ ] **Step 8: Write a handler test**

`internal/adapter/httpserver/webui_wissen_test.go` — mirror `webui_docs_test.go`: build a `Server` with fakes, seed a few documents of different kinds, `GET /wissen` via `httptest`, assert 200 + that the response contains the daily/free/system section markers and a known title. Also test `q=` returns search results markup.

- [ ] **Step 9: Add i18n keys**

In `internal/i18n/catalog_de.go` add to the DE map:
```go
"wissen.title":      "Wissen",
"wissen.categories": "Kategorien",
"wissen.daily":      "Täglich",
"wissen.notes":      "Projekt-Notizen",
"wissen.free":       "Frei",
"wissen.system":     "Agent & System",
"wissen.empty":      "Kompendium leer — Neu anlegen",
"wissen.search":     "Suchen (Volltext + semantisch)",
"wissen.noresults":  "Keine Treffer",
```
Add the same keys to `catalog_en.go` (English values; stub is fine).

- [ ] **Step 10: Add the scroll-spy JS + link it**

`internal/adapter/webui/static/scrollspy.js` (vanilla, CSP-safe):
```js
// Highlights the active category-strip link as sections scroll into view.
(function () {
  function init() {
    var links = document.querySelectorAll('.catstrip-link');
    if (!links.length) return;
    var byId = {};
    links.forEach(function (l) { byId[l.dataset.target] = l; });
    var obs = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        var l = byId[e.target.id];
        if (l && e.isIntersecting) {
          links.forEach(function (x) { x.classList.remove('text-ink'); });
          l.classList.add('text-ink');
        }
      });
    }, { rootMargin: '-40% 0px -55% 0px' });
    ['daily-sec', 'notes-sec', 'free-sec', 'system-sec'].forEach(function (id) {
      var el = document.getElementById(id);
      if (el) obs.observe(el);
    });
  }
  document.addEventListener('DOMContentLoaded', init);
  document.body.addEventListener('htmx:afterSwap', init);
})();
```
Give each section wrapper the matching `id` (`daily-sec`, `notes-sec`, `free-sec`, `system-sec`) in `wissen.templ`. Link it in `base.templ` head: `<script src="/static/scrollspy.js" defer></script>` then `make generate`.

- [ ] **Step 11: Generate, build, test**

Run: `make generate && go build ./... && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -run 'Wissen|GroupDocs' -v`
Expected: PASS. (Routes aren't wired yet — handler tests construct requests directly or skip until Task 9; if the test needs the route, register it locally in the test's mux.)

- [ ] **Step 12: Commit**

```bash
make web
git add internal/adapter/webui/wissen.templ internal/adapter/webui/wissen_templ.go \
        internal/adapter/webui/wissen_vm.go internal/adapter/webui/wissen_vm_test.go \
        internal/adapter/webui/components/categorystrip.templ internal/adapter/webui/components/categorystrip_templ.go \
        internal/adapter/httpserver/webui_wissen.go internal/adapter/httpserver/webui_wissen_test.go \
        internal/adapter/webui/static/scrollspy.js internal/adapter/webui/components/base.templ internal/adapter/webui/components/base_templ.go \
        internal/i18n/catalog_de.go internal/i18n/catalog_en.go internal/adapter/webui/static/app.css
git commit -m "feat(webui): Wissen list page — category-specific sections + search + tags + pagination + SSE"
```

---

## Task 7: Document read page (`/wissen/{id}`) — header, prose, ToC, backlinks, embed

**Files:**
- Create: `internal/adapter/webui/document.templ` (`DocumentPage` + `DocumentFragment`)
- Create: `internal/adapter/webui/document_vm.go` (`DocumentVM`)
- Create: `internal/adapter/webui/components/toc.templ`, `internal/adapter/webui/components/backlinks.templ`
- Create: `internal/adapter/httpserver/webui_document.go`
- Create: `internal/adapter/webui/static/toc.js` (build ToC from headings + scroll-spy)
- Create: `internal/adapter/httpserver/webui_document_test.go`
- Modify: catalogs (keys)

**Interfaces:**
- Consumes: `GetDocument`, `BacklinksDocument`, `RenderDocument`, `domain.ResolveWikilink`, `ListDocuments` (for resolver corpus), `DocKindStyle`, `components.MarkdownProse`, embed-status usecase (`GetEmbedStatus`/`RetryEmbedding` — reuse from `webui_docs.go`).
- Produces:
  - `DocumentVM` with rendered HTML, backlinks, tags, embed view.
  - Handler `handleWebDocumentView(w,r)`.
  - Route (Task 9): `GET /wissen/{id}`.

> **Pattern to mirror:** `handleWebDocView` in `webui_docs.go:202` (resolver
> closure, backlinks, embed status). The ONLY behavioral changes: wikilink
> hrefs become `/wissen/{id}` (not `/docs/{id}`); the page wraps `AppShell`;
> prose uses the feature-complete renderer (already swapped in Tasks 1–3).

- [ ] **Step 1: Define the viewmodel**

`internal/adapter/webui/document_vm.go`:

```go
package webui

import "html/template"

type TagLink struct{ Tag, Href string }

type EmbedView struct {
    State     string
    LastError string
    ShowRetry bool
}

type DocumentVM struct {
    User        string
    ID          string
    Type        string
    KindLabel   string
    KindGlyph   string
    KindTone    string
    Title       string
    ProjectID   string
    ProjectName string
    ProjectColor string
    DateStr     string
    Tags        []TagLink
    HTML        template.HTML
    Backlinks   []DocRow
    Embed       *EmbedView
}
```

- [ ] **Step 2: Write a failing handler test**

`internal/adapter/httpserver/webui_document_test.go` — mirror `webui_docs_test.go`'s view test: seed a document whose body contains a GFM table, a callout, and a fenced code block; `GET /wissen/{id}`; assert 200 and that the body contains `<table`, `callout-`, and `class="chroma"`. Also assert the "Bearbeiten" link points to `/wissen/{id}/bearbeiten` and a backlink (seed a second doc linking to the first) appears.

- [ ] **Step 3: Run test, verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run Wissen.*Document -v`
Expected: FAIL (handler/route absent).

- [ ] **Step 4: Implement the handler**

`internal/adapter/httpserver/webui_document.go` — copy `handleWebDocView` logic, adapting:

```go
func (s *Server) handleWebDocumentView(w http.ResponseWriter, r *http.Request) {
    u, _ := userFrom(r.Context())
    id := r.PathValue("id")
    doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
    if errors.Is(err, ports.ErrDocumentNotFound) { http.Error(w, "not found", http.StatusNotFound); return }
    if err != nil { http.Error(w, "server error", http.StatusInternalServerError); return }

    all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
    if err != nil { http.Error(w, "server error", http.StatusInternalServerError); return }
    resolve := func(target string) (string, string, bool) {
        if t, ok := domain.ResolveWikilink(doc, target, all); ok {
            return "/wissen/" + t.ID, t.Title, true
        }
        return "", "", false
    }
    rendered := webui.RenderDocument(doc.Body, resolve)

    refs, err := s.BacklinksDocument.Execute(r.Context(), u.ID, id)
    if err != nil { http.Error(w, "server error", http.StatusInternalServerError); return }

    ks := webui.DocKindStyle(doc.Type)
    vm := webui.DocumentVM{
        User: u.Username, ID: doc.ID, Type: string(doc.Type),
        KindLabel: ks.Label, KindGlyph: ks.Glyph, KindTone: ks.Tone,
        Title: doc.Title, HTML: rendered,
    }
    if doc.Date != nil { vm.DateStr = doc.Date.Format("02.01.2006") }
    for _, t := range doc.Tags {
        vm.Tags = append(vm.Tags, webui.TagLink{Tag: t, Href: singleTagHref(t)})
    }
    for _, ref := range refs {
        vm.Backlinks = append(vm.Backlinks, webui.DocRow{ID: ref.ID, Type: string(ref.Type), Path: ref.Path, Title: ref.Title})
    }
    if st, serr := s.GetEmbedStatus.Execute(r.Context(), u.ID, id); serr == nil {
        vm.Embed = &webui.EmbedView{State: string(st.State), LastError: truncateError(st.LastError), ShowRetry: st.State == domain.EmbedFailed}
    }
    // project name/color if doc.ProjectID != nil — reuse projectNameColorMaps
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = webui.DocumentPage(vm).Render(r.Context(), w)
}
```

> `truncateError` and `singleTagHref` move into the wissen/document handler
> files (deleted from `webui_docs.go` in Task 9). Keep one copy.

- [ ] **Step 5: Implement the templ page**

`internal/adapter/webui/document.templ` — wrap `AppShell("docs", breadcrumb, nil, content)`; layout a 2-column reading area (text + aside). Aside holds `@components.Toc()` (built client-side from headings) and `@components.Backlinks(vm.Backlinks)`. Header shows kind badge (`vm.KindGlyph`/`KindLabel`/`KindTone`), title, project link, date, tag chips, a "Bearbeiten" link to `/wissen/{vm.ID}/bearbeiten`, and an overflow menu with a delete `ConfirmDialog`:

```templ
@components.ConfirmDialog(components.ConfirmSpec{
    ID: "del-" + vm.ID,
    TitleKey: "wissen.delete.title", BodyKey: "wissen.delete.body", ConfirmLabelKey: "common.delete",
    ConfirmAttrs: templ.Attributes{
        "hx-post": "/wissen/" + vm.ID + "/delete", "hx-target": "body", "type": "button",
    },
})
```

The reading column: `@components.MarkdownProse(vm.HTML)`. Add SSE live reload on the content container: `hx-trigger="sse:document.updated"` re-getting the page fragment for this id (use a `DocumentFragment`).

`internal/adapter/webui/components/backlinks.templ` and `toc.templ`: simple aside lists; ToC is an empty `<nav id="toc" aria-label="Inhalt">` filled by `toc.js`.

- [ ] **Step 6: Implement toc.js**

`internal/adapter/webui/static/toc.js`:
```js
(function () {
  function build() {
    var toc = document.getElementById('toc');
    var prose = document.querySelector('.prose');
    if (!toc || !prose) return;
    toc.innerHTML = '';
    var hs = prose.querySelectorAll('h1, h2, h3');
    hs.forEach(function (h, i) {
      if (!h.id) h.id = 'h-' + i;
      var a = document.createElement('a');
      a.href = '#' + h.id;
      a.textContent = h.textContent;
      a.className = 'block py-1 text-muted hover:text-ink toc-' + h.tagName.toLowerCase();
      toc.appendChild(a);
    });
  }
  document.addEventListener('DOMContentLoaded', build);
  document.body.addEventListener('htmx:afterSwap', build);
})();
```
Link it in `base.templ` head (`<script src="/static/toc.js" defer></script>`), `make generate`.

- [ ] **Step 7: Add i18n keys**

DE catalog: `"wissen.edit":"Bearbeiten"`, `"wissen.backlinks":"Referenziert von"`, `"wissen.toc":"Inhalt"`, `"wissen.delete.title":"Dokument löschen?"`, `"wissen.delete.body":"Das kann nicht rückgängig gemacht werden."`, `"common.delete":"Löschen"`. Mirror in EN.

- [ ] **Step 8: Generate, build, test (with a local route)**

In the test, register `mux.Handle("GET /wissen/{id}", s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))` (or rely on Task 9 wiring if running the full server). 
Run: `make generate && go build ./... && go test ./internal/adapter/httpserver/ -run Document -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
make web
git add internal/adapter/webui/document.templ internal/adapter/webui/document_templ.go \
        internal/adapter/webui/document_vm.go internal/adapter/webui/components/toc.templ internal/adapter/webui/components/toc_templ.go \
        internal/adapter/webui/components/backlinks.templ internal/adapter/webui/components/backlinks_templ.go \
        internal/adapter/httpserver/webui_document.go internal/adapter/httpserver/webui_document_test.go \
        internal/adapter/webui/static/toc.js internal/adapter/webui/components/base.templ internal/adapter/webui/components/base_templ.go \
        internal/i18n/catalog_de.go internal/i18n/catalog_en.go internal/adapter/webui/static/app.css
git commit -m "feat(webui): Wissen document read page — header + prose + ToC + backlinks + embed + SSE"
```

---

## Task 8: Editor (`/wissen/neu`, `/wissen/{id}/bearbeiten`) + live preview

**Files:**
- Create: `internal/adapter/webui/editor.templ` (`EditorPage` + `previewFragment`)
- Create: `internal/adapter/webui/editor_vm.go`
- Create: `internal/adapter/httpserver/webui_editor.go` (New/Edit/Create/Update/Delete/Preview)
- Create: `internal/adapter/httpserver/webui_editor_test.go`
- Modify: catalogs

**Interfaces:**
- Consumes: `CreateDocument`, `UpdateDocument`, `DeleteDocument`, `GetDocument`, `RenderDocument`, `ListDocuments` (resolver corpus for preview).
- Produces:
  - Handlers `handleWebEditorNew`, `handleWebEditorEdit`, `handleWebEditorCreate`, `handleWebEditorUpdate`, `handleWebEditorDelete`, `handleWebEditorPreview`.
  - Routes (Task 9): `GET /wissen/neu`, `GET /wissen/{id}/bearbeiten`, `POST /wissen`, `POST /wissen/{id}`, `POST /wissen/{id}/delete`, `POST /wissen/preview`.

> **Pattern to mirror:** `handleWebDocNew/Edit/Create/Update/Delete` in
> `webui_docs.go:261-367`. Same usecases + error handling + `Bus.Publish`. New
> bit: the preview endpoint + the htmx live-preview wiring in the template.

- [ ] **Step 1: Write a failing preview-handler test**

`internal/adapter/httpserver/webui_editor_test.go`:

```go
func TestEditorPreview(t *testing.T) {
    s := newTestServer(t) // existing test helper that builds a Server with fakes + a logged-in user
    body := "body=" + url.QueryEscape("# Title\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
    req := httptest.NewRequest("POST", "/wissen/preview", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req = withTestUser(req) // existing helper to attach the auth context
    rec := httptest.NewRecorder()
    s.handleWebEditorPreview(rec, req)
    if rec.Code != 200 { t.Fatalf("code=%d", rec.Code) }
    if !strings.Contains(rec.Body.String(), "<table") {
        t.Fatalf("preview did not render markdown: %s", rec.Body.String())
    }
}
```

> Use whatever the package's existing test scaffolding is (open
> `webui_docs_test.go` to copy `newTestServer`/`withTestUser` equivalents).

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/adapter/httpserver/ -run TestEditorPreview -v`
Expected: FAIL (handler undefined).

- [ ] **Step 3: Implement the preview handler**

`internal/adapter/httpserver/webui_editor.go`:

```go
func (s *Server) handleWebEditorPreview(w http.ResponseWriter, r *http.Request) {
    u, _ := userFrom(r.Context())
    _ = r.ParseForm()
    bodyMD := r.FormValue("body")
    all, _ := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
    resolve := func(target string) (string, string, bool) {
        // resolve against the corpus without a "current" doc context
        for _, d := range all {
            if strings.EqualFold(d.Path, target) || strings.EqualFold(d.Title, target) {
                return "/wissen/" + d.ID, d.Title, true
            }
        }
        return "", "", false
    }
    rendered := webui.RenderDocument(bodyMD, resolve)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = webui.MarkdownPreview(rendered).Render(r.Context(), w)
}
```

> If `ResolveWikilink` needs a "current" doc, pass a zero `domain.Document{}`
> with `OwnerID: u.ID` instead of the manual loop, to keep scope rules identical
> to the read page: `domain.ResolveWikilink(domain.Document{OwnerID: u.ID}, target, all)`.
> Pick whichever matches `ResolveWikilink`'s real contract (open
> `internal/domain` wikilink resolver to confirm). Prefer reusing `ResolveWikilink`.

Add `webui.MarkdownPreview`:
`internal/adapter/webui/editor.templ`:
```templ
package webui

import (
    "html/template"
    "github.com/serverkraken/flow/internal/adapter/webui/components"
)

templ MarkdownPreview(html template.HTML) {
    @components.MarkdownProse(html)
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `make generate && go test ./internal/adapter/httpserver/ -run TestEditorPreview -v`
Expected: PASS.

- [ ] **Step 5: Implement the editor page with live preview**

`internal/adapter/webui/editor_vm.go`:
```go
package webui

type EditorVM struct {
    User      string
    ID        string // empty for new
    Type      string
    ProjectID string
    Path      string
    Title     string
    Body      string
    Err       string
}
```

`internal/adapter/webui/editor.templ` (add to the file from Step 3) — `EditorPage(vm)` wraps `AppShell("docs", …)`; a two-pane layout: left = form, right = `<div id="preview">`. The textarea:

```templ
<textarea name="body" class="font-mono w-full h-[60vh]"
    hx-post="/wissen/preview"
    hx-trigger="keyup changed delay:400ms, load"
    hx-target="#preview" hx-swap="innerHTML">{ vm.Body }</textarea>
```

Form posts to `/wissen` (new) or `/wissen/{id}` (edit); kind/type select; project select; title input. Validation errors render inline (`vm.Err`), never via native bubbles. Save → handler redirects to `/wissen/{id}` (303).

- [ ] **Step 6: Implement New/Edit/Create/Update/Delete handlers**

Copy from `webui_docs.go:261-367` into `webui_editor.go`, renaming to the `Editor` handlers and changing redirect targets `/docs/...` → `/wissen/...`. Keep `Bus.Publish` of `document.created/updated/deleted`. Tags come from frontmatter in the body (single source) — do not add a separate tag field; `CreateDocument`/`UpdateDocument` already parse frontmatter.

- [ ] **Step 7: Add i18n keys + test create/update**

DE keys: `"wissen.new":"Neues Dokument"`, `"wissen.edit.title":"Dokument bearbeiten"`, `"wissen.body":"Inhalt"`, `"wissen.preview":"Vorschau"`, `"common.save":"Speichern"`, `"wissen.title.field":"Titel"`. Mirror EN.
Add a handler test that `POST /wissen` with valid form fields returns 303 to `/wissen/{id}` and publishes `document.created` (assert via the fake bus).

- [ ] **Step 8: Generate, build, full package test**

Run: `make generate && go build ./... && go test ./internal/adapter/httpserver/... ./internal/adapter/webui/... -run 'Editor|Preview' -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
make web
git add internal/adapter/webui/editor.templ internal/adapter/webui/editor_templ.go internal/adapter/webui/editor_vm.go \
        internal/adapter/httpserver/webui_editor.go internal/adapter/httpserver/webui_editor_test.go \
        internal/i18n/catalog_de.go internal/i18n/catalog_en.go internal/adapter/webui/static/app.css
git commit -m "feat(webui): Wissen editor with htmx live markdown preview"
```

---

## Task 9: Main wiring + remove old /docs + done-gate

**Files:**
- Modify: `internal/adapter/httpserver/server.go` (register `/wissen*` routes; inject `ListDocumentsPage`)
- Modify: `cmd/flow/*` (construct `ListDocumentsPage` and pass to `Server` — find the composition root where usecases are built)
- Modify: `internal/adapter/webui/components/sitenav.templ` (nav entry → `/wissen`, label "Wissen")
- Delete: `internal/adapter/httpserver/webui_docs.go`, `internal/adapter/webui/docs.templ` (+ `docs_templ.go`), and their now-duplicated helpers/tests (port any still-referenced helper before deleting)
- Modify: `internal/adapter/webui/wikilink.go`/tests if any `/docs` path literals remain

**Interfaces:**
- Consumes: all handlers from Tasks 6–8.
- Produces: a fully wired `/wissen` surface; `/docs` removed.

- [ ] **Step 1: Inject the ListDocumentsPage usecase**

Find where `*httpserver.Server` is constructed and where usecases are assigned (grep for `ListDocuments:` or `s.ListDocuments`). Add a `ListDocumentsPage *usecase.ListDocumentsPage` field to `Server`, construct it in the composition root with the same `DocumentStore`, and assign it.

- [ ] **Step 2: Register the routes**

In `server.go` near line 176 (where `/docs` routes are), add:

```go
mux.Handle("GET /wissen",                  s.webAuth(http.HandlerFunc(s.handleWebWissenHome)))
mux.Handle("GET /ui/wissen/list",          s.webAuth(http.HandlerFunc(s.handleWebWissenList)))
mux.Handle("GET /wissen/neu",              s.webAuth(http.HandlerFunc(s.handleWebEditorNew)))
mux.Handle("POST /wissen/preview",         s.webAuth(http.HandlerFunc(s.handleWebEditorPreview)))
mux.Handle("POST /wissen",                 s.webAuth(http.HandlerFunc(s.handleWebEditorCreate)))
mux.Handle("GET /wissen/{id}",             s.webAuth(http.HandlerFunc(s.handleWebDocumentView)))
mux.Handle("GET /wissen/{id}/bearbeiten",  s.webAuth(http.HandlerFunc(s.handleWebEditorEdit)))
mux.Handle("POST /wissen/{id}",            s.webAuth(http.HandlerFunc(s.handleWebEditorUpdate)))
mux.Handle("POST /wissen/{id}/delete",     s.webAuth(http.HandlerFunc(s.handleWebEditorDelete)))
mux.Handle("POST /wissen/{id}/reembed",    s.webAuth(http.HandlerFunc(s.handleWebDocReembed))) // keep reembed handler
```

> `/wissen/neu` and `/wissen/preview` are registered before `/wissen/{id}` —
> Go 1.22 ServeMux prefers the more specific static pattern, but keep this order
> for clarity. `handleWebDocReembed` can stay in a small `webui_document.go`
> (move it there from `webui_docs.go` before deleting).

- [ ] **Step 3: Update the nav**

In `internal/adapter/webui/components/sitenav.templ`, change the docs entry's `href` to `/wissen`, its active-key to `"docs"` (matches `AppShell("docs", …)`), and its label key to one that reads "Wissen" (e.g. `nav.wissen`). Add `"nav.wissen":"Wissen"` to both catalogs. `make generate`.

- [ ] **Step 4: Delete the old surface**

Remove `internal/adapter/httpserver/webui_docs.go` and `internal/adapter/webui/docs.templ` (+ generated `docs_templ.go`) and the old `/docs` route block in `server.go`. Move any helper still referenced (e.g. `renderSnippet`, `EmbedBadge`, `EmbedView`, `truncateError`, `handleWebDocReembed`) into the wissen/document files first. Update/replace the old `webui_docs_test.go` files: port the still-relevant assertions to `webui_wissen_test.go` / `webui_document_test.go`, delete the rest.

- [ ] **Step 5: Build + full CI**

Run: `make ci`
Expected: green — `lint verify-generate verify-css verify-no-popups cover build` all pass, coverage ≥ 75%. Fix any compile/lint errors (unused imports after deletions are common). If coverage dropped below 75%, add table-driven tests to the lowest-covered new file (renderer or vm helpers).

- [ ] **Step 6: Curl-smoke each route (live dev stack)**

Run:
```bash
make dev-up        # Postgres + Dex
make dev-run &     # server in background
TOKEN=$(make -s dev-token)
for path in /wissen /wissen/neu; do
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" http://localhost:8080$path)
  echo "$path -> $code"
done
```
Expected: each prints `200`. Create a doc via the API or `POST /wissen`, then smoke `GET /wissen/{id}`, `GET /wissen/{id}/bearbeiten`, and `POST /wissen/preview` (expects 200 + `<table>` for a table body). Confirm `GET /docs` now returns 404.

> If the dev stack auth flow differs, follow `AGENTS.md` → "Dev stack". The exact
> port/host may differ — check `make dev-run` output.

- [ ] **Step 7: Browser done-gate (manual, record result)**

With `make dev-run` up, log in via the browser (Dex) and verify against the mockups
`docs/superpowers/specs/assets/2026-06-23-webui/studio-docs-categories.html` and `…/studio-document-view.html`:
- `/wissen`: four category sections render; category strip scroll-spy highlights; search returns `<mark>` snippets; tag chips filter; pagination at >50 docs.
- A document with a table, tasklist, callout, footnote, fenced code, and a `[[wikilink]]`: all render correctly; code highlighting flips with the Dark/Light toggle; backlinks + ToC show; mobile (≤420px) stacks ToC under text.
- Editor: typing updates the live preview; Save redirects to the read page.
- SSE: create/update/delete a doc in another tab → the list/read page updates without reload.
Record the outcome in the commit message / PR notes.

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "feat(webui): wire /wissen routes, retire /docs surface, done-gate

Wissen surface (list/read/editor + live preview) live on AppShell; old
/docs handlers + templates removed; nav points to /wissen. make ci green;
curl-smoke + browser done-gate verified against the dev stack."
```

---

## Self-Review

**Spec coverage check (spec §-by-§):**
- §4 Markdown pipeline (GFM/Footnote/Callouts/Chroma/Wikilink/Frontmatter) → Tasks 1,2,3 ✓; sanitize §4.3 → Tasks 1,2,3 sanitizer steps ✓; Chroma two-stylesheet §4.2 → Task 3 ✓; tests §4.4 → Tasks 1–3 test steps ✓.
- §5 list (category-specific, strip, search, tag-filter, pagination, SSE, states) → Task 6 ✓.
- §6 read page (header, prose ~70ch, ToC, backlinks, embed, SSE) → Task 7 ✓.
- §7 editor + live preview → Task 8 ✓.
- §8 backend (pagination + preview endpoint; no REST/TUI/MCP change) → Task 5 + Task 8 ✓.
- §9 SSE map → Tasks 6,7 container triggers ✓.
- §10 i18n → key-add steps in Tasks 6,7,8 ✓.
- §11 a11y/responsive → prose max-width (Task 4), aside stacking + scroll-spy (Tasks 6,7), browser gate (Task 9) ✓.
- §12 testing & done-gate → per-task tests + Task 9 `make ci` + curl + browser ✓.
- §13 slicing → task mapping table ✓.
- id-based routing decision (§2) → enforced in routes (Task 9) ✓.

**Placeholder scan:** The `|>` pseudo-operator in Task 2 Step 3 is explicitly flagged as NOT valid Go with the correct `util.EscapeHTML` replacement called out in the note — intentional teaching callout, not a silent placeholder. Page templ tasks (6–8) reference an exact mirror file + provide VM structs, handler bodies, and test assertions; the templ section-rendering is described with concrete ids/classes/keys rather than dumped verbatim (acceptable: existing-pattern mirroring, per writing-plans guidance for established codebases).

**Type consistency:** `WikilinkResolver`/`RenderDocument` signatures consistent across Tasks 1–3,7,8. `DocKindStyle`→`DocKind{Label,Glyph,Tone}` used consistently in Tasks 4,6,7. `ListPage`/`ListDocumentsPage.Execute` arg order consistent Tasks 5→6. `PageNav{Page,Total,PageSize,BaseHref}` consistent Tasks 6. `CreateDocumentInput`/`UpdateDocumentInput` match Task 8 usage.

**Known confirm-on-touch items (open the named file before coding):** pgstore scan-helper/`s.db` names (Task 5 Step 5); the package's test scaffolding names `newTestServer`/`withTestUser` (Tasks 8); `ResolveWikilink`'s exact contract for the preview resolver (Task 8 Step 3); the composition root location for usecase wiring (Task 9 Step 1); `sitenav.templ` active-key string (Task 9 Step 3). Each is flagged inline.
