# SPEC-MODBUS-004: MODBUS Reader/Writer Processing Node

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-004 |
| 제목 | MODBUS Reader/Writer Processing Node (`modbus_rw`) |
| 버전 | 1.0.0 |
| 상태 | Completed |
| 우선순위 | High |
| 카테고리 | Backend + Frontend |
| 관련 SPEC | SPEC-MODBUS-001 (Client), SPEC-MODBUS-002 (Server), SPEC-MODBUS-003 (Multi-Data-Type) |
| 패키지 | `internal/node/modbus_rw.go`, `web/src/config/nodeSchemas.ts`, `web/src/pages/nodes/nodeTypeMeta.ts` |
| 생성일 | 2026-03-11 |
| 작성자 | xtra |

---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-03-11 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-03-11 | xtra | 구현 완료, 상태 Completed로 변경 |

---

## 1. Environment (환경)

### 1.1 현재 시스템 상태

#### 1.1.1 MODBUS Agent 시스템

MODBUS/TCP Client Agent (SPEC-MODBUS-001)와 MODBUS/TCP Server Agent (SPEC-MODBUS-002)가 운영 중이며, SPEC-MODBUS-003에 의해 다중 데이터 타입(uint16, int16, float32, uint32, int32)을 지원한다.

- **Client Agent** (`internal/agent/modbus/`): PLC/센서 등 원격 MODBUS 슬레이브 디바이스의 레지스터를 폴링하고, 캐시 기반 최적화를 제공한다.
- **Server Agent** (`internal/agent/modbusserver/`): xflow를 MODBUS/TCP 서버로 동작시켜 외부 SCADA/HMI 시스템이 레지스터를 읽기/쓰기할 수 있다.
- **공유 타입 패키지** (`internal/modbus/`): uint16, int16, float32, uint32, int32 레지스터 변환 유틸리티를 제공한다.

#### 1.1.2 노드 시스템

현재 11개의 빌트인 노드가 `internal/node/registry.go`에 등록되어 있다:

| 노드 | 카테고리 | 설명 |
|------|---------|------|
| filter | processing | 조건에 따라 메시지를 필터링 |
| transform | processing | 메시지 데이터를 변환 |
| switch | routing | 조건에 따라 메시지를 라우팅 |
| bridge | io | 외부 에이전트와 메시지 송수신 |
| script | processing | 스크립트로 메시지를 처리 |
| catch | error | 에러 메시지를 캐치하여 처리 |
| aggregate | processing | 여러 메시지를 집계 |
| mapping | processing | 키 기반 값 매핑 |
| debug | debug | 메시지를 디버그 출력 |
| status | debug | 플로우 상태를 모니터링 |
| deadletter | error | 처리 실패 메시지를 보관 |

#### 1.1.3 Agent 통신 인터페이스

`internal/node/bridge.go`에 정의된 Agent 통신 인터페이스:

- `AgentResolver`: 에이전트 참조를 `AgentTransport`로 해석
- `AgentTransport`: 에이전트와의 Send/Receive 통신
- `AgentAccessor`: `AgentTransport`에서 원본 `agent.Agent` 객체에 접근

`agent.Agent` 인터페이스의 `Process(ctx, msg)` 메서드를 통해 에이전트에 명령을 직접 전달할 수 있다.

### 1.2 현재 제약사항

| 항목 | 현재 상태 | 제약 |
|------|-----------|------|
| Bridge Node | Agent 연결 전용 (in/out/inout/request_reply) | 특정 레지스터 읽기/쓰기 로직 없음 |
| MODBUS 데이터 접근 | Bridge 노드로 Agent 전체 데이터 수신만 가능 | 개별 레지스터 주소 지정 읽기/쓰기 불가 |
| 플로우 내 MODBUS 제어 | Bridge + Script 조합으로 가능하나 복잡 | 전용 노드 없음 |

### 1.3 MODBUS 레지스터 영역

| 영역 | 접근 | 데이터 타입 | Function Code (Client) | Function Code (Server) |
|------|------|-----------|----------------------|----------------------|
| Coils | Read/Write | Boolean | FC01(Read), FC05/FC15(Write) | get_coils, set_coil/set_coils |
| Discrete Inputs | Read Only | Boolean | FC02(Read) | get_discrete_inputs |
| Holding Registers | Read/Write | uint16+ | FC03(Read), FC06/FC16(Write) | get_holding_registers, set_register/set_registers |
| Input Registers | Read Only | uint16+ | FC04(Read) | get_input_registers, set_input/set_inputs |

### 1.4 Agent Process() 명령 체계

#### Server Agent Process() 명령

| 명령 | 용도 | 주요 파라미터 |
|------|------|-------------|
| `get_coils` | 코일 읽기 | address, count |
| `get_discrete_inputs` | 이산 입력 읽기 | address, count |
| `get_holding_registers` | 홀딩 레지스터 읽기 | address, count |
| `get_input_registers` | 입력 레지스터 읽기 | address, count |
| `get_register_typed` | 타입 변환 레지스터 읽기 | address, data_type, byte_order |
| `set_coil` | 단일 코일 쓰기 | address, value |
| `set_coils` | 다중 코일 쓰기 | address, values |
| `set_register` | 단일 홀딩 레지스터 쓰기 | address, value, data_type |
| `set_registers` | 다중 홀딩 레지스터 쓰기 | address, values, data_type |
| `set_input` | 입력 레지스터 설정 | address, value |
| `set_inputs` | 다중 입력 레지스터 설정 | address, values |
| `bulk_write` | 배치 쓰기 | operations[] |

#### Client Agent Process() 명령

| 명령 | 용도 | 주요 파라미터 |
|------|------|-------------|
| `read_registers` | 레지스터 읽기 (캐시/강제/직접) | device_id, function_code, address, count, mode |
| `write_coil` | 단일 코일 쓰기 | device_id, address, value |
| `write_register` | 단일 레지스터 쓰기 | device_id, address, value |
| `write_coils` | 다중 코일 쓰기 | device_id, address, values |
| `write_registers` | 다중 레지스터 쓰기 | device_id, address, values |

---

## 2. Assumptions (가정)

### 2.1 설계 가정

- **A1**: `modbus_rw` 노드는 단일 노드로 설계하여 읽기와 쓰기를 모두 처리한다. operation 설정에 따라 동작이 결정된다.
- **A2**: 노드는 `AgentResolver`를 통해 MODBUS Agent를 resolve하고, `AgentAccessor`로 원본 Agent 객체를 얻어 `Process()` 메서드를 직접 호출한다.
- **A3**: Agent 타입 자동 감지는 `agent.Agent` 인터페이스의 실제 타입(MODBUSAgent vs MODBUSServerAgent)을 확인하여 적절한 Process() 명령을 선택한다.
- **A4**: WRITE 연산 시 값은 입력 메시지의 payload에서 `value` 또는 `values` 키로 추출한다.
- **A5**: READ 연산 시 결과는 출력 메시지의 payload에 `result` 키로 저장하고, 원본 메시지의 기존 payload 데이터를 보존한다.

### 2.2 기술 가정

- **A6**: 기존 `AgentResolver`, `AgentTransport`, `AgentAccessor` 인터페이스를 그대로 재사용한다. 새로운 인터페이스 추가는 불필요하다.
- **A7**: 데이터 타입 변환은 `internal/modbus/` 공유 패키지의 기존 함수를 활용한다.
- **A8**: Client Agent의 경우 `device_id`는 노드 설정의 `device_id` 필드 또는 입력 메시지의 payload `device_id` 키에서 추출한다.
- **A9**: 노드 카테고리는 `processing`으로 분류한다 (데이터를 변환/가공하는 성격).
- **A10**: 11개 기존 빌트인 노드에 추가되어 12번째 빌트인 노드가 된다.

### 2.3 프론트엔드 가정

- **A11**: `nodeSchemas.ts`에 `modbus_rw` 스키마를 추가한다. `agent_select` 타입 필드로 에이전트를 선택한다.
- **A12**: `nodeTypeMeta.ts`에 `modbus_rw` 메타데이터를 추가하여 노드 상세 정보 페이지에서 사용한다.

---

## 3. Requirements (요구사항)

### 3.1 Module 1: 노드 등록 및 설정 (Node Registration & Configuration) [R-MBRW-001 ~ R-MBRW-006]

**R-MBRW-001** (Ubiquitous):
시스템은 **항상** `modbus_rw` 타입 노드를 `processing` 카테고리의 빌트인 노드로 등록해야 한다.

**R-MBRW-002** (Event-Driven):
**WHEN** `modbus_rw` 노드가 Configure()를 통해 설정될 **THEN** 다음 설정 필드를 파싱해야 한다:
- `agent_ref` (string, 필수): 대상 MODBUS Agent 이름/ID
- `operation` (string, 필수): "read" 또는 "write"
- `register_area` (string, 필수): "coils", "discrete_inputs", "holding_registers", "input_registers"
- `address` (number, 필수): 시작 레지스터 주소
- `count` (number, 선택, 기본값 1): 읽기/쓰기할 레지스터 수
- `data_type` (string, 선택, 기본값 "uint16"): "uint16", "int16", "float32", "uint32", "int32"
- `byte_order` (string, 선택, 기본값 "big_endian"): "big_endian", "little_endian"
- `device_id` (number, 선택, 기본값 1): MODBUS Client Agent 전용, 대상 디바이스 ID

**R-MBRW-003** (Unwanted):
시스템은 `operation`이 "read" 또는 "write" 이외의 값일 때 설정 에러를 반환**해야 하며**, 유효하지 않은 값을 **허용하지 않아야 한다**.

**R-MBRW-004** (State-Driven):
**IF** `register_area`가 "discrete_inputs" 또는 "input_registers"이고 `operation`이 "write"일 **THEN** 시스템은 설정 에러를 반환해야 한다 (읽기 전용 영역에 쓰기 불가).

**R-MBRW-005** (State-Driven):
**IF** `register_area`가 "coils" 또는 "discrete_inputs"일 **THEN** `data_type` 설정은 무시되어야 한다 (boolean 영역은 데이터 타입 변환 불필요).

**R-MBRW-006** (Unwanted):
시스템은 지원하지 않는 `register_area` 값을 **허용하지 않아야 한다**. 알 수 없는 영역이 제공되면 명확한 에러 메시지를 반환해야 한다.

### 3.2 Module 2: Agent 해석 및 통신 (Agent Resolution & Communication) [R-MBRW-007 ~ R-MBRW-012]

**R-MBRW-007** (Event-Driven):
**WHEN** `modbus_rw` 노드가 Init()에서 초기화될 **THEN** `AgentResolver`를 통해 `agent_ref`에 해당하는 Agent를 resolve하고 `AgentTransport`를 획득해야 한다.

**R-MBRW-008** (Event-Driven):
**WHEN** Agent가 resolve된 후 **THEN** `AgentAccessor` 인터페이스를 통해 원본 `agent.Agent` 객체를 획득하고, 타입 어서션으로 Server Agent(`*modbusserver.MODBUSServerAgent`)인지 Client Agent(`*modbus.MODBUSAgent`)인지 자동 감지해야 한다.

**R-MBRW-009** (State-Driven):
**IF** resolve된 Agent가 MODBUS Server Agent도 Client Agent도 아닐 **THEN** 시스템은 초기화 에러를 반환해야 한다 (비-MODBUS 에이전트 연결 불가).

**R-MBRW-010** (Ubiquitous):
시스템은 **항상** Agent의 `Process(ctx, msg)` 메서드를 통해 레지스터 읽기/쓰기 명령을 전달해야 한다.

**R-MBRW-011** (Event-Driven):
**WHEN** Agent가 연결 해제 또는 비정상 상태일 **THEN** 에러 포트로 에러 메시지를 전달하고, 입력 메시지는 에러 포트로 라우팅해야 한다.

**R-MBRW-012** (Ubiquitous):
시스템은 **항상** Agent Process() 호출에 적절한 context timeout을 설정해야 한다 (기본 5초).

### 3.3 Module 3: 읽기 연산 (Read Operations) [R-MBRW-013 ~ R-MBRW-020]

**R-MBRW-013** (Event-Driven):
**WHEN** `operation`이 "read"이고 Agent가 Server Agent일 **THEN** `register_area`에 따라 다음 Process() 명령을 사용해야 한다:
- coils: `get_coils`
- discrete_inputs: `get_discrete_inputs`
- holding_registers: `get_holding_registers` 또는 `get_register_typed`
- input_registers: `get_input_registers` 또는 `get_register_typed`

**R-MBRW-014** (Event-Driven):
**WHEN** `operation`이 "read"이고 Agent가 Client Agent일 **THEN** `read_registers` 명령을 사용하며, `register_area`에 따라 적절한 function_code를 설정해야 한다:
- coils: FC01
- discrete_inputs: FC02
- holding_registers: FC03
- input_registers: FC04

**R-MBRW-015** (Event-Driven):
**WHEN** 읽기 연산이 성공적으로 완료되면 **THEN** 결과 값을 출력 메시지의 payload에 다음 구조로 저장해야 한다:
- `result`: 읽기 결과 값 (단일 값 또는 배열)
- `register_area`: 읽은 레지스터 영역
- `address`: 시작 주소
- `count`: 읽은 레지스터 수
- `data_type`: 적용된 데이터 타입
- `agent_type`: "server" 또는 "client"

**R-MBRW-016** (State-Driven):
**IF** `data_type`이 "uint16"이 아니고 `register_area`가 "holding_registers" 또는 "input_registers"일 **THEN** Server Agent는 `get_register_typed` 명령을 사용하여 타입 변환된 값을 반환해야 한다.

**R-MBRW-017** (Ubiquitous):
시스템은 **항상** 읽기 결과 메시지에 원본 입력 메시지의 기존 payload 데이터를 보존해야 한다 (병합, 덮어쓰기 아님).

**R-MBRW-018** (Event-Driven):
**WHEN** Client Agent에서 읽기 시 mode 파라미터가 지정되지 않았을 **THEN** 기본값 "force"를 사용하여 캐시가 아닌 실제 디바이스에서 읽어야 한다.

**R-MBRW-019** (Event-Driven):
**WHEN** 입력 메시지의 payload에 `read_mode` 키가 존재하면 **THEN** 해당 값을 Client Agent의 mode 파라미터로 사용해야 한다 ("cached", "force", "direct").

**R-MBRW-020** (Event-Driven):
**WHEN** 입력 메시지의 payload에 `device_id` 키가 존재하면 **THEN** 해당 값이 노드 설정의 `device_id`를 오버라이드해야 한다.

### 3.4 Module 4: 쓰기 연산 (Write Operations) [R-MBRW-021 ~ R-MBRW-028]

**R-MBRW-021** (Event-Driven):
**WHEN** `operation`이 "write"이고 Agent가 Server Agent일 **THEN** `register_area`에 따라 다음 Process() 명령을 사용해야 한다:
- coils (count=1): `set_coil`
- coils (count>1): `set_coils`
- holding_registers (count=1): `set_register`
- holding_registers (count>1): `set_registers`

**R-MBRW-022** (Event-Driven):
**WHEN** `operation`이 "write"이고 Agent가 Client Agent일 **THEN** `register_area`에 따라 다음 Process() 명령을 사용해야 한다:
- coils (count=1): `write_coil`
- coils (count>1): `write_coils`
- holding_registers (count=1): `write_register`
- holding_registers (count>1): `write_registers`

**R-MBRW-023** (Event-Driven):
**WHEN** 쓰기 연산 시 입력 메시지의 payload에 `value` 키가 존재하면 **THEN** 해당 값을 단일 레지스터/코일 쓰기에 사용해야 한다.

**R-MBRW-024** (Event-Driven):
**WHEN** 쓰기 연산 시 입력 메시지의 payload에 `values` 키가 존재하면 **THEN** 해당 배열을 다중 레지스터/코일 쓰기에 사용해야 한다.

**R-MBRW-025** (State-Driven):
**IF** 쓰기 연산 시 입력 메시지의 payload에 `value`도 `values`도 없을 **THEN** 에러 포트로 "쓰기 값이 없음" 에러를 전달해야 한다.

**R-MBRW-026** (Event-Driven):
**WHEN** Server Agent에 `data_type`이 지정된 상태로 쓰기 명령을 보낼 **THEN** Process() 메시지에 `data_type`과 `byte_order` 파라미터를 포함해야 한다.

**R-MBRW-027** (Event-Driven):
**WHEN** 쓰기 연산이 성공적으로 완료되면 **THEN** 출력 메시지의 payload에 쓰기 확인 정보를 포함해야 한다:
- `success`: true
- `register_area`: 쓰기한 레지스터 영역
- `address`: 시작 주소
- `count`: 쓰기한 레지스터 수
- `agent_type`: "server" 또는 "client"

**R-MBRW-028** (Ubiquitous):
시스템은 **항상** 쓰기 연산 시에도 원본 입력 메시지의 기존 payload 데이터를 보존해야 한다.

### 3.5 Module 5: 에러 처리 및 엣지 케이스 (Error Handling & Edge Cases) [R-MBRW-029 ~ R-MBRW-035]

**R-MBRW-029** (Event-Driven):
**WHEN** Agent Process() 호출이 에러를 반환하면 **THEN** 에러 포트로 원본 메시지와 에러 정보를 포함한 에러 메시지를 전달해야 한다.

**R-MBRW-030** (Event-Driven):
**WHEN** Agent Process() 호출이 timeout (context deadline exceeded)에 의해 실패하면 **THEN** 에러 메시지에 timeout 관련 정보를 포함해야 한다.

**R-MBRW-031** (State-Driven):
**IF** Agent가 Paused 또는 Stopped 상태일 **THEN** 에러 포트로 Agent 상태 관련 에러를 전달해야 한다.

**R-MBRW-032** (Unwanted):
시스템은 Agent Process() 호출 중 발생하는 패닉을 전파**하지 않아야 한다**. recover()로 패닉을 캐치하고 에러 포트로 전달해야 한다.

**R-MBRW-033** (State-Driven):
**IF** `count`가 0 이하이거나 `address`가 음수일 **THEN** 설정 검증 시점에 에러를 반환해야 한다.

**R-MBRW-034** (Unwanted):
시스템은 읽기 전용 영역(discrete_inputs, input_registers)에 쓰기를 시도**하지 않아야 한다**. 설정 시점에서 차단해야 한다.

**R-MBRW-035** (Event-Driven):
**WHEN** Client Agent의 `device_id`가 유효하지 않은 디바이스를 참조하면 **THEN** 에러 포트로 "디바이스를 찾을 수 없음" 에러를 전달해야 한다.

### 3.6 Module 6: 프론트엔드 스키마 통합 (Frontend Schema Integration) [R-MBRW-036 ~ R-MBRW-040]

**R-MBRW-036** (Ubiquitous):
시스템은 **항상** `nodeSchemas.ts`에 `modbus_rw` 노드의 `NodeTypeSchema`를 포함해야 한다.

**R-MBRW-037** (Ubiquitous):
시스템은 **항상** `modbus_rw` 스키마에 다음 configSchema 필드를 제공해야 한다:
- `agent_ref` (agent_select): 대상 MODBUS 에이전트 선택
- `operation` (select): "read" / "write"
- `register_area` (select): "coils" / "discrete_inputs" / "holding_registers" / "input_registers"
- `address` (number): 시작 레지스터 주소
- `count` (number): 레지스터 수 (기본값 1)
- `data_type` (select): "uint16" / "int16" / "float32" / "uint32" / "int32" (기본값 "uint16")
- `byte_order` (select): "big_endian" / "little_endian" (기본값 "big_endian")
- `device_id` (number): Client Agent 디바이스 ID (기본값 1)

**R-MBRW-038** (Ubiquitous):
시스템은 **항상** `modbus_rw`의 기본 포트를 input(입력), output(출력), error(에러) 3개로 정의해야 한다.

**R-MBRW-039** (Ubiquitous):
시스템은 **항상** `nodeTypeMeta.ts`에 `modbus_rw` 노드의 `NodeTypeDetailMeta`를 포함해야 한다.

**R-MBRW-040** (Event-Driven):
**WHEN** `operation`이 "write"로 선택되고 `register_area`가 "discrete_inputs" 또는 "input_registers"로 선택된 상태일 **THEN** 프론트엔드에서 유효성 경고를 표시할 수 있어야 한다.

---

## 4. Specifications (명세)

### 4.1 데이터 구조

#### 4.1.1 ModbusRWConfig (노드 설정 구조체)

```go
type ModbusRWConfig struct {
    AgentRef     string `json:"agent_ref"`      // 대상 Agent 이름/ID
    Operation    string `json:"operation"`       // "read" | "write"
    RegisterArea string `json:"register_area"`   // "coils" | "discrete_inputs" | "holding_registers" | "input_registers"
    Address      uint16 `json:"address"`         // 시작 레지스터 주소
    Count        uint16 `json:"count"`           // 레지스터 수 (기본값 1)
    DataType     string `json:"data_type"`       // "uint16" | "int16" | "float32" | "uint32" | "int32"
    ByteOrder    string `json:"byte_order"`      // "big_endian" | "little_endian"
    DeviceID     uint8  `json:"device_id"`       // Client Agent 전용 (기본값 1)
    Timeout      string `json:"timeout"`         // Process 호출 타임아웃 (기본값 "5s")
}
```

#### 4.1.2 ModbusRWNode (노드 구조체)

```go
type ModbusRWNode struct {
    *BaseNode
    config      ModbusRWConfig
    resolver    AgentResolver
    transport   AgentTransport
    agent       agent.Agent        // 원본 Agent 객체
    agentType   string             // "server" | "client"
    timeout     time.Duration
}
```

#### 4.1.3 Agent 타입 감지 흐름

```
AgentResolver.ResolveAgent(ref)
    -> AgentTransport
    -> AgentAccessor.UnderlyingAgent()
    -> agent.Agent
    -> 타입 어서션:
        *modbusserver.MODBUSServerAgent -> agentType = "server"
        *modbus.MODBUSAgent            -> agentType = "client"
        기타                           -> 에러 반환
```

### 4.2 포트 정의

| 포트 | 방향 | 설명 |
|------|------|------|
| input | input | 읽기/쓰기 트리거 메시지 입력 |
| output | output | 읽기 결과 또는 쓰기 확인 메시지 출력 |
| error | error | 처리 중 에러 발생 시 에러 메시지 출력 |

### 4.3 Process() 메시지 구성 (Agent에 전달)

#### 4.3.1 Server Agent - 읽기

```json
{
  "payload": {
    "command": "get_holding_registers",
    "address": 100,
    "count": 10
  }
}
```

타입 변환 읽기 (holding_registers/input_registers, data_type != "uint16"):
```json
{
  "payload": {
    "command": "get_register_typed",
    "address": 100,
    "data_type": "float32",
    "byte_order": "big_endian"
  }
}
```

#### 4.3.2 Client Agent - 읽기

```json
{
  "payload": {
    "command": "read_registers",
    "device_id": 1,
    "function_code": 3,
    "address": 100,
    "count": 10,
    "mode": "force"
  }
}
```

#### 4.3.3 Server Agent - 쓰기 (단일)

```json
{
  "payload": {
    "command": "set_register",
    "address": 100,
    "value": 42,
    "data_type": "float32",
    "byte_order": "big_endian"
  }
}
```

#### 4.3.4 Client Agent - 쓰기 (다중)

```json
{
  "payload": {
    "command": "write_registers",
    "device_id": 1,
    "address": 100,
    "values": [100, 200, 300]
  }
}
```

### 4.4 출력 메시지 구조

#### 4.4.1 읽기 결과 메시지

```json
{
  "payload": {
    "...원본 payload 데이터 보존...",
    "result": [42.5, 43.0],
    "register_area": "holding_registers",
    "address": 100,
    "count": 2,
    "data_type": "float32",
    "agent_type": "server"
  }
}
```

#### 4.4.2 쓰기 확인 메시지

```json
{
  "payload": {
    "...원본 payload 데이터 보존...",
    "success": true,
    "register_area": "holding_registers",
    "address": 100,
    "count": 1,
    "agent_type": "client"
  }
}
```

### 4.5 설정 스키마 (프론트엔드)

```typescript
const MODBUS_RW_SCHEMA: NodeTypeSchema = {
  configSchema: [
    { name: 'agent_ref', type: 'agent_select', label: 'MODBUS 에이전트', required: true, description: '연결할 MODBUS 에이전트를 선택합니다' },
    { name: 'operation', type: 'select', label: '연산', options: ['read', 'write'], default: 'read', required: true, description: '읽기 또는 쓰기 연산 선택' },
    { name: 'register_area', type: 'select', label: '레지스터 영역', options: ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'], default: 'holding_registers', required: true, description: 'MODBUS 레지스터 영역' },
    { name: 'address', type: 'number', label: '시작 주소', required: true, description: '시작 레지스터 주소 (0-65535)' },
    { name: 'count', type: 'number', label: '레지스터 수', default: '1', description: '읽기/쓰기할 레지스터 수' },
    { name: 'data_type', type: 'select', label: '데이터 타입', options: ['uint16', 'int16', 'float32', 'uint32', 'int32'], default: 'uint16', description: '레지스터 데이터 타입 (Holding/Input Registers 전용)' },
    { name: 'byte_order', type: 'select', label: '바이트 순서', options: ['big_endian', 'little_endian'], default: 'big_endian', description: '다중 레지스터 타입의 바이트 순서' },
    { name: 'device_id', type: 'number', label: '디바이스 ID', default: '1', description: 'MODBUS Client Agent 전용 디바이스 ID' },
  ],
  defaultPorts: [
    { name: 'input', direction: 'input' },
    { name: 'output', direction: 'output' },
    { name: 'error', direction: 'error' },
  ],
};
```

---

## 5. Traceability (추적성)

| TAG | 요구사항 | 모듈 |
|-----|---------|------|
| R-MBRW-001 | 빌트인 노드 등록 | Module 1 |
| R-MBRW-002 | 설정 필드 파싱 | Module 1 |
| R-MBRW-003 | operation 유효성 검증 | Module 1 |
| R-MBRW-004 | 읽기 전용 영역 쓰기 차단 | Module 1 |
| R-MBRW-005 | boolean 영역 data_type 무시 | Module 1 |
| R-MBRW-006 | register_area 유효성 검증 | Module 1 |
| R-MBRW-007 | Agent resolve | Module 2 |
| R-MBRW-008 | Agent 타입 자동 감지 | Module 2 |
| R-MBRW-009 | 비-MODBUS Agent 차단 | Module 2 |
| R-MBRW-010 | Process() 메서드 호출 | Module 2 |
| R-MBRW-011 | Agent 비정상 상태 처리 | Module 2 |
| R-MBRW-012 | context timeout 설정 | Module 2 |
| R-MBRW-013 | Server Agent 읽기 명령 매핑 | Module 3 |
| R-MBRW-014 | Client Agent 읽기 명령 매핑 | Module 3 |
| R-MBRW-015 | 읽기 결과 payload 구조 | Module 3 |
| R-MBRW-016 | Server Agent 타입 변환 읽기 | Module 3 |
| R-MBRW-017 | 원본 payload 보존 (읽기) | Module 3 |
| R-MBRW-018 | Client Agent 기본 read mode | Module 3 |
| R-MBRW-019 | 메시지 기반 read_mode 오버라이드 | Module 3 |
| R-MBRW-020 | 메시지 기반 device_id 오버라이드 | Module 3 |
| R-MBRW-021 | Server Agent 쓰기 명령 매핑 | Module 4 |
| R-MBRW-022 | Client Agent 쓰기 명령 매핑 | Module 4 |
| R-MBRW-023 | 단일 값 쓰기 (value) | Module 4 |
| R-MBRW-024 | 다중 값 쓰기 (values) | Module 4 |
| R-MBRW-025 | 쓰기 값 누락 에러 | Module 4 |
| R-MBRW-026 | Server Agent data_type 전달 | Module 4 |
| R-MBRW-027 | 쓰기 확인 payload 구조 | Module 4 |
| R-MBRW-028 | 원본 payload 보존 (쓰기) | Module 4 |
| R-MBRW-029 | Process 에러 처리 | Module 5 |
| R-MBRW-030 | timeout 에러 처리 | Module 5 |
| R-MBRW-031 | Agent 상태 검증 | Module 5 |
| R-MBRW-032 | 패닉 방지 | Module 5 |
| R-MBRW-033 | count/address 유효성 | Module 5 |
| R-MBRW-034 | 읽기 전용 영역 쓰기 차단 (실행 시) | Module 5 |
| R-MBRW-035 | Client device_id 유효성 | Module 5 |
| R-MBRW-036 | nodeSchemas.ts 등록 | Module 6 |
| R-MBRW-037 | configSchema 필드 정의 | Module 6 |
| R-MBRW-038 | 기본 포트 정의 | Module 6 |
| R-MBRW-039 | nodeTypeMeta.ts 등록 | Module 6 |
| R-MBRW-040 | 읽기 전용 영역 경고 | Module 6 |

---

## 6. 관련 SPEC 참조

| SPEC | 관계 | 설명 |
|------|------|------|
| SPEC-MODBUS-001 | 의존 | MODBUS/TCP Client Agent - `read_registers`, `write_*` 명령 사용 |
| SPEC-MODBUS-002 | 의존 | MODBUS/TCP Server Agent - `get_*`, `set_*` 명령 사용 |
| SPEC-MODBUS-003 | 의존 | Multi-Data-Type 지원 - `data_type`, `byte_order`, `get_register_typed` 활용 |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-11*
*작성: xtra*
