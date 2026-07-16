package artifactfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/artifactfile"
	"github.com/serverkraken/flow/internal/domain"
)

func TestGuessMime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, path, override, want string
	}{
		{name: "override", path: "image.png", override: "application/pdf", want: "application/pdf"},
		{name: "png", path: "image.png", want: "image/png"},
		{name: "pdf", path: "document.pdf", want: "application/pdf"},
		{name: "strip charset", path: "notes.txt", want: "text/plain"},
		{name: "unknown", path: "archive.unknown-flow-type", want: "application/octet-stream"},
		{name: "no extension", path: "README", want: "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := artifactfile.GuessMime(tt.path, tt.override); got != tt.want {
				t.Fatalf("GuessMime(%q, %q) = %q, want %q", tt.path, tt.override, got, tt.want)
			}
		})
	}
}

func TestReadSmallRegularFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := artifactfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("Read = %q, want payload", got)
	}
}

func TestReadRejectsOversizedFileBeforeAllocation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oversized.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, domain.MaxArtifactBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := artifactfile.Read(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized Read error = %v, want size rejection", err)
	}
}

func TestReadRejectsNonRegularFile(t *testing.T) {
	t.Parallel()
	if _, err := artifactfile.Read(t.TempDir()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory Read error = %v, want regular-file rejection", err)
	}
}
