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

// TestEditor_Command_RoundTrip exercises Command: seeds a temp file, runs the
// fake editor (a script that appends " APPENDED"), reads back via readback, and
// calls cleanup. It avoids tea.ExecProcess; the test wires stdin/stdout itself.
func TestEditor_Command_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fakecmd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ' APPENDED' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	e := editor.New()
	cmd, readback, cleanup, err := e.Command([]byte("initial"))
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd == nil {
		t.Fatal("Command returned nil cmd")
	}
	defer cleanup()

	// Run the cmd (fake editor appends " APPENDED" to the temp file).
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cmd.Run: %v", err)
	}

	body, err := readback()
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if !strings.Contains(string(body), "initial") {
		t.Errorf("readback missing 'initial', got %q", body)
	}
	if !strings.Contains(string(body), "APPENDED") {
		t.Errorf("readback missing 'APPENDED', got %q", body)
	}
}
