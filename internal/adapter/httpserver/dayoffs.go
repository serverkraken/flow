package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

const dayFmt = "2006-01-02"

// dayOffDTO is the wire shape: target as minutes (not Duration-nanoseconds),
// date as yyyy-mm-dd, and an explicit holiday flag so the UI can style
// computed vs. manual entries.
type dayOffDTO struct {
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	TargetMin int    `json:"targetMin"`
	Holiday   bool   `json:"holiday"`
}

func toDayOffDTO(d domain.DayOff) dayOffDTO {
	return dayOffDTO{
		Day:       d.Date.Format(dayFmt),
		Kind:      string(d.Kind),
		Label:     d.Label,
		TargetMin: int(d.Target / time.Minute),
		Holiday:   d.Kind == domain.KindHoliday,
	}
}

func (s *Server) handleListDayOffs(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	from, to, ok := parseRange(r)
	if !ok {
		http.Error(w, "from/to required (yyyy-mm-dd)", http.StatusBadRequest)
		return
	}
	list, err := s.ListDayOffs.Execute(r.Context(), u.ID, from, to)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	out := make([]dayOffDTO, 0, len(list))
	for _, d := range list {
		out = append(out, toDayOffDTO(d))
	}
	writeJSON(w, http.StatusOK, out)
}

type addDayOffReq struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Kind         string `json:"kind"`
	Label        string `json:"label"`
	TargetMin    int    `json:"targetMin"`
	SkipWeekends bool   `json:"skipWeekends"`
}

func (s *Server) handleAddDayOffs(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req addDayOffReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	from, err1 := time.ParseInLocation(dayFmt, req.From, time.Local)
	to, err2 := time.ParseInLocation(dayFmt, req.To, time.Local)
	if err1 != nil || err2 != nil {
		http.Error(w, "from/to must be yyyy-mm-dd", http.StatusBadRequest)
		return
	}
	kind, ok := domain.ParseKind(req.Kind)
	if !ok {
		http.Error(w, "invalid kind", http.StatusBadRequest)
		return
	}
	err := s.AddDayOffs.Execute(r.Context(), u.ID, from, to, kind,
		req.Label, time.Duration(req.TargetMin)*time.Minute, req.SkipWeekends)
	switch {
	case errors.Is(err, usecase.ErrHolidayNotManual) || errors.Is(err, domain.ErrInvalidDayOff):
		http.Error(w, "invalid day-off", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDeleteDayOff(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	day, err := time.ParseInLocation(dayFmt, r.PathValue("day"), time.Local)
	if err != nil {
		http.Error(w, "day must be yyyy-mm-dd", http.StatusBadRequest)
		return
	}
	if err := s.DeleteDayOff.Execute(r.Context(), u.ID, day); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseRange reads from/to query params (yyyy-mm-dd) in local time.
func parseRange(r *http.Request) (time.Time, time.Time, bool) {
	fs, ts := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	from, err1 := time.ParseInLocation(dayFmt, fs, time.Local)
	to, err2 := time.ParseInLocation(dayFmt, ts, time.Local)
	if err1 != nil || err2 != nil {
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

type settingsDTO struct {
	Bundesland       string         `json:"bundesland"`
	FeedURLs         []string       `json:"feedUrls"`
	DefaultTargetMin int            `json:"defaultTargetMin"`
	WeekdayTargetMin map[string]int `json:"weekdayTargetMin"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	set, toks, err := s.GetSettings.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	urls := make([]string, 0, len(toks))
	for _, t := range toks {
		urls = append(urls, icsURL(r, t.Token))
	}
	wdMin := make(map[string]int, len(set.WeekdayTargetMin))
	for k, v := range set.WeekdayTargetMin {
		wdMin[strconv.Itoa(int(k))] = v
	}
	writeJSON(w, http.StatusOK, settingsDTO{
		Bundesland:       set.Bundesland,
		FeedURLs:         urls,
		DefaultTargetMin: set.DefaultTargetMin,
		WeekdayTargetMin: wdMin,
	})
}

type setTargetReq struct {
	DefaultTargetMin int            `json:"defaultTargetMin"`
	WeekdayTargetMin map[string]int `json:"weekdayTargetMin"`
}

func (s *Server) handleSetTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setTargetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	weekday := make(map[time.Weekday]int, len(req.WeekdayTargetMin))
	for k, v := range req.WeekdayTargetMin {
		n, err := strconv.Atoi(k)
		if err != nil || n < 0 || n > 6 {
			http.Error(w, "weekday key must be 0-6", http.StatusBadRequest)
			return
		}
		weekday[time.Weekday(n)] = v
	}
	err := s.SetTarget.Execute(r.Context(), u.ID, req.DefaultTargetMin, weekday)
	if errors.Is(err, domain.ErrInvalidTarget) {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.Bus.Publish(domain.Event{Type: domain.EventSettingsChanged, UserID: u.ID})
	w.WriteHeader(http.StatusNoContent)
}

type setBundeslandReq struct {
	Bundesland string `json:"bundesland"`
}

func (s *Server) handleSetBundesland(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req setBundeslandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	err := s.SetBundesland.Execute(r.Context(), u.ID, req.Bundesland)
	switch {
	case errors.Is(err, domain.ErrInvalidDayOff):
		http.Error(w, "invalid bundesland", http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type tokenDTO struct {
	Token   string `json:"token"`
	FeedURL string `json:"feedUrl"`
}

func (s *Server) handleRegenIcsToken(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	tok, err := s.RegenIcsToken.Execute(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tokenDTO{Token: tok, FeedURL: icsURL(r, tok)})
}

// icsURL builds the absolute feed URL from the request host. Honors
// X-Forwarded-Proto behind the homelab ingress; defaults to https when set.
func icsURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/ics/" + token + ".ics"
}
