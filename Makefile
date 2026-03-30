# xflow - IoT Flow Engine
# Usage:
#   make          Build everything (frontend + backend)
#   make web      Build frontend only
#   make server   Build backend only
#   make run      Build and run xflowd
#   make dev      Run frontend dev server + backend concurrently
#   make test     Run all tests
#   make rpi      Cross-compile for Raspberry Pi (linux/arm64)
#   make rpi-deploy RPI_HOST=pi@192.168.1.100  Build and deploy to RPi
#   make rpi-pkg   Create installation package (tar.gz)
#   make clean    Remove build artifacts

APP        := xflowd
CLI_APPS   := xflow xflow-agent
MODULE     := github.com/xtra/xflow
BUILD_DIR  := bin
WEB_DIR    := web
WEB_DIST   := $(WEB_DIR)/dist
CONFIG     := examples/config/xflow.yaml

# Go build flags
GOFLAGS    ?=
LDFLAGS    ?=

# Raspberry Pi cross-compile
RPI_GOARCH ?= arm64
RPI_GOARM  ?=
RPI_HOST   ?=
RPI_DIR    ?= /opt/xflow
PKG_DIR    := $(BUILD_DIR)/pkg

.PHONY: all web server rpi rpi-deploy rpi-pkg run dev test test-go test-web lint clean help

## all: Build frontend and backend
all: web server

## web: Build frontend (web/dist/)
web:
	cd $(WEB_DIR) && npm install --silent && npm run build

## server: Build Go backend binary and CLI tools
server:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP) ./cmd/$(APP)
	@for cli in $(CLI_APPS); do \
		go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$$cli ./cmd/$$cli; \
	done

## rpi: Cross-compile for Raspberry Pi (linux/arm64)
rpi: web
	GOOS=linux GOARCH=$(RPI_GOARCH) GOARM=$(RPI_GOARM) \
		go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP)-linux-$(RPI_GOARCH) ./cmd/$(APP)
	@for cli in $(CLI_APPS); do \
		GOOS=linux GOARCH=$(RPI_GOARCH) GOARM=$(RPI_GOARM) \
			go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$$cli-linux-$(RPI_GOARCH) ./cmd/$$cli; \
	done

## rpi-deploy: Build and deploy to Raspberry Pi via scp (RPI_HOST required)
rpi-deploy: rpi
	@if [ -z "$(RPI_HOST)" ]; then echo "ERROR: RPI_HOST not set. Usage: make rpi-deploy RPI_HOST=pi@192.168.1.100"; exit 1; fi
	scp $(BUILD_DIR)/$(APP)-linux-$(RPI_GOARCH) $(RPI_HOST):$(RPI_DIR)/$(APP)
	scp -r $(WEB_DIST) $(RPI_HOST):$(RPI_DIR)/web/dist
	scp $(CONFIG) $(RPI_HOST):$(RPI_DIR)/xflow.yaml

## rpi-pkg: Create installation package (tar.gz) for Raspberry Pi
rpi-pkg: rpi
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)/web
	cp $(BUILD_DIR)/$(APP)-linux-$(RPI_GOARCH) $(PKG_DIR)/xflowd
	@for cli in $(CLI_APPS); do \
		cp $(BUILD_DIR)/$$cli-linux-$(RPI_GOARCH) $(PKG_DIR)/$$cli; \
	done
	cp -r $(WEB_DIST) $(PKG_DIR)/web/dist
	cp deploy/xflow.yaml $(PKG_DIR)/
	cp deploy/xflowd.service $(PKG_DIR)/
	cp deploy/install.sh $(PKG_DIR)/
	cp deploy/uninstall.sh $(PKG_DIR)/
	cd $(BUILD_DIR) && tar czf xflowd-linux-$(RPI_GOARCH).tar.gz -C pkg .
	rm -rf $(PKG_DIR)
	@echo "Package created: $(BUILD_DIR)/xflowd-linux-$(RPI_GOARCH).tar.gz"

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
