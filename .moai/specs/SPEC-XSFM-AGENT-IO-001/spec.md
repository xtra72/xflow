---
id: SPEC-XSFM-AGENT-IO-001
title: "xsfm 에이전트 수신-전달 옵션 + 상태 방출 모드(event/interval/both)"
version: "0.3.0"
status: completed
created: 2026-07-30
updated: 2026-07-30
author: xtra
priority: P2
phase: "v0.6.0 target"
module: "internal/agent/xsfm"
lifecycle: spec-anchored
tier: M
tags: "xsfm, agent-io, forward, state-emit, emit-mode, interval, snapshot, ticker, heartbeat, config, ingestState, msgCh, node-receive, frontend, no-regression"
---

## HISTORY

| 날짜         | 버전    | 변경 내용                                                                 |
| ---------- | ----- | --------------------------------------------------------------------- |
| 2026-07-30 | 0.1.0 | 초기 SPEC 작성 — xsfm 에이전트에 **(1) 수신 파싱-상태 전달 옵션**과 **(2) 상태 방출 모드**를 도입. (1) `forward_received_to_node`(기본 `false`) 옵션: ON 이면 수신된 **모든** 디바이스 상태를 파싱/정규화된 형태(원시 브로커 바이트 아님)로 xsfm 노드에 전달하는 패스스루 탭(`device_state_received`) — 상태 **변경 여부와 무관**하게 매 유입마다 방출(변경시에만 방출하는 `device_state_changed` 와 구분). (2) `state_emit_mode` enum `{event, interval, both}`(기본 `event`, 무회귀): `event`=현행 on-change 방출, `interval`=`state_emit_interval` 주기 ticker 가 **전체 디바이스 상태 풀 스냅샷**(`device_state_snapshot`)을 방출하고 on-change 는 억제, `both`=on-change + 주기 스냅샷 heartbeat 병행. ticker 는 `startOfflineMonitor` 패턴 미러(min-guard·goroutine+stopCh·snapshot-후-락해제, 채널 송신 중 락 미보유). 확정 설계 RD-1/RD-2 반영. interval 기본값(60s)·port 모드 forward 거동·메시지 타입 네이밍 등 열린 질문(OQ) 기재. |
| 2026-07-30 | 0.2.0 | 사용자 확정으로 열린 질문 OQ-1~4 를 확정 설계 RD-3~6 으로 승격하고 §6 Open Questions 를 제거(미해결 OQ 없음). (RD-3) `state_emit_interval` 기본값 **60s** 확정(offline_timeout 기본 90s 와 정합), min-interval 가드는 `minMonitorInterval` 미러. (RD-4) `forward_received_to_node` 는 **mode-agnostic** — `ingestState` 단일 시임에서 direct·port **양 모드** 적용, 특별 케이스 없음(정규화 상태 형태이므로 port 모드 에코 위험 없음). (RD-5) 메시지 타입/shape 확정: forward = `device_state_received`(단일 디바이스 파싱 상태), 주기 = `device_state_snapshot` 단일 `{timestamp, devices:[...]}` 배열 페이로드(request_state / deviceStateJSON shape 재사용, per-device N 메시지 아님). (RD-6) 프런트엔드(forward_received_to_node·state_emit_mode·state_emit_interval 설정 토글 UI)를 본 SPEC 저우선 마일스톤 **M5** 로 포함 확정. §4 config 표·메시지 shape·mode-agnostic forward, Module 1/2/3/5 EARS 요구사항 갱신. |
| 2026-07-30 | 0.3.0 | **구현 완료(M1~M5) 및 sync — status `draft→completed`**. 백엔드 M1~M4(`5fc11209`): config.go(forward_received_to_node·state_emit_mode enum·state_emit_interval 60s+min guard·ErrInvalidStateEmitMode), emit_mode.go(emitStateReceived 수신 forward 탭 + startStateEmitter/emitStateSnapshot 주기 풀 스냅샷, offline monitor 미러·monitorWG/stopCh 공유), ingestState on-change 게이팅(interval 억제·전이 이벤트 비게이팅). 기본 config byte-identical 무회귀, xsfm 커버리지 89.3%, `-race` 클린. 프런트 M5(`6fb084c3`): agentSchemas.ts XSFM_FIELDS 3개 컨트롤(forward 토글·mode select·interval duration, mode=interval\|both 시 표시), vitest 2287·tsc 클린. spec-anchored Level 2 규율에 따라 §7 Implementation Notes(as-implemented) 신설(분기 1~4). |

---

# SPEC-XSFM-AGENT-IO-001: xsfm 에이전트 수신-전달 옵션 + 상태 방출 모드

## 1. Environment (환경)

### 1.1 시스템 개요

xflow 는 Go 기반 IoT FBP 플랫폼이며, `internal/agent/xsfm` 는 지하철 역사(station) 설비를 MQTT 로 제어·모니터링하는 에이전트이다(SPEC-XSFM-001 에서 정의). 에이전트는 유입된 디바이스 상태를 인메모리 로스터에 반영하고, 상태가 **변경될 때만** `device_state_changed` 이벤트를 노드 방출 채널(`msgCh`)로 내보낸다. 노드(`xsfm` 노드)는 `agent.MessageReceiver.ReceiveMessage` 로 `msgCh` 를 drain 하여 flow 로 전달한다.

본 SPEC 은 이 방출 모델 위에 **두 가지 가산 기능**을 도입한다.

1. **수신 파싱-상태 전달 옵션(RD-1)**: 수신된 **모든** 디바이스 메시지를 파싱/정규화된 상태 형태로 노드에 전달하는 옵션(패스스루 탭). 변경 여부와 무관하게 매 유입마다 방출하므로, "변경분만" 방출하는 `device_state_changed` 와 목적이 다르다.
2. **상태 방출 모드(RD-2)**: 상태를 (a) 변경 이벤트로만, (b) 설정 주기로만(전체 스냅샷), (c) 둘 다 방출하도록 선택하는 모드. 현재 방출은 **오직 이벤트 기반(on-change)** 이며 주기적 방출 경로가 없다.

### 1.2 기술 환경

- **언어/모듈**: Go 1.23+ / `internal/agent/xsfm/` (기존 패키지 확장)
- **프런트엔드**: React + TypeScript (`web/src/`) — 에이전트 설정 UI(옵션 토글)
- **관련 기존 anchor (본 SPEC 이 정확히 참조·재사용)**:
  - Transport 모드: `config.go` `TransportMode` = `direct`(에이전트가 브로커 소유, `subscribeState`→`handleStateMessage`→`ingestState`) | `port`(외부 노드 I/O, 노드가 `FeedState`→`ingestState` 호출). (config.go:56, agent.go:411/424/737)
  - **mode-agnostic 시임**: `agent.go` `ingestState(composite, fields, st)`(agent.go:468) — direct·port 양 경로가 공유하는 단일 상태 반영 지점. 락 하에서 이전/신규 비교로 `changed` 산출, 방출 스냅샷을 뜬 뒤 락 해제하고 방출(락을 채널 송신에 걸쳐 잡지 않음).
  - **현행 방출(EVENT-DRIVEN ONLY)**: `ingestState` 말미의 `if len(changed) > 0 { a.emitStateChanged(...) }`(agent.go:554-556), `emitStateChanged`(agent.go:690, `device_state_changed` 메시지 조립 + `sendEvent`), `sendEvent`(control.go:13, `msgCh` non-blocking 송신, 가득 차면 drop), `msgCh`(agent.go:79, 버퍼 256).
  - **노드 수신**: `ReceiveMessage`(agent.go:901, `msgCh` drain, `MessageReceiver` 구현).
  - **상태 JSON shape**: `status.go` `deviceStateJSON(d)`(status.go:37, device_id/online + group_id(설정 시) + 관측 축 `StateForJSON`), `handleRequestState`(status.go:13, 전체 로스터 스냅샷 `{status, devices:[...]}` 반환) — `device_state_changed` emit 과 shape 일관.
  - **ticker 패턴 참조(본 SPEC 의 주기 방출기가 미러)**: `monitor.go` `startOfflineMonitor`(monitor.go:15, `minMonitorInterval` 하한 가드 + `monitorWG.Add(1)` + goroutine 내 `time.NewTicker` + `select{stopCh, t.C}` + defer `t.Stop`), `checkOfflineDevices`(monitor.go:45, 락 하 스냅샷·전환 후 **락 해제하고 방출**).
  - **로스터 스냅샷**: `agent.go` `ListDevices()`(agent.go:918, RLock 하 정렬 값 복사본 반환) — 주기 스냅샷 방출기가 재사용.
  - **생명주기**: `Start`(agent.go:383, `startOfflineMonitor()` 호출), `Stop`(agent.go:769, `close(stopCh)` 후 `monitorWG.Wait()`), `stopCh`/`monitorWG`(agent.go:80/84).
  - **설정 파싱**: `config.go` `parseXSFMConfig(opts)`(config.go:140, `Transport.Options` 맵 파싱·검증), enum 검증 패턴(`transport_mode` switch, config.go:164-169), `parseDurationOpt`(config.go:489, duration 문자열 파싱, 음수 거부).
  - 센티널 에러: `errors.go` (`ErrInvalidTransportMode`, `ErrInvalidLivenessSource` 등 enum 검증 에러 패턴).
- **테스트**: Go 표준 `testing` + `testify`; 프런트 `vitest`

### 1.3 설계 원칙

- **비침습 가산 레이어(핵심)**: 두 기능 모두 기존 방출 경로를 **재작성하지 않고** 가산한다. 기본 설정(forward off, mode=event)은 현행 동작과 **바이트 동일**(무회귀)해야 한다.
- **mode-agnostic 재사용**: forward 탭은 `ingestState`(direct·port 공유 시임)에 위치시켜 단일 지점으로 구현한다. 방출 shape 는 `deviceStateJSON`/`StateForJSON` 을 재사용해 `device_state_changed`·`request_state` 와 일관성을 유지한다.
- **ticker 규율 계승**: 주기 방출기는 `startOfflineMonitor` 를 그대로 미러한다 — min-interval 하한 가드, 전용 goroutine + `stopCh` 종료, `monitorWG` 로 누수 방지, **로스터 스냅샷을 락 하에서 뜬 뒤 락을 해제하고 방출**(프로젝트 RWMutex 재귀 deadlock 트랩 및 채널 송신 중 락 보유 금지).
- **MQTT 규약 불변**: 두 기능 모두 노드 방출(`msgCh`) 계층에만 작용하며 MQTT 토픽/페이로드 규약은 불변이다.

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - `forward_received_to_node` 옵션(기본 false) + `device_state_received` 방출(파싱 상태 패스스루 탭)
  - `state_emit_mode` enum `{event, interval, both}`(기본 event) + `state_emit_interval` duration
  - 주기 스냅샷 방출기(신설 goroutine, `startOfflineMonitor` 미러) + `device_state_snapshot` 방출(전체 등록 디바이스 풀 스냅샷)
  - on-change 방출의 모드 게이팅(interval 모드에서 억제)
  - 설정 파싱·검증(enum 검증, interval min-guard·기본값)
  - 프런트: 에이전트 설정 UI 옵션 토글(RD-6 확정, 저우선 M5)
- **범위 밖(Out-of-Scope)**:
  - MQTT 토픽/페이로드 규약 변경(불변)
  - `device_online`/`device_offline` 전이 이벤트 모델 변경(전이 이벤트는 상태 방출 모드와 무관하게 항상 방출 — §4.4)
  - `request_state`(pull 형) 명령 자체의 변경(스냅샷 shape 재사용만)
  - 노드/flow 측의 새 메시지 타입 소비 로직 재설계(에이전트는 방출만 담당; 소비 형태는 RD-5 shape 계약을 따름)
  - fan-out/제어 경로 변경

## 2. Assumptions (가정)

- **A-1**: `ingestState` 는 direct·port 양 경로가 공유하는 mode-agnostic 시임으로 유지된다. forward 탭을 이곳에 두면 두 경로에 자동 적용된다.
- **A-2**: `msgCh` 는 양 모드에서 노드(`ReceiveMessage`)에 의해 drain 된다. 신규 방출 메시지(`device_state_received`/`device_state_snapshot`)는 기존 `sendEvent` 경로(non-blocking, 가득 차면 drop)를 재사용한다.
- **A-3**: `ListDevices()` 는 RLock 하에서 정렬된 값 복사본을 반환하므로, 주기 스냅샷 방출기는 이를 호출해 락을 해제한 상태에서 메시지를 조립·송신할 수 있다(채널 송신 중 락 미보유).
- **A-4**: 기본 설정(forward off, mode=event)에서는 주기 방출기 goroutine 이 기동되지 않으며, on-change `device_state_changed` 방출이 현행과 동일하게 동작한다.
- **A-5**: `state_emit_interval` 은 mode 가 `interval`/`both` 일 때만 의미가 있으며, `event` 모드에서는 무시된다.

## 3. Requirements (EARS 요구사항)

### Module 1 — 수신 파싱-상태 전달 옵션 (REQ-01)

- **REQ-01-01** (Ubiquitous): 시스템은 항상 `forward_received_to_node` 옵션을 boolean 으로 취급하고 기본값을 `false` 로 유지해야 한다.
- **REQ-01-02** (State): IF `forward_received_to_node` 가 `true` THEN 시스템은 수신·파싱된 **모든** 디바이스 상태를 파싱/정규화된 형태(원시 브로커 바이트 아님)로 노드에 전달(`device_state_received` 방출)해야 하며, 이는 상태 **변경 여부와 무관**하게 매 유입마다 발생해야 한다.
- **REQ-01-03** (Unwanted): 시스템은 `forward_received_to_node` 방출을 원시 브로커 바이트로 수행해서는 안 된다 — 반드시 파싱/정규화된 디바이스 상태 형태(`deviceStateJSON` shape 재사용)여야 한다.
- **REQ-01-04** (Ubiquitous): 시스템은 항상 `device_state_received` 방출을 `device_state_changed`(변경분 방출)와 **구분되는 별도 메시지 타입**으로 취급해야 하며(패스스루 탭 vs 변경 이벤트), 이를 **단일 디바이스** 파싱 상태 메시지로 방출해야 한다(RD-5, 배열이 아닌 유입 디바이스 1건).
- **REQ-01-05** (Ubiquitous): 시스템은 항상 forward 탭을 `ingestState`(direct·port 공유 시임) 단일 지점에 위치시켜 **mode-agnostic** 하게(direct·port 양 모드에 동일 적용) 동작시켜야 하며, 모드별 특별 케이스(no-op-in-port 등)를 두어서는 안 된다(RD-4). 방출 형태는 파싱/정규화된 상태이므로 port 모드에서도 원시 바이트 에코가 아니다.
- **REQ-01-06** (Unwanted): 시스템은 `forward_received_to_node` 방출을 `state_emit_mode` 값에 종속시켜서는 안 된다 — forward 는 방출 모드와 독립적으로 매 수신마다 동작한다.

### Module 2 — 상태 방출 모드 (REQ-02)

- **REQ-02-01** (Ubiquitous): 시스템은 항상 `state_emit_mode` 를 enum `{event, interval, both}` 로 취급하고 기본값을 `event` 로 유지해야 한다.
- **REQ-02-02** (State): IF `state_emit_mode` 가 `event` THEN 시스템은 현행과 동일하게 상태 **변경 시에만** `device_state_changed` 를 방출해야 하며(무회귀), 주기 방출기를 기동하지 않아야 한다.
- **REQ-02-03** (State): IF `state_emit_mode` 가 `interval` THEN 시스템은 `state_emit_interval` 주기(기본 60s, RD-3)로 **전체 등록 디바이스 상태 풀 스냅샷**(`device_state_snapshot`)을 **단일 `{timestamp, devices:[...]}` 배열 메시지**(RD-5)로 방출해야 하며, on-change `device_state_changed` 는 **억제**해야 한다.
- **REQ-02-04** (State): IF `state_emit_mode` 가 `both` THEN 시스템은 on-change `device_state_changed` 방출과 주기 풀 스냅샷 heartbeat 를 **병행** 방출해야 한다.
- **REQ-02-05** (Ubiquitous): 시스템은 항상 주기 스냅샷 대상 집합을 **모든 등록 디바이스**(풀 스냅샷)로 해야 하며, offline 디바이스도 `online:false` 로 포함해야 한다(include-all, §4.3). 주기 스냅샷은 per-device N개 개별 메시지가 아니라 **단일 배열 메시지**로 방출해야 한다(RD-5).
- **REQ-02-06** (Ubiquitous): 시스템은 항상 주기 스냅샷 방출 goroutine 을 `startOfflineMonitor` 패턴으로 관리해야 한다 — min-interval 하한 가드, 전용 goroutine + `stopCh` 종료, WaitGroup 로 종료 확인(누수 방지).
- **REQ-02-07** (Ubiquitous): 시스템은 항상 주기 스냅샷을 **로스터 스냅샷을 락 하에서 뜬 뒤 락을 해제하고** 방출해야 한다(채널 송신 중 락 미보유, monitor.go 규율 계승).
- **REQ-02-08** (State): IF `state_emit_mode` 가 `event` 이거나 `state_emit_interval <= 0` THEN 시스템은 주기 방출기 goroutine 을 기동하지 않아야 한다(3-way: 0=비활성, monitor.go 계승).

### Module 3 — 설정 파싱·검증 (REQ-03)

- **REQ-03-01** (Event): WHEN `parseXSFMConfig` 가 `Transport.Options` 를 파싱 THEN 시스템은 `forward_received_to_node`(bool)·`state_emit_mode`(string enum)·`state_emit_interval`(duration)을 파싱해야 한다.
- **REQ-03-02** (Unwanted): 시스템은 `state_emit_mode` 가 `{event, interval, both}` 이외의 값이면 설정을 수락해서는 안 되며, `ErrInvalidStateEmitMode` 를 반환해야 한다(`transport_mode`/`liveness_source` enum 검증 패턴 동형 — 조용한 폴백보다 오타를 조기에 드러냄).
- **REQ-03-03** (State): IF `state_emit_interval` 이 미설정이면 THEN 시스템은 내부 기본값 **60s** 를 적용해야 한다(RD-3, offline_timeout 기본 90s 와 정합).
- **REQ-03-04** (Unwanted): 시스템은 `state_emit_interval` 이 음수이면 설정을 수락해서는 안 된다(`parseDurationOpt` 규약 계승).
- **REQ-03-05** (Ubiquitous): 시스템은 항상 유효 tick 주기가 하한(`minStateEmitInterval`) 미만이면 하한으로 올려 `time.NewTicker` panic 을 방지해야 한다(RD-3, monitor.go `minMonitorInterval` 미러).

### Module 4 — 비기능/무회귀 (REQ-04, NFR)

- **REQ-04-01** (Ubiquitous): 시스템은 항상 기본 설정(`forward_received_to_node=false`, `state_emit_mode=event`)에서 현행 방출 동작과 **바이트 동일**해야 한다(신규 메시지 미방출, on-change 경로 불변).
- **REQ-04-02** (Ubiquitous): 시스템은 항상 신규 goroutine(주기 방출기)을 `Start` 에서 기동하고 `Stop` 에서 `stopCh` 관측·WaitGroup 대기로 정리해 goroutine/타이머 누수를 방지해야 한다.
- **REQ-04-03** (Ubiquitous): 시스템은 항상 신규 방출 메시지를 기존 `sendEvent`(non-blocking, 가득 차면 drop) 경로로 송신해 `ReceiveMessage` 계약을 불변으로 유지해야 한다.
- **REQ-04-04** (Ubiquitous): 시스템은 항상 MQTT 토픽/페이로드 규약을 불변으로 유지해야 한다(두 기능은 `msgCh` 방출 계층에만 작용).
- **REQ-04-05** (Ubiquitous): 시스템은 항상 xsfm 패키지 테스트 커버리지 85% 이상 및 `-race` 클린을 유지해야 한다(TRUST 5 Tested).

### Module 5 — 프런트엔드 (REQ-05, 본 SPEC 범위 내 · 저우선 M5)

> RD-6 확정: 프런트엔드 설정 토글 UI 는 본 SPEC 범위에 포함(저우선 마일스톤 M5, 백엔드 M1~M4 완료 후).

- **REQ-05-01** (Ubiquitous): 프런트엔드는 에이전트 설정 UI 에서 `forward_received_to_node` 토글과 `state_emit_mode`(event/interval/both) 선택 및 `state_emit_interval` 입력을 제공해야 한다(RD-6).
- **REQ-05-02** (Ubiquitous): 프런트엔드는 항상 기존 에이전트 설정 UI 동작을 회귀 없이 유지해야 한다(가산 UI).
- **REQ-05-03** (Event): WHEN 사용자가 3개 옵션을 조작하면 THEN 프런트엔드는 해당 값을 `Transport.Options`(`forward_received_to_node`/`state_emit_mode`/`state_emit_interval`) 로 저장·전달해야 한다.

## 4. Specifications (사양)

### 4.1 설정 필드

| 필드                        | 타입        | 기본값     | 검증                                                        |
| ------------------------- | --------- | ------- | --------------------------------------------------------- |
| `forward_received_to_node`  | bool      | `false`   | 없음(불리언). ON 시 mode-agnostic(direct·port 양 모드) 적용(RD-4)      |
| `state_emit_mode`           | string enum | `event`   | `{event, interval, both}` 이외 → `ErrInvalidStateEmitMode`(REQ-03-02) |
| `state_emit_interval`       | duration  | `60s`(RD-3) | 음수 거부(`parseDurationOpt`), 유효 tick < `minStateEmitInterval` 이면 하한 클램프 |

`XSFMConfig`(config.go) 확장 필드 및 `parseXSFMConfig` 파싱 스케치:

```go
// XSFMConfig 확장 (config.go)
type XSFMConfig struct {
    // ... 기존 필드 ...

    // ForwardReceivedToNode 는 수신된 모든 파싱 상태를 노드로 전달(device_state_received)할지의
    // 패스스루 탭 옵션이다 (기본 false, RD-1). state_emit_mode 와 독립적으로 매 수신마다 방출된다.
    ForwardReceivedToNode bool

    // StateEmitMode 는 상태 방출 모드이다: "event"(기본, on-change) | "interval"(주기 풀 스냅샷,
    // on-change 억제) | "both"(on-change + 주기 스냅샷 heartbeat). (RD-2)
    StateEmitMode string

    // StateEmitInterval 은 interval/both 모드의 주기 스냅샷 방출 주기이다 (기본 60s, RD-3).
    // event 모드에서는 무시된다. <=0 이면 주기 방출기 미기동(3-way 비활성).
    StateEmitInterval time.Duration
}
```

```go
// parseXSFMConfig 파싱 스케치 (transport_mode 검증 패턴 동형)
cfg.StateEmitMode = stateEmitModeEvent            // 기본
cfg.StateEmitInterval = 60 * time.Second          // 기본(RD-3)

if v, ok := opts["forward_received_to_node"]; ok {
    if b, bok := v.(bool); bok { cfg.ForwardReceivedToNode = b }
}
if v, ok := opts["state_emit_mode"]; ok {
    s, sok := v.(string)
    if !sok { return XSFMConfig{}, fmt.Errorf("%w: state_emit_mode must be a string", ErrInvalidStateEmitMode) }
    if s != "" { cfg.StateEmitMode = s }
}
switch cfg.StateEmitMode {
case stateEmitModeEvent, stateEmitModeInterval, stateEmitModeBoth: // valid
default:
    return XSFMConfig{}, fmt.Errorf("%w: got %q", ErrInvalidStateEmitMode, cfg.StateEmitMode)
}
if d, ok, err := parseDurationOpt(opts, "state_emit_interval"); err != nil {
    return XSFMConfig{}, err
} else if ok {
    cfg.StateEmitInterval = d
}
```

상수(config.go): `stateEmitModeEvent = "event"`, `stateEmitModeInterval = "interval"`, `stateEmitModeBoth = "both"`.
신규 센티널 에러(errors.go): `ErrInvalidStateEmitMode = errors.New("xsfm: invalid state_emit_mode (must be 'event', 'interval', or 'both')")`.

### 4.2 forward 탭 (RD-1) — device_state_received

- 위치: `ingestState`(agent.go:468) 말미, on-change 게이트와 **독립**. 이미 락을 해제한 뒤 `stateAxes`/`groupID`/`emitTsMs`/`fields` 스냅샷이 준비된 지점(agent.go:526-556)에서 방출한다.
- **mode-agnostic(RD-4)**: `ingestState` 는 direct·port 양 경로가 공유하는 단일 시임이므로, forward 탭을 이 지점에 두면 두 모드에 **동일하게** 적용된다 — 모드별 분기(no-op-in-port 등)를 두지 않는다. 방출 형태가 파싱/정규화된 상태이므로 port 모드가 `FeedState` 로 넣은 원시 바이트와 중복(에코)이 아니며, 노드는 정규화 형태를 새로 획득한다.
- 조건: `if a.cfg.ForwardReceivedToNode { a.emitStateReceived(deviceID, groupID, newOnline, stateAxes, emitTsMs, fields) }` — `len(changed)` 와 무관(매 수신), `state_emit_mode` 와도 독립(REQ-01-06).
- shape(RD-5): `emitStateChanged` 조립을 재사용하되 **타입만 `device_state_received`** 로 하고 `changed_fields` 는 **생략**(패스스루는 "변경분" 개념이 없으므로). **단일 디바이스** 메시지 — `type/device_id/group_id(설정 시)/online/관측 축(StateForJSON)/timestamp` + meta placeholder(station_code/place_code/device_index/attribute).

```go
// emitStateReceived 는 수신된 파싱 상태를 device_state_received 로 노드에 전달한다 (RD-1).
// emitStateChanged 와 조립은 동일하되 타입이 다르고 changed_fields 를 싣지 않는다(패스스루 탭).
func (a *XSFMAgent) emitStateReceived(deviceID, groupID string, online bool, stateAxes map[string]any, timestampMs int64, meta map[string]string) {
    data := map[string]any{"device_id": deviceID, "online": online, "timestamp": timestampMs}
    if groupID != "" { data["group_id"] = groupID }
    for k, v := range stateAxes { data[k] = v }
    for k, v := range meta { /* device_id/충돌 키 skip, emitStateChanged 규약 동일 */ }
    a.sendEvent("device_state_received", data)
}
```

### 4.3 주기 스냅샷 방출기 (RD-2) — device_state_snapshot

- 신규 goroutine `startStateEmitter`(신규 파일 예: `emit_mode.go`), `Start`(agent.go:383)에서 `startOfflineMonitor()` 와 나란히 호출.
- 기동 조건(REQ-02-08): `state_emit_mode ∈ {interval, both}` **그리고** `state_emit_interval > 0`. 그 외에는 미기동.
- ticker 주기: `state_emit_interval` 을 **그대로** 사용한다(offline 모니터는 임계 감지를 위해 `timeout/4` 를 쓰지만, 여기서는 interval 이 곧 방출 주기이므로 나눔 없음). `minStateEmitInterval = time.Millisecond` 하한 가드.
- 누수 방지: `monitorWG` 재사용(`Stop` 이 이미 `monitorWG.Wait()` 로 대기 — 두 goroutine 을 함께 커버). `select{stopCh, t.C}` + defer `t.Stop`.
- 방출: `emitStateSnapshot()` — `ListDevices()`(RLock 하 정렬 복사본)로 로스터를 뜨고 락 해제 상태에서 `deviceStateJSON` 배열을 조립해 단일 메시지로 송신.

```go
const minStateEmitInterval = time.Millisecond

func (a *XSFMAgent) startStateEmitter() {
    if a.cfg.StateEmitInterval <= 0 { return }
    if a.cfg.StateEmitMode != stateEmitModeInterval && a.cfg.StateEmitMode != stateEmitModeBoth { return }
    interval := a.cfg.StateEmitInterval
    if interval < minStateEmitInterval { interval = minStateEmitInterval }

    a.monitorWG.Add(1)
    go func() {
        defer a.monitorWG.Done()
        t := time.NewTicker(interval)
        defer t.Stop()
        for {
            select {
            case <-a.stopCh:
                return
            case <-t.C:
                a.emitStateSnapshot()
            }
        }
    }()
}

// emitStateSnapshot 은 전체 등록 디바이스의 상태 풀 스냅샷을 device_state_snapshot 으로 방출한다.
// ListDevices 가 RLock 하에서 정렬 복사본을 반환하므로 락을 채널 송신에 걸쳐 잡지 않는다(REQ-02-07).
func (a *XSFMAgent) emitStateSnapshot() {
    devs := a.ListDevices() // RLock 하 스냅샷, 반환 시 락 해제됨
    out := make([]map[string]any, 0, len(devs))
    for i := range devs {
        out = append(out, deviceStateJSON(&devs[i])) // offline 포함(include-all, REQ-02-05)
    }
    a.sendEvent("device_state_snapshot", map[string]any{
        "timestamp": time.Now().UnixMilli(),
        "devices":   out,
    })
}
```

- **메시지 shape 결정(RD-5, 확정)**: 단일 `device_state_snapshot` 메시지에 `{timestamp, devices:[...]}` 배열(각 항목은 `deviceStateJSON` — `request_state` 전체 응답 shape 와 동형). N 개 per-device `device_state_changed` 를 개별 방출하는 대안은 **채택하지 않는다**(스냅샷은 배치/roster 개념).
- **빈 로스터 거동**: 등록 디바이스가 0개여도 `devices: []` 로 방출(heartbeat 성 liveness 유지).

### 4.4 on-change 게이팅

`ingestState` 말미의 on-change 방출을 모드로 게이팅한다.

```go
// 현행 (agent.go:554-556):
//   if len(changed) > 0 { a.emitStateChanged(...) }
// 신규:
if len(changed) > 0 && (a.cfg.StateEmitMode == stateEmitModeEvent || a.cfg.StateEmitMode == stateEmitModeBoth) {
    a.emitStateChanged(deviceID, groupID, newOnline, changed, stateAxes, emitTsMs, fields)
}
```

- `event`/`both`: on-change 방출(현행). `interval`: 억제.
- **전이 이벤트 불변(범위 밖)**: `device_online`/`device_offline`(agent.go:540-544, monitor.go, LWT)은 **상태 방출 모드와 무관하게 항상** 방출한다 — 이들은 주기적 상태 스트림이 아니라 생명주기 전이 이벤트이므로 게이팅 대상이 아니다. 이 결정은 확정 사양이며 OQ 가 아니다.

### 4.5 생명주기 & 락 규율

- `Start`: `startOfflineMonitor()` + `startStateEmitter()` 병렬 기동.
- `Stop`: `close(stopCh)` → `monitorWG.Wait()` 가 offline 모니터·상태 방출기 두 goroutine 종료를 함께 확인. 두 goroutine 모두 락을 채널 송신에 걸쳐 잡지 않으므로 `Wait` 가 데드락하지 않는다.
- 락 중첩 금지: `emitStateSnapshot` 은 `ListDevices()`(자체 RLock)만 사용하고 반환 후 방출한다. 로스터/pending/레지스트리 락과 중첩하지 않는다.

## 5. Resolved Decisions (확정된 설계 결정)

> 아래는 사용자 확정 사항이며 재검토하지 않는다.

- **RD-1 — 수신 파싱-상태 forward 옵션**: `forward_received_to_node`(기본 `false`) 옵션을 도입한다. ON 이면 수신된 **모든** 디바이스 메시지를 **파싱/정규화된 디바이스 상태 형태**(원시 브로커 바이트 아님)로 xsfm 노드에 전달한다. `device_state_changed`(상태 변경 시에만)와 구분되는 패스스루/탭 — 무변경 갱신을 포함해 매 유입마다 전달한다. `ingestState`(direct·port 공유 시임)에 위치하며 direct·port 양 모드에 동일 적용(mode-agnostic, RD-4).
- **RD-2 — 상태 방출 모드(event|interval|both)**: `state_emit_mode` enum `{event, interval, both}`(기본 `event`, 무회귀)을 도입한다. `event`=현행 on-change `device_state_changed`. `interval`=`state_emit_interval` 주기 ticker 가 **전체 등록 디바이스 상태 풀 스냅샷**을 방출하고 on-change 는 방출하지 않음. `both`=on-change 이벤트 + 주기 풀 스냅샷 heartbeat. ticker 는 `startOfflineMonitor` 를 미러(min-interval 가드, goroutine + stop 채널, snapshot-후-락해제 — 채널 송신 중 락 미보유). 주기 스냅샷 대상 = 전체 등록 디바이스(풀 스냅샷).
- **RD-3 — `state_emit_interval` 기본값 = 60s (OQ-1 확정)**: 미설정 시 기본 주기를 **60s** 로 확정한다. offline_timeout 기본값 90s 와 정합적이며 과도한 방출 트래픽을 회피한다. min-interval 가드(`minStateEmitInterval`)는 monitor.go 의 `minMonitorInterval` 을 미러하여 유효 tick 이 하한 미만이면 하한으로 클램프한다(`time.NewTicker` panic 방지).
- **RD-4 — forward 는 mode-agnostic (OQ-2 확정)**: `forward_received_to_node` 는 direct·port **양 모드에 동일 적용**한다. `ingestState`(direct·port 공유 시임)의 단일 지점에 forward 탭을 두어 자연히 두 경로를 커버하며, 모드별 특별 케이스(no-op-in-port 등)를 두지 않는다. 방출 형태가 파싱/정규화된 상태이므로 port 모드에서도 원시 바이트 에코가 아니다(노드가 정규화 형태를 새로 획득 — 에코 위험 없음).
- **RD-5 — 메시지 타입/shape (OQ-3 확정)**: (1) forward 타입 = `device_state_received`, `changed_fields` **생략**, **단일 디바이스** 파싱 상태 메시지. (2) 주기 스냅샷 타입 = `device_state_snapshot`, **단일 `{timestamp, devices:[...]}` 배열 페이로드**(각 항목은 `deviceStateJSON` — `request_state` 전체 응답 shape 재사용). 주기 스냅샷을 **N개 per-device 개별 메시지**로 방출하는 대안은 채택하지 않는다.
- **RD-6 — 프런트엔드 포함 (OQ-4 확정)**: 에이전트 설정 UI 의 3개 옵션 토글/입력(`forward_received_to_node` 토글 + `state_emit_mode` 선택 + `state_emit_interval` 입력)을 **본 SPEC 범위에 포함**한다(Module 5). 우선순위는 **저우선 마일스톤 M5** — 백엔드(M1~M4) 완료 후 진행한다.

## 6. Traceability

- 상위 SPEC: SPEC-XSFM-001(base agent), SPEC-XSFM-GROUP-001(그룹 1급화, completed v0.4.0), SPEC-XSFM-LINE-001(라인 1급화, completed v0.3.1)
- 코드 anchor: `config.go`(parseXSFMConfig / transport_mode enum 검증 / parseDurationOpt), `agent.go`(ingestState / emitStateChanged / sendEvent 경유 msgCh / ReceiveMessage / ListDevices / Start / Stop / stopCh / monitorWG), `status.go`(deviceStateJSON / handleRequestState), `monitor.go`(startOfflineMonitor / checkOfflineDevices / minMonitorInterval — 미러 대상), `control.go`(sendEvent), `errors.go`(enum 검증 에러 패턴)
- 하위 산출물: plan.md(마일스톤·기술 접근·리스크·TRUST 5), acceptance.md(Given-When-Then 인수 시나리오)

## 7. Implementation Notes (as-implemented, Level 2)

> spec-anchored Level 2 규율에 따라, 구현(M1~M5, `5fc11209` + `6fb084c3`)이 §3~§5 사양과 갈린 지점을 as-implemented 로 기록한다. 4건 모두 **사양 위반이 아닌 구현 선택**이며, 확정 설계(RD-1~6)·범위 규율(scope discipline)·무회귀 불변식에 부합한다.

### §7.1 state_emit_mode select — raw enum 표시 (프런트, M5)

- **사양**: REQ-05-01 은 `state_emit_mode`(event/interval/both) 선택 UI 제공을 요구(한국어 의미 표기 방식은 미규정).
- **구현**: `state_emit_mode` select 는 원시 enum 값(`event`/`interval`/`both`)을 옵션으로 노출하고, 각 값의 한국어 의미는 필드 설명(description)에 기술한다.
- **사유**: 공용 `FormField` select 위젯에 옵션 라벨 매핑(value→표시 라벨) 기능이 없으며, 기존 xsfm/HVACR 계열 select 가 모두 동일하게 raw enum 을 노출한다. 별도 라벨 맵을 도입하지 않고 **기존 패턴을 그대로 계승**(scope discipline — 본 SPEC 범위 밖 위젯 개편 회피).

### §7.2 interval 모드 on-change 억제 — 단일 시임 복합 조건 (백엔드, M3)

- **사양**: §4.4 는 `interval` 모드에서 on-change `device_state_changed` 방출을 **억제**하도록 요구.
- **구현**: 억제는 `ingestState` 의 **기존 단일 방출 지점**에서 복합 조건(`len(changed) > 0 && (mode==event || mode==both)`)으로 처리한다 — 별도 플래그/분기 경로를 신설하지 않는다.
- **사유**: 억제를 단일 게이트 조건으로 표현하는 것이 별도 상태/경로 신설보다 단순하며(enforce simplicity), §4.4 의 게이팅 스케치와 일치. on-change 방출 지점이 하나뿐이므로 복합 조건으로 충분.

### §7.3 device_state_snapshot / device_state_received shape (백엔드, M2·M3)

- **사양(RD-5)**: 주기 스냅샷 = `device_state_snapshot` 단일 `{timestamp, devices:[...]}` 배열 메시지; forward = `device_state_received` 단일 디바이스 메시지, `changed_fields` 생략.
- **구현**: 확정대로 — `device_state_snapshot` 은 per-device N 메시지가 아닌 **단일 `{type, timestamp, devices:[...]}` 배열 메시지**로 방출하며 `deviceStateJSON`/`handleRequestState` shape 를 재사용한다. `device_state_received` 는 단일 디바이스 파싱 상태로 `changed_fields` 를 싣지 않는다.
- **사유**: 스냅샷은 배치/roster 개념이므로 단일 배열이 request_state 응답 shape 와 동형(재사용). 패스스루 탭은 "변경분" 개념이 없어 `changed_fields` 생략.

### §7.4 Stop 변경 없음 — offline monitor 자원 공유 (백엔드, M4)

- **사양**: §4.5 는 신규 주기 방출기 goroutine 을 `Stop` 에서 정리(누수 방지)하도록 요구.
- **구현**: 주기 방출기는 offline 모니터의 `monitorWG`/`stopCh` 를 **재사용**하며, `Stop` 은 기존 단일 `monitorWG.Wait()` 로 두 goroutine 종료를 함께 커버한다 — `Stop` 코드 자체는 변경하지 않는다.
- **사유**: §4.5 사양(monitorWG 재사용, 한 번의 Wait 로 두 goroutine 커버)에 정확히 부합. 두 goroutine 모두 채널 송신에 걸쳐 락을 잡지 않으므로 `Wait` 가 데드락하지 않는다(monitor.go 락 규율 계승).
