package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestBuildContextInventory_MetadataOnlyWithActionableStanding(t *testing.T) {
	leaf := "leaf"
	cc := usecase.ComposedContext{
		Resolution: usecase.ContextResolution{Repo: &domain.Node{ID: leaf, Slug: "flow"}},
		Candidates: []usecase.ContextItem{
			{ID: "always", NodeID: &leaf, ScopeLabel: "repo:flow", Type: domain.DocInstruction, Path: "agents", Title: "Rules", EstTokens: 20, Body: "must not leak"},
			{ID: "included", NodeID: &leaf, ScopeLabel: "repo:flow", Type: domain.DocMemory, Path: "memory/in", Title: "Included", Priority: 2, EstTokens: 30},
			{ID: "dropped", NodeID: &leaf, ScopeLabel: "repo:flow", Type: domain.DocMemory, Path: "memory/out", Title: "Dropped", EstTokens: 200},
			{ID: "hidden", NodeID: &leaf, ScopeLabel: "repo:flow", Type: domain.DocMemory, Path: "memory/hidden", Title: "Hidden", ContextMode: domain.ContextModeNie, EstTokens: 10},
		},
		Instructions: []usecase.ContextItem{{ID: "always"}},
		Memories: map[string][]usecase.ContextItem{
			"leaf": {{ID: "included"}},
		},
		Ranked: []usecase.RankedItem{
			{Item: usecase.ContextItem{ID: "included"}, Included: true, Rank: 1},
			{Item: usecase.ContextItem{ID: "dropped"}, Included: false},
		},
		Hidden: []usecase.ContextItem{{ID: "hidden"}},
		Budget: usecase.ContextBudget{Cap: 100, Used: 50},
	}

	got := buildContextInventory(cc)
	if got.Repo != "flow" || got.Budget.Cap != 100 || len(got.Items) != 4 {
		t.Fatalf("inventory = %+v", got)
	}
	states := map[string]string{}
	for _, item := range got.Items {
		states[item.ID] = item.State
	}
	if states["always"] != "always" || states["included"] != "included" || states["dropped"] != "dropped" || states["hidden"] != "hidden" {
		t.Fatalf("states = %+v", states)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"body"`) || strings.Contains(string(b), "must not leak") {
		t.Fatalf("inventory JSON leaked document body: %s", b)
	}
}

func TestValidateCompleteContextOrder(t *testing.T) {
	want := []string{"a", "b", "c"}
	if err := validateCompleteContextOrder(want, []string{"c", "a", "b"}); err != nil {
		t.Fatalf("complete permutation rejected: %v", err)
	}
	for _, got := range [][]string{{"a", "b"}, {"a", "b", "b"}, {"a", "b", "foreign"}} {
		if err := validateCompleteContextOrder(want, got); err == nil {
			t.Fatalf("invalid order accepted: %v", got)
		}
	}
}

func TestValidateCurateContextInput_RequiresExactlyOneAction(t *testing.T) {
	mode := "nie"
	pinned := true
	archived := true

	for _, in := range []curateContextIn{
		{ID: "d1"},
		{ID: "d1", Mode: &mode, Pinned: &pinned},
		{ID: "d1", Pinned: &pinned, Archived: &archived},
	} {
		if _, err := validateCurateContextInput(in); err == nil {
			t.Fatalf("invalid input accepted: %+v", in)
		}
	}
	for _, in := range []curateContextIn{
		{ID: "d1", Mode: &mode},
		{ID: "d1", Pinned: &pinned},
		{ID: "d1", Archived: &archived},
	} {
		if _, err := validateCurateContextInput(in); err != nil {
			t.Fatalf("valid input rejected: %+v: %v", in, err)
		}
	}
}

func TestFilterArchivedDocuments_OwnerVisibleMetadataScope(t *testing.T) {
	p1, p2 := "p1", "p2"
	docs := []domain.Document{
		{ID: "p1-memory", NodeID: &p1, Type: domain.DocMemory, Body: "secret"},
		{ID: "p1-free", NodeID: &p1, Type: domain.DocFree},
		{ID: "p2-memory", NodeID: &p2, Type: domain.DocMemory},
		{ID: "global-memory", Type: domain.DocMemory},
	}

	got := filterArchivedDocuments(docs, &p1, domain.DocMemory, 10)
	if len(got) != 1 || got[0].ID != "p1-memory" {
		t.Fatalf("filtered docs = %+v", got)
	}
	b, err := json.Marshal(archivedMetadataOf(got[0]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "body") || strings.Contains(string(b), "secret") {
		t.Fatalf("archived metadata leaked body: %s", b)
	}
}
