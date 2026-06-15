package domain

import (
	"reflect"
	"testing"
)

func TestFindWikilinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []WikilinkSpan
	}{
		{"none", "plain text", nil},
		{"simple", "see [[arch]] now", []WikilinkSpan{{Start: 4, End: 12, Target: "arch", Display: ""}}},
		{"pipe", "see [[arch|Architecture]] x", []WikilinkSpan{{Start: 4, End: 25, Target: "arch", Display: "Architecture"}}},
		{"two", "[[a]] and [[b]]", []WikilinkSpan{
			{Start: 0, End: 5, Target: "a", Display: ""},
			{Start: 10, End: 15, Target: "b", Display: ""},
		}},
		{"empty target ignored", "[[]] and [[|d]]", nil},
		{"unterminated", "[[arch", nil},
		{"newline aborts", "[[ar\nch]]", nil},
		{"adjacent brackets", "[[a]][[b]]", []WikilinkSpan{
			{Start: 0, End: 5, Target: "a", Display: ""},
			{Start: 5, End: 10, Target: "b", Display: ""},
		}},
		{"path slug", "[[daily/2026-06-15]]", []WikilinkSpan{{Start: 0, End: 20, Target: "daily/2026-06-15", Display: ""}}},
		{"stray bracket in target aborts", "[[a]b]]", nil},
		{"stray bracket in display passes through", "[[a|b]c]]", []WikilinkSpan{{Start: 0, End: 9, Target: "a", Display: "b]c"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindWikilinks(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FindWikilinks(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestWikilinkTargets(t *testing.T) {
	got := WikilinkTargets("[[a]] [[b]] [[a]] [[c|x]]")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WikilinkTargets = %#v, want %#v", got, want)
	}
	if WikilinkTargets("no links") != nil {
		t.Fatalf("expected nil for no links")
	}
}
