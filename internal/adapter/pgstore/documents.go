package pgstore

import (
	"context"
	"database/sql"
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

const docCols = `id, owner_id, node_id, type, path, title, body, doc_date, role, extra, created_at, updated_at, pinned, archived, archived_at, updated_by_kind, updated_by_ref, priority, context_mode`

const prefixedDocCols = `d.id, d.owner_id, d.node_id, d.type, d.path, d.title, d.body, d.doc_date, d.role, d.extra, d.created_at, d.updated_at, d.pinned, d.archived, d.archived_at, d.updated_by_kind, d.updated_by_ref, d.priority, d.context_mode`

type documentQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

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

func appendLibraryNodeFilter(q, col string, args *[]any, query ports.DocumentLibraryQuery) string {
	if query.UnassignedOnly {
		return q + ` AND ` + col + ` IS NULL`
	}
	if !query.FilterNodeIDs {
		return q
	}
	if len(query.NodeIDs) == 0 {
		return q + ` AND false`
	}
	*args = append(*args, query.NodeIDs)
	return q + fmt.Sprintf(` AND %s = ANY($%d)`, col, len(*args))
}

func appendLibraryTypeFilter(q, col string, args *[]any, types []domain.DocumentType) string {
	if len(types) == 0 {
		return q
	}
	values := make([]string, len(types))
	for i, typ := range types {
		values[i] = string(typ)
	}
	*args = append(*args, values)
	return q + fmt.Sprintf(` AND %s = ANY($%d)`, col, len(*args))
}

// appendTagFilter adds an AND-containment junction subquery for the given tag slugs.
func appendTagFilter(q string, args *[]any, ownerID string, tags []string) string {
	return appendTagFilterOn(q, "id", args, ownerID, tags)
}

func appendTagFilterOn(q, idCol string, args *[]any, ownerID string, tags []string) string {
	tags = domain.NormalizeTags(tags)
	if len(tags) == 0 {
		return q
	}
	*args = append(*args, ownerID, tags)
	ownPos, tagPos := len(*args)-1, len(*args)
	return q + fmt.Sprintf(` AND %s IN (SELECT tg.taggable_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id `+
		`WHERE t.owner_id=$%d AND tg.taggable_type='document' AND t.slug = ANY($%d) `+
		`GROUP BY tg.taggable_id HAVING count(DISTINCT t.slug) = cardinality($%d))`, idCol, ownPos, tagPos, tagPos)
}

func (s *DocumentStore) hydrateTags(ctx context.Context, ownerID string, docs []domain.Document) ([]domain.Document, error) {
	return s.hydrateTagsWith(ctx, s.pool, ownerID, docs)
}

func (s *DocumentStore) hydrateTagsWith(ctx context.Context, querier documentQuerier, ownerID string, docs []domain.Document) ([]domain.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	const query = `SELECT tg.taggable_id, t.slug FROM taggings tg JOIN tags t ON t.id = tg.tag_id ` +
		`WHERE t.owner_id=$1 AND tg.taggable_type='document' AND tg.taggable_id = ANY($2) ORDER BY t.slug`
	rows, err := querier.Query(ctx, query, ownerID, ids)
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
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
RETURNING ` + docCols
	extra, err := json.Marshal(orEmpty(d.Extra))
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: marshal extra: %w", err)
	}
	out, err := scanDocument(s.pool.QueryRow(ctx, q,
		d.ID, d.OwnerID, d.NodeID, string(d.Type), d.Path, d.Title, d.Body,
		d.Date, d.Role, extra, d.CreatedAt, d.UpdatedAt, d.Pinned, d.Archived, d.ArchivedAt,
		nullIfEmpty(d.UpdatedByKind), nullIfEmpty(d.UpdatedByRef), d.Priority, string(d.ContextMode.OrAuto())))
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

func (s *DocumentStore) ListLibraryPage(ctx context.Context, ownerID string, query ports.DocumentLibraryQuery) (ports.DocumentLibraryPage, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: begin document library read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	if strings.TrimSpace(query.Search) != "" {
		page, err := s.searchLibraryPageTx(ctx, tx, ownerID, query, limit, offset)
		if err != nil {
			return ports.DocumentLibraryPage{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: commit document library search: %w", err)
		}
		return page, nil
	}

	where := ` WHERE d.owner_id=$1`
	args := []any{ownerID}
	where = appendLibraryNodeFilter(where, "d.node_id", &args, query)
	where = appendLibraryTypeFilter(where, "d.type", &args, query.Types)
	where = appendTagFilterOn(where, "d.id", &args, ownerID, query.Tags)

	page := ports.DocumentLibraryPage{}
	countSQL := `SELECT count(*) FILTER (WHERE NOT d.archived), count(*) FILTER (WHERE d.archived) FROM documents d` + where
	if err := tx.QueryRow(ctx, countSQL, args...).Scan(&page.ActiveTotal, &page.ArchivedTotal); err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: count document library: %w", err)
	}

	statusWhere := where
	switch query.Status {
	case ports.DocumentLibraryArchived:
		statusWhere += ` AND d.archived`
		page.Total = page.ArchivedTotal
	case ports.DocumentLibraryAll:
		page.Total = page.ActiveTotal + page.ArchivedTotal
	default:
		statusWhere += ` AND NOT d.archived`
		page.Total = page.ActiveTotal
	}
	if err := s.loadDocumentLibraryFacets(ctx, tx, ownerID, statusWhere, args, &page); err != nil {
		return ports.DocumentLibraryPage{}, err
	}

	pageArgs := append(append([]any(nil), args...), limit, offset)
	order := `d.updated_at DESC, d.id ASC`
	if query.Status == ports.DocumentLibraryArchived {
		order = `d.archived_at DESC NULLS LAST, d.id ASC`
	}
	listSQL := `SELECT ` + prefixedDocCols + ` FROM documents d` + statusWhere +
		fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, order, len(pageArgs)-1, len(pageArgs))
	rows, err := tx.Query(ctx, listSQL, pageArgs...)
	if err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: list document library: %w", err)
	}
	page.Documents, err = scanDocuments(rows)
	rows.Close()
	if err != nil {
		return ports.DocumentLibraryPage{}, err
	}
	page.Documents, err = s.hydrateTagsWith(ctx, tx, ownerID, page.Documents)
	if err != nil {
		return ports.DocumentLibraryPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: commit document library read: %w", err)
	}
	return page, nil
}

func (s *DocumentStore) loadDocumentLibraryFacets(ctx context.Context, tx pgx.Tx, ownerID, statusWhere string, args []any, page *ports.DocumentLibraryPage) error {
	page.TypeTotals = make(map[domain.DocumentType]int)
	rows, err := tx.Query(ctx, `SELECT d.type, count(*) FROM documents d`+statusWhere+` GROUP BY d.type`, args...)
	if err != nil {
		return fmt.Errorf("pgstore: list document library type facets: %w", err)
	}
	for rows.Next() {
		var typ domain.DocumentType
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			rows.Close()
			return err
		}
		page.TypeTotals[typ] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = tx.Query(ctx, `SELECT t.slug, count(DISTINCT d.id)
FROM documents d
JOIN taggings tg ON tg.taggable_id=d.id AND tg.taggable_type='document'
JOIN tags t ON t.id=tg.tag_id AND t.owner_id=$1`+statusWhere+`
GROUP BY t.slug ORDER BY count(DISTINCT d.id) DESC, t.slug ASC`, args...)
	if err != nil {
		return fmt.Errorf("pgstore: list document library tag facets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag domain.TagCount
		if err := rows.Scan(&tag.Tag, &tag.Count); err != nil {
			return err
		}
		page.TagTotals = append(page.TagTotals, tag)
	}
	return rows.Err()
}

const librarySearchCandidateLimit = 200

func (s *DocumentStore) searchLibraryPageTx(ctx context.Context, tx pgx.Tx, ownerID string, query ports.DocumentLibraryQuery, limit, offset int) (ports.DocumentLibraryPage, error) {
	cte, args := documentLibrarySearchCTE(ownerID, query)
	page := ports.DocumentLibraryPage{}
	countSQL := cte + `
SELECT count(*) FILTER (WHERE NOT d.archived), count(*) FILTER (WHERE d.archived)
FROM ranked r JOIN documents d ON d.id=r.id AND d.owner_id=$1`
	if err := tx.QueryRow(ctx, countSQL, args...).Scan(&page.ActiveTotal, &page.ArchivedTotal); err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: count document library search: %w", err)
	}

	statusWhere := ""
	switch query.Status {
	case ports.DocumentLibraryArchived:
		statusWhere = ` WHERE d.archived`
		page.Total = page.ArchivedTotal
	case ports.DocumentLibraryAll:
		page.Total = page.ActiveTotal + page.ArchivedTotal
	default:
		statusWhere = ` WHERE NOT d.archived`
		page.Total = page.ActiveTotal
	}
	if err := s.loadDocumentLibrarySearchFacets(ctx, tx, ownerID, cte, statusWhere, args, &page); err != nil {
		return ports.DocumentLibraryPage{}, err
	}

	pageArgs := append(append([]any(nil), args...), limit, offset)
	listSQL := cte + `
SELECT ` + prefixedDocCols + `, r.snippet
FROM ranked r JOIN documents d ON d.id=r.id AND d.owner_id=$1` + statusWhere +
		fmt.Sprintf(` ORDER BY r.score DESC, d.updated_at DESC, d.id ASC LIMIT $%d OFFSET $%d`, len(pageArgs)-1, len(pageArgs))
	rows, err := tx.Query(ctx, listSQL, pageArgs...)
	if err != nil {
		return ports.DocumentLibraryPage{}, fmt.Errorf("pgstore: list document library search: %w", err)
	}
	for rows.Next() {
		hit, scanErr := scanSearchHit(rows)
		if scanErr != nil {
			rows.Close()
			return ports.DocumentLibraryPage{}, scanErr
		}
		page.Results = append(page.Results, hit)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ports.DocumentLibraryPage{}, err
	}
	hitDocs := make([]domain.Document, len(page.Results))
	for i := range page.Results {
		hitDocs[i] = page.Results[i].Document
	}
	hitDocs, err = s.hydrateTagsWith(ctx, tx, ownerID, hitDocs)
	if err != nil {
		return ports.DocumentLibraryPage{}, err
	}
	for i := range page.Results {
		page.Results[i].Document = hitDocs[i]
	}
	return page, nil
}

func (s *DocumentStore) loadDocumentLibrarySearchFacets(ctx context.Context, tx pgx.Tx, ownerID, cte, statusWhere string, args []any, page *ports.DocumentLibraryPage) error {
	page.TypeTotals = make(map[domain.DocumentType]int)
	rows, err := tx.Query(ctx, cte+`
SELECT d.type, count(*) FROM ranked r
JOIN documents d ON d.id=r.id AND d.owner_id=$1`+statusWhere+` GROUP BY d.type`, args...)
	if err != nil {
		return fmt.Errorf("pgstore: list document library search type facets: %w", err)
	}
	for rows.Next() {
		var typ domain.DocumentType
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			rows.Close()
			return err
		}
		page.TypeTotals[typ] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = tx.Query(ctx, cte+`
SELECT t.slug, count(DISTINCT d.id) FROM ranked r
JOIN documents d ON d.id=r.id AND d.owner_id=$1
JOIN taggings tg ON tg.taggable_id=d.id AND tg.taggable_type='document'
JOIN tags t ON t.id=tg.tag_id AND t.owner_id=$1`+statusWhere+`
GROUP BY t.slug ORDER BY count(DISTINCT d.id) DESC, t.slug ASC`, args...)
	if err != nil {
		return fmt.Errorf("pgstore: list document library search tag facets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag domain.TagCount
		if err := rows.Scan(&tag.Tag, &tag.Count); err != nil {
			return err
		}
		page.TagTotals = append(page.TagTotals, tag)
	}
	return rows.Err()
}

func documentLibrarySearchCTE(ownerID string, query ports.DocumentLibraryQuery) (string, []any) {
	args := []any{ownerID}
	where := ` WHERE d.owner_id=$1`
	where = appendLibraryNodeFilter(where, "d.node_id", &args, query)
	where = appendLibraryTypeFilter(where, "d.type", &args, query.Types)
	where = appendTagFilterOn(where, "d.id", &args, ownerID, query.Tags)
	args = append(args, strings.TrimSpace(query.Search), headlineOpts)
	searchPos, headlinePos := len(args)-1, len(args)

	cte := fmt.Sprintf(`WITH filtered AS (
  SELECT d.id FROM documents d%s
), fts AS (
  SELECT websearch_to_tsquery('simple', $%d) AS ftsq,
         coalesce(to_tsquery('simple',
           (SELECT string_agg(w || ':*', ' | ')
            FROM unnest(tsvector_to_array(to_tsvector('simple', $%d))) AS w)), ''::tsquery) AS prefixq
), keyword_ranked AS (
  SELECT d.id,
         row_number() OVER (ORDER BY (d.search @@ fts.ftsq) DESC,
           ts_rank(d.search, fts.ftsq) DESC,
           GREATEST(word_similarity($%d, d.title), word_similarity($%d, d.body)) DESC,
           d.updated_at DESC, d.id ASC) AS rank,
         ts_headline('simple', coalesce(d.title,'')||' '||coalesce(d.body,''), fts.ftsq || fts.prefixq, $%d) AS snippet
  FROM documents d JOIN filtered f ON f.id=d.id CROSS JOIN fts
  WHERE d.search @@ fts.ftsq OR $%d <%% (coalesce(d.title,'')||' '||coalesce(d.body,''))
  ORDER BY rank
  LIMIT %d
)`, where, searchPos, searchPos, searchPos, searchPos, headlinePos, searchPos, librarySearchCandidateLimit)

	semanticUnion := ""
	if len(query.Embedding) > 0 {
		args = append(args, vectorLiteral(query.Embedding))
		vectorPos := len(args)
		cte += fmt.Sprintf(`, semantic_best AS (
  SELECT DISTINCT ON (c.document_id) c.document_id AS id, c.content AS snippet,
         c.embedding <=> $%d::vector AS distance
  FROM document_chunks c JOIN filtered f ON f.id=c.document_id
  WHERE c.owner_id=$1
  ORDER BY c.document_id, distance
), semantic_ranked AS (
  SELECT id, snippet, row_number() OVER (ORDER BY distance, id ASC) AS rank
  FROM semantic_best
  ORDER BY rank
  LIMIT %d
)`, vectorPos, librarySearchCandidateLimit)
		semanticUnion = `
  UNION ALL
  SELECT id, 1.0/(60+rank), snippet, false FROM semantic_ranked`
	}
	cte += `, candidate_rows AS (
  SELECT id, 1.0/(60+rank) AS score, snippet, true AS keyword FROM keyword_ranked` + semanticUnion + `
), ranked AS (
  SELECT id, sum(score) AS score,
         coalesce(max(snippet) FILTER (WHERE keyword), max(snippet)) AS snippet
  FROM candidate_rows GROUP BY id
)`
	return cte, args
}

func (s *DocumentStore) Update(ctx context.Context, d domain.Document) (domain.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: begin document update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := updateDocumentTx(ctx, tx, d)
	if err != nil {
		return domain.Document{}, err
	}
	if err := clearDocumentEmbedFailureTx(ctx, tx, out.ID, out.OwnerID); err != nil {
		return domain.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: commit document update: %w", err)
	}
	return out, nil
}

func (s *DocumentStore) Move(ctx context.Context, d domain.Document) (domain.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: begin document move: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if d.NodeID != nil && *d.NodeID != "" {
		var owned int
		err = tx.QueryRow(ctx,
			`SELECT 1 FROM nodes WHERE owner_id=$1 AND id=$2 FOR KEY SHARE`,
			d.OwnerID, *d.NodeID,
		).Scan(&owned)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Document{}, ports.ErrNodeNotFound
		}
		if err != nil {
			return domain.Document{}, fmt.Errorf("pgstore: lock document move node: %w", err)
		}
	}

	const q = `UPDATE documents
SET type=$1, node_id=$2, path=$3, doc_date=$4, updated_at=$5, updated_by_kind=$6, updated_by_ref=$7
WHERE owner_id=$8 AND id=$9
RETURNING ` + docCols
	out, err := scanDocument(tx.QueryRow(ctx, q,
		string(d.Type), d.NodeID, d.Path, d.Date, d.UpdatedAt,
		nullIfEmpty(d.UpdatedByKind), nullIfEmpty(d.UpdatedByRef), d.OwnerID, d.ID))
	if isUniqueViolation(err) {
		return domain.Document{}, ports.ErrDocumentExists
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	if err != nil {
		return domain.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Document{}, fmt.Errorf("pgstore: commit document move: %w", err)
	}
	return out, nil
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

// SetPriority sets the manual context-ranking priority (higher = ranked earlier
// within the memory pool). Owner-scoped; deliberately does NOT bump updated_at
// (priority is orthogonal to recency — see domain.Document.Priority).
func (s *DocumentStore) SetPriority(ctx context.Context, ownerID, id string, priority int) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET priority=$1 WHERE owner_id=$2 AND id=$3`, priority, ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set priority: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ports.ErrDocumentNotFound
	}
	return nil
}

func (s *DocumentStore) ReorderPriorities(ctx context.Context, ownerID string, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin reorder priorities: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id FROM documents WHERE owner_id=$1 AND id = ANY($2) ORDER BY id FOR UPDATE`, ownerID, orderedIDs)
	if err != nil {
		return fmt.Errorf("pgstore: lock reorder priorities: %w", err)
	}
	locked := 0
	for rows.Next() {
		locked++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("pgstore: scan reorder priorities: %w", err)
	}
	rows.Close()
	if locked != len(orderedIDs) {
		return ports.ErrDocumentNotFound
	}
	for i, id := range orderedIDs {
		if _, err := tx.Exec(ctx, `UPDATE documents SET priority=$1 WHERE owner_id=$2 AND id=$3`, len(orderedIDs)-i, ownerID, id); err != nil {
			return fmt.Errorf("pgstore: reorder priority: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgstore: commit reorder priorities: %w", err)
	}
	return nil
}

// SetContextMode sets a document's agent-context membership mode. Owner-scoped;
// deliberately does NOT bump updated_at (mode is curation, orthogonal to content
// recency — mirrors SetPriority; see domain.Document.ContextMode / Offene Entsch. #2).
func (s *DocumentStore) SetContextMode(ctx context.Context, ownerID, id string, mode domain.ContextMode) error {
	ct, err := s.pool.Exec(ctx, `UPDATE documents SET context_mode=$1 WHERE owner_id=$2 AND id=$3`, string(mode), ownerID, id)
	if err != nil {
		return fmt.Errorf("pgstore: set context mode: %w", err)
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

func (s *DocumentStore) CurateDocuments(ctx context.Context, ownerID string, mutation ports.DocumentCurationMutation) ([]domain.Document, error) {
	ids := uniqueDocumentIDs(mutation.IDs)
	if len(ids) == 0 || (mutation.Archived == nil) == (mutation.ContextMode == nil) || mutation.At.IsZero() {
		return nil, fmt.Errorf("%w: invalid document curation", domain.ErrInvalidDocument)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("pgstore: begin document curation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT `+docCols+` FROM documents WHERE owner_id=$1 AND id=ANY($2) FOR UPDATE`, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("pgstore: lock document curation: %w", err)
	}
	locked, err := scanDocuments(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(locked) != len(ids) {
		return nil, ports.ErrDocumentNotFound
	}
	byID := make(map[string]domain.Document, len(locked))
	for _, doc := range locked {
		if mutation.ContextMode != nil && !doc.Type.ContextEligible() {
			return nil, fmt.Errorf("%w: document %s is not context eligible", domain.ErrInvalidDocument, doc.ID)
		}
		byID[doc.ID] = doc
	}

	if mutation.Archived != nil {
		_, err = tx.Exec(ctx, `UPDATE documents
SET archived=$1, archived_at=CASE WHEN $1 THEN $2::timestamptz ELSE NULL END,
	    pinned=CASE WHEN $1 THEN false ELSE pinned END, updated_at=$2::timestamptz,
    updated_by_kind=$3, updated_by_ref=$4
WHERE owner_id=$5 AND id=ANY($6)`, *mutation.Archived, mutation.At,
			nullIfEmpty(mutation.ActorKind), nullIfEmpty(mutation.ActorRef), ownerID, ids)
	} else {
		_, err = tx.Exec(ctx, `UPDATE documents SET context_mode=$1, updated_by_kind=$2, updated_by_ref=$3
WHERE owner_id=$4 AND id=ANY($5)`, string(mutation.ContextMode.OrAuto()),
			nullIfEmpty(mutation.ActorKind), nullIfEmpty(mutation.ActorRef), ownerID, ids)
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: update document curation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("pgstore: commit document curation: %w", err)
	}

	out := make([]domain.Document, 0, len(ids))
	for _, id := range ids {
		doc := byID[id]
		doc.UpdatedByKind = mutation.ActorKind
		doc.UpdatedByRef = mutation.ActorRef
		if mutation.Archived != nil {
			doc.Archived = *mutation.Archived
			doc.UpdatedAt = mutation.At
			if *mutation.Archived {
				at := mutation.At
				doc.ArchivedAt = &at
				doc.Pinned = false
			} else {
				doc.ArchivedAt = nil
			}
		} else {
			doc.ContextMode = mutation.ContextMode.OrAuto()
		}
		out = append(out, doc)
	}
	return out, nil
}

func uniqueDocumentIDs(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *DocumentStore) ListArchived(ctx context.Context, ownerID string) ([]domain.Document, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM documents WHERE owner_id=$1 AND archived ORDER BY archived_at DESC NULLS LAST`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: list archived: %w", err)
	}
	defer rows.Close()
	docs, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTags(ctx, ownerID, docs)
}

func (s *DocumentStore) UpsertByPath(ctx context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned, archived bool, updatedByKind, updatedByRef string) (string, time.Time, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: begin upsert by path: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ids.NewID()
	const q = `
INSERT INTO documents (id, owner_id, node_id, type, path, title, body, extra, pinned, archived, created_at, updated_at, updated_by_kind, updated_by_ref)
VALUES ($1,$2,$3,$4,$5,$6,$7,'{}',$8,$9,now(),now(),$10,$11)
ON CONFLICT (owner_id, coalesce(node_id, ''), path)
DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body, type = EXCLUDED.type, updated_at = now(),
              updated_by_kind = EXCLUDED.updated_by_kind, updated_by_ref = EXCLUDED.updated_by_ref
RETURNING id, updated_at`
	var gotID string
	var updated time.Time
	err = tx.QueryRow(ctx, q, id, ownerID, nodeID, string(typ), path, title, body, pinned, archived,
		nullIfEmpty(updatedByKind), nullIfEmpty(updatedByRef)).Scan(&gotID, &updated)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: upsert by path: %w", err)
	}
	if err := clearDocumentEmbedFailureTx(ctx, tx, gotID, ownerID); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf("pgstore: commit upsert by path: %w", err)
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
	var updatedByKind, updatedByRef sql.NullString
	var mode string
	var snippet string
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt,
		&updatedByKind, &updatedByRef, &d.Priority, &mode, &snippet); err != nil {
		return domain.SearchHit{}, fmt.Errorf("pgstore: scan search hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	d.ContextMode = domain.ContextMode(mode)
	d.UpdatedByKind, d.UpdatedByRef = updatedByKind.String, updatedByRef.String
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

func (s *DocumentStore) ReplaceChunks(ctx context.Context, docID, ownerID, snapshotHash string, contents []string, embeddings [][]float32) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: replace chunks begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockEmbedSnapshot(ctx, tx, docID, ownerID, snapshotHash); err != nil {
		return err
	}
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
		`UPDATE documents SET chunks_hash = $3 WHERE id = $1 AND owner_id = $2`,
		docID, ownerID, snapshotHash); err != nil {
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
	var updatedByKind, updatedByRef sql.NullString
	var mode string
	var content string
	var dist float32
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt,
		&updatedByKind, &updatedByRef, &d.Priority, &mode, &content, &dist); err != nil {
		return domain.SemanticHit{}, fmt.Errorf("pgstore: scan semantic hit: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	d.ContextMode = domain.ContextMode(mode)
	d.UpdatedByKind, d.UpdatedByRef = updatedByKind.String, updatedByRef.String
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

// nullIfEmpty converts an empty Go string to a genuine SQL NULL so an unknown
// actor round-trips as NULL (not the empty-string literal) — matching
// pre-L3 rows that never had these columns populated.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanDocument(r rowScanner) (domain.Document, error) {
	var d domain.Document
	var typ string
	var extra []byte
	var updatedByKind, updatedByRef sql.NullString
	var mode string
	if err := r.Scan(&d.ID, &d.OwnerID, &d.NodeID, &typ, &d.Path, &d.Title, &d.Body,
		&d.Date, &d.Role, &extra, &d.CreatedAt, &d.UpdatedAt, &d.Pinned, &d.Archived, &d.ArchivedAt,
		&updatedByKind, &updatedByRef, &d.Priority, &mode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Document{}, err
		}
		return domain.Document{}, fmt.Errorf("pgstore: scan document: %w", err)
	}
	d.Type = domain.DocumentType(typ)
	d.ContextMode = domain.ContextMode(mode)
	d.UpdatedByKind, d.UpdatedByRef = updatedByKind.String, updatedByRef.String
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
