package usecase

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func kw(id string) domain.SearchHit {
	return domain.SearchHit{Document: domain.Document{ID: id}, Snippet: "kw-" + id}
}
func sem(id string) domain.SemanticHit {
	return domain.SemanticHit{Document: domain.Document{ID: id}, Snippet: "sem-" + id}
}

func TestRRFFuse_UnionAndOrder(t *testing.T) {
	keyword := []domain.SearchHit{kw("a"), kw("b")}
	semantic := []domain.SemanticHit{sem("b"), sem("c")}
	out := rrfFuse(keyword, semantic, 60)
	// union of {a,b,c}; b appears in both arms so ranks highest.
	if len(out) != 3 {
		t.Fatalf("want 3, got %d: %#v", len(out), out)
	}
	if out[0].ID != "b" {
		t.Fatalf("want b first (in both arms), got %q", out[0].ID)
	}
	ids := map[string]bool{}
	for _, h := range out {
		ids[h.ID] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] {
		t.Fatalf("missing ids: %#v", ids)
	}
}

func TestRRFFuse_KeywordSnippetWins(t *testing.T) {
	out := rrfFuse([]domain.SearchHit{kw("a")}, []domain.SemanticHit{sem("a")}, 60)
	if len(out) != 1 || out[0].Snippet != "kw-a" {
		t.Fatalf("keyword snippet should win: %#v", out)
	}
}

func TestRRFFuse_SemanticOnlyUsesChunkSnippet(t *testing.T) {
	out := rrfFuse(nil, []domain.SemanticHit{sem("z")}, 60)
	if len(out) != 1 || out[0].Snippet != "sem-z" {
		t.Fatalf("semantic-only snippet wrong: %#v", out)
	}
}
