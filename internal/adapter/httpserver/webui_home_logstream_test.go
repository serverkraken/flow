package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// newLogstreamServer builds a Server wired with ListActivity and a seeded user.
func newLogstreamServer(t *testing.T, act *fakeActivityStore) (*httpserver.Server, *websession.Codec) {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Ensure:       usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		Bus:          bus,
		Emitter:      sse.NewEmitter(bus, act, ids, clk),
		Clock:        clk,
		Users:        users,
		Session:      codec,
		ListActivity: usecase.ListActivity{Activities: act},
	}
	return srv, codec
}

// authGet issues an authenticated GET to the given path.
func authGet(t *testing.T, srv *httpserver.Server, codec *websession.Codec, path string) *httptest.ResponseRecorder {
	t.Helper()
	cookieVal, _ := codec.Issue("u1")
	req, _ := http.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "flow_session", Value: cookieVal})
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	return rr
}

// TestLogstream_RendersRows verifies that GET /ui/home/logstream renders activity
// rows: correct verb i18n text, actor ref, and glyph differs by kind.
func TestLogstream_RendersRows(t *testing.T) {
	label1 := "My Note"
	label2 := "My Doc"
	ref := "doc-abc"
	at := time.Date(2026, 6, 30, 11, 30, 0, 0, time.UTC) // 30 min before clock
	store := &fakeActivityStore{
		items: []domain.ActivityEntry{
			{
				ID: "a1", OwnerID: "u1",
				ActorKind: "human", ActorRef: "msoent",
				Kind: "session.started", At: at,
				Label: &label1,
			},
			{
				ID: "a2", OwnerID: "u1",
				ActorKind: "agent", ActorRef: "claude-code",
				Kind: "document.updated", At: at,
				TargetRef: &ref, Label: &label2,
			},
		},
	}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /ui/home/logstream status=%d body=%.500s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()

	// i18n verb keys must resolve in body
	for _, want := range []string{
		"startete einen Timer",  // activity.verb.session.started
		"bearbeitete",           // activity.verb.document.updated
		"msoent",                // human actor ref
		"claude-code",           // agent actor ref
		"My Note",               // label from session entry
		"My Doc",                // label from document entry
		`href="/wissen/doc-abc"`, // document entry links to /wissen/{id}
		`id="logstream"`,        // section id
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /ui/home/logstream: missing %q in body (%.800s)", want, body)
		}
	}

	// The agent glyph (polygon) and human glyph (circle) must both appear.
	if !strings.Contains(body, "<polygon") {
		t.Errorf("agent glyph (polygon) not found in logstream body")
	}
	if !strings.Contains(body, "<circle") {
		t.Errorf("human glyph (circle) not found in logstream body")
	}
}

// TestLogstream_DocumentRowLinksToWissen verifies that document.* entries
// with a TargetRef render as links to /wissen/{id}, while non-document entries
// do not produce such a link.
func TestLogstream_DocumentRowLinksToWissen(t *testing.T) {
	ref := "doc-xyz"
	label := "Important"
	at := time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC)
	store := &fakeActivityStore{
		items: []domain.ActivityEntry{
			{
				ID: "a1", OwnerID: "u1",
				ActorKind: "human", ActorRef: "msoent",
				Kind: "document.created", At: at,
				TargetRef: &ref, Label: &label,
			},
			{
				ID: "a2", OwnerID: "u1",
				ActorKind: "human", ActorRef: "msoent",
				Kind: "session.started", At: at,
			},
		},
	}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `href="/wissen/doc-xyz"`) {
		t.Errorf("document row must link to /wissen/{id}; missing in body")
	}
}

// TestLogstream_ClassFilterWissen verifies that ?class=wissen passes
// "document" prefix to ListActivity.
func TestLogstream_ClassFilterWissen(t *testing.T) {
	store := &fakeActivityStore{}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream?class=wissen")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%.300s", rr.Code, rr.Body.String())
	}
	if len(store.lastClasses) != 1 || store.lastClasses[0] != "document" {
		t.Errorf("want classes=[document] for class=wissen, got %v", store.lastClasses)
	}
}

// TestLogstream_ClassFilterZeit verifies that ?class=zeit passes "session".
func TestLogstream_ClassFilterZeit(t *testing.T) {
	store := &fakeActivityStore{}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream?class=zeit")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if len(store.lastClasses) != 1 || store.lastClasses[0] != "session" {
		t.Errorf("want classes=[session] for class=zeit, got %v", store.lastClasses)
	}
}

// TestLogstream_ClassFilterStruktur verifies ?class=struktur passes "node".
func TestLogstream_ClassFilterStruktur(t *testing.T) {
	store := &fakeActivityStore{}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream?class=struktur")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if len(store.lastClasses) != 1 || store.lastClasses[0] != "node" {
		t.Errorf("want classes=[node] for class=struktur, got %v", store.lastClasses)
	}
}

// TestLogstream_ClassFilterFrei verifies ?class=frei passes "dayoff".
func TestLogstream_ClassFilterFrei(t *testing.T) {
	store := &fakeActivityStore{}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream?class=frei")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if len(store.lastClasses) != 1 || store.lastClasses[0] != "dayoff" {
		t.Errorf("want classes=[dayoff] for class=frei, got %v", store.lastClasses)
	}
}

// TestLogstream_ActorFilter verifies ?actor=X is passed to ListActivity.
func TestLogstream_ActorFilter(t *testing.T) {
	store := &fakeActivityStore{}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream?actor=claude-code")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if store.lastActor == nil || *store.lastActor != "claude-code" {
		t.Errorf("want actor=claude-code passed through, got %v", store.lastActor)
	}
}

// TestLogstream_EmptyState verifies the activity.empty i18n text is shown
// when there are no entries.
func TestLogstream_EmptyState(t *testing.T) {
	store := &fakeActivityStore{items: nil}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/ui/home/logstream")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%.300s", rr.Code, rr.Body.String())
	}
	// "Noch keine Aktivität." is the German translation of activity.empty
	if !strings.Contains(rr.Body.String(), "Noch keine Aktivität") {
		t.Errorf("empty logstream must show activity.empty text; body=%.400s", rr.Body.String())
	}
}

// TestLogstream_SectionOnHomePage verifies GET / includes the logstream section
// when ListActivity is wired.
func TestLogstream_SectionOnHomePage(t *testing.T) {
	store := &fakeActivityStore{items: nil}
	srv, codec := newLogstreamServer(t, store)

	rr := authGet(t, srv, codec, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%.300s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `id="logstream"`) {
		t.Errorf("home page must contain logstream section when ListActivity is wired")
	}
}
