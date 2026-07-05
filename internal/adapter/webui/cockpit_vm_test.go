package webui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func TestNodeTimer_States(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	nameOf := func(id string) string { return map[string]string{"y": "Vorhaben Y"}[id] }

	// idle (bookable, nothing running)
	if got := webui.NodeTimer(nil, "x", true, now, nameOf); got.State != webui.TimerIdle {
		t.Errorf("idle: state=%v", got.State)
	}
	// not bookable (branch)
	if got := webui.NodeTimer(nil, "x", false, now, nameOf); got.State != webui.TimerNotBookable {
		t.Errorf("notBookable: state=%v", got.State)
	}
	// running on THIS node
	yid := "x"
	run := domain.WorkSession{ID: "s1", NodeID: &yid, Start: now.Add(-90 * time.Second)}
	if got := webui.NodeTimer(&run, "x", true, now, nameOf); got.State != webui.TimerHere || got.RunningID != "s1" || got.RunningBase != 90 {
		t.Errorf("here: %+v", got)
	}
	// running on ANOTHER node (bound)
	other := "y"
	run2 := domain.WorkSession{ID: "s2", NodeID: &other, Start: now}
	g := webui.NodeTimer(&run2, "x", true, now, nameOf)
	if g.State != webui.TimerOtherBound || g.OtherID != "y" || g.OtherName != "Vorhaben Y" {
		t.Errorf("otherBound: %+v", g)
	}
	// running unbound (started from Home, no node)
	run3 := domain.WorkSession{ID: "s3", NodeID: nil, Start: now}
	if got := webui.NodeTimer(&run3, "x", true, now, nameOf); got.State != webui.TimerUnbound {
		t.Errorf("unbound: state=%v", got.State)
	}
}

func TestNormalizeTab(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "uebersicht"}, {"bogus", "uebersicht"}, {"uebersicht", "uebersicht"},
		{"worktime", "worktime"}, {"wissen", "wissen"},
		{"struktur", "struktur"}, {"bindings", "bindings"},
	} {
		if got := webui.NormalizeTab(c.in); got != c.want {
			t.Errorf("NormalizeTab(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// TestBuildWissenRows_MapsChipAndMeta pins BuildWissenRows' field mapping:
// ChipClass/ChipLabel from DocTypeChipClass/DocTypeLabel, Meta = relative
// time + path (domain.Document carries no last-editor field — verified via
// `rg "type Document struct" internal/domain/` — so Meta degrades to
// "Zeit · Pfad", not "Akteur · Zeit · Pfad").
func TestBuildWissenRows_MapsChipAndMeta(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		{ID: "d1", Title: "Token-Integration", Path: "docs/gitlab-token-integration", Type: domain.DocProject, UpdatedAt: now.Add(-90 * time.Minute)},
	}
	rows := webui.BuildWissenRows(docs, now)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != "d1" || row.Title != "Token-Integration" {
		t.Errorf("ID/Title = %q/%q, want d1/Token-Integration", row.ID, row.Title)
	}
	if row.ChipClass != webui.DocTypeChipClass(domain.DocProject) || row.ChipLabel != webui.DocTypeLabel(domain.DocProject) {
		t.Errorf("chip = %q/%q, want DocTypeChipClass/Label(DocProject)", row.ChipClass, row.ChipLabel)
	}
	if !strings.Contains(row.Meta, "docs/gitlab-token-integration") {
		t.Errorf("Meta = %q, must contain the path", row.Meta)
	}
	if !strings.Contains(row.Meta, "vor") { // fmtRelTime's German relative-time prefix
		t.Errorf("Meta = %q, must contain a relative time", row.Meta)
	}
}

// TestBuildWissenRows_ReadTime pins the reading-time estimate (word count /
// 220, rounded up to at least 1 minute) and its "" fallback for an empty Body
// (a list query without full content — no blocker, Spec §16.9).
func TestBuildWissenRows_ReadTime(t *testing.T) {
	now := time.Now()
	words := make([]string, 440) // 440/220 = 2 min exactly
	for i := range words {
		words[i] = "w"
	}
	docs := []domain.Document{
		{ID: "d1", Body: strings.Join(words, " ")},
		{ID: "d2", Body: "just a few words"},
		{ID: "d3", Body: ""},
	}
	rows := webui.BuildWissenRows(docs, now)
	if rows[0].ReadTime != "2 min" {
		t.Errorf("440 words: ReadTime = %q, want %q", rows[0].ReadTime, "2 min")
	}
	if rows[1].ReadTime != "1 min" {
		t.Errorf("short body: ReadTime = %q, want %q (rounds up to 1)", rows[1].ReadTime, "1 min")
	}
	if rows[2].ReadTime != "" {
		t.Errorf("empty body: ReadTime = %q, want \"\" (no blocker)", rows[2].ReadTime)
	}
}
