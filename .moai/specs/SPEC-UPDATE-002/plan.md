---
id: SPEC-UPDATE-002
title: xflowd 자동 업데이트 v0.2.0 — 구현 계획
version: 0.1.0
status: draft
created: 2026-05-06
updated: 2026-05-06
author: xtra
priority: medium
---

# SPEC-UPDATE-002: 구현 계획 (Implementation Plan)

## v0.2.0 Scope Note

본 계획서는 SPEC-UPDATE-001 v0.1.0 의 알려진 3가지 한계를 통합 해결하는 v0.2.0 구현을
다룬다. 핵심 도입:

- **M-1 In-Process Restart**: `restart_orchestrator.go` 신규 (drain → exec → self health
  check → 자동 rollback wiring)
- **M-2 채널 REST API**: `system_update_channel.go` 신규 핸들러 (GET/PUT
  `/system/update/channel`) + Web UI 의 `ChannelChangeDialog` 신규 컴포넌트
- **M-3 멀티 바이너리**: `manifest.json` 형식 + Checker 의 의존성 매트릭스 검증 + target 옵션
  (xflowd | xflow | xflow-agent)
- 신규 sentinel error 3종 (`ErrUpdateRestartFailed`, `ErrUpdateHealthCheckFailed`,
  `ErrUpdateIncompatibleVersion`)
- OperationStatus 9-state → 11-state (`restarting`, `health_checking` 추가)
- v0.1.0 backward 호환 (default 값 보존)

기존 SPEC (특히 SPEC-UPDATE-001 v0.1.0) 의 동작은 default 값으로 그대로 유지되며, 신규 옵션
(`auto_restart=true`, `target` 명시) 사용 시에만 v0.2.0 동작이 활성화된다.

## 기술 스택 (Technical Stack)

### 언어 및 런타임

- **Go 1.22+** (기존 프로젝트 표준)
- **TypeScript 5.9+** (Web UI, 기존 SPEC-WEB-006 스택)
- **React 19** (기존 SPEC-WEB-006 스택)

### 외부 라이브러리 (신규: 0)

- **신규 외부 의존성 없음** (모두 v0.1.0 의 internal/updater + Go stdlib + 기존 web 스택 활용)

### Go stdlib 활용 (v0.1.0 + 추가)

- `crypto/ed25519` — 서명 검증 (v0.1.0 그대로)
- `crypto/sha256` — 체크섬 (v0.1.0 그대로)
- `encoding/json` — manifest.json 파싱 (M-3 신규 사용)
- `golang.org/x/mod/semver` 또는 `github.com/Masterminds/semver` — semver 비교 (M-3 신규)
- `log/slog` — 구조화 로깅 (v0.1.0 그대로)
- `net/http` — channel REST + manifest 다운로드 (v0.1.0 그대로)
- `os`/`syscall` — graceful drain + Exec (v0.1.0 그대로)

semver 라이브러리 결정: **`golang.org/x/mod/semver`** (stdlib 준-stdlib, Google 유지보수,
신규 외부 의존성 아님). 대안 `github.com/Masterminds/semver` 는 더 풍부한 API 지만 외부
의존성 추가 부담.

### Web 스택 활용 (SPEC-WEB-006 그대로)

- TanStack Query (`@tanstack/react-query`) — channel hook
- Tailwind CSS — 스타일링
- Vitest + Testing Library — 테스트
- Headless UI 또는 SPEC-WEB-006 의 기존 Dialog 컴포넌트 — ChannelChangeDialog

## 영향 범위 (Impact Scope)

### Backend 신규 파일 (3개)

- `internal/updater/restart_orchestrator.go` — M-1 wiring (drain → exec → self health check →
  auto rollback). v0.1.0 의 `restarter.go`, `healthcheck.go`, `rollback.go` 를 조합.
- `internal/updater/manifest.go` — manifest.json 형식 + 다운로드 + semver 비교 검증
- `internal/api/handler/system_update_channel.go` — GET/PUT `/system/update/channel` 핸들러

### Backend 수정 파일 (8개)

- `internal/updater/errors.go` — 신규 3종 sentinel error 추가
- `internal/updater/types.go` — `OperationStatus` 에 `restarting`, `health_checking` 추가;
  `ApplyOptions` 에 `AutoRestart bool`, `Target string` 추가
- `internal/updater/manager.go` — Apply() 에 AutoRestart / Target 분기, restart_orchestrator
  호출
- `internal/updater/checker.go` — manifest.json 다운로드 + 의존성 검증 통합
- `internal/api/handler/system_update.go` — 요청 DTO 에 `auto_restart`, `target` 필드 추가
- `internal/api/dto/update.go` — `ApplyRequest` 확장; 신규 `ChannelGetResponse`,
  `ChannelPutRequest`, `ChannelPutResponse` DTO
- `internal/api/router.go` — `/system/update/channel` GET/PUT 라우트 등록
- `internal/config/update.go` — `health_check_interval`, `health_check_endpoint`,
  `target_binaries` 필드 추가; `health_check_timeout` default `5s` → `30s`
- `cmd/xflowd/update.go` — `--auto-restart`, `--target` flag 추가

### Frontend 신규 파일 (1개)

- `web/src/components/system/ChannelChangeDialog.tsx` — 채널 변경 확인 dialog (M-8)

### Frontend 수정 파일 (2개)

- `web/src/components/system/SystemVersionCard.tsx` — 채널 표시를 dropdown 으로 활성화
- `web/src/lib/api/systemUpdate.ts` — `useChannelGet`, `useChannelPut` hooks 추가
  (기존 5 hooks → 7 hooks)

### 테스트 신규 파일 (5개)

- `internal/updater/restart_orchestrator_test.go`
- `internal/updater/manifest_test.go`
- `internal/api/handler/system_update_channel_test.go`
- `web/src/components/system/__tests__/ChannelChangeDialog.test.tsx`
- `web/src/components/system/__tests__/SystemVersionCard.channel.test.tsx` (채널 dropdown
  특화 테스트)

### 문서 변경

- `docs/updater-design.md` — v0.2.0 섹션 추가 (auto_restart 흐름, manifest 형식,
  의존성 매트릭스 사용 가이드)
- `CHANGELOG.md` — v0.2.0 entry 추가

### 빌드 시스템 변경 (CI 가이드)

- GitHub Actions release workflow — `manifest.json` asset 자동 생성 (각 바이너리마다)
- 모든 바이너리 (xflowd, xflow, xflow-agent) 동일 키로 서명 + 단일 `signature.bin` (또는
  `signature-{binary}.bin`)
- 본 SPEC 은 CI 변경의 명세만 정의; 실제 workflow 변경은 별도 PR 에서 수행

## 기술적 접근 (Technical Approach)

### 1. M-1 In-Process Restart Wiring

핵심 컴포넌트 = 신규 `restart_orchestrator.go`. v0.1.0 에는 `restarter.go` (graceful drain +
exec) 와 `healthcheck.go` (WaitHealthy) 와 `rollback.go` (.previous 복원) 가 독립적으로 존재
하나, **순차 호출 + 실패 시 자동 rollback 흐름** 이 wiring 되어 있지 않다.

```go
// internal/updater/restart_orchestrator.go (개념적 시그니처)
type RestartOrchestrator interface {
    // Run executes the full automatic restart sequence:
    // 1. Graceful drain (HTTP server, FBP messages)
    // 2. syscall.Exec (or systemd SIGTERM)
    // 3. Self health check (after new process boot)
    // 4. On health check failure: trigger rollback + retry health check
    Run(ctx context.Context, op *Operation) error
}
```

핵심 로직 흐름:

1. `restarter.GracefulDrain(ctx, drainTimeout)` 호출 → drain 완료 또는 timeout
2. OperationStatus 업데이트: `applying` → `restarting`
3. `restarter.Exec(newBinaryPath)` 호출 → 새 프로세스로 전환 (이 시점에서 현재 프로세스 종료)
4. **신규 프로세스에서**: 부팅 직후 OperationStatus = `health_checking` 으로 진입
5. `healthcheck.WaitHealthy(ctx, healthCheckTimeout, interval, endpoint)` 호출
6. 성공 → `completed`, 실패 → `rollback.Restore()` + 두 번째 health check
7. 두 번째도 실패 → `failed` + `ErrUpdateRollbackFailed` (자동 update 일시 중단)

도전 과제: **프로세스 전환 후 OperationStatus 추적**. 새 프로세스는 기존 in-memory state 를
잃으므로, OperationStatus 는 disk persistence 가 필요하다. v0.1.0 의 Manager 는 in-memory
이므로, 본 SPEC 에서 `update_state.json` (또는 동등) 으로 disk 영속화 추가.

### 2. M-2 채널 REST API

GET 핸들러는 단순:

```go
func (h *SystemUpdateChannelHandler) Get(w http.ResponseWriter, r *http.Request) {
    current := h.Manager.GetChannel()
    available := []string{"stable", "beta", "nightly"}
    writeJSON(w, ChannelGetResponse{Current: current, Available: available})
}
```

PUT 핸들러는 admin 검증 + SetChannel + Check 조합:

```go
func (h *SystemUpdateChannelHandler) Put(w http.ResponseWriter, r *http.Request) {
    if !isAdmin(r) {
        writeError(w, http.StatusForbidden, "admin permission required")
        return
    }
    var req ChannelPutRequest
    decode(r, &req)
    if !isValidChannel(req.Channel) {
        writeError(w, http.StatusBadRequest, ErrUpdateChannelInvalid)
        return
    }
    previous := h.Manager.GetChannel()
    if err := h.Manager.SetChannel(req.Channel); err != nil { ... }
    checkResult, err := h.Manager.Check(r.Context())
    if err != nil { ... }
    writeJSON(w, ChannelPutResponse{
        Previous: previous, Current: req.Channel, CheckResult: checkResult,
    })
}
```

admin 검증은 v0.1.0 의 인증 미들웨어 활용. JWT 토큰의 `role: admin` 클레임 또는 SPEC-AUTH 의
패턴을 따른다.

### 3. M-2 Web UI: ChannelChangeDialog + SystemVersionCard 활성화

```tsx
// web/src/components/system/ChannelChangeDialog.tsx
export function ChannelChangeDialog({
  open,
  previous,
  next,
  onConfirm,
  onCancel,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onCancel}>
      <DialogContent>
        <DialogTitle>채널 변경</DialogTitle>
        <DialogDescription>
          채널을 <Badge>{previous}</Badge> 에서 <Badge>{next}</Badge> 로 변경합니다.
          다음 update check 부터 새 채널이 반영됩니다.
        </DialogDescription>
        <DialogFooter>
          <Button onClick={onCancel}>취소</Button>
          <Button onClick={onConfirm}>변경</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

```tsx
// web/src/components/system/SystemVersionCard.tsx (수정)
const channelMutation = useChannelPut();
const handleChannelChange = (newChannel: string) => {
  setPendingChannel(newChannel);
  setDialogOpen(true);
};
const handleConfirm = async () => {
  try {
    const result = await channelMutation.mutateAsync({ channel: pendingChannel });
    // result.check_result 로 즉시 update_available 갱신
    queryClient.setQueryData(['systemVersion'], result.check_result);
    setDialogOpen(false);
    toast.success(`채널이 ${result.current} 로 변경되었습니다`);
  } catch (err) {
    if (err.status === 403) {
      toast.error('채널 변경에는 admin 권한이 필요합니다');
    } else {
      toast.error('채널 변경 실패: ' + err.message);
    }
    // dropdown 을 이전 값으로 복원
    setSelectedChannel(currentChannel);
  }
};
```

### 4. M-3 멀티 바이너리: target 옵션 + manifest 검증

manifest.json 형식 (각 바이너리 release 별):

```json
{
  "version": "v0.4.0",
  "binary": "xflow-agent",
  "compat": {
    "xflowd": ">=v0.4.0",
    "xflow": ">=v0.3.0"
  },
  "checksum": "sha256:abc123...",
  "signature": "ed25519:..."
}
```

Checker 는 apply 시작 시 manifest.json 다운로드 → semver 검증:

```go
// internal/updater/manifest.go
type Manifest struct {
    Version string            `json:"version"`
    Binary  string            `json:"binary"`
    Compat  map[string]string `json:"compat"`
}

func (c *Checker) ValidateCompat(ctx context.Context, m *Manifest) error {
    for binary, constraint := range m.Compat {
        currentVer, err := c.queryBinaryVersion(binary)
        if err != nil { return err }
        if !semver.Satisfies(currentVer, constraint) {
            return fmt.Errorf("%w: %s %s (current %s, required %s)",
                ErrUpdateIncompatibleVersion, m.Binary, m.Version, currentVer, constraint)
        }
    }
    return nil
}
```

target=xflow (CLI) 의 경우 graceful drain SKIP, atomic replace 후 즉시 종료:

```go
// internal/updater/manager.go (개념적)
func (m *Manager) applyTarget(ctx context.Context, target string, opts ApplyOptions) error {
    // ... download + verify ...
    if err := m.applier.Apply(newBinary, currentPath); err != nil { return err }
    if target == "xflow" {
        // CLI 는 drain / restart 불필요
        m.setStatus(StatusCompleted)
        return nil
    }
    if opts.AutoRestart {
        return m.restartOrchestrator.Run(ctx, op)
    }
    m.setStatus(StatusReadyToRestart)
    return nil
}
```

### 5. v0.1.0 호환성 보존 전략

핵심 원칙: **default 값 = v0.1.0 동작**.

- `ApplyOptions.AutoRestart` = `false` default → v0.1.0 흐름 그대로
- `ApplyOptions.Target` = `"xflowd"` default → v0.1.0 흐름 그대로
- `health_check_timeout` 의 default 변경 (`5s` → `30s`) 은 v0.1.0 의 명시 설정값이 있으면
  그대로 사용, 없을 때만 새 default. v0.1.0 의 health check 자체가 wiring 되지 않았으므로
  실제 동작 영향 없음.
- 신규 OperationStatus 2종 (`restarting`, `health_checking`) 은 `auto_restart=true` 일 때만
  관찰됨. v0.1.0 호출자는 9-state 만 관찰 (변경 없음).
- 신규 sentinel error 3종은 추가만; 기존 9종은 그대로.

### 6. 동시성 제어

- Manager 의 `mutex` 로 동시 apply 호출 직렬화 (v0.1.0 패턴 그대로)
- target 이 다르더라도 동시 apply 거부 (Decision Point 3 = independent + 직렬화)
- 채널 변경 (PUT `/update/channel`) 은 apply 와 별도 lock; check 와는 동기적 sequential

### 7. 신규 disk state file (선택, M-1 위해 필요)

`<datadir>/update_state.json`:

```json
{
  "operation_id": "upd-2026-05-06-001",
  "target_version": "v0.4.0",
  "current_version": "v0.3.0",
  "status": "health_checking",
  "started_at": "2026-05-06T10:00:00Z",
  "auto_restart": true,
  "target": "xflowd"
}
```

새 프로세스 부팅 시 이 파일을 읽고 `health_checking` 단계 이어서 진행. 정상 완료 또는 실패 후
파일 삭제. 운영자가 수동 cleanup 도 가능.

### 8. 보안 고려

- `manifest.json` 도 SHA256 + Ed25519 서명 검증 대상 (v0.1.0 의 binary 검증과 동일 패턴)
- PUT `/update/channel` 의 admin 검증은 JWT 클레임 기반 (v0.1.0 인증 패턴)
- self-probe 는 localhost 만 (외부 IP 아님), 별도 인증 면제 (자가 호출)
- exec 권한 검증: 새 바이너리 권한 0755 확인 후 exec; 권한 부족 시 `ErrUpdateRestartFailed`

### 9. 로깅 보강

신규 구조화 로그 이벤트:

- `update.exec` (M-3): exec 호출 직전. 필드 = `current_version`, `target_version`,
  `drain_duration_ms`
- `update.health_check` (M-4): self-probe 시도. 필드 = `attempt`, `status_code`,
  `version_match`
- `update.auto_rollback` (M-5): 자동 rollback 트리거. 필드 = `reason`, `result`
- `update.channel_change` (M-7): 채널 변경 시도. 필드 = `previous`, `requested`, `result`,
  `actor`
- `update.compat_check` (M-11): 의존성 매트릭스 검증. 필드 = `target`, `compat`, `result`

기존 v0.1.0 이벤트 (`update.check`, `update.download`, ..., `update.rollback`) 는 그대로 유지.

## 영향 범위 표 (Impact Summary Table)

| 카테고리 | 신규 파일 | 수정 파일 | LOC 추정 |
|---------|----------|----------|---------|
| Backend (Go) | 3 | 8 | ~1,200 |
| Frontend (TS/TSX) | 1 | 2 | ~300 |
| 테스트 | 5 | (수정된 모듈에 대응 추가) | ~800 |
| 문서 | 0 | 2 | ~150 |
| **합계** | **9** | **12** | **~2,450** |

## Phases

### Phase A: M-1 In-Process Restart Core (TDD)

목표: `restart_orchestrator.go` + state persistence + 11-state machine

작업:

1. `internal/updater/types.go` 수정: OperationStatus 11-state, ApplyOptions 확장
2. `internal/updater/errors.go` 수정: `ErrUpdateRestartFailed`, `ErrUpdateHealthCheckFailed` 추가
3. `internal/updater/restart_orchestrator.go` 신규 + 단위 테스트 (mock restarter, healthcheck,
   rollback)
4. State persistence (`update_state.json`) 구현 + 테스트
5. `internal/updater/manager.go` 수정: Apply() 분기 (AutoRestart 시 orchestrator 호출)
6. 통합 테스트: drain timeout, exec 실패, health check 실패 → 자동 rollback 시나리오

성공 조건: `auto_restart=true` 시 drain → exec → self health check → completed (또는 자동
rollback) 흐름이 단위/통합 테스트로 검증됨. 커버리지 목표 ≥ 90% (orchestrator 신규 모듈).

### Phase B: M-2 Channel REST API (TDD + DDD)

목표: GET/PUT `/system/update/channel` 핸들러 + DTO + 라우트

작업:

1. `internal/api/dto/update.go` 수정: ChannelGetResponse, ChannelPutRequest/Response 추가
2. `internal/api/handler/system_update_channel.go` 신규 + 단위 테스트
3. admin 권한 검증 미들웨어 통합 (기존 인증 패턴 활용)
4. `internal/api/router.go` 수정: 채널 라우트 등록
5. 통합 테스트: 정상 변경, 잘못된 channel 값, 인증 실패, 권한 부족 시나리오

성공 조건: PUT 정상 응답에 `previous`, `current`, `check_result` 포함. 커버리지 ≥ 90%.

### Phase C: M-2 Web UI 채널 변경 활성화 (TDD)

목표: ChannelChangeDialog 신규 + SystemVersionCard 의 dropdown 활성화

작업:

1. `web/src/lib/api/systemUpdate.ts` 수정: useChannelGet, useChannelPut hooks 추가
2. `web/src/components/system/ChannelChangeDialog.tsx` 신규 + Vitest 테스트
3. `web/src/components/system/SystemVersionCard.tsx` 수정: dropdown + dialog 통합
4. 사용자 시나리오 테스트: 정상 변경, 권한 부족 시 dropdown 복원, 잘못된 채널 거부

성공 조건: 사용자가 dropdown → dialog 확인 → 채널 변경 → SystemVersionCard 즉시 갱신 흐름이
정상 작동. Vitest 커버리지 ≥ 85%.

### Phase D: M-3 Multi-binary Backend (TDD + DDD)

목표: `manifest.json` 형식 + Checker 의 의존성 검증 + target 옵션

작업:

1. `internal/updater/manifest.go` 신규 + 단위 테스트 (manifest 파싱, semver 비교)
2. `internal/updater/errors.go` 수정: `ErrUpdateIncompatibleVersion` 추가
3. `internal/updater/checker.go` 수정: manifest 다운로드 + ValidateCompat() 통합
4. `internal/updater/manager.go` 수정: applyTarget(target) 분기 (xflow CLI 의 drain skip)
5. `internal/api/handler/system_update.go` 수정: target 필드 처리
6. `cmd/xflowd/update.go` 수정: --target flag
7. 통합 테스트: target=xflow / xflow-agent / xflowd 각 케이스, compat 위반 거부

성공 조건: 3종 target 모두 정상 흐름 + 의존성 매트릭스 위반 시 거부. 커버리지 ≥ 88%.

### Phase E: M-3 Frontend target 지원 + 호환성 매트릭스 UX

목표: Web UI 의 target 선택 (선택적, 운영 가시성) + Update Dialog 확장

작업:

1. SystemVersionCard 또는 별도 BinaryStatusPanel 에 다중 바이너리 버전 표시 (xflowd, xflow,
   xflow-agent 각각)
2. UpdateDialog 에 target 선택 dropdown 추가 (default xflowd, 선택 시 다른 바이너리 update)
3. compat 위반 응답 (`ErrUpdateIncompatibleVersion`) 의 사용자 친화 토스트 + 권장 순서 안내
4. 사용자 시나리오 테스트

성공 조건: Web UI 에서 다중 바이너리 update 흐름이 운영자에게 명확. compat 위반이 명시적으로
안내됨.

## 12-15 작업 단위 (Tasks)

| # | Task | Phase | 예상 LOC |
|---|------|-------|---------|
| T1 | OperationStatus 11-state + ApplyOptions 확장 | A | 80 |
| T2 | 신규 sentinel error 3종 추가 | A, D | 60 |
| T3 | `restart_orchestrator.go` 신규 (drain → exec → self health check → rollback) | A | 350 |
| T4 | `update_state.json` disk persistence | A | 150 |
| T5 | Manager.Apply() 분기 + AutoRestart 통합 | A, D | 200 |
| T6 | `system_update_channel.go` GET/PUT 핸들러 | B | 250 |
| T7 | DTO 신규 (ChannelGet/Put) + 라우트 등록 | B | 120 |
| T8 | admin 권한 검증 통합 | B | 80 |
| T9 | `manifest.json` 형식 + Checker.ValidateCompat() | D | 280 |
| T10 | target 옵션 처리 (xflowd | xflow | xflow-agent) | D | 220 |
| T11 | CLI flags `--auto-restart` `--target` | A, D | 100 |
| T12 | Web UI: useChannelGet/Put hooks + ChannelChangeDialog | C | 200 |
| T13 | SystemVersionCard dropdown 활성화 + 권한 에러 처리 | C | 120 |
| T14 | Web UI: target 선택 (BinaryStatusPanel 또는 UpdateDialog 확장) | E | 180 |
| T15 | 문서 (`docs/updater-design.md` v0.2.0 섹션, CHANGELOG) + 통합 테스트 | All | 150 |

총 예상: ~2,540 LOC (테스트 포함 ~3,200 LOC).

## 리스크 및 완화 (Risks & Mitigation)

| # | 리스크 | 영향 | 완화 |
|---|-------|------|------|
| R1 | graceful drain 이 in-flight 메시지 손실 발생 | 운영 신뢰 저하 | drain timeout 길게 (default 30s); 손실 카운트 명시 로깅; 운영자 안내 문서 |
| R2 | self health check 가 timeout 내 실패 (느린 부팅) | 무의미한 자동 rollback | health_check_timeout default 30s (v0.1.0 의 5s 보다 길게); 운영자 yaml 명시 권장 |
| R3 | 두 번째 health check 도 실패 (rollback 도 실패) | 시스템 다운, 운영자 수동 개입 필요 | critical 로그 + alert (webhook/이메일은 future); .previous 백업 보존 가이드 |
| R4 | 의존성 매트릭스 검증 실패 (manifest 손상 또는 누락) | apply 거부 → 운영 부담 | --force flag 로 회피 가능; manifest 다운로드 retry; 명시적 에러 메시지 |
| R5 | manifest.json MITM 공격 (Ed25519 서명 미검증 시) | 잘못된 호환성 매트릭스로 비호환 update 적용 | manifest 도 서명 검증 (구현 단계 명시) |
| R6 | exec syscall 권한 부족 (root 아닌 사용자) | restart 실패 | 권한 검증 + 명시적 에러 (`ErrUpdateRestartFailed`); systemd 환경에서는 SIGTERM fallback |
| R7 | systemd 환경에서 Type=notify 미설정 시 exec 흐름 충돌 | 프로세스 충돌 | 환경 감지 후 SIGTERM 우선; v0.1.0 의 restarter.go 패턴 활용 |
| R8 | 채널 변경 PUT 의 race condition (동시 변경 + check) | 응답 결과 불일치 | Manager 의 mutex + sequential SetChannel → Check |
| R9 | Web UI 의 dropdown 을 권한 없는 사용자가 변경 시도 | 403 응답 후 dropdown 복원 실패 시 UI 혼란 | mutation onError 에서 명시적 dropdown 복원 + 토스트 |
| R10 | xflow CLI 자가 교체 중 사용자가 같은 CLI 실행 | 파일 lock 충돌 | atomic rename 의 원자성 활용; rare race 는 retry |
| R11 | OperationStatus 11-state 가 v0.1.0 호출자에게 unknown 으로 보임 | 클라이언트 호환성 | v0.1.0 호출은 auto_restart=false default → 11-state 진입 안 함; 클라이언트 enum 확장은 v0.2.0 명시 |
| R12 | update_state.json 손상 (디스크 가득 등) | 새 프로세스 부팅 시 health check 단계 추적 불가 | 손상 감지 시 graceful skip + 명시 로그; 운영자 cleanup 가이드 |

## TDD 테스트 전략

### 단위 테스트 (신규 모듈 ≥ 90% 커버리지 목표)

- `restart_orchestrator_test.go`: drain timeout, exec 실패, health check 성공/실패, rollback
  성공/실패 4×4 = 16 시나리오 (mock 활용)
- `manifest_test.go`: 정상 파싱, 잘못된 JSON, semver 비교 (다양한 제약 표현)
- `system_update_channel_test.go`: GET 정상, PUT 정상, 권한 부족, 잘못된 channel 값,
  Manager 에러 전파

### 통합 테스트 (테스트 GitHub Releases 서버 활용)

- M-1 end-to-end: apply --auto-restart → drain → exec → self health check → completed
- M-1 실패: health check 실패 → 자동 rollback → 두 번째 health check 성공 → failed
- M-1 catastrophic: rollback 도 실패 → ErrUpdateRollbackFailed + critical 로그
- M-2 정상: 채널 변경 + 즉시 check → response 검증
- M-3 정상: 3종 target 각각 성공
- M-3 위반: compat 제약 위반 → 거부

### Web UI 테스트 (Vitest, ≥ 85% 커버리지)

- ChannelChangeDialog: 정상 표시, 확인/취소 버튼
- SystemVersionCard: dropdown 변경 → dialog → mutation → 갱신
- 권한 부족: 403 응답 → 토스트 + dropdown 복원

### 호환성 회귀 테스트

- v0.1.0 호출 (auto_restart 미지정, target 미지정) 이 v0.2.0 코드에서 v0.1.0 동일 동작
- v0.1.0 의 9-state machine 만 관찰됨 (auto_restart=false 시)

## 마일스톤

| Milestone | 의존성 | 핵심 산출물 |
|-----------|-------|------------|
| **Primary Goal**: M-1 + M-3 backend (Phase A + Phase D) | SPEC-UPDATE-001 머지 | restart_orchestrator + manifest + target 옵션 |
| **Secondary Goal**: M-2 backend + frontend (Phase B + Phase C) | SPEC-WEB-006 머지 | 채널 REST + Web UI 활성화 |
| **Final Goal**: M-3 frontend (Phase E) + 통합 문서 | Primary + Secondary | BinaryStatusPanel + 문서 v0.2.0 섹션 |
| **Optional Goal**: 운영 가이드 + CI workflow 변경 PR | Final | 별도 PR 에서 GitHub Actions release workflow |

우선순위는 Primary > Secondary > Final 순. 각 마일스톤 완료 시 quality gate (TRUST 5) 통과 + 전체
테스트 통과 + 코드 리뷰 완료.

## 빌드 및 배포 영향

- 빌드 변수 변경 없음 (UpdaterPublicKey 등 v0.1.0 그대로)
- GitHub Actions release workflow 변경 필요 (별도 PR):
  - `manifest.json` 자동 생성 step 추가 (각 바이너리 별)
  - 모든 바이너리 동일 키로 서명
- yaml config schema 호환 (신규 필드는 모두 optional)
