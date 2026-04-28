package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// DefaultScanInterval 은 TTLScanner 의 기본 스캔 간격이다.
const DefaultScanInterval = 1 * time.Second

// DeadLetterEntry 는 Dead Letter 라우터로 전달되는 만료 메시지 정보이다.
type DeadLetterEntry struct {
	Message   message.Message
	Reason    string
	WireID    string
	ExpiredAt time.Time
}

// DeadLetterRouter 는 만료된 메시지를 Dead Letter 노드로 라우팅하는 인터페이스이다.
type DeadLetterRouter interface {
	Route(entry DeadLetterEntry) error
}

// IsExpired 는 메시지가 지정된 TTL을 초과했는지 검사한다.
// TTL이 0이면 만료되지 않는다 (TTL 미설정).
func IsExpired(msg message.Message, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	return time.Since(msg.Timestamp()) > ttl
}

// ShouldCheckTTL 은 와이어에 대해 TTL 검사를 수행해야 하는지 판단한다.
// 바이패스(언버퍼) 모드이거나 TTL이 0이면 검사를 건너뛴다.
func ShouldCheckTTL(wire *RuntimeWire) bool {
	if wire.Mode == flow.WireBypass {
		return false
	}
	if wire.TTL <= 0 {
		return false
	}
	return true
}

// TTLScannerOption 은 TTLScanner 생성 시 적용할 수 있는 옵션 함수 타입이다.
type TTLScannerOption func(*TTLScanner)

// WithDeadLetterRouter 는 TTLScanner 에 Dead Letter 라우터를 설정하는 옵션을 반환한다.
func WithDeadLetterRouter(dlr DeadLetterRouter) TTLScannerOption {
	return func(s *TTLScanner) {
		s.deadLetterRouter = dlr
	}
}

// WithScanInterval 은 TTLScanner 의 스캔 간격을 설정하는 옵션을 반환한다.
func WithScanInterval(d time.Duration) TTLScannerOption {
	return func(s *TTLScanner) {
		s.scanInterval = d
	}
}

// TTLScanner 는 버퍼링된 와이어의 메시지 TTL을 주기적으로 검사하는 구조체이다.
type TTLScanner struct {
	mu               sync.RWMutex
	wires            map[string]*RuntimeWire
	scanInterval     time.Duration
	deadLetterRouter DeadLetterRouter
	expiredCount     atomic.Int64
	discardedCount   atomic.Int64
	wg               sync.WaitGroup
}

// NewTTLScanner 는 지정된 옵션으로 새로운 TTLScanner 를 생성한다.
func NewTTLScanner(opts ...TTLScannerOption) *TTLScanner {
	s := &TTLScanner{
		wires:        make(map[string]*RuntimeWire),
		scanInterval: DefaultScanInterval,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ScanInterval 은 현재 설정된 스캔 간격을 반환한다.
func (s *TTLScanner) ScanInterval() time.Duration {
	return s.scanInterval
}

// ExpiredCount 는 만료된 메시지 수를 반환한다.
func (s *TTLScanner) ExpiredCount() int64 {
	return s.expiredCount.Load()
}

// DiscardedCount 는 Dead Letter 없이 폐기된 메시지 수를 반환한다.
func (s *TTLScanner) DiscardedCount() int64 {
	return s.discardedCount.Load()
}

// WireCount 는 현재 등록된 와이어 수를 반환한다.
func (s *TTLScanner) WireCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.wires)
}

// AddWire 는 TTL 검사 대상 와이어를 추가한다.
// 바이패스 모드이거나 TTL이 0인 와이어는 무시한다.
func (s *TTLScanner) AddWire(wire *RuntimeWire) {
	if !ShouldCheckTTL(wire) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wires[wire.ID] = wire
}

// RemoveWire 는 TTL 검사 대상에서 와이어를 제거한다.
func (s *TTLScanner) RemoveWire(wireID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.wires, wireID)
}

// HandleExpiredMessage 는 만료된 메시지를 처리한다.
// Dead Letter 라우터가 설정되어 있으면 라우팅하고, 없으면 자동 폐기한다.
// 항상 만료 메트릭을 기록한다.
func (s *TTLScanner) HandleExpiredMessage(msg message.Message, wireID string) {
	s.expiredCount.Add(1)

	entry := DeadLetterEntry{
		Message:   msg,
		Reason:    "ttl_expired",
		WireID:    wireID,
		ExpiredAt: time.Now(),
	}

	if s.deadLetterRouter != nil {
		_ = s.deadLetterRouter.Route(entry)
	} else {
		s.discardedCount.Add(1)
	}
}

// ScanOnce 는 등록된 모든 와이어의 버퍼를 한 번 스캔하여 만료된 메시지를 제거한다.
// 제거된 메시지 수를 반환한다.
func (s *TTLScanner) ScanOnce() int {
	s.mu.RLock()
	// 와이어 목록 스냅샷
	wires := make([]*RuntimeWire, 0, len(s.wires))
	for _, w := range s.wires {
		wires = append(wires, w)
	}
	s.mu.RUnlock()

	totalRemoved := 0
	for _, wire := range wires {
		removed := s.scanWire(wire)
		totalRemoved += removed
	}
	return totalRemoved
}

// scanWire 는 단일 와이어의 버퍼를 스캔하여 만료된 메시지를 제거한다.
func (s *TTLScanner) scanWire(wire *RuntimeWire) int {
	if wire.IsClosed() {
		return 0
	}

	bufLen := len(wire.Ch)
	if bufLen == 0 {
		return 0
	}

	removed := 0
	kept := make([]message.Message, 0, bufLen)

	// 채널에서 메시지를 모두 꺼낸다.
	// drain 라벨로 명시적 break: 채널이 도중에 비면 빈 select 를 반복하지 않고 즉시 종료.
drain:
	for i := 0; i < bufLen; i++ {
		select {
		case msg := <-wire.Ch:
			if IsExpired(msg, wire.TTL) {
				s.HandleExpiredMessage(msg, wire.ID)
				removed++
			} else {
				kept = append(kept, msg)
			}
		default:
			break drain
		}
	}

	// 유효한 메시지를 다시 채널에 넣는다.
	for _, msg := range kept {
		select {
		case wire.Ch <- msg:
		default:
			// 채널이 가득 찼으면 폐기 (이론적으로 발생하지 않아야 함)
			s.HandleExpiredMessage(msg, wire.ID)
			removed++
		}
	}

	return removed
}

// Start 는 주기적 TTL 스캔 goroutine 을 시작한다.
// context 가 취소되면 goroutine 이 종료된다.
func (s *TTLScanner) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.scanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ScanOnce()
			}
		}
	}()
}

// Wait 는 TTLScanner goroutine 이 종료될 때까지 대기한다.
func (s *TTLScanner) Wait() {
	s.wg.Wait()
}
