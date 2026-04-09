package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newFullAgentConfig 는 모든 필드가 설정된 테스트용 AgentConfig 를 반환한다.
func newFullAgentConfig() AgentConfig {
	return AgentConfig{
		ID:   "agent-001",
		Name: "NASA HVAC Controller",
		Type: "custom",
		Transport: TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"port":     "/dev/ttyUSB0",
				"baudrate": float64(9600),
				"parity":   "none",
				"enabled":  true,
			},
		},
		ProtocolFile:        "/etc/xflow/protocols/hvac.proto",
		HealthCheckInterval: 30 * time.Second,
		MaxRestarts:         5,
		StopOnZeroRef:       true,
		BufferSize:          2048,
		LogLevel:            "debug",
		Metadata: map[string]string{
			"location": "building-a",
			"floor":    "3",
		},
	}
}

// ---------------------------------------------------------------------------
// JSON 라운드트립 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigToJSON_Roundtrip(t *testing.T) {
	original := newFullAgentConfig()

	// 직렬화
	data, err := AgentConfigToJSON(original)
	require.NoError(t, err)
	require.True(t, json.Valid(data), "유효한 JSON이어야 한다")

	// 역직렬화
	restored, err := AgentConfigFromJSON(data)
	require.NoError(t, err)

	// 모든 필드 비교
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Type, restored.Type)
	assert.Equal(t, original.Transport.Type, restored.Transport.Type)
	assert.Equal(t, original.ProtocolFile, restored.ProtocolFile)
	assert.Equal(t, original.HealthCheckInterval, restored.HealthCheckInterval)
	assert.Equal(t, original.MaxRestarts, restored.MaxRestarts)
	assert.Equal(t, original.StopOnZeroRef, restored.StopOnZeroRef)
	assert.Equal(t, original.BufferSize, restored.BufferSize)
	assert.Equal(t, original.LogLevel, restored.LogLevel)
	assert.Equal(t, original.Metadata, restored.Metadata)

	// Transport.Options 의 각 값 비교
	assert.Equal(t, original.Transport.Options["port"], restored.Transport.Options["port"])
	assert.Equal(t, original.Transport.Options["parity"], restored.Transport.Options["parity"])
	assert.Equal(t, original.Transport.Options["enabled"], restored.Transport.Options["enabled"])
	// JSON 숫자는 float64 로 역직렬화되므로 타입 일치 확인
	assert.InDelta(t, original.Transport.Options["baudrate"], restored.Transport.Options["baudrate"], 0.001)
}

// ---------------------------------------------------------------------------
// YAML 라운드트립 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigToYAML_Roundtrip(t *testing.T) {
	original := newFullAgentConfig()

	// 직렬화
	data, err := AgentConfigToYAML(original)
	require.NoError(t, err)
	require.NotEmpty(t, data, "YAML 출력이 비어있으면 안 된다")

	// 역직렬화
	restored, err := AgentConfigFromYAML(data)
	require.NoError(t, err)

	// 모든 필드 비교
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.Type, restored.Type)
	assert.Equal(t, original.Transport.Type, restored.Transport.Type)
	assert.Equal(t, original.ProtocolFile, restored.ProtocolFile)
	assert.Equal(t, original.HealthCheckInterval, restored.HealthCheckInterval)
	assert.Equal(t, original.MaxRestarts, restored.MaxRestarts)
	assert.Equal(t, original.StopOnZeroRef, restored.StopOnZeroRef)
	assert.Equal(t, original.BufferSize, restored.BufferSize)
	assert.Equal(t, original.LogLevel, restored.LogLevel)
	assert.Equal(t, original.Metadata, restored.Metadata)

	// Transport.Options 의 각 값 비교
	assert.Equal(t, original.Transport.Options["port"], restored.Transport.Options["port"])
	assert.Equal(t, original.Transport.Options["parity"], restored.Transport.Options["parity"])
	// YAML 은 숫자를 int 로 역직렬화할 수 있으므로 float64 변환 후 비교
	assert.InDelta(t, original.Transport.Options["baudrate"], restored.Transport.Options["baudrate"], 0.001)
}

// ---------------------------------------------------------------------------
// 빈 필드 처리 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigFromJSON_EmptyFields(t *testing.T) {
	// 최소 필드만 설정된 JSON
	input := `{"id":"a1","name":"minimal"}`

	config, err := AgentConfigFromJSON([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "a1", config.ID)
	assert.Equal(t, "minimal", config.Name)
	assert.Empty(t, config.Type)
	assert.Empty(t, config.Transport.Type)
	assert.Nil(t, config.Transport.Options)
	assert.Empty(t, config.ProtocolFile)
	assert.Equal(t, time.Duration(0), config.HealthCheckInterval)
	assert.Equal(t, 0, config.MaxRestarts)
	assert.False(t, config.StopOnZeroRef)
	assert.Equal(t, 0, config.BufferSize)
	assert.Empty(t, config.LogLevel)
	assert.Nil(t, config.Metadata)
}

func TestAgentConfigFromYAML_EmptyFields(t *testing.T) {
	// 최소 필드만 설정된 YAML
	input := "id: a1\nname: minimal\n"

	config, err := AgentConfigFromYAML([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "a1", config.ID)
	assert.Equal(t, "minimal", config.Name)
	assert.Empty(t, config.Type)
	assert.Empty(t, config.Transport.Type)
	assert.Nil(t, config.Transport.Options)
	assert.Empty(t, config.ProtocolFile)
	assert.Equal(t, time.Duration(0), config.HealthCheckInterval)
	assert.Equal(t, 0, config.MaxRestarts)
	assert.False(t, config.StopOnZeroRef)
	assert.Equal(t, 0, config.BufferSize)
	assert.Empty(t, config.LogLevel)
	assert.Nil(t, config.Metadata)
}

// ---------------------------------------------------------------------------
// Duration 직렬화 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigToJSON_Duration(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     string // JSON 문자열에 포함되어야 하는 값
	}{
		{
			name:     "30초",
			interval: 30 * time.Second,
			want:     `"health_check_interval":"30s"`,
		},
		{
			name:     "5분",
			interval: 5 * time.Minute,
			want:     `"health_check_interval":"5m0s"`,
		},
		{
			name:     "1시간 30분",
			interval: 1*time.Hour + 30*time.Minute,
			want:     `"health_check_interval":"1h30m0s"`,
		},
		{
			name:     "0 (빈 문자열)",
			interval: 0,
			want:     "", // omitempty 로 인해 필드 자체가 생략됨
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := AgentConfig{
				ID:                  "test",
				Name:                "Test",
				HealthCheckInterval: tc.interval,
			}

			data, err := AgentConfigToJSON(config)
			require.NoError(t, err)

			s := string(data)
			if tc.want != "" {
				assert.Contains(t, s, tc.want, "JSON에 Duration 문자열이 포함되어야 한다")
			} else {
				assert.NotContains(t, s, "health_check_interval",
					"Duration이 0이면 health_check_interval 필드가 생략되어야 한다")
			}

			// 라운드트립 검증
			restored, err := AgentConfigFromJSON(data)
			require.NoError(t, err)
			assert.Equal(t, tc.interval, restored.HealthCheckInterval)
		})
	}
}

// ---------------------------------------------------------------------------
// Transport Options 라운드트립 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigFromJSON_Transport(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(t *testing.T, config AgentConfig)
		wantErr bool
	}{
		{
			name: "중첩 맵을 포함한 Transport Options",
			input: `{
				"id": "a1",
				"name": "nested",
				"transport": {
					"type": "tcp",
					"options": {
						"host": "192.168.1.100",
						"port": 502,
						"timeout": 5.5,
						"tls": {
							"enabled": true,
							"cert_file": "/etc/certs/client.pem"
						}
					}
				}
			}`,
			check: func(t *testing.T, config AgentConfig) {
				assert.Equal(t, "tcp", config.Transport.Type)
				assert.Equal(t, "192.168.1.100", config.Transport.Options["host"])
				// JSON 숫자는 float64 로 역직렬화된다
				assert.InDelta(t, float64(502), config.Transport.Options["port"], 0.001)
				assert.InDelta(t, 5.5, config.Transport.Options["timeout"], 0.001)

				// 중첩 맵 확인
				tls, ok := config.Transport.Options["tls"].(map[string]any)
				require.True(t, ok, "tls 는 map[string]any 타입이어야 한다")
				assert.Equal(t, true, tls["enabled"])
				assert.Equal(t, "/etc/certs/client.pem", tls["cert_file"])
			},
		},
		{
			name: "boolean 값을 포함한 Transport Options",
			input: `{
				"id": "a2",
				"name": "booleans",
				"transport": {
					"type": "serial",
					"options": {
						"port": "/dev/ttyS0",
						"rts_cts": true,
						"xon_xoff": false
					}
				}
			}`,
			check: func(t *testing.T, config AgentConfig) {
				assert.Equal(t, "serial", config.Transport.Type)
				assert.Equal(t, "/dev/ttyS0", config.Transport.Options["port"])
				assert.Equal(t, true, config.Transport.Options["rts_cts"])
				assert.Equal(t, false, config.Transport.Options["xon_xoff"])
			},
		},
		{
			name: "배열 값을 포함한 Transport Options",
			input: `{
				"id": "a3",
				"name": "arrays",
				"transport": {
					"type": "udp",
					"options": {
						"endpoints": ["10.0.0.1:5000", "10.0.0.2:5000"],
						"retry_intervals": [1, 2, 5]
					}
				}
			}`,
			check: func(t *testing.T, config AgentConfig) {
				assert.Equal(t, "udp", config.Transport.Type)

				endpoints, ok := config.Transport.Options["endpoints"].([]any)
				require.True(t, ok, "endpoints 는 []any 타입이어야 한다")
				assert.Len(t, endpoints, 2)
				assert.Equal(t, "10.0.0.1:5000", endpoints[0])

				retries, ok := config.Transport.Options["retry_intervals"].([]any)
				require.True(t, ok, "retry_intervals 는 []any 타입이어야 한다")
				assert.Len(t, retries, 3)
			},
		},
		{
			name: "빈 Transport",
			input: `{
				"id": "a4",
				"name": "empty-transport"
			}`,
			check: func(t *testing.T, config AgentConfig) {
				assert.Empty(t, config.Transport.Type)
				assert.Nil(t, config.Transport.Options)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config, err := AgentConfigFromJSON([]byte(tc.input))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tc.check(t, config)

			// JSON 라운드트립 검증
			data, err := AgentConfigToJSON(config)
			require.NoError(t, err)

			restored, err := AgentConfigFromJSON(data)
			require.NoError(t, err)
			assert.Equal(t, config.ID, restored.ID)
			assert.Equal(t, config.Name, restored.Name)
			assert.Equal(t, config.Transport.Type, restored.Transport.Type)
		})
	}
}

// ---------------------------------------------------------------------------
// 잘못된 입력 처리 테스트
// ---------------------------------------------------------------------------

func TestAgentConfigFromJSON_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "유효하지 않은 JSON",
			input: `{invalid json}`,
		},
		{
			name:  "잘못된 Duration 문자열",
			input: `{"id":"a1","name":"bad","health_check_interval":"not-a-duration"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AgentConfigFromJSON([]byte(tc.input))
			assert.Error(t, err)
		})
	}
}

func TestAgentConfigFromYAML_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "유효하지 않은 YAML",
			input: ":\n  :\n    - [invalid",
		},
		{
			name:  "잘못된 Duration 문자열",
			input: "id: a1\nname: bad\nhealth_check_interval: not-a-duration\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AgentConfigFromYAML([]byte(tc.input))
			assert.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// toJSON / fromJSON 내부 헬퍼 테스트
// ---------------------------------------------------------------------------

func TestToJSON_ZeroDuration(t *testing.T) {
	config := AgentConfig{
		ID:                  "test",
		Name:                "Test",
		HealthCheckInterval: 0,
	}

	j := toJSON(config)
	assert.Empty(t, j.HealthCheckInterval, "Duration 0 은 빈 문자열이어야 한다")
}

func TestFromJSON_EmptyDuration(t *testing.T) {
	j := agentConfigJSON{
		ID:                  "test",
		Name:                "Test",
		HealthCheckInterval: "",
	}

	config, err := fromJSON(j)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), config.HealthCheckInterval)
}

func TestFromJSON_InvalidDuration(t *testing.T) {
	j := agentConfigJSON{
		ID:                  "test",
		Name:                "Test",
		HealthCheckInterval: "invalid",
	}

	_, err := fromJSON(j)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health_check_interval")
}

// ---------------------------------------------------------------------------
// Enabled 필드 직렬화 테스트 (SPEC-AGENT-005)
// ---------------------------------------------------------------------------

// enabledBoolPtr 는 직렬화 테스트에서 *bool 리터럴을 편하게 만들기 위한 헬퍼이다.
func enabledBoolPtr(b bool) *bool {
	return &b
}

// newMinimalAgentConfig 는 Enabled 필드 테스트를 위해 최소 필드만 설정된 구성을 반환한다.
func newMinimalAgentConfig() AgentConfig {
	return AgentConfig{
		ID:   "agent-enabled",
		Name: "Enabled Test Agent",
		Type: "custom",
	}
}

// --- JSON: toJSON / Marshal: Enabled=nil 은 "enabled" 키를 생략해야 한다. ---

func TestAgentConfigToJSON_EnabledNil_OmitsKey(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = nil

	data, err := AgentConfigToJSON(cfg)
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"enabled"`,
		"Enabled 가 nil 이면 JSON 에 enabled 키가 포함되면 안 된다")
}

func TestAgentConfigToJSON_EnabledTrue_IncludesTrue(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = enabledBoolPtr(true)

	data, err := AgentConfigToJSON(cfg)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"enabled":true`,
		"Enabled=true 이면 JSON 에 \"enabled\":true 가 포함되어야 한다")
}

func TestAgentConfigToJSON_EnabledFalse_IncludesFalse(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = enabledBoolPtr(false)

	data, err := AgentConfigToJSON(cfg)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"enabled":false`,
		"Enabled=false 이면 JSON 에 \"enabled\":false 가 포함되어야 한다")
}

// --- JSON: fromJSON / Unmarshal ---

func TestAgentConfigFromJSON_MissingKey_NilEnabled(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"id":"a","name":"n","type":"t"}`)

	cfg, err := AgentConfigFromJSON(raw)
	require.NoError(t, err)

	assert.Nil(t, cfg.Enabled,
		"enabled 키가 누락되면 Enabled 는 nil 이어야 한다")
}

func TestAgentConfigFromJSON_ExplicitFalse_SetsFalse(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"id":"a","name":"n","type":"t","enabled":false}`)

	cfg, err := AgentConfigFromJSON(raw)
	require.NoError(t, err)

	require.NotNil(t, cfg.Enabled, "Enabled 는 nil 이면 안 된다")
	assert.False(t, *cfg.Enabled, "*Enabled 는 false 여야 한다")
}

func TestAgentConfigFromJSON_ExplicitTrue_SetsTrue(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"id":"a","name":"n","type":"t","enabled":true}`)

	cfg, err := AgentConfigFromJSON(raw)
	require.NoError(t, err)

	require.NotNil(t, cfg.Enabled, "Enabled 는 nil 이면 안 된다")
	assert.True(t, *cfg.Enabled, "*Enabled 는 true 여야 한다")
}

// --- JSON: 라운드트립을 통한 Enabled 보존 검증 ---

func TestAgentConfigJSON_EnabledRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled *bool
	}{
		{name: "nil", enabled: nil},
		{name: "true", enabled: enabledBoolPtr(true)},
		{name: "false", enabled: enabledBoolPtr(false)},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := newMinimalAgentConfig()
			original.Enabled = tc.enabled

			data, err := AgentConfigToJSON(original)
			require.NoError(t, err)

			restored, err := AgentConfigFromJSON(data)
			require.NoError(t, err)

			if tc.enabled == nil {
				assert.Nil(t, restored.Enabled)
			} else {
				require.NotNil(t, restored.Enabled)
				assert.Equal(t, *tc.enabled, *restored.Enabled)
			}
		})
	}
}

// --- YAML: toYAML: Enabled=nil 은 "enabled" 키를 생략해야 한다. ---

func TestAgentConfigToYAML_EnabledNil_OmitsKey(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = nil

	data, err := AgentConfigToYAML(cfg)
	require.NoError(t, err)

	assert.False(t, strings.Contains(string(data), "enabled:"),
		"Enabled 가 nil 이면 YAML 에 enabled 키가 포함되면 안 된다")
}

func TestAgentConfigToYAML_EnabledTrue_IncludesTrue(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = enabledBoolPtr(true)

	data, err := AgentConfigToYAML(cfg)
	require.NoError(t, err)

	assert.Contains(t, string(data), "enabled: true",
		"Enabled=true 이면 YAML 에 enabled: true 가 포함되어야 한다")
}

func TestAgentConfigToYAML_EnabledFalse_IncludesFalse(t *testing.T) {
	t.Parallel()

	cfg := newMinimalAgentConfig()
	cfg.Enabled = enabledBoolPtr(false)

	data, err := AgentConfigToYAML(cfg)
	require.NoError(t, err)

	assert.Contains(t, string(data), "enabled: false",
		"Enabled=false 이면 YAML 에 enabled: false 가 포함되어야 한다")
}

// --- YAML: fromYAML ---

func TestAgentConfigFromYAML_MissingKey_NilEnabled(t *testing.T) {
	t.Parallel()

	raw := []byte("id: a\nname: n\ntype: t\n")

	cfg, err := AgentConfigFromYAML(raw)
	require.NoError(t, err)

	assert.Nil(t, cfg.Enabled,
		"enabled 키가 누락되면 Enabled 는 nil 이어야 한다")
}

func TestAgentConfigFromYAML_ExplicitFalse_SetsFalse(t *testing.T) {
	t.Parallel()

	raw := []byte("id: a\nname: n\ntype: t\nenabled: false\n")

	cfg, err := AgentConfigFromYAML(raw)
	require.NoError(t, err)

	require.NotNil(t, cfg.Enabled, "Enabled 는 nil 이면 안 된다")
	assert.False(t, *cfg.Enabled, "*Enabled 는 false 여야 한다")
}

func TestAgentConfigFromYAML_ExplicitTrue_SetsTrue(t *testing.T) {
	t.Parallel()

	raw := []byte("id: a\nname: n\ntype: t\nenabled: true\n")

	cfg, err := AgentConfigFromYAML(raw)
	require.NoError(t, err)

	require.NotNil(t, cfg.Enabled, "Enabled 는 nil 이면 안 된다")
	assert.True(t, *cfg.Enabled, "*Enabled 는 true 여야 한다")
}

// --- YAML: 라운드트립을 통한 Enabled 보존 검증 ---

func TestAgentConfigYAML_EnabledRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled *bool
	}{
		{name: "nil", enabled: nil},
		{name: "true", enabled: enabledBoolPtr(true)},
		{name: "false", enabled: enabledBoolPtr(false)},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := newMinimalAgentConfig()
			original.Enabled = tc.enabled

			data, err := AgentConfigToYAML(original)
			require.NoError(t, err)

			restored, err := AgentConfigFromYAML(data)
			require.NoError(t, err)

			if tc.enabled == nil {
				assert.Nil(t, restored.Enabled)
			} else {
				require.NotNil(t, restored.Enabled)
				assert.Equal(t, *tc.enabled, *restored.Enabled)
			}
		})
	}
}

// --- 하위 호환성: 기존 JSON/YAML 에 enabled 키가 없어도 안정적으로 파싱되어야 한다. ---

func TestAgentConfigJSON_BackwardCompatibility_NoEnabledKey(t *testing.T) {
	t.Parallel()

	// 기존 형식(enabled 키 없음)의 JSON 을 사용한다.
	raw := []byte(`{"id":"legacy","name":"Legacy Agent","type":"custom","max_restarts":5}`)

	cfg, err := AgentConfigFromJSON(raw)
	require.NoError(t, err)

	// Enabled 는 nil 이어야 하고 IsEnabled() 는 기본값 true 를 반환해야 한다.
	assert.Nil(t, cfg.Enabled)
	assert.True(t, cfg.IsEnabled(), "하위 호환: 기존 설정은 기본 활성화 상태여야 한다")

	// 나머지 필드도 정상 파싱되었는지 확인한다.
	assert.Equal(t, "legacy", cfg.ID)
	assert.Equal(t, "Legacy Agent", cfg.Name)
	assert.Equal(t, 5, cfg.MaxRestarts)
}

// 빈 구조체가 JSON 라운드트립에서 유효한지 확인한다 (컴파일 확인용).
func TestAgentConfigJSON_EmptyStructRoundtrip(t *testing.T) {
	t.Parallel()

	var j agentConfigJSON
	data, err := json.Marshal(j)
	require.NoError(t, err)

	var restored agentConfigJSON
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Nil(t, restored.Enabled)
}

// 이 함수는 yaml 패키지가 실제로 사용되는지 검증하기 위한 컴파일 가드이다.
var _ = yaml.Marshal
