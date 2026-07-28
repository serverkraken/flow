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

// TestPatchMarkdownReplaceSectionSpansSubsections nagelt die Baum-Semantik fest:
// ein Abschnitt schließt seine Unterabschnitte ein. Das ist gewolltes Verhalten,
// kein Bug — der Bug war, dass es weder dokumentiert noch sichtbar war.
func TestPatchMarkdownReplaceSectionSpansSubsections(t *testing.T) {
	base := "# Doc\n\n## One\n\nintro\n\n### One A\n\ndetail a\n\n### One B\n\ndetail b\n\n## Two\n\nkeep two\n"

	got, err := patchMarkdown(base, patchDocIn{Operation: "replace_section", Section: "One", Body: "replacement"})
	if err != nil {
		t.Fatalf("replace_section on H2 with subsections: %v", err)
	}
	want := "# Doc\n\n## One\n\nreplacement\n\n## Two\n\nkeep two\n"
	if got != want {
		t.Fatalf("replace_section on H2 with subsections =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "detail a") || strings.Contains(got, "detail b") {
		t.Fatalf("subsections survived the replacement: %q", got)
	}
	if !strings.Contains(got, "keep two") {
		t.Fatalf("the following H2 section was swallowed: %q", got)
	}
}

// TestPatchMarkdownReplaceSectionOnH1SpansWholeDocument ist die Regression für
// den Verlustfall vom 2026-07-28: keine H2/H3 erfüllt gotLevel <= 1, also endet
// die H1-Sektion erst am Dateiende. Das Verhalten bleibt — der Schrumpf-Guard
// (Task 3/5) ist die Absicherung, nicht eine Änderung dieser Semantik.
func TestPatchMarkdownReplaceSectionOnH1SpansWholeDocument(t *testing.T) {
	base := "# Review\n\nintro\n\n## Findings\n\nS1 S2 S3\n\n## Method\n\nhow it was done\n"

	got, err := patchMarkdown(base, patchDocIn{Operation: "replace_section", Section: "Review", Body: "corrected intro"})
	if err != nil {
		t.Fatalf("replace_section on H1: %v", err)
	}
	want := "# Review\n\ncorrected intro\n"
	if got != want {
		t.Fatalf("replace_section on H1 =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "Findings") || strings.Contains(got, "Method") {
		t.Fatalf("chapters survived — semantics changed unexpectedly: %q", got)
	}
}

func TestNewBodyDelta(t *testing.T) {
	tests := []struct {
		name           string
		before, after  string
		wantBB, wantBA int
		wantLB, wantLA int
	}{
		{"empty after", "# A\n\nbody\n", "", 10, 0, 3, 0},
		{"empty before", "", "new\n", 0, 4, 0, 1},
		{"both empty", "", "", 0, 0, 0, 0},
		{"trailing newline counts once", "a\n", "a\nb\n", 2, 4, 1, 2},
		{"no trailing newline", "a\nb", "a", 3, 1, 2, 1},
		{"growth", "short", "much longer body", 5, 16, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newBodyDelta(tt.before, tt.after)
			want := bodyDelta{BytesBefore: tt.wantBB, BytesAfter: tt.wantBA, LinesBefore: tt.wantLB, LinesAfter: tt.wantLA}
			if got != want {
				t.Fatalf("newBodyDelta(%q, %q) = %+v, want %+v", tt.before, tt.after, got, want)
			}
		})
	}
}

func TestCheckShrink(t *testing.T) {
	// Der reale Verlustfall vom 2026-07-28: 14204 → 1021 Bytes, 312 → 25 Zeilen.
	// Nicht `real` nennen — das ist ein Go-Builtin.
	lossCase := bodyDelta{BytesBefore: 14204, BytesAfter: 1021, LinesBefore: 312, LinesAfter: 25}

	err := checkShrink("patch", lossCase, false)
	if err == nil {
		t.Fatal("the real loss case should be refused")
	}
	for _, want := range []string{"13183", "14204", "93%", "312", "25", "allowShrink", "flow_update_doc"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal %q does not mention %q", err.Error(), want)
		}
	}

	if err := checkShrink("patch", lossCase, true); err != nil {
		t.Fatalf("allowShrink=true should pass, got %v", err)
	}

	// update nennt flow_update_doc NICHT als Ausweg — das wäre zirkulär.
	err = checkShrink("update", lossCase, false)
	if err == nil || strings.Contains(err.Error(), "flow_update_doc") {
		t.Fatalf("update refusal = %v, want a refusal without the flow_update_doc hint", err)
	}
	if !strings.HasPrefix(err.Error(), "update would remove") {
		t.Fatalf("update refusal = %q, want it to start with 'update would remove'", err.Error())
	}

	passes := []struct {
		name string
		d    bodyDelta
	}{
		// Über 50 %, aber unter der absoluten Untergrenze: kleine Dokumente
		// dürfen nicht ständig anschlagen.
		{"above ratio, below floor", bodyDelta{BytesBefore: 612, BytesAfter: 280, LinesBefore: 20, LinesAfter: 9}},
		// Genau an der Untergrenze — > ist echt größer, 1024 selbst reicht nicht.
		{"exactly at the byte floor", bodyDelta{BytesBefore: 4096, BytesAfter: 3072, LinesBefore: 100, LinesAfter: 75}},
		// Genau 1024 Bytes entfernt, aber Ratio über 50% — isoliert die Byte-Grenze richtig.
		{"exactly at the byte floor, ratio above threshold", bodyDelta{BytesBefore: 2000, BytesAfter: 976, LinesBefore: 100, LinesAfter: 49}},
		// Viele Bytes entfernt, aber genau die Hälfte — > ist echt größer.
		{"exactly half removed", bodyDelta{BytesBefore: 8000, BytesAfter: 4000, LinesBefore: 200, LinesAfter: 100}},
		{"growth", bodyDelta{BytesBefore: 100, BytesAfter: 9000, LinesBefore: 3, LinesAfter: 300}},
		{"unchanged", bodyDelta{BytesBefore: 5000, BytesAfter: 5000, LinesBefore: 120, LinesAfter: 120}},
		{"empty before", bodyDelta{BytesBefore: 0, BytesAfter: 0, LinesBefore: 0, LinesAfter: 0}},
	}
	for _, tt := range passes {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkShrink("patch", tt.d, false); err != nil {
				t.Fatalf("checkShrink(%+v) = %v, want nil", tt.d, err)
			}
		})
	}
}
