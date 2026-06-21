---
id: SPEC-UPDATE-001
title: xflowd 애플리케이션 바이너리 자동 업데이트 메커니즘 — 구현 계획
version: 0.1.0
status: draft
created: 2026-05-05
updated: 2026-05-05
author: xtra
priority: high
---

# SPEC-UPDATE-001: 구현 계획 (Implementation Plan)

## v0.1.0 Scope Note

본 계획서는 xflowd 자가 교체 업데이트 메커니즘의 최초 구현(v0.1.0)을 다룬다. 핵심 도입:

- 신규 패키지 `internal/updater/` (10여 개 파일, 의존성 없는 독립 모듈)
- GitHub Releases 기본 채널 + 채널 선택(stable/beta/nightly)
- SHA256 + Ed25519 검증 (공개키 핀닝)
- 원자적 바이너리 교체 (`github.com/inconshreveable/go-update` 활용)
- 그레이스풀 재시작 (drain timeout + `syscall.Exec` 또는 systemd notify)
- 자동 롤백 (health check 실패 감지)
- CLI 명령 5종 + REST API 엔드포인트 5종
- 다운그레이드 방지 (`--force` 명시)
- 설정 섹션 신규 + 운영자 마이그레이션 가이드

기존 SPEC(STORE, AGENT, CLI 등)의 동작에는 영향이 없으며, `cmd/xflowd/main.go`에 update 서브커맨드 등록과 lifecycle hook 추가만 발생한다.

## v0.2.0 Scope Note (릴리스 도구 + 운영 요구사항, M15~M18)

v0.2.0 은 v0.1.0 자가 업데이트 메커니즘 위에 **릴리스 이미지 생성·서명 도구**와 **원격 업데이트 운영 요구사항**을 추가한다(SPEC-REMOTE-001 그룹 O 가 무변경 소비). 백엔드 완료(2026-06-21):

- **(M15) 릴리스 도구**: `cmd/xflowd/update_image.go`(`update keygen`/`update sign` — Ed25519 키쌍·바이너리 본문 서명), `Makefile`(`keygen`·`release-images VERSION SIGN_KEY`·`release-images-guard` — 6 타깃 교차컴파일+서명+`checksum.txt`), `.github/workflows/release.yml`(`release-images` 잡 — 시크릿 `XFLOW_RELEASE_PRIVATE_KEY` 주입·서명 이미지 GitHub Release 첨부).
- **(M16) 노드 설정 요구사항**: `update.public_key_path` 원격 업데이트 필수(내장 핀닝 키 없음 — 미설정 거부), `update.insecure_skip_verify` 를 원격 다운로드(Checker/Downloader)에 연결(Ed25519 검증 유지), 채널 불일치/자산 누락 시 구체적 진단. 공개키는 노드 로컬 신뢰 앵커(서버 비전송 — REQ-O09).
- **(M17) 배포**: systemd `ReadWritePaths=/opt/xflow`(설치 디렉토리 전체 — 바이너리 교체 + `.previous` 백업). `…/data` 만 허용 시 `read-only file system` 실패.
- **(M18) 버전 stamp**: LDFLAGS `-X main.Version=$(VERSION)`(대문자) — 노드 보고 버전·버전 표시·이력 갱신의 전제(대소문자 불일치 시 `dev` 폴백).

비공개키는 릴리스 담당자(`make keygen`) 또는 CI 시크릿에만 존재하고, 공개키만 노드로 배포된다. 서버는 사전 서명된 `.sig` 만 저장·배포하며 서명을 생성하지 않는다.

## 기술 스택 (Technical Stack)

### 언어 및 런타임

- **Go 1.22+** (기존 프로젝트 표준)

### 외부 라이브러리 (신규)

- **`github.com/inconshreveable/go-update`** (v0.0.0-latest) — atomic replacement + rollback helper
  - 단일 책임, 광범위 채택, Linux/macOS/Windows 호환
  - 핵심 로직만 사용하고 나머지는 자체 구현 (의존성 최소화)

### Go stdlib 활용

- `crypto/ed25519` — 디지털 서명 검증
- `crypto/sha256` — 체크섬 계산
- `crypto/tls` — HTTPS 강제 + 인증서 검증
- `encoding/json` — GitHub Releases API + 응답 직렬화
- `log/slog` — 구조화 로깅 (기존 패턴)
- `net/http` — 채널 fetch + 다운로드
- `os` / `os/exec` / `syscall` — 파일 조작 + Exec
- `path/filepath` — 임시 디렉토리 + 백업 경로
- `time` — 타임스탬프 + duration 검증

### 빌드 변수

빌드 시 임베드되는 신규 변수:

```go
// cmd/xflowd/main.go (확장)
var (
    Version    = "dev"           // 기존
    Commit     = "unknown"       // 기존
    BuildDate  = "unknown"       // 기존
    UpdaterPublicKey = ""         // 신규: PEM-encoded Ed25519 public key
    UpdaterChannelURL = "https://api.github.com/repos/xtra72/xflow/releases"  // 신규
)
```

`UpdaterPublicKey`는 빌드 시 `-ldflags "-X main.UpdaterPublicKey=$(cat keys/public.pem | base64)"` 형식으로 주입.

## 영향 범위 (Impact Scope)

### 신규 파일 (디렉토리: `internal/updater/`)

- `internal/updater/types.go` — `Update`, `Version`, `Channel`, `Manifest`, `Status` enum
- `internal/updater/checker.go` — GitHub Releases API + 버전 비교
- `internal/updater/downloader.go` — HTTPS 다운로드 + 진행률 콜백
- `internal/updater/verifier.go` — SHA256 + Ed25519 검증
- `internal/updater/applier.go` — atomic replacement (go-update wrapper)
- `internal/updater/restarter.go` — graceful shutdown + syscall.Exec
- `internal/updater/rollback.go` — 백업 + 복구 로직
- `internal/updater/scheduler.go` — 주기적 확인 (cron-like)
- `internal/updater/manager.go` — 위 컴포넌트 조립 + 동시성 제어 (mutex/lock)
- `internal/updater/errors.go` — 9종 sentinel error
- `internal/updater/keys.go` — 공개키 임베드 + 파싱
- 테스트: 위 각 파일 대응 `_test.go` + `integration_test.go` (mock GitHub server)

### 신규 파일 (CLI / API)

- `cmd/xflowd/update.go` — `update` 서브커맨드 (5종)
- `internal/api/handler/system_update.go` — REST 핸들러 5종
- `internal/api/dto/update.go` — 요청/응답 DTO

### 신규 파일 (설정)

- `internal/config/update.go` — `UpdateConfig` 구조체 + yaml 파싱 + 검증

### 수정 파일

- `cmd/xflowd/main.go` — `update` 서브커맨드 등록 + lifecycle hook (graceful drain 통합)
- `internal/config/config.go` — `Config.Update *UpdateConfig` 필드 추가
- `internal/api/router.go` (또는 동등) — `/api/v1/system/version` + `/system/update/*` 등록
- `go.mod` / `go.sum` — `github.com/inconshreveable/go-update` 의존성

### 신규 문서

- `docs/updater-design.md` — 운영자 가이드 (채널 설정, 키 관리, 롤백 시나리오)
- `docs/security/key-rotation.md` (선택, 미래) — 공개키 회전 정책
- CHANGELOG.md 갱신

### 빌드 시스템 변경

- `Makefile` 또는 `scripts/build.sh` — `UpdaterPublicKey` ldflags 주입
- GitHub Actions workflow — release 시 자동 서명 + checksum.txt + signature.bin 생성

## 기술적 접근 (Technical Approach)

### 1. updater 패키지 구조 (의존성 없음)

```
internal/updater/
├── types.go          # 데이터 모델: Version, Channel, Manifest, Status
├── errors.go         # sentinel errors: ErrUpdateChecksumMismatch 등
├── keys.go           # 공개키 임베드/파싱 (Ed25519)
├── checker.go        # Channel.LatestRelease() → Manifest
├── downloader.go     # Manifest → 임시 디렉토리에 다운로드
├── verifier.go       # SHA256 + Ed25519 검증
├── applier.go        # atomic replacement (go-update)
├── rollback.go       # 백업/복구
├── restarter.go      # graceful shutdown + Exec
├── scheduler.go      # 주기 확인 (interval)
├── manager.go        # 위 컴포넌트 조립; Manager 인터페이스
└── *_test.go         # 단위 + 통합 테스트
```

핵심 인터페이스:

```go
// internal/updater/manager.go
type Manager interface {
    Check(ctx context.Context) (*CheckResult, error)
    Apply(ctx context.Context, opts ApplyOptions) (*Operation, error)
    Status() *Operation
    Rollback(ctx context.Context) error
    SetChannel(ch Channel) error
}

type Operation struct {
    ID              string
    Status          Status
    CurrentVersion  string
    TargetVersion   string
    Progress        int           // 0-100
    StartedAt       time.Time
    CompletedAt     *time.Time
    Error           error
}

type Status int
const (
    StatusIdle Status = iota
    StatusDownloading
    StatusVerifying
    StatusApplying
    StatusRestarting
    StatusRollingBack
    StatusSuccess
    StatusFailed
)
```

### 2. GitHub Releases 채널 fetch

```go
// internal/updater/checker.go
func (c *Channel) LatestRelease(ctx context.Context) (*Manifest, error) {
    var path string
    switch c.Name {
    case ChannelStable:
        path = "/releases/latest"
    case ChannelBeta, ChannelNightly:
        path = "/releases" // 전체 목록 → prerelease 필터
    }
    url := c.BaseURL + path
    // HTTPS 강제 검사
    if !strings.HasPrefix(url, "https://") {
        return nil, ErrUpdateChannelInvalid
    }
    // GET, JSON 디코딩, asset 매칭 (OS/arch)
    // ...
}
```

asset 매칭 패턴: `xflowd-{os}-{arch}` (예: `xflowd-linux-amd64`).

### 3. Ed25519 서명 검증

```go
// internal/updater/verifier.go
func VerifySignature(binary []byte, signature []byte, publicKeyPEM []byte) error {
    block, _ := pem.Decode(publicKeyPEM)
    if block == nil {
        return fmt.Errorf("invalid public key PEM")
    }
    pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return err
    }
    edPub, ok := pubKey.(ed25519.PublicKey)
    if !ok {
        return fmt.Errorf("not an Ed25519 public key")
    }
    if !ed25519.Verify(edPub, binary, signature) {
        return ErrUpdateSignatureInvalid
    }
    return nil
}
```

공개키는 빌드 변수(`main.UpdaterPublicKey`)에서 base64 디코딩 후 사용. 설정의 `public_key_path`가 지정되면 그 파일을 우선 사용 (개발/테스트용).

### 4. 원자적 교체 (go-update 활용)

```go
// internal/updater/applier.go
import "github.com/inconshreveable/go-update"

func (a *Applier) Apply(newBinary io.Reader) error {
    // go-update가 다음을 atomic하게 수행:
    // 1. 임시 파일 생성 (같은 디렉토리)
    // 2. 새 바이너리 쓰기
    // 3. 현재 바이너리 → .previous로 rename
    // 4. 임시 파일 → 현재 위치로 rename (atomic)
    return update.Apply(newBinary, update.Options{
        TargetPath: a.targetPath,
        OldSavePath: a.targetPath + ".previous",  // 백업 보존
    })
}
```

go-update의 `update.Apply`는 내부적으로 POSIX rename을 사용. cross-device rename 회피를 위해 임시 디렉토리는 동일 마운트 포인트 내에 위치해야 함.

### 5. 그레이스풀 재시작 디자인

```go
// internal/updater/restarter.go
func (r *Restarter) Restart(ctx context.Context) error {
    // 1. 신규 요청 차단 (HTTP listener Shutdown)
    if err := r.httpServer.Shutdown(ctx); err != nil {
        slog.Warn("http shutdown error", "err", err)
    }

    // 2. in-flight FBP 메시지 drain (timeout)
    drainCtx, cancel := context.WithTimeout(ctx, r.drainTimeout)
    defer cancel()
    lost := r.engine.Drain(drainCtx)
    if lost > 0 {
        slog.Warn("drain timeout: in-flight messages lost", "count", lost)
    }

    // 3. 영속화 flush
    if err := r.persistence.Flush(); err != nil {
        slog.Error("persistence flush failed", "err", err)
    }

    // 4. systemd notify 또는 syscall.Exec
    if isSystemdNotify() {
        // SIGTERM → systemd가 재시작
        return r.signalSelf(syscall.SIGTERM)
    }
    // syscall.Exec: 동일 PID로 새 바이너리 전환
    return syscall.Exec(r.targetPath, os.Args, os.Environ())
}
```

### 6. 롤백 디자인

```go
// internal/updater/rollback.go
type Rollback struct {
    targetPath string
    backupPath string  // <target>.previous
    healthCheck func(context.Context) error
}

func (r *Rollback) AutoRollbackIfNeeded(ctx context.Context, healthCheckTimeout time.Duration) error {
    hcCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
    defer cancel()
    if err := r.healthCheck(hcCtx); err == nil {
        return nil // health OK, 롤백 불필요
    }
    return r.Restore()
}

func (r *Rollback) Restore() error {
    if _, err := os.Stat(r.backupPath); err != nil {
        return ErrUpdateRollbackFailed
    }
    // atomic restore: backup → target
    return os.Rename(r.backupPath, r.targetPath)
}
```

자동 롤백은 워치독 프로세스 또는 supervisor의 health check 실패 핸들러에서 트리거.
**대안**: 본 SPEC v0.1.0은 단순화를 위해 새 프로세스 자체가 시작 시 health check를 self-monitor하고, 실패 시 백업 파일로 self-revert (단, 백업 파일이 보존되어 있다는 가정).

### 7. 동시성 제어

`Manager` 구조체에 `sync.Mutex`로 update apply 직렬화. 동시에 두 호출이 들어오면 두 번째는 즉시 `ErrUpdateInProgress` (HTTP 409).

### 8. 설정 검증

```go
// internal/config/update.go
type UpdateConfig struct {
    Enabled              bool          `yaml:"enabled"`
    Channel              string        `yaml:"channel"`           // stable|beta|nightly
    CheckInterval        time.Duration `yaml:"check_interval"`
    AutoApply            bool          `yaml:"auto_apply"`
    NotifyOnly           bool          `yaml:"notify_only"`
    UpdateURL            string        `yaml:"update_url"`
    PublicKeyPath        string        `yaml:"public_key_path"`
    DrainTimeout         time.Duration `yaml:"drain_timeout"`
    HealthCheckTimeout   time.Duration `yaml:"health_check_timeout"`
    InsecureSkipVerify   bool          `yaml:"insecure_skip_verify"`
}

func (c *UpdateConfig) Validate() error {
    if c.AutoApply && c.NotifyOnly {
        return errors.New("auto_apply and notify_only are mutually exclusive")
    }
    if c.UpdateURL != "" && !strings.HasPrefix(c.UpdateURL, "https://") {
        return ErrUpdateChannelInvalid
    }
    switch c.Channel {
    case "", "stable", "beta", "nightly":
    default:
        return fmt.Errorf("invalid channel: %s", c.Channel)
    }
    // ... drain_timeout > 0, health_check_timeout > 0 등
    return nil
}
```

### 9. CLI 통합

```go
// cmd/xflowd/update.go
func newUpdateCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "update", Short: "Manage xflowd self-update"}
    cmd.AddCommand(newUpdateCheckCmd())
    cmd.AddCommand(newUpdateApplyCmd())
    cmd.AddCommand(newUpdateStatusCmd())
    cmd.AddCommand(newUpdateRollbackCmd())
    cmd.AddCommand(newUpdateChannelCmd())
    return cmd
}
```

각 서브커맨드는 `--json` flag를 공통 지원.

### 10. REST API 통합

```go
// internal/api/handler/system_update.go
type SystemUpdateHandler struct {
    manager updater.Manager
}

func (h *SystemUpdateHandler) Check(c *api.Context) error    { /* ... */ }
func (h *SystemUpdateHandler) Apply(c *api.Context) error    { /* ... */ }
func (h *SystemUpdateHandler) Status(c *api.Context) error   { /* ... */ }
func (h *SystemUpdateHandler) Rollback(c *api.Context) error { /* ... */ }

// router 등록
router.GET("/api/v1/system/version", versionHandler.Get, requireAuth())
router.POST("/api/v1/system/update/check", h.Check, requireAuth(), requireUpdatePerm())
router.POST("/api/v1/system/update/apply", h.Apply, requireAuth(), requireUpdatePerm())
router.GET("/api/v1/system/update/status", h.Status, requireAuth())
router.POST("/api/v1/system/update/rollback", h.Rollback, requireAuth(), requireUpdatePerm())
```

`requireUpdatePerm()` 미들웨어는 기존 인가 시스템(SPEC-AUTH-001/002) 활용.

## 마일스톤 (Milestones)

### Primary Goal: 코어 모듈 + 검증

- `internal/updater/` 패키지 코어 구현 (types, checker, downloader, verifier, applier)
- Ed25519 서명 검증 + 공개키 핀닝
- atomic replacement + 백업
- 단위 테스트 90% 이상 커버리지 (특히 verifier, rollback)
- `xflowd update check`, `xflowd update apply` CLI 동작

### Secondary Goal: 운영 통합

- graceful restart (drain + syscall.Exec / systemd notify)
- 자동 롤백 (health check 실패 감지)
- REST API 엔드포인트 5종
- 다운그레이드 방지 (`--force`)
- 설정 섹션 + reload 통합
- 통합 테스트 (mock GitHub server)
- 구조화 로깅 + 메트릭

### Final Goal: 폴리싱 + 문서

- 주기적 자동 확인 scheduler
- `xflowd update status / rollback / channel` 부가 명령
- 운영자 가이드 (`docs/updater-design.md`)
- CHANGELOG + 빌드 시스템 통합 (서명 + checksum 자동화)
- (분리) Web UI 통합은 SPEC-WEB-005 v0.6.0에서 다룸

## Task Decomposition (18 작업)

1. **신규 의존성 추가** — `go.mod`에 `github.com/inconshreveable/go-update` 추가
2. **`internal/updater/types.go`** — 데이터 모델 (Version, Channel, Manifest, Status, Operation)
3. **`internal/updater/errors.go`** — 9종 sentinel error 선언
4. **`internal/updater/keys.go`** — 공개키 PEM 파싱 + 빌드 변수 통합
5. **`internal/updater/checker.go`** — GitHub Releases API 호출 + 버전 비교 (semver) + asset 매칭
6. **`internal/updater/downloader.go`** — HTTPS 다운로드 + 진행률 콜백 + 디스크 공간 사전 검사
7. **`internal/updater/verifier.go`** — SHA256 + Ed25519 검증
8. **`internal/updater/applier.go`** — go-update wrapper + 백업 보존
9. **`internal/updater/rollback.go`** — Restore 로직 + 자동 트리거 (health check fail)
10. **`internal/updater/restarter.go`** — drain + syscall.Exec / systemd notify
11. **`internal/updater/scheduler.go`** — 주기적 check (configurable interval)
12. **`internal/updater/manager.go`** — Manager 인터페이스 조립 + sync.Mutex 동시성 제어
13. **`internal/config/update.go`** — UpdateConfig 구조체 + yaml 파싱 + Validate()
14. **`cmd/xflowd/update.go`** — CLI 서브커맨드 5종 (cobra 기반, --json 공통 flag)
15. **`internal/api/handler/system_update.go` + `dto/update.go`** — REST API 5종 핸들러 + DTO
16. **테스트: 단위** — verifier (100% 분기), checker, downloader, applier, rollback, scheduler — `go test -race ./internal/updater/...` 통과
17. **테스트: 통합** — mock GitHub Releases server (httptest) + end-to-end check/apply/rollback 흐름
18. **문서: `docs/updater-design.md` + CHANGELOG + Makefile (서명 자동화 ldflags)**

## 아키텍처 영향 (Architecture Impact)

### 레이어 영향

- **신규 모듈 레이어** (`internal/updater/`): 의존성 없는 self-contained 패키지. xflowd, xflow-agent, xflow CLI 모두에서 재사용 가능하도록 설계.
- **설정 레이어** (`internal/config/`): `UpdateConfig` 추가 + Validate() 로직.
- **CLI 레이어** (`cmd/xflowd/`): `update` 서브커맨드 등록만, 기존 명령에 영향 없음.
- **API 레이어** (`internal/api/handler/`): 신규 5개 엔드포인트, 기존 엔드포인트 영향 없음.
- **Lifecycle 레이어**: 기존 graceful shutdown 로직(SPEC-LIFE-001)과 통합. drain 함수 호출 추가.

### 의존성 그래프

```
cmd/xflowd/main.go
    ├── cmd/xflowd/update.go         (신규)
    │   └── internal/updater         (신규)
    │       └── github.com/inconshreveable/go-update  (신규 외부)
    ├── internal/api/handler/system_update.go  (신규)
    │   └── internal/updater
    └── internal/config (수정)
        └── internal/config/update.go  (신규)
```

### 호환성

- 기존 yaml 설정에 `update` 섹션이 없으면 → `UpdateConfig{Enabled: false}` 기본값 적용 → 자동 동작 0건 (운영 영향 없음)
- 기존 CLI 명령 (`xflowd run`, `xflowd config` 등) → 변경 없음
- 기존 REST API → 변경 없음 (신규 엔드포인트만 추가)
- 빌드 → `UpdaterPublicKey` ldflags가 비어 있으면 → `crypto/ed25519` 검증은 fail-closed (서명 검증 강제 실패) → 자동 업데이트는 동작하지 않으나 다른 기능 영향 없음. 운영자가 자체 빌드 시 명시적으로 키 주입 필요.

## 리스크 및 완화 (Risks & Mitigation)

| 리스크 | 심각도 | 발생 가능성 | 완화 전략 |
|--------|--------|-------------|-----------|
| 새 버전 부팅 실패 → 운영 중단 | High | Medium | M7 자동 롤백 + health check timeout (5초) + 운영자 알림 (critical log) |
| 중간자 공격(MITM) → 악성 바이너리 적용 | Critical | Low | M13 HTTPS 전용 + 공개키 핀닝 + Ed25519 서명 검증 + TLS 인증서 검증 |
| 디스크 여유 부족 → 다운로드 실패 후 시스템 불안정 | Medium | Low | M3 다운로드 전 사전 디스크 검사 + 명시적 `ErrUpdateInsufficientDiskSpace` |
| 다운그레이드 실수 → 데이터 손실 / 호환성 문제 | High | Medium | M8 `--force` 없이 자동 거부 + 명시적 경고 + 자동 적용은 다운그레이드 절대 금지 |
| graceful drain timeout → in-flight 메시지 유실 | Medium | Medium | M6 drain timeout 설정 가능 + 손실 카운트 명시 로그 + (미래) supervisor handoff |
| systemd / launchd supervisor와 충돌 | Medium | Medium | M6 systemd notify 모드 자동 감지 + Type=notify 호환 |
| 공개키 손상/회전 필요 시 운영 부담 | Medium | Low | 공개키는 빌드 변수로 임베드 → 새 빌드 + 새 키 주입으로 회전. 미래에 키 회전 자동화 SPEC. |
| GitHub Releases API rate limit | Low | Medium | Caching (최근 check 결과 보존) + check_interval 기본 24시간 |
| Cross-device rename 실패 (`/tmp`가 다른 마운트) | Medium | Low | M5 임시 디렉토리는 바이너리와 같은 디렉토리 사용 (fallback) |
| go-update 라이브러리 정체 / 미래 유지보수 | Low | Low | wrapper 격리로 미래 직접 구현 교체 가능 |
| Ed25519 키 비공개키 유출 | Critical | Low | GitHub Actions secrets + key rotation 정책 (별도 SPEC, 미래) |
| `syscall.Exec` 후 in-flight 소켓 인계 실패 | Medium | Medium | systemd Type=notify 모드에서는 supervisor가 재시작 → 자연스럽게 회피 |
| 동시 update apply 호출 → race condition | Medium | Low | M10 sync.Mutex로 직렬화 + HTTP 409 응답 |

## 테스트 전략 (Test Strategy)

### 개발 방법론: Hybrid (TDD 우선 for 신규 모듈)

본 SPEC은 신규 패키지 `internal/updater/`를 도입하므로 **TDD 우선** 적용:

- **TDD**: `internal/updater/` 모든 파일 (RED-GREEN-REFACTOR)
- **TDD**: `cmd/xflowd/update.go`, `internal/api/handler/system_update.go` (신규)
- **DDD characterization**: 기존 `cmd/xflowd/main.go` graceful shutdown 변경 시 회귀 방지

### 단위 테스트

| 모듈 | 커버리지 목표 | 핵심 케이스 |
|------|---------------|-------------|
| `verifier.go` | **100%** (보안 critical) | 정상 검증, SHA 불일치, Ed25519 위조, 손상 PEM, 빈 입력 |
| `checker.go` | 90%+ | latest fetch, beta/nightly 필터, asset 매칭, 네트워크 오류, malformed JSON |
| `downloader.go` | 90%+ | 정상 다운로드, 디스크 부족, 네트워크 중단, 진행률 콜백 |
| `applier.go` | 85%+ | atomic rename 성공/실패, 백업 보존 |
| `rollback.go` | 90%+ | Restore 성공, 백업 파일 없음, health check fail/pass |
| `restarter.go` | 80%+ | drain timeout 정상/초과, systemd notify 모드 분기 |
| `scheduler.go` | 85%+ | interval 0 (비활성), 주기 호출, 캐시 fallback |
| `manager.go` | 85%+ | 동시 호출 → ErrUpdateInProgress, 정상 흐름 |
| `config/update.go` | 90%+ | yaml 파싱, Validate (auto_apply + notify_only 충돌 등) |
| `cmd/xflowd/update.go` | 85%+ | exit code, --json 출력, --force 동작 |
| `handler/system_update.go` | 85%+ | 인증/인가, 동시 호출 409, 응답 형식 |

### 통합 테스트

- **mock GitHub Releases server** (`httptest.Server`):
  - 정상 latest 응답
  - prerelease beta 응답
  - 404 not found
  - malformed JSON
  - 다운로드 asset 정상 / checksum mismatch / signature invalid
  - HTTP → HTTPS 거부 검증
- **end-to-end 흐름**: check → apply → restart 시뮬레이션 (실제 syscall.Exec는 mock)
- **rollback 시나리오**: health check fail → backup restore
- **다운그레이드 거부**: `--force` 없으면 거부, 있으면 허용

### Race condition 검증

- `go test -race ./...` 모든 패키지 통과 필수
- `manager.go`의 동시 Apply 호출 → mutex 직렬화 검증
- `scheduler.go`의 정지/재시작 race 검증

### Fuzz 테스트 (선택)

- `verifier.go` Ed25519 입력 fuzz (corruption tolerance)
- `checker.go` GitHub API 응답 JSON fuzz (malformed input)

### 보안 테스트

- `verifier.go`: 알려진 잘못된 서명/체크섬 입력 → 모두 reject
- `applier.go`: 검증 전 실행 권한 부여 안 함 (`0600` 유지) 검증
- TLS: 자가 서명 인증서 → reject (insecure_skip_verify=false)
- HTTP 스킴 → reject

### 커버리지 목표

- `internal/updater/`: **90%+ 평균** (verifier 100%)
- `cmd/xflowd/update.go`: 85%+
- `internal/api/handler/system_update.go`: 85%+
- 전체 SPEC 신규 코드: 87%+ 평균

## 구현 순서 (Implementation Order)

권장 작업 순서 (의존성 고려):

1. Tasks 2, 3, 4 (types/errors/keys) — 다른 모듈의 토대
2. Tasks 5, 6, 7 (checker/downloader/verifier) — TDD로 코어 검증 로직 우선 작성
3. Task 8 (applier) — go-update 통합
4. Task 9 (rollback) — applier 결과 활용
5. Task 10 (restarter) — drain 통합
6. Task 12 (manager) — 위 컴포넌트 조립
7. Task 13 (config) — manager가 사용
8. Task 14 (CLI) — manager 인터페이스 호출
9. Task 15 (REST API) — manager 인터페이스 호출 (병렬 가능)
10. Task 11 (scheduler) — manager 활용
11. Task 16 (단위 테스트) — 각 모듈 작성과 동시 진행 (TDD)
12. Task 17 (통합 테스트) — 모든 모듈 완성 후
13. Task 18 (문서) — 마지막

Tasks 14, 15는 의존성 없이 병렬 작성 가능 (manager 인터페이스만 안정되면).
