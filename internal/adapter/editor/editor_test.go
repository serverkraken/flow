package editor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/editor"
)

func TestEditor_RoundTrip(t *testing.T) {
	// Fake editor: a shell script that appends " EDITED" to the file it's given.
	dir := t.TempDir()
	script := filepath.Join(dir, "fakeed.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ' EDITED' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	e := editor.New()
	out, err := e.Edit(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "hello") || !strings.Contains(string(out), "EDITED") {
		t.Errorf("editor round-trip got %q", out)
	}
}

func TestEditor_NonExistentEditor(t *testing.T) {
	t.Setenv("EDITOR", "/nonexistent/editor-that-does-not-exist")
	e := editor.New()
	_, err := e.Edit(context.Background(), []byte("hello"))
	if err == nil {
		t.Error("expected error for non-existent editor, got nil")
	}
}
