package node

import (
	"context"
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
//   rules: []map[string]any        // 각 규칙(From → 1..N To, fan-out):
//     {
//       "source_unit_id": number?, // From unit_id (선택). unit-id-optional 매칭(아래).
//       "source_area":    string,  // From 영역 (coils|discrete_inputs|holding_registers|input_registers)
//       "source_address": number,  // From 시작 주소 (엔트리 base)
//       "count":          number,  // 레지스터 수 (엔트리 count 와 정확 일치해야 함)
//       "targets": [               // 1..N 타깃 (fan-out) — 소스 1개 → 타깃마다 출력 엔트리 1개
//         {
//           "target_unit_id": number,  // To device_id/unit_id (To측 할당값)
//           "target_area":    string?, // To 영역 (생략 시 source_area 유지)
//           "target_address": number   // To 시작 주소 (targetStart)
//         }
//       ]
//     }
//     - 하위 호환: "targets" 가 없고 최상위 target_unit_id/target_area/target_address 가
//       있으면 단일 타깃(1-element targets)으로 취급한다(기존 config 계속 동작).
//       "targets" 가 있으면 우선한다.
//   templates: []map[string]any    // (백엔드 런타임 무시) named 멀티-규칙 패턴 저장 필드.
//     - 프론트엔드(RegisterRemapEditor)가 편집 시점에 템플릿 + base 시작주소 + device_id 를
//       구체 rules 로 materialize(전개)한다. 백엔드는 런타임에 templates 를 읽거나 확장하지
//       않는다(rules-only). config 에 templates 키가 있어도 무해하게 저장만 되며 출력에
//       영향을 주지 않는다(존재 시 에러도 아님). — 백엔드가 templates 를 또 전개하면
//       rules 가 이중 적용되므로 backend = rules-only 로 고정한다.
//
// ── unit_id 매칭 규칙 (Change 1, unit-id-optional) ────────────────────────────
//   From 매칭은 항상 (area, address, count) 정확 매칭이다. 추가로:
//   - 규칙에 source_unit_id 가 있고 입력 엔트리에 unit_id 필드가 있으면(상류가 또 다른
//     modbus-remap 인 경우 — 우리 출력 엔트리는 unit_id 를 포함) 엔트리 unit_id 가
//     source_unit_id 와 같아야 매칭된다.
//   - 입력 엔트리에 unit_id 가 없으면(일반 modbus-read) unit_id 를 매칭에서 제외한다
//     (area+address+count 로만 매칭).
//   - 규칙에 source_unit_id 가 없으면 unit_id 로 절대 제약하지 않는다.
//
// ── payload 오버라이드 (modbus_read.go command_set 선례) ──────────────────────
//   payload 에 "rules"(또는 "command_set") 키가 있으면 config 기본 rules 를 대체한다.
//   (templates 는 런타임에서 무시되므로 payload templates 오버라이드도 없다.)
//
// ── 출력 스키마 (downstream modbus-write command_set 소비 호환) ────────────────
//   { success: bool,
//     values: [ { index, area, address, count, unit_id, values[]|raw, data_type? } ],
//     errors?: [ { rule_index, source_area, source_address, count, reason } ],
//     agent_type: string, timestamp: int64(epoch-ms) }
//   - 매칭된 규칙마다 targets 개수만큼 재매핑 엔트리를 emit(fan-out). 미매칭 규칙은
//     거부(부분 매핑 금지)하고 errors 에 규칙별 사유를 기록하며 success=false.
//   - success = inputSuccess && (모든 규칙이 최소 1개 엔트리와 매칭). index 는 순차 출력 위치.
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

	// ErrModbusRemapEmptyRules 는 config/payload 어느 쪽에서도 규칙(rules)이 없을 때 반환된다.
	// (templates 는 런타임에서 무시되므로 "할 일 있음" 판정에 포함되지 않는다.)
	ErrModbusRemapEmptyRules = fmt.Errorf("modbus-remap: %w: no rules configured", ErrInvalidConfig)
)

// ModbusRemapNode 는 modbus-read 출력의 각 엔트리를 remap 규칙으로 재주소화하는 노드이다.
type ModbusRemapNode struct {
	*BaseNode
	rules []map[string]any // config 기본 규칙 목록 (런타임 처리 단위)
	mu    sync.RWMutex     // 설정 보호 뮤텍스
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
// rules(선택)만 파싱한다. templates 키는 프론트엔드 전용 저장 필드로 런타임에서 읽지 않는다.
// rules 가 비어 있으면 Process 시점에 거부된다.
func (n *ModbusRemapNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	// templates 는 의도적으로 소비하지 않는다(프론트엔드가 편집 시점에 rules 로 materialize).
	rules := toOpList(config["rules"])

	n.mu.Lock()
	n.rules = rules
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

	// 1) 유효 규칙 집합 = (config|payload) rules. (templates 는 런타임 무시.)
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
		// 모든 타깃 area 유효성 검증(하나라도 무효면 규칙 전체 거부).
		if bad := invalidTargetArea(rule.targets); bad != "" {
			errs = append(errs, remapRuleError(ri, rule, "invalid target area: "+bad))
			allMatched = false
			continue
		}

		entry := findExactEntry(entries, rule)
		if entry == nil {
			errs = append(errs, remapRuleError(ri, rule, fmt.Sprintf(
				"no entry exactly matches source (area=%s, address=%d, count=%d%s)",
				rule.sourceArea, rule.sourceAddress, rule.count, sourceUnitIDSuffix(rule))))
			allMatched = false
			continue
		}

		// fan-out: 매칭된 소스 1개 → 타깃마다 출력 엔트리 1개.
		for _, t := range rule.targets {
			outValues = append(outValues, remapEntryTo(entry, rule, t, len(outValues)))
		}
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

// effectiveRules 는 config 기본 rules 를 payload 오버라이드와 결합하여 유효 규칙 목록을 만든다.
// payload 의 rules(또는 command_set) 키가 있으면 config 기본을 대체한다.
// templates 는 런타임에서 무시된다(프론트엔드가 편집 시점에 rules 로 materialize).
func (n *ModbusRemapNode) effectiveRules(msg message.Message) ([]map[string]any, error) {
	n.mu.RLock()
	rulesCfg := n.rules
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
	}

	if len(rulesCfg) == 0 {
		return nil, ErrModbusRemapEmptyRules
	}
	return rulesCfg, nil
}

// ---------------------------------------------------------------------------
// 규칙 / 템플릿 파싱
// ---------------------------------------------------------------------------

// remapTarget 는 단일 To 타깃이다(fan-out 시 규칙당 1..N개).
type remapTarget struct {
	unitID  uint8
	area    string // 파싱 시 source_area 로 기본값 채움
	address uint16
}

// remapRule 는 파싱된 단일 remap 규칙이다(1..N 타깃 fan-out).
type remapRule struct {
	sourceArea      string
	sourceAddress   uint16
	count           uint16
	hasSourceUnitID bool
	sourceUnitID    uint8
	targets         []remapTarget
}

// parseRemapRule 은 규칙 맵을 remapRule로 파싱한다.
// targets 배열을 우선하고, 없으면 최상위 target_* 를 단일 타깃으로 취급(하위 호환).
func parseRemapRule(m map[string]any) remapRule {
	r := remapRule{}
	r.sourceArea, _ = m["source_area"].(string)
	r.sourceAddress = toUint16FromAny(m["source_address"])
	r.count = toUint16FromAny(m["count"])
	if r.count == 0 {
		r.count = 1
	}
	if v, ok := m["source_unit_id"]; ok {
		r.sourceUnitID = toByte(v)
		r.hasSourceUnitID = true
	}
	r.targets = parseRemapTargets(m, r.sourceArea)
	return r
}

// parseRemapTargets 는 규칙 맵에서 타깃 목록을 파싱한다.
// "targets" 배열이 있으면 우선하고, 없으면 최상위 target_* 를 단일 타깃으로 취급한다.
func parseRemapTargets(m map[string]any, sourceArea string) []remapTarget {
	if raw, ok := m["targets"]; ok {
		if list := toOpList(raw); len(list) > 0 {
			out := make([]remapTarget, 0, len(list))
			for _, tm := range list {
				out = append(out, parseRemapTarget(tm, sourceArea))
			}
			return out
		}
	}
	// 하위 호환: 최상위 target_unit_id/target_area/target_address → 단일 타깃.
	return []remapTarget{parseRemapTarget(m, sourceArea)}
}

// parseRemapTarget 은 타깃 맵을 remapTarget로 파싱한다.
// target_area 가 없으면 source_area 를 유지한다.
func parseRemapTarget(m map[string]any, sourceArea string) remapTarget {
	t := remapTarget{}
	t.unitID = toByte(m["target_unit_id"])
	if s, ok := m["target_area"].(string); ok && s != "" {
		t.area = s
	} else {
		t.area = sourceArea
	}
	t.address = toUint16FromAny(m["target_address"])
	return t
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
// unit_id 는 unit-id-optional 규칙으로 매칭한다(Change 1):
//   - 규칙에 source_unit_id 가 있고 엔트리에 unit_id 필드가 있으면 값이 같아야 한다.
//   - 엔트리에 unit_id 가 없으면 unit_id 제약을 무시한다(area+address+count 로만 매칭).
//   - 규칙에 source_unit_id 가 없으면 unit_id 로 제약하지 않는다.
//
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
		if rule.hasSourceUnitID {
			if euid, has := e["unit_id"]; has {
				// 입력 엔트리에 unit_id 가 있으면 정확 일치 요구(체인 remap).
				if toByte(euid) != rule.sourceUnitID {
					continue
				}
			}
			// 입력 엔트리에 unit_id 가 없으면 unit_id 제약 무시.
		}
		return e
	}
	return nil
}

// invalidTargetArea 는 타깃 중 유효하지 않은 area 를 하나 찾아 반환한다(없으면 "").
func invalidTargetArea(targets []remapTarget) string {
	for _, t := range targets {
		if !validRegisterAreas[t.area] {
			return t.area
		}
	}
	return ""
}

// sourceUnitIDSuffix 는 error reason 에 붙일 unit_id 접미사를 만든다.
func sourceUnitIDSuffix(rule remapRule) string {
	if rule.hasSourceUnitID {
		return fmt.Sprintf(", unit_id=%d", rule.sourceUnitID)
	}
	return ""
}

// remapEntryTo 는 매칭된 엔트리를 규칙+단일 타깃에 따라 재주소화한 새 엔트리를 만든다.
// 오프셋 변환은 일반형 targetAddr = targetStart + (srcAddr − srcStart) 를 유지한다.
// 값(values/raw)과 data_type 은 그대로 보존한다(변형 금지).
func remapEntryTo(entry map[string]any, rule remapRule, target remapTarget, outIndex int) map[string]any {
	entryAddr := toUint16FromAny(entry["address"])
	targetAddr := uint16(int(target.address) + (int(entryAddr) - int(rule.sourceAddress)))

	out := map[string]any{
		"index":   outIndex,
		"area":    target.area,
		"address": targetAddr,
		"count":   rule.count,
		"unit_id": target.unitID,
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
