package projects_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// The form is pushed onto the nav stack, so a back-key (q OR Esc — both match
// grammar.Back) would pop it via ResolveBack → BackPop UNLESS the form reports
// CapturesText()==true, which makes ResolveBack forward the key to the form
// instead. This guards the real shell routing decision (the form unit tests
// drive Update directly and never reach ResolveBack, which is how the bug — a
// q that discarded the whole form — slipped past the per-task review).
func TestFormBackKeysStayInFormNotPopped(t *testing.T) {
	r := projects.NewFormRoute(&fakeFormAPI{}, theme.Default, nil)

	if !r.CapturesText() {
		t.Fatal("form must report CapturesText()==true")
	}
	// stackDepth=2 (form pushed over the list), no overlay open.
	if got := shell.ResolveBack(r, 2, false); got != shell.BackForward {
		t.Fatalf("ResolveBack on the pushed form = %v, want BackForward (q/Esc must reach the form, not pop it)", got)
	}
}

// End-to-end: a literal 'q' keystroke is treated as text by the focused field
// (Name), not swallowed as a back-key.
func TestFormTypesLiteralQIntoField(t *testing.T) {
	r := projects.NewFormRoute(&fakeFormAPI{}, theme.Default, nil)
	nr, _ := r.Update(keyPress('q'))
	r = nr.(*projects.FormRoute)
	if got := r.Values().Name; got != "q" {
		t.Fatalf("after typing 'q' the Name field = %q, want \"q\" (q must be literal text input)", got)
	}
}
