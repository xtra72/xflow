package agent

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// AgentInfo is a snapshot of an agent's comprehensive state.
type AgentInfo struct {
	ID         string          // Unique agent identifier
	Name       string          // Human-readable agent name
	Type       string          // Agent type
	State      lifecycle.State // Current lifecycle state
	Health     HealthStatus    // Current health status
	Config     AgentConfig     // Current configuration snapshot
	Stats      StatsSnapshot   // Processing statistics snapshot
	SharedInfo *SharedInfo     // Shared reference info (nil if not shared)
	StartedAt  time.Time       // Last start time (zero if never started)
	Uptime     time.Duration   // Current uptime duration
	CreatedAt  time.Time       // Agent creation time
}

// StatsSnapshot은 에이전트 처리 통계의 불변 스냅샷이다.
type StatsSnapshot struct {
	MessagesReceived     int64         // 총 수신 메시지 수
	MessagesSent         int64         // 총 발신 메시지 수
	MessagesErrored      int64         // 처리 오류가 발생한 메시지 수
	BytesRead            int64         // 트랜스포트에서 읽은 총 바이트 수
	BytesWritten         int64         // 트랜스포트에 쓴 총 바이트 수
	LastActivityAt       time.Time     // 마지막 데이터 활동 시각
	AvgProcessingLatency time.Duration // 평균 메시지 처리 레이턴시
	RestartCount         int64         // 재시작 횟수
	MsgBufferPending     int           `json:"msg_buffer_pending"`  // 버퍼에 대기 중인 메시지 수
	MsgBufferCapacity    int           `json:"msg_buffer_capacity"` // 메시지 버퍼 전체 용량
	Extra                map[string]any `json:"extra,omitempty"`     // 에이전트별 추가 통계

	// 외부(External) 메시지 통계 - 외부 디바이스/시스템과의 통신
	ExternalMessagesReceived int64 `json:"external_messages_received"` // 외부 수신 메시지 수
	ExternalMessagesSent     int64 `json:"external_messages_sent"`     // 외부 발신 메시지 수
	ExternalMessagesErrored  int64 `json:"external_messages_errored"`  // 외부 메시지 오류 수

	// 내부(Internal) 메시지 통계 - 내부 플로우/노드 간 통신
	InternalMessagesReceived int64 `json:"internal_messages_received"` // 내부 수신 메시지 수
	InternalMessagesSent     int64 `json:"internal_messages_sent"`     // 내부 발신 메시지 수
	InternalMessagesErrored  int64 `json:"internal_messages_errored"`  // 내부 메시지 오류 수

	// 운영 카운터
	DroppedMessages int64         `json:"dropped_messages"` // 드롭된 메시지 수
	LoadTime        time.Duration `json:"load_time"`        // 에이전트 시작부터 첫 메시지까지의 시간

	// 노드 참조별 통계 스냅샷
	NodeRefs []NodeRefStats `json:"node_refs,omitempty"`
}

// SharedInfo holds shared reference information for an agent.
type SharedInfo struct {
	RefCount int32    // Current reference count
	Flows    []string // Flow IDs currently referencing this agent
}

// ManagerSummary provides an aggregate overview of all managed agents.
type ManagerSummary struct {
	TotalAgents            int         // Total number of agents
	RunningAgents          int         // Agents in Running state
	PausedAgents           int         // Agents in Paused state
	StoppedAgents          int         // Agents in Stopped state
	ErrorAgents            int         // Agents in Error state
	HealthyAgents          int         // Agents with Healthy health state
	UnhealthyAgents        int         // Agents with Unhealthy health state
	TotalMessagesProcessed int64       // Total messages processed across all agents
	TotalErrors            int64       // Total errors across all agents
	Agents                 []AgentInfo // Individual agent information
}

// NodeRefStats 는 에이전트에 연결된 개별 노드의 통계 스냅샷이다.
type NodeRefStats struct {
	NodeID           string    `json:"node_id"`           // 노드 식별자
	FlowID           string    `json:"flow_id"`           // 플로우 식별자
	MessagesReceived int64     `json:"messages_received"` // 노드로부터 수신한 메시지 수
	MessagesSent     int64     `json:"messages_sent"`     // 노드로 송신한 메시지 수
	MessagesErrored  int64     `json:"messages_errored"`  // 에러 메시지 수
	LastActivityAt   time.Time `json:"last_activity_at"`  // 마지막 활동 시각
}

// nodeRefStatsEntry 는 노드별 통계의 내부 변경 가능 항목이다.
type nodeRefStatsEntry struct {
	flowID           string
	messagesReceived atomic.Int64
	messagesSent     atomic.Int64
	messagesErrored  atomic.Int64
	lastActivityAt   time.Time
	mu               sync.Mutex // lastActivityAt 보호
}

// AgentStats는 에이전트 처리 통계를 위한 스레드 세이프 atomic 카운터를 제공한다.
type AgentStats struct {
	messagesReceived     atomic.Int64
	messagesSent         atomic.Int64
	messagesErrored      atomic.Int64
	bytesRead            atomic.Int64
	bytesWritten         atomic.Int64
	restartCount         atomic.Int64
	avgProcessingLatency atomic.Int64 // 나노초 단위로 저장
	mu                   sync.Mutex
	lastActivityAt       time.Time

	// 외부/내부 메시지 분류 카운터
	externalMessagesReceived atomic.Int64
	externalMessagesSent     atomic.Int64
	externalMessagesErrored  atomic.Int64
	internalMessagesReceived atomic.Int64
	internalMessagesSent     atomic.Int64
	internalMessagesErrored  atomic.Int64

	// 운영 카운터
	droppedMessages atomic.Int64
	loadTime        atomic.Int64 // 나노초 단위로 저장
	loadTimeSet     atomic.Bool  // LoadTime이 한 번만 설정되도록 보장
	startedAt       atomic.Int64 // UnixNano로 저장, 에이전트 시작 시각

	// 노드별 통계
	nodeRefsMu sync.RWMutex
	nodeRefs   map[string]*nodeRefStatsEntry // key: nodeID
}

// NewAgentStats creates a new AgentStats instance with all counters at zero.
func NewAgentStats() *AgentStats {
	return &AgentStats{
		nodeRefs: make(map[string]*nodeRefStatsEntry),
	}
}

// IncrMessagesReceived atomically increments the messages received counter.
func (s *AgentStats) IncrMessagesReceived() {
	s.messagesReceived.Add(1)
}

// IncrMessagesSent atomically increments the messages sent counter.
func (s *AgentStats) IncrMessagesSent() {
	s.messagesSent.Add(1)
}

// AddMessagesSent atomically adds n to the messages sent counter.
func (s *AgentStats) AddMessagesSent(n int64) {
	s.messagesSent.Add(n)
}

// IncrMessagesErrored atomically increments the messages errored counter.
func (s *AgentStats) IncrMessagesErrored() {
	s.messagesErrored.Add(1)
}

// AddBytesRead atomically adds n bytes to the bytes read counter.
func (s *AgentStats) AddBytesRead(n int64) {
	s.bytesRead.Add(n)
}

// AddBytesWritten atomically adds n bytes to the bytes written counter.
func (s *AgentStats) AddBytesWritten(n int64) {
	s.bytesWritten.Add(n)
}

// UpdateLastActivity updates the last activity timestamp to now.
func (s *AgentStats) UpdateLastActivity() {
	s.mu.Lock()
	s.lastActivityAt = time.Now()
	s.mu.Unlock()
}

// IncrRestartCount atomically increments the restart counter.
func (s *AgentStats) IncrRestartCount() {
	s.restartCount.Add(1)
}

// MessagesReceived returns the current messages received count.
func (s *AgentStats) MessagesReceived() int64 {
	return s.messagesReceived.Load()
}

// MessagesSent returns the current messages sent count.
func (s *AgentStats) MessagesSent() int64 {
	return s.messagesSent.Load()
}

// MessagesErrored returns the current messages errored count.
func (s *AgentStats) MessagesErrored() int64 {
	return s.messagesErrored.Load()
}

// BytesRead returns the current bytes read count.
func (s *AgentStats) BytesRead() int64 {
	return s.bytesRead.Load()
}

// BytesWritten returns the current bytes written count.
func (s *AgentStats) BytesWritten() int64 {
	return s.bytesWritten.Load()
}

// LastActivityAt returns the time of the last activity.
func (s *AgentStats) LastActivityAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivityAt
}

// AvgProcessingLatency returns the average processing latency.
func (s *AgentStats) AvgProcessingLatency() time.Duration {
	return time.Duration(s.avgProcessingLatency.Load())
}

// RestartCount returns the current restart count.
func (s *AgentStats) RestartCount() int64 {
	return s.restartCount.Load()
}

// IncrExternalMessagesReceived는 외부 수신 카운터와 총 수신 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrExternalMessagesReceived() {
	s.externalMessagesReceived.Add(1)
	s.messagesReceived.Add(1)
}

// IncrExternalMessagesSent는 외부 발신 카운터와 총 발신 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrExternalMessagesSent() {
	s.externalMessagesSent.Add(1)
	s.messagesSent.Add(1)
}

// IncrExternalMessagesErrored는 외부 오류 카운터와 총 오류 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrExternalMessagesErrored() {
	s.externalMessagesErrored.Add(1)
	s.messagesErrored.Add(1)
}

// IncrInternalMessagesReceived는 내부 수신 카운터와 총 수신 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrInternalMessagesReceived() {
	s.internalMessagesReceived.Add(1)
	s.messagesReceived.Add(1)
}

// IncrInternalMessagesSent는 내부 발신 카운터와 총 발신 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrInternalMessagesSent() {
	s.internalMessagesSent.Add(1)
	s.messagesSent.Add(1)
}

// AddInternalMessagesSent는 내부 발신 카운터와 총 발신 카운터를 n만큼 증가시킨다.
func (s *AgentStats) AddInternalMessagesSent(n int64) {
	s.internalMessagesSent.Add(n)
	s.messagesSent.Add(n)
}

// IncrInternalMessagesErrored는 내부 오류 카운터와 총 오류 카운터를 모두 증가시킨다.
func (s *AgentStats) IncrInternalMessagesErrored() {
	s.internalMessagesErrored.Add(1)
	s.messagesErrored.Add(1)
}

// IncrDroppedMessages는 드롭된 메시지 카운터를 증가시킨다.
func (s *AgentStats) IncrDroppedMessages() {
	s.droppedMessages.Add(1)
}

// SetLoadTime은 로드 시간을 설정한다. 이미 설정된 경우 무시한다.
func (s *AgentStats) SetLoadTime(d time.Duration) {
	if s.loadTimeSet.CompareAndSwap(false, true) {
		s.loadTime.Store(int64(d))
	}
}

// SetStartedAt은 에이전트 시작 시각을 저장한다. LoadTime 계산에 사용된다.
func (s *AgentStats) SetStartedAt(t time.Time) {
	s.startedAt.Store(t.UnixNano())
}

// RecordFirstMessage는 startedAt 기준으로 LoadTime을 계산하고 설정한다.
// 이미 LoadTime이 설정된 경우 무시한다.
func (s *AgentStats) RecordFirstMessage() {
	startNano := s.startedAt.Load()
	if startNano == 0 {
		return
	}
	startTime := time.Unix(0, startNano)
	d := time.Since(startTime)
	s.SetLoadTime(d)
}

// DroppedMessages는 현재 드롭된 메시지 수를 반환한다.
func (s *AgentStats) DroppedMessages() int64 {
	return s.droppedMessages.Load()
}

// LoadTime은 에이전트 로드 시간을 반환한다.
func (s *AgentStats) LoadTime() time.Duration {
	return time.Duration(s.loadTime.Load())
}

// IncrNodeRefReceived 는 지정 노드의 수신 카운터를 증가시킨다.
func (s *AgentStats) IncrNodeRefReceived(nodeID, flowID string) {
	entry := s.getOrCreateNodeRef(nodeID, flowID)
	entry.messagesReceived.Add(1)
	entry.mu.Lock()
	entry.lastActivityAt = time.Now()
	entry.mu.Unlock()
}

// IncrNodeRefSent 는 지정 노드의 송신 카운터를 증가시킨다.
func (s *AgentStats) IncrNodeRefSent(nodeID, flowID string) {
	entry := s.getOrCreateNodeRef(nodeID, flowID)
	entry.messagesSent.Add(1)
	entry.mu.Lock()
	entry.lastActivityAt = time.Now()
	entry.mu.Unlock()
}

// IncrNodeRefErrored 는 지정 노드의 에러 카운터를 증가시킨다.
func (s *AgentStats) IncrNodeRefErrored(nodeID, flowID string) {
	entry := s.getOrCreateNodeRef(nodeID, flowID)
	entry.messagesErrored.Add(1)
	entry.mu.Lock()
	entry.lastActivityAt = time.Now()
	entry.mu.Unlock()
}

// NodeRefStatsSnapshot 는 모든 노드 참조의 읽기 전용 스냅샷을 반환한다.
func (s *AgentStats) NodeRefStatsSnapshot() []NodeRefStats {
	s.nodeRefsMu.RLock()
	defer s.nodeRefsMu.RUnlock()

	result := make([]NodeRefStats, 0, len(s.nodeRefs))
	for nodeID, entry := range s.nodeRefs {
		entry.mu.Lock()
		lastActivity := entry.lastActivityAt
		entry.mu.Unlock()

		result = append(result, NodeRefStats{
			NodeID:           nodeID,
			FlowID:           entry.flowID,
			MessagesReceived: entry.messagesReceived.Load(),
			MessagesSent:     entry.messagesSent.Load(),
			MessagesErrored:  entry.messagesErrored.Load(),
			LastActivityAt:   lastActivity,
		})
	}
	return result
}

// getOrCreateNodeRef 는 노드 통계 항목을 가져오거나 새로 생성한다.
func (s *AgentStats) getOrCreateNodeRef(nodeID, flowID string) *nodeRefStatsEntry {
	s.nodeRefsMu.RLock()
	entry, ok := s.nodeRefs[nodeID]
	s.nodeRefsMu.RUnlock()
	if ok {
		return entry
	}

	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()
	// double-check: write lock 획득 후 재확인
	if entry, ok := s.nodeRefs[nodeID]; ok {
		return entry
	}
	entry = &nodeRefStatsEntry{flowID: flowID}
	s.nodeRefs[nodeID] = entry
	return entry
}

// ResetStats는 모든 카운터를 0으로 초기화한다.
func (s *AgentStats) ResetStats() {
	s.messagesReceived.Store(0)
	s.messagesSent.Store(0)
	s.messagesErrored.Store(0)
	s.bytesRead.Store(0)
	s.bytesWritten.Store(0)
	s.restartCount.Store(0)
	s.avgProcessingLatency.Store(0)
	s.mu.Lock()
	s.lastActivityAt = time.Time{}
	s.mu.Unlock()

	// 외부/내부 메시지 카운터 초기화
	s.externalMessagesReceived.Store(0)
	s.externalMessagesSent.Store(0)
	s.externalMessagesErrored.Store(0)
	s.internalMessagesReceived.Store(0)
	s.internalMessagesSent.Store(0)
	s.internalMessagesErrored.Store(0)

	// 운영 카운터 초기화
	s.droppedMessages.Store(0)
	s.loadTime.Store(0)
	s.loadTimeSet.Store(false)
	s.startedAt.Store(0)

	// 노드별 통계 초기화
	s.nodeRefsMu.Lock()
	s.nodeRefs = make(map[string]*nodeRefStatsEntry)
	s.nodeRefsMu.Unlock()
}

// Snapshot returns an immutable snapshot of the current statistics.
func (s *AgentStats) Snapshot() StatsSnapshot {
	s.mu.Lock()
	lastActivity := s.lastActivityAt
	s.mu.Unlock()

	snap := StatsSnapshot{
		MessagesReceived:     s.messagesReceived.Load(),
		MessagesSent:         s.messagesSent.Load(),
		MessagesErrored:      s.messagesErrored.Load(),
		BytesRead:            s.bytesRead.Load(),
		BytesWritten:         s.bytesWritten.Load(),
		LastActivityAt:       lastActivity,
		AvgProcessingLatency: time.Duration(s.avgProcessingLatency.Load()),
		RestartCount:         s.restartCount.Load(),

		ExternalMessagesReceived: s.externalMessagesReceived.Load(),
		ExternalMessagesSent:     s.externalMessagesSent.Load(),
		ExternalMessagesErrored:  s.externalMessagesErrored.Load(),
		InternalMessagesReceived: s.internalMessagesReceived.Load(),
		InternalMessagesSent:     s.internalMessagesSent.Load(),
		InternalMessagesErrored:  s.internalMessagesErrored.Load(),

		DroppedMessages: s.droppedMessages.Load(),
		LoadTime:        time.Duration(s.loadTime.Load()),
	}
	snap.NodeRefs = s.NodeRefStatsSnapshot()
	return snap
}
