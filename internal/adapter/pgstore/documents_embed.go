// Package pgstore — embed-worker bookkeeping (stale selection + failure state).
package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// StaleDocuments returns up to limit documents whose chunks are out of date,
// skipping dead-lettered docs and those still inside a backoff window, ordered
// oldest-update-first, each with its prior consecutive failure count.
func (s *DocumentStore) StaleDocuments(ctx context.Context, limit int) ([]ports.StaleDoc, error) {
	q := `SELECT ` + prefixedDocCols + `, coalesce(f.attempts, 0)
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,''))
  AND coalesce(f.dead, false) = false
  AND (f.next_retry_at IS NULL OR f.next_retry_at <= now())
ORDER BY d.updated_at ASC
LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("pgstore: stale documents: %w", err)
	}
	defer rows.Close()
	var out []ports.StaleDoc
	for rows.Next() {
		var d domain.Document
		var typ string
		var extra []byte
		var updatedByKind, updatedByRef sql.NullString
		var attempts int
		if err := rows.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
			&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt,
			&updatedByKind, &updatedByRef, &d.Priority, &attempts); err != nil {
			return nil, fmt.Errorf("pgstore: scan stale document: %w", err)
		}
		d.Type = domain.DocumentType(typ)
		d.UpdatedByKind, d.UpdatedByRef = updatedByKind.String, updatedByRef.String
		if len(extra) > 0 {
			if err := json.Unmarshal(extra, &d.Extra); err != nil {
				return nil, fmt.Errorf("pgstore: unmarshal extra: %w", err)
			}
		}
		out = append(out, ports.StaleDoc{Doc: d, Attempts: attempts})
	}
	return out, rows.Err()
}

func (s *DocumentStore) RecordEmbedFailure(ctx context.Context, docID, ownerID string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO document_embed_failures (document_id, owner_id, attempts, next_retry_at, last_error, dead)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (document_id) DO UPDATE
		   SET attempts = $3, next_retry_at = $4, last_error = $5, dead = $6`,
		docID, ownerID, attempts, nextRetryAt, lastErr, dead)
	if err != nil {
		return fmt.Errorf("pgstore: record embed failure: %w", err)
	}
	return nil
}

func (s *DocumentStore) ClearEmbedFailure(ctx context.Context, docID, ownerID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM document_embed_failures WHERE document_id = $1 AND owner_id = $2`, docID, ownerID)
	if err != nil {
		return fmt.Errorf("pgstore: clear embed failure: %w", err)
	}
	return nil
}

func (s *DocumentStore) EmbedStatus(ctx context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	q := `SELECT
  CASE
    WHEN f.dead THEN 'failed'
    WHEN f.document_id IS NOT NULL THEN 'retrying'
    WHEN d.chunks_hash IS DISTINCT FROM md5(coalesce(d.title,'')||coalesce(d.body,'')) THEN 'pending'
    ELSE 'ok'
  END,
  coalesce(f.attempts, 0), coalesce(f.last_error, ''), f.next_retry_at
FROM documents d
LEFT JOIN document_embed_failures f ON f.document_id = d.id
WHERE d.id = $1 AND d.owner_id = $2`
	var state string
	var st domain.EmbedStatus
	var next *time.Time
	err := s.pool.QueryRow(ctx, q, docID, ownerID).Scan(&state, &st.Attempts, &st.LastError, &next)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EmbedStatus{}, ports.ErrDocumentNotFound
	}
	if err != nil {
		return domain.EmbedStatus{}, fmt.Errorf("pgstore: embed status: %w", err)
	}
	st.State = domain.EmbedState(state)
	st.NextRetry = next
	return st, nil
}
