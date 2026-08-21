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
	set, toks, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	vm := s.einstellungenVM(r, u, set, toks)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenPage(vm).Render(r.Context(), w)
}

// einstellungenVM füllt Screen 30: Konto vom Anmeldedienst, Sollzeit der
// Woche, Engagements, ICS-Abos, Bundesland, Sprache aus dem Cookie.
func (s *Server) einstellungenVM(r *http.Request, u domain.User, set domain.Settings, toks []domain.FeedToken) webui.EinstellungenVM {
	vm := webui.EinstellungenVM{
		DefaultTarget: strconv.Itoa(set.DefaultTargetMin),
		Weekdays:      webui.StatsWeekdayVMs(set.WeekdayTargetMin),
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Email:         u.Email,
		WeekSoll:      webui.FmtMinutesClock(webui.WeekSollMinutes(set.DefaultTargetMin, set.WeekdayTargetMin)),
		FeedTokens:    len(toks),
		Bundesland:    set.Bundesland,
	}
	if c, err := r.Cookie("flow_lang"); err == nil && (c.Value == "de" || c.Value == "en") {
		vm.Lang = c.Value
	}
	if s.ListNodes.Nodes != nil {
		if nodes, err := s.ListNodes.Execute(r.Context(), u.ID); err == nil {
			for _, n := range nodes {
				if n.Kind == domain.KindEngagement {
					vm.Engagements++
				}
			}
		}
	}
	return vm
}

// handleWebSetLanguage serves POST /ui/einstellungen/sprache: setzt oder
// löscht das Sprach-Cookie, das i18n.Resolve vor Accept-Language liest.
func (s *Server) handleWebSetLanguage(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	lang := r.FormValue("lang")
	c := &http.Cookie{Name: "flow_lang", Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteLaxMode}
	switch lang {
	case "de", "en":
		c.Value = lang
		c.MaxAge = int((365 * 24 * time.Hour).Seconds())
	default:
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, "/einstellungen#sprache", http.StatusSeeOther)
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
	set, toks, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	vm := s.einstellungenVM(r, u, set, toks)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.EinstellungenTargetFragment(vm).Render(r.Context(), w)
}
