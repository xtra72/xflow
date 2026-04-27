package agent

import "time"

// HealthState represents the health state of an agent.
type HealthState string

const (
	// HealthHealthy indicates the agent is operating normally.
	HealthHealthy HealthState = "healthy"

	// HealthDegraded indicates the agent has degraded performance or intermittent failures.
	HealthDegraded HealthState = "degraded"

	// HealthUnhealthy indicates the agent is not operational.
	HealthUnhealthy HealthState = "unhealthy"
)

// HealthStatus holds the health check result for an agent.
type HealthStatus struct {
	Status              HealthState // Current health state
	LastCheck           time.Time   // Time of the last health check
	LastSuccess         time.Time   // Time of the last successful health check
	ConsecutiveFailures int         // Number of consecutive health check failures
	Message             string      // Human-readable status message
}
