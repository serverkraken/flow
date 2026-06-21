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
			ID: id, OwnerID: "u1", ProjectID: proj, Type: domain.DocFree,
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
	fake := seedScopeFake(t)
	emb := testutil.NewFakeEmbedder()
	chunkVec := func(body string) []float32 {
		v, err := emb.Embed(ctx, []string{body})
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		return v[0]
	}
	// one chunk per doc so the semantic arm has candidates
	_ = fake.ReplaceChunks(ctx, "d-a", "u1", []string{"alpha thing"}, [][]float32{chunkVec("alpha thing")})
	_ = fake.ReplaceChunks(ctx, "d-b", "u1", []string{"beta thing"}, [][]float32{chunkVec("beta thing")})

	uc := usecase.SearchDocuments{Docs: fake, Embedder: emb}
	got, err := uc.Execute(ctx, "u1", "thing", pa("proj-a"), nil)
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Search(proj-a) returned no hits")
	}
	for _, h := range got {
		if h.ProjectID == nil || *h.ProjectID != "proj-a" {
			t.Fatalf("hit %s escaped project scope (projectID=%v)", h.ID, h.ProjectID)
		}
	}
}
