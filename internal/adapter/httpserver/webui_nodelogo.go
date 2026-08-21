package httpserver

import (
	"net/http"
)

// handleWebNodeLogo serves the node's uploaded logo blob. The <img> URLs carry
// ?v={LogoRef}, so each URL's content is immutable — served with a strong ETag
// (the content hash) plus long-lived private caching; If-None-Match short-
// circuits to 304.
func (s *Server) handleWebNodeLogo(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	logo, err := s.GetNodeLogo.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.webNotFound(w, r)
		return
	}
	etag := `"` + logo.Ref + `"`
	// Set the caching headers before the 304 early-return too — RFC 7232 says a
	// 304 response SHOULD carry the ETag (and any Cache-Control) it would have
	// sent with a 200, so caches can refresh their freshness lifetime.
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", logo.Mime)
	_, _ = w.Write(logo.Bytes)
}
