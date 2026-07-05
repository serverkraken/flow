package httpserver

import (
	"net/http"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// handleWebNodeEditSession handles POST /nodes/{id}/sessions/{sid}/edit — a
// thin wrapper over the existing EditSession usecase (same shape as
// handleWebEdit in worktime.go), reached from the cockpit's shared
// SessionDialog in edit mode (cockpit.templ's ?edit={sid} round-trip).
//
// {id} is the currently VIEWED cockpit node — used only to know which panel
// to re-render, NEVER as the session's new node. The form's "node" field
// (hidden when the dialog has no reassignment picker, per
// sessionDialogBody/SessionDialogVM) carries the session's OWN booked node,
// so editing times from a containment view (an Engagement listing a
// descendant Repo's session, Spec §4) cannot silently reassign the booking
// up to the viewed node.
func (s *Server) handleWebNodeEditSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	sid := r.PathValue("sid")
	_ = r.ParseForm()

	day := parseDayParam(s, r.FormValue("date"))
	start, err1 := dayTime(day, r.FormValue("from"))
	stop, err2 := dayTime(day, r.FormValue("to"))
	if err1 != nil || err2 != nil || !stop.After(start) {
		s.renderCockpitMain(w, r, u, id, "ungültige Zeit — HH:MM, bis > von")
		return
	}

	var nodeID *string
	if v := r.FormValue("node"); v != "" {
		nodeID = &v
	}
	webTags := strings.Fields(r.FormValue("tag"))
	sess, err := s.EditSession.Execute(r.Context(), u.ID, sid, usecase.EditSessionInput{
		NodeID: nodeID,
		Tags:   &webTags,
		Note:   r.FormValue("note"),
		Start:  start,
		Stop:   &stop,
	})
	if err != nil {
		s.renderCockpitMain(w, r, u, id, "konnte nicht bearbeiten: "+err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{
		Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID),
	})
	s.renderCockpitMain(w, r, u, id, "")
}

// handleWebNodeDeleteSession handles POST /nodes/{id}/sessions/{sid}/delete —
// a thin wrapper over the existing DeleteSession usecase (same shape as
// handleWebDelete in worktime.go), reached from the cockpit worktime panel's
// per-row ConfirmDialog. {id} is only used to know which panel to re-render.
func (s *Server) handleWebNodeDeleteSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	sid := r.PathValue("sid")

	if err := s.DeleteSession.Execute(r.Context(), u.ID, sid); err != nil {
		s.renderCockpitMain(w, r, u, id, "konnte nicht löschen: "+err.Error())
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{
		Type: domain.EventSessionDeleted, UserID: u.ID,
		Data: map[string]any{"id": sid},
	})
	s.renderCockpitMain(w, r, u, id, "")
}
