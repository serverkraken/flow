package usecase

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
)

// rrfK is the Reciprocal Rank Fusion constant (standard default).
const rrfK = 60

// rrfFuse merges the keyword and semantic ranked lists into one ranking. A
// document's score is the sum, over the arms it appears in, of 1/(k+rank) with a
// 1-based rank. Keyword hits are added first, so a document present in both arms
// keeps its highlighted keyword snippet; a document seen only in the semantic arm
// keeps its chunk snippet. Ties break by first-seen order (stable).
func rrfFuse(keyword []domain.SearchHit, semantic []domain.SemanticHit, k int) []domain.SearchHit {
	type agg struct {
		hit   domain.SearchHit
		score float64
		order int
	}
	m := map[string]*agg{}
	order := 0
	add := func(id string, hit domain.SearchHit, rank int) {
		a, ok := m[id]
		if !ok {
			a = &agg{hit: hit, order: order}
			order++
			m[id] = a
		}
		a.score += 1.0 / float64(k+rank)
	}
	for i, h := range keyword {
		add(h.ID, h, i+1)
	}
	for i, h := range semantic {
		add(h.ID, domain.SearchHit{Document: h.Document, Snippet: h.Snippet}, i+1)
	}
	out := make([]*agg, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].order < out[j].order
	})
	res := make([]domain.SearchHit, len(out))
	for i, a := range out {
		res[i] = a.hit
	}
	return res
}
