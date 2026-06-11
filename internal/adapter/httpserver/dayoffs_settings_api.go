// Package httpserver implements the REST and bearer APIs.
//
// R1 Bearer-API für Day-Offs + User-Settings (Spec §7). Settings sind ein
// flaches key/value-Objekt; nur bekannte Keys werden akzeptiert und
// validiert (Zeitzone muss ladbar, daily_target eine Go-Duration sein).
package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// DayOffsServerStore is the server-side day-off surface (pgstore.DayOffs).
type DayOffsServerStore interface {
	List(userID string, year int) ([]domain.DayOff, error)
	Put(userID string, off domain.DayOff) error
	Delete(userID string, day time.Time) error
}

// SettingsServerStore is the user-settings surface (pgstore.Settings).
type SettingsServerStore interface {
	All(userID string) (map[string]string, error)
	Set(userID, key, value string) error
}

// DayOffsSettingsAPIDeps bundles both small APIs — they share validation
// helpers and always ship together.
type DayOffsSettingsAPIDeps struct {
	DayOffs  DayOffsServerStore
	Settings SettingsServerStore
	Bus      *sse.Broadcaster
}

// MountDayOffsSettingsAPI registers /day-offs and /settings on r.
func MountDayOffsSettingsAPI(r chi.Router, d DayOffsSettingsAPIDeps) {
	r.Get("/day-offs", d.handleDayOffsList)
	r.Put("/day-offs/{date}", d.handleDayOffPut)
	r.Delete("/day-offs/{date}", d.handleDayOffDelete)
	r.Get("/settings", d.handleSettingsGet)
	r.Put("/settings", d.handleSettingsPut)
}

type dayOffDTO struct {
	Day    string `json:"day"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Target string `json:"target,omitempty"` // Go-Duration, z.B. "4h"
}

func validKind(raw string) bool {
	for _, k := range domain.AllKinds {
		if string(k) == raw {
			return true
		}
	}
	return false
}

func (d DayOffsSettingsAPIDeps) handleDayOffsList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil || year < 2000 || year > 2200 {
		apiError(w, http.StatusUnprocessableEntity, "year=YYYY erforderlich")
		return
	}
	items, err := d.DayOffs.List(user.ID, year)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]dayOffDTO, 0, len(items))
	for _, off := range items {
		dto := dayOffDTO{Day: off.Date.Format("2006-01-02"), Kind: string(off.Kind), Label: off.Label}
		if off.Target > 0 {
			dto.Target = off.Target.String()
		}
		dtos = append(dtos, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d DayOffsSettingsAPIDeps) handleDayOffPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	day, err := time.Parse("2006-01-02", chi.URLParam(r, "date"))
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "Datum muss YYYY-MM-DD sein")
		return
	}
	var in dayOffDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	if !validKind(in.Kind) {
		apiError(w, http.StatusUnprocessableEntity, "kind muss holiday|vacation|sick sein")
		return
	}
	var target time.Duration
	if in.Target != "" {
		target, err = time.ParseDuration(in.Target)
		if err != nil || target < 0 {
			apiError(w, http.StatusUnprocessableEntity, "target muss eine Go-Duration sein (z.B. 4h)")
			return
		}
	}
	off := domain.DayOff{Date: day, Kind: domain.Kind(in.Kind), Label: in.Label, Target: target}
	if err := d.DayOffs.Put(user.ID, off); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Bus != nil {
		d.Bus.Changed(user.ID, "dayoffs")
	}
	writeJSON(w, http.StatusOK, in)
}

func (d DayOffsSettingsAPIDeps) handleDayOffDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	day, err := time.Parse("2006-01-02", chi.URLParam(r, "date"))
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "Datum muss YYYY-MM-DD sein")
		return
	}
	if err := d.DayOffs.Delete(user.ID, day); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Bus != nil {
		d.Bus.Changed(user.ID, "dayoffs")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// settingsValidators gates the accepted keys. Unbekannte Keys → 422, damit
// sich keine Tippfehler-Settings ansammeln.
var settingsValidators = map[string]func(string) bool{
	"daily_target": func(v string) bool { d, err := time.ParseDuration(v); return err == nil && d >= 0 },
	"timezone":     func(v string) bool { _, err := time.LoadLocation(v); return err == nil && v != "" },
}

func (d DayOffsSettingsAPIDeps) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	all, err := d.Settings.All(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}

func (d DayOffsSettingsAPIDeps) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in map[string]string
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	for k, v := range in {
		validate, known := settingsValidators[k]
		if !known {
			apiError(w, http.StatusUnprocessableEntity, "unbekannter Settings-Key: "+k)
			return
		}
		if !validate(v) {
			apiError(w, http.StatusUnprocessableEntity, "ungültiger Wert für "+k)
			return
		}
	}
	for k, v := range in {
		if err := d.Settings.Set(user.ID, k, v); err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	all, err := d.Settings.All(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}
