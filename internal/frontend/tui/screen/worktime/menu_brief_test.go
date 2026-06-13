// White-box tests for the Brief flow. Verify briefBasename, the
// dispatch routing through Output, and the integration with the menu
// model (Enter on Brief → Target picker → picked target → toast).

package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// briefRig wires the minimal Reporter + Stats + DayOffStore + Reader
// chain Brief needs against in-memory fakes. Mirrors the model_test.go
// rig but only for the Brief path. FakeOutput records the dispatched
// content/target for assertion.
type briefRig struct {
	deps  Deps
	clock *testutil.FixedClock
	out   *testutil.FakeOutput
}

func newBriefRig(t *testing.T) briefRig {
	t.Helper()
	clock := &testutil.FixedClock{T: time.Date(2026, 5, 6, 10, 0, 0, 0, time.Local)}
	sessions := &testutil.FakeSessionStore{
		Sessions: []domain.Session{
			{
				Date:    time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local),
				Start:   time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local),
				Stop:    time.Date(2026, 5, 4, 17, 0, 0, 0, time.Local),
				Elapsed: 8 * time.Hour,
				Tag:     "deep",
			},
		},
	}
	active := &testutil.FakeActiveSessionStore{}
	dayoffs := testutil.NewFakeDayOffStore()
	cfg := &testutil.FakeConfigReader{}
	out := &testutil.FakeOutput{}

	targets := &usecase.TargetResolver{Config: cfg, DayOffs: dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: sessions, State: active, Targets: targets, Clock: clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: dayoffs}
	reporter := &usecase.Reporter{Reader: reader, DayOffs: dayoffs, Targets: targets, Stats: stats, Clock: clock}

	return briefRig{
		deps: Deps{
			Reader:   reader,
			Stats:    stats,
			Reporter: reporter,
			Clock:    clock,
			Output:   out,
		},
		clock: clock,
		out:   out,
	}
}

func TestBriefBasename_WeekFormat(t *testing.T) {
	ref := time.Date(2026, 5, 6, 0, 0, 0, 0, time.Local) // Mi → KW 19
	got := briefBasename(domain.ReportWeek, ref)
	if got != "worktime-brief-week-2026-W19" {
		t.Errorf("week basename = %q, want worktime-brief-week-2026-W19", got)
	}
}

func TestBriefBasename_MonthFormat(t *testing.T) {
	ref := time.Date(2026, 5, 6, 0, 0, 0, 0, time.Local)
	got := briefBasename(domain.ReportMonth, ref)
	if got != "worktime-brief-month-2026-05" {
		t.Errorf("month basename = %q, want worktime-brief-month-2026-05", got)
	}
}

func TestBriefScopeFor_KindMapping(t *testing.T) {
	if got := briefScopeFor(menuActionBriefMonth); got != domain.ReportMonth {
		t.Errorf("BriefMonth → %v, want ReportMonth", got)
	}
	if got := briefScopeFor(menuActionBriefWeek); got != domain.ReportWeek {
		t.Errorf("BriefWeek → %v, want ReportWeek", got)
	}
	// Default for any unknown kind is Week — keeps Brief degraded-gracefully.
	if got := briefScopeFor(menuActionExportCSV); got != domain.ReportWeek {
		t.Errorf("default → %v, want ReportWeek", got)
	}
}

func TestBriefCmd_RoutesContentToClipboard(t *testing.T) {
	r := newBriefRig(t)
	cmd := briefCmd(r.deps, outputTargetClipboard, domain.ReportWeek)
	msg := cmd()
	done, ok := msg.(menuActionDoneMsg)
	if !ok {
		t.Fatalf("brief cmd returned %T, want menuActionDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("brief cmd err = %v", done.err)
	}
	if !strings.Contains(done.toast, "Zwischenablage") {
		t.Errorf("toast = %q, want clipboard confirmation", done.toast)
	}
	if len(r.out.Copies) != 1 {
		t.Fatalf("Copy must be called once, got %d", len(r.out.Copies))
	}
	body := r.out.Copies[0]
	if !strings.Contains(body, "KW 19") {
		t.Errorf("brief content must mention the ISO week; got:\n%s", body)
	}
	if !strings.Contains(body, "deep") {
		t.Errorf("brief content must surface tag rollup; got:\n%s", body)
	}
}

// TestBriefCmd_SplitTargetReturnsOverlayMsg: outputTargetSplit für
// Markdown-Brief routet zum integrierten Overlay (briefViewMsg) statt
// zum externen Pager. Der Worktime-Root fängt die Message ab und
// öffnet den view.Model — kein out.Pager-Call, kein externer Prozess.
func TestBriefCmd_SplitTargetReturnsOverlayMsg(t *testing.T) {
	r := newBriefRig(t)
	cmd := briefCmd(r.deps, outputTargetSplit, domain.ReportWeek)
	msg := cmd()
	bv, ok := msg.(briefViewMsg)
	if !ok {
		t.Fatalf("split target must return briefViewMsg, got %T", msg)
	}
	if bv.body == "" {
		t.Error("briefViewMsg.body must contain rendered brief markdown")
	}
	if bv.title == "" {
		t.Error("briefViewMsg.title must be non-empty (used as overlay title)")
	}
	if len(r.out.Pagers) != 0 {
		t.Errorf("split target must not invoke Pager (in-process overlay), got %d calls", len(r.out.Pagers))
	}
}

func TestBriefCmd_SaveFileGetsTimestampedBasename(t *testing.T) {
	r := newBriefRig(t)
	cmd := briefCmd(r.deps, outputTargetFile, domain.ReportMonth)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("brief cmd err = %v", done.err)
	}
	if len(r.out.Saves) != 1 {
		t.Fatalf("SaveFile must be called once, got %d", len(r.out.Saves))
	}
	save := r.out.Saves[0]
	if save.Ext != "md" {
		t.Errorf("SaveFile ext = %q, want md", save.Ext)
	}
	if save.Basename != "worktime-brief-month-2026-05" {
		t.Errorf("SaveFile basename = %q, want worktime-brief-month-2026-05", save.Basename)
	}
	// Toast should mention Path; with FakeOutput.SaveDir defaulting to
	// "/tmp/fake/", tildePath leaves it unchanged.
	if !strings.Contains(done.toast, "gespeichert") {
		t.Errorf("toast = %q, want save confirmation", done.toast)
	}
}

func TestBriefCmd_FailsCleanlyWithoutOutputPort(t *testing.T) {
	r := newBriefRig(t)
	r.deps.Output = nil
	cmd := briefCmd(r.deps, outputTargetClipboard, domain.ReportWeek)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err == nil {
		t.Error("brief cmd without Output must surface an error")
	}
}

func TestBriefCmd_FailsCleanlyWithoutReporter(t *testing.T) {
	r := newBriefRig(t)
	r.deps.Reporter = nil
	cmd := briefCmd(r.deps, outputTargetClipboard, domain.ReportWeek)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err == nil {
		t.Error("brief cmd without Reporter must surface an error")
	}
}

// — menu integration: list → target → dispatch —

func TestMenu_BriefActionEntersTargetSubMode(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	// Find the Brief Wochenbericht entry's index in m.filtered.
	idx := -1
	for i, a := range m.filtered {
		if a.kind == menuActionBriefWeek {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("Brief Wochenbericht must be in the list")
	}
	m.cursor = idx
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeTarget {
		t.Errorf("after Enter on Brief, subMode = %v, want menuSubModeTarget", m.subMode)
	}
	if m.pending.kind != menuActionBriefWeek {
		t.Errorf("pending = %v, want menuActionBriefWeek", m.pending.kind)
	}
	out := m.View()
	if !strings.Contains(out, "Brief Wochenbericht") {
		t.Errorf("target view should show parent action label; got:\n%s", out)
	}
}

func TestMenu_TargetEscReturnsToList(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionBriefWeek {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeTarget {
		t.Fatal("precondition: must be in target sub-mode")
	}
	m, _ = m.handleKey(keyName("esc"))
	if m.subMode != menuSubModeList {
		t.Errorf("Esc in target should return to list, got subMode = %v", m.subMode)
	}
	// Use label, not kind: menuActionBriefWeek is iota=0 and collides
	// with the zero-valued menuAction{}, so kind comparison can't tell
	// "pending cleared" apart from "pending was BriefWeek".
	if m.pending.label != "" {
		t.Errorf("pending must be cleared on Esc; label = %q", m.pending.label)
	}
}

func TestMenu_TargetClipboardDispatchesAndShowsToast(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionBriefWeek {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	// 'c' picks Clipboard directly.
	m, cmd := m.handleKey(runeKey('c'))
	if cmd == nil {
		t.Fatal("hotkey c must return a dispatch tea.Cmd")
	}
	// Run the cmd to surface the actionDoneMsg, then feed it back.
	msg := cmd()
	done, ok := msg.(menuActionDoneMsg)
	if !ok {
		t.Fatalf("dispatch cmd returned %T, want menuActionDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("dispatch err = %v", done.err)
	}
	m, _ = m.Update(done)
	if m.toast == nil {
		t.Error("successful dispatch should attach a toast")
	}
	if m.subMode != menuSubModeList {
		t.Errorf("after dispatch, subMode = %v, want list", m.subMode)
	}
	if len(r.out.Copies) != 1 {
		t.Errorf("Output.Copy should have been called once; got %d", len(r.out.Copies))
	}
}

func TestMenu_ApplyActionDoneSurfacesError(t *testing.T) {
	m := newMenuModel(pal(), Deps{}).SetSize(120, 40).openMenu(tabHeute)
	m, _ = m.applyActionDone(menuActionDoneMsg{err: errFakeDispatch})
	if m.errMsg == "" {
		t.Error("applyActionDone(err) must populate errMsg")
	}
	out := m.View()
	if !strings.Contains(out, errFakeDispatch.Error()) {
		t.Errorf("View should render errMsg; got:\n%s", out)
	}
}

func TestTildePath_ShortensHomePrefixedPath(t *testing.T) {
	const home = "/Users/test"
	if got := tildePath("/Users/test/Downloads/x.md", home); got != "~/Downloads/x.md" {
		t.Errorf("tildePath = %q, want ~/Downloads/x.md", got)
	}
	if got := tildePath("/elsewhere/x.md", home); got != "/elsewhere/x.md" {
		t.Errorf("tildePath should leave non-home paths intact, got %q", got)
	}
	// Empty home (composition root passed nothing) → return path verbatim.
	if got := tildePath("/Users/test/x.md", ""); got != "/Users/test/x.md" {
		t.Errorf("tildePath with empty home should return verbatim, got %q", got)
	}
}

// errFakeDispatch is the sentinel error used by ApplyActionDone tests.
type errFakeDispatchT string

func (e errFakeDispatchT) Error() string { return string(e) }

const errFakeDispatch errFakeDispatchT = "fake dispatch failure"
