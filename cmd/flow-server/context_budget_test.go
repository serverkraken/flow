package main

import "testing"

func TestContextBudget_DefaultAndEnvOverride(t *testing.T) {
	if got := contextBudget(func(string) string { return "" }); got != 12000 {
		t.Errorf("default budget = %d, want 12000", got)
	}
	getenv := func(k string) string {
		if k == "FLOW_CONTEXT_BUDGET" {
			return "5000"
		}
		return ""
	}
	if got := contextBudget(getenv); got != 5000 {
		t.Errorf("env override = %d, want 5000", got)
	}
}
