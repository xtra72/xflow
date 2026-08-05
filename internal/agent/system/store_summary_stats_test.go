package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// summaryMap 는 SummaryStat 슬라이스를 key→value 맵으로 변환한다 (테스트 편의).
func summaryMap(stats []agent.SummaryStat) map[string]int64 {
	m := make(map[string]int64, len(stats))
	for _, s := range stats {
		m[s.Key] = s.Value
	}
	return m
}

// TestStoreSummaryStats_Counts 는 키 개수/히스토리/접근 카운트 요약을 검증한다
// (SPEC-DASHBOARD-003 SummaryStatsProvider 를 store 로 확장).
func TestStoreSummaryStats_Counts(t *testing.T) {
	a := newTestUserStoreAgent(t)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")

	// k1: 값 2회 변경 → 히스토리 1개.
	require.NoError(t, store.Set(ctx, "k1", 1))
	require.NoError(t, store.Set(ctx, "k1", 2))
	// k2: 값 1회 → 히스토리 0개.
	require.NoError(t, store.Set(ctx, "k2", 10))
	// k3: 값 3회 변경 → 히스토리 2개.
	require.NoError(t, store.Set(ctx, "k3", 100))
	require.NoError(t, store.Set(ctx, "k3", 200))
	require.NoError(t, store.Set(ctx, "k3", 300))
	// expired: 짧은 TTL 후 만료 → keysTotal 에서 제외되어야 한다.
	require.NoError(t, store.SetWithTTL(ctx, "expired", 1, 5*time.Millisecond))
	time.Sleep(20 * time.Millisecond)

	// 접근 카운트 델타를 정밀 측정하기 위해 읽기 직전 기준값을 캡처한다.
	// (Set/SetWithTTL 은 접근 카운트를 증가시키지 않는다.)
	before := a.inner.store.AccessCount()

	// 4종 읽기 연산을 각 1회 수행 → 접근 카운트 정확히 +4.
	_, err := store.Get(ctx, "k1")
	require.NoError(t, err)
	_, err = store.Keys(ctx, "*")
	require.NoError(t, err)
	_, err = store.GetHistory(ctx, "k1")
	require.NoError(t, err)
	_, err = store.QueryHistory(ctx, "k1", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)

	summary := a.SummaryStats()
	got := summaryMap(summary)

	assert.Equal(t, int64(3), got["keysTotal"], "비만료 키는 k1/k2/k3 3개여야 한다 (expired 제외)")
	assert.Equal(t, int64(3), got["historyTotal"], "히스토리 합은 1+0+2=3 이어야 한다")
	assert.Equal(t, before+4, got["accessTotal"], "수행한 4회 읽기만큼 접근 수가 증가해야 한다")
	assert.GreaterOrEqual(t, got["accessPerMin"], int64(0), "분당 접근 수는 음수가 아니어야 한다")

	// Unit 표기 확인 — accessPerMin 은 "/min" 단위를 가진다.
	for _, s := range summary {
		if s.Key == "accessPerMin" {
			assert.Equal(t, "/min", s.Unit)
		}
	}
}

// TestStoreSummaryStats_NotStarted 는 미시작(inner nil) 상태에서 nil 을 반환하는지 검증한다
// (패널이 섹션을 생략하는 graceful 처리, xsfm/AC-07-2 와 동일).
func TestStoreSummaryStats_NotStarted(t *testing.T) {
	a := &UserStoreAgent{}
	assert.Nil(t, a.SummaryStats(), "미시작 에이전트는 nil 요약을 반환해야 한다")
}

// TestStoreSummaryStats_ProviderInterface 는 store 가 SummaryStatsProvider 를 만족하는지 확인한다.
func TestStoreSummaryStats_ProviderInterface(t *testing.T) {
	a := newTestUserStoreAgent(t)

	_, ok := agent.Agent(a).(agent.SummaryStatsProvider)
	assert.True(t, ok, "UserStoreAgent 는 SummaryStatsProvider 를 구현해야 한다")
}

// TestVolatileStore_AccessCount_IncrementsPerRead 는 4종 읽기 메서드가 각각 접근
// 카운트를 정확히 1씩 증가시키는지 검증한다 (쓰기는 증가시키지 않음).
func TestVolatileStore_AccessCount_IncrementsPerRead(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 100, 0)

	// 쓰기는 접근 카운트를 증가시키지 않는다.
	require.NoError(t, store.Set(ctx, "k", 1))
	require.NoError(t, store.Set(ctx, "k", 2))
	assert.Equal(t, int64(0), store.AccessCount(), "Set 은 접근 카운트를 증가시키지 않아야 한다")

	_, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, int64(1), store.AccessCount(), "Get 후 1")

	_, err = store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Equal(t, int64(2), store.AccessCount(), "Keys 후 2")

	_, err = store.GetHistory(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, int64(3), store.AccessCount(), "GetHistory 후 3")

	_, err = store.QueryHistory(ctx, "k", HistoryQuery{Mode: QueryModeLatest})
	require.NoError(t, err)
	assert.Equal(t, int64(4), store.AccessCount(), "QueryHistory 후 4")
}
