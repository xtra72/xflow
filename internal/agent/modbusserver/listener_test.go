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

func TestListener_StartStop(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	listener := NewListener(":0", 10, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := listener.Start(ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(0), listener.ActiveConnections())

	err = listener.Stop()
	require.NoError(t, err)
}

func TestListener_AcceptConnection(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{42})
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	listener := NewListener(":0", 10, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := listener.Start(ctx)
	require.NoError(t, err)
	defer listener.Stop()

	// Get the actual address the listener bound to
	addr := listener.listener.Addr().String()

	// Connect a TCP client
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Wait briefly for the connection to be registered
	time.Sleep(50 * time.Millisecond)
	assert.GreaterOrEqual(t, listener.ActiveConnections(), int32(1))

	// Send a valid MODBUS request
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 1, pdu)
	_, err = conn.Write(frame)
	require.NoError(t, err)

	// Read response
	respBuf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(respBuf)
	require.NoError(t, err)

	resp := respBuf[:n]
	require.True(t, len(resp) >= modbus.MBAPHeaderSize+2)
	// Verify response has correct FC
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, resp[modbus.MBAPHeaderSize])
}

func TestListener_MaxConnections(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	// Set max connections to 2
	listener := NewListener(":0", 2, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := listener.Start(ctx)
	require.NoError(t, err)
	defer listener.Stop()

	addr := listener.listener.Addr().String()

	// Create 2 connections (should succeed)
	conn1, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn1.Close()

	conn2, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn2.Close()

	// Wait for connections to be registered
	time.Sleep(100 * time.Millisecond)

	// Verify both are active
	assert.Equal(t, int32(2), listener.ActiveConnections())

	// Create a 3rd connection -- it should be accepted at TCP level but
	// immediately closed by the listener
	conn3, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn3.Close()

	// The 3rd connection should be closed by the server.
	// Try to read from it; it should get an error or EOF.
	conn3.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn3.Read(buf)
	// We expect either EOF or a timeout error (if the close hasn't propagated yet)
	// The key assertion is that activeConns stays at 2
	time.Sleep(100 * time.Millisecond)
	assert.LessOrEqual(t, listener.ActiveConnections(), int32(2))
}

func TestListener_AlreadyRunning(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	listener := NewListener(":0", 10, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := listener.Start(ctx)
	require.NoError(t, err)
	defer listener.Stop()

	// Starting again should fail
	err = listener.Start(ctx)
	assert.Equal(t, ErrServerAlreadyRunning, err)
}

func TestListener_ContextCancel(t *testing.T) {
	rm := newTestRegisterMap()
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	listener := NewListener(":0", 10, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	err := listener.Start(ctx)
	require.NoError(t, err)

	addr := listener.listener.Addr().String()

	// Connect a client
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Send a request to confirm the connection is working
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 1, pdu)
	_, err = conn.Write(frame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(respBuf)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Stop should return without hanging
	done := make(chan struct{})
	go func() {
		listener.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stopped cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("listener.Stop() did not complete after context cancellation")
	}
}

func TestListener_MultipleClients(t *testing.T) {
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{111, 222})
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)
	handler := NewModbusHandler(dm, msgCh, true, nil, nil)

	listener := NewListener(":0", 10, 30*time.Second, handler, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := listener.Start(ctx)
	require.NoError(t, err)
	defer listener.Stop()

	addr := listener.listener.Addr().String()

	// Connect 3 clients simultaneously
	for i := 0; i < 3; i++ {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		require.NoError(t, err)
		defer conn.Close()

		// Each client sends a read request
		pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 2)
		frame := buildMBAPFrame(uint16(i+1), 1, pdu)
		_, err = conn.Write(frame)
		require.NoError(t, err)

		respBuf := make([]byte, 256)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(respBuf)
		require.NoError(t, err)

		resp := respBuf[:n]
		// Verify transaction ID echoed back
		txID := binary.BigEndian.Uint16(resp[0:2])
		assert.Equal(t, uint16(i+1), txID)
	}
}
