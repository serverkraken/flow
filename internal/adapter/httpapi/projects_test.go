package httpapi_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/ports"
)

func TestProjects_EnsureBySlug_CreateAndGet(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	// Create a new project
	proj, err := projects.EnsureBySlug("", "My Test Project", "my-test-project")
	if err != nil {
		t.Fatalf("EnsureBySlug create: %v", err)
	}
	if proj.ID == "" {
		t.Fatal("expected project ID to be set")
	}
	if proj.Name != "My Test Project" {
		t.Errorf("name = %q, want %q", proj.Name, "My Test Project")
	}

	// Idempotent call returns existing project
	proj2, err := projects.EnsureBySlug("", "My Test Project", "my-test-project")
	if err != nil {
		t.Fatalf("EnsureBySlug idempotent: %v", err)
	}
	if proj2.ID != proj.ID {
		t.Errorf("expected same ID on second call; got %q and %q", proj.ID, proj2.ID)
	}
}

func TestProjects_ListActive(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	_, err := projects.EnsureBySlug("", "List Active Test", "list-active-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	active, err := projects.ListActive("")
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	found := false
	for _, p := range active {
		if p.Slug == "list-active-test" {
			found = true
			if p.ArchivedAt != nil {
				t.Error("active project has ArchivedAt set")
			}
		}
	}
	if !found {
		t.Error("created project not in ListActive result")
	}
}

func TestProjects_ListAll(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	p, err := projects.EnsureBySlug("", "List All Test", "list-all-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	// Archive it
	if err := projects.Archive("", p.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// ListActive should NOT include it
	active, err := projects.ListActive("")
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	for _, proj := range active {
		if proj.ID == p.ID {
			t.Error("archived project appears in ListActive")
		}
	}

	// ListAll SHOULD include it
	all, err := projects.ListAll("")
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	found := false
	for _, proj := range all {
		if proj.ID == p.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("archived project not found in ListAll")
	}
}

func TestProjects_GetByID(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	created, err := projects.EnsureBySlug("", "Get By ID Test", "get-by-id-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	found, err := projects.GetByID("", created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, created.ID)
	}
}

func TestProjects_GetByID_NotFound(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	_, err := projects.GetByID("", "00000000-0000-0000-0000-000000000000")
	if err != ports.ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got: %v", err)
	}
}

func TestProjects_GetBySlug_NotFound(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	_, err := projects.GetBySlug("", "nonexistent-slug-xyzzy")
	if err != ports.ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got: %v", err)
	}
}

func TestProjects_TouchLastUsed_NoOp(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	// Should not error — it's a documented no-op
	if err := projects.TouchLastUsed("", "any-id"); err != nil {
		t.Errorf("TouchLastUsed returned error: %v", err)
	}
}

func TestProjects_Offline_FallsBackToCache(t *testing.T) {
	api := newTestAPI(t)
	projects := httpapi.NewProjects(api.Client)

	_, err := projects.EnsureBySlug("", "Offline Project Test", "projects-offline-test")
	if err != nil {
		t.Fatalf("EnsureBySlug: %v", err)
	}

	// Populate cache with initial load
	first, err := projects.ListAll("")
	if err != nil {
		t.Fatalf("initial ListAll: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected projects in initial list")
	}

	// Kill the server
	api.Close()

	// Should return cached data
	second, err := projects.ListAll("")
	if err != nil {
		t.Fatalf("offline ListAll returned error: %v", err)
	}
	if len(second) == 0 {
		t.Error("expected cached projects offline, got empty slice")
	}
}
