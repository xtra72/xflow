package node

import (
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/flow"
)

// BridgeInfo 는 BridgeNode의 런타임 상태 정보를 나타내는 구조체이다.
// 외부에서 읽기 전용으로 사용되며, BridgeNode의 현재 상태를 스냅샷으로 제공한다.
type BridgeInfo struct {
	AgentID   string              // 연결된 에이전트 ID
	AgentName string              // 연결된 에이전트 이름
	Direction flow.BridgeDirection // 브릿지 방향
	Connected bool                // 연결 상태
	Stats     BridgeStatsSnapshot // 통계 스냅샷
}

// BridgeStatsSnapshot 은 BridgeNode의 통계 데이터 스냅샷이다.
// bridgeStatsCollector의 atomic 값을 안전하게 읽어 생성된다.
type BridgeStatsSnapshot struct {
	MessagesRelayed     int64         // 릴레이된 총 메시지 수
	MessagesFromAgent   int64         // 에이전트로부터 수신한 메시지 수
	MessagesToAgent     int64         // 에이전트로 전송한 메시지 수
	TransformErrors     int64         // 변환 에러 누적 횟수
	CorrelationTimeouts int64         // 상관관계 타임아웃 누적 횟수
	PendingCorrelations int           // 현재 대기 중인 상관관계 수
	AvgRelayLatency     time.Duration // 평균 릴레이 지연시간
	LastActivityAt      time.Time     // 마지막 활동 시각
}

// bridgeStatsCollector 는 BridgeNode의 통계 데이터를 원자적으로 수집하는 내부 구조체이다.
// 모든 필드는 atomic 타입이므로 별도의 뮤텍스 없이 동시 접근이 안전하다.
type bridgeStatsCollector struct {
	messagesRelayed     atomic.Int64 // 릴레이된 총 메시지 수
	messagesFromAgent   atomic.Int64 // 에이전트로부터 수신한 메시지 수
	messagesToAgent     atomic.Int64 // 에이전트로 전송한 메시지 수
	transformErrors     atomic.Int64 // 변환 에러 누적 횟수
	correlationTimeouts atomic.Int64 // 상관관계 타임아웃 누적 횟수
	totalLatency        atomic.Int64 // 총 지연시간 (나노초)
	lastActivityAt      atomic.Int64 // 마지막 활동 시각 (unix 나노초)
}

// newBridgeStatsCollector 는 새로운 bridgeStatsCollector를 생성하여 반환한다.
func newBridgeStatsCollector() *bridgeStatsCollector {
	return &bridgeStatsCollector{}
}

// RecordRelay 는 메시지 릴레이를 기록한다.
// 릴레이 횟수를 증가시키고 지연시간을 누적하며, 마지막 활동 시각을 갱신한다.
func (s *bridgeStatsCollector) RecordRelay(latency time.Duration) {
	s.messagesRelayed.Add(1)
	s.totalLatency.Add(int64(latency))
	s.lastActivityAt.Store(time.Now().UnixNano())
}

// RecordFromAgent 는 에이전트로부터 메시지 수신을 기록한다.
func (s *bridgeStatsCollector) RecordFromAgent() {
	s.messagesFromAgent.Add(1)
	s.lastActivityAt.Store(time.Now().UnixNano())
}

// RecordToAgent 는 에이전트로 메시지 전송을 기록한다.
func (s *bridgeStatsCollector) RecordToAgent() {
	s.messagesToAgent.Add(1)
	s.lastActivityAt.Store(time.Now().UnixNano())
}

// RecordTransformError 는 변환 에러를 기록한다.
func (s *bridgeStatsCollector) RecordTransformError() {
	s.transformErrors.Add(1)
}

// RecordCorrelationTimeout 은 상관관계 타임아웃을 기록한다.
func (s *bridgeStatsCollector) RecordCorrelationTimeout() {
	s.correlationTimeouts.Add(1)
}

// Snapshot 은 현재까지의 통계를 읽기 전용 스냅샷으로 반환한다.
// pendingCorrelations 는 외부에서 전달받는 현재 대기 중인 상관관계 수이다.
func (s *bridgeStatsCollector) Snapshot(pendingCorrelations int) BridgeStatsSnapshot {
	relayed := s.messagesRelayed.Load()
	totalLatency := s.totalLatency.Load()
	lastActivity := s.lastActivityAt.Load()

	var avgLatency time.Duration
	if relayed > 0 {
		avgLatency = time.Duration(totalLatency / relayed)
	}

	var lastActivityTime time.Time
	if lastActivity > 0 {
		lastActivityTime = time.Unix(0, lastActivity)
	}

	return BridgeStatsSnapshot{
		MessagesRelayed:     relayed,
		MessagesFromAgent:   s.messagesFromAgent.Load(),
		MessagesToAgent:     s.messagesToAgent.Load(),
		TransformErrors:     s.transformErrors.Load(),
		CorrelationTimeouts: s.correlationTimeouts.Load(),
		PendingCorrelations: pendingCorrelations,
		AvgRelayLatency:     avgLatency,
		LastActivityAt:      lastActivityTime,
	}
}
