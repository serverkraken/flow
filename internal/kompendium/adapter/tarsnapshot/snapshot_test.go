package tarsnapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/tarsnapshot"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestExportImport_Roundtrip(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeFile(t, src, "daily/2026-04-25.md", "daily body\n")
	writeFile(t, src, "projects/foo/2026-04-25.md", "project body\n")
	writeFile(t, src, "notes/setup.md", "setup body\n")

	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := tarsnapshot.New().Export(context.Background(), src, out); err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := t.TempDir()
	if err := tarsnapshot.New().Import(context.Background(), out, dst, ports.ConflictAbort); err != nil {
		t.Fatalf("Import: %v", err)
	}

	for _, want := range []string{"daily/2026-04-25.md", "projects/foo/2026-04-25.md", "notes/setup.md"} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(want)))
		if err != nil {
			t.Errorf("missing %q: %v", want, err)
			continue
		}
		if len(got) == 0 {
			t.Errorf("%q is empty after import", want)
		}
	}
}

// --- helpers shared with export_test.go / import_test.go -----------------

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
