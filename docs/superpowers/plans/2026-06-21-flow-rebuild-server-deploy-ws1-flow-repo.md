# flow `rebuild` Server Deploy — WS1 (flow repo) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `rebuild` branch produce a deployable flow-server container image and re-add multi-issuer OIDC token verification, so the homelab-study GitOps deploy (Plan 2) can pin the image digest and run prod auth.

**Architecture:** Three small flow-repo changes — (1) a multi-issuer OIDC verifier with **per-issuer audiences** in `internal/adapter/oidcverify` (restoring what `next` had, but stricter), (2) a simplified `deploy/podman/Dockerfile.server` (no Node — assets are `go:embed`-ed), (3) a `build-server-image.yml` GHA workflow — then push `rebuild` to origin and record the built `@sha256` digest.

**Tech Stack:** Go 1.25.7, `github.com/coreos/go-oidc/v3` v3.18.0, `github.com/golang-jwt/jwt/v5` v5.3.1 (test-only minting), `a-h/templ` (go.mod tool directive), distroless runtime, GitHub Actions + buildx.

## Global Constraints

- **Go 1.25.7**; templ is a **go.mod tool directive** — generate via `go tool templ generate ./...` (NOT `go run …@version`).
- **`make ci` must stay green**: `ci: lint verify-generate cover build` → golangci-lint 0 issues, `*_templ.go` up to date, **coverage ≥ 80%** (`COVER_THRESHOLD=80`, `COVER_PKG=./internal/...`), build OK.
- **No new dependencies** — go-oidc v3.18.0, golang-jwt/jwt v5.3.1, golang.org/x/oauth2 v0.36.0 are already in `go.mod`.
- **`ports.Identity` shape is fixed:** `{Subject, Username, Email, Name, Groups}` (in `internal/ports`). Do NOT adopt `next`'s `{Sub, EmailVerified, …}` shape — preserve the `rebuild` field names and the `preferred_username → Username` mapping.
- **`FLOW_OIDC_CLI_ISSUER` is OPTIONAL** — it must NOT be added to `config.Load`'s required-env list. Empty ⇒ today's single-issuer behaviour (dev: one Dex issuer, two audiences).
- **Per-issuer audiences, not a union set** — a token from the web issuer carrying the CLI audience (or vice-versa) MUST be rejected. This is the tightness the design chose over `next`'s union model.
- **"Keine Monolithen":** focused files per responsibility; `cmd/flow-server/main.go` stays wiring-only.
- The image is built by **CI**, not merged to main: tags are `:<sha>` always + `:rebuild` on the branch. `rebuild` is the long-lived integration branch and is NOT merged to `main`.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `internal/config/config.go` | add optional `OIDCCliIssuer` field + load | Modify |
| `internal/config/config_test.go` | assert optional parse + not-required | Modify |
| `internal/adapter/oidcverify/verifier.go` | multi-issuer verifier mechanics (`IssuerAudiences`, `New`, per-issuer `Verify`) | Modify |
| `internal/adapter/oidcverify/verifier_test.go` | migrate black-box discovery tests to slice API | Modify |
| `internal/adapter/oidcverify/verifier_multi_test.go` | white-box multi-issuer + per-issuer-audience strictness (StaticKeySet) | Create |
| `internal/adapter/oidcverify/pairs.go` | `VerifierPairs` policy (prod two-issuer / dev fallback) | Create |
| `internal/adapter/oidcverify/pairs_test.go` | pure-function policy tests | Create |
| `cmd/flow-server/main.go` | wire `VerifierPairs` + new `New` signature | Modify (line ~56) |
| `deploy/podman/Dockerfile.server` | simplified container image | Create |
| `.dockerignore` | keep build context clean (no `.git`/`bin`) | Create |
| `.github/workflows/build-server-image.yml` | build+push image to GHCR | Create |

---

### Task 1: config — optional `OIDCCliIssuer`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.OIDCCliIssuer string` — read from `FLOW_OIDC_CLI_ISSUER`, empty allowed. Consumed by Task 3 (`VerifierPairs`).

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go` (the happy-path test already sets the other vars; add the CLI issuer there and a new assertion, plus a not-required test):

```go
func TestLoadOptionalCliIssuer(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://id.example/application/o/flow/",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "shh",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example",
		"FLOW_SESSION_SECRET":     "sess",
		"FLOW_OIDC_CLI_ISSUER":    "https://id.example/application/o/flow-cli/",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OIDCCliIssuer != "https://id.example/application/o/flow-cli/" {
		t.Fatalf("OIDCCliIssuer = %q", c.OIDCCliIssuer)
	}

	// Optional: Load must still succeed with FLOW_OIDC_CLI_ISSUER unset.
	delete(env, "FLOW_OIDC_CLI_ISSUER")
	c2, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load without cli issuer must succeed: %v", err)
	}
	if c2.OIDCCliIssuer != "" {
		t.Fatalf("OIDCCliIssuer should be empty, got %q", c2.OIDCCliIssuer)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadOptionalCliIssuer -v`
Expected: FAIL — `c.OIDCCliIssuer undefined (type Config has no field or method OIDCCliIssuer)`.

- [ ] **Step 3: Add the field + load (NOT required)**

In `internal/config/config.go`, add the struct field after `OIDCIssuer`:

```go
	OIDCIssuer       string
	OIDCCliIssuer    string
	OIDCClientID     string
```

And in `Load`, after the `OIDCIssuer` line:

```go
		OIDCIssuer:       getenv("FLOW_OIDC_ISSUER"),
		OIDCCliIssuer:    getenv("FLOW_OIDC_CLI_ISSUER"),
		OIDCClientID:     getenv("FLOW_OIDC_CLIENT_ID"),
```

Do **not** add `FLOW_OIDC_CLI_ISSUER` to the required-env loop — it stays optional.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoadOptionalCliIssuer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): optional FLOW_OIDC_CLI_ISSUER"
```

---

### Task 2: oidcverify — multi-issuer verifier with per-issuer audiences

**Files:**
- Modify: `internal/adapter/oidcverify/verifier.go`
- Modify: `internal/adapter/oidcverify/verifier_test.go` (migrate to new signature)
- Create: `internal/adapter/oidcverify/verifier_multi_test.go` (white-box)

**Interfaces:**
- Produces:
  - `type IssuerAudiences struct { Issuer string; Audiences []string }`
  - `func New(ctx context.Context, pairs []IssuerAudiences) (*Verifier, error)` — **signature change** from `New(ctx, issuer string, audiences []string)`.
  - `func (vr *Verifier) Verify(ctx, raw string) (ports.Identity, error)` — unchanged signature; per-issuer audience enforcement.
- Consumed by Task 3 (main wiring via `VerifierPairs`).

- [ ] **Step 1: Migrate the existing black-box tests to the new slice API (these are the "make it fail to compile" driver)**

Rewrite the four call sites in `internal/adapter/oidcverify/verifier_test.go` from the two-arg form to a single-pair slice. The harness and assertions stay; only the `New(...)` calls change:

```go
// TestVerifyValidToken
v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []string{"flow"}}})

// TestVerifyRejectsExpiredToken
v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []string{"flow"}}})

// TestVerifyRejectsWrongAudience
v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []string{"flow"}}})

// TestVerifyAcceptsSecondAudience  (single issuer, two audiences — the dev-fallback shape)
v, err := oidcverify.New(ctx, []oidcverify.IssuerAudiences{{Issuer: h.issuer, Audiences: []string{"flow", "flow-cli"}}})
```

- [ ] **Step 2: Create the white-box multi-issuer test**

Create `internal/adapter/oidcverify/verifier_multi_test.go` (note `package oidcverify` — white-box, so it can build the unexported verifier struct with `oidc.StaticKeySet`, no network):

```go
package oidcverify

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// mintRS256 signs a minimal token with the given key/issuer/audience.
func mintRS256(t *testing.T, key *rsa.PrivateKey, iss, aud string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                iss,
		"aud":                aud,
		"sub":                "msoent",
		"preferred_username": "msoent",
		"email":              "m@x.de",
		"name":               "Martin",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	})
	tok.Header["kid"] = "k"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// staticIssuerVerifier builds a network-free issuerVerifier bound to one issuer,
// one key, and one allowed audience set.
func staticIssuerVerifier(iss string, pub *rsa.PublicKey, auds ...string) issuerVerifier {
	allow := map[string]bool{}
	for _, a := range auds {
		allow[a] = true
	}
	return issuerVerifier{
		v: oidc.NewVerifier(iss,
			&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{pub}},
			&oidc.Config{SkipClientIDCheck: true}),
		auds: allow,
	}
}

func TestVerifyMultiIssuerPerIssuerAudience(t *testing.T) {
	keyA, keyB := genKey(t), genKey(t)
	const (
		issA = "https://id.example/application/o/flow/"
		issB = "https://id.example/application/o/flow-cli/"
	)
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(issA, &keyA.PublicKey, "flow"),
		staticIssuerVerifier(issB, &keyB.PublicKey, "flow-cli"),
	}}
	ctx := context.Background()

	// Web token on the web issuer → accepted, identity mapped.
	id, err := vr.Verify(ctx, mintRS256(t, keyA, issA, "flow"))
	if err != nil {
		t.Fatalf("web token: %v", err)
	}
	if id.Subject != "msoent" || id.Username != "msoent" {
		t.Fatalf("identity mismatch: %+v", id)
	}

	// CLI token on the CLI issuer (the path that regressed) → accepted.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, issB, "flow-cli")); err != nil {
		t.Fatalf("cli token: %v", err)
	}

	// Per-issuer strictness: CLI audience presented on the WEB issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyA, issA, "flow-cli")); err == nil {
		t.Fatal("expected reject: cli audience on web issuer")
	}

	// Per-issuer strictness: web audience on the CLI issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, issB, "flow")); err == nil {
		t.Fatal("expected reject: web audience on cli issuer")
	}

	// Forged: issB claim signed with key A → signature reject (defence-in-depth).
	if _, err := vr.Verify(ctx, mintRS256(t, keyA, issB, "flow-cli")); err == nil {
		t.Fatal("expected reject: issB claim signed with key A")
	}

	// Untrusted issuer → reject.
	if _, err := vr.Verify(ctx, mintRS256(t, keyB, "https://id.example/application/o/evil/", "flow-cli")); err == nil {
		t.Fatal("expected reject: untrusted issuer")
	}
}

func TestNewRejectsEmptyPairs(t *testing.T) {
	if _, err := New(context.Background(), nil); err == nil {
		t.Fatal("expected error for empty pairs")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail to compile**

Run: `go test ./internal/adapter/oidcverify/ -v`
Expected: FAIL to compile — `New` takes `(context.Context, string, []string)` not the slice; `issuerVerifier` / `Verifier.verifiers` undefined.

- [ ] **Step 4: Rewrite `verifier.go` to the multi-issuer mechanics**

Replace the body of `internal/adapter/oidcverify/verifier.go` (keep the package doc + `claims` struct + the `ports.Identity` mapping intact):

```go
// Package oidcverify verifies Authentik/Dex-issued JWT tokens against a set of
// trusted issuers, each bound to the audiences accepted for that issuer.
package oidcverify

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/ports"
)

// IssuerAudiences binds one trusted OIDC issuer to the audiences (client_ids)
// accepted for tokens minted by that issuer.
type IssuerAudiences struct {
	Issuer    string
	Audiences []string
}

// issuerVerifier is one issuer's token verifier plus its accepted-audience set.
type issuerVerifier struct {
	v    *oidc.IDTokenVerifier
	auds map[string]bool
}

// Verifier validates tokens against several issuers. Authentik runs in
// per_provider issuer mode, so the browser provider and the CLI/device provider
// mint tokens with distinct `iss` values AND sign against distinct JWKS; a
// single-issuer verifier rejects the other before any audience check.
type Verifier struct {
	verifiers []issuerVerifier
}

// New runs OIDC discovery for each issuer (fetching its discovery doc + JWKS)
// and builds one verifier per issuer. Each verifier skips go-oidc's built-in
// single-client_id audience check; Verify re-applies a stricter PER-ISSUER
// audience check. Discovery failure on any issuer fails loudly.
func New(ctx context.Context, pairs []IssuerAudiences) (*Verifier, error) {
	if len(pairs) == 0 {
		return nil, fmt.Errorf("oidcverify: no issuer/audience pairs")
	}
	vs := make([]issuerVerifier, 0, len(pairs))
	for _, p := range pairs {
		if p.Issuer == "" {
			return nil, fmt.Errorf("oidcverify: empty issuer")
		}
		prov, err := oidc.NewProvider(ctx, p.Issuer)
		if err != nil {
			return nil, fmt.Errorf("oidcverify: provider(%s): %w", p.Issuer, err)
		}
		auds := make(map[string]bool, len(p.Audiences))
		for _, a := range p.Audiences {
			if a != "" {
				auds[a] = true
			}
		}
		vs = append(vs, issuerVerifier{
			v:    prov.Verifier(&oidc.Config{SkipClientIDCheck: true}),
			auds: auds,
		})
	}
	return &Verifier{verifiers: vs}, nil
}

type claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// Verify tries each issuer's verifier; the first whose signature, `iss`, and
// `exp` check out OWNS the token, and that issuer's audience set must then
// contain at least one of the token's audiences (per-issuer tightness). If no
// issuer accepts the token, the last verifier error is returned.
func (vr *Verifier) Verify(ctx context.Context, raw string) (ports.Identity, error) {
	var lastErr error
	for _, iv := range vr.verifiers {
		tok, err := iv.v.Verify(ctx, raw)
		if err != nil {
			lastErr = err
			continue
		}
		ok := false
		for _, a := range tok.Audience {
			if iv.auds[a] {
				ok = true
				break
			}
		}
		if !ok {
			return ports.Identity{}, fmt.Errorf("oidcverify: audience %v not allowed for issuer %s", tok.Audience, tok.Issuer)
		}
		var c claims
		if err := tok.Claims(&c); err != nil {
			return ports.Identity{}, fmt.Errorf("oidcverify: claims: %w", err)
		}
		return ports.Identity{Subject: c.Sub, Username: c.PreferredUsername, Email: c.Email, Name: c.Name, Groups: c.Groups}, nil
	}
	return ports.Identity{}, fmt.Errorf("oidcverify: no trusted issuer accepted token: %w", lastErr)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/oidcverify/ -v`
Expected: PASS — both the migrated black-box discovery tests and the white-box multi-issuer tests.

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/oidcverify/
git commit -m "feat(oidcverify): multi-issuer verifier with per-issuer audiences"
```

---

### Task 3: oidcverify — `VerifierPairs` policy + main.go wiring

**Files:**
- Create: `internal/adapter/oidcverify/pairs.go`
- Create: `internal/adapter/oidcverify/pairs_test.go`
- Modify: `cmd/flow-server/main.go` (the `oidcverify.New(...)` call, ~line 56)

**Interfaces:**
- Consumes: `IssuerAudiences`, `New` (Task 2); `Config.OIDCCliIssuer` (Task 1).
- Produces: `func VerifierPairs(webIssuer, webClient, cliIssuer, cliClient string) []IssuerAudiences`.

- [ ] **Step 1: Write the failing policy test**

Create `internal/adapter/oidcverify/pairs_test.go` (`package oidcverify_test`, pure — no network):

```go
package oidcverify_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcverify"
)

func TestVerifierPairs(t *testing.T) {
	type IA = oidcverify.IssuerAudiences
	cases := []struct {
		name                                       string
		webIssuer, webClient, cliIssuer, cliClient string
		want                                       []IA
	}{
		{
			name:      "prod two distinct issuers → per-issuer audiences",
			webIssuer: "https://id/o/flow/", webClient: "flow",
			cliIssuer: "https://id/o/flow-cli/", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "https://id/o/flow/", Audiences: []string{"flow"}},
				{Issuer: "https://id/o/flow-cli/", Audiences: []string{"flow-cli"}},
			},
		},
		{
			name:      "dev empty cli issuer → one issuer accepts both clients",
			webIssuer: "http://localhost:5556/dex", webClient: "flow-dev",
			cliIssuer: "", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []string{"flow-dev", "flow-cli"}},
			},
		},
		{
			name:      "explicit equal issuers → collapse to one with both audiences",
			webIssuer: "http://localhost:5556/dex", webClient: "flow-dev",
			cliIssuer: "http://localhost:5556/dex", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []string{"flow-dev", "flow-cli"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := oidcverify.VerifierPairs(tc.webIssuer, tc.webClient, tc.cliIssuer, tc.cliClient)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("VerifierPairs = %+v, want %+v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/oidcverify/ -run TestVerifierPairs -v`
Expected: FAIL — `undefined: oidcverify.VerifierPairs`.

- [ ] **Step 3: Create `pairs.go`**

```go
package oidcverify

// VerifierPairs derives issuer→audience bindings from flow's OIDC config.
//
// Prod (Authentik per_provider): webIssuer and cliIssuer differ, so each issuer
// is bound to ITS OWN client only — a web-issued token may not carry the CLI
// audience and vice-versa (per-issuer tightness).
//
// Dev (a single Dex issuer fronting both clients): cliIssuer is empty, or equal
// to webIssuer, so one issuer is returned that accepts BOTH client audiences —
// preserving the pre-multi-issuer single-issuer behaviour.
func VerifierPairs(webIssuer, webClient, cliIssuer, cliClient string) []IssuerAudiences {
	if cliIssuer != "" && cliIssuer != webIssuer {
		return []IssuerAudiences{
			{Issuer: webIssuer, Audiences: []string{webClient}},
			{Issuer: cliIssuer, Audiences: []string{cliClient}},
		}
	}
	return []IssuerAudiences{
		{Issuer: webIssuer, Audiences: []string{webClient, cliClient}},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapter/oidcverify/ -run TestVerifierPairs -v`
Expected: PASS.

- [ ] **Step 5: Wire it into `cmd/flow-server/main.go`**

Replace the existing verifier construction (currently line ~56):

```go
	verifier, err := oidcverify.New(ctx, cfg.OIDCIssuer, []string{cfg.OIDCClientID, cfg.OIDCCliClientID})
```

with:

```go
	verifier, err := oidcverify.New(ctx, oidcverify.VerifierPairs(
		cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCCliIssuer, cfg.OIDCCliClientID))
```

Leave the `oidcauth.New(...)` call (web auth-code flow) unchanged — it is correctly single-issuer.

- [ ] **Step 6: Verify the whole module builds + tests pass**

Run: `go build ./cmd/flow-server && go test ./internal/adapter/oidcverify/ ./internal/config/ -v`
Expected: build OK, all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/oidcverify/pairs.go internal/adapter/oidcverify/pairs_test.go cmd/flow-server/main.go
git commit -m "feat(oidcverify): VerifierPairs policy + wire multi-issuer into flow-server"
```

---

### Task 4: Dockerfile.server (simplified port)

**Files:**
- Create: `deploy/podman/Dockerfile.server`
- Create: `.dockerignore`

**Interfaces:**
- Consumes: nothing from earlier tasks. Produces a buildable image consumed by Task 5 (CI) and Plan 2 (digest pin).

- [ ] **Step 1: Create `.dockerignore`**

Keeps the worktree's `.git` file and local build output out of the build context (the `.git` in a worktree is a *file* pointing at the main repo, which would break VCS stamping — we disable that with `-buildvcs=false` regardless):

```
.git
bin/
coverage.out
*.test
```

- [ ] **Step 2: Create `deploy/podman/Dockerfile.server`**

```dockerfile
# syntax=docker/dockerfile:1.7
#
# flow-server image (rebuild branch).
#
# Builder: golang:1.25-alpine (matches go.mod toolchain go 1.25.7). Unlike the
# next chart's image there is NO Node/npm step — the WebUI assets are committed
# and go:embed-ed: internal/adapter/webui/static/app.css plus the generated
# *_templ.go. templ is a go.mod tool directive, so we regenerate via
# `go tool templ generate ./...` (a no-op against the committed files, kept for
# a clean-room build).
#
# -buildvcs=false: the build runs from a git worktree whose .git is a file (and
# CI checks out a detached tree); flow-server reads no VCS/version stamp, so we
# disable VCS stamping to keep both local `podman build` and CI deterministic.
#
# Runtime: distroless/static-debian12:nonroot — no shell, uid 65532.
#
# Build: podman build -t flow-server:dev -f deploy/podman/Dockerfile.server .

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go tool templ generate ./...
RUN CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false" \
    go build -ldflags="-s -w" -o /out/flow-server ./cmd/flow-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/flow-server /usr/local/bin/flow-server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/flow-server"]
```

- [ ] **Step 3: Verify the build steps locally (no Docker daemon needed for the Go parts)**

Run the exact builder commands against the working tree to prove they succeed before CI runs them:

```bash
go tool templ generate ./...
CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false" go build -ldflags="-s -w" -o /tmp/flow-server ./cmd/flow-server
ls -l /tmp/flow-server && rm /tmp/flow-server
```

Expected: templ generates with no diff (committed files current), binary builds.

Optional (if a container runtime is available): `podman build -t flow-server:dev -f deploy/podman/Dockerfile.server .` — expected: image builds, final stage is distroless.

- [ ] **Step 4: Confirm no generated drift (make ci's verify-generate will enforce this)**

Run: `git diff --quiet -- ':*_templ.go' && echo "no templ drift" || echo "DRIFT — commit regenerated files"`
Expected: `no templ drift`.

- [ ] **Step 5: Commit**

```bash
git add deploy/podman/Dockerfile.server .dockerignore
git commit -m "build(deploy): simplified flow-server Dockerfile (no node, go:embed assets)"
```

---

### Task 5: build-server-image.yml (GHA workflow)

**Files:**
- Create: `.github/workflows/build-server-image.yml`

**Interfaces:**
- Consumes: `deploy/podman/Dockerfile.server` (Task 4). Produces the pushed GHCR image whose digest Plan 2 pins.

- [ ] **Step 1: Create the workflow**

```yaml
name: Build flow-server image

# Builds the flow-server container image and pushes to GHCR.
#
# rebuild is the long-lived integration branch for the server-only green-field
# rewrite; it is NOT merged to main. Pushes here build a digest-pinnable image
# tagged with the commit SHA plus the branch name (`:rebuild`). The `main` entry
# is harmless dead weight (this workflow file lives only on rebuild) and is kept
# to mirror the next pipeline.
#
# Multi-arch: linux/amd64 (cluster nodes) + linux/arm64 (Apple Silicon). QEMU
# emulates the off-host arch.

on:
  push:
    branches: [main, rebuild]
    paths:
      - 'cmd/flow-server/**'
      - 'internal/**'
      - 'web/tailwind.css'
      - 'deploy/podman/Dockerfile.server'
      - 'go.mod'
      - 'go.sum'
      - 'Makefile'
      - '.github/workflows/build-server-image.yml'

permissions:
  contents: read
  packages: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Buildx
        uses: docker/setup-buildx-action@v3

      - name: Compute tags
        id: tags
        run: |
          set -euo pipefail
          OWNER=$(echo "${{ github.repository_owner }}" | tr '[:upper:]' '[:lower:]')
          IMAGE="ghcr.io/${OWNER}/flow-server"
          TAGS="${IMAGE}:${{ github.sha }}"
          BRANCH="${GITHUB_REF##*/}"
          if [ "$BRANCH" = "main" ]; then
            TAGS="${TAGS},${IMAGE}:latest"
          else
            TAGS="${TAGS},${IMAGE}:${BRANCH}"
          fi
          echo "tags=${TAGS}" >> "$GITHUB_OUTPUT"
          echo "Computed tags: ${TAGS}"

      - name: Build and push
        id: build
        uses: docker/build-push-action@v5
        with:
          context: .
          file: deploy/podman/Dockerfile.server
          push: true
          platforms: linux/amd64,linux/arm64
          tags: ${{ steps.tags.outputs.tags }}
          labels: |
            org.opencontainers.image.source=https://github.com/${{ github.repository }}
            org.opencontainers.image.revision=${{ github.sha }}
            org.opencontainers.image.title=flow-server
          cache-from: type=gha,scope=flow-server
          cache-to: type=gha,mode=max,scope=flow-server

      - name: Print image digest
        run: echo "Image digest:: ${{ steps.build.outputs.digest }}"
```

(`-buildvcs=false` in the Dockerfile means we drop `next`'s `fetch-depth: 0` and `VERSION` build-arg — no VCS stamp is consumed.)

- [ ] **Step 2: Lint the YAML / sanity-check paths**

Run: `go tool yamlfmt -lint .github/workflows/build-server-image.yml 2>/dev/null || python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/build-server-image.yml')); print('yaml ok')"`
Expected: `yaml ok` (or yamlfmt clean). Confirm `file: deploy/podman/Dockerfile.server` matches Task 4's path.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build-server-image.yml
git commit -m "ci(deploy): build+push flow-server image to GHCR on rebuild"
```

---

### Task 6: Final gate — `make ci` + dev fallback smoke + push + record digest

> **Main-thread + user-gated.** Do NOT delegate the push to a subagent (subagent commits/pushes can be branch-isolated — see the project's git-isolation lesson). The push is outward-facing and first-of-its-kind for this branch; confirm with the user before it runs.

**Files:** none (verification + git).

- [ ] **Step 1: Full CI gate**

Run: `make ci`
Expected: `lint` 0 issues, `verify-generate: OK`, coverage **≥ 80%**, build OK. If coverage dipped below 80, add a missing-case test in the task that introduced the gap (the new code in Tasks 2–3 is already covered; a dip would point at an untested branch).

- [ ] **Step 2: Dev fallback smoke — prove the single-issuer dev path is unchanged**

The design's load-bearing invariant: with `FLOW_OIDC_CLI_ISSUER` unset, dev (one Dex issuer, two audiences) keeps working. Verify end-to-end against the local stack:

```bash
make dev-up
make dev-token   # mints a CLI token via the public flow-cli client against Dex
make dev-run &   # starts flow-server with deploy/dev/flow.env (no FLOW_OIDC_CLI_ISSUER)
sleep 2
set -a; . deploy/dev/flow-cli.env; set +a
go run ./cmd/flow whoami    # expected: 200, prints user msoent
```

Expected: `flow whoami` returns the dev identity (the CLI token's `aud=flow-cli` is accepted by the single Dex issuer via the both-audiences fallback pair). Tear down: `make dev-down` and kill the backgrounded `dev-run`.

If `whoami` 401s, the fallback pair is wrong — re-check `VerifierPairs` returns one pair with BOTH audiences when `cfg.OIDCCliIssuer == ""`.

- [ ] **Step 3: Confirm the branch state is clean and ahead of origin only on rebuild**

```bash
git status --short            # expected: clean
git log --oneline origin/main..HEAD | head   # the rebuild-only commits (origin has no rebuild branch yet)
```

- [ ] **Step 4: GATE — ask the user to confirm the first push of `rebuild` to origin**

State explicitly: this is the first push of the local-only `rebuild` branch to `origin`, it will trigger `build-server-image.yml`, and it does not touch `main`/`next`. Wait for an explicit "yes".

- [ ] **Step 5: Push (after confirmation)**

```bash
git push -u origin rebuild
```

- [ ] **Step 6: Watch CI and record the digest**

```bash
gh run list --branch rebuild --workflow "Build flow-server image" --limit 1
gh run watch <run-id>   # or: gh run view <run-id> --log | rg -i "digest"
```

Record the resulting `ghcr.io/serverkraken/flow-server@sha256:…` digest — it is the input to Plan 2 (homelab-study WS2c image pin). Also confirm the `:rebuild` and `:<sha>` tags exist:

```bash
gh api /orgs/serverkraken/packages/container/flow-server/versions --jq '.[0].metadata.container.tags' 2>/dev/null || echo "(check GHCR UI for tags)"
```

- [ ] **Step 7: Final commit (if any digest record file is kept) / done**

No code commit needed here. The deliverable is: `rebuild` on origin + a recorded image digest. Hand off to Plan 2.

---

## Self-Review

**1. Spec coverage (design WS1 a–d):**
- 1a Dockerfile (simplified, no node, templ tool, embed) → Task 4 ✓
- 1b build-server-image.yml (branches, paths, tags, multi-arch) → Task 5 ✓
- 1c multi-issuer verifier: config `OIDCCliIssuer` (Task 1) + `oidcverify` per-issuer (Task 2) + main wiring with empty-cli fallback (Task 3) ✓; `oidcauth` untouched ✓; per-issuer audiences (not union) ✓
- 1d push rebuild + record digest → Task 6 ✓
- Dev-env-impact required test cases: single-issuer fallback parse (Task 1), fallback-pair shape (Task 3 `pairs_test`), end-to-end dev whoami (Task 6 Step 2) ✓

**2. Placeholder scan:** No TBD/TODO; every code step shows full code; commands have expected output. ✓

**3. Type consistency:** `IssuerAudiences{Issuer, Audiences}`, `New(ctx, []IssuerAudiences)`, `issuerVerifier{v, auds}`, `Verifier{verifiers}`, `VerifierPairs(webIssuer, webClient, cliIssuer, cliClient) []IssuerAudiences`, `Config.OIDCCliIssuer` — names match across Tasks 1–3 and the white-box test. `ports.Identity{Subject, Username, Email, Name, Groups}` preserved (verified in `internal/ports`). ✓

## Out of scope (→ Plan 2: homelab-study)

pgvector CNPG image, fresh DB, Deployment env rewrite + digest pin, secrets (`session_secret`), Ollama `nomic-embed-text` pull, Authentik blueprint verify, ArgoCD sync, and the live WebUI/CLI/flow-mcp dogfood gate. Plan 2 starts once Task 6 yields the image digest.
