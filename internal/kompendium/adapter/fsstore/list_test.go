package fsstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestStore_List_Empty(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	got, err := s.List(context.Background(), ports.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d entries", len(got))
	}
}

func TestStore_List_FiltersAndOrder(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	puts := []struct {
		id      string
		typ     domain.NoteType
		project string
	}{
		{"daily/2026-04-22", domain.TypeDaily, ""},
		{"daily/2026-04-25", domain.TypeDaily, ""},
		{"projects/github.com/foo/bar/2026-04-25", domain.TypeProject, "github.com/foo/bar"},
		{"projects/github.com/foo/baz/2026-04-25", domain.TypeProject, "github.com/foo/baz"},
		{"notes/setup", domain.TypeFree, ""},
	}
	for i, p := range puts {
		n := makeNote(t, p.id, p.typ, p.project)
		if err := s.Put(ctx, n); err != nil {
			t.Fatal(err)
		}
		// Ensure mtimes are distinct so ordering is deterministic.
		mt := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(filepath.Join(s.Root(), filepath.FromSlash(p.id)+".md"), mt, mt)
	}

	t.Run("all", func(t *testing.T) {
		got, err := s.List(ctx, ports.ListFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(puts) {
			t.Errorf("got %d, want %d", len(got), len(puts))
		}
	})

	t.Run("filter by type", func(t *testing.T) {
		got, err := s.List(ctx, ports.ListFilter{Type: domain.TypeDaily})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 daily notes, got %d", len(got))
		}
		for _, e := range got {
			if e.Meta.Type != domain.TypeDaily {
				t.Errorf("non-daily entry leaked: %v", e.Meta)
			}
		}
	})

	t.Run("filter by project", func(t *testing.T) {
		got, err := s.List(ctx, ports.ListFilter{Project: "github.com/foo/bar"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Meta.Project != "github.com/foo/bar" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got, err := s.List(ctx, ports.ListFilter{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 entries (limit), got %d", len(got))
		}
	})

	t.Run("ordered by mtime desc", func(t *testing.T) {
		got, err := s.List(ctx, ports.ListFilter{})
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(got); i++ {
			if got[i-1].Mtime.Before(got[i].Mtime) {
				t.Errorf("entries not sorted by mtime desc at index %d", i)
			}
		}
	})
}

func TestStore_List_SkipsInvalidIDs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	// File named ".md" — relpath is ".md", TrimSuffix makes it empty, and
	// ParseID rejects empty strings. The walk must silently skip it.
	bad := filepath.Join(s.Root(), ".md")
	if err := os.WriteFile(bad, []byte("---\nid: x\ntype: free\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(context.Background(), ports.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != n.ID {
		t.Errorf("expected only the valid note, got %+v", got)
	}
}

func TestStore_List_SkipsMissingFrontmatter(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	bad := filepath.Join(s.Root(), "bare.md")
	if err := os.WriteFile(bad, []byte("# heading without frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(context.Background(), ports.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got {
		if e.ID == "bare" {
			t.Error("file without frontmatter should have been skipped")
		}
	}
}

func TestStore_List_WalkError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(s.Root(), "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	_, err := s.List(context.Background(), ports.ListFilter{})
	if err == nil {
		t.Error("expected walk error when a subdir is unreadable")
	}
}

func TestStore_List_ReadError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 0o000 does not block reads")
	}
	s := newStore(t)
	n := makeNote(t, "daily/2026-04-25", domain.TypeDaily, "")
	if err := s.Put(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Root(), "daily", "2026-04-25.md")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	_, err := s.List(context.Background(), ports.ListFilter{})
	if err == nil {
		t.Error("expected read error during walk")
	}
}
