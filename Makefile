# xflow - IoT Flow Engine
# Usage:
#   make          Build everything (frontend + backend)
#   make web      Build frontend only
#   make server   Build backend only
#   make run      Build and run xflowd
#   make dev      Run frontend dev server + backend concurrently
#   make test     Run all tests
#   make clean    Remove build artifacts

APP        := xflowd
MODULE     := github.com/xtra/xflow
BUILD_DIR  := bin
WEB_DIR    := web
WEB_DIST   := $(WEB_DIR)/dist
CONFIG     := examples/config/xflow.yaml

# Go build flags
GOFLAGS    ?=
LDFLAGS    ?=

.PHONY: all web server run dev test test-go test-web lint clean help

## all: Build frontend and backend
all: web server

## web: Build frontend (web/dist/)
web:
	cd $(WEB_DIR) && npm install --silent && npm run build

## server: Build Go backend binary
server:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP) ./cmd/$(APP)

## run: Build all and run xflowd
run: all
	./$(BUILD_DIR)/$(APP) --config $(CONFIG)

## dev: Run frontend dev server and backend concurrently
dev:
	@echo "Starting backend..."
	@go run ./cmd/$(APP) --config $(CONFIG) &
	@echo "Starting frontend dev server..."
	@cd $(WEB_DIR) && npm run dev

## test: Run all tests (Go + Web)
test: test-go test-web

## test-go: Run Go tests with race detector
test-go:
	go test -race ./...

## test-web: Run frontend tests
test-web:
	cd $(WEB_DIR) && npm test

## lint: Run linters
lint:
	go vet ./...
	cd $(WEB_DIR) && npx tsc --noEmit

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR) $(WEB_DIST)

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
