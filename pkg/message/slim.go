package message

// slim.go (message-slim-metadata) 는 외부 경계(egress)에서 메시지 메타데이터의
// agent / device 그룹을 id-only 로 축소하는 순수 헬퍼를 제공한다.
//
// 배경:
//   - 내부 메시지 흐름(노드/transform/filter/dedup)은 항상 full 그룹
//     (agent:{type,id,name}, device:{type,id,name}) 을 사용한다 — 변경 없음.
//   - 외부 클라이언트로 직렬화되는 경계(에디터 WS 스트림, 스토리지 write)에서는
//     레지스트리에 이미 존재하는 정규 데이터(type/name)를 중복 운반하지 않도록
//     그룹을 id-only 로 슬림화한다. 클라이언트는 필요 시 레지스트리에서 조회한다.
//
// 설계 원칙:
//   - 순수 함수 — 원본 메시지/메타데이터를 변형하지 않는다. wire DTO
//     (map[string]any, 즉 Metadata().Raw() 의 산출물) 위에서만 동작한다.
//   - Message.Clone() 은 새 UUID 를 부여하므로(인터페이스 계약) 슬림화에 사용하지
//     않는다. DTO 레이어에서 슬림화하는 것이 가장 안전하다.
//   - 그룹 형태(metadata.agent.id, metadata.device.id)는 유지한다 — flat 키
//     (agent_id 등)로 평탄화하지 않는다. 기존 consumer 의 metadata.agent.id 읽기를
//     깨지 않기 위함이며, type/name 만 wire 에서 사라진다.

// slimGroupKeys 는 슬림화 대상 그룹 키이다. 각 그룹은 id 필드만 남긴다.
var slimGroupKeys = []string{"agent", "device"}

// SlimGroupsToID 는 wire DTO 메타데이터 맵의 agent / device 그룹을 id-only 로
// 축소한 새 맵을 반환한다.
//
// 동작:
//   - 입력 raw 는 Metadata().Raw() 형태: 값이 string(flat) 또는
//     map[string]string(group) 인 map[string]any.
//   - agent / device 그룹에 대해:
//   - id 필드가 있으면 {id: <id>} 만 남긴 새 그룹으로 치환.
//   - id 필드가 없거나 빈 문자열이면 그룹을 제거(빈 그룹을 만들지 않음 —
//     SetGroup 의 빈 객체 비저장 규칙과 일관).
//   - 그 외 키(flat string, 다른 그룹)는 그대로 보존한다.
//
// 반환 맵은 입력과 독립적인 새 맵이며, 그룹 맵도 새로 할당한다(입력 비변형).
// nil 입력에는 nil 을 반환한다. 멱등성: 이미 슬림화된 맵에 다시 적용해도
// 동일 결과(id 만 유지)를 반환한다.
func SlimGroupsToID(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}

	// 슬림 대상 그룹 키를 빠르게 판별하기 위한 집합.
	slimSet := make(map[string]struct{}, len(slimGroupKeys))
	for _, k := range slimGroupKeys {
		slimSet[k] = struct{}{}
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if _, isSlimTarget := slimSet[k]; !isSlimTarget {
			// 슬림 대상이 아닌 키는 그대로 보존한다.
			out[k] = v
			continue
		}

		group, ok := v.(map[string]string)
		if !ok {
			// 슬림 대상 키이지만 그룹 형태가 아니면(예: 누군가 flat 으로 덮어씀)
			// 변경 없이 보존한다 — forward-compat / 데이터 손실 방지.
			out[k] = v
			continue
		}

		id := group["id"]
		if id == "" {
			// id 가 없거나 비어있으면 의미 있는 슬림 결과를 만들 수 없으므로
			// 그룹을 drop 한다(빈 그룹 생성 금지).
			continue
		}
		// id-only 새 그룹으로 치환(입력 그룹 맵을 변형하지 않음).
		out[k] = map[string]string{"id": id}
	}

	return out
}
