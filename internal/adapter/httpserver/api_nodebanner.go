package httpserver

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// maxBannerJSONBody bounds the JSON PUT body: 1 MiB of raw banner bytes
// base64-encode to ~1.4 MiB; 2 MiB leaves headroom for the JSON envelope
// without letting grossly oversized uploads reach the decoder.
const maxBannerJSONBody = 2 << 20

type setNodeBannerReq struct {
	DataBase64 string `json:"dataBase64"`
}

// handleAPISetNodeBanner is PUT /api/v1/nodes/{id}/banner — the token-authed
// REST counterpart of the WebUI's multipart banner upload, so non-browser
// clients (flow-mcp, CLI) can set a register's banner. It mirrors
// handleAPISetNodeLogo; the usecase re-validates type, size and decodability,
// the handler only undoes the transport encoding and maps errors.
func (s *Server) handleAPISetNodeBanner(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setNodeBannerReq
	if !decodeJSONBody(w, r, &req, maxBannerJSONBody, false) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		http.Error(w, "invalid dataBase64", http.StatusBadRequest)
		return
	}
	n, err := s.UploadNodeBanner.Execute(r.Context(), u.ID, r.PathValue("id"), data)
	switch {
	case errors.Is(err, usecase.ErrBannerTooLarge), errors.Is(err, usecase.ErrBannerBadType):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ports.ErrNodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": n.ID, "name": n.Name}})
		writeJSON(w, http.StatusOK, n)
	}
}
