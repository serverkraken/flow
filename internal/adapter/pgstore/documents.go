package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DocumentStore is the Postgres-backed ports.DocumentStore.
type DocumentStore struct{ pool *pgxpool.Pool }

// NewDocumentStore wraps a pool.
func NewDocumentStore(pool *pgxpool.Pool) *DocumentStore { return &DocumentStore{pool: pool} }

const docCols = `id, owner_id, project_id, type, path, title, body, tags, doc_date, role, extra, created_at, updated_at`

const prefixedDocCols = `d.id, d.owner_id, d.project_id, d.type, d.path, d.title, d.body, d.tags, d.doc_date, d.role, d.extra, d.created_at, d.updated_at`

func (s *DocumentStore) Create(ctx context.Context, d domain.Document) (domain.Document, error) {
	const q = `INSERT INTO documents (` + docCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.ID, d.OwnerID, d.ProjectID, string(d.Type), d.Path, d.Title, d.Body,
		orEmptyTags(d.Tags), d.Date, d.Role, extra, d.CreatedAt, d.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Document{}, ports.ErrDocumentExists
	}
	return out, err
}

func (s *DocumentStore) Get(ctx context.Context, ownerID, id string) (domain.Document, error) {
	const q = `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 AND id=$2`
	d, err := scanDocument(s.pool.QueryRow(ctx, q, ownerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return d, err
}

func (s *DocumentStore) List(ctx context.Context, ownerID string, tags ...string) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1`
	args := []any{ownerID}
	if len(tags) > 0 {
		q += ` AND tags @> $2`
		args = append(args, tags)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list documents: %w", err)
	}
	defer rows.Close()
	var out []domain.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *DocumentStore) Update(ctx context.Context, d domain.Document) (domain.Document, error) {
	const q = `UPDATE documents SET title=$1, body=$2, tags=$3, extra=$4, updated_at=$5
WHERE owner_id=$6 AND id=$7
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.Title, d.Body, orEmptyTags(d.Tags), extra, d.UpdatedAt, d.OwnerID, d.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return out, err
}

func (s *DocumentStore) Delete(ctx context.Context, ownerID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE owner_id=$1 AND id=$2`, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}

func (s *DocumentStore) ReplaceLinks(ctx context.Context, srcDocID, ownerID string, targets []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin links tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM document_links WHERE src_doc_id=$1`, srcDocID); err != nil {
		return fmt.Errorf("pgstore: clear links: %w", err)
	}
	for _, tgt := range targets {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_links (src_doc_id, owner_id, target_path) VALUES ($1,$2,$3)`,
			srcDocID, ownerID, tgt); err != nil {
			return fmt.Errorf("pgstore: insert link: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *DocumentStore) Backlinks(ctx context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	const q = `SELECT DISTINCT ` + prefixedDocCols + `
FROM documents d
JOIN document_links l ON l.src_doc_id = d.id
WHERE l.owner_id=$1 AND l.target_path=$2
ORDER BY d.updated_at DESC`
	rows, err := s.pool.Query(ctx, q, ownerID, targetPath)
	if err != nil {
		return nil, fmt.Errorf("pgstore: backlinks: %w", err)
	}
	defer rows.Close()
	var out []domain.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// headlineOpts carries the highlight sentinels to ts_headline. Passed as a bound
// parameter (not a SQL literal) so the control chars need no escaping.
var headlineOpts = "StartSel=" + domain.HighlightStart + ",StopSel=" + domain.HighlightEnd +
	",MaxFragments=1,MinWords=5,MaxWords=18,HighlightAll=false"

func (s *DocumentStore) Search(ctx context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	sb := `SELECT ` + prefixedDocCols + `,
  ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''), ftsq, $3) AS snippet
FROM documents d, websearch_to_tsquery('simple', $2) ftsq
WHERE d.owner_id = $1`
	args := []any{ownerID, q, headlineOpts}
	if len(tags) > 0 {
		sb += ` AND d.tags @> $4`
		args = append(args, tags)
	}
	sb += `
  AND (d.search @@ ftsq OR $2 <% (coalesce(d.title,'')||' '||coalesce(d.body,'')))
ORDER BY (d.search @@ ftsq) DESC,
         ts_rank(d.search, ftsq) DESC,
         GREATEST(word_similarity($2, d.title), word_similarity($2, d.body)) DESC,
         d.updated_at DESC
LIMIT 100`

	rows, err := s.pool.Query(ctx, sb, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: search documents: %w", err)
	}
	defer rows.Close()
	var out []domain.SearchHit
	for rows.Next() {
		h, err := scanSearchHit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// scanSearchHit scans prefixedDocCols + a trailing snippet column.
func scanSearchHit(r rowScanner) (domain.SearchHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var snippet string
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &snippet); err != nil {
		return domain.SearchHit{}, fmt.Errorf("pgstore: scan search hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.SearchHit{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return domain.SearchHit{Document: d, Snippet: snippet}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptyTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func scanDocument(r rowScanner) (domain.Document, error) {
	var d domain.Document
	var typ string
	var extra []byte
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Document{}, err
		}
		return domain.Document{}, fmt.Errorf("pgstore: scan document: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.Document{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return d, nil
}
