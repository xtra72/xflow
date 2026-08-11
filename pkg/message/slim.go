package message

// slim.go (message-slim-metadata) 는 외부 경계(egress)에서 메시지 메타데이터를
// wire DTO 로 정규화하는 순수 헬퍼를 제공한다.
//
// egress 슬림화 제거(2026-08): 과거에는 agent / device 그룹을 id-only 로 축소했으나,
// 클라이언트/스토리지가 type/name 을 직접 필요로 하는 경우가 많아 슬림화를 중단한다.
// egress 는 이제 full 그룹(agent/device 의 type/id/name)을 그대로 내보낸다.
//
// 함수는 슬림화만 중단하고 남은 책임(내부 제어 마커 `_slimKeep` 를 egress 출력에서
// 제거)은 유지한다. 호출처(WS tap / egress 노드 / influxdb-write)를 바꾸지 않도록
// 함수 이름(SlimGroupsToID)은 유지한다 — 최소 변경 범위.
//
// 설계 원칙(유지):
//   - 순수 함수 — 원본 메시지/메타데이터를 변형하지 않는다. wire DTO
//     (map[string]any, 즉 Metadata().Raw() 의 산출물) 위에서만 동작한다.
//   - 반환 맵은 입력과 독립적인 새 맵이며, 그룹 맵도 새로 할당한다(입력 비변형).

// SlimGroupsToID 는 wire DTO 메타데이터 맵을 egress 출력용으로 정규화한 새 맵을 반환한다.
//
// 동작(슬림화 제거 후):
//   - agent / device 를 포함한 모든 그룹을 full 로 그대로 전달한다(type/id/name 유지).
//   - 내부 제어 마커 MetaKeySlimKeep("_slimKeep") 는 출력에서 항상 제거한다(외부 누출 금지).
//     enrich 노드가 이 마커를 설정하더라도 슬림화가 없으므로 기능적 효과는 없으며,
//     마커가 외부로 새어 나가지 않도록 제거만 수행한다.
//   - 그룹 맵(map[string]string)은 입력 비변형을 위해 새 맵으로 복사한다.
//   - 그 외 키(flat string 등)는 그대로 보존한다.
//
// nil 입력에는 nil 을 반환한다. 멱등이다(마커 없는 출력에 다시 적용해도 동일).
func SlimGroupsToID(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		// 내부 제어 마커는 egress 출력에서 항상 제거한다(누출 금지).
		if k == MetaKeySlimKeep {
			continue
		}

		// 그룹은 full 로 전달하되, 입력 비변형을 위해 새 맵으로 복사한다.
		if group, ok := v.(map[string]string); ok {
			cp := make(map[string]string, len(group))
			for gk, gv := range group {
				cp[gk] = gv
			}
			out[k] = cp
			continue
		}

		// flat string 등 그 외 키는 그대로 보존한다.
		out[k] = v
	}

	return out
}
