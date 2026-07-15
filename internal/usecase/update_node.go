package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateNodeInput is a PARTIAL update: a nil field is left untouched, a non-nil
// field is applied (a non-nil pointer to "" deliberately clears the field).
// Rate is excluded — see SetNodeRate. syncRemoteBinding only fires when
// UpstreamGit is provided (non-nil) and actually changes, so an update that
// omits UpstreamGit can never delete the node's remote binding.
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
}

// UpdateNode overwrites a project's metadata and keeps the auto-managed
// remote binding in sync with its upstream git (set/clear/repoint).
type UpdateNode struct {
	Nodes    ports.NodeStore
	Bindings ports.ProjectBindingStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc UpdateNode) Execute(ctx context.Context, ownerID, id string, in UpdateNodeInput) (domain.Node, error) {
	cur, err := uc.Nodes.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Node{}, err
	}
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
	if in.CountsTowardTarget != nil {
		p.CountsTowardTarget = in.CountsTowardTarget
	}
	p.UpdatedAt = uc.Clock.Now()
	if err := p.Validate(); err != nil {
		return domain.Node{}, err
	}
	// Pre-validate the upstream so a bad URL rejects the whole update before any
	// write or binding mutation.
	var newSlug string
	if p.UpstreamGit != "" {
		s, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)
		if !ok {
			return domain.Node{}, domain.ErrInvalidUpstream
		}
		newSlug = s
	}
	// origin_slug is the resolution projection of a repo's display/clone URL.
	// Only an explicit upstream update changes it; omitted partial fields retain
	// the stored value, and the database restricts origin_slug to repo nodes.
	if in.UpstreamGit != nil && p.Kind == domain.KindRepo {
		p.OriginSlug = newSlug
	}
	saved, err := uc.Nodes.Update(ctx, ownerID, p)
	if err != nil {
		return domain.Node{}, err
	}
	if cur.UpstreamGit != p.UpstreamGit {
		if err := uc.syncRemoteBinding(ctx, ownerID, saved.ID, cur.UpstreamGit, newSlug); err != nil {
			return domain.Node{}, err
		}
	}
	return saved, nil
}

// syncRemoteBinding drops the previous upstream's remote binding (when it
// changed) and upserts the new one. newSlug == "" means the upstream was cleared.
func (uc UpdateNode) syncRemoteBinding(ctx context.Context, ownerID, nodeID, oldURL, newSlug string) error {
	if oldSlug, ok := domain.NormalizeRemoteSlug(oldURL); ok && oldSlug != newSlug {
		if err := uc.Bindings.DeleteRemote(ctx, ownerID, oldSlug); err != nil {
			return err
		}
	}
	if newSlug == "" {
		return nil
	}
	now := uc.Clock.Now()
	_, err := uc.Bindings.Upsert(ctx, domain.ProjectBinding{
		ID:         uc.IDs.NewID(),
		OwnerID:    ownerID,
		NodeID:     nodeID,
		Kind:       domain.BindingRemote,
		RemoteSlug: newSlug,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return err
}
