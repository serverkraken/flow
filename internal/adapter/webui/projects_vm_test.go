package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// TestBuildProjectsVM_GroupsAndDedup pins the Projekte-page tree-as-content
// grouping: vorhaben children become a VorhabenGroup, repo children hanging
// directly off the engagement land in DirectRepos, and short-name collisions
// are deduped within the ENGAGEMENT's visible scope (Spec §5.5).
//
// Deviation from the plan's literal fixture (documented, per task brief):
// the plan's original r1/r2 pair ("…/gitlab/group" vs "…/gitlab/project") do
// NOT actually collide under DisplayNames' collision rule (different short
// names "group" vs "project") even though the plan's assertion expected a
// "gitlab / …" prefix on both. Task 2 hit the identical inconsistency and
// resolved it by adding a genuine second collision partner per leaf instead of
// changing DisplayNames' (already-committed) collision-based behavior. Here
// r4/r5 give "group"/"project" real collision partners so both branches
// (collision → prefix, unique → bare short name) are exercised for real.
func TestBuildProjectsVM_GroupsAndDedup(t *testing.T) {
	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	vor := domain.Node{ID: "v1", Name: "tf-modules", Kind: domain.KindVorhaben, ParentID: ptr("e1")}
	r1 := domain.Node{ID: "r1", Name: "gitlab.com/x/tf-modules/gitlab/group", Kind: domain.KindRepo, ParentID: ptr("v1")}
	r2 := domain.Node{ID: "r2", Name: "gitlab.com/x/tf-modules/gitlab/project", Kind: domain.KindRepo, ParentID: ptr("v1")}
	// r4/r5: genuine collision partners for r1 ("group") and r2 ("project") —
	// same leaf segment, different parent path, so DisplayNames' dedup rule
	// actually fires for both.
	r4 := domain.Node{ID: "r4", Name: "gitlab.com/y/other-modules/gitlab/group", Kind: domain.KindRepo, ParentID: ptr("v1")}
	r5 := domain.Node{ID: "r5", Name: "gitlab.com/y/other-modules/gitlab/project", Kind: domain.KindRepo, ParentID: ptr("v1")}
	direct := domain.Node{ID: "r3", Name: "gitlab.com/x/products/foolu/product/backstage", Kind: domain.KindRepo, ParentID: ptr("e1")}
	nodes := []domain.Node{eng, vor, r1, r2, r4, r5, direct}
	vm := BuildProjectsVM(nodes, nil, nil, nil, time.Now())
	if len(vm.Engagements) != 1 {
		t.Fatalf("want 1 engagement, got %d", len(vm.Engagements))
	}
	es := vm.Engagements[0]
	if len(es.Groups) != 1 || es.Groups[0].Short != "tf-modules" {
		t.Fatalf("vorhaben group wrong: %+v", es.Groups)
	}
	// dedup within the visible engagement: group/project collide → "gitlab / …"
	got := map[string]string{}
	for _, row := range es.Groups[0].Rows {
		got[row.ID] = row.Short
	}
	if got["r1"] != "gitlab / group" || got["r2"] != "gitlab / project" {
		t.Fatalf("dedup failed: %+v", got)
	}
	if len(es.DirectRepos) != 1 || es.DirectRepos[0].Short != "backstage" {
		t.Fatalf("direct repos wrong: %+v", es.DirectRepos)
	}
}

// TestBuildProjectsVM_NestedVorhabenAndTimerAndPathWarn pins: a sub-vorhaben
// (vorhaben-under-vorhaben) renders inline as an IsVorhaben row in its
// parent's group (not its own .vh head) and its children are not expanded;
// the running session's node gets RightK="nodes.row.timerRunning"; and a
// name containing ">" trips PathWarn.
func TestBuildProjectsVM_NestedVorhabenAndTimerAndPathWarn(t *testing.T) {
	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	infra := domain.Node{ID: "infra", Name: "infra", Kind: domain.KindVorhaben, ParentID: ptr("e1")}
	baseInfra := domain.Node{ID: "base-infra", Name: "base-infra", Kind: domain.KindVorhaben, ParentID: ptr("infra")}
	// deeper child of the sub-vorhaben — must NOT be dropped (cycle/orphan-safe)
	// but also must NOT appear expanded on this page.
	deepRepo := domain.Node{ID: "deep1", Name: "deep-repo", Kind: domain.KindRepo, ParentID: ptr("base-infra")}
	backstage := domain.Node{ID: "backstage", Name: "gitlab.com/x/products/foolu/product/backstage", Kind: domain.KindRepo, ParentID: ptr("e1")}
	badPath := domain.Node{ID: "bad1", Name: "gitlab.com>/dataalliance/infra/common/ci-templates", Kind: domain.KindRepo, ParentID: ptr("e1")}
	nodes := []domain.Node{eng, infra, baseInfra, deepRepo, backstage, badPath}

	nid := "backstage"
	running := &domain.WorkSession{ID: "s1", NodeID: &nid}

	vm := BuildProjectsVM(nodes, nil, nil, running, time.Now())
	if len(vm.Engagements) != 1 {
		t.Fatalf("want 1 engagement, got %d", len(vm.Engagements))
	}
	es := vm.Engagements[0]
	if len(es.Groups) != 1 || es.Groups[0].Short != "infra" {
		t.Fatalf("expected 1 group (infra), got %+v", es.Groups)
	}
	var subRow *ProjRow
	for i, row := range es.Groups[0].Rows {
		if row.ID == "base-infra" {
			subRow = &es.Groups[0].Rows[i]
		}
		if row.ID == "deep1" {
			t.Fatalf("deep child of a sub-vorhaben must not be expanded on this page: %+v", row)
		}
	}
	if subRow == nil || !subRow.IsVorhaben {
		t.Fatalf("base-infra must render as an inline IsVorhaben row: %+v", es.Groups[0].Rows)
	}

	// deep1 must still be reachable somewhere (never silently dropped) even
	// though it's not expanded here: it must be marked as visited, i.e. not
	// re-surfaced as an orphan pseudo-section of its own.
	for _, sec := range vm.Engagements {
		if sec.N.ID == "deep1" {
			t.Fatalf("deep1 must not become its own orphan section: %+v", sec)
		}
	}

	var backstageRow, badRow *ProjRow
	for i, row := range es.DirectRepos {
		if row.ID == "backstage" {
			backstageRow = &es.DirectRepos[i]
		}
		if row.ID == "bad1" {
			badRow = &es.DirectRepos[i]
		}
	}
	if backstageRow == nil || backstageRow.RightK != "nodes.row.timerRunning" {
		t.Fatalf("running session's node must carry the timerRunning note: %+v", backstageRow)
	}
	if badRow == nil || !badRow.PathWarn || badRow.RightK != "nodes.row.pathWarn" {
		t.Fatalf("bad path must trip PathWarn + pathWarn note: %+v", badRow)
	}
}

// TestBuildProjectsVM_OrphanFallback pins that a node whose parent is absent
// from the visible set is never silently dropped — it surfaces as its own
// engagement-level section (buildNodeTree's orphan rule, applied here).
func TestBuildProjectsVM_OrphanFallback(t *testing.T) {
	orphanRepo := domain.Node{ID: "ghost1", Name: "ghost-repo", Kind: domain.KindRepo, ParentID: ptr("missing-parent")}
	vm := BuildProjectsVM([]domain.Node{orphanRepo}, nil, nil, nil, time.Now())
	found := false
	for _, sec := range vm.Engagements {
		if sec.N.ID == "ghost1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("orphan node must surface as its own section, got %+v", vm.Engagements)
	}
}

// TestBuildProjectsVM_EmptyIsQuiet pins the "leer" state: no nodes at all
// produces a summary and zero engagement sections (the template renders the
// empty state instead of a card grid).
func TestBuildProjectsVM_EmptyIsQuiet(t *testing.T) {
	vm := BuildProjectsVM(nil, nil, nil, nil, time.Now())
	if len(vm.Engagements) != 0 {
		t.Fatalf("want 0 engagements, got %+v", vm.Engagements)
	}
	if vm.TotalHoursStr == "" {
		t.Fatalf("summary counts must still render (0 counts), got empty TotalHoursStr")
	}
}
