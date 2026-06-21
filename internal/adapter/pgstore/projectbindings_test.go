package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestProjectBindingStore(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// seed user and two projects for FK constraints
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-bind", "sub-bind", "binduser", "bind@x.de", "Bind User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	projects := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	p1, _ := domain.NewProject("p-bind-1", "u-bind", "Project 1", "project-1", now)
	p2, _ := domain.NewProject("p-bind-2", "u-bind", "Project 2", "project-2", now)
	if _, err := projects.Create(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Create(ctx, p2); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewProjectBindingStore(pool)

	t.Run("upsert remote binding", func(t *testing.T) {
		b := domain.ProjectBinding{
			ID:         "bind-1",
			OwnerID:    "u-bind",
			ProjectID:  "p-bind-1",
			Kind:       domain.BindingRemote,
			RemoteSlug: "github.com/org/repo",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		got, err := st.Upsert(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "bind-1" {
			t.Errorf("want id bind-1, got %q", got.ID)
		}
		if got.ProjectID != "p-bind-1" {
			t.Errorf("want project p-bind-1, got %q", got.ProjectID)
		}
		if got.RemoteSlug != "github.com/org/repo" {
			t.Errorf("want slug github.com/org/repo, got %q", got.RemoteSlug)
		}
	})

	t.Run("re-upsert same remote_slug reassigns project", func(t *testing.T) {
		// same (owner, remote_slug) but different project: should reassign, not insert
		b2 := domain.ProjectBinding{
			ID:         "bind-2", // different id — conflict target must win
			OwnerID:    "u-bind",
			ProjectID:  "p-bind-2",
			Kind:       domain.BindingRemote,
			RemoteSlug: "github.com/org/repo",
			CreatedAt:  now,
			UpdatedAt:  now.Add(time.Second),
		}
		got, err := st.Upsert(ctx, b2)
		if err != nil {
			t.Fatal(err)
		}
		// id should be the original row's id (ON CONFLICT returns the existing row)
		if got.ProjectID != "p-bind-2" {
			t.Errorf("want project reassigned to p-bind-2, got %q", got.ProjectID)
		}

		// verify exactly one row in the DB
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 1 {
			t.Fatalf("want 1 binding after re-upsert, got %d", len(all))
		}
		if all[0].ProjectID != "p-bind-2" {
			t.Errorf("stored binding still has old project %q", all[0].ProjectID)
		}
	})

	t.Run("List is owner-scoped", func(t *testing.T) {
		// upsert a binding for a different owner — should not appear in u-bind's list
		u2, _ := domain.NewUser("u-other", "sub-other", "other", "other@x.de", "Other")
		if _, err := users.UpsertBySub(ctx, u2); err != nil {
			t.Fatal(err)
		}
		p3, _ := domain.NewProject("p-other-1", "u-other", "Other P", "other-p", now)
		if _, err := projects.Create(ctx, p3); err != nil {
			t.Fatal(err)
		}
		bOther := domain.ProjectBinding{
			ID:         "bind-other",
			OwnerID:    "u-other",
			ProjectID:  "p-other-1",
			Kind:       domain.BindingRemote,
			RemoteSlug: "github.com/org/repo", // same slug, different owner → no conflict
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if _, err := st.Upsert(ctx, bOther); err != nil {
			t.Fatal(err)
		}

		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.OwnerID != "u-bind" {
				t.Errorf("List returned binding for wrong owner: %q", b.OwnerID)
			}
		}
	})

	t.Run("ListByProject filters by project", func(t *testing.T) {
		// add a second binding for u-bind pointing to p-bind-1
		b3 := domain.ProjectBinding{
			ID:         "bind-3",
			OwnerID:    "u-bind",
			ProjectID:  "p-bind-1",
			Kind:       domain.BindingRemote,
			RemoteSlug: "github.com/org/other-repo",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if _, err := st.Upsert(ctx, b3); err != nil {
			t.Fatal(err)
		}

		byP1, err := st.ListByProject(ctx, "u-bind", "p-bind-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(byP1) != 1 {
			t.Fatalf("want 1 binding for p-bind-1, got %d", len(byP1))
		}
		if byP1[0].RemoteSlug != "github.com/org/other-repo" {
			t.Errorf("unexpected slug %q", byP1[0].RemoteSlug)
		}
	})

	t.Run("DeleteRemote removes binding", func(t *testing.T) {
		if err := st.DeleteRemote(ctx, "u-bind", "github.com/org/other-repo"); err != nil {
			t.Fatal(err)
		}
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.RemoteSlug == "github.com/org/other-repo" {
				t.Error("binding still present after delete")
			}
		}
	})

	t.Run("DeleteRemote missing returns ErrBindingNotFound", func(t *testing.T) {
		err := st.DeleteRemote(ctx, "u-bind", "github.com/no/such-repo")
		if !errors.Is(err, ports.ErrBindingNotFound) {
			t.Errorf("want ErrBindingNotFound, got %v", err)
		}
	})

	t.Run("cascade on project delete", func(t *testing.T) {
		// the current binding in u-bind points to p-bind-2; deleting that project
		// should cascade-delete the binding
		const delQ = `DELETE FROM projects WHERE id=$1`
		if _, err := pool.Exec(ctx, delQ, "p-bind-2"); err != nil {
			t.Fatal(err)
		}
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.ProjectID == "p-bind-2" {
				t.Error("binding not cascade-deleted with project")
			}
		}
	})
}
