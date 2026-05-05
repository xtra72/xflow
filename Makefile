# xflow - IoT Flow Engine
# Usage:
#   make             Build everything (frontend + backend)
#   make web         Build frontend only
#   make server      Build backend only
#   make run         Build and run xflowd
#   make dev         Run frontend dev server + backend concurrently
#   make test        Run all tests
#   make rpi         Cross-compile for Raspberry Pi (linux/arm64)
#   make rpi-deploy RPI_HOST=pi@192.168.1.100  Build and deploy to RPi
#   make rpi-pkg     Create installation package (tar.gz)
#   make release-all VERSION=v0.1.0  Build all 6 platform release packages locally
#   make checksums   Generate SHA256 checksums.txt for dist/
#   make docker-build  Build Docker image locally
#   make docker-run    Run Docker image locally
#   make clean       Remove build artifacts

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

# Release packaging (SPEC-CICD-001)
# CI 워크플로우와 동일한 산출물을 로컬에서 미러링한다.
VERSION    ?= dev
DIST_DIR   := dist
RELEASE_LDFLAGS := -s -w -X main.version=$(VERSION)

# Docker
DOCKER_IMAGE ?= xflow
DOCKER_TAG   ?= $(VERSION)
DOCKER_PORT  ?= 8081

.PHONY: all web server rpi rpi-deploy rpi-pkg run dev test test-go test-web lint clean help \
        release-all release-platform checksums docker-build docker-run

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
	rm -rf $(BUILD_DIR) $(DIST_DIR) $(WEB_DIST)

# ----------------------------------------------------------------------------
# Release packaging (SPEC-CICD-001) - CI 워크플로우 로컬 미러링
# ----------------------------------------------------------------------------

# release-platform: 단일 플랫폼 릴리스 패키지 생성
# 사용 예:
#   make release-platform VERSION=v0.1.0 GOOS=linux GOARCH=amd64 SUFFIX=linux-amd64
#   make release-platform VERSION=v0.1.0 GOOS=linux GOARCH=arm GOARM=7 SUFFIX=linux-armv7
release-platform: web
	@if [ -z "$(GOOS)" ] || [ -z "$(GOARCH)" ] || [ -z "$(SUFFIX)" ]; then \
		echo "ERROR: GOOS, GOARCH, SUFFIX are required"; exit 1; fi
	@mkdir -p $(DIST_DIR)/staging-$(SUFFIX)/web
	@echo "==> Building binaries for $(SUFFIX) (VERSION=$(VERSION))"
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) \
		go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" \
		-o $(DIST_DIR)/staging-$(SUFFIX)/xflowd ./cmd/xflowd
	@for cli in $(CLI_APPS); do \
		CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) \
			go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" \
			-o $(DIST_DIR)/staging-$(SUFFIX)/$$cli ./cmd/$$cli; \
	done
	@cp -r $(WEB_DIST) $(DIST_DIR)/staging-$(SUFFIX)/web/dist
	@cp deploy/xflow.yaml $(DIST_DIR)/staging-$(SUFFIX)/
	@[ -f README.md ] && cp README.md $(DIST_DIR)/staging-$(SUFFIX)/ || true
	@[ -f LICENSE ]   && cp LICENSE   $(DIST_DIR)/staging-$(SUFFIX)/ || true
	@if [ "$(GOOS)" = "linux" ]; then \
		cp deploy/xflowd.service $(DIST_DIR)/staging-$(SUFFIX)/; \
		cp deploy/install.sh $(DIST_DIR)/staging-$(SUFFIX)/; \
		cp deploy/uninstall.sh $(DIST_DIR)/staging-$(SUFFIX)/; \
		chmod 0755 $(DIST_DIR)/staging-$(SUFFIX)/install.sh $(DIST_DIR)/staging-$(SUFFIX)/uninstall.sh; \
	fi
	@cd $(DIST_DIR) && tar czf xflow-$(VERSION)-$(SUFFIX).tar.gz -C staging-$(SUFFIX) .
	@rm -rf $(DIST_DIR)/staging-$(SUFFIX)
	@echo "==> Package: $(DIST_DIR)/xflow-$(VERSION)-$(SUFFIX).tar.gz"

## release-all: Build all 6 platform release packages (mirrors CI matrix)
release-all:
	@echo "==> Building 6 platform release packages (VERSION=$(VERSION))"
	@$(MAKE) --no-print-directory release-platform GOOS=darwin GOARCH=amd64 SUFFIX=darwin-amd64
	@$(MAKE) --no-print-directory release-platform GOOS=darwin GOARCH=arm64 SUFFIX=darwin-arm64
	@$(MAKE) --no-print-directory release-platform GOOS=linux  GOARCH=amd64 SUFFIX=linux-amd64
	@$(MAKE) --no-print-directory release-platform GOOS=linux  GOARCH=arm64 SUFFIX=linux-arm64
	@$(MAKE) --no-print-directory release-platform GOOS=linux  GOARCH=arm   GOARM=7 SUFFIX=linux-armv7
	@$(MAKE) --no-print-directory release-platform GOOS=linux  GOARCH=arm   GOARM=6 SUFFIX=linux-armv6
	@echo "==> All 6 packages created in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/xflow-*.tar.gz

## checksums: Generate SHA256 checksums.txt for dist/ tarballs
checksums:
	@if [ ! -d $(DIST_DIR) ] || [ -z "$$(ls $(DIST_DIR)/xflow-*.tar.gz 2>/dev/null)" ]; then \
		echo "ERROR: No release tarballs in $(DIST_DIR)/. Run 'make release-all' first."; exit 1; fi
	@cd $(DIST_DIR) && \
		(command -v sha256sum >/dev/null && sha256sum xflow-*.tar.gz > checksums.txt) || \
		shasum -a 256 xflow-*.tar.gz > checksums.txt
	@echo "==> Checksums: $(DIST_DIR)/checksums.txt"
	@cat $(DIST_DIR)/checksums.txt

# ----------------------------------------------------------------------------
# Docker (로컬 빌드/실행)
# ----------------------------------------------------------------------------

## docker-build: Build Docker image locally (single-arch, host platform)
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "==> Image: $(DOCKER_IMAGE):$(DOCKER_TAG)"

## docker-run: Run xflowd container locally (port $(DOCKER_PORT))
docker-run:
	docker run --rm -p $(DOCKER_PORT):8081 \
		--name xflow-local \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
