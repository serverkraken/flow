package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) handleWebEinstellungenHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	set, _, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	vm := webui.EinstellungenVM{
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenPage(vm).Render(r.Context(), w)
}

// handleWebSetTargetEinst handles POST /ui/einstellungen/target. It is a port
// of handleWebSetTarget from webui_stats.go (which remains on /ui/stats/target
// until Task 6 removes the Stats page). Handler name differs to avoid collision.
func (s *Server) handleWebSetTargetEinst(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	defaultMin, err := strconv.Atoi(r.FormValue("defaultTargetMin"))
	if err != nil || defaultMin < 0 {
		http.Error(w, "invalid defaultTargetMin", http.StatusBadRequest)
		return
	}
	weekday, err := parseWeekdayTargets(r.Form)
	if err != nil {
		http.Error(w, "invalid weekday target", http.StatusBadRequest)
		return
	}
	if err := s.SetTarget.Execute(r.Context(), u.ID, defaultMin, weekday); err != nil {
		if errors.Is(err, domain.ErrInvalidTarget) {
			http.Error(w, "invalid target", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	// Re-read settings to render the fragment with persisted data.
	set, _, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	vm := webui.EinstellungenVM{
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenTargetFragment(vm).Render(r.Context(), w)
}
