package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func strp(s string) *string { return &s }

func TestFormatDocList(t *testing.T) {
	docs := []domain.Document{
		{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory, Tags: []string{"go", "design"}},
	}
	out := formatDocList(docs, scope{label: "in project Alpha"})
	for _, want := range []string{"1 document", "Alpha", "d1", "Arch", "notes/arch", "memory", "go, design"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatDocList missing %q in:\n%s", want, out)
		}
	}
	if empty := formatDocList(nil, scope{label: "in project Alpha"}); !strings.HasPrefix(empty, "No documents") {
		t.Errorf("empty list = %q, want a 'No documents' message", empty)
	}
}

func TestFormatSearchHits(t *testing.T) {
	hits := []domain.SearchHit{
		{Document: domain.Document{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory}, Snippet: "the needle here"},
	}
	out := formatSearchHits(hits, "needle", scope{label: "in project Alpha"})
	for _, want := range []string{"1 match", "needle", "d1", "the needle here"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatSearchHits missing %q in:\n%s", want, out)
		}
	}
	if empty := formatSearchHits(nil, "needle", scope{label: "in project Alpha"}); !strings.Contains(empty, "No matches") {
		t.Errorf("empty search = %q, want a 'No matches' message", empty)
	}
}

func TestFormatDoc(t *testing.T) {
	d := domain.Document{ID: "d1", Title: "Arch", Path: "notes/arch", Type: domain.DocMemory, Body: "BODY", Tags: []string{"go"}, Role: strp("brief")}
	out := formatDoc(d, "Alpha")
	for _, want := range []string{"Arch", "notes/arch", "memory", "Alpha", "go", "brief", "d1", "BODY"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatDoc missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatTags(t *testing.T) {
	tags := []domain.TagCount{{Tag: "go", Count: 1}, {Tag: "design", Count: 3}}
	out := formatTags(tags, scope{label: "in project Alpha"})
	// highest count first
	if strings.Index(out, "design") > strings.Index(out, "go") {
		t.Errorf("formatTags should sort by count desc:\n%s", out)
	}
	if empty := formatTags(nil, scope{label: "in project Alpha"}); !strings.Contains(empty, "No tags") {
		t.Errorf("empty tags = %q, want a 'No tags' message", empty)
	}
}

func TestFormatBacklinks(t *testing.T) {
	refs := []domain.BacklinkRef{{ID: "d2", Title: "Todo", Path: "notes/todo", Type: domain.DocFree}}
	out := formatBacklinks(refs, "notes/arch")
	for _, want := range []string{"1 document", "notes/arch", "d2", "Todo"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatBacklinks missing %q in:\n%s", want, out)
		}
	}
	if empty := formatBacklinks(nil, "notes/arch"); !strings.Contains(empty, "No documents link") {
		t.Errorf("empty backlinks = %q, want a 'No documents link' message", empty)
	}
}
