package httpserver

import (
	"fmt"
	"net/http"
)

// handleServeArtifact serves an artifact's raw bytes (image or download).
// Cache-Split (spec §5): a bare URL is revalidated every time (private,
// no-cache — renaming doesn't bump ref, so a stale cached name would be
// wrong); ?v={ref} is content-addressed and thus immutable. The ETag +
// Cache-Control are set before the 304 early-return too — RFC 7232 says a 304
// SHOULD carry the headers it would have sent with a 200 (mirrors
// handleWebNodeLogo). nosniff blocks the browser from re-guessing the type;
// images render inline, everything else downloads as an attachment.
func (s *Server) handleServeArtifact(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	a, err := s.GetArtifact.Execute(r.Context(), u.ID, r.PathValue("id"), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	etag := `"` + a.Ref + `"`
	w.Header().Set("ETag", etag)
	if r.URL.Query().Get("v") == a.Ref {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", a.Mime)
	if a.IsImage() {
		w.Header().Set("Content-Disposition", "inline")
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Name))
	}
	_, _ = w.Write(a.Bytes)
}
