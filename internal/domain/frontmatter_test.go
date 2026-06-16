package domain

import (
	"reflect"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantTags  []string
		wantStart int
	}{
		{"none", "# Title\n\nbody", nil, 0},
		{"inline list", "---\ntags: [go, tui]\n---\nbody\n", []string{"go", "tui"}, 24},
		{"block list", "---\ntags:\n  - Go\n  - TUI\n---\nrest", []string{"go", "tui"}, 29},
		{"normalize+dedupe", "---\ntags: [Go, go, \" TUI \", \"\"]\n---\nx", []string{"go", "tui"}, 36},
		{"no tags key", "---\ntitle: hi\n---\nbody", nil, 18},
		{"missing close fence", "---\ntags: [go]\nbody without close", nil, 0},
		{"unparseable yaml", "---\ntags: [go\n---\nbody", nil, 0},
		{"close at EOF", "---\ntags: [go]\n---", []string{"go"}, 18},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tags, start := ParseFrontmatter(tc.body)
			if !reflect.DeepEqual(tags, tc.wantTags) {
				t.Errorf("tags = %#v, want %#v", tags, tc.wantTags)
			}
			if start != tc.wantStart {
				t.Errorf("start = %d, want %d", start, tc.wantStart)
			}
			if start > 0 && start <= len(tc.body) {
				_ = tc.body[start:] // must not panic
			}
		})
	}
}

func TestCollectTags(t *testing.T) {
	docs := []Document{
		{Tags: []string{"go", "tui"}},
		{Tags: []string{"go"}},
		{Tags: []string{"web"}},
		{Tags: nil},
	}
	got := CollectTags(docs)
	want := []TagCount{{"go", 2}, {"tui", 1}, {"web", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectTags = %#v, want %#v", got, want)
	}
}
