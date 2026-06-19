package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestProjectResolver_DryRunCreatesNoApi(t *testing.T) {
	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST" && r.URL.Path == "/api/v1/projects":
			postCount++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(domain.Project{ID: "p-new", Name: in["name"]})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	pr := newProjectResolver(c, true) // dry-run=true

	// resolve unknown project in dry-run mode
	id, err := pr.resolve(context.Background(), "unknown/project")
	if err != nil {
		t.Fatalf("resolve in dry-run: err=%v", err)
	}
	if id == nil {
		t.Fatal("resolve in dry-run: want non-nil id")
	}

	// assert no POST was made
	if postCount != 0 {
		t.Fatalf("dry-run made %d POST requests, want 0", postCount)
	}

	// assert created counter incremented
	if pr.created != 1 {
		t.Fatalf("pr.created = %d, want 1", pr.created)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunImport_ImportsSkipsAndDryRun(t *testing.T) {
	var posts int
	existing := []domain.Document{{ID: "d-exist", Path: "notes/existing"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/documents":
			_ = json.NewEncoder(w).Encode(existing)
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.URL.Path == "/api/v1/documents/import":
			posts++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(domain.Document{ID: "new"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	writeFile(t, dir, "daily/2026-04-28.md", "---\nid: daily/2026-04-28\ntype: daily\ndate: \"2026-04-28\"\n---\n# 2026-04-28\n")
	writeFile(t, dir, "notes/Onboarding.md", "---\nid: notes/Onboarding\ntype: free\n---\n# Onboarding\n")
	writeFile(t, dir, "notes/existing.md", "---\nid: notes/existing\ntype: free\n---\n# Existing\n") // path already on server

	// dry-run writes nothing
	st, err := runImport(context.Background(), c, dir, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("dry-run posted %d (want 0)", posts)
	}
	if st.imported != 2 || st.skipped != 1 {
		t.Fatalf("dry-run stats = %+v (want imported 2, skipped 1)", st)
	}

	// real run imports the 2 new, skips the existing
	posts = 0
	st, err = runImport(context.Background(), c, dir, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 2 || st.imported != 2 || st.skipped != 1 {
		t.Fatalf("run posts=%d stats=%+v (want 2 imported, 1 skipped)", posts, st)
	}
}

func TestRunImport_DailyTitleIsDate(t *testing.T) {
	var importedDocs []apiclient.ImportDocumentInput
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/documents":
			_ = json.NewEncoder(w).Encode([]domain.Document{})
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{})
		case r.Method == "POST" && r.URL.Path == "/api/v1/documents/import":
			var in apiclient.ImportDocumentInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			importedDocs = append(importedDocs, in)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(domain.Document{ID: "new"})
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")

	dir := t.TempDir()
	// Daily note with date in frontmatter; first H1 is a section heading, not the date
	writeFile(t, dir, "daily/2026-05-11.md", "---\nid: daily/2026-05-11\ntype: daily\ndate: \"2026-05-11\"\n---\n# Tickets\n\nSome daily content")

	st, err := runImport(context.Background(), c, dir, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.imported != 1 {
		t.Fatalf("expected 1 imported, got %d", st.imported)
	}
	if len(importedDocs) != 1 {
		t.Fatalf("expected 1 imported document, got %d", len(importedDocs))
	}
	doc := importedDocs[0]
	if doc.Title != "2026-05-11" {
		t.Errorf("daily title = %q, want %q", doc.Title, "2026-05-11")
	}
}
