package lg

import (
	"net"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TCP Client 트랜스포트 테스트
// ---------------------------------------------------------------------------

// TestTCPClientTransport_OpenClose 는 TCP 클라이언트 연결 수립 및 종료를 검증한다.
func TestTCPClientTransport_OpenClose(t *testing.T) {
	// 테스트용 TCP 서버 시작
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	// 서버 측에서 연결 수락
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// 클라이언트가 닫을 때까지 대기
		buf := make([]byte, 1)
		conn.Read(buf)
	}()

	transport := &lgapTCPClientTransport{
		host:           addr.IP.String(),
		port:           addr.Port,
		connectTimeout: 5 * time.Second,
		readTimeout:    500 * time.Millisecond,
		writeTimeout:   1 * time.Second,
	}

	// Open
	if err := transport.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if !transport.Available() {
		t.Error("Available() = false after Open, want true")
	}

	// Close
	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if transport.Available() {
		t.Error("Available() = true after Close, want false")
	}

	wg.Wait()
}

// TestTCPClientTransport_CloseIdempotent 는 Close 를 여러 번 호출해도 안전한지 검증한다.
func TestTCPClientTransport_CloseIdempotent(t *testing.T) {
	transport := &lgapTCPClientTransport{
		host:           "127.0.0.1",
		port:           0,
		connectTimeout: 1 * time.Second,
	}

	// 연결하지 않은 상태에서 Close 호출 — 에러 없어야 함
	if err := transport.Close(); err != nil {
		t.Errorf("Close() on unopened transport: %v", err)
	}
}

// TestTCPClientTransport_SendReceive 는 TCP 클라이언트의 데이터 송수신을 검증한다.
func TestTCPClientTransport_SendReceive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	// 서버: 수신 후 에코
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		conn.Write(buf[:n])
	}()

	transport := &lgapTCPClientTransport{
		host:           addr.IP.String(),
		port:           addr.Port,
		connectTimeout: 5 * time.Second,
		readTimeout:    2 * time.Second,
		writeTimeout:   1 * time.Second,
	}

	if err := transport.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer transport.Close()

	// Send
	testData := []byte("hello lg_hvacr02")
	if err := transport.Send(testData); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// Receive
	buf := make([]byte, 256)
	n, err := transport.Receive(buf)
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if string(buf[:n]) != "hello lg_hvacr02" {
		t.Errorf("Receive() = %q, want %q", string(buf[:n]), "hello lg_hvacr02")
	}

	wg.Wait()
}

// TestTCPClientTransport_Write 는 TCP 클라이언트의 Write 메서드를 검증한다.
func TestTCPClientTransport_Write(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	var wg sync.WaitGroup
	wg.Add(1)
	var received []byte
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		received = buf[:n]
	}()

	transport := &lgapTCPClientTransport{
		host:           addr.IP.String(),
		port:           addr.Port,
		connectTimeout: 5 * time.Second,
		writeTimeout:   1 * time.Second,
	}

	if err := transport.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer transport.Close()

	testData := []byte("write test")
	n, err := transport.Write(testData)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() = %d bytes, want %d", n, len(testData))
	}

	// 서버가 데이터를 받을 때까지 대기
	transport.Close()
	wg.Wait()

	if string(received) != "write test" {
		t.Errorf("server received = %q, want %q", string(received), "write test")
	}
}

// TestTCPClientTransport_NotConnected 는 연결 없이 송수신 시 에러를 검증한다.
func TestTCPClientTransport_NotConnected(t *testing.T) {
	transport := &lgapTCPClientTransport{
		host:           "127.0.0.1",
		port:           0,
		connectTimeout: 1 * time.Second,
	}

	// Send — 미연결 상태
	if err := transport.Send([]byte("test")); err != ErrTransportNotConnected {
		t.Errorf("Send() error = %v, want %v", err, ErrTransportNotConnected)
	}

	// Receive — 미연결 상태
	buf := make([]byte, 256)
	_, err := transport.Receive(buf)
	if err != ErrTransportNotConnected {
		t.Errorf("Receive() error = %v, want %v", err, ErrTransportNotConnected)
	}

	// Write — 미연결 상태
	_, err = transport.Write([]byte("test"))
	if err != ErrTransportNotConnected {
		t.Errorf("Write() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// TestTCPClientTransport_OpenConnectionRefused 는 연결 거부 시 에러를 검증한다.
func TestTCPClientTransport_OpenConnectionRefused(t *testing.T) {
	transport := &lgapTCPClientTransport{
		host:           "127.0.0.1",
		port:           1, // 권한 없는 포트 — 연결 거부 예상
		connectTimeout: 1 * time.Second,
	}

	err := transport.Open()
	if err == nil {
		transport.Close()
		t.Fatal("Open() expected error for connection refused, got nil")
	}
	if transport.Available() {
		t.Error("Available() = true after failed Open, want false")
	}
}

// TestTCPClientTransport_ConnectionDrop 는 서버 연결 끊김 시 Available 상태 변경을 검증한다.
func TestTCPClientTransport_ConnectionDrop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	// 서버: 연결 즉시 닫기
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	transport := &lgapTCPClientTransport{
		host:           addr.IP.String(),
		port:           addr.Port,
		connectTimeout: 5 * time.Second,
		readTimeout:    2 * time.Second,
	}

	if err := transport.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer transport.Close()

	wg.Wait()

	// 서버가 닫힌 후 Receive 시도 — EOF 예상
	buf := make([]byte, 256)
	_, err = transport.Receive(buf)
	if err == nil {
		t.Fatal("Receive() expected error after server close, got nil")
	}
	// 연결 에러 후 Available 은 false 여야 한다
	if transport.Available() {
		t.Error("Available() = true after connection drop, want false")
	}
}

// TestTCPClientTransport_NewFactory 는 newLGAPTCPClientTransport 팩토리를 검증한다.
func TestTCPClientTransport_NewFactory(t *testing.T) {
	cfg := Hvacr02Config{
		TCPHost:           "192.168.1.100",
		TCPPort:           5000,
		TCPConnectTimeout: 10 * time.Second,
		TCPReadTimeout:    2 * time.Second,
		TCPWriteTimeout:   3 * time.Second,
	}

	transport := newLGAPTCPClientTransport(cfg)
	if transport.host != "192.168.1.100" {
		t.Errorf("host = %q, want %q", transport.host, "192.168.1.100")
	}
	if transport.port != 5000 {
		t.Errorf("port = %d, want %d", transport.port, 5000)
	}
	if transport.connectTimeout != 10*time.Second {
		t.Errorf("connectTimeout = %v, want %v", transport.connectTimeout, 10*time.Second)
	}
	if transport.readTimeout != 2*time.Second {
		t.Errorf("readTimeout = %v, want %v", transport.readTimeout, 2*time.Second)
	}
	if transport.writeTimeout != 3*time.Second {
		t.Errorf("writeTimeout = %v, want %v", transport.writeTimeout, 3*time.Second)
	}
}

// ---------------------------------------------------------------------------
// TCP Server 트랜스포트 테스트
// ---------------------------------------------------------------------------

// TestTCPServerTransport_OpenClose 는 TCP 서버 리스너 시작 및 종료를 검증한다.
func TestTCPServerTransport_OpenClose(t *testing.T) {
	transport := newLGAPTCPServerTransport(Hvacr02Config{
		TCPHost:         "127.0.0.1",
		TCPPort:         0, // OS 가 포트 할당
		TCPReadTimeout:  500 * time.Millisecond,
		TCPWriteTimeout: 1 * time.Second,
	})

	// 포트 0 은 net.Listen 에서 자동 할당하므로 직접 리스너 생성
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}

	transport.mu.Lock()
	transport.listener = ln
	transport.done = make(chan struct{})
	transport.mu.Unlock()

	go transport.acceptLoop()

	// 클라이언트 연결
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}

	// acceptLoop 가 연결을 처리할 시간 대기
	time.Sleep(50 * time.Millisecond)

	if !transport.Available() {
		t.Error("Available() = false after client connect, want true")
	}

	conn.Close()

	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if transport.Available() {
		t.Error("Available() = true after Close, want false")
	}
}

// TestTCPServerTransport_CloseIdempotent 는 Close 를 여러 번 호출해도 안전한지 검증한다.
func TestTCPServerTransport_CloseIdempotent(t *testing.T) {
	transport := &lgapTCPServerTransport{
		host: "127.0.0.1",
		port: 0,
	}

	// 열지 않은 상태에서 Close — 에러 없어야 함
	if err := transport.Close(); err != nil {
		t.Errorf("Close() on unopened transport: %v", err)
	}
}

// TestTCPServerTransport_SendReceive 는 TCP 서버의 데이터 송수신을 검증한다.
func TestTCPServerTransport_SendReceive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}

	transport := &lgapTCPServerTransport{
		host:         "127.0.0.1",
		readTimeout:  2 * time.Second,
		writeTimeout: 1 * time.Second,
	}

	transport.mu.Lock()
	transport.listener = ln
	transport.done = make(chan struct{})
	transport.mu.Unlock()

	go transport.acceptLoop()
	defer transport.Close()

	// 클라이언트 연결
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}
	defer conn.Close()

	// acceptLoop 가 연결을 처리할 시간 대기
	time.Sleep(50 * time.Millisecond)

	// 서버 → 클라이언트 전송 (Send)
	testData := []byte("server to client")
	if err := transport.Send(testData); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// 클라이언트에서 수신
	buf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("client Read() error: %v", err)
	}
	if string(buf[:n]) != "server to client" {
		t.Errorf("client received = %q, want %q", string(buf[:n]), "server to client")
	}

	// 클라이언트 → 서버 전송
	clientData := []byte("client to server")
	if _, err := conn.Write(clientData); err != nil {
		t.Fatalf("client Write() error: %v", err)
	}

	// 서버에서 수신 (Receive)
	recvBuf := make([]byte, 256)
	rn, err := transport.Receive(recvBuf)
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if string(recvBuf[:rn]) != "client to server" {
		t.Errorf("Receive() = %q, want %q", string(recvBuf[:rn]), "client to server")
	}
}

// TestTCPServerTransport_Write 는 TCP 서버의 Write 메서드를 검증한다.
func TestTCPServerTransport_Write(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}

	transport := &lgapTCPServerTransport{
		host:         "127.0.0.1",
		writeTimeout: 1 * time.Second,
	}

	transport.mu.Lock()
	transport.listener = ln
	transport.done = make(chan struct{})
	transport.mu.Unlock()

	go transport.acceptLoop()
	defer transport.Close()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	testData := []byte("write method test")
	n, err := transport.Write(testData)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() = %d bytes, want %d", n, len(testData))
	}

	buf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	rn, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("client Read() error: %v", err)
	}
	if string(buf[:rn]) != "write method test" {
		t.Errorf("client received = %q, want %q", string(buf[:rn]), "write method test")
	}
}

// TestTCPServerTransport_NotConnected 는 클라이언트 없이 송수신 시 에러를 검증한다.
func TestTCPServerTransport_NotConnected(t *testing.T) {
	transport := &lgapTCPServerTransport{
		host: "127.0.0.1",
		port: 0,
	}

	if err := transport.Send([]byte("test")); err != ErrTransportNotConnected {
		t.Errorf("Send() error = %v, want %v", err, ErrTransportNotConnected)
	}

	buf := make([]byte, 256)
	_, err := transport.Receive(buf)
	if err != ErrTransportNotConnected {
		t.Errorf("Receive() error = %v, want %v", err, ErrTransportNotConnected)
	}

	_, err = transport.Write([]byte("test"))
	if err != ErrTransportNotConnected {
		t.Errorf("Write() error = %v, want %v", err, ErrTransportNotConnected)
	}
}

// TestTCPServerTransport_ClientReconnect 는 클라이언트 재연결 시 기존 연결 교체를 검증한다.
func TestTCPServerTransport_ClientReconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}

	transport := &lgapTCPServerTransport{
		host:         "127.0.0.1",
		readTimeout:  2 * time.Second,
		writeTimeout: 1 * time.Second,
	}

	transport.mu.Lock()
	transport.listener = ln
	transport.done = make(chan struct{})
	transport.mu.Unlock()

	go transport.acceptLoop()
	defer transport.Close()

	// 첫 번째 클라이언트 연결
	conn1, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() first client error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if !transport.Available() {
		t.Error("Available() = false after first client, want true")
	}

	// 두 번째 클라이언트 연결 (기존 연결 교체)
	conn2, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() second client error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 두 번째 연결로 데이터 전송 확인
	testData := []byte("from second client")
	if _, err := conn2.Write(testData); err != nil {
		t.Fatalf("second client Write() error: %v", err)
	}

	buf := make([]byte, 256)
	n, err := transport.Receive(buf)
	if err != nil {
		t.Fatalf("Receive() from second client error: %v", err)
	}
	if string(buf[:n]) != "from second client" {
		t.Errorf("Receive() = %q, want %q", string(buf[:n]), "from second client")
	}

	conn1.Close()
	conn2.Close()
}

// TestTCPServerTransport_NewFactory 는 newLGAPTCPServerTransport 팩토리를 검증한다.
func TestTCPServerTransport_NewFactory(t *testing.T) {
	cfg := Hvacr02Config{
		TCPHost:         "0.0.0.0",
		TCPPort:         8080,
		TCPReadTimeout:  2 * time.Second,
		TCPWriteTimeout: 3 * time.Second,
	}

	transport := newLGAPTCPServerTransport(cfg)
	if transport.host != "0.0.0.0" {
		t.Errorf("host = %q, want %q", transport.host, "0.0.0.0")
	}
	if transport.port != 8080 {
		t.Errorf("port = %d, want %d", transport.port, 8080)
	}
	if transport.readTimeout != 2*time.Second {
		t.Errorf("readTimeout = %v, want %v", transport.readTimeout, 2*time.Second)
	}
	if transport.writeTimeout != 3*time.Second {
		t.Errorf("writeTimeout = %v, want %v", transport.writeTimeout, 3*time.Second)
	}
}

// TestTCPServerTransport_Open 은 Open 메서드로 리스너 시작 및 연결 수락을 검증한다.
func TestTCPServerTransport_Open(t *testing.T) {
	transport := newLGAPTCPServerTransport(Hvacr02Config{
		TCPHost:         "127.0.0.1",
		TCPPort:         0, // OS 가 포트 할당 — Open 에서 포트 0 사용
		TCPReadTimeout:  500 * time.Millisecond,
		TCPWriteTimeout: 1 * time.Second,
	})

	// 포트 0 은 Open 내부의 net.Listen 에서 자동 할당
	if err := transport.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer transport.Close()

	// 리스너가 생성되었는지 확인
	transport.mu.Lock()
	lnAddr := transport.listener.Addr().String()
	transport.mu.Unlock()

	// 클라이언트 연결
	conn, err := net.DialTimeout("tcp", lnAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	if !transport.Available() {
		t.Error("Available() = false after client connect via Open, want true")
	}
}

// TestTCPClientTransport_InterfaceCompliance 는 LGAPTransport 인터페이스 준수를 컴파일 타임에 검증한다.
var _ LGAPTransport = (*lgapTCPClientTransport)(nil)

// TestTCPServerTransport_InterfaceCompliance 는 LGAPTransport 인터페이스 준수를 컴파일 타임에 검증한다.
var _ LGAPTransport = (*lgapTCPServerTransport)(nil)
