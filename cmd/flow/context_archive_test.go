package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

// stubArchiveClient implements archiveClient for unit tests.
type stubArchiveClient struct {
	resolve     func(path string) (string, error)
	setArchived func(id string, a bool) error
}

func (s *stubArchiveClient) resolvePath(path string) (string, error) {
	return s.resolve(path)
}

func (s *stubArchiveClient) SetArchived(id string, a bool) error {
	return s.setArchived(id, a)
}

func TestContextArchive_ApplyFromTSV(t *testing.T) {
	calls := map[string]bool{}
	stub := &stubArchiveClient{
		resolve: func(path string) (string, error) { return "id-" + path, nil },
		setArchived: func(id string, a bool) error { calls[id] = a; return nil },
	}
	tsv := "path\tarchive\nm_done\ty\nm_keep\tn\n"
	if err := runArchive(context.Background(), io.Discard, stub, strings.NewReader(tsv), false); err != nil {
		t.Fatal(err)
	}
	if !calls["id-m_done"] || calls["id-m_keep"] {
		t.Fatalf("apply: %+v", calls)
	}
}

func TestContextArchive_DryRun(t *testing.T) {
	stub := &stubArchiveClient{
		resolve:     func(path string) (string, error) { return "id-" + path, nil },
		setArchived: func(id string, a bool) error { t.Error("SetArchived must not be called in dry-run"); return nil },
	}
	tsv := "path\tarchive\na\ty\nb\ty\nc\tn\n"
	var out strings.Builder
	if err := runArchive(context.Background(), &out, stub, strings.NewReader(tsv), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "2") {
		t.Errorf("expected 'would archive 2', got %q", out.String())
	}
}
