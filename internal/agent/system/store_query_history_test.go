package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeEntries 는 테스트용 최신순 HistoryEntry 슬라이스를 생성한다.
// base 시각으로부터 step 간격으로 count 개를 만들며, index 0 이 가장 최신이다.
func makeEntries(base time.Time, step time.Duration, values ...any) []HistoryEntry {
	result := make([]HistoryEntry, len(values))
	for i, v := range values {
		result[i] = HistoryEntry{
			Value:     v,
			Timestamp: base.Add(-time.Duration(i) * step),
		}
	}
	return result
}

// TestFilterHistoryByQuery_Latest 는 latest 모드가 현재값 1개만 반환하는지 검증한다.
func TestFilterHistoryByQuery_Latest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	entries := makeEntries(now, time.Minute, "c", "b", "a")

	got := filterHistoryByQuery(entries, HistoryQuery{Mode: QueryModeLatest}, now)

	require.Len(t, got, 1)
	assert.Equal(t, "c", got[0].Value)
}

// TestFilterHistoryByQuery_LatestEmpty 는 빈 입력에 대해 빈 결과를 반환하는지 검증한다.
func TestFilterHistoryByQuery_LatestEmpty(t *testing.T) {
	t.Parallel()
	got := filterHistoryByQuery(nil, HistoryQuery{Mode: QueryModeLatest}, time.Now())
	assert.Empty(t, got)
}

// TestFilterHistoryByQuery_LastN 은 last_n 모드가 최신순 N개를 반환하는지 검증한다.
func TestFilterHistoryByQuery_LastN(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	entries := makeEntries(now, time.Minute, "c", "b", "a")

	tests := []struct {
		name  string
		count int
		want  []any
	}{
		{"Count=1", 1, []any{"c"}},
		{"Count=2", 2, []any{"c", "b"}},
		{"Count=3 전체", 3, []any{"c", "b", "a"}},
		{"Count=10 초과분 안전처리", 10, []any{"c", "b", "a"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterHistoryByQuery(entries, HistoryQuery{Mode: QueryModeLastN, Count: tc.count}, now)
			require.Len(t, got, len(tc.want))
			for i := range got {
				assert.Equal(t, tc.want[i], got[i].Value)
			}
		})
	}
}

// TestFilterHistoryByQuery_Duration 은 duration 모드가 지정 기간 내 항목만 반환하는지 검증한다.
func TestFilterHistoryByQuery_Duration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	// 간격: 1분씩. c=now, b=now-1m, a=now-2m
	entries := makeEntries(now, time.Minute, "c", "b", "a")

	tests := []struct {
		name string
		dur  time.Duration
		want []any
	}{
		{"30초 내 현재값만", 30 * time.Second, []any{"c"}},
		{"90초 내 c,b", 90 * time.Second, []any{"c", "b"}},
		{"3분 내 전체", 3 * time.Minute, []any{"c", "b", "a"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterHistoryByQuery(entries, HistoryQuery{Mode: QueryModeDuration, Duration: tc.dur}, now)
			require.Len(t, got, len(tc.want))
			for i := range got {
				assert.Equal(t, tc.want[i], got[i].Value)
			}
		})
	}
}

// TestFilterHistoryByQuery_TimeRange 는 time_range 모드가 [From, To] 구간을 반환하는지 검증한다.
func TestFilterHistoryByQuery_TimeRange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	// c=now, b=now-1m, a=now-2m, z=now-3m
	entries := makeEntries(now, time.Minute, "c", "b", "a", "z")

	tests := []struct {
		name string
		from time.Time
		to   time.Time
		want []any
	}{
		{"전체 범위", now.Add(-10 * time.Minute), now, []any{"c", "b", "a", "z"}},
		{"가운데 범위", now.Add(-2 * time.Minute), now.Add(-1 * time.Minute), []any{"b", "a"}},
		{"정확 경계 포함", now.Add(-time.Minute), now, []any{"c", "b"}},
		{"미래 범위 비어 있음", now.Add(time.Minute), now.Add(2 * time.Minute), []any{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterHistoryByQuery(entries, HistoryQuery{Mode: QueryModeTimeRange, From: tc.from, To: tc.to}, now)
			require.Len(t, got, len(tc.want))
			for i := range got {
				assert.Equal(t, tc.want[i], got[i].Value)
			}
		})
	}
}

// TestFilterHistoryByQuery_SinceN 은 since_n 모드가 특정 시점부터 N개 제한을 적용하는지 검증한다.
func TestFilterHistoryByQuery_SinceN(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	entries := makeEntries(now, time.Minute, "c", "b", "a", "z")

	tests := []struct {
		name  string
		since time.Time
		count int
		want  []any
	}{
		{"경계부터 최대 2개", now.Add(-2 * time.Minute), 2, []any{"c", "b"}},
		{"전체 범위 3개 제한", now.Add(-10 * time.Minute), 3, []any{"c", "b", "a"}},
		{"미래 시점 비어 있음", now.Add(time.Minute), 5, []any{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := HistoryQuery{Mode: QueryModeSinceN, Since: tc.since, Count: tc.count}
			got := filterHistoryByQuery(entries, q, now)
			require.Len(t, got, len(tc.want))
			for i := range got {
				assert.Equal(t, tc.want[i], got[i].Value)
			}
		})
	}
}

// TestHistoryQuery_Validate 는 각 모드별 필수 파라미터 검증을 확인한다.
func TestHistoryQuery_Validate(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		query   HistoryQuery
		wantErr bool
	}{
		{"latest OK", HistoryQuery{Mode: QueryModeLatest}, false},
		{"last_n OK", HistoryQuery{Mode: QueryModeLastN, Count: 3}, false},
		{"last_n count=0 실패", HistoryQuery{Mode: QueryModeLastN, Count: 0}, true},
		{"duration OK", HistoryQuery{Mode: QueryModeDuration, Duration: time.Minute}, false},
		{"duration 0 실패", HistoryQuery{Mode: QueryModeDuration}, true},
		{"time_range OK", HistoryQuery{Mode: QueryModeTimeRange, From: now.Add(-time.Hour), To: now}, false},
		{"time_range from>to 실패", HistoryQuery{Mode: QueryModeTimeRange, From: now, To: now.Add(-time.Hour)}, true},
		{"time_range 누락 실패", HistoryQuery{Mode: QueryModeTimeRange, From: now}, true},
		{"since_n OK", HistoryQuery{Mode: QueryModeSinceN, Since: now, Count: 1}, false},
		{"since_n count 누락 실패", HistoryQuery{Mode: QueryModeSinceN, Since: now}, true},
		{"since_n since 누락 실패", HistoryQuery{Mode: QueryModeSinceN, Count: 1}, true},
		{"알 수 없는 모드 실패", HistoryQuery{Mode: "bogus"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.query.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestVolatileStore_QueryHistory_IncludesCurrentValue 는 QueryHistory 결과가
// 현재값(Set 이후 유지된 item.value)을 포함하는지 통합 검증한다.
func TestVolatileStore_QueryHistory_IncludesCurrentValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	require.NoError(t, store.Set(ctx, "k", "a"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, store.Set(ctx, "k", "b"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, store.Set(ctx, "k", "c"))

	got, err := store.QueryHistory(ctx, "k", HistoryQuery{Mode: QueryModeLastN, Count: 10})
	require.NoError(t, err)

	// 현재값 "c" + 히스토리 "b", "a" = 총 3개
	require.Len(t, got, 3)
	assert.Equal(t, "c", got[0].Value)
	assert.Equal(t, "b", got[1].Value)
	assert.Equal(t, "a", got[2].Value)
}

// TestVolatileStore_QueryHistory_NotFound 는 존재하지 않는 키에 ErrKeyNotFound를 반환하는지 검증한다.
func TestVolatileStore_QueryHistory_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	_, err := store.QueryHistory(ctx, "missing", HistoryQuery{Mode: QueryModeLatest})
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_QueryHistory_InvalidQuery 는 잘못된 쿼리에 검증 에러를 반환하는지 확인한다.
func TestVolatileStore_QueryHistory_InvalidQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)
	require.NoError(t, store.Set(ctx, "k", "v"))

	_, err := store.QueryHistory(ctx, "k", HistoryQuery{Mode: QueryModeLastN, Count: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count > 0")
}
