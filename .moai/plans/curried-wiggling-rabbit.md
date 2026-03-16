# Plan: modbus-writer / modbus-poller 전용 노드 구현

## Context

현재 `modbus` 노드는 read/write를 하나의 ProcessNode에서 처리한다. MQTT에서 `mqtt-subscriber`/`mqtt-publisher`로 분리한 것처럼, Modbus도 전용 노드로 분리하여 사용성을 개선한다.

- **modbus-writer**: 쓰기 전용 ProcessNode (입력 메시지 → 레지스터 쓰기)
- **modbus-poller**: 주기적 읽기 SourceNode (poll_interval + register_map → 자동 폴링)

기존 `modbus` 노드는 그대로 유지한다 (하위 호환).

---

## Milestone 1: modbus-poller SourceNode 구현 [Completed]

**목표**: poll_interval 기반 주기적 레지스터 읽기 SourceNode 완성

**파일**: [modbus_poller.go](internal/node/modbus_poller.go)

### 구조체 설계

```
RegisterMapEntry:
  Name         string   // 필수
  RegisterArea string   // 기본: holding_registers (항목별 지정)
  Address      uint16   // 시작 주소
  Count        uint16   // 기본: 1
  DataType     string   // 기본: uint16
  ByteOrder    string   // 기본: big_endian
  DeviceID     uint8    // 기본: config.DeviceID 상속

ModbusPollerConfig:
  AgentRef     string               // 필수
  DeviceID     uint8                // 기본: 1 (register_map 항목의 기본값)
  PollInterval string               // 기본: "5s" (time.ParseDuration)
  Timeout      string               // 기본: "5s"
  RegisterMap  []RegisterMapEntry   // 필수

ModbusPollerNode:
  modbusNodeBase
  pollerConfig  ModbusPollerConfig
  pollInterval  time.Duration
  sourceCh      chan message.Message  // 버퍼 64
  stopCh        chan struct{}
  stopOnce      sync.Once
  reconfigCh    chan struct{}         // poll_interval 변경 시 ticker 리셋
```

### 설계 결정

- **register_map 전용**: 개별 레지스터 필드(register_area, address, count, data_type, byte_order) 제거. 모든 읽기는 register_map으로 정의
- **항목별 register_area/device_id**: 각 register_map 항목에서 레지스터 영역과 디바이스 ID를 개별 지정 가능
- **pollSingle 제거**: register_map이 필수이므로 단일 읽기 경로 불필요
- **동적 설정 변경**: in 포트로 device_id, poll_interval, register_map 오버라이드 가능

### 완료 항목

- [x] RegisterMapEntry 구조체 (register_area, device_id 포함)
- [x] ModbusPollerConfig 구조체 (register_map 전용)
- [x] NewModbusPollerNode 팩토리 함수
- [x] Configure() — agent_ref 필수, register_map 필수, poll_interval 최소 100ms
- [x] Init() — AgentResolver → agent 타입 감지, pollLoop 시작
- [x] pollLoop() — ticker + stopCh + reconfigCh
- [x] pollRegisterMap() — 항목별 읽기, 이름 기반 payload 조립
- [x] buildPollerReadCommand() — Server/Client별 읽기 명령
- [x] Process() — device_id, poll_interval, register_map 동적 오버라이드
- [x] SourceCh(), Shutdown() (sync.Once)
- [x] parseRegisterMap() — register_area, device_id 파싱 포함
- [x] 센티널 에러: ErrModbusPollerMissingAgentRef, ErrModbusPollerInvalidPollInterval, ErrModbusPollerMissingRegisterMap
- [x] 컴파일 타임 인터페이스 체크 (Node, SourceNode)

---

## Milestone 2: modbus-writer ProcessNode 구현 [Completed]

**목표**: 쓰기 전용 ProcessNode 완성

**파일**: [modbus_writer.go](internal/node/modbus_writer.go)

### 구조체 설계

```
ModbusWriterConfig:
  AgentRef     string   // 필수
  RegisterArea string   // 기본: holding_registers, 쓰기 가능 영역만
  Address      uint16
  Count        uint16   // 기본: 1
  DataType     string   // 기본: uint16
  ByteOrder    string   // 기본: big_endian
  DeviceID     uint8    // 기본: 1
  Timeout      string   // 기본: "5s"

ModbusWriterNode:
  modbusNodeBase
  writerConfig  ModbusWriterConfig
```

### 설계 결정

- **출력 메시지 최적화**: count 필드 미포함 (data_type에서 유추 가능), byte_order는 기본값(big_endian)일 때 생략
- **strip_nulls 호환**: value/values 없으면 조용히 스킵 (`return nil, nil`)
- **메시지 오버라이드**: applyMessageOverrides로 address, count, data_type, byte_order, device_id, register_area 런타임 변경

### 완료 항목

- [x] ModbusWriterConfig, ModbusWriterNode 구조체 정의
- [x] NewModbusWriterNode 팩토리 함수
- [x] Configure() — agent_ref 필수, 쓰기 가능 영역만 (coils, holding_registers)
- [x] Init() — AgentResolver → agent 타입 감지
- [x] Process() — 값 추출, 명령 생성, agent 호출, 출력 메시지
- [x] Shutdown()
- [x] buildModbusServerWriteCommand(), buildModbusClientWriteCommand()
- [x] 센티널 에러: ErrModbusWriterMissingAgentRef, ErrModbusWriterInvalidRegisterArea
- [x] 컴파일 타임 인터페이스 체크

---

## Milestone 3: 공통 유틸리티 추출 [Completed]

**목표**: modbus.go, modbus_poller.go, modbus_writer.go 간 중복 코드 제거

**파일**: [modbus_common.go](internal/node/modbus_common.go)

### 추출된 공통 코드

- `modbusNodeBase` 구조체 (BaseNode, agent, agentType, timeout, mu 내장)
- `initModbusResolver()` — 옵션에서 resolver 추출
- `resolveModbusAgent()` — AgentResolver → AgentAccessor → agent 타입 감지
- `callModbusAgentProcess()` — goroutine + select + timeout
- `modbusShutdown()` — lifecycle 전이
- `applyMessageOverrides()` — 메시지 오버라이드 로직
- `extractReadResult()` — 읽기 응답에서 결과 추출
- 공통 상수/맵: areaToFunctionCode, validRegisterAreas, readOnlyAreas, booleanAreas 등

---

## Milestone 4: 노드 레지스트리 등록 [Completed]

**파일**: [registry.go](internal/node/registry.go)

`registerBuiltins()`에 추가 완료:
```
{"modbus-poller", NewModbusPollerNode, "io", "MODBUS 레지스터를 주기적으로 폴링 읽기"}
{"modbus-writer", NewModbusWriterNode, "io", "MODBUS 레지스터 쓰기 전용"}
```

---

## Milestone 5: 프론트엔드 스키마 [Completed]

### 5.1 nodeSchemas.ts

**파일**: [nodeSchemas.ts](web/src/config/nodeSchemas.ts)

**modbus-poller** 스키마:
- agent_ref: agent_select, required
- device_id: number, default: 1 (register_map 항목의 기본값)
- poll_interval: string, default: "5s"
- register_map: register_map, required (항목별 register_area, device_id 지정)
- defaultPorts: [in, out, error]

**modbus-writer** 스키마:
- agent_ref: agent_select, required
- register_area: select (coils, holding_registers), default: holding_registers
- address: number, default: 0
- data_type: select (5가지), default: uint16
- byte_order: select (2가지), default: big_endian
- device_id: number, default: 1
- defaultPorts: [in, out, error]

### 5.2 nodeTypeMeta.ts

**파일**: [nodeTypeMeta.ts](web/src/pages/nodes/nodeTypeMeta.ts)

modbus-poller, modbus-writer 메타데이터 추가 완료.

---

## Milestone 6: 테스트 [Completed]

**파일**: [modbus_poller_test.go](internal/node/modbus_poller_test.go), [modbus_writer_test.go](internal/node/modbus_writer_test.go)

### modbus-poller 테스트

- [x] TestNewModbusPollerNode_Basic
- [x] TestModbusPollerNode_Configure_Valid
- [x] TestModbusPollerNode_Configure_Defaults (register_map 항목 기본값 포함)
- [x] TestModbusPollerNode_Configure_MissingAgentRef
- [x] TestModbusPollerNode_Configure_MissingRegisterMap
- [x] TestModbusPollerNode_Configure_InvalidPollInterval
- [x] TestModbusPollerNode_Configure_WithRegisterMap (register_area, device_id 항목별)
- [x] TestModbusPollerNode_Init_Success
- [x] TestModbusPollerNode_Init_NoResolver
- [x] TestModbusPollerNode_PollLoop_RegisterMap
- [x] TestModbusPollerNode_Process_OverrideDeviceID
- [x] TestModbusPollerNode_Process_OverridePollInterval
- [x] TestModbusPollerNode_Process_OverridePollInterval_TooShort
- [x] TestModbusPollerNode_Process_OverrideRegisterMap
- [x] TestModbusPollerNode_Process_NilPayload
- [x] TestModbusPollerNode_Shutdown

### modbus-writer 테스트

- [x] TestNewModbusWriterNode_Basic
- [x] TestModbusWriterNode_Configure_Valid
- [x] TestModbusWriterNode_Configure_MissingAgentRef
- [x] TestModbusWriterNode_Configure_ReadOnlyArea
- [x] TestModbusWriterNode_Init_Success
- [x] TestModbusWriterNode_Process_ServerSingleWrite
- [x] TestModbusWriterNode_Process_ServerBulkWrite
- [x] TestModbusWriterNode_Process_ClientWrite
- [x] TestModbusWriterNode_Process_MessageOverride
- [x] TestModbusWriterNode_Process_ReadOnlyOverride
- [x] TestModbusWriterNode_Process_NoValue (조용히 스킵)
- [x] TestModbusWriterNode_Process_CoilWrite
- [x] TestModbusWriterNode_Process_OutputPayload (count 미포함, byte_order 조건부)
- [x] TestModbusWriterNode_Shutdown

---

## Milestone 7: 예제 플로우 [Completed]

**파일**: [modbus-poller-node.yaml](examples/flows/modbus-poller-node.yaml), [modbus-writer-node.yaml](examples/flows/modbus-writer-node.yaml)

---

## Files

| File | Action | Status | Description |
|------|--------|--------|-------------|
| [modbus_common.go](internal/node/modbus_common.go) | Create | Done | 공통 유틸리티 (modbusNodeBase, 명령 빌더) |
| [modbus_poller.go](internal/node/modbus_poller.go) | Create | Done | SourceNode 주기적 폴링 읽기 (register_map 전용) |
| [modbus_writer.go](internal/node/modbus_writer.go) | Create | Done | ProcessNode 쓰기 전용 |
| [modbus.go](internal/node/modbus.go) | Edit | Done | 공통 코드를 modbus_common.go로 이동 |
| [registry.go](internal/node/registry.go) | Edit | Done | modbus-poller, modbus-writer 등록 |
| [nodeSchemas.ts](web/src/config/nodeSchemas.ts) | Edit | Done | 프론트엔드 스키마 (poller: register_map 전용) |
| [nodeTypeMeta.ts](web/src/pages/nodes/nodeTypeMeta.ts) | Edit | Done | 프론트엔드 메타데이터 |
| [modbus_poller_test.go](internal/node/modbus_poller_test.go) | Create | Done | 폴러 테스트 (16개) |
| [modbus_writer_test.go](internal/node/modbus_writer_test.go) | Create | Done | 라이터 테스트 (14개) |
| [modbus-poller-node.yaml](examples/flows/modbus-poller-node.yaml) | Create | Done | 폴러 예제 플로우 |
| [modbus-writer-node.yaml](examples/flows/modbus-writer-node.yaml) | Create | Done | 라이터 예제 플로우 |
| [modbus-gateway-server.yaml](examples/agents/modbus-gateway-server.yaml) | Edit | Done | holding_registers count 20→48 |

## Verification

- [x] `go build ./cmd/xflowd` — 백엔드 빌드 확인
- [x] `go test ./internal/node/...` — 기존 modbus 테스트 + 신규 테스트 통과
- [x] `go test -race ./internal/node/...` — 동시성 안전 확인
- [x] `npx tsc --noEmit` (web/) — 프론트엔드 타입 체크
- [x] 기존 modbus 노드 동작 변경 없음 확인 (하위 호환)
