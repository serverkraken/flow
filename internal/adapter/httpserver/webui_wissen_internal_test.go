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

func TestRenderSnippet_StrayStentinelDropped(t *testing.T) {
	// An unmatched HighlightStart with no matching HighlightEnd must not produce
	// an unclosed <mark> tag that would corrupt the page layout.
	stray := "before " + domain.HighlightStart + "after"
	got := renderSnippet(stray)
	if strings.Contains(got, "<mark>") {
		t.Fatalf("unmatched sentinel should not produce <mark>: %q", got)
	}
	if strings.Contains(got, domain.HighlightStart) || strings.Contains(got, domain.HighlightEnd) {
		t.Fatalf("raw sentinel should be stripped from output: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("text around stray sentinel should be preserved: %q", got)
	}
}

func TestWissenQueryString(t *testing.T) {
	tests := []struct {
		name    string
		typeKey string
		tags    []string
		q       string
		want    string
	}{
		{"empty", "", nil, "", ""},
		{"q only", "", nil, "foo", "?q=foo"},
		{"tags only", "", []string{"go"}, "", "?tag=go"},
		{"tags and q", "", []string{"go"}, "bar", "?q=bar&tag=go"},
		{"type only", "daily", nil, "", "?type=daily"},
		{"type and tags and q", "daily", []string{"go"}, "bar", "?q=bar&tag=go&type=daily"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wissenQueryString(tc.typeKey, tc.tags, tc.q)
			if got != tc.want {
				t.Errorf("wissenQueryString(%q, %v, %q) = %q, want %q", tc.typeKey, tc.tags, tc.q, got, tc.want)
			}
		})
	}
}

func TestWissenQueryStringFullPreservesManagementFilters(t *testing.T) {
	got := wissenQueryStringFull("memory", []string{"ops"}, "alpha", "archived", "node-1", "scope", "subtree", "recent", "all")
	want := "?node=node-1&q=alpha&recent=all&scope=subtree&status=archived&tag=ops&type=memory"
	if got != want {
		t.Fatalf("wissenQueryStringFull = %q, want %q", got, want)
	}
	if got := wissenStatus("bogus"); got != "active" {
		t.Fatalf("wissenStatus(bogus) = %q, want active", got)
	}
	if got := wissenScope("", "node-1"); got != "subtree" {
		t.Fatalf("wissenScope(default node filter) = %q, want subtree", got)
	}
	if got := wissenScope("self", "node-1"); got != "self" {
		t.Fatalf("wissenScope(explicit self) = %q, want self", got)
	}
	if got := wissenScope("subtree", "none"); got != "" {
		t.Fatalf("wissenScope(unassigned) = %q, want empty", got)
	}
}

func TestToggledTags(t *testing.T) {
	tests := []struct {
		name   string
		active []string
		tag    string
		want   []string
	}{
		{"add to empty", nil, "go", []string{"go"}},
		{"remove only tag", []string{"go"}, "go", nil},
		{"add second tag", []string{"go"}, "tui", []string{"go", "tui"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toggledTags(tc.active, tc.tag)
			if len(got) != len(tc.want) {
				t.Fatalf("toggledTags(%v, %q) = %v, want %v", tc.active, tc.tag, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("toggledTags(%v, %q) = %v, want %v", tc.active, tc.tag, got, tc.want)
				}
			}
		})
	}
}
