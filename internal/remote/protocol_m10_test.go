// protocol_m10_test.go 는 M10(그룹 L) 원격 노드 대시보드 패리티 프로토콜 확장을
// 검증한다(@SPEC:SPEC-REMOTE-001 M10, REQ-L01/L05/L06/L07, spec §5.1/§5.10).
//
// M10 은 M8(그룹 J) query/stream 프록시를 확장한다(신규 메시지 타입 없음 — REQ-N04):
//   - query-action allowlist: dashboard.{get_shared,get_mine}(READ-ONLY), monitor.metrics
//   - stream-action allowlist: chart.chart, monitor.logs
//   - 변경 의미 action(dashboard.put/delete 등)은 명시 배제(READ-ONLY — REQ-J03/L01).
package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestM10DomainConstants 는 M10 신규 도메인 상수를 검증한다(REQ-L01/L05/L06/L07).
func TestM10DomainConstants(t *testing.T) {
	assert.Equal(t, "dashboard", DomainDashboard)
	assert.Equal(t, "monitor", DomainMonitor)
	assert.Equal(t, "chart", DomainChart)
}

// TestM10QueryActionConstants 는 M10 신규 query-action 상수를 검증한다(REQ-L01/L05).
func TestM10QueryActionConstants(t *testing.T) {
	assert.Equal(t, "get_shared", QueryActionGetShared)
	assert.Equal(t, "get_mine", QueryActionGetMine)
	assert.Equal(t, "metrics", QueryActionMetrics)
}

// TestM10StreamActionConstants 는 M10 신규 stream-action 상수를 검증한다(REQ-L06/L07).
func TestM10StreamActionConstants(t *testing.T) {
	assert.Equal(t, "chart", StreamActionChart)
	assert.Equal(t, "logs", StreamActionLogs)
}

// TestM10DashboardQueryAllowlist 는 dashboard 도메인 read query-action allowlist 를
// 검증한다(REQ-L01 — get_shared/get_mine 만 허용, 변경 의미 action 배제).
func TestM10DashboardQueryAllowlist(t *testing.T) {
	// 허용: get_shared, get_mine (READ-ONLY).
	assert.True(t, IsAllowedQueryAction(DomainDashboard, QueryActionGetShared))
	assert.True(t, IsAllowedQueryAction(DomainDashboard, QueryActionGetMine))

	// 변경 의미 action 명시 배제(원격 config 편집 비목표 — REQ-J03/L01/L12).
	for _, mut := range []string{"put", "delete", "create", "update", "save"} {
		assert.Falsef(t, IsAllowedQueryAction(DomainDashboard, mut),
			"dashboard.%s 변경 action 은 allowlist 에서 배제되어야 함(READ-ONLY)", mut)
	}

	// 미열거 read action 도 거부(엄격 allowlist).
	assert.False(t, IsAllowedQueryAction(DomainDashboard, QueryActionGet))
	assert.False(t, IsAllowedQueryAction(DomainDashboard, QueryActionList))
}

// TestM10MonitorMetricsQueryAllowlist 는 monitor.metrics query-action allowlist 를
// 검증한다(REQ-L05 — 온디맨드 read, 캐시 대상).
func TestM10MonitorMetricsQueryAllowlist(t *testing.T) {
	assert.True(t, IsAllowedQueryAction(DomainMonitor, QueryActionMetrics))

	// monitor.logs 는 스트림 action 이지 query-action 이 아니다(REQ-L06).
	assert.False(t, IsAllowedQueryAction(DomainMonitor, StreamActionLogs))
	// 변경 의미 action 배제.
	assert.False(t, IsAllowedQueryAction(DomainMonitor, "set"))
}

// TestM10ChartStreamAllowlist 는 chart stream-action allowlist 를 검증한다(REQ-L07).
// chart 는 라이브 스트림이므로 캐시 우회 대상이다(REQ-J16).
func TestM10ChartStreamAllowlist(t *testing.T) {
	assert.True(t, IsStreamableAction(DomainChart, StreamActionChart))

	// chart 는 query-action 이 아니다(스트림 전용 — 폴링 폴백은 프론트 관심사).
	assert.False(t, IsAllowedQueryAction(DomainChart, StreamActionChart))
}

// TestM10MonitorLogsStreamAllowlist 는 monitor.logs stream-action allowlist 를
// 검증한다(REQ-L06 — 라이브 로그 tail, 캐시 우회).
func TestM10MonitorLogsStreamAllowlist(t *testing.T) {
	assert.True(t, IsStreamableAction(DomainMonitor, StreamActionLogs))

	// monitor.metrics 는 스트림이 아니다(query-action — 캐시 대상).
	assert.False(t, IsStreamableAction(DomainMonitor, QueryActionMetrics))
}

// TestM10DashboardMetricsNotStreamable 는 dashboard/metrics 가 스트림 가능하지
// 않음(완만 변동 — 캐시 대상)을 검증한다(REQ-J16/L02).
func TestM10DashboardMetricsNotStreamable(t *testing.T) {
	assert.False(t, IsStreamableAction(DomainDashboard, QueryActionGetShared))
	assert.False(t, IsStreamableAction(DomainDashboard, QueryActionGetMine))
	assert.False(t, IsStreamableAction(DomainMonitor, QueryActionMetrics))
}
