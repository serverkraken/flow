package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

// appendProjectFilter adds a project predicate to q, binding the next positional
// parameter when needed. projectID == nil → no filter; *projectID == "none" →
// IS NULL (unassigned); otherwise equality. col is the (possibly qualified)
// column, e.g. "project_id" or "d.project_id".
func appendProjectFilter(q, col string, args *[]any, projectID *string) string {
	if projectID == nil {
		return q
	}
	if *projectID == "none" {
		return q + ` AND ` + col + ` IS NULL`
	}
	*args = append(*args, *projectID)
	return q + fmt.Sprintf(` AND %s = $%d`, col, len(*args))
}

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

func (s *DocumentStore) List(ctx context.Context, ownerID string, projectID *string, tags ...string) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1`
	args := []any{ownerID}
	q = appendProjectFilter(q, "project_id", &args, projectID)
	if len(tags) > 0 {
		args = append(args, tags)
		q += fmt.Sprintf(` AND tags @> $%d`, len(args))
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list documents: %w", err)
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (s *DocumentStore) ListPage(ctx context.Context, ownerID string, projectID *string, limit, offset int, tags ...string) ([]domain.Document, int, error) {
	where := ` WHERE owner_id=$1`
	args := []any{ownerID}
	where = appendProjectFilter(where, "project_id", &args, projectID)
	if len(tags) > 0 {
		args = append(args, tags)
		where += fmt.Sprintf(` AND tags @> $%d`, len(args))
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM documents`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count documents page: %w", err)
	}

	args = append(args, limit, offset)
	q := `SELECT ` + docCols + ` FROM documents` + where +
		fmt.Sprintf(` ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list documents page: %w", err)
	}
	defer rows.Close()
	docs, err := scanDocuments(rows)
	if err != nil {
		return nil, 0, err
	}
	return docs, total, nil
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

func (s *DocumentStore) Search(ctx context.Context, ownerID, q string, projectID *string, tags []string) ([]domain.SearchHit, error) {
	sb := `SELECT ` + prefixedDocCols + `,
  ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''), ftsq || pq.prefixq, $3) AS snippet
FROM documents d,
     websearch_to_tsquery('simple', $2) ftsq,
     (SELECT coalesce(
        to_tsquery('simple',
          (SELECT string_agg(w || ':*', ' | ')
           FROM unnest(tsvector_to_array(to_tsvector('simple', $2))) AS w)),
        ''::tsquery)) AS pq(prefixq)
WHERE d.owner_id = $1`
	args := []any{ownerID, q, headlineOpts}
	sb = appendProjectFilter(sb, "d.project_id", &args, projectID)
	if len(tags) > 0 {
		args = append(args, tags)
		sb += fmt.Sprintf(` AND d.tags @> $%d`, len(args))
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

// vectorLiteral formats a vector as a pgvector text literal ("[1,2,3]") for a
// $N::vector bind — no extra dependency needed.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (s *DocumentStore) ReplaceChunks(ctx context.Context, docID, ownerID string, contents []string, embeddings [][]float32) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: replace chunks begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, docID); err != nil {
		return fmt.Errorf("pgstore: delete chunks: %w", err)
	}
	for i := range contents {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_chunks (document_id, owner_id, chunk_index, content, embedding)
			 VALUES ($1,$2,$3,$4,$5::vector)`,
			docID, ownerID, i, contents[i], vectorLiteral(embeddings[i])); err != nil {
			return fmt.Errorf("pgstore: insert chunk: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE documents SET chunks_hash = md5(coalesce(title,'')||coalesce(body,'')) WHERE id = $1`,
		docID); err != nil {
		return fmt.Errorf("pgstore: stamp chunks_hash: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM document_embed_failures WHERE document_id = $1`, docID); err != nil {
		return fmt.Errorf("pgstore: clear embed failure: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *DocumentStore) SemanticSearch(ctx context.Context, ownerID string, query []float32, projectID *string, tags []string, limit int) ([]domain.SemanticHit, error) {
	q := `SELECT ` + prefixedDocCols + `, x.content, x.dist
FROM (
  SELECT DISTINCT ON (c.document_id) c.document_id AS did, c.content AS content,
         (c.embedding <=> $2::vector) AS dist
  FROM document_chunks c
  WHERE c.owner_id = $1
  ORDER BY c.document_id, dist
) x
JOIN documents d ON d.id = x.did AND d.owner_id = $1`
	args := []any{ownerID, vectorLiteral(query)}
	var preds []string
	if projectID != nil {
		if *projectID == "none" {
			preds = append(preds, "d.project_id IS NULL")
		} else {
			args = append(args, *projectID)
			preds = append(preds, fmt.Sprintf("d.project_id = $%d", len(args)))
		}
	}
	if len(tags) > 0 {
		args = append(args, tags)
		preds = append(preds, fmt.Sprintf("d.tags @> $%d", len(args)))
	}
	if len(preds) > 0 {
		q += "\nWHERE " + strings.Join(preds, " AND ")
	}
	args = append(args, limit)
	q += fmt.Sprintf("\nORDER BY x.dist\nLIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: semantic search: %w", err)
	}
	defer rows.Close()
	var out []domain.SemanticHit
	for rows.Next() {
		h, err := scanSemanticHit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// scanSemanticHit scans prefixedDocCols (same order as scanDocument) + a
// trailing content + dist column.
func scanSemanticHit(r rowScanner) (domain.SemanticHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var content string
	var dist float32
	if err := r.Scan(&d.ID, &d.OwnerID, &d.ProjectID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Tags, &d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &content, &dist); err != nil {
		return domain.SemanticHit{}, fmt.Errorf("pgstore: scan semantic hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &d.Extra); err != nil {
			return domain.SemanticHit{}, fmt.Errorf("pgstore: unmarshal extra: %w", err)
		}
	}
	return domain.SemanticHit{Document: d, Snippet: content, Distance: dist}, nil
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

func scanDocuments(rows pgx.Rows) ([]domain.Document, error) {
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
