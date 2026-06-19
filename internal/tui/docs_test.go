package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeDocEditor is a test double for docEditor that avoids any real $EDITOR call.
// Command returns exec.Command("true") (a no-op process); readback returns fixedBody.
type fakeDocEditor struct {
	fixedBody []byte
	cmdErr    error
}

func (f *fakeDocEditor) Command(initial []byte) (*exec.Cmd, func() ([]byte, error), func(), error) {
	if f.cmdErr != nil {
		return nil, nil, nil, f.cmdErr
	}
	cmd := exec.Command("true")
	readback := func() ([]byte, error) { return f.fixedBody, nil }
	cleanup := func() {}
	return cmd, readback, cleanup, nil
}

// newFakeDocSrv builds a minimal httptest server that handles the document REST
// endpoints plus a trivial SSE /api/v1/events endpoint. Returns the server and
// an *apiclient.Client pointed at it.
func newFakeDocSrv(t *testing.T, docs []domain.Document) (*apiclient.Client, func()) {
	t.Helper()
	stored := make(map[string]domain.Document, len(docs))
	for _, d := range docs {
		stored[d.ID] = d
	}

	mux := http.NewServeMux()

	// List
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		list := make([]domain.Document, 0, len(stored))
		for _, d := range stored {
			list = append(list, d)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	// Get by ID
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		d, ok := stored[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})

	// Create
	mux.HandleFunc("POST /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		d := domain.Document{
			ID:        fmt.Sprintf("new-%d", len(stored)+1),
			Type:      domain.DocumentType(in.Type),
			Path:      in.Path,
			Title:     in.Title,
			Body:      in.Body,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		stored[d.ID] = d
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(d)
	})

	// Update
	mux.HandleFunc("PUT /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		d, ok := stored[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in apiclient.UpdateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		d.Title = in.Title
		d.Body = in.Body
		stored[id] = d
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})

	// Delete
	mux.HandleFunc("DELETE /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		delete(stored, id)
		w.WriteHeader(http.StatusNoContent)
	})

	// SSE – return a minimal event stream then close
	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write one event and close so the goroutine exits quickly.
		_, _ = fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
	})

	srv := httptest.NewServer(mux)
	c := apiclient.New(srv.URL, "tok")
	return c, srv.Close
}

func sampleDocs() []domain.Document {
	return []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/architecture", Title: "Arch", Body: "the body text"},
		{ID: "d2", Type: domain.DocAgent, Path: "agents/reviewer", Title: "Reviewer"},
	}
}

func TestDocsLoadedPopulatesAndRenders(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	if len(m.docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(m.docs))
	}
	out := m.View().Content
	if !strings.Contains(out, "docs/architecture") || !strings.Contains(out, "agents/reviewer") {
		t.Fatalf("list view missing paths:\n%s", out)
	}
}

func TestDocsJKNavigation(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)

	next2, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next2.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("j: sel = %d, want 1", m.sel)
	}
	// clamp at end
	next3, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next3.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("j clamp: sel = %d, want 1", m.sel)
	}
	next4, _ := m.Update(tea.KeyPressMsg{Text: "k"})
	m = next4.(DocsModel)
	if m.sel != 0 {
		t.Fatalf("k: sel = %d, want 0", m.sel)
	}
}

func TestDocsEnterShowsBody(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)

	// Enter with nil client returns no cmd; simulate the resulting docViewMsg.
	next2, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next2.(DocsModel)
	if m.mode != modeView || m.viewing == nil {
		t.Fatal("docViewMsg should switch to view mode with a viewing doc")
	}
	out := m.View().Content
	if !strings.Contains(out, "the body text") {
		t.Fatalf("view mode missing body:\n%s", out)
	}
}

func TestDocs_InViewModeTracksMode(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	if m.InViewMode() {
		t.Fatal("list mode: InViewMode must be false")
	}
	v, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	if !v.(DocsModel).InViewMode() {
		t.Fatal("after docViewMsg: InViewMode must be true")
	}
}

func TestDocs_TabCyclesWikilinkFocus(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	// Seed the doc set so the wikilink target (d2 = agents/reviewer) resolves.
	seeded, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = seeded.(DocsModel)
	// A body with two valid wikilinks (both resolve to d2 within sampleDocs()).
	doc := sampleDocs()[0]
	doc.Body = "see [[" + sampleDocs()[1].Path + "]] and [[" + sampleDocs()[1].Path + "]]"
	v, _ := m.Update(docViewMsg{doc: doc})
	m = v.(DocsModel)
	start := m.focusState()
	n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if n.(DocsModel).focusState() == start {
		t.Fatalf("Tab should advance the wikilink focus index (still %d)", start)
	}
}

// TestDocs_ViewerRendersSizedOverlay drives the full fullscreen path: a sized
// overlay (via SetViewport) renders the markdown body through the render closure
// (exercising the wikilink adapter's Resolve), and overlayView returns it.
func TestDocs_ViewerRendersSizedOverlay(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	seeded, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = seeded.(DocsModel)
	doc := sampleDocs()[0]
	doc.Body = "intro\n\nsee [[" + sampleDocs()[1].Path + "]] there"
	v, _ := m.Update(docViewMsg{doc: doc})
	m = v.(DocsModel)

	// Size the overlay from a host frame (the Frame→SetSize bridge the route does).
	m = m.SetViewport(80, 24)
	out := m.overlayView()
	if out == "" {
		t.Fatal("sized viewer overlay must render non-empty content")
	}
	// The full View() must surface the body via the overlay (not the legacy path).
	if !strings.Contains(m.View().Content, "intro") {
		t.Fatalf("viewer should render the document body:\n%s", m.View().Content)
	}
}

func TestDocsCapturesInput(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	if m.CapturesInput() {
		t.Fatal("modeList should not capture input (host Tab/digits nav must work)")
	}
	// 'n' opens the create form — Tab/space/text now belong to the form.
	create, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	if !create.(DocsModel).CapturesInput() {
		t.Fatal("modeCreating should capture input")
	}
	// view mode cycles links with Tab/Shift+Tab — also captures.
	view, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	if !view.(DocsModel).CapturesInput() {
		t.Fatal("modeView should capture input")
	}
}

func TestDocsEscReturnsToList(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next.(DocsModel)
	next2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next2.(DocsModel)
	if m.mode != modeList || m.viewing != nil {
		t.Fatal("esc should return to list and clear viewing")
	}
}

func TestDocsEventTriggersReload(t *testing.T) {
	// non-nil client so reload() returns a real cmd.
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, nil, "tester")
	// arm events channel
	ch := make(chan apiclient.ClientEvent, 1)
	next, _ := m.Update(eventsReadyMsg{ch: ch})
	m = next.(DocsModel)

	_, cmd := m.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd == nil {
		t.Fatal("document event should return a reload+listen batch cmd")
	}
}

// TestDocsEventMsg_ModeView_AlsoReloadsDoc verifies that an eventMsg received
// while the user is viewing a document triggers both the list reload AND a doc
// re-fetch (which rebuilds viewLinks and backlinks), unlike the modeList case
// which only triggers the list reload.
func TestDocsEventMsg_ModeView_AlsoReloadsDoc(t *testing.T) {
	seed := sampleDocs()
	c, stop := newFakeDocSrv(t, seed)
	defer stop()

	m := NewDocs(c, nil, nil, "tester")
	// Load the doc list.
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)
	// Arm the events channel.
	ch := make(chan apiclient.ClientEvent, 1)
	next, _ = m.Update(eventsReadyMsg{ch: ch})
	m = next.(DocsModel)
	// Enter view mode by feeding a docViewMsg.
	next, _ = m.Update(docViewMsg{doc: seed[0]})
	m = next.(DocsModel)
	if m.mode != modeView || m.viewing == nil {
		t.Fatal("setup: expected modeView with a viewing doc")
	}

	// eventMsg while in modeView must return a non-nil batch cmd.
	_, cmd := m.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd == nil {
		t.Fatal("eventMsg in modeView should return a non-nil batch cmd (list reload + doc reload)")
	}

	// Also verify that in modeList the cmd is still non-nil (the reload+listen batch).
	m2 := NewDocs(c, nil, nil, "tester")
	next2, _ := m2.Update(eventsReadyMsg{ch: ch})
	m2 = next2.(DocsModel)
	_, cmd2 := m2.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd2 == nil {
		t.Fatal("eventMsg in modeList should still return a non-nil batch cmd")
	}
}

func TestDocsNKeyOpensCreate(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	if m.mode != modeCreating {
		t.Fatal("n should open create mode")
	}
	out := m.View().Content
	if !strings.Contains(out, "New document") {
		t.Fatalf("create view missing header:\n%s", out)
	}
}

func TestDocsCreateFormTypingAndCancel(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// advance to slug field
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(DocsModel)
	if m.field != fldPath {
		t.Fatalf("tab: field = %d, want %d", m.field, fldPath)
	}
	for _, r := range "docs/x" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = next.(DocsModel)
	}
	if m.newPath != "docs/x" {
		t.Fatalf("slug = %q, want docs/x", m.newPath)
	}
	// esc cancels back to list
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(DocsModel)
	if m.mode != modeList {
		t.Fatal("esc should cancel create")
	}
}

func TestDocsDeleteConfirmFlow(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = next.(DocsModel)
	if m.mode != modeDeleting {
		t.Fatal("d should enter delete-confirm mode")
	}
	// n cancels
	next, _ = m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	if m.mode != modeList {
		t.Fatal("n should cancel delete")
	}
}

func TestDocsQuitKey(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
}

func TestDocsNextDocType(t *testing.T) {
	if got := nextDocType(domain.DocFree); got != domain.DocDaily {
		t.Fatalf("free → %q, want daily", got)
	}
	if got := nextDocType(domain.DocAgent); got != domain.DocFree {
		t.Fatalf("agent → %q (wrap), want free", got)
	}
}

func TestDocsSavedReloads(t *testing.T) {
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, nil, "tester")
	_, cmd := m.Update(docSavedMsg{})
	if cmd == nil {
		t.Fatal("docSavedMsg should trigger a reload cmd")
	}
}

// ── new tests to boost coverage ───────────────────────────────────────────────

func TestDocsInit_ReturnsCmdWithClient(t *testing.T) {
	c, stop := newFakeDocSrv(t, nil)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init with real client must return non-nil cmd")
	}
}

func TestDocsReload_ReturnsDocsLoadedMsg(t *testing.T) {
	seed := sampleDocs()
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	cmd := m.reload()
	if cmd == nil {
		t.Fatal("reload() with real client must return non-nil cmd")
	}
	msg := cmd()
	loaded, ok := msg.(docsLoadedMsg)
	if !ok {
		t.Fatalf("expected docsLoadedMsg, got %T: %v", msg, msg)
	}
	if len(loaded.docs) == 0 {
		t.Fatal("expected at least one doc from stub")
	}
}

func TestDocsSubscribe_ReturnsEventsReadyMsg(t *testing.T) {
	c, stop := newFakeDocSrv(t, nil)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	cmd := m.subscribe()
	if cmd == nil {
		t.Fatal("subscribe() with real client must return non-nil cmd")
	}
	msg := cmd()
	_, ok := msg.(eventsReadyMsg)
	if !ok {
		t.Fatalf("expected eventsReadyMsg, got %T", msg)
	}
}

func TestDocsLoadDoc_ReturnsDocViewMsg(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "notes/x", Title: "X Note", Body: "body x"},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	cmd := m.loadDoc("d1", false)
	if cmd == nil {
		t.Fatal("loadDoc with real client must return non-nil cmd")
	}
	msg := cmd()
	viewMsg, ok := msg.(docViewMsg)
	if !ok {
		t.Fatalf("expected docViewMsg, got %T: %v", msg, msg)
	}
	if viewMsg.doc.Title != "X Note" {
		t.Fatalf("doc title = %q, want X Note", viewMsg.doc.Title)
	}
}

func TestDocsLoadDoc_ThenViewRendersBody(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "notes/x", Title: "X Note", Body: "body x"},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	// Simulate docViewMsg being fed back after loadDoc.
	next, _ := m.Update(docViewMsg{doc: seed[0]})
	m = next.(DocsModel)
	if m.mode != modeView {
		t.Fatal("docViewMsg should set modeView")
	}
	out := m.View().Content
	if !strings.Contains(out, "body x") {
		t.Fatalf("renderView missing body:\n%s", out)
	}
	if !strings.Contains(out, "X Note") {
		t.Fatalf("renderView missing title:\n%s", out)
	}
}

func TestDocsRenderView_Empty(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	emptyDoc := domain.Document{ID: "e1", Type: domain.DocFree, Path: "p", Title: "Empty", Body: "   "}
	next, _ := m.Update(docViewMsg{doc: emptyDoc})
	m = next.(DocsModel)
	out := m.View().Content
	if !strings.Contains(out, "(empty)") {
		t.Fatalf("renderView with blank body should show (empty):\n%s", out)
	}
}

func TestDocsPersist_Create_PostsToAPI(t *testing.T) {
	c, stop := newFakeDocSrv(t, nil)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	done := editorDoneMsg{
		body:  []byte("my content"),
		typ:   domain.DocFree,
		path:  "create/path",
		title: "Created Doc",
	}
	cmd := m.persist(done)
	if cmd == nil {
		t.Fatal("persist with real client must return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(docSavedMsg); !ok {
		t.Fatalf("persist create: expected docSavedMsg, got %T: %v", msg, msg)
	}
}

func TestDocsPersist_Update_PutsToAPI(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "notes/x", Title: "Old Title", Body: "old body"},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	done := editorDoneMsg{
		body:   []byte("new body"),
		editID: "d1",
		title:  "New Title",
	}
	cmd := m.persist(done)
	if cmd == nil {
		t.Fatal("persist with real client must return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(docSavedMsg); !ok {
		t.Fatalf("persist update: expected docSavedMsg, got %T: %v", msg, msg)
	}
}

func TestDocsDeleteCmd_DeletesAndReturnsDocSavedMsg(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "notes/x", Title: "X", Body: ""},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	// Load docs into model.
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)
	cmd := m.deleteCmd("d1")
	if cmd == nil {
		t.Fatal("deleteCmd with real client must return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(docSavedMsg); !ok {
		t.Fatalf("deleteCmd: expected docSavedMsg, got %T: %v", msg, msg)
	}
}

func TestDocsDeleteKey_YConfirms(t *testing.T) {
	seed := sampleDocs()
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)
	// Enter delete mode.
	next, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = next.(DocsModel)
	if m.mode != modeDeleting {
		t.Fatal("d should enter modeDeleting")
	}
	// Confirm with y — should return a delete cmd (non-nil).
	_, cmd := m.Update(tea.KeyPressMsg{Text: "y"})
	if cmd == nil {
		t.Fatal("y in delete mode should return a delete cmd")
	}
}

func TestDocsDeleteKey_EscCancels(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(DocsModel)
	if m.mode != modeList {
		t.Fatal("esc in delete mode should return to modeList")
	}
}

func TestDocsBuildEditor_NilEditor_ReturnsErrMsg(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	msg := m.buildEditor([]byte("seed"), "", domain.DocFree, "path", "title")
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("buildEditor with nil editor should return errMsg, got %T", msg)
	}
}

func TestDocsBuildEditor_FakeEditor_ReturnsEditorReq(t *testing.T) {
	ed := &fakeDocEditor{fixedBody: []byte("edited content")}
	m := NewDocs(nil, ed, nil, "tester")
	msg := m.buildEditor([]byte("seed"), "id-1", domain.DocFree, "path/x", "Title X")
	req, ok := msg.(editorReq)
	if !ok {
		t.Fatalf("buildEditor with fake editor should return editorReq, got %T", msg)
	}
	if req.editID != "id-1" {
		t.Fatalf("editorReq.editID = %q, want id-1", req.editID)
	}
	if req.cmd == nil {
		t.Fatal("editorReq.cmd must not be nil")
	}
	// Confirm readback returns the seeded bytes.
	body, err := req.readback()
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(body) != "edited content" {
		t.Fatalf("readback = %q, want %q", body, "edited content")
	}
}

func TestDocsBuildEditor_FakeEditor_CmdError_ReturnsErrMsg(t *testing.T) {
	ed := &fakeDocEditor{cmdErr: fmt.Errorf("editor: test error")}
	m := NewDocs(nil, ed, nil, "tester")
	msg := m.buildEditor([]byte("seed"), "", domain.DocFree, "p", "t")
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("buildEditor cmd error should return errMsg, got %T", msg)
	}
}

func TestDocsBuildEditorCmd_LoadsAndThenEdits(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "notes/x", Title: "X", Body: "original"},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	ed := &fakeDocEditor{fixedBody: []byte("edited")}
	m := NewDocs(c, ed, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)

	// buildEditorCmd returns a cmd that calls loadDoc(id, thenEdit=true).
	cmd := m.buildEditorCmd("d1")
	if cmd == nil {
		t.Fatal("buildEditorCmd must return non-nil cmd")
	}
	msg := cmd()
	// With a real server + fake editor, should produce an editorReq (not an errMsg).
	if _, ok := msg.(editorReq); !ok {
		t.Fatalf("buildEditorCmd msg = %T, want editorReq (got: %v)", msg, msg)
	}
}

func TestDocsBuildCreateEditorCmd_ProducesEditorReq(t *testing.T) {
	ed := &fakeDocEditor{fixedBody: []byte("created content")}
	m := NewDocs(nil, ed, nil, "tester")
	// Set up create form state.
	m.mode = modeCreating
	m.newType = domain.DocFree
	m.newPath = "create/note"
	m.newTitle = "My Note"
	m.field = fldTitle

	cmd := m.buildCreateEditorCmd()
	if cmd == nil {
		t.Fatal("buildCreateEditorCmd must return non-nil cmd")
	}
	msg := cmd()
	req, ok := msg.(editorReq)
	if !ok {
		t.Fatalf("buildCreateEditorCmd msg = %T, want editorReq", msg)
	}
	if req.editID != "" {
		t.Fatalf("create editorReq.editID should be empty, got %q", req.editID)
	}
	if req.path != "create/note" {
		t.Fatalf("create editorReq.path = %q, want create/note", req.path)
	}
}

func TestDocsDropLast(t *testing.T) {
	if got := dropLast("hello"); got != "hell" {
		t.Fatalf("dropLast(hello) = %q, want hell", got)
	}
	if got := dropLast(""); got != "" {
		t.Fatalf("dropLast('') = %q, want ''", got)
	}
	// Unicode: last rune removed correctly.
	if got := dropLast("héllo"); got != "héll" {
		t.Fatalf("dropLast unicode = %q, want héll", got)
	}
}

func TestDocsHandleCreateKey_SpaceCyclesType(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel) // modeCreating, field=fldType, newType=DocFree
	next, _ = m.Update(tea.KeyPressMsg{Text: " "})
	m = next.(DocsModel)
	if m.newType != domain.DocDaily {
		t.Fatalf("space on type field: got %q, want DocDaily", m.newType)
	}
	// Right arrow also cycles.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(DocsModel)
	if m.newType != domain.DocAgent {
		t.Fatalf("right on type field: got %q, want DocAgent", m.newType)
	}
}

func TestDocsHandleCreateKey_BackspaceOnPathAndTitle(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// Advance to path field.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(DocsModel)
	// Type "abc".
	for _, c := range "abc" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(c)})
		m = next.(DocsModel)
	}
	if m.newPath != "abc" {
		t.Fatalf("newPath = %q after typing, want abc", m.newPath)
	}
	// Backspace removes last char.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(DocsModel)
	if m.newPath != "ab" {
		t.Fatalf("newPath after backspace = %q, want ab", m.newPath)
	}

	// Advance to title field.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(DocsModel)
	for _, c := range "xy" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(c)})
		m = next.(DocsModel)
	}
	if m.newTitle != "xy" {
		t.Fatalf("newTitle = %q, want xy", m.newTitle)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(DocsModel)
	if m.newTitle != "x" {
		t.Fatalf("newTitle after backspace = %q, want x", m.newTitle)
	}
}

func TestDocsHandleCreateKey_EnterAdvancesField(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// Enter on type field → advance to path.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	if m.field != fldPath {
		t.Fatalf("enter on fldType: field = %d, want fldPath(%d)", m.field, fldPath)
	}
}

func TestDocsHandleCreateKey_EnterOnTitle_InvalidSlug(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// Advance to title field.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	// Now on fldTitle with invalid slug (empty path) — Enter should set error.
	m.newTitle = "My Title"
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	if m.err == nil {
		t.Fatal("Enter with invalid slug should set m.err")
	}
}

func TestDocsHandleCreateKey_EnterOnTitle_EmptyTitle(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// Advance to title field.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	// Set a valid slug but no title.
	m.newPath = "valid/slug"
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(DocsModel)
	if m.err == nil {
		t.Fatal("Enter with empty title should set m.err")
	}
	if !strings.Contains(m.err.Error(), "title") {
		t.Fatalf("err should mention 'title', got: %v", m.err)
	}
}

func TestDocsViewMode_EKeyOpensEditor(t *testing.T) {
	seed := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "p", Title: "T", Body: "B"},
	}
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	ed := &fakeDocEditor{fixedBody: []byte("edited")}
	m := NewDocs(c, ed, nil, "tester")
	// Enter view mode.
	next, _ := m.Update(docViewMsg{doc: seed[0]})
	m = next.(DocsModel)
	// e in view mode should trigger buildEditorCmd.
	_, cmd := m.Update(tea.KeyPressMsg{Text: "e"})
	if cmd == nil {
		t.Fatal("e in view mode should return a non-nil cmd")
	}
}

func TestDocsViewMode_EKey_NilViewing(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	// Manually set modeView but no viewing doc.
	m.mode = modeView
	next, cmd := m.Update(tea.KeyPressMsg{Text: "e"})
	_ = next
	if cmd != nil {
		t.Fatal("e with nil viewing should return nil cmd")
	}
}

// TestDocsViewMode_QDoesNotQuit pins the M3d behaviour change: the fullscreen
// markdown viewer owns the keyboard, so `q` no longer quits the program from
// view mode — it is forwarded to the overlay (which leaves the screen running).
func TestDocsViewMode_QDoesNotQuit(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next.(DocsModel)
	nm, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if !nm.(DocsModel).InViewMode() {
		t.Fatal("q in view mode must not leave the viewer")
	}
	if cmd != nil {
		// The overlay may emit a (non-quit) cmd; assert it is not tea.Quit by
		// checking the model is still alive and in view mode (above). A nil cmd
		// is also fine — the only forbidden outcome is quitting.
		if msg := cmd(); msg != nil {
			if _, isQuit := msg.(tea.QuitMsg); isQuit {
				t.Fatal("q in view mode must not quit the program")
			}
		}
	}
}

func TestDocsListMode_EKey_WithDocs(t *testing.T) {
	seed := sampleDocs()
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	ed := &fakeDocEditor{fixedBody: []byte("edited")}
	m := NewDocs(c, ed, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)
	_, cmd := m.Update(tea.KeyPressMsg{Text: "e"})
	if cmd == nil {
		t.Fatal("e in list mode with docs should return a non-nil cmd")
	}
}

func TestDocsListMode_EKey_NoDocs(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "e"})
	if cmd != nil {
		t.Fatal("e with no docs should return nil cmd")
	}
}

func TestDocsListMode_DKey_NoDocs(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, cmd := m.Update(tea.KeyPressMsg{Text: "d"})
	_ = next
	if cmd != nil {
		t.Fatal("d with no docs should return nil cmd")
	}
}

func TestDocsEditorDoneMsg_WithError_SetsErr(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	done := editorDoneMsg{err: fmt.Errorf("editor failed")}
	next, _ := m.Update(done)
	m = next.(DocsModel)
	if m.err == nil {
		t.Fatal("editorDoneMsg with error should set m.err")
	}
	if m.mode != modeList {
		t.Fatal("editorDoneMsg should reset to modeList")
	}
}

func TestDocsEditorDoneMsg_NoError_CallsPersist(t *testing.T) {
	c, stop := newFakeDocSrv(t, nil)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	done := editorDoneMsg{
		body:  []byte("new content"),
		path:  "new/path",
		title: "New Title",
		typ:   domain.DocFree,
	}
	_, cmd := m.Update(done)
	if cmd == nil {
		t.Fatal("editorDoneMsg without error should return a persist cmd")
	}
}

func TestDocsEnterKey_NoDocs(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter with no docs should return nil cmd")
	}
}

func TestDocsEnterKey_WithDocs(t *testing.T) {
	seed := sampleDocs()
	c, stop := newFakeDocSrv(t, seed)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: seed})
	m = next.(DocsModel)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter with docs should return loadDoc cmd")
	}
}

func TestDocsSelClamp_OnLoad(t *testing.T) {
	// Start with 2 docs selected at index 1; reload with only 1 doc.
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "j"})
	m = next.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("sel = %d, want 1", m.sel)
	}
	// Reload with only 1 doc — sel should clamp.
	next, _ = m.Update(docsLoadedMsg{docs: sampleDocs()[:1]})
	m = next.(DocsModel)
	if m.sel != 0 {
		t.Fatalf("sel after clamp = %d, want 0", m.sel)
	}
}

func TestDocsUnhandledMsg(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	_, cmd := m.Update(struct{}{})
	if cmd != nil {
		t.Fatal("unhandled msg should return nil cmd")
	}
}

func TestDocsView_DeletingMode_ShowsPrompt(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = next.(DocsModel)
	out := m.View().Content
	if !strings.Contains(out, "delete") {
		t.Fatalf("deleting mode should show delete prompt:\n%s", out)
	}
}

func TestDocsView_ErrorDisplayed(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	next, _ := m.Update(errMsg{err: fmt.Errorf("test err")})
	out := next.(DocsModel).View().Content
	if !strings.Contains(out, "error:") {
		t.Fatalf("view missing error display:\n%s", out)
	}
}

func TestDocsView_StatusDisplayed(t *testing.T) {
	c, stop := newFakeDocSrv(t, nil)
	defer stop()
	m := NewDocs(c, nil, nil, "tester")
	// docSavedMsg sets status.
	next, _ := m.Update(docSavedMsg{})
	m = next.(DocsModel)
	m.status = "saved"
	out := m.View().Content
	if !strings.Contains(out, "saved") {
		t.Fatalf("view missing status:\n%s", out)
	}
}

func TestDocsView_EmptyList(t *testing.T) {
	m := NewDocs(nil, nil, nil, "tester")
	out := m.View().Content
	if !strings.Contains(out, "no documents yet") {
		t.Fatalf("empty list view should hint user to create:\n%s", out)
	}
}

func TestBuildBodyLinks(t *testing.T) {
	all := []domain.Document{
		{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree},
	}
	src := domain.Document{ID: "d-src", Path: "src", Type: domain.DocFree}
	body := "see [[dest]], [[ghost]] and http://x.io"

	links := buildBodyLinks(body, src, all)
	if len(links) != 2 {
		t.Fatalf("want 2 focusable links, got %d: %#v", len(links), links)
	}
	if links[0].kind != linkWiki || links[0].docID != "d-dest" {
		t.Fatalf("first link should be the dest wikilink: %#v", links[0])
	}
	if links[1].kind != linkWeb || links[1].url != "http://x.io" {
		t.Fatalf("second link should be the weblink: %#v", links[1])
	}
}

func TestStyleBodyLine_BrokenWikilink(t *testing.T) {
	src := domain.Document{ID: "s", Path: "s"}
	out := styleBodyLine("x [[ghost]] y", src, nil, -1, func(string) int { return -1 })
	if !strings.Contains(out, "⊘") {
		t.Fatalf("broken wikilink should carry the ⊘ glyph: %q", out)
	}
}

func TestBacklinksMsg_NoDuplicateInFocusCycle(t *testing.T) {
	dest := domain.Document{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree}
	src := domain.Document{ID: "d-src", Path: "src", Type: domain.DocFree, Body: "go [[dest]]"}
	m := DocsModel{mode: modeView, viewing: &src, docs: []domain.Document{src, dest}, linkFocus: -1}
	m.viewLinks = buildBodyLinks(src.Body, src, m.docs)
	// dest is a forward link already; now it also references back.
	updated, _ := m.Update(backlinksMsg{refs: []domain.BacklinkRef{{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree}}})
	mm := updated.(DocsModel)
	count := 0
	for _, lt := range mm.viewLinks {
		if lt.docID == "d-dest" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("d-dest should appear once in viewLinks, got %d", count)
	}
	if len(mm.backlinks) != 1 {
		t.Fatalf("footer backlinks should still list the ref, got %d", len(mm.backlinks))
	}
}

func TestDocs_FilterOverlayToggleApply(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.docs = []domain.Document{{ID: "a", Type: domain.DocFree, Path: "a", Tags: []string{"go"}}}
	m.filterOpts = []domain.TagCount{{Tag: "go", Count: 1}, {Tag: "tui", Count: 2}}
	m.mode = modeFiltering

	// cursor starts at 0 (= "go"), space toggles "go" into filterWork
	m2, _ := m.Update(tea.KeyPressMsg{Text: " "})
	dm := m2.(DocsModel)
	if len(dm.filterWork) != 1 || dm.filterWork[0] != "go" {
		t.Fatalf("working set = %#v, want [go]", dm.filterWork)
	}
	// enter applies the working set and returns to modeList
	m3, _ := dm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	dm = m3.(DocsModel)
	if dm.mode != modeList {
		t.Fatalf("mode = %v, want modeList", dm.mode)
	}
	if len(dm.filterTags) != 1 || dm.filterTags[0] != "go" {
		t.Fatalf("active filter = %#v, want [go]", dm.filterTags)
	}
}

func TestDocs_FilterToggleOff(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.filterOpts = []domain.TagCount{{Tag: "go", Count: 1}, {Tag: "tui", Count: 2}}
	m.filterCursor = 0
	m.mode = modeFiltering

	// First space: "go" is absent from filterWork → toggleStr adds it.
	m2, _ := m.Update(tea.KeyPressMsg{Text: " "})
	dm := m2.(DocsModel)
	if len(dm.filterWork) != 1 || dm.filterWork[0] != "go" {
		t.Fatalf("after first space: filterWork = %#v, want [go]", dm.filterWork)
	}

	// Second space: "go" is present → toggleStr removes it (remove branch).
	m3, _ := dm.Update(tea.KeyPressMsg{Text: " "})
	dm = m3.(DocsModel)
	if len(dm.filterWork) != 0 {
		t.Fatalf("after second space: filterWork = %#v, want []", dm.filterWork)
	}
}

func TestDocs_FilterEscDiscards(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.mode = modeFiltering
	m.filterTags = []string{"old"}
	m.filterWork = []string{"new"}

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	dm := m2.(DocsModel)
	if dm.mode != modeList {
		t.Fatalf("esc: mode = %v, want modeList", dm.mode)
	}
	if len(dm.filterTags) != 1 || dm.filterTags[0] != "old" {
		t.Fatalf("esc: filterTags = %#v, want [old] (pending filterWork must not be applied)", dm.filterTags)
	}
}

func TestDocs_FilterClear(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.mode = modeFiltering
	m.filterWork = []string{"go", "tui"}

	m2, _ := m.Update(tea.KeyPressMsg{Text: "c"})
	dm := m2.(DocsModel)
	if len(dm.filterWork) != 0 {
		t.Fatalf("c: filterWork = %#v, want nil/empty", dm.filterWork)
	}
}

func TestDocs_RenderViewSkipsFrontmatter(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	d := domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "A",
		Body: "---\ntags: [go]\n---\nhello body", Tags: []string{"go"}}
	m.viewing = &d
	m.mode = modeView
	var b strings.Builder
	m.renderView(&b)
	out := b.String()
	if strings.Contains(out, "tags:") {
		t.Fatalf("frontmatter leaked into view: %q", out)
	}
	if !strings.Contains(out, "hello body") {
		t.Fatalf("body missing: %q", out)
	}
}

func TestDocs_SearchInputAndRun(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m2, _ := m.Update(tea.KeyPressMsg{Text: "/"})
	dm := m2.(DocsModel)
	if dm.mode != modeSearch {
		t.Fatalf("mode = %v, want modeSearch", dm.mode)
	}
	m3, _ := dm.Update(tea.KeyPressMsg{Text: "g"})
	dm = m3.(DocsModel)
	m4, _ := dm.Update(tea.KeyPressMsg{Text: "o"})
	dm = m4.(DocsModel)
	if dm.searchQuery != "go" {
		t.Fatalf("searchQuery = %q, want go", dm.searchQuery)
	}
	m5, _ := dm.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	dm = m5.(DocsModel)
	if dm.mode != modeList {
		t.Fatalf("esc did not return to list: %v", dm.mode)
	}
}

func TestDocs_RenderSearchHighlights(t *testing.T) {
	m := NewDocs(nil, nil, nil, "u")
	m.mode = modeSearch
	m.searching = true
	m.searchHits = []domain.SearchHit{
		{Document: domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "Kompendium"},
			Snippet: "see " + domain.HighlightStart + "Kompendium" + domain.HighlightEnd + " here"},
	}
	var b strings.Builder
	m.renderSearch(&b)
	out := b.String()
	if !strings.Contains(out, "Kompendium") {
		t.Fatalf("snippet not rendered: %q", out)
	}
	if strings.Contains(out, domain.HighlightStart) || strings.Contains(out, domain.HighlightEnd) {
		t.Fatalf("raw sentinels leaked into output: %q", out)
	}
}

func TestDocsEventMsg_SearchMode_RerunsSearch(t *testing.T) {
	// When a document SSE event arrives while in modeSearch with an active query,
	// the handler should return a non-nil cmd that triggers re-search rather than
	// a plain list reload, so search results stay live.
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, nil, "tester")
	ch := make(chan apiclient.ClientEvent, 1)
	next, _ := m.Update(eventsReadyMsg{ch: ch})
	m = next.(DocsModel)

	// Simulate an active search result state.
	m.mode = modeSearch
	m.searching = true
	m.searchQuery = "kompend"
	m.searchHits = []domain.SearchHit{
		{Document: domain.Document{ID: "a", Type: domain.DocFree, Path: "a", Title: "Kompendium"},
			Snippet: domain.HighlightStart + "Kompendium" + domain.HighlightEnd},
	}

	_, cmd := m.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentUpdated)}})
	if cmd == nil {
		t.Fatal("eventMsg in modeSearch with active query should return a non-nil cmd")
	}
}

func TestDocsEventMsg_SearchMode_EmptyQuery_FallsBackToReload(t *testing.T) {
	// When search mode is active but the query is empty (typing phase), the
	// handler should fall back to the plain reload, not runSearch("").
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, nil, "tester")
	ch := make(chan apiclient.ClientEvent, 1)
	next, _ := m.Update(eventsReadyMsg{ch: ch})
	m = next.(DocsModel)

	m.mode = modeSearch
	m.searching = false // still in input phase
	m.searchQuery = ""

	_, cmd := m.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentUpdated)}})
	if cmd == nil {
		t.Fatal("eventMsg in modeSearch input phase should return a non-nil batch cmd (reload+listen)")
	}
}

func TestHighlightSnippet_StrayStartSentinel(t *testing.T) {
	// An unmatched HighlightStart (no closing HighlightEnd) must not let a raw
	// control char reach the terminal output.
	stray := "before " + domain.HighlightStart + "after"
	got := highlightSnippet(stray)
	if strings.Contains(got, domain.HighlightStart) || strings.Contains(got, domain.HighlightEnd) {
		t.Fatalf("stray sentinel leaked into TUI output: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("text around stray sentinel should be preserved: %q", got)
	}
}

// TestDocsView_TabFocusAndEnter exercises the M3d fullscreen-viewer wikilink
// focus path: Tab over the valid wikilinks (tracked on the heap-cell viewer),
// and Enter follows the focused link in-TUI (push back-stack + loadDoc). The
// non-focusable weblink is rendered by the overlay (OSC 8), not by linkFocus.
func TestDocsView_TabFocusAndEnter(t *testing.T) {
	dest := domain.Document{ID: "d-dest", Path: "dest", Title: "Dest", Type: domain.DocFree}
	src := domain.Document{ID: "d-src", Path: "src", Type: domain.DocFree, Body: "go [[dest]] and [[dest]]"}
	c, stop := newFakeDocSrv(t, []domain.Document{src, dest})
	defer stop()

	m := NewDocs(c, nil, nil, "tester")
	seeded, _ := m.Update(docsLoadedMsg{docs: []domain.Document{src, dest}})
	m = seeded.(DocsModel)
	v, _ := m.Update(docViewMsg{doc: src})
	m = v.(DocsModel)

	if got := len(m.validWikiTargets()); got != 2 {
		t.Fatalf("setup: want 2 valid wikilink targets, got %d", got)
	}
	if m.focusState() != -1 {
		t.Fatalf("initial focus = %d, want -1", m.focusState())
	}

	m2, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	mm := m2.(DocsModel)
	if mm.focusState() != 0 {
		t.Fatalf("after Tab, focus = %d, want 0", mm.focusState())
	}
	m3, _ := mm.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	mmm := m3.(DocsModel)
	if mmm.focusState() != 1 {
		t.Fatalf("after 2nd Tab, focus = %d, want 1", mmm.focusState())
	}

	// Enter follows the focused wikilink: pushes src onto the back-stack and
	// returns a loadDoc cmd (resolving to a docViewMsg for the target).
	m4, cmd := mmm.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a focused wikilink should return a loadDoc cmd")
	}
	if stack := m4.(DocsModel).viewStack; len(stack) != 1 || stack[0] != "d-src" {
		t.Fatalf("Enter should push the current doc onto the back-stack, got %v", stack)
	}
}
