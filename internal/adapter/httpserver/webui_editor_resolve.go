package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

// editorResolveMax caps how many targets one request may ask about — a
// per-request bound (and so per tenant), not a global one.
const editorResolveMax = 200

// editorResolveResponse is what the rich-text editor gets back: only the
// targets that resolve. An absent key means "broken" — the editor marks it
// the way the reading view does (wikilink-broken).
type editorResolveResponse struct {
	Wikilinks map[string]editorWikilinkRef `json:"wikilinks"`
	Embeds    map[string]editorEmbedRef    `json:"embeds"`
}

type editorWikilinkRef struct {
	Href  string `json:"href"`
	Title string `json:"title"`
}

type editorEmbedRef struct {
	Src      string `json:"src"`      // <img src> incl. the cache-busting ?v=ref, as the reading view writes it
	Download string `json:"download"` // the bare serve route for non-images (filechip)
	Name     string `json:"name"`
	Size     string `json:"size"`
	IsImage  bool   `json:"isImage"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// handleWebEditorResolve serves GET /ui/editor/aufloesen: the rich-text editor
// asks the SERVER what its [[Verweise]] and ![[Einbettungen]] resolve to —
// with exactly the rules the reading view applies (domain.ResolveWikilink
// from the editor's node scope, the nearest-ancestor artifact resolver). One
// mechanism: the editor never re-implements resolution in JavaScript.
//
// Query: node (the editor's current node, "" for free), wl (repeatable
// wikilink targets), embed (repeatable artifact slugs). Owner-scoped through
// ListDocuments/ListArtifacts; a foreign node resolves to nothing.
func (s *Server) handleWebEditorResolve(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	q := r.URL.Query()
	nodeID := q.Get("node")
	out := editorResolveResponse{
		Wikilinks: map[string]editorWikilinkRef{},
		Embeds:    map[string]editorEmbedRef{},
	}

	if targets := capStrings(q["wl"], editorResolveMax); len(targets) > 0 && s.ListDocuments.Docs != nil {
		all, err := s.ListDocuments.Execute(r.Context(), u.ID, nil, nil)
		if err != nil {
			s.webServerError(w, r, err)
			return
		}
		src := domain.Document{OwnerID: u.ID}
		if nodeID != "" {
			src.NodeID = &nodeID
		}
		for _, t := range targets {
			if d, ok := domain.ResolveWikilink(src, t, all); ok {
				out.Wikilinks[t] = editorWikilinkRef{Href: "/wissen/" + d.ID, Title: d.Title}
			}
		}
	}

	if slugs := capStrings(q["embed"], editorResolveMax); len(slugs) > 0 {
		if resolve := s.buildEditorArtifactResolver(r, u.ID, nodeID); resolve != nil {
			for _, slug := range slugs {
				if ref, ok := resolve(slug); ok {
					out.Embeds[slug] = editorEmbedRef{
						Src: ref.Href + "?v=" + ref.Ref, Download: ref.Href,
						Name: ref.Name, Size: ref.SizeStr, IsImage: ref.IsImage,
						Width: ref.Width, Height: ref.Height,
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

func capStrings(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}
