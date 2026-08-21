package webui

import "testing"

func TestBuildFreiSummary(t *testing.T) {
	rows := []FreiRowVM{
		{Day: "2026-03-02", KindLabel: "Urlaub", IsHoliday: false},
		{Day: "2026-03-03", KindLabel: "Urlaub", IsHoliday: false},
		{Day: "2026-05-01", KindLabel: "Feiertag", Label: "Tag der Arbeit", IsHoliday: true},
		{Day: "2026-09-10", KindLabel: "Krank", IsHoliday: false},
		{Day: "2026-10-03", KindLabel: "Feiertag", Label: "Einheit", IsHoliday: true},
		{Day: "2026-12-25", KindLabel: "Feiertag", Label: "Weihnachten", IsHoliday: true},
	}
	own, kinds, next := BuildFreiSummary(rows, "2026-08-21")
	if own != 3 || len(kinds) != 2 || kinds[0].Label != "Urlaub" || kinds[0].Count != 2 || kinds[1].Count != 1 {
		t.Errorf("own=%d kinds=%+v", own, kinds)
	}
	if len(next) != 2 || next[0].Label != "Einheit" {
		t.Errorf("nächste Feiertage ab heute: %+v", next)
	}
}
