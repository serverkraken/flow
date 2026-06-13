// internal/adapter/pgstore/documents_revisions_test.go
package pgstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

type revisionRow struct {
	DocumentID string
	Path       string
	Body       string
	Version    int64
	Deleted    bool
}

// revisionsFor liest alle Revisionszeilen eines Pfads in Insert-Reihenfolge.
// Direkt über den Pool — es gibt bewusst keinen Port dafür (Phase 1: psql-only).
func revisionsFor(t *testing.T, userID, path string) []revisionRow {
	t.Helper()
	rows, err := testStore.Pool().Query(context.Background(), `
		SELECT document_id, path, body, version, deleted
		FROM document_revisions
		WHERE user_id = $1 AND path = $2
		ORDER BY id ASC`, userID, path)
	if err != nil {
		t.Fatalf("query revisions: %v", err)
	}
	defer rows.Close()
	var out []revisionRow
	for rows.Next() {
		var r revisionRow
		if err := rows.Scan(&r.DocumentID, &r.Path, &r.Body, &r.Version, &r.Deleted); err != nil {
			t.Fatalf("scan revision: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func TestDocuments_Put_WritesRevisionPerSave(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-1")
	const p = "projects/flow/revisions.md"

	created, err := docs.Put(uid, p, "v1 body", "", 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := docs.Put(uid, p, "v2 body", "", created.Version); err != nil {
		t.Fatalf("update: %v", err)
	}

	revs := revisionsFor(t, uid, p)
	if len(revs) != 2 {
		t.Fatalf("revisions: want 2, got %d (%+v)", len(revs), revs)
	}
	if revs[0].Body != "v1 body" || revs[0].Version != 1 || revs[0].Deleted {
		t.Errorf("rev[0]: want v1/&body/false, got %+v", revs[0])
	}
	if revs[1].Body != "v2 body" || revs[1].Version != 2 || revs[1].Deleted {
		t.Errorf("rev[1]: want v2/&body/false, got %+v", revs[1])
	}
	if revs[0].DocumentID != created.ID || revs[1].DocumentID != created.ID {
		t.Errorf("document_id: want %s in allen Revisionen, got %+v", created.ID, revs)
	}
}

func TestDocuments_Put_ConflictWritesNoRevision(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-2")
	const p = "projects/flow/conflict.md"

	if _, err := docs.Put(uid, p, "v1", "", 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	// create-only auf existierenden Pfad → Konflikt, keine neue Revision
	if _, err := docs.Put(uid, p, "x", "", 0); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Fatalf("create on existing: want conflict, got %v", err)
	}
	// stale If-Match → Konflikt, keine neue Revision
	if _, err := docs.Put(uid, p, "stale", "", 99); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Fatalf("stale update: want conflict, got %v", err)
	}

	if revs := revisionsFor(t, uid, p); len(revs) != 1 {
		t.Errorf("revisions nach Konflikten: want 1, got %d (%+v)", len(revs), revs)
	}
}

func TestDocuments_Delete_WritesDeletedMarkerAndSurvives(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-3")
	const p = "projects/flow/deleted.md"

	if _, err := docs.Put(uid, p, "final body", "", 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := docs.Delete(uid, p); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Dokument weg …
	if _, err := docs.Get(uid, p); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
	// … Revisionen überleben (kein FK): Save + Lösch-Marker mit letztem Body.
	revs := revisionsFor(t, uid, p)
	if len(revs) != 2 {
		t.Fatalf("revisions: want 2 (save + marker), got %d (%+v)", len(revs), revs)
	}
	marker := revs[1]
	if !marker.Deleted || marker.Body != "final body" || marker.Version != 1 {
		t.Errorf("marker: want deleted/final body/v1, got %+v", marker)
	}

	// Delete auf nicht-existenten Pfad bleibt idempotent und schreibt nichts.
	if err := docs.Delete(uid, p); err != nil {
		t.Fatalf("delete idempotent: %v", err)
	}
	if revs := revisionsFor(t, uid, p); len(revs) != 2 {
		t.Errorf("revisions nach No-op-Delete: want 2, got %d", len(revs))
	}
}
