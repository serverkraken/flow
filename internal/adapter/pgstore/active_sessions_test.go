// internal/adapter/pgstore/active_sessions_test.go
package pgstore_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestActive_StartStopCycle(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-1")
	pid := mustProject(t, uid, "active-work")

	as, err := a.Start(uid, pid, time.Time{}, "laptop", 0, "deep", "fokus")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if as.Version != 1 || as.Tag != "deep" || as.PausedAt != nil {
		t.Fatalf("unexpected active: %+v", as)
	}

	// Doppel-Start auf dasselbe Projekt → Konflikt (Spec §7: 409)
	if _, err := a.Start(uid, pid, time.Time{}, "phone", 0, "", ""); !errors.Is(err, ports.ErrActiveSessionConflict) {
		t.Errorf("double start: want conflict, got %v", err)
	}

	sess, err := a.Stop(uid, pid, as.Version, "", "")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if sess.Elapsed <= 0 || sess.Tag != "deep" {
		t.Errorf("stopped session: %+v", sess)
	}
	if sess.Version != 1 {
		t.Errorf("session version after stop: got %d want 1", sess.Version)
	}

	// Stop ohne aktive Session → NotFound
	if _, err := a.Stop(uid, pid, 1, "", ""); !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("double stop: want not found, got %v", err)
	}
}

func TestActive_PauseResumeIdempotent(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-2")
	pid := mustProject(t, uid, "pause-work")

	started, _ := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")

	paused, err := a.Pause(uid, pid)
	if err != nil || paused.PausedAt == nil {
		t.Fatalf("Pause: err=%v PausedAt=%v", err, paused.PausedAt)
	}
	if paused.Version <= started.Version {
		t.Errorf("Pause must bump version: %d <= %d", paused.Version, started.Version)
	}

	// Pause auf pausierter Session → idempotent, gleicher Zustand
	paused2, err := a.Pause(uid, pid)
	if err != nil || paused2.PausedAt == nil || paused2.Version != paused.Version {
		t.Errorf("Pause idempotent: err=%v %+v", err, paused2)
	}

	resumed, err := a.Resume(uid, pid)
	if err != nil || resumed.PausedAt != nil {
		t.Fatalf("Resume: err=%v PausedAt=%v", err, resumed.PausedAt)
	}
	if resumed.PauseTotal <= 0 {
		t.Errorf("PauseTotal after resume: %v", resumed.PauseTotal)
	}

	// Resume ohne Pause → idempotent
	resumed2, err := a.Resume(uid, pid)
	if err != nil || resumed2.Version != resumed.Version {
		t.Errorf("Resume idempotent: err=%v %+v", err, resumed2)
	}

	// Pause/Resume ohne aktive Session → NotFound
	if _, err := a.Pause(uid, "00000000-0000-0000-0000-000000000009"); !errors.Is(err, ports.ErrActiveSessionNotFound) {
		t.Errorf("pause w/o active: want not found, got %v", err)
	}
}

func TestActive_StopDuringPauseEndsPause(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))
	uid := mustUser(t, "active-3")
	pid := mustProject(t, uid, "pausestop")

	started, _ := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")
	paused, _ := a.Pause(uid, pid)

	sess, err := a.Stop(uid, pid, paused.Version, "", "")
	if err != nil {
		t.Fatalf("Stop while paused: %v", err)
	}
	// elapsed = Wandzeit − Pausen; bei sofortigem Pause→Stop nahe 0, nie negativ
	if sess.Elapsed < 0 {
		t.Errorf("elapsed negative: %v", sess.Elapsed)
	}
	wall := sess.Stop.Sub(started.StartedAt)
	if sess.Elapsed > wall {
		t.Errorf("elapsed %v exceeds wall time %v", sess.Elapsed, wall)
	}
}

func TestActiveSessionsCorrectStart(t *testing.T) {
	t.Parallel()
	a := pgstore.NewActiveSessions(testStore, pgstore.NewSessions(testStore), pgstore.NewSettings(testStore))

	tests := []struct {
		name         string
		setup        func(uid, pid string) error
		startedAt    func() time.Time
		wantErr      bool
		wantSentinel error
		checkVersion func(t *testing.T, before, after int64)
	}{
		{
			name: "shifts started_at and increments version",
			setup: func(uid, pid string) error {
				_, err := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")
				return err
			},
			startedAt: func() time.Time { return time.Now().UTC().Add(-1 * time.Hour) },
			wantErr:   false,
			checkVersion: func(t *testing.T, before, after int64) {
				if after <= before {
					t.Errorf("version should increase: %d <= %d", after, before)
				}
			},
		},
		{
			name:         "non-existent session returns ErrActiveSessionNotFound",
			setup:        func(_, _ string) error { return nil },
			startedAt:    func() time.Time { return time.Now().UTC().Add(-1 * time.Hour) },
			wantErr:      true,
			wantSentinel: ports.ErrActiveSessionNotFound,
		},
		{
			name: "future time returns error",
			setup: func(uid, pid string) error {
				_, err := a.Start(uid, pid, time.Time{}, "mac", 0, "", "")
				return err
			},
			startedAt: func() time.Time { return time.Now().UTC().Add(1 * time.Hour) },
			wantErr:   true,
		},
	}

	for i, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uid := mustUser(t, fmt.Sprintf("cs-user-%d", i))
			pid := mustProject(t, uid, fmt.Sprintf("cs-proj-%d", i))

			if err := tc.setup(uid, pid); err != nil {
				t.Fatalf("setup: %v", err)
			}

			// Capture version before correction (if session exists)
			var versionBefore int64
			if cur, err := a.Get(uid, pid); err == nil {
				versionBefore = cur.Version
			}

			corrected, err := a.CorrectStart(uid, pid, tc.startedAt())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantSentinel != nil && !errors.Is(err, tc.wantSentinel) {
					t.Errorf("want sentinel %v, got %v", tc.wantSentinel, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkVersion != nil {
				tc.checkVersion(t, versionBefore, corrected.Version)
			}
			// Verify started_at was updated
			diff := corrected.StartedAt.Sub(tc.startedAt().Truncate(time.Second))
			if diff < 0 {
				diff = -diff
			}
			if diff > 2*time.Second {
				t.Errorf("StartedAt not updated: got %v, want ~%v", corrected.StartedAt, tc.startedAt())
			}
			// Cleanup: stop the session
			_, _ = a.Stop(uid, pid, corrected.Version, "", "")
		})
	}
}
