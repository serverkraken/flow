package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func pa(s string) *string { return &s }

func seedScopeFake(t *testing.T) *testutil.FakeDocumentStore {
	t.Helper()
	fake := testutil.NewFakeDocumentStore()
	mk := func(id string, proj *string, body string) {
		if _, err := fake.Create(context.Background(), domain.Document{
			ID: id, OwnerID: "u1", NodeID: proj, Type: domain.DocFree,
			Path: id, Title: id, Body: body,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	mk("d-a", pa("proj-a"), "alpha thing")
	mk("d-b", pa("proj-b"), "beta thing")
	mk("d-x", nil, "free thing")
	return fake
}

func TestListDocuments_ProjectScope(t *testing.T) {
	fake := seedScopeFake(t)
	uc := usecase.ListDocuments{Docs: fake}
	got, err := uc.Execute(context.Background(), "u1", pa("proj-a"), nil)
	if err != nil || len(got) != 1 || got[0].ID != "d-a" {
		t.Fatalf("ListDocuments(proj-a) = %d, %v; want [d-a]", len(got), err)
	}
	none, _ := uc.Execute(context.Background(), "u1", pa("none"), nil)
	if len(none) != 1 || none[0].ID != "d-x" {
		t.Fatalf("ListDocuments(none) = %d; want [d-x]", len(none))
	}
}

func TestSearchDocuments_ProjectScopeReachesSemanticArm(t *testing.T) {
	ctx := context.Background()
	fake := testutil.NewFakeDocumentStore()
	emb := testutil.NewFakeEmbedder()
	// "needle" appears in NO document body, so the keyword arm returns nothing
	// and every returned hit must originate from the semantic arm.
	const query = "needle"
	qvec, err := emb.Embed(ctx, []string{query})
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	mk := func(id string, proj *string) {
		if _, err := fake.Create(ctx, domain.Document{
			ID: id, OwnerID: "u1", NodeID: proj, Type: domain.DocFree,
			Path: id, Title: id, Body: "haystack " + id, // deliberately lacks "needle"
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		// chunk embedding == query vector → the semantic arm ranks it top
		if err := fake.ReplaceChunks(ctx, id, "u1", []string{"haystack"}, [][]float32{qvec[0]}); err != nil {
			t.Fatalf("chunks %s: %v", id, err)
		}
	}
	mk("d-a", pa("proj-a"))
	mk("d-b", pa("proj-b"))

	uc := usecase.SearchDocuments{Docs: fake, Embedder: emb}
	got, err := uc.Execute(ctx, "u1", query, pa("proj-a"), nil)
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	// d-a is reachable ONLY via the semantic arm (its body lacks "needle");
	// d-b (proj-b) must be excluded by the semantic arm's project scope.
	var sawA, sawB bool
	for _, h := range got {
		switch h.ID {
		case "d-a":
			sawA = true
		case "d-b":
			sawB = true
		}
		if h.NodeID == nil || *h.NodeID != "proj-a" {
			t.Fatalf("hit %s escaped project scope (nodeID=%v)", h.ID, h.NodeID)
		}
	}
	if !sawA {
		t.Fatal("semantic arm did not return d-a (scoped to proj-a) — keyword arm cannot, body lacks the query term")
	}
	if sawB {
		t.Fatal("semantic arm leaked d-b from proj-b: project scope not applied to the semantic arm")
	}
}
