package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateNodeInput is the full mutable field set of a project (rate excluded —
// see SetNodeRate). Update is a full replace: callers send current values.
// CountsTowardTarget is a pointer so omission (nil) preserves the node's
// existing value; only an explicit true/false changes it.
type UpdateNodeInput struct {
	Name               string
	Slug               string
	Color              string
	Glyph              string
	Description        string
	UpstreamGit        string
	Status             domain.NodeStatus
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
	p.Name, p.Slug, p.Color, p.Glyph = in.Name, in.Slug, in.Color, in.Glyph
	p.Description, p.UpstreamGit, p.Status = in.Description, in.UpstreamGit, in.Status
	if in.CountsTowardTarget != nil {
		p.CountsTowardTarget = *in.CountsTowardTarget
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
