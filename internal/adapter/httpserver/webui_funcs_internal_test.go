package httpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestProjectHue covers projectHue's three branches:
// nil id → "", id found → color, id not found → "".
func TestProjectHue(t *testing.T) {
	projects := []domain.Project{
		{ID: "p1", Color: "blue"},
		{ID: "p2", Color: "green"},
	}
	p1 := "p1"
	p99 := "p99"

	// nil id → ""
	if got := projectHue(projects, nil); got != "" {
		t.Errorf("projectHue(nil) = %q, want \"\"", got)
	}
	// found → color
	if got := projectHue(projects, &p1); got != "blue" {
		t.Errorf("projectHue(p1) = %q, want \"blue\"", got)
	}
	// not found → ""
	if got := projectHue(projects, &p99); got != "" {
		t.Errorf("projectHue(unknown) = %q, want \"\"", got)
	}
}

// TestHistorieWeekParam covers both branches of historieWeekParam:
// isCurrent=true → "", isCurrent=false → "&week=YYYY-MM-DD".
func TestHistorieWeekParam(t *testing.T) {
	ws := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // Mon 2026-06-15
	if got := historieWeekParam(ws, true); got != "" {
		t.Errorf("historieWeekParam(current) = %q, want \"\"", got)
	}
	got := historieWeekParam(ws, false)
	if !strings.Contains(got, "2026-06-15") {
		t.Errorf("historieWeekParam(non-current) = %q, want &week=2026-06-15...", got)
	}
}

// TestOrStatus covers the orStatus helper:
// empty → "active", non-empty → passthrough.
func TestOrStatus(t *testing.T) {
	if got := orStatus(""); got != "active" {
		t.Errorf("orStatus(\"\") = %q, want \"active\"", got)
	}
	if got := orStatus("paused"); got != "paused" {
		t.Errorf("orStatus(\"paused\") = %q, want \"paused\"", got)
	}
}
