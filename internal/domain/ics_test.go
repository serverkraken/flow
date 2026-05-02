package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestWriteICS_BasicShape(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	d1 := domain.DayOff{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Tag der Arbeit"}
	d2 := domain.DayOff{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.Local), Kind: domain.KindVacation, Label: "Brückentag"}

	var b strings.Builder
	if err := domain.WriteICS(&b, []domain.DayOff{d1, d2}, now); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"PRODID:-//serverkraken//flow worktime//DE\r\n",
		"BEGIN:VEVENT\r\n",
		"DTSTART;VALUE=DATE:20260501\r\n",
		"DTEND;VALUE=DATE:20260502\r\n", // exclusive
		"DTSTART;VALUE=DATE:20260508\r\n",
		"DTEND;VALUE=DATE:20260509\r\n",
		"SUMMARY:Tag der Arbeit (Feiertag)\r\n",
		"SUMMARY:Brückentag (Urlaub)\r\n",
		"CATEGORIES:Feiertag\r\n",
		"TRANSP:TRANSPARENT\r\n",
		"END:VEVENT\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ics output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestWriteICS_SortsByDate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	// Out-of-order input.
	d1 := domain.DayOff{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.Local), Kind: domain.KindVacation, Label: "Later"}
	d2 := domain.DayOff{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local), Kind: domain.KindHoliday, Label: "Earlier"}

	var b strings.Builder
	if err := domain.WriteICS(&b, []domain.DayOff{d1, d2}, now); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	earlier := strings.Index(out, "Earlier")
	later := strings.Index(out, "Later")
	if earlier < 0 || later < 0 || earlier > later {
		t.Errorf("expected Earlier (5/1) before Later (5/8), got Earlier=%d Later=%d", earlier, later)
	}
}

func TestWriteICS_EmptyEmitsEnvelope(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	var b strings.Builder
	if err := domain.WriteICS(&b, nil, now); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "BEGIN:VCALENDAR") || !strings.Contains(out, "END:VCALENDAR") {
		t.Errorf("empty export should still emit envelope, got:\n%s", out)
	}
	if strings.Contains(out, "BEGIN:VEVENT") {
		t.Errorf("empty input should not emit any VEVENTs, got:\n%s", out)
	}
}

func TestWriteICS_EscapesLabel(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	d := domain.DayOff{
		Date:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local),
		Kind:  domain.KindVacation,
		Label: "Urlaub, Ausland; Spanien",
	}
	var b strings.Builder
	if err := domain.WriteICS(&b, []domain.DayOff{d}, now); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `Urlaub\, Ausland\; Spanien`) {
		t.Errorf("comma + semicolon should be escaped:\n%s", out)
	}
}

func TestWriteICS_LabelOmittedFallsBackToKindLabel(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	d := domain.DayOff{
		Date:  time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local),
		Kind:  domain.KindSick,
		Label: "",
	}
	var b strings.Builder
	if err := domain.WriteICS(&b, []domain.DayOff{d}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "SUMMARY:Krank\r\n") {
		t.Errorf("empty label should fall back to kind label, got:\n%s", b.String())
	}
}

func TestWriteICS_StampIsUTC(t *testing.T) {
	// "now" in a non-UTC timezone — stamp must still be UTC.
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no Europe/Berlin tz data")
	}
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, loc) // CEST = UTC+2
	d := domain.DayOff{Date: time.Date(2026, 6, 15, 0, 0, 0, 0, loc), Kind: domain.KindHoliday, Label: "Test"}

	var b strings.Builder
	if err := domain.WriteICS(&b, []domain.DayOff{d}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "DTSTAMP:20260615T120000Z\r\n") {
		t.Errorf("DTSTAMP should be UTC (12:00Z, was 14:00 CEST), got:\n%s", b.String())
	}
}

type icsErrWriter struct{}

func (icsErrWriter) Write(_ []byte) (int, error) { return 0, errFailed }

var errFailed = errStatic("failed")

type errStatic string

func (e errStatic) Error() string { return string(e) }

func TestWriteICS_WriterErrorPropagates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if err := domain.WriteICS(icsErrWriter{}, nil, now); err == nil {
		t.Error("expected writer error, got nil")
	}
}
