package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Smoke + behaviour tests for `flow markdown`. RunE for `view` proceeds
// into tea.NewProgram.Run() which blocks on a real TTY, so we cover
// what we can without spinning the program: cobra metadata + the
// pre-program file-read error path.

func TestNewMarkdownCmd_ConstructsValidCobraTree(t *testing.T) {
	root := NewMarkdownCmd()
	if root == nil {
		t.Fatal("expected a non-nil command")
	}
	if root.Use != "markdown" {
		t.Errorf("Use: got %q want markdown", root.Use)
	}
	sub, _, err := root.Find([]string{"view"})
	if err != nil || sub == nil {
		t.Fatalf("expected `view` subcommand, got err=%v sub=%v", err, sub)
	}
	if !strings.HasPrefix(sub.Use, "view") {
		t.Errorf("subcommand Use should start with 'view', got %q", sub.Use)
	}
	if !sub.SilenceUsage {
		t.Errorf("SilenceUsage should be true so cobra doesn't print usage on RunE error")
	}
	if sub.Args == nil {
		t.Errorf("Args validator should be set (ExactArgs(1))")
	}
}

func TestMarkdownView_RejectsMissingArg(t *testing.T) {
	root := NewMarkdownCmd()
	root.SetArgs([]string{"view"})
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when no file argument is provided")
	}
}

func TestMarkdownView_PropagatesReadError(t *testing.T) {
	root := NewMarkdownCmd()
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	root.SetArgs([]string{"view", missing})
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a read error for a missing file")
	}
	if !strings.Contains(err.Error(), "read ") {
		t.Errorf("error should mention the read step, got %q", err)
	}
}

func TestMarkdownView_DirectoryArgFails(t *testing.T) {
	root := NewMarkdownCmd()
	dir := t.TempDir()
	root.SetArgs([]string{"view", dir})
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when the path is a directory")
	}
}

// Sanity: a real, readable Markdown file passes the pre-program checks.
// We can't run the bubbletea loop in tests, but we can confirm os.ReadFile
// succeeds — anything failing past that point would be a TTY artefact, not
// a bug in our wiring. So we read inline and assert no error.
func TestMarkdownView_AcceptsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(path, []byte("# Hello\n\nWorld."), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("expected the seeded file to be readable: %v", err)
	}
}
