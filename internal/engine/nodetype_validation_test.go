// 등록되지 않은 노드 타입의 보고 방식 (@SPEC:SPEC-FLOW-NODETYPE-001).
//
// 신고(2026-09-21): `failed to create node "Temperature": node: type not found`.
// 노드 **이름**은 있는데 없는 **타입**이 없어, 운영자는 플로우 JSON 을 열어야 했다.
package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

/** 주어진 (이름, 타입) 노드들로 플로우를 만든다. */
func flowWithNodes(t *testing.T, pairs ...[2]string) flow.Flow {
	t.Helper()
	nodes := make([]flow.NodeDef, 0, len(pairs))
	for i, p := range pairs {
		nodes = append(nodes, flow.NodeDef{
			ID:   string(rune('a' + i)),
			Name: p[0],
			Type: p[1],
		})
	}
	return flow.NewFlowWithID("f1", "test", flow.WithNodes(nodes...))
}

func TestDeploy_UnknownTypeErrorNamesTheType(t *testing.T) {
	e := NewEngine(WithNodeRegistry(node.NewRegistry()))
	f := flowWithNodes(t, [2]string{"Temperature", "sensor-temperature"})

	err := e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	require.ErrorIs(t, err, node.ErrNodeTypeNotFound, "타입 미등록으로 식별되어야 한다")
	require.Contains(t, err.Error(), "sensor-temperature", "없는 타입 이름이 메시지에 있어야 한다")
	require.Contains(t, err.Error(), "Temperature", "어느 노드인지도 함께 있어야 한다")
}

func TestDeploy_ReportsEveryUnknownTypeAtOnce(t *testing.T) {
	e := NewEngine(WithNodeRegistry(node.NewRegistry()))
	f := flowWithNodes(t,
		[2]string{"Temperature", "sensor-temperature"},
		[2]string{"Humidity", "sensor-humidity"},
		[2]string{"Filter", "filter"}, // 이건 빌트인이라 걸리지 않는다.
	)

	err := e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "sensor-temperature")
	require.Contains(t, msg, "sensor-humidity",
		"첫 번째에서 멈추면 둘째는 다음 시도에서야 드러난다 — 한꺼번에 알려야 한다")
	require.Contains(t, msg, "2종")
	require.NotContains(t, msg, "\"filter\"", "등록된 타입은 끼어들면 안 된다")
}

func TestDeploy_SameTypeManyNodesListsNodesOnce(t *testing.T) {
	e := NewEngine(WithNodeRegistry(node.NewRegistry()))
	f := flowWithNodes(t,
		[2]string{"T1", "sensor-temperature"},
		[2]string{"T2", "sensor-temperature"},
	)

	err := e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	msg := err.Error()
	require.Equal(t, 1, strings.Count(msg, "sensor-temperature"), "타입은 한 번만 적는다")
	require.Contains(t, msg, "T1")
	require.Contains(t, msg, "T2")
	require.Contains(t, msg, "1종")
}

func TestDeploy_KnownTypesPassValidation(t *testing.T) {
	e := NewEngine(WithNodeRegistry(node.NewRegistry()))
	f := flowWithNodes(t, [2]string{"Filter", "filter"})

	err := e.DeployFlow(context.Background(), f)
	// 배포가 다른 이유로 실패할 수는 있으나, 타입 미등록으로는 아니어야 한다.
	if err != nil {
		require.False(t, errors.Is(err, node.ErrNodeTypeNotFound),
			"등록된 타입이 미등록으로 보고되면 안 된다: %v", err)
	}
}
