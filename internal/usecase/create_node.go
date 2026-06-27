package usecase

import (
	"context"
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateNode creates an owner-scoped project. When slug is empty it is
// derived from name.
type CreateNode struct {
	Nodes	ports.NodeStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc CreateNode) Execute(ctx context.Context, ownerID, name, slug, color, glyph string) (domain.Node, error) {
	if slug == "" {
		slug = Slugify(name)
	}
	p, err := domain.NewNode(uc.IDs.NewID(), ownerID, name, slug, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	p.Color, p.Glyph = color, glyph
	return uc.Nodes.Create(ctx, p)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases name and collapses non-alphanumerics to single hyphens.
func Slugify(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}
