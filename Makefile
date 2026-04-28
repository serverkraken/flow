BIN             := flow
PKG             := ./cmd/flow
COVER_OUT       := coverage.out
COVER_THRESHOLD := 20

.PHONY: build install test cover lint fmt clean ci

build:
	@mkdir -p bin
	go build -o bin/$(BIN) $(PKG)

install:
	GOBIN="$(HOME)/.local/bin" go install $(PKG)

test:
	go test -race ./...

cover:
	go test -race -covermode=atomic -coverprofile=$(COVER_OUT) ./...
	@./scripts/coverage-gate.sh $(COVER_OUT) $(COVER_THRESHOLD)

lint:
	golangci-lint run

fmt:
	gofumpt -w .
	goimports -w .

clean:
	rm -rf bin/ dist/ $(COVER_OUT) coverage.html

ci: lint cover build
