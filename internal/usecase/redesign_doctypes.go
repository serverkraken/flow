package usecase

import (
	"context"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RedesignDocTypes is an idempotent maintenance op that rewrites every legacy
// `agent` document to its new type (spec|plan) and slim path (see
// domain.RedesignedDocType). Run with dryRun=true to audit without mutating.
type RedesignDocTypes struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

func (uc RedesignDocTypes) Execute(ctx context.Context, ownerID string, dryRun bool) (domain.RedesignReport, error) {
	docs, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return domain.RedesignReport{}, err
	}
	var rep domain.RedesignReport
	for _, d := range docs {
		if d.Type != domain.DocAgent {
			continue
		}
		rep.Scanned++
		if dryRun {
			continue
		}
		d.Type, d.Path = domain.RedesignedDocType(d.Path)
		d.UpdatedAt = uc.Clock.Now()
		if _, err := uc.Docs.Update(ctx, d); err != nil {
			return rep, err
		}
		rep.Converted++
	}
	if dryRun {
		rep.Converted = rep.Scanned
	}
	return rep, nil
}
