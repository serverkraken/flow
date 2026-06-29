BIN             := flow-server
PKG             := ./cmd/flow-server
COVER_OUT       := coverage.out
COVER_THRESHOLD := 75
COVER_PKG       := ./internal/...
PREFIX          ?= $(HOME)/.local
BINDIR          ?= $(PREFIX)/bin

.PHONY: build install test cover lint fmt ci db-up db-down smoke smoke-m1b web generate verify-generate verify-css verify-no-popups dev-up dev-down dev-run dev-token dev-login
build:
	@mkdir -p bin
	go build -o bin/flow-server ./cmd/flow-server
	go build -o bin/flow ./cmd/flow
	go build -o bin/flow-mcp ./cmd/flow-mcp
# install copies the freshly-built binaries to $(BINDIR) (default ~/.local/bin) via install(1),
# which writes a temp file then renames it into place — a FRESH inode. That avoids the macOS
# "Killed: 9" (SIGKILL) you get when `cp` overwrites a signed binary in place and the kernel's
# cached code-signature (cdhash) for that inode goes stale. Override dest: make install PREFIX=/usr/local
install: build
	@install -d "$(BINDIR)"
	install -m 0755 bin/flow-server "$(BINDIR)/flow-server"
	install -m 0755 bin/flow "$(BINDIR)/flow"
	install -m 0755 bin/flow-mcp "$(BINDIR)/flow-mcp"
	@echo "installed flow, flow-server, flow-mcp -> $(BINDIR)"
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
smoke-m1b:
	./scripts/smoke-m1b.sh
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
# verify-css checks the committed app.css matches a fresh tailwind build.
verify-css:
	@./scripts/verify-css.sh
# verify-no-popups bans native browser popups in the WebUI (use Dialog instead).
verify-no-popups:
	@./scripts/verify-no-popups.sh
ci: lint verify-generate verify-css verify-no-popups cover build
