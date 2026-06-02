package tsvsessions_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/tsvsessions"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Compile-time check that *Store satisfies the SessionStore port.
var _ ports.SessionStore = (*tsvsessions.Store)(nil)

func mkSession(t *testing.T, day, start, stop string, elapsedSec int, tag, note string) domain.Session {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		t.Fatalf("parse date %q: %v", day, err)
	}
	sh, err := domain.ParseHM(start)
	if err != nil {
		t.Fatalf("parse start %q: %v", start, err)
	}
	eh, err := domain.ParseHM(stop)
	if err != nil {
		t.Fatalf("parse stop %q: %v", stop, err)
	}
	return domain.Session{
		Date:    d,
		Start:   d.Add(sh),
		Stop:    d.Add(eh),
		Elapsed: time.Duration(elapsedSec) * time.Second,
		Tag:     tag,
		Note:    note,
	}
}

func TestLoadAll_MissingFile(t *testing.T) {
	s := tsvsessions.New(filepath.Join(t.TempDir(), "missing.log"))
	got, err := s.LoadAllLegacy()
	if err != nil {
		t.Fatalf("LoadAll on missing file: %v", err)
	}
	if got != nil {
		t.Fatalf("want nil slice, got %v", got)
	}
}

func TestLoadAll_MixedColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worktime.log")
	// 4-col legacy, 5-col with tag, 6-col with tag+note.
	body := "" +
		"2026-04-29\t09:00\t10:00\t3600\n" +
		"2026-04-29\t11:00\t12:00\t3600\tdeep\n" +
		"2026-04-29\t13:00\t14:30\t5400\tdeep\twrote plan\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := tsvsessions.New(path).LoadAllLegacy()
	if err != nil {
		t.Fatalf("LoadAllLegacy: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(got))
	}
	if got[0].Tag != "" || got[0].Note != "" {
		t.Errorf("row 0: want empty tag/note, got %+v", got[0])
	}
	if got[1].Tag != "deep" || got[1].Note != "" {
		t.Errorf("row 1: want tag=deep, empty note, got %+v", got[1])
	}
	if got[2].Tag != "deep" || got[2].Note != "wrote plan" {
		t.Errorf("row 2: want tag=deep, note=wrote plan, got %+v", got[2])
	}
	if got[0].Elapsed != time.Hour {
		t.Errorf("row 0 elapsed: want 1h, got %v", got[0].Elapsed)
	}
}

func TestLoadAll_SkipsInvalidLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worktime.log")
	body := "" +
		"# leading comment\n" +
		"\n" +
		"   \n" +
		"2026-04-29\t09:00\t10:00\t3600\n" + // valid
		"2026-04-29\t09:00\t10:00\n" + // too few cols
		"BOGUS\t09:00\t10:00\t3600\n" + // bad date
		"2026-04-29\tXX:00\t10:00\t3600\n" + // bad start
		"2026-04-29\t09:00\tXX:00\t3600\n" + // bad stop
		"2026-04-29\t09:00\t10:00\tabc\n" + // bad elapsed
		"2026-04-29\t11:00\t12:00\t3600\n" // valid
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := tsvsessions.New(path).LoadAllLegacy()
	if err != nil {
		t.Fatalf("LoadAllLegacy: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 valid sessions, got %d: %+v", len(got), got)
	}
}

func TestLoadFiltered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worktime.log")
	body := "" +
		"2026-04-28\t09:00\t10:00\t3600\n" +
		"2026-04-29\t11:00\t12:00\t3600\n" +
		"2026-04-30\t13:00\t14:30\t5400\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	target, _ := time.ParseInLocation("2006-01-02", "2026-04-29", time.Local)
	got, err := tsvsessions.New(path).LoadFilteredLegacy(func(s domain.Session) bool {
		return s.Date.Equal(target)
	})
	if err != nil {
		t.Fatalf("LoadFilteredLegacy: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session, got %d", len(got))
	}
	if !got[0].Date.Equal(target) {
		t.Errorf("filtered session date mismatch: %v", got[0].Date)
	}
}

func TestAppend_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-dir", "worktime.log")
	store := tsvsessions.New(path)

	if err := store.Append(mkSession(t, "2026-04-30", "09:00", "10:00", 3600, "", "")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestAppend_FormatVariants(t *testing.T) {
	cases := []struct {
		name      string
		sess      domain.Session
		wantCols  int
		wantTrail string
	}{
		{"empty tag and note → 4 cols", mkSessionFor("2026-04-30", "09:00", "10:00", 3600, "", ""), 4, ""},
		{"tag only → 5 cols", mkSessionFor("2026-04-30", "11:00", "12:00", 3600, "deep", ""), 5, "\tdeep"},
		{"tag and note → 6 cols", mkSessionFor("2026-04-30", "13:00", "14:30", 5400, "deep", "plan"), 6, "\tdeep\tplan"},
		{"empty tag, note set → 6 cols with empty tag", mkSessionFor("2026-04-30", "15:00", "16:00", 3600, "", "note only"), 6, "\t\tnote only"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "worktime.log")
			store := tsvsessions.New(path)
			if err := store.Append(tc.sess); err != nil {
				t.Fatalf("Append: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			line := strings.TrimRight(string(raw), "\n")
			cols := strings.Count(line, "\t") + 1
			if cols != tc.wantCols {
				t.Errorf("col count: want %d, got %d (%q)", tc.wantCols, cols, line)
			}
			if tc.wantTrail != "" && !strings.HasSuffix(line, tc.wantTrail) {
				t.Errorf("trail: want suffix %q, got %q", tc.wantTrail, line)
			}

			// Round-trip read must yield identical values.
			got, err := store.LoadAllLegacy()
			if err != nil {
				t.Fatalf("LoadAllLegacy: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("want 1 session, got %d", len(got))
			}
			if got[0].Tag != tc.sess.Tag || got[0].Note != tc.sess.Note {
				t.Errorf("round-trip: got tag=%q note=%q, want tag=%q note=%q",
					got[0].Tag, got[0].Note, tc.sess.Tag, tc.sess.Note)
			}
		})
	}
}

// mkSessionFor mirrors mkSession but without a *testing.T receiver, so it can
// be used in table literals.
func mkSessionFor(day, start, stop string, elapsedSec int, tag, note string) domain.Session {
	d, _ := time.ParseInLocation("2006-01-02", day, time.Local)
	sh, _ := domain.ParseHM(start)
	eh, _ := domain.ParseHM(stop)
	return domain.Session{
		Date:    d,
		Start:   d.Add(sh),
		Stop:    d.Add(eh),
		Elapsed: time.Duration(elapsedSec) * time.Second,
		Tag:     tag,
		Note:    note,
	}
}

func TestRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktime.log")
	store := tsvsessions.New(path)

	if err := store.Append(mkSession(t, "2026-04-29", "09:00", "10:00", 3600, "old", "")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(mkSession(t, "2026-04-29", "11:00", "12:00", 3600, "old", "")); err != nil {
		t.Fatal(err)
	}

	replacement := []domain.Session{
		mkSession(t, "2026-04-30", "13:00", "14:00", 3600, "new", "after rewrite"),
	}
	if err := store.Rewrite(replacement); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}

	got, err := store.LoadAllLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "new" {
		t.Errorf("after rewrite: got %+v", got)
	}

	// Tmp file must not linger after a successful rename.
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".tmp file should not exist post-rewrite (err=%v)", err)
	}
}

func TestRewrite_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktime.log")
	store := tsvsessions.New(path)

	if err := store.Append(mkSession(t, "2026-04-29", "09:00", "10:00", 3600, "x", "")); err != nil {
		t.Fatal(err)
	}
	if err := store.Rewrite(nil); err != nil {
		t.Fatalf("Rewrite(nil): %v", err)
	}

	got, err := store.LoadAllLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("after empty rewrite: want 0, got %d", len(got))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("file should be 0 bytes, got %d", info.Size())
	}
}

func TestRewrite_CreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "worktime.log")
	store := tsvsessions.New(path)

	if err := store.Rewrite([]domain.Session{
		mkSession(t, "2026-04-30", "09:00", "10:00", 3600, "", ""),
	}); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestRewrite_Multiple(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktime.log")
	store := tsvsessions.New(path)

	err := store.Rewrite([]domain.Session{
		mkSession(t, "2026-04-29", "09:00", "10:00", 3600, "a", ""),
		mkSession(t, "2026-04-29", "11:00", "12:00", 3600, "b", "n"),
		mkSession(t, "2026-04-30", "13:00", "14:00", 3600, "", ""),
	})
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, err := store.LoadAllLegacy()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(got))
	}
}

// pathUnderRegularFile constructs a path whose parent directory is actually a
// regular file. Operations that need to create or open under that path fail
// with ENOTDIR (not ErrNotExist).
func pathUnderRegularFile(t *testing.T, leaf string) string {
	t.Helper()
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(regular, leaf)
}

func TestLoadAll_OpenError(t *testing.T) {
	store := tsvsessions.New(pathUnderRegularFile(t, "child"))
	_, err := store.LoadAllLegacy()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("want non-NotExist error, got %v", err)
	}
}

func TestAppend_MkdirError(t *testing.T) {
	store := tsvsessions.New(pathUnderRegularFile(t, "subdir/log"))
	err := store.Append(mkSession(t, "2026-04-30", "09:00", "10:00", 3600, "", ""))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestRewrite_MkdirError(t *testing.T) {
	store := tsvsessions.New(pathUnderRegularFile(t, "subdir/log"))
	err := store.Rewrite([]domain.Session{
		mkSession(t, "2026-04-30", "09:00", "10:00", 3600, "", ""),
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
