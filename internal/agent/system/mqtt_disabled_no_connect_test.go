package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// unreachableBroker 는 RFC 5737 문서용 IP로, 절대 연결되지 않는 브로커 주소이다.
// 실제 네트워크(라이브 브로커) 없이 "연결 시도 여부"를 상태로 검증하기 위해 사용한다.
const unreachableBroker = "tcp://192.0.2.1:1883"

// newMQTTConfigEnabled 는 지정된 enabled 상태를 가진 MQTT 에이전트 설정을 만든다.
// 도달 불가 브로커 + auto_reconnect=true 로, 활성화 시에는 논블로킹 Running(degraded)에
// 진입하고, 비활성화 시에는 연결을 시도하지 않아야 한다.
func newMQTTConfigEnabled(id string, enabled *bool) agent.AgentConfig {
	return agent.AgentConfig{
		ID:      id,
		Name:    "disabled-gate-mqtt",
		Type:    "mqtt-client",
		Enabled: enabled,
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              unreachableBroker,
				"client_id":           "xflow-disabled-gate-" + id,
				"connect_timeout_sec": 1,
				"buffer_size":         10,
				"auto_reconnect":      true,
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }

// TestMQTTAgent_Init_Disabled_DoesNotConnect 는 재현 테스트(RED)이다.
//
// 증상(루트 원인): MQTTAgent.Init() 이 IsEnabled() 와 무관하게 브로커에 연결을
// 시도한다(client.Connect() 호출 + StateRunning 전이). 데몬 부팅 시 restoreAgents 가
// disabled 에이전트도 List API 노출을 위해 Create(=Init) 하므로, 비활성화 에이전트가
// 부팅 시 브로커에 연결되어 버린다.
//
// 수정 전(RED): disabled 에이전트도 Init 에서 Connect+Running 에 진입하므로
//
//	State == Running 이 되어 아래 "State != Running" 단언이 실패한다.
//
// 수정 후(GREEN): disabled 에이전트는 연결을 건너뛰고 Running 에 진입하지 않는다.
func TestMQTTAgent_Init_Disabled_DoesNotConnect(t *testing.T) {
	cfg := newMQTTConfigEnabled("agent-mqtt-disabled", boolPtr(false))

	a, err := NewMQTTAgent(cfg)
	require.NoError(t, err, "disabled 에이전트 생성은 에러 없이 성공해야 한다(List API 노출을 위해 등록)")
	require.NotNil(t, a)

	// 핵심: 비활성화 에이전트는 Running 상태로 진입하면 안 된다.
	// Running 으로 진입했다면 Init 이 Connect 를 호출했다는 의미이다.
	assert.NotEqual(t, lifecycle.StateRunning, a.Info().State,
		"disabled 에이전트는 Init 에서 브로커에 연결/Running 진입하지 않아야 한다")

	// 트랜스포트는 연결되지 않아야 한다.
	tc, ok := a.(agent.TransportChecker)
	require.True(t, ok)
	assert.False(t, tc.TransportConnected(),
		"disabled 에이전트는 TransportConnected() 가 false 여야 한다(연결 시도 없음)")

	// 그러나 에이전트는 등록/사용 가능해야 한다(List API 노출).
	info := a.Info()
	assert.Equal(t, "agent-mqtt-disabled", info.ID)
	assert.Equal(t, "mqtt-client", info.Type)
}

// TestMQTTAgent_Init_Enabled_Connects 는 회귀 가드이다.
// 활성화(Enabled=nil 또는 true) 에이전트는 기존처럼 Init 에서 연결을 시도하여
// Running(도달 불가 브로커에서는 degraded) 상태에 진입해야 한다.
func TestMQTTAgent_Init_Enabled_Connects(t *testing.T) {
	// Enabled=nil (기본 true)
	cfgNil := newMQTTConfigEnabled("agent-mqtt-enabled-nil", nil)
	aNil, err := NewMQTTAgent(cfgNil)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, aNil.Info().State,
		"Enabled=nil(기본 활성) 에이전트는 Init 에서 Running(degraded) 에 진입해야 한다")

	// Enabled=true 명시
	cfgTrue := newMQTTConfigEnabled("agent-mqtt-enabled-true", boolPtr(true))
	aTrue, err := NewMQTTAgent(cfgTrue)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, aTrue.Info().State,
		"Enabled=true 에이전트는 Init 에서 Running(degraded) 에 진입해야 한다")
}

// TestMQTTAgent_DisabledThenEnableStart_Connects 는 비활성화 상태로 생성된 뒤
// enable + start 흐름을 거치면 연결(Running 진입)이 이뤄지는지 검증한다.
//
// 흐름: Create(disabled) → Created(비연결) → Configure(Enabled=true) → Start()
//
//	Start() 는 Created/Stopped 상태에서 Init(agentConfig) 를 재호출하여 연결한다.
func TestMQTTAgent_DisabledThenEnableStart_Connects(t *testing.T) {
	cfg := newMQTTConfigEnabled("agent-mqtt-enable-later", boolPtr(false))

	a, err := NewMQTTAgent(cfg)
	require.NoError(t, err)
	require.NotEqual(t, lifecycle.StateRunning, a.Info().State,
		"생성 직후 disabled 에이전트는 Running 이 아니어야 한다")

	// enable: Configure 로 Enabled=true 를 반영한다(setAgentEnabled 경로와 동일).
	enabledCfg := cfg
	enabledCfg.Enabled = boolPtr(true)
	require.NoError(t, a.Configure(enabledCfg))

	// start: enable 이후 Start() 가 연결을 시작해야 한다.
	require.NoError(t, a.Start(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, a.Info().State,
		"enable 후 Start() 는 Init 을 재호출하여 Running(degraded) 에 진입해야 한다")
}
