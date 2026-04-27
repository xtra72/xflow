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

// --- External/Internal 메시지 카운터 테스트 ---

func TestAgentStats_IncrExternalMessagesReceived(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrExternalMessagesReceived()
	stats.IncrExternalMessagesReceived()

	// 외부 카운터와 총 카운터 모두 증가해야 한다.
	assert.Equal(t, int64(2), stats.externalMessagesReceived.Load())
	assert.Equal(t, int64(2), stats.MessagesReceived())
}

func TestAgentStats_IncrExternalMessagesSent(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrExternalMessagesSent()
	stats.IncrExternalMessagesSent()
	stats.IncrExternalMessagesSent()

	assert.Equal(t, int64(3), stats.externalMessagesSent.Load())
	assert.Equal(t, int64(3), stats.MessagesSent())
}

func TestAgentStats_IncrExternalMessagesErrored(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrExternalMessagesErrored()

	assert.Equal(t, int64(1), stats.externalMessagesErrored.Load())
	assert.Equal(t, int64(1), stats.MessagesErrored())
}

func TestAgentStats_IncrInternalMessagesReceived(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrInternalMessagesReceived()
	stats.IncrInternalMessagesReceived()

	assert.Equal(t, int64(2), stats.internalMessagesReceived.Load())
	assert.Equal(t, int64(2), stats.MessagesReceived())
}

func TestAgentStats_IncrInternalMessagesSent(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrInternalMessagesSent()

	assert.Equal(t, int64(1), stats.internalMessagesSent.Load())
	assert.Equal(t, int64(1), stats.MessagesSent())
}

func TestAgentStats_IncrInternalMessagesErrored(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrInternalMessagesErrored()
	stats.IncrInternalMessagesErrored()

	assert.Equal(t, int64(2), stats.internalMessagesErrored.Load())
	assert.Equal(t, int64(2), stats.MessagesErrored())
}

func TestAgentStats_SumConsistency(t *testing.T) {
	// External + Internal == Total 이 보장되는지 확인한다.
	stats := NewAgentStats()

	// 외부/내부 혼합 호출
	stats.IncrExternalMessagesReceived() // ext=1, total=1
	stats.IncrExternalMessagesReceived() // ext=2, total=2
	stats.IncrInternalMessagesReceived() // int=1, total=3
	stats.IncrInternalMessagesReceived() // int=2, total=4
	stats.IncrInternalMessagesReceived() // int=3, total=5

	stats.IncrExternalMessagesSent()  // ext=1, total=1
	stats.IncrInternalMessagesSent()  // int=1, total=2
	stats.IncrInternalMessagesSent()  // int=2, total=3

	stats.IncrExternalMessagesErrored()  // ext=1, total=1
	stats.IncrInternalMessagesErrored()  // int=1, total=2

	// Received: 2 + 3 == 5
	assert.Equal(t, int64(2), stats.externalMessagesReceived.Load())
	assert.Equal(t, int64(3), stats.internalMessagesReceived.Load())
	assert.Equal(t, int64(5), stats.MessagesReceived())

	// Sent: 1 + 2 == 3
	assert.Equal(t, int64(1), stats.externalMessagesSent.Load())
	assert.Equal(t, int64(2), stats.internalMessagesSent.Load())
	assert.Equal(t, int64(3), stats.MessagesSent())

	// Errored: 1 + 1 == 2
	assert.Equal(t, int64(1), stats.externalMessagesErrored.Load())
	assert.Equal(t, int64(1), stats.internalMessagesErrored.Load())
	assert.Equal(t, int64(2), stats.MessagesErrored())
}

// --- DroppedMessages 테스트 ---

func TestAgentStats_IncrDroppedMessages(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrDroppedMessages()
	stats.IncrDroppedMessages()
	stats.IncrDroppedMessages()

	assert.Equal(t, int64(3), stats.DroppedMessages())
}

// --- LoadTime 테스트 ---

func TestAgentStats_SetLoadTime(t *testing.T) {
	// SetLoadTime은 한 번만 설정되어야 한다.
	stats := NewAgentStats()

	stats.SetLoadTime(500 * time.Millisecond)
	assert.Equal(t, 500*time.Millisecond, stats.LoadTime())

	// 두 번째 설정은 무시되어야 한다.
	stats.SetLoadTime(999 * time.Millisecond)
	assert.Equal(t, 500*time.Millisecond, stats.LoadTime())
}

func TestAgentStats_RecordFirstMessage(t *testing.T) {
	// startedAt 기준으로 LoadTime을 계산하는지 확인한다.
	stats := NewAgentStats()

	// startedAt이 설정되지 않은 경우 아무것도 하지 않아야 한다.
	stats.RecordFirstMessage()
	assert.Equal(t, time.Duration(0), stats.LoadTime())

	// startedAt을 100ms 전으로 설정한다.
	startTime := time.Now().Add(-100 * time.Millisecond)
	stats.SetStartedAt(startTime)

	stats.RecordFirstMessage()
	loadTime := stats.LoadTime()
	assert.True(t, loadTime >= 100*time.Millisecond, "LoadTime(%v)은 100ms 이상이어야 한다", loadTime)
	assert.True(t, loadTime < 500*time.Millisecond, "LoadTime(%v)이 너무 크다", loadTime)

	// 두 번째 호출은 무시되어야 한다.
	time.Sleep(10 * time.Millisecond)
	stats.RecordFirstMessage()
	assert.Equal(t, loadTime, stats.LoadTime(), "두 번째 RecordFirstMessage는 무시되어야 한다")
}

// --- Snapshot 새 필드 테스트 ---

func TestAgentStats_Snapshot_NewFields(t *testing.T) {
	stats := NewAgentStats()

	stats.IncrExternalMessagesReceived()
	stats.IncrExternalMessagesReceived()
	stats.IncrInternalMessagesReceived()
	stats.IncrExternalMessagesSent()
	stats.IncrInternalMessagesSent()
	stats.IncrInternalMessagesSent()
	stats.IncrExternalMessagesErrored()
	stats.IncrInternalMessagesErrored()
	stats.IncrDroppedMessages()
	stats.IncrDroppedMessages()
	stats.SetLoadTime(250 * time.Millisecond)

	snap := stats.Snapshot()

	// 외부/내부 카운터
	assert.Equal(t, int64(2), snap.ExternalMessagesReceived)
	assert.Equal(t, int64(1), snap.InternalMessagesReceived)
	assert.Equal(t, int64(1), snap.ExternalMessagesSent)
	assert.Equal(t, int64(2), snap.InternalMessagesSent)
	assert.Equal(t, int64(1), snap.ExternalMessagesErrored)
	assert.Equal(t, int64(1), snap.InternalMessagesErrored)

	// 총합 확인
	assert.Equal(t, int64(3), snap.MessagesReceived)
	assert.Equal(t, int64(3), snap.MessagesSent)
	assert.Equal(t, int64(2), snap.MessagesErrored)

	// 운영 카운터
	assert.Equal(t, int64(2), snap.DroppedMessages)
	assert.Equal(t, 250*time.Millisecond, snap.LoadTime)
}

// --- ResetStats 새 필드 테스트 ---

func TestAgentStats_ResetStats_NewFields(t *testing.T) {
	stats := NewAgentStats()

	// 새 필드들을 설정한다.
	stats.IncrExternalMessagesReceived()
	stats.IncrInternalMessagesReceived()
	stats.IncrExternalMessagesSent()
	stats.IncrInternalMessagesSent()
	stats.IncrExternalMessagesErrored()
	stats.IncrInternalMessagesErrored()
	stats.IncrDroppedMessages()
	stats.SetLoadTime(100 * time.Millisecond)
	stats.SetStartedAt(time.Now())

	// 리셋한다.
	stats.ResetStats()

	// 모든 새 필드가 0인지 확인한다.
	assert.Equal(t, int64(0), stats.externalMessagesReceived.Load())
	assert.Equal(t, int64(0), stats.externalMessagesSent.Load())
	assert.Equal(t, int64(0), stats.externalMessagesErrored.Load())
	assert.Equal(t, int64(0), stats.internalMessagesReceived.Load())
	assert.Equal(t, int64(0), stats.internalMessagesSent.Load())
	assert.Equal(t, int64(0), stats.internalMessagesErrored.Load())
	assert.Equal(t, int64(0), stats.DroppedMessages())
	assert.Equal(t, time.Duration(0), stats.LoadTime())

	// 리셋 후 LoadTime을 다시 설정할 수 있어야 한다.
	stats.SetLoadTime(200 * time.Millisecond)
	assert.Equal(t, 200*time.Millisecond, stats.LoadTime())
}

// --- 동시성 테스트 ---

func TestAgentStats_ConcurrentExternalInternal(t *testing.T) {
	// 여러 goroutine에서 외부/내부 카운터를 동시에 증가시켜도 race가 없는지 확인한다.
	stats := NewAgentStats()

	var wg sync.WaitGroup
	iterations := 1000

	wg.Add(6)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrExternalMessagesReceived()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrInternalMessagesReceived()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrExternalMessagesSent()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrInternalMessagesSent()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			stats.IncrDroppedMessages()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = stats.Snapshot()
		}
	}()

	wg.Wait()

	// External + Internal == Total
	assert.Equal(t, int64(iterations), stats.externalMessagesReceived.Load())
	assert.Equal(t, int64(iterations), stats.internalMessagesReceived.Load())
	assert.Equal(t, int64(2*iterations), stats.MessagesReceived())

	assert.Equal(t, int64(iterations), stats.externalMessagesSent.Load())
	assert.Equal(t, int64(iterations), stats.internalMessagesSent.Load())
	assert.Equal(t, int64(2*iterations), stats.MessagesSent())

	assert.Equal(t, int64(iterations), stats.DroppedMessages())
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

// --- NodeRefStats 테스트 ---

func TestAgentStats_IncrNodeRefReceived(t *testing.T) {
	// 지정 노드의 수신 카운터가 정확히 증가하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefReceived("node-1", "flow-a")
	stats.IncrNodeRefReceived("node-1", "flow-a")
	stats.IncrNodeRefReceived("node-1", "flow-a")

	snap := stats.NodeRefStatsSnapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "node-1", snap[0].NodeID)
	assert.Equal(t, int64(3), snap[0].MessagesReceived)
	assert.Equal(t, int64(0), snap[0].MessagesSent)
	assert.Equal(t, int64(0), snap[0].MessagesErrored)
	assert.False(t, snap[0].LastActivityAt.IsZero())
}

func TestAgentStats_IncrNodeRefSent(t *testing.T) {
	// 지정 노드의 송신 카운터가 정확히 증가하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefSent("node-2", "flow-b")
	stats.IncrNodeRefSent("node-2", "flow-b")

	snap := stats.NodeRefStatsSnapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "node-2", snap[0].NodeID)
	assert.Equal(t, int64(0), snap[0].MessagesReceived)
	assert.Equal(t, int64(2), snap[0].MessagesSent)
	assert.Equal(t, int64(0), snap[0].MessagesErrored)
	assert.False(t, snap[0].LastActivityAt.IsZero())
}

func TestAgentStats_IncrNodeRefErrored(t *testing.T) {
	// 지정 노드의 에러 카운터가 정확히 증가하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefErrored("node-3", "flow-c")

	snap := stats.NodeRefStatsSnapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "node-3", snap[0].NodeID)
	assert.Equal(t, int64(0), snap[0].MessagesReceived)
	assert.Equal(t, int64(0), snap[0].MessagesSent)
	assert.Equal(t, int64(1), snap[0].MessagesErrored)
	assert.False(t, snap[0].LastActivityAt.IsZero())
}

func TestAgentStats_NodeRefStatsSnapshot(t *testing.T) {
	// 여러 노드의 스냅샷이 올바른 데이터를 반환하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefReceived("node-a", "flow-1")
	stats.IncrNodeRefReceived("node-a", "flow-1")
	stats.IncrNodeRefSent("node-a", "flow-1")

	stats.IncrNodeRefReceived("node-b", "flow-2")
	stats.IncrNodeRefErrored("node-b", "flow-2")

	snap := stats.NodeRefStatsSnapshot()
	require.Len(t, snap, 2)

	// 맵으로 변환하여 순서에 무관하게 검증
	byNode := make(map[string]NodeRefStats)
	for _, s := range snap {
		byNode[s.NodeID] = s
	}

	nodeA := byNode["node-a"]
	assert.Equal(t, "flow-1", nodeA.FlowID)
	assert.Equal(t, int64(2), nodeA.MessagesReceived)
	assert.Equal(t, int64(1), nodeA.MessagesSent)
	assert.Equal(t, int64(0), nodeA.MessagesErrored)

	nodeB := byNode["node-b"]
	assert.Equal(t, "flow-2", nodeB.FlowID)
	assert.Equal(t, int64(1), nodeB.MessagesReceived)
	assert.Equal(t, int64(0), nodeB.MessagesSent)
	assert.Equal(t, int64(1), nodeB.MessagesErrored)
}

func TestAgentStats_NodeRefStatsSnapshot_Immutability(t *testing.T) {
	// 스냅샷 후 추가 증가해도 이전 스냅샷이 변경되지 않는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefReceived("node-x", "flow-1")
	snap1 := stats.NodeRefStatsSnapshot()
	require.Len(t, snap1, 1)
	assert.Equal(t, int64(1), snap1[0].MessagesReceived)

	// 추가 증가
	stats.IncrNodeRefReceived("node-x", "flow-1")
	stats.IncrNodeRefReceived("node-x", "flow-1")

	// 이전 스냅샷은 변경되지 않아야 한다.
	assert.Equal(t, int64(1), snap1[0].MessagesReceived)

	// 새 스냅샷은 업데이트된 값을 반영해야 한다.
	snap2 := stats.NodeRefStatsSnapshot()
	require.Len(t, snap2, 1)
	assert.Equal(t, int64(3), snap2[0].MessagesReceived)
}

func TestAgentStats_NodeRefStats_FlowID(t *testing.T) {
	// flowID가 올바르게 기록되는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefReceived("node-1", "flow-alpha")
	stats.IncrNodeRefSent("node-2", "flow-beta")

	snap := stats.NodeRefStatsSnapshot()
	require.Len(t, snap, 2)

	byNode := make(map[string]NodeRefStats)
	for _, s := range snap {
		byNode[s.NodeID] = s
	}

	assert.Equal(t, "flow-alpha", byNode["node-1"].FlowID)
	assert.Equal(t, "flow-beta", byNode["node-2"].FlowID)
}

func TestAgentStats_NodeRefStats_ConcurrentAccess(t *testing.T) {
	// 10개 goroutine이 서로 다른 노드를 동시에 증가시켜도 race가 없는지 확인한다.
	stats := NewAgentStats()

	var wg sync.WaitGroup
	numGoroutines := 10
	iterations := 500

	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			nodeID := "node-" + string(rune('A'+id))
			flowID := "flow-" + string(rune('0'+id))
			for i := 0; i < iterations; i++ {
				stats.IncrNodeRefReceived(nodeID, flowID)
				stats.IncrNodeRefSent(nodeID, flowID)
				stats.IncrNodeRefErrored(nodeID, flowID)
				_ = stats.NodeRefStatsSnapshot()
			}
		}(g)
	}

	wg.Wait()

	snap := stats.NodeRefStatsSnapshot()
	assert.Len(t, snap, numGoroutines)

	for _, ns := range snap {
		assert.Equal(t, int64(iterations), ns.MessagesReceived)
		assert.Equal(t, int64(iterations), ns.MessagesSent)
		assert.Equal(t, int64(iterations), ns.MessagesErrored)
	}
}

func TestAgentStats_ResetStats_ClearsNodeRefs(t *testing.T) {
	// ResetStats가 nodeRefs를 올바르게 초기화하는지 확인한다.
	stats := NewAgentStats()

	stats.IncrNodeRefReceived("node-1", "flow-1")
	stats.IncrNodeRefSent("node-2", "flow-2")
	require.Len(t, stats.NodeRefStatsSnapshot(), 2)

	stats.ResetStats()

	snap := stats.NodeRefStatsSnapshot()
	assert.Len(t, snap, 0)
}
