package nvimeditor

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// runFunc is unexported, so the env-driven editor resolution and the error
// wrapping live in this internal test file. The external _test.go also
// exercises realRun against a real /bin/true and /bin/false.
//
// Every test that drives env explicitly must clear BOTH $VISUAL and $EDITOR
// — otherwise the developer's ambient environment leaks in (e.g. a global
// VISUAL=nvim spawning a real editor mid-test).

func TestEdit_FallbackToNvim(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	var captured string
	e := Editor{runner: func(_ context.Context, name string, _ ...string) error {
		captured = name
		return nil
	}}
	if err := e.Edit(context.Background(), "/x"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if captured != defaultEditor {
		t.Errorf("fallback got %q, want %q", captured, defaultEditor)
	}
}

func TestEdit_RespectsEditorEnvVar(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vi")

	var captured string
	e := Editor{runner: func(_ context.Context, name string, _ ...string) error {
		captured = name
		return nil
	}}
	if err := e.Edit(context.Background(), "/x"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if captured != "vi" {
		t.Errorf("got %q, want vi", captured)
	}
}

// TestEdit_VISUALWinsOverEDITOR mirrors POSIX convention — $VISUAL takes
// precedence so users with `EDITOR=vi` for plain shell commands and
// `VISUAL=nvim` for interactive editing get the latter for kompendium.
func TestEdit_VISUALWinsOverEDITOR(t *testing.T) {
	t.Setenv("VISUAL", "code")
	t.Setenv("EDITOR", "vi")

	var captured string
	e := Editor{runner: func(_ context.Context, name string, _ ...string) error {
		captured = name
		return nil
	}}
	if err := e.Edit(context.Background(), "/x"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if captured != "code" {
		t.Errorf("got %q, want code", captured)
	}
}

func TestEdit_PathPassedThrough(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vi")

	var capturedArgs []string
	e := Editor{runner: func(_ context.Context, _ string, args ...string) error {
		capturedArgs = args
		return nil
	}}
	if err := e.Edit(context.Background(), "/some/note.md"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != "/some/note.md" {
		t.Errorf("args got %+v, want [/some/note.md]", capturedArgs)
	}
}

// TestEdit_SplitsCommandWithFlags ensures `EDITOR="code -w"` lands as
// argv ["code", "-w", "<path>"] — without splitting, exec.Command would
// look for a binary literally named "code -w" and fail.
func TestEdit_SplitsCommandWithFlags(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code -w")

	var capturedBin string
	var capturedArgs []string
	e := Editor{runner: func(_ context.Context, name string, args ...string) error {
		capturedBin = name
		capturedArgs = args
		return nil
	}}
	if err := e.Edit(context.Background(), "/some/note.md"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if capturedBin != "code" {
		t.Errorf("bin got %q, want code", capturedBin)
	}
	if !reflect.DeepEqual(capturedArgs, []string{"-w", "/some/note.md"}) {
		t.Errorf("args got %+v, want [-w /some/note.md]", capturedArgs)
	}
}

// TestEdit_SplitsQuotedPath covers the rarer case of a quoted segment
// in $EDITOR — e.g. an editor binary inside a directory with spaces.
func TestEdit_SplitsQuotedPath(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `'/path with space/edit' --wait`)

	var capturedBin string
	var capturedArgs []string
	e := Editor{runner: func(_ context.Context, name string, args ...string) error {
		capturedBin = name
		capturedArgs = args
		return nil
	}}
	if err := e.Edit(context.Background(), "/note.md"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if capturedBin != "/path with space/edit" {
		t.Errorf("bin got %q", capturedBin)
	}
	if !reflect.DeepEqual(capturedArgs, []string{"--wait", "/note.md"}) {
		t.Errorf("args got %+v", capturedArgs)
	}
}

// TestEdit_UnterminatedQuoteErrors covers the malformed-env case — the
// caller should see a clear error rather than a confusing exec failure.
func TestEdit_UnterminatedQuoteErrors(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `'unclosed`)

	e := Editor{runner: func(_ context.Context, _ string, _ ...string) error { return nil }}
	if err := e.Edit(context.Background(), "/x"); err == nil {
		t.Error("unterminated quote in EDITOR should surface as an error")
	}
}

func TestEdit_WrapsRunnerError(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vi")

	forced := errors.New("forced runner error")
	e := Editor{runner: func(_ context.Context, _ string, _ ...string) error {
		return forced
	}}
	err := e.Edit(context.Background(), "/x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, forced) {
		t.Errorf("err must wrap forced, got %v", err)
	}
}

// TestCmd_BuildsExecCmd covers Editor.Cmd, the variant the browse TUI
// hands to tea.ExecProcess. Same env resolution as Edit, but returns an
// unstarted *exec.Cmd whose Args we can inspect.
func TestCmd_BuildsExecCmd(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim -O")

	cmd := Editor{}.Cmd("/note.md")
	if cmd == nil {
		t.Fatal("Cmd should never return nil")
	}
	// Args[0] is the resolved binary, then the parsed flags + path.
	want := []string{"vim", "-O", "/note.md"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Cmd.Args got %+v, want %+v", cmd.Args, want)
	}
}

func TestCmd_FallsBackOnMalformedEnv(t *testing.T) {
	t.Setenv("VISUAL", `'unclosed`)
	t.Setenv("EDITOR", "")

	cmd := Editor{}.Cmd("/note.md")
	if cmd == nil {
		t.Fatal("Cmd should degrade gracefully, not return nil")
	}
	// The fallback path uses the default editor with just the note path.
	want := []string{defaultEditor, "/note.md"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Cmd.Args got %+v, want %+v", cmd.Args, want)
	}
}

func TestCmd_HonorsVISUAL(t *testing.T) {
	t.Setenv("VISUAL", "code -w")
	t.Setenv("EDITOR", "vi")

	cmd := Editor{}.Cmd("/note.md")
	want := []string{"code", "-w", "/note.md"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Cmd.Args got %+v, want %+v", cmd.Args, want)
	}
}
