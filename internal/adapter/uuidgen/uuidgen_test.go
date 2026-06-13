package uuidgen

import "testing"

func TestNewIDUnique(t *testing.T) {
	g := Gen{}
	a, b := g.NewID(), g.NewID()
	if a == "" || a == b {
		t.Fatalf("bad ids: %q %q", a, b)
	}
}
