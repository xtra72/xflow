package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterLGHvacr03Types 는 LG HVACR-03 (PMBUSB00A Modbus 게이트웨이) 에이전트 타입을
// Manager 에 등록한다.
//
// 이 함수를 정의만 하고 cmd/xflowd/main.go 에 배선하지 않으면 에이전트가 조용히
// 사라진다 — 등록 누락은 컴파일 오류를 내지 않는다.
func RegisterLGHvacr03Types(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lg_hvacr03", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHvacr03Agent(config)
	})
}
