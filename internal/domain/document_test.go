package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocument_Validate(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	base := func() domain.Document {
		return domain.Document{ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "docs/architecture", Title: "Arch", CreatedAt: now, UpdatedAt: now}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid free doc: %v", err)
	}
	// bad type
	d := base()
	d.Type = "bogus"
	if err := d.Validate(); err == nil {
		t.Error("bad type should fail")
	}
	// project doc without project id
	d = base()
	d.Type = domain.DocProject
	if err := d.Validate(); err == nil {
		t.Error("project doc without nodeID should fail")
	}
	// project doc with project id
	pid := "proj-1"
	d = base()
	d.Type = domain.DocProject
	d.NodeID = &pid
	if err := d.Validate(); err != nil {
		t.Errorf("valid project doc: %v", err)
	}
	// empty/invalid slug
	d = base()
	d.Path = "Bad Slug!"
	if err := d.Validate(); err == nil {
		t.Error("non-slug path should fail")
	}
	d = base()
	d.Path = ""
	if err := d.Validate(); err == nil {
		t.Error("empty path should fail")
	}
	// daily requires date + derived path
	d = base()
	d.Type = domain.DocDaily
	d.Path = "daily/2026-06-15"
	d.Date = &now
	if err := d.Validate(); err != nil {
		t.Errorf("valid daily: %v", err)
	}
	d = base()
	d.Type = domain.DocDaily // no date
	if err := d.Validate(); err == nil {
		t.Error("daily without date should fail")
	}
}

func TestDailyPath(t *testing.T) {
	d := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	if got := domain.DailyPath(d); got != "daily/2026-06-15" {
		t.Errorf("DailyPath = %q", got)
	}
}

func TestSlugOK(t *testing.T) {
	ok := []string{"docs/architecture", "plans/2026-06-13-rebuild", "a", "x/y/z-1"}
	bad := []string{"", "Bad Slug", "with space", "UPPER", "trailing/", "/leading", "a//b"}
	for _, s := range ok {
		if !domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if domain.SlugOK(s) {
			t.Errorf("SlugOK(%q) = true, want false", s)
		}
	}
}
