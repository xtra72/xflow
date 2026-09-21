// 노드 설정이 API 로 나갈 때의 계약 (@SPEC:SPEC-FLOW-NODECONFIG-001).
//
// 신고(2026-09-21): GET /flows/{id}/nodes →
// `json: unsupported type: node.AgentLookupFunc` (500).
// 노드 옵션이 `_` 키에 주입한 함수가 응답에 실려 직렬화가 통째로 깨졌다.
package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

/** 의존성이 주입된 노드 하나를 만든다(데몬이 모든 노드에 거는 옵션과 같은 방식). */
func nodeWithInjectedDeps(t *testing.T, userCfg map[string]any) node.Node {
	t.Helper()
	reg := node.NewRegistry()
	lookup := node.AgentLookupFunc(func(string) (node.RegistryMeta, bool) {
		return node.RegistryMeta{}, false
	})
	deviceLookup := node.DeviceLookupFunc(func(string) (node.RegistryMeta, bool) {
		return node.RegistryMeta{}, false
	})
	n, err := reg.Create(
		flow.NodeDef{ID: "n1", Name: "Temperature", Type: "filter", Config: userCfg},
		node.WithAgentInfoLookup(lookup),
		node.WithDeviceInfoLookup(deviceLookup),
	)
	require.NoError(t, err)
	return n
}

func TestNodeInfo_IsJSONSerializable(t *testing.T) {
	info := buildNodeInstanceInfo(nodeWithInjectedDeps(t, nil), nil)

	_, err := json.Marshal(info)
	require.NoError(t, err,
		"주입된 의존성이 응답에 실리면 목록 요청 하나가 통째로 500 이 된다")
}

func TestNodeInfo_DropsInternalInjectedKeys(t *testing.T) {
	info := buildNodeInstanceInfo(nodeWithInjectedDeps(t, nil), nil)

	for k := range info.Config {
		require.NotContains(t, k, "_enrich_agent_lookup")
		require.NotContains(t, k, "_enrich_device_lookup")
	}
}

func TestPublicNodeConfig_KeepsUserKeysDropsInternal(t *testing.T) {
	// 사용자가 편집기에서 넣은 설정은 그대로 나가야 한다 — 화면이 그 값을 읽는다.
	// 내부 주입 키만 빠진다.
	//
	// 노드를 거치지 않고 순수 함수를 직접 부른다. 엔진은 노드를 만든 뒤
	// `Configure(nd.Config)` 로 설정 map 을 **통째로 교체**하므로(engine.go), 노드
	// 하나에서 두 종류의 키가 동시에 살아 있는 상태를 만들기가 오히려 부자연스럽다.
	// 걸러야 할 규칙 자체를 여기서 고정한다.
	out := publicNodeConfig(map[string]any{
		"property":             "temp",
		"value":                42,
		"_enrich_agent_lookup": func() {},
		"_agent_resolver":      struct{}{},
	})

	require.Equal(t, "temp", out["property"])
	require.EqualValues(t, 42, out["value"])
	require.NotContains(t, out, "_enrich_agent_lookup")
	require.NotContains(t, out, "_agent_resolver")

	raw, err := json.Marshal(out)
	require.NoError(t, err, "걸러 낸 뒤에는 직렬화가 성공해야 한다")
	require.Contains(t, string(raw), "temp")
}

func TestPublicNodeConfig_NilStaysNil(t *testing.T) {
	// 설정이 없는 노드는 빈 map 이 아니라 nil 이어야 한다(응답 모양 보존).
	require.Nil(t, publicNodeConfig(nil))
}
