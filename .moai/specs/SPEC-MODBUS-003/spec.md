# SPEC-MODBUS-003: MODBUS/TCP 다중 데이터 타입 지원

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-003 |
| 제목 | MODBUS/TCP Multi-Data-Type Support (Server + Client) |
| 상태 | Completed |
| 우선순위 | High |
| 관련 SPEC | SPEC-MODBUS-001 (Client), SPEC-MODBUS-002 (Server) |
| 패키지 | `internal/modbus/`, `internal/agent/modbusserver/`, `internal/agent/modbus/` |
| 생성일 | 2026-02-27 |

---

## 1. Environment (환경)

### 1.1 현재 시스템 상태

#### 1.1.1 Server Agent (`internal/agent/modbusserver/`)

MODBUS/TCP Server Agent (SPEC-MODBUS-002)는 현재 운영 중이며, 다음 구성 요소로 이루어져 있다:

- **소스 파일 (8개)**: `errors.go`, `config.go`, `register_map.go`, `request.go`, `handler.go`, `listener.go`, `agent.go`, `register.go`
- **테스트 파일 (6개)**: `config_test.go`, `register_map_test.go`, `request_test.go`, `handler_test.go`, `listener_test.go`, `agent_test.go`

#### 1.1.2 Client Agent (`internal/agent/modbus/`)

MODBUS/TCP Client Agent (SPEC-MODBUS-001)는 현재 운영 중이며, 다음 구성 요소로 이루어져 있다:

- **소스 파일 (8개)**: `agent.go`, `cache.go`, `config.go`, `device.go`, `errors.go`, `protocol.go`, `register.go`, `transport.go`, `write.go`
- **테스트 파일 (5개)**: `agent_test.go`, `cache_test.go`, `config_test.go`, `protocol_test.go`, `write_test.go`

### 1.2 현재 제약사항

#### 1.2.1 Server 제약사항

| 구성 요소 | 현재 상태 | 제약 |
|-----------|-----------|------|
| `RegisterMap` | `map[uint16]uint16` | uint16만 저장 가능 |
| `Process()` 명령 | `getParamInt() -> uint16()` | 정밀도 손실 (float, int32 등) |
| Wire Protocol | `encodeRegisters([]uint16)` / `decodeRegisterBytes() -> []uint16` | uint16 하드코딩 |
| Config | `anyToUint16()` | float64 -> uint16 변환 시 정밀도 손실 |
| ChangeSet | `OldValues/NewValues any` | 이미 다중 타입 호환 |

#### 1.2.2 Client 제약사항

| 구성 요소 | 현재 상태 | 제약 |
|-----------|-----------|------|
| `RegisterCache` | `HoldingRegisters map[uint16]uint16`, `InputRegisters map[uint16]uint16` | uint16만 저장 가능 |
| `RegisterGroupConfig` | `Name`, `FunctionCode`, `StartAddress`, `Quantity` 필드만 존재 | 데이터 타입 정보 없음 |
| `paramUint16()` / `paramUint16Slice()` | uint16 하드코딩 | 다중 타입 값 추출 불가 |
| `processWriteRegister()` | FC06 (Write Single Register) 고정 | 2-레지스터 타입 불가 |
| `processWriteRegisters()` | `paramUint16Slice()` 사용 | uint16 배열만 지원 |
| `decodeRegisters()` | `-> []uint16` | 타입 해석 없음 |
| `CompareAndUpdate()` | `compareRegisterValues()` → uint16 비교 | 타입 변환된 변경 감지 불가 |
| `GetSnapshot()` | raw uint16 값만 반환 | 타입 변환 정보 없음 |

### 1.3 MODBUS 프로토콜 특성

MODBUS 와이어 프로토콜은 16비트 레지스터 단위로 동작한다. 다중 레지스터 타입은 연속 레지스터를 점유한다:

| 데이터 타입 | 레지스터 수 | 바이트 | 비고 |
|------------|-----------|--------|------|
| `uint16` | 1 | 2 | 기본값, 현재 동작 |
| `int16` | 1 | 2 | 동일 비트의 부호 있는 해석 |
| `float32` | 2 | 4 | IEEE 754 Big-Endian |
| `uint32` | 2 | 4 | Big-Endian |
| `int32` | 2 | 4 | 부호 있는 Big-Endian |

---

## 2. Assumptions (가정)

### 2.1 설계 가정 (공통)

- **A1**: 내부 저장소(`map[uint16]uint16`)는 Server와 Client 모두 변경하지 않는다. MODBUS 와이어 프로토콜 호환성을 유지하기 위함이다.
- **A2**: 타입 해석은 오버레이(overlay) 패턴으로 구현한다. Server는 Bridge 레벨, Client는 Cache 레벨에서 적용한다.
- **A3**: 기본 Byte Order는 Big-Endian이다. MODBUS 표준이 Big-Endian을 사용하므로 이를 기본값으로 한다.
- **A4**: `data_type` 파라미터가 없으면 기존 uint16 동작을 유지한다 (하위 호환성 100%).
- **A5**: 코일(`coils`)과 이산 입력(`discrete_inputs`)은 bool 타입이므로 다중 데이터 타입 지원 대상에서 제외한다.

### 2.2 기술 가정 (공통)

- **A6**: Go 표준 라이브러리의 `math` 및 `encoding/binary` 패키지로 모든 타입 변환이 가능하다.
- **A7**: 2-레지스터 타입(float32, uint32, int32)은 연속된 주소에 저장되어야 하며, 주소 겹침은 설정 검증 시점에 차단한다.
- **A8**: 기존 테스트는 모두 통과해야 하며, 새 기능에 대한 테스트가 추가되어야 한다.

### 2.3 설계 가정 (Client 전용)

- **A9**: Client의 `RegisterCache` 내부 저장소(`HoldingRegisters map[uint16]uint16`, `InputRegisters map[uint16]uint16`)는 변경하지 않는다. TypeOverlay는 해석 계층으로만 동작한다.
- **A10**: Client `write_register` 명령에 `data_type`이 2-레지스터 타입(float32, uint32, int32)이면 FC06(Write Single Register) 대신 FC16(Write Multiple Registers)을 자동 사용한다.
- **A11**: `register_data` 및 `register_changed` 이벤트에 TypeOverlay가 설정된 경우 `typed_values` 필드를 추가한다.

### 2.4 공유 패키지 가정

- **A12**: 타입 변환 코드는 `internal/modbus/types.go` 공유 패키지에 위치하여 Server와 Client 모두 동일한 변환 로직을 사용한다.
- **A13**: Server의 기존 `internal/agent/modbusserver/types.go` 파일은 공유 패키지를 import하는 형태로 리팩토링한다.

---

## 3. Requirements (요구사항)

### 3.1 Module 1: 타입 정의 및 설정 - Server (Type Definition & Configuration)

**REQ-M1-01** (Ubiquitous):
시스템은 **항상** `uint16`, `int16`, `float32`, `uint32`, `int32` 다섯 가지 데이터 타입을 지원해야 한다.

**REQ-M1-02** (Event-Driven):
**WHEN** `RegisterAreaConfig`에 `data_type` 필드가 지정되지 않았을 **THEN** 시스템은 기본값으로 `"uint16"`을 사용해야 한다.

**REQ-M1-03** (Event-Driven):
**WHEN** `RegisterAreaConfig`에 `type_map` 필드가 지정되었을 **THEN** 시스템은 개별 주소별 타입 오버라이드를 적용해야 한다.

**REQ-M1-04** (Event-Driven):
**WHEN** `type_map` 엔트리의 `byte_order`가 `"little_endian"`으로 지정되었을 **THEN** 시스템은 해당 주소의 다중 레지스터 값을 Little-Endian 순서로 인코딩/디코딩해야 한다.

**REQ-M1-05** (State-Driven):
**IF** `type_map` 엔트리의 다중 레지스터 타입이 영역(area)의 `count`를 초과하거나 다른 `type_map` 엔트리와 주소가 겹칠 **THEN** 시스템은 설정 파싱 시점에 검증 에러를 반환해야 한다.

**REQ-M1-06** (Unwanted):
시스템은 지원하지 않는 `data_type` 문자열을 **허용하지 않아야 한다**. 알 수 없는 타입이 제공되면 명확한 에러 메시지를 반환해야 한다.

### 3.2 Module 2: 타입 변환 유틸리티 - Server (Type Conversion Utilities)

**REQ-M2-01** (Ubiquitous):
시스템은 **항상** `float32 <-> [2]uint16` 변환을 IEEE 754 Big-Endian 형식으로 수행해야 한다.

**REQ-M2-02** (Ubiquitous):
시스템은 **항상** `int32 <-> [2]uint16`, `uint32 <-> [2]uint16` 변환을 Big-Endian 형식으로 수행해야 한다.

**REQ-M2-03** (Ubiquitous):
시스템은 **항상** `int16 <-> uint16` 변환을 비트 재해석(bit reinterpretation)으로 수행해야 한다.

**REQ-M2-04** (Event-Driven):
**WHEN** `byte_order`가 `"little_endian"`으로 지정되었을 **THEN** 2-레지스터 타입의 상위/하위 워드 순서가 반전되어야 한다.

**REQ-M2-05** (Ubiquitous):
시스템은 **항상** 제네릭 변환 함수 `typedValueToRegisters(value any, dataType string) ([]uint16, error)`와 `registersToTypedValue(regs []uint16, dataType string) (any, error)`를 제공해야 한다.

**REQ-M2-06** (Unwanted):
타입 변환 함수는 잘못된 레지스터 수가 전달되었을 때 패닉하지 **않아야 한다**. 대신 명확한 에러를 반환해야 한다.

### 3.3 Module 3: RegisterMap 타입 오버레이 - Server (Type Overlay)

**REQ-M3-01** (Event-Driven):
**WHEN** `NewRegisterMap(cfg)`가 호출될 **THEN** 설정의 `data_type` 및 `type_map`으로부터 `TypeOverlay`를 구축해야 한다.

**REQ-M3-02** (Event-Driven):
**WHEN** `ReadTyped(area, address, dataType)` 가 호출될 **THEN** 해당 주소에서 적절한 레지스터 수만큼 읽어서 타입 변환된 값을 반환해야 한다.

**REQ-M3-03** (Event-Driven):
**WHEN** `WriteTyped(area, address, value, dataType)`가 호출될 **THEN** 값을 해당 타입의 레지스터 표현으로 변환하여 연속 레지스터에 기록해야 한다.

**REQ-M3-04** (Ubiquitous):
TypeOverlay는 기존 `ReadHoldingRegisters`, `WriteHoldingRegisters` 등의 raw uint16 동작에 영향을 주지 **않아야 한다**.

**REQ-M3-05** (State-Driven):
**IF** TypeOverlay에 해당 주소의 타입 정보가 없을 **THEN** `uint16`을 기본 타입으로 사용해야 한다.

### 3.4 Module 4: Process() 명령 확장 - Server (Command Enhancement)

**REQ-M4-01** (Event-Driven):
**WHEN** `set_register` 명령에 `data_type` 파라미터가 포함되었을 **THEN** 시스템은 해당 타입에 따라 값을 변환하여 레지스터에 기록해야 한다.

**REQ-M4-02** (Event-Driven):
**WHEN** `set_registers` 명령에 `data_type` 파라미터가 포함되었을 **THEN** 시스템은 각 값을 해당 타입의 레지스터 표현으로 변환하여 기록해야 한다.

**REQ-M4-03** (Event-Driven):
**WHEN** `set_register` 또는 `set_registers`에 `data_type`이 생략되었을 **THEN** TypeOverlay의 기본 타입을 사용하거나, TypeOverlay에도 없으면 `uint16`을 사용해야 한다.

**REQ-M4-04** (Event-Driven):
**WHEN** `get_map` 명령이 실행될 **THEN** TypeOverlay가 있는 주소에 대해 타입 변환된 값과 타입 정보를 포함하여 반환해야 한다.

**REQ-M4-05** (Event-Driven):
**WHEN** `get_register_typed` 명령이 실행될 **THEN** 지정된 주소의 타입 변환된 값을 반환해야 한다.

**REQ-M4-06** (Event-Driven):
**WHEN** 레지스터 값이 변경되어 ChangeSet이 생성될 **THEN** change event에 `data_type` 정보를 포함해야 한다.

**REQ-M4-07** (State-Driven):
**IF** `data_type`이 `float32`이고 `value` 파라미터가 정수로 전달되었을 **THEN** 시스템은 자동으로 float64 -> float32 변환을 수행해야 한다.

### 3.5 Module 5: 설정 확장 - Server (Configuration Enhancement)

**REQ-M5-01** (Event-Driven):
**WHEN** `RegisterAreaConfig`가 파싱될 **THEN** 선택적 `data_type` 문자열 필드를 인식해야 한다.

**REQ-M5-02** (Event-Driven):
**WHEN** `RegisterAreaConfig`에 `type_map` 배열이 포함되었을 **THEN** 각 `TypeMapEntry`(`address`, `data_type`, `byte_order`)를 파싱해야 한다.

**REQ-M5-03** (Event-Driven):
**WHEN** `initial_values`가 `data_type`과 함께 제공되었을 **THEN** 초기값을 해당 타입으로 해석하여 레지스터에 기록해야 한다. (예: float64 3.14 -> float32 -> 2개 uint16 레지스터)

**REQ-M5-04** (State-Driven):
**IF** `type_map` 엔트리의 주소 범위가 다른 엔트리와 겹칠 **THEN** 설정 파싱 시 `ErrTypeMapOverlap` 에러를 반환해야 한다.

**REQ-M5-05** (State-Driven):
**IF** `type_map` 엔트리의 다중 레지스터 타입이 영역의 `start_address + count` 범위를 초과할 **THEN** 설정 파싱 시 `ErrTypeMapOutOfRange` 에러를 반환해야 한다.

### 3.6 Module 6: 공유 타입 변환 패키지 (Shared Type Conversion Package)

**REQ-M6-01** (Ubiquitous):
`internal/modbus/types.go` 공유 패키지는 **항상** 데이터 타입 상수(`DataTypeUint16`, `DataTypeInt16`, `DataTypeFloat32`, `DataTypeUint32`, `DataTypeInt32`)를 제공해야 한다.

**REQ-M6-02** (Ubiquitous):
공유 패키지는 **항상** Byte Order 상수(`ByteOrderBigEndian`, `ByteOrderLittleEndian`)를 제공해야 한다.

**REQ-M6-03** (Ubiquitous):
공유 패키지는 **항상** `TypedValueToRegisters(value any, dataType string, byteOrder string) ([]uint16, error)`와 `RegistersToTypedValue(regs []uint16, dataType string, byteOrder string) (any, error)` 공개 함수를 제공해야 한다.

**REQ-M6-04** (Ubiquitous):
공유 패키지는 **항상** `RegisterCountForType(dataType string) (uint16, error)` 및 `IsValidDataType(dataType string) bool` 헬퍼 함수를 제공해야 한다.

**REQ-M6-05** (Ubiquitous):
공유 패키지는 **항상** `TypeOverlayEntry` 구조체와 `TypeOverlay` 관련 타입을 제공하여 Server와 Client 모두 동일한 타입 정의를 사용해야 한다.

**REQ-M6-06** (Event-Driven):
**WHEN** Server(`internal/agent/modbusserver/`)가 타입 변환을 수행할 **THEN** 공유 패키지의 함수를 import하여 사용해야 한다.

**REQ-M6-07** (Event-Driven):
**WHEN** Client(`internal/agent/modbus/`)가 타입 변환을 수행할 **THEN** 공유 패키지의 함수를 import하여 사용해야 한다.

### 3.7 Module 7: Client 설정 확장 (Client Configuration Enhancement)

**REQ-M7-01** (Event-Driven):
**WHEN** `RegisterGroupConfig`가 파싱될 **THEN** 선택적 `DataType` 문자열 필드를 인식해야 한다.

**REQ-M7-02** (Event-Driven):
**WHEN** `RegisterGroupConfig`에 `TypeMap` 배열이 포함되었을 **THEN** 각 `TypeMapEntry`(`address`, `data_type`, `byte_order`)를 파싱해야 한다.

**REQ-M7-03** (Event-Driven):
**WHEN** `RegisterGroupConfig`에 `data_type` 필드가 지정되지 않았을 **THEN** 시스템은 기본값으로 `"uint16"`을 사용해야 한다 (하위 호환성).

**REQ-M7-04** (State-Driven):
**IF** `type_map` 엔트리의 다중 레지스터 타입이 그룹의 `StartAddress + Quantity` 범위를 초과하거나 다른 `type_map` 엔트리와 주소가 겹칠 **THEN** 시스템은 설정 파싱 시점에 검증 에러를 반환해야 한다.

**REQ-M7-05** (Unwanted):
시스템은 Client `RegisterGroupConfig`에서 지원하지 않는 `data_type` 문자열을 **허용하지 않아야 한다**.

### 3.8 Module 8: Client Cache 타입 오버레이 (Client Cache TypeOverlay)

**REQ-M8-01** (Event-Driven):
**WHEN** `RegisterGroupConfig`에 `DataType` 또는 `TypeMap`이 설정된 경우 **THEN** 해당 디바이스의 `RegisterCache`에 TypeOverlay를 구축해야 한다.

**REQ-M8-02** (Event-Driven):
**WHEN** `ReadTyped(fc byte, address uint16, dataType string, byteOrder string)` 가 호출될 **THEN** 캐시에서 적절한 레지스터 수만큼 읽어서 타입 변환된 값을 반환해야 한다.

**REQ-M8-03** (Ubiquitous):
Client TypeOverlay는 기존 `UpdateHoldingRegisters`, `UpdateInputRegisters`, `GetSnapshot` 등의 raw uint16 동작에 영향을 주지 **않아야 한다**.

**REQ-M8-04** (Event-Driven):
**WHEN** `CompareAndUpdate()`가 호출되고 TypeOverlay가 설정되어 있을 **THEN** `changedData`에 `typed_changed_values` 필드를 추가하여 타입 변환된 변경 값을 포함해야 한다.

**REQ-M8-05** (Event-Driven):
**WHEN** `GetSnapshot()`이 호출되고 TypeOverlay가 설정되어 있을 **THEN** 반환 맵에 `typed_holding_registers`와 `typed_input_registers` 필드를 추가해야 한다.

**REQ-M8-06** (State-Driven):
**IF** TypeOverlay에 해당 주소의 타입 정보가 없을 **THEN** `uint16`을 기본 타입으로 사용해야 한다.

### 3.9 Module 9: Client Process() 명령 확장 (Client Command Enhancement)

**REQ-M9-01** (Event-Driven):
**WHEN** `write_register` 명령에 `data_type` 파라미터가 포함되고 해당 타입이 2-레지스터 타입(float32, uint32, int32)일 **THEN** 시스템은 FC06 대신 FC16(Write Multiple Registers)을 사용하여 값을 기록해야 한다.

**REQ-M9-02** (Event-Driven):
**WHEN** `write_register` 명령에 `data_type` 파라미터가 포함되고 해당 타입이 1-레지스터 타입(uint16, int16)일 **THEN** 시스템은 기존 FC06(Write Single Register)을 사용해야 한다.

**REQ-M9-03** (Event-Driven):
**WHEN** `write_registers` 명령에 `data_type` 파라미터가 포함되었을 **THEN** 시스템은 각 값을 해당 타입의 레지스터 표현으로 변환하여 FC16으로 기록해야 한다.

**REQ-M9-04** (Event-Driven):
**WHEN** `write_register` 또는 `write_registers`에 `data_type`이 생략되었을 **THEN** 기존 uint16 동작을 유지해야 한다 (하위 호환성).

**REQ-M9-05** (Event-Driven):
**WHEN** `read_registers` 명령이 실행되고 해당 주소에 TypeOverlay가 설정되어 있을 **THEN** 응답에 `typed_values` 필드를 추가하여 타입 변환된 값을 포함해야 한다.

**REQ-M9-06** (Event-Driven):
**WHEN** `get_cache` 명령이 실행되고 TypeOverlay가 설정되어 있을 **THEN** 캐시 스냅샷에 `typed_holding_registers`와 `typed_input_registers` 필드를 포함해야 한다.

**REQ-M9-07** (Event-Driven):
**WHEN** `register_data` 이벤트가 발생하고 TypeOverlay가 설정되어 있을 **THEN** 이벤트 데이터에 `typed_values` 필드를 포함해야 한다.

**REQ-M9-08** (Event-Driven):
**WHEN** `register_changed` 이벤트가 발생하고 TypeOverlay가 설정되어 있을 **THEN** 이벤트 데이터에 `typed_changed_values` 필드를 포함해야 한다.

**REQ-M9-09** (State-Driven):
**IF** `data_type`이 `float32`이고 `value` 파라미터가 정수로 전달되었을 **THEN** 시스템은 자동으로 float64 -> float32 변환을 수행해야 한다.

**REQ-M9-10** (Unwanted):
시스템은 Client `write_register`/`write_registers` 명령에서 지원하지 않는 `data_type`을 **허용하지 않아야 한다**.

---

## 4. Specifications (명세)

### 4.1 새로운 파일

| 파일 | 목적 |
|------|------|
| `internal/modbus/types.go` | 공유 데이터 타입 상수 정의, 타입 변환 함수, TypeOverlay 관련 타입 |
| `internal/modbus/types_test.go` | 공유 타입 변환 단위 테스트 |

### 4.2 수정 파일 - Server (`internal/agent/modbusserver/`)

| 파일 | 변경 내용 |
|------|----------|
| `config.go` | `RegisterAreaConfig`에 `DataType`, `TypeMap` 필드 추가; `TypeMapEntry` 구조체 정의; 파싱 및 검증 로직 추가 |
| `config_test.go` | 새 설정 필드 파싱 및 검증 테스트 추가 |
| `register_map.go` | `TypeOverlay` 필드 추가; `ReadTyped`, `WriteTyped` 메서드 추가; `NewRegisterMap`에서 TypeOverlay 초기화; 초기값 타입 적용 |
| `register_map_test.go` | ReadTyped, WriteTyped 테스트 추가; TypeOverlay 초기화 테스트 |
| `agent.go` | `processSetRegister`, `processSetRegisters`에 `data_type` 파라미터 처리; `processGetRegisterTyped` 신규 명령; `processGetMap` 타입 정보 포함; `sendChangeEvent`에 `data_type` 정보 추가 |
| `agent_test.go` | 새 명령 및 타입 파라미터 테스트 추가 |
| `errors.go` | `ErrTypeMapOverlap`, `ErrTypeMapOutOfRange`, `ErrUnsupportedDataType` 추가 |
| `types.go` | 기존 로컬 타입 정의를 공유 패키지(`internal/modbus`) import로 대체 |

### 4.3 수정 파일 - Client (`internal/agent/modbus/`)

| 파일 | 변경 내용 |
|------|----------|
| `config.go` | `RegisterGroupConfig`에 `DataType`, `TypeMap` 필드 추가; `parseRegisterGroupConfig()`에 파싱 및 검증 로직 추가 |
| `config_test.go` | 새 설정 필드 파싱 및 검증 테스트 추가 |
| `cache.go` | `TypeOverlay` 필드 추가; `ReadTyped` 메서드 추가; `CompareAndUpdate`에 typed 변경 감지; `GetSnapshot`에 typed 값 포함 |
| `cache_test.go` | ReadTyped, TypeOverlay 초기화, typed 스냅샷 테스트 추가 |
| `write.go` | `processWriteRegister`에 `data_type` 파라미터 처리 (FC06->FC16 자동 전환); `processWriteRegisters`에 `data_type` 파라미터 처리; `paramFloat64`, `paramString` 헬퍼 추가 |
| `write_test.go` | 타입 지정 쓰기 테스트 추가 |
| `agent.go` | `read_registers` 응답에 `typed_values` 추가; `get_cache`에 typed 값 추가; `register_data`/`register_changed` 이벤트에 typed 값 추가; TypeOverlay 초기화 |
| `agent_test.go` | 타입 지정 명령 및 이벤트 테스트 추가 |
| `errors.go` | `ErrUnsupportedDataType` 추가 |

### 4.4 타입 정의 (공유 패키지: `internal/modbus/types.go`)

```go
package modbus

// DataType 은 MODBUS 레지스터의 데이터 타입을 나타내는 문자열 상수이다.
const (
    DataTypeUint16  = "uint16"   // 1 레지스터, 기본값
    DataTypeInt16   = "int16"    // 1 레지스터
    DataTypeFloat32 = "float32"  // 2 레지스터, IEEE 754
    DataTypeUint32  = "uint32"   // 2 레지스터
    DataTypeInt32   = "int32"    // 2 레지스터
)

// ByteOrder 는 다중 레지스터 타입의 바이트 순서를 나타낸다.
const (
    ByteOrderBigEndian    = "big_endian"    // 기본값
    ByteOrderLittleEndian = "little_endian"
)

// TypeMapEntry 는 개별 주소의 타입 오버라이드를 나타낸다.
type TypeMapEntry struct {
    Address   uint16 // 시작 주소
    DataType  string // 데이터 타입 (DataType 상수)
    ByteOrder string // 바이트 순서 (ByteOrder 상수, 기본: big_endian)
}

// TypeOverlayEntry 는 TypeOverlay 내부의 주소별 타입 정보이다.
type TypeOverlayEntry struct {
    DataType      string // 데이터 타입
    RegisterCount uint16 // 점유 레지스터 수 (1 또는 2)
    ByteOrder     string // 바이트 순서
}
```

### 4.5 설정 YAML 예시

#### 4.5.1 Server 설정

```yaml
register_map:
  holding_registers:
    start_address: 0
    count: 20
    data_type: "uint16"          # 영역 기본 타입
    type_map:                     # 주소별 타입 오버라이드
      - address: 0
        data_type: "float32"     # 주소 0-1에 float32 (2 레지스터)
      - address: 2
        data_type: "int32"       # 주소 2-3에 int32 (2 레지스터)
      - address: 4
        data_type: "uint32"      # 주소 4-5에 uint32 (2 레지스터)
      - address: 10
        data_type: "int16"       # 주소 10에 int16 (1 레지스터)
      - address: 15
        data_type: "float32"
        byte_order: "little_endian"  # Little-Endian float32
    initial_values: [3.14, 0, -1000, 0, 100000, 0]
  input_registers:
    start_address: 100
    count: 10
    data_type: "float32"         # 전체 영역이 float32
```

#### 4.5.2 Client 설정

```yaml
devices:
  - id: "sensor-01"
    host: "192.168.1.100"
    port: 502
    unit_id: 1
    register_groups:
      - name: "temperature"
        function_code: 3          # FC03 Holding Registers
        start_address: 0
        quantity: 4
        data_type: "float32"     # 그룹 기본 타입: float32 (2 레지스터씩)
      - name: "status"
        function_code: 3
        start_address: 100
        quantity: 10
        data_type: "uint16"      # 그룹 기본 타입: uint16
        type_map:                 # 주소별 타입 오버라이드
          - address: 100
            data_type: "int32"   # 주소 100-101에 int32
          - address: 105
            data_type: "float32"
            byte_order: "little_endian"
```

### 4.6 Process() 명령 확장 예시

#### 4.6.1 Server 명령 예시

```json
// set_register with data_type
{
  "command": "set_register",
  "params": {
    "address": 0,
    "value": 3.14,
    "data_type": "float32"
  }
}

// get_register_typed (신규 명령)
{
  "command": "get_register_typed",
  "params": {
    "address": 0,
    "area": "holding_registers",
    "data_type": "float32"
  }
}
// 응답: {"ok": true, "address": 0, "value": 3.14, "data_type": "float32"}
```

#### 4.6.2 Client 명령 예시

```json
// write_register with data_type (자동 FC16 전환)
{
  "command": "write_register",
  "params": {
    "device_id": "sensor-01",
    "address": 0,
    "value": 3.14,
    "data_type": "float32"
  }
}
// float32는 2 레지스터를 점유하므로 FC06 대신 FC16 사용

// write_registers with data_type
{
  "command": "write_registers",
  "params": {
    "device_id": "sensor-01",
    "address": 0,
    "values": [25.5, -10.3],
    "data_type": "float32"
  }
}
// 2개 float32 = 4 레지스터로 FC16 전송

// read_registers 응답 (TypeOverlay 설정 시)
// 기존 응답에 typed_values 추가:
{
  "status": "ok",
  "device_id": "sensor-01",
  "values": [16456, 0, 16601, 13107],
  "typed_values": {
    "0": {"value": 25.5, "data_type": "float32"},
    "2": {"value": -10.3, "data_type": "float32"}
  }
}

// get_cache 응답 (TypeOverlay 설정 시)
{
  "holding_registers": {"0": 16456, "1": 0, "2": 16601, "3": 13107},
  "typed_holding_registers": {
    "0": {"value": 25.5, "data_type": "float32"},
    "2": {"value": -10.3, "data_type": "float32"}
  }
}
```

### 4.7 하위 호환성 보장

- 기존 Server `set_register`, `set_registers` 명령은 `data_type` 파라미터 없이 기존과 동일하게 동작
- 기존 Client `write_register`, `write_registers` 명령은 `data_type` 파라미터 없이 기존 uint16으로 동작
- 기존 YAML 설정에 `data_type`, `type_map` 필드가 없으면 모든 레지스터가 `uint16`으로 동작
- 외부 MODBUS 클라이언트의 raw uint16 읽기/쓰기는 영향 없음
- 기존 Server ChangeSet의 `OldValues`/`NewValues` 필드는 이미 `any` 타입이므로 호환
- 기존 Client `register_data`/`register_changed` 이벤트는 `typed_values` 필드가 없으면 기존 동작 유지
- 기존 Client `get_cache` 응답은 `typed_*` 필드 추가로 기존 필드에 영향 없음

### 4.8 Traceability (추적성)

| 요구사항 | 구현 모듈 | 테스트 |
|----------|----------|--------|
| REQ-M1-01 ~ M1-06 | `modbusserver/config.go`, `modbus/types.go` | `modbusserver/config_test.go`, `modbus/types_test.go` |
| REQ-M2-01 ~ M2-06 | `modbus/types.go` | `modbus/types_test.go` |
| REQ-M3-01 ~ M3-05 | `modbusserver/register_map.go`, `modbus/types.go` | `modbusserver/register_map_test.go` |
| REQ-M4-01 ~ M4-07 | `modbusserver/agent.go` | `modbusserver/agent_test.go` |
| REQ-M5-01 ~ M5-05 | `modbusserver/config.go` | `modbusserver/config_test.go` |
| REQ-M6-01 ~ M6-07 | `modbus/types.go` | `modbus/types_test.go` |
| REQ-M7-01 ~ M7-05 | `modbus/config.go` | `modbus/config_test.go` |
| REQ-M8-01 ~ M8-06 | `modbus/cache.go` | `modbus/cache_test.go` |
| REQ-M9-01 ~ M9-10 | `modbus/agent.go`, `modbus/write.go` | `modbus/agent_test.go`, `modbus/write_test.go` |
