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
#   make amd64       Cross-compile for Linux x86-64 (linux/amd64)
#   make amd64-pkg   Create installation package (tar.gz) for Linux x86-64
#   make release-all VERSION=v0.1.0  Build all 6 platform release packages locally
#   make release-images VERSION=v0.4.0 SIGN_KEY=xflow-release.key  Build + sign store images
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

# Linux x86-64 cross-compile
AMD64_GOARCH := amd64

# Release packaging (SPEC-CICD-001)
# CI 워크플로우와 동일한 산출물을 로컬에서 미러링한다.
VERSION    ?= dev
DIST_DIR   := dist
RELEASE_LDFLAGS := -s -w -X main.Version=$(VERSION)

# Docker
DOCKER_IMAGE ?= xflow
DOCKER_TAG   ?= $(VERSION)
DOCKER_PORT  ?= 8081

.PHONY: all web server rpi rpi-deploy rpi-pkg amd64 amd64-pkg run dev test test-go test-web lint clean help \
        release-all release-platform checksums docker-build docker-run keygen release-images release-image-one release-images-guard

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
# 버전을 main.Version 으로 주입한다(VERSION 미지정 시 기본 "dev"). 원격 자가 업데이트
# 의 버전 표시/이력은 노드가 보고하는 이 값이 바뀔 때만 갱신되므로, 패키지에는 반드시
# 실제 버전을 박아야 한다(예: make rpi-pkg VERSION=v1.3.0).
RPI_LDFLAGS := $(LDFLAGS) -X main.Version=$(VERSION)
rpi: web
	GOOS=linux GOARCH=$(RPI_GOARCH) GOARM=$(RPI_GOARM) \
		go build $(GOFLAGS) -ldflags "$(RPI_LDFLAGS)" -o $(BUILD_DIR)/$(APP)-linux-$(RPI_GOARCH) ./cmd/$(APP)
	@for cli in $(CLI_APPS); do \
		GOOS=linux GOARCH=$(RPI_GOARCH) GOARM=$(RPI_GOARM) \
			go build $(GOFLAGS) -ldflags "$(RPI_LDFLAGS)" -o $(BUILD_DIR)/$$cli-linux-$(RPI_GOARCH) ./cmd/$$cli; \
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

## amd64: Cross-compile for Linux x86-64 (linux/amd64)
# rpi 타깃과 동일하게 실제 버전을 main.Version 으로 주입한다(예: make amd64-pkg VERSION=v1.3.0).
AMD64_LDFLAGS := $(LDFLAGS) -X main.Version=$(VERSION)
amd64: web
	GOOS=linux GOARCH=$(AMD64_GOARCH) \
		go build $(GOFLAGS) -ldflags "$(AMD64_LDFLAGS)" -o $(BUILD_DIR)/$(APP)-linux-$(AMD64_GOARCH) ./cmd/$(APP)
	@for cli in $(CLI_APPS); do \
		GOOS=linux GOARCH=$(AMD64_GOARCH) \
			go build $(GOFLAGS) -ldflags "$(AMD64_LDFLAGS)" -o $(BUILD_DIR)/$$cli-linux-$(AMD64_GOARCH) ./cmd/$$cli; \
	done

## amd64-pkg: Create installation package (tar.gz) for Linux x86-64
amd64-pkg: amd64
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)/web
	cp $(BUILD_DIR)/$(APP)-linux-$(AMD64_GOARCH) $(PKG_DIR)/xflowd
	@for cli in $(CLI_APPS); do \
		cp $(BUILD_DIR)/$$cli-linux-$(AMD64_GOARCH) $(PKG_DIR)/$$cli; \
	done
	cp -r $(WEB_DIST) $(PKG_DIR)/web/dist
	cp deploy/xflow.yaml $(PKG_DIR)/
	cp deploy/xflowd.service $(PKG_DIR)/
	cp deploy/install.sh $(PKG_DIR)/
	cp deploy/uninstall.sh $(PKG_DIR)/
	cd $(BUILD_DIR) && tar czf xflowd-linux-$(AMD64_GOARCH).tar.gz -C pkg .
	rm -rf $(PKG_DIR)
	@echo "Package created: $(BUILD_DIR)/xflowd-linux-$(AMD64_GOARCH).tar.gz"

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
# Release images (SPEC-UPDATE-001) - 스토어 업로드용 서명된 단일 바이너리 이미지
# ----------------------------------------------------------------------------

# IMAGES_DIR: 스토어용 xflowd-{os}-{arch} + .sig 산출물 위치.
IMAGES_DIR := $(BUILD_DIR)/images

# KEY_OUT_DIR / KEY_NAME: keygen 산출 위치/이름(개인키 {NAME}.key, 공개키 {NAME}.pub).
KEY_OUT_DIR ?= .
KEY_NAME    ?= xflow-release

## keygen: Generate an Ed25519 release signing keypair (private .key 0600 + public .pub)
# 사용 예:
#   make keygen                                  # ./xflow-release.key + ./xflow-release.pub
#   make keygen KEY_OUT_DIR=secrets KEY_NAME=prod
#   make keygen FORCE=1                          # 기존 키 덮어쓰기
#
# 개인키(.key)는 서명 전용이며 절대 커밋 금지(.gitignore 의 *.key). 공개키(.pub)만
# 각 노드의 update.public_key_path 에 배포한다. 자세한 내용은 deploy/RELEASE_IMAGES.md.
keygen:
	@go run ./cmd/xflowd update keygen --out-dir $(KEY_OUT_DIR) --name $(KEY_NAME) $(if $(FORCE),--force,)

## release-images: Build + sign store-ready xflowd images for all 6 targets
# 사용 예:
#   make release-images VERSION=v0.4.0 SIGN_KEY=path/to/xflow-release.key
#
# 각 타깃 {os}/{arch} 에 대해:
#   1) CGO_ENABLED=0 으로 cross-compile → $(IMAGES_DIR)/xflowd-{os}-{arch} (확장자 없음)
#   2) `go run ./cmd/xflowd update sign --key $(SIGN_KEY)` 로 raw 64-byte .sig 생성
# 마지막에 checksum.txt (sha256) 를 생성하고 Web UI 업로드 안내를 출력한다.
release-images: release-images-guard web
	@rm -rf $(IMAGES_DIR)
	@mkdir -p $(IMAGES_DIR)
	@echo "==> Building + signing release images (VERSION=$(VERSION))"
	@$(MAKE) --no-print-directory release-image-one GOOS=linux  GOARCH=amd64       SUFFIX=linux-amd64
	@$(MAKE) --no-print-directory release-image-one GOOS=linux  GOARCH=arm64       SUFFIX=linux-arm64
	@$(MAKE) --no-print-directory release-image-one GOOS=linux  GOARCH=arm GOARM=6 SUFFIX=linux-arm
	@$(MAKE) --no-print-directory release-image-one GOOS=darwin GOARCH=amd64       SUFFIX=darwin-amd64
	@$(MAKE) --no-print-directory release-image-one GOOS=darwin GOARCH=arm64       SUFFIX=darwin-arm64
	@$(MAKE) --no-print-directory release-image-one GOOS=windows GOARCH=amd64      SUFFIX=windows-amd64
	@echo "==> Generating $(IMAGES_DIR)/checksum.txt"
	@cd $(IMAGES_DIR) && \
		(command -v sha256sum >/dev/null && sha256sum xflowd-* > checksum.txt) || \
		shasum -a 256 xflowd-* | sed 's/ \*/  /' > checksum.txt
	@echo "==> Release images ready in $(IMAGES_DIR)/ (upload to release store via Web UI):"
	@ls -la $(IMAGES_DIR)/

# release-images-guard: 필수 인자 검증 (web 빌드 전에 먼저 실패하도록 선행 의존성으로 배치).
release-images-guard:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then \
		echo "ERROR: VERSION is required. Usage: make release-images VERSION=v0.4.0 SIGN_KEY=path/to/xflow-release.key"; exit 1; fi
	@if [ -z "$(SIGN_KEY)" ]; then \
		echo "ERROR: SIGN_KEY is required. Usage: make release-images VERSION=v0.4.0 SIGN_KEY=path/to/xflow-release.key"; exit 1; fi
	@if [ ! -f "$(SIGN_KEY)" ]; then \
		echo "ERROR: SIGN_KEY file not found: $(SIGN_KEY)"; exit 1; fi

# release-image-one: 단일 타깃 build + sign (release-images 내부 호출용).
release-image-one:
	@if [ -z "$(GOOS)" ] || [ -z "$(GOARCH)" ] || [ -z "$(SUFFIX)" ]; then \
		echo "ERROR: GOOS, GOARCH, SUFFIX are required"; exit 1; fi
	@echo "  -> xflowd-$(SUFFIX)"
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GOARM=$(GOARM) \
		go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" \
		-o $(IMAGES_DIR)/xflowd-$(SUFFIX) ./cmd/xflowd
	@# 서명 도구(go run)는 HOST 에서 실행되어야 하므로 GOOS/GOARCH/GOARM 을 비운다.
	@# (sign 은 바이트만 읽으므로 임의 arch 바이너리를 호스트에서 서명 가능)
	@GOOS= GOARCH= GOARM= CGO_ENABLED= \
		go run ./cmd/xflowd update sign --key $(SIGN_KEY) $(IMAGES_DIR)/xflowd-$(SUFFIX)

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
