package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/sse"
	"github.com/serverkraken/flow/internal/adapter/websession"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSessionEventData(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", time.Now())
	n.Kind = domain.KindRepo
	_, _ = ns.Create(ctx, n)
	s := &Server{GetNode: usecase.GetNode{Nodes: ns}}

	nid := "n1"
	got := s.sessionEventData(ctx, "u1", "sess-1", &nid)
	if got["id"] != "sess-1" || got["node"] != "n1" || got["name"] != "flow" || got["kind"] != "repo" {
		t.Errorf("booked session data = %#v", got)
	}

	if got := s.sessionEventData(ctx, "u1", "sess-2", nil); len(got) != 1 || got["id"] != "sess-2" {
		t.Errorf("unbooked session must carry id only, got %#v", got)
	}

	ghost := "ghost"
	if got := s.sessionEventData(ctx, "u1", "sess-3", &ghost); len(got) != 1 || got["id"] != "sess-3" {
		t.Errorf("unknown node must degrade to id only, got %#v", got)
	}
}

// TestSessionEvents_CarryTarget verifies end-to-end that session.* events
// published on the Bus carry the target's identity when the session is
// booked, and degrade to id-only when it is not. Mirrors the bus-capture
// pattern from TestEditorCreatePublishesEvent (webui_editor_test.go): the
// test subscribes directly to the raw domain.Event (pre activity-log
// conversion) rather than reading the persisted ActivityEntry.
func TestSessionEvents_CarryTarget(t *testing.T) {
	clk := testutil.FakeClock{T: time.Date(2026, 6, 30, 12, 0, 0, 0, time.Local)}
	ids := &testutil.FakeIDGen{}
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeNodeStore()
	users := testutil.NewFakeUserStore()
	u, _ := domain.NewUser("u1", "sub-1", "msoent", "m@x", "Martin")
	_, _ = users.UpsertBySub(context.Background(), u)
	codec := websession.NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	bus := sse.NewBus()
	dos := testutil.NewFakeDayOffStore()
	settings := testutil.NewFakeUserSettingsStore()
	listDayOffs := usecase.ListDayOffs{Store: dos, Settings: settings, Loc: time.Local}

	srv := &Server{
		Bus:               bus,
		Emitter:           sse.NewEmitter(bus, noopActivityStore{}, ids, clk),
		Clock:             clk,
		Users:             users,
		Session:           codec,
		StartSession:      usecase.StartSession{Sessions: ss, Nodes: ps, IDs: ids, Clock: clk},
		StopSession:       usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk},
		GetRunningSession: usecase.GetRunningSession{Sessions: ss},
		GetNode:           usecase.GetNode{Nodes: ps},
		ListNodes:         usecase.ListNodes{Nodes: ps},
		NodeAncestors:     usecase.NodeAncestors{Nodes: ps},
		Stats: usecase.StatsComputer{
			Sessions: ss,
			Nodes:    ps,
			Settings: settings,
			DayOffs:  listDayOffs,
			Clock:    clk,
			Loc:      time.Local,
		},
	}

	n, err := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	if err != nil {
		t.Fatalf("build node: %v", err)
	}
	n.Kind = domain.KindRepo
	if _, err := ps.Create(context.Background(), n); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	cookieVal, err := codec.Issue("u1")
	if err != nil {
		t.Fatalf("issue cookie: %v", err)
	}

	// Case 1: POST /nodes/n1/start (booked) → Data carries node id/name/kind.
	ch, cancel := srv.Bus.Subscribe("u1")
	defer cancel()

	req := httptest.NewRequest(http.MethodPost, "/nodes/n1/start", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /nodes/n1/start status=%d body=%s", rec.Code, rec.Body.String())
	}

	select {
	case ev := <-ch:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("event type=%q, want session.started", ev.Type)
		}
		if ev.Data["node"] != "n1" || ev.Data["name"] != "flow" || ev.Data["kind"] != "repo" {
			t.Errorf("booked start event data = %#v", ev.Data)
		}
		if id, _ := ev.Data["id"].(string); id == "" {
			t.Errorf("booked start event missing id: %#v", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("no session.started event received for booked start")
	}

	// Stop the running session directly via the usecase so the second case
	// starts from an idle state without a second HTTP round trip muddying it.
	rs, ok, err := (usecase.GetRunningSession{Sessions: ss}).Execute(context.Background(), "u1")
	if err != nil || !ok {
		t.Fatalf("expected a running session: ok=%v err=%v", ok, err)
	}
	if _, err := (usecase.StopSession{Sessions: ss, Nodes: ps, Clock: clk}).Execute(context.Background(), "u1", rs.ID, rs.NodeID); err != nil {
		t.Fatalf("stop running session: %v", err)
	}

	// Case 2: POST /ui/home/start (no node) → Data carries id only.
	ch2, cancel2 := srv.Bus.Subscribe("u1")
	defer cancel2()

	req2 := httptest.NewRequest(http.MethodPost, "/ui/home/start", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookieVal})
	rec2 := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST /ui/home/start status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	select {
	case ev := <-ch2:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("event type=%q, want session.started", ev.Type)
		}
		if len(ev.Data) != 1 {
			t.Errorf("unbooked start event data = %#v, want id only", ev.Data)
		}
		if id, _ := ev.Data["id"].(string); id == "" {
			t.Errorf("unbooked start event missing id: %#v", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("no session.started event received for unbooked start")
	}
}
