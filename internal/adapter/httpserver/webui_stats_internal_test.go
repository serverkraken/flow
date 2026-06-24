package httpserver

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBurndownBannerVM_PaceMath(t *testing.T) {
	rep := domain.MonthBurndownReport{
		Total:   78 * time.Hour,
		Target:  160 * time.Hour,
		Saldo:   2 * time.Hour, // expected-by-now = 78 − 2 = 76h
		OnTrack: true,
	}
	vm := burndownBannerVM(rep)
	if vm.Pct != 48 { // 78/160 = 48.75 → int 48
		t.Errorf("Pct: want 48, got %d", vm.Pct)
	}
	if vm.PacePct != 47 { // 76/160 = 47.5 → int 47
		t.Errorf("PacePct: want 47, got %d", vm.PacePct)
	}
	if vm.Variant != "hit" {
		t.Errorf("Variant: want hit, got %q", vm.Variant)
	}
}

func TestBurndownBannerVM_Behind_ZeroTarget(t *testing.T) {
	behind := burndownBannerVM(domain.MonthBurndownReport{
		Total: 40 * time.Hour, Target: 160 * time.Hour, Saldo: -36 * time.Hour, OnTrack: false,
	})
	if behind.Variant != "under" {
		t.Errorf("Variant: want under, got %q", behind.Variant)
	}
	if behind.PacePct != 47 { // expected = 40 − (−36) = 76h → 47
		t.Errorf("PacePct: want 47, got %d", behind.PacePct)
	}
	zero := burndownBannerVM(domain.MonthBurndownReport{Total: 10 * time.Hour, Target: 0})
	if zero.Pct != 0 || zero.PacePct != 0 {
		t.Errorf("zero target: want 0/0, got %d/%d", zero.Pct, zero.PacePct)
	}
}
