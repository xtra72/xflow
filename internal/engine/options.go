package engine

import (
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/observe"
)

// EngineOption 은 Engine 생성 시 적용할 수 있는 옵션 함수 타입이다.
type EngineOption func(*Engine)

// WithLogger 는 Engine에 ComponentLogger를 설정하는 옵션을 반환한다.
func WithLogger(logger observe.ComponentLogger) EngineOption {
	return func(e *Engine) {
		e.logger = logger
	}
}

// WithMetrics 는 Engine에 MetricsCollector를 설정하는 옵션을 반환한다.
func WithMetrics(metrics observe.MetricsCollector) EngineOption {
	return func(e *Engine) {
		e.metrics = metrics
	}
}

// WithScheduler 는 Engine에 Scheduler를 설정하는 옵션을 반환한다.
func WithScheduler(s Scheduler) EngineOption {
	return func(e *Engine) {
		e.scheduler = s
	}
}

// WithNodeRegistry 는 Engine에 노드 레지스트리를 설정하는 옵션을 반환한다.
func WithNodeRegistry(r *node.Registry) EngineOption {
	return func(e *Engine) {
		e.nodeRegistry = r
	}
}

// WithShutdownTimeout 은 Engine의 종료 대기 시간을 설정하는 옵션을 반환한다.
func WithShutdownTimeout(d time.Duration) EngineOption {
	return func(e *Engine) {
		e.shutdownTimeout = d
	}
}

// WithBackpressurePolicy 는 Engine의 백프레셔 정책을 설정하는 옵션을 반환한다.
func WithBackpressurePolicy(p BackpressurePolicy) EngineOption {
	return func(e *Engine) {
		e.bpPolicy = p
	}
}

// WithObserver 는 Engine에 Observer를 설정하는 옵션을 반환한다.
// Observer가 설정되면 DeployFlow에서 노드별 계층적 로그 레벨이 적용된다.
func WithObserver(obs *observe.Observer) EngineOption {
	return func(e *Engine) {
		e.observer = obs
	}
}

// WithNodeOptions 는 노드 생성 시 적용할 NodeOption을 설정하는 옵션을 반환한다.
// DeployFlow에서 노드를 생성할 때 이 옵션들이 각 노드에 전달된다.
func WithNodeOptions(opts ...node.NodeOption) EngineOption {
	return func(e *Engine) {
		e.nodeOpts = opts
	}
}

// WithDebugSink 는 Engine에 DebugSink를 설정하는 옵션을 반환한다.
// 설정하면 output 노드(editor 출력)가 이 싱크로 메시지를 전송한다.
func WithDebugSink(sink node.DebugSink) EngineOption {
	return func(e *Engine) {
		e.debugSink = sink
	}
}

// SetDebugSink 는 Engine에 DebugSink를 사후 설정한다.
// Engine 생성 후 WebSocket Hub가 초기화된 뒤 호출하여 싱크를 주입할 수 있다.
func (e *Engine) SetDebugSink(sink node.DebugSink) {
	e.mu.Lock()
	e.debugSink = sink
	e.mu.Unlock()
}

// WithOutputObserver 는 Engine에 OutputObserver를 설정하는 옵션을 반환한다.
// 설정하면 노드가 출력 포트로 메시지를 방출할 때마다 OnNodeOutput이 호출된다.
// 와이어 연결 여부와 무관하게 호출되며, 미설정 시 핫 패스 오버헤드는 없다.
func WithOutputObserver(obs OutputObserver) EngineOption {
	return func(e *Engine) {
		e.SetOutputObserver(obs)
	}
}

// WithOnAgentStart 는 엔진이 에이전트를 자동 시작한 후 호출되는 콜백을 등록한다.
// 매니저의 OnStart 훅이 실행되지 않는 autoStartAgents 경로를 보완한다.
func WithOnAgentStart(fn func(agent.Agent)) EngineOption {
	return func(e *Engine) {
		e.onAgentStart = fn
	}
}

// WithAgentManager 는 플로우 배포 시 에이전트 참조 유효성 검증에 사용할 Manager를 설정한다.
// 설정하면 DeployFlow에서 AgentRef를 조기 검증하여 명확한 에러 메시지를 제공한다.
func WithAgentManager(mgr agent.Manager) EngineOption {
	return func(e *Engine) {
		e.agentManager = mgr
	}
}
