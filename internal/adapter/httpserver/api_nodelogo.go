package httpserver

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// maxLogoJSONBody bounds the JSON PUT body: 512 KiB of raw logo bytes
// base64-encode to ~700 KiB; 1 MiB leaves headroom for the JSON envelope
// without letting grossly oversized uploads reach the decoder.
const maxLogoJSONBody = 1 << 20

type setNodeLogoReq struct {
	DataBase64 string `json:"dataBase64"`
}

// handleAPISetNodeLogo is PUT /api/v1/nodes/{id}/logo — the token-authed REST
// counterpart of the WebUI multipart logo upload (replace-on-upload), so
// non-browser clients (flow-mcp, CLI) can set a node logo. The usecase
// re-validates type (PNG/JPEG/WebP), size and decodability; the handler only
// undoes the transport encoding and maps errors.
func (s *Server) handleAPISetNodeLogo(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setNodeLogoReq
	if !decodeJSONBody(w, r, &req, maxLogoJSONBody, false) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		http.Error(w, "invalid dataBase64", http.StatusBadRequest)
		return
	}
	n, err := s.UploadNodeLogo.Execute(r.Context(), u.ID, r.PathValue("id"), data)
	switch {
	case errors.Is(err, usecase.ErrLogoTooLarge), errors.Is(err, usecase.ErrLogoBadType):
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
