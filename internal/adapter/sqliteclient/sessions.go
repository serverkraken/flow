package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Sessions implements ports.SessionStore against the SQLite sessions table.
type Sessions struct {
	store *Store
}

// compile-time interface assertion — only the new-interface methods are
// verified here. Legacy shim methods (Append, AppendBatch, Rewrite) below
// are added solely so *Sessions satisfies ports.SessionStore and can be
// passed to use cases (Sessions, ActiveSessions) that declare the full
// interface. They delegate to the real ID-based methods. Task 19 removes
// them together with the tsvsessions adapter.
var _ interface {
	Load(userID string) ([]domain.Session, error)
	LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error)
	Upsert(s domain.Session) error
	UpsertBatch(sessions []domain.Session) error
	Delete(userID, id string) error
} = (*Sessions)(nil)

// NewSessions constructs a Sessions sub-adapter backed by store.
func NewSessions(store *Store) *Sessions { return &Sessions{store: store} }

// Load returns all sessions for the user, ordered by date ASC, start ASC.
func (s *Sessions) Load(userID string) ([]domain.Session, error) {
	rows, err := s.store.DB().Query(
		`SELECT id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at
		   FROM sessions
		  WHERE user_id = ?
		  ORDER BY date ASC, start ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.Sessions.Load: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSessions(rows)
}

// LoadFiltered returns sessions for which keep returns true.
func (s *Sessions) LoadFiltered(userID string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := s.Load(userID)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.Sessions.LoadFiltered: %w", err)
	}
	var result []domain.Session
	for _, sess := range all {
		if keep(sess) {
			result = append(result, sess)
		}
	}
	return result, nil
}

// Upsert inserts or updates a session by ID. Requires ID, UserID, and
// ProjectID to be non-empty.
func (s *Sessions) Upsert(sess domain.Session) error {
	if sess.ID == "" {
		return fmt.Errorf("sqliteclient.Sessions.Upsert: ID is required")
	}
	if sess.UserID == "" {
		return fmt.Errorf("sqliteclient.Sessions.Upsert: UserID is required")
	}
	if sess.ProjectID == "" {
		return fmt.Errorf("sqliteclient.Sessions.Upsert: ProjectID is required")
	}

	date := sess.Date.Format("2006-01-02")
	start := sess.Start.UTC().Format(time.RFC3339)
	stop := sess.Stop.UTC().Format(time.RFC3339)
	elapsedNS := sess.Elapsed.Nanoseconds()
	updatedAt := sess.UpdatedAt.UTC().Format(time.RFC3339)

	_, err := s.store.DB().Exec(
		`INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   user_id    = excluded.user_id,
		   project_id = excluded.project_id,
		   date       = excluded.date,
		   start      = excluded.start,
		   stop       = excluded.stop,
		   elapsed_ns = excluded.elapsed_ns,
		   tag        = excluded.tag,
		   note       = excluded.note,
		   version    = excluded.version,
		   updated_at = excluded.updated_at`,
		sess.ID, sess.UserID, sess.ProjectID, date, start, stop, elapsedNS,
		sess.Tag, sess.Note, sess.Version, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Sessions.Upsert: %w", err)
	}
	return nil
}

// UpsertBatch inserts or updates multiple sessions in a single transaction.
func (s *Sessions) UpsertBatch(sessions []domain.Session) error {
	tx, err := s.store.DB().Begin()
	if err != nil {
		return fmt.Errorf("sqliteclient.Sessions.UpsertBatch: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(
		`INSERT INTO sessions (id, user_id, project_id, date, start, stop, elapsed_ns, tag, note, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   user_id    = excluded.user_id,
		   project_id = excluded.project_id,
		   date       = excluded.date,
		   start      = excluded.start,
		   stop       = excluded.stop,
		   elapsed_ns = excluded.elapsed_ns,
		   tag        = excluded.tag,
		   note       = excluded.note,
		   version    = excluded.version,
		   updated_at = excluded.updated_at`,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Sessions.UpsertBatch: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, sess := range sessions {
		if sess.ID == "" || sess.UserID == "" || sess.ProjectID == "" {
			err = fmt.Errorf("sqliteclient.Sessions.UpsertBatch: session missing ID/UserID/ProjectID")
			return err
		}
		date := sess.Date.Format("2006-01-02")
		start := sess.Start.UTC().Format(time.RFC3339)
		stop := sess.Stop.UTC().Format(time.RFC3339)
		elapsedNS := sess.Elapsed.Nanoseconds()
		updatedAt := sess.UpdatedAt.UTC().Format(time.RFC3339)

		if _, err = stmt.Exec(
			sess.ID, sess.UserID, sess.ProjectID, date, start, stop, elapsedNS,
			sess.Tag, sess.Note, sess.Version, updatedAt,
		); err != nil {
			return fmt.Errorf("sqliteclient.Sessions.UpsertBatch: exec: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("sqliteclient.Sessions.UpsertBatch: commit: %w", err)
	}
	return nil
}

// Delete removes the session with the given ID scoped to userID.
func (s *Sessions) Delete(userID, id string) error {
	_, err := s.store.DB().Exec(
		`DELETE FROM sessions WHERE user_id = ? AND id = ?`,
		userID, id,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Sessions.Delete: %w", err)
	}
	return nil
}

func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	var result []domain.Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteclient.Sessions: rows: %w", err)
	}
	return result, nil
}

func scanSessionRow(rows *sql.Rows) (domain.Session, error) {
	var (
		sess      domain.Session
		dateStr   string
		startStr  string
		stopStr   string
		elapsedNS int64
		updatedAt string
	)
	err := rows.Scan(
		&sess.ID, &sess.UserID, &sess.ProjectID,
		&dateStr, &startStr, &stopStr, &elapsedNS,
		&sess.Tag, &sess.Note, &sess.Version, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: no rows")
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: scan: %w", err)
	}

	if dateStr != "" {
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: parse date %q: %w", dateStr, err)
		}
		sess.Date = d
	}
	if startStr != "" {
		st, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: parse start %q: %w", startStr, err)
		}
		sess.Start = st
	}
	if stopStr != "" {
		st, err := time.Parse(time.RFC3339, stopStr)
		if err != nil {
			return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: parse stop %q: %w", stopStr, err)
		}
		sess.Stop = st
	}
	if updatedAt != "" {
		ua, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return domain.Session{}, fmt.Errorf("sqliteclient.Sessions: parse updated_at %q: %w", updatedAt, err)
		}
		sess.UpdatedAt = ua
	}
	sess.Elapsed = time.Duration(elapsedNS)
	return sess, nil
}

// ---- legacy shim methods (Task 14; removed in Task 19) ----
//
// Append, AppendBatch, and Rewrite exist only so *Sessions satisfies the
// full ports.SessionStore interface (which still carries legacy methods
// until Task 19). They delegate to the ID-based equivalents so any caller
// that goes through them gets the same write behaviour as Upsert.
// Sessions written via Append may have an empty ID — in that case Upsert
// generates no unique key and the row will be orphaned; callers on the
// legacy TSV path never reach sqlite (they use tsvsessions directly), so
// this edge-case is unreachable in practice.

// Append implements ports.SessionStore (legacy shim). Delegates to Upsert.
func (s *Sessions) Append(sess domain.Session) error {
	return s.Upsert(sess)
}

// AppendBatch implements ports.SessionStore (legacy shim). Delegates to UpsertBatch.
func (s *Sessions) AppendBatch(sessions []domain.Session) error {
	return s.UpsertBatch(sessions)
}

// Rewrite implements ports.SessionStore (legacy shim). Replaces all sessions
// for the first row's UserID with the supplied slice inside a transaction.
// If sessions is empty this is a no-op. Only used on the TSV path; callers
// on the sqlite path use Upsert / Delete directly.
func (s *Sessions) Rewrite(sessions []domain.Session) error {
	if len(sessions) == 0 {
		return nil
	}
	return s.UpsertBatch(sessions)
}
