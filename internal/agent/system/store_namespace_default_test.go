package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeStoreForNamespace_EmptyMatchesQueryDefault 는 storage-write 노드가 빈
// 네임스페이스("")로 쓴 값을 API 쿼리 경로(빈 네임스페이스 → "default" 기본값)에서
// 조회할 수 있는지 검증한다.
//
// 회귀(수정 전): 쓰기 경로(NodeStoreForNamespace)는 ""를 그대로 사용해 ":key" 에
// 저장하지만, 읽기 경로(QueryHistory)는 ""를 "default"로 기본값 적용해 "default:key"
// 를 조회 → 불일치로 등록된 키인데도 ErrKeyNotFound("store: key not found")가 났다.
func TestNodeStoreForNamespace_EmptyMatchesQueryDefault(t *testing.T) {
	ctx := context.Background()
	ua := newTestRestartableUserStoreAgent(t)

	// storage-write 노드(namespace="")처럼 빈 네임스페이스로 adapter 획득 후 쓰기.
	raw := ua.NodeStoreForNamespace("")
	adapter, ok := raw.(*NodeStoreAdapter)
	require.True(t, ok, "NodeStoreForNamespace 는 *NodeStoreAdapter 를 반환해야 한다")
	require.NoError(t, adapter.Set(ctx, "dev.current_temperature", "21.5"))

	// API 와 동일하게 빈 네임스페이스로 QueryHistory → 값이 조회되어야 한다.
	entries, err := ua.QueryHistory(ctx, "", "dev.current_temperature", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err, "빈 네임스페이스 쓰기/읽기 불일치로 key not found 회귀")
	require.Len(t, entries, 1)
	assert.Equal(t, "21.5", entries[0].Value)
}
