package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func sp(s string) *string { return &s }

func TestRequireType(t *testing.T) {
	if got, err := requireType("memory"); err != nil || got != domain.DocMemory {
		t.Fatalf("requireType(memory) = (%q,%v), want (memory,nil)", got, err)
	}
	if _, err := requireType(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("requireType(\"\") err = %v, want a 'required' error", err)
	}
	if _, err := requireType("bogus"); err == nil || !strings.Contains(err.Error(), "memory") {
		t.Fatalf("requireType(bogus) err = %v, want it to list valid types", err)
	}
}

func TestMergeUpdate(t *testing.T) {
	cur := domain.Document{Title: "Old", Body: "old body"}
	// title only → body carried over
	got, err := mergeUpdate(cur, sp("New"), nil)
	if err != nil || got.Title != "New" || got.Body != "old body" {
		t.Fatalf("title-only merge = (%+v,%v), want New/old body", got, err)
	}
	// body only → title carried over
	got, err = mergeUpdate(cur, nil, sp("new body"))
	if err != nil || got.Title != "Old" || got.Body != "new body" {
		t.Fatalf("body-only merge = (%+v,%v), want Old/new body", got, err)
	}
	// both nil → error
	if _, err := mergeUpdate(cur, nil, nil); err == nil {
		t.Fatal("merge with no fields should error")
	}
}

func TestGuardMutation(t *testing.T) {
	human := domain.Document{ID: "d1", Type: domain.DocFree}
	agent := domain.Document{ID: "d2", Type: domain.DocMemory}
	if err := guardMutation(agent, false); err != nil {
		t.Fatalf("agent-owned without confirm should pass, got %v", err)
	}
	if err := guardMutation(human, true); err != nil {
		t.Fatalf("human-owned WITH confirm should pass, got %v", err)
	}
	err := guardMutation(human, false)
	if err == nil || !strings.Contains(err.Error(), "confirm") || !strings.Contains(err.Error(), "free") {
		t.Fatalf("human-owned without confirm = %v, want an error naming confirm + the type", err)
	}
}
