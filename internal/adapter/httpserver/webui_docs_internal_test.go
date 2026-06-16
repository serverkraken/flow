package httpserver

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestRenderSnippet_EscapesThenMarks(t *testing.T) {
	got := renderSnippet("a<b " + domain.HighlightStart + "x" + domain.HighlightEnd)
	if !strings.Contains(got, "&lt;b") {
		t.Fatalf("did not escape HTML: %q", got)
	}
	if !strings.Contains(got, "<mark>x</mark>") {
		t.Fatalf("did not wrap with <mark>: %q", got)
	}
}

func TestRenderSnippet_NoInjection(t *testing.T) {
	malicious := "<script>alert(1)</script> " + domain.HighlightStart + "match" + domain.HighlightEnd
	got := renderSnippet(malicious)
	if strings.Contains(got, "<script>") {
		t.Fatalf("HTML injection not escaped: %q", got)
	}
	if !strings.Contains(got, "<mark>match</mark>") {
		t.Fatalf("legitimate mark missing: %q", got)
	}
}

func TestEncodeListQuery(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		q    string
		want string
	}{
		{"empty", nil, "", ""},
		{"q only", nil, "foo", "?q=foo"},
		{"tags only", []string{"go"}, "", "?tag=go"},
		{"tags and q", []string{"go"}, "bar", "?q=bar&tag=go"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeListQuery(tc.tags, tc.q)
			if got != tc.want {
				t.Errorf("encodeListQuery(%v, %q) = %q, want %q", tc.tags, tc.q, got, tc.want)
			}
		})
	}
}

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
