package cli_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/cli"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// fixedNow pinnt die Zeit auf Donnerstag, 30. April 2026, 14:30 Local.
// Die laufende Woche reicht von Mo 27.4. bis So 3.5.; darin liegt
// Fr 1.5. (Tag der Arbeit, bundesweiter Feiertag).
var fixedNow = time.Date(2026, 4, 30, 14, 30, 0, 0, time.Local)

var update = flag.Bool("update", false, "update golden files in testdata/")

type fixture struct {
	sessions *testutil.FakeSessionStore
	active   *testutil.FakeActiveSessionStore
	lock     *testutil.FakeLock
	dayoffs  *testutil.FakeDayOffStore
	config   *testutil.FakeConfigReader
	tmux     *testutil.FakeTmux
	clock    *testutil.FixedClock
}

func newFixture() *fixture {
	return &fixture{
		sessions: &testutil.FakeSessionStore{},
		active:   &testutil.FakeActiveSessionStore{},
		lock:     &testutil.FakeLock{},
		dayoffs:  testutil.NewFakeDayOffStore(),
		config:   &testutil.FakeConfigReader{},
		tmux:     &testutil.FakeTmux{},
		clock:    &testutil.FixedClock{T: fixedNow},
	}
}

func (f *fixture) deps() cli.WorktimeDeps {
	targets := &usecase.TargetResolver{Config: f.config, DayOffs: f.dayoffs, DefaultTarget: 8 * time.Hour}
	reader := &usecase.WorktimeReader{Sessions: f.sessions, State: f.active, Targets: targets, Clock: f.clock}
	stats := &usecase.StatsComputer{Reader: reader, Targets: targets, DayOffs: f.dayoffs}
	return cli.WorktimeDeps{
		Clock: f.clock,
		Tmux:  f.tmux,
		SessionWriter: &usecase.SessionWriter{
			Sessions: f.sessions, State: f.active, Lock: f.lock, Reader: reader, Clock: f.clock,
		},
		StatusComposer: &usecase.StatusComposer{
			Reader: reader, DayOffs: f.dayoffs, Targets: targets, Stats: stats, Tmux: f.tmux, Clock: f.clock,
		},
		Reporter: &usecase.Reporter{
			Reader: reader, DayOffs: f.dayoffs, Targets: targets, Stats: stats, Clock: f.clock,
		},
		Stats:        stats,
		DayOffWriter: &usecase.DayOffWriter{Store: f.dayoffs},
		DayOffStore:  f.dayoffs,
		Reader:       reader,
	}
}

func (f *fixture) run(args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(f.deps())
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// goldenAssert compares got to testdata/<name>.golden, or rewrites it
// when -update is passed (`go test ./internal/frontend/cli -update`).
func goldenAssert(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if got != string(want) {
		t.Errorf("golden %s mismatch:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

// session is a small constructor to keep test-data declarations terse.
func session(date time.Time, startH, startM, stopH, stopM int, tag, note string) domain.Session {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	start := time.Date(date.Year(), date.Month(), date.Day(), startH, startM, 0, 0, time.Local)
	stop := time.Date(date.Year(), date.Month(), date.Day(), stopH, stopM, 0, 0, time.Local)
	// Stable deterministic ID — tests just need uniqueness within the seed
	// after Plan-B follow-up #1 made ports.SessionStore strictly ID-based.
	id := fmt.Sprintf("cli-%s-%02d%02d", d.Format("20060102"), startH, startM)
	return domain.Session{ID: id, Date: d, Start: start, Stop: stop, Elapsed: stop.Sub(start), Tag: tag, Note: note}
}

// TestHelp captures the cobra-generated --help output for the worktime
// command tree. Acts as a regression net for accidental flag/short-help
// edits.
func TestHelp(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"help_worktime", []string{"--help"}},
		{"help_status", []string{"status", "--help"}},
		{"help_start", []string{"start", "--help"}},
		{"help_pause", []string{"pause", "--help"}},
		{"help_resume", []string{"resume", "--help"}},
		{"help_brief", []string{"brief", "--help"}},
		{"help_stop", []string{"stop", "--help"}},
		{"help_toggle", []string{"toggle", "--help"}},
		{"help_correct", []string{"correct", "--help"}},
		{"help_export", []string{"export", "--help"}},
		{"help_stats", []string{"stats", "--help"}},
		{"help_tag", []string{"tag", "--help"}},
		{"help_note", []string{"note", "--help"}},
		{"help_dayoff", []string{"dayoff", "--help"}},
		{"help_dayoff_add", []string{"dayoff", "add", "--help"}},
		{"help_dayoff_remove", []string{"dayoff", "remove", "--help"}},
		{"help_dayoff_list", []string{"dayoff", "list", "--help"}},
		{"help_dayoff_sync", []string{"dayoff", "sync", "--help"}},
		{"help_dayoff_export", []string{"dayoff", "export", "--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			stdout, _, err := f.run(tc.args...)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			goldenAssert(t, tc.name, stdout)
		})
	}
}

func TestStatus(t *testing.T) {
	f := newFixture()
	must(t, f.dayoffs.Add(domain.DayOff{
		Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}))
	stdout, _, err := f.run("status")
	if err != nil {
		t.Fatal(err)
	}
	goldenAssert(t, "status", stdout)
	if f.tmux.Refreshes != 0 {
		t.Errorf("status must not refresh tmux, got %d", f.tmux.Refreshes)
	}
}

func TestStart_Idle(t *testing.T) {
	f := newFixture()
	_, stderr, err := f.run("start", "10:00")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "Worktime läuft seit 10:00\n"; stderr != want {
		t.Errorf("stderr: got %q want %q", stderr, want)
	}
	if f.active.Active == nil {
		t.Fatal("active session not set")
	}
	if got, want := f.active.Active.Format("15:04"), "10:00"; got != want {
		t.Errorf("active start: got %s want %s", got, want)
	}
	if f.tmux.Refreshes != 1 {
		t.Errorf("tmux refresh count: got %d want 1", f.tmux.Refreshes)
	}
}

func TestStart_AlreadyRunning_NoForce(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-2 * time.Hour)
	f.active.Active = &t1
	_, stderr, err := f.run("start")
	if err != nil {
		// Idempotent: already-running is a no-op exit-0 with a hint on
		// stderr instead of a hard error, so tmux bindings can press
		// 'start' blindly without surfacing red noise.
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if !strings.Contains(stderr, "läuft bereits") {
		t.Errorf("stderr should hint that a session is running, got %q", stderr)
	}
	if f.tmux.Refreshes != 0 {
		t.Errorf("idempotent no-op must not refresh, got %d", f.tmux.Refreshes)
	}
}

func TestStart_Force_Overwrites(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-2 * time.Hour)
	f.active.Active = &t1
	_, stderr, err := f.run("start", "--force", "13:00")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "13:00") {
		t.Errorf("stderr: %q", stderr)
	}
	if got := f.active.Active.Format("15:04"); got != "13:00" {
		t.Errorf("active start: got %s want 13:00", got)
	}
}

func TestPause_Active(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-90 * time.Minute)
	f.active.Active = &t1
	_, stderr, err := f.run("pause")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "Pausiert nach 1h 30m") {
		t.Errorf("stderr: %q", stderr)
	}
	if f.active.Pause == nil {
		t.Error("pause marker not set")
	}
	if f.tmux.Refreshes != 1 {
		t.Errorf("expected 1 refresh, got %d", f.tmux.Refreshes)
	}
}

func TestResume_Paused(t *testing.T) {
	f := newFixture()
	pauseAt := fixedNow.Add(-15 * time.Minute)
	f.active.Pause = &pauseAt
	_, stderr, err := f.run("resume")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "Resume") {
		t.Errorf("stderr: %q", stderr)
	}
	if f.active.Pause != nil {
		t.Error("pause marker should be cleared")
	}
	if f.active.Active == nil {
		t.Error("active should be set")
	}
}

func TestStop_Running(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-2 * time.Hour)
	f.active.Active = &t1
	_, stderr, err := f.run("stop")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "Gestoppt nach 2h 00m") {
		t.Errorf("stderr: %q", stderr)
	}
	if f.active.Active != nil {
		t.Error("active should be cleared")
	}
	if len(f.sessions.Sessions) != 1 {
		t.Errorf("expected 1 logged session, got %d", len(f.sessions.Sessions))
	}
}

func TestStop_AtTime(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-3 * time.Hour)
	f.active.Active = &t1
	_, stderr, err := f.run("stop", "13:00")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "Gestoppt") {
		t.Errorf("stderr: %q", stderr)
	}
	if len(f.sessions.Sessions) != 1 {
		t.Fatalf("expected 1 session logged, got %d", len(f.sessions.Sessions))
	}
	if got := f.sessions.Sessions[0].Stop.Format("15:04"); got != "13:00" {
		t.Errorf("stop time: got %s want 13:00", got)
	}
}

func TestStop_Idle_Idempotent(t *testing.T) {
	f := newFixture()
	_, stderr, err := f.run("stop")
	if err != nil {
		t.Fatalf("idle stop must succeed: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr should be silent, got %q", stderr)
	}
}

func TestToggle_Idle_Starts(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("toggle")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if f.active.Active == nil {
		t.Error("toggle from idle should start")
	}
}

func TestToggle_Running_Stops(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-30 * time.Minute)
	f.active.Active = &t1
	_, _, err := f.run("toggle")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if f.active.Active != nil {
		t.Error("toggle from running should stop")
	}
	if len(f.sessions.Sessions) != 1 {
		t.Errorf("expected 1 logged session, got %d", len(f.sessions.Sessions))
	}
}

func TestCorrect(t *testing.T) {
	f := newFixture()
	t1 := fixedNow.Add(-2 * time.Hour)
	f.active.Active = &t1
	_, stderr, err := f.run("correct", "11:30")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "11:30") {
		t.Errorf("stderr: %q", stderr)
	}
	if got := f.active.Active.Format("15:04"); got != "11:30" {
		t.Errorf("active start: got %s want 11:30", got)
	}
}

func TestBrief(t *testing.T) {
	f := newFixture()
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	tue := mon.AddDate(0, 0, 1)
	wed := mon.AddDate(0, 0, 2)
	must(t, f.sessions.Upsert(session(mon, 9, 0, 12, 30, "build", "")))
	must(t, f.sessions.Upsert(session(tue, 8, 30, 17, 0, "review", "PRs gemerged")))
	must(t, f.sessions.Upsert(session(wed, 9, 15, 12, 0, "build", "")))
	stdout, _, err := f.run("brief")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "brief_week", stdout)
}

func TestBrief_MonthScope(t *testing.T) {
	f := newFixture()
	stdout, _, err := f.run("brief", "month")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Apr 2026") {
		t.Errorf("month brief should mention Apr 2026, got:\n%s", stdout)
	}
}

func TestBrief_DateArg(t *testing.T) {
	f := newFixture()
	stdout, _, err := f.run("brief", "2026-04-15")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "13.04. – 19.04.2026") {
		t.Errorf("date-arg brief should pick 13.-19. April week, got:\n%s", stdout)
	}
}

func TestBrief_UnknownScope(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("brief", "garbage")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExport_CSV(t *testing.T) {
	f := newFixture()
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	must(t, f.sessions.Upsert(session(mon, 9, 0, 11, 0, "build", "")))
	must(t, f.sessions.Upsert(session(mon.AddDate(0, 0, 1), 14, 0, 16, 30, "review", "")))
	stdout, _, err := f.run("export", "week")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "export_csv_week", stdout)
}

func TestExport_JSON(t *testing.T) {
	f := newFixture()
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	must(t, f.sessions.Upsert(session(mon, 9, 0, 11, 0, "build", "")))
	stdout, _, err := f.run("export", "--format", "json", "week")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "export_json_week", stdout)
}

func TestExport_UnknownFormat(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("export", "--format", "xml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("error: %v", err)
	}
}

func TestStats_Empty(t *testing.T) {
	f := newFixture()
	stdout, _, err := f.run("stats", "today")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Total:    0h 00m") {
		t.Errorf("empty stats should report zero total, got:\n%s", stdout)
	}
}

func TestStats_Text(t *testing.T) {
	f := newFixture()
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	must(t, f.sessions.Upsert(session(mon, 9, 0, 17, 0, "build", "")))
	must(t, f.sessions.Upsert(session(mon.AddDate(0, 0, 1), 9, 0, 17, 30, "build", "")))
	stdout, _, err := f.run("stats", "week")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "stats_text_week", stdout)
}

func TestStats_JSON(t *testing.T) {
	f := newFixture()
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	must(t, f.sessions.Upsert(session(mon, 9, 0, 17, 0, "build", "")))
	stdout, _, err := f.run("stats", "--format", "json", "week")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "stats_json_week", stdout)
}

func TestTag_SetsTag(t *testing.T) {
	f := newFixture()
	today := fixedNow
	must(t, f.sessions.Upsert(session(today, 9, 0, 11, 0, "", "")))
	_, _, err := f.run("tag", "1", "build")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := f.sessions.Sessions[0].Tag; got != "build" {
		t.Errorf("tag: got %q want %q", got, "build")
	}
}

func TestTag_InvalidIdx(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("tag", "abc", "build")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNote_SetsNote(t *testing.T) {
	f := newFixture()
	today := fixedNow
	must(t, f.sessions.Upsert(session(today, 9, 0, 11, 0, "build", "")))
	_, _, err := f.run("note", "1", "morning sprint")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := f.sessions.Sessions[0].Note; got != "morning sprint" {
		t.Errorf("note: got %q", got)
	}
}

func TestDayOff_AddSingle(t *testing.T) {
	f := newFixture()
	_, stderr, err := f.run("dayoff", "add", "2026-05-29", "vacation", "Brückentag")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "2026-05-29") {
		t.Errorf("stderr: %q", stderr)
	}
	if _, ok := f.dayoffs.Lookup(time.Date(2026, 5, 29, 0, 0, 0, 0, time.Local)); !ok {
		t.Error("entry not stored")
	}
}

func TestDayOff_AddRange(t *testing.T) {
	f := newFixture()
	_, stderr, err := f.run("dayoff", "add", "2026-07-13..2026-07-17", "vacation", "Sommerurlaub")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "5 Tag(e)") {
		t.Errorf("stderr: %q", stderr)
	}
	if len(f.dayoffs.Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(f.dayoffs.Entries))
	}
}

func TestDayOff_AddInvalidKind(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("dayoff", "add", "2026-05-29", "bogus")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDayOff_Remove_InvalidDate(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("dayoff", "remove", "garbage")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDayOff_Add_InvalidDate(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("dayoff", "add", "garbage", "vacation")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDayOff_Add_InvalidRange(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("dayoff", "add", "garbage..2026-05-01", "vacation")
	if err == nil {
		t.Fatal("expected error")
	}
	_, _, err = f.run("dayoff", "add", "2026-05-01..garbage", "vacation")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDayOff_Export_UnknownFormat(t *testing.T) {
	f := newFixture()
	_, _, err := f.run("dayoff", "export", "--format", "xml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDayOff_Remove(t *testing.T) {
	f := newFixture()
	d := time.Date(2026, 5, 29, 0, 0, 0, 0, time.Local)
	must(t, f.dayoffs.Add(domain.DayOff{Date: d, Kind: domain.KindVacation, Label: "x"}))
	_, _, err := f.run("dayoff", "remove", "2026-05-29")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := f.dayoffs.Lookup(d); ok {
		t.Error("entry not removed")
	}
}

func TestDayOff_List_Empty(t *testing.T) {
	f := newFixture()
	stdout, stderr, err := f.run("dayoff", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Empty result is a silent empty stdout (Unix shape) so the
	// caller can `dayoff list --year 2099 | wc -l` without parsing
	// stderr noise out of an empty success path.
	if stdout != "" {
		t.Errorf("stdout should be empty, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr should be empty for a clean empty read, got %q", stderr)
	}
}

func TestDayOff_List(t *testing.T) {
	f := newFixture()
	must(t, f.dayoffs.Add(domain.DayOff{
		Date: time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Karfreitag",
	}))
	must(t, f.dayoffs.Add(domain.DayOff{
		Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}))
	stdout, _, err := f.run("dayoff", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "dayoff_list", stdout)
}

func TestDayOff_Sync_NW(t *testing.T) {
	f := newFixture()
	_, stderr, err := f.run("dayoff", "sync", "--year", "2026", "--land", "NW")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr, "Feiertag(e) hinzugefügt") {
		t.Errorf("stderr: %q", stderr)
	}
	if len(f.dayoffs.Entries) == 0 {
		t.Error("no holidays added")
	}
}

func TestDayOff_Export_TSV(t *testing.T) {
	f := newFixture()
	must(t, f.dayoffs.Add(domain.DayOff{
		Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}))
	stdout, _, err := f.run("dayoff", "export", "--year", "2026", "--format", "tsv")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	goldenAssert(t, "dayoff_export_tsv", stdout)
}

func TestDayOff_Export_ICS(t *testing.T) {
	f := newFixture()
	must(t, f.dayoffs.Add(domain.DayOff{
		Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit",
	}))
	stdout, _, err := f.run("dayoff", "export", "--year", "2026", "--format", "ics")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// ICS output is deterministic except for the DTSTAMP line, which
	// reaches into time.Now(). Strip it before the golden compare.
	stripped := stripDTSTAMP(stdout)
	goldenAssert(t, "dayoff_export_ics", stripped)
}

func stripDTSTAMP(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "DTSTAMP:") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Server-mode pause/resume tests (C1 fix)
// ---------------------------------------------------------------------------

// serverDeps builds a minimal WorktimeDeps wired for server mode:
// ListActiveSessions, PauseActiveSession, ResumeActiveSession are populated;
// PauseMarker is nil (server mode), SessionWriter is nil (server mode).
func serverDeps(
	list func(userID string) ([]domain.ActiveSession, error),
	pause func(userID, projectID string) (domain.ActiveSession, error),
	resume func(userID, projectID string) (domain.ActiveSession, error),
) cli.WorktimeDeps {
	clock := &testutil.FixedClock{T: fixedNow}
	tmux := &testutil.FakeTmux{}
	return cli.WorktimeDeps{
		Clock:               clock,
		Tmux:                tmux,
		UserID:              "user-1",
		ListActiveSessions:  list,
		PauseActiveSession:  pause,
		ResumeActiveSession: resume,
		// PauseMarker: nil  — server mode, no local marker
		// SessionWriter: nil — server mode, all lifecycle via ActiveSessions
	}
}

// TestServerPause_Active verifies that pause in server mode calls
// PauseActiveSession with the right projectID and does NOT panic.
func TestServerPause_Active(t *testing.T) {
	projectID := "proj-abc"
	startedAt := fixedNow.Add(-90 * time.Minute)
	sess := domain.ActiveSession{ProjectID: projectID, StartedAt: startedAt}

	var calledWith string
	deps := serverDeps(
		func(_ string) ([]domain.ActiveSession, error) { return []domain.ActiveSession{sess}, nil },
		func(_ string, pid string) (domain.ActiveSession, error) {
			calledWith = pid
			return sess, nil
		},
		nil,
	)

	var errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(deps)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"pause"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pause: unexpected error: %v", err)
	}
	if calledWith != projectID {
		t.Errorf("PauseActiveSession: want projectID %q, got %q", projectID, calledWith)
	}
	if !strings.Contains(errBuf.String(), "Pausiert") {
		t.Errorf("stderr: expected 'Pausiert', got %q", errBuf.String())
	}
}

// TestServerPause_Idle verifies that pause when no sessions are active
// prints a friendly message and returns nil.
func TestServerPause_Idle(t *testing.T) {
	deps := serverDeps(
		func(_ string) ([]domain.ActiveSession, error) { return nil, nil },
		func(_ string, _ string) (domain.ActiveSession, error) {
			return domain.ActiveSession{}, nil
		},
		nil,
	)

	var errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(deps)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"pause"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("idle pause must not error: %v", err)
	}
	out := errBuf.String()
	if out != "" && !strings.Contains(out, "Keine") {
		t.Errorf("idle pause: want empty or a 'Keine…' message, got %q", out)
	}
}

// TestServerResume_Active verifies that resume in server mode calls
// ResumeActiveSession with the right projectID and does NOT panic.
func TestServerResume_Active(t *testing.T) {
	projectID := "proj-xyz"
	startedAt := fixedNow.Add(-30 * time.Minute)
	sess := domain.ActiveSession{ProjectID: projectID, StartedAt: startedAt}

	var calledWith string
	deps := serverDeps(
		func(_ string) ([]domain.ActiveSession, error) { return []domain.ActiveSession{sess}, nil },
		nil,
		func(_ string, pid string) (domain.ActiveSession, error) {
			calledWith = pid
			return sess, nil
		},
	)

	var errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(deps)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"resume"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("resume: unexpected error: %v", err)
	}
	if calledWith != projectID {
		t.Errorf("ResumeActiveSession: want projectID %q, got %q", projectID, calledWith)
	}
	if !strings.Contains(errBuf.String(), "Resume") {
		t.Errorf("stderr: expected 'Resume', got %q", errBuf.String())
	}
}

// TestServerResume_Idle verifies that resume when no session is running
// prints a friendly message and returns nil (does NOT panic).
func TestServerResume_Idle(t *testing.T) {
	deps := serverDeps(
		func(_ string) ([]domain.ActiveSession, error) { return nil, nil },
		nil,
		func(_ string, _ string) (domain.ActiveSession, error) {
			return domain.ActiveSession{}, nil
		},
	)

	var errBuf bytes.Buffer
	cmd := cli.NewWorktimeCmd(deps)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"resume"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("idle resume must not error: %v", err)
	}
}
