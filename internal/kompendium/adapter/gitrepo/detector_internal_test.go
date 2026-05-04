package gitrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Non-exit errors (e.g. git binary missing) can't be triggered through the
// real `git` binary in tests. The pluggable runFunc lets us cover those
// branches anyway.

func TestDetect_NonExitErrFromRevParse(t *testing.T) {
	t.Parallel()

	forced := errors.New("forced non-exit error")
	d := Detector{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", forced
	}}

	// Use t.TempDir so the upfront cwd-stat check passes — without an
	// existing dir we'd hit the new "no such directory" branch instead
	// of the runFunc path under test.
	_, err := d.Detect(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ports.ErrNotInRepo) {
		t.Error("non-exit error must not be reported as ErrNotInRepo")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err must wrap the forced runner error, got %v", err)
	}
}

func TestDetect_NonExitErrFromRemoteGetURL(t *testing.T) {
	t.Parallel()

	forced := errors.New("forced non-exit error")
	var calls int
	d := Detector{run: func(_ context.Context, _ string, _ ...string) (string, error) {
		calls++
		if calls == 1 {
			return "/repo/root", nil
		}
		return "", forced
	}}

	_, err := d.Detect(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err must wrap the forced runner error, got %v", err)
	}
}
