package usecase_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkReporter(now time.Time, sessions []domain.Session, opts ...readerOpt) *usecase.Reporter {
	reader := mkReader(now, sessions, opts...)
	stats := &usecase.StatsComputer{
		Reader:  reader,
		Targets: reader.Targets,
		DayOffs: reader.Targets.DayOffs,
		State:   reader.State,
	}
	return &usecase.Reporter{
		Reader:  reader,
		DayOffs: reader.Targets.DayOffs,
		Targets: reader.Targets,
		Stats:   stats,
		Clock:   reader.Clock,
	}
}

func TestReporter_WriteBrief(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local) // Wednesday
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(8 * time.Hour), Elapsed: 8 * time.Hour, Tag: "deep"},
	}
	r := mkReporter(now, sessions)
	var buf bytes.Buffer
	if err := r.WriteBrief(&buf, now, domain.ReportWeek); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "## Übersicht") {
		t.Errorf("expected Übersicht section, got %q", out)
	}
	if !strings.Contains(out, "deep") {
		t.Errorf("expected tag deep, got %q", out)
	}
}

func TestReporter_WriteBrief_LoadErrPropagates(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReporter(now, nil)
	r.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if err := r.WriteBrief(&bytes.Buffer{}, now, domain.ReportWeek); err == nil {
		t.Error("expected error")
	}
}

func TestReporter_WriteCSV(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(8 * time.Hour), Elapsed: 8 * time.Hour, Tag: "deep"},
	}
	r := mkReporter(now, sessions)
	var buf bytes.Buffer
	if err := r.WriteCSV(&buf, domain.Range{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "date,start,stop,elapsed_seconds,tag,note") {
		t.Errorf("missing CSV header in %q", out)
	}
	if !strings.Contains(out, "2026-04-27") {
		t.Errorf("missing session row in %q", out)
	}
}

func TestReporter_WriteCSV_LoadErrPropagates(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReporter(now, nil)
	r.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if err := r.WriteCSV(&bytes.Buffer{}, domain.Range{}); err == nil {
		t.Error("expected error")
	}
}

func TestReporter_WriteJSON(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	mon := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.Local), Start: mon, Stop: mon.Add(time.Hour), Elapsed: time.Hour},
	}
	r := mkReporter(now, sessions)
	var buf bytes.Buffer
	if err := r.WriteJSON(&buf, domain.Range{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "[") {
		t.Errorf("expected JSON array, got %q", buf.String())
	}
}

func TestReporter_WriteJSON_LoadErrPropagates(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReporter(now, nil)
	r.Reader.Sessions = &testutil.FakeSessionStore{Err: errors.New("boom")}
	if err := r.WriteJSON(&bytes.Buffer{}, domain.Range{}); err == nil {
		t.Error("expected error")
	}
}

func TestReporter_WriteICS(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	r := mkReporter(now, nil)
	d := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	if err := r.DayOffs.Add(domain.DayOff{Date: d, Kind: domain.KindHoliday, Label: "Tag der Arbeit"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	if err := r.WriteICS(&buf, from, to); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Tag der Arbeit") {
		t.Errorf("expected dayoff in ICS output, got %q", buf.String())
	}
}
