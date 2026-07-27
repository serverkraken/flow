package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// refTestNodes models a tree with two slug collisions:
//
//	privat (eng)
//	  └─ strassenfuchs (vorhaben)
//	       ├─ strassenfuchs (repo)   ← shares slug with its parent
//	       └─ dup (repo)
//	rtl (eng)
//	  └─ api (vorhaben)
//	       └─ dup (repo)              ← shares slug with privat/.../dup
func refTestNodes() []domain.Node {
	return []domain.Node{
		{ID: "eng1", Slug: "privat", Kind: domain.KindEngagement},
		{ID: "vor1", Slug: "strassenfuchs", Kind: domain.KindVorhaben, ParentID: p("eng1")},
		{ID: "repo1", Slug: "strassenfuchs", Kind: domain.KindRepo, ParentID: p("vor1")},
		{ID: "rdup1", Slug: "dup", Kind: domain.KindRepo, ParentID: p("vor1")},
		{ID: "eng2", Slug: "rtl", Kind: domain.KindEngagement},
		{ID: "vorApi", Slug: "api", Kind: domain.KindVorhaben, ParentID: p("eng2")},
		{ID: "rdup2", Slug: "dup", Kind: domain.KindRepo, ParentID: p("vorApi")},
	}
}

func TestResolveNodeRef_UniqueBareSlug(t *testing.T) {
	id, err := resolveNodeRef(refTestNodes(), "api")
	if err != nil || id != "vorApi" {
		t.Fatalf("api → (%q, %v), want vorApi", id, err)
	}
}

func TestResolveNodeRef_UnknownSlug(t *testing.T) {
	_, err := resolveNodeRef(refTestNodes(), "ghost")
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("want error naming ghost, got %v", err)
	}
}

func TestResolveNodeRef_AmbiguousSlugListsPaths(t *testing.T) {
	_, err := resolveNodeRef(refTestNodes(), "dup")
	if err == nil {
		t.Fatal("want ambiguity error for slug shared by two nodes")
	}
	// Both fully-qualified paths must be offered so the user can disambiguate.
	for _, want := range []string{"privat/strassenfuchs/dup", "rtl/api/dup"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ambiguity error missing %q; got: %v", want, err)
		}
	}
}

func TestResolveNodeRef_PathToLeaf(t *testing.T) {
	id, err := resolveNodeRef(refTestNodes(), "privat/strassenfuchs/strassenfuchs")
	if err != nil || id != "repo1" {
		t.Fatalf("path → (%q, %v), want repo1", id, err)
	}
}

func TestResolveNodeRef_PathLeadingSlashTolerated(t *testing.T) {
	id, err := resolveNodeRef(refTestNodes(), "/privat/strassenfuchs")
	if err != nil || id != "vor1" {
		t.Fatalf("leading-slash path → (%q, %v), want vor1", id, err)
	}
}

func TestResolveNodeRef_PathMissingSegment(t *testing.T) {
	_, err := resolveNodeRef(refTestNodes(), "privat/nope")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("want error naming the missing segment, got %v", err)
	}
}

func TestResolveNodeRef_PathDisambiguatesCollidingSlug(t *testing.T) {
	id, err := resolveNodeRef(refTestNodes(), "privat/strassenfuchs/dup")
	if err != nil || id != "rdup1" {
		t.Fatalf("path → (%q, %v), want rdup1", id, err)
	}
}
