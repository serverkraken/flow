BIN             := flow
PKG             := ./cmd/flow
COVER_OUT       := coverage.out
# 87% is a deliberate slip from the 90% schema target. Reason: every
# CLI verb that launches a tea.Program (`flow sidekick`,
# `flow worktime today`, `flow kompendium browse`) carries an
# untestable RunE body — tea.NewProgram with WithAltScreen requires a
# real /dev/tty, which Go test runners don't provide. Each new such
# verb costs ~2-3% on the cli package coverage; aggregate has settled
# at ~87%. Compensating with broad cobra Execute() smoke tests would
# only paper over the structural reality. Drop further only with a
# justification in the corresponding plan file.
COVER_THRESHOLD := 87

# Coverage measurement targets the hexagonal layers under internal/.
# cmd/flow is the composition root (wiring only, no business logic) and
# is intentionally excluded — see CLAUDE.md "Architecture — hexagonal".
# -coverpkg=./internal/... attributes coverage from any test (including
# adapter/usecase tests that exercise testutil fakes) to all internal
# packages, so testutil's fakes register as covered when their callers
# are.
COVER_PKG       := ./internal/...

.PHONY: build install test cover lint fmt clean ci

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
