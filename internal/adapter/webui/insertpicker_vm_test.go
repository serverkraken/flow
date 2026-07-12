package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestBuildArtefaktInsertRows_DedupsBySlugNodeWins is the codex #1 / OE #6
// mandatory test: ListArtifacts(node) = chain ++ free can return the SAME
// slug twice (a node-"logo" and a free-"logo"). Both rows would otherwise
// insert the identical "![[logo]]" markdown, which the read-side resolver
// resolves node-wins — a duplicate free row would be a silent misdirect (it
// looks like a second valid choice but is actually shadowed). The picker
// must dedup by slug, first hit wins; ListArtifacts returns node-before-free
// (append(nodeArts, free...)), so the node entry — listed first here — must
// be the one that survives.
func TestBuildArtefaktInsertRows_DedupsBySlugNodeWins(t *testing.T) {
	arts := []domain.Artifact{
		{NodeID: "n1", Slug: "logo", Name: "Node-Logo.png", Mime: "image/png"},
		{NodeID: "", Slug: "logo", Name: "Free-Logo.png", Mime: "image/png"},
	}
	rows := BuildArtefaktInsertRows(arts, "")
	count := 0
	var label string
	for _, r := range rows {
		if r.Sub == "logo" {
			count++
			label = r.Label
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one 'logo' row (deduped), got %d: %+v", count, rows)
	}
	if label != "Node-Logo.png" {
		t.Fatalf("the node row must win the dedup, got label=%q", label)
	}
}
