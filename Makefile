BIN             := flow-server
PKG             := ./cmd/flow-server
COVER_OUT       := coverage.out
COVER_THRESHOLD := 80
COVER_PKG       := ./internal/...

.PHONY: build test cover lint fmt ci db-up db-down smoke web generate verify-generate dev-up dev-down dev-run dev-token dev-login
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
	./scripts/smoke-m1a.sh
# --- self-contained dev env (Postgres + Dex OIDC); see deploy/dev/README.md ---
dev-up:
	./scripts/dev-up.sh
dev-down:
	./scripts/dev-down.sh $(ARGS)
dev-run:
	./scripts/dev-run.sh
dev-token:
	@./scripts/dev-token.sh
dev-login:
	set -a; . deploy/dev/flow-cli.env; set +a; go run ./cmd/flow login
# web builds the Tailwind v4 stylesheet. Requires the tailwindcss CLI (NOT part of make ci).
web:
	tailwindcss --input web/tailwind.css --output internal/adapter/webui/static/app.css --minify
# generate runs all code generators (templ, etc.).
generate:
	go tool templ generate
# verify-generate checks that generated files are up to date.
verify-generate:
	go tool templ generate
	@if ! git diff --quiet -- ':*_templ.go'; then \
		echo "ERROR: generated *_templ.go is out of date — run make generate"; \
		git diff -- ':*_templ.go'; \
		exit 1; \
	fi
	@echo "verify-generate: OK"
ci: lint verify-generate cover build
