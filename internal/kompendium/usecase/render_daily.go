package usecase

import (
	"context"
	"sort"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// RenderDaily resolves a daily note plus the project notes that share its
// date, so the read view can render the day's project activity inline
// without writing into the daily file itself (cross-link model C — see
// CLAUDE.md section 9).
type RenderDaily struct {
	Store ports.NoteStore
}

// NewRenderDaily returns a RenderDaily using the given store.
func NewRenderDaily(store ports.NoteStore) *RenderDaily {
	return &RenderDaily{Store: store}
}

// RenderDailyInput configures one Execute call.
type RenderDailyInput struct {
	DailyID domain.ID
}

// RenderDailyOutput bundles the daily note with the project notes that share
// its Date.
type RenderDailyOutput struct {
	Daily          domain.Note
	ProjectsForDay []ports.NoteEntry
}

// Execute fetches the daily note and the project notes whose frontmatter
// Date matches it. Project notes are sorted by Project then mtime DESC.
func (u *RenderDaily) Execute(ctx context.Context, in RenderDailyInput) (RenderDailyOutput, error) {
	daily, err := u.Store.Get(ctx, in.DailyID)
	if err != nil {
		return RenderDailyOutput{}, err
	}

	all, err := u.Store.List(ctx, ports.ListFilter{Type: domain.TypeProject})
	if err != nil {
		return RenderDailyOutput{}, err
	}

	var projects []ports.NoteEntry
	if daily.Meta.Date != "" {
		for _, e := range all {
			if e.Meta.Date == daily.Meta.Date {
				projects = append(projects, e)
			}
		}
	}
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].Meta.Project != projects[j].Meta.Project {
			return projects[i].Meta.Project < projects[j].Meta.Project
		}
		return projects[i].Mtime.After(projects[j].Mtime)
	})

	return RenderDailyOutput{Daily: daily, ProjectsForDay: projects}, nil
}
