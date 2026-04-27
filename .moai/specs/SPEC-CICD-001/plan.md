---
id: SPEC-CICD-001
type: plan
version: "1.0.0"
status: draft
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
---

# SPEC-CICD-001 구현 계획 (Implementation Plan)

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드(TDD for new + DDD for legacy):

- **CI/CD 워크플로우 파일**: 신규 생성이므로 RED-GREEN-REFACTOR 사이클을 적용한다. 워크플로우별로 최소 구성(GREEN) → 매트릭스 확장 → 캐시/최적화(REFACTOR)의 단계적 작성.
- **Makefile 확장**: 기존 `rpi`, `rpi-pkg` 타깃을 보존(PRESERVE)하면서 `release-all`, `checksums` 타깃을 추가(IMPROVE)한다.
- **deploy/ 자산**: 현행 스크립트 로직을 유지하며, 패키징 단계에서 그대로 복사만 수행한다.

### 1.2 설계 원칙

- **재사용 우선**: `Makefile`의 `rpi-pkg`, `deploy/install.sh` 등 기존 자산을 파일 복사 수준으로만 활용한다. 워크플로우에서 빌드 로직을 중복 작성하지 않는다.
- **순수 Go 크로스컴파일**: `CGO_ENABLED=0`으로 모든 6개 플랫폼을 단일 runner(ubuntu-latest)에서 빌드한다.
- **매트릭스 빌드**: GitHub Actions `strategy.matrix`를 사용하여 6개 플랫폼 바이너리 빌드를 병렬 실행한다.
- **Job 분리**: `build-frontend`, `build-binaries`, `package`, `checksums`, `release`, `docker-publish`로 책임을 분리하여 재시도 및 디버깅 용이성을 확보한다.
- **캐시 최적화**: `actions/setup-go`와 `actions/setup-node`의 내장 캐시 활용 + 명시적 `actions/cache`로 Go 모듈 및 npm 의존성 캐싱.
- **보안 원칙**: `permissions: read-all` 기본값 + Job 수준에서 최소 권한(`contents: write`, `packages: write`) 승격.
- **결정적 빌드**: `go.sum`, `web/package-lock.json`을 반드시 커밋하며 `npm ci` 사용(`npm install` 금지).
- **시맨틱 버저닝 준수**: `v{MAJOR}.{MINOR}.{PATCH}` 정규 표현식 기반 태그 트리거.

### 1.3 워크플로우 파일 구조 결정

```
.github/
├── workflows/
│   ├── ci.yml             # PR/push 검증 (필수)
│   ├── release.yml        # 태그 릴리스 + Docker 멀티아키 (필수, 통합)
│   └── codeql.yml         # 향후 확장 (v1에서 미구현)
├── dependabot.yml         # 의존성 업데이트 (필수)
└── release.yml (선택)     # 릴리스 노트 카테고리 템플릿 (OP)
```

결정: `docker-publish`는 별도 워크플로우로 분리하지 않고 `release.yml` 내 Job으로 통합한다. 이유는 다음과 같다.

- 릴리스 바이너리와 Docker 이미지의 버전 정합성 보장이 용이함
- 동일 릴리스 컨텍스트에서 태그, 체크섬, 이미지가 함께 갱신되어야 함
- Job 의존성(`needs`)으로 품질 게이트를 단일 파이프라인에서 관리

---

## 2. 파일별 구현 계획

### 2.1 Phase 1: CI 워크플로우 (P0 - 최우선)

#### `.github/workflows/ci.yml`

**트리거 정의**

- `pull_request`: `[opened, synchronize, reopened]`
- `push`: `branches: [main, develop]`
- `workflow_dispatch`: 수동 실행 허용

**권한 설정**

- `permissions: contents: read`만 부여(최소 권한)

**매트릭스 구성**

- `strategy.fail-fast: false`로 설정하여 한 OS 실패가 다른 OS 실행을 중단하지 않도록 한다.
- `matrix`:
  - `os: [ubuntu-latest, macos-latest]`
  - `go-version: ['1.25.x']`
  - `node-version: ['20.x']`

**Job 1 `quality-gate`의 Step 구성**

1. `actions/checkout@v4` with `fetch-depth: 0` (릴리스 노트용 히스토리 확보)
2. `actions/setup-go@v5` with `go-version: ${{ matrix.go-version }}`, `cache: true`, `cache-dependency-path: go.sum`
3. `actions/setup-node@v4` with `node-version: ${{ matrix.node-version }}`, `cache: 'npm'`, `cache-dependency-path: web/package-lock.json`
4. `go mod download`
5. `go vet ./...`
6. `golangci/golangci-lint-action@v6` with `version: latest`, `args: --timeout=5m`
7. `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
8. `actions/upload-artifact@v4` for `coverage.out` (이름: `coverage-${{ matrix.os }}`)
9. `cd web && npm ci`
10. `cd web && npm run build`
11. `actions/upload-artifact@v4` for `web/dist` (ubuntu-latest만, 이름: `frontend-dist`)
12. `go install golang.org/x/vuln/cmd/govulncheck@latest`
13. `govulncheck ./...` (HIGH/CRITICAL 발견 시 exit code !=0으로 Job 실패)

**예상 실행 시간**: 8~12분 (캐시 적중 시), 15~20분 (캐시 없음)

**테스트**: 로컬에서 `act` 또는 실제 PR 시뮬레이션으로 사전 검증.

---

### 2.2 Phase 2: Release 워크플로우 (P0)

#### `.github/workflows/release.yml`

**트리거 정의**

- `push`: `tags: ['v[0-9]+.[0-9]+.[0-9]+*']` (시맨틱 버전 패턴, 프리릴리스 포함)
- `workflow_dispatch`:
  - `inputs.version`: string, required (예: `v0.1.0`)
  - `inputs.prerelease`: boolean, default: `false`
  - `inputs.override_vuln`: boolean, default: `false` (취약점 무시 비상 플래그)

**권한 설정**

- `permissions: contents: write, packages: write` (Release 생성 + ghcr.io 푸시)

**Job 구조 및 의존 관계**

```
quality-gate (재사용)
    │
    ├─→ build-frontend ────┐
    │                      │
    └─→ build-binaries ────┤
         (matrix x 6)      │
                           ↓
                        package (matrix x 6)
                           │
                           ↓
                        checksums
                           │
                           ↓
                        release ← docker-publish (병렬)
```

**Job 1 `quality-gate`**

- `ci.yml`과 동일 로직을 재실행하여 릴리스 품질 게이트 강제
- `govulncheck` 실패 시 `inputs.override_vuln == true`가 아닌 한 전체 파이프라인 중단
- 실행 환경: `ubuntu-latest` 단일 (매트릭스 불필요)

**Job 2 `build-frontend`**

- `needs: quality-gate`
- `ubuntu-latest` runner
- Step:
  1. `actions/checkout@v4`
  2. `actions/setup-node@v4` with cache
  3. `cd web && npm ci`
  4. `cd web && npm run build`
  5. `actions/upload-artifact@v4` with `name: frontend-dist`, `path: web/dist`

**Job 3 `build-binaries`**

- `needs: quality-gate`
- `runs-on: ubuntu-latest` (크로스컴파일은 Linux에서 수행)
- `strategy.matrix.platform`:

```yaml
platform:
  - {goos: darwin,  goarch: amd64, goarm: '', suffix: darwin-amd64}
  - {goos: darwin,  goarch: arm64, goarm: '', suffix: darwin-arm64}
  - {goos: linux,   goarch: amd64, goarm: '', suffix: linux-amd64}
  - {goos: linux,   goarch: arm64, goarm: '', suffix: linux-arm64}
  - {goos: linux,   goarch: arm,   goarm: '7', suffix: linux-arm-v7}
  - {goos: linux,   goarch: arm,   goarm: '6', suffix: linux-arm-v6}
```

- Env: `CGO_ENABLED=0`, `GOOS=${{ matrix.platform.goos }}`, `GOARCH=${{ matrix.platform.goarch }}`, `GOARM=${{ matrix.platform.goarm }}`
- Step (바이너리 3종을 loop로 빌드):
  1. `actions/checkout@v4`
  2. `actions/setup-go@v5` with cache
  3. 버전 추출: `echo "VERSION=${GITHUB_REF_NAME}" >> $GITHUB_ENV` (태그) 또는 `inputs.version` (dispatch)
  4. 바이너리별 빌드 명령:

```
for bin in xflowd xflow xflow-agent; do
  go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o dist/${bin}-${{ matrix.platform.suffix }} \
    ./cmd/${bin}
done
```

  5. `actions/upload-artifact@v4` with `name: binaries-${{ matrix.platform.suffix }}`, `path: dist/`

**Job 4 `package`**

- `needs: [build-frontend, build-binaries]`
- `strategy.matrix.platform`: build-binaries와 동일
- `ubuntu-latest` runner
- Step:
  1. `actions/checkout@v4`
  2. `actions/download-artifact@v4` with `name: frontend-dist`, `path: pkg/web/dist`
  3. `actions/download-artifact@v4` with `name: binaries-${{ matrix.platform.suffix }}`, `path: dist/`
  4. 패키지 조립:

```bash
mkdir -p pkg
cp dist/xflowd-${{ matrix.platform.suffix }} pkg/xflowd
cp dist/xflow-${{ matrix.platform.suffix }} pkg/xflow
cp dist/xflow-agent-${{ matrix.platform.suffix }} pkg/xflow-agent
cp deploy/xflow.yaml pkg/
# Linux 플랫폼만 systemd 유닛 및 설치 스크립트 포함
if [[ "${{ matrix.platform.goos }}" == "linux" ]]; then
  cp deploy/xflowd.service pkg/
  cp deploy/install.sh pkg/
  cp deploy/uninstall.sh pkg/
fi
cp README.md pkg/ || true
# macOS Gatekeeper 가이드 포함
if [[ "${{ matrix.platform.goos }}" == "darwin" ]]; then
  cp docs/gatekeeper-bypass.md pkg/ 2>/dev/null || true
fi
tar czf xflow-${VERSION}-${{ matrix.platform.suffix }}.tar.gz -C pkg .
```

  5. `actions/upload-artifact@v4` with `name: package-${{ matrix.platform.suffix }}`, `path: xflow-*.tar.gz`

**Job 5 `checksums`**

- `needs: package`
- `ubuntu-latest` runner
- Step:
  1. `actions/download-artifact@v4` with `pattern: package-*`, `path: packages/`, `merge-multiple: true`
  2. `cd packages && sha256sum xflow-*.tar.gz > checksums.txt`
  3. `actions/upload-artifact@v4` with `name: checksums`, `path: packages/checksums.txt`

**Job 6 `release`**

- `needs: checksums`
- `ubuntu-latest` runner
- `permissions: contents: write`
- Step:
  1. `actions/checkout@v4`
  2. `actions/download-artifact@v4` with `pattern: package-*`, `path: release-assets/`, `merge-multiple: true`
  3. `actions/download-artifact@v4` with `name: checksums`, `path: release-assets/`
  4. 프리릴리스 판별:

```bash
IS_PRERELEASE=false
if [[ "${GITHUB_REF_NAME}" =~ -(rc|beta|alpha)\. ]] || \
   [[ "${{ inputs.prerelease }}" == "true" ]]; then
  IS_PRERELEASE=true
fi
echo "IS_PRERELEASE=${IS_PRERELEASE}" >> $GITHUB_ENV
```

  5. `softprops/action-gh-release@v2`:

```yaml
with:
  tag_name: ${{ env.VERSION }}
  name: xflow ${{ env.VERSION }}
  generate_release_notes: true
  prerelease: ${{ env.IS_PRERELEASE }}
  files: |
    release-assets/*.tar.gz
    release-assets/checksums.txt
```

**Job 7 `docker-publish`**

- `needs: quality-gate` (바이너리 빌드와 독립적 - Docker 내부에서 자체 빌드)
- `ubuntu-latest` runner
- `permissions: contents: read, packages: write`
- Step:
  1. `actions/checkout@v4`
  2. `docker/setup-qemu-action@v3` (linux/arm64 에뮬레이션)
  3. `docker/setup-buildx-action@v3`
  4. `docker/login-action@v3` with `registry: ghcr.io`, `username: ${{ github.actor }}`, `password: ${{ secrets.GITHUB_TOKEN }}`
  5. 태그 메타데이터 생성 (`docker/metadata-action@v5`):

```yaml
tags: |
  type=semver,pattern={{version}}
  type=semver,pattern={{major}}.{{minor}}
  type=raw,value=latest,enable=${{ !env.IS_PRERELEASE }}
```

  6. `docker/build-push-action@v6`:

```yaml
platforms: linux/amd64,linux/arm64
context: .
file: ./Dockerfile
push: true
tags: ${{ steps.meta.outputs.tags }}
labels: ${{ steps.meta.outputs.labels }}
build-args: |
  VERSION=${{ env.VERSION }}
cache-from: type=gha
cache-to: type=gha,mode=max
```

**예상 실행 시간**: 30~45분 (전체 파이프라인, Docker 빌드가 가장 오래 걸림)

---

### 2.3 Phase 3: Dockerfile 설계 (P0)

#### `Dockerfile` (프로젝트 최상위)

**멀티스테이지 빌드 구조**

```dockerfile
# Stage 1: Frontend builder
FROM node:20-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Go binary builder
FROM golang:1.25-alpine AS go-builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/xflowd ./cmd/xflowd
RUN CGO_ENABLED=0 go build \
    -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/xflow ./cmd/xflow
RUN CGO_ENABLED=0 go build \
    -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/xflow-agent ./cmd/xflow-agent

# Stage 3: Runtime (distroless)
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /opt/xflow
COPY --from=go-builder /out/xflowd /opt/xflow/xflowd
COPY --from=go-builder /out/xflow /usr/local/bin/xflow
COPY --from=go-builder /out/xflow-agent /usr/local/bin/xflow-agent
COPY --from=web-builder /web/dist /opt/xflow/web/dist
COPY deploy/xflow.yaml /etc/xflow/xflow.yaml
EXPOSE 8080
VOLUME ["/opt/xflow/data"]
USER nonroot:nonroot
ENTRYPOINT ["/opt/xflow/xflowd"]
CMD ["--config", "/etc/xflow/xflow.yaml"]
```

**설계 결정**

- **Runtime 이미지**: `gcr.io/distroless/base-debian12:nonroot` 선택
  - 이유: TLS 루트 CA 포함(HTTPS 클라이언트 지원), nonroot 사용자 기본 설정(보안), 최소 공격 표면
  - 대안 `alpine:3`: 패키지 매니저(apk)가 있어 공격 표면 증가. 선택하지 않음.
- **프론트엔드 포함**: `web/dist`를 이미지에 포함하여 단일 컨테이너로 UI 제공
- **멀티아키**: `docker/build-push-action` + QEMU로 `linux/amd64`, `linux/arm64` 지원
- **포트**: 기본 8080 (xflowd HTTP 서버 포트와 일치해야 함, 설정 기반)
- **볼륨**: `/opt/xflow/data` (xflowd.service의 `ReadWritePaths`와 일치)

---

### 2.4 Phase 4: Dependabot 설정 (P0)

#### `.github/dependabot.yml`

```yaml
version: 2
updates:
  # Go modules
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "09:00"
      timezone: "Asia/Seoul"
    open-pull-requests-limit: 5
    labels:
      - "dependencies"
      - "go"
    groups:
      go-minor-patch:
        update-types:
          - "minor"
          - "patch"
    commit-message:
      prefix: "chore(deps)"
      include: "scope"

  # npm (web/)
  - package-ecosystem: "npm"
    directory: "/web"
    schedule:
      interval: "weekly"
      day: "monday"
      time: "09:00"
      timezone: "Asia/Seoul"
    open-pull-requests-limit: 5
    labels:
      - "dependencies"
      - "javascript"
    groups:
      npm-minor-patch:
        update-types:
          - "minor"
          - "patch"
    commit-message:
      prefix: "chore(deps)"
      include: "scope"

  # GitHub Actions
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
      day: "monday"
    open-pull-requests-limit: 3
    labels:
      - "dependencies"
      - "github-actions"
    commit-message:
      prefix: "chore(ci)"
      include: "scope"
```

**설계 결정**

- **주간 스케줄**: 월요일 오전 9시(KST) 일괄 처리로 리뷰 집중도 향상
- **보안 업데이트**: Dependabot은 `schedule`과 무관하게 보안 권고 발견 시 즉시 PR 생성(기본 동작)
- **그룹핑**: minor/patch 업데이트를 단일 PR로 그룹화하여 리뷰 부담 감소
- **Major 업데이트**: 개별 PR로 생성(기본 동작)되어 Breaking Changes 수동 검증 가능
- **라벨링**: `dependencies`, `go`, `javascript`, `github-actions`로 자동 분류

---

### 2.5 Phase 5: govulncheck 통합 (P0)

#### CI 파이프라인 통합 (`ci.yml` 내 Step)

```yaml
- name: Install govulncheck
  run: go install golang.org/x/vuln/cmd/govulncheck@latest

- name: Run govulncheck
  id: vuln
  run: govulncheck -show verbose ./...
  continue-on-error: false
```

**실패 조건**

- `govulncheck`가 HIGH/CRITICAL 취약점 탐지 시 Job이 즉시 실패
- CI 레벨에서는 `continue-on-error: false`로 엄격하게 차단
- 릴리스 워크플로우에서 `inputs.override_vuln == true`인 경우에만 실패 무시 (비상용)

**릴리스 파이프라인 통합**

```yaml
- name: Run govulncheck (release gate)
  run: govulncheck ./...
  continue-on-error: ${{ inputs.override_vuln || false }}
```

---

### 2.6 Phase 6: Makefile 확장안 (P1, OP)

#### 기존 `Makefile`의 `rpi`, `rpi-pkg`를 일반화

```makefile
# 플랫폼 매개변수화
TARGET_GOOS   ?= linux
TARGET_GOARCH ?= amd64
TARGET_GOARM  ?=
TARGET_SUFFIX := $(TARGET_GOOS)-$(TARGET_GOARCH)$(if $(TARGET_GOARM),-v$(TARGET_GOARM),)
VERSION       ?= dev

# 일반화된 크로스컴파일 타깃
## cross: Cross-compile for TARGET_GOOS/TARGET_GOARCH[/TARGET_GOARM]
cross: web
	CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOARM=$(TARGET_GOARM) \
		go build $(GOFLAGS) -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o $(BUILD_DIR)/$(APP)-$(TARGET_SUFFIX) ./cmd/$(APP)
	@for cli in $(CLI_APPS); do \
		CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) GOARM=$(TARGET_GOARM) \
		go build $(GOFLAGS) -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o $(BUILD_DIR)/$$cli-$(TARGET_SUFFIX) ./cmd/$$cli; \
	done

## cross-pkg: Create tar.gz package for TARGET_GOOS/TARGET_GOARCH[/TARGET_GOARM]
cross-pkg: cross
	rm -rf $(PKG_DIR)
	mkdir -p $(PKG_DIR)/web
	cp $(BUILD_DIR)/$(APP)-$(TARGET_SUFFIX) $(PKG_DIR)/xflowd
	@for cli in $(CLI_APPS); do \
		cp $(BUILD_DIR)/$$cli-$(TARGET_SUFFIX) $(PKG_DIR)/$$cli; \
	done
	cp -r $(WEB_DIST) $(PKG_DIR)/web/dist
	cp deploy/xflow.yaml $(PKG_DIR)/
	@if [ "$(TARGET_GOOS)" = "linux" ]; then \
		cp deploy/xflowd.service $(PKG_DIR)/; \
		cp deploy/install.sh $(PKG_DIR)/; \
		cp deploy/uninstall.sh $(PKG_DIR)/; \
	fi
	cd $(BUILD_DIR) && tar czf xflow-$(VERSION)-$(TARGET_SUFFIX).tar.gz -C pkg .
	rm -rf $(PKG_DIR)
	@echo "Package: $(BUILD_DIR)/xflow-$(VERSION)-$(TARGET_SUFFIX).tar.gz"

## release-all: Build all 6 platform packages locally
release-all:
	@$(MAKE) cross-pkg TARGET_GOOS=darwin TARGET_GOARCH=amd64
	@$(MAKE) cross-pkg TARGET_GOOS=darwin TARGET_GOARCH=arm64
	@$(MAKE) cross-pkg TARGET_GOOS=linux  TARGET_GOARCH=amd64
	@$(MAKE) cross-pkg TARGET_GOOS=linux  TARGET_GOARCH=arm64
	@$(MAKE) cross-pkg TARGET_GOOS=linux  TARGET_GOARCH=arm   TARGET_GOARM=7
	@$(MAKE) cross-pkg TARGET_GOOS=linux  TARGET_GOARCH=arm   TARGET_GOARM=6

## checksums: Generate SHA256 checksums for all release tarballs
checksums:
	cd $(BUILD_DIR) && sha256sum xflow-*.tar.gz > checksums.txt
	@echo "Checksums: $(BUILD_DIR)/checksums.txt"

# 기존 rpi 타깃은 하위 호환을 위해 유지 (alias)
rpi:
	@$(MAKE) cross TARGET_GOOS=linux TARGET_GOARCH=$(RPI_GOARCH) TARGET_GOARM=$(RPI_GOARM)

rpi-pkg:
	@$(MAKE) cross-pkg TARGET_GOOS=linux TARGET_GOARCH=$(RPI_GOARCH) TARGET_GOARM=$(RPI_GOARM)
```

**구현 결정**

- 기존 `rpi`, `rpi-pkg` 타깃은 `cross`, `cross-pkg`로의 alias로 재구현하여 하위 호환성 유지
- `release-all`, `checksums`는 신규 타깃으로 추가
- `VERSION`은 기본값 `dev`이며, 릴리스 시 `make release-all VERSION=v0.1.0` 형태로 지정
- `-trimpath`, `-s -w` 링커 플래그로 바이너리 크기 축소 및 재현 가능 빌드

---

### 2.7 Phase 7: macOS Gatekeeper 우회 가이드 (P0)

#### `docs/gatekeeper-bypass.md` (신규 문서, 릴리스 노트에도 포함)

**v1 제약 명시**: 본 배포의 macOS 바이너리는 코드서명 및 공증이 되지 않은 상태로 배포된다. Apple Developer ID 및 Notarization 구축은 향후 작업(Future Work) 대상이다.

**사용자 절차 (릴리스 노트 템플릿에 포함)**

1. 다운로드한 `tar.gz` 압축 해제
2. 첫 실행 시 "확인되지 않은 개발자" 경고 발생
3. 우회 방법 A (Control-click): Finder에서 `xflowd`에 Control-click → "열기" → 확인 대화 상자에서 "열기"
4. 우회 방법 B (System Settings): System Settings → Privacy & Security → 하단 "그래도 실행 허용"
5. 터미널 우회: `xattr -d com.apple.quarantine /path/to/xflowd`
6. 이후 실행은 경고 없이 진행됨

### 2.8 Phase 8: 버전 태깅 전략 (P0)

#### SemVer 태깅 규칙

- **정식 릴리스**: `v{MAJOR}.{MINOR}.{PATCH}` (예: `v1.0.0`, `v1.2.3`)
- **프리릴리스**:
  - Release Candidate: `v1.0.0-rc.1`, `v1.0.0-rc.2`
  - Beta: `v1.0.0-beta.1`
  - Alpha: `v1.0.0-alpha.1`
- **개발 빌드**: 태그 없음, `main.version=dev` 기본값

#### 태그 생성 절차

```bash
# 1. main 브랜치 최신화
git checkout main && git pull

# 2. 태그 생성 (annotated)
git tag -a v0.1.0 -m "Release v0.1.0: Initial CI/CD integration"

# 3. 태그 푸시 (release.yml 자동 트리거)
git push origin v0.1.0
```

#### 빌드 시 버전 주입

- 워크플로우: `VERSION=${GITHUB_REF_NAME}` (태그명 그대로 사용)
- 로컬: `make release-all VERSION=v0.1.0`
- Docker: `docker build --build-arg VERSION=v0.1.0 ...`

#### 런타임 출력 (각 cmd/*/main.go에서 구현 필요, 본 SPEC 범위 외)

```go
var version = "dev"

// --version 플래그 처리 시 출력
fmt.Printf("xflowd version %s\n", version)
```

---

### 2.9 Phase 9: CGO 호환성 감사 (P0)

#### 의존성 CGO 감사 결과 (go.mod 기준)

| 패키지 | 순수 Go? | 비고 |
|--------|----------|------|
| `github.com/jackc/pgx/v5` | O | PostgreSQL 드라이버, pure Go |
| `github.com/eclipse/paho.mqtt.golang` | O | MQTT 클라이언트, pure Go |
| `github.com/InfluxCommunity/influxdb3-go` | O | InfluxDB v3 클라이언트 |
| `github.com/influxdata/influxdb-client-go/v2` | O | InfluxDB v2 클라이언트 |
| `github.com/gorilla/websocket` | O | WebSocket, pure Go |
| `github.com/prometheus/client_golang` | O | Prometheus 클라이언트 |
| `modernc.org/sqlite` | O | CGO-free SQLite 재구현체 (핵심 검증 대상) |
| `go.bug.st/serial` | 확인 필요 | 시리얼 포트 접근, OS별 syscall 사용 |
| `github.com/spf13/cobra` | O | CLI 프레임워크 |
| `github.com/spf13/viper` | O | 설정 관리 |

**위험 항목**: `go.bug.st/serial`은 syscall 기반으로 OS별 분기 처리를 한다. 현재 버전(v1.6.4)은 `CGO_ENABLED=0`에서 linux/arm, linux/arm64 빌드 성공이 보고되나, CI 매트릭스 빌드에서 전 6개 플랫폼 성공을 최종 확인해야 한다.

**검증 방법**: Phase 2의 `build-binaries` Job이 실패하면 해당 의존성을 추적하여 대체 또는 build tag 분기로 해결한다.

**권장 조치**: PR #1(CI 설정)에서 `go build -v -x` 로그를 수집하여 각 플랫폼별 빌드 성공 여부를 확인한다.

---

## 3. 시크릿 및 권한 요구사항

### 3.1 v1에 필요한 시크릿

| 시크릿 | 제공 주체 | 용도 | 필수 여부 |
|--------|-----------|------|-----------|
| `GITHUB_TOKEN` | GitHub Actions 자동 | 릴리스 발행, ghcr.io 푸시 | 필수 (자동) |

**v1에서 추가 시크릿 설정 불필요**: `GITHUB_TOKEN`만으로 모든 기본 동작 가능.

### 3.2 향후 필요 시크릿 (Future Work)

| 시크릿 | 용도 |
|--------|------|
| `APPLE_ID_CERT_BASE64` | macOS 코드사이닝용 Developer ID 인증서(base64 인코딩) |
| `APPLE_ID_CERT_PASSWORD` | 인증서 비밀번호 |
| `APPLE_ID` | Apple ID 계정 (공증용) |
| `APPLE_APP_PASSWORD` | 앱 전용 비밀번호 |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `COSIGN_PRIVATE_KEY` | 아티팩트 서명용 cosign 키 |
| `COSIGN_PASSWORD` | cosign 키 비밀번호 |

### 3.3 브랜치 보호 규칙 (운영 권고사항, 코드로 강제 불가)

**`main` 브랜치**:

- PR을 통해서만 머지 허용 (직접 푸시 차단)
- `ci.yml` 통과를 머지 필수 조건으로 설정
- 최소 1명의 승인 리뷰 요구 (팀 모드 시)
- 강제 푸시 차단
- 삭제 차단

**태그 보호**:

- `v*.*.*` 패턴 태그는 `main` 브랜치에서만 생성 가능 (Rulesets 사용)

이 항목들은 SPEC이 아닌 GitHub 저장소 설정에서 관리하며, 체크리스트로만 문서화한다.

---

## 4. 구현 우선순위 (Priority Matrix)

| 우선순위 | 작업 항목 | 설명 | 선행 조건 |
|---------|-----------|------|-----------|
| P0 | ci.yml | PR/push 품질 게이트 | Go 1.25, Node 20 설치 가능 |
| P0 | Dockerfile | 멀티스테이지 빌드 정의 | go.mod, web/package.json 존재 |
| P0 | release.yml (build Jobs) | 6 플랫폼 매트릭스 빌드 | ci.yml 완료, Dockerfile 완료 |
| P0 | release.yml (docker-publish) | ghcr.io 멀티아키 발행 | Dockerfile 완료 |
| P0 | dependabot.yml | 의존성 업데이트 자동화 | 없음 |
| P0 | govulncheck 통합 | CI 및 릴리스 게이트 | ci.yml 완료 |
| P0 | docs/gatekeeper-bypass.md | macOS 사용자 가이드 | 없음 |
| P1 | Makefile 확장 | `release-all`, `checksums` | 기존 `rpi-pkg` 로직 검증 |
| P1 | release.yml (workflow_dispatch) | 수동 프리릴리스 지원 | release.yml 기본 완료 |
| P2 | codeql.yml | 심층 보안 분석 | 향후 확장 |

---

## 5. 위험 및 완화 방안

### 5.1 기술적 위험

| 위험 | 영향 | 확률 | 완화 방안 |
|------|------|------|-----------|
| `go.bug.st/serial` CGO 의존 발견 | 전 플랫폼 빌드 실패 | 중 | 매트릭스 빌드로 조기 탐지, build tag 분기 또는 대체 패키지 검토 |
| Docker QEMU arm64 빌드 타임아웃 | 릴리스 지연 | 중 | `cache-to: type=gha,mode=max`로 레이어 캐싱, 필요 시 빌드 분리 |
| GitHub Actions runner OS 업그레이드로 동작 변경 | CI 간헐적 실패 | 저 | `ubuntu-latest` 대신 명시적 버전(`ubuntu-24.04`) 고려 |
| modernc.org/sqlite의 arm6 지원 불완전 | RPi Zero 빌드 실패 | 중 | CI에서 조기 확인, 실패 시 arm6를 P1에서 P2로 강등 검토 |
| macOS 사용자 Gatekeeper 불편 | 사용자 이탈 | 고 | 릴리스 노트 우회 가이드 명확히, Future Work으로 코드사이닝 추진 |
| `softprops/action-gh-release` 동작 변경 | 릴리스 실패 | 저 | 버전 고정(`@v2` 대신 `@v2.1.0` 등 마이너 고정) 가능 |
| Dependabot 과다 PR 생성 | 리뷰 부담 | 중 | `open-pull-requests-limit` 및 minor/patch 그룹핑 |

### 5.2 운영적 위험

| 위험 | 영향 | 완화 방안 |
|------|------|-----------|
| 시크릿 유출 (ghcr.io 토큰 등) | 공급망 공격 | `GITHUB_TOKEN` 기본 사용, 로그 마스킹, Job별 최소 권한 |
| 릴리스 태그 오발행 | 사용자 혼란 | annotated tag + 태그 보호 Rulesets |
| CI 비용 증가 (macOS runner) | 예산 초과 | CI는 ubuntu/macos 매트릭스, 릴리스는 ubuntu 단일 runner |
| Docker 이미지 크기 과다 | pull 시간 증가 | distroless base 사용, `.dockerignore` 적용 |

### 5.3 `.dockerignore` 권장 내용

```
.git
.github
.moai
.vscode
node_modules
web/node_modules
bin
web/dist
*.md
*.log
Makefile
```

빌드 컨텍스트 최소화로 빌드 속도 및 이미지 크기 개선.

---

## 6. 향후 작업 (Future Work)

v1 이후 점진적으로 도입할 개선 사항:

### 6.1 macOS 코드사이닝 및 공증 (Priority: High)

- Apple Developer Program 가입
- Developer ID Application 인증서 발급 및 GitHub Secrets 등록
- `release.yml`에 macOS 서명 Step 추가 (`codesign`, `xcrun notarytool`)
- 공증 완료 후 `stapler staple`로 티켓 첨부
- 사용자는 Gatekeeper 경고 없이 실행 가능

### 6.2 아티팩트 서명 (cosign) (Priority: Medium)

- `sigstore/cosign`으로 `tar.gz` 및 Docker 이미지 서명
- 키리스 서명(keyless signing with OIDC) 또는 키 기반 서명
- 사용자는 `cosign verify`로 무결성 검증

### 6.3 SBOM 생성 (Priority: Medium)

- `anchore/sbom-action` 또는 `cyclonedx-gomod`로 SPDX/CycloneDX SBOM 생성
- 릴리스 아티팩트에 `xflow-{version}.sbom.json` 첨부
- 공급망 투명성 확보

### 6.4 Windows 지원 (Priority: Low)

- `windows/amd64`, `windows/arm64` 빌드 타깃 추가
- `.exe` 확장자 처리, zip 패키징(Windows 관례)
- Windows Service 설치 스크립트(.ps1)

### 6.5 CodeQL 심층 보안 분석 (Priority: Medium)

- `.github/workflows/codeql.yml` 추가
- 주간 스케줄 + PR 트리거
- 결과를 Security 탭에 통합

### 6.6 릴리스 서명 키 로테이션 (Priority: Low)

- 1년 단위 cosign 키 교체 절차 문서화
- 이전 키로 서명된 릴리스 검증 가능성 유지

### 6.7 패키지 레지스트리 발행 (Priority: Low)

- Homebrew Tap (macOS 사용자 편의)
- APT repository (Debian/Ubuntu)
- RPM repository (Fedora/RHEL)

### 6.8 Kubernetes Helm Chart (Priority: Low)

- `charts/xflow/` 디렉터리 추가
- `helm package` 후 GitHub Pages에 publish 또는 Helm 레지스트리(ghcr.io) 발행

---

## 7. 파일 산출물 요약

```
.github/
├── workflows/
│   ├── ci.yml              # Phase 1 (P0)
│   └── release.yml         # Phase 2 + 3 (P0, docker-publish 통합)
└── dependabot.yml          # Phase 4 (P0)

./
├── Dockerfile              # Phase 3 (P0)
├── .dockerignore           # Phase 3 (P0)
└── Makefile                # Phase 6 확장 (P1)

docs/
└── gatekeeper-bypass.md    # Phase 7 (P0)
```

구현 순서 권장:

1. `dependabot.yml` 및 `docs/gatekeeper-bypass.md` (가장 빠른 가치, 독립 실행)
2. `ci.yml` 최소 버전 작성 → PR 머지 → 이후 반복 개선
3. `Dockerfile` + `.dockerignore` 작성 후 로컬 `docker buildx build` 검증
4. `release.yml` build-frontend + build-binaries Job 작성 → 수동 dispatch로 검증
5. `release.yml` package + checksums + release Job 추가 → 드라이런 태그로 검증
6. `release.yml` docker-publish Job 추가 → ghcr.io 최초 발행 검증
7. `Makefile` 확장 (선택, 로컬 개발 편의용)
8. 브랜치 보호 Rulesets 수동 설정
9. 첫 정식 태그 `v0.1.0` 발행 및 End-to-End 검증

---

## 8. 인수 검증 및 테스트 접근

세부 인수 시나리오는 `acceptance.md`에서 Given-When-Then 형식으로 정의한다. 본 SPEC의 테스트 접근은 다음과 같이 구분한다:

- **단위 검증**: 각 워크플로우 Job을 독립적으로 `workflow_dispatch`로 수동 트리거하여 동작 확인
- **통합 검증**: 프리릴리스 태그(예: `v0.0.1-rc.1`)를 발행하여 End-to-End 파이프라인 검증
- **회귀 검증**: 태그 삭제 후 재발행으로 idempotency 확인 (단, `softprops/action-gh-release`는 기본적으로 기존 릴리스 갱신)
- **수동 스모크 테스트**: 릴리스 후 6개 플랫폼 중 최소 3개(darwin/arm64, linux/amd64, linux/arm64)에서 바이너리 실행 확인
- **체크섬 검증**: `shasum -a 256 -c checksums.txt` 수동 실행
- **Docker 검증**: `docker pull ghcr.io/{owner}/xflow:{version}` 후 `docker run --rm xflow:version --version` 실행

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-21*
*작성: MoAI SPEC Builder (manager-spec)*
