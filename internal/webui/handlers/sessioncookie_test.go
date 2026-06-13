package handlers

// sessioncookie_test.go — shared session-cookie encoding helper for
// router-level WebUI tests. Extracted from project_actions_test.go so
// the upcoming Task 14 (and any future router-level test) can reuse
// the same shape without re-declaring the mirror struct.

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpserver"
)

// testSessionValue mirrors httpserver.sessionValue for gob encoding in
// router-level tests. The gob protocol keys on field names + types,
// not on Go type identity, so encoding this exported struct decodes
// cleanly into the unexported original — as long as the shape matches.
//
// If httpserver.sessionValue gains a field (e.g. Device string) the
// gob decode silently ignores extras here. Keep this in sync by hand
// for now; promote to an exported `httpserver.NewTestSession` helper
// if drift becomes a problem.
type testSessionValue struct {
	Sub       string
	Email     string
	Name      string
	ExpiresAt int64
}

// encodeTestSession wraps Session.Encode with a testSessionValue
// populated from the supplied OIDC identity. Returns the cookie value
// ready to be attached to an *http.Cookie.
//
// ttl is the session lifetime relative to time.Now(); use time.Hour
// for typical "still valid" tests.
func encodeTestSession(t *testing.T, sess *httpserver.Session, cookieName, sub, email, name string, ttl time.Duration) string {
	t.Helper()
	val, err := sess.Encode(cookieName, testSessionValue{
		Sub:       sub,
		Email:     email,
		Name:      name,
		ExpiresAt: time.Now().Add(ttl).Unix(),
	})
	if err != nil {
		t.Fatalf("encode session cookie: %v", err)
	}
	return val
}
