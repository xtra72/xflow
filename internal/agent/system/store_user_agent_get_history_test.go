// store_user_agent_get_history_test.go 는 store 에이전트 exec get_history 명령의
// 키 없음(ErrKeyNotFound) 처리를 검증한다. 존재하지 않거나 TTL 만료된 키는
// 500 INTERNAL_ERROR 가 아니라 graceful 한 빈 이력으로 응답해야 한다.
package system

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

func newGetHistoryTestAgent(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type:    "store",
			Options: map[string]any{"registration_type": "auto"},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// 존재하지 않는 키의 get_history 는 에러 없이 빈 이력(count 0, found false)을 반환한다.
func TestProcessGetHistory_키없음_graceful빈이력(t *testing.T) {
	u := newGetHistoryTestAgent(t)

	req := []byte(`{"command":"get_history","params":{"key":"does-not-exist","namespace":"default"}}`)
	out, err := u.Process(req)
	require.NoError(t, err, "키 없음은 에러가 아니라 graceful 응답이어야 한다")

	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, "does-not-exist", res["key"])
	assert.Equal(t, float64(0), res["count"])
	assert.Equal(t, false, res["found"])
	hist, ok := res["history"].([]any)
	require.True(t, ok, "history 는 배열이어야 한다")
	assert.Empty(t, hist)
}

// 존재하는 키(이력 없음)는 기존대로 빈 이력을 반환한다(회귀 확인).
func TestProcessGetHistory_키존재_이력없음(t *testing.T) {
	u := newGetHistoryTestAgent(t)
	require.NoError(t, u.inner.ForNamespace("default").Set(context.Background(), "k1", 1.0))

	req := []byte(`{"command":"get_history","params":{"key":"k1","namespace":"default"}}`)
	out, err := u.Process(req)
	require.NoError(t, err)

	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, float64(0), res["count"])
	hist, _ := res["history"].([]any)
	assert.Empty(t, hist)
}
