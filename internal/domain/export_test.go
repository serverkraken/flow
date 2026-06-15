package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func sampleExport() domain.ExportData {
	d := func(h, m int) time.Time { return time.Date(2026, 6, 15, h, m, 0, 0, time.UTC) }
	rate := domain.Money{Amount: 8000, Currency: "EUR"}
	amt := rate.Mul(2 * time.Hour)
	return domain.ExportData{
		From: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		ByProject: []domain.ProjectTotal{
			{ProjectID: "p1", ProjectName: "Acme", Total: 2 * time.Hour, SessionCount: 1, Rate: &rate, Amount: &amt},
			{ProjectID: "p2", ProjectName: "Beta", Total: 30 * time.Minute, SessionCount: 1},
		},
		Sessions: []domain.ExportRow{
			{Date: d(9, 0), Start: d(9, 0), Stop: d(11, 0), Elapsed: 2 * time.Hour, ProjectName: "Acme", Tag: "deep", Note: "x"},
			{Date: d(13, 0), Start: d(13, 0), Stop: d(13, 30), Elapsed: 30 * time.Minute, ProjectName: "Beta"},
		},
	}
}

func TestWriteCSV(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteCSV(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.HasPrefix(out, "date,start,stop,duration_seconds,project,tag,note\n") {
		t.Errorf("header missing: %q", out)
	}
	if !strings.Contains(out, "2026-06-15,09:00,11:00,7200,Acme,deep,x") {
		t.Errorf("detail row missing: %q", out)
	}
}

func TestWriteJSON(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteJSON(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`"project": "Acme"`, `"totalSeconds": 7200`, `"amountMinor": 16000`, `"sessions"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json missing %q in %s", want, out)
		}
	}
}

func TestWriteMarkdown(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteMarkdown(&b, sampleExport()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"# Worktime", "Acme", "2h 00m", "160.00 EUR", "## Sessions"} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q in %s", want, out)
		}
	}
}

func TestWriteCSV_Empty(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteCSV(&b, domain.ExportData{}); err != nil {
		t.Fatal(err)
	}
	if b.String() != "date,start,stop,duration_seconds,project,tag,note\n" {
		t.Errorf("empty CSV should be header only, got %q", b.String())
	}
}

func TestWriteMarkdown_Empty(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteMarkdown(&b, domain.ExportData{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	// Renders a valid skeleton: heading, Projekte + Sessions sections, zero grand total.
	for _, want := range []string{"# Worktime", "## Projekte", "**Summe:** 0h 00m", "## Sessions"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty markdown missing %q in %s", want, out)
		}
	}
}

func TestWriteMarkdown_EscapesPipes(t *testing.T) {
	var b strings.Builder
	d := domain.ExportData{
		From:      time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		To:        time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		ByProject: []domain.ProjectTotal{{ProjectID: "p", ProjectName: "A|B", Total: time.Hour, SessionCount: 1}},
		Sessions: []domain.ExportRow{{
			Date: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
			Stop: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC), Elapsed: time.Hour, ProjectName: "A|B", Note: "x|y",
		}},
	}
	if err := domain.WriteMarkdown(&b, d); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "A|B") || strings.Contains(out, "x|y") {
		t.Errorf("unescaped pipe leaked into markdown: %s", out)
	}
	if !strings.Contains(out, "A\\|B") {
		t.Errorf("escaped pipe missing: %s", out)
	}
}

func TestWriteJSON_EmptyArrays(t *testing.T) {
	var b strings.Builder
	if err := domain.WriteJSON(&b, domain.ExportData{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "null") {
		t.Errorf("empty export should use [] not null: %s", out)
	}
	for _, want := range []string{`"byProject": []`, `"sessions": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}
