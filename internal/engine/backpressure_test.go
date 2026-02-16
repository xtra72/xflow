package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func TestBackpressureStrategy_Constants(t *testing.T) {
	// 백프레셔 전략 상수가 올바른지 검증한다.
	if StrategyBlock != "block" {
		t.Errorf("expected StrategyBlock to be 'block', got %q", StrategyBlock)
	}
	if StrategyDrop != "drop" {
		t.Errorf("expected StrategyDrop to be 'drop', got %q", StrategyDrop)
	}
}

func TestDropPolicy_Constants(t *testing.T) {
	// 드롭 정책 상수가 올바른지 검증한다.
	if DropNewest != "drop_newest" {
		t.Errorf("expected DropNewest to be 'drop_newest', got %q", DropNewest)
	}
	if DropOldest != "drop_oldest" {
		t.Errorf("expected DropOldest to be 'drop_oldest', got %q", DropOldest)
	}
}

func TestDefaultBackpressurePolicy(t *testing.T) {
	// 기본 백프레셔 정책이 올바른 기본값을 반환하는지 검증한다.
	policy := DefaultBackpressurePolicy()

	if policy.Strategy != StrategyBlock {
		t.Errorf("expected default strategy StrategyBlock, got %q", policy.Strategy)
	}
	if policy.BufferHighWaterMark != 0.8 {
		t.Errorf("expected default BufferHighWaterMark 0.8, got %f", policy.BufferHighWaterMark)
	}
	if policy.DropPolicy != DropNewest {
		t.Errorf("expected default DropPolicy DropNewest, got %q", policy.DropPolicy)
	}
}

// ---------------------------------------------------------------------------
// REQ-04-04: Block strategy - Go channel natural backpressure
// ---------------------------------------------------------------------------

func TestSendWithBackpressure_BlockStrategy_SendsNormally(t *testing.T) {
	// StrategyBlock 전략에서 버퍼에 여유가 있으면 정상 전송되어야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 5,
		Ch:         make(chan message.Message, 5),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}

	msg := message.New()
	ctx := context.Background()

	result, err := SendWithBackpressure(ctx, wire, msg, policy)
	require.NoError(t, err)
	assert.False(t, result.Dropped)
	assert.False(t, result.HighWaterMarkReached)

	// 메시지가 채널에 있어야 한다.
	received := <-wire.Ch
	assert.Equal(t, msg.ID(), received.ID())
}

func TestSendWithBackpressure_BlockStrategy_BlocksWhenFull(t *testing.T) {
	// StrategyBlock 전략에서 버퍼가 가득 차면 블로킹되어야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 1,
		Ch:         make(chan message.Message, 1),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}

	// 버퍼를 가득 채운다.
	wire.Ch <- message.New()

	// 두 번째 전송은 블로킹되어야 한다.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	msg := message.New()
	_, err := SendWithBackpressure(ctx, wire, msg, policy)

	// 컨텍스트 타임아웃으로 에러 발생
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// REQ-04-05: Drop strategy
// ---------------------------------------------------------------------------

func TestSendWithBackpressure_DropNewest_WhenBufferFull(t *testing.T) {
	// DropNewest 전략에서 버퍼가 가득 차면 새 메시지를 드롭해야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 2,
		Ch:         make(chan message.Message, 2),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyDrop,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}

	// 버퍼를 가득 채운다.
	msg1 := message.New()
	msg2 := message.New()
	wire.Ch <- msg1
	wire.Ch <- msg2

	// 새 메시지 전송 시도 - 드롭되어야 한다.
	ctx := context.Background()
	newMsg := message.New()
	result, err := SendWithBackpressure(ctx, wire, newMsg, policy)

	require.NoError(t, err)
	assert.True(t, result.Dropped)

	// 채널에는 원래 메시지가 그대로 있어야 한다.
	assert.Equal(t, 2, len(wire.Ch))
	received1 := <-wire.Ch
	assert.Equal(t, msg1.ID(), received1.ID())
	received2 := <-wire.Ch
	assert.Equal(t, msg2.ID(), received2.ID())
}

func TestSendWithBackpressure_DropNewest_WhenBufferNotFull(t *testing.T) {
	// DropNewest 전략에서 버퍼에 여유가 있으면 정상 전송되어야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 5,
		Ch:         make(chan message.Message, 5),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyDrop,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}

	ctx := context.Background()
	msg := message.New()
	result, err := SendWithBackpressure(ctx, wire, msg, policy)

	require.NoError(t, err)
	assert.False(t, result.Dropped)
	assert.Equal(t, 1, len(wire.Ch))
}

func TestSendWithBackpressure_DropOldest_WhenBufferFull(t *testing.T) {
	// DropOldest 전략에서 버퍼가 가득 차면 가장 오래된 메시지를 제거하고 새 메시지를 추가해야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 2,
		Ch:         make(chan message.Message, 2),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyDrop,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropOldest,
	}

	// 버퍼를 가득 채운다.
	msg1 := message.New()
	msg2 := message.New()
	wire.Ch <- msg1
	wire.Ch <- msg2

	// 새 메시지 전송 - 가장 오래된 것을 제거하고 새 것을 추가해야 한다.
	ctx := context.Background()
	newMsg := message.New()
	result, err := SendWithBackpressure(ctx, wire, newMsg, policy)

	require.NoError(t, err)
	assert.True(t, result.Dropped)

	// 채널에는 msg2와 newMsg가 있어야 한다 (msg1이 드롭됨).
	assert.Equal(t, 2, len(wire.Ch))
	received1 := <-wire.Ch
	assert.Equal(t, msg2.ID(), received1.ID())
	received2 := <-wire.Ch
	assert.Equal(t, newMsg.ID(), received2.ID())
}

func TestSendWithBackpressure_DropOldest_WhenBufferNotFull(t *testing.T) {
	// DropOldest 전략에서 버퍼에 여유가 있으면 정상 전송되어야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 5,
		Ch:         make(chan message.Message, 5),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyDrop,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropOldest,
	}

	ctx := context.Background()
	msg := message.New()
	result, err := SendWithBackpressure(ctx, wire, msg, policy)

	require.NoError(t, err)
	assert.False(t, result.Dropped)
	assert.Equal(t, 1, len(wire.Ch))
}

// ---------------------------------------------------------------------------
// REQ-04-06: High water mark warning
// ---------------------------------------------------------------------------

func TestSendWithBackpressure_HighWaterMark_Exceeded(t *testing.T) {
	// 버퍼 사용량이 HighWaterMark를 초과하면 경고를 반환해야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.5, // 50%
		DropPolicy:          DropNewest,
	}

	// 버퍼를 50% 이상 채운다 (6/10).
	for i := 0; i < 6; i++ {
		wire.Ch <- message.New()
	}

	ctx := context.Background()
	msg := message.New()
	result, err := SendWithBackpressure(ctx, wire, msg, policy)

	require.NoError(t, err)
	assert.True(t, result.HighWaterMarkReached)
}

func TestSendWithBackpressure_HighWaterMark_NotExceeded(t *testing.T) {
	// 버퍼 사용량이 HighWaterMark 미만이면 경고가 없어야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 10,
		Ch:         make(chan message.Message, 10),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.8, // 80%
		DropPolicy:          DropNewest,
	}

	// 버퍼를 20%만 채운다 (2/10).
	wire.Ch <- message.New()
	wire.Ch <- message.New()

	ctx := context.Background()
	msg := message.New()
	result, err := SendWithBackpressure(ctx, wire, msg, policy)

	require.NoError(t, err)
	assert.False(t, result.HighWaterMarkReached)
}

func TestSendWithBackpressure_HighWaterMark_UnbufferedChannel(t *testing.T) {
	// 언버퍼 채널에서는 HWM 검사가 적용되지 않아야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBypass,
		BufferSize: 0,
		Ch:         make(chan message.Message),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.5,
		DropPolicy:          DropNewest,
	}

	// 수신자를 goroutine 으로 시작
	go func() {
		<-wire.Ch
	}()

	ctx := context.Background()
	msg := message.New()
	result, err := SendWithBackpressure(ctx, wire, msg, policy)

	require.NoError(t, err)
	assert.False(t, result.HighWaterMarkReached)
}

// ---------------------------------------------------------------------------
// 추가 엣지 케이스
// ---------------------------------------------------------------------------

func TestSendWithBackpressure_ClosedChannel(t *testing.T) {
	// 닫힌 채널에 전송 시 ErrChannelClosed를 반환해야 한다.
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 5,
		Ch:         make(chan message.Message, 5),
	}
	wire.Close()

	policy := DefaultBackpressurePolicy()
	ctx := context.Background()
	msg := message.New()

	_, err := SendWithBackpressure(ctx, wire, msg, policy)
	assert.ErrorIs(t, err, ErrChannelClosed)
}

func TestBackpressureResult_Fields(t *testing.T) {
	// BackpressureResult 구조체가 올바른 필드를 가지는지 검증한다.
	result := BackpressureResult{
		Dropped:              true,
		HighWaterMarkReached: true,
	}
	assert.True(t, result.Dropped)
	assert.True(t, result.HighWaterMarkReached)
}

func TestSendWithBackpressure_ContextCancelled(t *testing.T) {
	// 컨텍스트가 취소되면 전송이 실패해야 한다 (Block 전략에서).
	wire := &RuntimeWire{
		ID:         "w1",
		Mode:       flow.WireBuffer,
		BufferSize: 1,
		Ch:         make(chan message.Message, 1),
	}
	policy := BackpressurePolicy{
		Strategy:            StrategyBlock,
		BufferHighWaterMark: 0.8,
		DropPolicy:          DropNewest,
	}

	// 버퍼를 가득 채운다.
	wire.Ch <- message.New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := message.New()
	_, err := SendWithBackpressure(ctx, wire, msg, policy)
	assert.Error(t, err)
}
