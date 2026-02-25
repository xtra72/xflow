package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// JSON/YAML 직렬화를 위한 중간 표현 구조체
// ---------------------------------------------------------------------------

// agentConfigJSON 은 AgentConfig 의 JSON/YAML 직렬화를 위한 보조 구조체이다.
// time.Duration 은 문자열로 직렬화한다 (예: "30s").
type agentConfigJSON struct {
	ID                  string            `json:"id" yaml:"id"`
	Name                string            `json:"name" yaml:"name"`
	Type                string            `json:"type" yaml:"type"`
	Transport           transportJSON     `json:"transport,omitempty" yaml:"transport,omitempty"`
	ProtocolFile        string            `json:"protocol_file,omitempty" yaml:"protocol_file,omitempty"`
	HealthCheckInterval string            `json:"health_check_interval,omitempty" yaml:"health_check_interval,omitempty"`
	MaxRestarts         int               `json:"max_restarts,omitempty" yaml:"max_restarts,omitempty"`
	StopOnZeroRef       bool              `json:"stop_on_zero_ref,omitempty" yaml:"stop_on_zero_ref,omitempty"`
	BufferSize          int               `json:"buffer_size,omitempty" yaml:"buffer_size,omitempty"`
	LogLevel            string            `json:"log_level,omitempty" yaml:"log_level,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// transportJSON 은 TransportConfig 의 JSON/YAML 직렬화를 위한 보조 구조체이다.
type transportJSON struct {
	Type    string         `json:"type,omitempty" yaml:"type,omitempty"`
	Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
}

// ---------------------------------------------------------------------------
// 내부 변환 헬퍼
// ---------------------------------------------------------------------------

// toJSON 은 AgentConfig 를 agentConfigJSON 중간 구조체로 변환한다.
// time.Duration 은 문자열 표현으로 변환한다 (예: 30*time.Second → "30s").
// Duration 이 0 이면 빈 문자열로 변환한다.
func toJSON(config AgentConfig) agentConfigJSON {
	var interval string
	if config.HealthCheckInterval != 0 {
		interval = config.HealthCheckInterval.String()
	}

	return agentConfigJSON{
		ID:   config.ID,
		Name: config.Name,
		Type: config.Type,
		Transport: transportJSON{
			Type:    config.Transport.Type,
			Options: config.Transport.Options,
		},
		ProtocolFile:        config.ProtocolFile,
		HealthCheckInterval: interval,
		MaxRestarts:         config.MaxRestarts,
		StopOnZeroRef:       config.StopOnZeroRef,
		BufferSize:          config.BufferSize,
		LogLevel:            config.LogLevel,
		Metadata:            config.Metadata,
	}
}

// fromJSON 은 agentConfigJSON 중간 구조체를 AgentConfig 로 변환한다.
// health_check_interval 문자열을 time.Duration 으로 파싱한다.
// 빈 문자열이면 0 Duration 으로 변환한다.
func fromJSON(j agentConfigJSON) (AgentConfig, error) {
	var interval time.Duration
	if j.HealthCheckInterval != "" {
		parsed, err := time.ParseDuration(j.HealthCheckInterval)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("health_check_interval 파싱 실패: %w", err)
		}
		interval = parsed
	}

	return AgentConfig{
		ID:   j.ID,
		Name: j.Name,
		Type: j.Type,
		Transport: TransportConfig{
			Type:    j.Transport.Type,
			Options: j.Transport.Options,
		},
		ProtocolFile:        j.ProtocolFile,
		HealthCheckInterval: interval,
		MaxRestarts:         j.MaxRestarts,
		StopOnZeroRef:       j.StopOnZeroRef,
		BufferSize:          j.BufferSize,
		LogLevel:            j.LogLevel,
		Metadata:            j.Metadata,
	}, nil
}

// ---------------------------------------------------------------------------
// 공개 직렬화 함수
// ---------------------------------------------------------------------------

// AgentConfigToJSON 은 AgentConfig 를 JSON 바이트로 직렬화한다.
// time.Duration 필드는 문자열로 직렬화된다 (예: "30s", "5m").
func AgentConfigToJSON(config AgentConfig) ([]byte, error) {
	j := toJSON(config)
	data, err := json.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("json 직렬화 실패: %w", err)
	}
	return data, nil
}

// AgentConfigFromJSON 은 JSON 바이트를 AgentConfig 로 역직렬화한다.
// health_check_interval 문자열은 time.Duration 으로 파싱된다.
func AgentConfigFromJSON(data []byte) (AgentConfig, error) {
	var j agentConfigJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return AgentConfig{}, fmt.Errorf("json 역직렬화 실패: %w", err)
	}
	return fromJSON(j)
}

// AgentConfigToYAML 은 AgentConfig 를 YAML 바이트로 직렬화한다.
// time.Duration 필드는 문자열로 직렬화된다 (예: "30s", "5m").
func AgentConfigToYAML(config AgentConfig) ([]byte, error) {
	j := toJSON(config)
	data, err := yaml.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("yaml 직렬화 실패: %w", err)
	}
	return data, nil
}

// AgentConfigFromYAML 은 YAML 바이트를 AgentConfig 로 역직렬화한다.
// health_check_interval 문자열은 time.Duration 으로 파싱된다.
func AgentConfigFromYAML(data []byte) (AgentConfig, error) {
	var j agentConfigJSON
	if err := yaml.Unmarshal(data, &j); err != nil {
		return AgentConfig{}, fmt.Errorf("yaml 역직렬화 실패: %w", err)
	}
	return fromJSON(j)
}
