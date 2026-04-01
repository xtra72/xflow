package socket

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionInfo 는 하나의 TCP 연결 정보를 나타낸다.
type ConnectionInfo struct {
	RemoteAddr    string       // 원격 주소 (host:port)
	ConnectedAt   time.Time    // 연결 시각
	BytesSent     atomic.Int64 // 송신 바이트 수
	BytesReceived atomic.Int64 // 수신 바이트 수
	conn          net.Conn     // 내부 관리용 (비공개)
}

// ConnectionManager 는 TCP 연결을 추적하고 관리하는 인터페이스이다.
type ConnectionManager interface {
	// Add 는 새 연결을 추가한다. 최대 연결 수 초과 시 ErrMaxConnections 반환.
	Add(conn net.Conn) error
	// Remove 는 주소로 연결을 제거한다.
	Remove(remoteAddr string) error
	// Get 은 주소로 연결 정보를 조회한다.
	Get(remoteAddr string) (*ConnectionInfo, bool)
	// Block 은 주소를 차단한다. 기존 연결이 있으면 닫고 차단 목록에 추가.
	Block(remoteAddr string) error
	// Unblock 은 주소 차단을 해제한다.
	Unblock(remoteAddr string) error
	// IsBlocked 는 주소가 차단되었는지 확인한다 (IP 기준).
	IsBlocked(remoteAddr string) bool
	// List 는 모든 활성 연결 정보를 반환한다.
	List() []ConnectionInfo
	// BlockedList 는 차단된 IP 목록을 반환한다.
	BlockedList() []string
	// Count 는 활성 연결 수를 반환한다.
	Count() int
	// CloseAll 은 모든 연결을 닫는다.
	CloseAll() error
	// SetMaxConnections 는 최대 연결 수를 변경한다.
	SetMaxConnections(max int)
}

// connManager 는 ConnectionManager 의 기본 구현체이다.
type connManager struct {
	mu             sync.RWMutex
	connections    map[string]*ConnectionInfo
	blocked        map[string]bool // IP 기준 차단 맵
	maxConnections int
}

// NewConnectionManager 는 새 ConnectionManager 를 생성한다.
// maxConnections 가 0이면 무제한.
func NewConnectionManager(maxConnections int) ConnectionManager {
	return &connManager{
		connections:    make(map[string]*ConnectionInfo),
		blocked:        make(map[string]bool),
		maxConnections: maxConnections,
	}
}

func (m *connManager) Add(conn net.Conn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.maxConnections > 0 && len(m.connections) >= m.maxConnections {
		return ErrMaxConnections
	}

	addr := conn.RemoteAddr().String()
	m.connections[addr] = &ConnectionInfo{
		RemoteAddr:  addr,
		ConnectedAt: time.Now(),
		conn:        conn,
	}
	return nil
}

func (m *connManager) Remove(remoteAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.connections, remoteAddr)
	return nil
}

func (m *connManager) Get(remoteAddr string) (*ConnectionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.connections[remoteAddr]
	return info, ok
}

func (m *connManager) Block(remoteAddr string) error {
	host := extractHost(remoteAddr)

	m.mu.Lock()
	defer m.mu.Unlock()

	// 기존 연결 중 해당 IP 의 연결을 모두 닫고 제거
	for addr, info := range m.connections {
		if extractHost(addr) == host {
			_ = info.conn.Close()
			delete(m.connections, addr)
		}
	}

	m.blocked[host] = true
	return nil
}

func (m *connManager) Unblock(remoteAddr string) error {
	host := extractHost(remoteAddr)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.blocked, host)
	return nil
}

func (m *connManager) IsBlocked(remoteAddr string) bool {
	host := extractHost(remoteAddr)

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.blocked[host]
}

func (m *connManager) List() []ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]ConnectionInfo, 0, len(m.connections))
	for _, info := range m.connections {
		list = append(list, ConnectionInfo{
			RemoteAddr:  info.RemoteAddr,
			ConnectedAt: info.ConnectedAt,
		})
	}
	return list
}

func (m *connManager) BlockedList() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]string, 0, len(m.blocked))
	for ip := range m.blocked {
		list = append(list, ip)
	}
	return list
}

func (m *connManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.connections)
}

func (m *connManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for addr, info := range m.connections {
		if err := info.conn.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(m.connections, addr)
	}

	return errors.Join(errs...)
}

func (m *connManager) SetMaxConnections(max int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.maxConnections = max
}

// extractHost 는 "host:port" 형식에서 host 부분만 추출한다.
// SplitHostPort 실패 시 원본 주소를 그대로 반환한다.
func extractHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
