package node

import "github.com/xtra/xflow/pkg/message"

// egress_metadata.go (message-slim-metadata) 는 노드 레벨 외부 egress(에디터로의
// debug/output 직렬화)에서 메시지 메타데이터의 agent / device 그룹을 슬림화하는
// 헬퍼를 제공한다.
//
// debug/output 노드는 레지스트리에 접근하지 않으므로 기본 슬림(id-only) 만 수행한다.
// expand opt-in 은 레지스트리를 보유한 상위 레이어(api/ws)의 egress 경로에서 처리한다.
// 내부 메시지 흐름(Process 가 하류로 통과시키는 메시지)은 영향받지 않는다 — 본 헬퍼는
// Raw() 복사본(wire DTO) 위에서만 동작하며 원본 메타데이터를 변형하지 않는다.

// slimEgressMetadata 는 외부 egress 표시용 메타데이터 맵을 반환한다.
// agent / device 그룹은 id-only 로 슬림화되고, 그 외 키는 그대로 보존된다.
func slimEgressMetadata(msg message.Message) map[string]any {
	return message.SlimGroupsToID(msg.Metadata().Raw())
}
