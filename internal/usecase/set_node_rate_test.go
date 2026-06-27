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
	ns := testutil.NewFakeNodeStore()
	seedRepo(t, ns, "u1", "repo1")
	uc := usecase.SetNodeRate{Nodes: ns}
	if err := uc.Execute(context.Background(), "u1", "repo1", &domain.Money{Amount: 5000, Currency: "EUR"}); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("want ErrInvalidNode setting rate on a repo, got %v", err)
	}
}

func TestSetNodeRate_MissingNode(t *testing.T) {
	t.Parallel()
	uc := usecase.SetNodeRate{Nodes: testutil.NewFakeNodeStore()}
	if err := uc.Execute(context.Background(), "u1", "ghost", &domain.Money{Amount: 5000, Currency: "EUR"}); err == nil {
		t.Fatal("want error for missing node")
	}
}
