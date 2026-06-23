package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewProjectDefaultsActive(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	p, err := NewProject("p1", "u1", "Flow Rebuild", "flow-rebuild", now)
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	if p.Status != ProjectActive {
		t.Fatalf("status = %q, want active", p.Status)
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Fatal("timestamps not set to now")
	}
}

func TestNewProjectValidates(t *testing.T) {
	now := time.Now()
	cases := map[string]struct{ id, owner, name, slug string }{
		"no id":    {"", "u1", "n", "s"},
		"no owner": {"p1", "", "n", "s"},
		"no name":  {"p1", "u1", "", "s"},
		"no slug":  {"p1", "u1", "n", ""},
	}
	for label, c := range cases {
		if _, err := NewProject(c.id, c.owner, c.name, c.slug, now); err == nil {
			t.Fatalf("%s: expected error", label)
		}
	}
}

func TestProjectValidate(t *testing.T) {
	base := func() Project {
		return Project{Name: "Flow", Slug: "flow", Status: ProjectActive}
	}
	t.Run("ok active/paused/archived", func(t *testing.T) {
		for _, st := range []ProjectStatus{ProjectActive, ProjectPaused, ProjectArchived} {
			p := base()
			p.Status = st
			if err := p.Validate(); err != nil {
				t.Errorf("status %q: unexpected error %v", st, err)
			}
		}
	})
	t.Run("missing name", func(t *testing.T) {
		p := base()
		p.Name = ""
		if !errors.Is(p.Validate(), ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for empty name")
		}
	})
	t.Run("missing slug", func(t *testing.T) {
		p := base()
		p.Slug = ""
		if !errors.Is(p.Validate(), ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for empty slug")
		}
	})
	t.Run("bad status", func(t *testing.T) {
		p := base()
		p.Status = "weird"
		if !errors.Is(p.Validate(), ErrInvalidProject) {
			t.Errorf("want ErrInvalidProject for bad status")
		}
	})
}
