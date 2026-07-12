package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseDevices_Source 는 devices 항목의 "source" 키가 DeviceEntry.Source 로
// 파싱되고, 없으면 빈 문자열(로더가 "config" 로 간주)로 남는지 검증한다.
func TestParseDevices_Source(t *testing.T) {
	opts := map[string]any{
		"devices": []any{
			map[string]any{"address": "200001", "name": "runtime", "source": "bridge"},
			map[string]any{"address": "200002", "name": "declared"}, // source 미지정
		},
	}
	got := ParseDevices(opts)
	require.Len(t, got, 2)
	assert.Equal(t, "bridge", got[0].Source, "source 키가 보존되어야 함")
	assert.Equal(t, "", got[1].Source, "source 미지정은 빈 문자열(로더가 config 처리)")
}

func TestAgentConfig_Validate_Valid(t *testing.T) {
	cfg := AgentConfig{
		ID:                  "agent-1",
		Name:                "Test Agent",
		HealthCheckInterval: 30 * time.Second,
		MaxRestarts:         10,
		BufferSize:          1024,
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

func TestAgentConfig_Validate_EmptyID(t *testing.T) {
	cfg := AgentConfig{
		ID:   "",
		Name: "Test Agent",
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
	assert.Contains(t, err.Error(), "ID")
}

func TestAgentConfig_Validate_EmptyName(t *testing.T) {
	cfg := AgentConfig{
		ID:   "agent-1",
		Name: "",
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
	assert.Contains(t, err.Error(), "Name")
}

func TestAgentConfig_Validate_NegativeHealthCheckInterval(t *testing.T) {
	cfg := AgentConfig{
		ID:                  "agent-1",
		Name:                "Test Agent",
		HealthCheckInterval: -1 * time.Second,
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
	assert.Contains(t, err.Error(), "HealthCheckInterval")
}

func TestAgentConfig_Validate_NegativeMaxRestarts(t *testing.T) {
	cfg := AgentConfig{
		ID:          "agent-1",
		Name:        "Test Agent",
		MaxRestarts: -1,
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
	assert.Contains(t, err.Error(), "MaxRestarts")
}

func TestAgentConfig_Validate_NegativeBufferSize(t *testing.T) {
	cfg := AgentConfig{
		ID:         "agent-1",
		Name:       "Test Agent",
		BufferSize: -1,
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
	assert.Contains(t, err.Error(), "BufferSize")
}

func TestAgentConfig_Validate_ZeroHealthCheckInterval_SetsDefault(t *testing.T) {
	cfg := AgentConfig{
		ID:                  "agent-1",
		Name:                "Test Agent",
		HealthCheckInterval: 0,
	}

	err := cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.HealthCheckInterval)
}

func TestAgentConfig_Validate_ZeroBufferSize_SetsDefault(t *testing.T) {
	cfg := AgentConfig{
		ID:         "agent-1",
		Name:       "Test Agent",
		BufferSize: 0,
	}

	err := cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, 1024, cfg.BufferSize)
}

func TestAgentConfig_Validate_ZeroMaxRestarts_Allowed(t *testing.T) {
	// MaxRestarts == 0 은 재시작 비활성화를 의미하므로 허용된다.
	cfg := AgentConfig{
		ID:          "agent-1",
		Name:        "Test Agent",
		MaxRestarts: 0,
	}

	err := cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.MaxRestarts)
}

func TestDefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()

	assert.Equal(t, 30*time.Second, cfg.HealthCheckInterval)
	assert.Equal(t, 10, cfg.MaxRestarts)
	assert.True(t, cfg.StopOnZeroRef)
	assert.Equal(t, 1024, cfg.BufferSize)
	assert.Empty(t, cfg.ID, "기본 설정에는 ID가 비어있어야 한다")
	assert.Empty(t, cfg.Name, "기본 설정에는 Name이 비어있어야 한다")
}

func TestTransportConfig_Creation(t *testing.T) {
	tc := TransportConfig{
		Type: "serial",
		Options: map[string]any{
			"port":     "/dev/ttyUSB0",
			"baudrate": 9600,
		},
	}

	assert.Equal(t, "serial", tc.Type)
	assert.Equal(t, "/dev/ttyUSB0", tc.Options["port"])
	assert.Equal(t, 9600, tc.Options["baudrate"])
}

func TestAgentConfig_MetadataCopy(t *testing.T) {
	// Metadata가 독립적으로 동작하는지 확인한다.
	cfg1 := AgentConfig{
		ID:   "agent-1",
		Name: "Agent 1",
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	// cfg2에 같은 Metadata 맵을 공유하지 않는지 확인한다.
	cfg2 := AgentConfig{
		ID:   "agent-2",
		Name: "Agent 2",
		Metadata: map[string]string{
			"key2": "value2",
		},
	}

	assert.NotEqual(t, cfg1.Metadata, cfg2.Metadata)
	cfg1.Metadata["key3"] = "value3"
	_, exists := cfg2.Metadata["key3"]
	assert.False(t, exists, "cfg1의 Metadata 변경이 cfg2에 영향을 주면 안 된다")
}

// boolPtr 는 테스트에서 *bool 리터럴을 편하게 만들기 위한 헬퍼이다.
func boolPtr(b bool) *bool {
	return &b
}

func TestAgentConfig_IsEnabled_NilDefaultsToTrue(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{}

	assert.True(t, cfg.IsEnabled(), "Enabled 가 nil 이면 기본값 true 를 반환해야 한다")
}

func TestAgentConfig_IsEnabled_ExplicitTrue(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{Enabled: boolPtr(true)}

	assert.True(t, cfg.IsEnabled(), "Enabled 가 명시적으로 true 이면 true 를 반환해야 한다")
}

func TestAgentConfig_IsEnabled_ExplicitFalse(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{Enabled: boolPtr(false)}

	assert.False(t, cfg.IsEnabled(), "Enabled 가 명시적으로 false 이면 false 를 반환해야 한다")
}

func TestAgentConfig_IsEnabled_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{name: "nil defaults to true", enabled: nil, want: true},
		{name: "explicit true", enabled: boolPtr(true), want: true},
		{name: "explicit false", enabled: boolPtr(false), want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := AgentConfig{Enabled: tc.enabled}
			assert.Equal(t, tc.want, cfg.IsEnabled())
		})
	}
}

func TestAgentConfig_Validate_TableDriven(t *testing.T) {
	// table-driven 방식으로 다양한 유효/무효 설정을 테스트한다.
	tests := []struct {
		name    string
		config  AgentConfig
		wantErr bool
	}{
		{
			name: "valid minimal config",
			config: AgentConfig{
				ID:   "a1",
				Name: "Agent 1",
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: AgentConfig{
				ID:                  "a2",
				Name:                "Agent 2",
				Type:                "custom",
				HealthCheckInterval: 15 * time.Second,
				MaxRestarts:         5,
				BufferSize:          2048,
				Metadata:            map[string]string{"env": "test"},
			},
			wantErr: false,
		},
		{
			name:    "empty config",
			config:  AgentConfig{},
			wantErr: true,
		},
		{
			name: "missing name",
			config: AgentConfig{
				ID: "a3",
			},
			wantErr: true,
		},
		{
			name: "missing id",
			config: AgentConfig{
				Name: "Agent 4",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
