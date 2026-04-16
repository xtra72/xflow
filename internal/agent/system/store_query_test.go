package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// UserStoreAgent.QueryHistory 는 SPEC-CHART-001 M3 (REQ-M3-01) 에서 추가된
// HTTP 핸들러용 진입점이다.

// newTestUserStoreAgent 는 테스트용으로 Running 상태의 UserStoreAgent 를 생성한다.
func newTestUserStoreAgent(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "store-test",
		Name: "test-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":          "volatile",
				"max_history_size": 100,
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ag.Stop(context.Background())
	})
	return ag.(*UserStoreAgent)
}

// TestUserStoreAgent_QueryHistory_latest 는 latest 모드에서 가장 최근 값을 1개
// 반환하는지 확인한다.
func TestUserStoreAgent_QueryHistory_latest(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "k1", 10))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, store.Set(ctx, "k1", 20))

	entries, err := a.QueryHistory(ctx, "default", "k1", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 20, entries[0].Value)
}

// TestUserStoreAgent_QueryHistory_last_n 는 last_n 모드에서 최신 N 개를 반환하는지
// 확인한다.
func TestUserStoreAgent_QueryHistory_last_n(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Set(ctx, "k", i))
		time.Sleep(2 * time.Millisecond)
	}

	entries, err := a.QueryHistory(ctx, "default", "k", HistoryQuery{Mode: QueryModeLastN, Count: 3})
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

// TestUserStoreAgent_QueryHistory_namespace_분리 는 서로 다른 네임스페이스의
// 데이터가 섞이지 않음을 확인한다.
func TestUserStoreAgent_QueryHistory_namespace_분리(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()
	require.NoError(t, a.inner.ForNamespace("ns1").Set(ctx, "shared", "v1"))
	require.NoError(t, a.inner.ForNamespace("ns2").Set(ctx, "shared", "v2"))

	entries1, err := a.QueryHistory(ctx, "ns1", "shared", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	require.Len(t, entries1, 1)
	assert.Equal(t, "v1", entries1[0].Value)

	entries2, err := a.QueryHistory(ctx, "ns2", "shared", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	require.Len(t, entries2, 1)
	assert.Equal(t, "v2", entries2[0].Value)
}

// TestUserStoreAgent_QueryHistory_validate_에러 는 HistoryQuery 필수 필드가
// 누락되면 에러를 반환하는지 확인한다.
func TestUserStoreAgent_QueryHistory_validate_에러(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()

	// last_n 인데 count 0 → validate 실패
	_, err := a.QueryHistory(ctx, "default", "k", HistoryQuery{Mode: QueryModeLastN, Count: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

// TestUserStoreAgent_QueryHistory_빈네임스페이스_default 는 빈 네임스페이스가
// "default" 로 취급되는지 확인한다.
func TestUserStoreAgent_QueryHistory_빈네임스페이스_default(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()
	require.NoError(t, a.inner.ForNamespace("default").Set(ctx, "x", 42))

	entries, err := a.QueryHistory(ctx, "", "x", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 42, entries[0].Value)
}

// TestUserStoreAgent_QueryHistory_미존재키 는 존재하지 않는 키에 대해
// ErrKeyNotFound 를 전파함을 확인한다.
func TestUserStoreAgent_QueryHistory_미존재키(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()

	_, err := a.QueryHistory(ctx, "default", "ghost", HistoryQuery{Mode: QueryModeLatest})
	require.Error(t, err)
}
