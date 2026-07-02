package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// TestTimerWidget_Lifecycle drives the global shell timer widget end-to-end
// through its five fragment endpoints (Kristall K1: ONE global home for the
// running timer). It reuses the cockpitTestServer harness (webui_cockpit_test.go)
// and captures raw domain.Event values off the SSE bus — mirroring the
// bus-capture pattern in TestSessionEvents_CarryTarget (session_event_test.go)
// — to assert that every mutation emits via sessionEventData with the node's
// identity attached, not just a bare id.
func TestTimerWidget_Lifecycle(t *testing.T) {
	c := newCockpitTestServer(t)
	c.seedNode(t, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Slug: "flow", Kind: domain.KindRepo, Color: "cyan"})
	c.seedNode(t, domain.Node{ID: "n2", OwnerID: "u1", Name: "homelab", Slug: "homelab", Kind: domain.KindRepo, Color: "purple"})

	// ---- 1) GET /ui/timer (idle) ----
	rec := c.do(t, "GET", "/ui/timer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ui/timer (idle): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	idleBody := rec.Body.String()
	for _, want := range []string{`hx-post="/ui/timer/start"`, "Timer starten", `value="n1"`, `name="newProject"`} {
		if !strings.Contains(idleBody, want) {
			t.Errorf("idle widget missing %q, got: %.800s", want, idleBody)
		}
	}

	// ---- 2) POST /ui/timer/start {projectId: n1} ----
	ch, cancel := c.srv.Bus.Subscribe("u1")
	defer cancel()

	rec = c.do(t, "POST", "/ui/timer/start", map[string]string{"projectId": "n1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/start n1: status %d body=%.400s", rec.Code, rec.Body.String())
	}
	runningBody := rec.Body.String()
	for _, want := range []string{"data-timer", "flow", `href="/nodes/n1"`, `hx-post="/ui/timer/stop"`, "Stoppen"} {
		if !strings.Contains(runningBody, want) {
			t.Errorf("running widget missing %q, got: %.800s", want, runningBody)
		}
	}

	select {
	case ev := <-ch:
		if ev.Type != domain.EventSessionStarted {
			t.Fatalf("event type=%q, want session.started", ev.Type)
		}
		if ev.Data["node"] != "n1" || ev.Data["name"] != "flow" || ev.Data["kind"] != "repo" {
			t.Errorf("start event Data (sessionEventData) = %#v, want node/name/kind for n1", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("no session.started event received for POST /ui/timer/start")
	}

	// Route wiring smoke check for GET /ui/timer/chip while a session is running.
	chipRec := c.do(t, "GET", "/ui/timer/chip", nil)
	if chipRec.Code != http.StatusOK {
		t.Fatalf("GET /ui/timer/chip: status %d body=%.400s", chipRec.Code, chipRec.Body.String())
	}
	chipBody := chipRec.Body.String()
	for _, want := range []string{"data-mini-timer", `data-dialog-open="timer-sheet"`} {
		if !strings.Contains(chipBody, want) {
			t.Errorf("running chip missing %q, got: %.800s", want, chipBody)
		}
	}

	// Double-submit guard: starting again while n1 is already running must
	// hit domain.ErrAlreadyRunning and simply re-render the real (running)
	// state — never the contradictory "no timer running" banner over a live
	// clock (self-review finding: the naive "always show timer.idle on any
	// start error" would do exactly that).
	rec = c.do(t, "POST", "/ui/timer/start", map[string]string{"projectId": "n2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/start (already running): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	dupBody := rec.Body.String()
	if !strings.Contains(dupBody, "data-timer") || !strings.Contains(dupBody, "flow") {
		t.Errorf("double-submit start must keep showing the real running session (n1/flow), got: %.800s", dupBody)
	}
	if strings.Contains(dupBody, "Kein Timer läuft") {
		t.Errorf("double-submit start must NOT render the contradictory timer.idle banner over a live clock, got: %.800s", dupBody)
	}

	// ---- 3) POST /ui/timer/switch {projectId: n2} — stops n1, starts n2 ----
	ch2, cancel2 := c.srv.Bus.Subscribe("u1")
	defer cancel2()

	rec = c.do(t, "POST", "/ui/timer/switch", map[string]string{"projectId": "n2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/switch n2: status %d body=%.400s", rec.Code, rec.Body.String())
	}
	if rs, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); !ok || rs.NodeID == nil || *rs.NodeID != "n2" {
		t.Fatalf("after switch expected running on n2, got ok=%v rs=%+v", ok, rs)
	}

	// sse.Emitter.Emit publishes the raw event AND a trailing activity.logged
	// echo per call (internal/adapter/sse/emitter.go), so a switch (stop+start)
	// yields 4 bus messages, not 2. Drain until both session.* events are seen,
	// tolerating (and ignoring) the interleaved activity.logged echoes.
	var sawStopped, sawStarted bool
	deadline := time.After(2 * time.Second)
	for !sawStopped || !sawStarted {
		select {
		case ev := <-ch2:
			switch ev.Type {
			case domain.EventSessionStopped:
				sawStopped = true
				if ev.Data["node"] != "n1" || ev.Data["name"] != "flow" || ev.Data["kind"] != "repo" {
					t.Errorf("switch stop event should still carry the old node n1/flow/repo, got %#v", ev.Data)
				}
			case domain.EventSessionStarted:
				sawStarted = true
				if ev.Data["node"] != "n2" || ev.Data["name"] != "homelab" || ev.Data["kind"] != "repo" {
					t.Errorf("switch start event should carry n2/homelab/repo, got %#v", ev.Data)
				}
			case domain.EventActivityLogged:
				// expected echo from the Emitter — not a session mutation.
			default:
				t.Errorf("unexpected event type during switch: %q", ev.Type)
			}
		case <-deadline:
			t.Fatalf("switch: timed out waiting for both events (stopped=%v started=%v)", sawStopped, sawStarted)
		}
	}

	// ---- 4) POST /ui/timer/stop {} on a BOUND session (n2) → idle again ----
	rec = c.do(t, "POST", "/ui/timer/stop", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/stop (bound, no projectId): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	idleAgain := rec.Body.String()
	if !strings.Contains(idleAgain, "Timer starten") || strings.Contains(idleAgain, "data-timer") {
		t.Errorf("stop on bound session must re-render idle widget, got: %.800s", idleAgain)
	}
	if _, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); ok {
		t.Fatalf("session still running after bound stop")
	}

	// ---- 5) Unbound flow ----
	// 5a) start without projectId → running-unbound fragment.
	rec = c.do(t, "POST", "/ui/timer/start", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/start (unbound): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	unboundBody := rec.Body.String()
	for _, want := range []string{"ohne Projekt", "Zum Stoppen Projekt wählen", "data-timer"} {
		if !strings.Contains(unboundBody, want) {
			t.Errorf("unbound running widget missing %q, got: %.800s", want, unboundBody)
		}
	}

	// 5b) stop WITHOUT projectId → re-renders with vm.Err (timer.needNode),
	// session STILL running (the whole point of the unbound guard).
	rec = c.do(t, "POST", "/ui/timer/stop", map[string]string{})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/stop (unbound, no projectId): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	errBody := rec.Body.String()
	if !strings.Contains(errBody, "Zum Stoppen Projekt wählen") || !strings.Contains(errBody, `role="alert"`) {
		t.Errorf("unbound stop without projectId must render inline timer.needNode error, got: %.800s", errBody)
	}
	if !strings.Contains(errBody, "data-timer") {
		t.Errorf("unbound stop without projectId must keep showing the running timer, got: %.800s", errBody)
	}
	if _, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); !ok {
		t.Fatalf("session must still be running after a rejected unbound stop")
	}

	// 5c) stop WITH projectId → idle.
	rec = c.do(t, "POST", "/ui/timer/stop", map[string]string{"projectId": "n1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /ui/timer/stop (unbound, with projectId): status %d body=%.400s", rec.Code, rec.Body.String())
	}
	finalBody := rec.Body.String()
	if !strings.Contains(finalBody, "Timer starten") || strings.Contains(finalBody, "data-timer") {
		t.Errorf("stop with projectId on unbound session must re-render idle widget, got: %.800s", finalBody)
	}
	if _, ok, _ := (usecase.GetRunningSession{Sessions: c.ss}).Execute(context.Background(), "u1"); ok {
		t.Fatalf("session still running after binding stop")
	}
}
