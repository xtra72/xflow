package engine

import "github.com/xtra/xflow/pkg/flow"

// Scheduler 는 Flow의 노드 실행 순서를 계획하는 인터페이스이다.
type Scheduler interface {
	Plan(f flow.Flow) (ExecutionPlan, error)
}

// ExecutionPlan 은 노드 실행 계획을 나타내는 구조체이다.
type ExecutionPlan struct {
	// Levels 는 레벨 기반 병렬 실행 그룹이다.
	// 같은 레벨의 노드는 병렬로 실행할 수 있다.
	Levels [][]string

	// Order 는 토폴로지 정렬 순서이다.
	Order []string
}

// DAGScheduler 는 Kahn 알고리즘을 사용하여 DAG의 토폴로지 정렬을 수행하는 스케줄러이다.
type DAGScheduler struct{}

// NewDAGScheduler 는 새로운 DAGScheduler를 생성한다.
func NewDAGScheduler() *DAGScheduler {
	return &DAGScheduler{}
}

// Plan 은 Kahn 알고리즘으로 Flow의 토폴로지 정렬을 수행하고 실행 계획을 반환한다.
// 순환이 감지되면 ErrCycleDetected를 반환한다.
func (s *DAGScheduler) Plan(f flow.Flow) (ExecutionPlan, error) {
	nodes := f.Nodes()
	wires := f.Wires()

	if len(nodes) == 0 {
		return ExecutionPlan{}, nil
	}

	// 인접 리스트와 진입 차수 맵 구축
	adj := make(map[string][]string)    // 노드 ID -> 하류 노드 ID 리스트
	inDegree := make(map[string]int)    // 노드 ID -> 진입 차수

	// 모든 노드의 진입 차수를 0으로 초기화
	for _, n := range nodes {
		inDegree[n.ID] = 0
		adj[n.ID] = nil
	}

	// 와이어 정보로 인접 리스트와 진입 차수 업데이트
	for _, w := range wires {
		adj[w.SourceNodeID] = append(adj[w.SourceNodeID], w.TargetNodeID)
		inDegree[w.TargetNodeID]++
	}

	// 진입 차수가 0인 노드들로 시작 (소스 노드)
	var queue []string
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	var levels [][]string
	var order []string
	processed := 0

	for len(queue) > 0 {
		// 현재 레벨의 모든 노드를 처리
		level := make([]string, len(queue))
		copy(level, queue)
		levels = append(levels, level)
		order = append(order, level...)

		var nextQueue []string
		for _, nodeID := range queue {
			processed++
			for _, downstream := range adj[nodeID] {
				inDegree[downstream]--
				if inDegree[downstream] == 0 {
					nextQueue = append(nextQueue, downstream)
				}
			}
		}
		queue = nextQueue
	}

	// 처리되지 않은 노드가 있으면 순환이 존재한다.
	if processed != len(nodes) {
		return ExecutionPlan{}, ErrCycleDetected
	}

	return ExecutionPlan{
		Levels: levels,
		Order:  order,
	}, nil
}
