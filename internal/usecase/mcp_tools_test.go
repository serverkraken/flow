package usecase_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeWorktimeReader is a tiny stub for the WorktimeStatusReader
// interface. The real *usecase.WorktimeReader pulls in
// TargetResolver + ConfigReader + DayOffStore + LegacyActiveStore;
// tests don't need that machinery to verify the MCP status text.
type fakeWorktimeReader struct {
	day domain.Day
	err error
}

func (f *fakeWorktimeReader) Today() (domain.Day, error) { return f.day, f.err }

// fakeDocumentStore is an in-memory ports.DocumentStore for MCP tests.
// It stores documents keyed by (userID, path) and repo notes by repoKey.
type fakeDocumentStore struct {
	byPath    map[string]ports.Document // "userID|path" → Document
	byRepoKey map[string]ports.Document // "userID|repoKey" → Document
	listErr   error
	getErr    error
	putErr    error
}

func newFakeDocumentStore() *fakeDocumentStore {
	return &fakeDocumentStore{
		byPath:    make(map[string]ports.Document),
		byRepoKey: make(map[string]ports.Document),
	}
}

func (f *fakeDocumentStore) Get(userID, path string) (ports.Document, error) {
	if f.getErr != nil {
		return ports.Document{}, f.getErr
	}
	d, ok := f.byPath[userID+"|"+path]
	if !ok {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	return d, nil
}

func (f *fakeDocumentStore) GetByRepoKey(userID, repoKey string) (ports.Document, error) {
	if f.getErr != nil {
		return ports.Document{}, f.getErr
	}
	d, ok := f.byRepoKey[userID+"|"+repoKey]
	if !ok {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	return d, nil
}

func (f *fakeDocumentStore) List(userID, prefix, query string, limit int) ([]ports.DocumentEntry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	q := strings.ToLower(query)
	var out []ports.DocumentEntry
	for _, d := range f.byRepoKey {
		if d.UserID != userID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(d.Body), q) {
			continue
		}
		out = append(out, ports.DocumentEntry{
			Path:      d.Path,
			RepoKey:   d.RepoKey,
			Version:   d.Version,
			UpdatedAt: d.UpdatedAt,
			Snippet:   query, // simulate FTS headline
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeDocumentStore) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	if f.putErr != nil {
		return ports.Document{}, f.putErr
	}
	d := ports.Document{
		ID:        "doc-" + repoKey + path,
		UserID:    userID,
		Path:      path,
		Body:      body,
		RepoKey:   repoKey,
		Version:   ifMatch + 1,
		UpdatedAt: time.Now().UTC(),
	}
	if repoKey != "" {
		f.byRepoKey[userID+"|"+repoKey] = d
	} else {
		f.byPath[userID+"|"+path] = d
	}
	return d, nil
}

func (f *fakeDocumentStore) Delete(userID, path string) error {
	delete(f.byPath, userID+"|"+path)
	return nil
}

// mkTools wires a fully-functional MCPTools backed by in-memory fakes.
// Returns the MCPTools, the document store, the active store, the project store,
// and the machine so individual tests can seed state or configure error returns.
// authed controls whether the auth gate is open.
func mkTools(
	t *testing.T,
	authed bool,
) (*usecase.MCPTools, *fakeDocumentStore, *fakeActiveSessionStore, *fakeASProjectStore, *fakeWorktimeMachine) {
	t.Helper()
	docs := newFakeDocumentStore()

	active := newFakeActiveSessionStore()
	projects := &fakeASProjectStore{}
	sessions := &fakeASSessionStore{}
	machine := &fakeWorktimeMachine{}
	activeUC := usecase.NewActiveSessions(nil, projects, active, machine)
	sessionsUC := usecase.NewSessions(nil, projects, sessions, nil)

	reader := &fakeWorktimeReader{day: domain.Day{
		Target: 8 * time.Hour, Logged: 90 * time.Minute,
	}}

	return &usecase.MCPTools{
		UserID:       "u1",
		Pwd:          "/home/me/code/widget",
		Authed:       authed,
		Documents:    docs,
		Resolver:     fakeResolverPkg{url: "git@github.com:acme/widget.git", ok: true},
		Active:       activeUC,
		Sessions:     sessionsUC,
		Reader:       reader,
		ProjectStore: projects,
	}, docs, active, projects, machine
}

// ---- Catalog ----

func TestMCPTools_Catalog_ShipsSevenTools(t *testing.T) {
	t.Parallel()
	m := &usecase.MCPTools{}
	cat := m.Catalog()
	if len(cat) != 7 {
		t.Fatalf("Catalog: got %d tools, want 7", len(cat))
	}
	wantNames := map[string]bool{
		"flow_get_repo_note":   true,
		"flow_save_repo_note":  true,
		"flow_list_repo_notes": true,
		"flow_search_notes":    true,
		"flow_worktime_status": true,
		"flow_start_session":   true,
		"flow_stop_session":    true,
	}
	for _, td := range cat {
		if !wantNames[td.Name] {
			t.Errorf("unexpected tool: %s", td.Name)
		}
		delete(wantNames, td.Name)
		if td.Description == "" {
			t.Errorf("%s: missing description", td.Name)
		}
		if td.InputSchema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want object", td.Name, td.InputSchema["type"])
		}
	}
	for n := range wantNames {
		t.Errorf("missing tool: %s", n)
	}
}

// ---- Auth gate ----

func TestMCPTools_Authed_False_ReturnsLoginRequired(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, false)
	for _, name := range []string{
		"flow_get_repo_note", "flow_save_repo_note", "flow_list_repo_notes",
		"flow_search_notes", "flow_worktime_status",
		"flow_start_session", "flow_stop_session",
	} {
		got := m.Call(name, map[string]any{"content": "x", "query": "x"})
		if !got.IsError {
			t.Errorf("%s: IsError=false, want true", name)
		}
		if !strings.Contains(got.Text, "Login required") {
			t.Errorf("%s: text = %q, want contains 'Login required'", name, got.Text)
		}
	}
}

func TestMCPTools_Authed_False_ResourceCatalogEmpty(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, false)
	if got := m.ResourceCatalog(); got != nil {
		t.Errorf("ResourceCatalog: got %d entries, want nil", len(got))
	}
}

// ---- Unknown tool ----

func TestMCPTools_UnknownTool(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_nonexistent", nil)
	if !got.IsError || !strings.Contains(got.Text, "unknown tool") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_get_repo_note ----

func TestMCPTools_GetRepoNote_NewRepo_ReturnsHint(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_get_repo_note", map[string]any{})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no RepoNote yet") {
		t.Errorf("text = %q, want 'no RepoNote yet'", got.Text)
	}
}

func TestMCPTools_GetRepoNote_ExistingNote_ReturnsBody(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	// Seed a note for the widget repo (resolved via git remote in the fake resolver).
	_, _ = docs.Put("u1", "", "# widget rules\nbe nice", "git:github.com/acme/widget", 0)
	got := m.Call("flow_get_repo_note", map[string]any{})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "be nice") {
		t.Errorf("text missing body: %q", got.Text)
	}
}

func TestMCPTools_GetRepoNote_NoPwd(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	m.Pwd = ""
	got := m.Call("flow_get_repo_note", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "PWD is empty") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_save_repo_note ----

func TestMCPTools_SaveRepoNote_RoundTrip(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_save_repo_note", map[string]any{"content": "hello"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "saved") {
		t.Errorf("text = %q", got.Text)
	}
	// Verify persistence by reading back.
	read := m.Call("flow_get_repo_note", map[string]any{})
	if !strings.Contains(read.Text, "hello") {
		t.Errorf("get after save missing content: %q", read.Text)
	}
}

func TestMCPTools_SaveRepoNote_MissingContent(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_save_repo_note", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "'content' is required") {
		t.Fatalf("got %+v", got)
	}
}

// ---- flow_list_repo_notes ----

func TestMCPTools_ListRepoNotes_Empty(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_list_repo_notes", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "0 repo") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_ListRepoNotes_WithEntries(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	_, _ = docs.Put("u1", "", "alpha content", "git:github.com/acme/widget", 0)
	got := m.Call("flow_list_repo_notes", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "widget") {
		t.Errorf("text missing repo: %q", got.Text)
	}
}

// ---- flow_search_notes ----

func TestMCPTools_SearchNotes_MissingQuery(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_search_notes", map[string]any{})
	if !got.IsError || !strings.Contains(got.Text, "'query' is required") {
		t.Fatalf("got %+v", got)
	}
}

func TestMCPTools_SearchNotes_NoMatches(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	_, _ = docs.Put("u1", "", "alpha beta", "git:github.com/acme/widget", 0)
	got := m.Call("flow_search_notes", map[string]any{"query": "zeta"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no matches") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_SearchNotes_FindsMatchCaseInsensitive(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	_, _ = docs.Put("u1", "", "ALPHA bravo charlie", "git:github.com/acme/widget", 0)
	got := m.Call("flow_search_notes", map[string]any{"query": "alpha"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "match") {
		t.Errorf("text = %q", got.Text)
	}
}

// ---- flow_worktime_status ----

func TestMCPTools_WorktimeStatus_NoActive(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	got := m.Call("flow_worktime_status", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "no active session") {
		t.Errorf("text = %q", got.Text)
	}
	if !strings.Contains(got.Text, "logged=1h30m0s") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_WorktimeStatus_WithActive(t *testing.T) {
	t.Parallel()
	m, _, active, projects, _ := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	_ = active.Upsert(domain.ActiveSession{
		UserID: "u1", ProjectID: "p1",
		StartedAt:       time.Now().Add(-30 * time.Minute),
		StartedOnDevice: "macbook",
		Tag:             "deep",
	})
	got := m.Call("flow_worktime_status", nil)
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "Widget (widget)") {
		t.Errorf("text missing project name: %q", got.Text)
	}
	if !strings.Contains(got.Text, "macbook") {
		t.Errorf("text missing device: %q", got.Text)
	}
}

// ---- flow_start_session / flow_stop_session ----

func TestMCPTools_StartSession_HappyPath(t *testing.T) {
	t.Parallel()
	m, _, _, projects, machine := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	// Machine returns a successful ActiveSession for the project.
	machine.startResult = domain.ActiveSession{
		UserID: "u1", ProjectID: "p1",
		StartedAt: time.Now().UTC(),
		Tag:       "deep",
	}
	got := m.Call("flow_start_session", map[string]any{"project": "widget", "tag": "deep"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "started project=Widget") || !strings.Contains(got.Text, "tag=\"deep\"") {
		t.Errorf("text = %q", got.Text)
	}
}

func TestMCPTools_StartSession_AlreadyRunning(t *testing.T) {
	t.Parallel()
	m, _, _, projects, machine := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	// Machine returns conflict → mapped to ErrActiveSessionExists by the use case.
	machine.startErr = ports.ErrActiveSessionConflict
	got := m.Call("flow_start_session", map[string]any{"project": "widget"})
	if !got.IsError || !strings.Contains(got.Text, "already running") {
		t.Fatalf("got %+v", got)
	}
}

func TestMCPTools_StopSession_HappyPath(t *testing.T) {
	t.Parallel()
	m, _, _, projects, machine := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	machine.stopResult = domain.Session{
		ID:        "sess-1",
		UserID:    "u1",
		ProjectID: "p1",
		Elapsed:   time.Hour,
		Tag:       "deep",
	}
	got := m.Call("flow_stop_session", map[string]any{"project": "widget"})
	if got.IsError {
		t.Fatalf("unexpected error: %+v", got)
	}
	if !strings.Contains(got.Text, "stopped project=Widget") {
		t.Errorf("text = %q", got.Text)
	}
}

// ---- Resources ----

func TestMCPTools_Resources_EmptyWhenNoRepos(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	if got := m.ResourceCatalog(); len(got) != 0 {
		t.Errorf("ResourceCatalog: got %d entries, want 0", len(got))
	}
}

func TestMCPTools_Resources_OneEntryPerRepo(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	_, _ = docs.Put("u1", "", "content1", "git:github.com/acme/widget", 0)
	_, _ = docs.Put("u1", "", "content2", "git:github.com/acme/gadget", 0)

	got := m.ResourceCatalog()
	if len(got) != 2 {
		t.Fatalf("ResourceCatalog: got %d entries, want 2", len(got))
	}
	for _, r := range got {
		if !strings.HasPrefix(r.URI, "flow://repos/") || !strings.HasSuffix(r.URI, "/note") {
			t.Errorf("bad URI shape: %q", r.URI)
		}
		if r.MimeType != "text/markdown" {
			t.Errorf("MimeType: %q", r.MimeType)
		}
	}
}

func TestMCPTools_Resources_URLEscapesCanonicalKey(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	key := "git:github.com/acme/space project"
	_, _ = docs.Put("u1", "", "body", key, 0)

	got := m.ResourceCatalog()
	if len(got) != 1 {
		t.Fatalf("ResourceCatalog: got %d entries, want 1", len(got))
	}
	uri := got[0].URI
	if !strings.Contains(uri, "%20") {
		t.Errorf("URI %q missing %%20 for space", uri)
	}
	if !strings.Contains(uri, "%2F") && !strings.Contains(uri, "%2f") {
		t.Errorf("URI %q missing %%2F for slash", uri)
	}
}

func TestMCPTools_Resources_ReadByURI_ReturnsNote(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	_, _ = docs.Put("u1", "", "# rules", "git:github.com/acme/widget", 0)

	cat := m.ResourceCatalog()
	if len(cat) != 1 {
		t.Fatalf("cat: %d", len(cat))
	}
	content, err := m.ReadResource(cat[0].URI)
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content.Text != "# rules" {
		t.Errorf("body = %q", content.Text)
	}
	if content.MimeType != "text/markdown" {
		t.Errorf("MimeType: %q", content.MimeType)
	}
}

func TestMCPTools_Resources_ReadByURI_EmptyBody(t *testing.T) {
	t.Parallel()
	m, docs, _, _, _ := mkTools(t, true)
	// Seed a repo note with an empty body (created but blank).
	_, _ = docs.Put("u1", "", "", "git:github.com/acme/widget", 0)

	cat := m.ResourceCatalog()
	if len(cat) != 1 {
		t.Fatalf("cat: %d", len(cat))
	}
	content, err := m.ReadResource(cat[0].URI)
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content.Text != "" {
		t.Errorf("expected empty body, got %q", content.Text)
	}
	if content.MimeType != "text/markdown" {
		t.Errorf("MimeType: %q", content.MimeType)
	}
}

func TestMCPTools_Resources_ReadByURI_UnknownReturnsNotFound(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	_, err := m.ReadResource("flow://repos/does-not-exist/note")
	if err == nil {
		t.Fatal("expected error for unknown URI")
	}
	if err != usecase.ErrResourceNotFound {
		t.Errorf("error = %v, want ErrResourceNotFound", err)
	}
}

func TestMCPTools_Resources_ReadByURI_MalformedReturnsNotFound(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := mkTools(t, true)
	for _, uri := range []string{"", "flow://nope", "flow://repos//note", "http://foo"} {
		_, err := m.ReadResource(uri)
		if err != usecase.ErrResourceNotFound {
			t.Errorf("uri=%q: err=%v, want ErrResourceNotFound", uri, err)
		}
	}
}

func TestMCPTools_StopSession_NotRunning(t *testing.T) {
	t.Parallel()
	m, _, _, projects, machine := mkTools(t, true)
	projects.projects = append(projects.projects, domain.Project{ID: "p1", Name: "Widget", Slug: "widget"})
	machine.stopErr = ports.ErrActiveSessionNotFound
	got := m.Call("flow_stop_session", map[string]any{"project": "widget"})
	if !got.IsError || !strings.Contains(got.Text, "no active session") {
		t.Fatalf("got %+v", got)
	}
}
