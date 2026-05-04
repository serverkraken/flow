package sqliteindex

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Upsert implements ports.Indexer. The metadata row, FTS5 row, and outgoing
// links are replaced atomically.
func (i *Indexer) Upsert(ctx context.Context, note domain.Note, mtime time.Time) error {
	return i.inTx(ctx, func(tx *sql.Tx) error {
		return upsertWithin(ctx, tx, note, mtime)
	})
}

// Delete implements ports.Indexer. Removes the metadata row, FTS5 row, and
// every link involving id (both directions) in one transaction. Missing
// IDs are a no-op so callers can use Delete defensively.
//
// Inbound links (`dst_id = id`) are dropped alongside outbound ones —
// previously only `src_id = id` was deleted, so every reference TO a
// deleted note accumulated forever. BacklinksOf masked the leak via
// LEFT JOIN (returning empty title for the dangling target), but the
// links table grew monotonically until the next Rebuild.
func (i *Indexer) Delete(ctx context.Context, id domain.ID) error {
	return i.inTx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM notes WHERE id = ?`,
			`DELETE FROM notes_fts WHERE id = ?`,
			`DELETE FROM links WHERE src_id = ? OR dst_id = ?`,
		} {
			args := []any{id.String()}
			if q[len(q)-len("? OR dst_id = ?"):] == "? OR dst_id = ?" {
				args = append(args, id.String())
			}
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		}
		return nil
	})
}

// Rebuild implements ports.Indexer. The whole index is replaced in a single
// transaction; on error, the existing index is preserved.
func (i *Indexer) Rebuild(ctx context.Context, all []ports.IndexEntry) error {
	return i.inTx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM notes`,
			`DELETE FROM notes_fts`,
			`DELETE FROM links`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("truncate: %w", err)
			}
		}
		for _, e := range all {
			if err := upsertWithin(ctx, tx, e.Note, e.Mtime); err != nil {
				return err
			}
		}
		return nil
	})
}

// inTx runs fn inside a write transaction. Commit is automatic on success;
// any error from fn rolls back. The deferred Rollback after a successful
// Commit is a no-op (database/sql guarantees this), so it is safe to leave.
//
// Holds the read-side guard against Close so a shutdown during a long
// Rebuild doesn't pull the *sql.DB out from under us mid-transaction.
func (i *Indexer) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	release, err := i.guard()
	if err != nil {
		return err
	}
	defer release()
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func upsertWithin(ctx context.Context, tx *sql.Tx, n domain.Note, mtime time.Time) error {
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO notes (id, type, project, date, title, mtime)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            type = excluded.type,
            project = excluded.project,
            date = excluded.date,
            title = excluded.title,
            mtime = excluded.mtime
    `, n.ID.String(), string(n.Meta.Type), n.Meta.Project, n.Meta.Date, n.Meta.Title, mtime.Unix()); err != nil {
		return fmt.Errorf("upsert notes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM notes_fts WHERE id = ?`, n.ID.String()); err != nil {
		return fmt.Errorf("delete fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO notes_fts (id, title, body) VALUES (?, ?, ?)
    `, n.ID.String(), n.Meta.Title, string(n.Body)); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM links WHERE src_id = ?`, n.ID.String()); err != nil {
		return fmt.Errorf("delete links: %w", err)
	}
	seen := map[string]struct{}{}
	for _, l := range n.Links() {
		if _, dup := seen[l.Target]; dup {
			continue
		}
		seen[l.Target] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO links (src_id, dst_id) VALUES (?, ?)`,
			n.ID.String(), l.Target,
		); err != nil {
			return fmt.Errorf("insert link: %w", err)
		}
	}
	return nil
}
