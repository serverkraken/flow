package apistore

import (
	"context"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// BacklinksOf returns every note in the corpus that contains a wikilink
// pointing TO id. It implements the backlinkProvider interface consumed by
// usecase.RenderBacklinks.
func (s *Store) BacklinksOf(ctx context.Context, id domain.ID) ([]domain.LinkRef, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot := make(map[string]corpusEntry, len(s.corpus))
	for k, v := range s.corpus {
		snapshot[k] = v
	}
	s.mu.Unlock()

	target := id.String()
	var out []domain.LinkRef
	for p, e := range snapshot {
		srcID, err := idFromPath(p)
		if err != nil {
			continue
		}
		links := domain.ExtractLinks([]byte(e.doc.Body))
		for _, l := range links {
			if l.Target == target {
				// Resolve the source note's title from frontmatter.
				fm, _, ferr := domain.ParseFrontmatter([]byte(e.doc.Body))
				title := ""
				if ferr == nil {
					title = fm.Title
				}
				out = append(out, domain.LinkRef{ID: srcID, Title: title})
				break // only one ref per source doc
			}
		}
	}
	return out, nil
}

// LinksFrom returns every note in the corpus that id links TO, resolved with
// titles from the corpus cache. A link whose target is not in the corpus gets
// Title="".
func (s *Store) LinksFrom(ctx context.Context, id domain.ID) ([]domain.LinkRef, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	snapshot := make(map[string]corpusEntry, len(s.corpus))
	for k, v := range s.corpus {
		snapshot[k] = v
	}
	s.mu.Unlock()

	p := docPath(id)
	e, ok := snapshot[p]
	if !ok {
		return nil, nil
	}

	links := domain.ExtractLinks([]byte(e.doc.Body))
	out := make([]domain.LinkRef, 0, len(links))
	for _, l := range links {
		targetID := domain.ID(l.Target)
		title := ""
		if te, found := snapshot[docPath(targetID)]; found {
			fm, _, ferr := domain.ParseFrontmatter([]byte(te.doc.Body))
			if ferr == nil {
				title = fm.Title
			}
		}
		out = append(out, domain.LinkRef{ID: targetID, Title: title})
	}
	return out, nil
}
