package engine

import (
	"time"

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

// WithNodeOptions 는 노드 생성 시 적용할 NodeOption을 설정하는 옵션을 반환한다.
// DeployFlow에서 노드를 생성할 때 이 옵션들이 각 노드에 전달된다.
func WithNodeOptions(opts ...node.NodeOption) EngineOption {
	return func(e *Engine) {
		e.nodeOpts = opts
	}
}
