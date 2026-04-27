// Package modbus는 MODBUS/TCP 프로토콜에서 사용하는 데이터 타입 변환 유틸리티를 제공한다.
// 서버와 클라이언트 에이전트 모두에서 공유되는 레지스터 ↔ 타입 변환 함수를 포함한다.
package modbus

import (
	"errors"
	"math"
)

// ---------------------------------------------------------------------------
// 데이터 타입 상수
// ---------------------------------------------------------------------------

const (
	DataTypeUint16  = "uint16"  // 1 레지스터, 기본값
	DataTypeInt16   = "int16"   // 1 레지스터
	DataTypeFloat32 = "float32" // 2 레지스터, IEEE 754
	DataTypeUint32  = "uint32"  // 2 레지스터
	DataTypeInt32   = "int32"   // 2 레지스터
)

// ---------------------------------------------------------------------------
// 바이트 순서 상수
// ---------------------------------------------------------------------------

const (
	ByteOrderBigEndian    = "big_endian"    // 기본값
	ByteOrderLittleEndian = "little_endian" // 워드 스왑
)

// ---------------------------------------------------------------------------
// 에러 변수
// ---------------------------------------------------------------------------

var (
	ErrUnsupportedDataType   = errors.New("modbus: unsupported data type")
	ErrInsufficientRegisters = errors.New("modbus: insufficient registers for data type")
	ErrInvalidValue          = errors.New("modbus: invalid value for data type")
)

// ---------------------------------------------------------------------------
// 구조체 정의
// ---------------------------------------------------------------------------

// TypeMapEntry는 설정에서 주소별 타입 오버라이드를 나타낸다.
type TypeMapEntry struct {
	Address   uint16
	DataType  string
	ByteOrder string // 기본값: big_endian
}

// TypeOverlayEntry는 오버레이에서 단일 주소의 타입 정보를 나타낸다.
type TypeOverlayEntry struct {
	DataType      string
	RegisterCount uint16 // 1 또는 2
	ByteOrder     string
}

// TypeOverlay 는 주소별 타입 오버레이 매핑이다.
// 키는 레지스터 주소(uint16)이다.
type TypeOverlay map[uint16]TypeOverlayEntry

// ---------------------------------------------------------------------------
// 헬퍼 함수
// ---------------------------------------------------------------------------

// RegisterCountForType은 주어진 데이터 타입에 필요한 레지스터 수를 반환한다.
// uint16/int16은 1, float32/uint32/int32는 2를 반환한다.
// 알 수 없는 타입이면 ErrUnsupportedDataType을 반환한다.
func RegisterCountForType(dataType string) (uint16, error) {
	switch dataType {
	case DataTypeUint16, DataTypeInt16:
		return 1, nil
	case DataTypeFloat32, DataTypeUint32, DataTypeInt32:
		return 2, nil
	default:
		return 0, ErrUnsupportedDataType
	}
}

// IsValidDataType은 지원되는 5가지 데이터 타입인지 확인한다.
func IsValidDataType(dataType string) bool {
	switch dataType {
	case DataTypeUint16, DataTypeInt16, DataTypeFloat32, DataTypeUint32, DataTypeInt32:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Float32 변환 함수
// ---------------------------------------------------------------------------

// Float32ToRegisters는 float32 값을 IEEE 754 형식으로 인코딩하여 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다. 리틀엔디안: 하위 워드가 먼저 온다.
func Float32ToRegisters(f float32, byteOrder string) [2]uint16 {
	bits := math.Float32bits(f)
	high := uint16(bits >> 16)
	low := uint16(bits & 0xFFFF)

	if byteOrder == ByteOrderLittleEndian {
		return [2]uint16{low, high}
	}
	return [2]uint16{high, low}
}

// RegistersToFloat32는 2개의 레지스터를 IEEE 754 형식으로 디코딩하여 float32 값을 반환한다.
func RegistersToFloat32(regs [2]uint16, byteOrder string) float32 {
	var bits uint32
	if byteOrder == ByteOrderLittleEndian {
		bits = uint32(regs[1])<<16 | uint32(regs[0])
	} else {
		bits = uint32(regs[0])<<16 | uint32(regs[1])
	}
	return math.Float32frombits(bits)
}

// ---------------------------------------------------------------------------
// Int32 변환 함수
// ---------------------------------------------------------------------------

// Int32ToRegisters는 int32 값을 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다.
func Int32ToRegisters(i int32, byteOrder string) [2]uint16 {
	u := uint32(i)
	high := uint16(u >> 16)
	low := uint16(u & 0xFFFF)

	if byteOrder == ByteOrderLittleEndian {
		return [2]uint16{low, high}
	}
	return [2]uint16{high, low}
}

// RegistersToInt32는 2개의 레지스터를 int32 값으로 변환한다.
func RegistersToInt32(regs [2]uint16, byteOrder string) int32 {
	var u uint32
	if byteOrder == ByteOrderLittleEndian {
		u = uint32(regs[1])<<16 | uint32(regs[0])
	} else {
		u = uint32(regs[0])<<16 | uint32(regs[1])
	}
	return int32(u)
}

// ---------------------------------------------------------------------------
// Uint32 변환 함수
// ---------------------------------------------------------------------------

// Uint32ToRegisters는 uint32 값을 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다.
func Uint32ToRegisters(u uint32, byteOrder string) [2]uint16 {
	high := uint16(u >> 16)
	low := uint16(u & 0xFFFF)

	if byteOrder == ByteOrderLittleEndian {
		return [2]uint16{low, high}
	}
	return [2]uint16{high, low}
}

// RegistersToUint32는 2개의 레지스터를 uint32 값으로 변환한다.
func RegistersToUint32(regs [2]uint16, byteOrder string) uint32 {
	if byteOrder == ByteOrderLittleEndian {
		return uint32(regs[1])<<16 | uint32(regs[0])
	}
	return uint32(regs[0])<<16 | uint32(regs[1])
}

// ---------------------------------------------------------------------------
// Int16 <-> Uint16 변환 함수
// ---------------------------------------------------------------------------

// Int16ToRegister는 int16 값을 비트 재해석으로 uint16으로 변환한다.
// 예: int16(-1) → uint16(0xFFFF)
func Int16ToRegister(i int16) uint16 {
	return uint16(i)
}

// RegisterToInt16는 uint16 값을 비트 재해석으로 int16으로 변환한다.
// 예: uint16(0x8000) → int16(-32768)
func RegisterToInt16(reg uint16) int16 {
	return int16(reg)
}

// ---------------------------------------------------------------------------
// 범용 변환 함수
// ---------------------------------------------------------------------------

// TypedValueToRegisters는 임의의 값을 지정된 데이터 타입에 맞는 레지스터 배열로 변환한다.
// float64, float32, int, int64, int32, int16, uint16 값을 받아들인다.
// 지원하지 않는 데이터 타입이면 ErrUnsupportedDataType을 반환한다.
// 값 타입이 맞지 않으면 ErrInvalidValue를 반환한다.
func TypedValueToRegisters(value any, dataType string, byteOrder string) ([]uint16, error) {
	switch dataType {
	case DataTypeUint16:
		v, err := toUint16Value(value)
		if err != nil {
			return nil, err
		}
		return []uint16{v}, nil

	case DataTypeInt16:
		v, err := toInt16Value(value)
		if err != nil {
			return nil, err
		}
		return []uint16{Int16ToRegister(v)}, nil

	case DataTypeFloat32:
		v, err := toFloat32Value(value)
		if err != nil {
			return nil, err
		}
		regs := Float32ToRegisters(v, byteOrder)
		return regs[:], nil

	case DataTypeInt32:
		v, err := toInt32Value(value)
		if err != nil {
			return nil, err
		}
		regs := Int32ToRegisters(v, byteOrder)
		return regs[:], nil

	case DataTypeUint32:
		v, err := toUint32Value(value)
		if err != nil {
			return nil, err
		}
		regs := Uint32ToRegisters(v, byteOrder)
		return regs[:], nil

	default:
		return nil, ErrUnsupportedDataType
	}
}

// RegistersToTypedValue는 레지스터 배열을 지정된 데이터 타입의 값으로 변환한다.
// 반환 타입은 float32, int32, uint32, int16, uint16 중 하나이다.
// 레지스터 수가 부족하면 ErrInsufficientRegisters를 반환한다.
// 지원하지 않는 데이터 타입이면 ErrUnsupportedDataType을 반환한다.
// nil 슬라이스에 대해 패닉하지 않는다.
func RegistersToTypedValue(regs []uint16, dataType string, byteOrder string) (any, error) {
	count, err := RegisterCountForType(dataType)
	if err != nil {
		return nil, err
	}
	if len(regs) < int(count) {
		return nil, ErrInsufficientRegisters
	}

	switch dataType {
	case DataTypeUint16:
		return regs[0], nil

	case DataTypeInt16:
		return RegisterToInt16(regs[0]), nil

	case DataTypeFloat32:
		return RegistersToFloat32([2]uint16{regs[0], regs[1]}, byteOrder), nil

	case DataTypeInt32:
		return RegistersToInt32([2]uint16{regs[0], regs[1]}, byteOrder), nil

	case DataTypeUint32:
		return RegistersToUint32([2]uint16{regs[0], regs[1]}, byteOrder), nil

	default:
		return nil, ErrUnsupportedDataType
	}
}

// ---------------------------------------------------------------------------
// 내부 값 변환 헬퍼
// ---------------------------------------------------------------------------

// toUint16Value는 다양한 숫자 타입을 uint16으로 변환한다.
// 축소 변환 시 [0, 65535] 범위를 검사하여 데이터 손실을 방지한다.
func toUint16Value(value any) (uint16, error) {
	switch v := value.(type) {
	case uint16:
		return v, nil
	case int:
		if v < 0 || v > math.MaxUint16 {
			return 0, ErrInvalidValue
		}
		return uint16(v), nil
	case int64:
		if v < 0 || v > math.MaxUint16 {
			return 0, ErrInvalidValue
		}
		return uint16(v), nil
	case int32:
		if v < 0 || v > math.MaxUint16 {
			return 0, ErrInvalidValue
		}
		return uint16(v), nil
	case uint32:
		if v > math.MaxUint16 {
			return 0, ErrInvalidValue
		}
		return uint16(v), nil
	case int16:
		return uint16(v), nil
	case float64:
		if v < 0 || v > math.MaxUint16 {
			return 0, ErrInvalidValue
		}
		return uint16(v), nil
	case float32:
		return uint16(v), nil
	default:
		return 0, ErrInvalidValue
	}
}

// toInt16Value는 다양한 숫자 타입을 int16으로 변환한다.
// 축소 변환 시 [-32768, 32767] 범위를 검사하여 데이터 손실을 방지한다.
func toInt16Value(value any) (int16, error) {
	switch v := value.(type) {
	case int16:
		return v, nil
	case int:
		if v < math.MinInt16 || v > math.MaxInt16 {
			return 0, ErrInvalidValue
		}
		return int16(v), nil
	case int64:
		if v < math.MinInt16 || v > math.MaxInt16 {
			return 0, ErrInvalidValue
		}
		return int16(v), nil
	case int32:
		if v < math.MinInt16 || v > math.MaxInt16 {
			return 0, ErrInvalidValue
		}
		return int16(v), nil
	case uint32:
		if v > math.MaxInt16 {
			return 0, ErrInvalidValue
		}
		return int16(v), nil
	case uint16:
		return int16(v), nil
	case float64:
		if v < math.MinInt16 || v > math.MaxInt16 {
			return 0, ErrInvalidValue
		}
		return int16(v), nil
	case float32:
		return int16(v), nil
	default:
		return 0, ErrInvalidValue
	}
}

// toFloat32Value는 다양한 숫자 타입을 float32로 변환한다.
// float64→float32 변환 시 정밀도 손실은 허용한다.
func toFloat32Value(value any) (float32, error) {
	switch v := value.(type) {
	case float32:
		return v, nil
	case float64:
		return float32(v), nil
	case int:
		return float32(v), nil
	case int64:
		return float32(v), nil
	case int32:
		return float32(v), nil
	case uint32:
		return float32(v), nil
	case int16:
		return float32(v), nil
	case uint16:
		return float32(v), nil
	default:
		return 0, ErrInvalidValue
	}
}

// toInt32Value는 다양한 숫자 타입을 int32로 변환한다.
// float64에서 축소 변환 시 [-2147483648, 2147483647] 범위를 검사한다.
func toInt32Value(value any) (int32, error) {
	switch v := value.(type) {
	case int32:
		return v, nil
	case int:
		return int32(v), nil
	case int64:
		return int32(v), nil
	case uint32:
		if v > math.MaxInt32 {
			return 0, ErrInvalidValue
		}
		return int32(v), nil
	case int16:
		return int32(v), nil
	case float64:
		if v < math.MinInt32 || v > math.MaxInt32 {
			return 0, ErrInvalidValue
		}
		return int32(v), nil
	case uint16:
		return int32(v), nil
	case float32:
		return int32(v), nil
	default:
		return 0, ErrInvalidValue
	}
}

// toUint32Value는 다양한 숫자 타입을 uint32로 변환한다.
// 축소 변환 시 [0, 4294967295] 범위를 검사하여 데이터 손실을 방지한다.
func toUint32Value(value any) (uint32, error) {
	switch v := value.(type) {
	case uint32:
		return v, nil
	case int:
		if v < 0 || v > math.MaxUint32 {
			return 0, ErrInvalidValue
		}
		return uint32(v), nil
	case int64:
		if v < 0 || v > math.MaxUint32 {
			return 0, ErrInvalidValue
		}
		return uint32(v), nil
	case int32:
		if v < 0 {
			return 0, ErrInvalidValue
		}
		return uint32(v), nil
	case int16:
		if v < 0 {
			return 0, ErrInvalidValue
		}
		return uint32(v), nil
	case float64:
		if v < 0 || v > math.MaxUint32 {
			return 0, ErrInvalidValue
		}
		return uint32(v), nil
	case uint16:
		return uint32(v), nil
	case float32:
		return uint32(v), nil
	default:
		return 0, ErrInvalidValue
	}
}
