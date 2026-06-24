package webui

import "testing"

func TestStatsSaldoHue(t *testing.T) {
	if got := statsSaldoHue(true); got != "text-green" {
		t.Errorf("statsSaldoHue(true) = %q, want text-green", got)
	}
	if got := statsSaldoHue(false); got != "text-red" {
		t.Errorf("statsSaldoHue(false) = %q, want text-red", got)
	}
}
