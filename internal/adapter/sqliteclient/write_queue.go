package sqliteclient

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// WriteQueue implements ports.WriteQueue against the SQLite write_queue table.
type WriteQueue struct {
	store *Store
}

// compile-time interface assertion
var _ ports.WriteQueue = (*WriteQueue)(nil)

// NewWriteQueue constructs a WriteQueue sub-adapter backed by store.
func NewWriteQueue(store *Store) *WriteQueue { return &WriteQueue{store: store} }

// Enqueue inserts a new entry into the write queue and returns its sequence number.
func (wq *WriteQueue) Enqueue(resource, rowID string, payload []byte, expectedVersion int64) (int64, error) {
	enqueuedAt := time.Now().UTC().Format(time.RFC3339)
	res, err := wq.store.DB().Exec(
		`INSERT INTO write_queue (resource, row_id, payload, expected_version, enqueued_at)
		 VALUES (?, ?, ?, ?, ?)`,
		resource, rowID, string(payload), expectedVersion, enqueuedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.WriteQueue.Enqueue: %w", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.WriteQueue.Enqueue: last insert id: %w", err)
	}
	return seq, nil
}

// Peek returns up to limit entries from the queue in FIFO order (seq ASC),
// filtering out rows whose next_retry_at is in the future (so the worker
// honours the backoff schedule set by SetRetry).
func (wq *WriteQueue) Peek(limit int) ([]ports.WriteQueueEntry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := wq.store.DB().Query(
		`SELECT seq, resource, row_id, payload, expected_version, enqueued_at, last_error, attempt, next_retry_at
		   FROM write_queue
		  WHERE next_retry_at = '' OR next_retry_at <= ?
		  ORDER BY seq ASC
		  LIMIT ?`,
		now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteclient.WriteQueue.Peek: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWriteQueueEntries(rows)
}

// Remove deletes the entry with the given sequence number.
func (wq *WriteQueue) Remove(seq int64) error {
	_, err := wq.store.DB().Exec(
		`DELETE FROM write_queue WHERE seq = ?`, seq,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.WriteQueue.Remove: %w", err)
	}
	return nil
}

// SetError records an error message on the entry with the given sequence
// number. It does NOT bump the attempt counter or set next_retry_at — use
// SetRetry for transient errors that should be retried.
func (wq *WriteQueue) SetError(seq int64, errMsg string) error {
	_, err := wq.store.DB().Exec(
		`UPDATE write_queue SET last_error = ? WHERE seq = ?`, errMsg, seq,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.WriteQueue.SetError: %w", err)
	}
	return nil
}

// SetRetry records a transient error, bumps the attempt counter, and stamps
// next_retry_at so Peek skips the row until the backoff elapses. The caller
// (httpsync.Worker) computes nextRetryAt from a Backoff instance.
func (wq *WriteQueue) SetRetry(seq int64, errMsg string, nextRetryAt string) error {
	_, err := wq.store.DB().Exec(
		`UPDATE write_queue
		    SET last_error = ?, attempt = attempt + 1, next_retry_at = ?
		  WHERE seq = ?`,
		errMsg, nextRetryAt, seq,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.WriteQueue.SetRetry: %w", err)
	}
	return nil
}

func scanWriteQueueEntries(rows *sql.Rows) ([]ports.WriteQueueEntry, error) {
	var result []ports.WriteQueueEntry
	for rows.Next() {
		var entry ports.WriteQueueEntry
		var payloadStr string
		err := rows.Scan(
			&entry.Seq, &entry.Resource, &entry.RowID, &payloadStr,
			&entry.ExpectedVersion, &entry.EnqueuedAt, &entry.LastError,
			&entry.Attempt, &entry.NextRetryAt,
		)
		if err != nil {
			return nil, fmt.Errorf("sqliteclient.WriteQueue: scan: %w", err)
		}
		entry.Payload = []byte(payloadStr)
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteclient.WriteQueue: rows: %w", err)
	}
	return result, nil
}
