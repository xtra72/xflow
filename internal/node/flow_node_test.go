package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xtra/xflow/pkg/flow"
)

// TestFlowNode_레지스트리_등록 은 flow-node 타입이 빌트인으로 등록되어
// 팔레트(AllTypeMeta)에 노출되는지 확인한다(SPEC-SUBFLOW-001 마일스톤 2 PART 1).
func TestFlowNode_레지스트리_등록(t *testing.T) {
	r := NewRegistry()

	assert.True(t, r.Has("flow-node"), "flow-node 타입이 등록되어야 한다")

	meta, ok := r.TypeMeta("flow-node")
	assert.True(t, ok)
	assert.Equal(t, "flow-node", meta.Type)
	assert.Equal(t, "composition", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
	assert.NotEmpty(t, meta.Description)
}

// TestFlowNode_팩토리_확장요구_에러 는 안전망 팩토리가 항상 "확장 필요" 에러를
// 반환하는지 확인한다(SPEC-SUBFLOW-001 결정 1 — 배포 시 서브그래프 확장).
// flow-node 는 정상 경로에서 확장으로 치환되므로 직접 인스턴스화는 에러여야 한다.
func TestFlowNode_팩토리_확장요구_에러(t *testing.T) {
	def := flow.NodeDef{
		ID:   "fn-1",
		Type: "flow-node",
		Config: map[string]any{
			"flow_id": "flow-abc",
		},
	}

	n, err := NewFlowNodePlaceholder(def)

	assert.Nil(t, n)
	assert.Error(t, err)
	// 에러 메시지에 참조 플로우 id 가 포함되어야 한다(진단 가독성).
	assert.Contains(t, err.Error(), "flow-abc")
	assert.Contains(t, err.Error(), "expanded")
}

// TestFlowNode_레지스트리_Create_에러전파 는 레지스트리 Create 경로에서도
// flow-node 팩토리의 에러가 그대로 전파되는지 확인한다.
func TestFlowNode_레지스트리_Create_에러전파(t *testing.T) {
	r := NewRegistry()

	def := flow.NodeDef{
		ID:     "fn-2",
		Type:   "flow-node",
		Config: map[string]any{"flow_id": "flow-xyz"},
	}

	n, err := r.Create(def)
	assert.Nil(t, n)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flow-xyz")
}
