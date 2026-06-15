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

func strptr(s string) *string { return &s }

func TestResolveWikilink(t *testing.T) {
	pA := strptr("proj-a")
	pB := strptr("proj-b")
	all := []Document{
		{ID: "free1", Path: "shared", ProjectID: nil, Title: "Shared Free"},
		{ID: "a1", Path: "notes", ProjectID: pA, Title: "A Notes"},
		{ID: "b1", Path: "notes", ProjectID: pB, Title: "B Notes"},
		{ID: "bonly", Path: "bsecret", ProjectID: pB, Title: "B Secret"},
	}

	tests := []struct {
		name   string
		src    Document
		target string
		wantID string // "" => not found
	}{
		{"same project wins", Document{ProjectID: pA}, "notes", "a1"},
		{"other project same slug", Document{ProjectID: pB}, "notes", "b1"},
		{"free from project falls back to free", Document{ProjectID: pA}, "shared", "free1"},
		{"free doc links free", Document{ProjectID: nil}, "shared", "free1"},
		{"free doc cannot reach project", Document{ProjectID: nil}, "notes", ""},
		{"project doc cannot reach foreign project", Document{ProjectID: pA}, "bsecret", ""},
		{"missing", Document{ProjectID: pA}, "nope", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveWikilink(tt.src, tt.target, all)
			if tt.wantID == "" {
				if ok {
					t.Fatalf("expected broken, got %q", got.ID)
				}
				return
			}
			if !ok || got.ID != tt.wantID {
				t.Fatalf("ResolveWikilink = (%q,%v), want %q", got.ID, ok, tt.wantID)
			}
		})
	}
}
