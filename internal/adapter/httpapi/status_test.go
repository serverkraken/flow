package httpapi

import (
	"testing"
	"time"
)

func TestStatus_Snapshot_InitialStateUnknown(t *testing.T) {
	s := newStatus()
	snap := s.Snapshot()
	if snap.State != StateUnknown {
		t.Errorf("initial state = %d, want StateUnknown (%d)", snap.State, StateUnknown)
	}
}

func TestStatus_SetOnline_UpdatesHostAndLastFetched(t *testing.T) {
	s := newStatus()
	before := time.Now()
	s.setOnline("https://flow.example.com")
	snap := s.Snapshot()
	if snap.State != StateOnline {
		t.Errorf("state = %d, want StateOnline", snap.State)
	}
	if snap.Host != "https://flow.example.com" {
		t.Errorf("host = %q, want https://flow.example.com", snap.Host)
	}
	if snap.LastFetched.Before(before) {
		t.Errorf("LastFetched %v is before test start %v", snap.LastFetched, before)
	}
}

func TestStatus_SetOffline(t *testing.T) {
	s := newStatus()
	s.setOnline("https://example.com")
	s.setOffline()
	snap := s.Snapshot()
	if snap.State != StateOffline {
		t.Errorf("state = %d, want StateOffline", snap.State)
	}
}

func TestStatus_SetLoggedOut(t *testing.T) {
	s := newStatus()
	s.setLoggedOut()
	snap := s.Snapshot()
	if snap.State != StateLoggedOut {
		t.Errorf("state = %d, want StateLoggedOut", snap.State)
	}
}

func TestStatus_Outdated_SticksAfterSetOnline(t *testing.T) {
	s := newStatus()
	// Manually put into Outdated state
	s.set(func(sn *StatusSnapshot) { sn.State = StateOutdated })
	// setOnline should NOT overwrite Outdated
	s.setOnline("https://example.com")
	snap := s.Snapshot()
	if snap.State != StateOutdated {
		t.Errorf("state = %d after setOnline, want StateOutdated (sticky)", snap.State)
	}
}

func TestStatus_Changed_FiresOnStateChange(t *testing.T) {
	s := newStatus()
	s.setOffline()
	select {
	case <-s.Changed():
		// good
	default:
		t.Fatal("Changed() channel not fired after setOffline")
	}
}

func TestStatus_Changed_NotFiredWhenStateUnchanged(t *testing.T) {
	s := newStatus()
	s.setOffline()
	// drain
	<-s.Changed()
	// same state again — should not fire again (coalesced)
	s.setOffline()
	select {
	case <-s.Changed():
		t.Fatal("Changed() fired when state did not change")
	default:
		// good
	}
}
