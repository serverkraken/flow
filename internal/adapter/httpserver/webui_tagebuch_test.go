package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type tagebuchSrv struct {
	ts     *httptest.Server
	cookie *http.Cookie
	docs   *testutil.FakeDocumentStore
	nodes  *testutil.FakeNodeStore
	hs     *testutil.FakeNodeHighlightStore
	clk    testutil.FakeClock
}

func newTagebuchSrv(t *testing.T) *tagebuchSrv {
	t.Helper()
	clk := testutil.FakeClock{T: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	hs := testutil.NewFakeNodeHighlightStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x.de", "M")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("test-secret-test-secret-test-12", time.Hour)
	bus := sse.NewBus()
	srv := &httpserver.Server{
		Users:                  users,
		Session:                codec,
		Bus:                    bus,
		Emitter:                sse.NewEmitter(bus, &fakeActivityStore{}, ids, clk),
		Clock:                  clk,
		Ensure:                 usecase.EnsureUser{Users: users, IDs: ids, Allow: func(ports.Identity) bool { return true }},
		ListDocuments:          usecase.ListDocuments{Docs: docs},
		GetDocument:            usecase.GetDocument{Docs: docs},
		CreateDocument:         usecase.CreateDocument{Docs: docs, IDs: ids, Clock: clk},
		ListNodes:              usecase.ListNodes{Nodes: nodes},
		AssignHighlight:        usecase.AssignHighlight{Highlights: hs, IDs: ids, Clock: clk},
		RemoveHighlight:        usecase.RemoveHighlight{Highlights: hs},
		ListDocumentHighlights: usecase.ListDocumentHighlights{Highlights: hs},
		ListRecentHighlights:   usecase.ListRecentHighlights{Highlights: hs},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cv, _ := codec.Issue("u1")
	return &tagebuchSrv{ts: ts, cookie: &http.Cookie{Name: "flow_session", Value: cv}, docs: docs, nodes: nodes, hs: hs, clk: clk}
}

func (s *tagebuchSrv) get(t *testing.T, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", s.ts.URL+path, nil)
	req.AddCookie(s.cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (s *tagebuchSrv) post(t *testing.T, path string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", s.ts.URL+path, strings.NewReader(form.Encode()))
	req.AddCookie(s.cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.ts.URL)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (s *tagebuchSrv) seedDaily(t *testing.T, id, title, body string, at time.Time) domain.Document {
	t.Helper()
	// Date is what the month list sorts and filters on — CreateDocument
	// stamps it for daily notes, so a fixture without it would silently sit
	// outside every month.
	when := at
	d, err := s.docs.Create(context.Background(), domain.Document{
		ID: id, OwnerID: "u1", Type: domain.DocDaily, Path: domain.DailyPath(at),
		Title: title, Body: body, Date: &when, CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestWebTagebuch_ShowsTheMonthsNotes covers Screen 04: the month's daily
// notes with the selected one's body — and nothing that is not a daily note.
func TestWebTagebuch_ShowsTheMonthsNotes(t *testing.T) {
	srv := newTagebuchSrv(t)
	d := srv.seedDaily(t, "d1", "2026-08-20", "Heute am Karteikasten gebaut.", srv.clk.T.Add(-24*time.Hour))
	// A free note dressed like a daily one: same month, same shape, only the
	// TYPE separates it. That is the only thing the filter may go by.
	sameDay := srv.clk.T
	if _, err := srv.docs.Create(context.Background(), domain.Document{
		ID: "frei1", OwnerID: "u1", Type: domain.DocFree, Path: domain.DailyPath(srv.clk.T),
		Title: "Kein Tagebuch", Body: "Fremder Text", Date: &sameDay,
		CreatedAt: srv.clk.T, UpdatedAt: srv.clk.T,
	}); err != nil {
		t.Fatal(err)
	}

	code, body := srv.get(t, "/tagebuch?selected="+d.ID)
	if code != http.StatusOK {
		t.Fatalf("GET /tagebuch = %d", code)
	}
	if !strings.Contains(body, "Karteikasten gebaut") {
		t.Errorf("selected note's body missing; body=%.1200s", body)
	}
	// The month list renders excerpts, not titles — so the excerpt text is
	// what proves whether the foreign note leaked in.
	if strings.Contains(body, "Fremder Text") {
		t.Errorf("a non-daily document leaked into the Tagebuch; body=%.1200s", body)
	}
}

// TestWebTagebuchToday_CreatesTodaysNoteOnce pins "Heute schreiben": the first
// visit creates today's note, the second reuses it instead of piling up
// duplicates for one day.
func TestWebTagebuchToday_CreatesTodaysNoteOnce(t *testing.T) {
	srv := newTagebuchSrv(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	visit := func() string {
		req, _ := http.NewRequest("GET", srv.ts.URL+"/tagebuch/heute", nil)
		req.AddCookie(srv.cookie)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("GET /tagebuch/heute = %d, want 303", res.StatusCode)
		}
		return res.Header.Get("Location")
	}
	first := visit()
	second := visit()
	if first != second {
		t.Errorf("second visit made another note: %q vs %q", first, second)
	}
	all, _ := srv.docs.List(context.Background(), "u1", nil)
	daily := 0
	for _, d := range all {
		if d.Type == domain.DocDaily {
			daily++
		}
	}
	if daily != 1 {
		t.Errorf("got %d daily notes for one day, want 1", daily)
	}
}

// TestWebTagebuchHighlights_AssignAndRemove covers Screen 27 end to end and
// keeps the assignment owner-scoped.
func TestWebTagebuchHighlights_AssignAndRemove(t *testing.T) {
	srv := newTagebuchSrv(t)
	d := srv.seedDaily(t, "d1", "2026-08-21", "Eine Stelle, die zählt.", srv.clk.T)
	n, _ := domain.NewNode("v1", "u1", "Karteikasten", "karteikasten", srv.clk.T)
	n.Kind = domain.KindVorhaben
	if _, err := srv.nodes.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	res := srv.post(t, "/tagebuch/"+d.ID+"/highlights", url.Values{
		"quote": {"Eine Stelle, die zählt."}, "nodeId": {"v1"},
	})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("assign = %d, want 303", res.StatusCode)
	}
	list, err := srv.hs.ListForDocument(context.Background(), "u1", d.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("highlight not stored: %+v err=%v", list, err)
	}

	code, body := srv.get(t, "/tagebuch/"+d.ID+"/markieren")
	if code != http.StatusOK {
		t.Fatalf("GET markieren = %d", code)
	}
	if !strings.Contains(body, "Karteikasten") {
		t.Errorf("assignment must name its register; body=%.1500s", body)
	}

	// A blank quote is not an assignment — it must not create a second row.
	res2 := srv.post(t, "/tagebuch/"+d.ID+"/highlights", url.Values{"quote": {"   "}, "nodeId": {"v1"}})
	_ = res2.Body.Close()
	if l, _ := srv.hs.ListForDocument(context.Background(), "u1", d.ID); len(l) != 1 {
		t.Errorf("blank quote created a highlight: %+v", l)
	}

	res3 := srv.post(t, "/tagebuch/"+d.ID+"/highlights/"+list[0].ID+"/delete", url.Values{})
	_ = res3.Body.Close()
	if l, _ := srv.hs.ListForDocument(context.Background(), "u1", d.ID); len(l) != 0 {
		t.Errorf("highlight survived removal: %+v", l)
	}
}
