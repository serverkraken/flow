package domain_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWriteBrief_FullExample(t *testing.T) {
	mon := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	tue := time.Date(2026, 4, 28, 0, 0, 0, 0, time.Local)

	records := []domain.DayRecord{
		// Newest-first (matches LoadHistory's actual order).
		{
			Date: tue, Target: 8 * time.Hour, Total: 8 * time.Hour,
			Sessions: []domain.Session{
				{Start: tue.Add(9 * time.Hour), Stop: tue.Add(17 * time.Hour), Elapsed: 8 * time.Hour, Tag: "deep", Note: "auth refactor"},
			},
		},
		{
			Date: mon, Target: 8 * time.Hour, Total: 6 * time.Hour,
			Sessions: []domain.Session{
				{Start: mon.Add(9 * time.Hour), Stop: mon.Add(11 * time.Hour), Elapsed: 2 * time.Hour, Tag: "deep"},
				{Start: mon.Add(12 * time.Hour), Stop: mon.Add(16 * time.Hour), Elapsed: 4 * time.Hour},
			},
		},
	}
	stats := domain.Aggregate(records, func(time.Time) bool { return true }, nil)
	in := domain.BriefInputs{
		Title:   "Worktime · KW 18 · 27.04. – 03.05.2026",
		Records: records,
		Stats:   stats,
		Planned: 16 * time.Hour,
		DayOffs: []domain.DayOff{
			{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit"},
		},
	}
	var buf bytes.Buffer
	if err := domain.WriteBrief(&buf, in); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	mustContain := []string{
		"# Worktime · KW 18 · 27.04. – 03.05.2026",
		"## Übersicht",
		"- **Total**: 14h 00m / 16h 00m",
		"## Tage",
		"**Mo, 27.04.**",
		"09:00–11:00",
		"12:00–16:00",
		"**Di, 28.04.**",
		"09:00–17:00",
		"[deep]",
		"— auth refactor",
		"## Tags",
		"- **deep** —",
		"_(ohne Tag)_",
		"## Frei",
		"**Fr, 01.05.** — Feiertag  ·  Tag der Arbeit",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}

	// Records are newest-first in input; brief renders chronologically (Mo
	// before Di). Verify that ordering.
	mo := strings.Index(out, "**Mo, 27.04.**")
	di := strings.Index(out, "**Di, 28.04.**")
	if mo < 0 || di < 0 || mo > di {
		t.Errorf("brief should render chronologically: Mo before Di, got Mo=%d Di=%d", mo, di)
	}
}

func TestWriteBrief_NoRecordsSkipsTageAndTags(t *testing.T) {
	var buf bytes.Buffer
	in := domain.BriefInputs{
		Title:   "Worktime · KW 18",
		Stats:   domain.Stats{ByTag: map[string]time.Duration{}, CountByTag: map[string]int{}},
		Planned: 0,
	}
	if err := domain.WriteBrief(&buf, in); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "## Tage") {
		t.Error("empty brief should not include ## Tage")
	}
	if strings.Contains(out, "## Tags") {
		t.Error("empty brief should not include ## Tags")
	}
	if strings.Contains(out, "## Frei") {
		t.Error("empty brief should not include ## Frei")
	}
}

func TestWriteBrief_TickAppearsOnHit(t *testing.T) {
	day := time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local)
	records := []domain.DayRecord{{
		Date: day, Total: 8 * time.Hour, Target: 8 * time.Hour,
	}}
	in := domain.BriefInputs{
		Title:   "T",
		Records: records,
		Stats:   domain.Stats{ByTag: map[string]time.Duration{}, CountByTag: map[string]int{}},
	}
	var buf bytes.Buffer
	if err := domain.WriteBrief(&buf, in); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "✓") {
		t.Errorf("hit day should render ✓:\n%s", buf.String())
	}
}

func TestWriteBrief_DayOffWithoutLabelOmitsSeparator(t *testing.T) {
	in := domain.BriefInputs{
		Title: "T",
		Stats: domain.Stats{},
		DayOffs: []domain.DayOff{
			{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: ""},
		},
	}
	var buf bytes.Buffer
	if err := domain.WriteBrief(&buf, in); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Should end with "Feiertag" and a newline — no trailing " · " separator.
	if !strings.Contains(out, "Feiertag\n") {
		t.Errorf("dayoff with empty label should render plain Feiertag, got:\n%s", out)
	}
	if strings.Contains(out, "Feiertag  ·  ") {
		t.Errorf("empty label should not produce ' · ' separator")
	}
}

// errWriter triggers the io.Writer error branch in WriteBrief.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrShortWrite }

func TestWriteBrief_TitleWriteFailurePropagates(t *testing.T) {
	in := domain.BriefInputs{Title: "T"}
	if err := domain.WriteBrief(errWriter{}, in); err == nil {
		t.Error("expected error from failing writer, got nil")
	}
}
