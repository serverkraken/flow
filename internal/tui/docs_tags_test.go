package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// newDocTagsSrv builds a minimal httptest server that tracks UpdateDocument
// requests so tests can assert the tags that were sent.
func newDocTagsSrv(t *testing.T, docs []domain.Document) (*apiclient.Client, *sync.Mutex, *[]string, func()) {
	t.Helper()
	stored := make(map[string]domain.Document, len(docs))
	for _, d := range docs {
		stored[d.ID] = d
	}
	var mu sync.Mutex
	capturedTags := new([]string)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		list := make([]domain.Document, 0, len(stored))
		for _, d := range stored {
			list = append(list, d)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

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

	mux.HandleFunc("GET /api/v1/documents/tags", func(w http.ResponseWriter, r *http.Request) {
		tags := []domain.TagCount{
			{Tag: "existing", Count: 1},
			{Tag: "new-tag", Count: 0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tags)
	})

	mux.HandleFunc("PUT /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		d, ok := stored[id]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in apiclient.UpdateDocumentInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		if in.Tags != nil {
			*capturedTags = append([]string(nil), *in.Tags...)
		}
		mu.Unlock()
		if in.Tags != nil {
			d.Tags = *in.Tags
		}
		stored[id] = d
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})

	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
	})

	srv := httptest.NewServer(mux)
	c := apiclient.New(srv.URL, "tok")
	return c, &mu, capturedTags, srv.Close
}

// TestDocsTags_ModeDocTags_OpenedViaTagsLoadedMsg verifies that a tagsLoadedMsg
// received while m.tagsTarget == modeDocTags switches to modeDocTags and seeds
// filterWork from the viewed document's Tags (not from filterTags).
func TestDocsTags_ModeDocTags_OpenedViaTagsLoadedMsg(t *testing.T) {
	t.Parallel()
	doc := domain.Document{
		ID:    "d1",
		Type:  domain.DocFree,
		Path:  "docs/note",
		Title: "My Note",
		Body:  "body text",
		Tags:  []string{"alpha", "beta"},
	}
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: []domain.Document{doc}})
	m = next.(DocsModel)
	next, _ = m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)
	if m.mode != modeView {
		t.Fatalf("setup: expected modeView, got %v", m.mode)
	}

	// Simulate what the 't' key handler does: set tagsTarget then inject tagsLoadedMsg.
	m.tagsTarget = modeDocTags
	tags := []domain.TagCount{
		{Tag: "alpha", Count: 2},
		{Tag: "beta", Count: 1},
		{Tag: "gamma", Count: 3},
	}
	next, _ = m.Update(tagsLoadedMsg{tags: tags})
	m = next.(DocsModel)

	if m.mode != modeDocTags {
		t.Fatalf("tagsLoadedMsg with tagsTarget=modeDocTags: mode=%v, want modeDocTags", m.mode)
	}
	if m.tagsTarget != 0 {
		t.Fatalf("tagsTarget should be reset to 0 after tagsLoadedMsg, got %v", m.tagsTarget)
	}
	// filterWork must be seeded from doc.Tags, not from filterTags (which is empty).
	if len(m.filterWork) != 2 {
		t.Fatalf("filterWork = %#v, want [alpha beta]", m.filterWork)
	}
	if !containsStr(m.filterWork, "alpha") || !containsStr(m.filterWork, "beta") {
		t.Fatalf("filterWork should contain doc's tags: got %#v", m.filterWork)
	}
}

// TestDocsTags_TagsLoadedMsg_DefaultOpensFiltering confirms that a tagsLoadedMsg
// with tagsTarget at zero (the default) still opens modeFiltering (existing
// behaviour guard).
func TestDocsTags_TagsLoadedMsg_DefaultOpensFiltering(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)

	// tagsTarget is zero (default) → must open modeFiltering, not modeDocTags.
	tags := []domain.TagCount{{Tag: "go", Count: 1}}
	next, _ = m.Update(tagsLoadedMsg{tags: tags})
	m = next.(DocsModel)
	if m.mode != modeFiltering {
		t.Fatalf("default tagsLoadedMsg should open modeFiltering, got %v", m.mode)
	}
}

// TestDocsTags_ToggleAndCommit is the primary integration test: drive the model
// into modeDocTags, toggle one additional tag, press Enter, and assert the fake
// server received UpdateDocument with both the original and the newly toggled tag.
func TestDocsTags_ToggleAndCommit(t *testing.T) {
	t.Parallel()
	doc := domain.Document{
		ID:    "d1",
		Type:  domain.DocFree,
		Path:  "docs/note",
		Title: "My Note",
		Body:  "body text",
		Tags:  []string{"existing"},
	}
	c, mu, capturedTags, stop := newDocTagsSrv(t, []domain.Document{doc})
	defer stop()

	m := NewDocs(c, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: []domain.Document{doc}})
	m = next.(DocsModel)
	next, _ = m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)

	// Enter modeDocTags by simulating what 't' + loadTags does.
	m.tagsTarget = modeDocTags
	opts := []domain.TagCount{
		{Tag: "existing", Count: 1},
		{Tag: "new-tag", Count: 0},
	}
	next, _ = m.Update(tagsLoadedMsg{tags: opts})
	m = next.(DocsModel)
	if m.mode != modeDocTags {
		t.Fatalf("expected modeDocTags, got %v", m.mode)
	}
	// filterWork seeded from doc.Tags = [existing].
	if len(m.filterWork) != 1 || m.filterWork[0] != "existing" {
		t.Fatalf("filterWork = %#v, want [existing]", m.filterWork)
	}

	// Move cursor to "new-tag" (index 1) and toggle it in.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(DocsModel)
	if m.filterCursor != 1 {
		t.Fatalf("filterCursor = %d, want 1 after KeyDown", m.filterCursor)
	}
	next, _ = m.Update(tea.KeyPressMsg{Text: " "})
	m = next.(DocsModel)
	if len(m.filterWork) != 2 {
		t.Fatalf("filterWork after toggle = %#v, want [existing new-tag]", m.filterWork)
	}
	if !containsStr(m.filterWork, "new-tag") {
		t.Fatalf("filterWork should contain new-tag: %#v", m.filterWork)
	}

	// Press Enter to commit.
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if m.mode != modeView {
		t.Fatalf("after Enter in modeDocTags: mode=%v, want modeView", m.mode)
	}
	if cmd == nil {
		t.Fatal("Enter in modeDocTags must return a non-nil UpdateDocument cmd")
	}

	// Execute the cmd; it should call UpdateDocument and return docTagsSavedMsg.
	msg := cmd()
	saved, ok := msg.(docTagsSavedMsg)
	if !ok {
		t.Fatalf("cmd returned %T (%v), want docTagsSavedMsg", msg, msg)
	}
	if saved.docID != "d1" {
		t.Fatalf("docTagsSavedMsg.docID = %q, want d1", saved.docID)
	}

	// Verify the server received the expected tags.
	mu.Lock()
	got := append([]string(nil), *capturedTags...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("UpdateDocument received tags %#v, want exactly [existing new-tag]", got)
	}
	tagSet := make(map[string]bool, len(got))
	for _, tg := range got {
		tagSet[tg] = true
	}
	if !tagSet["existing"] || !tagSet["new-tag"] {
		t.Fatalf("UpdateDocument tags %#v, want {existing, new-tag}", got)
	}
}

// TestDocsTags_EscDiscards verifies that pressing Esc in modeDocTags returns to
// modeView without persisting any changes.
func TestDocsTags_EscDiscards(t *testing.T) {
	t.Parallel()
	doc := sampleDocs()[0]
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)

	// Open modeDocTags.
	m.tagsTarget = modeDocTags
	next, _ = m.Update(tagsLoadedMsg{tags: []domain.TagCount{{Tag: "go", Count: 1}}})
	m = next.(DocsModel)
	if m.mode != modeDocTags {
		t.Fatalf("setup: expected modeDocTags, got %v", m.mode)
	}

	// Toggle a tag so filterWork is non-empty.
	next, _ = m.Update(tea.KeyPressMsg{Text: " "})
	m = next.(DocsModel)

	// Esc must return to modeView without calling UpdateDocument.
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = nm.(DocsModel)
	if m.mode != modeView {
		t.Fatalf("Esc should return to modeView, got %v", m.mode)
	}
	if cmd != nil {
		t.Fatal("Esc in modeDocTags must not return a cmd (no UpdateDocument call)")
	}
	if m.tagNewBuf != "" {
		t.Fatalf("tagNewBuf should be cleared on Esc, got %q", m.tagNewBuf)
	}
}

// TestDocsTags_TypeNewTagAndAdd verifies that typing characters in modeDocTags
// builds tagNewBuf and Enter adds the typed tag to filterWork without committing.
func TestDocsTags_TypeNewTagAndAdd(t *testing.T) {
	t.Parallel()
	doc := sampleDocs()[0]
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)

	m.tagsTarget = modeDocTags
	next, _ = m.Update(tagsLoadedMsg{tags: []domain.TagCount{{Tag: "go", Count: 1}}})
	m = next.(DocsModel)

	// Type "rust".
	for _, ch := range "rust" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(ch)})
		m = next.(DocsModel)
	}
	if m.tagNewBuf != "rust" {
		t.Fatalf("tagNewBuf = %q, want rust", m.tagNewBuf)
	}
	// Backspace removes last char.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(DocsModel)
	if m.tagNewBuf != "rus" {
		t.Fatalf("tagNewBuf after backspace = %q, want rus", m.tagNewBuf)
	}
	// Type the 't' back.
	next, _ = m.Update(tea.KeyPressMsg{Text: "t"})
	m = next.(DocsModel)

	// Enter with non-empty buffer adds the tag to filterWork and clears the buffer.
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(DocsModel)
	if cmd != nil {
		t.Fatal("Enter with non-empty buffer should not commit (no UpdateDocument cmd)")
	}
	if m.tagNewBuf != "" {
		t.Fatalf("tagNewBuf should be cleared after Enter, got %q", m.tagNewBuf)
	}
	if !containsStr(m.filterWork, "rust") {
		t.Fatalf("filterWork should contain rust after Enter, got %#v", m.filterWork)
	}
	// Mode should stay modeDocTags (not committed yet).
	if m.mode != modeDocTags {
		t.Fatalf("mode should stay modeDocTags after adding a new tag, got %v", m.mode)
	}
}

// TestDocsTags_CapturesText verifies that modeDocTags returns true from CapturesText.
func TestDocsTags_CapturesText(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.mode = modeDocTags
	if !m.CapturesText() {
		t.Fatal("modeDocTags must return CapturesText=true so the shell frame does not intercept keys")
	}
	if !m.CapturesInput() {
		t.Fatal("modeDocTags must return CapturesInput=true")
	}
}

// TestDocsTags_RenderDocTags exercises renderDocTags via View() and verifies the
// heading, [x]/[ ] marks, cursor highlight, new-tag input row, and working-set
// summary all appear.
func TestDocsTags_RenderDocTags(t *testing.T) {
	t.Parallel()
	doc := domain.Document{
		ID:    "d1",
		Type:  domain.DocFree,
		Path:  "docs/note",
		Title: "My Note",
		Body:  "body",
		Tags:  []string{"go"},
	}
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)

	m.tagsTarget = modeDocTags
	opts := []domain.TagCount{
		{Tag: "go", Count: 3},
		{Tag: "tui", Count: 1},
	}
	next, _ = m.Update(tagsLoadedMsg{tags: opts})
	m = next.(DocsModel)
	if m.mode != modeDocTags {
		t.Fatalf("setup: expected modeDocTags, got %v", m.mode)
	}

	out := m.View().Content
	if !strings.Contains(out, "Tags — My Note") {
		t.Errorf("view missing 'Tags — My Note' heading; got:\n%.300s", out)
	}
	if !strings.Contains(out, "#go") {
		t.Errorf("view missing tag #go; got:\n%.300s", out)
	}
	if !strings.Contains(out, "#tui") {
		t.Errorf("view missing tag #tui; got:\n%.300s", out)
	}
	// "go" is in filterWork (seeded from doc.Tags) → must show [x].
	if !strings.Contains(out, "[x]") {
		t.Errorf("view should show [x] for selected tag go; got:\n%.300s", out)
	}
	// "tui" is not selected → must show [ ].
	if !strings.Contains(out, "[ ]") {
		t.Errorf("view should show [ ] for unselected tag tui; got:\n%.300s", out)
	}
	// New-tag input row.
	if !strings.Contains(out, "neuer tag") {
		t.Errorf("view missing 'neuer tag' input row; got:\n%.300s", out)
	}
}

// TestDocsTags_TKeyInViewMode verifies that pressing 't' in modeView sets
// tagsTarget=modeDocTags and returns a non-nil cmd (loadTags). With a nil
// client the cmd is nil; with a real client it is non-nil.
func TestDocsTags_TKeyInViewMode(t *testing.T) {
	t.Parallel()
	doc := sampleDocs()[0]
	c, _, _, stop := newDocTagsSrv(t, sampleDocs())
	defer stop()

	m := NewDocs(c, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(docViewMsg{doc: doc})
	m = next.(DocsModel)

	nm, cmd := m.Update(tea.KeyPressMsg{Text: "t"})
	mm := nm.(DocsModel)
	if cmd == nil {
		t.Fatal("'t' in modeView with a real client should return non-nil loadTags cmd")
	}
	// Model should still be in modeView (not modeDocTags yet — tagsLoadedMsg hasn't arrived).
	if mm.mode != modeView {
		t.Fatalf("mode after 't' key = %v, want modeView (tagsLoadedMsg hasn't arrived)", mm.mode)
	}
}

// TestDocsTags_TKeyInViewMode_NilViewing verifies the nil-guard: if m.viewing is
// nil, 't' returns a nil cmd and stays in modeView.
func TestDocsTags_TKeyInViewMode_NilViewing(t *testing.T) {
	t.Parallel()
	m := NewDocs(nil, nil, nil, theme.Default, "tester")
	m.mode = modeView // viewing is nil
	_, cmd := m.Update(tea.KeyPressMsg{Text: "t"})
	if cmd != nil {
		t.Fatal("'t' with nil viewing should return nil cmd")
	}
}

// TestDocsTags_DocTagsSavedMsg_TriggersReload verifies that docTagsSavedMsg
// sets the status message and returns a non-nil batch cmd (list reload + doc
// re-fetch) when the client is non-nil.
func TestDocsTags_DocTagsSavedMsg_TriggersReload(t *testing.T) {
	t.Parallel()
	c, _, _, stop := newDocTagsSrv(t, sampleDocs())
	defer stop()

	m := NewDocs(c, nil, nil, theme.Default, "tester")
	next, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next.(DocsModel)

	nm, cmd := m.Update(docTagsSavedMsg{docID: "d1"})
	mm := nm.(DocsModel)
	if !strings.Contains(mm.status, "tags") {
		t.Fatalf("status should mention tags, got %q", mm.status)
	}
	if cmd == nil {
		t.Fatal("docTagsSavedMsg should return a non-nil batch cmd (reload + loadDoc)")
	}
}

// TestDocsTags_LoadTagsScope verifies that loadTags() (with a real HTTP server)
// calls GET /api/v1/documents/tags?type=document rather than the unscoped
// /api/v1/documents/tags endpoint. We do this by checking the query string the
// server receives.
func TestDocsTags_LoadTagsScope(t *testing.T) {
	t.Parallel()
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/documents/tags") {
			receivedQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domain.TagCount{})
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tok")

	m := NewDocs(c, nil, nil, theme.Default, "tester")
	cmd := m.loadTags()
	if cmd == nil {
		t.Fatal("loadTags with real client must return non-nil cmd")
	}
	_ = cmd() // execute the HTTP call

	if receivedQuery != "type=document" {
		t.Fatalf("loadTags query = %q, want type=document (scoped, not owner-wide)", receivedQuery)
	}
}
