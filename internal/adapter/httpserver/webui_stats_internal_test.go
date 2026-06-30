package httpserver

import (
	"net/url"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBurndownBannerVM_PaceMath(t *testing.T) {
	rep := domain.MonthBurndownReport{
		Total:       78 * time.Hour,
		TargetTotal: 78 * time.Hour,
		Target:      160 * time.Hour,
		Saldo:       2 * time.Hour, // expected-by-now = TargetTotal − Saldo = 78 − 2 = 76h
		OnTrack:     true,
	}
	vm := burndownBannerVM(rep)
	if vm.Pct != 48 { // TargetTotal/Target = 78/160 = 48.75 → int 48
		t.Errorf("Pct: want 48, got %d", vm.Pct)
	}
	if vm.PacePct != 47 { // 76/160 = 47.5 → int 47
		t.Errorf("PacePct: want 47, got %d", vm.PacePct)
	}
	if vm.Variant != "hit" {
		t.Errorf("Variant: want hit, got %q", vm.Variant)
	}
}

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

func TestBurndownBannerVM_Behind_ZeroTarget(t *testing.T) {
	behind := burndownBannerVM(domain.MonthBurndownReport{
		Total: 40 * time.Hour, TargetTotal: 40 * time.Hour, Target: 160 * time.Hour, Saldo: -36 * time.Hour, OnTrack: false,
	})
	if behind.Variant != "under" {
		t.Errorf("Variant: want under, got %q", behind.Variant)
	}
	if behind.PacePct != 47 { // expected = TargetTotal − Saldo = 40 − (−36) = 76h → 47
		t.Errorf("PacePct: want 47, got %d", behind.PacePct)
	}
	zero := burndownBannerVM(domain.MonthBurndownReport{Total: 10 * time.Hour, Target: 0})
	if zero.Pct != 0 || zero.PacePct != 0 {
		t.Errorf("zero target: want 0/0, got %d/%d", zero.Pct, zero.PacePct)
	}
}

// TestBurndownBannerVM_PrivateTimeExcluded is the RED→GREEN guard for I1:
// when Total > TargetTotal (private/non-counting time exists), the pace marker
// and fill percentage must be based on TargetTotal (job-Soll), not Total.
func TestBurndownBannerVM_PrivateTimeExcluded(t *testing.T) {
	// 50h logged total, but only 40h count toward the job target.
	// Month target = 100h. User is 5h ahead of the job schedule.
	// Saldo = TargetTotal − expected = 40h − 35h = +5h.
	rep := domain.MonthBurndownReport{
		Total:       50 * time.Hour, // includes 10h private time
		TargetTotal: 40 * time.Hour, // job-counting hours only
		Target:      100 * time.Hour,
		Saldo:       5 * time.Hour, // TargetTotal(40) − expected(35) = +5h
		OnTrack:     true,
	}
	vm := burndownBannerVM(rep)
	// Fill: TargetTotal/Target = 40/100 = 40 (NOT 50 from Total).
	if vm.Pct != 40 {
		t.Errorf("Pct: want 40 (job-scoped), got %d — private time must not inflate fill", vm.Pct)
	}
	// Pace: expected = TargetTotal − Saldo = 40 − 5 = 35h; 35/100 = 35 (NOT 45 from Total-based math).
	if vm.PacePct != 35 {
		t.Errorf("PacePct: want 35 (job-scoped), got %d — private time must not inflate pace marker", vm.PacePct)
	}
	if vm.Variant != "hit" {
		t.Errorf("Variant: want hit (OnTrack=true), got %q", vm.Variant)
	}
}
