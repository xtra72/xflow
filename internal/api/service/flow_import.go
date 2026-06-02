package service

import (
	"github.com/google/uuid"
)

// 본 파일은 플로우 "import 모드" 의 ID 재생성을 담당한다.
//
// 동기: 동일한 플로우를 여러 번 import 할 때 노드/와이어 ID 가 충돌하면 안 된다.
// import 시 모든 노드 ID 를 새 UUID 로 교체하고, 와이어/엣지의 source/target
// 참조를 oldID→newID 매핑으로 일괄 재작성하며, 와이어/엣지 ID 도 새로 발급한다.
//
// pkg/flow/serialize.go 의 normalizeNodeDefaults/normalizeWireDefaults 는 빈 ID 만
// 채우므로(idempotent fill), import 모드의 "강제 재생성" 요구를 충족하지 못한다.
// 따라서 raw React Flow/XFlow definition(map[string]any) 단계에서 동작하는 전용
// 함수를 둔다. 이 함수는 normalizeReactFlowDefinition 변환 이전에 호출되어야
// 하며, React Flow 포맷(edges, source/target)과 XFlow 포맷(wires,
// source_node_id/target_node_id) 을 모두 처리한다.
//
// 에이전트 참조(agent_ref)는 이름 기반으로 재바인딩되므로 여기서 건드리지 않는다.
//
// SPEC: flow-management requirement 2 (import 시 ID 재생성)

// RegenerateDefinitionIDs 는 입력 definition 의 깊은 복사본을 반환하되,
// 모든 노드 ID 를 새 UUID 로 교체하고 와이어/엣지 참조를 일괄 재작성한다.
//
//   - 입력 맵을 절대 변경하지 않는다(깊은 복사 후 작업).
//   - 노드 id → 새 UUID 매핑을 만들고, edges(source/target) 와
//     wires(source_node_id/target_node_id) 의 노드 참조를 매핑으로 재작성한다.
//   - 매핑되지 않는 참조(존재하지 않는 노드를 가리키는 비정상 입력)는 보존한다.
//   - edges/wires 의 id 는 새 UUID 로 교체한다.
//
// nil 입력에는 nil 을 반환한다.
func RegenerateDefinitionIDs(def map[string]any) map[string]any {
	if def == nil {
		return nil
	}

	// 1) 깊은 복사 — 원본 불변 보장
	out := deepCopyMap(def)

	// 2) 노드 ID 재생성 + oldID→newID 매핑 구축
	idMap := regenerateNodeIDs(out)

	// 3) edges(React Flow) 와 wires(XFlow) 의 노드 참조 및 자체 id 재작성
	rewriteEdgeRefs(out, "edges", "source", "target", idMap)
	rewriteEdgeRefs(out, "wires", "source_node_id", "target_node_id", idMap)

	// 노드 config 내 노드 ID 참조: 본 코드베이스의 NodeDef config 에는 다른 노드
	// ID 를 직접 참조하는 스키마가 없다(연결은 전적으로 wires/edges 로만 표현됨).
	// 따라서 추가 재작성 대상은 없다. (조사 결과: node-config 내 node-ID 참조 없음)

	return out
}

// regenerateNodeIDs 는 def["nodes"] 의 각 노드 id 를 새 UUID 로 교체하고
// oldID→newID 매핑을 반환한다. id 가 없는 노드는 새 id 를 부여한다(매핑 없음).
func regenerateNodeIDs(def map[string]any) map[string]string {
	idMap := make(map[string]string)

	nodes, ok := def["nodes"].([]any)
	if !ok {
		return idMap
	}
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		newID := uuid.New().String()
		if oldID, ok := node["id"].(string); ok && oldID != "" {
			idMap[oldID] = newID
		}
		node["id"] = newID
	}
	return idMap
}

// rewriteEdgeRefs 는 def[edgeKey] 의 각 엣지/와이어에 대해 srcField/dstField
// 노드 참조를 idMap 으로 재작성하고, 엣지/와이어 자체 id 를 새 UUID 로 교체한다.
// 매핑에 없는 참조는 보존한다.
func rewriteEdgeRefs(def map[string]any, edgeKey, srcField, dstField string, idMap map[string]string) {
	edges, ok := def[edgeKey].([]any)
	if !ok {
		return
	}
	for _, raw := range edges {
		edge, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		// source 재작성 (매핑 없으면 보존)
		if src, ok := edge[srcField].(string); ok {
			if mapped, found := idMap[src]; found {
				edge[srcField] = mapped
			}
		}
		// target 재작성 (매핑 없으면 보존)
		if dst, ok := edge[dstField].(string); ok {
			if mapped, found := idMap[dst]; found {
				edge[dstField] = mapped
			}
		}
		// 엣지/와이어 자체 id 는 항상 새로 발급한다(충돌 방지)
		edge["id"] = uuid.New().String()
	}
}

// deepCopyMap 은 map[string]any 의 깊은 복사본을 만든다.
// 중첩 map[string]any, []any 를 재귀적으로 복사하며, 그 외 스칼라/기타 타입은
// 값 그대로 복사한다(불변 취급). JSON 디코딩 결과(map/slice/스칼라)에 대해
// 충분한 깊이의 복사를 보장한다.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue 는 값이 map/slice 이면 재귀 복사한 새 값을, 그 외는 그대로 반환한다.
func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopyValue(item)
		}
		return out
	default:
		return v
	}
}
