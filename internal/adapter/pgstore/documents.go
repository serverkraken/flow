package pgstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/serverkraken/flow/internal/ports"
)

// Documents implements ports.DocumentStore with Postgres-FTS
// ('simple'-configuration, websearch_to_tsquery — Spec §6).
type Documents struct{ store *Store }

// NewDocuments creates a new Documents store adapter.
func NewDocuments(s *Store) *Documents { return &Documents{store: s} }

var _ ports.DocumentStore = (*Documents)(nil)

const documentCols = `id, user_id, path, body, COALESCE(repo_key, ''), version, updated_at`

const defaultListLimit = 200

// Get retrieves a document by path.
func (d *Documents) Get(userID, path string) (ports.Document, error) {
	row := d.store.Pool().QueryRow(context.Background(),
		`SELECT `+documentCols+` FROM documents WHERE user_id = $1 AND path = $2`,
		userID, path)
	return scanDocument(row)
}

// GetByRepoKey retrieves a document by its repo key.
func (d *Documents) GetByRepoKey(userID, repoKey string) (ports.Document, error) {
	row := d.store.Pool().QueryRow(context.Background(),
		`SELECT `+documentCols+` FROM documents WHERE user_id = $1 AND repo_key = $2`,
		userID, repoKey)
	return scanDocument(row)
}

// List searches and lists documents.
func (d *Documents) List(userID, prefix, query string, limit int) ([]ports.DocumentEntry, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	args := []any{userID}
	conds := []string{"user_id = $1"}
	order := "path ASC"
	snippet := "''"
	if prefix != "" {
		args = append(args, prefix+"%")
		conds = append(conds, fmt.Sprintf("path LIKE $%d", len(args)))
	}
	if query != "" {
		args = append(args, query)
		conds = append(conds, fmt.Sprintf("search @@ websearch_to_tsquery('simple', $%d)", len(args)))
		snippet = fmt.Sprintf("ts_headline('simple', body, websearch_to_tsquery('simple', $%d), 'MaxWords=18,MinWords=8')", len(args))
		order = fmt.Sprintf("ts_rank(search, websearch_to_tsquery('simple', $%d)) DESC, path ASC", len(args))
	}
	args = append(args, limit)
	q := fmt.Sprintf(
		`SELECT path, COALESCE(repo_key, ''), version, updated_at, %s
		 FROM documents WHERE %s ORDER BY %s LIMIT $%d`,
		snippet, strings.Join(conds, " AND "), order, len(args),
	)
	rows, err := d.store.Pool().Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ports.DocumentEntry
	for rows.Next() {
		var e ports.DocumentEntry
		if err := rows.Scan(&e.Path, &e.RepoKey, &e.Version, &e.UpdatedAt, &e.Snippet); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Put upserts a document.
func (d *Documents) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	ctx := context.Background()
	var repoKeyArg *string
	if repoKey != "" {
		repoKeyArg = &repoKey
	}
	if ifMatch == 0 {
		row := d.store.Pool().QueryRow(ctx, `
			INSERT INTO documents (user_id, path, body, repo_key)
			VALUES ($1, $2, $3, $4)
			RETURNING `+documentCols,
			userID, path, body, repoKeyArg)
		doc, err := scanDocument(row)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ports.Document{}, ports.ErrDocumentVersionConflict
		}
		return doc, err
	}
	row := d.store.Pool().QueryRow(ctx, `
		UPDATE documents
		SET body = $3, repo_key = COALESCE($4, repo_key), version = version + 1, updated_at = now()
		WHERE user_id = $1 AND path = $2 AND version = $5
		RETURNING `+documentCols,
		userID, path, body, repoKeyArg, ifMatch)
	doc, err := scanDocument(row)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		return ports.Document{}, ports.ErrDocumentVersionConflict
	}
	return doc, err
}

// Delete deletes a document by path.
func (d *Documents) Delete(userID, path string) error {
	_, err := d.store.Pool().Exec(context.Background(),
		`DELETE FROM documents WHERE user_id = $1 AND path = $2`, userID, path)
	return err
}

func scanDocument(r rowScanner) (ports.Document, error) {
	var out ports.Document
	err := r.Scan(&out.ID, &out.UserID, &out.Path, &out.Body, &out.RepoKey, &out.Version, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.Document{}, ports.ErrDocumentNotFound
	}
	if err != nil {
		return ports.Document{}, err
	}
	return out, nil
}
