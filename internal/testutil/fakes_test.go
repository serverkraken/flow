package testutil

// Sanity tests for the testutil fakes themselves. They earn per-package
// coverage (CI's gate measures each package's own statements; without
// these tests the testutil package shows up as 0%). Each test exercises
// the happy path AND the Err-injection path so the early-return guards
// every fake carries are reached too.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

var errInjected = errors.New("injected")

// — clock.go —

func TestFakeClock_NowAndAdvance(t *testing.T) {
	c := &FixedClock{T: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)}
	if !c.Now().Equal(c.T) {
		t.Errorf("Now should mirror T")
	}
	c.Advance(2 * time.Hour)
	if c.Now().Hour() != 11 {
		t.Errorf("Advance(2h): hour should be 11, got %d", c.Now().Hour())
	}
	c.Advance(-90 * time.Minute)
	if c.Now().Minute() != 30 {
		t.Errorf("Advance(-90m): minute should be 30, got %d", c.Now().Minute())
	}
}

// — cheatsheet.go —

func TestFakeCheatsheetReader_LoadAndErr(t *testing.T) {
	r := &FakeCheatsheetReader{Content: "hello"}
	got, err := r.Load()
	if err != nil || got != "hello" {
		t.Errorf("Load() = (%q, %v), want (hello, nil)", got, err)
	}
	r.Err = errInjected
	if _, err := r.Load(); err == nil {
		t.Errorf("Load with Err should error")
	}
}

// — config.go —

func TestFakeConfigReader_LoadAndErr(t *testing.T) {
	r := &FakeConfigReader{Cfg: domain.Config{DefaultTarget: 8 * time.Hour}}
	got, err := r.Load()
	if err != nil || got.DefaultTarget != 8*time.Hour {
		t.Errorf("Load() = (%+v, %v)", got, err)
	}
	r.Err = errInjected
	if _, err := r.Load(); err == nil {
		t.Errorf("Load with Err should error")
	}
}

// — dayoffs.go —

func TestFakeDayOffStore_ConstructAndList(t *testing.T) {
	d1 := domain.DayOff{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Kind: domain.KindHoliday, Label: "May Day"}
	d2 := domain.DayOff{Date: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), Kind: domain.KindVacation, Label: "Trip"}
	s := NewFakeDayOffStore(d1, d2)
	// List with zero from/to returns all, sorted
	all := s.List(time.Time{}, time.Time{})
	if len(all) != 2 || !all[0].Date.Equal(d1.Date) {
		t.Errorf("List should return sorted entries, got %+v", all)
	}
	// Bounded list excludes out-of-range
	from := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	bounded := s.List(from, to)
	if len(bounded) != 1 || !bounded[0].Date.Equal(d2.Date) {
		t.Errorf("Bounded list should keep only d2, got %+v", bounded)
	}
}

func TestFakeDayOffStore_LookupAddRemove(t *testing.T) {
	s := NewFakeDayOffStore()
	dt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, ok := s.Lookup(dt); ok {
		t.Errorf("empty store should not find entry")
	}
	if err := s.Add(domain.DayOff{Date: dt, Kind: domain.KindHoliday}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, ok := s.Lookup(dt); !ok {
		t.Errorf("Lookup after Add should find entry")
	}
	if err := s.Remove(dt); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if _, ok := s.Lookup(dt); ok {
		t.Errorf("Lookup after Remove should not find entry")
	}
}

func TestFakeDayOffStore_AddNilMapInitialised(t *testing.T) {
	s := &FakeDayOffStore{} // Entries map is nil
	if err := s.Add(domain.DayOff{Date: time.Now(), Kind: domain.KindHoliday}); err != nil {
		t.Errorf("Add on nil-map store: %v", err)
	}
}

func TestFakeDayOffStore_AddBatchHappyEmptyAndErr(t *testing.T) {
	s := &FakeDayOffStore{}
	if err := s.AddBatch(nil); err != nil {
		t.Errorf("AddBatch(nil): %v", err)
	}
	batch := []domain.DayOff{
		{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Kind: domain.KindHoliday},
		{Date: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), Kind: domain.KindHoliday},
	}
	if err := s.AddBatch(batch); err != nil {
		t.Errorf("AddBatch: %v", err)
	}
	if len(s.Entries) != 2 {
		t.Errorf("AddBatch should land both entries, got %d", len(s.Entries))
	}
	s.Err = errInjected
	if err := s.AddBatch(batch); err == nil {
		t.Errorf("AddBatch with Err should error")
	}
}

func TestFakeDayOffStore_ErrPaths(t *testing.T) {
	s := NewFakeDayOffStore()
	s.Err = errInjected
	if err := s.Add(domain.DayOff{Date: time.Now(), Kind: domain.KindHoliday}); err == nil {
		t.Errorf("Add with Err should error")
	}
	if err := s.Remove(time.Now()); err == nil {
		t.Errorf("Remove with Err should error")
	}
}

// — flowstate.go —

func TestFakeFlowStateStore_LoadSaveNext(t *testing.T) {
	s := &FakeFlowStateStore{}
	got, err := s.Load()
	if err != nil || got.Screen != "" {
		t.Errorf("Load empty: got (%+v, %v)", got, err)
	}
	if err := s.Save(domain.FlowState{Screen: "palette", Cursor: 3}); err != nil {
		t.Errorf("Save: %v", err)
	}
	if got, _ := s.Load(); got.Screen != "palette" || got.Cursor != 3 {
		t.Errorf("Load after Save: got %+v", got)
	}
	if err := s.WriteNextScreen("palette"); err != nil {
		t.Errorf("WriteNextScreen: %v", err)
	}
	if got, _ := s.ConsumeNextScreen(); got != "palette" {
		t.Errorf("ConsumeNextScreen first call: %q", got)
	}
	if got, _ := s.ConsumeNextScreen(); got != "" {
		t.Errorf("ConsumeNextScreen second call should be cleared: %q", got)
	}
}

func TestFakeFlowStateStore_LoadAndSaveErrors(t *testing.T) {
	s := &FakeFlowStateStore{LoadErr: errInjected}
	if _, err := s.Load(); err == nil {
		t.Errorf("Load with LoadErr should fail")
	}
	s2 := &FakeFlowStateStore{SaveErr: errInjected}
	if err := s2.Save(domain.FlowState{}); err == nil {
		t.Errorf("Save with SaveErr should fail")
	}
}

// — kompendium.go —

func TestFakeNoteLauncher_OpenAndErr(t *testing.T) {
	l := &FakeNoteLauncher{}
	if err := l.Open("abc"); err != nil {
		t.Errorf("Open: %v", err)
	}
	if len(l.Calls) != 1 || l.Calls[0] != "open:abc" {
		t.Errorf("Calls = %+v", l.Calls)
	}
	l.Err = errInjected
	if err := l.Open("def"); err == nil {
		t.Errorf("Open with Err should fail")
	}
	// Calls is still appended even when Err is set — fake records intent.
	if len(l.Calls) != 2 {
		t.Errorf("Calls should be appended even on error, got %d", len(l.Calls))
	}
}

// — links.go —

func TestFakeLinkStore_AddListCountRemove(t *testing.T) {
	s := &FakeLinkStore{}
	dt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := s.Add(dt, "id1"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Idempotent — adding the same id again does not duplicate.
	if err := s.Add(dt, "id1"); err != nil {
		t.Fatalf("Add idempotent: %v", err)
	}
	if err := s.Add(dt, "id2"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ids, err := s.ListByDate(dt)
	if err != nil || len(ids) != 2 {
		t.Errorf("ListByDate: (%+v, %v)", ids, err)
	}
	counts, err := s.CountsByDate()
	if err != nil || counts[dt.Format("2006-01-02")] != 2 {
		t.Errorf("CountsByDate: (%+v, %v)", counts, err)
	}
	if err := s.Remove(dt, "id1"); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if ids, _ := s.ListByDate(dt); len(ids) != 1 || ids[0] != "id2" {
		t.Errorf("After remove ids = %+v", ids)
	}
	// Removing missing is no-op
	if err := s.Remove(dt, "missing"); err != nil {
		t.Errorf("Remove missing should not error: %v", err)
	}
}

func TestFakeLinkStore_ErrPaths(t *testing.T) {
	s := &FakeLinkStore{Err: errInjected}
	if _, err := s.ListByDate(time.Now()); err == nil {
		t.Errorf("ListByDate with Err should fail")
	}
	if err := s.Add(time.Now(), "x"); err == nil {
		t.Errorf("Add with Err should fail")
	}
	if _, err := s.CountsByDate(); err == nil {
		t.Errorf("CountsByDate with Err should fail")
	}
	if err := s.Remove(time.Now(), "x"); err == nil {
		t.Errorf("Remove with Err should fail")
	}
}

// — markdown.go —

func TestFakeMarkdownRenderer(t *testing.T) {
	r := &FakeMarkdownRenderer{Prefix: ">> "}
	out, err := r.Render("body", 80)
	if err != nil || out != ">> body" || r.LastWidth != 80 {
		t.Errorf("Render = (%q, %v), width=%d", out, err, r.LastWidth)
	}
	r.Err = errInjected
	if _, err := r.Render("x", 1); err == nil {
		t.Errorf("Render with Err should fail")
	}
}

// — output.go —

func TestFakeOutput_CopyPagerSave(t *testing.T) {
	o := &FakeOutput{}
	if err := o.Copy("hi"); err != nil {
		t.Errorf("Copy: %v", err)
	}
	if len(o.Copies) != 1 || o.Copies[0] != "hi" {
		t.Errorf("Copies: %+v", o.Copies)
	}
	if err := o.Pager("body", "less", "md"); err != nil {
		t.Errorf("Pager: %v", err)
	}
	if len(o.Pagers) != 1 || o.Pagers[0].Viewer != "less" {
		t.Errorf("Pagers: %+v", o.Pagers)
	}
	path, err := o.SaveFile("name", "md", []byte("data"))
	if err != nil || !strings.HasSuffix(path, "name.md") {
		t.Errorf("SaveFile = (%q, %v)", path, err)
	}
	// Custom SaveDir (without trailing slash) gets one appended.
	o.SaveDir = "/var/out"
	path2, _ := o.SaveFile("a", "csv", []byte{})
	if path2 != "/var/out/a.csv" {
		t.Errorf("SaveFile with custom dir: %q", path2)
	}
	// And one with trailing slash
	o.SaveDir = "/var/out/"
	path3, _ := o.SaveFile("a", "csv", []byte{})
	if path3 != "/var/out/a.csv" {
		t.Errorf("SaveFile with trailing slash: %q", path3)
	}
}

func TestFakeOutput_ErrPaths(t *testing.T) {
	o := &FakeOutput{CopyErr: errInjected, PagerErr: errInjected, SaveErr: errInjected}
	if err := o.Copy("x"); err == nil {
		t.Errorf("Copy with err should fail")
	}
	if err := o.Pager("x", "less", "md"); err == nil {
		t.Errorf("Pager with err should fail")
	}
	if _, err := o.SaveFile("a", "b", nil); err == nil {
		t.Errorf("SaveFile with err should fail")
	}
}

// — palette.go —

func TestFakePaletteEntryReader(t *testing.T) {
	r := &FakePaletteEntryReader{Entries: []domain.PaletteEntry{{Label: "A"}, {Label: "B"}}}
	got, err := r.List()
	if err != nil || len(got) != 2 {
		t.Errorf("List = (%+v, %v)", got, err)
	}
	r.Err = errInjected
	if _, err := r.List(); err == nil {
		t.Errorf("List with Err should fail")
	}
}

func TestFakePaletteStatsStore(t *testing.T) {
	s := &FakePaletteStatsStore{}
	got, err := s.Load()
	if err != nil || got.Actions == nil {
		t.Errorf("Load empty: (%+v, %v)", got, err)
	}
	st := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{"a": {Count: 2}}}
	if err := s.Save(st); err != nil {
		t.Errorf("Save: %v", err)
	}
	got2, _ := s.Load()
	if got2.Actions["a"].Count != 2 {
		t.Errorf("Load after Save: %+v", got2)
	}
	s.LoadErr = errInjected
	if _, err := s.Load(); err == nil {
		t.Errorf("Load with LoadErr should fail")
	}
	s.LoadErr = nil
	s.SaveErr = errInjected
	if err := s.Save(st); err == nil {
		t.Errorf("Save with SaveErr should fail")
	}
}

// — projects.go —

func TestFakeProjectScanner(t *testing.T) {
	// Names path
	s := &FakeProjectScanner{Names: []string{"a", "b"}}
	got, err := s.List()
	if err != nil || len(got) != 2 || got[0].Path != "/tmp/a" {
		t.Errorf("List Names path = (%+v, %v)", got, err)
	}
	// Projects path
	s2 := &FakeProjectScanner{Projects: []domain.Project{{Name: "x", Path: "/srv/x"}}}
	got2, _ := s2.List()
	if len(got2) != 1 || got2[0].Path != "/srv/x" {
		t.Errorf("List Projects path = %+v", got2)
	}
	// Err path
	s.Err = errInjected
	if _, err := s.List(); err == nil {
		t.Errorf("List with Err should fail")
	}
}

// — sessions.go —

func TestFakeSessionStore_LoadAndFilter(t *testing.T) {
	s := &FakeSessionStore{Sessions: []domain.Session{
		{Tag: "a"}, {Tag: "b"}, {Tag: "a"},
	}}
	all, err := s.LoadAll()
	if err != nil || len(all) != 3 {
		t.Errorf("LoadAll = (%+v, %v)", all, err)
	}
	got, _ := s.LoadFiltered(func(x domain.Session) bool { return x.Tag == "a" })
	if len(got) != 2 {
		t.Errorf("LoadFiltered tag=a should return 2, got %d", len(got))
	}
}

func TestFakeSessionStore_AppendRewriteAndErr(t *testing.T) {
	s := &FakeSessionStore{}
	if err := s.Append(domain.Session{Tag: "x"}); err != nil {
		t.Errorf("Append: %v", err)
	}
	if err := s.AppendBatch([]domain.Session{{Tag: "y"}, {Tag: "z"}}); err != nil {
		t.Errorf("AppendBatch: %v", err)
	}
	if len(s.Sessions) != 3 {
		t.Errorf("Sessions after appends: %d", len(s.Sessions))
	}
	if err := s.Rewrite([]domain.Session{{Tag: "only"}}); err != nil {
		t.Errorf("Rewrite: %v", err)
	}
	if len(s.Sessions) != 1 || s.Sessions[0].Tag != "only" {
		t.Errorf("After Rewrite: %+v", s.Sessions)
	}
	s.Err = errInjected
	if _, err := s.LoadAll(); err == nil {
		t.Errorf("LoadAll with Err should fail")
	}
	if _, err := s.LoadFiltered(func(domain.Session) bool { return true }); err == nil {
		t.Errorf("LoadFiltered with Err should fail")
	}
	if err := s.Append(domain.Session{}); err == nil {
		t.Errorf("Append with Err should fail")
	}
	if err := s.AppendBatch(nil); err == nil {
		t.Errorf("AppendBatch with Err should fail")
	}
	if err := s.Rewrite(nil); err == nil {
		t.Errorf("Rewrite with Err should fail")
	}
}

// — state.go —

func TestFakeActiveSessionStore(t *testing.T) {
	s := &FakeActiveSessionStore{}
	if got, _ := s.GetActive(); got != nil {
		t.Errorf("empty GetActive should be nil, got %v", got)
	}
	now := time.Now()
	if err := s.SetActive(now); err != nil {
		t.Errorf("SetActive: %v", err)
	}
	if got, _ := s.GetActive(); got == nil || !got.Equal(now) {
		t.Errorf("GetActive after SetActive: %v", got)
	}
	if err := s.ClearActive(); err != nil {
		t.Errorf("ClearActive: %v", err)
	}
	if got, _ := s.GetActive(); got != nil {
		t.Errorf("after ClearActive: %v", got)
	}
	// Pause analogue
	if got, _ := s.GetPause(); got != nil {
		t.Errorf("empty GetPause should be nil")
	}
	if err := s.SetPause(now); err != nil {
		t.Errorf("SetPause: %v", err)
	}
	if got, _ := s.GetPause(); got == nil || !got.Equal(now) {
		t.Errorf("GetPause after SetPause: %v", got)
	}
	if err := s.ClearPause(); err != nil {
		t.Errorf("ClearPause: %v", err)
	}
	if got, _ := s.GetPause(); got != nil {
		t.Errorf("after ClearPause: %v", got)
	}
	// Err paths
	s.Err = errInjected
	if _, err := s.GetActive(); err == nil {
		t.Errorf("GetActive with Err should fail")
	}
	if err := s.SetActive(now); err == nil {
		t.Errorf("SetActive with Err should fail")
	}
	if err := s.ClearActive(); err == nil {
		t.Errorf("ClearActive with Err should fail")
	}
	if _, err := s.GetPause(); err == nil {
		t.Errorf("GetPause with Err should fail")
	}
	if err := s.SetPause(now); err == nil {
		t.Errorf("SetPause with Err should fail")
	}
	if err := s.ClearPause(); err == nil {
		t.Errorf("ClearPause with Err should fail")
	}
}

func TestFakeLock(t *testing.T) {
	l := &FakeLock{}
	called := 0
	if err := l.With(func() error { called++; return nil }); err != nil {
		t.Errorf("With: %v", err)
	}
	if called != 1 || l.Calls != 1 {
		t.Errorf("called=%d Calls=%d", called, l.Calls)
	}
	// fn error propagates
	if err := l.With(func() error { return errInjected }); err == nil {
		t.Errorf("fn error should propagate")
	}
	// Lock error short-circuits fn
	l.Err = errInjected
	called = 0
	if err := l.With(func() error { called++; return nil }); err == nil {
		t.Errorf("Lock Err should fail")
	}
	if called != 0 {
		t.Errorf("fn should NOT be invoked when Lock Err set")
	}
}

// — tmux.go —

func TestFakeTmux_AllMethods(t *testing.T) {
	tx := &FakeTmux{
		Session:  "main",
		Sessions: []string{"main", "scratch"},
		Options:  map[string]string{"@theme": "storm"},
	}
	if err := tx.RefreshClient(); err != nil {
		t.Errorf("RefreshClient: %v", err)
	}
	if tx.Refreshes != 1 {
		t.Errorf("Refreshes=%d, want 1", tx.Refreshes)
	}
	if got := tx.ShowOption("@theme"); got != "storm" {
		t.Errorf("ShowOption: %q", got)
	}
	if got := tx.ShowOption("missing"); got != "" {
		t.Errorf("ShowOption missing should be empty, got %q", got)
	}
	// Options==nil branch
	tx2 := &FakeTmux{}
	if got := tx2.ShowOption("anything"); got != "" {
		t.Errorf("ShowOption with nil Options: %q", got)
	}
	if tx.CurrentSessionName() != "main" {
		t.Errorf("CurrentSessionName: %q", tx.CurrentSessionName())
	}
	list, err := tx.ListSessions()
	if err != nil || len(list) != 2 {
		t.Errorf("ListSessions = (%+v, %v)", list, err)
	}
	if !tx.HasSession("scratch") {
		t.Errorf("HasSession scratch should be true")
	}
	if tx.HasSession("nope") {
		t.Errorf("HasSession nope should be false")
	}
	if err := tx.NewSessionAt("new", "/tmp"); err != nil {
		t.Errorf("NewSessionAt: %v", err)
	}
	if len(tx.New) != 1 || tx.New[0] != "new@/tmp" {
		t.Errorf("New=%+v", tx.New)
	}
	if !tx.HasSession("new") {
		t.Errorf("New session should be visible to HasSession")
	}
	if err := tx.SwitchClient("scratch"); err != nil {
		t.Errorf("SwitchClient: %v", err)
	}
	if len(tx.Switches) != 1 || tx.Switches[0] != "scratch" {
		t.Errorf("Switches=%+v", tx.Switches)
	}
	if err := tx.SplitWindowH("vim", "-c", "echo"); err != nil {
		t.Errorf("SplitWindowH: %v", err)
	}
	if len(tx.Splits) != 1 || tx.Splits[0] != "vim -c echo" {
		t.Errorf("Splits=%+v", tx.Splits)
	}
	if err := tx.RunTmuxAction("split-window"); err != nil {
		t.Errorf("RunTmuxAction: %v", err)
	}
	if len(tx.Actions) != 1 {
		t.Errorf("Actions=%+v", tx.Actions)
	}
}

func TestFakeTmux_ErrPaths(t *testing.T) {
	tx := &FakeTmux{
		RefreshErr:      errInjected,
		SplitErr:        errInjected,
		ActionErr:       errInjected,
		ListSessionsErr: errInjected,
		NewSessionErr:   errInjected,
		SwitchErr:       errInjected,
	}
	if err := tx.RefreshClient(); err == nil {
		t.Errorf("RefreshClient with err should fail")
	}
	if _, err := tx.ListSessions(); err == nil {
		t.Errorf("ListSessions with err should fail")
	}
	if err := tx.NewSessionAt("a", "/"); err == nil {
		t.Errorf("NewSessionAt with err should fail")
	}
	if err := tx.SwitchClient("a"); err == nil {
		t.Errorf("SwitchClient with err should fail")
	}
	if err := tx.SplitWindowH("cmd"); err == nil {
		t.Errorf("SplitWindowH with err should fail")
	}
	if err := tx.RunTmuxAction("x"); err == nil {
		t.Errorf("RunTmuxAction with err should fail")
	}
}

// — wikilink.go —

func TestFakeWikilinkResolver(t *testing.T) {
	// Nil-entries fast path
	r := &FakeWikilinkResolver{}
	if _, _, ok := r.Resolve("anything"); ok {
		t.Errorf("nil-entries resolver should not match")
	}
	r2 := &FakeWikilinkResolver{Entries: map[string]FakeWikilinkEntry{
		"id1": {URI: "kompendium://note/id1", Title: "Note 1"},
	}}
	uri, title, ok := r2.Resolve("id1")
	if !ok || uri != "kompendium://note/id1" || title != "Note 1" {
		t.Errorf("Resolve id1: (%q, %q, %v)", uri, title, ok)
	}
	if _, _, ok := r2.Resolve("missing"); ok {
		t.Errorf("missing id should not resolve")
	}
}
