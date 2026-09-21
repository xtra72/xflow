// 미등록 타입 에러가 타입 이름을 싣는다 (@SPEC:SPEC-FLOW-NODETYPE-001).
//
// 엔진의 사전 검사가 대부분을 먼저 잡지만, 레지스트리를 직접 쓰는 다른 호출자
// (노드 어댑터 등)도 같은 질문에 답할 수 있어야 한다 — "어느 타입이 없는가".
package node

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

func TestRegistryCreate_UnknownTypeNamesTheType(t *testing.T) {
	r := NewRegistry()

	_, err := r.Create(flow.NodeDef{ID: "n1", Name: "Temperature", Type: "sensor-temperature"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNodeTypeNotFound, "식별은 그대로여야 한다(API 404 매핑)")
	require.Contains(t, err.Error(), "sensor-temperature",
		"없는 타입 이름이 없으면 운영자는 플로우 JSON 을 열어야 한다")
}
