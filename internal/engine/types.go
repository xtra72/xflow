package engine

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
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

// NodeInstanceInfo 는 배포된 Flow 내 개별 노드 인스턴스의 런타임 정보를 나타내는 구조체이다.
type NodeInstanceInfo struct {
	NodeID    string
	Name      string
	Type      string
	State     string            // lifecycle state (created, running, stopped, error, ...)
	Config    map[string]any    // 노드 설정 복사본
	Ports     []NodePortInfo    // 포트 목록
	Processed int64             // 처리한 메시지 수
	Errors    int64             // 에러 수
}

// NodePortInfo 는 노드 포트의 런타임 정보를 나타내는 구조체이다.
type NodePortInfo struct {
	ID        string
	Name      string
	Direction string // input, output, error
	Connected bool
}

// nodeCounter 는 노드별 메시지 처리/에러 카운터이다.
type nodeCounter struct {
	processed atomic.Int64
	errors    atomic.Int64
}

// flowRuntime 은 배포된 Flow의 내부 런타임 상태를 관리하는 구조체이다.
type flowRuntime struct {
	flow         flow.Flow
	nodes        map[string]node.Node    // nodeID -> 런타임 노드
	wires        []*RuntimeWire
	cancel       func()
	wg           sync.WaitGroup
	paused       atomic.Bool
	messageCount atomic.Int64
	errorCount   atomic.Int64
	droppedCount atomic.Int64
	nodeCounters map[string]*nodeCounter // nodeID -> 노드별 카운터
	startedAt    time.Time
	closers           []io.Closer    // 로그 출력 파일 핸들 (StopFlow/UndeployFlow에서 정리)
	autoStartedAgents []agent.Agent  // 플로우 시작 시 자동 시작된 에이전트 (StopFlow에서 자동 정지)
}
