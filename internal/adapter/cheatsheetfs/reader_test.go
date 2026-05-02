package cheatsheetfs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/cheatsheetfs"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.CheatsheetReader = (*cheatsheetfs.Reader)(nil)

func TestLoad_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cheatsheet.md")
	want := "# Cheatsheet\n\nstuff\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := cheatsheetfs.New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoad_MissingFile_PropagatesError(t *testing.T) {
	r := cheatsheetfs.New(filepath.Join(t.TempDir(), "missing.md"))
	_, err := r.Load()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want NotExist, got %v", err)
	}
}
