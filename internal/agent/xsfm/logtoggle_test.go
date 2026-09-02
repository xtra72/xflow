package xsfm

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// captureLogger 는 INFO 이상을 버퍼로 캡처하는 slog 로거를 반환한다(로그 게이팅 검증용).
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), &buf
}

// agentConfigWithLogger 는 baseAgentConfig 에 캡처 로거를 주입한다.
func agentConfigWithLogger(opts map[string]any, logger *slog.Logger) agent.AgentConfig {
	cfg := baseAgentConfig(opts)
	cfg.Logger = logger
	return cfg
}

// log_messages / log_mqtt 파싱: true 로 설정 시 필드가 켜지고, 미지정 시 기본 false.
func TestParseConfig_LogToggles(t *testing.T) {
	// 기본값 false.
	cfg, err := parseXSFMConfig(directOpts())
	require.NoError(t, err)
	assert.False(t, cfg.LogMessages)
	assert.False(t, cfg.LogMQTT)

	// 명시적 true.
	opts := directOpts()
	opts["log_messages"] = true
	opts["log_mqtt"] = true
	cfg, err = parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.True(t, cfg.LogMessages)
	assert.True(t, cfg.LogMQTT)
}

// log_messages=true 이면 상태 유입(RX)에 INFO 프레임 로그가 남고, false 이면 남지 않는다.
func TestLogMessages_RXGating(t *testing.T) {
	payload := []byte(`{"power":true,"fan_speed":2}`)

	// ON: RX 로그가 남아야 한다.
	logOn, bufOn := captureLogger()
	optsOn := portOpts()
	optsOn["log_messages"] = true
	aOn, err := NewXSFMAgent(agentConfigWithLogger(optsOn, logOn))
	require.NoError(t, err)
	apOn := asAP(t, aOn)
	bufOn.Reset() // 초기화 로그 제거 후 관측.
	apOn.FeedState("dev-1", payload)
	assert.Contains(t, bufOn.String(), "xsfm: RX", "log_messages=true 이면 RX 프레임 로그가 남아야 한다")
	assert.Contains(t, bufOn.String(), "power=true")

	// OFF: RX 로그가 없어야 한다 (zero overhead).
	logOff, bufOff := captureLogger()
	aOff, err := NewXSFMAgent(agentConfigWithLogger(portOpts(), logOff))
	require.NoError(t, err)
	apOff := asAP(t, aOff)
	bufOff.Reset()
	apOff.FeedState("dev-1", payload)
	assert.NotContains(t, bufOff.String(), "xsfm: RX", "log_messages=false 이면 RX 프레임 로그가 없어야 한다")
}

// log_messages=true 이면 명령 방출(TX)에 INFO 프레임 로그가 남고, false 이면 남지 않는다.
func TestLogMessages_TXGating(t *testing.T) {
	setPower := func() []byte {
		b, _ := json.Marshal(map[string]any{
			"command":   "set_power",
			"device_id": "dev-1",
			"params":    map[string]any{"power": true},
		})
		return b
	}

	newAgent := func(logMessages bool) (*XSFMAgent, *bytes.Buffer) {
		logger, buf := captureLogger()
		opts := portOpts()
		opts["control_response_timeout"] = "0s" // fire-and-forget, pending 대기 없음.
		opts["devices"] = []any{map[string]any{"device_id": "dev-1"}}
		if logMessages {
			opts["log_messages"] = true
		}
		a, err := NewXSFMAgent(agentConfigWithLogger(opts, logger))
		require.NoError(t, err)
		return asAP(t, a), buf
	}

	// ON: TX 로그가 남아야 한다.
	apOn, bufOn := newAgent(true)
	bufOn.Reset()
	_, err := apOn.Process(setPower())
	require.NoError(t, err)
	assert.Contains(t, bufOn.String(), "xsfm: TX", "log_messages=true 이면 TX 프레임 로그가 남아야 한다")
	assert.Contains(t, bufOn.String(), "set_power")

	// OFF: TX 로그가 없어야 한다.
	apOff, bufOff := newAgent(false)
	bufOff.Reset()
	_, err = apOff.Process(setPower())
	require.NoError(t, err)
	assert.NotContains(t, bufOff.String(), "xsfm: TX", "log_messages=false 이면 TX 프레임 로그가 없어야 한다")
}

// attribute-per-topic 모드 TX 로그: 명령 토픽이 축별로 재구성되고 각 축이 로그된다.
// commandHasAttribute 를 켜기 위해 command_topic_template 에 {attribute} 를 둔다(port 모드는
// direct 검증을 타지 않아 임의 템플릿 허용). set_fan_speed 로 fan_speed 축을 방출한다.
func TestLogMessages_TXAttributeMode(t *testing.T) {
	logger, buf := captureLogger()
	opts := map[string]any{
		"transport_mode":           "port",
		"payload_mapping":          validPayloadMapping(),
		"command_topic_template":   "ap/{device_id}/{attribute}/set",
		"control_response_timeout": "0s",
		"log_messages":             true,
		"devices": []any{
			map[string]any{"device_id": "dev-1"},
		},
	}
	a, err := NewXSFMAgent(agentConfigWithLogger(opts, logger))
	require.NoError(t, err)
	ap := asAP(t, a)

	// 디바이스를 전원 ON 상태로 만들어 fan_speed 가 거부되지 않게 한다(power_off 게이트 회피).
	ap.FeedState("dev-1", []byte(`{"power":true}`))
	buf.Reset()

	cmd, _ := json.Marshal(map[string]any{
		"command": "set_fan_speed", "device_id": "dev-1",
		"params": map[string]any{"fan_speed": 3},
	})
	_, err = ap.Process(cmd)
	require.NoError(t, err)

	got := buf.String()
	assert.Contains(t, got, "xsfm: TX")
	// 재구성된 attribute 토픽이 로그에 실린다.
	assert.Contains(t, got, "ap/dev-1/fan_speed/set")
}

// commandSummary 는 blob 모드에서 fan_speed 축 요약을 포함한다(set_multiple 결합 방출).
func TestLogMessages_TXBlobFanSpeed(t *testing.T) {
	logger, buf := captureLogger()
	opts := portOpts()
	opts["control_response_timeout"] = "0s"
	opts["log_messages"] = true
	opts["devices"] = []any{map[string]any{"device_id": "dev-1"}}
	a, err := NewXSFMAgent(agentConfigWithLogger(opts, logger))
	require.NoError(t, err)
	ap := asAP(t, a)
	buf.Reset()

	// power=false + fan_speed → 단일 결합 방출(blob), commandSummary 가 두 축을 요약.
	cmd, _ := json.Marshal(map[string]any{
		"command": "set_multiple", "device_id": "dev-1",
		"params": map[string]any{"power": false, "fan_speed": 2},
	})
	_, err = ap.Process(cmd)
	require.NoError(t, err)

	got := buf.String()
	assert.Contains(t, got, "xsfm: TX")
	assert.Contains(t, got, "fan_speed=2")
}

// log_mqtt 게이팅: pahoMQTTClient.logLifecycle 은 토글이 꺼져 있으면 무출력, 켜져 있으면 출력.
func TestLogMQTT_LifecycleGating(t *testing.T) {
	// OFF: 생명주기 로그 없음.
	logOff, bufOff := captureLogger()
	cOff := newPahoMQTTClient(XSFMConfig{LogMQTT: false}, logOff)
	cOff.logLifecycle("connect succeeded", "broker", "tcp://x:1883")
	assert.Empty(t, strings.TrimSpace(bufOff.String()), "log_mqtt=false 이면 생명주기 로그가 없어야 한다")

	// ON: 생명주기 로그 출력.
	logOn, bufOn := captureLogger()
	cOn := newPahoMQTTClient(XSFMConfig{LogMQTT: true}, logOn)
	cOn.logLifecycle("connect succeeded", "broker", "tcp://x:1883")
	assert.Contains(t, bufOn.String(), "xsfm mqtt: connect succeeded")
}
