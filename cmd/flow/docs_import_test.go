package main

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestSlugify_ProducesValidSlugs(t *testing.T) {
	cases := map[string]string{
		"daily/2026-04-28":   "daily/2026-04-28",
		"notes/Onboarding":   "notes/onboarding",
		"notes/Jira Service Manager": "notes/jira-service-manager",
		"projects/gitlab.com/dataalliance/sql-credentials/_project": "projects/gitlab-com/dataalliance/sql-credentials/project",
		"notes/product42":    "notes/product42",
	}
	for in, want := range cases {
		got := slugify(in)
		if got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
		if !domain.SlugOK(got) {
			t.Errorf("slugify(%q) = %q is not SlugOK", in, got)
		}
	}
}

func TestParseVaultFrontmatter(t *testing.T) {
	body := "---\nid: notes/Onboarding\ntype: free\n---\n# Onboarding\nbody"
	fm := parseVaultFrontmatter(body)
	if fm.ID != "notes/Onboarding" || fm.Type != "free" {
		t.Fatalf("fm = %+v", fm)
	}
}

func TestTitleFromBody(t *testing.T) {
	body := "---\ntype: free\n---\n\n# Onboarding: Neues Projekt\n\ntext"
	if got := titleFromBody(body); got != "Onboarding: Neues Projekt" {
		t.Fatalf("title = %q", got)
	}
	if got := titleFromBody("no heading here"); got != "" {
		t.Fatalf("want empty title, got %q", got)
	}
}

func TestImportDate(t *testing.T) {
	fm := vaultFrontmatter{Date: "2026-04-28"}
	d := importDate(fm, "daily/whatever.md")
	if d == nil || !d.Equal(time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date from frontmatter = %v", d)
	}
	d2 := importDate(vaultFrontmatter{}, "daily/2026-05-09.md")
	if d2 == nil || d2.Day() != 9 {
		t.Fatalf("date from filename = %v", d2)
	}
	if importDate(vaultFrontmatter{}, "notes/foo.md") != nil {
		t.Fatal("non-date file should yield nil")
	}
}
