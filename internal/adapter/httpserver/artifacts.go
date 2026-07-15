package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// maxArtifactJSONBody bounds the JSON POST body: 8 MiB of raw bytes base64-
// encodes to ~10.67 MiB (× 4/3), plus room for the {name,mime,dataBase64,slug}
// envelope — 12 MiB comfortably covers both.
const maxArtifactJSONBody = 12 << 20

type uploadArtifactReq struct {
	Name       string `json:"name"`
	Mime       string `json:"mime"`
	DataBase64 string `json:"dataBase64"`
	Slug       string `json:"slug"` // optional: set → replace/re-upload this slug
}

// handleUploadArtifact is the JSON-only REST upload (POST body carries the
// bytes base64-encoded; the web gallery's multipart upload is Task 5) for a
// node-scoped artifact. handleUploadFreeArtifact is its free-artifacts Task 3
// counterpart (POST /api/v1/artifacts, no {id} in the path) — both share
// uploadArtifactJSON, which does not emit itself (the usecase emits
// artifact.created/artifact.updated — Codex-Fund #3, see
// usecase.UploadArtifact's doc comment).
func (s *Server) handleUploadArtifact(w http.ResponseWriter, r *http.Request) {
	s.uploadArtifactJSON(w, r, r.PathValue("id"))
}

// handleUploadFreeArtifact is the owner-global counterpart of
// handleUploadArtifact: POST /api/v1/artifacts (no node in the path) uploads
// a free (node-less) artifact.
func (s *Server) handleUploadFreeArtifact(w http.ResponseWriter, r *http.Request) {
	s.uploadArtifactJSON(w, r, "")
}

// uploadArtifactJSON is the shared JSON-upload-parse + usecase-call +
// error-switch both handleUploadArtifact (nodeID from the path) and
// handleUploadFreeArtifact (nodeID=="") call — the free-artifacts Task 3
// extraction that keeps the two REST entry points from duplicating the
// base64-decode/error-mapping logic.
func (s *Server) uploadArtifactJSON(w http.ResponseWriter, r *http.Request, nodeID string) {
	u, _ := userFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxArtifactJSONBody)
	var req uploadArtifactReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		http.Error(w, "invalid dataBase64", http.StatusBadRequest)
		return
	}
	a := actor.FromContext(r.Context())
	art, err := s.UploadArtifact.Execute(r.Context(), u.ID, nodeID, req.Name, req.Mime, data, req.Slug, string(a.Kind), a.Ref)
	switch {
	case errors.Is(err, usecase.ErrArtifactTooLarge), errors.Is(err, usecase.ErrArtifactBadType), errors.Is(err, domain.ErrInvalidArtifact):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, usecase.ErrArtifactQuotaExceeded):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, ports.ErrNodeNotFound), errors.Is(err, ports.ErrArtifactNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusCreated, art)
	}
}

// handleListArtifacts returns artifact meta (Ahnenkette, not subtree) for the
// node in the URL. handleListFreeArtifacts is its free-artifacts Task 3
// counterpart (GET /api/v1/artifacts) — both share listArtifactsJSON.
func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	s.listArtifactsJSON(w, r, r.PathValue("id"))
}

// handleListFreeArtifacts is the owner-global counterpart of
// handleListArtifacts: GET /api/v1/artifacts (no node in the path) returns
// the owner's free (node-less) artifact library.
func (s *Server) handleListFreeArtifacts(w http.ResponseWriter, r *http.Request) {
	s.listArtifactsJSON(w, r, "")
}

func (s *Server) listArtifactsJSON(w http.ResponseWriter, r *http.Request, nodeID string) {
	u, _ := userFrom(r.Context())
	list, err := s.ListArtifacts.Execute(r.Context(), u.ID, nodeID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []domain.Artifact{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleDeleteArtifact removes one node-scoped artifact; the usecase emits
// artifact.deleted. handleDeleteFreeArtifact is its free-artifacts Task 3
// counterpart (DELETE /api/v1/artifacts/{slug}) — both share
// deleteArtifactJSON.
func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	s.deleteArtifactJSON(w, r, r.PathValue("id"), r.PathValue("slug"))
}

// handleDeleteFreeArtifact is the owner-global counterpart of
// handleDeleteArtifact: DELETE /api/v1/artifacts/{slug} (no node in the path)
// removes a free (node-less) artifact by slug.
func (s *Server) handleDeleteFreeArtifact(w http.ResponseWriter, r *http.Request) {
	s.deleteArtifactJSON(w, r, "", r.PathValue("slug"))
}

func (s *Server) deleteArtifactJSON(w http.ResponseWriter, r *http.Request, nodeID, slug string) {
	u, _ := userFrom(r.Context())
	err := s.DeleteArtifact.Execute(r.Context(), u.ID, nodeID, slug)
	switch {
	case errors.Is(err, ports.ErrArtifactNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
