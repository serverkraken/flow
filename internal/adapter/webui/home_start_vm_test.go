package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

func TestGreetingAndTargetLine(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	morning := time.Date(2026, 8, 21, 8, 0, 0, 0, time.Local)
	if got := Greeting(ctx, morning, "Martin Söntgenrath"); got != "Guten Morgen, Martin" {
		t.Errorf("Greeting = %q", got)
	}
	if got := Greeting(ctx, morning.Add(12*time.Hour), ""); got != "Guten Abend" {
		t.Errorf("Greeting ohne Namen = %q", got)
	}
	if got := TargetLine(ctx, 6*time.Hour+12*time.Minute, 8*time.Hour); got != "Ziel 8:00 h · noch 1:48 h" {
		t.Errorf("TargetLine = %q", got)
	}
	if got := TargetLine(ctx, 9*time.Hour, 8*time.Hour); got != "Ziel erreicht" {
		t.Errorf("TargetLine erreicht = %q", got)
	}
	if got := TargetLine(ctx, time.Hour, 0); got != "" {
		t.Errorf("ohne Ziel keine Zeile: %q", got)
	}
}

func TestBuildAttention(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	eng := []domain.Node{{ID: "a", Name: "serverkraken/Lesesaal", Kind: domain.KindEngagement}, {ID: "b", Name: "blupod", Kind: domain.KindEngagement}, {ID: "c", Name: "ruhig", Kind: domain.KindEngagement}}
	nid := "x"
	docs := []domain.Document{
		{ID: "i1", Type: domain.DocInstruction, Title: "Arbeitsregeln", NodeID: &nid, UpdatedAt: now.AddDate(0, 0, -41)},
		{ID: "i2", Type: domain.DocInstruction, Title: "frisch", NodeID: &nid, UpdatedAt: now.AddDate(0, 0, -2)},
		{ID: "f1", Type: domain.DocFree, Title: "lose"},
		{ID: "f2", Type: domain.DocFree, Title: "lose 2"},
		{ID: "d1", Type: domain.DocDaily, Title: "heute"},
	}
	rows := BuildAttention(ctx, AttentionInput{
		Engagements:    eng,
		ContextDropped: map[string]int{"a": 1},
		LastBooking:    map[string]time.Time{"a": now.Add(-time.Hour), "b": now.AddDate(0, 0, -4)},
		Docs:           docs, Now: now,
	})
	kinds := make([]string, 0, len(rows))
	for _, r := range rows {
		kinds = append(kinds, r.Kind)
	}
	if got := strings.Join(kinds, ","); got != "context,idle,stale,unfiled" {
		t.Fatalf("Reihenfolge = %s (%+v)", got, rows)
	}
	if rows[0].Note != "Lesesaal · 1 Karte fällt raus" || rows[0].Href != "/kontext/a" {
		t.Errorf("context: %+v", rows[0])
	}
	if rows[1].Title != "blupod ohne Zeitbuchung" || rows[1].Note != "seit 4 Tagen nichts erfasst" {
		t.Errorf("idle: %+v", rows[1])
	}
	if !strings.HasPrefix(rows[2].Note, "Arbeitsregeln · 41 Tage") {
		t.Errorf("stale: %+v", rows[2])
	}
	if rows[3].Note != "2 im Eingang · zuordnen" {
		t.Errorf("unfiled zählt keine Tagesnotizen: %+v", rows[3])
	}
}

func TestBuildBestandAndErtraege(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	nodes := []domain.Node{
		{ID: "e", Kind: domain.KindEngagement}, {ID: "v", Kind: domain.KindVorhaben}, {ID: "r", Kind: domain.KindRepo},
		{ID: "old", Kind: domain.KindRepo, Status: domain.NodeArchived},
	}
	b := BuildBestand(nodes, []domain.Document{{ID: "1"}, {ID: "2", Archived: true}}, 3)
	if b.Engagements != 1 || b.Vorhaben != 1 || b.Repos != 1 || b.CardsActive != 1 || b.CardsArchived != 3 || b.CardsTotal != 4 {
		t.Errorf("Bestand = %+v", b)
	}
	rate := &domain.Money{Amount: 9500, Currency: "EUR"}
	e := BuildErtraege(ctx, "August", []EngagementMonth{
		{Node: domain.Node{ID: "k", Name: "Kunde"}, Month: 10 * time.Hour, Rate: rate},
		{Node: domain.Node{ID: "p", Name: "Privat"}, Month: 8 * time.Hour},
		{Node: domain.Node{ID: "z", Name: "Leer"}, Month: 0},
	})
	if e == nil || len(e.Rows) != 2 || e.Rows[0].Name != "Kunde" || e.Rows[0].Amount != "950.00 EUR" || e.Rows[1].Amount != "kein Satz" {
		t.Fatalf("Erträge = %+v", e)
	}
	if e.TotalStr != "950.00 EUR" || e.NoRateHours != "8:00 h ohne Satz" {
		t.Errorf("Summen: %q / %q", e.TotalStr, e.NoRateHours)
	}
	if BuildErtraege(ctx, "August", nil) != nil {
		t.Errorf("ohne Buchung kein Block")
	}
}

func TestFindYesterdayNote(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	now := time.Date(2026, 8, 21, 9, 0, 0, 0, time.Local)
	y := now.AddDate(0, 0, -1)
	docs := []domain.Document{{ID: "d", Type: domain.DocDaily, Date: &y, Body: "# 2026-08-20\n\nAbends die Token notiert."}}
	n := FindYesterdayNote(ctx, docs, now)
	if n == nil || n.ID != "d" || !strings.Contains(n.Excerpt, "Token notiert") {
		t.Errorf("gestern: %+v", n)
	}
	if FindYesterdayNote(ctx, nil, now) != nil {
		t.Errorf("ohne Notiz nil")
	}
}
