BIN             := flow
PKG             := ./cmd/flow
COVER_OUT       := coverage.out
# 90% target — the project's schema goal. The previous 85% slip was a
# pragmatic floor before the testutil package and the cancelled-context
# cobra-RunE pattern brought every CLI verb in reach. Specifically:
#  1. testutil/* now self-tests its fakes (was 0%, now 99%+).
#  2. Standalone cobra commands (sidekick / palette / projects /
#     cheatsheet / markdown / worktime today) run their RunE under an
#     already-cancelled tea.WithContext, so the full constructor +
#     theme + factory chain executes without needing a real TTY.
#  3. cmd/flow's composition root has direct tests for buildDeps /
#     buildKompendiumDeps / buildNotesScreen / parseEnvHoursDuration —
#     only main() itself stays uncovered (os.Exit makes it untestable).
# Aggregate sits around 90% with the `-coverpkg=./internal/...` measure
# this target uses. Drop the threshold only with a justification in the
# corresponding plan file.
#
# Phase-1 M1 (2026-06-02) drops the gate to 89% temporarily: the new auth
# code (oidcserver.Provider, real keyringadapter.Keyring, testutil/oidctest)
# is integration-tested only (//go:build integration against dex
# testcontainers + real Authentik per docs/runbook/m1-smoke-test.md) — Unit
# coverage on these surfaces would require either OS-keychain access in CI
# or a Docker daemon, neither of which `make ci` provides. M2/M3
# (sqliteclient + sync use cases) is expected to bring the aggregate back
# above 90%; raise this threshold then. Plan reference:
# docs/superpowers/plans/2026-06-02-flow-phase1-m1-server-skeleton-oidc.md
COVER_THRESHOLD := 89

# Coverage measurement targets the hexagonal layers under internal/.
# cmd/flow is the composition root (wiring only, no business logic) and
# is intentionally excluded — see CLAUDE.md "Architecture — hexagonal".
# -coverpkg=./internal/... attributes coverage from any test (including
# adapter/usecase tests that exercise testutil fakes) to all internal
# packages, so testutil's fakes register as covered when their callers
# are.
COVER_PKG       := ./internal/...

.PHONY: build install test cover lint fmt clean ci \
        build-server test-server test-integration \
        dex-up dex-down dex-logs run-server

build:
	@mkdir -p bin
	go build -o bin/$(BIN) $(PKG)

install:
	GOBIN="$(HOME)/.local/bin" go install $(PKG)

test:
	go test -race ./...

cover:
	# -race is intentionally NOT passed here. With -race, Go's coverage
	# attribution under -coverpkg=./internal/... drops random hits in
	# parallel-test packages (sidekick + cli/sidekick lose ~1.5% under
	# -race vs the same suite without). Race correctness is enforced by
	# the separate `make test` target; coverage is a measurement, not a
	# correctness check.
	go test -covermode=atomic -coverprofile=$(COVER_OUT) -coverpkg=$(COVER_PKG) ./...
	@./scripts/coverage-gate.sh $(COVER_OUT) $(COVER_THRESHOLD)

lint:
	golangci-lint run

fmt:
	gofumpt -w .
	goimports -w .

clean:
	rm -rf bin/ dist/ $(COVER_OUT) coverage.html

ci: lint cover build

build-server:
	@mkdir -p bin
	go build -o bin/flow-server ./cmd/flow-server

test-server:
	go test ./internal/adapter/httpserver/... ./internal/adapter/oidcserver/... \
	        ./internal/adapter/oidcclient/... ./internal/adapter/keyringadapter/...

test-integration:
	go test -tags integration -count=1 -timeout 120s \
	        ./internal/adapter/httpserver/... \
	        ./internal/adapter/oidcserver/... \
	        ./internal/testutil/oidctest/...

# Local dev convenience: dex stack via podman-compose. dex serves on
# localhost:5556; static credentials are alice@example.com / password (see
# deploy/podman/dex-config.yaml).
dex-up:
	cd deploy/podman && podman-compose up -d dex

dex-down:
	cd deploy/podman && podman-compose down dex

dex-logs:
	cd deploy/podman && podman-compose logs -f dex

# Local-dev launcher that loads sensible FLOW_* defaults matching the dex
# stack from `make dex-up`. Cookie keys persist in .flow-cookie-keys
# (gitignored) so sessions stay valid across restarts. Override any var by
# exporting before invocation.
run-server: build-server
	./scripts/run-flow-server.sh
