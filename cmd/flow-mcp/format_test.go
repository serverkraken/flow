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

func TestFormatSearchHitsCentersAndBoundsTheActualMatch(t *testing.T) {
	farPrefix := strings.Repeat("irrelevant opening paragraph ", 40)
	hits := []domain.SearchHit{{
		Document: domain.Document{ID: "d1", Title: "Review", Path: "reviews/main", Type: domain.DocMemory, Body: farPrefix + "the decisive F70 checkbox finding is here " + strings.Repeat("tail ", 80)},
		Snippet:  farPrefix,
	}}

	out := formatSearchHits(hits, "F70 checkbox", scope{label: "in project Flow"})
	if !strings.Contains(out, "F70 checkbox") {
		t.Fatalf("snippet missed actual match: %q", out)
	}
	if !strings.Contains(out, "\n    …") {
		t.Fatalf("snippet was not centered away from the unrelated beginning: %q", out)
	}
	if len(out) > 700 {
		t.Fatalf("search output is not bounded: len=%d output=%q", len(out), out)
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

func TestFormatProjects(t *testing.T) {
	ps := []domain.Node{
		{ID: "p1", Name: "Alpha", Slug: "alpha"},
		{ID: "p2", Name: "Beta Project", Slug: "beta"},
	}
	got := formatProjects(ps)
	for _, want := range []string{"Alpha", "alpha", "p1", "Beta Project", "beta", "p2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatProjects missing %q in:\n%s", want, got)
		}
	}
	if formatProjects(nil) == "" {
		t.Fatal("formatProjects(nil) must return a non-empty 'no projects' message")
	}
}

func TestFormatProjectsIncludesStatus(t *testing.T) {
	out := formatProjects([]domain.Node{
		{ID: "p1", Name: "Flow", Slug: "flow", Status: domain.NodePaused, UpstreamGit: "git@github.com:serverkraken/flow.git"},
	})
	if !strings.Contains(out, "paused") {
		t.Errorf("formatProjects must include status, got %q", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("formatProjects must include upstream, got %q", out)
	}
}
