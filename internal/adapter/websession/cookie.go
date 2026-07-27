// Package websession issues and parses the signed browser session cookie.
package websession

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Codec signs/verifies a session value (an HS256 JWT carrying the user id).
type Codec struct {
	secret []byte
	ttl    time.Duration
}

func NewCodec(secret string, ttl time.Duration) *Codec {
	return &Codec{secret: []byte(secret), ttl: ttl}
}

// Issue returns a signed cookie value for userID, valid for the codec's TTL.
func (c *Codec) Issue(userID string) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(c.ttl)),
	})
	s, err := tok.SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("websession: sign: %w", err)
	}
	return s, nil
}

// Parse verifies the value and returns the user id, or an error when the
// signature/expiry is invalid.
func (c *Codec) Parse(raw string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return c.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("websession: parse: %w", err)
	}
	return claims.Subject, nil
}
