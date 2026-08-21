package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

func TestBuildHistorieMonate(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	var months []domain.MonthLedger
	for m := time.January; m <= time.December; m++ {
		first := time.Date(2026, m, 1, 0, 0, 0, 0, time.Local)
		ml := domain.MonthLedger{Month: first, Target: 160 * time.Hour, Current: m == time.August, Future: first.After(now)}
		switch m {
		case time.July:
			ml.Logged = 169*time.Hour + 15*time.Minute
		case time.June:
			ml.Logged = 150 * time.Hour
		case time.August:
			ml.Logged = 68 * time.Hour
		}
		months = append(months, ml)
	}
	docs := []domain.Document{{CreatedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.Local)}, {CreatedAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.Local)}, {CreatedAt: time.Date(2025, 7, 9, 0, 0, 0, 0, time.Local)}}
	vm := BuildHistorieMonate(ctx, 2026, months, docs, now)
	if len(vm.Rows) != 12 || vm.NextHref != "" || vm.PrevHref != "/historie?view=monate&jahr=2025" {
		t.Fatalf("vm = %+v", vm)
	}
	jul, jun, aug, dec := vm.Rows[6], vm.Rows[5], vm.Rows[7], vm.Rows[11]
	if jul.Diff != "+9:15" || jul.DiffTone != "text-live" || jul.Cards != 2 || jul.Label != "Juli" {
		t.Errorf("Juli: %+v", jul)
	}
	if jun.Diff != "−10:00" || jun.DiffTone != "text-red" {
		t.Errorf("Juni: %+v", jun)
	}
	if !aug.Current || aug.Diff != "—" || aug.Logged != "68:00" {
		t.Errorf("August läuft ohne Differenz: %+v", aug)
	}
	if !dec.Future || dec.Logged != "—" || dec.Target != "160:00" {
		t.Errorf("Dezember: %+v", dec)
	}
	if !strings.HasPrefix(jul.Href, "/historie?view=cal&cal=month&week=2026-07-01") {
		t.Errorf("Zeile führt in den Monat: %s", jul.Href)
	}
	// Summe nur über begonnene Monate: Jan–Aug Soll 8×160, erfasst 387:15.
	if vm.TotalLogged != "387:15" || vm.TotalTarget != "1280:00" || vm.TotalCards != 2 {
		t.Errorf("Summe: %s / %s / %d", vm.TotalLogged, vm.TotalTarget, vm.TotalCards)
	}
}
