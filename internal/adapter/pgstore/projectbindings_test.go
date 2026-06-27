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

	projects := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	p1, _ := domain.NewNode("p-bind-1", "u-bind", "Project 1", "project-1", now)
	p1.Kind = domain.KindEngagement
	p2, _ := domain.NewNode("p-bind-2", "u-bind", "Project 2", "project-2", now)
	p2.Kind = domain.KindEngagement
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
			NodeID:  "p-bind-1",
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
		if got.NodeID != "p-bind-1" {
			t.Errorf("want project p-bind-1, got %q", got.NodeID)
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
			NodeID:  "p-bind-2",
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
		if got.NodeID != "p-bind-2" {
			t.Errorf("want project reassigned to p-bind-2, got %q", got.NodeID)
		}

		// verify exactly one row in the DB
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 1 {
			t.Fatalf("want 1 binding after re-upsert, got %d", len(all))
		}
		if all[0].NodeID != "p-bind-2" {
			t.Errorf("stored binding still has old project %q", all[0].NodeID)
		}
	})

	t.Run("List is owner-scoped", func(t *testing.T) {
		// upsert a binding for a different owner — should not appear in u-bind's list
		u2, _ := domain.NewUser("u-other", "sub-other", "other", "other@x.de", "Other")
		if _, err := users.UpsertBySub(ctx, u2); err != nil {
			t.Fatal(err)
		}
		p3, _ := domain.NewNode("p-other-1", "u-other", "Other P", "other-p", now)
		p3.Kind = domain.KindEngagement
		if _, err := projects.Create(ctx, p3); err != nil {
			t.Fatal(err)
		}
		bOther := domain.ProjectBinding{
			ID:         "bind-other",
			OwnerID:    "u-other",
			NodeID:  "p-other-1",
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
			NodeID:  "p-bind-1",
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
		const delQ = `DELETE FROM nodes WHERE id=$1`
		if _, err := pool.Exec(ctx, delQ, "p-bind-2"); err != nil {
			t.Fatal(err)
		}
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.NodeID == "p-bind-2" {
				t.Error("binding not cascade-deleted with project")
			}
		}
	})

	// Re-create p-bind-2 (was cascade-deleted above) so path sub-tests can use both projects.
	p2recreated, _ := domain.NewNode("p-bind-2", "u-bind", "Project 2", "project-2", now)
	p2recreated.Kind = domain.KindEngagement
	if _, err := projects.Create(ctx, p2recreated); err != nil {
		t.Fatal(err)
	}

	t.Run("path: upsert creates binding", func(t *testing.T) {
		b := domain.ProjectBinding{
			ID:           "path-bind-1",
			OwnerID:      "u-bind",
			NodeID:    "p-bind-1",
			Kind:         domain.BindingPath,
			MachineID:    "machine-a",
			MachineLabel: "laptop",
			Path:         "/a/b",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		got, err := st.Upsert(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != domain.BindingPath {
			t.Errorf("want kind path, got %q", got.Kind)
		}
		if got.NodeID != "p-bind-1" {
			t.Errorf("want project p-bind-1, got %q", got.NodeID)
		}
		if got.MachineID != "machine-a" {
			t.Errorf("want machine-a, got %q", got.MachineID)
		}
		if got.Path != "/a/b" {
			t.Errorf("want path /a/b, got %q", got.Path)
		}
	})

	t.Run("path: re-upsert same (owner,machine,path) reassigns project to p2", func(t *testing.T) {
		b2 := domain.ProjectBinding{
			ID:           "path-bind-1b", // different id — conflict target must win
			OwnerID:      "u-bind",
			NodeID:    "p-bind-2",
			Kind:         domain.BindingPath,
			MachineID:    "machine-a",
			MachineLabel: "laptop-renamed",
			Path:         "/a/b",
			CreatedAt:    now,
			UpdatedAt:    now.Add(time.Second),
		}
		got, err := st.Upsert(ctx, b2)
		if err != nil {
			t.Fatal(err)
		}
		if got.NodeID != "p-bind-2" {
			t.Errorf("want project reassigned to p-bind-2, got %q", got.NodeID)
		}
		if got.MachineLabel != "laptop-renamed" {
			t.Errorf("want machine_label updated to laptop-renamed, got %q", got.MachineLabel)
		}

		// verify exactly ONE path row for (owner, machine, /a/b)
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		var pathRows []domain.ProjectBinding
		for _, b := range all {
			if b.Kind == domain.BindingPath && b.MachineID == "machine-a" && b.Path == "/a/b" {
				pathRows = append(pathRows, b)
			}
		}
		if len(pathRows) != 1 {
			t.Fatalf("want exactly 1 path row for (machine-a, /a/b), got %d", len(pathRows))
		}
		if pathRows[0].NodeID != "p-bind-2" {
			t.Errorf("stored path binding still has old project %q", pathRows[0].NodeID)
		}
	})

	t.Run("path: different path is a separate row", func(t *testing.T) {
		b := domain.ProjectBinding{
			ID:           "path-bind-2",
			OwnerID:      "u-bind",
			NodeID:    "p-bind-1",
			Kind:         domain.BindingPath,
			MachineID:    "machine-a",
			MachineLabel: "laptop",
			Path:         "/a/c",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := st.Upsert(ctx, b); err != nil {
			t.Fatal(err)
		}

		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		var pathRows []domain.ProjectBinding
		for _, b := range all {
			if b.Kind == domain.BindingPath && b.MachineID == "machine-a" {
				pathRows = append(pathRows, b)
			}
		}
		if len(pathRows) != 2 {
			t.Fatalf("want 2 path rows for machine-a (paths /a/b and /a/c), got %d", len(pathRows))
		}
	})

	t.Run("path: ListByProject returns path rows", func(t *testing.T) {
		byP1, err := st.ListByProject(ctx, "u-bind", "p-bind-1")
		if err != nil {
			t.Fatal(err)
		}
		var pathForP1 int
		for _, b := range byP1 {
			if b.Kind == domain.BindingPath {
				pathForP1++
			}
		}
		if pathForP1 != 1 {
			t.Errorf("want 1 path binding for p-bind-1, got %d", pathForP1)
		}
	})

	t.Run("path: DeletePath removes only the target row", func(t *testing.T) {
		if err := st.DeletePath(ctx, "u-bind", "machine-a", "/a/b"); err != nil {
			t.Fatal(err)
		}
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.Kind == domain.BindingPath && b.Path == "/a/b" {
				t.Error("path binding /a/b still present after DeletePath")
			}
		}
		// /a/c must still be there
		var found bool
		for _, b := range all {
			if b.Kind == domain.BindingPath && b.Path == "/a/c" {
				found = true
			}
		}
		if !found {
			t.Error("path binding /a/c was unexpectedly deleted")
		}
	})

	t.Run("path: DeletePath missing returns ErrBindingNotFound", func(t *testing.T) {
		err := st.DeletePath(ctx, "u-bind", "machine-a", "/no/such/path")
		if !errors.Is(err, ports.ErrBindingNotFound) {
			t.Errorf("want ErrBindingNotFound, got %v", err)
		}
	})

	t.Run("path: cascade on project delete removes path binding", func(t *testing.T) {
		// /a/c points to p-bind-1; deleting p-bind-1 must cascade
		const delQ = `DELETE FROM nodes WHERE id=$1`
		if _, err := pool.Exec(ctx, delQ, "p-bind-1"); err != nil {
			t.Fatal(err)
		}
		all, err := st.List(ctx, "u-bind")
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			if b.NodeID == "p-bind-1" {
				t.Error("path binding not cascade-deleted with project")
			}
		}
	})

	t.Run("path: remote and path bindings for same owner+project coexist", func(t *testing.T) {
		// Re-create p-bind-1 (just cascade-deleted above) for this coexistence test.
		p1recreated, _ := domain.NewNode("p-bind-coex", "u-bind", "Coex Project", "coex-project", now)
		p1recreated.Kind = domain.KindEngagement
		if _, err := projects.Create(ctx, p1recreated); err != nil {
			t.Fatal(err)
		}

		remote := domain.ProjectBinding{
			ID:         "coex-remote",
			OwnerID:    "u-bind",
			NodeID:  "p-bind-coex",
			Kind:       domain.BindingRemote,
			RemoteSlug: "github.com/org/coex-repo",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		path := domain.ProjectBinding{
			ID:           "coex-path",
			OwnerID:      "u-bind",
			NodeID:    "p-bind-coex",
			Kind:         domain.BindingPath,
			MachineID:    "machine-b",
			MachineLabel: "desktop",
			Path:         "/work/coex",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if _, err := st.Upsert(ctx, remote); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Upsert(ctx, path); err != nil {
			t.Fatal(err)
		}

		byProject, err := st.ListByProject(ctx, "u-bind", "p-bind-coex")
		if err != nil {
			t.Fatal(err)
		}
		var hasRemote, hasPath bool
		for _, b := range byProject {
			if b.Kind == domain.BindingRemote {
				hasRemote = true
			}
			if b.Kind == domain.BindingPath {
				hasPath = true
			}
		}
		if !hasRemote {
			t.Error("coexistence test: remote binding missing from ListByProject")
		}
		if !hasPath {
			t.Error("coexistence test: path binding missing from ListByProject")
		}
		if len(byProject) != 2 {
			t.Errorf("want 2 bindings for coex project (1 remote + 1 path), got %d", len(byProject))
		}
	})
}
