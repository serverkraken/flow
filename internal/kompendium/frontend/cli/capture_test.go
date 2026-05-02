package cli_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestCapture_AppendsBullet(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	stdout, _, err := runCmd(t, env.deps, "capture", "got the build green")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !strings.Contains(stdout, "Created daily and captured") && !strings.Contains(stdout, "Captured") {
		t.Errorf("missing acknowledgement, stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "got the build green") {
		t.Errorf("captured text not echoed: %q", stdout)
	}
	if !strings.Contains(stdout, "daily/2026-04-25") {
		t.Errorf("daily ID not echoed: %q", stdout)
	}
}

func TestCapture_JoinsArgs(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	stdout, _, err := runCmd(t, env.deps, "capture", "two", "words")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !strings.Contains(stdout, "two words") {
		t.Errorf("multiple args should be joined with spaces, got %q", stdout)
	}
}

func TestCapture_RejectsEmpty(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	_, _, err := runCmd(t, env.deps, "capture", "   ")
	if err == nil {
		t.Error("capture with whitespace-only text should fail")
	}
	_ = usecase.ErrCaptureEmpty // ensure import is anchored to a real symbol
}

func TestCapture_HelpListsTheSubcommand(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "capture", "--help")
	if err != nil {
		t.Fatalf("capture --help: %v", err)
	}
	if !strings.Contains(stdout, "timestamped bullet") && !strings.Contains(stdout, "Quick capture") {
		t.Errorf("help text missing description: %q", stdout)
	}
}
