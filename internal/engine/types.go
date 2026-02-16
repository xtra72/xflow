package engine

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// FlowStatus 는 배포된 Flow의 런타임 상태 정보를 나타내는 구조체이다.
type FlowStatus struct {
	FlowID       string
	FlowName     string
	State        flow.FlowState
	NodeCount    int
	ActiveNodes  int
	WireCount    int
	MessageCount int64
	ErrorCount   int64
	DroppedCount int64
	StartedAt    time.Time
	Uptime       time.Duration
}

// flowRuntime 은 배포된 Flow의 내부 런타임 상태를 관리하는 구조체이다.
type flowRuntime struct {
	flow         flow.Flow
	nodes        map[string]node.Node // nodeID -> 런타임 노드
	wires        []*RuntimeWire
	cancel       func()
	wg           sync.WaitGroup
	paused       atomic.Bool
	messageCount atomic.Int64
	errorCount   atomic.Int64
	droppedCount atomic.Int64
	startedAt    time.Time
}
