package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestRequireType(t *testing.T) {
	if got, err := requireType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("requireType(memory) = (%q,%v), want (memory,nil)", got, err)
	}
	if _, err := requireType(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("requireType(\"\") err = %v, want a 'required' error", err)
	}
	if _, err := requireType("bogus"); err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("requireType(bogus) err = %v, want it to list valid types", err)
	}
}

func TestPatchMarkdownSectionsAndCheckboxes(t *testing.T) {
	base := "# Review\n\n## Checklist\n\n- [ ] F40 context\n- [x] F39 done\n\n## Notes\n\nkeep\n"

	replaced, err := patchMarkdown(base, patchDocIn{Operation: "replace_section", Section: "Notes", Body: "replacement"})
	if err != nil || replaced != "# Review\n\n## Checklist\n\n- [ ] F40 context\n- [x] F39 done\n\n## Notes\n\nreplacement\n" {
		t.Fatalf("replace section = %q, %v", replaced, err)
	}

	appended, err := patchMarkdown(base, patchDocIn{Operation: "append_section", Section: "Checklist", Body: "- [ ] F41 scope"})
	if err != nil || !strings.Contains(appended, "- [x] F39 done\n\n- [ ] F41 scope\n\n## Notes") {
		t.Fatalf("append section = %q, %v", appended, err)
	}

	checked := true
	newLabel := "F40 — Behoben: context is CAS-safe"
	toggled, err := patchMarkdown(base, patchDocIn{Operation: "set_checkbox", Checkbox: "F40 context", Checked: &checked, Label: &newLabel})
	if err != nil || !strings.Contains(toggled, "- [x] F40 — Behoben: context is CAS-safe") || strings.Contains(toggled, "F40 context") {
		t.Fatalf("set checkbox = %q, %v", toggled, err)
	}
}

func TestPatchMarkdownRejectsAmbiguousOrMissingTargets(t *testing.T) {
	checked := true
	if _, err := patchMarkdown("## A\n\n## A\n", patchDocIn{Operation: "replace_section", Section: "A", Body: "x"}); err == nil {
		t.Fatal("duplicate section should be rejected")
	}
	if _, err := patchMarkdown("- [ ] other\n", patchDocIn{Operation: "set_checkbox", Checkbox: "missing", Checked: &checked}); err == nil {
		t.Fatal("missing checkbox should be rejected")
	}
	empty := "  "
	if _, err := patchMarkdown("- [ ] other\n", patchDocIn{Operation: "set_checkbox", Checkbox: "other", Checked: &checked, Label: &empty}); err == nil {
		t.Fatal("empty replacement checkbox label should be rejected")
	}
	if _, err := patchMarkdown("body", patchDocIn{Operation: "bogus"}); err == nil {
		t.Fatal("unknown operation should be rejected")
	}
}

func TestGuardMutation(t *testing.T) {
	human := domain.Document{ID: "d1", Type: domain.DocFree}
	agent := domain.Document{ID: "d2", Type: domain.DocMemory}
	if err := guardMutation(agent, false); err != nil {
		t.Fatalf("agent-owned without confirm should pass, got %v", err)
	}
	if err := guardMutation(human, true); err != nil {
		t.Fatalf("human-owned WITH confirm should pass, got %v", err)
	}
	err := guardMutation(human, false)
	if err == nil || !strings.Contains(err.Error(), "confirm") || !strings.Contains(err.Error(), "free") {
		t.Fatalf("human-owned without confirm = %v, want an error naming confirm + the type", err)
	}
}
