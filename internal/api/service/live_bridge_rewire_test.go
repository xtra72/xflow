// live_bridge_rewire_test.go 는 rewireLiveBridges 의 종류별 opener 매핑(remote vs local-shared)을
// 검증한다(@SPEC:SPEC-SUBFLOW-002 SH04/IN03/MG03, REQ-SUBFLOW2-SH01).
package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// TestRewireLiveBridges_로컬shared_로컬opener바인딩 은 살아남은 로컬 shared flow-node 가
// 로컬 opener 로 컨트롤러에 바인딩되고 fwd+emit 로 재배선되는지 검증한다(SH01/SH04).
func TestRewireLiveBridges_로컬shared_로컬opener바인딩(t *testing.T) {
	nodes := []flow.NodeDef{
		{ID: "up", Name: "up", Type: "transform"},
		{ID: "sfn", Name: "sfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "flow-local", flowNodeModeKey: flowModeShared},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}},
		},
		{ID: "down", Name: "down", Type: "transform"},
	}
	wires := []flow.Wire{
		{ID: "w1", SourceNodeID: "up", SourcePort: "out", TargetNodeID: "sfn", TargetPort: "in1"},
		{ID: "w2", SourceNodeID: "sfn", SourcePort: "out1", TargetNodeID: "down", TargetPort: "in"},
	}
	f := flow.RebuildFlow(flow.NewFlow("f1"), nodes, wires)

	localOpener := &fakeBridgeOpener{}
	tbl := newManagerBridgeTable()
	rewired, ctrls, err := rewireLiveBridges(f, nil /*remote*/, localOpener, tbl, nil)
	require.NoError(t, err)
	require.Len(t, ctrls, 1, "로컬 shared flow-node 1개당 컨트롤러 1개")

	// 컨트롤러가 로컬 참조 flow_id 로 구성되어야 한다(instanceID="" — 직교).
	assert.Equal(t, "", ctrls[0].instanceID)
	assert.Equal(t, "flow-local", ctrls[0].remoteFlowID)

	// flow-node 가 fwd+emit 로 치환되어야 한다.
	for _, n := range rewired.Nodes() {
		assert.NotEqual(t, "sfn", n.ID, "shared flow-node 는 fwd+emit 로 치환되어야 함")
	}
}

// TestRewireLiveBridges_로컬shared_opener미주입_거부 은 로컬 opener 가 없으면 명확히 거부되는지
// 검증한다(shared 미지원 모드).
func TestRewireLiveBridges_로컬shared_opener미주입_거부(t *testing.T) {
	nodes := []flow.NodeDef{
		{ID: "sfn", Name: "sfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "flow-local", flowNodeModeKey: flowModeShared},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}},
		},
	}
	f := flow.RebuildFlow(flow.NewFlow("f1"), nodes, nil)
	tbl := newManagerBridgeTable()

	_, _, err := rewireLiveBridges(f, nil, nil, tbl, nil)
	assert.ErrorIs(t, err, ErrSharedBridgeUnavailable)
}

// TestRewireLiveBridges_혼합_remote와로컬shared 는 한 플로우의 remote 와 로컬 shared flow-node 가
// 각각 자신의 opener 로 바인딩되는지 검증한다(IN03 직교 — 두 종류 공존).
func TestRewireLiveBridges_혼합_remote와로컬shared(t *testing.T) {
	nodes := []flow.NodeDef{
		{ID: "rfn", Name: "rfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "remote://node-1/flow-r"},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}},
		},
		{ID: "sfn", Name: "sfn", Type: flowNodeType,
			Config:  map[string]any{flowNodeFlowIDKey: "flow-local", flowNodeModeKey: flowModeShared},
			Inputs:  []flow.Port{{ID: "in1", Name: "in1", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out1", Name: "out1", Direction: flow.PortOutput}},
		},
	}
	f := flow.RebuildFlow(flow.NewFlow("f1"), nodes, nil)

	remoteOpener := &fakeBridgeOpener{}
	localOpener := &fakeBridgeOpener{}
	tbl := newManagerBridgeTable()
	_, ctrls, err := rewireLiveBridges(f, remoteOpener, localOpener, tbl, nil)
	require.NoError(t, err)
	require.Len(t, ctrls, 2, "remote+local-shared 각각 컨트롤러 1개")

	// 종류별로 instanceID 구분(remote 는 node-1, local 은 "").
	var sawRemote, sawLocal bool
	for _, c := range ctrls {
		switch c.instanceID {
		case "node-1":
			sawRemote = true
			assert.Equal(t, "flow-r", c.remoteFlowID)
		case "":
			sawLocal = true
			assert.Equal(t, "flow-local", c.remoteFlowID)
		}
	}
	assert.True(t, sawRemote, "remote flow-node 컨트롤러 존재")
	assert.True(t, sawLocal, "local shared flow-node 컨트롤러 존재")
}
