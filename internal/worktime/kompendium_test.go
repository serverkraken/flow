package worktime_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/worktime"
)

func TestDailyNoteID(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 4, 28, 10, 30, 0, 0, time.Local)
	got := worktime.DailyNoteID(d)
	want := "daily/2026-04-28"
	if got != want {
		t.Errorf("DailyNoteID = %q, want %q", got, want)
	}
}

func TestHumanizeNoteID(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"daily/2026-04-28":                     "Daily 2026-04-28",
		"projects/github.com/serverkraken/foo": "Projekt github.com/serverkraken/foo",
		"notes/some-zettel":                    "Notiz some-zettel",
		"unprefixed":                           "unprefixed",
		"":                                     "",
	}
	for in, want := range cases {
		if got := worktime.HumanizeNoteID(in); got != want {
			t.Errorf("HumanizeNoteID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenNote_RejectsEmptyID(t *testing.T) {
	t.Parallel()
	if err := worktime.OpenNote(""); err == nil {
		t.Error("expected error for empty id")
	}
	if err := worktime.OpenNote("   "); err == nil {
		t.Error("expected error for whitespace id")
	}
}
