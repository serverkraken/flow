package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetNodeRate_ValidatesRate(t *testing.T) {
	t.Parallel()
	ns := testutil.NewFakeNodeStore()
	seedEngagement(t, ns, "u1", "eng1")
	uc := usecase.SetNodeRate{Nodes: ns}
	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: -1, Currency: "EUR"}); !errors.Is(err, domain.ErrInvalidRate) {
		t.Errorf("negative amount: want ErrInvalidRate, got %v", err)
	}
	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: 1, Currency: "EU"}); !errors.Is(err, domain.ErrInvalidRate) {
		t.Errorf("bad currency: want ErrInvalidRate, got %v", err)
	}
	if err := uc.Execute(context.Background(), "u1", "eng1", nil); err != nil {
		t.Errorf("nil rate (clear) on engagement should succeed: %v", err)
	}
	if err := uc.Execute(context.Background(), "u1", "eng1", &domain.Money{Amount: 5000, Currency: "EUR"}); err != nil {
		t.Errorf("valid rate on engagement should succeed: %v", err)
	}
}

func TestSetNodeRate_RepoRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	parent := "eng"
	if _, err := ns.Create(ctx, domain.Node{
		ID: "repo1", OwnerID: "u1", ParentID: &parent, Kind: domain.KindRepo,
		Name: "repo1", Slug: "repo1", Status: domain.NodeActive,
	}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	uc := usecase.SetNodeRate{Nodes: ns}
	rate := domain.Money{Amount: 12000, Currency: "EUR"}
	if err := uc.Execute(ctx, "u1", "repo1", &rate); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("set rate on repo: want ErrInvalidNode, got %v", err)
	}
	got, _ := ns.Get(ctx, "u1", "repo1")
	if got.Rate != nil {
		t.Errorf("repo rate was persisted: %+v", got.Rate)
	}
}

func TestSetNodeRate_MissingNode(t *testing.T) {
	t.Parallel()
	uc := usecase.SetNodeRate{Nodes: testutil.NewFakeNodeStore()}
	if err := uc.Execute(context.Background(), "u1", "ghost", &domain.Money{Amount: 5000, Currency: "EUR"}); err == nil {
		t.Fatal("want error for missing node")
	}
}
