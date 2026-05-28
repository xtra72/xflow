package flow

import "fmt"

// ---------------------------------------------------------------------------
// ValidationSeverity
// ---------------------------------------------------------------------------

// ValidationSeverity 는 유효성 검사 결과의 심각도를 나타내는 문자열 타입이다.
type ValidationSeverity string

const (
	// SeverityError 는 Flow 실행을 불가능하게 만드는 심각한 에러를 나타낸다.
	SeverityError ValidationSeverity = "error"

	// SeverityWarning 은 실행은 가능하나 잠재적 문제가 있음을 나타낸다.
	SeverityWarning ValidationSeverity = "warning"
)

// ---------------------------------------------------------------------------
// ValidationError
// ---------------------------------------------------------------------------

// ValidationError 는 Flow 유효성 검사에서 발견된 개별 문제를 나타내는 구조체이다.
type ValidationError struct {
	Code     string             // 에러 코드 (예: "WIRE_ORPHAN_SOURCE")
	Severity ValidationSeverity // 심각도
	Message  string             // 사람이 읽을 수 있는 에러 메시지
	Path     string             // 에러 발생 위치 (예: "wires[0].source_node_id")
}

// Error 는 ValidationError를 문자열로 표현하여 error 인터페이스를 구현한다.
func (e ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s (at %s)", e.Severity, e.Code, e.Message, e.Path)
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

// Validate 는 Flow의 모든 유효성 규칙을 검사하고 발견된 모든 에러를 반환한다.
// 유효한 Flow이면 빈 슬라이스를 반환한다.
//
// 검사 규칙:
//   - NODE_DUPLICATE_ID: 동일 ID를 가진 노드가 2개 이상 존재
//   - NODE_DUPLICATE_NAME: 동일 Name을 가진 노드가 2개 이상 존재
//   - NODE_BRIDGE_NO_AGENT: "bridge" 타입 노드에 AgentRef가 nil
//   - NODE_DISCONNECTED: 와이어에 연결되지 않은 노드 (Warning)
//   - WIRE_ORPHAN_SOURCE: Wire의 SourceNodeID에 해당하는 노드가 없음
//   - WIRE_ORPHAN_TARGET: Wire의 TargetNodeID에 해당하는 노드가 없음
//   - WIRE_INVALID_SOURCE_PORT: Wire의 SourcePort가 소스 노드의 Outputs/Errors에 없음
//   - WIRE_INVALID_TARGET_PORT: Wire의 TargetPort가 타겟 노드의 Inputs에 없음
//   - WIRE_DUPLICATE: 동일한 (SourceNodeID, SourcePort, TargetNodeID, TargetPort) 조합이 2개 이상
//   - WIRE_SELF_REFERENCE: Wire의 SourceNodeID와 TargetNodeID가 동일 (Warning)
//   - WIRE_BUFFER_INVALID_SIZE: WireBuffer 모드에서 BufferSize가 0 이하
func Validate(f Flow) []ValidationError {
	var errs []ValidationError

	nodes := f.Nodes()
	wires := f.Wires()

	// 노드 ID → 인덱스 맵 구축
	nodeByID := make(map[string]NodeDef, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}

	// -------------------------------------------------------------------
	// 노드 검증
	// -------------------------------------------------------------------
	errs = append(errs, validateDuplicateNodeIDs(nodes)...)
	errs = append(errs, validateDuplicateNodeNames(nodes)...)
	errs = append(errs, validateAgentRefNodes(nodes)...)

	// -------------------------------------------------------------------
	// 와이어 검증
	// -------------------------------------------------------------------
	errs = append(errs, validateWires(wires, nodeByID)...)

	// -------------------------------------------------------------------
	// 연결되지 않은 노드 검증 (Warning)
	// -------------------------------------------------------------------
	errs = append(errs, validateDisconnectedNodes(nodes, wires)...)

	return errs
}

// ---------------------------------------------------------------------------
// 노드 검증 함수
// ---------------------------------------------------------------------------

// validateDuplicateNodeIDs 는 동일 ID를 가진 노드가 있는지 검사한다.
func validateDuplicateNodeIDs(nodes []NodeDef) []ValidationError {
	var errs []ValidationError
	seen := make(map[string]int) // ID → 처음 발견된 인덱스

	for i, n := range nodes {
		if firstIdx, exists := seen[n.ID]; exists {
			errs = append(errs, ValidationError{
				Code:     "NODE_DUPLICATE_ID",
				Severity: SeverityError,
				Message:  fmt.Sprintf("노드 ID %q가 nodes[%d]과(와) 중복됩니다", n.ID, firstIdx),
				Path:     fmt.Sprintf("nodes[%d].id", i),
			})
		} else {
			seen[n.ID] = i
		}
	}

	return errs
}

// validateDuplicateNodeNames 는 동일 Name을 가진 노드가 있는지 검사한다.
// 와이어는 ID로 연결되므로 이름 중복은 경고(warning)로만 처리한다.
func validateDuplicateNodeNames(nodes []NodeDef) []ValidationError {
	var errs []ValidationError
	seen := make(map[string]int) // Name → 처음 발견된 인덱스

	for i, n := range nodes {
		if firstIdx, exists := seen[n.Name]; exists {
			errs = append(errs, ValidationError{
				Code:     "NODE_DUPLICATE_NAME",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("노드 이름 %q이(가) nodes[%d]과(와) 중복됩니다", n.Name, firstIdx),
				Path:     fmt.Sprintf("nodes[%d].name", i),
			})
		} else {
			seen[n.Name] = i
		}
	}

	return errs
}

// agentRefRequiredTypes 는 AgentRef 가 반드시 설정되어야 하는 노드 타입 집합이다.
// 해당 타입의 노드가 플로우에 포함될 때, AgentRef 미설정은 Init 단계에서 실패한다.
// 이 집합은 내부 구현체(internal/node/*.go)에서 Init 시 AgentRef nil 을 에러로
// 처리하는 노드 타입과 동기화되어 있어야 한다.
var agentRefRequiredTypes = map[string]struct{}{
	"bridge":                  {},
	"store-read":              {},
	"store-write":             {},
	"influxdb-read":           {},
	"influxdb-write":          {},
	"influxdb-query":          {},
	"tsdb-write":              {},
	"tsdb-query":              {},
	"mqtt-subscriber":         {},
	"mqtt-publisher":          {},
	"samsung_hvacr01":         {},
	"samsung_hvacr01_status":  {},
	"samsung_hvacr01_control": {},
	"lgap":                    {},
	"lgap-status":             {},
	"lgap-control":            {},
	"lgcp":                    {},
	"lgcp-status":             {},
	"lgcp-control":            {},
	"lg_hvacr01":              {},
	"lg_hvacr01_status":       {},
	"lg_hvacr01_control":      {},
	"modbus":                  {},
	"modbus-poller":           {},
	"modbus-writer":           {},
	"serial-in":               {},
	"serial-out":              {},
	"tcp-in":                  {},
	"tcp-out":                 {},
}

// validateAgentRefNodes 는 AgentRef 가 필수인 노드 타입에 대해 설정 여부를 검사한다.
// 기존의 "bridge" 전용 검사를 전체 스토리지/IO 노드로 확장한 것이다.
func validateAgentRefNodes(nodes []NodeDef) []ValidationError {
	var errs []ValidationError

	for i, n := range nodes {
		if _, required := agentRefRequiredTypes[n.Type]; !required {
			continue
		}
		if n.AgentRef != nil && (n.AgentRef.AgentID != "" || n.AgentRef.AgentName != "") {
			continue
		}

		// bridge 는 하위 호환을 위해 기존 코드를 유지한다.
		code := "NODE_MISSING_AGENT_REF"
		if n.Type == "bridge" {
			code = "NODE_BRIDGE_NO_AGENT"
		}

		errs = append(errs, ValidationError{
			Code:     code,
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s 노드 %q에 agent_ref 가 설정되지 않았습니다", n.Type, n.Name),
			Path:     fmt.Sprintf("nodes[%d].agent_ref", i),
		})
	}

	return errs
}

// ---------------------------------------------------------------------------
// 와이어 검증 함수
// ---------------------------------------------------------------------------

// wireKey 는 와이어 중복 검사에 사용되는 복합 키이다.
type wireKey struct {
	SourceNodeID string
	SourcePort   string
	TargetNodeID string
	TargetPort   string
}

// validateWires 는 모든 와이어 관련 유효성 규칙을 검사한다.
func validateWires(wires []Wire, nodeByID map[string]NodeDef) []ValidationError {
	var errs []ValidationError
	seen := make(map[wireKey]int) // wireKey → 처음 발견된 인덱스

	for i, w := range wires {
		// WIRE_ORPHAN_SOURCE: 소스 노드가 존재하지 않음
		srcNode, srcFound := nodeByID[w.SourceNodeID]
		if !srcFound {
			errs = append(errs, ValidationError{
				Code:     "WIRE_ORPHAN_SOURCE",
				Severity: SeverityError,
				Message:  fmt.Sprintf("와이어의 소스 노드 %q를 찾을 수 없습니다", w.SourceNodeID),
				Path:     fmt.Sprintf("wires[%d].source_node_id", i),
			})
		}

		// WIRE_ORPHAN_TARGET: 타겟 노드가 존재하지 않음
		tgtNode, tgtFound := nodeByID[w.TargetNodeID]
		if !tgtFound {
			errs = append(errs, ValidationError{
				Code:     "WIRE_ORPHAN_TARGET",
				Severity: SeverityError,
				Message:  fmt.Sprintf("와이어의 타겟 노드 %q를 찾을 수 없습니다", w.TargetNodeID),
				Path:     fmt.Sprintf("wires[%d].target_node_id", i),
			})
		}

		// WIRE_INVALID_SOURCE_PORT: 소스 노드가 존재할 때만 포트 검증
		if srcFound && !hasOutputPortByName(srcNode, w.SourcePort) {
			errs = append(errs, ValidationError{
				Code:     "WIRE_INVALID_SOURCE_PORT",
				Severity: SeverityError,
				Message:  fmt.Sprintf("소스 노드 %q에 출력 포트 %q가 없습니다", srcNode.Name, w.SourcePort),
				Path:     fmt.Sprintf("wires[%d].source_port", i),
			})
		}

		// WIRE_INVALID_TARGET_PORT: 타겟 노드가 존재할 때만 포트 검증
		if tgtFound && !hasInputPortByName(tgtNode, w.TargetPort) {
			errs = append(errs, ValidationError{
				Code:     "WIRE_INVALID_TARGET_PORT",
				Severity: SeverityError,
				Message:  fmt.Sprintf("타겟 노드 %q에 입력 포트 %q가 없습니다", tgtNode.Name, w.TargetPort),
				Path:     fmt.Sprintf("wires[%d].target_port", i),
			})
		}

		// WIRE_DUPLICATE: 동일한 소스-타겟 조합의 와이어 중복
		key := wireKey{
			SourceNodeID: w.SourceNodeID,
			SourcePort:   w.SourcePort,
			TargetNodeID: w.TargetNodeID,
			TargetPort:   w.TargetPort,
		}
		if firstIdx, exists := seen[key]; exists {
			errs = append(errs, ValidationError{
				Code:     "WIRE_DUPLICATE",
				Severity: SeverityError,
				Message:  fmt.Sprintf("와이어가 wires[%d]과(와) 중복됩니다", firstIdx),
				Path:     fmt.Sprintf("wires[%d]", i),
			})
		} else {
			seen[key] = i
		}

		// WIRE_SELF_REFERENCE: 소스와 타겟이 같은 노드
		if w.SourceNodeID == w.TargetNodeID {
			errs = append(errs, ValidationError{
				Code:     "WIRE_SELF_REFERENCE",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("와이어가 같은 노드 %q를 참조합니다", w.SourceNodeID),
				Path:     fmt.Sprintf("wires[%d]", i),
			})
		}

		// WIRE_BUFFER_INVALID_SIZE: 버퍼 모드에서 크기가 0 이하
		if w.Mode == WireBuffer && w.BufferSize <= 0 {
			errs = append(errs, ValidationError{
				Code:     "WIRE_BUFFER_INVALID_SIZE",
				Severity: SeverityError,
				Message:  fmt.Sprintf("버퍼 모드 와이어의 BufferSize가 %d입니다 (1 이상이어야 합니다)", w.BufferSize),
				Path:     fmt.Sprintf("wires[%d]", i),
			})
		}
	}

	return errs
}

// ---------------------------------------------------------------------------
// 연결 검증 함수
// ---------------------------------------------------------------------------

// validateDisconnectedNodes 는 와이어에 연결되지 않은 노드를 검사한다.
func validateDisconnectedNodes(nodes []NodeDef, wires []Wire) []ValidationError {
	var errs []ValidationError

	// 와이어에 참조된 모든 노드 ID를 수집
	connected := make(map[string]bool, len(wires)*2)
	for _, w := range wires {
		connected[w.SourceNodeID] = true
		connected[w.TargetNodeID] = true
	}

	for i, n := range nodes {
		if !connected[n.ID] {
			errs = append(errs, ValidationError{
				Code:     "NODE_DISCONNECTED",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("노드 %q가 어떤 와이어에도 연결되어 있지 않습니다", n.Name),
				Path:     fmt.Sprintf("nodes[%d].id", i),
			})
		}
	}

	return errs
}

// ---------------------------------------------------------------------------
// 비공개 헬퍼 함수
// ---------------------------------------------------------------------------

// hasOutputPortByName 은 노드의 Outputs 또는 Errors에서 지정된 이름의 포트가 존재하는지 확인한다.
func hasOutputPortByName(node NodeDef, portName string) bool {
	for _, p := range node.Outputs {
		if p.Name == portName {
			return true
		}
	}
	for _, p := range node.Errors {
		if p.Name == portName {
			return true
		}
	}
	return false
}

// hasInputPortByName 은 노드의 Inputs에서 지정된 이름의 포트가 존재하는지 확인한다.
func hasInputPortByName(node NodeDef, portName string) bool {
	for _, p := range node.Inputs {
		if p.Name == portName {
			return true
		}
	}
	return false
}
