package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// CreateNodeInput is the field set for creating a node. Kind+ParentID drive the
// hierarchy validation; a nil ParentID means a root, which must be an engagement.
type CreateNodeInput struct {
	Name, Slug                                   string
	Kind                                         domain.NodeKind
	ParentID                                     *string
	Color, Glyph, Icon, Description, UpstreamGit string
	// CountsTowardTarget overrides the domain default (nil = erbt) when non-nil.
	// Omitting it (nil) preserves NewNode's default of nil (inherit).
	CountsTowardTarget *bool
}

// CreateNode creates an owner-scoped node, validating kind+parent placement.
type CreateNode struct {
	Nodes ports.NodeStore
	IDs   ports.IDGen
	Clock ports.Clock
}

func (uc CreateNode) Execute(ctx context.Context, ownerID string, in CreateNodeInput) (domain.Node, error) {
	slug := in.Slug
	if slug == "" {
		slug = Slugify(in.Name)
	}
	if in.ParentID == nil {
		if in.Kind != domain.KindEngagement {
			return domain.Node{}, fmt.Errorf("%w: root node must be an engagement", domain.ErrInvalidNode)
		}
	} else {
		parent, err := uc.Nodes.Get(ctx, ownerID, *in.ParentID)
		if err != nil {
			return domain.Node{}, err
		}
		if !domain.AllowedChildKind(parent.Kind, in.Kind) {
			return domain.Node{}, fmt.Errorf("%w: %s cannot be a child of %s", domain.ErrInvalidNode, in.Kind, parent.Kind)
		}
	}
	n, err := domain.NewNode(uc.IDs.NewID(), ownerID, in.Name, slug, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	n.Kind = in.Kind
	n.ParentID = in.ParentID
	n.Color, n.Glyph, n.Icon = in.Color, in.Glyph, in.Icon
	n.CountsTowardTarget = in.CountsTowardTarget
	n.Description, n.UpstreamGit = in.Description, in.UpstreamGit
	if err := n.Validate(); err != nil {
		return domain.Node{}, err
	}
	return uc.Nodes.Create(ctx, n)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// deUmlauts transliterates German special letters to their ASCII equivalents so
// "straßenfuchs" slugs to "strassenfuchs" rather than "stra-enfuchs". Applied to
// the already-lowercased name, so only lowercase forms need mapping.
var deUmlauts = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")

// Slugify lowercases name, transliterates German umlauts, then collapses
// non-alphanumerics to single hyphens.
func Slugify(name string) string {
	s := deUmlauts.Replace(strings.ToLower(name))
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
