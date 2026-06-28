package usecase

import (
	"context"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StripFrontmatter is an idempotent maintenance operation that removes the
// leading YAML frontmatter block from document bodies. The full parsed
// frontmatter map is preserved verbatim into documents.Extra["frontmatter"]
// so the operation is verlustfrei (reversible). Run with dryRun=true to
// audit without mutating.
type StripFrontmatter struct {
	Docs  ports.DocumentStore
	Clock ports.Clock
}

func (uc StripFrontmatter) Execute(ctx context.Context, ownerID string, dryRun bool) (domain.StripReport, error) {
	docs, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return domain.StripReport{}, err
	}
	var rep domain.StripReport
	for _, d := range docs {
		rep.Scanned++
		fm, bodyStart := domain.ParseFrontmatterMap(d.Body)
		if fm == nil || bodyStart == 0 {
			continue
		}
		rep.Stripped++
		if dryRun {
			continue
		}
		if d.Extra == nil {
			d.Extra = map[string]any{}
		}
		d.Extra["frontmatter"] = fm
		d.Body = strings.TrimLeft(d.Body[bodyStart:], "\n")
		d.UpdatedAt = uc.Clock.Now()
		if _, err := uc.Docs.Update(ctx, d); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
