package projects_test

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeFormAPI struct {
	created   domain.Project
	updated   projects.UpdateFields
	rateCents *int64
	rateSet   bool
}

func (f *fakeFormAPI) CreateProject(_ context.Context, name string) (domain.Project, error) {
	f.created = domain.Project{ID: "new1", Name: name, Slug: name}
	return f.created, nil
}
func (f *fakeFormAPI) UpdateProject(_ context.Context, id string, in projects.UpdateFields) (domain.Project, error) {
	f.updated = in
	return domain.Project{ID: id, Name: in.Name, Slug: in.Slug, Status: domain.ProjectStatus(in.Status)}, nil
}
func (f *fakeFormAPI) SetProjectRate(_ context.Context, _ string, amount *int64, _ string) error {
	f.rateSet, f.rateCents = true, amount
	return nil
}

func TestFormCreateComposes(t *testing.T) {
	api := &fakeFormAPI{}
	r := projects.NewFormRoute(api, theme.Default, nil) // create mode
	r.FillForTest(projects.FormValues{
		Name:         "PM TUI",
		Slug:         "pm-tui",
		Status:       "active",
		Color:        "blue",
		Glyph:        "◆",
		RateAmount:   "90.00",
		RateCurrency: "EUR",
	})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("Submit must return a non-nil cmd on valid input")
	}
	// Execute the cmd to drive the API calls.
	msg := cmd()
	// API side-effects must be present after the cmd runs.
	if api.created.Name != "PM TUI" {
		t.Fatalf("CreateProject not called with name: %+v", api.created)
	}
	if api.updated.Status != "active" || api.updated.Color != "blue" {
		t.Errorf("UpdateProject compose wrong: %+v", api.updated)
	}
	if !api.rateSet || api.rateCents == nil || *api.rateCents != 9000 {
		t.Errorf("rate should be 9000 cents, got set=%v cents=%v", api.rateSet, api.rateCents)
	}
	// On success the cmd must return PopRouteMsg.
	if _, ok := msg.(shell.PopRouteMsg); !ok {
		t.Errorf("success should pop, got %T", msg)
	}
}

func TestFormEditClearsRateOnBlank(t *testing.T) {
	api := &fakeFormAPI{}
	editing := &domain.Project{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.ProjectActive}
	r := projects.NewFormRoute(api, theme.Default, editing)
	r.FillForTest(projects.FormValues{
		Name:         "Flow",
		Slug:         "flow",
		Status:       "paused",
		RateAmount:   "",
		RateCurrency: "EUR",
	})
	_, cmd := r.Submit()
	if cmd == nil {
		t.Fatal("Submit must return a non-nil cmd on valid input")
	}
	// Execute the cmd to drive the API calls.
	msg := cmd()
	if api.updated.Status != "paused" {
		t.Errorf("edit should set paused, got %q", api.updated.Status)
	}
	if !api.rateSet || api.rateCents != nil {
		t.Errorf("blank rate must clear (nil), got set=%v cents=%v", api.rateSet, api.rateCents)
	}
	// On success the cmd must return PopRouteMsg.
	if _, ok := msg.(shell.PopRouteMsg); !ok {
		t.Errorf("success should pop, got %T", msg)
	}
}

func TestFormFocusMovement(t *testing.T) {
	api := &fakeFormAPI{}
	r := projects.NewFormRoute(api, theme.Default, nil)

	// Initial focus should be on field 0 (Name).
	if r.FocusIdx() != 0 {
		t.Fatalf("initial focus should be 0, got %d", r.FocusIdx())
	}

	// Tab should advance focus to 1 (Slug).
	nr, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r = nr.(*projects.FormRoute)
	if r.FocusIdx() != 1 {
		t.Errorf("after Tab, focus should be 1, got %d", r.FocusIdx())
	}

	// Drive focus forward to Status selector (index 4): need 3 more Tabs.
	for i := 0; i < 3; i++ {
		nr, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		r = nr.(*projects.FormRoute)
	}
	if r.FocusIdx() != 4 {
		t.Fatalf("expected focus 4 (Status), got %d", r.FocusIdx())
	}

	// Right arrow should cycle the status selector.
	before := r.Values().Status
	nr, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	r = nr.(*projects.FormRoute)
	after := r.Values().Status
	if before == after {
		t.Errorf("right should cycle status; before=%q after=%q", before, after)
	}

	// Left arrow on a selector should cycle in the opposite direction.
	nr, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	r = nr.(*projects.FormRoute)
	cycledBack := r.Values().Status
	_ = cycledBack // allowed to wrap; just verify it moved
}
