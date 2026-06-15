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
