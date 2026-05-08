// White-box tests for the Range-Form sub-picker.

package worktime

import (
	"strings"
	"testing"
	"time"
)

// fixedNowForRange is the anchor for ParseRange in these tests. Wed,
// 2026-05-06 makes "week" → 2026-05-04..05-11 and "month" →
// 2026-05-01..06-01. Stable enough for the validation paths we
// exercise.
var fixedNowForRange = time.Date(2026, 5, 6, 12, 0, 0, 0, time.Local)

func TestRangeForm_NewPrefillsDefault(t *testing.T) {
	r := newRangeForm(pal(), "month", "Stats für Range")
	if r.input.Value() != "month" {
		t.Errorf("default = %q, want month", r.input.Value())
	}
	if r.parent != "Stats für Range" {
		t.Errorf("parent = %q, want Stats für Range", r.parent)
	}
	if r.errMsg != "" {
		t.Errorf("fresh form should have no errMsg; got %q", r.errMsg)
	}
}

func TestRangeForm_EnterValidatesAgainstParseRange(t *testing.T) {
	r := newRangeForm(pal(), "month", "Stats")
	_, _, ev := r.handleKey(keyName("enter"), fixedNowForRange)
	if !ev.submitted {
		t.Fatal("Enter on a valid expression must submit")
	}
	if ev.expr != "month" {
		t.Errorf("submitted expr = %q, want month", ev.expr)
	}
}

func TestRangeForm_EnterEmptyExpressionAccepted(t *testing.T) {
	r := newRangeForm(pal(), "", "Export")
	_, _, ev := r.handleKey(keyName("enter"), fixedNowForRange)
	if !ev.submitted {
		t.Fatal("empty range should submit (= all time)")
	}
	if ev.expr != "" {
		t.Errorf("expr = %q, want empty", ev.expr)
	}
}

func TestRangeForm_EnterInvalidExpressionPopulatesErr(t *testing.T) {
	r := newRangeForm(pal(), "garbage-input", "Stats")
	r2, _, ev := r.handleKey(keyName("enter"), fixedNowForRange)
	if ev.submitted || ev.canceled {
		t.Errorf("invalid range should NOT submit/cancel; got %+v", ev)
	}
	if r2.errMsg == "" {
		t.Error("invalid range must populate errMsg")
	}
}

func TestRangeForm_EditClearsErrMsg(t *testing.T) {
	r := newRangeForm(pal(), "garbage-input", "Stats")
	r, _, _ = r.handleKey(keyName("enter"), fixedNowForRange)
	if r.errMsg == "" {
		t.Fatal("precondition: errMsg should be set after invalid submit")
	}
	r, _, _ = r.handleKey(runeKey('x'), fixedNowForRange)
	if r.errMsg != "" {
		t.Errorf("typing must clear errMsg; got %q", r.errMsg)
	}
}

func TestRangeForm_EscCancels(t *testing.T) {
	r := newRangeForm(pal(), "month", "Stats")
	_, _, ev := r.handleKey(keyName("esc"), fixedNowForRange)
	if !ev.canceled {
		t.Error("Esc should cancel the form")
	}
}

func TestRangeForm_ViewRendersInputAndExamples(t *testing.T) {
	r := newRangeForm(pal(), "month", "Export CSV")
	out := r.view(pal(), 100)
	for _, want := range []string{
		"Export CSV",
		"Beispiele",
		"2026-04-01..2026-04-30",
		"enter → weiter",
		"leer → alles",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("range form view missing %q in:\n%s", want, out)
		}
	}
}

func TestRangeForm_ViewSurfacesErrMsg(t *testing.T) {
	r := newRangeForm(pal(), "garbage", "Stats")
	r, _, _ = r.handleKey(keyName("enter"), fixedNowForRange)
	out := r.view(pal(), 100)
	if r.errMsg == "" {
		t.Fatal("precondition: errMsg must be set")
	}
	if !strings.Contains(out, r.errMsg) {
		t.Errorf("view must render errMsg; got:\n%s", out)
	}
}

func TestDefaultRangeFor_PerKind(t *testing.T) {
	cases := []struct {
		kind menuActionKind
		want string
	}{
		{menuActionStats, "month"},
		{menuActionExportCSV, "month"},
		{menuActionExportJSON, "month"},
		{menuActionBriefWeek, ""},
		{menuActionLand, ""},
	}
	for _, c := range cases {
		if got := defaultRangeFor(c.kind); got != c.want {
			t.Errorf("defaultRangeFor(%v) = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestViewerForKind_ChoosesPerOutputType(t *testing.T) {
	cases := []struct {
		kind menuActionKind
		want string
	}{
		{menuActionBriefWeek, briefViewer},
		{menuActionBriefMonth, briefViewer},
		{menuActionExportCSV, exportPager},
		{menuActionExportJSON, exportPager},
		{menuActionStats, statsPager},
	}
	for _, c := range cases {
		if got := viewerForKind(c.kind); got != c.want {
			t.Errorf("viewerForKind(%v) = %q, want %q", c.kind, got, c.want)
		}
	}
}
