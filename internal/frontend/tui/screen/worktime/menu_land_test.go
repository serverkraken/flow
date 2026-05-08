// White-box tests for the Bundesland-Picker.

package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// dayoffRig wires the bare minimum to dispatch landSyncCmd: a clock,
// a FakeDayOffStore behind a DayOffWriter use case, and a dummy
// reader so the menu's predicate doesn't blow up.
type dayoffRig struct {
	deps  Deps
	store *testutil.FakeDayOffStore
}

func newDayoffRig(t *testing.T) dayoffRig {
	t.Helper()
	clock := &testutil.FixedClock{T: time.Date(2026, 5, 6, 14, 0, 0, 0, time.Local)}
	store := testutil.NewFakeDayOffStore()
	writer := &usecase.DayOffWriter{Store: store}
	return dayoffRig{
		deps: Deps{
			DayOffWriter: writer,
			Clock:        clock,
		},
		store: store,
	}
}

func TestIndexOfLand_KnownAndAlias(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"NW", 10},
		{"nw", 10},
		{"  NW  ", 10},
		{"NRW", 10}, // documented alias from the CLI
		{"DE", 0},
		{"BW", 1},
		{"unknown", -1},
		{"", -1},
	}
	for _, c := range cases {
		if got := indexOfLand(c.in); got != c.want {
			t.Errorf("indexOfLand(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNewLandPicker_StartsOnCurrentLand(t *testing.T) {
	lp := newLandPicker("NW")
	if lp.cursor != indexOfLand("NW") {
		t.Errorf("cursor = %d, want %d", lp.cursor, indexOfLand("NW"))
	}
}

func TestNewLandPicker_FallsBackToFirstWhenUnknown(t *testing.T) {
	lp := newLandPicker("zzz")
	if lp.cursor != 0 {
		t.Errorf("cursor = %d, want 0", lp.cursor)
	}
}

func TestLandPicker_NavigationWraps(t *testing.T) {
	lp := newLandPicker("DE")
	lp, _ = lp.handleKey(keyName("k"))
	if lp.cursor != len(landEntries)-1 {
		t.Errorf("k from 0 should wrap to last, got %d", lp.cursor)
	}
	lp, _ = lp.handleKey(keyName("j"))
	if lp.cursor != 0 {
		t.Errorf("j from last should wrap to first, got %d", lp.cursor)
	}
}

func TestLandPicker_GAndShiftGJump(t *testing.T) {
	lp := newLandPicker("DE")
	lp, _ = lp.handleKey(runeKey('G'))
	if lp.cursor != len(landEntries)-1 {
		t.Errorf("G should jump to last, got %d", lp.cursor)
	}
	lp, _ = lp.handleKey(runeKey('g'))
	if lp.cursor != 0 {
		t.Errorf("g should jump to first, got %d", lp.cursor)
	}
}

func TestLandPicker_EnterPicksFocused(t *testing.T) {
	lp := newLandPicker("BY")
	_, ev := lp.handleKey(keyName("enter"))
	if !ev.picked {
		t.Fatal("Enter should pick the focused entry")
	}
	if ev.entry.code != "BY" {
		t.Errorf("picked code = %q, want BY", ev.entry.code)
	}
}

func TestLandPicker_EscCancels(t *testing.T) {
	lp := newLandPicker("BY")
	_, ev := lp.handleKey(keyName("esc"))
	if !ev.canceled {
		t.Error("Esc should cancel")
	}
}

func TestLandPicker_ViewRendersAllEntriesAndCodes(t *testing.T) {
	lp := newLandPicker("DE")
	out := lp.view("Land für Feiertage", pal(), 130)
	for _, want := range []string{
		"Land für Feiertage",
		"Deutschland (bundesweit)",
		"Bayern",
		"Nordrhein-Westfalen",
		"Thüringen",
		"BUNDESLAND",
		"j/k → bewegen",
		"enter → syncen",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("land view missing %q in:\n%s", want, out)
		}
	}
}

func TestLandSyncCmd_AddsHolidaysAndToasts(t *testing.T) {
	r := newDayoffRig(t)
	cmd := landSyncCmd(r.deps, "BY")
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("sync err = %v", done.err)
	}
	if len(r.store.Entries) == 0 {
		t.Error("SyncGermanHolidays should have populated the store")
	}
	if !strings.Contains(done.toast, "BY/2026") {
		t.Errorf("toast = %q, want BY/2026 mention", done.toast)
	}
}

func TestLandSyncCmd_FailsWithoutWriter(t *testing.T) {
	cmd := landSyncCmd(Deps{Clock: &testutil.FixedClock{T: time.Now()}}, "DE")
	if cmd().(menuActionDoneMsg).err == nil {
		t.Error("sync without DayOffWriter must fail cleanly")
	}
}

// — menu integration: Enter on Land → picker → Enter on Land → SyncGermanHolidays —

func TestMenu_LandFlowSyncsHolidays(t *testing.T) {
	r := newDayoffRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(140, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionLand {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeLand {
		t.Fatalf("after Enter on Land, subMode = %v, want land", m.subMode)
	}
	// Pick BY (index 2) — j twice from default cursor (NW = 10) wraps
	// up; simpler: Set cursor explicitly.
	m.landP.cursor = indexOfLand("BY")
	m, cmd := m.handleKey(keyName("enter"))
	if cmd == nil {
		t.Fatal("Land pick should return a dispatch tea.Cmd")
	}
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("dispatch err = %v", done.err)
	}
	if len(r.store.Entries) == 0 {
		t.Error("SyncGermanHolidays must populate the store")
	}
	if m.subMode != menuSubModeList {
		t.Errorf("after dispatch, subMode = %v, want list", m.subMode)
	}
}

func TestMenu_LandFlowEscReturnsToList(t *testing.T) {
	r := newDayoffRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(140, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionLand {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeLand {
		t.Fatal("precondition: must be in land sub-mode")
	}
	m, _ = m.handleKey(keyName("esc"))
	if m.subMode != menuSubModeList {
		t.Errorf("Esc should return to list, got %v", m.subMode)
	}
	if len(r.store.Entries) != 0 {
		t.Error("Esc must NOT trigger sync")
	}
}
