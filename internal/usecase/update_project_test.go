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

func newUpdateUC() (usecase.UpdateProject, *testutil.FakeProjectStore, *testutil.FakeProjectBindingStore) {
	ps := testutil.NewFakeProjectStore()
	bs := testutil.NewFakeProjectBindingStore()
	uc := usecase.UpdateProject{
		Projects: ps, Bindings: bs,
		IDs:   &testutil.FakeIDGen{},
		Clock: testutil.FakeClock{T: time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)},
	}
	return uc, ps, bs
}

func seedProj(t *testing.T, ps *testutil.FakeProjectStore, id, upstream string) {
	t.Helper()
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	p, _ := domain.NewProject(id, "u1", "Flow", "flow", now)
	p.UpstreamGit = upstream
	_, _ = ps.Create(context.Background(), p)
}

func baseInput() usecase.UpdateProjectInput {
	return usecase.UpdateProjectInput{Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
}

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

func TestUpdateProject_SetUpstreamCreatesBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = "git@github.com:serverkraken/flow.git"
	got, err := uc.Execute(context.Background(), "u1", "p1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpstreamGit != in.UpstreamGit {
		t.Errorf("upstream not saved: %q", got.UpstreamGit)
	}
	if slugs := remoteSlugs(bs); len(slugs) != 1 || slugs[0] != "github.com/serverkraken/flow" {
		t.Errorf("want one remote binding github.com/serverkraken/flow, got %v", slugs)
	}
}

func TestUpdateProject_ClearUpstreamRemovesBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "git@github.com:serverkraken/flow.git")
	// pre-create the matching binding
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", ProjectID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/flow",
	})
	in := baseInput() // UpstreamGit == ""
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	if slugs := remoteSlugs(bs); len(slugs) != 0 {
		t.Errorf("binding should be gone, got %v", slugs)
	}
}

func TestUpdateProject_ReassignUpstreamRepointsBinding(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "git@github.com:serverkraken/old.git")
	_, _ = bs.Upsert(context.Background(), domain.ProjectBinding{
		ID: "b1", OwnerID: "u1", ProjectID: "p1",
		Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/old",
	})
	in := baseInput()
	in.UpstreamGit = "https://github.com/serverkraken/new.git"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	slugs := remoteSlugs(bs)
	if len(slugs) != 1 || slugs[0] != "github.com/serverkraken/new" {
		t.Errorf("want only github.com/serverkraken/new, got %v", slugs)
	}
}

func TestUpdateProject_InvalidUpstreamRejected(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.UpstreamGit = "not a url"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidUpstream) {
		t.Fatalf("want ErrInvalidUpstream, got %v", err)
	}
	// nothing persisted, no binding
	got, _ := ps.Get(context.Background(), "u1", "p1")
	if got.UpstreamGit != "" {
		t.Errorf("upstream must not be persisted on reject, got %q", got.UpstreamGit)
	}
	if len(remoteSlugs(bs)) != 0 {
		t.Errorf("no binding expected on reject")
	}
}

func TestUpdateProject_BadStatusRejected(t *testing.T) {
	uc, ps, _ := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Status = "weird"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); !errors.Is(err, domain.ErrInvalidProject) {
		t.Fatalf("want ErrInvalidProject, got %v", err)
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	uc, _, _ := newUpdateUC()
	if _, err := uc.Execute(context.Background(), "u1", "missing", baseInput()); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("want ErrProjectNotFound, got %v", err)
	}
}

func TestUpdateProject_DescriptionOnlyNoBindingChurn(t *testing.T) {
	uc, ps, bs := newUpdateUC()
	seedProj(t, ps, "p1", "")
	in := baseInput()
	in.Description = "just notes"
	if _, err := uc.Execute(context.Background(), "u1", "p1", in); err != nil {
		t.Fatal(err)
	}
	if len(remoteSlugs(bs)) != 0 {
		t.Errorf("description-only edit must not create a binding")
	}
}
