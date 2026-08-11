// Package node - split.go: Split 노드 (배열 payload → N 메시지 팬아웃)
//
// Split 노드는 하나의 입력 메시지를 받아, config 로 지정한 위치(path)의 배열을
// 꺼내 요소마다 한 개씩 N개의 메시지로 팬아웃한다. 팬아웃은 이미 파이프라인
// 계약이다 — Node.Process 는 []message.Message 를 반환하며, InventoryNode(청크
// 팬아웃)·FramerNode(프레임 팬아웃)와 동일한 계약을 사용한다.
//
// 요소 타입은 두 케이스를 지원한다:
//   - payloads (케이스 A): 각 요소가 새 메시지의 payload 가 되며, 부모 메타/타입/
//     타임스탬프를 공유한다.
//   - messages (케이스 B): 각 요소를 완전한 메시지 객체({metadata, payload, type?,
//     timestamp?})로 취급하여 재구성한다. 메타데이터는 부모(공유) base 에 요소
//     메타가 override(요소 우선)된다.
//
// mode=auto(기본)는 요소별로 자동 감지한다: 요소가 metadata/payload 키를 가진
// map 이면 messages, 아니면 payloads.
package node

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 (mode / on_missing / on_empty enum 및 요소 예약 키)
// ---------------------------------------------------------------------------

const (
	// splitModeAuto 는 요소별 자동 감지 모드이다 (기본값).
	splitModeAuto = "auto"
	// splitModePayloads 는 각 요소를 payload 로 취급하는 모드이다.
	splitModePayloads = "payloads"
	// splitModeMessages 는 각 요소를 완전한 메시지 객체로 취급하는 모드이다.
	splitModeMessages = "messages"

	// splitOnMissingPassthrough 는 path 누락/비배열 시 입력을 그대로 방출한다 (기본값).
	splitOnMissingPassthrough = "passthrough"
	// splitOnMissingError 는 path 누락/비배열 시 에러를 반환한다.
	splitOnMissingError = "error"

	// splitOnEmptyEmitNone 는 빈 배열 시 0개 메시지를 반환한다 (기본값).
	splitOnEmptyEmitNone = "emit_none"
	// splitOnEmptyPassthrough 는 빈 배열 시 입력을 그대로 방출한다.
	splitOnEmptyPassthrough = "passthrough"

	// splitDefaultScalarKey 는 payloads 모드에서 비객체 요소를 감쌀 기본 키이다.
	splitDefaultScalarKey = "value"

	// 요소 메시지 객체(케이스 B)의 예약 키
	splitElemKeyMetadata  = "metadata"
	splitElemKeyPayload   = "payload"
	splitElemKeyType      = "type"
	splitElemKeyTimestamp = "timestamp"
)

// ---------------------------------------------------------------------------
// Sentinel 에러
// ---------------------------------------------------------------------------

var (
	// ErrSplitInvalidConfig 는 잘못된 config 타입/enum 값일 때 반환된다.
	ErrSplitInvalidConfig = fmt.Errorf("split: %w: invalid config", ErrInvalidConfig)
	// ErrSplitPathMissing 은 필수 path 가 누락/빈 값일 때 반환된다.
	ErrSplitPathMissing = fmt.Errorf("split: %w: missing or empty 'path' field", ErrInvalidConfig)
	// ErrSplitPathNotArray 는 on_missing=error 에서 path 가 배열로 해석되지 않을 때 반환된다.
	ErrSplitPathNotArray = fmt.Errorf("split: path did not resolve to an array")
)

// ---------------------------------------------------------------------------
// SplitNode 구조체
// ---------------------------------------------------------------------------

// SplitNode 는 입력 메시지 payload 의 배열을 요소별 N개 메시지로 팬아웃하는
// 처리 노드이다. 외부 의존성이 없는 순수 payload 변환 노드이다.
//
// @MX:NOTE: [AUTO] SPEC-MESSAGE-SPLIT-001 — path 기반 배열 팬아웃, payloads/messages/auto 3모드.
type SplitNode struct {
	*BaseNode

	path          string // "items"(payload 단축) 또는 "$.payload.items[*]"(메시지 루트)
	isJSONPath    bool   // path 가 "$." 로 시작하는지
	mode          string // auto | payloads | messages
	shareMetadata bool   // split 메시지에 부모 메타 공유 여부
	scalarKey     string // payloads 모드 비객체 요소 래핑 키
	onMissing     string // passthrough | error
	onEmpty       string // emit_none | passthrough
}

// 컴파일 타임 인터페이스 체크
var _ Node = (*SplitNode)(nil)

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// NewSplitNode 는 NodeDef 와 옵션으로부터 split 노드를 생성한다.
//
// config 키:
//   - path (string, required): 배열 위치(메시지 루트). 평면 top-level payload
//     키 단축("items" = msg.payload.items, Payload().Get) 또는 "$." 접두 메시지
//     루트 JSONPath("$.payload.items[*]" = msg.payload.items, messageToMap 리졸버).
//   - mode (enum, optional, default "auto"): auto | payloads | messages.
//   - share_metadata (bool, optional, default true): 부모 메타 공유 여부.
//   - scalar_key (string, optional, default "value"): 비객체 요소 래핑 키.
//   - on_missing (enum, optional, default "passthrough"): passthrough | error.
//   - on_empty (enum, optional, default "emit_none"): emit_none | passthrough.
//
// nil/누락/잘못된 config 는 sentinel 에러를 반환하며 panic 하지 않는다 (REQ-03).
//
// @MX:ANCHOR: [AUTO] 빌트인 레지스트리 등록(NodeFactory) 진입점.
// @MX:REASON: registerBuiltins() 테이블 + 테스트에서 참조되는 팩토리 계약이다.
func NewSplitNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	cfg := def.Config // nil map 접근은 Go 에서 안전하다 (zero value 반환).

	path, err := parseSplitPath(cfg)
	if err != nil {
		return nil, err
	}
	mode, err := parseSplitEnum(cfg, "mode", splitModeAuto,
		splitModeAuto, splitModePayloads, splitModeMessages)
	if err != nil {
		return nil, err
	}
	onMissing, err := parseSplitEnum(cfg, "on_missing", splitOnMissingPassthrough,
		splitOnMissingPassthrough, splitOnMissingError)
	if err != nil {
		return nil, err
	}
	onEmpty, err := parseSplitEnum(cfg, "on_empty", splitOnEmptyEmitNone,
		splitOnEmptyEmitNone, splitOnEmptyPassthrough)
	if err != nil {
		return nil, err
	}
	shareMetadata, err := parseSplitBool(cfg, "share_metadata", true)
	if err != nil {
		return nil, err
	}
	scalarKey, err := parseSplitScalarKey(cfg)
	if err != nil {
		return nil, err
	}

	return &SplitNode{
		BaseNode:      base,
		path:          path,
		isJSONPath:    strings.HasPrefix(path, "$."),
		mode:          mode,
		shareMetadata: shareMetadata,
		scalarKey:     scalarKey,
		onMissing:     onMissing,
		onEmpty:       onEmpty,
	}, nil
}

// parseSplitPath 는 필수 path 키를 추출/검증한다.
func parseSplitPath(cfg map[string]any) (string, error) {
	raw, ok := cfg["path"]
	if !ok || raw == nil {
		return "", ErrSplitPathMissing
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: 'path' must be string, got %T", ErrSplitInvalidConfig, raw)
	}
	if strings.TrimSpace(s) == "" {
		return "", ErrSplitPathMissing
	}
	return s, nil
}

// parseSplitEnum 은 선택적 enum 문자열 키를 파싱한다. 미지정이면 def, 허용 목록에
// 없으면 에러를 반환한다.
func parseSplitEnum(cfg map[string]any, key, def string, allowed ...string) (string, error) {
	raw, ok := cfg[key]
	if !ok || raw == nil {
		return def, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: '%s' must be string, got %T", ErrSplitInvalidConfig, key, raw)
	}
	for _, a := range allowed {
		if s == a {
			return s, nil
		}
	}
	return "", fmt.Errorf("%w: unknown %s %q", ErrSplitInvalidConfig, key, s)
}

// parseSplitBool 은 선택적 bool 키를 파싱한다. 미지정이면 def 를 반환한다.
func parseSplitBool(cfg map[string]any, key string, def bool) (bool, error) {
	raw, ok := cfg[key]
	if !ok || raw == nil {
		return def, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%w: '%s' must be bool, got %T", ErrSplitInvalidConfig, key, raw)
	}
	return b, nil
}

// parseSplitScalarKey 는 선택적 scalar_key 를 파싱한다. 미지정/빈 값이면 기본값.
func parseSplitScalarKey(cfg map[string]any) (string, error) {
	raw, ok := cfg["scalar_key"]
	if !ok || raw == nil {
		return splitDefaultScalarKey, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: 'scalar_key' must be string, got %T", ErrSplitInvalidConfig, raw)
	}
	if s == "" {
		return splitDefaultScalarKey, nil
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init 은 노드를 초기화한다. split 은 stateless 이므로 상태 전이만 수행한다.
func (n *SplitNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 종료한다. split 은 stateless 이므로 정리 작업이 없다.
func (n *SplitNode) Shutdown(_ context.Context) error {
	state := n.BaseNode.CurrentState()
	if state == lifecycle.StateStopped {
		return nil
	}
	if err := n.BaseNode.TransitionTo(lifecycle.StateStopping); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopped)
}

// ---------------------------------------------------------------------------
// Process - 핵심 로직 (수집 → 엣지 → 요소별 팬아웃)
// ---------------------------------------------------------------------------

// Process 는 입력 메시지 payload 의 path 배열을 요소별 N개 메시지로 팬아웃한다.
// 처리 순서: (1) 배열 추출 → (2) 누락/비배열 엣지(on_missing) → (3) 빈 배열
// 엣지(on_empty) → (4) 요소별 팬아웃. 입력 배열 순서는 출력에서 보존된다.
func (n *SplitNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	arr, ok := n.extractArray(msg)
	if !ok {
		// (2) 누락/비배열 (REQ-18)
		if n.onMissing == splitOnMissingError {
			return nil, fmt.Errorf("%w: path=%q", ErrSplitPathNotArray, n.path)
		}
		if n.logger != nil {
			n.logger.Warn("split: path 누락 또는 비배열, passthrough", "path", n.path)
		}
		return []message.Message{msg}, nil
	}

	// (3) 빈 배열 (REQ-19)
	if len(arr) == 0 {
		if n.onEmpty == splitOnEmptyPassthrough {
			return []message.Message{msg}, nil
		}
		return nil, nil
	}

	// (4) 요소별 팬아웃
	results := make([]message.Message, 0, len(arr))
	for _, elem := range arr {
		out := n.buildSplitMessage(msg, elem)
		results = append(results, out)
	}
	return results, nil
}

// extractArray 는 path 에서 배열을 추출한다. "$." 접두 JSONPath 는 메시지 루트
// 리졸버(messageToMap → NewPayload → GetPath, mapping.go:111 과 동일 패턴)로
// 메시지 전체 기준 해석하고("$.payload.items" = msg.payload.items,
// "$.metadata.x" = msg.metadata.x), 평면(비-"$.") 키는 하위호환 편의로
// top-level payload 키 단축(Payload().Get)으로 해석한다. 결과를 []any 로
// 변환하며, 누락/비슬라이스면 (nil, false)를 반환한다.
func (n *SplitNode) extractArray(msg message.Message) ([]any, bool) {
	var raw any
	if n.isJSONPath {
		// 메시지 루트 리졸버: "$." 경로를 payload 가 아닌 메시지 전체에 루팅한다.
		// 코드베이스 전역 관례(mapping.go/store_write.go/enrich.go)와 일치한다.
		v, err := message.NewPayload(messageToMap(msg)).GetPath(n.path)
		if err != nil {
			return nil, false
		}
		raw = v
	} else {
		v, ok := msg.Payload().Get(n.path)
		if !ok {
			return nil, false
		}
		raw = v
	}
	return splitToAnySlice(raw)
}

// buildSplitMessage 는 요소 하나를 split 메시지로 변환한다. 모드 판정에 따라
// 케이스 A(payloads) 또는 케이스 B(messages)로 분기한다.
func (n *SplitNode) buildSplitMessage(parent message.Message, elem any) message.Message {
	if n.elementIsMessage(elem) {
		return n.buildFromMessageElem(parent, elem.(map[string]any))
	}
	return n.buildFromPayloadElem(parent, elem)
}

// elementIsMessage 는 요소를 케이스 B(messages)로 취급할지 판정한다.
//   - payloads: 항상 false
//   - messages: 요소가 map 이면 true, 아니면 false(payload 폴백)
//   - auto: 요소가 metadata/payload 키를 가진 map 이면 true
func (n *SplitNode) elementIsMessage(elem any) bool {
	switch n.mode {
	case splitModePayloads:
		return false
	case splitModeMessages:
		_, ok := elem.(map[string]any)
		return ok
	default: // auto
		m, ok := elem.(map[string]any)
		if !ok {
			return false
		}
		_, hasMeta := m[splitElemKeyMetadata]
		_, hasPayload := m[splitElemKeyPayload]
		return hasMeta || hasPayload
	}
}

// buildFromPayloadElem 은 케이스 A: 요소를 새 메시지의 payload 로 만든다.
// 요소가 map 이면 그 자체, 스칼라면 scalar_key 로 감싼다(REQ-16). 부모 type/
// timestamp 를 보존하고 새 UUID 를 부여하며, share_metadata 면 부모 메타를 공유한다.
func (n *SplitNode) buildFromPayloadElem(parent message.Message, elem any) message.Message {
	var payloadMap map[string]any
	if m, ok := elem.(map[string]any); ok {
		payloadMap = m
	} else {
		payloadMap = map[string]any{n.scalarKey: elem}
	}
	out := message.New(
		message.WithType(parent.Type()),
		message.WithTimestamp(parent.Timestamp()),
		message.WithPayload(message.NewPayload(payloadMap)),
	)
	if n.shareMetadata {
		splitShareMetadata(parent.Metadata(), out.Metadata())
	}
	return out
}

// buildFromMessageElem 은 케이스 B: 요소를 완전한 메시지 객체로 재구성한다.
// payload = 요소의 payload(map), type/timestamp = 요소 값이 있으면 우선(REQ-15),
// 없으면 부모 값. 메타데이터는 부모(공유) base 에 요소 메타가 override(요소 우선,
// REQ-12). 비문자열 요소 메타 값은 문자열화한다.
func (n *SplitNode) buildFromMessageElem(parent message.Message, elem map[string]any) message.Message {
	// payload
	payloadMap := map[string]any{}
	if p, ok := elem[splitElemKeyPayload].(map[string]any); ok {
		payloadMap = p
	}

	// type (요소 우선, 없으면 부모)
	msgType := parent.Type()
	if t, ok := elem[splitElemKeyType].(string); ok && t != "" {
		msgType = t
	}

	// timestamp (요소 우선, 없으면 부모)
	ts := parent.Timestamp()
	if v, ok := elem[splitElemKeyTimestamp]; ok {
		if parsed, ok := parseElemTimestamp(v); ok {
			ts = parsed
		}
	}

	out := message.New(
		message.WithType(msgType),
		message.WithTimestamp(ts),
		message.WithPayload(message.NewPayload(payloadMap)),
	)

	// 메타 병합: 부모(공유) base → 요소 override
	if n.shareMetadata {
		splitShareMetadata(parent.Metadata(), out.Metadata())
	}
	if rawMeta, ok := elem[splitElemKeyMetadata].(map[string]any); ok {
		for k, v := range rawMeta {
			out.Metadata().Set(k, stringifyMetaValue(v))
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitShareMetadata 는 src 메타데이터를 dst 로 복사한다. flat string 값은 Set,
// group(map[string]string) 값은 SetGroup 으로 보존한다(REQ-11).
func splitShareMetadata(src, dst message.Metadata) {
	for k, v := range src.Raw() {
		switch val := v.(type) {
		case string:
			dst.Set(k, val)
		case map[string]string:
			dst.SetGroup(k, val)
		}
	}
}

// stringifyMetaValue 는 요소 메타 값을 메타데이터 값 계약(string only)에 맞게
// 문자열화한다. 이미 string 이면 그대로, 아니면 fmt.Sprintf("%v", v)(REQ-12).
func stringifyMetaValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// parseElemTimestamp 는 요소의 timestamp 값을 time.Time 으로 파싱한다.
// epoch ms(int/int64/float64), RFC3339 문자열, time.Time 을 지원한다. 파싱 실패
// 시 (zero, false)를 반환하여 호출측이 부모 값을 유지하도록 한다.
func parseElemTimestamp(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case int64:
		return time.UnixMilli(t), true
	case int:
		return time.UnixMilli(int64(t)), true
	case float64:
		return time.UnixMilli(int64(t)), true
	case string:
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed, true
		}
		return time.Time{}, false
	default:
		return time.Time{}, false
	}
}

// splitToAnySlice 는 임의의 슬라이스 타입을 []any 로 변환한다([]any/[]map[string]any
// /[]string 등 수용). 비슬라이스이거나 nil 이면 (nil, false)를 반환한다.
// pkg/message.toAnySlice 와 동일 정책이나 해당 함수가 unexported 라 재구현한다.
func splitToAnySlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if arr, ok := v.([]any); ok {
		return arr, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, false
	}
	nlen := rv.Len()
	out := make([]any, nlen)
	for i := 0; i < nlen; i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}
