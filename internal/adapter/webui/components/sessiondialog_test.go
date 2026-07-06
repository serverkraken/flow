package components_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// TestSessionDialogClosesOnSuccess guards carry-forward #4 (Nachbuchen dialog
// stayed open after a successful add/edit submit). The form opts into the
// generic dialog.js close-on-success behavior via the marker attribute; the
// JS wiring itself is verified live (Lesesaal L3 Gate smoke, AGENTS.md: no
// Node test harness for JS logic).
func TestSessionDialogClosesOnSuccess(t *testing.T) {
	out := render(t, components.SessionDialog(components.SessionDialogVM{
		DialogID: "session-dialog",
		Mode:     "add",
		Action:   "/nodes/n1/sessions",
		Target:   "#cockpit-main",
	}))
	if !strings.Contains(out, "data-dialog-close-on-success") {
		t.Errorf("SessionDialog form missing data-dialog-close-on-success: %s", out)
	}
}
