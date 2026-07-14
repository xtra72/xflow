package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// mockRxAgent 는 agent.Agent + agent.MessageReceiver 를 구현하며 최초 1회 data 를 반환한
// 뒤 블록한다(tcp-in 이 붙는 수신 에이전트 모사).
type mockRxAgent struct {
	once sync.Once
	data []byte
}

func (m *mockRxAgent) Init(agent.AgentConfig) error      { return nil }
func (m *mockRxAgent) Start(context.Context) error       { return nil }
func (m *mockRxAgent) Stop(context.Context) error        { return nil }
func (m *mockRxAgent) Pause(context.Context) error       { return nil }
func (m *mockRxAgent) Resume(context.Context) error      { return nil }
func (m *mockRxAgent) Health() agent.HealthStatus        { return agent.HealthStatus{} }
func (m *mockRxAgent) Process([]byte) ([]byte, error)    { return nil, nil }
func (m *mockRxAgent) Configure(agent.AgentConfig) error { return nil }
func (m *mockRxAgent) ID() string                        { return "mock-rx" }
func (m *mockRxAgent) Name() string                      { return "mock-rx" }
func (m *mockRxAgent) Type() string                      { return "tcp-server" }
func (m *mockRxAgent) Info() agent.AgentInfo             { return agent.AgentInfo{} }
func (m *mockRxAgent) Stats() agent.StatsSnapshot        { return agent.StatsSnapshot{} }
func (m *mockRxAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	first := false
	m.once.Do(func() { first = true })
	if first {
		return m.data, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

// rxTransport 는 AgentTransport + AgentAccessor 이며 UnderlyingAgent 로 mockRxAgent 를 노출한다.
type rxTransport struct{ ag agent.Agent }

func (t *rxTransport) Send(context.Context, message.Message) error { return nil }
func (t *rxTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (t *rxTransport) UnderlyingAgent() agent.Agent { return t.ag }

type rxResolver struct{ t node.AgentTransport }

func (r rxResolver) ResolveAgent(context.Context, flow.AgentRef) (node.AgentTransport, error) {
	return r.t, nil
}

// stubResolver 는 항상 "agent not available" 를 반환하는 AgentResolver 이다. serial-out
// 노드가 에이전트 없이도 defer 후 Running 으로 진행해 입력을 소비(passthrough)하도록 한다.
type stubResolver struct{}

func (stubResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (node.AgentTransport, error) {
	return nil, errors.New("stub: agent not available")
}

// testSourceNode 는 SourceCh 로 메시지를 방출하는 최소 SourceNode 이다. tcp-in / serial-in
// 등 실제 SourceNode 가 엔진 source 분기를 통해 하류 process 노드로 라우팅되는 경로를
// 대표한다(실제 tcp/serial 에이전트 없이 라우팅만 검증).
type testSourceNode struct {
	id, name string
	ch       chan message.Message
}

func newTestSourceNode(id, name string) *testSourceNode {
	return &testSourceNode{id: id, name: name, ch: make(chan message.Message, 4)}
}

func (s *testSourceNode) ID() string                     { return s.id }
func (s *testSourceNode) Name() string                   { return s.name }
func (s *testSourceNode) Type() string                   { return "testsource" }
func (s *testSourceNode) Init(context.Context) error     { return nil }
func (s *testSourceNode) Shutdown(context.Context) error { return nil }
func (s *testSourceNode) Configure(map[string]any) error { return nil }
func (s *testSourceNode) Process(_ context.Context, m message.Message) ([]message.Message, error) {
	return []message.Message{m}, nil
}
func (s *testSourceNode) SourceCh() <-chan message.Message { return s.ch }
func (s *testSourceNode) Ports() []node.NodePort {
	return []node.NodePort{{ID: "out", Name: "out", Direction: flow.PortOutput}}
}

// TestSourceNodeDeliversToProcessNode 는 실제 SourceNode 가 SourceCh 로 방출한 메시지가
// 엔진의 source 분기를 통해 하류 process 노드로 전달되는지 검증한다.
// (tcp-in → serial-out 형태의 "source → process" 노드 연결 라우팅 회귀 방지.)
func TestSourceNodeDeliversToProcessNode(t *testing.T) {
	src := newTestSourceNode("SRC", "SRC")
	gate := newGateProcNode("GATE", "GATE", true) // 하류 process 노드(입력 "in" 선언)

	srcDef := flow.NewNodeDef("SRC", "testsource")
	gateDef := flow.NewNodeDef("GATE", "gateproc")
	src.id = srcDef.ID
	gate.id = gateDef.ID

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("testsource", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return src, nil
	}))
	require.NoError(t, registry.Register("gateproc", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return gate, nil
	}))

	f := flow.NewFlow("src-proc-flow",
		flow.WithNodes(srcDef, gateDef),
		flow.WithWires(flow.NewWire(srcDef.ID, "out", gateDef.ID, "in")),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	// SourceNode 가 SourceCh 로 메시지를 방출한다(tcp-in 수신 이벤트 모사).
	src.ch <- message.New()

	waitProcessed(t, gate)
	require.GreaterOrEqual(t, gate.processedCount(), 1,
		"SourceNode 가 SourceCh 로 방출한 메시지는 하류 process 노드로 전달되어야 한다")
}

// TestSourceNodeDeliversToRealSerialOutNode 는 SourceNode 방출 메시지가 실제
// SerialOutNode 로 전달되어 Process(입력 "in" 카운터)가 호출되는지 검증한다.
// (사용자 보고: tcp-in "43 out" 인데 serial-out "in 0" — 실제 노드 타입 재현.)
func TestSourceNodeDeliversToRealSerialOutNode(t *testing.T) {
	src := newTestSourceNode("SRC", "SRC")
	srcDef := flow.NewNodeDef("SRC", "testsource")
	src.id = srcDef.ID

	outDef := flow.NewNodeDef("OUT", "serial-out",
		flow.WithAgentRef(flow.AgentRef{AgentName: "serial-x"}),
		flow.WithNodeConfig("agent_ref", "serial-x"))

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("testsource", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return src, nil
	}))
	require.NoError(t, registry.Register("serial-out", func(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
		return node.NewSerialOutNode(def, append(opts, node.WithAgentResolver(stubResolver{}))...)
	}))

	f := flow.NewFlow("src-serialout-flow",
		flow.WithNodes(srcDef, outDef),
		flow.WithWires(flow.NewWire(srcDef.ID, "out", outDef.ID, "in")),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	m := message.New()
	m.Payload().Set("raw", []byte{0x01, 0x02, 0x03})
	src.ch <- m

	// serial-out 의 "in" 포트 카운터가 증가할 때까지 대기(= Process 호출됨).
	deadline := time.After(2 * time.Second)
	for {
		e.mu.RLock()
		rt := e.flows[f.ID()]
		e.mu.RUnlock()
		var inCount int64
		if rt != nil {
			if nc := rt.nodeCounters[outDef.ID]; nc != nil {
				if pc := nc.portCounters["in"]; pc != nil {
					inCount = pc.messages.Load()
				}
			}
		}
		if inCount > 0 {
			return // 전달됨 — 정상
		}
		select {
		case <-deadline:
			t.Fatal("serial-out 이 입력을 소비하지 않음 (in 카운터 0) — tcp-in→serial-out 미전달 재현")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRealTCPInNodeDeliversToProcessNode 는 실제 TCPInNode 가 수신한 데이터를 SourceCh 로
// 방출하고, 엔진 source 분기를 통해 하류 process 노드로 전달하는지 검증한다.
// (사용자 보고 재현: tcp-in "43 out" 인데 하류 노드 "in 0".)
func TestRealTCPInNodeDeliversToProcessNode(t *testing.T) {
	ag := &mockRxAgent{data: []byte{0xAA, 0xBB, 0xCC}}
	resolver := rxResolver{t: &rxTransport{ag: ag}}

	inDef := flow.NewNodeDef("IN", "tcp-in",
		flow.WithAgentRef(flow.AgentRef{AgentName: "tcp-x"}),
		flow.WithNodeConfig("agent_ref", "tcp-x"))
	gate := newGateProcNode("GATE", "GATE", true)
	gateDef := flow.NewNodeDef("GATE", "gateproc")
	gate.id = gateDef.ID

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("tcp-in", func(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
		return node.NewTCPInNode(def, append(opts, node.WithAgentResolver(resolver))...)
	}))
	require.NoError(t, registry.Register("gateproc", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return gate, nil
	}))

	f := flow.NewFlow("tcpin-proc-flow",
		flow.WithNodes(inDef, gateDef),
		flow.WithWires(flow.NewWire(inDef.ID, "out", gateDef.ID, "in")),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	waitProcessed(t, gate)
	require.GreaterOrEqual(t, gate.processedCount(), 1,
		"tcp-in 이 수신한 데이터는 하류 process 노드로 전달되어야 한다")
}

// TestDisabledProcessNodeDropsInput 는 하류 process 노드가 비활성화(Enabled=false)되면
// 엔진이 입력을 드레인·폐기하여 Process("in" 카운터)가 호출되지 않음을 확인한다.
// 이것이 사용자 보고 증상(source out>0, target in=0)의 유력 원인이다.
func TestDisabledProcessNodeDropsInput(t *testing.T) {
	src := newTestSourceNode("SRC", "SRC")
	gate := newGateProcNode("GATE", "GATE", true)

	srcDef := flow.NewNodeDef("SRC", "testsource")
	gateDef := flow.NewNodeDef("GATE", "gateproc", flow.WithEnabled(false)) // 비활성화
	src.id = srcDef.ID
	gate.id = gateDef.ID

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("testsource", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return src, nil
	}))
	require.NoError(t, registry.Register("gateproc", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return gate, nil
	}))

	f := flow.NewFlow("disabled-target-flow",
		flow.WithNodes(srcDef, gateDef),
		flow.WithWires(flow.NewWire(srcDef.ID, "out", gateDef.ID, "in")),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	src.ch <- message.New()
	time.Sleep(200 * time.Millisecond)

	// 비활성화 노드는 Process 가 호출되지 않는다(입력 드레인·폐기) → in 카운터 0.
	require.Equal(t, 0, gate.processedCount(),
		"비활성화된 하류 노드는 입력을 처리하지 않아야 한다 (사용자 증상: target in=0 재현)")
}
