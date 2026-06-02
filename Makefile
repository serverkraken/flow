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
COVER_THRESHOLD := 90

# Coverage measurement targets the hexagonal layers under internal/.
# cmd/flow is the composition root (wiring only, no business logic) and
# is intentionally excluded — see CLAUDE.md "Architecture — hexagonal".
# -coverpkg=./internal/... attributes coverage from any test (including
# adapter/usecase tests that exercise testutil fakes) to all internal
# packages, so testutil's fakes register as covered when their callers
# are.
COVER_PKG       := ./internal/...

.PHONY: build install test cover lint fmt clean ci \
        build-server test-server test-integration

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
