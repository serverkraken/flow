package usecase

import (
	"crypto/rand"
	"encoding/hex"
)

// newUUID returns a random UUID v4 string using crypto/rand. The usecase layer
// may not import github.com/google/uuid (depguard strict mode); this minimal
// implementation covers the 8-4-4-4-12 hex format with the version/variant
// bits set per RFC 4122 §4.4.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
