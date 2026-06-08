package oidcclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Claims is the subset of OIDC claims the client needs for its local identity.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ClaimsFromToken decodes the payload segment of a compact JWS WITHOUT verifying
// the signature. The token has already been validated by flow-server; the client
// only reads claims to label its local user. Never use this to make a trust
// decision.
func ClaimsFromToken(raw string) (Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("oidcclient: malformed jwt: want 3 segments, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("oidcclient: decode payload: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, fmt.Errorf("oidcclient: parse claims: %w", err)
	}
	if c.Sub == "" {
		return Claims{}, fmt.Errorf("oidcclient: token has no sub claim")
	}
	return c, nil
}
