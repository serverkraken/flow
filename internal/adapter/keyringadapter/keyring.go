package keyringadapter

import (
	"encoding/json"
	"errors"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// service is the keyring "service name" under which all flow tokens are
// stored. SlotName from TokenStore becomes the "account" part. This way the
// macOS Keychain (and equivalents) shows tokens grouped under one entry
// labelled "flow".
const service = "flow"

// Keyring stores ports.Tokens in the OS keychain via zalando/go-keyring.
// Each slot is a separate keychain entry; values are JSON-encoded.
type Keyring struct{}

func New() *Keyring { return &Keyring{} }

func (Keyring) Get(slot string) (ports.Tokens, error) {
	raw, err := keyring.Get(service, slot)
	if errors.Is(err, keyring.ErrNotFound) {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	if err != nil {
		return ports.Tokens{}, err
	}
	var t ports.Tokens
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return ports.Tokens{}, err
	}
	return t, nil
}

func (Keyring) Put(slot string, t ports.Tokens) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(service, slot, string(b))
}

func (Keyring) Delete(slot string) error {
	err := keyring.Delete(service, slot)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Compile-time assertion.
var _ ports.TokenStore = (*Keyring)(nil)
