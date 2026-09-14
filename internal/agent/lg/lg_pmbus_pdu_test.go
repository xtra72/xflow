package lg

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	imodbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// 문서 §5 완성 프레임 골든 테스트 (AC-013 ~ AC-016)
//
// 이 테스트가 통과하면 CRC 다항식·초기값·엔디안, 주소 0-base 체계, 온도 ×10 스케일이
// 한 번에 고정된다. 기대값은 SPEC 작성 시점에 CRC-16/MODBUS 로 독립 재계산하여
// 13/13 일치를 확인한 값이다.
// ---------------------------------------------------------------------------

// mustHex 는 공백을 포함한 hex 문자열을 바이트로 변환한다.
func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("invalid hex %q: %v", s, err)
	}
	return b
}

// pmbusGoldenSlaveID 는 문서 §5 예제가 사용하는 게이트웨이 슬레이브 주소이다.
const pmbusGoldenSlaveID byte = 1

func TestPmbusGoldenFrames_Write(t *testing.T) {
	const scale = 10

	tests := []struct {
		name string
		pdu  func(t *testing.T) []byte
		want string
	}{
		{
			name: "IDU 0 power ON (FC05 coil 0)",
			pdu: func(*testing.T) []byte {
				return buildWriteCoilPDU(pmbusCoilAddr(0, pmbusCoilPower), true)
			},
			want: "01 05 00 00 FF 00 8C 3A",
		},
		{
			name: "IDU 0 power OFF (FC05 coil 0)",
			pdu: func(*testing.T) []byte {
				return buildWriteCoilPDU(pmbusCoilAddr(0, pmbusCoilPower), false)
			},
			want: "01 05 00 00 00 00 CD CA",
		},
		{
			name: "IDU 3 power ON (FC05 coil 48)",
			pdu: func(*testing.T) []byte {
				return buildWriteCoilPDU(pmbusCoilAddr(3, pmbusCoilPower), true)
			},
			want: "01 05 00 30 FF 00 8C 35",
		},
		{
			name: "IDU 0 heat mode (FC06 hold 0 = 4)",
			pdu: func(*testing.T) []byte {
				return buildWriteRegisterPDU(pmbusHoldingAddr(0, pmbusHoldingMode), pmbusModeHeat)
			},
			want: "01 06 00 00 00 04 88 09",
		},
		{
			name: "IDU 0 fan high (FC06 hold 1 = 3)",
			pdu: func(*testing.T) []byte {
				return buildWriteRegisterPDU(pmbusHoldingAddr(0, pmbusHoldingFanSpeed), pmbusFanHigh)
			},
			want: "01 06 00 01 00 03 98 0B",
		},
		{
			name: "IDU 0 set temp 24.0 (FC06 hold 2 = 240)",
			pdu: func(*testing.T) []byte {
				return buildWriteRegisterPDU(pmbusHoldingAddr(0, pmbusHoldingSetTemp), pmbusEncodeTemp(24.0, scale))
			},
			want: "01 06 00 02 00 F0 28 4E",
		},
		{
			name: "IDU 3 set temp 26.0 (FC06 hold 62 = 260)",
			pdu: func(*testing.T) []byte {
				return buildWriteRegisterPDU(pmbusHoldingAddr(3, pmbusHoldingSetTemp), pmbusEncodeTemp(26.0, scale))
			},
			want: "01 06 00 3E 01 04 E8 55",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adu := imodbus.BuildRTUADU(pmbusGoldenSlaveID, tc.pdu(t))
			want := mustHex(t, tc.want)
			if !bytesEqual(adu, want) {
				t.Errorf("frame mismatch\n got: % X\nwant: % X", adu, want)
			}
		})
	}
}

func TestPmbusGoldenFrames_WriteMultiple(t *testing.T) {
	// IDU 0 모드+풍량+온도 일괄 (FC16, hold 0~2): 난방(4), 강(3), 24.0 °C(240).
	values := []uint16{pmbusModeHeat, pmbusFanHigh, pmbusEncodeTemp(24.0, 10)}
	pdu, err := buildWriteMultiplePDU(pmbusHoldingAddr(0, pmbusHoldingMode), values)
	if err != nil {
		t.Fatalf("buildWriteMultiplePDU: %v", err)
	}
	adu := imodbus.BuildRTUADU(pmbusGoldenSlaveID, pdu)
	want := mustHex(t, "01 10 00 00 00 03 06 00 04 00 03 00 F0 E7 04")
	if !bytesEqual(adu, want) {
		t.Errorf("FC16 frame mismatch\n got: % X\nwant: % X", adu, want)
	}
}

func TestPmbusGoldenFrames_Read(t *testing.T) {
	tests := []struct {
		name     string
		fc       byte
		addr     uint16
		quantity uint16
		want     string
	}{
		{
			name: "IDU 0 holding read (FC03 0~5)",
			fc:   pmbusFCReadHolding, addr: pmbusHoldingAddr(0, 0), quantity: pmbusHoldingCount,
			want: "01 03 00 00 00 06 C5 C8",
		},
		{
			name: "IDU 0 input read (FC04 0~5)",
			fc:   pmbusFCReadInput, addr: pmbusInputAddr(0, 0), quantity: pmbusInputCount,
			want: "01 04 00 00 00 06 70 08",
		},
		{
			name: "IDU 0 discrete read (FC02 0~4)",
			fc:   pmbusFCReadDiscreteInputs, addr: pmbusDiscreteAddr(0, 0), quantity: pmbusDiscreteCount,
			want: "01 02 00 00 00 05 B8 09",
		},
		{
			name: "IDU 0 coil read (FC01 0~9)",
			fc:   pmbusFCReadCoils, addr: pmbusCoilAddr(0, 0), quantity: pmbusCoilCount,
			want: "01 01 00 00 00 0A BC 0D",
		},
		{
			name: "full scan (FC02 0~255)",
			fc:   pmbusFCReadDiscreteInputs, addr: 0, quantity: pmbusScanBitCount,
			want: "01 02 00 00 01 00 79 9A",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pdu, err := buildReadPDU(tc.fc, tc.addr, tc.quantity)
			if err != nil {
				t.Fatalf("buildReadPDU: %v", err)
			}
			adu := imodbus.BuildRTUADU(pmbusGoldenSlaveID, pdu)
			want := mustHex(t, tc.want)
			if !bytesEqual(adu, want) {
				t.Errorf("frame mismatch\n got: % X\nwant: % X", adu, want)
			}
		})
	}
}

// TestPmbusGoldenFrames_CRCIsModbusNotXmodem 은 Modbus 구간이 CRC-16/MODBUS 를
// 쓰는지 확인한다. LGCP 계열의 CRC-16/XMODEM 과 혼용하면 모든 프레임이 거부된다.
func TestPmbusGoldenFrames_CRCIsModbusNotXmodem(t *testing.T) {
	pdu := buildWriteCoilPDU(pmbusCoilAddr(0, pmbusCoilPower), true)
	adu := imodbus.BuildRTUADU(pmbusGoldenSlaveID, pdu)

	// 문서 §5 의 CRC 는 8C 3A (Little-Endian 전송: lo=0x8C, hi=0x3A).
	gotCRC := adu[len(adu)-2:]
	if gotCRC[0] != 0x8C || gotCRC[1] != 0x3A {
		t.Fatalf("CRC mismatch: got % X, want 8C 3A", gotCRC)
	}

	// XMODEM(0x1021, init 0) 이었다면 다른 값이 나온다. 두 체계가 우연히 같지
	// 않음을 명시적으로 확인한다.
	body := adu[:len(adu)-2]
	modbusCRC := imodbus.CRC16(body)
	if modbusCRC != 0x3A8C {
		t.Fatalf("CRC16 returned 0x%04X, want 0x3A8C", modbusCRC)
	}
}

// ---------------------------------------------------------------------------
// 응답 파싱 (AC-017 ~ AC-019)
// ---------------------------------------------------------------------------

// TestPmbusParseInputResponse_DocumentExample 은 문서 §5.1 의 응답 예를 검증한다.
func TestPmbusParseInputResponse_DocumentExample(t *testing.T) {
	pdu := mustHex(t, "04 0C 0000 00F5 00D2 00E8 0000 0000")
	regs, err := parseRegisterResponse(pdu, pmbusFCReadInput, pmbusInputCount)
	if err != nil {
		t.Fatalf("parseRegisterResponse: %v", err)
	}

	const scale = 10
	if got := regs[pmbusInputErrorCode]; got != 0 {
		t.Errorf("error code = %d, want 0", got)
	}
	if got := pmbusDecodeTemp(regs[pmbusInputRoomTemp], scale); got != 24.5 {
		t.Errorf("room temp = %.1f, want 24.5", got)
	}
	if got := pmbusDecodeTemp(regs[pmbusInputPipeIn], scale); got != 21.0 {
		t.Errorf("pipe in = %.1f, want 21.0", got)
	}
	if got := pmbusDecodeTemp(regs[pmbusInputPipeOut], scale); got != 23.2 {
		t.Errorf("pipe out = %.1f, want 23.2", got)
	}
}

func TestPmbusParseBitResponse(t *testing.T) {
	// FC02, 5비트: 연결=1, 알람=0, 필터=1, 기준=0, 에러구분=0 → 0b00000101 = 0x05
	pdu := mustHex(t, "02 01 05")
	bits, err := parseBitResponse(pdu, pmbusFCReadDiscreteInputs, pmbusDiscreteCount)
	if err != nil {
		t.Fatalf("parseBitResponse: %v", err)
	}
	want := []bool{true, false, true, false, false}
	for i := range want {
		if bits[i] != want[i] {
			t.Errorf("bit[%d] = %v, want %v", i, bits[i], want[i])
		}
	}
}

// TestPmbusParseBitResponse_ScanBitOrder 는 전체 스캔에서 특정 N 의 비트가
// 올바른 인덱스에 놓이는지 확인한다.
func TestPmbusParseBitResponse_ScanBitOrder(t *testing.T) {
	// 32비트(N=0,1) 분량. N=1 의 연결 비트(인덱스 16)만 1로 세운다.
	data := make([]byte, 4)
	data[2] = 0x01 // byte 2 의 bit0 = 비트 인덱스 16
	pdu := append([]byte{pmbusFCReadDiscreteInputs, 0x04}, data...)

	bits, err := parseBitResponse(pdu, pmbusFCReadDiscreteInputs, 32)
	if err != nil {
		t.Fatalf("parseBitResponse: %v", err)
	}
	if bits[pmbusScanBitIndex(0, pmbusDiscreteConnected)] {
		t.Error("N=0 connected should be false")
	}
	if !bits[pmbusScanBitIndex(1, pmbusDiscreteConnected)] {
		t.Error("N=1 connected should be true")
	}
}

func TestPmbusModbusException(t *testing.T) {
	// FC03 요청에 대한 ILLEGAL_DATA_ADDRESS 예외 응답.
	pdu := mustHex(t, "83 02")
	_, err := parseRegisterResponse(pdu, pmbusFCReadHolding, 6)
	if err == nil {
		t.Fatal("expected exception error, got nil")
	}
	var me *ModbusException
	if !errors.As(err, &me) {
		t.Fatalf("expected *ModbusException, got %T: %v", err, err)
	}
	if me.Code != 0x02 {
		t.Errorf("exception code = 0x%02X, want 0x02", me.Code)
	}
	if !strings.Contains(err.Error(), "ILLEGAL_DATA_ADDRESS") {
		t.Errorf("error message should name the exception: %v", err)
	}
}

func TestPmbusParseResponse_ShortPDU(t *testing.T) {
	// byte count 는 12 를 선언했지만 실제 데이터는 4바이트뿐.
	pdu := mustHex(t, "04 0C 0000 00F5")
	if _, err := parseRegisterResponse(pdu, pmbusFCReadInput, 6); err == nil {
		t.Fatal("expected short-response error, got nil")
	}
}

func TestPmbusParseResponse_UnexpectedFC(t *testing.T) {
	pdu := mustHex(t, "03 02 0000")
	_, err := parseRegisterResponse(pdu, pmbusFCReadInput, 1)
	if !errors.Is(err, ErrHvacr03UnexpectedFunctionCode) {
		t.Fatalf("expected ErrHvacr03UnexpectedFunctionCode, got %v", err)
	}
}

func TestPmbusWriteEcho(t *testing.T) {
	addr := pmbusCoilAddr(0, pmbusCoilPower)
	// 정상 에코.
	if err := parseWriteEchoResponse(mustHex(t, "05 0000 FF00"), pmbusFCWriteSingleCoil, addr, pmbusCoilOn); err != nil {
		t.Errorf("valid echo rejected: %v", err)
	}
	// 값 불일치.
	if err := parseWriteEchoResponse(mustHex(t, "05 0000 0000"), pmbusFCWriteSingleCoil, addr, pmbusCoilOn); err == nil {
		t.Error("mismatched echo should be rejected")
	}
}

func TestPmbusWriteMultipleEcho(t *testing.T) {
	addr := pmbusHoldingAddr(0, pmbusHoldingMode)
	if err := parseWriteMultipleResponse(mustHex(t, "10 0000 0003"), addr, 3); err != nil {
		t.Errorf("valid FC16 echo rejected: %v", err)
	}
	if err := parseWriteMultipleResponse(mustHex(t, "10 0000 0002"), addr, 3); err == nil {
		t.Error("quantity mismatch should be rejected")
	}
}

func TestBuildReadPDU_InvalidQuantity(t *testing.T) {
	if _, err := buildReadPDU(pmbusFCReadHolding, 0, 0); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("quantity 0 should be rejected, got %v", err)
	}
	if _, err := buildReadPDU(pmbusFCReadHolding, 0, 126); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("quantity 126 should be rejected for registers, got %v", err)
	}
	if _, err := buildReadPDU(0x07, 0, 1); !errors.Is(err, ErrHvacr03UnexpectedFunctionCode) {
		t.Errorf("unsupported fc should be rejected, got %v", err)
	}
}

func TestBuildWriteMultiplePDU_InvalidCount(t *testing.T) {
	if _, err := buildWriteMultiplePDU(0, nil); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("empty values should be rejected, got %v", err)
	}
	if _, err := buildWriteMultiplePDU(0, make([]uint16, 124)); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("124 registers should be rejected, got %v", err)
	}
}

// bytesEqual 은 두 바이트 슬라이스의 동일성을 확인한다.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
