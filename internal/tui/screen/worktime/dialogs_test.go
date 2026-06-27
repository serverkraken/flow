package worktime

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
)

func bookingRoute(api todayAPI) *TodayRoute {
	r := NewTodayRoute(api, time.Now, theme.Default, nil)
	r.st.Running = true
	r.st.ActiveID = "sess-1"
	r.dialog = dialogBooking
	r.booking = bookingState{list: fuzzylist.New([]fuzzylist.Item{{ID: "p1", Label: "flow"}}, theme.Default).WithCreateHint("neu: %s")}
	return r
}

func TestBooking_SelectExistingProject(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	r := bookingRoute(api)
	// cursor on the single project, Enter → StopSession with p1
	_, cmd := r.handleBookingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a stop command")
	}
	cmd() // run the async closure
	if api.stopped[1] != "p1" {
		t.Errorf("stopProjectID = %q, want p1", api.stopped[1])
	}
}

func TestBooking_InlineCreate(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	r := bookingRoute(api)
	// type a new name, move onto the create row, Enter → CreateNode + Stop
	r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Text: "z"})
	for i := 0; i < 5; i++ {
		r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, cmd := r.handleBookingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a create+stop command")
	}
	cmd()
	// fakeAPI.CreateNode returns domain.Node{ID: "p-z", Name: "z"}
	if api.stopped[1] != "p-z" {
		t.Errorf("stopProjectID = %q, want p-z", api.stopped[1])
	}
}

func TestBooking_OnlyEngagementsListed(t *testing.T) {
	t.Parallel()
	nodes := []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "RTL Extern"},
		{ID: "r1", Kind: domain.KindRepo, Name: "flow"},
	}
	got := engagementItems(mruEngagements(nodes, nil))
	if len(got) != 1 || got[0].ID != "e1" {
		t.Fatalf("booking list must contain only engagements, got %+v", got)
	}
}

func TestBooking_InlineCreate_SendsEngagementKind(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{createKind: ""}
	r := bookingRoute(api)
	// type a new name, move onto the create row
	r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Text: "myeng"})
	for i := 0; i < 5; i++ {
		r.booking.list = r.booking.list.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	_, cmd := r.handleBookingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a create+stop command")
	}
	cmd()
	if api.createKind != string(domain.KindEngagement) {
		t.Errorf("CreateNode Kind = %q, want %q", api.createKind, string(domain.KindEngagement))
	}
}
