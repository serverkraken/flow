package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func fakeProjects() []domain.Node {
	return []domain.Node{
		{ID: "p1", Name: "Alpha", Slug: "alpha"},
		{ID: "p2", Name: "Beta", Slug: "beta"},
	}
}

func TestResolveScope_DefaultUsesMatchedProject(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Node{ID: "p1", Name: "Alpha"}}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID == nil || *sc.nodeID != "p1" {
		t.Fatalf("nodeID = %v, want &\"p1\"", sc.nodeID)
	}
	if !strings.Contains(sc.label, "Alpha") {
		t.Fatalf("label = %q, want it to mention Alpha", sc.label)
	}
}

func TestResolveScope_DefaultUnmatchedIsGlobal(t *testing.T) {
	h := &handlers{matched: false}
	sc, err := h.resolveScope(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID != nil {
		t.Fatalf("nodeID = %v, want nil (global)", sc.nodeID)
	}
}

func TestResolveScope_GlobalAndNoneSentinels(t *testing.T) {
	h := &handlers{matched: true, proj: domain.Node{ID: "p1"}}
	g, err := h.resolveScope(context.Background(), "global")
	if err != nil {
		t.Fatal(err)
	}
	if g.nodeID != nil {
		t.Fatalf("global nodeID = %v, want nil", g.nodeID)
	}
	n, err := h.resolveScope(context.Background(), "none")
	if err != nil {
		t.Fatal(err)
	}
	if n.nodeID == nil || *n.nodeID != "none" {
		t.Fatalf("none nodeID = %v, want &\"none\"", n.nodeID)
	}
}

func TestResolveScope_ExplicitBySlugAndName(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		return fakeProjects(), nil
	}}
	bySlug, err := h.resolveScope(context.Background(), "beta")
	if err != nil {
		t.Fatal(err)
	}
	if bySlug.nodeID == nil || *bySlug.nodeID != "p2" {
		t.Fatalf("by slug = %v, want &\"p2\"", bySlug.nodeID)
	}
	byName, err := h.resolveScope(context.Background(), "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if byName.nodeID == nil || *byName.nodeID != "p1" {
		t.Fatalf("by name = %v, want &\"p1\"", byName.nodeID)
	}
	if calls != 1 {
		t.Fatalf("listProjects called %d times, want 1 (cached after first fetch)", calls)
	}
}

func TestResolveScope_UnknownRefreshesOnceThenErrors(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		return fakeProjects(), nil // never contains "gamma"
	}}
	_, err := h.resolveScope(context.Background(), "gamma")
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !strings.Contains(err.Error(), "gamma") || !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("error %q should name the bad ref and list known slugs", err)
	}
	if calls != 2 {
		t.Fatalf("listProjects called %d times, want 2 (initial + one refresh on miss)", calls)
	}
}

func TestResolveScope_NewlyCreatedFoundAfterRefresh(t *testing.T) {
	calls := 0
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		calls++
		if calls == 1 {
			return fakeProjects(), nil // gamma not yet visible
		}
		return append(fakeProjects(), domain.Node{ID: "p3", Name: "Gamma", Slug: "gamma"}), nil
	}}
	sc, err := h.resolveScope(context.Background(), "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if sc.nodeID == nil || *sc.nodeID != "p3" {
		t.Fatalf("nodeID = %v, want &\"p3\" after refresh", sc.nodeID)
	}
}

func TestResolveScope_ListProjectsError(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		return nil, errors.New("boom")
	}}
	_, err := h.resolveScope(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected the underlying list error to surface")
	}
}

func TestProjectName(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) {
		return fakeProjects(), nil
	}}
	p1 := "p1"
	if got := h.projectName(context.Background(), &p1); got != "Alpha" {
		t.Fatalf("projectName(&p1) = %q, want Alpha", got)
	}
	if got := h.projectName(context.Background(), nil); got != "" {
		t.Fatalf("projectName(nil) = %q, want \"\"", got)
	}
	unknown := "pX"
	if got := h.projectName(context.Background(), &unknown); got != "" {
		t.Fatalf("projectName(unknown) = %q, want \"\"", got)
	}
}

func TestCheckType(t *testing.T) {
	if got, err := checkType(""); err != nil || got != "" {
		t.Fatalf("checkType(\"\") = (%q,%v), want (\"\",nil)", got, err)
	}
	if got, err := checkType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("checkType(\"memory\") = (%q,%v), want (memory,nil)", got, err)
	}
	_, err := checkType("bogus")
	if err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("checkType(\"bogus\") err = %v, want it to list valid types", err)
	}
}
