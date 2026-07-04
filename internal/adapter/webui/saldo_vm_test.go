package webui

import "testing"

func TestStatsSaldoHue(t *testing.T) {
	if got := statsSaldoHue(true); got != "green" {
		t.Errorf("statsSaldoHue(true) = %q, want green", got)
	}
	if got := statsSaldoHue(false); got != "red" {
		t.Errorf("statsSaldoHue(false) = %q, want red", got)
	}
}
