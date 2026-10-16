# CodeDiver build tasks. `make help` lists targets.
#
# Builds go through scripts/build.sh (Linux/macOS) and scripts/build.ps1
# (Windows), the same scripts the release workflow runs.
#
# Linux desktop builds need: sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
WAILS_VERSION := v2.16.0
WAILS ?= $(shell go env GOPATH)/bin/wails
LOGO ?= cd_logo_1.png
ifeq ($(shell uname -s),Linux)
  WAILS_TAGS := -tags webkit2_41
endif

.PHONY: help tools deps frontend test test-go test-frontend vet lint build package headless headless-all serve dev icon release clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

tools: ## Install the Wails CLI
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

deps: ## Install frontend dependencies
	cd frontend && npm ci

frontend: ## Build the frontend bundle (frontend/dist)
	cd frontend && npm run build

test: test-go test-frontend ## Run all tests

test-go: ## Go tests with the race detector (headless build tag)
	go test -race -tags nogui ./...

test-frontend: ## Frontend type check and unit tests
	cd frontend && npm run typecheck && npm test

vet: ## go vet
	go vet -tags nogui ./...

lint: vet ## vet + gofmt check
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './frontend/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './frontend/*'); echo "run gofmt -w"; exit 1)

build: ## Desktop app for this machine → build/bin
	scripts/build.sh --version $(VERSION)

package: ## Desktop app + release artifacts for this machine → dist/
	scripts/build.sh --version $(VERSION) --package

headless: frontend ## Headless binary for this machine → build/bin/codediver-headless
	go build -tags nogui -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o build/bin/codediver-headless .

headless-all: ## Headless binaries for every OS/arch → dist/
	scripts/build.sh --headless --package --version $(VERSION)

serve: headless ## Run the browser UI against your config
	./build/bin/codediver-headless serve

dev: ## Wails dev mode with hot reload
	$(WAILS) dev $(WAILS_TAGS)

icon: ## Regenerate icons and logos from $(LOGO)
	go run ./scripts/brand -src $(LOGO)

release: ## Tag and push a release: make release VERSION=v1.2.3
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || (echo "VERSION must look like v1.2.3 (got $(VERSION))"; exit 1)
	@test -z "$$(git status --porcelain)" || (echo "working tree is dirty; commit first"; exit 1)
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || (echo "release from main"; exit 1)
	@! git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null || (echo "tag $(VERSION) already exists"; exit 1)
	$(MAKE) test
	git tag -a $(VERSION) -m "CodeDiver $(VERSION)"
	git push origin $(VERSION)
	@echo "Pushed $(VERSION); the Release workflow will publish it."

clean: ## Remove build outputs
	rm -rf build/bin dist frontend/dist/assets frontend/dist/index.html
