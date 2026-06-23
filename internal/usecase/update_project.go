package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// UpdateProjectInput is the full mutable field set of a project (rate excluded —
// see SetProjectRate). Update is a full replace: callers send current values.
type UpdateProjectInput struct {
	Name        string
	Slug        string
	Color       string
	Glyph       string
	Description string
	UpstreamGit string
	Status      domain.ProjectStatus
}

// UpdateProject overwrites a project's metadata and keeps the auto-managed
// remote binding in sync with its upstream git (set/clear/repoint).
type UpdateProject struct {
	Projects ports.ProjectStore
	Bindings ports.ProjectBindingStore
	IDs      ports.IDGen
	Clock    ports.Clock
}

func (uc UpdateProject) Execute(ctx context.Context, ownerID, id string, in UpdateProjectInput) (domain.Project, error) {
	cur, err := uc.Projects.Get(ctx, ownerID, id)
	if err != nil {
		return domain.Project{}, err
	}
	p := cur
	p.Name, p.Slug, p.Color, p.Glyph = in.Name, in.Slug, in.Color, in.Glyph
	p.Description, p.UpstreamGit, p.Status = in.Description, in.UpstreamGit, in.Status
	p.UpdatedAt = uc.Clock.Now()
	if err := p.Validate(); err != nil {
		return domain.Project{}, err
	}
	// Pre-validate the upstream so a bad URL rejects the whole update before any
	// write or binding mutation.
	var newSlug string
	if p.UpstreamGit != "" {
		s, ok := domain.NormalizeRemoteSlug(p.UpstreamGit)
		if !ok {
			return domain.Project{}, domain.ErrInvalidUpstream
		}
		newSlug = s
	}
	saved, err := uc.Projects.Update(ctx, ownerID, p)
	if err != nil {
		return domain.Project{}, err
	}
	if cur.UpstreamGit != p.UpstreamGit {
		if err := uc.syncRemoteBinding(ctx, ownerID, saved.ID, cur.UpstreamGit, newSlug); err != nil {
			return domain.Project{}, err
		}
	}
	return saved, nil
}

// syncRemoteBinding drops the previous upstream's remote binding (when it
// changed) and upserts the new one. newSlug == "" means the upstream was cleared.
func (uc UpdateProject) syncRemoteBinding(ctx context.Context, ownerID, projectID, oldURL, newSlug string) error {
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
		ProjectID:  projectID,
		Kind:       domain.BindingRemote,
		RemoteSlug: newSlug,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return err
}
