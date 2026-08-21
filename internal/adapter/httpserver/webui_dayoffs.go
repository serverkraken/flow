package httpserver

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// germanStates is the Bundesland <select> source (DE = bundesweit only).
var germanStates = []webui.FreiBundeslandOption{
	{Code: "DE", Name: "Bundesweit"},
	{Code: "BW", Name: "Baden-Württemberg"},
	{Code: "BY", Name: "Bayern"},
	{Code: "BE", Name: "Berlin"},
	{Code: "BB", Name: "Brandenburg"},
	{Code: "HB", Name: "Bremen"},
	{Code: "HH", Name: "Hamburg"},
	{Code: "HE", Name: "Hessen"},
	{Code: "MV", Name: "Mecklenburg-Vorpommern"},
	{Code: "NI", Name: "Niedersachsen"},
	{Code: "NW", Name: "Nordrhein-Westfalen"},
	{Code: "RP", Name: "Rheinland-Pfalz"},
	{Code: "SL", Name: "Saarland"},
	{Code: "SN", Name: "Sachsen"},
	{Code: "ST", Name: "Sachsen-Anhalt"},
	{Code: "SH", Name: "Schleswig-Holstein"},
	{Code: "TH", Name: "Thüringen"},
}

func bundeslandOptions() []webui.FreiBundeslandOption { return germanStates }

func bundeslandName(code string) string {
	for _, o := range germanStates {
		if o.Code == code {
			return o.Name
		}
	}
	return code
}

func (s *Server) dayOffData(ctx context.Context, u domain.User) (webui.FreiVM, error) {
	now := s.Clock.Now()
	loc := now.Location()
	year := now.Year()
	from := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(year, 12, 31, 0, 0, 0, 0, loc)

	list, err := s.ListDayOffs.Execute(ctx, u.ID, from, to)
	if err != nil {
		return webui.FreiVM{}, err
	}
	set, toks, err := s.GetSettings.Execute(ctx, u.ID)
	if err != nil {
		return webui.FreiVM{}, err
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Date.Before(list[j].Date) })
	rows := make([]webui.FreiRowVM, 0, len(list))
	hasOwn := false
	for _, d := range list {
		isHol := d.Kind == domain.KindHoliday
		if !isHol {
			hasOwn = true
		}
		rows = append(rows, webui.FreiRowVM{
			DateLabel: d.Date.In(loc).Format("02.01.2006"),
			KindLabel: d.Kind.LabelDe(),
			Hue:       dayOffHue(d.Kind),
			Label:     d.Label,
			IsHoliday: isHol,
			Day:       d.Date.In(loc).Format("2006-01-02"),
		})
	}

	code, _ := domain.ValidBundesland(set.Bundesland)
	if code == "" {
		code = "DE"
	}
	own, kinds, next := webui.BuildFreiSummary(rows, now.In(loc).Format("2006-01-02"))
	return webui.FreiVM{
		OwnDays:           own,
		KindCounts:        kinds,
		NextHolidays:      next,
		User:              u.Username,
		BundeslandCode:    code,
		BundeslandName:    bundeslandName(code),
		BundeslandOptions: bundeslandOptions(),
		Year:              strconv.Itoa(year),
		IcsURL:            firstFeedURL(toks),
		Rows:              rows,
		HasOwn:            hasOwn,
	}, nil
}

func firstFeedURL(toks []domain.FeedToken) string {
	if len(toks) == 0 {
		return "(none — regenerate below)"
	}
	return "/ics/" + toks[0].Token + ".ics"
}

func (s *Server) renderDayOffFragment(w http.ResponseWriter, r *http.Request, u domain.User) {
	s.renderDayOffFragmentError(w, r, u, "")
}

func (s *Server) renderDayOffFragmentError(w http.ResponseWriter, r *http.Request, u domain.User, errorKey string) {
	vm, err := s.dayOffData(r.Context(), u)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	if errorKey != "" {
		vm.Err = components.T(r.Context(), errorKey)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.FreiFragment(vm).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffHome(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	vm, err := s.dayOffData(r.Context(), u)
	if err != nil {
		s.webServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.FreiPage(vm).Render(r.Context(), w)
}

func (s *Server) handleWebDayOffFragment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	kind, ok := domain.ParseKind(r.FormValue("kind"))
	from, err1 := time.ParseInLocation("2006-01-02", r.FormValue("from"), time.Local)
	to, err2 := time.ParseInLocation("2006-01-02", r.FormValue("to"), time.Local)
	if !ok || err1 != nil || err2 != nil {
		s.renderDayOffFragmentError(w, r, u, "frei.error.invalid")
		return
	}
	if err := s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind, r.FormValue("label"), 0, r.FormValue("skipWeekends") == "true"); err != nil {
		key := "frei.error.save"
		if errors.Is(err, usecase.ErrHolidayNotManual) || errors.Is(err, usecase.ErrDayOffRangeTooLarge) || errors.Is(err, domain.ErrInvalidDayOff) {
			key = "frei.error.invalid"
		}
		s.renderDayOffFragmentError(w, r, u, key)
		return
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebDayOffDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	day, err := time.ParseInLocation("2006-01-02", r.FormValue("day"), time.Local)
	if err != nil {
		s.renderDayOffFragmentError(w, r, u, "frei.error.invalid")
		return
	}
	if err := s.DeleteDayOff.Execute(r.Context(), u.ID, day); err != nil {
		s.renderDayOffFragmentError(w, r, u, "frei.error.delete")
		return
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebRegenToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if _, err := s.RegenIcsToken.Execute(r.Context(), u.ID); err != nil {
		s.renderDayOffFragmentError(w, r, u, "frei.error.token")
		return
	}
	s.renderDayOffFragment(w, r, u)
}

func (s *Server) handleWebSetBundesland(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	_ = r.ParseForm()
	if err := s.SetBundesland.Execute(r.Context(), u.ID, r.FormValue("bundesland")); err != nil {
		if errors.Is(err, domain.ErrInvalidDayOff) {
			http.Error(w, "invalid bundesland", http.StatusBadRequest)
			return
		}
		s.webServerError(w, r, err)
		return
	}
	// Holidays are derived from the Bundesland → notify other tabs to reload.
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	s.renderDayOffFragment(w, r, u)
}
