// Package httpserver implements the REST and bearer APIs.
//
// R1 Bearer-API für Worktime (Spec §7). Ersetzt die alten pull/push-Sync-
// Routen. DTOs sind snake_case-JSON; If-Match trägt die nackte Versions-
// zahl; 412 = Version-Mismatch, 409 = ActiveSession existiert bereits.
package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/sse"
)

// WorktimeSessionsStore is the narrow store surface the worktime API needs.
// Satisfied by *pgstore.Sessions.
type WorktimeSessionsStore interface {
	ListByUserDateRange(userID string, from, to time.Time) ([]domain.Session, error)
	GetByID(userID, id string) (domain.Session, error)
	Upsert(in domain.Session, expectedVersion int64) (domain.Session, error)
	BulkUpsert(sessions []domain.Session) error
	Delete(userID, id string, expectedVersion int64) error
}

// WorktimeActiveStore is the statemachine surface. Satisfied by
// *pgstore.ActiveSessions.
type WorktimeActiveStore interface {
	ListByUser(userID string) ([]domain.ActiveSession, error)
	Get(userID, projectID string) (domain.ActiveSession, error)
	Start(userID, projectID string, startedAt time.Time, device string, expectedVersion int64, tag, note string) (domain.ActiveSession, error)
	Stop(userID, projectID string, expectedVersion int64, tag, note string) (domain.Session, error)
	Pause(userID, projectID string) (domain.ActiveSession, error)
	Resume(userID, projectID string) (domain.ActiveSession, error)
	CorrectStart(userID, projectID string, startedAt time.Time) (domain.ActiveSession, error)
}

// TimezoneResolver liefert die Buchungs-Zeitzone des Users (pgstore.Settings).
type TimezoneResolver interface {
	Location(userID string) *time.Location
}

// ProjectToucher is optional — when set, the active/start handler updates
// last_used_at on the project after a successful session start.
type ProjectToucher interface {
	TouchLastUsed(userID, projectID string) error
}

// WorktimeAPIDeps bundles the worktime API dependencies. Bus is optional
// (nil = no SSE fan-out, e.g. in focused handler tests).
type WorktimeAPIDeps struct {
	Sessions WorktimeSessionsStore
	Active   WorktimeActiveStore
	Settings TimezoneResolver
	Bus      *sse.Broadcaster
	Projects ProjectToucher // optional — updates last_used_at on start
}

func (d WorktimeAPIDeps) changed(userID string) {
	if d.Bus != nil {
		d.Bus.Changed(userID, "worktime")
	}
}

// MountWorktimeAPI registers the §7 worktime routes on r. The caller wraps
// r in the bearer middleware (Task 18).
func MountWorktimeAPI(r chi.Router, d WorktimeAPIDeps) {
	r.Get("/worktime/sessions", d.handleSessionsList)
	r.Post("/worktime/sessions", d.handleSessionCreate)
	r.Post("/worktime/sessions:bulk", d.handleSessionsBulk)
	r.Put("/worktime/sessions/{id}", d.handleSessionPut)
	r.Delete("/worktime/sessions/{id}", d.handleSessionDelete)
	r.Get("/worktime/active", d.handleActiveList)
	r.Post("/worktime/active/start", d.handleActiveStart)
	r.Post("/worktime/active/stop", d.handleActiveStop)
	r.Post("/worktime/active/pause", d.handleActivePause)
	r.Post("/worktime/active/resume", d.handleActiveResume)
	r.Post("/worktime/active/correct", d.handleActiveCorrect)
}

// — DTOs ---------------------------------------------------------------------

type sessionDTO struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Day       string    `json:"day"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toSessionDTO(s domain.Session) sessionDTO {
	return sessionDTO{
		ID: s.ID, ProjectID: s.ProjectID, Day: s.Date.Format("2006-01-02"),
		StartedAt: s.Start, StoppedAt: s.Stop, Tag: s.Tag, Note: s.Note,
		Version: s.Version, UpdatedAt: s.UpdatedAt,
	}
}

type activeDTO struct {
	ProjectID       string     `json:"project_id"`
	StartedAt       time.Time  `json:"started_at"`
	PausedAt        *time.Time `json:"paused_at"`
	PauseTotalMS    int64      `json:"pause_total_ms"`
	StartedOnDevice string     `json:"started_on_device"`
	Tag             string     `json:"tag"`
	Note            string     `json:"note"`
	Version         int64      `json:"version"`
}

func toActiveDTO(a domain.ActiveSession) activeDTO {
	return activeDTO{
		ProjectID: a.ProjectID, StartedAt: a.StartedAt, PausedAt: a.PausedAt,
		PauseTotalMS: a.PauseTotal.Milliseconds(), StartedOnDevice: a.StartedOnDevice,
		Tag: a.Tag, Note: a.Note, Version: a.Version,
	}
}

type sessionWriteDTO struct {
	ID        string    `json:"id"` // nur bulk: Client-UUIDv5; sonst ignoriert
	ProjectID string    `json:"project_id"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
}

type projectIDBody struct {
	ProjectID string `json:"project_id"`
	Tag       string `json:"tag"`
	Note      string `json:"note"`
}

// — Helpers ------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ifMatchVersion parses the bare-integer If-Match header; ok=false when
// the header is absent or not a number (caller answers 422).
func ifMatchVersion(r *http.Request) (int64, bool) {
	v := r.Header.Get("If-Match")
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (d WorktimeAPIDeps) sessionFromWrite(userID string, in sessionWriteDTO) (domain.Session, string) {
	if in.ProjectID == "" {
		return domain.Session{}, "project_id fehlt"
	}
	if in.StartedAt.IsZero() || in.StoppedAt.IsZero() || !in.StoppedAt.After(in.StartedAt) {
		return domain.Session{}, "started_at/stopped_at ungültig (stop muss nach start liegen)"
	}
	loc := time.UTC
	if d.Settings != nil {
		loc = d.Settings.Location(userID)
	}
	return domain.Session{
		ID: in.ID, UserID: userID, ProjectID: in.ProjectID,
		Date:  bookingDayOf(in.StartedAt, loc),
		Start: in.StartedAt.UTC(), Stop: in.StoppedAt.UTC(),
		Tag: in.Tag, Note: in.Note,
	}, ""
}

// bookingDayOf duplicates pgstore.BookingDay's tiny formula to keep the
// adapter dependency direction clean (httpserver kennt pgstore nicht).
func bookingDayOf(startedAt time.Time, loc *time.Location) time.Time {
	l := startedAt.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// — Sessions -----------------------------------------------------------------

func (d WorktimeAPIDeps) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	from, err1 := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	to, err2 := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || to.Before(from) {
		apiError(w, http.StatusUnprocessableEntity, "from/to müssen YYYY-MM-DD sein, to >= from")
		return
	}
	items, err := d.Sessions.ListByUserDateRange(user.ID, from, to)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]sessionDTO, 0, len(items))
	for _, s := range items {
		dtos = append(dtos, toSessionDTO(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d WorktimeAPIDeps) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in sessionWriteDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	sess, problem := d.sessionFromWrite(user.ID, in)
	if problem != "" {
		apiError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	sess.ID = uuid.NewString() // manuelle Session: Server vergibt die ID
	saved, err := d.Sessions.Upsert(sess, 0)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(saved))
}

func (d WorktimeAPIDeps) handleSessionsBulk(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in struct {
		Sessions []sessionWriteDTO `json:"sessions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	batch := make([]domain.Session, 0, len(in.Sessions))
	for i, row := range in.Sessions {
		sess, problem := d.sessionFromWrite(user.ID, row)
		if problem != "" {
			apiError(w, http.StatusUnprocessableEntity,
				"sessions["+strconv.Itoa(i)+"]: "+problem)
			return
		}
		if sess.ID == "" {
			apiError(w, http.StatusUnprocessableEntity,
				"sessions["+strconv.Itoa(i)+"]: id (Client-UUIDv5) fehlt — Import-Idempotenz braucht stabile IDs")
			return
		}
		batch = append(batch, sess)
	}
	if err := d.Sessions.BulkUpsert(batch); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"received": len(batch)})
}

func (d WorktimeAPIDeps) handleSessionPut(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	var in sessionWriteDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiError(w, http.StatusBadRequest, "bad json")
		return
	}
	sess, problem := d.sessionFromWrite(user.ID, in)
	if problem != "" {
		apiError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	sess.ID = id
	saved, err := d.Sessions.Upsert(sess, expected)
	if errors.Is(err, ports.ErrSessionVersionConflict) {
		current, gerr := d.Sessions.GetByID(user.ID, id)
		if errors.Is(gerr, ports.ErrSessionNotFound) {
			apiError(w, http.StatusNotFound, "session existiert nicht")
			return
		}
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toSessionDTO(current)})
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(saved))
}

func (d WorktimeAPIDeps) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	expected, ok := ifMatchVersion(r)
	if !ok {
		apiError(w, http.StatusUnprocessableEntity, "If-Match-Header (Version) fehlt")
		return
	}
	err := d.Sessions.Delete(user.ID, id, expected)
	switch {
	case errors.Is(err, ports.ErrSessionNotFound):
		apiError(w, http.StatusNotFound, "session existiert nicht")
	case errors.Is(err, ports.ErrSessionVersionConflict):
		current, _ := d.Sessions.GetByID(user.ID, id)
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"current": toSessionDTO(current)})
	case err != nil:
		apiError(w, http.StatusInternalServerError, err.Error())
	default:
		d.changed(user.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// — Active -------------------------------------------------------------------

func (d WorktimeAPIDeps) handleActiveList(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	items, err := d.Active.ListByUser(user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dtos := make([]activeDTO, 0, len(items))
	for _, a := range items {
		dtos = append(dtos, toActiveDTO(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dtos})
}

func (d WorktimeAPIDeps) handleActiveStart(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	device := r.Header.Get("X-Flow-Device")
	a, err := d.Active.Start(user.ID, in.ProjectID, time.Time{}, device, 0, in.Tag, in.Note)
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		apiError(w, http.StatusConflict, "für dieses Projekt läuft bereits eine Session")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Projects != nil {
		if terr := d.Projects.TouchLastUsed(user.ID, in.ProjectID); terr != nil {
			// Non-fatal: log but do not fail the request.
			_ = terr
		}
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toActiveDTO(a))
}

func (d WorktimeAPIDeps) handleActiveStop(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	cur, err := d.Active.Get(user.ID, in.ProjectID)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess, err := d.Active.Stop(user.ID, in.ProjectID, cur.Version, in.Tag, in.Note)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if errors.Is(err, ports.ErrActiveSessionConflict) {
		apiError(w, http.StatusConflict, "Zustand hat sich parallel geändert — neu laden")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toSessionDTO(sess))
}

func (d WorktimeAPIDeps) handleActivePause(w http.ResponseWriter, r *http.Request) {
	d.pauseResume(w, r, d.Active.Pause)
}

func (d WorktimeAPIDeps) handleActiveResume(w http.ResponseWriter, r *http.Request) {
	d.pauseResume(w, r, d.Active.Resume)
}

func (d WorktimeAPIDeps) pauseResume(w http.ResponseWriter, r *http.Request, op func(userID, projectID string) (domain.ActiveSession, error)) {
	user, _ := UserFromContext(r.Context())
	var in projectIDBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id fehlt")
		return
	}
	a, err := op(user.ID, in.ProjectID)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toActiveDTO(a))
}

func (d WorktimeAPIDeps) handleActiveCorrect(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	var in struct {
		ProjectID string    `json:"project_id"`
		StartedAt time.Time `json:"started_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProjectID == "" {
		apiError(w, http.StatusUnprocessableEntity, "project_id und started_at erforderlich")
		return
	}
	if in.StartedAt.IsZero() {
		apiError(w, http.StatusUnprocessableEntity, "started_at fehlt")
		return
	}
	if in.StartedAt.After(time.Now().UTC()) {
		apiError(w, http.StatusUnprocessableEntity, "started_at darf nicht in der Zukunft liegen")
		return
	}
	a, err := d.Active.CorrectStart(user.ID, in.ProjectID, in.StartedAt)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		apiError(w, http.StatusNotFound, "keine aktive Session für dieses Projekt")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.changed(user.ID)
	writeJSON(w, http.StatusOK, toActiveDTO(a))
}
