package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// @spec SPEC-STORE-003 v0.3.0 (Phase B 회귀 수정)
// UserStoreAgent.Configure 가 max_history_size 등 운영 필드를 런타임에 전파하는지 검증.
//
// 버그: Phase B 에서 SetRegistrationType / SetStaticKeys 만 추가하고
// 기존 v0.2.0 운영 필드 setter 가 누락되어 hot reload 가 작동하지 않음.

func TestConfigure_MaxHistorySize_PropagatedToInnerStore(t *testing.T) {
	// GIVEN: max_history_size=0 으로 시작 (default)
	cfg := agent.AgentConfig{
		ID:   "test-store",
		Name: "test-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)

	storeAg := ag.(*UserStoreAgent)
	require.NoError(t, storeAg.Start(context.Background()))
	defer func() { _ = storeAg.Stop(context.Background()) }()

	// 초기에는 history 비활성
	assert.Equal(t, 0, storeAg.inner.config.maxHistorySize, "초기 maxHistorySize=0 확인")
	assert.Equal(t, 0, storeAg.inner.store.maxHistorySize, "초기 inner.store.maxHistorySize=0 확인")

	// WHEN: Configure 로 max_history_size: 1000 설정
	newCfg := agent.AgentConfig{
		ID:   "test-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"max_history_size": 1000,
			},
		},
	}
	require.NoError(t, storeAg.Configure(newCfg))

	// THEN: inner StoreAgent + VolatileStore 모두 1000 으로 변경되어야 함
	assert.Equal(t, 1000, storeAg.inner.config.maxHistorySize, "inner.config.maxHistorySize=1000")
	assert.Equal(t, 1000, storeAg.inner.store.maxHistorySize, "inner.store.maxHistorySize=1000 (핫 리로드)")
}

func TestConfigure_HistoryAccumulates_AfterRuntimeUpdate(t *testing.T) {
	// GIVEN: max_history_size=0 으로 부팅, 후 Configure 로 1000 변경
	cfg := agent.AgentConfig{
		ID:   "test-store",
		Name: "test-store",
		Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	storeAg := ag.(*UserStoreAgent)
	require.NoError(t, storeAg.Start(context.Background()))
	defer func() { _ = storeAg.Stop(context.Background()) }()

	require.NoError(t, storeAg.Configure(agent.AgentConfig{
		ID: "test-store", Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{"max_history_size": 1000},
		},
	}))

	// WHEN: 같은 키에 5번 다른 값 쓰기
	store := storeAg.inner.ForNamespace("default")
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Set(ctx, "k1", i))
	}

	// THEN: history 4개 누적 (현재 값 제외)
	history, err := store.GetHistory(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, 4, len(history), "5번 쓰기 후 history 4개 (5번째는 현재 값)")
}

func TestConfigure_HistoryTTL_PropagatedToInnerStore(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "test-store", Name: "test-store", Type: "store",
		Transport: agent.TransportConfig{Options: map[string]any{}},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	storeAg := ag.(*UserStoreAgent)
	require.NoError(t, storeAg.Start(context.Background()))
	defer func() { _ = storeAg.Stop(context.Background()) }()

	require.NoError(t, storeAg.Configure(agent.AgentConfig{
		ID: "test-store", Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{"history_ttl": "1h"},
		},
	}))

	assert.Equal(t, time.Hour, storeAg.inner.config.historyTTL)
	assert.Equal(t, time.Hour, storeAg.inner.store.historyTTL)
}

func TestConfigure_ScanInterval_PropagatedToInnerStore(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "test-store", Name: "test-store", Type: "store",
		Transport: agent.TransportConfig{Options: map[string]any{}},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	storeAg := ag.(*UserStoreAgent)
	require.NoError(t, storeAg.Start(context.Background()))
	defer func() { _ = storeAg.Stop(context.Background()) }()

	require.NoError(t, storeAg.Configure(agent.AgentConfig{
		ID: "test-store", Name: "test-store", Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{"scan_interval": "5s"},
		},
	}))

	assert.Equal(t, 5*time.Second, storeAg.inner.config.scanInterval)
	// ttlMgr.SetInterval 호출 검증은 별도 (ttlMgr 내부 상태 직접 접근 어려움)
}

func TestConfigure_PreservesExistingEntries_AfterRuntimeUpdate(t *testing.T) {
	// SPEC-STORE-003 M5: 정적 키 정의 변경되어도 기존 entries/history 보존
	cfg := agent.AgentConfig{
		ID: "test-store", Name: "test-store", Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{"max_history_size": 100},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	storeAg := ag.(*UserStoreAgent)
	require.NoError(t, storeAg.Start(context.Background()))
	defer func() { _ = storeAg.Stop(context.Background()) }()

	// 데이터 쓰기
	store := storeAg.inner.ForNamespace("default")
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "k1", 100))
	require.NoError(t, store.Set(ctx, "k1", 200))

	// Configure 호출 (max_history_size 변경)
	require.NoError(t, storeAg.Configure(agent.AgentConfig{
		ID: "test-store", Name: "test-store", Type: "store",
		Transport: agent.TransportConfig{
			Options: map[string]any{"max_history_size": 1000},
		},
	}))

	// 기존 데이터 보존 확인
	entry, err := store.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, 200, entry.Value)

	history, err := store.GetHistory(ctx, "k1")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(history), 1, "이전 history 보존")
}
