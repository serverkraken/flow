package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/serverkraken/flow/internal/usecase"
)

// handleWebNodeLogo serves the node's uploaded logo blob. The <img> URLs carry
// ?v={LogoRef}, so each URL's content is immutable — served with a strong ETag
// (the content hash) plus long-lived private caching; If-None-Match short-
// circuits to 304.
func (s *Server) handleWebNodeLogo(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	logo, err := s.GetNodeLogo.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	etag := `"` + logo.Ref + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", logo.Mime)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(logo.Bytes)
}

// readLogoUpload pulls the optional multipart "logo" file field: nil bytes when
// the user picked no file, or the request wasn't multipart at all (e.g. a
// plain application/x-www-form-urlencoded submission, which never carries a
// file field). Reads at most MaxNodeLogoBytes+1 so an oversized upload fails
// validation instead of ballooning memory.
func readLogoUpload(r *http.Request) ([]byte, error) {
	f, hdr, err := r.FormFile("logo")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if hdr == nil || hdr.Filename == "" || hdr.Size == 0 {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(f, usecase.MaxNodeLogoBytes+1))
}
