package nodetree

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func sp(s string) *string { return &s }

func sample() []domain.Node {
	return []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "Privat"},
		{ID: "e2", Kind: domain.KindEngagement, Name: "RTL Extern"},
		{ID: "r1", ParentID: sp("e1"), Kind: domain.KindRepo, Name: "flow"},
		{ID: "v1", ParentID: sp("e1"), Kind: domain.KindVorhaben, Name: "Buch"},
		{ID: "r2", ParentID: sp("e2"), Kind: domain.KindRepo, Name: "gitlab-x"},
	}
}

func TestBuildTree_PreOrderDepthSorted(t *testing.T) {
	t.Parallel()
	rows := BuildTree(sample())
	// roots name-sorted (Privat < RTL Extern); children name-sorted (Buch < flow).
	want := []struct {
		id    string
		depth int
	}{
		{"e1", 0}, {"v1", 1}, {"r1", 1},
		{"e2", 0}, {"r2", 1},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i].Node.ID != w.id || rows[i].Depth != w.depth {
			t.Errorf("row %d = (%s,%d), want (%s,%d)", i, rows[i].Node.ID, rows[i].Depth, w.id, w.depth)
		}
	}
}

func TestBuildTree_OrphanTreatedAsRoot(t *testing.T) {
	t.Parallel()
	rows := BuildTree([]domain.Node{{ID: "x", ParentID: sp("ghost"), Kind: domain.KindRepo, Name: "x"}})
	if len(rows) != 1 || rows[0].Depth != 0 {
		t.Fatalf("orphan not surfaced as root: %+v", rows)
	}
}

func TestFilterKind_FlattensToZeroDepth(t *testing.T) {
	t.Parallel()
	rows := FilterKind(BuildTree(sample()), domain.KindRepo)
	if len(rows) != 2 {
		t.Fatalf("repos = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Node.Kind != domain.KindRepo || r.Depth != 0 {
			t.Errorf("bad filtered row %+v", r)
		}
	}
	if got := FilterKind(BuildTree(sample()), ""); len(got) != 5 {
		t.Errorf("empty kind must keep all, got %d", len(got))
	}
}

func TestFuzzyFilter_KeepsMatchPlusAncestors(t *testing.T) {
	t.Parallel()
	rows := FuzzyFilter(BuildTree(sample()), "flow")
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.Node.ID] = true
	}
	if !ids["r1"] || !ids["e1"] {
		t.Errorf("fuzzy 'flow' must keep r1 + ancestor e1, got %v", ids)
	}
	if ids["e2"] || ids["r2"] {
		t.Errorf("unrelated subtree must be dropped, got %v", ids)
	}
	if got := FuzzyFilter(rows, "   "); len(got) != len(rows) {
		t.Errorf("blank query must be a no-op")
	}
}

func TestMoveCandidates_KindValidNoCycle(t *testing.T) {
	t.Parallel()
	all := sample()
	var r1 domain.Node
	for _, n := range all {
		if n.ID == "r1" {
			r1 = n
		}
	}
	cands := MoveCandidates(all, r1) // repo: parent ∈ {engagement, vorhaben}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.ID] = true
	}
	if !got["e1"] || !got["e2"] || !got["v1"] {
		t.Errorf("repo candidates must include engagements + vorhaben, got %v", got)
	}
	if got["r1"] || got["r2"] {
		t.Errorf("repos are not valid parents of a repo, got %v", got)
	}
}

func TestMoveCandidates_ExcludesOwnSubtree(t *testing.T) {
	t.Parallel()
	all := []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement, Name: "E"},
		{ID: "v1", ParentID: sp("e1"), Kind: domain.KindVorhaben, Name: "V"},
		{ID: "v2", ParentID: sp("v1"), Kind: domain.KindVorhaben, Name: "V2"},
	}
	var v1 domain.Node
	for _, n := range all {
		if n.ID == "v1" {
			v1 = n
		}
	}
	for _, c := range MoveCandidates(all, v1) {
		if c.ID == "v2" || c.ID == "v1" {
			t.Errorf("candidate %s is inside v1's subtree (cycle)", c.ID)
		}
	}
}
