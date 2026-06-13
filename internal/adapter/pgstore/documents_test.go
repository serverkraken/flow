// internal/adapter/pgstore/documents_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

func TestDocuments_PutGetUpdateDelete(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-1")

	created, err := docs.Put(uid, "projects/flow/ideen.md", "# Ideen", "", 0)
	if err != nil || created.Version != 1 {
		t.Fatalf("create: err=%v %+v", err, created)
	}

	// create-only auf existierenden Pfad → Konflikt
	if _, err := docs.Put(uid, "projects/flow/ideen.md", "x", "", 0); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("create on existing: want conflict, got %v", err)
	}

	updated, err := docs.Put(uid, "projects/flow/ideen.md", "# Ideen v2", "", 1)
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: err=%v %+v", err, updated)
	}

	// stale If-Match → Konflikt
	if _, err := docs.Put(uid, "projects/flow/ideen.md", "stale", "", 1); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Errorf("stale update: want conflict, got %v", err)
	}

	got, err := docs.Get(uid, "projects/flow/ideen.md")
	if err != nil || got.Body != "# Ideen v2" {
		t.Fatalf("get: err=%v body=%q", err, got.Body)
	}

	if err := docs.Delete(uid, "projects/flow/ideen.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := docs.Get(uid, "projects/flow/ideen.md"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("after delete: want not found, got %v", err)
	}
	if err := docs.Delete(uid, "projects/flow/ideen.md"); err != nil {
		t.Errorf("delete idempotent: %v", err)
	}
}

func TestDocuments_RepoKeyAlias(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-2")

	key := "git:github.com/serverkraken/flow"
	path := "repos/git%3Agithub.com%2Fserverkraken%2Fflow.md"
	if _, err := docs.Put(uid, path, "repo note", key, 0); err != nil {
		t.Fatalf("put repo note: %v", err)
	}
	got, err := docs.GetByRepoKey(uid, key)
	if err != nil || got.Path != path || got.RepoKey != key {
		t.Fatalf("GetByRepoKey: err=%v %+v", err, got)
	}
	if _, err := docs.GetByRepoKey(uid, "git:github.com/nope/nope"); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Errorf("missing key: want not found, got %v", err)
	}
}

func TestDocuments_ListPrefixAndFTS(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "docs-3")

	seed := map[string]string{
		"daily/2026-06-10.md":     "standup kubernetes cluster",
		"daily/2026-06-11.md":     "postgres migration notes",
		"projects/flow/arch.md":   "kubernetes deployment der webui",
		"projects/flow/random.md": "nichts besonderes",
	}
	for p, body := range seed {
		if _, err := docs.Put(uid, p, body, "", 0); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	byPrefix, err := docs.List(uid, "daily/", "", 0)
	if err != nil || len(byPrefix) != 2 {
		t.Fatalf("prefix list: err=%v len=%d", err, len(byPrefix))
	}

	byQuery, err := docs.List(uid, "", "kubernetes", 0)
	if err != nil || len(byQuery) != 2 {
		t.Fatalf("fts list: err=%v len=%d", err, len(byQuery))
	}

	both, err := docs.List(uid, "projects/", "kubernetes", 0)
	if err != nil || len(both) != 1 {
		t.Fatalf("prefix+fts: err=%v len=%d", err, len(both))
	}

	limited, _ := docs.List(uid, "", "", 2)
	if len(limited) != 2 {
		t.Errorf("limit: want 2, got %d", len(limited))
	}
}
