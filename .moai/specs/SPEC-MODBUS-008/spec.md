---
id: SPEC-MODBUS-008
title: "MODBUS Client 에이전트 확장 (디바이스별 transport · 세션 공유 · 런타임 디바이스 등록 · 프레임 로그)"
version: "0.1.0"
status: draft
created: 2026-08-03
updated: 2026-08-03
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/agent/modbus"
lifecycle: spec-first
tags: "modbus, client, transport, per-device, session-sharing, device-registration, frame-log, observability"
---

# SPEC-MODBUS-008: MODBUS Client 에이전트 확장 (디바이스별 transport · 세션 공유 · 런타임 디바이스 등록 · 프레임 로그)

## HISTORY

| 버전  | 날짜       | 변경 내용                                                                                                       |
|-------|------------|----------------------------------------------------------------------------------------------------------------|
| 0.1.0 | 2026-08-03 | 초안 작성 (Draft) — `modbus-client`(type id, 패키지 `internal/agent/modbus`) 4-기능 확장 명세. F1 런타임 디바이스 등록, F2 디바이스별 transport override, F3 세션 공유 opt-in, F4 프레임 로그(게이트웨이 관측성 포팅). 확정 설계 반영: F2=에이전트 기본값+디바이스 override, F3=opt-in 플래그, F4=게이트웨이 `observability.go` 패턴 포팅. REQ 5모듈(REQ-01~05), AC-01~10. 하위 호환은 HARD 게이트. |
| 0.2.0 | 2026-08-03 | plan-phase 리뷰 — 5개 미결 설계 결정 전량 해소(사용자 확정). (1) F1 명령 표면 = 신규 `add_device`/`remove_device` 명령(기존 1-동사-1-case 디스패치 스타일), (2) F3 플래그 스코프 = 에이전트 레벨 기본 + 선택적 per-device 오버라이드, (3) F3 공유 연결 제거 = 참조 카운팅 마지막-참조 close, (4) F4 raw 프레임 = 로그 전용(이벤트/조회 노출은 향후 과제로 이연), (5) F1 런타임 통계 = init-불변 통계 맵을 런타임 추가 디바이스/그룹까지 확장(신규 스레드 안전성 요구). REQ-01/03/04/05 확정 반영, AC-06/08/10 갱신. §7 미결 결정 섹션 해소. |

---

## 1. 개요 (Overview)

xflow에는 이미 MODBUS 클라이언트 에이전트(type id `modbus-client`, 패키지 `internal/agent/modbus/`, ~12K LoC)가 존재한다. 이 에이전트는 다중 디바이스 폴링, FC01~04 읽기 + FC05/06/15/16 쓰기, TCP(MBAP)·RTU(CRC) 트랜스포트 선택(SPEC-MODBUS-006), 그룹별 독립 폴링, 노드→에이전트 `Process()` JSON 명령(`set_config` 런타임 재구성)을 구현하고 있다.

> **주의(패키지 혼동 금지)**: 본 SPEC의 대상은 클라이언트 `internal/agent/modbus`(type id `modbus-client`)이다. 서버(slave)인 `internal/agent/modbusserver`(type id `modbus-gateway`)와 혼동하지 않는다. 다만 F4 프레임 로그는 게이트웨이 패키지의 관측성 구현을 **참조 패턴**으로 삼는다(§2.2).

본 SPEC은 이 에이전트를 **신규 패키지로 복제하지 않고 확장**하여 다음 4개 기능을 추가한다.

- **F1 — 런타임 디바이스 등록 (device registration)**: 설정 시점의 `devices[]` 배열은 이미 존재한다. 신규 능력은 **에이전트 재시작 없이 런타임에 디바이스를 추가/삭제**하는 것이다(현재 `set_config`는 디바이스 add/remove를 거부하고 register_groups/unit_id/poll_interval/timeout/reconnect만 변경).
- **F2 — 디바이스별 TCP/RTU 설정 (per-device transport)**: 에이전트 레벨 `transport`/`Serial`을 **기본값**으로 유지하고, `DeviceConfig`에 이를 **오버라이드**하는 선택적 per-device `transport`(+ RTU용 per-device 시리얼 파라미터)를 추가한다. `transport`를 생략한 디바이스는 에이전트 기본값을 상속한다(완전한 하위 호환).
- **F3 — 세션 공유 옵션 (session sharing)**: 기본 동작(TCP=디바이스별 독립 연결, RTU=단일 버스 공유)을 유지하되, **opt-in 플래그**(`share_session`)로 동일 엔드포인트를 대상으로 하는 디바이스들이 하나의 트랜스포트/연결을 공유하도록 한다.
- **F4 — 프레임 로그 / Raw 프레임 (frame log)**: 게이트웨이(`modbus-gateway`)의 프레임 관측성을 클라이언트로 포팅한다. 트랜스포트 계층의 프레임 경계에서 각 TX/RX ADU를 hex로 로그한다(TCP MBAP + RTU CRC 프레이밍 양쪽). 로그 폭주 방지를 위해 opt-in 플래그로 게이팅한다.

본 SPEC은 문서만 산출하며 구현 코드를 포함하지 않는다. 기존 `modbus-client` 동작과 type id는 반드시 보존한다(하위 호환, 기존 테스트는 특성화/행위 보존 대상 — DDD/hybrid).

### 1.1 확정 설계 결정 (Locked)

1. **F2 = 에이전트 기본값 + 디바이스 override**: 에이전트 레벨 `transport`/`Serial`이 DEFAULT이며, `DeviceConfig.transport`(선택)가 이를 오버라이드한다. `transport` 생략 디바이스는 에이전트 값을 상속하여 **기존 설정과 바이트 단위로 동일하게 동작**한다(HARD 하위 호환).
2. **F3 = opt-in 플래그**: 기본값 = 현재 동작(TCP 디바이스별 독립 연결, RTU 단일 버스 공유 유지). `share_session` 플래그가 켜질 때만 동일 엔드포인트 디바이스가 트랜스포트/연결을 공유한다. 엔드포인트 키 = TCP는 `(host, port)`, RTU는 `serial_port`.
3. **F4 = 게이트웨이 관측성 패턴 포팅**: `internal/agent/modbusserver/observability.go`의 `serverObs`/`logFrame` 규약(dir/unit_id/fc/hex, `log_frames`/`log_raw_frames` atomic 토글)을 클라이언트로 포팅하되 명명을 "modbus-client: frame"으로 적응한다. raw 프레임은 로그 폭주 방지를 위해 opt-in.
4. **하위 호환 = HARD 게이트**: per-device 필드 없음 + `share_session` 없음 + 프레임 로그 없음의 기존 설정은 모두 **오늘과 동일하게 동작**해야 한다. 특성화 테스트로 강제한다.

---

## 2. 환경 (Environment)

| 항목                | 현재 상태 / 제약 (file:line 근거)                                                                                     |
|---------------------|----------------------------------------------------------------------------------------------------------------------|
| 백엔드 언어         | Go 1.23+                                                                                                              |
| 클라이언트 패키지   | `internal/agent/modbus/` (type id `modbus-client`, ~12K LoC)                                                          |
| 서버 패키지(구분)   | `internal/agent/modbusserver/` (type id `modbus-gateway`) — 본 SPEC 대상 아님, F4 참조 패턴만 사용                    |
| 트랜스포트(에이전트 레벨) | `ModbusConfig.Transport` = "tcp" \| "rtu" (config.go:20), `ModbusConfig.Serial SerialConfig`(config.go:21)      |
| 트랜스포트 구성     | `buildDevices`(agent.go:160-173): TCP=디바이스별 독립 `NewModbusDevice`(자체 `ModbusTCPTransport`); RTU=전 디바이스가 단일 `NewModbusRTUTransport` 공유 |
| 디바이스 설정       | `DeviceConfig`(config.go:46-53): ID, Host, Port, UnitID, RegisterGroups — **per-device transport 필드 없음**         |
| 설정 파싱           | `parseModbusConfig`(config.go:66-221); devices 파싱 config.go:194-209; `parseDeviceConfig(devMap, i, cfg.Transport)` config.go:202 |
| 트랜스포트 인터페이스 | `ModbusTransport.SendAndReceive(ctx, unitID byte, pdu []byte) → respPDU`(transport.go:18-25); 구현: `ModbusTCPTransport`(transport.go:87), `ModbusRTUTransport`(transport_rtu.go:99) |
| RTU 공유 뮤텍스     | RTU turnaround mutex — 단일 버스 직렬화(transport_rtu.go:23, buildDevices agent.go:161-167) — F3 공유 접근 직렬화의 기존 선례 |
| 런타임 재구성       | `ModbusAgent.Process`(agent.go:795) 문자열 명령 스위치(agent.go:801-824): read_registers/get_status/get_cache/get_all_caches/write_*/read_raw/**set_config** |
| set_config 범위     | `processSetConfig`(agent.go:821, set_config.go): register_groups/unit_id/poll_interval/request_timeout/reconnect만 변경. 디바이스 add/remove **없음**. `rejectInitOnlyFields`(set_config.go:204-217)로 transport 전환·RTU 시리얼 하드웨어 파라미터 거부 |
| 디바이스 optional   | `parseModbusConfig` 주석(config.go:211-213): 디바이스 없이 에이전트 생성 후 이후 설정/런타임으로 디바이스 추가 가능(0-device Start/pollLoop no-op 안전) — F1의 기존 안전 훅 |
| 통계 맵 불변        | `initRequestStats`(agent.go:177-186): `devStats`/`groupStats` 맵은 init 시 1회 생성 후 불변(폴링 goroutine이 락 없이 읽음) — 런타임 추가 디바이스/그룹 통계 미생성 한계(SPEC-006 IN-5) |
| 프레임 로그(클라이언트) | **부재**. 온디맨드 `read_raw` 명령(agent.go:943-952)과 event `data` raw 바이트만 존재. TX/RX ADU hex 프레임 로그 없음 |
| 프레임 로그(게이트웨이) | `internal/agent/modbusserver/observability.go` `serverObs`{logFrames, logRawFrames atomic.Bool} + `logFrame(logger, frames, raw, dir, addr, unitID, fc, adu)`; `client_registry.go` — F4 참조 패턴 |
| 등록 배선           | `register.go` `RegisterModbusTypes(mgr)` → `cmd/xflowd/main.go` 배선, type id `modbus-client` 보존                    |
| 프론트엔드 스키마   | `web/src/config/agentSchemas.ts` — 스키마 주도, 신규 React 컴포넌트 불필요(SPEC-006 선례)                             |
| 개발 방법론         | hybrid — 신규 코드는 TDD, 기존 패키지 편집은 DDD(특성화 테스트). 커버리지 목표 85%, `code_comments: ko`               |

### 2.1 현재 아키텍처 (ANALYZE — F2/F3 근거)

- **에이전트 레벨 트랜스포트**: 전 디바이스가 하나의 `transport`("tcp" 또는 "rtu")를 공유한다(config.go:20, `parseModbusConfig` config.go:66-221). per-device 오버라이드가 없다 → F2가 이를 추가.
- **연결 소유**: TCP는 `buildDevices`(agent.go:169-172)에서 디바이스마다 자체 `ModbusTCPTransport`를 생성한다(엔드포인트가 같아도 공유 없음). RTU는 단일 `ModbusRTUTransport`를 전 디바이스에 주입(agent.go:161-167, 이미 세션 공유의 특수 케이스). → F3가 TCP에도 opt-in 공유를 도입.
- **직렬화 선례**: RTU 공유 버스는 turnaround mutex로 `SendAndReceive` 전체를 직렬화한다(transport_rtu.go:23). → F3의 공유 연결 스레드 안전성은 이 패턴을 준거로 한다.

### 2.2 게이트웨이 관측성 참조 (F4 근거)

`modbus-gateway`(`internal/agent/modbusserver/observability.go`)는 다음을 구현한다.

- `serverObs`: `logFrames *atomic.Bool`(프레임 요약 토글), `logRawFrames *atomic.Bool`(전체 ADU hex 포함 토글), `framesOn()`/`rawOn()` 접근자. atomic 미러로 재시작 없이 `Configure`에서 갱신.
- `logFrame(logger, frames, raw, dir, addr, unitID, fc, adu)`: frames가 켜져 있을 때 TX/RX 프레임 요약(방향/주소/unit_id/FC/바이트 길이)을 INFO 로그하고, raw가 true면 전체 ADU hex를 함께 남긴다. frames off거나 logger nil이면 no-op(opt-in 진단용).

F4는 이 규약을 클라이언트 트랜스포트 계층(`transport.go` SendAndReceive / `transport_rtu.go`)으로 포팅하며 명명을 "modbus-client: frame"으로 적응한다.

---

## 3. 가정 (Assumptions)

| ID   | 가정 사항                                                                                                          |
|------|-------------------------------------------------------------------------------------------------------------------|
| A-1  | 기존 `modbus-client` type id와 동작은 그대로 유지되며, 본 SPEC은 동일 패키지 내 확장으로만 수행한다(하위 호환 HARD) |
| A-2  | per-device `transport` 키가 없는 기존 설정은 에이전트 레벨 `transport`(생략 시 tcp)를 상속하여 기존과 바이트 동일하게 동작한다 |
| A-3  | `share_session` 키가 없거나 false면 현재 연결 토폴로지(TCP 디바이스별 독립, RTU 단일 버스)가 그대로 유지된다        |
| A-4  | 프레임 로그 플래그(`log_frames`/`log_raw_frames`)가 없거나 false면 프레임 로그는 no-op이며 성능/출력에 영향이 없다  |
| A-5  | F2 per-device RTU 오버라이드가 서로 다른 시리얼 포트를 지정하면 각 포트마다 별개의 RTU 버스/트랜스포트가 존재한다   |
| A-6  | F3 엔드포인트 동일성은 TCP `(host, port)`, RTU `serial_port` 기준으로 판정하며, 이 키가 같은 디바이스만 연결을 공유한다 |
| A-7  | F3 공유 연결의 동시 접근은 기존 RTU turnaround mutex 패턴(transport_rtu.go:23)에 준하여 직렬화한다                  |
| A-8  | F1 런타임 디바이스 추가/삭제는 `a.mu`(RWMutex) 보호 하에서 수행되며, 0-device no-op 안전성(config.go:211-213)을 전제로 한다 |
| A-9  | F4 프레임 로그는 트랜스포트 `SendAndReceive` 경계(ADU 프레이밍 직후 TX, 응답 수신 직후 RX)에서 hook되며 값 디코딩과 무관하다 |
| A-10 | 프론트엔드는 스키마 주도(`agentSchemas.ts`)로 신규 필드를 노출하며, 신규 React 컴포넌트를 도입하지 않는다(SPEC-006 선례) |

---

## 4. 요구사항 (Requirements — EARS)

EARS 형식. 요구사항 모듈은 **5개**(REQ-01~05)로 구성한다. F4(가장 독립적) → F2 → F3(F2 위) → F1 순의 구현 의존 관계를 반영한다.

### REQ-MODBUS-008-01 — F4 프레임 로그 / Raw 프레임 관측성 (opt-in)

- **Ubiquitous**: 시스템은 항상 트랜스포트 계층의 프레임 경계(`SendAndReceive` TX/RX)에서 프레임 로그 hook을 제공해야 하며, 프레임 요약은 방향(dir)·unit_id·function code·바이트 길이를 포함해야 한다(게이트웨이 `logFrame` 규약 준거).
- **Event-driven**: WHEN `log_frames`가 활성인 상태에서 TCP 또는 RTU ADU를 송신/수신할 때 THEN 시스템은 해당 프레임 요약을 "modbus-client: frame"으로 INFO 로그해야 한다(TCP MBAP·RTU CRC 프레이밍 양쪽).
- **State-driven (raw 게이팅)**: IF `log_raw_frames`가 활성이고 `log_frames`도 활성이면 THEN 시스템은 전체 ADU hex를 함께 로그해야 한다. IF `log_frames`가 비활성이면 THEN raw 여부와 무관하게 프레임 로그는 no-op이어야 한다(로그 폭주 방지).
- **Optional (런타임 토글)**: 가능하면 `log_frames`/`log_raw_frames`를 atomic 미러로 구현하여 `set_config`(또는 Configure) 경로로 에이전트 재시작 없이 토글할 수 있게 한다(게이트웨이 atomic.Bool 패턴).
- **Optional (이벤트 표면화)**: 가능하면 프레임을 게이트웨이가 노출하는 방식과 정렬하여 이벤트/조회 가능한 형태로도 표면화한다.
- **Unwanted**: 시스템은 프레임 로그 플래그가 없거나 false인 기존 설정에서 어떤 프레임 로그도 방출하지 않아야 하며(no-op), 프레임 로그 hook이 읽기/쓰기 값 자체를 변형하지 않아야 한다.

### REQ-MODBUS-008-02 — F2 디바이스별 트랜스포트 (agent-default + per-device override)

- **Ubiquitous**: 시스템은 항상 에이전트 레벨 `transport`/`Serial`을 기본값으로 사용하고, `DeviceConfig`에 선택적 per-device `transport`(+ RTU용 per-device 시리얼 파라미터)가 있으면 이를 오버라이드로 적용해야 한다.
- **Event-driven**: WHEN `buildDevices`가 디바이스를 생성할 때 THEN 각 디바이스는 자신의 per-device `transport`(있으면) 또는 에이전트 기본값(없으면)에 따라 대응 트랜스포트로 구성되어야 한다.
- **State-driven (상속)**: IF 디바이스가 `transport`를 생략하면 THEN 에이전트 레벨 값을 상속하여 기존 설정과 바이트 단위로 동일하게 동작해야 한다(하위 호환).
- **State-driven (검증)**: IF 디바이스가 per-device RTU 오버라이드를 지정하면 THEN 해당 디바이스는 시리얼 파라미터(port 등)를 가져야 하고, TCP 오버라이드를 지정하면 host를 가져야 하며, 누락 시 설정 오류로 거부해야 한다.
- **Unwanted**: 시스템은 per-device 오버라이드가 없는 디바이스에 대해 에이전트 기본값과 다른 트랜스포트를 임의로 선택하지 않아야 한다.

### REQ-MODBUS-008-03 — F3 세션 공유 옵션 (opt-in `share_session`)

- **Ubiquitous**: 시스템은 항상 기본값으로 현재 연결 토폴로지(TCP=디바이스별 독립 연결, RTU=단일 버스 공유)를 유지해야 한다.
- **Event-driven (opt-in 공유)**: WHEN `share_session`이 활성이면 THEN 동일 엔드포인트 키(TCP `(host, port)`, RTU `serial_port`)를 갖는 디바이스들이 하나의 트랜스포트/연결을 공유하도록 `buildDevices`가 구성해야 한다.
- **State-driven (F2 상호작용)**: IF `share_session`이 활성이고 F2 per-device transport가 혼재하면 THEN 공유는 동일 트랜스포트 종류(tcp↔tcp, rtu↔rtu) + 동일 엔드포인트 키 범위 내에서만 성립해야 하며, 종류/엔드포인트가 다른 디바이스는 공유하지 않아야 한다.
- **State-driven (연결 수명)**: IF 여러 디바이스가 하나의 연결을 공유하면 THEN 연결의 connect/close/reconnect 소유권은 공유 트랜스포트가 가지며, 마지막 참조 디바이스가 사라질 때까지 close하지 않아야 한다(참조 카운팅 또는 동등 메커니즘).
- **State-driven (스레드 안전성)**: IF 공유 연결이 여러 디바이스의 요청을 처리하면 THEN 기존 RTU turnaround mutex(transport_rtu.go:23) 패턴에 준하여 `SendAndReceive` 접근을 직렬화해야 한다.
- **Unwanted**: 시스템은 `share_session`이 비활성인 설정에서 연결을 공유하지 않아야 하며(현 토폴로지 유지), 한 공유 디바이스의 오류/제거가 동일 연결을 쓰는 다른 디바이스의 트랜잭션을 중단시키지 않아야 한다.

### REQ-MODBUS-008-04 — F1 런타임 디바이스 등록 (add/remove, 재시작 없음)

- **Ubiquitous**: 시스템은 항상 노드→에이전트 `Process()` 명령 경로로 런타임에 디바이스를 추가/삭제할 수 있어야 하며, 이는 `a.mu`(RWMutex) 보호 하에서 수행되어야 한다.
- **Event-driven (add)**: WHEN 디바이스 추가 명령이 발행될 때 THEN 시스템은 `parseDeviceConfig` 검증 규칙(init 경로와 동일)으로 디바이스 설정을 파싱·검증하고, F2 규칙에 따라 트랜스포트를 선택(에이전트 기본/디바이스 override)하여 디바이스를 생성·연결(connect on add)하고 폴링 대상에 포함해야 한다.
- **Event-driven (remove)**: WHEN 디바이스 삭제 명령이 발행될 때 THEN 시스템은 해당 디바이스를 폴링 대상에서 제외하고 연결을 close(close on remove)해야 한다. IF 삭제 디바이스가 F3 공유 연결을 사용 중이면 THEN 마지막 참조일 때만 실제 close해야 한다.
- **State-driven (register_groups/transport 정의)**: IF 추가 디바이스가 register_groups를 지정하면 THEN 해당 그룹으로 폴링을 시작하고, transport override를 지정하면 F2 규칙을 따르며, 생략 시 에이전트 기본값을 상속해야 한다.
- **Unwanted**: 시스템은 유효하지 않은 디바이스 추가/삭제 요청에 대해 부분 적용을 하지 않아야 하며(원자적 거부), 존재하지 않는 디바이스 삭제나 중복 ID 추가를 오류로 거부하고 직전 상태로 계속 동작해야 한다.

### REQ-MODBUS-008-05 — 하위 호환 · 명령 표면 · 등록/프론트엔드 · 통계 (cross-cutting)

- **Ubiquitous (하위 호환 HARD)**: 시스템은 항상 per-device 필드 없음 + `share_session` 없음 + 프레임 로그 없음의 기존 `modbus-client` 설정을 오늘과 동일하게(바이트 단위) 동작시켜야 한다.
- **Ubiquitous (명령 표면)**: 시스템은 항상 F1 디바이스 등록을 기존 `Process` 문자열 명령 스위치(agent.go:801-824)와 일관된 방식으로 노출해야 한다(신규 병렬 메커니즘 금지). [설계 결정: 신규 `add_device`/`remove_device` 명령 vs `set_config` 확장 — §5.5 및 §7 참조].
- **State-driven (통계 한계)**: IF 런타임에 디바이스/그룹이 추가되면 THEN per-device/per-group 통계 맵(init-시점 불변, agent.go:177-186) 확장 여부는 명시적으로 결정되어야 하며(생성 또는 문서화된 한계), 폴링 goroutine과의 경합을 유발하지 않아야 한다.
- **Ubiquitous (등록/프론트엔드)**: 시스템은 항상 `modbus-client` type id를 보존하고(`register.go`/`cmd/xflowd/main.go`), 신규 필드(per-device transport, share_session, 프레임 로그 토글)를 `agentSchemas.ts` 스키마 주도로 노출해야 한다(신규 React 컴포넌트 없이).
- **Unwanted**: 시스템은 신규 외부 modbus 라이브러리를 `go.mod`에 추가하지 않아야 하며, 런타임 변경 적용 중 공유 상태(`a.config`, devices, 트랜스포트 맵)를 뮤텍스 보호 없이 수정하지 않아야 한다.

---

## 5. 명세 (Specifications)

### 5.1 F4 — 프레임 로그 (참조 패턴 포팅)

- 게이트웨이 `observability.go`의 `serverObs`/`logFrame` 규약을 클라이언트로 포팅: 클라이언트 측 관측성 묶음(예: `clientObs`)에 `logFrames`/`logRawFrames` atomic.Bool 미러 + `framesOn()`/`rawOn()` 접근자.
- hook 지점: `transport.go` `ModbusTCPTransport.SendAndReceive`(transport.go:87)와 `transport_rtu.go` `ModbusRTUTransport.SendAndReceive`(transport_rtu.go:99)의 ADU 프레이밍 직후(TX)·응답 수신 직후(RX). dir/unit_id/fc/adu-hex를 "modbus-client: frame"으로 로그.
- 설정 키: `log_frames`(요약), `log_raw_frames`(전체 hex, `log_frames` 활성 시에만 의미). 기본값 false(no-op).
- raw 프레임은 **로그 전용(확정)** — 이벤트/조회 표면화는 향후 과제로 이연. `set_config`/Configure 경로로 atomic 토글은 지원.

### 5.2 F2 — 디바이스별 트랜스포트

- `DeviceConfig`(config.go:46-53)에 선택적 `Transport string` + per-device `Serial`(RTU override용) 추가. `parseDeviceConfig`(config.go:202)가 이를 파싱·검증(RTU override → 시리얼 파라미터 필수, TCP override → host 필수).
- `buildDevices`(agent.go:160-173) 변경: 디바이스마다 유효 트랜스포트 = per-device override ?? 에이전트 기본값. override 없으면 기존 경로(TCP 디바이스별/RTU 단일 버스) 그대로.

### 5.3 F3 — 세션 공유 (opt-in)

- 설정 키 `share_session`(bool). 스코프 = **에이전트 레벨 기본 + 선택적 per-device 오버라이드(확정)**. 기본 false.
- 활성 시 `buildDevices`가 엔드포인트 키((host,port) TCP / serial_port RTU) + 트랜스포트 종류로 디바이스를 그룹화하여, 그룹당 하나의 공유 트랜스포트를 생성·주입.
- 공유 트랜스포트: 참조 카운팅으로 connect/close 소유(마지막 참조 시 close), reconnect는 공유 트랜스포트가 수행. `SendAndReceive` 접근은 turnaround mutex 준거 직렬화(transport_rtu.go:23 패턴).
- F2 상호작용: 종류/엔드포인트가 일치하는 디바이스만 공유. RTU 기본 단일 버스 공유는 이 규칙의 특수 케이스로 보존.

### 5.4 F1 — 런타임 디바이스 등록

- 명령 경로: 기존 `agent_ref` → `AgentAccessor.UnderlyingAgent()` → `Process([]byte)` JSON 명령(SPEC-006 선례). **신규 `add_device`/`remove_device` 명령**으로 디바이스 add/remove 라우팅(확정, §5.5).
- add: `parseDeviceConfig` 재사용 검증 → F2 트랜스포트 선택 → 디바이스 생성·connect → devices 슬라이스 + 폴링 스케줄에 편입(`a.mu.Lock()`). 0-device no-op 안전성(config.go:211-213) 전제.
- remove: 폴링 제외 → 연결 close(F3 공유 시 마지막 참조만). 미존재/중복 ID는 오류 거부(원자적).
- 통계: init-시점 불변 맵(agent.go:177-186)을 **런타임 추가 디바이스/그룹까지 확장(확정)**. 이전에 init-후 불변이던 맵을 런타임 변경하므로 스레드 안전성(a.mu 보호 또는 원자적 교체)이 신규 요구사항이다.

### 5.5 명령 표면 설계 결정 (EARS 설계 결정 — 권장안 포함)

F1 디바이스 등록의 노드→에이전트 명령 표면은 두 가지 후보가 있다.

- **후보 A (권장) — 신규 `add_device` / `remove_device` 명령**: `Process` 스위치(agent.go:801-824)는 한 동사당 하나의 `case` + 하나의 `processXxx` 핸들러 스타일이다(`read_registers`, `write_register`, `read_raw`, `set_config`). 디바이스 라이프사이클(생성/연결/제거)은 `set_config`의 "기존 디바이스/그룹 재구성" 의미와 성격이 다르므로, 신규 동사 2개가 기존 디스패치 스타일에 가장 잘 부합한다. **권장 근거**: 기존 명령 분해 스타일(1 동사 = 1 case)과의 일관성, `set_config` 책임 비대화 방지.
- **후보 B — `set_config` 확장**: `set_config` params에 `add_devices`/`remove_devices` 필드를 추가. 장점은 단일 재구성 엔드포인트 유지. 단점은 `set_config`가 "실행 중 디바이스/그룹 미세조정"에서 "디바이스 라이프사이클 관리"까지 확장되어 책임이 커진다.

→ **확정: 후보 A (신규 `add_device`/`remove_device` 명령)** — plan 리뷰에서 사용자 확정됨. 후보 B(`set_config` 확장)는 미채택.

---

## 6. 추적성 (Traceability)

| 요구사항 모듈          | 명세 절      | 수용 시나리오(acceptance.md) | 주요 파일(reuse map은 plan.md 참조)                                   |
|------------------------|--------------|------------------------------|-----------------------------------------------------------------------|
| REQ-MODBUS-008-01 (F4) | 5.1          | AC-01, AC-02                 | transport.go, transport_rtu.go, (신규) client observability, modbusserver/observability.go(참조) |
| REQ-MODBUS-008-02 (F2) | 5.2          | AC-03, AC-04                 | config.go(DeviceConfig, parseDeviceConfig), agent.go(buildDevices)    |
| REQ-MODBUS-008-03 (F3) | 5.3          | AC-05, AC-06                 | agent.go(buildDevices), transport.go/transport_rtu.go(공유·참조카운팅) |
| REQ-MODBUS-008-04 (F1) | 5.4, 5.5     | AC-07, AC-08                 | agent.go(Process 스위치, devices/폴링), config.go(parseDeviceConfig), set_config.go |
| REQ-MODBUS-008-05      | 5.2~5.5      | AC-09, AC-10                 | config.go, agent.go(a.mu, initRequestStats), register.go, cmd/xflowd/main.go, agentSchemas.ts |

---

## 7. 설계 결정 (해소됨 — plan 리뷰 사용자 확정)

> 초안의 미결 5건은 plan 리뷰에서 전량 사용자 확정되었다. 아래는 확정 내역이며 run 단계는 이를 전제로 진행한다.

1. **F1 명령 표면 (§5.5)**: 신규 `add_device`/`remove_device` 명령 — **확정**. (`set_config` 확장은 미채택.)
2. **F3 플래그 스코프**: 에이전트 레벨 기본 + 선택적 per-device 오버라이드 — **확정**.
3. **F3 공유 연결 제거 시맨틱**: 참조 카운팅, 마지막 참조 시 공유 트랜스포트 close — **확정**.
4. **F4 raw 프레임 표면화**: 로그 전용 — **확정**. (이벤트/조회 노출은 향후 과제로 이연.)
5. **F1/런타임 그룹 통계**: init-불변 통계 맵을 런타임 추가 디바이스/그룹까지 확장(신규 스레드 안전성 요구) — **확정**.

---

## 8. 범위 밖 (Non-Goals)

- MODBUS 게이트웨이(`internal/agent/modbusserver/`, type id `modbus-gateway`) 변경 — F4는 참조 패턴만 차용.
- 신규 외부 modbus 라이브러리 도입.
- 기존 `modbus-client` type id 변경 또는 신규 패키지 복제.
- 프레임 로그의 영구 저장/외부 전송(로그 싱크는 기존 slog 경로 사용).
- 실제 하드웨어 RTU 타이밍 튜닝(잔여 위험으로 plan.md에 기록).
