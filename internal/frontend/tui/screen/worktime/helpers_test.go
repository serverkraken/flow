package worktime

// White-box tests for the package-private pure helpers in history.go,
// dayoffs.go, today.go and week.go. Black-box (worktime_test) cannot
// reach unexported funcs; covering them through View() driven tests is
// possible but indirect. These pure helpers are the cheapest path to
// raise per-package coverage above the per-layer 70% target.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — shared test helpers for the worktime package —

// mustTime parses an RFC3339 timestamp or panics with a descriptive
// message. Test-only convenience for building fixed clocks; promoted
// here so every *_test.go file in the package can share one helper
// instead of redefining it locally (cf. plan tasks 1.3/1.4/1.5).
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(fmt.Sprintf("mustTime: cannot parse %q as RFC3339: %v", s, err))
	}
	return t
}

// containsFgSGR reports whether a rendered string carries an ANSI
// foreground-SGR sequence matching the given theme color. lipgloss v2
// emits truecolor as `38;2;R;G;B`; the `#rrggbb` form is what `%v`
// prints but never appears literally in the output. We decode the hex
// to RGB and search for the `;38;2;R;G;B` / `[38;2;R;G;B` substring.
// Mirrors theme/builders_test.go:containsForeground but lives here so
// the worktime tests don't reach into a sibling package.
func containsFgSGR(out string, c theme.Color) bool {
	hex := strings.TrimPrefix(fmt.Sprintf("%v", c), "#")
	if len(hex) != 6 {
		return false
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return false
	}
	return strings.Contains(out, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
}

// — history.go pure helpers —

func TestFilteredHistory_EmptyQueryReturnsAll(t *testing.T) {
	recs := []domain.DayRecord{
		{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)},
	}
	got := filteredHistory(recs, "", time.Now())
	if len(got) != 1 {
		t.Fatalf("empty query should pass-through, got %d records", len(got))
	}
}

func TestFilteredHistory_TagDispatchesToFilterByTag(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	recs := []domain.DayRecord{
		{
			Date: now,
			Sessions: []domain.Session{
				{Tag: "deep", Elapsed: time.Hour},
				{Tag: "shallow", Elapsed: 30 * time.Minute},
			},
			Total: 90 * time.Minute,
		},
	}
	got := filteredHistory(recs, "tag:deep", now)
	if len(got) != 1 || len(got[0].Sessions) != 1 || got[0].Sessions[0].Tag != "deep" {
		t.Errorf("tag:deep filter should keep only deep sessions, got %+v", got)
	}
}

func TestFilterByNote_PrefixOnly(t *testing.T) {
	recs := []domain.DayRecord{
		{Sessions: []domain.Session{{Note: "Alpha", Elapsed: time.Hour}, {Note: "Beta", Elapsed: time.Hour}}},
	}
	out, ok := filterByNote(recs, "tag:foo")
	if ok {
		t.Errorf("non-note: prefix should return ok=false, got %+v", out)
	}
}

func TestFilterByNote_EmptyValueReturnsAll(t *testing.T) {
	recs := []domain.DayRecord{{Sessions: []domain.Session{{Note: "x"}}}}
	out, ok := filterByNote(recs, "note:")
	if !ok {
		t.Fatalf("note: with empty value should be ok=true")
	}
	if len(out) != 1 {
		t.Errorf("empty note: should pass-through all records, got %d", len(out))
	}
}

func TestFilterByNote_CaseInsensitiveSubstring(t *testing.T) {
	recs := []domain.DayRecord{
		{
			Sessions: []domain.Session{
				{Note: "Refactor Auth", Elapsed: time.Hour},
				{Note: "Lunch", Elapsed: 30 * time.Minute},
			},
		},
	}
	out, ok := filterByNote(recs, "note:auth")
	if !ok {
		t.Fatalf("ok=false unexpected")
	}
	if len(out) != 1 || len(out[0].Sessions) != 1 || out[0].Sessions[0].Note != "Refactor Auth" {
		t.Errorf("case-insensitive substring match failed: %+v", out)
	}
}

func TestFilterByISOWeek_NotKWPrefix(t *testing.T) {
	if _, ok := filterByISOWeek(nil, "2026", time.Now()); ok {
		t.Errorf("non-KW prefix must return ok=false")
	}
}

func TestFilterByISOWeek_InvalidNumber(t *testing.T) {
	if _, ok := filterByISOWeek(nil, "KWxx", time.Now()); ok {
		t.Errorf("KWxx must fail to parse → ok=false")
	}
	if _, ok := filterByISOWeek(nil, "KW0", time.Now()); ok {
		t.Errorf("KW0 must be rejected (week 0 invalid)")
	}
}

func TestFilterByISOWeek_KeepsMatchingYearAndWeek(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	wk18Mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local) // ISO week 18 of 2026
	other := time.Date(2025, 4, 28, 0, 0, 0, 0, time.Local)
	recs := []domain.DayRecord{{Date: wk18Mon}, {Date: other}}
	out, ok := filterByISOWeek(recs, "KW18", now)
	if !ok || len(out) != 1 || !out[0].Date.Equal(wk18Mon) {
		t.Errorf("KW18 should keep only the 2026 week-18 record, got %+v", out)
	}
}

func TestFilterByRange_RejectsUnparseable(t *testing.T) {
	if _, ok := filterByRange(nil, "garbage", time.Now()); ok {
		t.Errorf("unparseable range must return ok=false")
	}
}

func TestFilterByRange_KeepsRecordsInRange(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	in := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	out := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	recs := []domain.DayRecord{{Date: in}, {Date: out}}
	got, ok := filterByRange(recs, "2026-05", now)
	if !ok {
		t.Fatalf("YYYY-MM should parse via domain.ParseRange")
	}
	if len(got) != 1 || !got[0].Date.Equal(in) {
		t.Errorf("only May record should remain, got %+v", got)
	}
}

func TestIsTagOrNote(t *testing.T) {
	for _, q := range []string{"tag:foo", "Tag:foo", "note:bar", "NOTE:x"} {
		if !isTagOrNote(q) {
			t.Errorf("isTagOrNote(%q) should be true", q)
		}
	}
	for _, q := range []string{"", "KW18", "2026", "tagfoo", "notebar"} {
		if isTagOrNote(q) {
			t.Errorf("isTagOrNote(%q) should be false", q)
		}
	}
}

func TestIsISOWeek(t *testing.T) {
	for _, q := range []string{"KW1", "KW42", "kw18"} {
		if !isISOWeek(q) {
			t.Errorf("isISOWeek(%q) should be true", q)
		}
	}
	for _, q := range []string{"", "KW", "KW0", "KWfoo", "2026"} {
		if isISOWeek(q) {
			t.Errorf("isISOWeek(%q) should be false", q)
		}
	}
}

func TestStepHistFilter_EmptySeedsCurrentWeek(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local) // ISO week 18
	got, ok := stepHistFilter("", now, 1)
	if !ok {
		t.Fatalf("seeded step should be ok=true")
	}
	if got != "KW19" {
		t.Errorf("step from empty (seeded KW18) +1 should be KW19, got %q", got)
	}
}

func TestStepHistFilter_TagOrNoteIsNotStepable(t *testing.T) {
	now := time.Now()
	if got, ok := stepHistFilter("tag:foo", now, 1); ok || got != "tag:foo" {
		t.Errorf("tag: filter must not step, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_KWForwardAndBack(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	if got, ok := stepHistFilter("KW18", now, -1); !ok || got != "KW17" {
		t.Errorf("KW18 -1 → KW17, got (%q,%v)", got, ok)
	}
	if got, ok := stepHistFilter("KW42", now, 2); !ok || got != "KW44" {
		t.Errorf("KW42 +2 → KW44, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_KWInvalidNumberKeepsAndFails(t *testing.T) {
	got, ok := stepHistFilter("KWXX", time.Now(), 1)
	if ok || got != "KWXX" {
		t.Errorf("invalid KW must keep input + ok=false, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_MonthForwardAndBack(t *testing.T) {
	now := time.Now()
	if got, ok := stepHistFilter("2026-05", now, 1); !ok || got != "2026-06" {
		t.Errorf("2026-05 +1 → 2026-06, got (%q,%v)", got, ok)
	}
	if got, ok := stepHistFilter("2026-01", now, -1); !ok || got != "2025-12" {
		t.Errorf("2026-01 -1 → 2025-12, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_MonthUnparseable(t *testing.T) {
	got, ok := stepHistFilter("9999-99", time.Now(), 1)
	if ok || got != "9999-99" {
		t.Errorf("invalid YYYY-MM must keep input, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_YearForwardAndBack(t *testing.T) {
	now := time.Now()
	if got, ok := stepHistFilter("2026", now, 1); !ok || got != "2027" {
		t.Errorf("2026 +1 → 2027, got (%q,%v)", got, ok)
	}
	if got, ok := stepHistFilter("2026", now, -3); !ok || got != "2023" {
		t.Errorf("2026 -3 → 2023, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_YearUnparseable(t *testing.T) {
	got, ok := stepHistFilter("ABCD", time.Now(), 1)
	if ok || got != "ABCD" {
		t.Errorf("4-char non-numeric must keep input, got (%q,%v)", got, ok)
	}
}

func TestStepHistFilter_UnknownShape(t *testing.T) {
	got, ok := stepHistFilter("xyz", time.Now(), 1)
	if ok || got != "xyz" {
		t.Errorf("unknown shape must keep input, got (%q,%v)", got, ok)
	}
}

func TestIsoMondayOfISOWeek_Week1And42(t *testing.T) {
	// 2026-W01 starts on 2025-12-29 (Mon), per ISO 8601 (4-day overlap rule).
	got := isoMondayOfISOWeek(2026, 1, time.Local)
	want := time.Date(2025, 12, 29, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("2026-W01 monday: got %s want %s", got, want)
	}
	// Sanity: week 18 of 2026 → 2026-04-27.
	got = isoMondayOfISOWeek(2026, 18, time.Local)
	want = time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("2026-W18 monday: got %s want %s", got, want)
	}
}

func TestIsoMondayOfISOWeek_SundayJan4Branch(t *testing.T) {
	// 2026-01-04 is a Sunday → triggers the wd==0 branch (mapped to 7).
	// Verify that branch is exercised and the math still produces a Monday.
	got := isoMondayOfISOWeek(2026, 1, time.Local)
	if got.Weekday() != time.Monday {
		t.Errorf("week-1 anchor must be a Monday, got %s (%s)", got, got.Weekday())
	}
}

func TestMonthClampDay(t *testing.T) {
	may := time.Date(2026, 5, 15, 0, 0, 0, 0, time.Local)  // 31-day month
	feb := time.Date(2026, 2, 10, 0, 0, 0, 0, time.Local)  // 28-day month
	leap := time.Date(2024, 2, 10, 0, 0, 0, 0, time.Local) // 29-day month
	cases := []struct {
		ref      time.Time
		day, out int
	}{
		{may, 0, 1},    // clamp low
		{may, -5, 1},   // clamp low
		{may, 1, 1},    // edge
		{may, 31, 31},  // edge
		{may, 32, 31},  // clamp high
		{feb, 28, 28},  // edge
		{feb, 29, 28},  // clamp into 28
		{feb, 30, 28},  // clamp high
		{leap, 29, 29}, // leap-year keeps 29
		{leap, 30, 29}, // clamp high
	}
	for _, c := range cases {
		if got := monthClampDay(c.ref, c.day); got != c.out {
			t.Errorf("monthClampDay(%s, %d) = %d, want %d", c.ref.Format("2006-01"), c.day, got, c.out)
		}
	}
}

func TestTagClockCellGlyph_Levels(t *testing.T) {
	pal := theme.Palette{}
	cases := []struct {
		cell time.Duration
		frac float64
		want string
	}{
		{0, 0.0, "··"},
		{time.Minute, 0.0001, "░░"}, // > 0 but tiny
		{time.Hour, 0.3, "▒▒"},      // 0.25 ≤ frac < 0.5
		{time.Hour, 0.5, "▓▓"},      // 0.5  ≤ frac < 0.75
		{time.Hour, 0.9, "██"},      // ≥ 0.75
	}
	for _, c := range cases {
		got, _ := tagClockCellGlyph(pal, c.cell, c.frac)
		if got != c.want {
			t.Errorf("tagClockCellGlyph(cell=%s frac=%.3f) = %q, want %q", c.cell, c.frac, got, c.want)
		}
	}
}

// — today.go pure helper —

func TestFormatDurLive(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0h 00m 00s"},
		{-time.Hour, "0h 00m 00s"}, // negative → 0
		{time.Hour + 30*time.Minute + 5*time.Second, "1h 30m 05s"},
		{2*time.Hour + 0*time.Minute + 1*time.Second, "2h 00m 01s"},
	}
	for _, c := range cases {
		if got := formatDurLive(c.d); got != c.want {
			t.Errorf("formatDurLive(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
