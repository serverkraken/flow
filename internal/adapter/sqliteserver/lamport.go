package sqliteserver

import "database/sql"

// NextLamport atomically increments the global counter and returns the new
// value. Must be called inside a transaction by the caller; the helper
// uses the supplied *sql.Tx so the increment + the row-update share one
// commit boundary.
func NextLamport(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(`UPDATE lamport SET counter = counter + 1 WHERE id = 1`); err != nil {
		return 0, err
	}
	var v int64
	if err := tx.QueryRow(`SELECT counter FROM lamport WHERE id = 1`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
