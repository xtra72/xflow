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

// StatsSnapshot is an immutable snapshot of agent processing statistics.
type StatsSnapshot struct {
	MessagesReceived     int64         // Total messages received
	MessagesSent         int64         // Total messages sent
	MessagesErrored      int64         // Total messages with processing errors
	BytesRead            int64         // Total bytes read from transport
	BytesWritten         int64         // Total bytes written to transport
	LastActivityAt       time.Time     // Time of last data activity
	AvgProcessingLatency time.Duration // Average message processing latency
	RestartCount         int64         // Number of restarts
	MsgBufferPending     int           `json:"msg_buffer_pending"`  // Current number of pending messages in buffer
	MsgBufferCapacity    int           `json:"msg_buffer_capacity"` // Total capacity of message buffer
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

// AgentStats provides thread-safe atomic counters for agent processing statistics.
type AgentStats struct {
	messagesReceived     atomic.Int64
	messagesSent         atomic.Int64
	messagesErrored      atomic.Int64
	bytesRead            atomic.Int64
	bytesWritten         atomic.Int64
	restartCount         atomic.Int64
	avgProcessingLatency atomic.Int64 // stored as nanoseconds
	mu                   sync.Mutex
	lastActivityAt       time.Time
}

// NewAgentStats creates a new AgentStats instance with all counters at zero.
func NewAgentStats() *AgentStats {
	return &AgentStats{}
}

// IncrMessagesReceived atomically increments the messages received counter.
func (s *AgentStats) IncrMessagesReceived() {
	s.messagesReceived.Add(1)
}

// IncrMessagesSent atomically increments the messages sent counter.
func (s *AgentStats) IncrMessagesSent() {
	s.messagesSent.Add(1)
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

// ResetStats resets all counters to zero.
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
}

// Snapshot returns an immutable snapshot of the current statistics.
func (s *AgentStats) Snapshot() StatsSnapshot {
	s.mu.Lock()
	lastActivity := s.lastActivityAt
	s.mu.Unlock()

	return StatsSnapshot{
		MessagesReceived:     s.messagesReceived.Load(),
		MessagesSent:         s.messagesSent.Load(),
		MessagesErrored:      s.messagesErrored.Load(),
		BytesRead:            s.bytesRead.Load(),
		BytesWritten:         s.bytesWritten.Load(),
		LastActivityAt:       lastActivity,
		AvgProcessingLatency: time.Duration(s.avgProcessingLatency.Load()),
		RestartCount:         s.restartCount.Load(),
	}
}
