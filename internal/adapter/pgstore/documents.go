package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DocumentStore is the Postgres-backed ports.DocumentStore.
type DocumentStore struct {
	pool *pgxpool.Pool
	ids  ports.IDGen
}

// NewDocumentStore wraps a pool and an ID generator.
func NewDocumentStore(pool *pgxpool.Pool, ids ports.IDGen) *DocumentStore {
	return &DocumentStore{pool: pool, ids: ids}
}

const docCols = `id, owner_id, node_id, type, path, title, body, doc_date, role, extra, created_at, updated_at, pinned, archived, archived_at`

const prefixedDocCols = `d.id, d.owner_id, d.node_id, d.type, d.path, d.title, d.body, d.doc_date, d.role, d.extra, d.created_at, d.updated_at, d.pinned, d.archived, d.archived_at`

// appendNodeFilter adds a project predicate to q, binding the next positional
// parameter when needed. nodeID == nil → no filter; *nodeID == "none" →
// IS NULL (unassigned); otherwise equality. col is the (possibly qualified)
// column, e.g. "node_id" or "d.node_id".
func appendNodeFilter(q, col string, args *[]any, nodeID *string) string {
	if nodeID == nil {
		return q
	}
	if *nodeID == "none" {
		return q + ` AND ` + col + ` IS NULL`
	}
	*args = append(*args, *nodeID)
	return q + fmt.Sprintf(` AND %s = $%d`, col, len(*args))
}

// appendTagFilter adds an AND-containment junction subquery for the given tag slugs.
func appendTagFilter(q string, args *[]any, ownerID string, tags []string) string {
	tags = domain.NormalizeTags(tags)
	if len(tags) == 0 {
		return q
	}
	*args = append(*args, ownerID, tags)
	ownPos, tagPos := len(*args)-1, len(*args)
	return q + fmt.Sprintf(` AND id IN (SELECT tg.taggable_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id `+
		`WHERE t.owner_id=$%d AND tg.taggable_type='document' AND t.slug = ANY($%d) `+
		`GROUP BY tg.taggable_id HAVING count(DISTINCT t.slug) = cardinality($%d))`, ownPos, tagPos, tagPos)
}

func (s *DocumentStore) hydrateTags(ctx context.Context, ownerID string, docs []domain.Document) ([]domain.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	const q = `SELECT tg.taggable_id, t.slug FROM taggings tg JOIN tags t ON t.id = tg.tag_id ` +
		`WHERE t.owner_id=$1 AND tg.taggable_type='document' AND tg.taggable_id = ANY($2) ORDER BY t.slug`
	rows, err := s.pool.Query(ctx, q, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: hydrate doc tags: %w", err)
	}
	defer rows.Close()
	byID := map[string][]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return nil, err
		}
		byID[id] = append(byID[id], slug)
	}
	for i := range docs {
		docs[i].Tags = byID[docs[i].ID]
	}
	return docs, rows.Err()
}

func (s *DocumentStore) Create(ctx context.Context, d domain.Document) (domain.Document, error) {
	const q = `INSERT INTO documents (` + docCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.ID, d.OwnerID, d.NodeID, string(d.Type), d.Path, d.Title, d.Body,
		d.Date, d.Role, extra, d.CreatedAt, d.UpdatedAt, d.Pinned, d.Archived, d.ArchivedAt))
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
	if err != nil {
		return domain.Document{}, err
	}
	hyd, err := s.hydrateTags(ctx, ownerID, []domain.Document{d})
	if err != nil {
		return domain.Document{}, err
	}
	return hyd[0], nil
}

func (s *DocumentStore) List(ctx context.Context, ownerID string, nodeID *string, tags ...string) ([]domain.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 AND NOT archived`
	args := []any{ownerID}
	q = appendNodeFilter(q, "node_id", &args, nodeID)
	q = appendTagFilter(q, &args, ownerID, tags)
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list documents: %w", err)
	}
	defer rows.Close()
	docs, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, docs)
}

func (s *DocumentStore) ListPage(ctx context.Context, ownerID string, nodeID *string, limit, offset int, tags ...string) ([]domain.Document, int, error) {
	where := ` WHERE owner_id=$1 AND NOT archived`
	args := []any{ownerID}
	where = appendNodeFilter(where, "node_id", &args, nodeID)
	where = appendTagFilter(where, &args, ownerID, tags)

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
	docs, err = s.hydrateTags(ctx, ownerID, docs)
	if err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

func (s *DocumentStore) Update(ctx context.Context, d domain.Document) (domain.Document, error) {
	// type and path are included so maintenance ops (RedesignDocTypes) can reclassify docs.
	const q = `UPDATE documents SET title=$1, body=$2, extra=$3, updated_at=$4, type=$5, path=$6
WHERE owner_id=$7 AND id=$8
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.Title, d.Body, extra, d.UpdatedAt, string(d.Type), d.Path, d.OwnerID, d.ID))
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

func (s *DocumentStore) SetPinned(ctx context.Context, ownerID, id string, pinned bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET pinned=$1, updated_at=now() WHERE owner_id=$2 AND id=$3`, pinned, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set pinned: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}

func (s *DocumentStore) SetArchived(ctx context.Context, ownerID, id string, archived bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET archived=$1, archived_at = CASE WHEN $1 THEN now() ELSE NULL END, pinned = CASE WHEN $1 THEN false ELSE pinned END, updated_at=now() WHERE owner_id=$2 AND id=$3`, archived, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set archived: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}

func (s *DocumentStore) ListArchived(ctx context.Context, ownerID string) ([]domain.Document, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM documents WHERE owner_id=$1 AND archived ORDER BY archived_at DESC NULLS LAST`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list archived: %w", err)
	}
	defer rows.Close()
	return scanDocuments(rows)
}

func (s *DocumentStore) UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned, archived bool) (string, time.Time, error) {
	id := s.ids.NewID()
	const q = `
INSERT INTO documents (id, owner_id, node_id, type, path, title, body, extra, pinned, archived, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'{}',$8,$9,now(),now())
ON CONFLICT (owner_id, coalesce(node_id, ''), path)
DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body, type = EXCLUDED.type, updated_at = now()
RETURNING id, updated_at`
	var gotID string
	var updated time.Time
	err := s.pool.QueryRow(ctx, q, id, ownerID, nodeID, string(typ), path, title, body, pinned, archived).Scan(&gotID, &updated)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: upsert by path: %w", err)
	}
	return gotID, updated, nil
}

func (s *DocumentStore) ListForContext(ctx context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error) {
	ts := make([]string, len(types))
	for i, t := range types {
		ts[i] = string(t)
	}
	args := []any{ownerID, ts}
	q := `SELECT ` + docCols + ` FROM documents WHERE owner_id=$1 AND type = ANY($2) AND NOT archived`
	switch {
	case len(nodeIDs) > 0 && includeGlobal:
		args = append(args, nodeIDs)
		q += ` AND (node_id = ANY($3) OR node_id IS NULL)`
	case len(nodeIDs) > 0:
		args = append(args, nodeIDs)
		q += ` AND node_id = ANY($3)`
	case includeGlobal:
		q += ` AND node_id IS NULL`
	default:
		return nil, nil
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list for context: %w", err)
	}
	defer rows.Close()
	docs, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, docs)
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

func (s *DocumentStore) Search(ctx context.Context, ownerID, q string, nodeID *string, tags []string) ([]domain.SearchHit, error) {
	sb := `SELECT ` + prefixedDocCols + `,
  ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''), ftsq || pq.prefixq, $3) AS snippet
FROM documents d,
     websearch_to_tsquery('simple', $2) ftsq,
     (SELECT coalesce(
        to_tsquery('simple',
          (SELECT string_agg(w || ':*', ' | ')
           FROM unnest(tsvector_to_array(to_tsvector('simple', $2))) AS w)),
        ''::tsquery)) AS pq(prefixq)
WHERE d.owner_id = $1 AND NOT d.archived`
	args := []any{ownerID, q, headlineOpts}
	sb = appendNodeFilter(sb, "d.node_id", &args, nodeID)
	sb = appendTagFilter(sb, &args, ownerID, tags)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Hydrate tags from taggings for each hit.
	hitDocs := make([]domain.Document, len(out))
	for i, h := range out {
		hitDocs[i] = h.Document
	}
	hitDocs, err = s.hydrateTags(ctx, ownerID, hitDocs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Document = hitDocs[i]
	}
	return out, nil
}

// scanSearchHit scans prefixedDocCols + a trailing snippet column.
func scanSearchHit(r rowScanner) (domain.SearchHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var snippet string
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt, &snippet); err != nil {
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

func (s *DocumentStore) SemanticSearch(ctx context.Context, ownerID string, query []float32, nodeID *string, tags []string, limit int) ([]domain.SemanticHit, error) {
	q := `SELECT ` + prefixedDocCols + `, x.content, x.dist
FROM (
  SELECT DISTINCT ON (c.document_id) c.document_id AS did, c.content AS content,
         (c.embedding <=> $2::vector) AS dist
  FROM document_chunks c
  WHERE c.owner_id = $1
  ORDER BY c.document_id, dist
) x
JOIN documents d ON d.id = x.did AND d.owner_id = $1
WHERE NOT d.archived`
	args := []any{ownerID, vectorLiteral(query)}
	q = appendNodeFilter(q, "d.node_id", &args, nodeID)
	q = appendTagFilter(q, &args, ownerID, tags)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Hydrate tags from taggings for each hit.
	hitDocs := make([]domain.Document, len(out))
	for i, h := range out {
		hitDocs[i] = h.Document
	}
	hitDocs, err = s.hydrateTags(ctx, ownerID, hitDocs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Document = hitDocs[i]
	}
	return out, nil
}

// scanSemanticHit scans prefixedDocCols (same order as scanDocument) + a
// trailing content + dist column.
func scanSemanticHit(r rowScanner) (domain.SemanticHit, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var content string
	var dist float32
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt, &content, &dist); err != nil {
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

func scanDocument(r rowScanner) (domain.Document, error) {
	var d domain.Document
	var typ string
	var extra []byte
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt); err != nil {
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
