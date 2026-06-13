package httpapi

import (
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

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

type sessionWriteDTO struct {
	ID        string    `json:"id,omitempty"`
	ProjectID string    `json:"project_id"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at"`
	Tag       string    `json:"tag"`
	Note      string    `json:"note"`
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

type projectDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Slug       string     `json:"slug"`
	ArchivedAt *time.Time `json:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
	Version    int64      `json:"version"`
}

type dayOffDTO struct {
	Day    string `json:"day"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Target string `json:"target,omitempty"`
}

type documentDTO struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

type entryDTO struct {
	Path      string `json:"path"`
	RepoKey   string `json:"repo_key"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Snippet   string `json:"snippet,omitempty"`
}

type metaDTO struct {
	ServerVersion    string `json:"server_version"`
	MinClientVersion string `json:"min_client_version"`
}

type itemsEnvelope[T any] struct {
	Items []T `json:"items"`
}

// — Domain converters ---------------------------------------------------------

func sessionFromDTO(d sessionDTO, userID string) (domain.Session, error) {
	date, err := time.Parse(time.DateOnly, d.Day)
	if err != nil {
		return domain.Session{}, fmt.Errorf("httpapi: session %q: bad day %q: %w", d.ID, d.Day, err)
	}
	elapsed := d.StoppedAt.Sub(d.StartedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	return domain.Session{
		ID:        d.ID,
		UserID:    userID,
		ProjectID: d.ProjectID,
		Date:      date,
		Start:     d.StartedAt,
		Stop:      d.StoppedAt,
		Elapsed:   elapsed,
		Tag:       d.Tag,
		Note:      d.Note,
		Version:   d.Version,
		UpdatedAt: d.UpdatedAt,
	}, nil
}

func activeFromDTO(d activeDTO, userID string) domain.ActiveSession {
	return domain.ActiveSession{
		UserID:          userID,
		ProjectID:       d.ProjectID,
		StartedAt:       d.StartedAt,
		PausedAt:        d.PausedAt,
		PauseTotal:      time.Duration(d.PauseTotalMS) * time.Millisecond,
		StartedOnDevice: d.StartedOnDevice,
		Tag:             d.Tag,
		Note:            d.Note,
		Version:         d.Version,
	}
}

func projectFromDTO(d projectDTO, userID string) domain.Project {
	return domain.Project{
		ID:         d.ID,
		UserID:     userID,
		Name:       d.Name,
		Slug:       d.Slug,
		CreatedAt:  d.CreatedAt,
		LastUsedAt: d.LastUsedAt,
		ArchivedAt: d.ArchivedAt,
		Version:    d.Version,
	}
}

func dayOffFromDTO(d dayOffDTO) (domain.DayOff, error) {
	date, err := time.Parse(time.DateOnly, d.Day)
	if err != nil {
		return domain.DayOff{}, fmt.Errorf("httpapi: dayoff day %q: %w", d.Day, err)
	}
	kind, ok := domain.ParseKind(d.Kind)
	if !ok {
		return domain.DayOff{}, fmt.Errorf("httpapi: unknown dayoff kind %q", d.Kind)
	}
	var target time.Duration
	if d.Target != "" {
		target, err = time.ParseDuration(d.Target)
		if err != nil {
			return domain.DayOff{}, fmt.Errorf("httpapi: dayoff target %q: %w", d.Target, err)
		}
	}
	return domain.DayOff{
		Date:   date,
		Kind:   kind,
		Label:  d.Label,
		Target: target,
	}, nil
}
