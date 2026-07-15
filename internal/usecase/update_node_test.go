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

// countingBindingStore wraps the fake bindings store to make DeleteRemote
// calls observable — used to assert that a partial update omitting
// UpstreamGit never touches the node's remote binding.
type countingBindingStore struct {
	*testutil.FakeProjectBindingStore
	deletedRemotes int
}

func (s *countingBindingStore) DeleteRemote(ctx context.Context, ownerID, remoteSlug string) error {
	s.deletedRemotes++
	return s.FakeProjectBindingStore.DeleteRemote(ctx, ownerID, remoteSlug)
}

func newUpdateNodeFixture(t *testing.T) (usecase.UpdateNode, *testutil.FakeNodeStore, *countingBindingStore) {
	t.Helper()
	ns := testutil.NewFakeNodeStore()
	bs := &countingBindingStore{FakeProjectBindingStore: testutil.NewFakeProjectBindingStore()}
	uc := usecase.UpdateNode{
		Nodes: ns, Bindings: bs,
		IDs:   &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)},
	}
	return uc, ns, bs
}

// putNode seeds ns with n directly (no domain.Validate — mirrors how a
// pre-existing stored node may carry fields set at creation time).
func putNode(t *testing.T, ns *testutil.FakeNodeStore, n domain.Node) {
	t.Helper()
	if _, err := ns.Create(context.Background(), n); err != nil {
		t.Fatalf("seed node: %v", err)
	}
}

func seedProj(t *testing.T, ps *testutil.FakeNodeStore, id, upstream string) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewNode(id, "u1", "Flow", "flow", now)
	p.Kind = domain.KindRepo
	p.UpstreamGit = upstream
	if slug, ok := domain.NormalizeRemoteSlug(upstream); ok {
		p.OriginSlug = slug
	}
	_, _ = ps.Create(context.Background(), p)
}

func baseInput() usecase.UpdateNodeInput {
	return usecase.UpdateNodeInput{Name: sp("Flow"), Slug: sp("flow"), Status: statusPtr(domain.NodeActive)}
}

func statusPtr(s domain.NodeStatus) *domain.NodeStatus { return &s }

func remoteSlugs(bs *testutil.FakeProjectBindingStore) []string {
	all, _ := bs.List(context.Background(), "u1")
	var out []string
	for _, b := range all {
		if b.Kind == domain.BindingRemote {
			out = append(out, b.RemoteSlug)
		}
	}
	return out
}

func TestUpdateNode_SetUpstreamCreatesBinding(t *testing.T) {
	uc, ps, bs := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = sp("git@github.com:serverkraken/flow.git")
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpstreamGit != *in.UpstreamGit {
		t.Errorf("upstream not saved: %q", got.UpstreamGit)
	}
	if got.OriginSlug != "github.com/serverkraken/flow" {
		t.Errorf("origin slug = %q, want normalized upstream", got.OriginSlug)
	}
	if slugs := remoteSlugs(bs.FakeProjectBindingStore); len(slugs) != 1 || slugs[0] != "github.com/serverkraken/flow" {
		t.Errorf("want one remote binding github.com/serverkraken/flow, got %v", slugs)
	}
}

func TestUpdateNode_ClearUpstreamRemovesBinding(t *testing.T) {
	uc, ps, bs := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "git@github.com:serverkraken/flow.git")
	// pre-create the matching binding
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", NodeID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow",
	})
	in := baseInput()
	in.UpstreamGit = sp("") // explicit clear
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginSlug != "" {
		t.Errorf("origin slug should be cleared, got %q", got.OriginSlug)
	}
	if slugs := remoteSlugs(bs.FakeProjectBindingStore); len(slugs) != 0 {
		t.Errorf("binding should be gone, got %v", slugs)
	}
}

func TestUpdateNode_ReassignUpstreamRepointsBinding(t *testing.T) {
	uc, ps, bs := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "git@github.com:serverkraken/old.git")
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", NodeID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/old",
	})
	in := baseInput()
	in.UpstreamGit = sp("https://github.com/serverkraken/new.git")
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginSlug != "github.com/serverkraken/new" {
		t.Errorf("origin slug = %q, want github.com/serverkraken/new", got.OriginSlug)
	}
	slugs := remoteSlugs(bs.FakeProjectBindingStore)
	if len(slugs) != 1 || slugs[0] != "github.com/serverkraken/new" {
		t.Errorf("want only github.com/serverkraken/new, got %v", slugs)
	}
}

func TestUpdateNode_InvalidUpstreamRejected(t *testing.T) {
	uc, ps, bs := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = sp("not a url")
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidUpstream) {
		t.Fatalf("want ErrInvalidUpstream, got %v", err)
	}
	// nothing persisted, no binding
	got, _ := ps.Get(context.Background(), "u1", "p1")
	if got.UpstreamGit != "" {
		t.Errorf("upstream must not be persisted on reject, got %q", got.UpstreamGit)
	}
	if len(remoteSlugs(bs.FakeProjectBindingStore)) != 0 {
		t.Errorf("no binding expected on reject")
	}
}

func TestUpdateNode_BadStatusRejected(t *testing.T) {
	uc, ps, _ := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Status = statusPtr(domain.NodeStatus("weird"))
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidNode) {
		t.Fatalf("want ErrInvalidNode, got %v", err)
	}
	// nothing persisted
	got, _ := ps.Get(context.Background(), "u1", "p1")
	if got.Status == "weird" {
		t.Errorf("bad status must not be persisted, got %q", got.Status)
	}
}

func TestUpdateNode_NotFound(t *testing.T) {
	uc, _, _ := newUpdateNodeFixture(t)
	if _, err := uc.Execute(context.Background(), "u1", "missing", baseInput()); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}

func TestUpdateNode_DescriptionOnlyNoBindingChurn(t *testing.T) {
	uc, ps, bs := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Description = sp("just notes")
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	if len(remoteSlugs(bs.FakeProjectBindingStore)) != 0 {
		t.Errorf("description-only edit must not create a binding")
	}
}

func TestUpdateNode_CountsTowardTarget(t *testing.T) {
	uc, ps, _ := newUpdateNodeFixture(t)
	seedProj(t, ps, "p1", "")

	// Explicit false → persists false (node was seeded with default nil/inherit).
	in := baseInput()
	in.CountsTowardTarget = ptrBool(false)
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.CountsTowardTarget == nil || *got.CountsTowardTarget {
		t.Fatalf("countsTowardTarget: want *false, got %v", got.CountsTowardTarget)
	}

	// Omit (nil) → existing false is preserved.
	in2 := baseInput()
	got2, err := uc.Execute(context.Background(), "u1", "p1", in2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.CountsTowardTarget == nil || *got2.CountsTowardTarget {
		t.Fatalf("omit: want existing false preserved, got %v", got2.CountsTowardTarget)
	}
}

// --- Partial-pointer semantics (Task 1) ---

func TestUpdateNode_PartialLeavesOtherFieldsUntouched(t *testing.T) {
	uc, ns, _ := newUpdateNodeFixture(t)
	seed := domain.Node{
		ID: "n1", OwnerID: "o1", Kind: domain.KindRepo, Status: domain.NodeActive,
		Name: "Alt", Slug: "alt", Icon: "rocket", Description: "d",
		UpstreamGit: "https://github.com/a/b",
	}
	putNode(t, ns, seed)

	desc := "neu"
	got, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Description: &desc})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Description != "neu" {
		t.Errorf("Description = %q, want neu", got.Description)
	}
	if got.Name != "Alt" || got.Slug != "alt" || got.Icon != "rocket" || got.UpstreamGit != "https://github.com/a/b" {
		t.Errorf("partial update mutated untouched fields: %+v", got)
	}
}

func TestUpdateNode_NilUpstreamKeepsBinding(t *testing.T) {
	uc, ns, bs := newUpdateNodeFixture(t)
	putNode(t, ns, domain.Node{
		ID: "n1", OwnerID: "o1", Kind: domain.KindRepo, Status: domain.NodeActive,
		Name: "R", Slug: "r", UpstreamGit: "https://github.com/a/b",
	})
	name := "R2"
	if _, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Name: &name}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bs.deletedRemotes != 0 {
		t.Errorf("partial update without upstream deleted %d bindings, want 0", bs.deletedRemotes)
	}
}

func TestUpdateNode_EmptyStringPointerClears(t *testing.T) {
	uc, ns, _ := newUpdateNodeFixture(t)
	putNode(t, ns, domain.Node{
		ID: "n1", OwnerID: "o1", Kind: domain.KindRepo, Status: domain.NodeActive,
		Name: "R", Slug: "r", Icon: "rocket",
	})
	empty := ""
	got, err := uc.Execute(context.Background(), "o1", "n1", usecase.UpdateNodeInput{Icon: &empty})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Icon != "" {
		t.Errorf("Icon = %q, want cleared", got.Icon)
	}
}
