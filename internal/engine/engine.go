package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// Engine 은 Flow의 배포, 실행, 관리를 담당하는 핵심 엔진이다.
type Engine struct {
	*lifecycle.BaseLifecycle

	mu              sync.RWMutex
	flows           map[string]*flowRuntime
	scheduler       Scheduler
	logger          observe.ComponentLogger
	metrics         observe.MetricsCollector
	nodeRegistry    *node.Registry
	shutdownTimeout time.Duration
	bpPolicy        BackpressurePolicy
	nodeOpts        []node.NodeOption
	config          map[string]any
}

// NewEngine 은 지정된 옵션으로 새로운 Engine을 생성한다.
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		BaseLifecycle:   lifecycle.NewBaseLifecycle(lifecycle.WithName("engine")),
		flows:           make(map[string]*flowRuntime),
		shutdownTimeout: 30 * time.Second,
		bpPolicy:        DefaultBackpressurePolicy(),
		config:          make(map[string]any),
	}

	for _, opt := range opts {
		opt(e)
	}

	// 기본 스케줄러 설정
	if e.scheduler == nil {
		e.scheduler = NewDAGScheduler()
	}

	// 기본 노드 레지스트리 설정
	if e.nodeRegistry == nil {
		e.nodeRegistry = node.NewRegistry()
	}

	return e
}

// DeployFlow 는 Flow를 검증하고 Engine에 배포한다.
func (e *Engine) DeployFlow(ctx context.Context, f flow.Flow) error {
	// 1. Flow 유효성 검사
	validationErrors := flow.Validate(f)
	for _, ve := range validationErrors {
		if ve.Severity == flow.SeverityError {
			return fmt.Errorf("%w: %s", ErrFlowValidationFailed, ve.Message)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 2. 중복 Flow ID 검사
	if _, exists := e.flows[f.ID()]; exists {
		return ErrFlowAlreadyDeployed
	}

	// 3. 각 NodeDef에 대해 런타임 노드 생성
	runtimeNodes := make(map[string]node.Node)
	for _, nd := range f.Nodes() {
		n, err := e.nodeRegistry.Create(nd, e.nodeOpts...)
		if err != nil {
			return fmt.Errorf("engine: failed to create node %q: %w", nd.Name, err)
		}
		runtimeNodes[nd.ID] = n
	}

	// 4. RuntimeWire 생성
	runtimeWires, err := CreateRuntimeWires(f.Wires())
	if err != nil {
		return fmt.Errorf("engine: failed to create runtime wires: %w", err)
	}

	// 5. flowRuntime 등록
	rt := &flowRuntime{
		flow:  f,
		nodes: runtimeNodes,
		wires: runtimeWires,
	}

	// Flow 상태를 FlowLoaded로 설정
	if err := f.SetState(flow.FlowLoaded); err != nil {
		return fmt.Errorf("engine: failed to set flow state to loaded: %w", err)
	}

	e.flows[f.ID()] = rt

	if e.logger != nil {
		e.logger.Info("flow deployed", "flowID", f.ID(), "flowName", f.Name())
	}

	return nil
}

// StartFlow 는 배포된 Flow를 시작한다.
func (e *Engine) StartFlow(ctx context.Context, flowID string) error {
	e.mu.Lock()
	rt, exists := e.flows[flowID]
	if !exists {
		e.mu.Unlock()
		return ErrFlowNotFound
	}

	// FlowLoaded 상태여야 시작 가능
	if rt.flow.State() != flow.FlowLoaded {
		e.mu.Unlock()
		return ErrFlowNotLoaded
	}

	// FlowInitializing으로 전이
	if err := rt.flow.SetState(flow.FlowInitializing); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("engine: failed to transition to initializing: %w", err)
	}
	e.mu.Unlock()

	// 스케줄러로 실행 순서 결정
	plan, err := e.scheduler.Plan(rt.flow)
	if err != nil {
		// Initializing → Stopped → Loaded 순으로 롤백하여 재시작이 가능하게 한다.
		e.mu.Lock()
		_ = rt.flow.SetState(flow.FlowStopped)
		_ = rt.flow.SetState(flow.FlowLoaded)
		e.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrNodeStartFailed, err)
	}

	// 실행 순서대로 노드 초기화
	var initializedNodes []string
	for _, nodeID := range plan.Order {
		n := rt.nodes[nodeID]
		if n == nil {
			continue
		}
		if err := n.Init(ctx); err != nil {
			// 이미 초기화된 노드들을 역순으로 정리한다.
			for i := len(initializedNodes) - 1; i >= 0; i-- {
				if nd := rt.nodes[initializedNodes[i]]; nd != nil {
					_ = nd.Shutdown(ctx)
				}
			}
			// Initializing → Stopped → Loaded 순으로 롤백하여 재시작이 가능하게 한다.
			e.mu.Lock()
			_ = rt.flow.SetState(flow.FlowStopped)
			_ = rt.flow.SetState(flow.FlowLoaded)
			e.mu.Unlock()
			return fmt.Errorf("%w: node %q init failed: %v", ErrNodeStartFailed, n.Name(), err)
		}
		initializedNodes = append(initializedNodes, nodeID)
	}

	// 노드별 goroutine 시작
	nodeCtx, cancel := context.WithCancel(ctx)
	rt.cancel = cancel
	rt.startedAt = time.Now()

	// 각 노드에 대해 입력/출력 와이어를 매핑한다.
	inputWires := e.buildInputWireMap(rt)
	outputWires := e.buildOutputWireMap(rt)

	for _, nodeID := range plan.Order {
		n := rt.nodes[nodeID]
		if n == nil {
			continue
		}
		nInputWires := inputWires[nodeID]
		nOutputWires := outputWires[nodeID]

		rt.wg.Add(1)
		go e.runNode(nodeCtx, rt, n, nInputWires, nOutputWires)
	}

	// FlowRunning으로 전이
	e.mu.Lock()
	if err := rt.flow.SetState(flow.FlowRunning); err != nil {
		e.mu.Unlock()
		cancel()
		return fmt.Errorf("engine: failed to transition to running: %w", err)
	}
	e.mu.Unlock()

	if e.logger != nil {
		e.logger.Info("flow started", "flowID", flowID)
	}

	return nil
}

// StopFlow 는 실행 중이거나 일시정지된 Flow를 중지한다.
func (e *Engine) StopFlow(ctx context.Context, flowID string) error {
	e.mu.Lock()
	rt, exists := e.flows[flowID]
	if !exists {
		e.mu.Unlock()
		return ErrFlowNotFound
	}

	state := rt.flow.State()
	if state != flow.FlowRunning && state != flow.FlowPaused {
		e.mu.Unlock()
		return ErrFlowNotRunning
	}

	// FlowStopping으로 전이
	if err := rt.flow.SetState(flow.FlowStopping); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("engine: failed to transition to stopping: %w", err)
	}
	e.mu.Unlock()

	// paused 플래그 해제 (일시정지된 goroutine이 context 취소를 감지하도록)
	rt.paused.Store(false)

	// goroutine 중지
	if rt.cancel != nil {
		rt.cancel()
	}

	// 타임아웃 내 goroutine 종료 대기
	done := make(chan struct{})
	go func() {
		rt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 정상 종료
	case <-time.After(e.shutdownTimeout):
		if e.logger != nil {
			e.logger.Warn("shutdown timeout exceeded", "flowID", flowID)
		}
	}

	// 모든 와이어 채널 닫기
	for _, w := range rt.wires {
		w.Close()
	}

	// 각 노드 Shutdown 호출
	for _, n := range rt.nodes {
		if err := n.Shutdown(ctx); err != nil {
			if e.logger != nil {
				e.logger.Error("node shutdown error", "nodeID", n.ID(), "error", err)
			}
		}
	}

	// FlowStopped로 전이
	e.mu.Lock()
	_ = rt.flow.SetState(flow.FlowStopped)
	e.mu.Unlock()

	if e.logger != nil {
		e.logger.Info("flow stopped", "flowID", flowID)
	}

	return nil
}

// PauseFlow 는 실행 중인 Flow를 일시정지한다.
func (e *Engine) PauseFlow(ctx context.Context, flowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return ErrFlowNotFound
	}

	if rt.flow.State() != flow.FlowRunning {
		return ErrFlowNotRunning
	}

	rt.paused.Store(true)

	if err := rt.flow.SetState(flow.FlowPaused); err != nil {
		return fmt.Errorf("engine: failed to transition to paused: %w", err)
	}

	if e.logger != nil {
		e.logger.Info("flow paused", "flowID", flowID)
	}

	return nil
}

// ResumeFlow 는 일시정지된 Flow를 재개한다.
func (e *Engine) ResumeFlow(ctx context.Context, flowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return ErrFlowNotFound
	}

	if rt.flow.State() != flow.FlowPaused {
		return ErrFlowNotPaused
	}

	rt.paused.Store(false)

	if err := rt.flow.SetState(flow.FlowRunning); err != nil {
		return fmt.Errorf("engine: failed to transition to running: %w", err)
	}

	if e.logger != nil {
		e.logger.Info("flow resumed", "flowID", flowID)
	}

	return nil
}

// UndeployFlow 는 중지된 Flow를 배포 해제한다.
func (e *Engine) UndeployFlow(ctx context.Context, flowID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return ErrFlowNotFound
	}

	if rt.flow.State() != flow.FlowStopped {
		return ErrFlowNotStopped
	}

	delete(e.flows, flowID)

	if e.logger != nil {
		e.logger.Info("flow undeployed", "flowID", flowID)
	}

	return nil
}

// GetFlowStatus 는 배포된 Flow의 현재 상태를 반환한다.
func (e *Engine) GetFlowStatus(flowID string) (FlowStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return FlowStatus{}, ErrFlowNotFound
	}

	status := FlowStatus{
		FlowID:       rt.flow.ID(),
		FlowName:     rt.flow.Name(),
		State:        rt.flow.State(),
		NodeCount:    len(rt.nodes),
		WireCount:    len(rt.wires),
		MessageCount: rt.messageCount.Load(),
		ErrorCount:   rt.errorCount.Load(),
		DroppedCount: rt.droppedCount.Load(),
		StartedAt:    rt.startedAt,
	}

	if !rt.startedAt.IsZero() && rt.flow.State() == flow.FlowRunning {
		status.Uptime = time.Since(rt.startedAt)
	}

	// ActiveNodes 계산: Running 상태에서만 활성
	if rt.flow.State() == flow.FlowRunning {
		status.ActiveNodes = len(rt.nodes)
	}

	return status, nil
}

// ListFlows 는 배포된 모든 Flow의 상태 목록을 반환한다.
func (e *Engine) ListFlows() []FlowStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]FlowStatus, 0, len(e.flows))
	for flowID := range e.flows {
		status, err := e.getFlowStatusLocked(flowID)
		if err == nil {
			result = append(result, status)
		}
	}

	return result
}

// Configure 는 Engine의 설정을 변경한다.
func (e *Engine) Configure(ctx context.Context, cfg map[string]any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = make(map[string]any, len(cfg))
	for k, v := range cfg {
		e.config[k] = v
	}

	return nil
}

// GetConfig 는 Engine의 현재 설정을 반환한다.
func (e *Engine) GetConfig() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cp := make(map[string]any, len(e.config))
	for k, v := range e.config {
		cp[k] = v
	}
	return cp
}

// HealthCheck 는 Engine의 건강 상태를 확인한다.
func (e *Engine) HealthCheck(ctx context.Context) lifecycle.HealthStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	details := map[string]any{
		"deployed_flows": len(e.flows),
	}

	healthy := true
	msg := "engine is healthy"

	// 에러 상태의 Flow가 있으면 unhealthy
	for _, rt := range e.flows {
		if rt.flow.State() == flow.FlowError {
			healthy = false
			msg = "one or more flows in error state"
			break
		}
	}

	return lifecycle.HealthStatus{
		Healthy:     healthy,
		Message:     msg,
		LastChecked: time.Now(),
		Details:     details,
	}
}

// ---------------------------------------------------------------------------
// 비공개 메서드
// ---------------------------------------------------------------------------

// getFlowStatusLocked 는 잠금이 획득된 상태에서 Flow 상태를 조회한다.
func (e *Engine) getFlowStatusLocked(flowID string) (FlowStatus, error) {
	rt, exists := e.flows[flowID]
	if !exists {
		return FlowStatus{}, ErrFlowNotFound
	}

	return FlowStatus{
		FlowID:       rt.flow.ID(),
		FlowName:     rt.flow.Name(),
		State:        rt.flow.State(),
		NodeCount:    len(rt.nodes),
		WireCount:    len(rt.wires),
		MessageCount: rt.messageCount.Load(),
		ErrorCount:   rt.errorCount.Load(),
		DroppedCount: rt.droppedCount.Load(),
		StartedAt:    rt.startedAt,
	}, nil
}

// buildInputWireMap 은 노드 ID를 키로 하는 입력 와이어 맵을 생성한다.
func (e *Engine) buildInputWireMap(rt *flowRuntime) map[string][]*RuntimeWire {
	result := make(map[string][]*RuntimeWire)
	for _, w := range rt.wires {
		result[w.TargetNodeID] = append(result[w.TargetNodeID], w)
	}
	return result
}

// buildOutputWireMap 은 노드 ID를 키로 하는 출력 와이어 맵을 생성한다.
func (e *Engine) buildOutputWireMap(rt *flowRuntime) map[string][]*RuntimeWire {
	result := make(map[string][]*RuntimeWire)
	for _, w := range rt.wires {
		result[w.SourceNodeID] = append(result[w.SourceNodeID], w)
	}
	return result
}

// runNode 는 단일 노드의 메시지 처리 goroutine이다.
// 입력 와이어에서 메시지를 읽어 노드의 Process를 호출하고 출력 와이어로 전송한다.
func (e *Engine) runNode(
	ctx context.Context,
	rt *flowRuntime,
	n node.Node,
	inputWires []*RuntimeWire,
	outputWires []*RuntimeWire,
) {
	defer rt.wg.Done()

	// 입력 와이어가 없는 소스 노드는 context 취소만 대기한다.
	if len(inputWires) == 0 {
		<-ctx.Done()
		return
	}

	// 여러 입력 와이어를 하나의 merged 채널로 합친다 (fan-in).
	merged := e.mergeInputWires(ctx, inputWires)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-merged:
			if !ok {
				return
			}

			// 일시정지 대기
			for rt.paused.Load() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}

			// 노드 처리
			results, err := n.Process(ctx, msg)
			rt.messageCount.Add(1)

			if err != nil {
				rt.errorCount.Add(1)
				if e.logger != nil {
					e.logger.Error("node process error",
						"nodeID", n.ID(),
						"error", err,
					)
				}
				continue
			}

			// 출력 와이어로 결과 전송 (fan-out)
			for _, result := range results {
				for i, w := range outputWires {
					var msgToSend message.Message
					if i == len(outputWires)-1 {
						// 마지막 와이어에는 원본 메시지를 전송
						msgToSend = result
					} else {
						// 나머지 와이어에는 복사본을 전송
						msgToSend = result.Clone()
					}

					if err := w.Send(ctx, msgToSend); err != nil {
						if e.logger != nil {
							e.logger.Error("wire send error",
								"wireID", w.ID,
								"error", err,
							)
						}
					}
				}
			}
		}
	}
}

// mergeInputWires 는 여러 입력 와이어 채널을 하나로 병합한다 (fan-in).
func (e *Engine) mergeInputWires(ctx context.Context, wires []*RuntimeWire) <-chan message.Message {
	merged := make(chan message.Message)

	var wg sync.WaitGroup
	for _, w := range wires {
		wg.Add(1)
		go func(ch <-chan message.Message) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case merged <- msg:
					}
				}
			}
		}(w.Ch)
	}

	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}
