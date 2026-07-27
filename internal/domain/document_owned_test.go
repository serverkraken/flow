package domain

import "testing"

func TestHumanOwned(t *testing.T) {
	human := []DocumentType{DocDaily, DocProject, DocFree}
	agent := []DocumentType{DocAgent, DocMemory, DocInstruction, DocSkill, DocPlan}
	for _, dt := range human {
		if !dt.HumanOwned() {
			t.Errorf("%q should be human-owned", dt)
		}
	}
	for _, dt := range agent {
		if dt.HumanOwned() {
			t.Errorf("%q should be agent-owned", dt)
		}
	}
	// A future / unknown type defaults to agent-owned (not guarded).
	if DocumentType("future-kind").HumanOwned() {
		t.Error("an unknown type should default to agent-owned (not human-owned)")
	}
}
