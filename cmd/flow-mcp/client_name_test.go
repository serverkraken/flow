package main

import (
	"testing"
)

func TestClientName_EnvOverride(t *testing.T) {
	t.Setenv("FLOW_ACTOR", "my-agent")
	got := clientName(nil)
	if got != "my-agent" {
		t.Fatalf("clientName with FLOW_ACTOR=my-agent: %q, want %q", got, "my-agent")
	}
}

func TestClientName_NilRequest(t *testing.T) {
	t.Setenv("FLOW_ACTOR", "")
	got := clientName(nil)
	if got != "" {
		t.Fatalf("clientName(nil): %q, want empty", got)
	}
}
