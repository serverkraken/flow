package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func atHM(h, m int) time.Time {
	return time.Date(2026, 6, 15, h, m, 0, 0, time.UTC)
}

func ptrTime(t time.Time) *time.Time { return &t }

func workSession(id string, startH, startM int, stop *time.Time) domain.WorkSession {
	return domain.WorkSession{ID: id, OwnerID: "u1", Start: atHM(startH, startM), Stop: stop}
}

func TestHasOverlap(t *testing.T) {
	t.Parallel()
	existing := []domain.WorkSession{
		workSession("a", 9, 0, ptrTime(atHM(11, 0))), // 09:00–11:00
		workSession("b", 13, 0, ptrTime(atHM(14, 0))), // 13:00–14:00
	}
	cases := []struct {
		name      string
		start     time.Time
		stop      *time.Time
		excludeID string
		want      bool
	}{
		{"disjoint before", atHM(7, 0), ptrTime(atHM(8, 0)), "", false},
		{"disjoint between", atHM(11, 30), ptrTime(atHM(12, 0)), "", false},
		{"touching edge end", atHM(11, 0), ptrTime(atHM(12, 0)), "", false},   // a.Stop == start
		{"touching edge start", atHM(8, 0), ptrTime(atHM(9, 0)), "", false},   // stop == a.Start
		{"partial overlap left", atHM(8, 30), ptrTime(atHM(9, 30)), "", true},
		{"partial overlap right", atHM(10, 30), ptrTime(atHM(11, 30)), "", true},
		{"contained", atHM(9, 30), ptrTime(atHM(10, 0)), "", true},
		{"contains existing", atHM(8, 0), ptrTime(atHM(15, 0)), "", true},
		{"identical to a", atHM(9, 0), ptrTime(atHM(11, 0)), "", true},
		{"identical but excluded", atHM(9, 0), ptrTime(atHM(11, 0)), "a", false},
		{"running candidate over a", atHM(10, 0), nil, "", true},        // [10:00,+inf) hits a
		{"running candidate after all", atHM(15, 0), nil, "", false},    // [15:00,+inf) hits nothing
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.HasOverlap(existing, c.start, c.stop, c.excludeID); got != c.want {
				t.Errorf("HasOverlap(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestHasOverlap_RunningExisting(t *testing.T) {
	t.Parallel()
	existing := []domain.WorkSession{workSession("run", 9, 0, nil)} // 09:00–+inf
	if !domain.HasOverlap(existing, atHM(10, 0), ptrTime(atHM(10, 30)), "") {
		t.Error("candidate inside a running session must overlap")
	}
	if domain.HasOverlap(existing, atHM(8, 0), ptrTime(atHM(9, 0)), "") {
		t.Error("candidate ending at the running session's start must not overlap")
	}
}
