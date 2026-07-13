package nodetree

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeFormAPI struct {
	nodes   []domain.Node
	created CreateFields
	updated UpdateFields
	updID   string
	rateAmt *int64
	rateCur string
	createN domain.Node
}

func (f *fakeFormAPI) ListNodes(context.Context) ([]domain.Node, error) { return f.nodes, nil }
func (f *fakeFormAPI) CreateNode(_ context.Context, in CreateFields) (domain.Node, error) {
	f.created = in
	if f.createN.ID == "" {
		f.createN = domain.Node{ID: "new1", Name: in.Name, Kind: domain.NodeKind(in.Kind)}
	}
	return f.createN, nil
}
func (f *fakeFormAPI) UpdateNode(_ context.Context, id string, in UpdateFields) (domain.Node, error) {
	f.updID, f.updated = id, in
	return domain.Node{ID: id}, nil
}
func (f *fakeFormAPI) SetNodeRate(_ context.Context, _ string, amount *int64, cur string) error {
	f.rateAmt, f.rateCur = amount, cur
	return nil
}

func TestForm_CreateRepoUnderParent(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{nodes: []domain.Node{{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"}}}
	r := NewFormRoute(f, theme.Default, nil)
	r.Update(nodesLoadedMsg{nodes: f.nodes})
	r.FillForTest(FormValues{Name: "flow", Kind: string(domain.KindRepo), ParentID: "e1"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("valid create must return a cmd")
	}
	cmd()
	if f.created.Name != "flow" || f.created.Kind != string(domain.KindRepo) {
		t.Fatalf("CreateNode fields wrong: %+v", f.created)
	}
	if f.created.ParentID == nil || *f.created.ParentID != "e1" {
		t.Fatalf("ParentID = %v, want e1", f.created.ParentID)
	}
}

func TestForm_CreateEngagementWithRate(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{}
	r := NewFormRoute(f, theme.Default, nil)
	r.Update(nodesLoadedMsg{})
	r.FillForTest(FormValues{Name: "RTL", Kind: string(domain.KindEngagement), RateAmount: "95", RateCurrency: "EUR"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("valid create must return cmd")
	}
	cmd()
	if f.created.ParentID != nil {
		t.Fatalf("engagement must be root, ParentID=%v", f.created.ParentID)
	}
	if f.rateAmt == nil || *f.rateAmt != 9500 || f.rateCur != "EUR" {
		t.Fatalf("rate not set: amt=%v cur=%q", f.rateAmt, f.rateCur)
	}
}

func TestForm_NonEngagementNeedsParent(t *testing.T) {
	t.Parallel()
	r := NewFormRoute(&fakeFormAPI{}, theme.Default, nil)
	r.FillForTest(FormValues{Name: "x", Kind: string(domain.KindRepo)}) // no parent
	if _, cmd := r.Submit(); cmd != nil {
		t.Fatal("repo without parent must fail validation")
	}
	if r.err == "" {
		t.Fatal("expected validation error")
	}
}

func TestForm_EditUpdatesMetadata(t *testing.T) {
	t.Parallel()
	f := &fakeFormAPI{}
	editing := domain.Node{ID: "r1", Kind: domain.KindRepo, Name: "flow", Slug: "flow", Status: domain.NodeActive}
	r := NewFormRoute(f, theme.Default, &editing)
	r.FillForTest(FormValues{Name: "flow2", Slug: "flow", Status: "active"})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("edit must return cmd")
	}
	cmd()
	if f.updID != "r1" || f.updated.Name == nil || *f.updated.Name != "flow2" {
		t.Fatalf("UpdateNode wrong: id=%q %+v", f.updID, f.updated)
	}
	if f.created.Name != "" {
		t.Fatal("edit must not call CreateNode")
	}
}
