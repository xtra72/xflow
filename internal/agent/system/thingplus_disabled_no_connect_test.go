package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// newThingplusConfigEnabled 는 지정된 enabled 상태를 가진 thingplus 게이트웨이 설정을
// 만든다. 도달 불가 브로커(RFC 5737) + auto_reconnect=true 로, 활성화 시에는 논블로킹
// Running(degraded) 에 진입하고, 비활성화 시에는 연결을 시도하지 않아야 한다.
func newThingplusConfigEnabled(id string, enabled *bool) agent.AgentConfig {
	return agent.AgentConfig{
		ID:      id,
		Name:    "disabled-gate-thingplus",
		Type:    "thingplus-gateway",
		Enabled: enabled,
		Transport: agent.TransportConfig{
			Type: "thingplus",
			Options: map[string]any{
				"broker":              "192.0.2.1", // RFC 5737 문서용 IP (연결 불가)
				"access_token":        "TOKEN",
				"connect_timeout_sec": 1,
				"buffer_size":         10,
				"auto_reconnect":      true,
			},
		},
	}
}

// TestThingplusAgent_Init_Disabled_DoesNotConnect 는 재현 테스트(RED)이다.
//
// 증상(루트 원인): ThingplusGatewayAgent.Init() 이 IsEnabled() 와 무관하게 게이트웨이
// 브로커에 연결을 시도한다(client.Connect() 호출 + StateRunning 전이). 데몬 부팅 시
// restoreAgents 가 disabled 에이전트도 List API 노출을 위해 Create(=Init) 하므로,
// 비활성화 에이전트가 부팅 시 브로커에 연결되어 버린다.
//
// 수정 전(RED): disabled 에이전트도 Init 에서 Connect+Running 에 진입하므로
//
//	State == Running 이 되어 "State != Running" 단언이 실패한다.
//
// 수정 후(GREEN): disabled 에이전트는 연결을 건너뛰고 Running 에 진입하지 않는다.
func TestThingplusAgent_Init_Disabled_DoesNotConnect(t *testing.T) {
	cfg := newThingplusConfigEnabled("agent-thingplus-disabled", boolPtr(false))

	a, err := NewThingplusGatewayAgent(cfg)
	require.NoError(t, err, "disabled 에이전트 생성은 에러 없이 성공해야 한다(List API 노출을 위해 등록)")
	require.NotNil(t, a)

	assert.NotEqual(t, lifecycle.StateRunning, a.Info().State,
		"disabled 에이전트는 Init 에서 게이트웨이에 연결/Running 진입하지 않아야 한다")

	tc, ok := a.(agent.TransportChecker)
	require.True(t, ok)
	assert.False(t, tc.TransportConnected(),
		"disabled 에이전트는 TransportConnected() 가 false 여야 한다(연결 시도 없음)")

	info := a.Info()
	assert.Equal(t, "agent-thingplus-disabled", info.ID)
	assert.Equal(t, "thingplus-gateway", info.Type)
}

// TestThingplusAgent_Init_Enabled_Connects 는 회귀 가드이다.
// 활성화(Enabled=nil 또는 true) 에이전트는 기존처럼 Init 에서 연결을 시도하여
// Running(도달 불가 브로커에서는 degraded) 상태에 진입해야 한다.
func TestThingplusAgent_Init_Enabled_Connects(t *testing.T) {
	cfgNil := newThingplusConfigEnabled("agent-thingplus-enabled-nil", nil)
	aNil, err := NewThingplusGatewayAgent(cfgNil)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, aNil.Info().State,
		"Enabled=nil(기본 활성) 에이전트는 Init 에서 Running(degraded) 에 진입해야 한다")

	cfgTrue := newThingplusConfigEnabled("agent-thingplus-enabled-true", boolPtr(true))
	aTrue, err := NewThingplusGatewayAgent(cfgTrue)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, aTrue.Info().State,
		"Enabled=true 에이전트는 Init 에서 Running(degraded) 에 진입해야 한다")
}

// TestThingplusAgent_DisabledThenEnableStart_Connects 는 비활성화 상태로 생성된 뒤
// enable + start 흐름을 거치면 연결(Running 진입)이 이뤄지는지 검증한다.
func TestThingplusAgent_DisabledThenEnableStart_Connects(t *testing.T) {
	cfg := newThingplusConfigEnabled("agent-thingplus-enable-later", boolPtr(false))

	a, err := NewThingplusGatewayAgent(cfg)
	require.NoError(t, err)
	require.NotEqual(t, lifecycle.StateRunning, a.Info().State,
		"생성 직후 disabled 에이전트는 Running 이 아니어야 한다")

	enabledCfg := cfg
	enabledCfg.Enabled = boolPtr(true)
	require.NoError(t, a.Configure(enabledCfg))

	require.NoError(t, a.Start(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, a.Info().State,
		"enable 후 Start() 는 Init 을 재호출하여 Running(degraded) 에 진입해야 한다")
}
