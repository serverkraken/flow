package httpserver

import "testing"

func TestEncodeTagQuery(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"nil", nil, ""},
		{"empty", []string{}, ""},
		{"one tag", []string{"go"}, "?tag=go"},
		{"two tags", []string{"go", "tui"}, "?tag=go&tag=tui"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeTagQuery(tc.tags)
			if got != tc.want {
				t.Errorf("encodeTagQuery(%v) = %q, want %q", tc.tags, got, tc.want)
			}
		})
	}
}

func TestToggleTagHref(t *testing.T) {
	tests := []struct {
		name   string
		active []string
		tag    string
		want   string
	}{
		{"add to empty", nil, "go", "/docs?tag=go"},
		{"remove only tag", []string{"go"}, "go", "/docs"},
		{"add second tag", []string{"go"}, "tui", "/docs?tag=go&tag=tui"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toggleTagHref(tc.active, tc.tag)
			if got != tc.want {
				t.Errorf("toggleTagHref(%v, %q) = %q, want %q", tc.active, tc.tag, got, tc.want)
			}
		})
	}
}
