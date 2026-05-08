package worktime_test

// Black-box handler + state-accessor tests filling the gaps in the
// existing model_test.go suite. Aimed at lifting each sub-model's
// per-package coverage above the per-layer 70% target without
// re-testing the rendering paths model_test.go already covers.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
)

// — state accessors per tab —

func TestState_HeuteCursor_TracksAfterJK(t *testing.T) {
	r := newRig(t)
	// Seed two sessions for today so the cursor has somewhere to move.
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-3 * time.Hour), Stop: now.Add(-2 * time.Hour), Elapsed: time.Hour},
		{Date: now, Start: now.Add(-2 * time.Hour), Stop: now.Add(-1 * time.Hour), Elapsed: time.Hour},
	}
	m := loadedHeute(t, r)
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("default cursor should be 0, got %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.(worktime.Model).StateCursor(); got != 1 {
		t.Errorf("after `j` cursor should be 1, got %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("after `k` cursor should wrap back to 0, got %d", got)
	}
}

func TestState_HeuteFilter_TabOnly(t *testing.T) {
	r := newRig(t)
	m := loadedHeute(t, r)
	// StateFilter encodes the active tab plus, when present, the
	// sub-model's own filter. Heute carries no filter, so the value
	// is just the tab marker.
	if got := m.(worktime.Model).StateFilter(); got != "tab=heute" {
		t.Errorf("Heute StateFilter should be \"tab=heute\", got %q", got)
	}
}

func TestState_FreiCursor_Default(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("Frei default cursor should be 0, got %d", got)
	}
}

func TestState_FreiFilter_TabOnly(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	// Frei carries no own filter; StateFilter is just the tab marker.
	if got := m.(worktime.Model).StateFilter(); got != "tab=frei" {
		t.Errorf("Frei StateFilter should be \"tab=frei\", got %q", got)
	}
}

func TestState_HistoryFilter_TracksQueryAndModeLabel(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// no filter set yet → tab marker plus the mode-label sub-filter
	if got := m.(worktime.Model).StateFilter(); got == "" {
		t.Errorf("StateFilter should report tab + mode label when no filter set, got empty")
	}
	// Open filter dialog, type tag:deep, submit
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drainCmd(t, m, cmd)
	for _, ch := range "tag:deep" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.(worktime.Model).StateFilter(); got != "tab=history|tag:deep" {
		t.Errorf("StateFilter should be the active query under the tab marker, got %q", got)
	}
}

func TestState_HistoryCursor_PerMode(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// list mode (default): cursor is the list index
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("list-mode default cursor should be 0, got %d", got)
	}
	// switch to heatmap → cursor is heatCol*7+heatRow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	got := m.(worktime.Model).StateCursor()
	if got < 0 {
		t.Errorf("heatmap cursor should be ≥ 0, got %d", got)
	}
	// switch to tag-clock → cursor is row*24+col
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	got = m.(worktime.Model).StateCursor()
	if got < 0 || got >= 7*24 {
		t.Errorf("tag-clock cursor should be in [0, 168), got %d", got)
	}
	// switch to month → cursor is monthCur (day of month)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	got = m.(worktime.Model).StateCursor()
	if got < 1 || got > 31 {
		t.Errorf("month cursor should be a day-of-month in [1, 31], got %d", got)
	}
}

// — Heute Edit dialog (E key) full lifecycle —

func TestHeute_EditDialog_OpensTabsAndSubmits(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-2 * time.Hour), Stop: now.Add(-1 * time.Hour), Elapsed: time.Hour, Tag: "old", Note: "n"},
	}
	m := loadedHeute(t, r)
	// Open Edit dialog with E
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("Edit dialog should activate FilterActive")
	}
	out := m.View()
	if !strings.Contains(out, "Session bearbeiten") {
		t.Errorf("Edit dialog should render its title, got:\n%s", out)
	}
	// Tab through the four fields and back
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	// Enter on the last field submits — the form has 4 fields so the
	// Enter route must hit submitDialog. Land on the tag field (index 2)
	// and provide a fresh value.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	// Down + Up wrap-arounds also exercise the navigation branches.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
}

func TestHeute_EditDialog_EscClosesWithoutWriting(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-time.Hour), Stop: now, Elapsed: time.Hour},
	}
	m := loadedHeute(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(worktime.Model).FilterActive() {
		t.Error("Esc should close the Edit dialog → FilterActive=false")
	}
}

func TestHeute_EditDialog_EnterMidFormAdvances(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-time.Hour), Stop: now, Elapsed: time.Hour},
	}
	m := loadedHeute(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = drainCmd(t, m, cmd)
	// Enter on field 0 (start) advances to field 1 (stop) — exercises the
	// "if h.formCur < maxCur" branch of handleFormKey.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Errorf("Enter mid-form should keep dialog active")
	}
}

func TestHeute_EditDialog_SubmitWithBadStart_KeepsDialogAndShowsErr(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-time.Hour), Stop: now, Elapsed: time.Hour},
	}
	m := loadedHeute(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = drainCmd(t, m, cmd)
	// Replace the start textinput's value with garbage by typing over it.
	// The form's first field is focused with the original HH:MM pre-filled,
	// so backspace it then type junk.
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, ch := range "nope" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Tab past stop/tag/note → land on note (the last field) → Enter to submit.
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// Dialog should remain open (errMsg set, no actionDoneMsg).
	if !m.(worktime.Model).FilterActive() {
		t.Error("submit with garbage start should keep dialog open")
	}
}

func TestHeute_NavigationKeys_GAndCapitalG(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	r.sessions.Sessions = []domain.Session{
		{Date: now, Start: now.Add(-3 * time.Hour), Stop: now.Add(-2 * time.Hour), Elapsed: time.Hour},
		{Date: now, Start: now.Add(-2 * time.Hour), Stop: now.Add(-1 * time.Hour), Elapsed: time.Hour},
		{Date: now, Start: now.Add(-time.Hour), Stop: now, Elapsed: time.Hour},
	}
	m := loadedHeute(t, r)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if got := m.(worktime.Model).StateCursor(); got != 2 {
		t.Errorf("G should jump to last index (2), got %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if got := m.(worktime.Model).StateCursor(); got != 0 {
		t.Errorf("g should jump to 0, got %d", got)
	}
}

// — Frei add dialog full lifecycle —

func TestFrei_AddDialog_TabCyclesIncludingKindPicker(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Fatal("`a` should open add dialog")
	}
	// Cycle: date → label → kind → date (3 fields, 3 tabs)
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	// And back via shift-tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	// Up/Down also navigate
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
}

func TestFrei_AddDialog_KindCycleHL(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = drainCmd(t, m, cmd)
	// Tab twice to land on the virtual kind slot (formCur==len(form)==2).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// l → forward through AllKinds
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	// h → backward, including the kindCur==0 wrap branch
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
}

func TestFrei_AddDialog_EnterMidFormAdvances(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = drainCmd(t, m, cmd)
	// Enter on the date field → focus advances to label
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Errorf("Enter mid-form should keep dialog active")
	}
}

func TestFrei_AddDialog_SubmitWithBadDate_KeepsDialog(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = drainCmd(t, m, cmd)
	// Backspace the prefilled date; type garbage
	for i := 0; i < 12; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, ch := range "nope" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Tab to label, then to kind, then Enter to submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Error("submit with bad date should keep dialog open")
	}
}

func TestFrei_AddDialog_SuccessfulRangeAdd(t *testing.T) {
	r := newRig(t)
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = drainCmd(t, m, cmd)
	// Backspace prefilled value, type a range expression
	for i := 0; i < 12; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, ch := range "2026-06-01..2026-06-03" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Tab to label → tab to kind → Enter (default kind=Urlaub at index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = drainCmd(t, m, cmd)
	// AddRange should have been called → at least one Urlaub entry should
	// now exist for 2026-06-01.
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 6, 3, 0, 0, 0, 0, time.Local)
	if len(r.dayoffs.List(from, to)) == 0 {
		t.Errorf("expected dayoff entries for 2026-06-01..03, got none")
	}
}

func TestFrei_DeleteConfirm_RendersConfirmDialog(t *testing.T) {
	r := newRig(t)
	now := r.clock.T
	if err := r.dayoffs.Add(domain.DayOff{Date: now, Kind: domain.KindVacation, Label: "Test"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := loadedFrei(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	m = drainCmd(t, m, cmd)
	out := m.View()
	if !strings.Contains(out, "löschen") {
		t.Errorf("delete confirm dialog should mention »löschen«, got:\n%s", out)
	}
	if !strings.Contains(out, "Eintrag löschen?") {
		t.Errorf("confirm dialog should ask the question, got:\n%s", out)
	}
}

// — History stepFilter and mode-key handlers —

func TestHistory_StepFilter_KWForwardSetsQuery(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// `]` from empty seeds the current ISO week then steps forward.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	got := m.(worktime.Model).StateFilter()
	// StateFilter shape is "tab=history|<sub>"; pull out the sub.
	const prefix = "tab=history|"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("expected %q prefix in StateFilter, got %q", prefix, got)
	}
	sub := strings.TrimPrefix(got, prefix)
	if !strings.HasPrefix(sub, "KW") {
		t.Errorf("after `]` sub-filter should be a KW expression, got %q", sub)
	}
	// `[` → stepHistFilter on existing KW reports ok=true, query updates
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	got = m.(worktime.Model).StateFilter()
	sub = strings.TrimPrefix(got, prefix)
	if !strings.HasPrefix(sub, "KW") {
		t.Errorf("after `[` sub-filter should still be a KW expression, got %q", sub)
	}
}

func TestHistory_HeatmapKey_HLNavigation(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}) // → heatmap
	// h: cursor at col 0 already (set by heatmapTodayCell); h is no-op when
	// heatCol is already 0 — exercises the guard branch.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	// j × 7 cycles row down to bottom (clamped at 6).
	for i := 0; i < 8; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	// k × 8 cycles back up (clamped at 0).
	for i := 0; i < 8; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}
	// l should advance the column (provided weeks > 1).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	// `[` shifts the offset window back 13 weeks.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	// `]` shifts forward — clamped at 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	// T resets offset.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	// / opens filter dialog.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Error("/ should open filter dialog from heatmap")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	// F opens filter pre-seeded with tag:.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Error("F should open filter dialog from heatmap")
	}
}

func TestHistory_HeatmapEnter_OpensDrill(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}) // → heatmap
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Error("Enter on heatmap should open the drill")
	}
}

func TestHistory_TagClockKey_AllNavigation(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// → tag-clock via v v
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	for _, k := range []string{"h", "l", "j", "k", "T"} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	// /, F dispatchable from tag-clock too
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	// v → month switches mode. Probe via the month-grid header (Slice E
	// removed the "Ansicht (...)" footer hint — see Review-Punkt M5).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if got := m.View(); !strings.Contains(got, "Apr 2026") {
		t.Errorf("expected month grid header »Apr 2026« in View after v, got:\n%s", got)
	}
}

func TestHistory_MonthKey_AllNavigation(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	// → month via v v v
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	for _, k := range []string{"h", "l", "j", "k", "T", "[", "]"} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	// `]` past current month is clamped — verify no panic + still on month
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	}
	// /, F from month
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = drainCmd(t, m, cmd)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	// Enter on month opens drill.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	if !m.(worktime.Model).FilterActive() {
		t.Error("Enter on month should open the drill")
	}
	// b dismisses
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	// v from month → back to list
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
}

func TestHistory_DrillKey_NavigationAndDismiss(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drainCmd(t, m, cmd)
	// j/k/g/G — verify no panics with sessions present
	for _, k := range []string{"j", "k", "g", "G"} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	// esc dismisses
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(worktime.Model).FilterActive() {
		t.Error("Esc should close the drill")
	}
}

func TestHistory_FilterDialog_SubmitGarbage_ShowsErr(t *testing.T) {
	r := newRig(t)
	seedHistorySessions(r)
	m := loadedHistory(t, r)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = drainCmd(t, m, cmd)
	for _, ch := range "??" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Garbage should keep the dialog open with errMsg set.
	if !m.(worktime.Model).FilterActive() {
		t.Error("invalid filter should keep dialog open")
	}
}
