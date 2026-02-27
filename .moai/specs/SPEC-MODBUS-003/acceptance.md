# SPEC-MODBUS-003: 인수 기준 (Acceptance Criteria)

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-003 |
| 제목 | MODBUS/TCP Multi-Data-Type Support (Server + Client) |
| 테스트 형식 | Given-When-Then (Gherkin) |
| 커버리지 목표 | 85%+ (전체 패키지), 90%+ (internal/modbus/types.go) |

---

## 1. Module 6: 공유 타입 변환 패키지 (`internal/modbus/`)

### AC-M6-01: float32 Big-Endian 변환

```gherkin
Scenario: float32 값을 Big-Endian 레지스터로 변환
  Given float32 값 3.14가 주어졌을 때
  When Float32ToRegisters(3.14, "big_endian")를 호출하면
  Then [0x4048, 0xF5C3] (IEEE 754 Big-Endian)이 반환되어야 한다

Scenario: Big-Endian 레지스터를 float32로 역변환
  Given uint16 레지스터 [0x4048, 0xF5C3]이 주어졌을 때
  When RegistersToFloat32([0x4048, 0xF5C3], "big_endian")를 호출하면
  Then float32 값 3.14(오차 1e-6 이내)가 반환되어야 한다
```

### AC-M6-02: float32 Round-Trip 검증

```gherkin
Scenario Outline: float32 왕복 변환 정밀도 검증
  Given float32 값 <input>이 주어졌을 때
  When Float32ToRegisters -> RegistersToFloat32 왕복 변환을 수행하면
  Then 결과가 원래 값과 동일해야 한다 (비트 수준 일치)

  Examples:
    | input           | 설명               |
    | 0.0             | 영                 |
    | -0.0            | 음수 영             |
    | 1.0             | 단위값              |
    | -1.0            | 음수 단위값          |
    | 3.14            | 일반 실수           |
    | 1.175494e-38    | MinFloat32 (최소 양수) |
    | 3.4028235e+38   | MaxFloat32 (최대값)  |
    | math.Inf(1)     | 양의 무한대          |
    | math.Inf(-1)    | 음의 무한대          |
    | math.NaN()      | NaN                |
```

### AC-M6-03: float32 Little-Endian 변환

```gherkin
Scenario: float32 값을 Little-Endian 레지스터로 변환
  Given float32 값 3.14가 주어졌을 때
  When Float32ToRegisters(3.14, "little_endian")를 호출하면
  Then [0xF5C3, 0x4048] (워드 순서 반전)이 반환되어야 한다

Scenario: Little-Endian 레지스터를 float32로 역변환
  Given uint16 레지스터 [0xF5C3, 0x4048]이 주어졌을 때
  When RegistersToFloat32([0xF5C3, 0x4048], "little_endian")를 호출하면
  Then float32 값 3.14(오차 1e-6 이내)가 반환되어야 한다
```

### AC-M6-04: int32 변환

```gherkin
Scenario: 양수 int32를 레지스터로 변환
  Given int32 값 100000이 주어졌을 때
  When Int32ToRegisters(100000, "big_endian")를 호출하면
  Then [0x0001, 0x86A0]이 반환되어야 한다

Scenario: 음수 int32를 레지스터로 변환
  Given int32 값 -1이 주어졌을 때
  When Int32ToRegisters(-1, "big_endian")를 호출하면
  Then [0xFFFF, 0xFFFF]이 반환되어야 한다

Scenario: int32 경계값 Round-Trip
  Given int32 값 math.MinInt32 (-2147483648)이 주어졌을 때
  When Int32ToRegisters -> RegistersToInt32 왕복 변환을 수행하면
  Then 결과가 math.MinInt32와 동일해야 한다
```

### AC-M6-05: uint32 변환

```gherkin
Scenario: uint32를 레지스터로 변환
  Given uint32 값 70000이 주어졌을 때
  When Uint32ToRegisters(70000, "big_endian")를 호출하면
  Then [0x0001, 0x1170]이 반환되어야 한다

Scenario: uint32 최대값 Round-Trip
  Given uint32 값 math.MaxUint32 (4294967295)가 주어졌을 때
  When Uint32ToRegisters -> RegistersToUint32 왕복 변환을 수행하면
  Then 결과가 math.MaxUint32와 동일해야 한다
```

### AC-M6-06: int16 변환

```gherkin
Scenario: 음수 int16를 uint16 레지스터로 변환
  Given int16 값 -1이 주어졌을 때
  When Int16ToRegister(-1)를 호출하면
  Then uint16 값 0xFFFF가 반환되어야 한다

Scenario: uint16를 음수 int16으로 역변환
  Given uint16 값 0xFFFF가 주어졌을 때
  When RegisterToInt16(0xFFFF)를 호출하면
  Then int16 값 -1이 반환되어야 한다

Scenario: int16 경계값 검증
  Given int16 값 -32768 (MinInt16)이 주어졌을 때
  When Int16ToRegister(-32768)를 호출하면
  Then uint16 값 0x8000이 반환되어야 한다
```

### AC-M6-07: 제네릭 변환 함수

```gherkin
Scenario: TypedValueToRegisters로 float32 변환
  Given any 타입의 float64 값 3.14와 dataType "float32"가 주어졌을 때
  When TypedValueToRegisters(3.14, "float32", "big_endian")를 호출하면
  Then []uint16{0x4048, 0xF5C3}이 반환되어야 한다
  And error는 nil이어야 한다

Scenario: RegistersToTypedValue로 float32 역변환
  Given []uint16{0x4048, 0xF5C3}과 dataType "float32"가 주어졌을 때
  When RegistersToTypedValue(regs, "float32", "big_endian")를 호출하면
  Then float32(3.14)가 반환되어야 한다
  And error는 nil이어야 한다

Scenario: 지원하지 않는 타입에 대한 에러
  Given dataType "float64"가 주어졌을 때
  When TypedValueToRegisters(1.0, "float64", "big_endian")를 호출하면
  Then error가 반환되어야 한다
```

### AC-M6-08: 에러 처리 (패닉 방지)

```gherkin
Scenario: 부족한 레지스터로 float32 디코딩 시 에러 반환
  Given []uint16{0x4048} (1개 레지스터)와 dataType "float32"가 주어졌을 때
  When RegistersToTypedValue(regs, "float32", "big_endian")를 호출하면
  Then error가 반환되어야 한다
  And 패닉이 발생하지 않아야 한다

Scenario: 빈 레지스터로 디코딩 시 에러 반환
  Given 빈 []uint16{}과 dataType "int32"가 주어졌을 때
  When RegistersToTypedValue(regs, "int32", "big_endian")를 호출하면
  Then error가 반환되어야 한다
  And 패닉이 발생하지 않아야 한다
```

### AC-M6-09: 헬퍼 함수

```gherkin
Scenario: RegisterCountForType 정상 동작
  Given dataType "float32"가 주어졌을 때
  When RegisterCountForType("float32")를 호출하면
  Then uint16(2)가 반환되어야 한다
  And error는 nil이어야 한다

Scenario: IsValidDataType 정상 동작
  Given dataType "uint16"가 주어졌을 때
  When IsValidDataType("uint16")를 호출하면
  Then true가 반환되어야 한다

Scenario: IsValidDataType 잘못된 타입
  Given dataType "float64"가 주어졌을 때
  When IsValidDataType("float64")를 호출하면
  Then false가 반환되어야 한다
```

---

## 2. Module 1 & 5: Server 설정 파싱 및 검증

### AC-M1-01: data_type 필드 파싱

```gherkin
Scenario: RegisterAreaConfig에서 data_type 파싱
  Given YAML 설정에 data_type: "float32"가 포함되었을 때
  When parseRegisterAreaConfig()를 호출하면
  Then RegisterAreaConfig.DataType이 "float32"이어야 한다

Scenario: data_type 생략 시 기본값
  Given YAML 설정에 data_type가 없을 때
  When parseRegisterAreaConfig()를 호출하면
  Then RegisterAreaConfig.DataType이 "" (빈 문자열)이어야 한다
  And 시스템은 이를 "uint16"으로 해석해야 한다
```

### AC-M1-02: type_map 파싱

```gherkin
Scenario: type_map 배열 파싱
  Given YAML 설정에 다음 type_map이 포함되었을 때:
    | address | data_type | byte_order     |
    | 0       | float32   | big_endian     |
    | 2       | int32     |                |
    | 10      | int16     |                |
  When parseRegisterAreaConfig()를 호출하면
  Then RegisterAreaConfig.TypeMap이 3개의 엔트리를 가져야 한다
  And TypeMap[0].Address가 0이어야 한다
  And TypeMap[0].DataType이 "float32"이어야 한다
  And TypeMap[0].ByteOrder가 "big_endian"이어야 한다
  And TypeMap[1].ByteOrder가 "" (빈 문자열, 기본 big_endian)이어야 한다
```

### AC-M1-03: type_map 주소 겹침 검증

```gherkin
Scenario: float32 영역이 겹치는 type_map 검증
  Given type_map에 다음이 포함되었을 때:
    | address | data_type |
    | 0       | float32   |  (주소 0-1 점유)
    | 1       | uint32    |  (주소 1-2 점유, 겹침!)
  When parseRegisterAreaConfig()를 호출하면
  Then ErrTypeMapOverlap 에러가 반환되어야 한다

Scenario: 겹치지 않는 type_map 검증
  Given type_map에 다음이 포함되었을 때:
    | address | data_type |
    | 0       | float32   |  (주소 0-1 점유)
    | 2       | uint32    |  (주소 2-3 점유, OK)
  When parseRegisterAreaConfig()를 호출하면
  Then 에러가 발생하지 않아야 한다
```

### AC-M1-04: type_map 범위 초과 검증

```gherkin
Scenario: type_map 주소가 영역 범위를 초과
  Given start_address: 0, count: 5인 영역에서
  And type_map에 address: 4, data_type: "float32" (주소 4-5, count 초과)가 있을 때
  When parseRegisterAreaConfig()를 호출하면
  Then ErrTypeMapOutOfRange 에러가 반환되어야 한다
```

### AC-M1-05: 지원하지 않는 data_type 검증

```gherkin
Scenario: 알 수 없는 data_type 에러
  Given data_type: "float64"가 지정되었을 때
  When parseRegisterAreaConfig()를 호출하면
  Then ErrUnsupportedDataType 에러가 반환되어야 한다

Scenario: type_map 내 알 수 없는 data_type 에러
  Given type_map에 data_type: "string"인 엔트리가 있을 때
  When parseRegisterAreaConfig()를 호출하면
  Then ErrUnsupportedDataType 에러가 반환되어야 한다
```

### AC-M1-06: Server 하위 호환성

```gherkin
Scenario: 기존 설정(data_type/type_map 없음)으로 정상 동작
  Given 기존 YAML 설정에 data_type와 type_map이 없을 때
  When parseModbusServerConfig()를 호출하면
  Then 에러 없이 파싱되어야 한다
  And RegisterAreaConfig.DataType이 ""이어야 한다
  And RegisterAreaConfig.TypeMap이 nil 또는 빈 슬라이스이어야 한다
```

---

## 3. Module 3: Server RegisterMap 타입 오버레이

### AC-M3-01: TypeOverlay 초기화

```gherkin
Scenario: 설정에서 TypeOverlay 자동 구축
  Given holding_registers에 type_map [{address:0, data_type:"float32"}]가 설정되었을 때
  When NewRegisterMap(cfg)를 호출하면
  Then RegisterMap의 TypeOverlay가 nil이 아니어야 한다
  And TypeOverlay.GetDataType("holding_registers", 0)이 "float32"를 반환해야 한다
  And TypeOverlay.GetDataType("holding_registers", 5)가 "uint16"을 반환해야 한다 (기본값)
```

### AC-M3-02: ReadTyped float32

```gherkin
Scenario: float32 타입 레지스터 읽기
  Given holding_registers 주소 0에 raw 값 [0x4048, 0xF5C3]이 저장되어 있을 때
  When ReadTyped("holding_registers", 0, "float32")를 호출하면
  Then float32(3.14) (오차 1e-6 이내)가 반환되어야 한다

Scenario: float32 타입 레지스터 읽기 (주소 범위 검증)
  Given holding_registers 주소 범위가 0-9일 때
  And 주소 9에 float32 읽기를 시도하면 (주소 9-10 필요, 범위 초과)
  When ReadTyped("holding_registers", 9, "float32")를 호출하면
  Then ErrAddressNotMapped 에러가 반환되어야 한다
```

### AC-M3-03: WriteTyped float32

```gherkin
Scenario: float32 값을 타입 레지스터로 쓰기
  Given holding_registers 주소 0-1이 사용 가능할 때
  When WriteTyped("holding_registers", 0, float64(3.14), "float32")를 호출하면
  Then 주소 0에 0x4048이 저장되어야 한다
  And 주소 1에 0xF5C3이 저장되어야 한다
  And ChangeSet이 반환되어야 한다
```

### AC-M3-04: raw 읽기 비영향

```gherkin
Scenario: TypeOverlay가 raw uint16 읽기에 영향 없음
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And 주소 0-1에 raw 값 [0x4048, 0xF5C3]이 저장되어 있을 때
  When ReadHoldingRegisters(0, 2)를 호출하면
  Then []uint16{0x4048, 0xF5C3}이 반환되어야 한다 (타입 변환 없음)
```

### AC-M3-05: 초기값 타입 적용

```gherkin
Scenario: float32 타입의 초기값 로드
  Given holding_registers 설정에 data_type: "float32"와 initial_values: [3.14, -1.5]가 있을 때
  When NewRegisterMap(cfg)를 호출하면
  Then 주소 0-1에 3.14의 IEEE 754 Big-Endian 표현이 저장되어야 한다
  And 주소 2-3에 -1.5의 IEEE 754 Big-Endian 표현이 저장되어야 한다
```

---

## 4. Module 4: Server Process() 명령 확장

### AC-M4-01: set_register with data_type

```gherkin
Scenario: float32 타입으로 레지스터 설정
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": 3.14, "data_type": "float32"}}
  Then 응답에 {"ok": true, "address": 0, "value": 3.14, "data_type": "float32"}가 포함되어야 한다
  And 레지스터 주소 0-1에 float32(3.14)의 Big-Endian 표현이 저장되어야 한다

Scenario: int32 타입으로 레지스터 설정
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": -100000, "data_type": "int32"}}
  Then 응답에 {"ok": true}가 포함되어야 한다
  And 레지스터 주소 0-1에 int32(-100000)의 Big-Endian 표현이 저장되어야 한다

Scenario: int16 타입으로 레지스터 설정
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": -1, "data_type": "int16"}}
  Then 레지스터 주소 0에 0xFFFF가 저장되어야 한다
```

### AC-M4-02: set_register without data_type (하위 호환성)

```gherkin
Scenario: data_type 없이 레지스터 설정 (기존 동작 유지)
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": 42}}
  Then 레지스터 주소 0에 uint16(42)가 저장되어야 한다
  And 기존 동작과 100% 동일해야 한다
```

### AC-M4-03: set_registers with data_type

```gherkin
Scenario: 복수 float32 값을 한 번에 설정
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_registers", "params": {"address": 0, "values": [3.14, -1.5], "data_type": "float32"}}
  Then 레지스터 주소 0-3에 두 개의 float32 값이 저장되어야 한다
  And 응답에 {"ok": true, "address": 0, "quantity": 4}가 포함되어야 한다 (4 레지스터 = 2 float32)
```

### AC-M4-04: get_register_typed 신규 명령

```gherkin
Scenario: 타입 변환된 레지스터 값 읽기
  Given 주소 0-1에 float32(3.14)가 저장되어 있을 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "get_register_typed", "params": {"address": 0, "area": "holding_registers", "data_type": "float32"}}
  Then 응답에 {"ok": true, "address": 0, "value": 3.14, "data_type": "float32"}가 포함되어야 한다

Scenario: area 파라미터 생략 시 holding_registers 기본값
  Given 주소 0에 uint16(42)가 저장되어 있을 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "get_register_typed", "params": {"address": 0, "data_type": "uint16"}}
  Then 응답에 {"ok": true, "value": 42}가 포함되어야 한다

Scenario: input_registers 영역에서 타입 읽기
  Given input_registers 주소 100-101에 float32(25.5)가 저장되어 있을 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "get_register_typed", "params": {"address": 100, "area": "input_registers", "data_type": "float32"}}
  Then 응답에 {"ok": true, "value": 25.5}가 포함되어야 한다
```

### AC-M4-05: get_map 타입 정보 포함

```gherkin
Scenario: get_map에 TypeOverlay 정보 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And 주소 0-1에 float32(3.14)가 저장되어 있을 때
  When Process()에 {"command": "get_map"}이 전달되면
  Then 응답의 register_map에 type_overlay 정보가 포함되어야 한다
  And holding_registers의 typed_values에 주소 0의 float32 값이 포함되어야 한다
```

### AC-M4-06: Server Change Event에 data_type 포함

```gherkin
Scenario: 타입 지정 쓰기 시 change event에 data_type 포함
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 {"command": "set_register", "params": {"address": 0, "value": 3.14, "data_type": "float32"}}가 전달되면
  Then msgCh에 전송된 notification에 "data_type": "float32"가 포함되어야 한다

Scenario: 타입 미지정 쓰기 시 change event에 data_type 없음 (하위 호환)
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 {"command": "set_register", "params": {"address": 0, "value": 42}}가 전달되면
  Then msgCh에 전송된 notification에 "data_type" 키가 없거나 빈 값이어야 한다
```

### AC-M4-07: 자동 타입 변환

```gherkin
Scenario: 정수 값이 float32 타입으로 자동 변환
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": 42, "data_type": "float32"}}
  Then 정수 42가 float32(42.0)으로 변환되어 저장되어야 한다
  And 에러가 발생하지 않아야 한다
```

### AC-M4-08: Server 에러 케이스

```gherkin
Scenario: 잘못된 data_type 에러
  Given MODBUS 서버 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "set_register", "params": {"address": 0, "value": 42, "data_type": "float64"}}
  Then 에러가 반환되어야 한다
  And 에러 메시지에 "unsupported" 또는 "invalid" data_type 관련 내용이 포함되어야 한다
```

---

## 5. Server 통합 시나리오

### AC-INT-01: End-to-End float32 시나리오

```gherkin
Scenario: float32 설정 -> 초기화 -> Bridge 쓰기 -> Bridge 읽기 -> MODBUS 읽기
  Given 다음 설정으로 MODBUS 서버를 생성할 때:
    holding_registers:
      start_address: 0
      count: 10
      type_map:
        - address: 0
          data_type: "float32"
  And 서버가 시작된 후
  When Process()로 set_register(address=0, value=25.5, data_type="float32")를 실행하면
  Then get_register_typed(address=0, data_type="float32")가 25.5를 반환해야 한다
  And ReadHoldingRegisters(0, 2)로 raw uint16 레지스터를 읽으면 IEEE 754 Big-Endian 바이트가 반환되어야 한다

Scenario: 복합 type_map 통합 시나리오
  Given 다음 설정으로 MODBUS 서버를 생성할 때:
    holding_registers:
      start_address: 0
      count: 10
      type_map:
        - address: 0
          data_type: "float32"
        - address: 2
          data_type: "int32"
        - address: 4
          data_type: "uint32"
        - address: 6
          data_type: "int16"
  When 각 주소에 적절한 타입의 값을 쓰면
  Then get_register_typed로 각 값이 정확히 읽혀야 한다
  And get_map 결과에 모든 타입 정보가 포함되어야 한다
```

### AC-INT-02: Little-Endian 통합 시나리오

```gherkin
Scenario: Little-Endian byte_order 설정 및 동작
  Given 다음 설정으로 MODBUS 서버를 생성할 때:
    holding_registers:
      start_address: 0
      count: 4
      type_map:
        - address: 0
          data_type: "float32"
          byte_order: "big_endian"
        - address: 2
          data_type: "float32"
          byte_order: "little_endian"
  When 주소 0과 주소 2 모두에 float32(3.14)를 쓰면
  Then raw 레지스터 읽기에서 주소 0-1과 주소 2-3의 워드 순서가 반대여야 한다
  And get_register_typed로 두 주소 모두 3.14를 읽어야 한다
```

### AC-INT-03: 초기값 타입 적용 시나리오

```gherkin
Scenario: float32 초기값으로 서버 시작
  Given 다음 설정으로 MODBUS 서버를 생성할 때:
    holding_registers:
      start_address: 0
      count: 4
      data_type: "float32"
      initial_values: [3.14, -1.5]
  When 서버가 초기화되면
  Then get_register_typed(address=0, data_type="float32")가 3.14를 반환해야 한다
  And get_register_typed(address=2, data_type="float32")가 -1.5를 반환해야 한다
```

---

## 6. Module 7: Client 설정 파싱 및 검증

### AC-M7-01: Client data_type 필드 파싱

```gherkin
Scenario: RegisterGroupConfig에서 data_type 파싱
  Given YAML 설정의 register_groups에 data_type: "float32"가 포함되었을 때
  When parseRegisterGroupConfig()를 호출하면
  Then RegisterGroupConfig.DataType이 "float32"이어야 한다

Scenario: Client data_type 생략 시 기본값
  Given YAML 설정에 data_type가 없을 때
  When parseRegisterGroupConfig()를 호출하면
  Then RegisterGroupConfig.DataType이 "" (빈 문자열)이어야 한다
  And 시스템은 이를 "uint16"으로 해석해야 한다
```

### AC-M7-02: Client type_map 파싱

```gherkin
Scenario: Client type_map 배열 파싱
  Given YAML 설정의 register_groups에 다음 type_map이 포함되었을 때:
    | address | data_type | byte_order     |
    | 100     | float32   | big_endian     |
    | 102     | int32     |                |
    | 105     | int16     |                |
  When parseRegisterGroupConfig()를 호출하면
  Then RegisterGroupConfig.TypeMap이 3개의 엔트리를 가져야 한다
  And TypeMap[0].Address가 100이어야 한다
  And TypeMap[0].DataType이 "float32"이어야 한다
```

### AC-M7-03: Client type_map 주소 겹침 검증

```gherkin
Scenario: Client type_map 겹침 검증
  Given start_address: 0, quantity: 10인 register_group에서
  And type_map에 다음이 포함되었을 때:
    | address | data_type |
    | 0       | float32   |  (주소 0-1 점유)
    | 1       | uint32    |  (주소 1-2 점유, 겹침!)
  When parseRegisterGroupConfig()를 호출하면
  Then 에러가 반환되어야 한다
```

### AC-M7-04: Client type_map 범위 초과 검증

```gherkin
Scenario: Client type_map 범위 초과
  Given start_address: 100, quantity: 5인 register_group에서
  And type_map에 address: 104, data_type: "float32" (주소 104-105, 범위 초과)가 있을 때
  When parseRegisterGroupConfig()를 호출하면
  Then 에러가 반환되어야 한다
```

### AC-M7-05: Client 지원하지 않는 data_type 검증

```gherkin
Scenario: Client 알 수 없는 data_type 에러
  Given data_type: "float64"가 register_group에 지정되었을 때
  When parseRegisterGroupConfig()를 호출하면
  Then ErrUnsupportedDataType 에러가 반환되어야 한다
```

### AC-M7-06: Client 설정 하위 호환성

```gherkin
Scenario: 기존 Client 설정(data_type/type_map 없음)으로 정상 동작
  Given 기존 YAML 설정에 register_groups에 data_type와 type_map이 없을 때
  When parseModbusConfig()를 호출하면
  Then 에러 없이 파싱되어야 한다
  And RegisterGroupConfig.DataType이 ""이어야 한다
  And RegisterGroupConfig.TypeMap이 nil 또는 빈 슬라이스이어야 한다
```

---

## 7. Module 8: Client Cache 타입 오버레이

### AC-M8-01: Client TypeOverlay 설정

```gherkin
Scenario: RegisterCache에 TypeOverlay 설정
  Given RegisterGroupConfig에 type_map [{address:0, data_type:"float32"}]가 설정되었을 때
  When TypeOverlay를 구축하여 RegisterCache에 SetTypeOverlay()로 주입하면
  Then RegisterCache의 typeOverlay가 nil이 아니어야 한다
```

### AC-M8-02: Client ReadTyped float32

```gherkin
Scenario: Cache에서 float32 타입 읽기
  Given HoldingRegisters에 주소 0: 0x4048, 주소 1: 0xF5C3이 캐시되어 있을 때
  When ReadTyped(FC03, 0, "float32", "big_endian")를 호출하면
  Then float32(3.14) (오차 1e-6 이내)가 반환되어야 한다

Scenario: Cache에서 int32 타입 읽기
  Given HoldingRegisters에 주소 0: 0x0001, 주소 1: 0x86A0이 캐시되어 있을 때
  When ReadTyped(FC03, 0, "int32", "big_endian")를 호출하면
  Then int32(100000)이 반환되어야 한다
```

### AC-M8-03: Client raw 읽기 비영향

```gherkin
Scenario: Client TypeOverlay가 기존 UpdateHoldingRegisters에 영향 없음
  Given TypeOverlay가 설정되어 있을 때
  When UpdateHoldingRegisters(0, []uint16{0x4048, 0xF5C3})를 호출하면
  Then HoldingRegisters[0]이 0x4048이어야 한다
  And HoldingRegisters[1]이 0xF5C3이어야 한다
  And 기존 동작과 100% 동일해야 한다

Scenario: Client TypeOverlay가 기존 GetSnapshot에 영향 없음 (raw 필드)
  Given TypeOverlay가 설정되어 있을 때
  When GetSnapshot()를 호출하면
  Then "holding_registers" 필드에 raw uint16 값이 포함되어야 한다
  And 기존 필드 구조가 변경되지 않아야 한다
```

### AC-M8-04: Client CompareAndUpdate typed 변경 감지

```gherkin
Scenario: CompareAndUpdate에서 typed 변경 감지
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And 기존 캐시에 주소 0-1에 float32(3.14) 값이 있을 때
  When CompareAndUpdate(FC03, 0, newRawData, 4)를 호출하고 주소 0-1이 변경되었을 때
  Then changed가 true이어야 한다
  And changedData에 "typed_changed_values" 키가 포함되어야 한다
  And typed_changed_values에 float32 타입의 새 값이 포함되어야 한다
```

### AC-M8-05: Client GetSnapshot typed 값 포함

```gherkin
Scenario: GetSnapshot에 typed 값 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And HoldingRegisters에 주소 0-1에 float32(25.5) 값이 캐시되어 있을 때
  When GetSnapshot()를 호출하면
  Then 반환 맵에 "typed_holding_registers" 키가 포함되어야 한다
  And typed_holding_registers에 주소 0의 float32(25.5) 값이 포함되어야 한다

Scenario: TypeOverlay 미설정 시 GetSnapshot에 typed 필드 없음
  Given TypeOverlay가 설정되지 않았을 때
  When GetSnapshot()를 호출하면
  Then 반환 맵에 "typed_holding_registers" 키가 없어야 한다
  And 기존 응답 형식과 동일해야 한다
```

---

## 8. Module 9: Client Process() 명령 확장

### AC-M9-01: Client write_register with data_type (FC16 자동 전환)

```gherkin
Scenario: float32 타입으로 write_register (FC06 -> FC16 자동 전환)
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": 3.14, "data_type": "float32"}}
  Then FC16 (Write Multiple Registers) 프레임이 2개 레지스터로 전송되어야 한다
  And 응답에 {"status": "ok", "quantity": 2}가 포함되어야 한다

Scenario: int16 타입으로 write_register (FC06 유지)
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": -1, "data_type": "int16"}}
  Then FC06 (Write Single Register) 프레임이 전송되어야 한다
  And 레지스터 값이 int16(-1) = 0xFFFF로 변환되어야 한다

Scenario: uint32 타입으로 write_register (FC16 자동 전환)
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": 100000, "data_type": "uint32"}}
  Then FC16 프레임이 2개 레지스터로 전송되어야 한다
```

### AC-M9-02: Client write_register without data_type (하위 호환성)

```gherkin
Scenario: data_type 없이 write_register (기존 동작 유지)
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": 42}}
  Then FC06 (Write Single Register) 프레임이 전송되어야 한다
  And uint16(42)가 기존 방식 그대로 전송되어야 한다
  And 기존 동작과 100% 동일해야 한다
```

### AC-M9-03: Client write_registers with data_type

```gherkin
Scenario: 복수 float32 값을 write_registers로 전송
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_registers", "params": {"device_id": "dev1", "address": 0, "values": [3.14, -1.5], "data_type": "float32"}}
  Then FC16 프레임이 4개 레지스터 (2 float32 x 2 registers)로 전송되어야 한다
  And 응답에 {"status": "ok", "quantity": 4}가 포함되어야 한다

Scenario: data_type 없이 write_registers (기존 동작 유지)
  Given MODBUS 클라이언트 에이전트가 실행 중이고 디바이스가 온라인일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_registers", "params": {"device_id": "dev1", "address": 0, "values": [100, 200, 300]}}
  Then FC16 프레임이 3개 uint16 레지스터로 전송되어야 한다
  And 기존 동작과 100% 동일해야 한다
```

### AC-M9-04: Client read_registers typed_values 응답

```gherkin
Scenario: read_registers 응답에 typed_values 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And 디바이스에서 FC03으로 주소 0-3의 레지스터를 읽었을 때
  When read_registers 응답이 구성되면
  Then 응답에 "values" 필드 (raw uint16 배열)가 포함되어야 한다
  And 응답에 "typed_values" 필드가 포함되어야 한다
  And typed_values에 주소 0의 {"value": <float32값>, "data_type": "float32"}가 포함되어야 한다

Scenario: TypeOverlay 미설정 시 typed_values 없음
  Given TypeOverlay가 설정되지 않았을 때
  When read_registers 응답이 구성되면
  Then 응답에 "typed_values" 필드가 없어야 한다
  And 기존 응답 형식과 동일해야 한다
```

### AC-M9-05: Client get_cache typed 값 포함

```gherkin
Scenario: get_cache 응답에 typed 값 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And HoldingRegisters에 float32(25.5) 값이 캐시되어 있을 때
  When Process()에 {"command": "get_cache", "params": {"device_id": "dev1"}}이 전달되면
  Then 응답에 "typed_holding_registers" 필드가 포함되어야 한다
  And typed_holding_registers에 주소 0의 float32(25.5) 값이 포함되어야 한다
```

### AC-M9-06: Client register_data 이벤트 typed_values 포함

```gherkin
Scenario: register_data 이벤트에 typed_values 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  When 디바이스에서 FC03 폴링 결과로 register_data 이벤트가 발생하면
  Then 이벤트 데이터에 "typed_values" 필드가 포함되어야 한다
  And typed_values에 주소 0의 float32 변환 값이 포함되어야 한다

Scenario: TypeOverlay 미설정 시 register_data 이벤트에 typed_values 없음
  Given TypeOverlay가 설정되지 않았을 때
  When register_data 이벤트가 발생하면
  Then 이벤트 데이터에 "typed_values" 필드가 없어야 한다
  And 기존 이벤트 형식과 동일해야 한다
```

### AC-M9-07: Client register_changed 이벤트 typed 변경 값 포함

```gherkin
Scenario: register_changed 이벤트에 typed_changed_values 포함
  Given TypeOverlay에 주소 0이 float32로 설정되어 있을 때
  And 이전 폴링에서 주소 0-1에 float32(25.5)가 캐시되어 있을 때
  When 새 폴링 결과에서 주소 0-1의 값이 float32(30.0)으로 변경되면
  Then register_changed 이벤트 데이터에 "typed_changed_values" 필드가 포함되어야 한다
  And typed_changed_values에 주소 0의 새 float32 값이 포함되어야 한다
```

### AC-M9-08: Client 자동 타입 변환

```gherkin
Scenario: Client 정수 값이 float32 타입으로 자동 변환
  Given MODBUS 클라이언트 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": 42, "data_type": "float32"}}
  Then 정수 42가 float32(42.0)으로 변환되어 전송되어야 한다
  And 에러가 발생하지 않아야 한다
```

### AC-M9-09: Client 에러 케이스

```gherkin
Scenario: Client 잘못된 data_type 에러
  Given MODBUS 클라이언트 에이전트가 실행 중일 때
  When Process()에 다음 JSON이 전달되면:
    {"command": "write_register", "params": {"device_id": "dev1", "address": 0, "value": 42, "data_type": "float64"}}
  Then 에러가 반환되어야 한다
  And 에러 메시지에 "unsupported" data_type 관련 내용이 포함되어야 한다
```

---

## 9. Client 통합 시나리오

### AC-CINT-01: Client End-to-End float32 시나리오

```gherkin
Scenario: Client float32 설정 -> 쓰기 -> 캐시 읽기 -> 이벤트
  Given 다음 설정으로 MODBUS 클라이언트를 생성할 때:
    devices:
      - id: "dev1"
        host: "127.0.0.1"
        port: 502
        register_groups:
          - name: "temp"
            function_code: 3
            start_address: 0
            quantity: 4
            data_type: "float32"
  And 디바이스가 온라인이고 TypeOverlay가 초기화된 후
  When write_register(address=0, value=25.5, data_type="float32")를 실행하면
  Then FC16 프레임이 2개 레지스터로 전송되어야 한다
  And 캐시 갱신 후 ReadTyped(FC03, 0, "float32", "big_endian")가 25.5를 반환해야 한다
  And get_cache 응답에 typed_holding_registers가 포함되어야 한다
```

### AC-CINT-02: Client 복합 type_map 시나리오

```gherkin
Scenario: Client 복합 type_map 통합 시나리오
  Given 다음 설정으로 MODBUS 클라이언트를 생성할 때:
    devices:
      - id: "dev1"
        host: "127.0.0.1"
        port: 502
        register_groups:
          - name: "mixed"
            function_code: 3
            start_address: 0
            quantity: 10
            type_map:
              - address: 0
                data_type: "float32"
              - address: 2
                data_type: "int32"
              - address: 4
                data_type: "uint32"
              - address: 6
                data_type: "int16"
  When 각 주소에 적절한 타입의 값을 write_register로 쓰면
  Then 각 값이 올바른 Function Code(FC06 또는 FC16)로 전송되어야 한다
  And get_cache 응답의 typed_holding_registers에 모든 타입 변환 값이 포함되어야 한다
```

### AC-CINT-03: Client Little-Endian 시나리오

```gherkin
Scenario: Client Little-Endian byte_order 설정 및 동작
  Given type_map에 address: 0, data_type: "float32", byte_order: "little_endian"이 설정되었을 때
  When write_register(address=0, value=3.14, data_type="float32")를 실행하면
  Then FC16 프레임에서 레지스터 워드 순서가 Little-Endian이어야 한다
  And ReadTyped로 읽을 때 3.14가 정확히 반환되어야 한다
```

### AC-CINT-04: Client 하위 호환성 통합 시나리오

```gherkin
Scenario: 기존 Client 설정으로 전체 동작 확인
  Given 기존 YAML 설정에 data_type와 type_map이 없을 때
  When 에이전트를 시작하면
  Then 에러 없이 시작되어야 한다
  And write_register(address=0, value=42)가 FC06으로 기존과 동일하게 동작해야 한다
  And write_registers(address=0, values=[1,2,3])가 FC16으로 기존과 동일하게 동작해야 한다
  And get_cache 응답에 typed_* 필드가 없어야 한다
  And register_data/register_changed 이벤트에 typed_* 필드가 없어야 한다
```

---

## 10. Quality Gate 기준

### 10.1 Definition of Done

- [ ] 모든 EARS 요구사항(REQ-M1 ~ REQ-M9)에 대한 테스트 존재
- [ ] `go test -race ./internal/modbus/...` 전체 통과
- [ ] `go test -race ./internal/agent/modbusserver/...` 전체 통과
- [ ] `go test -race ./internal/agent/modbus/...` 전체 통과
- [ ] `internal/modbus/` 패키지 커버리지 90% 이상
- [ ] `internal/agent/modbusserver/` 패키지 커버리지 85% 이상
- [ ] `internal/agent/modbus/` 패키지 커버리지 85% 이상
- [ ] 기존 테스트 전수 통과 (Server + Client 하위 호환성)
- [ ] `go vet ./internal/modbus/... ./internal/agent/modbusserver/... ./internal/agent/modbus/...` 경고 없음
- [ ] 새 에러 타입에 대한 에러 메시지가 명확하고 디버깅 가능
- [ ] Server: `request.go`, `handler.go`, `listener.go` 변경 없음 확인
- [ ] Client: `protocol.go`, `transport.go`, `device.go` 변경 없음 확인
- [ ] 공유 패키지(`internal/modbus/`)에 외부 의존성 없음 확인

### 10.2 테스트 검증 도구

| 도구 | 명령 | 목적 |
|------|------|------|
| Go Test (공유) | `go test -race -count=1 ./internal/modbus/...` | 공유 패키지 테스트 + 레이스 감지 |
| Go Test (Server) | `go test -race -count=1 ./internal/agent/modbusserver/...` | Server 테스트 + 레이스 감지 |
| Go Test (Client) | `go test -race -count=1 ./internal/agent/modbus/...` | Client 테스트 + 레이스 감지 |
| Coverage (공유) | `go test -coverprofile=coverage_modbus.out ./internal/modbus/...` | 공유 패키지 커버리지 |
| Coverage (Server) | `go test -coverprofile=coverage_server.out ./internal/agent/modbusserver/...` | Server 커버리지 |
| Coverage (Client) | `go test -coverprofile=coverage_client.out ./internal/agent/modbus/...` | Client 커버리지 |
| Go Vet | `go vet ./internal/modbus/... ./internal/agent/modbusserver/... ./internal/agent/modbus/...` | 정적 분석 |

### 10.3 검증 매트릭스

| 요구사항 | 인수 기준 | 테스트 파일 |
|----------|----------|------------|
| REQ-M1-01 ~ M1-06 | AC-M1-01 ~ AC-M1-06 | `modbusserver/config_test.go` |
| REQ-M2-01 ~ M2-06 | AC-M6-01 ~ AC-M6-08 | `modbus/types_test.go` |
| REQ-M3-01 ~ M3-05 | AC-M3-01 ~ AC-M3-05 | `modbusserver/register_map_test.go` |
| REQ-M4-01 ~ M4-07 | AC-M4-01 ~ AC-M4-08 | `modbusserver/agent_test.go` |
| REQ-M5-01 ~ M5-05 | AC-M1-01 ~ AC-M1-06 | `modbusserver/config_test.go` |
| REQ-M6-01 ~ M6-07 | AC-M6-01 ~ AC-M6-09 | `modbus/types_test.go` |
| REQ-M7-01 ~ M7-05 | AC-M7-01 ~ AC-M7-06 | `modbus/config_test.go` |
| REQ-M8-01 ~ M8-06 | AC-M8-01 ~ AC-M8-05 | `modbus/cache_test.go` |
| REQ-M9-01 ~ M9-10 | AC-M9-01 ~ AC-M9-09 | `modbus/agent_test.go`, `modbus/write_test.go` |
| Server 통합 | AC-INT-01 ~ AC-INT-03 | `modbusserver/agent_test.go` |
| Client 통합 | AC-CINT-01 ~ AC-CINT-04 | `modbus/agent_test.go` |
