# flow Rebuild M1b — Device-Flow-Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the manual `FLOW_TOKEN` env paste with a real `flow login` (OIDC Device-Flow, RFC 8628), persisting the token in the OS keyring (file fallback), refreshing silently, so every CLI/TUI command authenticates from stored credentials.

**Architecture:** Client uses `golang.org/x/oauth2` device-flow built-ins (`DeviceAuth`/`DeviceAccessToken`/`TokenSource`) over endpoints discovered via `go-oidc`. The CLI uses a dedicated **public** OAuth client `flow-cli`; the server's verifier is widened to accept **multiple audiences** (web client + CLI client). The `apiclient` is refactored so auth lives in an `http.RoundTripper` (static bearer for CI, refreshing oauth2 transport otherwise), which also fixes SSE auth. Tokens are stored split-field in the keyring (macOS 2 KiB/item limit) or a `0600` JSON file.

**Tech Stack:** Go, `golang.org/x/oauth2 v0.36.0`, `github.com/coreos/go-oidc/v3 v3.18.0`, `github.com/zalando/go-keyring` (new dep), `github.com/golang-jwt/jwt/v5` (tests), Dex (dev OIDC), Cobra.

**Spec:** `docs/superpowers/specs/2026-06-14-flow-rebuild-m1b-device-flow-design.md`

**Module path:** `github.com/serverkraken/flow`

---

## File Structure

**New files:**
- `internal/clientconfig/clientconfig.go` — CLI-side config loader (server URL, issuer, CLI client id)
- `internal/clientconfig/clientconfig_test.go`
- `internal/adapter/tokenstore/file.go` — `0600` JSON token store
- `internal/adapter/tokenstore/file_test.go`
- `internal/adapter/tokenstore/keyring.go` — split-field keyring token store
- `internal/adapter/tokenstore/keyring_test.go`
- `internal/adapter/tokenstore/store.go` — `Open()` picks keyring, falls back to file
- `internal/adapter/tokenstore/store_test.go`
- `internal/adapter/oidcdevice/device.go` — device-flow client (start/poll/refresh)
- `internal/adapter/oidcdevice/device_test.go`
- `cmd/flow/auth.go` — `clientFromStore` + persisting token source
- `cmd/flow/auth_test.go`
- `cmd/flow/login.go` — `flow login`
- `cmd/flow/logout.go` — `flow logout`
- `deploy/dev/flow-cli.env` — dev env for the CLI
- `scripts/smoke-m1b.sh` — server multi-audience smoke

**Modified files:**
- `internal/ports/ports.go` — add `Token` struct + `TokenStore` interface
- `internal/config/config.go` + `internal/config/config_test.go` — add `FLOW_OIDC_CLI_CLIENT_ID`
- `internal/adapter/oidcverify/verifier.go` + `internal/adapter/oidcverify/verifier_test.go` — multi-audience
- `internal/adapter/apiclient/client.go` — transport-based auth + `NewTransport`
- `internal/adapter/apiclient/events.go` — use client transport, drop manual header
- `cmd/flow/main.go` — register `login`/`logout`
- `cmd/flow/whoami.go` — use `clientFromStore`
- `cmd/flow/worktime.go` — use `clientFromStore`
- `cmd/flow-server/main.go` — pass both audiences to verifier
- `deploy/dev/dex.yaml` — add public `flow-cli` client
- `deploy/dev/flow.env` — add `FLOW_OIDC_CLI_CLIENT_ID=flow-cli`
- `Makefile` — `dev-login` target

---

## Task 1: Dev-env — public `flow-cli` client, CLI env, dependency

**Files:**
- Modify: `deploy/dev/dex.yaml`
- Modify: `deploy/dev/flow.env`
- Create: `deploy/dev/flow-cli.env`
- Modify: `Makefile`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the public `flow-cli` client to Dex**

In `deploy/dev/dex.yaml`, after the existing `staticClients:` block (the `flow-dev` entry ending at the `redirectURIs` list), add a second client:

```yaml
  - id: flow-cli
    public: true
    name: flow CLI (dev)
```

(A `public: true` Dex client needs no secret — matches the RFC 8628 public-client requirement.)

- [ ] **Step 2: Add the CLI client id to the server dev env**

In `deploy/dev/flow.env`, add after the `FLOW_OIDC_CLIENT_SECRET=flow-dev-secret` line:

```
# CLI/TUI device-flow uses a separate public client; the server must accept its audience too.
FLOW_OIDC_CLI_CLIENT_ID=flow-cli
```

- [ ] **Step 3: Create the CLI dev env file**

Create `deploy/dev/flow-cli.env`:

```
# Dev config for the flow CLI/TUI (device-flow login). Source before running:
#   set -a; . deploy/dev/flow-cli.env; set +a; go run ./cmd/flow login
# Pairs with deploy/dev/dex.yaml (public client flow-cli). NOT for production.
FLOW_SERVER_URL=http://localhost:8080
FLOW_OIDC_ISSUER=http://localhost:5556/dex
FLOW_OIDC_CLI_CLIENT_ID=flow-cli
```

- [ ] **Step 4: Add a `dev-login` Makefile target**

In `Makefile`, add `dev-login` to the `.PHONY` line, then add the target near `dev-run`:

```makefile
dev-login:
	set -a; . deploy/dev/flow-cli.env; set +a; go run ./cmd/flow login
```

- [ ] **Step 5: Add the go-keyring dependency**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go get github.com/zalando/go-keyring@latest`
Expected: `go.mod` gains `github.com/zalando/go-keyring`, `go.sum` updated, no error.

- [ ] **Step 6: Restart Dex and verify device-flow + refresh capability empirically**

Run (restart so the new client loads, then probe):

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
make dev-down && make dev-up
# device endpoint accepts the public flow-cli client (no secret):
curl -s -d "client_id=flow-cli" --data-urlencode "scope=openid profile email offline_access" \
  http://localhost:5556/dex/device/code
# Dex issues refresh_token with offline_access (password grant proves the token endpoint does):
curl -s -u "flow-dev:flow-dev-secret" -d grant_type=password \
  -d "username=msoent@dev.local" -d "password=password" \
  --data-urlencode "scope=openid profile email offline_access" \
  http://localhost:5556/dex/token | tr ',' '\n' | grep -c refresh_token
```

Expected: the first curl prints a JSON with `user_code`/`verification_uri`; the second prints `1` (a `refresh_token` is present). If the second prints `0`, note it — the device-flow refresh path then depends on Dex config and must be flagged before Task 10.

- [ ] **Step 7: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add deploy/dev/dex.yaml deploy/dev/flow.env deploy/dev/flow-cli.env Makefile go.mod go.sum
git commit -m "chore(dev): public flow-cli device client + go-keyring dep"
```

---

## Task 2: Server config — `FLOW_OIDC_CLI_CLIENT_ID`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Update tests for the new required field**

In `internal/config/config_test.go`, add `"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",` to the `env` map in **both** `TestLoadFromEnv` and `TestLoadDefaultsListenAddr`. In `TestLoadFromEnv`, add an assertion after the `OIDCClientSecret` check:

```go
	if c.OIDCCliClientID != "flow-cli" {
		t.Fatalf("CLI client id not parsed: %q", c.OIDCCliClientID)
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/config/ -run TestLoad`
Expected: FAIL — `c.OIDCCliClientID` undefined (compile error).

- [ ] **Step 3: Add the field to config**

In `internal/config/config.go`:
- Add `OIDCCliClientID string` to the `Config` struct (after `OIDCClientID`).
- In `Load`, add `OIDCCliClientID: getenv("FLOW_OIDC_CLI_CLIENT_ID"),` to the struct literal (after `OIDCClientID`).
- Add `{"FLOW_OIDC_CLI_CLIENT_ID", c.OIDCCliClientID},` to the required-fields loop slice (after the `FLOW_OIDC_CLIENT_ID` entry).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): require FLOW_OIDC_CLI_CLIENT_ID for CLI audience"
```

---

## Task 3: Multi-audience verifier

**Files:**
- Modify: `internal/adapter/oidcverify/verifier.go`
- Test: `internal/adapter/oidcverify/verifier_test.go`
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Update existing tests to the new signature + add a multi-aud test**

In `internal/adapter/oidcverify/verifier_test.go`, change every `oidcverify.New(ctx, h.issuer, "flow")` call to `oidcverify.New(ctx, h.issuer, []string{"flow"})` (three call sites: `TestVerifyValidToken`, `TestVerifyRejectsExpiredToken`, `TestVerifyRejectsWrongAudience`).

Then append a new test:

```go
func TestVerifyAcceptsSecondAudience(t *testing.T) {
	h := newOIDCHarness(t)
	defer h.srv.Close()

	// A token whose aud is the CLI client, not the primary web client.
	cliToken := h.signToken(t, jwt.MapClaims{"aud": "flow-cli"})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), h.issuer)
	v, err := oidcverify.New(ctx, h.issuer, []string{"flow", "flow-cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), cliToken); err != nil {
		t.Fatalf("verify CLI-audience token: %v", err)
	}

	// An audience outside the allowed set is still rejected.
	other := h.signToken(t, jwt.MapClaims{"aud": "evil"})
	if _, err := v.Verify(context.Background(), other); err == nil {
		t.Fatal("expected rejection for audience outside the allowed set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/oidcverify/`
Expected: FAIL — `New` signature mismatch (cannot use `[]string` ... ) compile error.

- [ ] **Step 3: Rewrite the verifier for multiple audiences**

Replace `internal/adapter/oidcverify/verifier.go` with:

```go
// Package oidcverify verifies Authentik/Dex-issued JWT access/ID tokens against
// a set of accepted audiences (the web client + the CLI client).
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

type Verifier struct {
	v         *oidc.IDTokenVerifier
	audiences map[string]bool
}

// New builds a verifier from the issuer's discovery document. A token is
// accepted if at least one of its audiences is in the allowed set.
func New(ctx context.Context, issuer string, audiences []string) (*Verifier, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcverify: provider: %w", err)
	}
	allowed := make(map[string]bool, len(audiences))
	for _, a := range audiences {
		if a != "" {
			allowed[a] = true
		}
	}
	// SkipClientIDCheck: go-oidc only compares a single clientID; we do the
	// (multi-audience) aud check ourselves below.
	return &Verifier{
		v:         p.Verifier(&oidc.Config{SkipClientIDCheck: true}),
		audiences: allowed,
	}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// Verify checks the token's signature (via the issuer JWKS), issuer, expiry,
// and that at least one audience is in the allowed set, then extracts the
// flow Identity.
func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	tok, err := vr.v.Verify(ctx, raw)
	if err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: verify: %w", err)
	}
	ok := false
	for _, a := range tok.Audience {
		if vr.audiences[a] {
			ok = true
			break
		}
	}
	if !ok {
		return ports.Identity{}, fmt.Errorf("oidcverify: audience %v not allowed", tok.Audience)
	}
	var c claims
	if err := tok.Claims(&c); err != nil {
		return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
	}
	return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/oidcverify/`
Expected: PASS (all four tests).

- [ ] **Step 5: Wire both audiences in the composition root**

In `cmd/flow-server/main.go`, change the verifier construction (line ~51):

```go
	verifier, err := oidcverify.New(ctx, cfg.OIDCIssuer, []string{cfg.OIDCClientID, cfg.OIDCCliClientID})
	if err != nil {
		return err
	}
```

- [ ] **Step 6: Verify the server still builds**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./cmd/flow-server`
Expected: builds, no error.

- [ ] **Step 7: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/oidcverify/ cmd/flow-server/main.go
git commit -m "feat(oidcverify): accept multiple audiences (web + CLI client)"
```

---

## Task 4: TokenStore port + file store

**Files:**
- Modify: `internal/ports/ports.go`
- Create: `internal/adapter/tokenstore/file.go`
- Test: `internal/adapter/tokenstore/file_test.go`

- [ ] **Step 1: Add the port type + interface**

In `internal/ports/ports.go`, after the `Identity` struct (around line 31), add:

```go
// Token is a stored OAuth token set for the CLI/TUI client.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

// TokenStore persists the CLI/TUI token between invocations.
type TokenStore interface {
	Save(t Token) error
	Load() (t Token, ok bool, err error)
	Clear() error
}
```

(`time` is already imported in this file.)

- [ ] **Step 2: Write the failing file-store test**

Create `internal/adapter/tokenstore/file_test.go`:

```go
package tokenstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "token.json")
	s := newFileStore(path)

	if _, ok, err := s.Load(); err != nil || ok {
		t.Fatalf("empty load: ok=%v err=%v", ok, err)
	}

	want := ports.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Unix(1000, 0).UTC()}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}

	got, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load after save: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}

	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Load(); ok {
		t.Fatal("token present after Clear")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/`
Expected: FAIL — `newFileStore` undefined.

- [ ] **Step 4: Implement the file store**

Create `internal/adapter/tokenstore/file.go`:

```go
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/ports/ports.go internal/adapter/tokenstore/file.go internal/adapter/tokenstore/file_test.go
git commit -m "feat(tokenstore): 0600 file store + TokenStore port"
```

---

## Task 5: Keyring store (split-field)

**Files:**
- Create: `internal/adapter/tokenstore/keyring.go`
- Test: `internal/adapter/tokenstore/keyring_test.go`

- [ ] **Step 1: Write the failing keyring test**

Create `internal/adapter/tokenstore/keyring_test.go`:

```go
package tokenstore

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestKeyringStoreRoundTrip(t *testing.T) {
	keyring.MockInit() // in-memory keyring for tests
	s := newKeyringStore()

	if _, ok, err := s.Load(); err != nil || ok {
		t.Fatalf("empty load: ok=%v err=%v", ok, err)
	}

	want := ports.Token{AccessToken: "acc", RefreshToken: "ref", Expiry: time.Unix(2000, 0).UTC()}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load after save: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}

	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Load(); ok {
		t.Fatal("token present after Clear")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/ -run TestKeyring`
Expected: FAIL — `newKeyringStore` undefined.

- [ ] **Step 3: Implement the keyring store**

Create `internal/adapter/tokenstore/keyring.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/`
Expected: PASS (file + keyring tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/tokenstore/keyring.go internal/adapter/tokenstore/keyring_test.go
git commit -m "feat(tokenstore): split-field keyring store"
```

---

## Task 6: `Open()` — keyring with file fallback

**Files:**
- Create: `internal/adapter/tokenstore/store.go`
- Test: `internal/adapter/tokenstore/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapter/tokenstore/store_test.go`:

```go
package tokenstore

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestOpenReturnsWorkingStore(t *testing.T) {
	keyring.MockInit()
	s := Open()
	if s == nil {
		t.Fatal("Open returned nil")
	}
	tok := ports.Token{AccessToken: "x", Expiry: time.Unix(1, 0).UTC()}
	if err := s.Save(tok); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok || got.AccessToken != "x" {
		t.Fatalf("load: got=%+v ok=%v err=%v", got, ok, err)
	}
	_ = s.Clear()
}

func TestDefaultFilePath(t *testing.T) {
	p, err := defaultFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty default path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/ -run "TestOpen|TestDefault"`
Expected: FAIL — `Open`/`defaultFilePath` undefined.

- [ ] **Step 3: Implement `Open` with a keyring probe**

Create `internal/adapter/tokenstore/store.go`:

```go
package tokenstore

import (
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// Open returns the keyring store when the OS keyring is usable, otherwise a
// 0600 file store (headless/CI/Linux-without-keyring).
func Open() ports.TokenStore {
	if keyringUsable() {
		return newKeyringStore()
	}
	path, err := defaultFilePath()
	if err != nil {
		// Last resort: a file in the working dir; better than a nil store.
		path = ".flow-token.json"
	}
	return newFileStore(path)
}

// keyringUsable probes the keyring with a throwaway item.
func keyringUsable() bool {
	const probe = "__probe__"
	if err := keyring.Set(keyringService, probe, "1"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, probe)
	return true
}

func defaultFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flow", "token.json"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/tokenstore/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/tokenstore/store.go internal/adapter/tokenstore/store_test.go
git commit -m "feat(tokenstore): Open() picks keyring with file fallback"
```

---

## Task 7: Device-flow adapter

**Files:**
- Create: `internal/adapter/oidcdevice/device.go`
- Test: `internal/adapter/oidcdevice/device_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapter/oidcdevice/device_test.go`:

```go
package oidcdevice_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
)

// newProviderServer serves an OIDC discovery doc plus device + token endpoints.
func newProviderServer(t *testing.T) (*httptest.Server, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                        srv.URL,
			"authorization_endpoint":        srv.URL + "/auth",
			"token_endpoint":                srv.URL + "/token",
			"device_authorization_endpoint": srv.URL + "/device/code",
			"jwks_uri":                      srv.URL + "/jwks",
		})
	})
	return srv, mux
}

func TestStartAndPoll(t *testing.T) {
	srv, mux := newProviderServer(t)
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "DEV-1",
			"user_code":        "ABCD-EFGH",
			"verification_uri": srv.URL + "/device",
			"expires_in":       300,
			"interval":         0,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "acc-tok",
			"refresh_token": "ref-tok",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	ctx := oidc.InsecureIssuerURLContext(context.Background(), srv.URL)
	fl, err := oidcdevice.New(ctx, srv.URL, "flow-cli")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	da, err := fl.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if da.UserCode != "ABCD-EFGH" {
		t.Fatalf("user code: %q", da.UserCode)
	}
	tok, err := fl.Poll(ctx, da)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if tok.AccessToken != "acc-tok" || tok.RefreshToken != "ref-tok" {
		t.Fatalf("token: %+v", tok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/oidcdevice/`
Expected: FAIL — package `oidcdevice` does not exist.

- [ ] **Step 3: Implement the device-flow client**

Create `internal/adapter/oidcdevice/device.go`:

```go
// Package oidcdevice runs the OAuth2 Device Authorization Grant (RFC 8628) for
// the CLI/TUI, using go-oidc for discovery and x/oauth2 for the flow itself.
package oidcdevice

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Flow is a configured device-flow client for one issuer + public client.
type Flow struct{ cfg oauth2.Config }

// New discovers the issuer endpoints and builds the device-flow config.
// go-oidc's Endpoint() omits the device endpoint, so it is read from the raw
// discovery document and set explicitly.
func New(ctx context.Context, issuer, clientID string) (*Flow, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: provider: %w", err)
	}
	var extra struct {
		DeviceAuthURL string `json:"device_authorization_endpoint"`
	}
	if err := p.Claims(&extra); err != nil {
		return nil, fmt.Errorf("oidcdevice: discovery claims: %w", err)
	}
	if extra.DeviceAuthURL == "" {
		return nil, fmt.Errorf("oidcdevice: issuer %q advertises no device_authorization_endpoint", issuer)
	}
	return &Flow{cfg: oauth2.Config{
		ClientID: clientID, // public client: no secret
		Endpoint: oauth2.Endpoint{
			AuthURL:       p.Endpoint().AuthURL,
			TokenURL:      p.Endpoint().TokenURL,
			DeviceAuthURL: extra.DeviceAuthURL,
		},
		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "offline_access"},
	}}, nil
}

// Start requests a device + user code from the issuer.
func (f *Flow) Start(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	da, err := f.cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: device auth: %w", err)
	}
	return da, nil
}

// Poll blocks until the user approves (or the code expires / is denied),
// honouring the server interval and slow_down responses internally.
func (f *Flow) Poll(ctx context.Context, da *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	tok, err := f.cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("oidcdevice: poll: %w", err)
	}
	return tok, nil
}

// TokenSource returns a refreshing source seeded with a stored token.
func (f *Flow) TokenSource(ctx context.Context, t *oauth2.Token) oauth2.TokenSource {
	return f.cfg.TokenSource(ctx, t)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/oidcdevice/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/oidcdevice/
git commit -m "feat(oidcdevice): RFC 8628 device-flow client"
```

---

## Task 8: Client config loader

**Files:**
- Create: `internal/clientconfig/clientconfig.go`
- Test: `internal/clientconfig/clientconfig_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/clientconfig/clientconfig_test.go`:

```go
package clientconfig

import "testing"

func TestLoadDefaults(t *testing.T) {
	env := map[string]string{"FLOW_OIDC_ISSUER": "http://localhost:5556/dex"}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerURL != "http://localhost:8080" {
		t.Fatalf("ServerURL default: %q", c.ServerURL)
	}
	if c.CliClientID != "flow-cli" {
		t.Fatalf("CliClientID default: %q", c.CliClientID)
	}
	if c.OIDCIssuer != "http://localhost:5556/dex" {
		t.Fatalf("OIDCIssuer: %q", c.OIDCIssuer)
	}
}

func TestLoadOverrides(t *testing.T) {
	env := map[string]string{
		"FLOW_SERVER_URL":         "https://flow.example.com",
		"FLOW_OIDC_ISSUER":        "https://id.example.com/o/flow/",
		"FLOW_OIDC_CLI_CLIENT_ID": "custom-cli",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerURL != "https://flow.example.com" || c.CliClientID != "custom-cli" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestLoadMissingIssuer(t *testing.T) {
	if _, err := Load(func(string) string { return "" }); err == nil {
		t.Fatal("expected error when FLOW_OIDC_ISSUER unset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/clientconfig/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement the loader**

Create `internal/clientconfig/clientconfig.go`:

```go
// Package clientconfig loads flow CLI/TUI configuration from the environment.
package clientconfig

import "fmt"

type Config struct {
	ServerURL   string
	OIDCIssuer  string
	CliClientID string
}

// Load reads config via getenv (injected for testability). ServerURL and
// CliClientID have dev-friendly defaults; OIDCIssuer is required.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		ServerURL:   getenv("FLOW_SERVER_URL"),
		OIDCIssuer:  getenv("FLOW_OIDC_ISSUER"),
		CliClientID: getenv("FLOW_OIDC_CLI_CLIENT_ID"),
	}
	if c.ServerURL == "" {
		c.ServerURL = "http://localhost:8080"
	}
	if c.CliClientID == "" {
		c.CliClientID = "flow-cli"
	}
	if c.OIDCIssuer == "" {
		return Config{}, fmt.Errorf("clientconfig: FLOW_OIDC_ISSUER is required")
	}
	return c, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/clientconfig/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/clientconfig/
git commit -m "feat(clientconfig): CLI/TUI env loader"
```

---

## Task 9: apiclient — transport-based auth

**Files:**
- Modify: `internal/adapter/apiclient/client.go`
- Modify: `internal/adapter/apiclient/events.go`
- Test: `internal/adapter/apiclient/client_test.go`

- [ ] **Step 1: Add a test for the transport constructor**

Append to `internal/adapter/apiclient/client_test.go`:

```go
type tagRoundTripper struct{ tag string }

func (rt tagRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+rt.tag)
	return http.DefaultTransport.RoundTrip(r2)
}

func TestNewTransportSetsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"u1","username":"msoent"}`))
	}))
	defer srv.Close()

	c := apiclient.NewTransport(srv.URL, tagRoundTripper{tag: "from-rt"})
	if _, err := c.Whoami(t.Context()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer from-rt" {
		t.Fatalf("auth header: %q", gotAuth)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/ -run TestNewTransport`
Expected: FAIL — `apiclient.NewTransport` undefined.

- [ ] **Step 3: Refactor the client to carry an auth RoundTripper**

In `internal/adapter/apiclient/client.go`, replace the `Client` struct and constructor (lines ~16-24) with:

```go
type Client struct {
	base string
	hc   *http.Client     // 15s timeout, for unary calls
	rt   http.RoundTripper // auth transport, reused for the no-timeout SSE client
}

// New builds a client that sends a fixed bearer token (CI / FLOW_TOKEN override).
func New(base, token string) *Client {
	return NewTransport(base, staticBearer{token})
}

// NewTransport builds a client whose auth (and refresh) is handled by rt.
func NewTransport(base string, rt http.RoundTripper) *Client {
	return &Client{base: base, rt: rt, hc: &http.Client{Timeout: 15 * time.Second, Transport: rt}}
}

// staticBearer injects a fixed bearer token on every request.
type staticBearer struct{ token string }

func (b staticBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r2)
}
```

Then remove the now-redundant manual header lines:
- In `Whoami`, delete `req.Header.Set("Authorization", "Bearer "+c.token)`.
- In `do`, delete `req.Header.Set("Authorization", "Bearer "+c.token)`.

(The `Content-Type` header in `do` stays.)

- [ ] **Step 4: Update the SSE client to use the auth transport**

In `internal/adapter/apiclient/events.go`:
- Delete the package-level `var streamClient = &http.Client{}` (lines ~12-14, keep no timeout behaviour via a per-call client).
- Delete `req.Header.Set("Authorization", "Bearer "+c.token)` (line ~30).
- Replace `res, err := streamClient.Do(req)` with:

```go
	res, err := (&http.Client{Transport: c.rt}).Do(req)
```

- [ ] **Step 5: Run the package tests**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./internal/adapter/apiclient/`
Expected: PASS (existing `New(base,"tok")` tests still pass — `staticBearer` sets the same header — plus the new transport test).

- [ ] **Step 6: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add internal/adapter/apiclient/client.go internal/adapter/apiclient/events.go internal/adapter/apiclient/client_test.go
git commit -m "refactor(apiclient): auth via RoundTripper, add NewTransport"
```

---

## Task 10: Session helper — `clientFromStore` + persisting source

**Files:**
- Create: `cmd/flow/auth.go`
- Test: `cmd/flow/auth_test.go`

- [ ] **Step 1: Write the failing test for the persisting source**

Create `cmd/flow/auth_test.go`:

```go
package main

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

// fakeSource hands out a fixed token.
type fakeSource struct{ tok *oauth2.Token }

func (f fakeSource) Token() (*oauth2.Token, error) { return f.tok, nil }

// memStore is an in-memory TokenStore.
type memStore struct {
	saved ports.Token
	calls int
}

func (m *memStore) Save(t ports.Token) error             { m.saved = t; m.calls++; return nil }
func (m *memStore) Load() (ports.Token, bool, error)     { return m.saved, m.calls > 0, nil }
func (m *memStore) Clear() error                         { m.saved = ports.Token{}; return nil }

func TestPersistingSourceSavesOnChange(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "r", Expiry: time.Unix(10, 0)}},
		store: store,
		last:  ports.Token{AccessToken: "old"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.saved.AccessToken != "new" {
		t.Fatalf("expected save of new token, got calls=%d saved=%+v", store.calls, store.saved)
	}
	// Second call with an unchanged token must not re-save.
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("expected no re-save, calls=%d", store.calls)
	}
}

func TestPersistingSourcePreservesRefreshWhenEmpty(t *testing.T) {
	store := &memStore{}
	src := &persistingSource{
		base:  fakeSource{tok: &oauth2.Token{AccessToken: "a2", RefreshToken: ""}},
		store: store,
		last:  ports.Token{AccessToken: "a1", RefreshToken: "keep"},
	}
	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if store.saved.RefreshToken != "keep" {
		t.Fatalf("refresh token not preserved: %q", store.saved.RefreshToken)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./cmd/flow/ -run TestPersisting`
Expected: FAIL — `persistingSource` undefined.

- [ ] **Step 3: Implement the session helper**

Create `cmd/flow/auth.go`:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
)

// persistingSource wraps a refreshing oauth2 source and writes refreshed
// tokens back to the store. It preserves the refresh token when a refresh
// response omits it (oauth2 already does this, but we guard the store too).
type persistingSource struct {
	base  oauth2.TokenSource
	store ports.TokenStore
	last  ports.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = p.last.RefreshToken
	}
	if tok.AccessToken != p.last.AccessToken {
		next := ports.Token{AccessToken: tok.AccessToken, RefreshToken: refresh, Expiry: tok.Expiry}
		if err := p.store.Save(next); err != nil {
			return nil, fmt.Errorf("flow: persist token: %w", err)
		}
		p.last = next
	}
	return tok, nil
}

// clientFromStore builds an authenticated apiclient. FLOW_TOKEN (if set) wins
// as a static, non-refreshing bearer (CI). Otherwise it loads the stored token
// and wraps it in a refreshing, self-persisting source.
func clientFromStore(ctx context.Context) (*apiclient.Client, error) {
	cfg, err := clientconfig.Load(os.Getenv)
	if err != nil {
		return nil, err
	}
	if t := os.Getenv("FLOW_TOKEN"); t != "" {
		return apiclient.New(cfg.ServerURL, t), nil
	}
	store := tokenstore.Open()
	loaded, ok, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !ok || loaded.AccessToken == "" {
		return nil, fmt.Errorf("not logged in — run `flow login`")
	}
	flow, err := oidcdevice.New(ctx, cfg.OIDCIssuer, cfg.CliClientID)
	if err != nil {
		return nil, err
	}
	base := flow.TokenSource(ctx, &oauth2.Token{
		AccessToken:  loaded.AccessToken,
		RefreshToken: loaded.RefreshToken,
		Expiry:       loaded.Expiry,
	})
	src := &persistingSource{base: base, store: store, last: loaded}
	rt := &oauth2.Transport{Source: src, Base: http.DefaultTransport}
	return apiclient.NewTransport(cfg.ServerURL, rt), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go test ./cmd/flow/ -run TestPersisting`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add cmd/flow/auth.go cmd/flow/auth_test.go
git commit -m "feat(flow): clientFromStore + self-persisting token source"
```

---

## Task 11: `flow login` and `flow logout`

**Files:**
- Create: `cmd/flow/login.go`
- Create: `cmd/flow/logout.go`
- Modify: `cmd/flow/main.go`

- [ ] **Step 1: Implement `flow login`**

Create `cmd/flow/login.go`:

```go
package main

import (
	"fmt"
	"os"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in via OIDC device flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, err := clientconfig.Load(os.Getenv)
			if err != nil {
				return err
			}
			flow, err := oidcdevice.New(ctx, cfg.OIDCIssuer, cfg.CliClientID)
			if err != nil {
				return err
			}
			da, err := flow.Start(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("\nTo log in, open:\n\n  %s\n\nand enter the code:  %s\n\n", da.VerificationURI, da.UserCode)
			if da.VerificationURIComplete != "" {
				fmt.Printf("Or open this URL directly:\n\n  %s\n\n", da.VerificationURIComplete)
			}
			fmt.Println("Waiting for approval...")

			tok, err := flow.Poll(ctx, da)
			if err != nil {
				return fmt.Errorf("login failed (the code may have expired or been denied): %w", err)
			}
			if err := tokenstore.Open().Save(ports.Token{
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Expiry:       tok.Expiry,
			}); err != nil {
				return fmt.Errorf("store token: %w", err)
			}
			u, err := apiclient.NewTransport(cfg.ServerURL, &oauth2.Transport{
				Source: oauth2.StaticTokenSource(tok),
			}).Whoami(ctx)
			if err != nil {
				return fmt.Errorf("token stored but server rejected it: %w", err)
			}
			fmt.Printf("\nLogged in as %s <%s>\n", u.DisplayName, u.Email)
			return nil
		},
	}
}
```

- [ ] **Step 2: Implement `flow logout`**

Create `cmd/flow/logout.go`:

```go
package main

import (
	"fmt"

	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/spf13/cobra"
)

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored flow credentials",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := tokenstore.Open().Clear(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}
```

- [ ] **Step 3: Register the commands**

In `cmd/flow/main.go`, add to `rootCmd` after `root.AddCommand(whoamiCmd())`:

```go
	root.AddCommand(loginCmd())
	root.AddCommand(logoutCmd())
```

- [ ] **Step 4: Verify it builds and the command tree is present**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./cmd/flow && go run ./cmd/flow --help 2>&1 | grep -E "login|logout"`
Expected: builds; help output lists `login` and `logout`.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add cmd/flow/login.go cmd/flow/logout.go cmd/flow/main.go
git commit -m "feat(flow): login (device flow) + logout commands"
```

---

## Task 12: Switch `whoami` and `worktime` to stored credentials

**Files:**
- Modify: `cmd/flow/whoami.go`
- Modify: `cmd/flow/worktime.go`

- [ ] **Step 1: Rewrite `whoami` to use `clientFromStore`**

Replace `cmd/flow/whoami.go` entirely with the clean form below. This drops the old `FLOW_TOKEN`-required error **and** the `envOr` helper (its only callers were `whoami.go` and `worktime.go`, both replaced in this task; `clientFromStore` now owns server-URL resolution via `clientconfig`):

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated flow user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			u, err := client.Whoami(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("%s <%s> (%s)\n", u.DisplayName, u.Email, u.Username)
			return nil
		},
	}
}
```

- [ ] **Step 2: Rewrite `worktime` to use `clientFromStore`**

Replace `cmd/flow/worktime.go` with:

```go
package main

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/spf13/cobra"
)

func worktimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worktime",
		Short: "Worktime timer (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			m := tui.New(client, os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
}
```

- [ ] **Step 3: Confirm `envOr` is fully removed**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && rg -n "envOr" cmd/flow`
Expected: no matches. (If any remain, delete the leftover definition — both call sites are gone.)

- [ ] **Step 4: Build and vet**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && go build ./... && go vet ./cmd/flow/...`
Expected: builds clean, no unused-import / unused-function errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add cmd/flow/whoami.go cmd/flow/worktime.go
git commit -m "feat(flow): whoami + worktime use stored credentials"
```

---

## Task 13: Wiring verification + done-gate

**Files:**
- Create: `scripts/smoke-m1b.sh`
- Modify: `Makefile`

- [ ] **Step 1: Write the server multi-audience smoke**

Create `scripts/smoke-m1b.sh`:

```bash
#!/usr/bin/env bash
# Smoke: a token minted for the public `flow-cli` client (aud=flow-cli) is
# accepted by flow-server's multi-audience verifier at /api/v1/me.
# Requires: make dev-up + flow-server running (make dev-run) in another shell.
set -euo pipefail
ISSUER="${ISSUER:-http://localhost:5556/dex}"
SERVER="${FLOW_SERVER_URL:-http://localhost:8080}"

# Password grant with the flow-cli client id (public client, no secret).
resp=$(curl -fsS -d client_id=flow-cli -d grant_type=password \
  -d "username=msoent@dev.local" -d "password=password" \
  --data-urlencode "scope=openid profile email offline_access" \
  "$ISSUER/token")
at=$(printf '%s' "$resp" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$at" ] || { echo "smoke-m1b: no access_token: $resp" >&2; exit 1; }

code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $at" "$SERVER/api/v1/me")
[ "$code" = "200" ] || { echo "smoke-m1b: /api/v1/me returned $code (want 200)" >&2; exit 1; }
echo "smoke-m1b: OK — flow-cli audience accepted at /api/v1/me"
```

Then `chmod +x scripts/smoke-m1b.sh`.

If the password grant against the public `flow-cli` client is rejected by Dex (some Dex versions require client auth for the password grant), fall back to verifying the audience path through the manual device login in Step 4 and note the deviation; do not block on the scripted smoke.

- [ ] **Step 2: Add a Makefile target**

In `Makefile`, add `smoke-m1b` to `.PHONY` and add:

```makefile
smoke-m1b:
	./scripts/smoke-m1b.sh
```

- [ ] **Step 3: Run the full CI gate**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-rebuild && make ci`
Expected: build + vet + tests + coverage gate all pass. If coverage dipped below the gate, add focused tests to the lightest-covered new file (most likely `cmd/flow` login/logout — extract any pure logic if needed) until green.

- [ ] **Step 4: Manual done-gate (dogfood against Dex)**

Run, in two shells:

```bash
# shell A — dependencies + server
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
make dev-up
make dev-run        # leave running

# shell B — the CLI
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
make smoke-m1b                                   # server accepts flow-cli audience → "OK"
set -a; . deploy/dev/flow-cli.env; set +a
go run ./cmd/flow login                          # open URL, log in as msoent@dev.local / password, approve
go run ./cmd/flow whoami                         # prints the user WITHOUT FLOW_TOKEN set
go run ./cmd/flow whoami                          # second run still works (token loaded from store)
go run ./cmd/flow logout                         # "Logged out."
go run ./cmd/flow whoami                          # fails with "not logged in — run `flow login`"
```

Expected:
1. `smoke-m1b` prints `OK`.
2. `login` prints a URL + code, then `Logged in as msoent <msoent@dev.local>` (display name may be `msoent`).
3. `whoami` (both runs) prints the user with no `FLOW_TOKEN` in the environment.
4. `logout` then `whoami` fails with the not-logged-in message.

Confirm the token actually persisted: on macOS check Keychain for service `flow` items `access_token`/`refresh_token`/`expiry`; on Linux without a keyring check `~/.config/flow/token.json` (mode `600`).

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-rebuild
git add scripts/smoke-m1b.sh Makefile
git commit -m "test(m1b): server multi-audience smoke + done-gate"
```

---

## Self-Review Notes (for the implementer)

- **Spec coverage:** full-lifecycle login/logout/whoami (T11–T12) · access_token bearer + refresh (T7 TokenSource, T10 persisting source) · keyring + file fallback + FLOW_TOKEN override (T4–T6, T10) · dedicated public `flow-cli` client (T1) · multi-audience verifier (T3) · x/oauth2 device built-ins (T7) · dev-env (T1) · tests + wiring + done-gate (T13). All spec sections map to a task.
- **macOS keyring 2 KiB:** handled by per-field items in T5.
- **Refresh-token-empty preservation:** guarded in T10 (`TestPersistingSourcePreservesRefreshWhenEmpty`).
- **Open question carried from spec** (Dex device-flow refresh_token) is verified empirically in T1/Step 6 before the refresh path is built in T10.
- **Type consistency:** `oidcverify.New(ctx, issuer, []string)`, `ports.Token{AccessToken,RefreshToken,Expiry}`, `ports.TokenStore{Save,Load,Clear}`, `apiclient.New`/`NewTransport`, `oidcdevice.New/Start/Poll/TokenSource`, `clientconfig.Config{ServerURL,OIDCIssuer,CliClientID}` are used identically across tasks.
