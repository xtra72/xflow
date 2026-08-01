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
	DataTypeRaw     = "raw"     // 변환 없이 읽은 워드(uint16 배열)를 그대로 전달 (A-9, REQ-03)
)

// ---------------------------------------------------------------------------
// 바이트 순서 상수
// ---------------------------------------------------------------------------

const (
	ByteOrderBigEndian    = "big_endian"    // 기본값 — ABCD 별칭 (스왑 없음, 하위 호환)
	ByteOrderLittleEndian = "little_endian" // 워드 스왑 — CDAB 별칭 (하위 호환)

	// 완전한 4순열 바이트순서 (2워드/32비트 값 대상, REQ-03).
	// 워드를 [A B][C D] (A=상위워드 상위바이트 … D=하위워드 하위바이트)로 보면:
	ByteOrderABCD = "ABCD" // 스왑 없음(빅엔디안). big_endian 별칭과 바이트 단위로 동일 결과
	ByteOrderBADC = "BADC" // 각 워드 내 바이트 스왑, 워드 순서 유지
	ByteOrderCDAB = "CDAB" // 워드 스왑, 워드 내 바이트 유지. little_endian 별칭과 동일 결과
	ByteOrderDCBA = "DCBA" // 워드 스왑 + 바이트 스왑(완전 역순)
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

// IsValidDataType은 지원되는 데이터 타입인지 확인한다.
// 5가지 스칼라 타입에 더해 raw(변환 없는 워드 패스스루)를 포함한다(REQ-03).
func IsValidDataType(dataType string) bool {
	switch dataType {
	case DataTypeUint16, DataTypeInt16, DataTypeFloat32, DataTypeUint32, DataTypeInt32, DataTypeRaw:
		return true
	default:
		return false
	}
}

// IsValidByteOrder는 지원되는 바이트순서 지정인지 확인한다(REQ-03).
// 하위 호환 별칭(big_endian/little_endian)과 4순열(ABCD/BADC/CDAB/DCBA)을 허용한다.
// 알 수 없는 지정은 설정 오류로 거부되어야 하므로 false 를 반환한다.
func IsValidByteOrder(byteOrder string) bool {
	switch byteOrder {
	case ByteOrderBigEndian, ByteOrderLittleEndian,
		ByteOrderABCD, ByteOrderBADC, ByteOrderCDAB, ByteOrderDCBA:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// 4순열 바이트순서 조립/분해 헬퍼 (2워드 32비트 값)
// ---------------------------------------------------------------------------

// swapBytes는 16비트 워드의 상·하위 바이트를 교환한다.
func swapBytes(w uint16) uint16 {
	return w<<8 | w>>8
}

// assemble32는 2개의 레지스터를 지정된 바이트순서에 따라 uint32 로 조립한다.
//   - big_endian/ABCD(기본): regs[0]<<16 | regs[1]  (스왑 없음)
//   - little_endian/CDAB: regs[1]<<16 | regs[0]      (워드 스왑)
//   - BADC: 각 워드 내 바이트 스왑
//   - DCBA: 워드 스왑 + 바이트 스왑(완전 역순)
//
// 알 수 없는 값은 하위 호환을 위해 ABCD(빅엔디안)로 처리한다(기존 else 분기 의미 보존).
func assemble32(regs [2]uint16, byteOrder string) uint32 {
	switch byteOrder {
	case ByteOrderLittleEndian, ByteOrderCDAB:
		return uint32(regs[1])<<16 | uint32(regs[0])
	case ByteOrderBADC:
		return uint32(swapBytes(regs[0]))<<16 | uint32(swapBytes(regs[1]))
	case ByteOrderDCBA:
		return uint32(swapBytes(regs[1]))<<16 | uint32(swapBytes(regs[0]))
	default: // ByteOrderBigEndian, ByteOrderABCD, "" 등 → ABCD
		return uint32(regs[0])<<16 | uint32(regs[1])
	}
}

// disassemble32는 uint32 를 지정된 바이트순서에 따라 2개의 레지스터로 분해한다.
// assemble32 의 역변환이며 인코딩(쓰기) 경로에서 사용된다.
func disassemble32(bits uint32, byteOrder string) [2]uint16 {
	hi := uint16(bits >> 16)
	lo := uint16(bits & 0xFFFF)
	switch byteOrder {
	case ByteOrderLittleEndian, ByteOrderCDAB:
		return [2]uint16{lo, hi}
	case ByteOrderBADC:
		return [2]uint16{swapBytes(hi), swapBytes(lo)}
	case ByteOrderDCBA:
		return [2]uint16{swapBytes(lo), swapBytes(hi)}
	default: // ByteOrderBigEndian, ByteOrderABCD, "" 등 → ABCD
		return [2]uint16{hi, lo}
	}
}

// ---------------------------------------------------------------------------
// Float32 변환 함수
// ---------------------------------------------------------------------------

// Float32ToRegisters는 float32 값을 IEEE 754 형식으로 인코딩하여 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다. 리틀엔디안: 하위 워드가 먼저 온다.
func Float32ToRegisters(f float32, byteOrder string) [2]uint16 {
	return disassemble32(math.Float32bits(f), byteOrder)
}

// RegistersToFloat32는 2개의 레지스터를 IEEE 754 형식으로 디코딩하여 float32 값을 반환한다.
func RegistersToFloat32(regs [2]uint16, byteOrder string) float32 {
	return math.Float32frombits(assemble32(regs, byteOrder))
}

// ---------------------------------------------------------------------------
// Int32 변환 함수
// ---------------------------------------------------------------------------

// Int32ToRegisters는 int32 값을 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다.
func Int32ToRegisters(i int32, byteOrder string) [2]uint16 {
	return disassemble32(uint32(i), byteOrder)
}

// RegistersToInt32는 2개의 레지스터를 int32 값으로 변환한다.
func RegistersToInt32(regs [2]uint16, byteOrder string) int32 {
	return int32(assemble32(regs, byteOrder))
}

// ---------------------------------------------------------------------------
// Uint32 변환 함수
// ---------------------------------------------------------------------------

// Uint32ToRegisters는 uint32 값을 2개의 레지스터로 변환한다.
// 빅엔디안: 상위 워드가 먼저 온다.
func Uint32ToRegisters(u uint32, byteOrder string) [2]uint16 {
	return disassemble32(u, byteOrder)
}

// RegistersToUint32는 2개의 레지스터를 uint32 값으로 변환한다.
func RegistersToUint32(regs [2]uint16, byteOrder string) uint32 {
	return assemble32(regs, byteOrder)
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
	// raw: 변환 없이 읽은 워드를 그대로(복사본) 반환한다 (A-9, REQ-03).
	// 호출자 슬라이스를 별칭하지 않도록 복사본을 반환한다.
	if dataType == DataTypeRaw {
		out := make([]uint16, len(regs))
		copy(out, regs)
		return out, nil
	}

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
		// 32비트 플랫폼(linux/arm GOARM=6/7 등)에서 int 는 int32 와 동일한 크기이므로
		// math.MaxUint32 (untyped int constant) 와 직접 비교 시 컴파일 에러가 발생한다.
		// int64 로 승격하여 비교하면 32/64비트 양쪽 모두 안전하게 동작한다.
		if v < 0 || int64(v) > math.MaxUint32 {
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
