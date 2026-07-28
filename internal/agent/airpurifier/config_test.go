package airpurifier

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// 옵션형 필드/타임아웃/레지스트리 시드 파싱 커버리지.
func TestParseConfig_OptionalFields(t *testing.T) {
	opts := directOpts()
	opts["tls"] = true
	opts["ca_cert"] = "/path/ca.pem"
	opts["client_id"] = "fixed-cid"
	opts["username"] = "u"
	opts["password"] = "p"
	opts["qos"] = 2
	opts["keep_alive_sec"] = 45
	opts["auto_reconnect"] = false
	opts["offline_timeout"] = "30s"
	opts["control_response_timeout"] = "0s"
	opts["connect_timeout"] = "3s"
	opts["lwt_enabled"] = false
	opts["registry_path"] = "/var/roster.json"
	opts["station_registry_path"] = "/var/stations.json"

	cfg, err := parseAirPurifierConfig(opts)
	require.NoError(t, err)
	assert.True(t, cfg.TLS)
	assert.Equal(t, "fixed-cid", cfg.ClientID)
	assert.Equal(t, byte(2), cfg.QoS)
	assert.Equal(t, 45*time.Second, cfg.KeepAlive)
	assert.False(t, cfg.AutoReconnect)
	assert.Equal(t, 30*time.Second, cfg.OfflineTimeout)
	assert.Equal(t, time.Duration(0), cfg.ControlResponseTimeout)
	assert.Equal(t, 3*time.Second, cfg.ConnectTimeout)
	assert.False(t, cfg.LWTEnabled)
	assert.Equal(t, "/var/roster.json", cfg.RegistryPath)
	assert.Equal(t, "/var/stations.json", cfg.StationRegistryPath)
}

func TestParseConfig_DurationErrors(t *testing.T) {
	opts := directOpts()
	opts["offline_timeout"] = "notaduration"
	_, err := parseAirPurifierConfig(opts)
	assert.Error(t, err)

	opts = directOpts()
	opts["control_response_timeout"] = "-5s"
	_, err = parseAirPurifierConfig(opts)
	assert.Error(t, err)

	opts = directOpts()
	opts["connect_timeout"] = "bad"
	_, err = parseAirPurifierConfig(opts)
	assert.Error(t, err)
}

func TestParseStationRegistry_MapAndSlice(t *testing.T) {
	// map 형태 시드.
	opts := directOpts()
	opts["station_registry"] = map[string]any{
		"gangnam": map[string]any{"line": "line2", "display_name": "강남", "order": 3},
	}
	cfg, err := parseAirPurifierConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.StationRegistry, 1)
	assert.Equal(t, "gangnam", cfg.StationRegistry[0].Station)
	assert.Equal(t, "line2", cfg.StationRegistry[0].Line)
	assert.Equal(t, 3, cfg.StationRegistry[0].Order)

	// slice 형태 시드.
	opts = directOpts()
	opts["station_registry"] = []any{
		map[string]any{"station": "seocho", "line": "line2"},
		map[string]any{"line": "no-station"}, // station 없음 → skip
	}
	cfg, err = parseAirPurifierConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.StationRegistry, 1)
	assert.Equal(t, "seocho", cfg.StationRegistry[0].Station)
}

// payload_mapping 객체형 (name/on_value/off_value/values) 파싱 + station/place/index 디바이스.
func TestParseConfig_ObjectMappingAndDeviceLocation(t *testing.T) {
	opts := directOpts()
	opts["payload_mapping"] = map[string]any{
		"power_field":     map[string]any{"name": "pw", "on_value": "ON", "off_value": "OFF"},
		"fan_speed_field": map[string]any{"name": "fan", "values": map[string]any{"1": "low", "2": "mid", "3": "high"}},
		"online_field":    "online",
	}
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-1", "station": "gangnam", "place": "platform", "index": 2},
	}
	cfg, err := parseAirPurifierConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, "pw", cfg.PayloadMapping.Power.Name)
	assert.Equal(t, "ON", cfg.PayloadMapping.Power.OnValue)
	require.NotNil(t, cfg.PayloadMapping.Online)
	assert.Equal(t, "online", cfg.PayloadMapping.Online.Name)
	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, "gangnam", cfg.Devices[0].Station)
	assert.Equal(t, "platform", cfg.Devices[0].Place)
	assert.Equal(t, 2, cfg.Devices[0].Index)
}

// truthy / decodeBool 의 native·미매핑 분기 커버리지.
func TestDecodeBool_NativeAndMappedMiss(t *testing.T) {
	// native bool 필드에 문자열 "on"/숫자 유입 (truthy 경로).
	native := BoolField{Name: "power"}
	assert.True(t, native.decodeBool("on"))
	assert.False(t, native.decodeBool("off"))
	assert.True(t, native.decodeBool(float64(1)))
	assert.False(t, native.decodeBool(float64(0)))
	assert.True(t, native.decodeBool(true))

	// 매핑형에서 on/off 어느 것과도 다른 값 → false.
	mapped := BoolField{Name: "pw", OnValue: "ON", OffValue: "OFF"}
	assert.False(t, mapped.decodeBool("UNKNOWN"))
	assert.True(t, mapped.decodeBool("ON"))
}

// FeedState (port 모드 상태 입력 포트) 는 direct 와 동일한 디코딩 경로로 로스터를 갱신한다.
func TestFeedState_PortIngress(t *testing.T) {
	opts := map[string]any{
		"transport_mode":  "port",
		"payload_mapping": validPayloadMapping(),
		"devices":         []any{map[string]any{"device_id": "ap-101"}},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	ap.FeedState("ap-101", []byte(`{"power":true,"fan_speed":3}`))
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Power)
	assert.Equal(t, 3, dev.FanSpeed)

	// 미등록 디바이스 유입은 무시.
	ap.FeedState("unknown", []byte(`{"power":true}`))
	// 잘못된 JSON 유입은 무시(패닉 없음).
	ap.FeedState("ap-101", []byte(`not json`))
}

// Configure 는 옵션을 재파싱하여 cfg 를 갱신한다.
func TestConfigure(t *testing.T) {
	a, err := NewAirPurifierAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	newOpts := directOpts()
	newOpts["qos"] = 2
	require.NoError(t, ap.Configure(baseAgentConfig(newOpts)))
	assert.Equal(t, byte(2), ap.cfg.QoS)

	// Transport.Options 없는 Configure 는 agentConfig 만 갱신.
	cfg := agent.AgentConfig{ID: "ap-agent-1", Name: "renamed", Type: agentType}
	require.NoError(t, ap.Configure(cfg))
	assert.Equal(t, "renamed", ap.Name())

	// 유효하지 않은 config 는 에러.
	assert.Error(t, ap.Configure(agent.AgentConfig{}))
}
