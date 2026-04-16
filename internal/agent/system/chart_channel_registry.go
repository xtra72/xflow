package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// 차트 채널 관련 상수.
const (
	// MinChartBufferSize 는 링버퍼 용량의 최소값이다.
	MinChartBufferSize = 1
	// MaxChartBufferSize 는 링버퍼 용량의 최대값이다.
	MaxChartBufferSize = 10000
	// MinChartRetentionSec 은 보관 시간의 최소값(초)이다. 0 이면 시간 기반 만료 비활성.
	MinChartRetentionSec = 0
	// MaxChartRetentionSec 은 보관 시간의 최대값(초)이다 (24 시간).
	MaxChartRetentionSec = 86400

	// DefaultChartSweepInterval 은 레지스트리 백그라운드 스윕 주기의 기본값이다.
	DefaultChartSweepInterval = 10 * time.Second
)

// chartChannelNameRegexp 는 channel_name 형식을 검증하는 정규식이다.
var chartChannelNameRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

// 차트 채널 관련 에러.
var (
	// ErrChartChannelNotFound 는 요청한 채널이 레지스트리에 없을 때 반환된다.
	ErrChartChannelNotFound = errors.New("chart channel: not found")

	// ErrChartChannelClosed 는 이미 Close 된 채널에 Publish 시도 시 반환된다.
	ErrChartChannelClosed = errors.New("chart channel: closed")

	// ErrChartChannelInvalidName 은 channel_name 형식이 유효하지 않을 때 반환된다.
	ErrChartChannelInvalidName = errors.New("chart channel: invalid channel_name")
)

// ValidateChartChannelName 은 channel_name 이 정규식 ^[a-zA-Z][a-zA-Z0-9_-]{0,63}$ 을 만족하는지 확인한다.
// 위반 시 ErrChartChannelInvalidName 을 래핑한 에러를 반환한다.
func ValidateChartChannelName(name string) error {
	if !chartChannelNameRegexp.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrChartChannelInvalidName, name)
	}
	return nil
}

// ChartEntry 는 차트 채널에 발행되는 단일 데이터 포인트이다.
// timestamp 는 프로젝트 규칙에 따라 epoch 밀리초(int64)이다.
type ChartEntry struct {
	Timestamp int64             `json:"timestamp"`
	Value     any               `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Meta      map[string]any    `json:"meta,omitempty"`
}

// ChartChannelInfo 는 레지스트리가 외부로 노출하는 채널 요약 정보이다.
type ChartChannelInfo struct {
	Name            string `json:"name"`
	FlowID          string `json:"flow_id"`
	NodeID          string `json:"node_id"`
	BufferSize      int    `json:"buffer_size"`
	RetentionSec    int    `json:"retention_sec"`
	SubscriberCount int    `json:"subscriber_count"`
	LastMessageMs   int64  `json:"last_message_ms"`
}

// ChartSubscriber 는 차트 채널의 메시지 수신자 계약이다.
// WebSocket 핸들러가 이 인터페이스를 구현하여 레지스트리에 등록한다.
type ChartSubscriber interface {
	// Send 는 직렬화된 JSON 바이트를 구독자에게 전송한다.
	Send(msgBytes []byte) error
	// Close 는 구독자 연결을 종료한다.
	Close() error
	// ID 는 구독자 고유 식별자를 반환한다 (디버깅/중복 제거용).
	ID() string
}

// ChartChannel 은 단일 채널의 링버퍼, 구독자, 수명주기를 관리한다.
type ChartChannel struct {
	name         string
	flowID       string
	nodeID       string
	bufferSize   int
	retentionSec int

	mu            sync.RWMutex
	ring          []ChartEntry // FIFO 링버퍼 (오래된→최신 순)
	subscribers   map[string]ChartSubscriber
	closed        bool
	lastMessageMs int64
	clock         func() int64
}

// SetClock 은 테스트용으로 현재 시각 함수(epoch ms)를 주입한다.
func (c *ChartChannel) SetClock(clock func() int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if clock != nil {
		c.clock = clock
	}
}

// Info 는 채널의 현재 요약 정보를 반환한다.
func (c *ChartChannel) Info() ChartChannelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ChartChannelInfo{
		Name:            c.name,
		FlowID:          c.flowID,
		NodeID:          c.nodeID,
		BufferSize:      c.bufferSize,
		RetentionSec:    c.retentionSec,
		SubscriberCount: len(c.subscribers),
		LastMessageMs:   c.lastMessageMs,
	}
}

// Subscribe 는 구독자를 등록하고 현재 링버퍼 내용(보관 기간 적용)을 backfill 로 반환한다.
// 프레임 직렬화는 호출자(WS 핸들러)가 담당한다.
func (c *ChartChannel) Subscribe(sub ChartSubscriber) []ChartEntry {
	if sub == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.subscribers[sub.ID()] = sub
	// backfill: 보관 기간을 초과한 항목은 제거한 뒤 반환
	return c.filteredSnapshotLocked()
}

// Unsubscribe 는 구독자를 제거한다. 이미 없으면 무시한다.
func (c *ChartChannel) Unsubscribe(sub ChartSubscriber) {
	if sub == nil {
		return
	}
	c.mu.Lock()
	delete(c.subscribers, sub.ID())
	c.mu.Unlock()
}

// Publish 는 링버퍼에 엔트리를 추가하고 모든 구독자에게 chart.append 프레임을 전송한다.
// FIFO 제거는 용량 초과 시 자동 적용된다.
func (c *ChartChannel) Publish(entry ChartEntry) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrChartChannelClosed
	}

	// 링버퍼 추가 (FIFO)
	c.ring = append(c.ring, entry)
	if len(c.ring) > c.bufferSize {
		// 가장 오래된 것부터 제거
		drop := len(c.ring) - c.bufferSize
		c.ring = append(c.ring[:0:0], c.ring[drop:]...)
	}

	c.lastMessageMs = c.clock()

	// 구독자 스냅샷 (fan-out 중 구독자 변경으로 인한 데드락 방지)
	subs := make([]ChartSubscriber, 0, len(c.subscribers))
	for _, s := range c.subscribers {
		subs = append(subs, s)
	}
	channelName := c.name
	c.mu.Unlock()

	// 프레임 직렬화 1회
	bytes, err := EncodeChartAppend(channelName, entry)
	if err != nil {
		return err
	}

	for _, s := range subs {
		_ = s.Send(bytes)
	}
	return nil
}

// Snapshot 은 현재 링버퍼 내용의 복사본을 반환한다 (time 필터 없이).
func (c *ChartChannel) Snapshot() []ChartEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ChartEntry, len(c.ring))
	copy(out, c.ring)
	return out
}

// SweepExpired 는 보관 기간을 초과한 엔트리를 링버퍼에서 제거한다.
// retentionSec == 0 이면 아무 것도 하지 않는다.
func (c *ChartChannel) SweepExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepExpiredLocked()
}

// sweepExpiredLocked 는 mu 가 쓰기 잠금된 상태에서 만료 엔트리를 제거한다.
func (c *ChartChannel) sweepExpiredLocked() {
	if c.retentionSec <= 0 {
		return
	}
	cutoff := c.clock() - int64(c.retentionSec)*1000
	// 링 엔트리는 엄격한 시간 순서 보장이 없으므로 전체 훑기.
	// 기존 메모리 공유를 피하기 위해 새 슬라이스를 할당한다.
	kept := make([]ChartEntry, 0, len(c.ring))
	for _, e := range c.ring {
		if e.Timestamp >= cutoff {
			kept = append(kept, e)
		}
	}
	c.ring = kept
}

// filteredSnapshotLocked 는 mu 가 읽기/쓰기 잠금된 상태에서
// 보관 기간을 적용한 엔트리 배열을 타임스탬프 오름차순으로 반환한다.
func (c *ChartChannel) filteredSnapshotLocked() []ChartEntry {
	var source []ChartEntry
	if c.retentionSec > 0 {
		cutoff := c.clock() - int64(c.retentionSec)*1000
		source = make([]ChartEntry, 0, len(c.ring))
		for _, e := range c.ring {
			if e.Timestamp >= cutoff {
				source = append(source, e)
			}
		}
	} else {
		source = make([]ChartEntry, len(c.ring))
		copy(source, c.ring)
	}
	sort.SliceStable(source, func(i, j int) bool {
		return source[i].Timestamp < source[j].Timestamp
	})
	return source
}

// Close 는 채널을 종료하고 모든 구독자에게 chart.closed 프레임 전송 후 연결을 닫는다.
// 이후 Publish 는 ErrChartChannelClosed 를 반환한다.
func (c *ChartChannel) Close(reason string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	subs := make([]ChartSubscriber, 0, len(c.subscribers))
	for _, s := range c.subscribers {
		subs = append(subs, s)
	}
	c.subscribers = map[string]ChartSubscriber{}
	name := c.name
	c.mu.Unlock()

	frame, err := EncodeChartClosed(name, reason)
	if err != nil {
		return
	}
	for _, s := range subs {
		_ = s.Send(frame)
		_ = s.Close()
	}
}

// --- Registry ---

// ChartChannelRegistryOption 은 레지스트리 생성 시 적용할 수 있는 옵션이다.
type ChartChannelRegistryOption func(*ChartChannelRegistry)

// WithSweepInterval 은 백그라운드 스윕 주기를 지정한다 (기본 10초).
func WithSweepInterval(d time.Duration) ChartChannelRegistryOption {
	return func(r *ChartChannelRegistry) {
		if d > 0 {
			r.sweepInterval = d
		}
	}
}

// ChartChannelRegistry 는 channel_name → *ChartChannel 매핑과 백그라운드 스윕 고루틴을 관리한다.
type ChartChannelRegistry struct {
	mu            sync.RWMutex
	channels      map[string]*ChartChannel
	sweepInterval time.Duration
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// --- 프로세스 전역 싱글톤 ---
//
// chart-emitter 노드와 WS 핸들러(/ws/chart/{channel_name}) 가 동일한 레지스트리 인스턴스를
// 공유해야 하므로, 프로세스 수명 동안 단일 인스턴스를 유지한다. main.go 초기화 시
// SetDefaultChartChannelRegistry 로 1회 주입한다. 테스트는 Set → Cleanup 으로 복원한다.

var (
	defaultChartRegistryMu sync.RWMutex
	defaultChartRegistry   *ChartChannelRegistry
)

// SetDefaultChartChannelRegistry 는 프로세스 전역 ChartChannelRegistry 인스턴스를 설정한다.
// nil 을 전달하면 레지스트리를 비활성화한다 (주로 테스트 복원용).
func SetDefaultChartChannelRegistry(r *ChartChannelRegistry) {
	defaultChartRegistryMu.Lock()
	defer defaultChartRegistryMu.Unlock()
	defaultChartRegistry = r
}

// DefaultChartChannelRegistry 는 현재 프로세스 전역 레지스트리를 반환한다.
// 설정되지 않았으면 nil 이다.
func DefaultChartChannelRegistry() *ChartChannelRegistry {
	defaultChartRegistryMu.RLock()
	defer defaultChartRegistryMu.RUnlock()
	return defaultChartRegistry
}

// NewChartChannelRegistry 는 새 레지스트리를 생성하고 백그라운드 스윕 고루틴을 시작한다.
func NewChartChannelRegistry(opts ...ChartChannelRegistryOption) *ChartChannelRegistry {
	r := &ChartChannelRegistry{
		channels:      make(map[string]*ChartChannel),
		sweepInterval: DefaultChartSweepInterval,
		stopCh:        make(chan struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.wg.Add(1)
	go r.sweepLoop()
	return r
}

// sweepLoop 는 주기적으로 모든 채널의 만료 엔트리를 제거한다.
func (r *ChartChannelRegistry) sweepLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.sweepAll()
		}
	}
}

// sweepAll 은 모든 채널을 순회하며 만료 엔트리를 제거한다.
func (r *ChartChannelRegistry) sweepAll() {
	r.mu.RLock()
	snapshot := make([]*ChartChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		snapshot = append(snapshot, ch)
	}
	r.mu.RUnlock()

	for _, ch := range snapshot {
		ch.SweepExpired()
	}
}

// Register 는 새 채널을 등록한다. 이름이 이미 존재하면 fail-fast 에러를 반환한다.
// buffer_size 는 [1, 10000], retention_sec 은 [0, 86400] 범위로 clamp 된다.
func (r *ChartChannelRegistry) Register(name, flowID, nodeID string, bufferSize, retentionSec int) (*ChartChannel, error) {
	if err := ValidateChartChannelName(name); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.channels[name]; ok {
		// SPEC-STORE-002 스타일 fail-fast 메시지
		info := existing.Info()
		return nil, fmt.Errorf("channel_name %q is already registered by flow %s node %s",
			name, info.FlowID, info.NodeID)
	}

	bufSize := clampInt(bufferSize, MinChartBufferSize, MaxChartBufferSize)
	retSec := clampInt(retentionSec, MinChartRetentionSec, MaxChartRetentionSec)

	ch := &ChartChannel{
		name:         name,
		flowID:       flowID,
		nodeID:       nodeID,
		bufferSize:   bufSize,
		retentionSec: retSec,
		ring:         make([]ChartEntry, 0, bufSize),
		subscribers:  make(map[string]ChartSubscriber),
		clock:        func() int64 { return time.Now().UnixMilli() },
	}
	r.channels[name] = ch
	return ch, nil
}

// Unregister 는 채널을 제거하고 `flow_undeployed` 사유로 Close 를 호출한다.
// 존재하지 않는 이름이면 ErrChartChannelNotFound 를 반환한다.
func (r *ChartChannelRegistry) Unregister(name string) error {
	r.mu.Lock()
	ch, ok := r.channels[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrChartChannelNotFound, name)
	}
	delete(r.channels, name)
	r.mu.Unlock()

	ch.Close("flow_undeployed")
	return nil
}

// Get 은 이름으로 채널을 조회한다.
func (r *ChartChannelRegistry) Get(name string) (*ChartChannel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[name]
	return ch, ok
}

// List 는 등록된 모든 채널의 요약 정보를 이름 오름차순으로 반환한다.
func (r *ChartChannelRegistry) List() []ChartChannelInfo {
	r.mu.RLock()
	names := make([]string, 0, len(r.channels))
	chans := make(map[string]*ChartChannel, len(r.channels))
	for n, ch := range r.channels {
		names = append(names, n)
		chans[n] = ch
	}
	r.mu.RUnlock()

	sort.Strings(names)
	out := make([]ChartChannelInfo, 0, len(names))
	for _, n := range names {
		out = append(out, chans[n].Info())
	}
	return out
}

// Close 는 레지스트리의 모든 채널을 종료하고 백그라운드 스윕을 정지한다.
func (r *ChartChannelRegistry) Close() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()

	r.mu.Lock()
	chans := r.channels
	r.channels = make(map[string]*ChartChannel)
	r.mu.Unlock()
	for _, ch := range chans {
		ch.Close("registry_closed")
	}
}

// clampInt 는 값을 [min, max] 범위로 제한한다.
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// --- Frame encoders (WS 핸들러가 재사용) ---

// chartBackfillFrame 은 chart.backfill 메시지의 JSON 구조체이다.
type chartBackfillFrame struct {
	Type            string       `json:"type"`
	Channel         string       `json:"channel"`
	Entries         []ChartEntry `json:"entries"`
	BackfilledCount int          `json:"backfilled_count"`
}

// chartAppendFrame 은 chart.append 메시지의 JSON 구조체이다.
type chartAppendFrame struct {
	Type    string     `json:"type"`
	Channel string     `json:"channel"`
	Entry   ChartEntry `json:"entry"`
}

// chartClosedFrame 은 chart.closed 메시지의 JSON 구조체이다.
type chartClosedFrame struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
}

// chartErrorFrame 은 chart.error 메시지의 JSON 구조체이다.
type chartErrorFrame struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
}

// EncodeChartBackfill 은 chart.backfill 프레임을 JSON 바이트로 직렬화한다.
func EncodeChartBackfill(channel string, entries []ChartEntry) ([]byte, error) {
	if entries == nil {
		entries = []ChartEntry{}
	}
	return json.Marshal(chartBackfillFrame{
		Type:            "chart.backfill",
		Channel:         channel,
		Entries:         entries,
		BackfilledCount: len(entries),
	})
}

// EncodeChartAppend 는 chart.append 프레임을 JSON 바이트로 직렬화한다.
func EncodeChartAppend(channel string, entry ChartEntry) ([]byte, error) {
	return json.Marshal(chartAppendFrame{
		Type:    "chart.append",
		Channel: channel,
		Entry:   entry,
	})
}

// EncodeChartClosed 는 chart.closed 프레임을 JSON 바이트로 직렬화한다.
func EncodeChartClosed(channel, reason string) ([]byte, error) {
	return json.Marshal(chartClosedFrame{
		Type:    "chart.closed",
		Channel: channel,
		Reason:  reason,
	})
}

// EncodeChartError 는 chart.error 프레임을 JSON 바이트로 직렬화한다.
func EncodeChartError(channel, reason string) ([]byte, error) {
	return json.Marshal(chartErrorFrame{
		Type:    "chart.error",
		Channel: channel,
		Reason:  reason,
	})
}
