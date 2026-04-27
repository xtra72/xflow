package node

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// --- BridgeInfo 테스트 ---

// TestBridgeInfo_구조체 은 BridgeInfo 구조체가 올바르게 초기화되는지 확인한다.
func TestBridgeInfo_구조체(t *testing.T) {
	info := BridgeInfo{
		AgentID:   "agent-1",
		AgentName: "test-agent",
		Direction: flow.BridgeRequestReply,
		Connected: true,
		Stats: BridgeStatsSnapshot{
			MessagesRelayed:   100,
			MessagesFromAgent: 50,
			MessagesToAgent:   50,
		},
	}

	assert.Equal(t, "agent-1", info.AgentID)
	assert.Equal(t, "test-agent", info.AgentName)
	assert.Equal(t, flow.BridgeRequestReply, info.Direction)
	assert.True(t, info.Connected)
	assert.Equal(t, int64(100), info.Stats.MessagesRelayed)
}

// --- bridgeStatsCollector 테스트 ---

// TestNewBridgeStatsCollector_초기값 은 새로 생성된 수집기의 초기값이 0인지 확인한다.
func TestNewBridgeStatsCollector_초기값(t *testing.T) {
	collector := newBridgeStatsCollector()
	require.NotNil(t, collector)

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(0), snapshot.MessagesRelayed)
	assert.Equal(t, int64(0), snapshot.MessagesFromAgent)
	assert.Equal(t, int64(0), snapshot.MessagesToAgent)
	assert.Equal(t, int64(0), snapshot.TransformErrors)
	assert.Equal(t, int64(0), snapshot.CorrelationTimeouts)
	assert.Equal(t, 0, snapshot.PendingCorrelations)
	assert.Equal(t, time.Duration(0), snapshot.AvgRelayLatency)
	assert.True(t, snapshot.LastActivityAt.IsZero())
}

// TestBridgeStatsCollector_RecordRelay 는 릴레이 기록이 올바르게 집계되는지 확인한다.
func TestBridgeStatsCollector_RecordRelay(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordRelay(10 * time.Millisecond)
	collector.RecordRelay(20 * time.Millisecond)
	collector.RecordRelay(30 * time.Millisecond)

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(3), snapshot.MessagesRelayed)

	// 평균 지연시간: (10+20+30) / 3 = 20ms
	assert.Equal(t, 20*time.Millisecond, snapshot.AvgRelayLatency)

	// 마지막 활동 시각이 기록되어야 한다
	assert.False(t, snapshot.LastActivityAt.IsZero())
	assert.WithinDuration(t, time.Now(), snapshot.LastActivityAt, 1*time.Second)
}

// TestBridgeStatsCollector_RecordFromAgent 는 에이전트로부터 수신 기록이 올바르게 집계되는지 확인한다.
func TestBridgeStatsCollector_RecordFromAgent(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordFromAgent()
	collector.RecordFromAgent()

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(2), snapshot.MessagesFromAgent)
	assert.False(t, snapshot.LastActivityAt.IsZero())
}

// TestBridgeStatsCollector_RecordToAgent 는 에이전트로 전송 기록이 올바르게 집계되는지 확인한다.
func TestBridgeStatsCollector_RecordToAgent(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordToAgent()
	collector.RecordToAgent()
	collector.RecordToAgent()

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(3), snapshot.MessagesToAgent)
	assert.False(t, snapshot.LastActivityAt.IsZero())
}

// TestBridgeStatsCollector_RecordTransformError 는 변환 에러 기록이 올바르게 집계되는지 확인한다.
func TestBridgeStatsCollector_RecordTransformError(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordTransformError()

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(1), snapshot.TransformErrors)
}

// TestBridgeStatsCollector_RecordCorrelationTimeout 은 상관관계 타임아웃 기록이 올바르게 집계되는지 확인한다.
func TestBridgeStatsCollector_RecordCorrelationTimeout(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordCorrelationTimeout()
	collector.RecordCorrelationTimeout()

	snapshot := collector.Snapshot(0)
	assert.Equal(t, int64(2), snapshot.CorrelationTimeouts)
}

// TestBridgeStatsCollector_Snapshot_PendingCorrelations 는 Snapshot이 전달받은
// pendingCorrelations 값을 올바르게 포함하는지 확인한다.
func TestBridgeStatsCollector_Snapshot_PendingCorrelations(t *testing.T) {
	collector := newBridgeStatsCollector()

	snapshot := collector.Snapshot(42)
	assert.Equal(t, 42, snapshot.PendingCorrelations)
}

// TestBridgeStatsCollector_AvgRelayLatency_0건 은 릴레이가 0건일 때 평균 지연시간이 0인지 확인한다.
func TestBridgeStatsCollector_AvgRelayLatency_0건(t *testing.T) {
	collector := newBridgeStatsCollector()

	snapshot := collector.Snapshot(0)
	assert.Equal(t, time.Duration(0), snapshot.AvgRelayLatency)
}

// TestBridgeStatsCollector_AvgRelayLatency_1건 은 릴레이가 1건일 때 평균 지연시간이 정확한지 확인한다.
func TestBridgeStatsCollector_AvgRelayLatency_1건(t *testing.T) {
	collector := newBridgeStatsCollector()

	collector.RecordRelay(100 * time.Millisecond)

	snapshot := collector.Snapshot(0)
	assert.Equal(t, 100*time.Millisecond, snapshot.AvgRelayLatency)
}

// --- 동시성 테스트 ---

// TestBridgeStatsCollector_동시기록 은 여러 고루틴에서 동시에 기록해도 안전한지 확인한다.
func TestBridgeStatsCollector_동시기록(t *testing.T) {
	collector := newBridgeStatsCollector()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 5) // 5가지 Record 메서드

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			collector.RecordRelay(1 * time.Millisecond)
		}()
		go func() {
			defer wg.Done()
			collector.RecordFromAgent()
		}()
		go func() {
			defer wg.Done()
			collector.RecordToAgent()
		}()
		go func() {
			defer wg.Done()
			collector.RecordTransformError()
		}()
		go func() {
			defer wg.Done()
			collector.RecordCorrelationTimeout()
		}()
	}

	wg.Wait()

	snapshot := collector.Snapshot(5)
	assert.Equal(t, int64(goroutines), snapshot.MessagesRelayed)
	assert.Equal(t, int64(goroutines), snapshot.MessagesFromAgent)
	assert.Equal(t, int64(goroutines), snapshot.MessagesToAgent)
	assert.Equal(t, int64(goroutines), snapshot.TransformErrors)
	assert.Equal(t, int64(goroutines), snapshot.CorrelationTimeouts)
	assert.Equal(t, 5, snapshot.PendingCorrelations)
}

// TestBridgeStatsCollector_동시Snapshot 은 기록과 Snapshot이 동시에 호출되어도 안전한지 확인한다.
func TestBridgeStatsCollector_동시Snapshot(t *testing.T) {
	collector := newBridgeStatsCollector()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // Record + Snapshot

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			collector.RecordRelay(5 * time.Millisecond)
		}()
		go func() {
			defer wg.Done()
			snapshot := collector.Snapshot(0)
			// 릴레이 수는 0에서 goroutines 사이
			assert.GreaterOrEqual(t, snapshot.MessagesRelayed, int64(0))
			assert.LessOrEqual(t, snapshot.MessagesRelayed, int64(goroutines))
		}()
	}

	wg.Wait()
}

// TestBridgeStatsCollector_복합시나리오 는 여러 Record를 혼합하여 호출한 후 Snapshot이 정확한지 확인한다.
func TestBridgeStatsCollector_복합시나리오(t *testing.T) {
	collector := newBridgeStatsCollector()

	// 시나리오: 10건 릴레이, 5건 에이전트로부터, 5건 에이전트로, 2건 변환 에러, 1건 타임아웃
	for i := 0; i < 10; i++ {
		collector.RecordRelay(time.Duration(i+1) * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		collector.RecordFromAgent()
	}
	for i := 0; i < 5; i++ {
		collector.RecordToAgent()
	}
	collector.RecordTransformError()
	collector.RecordTransformError()
	collector.RecordCorrelationTimeout()

	snapshot := collector.Snapshot(3)

	assert.Equal(t, int64(10), snapshot.MessagesRelayed)
	assert.Equal(t, int64(5), snapshot.MessagesFromAgent)
	assert.Equal(t, int64(5), snapshot.MessagesToAgent)
	assert.Equal(t, int64(2), snapshot.TransformErrors)
	assert.Equal(t, int64(1), snapshot.CorrelationTimeouts)
	assert.Equal(t, 3, snapshot.PendingCorrelations)

	// 평균 지연시간: (1+2+3+4+5+6+7+8+9+10) / 10 = 5.5ms
	expectedAvg := time.Duration(5500) * time.Microsecond
	assert.Equal(t, expectedAvg, snapshot.AvgRelayLatency)

	assert.False(t, snapshot.LastActivityAt.IsZero())
}
