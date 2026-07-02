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

// readLogoUpload pulls the optional multipart "logo" file field: nil bytes when
// the user picked no file, or the request wasn't multipart at all (e.g. a
// plain application/x-www-form-urlencoded submission, which never carries a
// file field). The handlers (handleWebNodeCreate/handleWebNodeUpdate) bound
// the whole request body with http.MaxBytesReader before this runs, so an
// oversized *body* fails fast there; the io.LimitReader below only guards the
// file copy itself (defense in depth, e.g. against a multipart part whose
// declared size lies).
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

// readValidatedLogo pulls and validates the optional logo upload; on failure it
// returns ok=false and the i18n message the caller should re-render with. It
// distinguishes the whole-body-too-large case (a *http.MaxBytesError from the
// MaxBytesReader the caller installed on r.Body) from a generic upload/parse
// failure, and reports the sniffed-content validation failure (bad type/size)
// via logoErrMsg. Both handleWebNodeCreate and handleWebNodeUpdate share this.
func readValidatedLogo(r *http.Request) (data []byte, errMsg string, ok bool) {
	logoData, lerr := readLogoUpload(r)
	if lerr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(lerr, &maxErr) {
			return nil, i18nT(r, "node.err.logoSize"), false
		}
		return nil, i18nT(r, "node.err.logo"), false
	}
	if len(logoData) > 0 {
		if _, verr := usecase.ValidateNodeLogo(logoData); verr != nil {
			return nil, logoErrMsg(r, verr), false
		}
	}
	return logoData, "", true
}
