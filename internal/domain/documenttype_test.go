package domain

import "testing"

func TestDocumentType_AgentOwnedValid(t *testing.T) {
	for _, ty := range []DocumentType{DocMemory, DocInstruction, DocSkill, DocPlan} {
		d := Document{Type: ty, Path: "agent/note"}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate(type=%q) = %v, want nil", ty, err)
		}
	}
}

func TestDocumentType_UnknownInvalid(t *testing.T) {
	d := Document{Type: DocumentType("bogus"), Path: "x"}
	if err := d.Validate(); err == nil {
		t.Fatal("Validate(type=bogus) = nil, want error")
	}
}
