package domain

import "testing"

func TestDocumentTypesAllValid(t *testing.T) {
	ts := DocumentTypes()
	if len(ts) != 8 {
		t.Fatalf("DocumentTypes() returned %d types, want 8", len(ts))
	}
	for _, dt := range ts {
		if !dt.valid() {
			t.Errorf("DocumentTypes() includes %q but valid() rejects it", dt)
		}
	}
	if DocumentType("bogus").valid() {
		t.Error("valid() accepted a bogus type")
	}
}
