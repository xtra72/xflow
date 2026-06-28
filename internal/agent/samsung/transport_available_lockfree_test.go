package samsung

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Available() lock-free 회귀 테스트 (성능 버그 재현)
//
// 배경: `agent list` 가 ListAgents → agentToHandlerInfo → TransportConnected()
// → transport.Available() 경로로 트랜스포트 liveness 를 조회한다.
// Available() 이 Send/Receive 가 블로킹 I/O 를 수행하는 동안 잡고 있는 동일한
// mutex 를 획득하면, 진행 중인 시리얼/TCP I/O 가 완료될 때까지 Available() 이
// 블록된다 (= `agent list` 가 멈춘다).
//
// 아래 테스트들은 블로킹 I/O 가 mutex 를 잡고 있는 상황을 만들고 Available() 이
// 지정 시간(50ms) 내에 반드시 반환되는지 검증한다. lock-free 수정 전에는
// 타임아웃(블록)으로 실패하고, 수정 후에는 즉시 통과한다.
// ---------------------------------------------------------------------------

// blockingReadWriteCloser 는 Read 가 release 채널이 닫힐 때까지 무한정 블록되는
// io.ReadWriteCloser 이다. 시리얼 포트의 블로킹 read 를 결정적으로 재현한다.
type blockingReadWriteCloser struct {
	release   chan struct{} // 닫히면 Read 가 io.EOF 로 풀린다
	readEntry chan struct{} // Read 진입 시 1회 신호 (테스트 동기화용)
	once      sync.Once
}

func newBlockingReadWriteCloser() *blockingReadWriteCloser {
	return &blockingReadWriteCloser{
		release:   make(chan struct{}),
		readEntry: make(chan struct{}, 1),
	}
}

func (b *blockingReadWriteCloser) Read(p []byte) (int, error) {
	// Read 진입을 테스트에 알린다 (1회).
	b.once.Do(func() { close(b.readEntry) })
	<-b.release
	return 0, io.EOF
}

func (b *blockingReadWriteCloser) Write(p []byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

func (b *blockingReadWriteCloser) Close() error {
	return nil
}

func (b *blockingReadWriteCloser) unblock() {
	close(b.release)
}

// assertAvailableReturnsPromptly 는 available 콜백이 50ms 내에 반환되는지 검증한다.
// 블록되면(타임아웃) 테스트를 실패시킨다.
func assertAvailableReturnsPromptly(t *testing.T, available func() bool) {
	t.Helper()
	done := make(chan bool, 1)
	go func() {
		done <- available()
	}()
	select {
	case <-done:
		// 즉시 반환됨 — 정상.
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Available() 이 50ms 내에 반환되지 않음 — I/O 블로킹 중인 mutex 에 막힘 (lock-free 위반)")
	}
}

// TestSerialTransport_Available_LockFreeDuringReceive 는 Receive 가 블로킹 read 로
// 인해 내부 mutex 를 잡고 있는 동안에도 Available() 이 즉시 반환되는지 검증한다.
func TestSerialTransport_Available_LockFreeDuringReceive(t *testing.T) {
	conn := newBlockingReadWriteCloser()
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return conn, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{port: "/dev/test", baudRate: 9600, dataBits: 8, stopBits: 1, parity: "even"}
	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if !st.Available() {
		t.Fatal("Open 후 Available() = false")
	}

	// Receive 를 goroutine 에서 호출 → 블로킹 read 진입 (mutex 보유).
	recvDone := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		_, _ = st.Receive(buf)
		close(recvDone)
	}()

	// Read 진입 보장 (mutex 가 확실히 잡힌 상태).
	select {
	case <-conn.readEntry:
	case <-time.After(time.Second):
		t.Fatal("Receive 가 Read 에 진입하지 못함")
	}

	// 핵심 단언: 블로킹 I/O 진행 중 Available() 은 즉시 반환되어야 한다.
	assertAvailableReturnsPromptly(t, st.Available)

	// 정리: read 를 풀어 goroutine 종료.
	conn.unblock()
	<-recvDone
}

// TestSerialTransport_Available_LockFreeDuringSend 는 Send 가 블로킹 write 로 인해
// 내부 mutex 를 잡고 있는 동안에도 Available() 이 즉시 반환되는지 검증한다.
func TestSerialTransport_Available_LockFreeDuringSend(t *testing.T) {
	conn := newBlockingReadWriteCloser()
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return conn, nil
	}
	defer func() { SerialOpener = nil }()

	st := &NasaSerialTransport{port: "/dev/test", baudRate: 9600, dataBits: 8, stopBits: 1, parity: "even"}
	if err := st.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	sendDone := make(chan struct{})
	go func() {
		_ = st.Send([]byte{0x01, 0x02})
		close(sendDone)
	}()

	// Write 가 블록에 진입할 시간을 잠시 준다 (mutex 보유 보장).
	time.Sleep(10 * time.Millisecond)

	assertAvailableReturnsPromptly(t, st.Available)

	conn.unblock()
	<-sendDone
}

// TestTCPTransport_Available_LockFreeDuringSend 는 NasaTCPTransport.Send 가 블로킹
// write 로 인해 mutex 를 잡고 있는 동안에도 Available() 이 즉시 반환되는지 검증한다.
// net.Pipe 는 unbuffered 이므로 reader 가 없으면 Write 가 블록된다.
func TestTCPTransport_Available_LockFreeDuringSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tr := newTestTCPTransport(client)

	sendDone := make(chan struct{})
	go func() {
		// net.Pipe 는 unbuffered → 상대가 Read 하지 않으면 Write 가 블록된다.
		_ = tr.Send([]byte{0x01, 0x02, 0x03})
		close(sendDone)
	}()

	// Send 가 Write 블록에 진입할 시간을 준다 (mutex 보유 보장).
	time.Sleep(10 * time.Millisecond)

	assertAvailableReturnsPromptly(t, tr.Available)

	// 정리: server 쪽에서 read 하여 Write 를 풀어준다.
	go func() { _, _ = server.Read(make([]byte, 256)) }()
	select {
	case <-sendDone:
	case <-time.After(time.Second):
		// 정리 실패는 본 테스트 단언과 무관하나 leak 방지.
	}
}

// TestTCPServerTransport_Available_LockFreeDuringSend 는 NasaTCPServerTransport.Send
// 가 블로킹 write 로 mutex 를 잡고 있는 동안에도 Available() 이 즉시 반환되는지 검증한다.
func TestTCPServerTransport_Available_LockFreeDuringSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ts := newTestTCPServerTransport(client)

	sendDone := make(chan struct{})
	go func() {
		_ = ts.Send([]byte{0x01, 0x02, 0x03})
		close(sendDone)
	}()

	time.Sleep(10 * time.Millisecond)

	assertAvailableReturnsPromptly(t, ts.Available)

	go func() { _, _ = server.Read(make([]byte, 256)) }()
	select {
	case <-sendDone:
	case <-time.After(time.Second):
	}
}

// newTestTCPTransport 는 이미 "열린" 상태의 NasaTCPTransport 를 주어진 conn 으로 만든다.
// open 플래그 설정 방식은 프로덕션 필드 타입(현재 bool, 수정 후 atomic.Bool)에 맞춰
// 이 헬퍼 한 곳에서만 갱신하면 된다.
func newTestTCPTransport(conn net.Conn) *NasaTCPTransport {
	tr := &NasaTCPTransport{address: "pipe", readTimeout: 0, conn: conn}
	tr.markOpenForTest()
	return tr
}

// newTestTCPServerTransport 는 이미 "열린" 상태의 NasaTCPServerTransport 를 만든다.
func newTestTCPServerTransport(conn net.Conn) *NasaTCPServerTransport {
	ts := &NasaTCPServerTransport{
		host:        "127.0.0.1",
		port:        0,
		readTimeout: 0,
		logger:      noopServerLogger{},
		conn:        conn,
	}
	ts.markOpenForTest()
	return ts
}

// markOpenForTest 는 트랜스포트를 "열린" 상태로 만든다 (open 플래그 set).
// open 필드 타입 변경(bool → atomic.Bool) 시 이 두 메서드만 갱신하면 된다.
func (t *NasaTCPTransport) markOpenForTest()       { t.open.Store(true) }
func (s *NasaTCPServerTransport) markOpenForTest() { s.open.Store(true) }
