package system

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSubscriber 는 테스트용 ChartSubscriber 구현체이다.
// Send/Close 호출 내역과 수신한 바이트를 저장한다.
type fakeSubscriber struct {
	id       string
	mu       sync.Mutex
	received [][]byte
	closed   atomic.Bool
	sendErr  error
}

func newFakeSubscriber(id string) *fakeSubscriber {
	return &fakeSubscriber{id: id}
}

func (f *fakeSubscriber) Send(msg []byte) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// 메시지 복사본을 저장해 race 를 방지한다.
	cp := make([]byte, len(msg))
	copy(cp, msg)
	f.received = append(f.received, cp)
	return nil
}

func (f *fakeSubscriber) Close() error {
	f.closed.Store(true)
	return nil
}

func (f *fakeSubscriber) ID() string {
	return f.id
}

func (f *fakeSubscriber) messages() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([][]byte, len(f.received))
	copy(cp, f.received)
	return cp
}

func (f *fakeSubscriber) isClosed() bool {
	return f.closed.Load()
}

// --- ValidateChartChannelName 테스트 ---

func TestValidateChartChannelName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_simple", "temp", false},
		{"valid_with_digits", "room1_temp", false},
		{"valid_with_hyphen", "room-1-temp", false},
		{"valid_mixed", "ABC_xyz-123", false},
		{"valid_max_length", "a" + strings.Repeat("b", 63), false},
		{"invalid_too_long", "a" + strings.Repeat("b", 64), true},
		{"invalid_empty", "", true},
		{"invalid_leading_digit", "1abc", true},
		{"invalid_leading_underscore", "_abc", true},
		{"invalid_leading_hyphen", "-abc", true},
		{"invalid_slash", "abc/def", true},
		{"invalid_space", "abc def", true},
		{"invalid_unicode", "온도", true},
		{"invalid_dot", "abc.def", true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateChartChannelName(c.input)
			if c.wantErr {
				assert.Error(t, err, "expected error for %q", c.input)
			} else {
				assert.NoError(t, err, "unexpected error for %q", c.input)
			}
		})
	}
}

// --- Registry 테스트 ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("room1_temp", "flow-A", "node-1", 100, 3600)
	require.NoError(t, err)
	require.NotNil(t, ch)

	got, ok := reg.Get("room1_temp")
	require.True(t, ok)
	assert.Same(t, ch, got)

	// 없는 이름 조회
	_, ok = reg.Get("nope")
	assert.False(t, ok)
}

func TestRegistry_DuplicateReturnsError(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	_, err := reg.Register("dup1", "flow-A", "node-1", 100, 3600)
	require.NoError(t, err)

	_, err = reg.Register("dup1", "flow-B", "node-2", 100, 3600)
	require.Error(t, err)

	// 에러 메시지 검증: 기존 flow/node 정보 포함
	msg := err.Error()
	assert.Contains(t, msg, `"dup1"`, "error must contain quoted channel name")
	assert.Contains(t, msg, "is already registered", "error must indicate duplicate")
	assert.Contains(t, msg, "flow-A", "error must include existing flow id")
	assert.Contains(t, msg, "node-1", "error must include existing node id")
}

func TestRegistry_InvalidNameReturnsError(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	_, err := reg.Register("1bad", "flow-A", "node-1", 100, 3600)
	assert.Error(t, err)
}

func TestRegistry_Unregister(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	_, err := reg.Register("room1", "flow-A", "node-1", 100, 3600)
	require.NoError(t, err)

	err = reg.Unregister("room1")
	assert.NoError(t, err)

	_, ok := reg.Get("room1")
	assert.False(t, ok)

	// 없는 것 해제
	err = reg.Unregister("missing")
	assert.Error(t, err)
}

func TestRegistry_UnregisterSendsClosedFrame(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("room1", "flow-A", "node-1", 100, 3600)
	require.NoError(t, err)

	sub := newFakeSubscriber("s1")
	ch.Subscribe(sub)

	err = reg.Unregister("room1")
	require.NoError(t, err)

	// 구독자가 chart.closed 프레임 수신
	msgs := sub.messages()
	require.NotEmpty(t, msgs, "subscriber should receive chart.closed on unregister")

	var frame map[string]any
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1], &frame))
	assert.Equal(t, "chart.closed", frame["type"])
	assert.Equal(t, "room1", frame["channel"])
	assert.Equal(t, "flow_undeployed", frame["reason"])

	// Close() 가 호출되었는지 확인
	assert.True(t, sub.isClosed())
}

func TestRegistry_List(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	_, err := reg.Register("ch-a", "flow-1", "node-a", 50, 600)
	require.NoError(t, err)
	_, err = reg.Register("ch-b", "flow-1", "node-b", 200, 1200)
	require.NoError(t, err)

	list := reg.List()
	require.Len(t, list, 2)

	byName := make(map[string]ChartChannelInfo, 2)
	for _, info := range list {
		byName[info.Name] = info
	}

	a := byName["ch-a"]
	assert.Equal(t, "flow-1", a.FlowID)
	assert.Equal(t, "node-a", a.NodeID)
	assert.Equal(t, 50, a.BufferSize)
	assert.Equal(t, 600, a.RetentionSec)
	assert.Equal(t, 0, a.SubscriberCount)
	assert.Equal(t, int64(0), a.LastMessageMs)

	b := byName["ch-b"]
	assert.Equal(t, 200, b.BufferSize)
}

// --- ChartChannel: 링버퍼 + Publish 테스트 ---

func TestChartChannel_RingBufferFIFO(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	// capacity=5, retention 비활성
	ch, err := reg.Register("ring", "flow-1", "node-1", 5, 0)
	require.NoError(t, err)

	// 7 개 추가 (ts=1..7)
	for i := int64(1); i <= 7; i++ {
		require.NoError(t, ch.Publish(ChartEntry{Timestamp: i, Value: i}))
	}

	snap := ch.Snapshot()
	require.Len(t, snap, 5)
	// FIFO 이므로 가장 오래된 1, 2 가 제거되고 3..7 이 남아야 한다
	for i, e := range snap {
		wantTs := int64(3 + i)
		assert.Equal(t, wantTs, e.Timestamp, "index %d", i)
	}
}

func TestChartChannel_BufferSizeClamp(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	// 너무 작음(0) -> 1 로 clamp
	ch1, err := reg.Register("c1", "f", "n", 0, 0)
	require.NoError(t, err)
	info := ch1.Info()
	assert.GreaterOrEqual(t, info.BufferSize, 1)

	// 너무 큼(20000) -> 10000 으로 clamp
	ch2, err := reg.Register("c2", "f", "n", 20000, 0)
	require.NoError(t, err)
	info2 := ch2.Info()
	assert.LessOrEqual(t, info2.BufferSize, 10000)
}

func TestChartChannel_RetentionClamp(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("rc", "f", "n", 10, 1000000)
	require.NoError(t, err)
	info := ch.Info()
	assert.LessOrEqual(t, info.RetentionSec, 86400)

	ch2, err := reg.Register("rc2", "f", "n", 10, -5)
	require.NoError(t, err)
	info2 := ch2.Info()
	assert.GreaterOrEqual(t, info2.RetentionSec, 0)
}

func TestChartChannel_PublishFanOut(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("fan", "f", "n", 100, 0)
	require.NoError(t, err)

	subs := []*fakeSubscriber{
		newFakeSubscriber("a"),
		newFakeSubscriber("b"),
		newFakeSubscriber("c"),
	}
	for _, s := range subs {
		ch.Subscribe(s)
	}

	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 1000, Value: 42.5}))

	for _, s := range subs {
		msgs := s.messages()
		require.Len(t, msgs, 1, "subscriber %s", s.ID())

		var frame map[string]any
		require.NoError(t, json.Unmarshal(msgs[0], &frame))
		assert.Equal(t, "chart.append", frame["type"])
		assert.Equal(t, "fan", frame["channel"])
	}
}

func TestChartChannel_Subscribe_ReturnsBackfill(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("bf", "f", "n", 10, 0)
	require.NoError(t, err)

	// 미리 3 개 publish
	for i := int64(1); i <= 3; i++ {
		require.NoError(t, ch.Publish(ChartEntry{Timestamp: i * 100, Value: i}))
	}

	sub := newFakeSubscriber("late")
	backfill := ch.Subscribe(sub)

	require.Len(t, backfill, 3)
	// 타임스탬프 오름차순
	assert.Equal(t, int64(100), backfill[0].Timestamp)
	assert.Equal(t, int64(200), backfill[1].Timestamp)
	assert.Equal(t, int64(300), backfill[2].Timestamp)
}

func TestChartChannel_Subscribe_BackfillRespectsRetention(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("bfr", "f", "n", 10, 5)
	require.NoError(t, err)

	// 가상 클럭을 고정: now = 100초 = 100000ms
	var nowMs int64 = 100000
	ch.SetClock(func() int64 { return nowMs })

	// 여러 타임스탬프로 publish
	// 오래된 것: 90s (10초 전) → 만료
	// 최근: 97s, 99s → 유지
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 90000, Value: "old"}))
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 97000, Value: "mid"}))
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 99000, Value: "new"}))

	sub := newFakeSubscriber("s")
	backfill := ch.Subscribe(sub)

	// 만료 기준: now - ts > retention*1000 → 100000 - 90000 = 10000 > 5000 → 만료
	// 97000: 100000 - 97000 = 3000 <= 5000 → 유지
	// 99000: 100000 - 99000 = 1000 → 유지
	require.Len(t, backfill, 2)
	assert.Equal(t, int64(97000), backfill[0].Timestamp)
	assert.Equal(t, int64(99000), backfill[1].Timestamp)
}

func TestChartChannel_Unsubscribe(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("unsub", "f", "n", 10, 0)
	require.NoError(t, err)

	sub := newFakeSubscriber("a")
	ch.Subscribe(sub)
	assert.Equal(t, 1, ch.Info().SubscriberCount)

	ch.Unsubscribe(sub)
	assert.Equal(t, 0, ch.Info().SubscriberCount)

	// 이제 publish 해도 sub 는 수신하지 않아야 함
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 1, Value: 1}))
	assert.Empty(t, sub.messages())
}

func TestChartChannel_RetentionSweep(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("sweep", "f", "n", 100, 5)
	require.NoError(t, err)

	var nowMs int64 = 100000
	ch.SetClock(func() int64 { return atomic.LoadInt64(&nowMs) })

	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 90000, Value: "old"}))
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 97000, Value: "mid"}))
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 99000, Value: "new"}))

	// 시각 진행
	atomic.StoreInt64(&nowMs, 101000)
	ch.SweepExpired()

	snap := ch.Snapshot()
	// 90000 은 101-90=11 > 5 초과로 제거, 97/99 는 유지
	require.Len(t, snap, 2)
	assert.Equal(t, int64(97000), snap[0].Timestamp)
	assert.Equal(t, int64(99000), snap[1].Timestamp)
}

func TestChartChannel_RetentionZeroDisabled(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("norte", "f", "n", 10, 0)
	require.NoError(t, err)

	var nowMs int64 = 1000000000
	ch.SetClock(func() int64 { return nowMs })

	// 매우 오래된 타임스탬프
	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 0, Value: "ancient"}))
	ch.SweepExpired()

	snap := ch.Snapshot()
	assert.Len(t, snap, 1, "retention=0 should disable time-based expiry")
}

func TestChartChannel_Close_EmitsClosedAndRejectsPublish(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("cls", "f", "n", 10, 0)
	require.NoError(t, err)

	sub := newFakeSubscriber("s")
	ch.Subscribe(sub)

	ch.Close("node_removed")

	// closed 프레임 수신
	msgs := sub.messages()
	require.NotEmpty(t, msgs)
	var frame map[string]any
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1], &frame))
	assert.Equal(t, "chart.closed", frame["type"])
	assert.Equal(t, "node_removed", frame["reason"])

	// 후속 Publish 는 에러
	err = ch.Publish(ChartEntry{Timestamp: 1, Value: 1})
	assert.Error(t, err)
}

func TestChartChannel_LastMessageMsTracked(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("lm", "f", "n", 10, 0)
	require.NoError(t, err)

	info := ch.Info()
	assert.Equal(t, int64(0), info.LastMessageMs)

	var nowMs int64 = 55555
	ch.SetClock(func() int64 { return nowMs })

	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 111, Value: 1}))

	info = ch.Info()
	assert.Greater(t, info.LastMessageMs, int64(0))
}

// Race-safe 다중 고루틴 Publish/Subscribe 테스트
func TestChartChannel_ConcurrentPublishAndSubscribe(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry()
	defer reg.Close()

	ch, err := reg.Register("conc", "f", "n", 1000, 0)
	require.NoError(t, err)

	const nSubs = 5
	var subs []*fakeSubscriber
	for i := 0; i < nSubs; i++ {
		s := newFakeSubscriber(fmt.Sprintf("s-%d", i))
		ch.Subscribe(s)
		subs = append(subs, s)
	}

	var wg sync.WaitGroup
	const nMsgs = 100
	const nWorkers = 4
	for w := 0; w < nWorkers; w++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < nMsgs; i++ {
				_ = ch.Publish(ChartEntry{Timestamp: int64(base*nMsgs + i), Value: i})
			}
		}(w)
	}
	wg.Wait()

	// 각 subscriber 는 nWorkers*nMsgs 개 append 를 수신 (순서 보장 안 함)
	for _, s := range subs {
		assert.Equal(t, nWorkers*nMsgs, len(s.messages()))
	}
}

// --- Framing 테스트 (EncodeChart*) ---

func TestEncodeChartBackfill(t *testing.T) {
	t.Parallel()

	entries := []ChartEntry{
		{Timestamp: 1000, Value: 42, Labels: map[string]string{"room": "A"}},
		{Timestamp: 2000, Value: "hello"},
	}
	b, err := EncodeChartBackfill("ch1", entries)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	assert.Equal(t, "chart.backfill", got["type"])
	assert.Equal(t, "ch1", got["channel"])
	assert.EqualValues(t, 2, got["backfilled_count"])

	list, ok := got["entries"].([]any)
	require.True(t, ok)
	require.Len(t, list, 2)
}

func TestEncodeChartAppend(t *testing.T) {
	t.Parallel()

	entry := ChartEntry{Timestamp: 1234, Value: 3.14, Labels: map[string]string{"x": "y"}}
	b, err := EncodeChartAppend("ch1", entry)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "chart.append", got["type"])
	assert.Equal(t, "ch1", got["channel"])

	e, ok := got["entry"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 1234, e["timestamp"])
}

func TestEncodeChartClosed(t *testing.T) {
	t.Parallel()

	b, err := EncodeChartClosed("ch1", "flow_undeployed")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "chart.closed", got["type"])
	assert.Equal(t, "ch1", got["channel"])
	assert.Equal(t, "flow_undeployed", got["reason"])
}

func TestEncodeChartError(t *testing.T) {
	t.Parallel()

	b, err := EncodeChartError("ch1", "channel_not_found")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "chart.error", got["type"])
	assert.Equal(t, "ch1", got["channel"])
	assert.Equal(t, "channel_not_found", got["reason"])
}

func TestEncodeChartAppend_EntryFieldsOrder(t *testing.T) {
	t.Parallel()

	entry := ChartEntry{Timestamp: 1, Value: 2}
	b, err := EncodeChartAppend("x", entry)
	require.NoError(t, err)

	// 프레임에 labels 가 nil 이면 포함되지 않아야 함 (omitempty)
	s := string(b)
	assert.NotContains(t, s, "labels")
	assert.NotContains(t, s, "meta")
}

func TestDefaultChartChannelRegistry_SetAndGet(t *testing.T) {
	// 전역 싱글톤이므로 병렬 실행 금지
	prev := DefaultChartChannelRegistry()
	t.Cleanup(func() {
		SetDefaultChartChannelRegistry(prev)
	})

	assert.Nil(t, nil) // sanity

	reg := NewChartChannelRegistry()
	defer reg.Close()

	SetDefaultChartChannelRegistry(reg)
	got := DefaultChartChannelRegistry()
	assert.Same(t, reg, got)

	SetDefaultChartChannelRegistry(nil)
	assert.Nil(t, DefaultChartChannelRegistry())
}

// 약간의 대기 기반 통합 테스트: registry 주기 스윕이 백그라운드에서 동작 (옵션)
func TestRegistry_BackgroundSweep(t *testing.T) {
	t.Parallel()

	reg := NewChartChannelRegistry(WithSweepInterval(20 * time.Millisecond))
	defer reg.Close()

	ch, err := reg.Register("bgs", "f", "n", 10, 1)
	require.NoError(t, err)

	var nowMs int64 = 100000
	ch.SetClock(func() int64 { return atomic.LoadInt64(&nowMs) })

	require.NoError(t, ch.Publish(ChartEntry{Timestamp: 50000, Value: "old"}))

	// 시간 진행시키고 약간 대기
	atomic.StoreInt64(&nowMs, 200000)
	time.Sleep(100 * time.Millisecond)

	snap := ch.Snapshot()
	assert.Empty(t, snap, "background sweep should have removed expired entry")
}
