package clientmachine_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/clientmachine"
)

func TestLoadFrom_StableAcrossCalls(t *testing.T) {
	dir := t.TempDir()

	m1, err := clientmachine.LoadFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m1.ID == "" || m1.Label == "" {
		t.Fatalf("empty machine: %+v", m1)
	}

	m2, err := clientmachine.LoadFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m2.ID != m1.ID {
		t.Fatalf("id not stable: %q vs %q", m1.ID, m2.ID)
	}
	if m2.Label != m1.Label {
		t.Fatalf("label not stable: %q vs %q", m1.Label, m2.Label)
	}
}
