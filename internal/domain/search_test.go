package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchHit_FlatJSON(t *testing.T) {
	h := SearchHit{
		Document: Document{ID: "a", Type: DocFree, Path: "p", Title: "T", Tags: []string{"go"}},
		Snippet:  "hello " + HighlightStart + "world" + HighlightEnd,
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if strings.Contains(js, `"Document"`) {
		t.Fatalf("JSON should be flat, got %s", js)
	}
	if !strings.Contains(js, `"id":"a"`) || !strings.Contains(js, `"snippet":`) {
		t.Fatalf("missing fields: %s", js)
	}
	var d Document
	if err := json.Unmarshal(b, &d); err != nil || d.ID != "a" {
		t.Fatalf("plain Document decode failed: %v / %#v", err, d)
	}
}

func TestHighlightSentinels(t *testing.T) {
	if HighlightStart == HighlightEnd || HighlightStart == "" {
		t.Fatal("sentinels must be distinct and non-empty")
	}
}

func TestStripHighlightSentinels(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"", ""},
		{HighlightStart + "match" + HighlightEnd, "match"},
		{"a" + HighlightStart + "b" + HighlightStart + "c" + HighlightEnd + "d" + HighlightEnd + "e", "abcde"},
		{HighlightStart + HighlightEnd, ""},
		{"plain text without sentinels", "plain text without sentinels"},
	}
	for _, tc := range tests {
		got := StripHighlightSentinels(tc.in)
		if got != tc.want {
			t.Errorf("StripHighlightSentinels(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
