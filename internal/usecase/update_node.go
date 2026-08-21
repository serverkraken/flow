package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateNodeInput is a PARTIAL update: a nil field is left untouched, a non-nil
// field is applied (a non-nil pointer to "" deliberately clears the field).
// Git identity is projected onto OriginSlug from an explicitly supplied
// UpstreamGit. Explicit bindings are independent aliases/overrides and are
// never mutated by this use case.
type UpdateNodeInput struct {
	Name               *string
	Slug               *string
	Color              *string
	Glyph              *string
	Icon               *string
	Description        *string
	UpstreamGit        *string
	Status             *domain.NodeStatus
	CountsTowardTarget *bool
	// ApplyCountsTowardTarget distinguishes "set inherit" (true + nil) from
	// an omitted partial field. The remaining fields are WebUI aggregate writes.
	ApplyCountsTowardTarget bool
	ApplyRate               bool
	Rate                    *domain.Money
	Tags                    *[]string
	LogoData                []byte
	DeleteLogo              bool
	BannerData              []byte
	DeleteBanner            bool
}

// UpdateNode overwrites a node's metadata. Canonical Git identity lives on the
// repo node itself (OriginSlug / normalized UpstreamGit); explicit bindings
// are managed only through BindNode and UnbindNode.
type UpdateNode struct {
	Nodes     ports.NodeStore
	Aggregate ports.NodeAggregateStore
	Clock     ports.Clock
}

func (uc UpdateNode) Execute(ctx context.Context, ownerID, id string, in UpdateNodeInput) (domain.Node, error) {
	if uc.Aggregate != nil {
		return uc.Aggregate.UpdateAggregate(ctx, ownerID, id, func(cur domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			return applyNodeUpdate(cur, ownerID, in, uc.Clock.Now())
		})
	}
	cur, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err
	}
	p, changes, err := applyNodeUpdate(cur, ownerID, in, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	if changes.SetRate || changes.SetTags || changes.Logo != ports.NodeLogoKeep {
		return domain.Node{}, errors.New("update node aggregate store is not configured")
	}
	return uc.Nodes.Update(ctx, ownerID, p)
}

func applyNodeUpdate(cur domain.Node, ownerID string, in UpdateNodeInput, now time.Time) (domain.Node, ports.NodeAggregateChanges, error) {
	p := cur
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Slug != nil {
		p.Slug = *in.Slug
	}
	if in.Color != nil {
		p.Color = *in.Color
	}
	if in.Glyph != nil {
		p.Glyph = *in.Glyph
	}
	if in.Icon != nil {
		p.Icon = *in.Icon
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.UpstreamGit != nil {
		p.UpstreamGit = *in.UpstreamGit
	}
	if in.Status != nil {
		p.Status = *in.Status
	}
	if in.ApplyCountsTowardTarget || in.CountsTowardTarget != nil {
		p.CountsTowardTarget = in.CountsTowardTarget
	}
	p.UpdatedAt = now
	changes := ports.NodeAggregateChanges{SetRate: in.ApplyRate, Rate: in.Rate}
	if in.ApplyRate {
		if p.Kind != domain.KindEngagement || (in.Rate != nil && (in.Rate.Amount < 0 || len(in.Rate.Currency) != 3)) {
			return domain.Node{}, ports.NodeAggregateChanges{}, domain.ErrInvalidRate
		}
	}
	if in.Tags != nil {
		changes.SetTags = true
		changes.Tags = *in.Tags
	}
	if in.DeleteLogo && len(in.LogoData) > 0 {
		return domain.Node{}, ports.NodeAggregateChanges{}, errors.New("cannot upload and delete a node logo together")
	}
	if in.DeleteLogo {
		p.LogoRef = ""
		changes.Logo = ports.NodeLogoDelete
	} else if len(in.LogoData) > 0 {
		logo, err := buildNodeLogo(ownerID, p.ID, in.LogoData, now)
		if err != nil {
			return domain.Node{}, ports.NodeAggregateChanges{}, err
		}
		p.LogoRef = logo.Ref
		changes.Logo = ports.NodeLogoPut
		changes.LogoValue = logo
	}
	if in.DeleteBanner && len(in.BannerData) > 0 {
		return domain.Node{}, ports.NodeAggregateChanges{}, errors.New("cannot upload and delete a node banner together")
	}
	if in.DeleteBanner {
		p.BannerRef = ""
		changes.Banner = ports.NodeBannerDelete
	} else if len(in.BannerData) > 0 {
		banner, err := buildNodeBanner(ownerID, p.ID, in.BannerData, now)
		if err != nil {
			return domain.Node{}, ports.NodeAggregateChanges{}, err
		}
		p.BannerRef = banner.Ref
		changes.Banner = ports.NodeBannerPut
		changes.BannerValue = banner
	}
	if err := p.Validate(); err != nil {
		return domain.Node{}, ports.NodeAggregateChanges{}, err
	}
	// Pre-validate the upstream so a bad URL rejects the whole update before any
	// write.
	var newSlug string
	if p.UpstreamGit != "" {
		s, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)
		if !ok {
			return domain.Node{}, ports.NodeAggregateChanges{}, domain.ErrInvalidUpstream
		}
		newSlug = s
	}
	// origin_slug is the resolution projection of a repo's display/clone URL.
	// Only an explicit upstream update changes it; omitted partial fields retain
	// the stored value, and the database restricts origin_slug to repo nodes.
	if in.UpstreamGit != nil && p.Kind == domain.KindRepo {
		p.OriginSlug = newSlug
	}
	return p, changes, nil
}
