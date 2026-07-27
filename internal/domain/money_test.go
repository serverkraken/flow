package domain_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestMoney_Mul(t *testing.T) {
	rate := domain.Money{Amount: 8000, Currency: "EUR"} // 80.00 EUR/h
	// 2h30m → 2.5h → 200.00 EUR → 20000 minor units
	got := rate.Mul(2*time.Hour + 30*time.Minute)
	if got.Amount != 20000 || got.Currency != "EUR" {
		t.Errorf("Mul: got %+v want {20000 EUR}", got)
	}
	// rounding: 1h20m = 1.3333h * 8000 = 10666.67 → 10667
	if g := rate.Mul(time.Hour + 20*time.Minute); g.Amount != 10667 {
		t.Errorf("rounding: got %d want 10667", g.Amount)
	}
}

func TestMoney_String(t *testing.T) {
	if s := (domain.Money{Amount: 480000, Currency: "EUR"}).String(); s != "4800.00 EUR" {
		t.Errorf("String: got %q want \"4800.00 EUR\"", s)
	}
}
