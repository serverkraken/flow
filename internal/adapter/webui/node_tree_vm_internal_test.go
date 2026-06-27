package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func ptr(s string) *string { return &s }

func nodesFixture() []domain.Node {
	return []domain.Node{
		{ID: "eng1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "repoB", Kind: domain.KindRepo, Name: "beta", ParentID: ptr("eng1")},
		{ID: "repoA", Kind: domain.KindRepo, Name: "alpha", ParentID: ptr("vor1")},
		{ID: "vor1", Kind: domain.KindVorhaben, Name: "Buch", ParentID: ptr("eng1")},
		{ID: "eng2", Kind: domain.KindEngagement, Name: "RTL Extern"},
	}
}

func TestBuildNodeTree_DFSIndentAndOrder(t *testing.T) {
	t.Parallel()
	rows := buildNodeTree(nodesFixture())
	type lr struct {
		id    string
		level int
	}
	got := make([]lr, len(rows))
	for i, r := range rows {
		got[i] = lr{r.Node.ID, r.Level}
	}
	want := []lr{
		{"eng1", 0}, {"vor1", 1}, {"repoA", 2}, {"repoB", 1}, {"eng2", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("rows=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildNodeTree_OrphanFallback(t *testing.T) {
	t.Parallel()
	rows := buildNodeTree([]domain.Node{{ID: "x", Kind: domain.KindRepo, Name: "x", ParentID: ptr("ghost")}})
	if len(rows) != 1 || rows[0].Level != 0 || rows[0].Node.ID != "x" {
		t.Fatalf("orphan not surfaced: %+v", rows)
	}
}

func TestValidParentsFor(t *testing.T) {
	t.Parallel()
	all := nodesFixture()
	if got := ValidParentsFor(domain.KindEngagement, all); len(got) != 0 {
		t.Errorf("engagement must be root-only, got %d", len(got))
	}
	// repo/vorhaben may hang under engagement or vorhaben.
	repoParents := ValidParentsFor(domain.KindRepo, all)
	ids := map[string]bool{}
	for _, n := range repoParents {
		ids[n.ID] = true
	}
	if !ids["eng1"] || !ids["eng2"] || !ids["vor1"] {
		t.Errorf("repo parents missing engagement/vorhaben: %v", ids)
	}
	if ids["repoA"] || ids["repoB"] {
		t.Errorf("repo may not parent a repo: %v", ids)
	}
}

func TestDescendantIDs(t *testing.T) {
	t.Parallel()
	d := descendantIDs(nodesFixture(), "eng1")
	for _, id := range []string{"eng1", "vor1", "repoA", "repoB"} {
		if !d[id] {
			t.Errorf("missing %q in subtree", id)
		}
	}
	if d["eng2"] {
		t.Errorf("eng2 must not be in eng1 subtree")
	}
}
