package main

import "testing"

func TestFlushDecision(t *testing.T) {
	cases := []struct {
		name string
		in   flushInput
		want bool
	}{
		{"loop guard", flushInput{StopHookActive: true, MutatingToolUses: 5, ActiveStale: true}, false},
		{"no work", flushInput{MutatingToolUses: 0, ActiveStale: true}, false},
		{"fresh already flushed", flushInput{MutatingToolUses: 3, ActiveStale: false}, false},
		{"work + stale → remind", flushInput{MutatingToolUses: 3, ActiveStale: true}, true},
	}
	for _, c := range cases {
		if got := flushDecision(c.in); got != c.want {
			t.Errorf("%s: flushDecision=%v want %v", c.name, got, c.want)
		}
	}
}
