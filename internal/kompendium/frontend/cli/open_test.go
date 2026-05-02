package cli_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestOpen_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.store.Seed(mustNote(t, "daily/2026-04-25", domain.TypeDaily, "", "", ""), time.Unix(1, 0))

	if _, _, err := runCmd(t, env.deps, "open", "daily/2026-04-25"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(env.editor.Calls) != 1 {
		t.Errorf("editor not called once: %+v", env.editor.Calls)
	}
}

func TestOpen_NotFound(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "open", "missing/note")
	if !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("got %v, want ErrNoteNotFound", err)
	}
}

func TestOpen_BadID(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "open", "/abs/path")
	if err == nil {
		t.Fatal("expected error for bad ID")
	}
}
