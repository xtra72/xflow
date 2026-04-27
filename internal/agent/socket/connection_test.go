package socket

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
)

// tcpConnPair 는 테스트용 TCP 연결 쌍을 생성한다.
// 반환된 서버 측 연결은 고유한 RemoteAddr 을 가진다.
func tcpConnPair(t *testing.T, ln net.Listener) (serverConn, clientConn net.Conn) {
	t.Helper()
	done := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		done <- c
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial 오류: %v", err)
	}

	server := <-done
	return server, client
}

// --- ConnectionManager 기본 동작 테스트 ---

func TestConnectionManager_AddRemove(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)
	s, c := tcpConnPair(t, ln)
	defer s.Close()
	defer c.Close()

	if err := cm.Add(s); err != nil {
		t.Fatalf("Add 오류: %v", err)
	}
	if cm.Count() != 1 {
		t.Fatalf("Count: 기대값 1, 실제값 %d", cm.Count())
	}

	addr := s.RemoteAddr().String()
	info, ok := cm.Get(addr)
	if !ok {
		t.Fatal("Get: 연결을 찾지 못함")
	}
	if info.RemoteAddr != addr {
		t.Fatalf("RemoteAddr: 기대값 %q, 실제값 %q", addr, info.RemoteAddr)
	}

	if err := cm.Remove(addr); err != nil {
		t.Fatalf("Remove 오류: %v", err)
	}
	if cm.Count() != 0 {
		t.Fatalf("Remove 후 Count: 기대값 0, 실제값 %d", cm.Count())
	}
}

func TestConnectionManager_MaxConnections(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(2)

	conns := make([]net.Conn, 0, 6)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 연결 2개 추가 (제한치)
	for i := 0; i < 2; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		if err := cm.Add(s); err != nil {
			t.Fatalf("Add #%d 오류: %v", i, err)
		}
	}
	if cm.Count() != 2 {
		t.Fatalf("Count: 기대값 2, 실제값 %d", cm.Count())
	}

	// 3번째 연결 추가 시 ErrMaxConnections
	s3, c3 := tcpConnPair(t, ln)
	conns = append(conns, s3, c3)
	addErr := cm.Add(s3)
	if !errors.Is(addErr, ErrMaxConnections) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxConnections, addErr)
	}
}

func TestConnectionManager_UnlimitedConnections(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 20)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < 10; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		if err := cm.Add(s); err != nil {
			t.Fatalf("Add #%d 오류: %v", i, err)
		}
	}
	if cm.Count() != 10 {
		t.Fatalf("Count: 기대값 10, 실제값 %d", cm.Count())
	}
}

// --- 차단(Block) 테스트 ---

func TestConnectionManager_Block(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)
	s, c := tcpConnPair(t, ln)
	defer c.Close()

	if err := cm.Add(s); err != nil {
		t.Fatalf("Add 오류: %v", err)
	}

	addr := s.RemoteAddr().String()
	if err := cm.Block(addr); err != nil {
		t.Fatalf("Block 오류: %v", err)
	}

	// 차단 후 연결이 제거되어야 함
	if cm.Count() != 0 {
		t.Fatalf("Block 후 Count: 기대값 0, 실제값 %d", cm.Count())
	}

	// 차단 목록에 있어야 함
	if !cm.IsBlocked(addr) {
		t.Fatal("차단된 주소가 IsBlocked 에서 false 반환")
	}
}

func TestConnectionManager_IsBlocked_ByIPOnly(t *testing.T) {
	cm := NewConnectionManager(0)

	// IP 기반 차단: 포트 무관
	if err := cm.Block("192.168.1.100:5000"); err != nil {
		t.Fatalf("Block 오류: %v", err)
	}

	// 같은 IP, 다른 포트도 차단됨
	if !cm.IsBlocked("192.168.1.100:9999") {
		t.Fatal("같은 IP의 다른 포트가 차단되지 않음")
	}

	// 다른 IP는 차단되지 않음
	if cm.IsBlocked("192.168.1.200:5000") {
		t.Fatal("다른 IP가 잘못 차단됨")
	}
}

func TestConnectionManager_Unblock(t *testing.T) {
	cm := NewConnectionManager(0)

	if err := cm.Block("10.0.0.1:8080"); err != nil {
		t.Fatalf("Block 오류: %v", err)
	}
	if !cm.IsBlocked("10.0.0.1:8080") {
		t.Fatal("차단되지 않음")
	}

	if err := cm.Unblock("10.0.0.1:8080"); err != nil {
		t.Fatalf("Unblock 오류: %v", err)
	}
	if cm.IsBlocked("10.0.0.1:8080") {
		t.Fatal("해제 후에도 차단되어 있음")
	}
}

// --- List / BlockedList 테스트 ---

func TestConnectionManager_List(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 6)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < 3; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		if err := cm.Add(s); err != nil {
			t.Fatalf("Add #%d 오류: %v", i, err)
		}
	}

	list := cm.List()
	if len(list) != 3 {
		t.Fatalf("List 길이: 기대값 3, 실제값 %d", len(list))
	}
}

func TestConnectionManager_BlockedList(t *testing.T) {
	cm := NewConnectionManager(0)

	_ = cm.Block("1.1.1.1:80")
	_ = cm.Block("2.2.2.2:80")

	blocked := cm.BlockedList()
	if len(blocked) != 2 {
		t.Fatalf("BlockedList 길이: 기대값 2, 실제값 %d", len(blocked))
	}
}

// --- CloseAll 테스트 ---

func TestConnectionManager_CloseAll(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 6)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < 3; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		if err := cm.Add(s); err != nil {
			t.Fatalf("Add #%d 오류: %v", i, err)
		}
	}

	if err := cm.CloseAll(); err != nil {
		t.Fatalf("CloseAll 오류: %v", err)
	}
	if cm.Count() != 0 {
		t.Fatalf("CloseAll 후 Count: 기대값 0, 실제값 %d", cm.Count())
	}
}

// --- SetMaxConnections 테스트 ---

func TestConnectionManager_SetMaxConnections(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 8)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 먼저 3개 연결
	for i := 0; i < 3; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		_ = cm.Add(s)
	}

	// 제한을 3으로 설정
	cm.SetMaxConnections(3)

	// 4번째 연결 거부
	s4, c4 := tcpConnPair(t, ln)
	conns = append(conns, s4, c4)
	addErr := cm.Add(s4)
	if !errors.Is(addErr, ErrMaxConnections) {
		t.Fatalf("기대한 오류: %v, 실제: %v", ErrMaxConnections, addErr)
	}
}

// --- 동시성 안전성 테스트 ---

func TestConnectionManager_ConcurrentAccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	var wg sync.WaitGroup
	conns := make([]net.Conn, 0, 100)
	var mu sync.Mutex

	// 동시에 20개 연결 추가
	for i := 0; i < 20; i++ {
		s, c := tcpConnPair(t, ln)
		mu.Lock()
		conns = append(conns, s, c)
		mu.Unlock()

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			_ = cm.Add(conn)
		}(s)
	}
	wg.Wait()

	// 동시에 List 와 Count 호출
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = cm.List()
		}()
		go func() {
			defer wg.Done()
			_ = cm.Count()
		}()
	}
	wg.Wait()

	// 정리
	_ = cm.CloseAll()
	for _, c := range conns {
		c.Close()
	}
}

func TestConnectionManager_CountAccuracy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 10)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 5개 추가
	addrs := make([]string, 5)
	for i := 0; i < 5; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		_ = cm.Add(s)
		addrs[i] = s.RemoteAddr().String()
	}
	if cm.Count() != 5 {
		t.Fatalf("Count: 기대값 5, 실제값 %d", cm.Count())
	}

	// 2개 제거
	_ = cm.Remove(addrs[0])
	_ = cm.Remove(addrs[1])
	if cm.Count() != 3 {
		t.Fatalf("Count: 기대값 3, 실제값 %d", cm.Count())
	}

	// 1개 차단 (같은 IP이므로 모든 127.0.0.1 연결이 닫히는 것에 주의)
	// 개별 주소 차단 테스트를 위해 특정 주소 직접 제거 후 확인
	_ = cm.Remove(addrs[2])
	if cm.Count() != 2 {
		t.Fatalf("Count: 기대값 2, 실제값 %d", cm.Count())
	}
}

func TestConnectionManager_BlockClosesMultipleSameIP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cm := NewConnectionManager(0)

	conns := make([]net.Conn, 0, 6)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 같은 IP(127.0.0.1)에서 3개 연결
	for i := 0; i < 3; i++ {
		s, c := tcpConnPair(t, ln)
		conns = append(conns, s, c)
		_ = cm.Add(s)
	}
	if cm.Count() != 3 {
		t.Fatalf("Count: 기대값 3, 실제값 %d", cm.Count())
	}

	// 127.0.0.1 차단 -> 모든 연결 닫힘
	_ = cm.Block(fmt.Sprintf("127.0.0.1:%d", 12345))
	if cm.Count() != 0 {
		t.Fatalf("Block 후 Count: 기대값 0, 실제값 %d", cm.Count())
	}
}
