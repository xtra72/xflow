package xsfm

import "github.com/xtra/xflow/internal/agent"

// 컴파일 타임 인터페이스 체크 — xsfm 는 타입별 요약 통계 provider 를 구현한다 (SPEC-DASHBOARD-003 REQ-03).
var _ agent.SummaryStatsProvider = (*XSFMAgent)(nil)

// SummaryStats 는 xsfm 타입에 유의미한 요약 카운트를 반환한다 (agent.SummaryStatsProvider).
// 등록 장비 수(devicesTotal)와 동작 중 장비 수(devicesOnline=Device.Online==true 개수)를 노출한다.
//
// 로스터 락(mu)을 단 한 번 RLock 으로 잡아 a.devices 를 직접 순회한다. lock 보유 중
// 동일 락을 재획득하는 헬퍼(ListDevices()/Name() 등)를 호출하지 않아 RWMutex 재귀 RLock
// deadlock 트랩을 회피한다 (ListDevices 와 동일한 RLock 규약, REQ-03 AC-03-2).
func (a *XSFMAgent) SummaryStats() []agent.SummaryStat {
	a.mu.RLock()
	defer a.mu.RUnlock()

	total := len(a.devices)
	online := 0
	for _, dev := range a.devices {
		if dev.Online {
			online++
		}
	}

	return []agent.SummaryStat{
		{Key: "devicesTotal", Value: int64(total)},
		{Key: "devicesOnline", Value: int64(online)},
	}
}
