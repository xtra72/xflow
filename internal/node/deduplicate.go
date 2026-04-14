package node

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// DeduplicateNode 는 지정 시간 창(window) 내에서 동일한 메시지를 제거하는 노드이다.
//
// 설정:
//   - key: 메시지 그룹핑 키 (페이로드 필드명). 빈 문자열이면 전체 메시지 기준.
//   - window: 중복 억제 시간 (예: "30s"). 기본값 30초.
//   - compare_fields: 비교 대상 필드 목록 (콤마 구분). 비어있으면 전체 페이로드 비교.
//     허용오차 지정 가능: "room_temp:0.5, set_temp, op_mode"
//     숫자 필드에 :N 을 붙이면 |현재-이전| > N 일 때만 변경으로 판정.
//   - on_duplicate: 중복 시 처리. "drop"(기본, 폐기) 또는 "reject_port"(reject 포트로 전달).
//
// 동작:
//   - key별로 마지막 통과 메시지의 값과 시각을 저장한다.
//   - 새 메시지의 비교 필드가 모두 동일(허용오차 이내)하고 window 내이면 중복으로 판정한다.
//   - window 초과 시 값이 동일해도 강제 통과 (주기적 갱신 보장).
type DeduplicateNode struct {
	*BaseNode

	mu          sync.RWMutex
	key         string           // 그룹핑 키 필드명
	window      time.Duration    // 중복 억제 시간
	fields      []compareField   // 비교 대상 필드 + 허용오차
	allFields   bool             // true면 전체 페이로드 비교
	onDuplicate string           // "drop" 또는 "reject_port"

	// 키별 마지막 통과 상태
	state map[string]*deduplicateEntry
}

// compareField 는 비교 대상 필드와 허용오차이다.
type compareField struct {
	name      string
	tolerance float64 // 0이면 완전 일치
}

// deduplicateEntry 는 키별 마지막 통과 메시지 상태이다.
type deduplicateEntry struct {
	values   map[string]any // 마지막 통과 시 필드 값
	lastSeen time.Time
}

// NewDeduplicateNode 는 새 DeduplicateNode를 생성한다.
func NewDeduplicateNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DeduplicateNode{
		BaseNode:    base,
		window:      30 * time.Second,
		allFields:   true,
		onDuplicate: "drop",
		state:       make(map[string]*deduplicateEntry),
	}
	return n, nil
}

// Init 은 노드를 초기화한다.
func (n *DeduplicateNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지의 중복 여부를 판정한다.
func (n *DeduplicateNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	keyField := n.key
	window := n.window
	fields := n.fields
	allFields := n.allFields
	onDup := n.onDuplicate
	n.mu.RUnlock()

	// 그룹핑 키 추출
	groupKey := "_all"
	if keyField != "" {
		if v, ok := msg.Payload().Get(keyField); ok {
			groupKey = fmt.Sprintf("%v", v)
		}
	}

	// 현재 메시지의 비교 값 추출
	currentValues := extractValues(msg.Payload(), fields, allFields)

	now := time.Now()

	n.mu.Lock()
	entry, exists := n.state[groupKey]

	if exists && now.Sub(entry.lastSeen) < window && valuesEqual(entry.values, currentValues, fields) {
		// 중복: window 내 동일 값 (허용오차 이내)
		n.mu.Unlock()

		switch onDup {
		case "reject_port":
			out := msg.Clone()
			out.Metadata().Set("_target_port", "reject")
			return []message.Message{out}, nil
		default:
			return []message.Message{}, nil
		}
	}

	// 신규 또는 변경 또는 window 초과 → 통과
	n.state[groupKey] = &deduplicateEntry{
		values:   currentValues,
		lastSeen: now,
	}
	n.mu.Unlock()

	return []message.Message{msg}, nil
}

// Shutdown 은 노드를 종료한다.
func (n *DeduplicateNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 노드 설정을 적용한다.
func (n *DeduplicateNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if v, ok := config["key"]; ok {
		if s, ok := v.(string); ok {
			n.key = s
		}
	}

	if v, ok := config["window"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("deduplicate: invalid window %q: %w", s, err)
			}
			n.window = d
		}
	}

	if v, ok := config["compare_fields"]; ok {
		if s, ok := v.(string); ok && s != "" {
			parsed, err := parseCompareFields(s)
			if err != nil {
				return fmt.Errorf("deduplicate: %w", err)
			}
			n.fields = parsed
			n.allFields = false
		}
	}

	if v, ok := config["on_duplicate"]; ok {
		if s, ok := v.(string); ok {
			n.onDuplicate = s
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// 내부 함수
// ---------------------------------------------------------------------------

// skipFields 는 전체 비교 시 항상 변하는 필드를 제외한다.
var skipFields = map[string]bool{
	"timestamp": true, "seq": true, "raw_hex": true,
}

// parseCompareFields 는 "room_temp:0.5, set_temp, op_mode" 형식을 파싱한다.
func parseCompareFields(s string) ([]compareField, error) {
	parts := strings.Split(s, ",")
	result := make([]compareField, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cf := compareField{}
		if idx := strings.IndexByte(p, ':'); idx >= 0 {
			cf.name = strings.TrimSpace(p[:idx])
			tolStr := strings.TrimSpace(p[idx+1:])
			tol, err := strconv.ParseFloat(tolStr, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid tolerance for %q: %w", cf.name, err)
			}
			cf.tolerance = math.Abs(tol)
		} else {
			cf.name = p
		}
		result = append(result, cf)
	}
	return result, nil
}

// extractValues 는 페이로드에서 비교 대상 값을 추출한다.
func extractValues(payload message.Payload, fields []compareField, allFields bool) map[string]any {
	if allFields {
		m := payload.ToMap()
		result := make(map[string]any, len(m))
		for k, v := range m {
			if !skipFields[k] {
				result[k] = v
			}
		}
		return result
	}
	result := make(map[string]any, len(fields))
	for _, f := range fields {
		v, _ := payload.Get(f.name)
		result[f.name] = v
	}
	return result
}

// valuesEqual 은 이전 값과 현재 값이 동일한지 비교한다.
// fields에 허용오차가 지정된 숫자 필드는 |prev-curr| <= tolerance 이면 동일.
// allFields 모드(fields가 비어있음)에서는 완전 일치만 사용.
func valuesEqual(prev, curr map[string]any, fields []compareField) bool {
	if len(fields) == 0 {
		// 전체 비교: 키 수 + 완전 일치
		if len(prev) != len(curr) {
			return false
		}
		for k, pv := range prev {
			cv, ok := curr[k]
			if !ok {
				return false
			}
			if fmt.Sprintf("%v", pv) != fmt.Sprintf("%v", cv) {
				return false
			}
		}
		return true
	}

	// 지정 필드 비교 (허용오차 포함)
	for _, f := range fields {
		pv := prev[f.name]
		cv := curr[f.name]

		if f.tolerance > 0 {
			// 숫자 허용오차 비교
			pn, pOk := toFloat64(pv)
			cn, cOk := toFloat64(cv)
			if pOk && cOk {
				if math.Abs(pn-cn) > f.tolerance {
					return false
				}
				continue
			}
			// 숫자 변환 실패 → 문자열 비교 폴백
		}

		// 완전 일치
		if fmt.Sprintf("%v", pv) != fmt.Sprintf("%v", cv) {
			return false
		}
	}
	return true
}

// toFloat64 는 aggregate.go에 정의되어 있다.
