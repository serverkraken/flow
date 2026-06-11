package httpapi_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

func TestDocumentsPutCreateGetUpdateDelete(t *testing.T) {
	api := newTestAPI(t)
	docs := httpapi.NewDocuments(api.Client)

	const docPath = "projects/myproject/ideas.md"

	// Create: If-Match 0
	created, err := docs.Put("", docPath, "# Ideas", "", 0)
	if err != nil {
		t.Fatalf("Put create: %v", err)
	}
	if created.Path != docPath {
		t.Errorf("path = %q, want %q", created.Path, docPath)
	}
	if created.Version != 1 {
		t.Errorf("version = %d, want 1", created.Version)
	}
	if created.Body != "# Ideas" {
		t.Errorf("body = %q, want %q", created.Body, "# Ideas")
	}

	// Get returns the created doc
	got, err := docs.Get("", docPath)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != "# Ideas" {
		t.Errorf("Get body = %q, want %q", got.Body, "# Ideas")
	}
	if got.Version != 1 {
		t.Errorf("Get version = %d, want 1", got.Version)
	}

	// Update: If-Match 1 (current version)
	updated, err := docs.Put("", docPath, "# Ideas v2", "", 1)
	if err != nil {
		t.Fatalf("Put update: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("updated version = %d, want 2", updated.Version)
	}
	if updated.Body != "# Ideas v2" {
		t.Errorf("updated body = %q, want %q", updated.Body, "# Ideas v2")
	}

	// Stale If-Match (1 when version is now 2) → ErrDocumentVersionConflict
	_, err = docs.Put("", docPath, "stale body", "", 1)
	if !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("stale put: expected ErrDocumentVersionConflict, got %v", err)
	}

	// List with q param returns the doc
	entries, err := docs.List("", "projects/myproject/", "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Path == docPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("doc %q not found in List results (got %d entries)", docPath, len(entries))
	}

	// Delete
	if err := docs.Delete("", docPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after Delete → ErrDocumentNotFound
	_, err = docs.Get("", docPath)
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("Get after delete: expected ErrDocumentNotFound, got %v", err)
	}

	// Delete again (idempotent)
	if err := docs.Delete("", docPath); err != nil {
		t.Errorf("Delete idempotent: %v", err)
	}
}

func TestDocumentsRepoKeyAlias(t *testing.T) {
	api := newTestAPI(t)
	docs := httpapi.NewDocuments(api.Client)

	const repoKey = "git:github.com/serverkraken/flow"

	// Create via repoKey alias
	created, err := docs.Put("", "", "# Flow repo note", repoKey, 0)
	if err != nil {
		t.Fatalf("Put repo note create: %v", err)
	}
	if created.RepoKey != repoKey {
		t.Errorf("repo_key = %q, want %q", created.RepoKey, repoKey)
	}
	if created.Version != 1 {
		t.Errorf("version = %d, want 1", created.Version)
	}

	// GetByRepoKey returns the doc
	got, err := docs.GetByRepoKey("", repoKey)
	if err != nil {
		t.Fatalf("GetByRepoKey: %v", err)
	}
	if got.Body != "# Flow repo note" {
		t.Errorf("GetByRepoKey body = %q, want %q", got.Body, "# Flow repo note")
	}
	if got.RepoKey != repoKey {
		t.Errorf("GetByRepoKey repo_key = %q, want %q", got.RepoKey, repoKey)
	}

	// Update via repoKey (If-Match = 1)
	updated, err := docs.Put("", "", "# Flow repo note v2", repoKey, 1)
	if err != nil {
		t.Fatalf("Put repo note update: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("updated version = %d, want 2", updated.Version)
	}
	if updated.Body != "# Flow repo note v2" {
		t.Errorf("updated body = %q, want %q", updated.Body, "# Flow repo note v2")
	}
}

func TestDocumentsGet_NotFound(t *testing.T) {
	api := newTestAPI(t)
	docs := httpapi.NewDocuments(api.Client)

	_, err := docs.Get("", "nonexistent/path/doc.md")
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

func TestDocumentsGetByRepoKey_NotFound(t *testing.T) {
	api := newTestAPI(t)
	docs := httpapi.NewDocuments(api.Client)

	_, err := docs.GetByRepoKey("", "git:github.com/nonexistent/repo")
	if !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

func TestDocumentsList_Empty(t *testing.T) {
	api := newTestAPI(t)
	docs := httpapi.NewDocuments(api.Client)

	entries, err := docs.List("", "nonexistent-prefix-xyzzy/", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty list, got %d entries", len(entries))
	}
}
