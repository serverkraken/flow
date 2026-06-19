package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ValidWikilinkTargets returns, in render order, the raw `[[target]]` inner
// target text for every wikilink the renderer treats as valid (resolvable via
// resolver). It applies the SAME goldmark parse as Render — sharing the wikilink
// inline parser via newGoldmark's config — so wikilinks inside fenced code
// blocks and inline code spans are excluded exactly as the renderer excludes
// them (goldmark never parses `[[…]]` there as a wikiLink node). The order and
// validity predicate match renderWikiLink's validWikilinkIdx counting, so a
// caller can index into the result with the renderer's focus ordinal and get the
// same link.
//
// resolver may be nil; in that case no link is valid (mirroring Render, where a
// nil resolver renders every wikilink as broken).
func ValidWikilinkTargets(src string, resolver WikilinkResolver) []string {
	if resolver == nil {
		return nil
	}
	// Parse through the same goldmark config the renderer uses. The nodeRenderer
	// is only needed so newGoldmark can register it; we never run Convert, we
	// walk the parsed AST ourselves, so width/options are immaterial here.
	md := newGoldmark(newNodeRenderer(1, buildOptions(nil)))
	source := []byte(src)
	doc := md.Parser().Parse(text.NewReader(source))

	var out []string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		wl, ok := n.(*wikiLink)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Same validity predicate as renderWikiLink: resolver.Resolve ok==true.
		if _, _, found := resolver.Resolve(wl.Target); found {
			out = append(out, wl.Target)
		}
		return ast.WalkSkipChildren, nil
	})
	return out
}
