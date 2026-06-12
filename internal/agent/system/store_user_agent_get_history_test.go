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
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"max_history_size":  100, // 히스토리 활성화
			},
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

// 빈 네임스페이스("")로 저장된 키의 네임스페이스 라운드트립 버그.
// 네임스페이스 라운드트립 불일치로 인해:
// 1. State() displayKey 에서 ns=""일 때 TrimPrefix 미적용 -> displayKey가 `:09a...power` 형태
// 2. 웹 UI에서 entry.namespace="" 가 falsy -> "default"로 강제
// 3. processGetHistory에서 namespace="" -> "default"로 강제
// 4. 결과: ForNamespace("").GetHistory() 대신 ForNamespace("default").GetHistory() 실행 -> 히스토리 0개
//
// 수정 후:
// - State() displayKey: ns=""일 때도 TrimPrefix(":") 적용 -> `:09a...power` -> `09a...power`
// - processGetHistory: namespace="" 강제 제거, 그대로 사용
// - 웹 UI: entry.namespace="" 도 전송 (|| 'default' -> ?? ”)
// - 결과: 저장된 실제 네임스페이스로 라운드트립됨
func TestProcessGetHistory_빈네임스페이스_라운드트립(t *testing.T) {
	u := newGetHistoryTestAgent(t)
	ctx := context.Background()

	// 1단계: 빈 네임스페이스에 13개 이력을 갖는 키 생성
	// (초기값 1개 + 12개 업데이트 = 13개 이력)
	keyName := "09a34282-7959-49e7-9517-e6bfa7ed267d.power"
	store := u.inner.ForNamespace("")
	for i := 0; i < 13; i++ {
		require.NoError(t, store.Set(ctx, keyName, float64(10+i)))
	}

	// 2단계: processGetHistory 에서 namespace="" 를 그대로 사용하면 12개 이력 반환
	// (버그 시 namespace="" -> "default" 강제로 인해 0개)
	req := []byte(`{"command":"get_history","params":{"key":"09a34282-7959-49e7-9517-e6bfa7ed267d.power","namespace":""}}`)
	out, err := u.Process(req)
	require.NoError(t, err, "get_history 응답에 에러가 없어야 함")

	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, keyName, res["key"])
	assert.Equal(t, "", res["namespace"], "응답 namespace는 빈 문자열이어야 함")

	// 핵심 검증: 12개 이력이 반환되어야 함 (버그 시 count=0)
	// (13번 Set했으므로 12개 업데이트 이력 반환 - GetHistory는 현재값 제외하고 히스토리만 반환)
	count, ok := res["count"].(float64)
	require.True(t, ok, "count는 float64여야 함")
	assert.Equal(t, float64(12), count, "namespace=\"\"로 저장된 키는 12개 이력이 있어야 함")

	hist, ok := res["history"].([]any)
	require.True(t, ok, "history 는 배열이어야 함")
	assert.Equal(t, 12, len(hist), "이력 배열이 12개여야 함")
}
