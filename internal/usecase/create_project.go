package usecase

import (
	"context"
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateProject creates an owner-scoped project. When slug is empty it is
// derived from name.
type CreateProject struct {
	Projects ports.ProjectStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc CreateProject) Execute(ctx context.Context, ownerID, name, slug, color, glyph string) (domain.Project, error) {
	if slug == "" {
		slug = Slugify(name)
	}
	p, err := domain.NewProject(uc.IDs.NewID(), ownerID, name, slug, uc.Clock.Now())
	if err != nil {
		return domain.Project{}, err
	}
	p.Color, p.Glyph = color, glyph
	return uc.Projects.Create(ctx, p)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases name and collapses non-alphanumerics to single hyphens.
func Slugify(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}
