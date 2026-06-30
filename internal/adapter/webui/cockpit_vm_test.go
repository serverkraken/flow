package webui_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func TestNodeTimer_States(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	nameOf := func(id string) string { return map[string]string{"y": "Vorhaben Y"}[id] }

	// idle (bookable, nothing running)
	if got := webui.NodeTimer(nil, "x", true, now, nameOf); got.State != webui.TimerIdle {
		t.Errorf("idle: state=%v", got.State)
	}
	// not bookable (branch)
	if got := webui.NodeTimer(nil, "x", false, now, nameOf); got.State != webui.TimerNotBookable {
		t.Errorf("notBookable: state=%v", got.State)
	}
	// running on THIS node
	yid := "x"
	run := domain.WorkSession{ID: "s1", NodeID: &yid, Start: now.Add(-90 * time.Second)}
	if got := webui.NodeTimer(&run, "x", true, now, nameOf); got.State != webui.TimerHere || got.RunningID != "s1" || got.RunningBase != 90 {
		t.Errorf("here: %+v", got)
	}
	// running on ANOTHER node (bound)
	other := "y"
	run2 := domain.WorkSession{ID: "s2", NodeID: &other, Start: now}
	g := webui.NodeTimer(&run2, "x", true, now, nameOf)
	if g.State != webui.TimerOtherBound || g.OtherID != "y" || g.OtherName != "Vorhaben Y" {
		t.Errorf("otherBound: %+v", g)
	}
	// running unbound (started from Home, no node)
	run3 := domain.WorkSession{ID: "s3", NodeID: nil, Start: now}
	if got := webui.NodeTimer(&run3, "x", true, now, nameOf); got.State != webui.TimerUnbound {
		t.Errorf("unbound: state=%v", got.State)
	}
}

func TestNormalizeTab(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "worktime"}, {"bogus", "worktime"}, {"wissen", "wissen"},
		{"struktur", "struktur"}, {"bindings", "bindings"},
	} {
		if got := webui.NormalizeTab(c.in); got != c.want {
			t.Errorf("NormalizeTab(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
