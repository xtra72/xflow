package samsung

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// mockReadWriteCloser: io.ReadWriteCloser mock for serial transport testing
// ---------------------------------------------------------------------------

type mockReadWriteCloser struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
	mu       sync.Mutex
}

func newMockReadWriteCloser() *mockReadWriteCloser {
	return &mockReadWriteCloser{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.readBuf.Read(p)
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(p)
}

func (m *mockReadWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return io.ErrClosedPipe
	}
	m.closed = true
	return nil
}

// ===========================================================================
// NewNASATransport factory tests (REQ-02-04)
// ===========================================================================

func TestNewNASATransport(t *testing.T) {
	tests := []struct {
		name          string
		transportType string
		opts          map[string]any
		wantErr       error
		wantType      string // "serial" or "tcp"
	}{
		{
			name:          "serial with valid port",
			transportType: "serial",
			opts:          map[string]any{"serial_port": "/dev/ttyUSB0"},
			wantErr:       nil,
			wantType:      "serial",
		},
		{
			name:          "serial without port returns ErrSerialPortRequired",
			transportType: "serial",
			opts:          map[string]any{},
			wantErr:       ErrSerialPortRequired,
		},
		{
			name:          "serial with nil opts returns ErrSerialPortRequired",
			transportType: "serial",
			opts:          nil,
			wantErr:       ErrSerialPortRequired,
		},
		{
			name:          "tcp with valid address",
			transportType: "tcp",
			opts:          map[string]any{"tcp_address": "192.168.1.100:4196"},
			wantErr:       nil,
			wantType:      "tcp",
		},
		{
			name:          "tcp without address returns ErrTCPAddressRequired",
			transportType: "tcp",
			opts:          map[string]any{},
			wantErr:       ErrTCPAddressRequired,
		},
		{
			name:          "tcp with nil opts returns ErrTCPAddressRequired",
			transportType: "tcp",
			opts:          nil,
			wantErr:       ErrTCPAddressRequired,
		},
		{
			name:          "unknown type returns ErrInvalidTransportType",
			transportType: "bluetooth",
			opts:          map[string]any{},
			wantErr:       ErrInvalidTransportType,
		},
		{
			name:          "empty type returns ErrInvalidTransportType",
			transportType: "",
			opts:          map[string]any{},
			wantErr:       ErrInvalidTransportType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewNASATransport(tt.transportType, tt.opts)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewNASATransport(%q) error = %v, want %v", tt.transportType, err, tt.wantErr)
				}
				if tr != nil {
					t.Fatalf("NewNASATransport(%q) returned non-nil transport on error", tt.transportType)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewNASATransport(%q) unexpected error: %v", tt.transportType, err)
			}
			if tr == nil {
				t.Fatalf("NewNASATransport(%q) returned nil", tt.transportType)
			}

			switch tt.wantType {
			case "serial":
				if _, ok := tr.(*NASASerialTransport); !ok {
					t.Fatalf("expected *NASASerialTransport, got %T", tr)
				}
			case "tcp":
				if _, ok := tr.(*NASATCPTransport); !ok {
					t.Fatalf("expected *NASATCPTransport, got %T", tr)
				}
			}
		})
	}
}

// ===========================================================================
// Serial transport default values (REQ-02-02)
// ===========================================================================

func TestNewNASATransport_SerialDefaults(t *testing.T) {
	tr, err := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, ok := tr.(*NASASerialTransport)
	if !ok {
		t.Fatalf("expected *NASASerialTransport, got %T", tr)
	}

	if st.port != "/dev/ttyUSB0" {
		t.Errorf("port = %q, want %q", st.port, "/dev/ttyUSB0")
	}
	if st.baudRate != 9600 {
		t.Errorf("baudRate = %d, want %d", st.baudRate, 9600)
	}
	if st.dataBits != 8 {
		t.Errorf("dataBits = %d, want %d", st.dataBits, 8)
	}
	if st.stopBits != 1 {
		t.Errorf("stopBits = %d, want %d", st.stopBits, 1)
	}
	if st.parity != "even" {
		t.Errorf("parity = %q, want %q", st.parity, "even")
	}
}

// ===========================================================================
// Serial transport custom values with float64 opts (YAML/JSON unmarshal)
// ===========================================================================

func TestNewNASATransport_SerialCustomValues(t *testing.T) {
	tr, err := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyS0",
		"baud_rate":   float64(19200), // YAML/JSON float64
		"data_bits":   float64(7),
		"stop_bits":   float64(2),
		"parity":      "odd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st := tr.(*NASASerialTransport)
	if st.port != "/dev/ttyS0" {
		t.Errorf("port = %q, want %q", st.port, "/dev/ttyS0")
	}
	if st.baudRate != 19200 {
		t.Errorf("baudRate = %d, want %d", st.baudRate, 19200)
	}
	if st.dataBits != 7 {
		t.Errorf("dataBits = %d, want %d", st.dataBits, 7)
	}
	if st.stopBits != 2 {
		t.Errorf("stopBits = %d, want %d", st.stopBits, 2)
	}
	if st.parity != "odd" {
		t.Errorf("parity = %q, want %q", st.parity, "odd")
	}
}

// ===========================================================================
// Serial transport custom values with int opts
// ===========================================================================

func TestNewNASATransport_SerialIntOpts(t *testing.T) {
	tr, err := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyS0",
		"baud_rate":   115200,
		"data_bits":   7,
		"stop_bits":   2,
		"parity":      "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st := tr.(*NASASerialTransport)
	if st.baudRate != 115200 {
		t.Errorf("baudRate = %d, want %d", st.baudRate, 115200)
	}
	if st.dataBits != 7 {
		t.Errorf("dataBits = %d, want %d", st.dataBits, 7)
	}
	if st.stopBits != 2 {
		t.Errorf("stopBits = %d, want %d", st.stopBits, 2)
	}
}

// ===========================================================================
// TCP transport default values (REQ-02-03)
// ===========================================================================

func TestNewNASATransport_TCPDefaults(t *testing.T) {
	tr, err := NewNASATransport("tcp", map[string]any{
		"tcp_address": "10.0.0.1:4196",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tt, ok := tr.(*NASATCPTransport)
	if !ok {
		t.Fatalf("expected *NASATCPTransport, got %T", tr)
	}

	if tt.address != "10.0.0.1:4196" {
		t.Errorf("address = %q, want %q", tt.address, "10.0.0.1:4196")
	}
	if tt.connectTimeout != 5*time.Second {
		t.Errorf("connectTimeout = %v, want %v", tt.connectTimeout, 5*time.Second)
	}
	if tt.readTimeout != 3*time.Second {
		t.Errorf("readTimeout = %v, want %v", tt.readTimeout, 3*time.Second)
	}
}

// ===========================================================================
// TCP transport custom timeout values
// ===========================================================================

func TestNewNASATransport_TCPCustomTimeouts(t *testing.T) {
	tr, err := NewNASATransport("tcp", map[string]any{
		"tcp_address":     "10.0.0.1:4196",
		"connect_timeout": "10s",
		"read_timeout":    "1s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tt := tr.(*NASATCPTransport)
	if tt.connectTimeout != 10*time.Second {
		t.Errorf("connectTimeout = %v, want %v", tt.connectTimeout, 10*time.Second)
	}
	if tt.readTimeout != 1*time.Second {
		t.Errorf("readTimeout = %v, want %v", tt.readTimeout, 1*time.Second)
	}
}

// ===========================================================================
// Serial transport: Available() before Open (REQ-02-02)
// ===========================================================================

func TestSerialTransport_AvailableBeforeOpen(t *testing.T) {
	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if tr.Available() {
		t.Error("Available() = true before Open(), want false")
	}
}

// ===========================================================================
// Serial transport: Send when not open returns ErrTransportNotConnected
// ===========================================================================

func TestSerialTransport_SendNotOpen(t *testing.T) {
	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	err := tr.Send([]byte{0x01, 0x02})
	if !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Send() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// ===========================================================================
// Serial transport: Receive when not open returns ErrTransportNotConnected
// ===========================================================================

func TestSerialTransport_ReceiveNotOpen(t *testing.T) {
	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	buf := make([]byte, 64)
	_, err := tr.Receive(buf)
	if !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Receive() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// ===========================================================================
// Serial transport: Open/Close/Available cycle with mock
// ===========================================================================

func TestSerialTransport_OpenCloseCycle(t *testing.T) {
	mock := newMockReadWriteCloser()

	// SerialOpener 를 mock 으로 대체
	origOpener := SerialOpener
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = origOpener }()

	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})

	// Open
	if err := tr.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !tr.Available() {
		t.Error("Available() = false after Open(), want true")
	}

	// Close
	if err := tr.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if tr.Available() {
		t.Error("Available() = true after Close(), want false")
	}
}

// ===========================================================================
// Serial transport: Send/Receive with mock
// ===========================================================================

func TestSerialTransport_SendReceive(t *testing.T) {
	mock := newMockReadWriteCloser()

	origOpener := SerialOpener
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = origOpener }()

	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if err := tr.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer tr.Close()

	// Send
	data := []byte{0x32, 0x00, 0x10, 0x34}
	if err := tr.Send(data); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// send 한 데이터가 mock writeBuf 에 기록되었는지 확인
	if !bytes.Equal(mock.writeBuf.Bytes(), data) {
		t.Errorf("written data = %x, want %x", mock.writeBuf.Bytes(), data)
	}

	// Receive: mock readBuf 에 데이터 주입
	mock.readBuf.Write([]byte{0xAA, 0xBB, 0xCC})
	buf := make([]byte, 64)
	n, err := tr.Receive(buf)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if n != 3 {
		t.Errorf("Receive() n = %d, want %d", n, 3)
	}
	if !bytes.Equal(buf[:n], []byte{0xAA, 0xBB, 0xCC}) {
		t.Errorf("received data = %x, want %x", buf[:n], []byte{0xAA, 0xBB, 0xCC})
	}
}

// ===========================================================================
// Serial transport: Open failure propagates error
// ===========================================================================

func TestSerialTransport_OpenFailure(t *testing.T) {
	origOpener := SerialOpener
	openErr := errors.New("permission denied")
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return nil, openErr
	}
	defer func() { SerialOpener = origOpener }()

	tr, _ := NewNASATransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})

	err := tr.Open()
	if !errors.Is(err, openErr) {
		t.Fatalf("Open() error = %v, want %v", err, openErr)
	}
	if tr.Available() {
		t.Error("Available() = true after failed Open(), want false")
	}
}

// ===========================================================================
// TCP transport: Available() before Open (REQ-02-03)
// ===========================================================================

func TestTCPTransport_AvailableBeforeOpen(t *testing.T) {
	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address": "10.0.0.1:4196",
	})
	if tr.Available() {
		t.Error("Available() = true before Open(), want false")
	}
}

// ===========================================================================
// TCP transport: Send when not open returns ErrTransportNotConnected
// ===========================================================================

func TestTCPTransport_SendNotOpen(t *testing.T) {
	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address": "10.0.0.1:4196",
	})
	err := tr.Send([]byte{0x01})
	if !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Send() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// ===========================================================================
// TCP transport: Receive when not open returns ErrTransportNotConnected
// ===========================================================================

func TestTCPTransport_ReceiveNotOpen(t *testing.T) {
	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address": "10.0.0.1:4196",
	})
	buf := make([]byte, 64)
	_, err := tr.Receive(buf)
	if !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Receive() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// ===========================================================================
// TCP transport: Open/Close/Available cycle with net.Pipe
// ===========================================================================

func TestTCPTransport_OpenCloseCycle(t *testing.T) {
	// net.Pipe 로 in-memory 연결 생성
	server, client := net.Pipe()
	defer server.Close()

	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address": "127.0.0.1:0",
	})
	tcp := tr.(*NASATCPTransport)

	// conn 을 직접 주입하여 실제 다이얼 없이 테스트
	tcp.mu.Lock()
	tcp.conn = client
	tcp.open = true
	tcp.mu.Unlock()

	if !tcp.Available() {
		t.Error("Available() = false after injecting conn, want true")
	}

	if err := tcp.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if tcp.Available() {
		t.Error("Available() = true after Close(), want false")
	}
}

// ===========================================================================
// TCP transport: Send/Receive with net.Pipe
// ===========================================================================

func TestTCPTransport_SendReceive(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address": "127.0.0.1:0",
	})
	tcp := tr.(*NASATCPTransport)

	// readTimeout 을 0 으로 설정하여 SetReadDeadline 을 스킵 (net.Pipe 호환)
	tcp.mu.Lock()
	tcp.conn = client
	tcp.open = true
	tcp.readTimeout = 0
	tcp.mu.Unlock()

	// Send: net.Pipe 는 unbuffered 이므로, server 쪽에서 동시에 Read 해야 함
	data := []byte{0x32, 0x00, 0x10, 0x34}
	serverBuf := make([]byte, 64)
	var serverN int
	var serverErr error
	done := make(chan struct{})

	go func() {
		serverN, serverErr = server.Read(serverBuf)
		close(done)
	}()

	if err := tcp.Send(data); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	<-done
	if serverErr != nil {
		t.Fatalf("server Read() error = %v", serverErr)
	}
	if !bytes.Equal(serverBuf[:serverN], data) {
		t.Errorf("server received = %x, want %x", serverBuf[:serverN], data)
	}

	// Receive: server -> client 로 데이터 전송
	go func() {
		server.Write([]byte{0xAA, 0xBB, 0xCC})
	}()

	recvBuf := make([]byte, 64)
	n, err := tcp.Receive(recvBuf)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if n != 3 {
		t.Errorf("Receive() n = %d, want %d", n, 3)
	}
	if !bytes.Equal(recvBuf[:n], []byte{0xAA, 0xBB, 0xCC}) {
		t.Errorf("received = %x, want %x", recvBuf[:n], []byte{0xAA, 0xBB, 0xCC})
	}
}

// ===========================================================================
// TCP transport: Open with real listener
// ===========================================================================

func TestTCPTransport_OpenWithListener(t *testing.T) {
	// 로컬 리스너 생성
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer ln.Close()

	// 서버: 접속 수락
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	tr, _ := NewNASATransport("tcp", map[string]any{
		"tcp_address":     ln.Addr().String(),
		"connect_timeout": "2s",
	})

	if err := tr.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer tr.Close()

	if !tr.Available() {
		t.Error("Available() = false after Open(), want true")
	}

	// 서버 연결 정리
	select {
	case conn := <-accepted:
		conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server accept")
	}
}

// ===========================================================================
// NASATransport interface compliance check
// ===========================================================================

func TestNASATransportInterface(t *testing.T) {
	// 컴파일 타임 인터페이스 준수 확인
	var _ NASATransport = (*NASASerialTransport)(nil)
	var _ NASATransport = (*NASATCPTransport)(nil)
}
