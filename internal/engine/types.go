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
	State     string         // lifecycle state (created, running, stopped, error, ...)
	Config    map[string]any // 노드 설정 복사본
	Ports     []NodePortInfo // 포트 목록
	Processed int64          // 처리한 메시지 수
	Errors    int64          // 에러 수
	Extra     map[string]any // 노드 타입별 추가 정보 (aggregate 통계 등)
}

// NodePortInfo 는 노드 포트의 런타임 정보를 나타내는 구조체이다.
type NodePortInfo struct {
	ID         string
	Name       string
	Direction  string // input, output, error
	Connected  bool
	Messages   int64         // 노드가 이 포트로 생산(emit)한 메시지 수
	Delivered  int64         // 연결된 와이어로 실제 전달(delivered)된 메시지 수
	Throughput float64       // 초당 처리량 (msg/sec)
	ActiveFor  time.Duration // 활동 시간
}

// nodeCounter 는 노드별 메시지 처리/에러 카운터이다.
type nodeCounter struct {
	processed    atomic.Int64
	errors       atomic.Int64
	startedAt    time.Time               // 노드 시작 시각
	portCounters map[string]*portCounter // portName -> counter
}

// portCounter 는 포트별 메시지 처리 통계 카운터이다.
// bridgeStatsCollector 패턴을 따르며, 모든 필드는 atomic 타입이다.
type portCounter struct {
	messages  atomic.Int64 // 노드가 이 포트로 생산(emit)한 메시지 수
	delivered atomic.Int64 // 연결된 와이어로 실제 전달(delivered)된 메시지 수
	firstSeen atomic.Int64 // 첫 메시지 시각 (unix 나노초, 처리량 계산용)
	lastSeen  atomic.Int64 // 마지막 메시지 시각 (unix 나노초)

	// warnedUnconnected 는 "연결된 와이어 없음" 경고를 포트당 1회만 로깅하기 위한
	// 디둡(dedupe) 가드이다. 메시지마다 경고가 반복되는 로그 스팸을 방지한다.
	warnedUnconnected atomic.Bool

	// suppressUnconnected 가 true 이면 이 포트의 미연결 출력 경고를 완전히
	// 억제한다. storage-write, storage-write 같은 터미널/라이터 노드는 미연결
	// pass-through "out" 이 의도된 동작이므로 경고가 노이즈가 된다. 노드 config
	// "suppress_unconnected_warning" 으로 옵트인하며 배포 시 설정된다.
	// 불변(immutable) 값으로 배포 시점에 한 번만 설정되므로 atomic 이 아니다.
	suppressUnconnected bool
}

// Record 는 노드가 이 포트로 메시지를 생산(emit)했음을 기록한다.
func (c *portCounter) Record() {
	c.messages.Add(1)
	now := time.Now().UnixNano()
	c.firstSeen.CompareAndSwap(0, now) // 최초 1회만
	c.lastSeen.Store(now)
}

// RecordDelivered 는 이 포트의 메시지가 연결된 와이어로 실제 전달되었음을 기록한다.
// emitted(messages)와 별개로 누적되며, 연결된 와이어가 없으면 호출되지 않으므로
// emitted > 0 이지만 delivered == 0 인 상태로 미연결 포트를 진단할 수 있다.
func (c *portCounter) RecordDelivered() {
	c.delivered.Add(1)
}

// Snapshot 은 현재 통계를 읽기 전용 스냅샷으로 반환한다.
func (c *portCounter) Snapshot() PortStatsSnapshot {
	msgs := c.messages.Load()
	delivered := c.delivered.Load()
	first := c.firstSeen.Load()
	last := c.lastSeen.Load()

	var throughput float64
	var activeFor time.Duration

	if first > 0 {
		activeFor = time.Since(time.Unix(0, first))
		elapsed := last - first
		if elapsed > 0 {
			throughput = float64(msgs) / (float64(elapsed) / float64(time.Second))
		}
	}

	return PortStatsSnapshot{
		Messages:   msgs,
		Delivered:  delivered,
		Throughput: throughput,
		ActiveFor:  activeFor,
	}
}

// PortStatsSnapshot 은 포트 통계의 읽기 전용 스냅샷이다.
type PortStatsSnapshot struct {
	Messages   int64         // 생산(emit)된 총 메시지 수
	Delivered  int64         // 연결된 와이어로 실제 전달된 메시지 수
	Throughput float64       // 메시지/초 (firstSeen~lastSeen 구간)
	ActiveFor  time.Duration // firstSeen ~ now 경과 시간
}

// flowRuntime 은 배포된 Flow의 내부 런타임 상태를 관리하는 구조체이다.
type flowRuntime struct {
	flow              flow.Flow
	nodes             map[string]node.Node // nodeID -> 런타임 노드
	wires             []*RuntimeWire
	cancel            func()
	wg                sync.WaitGroup
	paused            atomic.Bool
	messageCount      atomic.Int64
	errorCount        atomic.Int64
	droppedCount      atomic.Int64
	nodeCounters      map[string]*nodeCounter // nodeID -> 노드별 카운터
	startedAt         time.Time
	disabledNodes     map[string]bool // 비활성화된 노드 ID 집합 (메시지 처리 건너뜀)
	closers           []io.Closer     // 로그 출력 파일 핸들 (StopFlow/UndeployFlow에서 정리)
	autoStartedAgents []agent.Agent   // 플로우 시작 시 자동 시작된 에이전트 (StopFlow에서 자동 정지)
}
