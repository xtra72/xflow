package node

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- StatusNode 인터페이스 준수 ---

var _ Node = (*StatusNode)(nil)

// --- NewStatusNode 테스트 ---

// TestNewStatusNode_정상생성 은 StatusNode가 올바르게 생성되는지 확인한다.
func TestNewStatusNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("status-1", "status")
	node, err := NewStatusNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "status-1", node.Name())
	assert.Equal(t, "status", node.Type())
}

// --- Init 테스트 ---

// TestStatusNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestStatusNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("status-init", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	err := sn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, sn.CurrentState())
}

// --- Process 테스트 ---

// TestStatusNode_Process_watchNodes없음_모든이벤트통과 는 watchNodes가 없으면 모든 이벤트를 통과시키는지 확인한다.
func TestStatusNode_Process_watchNodes없음_모든이벤트통과(t *testing.T) {
	def := flow.NewNodeDef("status-all", "status")
	node, _ := NewStatusNode(def)

	msg := message.New(
		message.WithMetadata("_event_type", "state_change"),
		message.WithMetadata("_node_id", "node-123"),
		message.WithMetadata("_prev_state", "initializing"),
		message.WithMetadata("_new_state", "running"),
		message.WithMetadata("_timestamp", "2026-01-01T00:00:00Z"),
	)

	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestStatusNode_Process_watchNodes매칭_통과 는 watchNodes에 포함된 node_id 이벤트를 통과시키는지 확인한다.
func TestStatusNode_Process_watchNodes매칭_통과(t *testing.T) {
	def := flow.NewNodeDef("status-match", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	sn.watchNodes = []string{"node-A", "node-B"}

	msg := message.New(
		message.WithMetadata("_event_type", "state_change"),
		message.WithMetadata("_node_id", "node-A"),
		message.WithMetadata("_prev_state", "running"),
		message.WithMetadata("_new_state", "stopping"),
	)

	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestStatusNode_Process_watchNodes비매칭_필터링 은 watchNodes에 포함되지 않은 node_id 이벤트를 필터링하는지 확인한다.
func TestStatusNode_Process_watchNodes비매칭_필터링(t *testing.T) {
	def := flow.NewNodeDef("status-nomatch", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	sn.watchNodes = []string{"node-A", "node-B"}

	msg := message.New(
		message.WithMetadata("_event_type", "state_change"),
		message.WithMetadata("_node_id", "node-C"),
	)

	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestStatusNode_Process_nodeID없음_필터링 은 _node_id가 없는 메시지를 필터링하는지 확인한다.
func TestStatusNode_Process_nodeID없음_통과(t *testing.T) {
	def := flow.NewNodeDef("status-no-nodeid", "status")
	node, _ := NewStatusNode(def)

	// watchNodes가 비어있으면 모든 메시지 통과
	msg := message.New(
		message.WithMetadata("_event_type", "state_change"),
	)

	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestStatusNode_Process_다양한노드ID 는 테이블 드리븐 테스트로 다양한 노드 ID를 검증한다.
func TestStatusNode_Process_다양한노드ID(t *testing.T) {
	def := flow.NewNodeDef("status-table", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	sn.watchNodes = []string{"filter-1", "transform-1"}

	tests := []struct {
		name     string
		nodeID   string
		expected int // 결과 메시지 수
	}{
		{"감시 대상 filter-1", "filter-1", 1},
		{"감시 대상 transform-1", "transform-1", 1},
		{"감시 비대상 switch-1", "switch-1", 0},
		{"감시 비대상 bridge-1", "bridge-1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(
				message.WithMetadata("_event_type", "state_change"),
				message.WithMetadata("_node_id", tt.nodeID),
			)
			results, err := sn.Process(context.Background(), msg)
			require.NoError(t, err)
			assert.Len(t, results, tt.expected)
		})
	}
}

// --- Configure 테스트 ---

// TestStatusNode_Configure_watchNodes설정 은 Configure로 watchNodes를 설정할 수 있는지 확인한다.
func TestStatusNode_Configure_watchNodes설정(t *testing.T) {
	def := flow.NewNodeDef("status-cfg", "status")
	node, _ := NewStatusNode(def)

	err := node.Configure(map[string]any{
		"watch_nodes": []string{"node-1", "node-2"},
	})
	require.NoError(t, err)

	sn := node.(*StatusNode)
	assert.Equal(t, []string{"node-1", "node-2"}, sn.watchNodes)
}

// TestStatusNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestStatusNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("status-cfg-nil", "status")
	node, _ := NewStatusNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestStatusNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestStatusNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("status-shut", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	_ = sn.Init(context.Background())
	err := sn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, sn.CurrentState())
}

// --- 동시성 테스트 ---

// TestStatusNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestStatusNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("status-conc", "status")
	node, _ := NewStatusNode(def)
	sn := node.(*StatusNode)

	sn.watchNodes = []string{"node-A"}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New(
				message.WithMetadata("_node_id", "node-A"),
			)
			_, _ = sn.Process(context.Background(), msg)
		}()
	}
	wg.Wait()
}
