// internal/adapter/pgstore/projects_test.go
package pgstore_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// mustUser legt einen frischen User für Test-Isolation an (alle Tabellen
// sind user-gescoped; der geteilte Container braucht keine Truncates).
func mustUser(t *testing.T, sub string) string {
	t.Helper()
	u, err := pgstore.NewUsers(testStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("mustUser(%s): %v", sub, err)
	}
	return u.ID
}

func TestProjects_EnsureListArchive(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-1")

	proj, err := p.EnsureBySlug(uid, "Mein Projekt", "mein-projekt")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}
	if proj.Name != "Mein Projekt" || proj.Version != 1 {
		t.Fatalf("unexpected project: %+v", proj)
	}

	// idempotent: zweiter Ensure liefert dieselbe Row, legt nichts Neues an
	again, err := p.EnsureBySlug(uid, "ignoriert", "mein-projekt")
	if err != nil {
		t.Fatalf("EnsureBySlug again: %v", err)
	}
	if again.ID != proj.ID || again.Name != "Mein Projekt" {
		t.Errorf("EnsureBySlug not idempotent: %+v", again)
	}

	active, err := p.ListActive(uid)
	if err != nil || len(active) != 1 {
		t.Fatalf("ListActive: %v len=%d", err, len(active))
	}

	if err := p.Archive(uid, proj.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	active, _ = p.ListActive(uid)
	if len(active) != 0 {
		t.Errorf("after Archive ListActive should be empty, got %d", len(active))
	}
	all, _ := p.ListAll(uid)
	if len(all) != 1 || all[0].ArchivedAt == nil {
		t.Errorf("ListAll should contain archived project with ArchivedAt set: %+v", all)
	}
}

func TestProjects_UpsertOCC(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-2")
	proj, _ := p.EnsureBySlug(uid, "A", "a")

	proj.Name = "A umbenannt"
	saved, err := p.Upsert(proj, proj.Version)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if saved.Version != proj.Version+1 || saved.Name != "A umbenannt" {
		t.Errorf("version/name after upsert: %+v", saved)
	}

	// stale write → Konflikt
	if _, err := p.Upsert(proj, proj.Version); !errors.Is(err, ports.ErrProjectVersionConflict) {
		t.Errorf("stale upsert: want ErrProjectVersionConflict, got %v", err)
	}
}

func TestProjects_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	p := pgstore.NewProjects(testStore)
	uid := mustUser(t, "proj-3")
	if _, err := p.GetByID(uid, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("want ErrProjectNotFound, got %v", err)
	}
	var _ domain.Project // keep import obvious
}
