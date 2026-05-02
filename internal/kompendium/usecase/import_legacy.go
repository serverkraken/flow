package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ImportLegacy migrates Soenne's tmux-era notes/project-notes files into
// the kompendium notebook. One-shot; idempotent on re-run because target
// IDs that already exist are skipped, not overwritten.
//
// Index is optional — when set, every successfully migrated note is also
// upserted into the FTS5 index so `kompendium search` works immediately
// after the migration without a separate `kompendium index rebuild`.
type ImportLegacy struct {
	Store  ports.NoteStore
	Legacy ports.LegacySource
	Index  ports.Indexer
}

// NewImportLegacy wires the use case with its required ports.
func NewImportLegacy(store ports.NoteStore, legacy ports.LegacySource) *ImportLegacy {
	return &ImportLegacy{Store: store, Legacy: legacy}
}

// ImportLegacyInput points at the source directories.
type ImportLegacyInput struct {
	DailyDir   string
	ProjectDir string
}

// ImportLegacyOutput reports what landed and what was skipped.
type ImportLegacyOutput struct {
	Migrated []domain.ID
	Skipped  []SkipNote
}

// SkipNote names a legacy file that was not migrated, with a human-readable
// reason. Reasons include "already exists at <id>", "no Remote: URL
// extracted", and frontmatter-validation errors.
type SkipNote struct {
	Path   string
	Reason string
}

// Execute reads the legacy directories, writes new notes through the store,
// and returns a report.
func (u *ImportLegacy) Execute(ctx context.Context, in ImportLegacyInput) (ImportLegacyOutput, error) {
	out := ImportLegacyOutput{}

	dailies, err := u.Legacy.ListDailyNotes(ctx, in.DailyDir)
	if err != nil {
		return ImportLegacyOutput{}, fmt.Errorf("list daily: %w", err)
	}
	for _, d := range dailies {
		if err := u.migrateDaily(ctx, d, &out); err != nil {
			return ImportLegacyOutput{}, err
		}
	}

	projects, err := u.Legacy.ListProjectNotes(ctx, in.ProjectDir)
	if err != nil {
		return ImportLegacyOutput{}, fmt.Errorf("list project: %w", err)
	}
	for _, p := range projects {
		if err := u.migrateProject(ctx, p, &out); err != nil {
			return ImportLegacyOutput{}, err
		}
	}

	return out, nil
}

func (u *ImportLegacy) migrateDaily(ctx context.Context, d ports.LegacyDaily, out *ImportLegacyOutput) error {
	id := domain.ID("daily/" + d.Date)
	if skipped, err := u.skipIfExists(ctx, id, d.Path, out); err != nil || skipped {
		return err
	}
	note, err := domain.NewNote(id, domain.Frontmatter{
		ID: id.String(), Type: domain.TypeDaily, Date: d.Date,
	}, d.Body)
	if err != nil {
		out.Skipped = append(out.Skipped, SkipNote{Path: d.Path, Reason: err.Error()})
		return nil
	}
	if err := u.Store.Put(ctx, note); err != nil {
		return fmt.Errorf("put %q: %w", id, err)
	}
	reindex(ctx, u.Store, u.Index, id)
	out.Migrated = append(out.Migrated, id)
	return nil
}

func (u *ImportLegacy) migrateProject(ctx context.Context, p ports.LegacyProject, out *ImportLegacyOutput) error {
	if p.URL == "" {
		out.Skipped = append(out.Skipped, SkipNote{Path: p.Path, Reason: "no Remote: URL extracted"})
		return nil
	}
	canonical := domain.NormalizeURL(p.URL)
	id := domain.ID("projects/" + string(canonical) + "/_project")
	if skipped, err := u.skipIfExists(ctx, id, p.Path, out); err != nil || skipped {
		return err
	}
	note, err := domain.NewNote(id, domain.Frontmatter{
		ID: id.String(), Type: domain.TypeProject, Project: string(canonical),
	}, p.Body)
	if err != nil {
		out.Skipped = append(out.Skipped, SkipNote{Path: p.Path, Reason: err.Error()})
		return nil
	}
	if err := u.Store.Put(ctx, note); err != nil {
		return fmt.Errorf("put %q: %w", id, err)
	}
	reindex(ctx, u.Store, u.Index, id)
	out.Migrated = append(out.Migrated, id)
	return nil
}

func (u *ImportLegacy) skipIfExists(ctx context.Context, id domain.ID, path string, out *ImportLegacyOutput) (bool, error) {
	exists, err := u.Store.Exists(ctx, id)
	if err != nil {
		return false, fmt.Errorf("exists %q: %w", id, err)
	}
	if exists {
		out.Skipped = append(out.Skipped, SkipNote{
			Path:   path,
			Reason: "already exists at " + id.String(),
		})
		return true, nil
	}
	return false, nil
}
