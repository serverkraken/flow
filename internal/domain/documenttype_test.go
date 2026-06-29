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

func TestDocumentType_SpecAndActiveContextValid(t *testing.T) {
	for _, ty := range []DocumentType{DocSpec, DocActiveContext} {
		d := Document{Type: ty, Path: "x"}
		if err := d.Validate(); err != nil {
			t.Errorf("type %q should be valid: %v", ty, err)
		}
		if ty.HumanOwned() {
			t.Errorf("type %q must be agent-owned, not human-owned", ty)
		}
	}
}

func TestDocumentType_AgentStillValid(t *testing.T) {
	// B3d-6: agent is deprecated but must remain valid so unconverted rows load.
	if err := (Document{Type: DocAgent, Path: "x"}).Validate(); err != nil {
		t.Errorf("DocAgent must stay valid during B3d: %v", err)
	}
}

func TestSlugOK_AllowsUnderscores(t *testing.T) {
	for _, s := range []string{"feedback_no_icons", "project_flow_rebuild_m1a", "active-context", "2026-06-23-flow-webui-overhaul-design"} {
		if !SlugOK(s) {
			t.Errorf("SlugOK(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"Bad Upper", "trailing_", "/lead", "a//b"} {
		if SlugOK(s) {
			t.Errorf("SlugOK(%q) = true, want false", s)
		}
	}
}
