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

// Peek returns up to limit entries from the queue in FIFO order (seq ASC).
func (wq *WriteQueue) Peek(limit int) ([]ports.WriteQueueEntry, error) {
	rows, err := wq.store.DB().Query(
		`SELECT seq, resource, row_id, payload, expected_version, enqueued_at, last_error
		   FROM write_queue
		  ORDER BY seq ASC
		  LIMIT ?`,
		limit,
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

// SetError records an error message on the entry with the given sequence number.
func (wq *WriteQueue) SetError(seq int64, errMsg string) error {
	_, err := wq.store.DB().Exec(
		`UPDATE write_queue SET last_error = ? WHERE seq = ?`, errMsg, seq,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.WriteQueue.SetError: %w", err)
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
