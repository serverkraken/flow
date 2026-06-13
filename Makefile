BIN             := flow-server
PKG             := ./cmd/flow-server
COVER_OUT       := coverage.out
COVER_THRESHOLD := 80
COVER_PKG       := ./internal/...

.PHONY: build test cover lint fmt ci db-up db-down smoke
build:
	@mkdir -p bin
	go build -o bin/flow-server ./cmd/flow-server
	go build -o bin/flow ./cmd/flow
test:
	go test -race ./...
cover:
	go test -covermode=atomic -coverprofile=$(COVER_OUT) -coverpkg=$(COVER_PKG) ./...
	@./scripts/coverage-gate.sh $(COVER_OUT) $(COVER_THRESHOLD)
lint:
	golangci-lint run
fmt:
	gofumpt -w . && goimports -w .
db-up:
	docker compose -f deploy/docker-compose.yml up -d
db-down:
	docker compose -f deploy/docker-compose.yml down
smoke:
	./scripts/smoke-m0.sh
ci: lint cover build
