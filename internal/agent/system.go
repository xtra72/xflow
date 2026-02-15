package agent

// SystemAgent extends Agent with system-specific markers.
// System Agents do not require Transport or Protocol and are
// automatically activated when the system starts.
type SystemAgent interface {
	Agent
	IsSystem() bool
	RequiresTransport() bool
}

// SystemAgentType defines the type of a system agent.
type SystemAgentType string

const (
	// SystemAgentEvent is the system event publish/subscribe agent.
	SystemAgentEvent SystemAgentType = "event"

	// SystemAgentLogger is the system log management agent.
	SystemAgentLogger SystemAgentType = "logger"

	// SystemAgentFile is the system file access agent.
	SystemAgentFile SystemAgentType = "file"

	// SystemAgentTimer is the system timer/scheduler agent.
	SystemAgentTimer SystemAgentType = "timer"

	// SystemAgentStore is the system key-value store agent.
	SystemAgentStore SystemAgentType = "store"
)
