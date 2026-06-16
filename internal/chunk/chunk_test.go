package chunk

import (
	"strings"
	"testing"
)

func TestSplit_EmptyBody_TitleOnly(t *testing.T) {
	got := Split("Title", "")
	if len(got) != 1 || got[0] != "Title" {
		t.Fatalf("got %#v, want [\"Title\"]", got)
	}
}

func TestSplit_EmptyBoth_Nil(t *testing.T) {
	if got := Split("", ""); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestSplit_ShortBody_SingleChunkWithTitle(t *testing.T) {
	got := Split("Subj", "a short body")
	if len(got) != 1 {
		t.Fatalf("want 1 chunk, got %d: %#v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "Subj\n\n") || !strings.Contains(got[0], "a short body") {
		t.Fatalf("chunk missing title prefix or body: %q", got[0])
	}
}

func TestSplit_LongBody_MultipleOverlappingChunks(t *testing.T) {
	body := strings.Repeat("x", MaxChars*2)
	got := Split("T", body)
	if len(got) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(got))
	}
	// every chunk carries the title
	for i, c := range got {
		if !strings.HasPrefix(c, "T\n\n") {
			t.Fatalf("chunk %d missing title prefix: %q", i, c[:min(20, len(c))])
		}
	}
}

func min(a, b int) int { if a < b { return a }; return b }
