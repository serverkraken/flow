package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestDayOffReader_DelegatesToStore(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	store := testutil.NewFakeDayOffStore(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "T"})
	r := &usecase.DayOffReader{Store: store}

	if got, ok := r.Lookup(d); !ok || got.Label != "T" {
		t.Errorf("Lookup hit failed: got %+v ok %v", got, ok)
	}
	if _, ok := r.Lookup(time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local)); ok {
		t.Error("Lookup miss should return ok=false")
	}
	got := r.List(time.Time{}, time.Time{})
	if len(got) != 1 {
		t.Errorf("List unbounded: got %d entries", len(got))
	}
}

func TestLinkReader_DelegatesToStore(t *testing.T) {
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	store := &testutil.FakeLinkStore{ByDate: map[string][]string{
		d.Format("2006-01-02"): {"daily/2026-05-01"},
	}}
	r := &usecase.LinkReader{Store: store}

	got, err := r.ListByDate(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "daily/2026-05-01" {
		t.Errorf("got %+v", got)
	}
}

func TestLinkReader_PropagatesError(t *testing.T) {
	r := &usecase.LinkReader{Store: &testutil.FakeLinkStore{Err: errors.New("boom")}}
	if _, err := r.ListByDate(time.Now()); err == nil {
		t.Error("expected error")
	}
}

func TestCheatsheetLoader_LoadRaw(t *testing.T) {
	l := &usecase.CheatsheetLoader{
		Reader:   &testutil.FakeCheatsheetReader{Content: "# Hello"},
		Renderer: &testutil.FakeMarkdownRenderer{},
	}
	got, err := l.LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	if got != "# Hello" {
		t.Errorf("got %q", got)
	}
}

func TestCheatsheetLoader_Render(t *testing.T) {
	l := &usecase.CheatsheetLoader{
		Reader:   &testutil.FakeCheatsheetReader{Content: "# Hello"},
		Renderer: &testutil.FakeMarkdownRenderer{Prefix: "[r] "},
	}
	got, err := l.Render(80)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[r] # Hello" {
		t.Errorf("got %q", got)
	}
	if l.Renderer.(*testutil.FakeMarkdownRenderer).LastWidth != 80 {
		t.Errorf("LastWidth = %d, want 80", l.Renderer.(*testutil.FakeMarkdownRenderer).LastWidth)
	}
}

func TestCheatsheetLoader_Render_ReadError(t *testing.T) {
	l := &usecase.CheatsheetLoader{
		Reader:   &testutil.FakeCheatsheetReader{Err: errors.New("boom")},
		Renderer: &testutil.FakeMarkdownRenderer{},
	}
	if _, err := l.Render(80); err == nil {
		t.Error("expected error")
	}
}

func TestStateManager_Restore_NoNextScreenUsesPersisted(t *testing.T) {
	store := &testutil.FakeFlowStateStore{
		State: domain.FlowState{Screen: domain.ScreenWorktime, Filter: "deep", Cursor: 5},
	}
	got, err := (&usecase.StateManager{Store: store}).Restore()
	if err != nil {
		t.Fatal(err)
	}
	if got.Screen != domain.ScreenWorktime || got.Filter != "deep" || got.Cursor != 5 {
		t.Errorf("got %+v, want persisted", got)
	}
}

func TestStateManager_Restore_NextScreenOverridesAndResets(t *testing.T) {
	store := &testutil.FakeFlowStateStore{
		State:      domain.FlowState{Screen: domain.ScreenWorktime, Filter: "deep", Cursor: 5},
		NextScreen: domain.ScreenProjects,
	}
	got, err := (&usecase.StateManager{Store: store}).Restore()
	if err != nil {
		t.Fatal(err)
	}
	if got.Screen != domain.ScreenProjects {
		t.Errorf("Screen = %q, want next-screen override", got.Screen)
	}
	if got.Filter != "" || got.Cursor != 0 {
		t.Errorf("filter/cursor should reset on deep-link, got %+v", got)
	}
	// And the marker should have been consumed (single-shot).
	if store.NextScreen != "" {
		t.Errorf("NextScreen should be cleared after consume, got %q", store.NextScreen)
	}
}

func TestStateManager_Restore_LoadErrorReturnsDefault(t *testing.T) {
	store := &testutil.FakeFlowStateStore{LoadErr: errors.New("boom")}
	got, err := (&usecase.StateManager{Store: store}).Restore()
	if err == nil {
		t.Error("expected error propagated")
	}
	if got.Screen != domain.ScreenPalette {
		t.Errorf("on error should return DefaultFlowState (Palette), got %+v", got)
	}
}

func TestStateManager_Save(t *testing.T) {
	store := &testutil.FakeFlowStateStore{}
	mgr := &usecase.StateManager{Store: store}
	want := domain.FlowState{Screen: domain.ScreenCheatsheet, Filter: "x", Cursor: 3}
	if err := mgr.Save(want); err != nil {
		t.Fatal(err)
	}
	if store.State != want {
		t.Errorf("Save did not persist: got %+v want %+v", store.State, want)
	}
}

func TestStateManager_WriteNextScreen(t *testing.T) {
	store := &testutil.FakeFlowStateStore{}
	mgr := &usecase.StateManager{Store: store}
	if err := mgr.WriteNextScreen(domain.ScreenWorktime); err != nil {
		t.Fatal(err)
	}
	if store.NextScreen != domain.ScreenWorktime {
		t.Errorf("NextScreen = %q", store.NextScreen)
	}
}
