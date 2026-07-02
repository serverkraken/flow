package webui

import (
	"context"
	"strings"
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

func TestMoveTargetsFor(t *testing.T) {
	t.Parallel()
	all := nodesFixture()

	// repoB is under eng1; valid move targets are engagements + vorhaben, minus
	// repoB's own subtree (just itself, it has no children).
	repoBNode := domain.Node{ID: "repoB", Kind: domain.KindRepo, Name: "beta", ParentID: ptr("eng1")}
	targets := moveTargetsFor(all, repoBNode)
	ids := map[string]bool{}
	for _, n := range targets {
		ids[n.ID] = true
	}
	if !ids["eng1"] || !ids["eng2"] || !ids["vor1"] {
		t.Errorf("expected eng1/eng2/vor1 as move targets; got %v", ids)
	}
	if ids["repoA"] || ids["repoB"] {
		t.Errorf("repos must not be move targets for a repo; got %v", ids)
	}

	// vor1 descendants include repoA — those must be excluded from targets.
	vor1Node := domain.Node{ID: "vor1", Kind: domain.KindVorhaben, Name: "Buch", ParentID: ptr("eng1")}
	vTargets := moveTargetsFor(all, vor1Node)
	vids := map[string]bool{}
	for _, n := range vTargets {
		vids[n.ID] = true
	}
	if !vids["eng1"] || !vids["eng2"] {
		t.Errorf("vorhaben targets must include engagements; got %v", vids)
	}
	if vids["repoA"] || vids["vor1"] {
		t.Errorf("vorhaben and its descendants must be excluded; got %v", vids)
	}

	// Engagement has no valid parents → always empty.
	engNode := domain.Node{ID: "eng1", Kind: domain.KindEngagement}
	if got := moveTargetsFor(all, engNode); len(got) != 0 {
		t.Errorf("engagement move targets must be empty, got %v", got)
	}
}

func TestNavTree_FormCodedDots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rows := []TreeRow{
		{Node: domain.Node{ID: "e1", Name: "Kundenarbeit", Kind: domain.KindEngagement, Color: "magenta"}, Level: 0},
		{Node: domain.Node{ID: "v1", Name: "Plattform-Umbau", Kind: domain.KindVorhaben, Color: "purple"}, Level: 1},
		{Node: domain.Node{ID: "r1", Name: "flow", Kind: domain.KindRepo, Color: "blue"}, Level: 2},
	}
	body := renderToBuf(t, ctx, NavTree(rows))
	for _, want := range []string{"nvdot-eng", "nvdot-vor", "nvdot-repo", "--nc:var(--magenta)", "fade-label", `title="flow"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav tree missing %q in:\n%s", want, body)
		}
	}
}
