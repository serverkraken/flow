package webui

// Render tests for the node create/edit form's Move field (Task 7 Step 5):
// Move used to live on the cockpit page's Struktur tab (cockpitMoveForm,
// now deleted along with the tab strip) — it moved to the edit page, driven
// by the new NodeFormData.MoveTargets field.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNodeForm_EditModeRendersMoveForm verifies that NodeForm in edit mode
// (editing != nil) with MoveTargets filled renders the Move form — action
// posting to the existing POST /nodes/{id}/move handler, full-page
// (hx-boost="false"), with one <option> per candidate target plus the
// always-offered root option.
func TestNodeForm_EditModeRendersMoveForm(t *testing.T) {
	ctx := context.Background()
	editing := &domain.Node{ID: "c1", Name: "flow-api", Kind: domain.KindRepo, ParentID: ptr("v1")}
	d := NodeFormData{
		User: "u1",
		Vals: NodeFormValues{Name: "flow-api", Kind: "repo", ParentID: "v1", Status: "active"},
		MoveTargets: []domain.Node{
			{ID: "v1", Name: "Redesign", Kind: domain.KindVorhaben},
			{ID: "e2", Name: "Acme", Kind: domain.KindEngagement},
		},
	}
	out := renderToBuf(t, ctx, NodeForm(d, editing))

	if !strings.Contains(out, `action="/nodes/c1/move"`) {
		t.Fatalf("edit page missing the Move form action /nodes/c1/move:\n%.2000s", out)
	}
	if !strings.Contains(out, `hx-boost="false"`) {
		t.Fatalf("Move form must be a full-page submit (hx-boost=false):\n%.2000s", out)
	}
	for _, want := range []string{`value="v1"`, "Redesign", `value="e2"`, "Acme"} {
		if !strings.Contains(out, want) {
			t.Errorf("Move form missing target option %q:\n%.2000s", want, out)
		}
	}
	if !strings.Contains(out, `value="v1" selected`) {
		t.Errorf("Move form must preselect the node's current parent (v1):\n%.2000s", out)
	}
}

// TestNodeForm_CreateModeHasNoMoveForm verifies the Move form only appears in
// edit mode — creating a new node has no existing parent to move away from.
func TestNodeForm_CreateModeHasNoMoveForm(t *testing.T) {
	ctx := context.Background()
	d := NodeFormData{User: "u1", Vals: NodeFormValues{Kind: "engagement", Status: "active"}}
	out := renderToBuf(t, ctx, NodeForm(d, nil))
	if strings.Contains(out, "/move") {
		t.Fatalf("create-mode form must not render the Move form:\n%.2000s", out)
	}
}
