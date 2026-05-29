package samsung

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
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
	readErr  error // nil 이면 readBuf 에서 읽기, non-nil 이면 이 에러 반환
	writeErr error // nil 이면 writeBuf 에 쓰기, non-nil 이면 이 에러 반환
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
	if m.readErr != nil {
		return 0, m.readErr
	}
	return m.readBuf.Read(p)
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	if m.writeErr != nil {
		return 0, m.writeErr
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
// NewNasaTransport factory tests (REQ-02-04)
// ===========================================================================

func TestNewNasaTransport(t *testing.T) {
	tests := []struct {
		name          string
		transportType string
		opts          map[string]any
		wantErr       error
		wantType      string // "serial", "tcp-client", "tcp-server"
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
			name:          "tcp-client with valid host and port",
			transportType: "tcp-client",
			opts:          map[string]any{"tcp_host": "192.168.1.100", "tcp_port": 4196},
			wantErr:       nil,
			wantType:      "tcp-client",
		},
		{
			name:          "tcp-client without host returns ErrTCPHostRequired",
			transportType: "tcp-client",
			opts:          map[string]any{"tcp_port": 4196},
			wantErr:       ErrTCPHostRequired,
		},
		{
			name:          "tcp-client with nil opts returns ErrTCPHostRequired",
			transportType: "tcp-client",
			opts:          nil,
			wantErr:       ErrTCPHostRequired,
		},
		{
			name:          "tcp-client with host but no port returns ErrTCPPortRequired",
			transportType: "tcp-client",
			opts:          map[string]any{"tcp_host": "192.168.1.100"},
			wantErr:       ErrTCPPortRequired,
		},
		{
			name:          "tcp-server with explicit bind host and port",
			transportType: "tcp-server",
			opts:          map[string]any{"tcp_host": "127.0.0.1", "tcp_port": 4196},
			wantErr:       nil,
			wantType:      "tcp-server",
		},
		{
			name:          "tcp-server without host uses default 0.0.0.0",
			transportType: "tcp-server",
			opts:          map[string]any{"tcp_port": 4196},
			wantErr:       nil,
			wantType:      "tcp-server",
		},
		{
			name:          "tcp-server without port returns ErrTCPPortRequired",
			transportType: "tcp-server",
			opts:          map[string]any{"tcp_host": "127.0.0.1"},
			wantErr:       ErrTCPPortRequired,
		},
		{
			name:          "tcp-server with nil opts returns ErrTCPPortRequired",
			transportType: "tcp-server",
			opts:          nil,
			wantErr:       ErrTCPPortRequired,
		},
		{
			name:          "deprecated 'tcp' returns ErrDeprecatedTCPTransport (2026-05-29 breaking)",
			transportType: "tcp",
			opts:          map[string]any{"tcp_host": "192.168.1.100", "tcp_port": 4196},
			wantErr:       ErrDeprecatedTCPTransport,
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
			tr, err := NewNasaTransport(tt.transportType, tt.opts)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewNasaTransport(%q) error = %v, want %v", tt.transportType, err, tt.wantErr)
				}
				if tr != nil {
					t.Fatalf("NewNasaTransport(%q) returned non-nil transport on error", tt.transportType)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewNasaTransport(%q) unexpected error: %v", tt.transportType, err)
			}
			if tr == nil {
				t.Fatalf("NewNasaTransport(%q) returned nil", tt.transportType)
			}

			switch tt.wantType {
			case "serial":
				if _, ok := tr.(*NasaSerialTransport); !ok {
					t.Fatalf("expected *NasaSerialTransport, got %T", tr)
				}
			case "tcp-client":
				if _, ok := tr.(*NasaTCPTransport); !ok {
					t.Fatalf("expected *NasaTCPTransport, got %T", tr)
				}
			case "tcp-server":
				if _, ok := tr.(*NasaTCPServerTransport); !ok {
					t.Fatalf("expected *NasaTCPServerTransport, got %T", tr)
				}
			}
		})
	}
}

// ===========================================================================
// Serial transport default values (REQ-02-02)
// ===========================================================================

func TestNewNasaTransport_SerialDefaults(t *testing.T) {
	tr, err := NewNasaTransport("serial", map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, ok := tr.(*NasaSerialTransport)
	if !ok {
		t.Fatalf("expected *NasaSerialTransport, got %T", tr)
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

func TestNewNasaTransport_SerialCustomValues(t *testing.T) {
	tr, err := NewNasaTransport("serial", map[string]any{
		"serial_port": "/dev/ttyS0",
		"baud_rate":   float64(19200), // YAML/JSON float64
		"data_bits":   float64(7),
		"stop_bits":   float64(2),
		"parity":      "odd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st := tr.(*NasaSerialTransport)
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

func TestNewNasaTransport_SerialIntOpts(t *testing.T) {
	tr, err := NewNasaTransport("serial", map[string]any{
		"serial_port": "/dev/ttyS0",
		"baud_rate":   115200,
		"data_bits":   7,
		"stop_bits":   2,
		"parity":      "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st := tr.(*NasaSerialTransport)
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

func TestNewNasaTransport_TCPDefaults(t *testing.T) {
	tr, err := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "10.0.0.1",
		"tcp_port": 4196,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tt, ok := tr.(*NasaTCPTransport)
	if !ok {
		t.Fatalf("expected *NasaTCPTransport, got %T", tr)
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

func TestNewNasaTransport_TCPCustomTimeouts(t *testing.T) {
	tr, err := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host":        "10.0.0.1",
		"tcp_port":        4196,
		"connect_timeout": "10s",
		"read_timeout":    "1s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tt := tr.(*NasaTCPTransport)
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
	tr, _ := NewNasaTransport("serial", map[string]any{
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
	tr, _ := NewNasaTransport("serial", map[string]any{
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
	tr, _ := NewNasaTransport("serial", map[string]any{
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

	tr, _ := NewNasaTransport("serial", map[string]any{
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

	tr, _ := NewNasaTransport("serial", map[string]any{
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

	// send 한 데이터가 preamble(0x55 x 100) + data 로 기록되었는지 확인
	expected := prependPreamble(data)
	if !bytes.Equal(mock.writeBuf.Bytes(), expected) {
		t.Errorf("written data length = %d, want %d (preamble %d + data %d)",
			mock.writeBuf.Len(), len(expected), preambleLen, len(data))
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

	tr, _ := NewNasaTransport("serial", map[string]any{
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
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "10.0.0.1",
		"tcp_port": 4196,
	})
	if tr.Available() {
		t.Error("Available() = true before Open(), want false")
	}
}

// ===========================================================================
// TCP transport: Send when not open returns ErrTransportNotConnected
// ===========================================================================

func TestTCPTransport_SendNotOpen(t *testing.T) {
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "10.0.0.1",
		"tcp_port": 4196,
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
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "10.0.0.1",
		"tcp_port": 4196,
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

	// 실제 다이얼 없이 conn 을 직접 주입하므로, port 는 0 이 아닌 임의 값 사용
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4196,
	})
	tcp := tr.(*NasaTCPTransport)

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

	// 실제 다이얼 없이 conn 을 직접 주입하므로, port 는 0 이 아닌 임의 값 사용
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4196,
	})
	tcp := tr.(*NasaTCPTransport)

	// readTimeout 을 0 으로 설정하여 SetReadDeadline 을 스킵 (net.Pipe 호환)
	tcp.mu.Lock()
	tcp.conn = client
	tcp.open = true
	tcp.readTimeout = 0
	tcp.mu.Unlock()

	// Send: net.Pipe 는 unbuffered 이므로, server 쪽에서 동시에 Read 해야 함
	data := []byte{0x32, 0x00, 0x10, 0x34}
	serverBuf := make([]byte, preambleLen+64)
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
	expected := prependPreamble(data)
	if !bytes.Equal(serverBuf[:serverN], expected) {
		t.Errorf("server received length = %d, want %d (preamble %d + data %d)",
			serverN, len(expected), preambleLen, len(data))
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

	lnAddr := ln.Addr().(*net.TCPAddr)
	tr, _ := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host":        lnAddr.IP.String(),
		"tcp_port":        lnAddr.Port,
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
// NasaTransport interface compliance check
// ===========================================================================

func TestNasaTransportInterface(t *testing.T) {
	// 컴파일 타임 인터페이스 준수 확인
	var _ NasaTransport = (*NasaSerialTransport)(nil)
	var _ NasaTransport = (*NasaTCPTransport)(nil)
	var _ NasaTransport = (*NasaTCPServerTransport)(nil)
}

// ===========================================================================
// Transport Available() 상태 갱신 테스트 (REQ-NASA-001-02-05)
// ===========================================================================

// TestSerialTransport_ReceiveEOF_SetsAvailableFalse 는 Receive()에서
// io.EOF 수신 시 Available() 이 false 로 변경되는지 테스트한다.
func TestSerialTransport_ReceiveEOF_SetsAvailableFalse(t *testing.T) {
	mock := newMockReadWriteCloser()
	// EOF 를 반환하도록 설정
	mock.mu.Lock()
	mock.readErr = io.EOF
	mock.mu.Unlock()

	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{
		port:     "/dev/test",
		baudRate: 9600,
		dataBits: 8,
		stopBits: 1,
		parity:   "even",
	}

	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if !st.Available() {
		t.Fatal("Open 후 Available() = false")
	}

	buf := make([]byte, 64)
	_, err := st.Receive(buf)
	if err != io.EOF {
		t.Fatalf("Receive() error = %v, want io.EOF", err)
	}

	if st.Available() {
		t.Error("io.EOF 후 Available() 이 여전히 true (false 예상)")
	}
}

// TestSerialTransport_SendEOF_SetsAvailableFalse 는 Send()에서
// 연결 끊김 에러 시 Available() 이 false 로 변경되는지 테스트한다.
func TestSerialTransport_SendEOF_SetsAvailableFalse(t *testing.T) {
	mock := newMockReadWriteCloser()
	mock.mu.Lock()
	mock.writeErr = io.ErrClosedPipe
	mock.mu.Unlock()

	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{
		port:     "/dev/test",
		baudRate: 9600,
		dataBits: 8,
		stopBits: 1,
		parity:   "even",
	}

	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	err := st.Send([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("Send() should return error")
	}

	if st.Available() {
		t.Error("io.ErrClosedPipe 후 Available() 이 여전히 true (false 예상)")
	}
}

// TestSerialTransport_TimeoutDoesNotChangeAvailable 는 타임아웃 에러가
// Available() 상태를 변경하지 않는지 테스트한다.
func TestSerialTransport_TimeoutDoesNotChangeAvailable(t *testing.T) {
	timeoutErr := &mockTimeoutError{msg: "read timeout", isTimeout: true}
	mock := newMockReadWriteCloser()
	mock.mu.Lock()
	mock.readErr = timeoutErr
	mock.mu.Unlock()

	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{
		port:     "/dev/test",
		baudRate: 9600,
		dataBits: 8,
		stopBits: 1,
		parity:   "even",
	}

	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	buf := make([]byte, 64)
	_, _ = st.Receive(buf)

	if !st.Available() {
		t.Error("타임아웃 에러 후 Available() = false (true 유지 예상)")
	}
}

// TestSerialTransport_SendENXIO_SetsAvailableFalse 는 ENXIO (device not configured)
// 에러 발생 시 Available() 이 false 로 전환되는지 테스트한다.
func TestSerialTransport_SendENXIO_SetsAvailableFalse(t *testing.T) {
	// os.PathError 래핑: 시리얼 포트 write 에러 패턴
	enxioErr := &os.PathError{
		Op:   "write",
		Path: "/dev/ttyUSB0",
		Err:  syscall.ENXIO,
	}

	mock := newMockReadWriteCloser()
	mock.mu.Lock()
	mock.writeErr = enxioErr
	mock.mu.Unlock()

	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{
		port:     "/dev/test",
		baudRate: 9600,
		dataBits: 8,
		stopBits: 1,
		parity:   "even",
	}

	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	err := st.Send([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("Send() should return error on ENXIO")
	}

	if !errors.Is(err, syscall.ENXIO) {
		t.Errorf("error should wrap ENXIO, got: %v", err)
	}

	if st.Available() {
		t.Error("ENXIO 후 Available() 이 여전히 true (false 예상)")
	}
}

// TestSerialTransport_ReceiveENXIO_SetsAvailableFalse 는 Receive 에서 ENXIO
// 에러 발생 시 Available() 이 false 로 전환되는지 테스트한다.
func TestSerialTransport_ReceiveENXIO_SetsAvailableFalse(t *testing.T) {
	enxioErr := &os.PathError{
		Op:   "read",
		Path: "/dev/ttyUSB0",
		Err:  syscall.ENXIO,
	}

	mock := newMockReadWriteCloser()
	mock.mu.Lock()
	mock.readErr = enxioErr
	mock.mu.Unlock()

	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return mock, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{
		port:     "/dev/test",
		baudRate: 9600,
		dataBits: 8,
		stopBits: 1,
		parity:   "even",
	}

	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	buf := make([]byte, 64)
	_, err := st.Receive(buf)
	if err == nil {
		t.Fatal("Receive() should return error on ENXIO")
	}

	if st.Available() {
		t.Error("ENXIO 후 Available() 이 여전히 true (false 예상)")
	}
}

// mockTimeoutError 는 net.Error 인터페이스를 구현하는 타임아웃 에러이다.
type mockTimeoutError struct {
	msg       string
	isTimeout bool
}

func (e *mockTimeoutError) Error() string   { return e.msg }
func (e *mockTimeoutError) Timeout() bool   { return e.isTimeout }
func (e *mockTimeoutError) Temporary() bool { return e.isTimeout }

// ===========================================================================
// tcp_host / tcp_port 분리 필드 검증 (리팩토링 특성화 테스트)
// ===========================================================================

// TestNewTCPTransport_RequiresHost 는 tcp_host 가 비어 있을 때
// ErrTCPHostRequired 를 반환하는지 검증한다.
func TestNewTCPTransport_RequiresHost(t *testing.T) {
	t.Run("missing host", func(t *testing.T) {
		_, err := NewNasaTransport("tcp-client", map[string]any{
			"tcp_port": 4196,
		})
		if !errors.Is(err, ErrTCPHostRequired) {
			t.Fatalf("NewNasaTransport(tcp, host 누락) error = %v, want %v", err, ErrTCPHostRequired)
		}
	})

	t.Run("empty host", func(t *testing.T) {
		_, err := NewNasaTransport("tcp-client", map[string]any{
			"tcp_host": "",
			"tcp_port": 4196,
		})
		if !errors.Is(err, ErrTCPHostRequired) {
			t.Fatalf("NewNasaTransport(tcp, host 빈문자열) error = %v, want %v", err, ErrTCPHostRequired)
		}
	})
}

// TestNewTCPTransport_RequiresPort 는 tcp_port 가 비어 있거나 0 일 때
// ErrTCPPortRequired 를 반환하는지 검증한다.
// (포트 0 은 원격 서비스 연결에 유효하지 않으므로 "누락"으로 취급한다.)
func TestNewTCPTransport_RequiresPort(t *testing.T) {
	t.Run("missing port", func(t *testing.T) {
		_, err := NewNasaTransport("tcp-client", map[string]any{
			"tcp_host": "10.0.0.5",
		})
		if !errors.Is(err, ErrTCPPortRequired) {
			t.Fatalf("NewNasaTransport(tcp, port 누락) error = %v, want %v", err, ErrTCPPortRequired)
		}
	})

	t.Run("zero port", func(t *testing.T) {
		_, err := NewNasaTransport("tcp-client", map[string]any{
			"tcp_host": "10.0.0.5",
			"tcp_port": 0,
		})
		if !errors.Is(err, ErrTCPPortRequired) {
			t.Fatalf("NewNasaTransport(tcp, port=0) error = %v, want %v", err, ErrTCPPortRequired)
		}
	})
}

// TestNewTCPTransport_ComposesAddress 는 tcp_host + tcp_port 로부터
// "host:port" 형식의 address 가 합성되는지 검증한다.
func TestNewTCPTransport_ComposesAddress(t *testing.T) {
	tr, err := NewNasaTransport("tcp-client", map[string]any{
		"tcp_host": "10.0.0.5",
		"tcp_port": 4196,
	})
	if err != nil {
		t.Fatalf("NewNasaTransport() unexpected error: %v", err)
	}

	tt, ok := tr.(*NasaTCPTransport)
	if !ok {
		t.Fatalf("expected *NasaTCPTransport, got %T", tr)
	}

	if tt.address != "10.0.0.5:4196" {
		t.Errorf("address = %q, want %q", tt.address, "10.0.0.5:4196")
	}
}

// ===========================================================================
// TCP server transport tests (2026-05-29)
// ===========================================================================

// TestTCPServerTransport_DefaultBindHost 는 tcp_host 미지정 시 "0.0.0.0" 가 기본값으로 적용됨을 검증한다.
func TestTCPServerTransport_DefaultBindHost(t *testing.T) {
	tr, err := NewNasaTransport("tcp-server", map[string]any{
		"tcp_port": 4197,
	})
	if err != nil {
		t.Fatalf("NewNasaTransport(tcp-server) unexpected error: %v", err)
	}
	ts := tr.(*NasaTCPServerTransport)
	if ts.host != "0.0.0.0" {
		t.Errorf("host = %q, want %q", ts.host, "0.0.0.0")
	}
	if ts.port != 4197 {
		t.Errorf("port = %d, want 4197", ts.port)
	}
}

// TestTCPServerTransport_AvailableBeforeOpen 는 Open 전 Available 이 false 임을 검증한다.
func TestTCPServerTransport_AvailableBeforeOpen(t *testing.T) {
	tr, _ := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4198,
	})
	if tr.Available() {
		t.Error("Available() = true before Open(), want false")
	}
}

// TestTCPServerTransport_SendNotOpen 는 Open 전 Send 가 ErrTransportNotConnected 를 반환하는지 검증한다.
func TestTCPServerTransport_SendNotOpen(t *testing.T) {
	tr, _ := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4199,
	})
	if err := tr.Send([]byte{0x01}); !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Send() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// TestTCPServerTransport_ReceiveNotOpen 는 Open 전 Receive 가 ErrTransportNotConnected 를 반환하는지 검증한다.
func TestTCPServerTransport_ReceiveNotOpen(t *testing.T) {
	tr, _ := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4200,
	})
	buf := make([]byte, 64)
	if _, err := tr.Receive(buf); !errors.Is(err, ErrTransportNotConnected) {
		t.Fatalf("Receive() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// TestTCPServerTransport_OpenCloseCycle 는 Open → Available=true → Close → Available=false 사이클을 검증한다.
// port=0 으로 OS 가 임의 port 를 할당하도록 한다.
func TestTCPServerTransport_OpenCloseCycle(t *testing.T) {
	tr, err := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4201, // 고정 포트
	})
	if err != nil {
		t.Fatalf("NewNasaTransport() unexpected error: %v", err)
	}
	ts := tr.(*NasaTCPServerTransport)

	if err := ts.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !ts.Available() {
		t.Error("Available() = false after Open(), want true")
	}
	if ts.Addr() == nil {
		t.Error("Addr() = nil after Open()")
	}

	if err := ts.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if ts.Available() {
		t.Error("Available() = true after Close(), want false")
	}
}

// TestTCPServerTransport_AcceptAndReceive 는 클라이언트가 접속하여 데이터를 보내면
// transport.Receive 로 수신되는지 검증한다.
func TestTCPServerTransport_AcceptAndReceive(t *testing.T) {
	tr, err := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4202,
	})
	if err != nil {
		t.Fatalf("NewNasaTransport() unexpected error: %v", err)
	}
	ts := tr.(*NasaTCPServerTransport)

	if err := ts.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer ts.Close()

	// 클라이언트 다이얼.
	addr := ts.Addr().String()
	client, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("client Dial() error = %v", err)
	}
	defer client.Close()

	// acceptLoop 가 conn 을 채택할 시간을 짧게 부여.
	if _, err := client.Write([]byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatalf("client Write() error = %v", err)
	}

	// Receive 가 클라이언트가 보낸 데이터를 받을 때까지 폴링.
	buf := make([]byte, 64)
	deadline := time.Now().Add(2 * time.Second)
	var (
		gotN   int
		gotErr error
	)
	for time.Now().Before(deadline) {
		n, e := ts.Receive(buf)
		if n > 0 {
			gotN = n
			gotErr = e
			break
		}
		// 짧게 sleep — acceptLoop 가 conn 등록할 시간을 준다.
		time.Sleep(5 * time.Millisecond)
	}

	if gotN == 0 {
		t.Fatalf("Receive() never returned data (last err=%v)", gotErr)
	}
	if !bytes.Equal(buf[:gotN], []byte{0xAA, 0xBB, 0xCC}) {
		t.Errorf("received = %x, want %x", buf[:gotN], []byte{0xAA, 0xBB, 0xCC})
	}
}

// TestTCPServerTransport_SendToClient 는 클라이언트가 접속한 후 Send 가
// preamble + data 를 클라이언트에게 전달함을 검증한다.
func TestTCPServerTransport_SendToClient(t *testing.T) {
	tr, err := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4203,
	})
	if err != nil {
		t.Fatalf("NewNasaTransport() unexpected error: %v", err)
	}
	ts := tr.(*NasaTCPServerTransport)

	if err := ts.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer ts.Close()

	addr := ts.Addr().String()
	client, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("client Dial() error = %v", err)
	}
	defer client.Close()

	// acceptLoop 가 conn 등록할 시간을 부여.
	// Send 가 ErrTransportNotConnected 가 아닌 정상 전송이 될 때까지 잠깐 재시도.
	data := []byte{0x32, 0x00, 0x10, 0x34}
	deadline := time.Now().Add(2 * time.Second)
	var sendErr error
	for time.Now().Before(deadline) {
		sendErr = ts.Send(data)
		if sendErr == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sendErr != nil {
		t.Fatalf("Send() error = %v", sendErr)
	}

	// client 가 preamble + data 를 읽는지 확인.
	expected := prependPreamble(data)
	clientBuf := make([]byte, len(expected))
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(client, clientBuf); err != nil {
		t.Fatalf("client Read() error = %v", err)
	}
	if !bytes.Equal(clientBuf, expected) {
		t.Errorf("client received = %x (len %d), want preamble + data (len %d)",
			clientBuf, len(clientBuf), len(expected))
	}
}

// TestTCPServerTransport_ReplacesActiveConnection 는 두 번째 클라이언트가 접속하면
// 기존 conn 이 교체되는지 검증한다 (LG 패턴).
func TestTCPServerTransport_ReplacesActiveConnection(t *testing.T) {
	tr, err := NewNasaTransport("tcp-server", map[string]any{
		"tcp_host": "127.0.0.1",
		"tcp_port": 4204,
	})
	if err != nil {
		t.Fatalf("NewNasaTransport() unexpected error: %v", err)
	}
	ts := tr.(*NasaTCPServerTransport)

	if err := ts.Open(); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer ts.Close()

	addr := ts.Addr().String()

	// 첫 번째 클라이언트 접속.
	client1, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("client1 Dial() error = %v", err)
	}
	defer client1.Close()

	// acceptLoop 가 첫 conn 등록 대기.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		hasConn := ts.conn != nil
		ts.mu.Unlock()
		if hasConn {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// 두 번째 클라이언트 접속 — 기존 conn 을 교체해야 함.
	client2, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("client2 Dial() error = %v", err)
	}
	defer client2.Close()

	// client2 가 받아들여진 후 데이터 전송하면 Receive 가 새 conn 으로부터 읽어야 함.
	if _, err := client2.Write([]byte{0x11, 0x22, 0x33}); err != nil {
		t.Fatalf("client2 Write() error = %v", err)
	}

	// client1 은 닫혀야 한다 (read 가 EOF 또는 reset). client2 의 데이터가 Receive 로 들어와야 한다.
	buf := make([]byte, 64)
	deadline = time.Now().Add(2 * time.Second)
	var gotN int
	for time.Now().Before(deadline) {
		n, _ := ts.Receive(buf)
		if n > 0 {
			gotN = n
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if gotN == 0 || !bytes.Equal(buf[:gotN], []byte{0x11, 0x22, 0x33}) {
		t.Errorf("expected to receive client2 data, got n=%d data=%x", gotN, buf[:gotN])
	}
}
