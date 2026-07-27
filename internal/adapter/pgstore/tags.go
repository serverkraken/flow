package pgstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// TagStore is the Postgres-backed implementation of ports.TagStore.
type TagStore struct {
	pool *pgxpool.Pool
	ids  ports.IDGen
}

// NewTagStore returns a TagStore backed by the given pool and id generator.
func NewTagStore(pool *pgxpool.Pool, ids ports.IDGen) *TagStore {
	return &TagStore{pool: pool, ids: ids}
}

// upsertTag returns the tag id for (owner, slug), creating it with the given
// display string if it does not exist yet. On conflict the display is NOT
// updated (first-write-wins per spec).
func (s *TagStore) upsertTag(ctx context.Context, q pgx.Tx, ownerID, slug, display string) (string, error) {
	id := "tag-" + s.ids.NewID()
	const sql = `INSERT INTO tags (id, owner_id, slug, display)
VALUES ($1,$2,$3,$4)
ON CONFLICT (owner_id, slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id`
	var got string
	err := q.QueryRow(ctx, sql, id, ownerID, slug, display).Scan(&got)
	return got, err
}

// SetTags replaces all tags for the given (ownerID, type, taggableID) triple.
// It iterates raw inputs in order, normalises each to a slug, deduplicates by
// slug within the call, and records the first-seen raw casing as display.
func (s *TagStore) SetTags(ctx context.Context, ownerID string, typ domain.TaggableType, taggableID string, raw []string) ([]domain.Tag, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`,
		string(typ), taggableID, ownerID,
	); err != nil {
		return nil, fmt.Errorf("pgstore: clear taggings: %w", err)
	}

	seen := map[string]bool{}
	var out []domain.Tag
	for _, rawStr := range raw {
		slug, ok := domain.NormalizeTag(rawStr)
		if !ok || seen[slug] {
			continue
		}
		seen[slug] = true
		tagID, err := s.upsertTag(ctx, tx, ownerID, slug, rawStr)
		if err != nil {
			return nil, fmt.Errorf("pgstore: upsert tag: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO taggings (tag_id, taggable_type, taggable_id)
			VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			tagID, string(typ), taggableID,
		); err != nil {
			return nil, fmt.Errorf("pgstore: insert tagging: %w", err)
		}
		out = append(out, domain.Tag{ID: tagID, OwnerID: ownerID, Slug: slug, Display: rawStr})
	}
	return out, tx.Commit(ctx)
}

// TagsFor returns the tags for a single taggable entity.
func (s *TagStore) TagsFor(ctx context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	m, err := s.TagsForMany(ctx, ownerID, typ, []string{id})
	return m[id], err
}

// TagsForMany returns the tags for many taggable entities in a single query.
// The returned map key is the taggable id.
func (s *TagStore) TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error) {
	out := map[string][]domain.Tag{}
	if len(ids) == 0 {
		return out, nil
	}
	const q = `SELECT tg.taggable_id, t.id, t.slug, t.display
FROM taggings tg JOIN tags t ON t.id = tg.tag_id
WHERE t.owner_id=$1 AND tg.taggable_type=$2 AND tg.taggable_id = ANY($3)
ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, string(typ), ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: tags for many: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var tag domain.Tag
		if err := rows.Scan(&tid, &tag.ID, &tag.Slug, &tag.Display); err != nil {
			return nil, err
		}
		tag.OwnerID = ownerID
		out[tid] = append(out[tid], tag)
	}
	return out, rows.Err()
}

// FilterIDs returns the taggable ids that match the given slugs under mode
// (TagMatchAll = AND, TagMatchAny = OR).
func (s *TagStore) FilterIDs(ctx context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error) {
	want := domain.NormalizeTags(slugs)
	if len(want) == 0 {
		return nil, nil
	}
	q := `SELECT tg.taggable_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id
WHERE t.owner_id=$1 AND tg.taggable_type=$2 AND t.slug = ANY($3)
GROUP BY tg.taggable_id`
	if mode == domain.TagMatchAll {
		q += ` HAVING count(DISTINCT t.slug) = cardinality($3::text[])`
	}
	rows, err := s.pool.Query(ctx, q, ownerID, string(typ), want)
	if err != nil {
		return nil, fmt.Errorf("pgstore: filter ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListTags returns tags with usage counts for the given owner, optionally
// narrowed by a taggable type. Results are ordered by count DESC, slug ASC.
func (s *TagStore) ListTags(ctx context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	q := `SELECT t.slug, count(*) AS n
FROM tags t JOIN taggings tg ON tg.tag_id = t.id
WHERE t.owner_id=$1`
	args := []any{ownerID}
	if scope.Type != nil {
		args = append(args, string(*scope.Type))
		q += fmt.Sprintf(` AND tg.taggable_type = $%d`, len(args))
	}
	q += ` GROUP BY t.slug ORDER BY n DESC, t.slug ASC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list tags: %w", err)
	}
	defer rows.Close()
	var out []domain.TagCount
	for rows.Next() {
		var tc domain.TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// ClearTaggable removes all taggings for the given taggable entity.
func (s *TagStore) ClearTaggable(ctx context.Context, ownerID string, typ domain.TaggableType, id string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`,
		string(typ), id, ownerID,
	)
	return err
}

// MergeTags re-points all taggings from fromSlug to intoSlug (creating into
// if needed), then deletes the from tag. Idempotent: no-op if slugs are equal
// or either normalises to empty.
func (s *TagStore) MergeTags(ctx context.Context, ownerID, fromSlug, intoSlug string) error {
	from, _ := domain.NormalizeTag(fromSlug)
	into, _ := domain.NormalizeTag(intoSlug)
	if from == "" || into == "" || from == into {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intoID, err := s.upsertTag(ctx, tx, ownerID, into, into)
	if err != nil {
		return err
	}
	// Re-point taggings from `from` to `into`, skipping conflicts where the
	// taggable already carries `into`.
	if _, err := tx.Exec(ctx,
		`UPDATE taggings SET tag_id=$1
		WHERE tag_id IN (SELECT id FROM tags WHERE owner_id=$2 AND slug=$3)
		AND NOT EXISTS (
			SELECT 1 FROM taggings x
			WHERE x.tag_id=$1
			AND x.taggable_type=taggings.taggable_type
			AND x.taggable_id=taggings.taggable_id
		)`,
		intoID, ownerID, from,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM tags WHERE owner_id=$1 AND slug=$2`,
		ownerID, from,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Ensure TagStore satisfies ports.TagStore at compile time.
var _ ports.TagStore = (*TagStore)(nil)
