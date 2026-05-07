---
spec_id: SPEC-UPDATE-002
version: 0.1.0
status: draft
updated: 2026-05-06
---

# SPEC-UPDATE-002: 수용 기준 (Acceptance Criteria)

## v0.1.0 Note

본 SPEC 는 SPEC-UPDATE-001 v0.1.0 의 3가지 한계 (in-process restart 미지원, 채널 REST API 부재, 단일 바이너리만) 를 보완하는 v0.2.0 진화이다. 14 EARS 모듈 (M1~M14) 을 3 그룹 (A: in-process restart, B: 채널 API, C: 멀티 바이너리) 으로 분류하여 검증한다.

전제: SPEC-UPDATE-001 v0.1.0 + SPEC-WEB-006 v0.1.0 모두 main 머지 완료 상태.

---

## Group A: In-Process Restart (Scenario 1-4)

### Scenario 1: auto_restart 정상 흐름 (M1, M2, M3, M4)

**Given**
- xflowd v0.3.0 운영 중 (admin 인증된 운영자)
- 채널 stable 에 v0.4.0 배포됨 (서명/체크섬 모두 정상)
- 디스크 여유 충분, 네트워크 정상

**When**
- 운영자가 `POST /api/v1/system/update/apply { auto_restart: true }` 호출
- 또는 CLI: `xflowd update apply --auto-restart --yes`

**Then**
- 응답 즉시 반환: `{ operation_id, status: "starting", from_version: "v0.3.0", to_version: "v0.4.0" }`
- 백그라운드 흐름:
  1. checking → downloading → verifying → applying (기존 v0.1.0 흐름)
  2. atomic replace 완료 시 status 가 `restarting` 으로 전환
  3. graceful drain (HTTP server, 30s timeout 기본)
  4. syscall.Exec 으로 새 바이너리 부팅
  5. 새 프로세스에서 status 가 `health_checking` 으로 전환
  6. HealthChecker.WaitHealthy(timeout=30s, interval=1s) 가 새 /api/v1/system/version 응답 200 + version=v0.4.0 확인
  7. status 가 `completed` 로 전환, completed_at 기록
- 운영자 후속 조회: `GET /update/status` → `{ status: "completed", to_version: "v0.4.0", ... }`
- `GET /system/version` → version=v0.4.0 반환

---

### Scenario 2: health check 실패 → 자동 롤백 성공 (M5)

**Given**
- xflowd v0.3.0 운영 중
- v0.4.0 적용 중 (apply 후 새 프로세스 시작했으나 의도적으로 health endpoint 응답 실패)
- auto_restart=true

**When**
- HealthChecker 가 30s timeout 동안 ready 응답 못 받음
- ErrUpdateHealthCheckFailed 트리거

**Then**
- status 가 `failed` 로 전환되기 전에 자동 rollback 시작
- rollback 흐름:
  1. ErrUpdateHealthCheckFailed 감지 → log "update.health_check_failed"
  2. .previous (v0.3.0) 백업 파일 존재 확인
  3. atomic replace로 v0.3.0 복원
  4. syscall.Exec 으로 v0.3.0 재부팅
  5. 두 번째 health check (rollback 검증) → 성공
  6. status 가 `failed` 로 전환 + error="health check failed, rolled back to v0.3.0"
- `GET /system/version` → version=v0.3.0 (롤백 완료)
- 운영자 알림: 토스트 또는 dashboard 에 "롤백 완료" 메시지

---

### Scenario 3: 두 번째 health check 실패 → ErrUpdateRollbackFailed (M5)

**Given**
- Scenario 2 흐름 진행 중
- 그러나 .previous (v0.3.0) 도 어떤 이유로 health check 실패 (예: 디스크 손상, 네트워크 단절)

**When**
- rollback 후 health check 도 실패

**Then**
- status 가 `failed` 로 전환 + error="rollback failed: previous binary also unhealthy"
- 운영자 개입 필수 알림 (CRITICAL log level + 다이얼로그/이메일 등)
- 추가 자동 롤백 시도 안 함 (loop 방지)
- `xflowd update rollback` CLI 가 마지막 수단 (수동 복원 + 진단)

---

### Scenario 4: graceful drain timeout → 강제 종료 (M2)

**Given**
- xflowd v0.3.0 운영 중, in-flight 장기 요청 다수 (예: 대용량 store query)
- v0.4.0 적용 시작, drain_timeout=30s

**When**
- atomic replace 완료 후 SIGTERM → drain 시작
- 30s 안에 모든 in-flight 요청 완료되지 않음

**Then**
- 30s 도달 시 강제 종료
- 손실 메시지 카운트 로그: "update.restart.drain_timeout count=N"
- syscall.Exec 정상 진행 (강제 종료 후에도 진행)
- 새 프로세스 시작 + health check 정상 → completed
- 손실 요청 수는 `GET /update/status` 응답의 `metadata.dropped_requests` 필드에 노출 (선택)

---

## Group B: 채널 REST API (Scenario 5-9)

### Scenario 5: GET /system/update/channel 정상 (M6)

**Given**
- 인증된 사용자 (admin 아님, viewer 또는 editor 도 가능)
- 현재 채널 stable

**When**
- `GET /api/v1/system/update/channel` 호출

**Then**
- 200 OK + `{ "current": "stable", "available": ["stable", "beta", "nightly"] }` 반환
- 인증 없으면 401

---

### Scenario 6: PUT /system/update/channel 정상 (M7)

**Given**
- admin 인증된 운영자
- 현재 채널 stable, 최신 버전 v0.4.0 (stable)
- beta 채널에 v0.5.0-beta.1 배포됨

**When**
- `PUT /api/v1/system/update/channel { channel: "beta" }` 호출

**Then**
- 응답: 200 OK + `{ previous: "stable", current: "beta", check_result: { current: "v0.4.0", latest: "v0.5.0-beta.1", available: true, ... } }`
- 부수 효과:
  - Configure() 내부 호출로 update.channel 변경
  - 즉시 채널 재검증 (Checker.Check) → check_result 응답 포함
  - 이후 useSystemVersion 폴링 시 update_available=true 반영
- yaml 영구 저장: 다음 시작 시에도 beta 유지

---

### Scenario 7: 잘못된 채널 → 400 (M7)

**Given**
- admin 인증된 운영자

**When**
- `PUT /api/v1/system/update/channel { channel: "experimental" }` 호출 (enum 외 값)

**Then**
- 응답: 400 Bad Request + `{ "error": "invalid channel: must be one of stable, beta, nightly" }`
- ErrUpdateChannelInvalid 매핑
- 채널 변경 안 됨 (기존 값 유지)

---

### Scenario 8: 비-admin PUT → 401 (M7, SPEC-WEB-006 M11)

**Given**
- editor 또는 viewer 권한 사용자 (admin 아님)

**When**
- `PUT /api/v1/system/update/channel` 시도

**Then**
- 401 Unauthorized (또는 403 Forbidden, 인증 모델에 따라)
- 채널 변경 안 됨
- 감사 로그 기록 (insufficient privileges 시도)

---

### Scenario 9: Web UI 채널 변경 다이얼로그 (M8)

**Given**
- admin 사용자가 `/admin/system` 페이지 접근
- SystemVersionCard 의 채널 표시 = "stable"

**When**
- 사용자가 채널 dropdown 클릭 → "beta" 선택 → ChannelChangeDialog 노출
- "beta 채널로 변경하시겠습니까? 다음 check 부터 적용됩니다" 확인
- "변경" 버튼 클릭

**Then**
- PUT /system/update/channel 호출
- 응답 success → SystemVersionCard 즉시 갱신:
  - 채널 배지: "stable" → "beta"
  - 업데이트 가능 인디케이터: 응답의 check_result.available=true 시 노란 dot
  - latest_version: "v0.5.0-beta.1"
- 토스트: "beta 채널로 변경 완료"

**When (실패 케이스)**
- 같은 흐름, 응답 401 (서버에서 권한 변경됨)

**Then**
- 토스트: "채널 변경에 실패했습니다 (권한 부족)"
- SystemVersionCard 채널 표시는 변경 안 됨
- mapUpdateError → unauthorized 매핑

---

## Group C: 멀티 바이너리 (Scenario 10-13)

### Scenario 10: target=xflow-agent 정상 적용 (M9, M10)

**Given**
- xflowd v0.3.0 운영 중, xflow-agent v0.3.0 도 별도 프로세스로 실행 중
- xflow-agent v0.4.0 배포됨

**When**
- `POST /update/apply { target: "xflow-agent", auto_restart: true }` 호출

**Then**
- xflowd 자체는 재시작 안 됨
- xflow-agent 흐름:
  1. xflowd 가 xflow-agent 의 binary path 조회 (config 또는 process discovery)
  2. download → verify → atomic replace
  3. graceful drain (xflow-agent 의 in-flight 메시지 처리)
  4. syscall.Exec 또는 supervisor 신호로 새 xflow-agent 부팅
  5. health check (xflow-agent 자체 health endpoint)
  6. completed
- `GET /update/status` → `{ target: "xflow-agent", to_version: "v0.4.0", status: "completed" }`

---

### Scenario 11: target=xflow CLI 정상 적용 (M9, M10)

**Given**
- xflow CLI 가 /usr/local/bin/xflow 에 설치됨
- v0.3.0 → v0.4.0 업데이트

**When**
- `POST /update/apply { target: "xflow", auto_restart: false }` (CLI 는 long-running 아니므로 auto_restart 무관)

**Then**
- xflow 흐름:
  1. download → verify → atomic replace
  2. graceful drain 단계 **건너뜀** (CLI 는 daemon 아님)
  3. health check **건너뜀** (CLI 는 자가 probe 안 함)
  4. status 가 즉시 completed
- 다음 `xflow` 명령 실행 시 새 v0.4.0 자동 사용
- 백업 파일 .previous 자동 생성 (롤백 가능)

---

### Scenario 12: 의존성 매트릭스 위반 → 거부 (M11)

**Given**
- xflowd v0.3.0 운영 중
- xflow-agent v1.0.0 배포됨 (release manifest 에 `compat: { xflowd: ">=v1.0.0" }` 명시)

**When**
- `POST /update/apply { target: "xflow-agent" }` 호출

**Then**
- Checker 가 manifest.json 다운로드 + 비교
- xflowd 현재 v0.3.0 < v1.0.0 → 비호환 감지
- 응답: 400 Bad Request + `{ "error": "incompatible: xflow-agent v1.0.0 requires xflowd >=v1.0.0 (current v0.3.0)" }`
- ErrUpdateIncompatibleVersion 매핑
- atomic replace 안 함 (배포 거부)
- 운영자에게 명시적 안내: "xflowd 도 같이 업데이트하거나 호환 버전 선택"

---

### Scenario 13: 동시 멀티 바이너리 작업 → 409 (M9, single-op-only)

**Given**
- xflowd 가 xflow-agent 업데이트 진행 중 (status: applying)

**When**
- 같은 운영자 또는 다른 운영자가 `POST /update/apply { target: "xflow" }` 호출

**Then**
- 409 Conflict + `{ "error": "another update operation in progress (operation_id: ..., target: xflow-agent)" }`
- v0.2.0 는 single-op-only (단일 바이너리든 멀티든 한 번에 하나만)
- 향후 SPEC (atomic group update) 에서 다중 동시 허용 가능

---

## Backward Compatibility Scenario

### Scenario 14: v0.1.0 backward — auto_restart 미지정 (M14)

**Given**
- 기존 v0.1.0 클라이언트 (CLI 또는 외부 도구)

**When**
- `POST /update/apply` (auto_restart 필드 없음, target 필드 없음)

**Then**
- target 기본값: "xflowd"
- auto_restart 기본값: false
- 흐름: v0.1.0 과 동일 (download → verify → apply → ready_to_restart 종료)
- 운영자 수동 재시작 (systemctl restart xflowd 등)
- 신규 11-state machine 의 `restarting` / `health_checking` 단계 건너뜀

---

## Edge Case Checklist (~40 항목)

### Group A: In-Process Restart Edge Cases

#### A.1 Drain 관련
- [ ] graceful drain 진행 중 신규 connection 거부 (HTTP server SIGTERM 핸들링)
- [ ] in-flight 요청 처리 시간 vs drain_timeout 비교 + 손실 카운트
- [ ] WebSocket connection 도 drain 대상 (chart-emitter, ChartChannel)
- [ ] long-poll 요청 (예: useUpdateStatus 1s polling) 도 안전하게 종료
- [ ] drain_timeout=0 설정 시 즉시 강제 종료 (테스트 모드)
- [ ] HTTP/2 multiplexed streams drain 처리

#### A.2 Exec 관련
- [ ] syscall.Exec 실패 (예: 새 바이너리 권한 0755 누락) → ErrUpdateRestartFailed
- [ ] syscall.Exec 성공 후 새 프로세스 즉시 panic → systemd 가 재시작 시도
- [ ] systemd Type=notify 통합 (READY=1 / STOPPING=1 시그널)
- [ ] PID 보존 검증 (exec 후 PID 동일)
- [ ] file descriptor 보존 (CLOEXEC=false 설정된 fd 만 보존)

#### A.3 Health Check 관련
- [ ] health endpoint timeout=30s 정확
- [ ] health endpoint 가 self-probe (localhost) 인지 외부 가능한지 (Decision)
- [ ] health 응답에 version 검증 필수 (v0.4.0 인지 v0.3.0 인지)
- [ ] interval=1s 폴링 부하 (총 30회 시도)
- [ ] 첫 health check 응답 후 즉시 completed (조기 성공)

#### A.4 Rollback 관련
- [ ] .previous 백업 파일 존재 검증 (없으면 ErrUpdateRollbackFailed 즉시)
- [ ] 두 번째 health check 실패 시 운영자 개입 알림 (이메일/webhook)
- [ ] rollback 후 .previous 파일 삭제 (loop 방지)
- [ ] rollback 후 새 update 시도 차단 (lock 파일 또는 status check)

### Group B: Channel REST API Edge Cases

- [ ] 채널 변경 즉시 yaml 영구 저장 (재시작 후에도 유지)
- [ ] 채널 변경 도중 에러 → 원자적 (변경 안 됨 OR 완전 변경)
- [ ] 채널 변경 직후 update_available 폴링 (60s) 내 반영
- [ ] 동시에 두 admin이 PUT 호출 → mutex 직렬화
- [ ] 채널 변경 시 update_url 재검증 (HTTPS 강제)
- [ ] PUT 응답의 check_result 가 캐시되지 않고 신규 호출 결과 반환

### Group C: Multi-Binary Edge Cases

- [ ] target=xflowd 와 target=xflow-agent 동시 진행 차단 (single-op)
- [ ] target=xflow-agent 업데이트 시 xflowd 의 process discovery 실패 → 명시적 에러
- [ ] xflow-agent 가 supervisor (예: docker, k8s) 로 관리되는 경우 syscall.Exec 대신 SIGTERM 후 supervisor 재시작 의존
- [ ] target=xflow CLI 업데이트 후 PATH 캐시 (shell hash) → 다음 명령 정상
- [ ] 의존성 manifest 가 없는 (legacy) 바이너리 → 호환성 검증 스킵 + 경고
- [ ] 의존성 manifest 의 semver 비교 (>=, <, ==, !=) 모두 지원
- [ ] 공개키가 모든 바이너리에 동일 (단일 키 모델) 검증
- [ ] xflow-agent 의 health endpoint 가 다를 때 (config 별 endpoint) 처리
- [ ] xflow-agent 가 여러 인스턴스 (예: agent-1, agent-2) 일 때 모두 업데이트 vs 일부만

### Cross-Cutting Edge Cases

- [ ] auto_restart=true + target=xflow CLI 조합 → CLI 는 long-running 아니므로 무시 (force 적용 X)
- [ ] auto_restart=true + 다운그레이드 + force=true → 다운그레이드 후 자동 재시작
- [ ] 채널 변경 + 즉시 update apply → 변경된 채널 기준으로 진행
- [ ] backward compat: v0.1.0 클라이언트가 신규 필드 (auto_restart, target) 무시 → default 동작
- [ ] 11-state machine 의 신규 상태 (restarting, health_checking) 가 v0.1.0 클라이언트 응답 처리 시 unknown → "in progress" 폴백
- [ ] 모든 신규 sentinel error (RestartFailed, HealthCheckFailed, IncompatibleVersion) 가 mapUpdateError 매핑
- [ ] frontend SystemVersionCard 의 채널 dropdown 이 admin 만 활성

---

## TRUST 5 품질 게이트

### T — Tested

- [ ] internal/updater/restart_orchestrator.go (NEW, M-1) 커버리지 ≥ 90%
- [ ] internal/updater/manifest.go (NEW, M-3 호환성) 커버리지 = 100% (보안 critical)
- [ ] internal/api/handler/system_update.go (modified, +channel + auto_restart + target) 신규 코드 ≥ 90%
- [ ] cmd/xflowd/update.go (modified, --auto-restart --target flags) ≥ 85%
- [ ] web/src/components/system/ChannelChangeDialog.tsx (NEW, M-8) ≥ 90%
- [ ] web/src/components/system/SystemVersionCard.tsx (modified, channel dropdown) 신규 코드 ≥ 90%
- [ ] 통합 테스트: 14 GWT 시나리오 모두 자동화 (10 backend + 4 frontend)
- [ ] 보안 테스트 추가: graceful drain race, health check 실패 시 race-free rollback, manifest 위조
- [ ] `go test -race ./...` 통과 (특히 in-process restart 흐름)
- [ ] frontend `npm test` 통과 (신규 ChannelChangeDialog + SystemVersionCard 진화)

### R — Readable

- [ ] 신규 함수 (restart_orchestrator.Apply, manifest.Validate 등) 에 한국어 docstring + `@spec SPEC-UPDATE-002 v0.1.0` traceability
- [ ] 신규 frontend 컴포넌트에 JSDoc + 한국어 주석
- [ ] 11-state machine 다이어그램 또는 표를 docs/updater-state-machine.md 에 추가
- [ ] `gofmt`, `goimports`, `prettier` 모두 통과

### U — Unified

- [ ] 신규 sentinel errors 가 기존 `Err*` 패턴 일관 (ErrUpdateRestartFailed, ErrUpdateHealthCheckFailed, ErrUpdateIncompatibleVersion)
- [ ] HTTP error response 포맷 일관 (`{ "error": "..." }` 또는 dto.NewErrorResponse)
- [ ] frontend 신규 컴포넌트가 기존 모달/다이얼로그 패턴 따름 (ConfirmDialog, ChannelChangeDialog 일관)
- [ ] manifest.json 형식이 기존 GitHub Releases 메타데이터와 호환

### S — Secured

- [ ] graceful drain 중 신규 connection 거부 (DoS 방어)
- [ ] syscall.Exec 직전 바이너리 검증 (Verifier.VerifyAll 재호출, TOCTOU 방어)
- [ ] manifest.json 도 Ed25519 서명 검증 대상 (위조 방지)
- [ ] 의존성 매트릭스 비교 시 ReDoS 안전 (semver 정규식 anchored)
- [ ] PUT /channel 미인증 호출 거부 (admin 권한)
- [ ] target 옵션 검증 (whitelist: xflowd, xflow-agent, xflow 만)
- [ ] xflow-agent 의 binary path 가 운영자 통제 (config) 만 허용 (path traversal 방어)
- [ ] 자동 rollback 시 race-free (mutex 보호)
- [ ] gosec 신규 issue 0건 (또는 의도된 설계만)

### T — Trackable

- [ ] `@spec SPEC-UPDATE-002 v0.1.0` 태그가 신규 production 파일 모두에 부착
- [ ] M1-M14 모듈 매핑이 코드 주석에 명시
- [ ] structured log: update.restart.{drain_start,drain_complete,exec_start,health_check_start,health_check_success,rollback_triggered}
- [ ] CHANGELOG.md 항목 작성 (BREAKING 아님, 추가 + 호환성 보존)
- [ ] 신규 메트릭: update_restarts_total, update_health_check_failures_total, update_rollbacks_triggered_total

---

## Definition of Done

본 SPEC v0.1.0 이 완료되려면 다음 조건을 모두 만족해야 한다.

### 1. 기능 완전성

- [ ] 14 EARS 모듈 (M1~M14) 모두 구현
- [ ] 14 GWT 시나리오 모두 자동화 테스트로 통과
- [ ] Edge case checklist 40+ 항목 검증
- [ ] backward compatibility 보존 (auto_restart 미지정 + target 미지정 시 v0.1.0 동작 그대로)

### 2. 품질 기준

- [ ] TRUST 5 모든 항목 통과
- [ ] `go test -race ./...` 전체 통과
- [ ] frontend `npm test` 전체 통과
- [ ] internal/updater/restart_orchestrator.go ≥ 90%, manifest.go = 100%
- [ ] frontend 신규 코드 ≥ 90%
- [ ] golangci-lint, eslint 0 issues (사전 존재 제외)

### 3. 보안 검증

- [ ] graceful drain race-free (race detector clean)
- [ ] syscall.Exec 직전 TOCTOU 방어 (Verifier.VerifyAll 재호출)
- [ ] manifest.json Ed25519 서명 검증
- [ ] 의존성 매트릭스 위반 시 ErrUpdateIncompatibleVersion 거부
- [ ] PUT /channel admin 권한 강제
- [ ] target whitelist (xflowd, xflow-agent, xflow) 만 허용
- [ ] gosec 신규 issue 0건

### 4. 운영 보장

- [ ] systemd Type=notify 통합 (READY=1 / STOPPING=1)
- [ ] in-process restart 실패 시 자동 rollback + 운영자 알림
- [ ] 두 번째 health check 실패 시 명시적 운영자 개입 (CRITICAL log)
- [ ] CHANGELOG 운영자 마이그레이션 가이드 (auto_restart 활성화 시 systemd 설정 등)

### 5. 문서화

- [ ] spec.md, plan.md, acceptance.md 3개 파일 일관 작성
- [ ] CHANGELOG.md v0.2.0 항목 추가 (Unreleased)
- [ ] docs/updater-state-machine.md (11-state 다이어그램) 작성
- [ ] 11-state machine 변경 사항 명시

### 6. 통합

- [ ] **SPEC-WEB-006 v0.2.0 동시 업데이트** — 채널 변경 dialog + target 옵션 UI 활성화
- [ ] 기존 SPEC-UPDATE-001 v0.1.0 기능 회귀 없음 (모든 v0.1.0 시나리오 통과)
- [ ] 기존 SPEC-WEB-006 v0.1.0 기능 회귀 없음 (System Status Panel 정상)

### 7. 릴리즈 준비

- [ ] 변경 요약 + 운영자 마이그레이션 가이드 (auto_restart 활성화 영향) 포함한 PR 생성
- [ ] 코드 리뷰 1회 이상 승인
- [ ] 병합 가능 상태 (merge-ready)
- [ ] stage 환경에서 in-process restart 검증 (real systemd + real binary 시뮬레이션)
- [ ] 운영팀 사전 공지 (auto_restart 옵트인 안내)

---

@spec SPEC-UPDATE-002 v0.1.0
@depends SPEC-UPDATE-001 v0.1.0, SPEC-WEB-006 v0.1.0
