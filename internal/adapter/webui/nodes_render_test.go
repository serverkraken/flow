package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestNodesFragment_TreeAsContent renders the Projekte page (Task 3: the tree
// as content, Mockup Z.442–570) with a 2-engagement fixture and pins: the
// short name renders large ("backstage"), the full remote-style path renders
// UNCUT in the .path mono line (Spec §11 Eindämmung — no truncation of long
// paths), kind chips are neutral .typechip text (never a glyph), avatar sizes
// av-28/av-36 are wired, and "Direkt am Engagement" appears for the
// engagement that mixes a vorhaben group with direct repos. The dead
// ◆/▲/● kind-glyph coding (pre-L2) must never render.
func TestNodesFragment_TreeAsContent(t *testing.T) {
	longPath := "gitlab.com/dataalliance/products/foolu/product/backstage-application-service-x86"
	if len(longPath) < 80 {
		t.Fatalf("fixture path too short to exercise containment, len=%d", len(longPath))
	}

	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	vor := domain.Node{ID: "v1", Name: "backstage", Kind: domain.KindVorhaben, ParentID: ptr("e1")}
	repo := domain.Node{ID: "r1", Name: longPath, Kind: domain.KindRepo, ParentID: ptr("v1")}
	direct := domain.Node{ID: "r2", Name: "gitlab.com/dataalliance/infra/common/cmdb", Kind: domain.KindRepo, ParentID: ptr("e1")}

	eng2 := domain.Node{ID: "e2", Name: "Privat", Kind: domain.KindEngagement}
	repo2 := domain.Node{ID: "r3", Name: "github.com/serverkraken/flow", Kind: domain.KindRepo, ParentID: ptr("e2")}

	nodes := []domain.Node{eng, vor, repo, direct, eng2, repo2}
	vm := BuildProjectsVM(nodes, nil, nil, nil, time.Now())

	out := renderToBuf(t, context.Background(), NodesFragment(NodesPageData{User: "u1", VM: vm}))

	if !strings.Contains(out, ">backstage<") {
		t.Errorf("short name %q not rendered as its own element; out=%.2000s", "backstage", out)
	}
	if !strings.Contains(out, longPath) {
		t.Errorf("full path must render uncut (Spec §11 Eindämmung); want %q in out=%.2000s", longPath, out)
	}
	if !strings.Contains(out, `class="path"`) {
		t.Errorf("mono full-path line missing class=\"path\"; out=%.2000s", out)
	}
	if !strings.Contains(out, `class="typechip"`) {
		t.Errorf("neutral kind chip missing; out=%.2000s", out)
	}
	if !strings.Contains(out, "av-28") || !strings.Contains(out, "av-36") {
		t.Errorf("avatar sizes av-28 (repo)/av-36 (engagement) missing; out=%.2000s", out)
	}
	if !strings.Contains(out, "Direkt am Engagement") {
		t.Errorf("Direkt am Engagement group header missing for mixed engagement; out=%.2000s", out)
	}
	for _, dead := range []string{"◆", "▲", "●"} {
		if strings.Contains(out, dead) {
			t.Errorf("dead kind-glyph %q must never render on the Projekte page; out=%.2000s", dead, out)
		}
	}
}

// TestNodesFragment_RepoRowShowsDescriptionSubtitle pins Task 5's optional
// Step 5 (Offene Entscheidung #7, entschieden "ja"): a repo row's short
// Description renders as a dimmed, single-line subtitle under the node name
// (Bestand `.s` class, truncate — no new arbitrary CSS); an empty Description
// renders no subtitle at all.
func TestNodesFragment_RepoRowShowsDescriptionSubtitle(t *testing.T) {
	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	withDesc := domain.Node{ID: "r1", Name: "backstage", Kind: domain.KindRepo, ParentID: ptr("e1"), Description: "Kurz-Einzeiler"}
	withoutDesc := domain.Node{ID: "r2", Name: "cmdb", Kind: domain.KindRepo, ParentID: ptr("e1")}

	nodes := []domain.Node{eng, withDesc, withoutDesc}
	vm := BuildProjectsVM(nodes, nil, nil, nil, time.Now())
	out := renderToBuf(t, context.Background(), NodesFragment(NodesPageData{User: "u1", VM: vm}))

	if !strings.Contains(out, "Kurz-Einzeiler") {
		t.Errorf("repo row with a Description must render it as a subtitle; out=%.2000s", out)
	}
	if strings.Count(out, `class="s truncate"`) != 1 {
		t.Errorf("exactly one repo row (the one with a Description) may render the .s truncate subtitle; out=%.2000s", out)
	}
}

// TestNodesFragment_Empty pins the "leer" state: no engagements at all
// renders the calm empty-state copy, never a card grid, and stays glyph-free.
func TestNodesFragment_Empty(t *testing.T) {
	vm := BuildProjectsVM(nil, nil, nil, nil, time.Now())
	out := renderToBuf(t, context.Background(), NodesFragment(NodesPageData{User: "u1", VM: vm}))
	if !strings.Contains(out, "Keine Knoten") {
		t.Errorf("empty state copy missing; out=%.1000s", out)
	}
	if strings.Contains(out, "◆") {
		t.Errorf("empty state must not render the dead glyph; out=%.1000s", out)
	}
}

// TestNodesFragment_HeadsLinkToCockpit pins that the engagement section head
// and each vorhaben group head are links to their node's cockpit — without
// them, an engagement (or a vorhaben that has children) is unreachable from
// the Projekte page except via a child row plus the path backbone (Memory:
// Sichtbarkeit > Redundanz-Elimination). The synthetic "Direkt am Engagement"
// group head has no node and must stay a plain <div>.
func TestNodesFragment_HeadsLinkToCockpit(t *testing.T) {
	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	vor := domain.Node{ID: "v1", Name: "backstage", Kind: domain.KindVorhaben, ParentID: ptr("e1")}
	repo := domain.Node{ID: "r1", Name: "gitlab.com/x/backstage-app", Kind: domain.KindRepo, ParentID: ptr("v1")}
	direct := domain.Node{ID: "r2", Name: "gitlab.com/x/cmdb", Kind: domain.KindRepo, ParentID: ptr("e1")}

	vm := BuildProjectsVM([]domain.Node{eng, vor, repo, direct}, nil, nil, nil, time.Now())
	out := renderToBuf(t, context.Background(), NodesFragment(NodesPageData{User: "u1", VM: vm}))

	if !strings.Contains(out, `class="eng-h" href="/nodes/e1"`) {
		t.Errorf("engagement head must link to its cockpit /nodes/e1; out=%.2000s", out)
	}
	if !strings.Contains(out, `class="vh" href="/nodes/v1"`) {
		t.Errorf("vorhaben group head must link to its cockpit /nodes/v1; out=%.2000s", out)
	}
	// The "Direkt am Engagement" pseudo-group stays unlinked (no node behind it).
	if !strings.Contains(out, `<div class="vh">`) {
		t.Errorf("direct-repos pseudo group head must stay a plain div; out=%.2000s", out)
	}
}
