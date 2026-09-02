---
id: SPEC-MODBUS-010
title: "MODBUS Gateway 가상 디바이스의 실제(upstream) 디바이스 백킹 + 직접/간접 모드"
version: "0.1.0"
status: draft
created: 2026-08-04
updated: 2026-08-04
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/agent/modbusserver"
lifecycle: spec-anchored
tier: L
tags: "modbus, gateway, upstream-backing, master, direct-mode, indirect-mode, poller, disconnect-handling, register-map"
---

# SPEC-MODBUS-010: MODBUS Gateway 가상 디바이스의 실제(upstream) 디바이스 백킹 + 직접/간접 모드

## HISTORY

| 버전  | 날짜       | 변경 내용                                                                                                                                                                                                                                                                       |
|-------|------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 0.1.0 | 2026-08-04 | 초안 작성 (Draft, Tier L 5-산출물). MODBUS Gateway(`modbus-gateway`, 패키지 `internal/agent/modbusserver`)의 가상 디바이스가 실제(upstream) MODBUS 디바이스를 백킹하도록 확장. 확정 설계 반영: (1) per-virtual-device direct/indirect 모드, (2) 끊김 시 응답 = direct 즉각 예외 / indirect 타임아웃 기반 stale 허용, (3) 예외 코드 `0x0B`(Gateway Target Device Failed to Respond), (4) 쓰기는 양 모드 모두 upstream 전달 + RegisterMap 미러, (5) 하위 호환 = HARD 게이트(백킹 없는 디바이스 = 순수 slave), (6) modbus-client 트랜스포트(`internal/agent/modbus`) 재사용. REQ-01~06, AC-01~13. |

---

## 1. 개요 (Overview)

xflow의 MODBUS Gateway 에이전트(type id `modbus-gateway`, 패키지 `internal/agent/modbusserver/`)는 현재 **순수 slave(server)**다. 원격 마스터의 요청(FC01~04 읽기, FC05/06/15/16 쓰기)을 받아 각 가상 디바이스의 `RegisterMap` 저장값으로 서빙한다. upstream(실제 하드웨어) 디바이스에 대한 client(master) 역할은 없다.

본 SPEC은 게이트웨이의 각 가상 디바이스가 **실제(upstream) MODBUS 디바이스를 선택적으로 백킹**할 수 있게 확장한다. 백킹된 가상 디바이스는 두 가지 모드로 동작한다.

- **Direct(직접)**: 마스터의 읽기 요청 수신 시 실제 디바이스를 즉시 조회(`SendAndReceive`)하여 응답을 마스터에 전달하고 동시에 `RegisterMap`에 저장한다.
- **Indirect(간접)**: 백그라운드 폴러가 설정된 주기(`poll_interval`)마다 실제 디바이스를 폴링하여 `RegisterMap`을 갱신하고, 마스터의 읽기 요청은 `RegisterMap`의 저장값으로 응답한다.

어느 모드든 최신 레지스터 정보는 항상 가상 디바이스의 `RegisterMap`에 저장된다. 이를 위해 게이트웨이는 실제 디바이스에 대한 **master(client) 역할**을 새로 가지며, 통신은 기존 MODBUS 클라이언트 패키지(`internal/agent/modbus`)의 트랜스포트(`ModbusTransport.SendAndReceive`, TCP/RTU)를 재사용한다(신규 프로토콜 구현·신규 외부 라이브러리 금지).

> **주의(패키지 혼동 금지)**: 본 SPEC의 대상은 서버 `internal/agent/modbusserver`(type id `modbus-gateway`)이다. 클라이언트 `internal/agent/modbus`(type id `modbus-client`)는 **트랜스포트를 재사용**할 뿐 변경 대상이 아니다(§8 Non-Goals).

본 SPEC은 문서만 산출하며 구현 코드를 포함하지 않는다. 기존 `modbus-gateway` 동작과 type id는 반드시 보존한다(하위 호환 HARD, 기존 테스트는 특성화/행위 보존 대상 — DDD/hybrid).

### 1.1 확정 설계 결정 (Locked)

사용자 확정 사항이며 run 단계는 이를 전제로 진행한다. 근거·세부는 §5(명세)·§7(설계 결정)에 상세히 기술한다.

1. **동작 모드(per-virtual-device)**:
   - **Direct**: 마스터 읽기 요청 시 실제 디바이스 즉시 조회 → 응답 전달 + RegisterMap 저장.
   - **Indirect**: 백그라운드 폴러가 `poll_interval`마다 폴링하여 RegisterMap 갱신. 마스터 읽기는 RegisterMap 저장값으로 응답.
2. **연결 끊김 시 응답**:
   - **Direct**: 실제 디바이스 도달 불가 시 대기 없이 **MODBUS 예외 즉각 반환**(`0x0B`).
   - **Indirect**: 마지막 성공 갱신 시각 기준 **설정 타임아웃(`timeout`) 이내면 stale 저장값을 정상 응답**, **타임아웃 초과 시 MODBUS 예외(`0x0B`) 반환**.
3. **예외 코드**: 게이트웨이 의미론에 맞는 `0x0B`(Gateway Target Device Failed to Respond). `internal/agent/modbus`를 변경하지 않기 위해 `modbusserver` 패키지에 로컬 상수로 정의한다(§5.5, §7).
4. **쓰기(write) 처리**: 마스터의 쓰기 요청은 **실제 디바이스로 전달 + RegisterMap에도 저장**(direct/indirect 모두). 쓰기는 stale 개념이 없으므로 **양 모드 모두 upstream에 즉시 전달**하며, 미도달 시 `0x0B` 예외(indirect도 타임아웃과 무관하게 즉각 예외). upstream 성공 시에만 RegisterMap 미러(§5.4, §7).
5. **하위 호환(HARD 게이트)**: **백킹 설정이 없는 기존 가상 디바이스는 오늘과 동일하게 순수 slave로 동작**(RegisterMap 저장값 서빙, upstream 연결 없음). 특성화 테스트로 강제.
6. **재사용**: 실제 디바이스 통신은 modbus **client** 패키지의 트랜스포트(`internal/agent/modbus` `ModbusTransport.SendAndReceive`, TCP/RTU, per-device transport)를 재사용한다.

---

## 2. 환경 (Environment)

| 항목                | 현재 상태 / 제약 (file:line 근거)                                                                                                                            |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 백엔드 언어         | Go 1.23+                                                                                                                                                     |
| 대상 패키지         | `internal/agent/modbusserver/` (type id `modbus-gateway`, 순수 slave)                                                                                        |
| 재사용 패키지(client) | `internal/agent/modbus/` (type id `modbus-client`) — 트랜스포트만 재사용, 변경 금지                                                                       |
| Device 구조체       | `Device`(device_manager.go:17-24): UnitID, Name, `RegisterMap *RegisterMap`, `ReqHandler *RequestHandler`, Stats, RegisterDefs — **upstream 백킹 필드 없음** |
| DeviceManager       | `DeviceManager`(device_manager.go:66-71): `map[byte]*Device` + `sync.RWMutex`; `AddDevice`:211, `RemoveDevice`:226, `GetDevice`:177                          |
| RegisterMap         | `RegisterMap`(register_map.go): 자체 `sync.RWMutex`(register_map.go:63). `ReadTyped`:185/`WriteTyped`:209/`ReadCoils`:282/`WriteCoils`:351/`WriteHoldingRegisters`:365 — "최신 레지스터 저장"은 이미 여기 존재 |
| RequestHandler      | `RequestHandler`(request.go:29-49): `store registerStore` 위에서 서빙. 읽기 `HandleRequest`:59 → `handleReadCoils`:89/`handleReadHoldingRegisters`:127; 쓰기 `HandleWriteRequest`:171 → `handleWriteSingleCoil`:193 등 — **direct 조회 / indirect 서빙 / 예외 반환 hook 지점** |
| registerStore 인터페이스 | `registerStore`(request.go:17-24): `ReadCoils/ReadDiscreteInputs/ReadHoldingRegisters/ReadInputRegisters/WriteCoils/WriteHoldingRegisters` — 백킹 데코레이터가 삽입되는 **핵심 seam** |
| 예외 매핑           | `RequestHandler` 은 store 읽기 오류를 현재 일괄 `ExceptionIllegalDataAddress`(0x02)로 매핑(request.go:96,133 등). `makeExceptionPDU(fc, exCode)`(request.go:395)는 임의 exCode 수용 |
| 예외 상수           | `internal/agent/modbus/protocol.go:55-58`: `0x01`~`0x04`만 존재. **`0x0B`(Gateway Target Failed) 부재** → `modbusserver` 로컬 정의 필요 |
| ModbusHandler       | `ModbusHandler`(handler.go:30-49): 리스너 프레임 디스패치. `GetDevice(unitID)` → `dev.ReqHandler`(handler.go:125)                                             |
| DeviceConfig        | `DeviceConfig`(config.go:51-56): UnitID/Name/RegisterMap/RegisterDefs — **백킹 설정 필드 없음** → 추가 대상                                                   |
| 설정 파싱           | `parseModbusServerConfig`(config.go:90), `parseDevicesConfig`(config.go:257)                                                                                 |
| 트랜스포트(재사용)  | `internal/agent/modbus` `ModbusTransport`(transport.go:18-32): `Connect/SendAndReceive/Close/IsConnected`. `ModbusTCPTransport`(transport.go:36), `NewModbusTCPTransport`(transport.go:59); RTU: `transport_rtu.go`. TCP 트랜스포트는 자체 `sync.Mutex`(transport.go:40)로 `SendAndReceive` 직렬화 |
| 기존 import 선례    | `modbusserver`는 이미 `internal/agent/modbus`를 import(request.go:7, handler.go:11) — 크로스-패키지 재사용 아키텍처 확립됨                                     |
| Process 명령        | `Process`(agent.go:286-338) 문자열 스위치: set_*/get_*/bulk_write/read_raw/list_devices/**add_device**(agent.go:329)/**remove_device**(agent.go:331)/get_device_status. 런타임 add/remove **이미 존재**(`processAddDevice`:1217, `processRemoveDevice`:1288) — 백킹 배선의 신규 훅 지점 |
| 에이전트 수명       | `Init`:134, `Start`:165, `Stop`:200, `Pause`:237, `Resume`:248, `Configure`:1430. `a.mu sync.RWMutex` — 폴러 lifecycle 정합 대상                             |
| 등록 배선           | `register.go` `RegisterModbusServerTypes` → `cmd/xflowd/main.go`, type id `modbus-gateway` 보존                                                              |
| 프론트엔드 스키마   | `web/src/config/agentSchemas.ts`(modbus-gateway) + `ModbusServerDevicesEditor` — 스키마 주도(SPEC-006/008 선례)                                              |
| 개발 방법론         | hybrid — 신규 코드는 TDD, 기존 패키지 편집은 DDD(특성화 테스트). 커버리지 목표 85%, `code_comments: ko`, `go test -race` 클린                                 |

### 2.1 현재 아키텍처 (ANALYZE — 백킹 근거)

- **서빙 경로(순수 slave)**: 리스너 → `ModbusHandler.HandleConnection`(handler.go:55) → `GetDevice(unitID)`(handler.go:125) → `dev.ReqHandler.HandleRequest/HandleWriteRequest` → `registerStore`(=`*RegisterMap` 또는 `*deviceView`) 조회/기록. upstream 통신이 전혀 없다.
- **핵심 seam**: `registerStore` 인터페이스(request.go:17-24)가 `RequestHandler`와 스토리지를 분리한다. 백킹은 이 인터페이스를 만족하는 **데코레이터**로 삽입하는 것이 가장 침습이 적다 — 읽기 시 upstream 조회/저장값 서빙, 쓰기 시 upstream 전달 후 미러링을 데코레이터가 담당하고, `RequestHandler`/`ModbusHandler`는 대부분 불변.
- **예외 반환 한계**: 현재 `RequestHandler`는 store 오류를 `0x02`로 일괄 매핑한다. `0x0B`을 반환하려면 백킹 store가 반환하는 "게이트웨이 대상 실패" 오류를 `RequestHandler`가 식별해야 한다(sentinel 오류 + `errors.Is`) — 이것이 유일한 `RequestHandler` 변경(§5.5).
- **동시성 선례**: `RegisterMap`은 자체 `sync.RWMutex`(register_map.go:63)로 이미 스레드-세이프하다 → 폴러 write와 handler read의 동시 접근이 안전하다. TCP 트랜스포트는 `SendAndReceive`를 자체 mutex로 직렬화한다(transport.go:99-100) → 폴러/direct-read/write가 동일 트랜스포트를 공유해도 직렬화된다.
- **런타임 add/remove 선례**: `processAddDevice`(agent.go:1217)가 이미 `RegisterMap`+`RequestHandler`+`Device`를 생성하여 `AddDevice`한다 → 백킹 store/폴러 배선을 이 경로에 추가한다.

---

## 3. 가정 (Assumptions)

| ID   | 가정 사항                                                                                                                          |
|------|-----------------------------------------------------------------------------------------------------------------------------------|
| A-1  | 기존 `modbus-gateway` type id와 순수 slave 동작은 그대로 유지되며, 본 SPEC은 동일 패키지 내 확장으로만 수행한다(하위 호환 HARD)      |
| A-2  | `backing` 설정 키가 없는 기존 가상 디바이스는 upstream 연결 없이 RegisterMap 저장값을 서빙한다(오늘과 바이트 단위 동일)             |
| A-3  | 백킹 통신은 `internal/agent/modbus`의 `ModbusTransport`를 재사용하며, 신규 프로토콜 구현이나 신규 외부 라이브러리를 도입하지 않는다 |
| A-4  | 백킹 store 데코레이터는 `registerStore` 인터페이스(request.go:17-24)를 만족하여 기존 `RequestHandler` 서빙 경로에 삽입된다          |
| A-5  | `RegisterMap`의 자체 `sync.RWMutex`(register_map.go:63)가 폴러 write와 handler read의 동시 접근을 보호한다                         |
| A-6  | Direct 모드의 upstream 조회와 indirect 모드의 upstream 쓰기는 트랜스포트의 자체 직렬화(transport.go:99-100)에 의존하여 경합이 없다  |
| A-7  | Indirect 폴러 goroutine은 `context.Context` 취소로 종료되며, 에이전트 `Start`/`Stop` 및 디바이스 add/remove 수명과 정합한다        |
| A-8  | `0x0B`(Gateway Target Device Failed to Respond)은 `makeExceptionPDU`(request.go:395)가 이미 임의 exCode를 수용하므로 프레이밍 변경 없이 반환 가능하다 |
| A-9  | upstream unit_id는 가상 디바이스의 서빙 UnitID와 다를 수 있으며, 백킹 설정의 `unit_id`로 별도 지정한다                             |
| A-10 | 프론트엔드는 스키마 주도(`agentSchemas.ts`)로 백킹 필드를 노출하며, 신규 React 컴포넌트를 도입하지 않는다(SPEC-006/008 선례)         |
| A-11 | 백킹 대상 디바이스는 다시 백킹된 게이트웨이가 아니다(다단/cascade 백킹은 범위 밖, §8)                                              |

---

## 4. 요구사항 (Requirements — EARS)

EARS 형식. 요구사항 모듈은 **6개**(REQ-01~06)로 구성한다. 구현 의존 순서는 REQ-01(설정) → REQ-02(master 연결) → REQ-03(Direct) → REQ-04(Indirect) → REQ-05(하위 호환·품질) → REQ-06(프론트엔드)이다.

### REQ-MODBUS-010-01 — 백킹 설정 스키마 (optional; 없으면 순수 slave)

- **Ubiquitous**: 시스템은 항상 가상 디바이스(`DeviceConfig`)에 선택적 upstream 백킹 설정을 허용해야 하며, 백킹 설정은 endpoint(host/port 또는 시리얼 파라미터), transport(`tcp`|`rtu`), upstream `unit_id`, mode(`direct`|`indirect`), indirect의 `poll_interval`·`timeout`을 포함해야 한다.
- **State-driven (기본 = 순수 slave)**: IF 디바이스에 백킹 설정이 없으면 THEN 시스템은 upstream 연결 없이 RegisterMap 저장값을 서빙하는 순수 slave로 동작해야 한다(하위 호환).
- **State-driven (검증)**: IF 백킹 설정이 있으면 THEN `mode`는 필수이며, `mode==indirect`이면 `poll_interval`과 `timeout`을 요구하고, transport에 따라 host/port(tcp) 또는 시리얼 파라미터(rtu)를 요구하여 누락 시 설정 오류로 거부해야 한다.
- **Unwanted**: 시스템은 백킹 설정이 없는 디바이스에 대해 어떤 upstream 연결도 수립하지 않아야 한다.

### REQ-MODBUS-010-02 — upstream master 연결 (트랜스포트 재사용)

- **Ubiquitous**: 시스템은 항상 백킹된 디바이스마다 `internal/agent/modbus`의 `ModbusTransport`(TCP=MBAP, RTU=CRC)를 통해 실제 디바이스에 `SendAndReceive`할 수 있어야 한다(per-device transport).
- **Event-driven (수명)**: WHEN 백킹 디바이스가 생성(설정 시점 또는 런타임 `add_device`)될 때 THEN 시스템은 대응 트랜스포트를 구성·connect하고, 디바이스 제거(런타임 `remove_device`)나 에이전트 `Stop` 시 Close해야 한다.
- **State-driven (재연결)**: IF upstream 연결이 끊기면 THEN 시스템은 후속 요청/폴에서 재연결을 시도해야 하며, 재연결 소유권은 백킹 트랜스포트가 가진다.
- **Unwanted**: 시스템은 신규 외부 modbus 라이브러리를 `go.mod`에 추가하거나 신규 트랜스포트 프로토콜을 구현하지 않아야 한다(재사용만).

### REQ-MODBUS-010-03 — Direct 모드 (실시간 조회)

- **Event-driven (읽기)**: WHEN Direct 백킹 디바이스가 마스터의 읽기 요청(FC01~04)을 받을 때 THEN 시스템은 실제 디바이스를 즉시 조회(`SendAndReceive`)하고, 성공 시 결과를 RegisterMap에 저장하며 그 값을 마스터에 응답해야 한다.
- **Event-driven (쓰기)**: WHEN Direct 백킹 디바이스가 마스터의 쓰기 요청(FC05/06/15/16)을 받을 때 THEN 시스템은 요청을 실제 디바이스로 전달하고, 성공 시에만 RegisterMap에 미러 저장한 뒤 정상 응답해야 한다.
- **Unwanted (끊김 = 즉각 예외)**: IF Direct 모드에서 실제 디바이스가 도달 불가하면 THEN 시스템은 대기 없이 MODBUS 예외 `0x0B`(Gateway Target Device Failed to Respond)를 즉각 반환해야 하며, 실패한 쓰기는 RegisterMap을 변형하지 않아야 한다.

### REQ-MODBUS-010-04 — Indirect 모드 (주기 폴링 + stale 허용)

- **Ubiquitous (폴링)**: 시스템은 항상 Indirect 백킹 디바이스마다 백그라운드 폴러를 두어 `poll_interval`마다 실제 디바이스를 폴링하고 RegisterMap을 갱신해야 하며, 마지막 성공 갱신 시각을 기록해야 한다.
- **Event-driven (읽기 서빙)**: WHEN Indirect 백킹 디바이스가 마스터의 읽기 요청을 받을 때 THEN 시스템은 RegisterMap의 저장값으로 응답해야 한다.
- **State-driven (stale 허용/초과)**: IF 마지막 성공 갱신 시각으로부터 경과 시간이 `timeout` 이내이면 THEN 시스템은 (stale일지라도) 저장값을 정상 응답해야 하고, IF `timeout`을 초과하면 THEN MODBUS 예외 `0x0B`을 반환해야 한다.
- **Event-driven (쓰기)**: WHEN Indirect 백킹 디바이스가 마스터의 쓰기 요청을 받을 때 THEN 시스템은 쓰기를 실제 디바이스로 즉시 전달하고(폴 캐시 우회), 성공 시에만 RegisterMap에 미러 저장해야 한다. IF upstream 미도달이면 THEN `timeout`과 무관하게 `0x0B` 예외를 반환해야 한다(쓰기에는 stale 개념 없음).
- **Unwanted**: 시스템은 폴러 goroutine을 에이전트/디바이스 수명과 무관하게 누수시키지 않아야 하며(Stop/remove 시 종료), RegisterMap 갱신과 handler 서빙 간 데이터 경합을 유발하지 않아야 한다.

### REQ-MODBUS-010-05 — 하위 호환 · 예외 코드 · 동시성 · 품질 (cross-cutting)

- **Ubiquitous (하위 호환 HARD)**: 시스템은 항상 백킹 설정이 없는 기존 `modbus-gateway` 디바이스를 오늘과 동일하게(바이트 단위) 순수 slave로 동작시켜야 한다.
- **Ubiquitous (예외 코드)**: 시스템은 항상 게이트웨이 백킹 실패에 대해 `0x0B`(Gateway Target Device Failed to Respond)을 사용해야 하며, 이 상수는 `internal/agent/modbus` 변경 없이 `modbusserver` 패키지에 정의되어야 한다.
- **State-driven (동시성)**: IF 폴러가 RegisterMap을 갱신하고 handler가 동시에 서빙하면 THEN 접근은 RegisterMap의 자체 뮤텍스로 보호되어야 하고, 마지막 성공 시각·백킹 상태는 원자적/뮤텍스 보호 하에 갱신되어야 하며, 폴러 lifecycle은 `a.mu` 보호 하에 관리되어야 한다.
- **Ubiquitous (등록)**: 시스템은 항상 `modbus-gateway` type id를 보존해야 한다(`register.go`/`cmd/xflowd/main.go`).
- **Unwanted**: 시스템은 `go test -race`에서 경합을 보고하지 않아야 하며, 런타임 백킹 상태(트랜스포트 맵, 폴러 집합, 마지막 성공 시각)를 뮤텍스/원자적 보호 없이 수정하지 않아야 한다.

### REQ-MODBUS-010-06 — 프론트엔드 백킹 설정 노출 (스키마 주도)

- **Optional (스키마 주도)**: 가능하면 시스템은 `agentSchemas.ts`의 `modbus-gateway` 스키마와 `ModbusServerDevicesEditor`에 백킹 설정 필드(endpoint, transport, upstream unit_id, mode, poll_interval, timeout)를 노출하여 UI에서 편집 가능하게 해야 한다(신규 React 컴포넌트 없이).
- **Unwanted**: 시스템은 프론트엔드 노출을 위해 백엔드 백킹 의미론(direct/indirect, 예외 규칙)을 변경하지 않아야 한다.

---

## 5. 명세 (Specifications)

### 5.1 백킹 설정 스키마 (REQ-01)

`DeviceConfig`(config.go:51-56)에 선택적 `Backing *BackingConfig` 필드를 추가한다(nil = 순수 slave). 제안 구조:

```
BackingConfig{
  Transport   string        // "tcp" | "rtu" (기본 "tcp")
  Host        string        // TCP
  Port        int           // TCP
  Serial      SerialConfig  // RTU (config.go:42 재사용)
  UnitID      byte          // upstream 디바이스 unit id (가상 UnitID 와 독립)
  Mode        string        // "direct" | "indirect" (필수)
  PollInterval time.Duration // indirect 전용, > 0
  Timeout      time.Duration // indirect: stale 허용 한도 + upstream 요청 타임아웃; direct: upstream 요청 타임아웃
}
```

`parseDevicesConfig`(config.go:257)가 device 맵의 `backing` 키를 파싱·검증한다: `mode` 필수, `indirect`이면 `poll_interval`/`timeout` 필수, transport에 따라 host/port(tcp) 또는 serial(rtu) 필수. `backing` 키 부재 시 `Backing=nil`(하위 호환).

### 5.2 upstream master 연결 (REQ-02)

- 백킹 디바이스마다 `internal/agent/modbus`의 트랜스포트를 생성한다: TCP는 `NewModbusTCPTransport(host, port, timeout, logger)`(transport.go:59), RTU는 `NewModbusRTUTransport(...)`(transport_rtu.go). 이는 `internal/agent/modbus`가 이미 export하는 생성자를 사용하며 client 패키지를 변경하지 않는다.
- 연결 수명: 디바이스 생성 시(`Start` 또는 `add_device`) `Connect`, 제거/`Stop` 시 `Close`. 재연결은 트랜스포트가 소유(direct는 다음 요청에서, indirect는 다음 폴에서 재시도).
- 트랜스포트는 `RegisterMap`을 백킹하는 신규 store(§5.3)와 폴러(§5.4)가 공유한다.

### 5.3 Direct 모드 + 백킹 store 데코레이터 (REQ-03)

핵심 설계: `registerStore`(request.go:17-24)를 만족하는 **백킹 데코레이터**(예: `backedStore`)를 도입하여 기존 서빙 경로에 삽입한다. 순수 slave 디바이스는 오늘처럼 `*RegisterMap`/`*deviceView`를 직접 사용하고, 백킹 디바이스만 `backedStore`로 감싼다.

- **Direct 읽기**: `backedStore.ReadHoldingRegisters(start, qty)` 등이 → upstream `SendAndReceive`(대응 FC로 요청 PDU 구성) → 성공 시 결과를 내부 `*RegisterMap`에 `WriteXxx`로 저장하고 값을 반환. 실패 시 sentinel 오류 `ErrGatewayTargetFailed` 반환.
- **Direct 쓰기**: `backedStore.WriteHoldingRegisters(start, values)` 등이 → upstream 쓰기 `SendAndReceive`(FC06/16) → 성공 시 내부 `*RegisterMap`에 미러 저장 후 `ChangeSet` 반환. 실패 시 `ErrGatewayTargetFailed` 반환(RegisterMap 미변형).
- upstream 요청 PDU/응답 디코딩은 `internal/agent/modbus`의 PDU 헬퍼(protocol.go/write.go)를 재사용하거나, 트랜스포트가 PDU-중립이므로 `modbusserver` 측에서 FC별 PDU를 조립한다(재사용 우선; 상세는 design.md §설계).

### 5.4 Indirect 모드 + 폴러 (REQ-04)

- **폴러 goroutine**: 백킹 indirect 디바이스마다 하나의 폴러를 둔다. `time.Ticker`(period=`poll_interval`) + `context.Context` 취소. 각 tick마다 설정된 register 영역을 upstream에서 조회하여 내부 `*RegisterMap`을 갱신하고, 성공 시 `lastSuccess`(원자적/뮤텍스) 갱신.
- **읽기 서빙**: `backedStore(indirect)`의 Read 경로는 upstream을 호출하지 않고 내부 `*RegisterMap`에서 읽는다. 단, 서빙 직전 `now - lastSuccess <= timeout`을 검사하여 초과 시 `ErrGatewayTargetFailed` 반환, 이내면 저장값(stale 포함) 반환.
- **쓰기**: indirect 쓰기는 direct 쓰기와 동일하게 upstream에 **즉시 전달**한다(폴 캐시 우회). 성공 시 내부 `*RegisterMap` 미러, 실패 시 `timeout` 무관 `ErrGatewayTargetFailed`.
- **lifecycle**: 폴러 시작 = 디바이스 생성 후 `Start`/`add_device` 시, 중지 = `remove_device`/`Stop`. `a.mu` 보호 하에 폴러 핸들(취소 함수) 등록/해제. 폴러 goroutine 종료는 context 취소로 관측 가능하게 한다.

### 5.5 예외 코드 매핑 (REQ-05)

- `modbusserver` 패키지에 로컬 상수 `ExceptionGatewayTargetFailed byte = 0x0B`를 정의한다(`internal/agent/modbus` 미변경).
- `RequestHandler`의 읽기/쓰기 오류 매핑을 확장한다: store가 반환한 오류가 `errors.Is(err, ErrGatewayTargetFailed)`이면 `makeExceptionPDU(fc, 0x0B)`, 아니면 기존 매핑(`0x02` 등) 유지. 이것이 `RequestHandler`의 유일한 변경이며, 순수 slave 경로는 `ErrGatewayTargetFailed`를 결코 반환하지 않으므로 기존 동작 불변.

### 5.6 런타임 배선 (REQ-02/04/05)

- `processAddDevice`(agent.go:1217): `backing` 파라미터가 있으면 트랜스포트 생성·connect → `backedStore` 구성 → `RequestHandler(backedStore)` → (indirect면) 폴러 시작 → `AddDevice`. 없으면 기존 경로(순수 RegisterMap store).
- `processRemoveDevice`(agent.go:1288): 백킹 디바이스면 폴러 중지 + 트랜스포트 Close 후 `RemoveDevice`.
- `Start`(agent.go:165)/`Stop`(agent.go:200): 설정 시점 백킹 디바이스의 connect/폴러-시작 및 close/폴러-중지를 수명에 편입.
- `NewDeviceManager`(device_manager.go:95)/`Start` 경로: 설정 device가 `Backing`을 가지면 동일한 `backedStore` 배선을 적용.

### 5.7 프론트엔드 (REQ-06)

- `agentSchemas.ts`의 `modbus-gateway` device 스키마에 optional `backing` 오브젝트(transport/host/port/serial/unit_id/mode/poll_interval/timeout) 추가. `ModbusServerDevicesEditor`가 스키마를 렌더한다. mode에 따른 조건부 필드(indirect → poll_interval/timeout) 노출은 스키마/에디터 규약을 따른다.

---

## 6. 추적성 (Traceability)

| 요구사항 모듈          | 명세 절        | 수용 시나리오(acceptance.md)   | 주요 파일(reuse map은 plan.md 참조)                                              |
|------------------------|----------------|--------------------------------|----------------------------------------------------------------------------------|
| REQ-MODBUS-010-01      | 5.1            | AC-01, AC-02                   | config.go(DeviceConfig, BackingConfig, parseDevicesConfig)                        |
| REQ-MODBUS-010-02      | 5.2, 5.6       | AC-03, AC-11                   | agent.go(Start/Stop/processAddDevice/processRemoveDevice), (재사용) modbus/transport.go |
| REQ-MODBUS-010-03      | 5.3, 5.5       | AC-04, AC-05, AC-06            | (신규) backedStore, request.go(registerStore, 예외 매핑), errors.go               |
| REQ-MODBUS-010-04      | 5.4, 5.5       | AC-07, AC-08, AC-09, AC-10     | (신규) backedStore/poller, register_map.go, agent.go(폴러 lifecycle)              |
| REQ-MODBUS-010-05      | 5.5, 5.6       | AC-12, AC-13                   | request.go, errors.go, register.go, agent.go(a.mu), device_manager.go            |
| REQ-MODBUS-010-06      | 5.7            | AC-13                          | web/src/config/agentSchemas.ts, ModbusServerDevicesEditor                        |

---

## 7. 설계 결정 (Design Decisions)

> 사용자 확정 사항이며, run 단계는 아래를 전제로 진행한다.

1. **백킹 삽입점 = `registerStore` 데코레이터**: `RequestHandler`/`ModbusHandler`를 최소 변경하기 위해 `registerStore` 인터페이스(request.go:17)를 만족하는 `backedStore`로 삽입한다. 대안(핸들러 분기, 디바이스 레벨 분기)은 서빙 경로 침습이 크므로 미채택.
2. **예외 코드 = `0x0B`, `modbusserver` 로컬 정의**: 게이트웨이 의미론상 upstream 미도달은 `0x0B`(Gateway Target Device Failed to Respond)이 정확하다. `internal/agent/modbus` 미변경 제약(Non-Goal) 때문에 상수를 `modbusserver`에 정의한다. `makeExceptionPDU`가 임의 exCode를 수용하므로 프레이밍 변경 불필요.
3. **예외 식별 = sentinel 오류 + `errors.Is`**: 현재 store 오류가 일괄 `0x02`로 매핑되므로, 백킹 실패만 `0x0B`으로 매핑하려면 `ErrGatewayTargetFailed` sentinel을 `RequestHandler`가 식별해야 한다. 순수 slave 경로는 이 오류를 반환하지 않으므로 기존 동작 불변.
4. **write-on-disconnect 세부**: 쓰기는 stale 개념이 없으므로 **양 모드 모두 upstream 즉시 전달**한다. direct=즉각 예외(0x0B)는 자명하고, indirect도 쓰기는 폴 캐시를 우회하여 즉시 upstream에 전달하므로 `timeout` 이내라도 미도달 시 즉각 0x0B. upstream 성공 시에만 RegisterMap 미러(부분 적용 없음).
5. **stale 판정 = `lastSuccess` 기반(indirect read 전용)**: stale 허용은 **읽기**에만 적용된다. `now - lastSuccess <= timeout`이면 저장값 서빙, 초과면 0x0B. 쓰기·direct 읽기에는 적용되지 않는다.
6. **폴러 lifecycle = per-device goroutine + context 취소**: indirect 디바이스마다 폴러 하나. 시작=생성 시, 중지=제거/Stop 시. `a.mu`로 폴러 핸들 관리, RegisterMap은 자체 뮤텍스로 데이터 경합 보호. 트랜스포트 자체 직렬화가 upstream 접근 경합을 흡수.
7. **RegisterMap 동시성 모델**: RegisterMap(register_map.go:63)의 기존 `sync.RWMutex`를 재사용한다 — 폴러 write(WriteTyped/WriteHoldingRegisters 등)와 handler read(ReadCoils 등)가 동일 락 하에서 안전. 백킹 상태(`lastSuccess`, 연결 플래그)는 별도 원자/뮤텍스로 보호.

---

## 8. 범위 밖 (Non-Goals)

- MODBUS 클라이언트 패키지(`internal/agent/modbus`, type id `modbus-client`) 변경 — 트랜스포트/PDU 헬퍼는 **재사용만** 한다.
- 신규 외부 modbus 라이브러리 도입 또는 신규 트랜스포트 프로토콜 구현.
- 기존 `modbus-gateway` type id 변경 또는 신규 패키지 복제.
- 백킹 디바이스의 다단(cascade) 백킹(백킹 대상이 또 다른 백킹 게이트웨이인 경우).
- 실제 하드웨어 RTU 타이밍 정확성 튜닝(잔여 위험으로 plan.md에 기록).
- 백킹 실패/폴 통계의 영구 저장·외부 전송(로그 싱크는 기존 slog 경로 사용).
