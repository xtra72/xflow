package xsfm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// TestSummaryStats_TotalAndOnline 는 등록/동작 중 장비 수 요약 카운트를 검증한다
// (SPEC-DASHBOARD-003 REQ-03 AC-03-1).
func TestSummaryStats_TotalAndOnline(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "name": "장비-1"},
		map[string]any{"device_id": "ap-102", "name": "장비-2"},
		map[string]any{"device_id": "ap-103", "name": "장비-3"},
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// config 디바이스는 Online=false 로 등록된다 → 일부를 online 으로 표시.
	ap.mu.Lock()
	ap.devices["ap-101"].Online = true
	ap.devices["ap-102"].Online = true
	ap.mu.Unlock()

	summary := ap.SummaryStats()

	got := make(map[string]int64, len(summary))
	for _, s := range summary {
		got[s.Key] = s.Value
	}
	assert.Equal(t, int64(3), got["devicesTotal"], "등록 장비 수는 3 이어야 한다")
	assert.Equal(t, int64(2), got["devicesOnline"], "동작 중 장비 수는 2 여야 한다")
}

// TestSummaryStats_Empty 는 등록 장비가 없을 때 0 카운트를 반환하는지 검증한다.
func TestSummaryStats_Empty(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	summary := ap.SummaryStats()

	got := make(map[string]int64, len(summary))
	for _, s := range summary {
		got[s.Key] = s.Value
	}
	assert.Equal(t, int64(0), got["devicesTotal"])
	assert.Equal(t, int64(0), got["devicesOnline"])
}

// TestSummaryStats_ProviderInterface 는 xsfm 이 SummaryStatsProvider 를 만족하는지 확인한다
// (AC-01-1: 옵셔널 인터페이스 관례 준수).
func TestSummaryStats_ProviderInterface(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)

	_, ok := agent.Agent(a).(agent.SummaryStatsProvider)
	assert.True(t, ok, "XSFMAgent 는 SummaryStatsProvider 를 구현해야 한다")
}
