# SPEC-MODBUS-003: 구현 계획

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-003 |
| 제목 | MODBUS/TCP Multi-Data-Type Support (Server + Client) |
| 개발 방법론 | Hybrid (TDD for new + DDD for legacy) |
| 영향 범위 | 공유 패키지 1개 신규, Server 소스 2개 신규 + 7개 수정 / Client 소스 5개 수정 / 테스트 다수 추가 |

---

## 1. 기술 접근 방식

### 1.1 핵심 설계 원칙: Bridge-Level Type Overlay + Cache-Level Type Overlay

MODBUS 와이어 프로토콜은 16비트 레지스터 단위로 동작한다. 내부 저장소(`map[uint16]uint16`)를 변경하면 와이어 프로토콜 호환성이 깨지므로, **타입 오버레이(Type Overlay)** 패턴을 Server와 Client 모두에 적용한다.

#### Server: Bridge-Level Type Overlay

```
외부 MODBUS Client       RegisterMap (내부)       Bridge Process()
   |                        |                        |
   | FC03 Read Reg 0-1      |                        |
   |----------------------->| raw uint16 읽기         |
   |<-----------------------| [0x4048, 0xF5C3]       |
   |                        |                        |
   | (uint16 그대로)         |    TypeOverlay          |
   |                        |    addr 0: float32      |
   |                        |                        |
   |                        |  set_register           |
   |                        |  addr=0, value=3.14     |
   |                        |  data_type=float32      |
   |                        |<-----------------------|
   |                        | float32 -> [2]uint16    |
   |                        | 저장: reg[0]=0x4048     |
   |                        |       reg[1]=0xF5C3     |
```

#### Client: Cache-Level Type Overlay

```
MODBUS Server           RegisterCache (내부)      Client Process()
   |                        |                        |
   | FC03 응답: raw uint16   |                        |
   |----------------------->| raw uint16 캐시 저장     |
   |                        | [0x4048, 0xF5C3]       |
   |                        |                        |
   |                        |    TypeOverlay          |
   |                        |    addr 0: float32      |
   |                        |                        |
   |                        |  get_cache / read_regs  |
   |                        |  typed_values 포함       |
   |                        |----------------------->|
   |                        | float32(3.14) 반환       |
   |                        |                        |
   |  write_register         |                        |
   |  addr=0, value=3.14    |                        |
   |  data_type=float32     |                        |
   |<-----------------------| float32 -> [2]uint16    |
   |  FC16 전송 (2 regs)     | 자동 FC06->FC16 전환     |
```

**공통 장점**:
- 기존 MODBUS 와이어 프로토콜 100% 호환
- 내부 저장소 변경 없음 (Server: RegisterMap, Client: RegisterCache)
- 타입 해석은 오버레이 레벨에서만 적용
- 공유 패키지로 타입 변환 로직 중복 제거

### 1.2 파일 변경 전략

#### 공유 패키지 (`internal/modbus/`)

| 파일 | 전략 | 이유 |
|------|------|------|
| `types.go` (신규) | TDD | 신규 파일, 순수 함수로 구성 |
| `types_test.go` (신규) | TDD | 타입 변환 테이블 기반 테스트 |

#### Server (`internal/agent/modbusserver/`)

| 파일 | 전략 | 이유 |
|------|------|------|
| `types.go` | DDD | 기존 파일을 공유 패키지 import로 리팩토링 |
| `errors.go` | DDD | 기존 파일에 상수 추가 |
| `config.go` | DDD | 기존 파싱 로직 확장 |
| `register_map.go` | DDD | 기존 구조체에 필드/메서드 추가 |
| `agent.go` | DDD | 기존 Process() 로직 확장 |

#### Client (`internal/agent/modbus/`)

| 파일 | 전략 | 이유 |
|------|------|------|
| `config.go` | DDD | 기존 파싱 로직 확장 |
| `cache.go` | DDD | 기존 구조체에 필드/메서드 추가 |
| `write.go` | DDD | 기존 쓰기 핸들러 확장 |
| `agent.go` | DDD | 기존 Process() 로직 확장 |
| `errors.go` | DDD | 기존 파일에 에러 상수 추가 |

---

## 2. 마일스톤

### Milestone 1: 공유 타입 변환 패키지 (Primary Goal)

**대상 모듈**: Module 6 (Shared Type Conversion Package)
**대상 요구사항**: REQ-M6-01 ~ REQ-M6-07, REQ-M2-01 ~ REQ-M2-06
**개발 방법론**: TDD (신규 파일)

**작업 항목**:

1. `internal/modbus/` 패키지 디렉토리 생성

2. `internal/modbus/types.go` 파일 생성
   - 데이터 타입 상수 정의 (`DataTypeUint16`, `DataTypeInt16`, `DataTypeFloat32`, `DataTypeUint32`, `DataTypeInt32`)
   - Byte Order 상수 정의 (`ByteOrderBigEndian`, `ByteOrderLittleEndian`)
   - `RegisterCountForType(dataType string) (uint16, error)` 헬퍼 함수
   - `IsValidDataType(dataType string) bool` 검증 함수
   - `TypeMapEntry`, `TypeOverlayEntry` 구조체 정의

3. 개별 타입 변환 함수 구현 (공개 함수)
   - `Float32ToRegisters(f float32, byteOrder string) [2]uint16`
   - `RegistersToFloat32(regs [2]uint16, byteOrder string) float32`
   - `Int32ToRegisters(i int32, byteOrder string) [2]uint16`
   - `RegistersToInt32(regs [2]uint16, byteOrder string) int32`
   - `Uint32ToRegisters(u uint32, byteOrder string) [2]uint16`
   - `RegistersToUint32(regs [2]uint16, byteOrder string) uint32`
   - `Int16ToRegister(i int16) uint16`
   - `RegisterToInt16(reg uint16) int16`

4. 제네릭 변환 함수 구현 (공개 함수)
   - `TypedValueToRegisters(value any, dataType string, byteOrder string) ([]uint16, error)`
   - `RegistersToTypedValue(regs []uint16, dataType string, byteOrder string) (any, error)`

5. `internal/modbus/types_test.go` 테이블 기반 테스트
   - 각 타입별 정상 변환 케이스
   - 경계값 테스트 (MaxFloat32, MinInt32, MaxUint32 등)
   - Big-Endian / Little-Endian 검증
   - 에러 케이스 (잘못된 타입, 부족한 레지스터 수)
   - Round-trip 테스트 (값 -> 레지스터 -> 값)

**완료 기준**: 모든 타입 변환 함수의 테스트 통과, 90%+ 커버리지

---

### Milestone 2: Server 설정 확장 (Primary Goal)

**대상 모듈**: Module 1 (Type Definition & Configuration), Module 5 (Configuration Enhancement)
**대상 요구사항**: REQ-M1-01 ~ REQ-M1-06, REQ-M5-01 ~ REQ-M5-05
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbusserver/types.go` 리팩토링
   - 로컬 타입 상수/구조체를 `internal/modbus` 패키지 import로 대체
   - type alias 또는 re-export 패턴 사용

2. `modbusserver/errors.go` 에러 상수 추가
   - `ErrTypeMapOverlap`: type_map 주소 겹침
   - `ErrTypeMapOutOfRange`: type_map 주소 범위 초과
   - `ErrUnsupportedDataType`: 지원하지 않는 데이터 타입

3. `modbusserver/config.go` 구조체 확장
   - `RegisterAreaConfig`에 `DataType string` 필드 추가
   - `RegisterAreaConfig`에 `TypeMap []modbus.TypeMapEntry` 필드 추가

4. `modbusserver/config.go` 파싱 로직 확장
   - `parseRegisterAreaConfig()`에 `data_type` 파싱 추가
   - `parseRegisterAreaConfig()`에 `type_map` 배열 파싱 추가
   - `validateTypeMap()` 검증 함수: 겹침 검사, 범위 검사, 유효 타입 검사

5. `modbusserver/config_test.go` 테스트 추가
   - `data_type` 필드 파싱 테스트
   - `type_map` 파싱 테스트
   - 겹침 검증 에러 테스트
   - 범위 초과 검증 에러 테스트
   - 알 수 없는 `data_type` 에러 테스트
   - 기존 테스트 전체 통과 확인 (하위 호환성)

**완료 기준**: 설정 파싱 및 검증 테스트 통과, 기존 config 테스트 무결

---

### Milestone 3: Server RegisterMap 타입 오버레이 (Secondary Goal)

**대상 모듈**: Module 3 (RegisterMap Type Overlay)
**대상 요구사항**: REQ-M3-01 ~ REQ-M3-05
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbusserver/register_map.go` 확장
   - `RegisterMap`에 `typeOverlay` 필드 추가 (공유 패키지 타입 사용)
   - `NewRegisterMap(cfg)` 에서 TypeOverlay 초기화
   - `ReadTyped(area string, address uint16, dataType string) (any, error)` 메서드
   - `WriteTyped(area string, address uint16, value any, dataType string) (*ChangeSet, error)` 메서드
   - `GetTypedSnapshot() map[string]any` 메서드 (타입 정보 포함 스냅샷)
   - 초기값(`initial_values`) 타입 적용 로직 수정

2. `modbusserver/register_map_test.go` 테스트 추가
   - TypeOverlay 초기화 검증
   - `ReadTyped` float32 읽기 테스트
   - `WriteTyped` float32 쓰기 테스트
   - `ReadTyped` int32, uint32, int16 테스트
   - raw uint16 읽기가 영향받지 않는지 검증
   - TypeOverlay 없는 주소에서 uint16 기본 동작 검증
   - `initial_values` 타입 적용 검증

**완료 기준**: ReadTyped/WriteTyped 테스트 통과, 기존 RegisterMap 테스트 무결

---

### Milestone 4: Server Process() 명령 확장 (Secondary Goal)

**대상 모듈**: Module 4 (Process Command Enhancement)
**대상 요구사항**: REQ-M4-01 ~ REQ-M4-07
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbusserver/agent.go` 기존 명령 확장
   - `processSetRegister()`: `data_type` 파라미터 처리 추가
   - `processSetRegisters()`: `data_type` 파라미터 처리 추가
   - `processSetInput()`: `data_type` 파라미터 처리 추가 (input_registers 영역)
   - `processSetInputs()`: `data_type` 파라미터 처리 추가 (input_registers 영역)

2. `modbusserver/agent.go` 신규 명령 추가
   - `processGetRegisterTyped()`: 타입 변환된 레지스터 값 조회
   - `processGetMap()` 수정: TypeOverlay 정보 포함

3. `modbusserver/agent.go` 이벤트 확장
   - `sendChangeEvent()`: `data_type` 필드 추가
   - `getParamFloat64()` 헬퍼 함수 추가
   - `getParamString()` 헬퍼 함수 추가

4. `modbusserver/agent_test.go` 테스트 추가
   - `set_register` + `data_type: float32` 테스트
   - `set_register` + `data_type: int32` 테스트
   - `set_registers` + `data_type: float32` 테스트 (복수 float32 값)
   - `get_register_typed` 명령 테스트
   - `get_map` 타입 정보 포함 검증
   - change event에 `data_type` 포함 검증
   - `data_type` 생략 시 기존 동작 유지 검증 (하위 호환성)
   - 잘못된 `data_type` 에러 테스트

**완료 기준**: 모든 Process 명령 테스트 통과, 기존 agent 테스트 무결

---

### Milestone 5: Server 통합 테스트 및 검증 (Secondary Goal)

**작업 항목**:

1. 전체 테스트 실행
   - `go test -race ./internal/agent/modbusserver/...`
   - 모든 기존 테스트 통과 확인
   - 모든 신규 테스트 통과 확인

2. 커버리지 확인
   - 전체 패키지 85%+ 커버리지 목표
   - `internal/modbus/types.go` 90%+ 커버리지 목표 (신규 파일)

3. 통합 시나리오 검증
   - float32 설정 -> 초기값 로드 -> Bridge 읽기 -> MODBUS 클라이언트 읽기 전체 흐름
   - 복합 type_map 설정 (float32 + int32 + uint32 혼합) 시나리오
   - Little-Endian byte_order 시나리오

4. 하위 호환성 최종 검증
   - 기존 YAML 설정(data_type/type_map 없음)으로 전체 동작 확인
   - 기존 Process 명령(data_type 없음)으로 전체 동작 확인

**완료 기준**: 전체 테스트 통과, 커버리지 목표 달성, 하위 호환성 검증 완료

---

### Milestone 6: Client 설정 확장 (Secondary Goal)

**대상 모듈**: Module 7 (Client Configuration Enhancement)
**대상 요구사항**: REQ-M7-01 ~ REQ-M7-05
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbus/config.go` 구조체 확장
   - `RegisterGroupConfig`에 `DataType string` 필드 추가 (`yaml:"data_type"`)
   - `RegisterGroupConfig`에 `TypeMap []modbus_types.TypeMapEntry` 필드 추가 (`yaml:"type_map"`)

2. `modbus/config.go` 파싱 로직 확장
   - `parseRegisterGroupConfig()`에 `data_type` 파싱 추가
   - `parseRegisterGroupConfig()`에 `type_map` 배열 파싱 추가
   - `validateClientTypeMap()` 검증 함수: 겹침 검사, 범위 검사 (`StartAddress + Quantity`), 유효 타입 검사

3. `modbus/errors.go` 에러 상수 추가
   - `ErrUnsupportedDataType`: 공유 패키지 에러와 일관성 유지

4. `modbus/config_test.go` 테스트 추가
   - `data_type` 필드 파싱 테스트
   - `type_map` 파싱 테스트
   - 겹침 검증 에러 테스트
   - 범위 초과 검증 에러 테스트
   - 알 수 없는 `data_type` 에러 테스트
   - 기존 테스트 전체 통과 확인 (하위 호환성)

**완료 기준**: 설정 파싱 및 검증 테스트 통과, 기존 config 테스트 무결

---

### Milestone 7: Client Cache 타입 오버레이 (Secondary Goal)

**대상 모듈**: Module 8 (Client Cache TypeOverlay)
**대상 요구사항**: REQ-M8-01 ~ REQ-M8-06
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbus/cache.go` TypeOverlay 통합
   - `RegisterCache`에 `typeOverlay` 필드 추가 (공유 패키지 타입 사용)
   - `SetTypeOverlay(overlay)` 메서드로 외부에서 TypeOverlay 주입
   - `ReadTyped(fc byte, address uint16, dataType string, byteOrder string) (any, error)` 메서드 추가
     - FC03: HoldingRegisters에서 읽기
     - FC04: InputRegisters에서 읽기
     - 적절한 레지스터 수만큼 연속 읽기 후 타입 변환

2. `modbus/cache.go` CompareAndUpdate 확장
   - `compareRegisterValues()` 수정: TypeOverlay 설정 시 `typed_changed_values` 추가
   - 타입 변환된 변경 값 생성 로직 (변경된 주소에 해당하는 타입 값 계산)

3. `modbus/cache.go` GetSnapshot 확장
   - TypeOverlay 설정 시 `typed_holding_registers`, `typed_input_registers` 필드 추가
   - 각 TypeOverlay 주소에 대해 타입 변환된 값 포함

4. `modbus/cache_test.go` 테스트 추가
   - TypeOverlay 설정 및 초기화 테스트
   - `ReadTyped` float32/int32/uint32/int16 읽기 테스트
   - raw uint16 읽기가 영향받지 않는지 검증
   - `CompareAndUpdate` typed 변경 감지 테스트
   - `GetSnapshot` typed 값 포함 테스트
   - TypeOverlay 없는 주소에서 uint16 기본 동작 검증

**완료 기준**: ReadTyped, CompareAndUpdate, GetSnapshot 테스트 통과, 기존 cache 테스트 무결

---

### Milestone 8: Client Process() 명령 확장 (Final Goal)

**대상 모듈**: Module 9 (Client Command Enhancement)
**대상 요구사항**: REQ-M9-01 ~ REQ-M9-10
**개발 방법론**: DDD (기존 파일 수정)

**작업 항목**:

1. `modbus/write.go` 쓰기 명령 확장
   - `processWriteRegister()` 수정:
     - `data_type` 파라미터 옵셔널 처리
     - 2-레지스터 타입(float32, uint32, int32): FC06 대신 FC16 자동 전환
     - 1-레지스터 타입(uint16, int16): 기존 FC06 유지
     - `paramFloat64()` 헬퍼 함수 추가 (float64 값 추출)
     - `paramString()` 헬퍼 함수 추가 (문자열 값 추출)
   - `processWriteRegisters()` 수정:
     - `data_type` 파라미터 옵셔널 처리
     - 각 값을 해당 타입의 레지스터 표현으로 변환
     - 변환된 uint16 슬라이스로 FC16 전송

2. `modbus/agent.go` 읽기/캐시 명령 확장
   - `read_registers` 응답 수정: TypeOverlay 설정 시 `typed_values` 필드 추가
   - `get_cache` 응답 수정: TypeOverlay 설정 시 typed 필드 추가
   - TypeOverlay 초기화: Agent 시작 시 설정에서 TypeOverlay 구축하여 각 디바이스 캐시에 주입

3. `modbus/agent.go` 이벤트 확장
   - `register_data` 이벤트: TypeOverlay 설정 시 `typed_values` 포함
   - `register_changed` 이벤트: TypeOverlay 설정 시 `typed_changed_values` 포함

4. `modbus/write_test.go` 테스트 추가
   - `write_register` + `data_type: float32` 테스트 (FC16 자동 전환 검증)
   - `write_register` + `data_type: int16` 테스트 (FC06 유지 검증)
   - `write_registers` + `data_type: float32` 테스트
   - `data_type` 생략 시 기존 동작 유지 검증
   - 잘못된 `data_type` 에러 테스트

5. `modbus/agent_test.go` 테스트 추가
   - `read_registers` typed_values 응답 테스트
   - `get_cache` typed 필드 포함 테스트
   - `register_data` 이벤트 typed_values 테스트
   - `register_changed` 이벤트 typed_changed_values 테스트
   - TypeOverlay 초기화 검증

**완료 기준**: 모든 Client Process 명령 테스트 통과, 기존 agent/write 테스트 무결

---

### Milestone 9: Client 통합 테스트 및 전체 검증 (Final Goal)

**작업 항목**:

1. Client 전체 테스트 실행
   - `go test -race ./internal/agent/modbus/...`
   - 모든 기존 테스트 통과 확인
   - 모든 신규 테스트 통과 확인

2. 공유 패키지 테스트 실행
   - `go test -race ./internal/modbus/...`
   - 90%+ 커버리지 확인

3. Client 커버리지 확인
   - 전체 패키지 85%+ 커버리지 목표

4. Client 통합 시나리오 검증
   - float32 type_map 설정 -> 디바이스 연결 -> 레지스터 읽기 -> typed_values 포함 응답
   - write_register + data_type: float32 -> FC16 전송 -> 캐시 갱신 -> typed 값 확인
   - write_registers + data_type: float32 -> 복수 값 변환 -> FC16 전송
   - register_changed 이벤트에 typed_changed_values 포함 확인
   - 복합 type_map (float32 + int32 + uint32 혼합) 시나리오

5. Client 하위 호환성 최종 검증
   - 기존 YAML 설정(data_type/type_map 없음)으로 전체 동작 확인
   - 기존 write_register/write_registers(data_type 없음)으로 전체 동작 확인
   - 기존 get_cache/register_data/register_changed 이벤트 기존 형식 유지 확인

6. 전체 프로젝트 테스트
   - `go test -race ./...` 전체 패스 확인
   - Server + Client + 공유 패키지 상호작용 무결성 확인

**완료 기준**: 전체 테스트 통과, 커버리지 목표 달성, 하위 호환성 검증 완료

---

## 3. 리스크 및 대응

### 3.1 기술 리스크

| 리스크 | 심각도 | 대응 방안 |
|--------|--------|----------|
| float32 정밀도 손실 | Medium | IEEE 754 라운드트립 테스트로 정밀도 검증; Go의 `math.Float32frombits` 사용 |
| Byte Order 혼동 | Medium | 명시적 Big-Endian 기본값; 테스트에서 Big/Little 모두 검증 |
| TypeMap 겹침 감지 실패 | High | 설정 파싱 시점에 O(n^2) 검증; 에러 메시지에 겹치는 주소 명시 |
| 기존 테스트 깨짐 | High | Milestone별 기존 테스트 전수 실행; 구조체 필드 추가는 zero-value 호환 |
| 동시성 문제 | Medium | 기존 `sync.RWMutex` 패턴 유지; `ReadTyped`에서 동일한 락 사용 |
| 공유 패키지 순환 import | Medium | `internal/modbus/`는 순수 유틸리티로 외부 의존성 없음; Server/Client만 import |

### 3.2 설계 리스크

| 리스크 | 심각도 | 대응 방안 |
|--------|--------|----------|
| TypeOverlay가 복잡해짐 | Low | 공유 패키지에서 관리; 인터페이스 추출은 추후 필요 시 |
| Process 명령 API 비대화 | Low | 기존 명령에 선택적 파라미터 추가; 신규 명령 최소화 |
| 설정 YAML 복잡도 증가 | Low | `data_type` 기본값으로 단순 사용; `type_map`은 고급 옵션 |
| FC06->FC16 자동 전환 혼동 | Medium | write_register 응답에 실제 사용된 FC 코드 포함; 문서화 |
| Client/Server 타입 불일치 | Low | 공유 패키지로 동일 변환 로직 보장 |

### 3.3 Client 전용 리스크

| 리스크 | 심각도 | 대응 방안 |
|--------|--------|----------|
| FC06->FC16 전환 시 MODBUS 서버 호환성 | Medium | FC16은 표준 Function Code; 대부분의 서버가 지원 |
| 캐시 TypeOverlay 메모리 사용량 | Low | TypeOverlay는 설정된 주소만 저장; 대부분 수십 개 수준 |
| register_changed 이벤트 성능 | Low | 타입 변환은 변경된 주소에 대해서만 수행; 전체 캐시 변환 아님 |

---

## 4. 아키텍처 설계 방향

### 4.1 레이어 다이어그램

```
┌──────────────────────────────────────────────────────────┐
│              Shared Package (internal/modbus/)             │
│  DataType 상수, ByteOrder 상수, TypeMapEntry 구조체          │
│  TypedValueToRegisters / RegistersToTypedValue             │
│  RegisterCountForType / IsValidDataType                    │
├──────────────────────────────────────────────────────────┤
│                                                            │
│  ┌─────────────────────────┐  ┌──────────────────────────┐│
│  │ Server (modbusserver/)   │  │ Client (modbus/)          ││
│  │                          │  │                           ││
│  │ Bridge Layer (agent.go)  │  │ Process Layer (agent.go)  ││
│  │ set_register, get_typed  │  │ write_register, read_regs ││
│  │ data_type 파라미터 처리   │  │ data_type 파라미터 처리    ││
│  ├──────────────────────────┤  ├───────────────────────────┤│
│  │ Type Overlay             │  │ Cache TypeOverlay          ││
│  │ (register_map.go)        │  │ (cache.go)                ││
│  │ ReadTyped / WriteTyped   │  │ ReadTyped / CompareUpdate ││
│  ├──────────────────────────┤  ├───────────────────────────┤│
│  │ RegisterMap              │  │ RegisterCache              ││
│  │ map[uint16]uint16        │  │ map[uint16]uint16          ││
│  │ (변경 없음)              │  │ (변경 없음)               ││
│  ├──────────────────────────┤  ├───────────────────────────┤│
│  │ Wire Protocol            │  │ Wire Protocol              ││
│  │ request.go (변경 없음)    │  │ protocol.go (변경 없음)    ││
│  └─────────────────────────┘  └──────────────────────────┘│
└──────────────────────────────────────────────────────────┘
```

### 4.2 변경 영향도

#### 공유 패키지

| 파일 | 변경 규모 | 위험도 |
|------|----------|--------|
| `internal/modbus/types.go` (신규) | ~250 LOC | Low (신규 파일, 의존성 없음) |
| `internal/modbus/types_test.go` (신규) | ~350 LOC | Low (신규 파일) |

#### Server

| 파일 | 변경 규모 | 위험도 |
|------|----------|--------|
| `types.go` | ~30 LOC 수정 | Low (import 대체) |
| `errors.go` | +3 상수 | Low (추가만) |
| `config.go` | +80 LOC | Medium (파싱 로직 확장) |
| `config_test.go` | +100 LOC | Low (테스트 추가) |
| `register_map.go` | +80 LOC | Medium (메서드 추가, 초기화 수정) |
| `register_map_test.go` | +120 LOC | Low (테스트 추가) |
| `agent.go` | +100 LOC | Medium (명령 로직 확장) |
| `agent_test.go` | +150 LOC | Low (테스트 추가) |

#### Client

| 파일 | 변경 규모 | 위험도 |
|------|----------|--------|
| `config.go` | +60 LOC | Medium (파싱 로직 확장) |
| `config_test.go` | +80 LOC | Low (테스트 추가) |
| `cache.go` | +100 LOC | Medium (TypeOverlay, ReadTyped, 스냅샷 확장) |
| `cache_test.go` | +120 LOC | Low (테스트 추가) |
| `write.go` | +80 LOC | Medium (FC06->FC16 전환, 타입 파라미터) |
| `write_test.go` | +100 LOC | Low (테스트 추가) |
| `agent.go` | +80 LOC | Medium (이벤트 확장, TypeOverlay 초기화) |
| `agent_test.go` | +120 LOC | Low (테스트 추가) |
| `errors.go` | +1 상수 | Low (추가만) |

**총 변경량**: 약 +2,080 LOC (공유 ~250, Server 소스 ~290, Client 소스 ~320, 테스트 ~1,220)

---

## 5. 전문가 컨설팅 권장

### expert-backend 컨설팅 제안

이 SPEC는 다음 영역에서 expert-backend 컨설팅이 유용할 수 있다:

- **Byte Order 처리**: MODBUS 디바이스별 Big-Endian / Little-Endian / Mid-Endian(word-swapped) 변종 처리 전략
- **타입 변환 성능**: 대량 레지스터 읽기 시 타입 변환 오버헤드 최적화
- **동시성 패턴**: ReadTyped와 기존 raw 읽기/쓰기의 락 경합 최소화
- **FC06->FC16 전환 전략**: Client에서 자동 Function Code 전환의 에지 케이스 및 MODBUS 서버 호환성

---

## 6. 다음 단계

1. `/moai:2-run SPEC-MODBUS-003`으로 구현 시작
2. Milestone 1 (공유 타입 변환 패키지)부터 TDD로 진행
3. Milestone 2-5는 Server DDD (ANALYZE-PRESERVE-IMPROVE) 순서로 진행
4. Milestone 6-8은 Client DDD 순서로 진행
5. Milestone 9에서 전체 통합 검증
6. `/moai:3-sync SPEC-MODBUS-003`으로 문서 동기화
