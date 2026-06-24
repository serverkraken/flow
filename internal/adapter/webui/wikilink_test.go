package webui

import (
	"testing"
)

// TestWikiLinkNodeDump covers wikiLinkNode.Dump (0% coverage).
// Dump is goldmark's AST debug helper and is only called by ast.Dump();
// verify it does not panic with zero-value inputs.
func TestWikiLinkNodeDump(t *testing.T) {
	w := &wikiLinkNode{Target: "note-abc", Display: "Note ABC"}
	w.Dump([]byte{}, 0)
}
