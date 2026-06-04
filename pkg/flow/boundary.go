package flow

// ---------------------------------------------------------------------------
// 플로우 포트 경계 와이어 규약 (SPEC-SUBFLOW-001 그룹 B/D/F)
// ---------------------------------------------------------------------------
//
// 플로우 레벨 입출력 포트는 노드가 아니므로(REQ-SUBFLOW-B04), 내부 노드가 플로우
// 자신의 입출력 포트와 연결될 때는 예약된 센티넬 노드 ID 를 와이어 엔드포인트로 사용한다.
//
// 규약:
//   - 플로우 입력 포트에서 출발하는 와이어:
//     SourceNodeID = FlowInputBoundaryID,  SourcePort = <플로우 입력 포트 이름>
//   - 플로우 출력 포트로 도착하는 와이어:
//     TargetNodeID = FlowOutputBoundaryID, TargetPort = <플로우 출력 포트 이름>
//
// 이 센티넬 ID 는 실제 노드가 아니므로, 단독 배포(top-level standalone) 시에는 외부
// 카운터파트가 없어 의미를 가지지 않는다. 서브플로우로 확장(인스턴스화)될 때는 부모 와이어로
// 재배선(rewire)되지만, 그 확장 로직은 본 마일스톤의 범위가 아니다(다음 마일스톤).

const (
	// FlowInputBoundaryID 는 플로우 입력 포트를 가리키는 예약 센티넬 노드 ID 이다.
	// 이 ID 를 SourceNodeID 로 갖는 와이어는 "플로우 입력 포트 → 내부 노드" 연결을 의미한다.
	// (SPEC-SUBFLOW-001 그룹 B/D)
	FlowInputBoundaryID = "__flow_input__"

	// FlowOutputBoundaryID 는 플로우 출력 포트를 가리키는 예약 센티넬 노드 ID 이다.
	// 이 ID 를 TargetNodeID 로 갖는 와이어는 "내부 노드 → 플로우 출력 포트" 연결을 의미한다.
	// (SPEC-SUBFLOW-001 그룹 B/D)
	FlowOutputBoundaryID = "__flow_output__"
)

// IsBoundaryWire 는 와이어가 플로우 포트 경계 센티넬을 엔드포인트로 사용하는지 판별한다.
// 입력 경계(SourceNodeID==FlowInputBoundaryID) 또는 출력 경계(TargetNodeID==FlowOutputBoundaryID)
// 중 하나라도 해당하면 true 를 반환한다.
func IsBoundaryWire(w Wire) bool {
	return w.SourceNodeID == FlowInputBoundaryID || w.TargetNodeID == FlowOutputBoundaryID
}

// StripBoundaryWires 는 플로우의 경계(센티넬) 와이어를 제거한 복사본을 반환한다.
//
// 단독 배포(서브플로우로 확장되지 않는 top-level 배포) 시, 경계 와이어는 외부 카운터파트가
// 없으므로 엔진에 전달하기 전에 제거해야 한다. 경계 와이어가 가리키는 센티넬 노드 ID 는
// 실제 노드가 아니어서, 엔진이 해당 와이어로 메시지를 송신하려 하면 수신자 없는 채널로
// 블로킹되거나 dangling 상태가 된다(REQ-SUBFLOW-F01). 따라서 단독 배포 경로에서만 제거한다.
//
// 서브플로우 확장 경로에서는 경계 와이어를 제거하지 않고 부모 와이어로 재배선(rewire)하므로,
// 본 함수는 단독 배포 전처리에만 사용되도록 격리되어 있다(다음 마일스톤의 확장 로직과 분리).
//
// 원본 플로우는 변경하지 않으며, 노드/포트/메타데이터는 그대로 보존한 새 인스턴스를 반환한다.
func StripBoundaryWires(f Flow) Flow {
	// 동일 패키지의 기본 구현체이면 얕은 구조체 복제로 모든 필드(ID/상태/포트/메타데이터)를
	// 보존하면서 와이어 슬라이스만 필터링한다.
	if df, ok := f.(*defaultFlow); ok {
		return df.cloneWithoutBoundaryWires()
	}

	// 알 수 없는 구현체에 대한 폴백: 인터페이스만으로 경계 와이어를 제거한다.
	// (프로덕션 플로우는 모두 *defaultFlow 이므로 이 경로는 방어적 보장용이다.)
	for _, w := range f.Wires() {
		if IsBoundaryWire(w) {
			_ = f.RemoveWire(w.ID)
		}
	}
	return f
}

// RebuildFlow 는 base 플로우의 정체성(id/name/description/state/config/metadata/
// 입출력 포트)을 그대로 보존하면서, 노드와 와이어 슬라이스만 주어진 값으로 교체한
// 새 Flow 를 반환한다.
//
// 서브플로우 확장(인스턴스화)은 서비스 레이어(internal/api/service)에서 수행되지만,
// 확장 결과를 "부모의 id 를 유지한 단일 평탄화 플로우"로 조립하려면 부모의 ID 와 설정을
// 보존한 채 노드/와이어만 교체할 수 있어야 한다. NewFlow 는 항상 새 UUID 를 발급하고
// FlowFromJSON 은 이름→ID 재해석으로 네임스페이스 ID 를 훼손할 수 있으므로,
// 이 동일 패키지 헬퍼로 *defaultFlow 를 직접 재구성한다.
//
// 입력 슬라이스는 방어적으로 복사하여 호출자와의 별칭(aliasing)을 방지한다.
// base 가 *defaultFlow 가 아니면(테스트 더블 등) 인터페이스 메서드로 정체성을 복원한다.
// (SPEC-SUBFLOW-001 그룹 D — 서브그래프 확장 결과 조립)
func RebuildFlow(base Flow, nodes []NodeDef, wires []Wire) Flow {
	copiedNodes := make([]NodeDef, len(nodes))
	copy(copiedNodes, nodes)
	copiedWires := make([]Wire, len(wires))
	copy(copiedWires, wires)

	if df, ok := base.(*defaultFlow); ok {
		clone := df.cloneWithoutBoundaryWires()
		clone.nodes = copiedNodes
		clone.wires = copiedWires
		return clone
	}

	// 폴백: 인터페이스만으로 정체성을 복원한다(프로덕션 경로는 항상 *defaultFlow).
	rebuilt := &defaultFlow{
		id:          base.ID(),
		name:        base.Name(),
		description: base.Description(),
		state:       base.State(),
		nodes:       copiedNodes,
		wires:       copiedWires,
		config:      base.Config(),
		metadata:    base.Metadata(),
		createdAt:   base.CreatedAt(),
		updatedAt:   base.UpdatedAt(),
		inputs:      base.Inputs(),
		outputs:     base.Outputs(),
	}
	return rebuilt
}

// cloneWithoutBoundaryWires 는 defaultFlow 의 얕은 복제본을 만들되 경계 와이어를 제외한다.
// 슬라이스/맵은 방어적으로 복사하여 원본과의 별칭(aliasing)을 방지한다.
func (f *defaultFlow) cloneWithoutBoundaryWires() *defaultFlow {
	clone := &defaultFlow{
		id:          f.id,
		name:        f.name,
		description: f.description,
		state:       f.state,
		config:      f.config,
		createdAt:   f.createdAt,
		updatedAt:   f.updatedAt,
	}

	// 노드 복사(별칭 방지).
	if f.nodes != nil {
		clone.nodes = make([]NodeDef, len(f.nodes))
		copy(clone.nodes, f.nodes)
	}

	// 경계 와이어를 제외한 와이어만 복사.
	if f.wires != nil {
		filtered := make([]Wire, 0, len(f.wires))
		for _, w := range f.wires {
			if IsBoundaryWire(w) {
				continue
			}
			filtered = append(filtered, w)
		}
		clone.wires = filtered
	}

	// 메타데이터 복사.
	if f.metadata != nil {
		clone.metadata = make(map[string]string, len(f.metadata))
		for k, v := range f.metadata {
			clone.metadata[k] = v
		}
	}

	// 플로우 레벨 입출력 포트 복사.
	if f.inputs != nil {
		clone.inputs = make([]Port, len(f.inputs))
		copy(clone.inputs, f.inputs)
	}
	if f.outputs != nil {
		clone.outputs = make([]Port, len(f.outputs))
		copy(clone.outputs, f.outputs)
	}

	return clone
}
