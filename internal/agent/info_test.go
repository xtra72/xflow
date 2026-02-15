package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestAgentInfo_Creation(t *testing.T) {
	// AgentInfo 구조체를 생성하고 필드를 확인한다.
	now := time.Now()
	info := AgentInfo{
		ID:    "agent-1",
		Name:  "Test Agent",
		Type:  "custom",
		State: lifecycle.StateRunning,
		Health: HealthStatus{
			Status:  HealthHealthy,
			Message: "OK",
		},
		Config: AgentConfig{
			ID:   "agent-1",
			Name: "Test Agent",
		},
		Stats: StatsSnapshot{},
		SharedInfo: &SharedInfo{
			RefCount: 2,
			Flows:    []string{"flow-1", "flow-2"},
		},
		StartedAt: now,
		Uptime:    5 * time.Minute,
		CreatedAt: now.Add(-10 * time.Minute),
	}

	assert.Equal(t, "agent-1", info.ID)
	assert.Equal(t, "Test Agent", info.Name)
	assert.Equal(t, "custom", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.Equal(t, HealthHealthy, info.Health.Status)
	require.NotNil(t, info.SharedInfo)
	assert.Equal(t, int32(2), info.SharedInfo.RefCount)
	assert.Len(t, info.SharedInfo.Flows, 2)
	assert.Equal(t, 5*time.Minute, info.Uptime)
}

func TestAgentInfo_NilSharedInfo(t *testing.T) {
	// SharedInfo가 nil인 경우를 확인한다.
	info := AgentInfo{
		ID:         "agent-2",
		SharedInfo: nil,
	}

	assert.Nil(t, info.SharedInfo)
}

func TestAgentStats_ZeroValue(t *testing.T) {
	// AgentStats의 zero value를 확인한다.
	stats := NewAgentStats()

	assert.Equal(t, int64(0), stats.MessagesReceived())
	assert.Equal(t, int64(0), stats.MessagesSent())
	assert.Equal(t, int64(0), stats.MessagesErrored())
	assert.Equal(t, int64(0), stats.BytesRead())
	assert.Equal(t, int64(0), stats.BytesWritten())
	assert.True(t, stats.LastActivityAt().IsZero())
	assert.Equal(t, time.Duration(0), stats.AvgProcessingLatency())
	assert.Equal(t, int64(0), stats.RestartCount())
}

func TestAgentStats_IncrMessagesReceived(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrMessagesReceived()
	stats.IncrMessagesReceived()
	stats.IncrMessagesReceived()

	assert.Equal(t, int64(3), stats.MessagesReceived())
}

func TestAgentStats_IncrMessagesSent(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrMessagesSent()
	stats.IncrMessagesSent()

	assert.Equal(t, int64(2), stats.MessagesSent())
}

func TestAgentStats_IncrMessagesErrored(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrMessagesErrored()

	assert.Equal(t, int64(1), stats.MessagesErrored())
}

func TestAgentStats_AddBytesRead(t *testing.T) {
	stats := NewAgentStats()

	stats.AddBytesRead(100)
	stats.AddBytesRead(200)

	assert.Equal(t, int64(300), stats.BytesRead())
}

func TestAgentStats_AddBytesWritten(t *testing.T) {
	stats := NewAgentStats()

	stats.AddBytesWritten(50)
	stats.AddBytesWritten(150)

	assert.Equal(t, int64(200), stats.BytesWritten())
}

func TestAgentStats_UpdateLastActivity(t *testing.T) {
	stats := NewAgentStats()

	assert.True(t, stats.LastActivityAt().IsZero())

	stats.UpdateLastActivity()

	assert.False(t, stats.LastActivityAt().IsZero())
	assert.WithinDuration(t, time.Now(), stats.LastActivityAt(), time.Second)
}

func TestAgentStats_IncrRestartCount(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrRestartCount()
	stats.IncrRestartCount()

	assert.Equal(t, int64(2), stats.RestartCount())
}

func TestAgentStats_ResetStats(t *testing.T) {
	stats := NewAgentStats()

	// 카운터를 증가시킨다.
	stats.IncrMessagesReceived()
	stats.IncrMessagesSent()
	stats.IncrMessagesErrored()
	stats.AddBytesRead(100)
	stats.AddBytesWritten(200)
	stats.UpdateLastActivity()
	stats.IncrRestartCount()

	// 리셋 전 값이 있는지 확인한다.
	assert.NotEqual(t, int64(0), stats.MessagesReceived())

	// 리셋한다.
	stats.ResetStats()

	// 모든 카운터가 0인지 확인한다.
	assert.Equal(t, int64(0), stats.MessagesReceived())
	assert.Equal(t, int64(0), stats.MessagesSent())
	assert.Equal(t, int64(0), stats.MessagesErrored())
	assert.Equal(t, int64(0), stats.BytesRead())
	assert.Equal(t, int64(0), stats.BytesWritten())
	assert.Equal(t, int64(0), stats.RestartCount())
	assert.True(t, stats.LastActivityAt().IsZero())
	assert.Equal(t, time.Duration(0), stats.AvgProcessingLatency())
}

func TestAgentStats_Snapshot(t *testing.T) {
	// Snapshot()이 현재 상태의 불변 사본을 반환하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrMessagesReceived()
	stats.IncrMessagesSent()
	stats.AddBytesRead(512)
	stats.IncrRestartCount()

	snap := stats.Snapshot()

	assert.Equal(t, int64(1), snap.MessagesReceived)
	assert.Equal(t, int64(1), snap.MessagesSent)
	assert.Equal(t, int64(0), snap.MessagesErrored)
	assert.Equal(t, int64(512), snap.BytesRead)
	assert.Equal(t, int64(0), snap.BytesWritten)
	assert.Equal(t, int64(1), snap.RestartCount)
}

func TestAgentStats_ConcurrentAccess(t *testing.T) {
	// 여러 goroutine에서 동시에 카운터를 증가시켜도 안전한지 확인한다.
	stats := NewAgentStats()

	var wg sync.WaitGroup
	iterations := 1000

	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrMessagesReceived()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrMessagesSent()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.AddBytesRead(1)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.UpdateLastActivity()
		}
	}()

	wg.Wait()

	assert.Equal(t, int64(iterations), stats.MessagesReceived())
	assert.Equal(t, int64(iterations), stats.MessagesSent())
	assert.Equal(t, int64(iterations), stats.BytesRead())
}

func TestSharedInfo_Creation(t *testing.T) {
	// SharedInfo 구조체를 생성하고 확인한다.
	si := SharedInfo{
		RefCount: 3,
		Flows:    []string{"flow-a", "flow-b", "flow-c"},
	}

	assert.Equal(t, int32(3), si.RefCount)
	assert.Len(t, si.Flows, 3)
	assert.Contains(t, si.Flows, "flow-a")
}

func TestManagerSummary_Creation(t *testing.T) {
	// ManagerSummary 구조체를 생성하고 확인한다.
	summary := ManagerSummary{
		TotalAgents:            5,
		RunningAgents:          3,
		PausedAgents:           1,
		StoppedAgents:          1,
		ErrorAgents:            0,
		HealthyAgents:          4,
		UnhealthyAgents:        1,
		TotalMessagesProcessed: 1000,
		TotalErrors:            5,
		Agents:                 []AgentInfo{{ID: "a1"}, {ID: "a2"}},
	}

	assert.Equal(t, 5, summary.TotalAgents)
	assert.Equal(t, 3, summary.RunningAgents)
	assert.Equal(t, 1, summary.PausedAgents)
	assert.Equal(t, 1, summary.StoppedAgents)
	assert.Equal(t, 0, summary.ErrorAgents)
	assert.Equal(t, 4, summary.HealthyAgents)
	assert.Equal(t, 1, summary.UnhealthyAgents)
	assert.Equal(t, int64(1000), summary.TotalMessagesProcessed)
	assert.Equal(t, int64(5), summary.TotalErrors)
	assert.Len(t, summary.Agents, 2)
}

func TestStatsSnapshot_IsImmutable(t *testing.T) {
	// Snapshot이 반환된 후 원본을 변경해도 Snapshot에 영향이 없는지 확인한다.
	stats := NewAgentStats()
	stats.IncrMessagesReceived()

	snap := stats.Snapshot()
	assert.Equal(t, int64(1), snap.MessagesReceived)

	// 원본에 추가 증가
	stats.IncrMessagesReceived()

	// Snapshot은 변경되지 않아야 한다.
	assert.Equal(t, int64(1), snap.MessagesReceived)
	assert.Equal(t, int64(2), stats.MessagesReceived())
}
