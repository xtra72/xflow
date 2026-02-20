package node

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// --- DefaultBridgeConfig 테스트 ---

// TestDefaultBridgeConfig_기본값 은 기본 설정이 올바른 값을 가지는지 확인한다.
func TestDefaultBridgeConfig_기본값(t *testing.T) {
	ref := flow.AgentRef{
		AgentID:   "agent-1",
		AgentName: "test-agent",
		Direction: flow.BridgeOut,
	}

	cfg := DefaultBridgeConfig(ref)

	assert.Equal(t, ref, cfg.AgentRef)
	assert.Equal(t, flow.BridgeOut, cfg.Direction)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 5*time.Second, cfg.ReconnectInterval)
	assert.Equal(t, 10, cfg.MaxReconnectAttempts)
	assert.Equal(t, 256, cfg.BufferSize)
	assert.Equal(t, "auto", cfg.Transform.Mode)
	assert.Equal(t, PayloadFormatAuto, cfg.Transform.PayloadFormat)
	assert.Empty(t, cfg.Transform.LuaScript)
	assert.Empty(t, cfg.ResponseTarget)
}

// --- BridgeConfig.Validate 테스트 ---

// TestBridgeConfig_Validate_정상 은 유효한 설정이 에러를 반환하지 않는지 확인한다.
func TestBridgeConfig_Validate_정상(t *testing.T) {
	tests := []struct {
		name string
		cfg  BridgeConfig
	}{
		{
			name: "AgentID만_있는_경우",
			cfg: BridgeConfig{
				AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: flow.BridgeOut},
				Direction:            flow.BridgeOut,
				RequestTimeout:       30 * time.Second,
				MaxReconnectAttempts: 10,
				BufferSize:           256,
				Transform:            TransformConfig{Mode: "auto"},
			},
		},
		{
			name: "AgentName만_있는_경우",
			cfg: BridgeConfig{
				AgentRef:             flow.AgentRef{AgentName: "test-agent", Direction: flow.BridgeIn},
				Direction:            flow.BridgeIn,
				RequestTimeout:       30 * time.Second,
				MaxReconnectAttempts: 0,
				BufferSize:           1,
				Transform:            TransformConfig{Mode: "auto"},
			},
		},
		{
			name: "Lua_스크립트_모드",
			cfg: BridgeConfig{
				AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: flow.BridgeInOut},
				Direction:            flow.BridgeInOut,
				RequestTimeout:       10 * time.Second,
				MaxReconnectAttempts: 5,
				BufferSize:           128,
				Transform:            TransformConfig{Mode: "lua", LuaScript: "return msg"},
			},
		},
		{
			name: "RequestReply_모드",
			cfg: BridgeConfig{
				AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: flow.BridgeRequestReply},
				Direction:            flow.BridgeRequestReply,
				RequestTimeout:       5 * time.Second,
				MaxReconnectAttempts: 3,
				BufferSize:           64,
				Transform:            TransformConfig{Mode: "auto"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestBridgeConfig_Validate_에러 는 유효하지 않은 설정에서 올바른 에러를 반환하는지 확인한다.
func TestBridgeConfig_Validate_에러(t *testing.T) {
	validBase := func() BridgeConfig {
		return BridgeConfig{
			AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: flow.BridgeOut},
			Direction:            flow.BridgeOut,
			RequestTimeout:       30 * time.Second,
			MaxReconnectAttempts: 10,
			BufferSize:           256,
			Transform:            TransformConfig{Mode: "auto"},
		}
	}

	tests := []struct {
		name    string
		modify  func(*BridgeConfig)
		wantMsg string
	}{
		{
			name: "AgentRef_ID와_Name_모두_비어있음",
			modify: func(c *BridgeConfig) {
				c.AgentRef = flow.AgentRef{Direction: flow.BridgeOut}
			},
			wantMsg: "AgentName",
		},
		{
			name: "유효하지_않은_Direction",
			modify: func(c *BridgeConfig) {
				c.Direction = "invalid"
			},
			wantMsg: "direction",
		},
		{
			name: "RequestReply_타임아웃_0",
			modify: func(c *BridgeConfig) {
				c.Direction = flow.BridgeRequestReply
				c.RequestTimeout = 0
			},
			wantMsg: "RequestTimeout",
		},
		{
			name: "RequestReply_타임아웃_음수",
			modify: func(c *BridgeConfig) {
				c.Direction = flow.BridgeRequestReply
				c.RequestTimeout = -1 * time.Second
			},
			wantMsg: "RequestTimeout",
		},
		{
			name: "MaxReconnectAttempts_음수",
			modify: func(c *BridgeConfig) {
				c.MaxReconnectAttempts = -1
			},
			wantMsg: "MaxReconnectAttempts",
		},
		{
			name: "BufferSize_0",
			modify: func(c *BridgeConfig) {
				c.BufferSize = 0
			},
			wantMsg: "BufferSize",
		},
		{
			name: "유효하지_않은_Transform_Mode",
			modify: func(c *BridgeConfig) {
				c.Transform.Mode = "unknown"
			},
			wantMsg: "Transform.Mode",
		},
		{
			name: "Lua_모드_빈_스크립트",
			modify: func(c *BridgeConfig) {
				c.Transform.Mode = "lua"
				c.Transform.LuaScript = ""
			},
			wantMsg: "LuaScript",
		},
		{
			name: "유효하지_않은_PayloadFormat",
			modify: func(c *BridgeConfig) {
				c.Transform.PayloadFormat = "xml"
			},
			wantMsg: "PayloadFormat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBase()
			tt.modify(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestBridgeConfig_Validate_비RequestReply_타임아웃무관 은 RequestReply가 아닌 모드에서는
// RequestTimeout이 0이어도 유효한지 확인한다.
func TestBridgeConfig_Validate_비RequestReply_타임아웃무관(t *testing.T) {
	directions := []flow.BridgeDirection{
		flow.BridgeIn,
		flow.BridgeOut,
		flow.BridgeInOut,
	}

	for _, dir := range directions {
		t.Run(string(dir), func(t *testing.T) {
			cfg := BridgeConfig{
				AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: dir},
				Direction:            dir,
				RequestTimeout:       0, // RequestReply가 아니면 0이어도 됨
				MaxReconnectAttempts: 10,
				BufferSize:           256,
				Transform:            TransformConfig{Mode: "auto"},
			}
			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestBridgeConfig_Validate_PayloadFormat_유효값 은 모든 유효한 PayloadFormat이 에러를 반환하지 않는지 확인한다.
func TestBridgeConfig_Validate_PayloadFormat_유효값(t *testing.T) {
	formats := []string{"", PayloadFormatAuto, PayloadFormatJSON, PayloadFormatRaw, PayloadFormatBinary}

	for _, f := range formats {
		t.Run(f, func(t *testing.T) {
			cfg := BridgeConfig{
				AgentRef:             flow.AgentRef{AgentID: "agent-1", Direction: flow.BridgeOut},
				Direction:            flow.BridgeOut,
				RequestTimeout:       30 * time.Second,
				MaxReconnectAttempts: 10,
				BufferSize:           256,
				Transform:            TransformConfig{Mode: "auto", PayloadFormat: f},
			}
			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestDefaultBridgeConfig_방향상속 은 AgentRef의 Direction이 Config.Direction으로 복사되는지 확인한다.
func TestDefaultBridgeConfig_방향상속(t *testing.T) {
	directions := []flow.BridgeDirection{
		flow.BridgeIn,
		flow.BridgeOut,
		flow.BridgeInOut,
		flow.BridgeRequestReply,
	}

	for _, dir := range directions {
		t.Run(string(dir), func(t *testing.T) {
			ref := flow.AgentRef{AgentID: "agent-1", Direction: dir}
			cfg := DefaultBridgeConfig(ref)
			assert.Equal(t, dir, cfg.Direction)
		})
	}
}
