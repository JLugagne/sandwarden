BINARY      := bin/sandwarden
PKG         := ./...
GO          ?= go
NPM         ?= npm
WAILS       ?= wails3
DOCKER      ?= docker
TAGS        := gtk3
VERSION     ?= dev

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: bindings
bindings: ## Regenerate the TypeScript bindings from the Go services
	$(WAILS) generate bindings -ts -i -clean=true -d web/src/bindings -f "-tags $(TAGS)" .

.PHONY: web-deps
web-deps: ## Install frontend dependencies, regenerating bindings when wails3 is installed
	@if command -v $(WAILS) >/dev/null 2>&1; then $(MAKE) bindings; else echo "$(WAILS) not installed: using the committed bindings"; fi
	cd web && $(NPM) install --no-audit --no-fund

.PHONY: web
web: web-deps ## Build the React frontend into web/dist
	cd web && $(NPM) run build

.PHONY: web-test
web-test: web-deps ## Run frontend unit tests
	cd web && $(NPM) test

.PHONY: build
build: web ## Compile the desktop binary into bin/sandwarden (frontend embedded)
	$(GO) build -tags "$(TAGS),production" -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) .

.PHONY: app-darwin
app-darwin: ## Build bin/sandwarden.app on macOS (native notifications need the bundle)
	$(MAKE) build TAGS=
	build/darwin/bundle.sh $(BINARY) $(VERSION) bin

.PHONY: image
image: ## Build the Docker image (runtime stage)
	$(DOCKER) build -t sandwarden:local .

.PHONY: image-binary
image-binary: ## Export the Linux binary from the Docker build into bin/
	$(DOCKER) buildx build --target binary --output type=local,dest=bin .

.PHONY: run
run: build ## Run the desktop app in the foreground
	./$(BINARY)

.PHONY: dev
dev: web-deps ## Run the desktop app with Vite HMR and Go hot-restart
	$(WAILS) dev -config build/config.yml

.PHONY: dev-web
dev-web: web-deps ## Run the Vite dev server only
	cd web && $(NPM) run dev

.PHONY: test
test: web ## Run Go and frontend tests
	$(GO) test -tags $(TAGS) $(PKG)
	cd web && $(NPM) test

.PHONY: test-go
test-go: ## Run Go tests only (no frontend build needed)
	$(GO) test -tags $(TAGS) $(PKG)

.PHONY: test-race
test-race: web ## Run Go tests with the race detector
	$(GO) test -tags $(TAGS) -race $(PKG)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet -tags $(TAGS) $(PKG)

.PHONY: typecheck
typecheck: web-deps ## Typecheck the frontend
	cd web && $(NPM) run typecheck

.PHONY: fmt
fmt: ## Format Go sources
	$(GO) fmt $(PKG)

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

.PHONY: check
check: fmt vet test ## Format, vet and test

.PHONY: install
install: ## Install the binary into GOBIN/GOPATH/bin
	$(GO) install -tags "$(TAGS),production" -ldflags "-X main.version=$(VERSION)" .

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin web/dist
