package httpserver_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// newWebNodesServerFull builds a test server with node + tag usecases wired.
// Returns the server, session cookie, fake node store, and fake tag store.
func newWebNodesServerFull(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore, *testutil.FakeTagStore) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	bs := testutil.NewFakeProjectBindingStore()
	tags := testutil.NewFakeTagStore()
	ls := testutil.NewFakeNodeLogoStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	ss := testutil.NewFakeSessionStore()
	docs := testutil.NewFakeDocumentStore()
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Users:   users,
		Session: codec,
		Bus:     bus,
		Emitter: sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:   clk,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   ids,
			Allow: func(ports.Identity) bool { return true },
		},
		CreateNode:            usecase.CreateNode{Nodes: ns, IDs: ids, Clock: clk},
		ListNodes:             usecase.ListNodes{Nodes: ns},
		GetNode:               usecase.GetNode{Nodes: ns},
		UpdateNode:            usecase.UpdateNode{Nodes: ns, Bindings: bs, IDs: ids, Clock: clk},
		DeleteNode:            usecase.DeleteNode{Nodes: ns},
		SetNodeRate:           usecase.SetNodeRate{Nodes: ns},
		SetCountsTowardTarget: usecase.SetCountsTowardTarget{Nodes: ns, Clock: clk},
		UploadNodeLogo:        usecase.UploadNodeLogo{Nodes: ns, Logos: ls, Clock: clk},
		DeleteNodeLogo:        usecase.DeleteNodeLogo{Nodes: ns, Logos: ls, Clock: clk},
		GetNodeLogo:           usecase.GetNodeLogo{Logos: ls},
		MoveNode:              usecase.MoveNode{Nodes: ns},
		NodeAncestors:         usecase.NodeAncestors{Nodes: ns},
		ListNodeBindings:      usecase.ListNodeBindings{Bindings: bs},
		ListSessionsRange:     usecase.ListSessionsRange{Sessions: ss},
		GetRunningSession:     usecase.GetRunningSession{Sessions: ss},
		ListDocuments:         usecase.ListDocuments{Docs: docs},
		SetTags:               usecase.SetTags{Tags: tags},
		GetTags:               usecase.GetTags{Tags: tags},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ns,
			Clock:    clk,
			Loc:      time.UTC,
		},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return ts, &http.Cookie{Name: "flow_session", Value: cv}, ns, tags
}

// newWebNodesServer builds a test server with the node usecases wired and a
// seeded user. Returns the server, a session cookie, and the fake node store.
func newWebNodesServer(t *testing.T) (*httptest.Server, *http.Cookie, *testutil.FakeNodeStore) {
	t.Helper()
	ts, c, ns, _ := newWebNodesServerFull(t)
	return ts, c, ns
}

// seedTreeNode seeds a node with the given kind, name, and optional parent.
// It is distinct from the nodemove_test seedNode to allow a return value.
func seedTreeNode(t *testing.T, ns *testutil.FakeNodeStore, id, name string, kind domain.NodeKind, parent *string) domain.Node {
	t.Helper()
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	n, err := domain.NewNode(id, "u1", name, name, now)
	if err != nil {
		t.Fatalf("seedTreeNode NewNode: %v", err)
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
	b, _ := io.ReadAll(res.Body)
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

// postNMultipart is postN's multipart counterpart: posts a raw body under the
// given Content-Type (a multipart writer's boundary-bearing type) so tests
// can exercise the node form's file (logo) upload field.
func postNMultipart(t *testing.T, ts *httptest.Server, c *http.Cookie, path, contentType string, body *bytes.Buffer) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, body)
	req.AddCookie(c)
	req.Header.Set("Content-Type", contentType)
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestWebNodesPage_TreeAsContentAndFilter pins the Lesesaal L2 Task 3 redesign
// of /nodes: the tree renders as content (an .eng section per engagement, a
// .vh group per vorhaben, .projrow.lvl2 rows nested under it) rather than an
// indented <li> list — replacing the pre-Task-3
// "padding-left:1rem"-indentation assertion, which no longer applies now that
// nesting is expressed via named CSS classes instead of inline depth styles.
// The status filter (default hides archived; ?status=archived shows it)
// survives the redesign untouched.
func TestWebNodesPage_TreeAsContentAndFilter(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed: an engagement with a vorhaben group containing one repo (nested →
	// .lvl2), and an archived engagement.
	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	vor := seedTreeNode(t, ns, "vor1", "Buch", domain.KindVorhaben, &eng.ID)
	repo := seedTreeNode(t, ns, "repo1", "flow", domain.KindRepo, &vor.ID)
	repo.Color = domain.NodeColors[0]
	repo.Glyph = domain.NodeGlyphs[0]
	repo.UpstreamGit = "git@github.com:serverkraken/flow.git"
	_, _ = ns.Update(context.Background(), "u1", repo)
	arch := seedTreeNode(t, ns, "eng2", "Alt", domain.KindEngagement, nil)
	arch.Status = domain.NodeArchived
	_, _ = ns.Update(context.Background(), "u1", arch)

	// Default view: GET /nodes.
	code, body := getN(t, ts, c, "/nodes")
	if code != 200 {
		t.Fatalf("GET /nodes = %d; body=%.500s", code, body)
	}
	// Engagement, its vorhaben group and the nested repo must all appear.
	for _, want := range []string{"Privat", "Buch", "flow", `class="eng"`, `class="vh"`, `class="typechip"`} {
		if !strings.Contains(body, want) {
			t.Errorf("tree page missing %q; body=%.500s", want, body)
		}
	}
	// The repo row is nested under the vorhaben's .vh head → .lvl2.
	if !strings.Contains(body, `class="projrow lvl2"`) {
		t.Errorf("nested repo row missing projrow lvl2; body=%.500s", body)
	}
	// Archived node must be hidden by default.
	if strings.Contains(body, "Alt") {
		t.Errorf("default view must hide archived; body=%.500s", body)
	}

	// Archived filter: GET /nodes?status=archived.
	_, arr := getN(t, ts, c, "/nodes?status=archived")
	if !strings.Contains(arr, "Alt") {
		t.Errorf("archived filter must show Alt; body=%.500s", arr)
	}

	// SSE fragment route must return 200 and render the same nested row.
	code, frag := getN(t, ts, c, "/ui/nodes/list")
	if code != 200 {
		t.Errorf("GET /ui/nodes/list = %d, want 200", code)
	}
	if !strings.Contains(frag, `class="projrow lvl2"`) {
		t.Errorf("fragment missing nested projrow lvl2; body=%.500s", frag)
	}
}

// TestWebNodesPage_QuietCTAAndAvatarIdentity pins the Lesesaal L2 Task 3
// redesign of /nodes: the "Neuer Knoten" action is the quiet .btn.btn-q
// (not the old bg-gradient CTA), and a node's visual identity on this page is
// carried ONLY by its avatar (initials + deterministic tone) — never by the
// node's own stored Color/Glyph fields or a kind glyph (Spec §7 Farb-Gesetz:
// color lives only in the avatar; ◆▲● are dead everywhere on L2). The row
// link and the full remote-style path survive untouched.
func TestWebNodesPage_QuietCTAAndAvatarIdentity(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	repo := seedTreeNode(t, ns, "repo1", "gitlab.com/x/y/flow", domain.KindRepo, &eng.ID)
	repo.Color = domain.NodeColors[0]
	repo.Glyph = domain.NodeGlyphs[0]
	_, _ = ns.Update(context.Background(), "u1", repo)

	code, body := getN(t, ts, c, "/nodes")
	if code != 200 {
		t.Fatalf("GET /nodes = %d; body=%.500s", code, body)
	}

	// "Neuer Knoten" CTA: quiet .btn.btn-q, not the old bg-gradient chrome.
	ctaIdx := strings.Index(body, `href="/nodes/new"`)
	if ctaIdx < 0 {
		t.Fatalf("Neuer Knoten link /nodes/new not found; body=%.500s", body)
	}
	ctaCloseOffset := strings.Index(body[ctaIdx:], "</a>")
	if ctaCloseOffset < 0 {
		t.Fatalf("Neuer Knoten anchor not closed; body=%.500s", body)
	}
	ctaBlock := body[ctaIdx : ctaIdx+ctaCloseOffset]
	if !strings.Contains(ctaBlock, "btn-q") {
		t.Errorf("Neuer Knoten button missing the quiet btn-q class: %s", ctaBlock)
	}
	if strings.Contains(ctaBlock, "bg-gradient-to-r") {
		t.Errorf("Neuer Knoten button still uses the old gradient CTA: %s", ctaBlock)
	}

	// Row link + full path survive; the row's own Color/Glyph never render on
	// this page — identity comes from the avatar's tone/initials only.
	if !strings.Contains(body, `href="/nodes/`+repo.ID+`"`) {
		t.Errorf("tree row link to /nodes/%s missing; body=%.500s", repo.ID, body)
	}
	if !strings.Contains(body, "gitlab.com/x/y/flow") {
		t.Errorf("full mono path missing; body=%.500s", body)
	}
	if strings.Contains(body, "background-color:"+webui.ColorHex(repo.Color)) {
		t.Errorf("node's own Color must not render as a swatch on this page (Farb-Gesetz): body=%.500s", body)
	}
	if strings.Contains(body, repo.Glyph) {
		t.Errorf("node's own Glyph must not render on this page (Farb-Gesetz): body=%.500s", body)
	}
	for _, dead := range []string{"◆", "▲", "●"} {
		if strings.Contains(body, dead) {
			t.Errorf("dead kind-glyph %q must not render on the Projekte page; body=%.500s", dead, body)
		}
	}
	// Avatar identity: initials tile at the engagement (av-36) and repo (av-28)
	// sizes, both carrying a deterministic av-a..av-f tone class.
	if !strings.Contains(body, "av-36") || !strings.Contains(body, "av-28") {
		t.Errorf("avatar identity tiles missing (av-36 engagement / av-28 repo); body=%.500s", body)
	}
}

// seedEngNode seeds an engagement with the given status (extends seedTreeNode).
func seedEngNode(t *testing.T, ns *testutil.FakeNodeStore, id, name string, status domain.NodeStatus) domain.Node {
	t.Helper()
	n := seedTreeNode(t, ns, id, name, domain.KindEngagement, nil)
	if status != domain.NodeActive {
		n.Status = status
		_, _ = ns.Update(context.Background(), "u1", n)
	}
	return n
}

// TestWebNodeCockpit verifies the node cockpit at GET /nodes/{id}: ancestor
// breadcrumb, kind badge, cockpit shell ids, and 404 on unknown ID.
func TestWebNodeCockpit(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	eng := seedTreeNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)
	repo := seedTreeNode(t, ns, "r1", "flow", domain.KindRepo, &eng.ID)
	repo.Description = "# Notiz\nhallo"
	repo.UpstreamGit = "git@github.com:serverkraken/flow.git"
	_, _ = ns.Update(context.Background(), "u1", repo)

	code, body := getN(t, ts, c, "/nodes/r1")
	if code != 200 {
		t.Fatalf("cockpit = %d; body=%.700s", code, body)
	}
	for _, want := range []string{
		"flow",              // node name
		"RTL Extern",        // ancestor breadcrumb (engagement parent)
		"Repo",              // kind badge label
		`id="cockpit-rail"`, // Kristall K2 rail shell id (renamed from the old head container)
		`id="cockpit-main"`, // cockpit shell id
	} {
		if !strings.Contains(body, want) {
			t.Errorf("cockpit missing %q; body=%.700s", want, body)
		}
	}
	if code, _ := getN(t, ts, c, "/nodes/nope"); code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", code)
	}
}

// Ensure postN compiles (coverage guard).
var _ = postN

// TestWebNodeFormErrorPaths covers handler error branches: 404s and ErrNodeHasChildren.
func TestWebNodeFormErrorPaths(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed: engagement with a child repo.
	eng := seedTreeNode(t, ns, "eng1", "Privat", domain.KindEngagement, nil)
	seedTreeNode(t, ns, "repo1", "flow", domain.KindRepo, &eng.ID)

	// GET /nodes/{missing}/edit → 404
	code, _ := getN(t, ts, c, "/nodes/missing/edit")
	if code != http.StatusNotFound {
		t.Errorf("edit 404 = %d, want 404", code)
	}

	// POST /nodes/{missing} (update) → 404
	res := postN(t, ts, c, "/nodes/missing", url.Values{"name": {"x"}, "status": {"active"}})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("update 404 = %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()

	// POST /nodes/{missing}/status → 404
	res = postN(t, ts, c, "/nodes/missing/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status 404 = %d, want 404", res.StatusCode)
	}
	_ = res.Body.Close()

	// POST /nodes/{id}/delete with children → redirect with err=children
	res = postN(t, ts, c, "/nodes/"+eng.ID+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete-with-children = %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.Contains(loc, "err=children") {
		t.Errorf("delete-with-children redirect = %q, want ?err=children", loc)
	}
	// Engagement still exists (not deleted).
	if _, err := ns.Get(context.Background(), "u1", eng.ID); err != nil {
		t.Errorf("engagement should still exist after failed delete: %v", err)
	}
}

// TestWebNodeForm exercises the NodeForm-based create / edit / status / delete
// handlers (D4): kind select, constrained parent, engagement-only rate.
func TestWebNodeForm(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Seed an engagement to appear as a parent candidate.
	eng := seedTreeNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)

	// ── GET /nodes/new: form must contain the kind select and the parent option ──
	code, form := getN(t, ts, c, "/nodes/new")
	if code != 200 {
		t.Fatalf("GET /nodes/new = %d; body=%.500s", code, form)
	}
	if !strings.Contains(form, `select name="kind"`) {
		t.Errorf("new-form must contain kind select; body=%.500s", form)
	}
	if !strings.Contains(form, "RTL Extern") {
		t.Errorf("new-form must list parent engagement; body=%.500s", form)
	}

	// ── POST /nodes: create a repo under the engagement ──
	res := postN(t, ts, c, "/nodes", url.Values{
		"name":     {"flow"},
		"slug":     {"flow"},
		"kind":     {"repo"},
		"parentId": {eng.ID},
		"color":    {domain.NodeColors[0]},
		"glyph":    {domain.NodeGlyphs[0]},
		"status":   {"active"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create repo = %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if !strings.HasPrefix(loc, "/nodes/") {
		t.Fatalf("create redirect = %q, want /nodes/...", loc)
	}
	repoID := strings.TrimPrefix(loc, "/nodes/")

	got, err := ns.Get(context.Background(), "u1", repoID)
	if err != nil {
		t.Fatalf("created node not found: %v", err)
	}
	if got.Kind != domain.KindRepo {
		t.Errorf("created node kind = %q, want repo", got.Kind)
	}
	if got.ParentID == nil || *got.ParentID != eng.ID {
		t.Errorf("created node parentID = %v, want %q", got.ParentID, eng.ID)
	}

	// ── GET /nodes/{id}/edit: form pre-fills kind and name ──
	_, editBody := getN(t, ts, c, "/nodes/"+repoID+"/edit")
	if !strings.Contains(editBody, "flow") {
		t.Errorf("edit form must pre-fill node name; body=%.500s", editBody)
	}
	if !strings.Contains(editBody, `value="repo"`) {
		t.Errorf("edit form must pre-fill kind=repo; body=%.500s", editBody)
	}

	// ── GET /nodes: tree renders color/glyph swatch for node with those fields ──
	_, treePage := getN(t, ts, c, "/nodes")
	if !strings.Contains(treePage, "flow") {
		t.Errorf("tree should show created repo; body=%.300s", treePage)
	}

	// ── POST /nodes: create engagement with rate ──
	res = postN(t, ts, c, "/nodes", url.Values{
		"name":         {"Beratung"},
		"kind":         {"engagement"},
		"rateAmount":   {"95.00"},
		"rateCurrency": {"EUR"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create engagement = %d, want 303", res.StatusCode)
	}
	eid := strings.TrimPrefix(res.Header.Get("Location"), "/nodes/")
	_ = res.Body.Close()
	e2, err2 := ns.Get(context.Background(), "u1", eid)
	if err2 != nil {
		t.Fatalf("created engagement not found: %v", err2)
	}
	if e2.Rate == nil || e2.Rate.Amount != 9500 {
		t.Errorf("engagement rate not set: %+v", e2.Rate)
	}

	// ── POST /nodes: repo with rateAmount must NOT store a rate (non-engagement) ──
	res = postN(t, ts, c, "/nodes", url.Values{
		"name":         {"flow-ratetest"},
		"kind":         {"repo"},
		"parentId":     {eng.ID},
		"rateAmount":   {"120.00"},
		"rateCurrency": {"EUR"},
		"status":       {"active"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create repo-with-rate = %d, want 303", res.StatusCode)
	}
	rateTestID := strings.TrimPrefix(res.Header.Get("Location"), "/nodes/")
	_ = res.Body.Close()
	rtNode, _ := ns.Get(context.Background(), "u1", rateTestID)
	if rtNode.Rate != nil {
		t.Errorf("repo must not store a rate, got %+v", rtNode.Rate)
	}

	// ── POST /nodes (missing name) → 400 re-render ──
	res = postN(t, ts, c, "/nodes", url.Values{"kind": {"repo"}, "parentId": {eng.ID}})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing name = %d, want 400", res.StatusCode)
	}
	_ = res.Body.Close()

	// ── POST /nodes/{id}/status → archive repo ──
	res = postN(t, ts, c, "/nodes/"+repoID+"/status", url.Values{"status": {"archived"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	got, _ = ns.Get(context.Background(), "u1", repoID)
	if got.Status != domain.NodeArchived {
		t.Errorf("not archived: %s", got.Status)
	}

	// ── POST /nodes/{id}/delete (leaf) → deleted ──
	res = postN(t, ts, c, "/nodes/"+repoID+"/delete", url.Values{})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	if _, err := ns.Get(context.Background(), "u1", repoID); err == nil {
		t.Errorf("node should be deleted")
	}
}

// tagFor returns the innermost enclosing tag (from the last "<" before marker
// to the next ">") — used to scope class-attribute assertions to a single
// element rather than the whole page/form.
func tagFor(t *testing.T, s, marker string) string {
	t.Helper()
	idx := strings.Index(s, marker)
	if idx < 0 {
		t.Fatalf("marker %q not found; body=%.800s", marker, s)
	}
	start := strings.LastIndex(s[:idx], "<")
	end := strings.Index(s[start:], ">")
	if start < 0 || end < 0 {
		t.Fatalf("tag for marker %q not well-formed; body=%.800s", marker, s)
	}
	return s[start : start+end]
}

// TestWebNodeForm_FieldClass pins the K4 Kristall form-language sweep
// (Task 7): every text/select/textarea/file input in the node create/edit
// form carries the shared .field class (introduced in Task 5) instead of the
// old hand-rolled "rounded-lg border border-line bg-surface" chrome, while
// every field name/id, the disabled-parent-select semantics, the color/glyph
// radio grids, and the cancel+submit actions survive untouched. The negative
// assertion is scoped to the <form>...</form> block itself (not the whole
// page) since AppShell's mobile nav legitimately carries "bg-surface"
// elsewhere (Task 4/5/6 false-positive lesson).
func TestWebNodeForm_FieldClass(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	eng := seedTreeNode(t, ns, "eng1", "RTL Extern", domain.KindEngagement, nil)

	code, body := getN(t, ts, c, "/nodes/new")
	if code != 200 {
		t.Fatalf("GET /nodes/new = %d; body=%.500s", code, body)
	}

	formMarker := `enctype="multipart/form-data"`
	markerIdx := strings.Index(body, formMarker)
	if markerIdx < 0 {
		t.Fatalf("node form (multipart) not found; body=%.500s", body)
	}
	formIdx := strings.LastIndex(body[:markerIdx], "<form")
	if formIdx < 0 {
		t.Fatalf("node form not found; body=%.500s", body)
	}
	formCloseOffset := strings.Index(body[formIdx:], "</form>")
	if formCloseOffset < 0 {
		t.Fatalf("node form not closed; body=%.500s", body)
	}
	formBlock := body[formIdx : formIdx+formCloseOffset+len("</form>")]

	// Negative: the old flat chrome must be entirely gone from the form.
	if strings.Contains(formBlock, "bg-surface") {
		t.Errorf("node form still uses flat bg-surface chrome: %.2000s", formBlock)
	}

	// Every field carries .field; font-mono / width utilities preserved where
	// the brief calls for them.
	cases := []struct {
		marker string
		want   []string
	}{
		{`name="name"`, []string{"field"}},
		{`name="slug"`, []string{"field", "font-mono"}},
		{`id="node-kind"`, []string{"field"}},
		{`id="node-parent"`, []string{"field"}}, // create-mode: enabled parent select
		{`name="description"`, []string{"field", "font-mono"}},
		{`name="upstreamGit"`, []string{"field", "font-mono"}},
		{`name="status"`, []string{"field"}},
		{`name="countsMode"`, []string{"field"}},
		{`name="logo"`, []string{"field"}},
		{`name="tags"`, []string{"field", "font-mono"}},
		{`name="rateAmount"`, []string{"field", "w-32"}},
		{`name="rateCurrency"`, []string{"field", "w-20"}},
	}
	for _, tc := range cases {
		el := tagFor(t, formBlock, tc.marker)
		for _, want := range tc.want {
			if !strings.Contains(el, want) {
				t.Errorf("field %s missing %q: %s", tc.marker, want, el)
			}
		}
	}

	// Radio grids preserved: color/glyph option values still render.
	for _, name := range domain.NodeColors {
		if !strings.Contains(formBlock, `value="`+name+`"`) {
			t.Errorf("color radio %q missing; body=%.500s", name, formBlock)
		}
	}
	for _, g := range domain.NodeGlyphs {
		if !strings.Contains(formBlock, `value="`+g+`"`) {
			t.Errorf("glyph radio %q missing; body=%.500s", g, formBlock)
		}
	}

	// Cancel + submit present; submit already routes through @components.Button.
	if !strings.Contains(formBlock, `href="/nodes"`) {
		t.Errorf("cancel link missing; body=%.500s", formBlock)
	}
	if !strings.Contains(formBlock, `type="submit"`) {
		t.Errorf("submit button missing; body=%.500s", formBlock)
	}

	// Parent option from the seeded engagement still renders.
	if !strings.Contains(formBlock, eng.Name) {
		t.Errorf("parent option %q missing; body=%.500s", eng.Name, formBlock)
	}

	// Edit form: the disabled parent-select keeps .field alongside its
	// disabled-semantics utilities (opacity-50 / cursor-not-allowed).
	res := postN(t, ts, c, "/nodes", url.Values{
		"name": {"flow"}, "slug": {"flow"}, "kind": {"repo"}, "parentId": {eng.ID},
		"status": {"active"},
	})
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	repoID := strings.TrimPrefix(loc, "/nodes/")

	_, editBody := getN(t, ts, c, "/nodes/"+repoID+"/edit")
	pTag := tagFor(t, editBody, `id="node-parent"`)
	for _, want := range []string{"field", "opacity-50", "cursor-not-allowed"} {
		if !strings.Contains(pTag, want) {
			t.Errorf("disabled parent-select missing %q: %s", want, pTag)
		}
	}
}

// TestWebNodeMove exercises POST /nodes/{id}/move: successful reparent,
// cycle-rejection, invalid-kind rejection, and not-found (all redirect 303).
func TestWebNodeMove(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	e1 := seedTreeNode(t, ns, "e1", "Privat", domain.KindEngagement, nil)
	e2 := seedTreeNode(t, ns, "e2", "RTL", domain.KindEngagement, nil)
	repo := seedTreeNode(t, ns, "r1", "flow", domain.KindRepo, &e1.ID)

	// valid reparent: move repo from e1 to e2.
	res := postN(t, ts, c, "/nodes/"+repo.ID+"/move", url.Values{"parentId": {e2.ID}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("move = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	got, _ := ns.Get(context.Background(), "u1", repo.ID)
	if got.ParentID == nil || *got.ParentID != e2.ID {
		t.Fatalf("reparent failed: %+v", got.ParentID)
	}

	// true cycle: seed vor1 under e1, repoCycle under vor1, then try to move
	// vor1 under repoCycle (its own descendant) → ErrNodeCycle, redirect 303,
	// vor1's parent remains e1.
	vor1 := seedTreeNode(t, ns, "vor1", "Arbeit", domain.KindVorhaben, &e1.ID)
	repoCycle := seedTreeNode(t, ns, "rcycle", "sub", domain.KindRepo, &vor1.ID)
	res = postN(t, ts, c, "/nodes/"+vor1.ID+"/move", url.Values{"parentId": {repoCycle.ID}})
	if res.StatusCode != http.StatusSeeOther {
		t.Errorf("cycle move = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()
	vor1got, _ := ns.Get(context.Background(), "u1", vor1.ID)
	if vor1got.ParentID == nil || *vor1got.ParentID != e1.ID {
		t.Errorf("cycle move must be rejected, parent=%v", vor1got.ParentID)
	}

	// invalid: move a repo to root (parentId="") → ErrInvalidNode, redirect with err=move.
	res = postN(t, ts, c, "/nodes/"+repo.ID+"/move", url.Values{"parentId": {""}})
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("invalid-kind move = %d, want 303", res.StatusCode)
	}
	if !strings.Contains(loc, "err=move") {
		t.Errorf("invalid-kind redirect = %q, want ?err=move", loc)
	}

	// not-found: move a nonexistent node → ErrNodeNotFound → redirect 303.
	res = postN(t, ts, c, "/nodes/ghost/move", url.Values{"parentId": {e2.ID}})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Errorf("move nonexistent = %d, want 303", res.StatusCode)
	}
}

// TestWebNodeFormTags verifies that posting tags=infra terraform to the node
// create form results in the node carrying those 2 tags (E2).
func TestWebNodeFormTags(t *testing.T) {
	t.Parallel()
	ts, c, ns, tags := newWebNodesServerFull(t)

	// POST /nodes: create an engagement with two tags.
	res := postN(t, ts, c, "/nodes", url.Values{
		"name": {"Infra Eng"},
		"kind": {"engagement"},
		"tags": {"infra terraform"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create with tags = %d, want 303", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	nodeID := strings.TrimPrefix(loc, "/nodes/")
	if nodeID == "" {
		t.Fatalf("redirect missing node id: %q", loc)
	}

	// Verify the two tags were stored via the fake tag store.
	stored, err := tags.TagsFor(context.Background(), "u1", domain.TaggableNode, nodeID)
	if err != nil {
		t.Fatalf("TagsFor: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("want 2 tags, got %d: %+v", len(stored), stored)
	}
	tagSlugs := map[string]bool{}
	for _, tg := range stored {
		tagSlugs[tg.Slug] = true
	}
	for _, want := range []string{"infra", "terraform"} {
		if !tagSlugs[want] {
			t.Errorf("tag %q missing; got %+v", want, stored)
		}
	}

	// GET /nodes/{id}/edit: form must prefill the tags field.
	_, editBody := getN(t, ts, c, "/nodes/"+nodeID+"/edit")
	if !strings.Contains(editBody, "infra") {
		t.Errorf("edit form must prefill tags; body=%.500s", editBody)
	}

	// Fetch the current node slug so the update form passes a valid slug.
	node, nerr := ns.Get(context.Background(), "u1", nodeID)
	if nerr != nil {
		t.Fatalf("ns.Get: %v", nerr)
	}

	// POST /nodes/{id}: update with a different single tag.
	res = postN(t, ts, c, "/nodes/"+nodeID, url.Values{
		"name":   {"Infra Eng"},
		"slug":   {node.Slug},
		"kind":   {"engagement"},
		"status": {"active"},
		"tags":   {"kubernetes"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("update with tags = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()

	stored, err = tags.TagsFor(context.Background(), "u1", domain.TaggableNode, nodeID)
	if err != nil {
		t.Fatalf("TagsFor after update: %v", err)
	}
	if len(stored) != 1 || stored[0].Slug != "kubernetes" {
		t.Fatalf("want [kubernetes] after update, got %+v", stored)
	}
}

// TestWebNodeForm_CountsModeTriState pins the Work/Privat tri-state through the
// WebUI form: create with privat → explicit false; edit back to inherit → nil
// (SetCountsTowardTarget always-apply, which UpdateNodeInput alone can't express);
// the edit form pre-selects the current mode.
func TestWebNodeForm_CountsModeTriState(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// Create with countsMode=privat → node persisted with explicit false.
	res := postN(t, ts, c, "/nodes", url.Values{
		"name": {"Privatkram"}, "kind": {"engagement"}, "status": {"active"},
		"countsMode": {"privat"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status=%d want 303", res.StatusCode)
	}
	nodes, _ := ns.List(context.Background(), "u1")
	var created domain.Node
	for _, n := range nodes {
		if n.Name == "Privatkram" {
			created = n
		}
	}
	if created.ID == "" {
		t.Fatal("created node not found")
	}
	if created.CountsTowardTarget == nil || *created.CountsTowardTarget != false {
		t.Fatalf("create: want explicit false (privat), got %v", created.CountsTowardTarget)
	}

	// Edit form pre-selects the current mode.
	code, body := getN(t, ts, c, "/nodes/"+created.ID+"/edit")
	if code != http.StatusOK {
		t.Fatalf("edit form status=%d", code)
	}
	if !strings.Contains(body, `name="countsMode"`) {
		t.Errorf("edit form missing countsMode select")
	}
	if !strings.Contains(body, `value="privat" selected`) {
		t.Errorf("edit form should pre-select privat: %.300s", body)
	}

	// Edit back to inherit → override cleared to nil (always-apply).
	res = postN(t, ts, c, "/nodes/"+created.ID, url.Values{
		"name": {"Privatkram"}, "slug": {created.Slug}, "kind": {"engagement"},
		"status": {"active"}, "countsMode": {"inherit"},
	})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("update status=%d want 303", res.StatusCode)
	}
	got, err := ns.Get(context.Background(), "u1", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CountsTowardTarget != nil {
		t.Fatalf("update to inherit: want nil, got %v", *got.CountsTowardTarget)
	}
}

// TestWebNodeForm_IconAndLogo pins the multipart node form: icon round-trips
// as a plain form field, an uploaded logo is stored on create (LogoRef =
// 12-hex content hash), the logoRemove checkbox clears it on update, and a
// disguised-as-image bad upload (here: SVG, sniffed via ValidateNodeLogo)
// rejects the whole create with 400 — no half-created node.
func TestWebNodeForm_IconAndLogo(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	// multipart create: name + icon + logo file
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Iconic")
	_ = mw.WriteField("kind", "engagement")
	_ = mw.WriteField("status", "active")
	_ = mw.WriteField("icon", "rocket")
	fw, _ := mw.CreateFormFile("logo", "logo.png")
	_, _ = fw.Write(pngPixel(t))
	_ = mw.Close()
	res := postNMultipart(t, ts, c, "/nodes", mw.FormDataContentType(), &buf)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create → %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	nodeID := strings.TrimPrefix(loc, "/nodes/")

	n, err := ns.Get(context.Background(), "u1", nodeID)
	if err != nil {
		t.Fatalf("created node not found: %v", err)
	}
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
	_ = mw2.WriteField("slug", n.Slug)
	_ = mw2.WriteField("kind", "engagement")
	_ = mw2.WriteField("status", "active")
	_ = mw2.WriteField("icon", "rocket")
	_ = mw2.WriteField("logoRemove", "1")
	_ = mw2.Close()
	res2 := postNMultipart(t, ts, c, "/nodes/"+n.ID, mw2.FormDataContentType(), &buf2)
	if res2.StatusCode != http.StatusSeeOther {
		t.Fatalf("update → %d", res2.StatusCode)
	}
	_ = res2.Body.Close()
	n2, err := ns.Get(context.Background(), "u1", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n2.LogoRef != "" {
		t.Errorf("logoRef = %q after remove, want empty", n2.LogoRef)
	}
	if n2.Icon != "rocket" {
		t.Errorf("icon after update = %q, want rocket (round-trips)", n2.Icon)
	}

	// bad logo type rejects the whole create (400 re-render, no node created)
	var buf3 bytes.Buffer
	mw3 := multipart.NewWriter(&buf3)
	_ = mw3.WriteField("name", "BadLogo")
	_ = mw3.WriteField("kind", "engagement")
	fw3, _ := mw3.CreateFormFile("logo", "evil.svg")
	_, _ = fw3.Write([]byte("<svg onload=alert(1)></svg>"))
	_ = mw3.Close()
	res3 := postNMultipart(t, ts, c, "/nodes", mw3.FormDataContentType(), &buf3)
	if res3.StatusCode != http.StatusBadRequest {
		t.Errorf("svg upload → %d, want 400", res3.StatusCode)
	}
	body3, _ := io.ReadAll(res3.Body)
	_ = res3.Body.Close()
	if !strings.Contains(string(body3), "Logo muss PNG, JPEG oder WebP sein") {
		t.Errorf("re-rendered form must show the node.err.logoType i18n message; body=%.500s", body3)
	}
	nodes, _ := ns.List(context.Background(), "u1")
	for _, nn := range nodes {
		if nn.Name == "BadLogo" {
			t.Errorf("rejected upload must not create a half-configured node: %+v", nn)
		}
	}
}

// TestWebNodeForm_EditShowsIconAndLogo pins the edit-GET rendering: the icon
// radio group is always present (name="icon"), and once a node has a
// LogoRef, the edit form also offers a logoRemove checkbox.
func TestWebNodeForm_EditShowsIconAndLogo(t *testing.T) {
	ts, c, _ := newWebNodesServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Iconic2")
	_ = mw.WriteField("kind", "engagement")
	_ = mw.WriteField("status", "active")
	_ = mw.WriteField("icon", "rocket")
	fw, _ := mw.CreateFormFile("logo", "logo.png")
	_, _ = fw.Write(pngPixel(t))
	_ = mw.Close()
	res := postNMultipart(t, ts, c, "/nodes", mw.FormDataContentType(), &buf)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create → %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	nodeID := strings.TrimPrefix(loc, "/nodes/")

	code, body := getN(t, ts, c, "/nodes/"+nodeID+"/edit")
	if code != http.StatusOK {
		t.Fatalf("edit GET = %d", code)
	}
	if !strings.Contains(body, `name="icon"`) {
		t.Errorf("edit form must contain the icon radio group; body=%.500s", body)
	}
	if !strings.Contains(body, `value="rocket" checked`) {
		t.Errorf("edit form must pre-select the node's current icon (rocket); body=%.500s", body)
	}
	if !strings.Contains(body, `name="logoRemove"`) {
		t.Errorf("edit form with LogoRef set must offer logoRemove checkbox; body=%.500s", body)
	}
}

// TestWebNodeForm_LogoSizeLimit pins fix 1 (whole-branch review): the whole
// multipart body is bounded via http.MaxBytesReader in the handler, so an
// oversized logo upload fails fast with 400 + the i18n node.err.logoSize
// message instead of buffering an unbounded body — and no half-configured
// node is created.
//
// name/kind ride the URL query string rather than the multipart body: Go's
// mime/multipart.Reader.ReadForm discards ALL parsed fields (not just the
// oversized file) the instant the underlying reader errors, so a name field
// inside the (doomed) body would come back empty too and trip the unrelated
// "name required" check first. Query values are parsed separately (before
// the multipart body is even touched) and survive, isolating the assertion
// to the logo-size path.
func TestWebNodeForm_LogoSizeLimit(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("logo", "big.png")
	_, _ = fw.Write(make([]byte, usecase.MaxNodeLogoBytes+128*1024)) // well past MaxBytesReader's cap
	_ = mw.Close()

	res := postNMultipart(t, ts, c, "/nodes?name=TooBig&kind=engagement", mw.FormDataContentType(), &buf)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized upload → %d, want 400", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "Logo zu groß") {
		t.Errorf("re-rendered form must show the node.err.logoSize i18n message; body=%.500s", body)
	}
	nodes, _ := ns.List(context.Background(), "u1")
	for _, n := range nodes {
		if n.Name == "TooBig" {
			t.Errorf("oversized upload must not create a half-configured node: %+v", n)
		}
	}
}

// TestWebNodeStatus_PreservesIcon pins that handleWebNodeStatus's full-replace
// UpdateNode call round-trips the node's Icon field (not just Color/Glyph).
func TestWebNodeStatus_PreservesIcon(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	n := seedTreeNode(t, ns, "eng1", "Iconic", domain.KindEngagement, nil)
	n.Icon = "rocket"
	_, _ = ns.Update(context.Background(), "u1", n)

	res := postN(t, ts, c, "/nodes/"+n.ID+"/status", url.Values{"status": {"paused"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", res.StatusCode)
	}
	_ = res.Body.Close()

	got, err := ns.Get(context.Background(), "u1", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.NodePaused {
		t.Errorf("status = %q, want paused", got.Status)
	}
	if got.Icon != "rocket" {
		t.Errorf("icon = %q after status change, want rocket (preserved)", got.Icon)
	}
}

// TestWebNodeForm_EmptyLogoFilePinsNoUpload pins that a "logo" multipart part
// with an empty filename (browser-style "no file chosen" submission) is
// treated as no upload, not an error: the node is created with LogoRef empty.
func TestWebNodeForm_EmptyLogoFilePinsNoUpload(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "NoLogo")
	_ = mw.WriteField("kind", "engagement")
	_, _ = mw.CreateFormFile("logo", "") // empty filename + zero bytes written
	_ = mw.Close()

	res := postNMultipart(t, ts, c, "/nodes", mw.FormDataContentType(), &buf)
	if res.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("create with empty logo part → %d, want 303; body=%.500s", res.StatusCode, body)
	}
	loc := res.Header.Get("Location")
	_ = res.Body.Close()
	nodeID := strings.TrimPrefix(loc, "/nodes/")

	n, err := ns.Get(context.Background(), "u1", nodeID)
	if err != nil {
		t.Fatalf("created node not found: %v", err)
	}
	if n.LogoRef != "" {
		t.Errorf("LogoRef = %q, want empty (empty file part must not count as an upload)", n.LogoRef)
	}
}

// TestWebNodesPage_OwnerScope_NoCrossTenantLeak pins the owner-scope
// guarantee on the redesigned /nodes page (Lesesaal L2 Task 3): two owners
// share the same FakeNodeStore, and owner A's Projekte page must never
// contain owner B's engagement (or vice versa). flow is multi-tenant
// throughout — "it's just one user" is never a valid justification (AGENTS.md
// Grundsätze) — so a cross-tenant leak here is a Critical finding.
func TestWebNodesPage_OwnerScope_NoCrossTenantLeak(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	ns := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	uA, _ := domain.NewUser("uA", "sub-a", "alice", "a@x.de", "Alice")
	uB, _ := domain.NewUser("uB", "sub-b", "bob", "b@x.de", "Bob")
	_, _ = users.UpsertBySub(context.Background(), uA)
	_, _ = users.UpsertBySub(context.Background(), uB)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Users:     users,
		Session:   codec,
		Bus:       bus,
		Emitter:   sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:     clk,
		ListNodes: usecase.ListNodes{Nodes: ns},
		GetNode:   usecase.GetNode{Nodes: ns},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cvA, _ := codec.Issue("uA")
	cvB, _ := codec.Issue("uB")
	cA := &http.Cookie{Name: "flow_session", Value: cvA}
	cB := &http.Cookie{Name: "flow_session", Value: cvB}

	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)
	nA, _ := domain.NewNode("engA", "uA", "Alice Only Engagement", "alice-only", now)
	nA.Kind, nA.Status = domain.KindEngagement, domain.NodeActive
	if _, err := ns.Create(context.Background(), nA); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	nB, _ := domain.NewNode("engB", "uB", "Bob Only Engagement", "bob-only", now)
	nB.Kind, nB.Status = domain.KindEngagement, domain.NodeActive
	if _, err := ns.Create(context.Background(), nB); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	_, bodyA := getN(t, ts, cA, "/nodes")
	if !strings.Contains(bodyA, "Alice Only Engagement") {
		t.Fatalf("A's own engagement missing from A's page; body=%.500s", bodyA)
	}
	if strings.Contains(bodyA, "Bob Only Engagement") {
		t.Fatalf("CROSS-TENANT LEAK: B's engagement visible on A's /nodes page; body=%.500s", bodyA)
	}

	_, bodyB := getN(t, ts, cB, "/nodes")
	if !strings.Contains(bodyB, "Bob Only Engagement") {
		t.Fatalf("B's own engagement missing from B's page; body=%.500s", bodyB)
	}
	if strings.Contains(bodyB, "Alice Only Engagement") {
		t.Fatalf("CROSS-TENANT LEAK: A's engagement visible on B's /nodes page; body=%.500s", bodyB)
	}

	// Same guarantee on the SSE fragment route.
	_, fragA := getN(t, ts, cA, "/ui/nodes/list")
	if strings.Contains(fragA, "Bob Only Engagement") {
		t.Fatalf("CROSS-TENANT LEAK: B's engagement visible on A's /ui/nodes/list fragment; body=%.500s", fragA)
	}
}
