package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeActivityStore is a simple in-memory ActivityStore for handler tests.
// It records the last call's arguments so tests can assert filter pass-through.
type fakeActivityStore struct {
	items     []domain.ActivityEntry
	lastOwner string
	lastClasses []string
	lastActor *string
	lastLimit  int
	lastOffset int
}

func (f *fakeActivityStore) Append(_ context.Context, _ domain.ActivityEntry) error { return nil }

func (f *fakeActivityStore) ListPage(_ context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	f.lastOwner = ownerID
	f.lastClasses = classes
	f.lastActor = actorRef
	f.lastLimit = limit
	f.lastOffset = offset
	return f.items, len(f.items), nil
}

// DistinctActors returns de-duplicated actor_refs from the seeded items,
// mirroring the real store's behaviour for httpserver unit tests.
func (f *fakeActivityStore) DistinctActors(_ context.Context, _ string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	for _, e := range f.items {
		if _, ok := seen[e.ActorRef]; !ok {
			seen[e.ActorRef] = struct{}{}
			out = append(out, e.ActorRef)
		}
	}
	return out, nil
}

// newActivityServer builds a minimal Server wired with the ListActivity usecase
// backed by the given fakeActivityStore. Bearer "x" authenticates.
func newActivityServer(t *testing.T, store *fakeActivityStore) (*httptest.Server, *fakeActivityStore) {
	t.Helper()
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	srv := &httpserver.Server{
		Verifier:     testutil.FakeVerifier{ID: ports.Identity{Subject: "sub-1", Username: "msoent"}},
		Ensure:       usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:          sse.NewBus(),
		ListActivity: usecase.ListActivity{Activities: store},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, store
}

func doActivity(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer x")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return res
}

func TestHandleListActivity_HappyPath(t *testing.T) {
	at := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	label := "my-note"
	store := &fakeActivityStore{
		items: []domain.ActivityEntry{
			{ID: "a1", ActorKind: "human", ActorRef: "msoent", Kind: "document.updated", Label: &label, At: at},
		},
	}
	ts, st := newActivityServer(t, store)

	res := doActivity(t, ts, "/api/v1/activity?limit=10")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("want 200, got %d: %s", res.StatusCode, body)
	}
	var items []domain.ActivityEntry
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 1 || items[0].ID != "a1" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if st.lastLimit != 10 {
		t.Errorf("want limit=10 passed through, got %d", st.lastLimit)
	}
}

func TestHandleListActivity_ClassFilter(t *testing.T) {
	store := &fakeActivityStore{}
	ts, st := newActivityServer(t, store)

	res := doActivity(t, ts, "/api/v1/activity?class=document")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	if len(st.lastClasses) != 1 || st.lastClasses[0] != "document" {
		t.Errorf("want classes=[document], got %v", st.lastClasses)
	}
}

func TestHandleListActivity_ActorFilter(t *testing.T) {
	store := &fakeActivityStore{}
	ts, st := newActivityServer(t, store)

	res := doActivity(t, ts, "/api/v1/activity?actor=claude-code")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	if st.lastActor == nil || *st.lastActor != "claude-code" {
		t.Errorf("want actor=claude-code, got %v", st.lastActor)
	}
}

func TestHandleListActivity_NilResultSerializesAsEmptyArray(t *testing.T) {
	store := &fakeActivityStore{items: nil} // ListPage returns nil slice
	ts, _ := newActivityServer(t, store)

	res := doActivity(t, ts, "/api/v1/activity")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	// must serialize as [] not null
	bodyStr := string(body)
	if bodyStr == "null\n" || bodyStr == "null" {
		t.Errorf("nil result must serialize as [], got: %s", bodyStr)
	}
	var items []domain.ActivityEntry
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if items == nil {
		t.Error("want non-nil empty slice in JSON, got null")
	}
}

func TestHandleListActivity_RequiresAuth(t *testing.T) {
	store := &fakeActivityStore{}
	ts, _ := newActivityServer(t, store)

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/activity", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", res.StatusCode)
	}
}
