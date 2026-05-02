BIN             := flow
PKG             := ./cmd/flow
COVER_OUT       := coverage.out
COVER_THRESHOLD := 90

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
	go test -race -covermode=atomic -coverprofile=$(COVER_OUT) -coverpkg=$(COVER_PKG) ./...
	@./scripts/coverage-gate.sh $(COVER_OUT) $(COVER_THRESHOLD)

lint:
	golangci-lint run

fmt:
	gofumpt -w .
	goimports -w .

clean:
	rm -rf bin/ dist/ $(COVER_OUT) coverage.html

ci: lint cover build
