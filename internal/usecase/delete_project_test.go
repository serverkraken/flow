package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestDeleteProject(t *testing.T) {
	ps := testutil.NewFakeNodeStore()
	p, _ := ps.Create(context.Background(), domain.Node{ID: "p1", OwnerID: "u", Slug: "x"})
	uc := usecase.DeleteNode{Nodes: ps}
	if err := uc.Execute(context.Background(), "u", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Get(context.Background(), "u", p.ID); err == nil {
		t.Fatal("project should be gone")
	}
	if err := uc.Execute(context.Background(), "u", "missing"); err == nil {
		t.Fatal("deleting a missing project should error")
	}
}
