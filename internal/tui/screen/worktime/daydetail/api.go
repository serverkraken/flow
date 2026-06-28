package daydetail

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// API is the daydetail route's view of the backend.
type API interface {
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
	AddSession(ctx context.Context, nodeID *string, start, stop time.Time, tags []string, note string) (domain.WorkSession, error)
	EditSession(ctx context.Context, id string, nodeID *string, tags []string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error)
	DeleteSession(ctx context.Context, id string) error
	ListNodes(ctx context.Context) ([]domain.Node, error)
	CreateNode(ctx context.Context, in apiclient.CreateNodeFields) (domain.Node, error)
}
