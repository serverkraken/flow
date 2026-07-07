package httpserver

import (
	"net/url"
	"testing"
	"time"
)

func TestParseWeekdayTargets(t *testing.T) {
	form := url.Values{
		"mon": {"480"},
		"tue": {""}, // empty → omitted (inherit default)
		"fri": {"240"},
	}
	got, err := parseWeekdayTargets(form)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 keys, got %d (%v)", len(got), got)
	}
	if got[time.Monday] != 480 || got[time.Friday] != 240 {
		t.Errorf("want Mon=480 Fri=240, got %v", got)
	}
	if _, ok := got[time.Tuesday]; ok {
		t.Errorf("empty Tuesday should be omitted, got %v", got)
	}
}

func TestParseWeekdayTargets_Invalid(t *testing.T) {
	if _, err := parseWeekdayTargets(url.Values{"wed": {"-5"}}); err == nil {
		t.Error("negative value should error")
	}
	if _, err := parseWeekdayTargets(url.Values{"thu": {"abc"}}); err == nil {
		t.Error("non-numeric value should error")
	}
}
