package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateNode_AggregateRollsBackEveryFollowFailure(t *testing.T) {
	for _, stage := range []string{
		testutil.NodeAggregateFailRate,
		testutil.NodeAggregateFailTags,
		testutil.NodeAggregateFailLogo,
		testutil.NodeAggregateFailCommit,
	} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			nodes := testutil.NewFakeNodeStore()
			logos := testutil.NewFakeNodeLogoStore()
			tags := testutil.NewFakeTagStore()
			agg := testutil.NewFakeNodeAggregateStore(nodes, logos, tags)
			agg.FailStage = stage
			ids := &testutil.FakeIDGen{}
			clk := testutil.FakeClock{T: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)}
			rawTags := []string{"new"}
			uc := usecase.CreateNode{Nodes: nodes, Aggregate: agg, IDs: ids, Clock: clk}

			_, err := uc.Execute(ctx, "u1", usecase.CreateNodeInput{
				Name: "Atomic", Kind: domain.KindEngagement,
				Rate: &domain.Money{Amount: 9000, Currency: "EUR"},
				Tags: &rawTags, LogoData: pngPixel(t),
			})
			if err == nil {
				t.Fatal("create succeeded despite injected aggregate failure")
			}
			if got, err := nodes.List(ctx, "u1"); err != nil || len(got) != 0 {
				t.Fatalf("partial node survived rollback: nodes=%+v err=%v", got, err)
			}
			if _, err := logos.Get(ctx, "u1", "id-1"); !errors.Is(err, ports.ErrNodeLogoNotFound) {
				t.Fatalf("partial logo survived rollback: %v", err)
			}
			if got, err := tags.TagsFor(ctx, "u1", domain.TaggableNode, "id-1"); err != nil || len(got) != 0 {
				t.Fatalf("partial tags survived rollback: tags=%+v err=%v", got, err)
			}
		})
	}
}

func TestUpdateNode_AggregateRollsBackEveryFollowFailure(t *testing.T) {
	for _, stage := range []string{
		testutil.NodeAggregateFailRate,
		testutil.NodeAggregateFailTags,
		testutil.NodeAggregateFailLogo,
		testutil.NodeAggregateFailCommit,
	} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			nodes := testutil.NewFakeNodeStore()
			logos := testutil.NewFakeNodeLogoStore()
			tags := testutil.NewFakeTagStore()
			agg := testutil.NewFakeNodeAggregateStore(nodes, logos, tags)
			clk := testutil.FakeClock{T: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)}
			n, _ := domain.NewNode("n1", "u1", "Old", "old", clk.Now().Add(-time.Hour))
			n.Kind = domain.KindEngagement
			oldCounts := true
			n.CountsTowardTarget = &oldCounts
			_, _ = nodes.Create(ctx, n)
			_ = nodes.SetRate(ctx, "u1", n.ID, &domain.Money{Amount: 7000, Currency: "EUR"})
			_, _ = tags.SetTags(ctx, "u1", domain.TaggableNode, n.ID, []string{"old"})
			upload := usecase.UploadNodeLogo{Nodes: nodes, Logos: logos, Aggregate: agg, Clock: clk}
			before, err := upload.Execute(ctx, "u1", n.ID, pngPixel(t))
			if err != nil {
				t.Fatal(err)
			}
			agg.FailStage = stage
			name := "New"
			newCounts := false
			newTags := []string{"new"}
			uc := usecase.UpdateNode{Nodes: nodes, Aggregate: agg, Clock: clk}

			_, err = uc.Execute(ctx, "u1", n.ID, usecase.UpdateNodeInput{
				Name:      &name,
				ApplyRate: true, Rate: &domain.Money{Amount: 9000, Currency: "EUR"},
				ApplyCountsTowardTarget: true, CountsTowardTarget: &newCounts,
				Tags: &newTags, DeleteLogo: true,
			})
			if err == nil {
				t.Fatal("update succeeded despite injected aggregate failure")
			}
			got, err := nodes.Get(ctx, "u1", n.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "Old" || got.Rate == nil || got.Rate.Amount != 7000 || got.CountsTowardTarget == nil || !*got.CountsTowardTarget || got.LogoRef != before.LogoRef {
				t.Fatalf("node changed despite rollback: %+v", got)
			}
			gotTags, err := tags.TagsFor(ctx, "u1", domain.TaggableNode, n.ID)
			if err != nil || len(gotTags) != 1 || gotTags[0].Slug != "old" {
				t.Fatalf("tags changed despite rollback: tags=%+v err=%v", gotTags, err)
			}
			gotLogo, err := logos.Get(ctx, "u1", n.ID)
			if err != nil || gotLogo.Ref != before.LogoRef {
				t.Fatalf("logo changed despite rollback: logo=%+v err=%v", gotLogo, err)
			}
		})
	}
}

func TestUpdateNode_AggregateRemainsOwnerScoped(t *testing.T) {
	ctx := context.Background()
	nodes := testutil.NewFakeNodeStore()
	logos := testutil.NewFakeNodeLogoStore()
	tags := testutil.NewFakeTagStore()
	agg := testutil.NewFakeNodeAggregateStore(nodes, logos, tags)
	clk := testutil.FakeClock{T: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)}
	n, _ := domain.NewNode("n1", "u1", "Old", "old", clk.Now())
	n.Kind = domain.KindEngagement
	_, _ = nodes.Create(ctx, n)
	name := "Stolen"
	uc := usecase.UpdateNode{Nodes: nodes, Aggregate: agg, Clock: clk}
	if _, err := uc.Execute(ctx, "u2", n.ID, usecase.UpdateNodeInput{Name: &name}); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("foreign update: want ErrNodeNotFound, got %v", err)
	}
	got, _ := nodes.Get(ctx, "u1", n.ID)
	if got.Name != "Old" {
		t.Fatalf("foreign update changed owner node: %+v", got)
	}
}
