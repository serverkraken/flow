package dayoffstsv_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/dayoffstsv"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.DayOffStore = (*dayoffstsv.Store)(nil)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestList_EmptyStore(t *testing.T) {
	s := dayoffstsv.New(filepath.Join(t.TempDir(), "missing.tsv"), "")
	if got := s.List(time.Time{}, time.Time{}); len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestLookup_EmptyStore(t *testing.T) {
	s := dayoffstsv.New(filepath.Join(t.TempDir(), "missing.tsv"), "")
	_, ok := s.Lookup(mustParseDate(t, "2026-04-30"))
	if ok {
		t.Error("Lookup on missing file should report not-found")
	}
}

func TestRead_ModernAndLegacyRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dayoffs.tsv")
	body := "" +
		"# worktime day-offs\n" +
		"# kinds: holiday | vacation | sick\n" +
		"\n" +
		"2026-04-18\tholiday\tKarfreitag\n" +
		"2026-05-01\tvacation\tUrlaub Mai\t4.5\n" +
		"2026-05-15\tsick\tKrank\n" +
		// legacy 2-col
		"2026-12-25\tWeihnachten\n" +
		// legacy 3-col with hours
		"2026-12-31\tSilvester halb\t4\n" +
		// invalid rows
		"\t\n" +
		"BOGUS\tholiday\tnope\n" +
		"2026-06-01\n" + // too few cols
		"2026-06-02\tholiday\tneghours\t-1\n" + // negative hours → 0
		"2026-06-03\tholiday\tbadhours\tabc\n" // non-numeric → 0
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := dayoffstsv.New(path, "")

	got := s.List(time.Time{}, time.Time{})
	if len(got) != 7 {
		t.Fatalf("want 7 entries, got %d: %+v", len(got), got)
	}

	checks := map[string]struct {
		kind   domain.Kind
		label  string
		target time.Duration
	}{
		"2026-04-18": {domain.KindHoliday, "Karfreitag", 0},
		"2026-05-01": {domain.KindVacation, "Urlaub Mai", 4*time.Hour + 30*time.Minute},
		"2026-05-15": {domain.KindSick, "Krank", 0},
		"2026-12-25": {domain.KindHoliday, "Weihnachten", 0},
		"2026-12-31": {domain.KindHoliday, "Silvester halb", 4 * time.Hour},
		"2026-06-02": {domain.KindHoliday, "neghours", 0},
		"2026-06-03": {domain.KindHoliday, "badhours", 0},
	}
	for date, want := range checks {
		entry, ok := s.Lookup(mustParseDate(t, date))
		if !ok {
			t.Errorf("Lookup %s: not found", date)
			continue
		}
		if entry.Kind != want.kind {
			t.Errorf("%s kind: want %s, got %s", date, want.kind, entry.Kind)
		}
		if entry.Label != want.label {
			t.Errorf("%s label: want %q, got %q", date, want.label, entry.Label)
		}
		if entry.Target != want.target {
			t.Errorf("%s target: want %v, got %v", date, want.target, entry.Target)
		}
	}
}

func TestList_FromToBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dayoffs.tsv")
	body := "" +
		"2026-04-18\tholiday\tA\n" +
		"2026-05-01\tholiday\tB\n" +
		"2026-06-15\tholiday\tC\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s := dayoffstsv.New(path, "")

	from := mustParseDate(t, "2026-05-01")
	to := mustParseDate(t, "2026-05-31")
	got := s.List(from, to)
	if len(got) != 1 || got[0].Label != "B" {
		t.Errorf("from..to bound: got %+v", got)
	}

	got = s.List(from, time.Time{})
	if len(got) != 2 || got[0].Label != "B" || got[1].Label != "C" {
		t.Errorf("from-only bound: got %+v", got)
	}

	got = s.List(time.Time{}, to)
	if len(got) != 2 || got[0].Label != "A" || got[1].Label != "B" {
		t.Errorf("to-only bound: got %+v", got)
	}

	got = s.List(time.Time{}, time.Time{})
	if len(got) != 3 {
		t.Errorf("no bound: want 3, got %d", len(got))
	}
}

func TestAdd_NewEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	off := domain.DayOff{
		Date:  mustParseDate(t, "2026-04-30"),
		Kind:  domain.KindHoliday,
		Label: "Test",
	}
	if err := s.Add(off); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := s.Lookup(off.Date)
	if !ok {
		t.Fatal("Lookup after Add: not found")
	}
	if got.Label != "Test" || got.Kind != domain.KindHoliday {
		t.Errorf("after Add: got %+v", got)
	}

	// File should have header lines + the entry row.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-04-30\tholiday\tTest"; !contains(string(raw), want) {
		t.Errorf("file body missing %q\n%s", want, string(raw))
	}
}

func TestAdd_WithTargetHours(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	off := domain.DayOff{
		Date:   mustParseDate(t, "2026-05-01"),
		Kind:   domain.KindVacation,
		Label:  "Halbtag",
		Target: 4 * time.Hour,
	}
	if err := s.Add(off); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if !contains(string(raw), "2026-05-01\tvacation\tHalbtag\t4") {
		t.Errorf("hours not written: %s", string(raw))
	}

	got, ok := s.Lookup(off.Date)
	if !ok || got.Target != 4*time.Hour {
		t.Errorf("round-trip: got %+v ok=%v", got, ok)
	}
}

func TestAdd_ReplacesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	d := mustParseDate(t, "2026-04-30")
	if err := s.Add(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(domain.DayOff{Date: d, Kind: domain.KindVacation, Label: "Second"}); err != nil {
		t.Fatal(err)
	}

	got, ok := s.Lookup(d)
	if !ok {
		t.Fatal("not found")
	}
	if got.Kind != domain.KindVacation || got.Label != "Second" {
		t.Errorf("replace: got %+v", got)
	}
	if all := s.List(time.Time{}, time.Time{}); len(all) != 1 {
		t.Errorf("want 1 entry total, got %d", len(all))
	}
}

func TestRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	d := mustParseDate(t, "2026-04-30")
	if err := s.Add(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "X"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove(d); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := s.Lookup(d); ok {
		t.Error("entry still present after Remove")
	}
}

func TestRemove_Missing_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	if err := s.Remove(mustParseDate(t, "2026-04-30")); err != nil {
		t.Errorf("Remove on empty: %v", err)
	}

	// And again on a populated store but a missing date.
	if err := s.Add(domain.DayOff{
		Date: mustParseDate(t, "2026-05-01"), Kind: domain.KindHoliday, Label: "X",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(mustParseDate(t, "2026-05-02")); err != nil {
		t.Errorf("Remove of non-existent date: %v", err)
	}
}

func TestLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "dayoffs.tsv")
	legacy := filepath.Join(dir, "holidays.tsv")
	if err := os.WriteFile(legacy, []byte("2026-04-18\tholiday\tKarfreitag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := dayoffstsv.New(primary, legacy)

	got, ok := s.Lookup(mustParseDate(t, "2026-04-18"))
	if !ok {
		t.Fatal("legacy fallback: lookup not found")
	}
	if got.Label != "Karfreitag" {
		t.Errorf("legacy label: got %q", got.Label)
	}
}

func TestLegacyFallback_NotConsultedWhenPrimaryExists(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "dayoffs.tsv")
	legacy := filepath.Join(dir, "holidays.tsv")
	if err := os.WriteFile(primary, []byte("# empty primary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("2026-04-18\tholiday\tKarfreitag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := dayoffstsv.New(primary, legacy)

	if _, ok := s.Lookup(mustParseDate(t, "2026-04-18")); ok {
		t.Error("legacy entry leaked through despite primary existing")
	}
}

func TestCache_InvalidatesOnWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")
	s := dayoffstsv.New(path, "")

	d := mustParseDate(t, "2026-04-30")
	// Prime the cache via List.
	if all := s.List(time.Time{}, time.Time{}); len(all) != 0 {
		t.Fatalf("initial list: got %v", all)
	}
	if err := s.Add(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "X"}); err != nil {
		t.Fatal(err)
	}
	// Cache must reflect the Add.
	if _, ok := s.Lookup(d); !ok {
		t.Error("cache stale after Add")
	}
}

func TestAdd_MkdirError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	s := dayoffstsv.New(filepath.Join(regular, "subdir", "dayoffs.tsv"), "")
	err := s.Add(domain.DayOff{
		Date: mustParseDate(t, "2026-04-30"), Kind: domain.KindHoliday, Label: "X",
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestAdd_TwoInstancesSamePath_NoDataLoss simulates two independent
// processes (e.g. CLI + TUI) writing distinct dates concurrently. The
// in-process sync.Mutex does not protect against this case — only the
// POSIX file lock on the .lock sibling does. Without flock, one of the
// atomicfile.WriteFile renames would silently discard the other's row.
func TestAdd_TwoInstancesSamePath_NoDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dayoffs.tsv")

	const writesPerStore = 25
	s1 := dayoffstsv.New(path, "")
	s2 := dayoffstsv.New(path, "")

	add := func(s *dayoffstsv.Store, prefix string) error {
		for i := 0; i < writesPerStore; i++ {
			d := mustParseDate(t, fmt.Sprintf("2026-%02d-%02d", (i%12)+1, (i%28)+1))
			if err := s.Add(domain.DayOff{
				Date: d, Kind: domain.KindVacation,
				Label: fmt.Sprintf("%s-%d", prefix, i),
			}); err != nil {
				return err
			}
		}
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs <- add(s1, "A") }()
	go func() { defer wg.Done(); errs <- add(s2, "B") }()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// Fresh reader to bypass any per-instance cache.
	verifier := dayoffstsv.New(path, "")
	got := verifier.List(time.Time{}, time.Time{})
	// Both stores wrote to the same 25 keys (i % 12, i % 28 collides
	// across the two stores). Either A or B is the winner per key, but
	// no key may go missing: total entries == 25.
	if len(got) != writesPerStore {
		t.Fatalf("after concurrent Add across two stores: got %d entries, want %d", len(got), writesPerStore)
	}
}
