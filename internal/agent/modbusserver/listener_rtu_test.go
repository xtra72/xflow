package modbusserver

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// RTU 슬레이브 리스너 테스트 (하드웨어 없이 opener seam + mock 시리얼로 검증)
// ---------------------------------------------------------------------------

// mockRWCloser 는 io.ReadWriteCloser 를 만족하는 mock 시리얼 포트이다.
// in 은 인바운드 요청 바이트(소진 후 EOF), out 은 리스너가 되돌려 쓴 응답을 담는다.
type mockRWCloser struct {
	in     *bytes.Reader
	mu     sync.Mutex
	out    bytes.Buffer
	closed bool
}

func (m *mockRWCloser) Read(p []byte) (int, error) { return m.in.Read(p) }

func (m *mockRWCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.out.Write(p)
}

func (m *mockRWCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockRWCloser) response() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte{}, m.out.Bytes()...)
}

// newRTUTestHandler 는 unitID=1, holding_registers[0..10) 를 가진 ModbusHandler 를 만든다.
func newRTUTestHandler(t *testing.T, initial []any) *ModbusHandler {
	t.Helper()
	dm, err := NewDeviceManager([]DeviceConfig{
		{
			UnitID: 1,
			RegisterMap: RegisterMapConfig{
				HoldingRegisters: []*RegisterAreaConfig{
					{StartAddress: 0, Count: 10, InitialValues: initial},
				},
			},
		},
	}, nil)
	require.NoError(t, err)
	return NewModbusHandler(dm, make(chan map[string]any, 8), true, nil, nil)
}

// startRTUListenerWithRequest 는 mock 시리얼로 단일 요청을 주입하고 응답이 기록될
// 때까지 대기한 뒤 응답 ADU 를 반환한다.
func startRTUListenerWithRequest(t *testing.T, handler *ModbusHandler, reqADU []byte) []byte {
	t.Helper()
	mock := &mockRWCloser{in: bytes.NewReader(reqADU)}

	l := NewRTUListener(SerialConfig{Port: "/dev/mock", BaudRate: 9600, DataBits: 8, StopBits: 1, Parity: "none"}, handler, nil, nil)
	l.opener = func(_ SerialConfig) (io.ReadWriteCloser, error) { return mock, nil }

	require.NoError(t, l.Start(context.Background()))
	// 리스너가 요청을 처리하고 응답을 쓸 때까지 대기(취소 레이스 회피).
	require.Eventually(t, func() bool { return len(mock.response()) > 0 }, time.Second, 5*time.Millisecond)
	require.NoError(t, l.Stop())

	return mock.response()
}

// TestRTUListener_ReadHoldingRegisters 는 FC03 읽기 요청에 대한 RTU 응답을 검증한다.
func TestRTUListener_ReadHoldingRegisters(t *testing.T) {
	handler := newRTUTestHandler(t, []any{0x1234, 0x5678})

	// FC03, addr=0, qty=2
	reqPDU := []byte{0x03, 0x00, 0x00, 0x00, 0x02}
	reqADU := modbus.BuildRTUADU(1, reqPDU)

	respADU := startRTUListenerWithRequest(t, handler, reqADU)

	pdu, err := modbus.ParseRTUADU(respADU, 1, 0x03)
	require.NoError(t, err)
	// [FC=03][byteCount=04][0x1234][0x5678]
	assert.Equal(t, []byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78}, pdu)
}

// TestRTUListener_WriteSingleRegister 는 FC06 쓰기 요청 처리 + 에코 응답 + 레지스터
// 실제 변경을 검증한다.
func TestRTUListener_WriteSingleRegister(t *testing.T) {
	handler := newRTUTestHandler(t, nil)

	// FC06, addr=5, value=0xABCD
	reqPDU := []byte{0x06, 0x00, 0x05, 0xAB, 0xCD}
	reqADU := modbus.BuildRTUADU(1, reqPDU)

	respADU := startRTUListenerWithRequest(t, handler, reqADU)

	pdu, err := modbus.ParseRTUADU(respADU, 1, 0x06)
	require.NoError(t, err)
	assert.Equal(t, reqPDU, pdu, "FC06 응답은 요청 에코")

	// 레지스터가 실제로 변경되었는지 확인
	dev := handler.deviceManager.GetDevice(1)
	require.NotNil(t, dev)
	vals, err := dev.RegisterMap.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0xABCD}, vals)
}
