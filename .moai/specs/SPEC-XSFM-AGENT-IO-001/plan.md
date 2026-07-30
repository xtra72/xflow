# SPEC-XSFM-AGENT-IO-001 — Implementation Plan

> 상위 SPEC: `spec.md` (RD-1 수신 forward 옵션, RD-2 상태 방출 모드, RD-3~6 확정). Tier M.

> **버전 노트**: v0.2.0 (2026-07-30) — 열린 질문 OQ-1~4 가 확정 설계 RD-3~6 으로 승격됨. RD-3 interval 기본 60s, RD-4 forward mode-agnostic, RD-5 메시지 shape 확정, RD-6 프런트(M5) 본 SPEC 포함 확정. 마일스톤·리스크에 반영.
>
> **버전 노트**: v0.3.0 (2026-07-30) — **상태 `completed`(구현 M1~M5 완료 + sync)**. 백엔드 M1~M4(`5fc11209`) + 프런트 M5(`6fb084c3`) 완료. M1~M4(config·enum 검증·emit_mode.go 수신 forward + 주기 스냅샷·on-change 게이팅) + M5(agentSchemas.ts 3개 컨트롤) 전부 구현. 기본 config byte-identical 무회귀, xsfm 커버리지 89.3%, `-race` 클린, `go test ./...` exit 0(42 pkgs), 프런트 vitest 2287. as-implemented 분기 4건은 spec.md §7 기록.

## 1. 기술 접근 (Technical Approach)

### 1.1 핵심 전략 — 가산 방출 레이어

두 기능 모두 기존 방출 경로를 **재작성하지 않고** 가산한다. forward 탭은 `ingestState`(direct·port 공유 mode-agnostic 시임)의 단일 지점에, 주기 스냅샷은 `startOfflineMonitor` 를 미러한 신규 goroutine 으로 추가한다. 기본 설정(forward off, mode=event)은 현행 방출과 **바이트 동일**(무회귀)을 최우선 불변식으로 삼는다.

### 1.2 ticker 패턴 재현 (monitor.go 미러)

주기 스냅샷 방출기는 `startOfflineMonitor`(monitor.go:15)를 그대로 미러한다: `minStateEmitInterval` 하한 가드 → `monitorWG.Add(1)` → goroutine 내 `time.NewTicker` + `select{stopCh, t.C}` + defer `t.Stop`. 방출은 `checkOfflineDevices` 처럼 **락 하 스냅샷(ListDevices) → 락 해제 → 송신** 순서로, 채널 송신 중 락을 잡지 않는다. `Stop` 의 기존 `monitorWG.Wait()` 가 두 goroutine 종료를 함께 커버한다.

### 1.3 방출 shape 재사용

`device_state_received` 는 `emitStateChanged` 조립을 재사용(타입만 교체, changed_fields 생략). `device_state_snapshot` 은 `deviceStateJSON`/`handleRequestState` 전체 응답 shape 를 재사용(각 디바이스 항목 동형). 하류 influx 태그(station_code/place_code/device_index/attribute)와 timestamp 규약을 그대로 계승한다.

### 1.4 설정 검증 패턴 재현

`state_emit_mode` enum 검증은 `transport_mode`/`liveness_source` switch 패턴을 동형 재현(오타 조기 노출). `state_emit_interval` 은 `parseDurationOpt`(음수 거부) 재사용 + min-interval 클램프.

## 2. 우선순위 마일스톤 (의존성 순서)

### M1 — 설정 파싱·검증 (Priority High · 선행)

- `XSFMConfig` 에 `ForwardReceivedToNode`/`StateEmitMode`/`StateEmitInterval` 필드 추가(config.go).
- 상수 `stateEmitModeEvent|Interval|Both`, 센티널 `ErrInvalidStateEmitMode`(errors.go).
- `parseXSFMConfig`: 3개 필드 파싱 + enum 검증 + interval 기본값(60s, RD-3)/음수 거부/하한 클램프.
- 산출물: config.go, errors.go. 테스트: 파싱·기본값·enum 위반·음수 거부.

### M2 — 수신 forward 옵션 (Priority High · M1 의존)

- `emitStateReceived`(agent.go 또는 신규 emit_mode.go) 추가.
- `ingestState` 말미에 `if a.cfg.ForwardReceivedToNode { emitStateReceived(...) }` (on-change 게이트와 독립, 매 수신).
- **mode-agnostic(RD-4 확정)**: `ingestState` 단일 시임 배치로 direct·port 양 모드 자동 커버 — 모드별 분기 없음. `device_state_received` 는 단일 디바이스 shape(RD-5), `changed_fields` 생략.
- 산출물: agent.go(+emit_mode.go). 테스트: forward on(무변경 포함 매 수신 방출)·off(미방출)·direct/port 양 모드 방출.

### M3 — 상태 방출 모드 & 주기 방출기 (Priority High · M1 의존)

- `emitStateSnapshot` + `startStateEmitter`(신규 emit_mode.go, startOfflineMonitor 미러).
- `Start` 에 `startStateEmitter()` 배선.
- `ingestState` on-change 게이팅: `event`/`both` 만 방출, `interval` 억제.
- 전이 이벤트(device_online/offline)는 게이팅 제외(불변).
- `device_state_snapshot` 은 단일 `{timestamp, devices:[...]}` 배열 메시지(RD-5), 기본 주기 60s(RD-3).
- 산출물: emit_mode.go, agent.go. 테스트: event/interval/both 각 거동, 주기 풀 스냅샷(offline 포함, 단일 배열 shape), 억제 검증.

### M4 — 생명주기·동시성·무회귀 테스트 (Priority High · M2·M3 의존)

- ticker goroutine 누수 없음(`Stop` 후 `monitorWG.Wait` 반환), `-race` 클린.
- 기본 설정 바이트 동일 무회귀(신규 메시지 미방출, on-change 불변).
- 산출물: 테스트. 커버리지 85%+ 확인.

### M5 — 프런트엔드 (Priority Low · 본 SPEC 범위 내(RD-6) · M1~M4 백엔드 의존)

- 에이전트 설정 UI 에 3개 옵션 토글/입력(`forward_received_to_node` 토글 + `state_emit_mode` 선택 + `state_emit_interval` 입력) — RD-6 확정으로 본 SPEC 포함.
- 값은 `Transport.Options` 로 저장·전달, 기존 UI 무회귀 유지(가산 UI).
- 산출물: web/src/*. 테스트: vitest.

## 3. 아키텍처 설계 방향

- 신규 파일 `emit_mode.go`: `startStateEmitter`/`emitStateSnapshot`/`emitStateReceived`/`minStateEmitInterval` 집약(monitor.go 와 대칭 구조).
- `ingestState` 변경은 말미 2개 훅(forward 방출 + on-change 게이팅)에 국한 — 반영·비교 로직 불변.
- `msgCh`/`sendEvent`/`ReceiveMessage` 계약 불변 — 신규 메시지도 동일 채널·non-blocking·drop 규약.

## 4. 리스크 및 대응

| 리스크                                       | 영향  | 대응                                                                   |
| ----------------------------------------- | --- | -------------------------------------------------------------------- |
| 주기 방출기 goroutine 누수/타이머 누수                  | 중   | monitorWG 재사용 + stopCh, `Stop` 후 Wait 반환 테스트, `-race`                  |
| interval 모드에서 on-change 억제 누락(중복 방출)        | 중   | 게이팅 단위테스트(interval 시 device_state_changed 0건)                          |
| forward 를 원시 바이트로 오방출                       | 중   | shape 단위테스트(deviceStateJSON 형태 검증, changed_fields 부재)                  |
| 기본 설정 회귀(신규 메시지 누출)                         | 높   | 기본 설정 무회귀 골든 테스트(event+forward off → 신규 타입 0건, on-change 바이트 동일)        |
| 채널 송신 중 락 보유 → deadlock/경합                  | 높   | ListDevices 스냅샷-후-방출 패턴(monitor.go 계승), `-race`                        |
| ~~OQ 미확정 상태 구현 착수~~ (해소됨)                  | -   | OQ-1~4 가 RD-3~6 으로 확정(v0.2.0) — interval 60s·mode-agnostic forward·메시지 shape·프런트 M5. 미해결 OQ 없음 |

## 5. TRUST 5 준수

- **Tested**: xsfm 커버리지 85%+, `-race` 클린, event/interval/both·forward on/off·무회귀·ticker 경합 시나리오.
- **Readable**: emit_mode.go 신설로 방출 로직 응집, 기존 주석 규약(한국어) 계승.
- **Unified**: gofmt/goimports, transport_mode/monitor.go 패턴 동형.
- **Secured**: 입력 검증(enum·duration), 신규 외부 입력 경계 없음(방출 계층만).
- **Trackable**: SPEC-XSFM-AGENT-IO-001 참조 커밋, REQ ID 매핑.

## 6. 완료 정의 (Definition of Done)

- REQ-01~04 구현 완료. REQ-05(프런트 M5)는 RD-6 확정으로 본 SPEC 범위 내(저우선, 백엔드 완료 후).
- 기본 설정 무회귀(바이트 동일) 검증.
- event/interval/both 3모드 + forward on/off 인수 시나리오 green(acceptance.md).
- 커버리지 85%+, `-race` 클린, `go test ./...` exit 0.
- MQTT 규약 불변 확인.
