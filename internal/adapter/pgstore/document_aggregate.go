package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func (s *DocumentStore) CreateDocumentAggregate(ctx context.Context, d domain.Document, changes ports.DocumentAggregateChanges) (domain.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: begin document create aggregate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := createDocumentTx(ctx, tx, d)
	if err != nil {
		return domain.Document{}, err
	}
	if err := replaceDocumentLinksTx(ctx, tx, created.ID, created.OwnerID, changes.Links); err != nil {
		return domain.Document{}, err
	}
	if changes.Tags != nil {
		tags, err := s.replaceDocumentTagsTx(ctx, tx, created.OwnerID, created.ID, *changes.Tags)
		if err != nil {
			return domain.Document{}, err
		}
		created.Tags = slugsFromDomainTags(tags)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: commit document create aggregate: %w", err)
	}
	return created, nil
}

func (s *DocumentStore) UpdateDocumentAggregate(ctx context.Context, ownerID, id string, mutate func(domain.Document) (domain.Document, ports.DocumentAggregateChanges, error)) (domain.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: begin document update aggregate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := getDocumentForUpdateTx(ctx, tx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	current.Tags, err = documentTagSlugsTx(ctx, tx, ownerID, id)
	if err != nil {
		return domain.Document{}, err
	}
	next, changes, err := mutate(current)
	if err != nil {
		return domain.Document{}, err
	}
	if next.ID != current.ID || next.OwnerID != current.OwnerID {
		return domain.Document{}, fmt.Errorf("pgstore: document aggregate identity changed")
	}
	updated, err := updateDocumentTx(ctx, tx, next)
	if err != nil {
		return domain.Document{}, err
	}
	if err := clearDocumentEmbedFailureTx(ctx, tx, updated.ID, updated.OwnerID); err != nil {
		return domain.Document{}, err
	}
	if err := replaceDocumentLinksTx(ctx, tx, updated.ID, updated.OwnerID, changes.Links); err != nil {
		return domain.Document{}, err
	}
	if changes.Tags != nil {
		tags, err := s.replaceDocumentTagsTx(ctx, tx, updated.OwnerID, updated.ID, *changes.Tags)
		if err != nil {
			return domain.Document{}, err
		}
		updated.Tags = slugsFromDomainTags(tags)
	} else {
		updated.Tags = current.Tags
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: commit document update aggregate: %w", err)
	}
	return updated, nil
}

func (s *DocumentStore) UpsertDocumentAggregate(ctx context.Context, in ports.DocumentAggregateUpsert) (domain.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: begin document upsert aggregate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
INSERT INTO documents (id, owner_id, node_id, type, path, title, body, extra, pinned, archived, archived_at, created_at, updated_at, updated_by_kind, updated_by_ref)
VALUES ($1,$2,$3,$4,$5,$6,$7,'{}',CASE WHEN $9 THEN false ELSE $8 END,$9,CASE WHEN $9 THEN now() ELSE NULL END,now(),now(),$10,$11)
ON CONFLICT (owner_id, coalesce(node_id, ''), path)
DO UPDATE SET title = EXCLUDED.title,
              body = EXCLUDED.body,
              type = EXCLUDED.type,
              pinned = CASE WHEN EXCLUDED.archived THEN false ELSE EXCLUDED.pinned END,
              archived = EXCLUDED.archived,
              archived_at = CASE
                  WHEN NOT EXCLUDED.archived THEN NULL
                  WHEN NOT documents.archived THEN now()
                  ELSE documents.archived_at
              END,
              updated_at = now(),
              updated_by_kind = EXCLUDED.updated_by_kind,
              updated_by_ref = EXCLUDED.updated_by_ref
RETURNING ` + docCols
	upserted, err := scanDocument(tx.QueryRow(ctx, q,
		s.ids.NewID(), in.OwnerID, in.NodeID, string(in.Type), in.Path, in.Title, in.Body,
		in.Pinned, in.Archived, nullIfEmpty(in.UpdatedByKind), nullIfEmpty(in.UpdatedByRef)))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: upsert document aggregate: %w", err)
	}
	if err := clearDocumentEmbedFailureTx(ctx, tx, upserted.ID, upserted.OwnerID); err != nil {
		return domain.Document{}, err
	}
	if err := replaceDocumentLinksTx(ctx, tx, upserted.ID, upserted.OwnerID, in.Changes.Links); err != nil {
		return domain.Document{}, err
	}
	if in.Changes.Tags != nil {
		tags, err := s.replaceDocumentTagsTx(ctx, tx, upserted.OwnerID, upserted.ID, *in.Changes.Tags)
		if err != nil {
			return domain.Document{}, err
		}
		upserted.Tags = slugsFromDomainTags(tags)
	} else {
		upserted.Tags, err = documentTagSlugsTx(ctx, tx, upserted.OwnerID, upserted.ID)
		if err != nil {
			return domain.Document{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: commit document upsert aggregate: %w", err)
	}
	return upserted, nil
}

func (s *DocumentStore) DeleteDocumentAggregate(ctx context.Context, ownerID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin document delete aggregate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := getDocumentForUpdateTx(ctx, tx, ownerID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		 AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`,
		string(domain.TaggableDocument), id, ownerID); err != nil {
		return fmt.Errorf("pgstore: clear document taggings: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE owner_id=$1 AND id=$2`, ownerID, id); err != nil {
		return fmt.Errorf("pgstore: delete document aggregate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgstore: commit document delete aggregate: %w", err)
	}
	return nil
}

func createDocumentTx(ctx context.Context, tx pgx.Tx, d domain.Document) (domain.Document, error) {
	const q = `INSERT INTO documents (` + docCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(tx.QueryRow(ctx, q,
		d.ID, d.OwnerID, d.NodeID, string(d.Type), d.Path, d.Title, d.Body,
		d.Date, d.Role, extra, d.CreatedAt, d.UpdatedAt, d.Pinned, d.Archived, d.ArchivedAt,
		nullIfEmpty(d.UpdatedByKind), nullIfEmpty(d.UpdatedByRef), d.Priority, string(d.ContextMode.OrAuto())))
	if isUniqueViolation(err) {
		return domain.Document{}, ports.ErrDocumentExists
	}
	return out, err
}

func getDocumentForUpdateTx(ctx context.Context, tx pgx.Tx, ownerID, id string) (domain.Document, error) {
	d, err := scanDocument(tx.QueryRow(ctx,
		`SELECT `+docCols+` FROM documents WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return d, err
}

func updateDocumentTx(ctx context.Context, tx pgx.Tx, d domain.Document) (domain.Document, error) {
	const q = `UPDATE documents SET title=$1, body=$2, extra=$3, updated_at=$4, type=$5, path=$6, updated_by_kind=$7, updated_by_ref=$8
WHERE owner_id=$9 AND id=$10
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(tx.QueryRow(ctx, q,
		d.Title, d.Body, extra, d.UpdatedAt, string(d.Type), d.Path,
		nullIfEmpty(d.UpdatedByKind), nullIfEmpty(d.UpdatedByRef), d.OwnerID, d.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return out, err
}

func replaceDocumentLinksTx(ctx context.Context, tx pgx.Tx, srcDocID, ownerID string, targets []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM document_links WHERE src_doc_id=$1`, srcDocID); err != nil {
		return fmt.Errorf("pgstore: clear links: %w", err)
	}
	for _, target := range targets {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_links (src_doc_id, owner_id, target_path) VALUES ($1,$2,$3)`,
			srcDocID, ownerID, target); err != nil {
			return fmt.Errorf("pgstore: insert link: %w", err)
		}
	}
	return nil
}

func (s *DocumentStore) replaceDocumentTagsTx(ctx context.Context, tx pgx.Tx, ownerID, documentID string, raw []string) ([]domain.Tag, error) {
	if _, err := tx.Exec(ctx,
		`DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
		 AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`,
		string(domain.TaggableDocument), documentID, ownerID); err != nil {
		return nil, fmt.Errorf("pgstore: clear document taggings: %w", err)
	}
	tagStore := TagStore{ids: s.ids}
	seen := map[string]bool{}
	var out []domain.Tag
	for _, rawTag := range raw {
		slug, ok := domain.NormalizeTag(rawTag)
		if !ok || seen[slug] {
			continue
		}
		seen[slug] = true
		tagID, err := tagStore.upsertTag(ctx, tx, ownerID, slug, rawTag)
		if err != nil {
			return nil, fmt.Errorf("pgstore: upsert document tag: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO taggings (tag_id, taggable_type, taggable_id)
			 VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			tagID, string(domain.TaggableDocument), documentID); err != nil {
			return nil, fmt.Errorf("pgstore: insert document tagging: %w", err)
		}
		out = append(out, domain.Tag{ID: tagID, OwnerID: ownerID, Slug: slug, Display: rawTag})
	}
	return out, nil
}

func documentTagSlugsTx(ctx context.Context, tx pgx.Tx, ownerID, documentID string) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT t.slug FROM taggings tg JOIN tags t ON t.id=tg.tag_id
		 WHERE t.owner_id=$1 AND tg.taggable_type=$2 AND tg.taggable_id=$3 ORDER BY t.slug`,
		ownerID, string(domain.TaggableDocument), documentID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: load document tags: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

func slugsFromDomainTags(tags []domain.Tag) []string {
	out := make([]string, len(tags))
	for i := range tags {
		out[i] = tags[i].Slug
	}
	return out
}

var _ ports.DocumentAggregateStore = (*DocumentStore)(nil)
