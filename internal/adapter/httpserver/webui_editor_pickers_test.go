package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

// TestWebEditorArtefaktePicker_ListsAncestorChain is the Task 6 mandatory
// test for GET /ui/editor/artefakte?node={id}: the picker fragment must list
// the node's own artifacts (reachable via the Ahnenkette usecase.ListArtifacts
// already implements), each row carrying the exact ![[slug]] insert value
// editor-insert.js writes into the textarea.
func TestWebEditorArtefaktePicker_ListsAncestorChain(t *testing.T) {
	srv, _, _, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()

	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "bild", Name: "bild.png", Mime: "image/png", SizeBytes: 1024,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	req := authedEditorRequest(http.MethodGet, "/ui/editor/artefakte?node=n1", nil)
	rec := httptest.NewRecorder()
	srv.handleWebEditorArtefaktePicker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`data-insert-value="![[bild]]"`, "bild.png"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in picker fragment: %.800s", want, body)
		}
	}
}

// TestWebEditorArtefaktePicker_ForeignNodeIsEmpty is the Task 6 mandatory
// owner-scope test: a node owned by a different user must yield the
// picker's own empty state (never that owner's artifacts, never a 500) —
// NodeStore.Ancestors degrades a foreign nodeID to an empty chain, so
// ListArtifacts naturally returns nothing for it.
func TestWebEditorArtefaktePicker_ForeignNodeIsEmpty(t *testing.T) {
	srv, _, _, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()

	if _, err := projects.Create(ctx, domain.Node{ID: "foreign", OwnerID: "u2", Name: "other-repo", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed foreign node: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u2", NodeID: "foreign", Slug: "geheim", Name: "geheim.png", Mime: "image/png",
	}); err != nil {
		t.Fatalf("seed foreign artifact: %v", err)
	}

	req := authedEditorRequest(http.MethodGet, "/ui/editor/artefakte?node=foreign", nil)
	rec := httptest.NewRecorder()
	srv.handleWebEditorArtefaktePicker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "geheim") {
		t.Fatalf("foreign owner's artifact must not appear: %.800s", body)
	}
	if !strings.Contains(body, "insert-picker-empty") {
		t.Fatalf("expected the empty state for a foreign/unreachable node: %.800s", body)
	}
}

// TestWebEditorSeitenPicker_ListsDocs is the Task 6 mandatory test for
// GET /ui/editor/seiten: the picker fragment must list the owner's documents,
// each row carrying the exact [[path]] insert value editor-insert.js writes
// into the textarea.
func TestWebEditorSeitenPicker_ListsDocs(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := docs.Create(ctx, domain.Document{
		ID: "d1", OwnerID: "u1", Type: domain.DocFree, Path: "notes/seiten",
		Title: "Seiten-Test", Body: "hi", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	req := authedEditorRequest(http.MethodGet, "/ui/editor/seiten", nil)
	rec := httptest.NewRecorder()
	srv.handleWebEditorSeitenPicker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`data-insert-value="[[notes/seiten]]"`, "Seiten-Test"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in picker fragment: %.800s", want, body)
		}
	}
}

// TestWebEditorSeitenPicker_OwnerScopeNegative is the Codex-Fund #8
// owner-scope negative test: a second owner's document must never appear in
// the "Seite verlinken" picker regardless of q — ListDocuments (and its
// underlying DocumentStore) is owner-scoped, mirroring the Artefakt-picker's
// own foreign-node test above.
func TestWebEditorSeitenPicker_OwnerScopeNegative(t *testing.T) {
	srv, _, docs, _ := newWebWissenServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := docs.Create(ctx, domain.Document{
		ID: "foreign-doc", OwnerID: "u2", Type: domain.DocFree, Path: "notes/foreign",
		Title: "Foreign Secret", Body: "hi", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed foreign doc: %v", err)
	}

	req := authedEditorRequest(http.MethodGet, "/ui/editor/seiten", nil)
	rec := httptest.NewRecorder()
	srv.handleWebEditorSeitenPicker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%.400s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Foreign Secret") || strings.Contains(body, "notes/foreign") {
		t.Fatalf("a foreign owner's document must not appear: %.800s", body)
	}
	if !strings.Contains(body, "insert-picker-empty") {
		t.Fatalf("expected the empty state (owner u1 has no documents): %.800s", body)
	}
}
