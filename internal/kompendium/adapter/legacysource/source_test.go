package legacysource_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/legacysource"
)

func TestListDailyNotes_OnlyMatchesDateNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "2026-04-22.md", "daily body\n")
	writeFile(t, dir, "2026-04-25.md", "another daily\n")
	writeFile(t, dir, "stray.md", "not a daily")
	writeFile(t, dir, "README.md", "ignore me")
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := legacysource.New().ListDailyNotes(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListDailyNotes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	for _, d := range got {
		if d.Date == "" || d.Path == "" || len(d.Body) == 0 {
			t.Errorf("incomplete entry: %+v", d)
		}
	}
}

func TestListDailyNotes_MissingDirIsEmpty(t *testing.T) {
	t.Parallel()
	got, err := legacysource.New().ListDailyNotes(context.Background(), "/this-dir-does-not-exist-xyz")
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero entries, got %+v", got)
	}
}

func TestListProjectNotes_ExtractsRemote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "dotfiles-008be364.md",
		"# dotfiles\n\nRemote: git@github.com:serverkraken/dotfiles.git\nPfad:   /repos/dotfiles\n\n---\n\nbody\n")
	writeFile(t, dir, "no-remote-12345678.md",
		"# nothing\n\nRemote: (kein Remote)\nPfad:   /repos/x\n\n---\n\nbody\n")
	writeFile(t, dir, "missing-header.md",
		"plain content with no Remote header\n")
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := legacysource.New().ListProjectNotes(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListProjectNotes: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}

	byPath := map[string]string{}
	for _, p := range got {
		byPath[filepath.Base(p.Path)] = p.URL
	}
	if byPath["dotfiles-008be364.md"] != "git@github.com:serverkraken/dotfiles.git" {
		t.Errorf("dotfiles URL got %q", byPath["dotfiles-008be364.md"])
	}
	if byPath["no-remote-12345678.md"] != "" {
		t.Errorf("(kein Remote) placeholder must yield empty URL, got %q", byPath["no-remote-12345678.md"])
	}
	if byPath["missing-header.md"] != "" {
		t.Errorf("missing-header URL must be empty, got %q", byPath["missing-header.md"])
	}
}

func TestListProjectNotes_MissingDirIsEmpty(t *testing.T) {
	t.Parallel()
	got, err := legacysource.New().ListProjectNotes(context.Background(), "/no-such-dir-xyz")
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero entries, got %+v", got)
	}
}

func TestListDailyNotes_UnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	dir := t.TempDir()
	writeFile(t, dir, "2026-04-25.md", "body")
	bad := filepath.Join(dir, "2026-04-25.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod 0o000 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	_, err := legacysource.New().ListDailyNotes(context.Background(), dir)
	if err == nil {
		t.Error("expected read error for unreadable daily file")
	}
}

func TestListProjectNotes_UnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	dir := t.TempDir()
	writeFile(t, dir, "x-12345678.md", "Remote: git@host:f/b.git\n")
	bad := filepath.Join(dir, "x-12345678.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod 0o000 not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	_, err := legacysource.New().ListProjectNotes(context.Background(), dir)
	if err == nil {
		t.Error("expected read error for unreadable project file")
	}
}

func TestListDailyNotes_UnreadableDir(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := legacysource.New().ListDailyNotes(context.Background(), dir)
	if err == nil {
		t.Error("expected error reading unreadable directory")
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
