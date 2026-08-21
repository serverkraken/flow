package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

// parseWeekdayTargets reads the five optional Mon–Fri target inputs. An empty
// input omits that weekday (inherit the default); a non-numeric or negative
// value is rejected with domain.ErrInvalidTarget.
func parseWeekdayTargets(form url.Values) (map[time.Weekday]int, error) {
	fields := []struct {
		name string
		wd   time.Weekday
	}{
		{"mon", time.Monday},
		{"tue", time.Tuesday},
		{"wed", time.Wednesday},
		{"thu", time.Thursday},
		{"fri", time.Friday},
	}
	out := make(map[time.Weekday]int, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(form.Get(f.name))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, domain.ErrInvalidTarget
		}
		out[f.wd] = n
	}
	return out, nil
}

func (s *Server) handleWebEinstellungenHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	set, _, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	vm := webui.EinstellungenVM{
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenPage(vm).Render(r.Context(), w)
}

// handleWebSetTargetEinst handles POST /ui/einstellungen/target.
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
		s.webServerError(w, r, err)
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	// Re-read settings to render the fragment with persisted data.
	set, _, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	vm := webui.EinstellungenVM{
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenTargetFragment(vm).Render(r.Context(), w)
}
