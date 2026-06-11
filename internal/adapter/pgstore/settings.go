// internal/adapter/pgstore/settings.go
package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Settings stores per-user key/value settings ("daily_target", "timezone").
type Settings struct{ store *Store }

func NewSettings(s *Store) *Settings { return &Settings{store: s} }

// Get returns the value or "" when the key is unset.
func (s *Settings) Get(userID, key string) (string, error) {
	var v string
	err := s.store.Pool().QueryRow(context.Background(),
		`SELECT value FROM user_settings WHERE user_id = $1 AND key = $2`, userID, key).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

func (s *Settings) Set(userID, key, value string) error {
	_, err := s.store.Pool().Exec(context.Background(), `
		INSERT INTO user_settings (user_id, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value`,
		userID, key, value)
	return err
}

// All returns every setting for the user as a map.
func (s *Settings) All(userID string) (map[string]string, error) {
	rows, err := s.store.Pool().Query(context.Background(),
		`SELECT key, value FROM user_settings WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Location resolves the user's booking timezone (Spec §6: day is computed
// in the user's timezone). Unset or unparsable values fall back to
// Europe/Berlin — booking a session must never fail on a bad setting.
func (s *Settings) Location(userID string) *time.Location {
	tz, err := s.Get(userID, "timezone")
	if err != nil || tz == "" {
		return mustBerlin()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return mustBerlin()
	}
	return loc
}

func mustBerlin() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC // tzdata fehlt im Container? UTC ist der letzte Halt.
	}
	return loc
}
