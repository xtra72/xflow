---
id: SPEC-CICD-001
version: "1.0.0"
status: draft
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-21 | 1.0.0 | 초기 SPEC 작성 (CI/CD 파이프라인 및 멀티 플랫폼 릴리스 시스템) |

---

# SPEC-CICD-001: CI/CD Pipeline & Multi-Platform Release System - GitHub Actions 기반 CI/CD, 6개 플랫폼 릴리스 자동화, 취약점 스캐닝, Docker 멀티아키 이미지 배포

## 1. 개요 (Overview)

XFlow 프로젝트의 지속적 통합(Continuous Integration)과 지속적 배포(Continuous Delivery)를 위한 GitHub Actions 기반 파이프라인 및 멀티 플랫폼 릴리스 시스템을 정의한다. 본 SPEC은 Go 백엔드(xflowd, xflow, xflow-agent) 3개 바이너리와 TypeScript/Vite 프론트엔드(web/dist)로 구성된 하이브리드 프로젝트의 품질 검증, 취약점 스캐닝, 크로스 플랫폼 빌드, Docker 이미지 배포, 의존성 업데이트 자동화를 포괄한다.

XFlow는 IoT Flow Engine으로서 데스크톱 환경(macOS/Linux)과 엣지 디바이스(Raspberry Pi 전 세대)에서 동작해야 한다. 이에 따라 단일 tag 기반 릴리스로 6개 플랫폼(darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, linux/arm GOARM=7, linux/arm GOARM=6)에 대한 바이너리 및 배포 패키지를 자동 생성하며, 각 패키지는 Go 바이너리 3종 + `web/dist/` 정적 자산 + `deploy/` 설치 스크립트/systemd 유닛을 포함한다. 추가로 Linux x86_64/ARM64용 Docker 이미지를 `ghcr.io`에 발행한다.

현재 `.github/workflows/` 디렉터리가 존재하지 않으며(그린필드), 기존 `Makefile`의 `rpi`, `rpi-pkg` 타깃과 `deploy/` 디렉터리의 설치 자산(`install.sh`, `uninstall.sh`, `xflowd.service`, `xflow.yaml`)을 적극 재사용하여 CI/CD를 구축한다.

본 SPEC은 다음을 포함한다:

- **ci.yml**: PR/push 시 lint, vet, race test, 프론트엔드 빌드, govulncheck 실행
- **release.yml**: 태그(`v*.*.*`) 또는 수동 dispatch 시 6개 플랫폼 바이너리 빌드 및 패키징
- **docker.yml (또는 release.yml 통합)**: linux/amd64, linux/arm64 멀티아키 이미지를 ghcr.io에 발행
- **dependabot.yml**: gomod, npm, github-actions 3개 생태계 주간 업데이트
- **Dockerfile**: 멀티스테이지 빌드(builder + distroless runtime)
- **Makefile 확장안**: `release-all`, `checksums` 타깃 로컬 미러링
- **버전 태깅 전략**: SemVer, 프리릴리스, `-ldflags`를 통한 버전 주입
- **macOS Gatekeeper 우회 절차**: v1에서 코드사이닝 미수행, 사용자 릴리스 노트 가이드 제공

---

## 2. 범위 (Scope)

### 2.1 IN SCOPE (본 SPEC 범위 - v1)

- GitHub Actions 기반 CI(ci.yml) 및 릴리스(release.yml) 워크플로우 파일 설계
- 6개 플랫폼 빌드 매트릭스(darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, linux/arm v7, linux/arm v6)
- 각 플랫폼별 xflowd, xflow, xflow-agent 3개 바이너리 + web/dist + deploy 자산 포함 tar.gz 패키지
- SHA256 checksums.txt 생성 및 릴리스 첨부
- `govulncheck` 취약점 스캐닝 (CI 및 릴리스 게이트)
- `golangci-lint` + `go vet` + `go test -race -coverprofile` 품질 게이트
- 프론트엔드 빌드(`cd web && npm ci && npm run build`) 및 `web/dist/` 아티팩트 전달
- `ghcr.io`에 linux/amd64 + linux/arm64 멀티아키 Docker 이미지 발행
- Dependabot 주간 업데이트 (gomod, npm, github-actions)
- `-ldflags "-X main.version={tag}"`를 통한 버전 주입
- 수동 `workflow_dispatch` 트리거 지원 (프리릴리스 플래그 입력)
- `softprops/action-gh-release@v2`를 통한 릴리스 노트 자동 생성
- 프로덕션 태그 푸시부터 릴리스 발행까지 완전 자동화

### 2.2 OUT OF SCOPE (v1에서 제외, 향후 작업)

- macOS 코드 서명 및 공증(Notarization) - 사용자에게 Gatekeeper 우회 절차 안내로 대체
- Windows 빌드 타깃 (windows/amd64, windows/arm64)
- `cosign`을 통한 아티팩트 서명
- SBOM(Software Bill of Materials) 생성 (SPDX, CycloneDX)
- Kubernetes Helm 차트 발행
- 자동 롤백(rollback) 메커니즘
- CodeQL 심층 보안 스캐닝(.github/workflows/codeql.yml은 향후 확장)
- Homebrew, APT, RPM 패키지 레지스트리 발행
- 릴리스 서명 키 로테이션 자동화

### 2.3 Non-Goals (비고항목)

본 SPEC은 다음 항목을 **의도적으로** 배제한다:

- **Windows 지원**: v1에서 Windows 타깃은 비고 대상이며, 사용자 요구 증가 시 별도 SPEC으로 다룬다.
- **Kubernetes Helm Charts**: 배포 자동화는 systemd(Linux) 및 Docker 컨테이너 수준까지만 지원한다.
- **자동 롤백**: 릴리스 실패 시 수동 태그 재지정 및 재배포로 대응하며, 자동 롤백 메커니즘은 구현하지 않는다.
- **런타임 메트릭 통합**: CI/CD 파이프라인 자체의 관찰성(메트릭, 알림)은 v1에서 GitHub Actions 기본 로그에 의존한다.

---

## 3. 용어 정의 (Terminology)

| 용어 | 정의 |
|------|------|
| CI (Continuous Integration) | PR/push 시 자동으로 실행되는 코드 품질 검증 파이프라인 |
| CD (Continuous Delivery) | 태그 푸시 시 자동으로 바이너리를 빌드하고 릴리스를 발행하는 파이프라인 |
| GitHub Actions | GitHub가 제공하는 워크플로우 자동화 플랫폼 |
| Workflow | `.github/workflows/*.yml`에 정의된 자동화 절차 |
| Job | 워크플로우 내의 독립 실행 단위 (runner 1대에서 실행) |
| Matrix Build | 여러 OS/아키 조합을 병렬 빌드하는 GitHub Actions 기능 |
| SemVer | Semantic Versioning, `vMAJOR.MINOR.PATCH` 형식 |
| Prerelease | `v1.0.0-rc.1`, `v1.0.0-beta.2` 등 정식 릴리스 이전 버전 |
| govulncheck | Go 공식 취약점 스캐너 (Go 모듈 의존성 취약점 탐지) |
| Dependabot | GitHub의 의존성 자동 업데이트 봇 |
| ghcr.io | GitHub Container Registry (Docker 이미지 호스팅) |
| QEMU | 멀티아키 Docker 이미지 빌드를 위한 에뮬레이터 |
| Distroless | Google의 최소 런타임 컨테이너 이미지 (패키지 매니저 없음) |
| Gatekeeper | macOS의 서명되지 않은 바이너리 실행 차단 기능 |
| LDFLAGS | Go 링커 플래그, 빌드 시 변수 주입에 사용 (`-X main.version=...`) |
| CGO_ENABLED=0 | CGO 비활성화, 정적 바이너리 크로스컴파일에 필수 |

---

## 4. 의존성 (Dependencies)

### 4.1 외부 의존성 (GitHub Actions)

| 액션 | 버전 | 용도 |
|------|------|------|
| `actions/checkout` | v4 | 소스 체크아웃 |
| `actions/setup-go` | v5 | Go 1.25.x 설치 및 모듈 캐싱 |
| `actions/setup-node` | v4 | Node.js 20 LTS 설치 및 npm 캐싱 |
| `actions/cache` | v4 | Go 모듈 및 npm 캐시 |
| `actions/upload-artifact` | v4 | 아티팩트 업로드 (frontend build) |
| `actions/download-artifact` | v4 | 아티팩트 다운로드 (packaging 단계) |
| `golangci/golangci-lint-action` | v6 | Go 린터 실행 |
| `softprops/action-gh-release` | v2 | GitHub Release 생성 및 에셋 업로드 |
| `docker/setup-qemu-action` | v3 | 멀티아키 에뮬레이션 |
| `docker/setup-buildx-action` | v3 | Docker Buildx 활성화 |
| `docker/login-action` | v3 | ghcr.io 로그인 |
| `docker/build-push-action` | v6 | 멀티아키 이미지 빌드 및 푸시 |

### 4.2 외부 도구

| 도구 | 버전 | 용도 |
|------|------|------|
| `govulncheck` | 최신 안정 버전 | Go 모듈 취약점 스캐닝 |
| `golangci-lint` | v1.61+ | Go 정적 분석 |
| `Go` | 1.25.6 (프로젝트 고정) | Go 컴파일러 |
| `Node.js` | 20 LTS | 프론트엔드 빌드 |
| `Docker Buildx` | 최신 | 멀티아키 이미지 빌드 |

### 4.3 프로젝트 내부 자산 재사용

| 자산 | 경로 | 역할 |
|------|------|------|
| `Makefile` | `./Makefile` | `rpi`, `rpi-pkg` 타깃 로직을 `release-all`로 일반화 |
| `xflow.yaml` | `deploy/xflow.yaml` | 기본 설정 파일 (패키지 포함) |
| `xflowd.service` | `deploy/xflowd.service` | systemd 유닛 파일 (패키지 포함) |
| `install.sh` | `deploy/install.sh` | Debian/Ubuntu 설치 스크립트 (패키지 포함) |
| `uninstall.sh` | `deploy/uninstall.sh` | 제거 스크립트 (패키지 포함) |
| `web/package.json` | `web/package.json` | 프론트엔드 빌드 의존성 |
| `go.mod` | `go.mod` | Go 모듈 정의 (모듈 경로 `github.com/xtra/xflow`) |

---

## 5. 가정 사항 (Assumptions)

### 5.1 기술적 가정

- A1: Go 1.25.6 기반 의존성은 모두 순수 Go이며, `CGO_ENABLED=0`으로 크로스컴파일이 가능하다 (modernc.org/sqlite는 CGO-free SQLite 구현체로 검증됨).
- A2: GitHub Actions의 `ubuntu-latest`, `macos-latest` runner는 본 워크플로우 수명 주기 동안 안정적으로 제공된다.
- A3: `ghcr.io`에 이미지를 푸시할 때 `GITHUB_TOKEN`의 `packages: write` 권한으로 충분하다 (외부 PAT 불필요).
- A4: Docker Buildx의 QEMU 에뮬레이션을 통한 `linux/arm64` 이미지 빌드는 허용 가능한 시간 내에 완료된다 (일반적으로 10~20분).
- A5: `softprops/action-gh-release@v2`는 멀티파일 업로드와 자동 릴리스 노트 생성을 안정적으로 지원한다.
- A6: `govulncheck`는 Go 1.25 모듈 그래프를 정확히 해석한다.
- A7: 프로젝트 최상위에서 `go test -race ./...` 및 `cd web && npm ci && npm run build`가 동일하게 `ubuntu-latest`와 `macos-latest`에서 성공한다.

### 5.2 운영/비즈니스 가정

- A8: 릴리스 태그는 `main` 브랜치에서만 생성되며, 브랜치 보호 규칙에 의해 이를 강제한다 (운영 권고사항).
- A9: macOS 사용자는 코드사이닝되지 않은 바이너리에 대해 Gatekeeper 우회 절차를 수행할 의사가 있다 (v1 제약).
- A10: Linux ARMv6 타깃(Raspberry Pi Zero/1/Zero 2W)은 실사용자가 존재하며 빌드/패키징 비용이 정당화된다.
- A11: Dependabot PR은 리뷰어가 수동으로 머지하며, auto-merge는 사용하지 않는다 (v1 정책).
- A12: 릴리스 자동 생성 노트는 커밋 메시지에 의존하며, 팀은 Conventional Commits 규칙을 권장한다 (강제는 아님).

---

## 6. Requirements (요구사항)

본 SPEC은 EARS(Easy Approach to Requirements Syntax) 표기법을 사용하며, 요구사항 ID 체계는 `REQ-CICD-001-{범주}-{번호}` 형식을 따른다. 범주 코드는 UB(Ubiquitous, 항상), EV(Event-driven, 이벤트), ST(State-driven, 상태), UN(Unwanted, 금지), OP(Optional, 선택)이다.

### 6.1 Ubiquitous Requirements (항상 참 - UB)

#### REQ-CICD-001-UB-01: 멀티 플랫폼 바이너리 생성

시스템은 **항상** 단일 릴리스 태그로부터 다음 6개 플랫폼의 바이너리를 생성해야 한다:

1. darwin/amd64 (macOS Intel)
2. darwin/arm64 (macOS Apple Silicon)
3. linux/amd64 (Linux x86_64)
4. linux/arm64 (RPi 4/5, 64-bit ARM 서버)
5. linux/arm GOARM=7 (ARMv7, RPi 2/3/4 32-bit)
6. linux/arm GOARM=6 (ARMv6, RPi Zero/1/Zero 2W)

각 플랫폼 빌드는 `CGO_ENABLED=0`으로 정적 컴파일되며, xflowd, xflow, xflow-agent 3개 바이너리를 포함한다.

#### REQ-CICD-001-UB-02: 릴리스 패키지 구성

시스템은 **항상** 각 플랫폼별 `tar.gz` 패키지에 다음 항목을 포함해야 한다:

- `xflowd`, `xflow`, `xflow-agent` 3개 바이너리 (해당 플랫폼 크로스컴파일 결과물)
- `web/dist/` 정적 자산 (프론트엔드 빌드 결과)
- `xflow.yaml` (`deploy/xflow.yaml`에서 복사)
- `xflowd.service` (`deploy/xflowd.service`에서 복사, Linux 플랫폼 한정)
- `install.sh` (`deploy/install.sh`에서 복사, Linux 플랫폼 한정)
- `uninstall.sh` (`deploy/uninstall.sh`에서 복사, Linux 플랫폼 한정)
- `README.md` 또는 간략 사용 안내 (macOS Gatekeeper 우회 절차 포함)

#### REQ-CICD-001-UB-03: 패키지 명명 규약

시스템은 **항상** 릴리스 아티팩트를 다음 명명 규약에 따라 생성해야 한다:

- 패키지: `xflow-{version}-{os}-{arch}[-v6|-v7].tar.gz`
  - 예: `xflow-v0.1.0-darwin-arm64.tar.gz`, `xflow-v0.1.0-linux-arm-v7.tar.gz`
- 바이너리(패키지 외 개별 업로드 시): `xflowd-{os}-{arch}[-v6|-v7]`, `xflow-{os}-{arch}[-v6|-v7]`, `xflow-agent-{os}-{arch}[-v6|-v7]`
- 체크섬 파일: `checksums.txt` (단일 파일로 모든 `tar.gz` SHA256 포함)

#### REQ-CICD-001-UB-04: 버전 정보 주입

시스템은 **항상** Go 바이너리 빌드 시 `-ldflags "-X main.version={tag}"`를 사용하여 실행 파일 내에 버전 문자열을 주입해야 한다. 바이너리는 `--version` 플래그로 해당 버전을 출력할 수 있어야 한다(구현은 SPEC-CLI-001 등 별도 SPEC 책임이며, 본 SPEC은 주입만 보장한다).

#### REQ-CICD-001-UB-05: 체크섬 생성

시스템은 **항상** 릴리스의 모든 `tar.gz` 파일에 대해 SHA256 해시를 계산하여 `checksums.txt`로 생성하고 릴리스에 첨부해야 한다. 파일 형식은 `shasum -a 256 -c checksums.txt`로 검증 가능해야 한다.

#### REQ-CICD-001-UB-06: CI 품질 게이트 항목

시스템은 **항상** CI 파이프라인에서 다음 게이트를 모두 통과한 경우에만 `success` 상태를 반환해야 한다:

1. `go vet ./...` 통과 (경고 0개)
2. `golangci-lint run` 통과
3. `go test -race -coverprofile=coverage.out ./...` 통과
4. `cd web && npm ci && npm run build` 통과
5. `govulncheck ./...` 통과 (HIGH/CRITICAL 발견 시 실패)

#### REQ-CICD-001-UB-07: 플랫폼 매트릭스 표

시스템은 **항상** 다음 플랫폼 매트릭스를 릴리스 빌드의 기준으로 사용해야 한다:

| 플랫폼 | GOOS | GOARCH | GOARM | 바이너리 접미사 | 패키지 접미사 |
|--------|------|--------|-------|-----------------|---------------|
| macOS Intel | darwin | amd64 | - | `-darwin-amd64` | `-darwin-amd64` |
| macOS Apple Silicon | darwin | arm64 | - | `-darwin-arm64` | `-darwin-arm64` |
| Linux x86_64 | linux | amd64 | - | `-linux-amd64` | `-linux-amd64` |
| Linux ARM64 | linux | arm64 | - | `-linux-arm64` | `-linux-arm64` |
| Linux ARMv7 | linux | arm | 7 | `-linux-arm-v7` | `-linux-arm-v7` |
| Linux ARMv6 | linux | arm | 6 | `-linux-arm-v6` | `-linux-arm-v6` |

#### REQ-CICD-001-UB-08: 트리거 매트릭스

시스템은 **항상** 다음 트리거 매트릭스에 따라 각 워크플로우를 실행해야 한다:

| 워크플로우 | pull_request | push(main/develop) | tag(v*.*.*) | workflow_dispatch |
|------------|:------------:|:------------------:|:-----------:|:-----------------:|
| ci.yml | O | O | - | O |
| release.yml | - | - | O | O (프리릴리스 옵션) |
| docker (릴리스 내 Job) | - | - | O | O |

### 6.2 Event-Driven Requirements (이벤트 기반 - EV)

#### REQ-CICD-001-EV-01: PR 생성/갱신 시 CI 실행

**WHEN** `pull_request` 이벤트(opened, synchronize, reopened)가 발생 시, **THEN** `ci.yml` 워크플로우를 실행하여 lint, vet, race test, 프론트엔드 빌드, govulncheck를 수행해야 한다.

#### REQ-CICD-001-EV-02: 메인 브랜치 푸시 시 CI 실행

**WHEN** `main` 또는 `develop` 브랜치로 `push` 이벤트가 발생 시, **THEN** `ci.yml` 워크플로우를 실행해야 한다.

#### REQ-CICD-001-EV-03: 시맨틱 버전 태그 푸시 시 릴리스 실행

**WHEN** `v{MAJOR}.{MINOR}.{PATCH}` 형식(예: `v0.1.0`, `v1.2.3`, `v1.0.0-rc.1`)의 태그가 푸시될 때, **THEN** `release.yml` 워크플로우가 자동 실행되어 6개 플랫폼 빌드, 패키징, 체크섬 생성, GitHub Release 발행, Docker 이미지 발행을 수행해야 한다.

#### REQ-CICD-001-EV-04: 수동 workflow_dispatch 트리거

**WHEN** 사용자가 GitHub UI 또는 `gh workflow run`으로 `release.yml`을 수동 실행 시, **THEN** 입력된 `prerelease` 불린 플래그에 따라 릴리스를 프리릴리스로 표시하거나 정식 릴리스로 발행해야 한다. 수동 dispatch는 태그를 생성하지 않으며, 빌드 아티팩트는 임시 릴리스(드래프트)로 발행될 수 있다.

#### REQ-CICD-001-EV-05: 매트릭스 빌드 완료 후 패키징

**WHEN** 6개 플랫폼 각각의 바이너리 빌드 Job이 완료 시, **THEN** `package` Job이 프론트엔드 아티팩트와 바이너리를 다운로드하여 플랫폼별 `tar.gz`를 조립해야 한다.

#### REQ-CICD-001-EV-06: 패키징 완료 후 체크섬 생성

**WHEN** 모든 `tar.gz`가 조립 완료 시, **THEN** `checksums` Job이 `sha256sum`을 사용하여 `checksums.txt`를 생성해야 한다.

#### REQ-CICD-001-EV-07: Dependabot PR 생성

**WHEN** Dependabot이 주간 스케줄에 따라 신규 의존성 릴리스를 감지 시, **THEN** 자동으로 업데이트 PR을 생성해야 한다. 대상은 `gomod`, `npm` (web/), `github-actions` 3개 생태계이다.

#### REQ-CICD-001-EV-08: 보안 업데이트 즉시 처리

**WHEN** Dependabot이 보안 권고(Security Advisory)에 해당하는 의존성을 발견 시, **THEN** 주간 스케줄과 별개로 즉시(daily) 보안 PR을 생성해야 한다.

### 6.3 State-Driven Requirements (상태 기반 - ST)

#### REQ-CICD-001-ST-01: 릴리스 진행 조건

**WHILE** 릴리스 워크플로우가 진행 중인 동안, 다음 모든 게이트가 통과한 상태에서만 릴리스 발행 단계로 진입해야 한다:

1. 모든 테스트(`go test -race`) 통과
2. `golangci-lint` 및 `go vet` 통과
3. `govulncheck` 통과 (HIGH/CRITICAL 취약점 0개)
4. 모든 6개 플랫폼 바이너리 빌드 성공
5. 프론트엔드 빌드(`web/dist/`) 성공
6. 패키지 조립 및 체크섬 생성 성공

하나라도 실패 시 `release` Job은 실행되지 않으며, 릴리스가 발행되지 않는다.

#### REQ-CICD-001-ST-02: 프리릴리스 식별

**WHILE** 태그에 `-rc.{N}`, `-beta.{N}`, `-alpha.{N}` 접미사가 포함되어 있거나, `workflow_dispatch`에서 `prerelease=true` 입력이 있는 동안, 생성되는 GitHub Release는 `prerelease: true`로 표시되어야 한다.

#### REQ-CICD-001-ST-03: Docker 태그 전략

**WHILE** 정식 릴리스(`prerelease: false`)가 발행되는 동안, Docker 이미지는 `ghcr.io/{owner}/xflow:{version}`과 `ghcr.io/{owner}/xflow:latest` 두 태그로 동시에 푸시되어야 한다. 프리릴리스의 경우 `:latest` 태그는 갱신하지 않고 `{version}` 태그만 발행한다.

#### REQ-CICD-001-ST-04: 캐시 키 상태 관리

**WHILE** CI 및 릴리스 워크플로우가 실행되는 동안, Go 모듈 캐시 키는 `go.sum` 해시를 기반으로, npm 캐시 키는 `web/package-lock.json` 해시를 기반으로 유지되어야 한다. 동일 해시 내에서는 캐시를 재사용한다.

### 6.4 Unwanted Behavior Requirements (금지/실패 처리 - UN)

#### REQ-CICD-001-UN-01: 테스트 실패 시 릴리스 차단

시스템은 `go test`가 실패한 상태에서 릴리스 아티팩트를 **생성하지 않아야** 한다. `build-binaries` Job은 테스트 Job 완료를 `needs`로 의존해야 한다.

#### REQ-CICD-001-UN-02: 취약점 탐지 시 릴리스 차단

시스템은 `govulncheck`가 HIGH 또는 CRITICAL 수준의 취약점을 보고하는 한 릴리스 발행 단계로 **진입해서는 안 된다**. 단, 수동 dispatch에서 명시적으로 `override_vuln=true`가 입력된 경우에 한해 예외적으로 진행을 허용한다 (감사 로그에 기록).

#### REQ-CICD-001-UN-03: 서명되지 않은 바이너리 `latest` 태그 차단

시스템은 프리릴리스 또는 실패 상태의 릴리스에 대해 Docker `:latest` 태그 및 `:stable` 태그를 갱신해서는 **안 된다**.

#### REQ-CICD-001-UN-04: 시크릿 노출 금지

시스템은 `GITHUB_TOKEN` 외의 시크릿을 워크플로우 로그에 출력하거나, 프리뷰 URL, Job Summary 등에 포함해서는 **안 된다**. 모든 시크릿 참조는 `${{ secrets.NAME }}` 마스킹 메커니즘을 사용해야 한다.

#### REQ-CICD-001-UN-05: `main` 브랜치 강제 푸시 차단

시스템은 `main` 브랜치로의 강제 푸시(force push)를 허용해서는 **안 된다** (GitHub 브랜치 보호 규칙으로 강제, 운영 권고사항).

#### REQ-CICD-001-UN-06: CGO 의존성 재도입 금지

시스템은 `CGO_ENABLED=0` 크로스컴파일을 저해하는 CGO 의존성을 추가해서는 **안 된다**. 신규 의존성 추가 시 CI에서 크로스컴파일 성공 여부를 검증하며, 실패 시 PR을 차단한다 (CI 매트릭스 빌드 검증을 통한 감시).

### 6.5 Optional Requirements (선택 - OP)

#### REQ-CICD-001-OP-01: CodeQL 심층 보안 스캔

**가능하면** GitHub의 CodeQL 정적 분석을 `.github/workflows/codeql.yml`로 추가하여 Go 코드의 심층 보안 분석을 수행할 수 있다. v1에서는 `govulncheck`로 충분하며, CodeQL은 향후 확장 대상이다.

#### REQ-CICD-001-OP-02: 로컬 릴리스 미러링

**가능하면** `Makefile`에 `release-all`, `checksums` 타깃을 추가하여 GitHub Actions 없이도 로컬에서 6개 플랫폼 빌드를 수행할 수 있도록 한다. 기존 `rpi-pkg` 로직을 `TARGET_GOOS`, `TARGET_GOARCH`, `TARGET_GOARM` 매개변수화된 헬퍼로 일반화한다.

#### REQ-CICD-001-OP-03: Homebrew Tap 발행

**가능하면** macOS 사용자를 위한 Homebrew formula를 별도 tap 저장소에 발행할 수 있다. v1에서는 제외한다.

#### REQ-CICD-001-OP-04: 릴리스 노트 템플릿 커스터마이징

**가능하면** `.github/release.yml`을 통해 카테고리별 릴리스 노트 섹션(Features, Bug Fixes, Breaking Changes)을 자동 분류할 수 있다. Conventional Commits 채택 시 유용하다.

#### REQ-CICD-001-OP-05: 테스트 커버리지 리포트 업로드

**가능하면** `coverage.out`을 Codecov 또는 Coveralls에 업로드하여 PR 커버리지 변화를 추적할 수 있다. v1에서는 아티팩트 업로드까지만 수행한다.

---

## 7. 플랫폼 매트릭스 상세

| # | 플랫폼 이름 | GOOS | GOARCH | GOARM | 대상 하드웨어 | 바이너리 명명 | 패키지 명명 | 검증 우선순위 |
|---|-------------|------|--------|-------|---------------|---------------|-------------|---------------|
| 1 | macOS Intel | darwin | amd64 | - | Intel Mac (2020 이전) | `xflowd-darwin-amd64` | `xflow-{ver}-darwin-amd64.tar.gz` | P0 |
| 2 | macOS Apple Silicon | darwin | arm64 | - | M1/M2/M3/M4 Mac | `xflowd-darwin-arm64` | `xflow-{ver}-darwin-arm64.tar.gz` | P0 |
| 3 | Linux x86_64 | linux | amd64 | - | 일반 서버, 데스크톱 | `xflowd-linux-amd64` | `xflow-{ver}-linux-amd64.tar.gz` | P0 |
| 4 | Linux ARM64 | linux | arm64 | - | RPi 4/5 (64비트), AWS Graviton | `xflowd-linux-arm64` | `xflow-{ver}-linux-arm64.tar.gz` | P0 |
| 5 | Linux ARMv7 | linux | arm | 7 | RPi 2/3, RPi 4 (32비트) | `xflowd-linux-arm-v7` | `xflow-{ver}-linux-arm-v7.tar.gz` | P1 |
| 6 | Linux ARMv6 | linux | arm | 6 | RPi Zero/1/Zero 2W | `xflowd-linux-arm-v6` | `xflow-{ver}-linux-arm-v6.tar.gz` | P1 |

- P0: v1 릴리스 필수, 출시 전 수동 스모크 테스트 권장
- P1: v1 릴리스에 포함되나 CI 자동 검증 후 별도 수동 검증 선택 사항

---

## 8. 이해관계자 (Stakeholders)

| 역할 | 관심사 | 영향 |
|------|--------|------|
| 프로젝트 메인테이너 (xtra) | 태그 푸시 후 릴리스 자동화, 품질 게이트 적용 | 워크플로우 승인 및 시크릿 관리 |
| 컨트리뷰터 | PR 피드백 속도, CI 통과 조건 명확성 | ci.yml의 실행 시간 및 실패 메시지 품질 |
| 최종 사용자 (macOS/Linux) | 6개 플랫폼 바이너리 제공, 검증 가능한 체크섬 | Release 페이지, 체크섬 파일 |
| RPi 사용자 | ARMv6/v7/arm64 각 세대별 패키지 제공 | install.sh 호환성, 패키지 구성 |
| Docker 사용자 | ghcr.io 멀티아키 이미지 제공 | docker pull 호환성 |
| 보안 담당자 | 취약점 탐지 차단, Dependabot 커버리지 | govulncheck, Dependabot 설정 |

---

## 9. 인수 기준 요약 (Acceptance Criteria Summary)

상세한 인수 시나리오는 `acceptance.md`를 참조한다. 주요 인수 기준은 다음과 같다:

- **AC1**: PR 개설 시 ci.yml이 합리적 시간(30분 이내) 내에 모든 게이트 green 달성
- **AC2**: `v0.1.0` 태그 푸시 시 6개 `tar.gz` + `checksums.txt`를 포함한 GitHub Release 발행
- **AC3**: 각 `tar.gz`에 xflowd, xflow, xflow-agent 3개 바이너리 + web/dist + deploy 설정 파일 포함
- **AC4**: `shasum -a 256 -c checksums.txt`로 체크섬이 모두 OK 판정
- **AC5**: `ghcr.io/{owner}/xflow:{version}`이 amd64 및 arm64 호스트 양쪽에서 `docker pull` 성공
- **AC6**: Dependabot이 신규 Go 모듈 마이너 릴리스 감지 후 1주 이내 PR 생성
- **AC7**: 알려진 취약점 존재 시 `govulncheck`가 CI를 차단
- **AC8**: `go test` 실패 시 release 워크플로우가 아티팩트를 생성하지 않음
- **AC9**: 수동 `workflow_dispatch`가 태그 생성 없이 프리릴리스 빌드 생성
- **AC10**: macOS에서 Gatekeeper 우회 후 xflowd가 정상 실행되고 web/dist를 HTTP로 서빙

---

## 10. 성공 지표 (Success Metrics)

| 지표 | 목표값 | 측정 방법 |
|------|--------|-----------|
| CI 평균 실행 시간 | < 15분 | GitHub Actions 런 타임 |
| 릴리스 워크플로우 실행 시간 | < 45분 | 태그 푸시부터 릴리스 발행까지 |
| 6개 플랫폼 빌드 성공률 | 100% | 매 릴리스별 성공 Job 비율 |
| 캐시 적중률 (Go 모듈) | > 80% | actions/cache 통계 |
| Dependabot PR 처리 지연 | < 7일 | PR 생성부터 머지/클로즈까지 |
| govulncheck 차단 사례 | 0건 발생 시 주간 리뷰 | CI 로그 |
| 릴리스 아티팩트 다운로드 성공률 | 100% (체크섬 검증 기준) | 사용자 리포트 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-21*
*작성: MoAI SPEC Builder (manager-spec)*
