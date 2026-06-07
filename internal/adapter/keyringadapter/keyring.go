package keyringadapter

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// service is the keyring "service name" under which all flow tokens are
// stored. SlotName from TokenStore (suffixed per field) becomes the
// "account" part. Tokens for one server occupy four entries grouped under
// the same service in macOS Keychain / libsecret / wincred:
//
//	<service=flow>  <account=<slot>.access>
//	<service=flow>  <account=<slot>.refresh>
//	<service=flow>  <account=<slot>.id>
//	<service=flow>  <account=<slot>.expiry>
//
// Splitting across entries is required because go-keyring's macOS backend
// caps each generic-password item at ~2 KiB and modern IdPs (Authentik
// with `groups` + `entitlements` claims, refresh-rotating providers) emit
// JWT bundles well above that limit. The 4-way split fits comfortably:
// access ~1 KiB, refresh ~500 B, id ~1.5 KiB, expiry ~25 B.
const service = "flow"

// Slot suffixes for the four fields of ports.Tokens. The bare slot name
// (without suffix) is intentionally unused — we never store a combined
// blob, so accidental reads of the legacy single-slot return
// ErrTokenNotFound and trigger a clean re-login.
const (
	suffixAccess  = ".access"
	suffixRefresh = ".refresh"
	suffixID      = ".id"
	suffixExpiry  = ".expiry"
)

// Keyring stores ports.Tokens in the OS keychain via zalando/go-keyring,
// one keychain entry per token field. See the package comment for layout.
type Keyring struct{}

// New returns a Keyring backed by the OS keychain via zalando/go-keyring.
func New() *Keyring { return &Keyring{} }

// Get assembles a ports.Tokens from the four per-field keychain entries.
// Returns ports.ErrTokenNotFound if the access-token entry is missing —
// that's the canonical "not logged in" signal. Missing refresh/id/expiry
// degrade gracefully (caller may still use the access token until it
// expires).
func (Keyring) Get(slot string) (ports.Tokens, error) {
	access, err := keyring.Get(service, slot+suffixAccess)
	if errors.Is(err, keyring.ErrNotFound) {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	if err != nil {
		return ports.Tokens{}, err
	}
	refresh, err := getOptional(slot + suffixRefresh)
	if err != nil {
		return ports.Tokens{}, err
	}
	idToken, err := getOptional(slot + suffixID)
	if err != nil {
		return ports.Tokens{}, err
	}
	expiryStr, err := getOptional(slot + suffixExpiry)
	if err != nil {
		return ports.Tokens{}, err
	}
	var expiry time.Time
	if expiryStr != "" {
		expiry, err = time.Parse(time.RFC3339Nano, expiryStr)
		if err != nil {
			return ports.Tokens{}, err
		}
	}
	return ports.Tokens{
		AccessToken:  access,
		RefreshToken: refresh,
		IDToken:      idToken,
		Expiry:       expiry,
	}, nil
}

// Put writes each Tokens field to its own keychain entry. On the first
// write failure, attempts to roll back any entries written so far so a
// half-stored token can't cause silent auth weirdness on the next Get.
func (Keyring) Put(slot string, t ports.Tokens) error {
	writes := []struct {
		suffix string
		value  string
	}{
		{suffixAccess, t.AccessToken},
		{suffixRefresh, t.RefreshToken},
		{suffixID, t.IDToken},
		{suffixExpiry, t.Expiry.Format(time.RFC3339Nano)},
	}
	written := make([]string, 0, len(writes))
	for _, w := range writes {
		if err := keyring.Set(service, slot+w.suffix, w.value); err != nil {
			for _, done := range written {
				_ = keyring.Delete(service, done)
			}
			return err
		}
		written = append(written, slot+w.suffix)
	}
	return nil
}

// Delete removes all four per-field entries for slot. Each suffix is
// best-effort (NotFound is silently swallowed) so partial state from an
// interrupted Put still cleans up.
func (Keyring) Delete(slot string) error {
	var firstErr error
	for _, suffix := range []string{suffixAccess, suffixRefresh, suffixID, suffixExpiry} {
		err := keyring.Delete(service, slot+suffix)
		if errors.Is(err, keyring.ErrNotFound) {
			continue
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// getOptional fetches an entry that may legitimately be absent (refresh
// token rotation can race with reads; legacy slots may pre-date the
// expiry/id-token fields). Returns ("", nil) for NotFound so callers don't
// have to special-case it.
func getOptional(account string) (string, error) {
	v, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}

// Compile-time assertion.
var _ ports.TokenStore = (*Keyring)(nil)
