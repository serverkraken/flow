package httpapi_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

func newMachine(t *testing.T, srv *testAPI) (*httpapi.Machine, *httpapi.ActiveSessions, *httpapi.Sessions) {
	t.Helper()
	active := httpapi.NewActiveSessions(srv.Client)
	sessions := httpapi.NewSessions(srv.Client)
	m := httpapi.NewMachine(srv.Client, active, sessions)
	return m, active, sessions
}

func TestMachineStartStop(t *testing.T) {
	srv := newTestAPI(t)
	projects := httpapi.NewProjects(srv.Client)
	proj, err := projects.EnsureBySlug("", "machine-test", "machine-test")
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := newMachine(t, srv)

	as, err := m.Start(proj.ID, "work", "note")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if as.ProjectID != proj.ID {
		t.Errorf("got project %s, want %s", as.ProjectID, proj.ID)
	}
	if as.Version < 1 {
		t.Errorf("version after start: %d", as.Version)
	}

	sess, err := m.Stop(proj.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if sess.ProjectID != proj.ID {
		t.Errorf("got project %s, want %s", sess.ProjectID, proj.ID)
	}
}

func TestMachineDoubleStart(t *testing.T) {
	srv := newTestAPI(t)
	projects := httpapi.NewProjects(srv.Client)
	proj, err := projects.EnsureBySlug("", "machine-double", "machine-double")
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := newMachine(t, srv)

	if _, err := m.Start(proj.ID, "", ""); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _, _ = m.Stop(proj.ID) })

	_, err = m.Start(proj.ID, "", "")
	if !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("want ErrActiveSessionConflict, got %v", err)
	}
}

func TestMachineStopWithoutStart(t *testing.T) {
	srv := newTestAPI(t)
	projects := httpapi.NewProjects(srv.Client)
	proj, err := projects.EnsureBySlug("", "machine-stop-empty", "machine-stop-empty")
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := newMachine(t, srv)

	_, err = m.Stop(proj.ID)
	if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("want ErrActiveSessionNotFound, got %v", err)
	}
}

func TestMachinePauseResume(t *testing.T) {
	srv := newTestAPI(t)
	projects := httpapi.NewProjects(srv.Client)
	proj, err := projects.EnsureBySlug("", "machine-pause", "machine-pause")
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := newMachine(t, srv)

	if _, err := m.Start(proj.ID, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = m.Stop(proj.ID) })

	paused, err := m.Pause(proj.ID)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.PausedAt == nil {
		t.Error("PausedAt should be set after Pause")
	}

	resumed, err := m.Resume(proj.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.PausedAt != nil {
		t.Error("PausedAt should be nil after Resume")
	}
}

func TestMachineCorrectStart(t *testing.T) {
	srv := newTestAPI(t)
	projects := httpapi.NewProjects(srv.Client)
	proj, err := projects.EnsureBySlug("", "machine-correct", "machine-correct")
	if err != nil {
		t.Fatal(err)
	}
	m, _, _ := newMachine(t, srv)

	as, err := m.Start(proj.ID, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = m.Stop(proj.ID) })

	newStart := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	corrected, err := m.CorrectStart(proj.ID, newStart)
	if err != nil {
		t.Fatalf("CorrectStart: %v", err)
	}
	if corrected.Version <= as.Version {
		t.Errorf("version should increase after CorrectStart: %d <= %d", corrected.Version, as.Version)
	}
	// started_at should have shifted to approximately newStart
	diff := corrected.StartedAt.Sub(newStart)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("StartedAt not shifted: got %v, want ~%v", corrected.StartedAt, newStart)
	}
}
