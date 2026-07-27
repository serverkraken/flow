package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// contains is a small helper used by the pure-helper tests.
func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// --------------------------------------------------------------------------
// Pure-helper tests
// --------------------------------------------------------------------------

func TestParseManifest(t *testing.T) {
	in := strings.NewReader("# comment\nfile\tscope\ttags\tpin\tkeep\n" +
		"feedback_no_icons.md\tglobal\tux,style\ty\ty\n" +
		"project_x.md\tgithub-com-serverkraken-flow\t\t\ty\n" +
		"dead.md\tglobal\t\t\tskip\n")
	rows, err := parseManifest(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3", len(rows))
	}
	if rows[0].Scope != "global" || rows[0].Pin != true || !rows[0].Keep {
		t.Errorf("row0 = %+v", rows[0])
	}
	if len(rows[1].Tags) != 0 {
		t.Errorf("row1 tags = %v want empty", rows[1].Tags)
	}
	if rows[2].Keep {
		t.Errorf("dead.md should be skip")
	}
}

func TestDeriveMemoryDoc(t *testing.T) {
	body := "---\nname: feedback_no_icons\ndescription: avoid colored emoji\nmetadata:\n  type: feedback\n---\nUse monospace glyphs. See [[feedback_no_monoliths]].\n"
	row := manifestRow{File: "feedback_no_icons.md", Scope: "global", Tags: []string{"ux"}, Pin: true, Keep: true}
	doc := deriveMemoryDoc(body, row)
	if doc.Path != "feedback_no_icons" {
		t.Errorf("path = %q", doc.Path)
	}
	if doc.Title != "avoid colored emoji" {
		t.Errorf("title = %q", doc.Title)
	}
	if !contains(doc.Tags, "feedback") || !contains(doc.Tags, "ux") {
		t.Errorf("tags = %v want feedback+ux", doc.Tags)
	}
	if strings.Contains(doc.Body, "---") || !strings.HasPrefix(doc.Body, "Use monospace") {
		t.Errorf("frontmatter not stripped: %q", doc.Body)
	}
	if !doc.Pinned {
		t.Errorf("pinned should be true")
	}
}

// --------------------------------------------------------------------------
// CLI integration test (fake HTTP server)
// --------------------------------------------------------------------------

func TestRunMigrateMemories_UpsertAndDryRun(t *testing.T) {
	const nodeSlug = "github-com-serverkraken-flow"
	const nodeID = "node-abc-123"

	var puts []apiclient.UpsertByPathInput

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{
				{ID: nodeID, Slug: nodeSlug, Name: "github.com/serverkraken/flow"},
			})
		case r.Method == "PUT" && r.URL.Path == "/api/v1/documents/by-path":
			var in apiclient.UpsertByPathInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			puts = append(puts, in)
			_ = json.NewEncoder(w).Encode(apiclient.UpsertByPathResult{ID: "doc-1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Point clientFromStore at the fake server via env vars.
	t.Setenv("FLOW_SERVER_URL", srv.URL)
	t.Setenv("FLOW_TOKEN", "test-token")

	// Build temp memory directory with one memory file.
	dir := t.TempDir()
	memContent := "---\nname: feedback_no_icons\ndescription: avoid colored emoji\nmetadata:\n  type: feedback\n---\nUse monospace glyphs.\n"
	if err := os.WriteFile(filepath.Join(dir, "feedback_no_icons.md"), []byte(memContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build temp manifest pointing the file at the real node slug.
	manifestContent := "file\tscope\ttags\tpin\tkeep\n" +
		"feedback_no_icons.md\t" + nodeSlug + "\tux\ty\ty\n"
	mf := filepath.Join(t.TempDir(), "manifest.tsv")
	if err := os.WriteFile(mf, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- dry-run: assert no PUT is issued ---
	var dryOut bytes.Buffer
	if err := runMigrateMemories(context.Background(), &dryOut, dir, mf, true); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(puts) != 0 {
		t.Fatalf("dry-run issued %d PUT(s), want 0", len(puts))
	}
	if !strings.Contains(dryOut.String(), "dry-run") {
		t.Errorf("dry-run output missing 'dry-run': %q", dryOut.String())
	}

	// --- real run: assert one PUT with correct fields ---
	var realOut bytes.Buffer
	if err := runMigrateMemories(context.Background(), &realOut, dir, mf, false); err != nil {
		t.Fatalf("real run: %v", err)
	}
	if len(puts) != 1 {
		t.Fatalf("real run issued %d PUT(s), want 1", len(puts))
	}
	got := puts[0]
	if got.Path != "feedback_no_icons" {
		t.Errorf("path = %q, want %q", got.Path, "feedback_no_icons")
	}
	if got.NodeID == nil || *got.NodeID != nodeID {
		t.Errorf("projectId = %v, want %q", got.NodeID, nodeID)
	}
	if !contains(got.Tags, "feedback") || !contains(got.Tags, "ux") {
		t.Errorf("tags = %v, want feedback+ux", got.Tags)
	}
	if !got.Pinned {
		t.Errorf("pinned should be true")
	}
	if strings.Contains(got.Body, "---") || !strings.HasPrefix(got.Body, "Use monospace") {
		t.Errorf("body still has frontmatter or wrong prefix: %q", got.Body)
	}
	if got.Type != string(domain.DocMemory) {
		t.Errorf("type = %q, want %q", got.Type, domain.DocMemory)
	}
	if got.Title != "avoid colored emoji" {
		t.Errorf("title = %q, want %q", got.Title, "avoid colored emoji")
	}
}

func TestParseManifest_ArchivedColumn(t *testing.T) {
	in := "file\tscope\ttags\tpin\tkeep\tarchived\n" +
		"a_done.md\tglobal\t\t-\ty\ty\n" +
		"b.md\tglobal\t\t-\ty\t-\n" +
		"c.md\tglobal\t\t-\ty\n" // 5-col legacy row → archived defaults false
	rows, err := parseManifest(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !rows[0].Archived || rows[1].Archived || rows[2].Archived {
		t.Fatalf("archived parse: %+v", rows)
	}
}

func TestRunMigrateMemories_SkipsMemoryMdAndSkipRows(t *testing.T) {
	var puts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/nodes":
			_ = json.NewEncoder(w).Encode([]domain.Node{})
		case r.Method == "PUT" && r.URL.Path == "/api/v1/documents/by-path":
			puts++
			_ = json.NewEncoder(w).Encode(apiclient.UpsertByPathResult{ID: "doc-x"})
		}
	}))
	defer srv.Close()

	t.Setenv("FLOW_SERVER_URL", srv.URL)
	t.Setenv("FLOW_TOKEN", "test-token")

	dir := t.TempDir()
	// MEMORY.md should always be skipped even if in manifest with keep=y
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("big index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a "keep=skip" file
	if err := os.WriteFile(filepath.Join(dir, "old.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestContent := "file\tscope\ttags\tpin\tkeep\n" +
		"MEMORY.md\tglobal\t\t\ty\n" +
		"old.md\tglobal\t\t\tskip\n"
	mf := filepath.Join(t.TempDir(), "manifest.tsv")
	if err := os.WriteFile(mf, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runMigrateMemories(context.Background(), &out, dir, mf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if puts != 0 {
		t.Errorf("expected 0 PUTs, got %d", puts)
	}
	if !strings.Contains(out.String(), "skipped 2") {
		t.Errorf("output: %q", out.String())
	}
}

func TestRunMigrateMemories_UnknownScopeSlugErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/nodes" {
			_ = json.NewEncoder(w).Encode([]domain.Node{}) // no nodes
		}
	}))
	defer srv.Close()

	t.Setenv("FLOW_SERVER_URL", srv.URL)
	t.Setenv("FLOW_TOKEN", "test-token")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestContent := "file\tscope\ttags\tpin\tkeep\n" +
		"note.md\tunknown-slug\t\t\ty\n"
	mf := filepath.Join(t.TempDir(), "manifest.tsv")
	if err := os.WriteFile(mf, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runMigrateMemories(context.Background(), &bytes.Buffer{}, dir, mf, false)
	if err == nil {
		t.Fatal("expected error for unknown slug, got nil")
	}
	if !strings.Contains(err.Error(), "unknown scope slug") {
		t.Errorf("error = %q, want 'unknown scope slug'", err)
	}
}
