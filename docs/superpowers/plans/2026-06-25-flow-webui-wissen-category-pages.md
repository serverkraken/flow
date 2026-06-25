# Wissen Category Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the Wissen WebUI into a calm `/wissen` overview plus category subpages with article previews, and move the mobile document table of contents before the article content.

**Architecture:** Keep document storage and domain types unchanged. Add WebUI category view models and helper functions in `internal/adapter/webui`, route `/wissen/<category>` explicitly before `/wissen/{id}`, and reuse the existing server-rendered templ + htmx + SSE pattern. Generate templ and Tailwind artifacts after template/CSS edits.

**Tech Stack:** Go, `net/http`, templ, htmx SSE, Tailwind CSS, existing `domain.DocumentType`, existing `usecase.ListDocumentsPage`/`SearchDocuments`.

---

## Files

- Modify `internal/adapter/webui/wissen_vm.go`: category definitions, preview builder, overview/category VMs.
- Modify `internal/adapter/webui/wissen_vm_test.go`: helper and VM tests.
- Modify `internal/adapter/webui/wissen.templ`: overview, category page, category cards, category rows with previews.
- Generated `internal/adapter/webui/wissen_templ.go`.
- Modify `internal/adapter/httpserver/webui_wissen.go`: route data builders and category handlers.
- Modify `internal/adapter/httpserver/webui_wissen_test.go`: route and filtering tests.
- Modify `internal/adapter/httpserver/server.go`: explicit category routes and fragments before `/wissen/{id}`.
- Modify `internal/adapter/httpserver/routes_test.go`: expected route list.
- Modify `internal/adapter/webui/document.templ`: mobile ToC before article content, desktop rail unchanged.
- Generated `internal/adapter/webui/document_templ.go`.
- Modify `internal/adapter/webui/document_layout_test.go`: mobile order test.
- Modify `internal/i18n/catalog_de.go` and `internal/i18n/catalog_en.go`: category descriptions and labels.
- Modify `web/tailwind.css`: line clamp fallback for preview text if needed.
- Generated `internal/adapter/webui/static/app.css`.

## Task 1: Category Model And Preview Helpers

**Files:**
- Modify: `internal/adapter/webui/wissen_vm.go`
- Modify: `internal/adapter/webui/wissen_vm_test.go`

- [ ] **Step 1: Write failing tests**

Add tests covering category mapping, preview generation, and overview latest items:

```go
func TestWissenCategoryFromSlug(t *testing.T) {
	tests := map[string]struct {
		wantID string
		ok     bool
	}{
		"daily":    {"daily", true},
		"projekte": {"projekte", true},
		"frei":     {"frei", true},
		"system":   {"system", true},
		"bogus":    {"", false},
	}
	for slug, tt := range tests {
		got, ok := WissenCategoryFromSlug(slug)
		if ok != tt.ok || got.ID != tt.wantID {
			t.Fatalf("WissenCategoryFromSlug(%q) = (%q,%v), want (%q,%v)", slug, got.ID, ok, tt.wantID, tt.ok)
		}
	}
}

func TestDocPreviewTextStripsMarkdownAndLimitsLines(t *testing.T) {
	body := "---\ntags: [x]\n---\n# Heading\n\n- first [[daily/2026-06-25|Daily Link]]\n<script>alert(1)</script>\n```terraform\nresource \"x\" \"y\" {}\n```\nplain **bold** text\nsixth line\n"
	got := DocPreviewText(body, 5)
	for _, bad := range []string{"---", "tags:", "<script>", "```", "**", "[[", "]]"} {
		if strings.Contains(got, bad) {
			t.Fatalf("preview leaked %q: %q", bad, got)
		}
	}
	lines := strings.Split(got, "\n")
	if len(lines) > 5 {
		t.Fatalf("preview lines=%d, want <=5: %q", len(lines), got)
	}
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "Daily Link") {
		t.Fatalf("preview missing readable content: %q", got)
	}
}

func TestBuildWissenOverviewCountsAndLatest(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocDaily, Title: "Old daily", UpdatedAt: now.Add(-time.Hour)},
		{ID: "d2", Type: domain.DocDaily, Title: "New daily", UpdatedAt: now},
		{ID: "p1", Type: domain.DocProject, Title: "Project", UpdatedAt: now},
		{ID: "m1", Type: domain.DocMemory, Title: "Memory", UpdatedAt: now},
	}
	vm := BuildWissenOverview(docs, nil, nil)
	daily := vm.Categories[0]
	if daily.Count != 2 || len(daily.Latest) != 2 || daily.Latest[0].Title != "New daily" {
		t.Fatalf("daily overview = %+v", daily)
	}
	system := vm.Categories[3]
	if system.Count != 1 || system.Latest[0].Title != "Memory" {
		t.Fatalf("system overview = %+v", system)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/webui -run 'Test(WissenCategoryFromSlug|DocPreviewText|BuildWissenOverview)' -count=1 -v`

Expected: FAIL because `WissenCategoryFromSlug`, `DocPreviewText`, and `BuildWissenOverview` do not exist yet.

- [ ] **Step 3: Implement minimal helpers**

Add focused types/functions to `wissen_vm.go`:

```go
type WissenCategory struct {
	ID             string
	Slug           string
	LabelKey       string
	DescriptionKey string
	Href           string
	Types          []domain.DocumentType
}

type WissenOverviewVM struct {
	WissenVM
	Categories []WissenCategoryCard
}

type WissenCategoryCard struct {
	WissenCategory
	Count  int
	Latest []DocRow
}

type WissenCategoryVM struct {
	WissenVM
	Category WissenCategory
	Rows     []DocRow
	Groups   []ProjectGroup
	Total    int
}

var (
	wikiAliasPreviewRE  = regexp.MustCompile(`\[\[([^|\]]+)\|([^\]]+)\]\]`)
	wikiSimplePreviewRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	htmlPreviewRE       = regexp.MustCompile(`<[^>]*>`)
	markdownPreviewRE   = regexp.MustCompile(`[*_~` + "`" + `]+([^*_~` + "`" + `]+)[*_~` + "`" + `]+`)
)

func WissenCategories() []WissenCategory {
	return []WissenCategory{
		{ID: "daily", Slug: "daily", LabelKey: "wissen.daily", DescriptionKey: "wissen.daily.description", Href: "/wissen/daily", Types: []domain.DocumentType{domain.DocDaily}},
		{ID: "projekte", Slug: "projekte", LabelKey: "wissen.notes", DescriptionKey: "wissen.notes.description", Href: "/wissen/projekte", Types: []domain.DocumentType{domain.DocProject}},
		{ID: "frei", Slug: "frei", LabelKey: "wissen.free", DescriptionKey: "wissen.free.description", Href: "/wissen/frei", Types: []domain.DocumentType{domain.DocFree}},
		{ID: "system", Slug: "system", LabelKey: "wissen.system", DescriptionKey: "wissen.system.description", Href: "/wissen/system", Types: []domain.DocumentType{domain.DocAgent, domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocPlan}},
	}
}

func WissenCategoryFromSlug(slug string) (WissenCategory, bool) {
	for _, c := range WissenCategories() {
		if c.Slug == slug {
			return c, true
		}
	}
	return WissenCategory{}, false
}

func DocumentInWissenCategory(d domain.Document, c WissenCategory) bool {
	for _, typ := range c.Types {
		if d.Type == typ {
			return true
		}
	}
	return false
}

func BuildWissenOverview(docs []domain.Document, projectNames, projectColors map[string]string) WissenOverviewVM {
	vm := WissenOverviewVM{}
	for _, c := range WissenCategories() {
		card := WissenCategoryCard{WissenCategory: c}
		for _, d := range docs {
			if !DocumentInWissenCategory(d, c) {
				continue
			}
			card.Count++
			if len(card.Latest) < 3 {
				card.Latest = append(card.Latest, docRowFromDocument(d, projectColors))
			}
		}
		vm.Categories = append(vm.Categories, card)
	}
	return vm
}

func BuildWissenCategory(c WissenCategory, docs []domain.Document, projectNames, projectColors map[string]string) WissenCategoryVM {
	filtered := make([]domain.Document, 0, len(docs))
	for _, d := range docs {
		if DocumentInWissenCategory(d, c) {
			filtered = append(filtered, d)
		}
	}
	grouped := GroupDocsByCategory(filtered, projectNames, projectColors)
	vm := WissenCategoryVM{WissenVM: grouped, Category: c, Groups: grouped.Notes, Total: len(filtered)}
	for _, d := range filtered {
		row := docRowFromDocument(d, projectColors)
		row.Preview = DocPreviewText(d.Body, 5)
		vm.Rows = append(vm.Rows, row)
	}
	return vm
}

func DocPreviewText(body string, maxLines int) string {
	body = stripPreviewFrontmatter(body)
	var out []string
	inFence := false
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		line = simplifyPreviewMarkdown(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) == maxLines {
			break
		}
	}
	return strings.Join(out, "\n")
}

func stripPreviewFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	if end := strings.Index(body[4:], "\n---\n"); end >= 0 {
		return body[end+9:]
	}
	return body
}

func simplifyPreviewMarkdown(line string) string {
	line = strings.TrimLeft(line, "#>-*+ \t")
	line = wikiAliasPreviewRE.ReplaceAllString(line, "$2")
	line = wikiSimplePreviewRE.ReplaceAllString(line, "$1")
	line = htmlPreviewRE.ReplaceAllString(line, "")
	line = markdownPreviewRE.ReplaceAllString(line, "$1")
	return strings.TrimSpace(line)
}
```

Implementation notes:
- `daily` includes `DocDaily`.
- `projekte` includes `DocProject`.
- `frei` includes `DocFree`.
- `system` includes `DocAgent`, `DocMemory`, `DocInstruction`, `DocSkill`, `DocPlan`.
- `DocPreviewText` should strip frontmatter, skip fenced code contents, remove simple Markdown markers, convert `[[path|Label]]` to `Label`, convert `[[path]]` to `path`, strip HTML tags with a small regexp, compact blank lines, and cap to `maxLines`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/webui -run 'Test(WissenCategoryFromSlug|DocPreviewText|BuildWissenOverview)' -count=1 -v`

Expected: PASS.

- [ ] **Step 5: Run CI before marking task done**

Run: `env -u NO_COLOR make ci`

Expected: PASS with coverage at or above 75%.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/wissen_vm.go internal/adapter/webui/wissen_vm_test.go
git commit -m "feat(webui): add Wissen category view models"
```

## Task 2: `/wissen` Overview Page

**Files:**
- Modify: `internal/adapter/webui/wissen.templ`
- Generated: `internal/adapter/webui/wissen_templ.go`
- Modify: `internal/adapter/httpserver/webui_wissen.go`
- Modify: `internal/adapter/httpserver/webui_wissen_test.go`
- Modify: `internal/i18n/catalog_de.go`
- Modify: `internal/i18n/catalog_en.go`

- [ ] **Step 1: Write failing route/template tests**

Add or update tests in `webui_wissen_test.go`:

```go
func TestWebWissenHomeOverviewCards(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily One", Body: "daily body", Date: &now, CreatedAt: now, UpdatedAt: now},
		{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/idea", Title: "Free One", Body: "free body", CreatedAt: now, UpdatedAt: now},
	} {
		_, _ = docs.Create(context.Background(), doc)
	}
	body, status := getWissen(t, wissenTestMux(srv), "/wissen", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen status=%d body=%.300s", status, body)
	}
	for _, want := range []string{`href="/wissen/daily"`, `href="/wissen/projekte"`, `href="/wissen/frei"`, `href="/wissen/system"`, "Daily One", "Free One"} {
		if !strings.Contains(body, want) {
			t.Fatalf("overview missing %q in %.1200s", want, body)
		}
	}
	for _, notWant := range []string{"daily-sec", "notes-sec", "free-sec", "system-sec"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("overview should not render old long section %q", notWant)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/httpserver -run TestWebWissenHomeOverviewCards -count=1 -v`

Expected: FAIL because `/wissen` still renders old long sections and has no category card links.

- [ ] **Step 3: Implement overview data and template**

In `webui_wissen.go`, add `wissenOverviewData` that loads tags, all documents, project metadata, and returns the value from `webui.BuildWissenOverview(docs, names, colors)` with shared search/tag fields. Keep `?q=` behavior using the existing search results.

In `wissen.templ`:
- make `WissenPage` accept an overview VM or add `WissenOverviewPage(vm WissenOverviewVM)`;
- render header/search/tags;
- for non-search overview, render a grid of category cards from `vm.Categories`;
- each card links to `card.Href` and shows `card.Count` plus `card.Latest` titles;
- do not show body previews on overview.

Add i18n keys:

```go
"wissen.overview":             "Übersicht",
"wissen.daily.description":    "Journal- und Tagesnotizen.",
"wissen.notes.description":    "Notizen, Entscheidungen und Kontext zu Projekten.",
"wissen.free.description":     "Freie Ideen und lose Notizen.",
"wissen.system.description":   "Agenten-, Speicher- und Systemdokumente.",
"wissen.latest":               "Zuletzt aktualisiert",
```

English equivalents:

```go
"wissen.overview":             "Overview",
"wissen.daily.description":    "Journal and daily notes.",
"wissen.notes.description":    "Notes, decisions, and project context.",
"wissen.free.description":     "Freeform ideas and loose notes.",
"wissen.system.description":   "Agent, memory, and system documents.",
"wissen.latest":               "Recently updated",
```

- [ ] **Step 4: Generate templ and run tests**

Run:

```bash
make generate
go test ./internal/adapter/httpserver -run 'TestWebWissen(HomeOverviewCards|Search)' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run CI before marking task done**

Run: `env -u NO_COLOR make ci`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/wissen.templ internal/adapter/webui/wissen_templ.go internal/adapter/httpserver/webui_wissen.go internal/adapter/httpserver/webui_wissen_test.go internal/i18n/catalog_de.go internal/i18n/catalog_en.go
git commit -m "feat(webui): render Wissen overview"
```

## Task 3: Category Subpages With Previews

**Files:**
- Modify: `internal/adapter/httpserver/server.go`
- Modify: `internal/adapter/httpserver/routes_test.go`
- Modify: `internal/adapter/httpserver/webui_wissen.go`
- Modify: `internal/adapter/httpserver/webui_wissen_test.go`
- Modify: `internal/adapter/webui/wissen.templ`
- Generated: `internal/adapter/webui/wissen_templ.go`
- Modify: `web/tailwind.css`
- Generated: `internal/adapter/webui/static/app.css`

- [ ] **Step 1: Write failing route tests**

Add tests:

```go
func TestWebWissenCategoryRoutesFilterDocuments(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, doc := range []domain.Document{
		{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily One", Body: "daily preview\nline two", Date: &now, CreatedAt: now, UpdatedAt: now},
		{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/idea", Title: "Free One", Body: "free preview", CreatedAt: now, UpdatedAt: now},
		{ID: "mem-1", OwnerID: "u1", Type: domain.DocMemory, Path: "memory/x", Title: "Memory One", Body: "memory preview", CreatedAt: now, UpdatedAt: now},
	} {
		_, _ = docs.Create(ctx, doc)
	}
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/daily", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/daily status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Daily One") || !strings.Contains(body, "daily preview") {
		t.Fatalf("daily page missing daily doc/preview: %.1000s", body)
	}
	for _, notWant := range []string{"Free One", "Memory One"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("daily page leaked %q in %.1000s", notWant, body)
		}
	}
}

func TestWebWissenCategorySearchIsCategoryScoped(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	_, _ = docs.Create(context.Background(), domain.Document{ID: "daily-1", OwnerID: "u1", Type: domain.DocDaily, Path: "daily/2026-06-25", Title: "Daily Alpha", Body: "alpha", Date: &now, CreatedAt: now, UpdatedAt: now})
	_, _ = docs.Create(context.Background(), domain.Document{ID: "free-1", OwnerID: "u1", Type: domain.DocFree, Path: "free/alpha", Title: "Free Alpha", Body: "alpha", CreatedAt: now, UpdatedAt: now})
	body, status := getWissen(t, wissenTestMux(srv), "/wissen/frei?q=alpha", codec)
	if status != http.StatusOK {
		t.Fatalf("GET /wissen/frei?q=alpha status=%d body=%.300s", status, body)
	}
	if !strings.Contains(body, "Free Alpha") || strings.Contains(body, "Daily Alpha") {
		t.Fatalf("free search not category scoped: %.1000s", body)
	}
}
```

Update `wissenTestMux` with:

```go
for _, slug := range []string{"daily", "projekte", "frei", "system"} {
	mux.Handle("GET /wissen/"+slug, s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
	mux.Handle("GET /ui/wissen/list/"+slug, s.webAuth(http.HandlerFunc(s.handleWebWissenCategoryList)))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/httpserver -run 'TestWebWissenCategory' -count=1 -v`

Expected: FAIL with 404 or missing preview/category filtering.

- [ ] **Step 3: Implement routes, handlers, and template**

In `server.go`, register before `/wissen/{id}`:

```go
mux.Handle("GET /wissen/daily", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
mux.Handle("GET /wissen/projekte", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
mux.Handle("GET /wissen/frei", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
mux.Handle("GET /wissen/system", s.webAuth(http.HandlerFunc(s.handleWebWissenCategory)))
mux.Handle("GET /ui/wissen/list/{category}", s.webAuth(http.HandlerFunc(s.handleWebWissenCategoryList)))
```

In `webui_wissen.go`:
- parse category from `r.PathValue("category")` or trimmed `/wissen/<slug>`;
- for list mode, load documents and filter with `webui.DocumentInWissenCategory`;
- for search mode, call `SearchDocuments.Execute(r.Context(), u.ID, q, nil, active)`, then filter hits by category;
- compute pagination after filtering if using all-doc handler filtering;
- set `Page.BaseHref` to `/wissen/<slug>` plus query;
- render `WissenCategoryPage(vm)` and `WissenCategoryFragment(vm)`.

In `wissen.templ`:
- add category page wrapper with htmx fragment URL `/ui/wissen/list/<slug> + vm.Query`;
- add category navigation links;
- render project category with existing grouped project articles;
- render non-project categories as rows/cards with `row.Preview`;
- add the local preview clamp class defined in `web/tailwind.css`.

If Tailwind line clamp is unavailable, add CSS in `web/tailwind.css`:

```css
.preview-clamp {
  display: -webkit-box;
  -webkit-line-clamp: 5;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
```

- [ ] **Step 4: Generate CSS/templates and run tests**

Run:

```bash
make generate
make web
go test ./internal/adapter/httpserver ./internal/adapter/webui -run 'TestWebWissenCategory|TestWissenCategory|TestDocPreviewText' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run CI before marking task done**

Run: `env -u NO_COLOR make ci`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/httpserver/server.go internal/adapter/httpserver/routes_test.go internal/adapter/httpserver/webui_wissen.go internal/adapter/httpserver/webui_wissen_test.go internal/adapter/webui/wissen.templ internal/adapter/webui/wissen_templ.go web/tailwind.css internal/adapter/webui/static/app.css
git commit -m "feat(webui): add Wissen category pages"
```

## Task 4: Mobile Document ToC Before Content

**Files:**
- Modify: `internal/adapter/webui/document.templ`
- Generated: `internal/adapter/webui/document_templ.go`
- Modify: `internal/adapter/webui/document_layout_test.go`

- [ ] **Step 1: Write failing layout test**

Add test:

```go
func TestDocumentFragmentMobileTocPrecedesMarkdownAndBacklinks(t *testing.T) {
	vm := DocumentVM{
		ID: "d1", Title: "Doc", KindLabel: "Frei", KindGlyph: "○", KindTone: "highlight",
		HTML: templ.HTML("<h2 id=\"one\">One</h2><p>Body</p>"),
	}
	out := renderDocumentComponent(t, DocumentFragment(vm))
	mobile := strings.Index(out, `data-mobile-toc`)
	prose := strings.Index(out, `data-document-prose`)
	backlinks := strings.Index(out, `data-document-backlinks`)
	if mobile < 0 || prose < 0 || backlinks < 0 {
		t.Fatalf("missing layout markers: mobile=%d prose=%d backlinks=%d\n%s", mobile, prose, backlinks, out)
	}
	if !(mobile < prose && prose < backlinks) {
		t.Fatalf("mobile order wrong: mobile=%d prose=%d backlinks=%d", mobile, prose, backlinks)
	}
	if !strings.Contains(out, `data-mobile-toc`) || !strings.Contains(out, `data-desktop-toc`) {
		t.Fatalf("expected mobile/desktop ToC split classes in %.1200s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/webui -run TestDocumentFragmentMobileTocPrecedesMarkdownAndBacklinks -count=1 -v`

Expected: FAIL because ToC only exists in the right rail after prose.

- [ ] **Step 3: Implement layout split**

In `document.templ`:
- add a mobile-only wrapper marked with `data-mobile-toc` after the header;
- add `data-document-prose` to the markdown section;
- wrap desktop rail ToC in a desktop-only wrapper marked with `data-desktop-toc`;
- add `data-document-backlinks` around Backlinks;
- keep right rail sticky only on desktop.

- [ ] **Step 4: Generate and run tests**

Run:

```bash
make generate
go test ./internal/adapter/webui -run TestDocumentFragmentMobileTocPrecedesMarkdownAndBacklinks -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run CI before marking task done**

Run: `env -u NO_COLOR make ci`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/webui/document.templ internal/adapter/webui/document_templ.go internal/adapter/webui/document_layout_test.go
git commit -m "fix(webui): move mobile document toc before content"
```

## Task 5: Final Route And Live Verification Pass

**Files:**
- Modify tests only if this task reveals a small missing assertion.

- [ ] **Step 1: Run focused WebUI test suites**

Run:

```bash
go test ./internal/adapter/httpserver ./internal/adapter/webui ./internal/adapter/webui/components -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full CI**

Run: `env -u NO_COLOR make ci`

Expected: PASS with `golangci-lint run` reporting `0 issues`, generated checks OK, CSS checks OK, no popup checks OK, coverage at or above 75%, and both binaries building.

- [ ] **Step 3: Inspect final worktree**

Run:

```bash
git status --short --branch
git log --oneline -6
```

Expected: branch is ahead by the new task commits; `.mcp.json` may remain untracked and must not be staged.

- [ ] **Step 4: Commit only if final verification required a test/assertion change**

If Task 5 changed no files, do not create an empty commit. If it did, commit with:

```bash
git add internal/adapter/httpserver/webui_wissen_test.go internal/adapter/webui/wissen_vm_test.go internal/adapter/webui/document_layout_test.go
git commit -m "test(webui): cover Wissen category routing"
```

## Self-Review

- Spec coverage: routes, overview, category subpages, previews, mobile ToC, SSE fragment pattern, search/tags/pagination, i18n, generated assets, and CI gates are covered.
- Placeholder scan: no open placeholder markers or vague deferred steps.
- Type consistency: plan uses existing `domain.DocumentType`, `DocRow`, `ProjectGroup`, `WissenVM`, `DocumentVM`, and adds only explicitly named WebUI helpers.
