// remote_bridge_e2e_test.go 는 P3 매니저 측 라이브 브리지의 end-to-end 배포 경로를 검증한다
// (@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB01/RB05/RB07).
//
// 시나리오: LOCAL 플로우가 remote:// flow-node 를 참조한다. 실제 엔진에 배포·시작하면 살아남은
// remote flow-node 가 입력 forwarder + 출력 emitter 로 재배선되어 라이브 브리지(fake opener)에
// 바인딩된다. upstream 노드가 emit 한 메시지는 bridge.SendInput 으로 흐르고(입력 전달), fake
// bridge 가 흘린 출력은 emitter 를 통해 downstream collector 로 emit 된다(출력 중계).
package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트용 source/sink 노드: e2e 입력 주입/출력 관측용(엔진 일반 노드).
// ---------------------------------------------------------------------------

// e2eEmitterNode 는 시작 시 1건을 out 포트로 emit 하는 SourceNode 이다(입력 주입 트리거).
type e2eEmitterNode struct {
	*node.BaseNode
	ch   chan message.Message
	once sync.Once
}

func newE2EEmitterNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	return &e2eEmitterNode{BaseNode: node.NewBaseNode(def, opts...), ch: make(chan message.Message, 1)}, nil
}
func (n *e2eEmitterNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	n.once.Do(func() {
		n.ch <- message.New(message.WithPayload(message.NewPayload(map[string]any{"value": 42})))
	})
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}
func (n *e2eEmitterNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}
func (n *e2eEmitterNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}
func (n *e2eEmitterNode) SourceCh() <-chan message.Message { return n.ch }

var _ node.SourceNode = (*e2eEmitterNode)(nil)

// e2eCollector 는 수신 메시지를 기록하는 ProcessNode 이다(출력 관측). 전역 테이블로 수집한다.
type e2eCollector struct {
	*node.BaseNode
	sink *e2eSink
}

type e2eSink struct {
	mu  sync.Mutex
	got []json.RawMessage
}

func (s *e2eSink) add(m message.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := m.Payload().ToJSON()
	s.got = append(s.got, data)
}
func (s *e2eSink) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.got) }

var e2eSinkTable = struct {
	mu sync.Mutex
	m  map[string]*e2eSink
}{m: make(map[string]*e2eSink)}

func newE2ECollectorNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &e2eCollector{BaseNode: node.NewBaseNode(def, opts...)}
	sinkID, _ := def.Config["__sink_id__"].(string)
	e2eSinkTable.mu.Lock()
	if s, ok := e2eSinkTable.m[sinkID]; ok {
		n.sink = s
	}
	e2eSinkTable.mu.Unlock()
	return n, nil
}
func (n *e2eCollector) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}
func (n *e2eCollector) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.sink != nil {
		n.sink.add(msg)
	}
	return nil, nil
}
func (n *e2eCollector) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// TestRemoteBridgeNode_E2EDeployAndBridge 는 remote:// flow-node 를 가진 LOCAL 플로우가
// 배포·시작되고 입력 전달 + 출력 중계가 동작하는지 end-to-end 로 검증한다(RB01/RB05/RB07).
func TestRemoteBridgeNode_E2EDeployAndBridge(t *testing.T) {
	reg := node.NewRegistry()
	require.NoError(t, RegisterRemoteBridgeNodes(reg))
	require.NoError(t, reg.Register("e2e-emitter", newE2EEmitterNode))
	require.NoError(t, reg.Register("e2e-collector", newE2ECollectorNode))
	eng := engine.NewEngine(engine.WithNodeRegistry(reg))
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	opener := &fakeBridgeOpener{}
	adapter.SetRemoteBridgeOpener(opener)

	sinkID := "sink-e2e-1"
	sink := &e2eSink{}
	e2eSinkTable.mu.Lock()
	e2eSinkTable.m[sinkID] = sink
	e2eSinkTable.mu.Unlock()

	// emitter → remoteFN:in1, remoteFN:out1 → collector.
	nodes := []flow.NodeDef{
		{ID: "emitter", Name: "emitter", Type: "e2e-emitter",
			Outputs: []flow.Port{{ID: "out", Name: "out", Direction: flow.PortOutput}}},
		{ID: "rfn", Name: "rfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "remote://node-1/flow-x"},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}}},
		{ID: "collector", Name: "collector", Type: "e2e-collector",
			Config: map[string]any{"__sink_id__": sinkID},
			Inputs: []flow.Port{{ID: "in", Name: "in", Direction: flow.PortInput}}},
	}
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "emitter", SourcePort: "out", TargetNodeID: "rfn", TargetPort: "in1"},
		{ID: "w2", SourceNodeID: "rfn", SourcePort: "out1", TargetNodeID: "collector", TargetPort: "in"},
	}
	f := flow.RebuildFlow(flow.NewFlow("e2e-flow"), nodes, wires)
	require.NoError(t, adapter.repo.Save(context.Background(), f))

	ctx := context.Background()
	require.NoError(t, adapter.StartFlow(ctx, f.ID()), "remote:// flow-node 를 가진 플로우가 배포·시작되어야 함")
	defer func() { _ = adapter.StopFlow(ctx, f.ID()) }()

	// 브리지가 열렸어야 한다(rfn → fwd Init → OpenBridge).
	require.Eventually(t, func() bool {
		_, _, b := opener.snapshot()
		return b != nil
	}, 2*time.Second, 10*time.Millisecond, "라이브 브리지가 열려야 함(OpenBridge)")
	instanceID, remoteFlowID, fb := opener.snapshot()
	assert.Equal(t, "node-1", instanceID)
	assert.Equal(t, "flow-x", remoteFlowID)

	// 입력 전달: emitter 가 emit 한 메시지가 bridge.SendInput 으로 흘렀어야 한다(WRITE — RB07).
	require.Eventually(t, func() bool {
		return len(fb.sent()) >= 1
	}, 2*time.Second, 10*time.Millisecond, "입력이 bridge.SendInput 으로 전달되어야 함")
	assert.Equal(t, "in1", fb.sent()[0].Port)

	// 출력 중계: fake bridge 가 흘린 출력이 emitter → collector 로 emit 되어야 한다(READ — RB07).
	fb.outputs <- BridgeOutput{Port: "out1", Data: json.RawMessage(`{"state":"on"}`)}
	require.Eventually(t, func() bool {
		return sink.count() >= 1
	}, 2*time.Second, 10*time.Millisecond, "출력이 downstream 으로 emit 되어야 함")
}

// TestRemoteBridgeNode_NonServerModeRejected 는 opener 미주입(비-server 모드)에서 remote://
// flow-node 배포가 명확한 오류로 거부되는지 검증한다(RB 미지원 모드).
func TestRemoteBridgeNode_NonServerModeRejected(t *testing.T) {
	reg := node.NewRegistry()
	require.NoError(t, RegisterRemoteBridgeNodes(reg))
	eng := engine.NewEngine(engine.WithNodeRegistry(reg))
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	// opener 미주입.

	nodes := []flow.NodeDef{
		{ID: "rfn", Name: "rfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "remote://node-1/flow-x"},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}}},
	}
	f := flow.RebuildFlow(flow.NewFlow("nonserver-flow"), nodes, nil)
	require.NoError(t, adapter.repo.Save(context.Background(), f))

	err := adapter.DeployFlow(context.Background(), f.ID())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRemoteBridgeUnavailable)
}
