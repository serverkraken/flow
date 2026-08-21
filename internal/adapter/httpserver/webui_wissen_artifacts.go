package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// wissenArtifactsData loads the current owner's free-artifact library
// (ListArtifacts.Execute(owner, "") == ListFree(owner), owner-scoped) into
// the gallery VM — Muster nodeCockpitData's Artifacts wiring, but standalone
// since this page has no node/ancestor chain at all.
func (s *Server) wissenArtifactsData(r *http.Request, ownerID, errMsg string) (webui.WissenArtifactsVM, error) {
	arts, err := s.ListArtifacts.Execute(r.Context(), ownerID, "")
	if err != nil {
		return webui.WissenArtifactsVM{}, err
	}
	return webui.BuildWissenArtifactsVM(arts, errMsg), nil
}

// handleWebWissenArtifacts serves GET /wissen/artefakte: the FULL page
// (AppShell + pagehead + the #wissen-artefakte SSE container) — Muster
// handleWebWissenHome/WissenPage.
func (s *Server) handleWebWissenArtifacts(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.wissenArtifactsData(r, u.ID, "")
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenArtifactsPage(vm).Render(r.Context(), w)
}

// handleWebWissenArtifactsFragment serves GET /ui/wissen/artefakte:
// fragment-only (grid + upload form + error slot, NO AppShell). This is the
// #wissen-artefakte SSE container's hx-get target (gemini-Fund #2, CRITICAL):
// it must never be the full-page route /wissen/artefakte, or an SSE-
// triggered refetch would nest a whole AppShell page inside the container
// div.
func (s *Server) handleWebWissenArtifactsFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderFreeArtifacts(w, r, u.ID, "")
}

// renderFreeArtifacts re-renders the #wissen-artefakte fragment with an
// optional inline error — the shared render target for the fragment GET
// route and every upload/rename/delete mutation below (Muster
// renderNodeArtifacts, webui_cockpit.go).
func (s *Server) renderFreeArtifacts(w http.ResponseWriter, r *http.Request, ownerID, errMsg string) {
	vm, err := s.wissenArtifactsData(r, ownerID, errMsg)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.WissenArtifactsFragment(vm).Render(r.Context(), w)
}

// handleWebWissenArtifactUpload handles the free gallery's multipart upload
// form (Muster handleWebNodeArtifactUpload, nodeID=""): both a brand-new
// upload (no "slug" field — a fresh slug is derived, suffixing "-1"/"-2" on
// collision) and the "Ersetzen" affordance (hidden "slug" field). The
// usecase itself emits artifact.created/artifact.updated — this handler
// does not emit.
func (s *Server) handleWebWissenArtifactUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxArtifactBytes+64*1024)
	u, _ := userFrom(r.Context())
	data, filename, declaredMime, rerr := readArtifactUpload(r)
	if rerr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(rerr, &maxErr) {
			s.renderFreeArtifacts(w, r, u.ID, i18nT(r, "cockpit.artifacts.err.tooLarge"))
			return
		}
		s.renderFreeArtifacts(w, r, u.ID, i18nT(r, "cockpit.artifacts.err.generic"))
		return
	}
	if len(data) == 0 {
		s.renderFreeArtifacts(w, r, u.ID, i18nT(r, "cockpit.artifacts.err.generic"))
		return
	}
	name := r.FormValue("name")
	if name == "" {
		name = filename
	}
	replaceSlug := r.FormValue("slug")
	a := actor.FromContext(r.Context())
	_, err := s.UploadArtifact.Execute(r.Context(), u.ID, "", name, declaredMime, data, replaceSlug, string(a.Kind), a.Ref)
	if err != nil {
		s.renderFreeArtifacts(w, r, u.ID, artifactErrMsg(r, err))
		return
	}
	s.renderFreeArtifacts(w, r, u.ID, "")
}

// handleWebWissenArtifactRename handles the rename dialog's form submit
// (Muster handleWebNodeArtifactRename, nodeID=""). RenameArtifact's GetMeta
// guard is owner+slug scoped (NULL-safe in the store), so a foreign owner's
// identically-slugged free artifact is unreachable — ErrArtifactNotFound,
// surfaced inline rather than a 404.
func (s *Server) handleWebWissenArtifactRename(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	slug := r.PathValue("slug")
	err := s.RenameArtifact.Execute(r.Context(), u.ID, "", slug, r.FormValue("name"))
	if err != nil {
		s.renderFreeArtifacts(w, r, u.ID, artifactErrMsg(r, err))
		return
	}
	s.renderFreeArtifacts(w, r, u.ID, "")
}

// handleWebWissenArtifactDelete handles the ConfirmDialog's destructive
// hx-post (Muster handleWebNodeArtifactDelete, nodeID=""). A foreign owner's
// slug (or none at all) yields ErrArtifactNotFound: no effect, surfaced
// quietly rather than a 404 — the gallery still re-renders normally.
func (s *Server) handleWebWissenArtifactDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	slug := r.PathValue("slug")
	err := s.DeleteArtifact.Execute(r.Context(), u.ID, "", slug)
	if err != nil && !errors.Is(err, ports.ErrArtifactNotFound) {
		s.renderFreeArtifacts(w, r, u.ID, artifactErrMsg(r, err))
		return
	}
	s.renderFreeArtifacts(w, r, u.ID, "")
}
