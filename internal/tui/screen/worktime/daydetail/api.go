package daydetail

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// API is the daydetail route's view of the backend.
type API interface {
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
	AddSession(ctx context.Context, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)
	EditSession(ctx context.Context, id string, projectID *string, tag, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	DeleteSession(ctx context.Context, id string) error
	ListProjects(ctx context.Context) ([]domain.Project, error)
	CreateProject(ctx context.Context, name string) (domain.Project, error)
}
