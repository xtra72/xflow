package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterMQTTTypes 는 MQTT 관련 에이전트 타입을 Manager에 등록한다.
func RegisterMQTTTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("mqtt", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewMQTTAgent(config)
	})
}
