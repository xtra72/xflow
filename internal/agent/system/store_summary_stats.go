package system

import (
	"math"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// 컴파일 타임 인터페이스 체크 — store 는 타입별 요약 통계 provider 를 구현한다
// (SPEC-DASHBOARD-003 SummaryStatsProvider 패턴을 store 타입으로 확장).
var _ agent.SummaryStatsProvider = (*UserStoreAgent)(nil)

// SummaryStats 는 store 타입에 유의미한 요약 카운트를 반환한다 (agent.SummaryStatsProvider).
// 노출 항목:
//   - keysTotal:     비만료 키 개수 (State() 의 total_keys 와 동일한 만료 필터)
//   - accessTotal:   누적 접근(읽기) 횟수 (Get/Keys/GetHistory/QueryHistory 합)
//   - accessPerMin:  가동 이후 분당 평균 접근 수 (accessTotal / 가동분)
//   - historyTotal:  전체 히스토리 항목 수 (State() 의 total_history_entries 와 동일)
//
// 데드락 회피: a.mu 는 RWMutex 이고 Name()/Info()/State() 가 동일 락을 재획득한다.
// 따라서 a.mu 는 inner/startedAt 를 로컬로 스냅샷하는 동안에만 잡고, 순회 전에 해제한다.
// (State() 의 :680-682 스냅샷 패턴과 동일 — RWMutex 재귀 RLock deadlock 트랩 회피.)
func (a *UserStoreAgent) SummaryStats() []agent.SummaryStat {
	a.mu.RLock()
	inner := a.inner
	startedAt := a.startedAt
	a.mu.RUnlock()

	// 미시작(nil) 상태면 요약을 생략한다 (패널이 섹션을 표시하지 않음, xsfm/AC-07-2 와 동일).
	if inner == nil || inner.store == nil {
		return nil
	}

	now := time.Now()

	// inner.mu 를 잡고 data 를 순회하여 스레드 안전성을 보장한다 (State() 와 동일 규약).
	// State() 와 달리 maxEntries 상한 없이 전체를 순회하여 정확한 카운트를 얻는다.
	inner.mu.RLock()
	keysTotal := 0
	historyTotal := 0
	inner.store.data.Range(func(_, value any) bool {
		item, ok := value.(*storeItem)
		if !ok {
			return true
		}
		// 만료된 항목은 건너뛴다 (State() 의 만료 필터와 동일).
		if !item.expiresAt.IsZero() && item.expiresAt.Before(now) {
			return true
		}
		keysTotal++
		historyTotal += len(item.history)
		return true
	})
	inner.mu.RUnlock()

	accessTotal := inner.store.AccessCount()

	// 분당 평균 접근 수. 미시작(startedAt zero)이거나 경과 시간이 0 이하이면 0 으로 보고한다.
	var accessPerMin int64
	if !startedAt.IsZero() {
		elapsedMin := now.Sub(startedAt).Minutes()
		if elapsedMin > 0 {
			// 노이즈성 소수를 피하기 위해 정수로 반올림한다.
			accessPerMin = int64(math.Round(float64(accessTotal) / elapsedMin))
		}
	}

	return []agent.SummaryStat{
		{Key: "keysTotal", Value: int64(keysTotal)},
		{Key: "accessTotal", Value: accessTotal},
		{Key: "accessPerMin", Value: accessPerMin, Unit: "/min"},
		{Key: "historyTotal", Value: int64(historyTotal)},
	}
}
