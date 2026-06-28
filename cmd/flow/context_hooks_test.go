package main

import "testing"

func TestMergeHooks_AddsThenIdempotent(t *testing.T) {
	got, changed := mergeHooks(map[string]any{})
	if !changed {
		t.Fatal("first merge must report a change")
	}
	hooks, _ := got["hooks"].(map[string]any)
	if hooks["SessionStart"] == nil || hooks["Stop"] == nil {
		t.Fatalf("both hooks must be installed: %+v", hooks)
	}
	_, changed2 := mergeHooks(got)
	if changed2 {
		t.Fatal("second merge must be idempotent (no change)")
	}
}

func TestMergeHooks_PreservesForeignHooks(t *testing.T) {
	in := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "some-other-tool"},
		}}},
	}}
	got, _ := mergeHooks(in)
	ss, _ := got["hooks"].(map[string]any)["SessionStart"].([]any)
	// the foreign entry must survive AND our flow-context entry must be added.
	var foreign, ours bool
	for _, group := range ss {
		for _, h := range group.(map[string]any)["hooks"].([]any) {
			cmd, _ := h.(map[string]any)["command"].(string)
			if cmd == "some-other-tool" {
				foreign = true
			}
			if cmd == sessionStartCommand {
				ours = true
			}
		}
	}
	if !foreign || !ours {
		t.Fatalf("foreign=%v ours=%v", foreign, ours)
	}
}
