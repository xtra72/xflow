package message

// expand.go (message-slim-metadata) 는 슬림화된 메타데이터의 역-수화(re-hydration)
// 헬퍼를 제공한다. expand=true 로 요청한 클라이언트가 N 회 레지스트리 조회 없이
// agent / device 그룹의 type / name 을 한 번에 받도록 한다.
//
// 레이어링: pkg/message 는 internal/agent · internal/device 를 import 할 수 없다
// (의존 역전 금지). 따라서 조회는 호출 측(api/node 레이어)이 주입하는 함수로 수행한다.
// 이로써 pkg/message 는 레지스트리 구현에 결합되지 않는다.

// GroupExpander 는 id 로 agent / device 의 부가 필드(type/name)를 조회하는 콜백 묶음이다.
//
// 각 콜백은 (fields, ok) 를 반환한다:
//   - fields: 해당 id 에 대한 부가 필드 맵(예: {"type": ..., "name": ...}). id 는
//     포함하지 않아도 되며, 호출 측이 항상 id 를 우선 유지한다.
//   - ok: 조회 성공 여부. false 면 해당 그룹은 슬림 상태(id-only)로 유지한다.
//
// nil 콜백은 "조회 불가"로 간주되어 해당 그룹은 슬림 상태로 유지된다.
type GroupExpander struct {
	// Agent 는 agentID → agent 부가 필드(type/name) 조회.
	Agent func(id string) (fields map[string]string, ok bool)
	// Device 는 deviceID → device 부가 필드(type/name) 조회.
	Device func(id string) (fields map[string]string, ok bool)
}

// ExpandGroupsByID 는 wire DTO 메타데이터 맵의 agent / device 그룹을 레지스트리
// 조회로 역-수화한 새 맵을 반환한다.
//
// 동작:
//   - 입력 raw 는 Metadata().Raw() 형태(string 또는 map[string]string 값).
//   - agent / device 그룹에 id 가 있고 대응 expander 콜백이 성공하면, 조회된
//     부가 필드(type/name 등)를 그룹에 병합한다. id 는 항상 유지/우선한다
//     (콜백이 다른 id 를 돌려줘도 원본 id 를 신뢰).
//   - 콜백이 nil 이거나 ok=false 이면 해당 그룹은 입력 그대로(슬림 또는 full) 둔다.
//   - 그 외 키는 그대로 보존한다.
//
// 원본 비변형: 새 맵과 새 그룹 맵을 할당한다. nil 입력에는 nil 반환.
func ExpandGroupsByID(raw map[string]any, exp GroupExpander) map[string]any {
	if raw == nil {
		return nil
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}

	expandOne := func(key string, lookup func(string) (map[string]string, bool)) {
		if lookup == nil {
			return
		}
		group, ok := out[key].(map[string]string)
		if !ok {
			return
		}
		id := group["id"]
		if id == "" {
			return
		}
		fields, ok := lookup(id)
		if !ok {
			return
		}
		// 새 그룹 맵을 만들어 병합한다(입력 그룹 비변형). id 를 항상 우선 유지.
		merged := make(map[string]string, len(group)+len(fields))
		for fk, fv := range fields {
			if fv == "" {
				continue
			}
			merged[fk] = fv
		}
		for gk, gv := range group {
			merged[gk] = gv
		}
		merged["id"] = id
		out[key] = merged
	}

	expandOne("agent", exp.Agent)
	expandOne("device", exp.Device)

	return out
}
