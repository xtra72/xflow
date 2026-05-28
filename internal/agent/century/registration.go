package century

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterHvacr01Types 는 Century HVACR-01 에이전트 타입을 agent.DefaultManager 에 등록한다 (REQ-CENTURY-001, AC-D1).
//
// 등록 이름은 "century_hvacr01" 이다 (LG/Samsung HVACR-01 과의 일관성 — vendor_세대 패턴).
// 부트스트랩 호출 예시:
//
//	if err := century.RegisterHvacr01Types(agentMgr); err != nil {
//	    return fmt.Errorf("register century: %w", err)
//	}
//
// (cmd/xflowd/main.go 의 통합은 M4 의 노드 wiring 단계에서 함께 적용된다.)
func RegisterHvacr01Types(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("century_hvacr01", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHvacr01Agent(config)
	})
}
