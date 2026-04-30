package worktime_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

// withTempDayOffs points the day-offs cache at a fresh file under t.TempDir.
// Returns the file path so the test can pre-seed content if needed.
func withTempDayOffs(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	t.Setenv("WORKTIME_DAYOFFS_FILE", path)
	worktime.ResetCachesForTesting()
	t.Cleanup(worktime.ResetCachesForTesting)
	return path
}

func TestParseKind(t *testing.T) {
	useEmptyDayOffs(t)
	cases := map[string]worktime.Kind{
		"holiday":  worktime.KindHoliday,
		"H":        worktime.KindHoliday,
		"Feiertag": worktime.KindHoliday,
		"vacation": worktime.KindVacation,
		"v":        worktime.KindVacation,
		"Urlaub":   worktime.KindVacation,
		"sick":     worktime.KindSick,
		"krank":    worktime.KindSick,
	}
	for in, want := range cases {
		got, ok := worktime.ParseKind(in)
		if !ok || got != want {
			t.Errorf("ParseKind(%q) = (%q, %v), want (%q, true)", in, got, ok, want)
		}
	}
	if _, ok := worktime.ParseKind("garbage"); ok {
		t.Error("ParseKind('garbage') should fail")
	}
}

func TestAddDayOff_PersistsAndUpdatesTarget(t *testing.T) {
	withTempDayOffs(t)
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := worktime.AddDayOff(d, worktime.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	if !worktime.IsHoliday(d) {
		t.Error("expected IsHoliday=true after add")
	}
	if got := worktime.HolidayName(d); got != "Tag der Arbeit" {
		t.Errorf("HolidayName = %q, want %q", got, "Tag der Arbeit")
	}
	if got := worktime.TargetFor(d); got != 0 {
		t.Errorf("TargetFor on holiday = %v, want 0", got)
	}
}

func TestAddDayOffRange_CoversAllDays(t *testing.T) {
	withTempDayOffs(t)
	from := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local) // Mon
	to := time.Date(2026, 7, 17, 0, 0, 0, 0, time.Local)   // Fri
	n, err := worktime.AddDayOffRange(from, to, worktime.KindVacation, "Sommer")
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("added %d entries, want 5", n)
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !worktime.IsDayOff(d) {
			t.Errorf("%s should be a vacation day", d.Format("2006-01-02"))
		}
	}
}

func TestRemoveDayOff(t *testing.T) {
	withTempDayOffs(t)
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := worktime.AddDayOff(d, worktime.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	if err := worktime.RemoveDayOff(d); err != nil {
		t.Fatal(err)
	}
	if worktime.IsHoliday(d) {
		t.Error("entry should be gone after remove")
	}
}

func TestListDayOffs_Sorted(t *testing.T) {
	withTempDayOffs(t)
	dates := []string{"2026-05-01", "2026-04-06", "2026-12-25"}
	for _, ds := range dates {
		d, _ := time.ParseInLocation("2006-01-02", ds, time.Local)
		_ = worktime.AddDayOff(d, worktime.KindHoliday, ds)
	}
	out := worktime.ListDayOffs(time.Time{}, time.Time{})
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	for i := 1; i < len(out); i++ {
		if out[i].Date.Before(out[i-1].Date) {
			t.Error("ListDayOffs not sorted ascending")
		}
	}
}

func TestLegacyHolidaysFile_StillReadable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("WORKTIME_DAYOFFS_FILE", "")
	t.Setenv("WORKTIME_HOLIDAYS_FILE", "")
	if err := os.MkdirAll(filepath.Join(dir, ".tmux"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Write the legacy filename without kind column → must be parsed as holiday.
	content := "2026-01-01\tNeujahr\n"
	if err := os.WriteFile(filepath.Join(dir, ".tmux", "worktime-holidays.tsv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	worktime.ResetCachesForTesting()
	t.Cleanup(worktime.ResetCachesForTesting)

	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if !worktime.IsHoliday(d) {
		t.Error("legacy holidays file should still be read")
	}
}

func TestTargetFor_HolidayReturnsZero(t *testing.T) {
	withTempDayOffs(t)
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := worktime.AddDayOff(d, worktime.KindHoliday, "Tag der Arbeit"); err != nil {
		t.Fatal(err)
	}
	if got := worktime.TargetFor(d); got != 0 {
		t.Errorf("TargetFor on holiday = %v, want 0", got)
	}
}

func TestIsWorkday(t *testing.T) {
	withTempDayOffs(t)
	holiday := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local) // Fri
	_ = worktime.AddDayOff(holiday, worktime.KindHoliday, "Tag der Arbeit")
	saturday := time.Date(2026, 5, 2, 0, 0, 0, 0, time.Local)
	monday := time.Date(2026, 5, 4, 0, 0, 0, 0, time.Local)
	if worktime.IsWorkday(holiday) {
		t.Error("holiday should not be a workday")
	}
	if worktime.IsWorkday(saturday) {
		t.Error("Saturday should not be a workday")
	}
	if !worktime.IsWorkday(monday) {
		t.Error("ordinary Monday should be a workday")
	}
}
