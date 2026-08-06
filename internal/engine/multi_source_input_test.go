package engine

// SPEC-HVACR-SYNC-001 M10: 엔진 타입 게이트 확장 검증.
//
// 기존 엔진은 MultiSourceNode.ExtraSourceChannels 를 입력 와이어가 없을 때(순수 소스
// 경로, len(inputWires)==0)만 배출했다. M10 결합 노드(mirror-message)는 mirror-in 입력
// 와이어를 가지므로 fan-in Process 경로가 선택되어 ExtraSourceChannels(mirror-out)가
// 배출되지 않았다. 본 테스트는 "입력 와이어가 있어도 ExtraSourceChannels 를 배출"하는
// 확장을 검증하고(신규), 순수 소스 경로의 기존 배출 동작이 보존됨도 함께 확인한다.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// multiSourceInputNode 는 SourceNode 를 임베딩한 MultiSourceNode 이면서, 입력 와이어를
// 통해 fan-in Process 경로로도 동작할 수 있는 최소 테스트 노드이다("extra" 추가 소스 포트).
type multiSourceInputNode struct {
	id, name string
	extraCh  chan message.Message
	srcCh    chan message.Message
}

func newMultiSourceInputNode(id string) *multiSourceInputNode {
	return &multiSourceInputNode{
		id:      id,
		name:    id,
		extraCh: make(chan message.Message, 4),
		srcCh:   make(chan message.Message), // fan-in 경로에서는 배출되지 않음(미사용)
	}
}

func (n *multiSourceInputNode) ID() string                     { return n.id }
func (n *multiSourceInputNode) Name() string                   { return n.name }
func (n *multiSourceInputNode) Type() string                   { return "multisrcinput" }
func (n *multiSourceInputNode) Init(context.Context) error     { return nil }
func (n *multiSourceInputNode) Shutdown(context.Context) error { return nil }
func (n *multiSourceInputNode) Configure(map[string]any) error { return nil }
func (n *multiSourceInputNode) Process(_ context.Context, _ message.Message) ([]message.Message, error) {
	return nil, nil // 입력 소비만 (하류 emit 없음)
}
func (n *multiSourceInputNode) SourceCh() <-chan message.Message { return n.srcCh }
func (n *multiSourceInputNode) ExtraSourceChannels() map[string]<-chan message.Message {
	return map[string]<-chan message.Message{"extra": n.extraCh}
}
func (n *multiSourceInputNode) Ports() []node.NodePort {
	return []node.NodePort{
		{ID: "in", Name: "in", Direction: flow.PortInput},
		{ID: "extra", Name: "extra", Direction: flow.PortOutput},
	}
}

// TestMultiSourceNodeWithInputWiresEmitsExtraChannels 는 입력 와이어를 가진(fan-in Process
// 경로) MultiSourceNode 의 ExtraSourceChannels 가 하류로 배출됨을 검증한다(M10 엔진 확장).
func TestMultiSourceNodeWithInputWiresEmitsExtraChannels(t *testing.T) {
	src := newTestSourceNode("SRC", "SRC") // msn 에 입력 와이어를 부여(fan-in 경로 강제)
	msn := newMultiSourceInputNode("MSN")
	gate := newGateProcNode("GATE", "GATE", true)

	srcDef := flow.NewNodeDef("SRC", "testsource")
	msnDef := flow.NewNodeDef("MSN", "multisrcinput", flow.WithOutputPorts(
		flow.Port{Name: "out", Direction: flow.PortOutput},
		flow.Port{Name: "extra", Direction: flow.PortOutput},
	))
	gateDef := flow.NewNodeDef("GATE", "gateproc")
	src.id = srcDef.ID
	msn.id = msnDef.ID
	msn.name = msnDef.ID
	gate.id = gateDef.ID

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("testsource", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return src, nil
	}))
	require.NoError(t, registry.Register("multisrcinput", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return msn, nil
	}))
	require.NoError(t, registry.Register("gateproc", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return gate, nil
	}))

	f := flow.NewFlow("multisrc-input-flow",
		flow.WithNodes(srcDef, msnDef, gateDef),
		flow.WithWires(
			flow.NewWire(srcDef.ID, "out", msnDef.ID, "in"),    // msn 에 입력 와이어 부여
			flow.NewWire(msnDef.ID, "extra", gateDef.ID, "in"), // extra 소스 포트 → 하류
		),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	// msn 은 입력 와이어를 가지지만(fan-in Process 경로), ExtraSourceChannels 도 배출되어야 한다.
	msn.extraCh <- message.New()

	waitProcessed(t, gate)
	require.GreaterOrEqual(t, gate.processedCount(), 1,
		"입력 와이어가 있는 MultiSourceNode 의 ExtraSourceChannels 도 하류로 전달되어야 한다")
}

// TestMultiSourceNodeWithoutInputWiresStillEmitsExtraChannels 는 입력 와이어가 없는
// 순수 소스 경로에서도 ExtraSourceChannels 배출이 보존됨을 검증한다(행위 보존).
func TestMultiSourceNodeWithoutInputWiresStillEmitsExtraChannels(t *testing.T) {
	msn := newMultiSourceInputNode("MSN")
	gate := newGateProcNode("GATE", "GATE", true)

	msnDef := flow.NewNodeDef("MSN", "multisrcinput", flow.WithOutputPorts(
		flow.Port{Name: "out", Direction: flow.PortOutput},
		flow.Port{Name: "extra", Direction: flow.PortOutput},
	))
	gateDef := flow.NewNodeDef("GATE", "gateproc")
	msn.id = msnDef.ID
	msn.name = msnDef.ID
	gate.id = gateDef.ID

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("multisrcinput", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return msn, nil
	}))
	require.NoError(t, registry.Register("gateproc", func(_ flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
		return gate, nil
	}))

	// msn 에 입력 와이어를 붙이지 않음 → 순수 소스 경로(len(inputWires)==0).
	f := flow.NewFlow("multisrc-noinput-flow",
		flow.WithNodes(msnDef, gateDef),
		flow.WithWires(
			flow.NewWire(msnDef.ID, "extra", gateDef.ID, "in"),
		),
	)

	e := NewEngine(WithNodeRegistry(registry), WithShutdownTimeout(2*time.Second))
	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))
	defer func() { _ = e.StopFlow(ctx, f.ID()) }()

	msn.extraCh <- message.New()

	waitProcessed(t, gate)
	require.GreaterOrEqual(t, gate.processedCount(), 1,
		"순수 소스 경로의 ExtraSourceChannels 배출은 보존되어야 한다(행위 보존)")
}
