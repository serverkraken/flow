package projects

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// ProjectsAPI is the narrow read surface used by the list route. A fake
// implements it in tests; *apiclient.Client satisfies it in production.
type ProjectsAPI interface {
	ListProjects(ctx context.Context) ([]domain.Project, error)
}

var _ ProjectsAPI = (*apiclient.Client)(nil)
