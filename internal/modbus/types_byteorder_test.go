package modbus

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// M6 — 4순열 바이트순서 + raw 패스스루 (REQ-MODBUS-006-03)
// ---------------------------------------------------------------------------

// 기준 워드 페어: regs[0]=0xAABB (A=0xAA,B=0xBB), regs[1]=0xCCDD (C=0xCC,D=0xDD).
// 각 순열의 기대 uint32 조립 결과는 순열 정의로부터 직접 도출된다.
var byteOrderBaseRegs = [2]uint16{0xAABB, 0xCCDD}

// TestByteOrderPermutations_Uint32 는 4순열의 uint32 디코딩 결과를 고정값으로 검증한다.
func TestByteOrderPermutations_Uint32(t *testing.T) {
	tests := []struct {
		byteOrder string
		want      uint32
	}{
		{ByteOrderABCD, 0xAABBCCDD}, // 스왑 없음
		{ByteOrderBADC, 0xBBAADDCC}, // 각 워드 내 바이트 스왑
		{ByteOrderCDAB, 0xCCDDAABB}, // 워드 스왑
		{ByteOrderDCBA, 0xDDCCBBAA}, // 완전 역순
	}
	for _, tt := range tests {
		t.Run(tt.byteOrder, func(t *testing.T) {
			got := RegistersToUint32(byteOrderBaseRegs, tt.byteOrder)
			if got != tt.want {
				t.Errorf("RegistersToUint32(%#v, %q) = %#08x, want %#08x",
					byteOrderBaseRegs, tt.byteOrder, got, tt.want)
			}
			// 인코딩(분해)은 디코딩(조립)의 역변환이어야 한다 (라운드트립).
			regs := Uint32ToRegisters(tt.want, tt.byteOrder)
			if regs != byteOrderBaseRegs {
				t.Errorf("Uint32ToRegisters(%#08x, %q) = %#v, want %#v",
					tt.want, tt.byteOrder, regs, byteOrderBaseRegs)
			}
		})
	}
}

// TestAC05_ByteOrder_CDAB_Raw_AliasByteIdentical 는 AC-05 를 검증한다:
//   - CDAB 로 int32/float32/uint32 를 디코딩한 결과가 기대값과 일치한다.
//   - raw 는 변환 없이 원본 워드(uint16 배열)를 그대로 반환한다.
//   - 기존 big_endian/little_endian(word-swap)은 신규 ABCD/CDAB 순열과 바이트 단위로 동일하다.
func TestAC05_ByteOrder_CDAB_Raw_AliasByteIdentical(t *testing.T) {
	// (1) CDAB 디코딩: 알려진 워드 페어와 기대 타입 값.
	regs := [2]uint16{0x0001, 0x0002}
	// CDAB = regs[1]<<16 | regs[0] = 0x00020001 = 131073
	if got := RegistersToUint32(regs, ByteOrderCDAB); got != 0x00020001 {
		t.Errorf("uint32 CDAB = %#08x, want 0x00020001", got)
	}
	if got := RegistersToInt32(regs, ByteOrderCDAB); got != 0x00020001 {
		t.Errorf("int32 CDAB = %d, want %d", got, 0x00020001)
	}
	// float32 CDAB: bits = 0x00020001 → 대응 float32.
	wantF := math.Float32frombits(0x00020001)
	if got := RegistersToFloat32(regs, ByteOrderCDAB); got != wantF {
		t.Errorf("float32 CDAB = %v, want %v", got, wantF)
	}

	// (2) raw 패스스루: 변환 없이 원본 워드 배열을 그대로 반환한다.
	rawRegs := []uint16{0x1234, 0x5678, 0x9ABC}
	v, err := RegistersToTypedValue(rawRegs, DataTypeRaw, "")
	if err != nil {
		t.Fatalf("raw RegistersToTypedValue error: %v", err)
	}
	out, ok := v.([]uint16)
	if !ok {
		t.Fatalf("raw 반환 타입 = %T, want []uint16", v)
	}
	if len(out) != len(rawRegs) {
		t.Fatalf("raw 길이 = %d, want %d", len(out), len(rawRegs))
	}
	for i := range rawRegs {
		if out[i] != rawRegs[i] {
			t.Errorf("raw[%d] = %#04x, want %#04x", i, out[i], rawRegs[i])
		}
	}
	// raw 는 복사본이어야 한다: 원본 변경이 반환값에 영향을 주지 않는다.
	rawRegs[0] = 0xFFFF
	if out[0] == 0xFFFF {
		t.Error("raw 반환값이 원본 슬라이스를 별칭하고 있다(복사본이어야 함)")
	}

	// (3) 별칭 하위 호환: big_endian≡ABCD, little_endian≡CDAB (바이트 단위 동일).
	aliasCases := [][2]uint16{
		{0x0000, 0x0000}, {0xFFFF, 0xFFFF}, {0xAABB, 0xCCDD},
		{0x8000, 0x0001}, {0x1234, 0x5678}, {0x7FFF, 0xFFFF},
	}
	for _, r := range aliasCases {
		if RegistersToUint32(r, ByteOrderBigEndian) != RegistersToUint32(r, ByteOrderABCD) {
			t.Errorf("uint32 big_endian != ABCD for %#v", r)
		}
		if RegistersToUint32(r, ByteOrderLittleEndian) != RegistersToUint32(r, ByteOrderCDAB) {
			t.Errorf("uint32 little_endian != CDAB for %#v", r)
		}
		if RegistersToInt32(r, ByteOrderBigEndian) != RegistersToInt32(r, ByteOrderABCD) {
			t.Errorf("int32 big_endian != ABCD for %#v", r)
		}
		if RegistersToInt32(r, ByteOrderLittleEndian) != RegistersToInt32(r, ByteOrderCDAB) {
			t.Errorf("int32 little_endian != CDAB for %#v", r)
		}
		// float32 는 NaN 비교 회피를 위해 비트 단위로 비교한다.
		if math.Float32bits(RegistersToFloat32(r, ByteOrderBigEndian)) !=
			math.Float32bits(RegistersToFloat32(r, ByteOrderABCD)) {
			t.Errorf("float32 big_endian != ABCD for %#v", r)
		}
		if math.Float32bits(RegistersToFloat32(r, ByteOrderLittleEndian)) !=
			math.Float32bits(RegistersToFloat32(r, ByteOrderCDAB)) {
			t.Errorf("float32 little_endian != CDAB for %#v", r)
		}
		// 인코딩 방향 별칭도 바이트 단위로 동일해야 한다.
		if Uint32ToRegisters(0x11223344, ByteOrderBigEndian) != Uint32ToRegisters(0x11223344, ByteOrderABCD) {
			t.Error("Uint32ToRegisters big_endian != ABCD")
		}
		if Uint32ToRegisters(0x11223344, ByteOrderLittleEndian) != Uint32ToRegisters(0x11223344, ByteOrderCDAB) {
			t.Error("Uint32ToRegisters little_endian != CDAB")
		}
	}
}

// TestByteOrderRoundTrip_AllPermutations 는 모든 순열/별칭에서 분해→조립 라운드트립을 검증한다.
func TestByteOrderRoundTrip_AllPermutations(t *testing.T) {
	orders := []string{
		ByteOrderBigEndian, ByteOrderLittleEndian,
		ByteOrderABCD, ByteOrderBADC, ByteOrderCDAB, ByteOrderDCBA,
	}
	values := []uint32{0x00000000, 0xFFFFFFFF, 0x12345678, 0xDEADBEEF, 0x80000001}
	for _, bo := range orders {
		for _, v := range values {
			regs := Uint32ToRegisters(v, bo)
			if got := RegistersToUint32(regs, bo); got != v {
				t.Errorf("round-trip %q: got %#08x, want %#08x", bo, got, v)
			}
		}
	}
}

// TestIsValidByteOrder 는 별칭·4순열은 허용하고 알 수 없는 값은 거부함을 검증한다(REQ-03).
func TestIsValidByteOrder(t *testing.T) {
	valid := []string{
		ByteOrderBigEndian, ByteOrderLittleEndian,
		ByteOrderABCD, ByteOrderBADC, ByteOrderCDAB, ByteOrderDCBA,
	}
	for _, bo := range valid {
		if !IsValidByteOrder(bo) {
			t.Errorf("IsValidByteOrder(%q) = false, want true", bo)
		}
	}
	invalid := []string{"", "abcd", "middle_endian", "BEBE", "AB", "ABCDEF"}
	for _, bo := range invalid {
		if IsValidByteOrder(bo) {
			t.Errorf("IsValidByteOrder(%q) = true, want false", bo)
		}
	}
}

// TestIsValidDataType_Raw 는 raw 가 유효 타입으로 인정되는지 검증한다.
func TestIsValidDataType_Raw(t *testing.T) {
	if !IsValidDataType(DataTypeRaw) {
		t.Error("IsValidDataType(raw) = false, want true")
	}
	// raw 는 가변 길이이므로 RegisterCountForType 은 여전히 오류를 반환해야 한다.
	if _, err := RegisterCountForType(DataTypeRaw); err == nil {
		t.Error("RegisterCountForType(raw) = nil error, want ErrUnsupportedDataType")
	}
}

// TestRegistersToTypedValue_Raw_Empty 는 빈/nil 슬라이스에 대한 raw 동작을 검증한다.
func TestRegistersToTypedValue_Raw_Empty(t *testing.T) {
	v, err := RegistersToTypedValue(nil, DataTypeRaw, "")
	if err != nil {
		t.Fatalf("raw(nil) error: %v", err)
	}
	out, ok := v.([]uint16)
	if !ok || len(out) != 0 {
		t.Errorf("raw(nil) = %#v, want empty []uint16", v)
	}
}
