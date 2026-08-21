package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/serverkraken/flow/internal/usecase"
)

// handleWebNodeBanner serves the node's uploaded banner blob. Mirrors
// handleWebNodeLogo: the <img> URL carries ?v={BannerRef}, so each URL's
// content is immutable — strong ETag plus long-lived PRIVATE caching (a banner
// is user content and must never sit in a shared cache), If-None-Match
// short-circuits to 304 while still carrying the ETag.
func (s *Server) handleWebNodeBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	banner, err := s.GetNodeBanner.Execute(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		s.webNotFound(w, r)
		return
	}
	etag := `"` + banner.Ref + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", banner.Mime)
	_, _ = w.Write(banner.Bytes)
}

// readBannerUpload pulls the optional multipart "banner" file field. Mirrors
// readLogoUpload: nil bytes when no file was picked or the request wasn't
// multipart at all. The caller bounds the whole body with MaxBytesReader
// first; the LimitReader here only guards the file copy itself.
func readBannerUpload(r *http.Request) ([]byte, error) {
	f, hdr, err := r.FormFile("banner")
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
	return io.ReadAll(io.LimitReader(f, usecase.MaxNodeBannerBytes+1))
}

// readValidatedBanner pulls and validates the optional banner upload; on
// failure it returns ok=false and the i18n message the caller re-renders with.
// Mirrors readValidatedLogo, including the whole-body-too-large distinction.
func readValidatedBanner(r *http.Request) (data []byte, errMsg string, ok bool) {
	bannerData, berr := readBannerUpload(r)
	if berr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(berr, &maxErr) {
			return nil, i18nT(r, "node.err.bannerSize"), false
		}
		return nil, i18nT(r, "node.err.banner"), false
	}
	if len(bannerData) > 0 {
		if _, verr := usecase.ValidateNodeBanner(bannerData); verr != nil {
			return nil, bannerErrMsg(r, verr), false
		}
	}
	return bannerData, "", true
}

// bannerErrMsg maps a ValidateNodeBanner error to its i18n message.
func bannerErrMsg(r *http.Request, err error) string {
	switch {
	case errors.Is(err, usecase.ErrBannerTooLarge):
		return i18nT(r, "node.err.bannerSize")
	case errors.Is(err, usecase.ErrBannerBadType):
		return i18nT(r, "node.err.bannerType")
	default:
		return i18nT(r, "node.err.banner")
	}
}
