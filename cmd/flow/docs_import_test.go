package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
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
	// Happy path: proper frontmatter block
	body := "---\nid: notes/Onboarding\ntype: free\n---\n# Onboarding\nbody"
	fm := parseVaultFrontmatter(body)
	if fm.ID != "notes/Onboarding" || fm.Type != "free" {
		t.Fatalf("fm = %+v", fm)
	}

	// No frontmatter: body does not start with "---\n"
	bodyNoFM := "# Onboarding\nbody"
	fm2 := parseVaultFrontmatter(bodyNoFM)
	if fm2 != (vaultFrontmatter{}) {
		t.Fatalf("body with no frontmatter should return zero value, got %+v", fm2)
	}

	// Opening fence but no closing fence: should return zero value
	bodyNoClosed := "---\nid: notes/Onboarding\ntype: free\n# Body without close"
	fm3 := parseVaultFrontmatter(bodyNoClosed)
	if fm3 != (vaultFrontmatter{}) {
		t.Fatalf("body with opening fence but no closing fence should return zero value, got %+v", fm3)
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

func TestProjectResolver_MatchesThenCreates(t *testing.T) {
	var created []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p-existing", Name: "gitlab.com/a/existing"}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/projects":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			created = append(created, in["name"])
			_ = json.NewEncoder(w).Encode(domain.Project{ID: "p-new", Name: in["name"]})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	pr := newProjectResolver(c, false)

	// existing → matched by Name, no create
	id, err := pr.resolve(context.Background(), "gitlab.com/a/existing")
	if err != nil || id == nil || *id != "p-existing" {
		t.Fatalf("match existing: id=%v err=%v", id, err)
	}
	// unknown → created with full path as name
	id2, _ := pr.resolve(context.Background(), "gitlab.com/a/brand-new")
	if id2 == nil || *id2 != "p-new" {
		t.Fatalf("create: id=%v", id2)
	}
	// same path again → cached, no second create
	_, _ = pr.resolve(context.Background(), "gitlab.com/a/brand-new")
	if len(created) != 1 || created[0] != "gitlab.com/a/brand-new" {
		t.Fatalf("created = %v (want exactly one, full path)", created)
	}
	if pr.created != 1 {
		t.Fatalf("pr.created = %d, want 1", pr.created)
	}
	// empty project path → nil id, no error
	if id3, err := pr.resolve(context.Background(), ""); err != nil || id3 != nil {
		t.Fatalf("empty path: id=%v err=%v", id3, err)
	}
}
