package httpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
)

// --- buildArtifactResolver (pure function) -------------------------------

// TestBuildArtifactResolver_NearestAncestorWins is the Task 3 mandatory test
// (Spec §6.1): the same slug exists on the document's own node ("leaf") and
// on its ancestor ("parent") — the nearer node's artifact must win, never
// created-at ordering (List itself returns newest-first, which this test
// deliberately puts in the OPPOSITE order of "nearest wins" to prove the
// chain-position ranking, not list order, decides).
func TestBuildArtifactResolver_NearestAncestorWins(t *testing.T) {
	chain := []domain.Node{{ID: "leaf"}, {ID: "parent"}} // leaf→root, as NodeStore.Ancestors returns
	arts := []domain.Artifact{
		{NodeID: "parent", Slug: "logo", Name: "parent-logo.png", Mime: "image/png"},
		{NodeID: "leaf", Slug: "logo", Name: "leaf-logo.png", Mime: "image/png"},
	}
	resolve := buildArtifactResolver(chain, arts)
	if resolve == nil {
		t.Fatal("expected a non-nil resolver")
	}
	ref, ok := resolve("logo")
	if !ok {
		t.Fatal("expected slug \"logo\" to resolve")
	}
	if ref.Href != "/nodes/leaf/artifacts/logo" || ref.Name != "leaf-logo.png" {
		t.Fatalf("nearest ancestor (leaf) must win, got href=%q name=%q", ref.Href, ref.Name)
	}
}

// TestBuildArtifactResolver_HrefPointsAtArtifactsOwnNode is the other Task 3
// mandatory test: an artifact inherited from an ancestor must serve from
// THAT ancestor's node id, never the document's own node — pointing at the
// wrong node 404s on the real serve route (GET /nodes/{id}/artifacts/{slug}).
func TestBuildArtifactResolver_HrefPointsAtArtifactsOwnNode(t *testing.T) {
	chain := []domain.Node{{ID: "leaf"}, {ID: "ancestor"}}
	arts := []domain.Artifact{
		{NodeID: "ancestor", Slug: "spec", Name: "spec.pdf", Mime: "application/pdf", SizeBytes: 2048},
	}
	resolve := buildArtifactResolver(chain, arts)
	ref, ok := resolve("spec")
	if !ok {
		t.Fatal("expected slug \"spec\" to resolve")
	}
	if ref.Href != "/nodes/ancestor/artifacts/spec" {
		t.Fatalf("href must target the artifact's own node (ancestor), got %q", ref.Href)
	}
	if ref.IsImage {
		t.Fatal("application/pdf must not be IsImage")
	}
	if ref.SizeStr == "" {
		t.Fatal("expected a formatted SizeStr")
	}
}

// TestBuildArtifactResolver_NothingResolvesReturnsNilResolver covers the "no
// artifacts reachable" state (empty chain/list, or every artifact's node is
// outside the chain) — a nil resolver, so RenderDocument treats every embed
// as unresolved instead of the caller having to special-case an empty map.
func TestBuildArtifactResolver_NothingResolvesReturnsNilResolver(t *testing.T) {
	if r := buildArtifactResolver(nil, nil); r != nil {
		t.Fatal("expected nil resolver when nothing can resolve")
	}
	// An artifact whose node isn't even in the chain (shouldn't happen given
	// ListArtifacts only returns chain-scoped rows, but buildArtifactResolver
	// must not trust that blindly) is skipped rather than resolved.
	chain := []domain.Node{{ID: "leaf"}}
	arts := []domain.Artifact{{NodeID: "other", Slug: "x", Name: "x.png", Mime: "image/png"}}
	resolve := buildArtifactResolver(chain, arts)
	if resolve != nil {
		if _, ok := resolve("x"); ok {
			t.Fatal("artifact outside the chain must not resolve")
		}
	}
}

// --- Free artifacts (Task 2: read/serve path, dormant until Task 3) -----

// TestBuildArtifactResolver_FreeArtifactResolves covers a free artifact
// (NodeID=="") on its own, with no chain artifact shadowing it: it must
// resolve with an /artefakte/{slug} href (never /nodes//artifacts/...) and
// its Ref set.
func TestBuildArtifactResolver_FreeArtifactResolves(t *testing.T) {
	chain := []domain.Node{{ID: "leaf"}, {ID: "root"}}
	arts := []domain.Artifact{
		{NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png", Ref: "aaaaaaaaaaaa"},
	}
	resolve := buildArtifactResolver(chain, arts)
	if resolve == nil {
		t.Fatal("expected a non-nil resolver")
	}
	ref, ok := resolve("brand")
	if !ok {
		t.Fatal("expected slug \"brand\" to resolve")
	}
	if ref.Href != "/artefakte/brand" {
		t.Fatalf("free artifact href = %q, want /artefakte/brand", ref.Href)
	}
	if ref.Ref != "aaaaaaaaaaaa" {
		t.Fatalf("free artifact Ref = %q, want aaaaaaaaaaaa", ref.Ref)
	}
}

// TestBuildArtifactResolver_NodeWinsOverFree is the Spec §E1 nearest-wins
// rule extended to the free tier: the same slug exists on a chain node AND
// free — the node version must win (href points at the node, not /artefakte).
func TestBuildArtifactResolver_NodeWinsOverFree(t *testing.T) {
	chain := []domain.Node{{ID: "leaf"}, {ID: "root"}}
	arts := []domain.Artifact{
		{NodeID: "", Slug: "logo", Name: "free-logo.png", Mime: "image/png"},
		{NodeID: "root", Slug: "logo", Name: "root-logo.png", Mime: "image/png"},
	}
	resolve := buildArtifactResolver(chain, arts)
	ref, ok := resolve("logo")
	if !ok {
		t.Fatal("expected slug \"logo\" to resolve")
	}
	if ref.Href != "/nodes/root/artifacts/logo" || ref.Name != "root-logo.png" {
		t.Fatalf("node artifact must win over free, got href=%q name=%q", ref.Href, ref.Name)
	}
}

// TestBuildArtifactResolver_OnlyFreeWins covers a slug that exists ONLY as a
// free artifact — it must resolve (root-lowest priority is not "never wins",
// just "loses to any chain node").
func TestBuildArtifactResolver_OnlyFreeWins(t *testing.T) {
	chain := []domain.Node{{ID: "leaf"}}
	arts := []domain.Artifact{
		{NodeID: "", Slug: "spec", Name: "spec.pdf", Mime: "application/pdf"},
	}
	resolve := buildArtifactResolver(chain, arts)
	ref, ok := resolve("spec")
	if !ok {
		t.Fatal("expected the free-only slug to resolve")
	}
	if ref.Href != "/artefakte/spec" {
		t.Fatalf("free-only artifact href = %q, want /artefakte/spec", ref.Href)
	}
}

// --- End-to-end through the real /wissen/{id} route ----------------------

// TestWebDocumentView_ArtifactEmbedResolvesEndToEnd exercises the full path
// (real route → buildDocumentVM → RenderDocument → figure renderer) for a
// resolved image embed, complementing the webui-package unit test
// (TestArtifactEmbed_ResolvedImage) with the actual HTTP wiring.
func TestWebDocumentView_ArtifactEmbedResolvesEndToEnd(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	if _, err := projects.Create(ctx, domain.Node{ID: "n1", OwnerID: "u1", Name: "flow", Kind: domain.KindRepo}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "bild", Name: "bild.png", Mime: "image/png",
		SizeBytes: 1024, Ref: "abcdef123456", Width: 10, Height: 8,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	nodeID := "n1"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "with-embed", OwnerID: "u1", NodeID: &nodeID, Type: domain.DocFree,
		Path: "p/with-embed", Title: "Has Embed", Body: "See ![[bild]] above.\n",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/with-embed")
	if status != 200 {
		t.Fatalf("GET /wissen/with-embed status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		`class="figure"`,
		`src="/nodes/n1/artifacts/bild?v=abcdef123456"`,
		"Abb. 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in rendered document:\n%.1500s", want, body)
		}
	}
}

// TestWebDocumentView_ArtifactEmbedNearestAncestorWinsEndToEnd is the same
// Spec §6.1 rule as TestBuildArtifactResolver_NearestAncestorWins, but
// driven through the real route with the real ListArtifacts usecase (not
// just the pure resolver-builder function) — the document lives on the
// child node, the same slug exists on both child and parent, and the
// child's own artifact must win end to end.
func TestWebDocumentView_ArtifactEmbedNearestAncestorWinsEndToEnd(t *testing.T) {
	srv, codec, docs, projects := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	if _, err := projects.Create(ctx, domain.Node{ID: "root", OwnerID: "u1", Name: "eng", Kind: domain.KindEngagement}); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	rootID := "root"
	if _, err := projects.Create(ctx, domain.Node{ID: "child", OwnerID: "u1", Name: "repo", Kind: domain.KindRepo, ParentID: &rootID}); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "root", Slug: "logo", Name: "root-logo.png", Mime: "image/png", Ref: "111111111111",
	}); err != nil {
		t.Fatalf("seed root artifact: %v", err)
	}
	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "child", Slug: "logo", Name: "child-logo.png", Mime: "image/png", Ref: "222222222222",
	}); err != nil {
		t.Fatalf("seed child artifact: %v", err)
	}
	childID := "child"
	if _, err := docs.Create(ctx, domain.Document{
		ID: "on-child", OwnerID: "u1", NodeID: &childID, Type: domain.DocFree,
		Path: "p/on-child", Title: "On Child", Body: "![[logo]]\n",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/on-child")
	if status != 200 {
		t.Fatalf("GET /wissen/on-child status=%d body=%.400s", status, body)
	}
	if !strings.Contains(body, `src="/nodes/child/artifacts/logo?v=222222222222"`) {
		t.Fatalf("nearest (child) artifact must win end to end, got:\n%.1500s", body)
	}
	if strings.Contains(body, "root-logo.png") {
		t.Fatalf("root's shadowed artifact must not appear at all:\n%.1500s", body)
	}
}

// TestWebDocumentView_FreeDocResolvesFreeArtifactEmbed is the free-artifacts
// Task 3 mandatory regression test: a free doc (NodeID==nil) embedding a free
// image via ![[slug]] must resolve it end to end. Before this task,
// buildDocumentVM only built the artifact resolver inside the
// `if doc.NodeID != nil` branch, so a free doc's embed always stayed
// unresolved — the actual bug this task fixes.
func TestWebDocumentView_FreeDocResolvesFreeArtifactEmbed(t *testing.T) {
	srv, codec, docs, _ := newWebWissenServer(t)
	artifacts := srv.ListArtifacts.Artifacts.(*testutil.FakeArtifactStore)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	if err := artifacts.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "bild", Name: "bild.png", Mime: "image/png",
		SizeBytes: 1024, Ref: "abcdef123456", Width: 10, Height: 8,
	}); err != nil {
		t.Fatalf("seed free artifact: %v", err)
	}
	if _, err := docs.Create(ctx, domain.Document{
		ID: "free-with-embed", OwnerID: "u1", NodeID: nil, Type: domain.DocFree,
		Path: "p/free-with-embed", Title: "Free Has Embed", Body: "See ![[bild]] above.\n",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	body, status := getWissenDocumentAs(t, srv, codec, "u1", "/wissen/free-with-embed")
	if status != 200 {
		t.Fatalf("GET /wissen/free-with-embed status=%d body=%.400s", status, body)
	}
	for _, want := range []string{
		`class="figure"`,
		`src="/artefakte/bild?v=abcdef123456"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q — free doc did not resolve the free artifact embed:\n%.1500s", want, body)
		}
	}
}
