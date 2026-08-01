---
id: SPEC-MODBUS-006
title: "MODBUS Client RTU 지원 및 트랜스포트 선택 확장"
version: "0.2.0"
status: draft
created: 2026-08-01
updated: 2026-08-01
author: xtra
priority: P1
phase: "v0.1.0 target"
module: "internal/agent/modbus + internal/modbus + internal/node + web/src/config"
lifecycle: spec-anchored
tags: "modbus, client, rtu, serial, transport, polling"
depends_on:
  - SPEC-MODBUS-001
---

# SPEC-MODBUS-006: MODBUS Client RTU 지원 및 트랜스포트 선택 확장

## HISTORY

| 버전   | 날짜       | 변경 내용                                                        |
|--------|------------|-----------------------------------------------------------------|
| 0.1.0  | 2026-08-01 | 초안 작성 (Draft) — 기존 `modbus-tcp` 에이전트 확장(RTU + 트랜스포트 선택) 명세 |
| 0.2.0  | 2026-08-01 | 개정 (Draft, plan-phase 리뷰) — **노드를 통한 런타임 설정 변경** 능력 추가. 플로우 노드가 `agent_ref` → `Process()` JSON 명령(`set_config`)으로 실행 중인 클라이언트를 재시작 없이 재구성(레지스터 그룹·그룹별 poll_interval·타깃/디바이스 파라미터). REQ-02/REQ-05 확장, AC-08 추가. 신규 REQ 모듈 없음(최대 5개 유지). |

---

## 1. 개요 (Overview)

xflow에는 이미 MODBUS/TCP 클라이언트 에이전트(type id `modbus-tcp`, 패키지 `internal/agent/modbus/`)가 존재한다. 이 에이전트는 다중 디바이스 폴링, FC01~04 읽기 + FC05/06/15/16 쓰기, 디바이스별 online/offline 상태와 자동 재연결, online 비율 기반 health, `agent.AgentStats` 기반 통계, `MessageReceiver.ReceiveMessage` 외향 발신 + `Process([]byte)` JSON 명령 수신을 구현하고 있다.

본 SPEC은 이 에이전트를 **신규 패키지로 복제하지 않고 확장**하여 다음을 추가한다.

- **Modbus RTU 지원**: CRC-16(poly 0xA001, init 0xFFFF), RTU ADU 프레이밍(`[unitID][PDU][CRC-lo][CRC-hi]`), T3.5 프레임 간 정적(silence)을 준수하는 동기식 시리얼 마스터. 외부 modbus 라이브러리 도입 없이 in-house 구현한다.
- **트랜스포트 선택**: `transport: tcp | rtu` 디스크리미네이터. `transport`가 생략되면 기존 TCP 동작으로 기본 설정(하위 호환).
- **PDU 빌더 ADU-중립 리팩터링**: `protocol.go`의 PDU 빌더에 하드코딩된 7바이트 MBAP 헤더를 분리하여 TCP와 RTU가 동일한 PDU 빌더를 공유하게 한다.
- **레지스터 그룹별 독립 폴링 주기**: 그룹마다 선택적 `poll_interval`. 미지정 그룹은 에이전트/디바이스 기본 주기로 폴백(하위 호환).
- **데이터 타입 변환 확장**: `raw` 패스스루 + 완전한 4순열 바이트 순서(ABCD/BADC/CDAB/DCBA). 기존 word-swap(big_endian/little_endian) 의미는 유효하게 유지.
- **요청 처리 통계 표면화**: 디바이스별·그룹별 성공/오류/지연 카운터. 선택적으로 미구현 상태인 `ConnectionStatsProvider`(agent.go:97) 구현.
- **노드를 통한 런타임 설정 변경**: 실행 중인 클라이언트 에이전트의 설정을 **정적 `AgentConfig.Transport.Options`(init) 뿐 아니라** 플로우 노드가 발행하는 런타임 명령으로도 변경한다. 기존 노드→에이전트 제어 선례(`agent_ref` → `AgentResolver.ResolveAgent` → `AgentAccessor.UnderlyingAgent()` → `Process([]byte)` JSON 명령, `PollingConfigurable.SetPollInterval` 런타임 주기 오버라이드)를 **확장**하여, 신규 JSON 명령 `set_config`로 레지스터 그룹·그룹별 `poll_interval`·타깃/디바이스 파라미터를 **에이전트 재시작 없이** 재구성한다. 안전상 트랜스포트(tcp↔rtu) 전환 등은 init 전용으로 유지한다(신규 병렬 메커니즘을 만들지 않는다).

본 SPEC은 문서만 산출하며 구현 코드를 포함하지 않는다. 기존 `modbus-tcp` 동작과 type id는 반드시 보존한다(하위 호환, 기존 테스트는 특성화/행위 보존 대상으로 취급 — DDD).

---

## 2. 환경 (Environment)

| 항목                | 현재 상태 / 제약                                                              |
|---------------------|-------------------------------------------------------------------------------|
| 백엔드 언어         | Go 1.23+                                                                       |
| 클라이언트 패키지   | `internal/agent/modbus/` (type id `modbus-tcp`)                                |
| 폴링 루프           | `agent.go` `pollLoop` — 단일 `time.Ticker` (agent.go:340)                      |
| 프로토콜 구현       | `protocol.go`(FC01~04 읽기), `write.go`(FC05/06/15/16 쓰기)                    |
| 디바이스 상태       | `device.go` — 3회 연속 오류 후 offline, `TryReconnect` 자동 재연결             |
| Health              | online 비율 (agent.go:293)                                                     |
| 통계                | 공용 `agent.AgentStats` (`internal/agent/info.go`)                             |
| 선택적 통계 훅      | `ConnectionStatsProvider` (agent.go:97) — 미구현                              |
| 트랜스포트 인터페이스 | `ModbusTransport` (transport.go:16, mock 가능) — 현재 유일 구현 `ModbusTCPTransport`(MBAP/TCP) |
| 공용 데이터 타입 lib | `internal/modbus/types.go` — uint16/int16/uint32/int32/float32 + WORD 단위 word-swap(big_endian/little_endian) |
| 데이터 타입 공유    | `internal/agent/modbusserver/`(TCP 전용 서버/slave)와 `internal/modbus`를 공유 |
| 등록 패턴           | `register.go` `RegisterModbusTypes(mgr)` → `cmd/xflowd/main.go:396`에서 배선 |
| 설정 파싱           | `parseModbusConfig` (`config.go`) — `AgentConfig.Transport.Options`(map[string]any) |
| 프론트엔드 스키마   | `web/src/config/agentSchemas.ts` (MODBUS_TCP_FIELDS ~L74) — 스키마 주도, 신규 React 컴포넌트 불필요 |
| 시리얼 오픈 참고    | `century/transport_serial.go`(go.bug.st/serial) + `internal/agent/serial/`(half-duplex mutex) |
| CRC 재사용 금지     | `century` CRC = CRC-16/ARC(init 0x0000), `samsung` = CCITT — **Modbus RTU CRC 아님** |
| 개발 방법론         | hybrid — 신규 RTU/바이트순서 코드는 TDD, 기존 패키지 편집은 DDD(특성화 테스트) |

---

## 3. 가정 (Assumptions)

| ID   | 가정 사항                                                                                             |
|------|------------------------------------------------------------------------------------------------------|
| A-1  | 기존 `modbus-tcp` type id와 동작은 그대로 유지되어야 하며, 본 SPEC은 동일 패키지 내 확장으로만 수행한다 |
| A-2  | `transport` 키가 없는 기존 TCP 설정은 자동으로 `tcp`로 해석되어 기존 동작이 변경 없이 유지된다        |
| A-3  | TCP와 RTU는 동일한 PDU 빌더(ADU-중립 리팩터링 후)를 공유하며, ADU 계층(MBAP vs CRC 프레이밍)만 트랜스포트별로 다르다 |
| A-4  | RTU는 하나의 시리얼 포트 위에서 반이중(half-duplex) 동기식 마스터로 동작하며, 한 시점에 하나의 요청/응답만 처리한다 |
| A-5  | RTU 프레임 경계는 T3.5(3.5 문자 시간) 이상의 정적(silence)으로 판별하며, 저 baudrate에서는 실제 시간, 고 baudrate(>19200)에서는 고정 1.75ms 규약을 사용할 수 있다 |
| A-6  | go.bug.st/serial 의존성은 이미 century에서 사용 중이므로 재사용하며, 신규 modbus 라이브러리는 도입하지 않는다 |
| A-7  | 레지스터 그룹의 `poll_interval`이 없으면 에이전트/디바이스 기본 주기로 폴백하여 기존 단일-주기 동작과 동일하다 |
| A-8  | 기존 word-swap 의미(big_endian/little_endian)는 신규 4순열 바이트순서 체계 안에서 별칭으로 계속 유효하다 |
| A-9  | `raw` 타입은 어떤 변환도 하지 않고 읽은 워드(uint16 배열)를 그대로 전달한다                            |
| A-10 | RTU 시리얼 파라미터(port, baud, data bits, stop bits, parity)는 `Transport.Options`로 설정되며, 검증은 파싱 단계에서 수행한다 |

---

## 4. 요구사항 (Requirements)

EARS 형식. 요구사항 모듈은 **최대 5개**로 구성한다.

### REQ-MODBUS-006-01 — 트랜스포트 선택 및 RTU 시리얼 마스터

- **Ubiquitous**: 시스템은 항상 `transport` 설정값(`tcp` | `rtu`)에 따라 대응하는 `ModbusTransport` 구현으로 요청을 라우팅해야 한다.
- **Ubiquitous**: 시스템은 항상 RTU ADU를 `[unitID][PDU][CRC-lo][CRC-hi]` 형식으로 프레이밍하고, CRC-16(poly 0xA001, init 0xFFFF)로 계산해야 한다.
- **Event-driven**: WHEN RTU 요청을 전송할 때 THEN 시스템은 직전 프레임 이후 T3.5 이상의 정적을 확보한 뒤 ADU를 시리얼 포트로 기록해야 한다.
- **Event-driven**: WHEN RTU 응답을 수신할 때 THEN 시스템은 수신 CRC를 재계산·검증하고, unitID·function code 일치를 확인한 뒤 PDU를 상위로 반환해야 한다.
- **State-driven**: IF `transport`가 `rtu`이면 THEN 시스템은 반이중 시리얼 마스터로 동작하여 한 시점에 하나의 요청/응답 트랜잭션만 수행해야 한다(turnaround mutex).
- **Optional**: 가능하면 baudrate에 따라 T3.5 정적 시간을 자동 산출(≤19200: 문자시간 기반, >19200: 1.75ms 고정)하는 기능을 제공한다.
- **Unwanted**: 시스템은 CRC가 불일치하거나 프레임이 손상된 RTU 응답을 유효한 결과로 상위에 반환하지 않아야 한다(오류로 처리).

### REQ-MODBUS-006-02 — 레지스터 그룹별 독립 폴링 스케줄

- **Ubiquitous**: 시스템은 항상 각 `register_group`을 자신의 폴링 주기에 따라 독립적으로 폴링해야 한다.
- **State-driven**: IF `register_group`에 `poll_interval`이 지정되어 있으면 THEN 시스템은 해당 그룹을 그 주기로 폴링해야 한다.
- **State-driven**: IF `register_group`에 `poll_interval`이 없으면 THEN 시스템은 에이전트/디바이스 기본 주기로 해당 그룹을 폴링해야 한다(하위 호환).
- **Event-driven**: WHEN 런타임에 폴링 주기 변경 명령을 수신할 때 THEN 시스템은 기존 `PollingConfigurable` 경로(`SetPollInterval` → `pollResetCh` → `pollLoop`의 `pollTicker.Reset`)를 통해 대상 스코프(에이전트/디바이스/그룹)의 주기를 갱신해야 한다.
- **Event-driven**: WHEN 플로우 노드가 `Process()` `set_config` 명령으로 특정 그룹의 `poll_interval` 변경 또는 그룹 enable/disable를 요청할 때 THEN 시스템은 에이전트를 재시작하지 않고 해당 그룹의 폴링 케이던스를 갱신하며, 변경은 다음 폴 시점부터 반영되어야 한다.
- **State-driven**: IF 그룹이 런타임에 disable로 설정되면 THEN 시스템은 다음 폴 주기부터 해당 그룹을 폴링 대상에서 제외해야 하고, 이후 enable 시 다시 포함해야 한다.
- **Optional**: 가능하면 다수 그룹의 폴링을 다중 티커 또는 그룹별 스케줄러로 구성하여, 한 그룹의 지연이 다른 그룹의 정시성에 영향을 주지 않도록 한다.
- **Unwanted**: 시스템은 서로 다른 `poll_interval`을 가진 두 그룹을 하나의 전역 주기로 강제 병합하여 폴링하지 않아야 한다.
- **Unwanted**: 시스템은 런타임 그룹/주기 변경을 적용하는 동안 공유 상태(`config`, 그룹 스케줄)를 뮤텍스 보호 없이 수정하여 폴링 고루틴과의 경합을 유발하지 않아야 한다.

### REQ-MODBUS-006-03 — 데이터 타입 변환 확장

- **Ubiquitous**: 시스템은 항상 `internal/modbus`의 타입 변환기를 통해 읽은 워드를 지정된 타입/바이트순서로 디코딩해야 한다.
- **Ubiquitous**: 시스템은 항상 완전한 4순열 바이트순서(ABCD / BADC / CDAB / DCBA = word-swap × byte-swap)를 지원해야 한다.
- **State-driven**: IF 타입이 `raw`이면 THEN 시스템은 어떤 변환도 하지 않고 읽은 워드(uint16 배열)를 그대로 전달해야 한다.
- **State-driven**: IF 기존 `big_endian` / `little_endian`(word-swap) 설정이 주어지면 THEN 시스템은 기존과 동일한 결과를 산출해야 한다(하위 호환 별칭).
- **Optional**: 가능하면 float64/uint64(4워드) 디코딩을 추가로 제공한다.
- **Unwanted**: 시스템은 알 수 없는 바이트순서/타입 지정에 대해 잘못된 값을 반환하지 않고 파싱/설정 오류로 처리해야 한다.

### REQ-MODBUS-006-04 — 디바이스 상태 및 요청 처리 통계

- **Ubiquitous**: 시스템은 항상 각 트랜스포트(tcp/rtu)에 대해 디바이스의 online/offline 상태를 유지하고 3회 연속 오류 시 offline으로 전이해야 한다.
- **Event-driven**: WHEN 디바이스가 offline 상태에서 재연결 조건을 만족할 때 THEN 시스템은 해당 트랜스포트에 맞는 방식으로 `TryReconnect`를 수행해야 한다.
- **Event-driven**: WHEN 요청이 성공 또는 실패로 완료될 때 THEN 시스템은 디바이스별·그룹별 성공/오류/지연 카운터를 갱신해야 한다.
- **State-driven**: IF 디바이스가 offline이면 THEN health(online 비율) 계산에 offline으로 반영되어야 한다.
- **Optional**: 가능하면 `ConnectionStatsProvider`(agent.go:97)를 구현하여 디바이스별/그룹별 요청 통계를 외부로 표면화한다.
- **Unwanted**: 시스템은 트랜스포트 오류가 발생했음에도 오류 카운터를 증가시키지 않은 채 성공으로 통계에 반영하지 않아야 한다.

### REQ-MODBUS-006-05 — 설정 스키마·등록·프론트엔드 연동 및 하위 호환성

- **Ubiquitous**: 시스템은 항상 `AgentConfig.Transport.Options`에서 `transport`, RTU 시리얼 파라미터, 그룹별 `poll_interval`, 확장 데이터 타입/바이트순서를 파싱해야 한다(init 소스).
- **Ubiquitous**: 시스템은 항상 설정 소스를 **두 경로**로 지원해야 한다 — (1) init 시 정적 `Transport.Options`(기존, 불변), (2) 런타임 시 플로우 노드가 `agent_ref` → `Process([]byte)`로 발행하는 재구성 명령 `set_config`. 두 경로는 동일한 파싱/검증 규칙을 재사용해야 한다.
- **Event-driven**: WHEN 노드가 `set_config` 명령(`processRequest.Command == "set_config"`, `params`에 변경 필드)을 발행할 때 THEN `ModbusAgent.Process`의 스위치 디스패치가 이를 라우팅하여, 런타임 가변 필드(레지스터 그룹, 그룹별 `poll_interval`, 디바이스 unit_id/timeout/reconnect 등)를 기존 RWMutex(`a.mu`) 보호 하에 갱신해야 한다(에이전트 재시작 없이). 이는 신규 병렬 메커니즘이 아니라 기존 `Process()` 명령 디스패치 + `Configure`/`SetPollInterval` 런타임 변경 선례의 확장이다.
- **State-driven**: IF `transport` 키가 설정에 없으면 THEN `parseModbusConfig`는 이를 `tcp`로 해석하여 기존 동작을 변경 없이 유지해야 한다(init 경로 불변, 하위 호환).
- **Unwanted**: 시스템은 런타임 `set_config`로 init 전용 필드(트랜스포트 `tcp↔rtu` 전환, RTU 시리얼 하드웨어 파라미터 port/baud/parity)의 변경을 적용하지 않아야 하며, 이러한 요청은 오류로 거부하고 에이전트는 직전 설정으로 계속 동작해야 한다.
- **Ubiquitous**: 시스템은 항상 `modbus-tcp` type id를 보존하고, `register.go` `RegisterModbusTypes(mgr)` 및 `cmd/xflowd/main.go` 배선을 통해 확장 에이전트를 등록해야 한다.
- **Event-driven**: WHEN 프론트엔드가 MODBUS 에이전트 설정을 렌더링할 때 THEN `agentSchemas.ts`의 확장된 필드 세트(트랜스포트 선택 + RTU 시리얼 필드 + 그룹별 주기 + 데이터 타입/바이트순서)를 스키마 주도로 표시해야 한다(신규 React 컴포넌트 없이).
- **Optional**: 가능하면 새 스키마 맵 엔트리(RTU 필드 세트)를 추가하여 트랜스포트 선택에 따라 조건부로 시리얼 필드를 노출한다.
- **Unwanted**: 시스템은 신규 외부 modbus 라이브러리를 `go.mod`에 추가하지 않아야 한다(go.bug.st/serial 재사용).

---

## 5. 명세 (Specifications)

### 5.1 트랜스포트 계층

- `ModbusTransport`(transport.go:16) 인터페이스를 유지하고, 신규 `ModbusRTUTransport` 구현을 추가한다.
- `protocol.go`의 PDU 빌더를 ADU-중립으로 리팩터링: PDU(function code + payload)만 생성하고, MBAP(7바이트) 부착은 `ModbusTCPTransport`가, CRC 프레이밍은 `ModbusRTUTransport`가 담당한다.
- RTU CRC-16: 다항식 반사형 0xA001, 초기값 0xFFFF, 결과는 리틀엔디언 2바이트(CRC-lo, CRC-hi)로 부착.
- 시리얼 오픈은 `century/transport_serial.go`(go.bug.st/serial) 패턴을 따르고, 반이중 turnaround는 `internal/agent/serial/`의 mutex 패턴을 참고한다.

### 5.2 폴링 스케줄

- 각 `register_group`에 선택적 `poll_interval` 필드를 추가한다.
- 그룹별 독립 주기: 다중 티커 또는 그룹별 스케줄러. 미지정 그룹은 에이전트/디바이스 기본 주기로 폴백.
- 기존 `agent.go` `pollLoop`(단일 `time.Ticker`, agent.go:340) 동작은 "모든 그룹이 기본 주기" 특수 케이스로 하위 호환.
- 런타임 주기 변경은 기존 `PollingConfigurable` 경로 재사용: `SetPollInterval`(agent.go:1067)이 `a.mu` 하에 `config.PollInterval`을 갱신하고 `pollResetCh`로 시그널하면 `pollLoop`가 `pollTicker.Reset`(agent.go:368-370)으로 즉시 반영한다.
- 노드 주도 런타임 재구성: 플로우 노드가 `Process()` `set_config` 명령으로 그룹별 `poll_interval` 변경/그룹 enable-disable를 요청하면, 핸들러가 `a.mu.Lock()` 하에 그룹 스케줄을 갱신하고 동일한 `pollResetCh`/그룹 스케줄러 리셋 경로로 다음 폴 시점부터 반영한다(재시작 없음). 다중 그룹 케이던스는 REQ-02의 그룹 스케줄러 위에서 동일 시그널 방식을 확장한다.

### 5.3 데이터 타입 변환

- `internal/modbus`에 `raw` 패스스루와 4순열 바이트순서(ABCD/BADC/CDAB/DCBA)를 추가한다.
- 기존 word-swap(big_endian/little_endian) 의미를 4순열 체계의 별칭으로 매핑하여 하위 호환.
- (선택) float64/uint64(4워드) 추가.
- `internal/modbusserver`와의 공유는 유지하여 중복 구현을 피한다.

### 5.4 상태 및 통계

- 디바이스 상태/재연결(`device.go`)은 트랜스포트 추상화 위에서 tcp/rtu 공통으로 동작.
- 디바이스별·그룹별 성공/오류/지연 카운터를 `agent.AgentStats` 및/또는 `ConnectionStatsProvider`로 표면화.

### 5.5 설정·등록·프론트엔드

- `parseModbusConfig`(config.go)에 `transport`, RTU 시리얼 파라미터, 그룹별 `poll_interval`, 확장 타입/바이트순서 파싱 추가. `transport` 미지정 시 `tcp` 기본값.
- `register.go` `RegisterModbusTypes` 및 `cmd/xflowd/main.go` 배선에서 `modbus-tcp` type id 보존.
- `agentSchemas.ts`에 필드 세트 + 스키마 맵 엔트리 추가(조건부 시리얼 필드), 신규 React 컴포넌트 불필요.

#### 5.5.1 노드 주도 런타임 재구성 (`set_config` 명령)

설정 소스는 두 경로다: (1) init 시 정적 `Transport.Options`(`parseModbusConfig`), (2) 런타임 시 노드가 발행하는 `Process()` JSON 명령. 후자는 신규 병렬 서브시스템이 아니라 **기존 노드→에이전트 제어 선례의 확장**이다.

- 명령 경로: 플로우 노드(`internal/node/modbus.go`)는 이미 `agent_ref` → `AgentResolver.ResolveAgent` → `AgentAccessor.UnderlyingAgent()`로 `*modbus.ModbusAgent`를 획득하고(modbus.go:328-339), `callAgentProcess`(modbus.go:721-748)로 `{"command", "device_id", "params"}` JSON을 마샬링해 `agent.Process(cmdBytes)`를 호출한다.
- 신규 명령: `ModbusAgent.Process`의 스위치 디스패치(agent.go:576-597)에 기존 동사(`read_registers`/`write_registers`/`read_raw` 등)와 일관된 신규 명령 **`set_config`**를 추가한다. 핸들러(예: `processSetConfig(*processRequest)`)는 `req.Params`에서 변경 필드를 읽는다.
- 파싱/검증 재사용: `set_config` 핸들러는 `parseModbusConfig`가 사용하는 동일한 파싱·검증 규칙(그룹 스키마, `poll_interval` 하한, 데이터 타입/바이트순서 유효성)을 재사용하여 init 경로와 결과가 일치하도록 한다.
- 스레드 안전성: 핸들러는 기존 관례(`Configure` agent.go:934, `SetPollInterval` agent.go:1067)와 동일하게 `a.mu.Lock()`(RWMutex) 하에서 가변 상태(`a.config`, 레지스터 그룹, 그룹 스케줄, 캐시 TypeOverlay)를 수정하여 `pollLoop` 고루틴과의 경합을 방지한다. TypeOverlay 변경 시 `initCacheTypeOverlays`(agent.go:918)를 재적용한다.

런타임 가변(runtime-mutable) 필드 — `set_config`로 변경 가능:

- 레지스터 그룹: 추가 / 수정 / 삭제 / enable-disable
- 그룹별 `poll_interval` (§5.2 경로로 반영)
- 디바이스/타깃 파라미터: unit_id, request timeout, reconnect interval 등(안전 범위 내)
- 데이터 타입 / 바이트순서 오버레이(TypeOverlay 재구축)

init 전용(init-only) 필드 — 런타임 변경 거부(안전 결정):

- `transport`(tcp↔rtu 전환): 런타임 전환은 ADU 프레이밍·연결 토폴로지·시리얼 포트 오픈을 동시에 바꾸므로 **불허**한다. `set_config`가 트랜스포트 변경을 포함하면 오류를 반환하고 에이전트는 직전 설정으로 계속 동작한다.
- RTU 시리얼 하드웨어 파라미터(port/baud/data bits/stop bits/parity): 트랜스포트 오픈에 귀속되므로 init 전용으로 유지(런타임 변경은 오류로 거부).

---

## 6. 추적성 (Traceability)

| 요구사항 모듈          | 명세 절      | 수용 시나리오(acceptance.md)              | 주요 파일(reuse map은 plan.md 참조)                          |
|------------------------|--------------|-------------------------------------------|--------------------------------------------------------------|
| REQ-MODBUS-006-01      | 5.1          | AC-01, AC-02, AC-07                        | transport.go(신규 RTU), protocol.go(ADU-중립), internal/agent/serial |
| REQ-MODBUS-006-02      | 5.2, 5.5.1   | AC-04, AC-08                               | agent.go(pollLoop, SetPollInterval/pollResetCh), config.go, PollingConfigurable, internal/node/modbus.go, internal/node/modbus_poller.go |
| REQ-MODBUS-006-03      | 5.3          | AC-05                                      | internal/modbus/types.go                                     |
| REQ-MODBUS-006-04      | 5.4          | AC-06                                      | device.go, info.go(AgentStats), agent.go(ConnectionStatsProvider) |
| REQ-MODBUS-006-05      | 5.5, 5.5.1   | AC-03, AC-08                               | config.go(parseModbusConfig), agent.go(Process/set_config, Configure, a.mu RWMutex), register.go, cmd/xflowd/main.go, agentSchemas.ts, internal/node/modbus.go |

---

## 7. 범위 밖 (Non-Goals)

- MODBUS 서버(slave, `internal/agent/modbusserver/`) 변경.
- 신규 외부 modbus 라이브러리 도입.
- 기존 `modbus-tcp` type id 변경 또는 신규 패키지 복제.
- 실제 하드웨어 RTU 타이밍 튜닝(잔여 위험으로 plan.md에 기록).
