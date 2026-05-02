// Package usecase contains kompendium's application logic. Each use case
// orchestrates ports (NoteStore, Indexer, RepoDetector, Editor, Clock) and
// returns domain values. Use cases never import adapter packages directly —
// see CLAUDE.md section 2.1 for the dependency rule.
package usecase

import (
	"context"
	"sort"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ListNotes returns notes in the order the read view wants them: project
// notes for the current repo first (when known), then daily notes, then
// everything else; mtime DESC within each tier.
type ListNotes struct {
	Store ports.NoteStore
}

// NewListNotes returns a ListNotes using the given store.
func NewListNotes(store ports.NoteStore) *ListNotes {
	return &ListNotes{Store: store}
}

// ListNotesInput configures one Execute call.
type ListNotesInput struct {
	// Type filters by note type when non-empty.
	Type domain.NoteType
	// Project filters by canonical project URL when non-empty.
	Project string
	// CurrentRepo, when non-empty, promotes project notes for that repo to
	// the top tier of the result.
	CurrentRepo domain.CanonicalURL
	// Limit caps the number of returned entries; 0 means no limit.
	Limit int
}

// Execute returns the notes from the store, filtered, then re-ordered into
// (current-repo project, daily, other) tiers.
func (u *ListNotes) Execute(ctx context.Context, in ListNotesInput) ([]ports.NoteEntry, error) {
	entries, err := u.Store.List(ctx, ports.ListFilter{
		Type:    in.Type,
		Project: in.Project,
		// Limit is applied after re-ordering so tier boundaries stay coherent.
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ti, tj := tierOf(entries[i], in.CurrentRepo), tierOf(entries[j], in.CurrentRepo)
		if ti != tj {
			return ti < tj
		}
		return entries[i].Mtime.After(entries[j].Mtime)
	})
	if in.Limit > 0 && len(entries) > in.Limit {
		entries = entries[:in.Limit]
	}
	return entries, nil
}

// tierOf returns 0 for project notes of the current repo, 1 for daily notes,
// and 2 for everything else. Lower tiers sort first.
func tierOf(e ports.NoteEntry, currentRepo domain.CanonicalURL) int {
	if currentRepo != "" && e.Meta.Type == domain.TypeProject && e.Meta.Project == string(currentRepo) {
		return 0
	}
	if e.Meta.Type == domain.TypeDaily {
		return 1
	}
	return 2
}
