package httpserver

import (
	"encoding/hex"
	"errors"

	"github.com/gorilla/securecookie"
	"github.com/serverkraken/flow/internal/ports"
)

// Session wraps gorilla/securecookie so handlers can encode/decode arbitrary
// values to a single signed+encrypted cookie blob. The hash key is HMAC, the
// block key is AES.
type Session struct {
	sc *securecookie.SecureCookie
}

func NewSession(hashKey, blockKey []byte) *Session {
	return &Session{sc: securecookie.New(hashKey, blockKey)}
}

func NewSessionFromHex(hashHex, blockHex string) (*Session, error) {
	hashKey, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, errors.New("FLOW_COOKIE_HASH_KEY: invalid hex")
	}
	blockKey, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, errors.New("FLOW_COOKIE_BLOCK_KEY: invalid hex")
	}
	return NewSession(hashKey, blockKey), nil
}

func (s *Session) Encode(name string, value any) (string, error) {
	return s.sc.Encode(name, value)
}

func (s *Session) Decode(name, raw string, out any) error {
	return s.sc.Decode(name, raw, out)
}

// Compile-time assertion.
var _ ports.BrowserSessionStore = (*Session)(nil)
