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
//   - compare_fields: 비교 대상 필드 목록. 두 가지 형식 지원:
//   - 레거시 string: "room_temp:0.5, set_temp, op_mode" (콤마 구분, :tol 옵션)
//   - 신규 array (v0.18.4): [{name: "room_temp", tolerance: 0.5}, {name: "set_temp"}]
//     비어있으면 전체 페이로드 비교.
//   - on_duplicate: 중복 시 처리. "drop"(기본, 폐기) 또는 "reject_port"(reject 포트로 전달).
//   - missing_field_as_different: bool (v0.18.4, 기본 false). true 면 신규 메시지의
//     비교 필드 중 하나라도 부재 시 "다름" 으로 판정하여 통과시킴. false 면 부재 필드를
//     nil 로 간주하고 비교 (이전 값도 nil 이면 동일).
//
// 동작:
//   - key별로 마지막 통과 메시지의 값과 시각을 저장한다.
//   - 새 메시지의 비교 필드가 모두 동일(허용오차 이내)하고 window 내이면 중복으로 판정한다.
//   - window 초과 시 값이 동일해도 강제 통과 (주기적 갱신 보장).
type DeduplicateNode struct {
	*BaseNode

	mu                      sync.RWMutex
	key                     string         // 그룹핑 키 필드명
	window                  time.Duration  // 중복 억제 시간
	fields                  []compareField // 비교 대상 필드 + 허용오차
	allFields               bool           // true면 전체 페이로드 비교
	onDuplicate             string         // "drop" 또는 "reject_port"
	missingFieldAsDifferent bool           // v0.18.4: 부재 필드 → 다름으로 처리

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
	missingDiff := n.missingFieldAsDifferent
	n.mu.RUnlock()

	// 그룹핑 키 추출
	groupKey := "_all"
	if keyField != "" {
		if v, ok := msg.Payload().Get(keyField); ok {
			groupKey = fmt.Sprintf("%v", v)
		}
	}

	// 현재 메시지의 비교 값 추출 + 부재 필드 감지 (v0.18.4)
	currentValues, hasMissing := extractValuesWithMissing(msg.Payload(), fields, allFields)

	now := time.Now()

	n.mu.Lock()
	entry, exists := n.state[groupKey]

	// v0.18.4: missing_field_as_different=true 이고 부재 필드가 있으면 "다름" 으로 즉시 판정.
	dup := exists && now.Sub(entry.lastSeen) < window && valuesEqual(entry.values, currentValues, fields)
	if dup && missingDiff && hasMissing {
		dup = false
	}

	if dup {
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
		// v0.18.4: 두 가지 형식 지원 — string (legacy) 또는 []any (table 형식).
		switch raw := v.(type) {
		case string:
			if raw != "" {
				parsed, err := parseCompareFields(raw)
				if err != nil {
					return fmt.Errorf("deduplicate: %w", err)
				}
				n.fields = parsed
				n.allFields = false
			}
		case []any:
			if len(raw) > 0 {
				parsed, err := parseCompareFieldsArray(raw)
				if err != nil {
					return fmt.Errorf("deduplicate: %w", err)
				}
				n.fields = parsed
				n.allFields = false
			}
		}
	}

	if v, ok := config["on_duplicate"]; ok {
		if s, ok := v.(string); ok {
			n.onDuplicate = s
		}
	}

	// v0.18.4: missing_field_as_different
	if v, ok := config["missing_field_as_different"]; ok {
		if b, ok := v.(bool); ok {
			n.missingFieldAsDifferent = b
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

// parseCompareFieldsArray 는 v0.18.4 의 array (table) 형식을 파싱한다.
// 각 원소는 map[string]any 로 {name: string, tolerance?: number} 형태.
func parseCompareFieldsArray(arr []any) ([]compareField, error) {
	result := make([]compareField, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("compare_fields[%d]: expected object, got %T", i, item)
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue // 빈 행 무시
		}
		cf := compareField{name: name}
		if tolRaw, ok := m["tolerance"]; ok && tolRaw != nil {
			tol, ok := toFloat64(tolRaw)
			if !ok {
				return nil, fmt.Errorf("compare_fields[%d].tolerance: invalid number %v", i, tolRaw)
			}
			cf.tolerance = math.Abs(tol)
		}
		result = append(result, cf)
	}
	return result, nil
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
	result, _ := extractValuesWithMissing(payload, fields, allFields)
	return result
}

// extractValuesWithMissing 는 비교 대상 값과 부재 필드 존재 여부를 함께 반환한다 (v0.18.4).
// hasMissing=true 면 fields 중 페이로드에 존재하지 않는 키가 하나 이상 있음.
// allFields 모드에서는 hasMissing 이 항상 false (모든 키가 페이로드에서 추출됨).
func extractValuesWithMissing(payload message.Payload, fields []compareField, allFields bool) (map[string]any, bool) {
	if allFields {
		m := payload.ToMap()
		result := make(map[string]any, len(m))
		for k, v := range m {
			if !skipFields[k] {
				result[k] = v
			}
		}
		return result, false
	}
	result := make(map[string]any, len(fields))
	hasMissing := false
	for _, f := range fields {
		v, ok := payload.Get(f.name)
		if !ok {
			hasMissing = true
		}
		result[f.name] = v
	}
	return result, hasMissing
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
