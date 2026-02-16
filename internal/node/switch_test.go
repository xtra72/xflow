package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- SwitchNode 인터페이스 준수 ---

var _ Node = (*SwitchNode)(nil)

// --- NewSwitchNode 테스트 ---

// TestNewSwitchNode_정상생성 은 SwitchNode가 올바르게 생성되는지 확인한다.
func TestNewSwitchNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("switch-1", "switch")
	node, err := NewSwitchNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "switch-1", node.Name())
	assert.Equal(t, "switch", node.Type())
}

// --- Init 테스트 ---

// TestSwitchNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestSwitchNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("switch-init", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	err := sn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, sn.CurrentState())
}

// --- Process 테스트 ---

// TestSwitchNode_Process_라우트없음_드롭 은 라우트가 없고 기본 포트도 없으면 드롭하는지 확인한다.
func TestSwitchNode_Process_라우트없음_드롭(t *testing.T) {
	def := flow.NewNodeDef("switch-empty", "switch")
	node, _ := NewSwitchNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSwitchNode_Process_첫번째라우트매칭 은 첫 번째 매칭 라우트의 포트로 라우팅하는지 확인한다.
func TestSwitchNode_Process_첫번째라우트매칭(t *testing.T) {
	def := flow.NewNodeDef("switch-match", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-a",
		},
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-b",
		},
	}

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "port-a", target) // 첫 번째 매칭
}

// TestSwitchNode_Process_두번째라우트매칭 은 첫 번째가 실패하고 두 번째가 매칭되는 경우를 확인한다.
func TestSwitchNode_Process_두번째라우트매칭(t *testing.T) {
	def := flow.NewNodeDef("switch-second", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return false },
			TargetPort: "port-a",
		},
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-b",
		},
	}

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "port-b", target)
}

// TestSwitchNode_Process_기본포트사용 은 모든 라우트 실패 시 기본 포트를 사용하는지 확인한다.
func TestSwitchNode_Process_기본포트사용(t *testing.T) {
	def := flow.NewNodeDef("switch-default", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return false },
			TargetPort: "port-a",
		},
	}
	sn.defaultPort = "fallback"

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "fallback", target)
}

// TestSwitchNode_Process_메타데이터기반라우팅 은 메시지 메타데이터로 라우팅할 수 있는지 확인한다.
func TestSwitchNode_Process_메타데이터기반라우팅(t *testing.T) {
	def := flow.NewNodeDef("switch-meta", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition: func(msg message.Message) bool {
				v, ok := msg.Metadata().Get("priority")
				return ok && v == "high"
			},
			TargetPort: "urgent",
		},
		{
			Condition: func(msg message.Message) bool {
				v, ok := msg.Metadata().Get("priority")
				return ok && v == "low"
			},
			TargetPort: "normal",
		},
	}

	tests := []struct {
		name       string
		priority   string
		targetPort string
	}{
		{"높은 우선순위", "high", "urgent"},
		{"낮은 우선순위", "low", "normal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithMetadata("priority", tt.priority))
			results, err := sn.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			target, ok := results[0].Metadata().Get("_target_port")
			assert.True(t, ok)
			assert.Equal(t, tt.targetPort, target)
		})
	}
}

// TestSwitchNode_Process_복제된메시지반환 은 반환된 메시지가 원본과 다른 인스턴스인지 확인한다.
func TestSwitchNode_Process_복제된메시지반환(t *testing.T) {
	def := flow.NewNodeDef("switch-clone", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "out",
		},
	}

	msg := message.New()
	results, _ := sn.Process(context.Background(), msg)
	require.Len(t, results, 1)

	// Clone이므로 ID가 달라야 한다
	assert.NotEqual(t, msg.ID(), results[0].ID())
}

// --- Configure 테스트 ---

// TestSwitchNode_Configure_라우트설정 은 Configure로 라우트를 설정할 수 있는지 확인한다.
func TestSwitchNode_Configure_라우트설정(t *testing.T) {
	def := flow.NewNodeDef("switch-cfg", "switch")
	node, _ := NewSwitchNode(def)

	routes := []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "configured-port",
		},
	}
	err := node.Configure(map[string]any{
		"routes":       routes,
		"default_port": "default-out",
	})
	require.NoError(t, err)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "configured-port", target)
}

// --- Shutdown 테스트 ---

// TestSwitchNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestSwitchNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("switch-shut", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	_ = sn.Init(context.Background())
	err := sn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, sn.CurrentState())
}

// --- 동시성 테스트 ---

// TestSwitchNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestSwitchNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("switch-conc", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "out",
		},
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New()
			_, _ = sn.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
