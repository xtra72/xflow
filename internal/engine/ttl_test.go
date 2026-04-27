package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// REQ-06-01: Wire delivery TTL check
// ---------------------------------------------------------------------------

func TestIsExpired_MessageWithinTTL(t *testing.T) {
	// TTL 이내의 메시지는 만료되지 않아야 한다.
	msg := message.New()
	expired := IsExpired(msg, 5*time.Second)
	assert.False(t, expired)
}

func TestIsExpired_MessageExceededTTL(t *testing.T) {
	// TTL을 초과한 메시지는 만료되어야 한다.
	// 과거 타임스탬프를 가진 메시지를 시뮬레이션하기 위해 짧은 TTL 사용
	msg := message.New()
	// 아주 짧은 TTL로 만료를 강제
	time.Sleep(2 * time.Millisecond)
	expired := IsExpired(msg, 1*time.Millisecond)
	assert.True(t, expired)
}

func TestIsExpired_ZeroTTL_NeverExpires(t *testing.T) {
	// TTL이 0이면 만료되지 않아야 한다 (TTL 미설정 의미).
	msg := message.New()
	expired := IsExpired(msg, 0)
	assert.False(t, expired)
}

// ---------------------------------------------------------------------------
// REQ-06-04: Skip TTL check for bypass (unbuffered) mode
// ---------------------------------------------------------------------------

func TestShouldCheckTTL_BufferMode(t *testing.T) {
	// 버퍼 모드 와이어는 TTL 검사를 해야 한다.
	wire := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBuffer,
		TTL:  5 * time.Second,
		Ch:   make(chan message.Message, 10),
	}
	assert.True(t, ShouldCheckTTL(wire))
}

func TestShouldCheckTTL_BypassMode(t *testing.T) {
	// 바이패스(언버퍼) 모드 와이어는 TTL 검사를 건너뛰어야 한다.
	wire := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBypass,
		TTL:  5 * time.Second,
		Ch:   make(chan message.Message),
	}
	assert.False(t, ShouldCheckTTL(wire))
}

func TestShouldCheckTTL_ZeroTTL(t *testing.T) {
	// TTL이 0인 와이어는 TTL 검사를 건너뛰어야 한다.
	wire := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBuffer,
		TTL:  0,
		Ch:   make(chan message.Message, 10),
	}
	assert.False(t, ShouldCheckTTL(wire))
}

// ---------------------------------------------------------------------------
// REQ-06-02: Expired message Dead Letter routing
// ---------------------------------------------------------------------------

// mockDeadLetterRouter 는 테스트용 DeadLetterRouter 구현이다.
type mockDeadLetterRouter struct {
	mu       sync.Mutex
	messages []DeadLetterEntry
}

func (m *mockDeadLetterRouter) Route(entry DeadLetterEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, entry)
	return nil
}

func (m *mockDeadLetterRouter) getMessages() []DeadLetterEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]DeadLetterEntry, len(m.messages))
	copy(cp, m.messages)
	return cp
}

func TestDeadLetterEntry_Fields(t *testing.T) {
	// DeadLetterEntry 구조체가 올바른 필드를 가지는지 검증한다.
	msg := message.New()
	entry := DeadLetterEntry{
		Message:  msg,
		Reason:   "ttl_expired",
		WireID:   "w1",
		ExpiredAt: time.Now(),
	}
	assert.Equal(t, msg.ID(), entry.Message.ID())
	assert.Equal(t, "ttl_expired", entry.Reason)
	assert.Equal(t, "w1", entry.WireID)
}

func TestHandleExpiredMessage_WithDeadLetterRouter(t *testing.T) {
	// Dead Letter 라우터가 설정된 경우 만료 메시지를 라우팅해야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(100*time.Millisecond),
	)

	msg := message.New()
	scanner.HandleExpiredMessage(msg, "w1")

	entries := dlr.getMessages()
	require.Len(t, entries, 1)
	assert.Equal(t, msg.ID(), entries[0].Message.ID())
	assert.Equal(t, "ttl_expired", entries[0].Reason)
	assert.Equal(t, "w1", entries[0].WireID)
}

func TestHandleExpiredMessage_WithoutDeadLetterRouter(t *testing.T) {
	// Dead Letter 라우터가 없으면 자동 폐기하고 메트릭만 기록해야 한다.
	scanner := NewTTLScanner(
		WithScanInterval(100*time.Millisecond),
	)

	msg := message.New()
	scanner.HandleExpiredMessage(msg, "w1")

	// 폐기 카운트가 증가해야 한다.
	assert.Equal(t, int64(1), scanner.DiscardedCount())
}

func TestHandleExpiredMessage_Metrics(t *testing.T) {
	// 만료 메시지 처리 시 메트릭이 기록되어야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(100*time.Millisecond),
	)

	msg1 := message.New()
	msg2 := message.New()
	scanner.HandleExpiredMessage(msg1, "w1")
	scanner.HandleExpiredMessage(msg2, "w2")

	assert.Equal(t, int64(2), scanner.ExpiredCount())
}

// ---------------------------------------------------------------------------
// REQ-06-05: TTLScanner struct
// ---------------------------------------------------------------------------

func TestNewTTLScanner_Defaults(t *testing.T) {
	// 기본값으로 TTLScanner가 생성되어야 한다.
	scanner := NewTTLScanner()
	assert.NotNil(t, scanner)
	assert.Equal(t, DefaultScanInterval, scanner.ScanInterval())
	assert.Equal(t, int64(0), scanner.ExpiredCount())
	assert.Equal(t, int64(0), scanner.DiscardedCount())
}

func TestNewTTLScanner_WithOptions(t *testing.T) {
	// 옵션을 적용한 TTLScanner가 올바르게 생성되어야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(500*time.Millisecond),
	)

	assert.Equal(t, 500*time.Millisecond, scanner.ScanInterval())
}

func TestTTLScanner_AddRemoveWire(t *testing.T) {
	// 와이어를 추가하고 제거할 수 있어야 한다.
	scanner := NewTTLScanner()

	wire1 := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBuffer,
		TTL:  5 * time.Second,
		Ch:   make(chan message.Message, 10),
	}
	wire2 := &RuntimeWire{
		ID:   "w2",
		Mode: flow.WireBuffer,
		TTL:  3 * time.Second,
		Ch:   make(chan message.Message, 5),
	}

	scanner.AddWire(wire1)
	scanner.AddWire(wire2)
	assert.Equal(t, 2, scanner.WireCount())

	scanner.RemoveWire("w1")
	assert.Equal(t, 1, scanner.WireCount())

	scanner.RemoveWire("w2")
	assert.Equal(t, 0, scanner.WireCount())
}

func TestTTLScanner_AddWire_SkipsBypassMode(t *testing.T) {
	// 바이패스 모드 와이어는 추가해도 무시해야 한다.
	scanner := NewTTLScanner()

	wire := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBypass,
		TTL:  5 * time.Second,
		Ch:   make(chan message.Message),
	}

	scanner.AddWire(wire)
	assert.Equal(t, 0, scanner.WireCount())
}

func TestTTLScanner_AddWire_SkipsZeroTTL(t *testing.T) {
	// TTL이 0인 와이어는 추가해도 무시해야 한다.
	scanner := NewTTLScanner()

	wire := &RuntimeWire{
		ID:   "w1",
		Mode: flow.WireBuffer,
		TTL:  0,
		Ch:   make(chan message.Message, 10),
	}

	scanner.AddWire(wire)
	assert.Equal(t, 0, scanner.WireCount())
}

func TestTTLScanner_RemoveWire_NonExistent(t *testing.T) {
	// 존재하지 않는 와이어 제거는 안전하게 무시해야 한다.
	scanner := NewTTLScanner()
	scanner.RemoveWire("non_existent")
	assert.Equal(t, 0, scanner.WireCount())
}

// ---------------------------------------------------------------------------
// REQ-06-03: Periodic buffer scan
// ---------------------------------------------------------------------------

func TestTTLScanner_ScanOnce(t *testing.T) {
	// 단일 스캔에서 만료된 메시지를 발견하고 처리해야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(100*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        1 * time.Millisecond,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	// 채널에 메시지를 넣는다.
	msg := message.New()
	wire.Ch <- msg

	// TTL이 만료될 때까지 대기
	time.Sleep(5 * time.Millisecond)

	scanner.AddWire(wire)
	removed := scanner.ScanOnce()

	assert.Equal(t, 1, removed)
	assert.Equal(t, int64(1), scanner.ExpiredCount())

	entries := dlr.getMessages()
	require.Len(t, entries, 1)
	assert.Equal(t, msg.ID(), entries[0].Message.ID())
}

func TestTTLScanner_ScanOnce_KeepsValidMessages(t *testing.T) {
	// 유효한 메시지는 스캔 후에도 채널에 남아있어야 한다.
	scanner := NewTTLScanner(
		WithScanInterval(100*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        10 * time.Second,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	msg := message.New()
	wire.Ch <- msg

	scanner.AddWire(wire)
	removed := scanner.ScanOnce()

	assert.Equal(t, 0, removed)
	assert.Equal(t, 1, len(wire.Ch))
}

func TestTTLScanner_StartStop(t *testing.T) {
	// TTLScanner goroutine이 시작되고 정지되어야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(10*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        1 * time.Millisecond,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	// 만료될 메시지를 넣는다.
	msg := message.New()
	wire.Ch <- msg

	scanner.AddWire(wire)

	ctx, cancel := context.WithCancel(context.Background())
	scanner.Start(ctx)

	// 스캔이 실행될 시간을 준다.
	time.Sleep(50 * time.Millisecond)
	cancel()
	scanner.Wait()

	// 만료 메시지가 처리되었어야 한다.
	assert.Greater(t, scanner.ExpiredCount(), int64(0))
}

func TestTTLScanner_StartStop_ContextCancellation(t *testing.T) {
	// context 취소 시 goroutine이 정상 종료되어야 한다.
	scanner := NewTTLScanner(
		WithScanInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	scanner.Start(ctx)

	cancel()
	// Wait이 타임아웃 없이 반환되어야 한다.
	done := make(chan struct{})
	go func() {
		scanner.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 정상
	case <-time.After(1 * time.Second):
		t.Fatal("TTLScanner did not stop within timeout")
	}
}

func TestTTLScanner_ConcurrentAddRemove(t *testing.T) {
	// 동시 AddWire/RemoveWire가 race condition 없이 작동해야 한다.
	scanner := NewTTLScanner(
		WithScanInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	scanner.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wire := &RuntimeWire{
				ID:         "w" + string(rune('0'+idx)),
				Mode:       flow.WireBuffer,
				TTL:        5 * time.Second,
				BufferSize: 10,
				Ch:         make(chan message.Message, 10),
			}
			scanner.AddWire(wire)
			time.Sleep(5 * time.Millisecond)
			scanner.RemoveWire(wire.ID)
		}(i)
	}

	wg.Wait()
	cancel()
	scanner.Wait()
}

func TestTTLScanner_ScanOnce_MixedMessages(t *testing.T) {
	// 만료된 메시지와 유효한 메시지가 섞여 있을 때 만료된 것만 제거해야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(100*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        1 * time.Millisecond,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	// 만료될 메시지 먼저 넣기
	expiredMsg := message.New()
	wire.Ch <- expiredMsg

	time.Sleep(5 * time.Millisecond)

	// 유효한 메시지 넣기 (긴 TTL로 와이어 재설정)
	wire.TTL = 10 * time.Second
	validMsg := message.New()
	wire.Ch <- validMsg

	// 짧은 TTL로 다시 설정하여 첫 번째만 만료되도록
	wire.TTL = 3 * time.Millisecond

	scanner.AddWire(wire)
	removed := scanner.ScanOnce()

	// 만료된 메시지만 제거되어야 한다.
	assert.Equal(t, 1, removed)
	// 유효한 메시지는 채널에 남아있어야 한다.
	assert.Equal(t, 1, len(wire.Ch))
}

func TestTTLScanner_DiscardedCount_WithoutRouter(t *testing.T) {
	// Dead Letter 라우터 없이 만료 시 discarded 카운트가 증가해야 한다.
	scanner := NewTTLScanner(
		WithScanInterval(100*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        1 * time.Millisecond,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	msg := message.New()
	wire.Ch <- msg

	time.Sleep(5 * time.Millisecond)

	scanner.AddWire(wire)
	scanner.ScanOnce()

	assert.Equal(t, int64(1), scanner.ExpiredCount())
	assert.Equal(t, int64(1), scanner.DiscardedCount())
}

func TestTTLScanner_ExpiredCount_WithRouter(t *testing.T) {
	// Dead Letter 라우터가 있을 때 expired 카운트는 증가하지만 discarded는 증가하지 않아야 한다.
	dlr := &mockDeadLetterRouter{}
	scanner := NewTTLScanner(
		WithDeadLetterRouter(dlr),
		WithScanInterval(100*time.Millisecond),
	)

	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		TTL:        1 * time.Millisecond,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}

	msg := message.New()
	wire.Ch <- msg

	time.Sleep(5 * time.Millisecond)

	scanner.AddWire(wire)
	scanner.ScanOnce()

	assert.Equal(t, int64(1), scanner.ExpiredCount())
	assert.Equal(t, int64(0), scanner.DiscardedCount())
}
