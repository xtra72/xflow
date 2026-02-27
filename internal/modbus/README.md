# internal/modbus - MODBUS 데이터 타입 변환 유틸리티

## 개요

`internal/modbus`는 MODBUS/TCP 프로토콜에서 사용하는 데이터 타입 변환 유틸리티를 제공하는 공유 패키지이다. 서버 에이전트(`internal/agent/modbusserver`)와 클라이언트 에이전트(`internal/agent/modbus`) 모두에서 공통으로 사용하는 레지스터-타입 변환 함수, 타입 상수, 구조체를 포함한다.

## 주요 상수

### DataType 상수

| 상수 | 값 | 레지스터 수 | 설명 |
|------|------|------------|------|
| `DataTypeUint16` | `"uint16"` | 1 | 부호 없는 16비트 정수 (기본값) |
| `DataTypeInt16` | `"int16"` | 1 | 부호 있는 16비트 정수 |
| `DataTypeFloat32` | `"float32"` | 2 | IEEE 754 32비트 부동소수점 |
| `DataTypeUint32` | `"uint32"` | 2 | 부호 없는 32비트 정수 |
| `DataTypeInt32` | `"int32"` | 2 | 부호 있는 32비트 정수 |

### ByteOrder 상수

| 상수 | 값 | 설명 |
|------|------|------|
| `ByteOrderBigEndian` | `"big_endian"` | 상위 워드 우선 (기본값) |
| `ByteOrderLittleEndian` | `"little_endian"` | 하위 워드 우선 (워드 스왑) |

## 주요 타입

### TypeMapEntry

설정에서 주소별 타입 오버라이드를 나타내는 구조체이다.

```go
type TypeMapEntry struct {
    Address   uint16
    DataType  string
    ByteOrder string // 기본값: big_endian
}
```

### TypeOverlayEntry

오버레이에서 단일 주소의 타입 정보를 나타내는 구조체이다.

```go
type TypeOverlayEntry struct {
    DataType      string
    RegisterCount uint16 // 1 또는 2
    ByteOrder     string
}
```

### TypeOverlay

주소별 타입 오버레이 매핑 타입이다. 키는 레지스터 주소(`uint16`)이다.

```go
type TypeOverlay map[uint16]TypeOverlayEntry
```

## 주요 함수

### 헬퍼 함수

| 함수 | 시그니처 | 설명 |
|------|---------|------|
| `RegisterCountForType` | `(dataType string) (uint16, error)` | 데이터 타입에 필요한 레지스터 수 반환 |
| `IsValidDataType` | `(dataType string) bool` | 지원되는 5가지 데이터 타입인지 검증 |

### 타입별 변환 함수

| 함수 | 설명 |
|------|------|
| `Float32ToRegisters` / `RegistersToFloat32` | IEEE 754 float32 <-> 2개 레지스터 변환 |
| `Int32ToRegisters` / `RegistersToInt32` | 부호 있는 int32 <-> 2개 레지스터 변환 |
| `Uint32ToRegisters` / `RegistersToUint32` | 부호 없는 uint32 <-> 2개 레지스터 변환 |
| `Int16ToRegister` / `RegisterToInt16` | int16 <-> uint16 비트 재해석 변환 |

### 범용 변환 함수

| 함수 | 시그니처 | 설명 |
|------|---------|------|
| `TypedValueToRegisters` | `(value any, dataType, byteOrder string) ([]uint16, error)` | 임의의 값을 지정 타입의 레지스터 배열로 변환 |
| `RegistersToTypedValue` | `(regs []uint16, dataType, byteOrder string) (any, error)` | 레지스터 배열을 지정 타입의 값으로 변환 |

`TypedValueToRegisters`는 `float64`, `float32`, `int`, `int64`, `int32`, `int16`, `uint16` 입력을 받아들이며, 축소 변환 시 범위를 검사하여 데이터 손실을 방지한다.

## 에러 타입

| 에러 변수 | 설명 |
|-----------|------|
| `ErrUnsupportedDataType` | 지원하지 않는 데이터 타입 |
| `ErrInsufficientRegisters` | 데이터 타입에 필요한 레지스터 수 부족 |
| `ErrInvalidValue` | 데이터 타입에 맞지 않는 값 (범위 초과 등) |

모든 에러는 `errors.Is()`와 호환된다.

## 사용 예시

```go
import modbus "github.com/xtra/xflow/internal/modbus"

// float32 -> 레지스터 변환
regs := modbus.Float32ToRegisters(3.14, modbus.ByteOrderBigEndian)
// regs = [2]uint16{0x4048, 0xF5C3}

// 레지스터 -> float32 복원
val := modbus.RegistersToFloat32(regs, modbus.ByteOrderBigEndian)
// val = 3.14

// 범용 변환: int32 값을 레지스터로 변환
regs2, err := modbus.TypedValueToRegisters(int32(-100000), modbus.DataTypeInt32, modbus.ByteOrderBigEndian)

// 범용 변환: 레지스터를 타입 값으로 복원
val2, err := modbus.RegistersToTypedValue(regs2, modbus.DataTypeInt32, modbus.ByteOrderBigEndian)
// val2 = int32(-100000)
```

## 파일 구성

| 파일 | 설명 |
|------|------|
| `types.go` | 데이터 타입 상수, 바이트 순서 상수, 에러 변수, 구조체, 변환 함수 전체 |
| `types_test.go` | 모든 변환 함수의 테이블 기반 테스트, 범위 검증 테스트, 엣지 케이스 테스트 |

## 테스트

```bash
go test -v -race -cover ./internal/modbus/...
```

- 커버리지: 99.4%
- Race Detector: 이상 없음

## 의존성

표준 라이브러리만 사용한다 (외부 의존성 없음).

- `errors`: 에러 생성
- `math`: IEEE 754 변환, 범위 상수

## SPEC 문서

- SPEC ID: SPEC-MODBUS-003
- 상태: 구현 완료
