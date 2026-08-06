package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
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
	observer        *observe.Observer
	nodeRegistry    *node.Registry
	shutdownTimeout time.Duration
	bpPolicy        BackpressurePolicy
	nodeOpts        []node.NodeOption
	debugSink       node.DebugSink // output 노드의 editor 출력용 싱크
	config          map[string]any

	// outputObserver 는 노드 출력 메시지 관측용 옵저버를 보관한다 (선택).
	// atomic.Value 에 outputObserverHolder 를 저장하여 핫 패스에서 lock-free 로
	// 읽는다. 미설정 시 Load() 는 nil 을 반환한다. notifyOutputObserver 참조.
	outputObserver atomic.Value
	onAgentStart   func(agent.Agent) // 에이전트 자동 시작 후 콜백
	agentManager   agent.Manager     // 플로우 배포 시 에이전트 참조 검증용 (선택)

	// unconnectedWarned 는 portCounter 가 없는 경로(주로 테스트)에서 미연결 포트
	// 경고를 (nodeID, portName)당 1회로 제한하기 위한 폴백 dedupe 맵이다.
	unconnectedWarned sync.Map
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

// NodeRegistry 는 Engine에 설정된 노드 레지스트리를 반환한다.
func (e *Engine) NodeRegistry() *node.Registry {
	return e.nodeRegistry
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

	// 1.5. 에이전트 참조 유효성 검증 (agentManager가 설정된 경우)
	// 2026-05-14: 누락/비활성 에이전트는 deploy 를 막지 않고 경고만 남긴다.
	// 노드는 Init-tolerance 로 대기 상태가 되고, 에이전트가 활성화되면
	// ReinitNodesForAgent (OnStart 콜백) 가 자동 연결한다.
	// "시스템 시작 후 에이전트 수동 활성화" 워크플로우를 지원하기 위함이다.
	if err := e.validateAgentRefs(f); err != nil {
		if e.logger != nil {
			e.logger.Warn("engine: 에이전트 참조 검증 경고 — deploy 계속 진행",
				"flowID", f.ID(),
				"error", err,
			)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 2. 중복 Flow ID 검사
	if _, exists := e.flows[f.ID()]; exists {
		return ErrFlowAlreadyDeployed
	}

	// 3. 각 NodeDef에 대해 런타임 노드 생성
	//    Observer가 설정된 경우, 노드별 계층적 로그 레벨을 적용한다.
	//    우선순위: 노드 config["log_level"] → 플로우 config.log_level → 데몬 기본값
	var closers []io.Closer
	runtimeNodes := make(map[string]node.Node)
	flowPrefix := resolveFlowComponentPrefix(f)
	for _, nd := range f.Nodes() {
		nodeOpts := make([]node.NodeOption, len(e.nodeOpts))
		copy(nodeOpts, e.nodeOpts)

		if e.observer != nil {
			component := fmt.Sprintf("flow.%s.node.%s", flowPrefix, nd.Name)
			nodeLogger := e.observer.Loggers.NewLogger(component)

			// 계층적 로그 레벨 결정 (항상 SetLevel 호출하여 daemon 기본값도 명시적으로 적용)
			lvl, _ := resolveNodeLogLevel(nd, f.Config(), e.observer.Levels.DefaultLevel())
			e.observer.Levels.SetLevel(component, lvl)

			if e.logger != nil {
				e.logger.Info("engine: 노드 로그 레벨 설정",
					"node", nd.Name,
					"level", lvl.String(),
					"flowLogLevel", f.Config().LogLevel,
					"daemonDefault", e.observer.Levels.DefaultLevel().String(),
				)
			}

			nodeOpts = append(nodeOpts, node.WithLogger(nodeLogger))

			// 계층적 로그 출력 대상 결정
			if target, ok := resolveNodeLogOutput(nd, f.Config(), ""); ok {
				if target.FilePath != "" {
					file, err := openLogFile(target.FilePath)
					if err != nil {
						// 롤백: 이미 열린 파일 핸들 정리
						for _, c := range closers {
							c.Close()
						}
						return fmt.Errorf("engine: failed to open log file for node %q: %w", nd.Name, err)
					}
					closers = append(closers, file)
					e.observer.Streams.AddRoute(component, file)
					if target.UseStdout {
						e.observer.Streams.AddRoute(component, os.Stdout)
					}
				}
				// stdout 전용: 라우트 미등록 → defaultWriter(stdout) 사용
			}
		}

		n, err := e.nodeRegistry.Create(nd, nodeOpts...)
		if err != nil {
			return fmt.Errorf("engine: failed to create node %q: %w", nd.Name, err)
		}

		// DebugSink 주입: output 노드(editor 출력)에 WebSocket 싱크를 연결한다.
		if e.debugSink != nil {
			if dn, ok := n.(*node.DebugNode); ok {
				dn.SetDebugSink(e.debugSink)
			}
		}

		// NodeDef.AgentRef가 있으면 Config에 agent_ref를 주입한다.
		// NASA, LGAP, MQTT, Modbus 등 에이전트 참조 노드가 config["agent_ref"]를 읽는다.
		if nd.AgentRef != nil {
			if nd.Config == nil {
				nd.Config = make(map[string]any)
			}
			ref := nd.AgentRef.AgentID
			if ref == "" {
				ref = nd.AgentRef.AgentName
			}
			if ref != "" {
				nd.Config["agent_ref"] = ref
			}
		}

		// NodeDef.Config가 있으면 노드에 설정을 전달한다.
		// expression, condition 등 YAML 설정이 노드에 적용된다.
		if nd.Config != nil {
			if cfgErr := n.Configure(nd.Config); cfgErr != nil {
				return fmt.Errorf("engine: failed to configure node %q: %w", nd.Name, cfgErr)
			}
		}

		runtimeNodes[nd.ID] = n
	}

	// 4. RuntimeWire 생성
	runtimeWires, err := CreateRuntimeWires(f.Wires())
	if err != nil {
		return fmt.Errorf("engine: failed to create runtime wires: %w", err)
	}

	// 5. flowRuntime 등록
	// 노드별 카운터 초기화
	counters := make(map[string]*nodeCounter, len(runtimeNodes))
	for id := range runtimeNodes {
		counters[id] = &nodeCounter{
			portCounters: make(map[string]*portCounter),
		}
	}

	// 노드별 미연결 출력 경고 억제(opt-out) 플래그를 config 에서 읽어 둔다.
	// 배포 시점에 한 번만 파싱하며, 해당 노드의 모든 portCounter 에 전파한다.
	suppressByNode := make(map[string]bool, len(runtimeNodes))
	for _, nd := range f.Nodes() {
		if parseSuppressUnconnected(nd.Config) {
			suppressByNode[nd.ID] = true
		}
	}

	// 노드가 선언한 모든 포트에 대해 카운터를 초기화한다.
	// 와이어 연결 여부와 관계없이 모든 포트의 카운터가 존재해야
	// 엔진의 "in"/"out"/"error" 기록이 누락되지 않는다.
	for id, n := range runtimeNodes {
		nc := counters[id]
		suppress := suppressByNode[id]
		for _, p := range n.Ports() {
			name := p.Name
			// 에러 포트는 노드에서 "_error"로 선언되지만
			// 엔진은 "error"로 기록하므로 둘 다 초기화한다.
			if _, ok := nc.portCounters[name]; !ok {
				nc.portCounters[name] = &portCounter{suppressUnconnected: suppress}
			}
			if p.Direction == flow.PortError {
				if _, ok := nc.portCounters["error"]; !ok {
					nc.portCounters["error"] = &portCounter{suppressUnconnected: suppress}
				}
			}
		}
	}

	// 와이어 정보로 포트 연결 상태 설정 및 추가 카운터 보충
	type portGetter interface {
		GetPort(name string) (*node.NodePort, bool)
	}
	for _, w := range runtimeWires {
		if nc := counters[w.SourceNodeID]; nc != nil {
			if _, ok := nc.portCounters[w.SourcePort]; !ok {
				nc.portCounters[w.SourcePort] = &portCounter{}
			}
		}
		if nc := counters[w.TargetNodeID]; nc != nil {
			if _, ok := nc.portCounters[w.TargetPort]; !ok {
				nc.portCounters[w.TargetPort] = &portCounter{}
			}
		}
		// 소스/타겟 노드의 포트 연결 상태를 true로 설정
		if srcNode, ok := runtimeNodes[w.SourceNodeID]; ok {
			if pg, ok := srcNode.(portGetter); ok {
				if p, ok := pg.GetPort(w.SourcePort); ok {
					p.Connected = true
				}
			}
		}
		if tgtNode, ok := runtimeNodes[w.TargetNodeID]; ok {
			if pg, ok := tgtNode.(portGetter); ok {
				if p, ok := pg.GetPort(w.TargetPort); ok {
					p.Connected = true
				}
			}
		}
	}

	// 비활성화된 노드 ID 집합을 구축한다.
	disabledSet := make(map[string]bool)
	for _, nd := range f.Nodes() {
		if !nd.IsEnabled() {
			disabledSet[nd.ID] = true
		}
	}

	rt := &flowRuntime{
		flow:          f,
		nodes:         runtimeNodes,
		wires:         runtimeWires,
		nodeCounters:  counters,
		disabledNodes: disabledSet,
		closers:       closers,
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
			// 모든 노드 라이프사이클을 Created 상태로 리셋하여 재시작이 가능하게 한다.
			resetNodeLifecycles(rt.nodes)
			// Initializing → Stopped → Loaded 순으로 플로우 상태를 롤백한다.
			e.mu.Lock()
			_ = rt.flow.SetState(flow.FlowStopped)
			_ = rt.flow.SetState(flow.FlowLoaded)
			e.mu.Unlock()
			return fmt.Errorf("%w: node %q init failed: %v", ErrNodeStartFailed, n.Name(), err)
		}
		initializedNodes = append(initializedNodes, nodeID)
	}

	// 브릿지 노드에 연결된 에이전트를 자동 시작한다.
	// 에이전트가 시작되지 않으면 시리얼 포트/브로커 연결이 열리지 않아
	// ReceiveMessage가 영구 차단되고 메시지 수신이 불가능하다.
	rt.autoStartedAgents = e.autoStartAgents(ctx, rt)

	// 노드별 goroutine 시작
	// 주의: HTTP 요청 컨텍스트(ctx)를 사용하면 안 된다.
	// API 요청이 완료되면 ctx가 취소되어 모든 노드 고루틴이 종료되기 때문이다.
	// 노드 고루틴은 StopFlow에서 cancel()을 호출할 때까지 독립적으로 실행되어야 한다.
	nodeCtx, cancel := context.WithCancel(context.Background())
	rt.cancel = cancel
	rt.startedAt = time.Now()

	// 각 nodeCounter 에 시작 시각 설정
	for _, nc := range rt.nodeCounters {
		nc.startedAt = time.Now()
	}

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

	// 각 노드 Shutdown 호출 (타임아웃 적용)
	nodeShutdownCtx, nodeShutdownCancel := context.WithTimeout(context.Background(), e.shutdownTimeout/2)
	defer nodeShutdownCancel()
	for _, n := range rt.nodes {
		if err := n.Shutdown(nodeShutdownCtx); err != nil {
			if e.logger != nil {
				e.logger.Error("node shutdown error", "nodeID", n.ID(), "error", err)
			}
		}
	}

	// 자동 시작된 에이전트 정지
	e.autoStopAgents(ctx, rt)

	// 로그 출력 파일 핸들 정리
	for _, c := range rt.closers {
		if err := c.Close(); err != nil {
			if e.logger != nil {
				e.logger.Error("log file close error", "flowID", flowID, "error", err)
			}
		}
	}
	rt.closers = nil

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

	state := rt.flow.State()
	if state != flow.FlowStopped && state != flow.FlowLoaded {
		return ErrFlowNotStopped
	}

	// 로그 출력 파일 핸들 정리 (StopFlow에서 이미 정리되었을 수 있음)
	for _, c := range rt.closers {
		if err := c.Close(); err != nil {
			if e.logger != nil {
				e.logger.Error("log file close error", "flowID", flowID, "error", err)
			}
		}
	}
	rt.closers = nil

	// StreamRouter 에서 해당 플로우의 노드 라우트 제거
	if e.observer != nil {
		undeployFlowPrefix := resolveFlowComponentPrefix(rt.flow)
		for _, nd := range rt.flow.Nodes() {
			component := fmt.Sprintf("flow.%s.node.%s", undeployFlowPrefix, nd.Name)
			for _, w := range e.observer.Streams.Routes(component) {
				e.observer.Streams.RemoveRoute(component, w)
			}
		}
	}

	delete(e.flows, flowID)

	if e.logger != nil {
		e.logger.Info("flow undeployed", "flowID", flowID)
	}

	return nil
}

// GetFlow 는 배포된 플로우의 정의를 반환한다.
// 저장소에 플로우가 없을 때 엔진 런타임의 플로우 정의를 사용하여
// 재배포할 수 있도록 지원한다.
func (e *Engine) GetFlow(flowID string) (flow.Flow, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return nil, ErrFlowNotFound
	}
	return rt.flow, nil
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

// GetFlowNodes 는 배포된 Flow의 모든 노드 인스턴스 정보를 반환한다.
func (e *Engine) GetFlowNodes(flowID string) ([]NodeInstanceInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return nil, ErrFlowNotFound
	}

	result := make([]NodeInstanceInfo, 0, len(rt.nodes))
	for _, n := range rt.nodes {
		result = append(result, buildNodeInstanceInfo(n, rt.nodeCounters[n.ID()]))
	}

	return result, nil
}

// GetFlowNode 는 배포된 Flow 내 특정 노드 인스턴스의 정보를 반환한다.
// nodeIDOrName 은 노드 ID(UUID) 또는 노드 이름으로 검색할 수 있다.
// ID로 먼저 검색하고, 없으면 이름으로 폴백 검색한다.
func (e *Engine) GetFlowNode(flowID, nodeIDOrName string) (*NodeInstanceInfo, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, exists := e.flows[flowID]
	if !exists {
		return nil, ErrFlowNotFound
	}

	// ID로 먼저 검색
	if n, exists := rt.nodes[nodeIDOrName]; exists {
		info := buildNodeInstanceInfo(n, rt.nodeCounters[n.ID()])
		return &info, nil
	}

	// 이름으로 폴백 검색
	for _, n := range rt.nodes {
		if n.Name() == nodeIDOrName {
			info := buildNodeInstanceInfo(n, rt.nodeCounters[n.ID()])
			return &info, nil
		}
	}

	return nil, ErrNodeNotFound
}

// ReconfigureNode 는 실행 중인 Flow 내 특정 노드 인스턴스에 부분 설정을 즉시 적용한다.
// 저장/재배포 없이 동작 중인 노드의 Configure 를 호출하여 라이브로 설정을 반영한다.
// nodeIDOrName 은 노드 ID(UUID) 또는 노드 이름으로 검색하며, ID 우선·이름 폴백이다.
//
// config 는 변경된 키만 담은 부분 설정이다. 노드의 Configure 구현은 자신이 아는 키만
// 읽으므로(예: 출력 노드는 output_enabled 키만 읽고 나머지 상태는 건드리지 않는다)
// 부분 설정을 전달해도 기존 상태가 유실되지 않는다.
//
// 동시성: 엔진 락(e.mu)은 노드 참조를 확보할 때까지만 보유하며, n.Configure 호출 전에
// 반드시 해제한다. 노드 Configure 가 내부적으로 다른 엔진 경로를 호출하더라도
// 재진입(deadlock)이 발생하지 않도록 보장한다.
func (e *Engine) ReconfigureNode(flowID, nodeIDOrName string, config map[string]any) error {
	e.mu.RLock()

	rt, exists := e.flows[flowID]
	if !exists {
		e.mu.RUnlock()
		return ErrFlowNotFound
	}

	// ID로 먼저 검색하고, 없으면 이름으로 폴백 검색한다 (GetFlowNode 와 동일한 로직).
	var target node.Node
	if n, ok := rt.nodes[nodeIDOrName]; ok {
		target = n
	} else {
		for _, n := range rt.nodes {
			if n.Name() == nodeIDOrName {
				target = n
				break
			}
		}
	}

	// 노드 참조를 확보했으므로 Configure 호출 전에 엔진 락을 해제한다.
	e.mu.RUnlock()

	if target == nil {
		return ErrNodeNotFound
	}

	if err := target.Configure(config); err != nil {
		return fmt.Errorf("engine: 노드 재설정 실패 (flow=%s, node=%s): %w", flowID, nodeIDOrName, err)
	}

	return nil
}

// ResolveNodeName 은 배포된 플로우 내 노드 ID로부터 노드 이름을 반환한다.
// flowID가 빈 문자열이면 모든 배포된 플로우에서 nodeID를 검색한다.
// 플로우나 노드를 찾을 수 없으면 false 를 반환한다.
func (e *Engine) ResolveNodeName(flowID, nodeID string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if flowID != "" {
		rt, exists := e.flows[flowID]
		if !exists {
			return "", false
		}
		if n, exists := rt.nodes[nodeID]; exists {
			return n.Name(), true
		}
		return "", false
	}

	// flowID가 빈 경우: 모든 플로우에서 검색
	for _, rt := range e.flows {
		if n, exists := rt.nodes[nodeID]; exists {
			return n.Name(), true
		}
	}
	return "", false
}

// ResolveFlowName 은 배포된 플로우 ID로부터 플로우 이름을 반환한다.
// 플로우를 찾을 수 없으면 false 를 반환한다.
func (e *Engine) ResolveFlowName(flowID string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if flowID == "" {
		return "", false
	}
	rt, exists := e.flows[flowID]
	if !exists {
		return "", false
	}
	return rt.flow.Name(), true
}

// ResolveFlowNameByNodeID 는 노드 ID로부터 해당 노드가 속한 플로우의 이름과 ID를 반환한다.
// 노드를 찾을 수 없으면 false 를 반환한다.
func (e *Engine) ResolveFlowNameByNodeID(nodeID string) (flowID, flowName string, ok bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for fid, rt := range e.flows {
		if _, exists := rt.nodes[nodeID]; exists {
			return fid, rt.flow.Name(), true
		}
	}
	return "", "", false
}

// buildNodeInstanceInfo 는 node.Node로부터 NodeInstanceInfo를 구성한다.
// nc 가 nil 이 아니면 처리/에러 카운터를 포함한다.
func buildNodeInstanceInfo(n node.Node, nc *nodeCounter) NodeInstanceInfo {
	info := NodeInstanceInfo{
		NodeID: n.ID(),
		Name:   n.Name(),
		Type:   n.Type(),
	}

	// 라이프사이클 상태 조회 (BaseLifecycle 임베딩)
	type stateQuerier interface {
		CurrentState() lifecycle.State
	}
	if sq, ok := n.(stateQuerier); ok {
		info.State = string(sq.CurrentState())
	}

	// 설정 조회 (BaseNode.GetConfig)
	type configQuerier interface {
		GetConfig() map[string]any
	}
	if cq, ok := n.(configQuerier); ok {
		info.Config = cq.GetConfig()
	}

	// 포트 정보 조회 (포트별 통계 포함)
	for _, p := range n.Ports() {
		pi := NodePortInfo{
			ID:        p.ID,
			Name:      p.Name,
			Direction: string(p.Direction),
			Connected: p.Connected,
		}
		if nc != nil {
			// 포트 이름으로 카운터 조회; 에러 포트는 노드에서 "_error",
			// 와이어에서 "error"로 저장되므로 방향 기반 fallback 수행
			pc := nc.portCounters[p.Name]
			if pc == nil && p.Direction == flow.PortError {
				pc = nc.portCounters["error"]
			}
			if pc != nil {
				snap := pc.Snapshot()
				pi.Messages = snap.Messages
				pi.Delivered = snap.Delivered
				pi.Throughput = snap.Throughput
				pi.ActiveFor = snap.ActiveFor
			}
		}
		info.Ports = append(info.Ports, pi)
	}

	// 노드별 처리/에러 카운터
	if nc != nil {
		info.Processed = nc.processed.Load()
		info.Errors = nc.errors.Load()
	}

	// 노드 타입별 추가 정보 (aggregate 통계 등)
	type infoProvider interface {
		Info() map[string]any
	}
	if ip, ok := n.(infoProvider); ok {
		info.Extra = ip.Info()
	}

	return info
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

// FlowSummaries 는 배포된 모든 Flow의 요약 정보를 inventory 노드용 작은 DTO 로 반환한다.
// node.FlowRegistry 인터페이스를 만족하여, inventory 노드가 engine 패키지를 import 하지
// 않고도 flow 목록에 접근할 수 있게 한다 (SPEC-INVENTORY-001 결정 a3 — 단방향 의존 유지).
//
// 본 메서드는 read-only 이며 ListFlows 의 상위 호환 어댑터이다. FlowStatus 의
// 필드 중 inventory 가 필요로 하는 부분만 노출하므로 향후 engine 의 내부 구조 변경에
// 안정적이다.
func (e *Engine) FlowSummaries() []node.FlowSummary {
	statuses := e.ListFlows()
	result := make([]node.FlowSummary, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, node.FlowSummary{
			ID:        s.FlowID,
			Name:      s.FlowName,
			State:     string(s.State),
			NodeCount: s.NodeCount,
			WireCount: s.WireCount,
			Extra: map[string]any{
				"active_nodes":  s.ActiveNodes,
				"message_count": s.MessageCount,
				"error_count":   s.ErrorCount,
				"dropped_count": s.DroppedCount,
				"uptime_ms":     s.Uptime.Milliseconds(),
			},
		})
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
// resetNodeLifecycles 는 런타임의 모든 노드 라이프사이클을 Created 상태로 리셋한다.
// 초기화 실패 후 재시작을 가능하게 하기 위해 사용된다.
func resetNodeLifecycles(nodes map[string]node.Node) {
	type stateResetter interface {
		CurrentState() lifecycle.State
		TransitionTo(lifecycle.State) error
	}

	for _, n := range nodes {
		if n == nil {
			continue
		}
		sr, ok := n.(stateResetter)
		if !ok {
			continue
		}

		switch sr.CurrentState() {
		case lifecycle.StateCreated:
			// 이미 Created 상태
		case lifecycle.StateInitializing:
			_ = sr.TransitionTo(lifecycle.StateError)
			_ = sr.TransitionTo(lifecycle.StateStopped)
			_ = sr.TransitionTo(lifecycle.StateCreated)
		case lifecycle.StateStopping:
			_ = sr.TransitionTo(lifecycle.StateStopped)
			_ = sr.TransitionTo(lifecycle.StateCreated)
		case lifecycle.StateStopped:
			_ = sr.TransitionTo(lifecycle.StateCreated)
		case lifecycle.StateError:
			_ = sr.TransitionTo(lifecycle.StateStopped)
			_ = sr.TransitionTo(lifecycle.StateCreated)
		case lifecycle.StateRunning, lifecycle.StatePaused:
			_ = sr.TransitionTo(lifecycle.StateStopping)
			_ = sr.TransitionTo(lifecycle.StateStopped)
			_ = sr.TransitionTo(lifecycle.StateCreated)
		}
	}
}

// connectedAgentProvider 는 에이전트에 연결된 노드(BridgeNode)를 위한 인터페이스이다.
type connectedAgentProvider interface {
	ConnectedAgent() agent.Agent
}

// autoStartAgents 는 브릿지 노드에 연결된 에이전트를 자동 시작한다.
// 이미 실행 중인 에이전트는 건너뛴다. 자동 시작된 에이전트 목록을 반환한다.
func (e *Engine) autoStartAgents(ctx context.Context, rt *flowRuntime) []agent.Agent {
	started := make(map[string]bool)
	var autoStarted []agent.Agent

	for nodeID, n := range rt.nodes {
		provider, ok := n.(connectedAgentProvider)
		if !ok {
			continue
		}
		// 비활성화된 노드는 연결 에이전트를 자동 시작하지 않는다.
		// dedup(started) 이전에 건너뛰어, 같은 에이전트를 참조하는 다른 활성 노드는
		// 정상적으로 에이전트를 시작할 수 있게 한다.
		if rt.disabledNodes[nodeID] {
			if e.logger != nil {
				e.logger.Info("engine: 비활성화된 노드의 에이전트 자동 시작 건너뜀",
					"nodeID", nodeID)
			}
			continue
		}
		ag := provider.ConnectedAgent()
		if ag == nil || started[ag.ID()] {
			continue
		}
		started[ag.ID()] = true

		// SPEC-AGENT-005: Enabled=false 는 "실행 금지" 의도이므로 자동 시작에서 제외한다.
		// 부팅 시 restoreAgents(cmd/xflowd/main.go)가 비활성화 에이전트를 건너뛰는 것과
		// 동일하게, 플로우 트리거 경로에서도 사용자의 비활성화 의도를 존중해야 한다.
		// 결과적으로 비활성화 에이전트는 재활성화 전까지 시작되지 않는다.
		// Info().Config 는 값 복사본이며 IsEnabled 는 포인터 리시버이므로 지역 변수에 담아 호출한다.
		agCfg := ag.Info().Config
		if !agCfg.IsEnabled() {
			if e.logger != nil {
				e.logger.Info("engine: 비활성화된 에이전트 자동 시작 건너뜀",
					"agentID", ag.ID(), "agentName", ag.Name())
			}
			continue
		}

		// Init() 후 StateRunning 상태여도 Start()를 호출한다.
		// 각 에이전트가 자체 멱등성을 보장한다 (이미 시작된 경우 no-op).
		if err := ag.Start(ctx); err != nil {
			if e.logger != nil {
				e.logger.Warn("engine: auto-start agent failed",
					"agentID", ag.ID(), "agentName", ag.Name(), "error", err)
			}
			continue
		}
		autoStarted = append(autoStarted, ag)
		if e.onAgentStart != nil {
			e.onAgentStart(ag)
		}
		if e.logger != nil {
			e.logger.Info("engine: auto-started agent for flow",
				"agentID", ag.ID(), "agentName", ag.Name())
		}
	}
	return autoStarted
}

// autoStopAgents 는 플로우 시작 시 자동 시작된 에이전트를 정지한다.
func (e *Engine) autoStopAgents(_ context.Context, rt *flowRuntime) {
	perAgent := e.shutdownTimeout / time.Duration(max(len(rt.autoStartedAgents), 1))
	if perAgent > 15*time.Second {
		perAgent = 15 * time.Second
	}
	for _, ag := range rt.autoStartedAgents {
		agCtx, agCancel := context.WithTimeout(context.Background(), perAgent)
		if err := ag.Stop(agCtx); err != nil {
			if e.logger != nil {
				e.logger.Warn("engine: auto-stop agent failed",
					"agentID", ag.ID(), "agentName", ag.Name(), "error", err)
			}
		} else {
			if e.logger != nil {
				e.logger.Info("engine: auto-stopped agent",
					"agentID", ag.ID(), "agentName", ag.Name())
			}
		}
		agCancel()
	}
	rt.autoStartedAgents = nil
}

// ReinitNodesForAgent 는 지정된 에이전트를 참조하는 모든 실행 중인 에이전트 백엔드
// 노드를 재초기화한다. node.AgentReinitializer 인터페이스를 구현한 모든 노드
// (BridgeNode, NASA*, LGCP*, Hvacr01*, LGAP*, MQTT*, Modbus*, InfluxDB*, TSDB*,
// Serial*, TCP* 등) 가 대상이다.
//
// 에이전트 lifecycle 이벤트 - Restart() 또는 Stop()+Start() - 후에 호출되어
// transport 교체, agent 참조 갱신, FrameNotifier 채널 재구독, 폴링 / 수신
// 루프 재시작 등을 노드별로 처리한다.
//
// 실행 중이 아닌 (rt.cancel == nil) 플로우는 건너뛴다. 매칭되지 않는 (AgentID /
// AgentName 미일치) 노드도 건너뛴다.
func (e *Engine) ReinitNodesForAgent(agentID, agentName string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for flowID, rt := range e.flows {
		if rt.cancel == nil {
			continue // 실행 중이 아닌 플로우 건너뛰기
		}
		for _, n := range rt.nodes {
			reinit, ok := n.(node.AgentReinitializer)
			if !ok {
				continue
			}
			ref := reinit.AgentRef()
			if ref.AgentID != agentID && ref.AgentName != agentName {
				continue
			}
			if e.logger != nil {
				e.logger.Info("engine: 에이전트 재시작으로 노드 재초기화",
					"flowID", flowID,
					"nodeID", n.ID(),
					"agentName", agentName,
				)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := reinit.Reinit(ctx); err != nil {
				if e.logger != nil {
					e.logger.Error("engine: 노드 재초기화 실패",
						"flowID", flowID,
						"nodeID", n.ID(),
						"error", err,
					)
				}
			}
			cancel()
		}
	}
}

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

// splitOutputWires 는 출력 와이어를 SourcePort 기준으로 "out" 와이어와 "error" 와이어로 분리한다.
func splitOutputWires(wires []*RuntimeWire) (outWires, errWires []*RuntimeWire) {
	for _, w := range wires {
		if w.SourcePort == "error" {
			errWires = append(errWires, w)
		} else {
			outWires = append(outWires, w)
		}
	}
	return
}

// groupWiresBySourcePort 는 와이어를 SourcePort 이름 기준으로 그룹핑한다.
// MultiSourceNode의 추가 포트별 라우팅에 사용된다.
func groupWiresBySourcePort(wires []*RuntimeWire) map[string][]*RuntimeWire {
	result := make(map[string][]*RuntimeWire)
	for _, w := range wires {
		port := w.SourcePort
		if port == "" {
			port = "out"
		}
		result[port] = append(result[port], w)
	}
	return result
}

// nodeWithLogger 는 ComponentLogger를 보유한 노드의 선택적 인터페이스이다.
// BaseNode가 Logger()를 구현하므로 모든 구체 노드 타입이 이 인터페이스를 만족한다.
type nodeWithLogger interface {
	Logger() observe.ComponentLogger
}

// nodeDeclaresOutputPort 는 노드가 portName 을 출력 포트로 선언했는지 보고한다.
//
// 판정은 보수적(OR 결합)으로 수행하여 하위호환을 최대한 보장한다:
//   - Ports() 목록(node.Node 핵심 인터페이스, 항상 구현됨)에 Direction==PortOutput
//     이고 Name==portName 인 포트가 있으면 선언된 것으로 본다. switch 처럼 라우트
//     포트를 동적으로 계산하는 노드의 출력 포트도 이 경로로 정확히 인식된다.
//   - 추가로 선택적 GetOutputPort 조회 인터페이스(BaseNode 구현)가 선언을 보고하면
//     역시 선언된 것으로 본다(정적 outputs 기준).
//
// 두 출처 중 어느 쪽도 선언을 보고하지 않을 때에만 미선언(false)으로 판정하므로,
// 동적/정적 포트 모두에 대해 안전하다. 두 인터페이스를 모두 구현하지 않는 노드는
// 게이트 대상이 아니다(true 반환, 기존 동작 유지).
func nodeDeclaresOutputPort(n node.Node, portName string) bool {
	type outputPortGetter interface {
		GetOutputPort(name string) (*node.NodePort, bool)
	}

	declaredViaPorts := false
	type portsLister interface {
		Ports() []node.NodePort
	}
	if pl, ok := n.(portsLister); ok {
		for _, p := range pl.Ports() {
			if p.Direction == flow.PortOutput && p.Name == portName {
				declaredViaPorts = true
				break
			}
		}
	} else {
		// Ports() 를 구현하지 않으면 선언 여부를 알 수 없으므로 게이트하지 않는다.
		return true
	}

	if g, ok := n.(outputPortGetter); ok {
		if _, declared := g.GetOutputPort(portName); declared {
			return true
		}
	}

	return declaredViaPorts
}

// debugPortLog 는 노드 로그 레벨이 Debug일 때 포트 입출력 메시지를 로깅한다.
// slog.Logger.Enabled() 체크로 불필요한 Payload.ToMap() 비용을 방지한다.
func debugPortLog(ctx context.Context, logger observe.ComponentLogger, direction string, nodeID string, msg message.Message) {
	if logger == nil {
		return
	}
	slogger := logger.Logger()
	if slogger == nil || !slogger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	logger.Debug("port."+direction,
		"nodeID", nodeID,
		"msgID", msg.ID(),
		"payload", msg.Payload().ToMap(),
	)
}

// sendToWires 는 메시지를 와이어 목록으로 fan-out 전송한다.
// 마지막 와이어에는 원본을, 나머지에는 Clone을 전송한다.
// sendToWires 는 메시지를 매칭된 와이어들로 전송(fan-out)하고,
// 성공적으로 전달된 와이어 수를 반환한다.
// 반환값 > 0 이면 해당 포트의 메시지가 최소 1개 이상의 와이어로 전달되었음을 의미하며,
// 호출 측에서 이를 기반으로 포트의 delivered 카운터를 증가시킨다.
func (e *Engine) sendToWires(ctx context.Context, msg message.Message, wires []*RuntimeWire, nodeID string) int {
	delivered := 0
	for i, w := range wires {
		var msgToSend message.Message
		if i == len(wires)-1 {
			msgToSend = msg
		} else {
			msgToSend = msg.Clone()
		}
		if err := w.Send(ctx, msgToSend); err != nil {
			if e.logger != nil {
				e.logger.Error("wire send error",
					"nodeID", nodeID,
					"wireID", w.ID,
					"error", err,
				)
			}
			continue
		}
		delivered++
	}
	return delivered
}

// warnUnconnectedPort 는 메시지가 생산되었으나 연결된 와이어가 없는 포트에 대해
// 경고를 로깅한다. pc.warnedUnconnected 가드를 통해 (nodeID, portName) 조합당
// 단 한 번만 로깅되어 로그 스팸을 방지한다. pc 가 nil 이면(테스트 등) 가드 없이
// 매번 로깅하지 않도록 별도 dedupe 맵을 사용한다.
func (e *Engine) warnUnconnectedPort(nodeID, nodeName, portName string, pc *portCounter) {
	if e.logger == nil {
		return
	}
	if pc != nil {
		// 노드별 옵트아웃: suppress 설정 시 미연결 경고를 완전히 억제한다.
		// once-guard CAS 보다 먼저 검사하여 guard 를 소비하지 않는다.
		// 주의: pc == nil 경로(아래 단위 테스트 폴백)는 노드별 플래그가 없으므로
		// 억제할 수 없다. 억제는 실제 배포 경로(pc != nil)에서만 동작한다.
		if pc.suppressUnconnected {
			return
		}
		// portCounter 기반 1회 가드 (정상 경로).
		if !pc.warnedUnconnected.CompareAndSwap(false, true) {
			return // 이미 경고함
		}
	} else {
		// pc 가 없는 경우(주로 단위 테스트): (nodeID, portName) 키 기반 가드.
		key := nodeID + "\x00" + portName
		if _, loaded := e.unconnectedWarned.LoadOrStore(key, true); loaded {
			return // 이미 경고함
		}
	}
	e.logger.Warn("engine: 출력 포트에 연결된 와이어 없음 — 메시지 폐기",
		"nodeID", nodeID,
		"nodeName", nodeName,
		"port", portName,
	)
}

// suppressUnconnectedWarningKey 는 노드 미연결 출력 경고 억제 옵트인 config 키이다.
const suppressUnconnectedWarningKey = "suppress_unconnected_warning"

// parseSuppressUnconnected 는 노드 config 에서 suppress_unconnected_warning 값을
// 관대하게(tolerant) 파싱한다. Go bool 과 문자열("true"/"false"/"1"/"0" 등,
// strconv.ParseBool 규칙) 을 모두 허용하며, 키가 없거나 파싱 불가하면 false 를
// 반환한다 (기본 비활성).
func parseSuppressUnconnected(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	v, ok := cfg[suppressUnconnectedWarningKey]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		if err != nil {
			return false
		}
		return b
	default:
		return false
	}
}

// sendErrorToWires 는 에러가 발생한 원본 메시지에 에러 메타데이터를 추가하여 에러 와이어로 전송한다.
func (e *Engine) sendErrorToWires(ctx context.Context, msg message.Message, processErr error, errWires []*RuntimeWire, nodeID string) {
	if len(errWires) == 0 {
		return
	}
	errMsg := msg.Clone()
	errMsg.Metadata().Set(message.MetaKeyError, processErr.Error())
	errMsg.Metadata().Set(message.MetaKeyErrorNodeID, nodeID)
	e.sendToWires(ctx, errMsg, errWires, nodeID)
}

// runNode 는 단일 노드의 메시지 처리 goroutine이다.
// 입력 와이어에서 메시지를 읽어 노드의 Process를 호출하고 출력 와이어로 전송한다.
// 출력 와이어는 SourcePort 기준으로 "out"(정상)과 "error"(에러)로 분리되어 라우팅된다.
func (e *Engine) runNode(
	ctx context.Context,
	rt *flowRuntime,
	n node.Node,
	inputWires []*RuntimeWire,
	outputWires []*RuntimeWire,
) {
	defer rt.wg.Done()

	// 노드의 ComponentLogger를 추출한다 (디버그 포트 로깅에 사용).
	var nodeLogger observe.ComponentLogger
	if ln, ok := n.(nodeWithLogger); ok {
		nodeLogger = ln.Logger()
	}

	nodeID := n.ID()
	// flowID 는 OutputObserver(노드 출력 tap) 통지에 사용된다.
	flowID := rt.flow.ID()

	// 비활성화된 노드: 메시지를 소비만 하고 처리/전달하지 않는다.
	if rt.disabledNodes[nodeID] {
		if e.logger != nil {
			e.logger.Info("engine: 비활성화 노드, 메시지 건너뜀",
				"nodeID", nodeID,
				"nodeName", n.Name(),
			)
		}
		// SourceNode 인 경우 SourceCh 를 드레인한다.
		if src, ok := n.(node.SourceNode); ok && len(inputWires) == 0 {
			// MultiSourceNode인 경우 추가 채널도 드레인한다.
			if multi, ok := n.(node.MultiSourceNode); ok {
				for _, portCh := range multi.ExtraSourceChannels() {
					portCh := portCh
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case _, ok := <-portCh:
								if !ok {
									return
								}
								rt.droppedCount.Add(1)
							}
						}
					}()
				}
			}
			ch := src.SourceCh()
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-ch:
					if !ok {
						return
					}
					rt.droppedCount.Add(1)
				}
			}
		}
		// 일반 노드: 입력 와이어를 드레인한다.
		if len(inputWires) > 0 {
			merged := e.mergeInputWires(ctx, inputWires)
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-merged:
					if !ok {
						return
					}
					rt.droppedCount.Add(1)
				}
			}
		}
		<-ctx.Done()
		return
	}

	// 출력 와이어를 "out" 포트와 "error" 포트로 분리한다.
	outWires, errWires := splitOutputWires(outputWires)

	// 입력 와이어가 없는 노드: SourceNode이면 SourceCh에서 읽어 출력 와이어로 전달한다.
	if len(inputWires) == 0 {
		if src, ok := n.(node.SourceNode); ok {
			ch := src.SourceCh()

			// MultiSourceNode인 경우 추가 포트별 고루틴을 시작한다.
			if multi, ok := n.(node.MultiSourceNode); ok {
				portWires := groupWiresBySourcePort(outWires)
				for pn, pch := range multi.ExtraSourceChannels() {
					targetWires := portWires[pn]
					if len(targetWires) == 0 {
						continue // 연결된 와이어가 없으면 건너뜀
					}
					go func(portName string, portCh <-chan message.Message, wires []*RuntimeWire) {
						for {
							select {
							case <-ctx.Done():
								return
							case msg, ok := <-portCh:
								if !ok {
									return
								}
								for rt.paused.Load() {
									select {
									case <-ctx.Done():
										return
									case <-time.After(10 * time.Millisecond):
									}
								}
								rt.messageCount.Add(1)
								var pc *portCounter
								if nc := rt.nodeCounters[n.ID()]; nc != nil {
									nc.processed.Add(1)
									if pc = nc.portCounters[portName]; pc != nil {
										pc.Record() // emitted (생산)
									}
								}
								// 노드 출력 관측(tap): 와이어 연결 여부와 무관하게 방출 시점에 통지한다.
								e.notifyOutputObserver(flowID, n.ID(), portName, msg)
								// 이 고루틴은 len(wires) > 0 인 포트에 대해서만 시작되므로
								// 항상 연결된 와이어가 존재한다.
								if e.sendToWires(ctx, msg, wires, n.ID()) > 0 && pc != nil {
									pc.RecordDelivered() // delivered (실제 전달)
								}
							}
						}
					}(pn, pch, targetWires)
				}
				// 기본 "out" 와이어만 분리 (추가 포트 와이어 제외)
				if defaultWires, ok := portWires["out"]; ok {
					outWires = defaultWires
				}
			}

			if e.logger != nil {
				e.logger.Info("engine: SourceNode 수신 대기 시작",
					"nodeID", n.ID(),
					"nodeName", n.Name(),
					"outWires", len(outWires),
				)
			}
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
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
					rt.messageCount.Add(1)
					var pc *portCounter
					if nc := rt.nodeCounters[n.ID()]; nc != nil {
						nc.processed.Add(1)
						if pc = nc.portCounters["out"]; pc != nil {
							pc.Record() // emitted (생산)
						}
					}
					if e.logger != nil && nodeLogger != nil && nodeLogger.Logger().Enabled(ctx, slog.LevelDebug) {
						e.logger.Debug("engine: SourceNode 메시지 라우팅",
							"nodeID", n.ID(),
							"nodeName", n.Name(),
							"msgID", msg.ID(),
							"outWires", len(outWires),
							"totalMessages", rt.messageCount.Load(),
						)
					}
					debugPortLog(ctx, nodeLogger, "source", n.ID(), msg)
					// 미선언 출력 포트 emit 게이트(SourceNode "out" 경로, 보수적 적용):
					// "out" 와이어가 없고 노드가 "out" 출력 포트를 선언하지 않았다면
					// tap 통지와 미연결 경고를 생략한다. emit 카운트(pc.Record())는 위에서
					// 이미 기록되었으므로 source 경로에서는 동작을 깨지 않도록 통지+경고만
					// 게이트한다("out" 은 절대 "error" 포트가 아니므로 조건 2는 항상 참).
					if len(outWires) == 0 && !nodeDeclaresOutputPort(n, "out") {
						// 미선언+미연결: 조용히 폐기(통지·경고 생략).
						continue
					}
					// 노드 출력 관측(tap): 와이어 연결 여부와 무관하게 방출 시점에 통지한다.
					e.notifyOutputObserver(flowID, n.ID(), "out", msg)
					if len(outWires) == 0 {
						e.warnUnconnectedPort(n.ID(), n.Name(), "out", pc)
					} else if e.sendToWires(ctx, msg, outWires, n.ID()) > 0 && pc != nil {
						pc.RecordDelivered() // delivered (실제 전달)
					}
				}
			}
		}
		// SourceNode가 아니면 context 취소만 대기한다.
		if e.logger != nil {
			e.logger.Warn("engine: 입력 와이어 없는 비-SourceNode, 대기 중",
				"nodeID", n.ID(),
				"nodeName", n.Name(),
			)
		}
		<-ctx.Done()
		return
	}

	// MultiSourceNode 이면서 입력 와이어를 가진 노드: fan-in Process 경로와 병행하여
	// 추가 소스 포트(ExtraSourceChannels)도 배출한다. 순수 소스(입력 와이어 없음)는 위
	// len(inputWires)==0 경로에서 이미 처리되므로, 이 경로는 "MultiSourceNode + 입력 와이어"
	// 조합에만 적용된다(현재 유일 대상: Samsung mirror-message 결합 노드의 mirror-out).
	// 타입 게이트로 다른 노드(순수 소스 SerialInNode 등)는 영향받지 않는다(행위 보존).
	// "out" 포트는 Process 결과가 담당하며, ExtraSourceChannels 는 "out" 이 아닌 추가 포트만
	// 반환하므로 결과 라우팅(SourcePort=="out")과 충돌하지 않는다.
	if multi, ok := n.(node.MultiSourceNode); ok {
		portWires := groupWiresBySourcePort(outWires)
		for pn, pch := range multi.ExtraSourceChannels() {
			targetWires := portWires[pn]
			if len(targetWires) == 0 {
				continue // 연결된 와이어가 없으면 건너뜀
			}
			go func(portName string, portCh <-chan message.Message, wires []*RuntimeWire) {
				for {
					select {
					case <-ctx.Done():
						return
					case msg, ok := <-portCh:
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
						rt.messageCount.Add(1)
						var pc *portCounter
						if nc := rt.nodeCounters[n.ID()]; nc != nil {
							nc.processed.Add(1)
							if pc = nc.portCounters[portName]; pc != nil {
								pc.Record() // emitted (생산)
							}
						}
						// 노드 출력 관측(tap): 와이어 연결 여부와 무관하게 방출 시점에 통지한다.
						e.notifyOutputObserver(flowID, n.ID(), portName, msg)
						if e.sendToWires(ctx, msg, wires, n.ID()) > 0 && pc != nil {
							pc.RecordDelivered() // delivered (실제 전달)
						}
					}
				}
			}(pn, pch, targetWires)
		}
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

			// 입력 포트 디버그 로깅
			debugPortLog(ctx, nodeLogger, "input", n.ID(), msg)

			if e.logger != nil && nodeLogger != nil && nodeLogger.Logger().Enabled(ctx, slog.LevelDebug) {
				e.logger.Debug("engine: 노드 Process 호출",
					"nodeID", n.ID(),
					"nodeName", n.Name(),
					"msgID", msg.ID(),
				)
			}

			// 노드 처리
			results, err := n.Process(ctx, msg)
			rt.messageCount.Add(1)
			if nc := rt.nodeCounters[n.ID()]; nc != nil {
				nc.processed.Add(1)
				if pc := nc.portCounters["in"]; pc != nil {
					pc.Record()
				}
			}

			if err != nil {
				rt.errorCount.Add(1)
				if nc := rt.nodeCounters[n.ID()]; nc != nil {
					nc.errors.Add(1)
					if pc := nc.portCounters["error"]; pc != nil {
						pc.Record()
					}
				}
				if e.logger != nil {
					e.logger.Error("node process error",
						"nodeID", n.ID(),
						"error", err,
					)
				}
				// 에러 포트 디버그 로깅
				debugPortLog(ctx, nodeLogger, "error", n.ID(), msg)
				// 에러 와이어가 있으면 원본 메시지에 에러 정보를 추가하여 전송한다.
				e.sendErrorToWires(ctx, msg, err, errWires, n.ID())
				continue
			}

			// 출력 와이어로 결과 전송 (fan-out)
			// _target_port 메타데이터가 있으면 해당 포트의 와이어만 사용한다.
			for _, result := range results {
				targetPort := ""
				if tp, ok := result.Metadata().Get("_target_port"); ok {
					targetPort = tp
					result.Metadata().Remove("_target_port") // 하류 노드에 전달하지 않음
				}

				var targetWires []*RuntimeWire
				if targetPort != "" {
					// 포트별 와이어 매칭: 해당 포트 와이어만 사용 (없으면 폐기)
					for _, w := range outWires {
						if w.SourcePort == targetPort {
							targetWires = append(targetWires, w)
						}
					}
				} else {
					// 기본: "out" 포트 와이어 (SourcePort가 비어있거나 "out"인 것)
					for _, w := range outWires {
						if w.SourcePort == "" || w.SourcePort == "out" {
							targetWires = append(targetWires, w)
						}
					}
				}

				portName := targetPort
				if portName == "" {
					portName = "out"
				}
				// 미선언 출력 포트 emit 게이트:
				// (1) 연결된 와이어가 없고 (2) 에러 포트가 아니며 (3) 노드가 해당
				// 포트를 출력 포트로 선언하지 않았다면, 결과를 조용히 폐기한다.
				// emit·tap 통지·포트 카운트(pc.Record())·미연결 경고를 모두 생략하기
				// 위해 debugPortLog/notifyOutputObserver/pc 로직 이전에 게이트한다.
				// 사용자가 에디터에서 출력 포트를 삭제한 경우(def.Outputs 에서 빠짐)의
				// 노이즈(폐기 경고/유령 카운트)를 제거한다. 와이어가 있는 포트(조건 1)와
				// 선언된 포트(조건 3)는 게이트되지 않아 하위호환을 보장한다.
				if len(targetWires) == 0 && portName != "error" &&
					!nodeDeclaresOutputPort(n, portName) {
					continue
				}
				debugPortLog(ctx, nodeLogger, "output", n.ID(), result)
				// 노드 출력 관측(tap): 와이어 연결 여부와 무관하게 방출 시점에 통지한다.
				e.notifyOutputObserver(flowID, n.ID(), portName, result)
				var pc *portCounter
				if nc := rt.nodeCounters[n.ID()]; nc != nil {
					if pc = nc.portCounters[portName]; pc != nil {
						pc.Record() // emitted (생산)
					}
				}
				// 연결된 와이어가 없으면 경고(포트당 1회) 후 폐기, 있으면 전달 후 delivered 기록.
				if len(targetWires) == 0 {
					e.warnUnconnectedPort(n.ID(), n.Name(), portName, pc)
				} else if e.sendToWires(ctx, result, targetWires, n.ID()) > 0 && pc != nil {
					pc.RecordDelivered() // delivered (실제 전달)
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

// resolveFlowComponentPrefix 는 플로우의 컴포넌트 이름 접두사를 결정한다.
// 플로우 이름을 sanitize한 결과를 사용하며, 비어 있으면 ID를 사용한다.
// 둘 다 비어 있으면 "unnamed"을 반환한다.
func resolveFlowComponentPrefix(f flow.Flow) string {
	flowName := sanitizeFlowName(f.Name())
	if flowName == "" {
		flowName = sanitizeFlowName(f.ID())
	}
	if flowName == "" {
		flowName = "unnamed"
	}
	return flowName
}

// sanitizeFlowNameRe 는 영숫자와 하이픈 이외의 문자를 매칭하는 정규식이다.
var sanitizeFlowNameRe = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeFlowNameMultiHyphen 는 연속 하이픈을 매칭하는 정규식이다.
var sanitizeFlowNameMultiHyphen = regexp.MustCompile(`-{2,}`)

// sanitizeFlowName 은 플로우 이름을 컴포넌트 이름에 안전한 형식으로 변환한다.
// 소문자 변환, 영숫자와 하이픈만 허용, 연속 하이픈 축소, 앞뒤 하이픈 제거.
func sanitizeFlowName(name string) string {
	if name == "" {
		return ""
	}
	// 소문자 변환
	s := strings.ToLower(name)
	// 영숫자와 하이픈 이외의 문자를 하이픈으로 치환
	s = sanitizeFlowNameRe.ReplaceAllString(s, "-")
	// 연속 하이픈을 단일 하이픈으로 축소
	s = sanitizeFlowNameMultiHyphen.ReplaceAllString(s, "-")
	// 앞뒤 하이픈 제거
	s = strings.Trim(s, "-")
	return s
}

// resolveNodeLogLevel 은 노드의 로그 레벨을 계층적으로 결정한다.
// 우선순위: 노드 config["log_level"] → 플로우 config.log_level → 데몬 기본값
// 명시적 설정이 있으면 (level, true)를, 기본값을 사용하면 (_, false)를 반환한다.
func resolveNodeLogLevel(nd flow.NodeDef, flowCfg flow.FlowConfig, daemonDefault slog.Level) (slog.Level, bool) {
	// 1. 노드 config["log_level"] 확인
	if nd.Config != nil {
		if lvlStr, ok := nd.Config["log_level"].(string); ok && lvlStr != "" {
			if lvl, err := observe.ParseLogLevel(lvlStr); err == nil {
				return lvl, true
			}
		}
	}

	// 2. 플로우 config.log_level 확인
	if flowCfg.LogLevel != "" {
		if lvl, err := observe.ParseLogLevel(flowCfg.LogLevel); err == nil {
			return lvl, true
		}
	}

	// 3. 데몬 기본값 사용 (LevelManager 기본값이 이미 적용됨)
	return daemonDefault, false
}
