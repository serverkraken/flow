package httpserver

import (
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
	"github.com/serverkraken/flow/internal/usecase"
)

// timerWidgetVM assembles the shell timer state for one owner.
func (s *Server) timerWidgetVM(r *http.Request, u domain.User, errMsg string) webui.TimerWidgetVM {
	vm := webui.TimerWidgetVM{Err: errMsg}
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	for _, n := range all {
		if n.Status == domain.NodeActive && domain.IsBookable(n.Kind) {
			vm.Bookable = append(vm.Bookable, n)
		}
	}
	rs, ok, err := s.GetRunningSession.Execute(r.Context(), u.ID)
	if err != nil || !ok {
		return vm
	}
	vm.Running = true
	vm.SessionID = rs.ID
	vm.BaseSeconds = int64(rs.Elapsed(s.Clock.Now()) / time.Second)
	if rs.NodeID == nil {
		vm.Unbound = true
		return vm
	}
	vm.NodeID = *rs.NodeID
	for _, n := range all {
		if n.ID == vm.NodeID {
			vm.NodeName, vm.NodeColor, vm.NodeKind = n.Name, n.Color, n.Kind
			break
		}
	}
	return vm
}

func (s *Server) renderTimerWidget(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TimerWidget(s.timerWidgetVM(r, u, errMsg)).Render(r.Context(), w)
}

func (s *Server) handleTimerWidget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerChip(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TimerChip(s.timerWidgetVM(r, u, "")).Render(r.Context(), w)
}

// timerNodeFromForm resolves projectId / newProject (quick-create, mirrors
// handleWebStop) into a node id pointer. nil = unbound.
func (s *Server) timerNodeFromForm(r *http.Request, u domain.User) *string {
	_ = r.ParseForm()
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{Name: name, Kind: domain.KindEngagement}); err == nil {
			nodeID = p.ID
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
		}
	}
	if nodeID == "" {
		return nil
	}
	return &nodeID
}

func (s *Server) handleTimerStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := s.timerNodeFromForm(r, u)
	sess, err := s.StartSession.Execute(r.Context(), u.ID, nodeID, nil, "")
	if err != nil {
		// e.g. domain.ErrAlreadyRunning (double-submit / two tabs): simply
		// re-render the actual state — the fragment GET below already shows
		// the running session, so no error banner (never a contradictory
		// "no timer running" over a live clock).
		s.renderTimerWidget(w, r, u, "")
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID)
	if gerr != nil || !ok {
		s.renderTimerWidget(w, r, u, "")
		return
	}
	nodeID := s.timerNodeFromForm(r, u)
	if nodeID == nil {
		nodeID = rs.NodeID // bound session: stop books to its own node
	}
	sess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, nodeID)
	if err != nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.needNode"))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerSwitch(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	target := s.timerNodeFromForm(r, u)
	if target == nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.choose"))
		return
	}
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		stopNode := rs.NodeID
		if stopNode == nil {
			stopNode = target // unbound running: book it to the switch target
		}
		if sess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, stopNode); err == nil {
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
				Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
		}
	}
	sess, err := s.StartSession.Execute(r.Context(), u.ID, target, nil, "")
	if err != nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.choose"))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}
