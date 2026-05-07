---
id: SPEC-UPDATE-002
title: xflowd 자동 업데이트 v0.2.0 — in-process restart + 채널 API + 멀티 바이너리
version: 0.1.0
status: draft
created: 2026-05-06
updated: 2026-05-06
author: xtra
priority: medium
---

# SPEC-UPDATE-002: xflowd 자동 업데이트 v0.2.0

## HISTORY

- **0.1.0** (2026-05-06): Initial draft — SPEC-UPDATE-001 v0.1.0 의 알려진 3가지 한계
  ((1) in-process restart 미지원, (2) 채널 변경 REST API 부재, (3) 단일 바이너리 (xflowd) 만
  지원) 를 단일 v0.2.0 릴리즈로 통합 해결. 3 Milestone 구성: M-1 In-Process Restart
  (graceful drain → exec → self health check → 자동 rollback wiring), M-2 채널 REST API
  (GET/PUT `/system/update/channel`, Web UI 채널 변경 활성화), M-3 멀티 바이너리 지원
  (xflowd, xflow CLI, xflow-agent target 옵션 + 의존성 매트릭스). v0.1.0 backward
  호환 (auto_restart=false default, target 미지정 시 xflowd default).
  5 Decision Point 명시: 활성화 방식, 채널 변경 시맨틱, 멀티 바이너리 조정 모델, 공개키
  모델, 롤백 범위. SPEC-WEB-006 v0.2.0 의 채널 변경 UI 활성화를 가능하게 한다.

| Version | Date       | Author | Change                                                                                                |
| ------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------- |
| 0.1.0   | 2026-05-06 | xtra   | 최초 작성 — SPEC-UPDATE-001 v0.1.0 의 3가지 한계 통합 해결. M-1 in-process restart, M-2 채널 REST, M-3 멀티 바이너리 |

---

## 개요 (Overview)

### 목적

SPEC-UPDATE-001 v0.1.0 은 xflowd 자가 교체 자동 업데이트의 핵심 인프라(GitHub Releases 채널,
Ed25519 + SHA256 검증, atomic replacement, 그레이스풀 drain, 자동 rollback 컴포넌트, 9-state
operation, 5종 REST API/CLI) 를 도입했으나 운영 자동화 측면에서 3가지 한계를 명시적으로 남겼다.
본 SPEC 은 이 3가지를 단일 **v0.2.0** 릴리즈로 통합 해결하여 **API 한 번 호출 → 완전 자동 적용
→ 자가 health check → 실패 시 자동 rollback → 다중 바이너리 통일 흐름** 을 제공한다.

본 SPEC 은 v0.1.0 의 8개 internal/updater 컴포넌트(types, errors, verifier, keys, checker,
downloader, applier, rollback, restarter, healthcheck) 를 그대로 활용하되, 다음 신규 모듈을
추가한다:

- `internal/updater/restart_orchestrator.go` (M-1 wiring)
- `internal/updater/manifest.go` (M-3 의존성 매트릭스)
- `internal/api/handler/system_update_channel.go` (M-2 신규 핸들러)

또한 `internal/api/handler/system_update.go` 와 `cmd/xflowd/update.go`, `internal/config/update.go` 를
수정하여 신규 옵션(`auto_restart`, `target`) 을 지원한다.

### 배경

SPEC-UPDATE-001 v0.1.0 구현 완료 후 파악된 3가지 운영 부담:

1. **in-process restart 미지원**: Apply 작업이 `ready_to_restart` 상태에서 멈추고 운영자가
   `systemctl restart xflowd` 또는 수동 재시작을 직접 수행해야 한다. 새 바이너리가 정상 부팅되는지
   별도 health check 도 운영자 책임. 자동화 파이프라인(예: ArgoCD, GitOps) 에서 사용 불편.

2. **채널 변경 REST API 부재**: 채널 변경은 CLI (`xflowd update channel <name>`) 만 지원하며,
   Web UI (SPEC-WEB-006 v0.1.0) 에서는 현재 채널만 표시 (읽기 전용, M10 limitation 으로 명시).
   다중 운영자 협업 환경에서 SSH 접근 없이 Web UI 에서 채널을 전환할 수 없다.

3. **단일 바이너리(xflowd) 만 지원**: SPEC-UPDATE-001 의 M14 에서 xflow CLI / xflow-agent 자동
   업데이트는 후속 SPEC 으로 명시 deferred 되었으나, 통합 운영을 위해서는 단일 SPEC 으로
   해결하는 것이 운영 부담 측면에서 더 효율적이다 (CLI 명령 일관성, 의존성 매트릭스 통합).

### v0.2.0 목표

- **완전 자동화**: API 한 번 호출로 download → verify → apply → drain → exec → self health
  check → completed (또는 자동 rollback) 까지 자동
- **Web UI 통합**: 채널 변경을 Web UI 에서 수행 가능 (SPEC-WEB-006 v0.2.0 활성화 trigger)
- **다중 바이너리**: xflowd / xflow / xflow-agent 모두 동일 패턴으로 자가 교체
- **호환성 검증**: GitHub Releases asset 의 manifest.json 으로 바이너리 간 의존성 매트릭스 검증
- **v0.1.0 backward 호환**: `auto_restart=false` (default), `target` 미지정 시 xflowd default

### 범위

- **포함 (M-1 In-Process Restart)**:
  - POST `/api/v1/system/update/apply` 의 `auto_restart: bool` 옵션 추가
  - CLI `xflowd update apply --auto-restart` flag
  - `applying` 후 자동 graceful drain → `syscall.Exec` → 새 프로세스 self-probe
  - 새 프로세스 health check 실패 → 자동 rollback (.previous 복원 + 재시작)
  - 신규 OperationStatus 2종: `restarting`, `health_checking` (기존 9-state → 11-state)
  - 신규 sentinel error 2종: `ErrUpdateRestartFailed`, `ErrUpdateHealthCheckFailed`
- **포함 (M-2 채널 REST API)**:
  - GET `/api/v1/system/update/channel` (현재 채널 + 가용 목록)
  - PUT `/api/v1/system/update/channel` (변경 + 즉시 새 check 결과 반환, admin 권한 필수)
  - SPEC-WEB-006 의 SystemVersionCard 채널 dropdown 활성화 (v0.1.0 의 M10 limitation 해소)
- **포함 (M-3 멀티 바이너리)**:
  - `target` 옵션 (xflowd | xflow | xflow-agent), default `xflowd`
  - GitHub Releases asset 의 `manifest.json` 형식 정의 (호환성 매트릭스)
  - `Checker` 가 manifest 를 다운로드 + semver 비교 → 비호환 시 거부
  - 단일 Ed25519 공개키 모델 (모든 바이너리 동일 키)
  - xflow CLI 자가 교체 (drain 불필요, atomic replace 후 즉시 종료)
  - 신규 sentinel error 1종: `ErrUpdateIncompatibleVersion`
- **제외**:
  - **Atomic group update**: 여러 바이너리를 단일 트랜잭션으로 업데이트하는 흐름은 본 SPEC 에서
    excluded (Decision Point 3 권장값 = independent). 향후 SPEC-UPDATE-004 에서 검토.
  - **Windows 지원**: M-3 의 xflow CLI 자가 교체는 POSIX rename 기반. Windows 는 별도
    SPEC-UPDATE-003 에서 `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` + Service Control Manager
    통합으로 다룬다.
  - **세분화된 RBAC**: 채널 변경의 admin 권한은 v0.1.0 의 인증 패턴(JWT 토큰의 `role: admin`
    클레임 또는 동등) 을 그대로 활용. 정식 RBAC 은 SPEC-WEB-007 에서.
  - **Telemetry**: 업데이트 결과 통계 수집은 v0.2.0 에서 도입하지 않음 (privacy SPEC 별도).

### 가정

- SPEC-UPDATE-001 v0.1.0 이 main branch 에 머지되어 있음 (현재는 feature/SPEC-UPDATE-001 브랜치
  PR 미머지 상태이나, 본 SPEC 구현 시점 머지 가정)
- SPEC-WEB-006 v0.1.0 이 main branch 에 머지되어 있음 (현재는 feature/SPEC-WEB-006 브랜치
  PR 미머지 상태이나, M-2 의 Web UI 활성화 시점 머지 가정)
- GitHub Releases 가 `xflowd-{os}-{arch}`, `xflow-{os}-{arch}`, `xflow-agent-{os}-{arch}`,
  `manifest.json`, `signature.bin`, `checksums.txt` 6종 asset 을 동시 게시 (CI 변경 필요,
  운영 가이드 별도)
- 모든 바이너리(xflowd, xflow, xflow-agent) 가 동일한 Ed25519 키쌍으로 서명됨 (Decision Point 4)
- POSIX 환경 (Linux primary, macOS secondary), Windows 는 본 SPEC 범위 외

---

## EARS 요구사항 (EARS Requirements)

본 SPEC 은 14개 EARS 모듈로 구성된다. M1-M5 = Group A (M-1 In-Process Restart),
M6-M8 = Group B (M-2 채널 REST API), M9-M14 = Group C (M-3 멀티 바이너리).

---

### Group A: In-Process Restart (M1-M5)

#### M1: auto_restart 옵션 (Auto Restart Option)

- **Ubiquitous**: 시스템은 POST `/api/v1/system/update/apply` 요청 본문에 신규 필드
  `auto_restart` (bool, optional, default `false`) 를 지원해야 한다.
- **Ubiquitous**: 시스템은 CLI `xflowd update apply` 에 신규 flag `--auto-restart` 를 지원해야 한다.
- **State-driven**: IF `auto_restart == false` (default) 이면, THEN v0.1.0 의 기존 동작과
  완전히 동일하게 작동해야 한다 (`applying` 후 `ready_to_restart` 상태로 종료, 운영자 수동 재시작).
- **State-driven**: IF `auto_restart == true` 이면, THEN 시스템은 `applying` 완료 직후
  M2-M5 의 graceful drain → exec → self health check → (실패 시 rollback) 흐름을
  자동 수행해야 한다.
- **Ubiquitous**: REST API 호출자에게는 `applying` 단계까지의 결과를 동기 응답으로 반환하고,
  이후 `restarting` / `health_checking` 단계는 비동기로 진행되며 `GET /update/status` 폴링으로
  추적해야 한다 (HTTP 연결 자체가 재시작으로 끊길 수 있음 — clients MUST handle).

#### M2: graceful drain (Graceful Drain Sequence)

- **Event-driven**: WHEN `auto_restart == true` 인 apply 작업의 `applying` 단계가 완료되면,
  THEN 시스템은 OperationStatus 를 `restarting` 으로 전이하고 graceful drain 시퀀스를 시작해야 한다.
- **Ubiquitous**: drain 시퀀스는 다음을 순서대로 수행한다:
  1. HTTP server 의 신규 연결 차단 (`http.Server.Shutdown(ctx)`)
  2. in-flight 요청 완료 대기 (timeout `update.drain_timeout`, default `30s`)
  3. FBP 엔진 in-flight 메시지 drain (timeout 동일)
  4. 영속화 레이어 flush
- **Unwanted**: WHEN drain timeout 이 초과되면, THEN 시스템은 강제 종료를 수행하되 손실된
  in-flight 메시지/요청 카운트를 명시적으로 구조화 로그(`update.drain_timeout`)에 기록해야 한다.
- **Ubiquitous**: drain 진행 중 OperationStatus 는 `restarting` 으로 유지되며, `progress_percent`
  는 drain 진행률을 반영해야 한다 (예: timeout 의 경과 시간 비율).
- **State-driven**: IF 환경이 systemd notify 모드이면, THEN 시스템은 `STOPPING=1` 알림을
  systemd 에 발송한 후 drain 을 시작해야 한다.

#### M3: exec 전환 (syscall.Exec Transition)

- **Event-driven**: WHEN drain 완료되면, THEN 시스템은 새 바이너리(이미 atomic replace 된
  현재 경로) 로 `syscall.Exec` 호출을 통해 프로세스 이미지를 교체해야 한다.
- **Ubiquitous**: PID 는 보존되어야 한다 (process image 만 교체, kernel 관점에서 동일 프로세스).
- **State-driven**: IF systemd `Type=notify` 모드이면, THEN syscall.Exec 대신 SIGTERM →
  systemd 재시작 흐름을 사용해야 한다 (supervisor 의존성 자연스러움). 환경 감지 로직은 v0.1.0 의
  `restarter.go` 에서 구현됨.
- **Unwanted**: WHEN exec 호출이 실패하면 (권한, 파일 손상 등), THEN 시스템은 OperationStatus
  를 `failed` 로 전이하고 신규 sentinel error `ErrUpdateRestartFailed` 를 반환해야 한다.
  이 경우 .previous 백업 파일은 보존되며, 운영자가 `xflowd update rollback` 으로 복구 가능.
- **Ubiquitous**: exec 호출 직전 시스템은 구조화 로그 이벤트 `update.exec` 를 기록해야 한다
  (current_version, target_version, drain_duration_ms 필드 포함).

#### M4: 자가 health check (Self Health Check)

- **Event-driven**: WHEN 새 프로세스가 부팅되면, THEN HealthChecker (v0.1.0 의 `healthcheck.go`
  활용) 는 self-probe 를 수행해야 한다 (예: localhost 의 `GET /api/v1/system/version` 호출).
- **Ubiquitous**: self-probe 는 다음 조건을 충족해야 한다:
  - timeout: `update.health_check_timeout` (default `30s`)
  - 폴링 주기: `update.health_check_interval` (default `1s`)
  - 성공 조건: HTTP 200 + 응답 버전이 target_version 과 일치
- **Ubiquitous**: self-probe 진행 중 OperationStatus 는 `health_checking` 이어야 한다.
- **State-driven**: IF self-probe 가 timeout 내 성공하면, THEN OperationStatus 를 `completed`
  로 전이하고 구조화 로그 `update.health_check_passed` 를 기록한다.
- **Unwanted**: WHEN self-probe 가 timeout 동안 한 번도 성공하지 못하면, THEN 시스템은 신규
  sentinel error `ErrUpdateHealthCheckFailed` 를 발생시키고 M5 의 자동 rollback 을 트리거한다.

#### M5: 자동 rollback (Auto Rollback on Health Check Failure)

- **Event-driven**: WHEN `ErrUpdateHealthCheckFailed` 가 발생하면, THEN 시스템은 자동으로
  v0.1.0 의 rollback 로직을 호출해야 한다 (`<binary>.previous` → 원래 위치 atomic rename).
- **Ubiquitous**: 자동 rollback 후 시스템은 이전 바이너리로 재시작하고 두 번째 self health check
  을 수행한다.
- **State-driven**: IF 두 번째 self-probe 가 성공하면, THEN OperationStatus 를 `failed` 로
  전이하고 (rollback 자체는 성공했으나 원래 의도한 update 는 실패), `error` 필드에
  `ErrUpdateHealthCheckFailed` 메시지를 명시한다.
- **Unwanted**: WHEN 두 번째 self-probe 도 timeout 내 성공하지 못하면, THEN 시스템은 신규
  sentinel error `ErrUpdateRollbackFailed` (v0.1.0 에 이미 존재) 를 명시 발생시키고 critical
  로그를 남기며, 운영자 수동 개입을 요구한다 (자동 재시도 없음, infinite loop 방지).
- **Ubiquitous**: 자동 rollback 결과 (성공/실패) 는 구조화 로그 이벤트 `update.auto_rollback`
  으로 기록되어야 한다.
- **Ubiquitous**: 자동 rollback 발생 시 `update.enabled = false` (또는 임시 lock) 로 자동 적용
  을 일시 중단해야 한다 (v0.1.0 M7 의 loop 방지 정책 동일).

---

### Group B: 채널 REST API (M6-M8)

#### M6: GET /system/update/channel

- **Ubiquitous**: 시스템은 신규 엔드포인트 `GET /api/v1/system/update/channel` 을 제공해야 한다.
- **Ubiquitous**: 응답 페이로드는 `current` (현재 채널, string), `available` (가용 채널 목록,
  array of string) 을 포함해야 한다.
- **State-driven**: IF 인증되지 않은 호출자이면, THEN HTTP 401 Unauthorized.
- **State-driven**: IF 인증은 되었으나 권한 부족이면, THEN HTTP 200 (조회는 일반 사용자도 가능,
  v0.1.0 의 일반 인증 정책과 동일).
- **Ubiquitous**: 응답은 `{ "current": "stable", "available": ["stable", "beta", "nightly"] }`
  형식이어야 한다.

#### M7: PUT /system/update/channel

- **Ubiquitous**: 시스템은 신규 엔드포인트 `PUT /api/v1/system/update/channel` 을 제공해야 한다.
- **Ubiquitous**: 요청 바디는 `{ "channel": "<stable|beta|nightly>" }` 형식이며, 채널 enum
  외 값은 거부.
- **Ubiquitous**: 채널 변경 성공 시 응답은 `previous` (이전 채널), `current` (새 채널),
  `check_result` (즉시 새 채널로 수행한 CheckResult) 3개 필드를 포함해야 한다.
  WHY: Decision Point 2 권장값 = "변경 + 즉시 check" (운영자가 채널 변경 후 별도 check 호출
  없이 새 채널의 latest version 즉시 확인 가능).
- **State-driven**: IF 인증되지 않은 호출자이면, THEN HTTP 401 Unauthorized.
- **Unwanted**: WHEN 인증된 호출자이나 admin 권한 없으면 (JWT 토큰의 `role: admin` 또는 동등
  클레임 부재), THEN HTTP 403 Forbidden + 명시적 에러 메시지.
- **Unwanted**: WHEN 채널 enum 외 값 (`"unstable"` 등) 이 요청되면, THEN HTTP 400 Bad Request +
  v0.1.0 의 `ErrUpdateChannelInvalid` 반환.
- **Ubiquitous**: 채널 변경은 v0.1.0 의 Manager.SetChannel() 을 호출하며, 변경 직후 자동으로
  새 채널의 Manager.Check() 를 실행하여 그 결과를 응답에 포함한다.
- **Ubiquitous**: 채널 변경 시도 (성공/거부 무관) 는 구조화 로그 이벤트 `update.channel_change`
  로 기록되어야 한다 (`previous`, `requested`, `result`, `actor`).

#### M8: Web UI 채널 변경 UX (SPEC-WEB-006 v0.2.0 활성화)

- **Ubiquitous**: SPEC-WEB-006 의 `SystemVersionCard` 컴포넌트는 채널 표시를 dropdown 으로
  활성화해야 한다 (v0.1.0 의 읽기 전용 → v0.2.0 active).
- **Event-driven**: WHEN 사용자가 채널 dropdown 을 변경하면, THEN 시스템은 사전 확인 dialog
  (`ChannelChangeDialog`) 를 표시해야 한다.
- **Ubiquitous**: ChannelChangeDialog 는 다음을 명시해야 한다:
  - 이전 채널 → 새 채널
  - 다음 update check 부터 새 채널이 반영된다는 안내
  - admin 권한이 필요하다는 안내 (사용자가 권한 없는 경우 사전 차단 가능, M7 의 403 응답에 의존)
- **Event-driven**: WHEN 사용자가 ChannelChangeDialog 의 확인을 누르면, THEN frontend 는 PUT
  `/system/update/channel` 호출 → 응답의 `check_result` 로 SystemVersionCard 의
  `update_available` / `latest_version` 즉시 갱신해야 한다.
- **Unwanted**: WHEN PUT 응답이 HTTP 403 (권한 부족) 이면, THEN frontend 는 명시적 토스트
  ("채널 변경에는 admin 권한이 필요합니다") 를 표시하고 dropdown 을 이전 값으로 복원해야 한다.

---

### Group C: 멀티 바이너리 (M9-M14)

#### M9: target 옵션 (Target Binary Option)

- **Ubiquitous**: 시스템은 POST `/api/v1/system/update/apply` 요청 본문에 신규 필드
  `target` (string, optional, default `"xflowd"`) 을 지원해야 한다.
- **Ubiquitous**: 시스템은 CLI `xflowd update apply` 에 신규 flag `--target <binary>` 를
  지원해야 한다.
- **Ubiquitous**: target 의 허용 값은 `"xflowd"`, `"xflow"`, `"xflow-agent"` 3종이며 enum
  검증을 통과해야 한다.
- **State-driven**: IF target 미지정 (default) 이면, THEN v0.1.0 동작과 동일 (xflowd 적용).
- **Unwanted**: WHEN target 값이 enum 외이면, THEN HTTP 400 + `ErrUpdateChannelInvalid`
  유사의 신규 검증 에러를 반환.
- **Ubiquitous**: 동일 target 에 대해 동시에 두 개의 apply 작업이 들어오면 두 번째는 HTTP 409
  Conflict (v0.1.0 M10 정책 동일). 다른 target 의 동시 apply 는 v0.2.0 에서는 **거부** 한다
  (single-op-only, Decision Point 3 권장값 = independent + 직렬화).

#### M10: 바이너리별 자가 교체 (Per-Binary Self Replacement)

- **State-driven**: IF target == "xflowd" 이면, THEN v0.1.0 의 기존 흐름 + (M-1 활성화 시)
  graceful drain + exec + self health check 흐름을 따른다.
- **State-driven**: IF target == "xflow-agent" 이면, THEN xflowd 와 동일한 long-running
  process 흐름 + graceful drain (FBP 메시지 drain) + exec + self health check 을 따른다.
- **State-driven**: IF target == "xflow" (CLI) 이면, THEN graceful drain 단계를 SKIP 하고
  atomic replace 후 즉시 종료한다.
  WHY: xflow CLI 는 one-shot 도구로 long-running process 가 아님. drain 대상 in-flight 작업이
  존재하지 않음.
- **Ubiquitous**: target == "xflow" 인 경우 self health check 도 SKIP 한다 (CLI 는 부팅 후
  대기하지 않으므로 self-probe 무의미). 다음 CLI 실행 시 새 바이너리 자동 사용.
- **Ubiquitous**: target == "xflowd" 또는 "xflow-agent" 인 경우 v0.1.0 의 OperationStatus
  9-state + (M-1 활성화 시) 11-state 가 동일하게 적용된다.
- **Ubiquitous**: 모든 target 에 대해 v0.1.0 의 검증 단계 (체크섬 + Ed25519) 는 동일하게 적용된다.

#### M11: 의존성 매트릭스 검증 (Compatibility Matrix Validation)

- **Ubiquitous**: 시스템은 GitHub Releases 에 게시된 `manifest.json` asset 을 다운로드하고
  파싱해야 한다.
- **Ubiquitous**: manifest.json 의 형식은 다음과 같다:
  ```json
  {
    "version": "v0.4.0",
    "binary": "xflow-agent",
    "compat": {
      "xflowd": ">=v0.4.0",
      "xflow": ">=v0.3.0"
    }
  }
  ```
  - `version`: 본 바이너리의 버전
  - `binary`: 바이너리 이름 (xflowd | xflow | xflow-agent)
  - `compat`: 다른 바이너리에 대한 semver 제약 (선택, 없으면 모든 버전과 호환으로 간주)
- **Event-driven**: WHEN apply 작업 시작 시, THEN Checker 는 manifest.json 을 다운로드하고
  현재 시스템에 설치된 다른 바이너리들의 버전을 query 하여 semver 비교를 수행해야 한다.
- **State-driven**: IF 모든 compat 제약이 만족되면, THEN apply 를 진행한다.
- **Unwanted**: WHEN 하나라도 compat 제약이 위반되면 (예: target=xflow-agent v0.4.0 의
  `xflowd: ">=v0.4.0"` 제약을 현재 xflowd v0.3.5 가 위반), THEN apply 를 거부하고 신규
  sentinel error `ErrUpdateIncompatibleVersion` 을 반환해야 한다 (HTTP 409 또는 422).
- **Ubiquitous**: 거부 응답에는 위반된 제약과 현재 버전을 명시해야 한다 (운영자가 어떤 바이너리
  를 먼저 업데이트해야 하는지 판단 가능).
- **State-driven**: IF `--force` flag 가 명시되면, THEN compat 제약 위반을 경고로 강등하고
  apply 를 진행한다 (v0.1.0 의 다운그레이드 force 정책 일관성).
- **Ubiquitous**: 다른 바이너리의 현재 버전 query 방법:
  - xflowd: 현재 프로세스의 빌드 변수
  - xflow / xflow-agent: 동일 디렉토리의 바이너리에 대해 `<binary> --version` 호출 (best-effort)

#### M12: 단일 공개키 (Single Ed25519 Public Key Model)

- **Ubiquitous**: 모든 바이너리(xflowd, xflow, xflow-agent) 는 동일한 Ed25519 공개키로 서명
  검증을 수행한다.
  WHY: Decision Point 4 권장값 = single key. 운영 편의 + 단일 신뢰점 + 키 관리 단순화.
- **Ubiquitous**: 공개키는 v0.1.0 의 `internal/updater/keys.go` 에 임베드된 키를 그대로 사용
  하며, 빌드 시 `-ldflags "-X main.UpdaterPublicKey=..."` 로 주입된다.
- **Ubiquitous**: GitHub Actions release workflow 는 모든 바이너리 (xflowd, xflow,
  xflow-agent) 를 동일한 비공개키로 서명하고 단일 `signature.bin` 을 게시한다.
  주의: 각 바이너리에 대한 별도 서명 파일이 필요한 경우 `signature-{binary}.bin` 형식 사용
  (구현 단계에서 결정).
- **Unwanted**: 만약 향후 키 분리(per-binary key) 가 필요해지면, 별도 SPEC 으로 마이그레이션
  계획 + 이중 검증 기간을 명시한다 (본 SPEC 범위 외).

#### M13: 신규 sentinel errors

- **Ubiquitous**: 시스템은 다음 3종 신규 sentinel error 를 정의해야 한다:
  - `ErrUpdateRestartFailed`: graceful drain timeout 또는 syscall.Exec 실패 (M3-M5)
  - `ErrUpdateHealthCheckFailed`: 새 바이너리 self-probe 실패 (M4-M5)
  - `ErrUpdateIncompatibleVersion`: manifest.json 의 compat 제약 위반 (M11)
- **Ubiquitous**: v0.1.0 의 9종 기존 sentinel error (`ErrUpdateChannelInvalid`,
  `ErrUpdateDownloadFailed`, ..., `ErrUpdateRollbackFailed`) 는 그대로 유지된다.
- **Ubiquitous**: 모든 sentinel error 는 `errors.Is(err, ErrXxx)` 로 비교 가능하도록 패키지
  레벨 변수로 정의되어야 한다.
- **Ubiquitous**: REST API 응답에서 sentinel error 는 일관된 코드/메시지로 매핑되어야 한다 (예:
  `ErrUpdateIncompatibleVersion` → HTTP 409, `ErrUpdateHealthCheckFailed` → HTTP 500 또는
  status 응답의 `error` 필드).

#### M14: 호환성 보존 (Backward Compatibility with v0.1.0)

- **Ubiquitous**: v0.1.0 으로 작성된 모든 호출 (REST API, CLI, yaml config) 은 v0.2.0 에서
  변경 없이 동일하게 작동해야 한다.
- **State-driven**: IF `auto_restart` 가 미지정 (default `false`) 이면, THEN v0.1.0 동작 그대로
  (`ready_to_restart` 종료, 운영자 수동 재시작).
- **State-driven**: IF `target` 이 미지정 (default `"xflowd"`) 이면, THEN v0.1.0 동작 그대로
  (xflowd 만 적용).
- **State-driven**: IF 채널 변경에 CLI (`xflowd update channel <name>`) 만 사용하면, THEN
  v0.1.0 동작 그대로 (REST API 호출 없이 yaml + Manager.SetChannel() 만 호출).
- **Ubiquitous**: v0.2.0 의 신규 sentinel error 3종은 기존 9종에 추가되며, 기존 코드의
  `errors.Is` 패턴을 깨지 않는다.
- **Ubiquitous**: v0.1.0 의 9-state OperationStatus 는 그대로 유지되며, v0.2.0 의 11-state 는
  `auto_restart=true` 일 때만 사용된다 (default flow 영향 없음).

---

## 명세 (Specifications)

### Config 스키마 변경

```yaml
update:
  enabled: true
  channel: "stable"
  check_interval: "24h"
  auto_apply: false
  notify_only: true
  update_url: ""
  public_key_path: ""
  drain_timeout: "30s"
  health_check_timeout: "30s"     # CHANGED v0.2.0: default 5s → 30s (self-probe 시간 고려)
  health_check_interval: "1s"     # NEW v0.2.0: self-probe 폴링 주기
  health_check_endpoint: "/api/v1/system/version"  # NEW v0.2.0: self-probe target
  insecure_skip_verify: false
  target_binaries:                # NEW v0.2.0: 다중 바이너리 지원 (선택)
    - "xflowd"
    # - "xflow"
    # - "xflow-agent"
```

신규 필드:

- `health_check_interval` (duration, default `1s`): self-probe 폴링 주기
- `health_check_endpoint` (string, default `/api/v1/system/version`): self-probe HTTP path
- `target_binaries` (array of string, default `["xflowd"]`): 자동 업데이트 활성화할 바이너리

호환성:

- `health_check_timeout` 의 default 가 `5s` → `30s` 로 변경 (v0.1.0 보다 길게). 명시적 설정
  값이 있으면 그대로 사용.

### CLI 변경

```text
# 기존 v0.1.0 호환
xflowd update apply [--version <ver>] [--force] [--yes] [--json]
  → target=xflowd, auto_restart=false (default)

# v0.2.0 신규 옵션
xflowd update apply --auto-restart [--target <xflowd|xflow|xflow-agent>] ...
  → 자동 재시작 + self health check + 자동 rollback

# v0.2.0 신규 채널 명령은 CLI 에 변경 없음 (REST API 만 추가)
xflowd update channel <stable|beta|nightly>  # v0.1.0 그대로
```

### REST API 변경 (BREAKING 아님, 추가만)

**POST `/api/v1/system/update/apply`** (수정, 신규 필드 추가):

요청 바디:

```json
{
  "version": "v0.4.0",
  "force": false,
  "skip_confirm": true,
  "auto_restart": true,           // NEW v0.2.0
  "target": "xflow-agent"         // NEW v0.2.0, default "xflowd"
}
```

응답 형식은 v0.1.0 과 동일 (`operation_id`, `status`, `current_version`, `target_version`,
`started_at`).

**`GET /api/v1/system/update/status`** (수정, status enum 확장):

기존 9-state + 신규 2-state:

```text
idle | starting | checking | downloading | verifying | applying
  → ready_to_restart (auto_restart=false 시 종료)
  → restarting → health_checking → completed (auto_restart=true)
                    ↓ (실패)
                rollback → ... (자동)
  | failed
```

**`GET /api/v1/system/update/channel`** (NEW v0.2.0):

응답:

```json
{
  "success": true,
  "data": {
    "current": "stable",
    "available": ["stable", "beta", "nightly"]
  }
}
```

**`PUT /api/v1/system/update/channel`** (NEW v0.2.0, admin 필수):

요청:

```json
{ "channel": "beta" }
```

응답:

```json
{
  "success": true,
  "data": {
    "previous": "stable",
    "current": "beta",
    "check_result": {
      "current_version": "v0.3.0",
      "latest_version": "v0.4.0-beta.1",
      "update_available": true,
      "channel": "beta",
      "release_notes_url": "https://github.com/xtra72/xflow/releases/tag/v0.4.0-beta.1",
      "published_at": "2026-05-05T08:00:00Z"
    }
  }
}
```

### Operation State Machine 진화

```
v0.1.0 (auto_restart=false, default v0.2.0):
  idle → starting → checking → downloading → verifying → applying
                                                            ↓
                                                  ready_to_restart → completed
                                                            (운영자 수동 재시작 후)

v0.2.0 (auto_restart=true):
  idle → starting → checking → downloading → verifying → applying
                                                            ↓
                                                       restarting → health_checking
                                                                          ↓
                                                                    [success] completed
                                                                    [fail] → (자동 rollback)
                                                                              ↓
                                                                          health_checking
                                                                              ↓
                                                                    [success] failed (with rollback)
                                                                    [fail] failed (rollback failed,
                                                                                    수동 개입 필요)
```

### 5 Decision Points

본 SPEC 의 핵심 설계 결정 5종 — 각 항목에 권장값 명시:

#### Decision Point 1: In-process restart 활성화 방식

**옵션 A**: POST `/update/apply` 의 `auto_restart: bool` 옵션 (v0.1.0 endpoint 확장)
**옵션 B**: 별도 endpoint `POST /update/restart` (apply 와 분리)

**권장**: **A** (apply 옵션)

WHY:
- v0.1.0 backward 호환 유지 (default false → 기존 동작 그대로)
- 단일 호출로 download → verify → apply → restart → health check 을 atomic 하게 수행
- 운영자가 apply 와 restart 를 별도 호출할 필요 없음 (자동화 친화)
- B 옵션은 endpoint 가 늘어나고 apply 후 restart 사이의 race condition 처리가 복잡

IMPACT: A 채택 시 `system_update.go` 핸들러만 수정. B 채택 시 신규 핸들러 + state machine
복잡화.

#### Decision Point 2: 채널 변경 시맨틱

**옵션 A**: PUT 요청이 채널 변경 + 즉시 새 채널의 check 결과 반환 (단일 호출)
**옵션 B**: PUT 요청은 채널 변경만 수행. check 는 별도 POST `/update/check` 호출 필요

**권장**: **A** (변경 + 즉시 check)

WHY:
- 운영자가 채널 변경 후 별도 check 호출 없이 새 채널의 latest version 즉시 확인 가능
- Web UI 의 사용자 경험 단순화 (한 번 클릭으로 dropdown 변경 + 결과 확인)
- 채널 변경 의도는 본질적으로 "새 채널의 update 가능 여부 확인" 이므로 같이 묶는 것이 자연스러움

IMPACT: A 채택 시 PUT 핸들러에서 SetChannel() + Check() 를 sequential 호출. B 채택 시 두
번의 round-trip + 클라이언트 state 관리 복잡.

#### Decision Point 3: 멀티 바이너리 조정 모델

**옵션 A**: independent — 각 바이너리는 별도 apply 작업, 동시 작업 거부 (HTTP 409)
**옵션 B**: atomic group — 여러 바이너리를 단일 트랜잭션으로 update (모두 성공 또는 모두 rollback)

**권장**: **A** (independent, 직렬화)

WHY:
- 구현 단순성 (v0.1.0 의 단일 apply 흐름을 target 만 추가하여 재사용)
- B 는 분산 트랜잭션 / 2-phase commit 수준의 복잡도 (rollback orchestration, partial
  failure handling)
- 의존성 매트릭스 (M11) 가 운영자에게 어떤 순서로 update 해야 할지 안내 → atomic 그룹 update
  의 필요성 감소
- 향후 atomic group update 가 필요해지면 별도 SPEC-UPDATE-004 에서 검토

IMPACT: A 채택 시 v0.2.0 구현 범위 적정. B 채택 시 추가 30+ EARS 모듈 + orchestrator
계층 신규 + 6+ 추가 sentinel error 필요 → v0.2.0 범위 초과.

#### Decision Point 4: 공개키 모델

**옵션 A**: 단일 키 — 모든 바이너리 동일 Ed25519 공개키
**옵션 B**: 바이너리별 분리 — 각 바이너리(xflowd, xflow, xflow-agent) 별도 공개키

**권장**: **A** (단일 키)

WHY:
- 운영 편의 (단일 키 회전 정책, 단일 비공개키 관리)
- 단일 신뢰점 = 단일 보안 boundary (분산 키는 공격 표면 증가)
- 키 회전 시에도 모든 바이너리 동시 재서명 → manifest 일관성 자연스러움
- B 옵션은 키 분리의 보안 이점이 명확하지 않으면서 운영 부담만 증가

IMPACT: A 채택 시 v0.1.0 의 keys.go 그대로 활용. B 채택 시 keys.go 에 binary→key 매핑 추가
+ build pipeline 변경 + manifest 형식 확장 필요.

#### Decision Point 5: 롤백 범위

**옵션 A**: 단일 바이너리 — 자동 rollback 은 실패한 바이너리만 (target 별 독립)
**옵션 B**: 그룹 전체 — 한 바이너리 실패 시 다른 모든 바이너리 함께 rollback

**권장**: **A** (단일 바이너리)

WHY:
- Decision Point 3 의 권장값 (independent) 과 일관
- 각 바이너리는 독립된 .previous 백업을 가짐 → 단일 바이너리 rollback 자연스러움
- B 는 atomic group update 가 전제이므로 본 SPEC 범위 외

IMPACT: A 채택 시 v0.1.0 의 rollback.go 를 target 별로 호출 (변경 거의 없음). B 채택 시
group rollback orchestrator 필요.

### 에러 모델 변경

신규 sentinel error (3종):

- `ErrUpdateRestartFailed`: graceful drain timeout 또는 syscall.Exec 실패 (M3)
- `ErrUpdateHealthCheckFailed`: 새 바이너리 self-probe 실패 (M4-M5)
- `ErrUpdateIncompatibleVersion`: manifest.json 의 compat 제약 위반 (M11)

기존 9종 (v0.1.0) 은 그대로 유지.

---

## 관련 SPEC (Related SPECs)

- **SPEC-UPDATE-001 v0.1.0** (전제 조건, 구현 완료): 5 REST API + 9-state machine + 8 internal
  /updater 컴포넌트. 본 SPEC 은 이를 확장하며 BREAKING 변경 없음.
- **SPEC-WEB-006 v0.1.0** (전제 조건, 구현 완료): SystemVersionCard + Update Dialog +
  Header Badge + 5 systemUpdate API hooks. 본 SPEC 의 M-2 가 이 SPEC 의 채널 변경 UI 를
  활성화한다 (SPEC-WEB-006 v0.2.0 trigger).
- **SPEC-CLI-001 / -002 / -003**: CLI 명령 패턴 (`--auto-restart`, `--target` flag 추가)
- **SPEC-API-001**: REST API 패턴 (신규 엔드포인트 일관성)
- **SPEC-CFG-001**: 설정 시스템 (신규 필드 + 호환성 검증)
- **SPEC-OBS-001 / -002**: 구조화 로깅 (신규 이벤트 `update.exec`, `update.auto_rollback`,
  `update.channel_change`)
- **SPEC-LIFE-001**: 런타임 생명주기 (graceful drain 패턴 재사용)
- **SPEC-UPDATE-003 (예정)**: Windows 지원 (`MoveFileEx` + Service Control Manager)
- **SPEC-UPDATE-004 (예정)**: Atomic group update (Decision Point 3 의 옵션 B)
- **SPEC-WEB-007 (예정)**: 정식 RBAC (admin 권한 외 fine-grained role)

---

## TAG Traceability

- `@SPEC:SPEC-UPDATE-002` → spec.md (이 문서)
- `@PLAN:SPEC-UPDATE-002` → plan.md
- `@ACCEPTANCE:SPEC-UPDATE-002` → acceptance.md
- 구현 경로 (예정):
  - `internal/updater/restart_orchestrator.go` (NEW, M-1 wiring: drain → exec → self health
    check → auto rollback)
  - `internal/updater/manifest.go` (NEW, M-3 manifest.json 다운로드 + semver 검증)
  - `internal/updater/errors.go` (modified, +3 sentinel errors)
  - `internal/updater/types.go` (modified, +2 OperationStatus + ApplyOptions 필드)
  - `internal/updater/manager.go` (modified, target / auto_restart 옵션 통합)
  - `internal/updater/checker.go` (modified, manifest.json 다운로드 추가)
  - `internal/api/handler/system_update.go` (modified, +auto_restart + target 필드)
  - `internal/api/handler/system_update_channel.go` (NEW, GET/PUT /update/channel)
  - `internal/api/dto/update.go` (modified, +ChannelGetResponse, ChannelPutRequest/Response)
  - `internal/api/router.go` (modified, 채널 라우트 등록)
  - `internal/config/update.go` (modified, +health_check_interval, +health_check_endpoint,
    +target_binaries)
  - `cmd/xflowd/update.go` (modified, +--auto-restart, +--target flags)
  - `web/src/components/system/ChannelChangeDialog.tsx` (NEW, M-8 confirmation dialog)
  - `web/src/components/system/SystemVersionCard.tsx` (modified, 채널 dropdown 활성화)
  - `web/src/lib/api/systemUpdate.ts` (modified, +useChannelGet, +useChannelPut hooks)
  - 테스트: 위 각 신규/수정 파일 대응 `_test.go` + `web/src/components/__tests__/`

---

## Implementation Notes

### v0.1.0 → v0.2.0 마이그레이션

사용자 작업: **불필요** (default 값으로 v0.1.0 동작 보존).

운영자 권장 작업:

1. yaml config 의 `health_check_timeout` 을 `30s` 로 명시 설정 (default 변경 영향 회피)
2. `target_binaries` 를 yaml 에 명시 (확장 시점 통제)
3. CI/CD 스크립트가 PUT `/update/channel` 사용 시 admin 권한 토큰 사용 확인

### Future Extensions (본 SPEC 범위 외)

- **Atomic group update**: 여러 바이너리 동시 트랜잭션 (SPEC-UPDATE-004)
- **Windows 지원**: `MoveFileEx` + SCM 통합 (SPEC-UPDATE-003)
- **Per-binary key model**: 키 분리 마이그레이션 (필요 시 별도 SPEC)
- **Pre/post-update hooks**: 외부 명령 실행 (yaml 마이그레이션 스크립트 등)
- **Telemetry**: 익명 update 결과 통계 (privacy SPEC 별도)
- **Differential update**: bsdiff 기반 부분 패치 (현재 우선순위 낮음)
- **Update window**: 운영 시간 외 자동 적용 (cron-like 표현)
