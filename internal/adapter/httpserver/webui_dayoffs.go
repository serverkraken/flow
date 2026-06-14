package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
)

func (s *Server) dayOffData(ctx context.Context, u domain.User) (webui.DayOffData, error) {
	year := s.Clock.Now().Year()
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(year, 12, 31, 0, 0, 0, 0, time.Local)
	list, err := s.ListDayOffs.Execute(ctx, u.ID, from, to)
	if err != nil {
		return webui.DayOffData{}, err
	}
	dtos := make([]apiclient.DayOff, 0, len(list))
	for _, d := range list {
		dtos = append(dtos, apiclient.DayOff{
			Day: d.Date.Format("2006-01-02"), Kind: string(d.Kind),
			Label: d.Label, TargetMin: int(d.Target / time.Minute),
			Holiday: d.Kind == domain.KindHoliday,
		})
	}
	set, toks, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.DayOffData{}, err
	}
	return webui.DayOffData{User: u.Username, Bundesland: set.Bundesland, FeedURL: firstFeedURL(toks), DayOffs: dtos}, nil
}

func firstFeedURL(toks []domain.FeedToken) string {
	if len(toks) == 0 {
		return "(none — regenerate below)"
	}
	return "/ics/" + toks[0].Token + ".ics"
}

func (s *Server) renderDayOffFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	d, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DayOffFragment(d).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	d, err := s.dayOffData(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DayOffPage(d).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	kind, ok := domain.ParseKind(r.FormValue("kind"))
	if ok {
		from, err1 := time.ParseInLocation("2006-01-02", r.FormValue("from"), time.Local)
		to, err2 := time.ParseInLocation("2006-01-02", r.FormValue("to"), time.Local)
		if err1 == nil && err2 == nil {
			_ = s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind, r.FormValue("label"), 0, r.FormValue("skipWeekends") == "true")
		}
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	if day, err := time.ParseInLocation("2006-01-02", r.FormValue("day"), time.Local); err == nil {
		_ = s.DeleteDayOff.Execute(r.Context(), u.ID, day)
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebRegenToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_, _ = s.RegenIcsToken.Execute(r.Context(), u.ID)
	s.renderDayOffFragment(w, r, u)
}
