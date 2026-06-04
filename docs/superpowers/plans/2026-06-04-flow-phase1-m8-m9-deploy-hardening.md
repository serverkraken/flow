# flow Phase 1 — M8+M9 Deploy + Hardening Implementation Plan (Plan F)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take `flow-server` from "runs locally with `go run ./cmd/flow-server`" to "Soenne deploys it to his K8s cluster and uses it daily across laptops + browser, with backups he trusts." M8 ships the container plus Helm chart; M9 wraps Prometheus metrics, structured logging, retry policies for transient failures, and a Litestream-restore drill that proves the backup actually works.

**Architecture:** Single-container deploy with a Litestream sidecar that replicates the SQLite WAL to S3-compatible storage (Backblaze B2, Minio, or whatever Soenne has). One Pod, one PVC for `/var/lib/flow`, one Ingress fronted by cert-manager. The TLS + OIDC config comes from a Helm `values.yaml` that points at the existing Authentik issuer URL. Observability is minimal-but-real: structured JSON logging to stdout, Prometheus `/metrics`, no APM/tracing in Phase 1.

**Tech Stack:** Distroless Go base image (`gcr.io/distroless/static-debian12:nonroot`), Litestream as sidecar container (`benbjohnson/litestream`), Helm 3.x charts, GitHub Actions to build + push images to ghcr.io, cert-manager + nginx-ingress (or Traefik — Soenne's cluster choice) for TLS. Metrics: `github.com/prometheus/client_golang`, no Otel in Phase 1.

**Prerequisite:** All earlier milestones (M1-M7) on `next`. Plan F is the merge gate from `next` → `main` per `feedback_long_lived_integration_branch.md`.

---

## File Structure

**Create (deploy/podman):**
- `deploy/podman/docker-compose.yml` — local dev: flow-server + dex + minio + litestream sidecar.
- `deploy/podman/Dockerfile.server` — multi-stage build (alpine for cgo-free Go, distroless final).
- `deploy/podman/.env.example` — `FLOW_SERVER_URL`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `COOKIE_KEYS`, `LITESTREAM_REPLICA_URL`, `LITESTREAM_ACCESS_KEY_ID`, `LITESTREAM_SECRET_ACCESS_KEY`.
- `deploy/podman/dev-authentik.md` — optional dev-time Authentik docker-compose for offline testing.

**Create (deploy/helm):**
- `deploy/helm/flow-server/Chart.yaml`
- `deploy/helm/flow-server/values.yaml` (+ `values-soenne.yaml` for the actual deploy)
- `deploy/helm/flow-server/templates/deployment.yaml` (flow-server container + litestream sidecar)
- `deploy/helm/flow-server/templates/service.yaml`
- `deploy/helm/flow-server/templates/ingress.yaml`
- `deploy/helm/flow-server/templates/configmap.yaml`
- `deploy/helm/flow-server/templates/secret.yaml` (or `externalsecret.yaml` if Soenne uses External Secrets Operator)
- `deploy/helm/flow-server/templates/pvc.yaml`
- `deploy/helm/flow-server/templates/servicemonitor.yaml` (Prometheus Operator scrape target)
- `deploy/helm/flow-server/templates/_helpers.tpl`
- `deploy/helm/flow-server/.helmignore`

**Create (CI/CD):**
- `.github/workflows/build-server-image.yml` — on push to `main`, build + push `ghcr.io/<owner>/flow-server:<sha>` and `:latest`.
- `.github/workflows/helm-lint.yml` — `helm lint` + `helm template` on PRs touching `deploy/helm/`.

**Modify (server code — M9 hardening):**
- `internal/adapter/httpserver/metrics.go` — Prometheus collectors + `/metrics` handler.
- `internal/adapter/httpserver/logging.go` — request-logging middleware (method, path, status, latency, user-sub).
- `internal/adapter/httpsync/retry.go` — exponential backoff for the Worker's transient errors (5xx, network).
- `cmd/flow-server/main.go` — structured logger via `slog.NewJSONHandler(os.Stdout, ...)` (or text in dev), wired into every adapter.
- `cmd/flow-server/main.go` — graceful shutdown: 30s drain on SIGTERM.

**Create (M9 backup verification):**
- `scripts/litestream-restore-drill.sh` — fresh PVC + `litestream restore` against the configured replica + invariant check (row counts match production within tolerance).
- `docs/runbook/litestream-restore.md` — manual procedure for "I need to recover from backup".

**Create (M9 release):**
- `CHANGELOG.md` — first entry for `flow-server v0.1.0`.
- `deploy/helm/flow-server/values-soenne.yaml.example` — Soenne's actual values, committed as `.example` because the real one carries secrets-reference paths.

---

## Phase A: M8 Container + Compose

### Task 1: Dockerfile

**Files:**
- Create: `deploy/podman/Dockerfile.server`
- Create: `deploy/podman/.dockerignore`

- [ ] **Step 1: Multi-stage build**

```dockerfile
# deploy/podman/Dockerfile.server
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git nodejs npm make
COPY . .
RUN cd tools/tailwind && npm ci
RUN cd tools/codemirror && npm ci
RUN make webui
RUN CGO_ENABLED=0 GOFLAGS="-trimpath" go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags --dirty 2>/dev/null || echo dev)" \
    -o /out/flow-server ./cmd/flow-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/flow-server /usr/local/bin/flow-server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/flow-server"]
```

- [ ] **Step 2: .dockerignore**

```
.git
node_modules
**/node_modules
bin/
dist/
coverage.out
*.db
docs/
```

- [ ] **Step 3: Build smoke**

```bash
podman build -t flow-server:dev -f deploy/podman/Dockerfile.server .
podman run --rm -p 8080:8080 -e FLOW_SERVER_URL=http://localhost:8080 flow-server:dev &
sleep 2
curl -fsS http://localhost:8080/healthz
podman kill $(podman ps -q -f ancestor=flow-server:dev)
```

- [ ] **Step 4: Commit**

```bash
git add deploy/podman/Dockerfile.server deploy/podman/.dockerignore
git commit -m "feat(deploy): podman/docker multi-stage Dockerfile for flow-server"
```

### Task 2: Local dev compose

**Files:**
- Create: `deploy/podman/docker-compose.yml`
- Create: `deploy/podman/.env.example`

- [ ] **Step 1: Compose**

```yaml
# deploy/podman/docker-compose.yml
version: "3.9"

services:
  flow-server:
    build:
      context: ../..
      dockerfile: deploy/podman/Dockerfile.server
    image: flow-server:dev
    ports: ["8080:8080"]
    env_file: .env
    volumes:
      - flow-data:/var/lib/flow
    depends_on: [dex, minio]
    healthcheck:
      test: ["CMD", "/usr/local/bin/flow-server", "--healthcheck"]
      interval: 10s

  litestream:
    image: litestream/litestream:0.3.13
    depends_on: [flow-server]
    volumes:
      - flow-data:/var/lib/flow
      - ./litestream.yml:/etc/litestream.yml
    command: ["replicate", "-config", "/etc/litestream.yml"]
    env_file: .env

  dex:
    image: dexidp/dex:v2.40.0
    ports: ["5556:5556"]
    volumes: [./dex-config.yaml:/etc/dex/config.yaml]
    command: ["dex", "serve", "/etc/dex/config.yaml"]

  minio:
    image: minio/minio:latest
    ports: ["9000:9000", "9001:9001"]
    environment:
      MINIO_ROOT_USER: minio
      MINIO_ROOT_PASSWORD: minio123
    command: ["server", "/data", "--console-address", ":9001"]
    volumes: [minio-data:/data]

volumes:
  flow-data:
  minio-data:
```

- [ ] **Step 2: .env.example**

Lists every variable the server reads, with comments + defaults.

- [ ] **Step 3: litestream.yml**

```yaml
# deploy/podman/litestream.yml
dbs:
  - path: /var/lib/flow/server.db
    replicas:
      - type: s3
        bucket: flow-backups
        path: server.db
        endpoint: http://minio:9000
        access-key-id: ${LITESTREAM_ACCESS_KEY_ID}
        secret-access-key: ${LITESTREAM_SECRET_ACCESS_KEY}
        force-path-style: true
        sync-interval: 1s
        retention: 168h  # 1 week
```

- [ ] **Step 4: Smoke**

```bash
cd deploy/podman
cp .env.example .env
podman compose up -d
sleep 5
curl -fsS http://localhost:8080/readyz
podman compose logs litestream | grep "initialized" # litestream is replicating
podman compose down
```

- [ ] **Step 5: Commit**

```bash
git add deploy/podman/
git commit -m "feat(deploy): docker-compose with flow-server + litestream + dex + minio"
```

### Task 3: Helm chart skeleton

**Files:**
- Create: `deploy/helm/flow-server/Chart.yaml`, `values.yaml`, `.helmignore`
- Create: `deploy/helm/flow-server/templates/_helpers.tpl`, `deployment.yaml`, `service.yaml`

- [ ] **Step 1: Chart.yaml**

```yaml
apiVersion: v2
name: flow-server
description: Multi-device worktime + repo notes server
type: application
version: 0.1.0
appVersion: "0.1.0"
maintainers:
  - name: Soenne
```

- [ ] **Step 2: values.yaml**

```yaml
image:
  repository: ghcr.io/<owner>/flow-server
  tag: ""  # falls back to .Chart.AppVersion
  pullPolicy: IfNotPresent

replicaCount: 1  # SQLite means single-writer; never bump this

env:
  FLOW_SERVER_URL: "https://flow.example.com"
  OIDC_ISSUER: "https://auth.example.com/application/o/flow/"
  OIDC_CLIENT_ID: "flow-server"
  ALLOWED_SUBS: "soenne-sub-uuid"

secret:
  oidcClientSecret: ""  # required, set via --set or external secret
  cookieKeys: ""        # 32+32 random bytes, base64 — server signs cookies
  litestreamAccessKeyID: ""
  litestreamSecretAccessKey: ""

persistence:
  enabled: true
  storageClass: ""  # uses cluster default
  size: 4Gi

litestream:
  enabled: true
  image: litestream/litestream:0.3.13
  replicaURL: ""  # s3://bucket/path
  syncInterval: 1s

ingress:
  enabled: true
  className: nginx
  host: flow.example.com
  tlsSecret: flow-server-tls

resources:
  flowServer:
    requests: { cpu: 50m, memory: 64Mi }
    limits:   { cpu: 500m, memory: 256Mi }
  litestream:
    requests: { cpu: 10m, memory: 32Mi }
    limits:   { cpu: 100m, memory: 64Mi }

metrics:
  enabled: true
  serviceMonitor:
    enabled: false  # set true when Prometheus Operator is in the cluster
```

- [ ] **Step 3: deployment.yaml + helpers**

The deployment has two containers: `flow-server` (the Go binary) and `litestream` (sidecar reading the same volume). Both mount `/var/lib/flow` from one PVC. `litestream` runs `litestream replicate` against the S3 URL from values.

- [ ] **Step 4: Lint**

```bash
helm lint deploy/helm/flow-server
helm template deploy/helm/flow-server | kubectl apply --dry-run=client -f -
```

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/flow-server/
git commit -m "feat(deploy): Helm chart skeleton with flow-server + litestream sidecar"
```

### Task 4: Helm chart — service, ingress, configmap, secret, pvc

**Files:**
- Create: remaining `deploy/helm/flow-server/templates/*.yaml`

- [ ] **Step 1: service.yaml**

ClusterIP, exposes port 8080. ServiceMonitor (optional) scrapes `/metrics` on the same port.

- [ ] **Step 2: ingress.yaml**

Standard Ingress with `cert-manager.io/cluster-issuer` annotation. Routes everything to the service.

- [ ] **Step 3: configmap.yaml + secret.yaml**

ConfigMap holds non-secret env (`FLOW_SERVER_URL`, `OIDC_ISSUER`, `ALLOWED_SUBS`). Secret holds OIDC client secret, cookie signing keys, and Litestream S3 credentials. Mounted as env vars into both containers.

- [ ] **Step 4: pvc.yaml**

Standard PVC, RWO access mode (SQLite means one writer anyway, so RWO is fine; Litestream sidecar accesses the same volume since both containers share the Pod).

- [ ] **Step 5: Lint + smoke**

```bash
helm lint deploy/helm/flow-server
helm template deploy/helm/flow-server --debug
```

Optionally deploy to a kind cluster:

```bash
kind create cluster --name flow-test
helm install flow-server deploy/helm/flow-server \
  --set secret.oidcClientSecret=test \
  --set secret.cookieKeys=$(openssl rand -base64 32):$(openssl rand -base64 32) \
  --set ingress.enabled=false
kubectl wait --for=condition=ready pod -l app=flow-server --timeout=60s
kubectl port-forward svc/flow-server 8080:8080 &
curl -fsS http://localhost:8080/healthz
kind delete cluster --name flow-test
```

- [ ] **Step 6: Commit**

```bash
git add deploy/helm/flow-server/templates/
git commit -m "feat(deploy): Helm chart — service, ingress, configmap, secret, pvc"
```

### Task 5: GitHub Actions — image build + helm lint

**Files:**
- Create: `.github/workflows/build-server-image.yml`
- Create: `.github/workflows/helm-lint.yml`

- [ ] **Step 1: build-server-image.yml**

```yaml
name: Build flow-server image
on:
  push:
    branches: [main]
    paths:
      - 'cmd/flow-server/**'
      - 'internal/**'
      - 'deploy/podman/Dockerfile.server'
      - 'go.mod'
      - 'go.sum'
permissions:
  contents: read
  packages: write
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: deploy/podman/Dockerfile.server
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/flow-server:${{ github.sha }}
            ghcr.io/${{ github.repository_owner }}/flow-server:latest
```

- [ ] **Step 2: helm-lint.yml**

```yaml
name: Helm lint
on:
  pull_request:
    paths: ['deploy/helm/**']
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - run: helm lint deploy/helm/flow-server
      - run: helm template deploy/helm/flow-server > /dev/null
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/
git commit -m "ci: build flow-server image + helm lint"
```

---

## Phase B: M9 Hardening

### Task 6: Prometheus metrics

**Files:**
- Create: `internal/adapter/httpserver/metrics.go`
- Modify: `internal/adapter/httpserver/server.go` (register `/metrics` handler)
- Modify: `cmd/flow-server/main.go` (collector registration)

- [ ] **Step 1: Collectors**

```go
package httpserver

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "flow_http_requests_total",
        Help: "HTTP requests by method, route, status.",
    }, []string{"method", "route", "status"})

    HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "flow_http_request_duration_seconds",
        Help:    "HTTP request duration.",
        Buckets: prometheus.DefBuckets,
    }, []string{"method", "route"})

    SyncConflicts = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "flow_sync_conflicts_total",
        Help: "OCC conflicts surfaced to clients, by resource.",
    }, []string{"resource"})

    DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "flow_db_query_duration_seconds",
        Help:    "SQL query duration by adapter+method.",
        Buckets: prometheus.DefBuckets,
    }, []string{"adapter", "method"})
)

func NewMetricsHandler() http.Handler { return promhttp.Handler() }

func NewMetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rec := &statusRecorder{ResponseWriter: w, status: 200}
        next.ServeHTTP(rec, r)
        route := chi.RouteContext(r.Context()).RoutePattern()
        HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
        HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
    })
}
```

- [ ] **Step 2: Register**

In `NewWithAuth`:

```go
r.Handle("/metrics", NewMetricsHandler())
r.Use(NewMetricsMiddleware) // outside auth groups so /healthz/readyz also count
```

- [ ] **Step 3: Sync-conflict counter hookup**

In `httpserver/sessions_handlers.go` etc., the 409 paths `httpserver.SyncConflicts.WithLabelValues("sessions").Inc()` etc.

- [ ] **Step 4: Test + commit**

```bash
go test ./internal/adapter/httpserver/... -run Metrics -v
git add internal/adapter/httpserver/metrics.go internal/adapter/httpserver/server.go
git commit -m "feat(metrics): Prometheus collectors + /metrics endpoint"
```

### Task 7: Structured JSON logging

**Files:**
- Create: `internal/adapter/httpserver/logging.go`
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: Request log middleware**

```go
func NewLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rec := &statusRecorder{ResponseWriter: w, status: 200}
            next.ServeHTTP(rec, r)
            user, _ := UserFromContext(r.Context())
            logger.LogAttrs(r.Context(), slog.LevelInfo, "http",
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.Int("status", rec.status),
                slog.Duration("dur", time.Since(start)),
                slog.String("user_sub", user.OIDCSub),
                slog.String("remote", r.RemoteAddr),
            )
        })
    }
}
```

- [ ] **Step 2: Wire JSON logger in main**

```go
// cmd/flow-server/main.go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
slog.SetDefault(logger)
```

- [ ] **Step 3: Replace ad-hoc `log` calls**

Walk the new server code paths from M1-M7, replace any `log.Printf` / `fmt.Fprintf(os.Stderr, ...)` with `slog` calls.

- [ ] **Step 4: Commit**

```bash
go build ./...
git add internal/adapter/httpserver/logging.go cmd/flow-server/main.go
git commit -m "feat(logging): structured JSON request log + slog adoption"
```

### Task 8: Retry policy for httpsync worker

**Files:**
- Create: `internal/adapter/httpsync/retry.go`
- Modify: `internal/adapter/httpsync/worker.go`

- [ ] **Step 1: Backoff helper**

```go
package httpsync

import (
    "math/rand"
    "time"
)

type Backoff struct {
    Base    time.Duration // 500ms
    Max     time.Duration // 60s
    Factor  float64       // 2.0
    Jitter  float64       // 0.2
}

func (b Backoff) For(attempt int) time.Duration {
    d := float64(b.Base) * math.Pow(b.Factor, float64(attempt))
    d *= 1 + (rand.Float64()*2-1)*b.Jitter
    if d > float64(b.Max) { return b.Max }
    return time.Duration(d)
}
```

- [ ] **Step 2: Worker uses it**

The worker already retries pulls on the 30s tick. The new policy applies to **drain entries** — when an entry fails with a transient error (5xx, network) the worker records the attempt count via `queue.SetError` (which already exists) and on subsequent drain ticks honors a per-entry deadline computed via `Backoff`.

Add a `last_error_at` and `attempt` column to `write_queue` (sqliteclient migration `0004_write_queue_retry.sql`):

```sql
-- +goose Up
ALTER TABLE write_queue ADD COLUMN attempt      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE write_queue ADD COLUMN next_retry_at TEXT;

-- +goose Down (rebuild)
-- ... same pattern as 0002_active_tag_note.sql Down
```

The worker filters Peek to entries where `next_retry_at <= now OR next_retry_at IS NULL`, increments `attempt`, sets `next_retry_at = now + Backoff.For(attempt)` on failure.

- [ ] **Step 3: Tests**

`worker_test.go` adds a flaky transport (`httptest` server that returns 500 N times then 200) and asserts the worker eventually drains it with progressively longer intervals.

- [ ] **Step 4: Commit**

```bash
go test ./internal/adapter/httpsync/... -run "TestWorker_RetryBackoff" -v
git add internal/adapter/httpsync/retry.go internal/adapter/httpsync/worker.go \
        internal/adapter/sqliteclient/migrations/0004_write_queue_retry.sql \
        internal/adapter/sqliteclient/write_queue.go internal/adapter/sqliteclient/write_queue_test.go
git commit -m "feat(httpsync): exponential-backoff retry for transient push failures"
```

### Task 9: Graceful shutdown

**Files:**
- Modify: `cmd/flow-server/main.go`

- [ ] **Step 1: 30s drain**

```go
ctx, cancel := signal.NotifyContext(context.Background(),
    os.Interrupt, syscall.SIGTERM)
defer cancel()

srv := &http.Server{Addr: ":8080", Handler: handler}
go func() {
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        logger.Error("listen", slog.Any("err", err))
    }
}()

<-ctx.Done()
logger.Info("shutdown initiated, draining 30s")
shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutCancel()
if err := srv.Shutdown(shutCtx); err != nil {
    logger.Error("shutdown", slog.Any("err", err))
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/flow-server/main.go
git commit -m "feat(flow-server): graceful 30s drain on SIGTERM"
```

### Task 10: Litestream restore drill

**Files:**
- Create: `scripts/litestream-restore-drill.sh`
- Create: `docs/runbook/litestream-restore.md`

- [ ] **Step 1: Drill script**

```bash
#!/usr/bin/env bash
# scripts/litestream-restore-drill.sh — proves the backup is actually usable.
set -euo pipefail

PROD_DB="/var/lib/flow/server.db"
RESTORE_DIR="$(mktemp -d)"
RESTORE_DB="$RESTORE_DIR/server.db"

trap "rm -rf $RESTORE_DIR" EXIT

echo "== Snapshotting prod row counts =="
PROD_SESSIONS=$(sqlite3 "$PROD_DB" "SELECT COUNT(*) FROM sessions;")
PROD_REPOS=$(sqlite3 "$PROD_DB" "SELECT COUNT(*) FROM repos;")

echo "== Restoring from Litestream replica =="
litestream restore -if-replica-exists -config /etc/litestream.yml "$RESTORE_DB"

REST_SESSIONS=$(sqlite3 "$RESTORE_DB" "SELECT COUNT(*) FROM sessions;")
REST_REPOS=$(sqlite3 "$RESTORE_DB" "SELECT COUNT(*) FROM repos;")

echo "Prod  : sessions=$PROD_SESSIONS repos=$PROD_REPOS"
echo "Restore: sessions=$REST_SESSIONS repos=$REST_REPOS"

if [[ "$REST_SESSIONS" -lt "$((PROD_SESSIONS - 5))" ]]; then
  echo "FAIL: restored sessions diverge too far (>5 row loss)"
  exit 1
fi

echo "PASS"
```

- [ ] **Step 2: Runbook**

Walk through:
1. Identify the latest replica (`litestream snapshots`).
2. Stop the flow-server pod (`kubectl scale ... --replicas=0`).
3. `kubectl exec` into a fresh debug pod with the same PVC.
4. Run `litestream restore` to repopulate `/var/lib/flow/server.db`.
5. Scale back up.
6. Verify with `curl /readyz` and a TUI smoke.

- [ ] **Step 3: Commit**

```bash
chmod +x scripts/litestream-restore-drill.sh
git add scripts/litestream-restore-drill.sh docs/runbook/litestream-restore.md
git commit -m "docs(deploy): Litestream restore drill + runbook"
```

### Task 11: CHANGELOG + release tag

**Files:**
- Create: `CHANGELOG.md`
- Modify: `deploy/helm/flow-server/Chart.yaml` (`version` + `appVersion` bump)

- [ ] **Step 1: CHANGELOG**

```markdown
# Changelog

## flow-server v0.1.0 — 2026-MM-DD

First production-ready release of the flow client/server stack.

### Server
- HTTP API + WebUI on the same binary
- Authentik OIDC auth (browser + device flow)
- Multi-device sync for sessions, projects, repo notes
- Prometheus metrics, JSON logging, graceful shutdown
- Litestream sidecar for S3 backup

### Client
- Existing TUI/CLI gains background sync via httpsync
- New `flow repo note` commands for CLAUDE-style per-repo notes
- New `cmd/flow-mcp` stdio MCP server for Claude/Cursor/Codex

### Deploy
- Distroless container image at `ghcr.io/<owner>/flow-server:0.1.0`
- Helm chart at `deploy/helm/flow-server/`
- Local dev via `podman compose` in `deploy/podman/`

### Known limitations
- Single-user via OIDC `sub` allowlist (Phase 2 lifts this)
- LWW conflict resolution (CRDT is Phase 3 if needed)
- Kompendium notes still file-backed; sync deferred to Plan G
```

- [ ] **Step 2: Tag**

```bash
git tag -a flow-server/v0.1.0 -m "flow-server v0.1.0 — first production release"
git push origin flow-server/v0.1.0
```

- [ ] **Step 3: Commit + push**

```bash
git add CHANGELOG.md deploy/helm/flow-server/Chart.yaml
git commit -m "release: flow-server v0.1.0"
```

### Task 12: Merge `next` → `main`

This is the moment `feedback_long_lived_integration_branch.md` describes: the integration branch finally lands.

- [ ] **Step 1: Squash-merge**

```bash
git checkout main
git merge --squash next
git commit -m "$(cat <<'EOF'
feat: flow Phase 1 — multi-device sync, WebUI, MCP server, K8s deploy

Squash-merge of the long-lived `next` integration branch covering
M1-M9 of the Phase-1 spec at docs/superpowers/specs/2026-06-02-flow
-client-server-phase1-design.md.

Highlights:
* Authentik-OIDC auth for browser + CLI/TUI/MCP
* sqliteclient cache + httpsync background worker (sessions, projects,
  repo notes)
* flow-server HTTP API + WebUI (Templ/HTMX/Tailwind v4/CodeMirror 6)
* flow-mcp stdio MCP server for Claude/Cursor/Codex
* Distroless container + Helm chart + Litestream S3 backup
* Prometheus metrics, JSON logging, graceful shutdown, retry policy
EOF
)"
git push origin main
```

- [ ] **Step 2: Delete next**

```bash
git push origin --delete next
git branch -D next
```

- [ ] **Step 3: Memory update**

Promote the Plan-B + follow-ups + Plans C-F memory entries to "Phase 1 DONE 2026-MM-DD, flow-server v0.1.0 on ghcr.io". Add a fresh `project_phase2_kickoff` memory pointing at the Phase-2 spec when Soenne writes it.

- [ ] **Step 4: Production deploy**

```bash
helm upgrade --install flow-server deploy/helm/flow-server \
  --values deploy/helm/flow-server/values-soenne.yaml \
  --namespace flow --create-namespace \
  --wait
```

Then a manual smoke from Soenne's daily machines confirms multi-device sync works end-to-end.

---

## Risiken & Notes

1. **CGo-free build matters.** `modernc.org/sqlite` keeps the Dockerfile a one-stage scratch+distroless setup. If a future task switches to `mattn/go-sqlite3`, the deploy story restarts.
2. **Litestream + SQLite WAL mode** — make sure `flow-server`'s sqlite connection uses `_pragma=journal_mode(WAL)` (already true per sqliteserver/store.go). Litestream needs WAL.
3. **Single-pod constraint.** `replicaCount: 1` is load-bearing. Document it loudly in `values.yaml`; a future plan can introduce a leader-election + read-replicas variant if Soenne hits the single-writer ceiling.
4. **Backup verification cadence.** The drill script should run weekly via a CronJob. M9 lands the script; the CronJob is a small follow-up.
5. **Secret rotation.** Cookie keys + OIDC client secret should be re-rotatable without rebuilding the image. ConfigMap + Secret are mounted as env vars, so changing them and restarting the pod is the rotation procedure. Documented in the runbook.
