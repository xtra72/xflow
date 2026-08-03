package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// ModbusRemapNode - MODBUS 레지스터 재매핑 노드 (SPEC-MODBUS-007)
// ---------------------------------------------------------------------------
//
// modbus-remap 은 modbus-read 노드가 방출한 positional values[] 페이로드를 소비하여
// 각 read-op 엔트리를 remap 규칙에 따라 재주소화(주소+영역+device_id)한다.
// 라이브 에이전트/RegisterMap 을 참조하지 않는 stateless 변환 노드이다(mapping.go 계층).
//
// ── 입력 계약 (modbus-read 출력) ─────────────────────────────────────────────
//   { success: bool, agent_type: "server"|"client",
//     values: [ { index, area, address, count, values[]|raw, data_type? } ] }
//   - 엔트리에는 unit_id 가 없다. From 매칭은 (area, address, count) 정확 매칭.
//
// ── config 스키마 (프론트 RegisterRemapEditor 가 이 shape 를 그대로 emit) ──────
//   rules: []map[string]any        // 각 규칙(From→To):
//     {
//       "source_area":    string,  // From 영역 (coils|discrete_inputs|holding_registers|input_registers)
//       "source_address": number,  // From 시작 주소 (엔트리 base)
//       "count":          number,  // 레지스터 수 (엔트리 count 와 정확 일치해야 함)
//       "target_unit_id": number,  // To device_id/unit_id (To측 할당값)
//       "target_area":    string?, // To 영역 (생략 시 source_area 유지)
//       "target_address": number   // To 시작 주소 (targetStart)
//     }
//   templates: []map[string]any    // 각 템플릿({area, offset} + 적용 파라미터):
//     {
//       "area":        string,     // 템플릿 영역 (= source_area, target_area 기본값)
//       "offset":      number,     // 주소 오프셋 (target_address = start + offset; 음수 허용)
//       "device_id":   number,     // 적용: target_unit_id
//       "start":       number,     // 적용: source_address
//       "count":       number,     // 적용: count
//       "target_area": string?     // 적용: To 영역 (생략 시 area 유지)
//     }
//   - 템플릿은 Process 시점에 구체 규칙으로 인스턴스화되어 rules 뒤에 append 된다.
//
// ── payload 오버라이드 (modbus_read.go command_set 선례) ──────────────────────
//   payload 에 "rules"(또는 "command_set") 키가 있으면 config 기본 rules 를 대체하고,
//   payload 에 "templates" 키가 있으면 config 기본 templates 를 대체한다(각 독립).
//
// ── 출력 스키마 (downstream modbus-write command_set 소비 호환) ────────────────
//   { success: bool,
//     values: [ { index, area, address, count, unit_id, values[]|raw, data_type? } ],
//     errors?: [ { rule_index, source_area, source_address, count, reason } ],
//     agent_type: string, timestamp: int64(epoch-ms) }
//   - 매칭된 규칙마다 재매핑 엔트리 1개를 emit. 미매칭 규칙은 거부(부분 매핑 금지)하고
//     errors 에 규칙별 사유를 기록하며 success=false.
//   - 값(values/raw)과 data_type 은 그대로 보존(주소·영역·unit_id 만 재작성).
//
// ── 에러 라우팅 정책(코디네이터 확정 + acceptance.md AC-03/AC-05 해석) ──────────
//   - 구조적/입력계약 실패(비-modbus-read 페이로드, 빈 규칙 셋)만 return (nil, err)
//     → error port 로 라우팅한다(AC-05, 빈 규칙 셋 엣지).
//   - 규칙별 미매칭은 payload 로 승계한다: success=false + errors[] 를 정상 출력으로
//     emit 한다(AC-03 의미: 거부·부분매핑 금지·규칙별 error 표기). 노드 프레임워크는
//     payload 방출과 error port 라우팅을 동시에 할 수 없으므로, downstream 이 실패를
//     관측할 수 있도록 success=false payload 를 정상 포트로 내보낸다.

var (
	// ErrModbusRemapInvalidInput 은 입력이 modbus-read 페이로드(values[])가 아닐 때 반환된다.
	ErrModbusRemapInvalidInput = fmt.Errorf("modbus-remap: input is not a modbus-read payload (missing values[] list)")

	// ErrModbusRemapEmptyRules 는 config/payload 어느 쪽에서도 규칙/템플릿이 없을 때 반환된다.
	ErrModbusRemapEmptyRules = fmt.Errorf("modbus-remap: %w: no rules or templates configured", ErrInvalidConfig)
)

// ModbusRemapNode 는 modbus-read 출력의 각 엔트리를 remap 규칙으로 재주소화하는 노드이다.
type ModbusRemapNode struct {
	*BaseNode
	rules     []map[string]any // config 기본 규칙 목록
	templates []map[string]any // config 기본 템플릿 목록
	mu        sync.RWMutex     // 설정 보호 뮤텍스
}

// 인터페이스 컴파일 체크
var _ Node = (*ModbusRemapNode)(nil)

// ---------------------------------------------------------------------------
// 팩토리 / 라이프사이클
// ---------------------------------------------------------------------------

// NewModbusRemapNode 는 새로운 ModbusRemapNode를 생성하는 팩토리 함수이다.
func NewModbusRemapNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	return &ModbusRemapNode{BaseNode: base}, nil
}

// Init 은 ModbusRemapNode를 초기화한다(stateless — 에이전트 resolve 불필요).
func (n *ModbusRemapNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Configure 는 ModbusRemapNode의 설정을 적용한다.
// rules(선택), templates(선택)를 파싱한다. 둘 다 비어 있으면 Process 시점에 거부된다.
func (n *ModbusRemapNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	rules := toOpList(config["rules"])
	templates := toOpList(config["templates"])

	n.mu.Lock()
	n.rules = rules
	n.templates = templates
	n.mu.Unlock()
	return nil
}

// Shutdown 은 ModbusRemapNode를 종료한다.
func (n *ModbusRemapNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 입력 values[] 엔트리를 remap 규칙에 따라 재주소화하여 결과 1건을 emit한다.
func (n *ModbusRemapNode) Process(_ context.Context, msg message.Message) (result []message.Message, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus-remap: panic recovered: %v", r)
		}
	}()

	// 1) 유효 규칙 집합 = (config|payload) rules + 인스턴스화된 (config|payload) templates
	effective, err := n.effectiveRules(msg)
	if err != nil {
		return nil, err
	}

	// 2) 입력 파싱 (modbus-read 계약 검증)
	entries, ok := parseRemapInputEntries(msg)
	if !ok {
		return nil, ErrModbusRemapInvalidInput
	}
	inputSuccess := true
	agentType := ""
	if p := msg.Payload(); p != nil {
		if v, ok := p.Get("success"); ok {
			if b, ok := v.(bool); ok {
				inputSuccess = b
			}
		}
		if v, ok := p.Get("agent_type"); ok {
			if s, ok := v.(string); ok {
				agentType = s
			}
		}
	}

	// 3) 규칙별 remap (엔트리 전체 정확 매칭, 미매칭 거부)
	outValues := make([]map[string]any, 0, len(effective))
	var errs []map[string]any
	allMatched := true

	for ri, rm := range effective {
		rule := parseRemapRule(rm)

		if !validRegisterAreas[rule.sourceArea] {
			errs = append(errs, remapRuleError(ri, rule, "invalid source area: "+rule.sourceArea))
			allMatched = false
			continue
		}
		if !validRegisterAreas[rule.targetArea] {
			errs = append(errs, remapRuleError(ri, rule, "invalid target area: "+rule.targetArea))
			allMatched = false
			continue
		}

		entry := findExactEntry(entries, rule)
		if entry == nil {
			errs = append(errs, remapRuleError(ri, rule, fmt.Sprintf(
				"no entry exactly matches source (area=%s, address=%d, count=%d)",
				rule.sourceArea, rule.sourceAddress, rule.count)))
			allMatched = false
			continue
		}

		outValues = append(outValues, remapEntry(entry, rule, len(outValues)))
	}

	// 4) 출력 구성. 입력 실패 상태를 승계(왜곡 금지).
	success := inputSuccess && allMatched
	out := msg.Clone()
	out.Payload().Set("success", success)
	out.Payload().Set("values", outValues)
	if len(errs) > 0 {
		out.Payload().Set("errors", errs)
	}
	if agentType != "" {
		out.Payload().Set("agent_type", agentType)
	}
	out.Payload().Set("timestamp", time.Now().UnixMilli())
	out.SetType("response")
	return []message.Message{out}, nil
}

// effectiveRules 는 config 기본값과 payload 오버라이드를 결합하여 유효 규칙 맵 목록을 만든다.
// payload 의 rules(또는 command_set) / templates 키가 있으면 각 config 기본값을 대체한다.
func (n *ModbusRemapNode) effectiveRules(msg message.Message) ([]map[string]any, error) {
	n.mu.RLock()
	rulesCfg := n.rules
	templatesCfg := n.templates
	n.mu.RUnlock()

	if p := msg.Payload(); p != nil {
		if v, ok := p.Get("rules"); ok {
			if o := toOpList(v); len(o) > 0 {
				rulesCfg = o
			}
		} else if v, ok := p.Get("command_set"); ok {
			if o := toOpList(v); len(o) > 0 {
				rulesCfg = o
			}
		}
		if v, ok := p.Get("templates"); ok {
			if o := toOpList(v); len(o) > 0 {
				templatesCfg = o
			}
		}
	}

	effective := make([]map[string]any, 0, len(rulesCfg)+len(templatesCfg))
	effective = append(effective, rulesCfg...)
	for _, t := range templatesCfg {
		effective = append(effective, instantiateRemapTemplate(t))
	}
	if len(effective) == 0 {
		return nil, ErrModbusRemapEmptyRules
	}
	return effective, nil
}

// ---------------------------------------------------------------------------
// 규칙 / 템플릿 파싱
// ---------------------------------------------------------------------------

// remapRule 는 파싱된 단일 remap 규칙이다.
type remapRule struct {
	sourceArea    string
	sourceAddress uint16
	count         uint16
	targetUnitID  uint8
	targetArea    string
	targetAddress uint16
}

// parseRemapRule 은 규칙 맵을 remapRule로 파싱한다.
// target_area 가 없으면 source_area 를 유지한다.
func parseRemapRule(m map[string]any) remapRule {
	r := remapRule{}
	r.sourceArea, _ = m["source_area"].(string)
	r.sourceAddress = toUint16FromAny(m["source_address"])
	r.count = toUint16FromAny(m["count"])
	if r.count == 0 {
		r.count = 1
	}
	r.targetUnitID = toByte(m["target_unit_id"])
	if s, ok := m["target_area"].(string); ok && s != "" {
		r.targetArea = s
	} else {
		r.targetArea = r.sourceArea
	}
	r.targetAddress = toUint16FromAny(m["target_address"])
	return r
}

// instantiateRemapTemplate 은 템플릿({area, offset} + 적용 파라미터)을 구체 규칙 맵으로 전개한다.
//
//	source_area = area, source_address = start, count = count,
//	target_unit_id = device_id, target_area = target_area(없으면 area),
//	target_address = start + offset (음수 오프셋 허용).
func instantiateRemapTemplate(m map[string]any) map[string]any {
	area, _ := m["area"].(string)
	offset := toIntFromAny(m["offset"])
	start := toUint16FromAny(m["start"])
	count := toUint16FromAny(m["count"])
	deviceID := toByte(m["device_id"])

	targetArea := area
	if s, ok := m["target_area"].(string); ok && s != "" {
		targetArea = s
	}

	targetAddr := uint16(int(start) + offset)

	// int 로 저장한다: 하류 파서(toByte/toUint16FromAny)가 int 를 모두 처리하므로
	// uint8/uint16 저장 시 toByte 가 인식하지 못하는 문제를 피한다.
	return map[string]any{
		"source_area":    area,
		"source_address": int(start),
		"count":          int(count),
		"target_unit_id": int(deviceID),
		"target_area":    targetArea,
		"target_address": int(targetAddr),
	}
}

// ---------------------------------------------------------------------------
// 매칭 / 재작성
// ---------------------------------------------------------------------------

// parseRemapInputEntries 는 입력 payload 의 values[] 를 []map[string]any 로 파싱한다.
// values 키가 없거나 리스트가 아니면 (nil, false) — 비-modbus-read 페이로드로 판정한다.
func parseRemapInputEntries(msg message.Message) ([]map[string]any, bool) {
	p := msg.Payload()
	if p == nil {
		return nil, false
	}
	v, ok := p.Get("values")
	if !ok {
		return nil, false
	}
	switch arr := v.(type) {
	case []map[string]any: // 코드 직접 구성 경로
		return arr, true
	case []any: // JSON 디코드 경로
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, false // 엔트리 형식 오류 → 계약 위반
			}
			out = append(out, m)
		}
		return out, true
	}
	return nil, false
}

// findExactEntry 는 규칙 소스 (area, address, count)와 정확히 일치하는 엔트리를 찾는다.
// 일치하는 엔트리가 없으면 nil(부분 매핑 없이 거부).
func findExactEntry(entries []map[string]any, rule remapRule) map[string]any {
	for _, e := range entries {
		area, _ := e["area"].(string)
		if area != rule.sourceArea {
			continue
		}
		if toUint16FromAny(e["address"]) != rule.sourceAddress {
			continue
		}
		if toUint16FromAny(e["count"]) != rule.count {
			continue
		}
		return e
	}
	return nil
}

// remapEntry 는 매칭된 엔트리를 규칙에 따라 재주소화한 새 엔트리를 만든다.
// 오프셋 변환은 일반형 targetAddr = targetStart + (srcAddr − srcStart) 를 유지한다.
// 값(values/raw)과 data_type 은 그대로 보존한다(변형 금지).
func remapEntry(entry map[string]any, rule remapRule, outIndex int) map[string]any {
	entryAddr := toUint16FromAny(entry["address"])
	targetAddr := uint16(int(rule.targetAddress) + (int(entryAddr) - int(rule.sourceAddress)))

	out := map[string]any{
		"index":   outIndex,
		"area":    rule.targetArea,
		"address": targetAddr,
		"count":   rule.count,
		"unit_id": rule.targetUnitID,
	}
	// 값 보존: values[](+ data_type) 또는 raw 중 존재하는 것을 그대로 전달.
	if v, ok := entry["values"]; ok {
		out["values"] = v
	}
	if v, ok := entry["raw"]; ok {
		out["raw"] = v
	}
	if v, ok := entry["data_type"]; ok {
		out["data_type"] = v
	}
	return out
}

// remapRuleError 는 미매칭/무효 규칙에 대한 규칙별 error 엔트리를 만든다.
func remapRuleError(ruleIndex int, rule remapRule, reason string) map[string]any {
	return map[string]any{
		"rule_index":     ruleIndex,
		"source_area":    rule.sourceArea,
		"source_address": rule.sourceAddress,
		"count":          rule.count,
		"reason":         reason,
	}
}

// toIntFromAny 는 any 값을 int 로 변환한다(음수 오프셋 지원). 변환 불가 시 0.
func toIntFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case uint16:
		return int(n)
	case uint8:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}
