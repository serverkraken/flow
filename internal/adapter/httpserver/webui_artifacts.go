package httpserver

import (
	"errors"
	"io"
	"net/http"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// readArtifactUpload pulls the required multipart "file" field: the raw
// bytes, the original filename (used as the display Name when the form's
// "name" field is blank — the gallery's upload/replace forms never send an
// explicit name), and the part's declared Content-Type. The declared MIME is
// advisory only — ValidateArtifactBytes (usecase.UploadArtifact) re-sniffs
// the bytes and trusts the sniff for images, falling back to the declared
// value only for downloads. Mirrors readLogoUpload's
// ErrMissingFile/ErrNotMultipart degrade-to-nil-without-error contract.
func readArtifactUpload(r *http.Request) (data []byte, filename, declaredMime string, err error) {
	f, hdr, ferr := r.FormFile("file")
	if errors.Is(ferr, http.ErrMissingFile) || errors.Is(ferr, http.ErrNotMultipart) {
		return nil, "", "", nil
	}
	if ferr != nil {
		return nil, "", "", ferr
	}
	defer func() { _ = f.Close() }()
	if hdr == nil || hdr.Filename == "" {
		return nil, "", "", nil
	}
	b, rerr := io.ReadAll(io.LimitReader(f, domain.MaxArtifactBytes+1))
	if rerr != nil {
		return nil, "", "", rerr
	}
	return b, hdr.Filename, hdr.Header.Get("Content-Type"), nil
}

// artifactErrMsg maps the upload usecase's validation sentinels (and any
// other artifact usecase failure) to an inline i18n message — Spec: too
// large / wrong type / quota all surface inline, never a popup and never a
// 500 for a validation failure.
func artifactErrMsg(r *http.Request, err error) string {
	switch {
	case errors.Is(err, usecase.ErrArtifactTooLarge):
		return i18nT(r, "cockpit.artifacts.err.tooLarge")
	case errors.Is(err, usecase.ErrArtifactBadType):
		return i18nT(r, "cockpit.artifacts.err.badType")
	case errors.Is(err, usecase.ErrArtifactQuotaExceeded):
		return i18nT(r, "cockpit.artifacts.err.quota")
	default:
		return i18nT(r, "cockpit.artifacts.err.generic")
	}
}

// handleWebNodeArtifactUpload handles the gallery's multipart upload form —
// both a brand-new upload (no "slug" field: UploadArtifact.Execute derives a
// fresh slug, suffixing "-1"/"-2" on collision) and the "Ersetzen" affordance
// (a hidden "slug" field on the card's replace form carries the existing
// slug to overwrite). The whole body is capped up front (Muster
// webui_nodes.go/readValidatedLogo) so ParseMultipartForm never buffers an
// unbounded body before the artifact LimitReader runs. The usecase itself
// emits artifact.created/artifact.updated — this handler does not emit
// (Codex-Fund #3, same rule as the REST handler in artifacts.go).
func (s *Server) handleWebNodeArtifactUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxArtifactBytes+64*1024)
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	data, filename, declaredMime, rerr := readArtifactUpload(r)
	if rerr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(rerr, &maxErr) {
			s.renderNodeArtifacts(w, r, u, id, i18nT(r, "cockpit.artifacts.err.tooLarge"))
			return
		}
		s.renderNodeArtifacts(w, r, u, id, i18nT(r, "cockpit.artifacts.err.generic"))
		return
	}
	if len(data) == 0 {
		s.renderNodeArtifacts(w, r, u, id, i18nT(r, "cockpit.artifacts.err.generic"))
		return
	}
	name := r.FormValue("name")
	if name == "" {
		name = filename
	}
	replaceSlug := r.FormValue("slug")
	a := actor.FromContext(r.Context())
	_, err := s.UploadArtifact.Execute(r.Context(), u.ID, id, name, declaredMime, data, replaceSlug, string(a.Kind), a.Ref)
	switch {
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		s.renderNodeArtifacts(w, r, u, id, artifactErrMsg(r, err))
	default:
		s.renderNodeArtifacts(w, r, u, id, "")
	}
}

// handleWebNodeArtifactRename handles the rename dialog's form submit.
// RenameArtifact's GetMeta call enforces "own artifacts only": an inherited
// artifact's slug doesn't exist AT this node (it lives on an ancestor), so
// it yields ErrArtifactNotFound rather than silently renaming the ancestor's
// copy — the same guard that makes a foreign owner's identically-slugged
// artifact unreachable (owner-scoped).
func (s *Server) handleWebNodeArtifactRename(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	slug := r.PathValue("slug")
	err := s.RenameArtifact.Execute(r.Context(), u.ID, id, slug, r.FormValue("name"))
	if err != nil {
		s.renderNodeArtifacts(w, r, u, id, artifactErrMsg(r, err))
		return
	}
	s.renderNodeArtifacts(w, r, u, id, "")
}

// handleWebNodeArtifactDelete handles the ConfirmDialog's destructive
// hx-post. DeleteArtifact.Delete is owner+node+slug scoped, so a foreign
// owner's artifact — or an inherited (wrong-node) slug — simply yields
// ErrArtifactNotFound: no effect, surfaced as a quiet inline note rather
// than a 404 (the gallery still re-renders normally).
func (s *Server) handleWebNodeArtifactDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	slug := r.PathValue("slug")
	err := s.DeleteArtifact.Execute(r.Context(), u.ID, id, slug)
	if err != nil && !errors.Is(err, ports.ErrArtifactNotFound) {
		s.renderNodeArtifacts(w, r, u, id, artifactErrMsg(r, err))
		return
	}
	s.renderNodeArtifacts(w, r, u, id, "")
}
