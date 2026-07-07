package httpserver_test

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

var sinceBaseRe = regexp.MustCompile(`data-base="(\d+)" data-since="(\d+)"`)

// TestLiveTick_AbsoluteAnchorSharedAcrossSurfaces pins the Soenne-Live-Gate
// finding (L4): the Schreibtisch-Jetzt clock and the Zeit-LIVE clock ticked
// with up to ~1s offset because each element counted RELATIVE to its own
// render/bind moment (data-base truncates the sub-second phase, boundStart
// differs per surface). The fix anchors every live clock ABSOLUTELY via
// data-since (unix epoch of the effective start = now - elapsed): all
// surfaces compute floor(now)-since and flip on the same wall-clock second.
// The invariant is NOT a fixed epoch but (a) every running clock carries an
// anchor, (b) the anchor is IDENTICAL across surfaces for the same session,
// (c) anchor + rendered base equals the server clock's epoch.
func TestLiveTick_AbsoluteAnchorSharedAcrossSurfaces(t *testing.T) {
	srv := newWorktimeTestServer(t)
	nodeID := srv.seedBookableNode(t, "repo-a")
	srv.startSession(t, "u1", &nodeID)

	clockEpoch := time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local).Unix()

	anchors := map[string]int64{}
	for _, path := range []string{"/", "/zeit"} {
		body := getBody(t, srv, "u1", path)
		m := sinceBaseRe.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s: running live clock must carry data-base + data-since (absolute anchor); none found", path)
		}
		base, _ := strconv.ParseInt(m[1], 10, 64)
		since, _ := strconv.ParseInt(m[2], 10, 64)
		if since+base != clockEpoch {
			t.Errorf("%s: anchor inconsistent: data-since(%d) + data-base(%d) = %d, want server clock epoch %d", path, since, base, since+base, clockEpoch)
		}
		anchors[path] = since
	}
	if anchors["/"] != anchors["/zeit"] {
		t.Errorf("surfaces disagree on the absolute anchor: / has %d, /zeit has %d — clocks would flip on different wall-clock seconds", anchors["/"], anchors["/zeit"])
	}
}
