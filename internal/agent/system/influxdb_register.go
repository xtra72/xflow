package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterInfluxDBTypes 는 InfluxDB 관련 에이전트 타입을 Manager 에 등록한다.
//
// 본 함수는 dual-tag 부착이 필요 없는 환경 (테스트, 단독 실행) 에서 사용된다.
// dual-tag 부착을 활성화하려면 RegisterInfluxDBTypesWithResolver 를 사용한다.
func RegisterInfluxDBTypes(mgr *agent.DefaultManager) error {
	return RegisterInfluxDBTypesWithResolver(mgr, nil)
}

// RegisterInfluxDBTypesWithResolver 는 DeviceResolver 를 주입하여 InfluxDB
// 에이전트 타입을 Manager 에 등록한다.
//
// resolver 가 non-nil 이고 yaml 의 dual_tag_emit 옵션이 true (default) 이면,
// 본 manager 로 생성된 InfluxDBAgent 는 자동으로 dual-tag 부착 (composite + uid)
// 을 수행한다.
//
// resolver 가 nil 이면 RegisterInfluxDBTypes 와 동일하게 동작한다
// (dual-tag 부착 없음 — 단독 테스트/실행 환경 안전 보장).
//
// SPEC-DEVICE-IDENTITY-001 Phase C § C3. cmd/xflowd 의 startup 에서
// device.DeviceRegistry 를 직접 전달 가능 (Resolver 인터페이스 자연 만족).
func RegisterInfluxDBTypesWithResolver(mgr *agent.DefaultManager, resolver DeviceResolver) error {
	return mgr.RegisterType("influxdb", func(config agent.AgentConfig) (agent.Agent, error) {
		if resolver == nil {
			return NewInfluxDBAgent(config)
		}
		return NewInfluxDBAgentWithOptions(config, WithDeviceResolver(resolver))
	})
}
