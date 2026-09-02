package modbusserver

import (
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// ClientRegistry — 연결된 TCP 클라이언트(원격 마스터) 추적 (Clients 탭용)
// ---------------------------------------------------------------------------
//
// socket 패키지의 connManager(connection.go) 선례를 따라 RWMutex + 원격 주소 키
// map 으로 per-client 상태를 추적한다. RTU(시리얼 버스)는 per-client 개념이 없어
// 사용하지 않는다(TCP 전용).

// ClientInfo 는 하나의 TCP 클라이언트 연결 스냅샷이다.
type ClientInfo struct {
	RemoteAddr   string    // 원격 주소 (host:port)
	ConnectedAt  time.Time // 연결 시각
	UnitIDs      []byte    // 이 클라이언트가 접근한 unit_id 집합 (오름차순 스냅샷)
	RequestCount int64     // 처리한 요청 수
	LastSeen     time.Time // 마지막 요청 시각
}

// clientEntry 는 레지스트리 내부의 가변 per-client 상태이다.
// UnitIDs 는 중복 없는 집합으로 관리하기 위해 map 을 사용한다.
type clientEntry struct {
	remoteAddr   string
	connectedAt  time.Time
	unitIDs      map[byte]struct{}
	requestCount int64
	lastSeen     time.Time
}

// ClientRegistry 는 활성 TCP 클라이언트를 원격 주소로 키잉해 추적한다.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*clientEntry
}

// NewClientRegistry 는 빈 ClientRegistry 를 생성한다.
func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[string]*clientEntry)}
}

// Add 는 새 연결을 등록한다(accept 시점). 이미 존재하면 무시하여 연결 시각을 보존한다.
func (r *ClientRegistry) Add(remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[remoteAddr]; ok {
		return
	}
	now := time.Now()
	r.clients[remoteAddr] = &clientEntry{
		remoteAddr:  remoteAddr,
		connectedAt: now,
		unitIDs:     make(map[byte]struct{}),
		lastSeen:    now,
	}
}

// Remove 는 연결 종료 시 클라이언트를 제거한다.
func (r *ClientRegistry) Remove(remoteAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, remoteAddr)
}

// RecordAccess 는 요청 처리 시 접근한 unit_id 를 기록하고 RequestCount/LastSeen 을 갱신한다.
// accept 를 거치지 않은 경로(테스트 직접 호출 등)에서도 안전하도록 항목이 없으면 지연 생성한다.
func (r *ClientRegistry) RecordAccess(remoteAddr string, unitID byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.clients[remoteAddr]
	if !ok {
		now := time.Now()
		e = &clientEntry{
			remoteAddr:  remoteAddr,
			connectedAt: now,
			unitIDs:     make(map[byte]struct{}),
		}
		r.clients[remoteAddr] = e
	}
	e.unitIDs[unitID] = struct{}{}
	e.requestCount++
	e.lastSeen = time.Now()
}

// List 는 모든 활성 클라이언트의 스냅샷을 반환한다(각 클라이언트의 UnitIDs 는 오름차순).
func (r *ClientRegistry) List() []ClientInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ClientInfo, 0, len(r.clients))
	for _, e := range r.clients {
		ids := make([]byte, 0, len(e.unitIDs))
		for id := range e.unitIDs {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		out = append(out, ClientInfo{
			RemoteAddr:   e.remoteAddr,
			ConnectedAt:  e.connectedAt,
			UnitIDs:      ids,
			RequestCount: e.requestCount,
			LastSeen:     e.lastSeen,
		})
	}
	return out
}
