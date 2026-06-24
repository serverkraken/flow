package tui

import (
	"reflect"
	"testing"
)

func TestFindWeblinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []weblinkSpan
	}{
		{"none", "plain text", nil},
		{"bare", "go http://x.io now", []weblinkSpan{{Start: 3, End: 14, URL: "http://x.io", Display: "http://x.io"}}},
		{"https", "see https://example.com/a", []weblinkSpan{{Start: 4, End: 25, URL: "https://example.com/a", Display: "https://example.com/a"}}},
		{"markdown", "a [site](https://e.com) b", []weblinkSpan{{Start: 2, End: 23, URL: "https://e.com", Display: "site"}}},
		{"trailing punct trimmed", "(see https://e.com).", []weblinkSpan{{Start: 5, End: 18, URL: "https://e.com", Display: "https://e.com"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findWeblinks(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findWeblinks(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestSortByStart covers the sortByStart swap branch (currently at 50%).
// The existing findWeblinks tests never produce out-of-order spans, so the
// swap path (s[i], s[j] = s[j], s[i]) is never executed.
func TestSortByStart(t *testing.T) {
	spans := []weblinkSpan{
		{Start: 10, End: 20, URL: "http://b.com", Display: "b"},
		{Start: 2, End: 8, URL: "http://a.com", Display: "a"},
		{Start: 5, End: 9, URL: "http://m.com", Display: "m"},
	}
	sortByStart(spans)
	if spans[0].Start != 2 || spans[1].Start != 5 || spans[2].Start != 10 {
		t.Errorf("sortByStart: got starts %d %d %d, want 2 5 10",
			spans[0].Start, spans[1].Start, spans[2].Start)
	}
}
