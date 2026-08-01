package modbus

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RTU 시리얼 동기식 마스터 테스트 (TDD, M3)
//
// 시리얼 포트를 io.ReadWriteCloser mock 으로 대체하여 하드웨어 없이 검증한다.
//   - AC-01: 유효 CRC 를 포함한 FC03 응답 → 디코딩된 레지스터가 기대값과 일치.
//   - AC-07: CRC 손상 응답 → ErrCRCMismatch, 유효 결과 미반환.
// ---------------------------------------------------------------------------

// mockSerialPort 는 시리얼 바이트 스트림 seam(io.ReadWriteCloser)의 테스트 구현체이다.
// Write 는 요청 ADU 를 기록하고, Read 는 미리 준비한 응답 바이트를 순차 반환한다.
type mockSerialPort struct {
	written  bytes.Buffer // 마스터가 기록한 요청 ADU
	toRead   *bytes.Reader
	closed   bool
	writeErr error
}

func newMockSerialPort(response []byte) *mockSerialPort {
	return &mockSerialPort{toRead: bytes.NewReader(response)}
}

func (m *mockSerialPort) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return m.written.Write(p)
}

func (m *mockSerialPort) Read(p []byte) (int, error) { return m.toRead.Read(p) }

func (m *mockSerialPort) Close() error { m.closed = true; return nil }

// rtuTestLogger 는 조용한 테스트 로거를 반환한다.
func rtuTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestRTUTransport 는 mock 시리얼 포트를 주입한 ModbusRTUTransport 를 반환한다.
func newTestRTUTransport(port io.ReadWriteCloser) *ModbusRTUTransport {
	t := NewModbusRTUTransport(
		SerialConfig{Port: "/dev/mock", BaudRate: 9600, DataBits: 8, StopBits: 1, Parity: "none"},
		time.Second, rtuTestLogger(),
	)
	t.opener = func(SerialConfig, time.Duration) (io.ReadWriteCloser, error) { return port, nil }
	return t
}

// TestModbusRTUTransport_ReadRoundTrip 는 AC-01 을 검증한다:
// FC03 요청 → 유효 CRC 응답 → 디코딩된 레지스터 일치, 전송 ADU 형식 검증.
func TestModbusRTUTransport_ReadRoundTrip(t *testing.T) {
	// 서버 준비: FC03 응답 PDU = FC(0x03) + ByteCount(0x04) + 2 레지스터(0x1234, 0x5678)
	respPDU := []byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78}
	respADU := buildRTUADU(0x01, respPDU) // 유효 CRC 포함
	port := newMockSerialPort(respADU)

	tr := newTestRTUTransport(port)
	require.NoError(t, tr.Connect(context.Background()))
	require.True(t, tr.IsConnected())

	// FC03 읽기: unitID=1, start=0, qty=2
	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 2)
	gotPDU, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err)

	// 응답 PDU 는 unitID/CRC 를 제거한 순수 PDU 여야 한다.
	assert.Equal(t, respPDU, gotPDU)

	// 디코딩된 레지스터가 기대값과 일치해야 한다.
	fc, values, perr := parseReadResponse(gotPDU)
	require.NoError(t, perr)
	assert.Equal(t, FC03ReadHoldingRegisters, fc)
	assert.Equal(t, []uint16{0x1234, 0x5678}, decodeRegisters(values))

	// 전송된 요청 ADU 는 [unitID][PDU][CRC-lo][CRC-hi] 형식이어야 한다.
	wantReqADU := buildRTUADU(0x01, reqPDU)
	assert.Equal(t, wantReqADU, port.written.Bytes(), "요청 ADU 는 RTU CRC 프레이밍이어야 한다")
}

// TestModbusRTUTransport_BadCRCRejected 는 AC-07 을 검증한다:
// CRC 가 손상된 응답 → ErrCRCMismatch, 유효 PDU 미반환.
func TestModbusRTUTransport_BadCRCRejected(t *testing.T) {
	respPDU := []byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78}
	respADU := buildRTUADU(0x01, respPDU)
	// 마지막 CRC 바이트를 손상시킨다(프레임 길이는 유지).
	respADU[len(respADU)-1] ^= 0xFF
	port := newMockSerialPort(respADU)

	tr := newTestRTUTransport(port)
	require.NoError(t, tr.Connect(context.Background()))

	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 2)
	gotPDU, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)

	require.ErrorIs(t, err, ErrCRCMismatch, "손상된 CRC 는 ErrCRCMismatch 로 거부해야 한다")
	assert.Nil(t, gotPDU, "손상 프레임은 유효한 결과로 상위에 반환하지 않아야 한다")
	// CRC 불일치는 연결 오류가 아니므로 연결은 유지되어야 한다.
	assert.True(t, tr.IsConnected(), "CRC 불일치 후에도 연결은 유지되어야 한다")
}

// TestModbusRTUTransport_ExceptionResponse 는 예외 응답(fc|0x80)이 유효한 프레임으로
// 통과되고 상위 파서가 ModbusException 을 감지하는지 검증한다.
func TestModbusRTUTransport_ExceptionResponse(t *testing.T) {
	respPDU := []byte{FC03ReadHoldingRegisters | 0x80, ExceptionIllegalDataAddress}
	respADU := buildRTUADU(0x01, respPDU)
	port := newMockSerialPort(respADU)

	tr := newTestRTUTransport(port)
	require.NoError(t, tr.Connect(context.Background()))

	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 2)
	gotPDU, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err, "예외 응답은 유효 RTU 프레임으로 통과해야 한다")

	_, _, perr := parseReadResponse(gotPDU)
	var mexc *ModbusException
	require.ErrorAs(t, perr, &mexc)
	assert.Equal(t, ExceptionIllegalDataAddress, mexc.Code)
}

// TestModbusRTUTransport_WriteResponse 는 고정 8바이트 쓰기 응답(FC06) 프레이밍을 검증한다.
func TestModbusRTUTransport_WriteResponse(t *testing.T) {
	// FC06 응답 PDU = echo: FC(0x06) + addr(2) + value(2)
	respPDU := buildWriteSingleRegisterPDU(0x000A, 0x1234)
	respADU := buildRTUADU(0x01, respPDU)
	port := newMockSerialPort(respADU)

	tr := newTestRTUTransport(port)
	require.NoError(t, tr.Connect(context.Background()))

	reqPDU := buildWriteSingleRegisterPDU(0x000A, 0x1234)
	gotPDU, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err)
	assert.Equal(t, respPDU, gotPDU)

	fc, addr, qty, perr := parseWriteResponse(gotPDU)
	require.NoError(t, perr)
	assert.Equal(t, FC06WriteSingleRegister, fc)
	assert.Equal(t, uint16(0x000A), addr)
	assert.Equal(t, uint16(1), qty)
}

// TestModbusRTUTransport_NotConnected 는 미연결 상태에서 오류를 반환하는지 검증한다.
func TestModbusRTUTransport_NotConnected(t *testing.T) {
	tr := newTestRTUTransport(newMockSerialPort(nil))
	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0, 1)
	_, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.ErrorIs(t, err, ErrSerialConnectionFailed)
}

// TestModbusRTUTransport_ConnectClose 는 연결/종료 라이프사이클을 검증한다.
func TestModbusRTUTransport_ConnectClose(t *testing.T) {
	port := newMockSerialPort(nil)
	tr := newTestRTUTransport(port)

	require.NoError(t, tr.Connect(context.Background()))
	assert.True(t, tr.IsConnected())
	// 재연결 호출은 no-op 이어야 한다.
	require.NoError(t, tr.Connect(context.Background()))

	require.NoError(t, tr.Close())
	assert.False(t, tr.IsConnected())
	assert.True(t, port.closed)
	// 재종료는 no-op.
	require.NoError(t, tr.Close())
}

// TestModbusRTUTransport_ImplementsInterface 는 ModbusTransport 를 만족하는지 검증한다.
func TestModbusRTUTransport_ImplementsInterface(t *testing.T) {
	var _ ModbusTransport = NewModbusRTUTransport(SerialConfig{Port: "/dev/x", BaudRate: 9600}, time.Second, rtuTestLogger())
}

// ---------------------------------------------------------------------------
// T3.5 프레임 간 정적 산출 테스트 (A-5)
// ---------------------------------------------------------------------------

// TestRTUInterFrameDelay 는 baudrate 별 T3.5 정적 시간을 검증한다.
//   - ≤19200: 3.5 × 11비트 / baud 초
//   - >19200: 고정 1.75ms
func TestRTUInterFrameDelay(t *testing.T) {
	charTime := func(baud int) time.Duration {
		return time.Duration(float64(rtuCharBits) * 3.5 / float64(baud) * float64(time.Second))
	}

	tests := []struct {
		name string
		baud int
		want time.Duration
	}{
		{name: "9600", baud: 9600, want: charTime(9600)},
		{name: "19200_boundary", baud: 19200, want: charTime(19200)},
		{name: "38400_fixed", baud: 38400, want: 1750 * time.Microsecond},
		{name: "115200_fixed", baud: 115200, want: 1750 * time.Microsecond},
		{name: "zero_defaults_9600", baud: 0, want: charTime(9600)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, rtuInterFrameDelay(tt.baud))
		})
	}

	// 정적 시간은 baud 가 낮을수록 길어야 한다(단조성).
	assert.Greater(t, rtuInterFrameDelay(9600), rtuInterFrameDelay(19200))
	// >19200 은 고정값이므로 문자시간보다 짧다.
	assert.Less(t, rtuInterFrameDelay(38400), rtuInterFrameDelay(19200))
}

// TestRTUInterFrameSilenceEnforced 는 두 번째 프레임 전에 T3.5 정적을 대기하는지
// 검증한다(느린 baud 로 관측 가능한 지연을 유도).
func TestRTUInterFrameSilenceEnforced(t *testing.T) {
	respPDU := []byte{0x03, 0x02, 0xAB, 0xCD}
	// 두 개의 응답을 연속 배치(두 트랜잭션).
	buf := append(buildRTUADU(0x01, respPDU), buildRTUADU(0x01, respPDU)...)
	port := newMockSerialPort(buf)

	// 낮은 baud(1200)로 T3.5 를 크게 만들어 대기를 관측한다.
	tr := NewModbusRTUTransport(
		SerialConfig{Port: "/dev/mock", BaudRate: 1200, DataBits: 8, StopBits: 1, Parity: "none"},
		time.Second, rtuTestLogger(),
	)
	tr.opener = func(SerialConfig, time.Duration) (io.ReadWriteCloser, error) { return port, nil }
	require.NoError(t, tr.Connect(context.Background()))

	reqPDU := buildReadPDU(FC03ReadHoldingRegisters, 0, 1)
	_, err := tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err)

	start := time.Now()
	_, err = tr.SendAndReceive(context.Background(), 0x01, reqPDU)
	require.NoError(t, err)
	elapsed := time.Since(start)

	// 1200 baud 의 T3.5 ≈ 32ms; 최소 절반 이상 대기했어야 한다.
	assert.GreaterOrEqual(t, elapsed, rtuInterFrameDelay(1200)/2)
}
