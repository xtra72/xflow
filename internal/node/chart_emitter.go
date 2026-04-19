package node

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// chart-emitter 설정 기본값 및 범위 상수.
const (
	defaultChartBufferSize   = 100
	defaultChartRetentionSec = 3600
)

// chart-emitter 전용 에러.
var (
	// ErrChartChannelNameRequired 는 channel_name 설정이 누락되었을 때 반환된다.
	ErrChartChannelNameRequired = fmt.Errorf("chart-emitter: %w: channel_name is required", ErrInvalidConfig)
)

// --- 레지스트리 싱글톤 ---
//
// chart-emitter 노드와 M2 의 WebSocket 핸들러(/ws/chart/{channel_name}) 가 동일한 레지스트리
// 인스턴스를 공유해야 하므로, system 패키지의 프로세스 전역 싱글톤에 접근한다.
// main.go 초기화 시 system.SetDefaultChartChannelRegistry 로 1회 주입하고,
// chart-emitter 및 ws.ChartChannelHandler 가 system.DefaultChartChannelRegistry 로 조회한다.
//
// 본 파일의 SetChartChannelRegistry / GetChartChannelRegistry 는 기존 호출자
// (테스트, main.go) 호환성을 위한 얇은 래퍼이다.

// SetChartChannelRegistry 는 프로세스 전역 ChartChannelRegistry 를 설정한다.
// system.SetDefaultChartChannelRegistry 로 위임한다.
func SetChartChannelRegistry(r *system.ChartChannelRegistry) {
	system.SetDefaultChartChannelRegistry(r)
}

// GetChartChannelRegistry 는 현재 프로세스 전역 레지스트리를 반환한다.
// 설정되지 않았으면 nil 이다.
func GetChartChannelRegistry() *system.ChartChannelRegistry {
	return system.DefaultChartChannelRegistry()
}

// --- ChartEmitterNode ---

// ChartEmitterNode 는 입력 메시지를 정규화된 ChartEntry 로 변환하여
// system.ChartChannelRegistry 의 지정된 채널에 발행하는 종단(sink) 노드이다.
//
// 입력 포트: 1 개 ("in")
// 출력 포트: 0 개
//
// Config:
//   - channel_name   (string, required): 채널 이름, 정규식 ^[a-zA-Z][a-zA-Z0-9_-]{0,63}$
//   - buffer_size    (int, default 100):  링버퍼 용량 (1..10000)
//   - retention_sec  (int, default 3600): 시간 기반 만료 (0..86400, 0=비활성)
//   - entries_field  (string, optional):  배치 입력 모드.
//     설정 시 payload[entries_field] 를 []any 로 해석하여 각 element 를
//     독립된 ChartEntry 로 timestamp 오름차순 정렬 후 개별 publish 한다.
//     예) store-read(read_mode=last_n) 출력의 store_value 배열을 라인 차트에
//     공급할 때 "entries_field": "store_value" 로 설정.
//     필드가 없거나 배열이 아니면 단일 엔트리 경로로 fallback.
type ChartEmitterNode struct {
	*BaseNode

	mu           sync.RWMutex
	channelName  string
	bufferSize   int
	retentionSec int
	entriesField string
	channelsField string // 멀티채널: payload 에서 map[string]entries 추출할 필드
	channelPrefix string // 멀티채널: 채널 이름 접두사

	// flowID 는 NodeDef.Metadata["flow_id"] 에서 읽는다.
	// 엔진 측에서 주입되지 않으면 빈 문자열이며, 레지스트리 fail-fast 에러 문구에만 영향을 준다.
	flowID string
	nodeID string

	// channel 은 Init 성공 시 레지스트리에서 받은 채널 핸들이다 (단일 채널 모드).
	channel *system.ChartChannel

	// dynamicChannels 는 멀티채널 모드에서 lazy 등록된 채널들.
	dynamicChannels map[string]*system.ChartChannel

	// clock 은 timestamp 자동 주입에 사용된다. 기본 time.Now().UnixMilli.
	clock func() int64
}

// 컴파일 타임 인터페이스 검사.
var _ Node = (*ChartEmitterNode)(nil)

// NewChartEmitterNode 는 chart-emitter 노드를 생성하는 팩토리이다.
func NewChartEmitterNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ChartEmitterNode{
		BaseNode:     base,
		bufferSize:   defaultChartBufferSize,
		retentionSec: defaultChartRetentionSec,
		nodeID:       def.ID,
		clock:        func() int64 { return time.Now().UnixMilli() },
	}
	// NodeDef.Metadata 에 flow_id 가 있으면 활용 (fail-fast 에러 문구 개선용).
	if def.Metadata != nil {
		if fid, ok := def.Metadata["flow_id"]; ok {
			n.flowID = fid
		}
	}
	return n, nil
}

// SetClock 은 테스트에서 타임스탬프 주입 소스를 교체한다.
func (n *ChartEmitterNode) SetClock(clock func() int64) {
	if clock == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.clock = clock
}

// ChannelName 은 설정된 채널 이름을 반환한다 (테스트/관찰용).
func (n *ChartEmitterNode) ChannelName() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.channelName
}

// BufferSize 는 설정된 링버퍼 크기를 반환한다 (테스트/관찰용).
func (n *ChartEmitterNode) BufferSize() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.bufferSize
}

// RetentionSec 은 설정된 보관 시간(초)을 반환한다 (테스트/관찰용).
func (n *ChartEmitterNode) RetentionSec() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.retentionSec
}

// EntriesField 는 설정된 배치 입력 필드명을 반환한다 (테스트/관찰용).
// 빈 문자열이면 단일 엔트리 모드이다.
func (n *ChartEmitterNode) EntriesField() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.entriesField
}

// Configure 는 chart-emitter 설정을 적용하고 검증한다.
//
// 지원 키:
//   - channel_name  (string, required): 채널 이름 (regex 검증)
//   - buffer_size   (int, default 100): 링버퍼 용량 (범위 1..10000)
//   - retention_sec (int, default 3600): 보관 시간(초) (범위 0..86400)
func (n *ChartEmitterNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// channel_name: channels_field 미지정 시 필수 + 정규식 검증
	hasChannelsField := false
	if cf, ok := config["channels_field"]; ok {
		if s, ok := cf.(string); ok && s != "" {
			hasChannelsField = true
		}
	}
	name := ""
	if raw, ok := config["channel_name"]; ok {
		if s, ok := raw.(string); ok {
			name = s
		}
	}
	if !hasChannelsField {
		if name == "" {
			return ErrChartChannelNameRequired
		}
		if err := system.ValidateChartChannelName(name); err != nil {
			return fmt.Errorf("chart-emitter: %w", err)
		}
	}

	// buffer_size: 선택 (기본 100), 범위 1..10000
	bufSize := defaultChartBufferSize
	if raw, ok := config["buffer_size"]; ok {
		v, err := chartConfigToInt(raw)
		if err != nil {
			return fmt.Errorf("chart-emitter: %w: buffer_size %v", ErrInvalidConfig, err)
		}
		if v < system.MinChartBufferSize || v > system.MaxChartBufferSize {
			return fmt.Errorf("chart-emitter: %w: buffer_size %d out of range [%d,%d]",
				ErrInvalidConfig, v, system.MinChartBufferSize, system.MaxChartBufferSize)
		}
		bufSize = v
	}

	// retention_sec: 선택 (기본 3600), 범위 0..86400
	retSec := defaultChartRetentionSec
	if raw, ok := config["retention_sec"]; ok {
		v, err := chartConfigToInt(raw)
		if err != nil {
			return fmt.Errorf("chart-emitter: %w: retention_sec %v", ErrInvalidConfig, err)
		}
		if v < system.MinChartRetentionSec || v > system.MaxChartRetentionSec {
			return fmt.Errorf("chart-emitter: %w: retention_sec %d out of range [%d,%d]",
				ErrInvalidConfig, v, system.MinChartRetentionSec, system.MaxChartRetentionSec)
		}
		retSec = v
	}

	// entries_field: 선택 (빈 문자열이면 단일 엔트리 모드)
	entriesField := ""
	if raw, ok := config["entries_field"]; ok {
		switch v := raw.(type) {
		case string:
			entriesField = v
		case nil:
			// nil 이면 미설정과 동일
		default:
			return fmt.Errorf("chart-emitter: %w: entries_field must be string (got %T)", ErrInvalidConfig, raw)
		}
	}

	channelsField := ""
	if raw, ok := config["channels_field"]; ok {
		if s, ok := raw.(string); ok {
			channelsField = s
		}
	}
	channelPrefix := ""
	if raw, ok := config["channel_prefix"]; ok {
		if s, ok := raw.(string); ok {
			channelPrefix = s
		}
	}

	n.mu.Lock()
	n.channelName = name
	n.bufferSize = bufSize
	n.retentionSec = retSec
	n.entriesField = entriesField
	n.channelsField = channelsField
	n.channelPrefix = channelPrefix
	n.mu.Unlock()
	return nil
}

// Init 은 전역 레지스트리에 채널을 등록하고 Running 상태로 전이한다.
// 이미 동일 이름이 등록되어 있으면 fail-fast 에러를 반환한다 (SPEC-STORE-002 패턴).
func (n *ChartEmitterNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.RLock()
	name := n.channelName
	buf := n.bufferSize
	ret := n.retentionSec
	flowID := n.flowID
	nodeID := n.nodeID
	channelsField := n.channelsField
	n.mu.RUnlock()

	// 멀티채널 모드: channel_name 불필요, lazy 등록
	if channelsField != "" {
		n.mu.Lock()
		n.dynamicChannels = make(map[string]*system.ChartChannel)
		n.mu.Unlock()
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	if name == "" {
		return ErrChartChannelNameRequired
	}

	reg := GetChartChannelRegistry()
	if reg == nil {
		return fmt.Errorf("chart-emitter: %w: ChartChannelRegistry not initialized", ErrNodeNotInitialized)
	}

	ch, err := reg.Register(name, flowID, nodeID, buf, ret)
	if err != nil {
		if logger := n.BaseNode.Logger(); logger != nil {
			logger.Warn("chart-emitter: channel registration failed",
				"channel_name", name,
				"error", err.Error(),
			)
		}
		return err
	}

	n.mu.Lock()
	n.channel = ch
	ch.SetClock(n.clock)
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지 payload 를 ChartEntry 로 정규화하여 채널에 Publish 한다.
// 출력 메시지는 없다 (sink).
//
// 두 가지 모드:
//
// 1) 배치 모드 (entries_field 설정):
//   - payload[entries_field] 가 []any 이면 각 element 를 독립 ChartEntry 로 변환.
//   - Publish 전에 timestamp 오름차순 정렬 → FIFO 링버퍼가 가득 차면 오래된 timestamp 가
//     evict 되고 최신 N 개가 남는다 (사용자 예상과 일치).
//   - element 가 map 이면 buildChartEntry 규칙 적용.
//   - element 가 primitive (숫자/문자열) 이면 value 로 취급, timestamp 는 clock() 주입.
//   - 필드가 없거나 빈 배열이면 단일 엔트리 모드로 fallback (혼합 입력 허용).
//
// 2) 단일 엔트리 모드 (entries_field 미설정 또는 fallback):
//   - payload 전체에서 buildChartEntry 로 1 개 ChartEntry 생성 후 publish.
//   - 정규화 규칙 (REQ-M1-05):
//     · timestamp (int/int64/float64) 가 있으면 사용, 없으면 clock() 주입.
//     · value 가 있으면 그대로 사용, 없으면 payload 전체를 value 로 감싼다 (timestamp 제외).
//     · labels / meta 는 선택.
func (n *ChartEmitterNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	ch := n.channel
	clock := n.clock
	entriesField := n.entriesField
	channelName := n.channelName
	channelsField := n.channelsField
	channelPrefix := n.channelPrefix
	n.mu.RUnlock()

	// 멀티채널 모드
	if channelsField != "" {
		return n.processMultiChannel(msg, channelsField, channelPrefix, clock)
	}

	if ch == nil {
		return nil, fmt.Errorf("chart-emitter: %w", ErrNodeNotInitialized)
	}

	payload := msg.Payload().ToMap()
	logger := n.BaseNode.Logger()

	// 배치 모드 시도
	if entriesField != "" {
		arr, ok := extractEntriesArray(payload, entriesField)
		if ok && len(arr) > 0 {
			entries := buildChartEntriesFromArray(arr, clock)
			// timestamp 오름차순 정렬: FIFO 링버퍼에 최신 항목이 남도록.
			sort.SliceStable(entries, func(i, j int) bool {
				return entries[i].Timestamp < entries[j].Timestamp
			})
			for _, e := range entries {
				if err := ch.Publish(e); err != nil {
					return nil, fmt.Errorf("chart-emitter: publish failed: %w", err)
				}
			}
			if logger != nil {
				info := ch.Info()
				logger.Info("chart-emitter: batch published",
					"channel", channelName,
					"entries_field", entriesField,
					"batch_size", len(entries),
					"subscribers", info.SubscriberCount,
				)
			}
			return nil, nil
		}
		// 필드가 없거나 배열이 아니면 단일 엔트리 경로로 fallback.
		// 진단: 사용자가 entries_field 를 설정했으나 payload 에 필드가 없으면
		// 로그로 힌트를 남긴다 (가장 흔한 실수).
		if logger != nil {
			keys := make([]string, 0, len(payload))
			for k := range payload {
				keys = append(keys, k)
			}
			logger.Debug("chart-emitter: entries_field fallback to single-entry",
				"channel", channelName,
				"entries_field", entriesField,
				"payload_keys", keys,
				"hint", "entries_field 이 payload 에 없거나 배열이 아님 → 단일 엔트리로 처리",
			)
		}
	}

	// 단일 엔트리 모드
	entry := buildChartEntry(payload, clock)
	if err := ch.Publish(entry); err != nil {
		return nil, fmt.Errorf("chart-emitter: publish failed: %w", err)
	}
	if logger != nil {
		info := ch.Info()
		logger.Debug("chart-emitter: single entry published",
			"channel", channelName,
			"timestamp", entry.Timestamp,
			"subscribers", info.SubscriberCount,
		)
	}
	return nil, nil
}

// processMultiChannel 은 멀티채널 모드: payload[channelsField] 의 map 키별로 채널에 발행한다.
func (n *ChartEmitterNode) processMultiChannel(
	msg message.Message,
	channelsField, channelPrefix string,
	clock func() int64,
) ([]message.Message, error) {
	payload := msg.Payload().ToMap()
	raw, ok := payload[channelsField]
	if !ok {
		return nil, fmt.Errorf("chart-emitter: channels_field %q not found in payload", channelsField)
	}
	dataMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chart-emitter: channels_field %q is not a map", channelsField)
	}

	reg := GetChartChannelRegistry()
	if reg == nil {
		return nil, fmt.Errorf("chart-emitter: %w: ChartChannelRegistry not initialized", ErrNodeNotInitialized)
	}

	n.mu.RLock()
	bufSize := n.bufferSize
	retSec := n.retentionSec
	flowID := n.flowID
	nodeID := n.nodeID
	n.mu.RUnlock()

	for key, val := range dataMap {
		chName := channelPrefix + key
		// 채널 이름 유효성 보정: 영문자로 시작하지 않으면 "ch_" 접두사 추가
		if err := system.ValidateChartChannelName(chName); err != nil {
			chName = "ch_" + chName
		}

		// lazy 채널 등록
		n.mu.Lock()
		ch, exists := n.dynamicChannels[chName]
		if !exists {
			var err error
			ch, err = reg.Register(chName, flowID, nodeID, bufSize, retSec)
			if err != nil {
				n.mu.Unlock()
				return nil, fmt.Errorf("chart-emitter: multi-channel register %q: %w", chName, err)
			}
			ch.SetClock(clock)
			n.dynamicChannels[chName] = ch
		}
		n.mu.Unlock()

		// 값 발행: 배열이면 각 요소를 ChartEntry 로
		arr, isArr := val.([]any)
		if isArr {
			entries := buildChartEntriesFromArray(arr, clock)
			sort.SliceStable(entries, func(i, j int) bool {
				return entries[i].Timestamp < entries[j].Timestamp
			})
			for _, e := range entries {
				if err := ch.Publish(e); err != nil {
					return nil, fmt.Errorf("chart-emitter: multi-channel publish %q: %w", chName, err)
				}
			}
		} else if m, isMap := val.(map[string]any); isMap {
			entry := buildChartEntry(m, clock)
			if err := ch.Publish(entry); err != nil {
				return nil, fmt.Errorf("chart-emitter: multi-channel publish %q: %w", chName, err)
			}
		}
	}
	return nil, nil
}

// Shutdown 은 채널을 레지스트리에서 제거한다 (flow_undeployed 사유로 구독자에게 알림).
func (n *ChartEmitterNode) Shutdown(_ context.Context) error {
	n.mu.Lock()
	ch := n.channel
	name := n.channelName
	n.channel = nil
	dynChannels := n.dynamicChannels
	n.dynamicChannels = nil
	n.mu.Unlock()

	reg := GetChartChannelRegistry()
	if ch != nil && reg != nil && name != "" {
		_ = reg.Unregister(name)
	}
	// 멀티채널 해제
	if reg != nil && dynChannels != nil {
		for chName := range dynChannels {
			_ = reg.Unregister(chName)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// --- 내부 헬퍼 ---

// buildChartEntry 는 payload 로부터 ChartEntry 를 구성한다.
// 규칙은 Process docstring 참조.
func buildChartEntry(payload map[string]any, clock func() int64) system.ChartEntry {
	entry := system.ChartEntry{}

	// timestamp 추출
	ts, hasTs := extractTimestamp(payload)
	if !hasTs {
		ts = clock()
	}
	entry.Timestamp = ts

	// value 추출
	if v, ok := payload["value"]; ok {
		entry.Value = v
	} else {
		// timestamp 키만 제거한 나머지 payload 를 value 로 감싸기
		wrapped := make(map[string]any, len(payload))
		for k, v := range payload {
			if k == "timestamp" {
				continue
			}
			wrapped[k] = v
		}
		entry.Value = wrapped
	}

	// labels 추출 (map[string]string 또는 map[string]any 중 문자열 값만)
	if raw, ok := payload["labels"]; ok {
		if lbls := toStringMap(raw); lbls != nil {
			entry.Labels = lbls
		}
	}

	// meta 추출
	if raw, ok := payload["meta"]; ok {
		if m, ok := raw.(map[string]any); ok {
			entry.Meta = m
		}
	}

	return entry
}

// extractEntriesArray 는 payload[field] 를 []any 로 반환한다.
// payload 의 array 는 []any / []map[string]any / []interface{} 등으로 표현될 수 있으므로
// 모두 통일된 []any 로 수렴시킨다. 필드가 없거나 배열 계열이 아니면 (nil, false).
func extractEntriesArray(payload map[string]any, field string) ([]any, bool) {
	raw, ok := payload[field]
	if !ok || raw == nil {
		return nil, false
	}
	switch arr := raw.(type) {
	case []any:
		return arr, true
	case []map[string]any:
		out := make([]any, len(arr))
		for i, m := range arr {
			out[i] = m
		}
		return out, true
	default:
		return nil, false
	}
}

// buildChartEntriesFromArray 는 배열 요소를 개별 ChartEntry 로 변환한다.
// - map 요소: buildChartEntry 와 동일한 규칙 적용.
// - primitive 요소 (숫자/문자열/불리언): value 로 취급하고 timestamp 는 clock() 주입.
// - nil 요소: 스킵.
func buildChartEntriesFromArray(arr []any, clock func() int64) []system.ChartEntry {
	out := make([]system.ChartEntry, 0, len(arr))
	for _, raw := range arr {
		if raw == nil {
			continue
		}
		switch v := raw.(type) {
		case map[string]any:
			out = append(out, buildChartEntry(v, clock))
		default:
			// primitive: {timestamp: clock(), value: v}
			out = append(out, system.ChartEntry{
				Timestamp: clock(),
				Value:     v,
			})
		}
	}
	return out
}

// extractTimestamp 는 payload 에서 timestamp 값을 int64(epoch ms)로 추출한다.
// int/int64/float64 (정수 값) 를 허용한다. 변환 불가면 false 반환.
func extractTimestamp(payload map[string]any) (int64, bool) {
	raw, ok := payload["timestamp"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		if math.Trunc(v) != v {
			return 0, false
		}
		return int64(v), true
	case float32:
		if math.Trunc(float64(v)) != float64(v) {
			return 0, false
		}
		return int64(v), true
	default:
		return 0, false
	}
}

// toStringMap 은 map[string]string 또는 map[string]any(값이 모두 문자열)를
// map[string]string 으로 변환한다. 그 외에는 nil.
func toStringMap(v any) map[string]string {
	switch m := v.(type) {
	case map[string]string:
		if len(m) == 0 {
			return nil
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	case map[string]any:
		if len(m) == 0 {
			return nil
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			if s, ok := val.(string); ok {
				out[k] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

// chartConfigToInt 는 config 값을 int 로 변환한다. JSON 역직렬화 시 float64 가 들어오므로
// 정수 값인 float64 도 허용한다. fractional 값은 거부.
func chartConfigToInt(v any) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int32:
		return int(x), nil
	case int64:
		return int(x), nil
	case float64:
		if math.Trunc(x) != x {
			return 0, errors.New("must be an integer")
		}
		return int(x), nil
	case float32:
		if math.Trunc(float64(x)) != float64(x) {
			return 0, errors.New("must be an integer")
		}
		return int(x), nil
	default:
		return 0, fmt.Errorf("must be int (got %T)", v)
	}
}
