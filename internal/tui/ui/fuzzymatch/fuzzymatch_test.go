package fuzzymatch_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

func TestMatch_SubsequenceAndIndices(t *testing.T) {
	t.Parallel()
	idx, _, ok := fuzzymatch.Match("fb", "foobar")
	if !ok {
		t.Fatal("fb should match foobar")
	}
	// f at 0, b at 3
	if len(idx) != 2 || idx[0] != 0 || idx[1] != 3 {
		t.Errorf("idx = %v, want [0 3]", idx)
	}
}

func TestMatch_CaseInsensitiveAndEmptyAndNoMatch(t *testing.T) {
	t.Parallel()
	if _, _, ok := fuzzymatch.Match("FOO", "foobar"); !ok {
		t.Error("FOO should match foobar case-insensitively")
	}
	if idx, score, ok := fuzzymatch.Match("", "anything"); !ok || idx != nil || score != 0 {
		t.Errorf("empty query: got idx=%v score=%d ok=%v, want nil/0/true", idx, score, ok)
	}
	if idx, _, ok := fuzzymatch.Match("zzz", "foobar"); ok || idx != nil {
		t.Errorf("zzz should not match foobar (idx=%v ok=%v)", idx, ok)
	}
}

func TestMatch_ScoreFavoursContiguousEarly(t *testing.T) {
	t.Parallel()
	_, contig, _ := fuzzymatch.Match("ab", "abc")  // contiguous + at start
	_, spread, _ := fuzzymatch.Match("ab", "axxb") // spread + later
	if contig <= spread {
		t.Errorf("contiguous-early score %d should beat spread-late %d", contig, spread)
	}
}
