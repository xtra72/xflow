package node

import (
	"context"

	"github.com/xtra/xflow/pkg/flow"
)

// AgentReinitializer 는 에이전트 재시작 시 노드 자체를 재초기화하는 인터페이스이다.
//
// Engine.ReinitNodesForAgent 가 본 인터페이스를 구현한 노드를 발견하면,
// 에이전트 lifecycle 이벤트 (Start / Restart) 마다 Reinit 을 호출한다.
//
// 본 인터페이스는 에이전트 백엔드 노드 (Bridge, NASA, LGCP, LGCNP, LGAP, MQTT,
// Modbus, InfluxDB, TSDB, Serial, TCP 등) 전반에서 구현되어, 에이전트가
// 중지 → 시작 또는 Restart 호출로 새 인스턴스로 교체되었을 때, 노드 내부에
// 캐싱된 agent 참조, transport, FrameNotifier 채널 구독, 폴링 / 수신 고루틴
// 등을 일관되게 재구성한다.
//
// 호출 시점에는 노드가 이미 실행 중 (StateRunning) 이라고 가정한다.
// Reinit 구현자는 호출 사이의 race 를 자체적으로 처리할 책임이 있다 (예: 기존
// 고루틴을 안전하게 종료한 뒤 새 stopCh 으로 재시작).
type AgentReinitializer interface {
	// AgentRef 는 이 노드가 의존하는 에이전트의 식별자를 반환한다.
	// 엔진은 본 식별자와 재시작된 에이전트의 AgentID / AgentName 을 비교하여
	// Reinit 대상 여부를 결정한다.
	AgentRef() flow.AgentRef

	// Reinit 은 노드 내부 상태 (transport, agent 참조, FrameNotifier 채널,
	// 폴링 / 수신 루프 등) 를 재초기화한다.
	// 실패 시 에러를 반환하며, 엔진은 이를 로그로만 기록하고 다음 노드로 진행한다.
	Reinit(ctx context.Context) error
}
