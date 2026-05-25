package century

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterCenturyTypes 는 Century HVAC 에이전트 타입을 agent.DefaultManager 에 등록한다 (REQ-CENTURY-001, AC-D1).
//
// 등록 이름은 "century-hvac" 이다 (NASA / LGCNP 와의 일관성 — 프로토콜 이름을 그대로 노출).
// 부트스트랩 호출 예시:
//
//	if err := century.RegisterCenturyTypes(agentMgr); err != nil {
//	    return fmt.Errorf("register century: %w", err)
//	}
//
// (cmd/xflowd/main.go 의 통합은 M4 의 노드 wiring 단계에서 함께 적용된다.)
func RegisterCenturyTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("century-hvac", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewCenturyAgent(config)
	})
}
