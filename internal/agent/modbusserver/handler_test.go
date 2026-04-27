package modbusserver

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMBAPFrame constructs a complete MBAP frame for testing.
func buildMBAPFrame(txID uint16, unitID byte, pdu []byte) []byte {
	frame := make([]byte, modbus.MBAPHeaderSize+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], modbus.MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu))) // Length = UnitID + PDU
	frame[6] = unitID
	copy(frame[7:], pdu)
	return frame
}

// parseMBAPResponse parses an MBAP response frame, returning txID, unitID, and the response PDU.
func parseMBAPResponse(data []byte) (txID uint16, unitID byte, pdu []byte) {
	txID = binary.BigEndian.Uint16(data[0:2])
	unitID = data[6]
	length := binary.BigEndian.Uint16(data[4:6])
	pduLen := int(length) - 1
	pdu = data[modbus.MBAPHeaderSize : modbus.MBAPHeaderSize+pduLen]
	return
}

// newTestDeviceManager creates a DeviceManager with a single device (unitID=1)
// using the given RegisterMap. This is the standard helper for handler tests.
func newTestDeviceManager(rm *RegisterMap, unitID byte) *DeviceManager {
	reqHandler := NewRequestHandler(rm, nil)
	dev := &Device{
		UnitID:      unitID,
		Name:        "",
		RegisterMap: rm,
		ReqHandler:  reqHandler,
	}
	dm := &DeviceManager{
		devices: map[byte]*Device{unitID: dev},
		order:   []byte{unitID},
	}
	return dm
}

// newTestMultiDeviceManager creates a DeviceManager with multiple devices.
func newTestMultiDeviceManager(devices map[byte]*RegisterMap) *DeviceManager {
	dm := &DeviceManager{
		devices: make(map[byte]*Device, len(devices)),
		order:   make([]byte, 0, len(devices)),
	}
	for uid, rm := range devices {
		reqHandler := NewRequestHandler(rm, nil)
		dm.devices[uid] = &Device{
			UnitID:      uid,
			Name:        "",
			RegisterMap: rm,
			ReqHandler:  reqHandler,
		}
		dm.order = append(dm.order, uid)
	}
	return dm
}

func TestModbusHandler_ReadRequest(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{100, 200, 300})

	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	// Create pipe-based connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Send FC03 ReadHoldingRegisters: addr=0, quantity=3
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 3)
	frame := buildMBAPFrame(1, 1, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	// Read response
	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	txID, unitID, respPDU := parseMBAPResponse(resp)
	assert.Equal(t, uint16(1), txID)
	assert.Equal(t, byte(1), unitID)
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, respPDU[0])

	// Verify data: 3 registers * 2 bytes = 6 bytes
	byteCount := int(respPDU[1])
	assert.Equal(t, 6, byteCount)
	decoded := decodeRegisterBytes(respPDU[2 : 2+byteCount])
	assert.Equal(t, []uint16{100, 200, 300}, decoded)

	// No change notification for read requests
	select {
	case <-msgCh:
		t.Fatal("unexpected change notification for read request")
	default:
		// expected: no notification
	}
}

func TestModbusHandler_WriteRequest(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Send FC06 WriteSingleRegister: addr=0, value=0x1234
	pdu := []byte{modbus.FC06WriteSingleRegister, 0x00, 0x00, 0x12, 0x34}
	frame := buildMBAPFrame(42, 1, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	// Read response
	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	txID, _, respPDU := parseMBAPResponse(resp)
	assert.Equal(t, uint16(42), txID)
	// Echo-back verification
	assert.Equal(t, pdu, respPDU)

	// Verify register was written
	vals, err := rm.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), vals[0])

	// Verify change notification was sent
	select {
	case notification := <-msgCh:
		assert.Equal(t, "register_change", notification["type"])
		assert.Equal(t, "holding_registers", notification["area"])
		assert.Equal(t, byte(1), notification["unit_id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected change notification, got timeout")
	}
}

func TestModbusHandler_UnitIDMismatch(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Send request with wrong unit ID (99 instead of 1)
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 99, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	// The handler should silently skip the mismatched unit ID.
	// Send another request with correct unit ID to verify handler is still alive.
	rm.WriteHoldingRegisters(0, []uint16{42})
	frame2 := buildMBAPFrame(2, 1, pdu)
	_, err = client.Write(frame2)
	require.NoError(t, err)

	// Read response -- should be for txID=2
	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	txID, _, respPDU := parseMBAPResponse(resp)
	assert.Equal(t, uint16(2), txID)
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, respPDU[0])
}

func TestModbusHandler_InvalidProtocol(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Build a frame with invalid protocol ID (0x0001 instead of 0x0000)
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := make([]byte, modbus.MBAPHeaderSize+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], 1)      // txID
	binary.BigEndian.PutUint16(frame[2:4], 0x0001)  // invalid protocol ID
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu)))
	frame[6] = 1
	copy(frame[7:], pdu)

	_, err := client.Write(frame)
	require.NoError(t, err)

	// Handler should skip the invalid protocol frame.
	// Send a valid frame to verify the handler is still running.
	rm.WriteHoldingRegisters(0, []uint16{99})
	validFrame := buildMBAPFrame(2, 1, pdu)
	_, err = client.Write(validFrame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	txID, _, _ := parseMBAPResponse(resp)
	assert.Equal(t, uint16(2), txID)
}

func TestModbusHandler_ContextCancellation(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		handler.HandleConnection(ctx, server)
		close(done)
	}()

	// Cancel context
	cancel()
	// Close server-side so ReadFull returns immediately
	server.Close()

	select {
	case <-done:
		// HandleConnection exited cleanly
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConnection did not exit after context cancellation")
	}
}

func TestModbusHandler_BroadcastUnitID(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{77})
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Send request with broadcast unit ID (0)
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 0, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	_, unitID, respPDU := parseMBAPResponse(resp)
	assert.Equal(t, byte(0), unitID)
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, respPDU[0])
}

func TestModbusHandler_MultiDeviceRouting(t *testing.T) {
	// 디바이스 1 과 디바이스 2 를 별도의 RegisterMap 으로 생성
	rm1 := newTestRegisterMap()
	rm1.WriteHoldingRegisters(0, []uint16{111})

	rm2 := newTestRegisterMap()
	rm2.WriteHoldingRegisters(0, []uint16{222})

	dm := newTestMultiDeviceManager(map[byte]*RegisterMap{1: rm1, 2: rm2})
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// 디바이스 1 에서 읽기
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 1, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)

	_, _, respPDU := parseMBAPResponse(respBuf[:n])
	decoded := decodeRegisterBytes(respPDU[2 : 2+int(respPDU[1])])
	assert.Equal(t, []uint16{111}, decoded)

	// 디바이스 2 에서 읽기
	frame2 := buildMBAPFrame(2, 2, pdu)
	_, err = client.Write(frame2)
	require.NoError(t, err)

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = client.Read(respBuf)
	require.NoError(t, err)

	_, _, respPDU2 := parseMBAPResponse(respBuf[:n])
	decoded2 := decodeRegisterBytes(respPDU2[2 : 2+int(respPDU2[1])])
	assert.Equal(t, []uint16{222}, decoded2)
}

func TestModbusHandler_BroadcastWrite_FanOut(t *testing.T) {
	// Broadcast write 는 모든 디바이스에 전파되어야 한다
	rm1 := newTestRegisterMap()
	rm2 := newTestRegisterMap()

	dm := newTestMultiDeviceManager(map[byte]*RegisterMap{1: rm1, 2: rm2})
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, nil)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx, server)

	// Broadcast write FC06: addr=0, value=0xABCD
	pdu := []byte{modbus.FC06WriteSingleRegister, 0x00, 0x00, 0xAB, 0xCD}
	frame := buildMBAPFrame(1, 0, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	// Read response
	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := client.Read(respBuf)
	require.NoError(t, err)
	assert.True(t, n > 0)

	// 두 디바이스 모두에 값이 기록되었는지 확인
	vals1, err := rm1.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0xABCD), vals1[0])

	vals2, err := rm2.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0xABCD), vals2[0])
}
