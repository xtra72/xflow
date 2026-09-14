package lg

import (
	"errors"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 프로토콜 층 경계 조건 (AC-081 — 프로토콜 층 100% 목표)
// ---------------------------------------------------------------------------

// TestPmbusTempScaleGuard 는 잘못된 스케일이 0 나눗셈을 일으키지 않는지 확인한다.
// 설정 파싱이 양수를 강제하지만, 순수 함수는 자체 방어를 갖는다.
func TestPmbusTempScaleGuard(t *testing.T) {
	for _, scale := range []int{0, -1, -10} {
		if got := pmbusDecodeTemp(240, scale); got != 240 {
			t.Errorf("decodeTemp with scale %d = %v, want 240 (scale 1 fallback)", scale, got)
		}
		if got := pmbusEncodeTemp(240, scale); got != 240 {
			t.Errorf("encodeTemp with scale %d = %v, want 240 (scale 1 fallback)", scale, got)
		}
	}
}

// TestPmbusEncodeTempRounding 은 부동소수 오차가 값을 한 눈금 떨어뜨리지 않는지
// 확인한다. 24.0×10 이 239 로 내려앉으면 설정 온도가 23.9 로 기록된다.
func TestPmbusEncodeTempRounding(t *testing.T) {
	cases := []struct {
		tempC float64
		want  uint16
	}{
		{24.0, 240},
		{24.1, 241},
		{24.9, 249},
		{16.7, 167},
		{29.3, 293},
	}
	for _, c := range cases {
		if got := pmbusEncodeTemp(c.tempC, 10); got != c.want {
			t.Errorf("encodeTemp(%v) = %d, want %d", c.tempC, got, c.want)
		}
	}
}

// TestPmbusFanFromNameAutoCodeUnset 은 autoCode 가 설정되지 않은 경우를 확인한다.
func TestPmbusFanFromNameAutoCodeUnset(t *testing.T) {
	if _, err := pmbusFanFromName("auto", 0); !errors.Is(err, ErrHvacr03InvalidFanSpeed) {
		t.Errorf("auto with no configured code should be rejected, got %v", err)
	}
	if _, err := pmbusFanFromName("auto", -1); !errors.Is(err, ErrHvacr03InvalidFanSpeed) {
		t.Errorf("auto with a negative code should be rejected, got %v", err)
	}
}

// TestBuildReadPDU_BitQuantityLimits 는 비트 읽기 개수 한계를 확인한다.
func TestBuildReadPDU_BitQuantityLimits(t *testing.T) {
	// 비트 읽기는 최대 2000 까지 허용된다 — 전체 스캔(256)은 여유롭게 통과.
	if _, err := buildReadPDU(pmbusFCReadCoils, 0, pmbusMaxReadBits); err != nil {
		t.Errorf("2000 bits should be allowed: %v", err)
	}
	if _, err := buildReadPDU(pmbusFCReadCoils, 0, pmbusMaxReadBits+1); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("2001 bits should be rejected, got %v", err)
	}
	if _, err := buildReadPDU(pmbusFCReadDiscreteInputs, 0, 0); !errors.Is(err, ErrHvacr03InvalidQuantity) {
		t.Errorf("zero bits should be rejected, got %v", err)
	}
	// 레지스터 읽기 상한.
	if _, err := buildReadPDU(pmbusFCReadInput, 0, pmbusMaxReadRegs); err != nil {
		t.Errorf("125 registers should be allowed: %v", err)
	}
}

// TestCheckResponseFC_ShortPDU 는 2바이트 미만 응답을 거부하는지 확인한다.
func TestCheckResponseFC_ShortPDU(t *testing.T) {
	for _, pdu := range [][]byte{{}, {0x03}} {
		if err := checkResponseFC(pdu, pmbusFCReadHolding); !errors.Is(err, ErrHvacr03ShortResponse) {
			t.Errorf("pdu %X should be rejected as short, got %v", pdu, err)
		}
	}
}

// TestParseBitResponse_DeclaredCountShort 는 byte count 선언과 실제 길이가 다른
// 응답을 거부하는지 확인한다.
func TestParseBitResponse_DeclaredCountShort(t *testing.T) {
	// byte count 4 를 선언했지만 실제로는 1바이트뿐.
	pdu := []byte{pmbusFCReadCoils, 0x04, 0xFF}
	if _, err := parseBitResponse(pdu, pmbusFCReadCoils, 32); !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("truncated bit response should be rejected, got %v", err)
	}
}

// TestParseBitResponse_InsufficientForQuantity 는 선언한 바이트 수가 요청 비트
// 수에 못 미치는 응답을 거부하는지 확인한다.
func TestParseBitResponse_InsufficientForQuantity(t *testing.T) {
	// 1바이트(8비트)만 왔는데 16비트를 요청했다.
	pdu := []byte{pmbusFCReadCoils, 0x01, 0xFF}
	if _, err := parseBitResponse(pdu, pmbusFCReadCoils, 16); !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("insufficient bit data should be rejected, got %v", err)
	}
}

// TestParseBitResponse_Exception 은 비트 읽기의 예외 응답을 확인한다.
func TestParseBitResponse_Exception(t *testing.T) {
	pdu := []byte{pmbusFCReadCoils | 0x80, 0x02}
	_, err := parseBitResponse(pdu, pmbusFCReadCoils, 8)
	var me *ModbusException
	if !errors.As(err, &me) {
		t.Fatalf("error = %v (%T), want *ModbusException", err, err)
	}
}

// TestParseRegisterResponse_DeclaredCountShort 는 워드 읽기의 절단 응답을 확인한다.
func TestParseRegisterResponse_DeclaredCountShort(t *testing.T) {
	pdu := []byte{pmbusFCReadHolding, 0x0C, 0x00, 0x00}
	if _, err := parseRegisterResponse(pdu, pmbusFCReadHolding, 6); !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("truncated register response should be rejected, got %v", err)
	}
}

// TestParseRegisterResponse_InsufficientForQuantity 는 선언한 바이트 수가 요청
// 워드 수에 못 미치는 응답을 거부하는지 확인한다. 길이는 일관되지만 요청한 만큼
// 담기지 않은 경우로, 절단 응답과는 다른 실패 방식이다.
func TestParseRegisterResponse_InsufficientForQuantity(t *testing.T) {
	// 2워드(4바이트)만 왔는데 6워드를 요청했다.
	pdu := []byte{pmbusFCReadHolding, 0x04, 0x00, 0x01, 0x00, 0x02}
	if _, err := parseRegisterResponse(pdu, pmbusFCReadHolding, 6); !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("insufficient register data should be rejected, got %v", err)
	}
	// 같은 응답을 2워드로 요청하면 정상 파싱된다.
	regs, err := parseRegisterResponse(pdu, pmbusFCReadHolding, 2)
	if err != nil {
		t.Fatalf("2-register read of the same response: %v", err)
	}
	if regs[0] != 1 || regs[1] != 2 {
		t.Errorf("regs = %v, want [1 2]", regs)
	}
}

// TestParseWriteEchoResponse_Errors 는 쓰기 에코 검증의 실패 경로를 확인한다.
func TestParseWriteEchoResponse_Errors(t *testing.T) {
	// 예외 응답.
	err := parseWriteEchoResponse([]byte{pmbusFCWriteSingleCoil | 0x80, 0x02},
		pmbusFCWriteSingleCoil, 0, pmbusCoilOn)
	var me *ModbusException
	if !errors.As(err, &me) {
		t.Errorf("exception should surface as *ModbusException, got %v", err)
	}

	// 5바이트 미만.
	err = parseWriteEchoResponse([]byte{pmbusFCWriteSingleCoil, 0x00, 0x00},
		pmbusFCWriteSingleCoil, 0, pmbusCoilOn)
	if !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("short echo should be rejected, got %v", err)
	}

	// 주소 불일치.
	err = parseWriteEchoResponse([]byte{pmbusFCWriteSingleCoil, 0x00, 0x10, 0xFF, 0x00},
		pmbusFCWriteSingleCoil, 0, pmbusCoilOn)
	if err == nil {
		t.Error("address mismatch should be rejected")
	}
}

// TestParseWriteMultipleResponse_Errors 는 FC16 응답 검증의 실패 경로를 확인한다.
func TestParseWriteMultipleResponse_Errors(t *testing.T) {
	// 예외 응답.
	err := parseWriteMultipleResponse([]byte{pmbusFCWriteMultipleRegs | 0x80, 0x03}, 0, 3)
	var me *ModbusException
	if !errors.As(err, &me) {
		t.Errorf("exception should surface as *ModbusException, got %v", err)
	}

	// 5바이트 미만.
	err = parseWriteMultipleResponse([]byte{pmbusFCWriteMultipleRegs, 0x00, 0x00}, 0, 3)
	if !errors.Is(err, ErrHvacr03ShortResponse) {
		t.Errorf("short response should be rejected, got %v", err)
	}

	// 주소 불일치.
	err = parseWriteMultipleResponse([]byte{pmbusFCWriteMultipleRegs, 0x00, 0x14, 0x00, 0x03}, 0, 3)
	if err == nil {
		t.Error("address mismatch should be rejected")
	}
}

// ---------------------------------------------------------------------------
// 타입 등록 (AC-072)
// ---------------------------------------------------------------------------

// TestRegisterLGHvacr03Types 는 타입 등록과 팩토리 동작을 확인한다.
func TestRegisterLGHvacr03Types(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterLGHvacr03Types(mgr); err != nil {
		t.Fatalf("RegisterLGHvacr03Types: %v", err)
	}

	created, err := mgr.Create(agent.AgentConfig{
		ID:   "registered-agent",
		Name: "registered-pmbus",
		Type: "lg_hvacr03",
		Transport: agent.TransportConfig{Options: map[string]any{
			"transport_type": "rtu",
			"serial_port":    "/dev/null",
		}},
	})
	if err != nil {
		t.Fatalf("Create through the registered type: %v", err)
	}
	if created.Type() != "lg_hvacr03" {
		t.Errorf("Type() = %q, want lg_hvacr03", created.Type())
	}
	if _, ok := created.(*Hvacr03Agent); !ok {
		t.Errorf("created agent is %T, want *Hvacr03Agent", created)
	}
}
