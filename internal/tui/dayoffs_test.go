package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestModel_TogglesDayOffView(t *testing.T) {
	m := New(nil, "msoent")
	// 'd' switches to the dayoff view.
	updated, _ := m.Update(tea.KeyPressMsg{Text: "d"})
	if !updated.(Model).showDayOffs {
		t.Fatal("expected dayoff view active after 'd'")
	}
	// 'esc' returns to worktime.
	back, _ := updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(Model).showDayOffs {
		t.Fatal("expected worktime view after esc")
	}
}

func TestModel_DayOffViewRenders(t *testing.T) {
	m := New(nil, "msoent")
	// Feed loaded dayoffs through Update, then render the dayoff view.
	loaded, _ := m.Update(dayoffsLoadedMsg{list: []apiclient.DayOff{
		{Day: "2026-06-15", Kind: "vacation", Label: "Sommer"},
		{Day: "2026-01-01", Kind: "holiday", Label: "Neujahr", Holiday: true},
	}})
	mm := loaded.(Model)
	mm.showDayOffs = true
	out := mm.View().Content
	if !strings.Contains(out, "flow · dayoffs") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "2026-06-15") || !strings.Contains(out, "Sommer") {
		t.Fatalf("missing vacation entry:\n%s", out)
	}
	if !strings.Contains(out, "Neujahr") {
		t.Fatalf("missing holiday entry:\n%s", out)
	}
}

func TestModel_DayOffViewEmpty(t *testing.T) {
	m := New(nil, "msoent")
	m.showDayOffs = true
	if out := m.View().Content; !strings.Contains(out, "no day-offs this year") {
		t.Fatalf("empty state not rendered:\n%s", out)
	}
}
