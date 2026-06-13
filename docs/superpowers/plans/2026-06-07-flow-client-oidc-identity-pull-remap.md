# flow Client OIDC Identity Adoption + Pull-Remap — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a logged-in flow client operate under its real OIDC identity so pulled rows store without FK errors and sync round-trips, multi-user-ready, client-only.

**Architecture:** `user_id` columns stay store-local UUIDs; the OIDC `sub` is the canonical cross-store identity (server-side, via bearer auth + per-user pull scoping). The sync worker remaps every pulled row's `UserID` to the local user (load-bearing fix). At startup the client resolves the active user from the stored token's `sub` (or the `local` placeholder offline). `flow login` offers a one-time, prompt-gated adoption of pre-login `local` data by *re-labelling* the local user row (no data re-keying).

**Tech Stack:** Go, SQLite (modernc/goose), cobra CLI, go-oidc device flow, hexagonal ports/adapters.

**Spec:** `docs/superpowers/specs/2026-06-07-flow-client-oidc-identity-pull-remap-design.md`

**Prerequisite (already landed):** `b85e144` project-sync fix.

---

## File Structure

- `internal/adapter/httpsync/worker.go` — MODIFY: pull-remap in the five pull paths.
- `internal/adapter/httpsync/worker_test.go` — MODIFY: pull-remap test.
- `internal/adapter/oidcclient/claims.go` — CREATE: unverified JWT claims decode (`sub`/`email`/`name`).
- `internal/adapter/oidcclient/claims_test.go` — CREATE.
- `internal/adapter/sqliteclient/users.go` — MODIFY: `RelabelBySub` + `CountOwnedRows`.
- `internal/adapter/sqliteclient/users_test.go` — MODIFY/CREATE.
- `internal/usecase/identity.go` — CREATE: `ResolveActiveUser` + `AdoptLocalData` use case (pure orchestration over ports).
- `internal/usecase/identity_test.go` — CREATE.
- `cmd/flow/main.go` — MODIFY: resolve active user from token instead of hardcoded `local`.
- `cmd/flow-mcp/main.go` — MODIFY: same resolution.
- `cmd/flow/login.go` — MODIFY: first-login adoption prompt.

> Per `feedback_no_monoliths`: identity logic lives in `internal/usecase/identity.go`, not inline in `main.go`. `main.go` stays wiring.

---

## Task 1: Pull-remap in the sync worker

This is the load-bearing fix and is independent of the rest — land it first.

**Files:**
- Modify: `internal/adapter/httpsync/worker.go:238-299`
- Test: `internal/adapter/httpsync/worker_test.go`

- [ ] **Step 1: Write the failing test**

Mirror the existing fake-client + fake-store setup in `worker_test.go` (search it for `pullActivePage`/`PullActive`/a fake `syncClient`). Add:

```go
func TestUnit_Worker_PullRemapsUserIDToLocalUser(t *testing.T) {
	// Fake client returns one active session owned by a DIFFERENT (server) user id.
	fc := &fakeSyncClient{active: []domain.ActiveSession{
		{UserID: "server-uuid", ProjectID: "p1", StartedAt: time.Now().UTC()},
	}}
	store := &fakeActiveStore{} // records Upsert args in .upserted
	w := newTestWorker(t, withClient(fc), withActiveStore(store), withUserID("local-uuid"))

	if _, _, err := w.pullActivePage(context.Background(), 0); err != nil {
		t.Fatalf("pullActivePage: %v", err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}
	if got := store.upserted[0].UserID; got != "local-uuid" {
		t.Errorf("UserID = %q, want local-uuid (remapped to the worker's user)", got)
	}
}
```

If `worker_test.go` has no reusable fakes/constructor helper, add minimal ones next to the existing tests (a `fakeSyncClient` implementing the `PullActive` method used here, and a `fakeActiveStore` implementing `ports.ActiveSessionStore` that appends to `upserted` on `Upsert`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUnit_Worker_PullRemapsUserIDToLocalUser ./internal/adapter/httpsync/`
Expected: FAIL — `UserID = "server-uuid"`, want `local-uuid`.

- [ ] **Step 3: Add the remap to all five pull paths**

In `worker.go`, set `UserID = w.userID` before every Upsert. The per-item loops:

```go
// pullReposPage
for _, r := range items {
	r.UserID = w.userID
	if err := w.repos.Upsert(r); err != nil {
		return 0, false, err
	}
}
// pullRepoNotesPage
for _, n := range items {
	n.UserID = w.userID
	if err := w.notes.Upsert(n); err != nil {
		return 0, false, err
	}
}
// pullProjectsPage
for _, p := range items {
	p.UserID = w.userID
	if err := w.projects.Upsert(p); err != nil {
		return 0, false, err
	}
}
// pullActivePage
for _, a := range items {
	a.UserID = w.userID
	if err := w.active.Upsert(a); err != nil {
		return 0, false, err
	}
}
```

`pullSessionsPage` uses a batch — remap in place before the batch call:

```go
func (w *Worker) pullSessionsPage(ctx context.Context, since int64) (int64, bool, error) {
	items, hi, more, err := w.client.PullSessions(ctx, since, 200)
	if err != nil {
		return 0, false, err
	}
	for i := range items {
		items[i].UserID = w.userID
	}
	if err := w.sessions.UpsertBatch(items); err != nil {
		return 0, false, err
	}
	return hi, more, nil
}
```

Add a one-line comment above the first remap explaining WHY: the server scopes each pull to the authenticated user, so every pulled row belongs to the local logged-in user; rewriting `UserID` keeps the local FK (`user_id → users(id)`) satisfiable when the server's user UUID differs from the client's.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/httpsync/`
Expected: PASS (new test + existing).

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/httpsync/worker.go internal/adapter/httpsync/worker_test.go
git commit -m "fix(httpsync): remap pulled rows' user_id to the local user

Pulled rows carry the server's user UUID, which never matches the client's
local user UUID, so active_sessions (ON CONFLICT(user_id,project_id)) FK-failed
on pull. The server scopes every pull to the authenticated user, so all pulled
rows belong to the local user — rewrite UserID before Upsert across all five
pull paths."
```

---

## Task 2: Unverified JWT claims decoder

The client needs `sub` (and `email`/`name` from the ID token) for local identity. The token is already server-validated; the client only reads claims.

**Files:**
- Create: `internal/adapter/oidcclient/claims.go`
- Test: `internal/adapter/oidcclient/claims_test.go`

- [ ] **Step 1: Write the failing test**

```go
package oidcclient

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestUnit_ClaimsFromToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := signTestJWT(t, key, map[string]any{
		"sub": "msoent", "email": "m@x.de", "name": "Soenne",
	})
	c, err := ClaimsFromToken(tok)
	if err != nil {
		t.Fatalf("ClaimsFromToken: %v", err)
	}
	if c.Sub != "msoent" || c.Email != "m@x.de" || c.Name != "Soenne" {
		t.Errorf("got %+v", c)
	}
	if _, err := ClaimsFromToken("not.a.jwt-only-two"); err == nil {
		t.Error("expected error for malformed token")
	}
}

// signTestJWT builds a compact RS256 JWS; signature is irrelevant to the
// decoder but keeps the token well-formed.
func signTestJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	b64 := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	si := b64(map[string]any{"alg": "RS256", "typ": "JWT"}) + "." + b64(claims)
	sum := sha256.Sum256([]byte(si))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	return si + "." + base64.RawURLEncoding.EncodeToString(sig)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUnit_ClaimsFromToken ./internal/adapter/oidcclient/`
Expected: FAIL — `ClaimsFromToken` undefined.

- [ ] **Step 3: Implement the decoder**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestUnit_ClaimsFromToken ./internal/adapter/oidcclient/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/oidcclient/claims.go internal/adapter/oidcclient/claims_test.go
git commit -m "feat(oidcclient): decode sub/email/name from a token (unverified)"
```

---

## Task 3: sqliteclient users — relabel + owned-row count

**Files:**
- Modify: `internal/adapter/sqliteclient/users.go`
- Test: `internal/adapter/sqliteclient/users_test.go`

- [ ] **Step 1: Write the failing test**

Mirror the existing `users_test.go` store setup (`newTestStore(t)` / migrations). Add:

```go
func TestUnit_Users_RelabelBySub(t *testing.T) {
	u := NewUsers(newTestStore(t))
	orig, _ := u.EnsureBySub("local", "", "")
	if err := u.RelabelBySub("local", "msoent", "m@x.de", "Soenne"); err != nil {
		t.Fatalf("RelabelBySub: %v", err)
	}
	got, err := u.GetBySub("msoent")
	if err != nil {
		t.Fatalf("GetBySub(msoent): %v", err)
	}
	if got.ID != orig.ID {
		t.Errorf("relabel changed id: %q != %q (must keep id so data stays owned)", got.ID, orig.ID)
	}
	if _, err := u.GetBySub("local"); err == nil {
		t.Error("old 'local' sub should no longer resolve")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUnit_Users_RelabelBySub ./internal/adapter/sqliteclient/`
Expected: FAIL — `RelabelBySub` undefined.

- [ ] **Step 3: Implement RelabelBySub and CountOwnedRows**

In `users.go`:

```go
// RelabelBySub re-points the user row identified by fromSub to a new identity,
// keeping the same primary-key id so all rows that reference it stay owned.
// Used for first-login adoption of the offline `local` profile. Caller must
// ensure toSub is not already present (oidc_sub is UNIQUE).
func (u *Users) RelabelBySub(fromSub, toSub, email, displayName string) error {
	res, err := u.store.DB().Exec(
		`UPDATE users SET oidc_sub = ?, email = ?, display_name = ? WHERE oidc_sub = ?`,
		toSub, email, displayName, fromSub,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Users.RelabelBySub: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sqliteclient.Users.RelabelBySub: no user with sub %q", fromSub)
	}
	return nil
}

// CountOwnedRows returns how many projects + sessions reference the given user.
// Used to decide whether the first-login adoption prompt is worth showing.
func (u *Users) CountOwnedRows(userID string) (int, error) {
	var n int
	err := u.store.DB().QueryRow(
		`SELECT (SELECT COUNT(*) FROM projects WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM sessions WHERE user_id = ?)`,
		userID, userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.Users.CountOwnedRows: %w", err)
	}
	return n, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestUnit_Users_Relabel ./internal/adapter/sqliteclient/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/sqliteclient/users.go internal/adapter/sqliteclient/users_test.go
git commit -m "feat(sqliteclient): RelabelBySub + CountOwnedRows for identity adoption"
```

---

## Task 4: Identity use case (resolve active user + adopt)

Pure orchestration so `main.go` and `login.go` stay thin. Define a small port for the user operations it needs (interface segregation — only what's used here).

**Files:**
- Create: `internal/usecase/identity.go`
- Test: `internal/usecase/identity_test.go`

- [ ] **Step 1: Write the failing test**

```go
package usecase_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeIdentityStore struct {
	bySub    map[string]domain.User
	counts   map[string]int
	relabels [][2]string // (from,to)
}

func (f *fakeIdentityStore) EnsureBySub(sub, email, name string) (domain.User, error) {
	if u, ok := f.bySub[sub]; ok {
		return u, nil
	}
	u := domain.User{ID: "id-" + sub, OIDCSub: sub}
	if f.bySub == nil {
		f.bySub = map[string]domain.User{}
	}
	f.bySub[sub] = u
	return u, nil
}
func (f *fakeIdentityStore) GetBySub(sub string) (domain.User, error) {
	if u, ok := f.bySub[sub]; ok {
		return u, nil
	}
	return domain.User{}, ports.ErrUserNotFound
}
func (f *fakeIdentityStore) CountOwnedRows(id string) (int, error) { return f.counts[id], nil }
func (f *fakeIdentityStore) RelabelBySub(from, to, _, _ string) error {
	f.relabels = append(f.relabels, [2]string{from, to})
	u := f.bySub[from]
	delete(f.bySub, from)
	u.OIDCSub = to
	f.bySub[to] = u
	return nil
}

func TestUnit_Identity_ResolveActiveUser_FallsBackToLocalWhenNoSub(t *testing.T) {
	store := &fakeIdentityStore{}
	id := usecase.NewIdentity(store, "local")
	u, err := id.ResolveActiveUser("") // no token sub
	if err != nil {
		t.Fatal(err)
	}
	if u.OIDCSub != "local" {
		t.Errorf("sub = %q, want local", u.OIDCSub)
	}
}

func TestUnit_Identity_Adopt_RelabelsLocalWhenFirstLoginWithData(t *testing.T) {
	store := &fakeIdentityStore{
		bySub:  map[string]domain.User{"local": {ID: "id-local", OIDCSub: "local"}},
		counts: map[string]int{"id-local": 3},
	}
	id := usecase.NewIdentity(store, "local")
	adopted, n, err := id.AdoptLocalDataIfFirstLogin("msoent", "m@x.de", "Soenne")
	if err != nil {
		t.Fatal(err)
	}
	if !adopted || n != 3 {
		t.Fatalf("adopted=%v n=%d, want true/3", adopted, n)
	}
	if len(store.relabels) != 1 || store.relabels[0] != [2]string{"local", "msoent"} {
		t.Errorf("relabels = %v", store.relabels)
	}
}

func TestUnit_Identity_Adopt_SkipsWhenOidcUserAlreadyExists(t *testing.T) {
	store := &fakeIdentityStore{
		bySub:  map[string]domain.User{"local": {ID: "id-local", OIDCSub: "local"}, "msoent": {ID: "id-msoent", OIDCSub: "msoent"}},
		counts: map[string]int{"id-local": 3},
	}
	id := usecase.NewIdentity(store, "local")
	adopted, _, err := id.AdoptLocalDataIfFirstLogin("msoent", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if adopted {
		t.Error("must not adopt when an OIDC user already exists (not first login)")
	}
}
```

Add `"github.com/serverkraken/flow/internal/ports"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestUnit_Identity ./internal/usecase/`
Expected: FAIL — `usecase.NewIdentity` undefined.

- [ ] **Step 3: Implement the Identity use case**

```go
package usecase

import "github.com/serverkraken/flow/internal/domain"

// IdentityStore is the subset of the user store the Identity use case needs.
type IdentityStore interface {
	EnsureBySub(sub, email, displayName string) (domain.User, error)
	GetBySub(sub string) (domain.User, error)
	CountOwnedRows(userID string) (int, error)
	RelabelBySub(fromSub, toSub, email, displayName string) error
}

// Identity resolves which local user a client runs as, and adopts the offline
// `local` profile into the OIDC identity on first login. See
// docs/superpowers/specs/2026-06-07-flow-client-oidc-identity-pull-remap-design.md.
type Identity struct {
	store    IdentityStore
	localSub string // FLOW_LOCAL_USER_SUB (default "local")
}

func NewIdentity(store IdentityStore, localSub string) *Identity {
	return &Identity{store: store, localSub: localSub}
}

// ResolveActiveUser returns the local user the client should run as. tokenSub is
// the sub decoded from the stored token (empty when logged out). With a sub it
// ensures/returns the OIDC user; otherwise the `local` placeholder.
func (i *Identity) ResolveActiveUser(tokenSub string) (domain.User, error) {
	sub := tokenSub
	if sub == "" {
		sub = i.localSub
	}
	return i.store.EnsureBySub(sub, "", "")
}

// AdoptLocalDataIfFirstLogin re-labels the `local` user into the OIDC identity
// when (a) no OIDC user for sub exists yet (first login) and (b) the `local`
// user owns data. Returns whether it adopted and how many rows it carried over.
// Caller (flow login) shows the prompt and only calls this on user consent.
func (i *Identity) AdoptLocalDataIfFirstLogin(sub, email, name string) (bool, int, error) {
	if _, err := i.store.GetBySub(sub); err == nil {
		return false, 0, nil // OIDC user already exists → not first login
	}
	localUser, err := i.store.GetBySub(i.localSub)
	if err != nil {
		return false, 0, nil // no local profile to adopt
	}
	n, err := i.store.CountOwnedRows(localUser.ID)
	if err != nil {
		return false, 0, err
	}
	if n == 0 {
		return false, 0, nil
	}
	if err := i.store.RelabelBySub(i.localSub, sub, email, name); err != nil {
		return false, 0, err
	}
	return true, n, nil
}
```

> NOTE: `GetBySub` must return `ports.ErrUserNotFound` (not a wrapped error) on miss — confirm `sqliteclient.Users.GetBySub` does (it does per users.go:61). The `err == nil` check above treats any successful lookup as "exists".

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestUnit_Identity ./internal/usecase/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/identity.go internal/usecase/identity_test.go
git commit -m "feat(usecase): Identity — resolve active user + first-login adoption"
```

---

## Task 5: Wire current-user resolution into main (flow + flow-mcp)

**Files:**
- Modify: `cmd/flow/main.go:150-155`
- Modify: `cmd/flow-mcp/main.go` (the matching `localSub`/`EnsureBySub("local")` block)

- [ ] **Step 1: Read the current block**

`cmd/flow/main.go` currently:
```go
localSub := env.LocalUserSub
if localSub == "" {
	localSub = "local"
}
localUser, err := cacheUsers.EnsureBySub(localSub, "", "")
```

- [ ] **Step 2: Replace with token-aware resolution**

The token store is already constructed for sync further down (`keyringadapter.New()`, slot `"tokens:"+serverURL`). Resolve the sub BEFORE the user, reading the token best-effort:

```go
localSub := env.LocalUserSub
if localSub == "" {
	localSub = "local"
}
// Resolve the active identity: if a token is present for this server, run as the
// OIDC user (sub from the token); otherwise the offline `local` placeholder.
tokenSub := ""
if toks, err := keyringadapter.New().Get("tokens:" + env.ServerURL); err == nil {
	src := toks.IDToken
	if src == "" {
		src = toks.AccessToken
	}
	if c, err := oidcclient.ClaimsFromToken(src); err == nil {
		tokenSub = c.Sub
	}
}
identityUC := usecase.NewIdentity(cacheUsers, localSub)
localUser, err := identityUC.ResolveActiveUser(tokenSub)
if err != nil {
	return fmt.Errorf("resolve active user: %w", err)
}
```

Add imports `oidcclient` and (already present) `usecase`, `keyringadapter`. Keep the existing `localUser.ID` usages unchanged. Apply the identical change to `cmd/flow-mcp/main.go`.

> The keyring `Get` is best-effort: an absent/locked keychain or a logged-out client yields `tokenSub == ""` → the existing `local` behaviour. No new failure path at startup.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/main.go cmd/flow-mcp/main.go
git commit -m "feat(flow): run under the OIDC identity when a token is present"
```

---

## Task 6: First-login adoption prompt in `flow login`

**Files:**
- Modify: `cmd/flow/login.go` (after successful token persistence)

- [ ] **Step 1: Locate the post-persist point**

In `cmd/flow/login.go`, find where the device-flow tokens are saved (the success path that prints "Login erfolgreich, Token im Keychain gespeichert."). The adoption runs right after a successful save, before returning.

- [ ] **Step 2: Add the prompt + adoption**

Open the same cache store the rest of the client uses (`sqliteclient.Open(<cacheDBPath>)` — reuse the path helper `main.go` uses; if the login command already has the path, pass it in). Then:

```go
// First-login adoption: offer to carry the offline `local` profile into this
// OIDC identity. Decode the sub/email/name from the freshly minted token.
claims, err := oidcclient.ClaimsFromToken(firstNonEmpty(tok.IDToken, tok.AccessToken))
if err == nil {
	users := sqliteclient.NewUsers(cacheStore)
	id := usecase.NewIdentity(users, localSub) // localSub = "local" default
	if local, derr := users.GetBySub(localSub); derr == nil {
		if n, _ := users.CountOwnedRows(local.ID); n > 0 {
			if promptYesNo(out, in, fmt.Sprintf(
				"%d lokale Projekte/Sessions unter dem Offline-Profil gefunden. Unter %s übernehmen? [y/N] ",
				n, firstNonEmpty(claims.Email, claims.Sub))) {
				if adopted, carried, aerr := id.AdoptLocalDataIfFirstLogin(claims.Sub, claims.Email, claims.Name); aerr != nil {
					fmt.Fprintf(out, "⚠ Übernahme fehlgeschlagen: %v\n", aerr)
				} else if adopted {
					fmt.Fprintf(out, "✓ %d Einträge unter %s übernommen.\n", carried, claims.Sub)
				}
			}
		}
	}
}
```

Helpers (add to `login.go` or a small util if not present):
```go
func firstNonEmpty(a, b string) string { if a != "" { return a }; return b }

func promptYesNo(out io.Writer, in io.Reader, q string) bool {
	fmt.Fprint(out, q)
	r := bufio.NewReader(in)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes" || line == "j" || line == "ja"
}
```

Wire `out`/`in` from the cobra command (`cmd.OutOrStdout()`, `cmd.InOrStdin()`). Reuse the existing cache-DB path the command already knows; if it doesn't, thread it in from `main.go` the same way other commands receive config.

- [ ] **Step 3: Build + manual check**

Run: `go build -o bin/flow ./cmd/flow`
Expected: success. (Interactive prompt is covered by manual smoke; a unit test for `AdoptLocalDataIfFirstLogin` already exists in Task 4.)

- [ ] **Step 4: Commit**

```bash
git add cmd/flow/login.go
git commit -m "feat(flow): prompt to adopt the offline profile on first login"
```

---

## Task 7: Full verification

- [ ] **Step 1: make ci**

Run: `make ci`
Expected: green, coverage ≥ 77%.

- [ ] **Step 2: Manual end-to-end (real server)**

```bash
make build-server >/dev/null 2>&1 || true
./bin/flow logout 2>/dev/null
FLOW_SERVER_URL=https://flow.thebackend.org ./bin/flow login   # answer the adopt prompt
FLOW_SERVER_URL=https://flow.thebackend.org ./bin/flow whoami   # → msoent
# Then watch the sync log: no "pull active_sessions ... FOREIGN KEY" warnings;
# active_sessions watermark advances:
sqlite3 ~/.local/share/flow/cache.db "SELECT resource, watermark FROM sync_state;"
```
Expected: `active_sessions` watermark > 0, no FK warnings, TUI shows the adopted projects under the OIDC identity.

- [ ] **Step 3: Commit any doc/runbook updates and update `CLAUDE-activeContext.md`.**

---

## Self-Review notes (author)

- **Spec coverage:** §1 identity model → Tasks 4/5; §2 re-label adoption → Tasks 3/4/6; §3 login prompt → Task 6; §4 current-user resolution → Task 5; §5 pull-remap → Task 1; §6 multi-user (first-login-only adoption, fresh user otherwise) → Task 4 logic (`AdoptLocalDataIfFirstLogin` skips when OIDC user exists); §7 offline → Task 5 (`tokenSub == ""` → local).
- **Confirmed:** `domain.User` fields are `ID`, `OIDCSub`, `Email`, `DisplayName`, `CreatedAt` (`internal/domain/user.go`); the users tables use column `oidc_sub`. The plan's test code (`OIDCSub`, `GetBySub`) matches.
- `cmd/flow-mcp` does NOT construct the Projects use case, so it needs only the Task-5 resolution change, not the login prompt.
