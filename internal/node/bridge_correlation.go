package node

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// correlationEntry 는 단일 상관관계 추적 항목을 나타내는 구조체이다.
type correlationEntry struct {
	responseCh chan message.Message // 응답을 수신할 채널
	createdAt  time.Time           // 항목 생성 시각
	timeout    time.Duration       // 이 항목의 타임아웃 기간
}

// CorrelationTracker 는 요청-응답 패턴의 상관관계를 추적하는 구조체이다.
// 각 요청에 대해 correlationID로 등록하고, 응답이 도착하면 해당 채널로 전달한다.
// 만료된 항목은 주기적으로 정리된다.
type CorrelationTracker struct {
	entries    sync.Map      // map[string]*correlationEntry
	timeout    time.Duration // 기본 타임아웃 기간
	timeoutMu  sync.RWMutex  // timeout 필드 보호
	cancelFn   context.CancelFunc
	timeoutCnt atomic.Int64 // 타임아웃된 항목 누적 카운트
	closed     atomic.Bool  // Close 중복 호출 방지
}

// NewCorrelationTracker 는 지정된 기본 타임아웃으로 새로운 CorrelationTracker를 생성한다.
func NewCorrelationTracker(timeout time.Duration) *CorrelationTracker {
	return &CorrelationTracker{
		timeout: timeout,
	}
}

// Track 은 주어진 correlationID에 대한 응답 채널을 등록한다.
// 동일한 ID가 이미 존재하면 덮어쓴다.
func (t *CorrelationTracker) Track(correlationID string, responseCh chan message.Message) {
	t.timeoutMu.RLock()
	timeout := t.timeout
	t.timeoutMu.RUnlock()

	entry := &correlationEntry{
		responseCh: responseCh,
		createdAt:  time.Now(),
		timeout:    timeout,
	}
	t.entries.Store(correlationID, entry)
}

// Resolve 는 주어진 correlationID에 대한 응답을 전달하고, 항목을 제거한다.
// 해당 ID가 존재하고 응답이 전달되면 true를, 그렇지 않으면 false를 반환한다.
func (t *CorrelationTracker) Resolve(correlationID string, response message.Message) bool {
	val, loaded := t.entries.LoadAndDelete(correlationID)
	if !loaded {
		return false
	}

	entry := val.(*correlationEntry)
	select {
	case entry.responseCh <- response:
		return true
	default:
		// 채널 버퍼가 가득 찬 경우 - 이미 타임아웃 되었거나 닫힌 경우
		return false
	}
}

// SetTimeout 은 새로 등록되는 항목에 적용할 기본 타임아웃을 변경한다.
// 이미 등록된 항목의 타임아웃은 변경되지 않는다.
func (t *CorrelationTracker) SetTimeout(d time.Duration) {
	t.timeoutMu.Lock()
	t.timeout = d
	t.timeoutMu.Unlock()
}

// Cleanup 은 만료된 항목을 제거하고, 해당 채널을 닫는다.
// 타임아웃된 항목은 timeoutCnt에 누적된다.
func (t *CorrelationTracker) Cleanup() {
	now := time.Now()
	t.entries.Range(func(key, value any) bool {
		entry := value.(*correlationEntry)
		if now.Sub(entry.createdAt) > entry.timeout {
			// 만료된 항목 제거
			if _, loaded := t.entries.LoadAndDelete(key); loaded {
				t.timeoutCnt.Add(1)
				close(entry.responseCh)
			}
		}
		return true
	})
}

// PendingCount 는 현재 추적 중인 항목 수를 반환한다.
func (t *CorrelationTracker) PendingCount() int {
	count := 0
	t.entries.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// TimeoutCount 는 누적 타임아웃 횟수를 반환한다.
func (t *CorrelationTracker) TimeoutCount() int64 {
	return t.timeoutCnt.Load()
}

// Close 는 모든 대기 중인 채널을 닫고 항목을 제거한다.
// 여러 번 호출해도 안전하다.
func (t *CorrelationTracker) Close() {
	if !t.closed.CompareAndSwap(false, true) {
		return // 이미 닫힘
	}

	if t.cancelFn != nil {
		t.cancelFn()
	}

	t.entries.Range(func(key, value any) bool {
		entry := value.(*correlationEntry)
		t.entries.Delete(key)
		close(entry.responseCh)
		return true
	})
}

// StartCleanupLoop 은 주어진 컨텍스트가 유효한 동안 주기적으로 Cleanup을 실행하는 고루틴을 시작한다.
// 클린업 간격은 timeout/2 이며, 최소 10밀리초이다.
func (t *CorrelationTracker) StartCleanupLoop(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	t.cancelFn = cancel

	t.timeoutMu.RLock()
	interval := t.timeout / 2
	t.timeoutMu.RUnlock()

	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.Cleanup()
			}
		}
	}()
}
