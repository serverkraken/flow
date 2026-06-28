package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNodeSelectOptions_HierarchyAndKind verifies the document-editor "Projekt"
// picker is no longer a flat name list: options come back hierarchy-ordered
// (engagement root before its children), depth-indented with non-breaking
// spaces, and each label carries the node's kind glyph + localized kind label.
func TestNodeSelectOptions_HierarchyAndKind(t *testing.T) {
	t.Parallel()
	p := func(s string) *string { return &s }
	eng := domain.Node{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"}
	vor := domain.Node{ID: "v1", Kind: domain.KindVorhaben, Name: "Buch", ParentID: p("e1")}
	repo := domain.Node{ID: "r1", Kind: domain.KindRepo, Name: "flow", ParentID: p("e1")}

	// Intentionally unordered input — the helper must rebuild the hierarchy.
	opts := NodeSelectOptions(context.Background(), []domain.Node{repo, eng, vor})
	if len(opts) != 3 {
		t.Fatalf("want 3 options, got %d: %+v", len(opts), opts)
	}
	if opts[0].Value != "e1" {
		t.Errorf("engagement root must come first, got value %q (label %q)", opts[0].Value, opts[0].Label)
	}
	if !strings.Contains(opts[0].Label, "◆") || !strings.Contains(opts[0].Label, "Privat") {
		t.Errorf("root label must carry kind glyph + name, got %q", opts[0].Label)
	}
	if strings.HasPrefix(opts[0].Label, " ") {
		t.Errorf("engagement root must NOT be indented, got %q", opts[0].Label)
	}
	for _, o := range opts[1:] {
		if !strings.HasPrefix(o.Label, " ") {
			t.Errorf("child option must be nbsp-indented, got %q", o.Label)
		}
	}
}
