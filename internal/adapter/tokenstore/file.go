// Package tokenstore persists the CLI/TUI OAuth token (keyring or 0600 file).
package tokenstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// fileStore keeps the token as a 0600 JSON file. Plaintext on disk is an
// accepted trade-off for the headless fallback (see spec Non-Goals).
type fileStore struct{ path string }

func newFileStore(path string) *fileStore { return &fileStore{path: path} }

type fileToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

func (s *fileStore) Save(t ports.Token) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(fileToken{t.AccessToken, t.RefreshToken, t.Expiry})
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *fileStore) Load() (ports.Token, bool, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return ports.Token{}, false, nil
	}
	if err != nil {
		return ports.Token{}, false, err
	}
	var ft fileToken
	if err := json.Unmarshal(b, &ft); err != nil {
		return ports.Token{}, false, err
	}
	return ports.Token{AccessToken: ft.AccessToken, RefreshToken: ft.RefreshToken, Expiry: ft.Expiry}, true, nil
}

func (s *fileStore) Clear() error {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
