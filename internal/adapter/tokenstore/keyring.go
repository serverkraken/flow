package tokenstore

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// Fields are stored as separate keyring items because the macOS keyring caps
// each item at ~2 KiB and a single JWT can exceed that.
const (
	keyringService = "flow"
	itemAccess     = "access_token"
	itemRefresh    = "refresh_token"
	itemExpiry     = "expiry"
)

type keyringStore struct{}

func newKeyringStore() *keyringStore { return &keyringStore{} }

func (keyringStore) Save(t ports.Token) error {
	if err := keyring.Set(keyringService, itemAccess, t.AccessToken); err != nil {
		return err
	}
	if err := keyring.Set(keyringService, itemRefresh, t.RefreshToken); err != nil {
		return err
	}
	return keyring.Set(keyringService, itemExpiry, t.Expiry.UTC().Format(time.RFC3339Nano))
}

func (keyringStore) Load() (ports.Token, bool, error) {
	access, err := keyring.Get(keyringService, itemAccess)
	if errors.Is(err, keyring.ErrNotFound) {
		return ports.Token{}, false, nil
	}
	if err != nil {
		return ports.Token{}, false, err
	}
	refresh, err := keyring.Get(keyringService, itemRefresh)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ports.Token{}, false, err
	}
	var expiry time.Time
	if raw, err := keyring.Get(keyringService, itemExpiry); err == nil && raw != "" {
		expiry, _ = time.Parse(time.RFC3339Nano, raw)
	}
	return ports.Token{AccessToken: access, RefreshToken: refresh, Expiry: expiry}, true, nil
}

func (keyringStore) Clear() error {
	for _, item := range []string{itemAccess, itemRefresh, itemExpiry} {
		if err := keyring.Delete(keyringService, item); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return err
		}
	}
	return nil
}
