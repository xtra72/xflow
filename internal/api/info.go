package api

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ServerInfo 는 서버 상태의 런타임 스냅샷이다.
type ServerInfo struct {
	Version           string    `json:"version"`
	Uptime            string    `json:"uptime"`
	StartedAt         time.Time `json:"started_at"`
	ActiveConnections int64     `json:"active_connections"`
	RegisteredRoutes  int       `json:"registered_routes"`
	GoVersion         string    `json:"go_version"`
}

// ServerStats 는 요청 통계를 포함한다.
type ServerStats struct {
	TotalRequests int64                    `json:"total_requests"`
	ErrorCount    int64                    `json:"error_count"`
	ErrorRate     float64                  `json:"error_rate"`
	AvgLatencyMs  float64                  `json:"avg_latency_ms"`
	EndpointStats map[string]*EndpointStat `json:"endpoint_stats,omitempty"`
}

// EndpointStat 는 엔드포인트별 통계를 포함한다.
type EndpointStat struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Requests   int64   `json:"requests"`
	Errors     int64   `json:"errors"`
	AvgLatency float64 `json:"avg_latency_ms"`
}

// endpointStatCollector 는 엔드포인트별 통계를 수집하는 내부 타입이다.
type endpointStatCollector struct {
	method       string
	path         string
	requests     atomic.Int64
	errors       atomic.Int64
	totalLatency atomic.Int64 // 나노초 단위
}

// statsCollector 는 원자적 연산을 사용하여 스레드 안전하게 API 통계를 수집한다.
type statsCollector struct {
	totalRequests  atomic.Int64
	errorCount     atomic.Int64
	totalLatencyNs atomic.Int64
	activeConns    atomic.Int64
	startedAt      time.Time
	endpointStats  sync.Map // map[string]*endpointStatCollector
}

// newStatsCollector 는 새로운 statsCollector를 생성한다.
func newStatsCollector() *statsCollector {
	return &statsCollector{
		startedAt: time.Now(),
	}
}

// RecordRequest 는 완료된 요청을 기간과 에러 상태와 함께 기록한다.
func (sc *statsCollector) RecordRequest(method, path string, duration time.Duration, isError bool) {
	sc.totalRequests.Add(1)
	sc.totalLatencyNs.Add(int64(duration))

	if isError {
		sc.errorCount.Add(1)
	}

	// 엔드포인트별 통계 업데이트
	key := fmt.Sprintf("%s %s", method, path)
	val, _ := sc.endpointStats.LoadOrStore(key, &endpointStatCollector{
		method: method,
		path:   path,
	})
	epStat := val.(*endpointStatCollector)
	epStat.requests.Add(1)
	epStat.totalLatency.Add(int64(duration))
	if isError {
		epStat.errors.Add(1)
	}
}

// IncrementConnections 는 활성 연결 수를 증가시킨다.
func (sc *statsCollector) IncrementConnections() {
	sc.activeConns.Add(1)
}

// DecrementConnections 는 활성 연결 수를 감소시킨다.
func (sc *statsCollector) DecrementConnections() {
	sc.activeConns.Add(-1)
}

// ActiveConnections 는 현재 활성 연결 수를 반환한다.
func (sc *statsCollector) ActiveConnections() int64 {
	return sc.activeConns.Load()
}

// StartedAt 은 statsCollector가 생성된 시각을 반환한다.
func (sc *statsCollector) StartedAt() time.Time {
	return sc.startedAt
}

// Snapshot 은 읽기 전용 ServerStats 스냅샷을 반환한다.
func (sc *statsCollector) Snapshot() ServerStats {
	total := sc.totalRequests.Load()
	errCount := sc.errorCount.Load()
	totalLatency := sc.totalLatencyNs.Load()

	var errorRate float64
	var avgLatencyMs float64

	if total > 0 {
		errorRate = float64(errCount) / float64(total)
		avgLatencyMs = float64(totalLatency) / float64(total) / float64(time.Millisecond)
	}

	endpointStats := make(map[string]*EndpointStat)
	sc.endpointStats.Range(func(key, value any) bool {
		k := key.(string)
		v := value.(*endpointStatCollector)

		reqs := v.requests.Load()
		var avgLat float64
		if reqs > 0 {
			avgLat = float64(v.totalLatency.Load()) / float64(reqs) / float64(time.Millisecond)
		}

		endpointStats[k] = &EndpointStat{
			Method:     v.method,
			Path:       v.path,
			Requests:   reqs,
			Errors:     v.errors.Load(),
			AvgLatency: avgLat,
		}
		return true
	})

	return ServerStats{
		TotalRequests: total,
		ErrorCount:    errCount,
		ErrorRate:     errorRate,
		AvgLatencyMs:  avgLatencyMs,
		EndpointStats: endpointStats,
	}
}
