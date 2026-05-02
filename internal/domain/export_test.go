package domain_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func mkSession(date string, h, m int, dur time.Duration, tag, note string) domain.Session {
	d, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	start := d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	return domain.Session{
		Date:    d,
		Start:   start,
		Stop:    start.Add(dur),
		Elapsed: dur,
		Tag:     tag,
		Note:    note,
	}
}

func TestWriteCSV_HeaderAndRows(t *testing.T) {
	sessions := []domain.Session{
		mkSession("2026-04-27", 9, 0, 2*time.Hour, "deep", "auth"),
		mkSession("2026-04-28", 10, 30, 90*time.Minute, "", ""),
	}
	var b bytes.Buffer
	if err := domain.WriteCSV(&b, sessions); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	// Header row.
	if !strings.HasPrefix(out, "date,start,stop,elapsed_seconds,tag,note\n") {
		t.Errorf("missing header row, got:\n%s", out)
	}
	// First data row.
	if !strings.Contains(out, "2026-04-27,09:00,11:00,7200,deep,auth\n") {
		t.Errorf("missing first row, got:\n%s", out)
	}
	// Second data row (empty tag/note → trailing commas).
	if !strings.Contains(out, "2026-04-28,10:30,12:00,5400,,\n") {
		t.Errorf("missing second row, got:\n%s", out)
	}
}

func TestWriteCSV_QuotesSpecialChars(t *testing.T) {
	sessions := []domain.Session{
		mkSession("2026-04-27", 9, 0, time.Hour, "deep", `note with, comma`),
	}
	var b bytes.Buffer
	if err := domain.WriteCSV(&b, sessions); err != nil {
		t.Fatal(err)
	}
	// CSV must quote fields containing commas — encoding/csv handles this.
	if !strings.Contains(b.String(), `"note with, comma"`) {
		t.Errorf("expected quoted comma in note, got:\n%s", b.String())
	}
}

func TestWriteCSV_EmptyJustHeader(t *testing.T) {
	var b bytes.Buffer
	if err := domain.WriteCSV(&b, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != "date,start,stop,elapsed_seconds,tag,note" {
		t.Errorf("empty input should yield header only, got:\n%s", b.String())
	}
}

func TestWriteJSON_StructureAndTypes(t *testing.T) {
	sessions := []domain.Session{
		mkSession("2026-04-27", 9, 0, 2*time.Hour, "deep", "auth"),
	}
	var b bytes.Buffer
	if err := domain.WriteJSON(&b, sessions); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b.String())
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	want := map[string]any{
		"date":            "2026-04-27",
		"start":           "09:00",
		"stop":            "11:00",
		"elapsed_seconds": float64(7200), // JSON numbers come back as float64
		"tag":             "deep",
		"note":            "auth",
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("key %q = %v, want %v", k, got[0][k], v)
		}
	}
}

func TestWriteJSON_EmptyArray(t *testing.T) {
	var b bytes.Buffer
	if err := domain.WriteJSON(&b, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != "[]" {
		t.Errorf("empty input should yield empty JSON array, got:\n%s", b.String())
	}
}

type csvErrWriter struct {
	failAfter int
	count     int
}

func (e *csvErrWriter) Write(p []byte) (int, error) {
	e.count++
	if e.count > e.failAfter {
		return 0, errFailed
	}
	return len(p), nil
}

func TestWriteCSV_WriterErrorPropagates(t *testing.T) {
	// failAfter 0 → first Write fails, which is the header row.
	w := &csvErrWriter{failAfter: 0}
	sessions := []domain.Session{mkSession("2026-04-27", 9, 0, time.Hour, "", "")}
	if err := domain.WriteCSV(w, sessions); err == nil {
		t.Error("expected writer error, got nil")
	}
}

func TestWriteJSON_WriterErrorPropagates(t *testing.T) {
	w := &csvErrWriter{failAfter: 0}
	if err := domain.WriteJSON(w, nil); err == nil {
		t.Error("expected writer error, got nil")
	}
}
