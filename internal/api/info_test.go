package api

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerInfo_Fields(t *testing.T) {
	now := time.Now()
	info := ServerInfo{
		Version:           "1.0.0",
		Uptime:            "1h30m",
		StartedAt:         now,
		ActiveConnections: 10,
		RegisteredRoutes:  5,
		GoVersion:         runtime.Version(),
	}

	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "1h30m", info.Uptime)
	assert.Equal(t, now, info.StartedAt)
	assert.Equal(t, int64(10), info.ActiveConnections)
	assert.Equal(t, 5, info.RegisteredRoutes)
	assert.Equal(t, runtime.Version(), info.GoVersion)
}

func TestNewStatsCollector(t *testing.T) {
	sc := newStatsCollector()
	require.NotNil(t, sc)

	// 초기 상태 확인
	snap := sc.Snapshot()
	assert.Equal(t, int64(0), snap.TotalRequests)
	assert.Equal(t, int64(0), snap.ErrorCount)
	assert.Equal(t, float64(0), snap.ErrorRate)
	assert.Equal(t, float64(0), snap.AvgLatencyMs)
}

func TestStatsCollector_RecordRequest(t *testing.T) {
	tests := []struct {
		name            string
		requests        []struct {
			method  string
			path    string
			dur     time.Duration
			isError bool
		}
		expectedTotal  int64
		expectedErrors int64
	}{
		{
			name: "single successful request",
			requests: []struct {
				method  string
				path    string
				dur     time.Duration
				isError bool
			}{
				{"GET", "/api/flows", 100 * time.Millisecond, false},
			},
			expectedTotal:  1,
			expectedErrors: 0,
		},
		{
			name: "single error request",
			requests: []struct {
				method  string
				path    string
				dur     time.Duration
				isError bool
			}{
				{"POST", "/api/flows", 200 * time.Millisecond, true},
			},
			expectedTotal:  1,
			expectedErrors: 1,
		},
		{
			name: "mixed requests",
			requests: []struct {
				method  string
				path    string
				dur     time.Duration
				isError bool
			}{
				{"GET", "/api/flows", 50 * time.Millisecond, false},
				{"POST", "/api/flows", 100 * time.Millisecond, false},
				{"GET", "/api/flows/1", 150 * time.Millisecond, true},
				{"DELETE", "/api/flows/1", 75 * time.Millisecond, false},
			},
			expectedTotal:  4,
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := newStatsCollector()
			for _, r := range tt.requests {
				sc.RecordRequest(r.method, r.path, r.dur, r.isError)
			}

			snap := sc.Snapshot()
			assert.Equal(t, tt.expectedTotal, snap.TotalRequests)
			assert.Equal(t, tt.expectedErrors, snap.ErrorCount)
		})
	}
}

func TestStatsCollector_ErrorRate(t *testing.T) {
	sc := newStatsCollector()

	// 에러율 0% (요청 없음)
	snap := sc.Snapshot()
	assert.Equal(t, float64(0), snap.ErrorRate)

	// 에러율 50%
	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, false)
	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, true)

	snap = sc.Snapshot()
	assert.InDelta(t, 0.5, snap.ErrorRate, 0.001)
}

func TestStatsCollector_AvgLatency(t *testing.T) {
	sc := newStatsCollector()

	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, false)
	sc.RecordRequest("GET", "/api/flows", 200*time.Millisecond, false)
	sc.RecordRequest("GET", "/api/flows", 300*time.Millisecond, false)

	snap := sc.Snapshot()
	// 평균: (100+200+300)/3 = 200ms
	assert.InDelta(t, 200.0, snap.AvgLatencyMs, 1.0)
}

func TestStatsCollector_Connections(t *testing.T) {
	sc := newStatsCollector()

	sc.IncrementConnections()
	sc.IncrementConnections()
	sc.IncrementConnections()

	assert.Equal(t, int64(3), sc.ActiveConnections())

	sc.DecrementConnections()

	assert.Equal(t, int64(2), sc.ActiveConnections())
}

func TestStatsCollector_EndpointStats(t *testing.T) {
	sc := newStatsCollector()

	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, false)
	sc.RecordRequest("GET", "/api/flows", 200*time.Millisecond, false)
	sc.RecordRequest("POST", "/api/flows", 150*time.Millisecond, true)
	sc.RecordRequest("GET", "/api/agents", 50*time.Millisecond, false)

	snap := sc.Snapshot()

	require.NotNil(t, snap.EndpointStats)

	// "GET /api/flows" 엔드포인트 확인
	flowStat, exists := snap.EndpointStats["GET /api/flows"]
	require.True(t, exists)
	assert.Equal(t, "GET", flowStat.Method)
	assert.Equal(t, "/api/flows", flowStat.Path)
	assert.Equal(t, int64(2), flowStat.Requests)
	assert.Equal(t, int64(0), flowStat.Errors)
	assert.InDelta(t, 150.0, flowStat.AvgLatency, 1.0)

	// "POST /api/flows" 엔드포인트 확인
	postStat, exists := snap.EndpointStats["POST /api/flows"]
	require.True(t, exists)
	assert.Equal(t, int64(1), postStat.Requests)
	assert.Equal(t, int64(1), postStat.Errors)

	// "GET /api/agents" 엔드포인트 확인
	agentStat, exists := snap.EndpointStats["GET /api/agents"]
	require.True(t, exists)
	assert.Equal(t, int64(1), agentStat.Requests)
}

func TestStatsCollector_ConcurrentAccess(t *testing.T) {
	sc := newStatsCollector()

	const numGoroutines = 100
	const requestsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				isErr := j%10 == 0 // 10% 에러율
				sc.RecordRequest("GET", "/api/flows", time.Duration(j)*time.Millisecond, isErr)
			}
		}(i)
	}

	// 동시 연결 증감
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			sc.IncrementConnections()
			sc.DecrementConnections()
		}()
	}

	wg.Wait()

	snap := sc.Snapshot()
	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	assert.Equal(t, expectedTotal, snap.TotalRequests)

	// 에러 수: 각 고루틴에서 50개 중 j%10==0인 것 (j=0,10,20,30,40) = 5개, 100개 고루틴
	expectedErrors := int64(numGoroutines * 5)
	assert.Equal(t, expectedErrors, snap.ErrorCount)

	// 연결은 모두 증가 후 감소되었으므로 0이어야 한다
	assert.Equal(t, int64(0), sc.ActiveConnections())
}

func TestStatsCollector_SnapshotIsImmutable(t *testing.T) {
	sc := newStatsCollector()

	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, false)
	snap1 := sc.Snapshot()

	sc.RecordRequest("GET", "/api/flows", 100*time.Millisecond, false)
	snap2 := sc.Snapshot()

	// snap1은 변경되지 않아야 한다
	assert.Equal(t, int64(1), snap1.TotalRequests)
	assert.Equal(t, int64(2), snap2.TotalRequests)
}

func TestStatsCollector_StartedAt(t *testing.T) {
	before := time.Now()
	sc := newStatsCollector()
	after := time.Now()

	assert.False(t, sc.StartedAt().Before(before))
	assert.False(t, sc.StartedAt().After(after))
}

func TestServerStats_Fields(t *testing.T) {
	stats := ServerStats{
		TotalRequests: 100,
		ErrorCount:    5,
		ErrorRate:     0.05,
		AvgLatencyMs:  150.5,
		EndpointStats: map[string]*EndpointStat{
			"GET /api/flows": {
				Method:     "GET",
				Path:       "/api/flows",
				Requests:   50,
				Errors:     2,
				AvgLatency: 120.0,
			},
		},
	}

	assert.Equal(t, int64(100), stats.TotalRequests)
	assert.Equal(t, int64(5), stats.ErrorCount)
	assert.InDelta(t, 0.05, stats.ErrorRate, 0.001)
	assert.InDelta(t, 150.5, stats.AvgLatencyMs, 0.1)
	assert.Len(t, stats.EndpointStats, 1)
}

func TestEndpointStat_Fields(t *testing.T) {
	stat := EndpointStat{
		Method:     "POST",
		Path:       "/api/agents",
		Requests:   25,
		Errors:     3,
		AvgLatency: 200.5,
	}

	assert.Equal(t, "POST", stat.Method)
	assert.Equal(t, "/api/agents", stat.Path)
	assert.Equal(t, int64(25), stat.Requests)
	assert.Equal(t, int64(3), stat.Errors)
	assert.InDelta(t, 200.5, stat.AvgLatency, 0.1)
}
