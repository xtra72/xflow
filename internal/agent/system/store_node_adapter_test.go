package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// NodeStoreAdapter: 에이전트 재시작 회귀 테스트
// ---------------------------------------------------------------------------
//
// 배경:
//   UserStoreAgent.Stop → Start 경로는 내부 StoreAgent(a.inner) 를 새 인스턴스로
//   교체한다. NodeStoreAdapter 가 생성 시점의 Store 를 스냅샷으로 잡고 있으면,
//   플로우 노드(store-read/store-write)가 들고 있던 어댑터는 정지된 옛 inner 의
//   네임스페이스 뷰를 계속 가리켜 쓰기/읽기가 모두 잘못된 저장소를 향하게 된다.
//
// 본 테스트는 그 회귀를 재현·방지한다. Adapter 는 매 호출마다 resolver 를
// 통해 현재 유효한 Store 를 조회해야 한다.

// newTestRestartableUserStoreAgent 는 재시작 테스트용 UserStoreAgent 를 생성한다.
// volatile 백엔드 + 히스토리 활성화로 네임스페이스별 Store 동작을 검증할 수 있다.
func newTestRestartableUserStoreAgent(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "store-restart-test",
		Name: "restartable-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":          "volatile",
				"max_history_size": 10,
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		// 테스트 종료 시 이미 Stopped 상태일 수 있으므로 에러는 무시한다.
		_ = ag.Stop(context.Background())
	})
	return ag.(*UserStoreAgent)
}

// TestNodeStoreAdapter_SurvivesAgentRestart 는 에이전트 재시작 후에도
// 이전에 resolve 된 NodeStoreAdapter 가 새 inner 로 라우팅되는지 검증한다.
//
// 재현 시나리오 (수정 전 실패):
//  1) UserStoreAgent 생성 → NodeStoreForNamespace("default") 로 adapter 획득
//  2) adapter.Set/Get 동작 확인
//  3) Stop → Start (restart)
//  4) 동일 adapter 로 다시 Set/Get → 수정 전에는 정지된 옛 inner 로 향한다
//
// 기대 동작: 같은 adapter 인스턴스로도 재시작 후 새 inner 에 값이 기록·조회되어야 한다.
func TestNodeStoreAdapter_SurvivesAgentRestart(t *testing.T) {
	ctx := context.Background()

	userAgent := newTestRestartableUserStoreAgent(t)

	// 1) 플로우 초기화 시점에 adapter 를 한 번만 획득한다.
	raw := userAgent.NodeStoreForNamespace("default")
	adapter, ok := raw.(*NodeStoreAdapter)
	require.True(t, ok, "NodeStoreForNamespace 는 *NodeStoreAdapter 를 반환해야 한다")

	// 2) 재시작 전 기본 동작 확인.
	require.NoError(t, adapter.Set(ctx, "k1", "v1"))
	val, found, err := adapter.Get(ctx, "k1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "v1", val)

	// 3) Stop + Start 로 재시작한다. Start 경로는 a.inner 를 새 StoreAgent 로 교체한다.
	require.NoError(t, userAgent.Stop(ctx))
	require.NoError(t, userAgent.Start(ctx))

	// 4) 재시작 후에도 동일 adapter 가 유효한 inner 로 향해야 한다.
	//    (수정 전에는 여기서 옛 inner 로 쓰기가 가거나 에러가 발생한다.)
	require.NoError(t, adapter.Set(ctx, "k2", "v2"),
		"재시작 후 adapter 는 새 inner 로 라우팅되어야 한다")

	val2, found2, err := adapter.Get(ctx, "k2")
	require.NoError(t, err)
	require.True(t, found2, "재시작 후 새 inner 에서 기록된 값이 조회되어야 한다")
	assert.Equal(t, "v2", val2)

	// 5) 교차 검증: 재시작으로 옛 inner 가 날아갔으므로, 재시작 전에 쓴 k1 은
	//    새 inner 에는 존재하지 않아야 한다. 이로써 adapter 가 "옛 inner 의
	//    fork" 같은 환상을 제공하지 않음을 확인한다.
	_, foundOld, err := adapter.Get(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, foundOld,
		"재시작으로 inner 가 교체되었으므로 재시작 전 값은 남아있지 않아야 한다")
}

// TestNodeStoreAdapter_RespectsStaticKeysAfterRestart 는 재시작 전에 Configure
// 로 적용된 정적 키 정책이 재시작 후에도 유지되고, 기존 adapter 가 새 inner 의
// 정책을 그대로 따르는지 검증한다.
//
// Start(ctx) 경로는 a.agentConfig 를 다시 파싱하여 새 inner 를 만들므로,
// Configure 가 a.agentConfig 를 갱신해 둔 상태라면 정적 키 제한이 재시작 후에도
// 살아있어야 한다. 또한 재시작 전에 획득한 adapter 는 새 inner 의 strict 모드를
// 즉시 반영해야 한다.
func TestNodeStoreAdapter_RespectsStaticKeysAfterRestart(t *testing.T) {
	ctx := context.Background()

	userAgent := newTestRestartableUserStoreAgent(t)

	// 플로우 초기화 시점에 adapter 를 확보한다.
	raw := userAgent.NodeStoreForNamespace("default")
	adapter, ok := raw.(*NodeStoreAdapter)
	require.True(t, ok)

	// Configure 로 strict 모드 + 정적 키 1개 적용.
	strictCfg := agent.AgentConfig{
		ID:   "store-restart-test",
		Name: "restartable-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":            "volatile",
				"max_history_size":   10,
				"allow_dynamic_keys": false,
				"keys": []any{
					map[string]any{"key": "allowed_key"},
				},
			},
		},
	}
	require.NoError(t, userAgent.Configure(strictCfg))

	// Configure 직후: 허용된 키는 쓰기 성공, 동적 키는 거부되어야 한다.
	require.NoError(t, adapter.Set(ctx, "allowed_key", "ok"))
	err := adapter.Set(ctx, "forbidden_key", "nope")
	require.ErrorIs(t, err, ErrKeyNotAllowed,
		"Configure 적용 후 정적 키 외에는 거부되어야 한다")

	// 재시작.
	require.NoError(t, userAgent.Stop(ctx))
	require.NoError(t, userAgent.Start(ctx))

	// 재시작 후에도: 정책이 살아있고, 같은 adapter 를 통해 정책이 반영되어야 한다.
	require.NoError(t, adapter.Set(ctx, "allowed_key", "still-ok"),
		"재시작 후 허용 키 쓰기는 계속 성공해야 한다")

	err = adapter.Set(ctx, "forbidden_key", "nope")
	require.ErrorIs(t, err, ErrKeyNotAllowed,
		"재시작 후에도 정적 키 정책이 유지되어야 한다")
}

// TestNodeStoreAdapter_NamespaceIsolationAfterRestart 는 네임스페이스별
// adapter 들이 재시작 이후에도 서로 격리된 뷰를 유지하는지 검증한다.
func TestNodeStoreAdapter_NamespaceIsolationAfterRestart(t *testing.T) {
	ctx := context.Background()
	userAgent := newTestRestartableUserStoreAgent(t)

	adapterA := userAgent.NodeStoreForNamespace("ns-a").(*NodeStoreAdapter)
	adapterB := userAgent.NodeStoreForNamespace("ns-b").(*NodeStoreAdapter)

	// 재시작 전 기본 격리 확인.
	require.NoError(t, adapterA.Set(ctx, "shared", "from-a"))
	require.NoError(t, adapterB.Set(ctx, "shared", "from-b"))

	// Stop + Start.
	require.NoError(t, userAgent.Stop(ctx))
	require.NoError(t, userAgent.Start(ctx))

	// 재시작 후 다시 써서 네임스페이스가 분리되어 있는지 확인한다.
	require.NoError(t, adapterA.Set(ctx, "shared", "from-a-2"))
	require.NoError(t, adapterB.Set(ctx, "shared", "from-b-2"))

	valA, foundA, err := adapterA.Get(ctx, "shared")
	require.NoError(t, err)
	require.True(t, foundA)
	assert.Equal(t, "from-a-2", valA)

	valB, foundB, err := adapterB.Get(ctx, "shared")
	require.NoError(t, err)
	require.True(t, foundB)
	assert.Equal(t, "from-b-2", valB)
}

// ---------------------------------------------------------------------------
// NodeStoreAdapter: 단위 테스트 (고정 store 경로 및 개별 메서드)
// ---------------------------------------------------------------------------

// newStandaloneStore 는 NewNodeStoreAdapter 의 하위 호환 경로 및 개별 메서드
// 테스트를 위해 독립 StoreAgent 를 만들고 "default" 네임스페이스의 Store 를
// 반환한다.
func newStandaloneStore(t *testing.T) Store {
	t.Helper()
	sa := NewStoreAgent(WithMaxHistorySize(10))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() {
		_ = sa.Stop(context.Background())
	})
	return sa.ForNamespace("default")
}

// TestNewNodeStoreAdapter_BackwardsCompatible 는 기존 시그니처의 생성자가
// 여전히 동작하며 고정된 Store 로 동작함을 검증한다.
func TestNewNodeStoreAdapter_BackwardsCompatible(t *testing.T) {
	ctx := context.Background()
	store := newStandaloneStore(t)

	adapter := NewNodeStoreAdapter(store)
	require.NotNil(t, adapter)

	require.NoError(t, adapter.Set(ctx, "k", "v"))
	val, found, err := adapter.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "v", val)
}

// TestNodeStoreAdapter_Get_MissingKey 는 존재하지 않는 키에 대해
// ErrKeyNotFound 를 (nil, false, nil) 로 변환하는지 확인한다.
func TestNodeStoreAdapter_Get_MissingKey(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	val, found, err := adapter.Get(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, val)
}

// TestNodeStoreAdapter_SetWithTTL 는 TTL 설정 후 만료 전까지 값을 조회할 수
// 있고, 만료 후에는 not-found 로 전환됨을 확인한다.
func TestNodeStoreAdapter_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	require.NoError(t, adapter.SetWithTTL(ctx, "ephemeral", 42, 20*time.Millisecond))
	val, found, err := adapter.Get(ctx, "ephemeral")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 42, val)

	// 만료 대기. 조회 시 lazy expiration 으로 사라진다.
	time.Sleep(40 * time.Millisecond)
	_, found2, err := adapter.Get(ctx, "ephemeral")
	require.NoError(t, err)
	assert.False(t, found2, "TTL 만료 후에는 값이 조회되지 않아야 한다")
}

// TestNodeStoreAdapter_Has 는 Has 메서드가 존재/부존재를 올바르게 구분함을
// 확인한다.
func TestNodeStoreAdapter_Has(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	ok, err := adapter.Has(ctx, "absent")
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, adapter.Set(ctx, "present", "x"))
	ok, err = adapter.Has(ctx, "present")
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestNodeStoreAdapter_GetHistory 는 GetHistory 가 최신순 이전값만 반환하고
// 현재값은 제외하는 어댑터 계층의 규약을 유지함을 확인한다.
func TestNodeStoreAdapter_GetHistory(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	// 3회 Set → 히스토리에는 이전값 2개(v1, v2)가 최신순으로 남는다.
	require.NoError(t, adapter.Set(ctx, "k", "v1"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, adapter.Set(ctx, "k", "v2"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, adapter.Set(ctx, "k", "v3"))

	history, err := adapter.GetHistory(ctx, "k")
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, "v2", history[0])
	assert.Equal(t, "v1", history[1])
}

// TestNodeStoreAdapter_QueryHistory 는 QueryHistory 가 current 값 포함 최신순
// map 슬라이스를 반환하고 timestamp 를 epoch millis(int64) 로 직렬화하는지
// 확인한다 (프로젝트 타임스탬프 컨벤션).
func TestNodeStoreAdapter_QueryHistory(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	require.NoError(t, adapter.Set(ctx, "k", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, adapter.Set(ctx, "k", 2))

	results, err := adapter.QueryHistory(ctx, "k", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 2, results[0]["value"])

	ts, ok := results[0]["timestamp"].(int64)
	require.True(t, ok, "timestamp 는 epoch millis(int64) 로 직렬화되어야 한다")
	assert.Greater(t, ts, int64(0))
}

// TestNodeStoreAdapter_QueryHistory_MissingKey 는 존재하지 않는 키에 대해
// (nil, nil) 을 반환하는 관용을 유지함을 확인한다.
func TestNodeStoreAdapter_QueryHistory_MissingKey(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	results, err := adapter.QueryHistory(ctx, "nope", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	assert.Nil(t, results)
}

// TestNodeStoreAdapter_GetMetadata 는 GetMetadata 가 현재/과거 타임스탬프를
// 정상적으로 반환하는지 확인한다.
func TestNodeStoreAdapter_GetMetadata(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	require.NoError(t, adapter.Set(ctx, "k", "v1"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, adapter.Set(ctx, "k", "v2"))

	meta, err := adapter.GetMetadata(ctx, "k")
	require.NoError(t, err)
	require.NotNil(t, meta)

	// 히스토리 1개가 쌓여 있어야 한다 (v1 이 이전값으로 보관).
	count, ok := meta["store_count"].(int)
	require.True(t, ok, "store_count 는 int 이어야 한다")
	assert.Equal(t, 1, count)

	createdAt, ok := meta["store_created_at"].(time.Time)
	require.True(t, ok)
	assert.False(t, createdAt.IsZero())

	updatedAt, ok := meta["store_updated_at"].(time.Time)
	require.True(t, ok)
	assert.True(t, !updatedAt.Before(createdAt), "updated_at 은 created_at 이후여야 한다")

	oldestAt, ok := meta["store_oldest_at"].(time.Time)
	require.True(t, ok)
	assert.False(t, oldestAt.IsZero())
}

// TestNodeStoreAdapter_GetMetadata_MissingKey 는 존재하지 않는 키에 대해
// 에러를 전파함을 확인한다 (Get 경로와 달리 not-found 는 변환되지 않는다).
func TestNodeStoreAdapter_GetMetadata_MissingKey(t *testing.T) {
	ctx := context.Background()
	adapter := NewNodeStoreAdapter(newStandaloneStore(t))

	_, err := adapter.GetMetadata(ctx, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotFound)
}
