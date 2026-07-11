package samsung

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// TestParseHvacr01Config_FullValid 는 모든 필드가 지정된 설정을 올바르게 파싱하는지 검증한다.
func TestParseHvacr01Config_FullValid(t *testing.T) {
	opts := map[string]any{
		"transport_type":  "serial",
		"serial_port":     "/dev/ttyUSB0",
		"baud_rate":       19200,
		"data_bits":       7,
		"stop_bits":       2,
		"parity":          "none",
		"tcp_host":        "192.168.1.100",
		"tcp_port":        4196,
		"connect_timeout": "10s",
		"read_timeout":    "5s",
		"poll_interval":   "1m",
		"report_interval": "500ms",
		"devices": []any{
			map[string]any{"address": "200001", "name": "living-room"},
			map[string]any{"address": "200002", "name": "bedroom"},
		},
		"protocol_file":     "/etc/xflow/nasa.json",
		"auto_discovery":    true,
		"registry_path":     "/var/lib/xflow/registry.json",
		"offline_threshold": 5,
		"msg_channel_size":  512,
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	if cfg.TransportType != "serial" {
		t.Errorf("TransportType = %q, want %q", cfg.TransportType, "serial")
	}
	if cfg.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("SerialPort = %q, want %q", cfg.SerialPort, "/dev/ttyUSB0")
	}
	if cfg.BaudRate != 19200 {
		t.Errorf("BaudRate = %d, want %d", cfg.BaudRate, 19200)
	}
	if cfg.DataBits != 7 {
		t.Errorf("DataBits = %d, want %d", cfg.DataBits, 7)
	}
	if cfg.StopBits != 2 {
		t.Errorf("StopBits = %d, want %d", cfg.StopBits, 2)
	}
	if cfg.Parity != "none" {
		t.Errorf("Parity = %q, want %q", cfg.Parity, "none")
	}
	if cfg.TCPHost != "192.168.1.100" {
		t.Errorf("TCPHost = %q, want %q", cfg.TCPHost, "192.168.1.100")
	}
	if cfg.TCPPort != 4196 {
		t.Errorf("TCPPort = %d, want %d", cfg.TCPPort, 4196)
	}
	if cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("ConnectTimeout = %v, want %v", cfg.ConnectTimeout, 10*time.Second)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 5*time.Second)
	}
	if cfg.PollInterval != time.Minute {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, time.Minute)
	}
	if cfg.NotifyInterval != 500*time.Millisecond {
		t.Errorf("NotifyInterval (report_interval) = %v, want %v", cfg.NotifyInterval, 500*time.Millisecond)
	}
	if len(cfg.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want %d", len(cfg.Devices), 2)
	}
	if cfg.Devices[0] != (agent.DeviceEntry{Address: "200001", Name: "living-room"}) {
		t.Errorf("Devices[0] = %+v, want {Address:200001 Name:living-room}", cfg.Devices[0])
	}
	if cfg.Devices[1] != (agent.DeviceEntry{Address: "200002", Name: "bedroom"}) {
		t.Errorf("Devices[1] = %+v, want {Address:200002 Name:bedroom}", cfg.Devices[1])
	}
	if cfg.ProtocolFile != "/etc/xflow/nasa.json" {
		t.Errorf("ProtocolFile = %q, want %q", cfg.ProtocolFile, "/etc/xflow/nasa.json")
	}
	if !cfg.AutoDiscovery {
		t.Errorf("AutoDiscovery = %v, want %v", cfg.AutoDiscovery, true)
	}
	if cfg.RegistryPath != "/var/lib/xflow/registry.json" {
		t.Errorf("RegistryPath = %q, want %q", cfg.RegistryPath, "/var/lib/xflow/registry.json")
	}
	if cfg.OfflineThreshold != 5 {
		t.Errorf("OfflineThreshold = %d, want %d", cfg.OfflineThreshold, 5)
	}
	if cfg.MsgChannelSize != 512 {
		t.Errorf("MsgChannelSize = %d, want %d", cfg.MsgChannelSize, 512)
	}
}

// TestParseHvacr01Config_MinimalValid 는 필수 필드만으로 기본값이 올바르게 적용되는지 검증한다.
func TestParseHvacr01Config_MinimalValid(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
		"devices":        []any{map[string]any{"address": "200001"}},
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	// 기본값 검증
	if cfg.BaudRate != 9600 {
		t.Errorf("BaudRate default = %d, want %d", cfg.BaudRate, 9600)
	}
	if cfg.DataBits != 8 {
		t.Errorf("DataBits default = %d, want %d", cfg.DataBits, 8)
	}
	if cfg.StopBits != 1 {
		t.Errorf("StopBits default = %d, want %d", cfg.StopBits, 1)
	}
	if cfg.Parity != "even" {
		t.Errorf("Parity default = %q, want %q", cfg.Parity, "even")
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout default = %v, want %v", cfg.ConnectTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("ReadTimeout default = %v, want %v", cfg.ReadTimeout, 3*time.Second)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval default = %v, want %v", cfg.PollInterval, 30*time.Second)
	}
	// 2026-05-29: report_interval (NotifyInterval) 기본값 60s (LG 통일).
	if cfg.NotifyInterval != 60*time.Second {
		t.Errorf("NotifyInterval default = %v, want %v", cfg.NotifyInterval, 60*time.Second)
	}
	if cfg.OfflineThreshold != 3 {
		t.Errorf("OfflineThreshold default = %d, want %d", cfg.OfflineThreshold, 3)
	}
	if cfg.MsgChannelSize != 256 {
		t.Errorf("MsgChannelSize default = %d, want %d", cfg.MsgChannelSize, 256)
	}
	// 2026-05-29: auto_discovery 기본값 true (LG / Century 통일).
	if cfg.AutoDiscovery != true {
		t.Errorf("AutoDiscovery default = %v, want %v", cfg.AutoDiscovery, true)
	}
	if cfg.ProtocolFile != "" {
		t.Errorf("ProtocolFile default = %q, want %q", cfg.ProtocolFile, "")
	}
	if cfg.RegistryPath != "" {
		t.Errorf("RegistryPath default = %q, want %q", cfg.RegistryPath, "")
	}
	// 2026-05-29: include_raw_hex (이전 include_raw_message_sets) 기본값 false (opt-in)
	if cfg.IncludeRawHex != false {
		t.Errorf("IncludeRawHex default = %v, want %v", cfg.IncludeRawHex, false)
	}
}

// TestParseHvacr01Config_IncludeRawHex 는 2026-05-29 이름이 통일된 include_raw_hex
// 옵션이 올바르게 반영되는지 검증한다 (이전: include_raw_message_sets).
func TestParseHvacr01Config_IncludeRawHex(t *testing.T) {
	tests := []struct {
		name string
		opt  any
		want bool
	}{
		{name: "explicit true", opt: true, want: true},
		{name: "explicit false", opt: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"transport_type":  "serial",
				"devices":         []any{map[string]any{"address": "200001"}},
				"include_raw_hex": tt.opt,
			}
			cfg, err := parseHvacr01Config(opts)
			if err != nil {
				t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
			}
			if cfg.IncludeRawHex != tt.want {
				t.Errorf("IncludeRawHex = %v, want %v", cfg.IncludeRawHex, tt.want)
			}
		})
	}
}

// TestParseHvacr01Config_DeprecatedAliasesRejected 는 2026-05-29 breaking 변경으로
// 더 이상 받지 않는 옵션들이 명시적 에러로 거부되는지 검증한다.
func TestParseHvacr01Config_DeprecatedAliasesRejected(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  any
	}{
		{name: "notify_interval", key: "notify_interval", val: "60s"},
		{name: "include_raw_message_sets", key: "include_raw_message_sets", val: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := map[string]any{
				"transport_type": "serial",
				"serial_port":    "/dev/ttyUSB0",
				tc.key:           tc.val,
			}
			_, err := parseHvacr01Config(opts)
			if err == nil {
				t.Fatalf("expected error for deprecated %q, got nil", tc.key)
			}
		})
	}
}

// TestParseHvacr01Config_MissingTransportType 는 transport_type 누락 시 에러를 반환하는지 검증한다.
func TestParseHvacr01Config_MissingTransportType(t *testing.T) {
	opts := map[string]any{
		"devices": []any{map[string]any{"address": "200001"}},
	}

	_, err := parseHvacr01Config(opts)
	if err == nil {
		t.Fatal("parseHvacr01Config() expected error for missing transport_type, got nil")
	}
}

// TestParseHvacr01Config_MissingDevices 는 devices 누락 시 성공하는지 검증한다.
func TestParseHvacr01Config_MissingDevices(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyUSB0",
	}
	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}
	if cfg.Devices != nil {
		t.Errorf("Devices = %v, want nil", cfg.Devices)
	}
}

// TestParseHvacr01Config_EmptyDevices 는 빈 devices 시 성공하는지 검증한다.
func TestParseHvacr01Config_EmptyDevices(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyUSB0",
		"devices":        []any{},
	}
	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}
	if cfg.Devices != nil {
		t.Errorf("Devices = %v, want nil", cfg.Devices)
	}
}

// TestParseHvacr01Config_DurationParsing 은 다양한 duration 형식이 올바르게 파싱되는지 검증한다.
func TestParseHvacr01Config_DurationParsing(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		field    string
		expected time.Duration
	}{
		{name: "seconds", key: "poll_interval", value: "30s", field: "PollInterval", expected: 30 * time.Second},
		{name: "minutes", key: "poll_interval", value: "1m", field: "PollInterval", expected: time.Minute},
		{name: "milliseconds", key: "read_timeout", value: "500ms", field: "ReadTimeout", expected: 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"transport_type": "serial",
				"devices":        []any{map[string]any{"address": "200001"}},
				tt.key:           tt.value,
			}
			cfg, err := parseHvacr01Config(opts)
			if err != nil {
				t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
			}

			var got time.Duration
			switch tt.field {
			case "PollInterval":
				got = cfg.PollInterval
			case "ReadTimeout":
				got = cfg.ReadTimeout
			case "ConnectTimeout":
				got = cfg.ConnectTimeout
			}

			if got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, got, tt.expected)
			}
		})
	}
}

// TestParseHvacr01Config_NumericAsFloat64 는 YAML 파싱 호환을 위해
// 숫자 필드가 float64 로 전달될 때 올바르게 처리되는지 검증한다.
func TestParseHvacr01Config_NumericAsFloat64(t *testing.T) {
	opts := map[string]any{
		"transport_type":    "serial",
		"devices":           []any{map[string]any{"address": "200001"}},
		"baud_rate":         float64(9600),
		"data_bits":         float64(8),
		"stop_bits":         float64(1),
		"offline_threshold": float64(5),
		"msg_channel_size":  float64(1024),
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	if cfg.BaudRate != 9600 {
		t.Errorf("BaudRate = %d, want %d", cfg.BaudRate, 9600)
	}
	if cfg.DataBits != 8 {
		t.Errorf("DataBits = %d, want %d", cfg.DataBits, 8)
	}
	if cfg.StopBits != 1 {
		t.Errorf("StopBits = %d, want %d", cfg.StopBits, 1)
	}
	if cfg.OfflineThreshold != 5 {
		t.Errorf("OfflineThreshold = %d, want %d", cfg.OfflineThreshold, 5)
	}
	if cfg.MsgChannelSize != 1024 {
		t.Errorf("MsgChannelSize = %d, want %d", cfg.MsgChannelSize, 1024)
	}
}

// TestParseHvacr01Config_DevicesMultiple 는 YAML 파싱에서
// devices 가 []any 로 전달될 때 처리하는지 검증한다.
func TestParseHvacr01Config_DevicesMultiple(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
		"devices": []any{
			map[string]any{"address": "200001", "name": "unit-a"},
			map[string]any{"address": "200002"},
			map[string]any{"address": "100000", "name": "unit-c"},
		},
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	require.Len(t, cfg.Devices, 3)
	assert.Equal(t, agent.DeviceEntry{Address: "200001", Name: "unit-a"}, cfg.Devices[0])
	assert.Equal(t, agent.DeviceEntry{Address: "200002"}, cfg.Devices[1])
	assert.Equal(t, agent.DeviceEntry{Address: "100000", Name: "unit-c"}, cfg.Devices[2])
}

// TestParseHvacr01Config_DevicesWithName 는 devices 항목에 name 이 포함될 때 처리하는지 검증한다.
func TestParseHvacr01Config_DevicesWithName(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
		"devices": []any{
			map[string]any{"address": "200001", "name": "living-room"},
		},
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, agent.DeviceEntry{Address: "200001", Name: "living-room"}, cfg.Devices[0])
}

// TestParseHvacr01Config_Defaults 는 모든 기본값이 올바르게 적용되는지 테이블 기반으로 검증한다.
func TestParseHvacr01Config_Defaults(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "BaudRate", got: cfg.BaudRate, want: 9600},
		{name: "DataBits", got: cfg.DataBits, want: 8},
		{name: "StopBits", got: cfg.StopBits, want: 1},
		{name: "Parity", got: cfg.Parity, want: "even"},
		{name: "ConnectTimeout", got: cfg.ConnectTimeout, want: 5 * time.Second},
		{name: "ReadTimeout", got: cfg.ReadTimeout, want: 3 * time.Second},
		{name: "PollInterval", got: cfg.PollInterval, want: 30 * time.Second},
		// 2026-05-29: 통합 기본값.
		{name: "NotifyInterval", got: cfg.NotifyInterval, want: 60 * time.Second},
		{name: "OfflineThreshold", got: cfg.OfflineThreshold, want: 3},
		{name: "MsgChannelSize", got: cfg.MsgChannelSize, want: 256},
		{name: "AutoDiscovery", got: cfg.AutoDiscovery, want: true},
		{name: "ReconnectInterval", got: cfg.ReconnectInterval, want: 5 * time.Second},
		{name: "MaxReconnectBackoff", got: cfg.MaxReconnectBackoff, want: 5 * time.Minute},
	}

	// Devices 는 nil 이어야 한다
	if cfg.Devices != nil {
		t.Errorf("Devices default = %v, want nil", cfg.Devices)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestParseHvacr01Config_UnsupportedMsgSets(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
		"devices":        []any{map[string]any{"address": "200000"}},
		"unsupported_msg_sets": []any{
			0x4100,  // int (YAML 0x4100 → int)
			0x4102,  // int
			16657.0, // float64 (0x4111)
		},
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Len(t, cfg.UnsupportedMsgSets, 3)
	assert.True(t, cfg.UnsupportedMsgSets[0x4100])
	assert.True(t, cfg.UnsupportedMsgSets[0x4102])
	assert.True(t, cfg.UnsupportedMsgSets[0x4111])
}

func TestParseHvacr01Config_UnsupportedMsgSets_Empty(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
		"devices":        []any{map[string]any{"address": "200000"}},
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Nil(t, cfg.UnsupportedMsgSets)
}

// TestParseHvacr01Config_UnsupportedMsgSets_JSONRoundTrip 는 JSON 역직렬화 후
// (float64 값) unsupported_msg_sets 파싱이 정상 동작하는지 검증한다.
// 실제 SQLite DB에서 로드할 때의 시나리오.
func TestParseHvacr01Config_UnsupportedMsgSets_JSONRoundTrip(t *testing.T) {
	// JSON 역직렬화 시 숫자는 float64로 변환됨
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
		"devices":        []any{map[string]any{"address": "200000"}},
		"unsupported_msg_sets": []any{
			float64(0x0608), // 1544.0
			float64(0x060C), // 1548.0
			float64(0x8601), // 34305.0
			float64(0x860C), // 34316.0
			float64(0x860D), // 34317.0
		},
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Len(t, cfg.UnsupportedMsgSets, 5)
	assert.True(t, cfg.UnsupportedMsgSets[0x0608], "0x0608 should be filtered")
	assert.True(t, cfg.UnsupportedMsgSets[0x060C], "0x060C should be filtered")
	assert.True(t, cfg.UnsupportedMsgSets[0x8601], "0x8601 should be filtered")
	assert.True(t, cfg.UnsupportedMsgSets[0x860C], "0x860C should be filtered")
	assert.True(t, cfg.UnsupportedMsgSets[0x860D], "0x860D should be filtered")
}

func TestParseHvacr01Config_ReconnectIntervalCustom(t *testing.T) {
	opts := map[string]any{
		"transport_type":        "tcp-client",
		"tcp_host":              "192.168.1.100",
		"tcp_port":              4196,
		"devices":               []any{map[string]any{"address": "200000"}},
		"reconnect_interval":    "10s",
		"max_reconnect_backoff": "2m",
	}

	cfg, err := parseHvacr01Config(opts)
	if err != nil {
		t.Fatalf("parseHvacr01Config() unexpected error: %v", err)
	}

	if cfg.ReconnectInterval != 10*time.Second {
		t.Errorf("ReconnectInterval = %v, want 10s", cfg.ReconnectInterval)
	}
	if cfg.MaxReconnectBackoff != 2*time.Minute {
		t.Errorf("MaxReconnectBackoff = %v, want 2m", cfg.MaxReconnectBackoff)
	}
}

func TestParseHvacr01Config_ReconnectIntervalInvalid(t *testing.T) {
	opts := map[string]any{
		"transport_type":     "tcp-client",
		"tcp_host":           "192.168.1.100",
		"tcp_port":           4196,
		"devices":            []any{map[string]any{"address": "200000"}},
		"reconnect_interval": "not-a-duration",
	}

	_, err := parseHvacr01Config(opts)
	if err == nil {
		t.Fatal("parseHvacr01Config() should return error for invalid reconnect_interval")
	}
}

func TestParseHvacr01Config_MaxReconnectBackoffInvalid(t *testing.T) {
	opts := map[string]any{
		"transport_type":        "tcp-client",
		"tcp_host":              "192.168.1.100",
		"tcp_port":              4196,
		"devices":               []any{map[string]any{"address": "200000"}},
		"max_reconnect_backoff": "invalid",
	}

	_, err := parseHvacr01Config(opts)
	if err == nil {
		t.Fatal("parseHvacr01Config() should return error for invalid max_reconnect_backoff")
	}
}

// TestParseHvacr01Config_TCPHostAndPort 는 tcp_host / tcp_port 분리 필드가
// 올바르게 Hvacr01Config.TCPHost / TCPPort 에 매핑되는지 검증한다.
// (특성화 테스트: tcp_address 단일 필드를 tcp_host + tcp_port 로 분리하는 리팩토링용)
func TestParseHvacr01Config_TCPHostAndPort(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "10.0.0.5",
		"tcp_port":       4196,
		"devices":        []any{map[string]any{"address": "200000"}},
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", cfg.TCPHost, "TCPHost 가 tcp_host 옵션 값으로 설정되어야 한다")
	assert.Equal(t, 4196, cfg.TCPPort, "TCPPort 가 tcp_port 옵션 값으로 설정되어야 한다")
}

// TestParseHvacr01Config_TCPAddressKeyIgnored 는 레거시 tcp_address 키가
// 더 이상 처리되지 않으며, parse 단계에서 ErrTCPHostRequired 가 즉시 발생함을 검증한다.
// (2026-05-29 tcp-server 추가 후 — tcp-client 는 tcp_host 필수가 parse 단계에서 강제됨.)
func TestParseHvacr01Config_TCPAddressKeyIgnored(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_address":    "10.0.0.5:4196", // 레거시 키는 무시되어야 함
		"devices":        []any{map[string]any{"address": "200000"}},
	}

	_, err := parseHvacr01Config(opts)
	require.Error(t, err, "레거시 tcp_address 만 있고 tcp_host 누락 시 ErrTCPHostRequired 발생")
	require.ErrorIs(t, err, ErrTCPHostRequired)
}

// TestParseHvacr01Config_DeprecatedTCPRejected 는 2026-05-29 breaking change 로
// "tcp" 값이 parse 단계에서 거부되는지 검증한다.
func TestParseHvacr01Config_DeprecatedTCPRejected(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
	}
	_, err := parseHvacr01Config(opts)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeprecatedTCPTransport, "transport_type=tcp 는 breaking change 로 거부되어야 한다")
}

// TestParseHvacr01Config_TCPServerDefaultBindHost 는 tcp-server 모드에서
// tcp_host 미지정 시 "0.0.0.0" 가 기본값으로 설정되는지 검증한다.
func TestParseHvacr01Config_TCPServerDefaultBindHost(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
		"tcp_port":       4196,
	}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	require.Equal(t, "0.0.0.0", cfg.TCPHost, "tcp-server 의 tcp_host 미지정 시 0.0.0.0 가 적용되어야 한다")
	require.Equal(t, 4196, cfg.TCPPort)
}

// TestParseHvacr01Config_TCPServerCustomBindHost 는 tcp-server 모드에서
// 명시적인 tcp_host 가 그대로 사용되는지 검증한다.
func TestParseHvacr01Config_TCPServerCustomBindHost(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
		"tcp_host":       "192.168.1.10",
		"tcp_port":       4196,
	}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	require.Equal(t, "192.168.1.10", cfg.TCPHost)
	require.Equal(t, 4196, cfg.TCPPort)
}

// TestParseHvacr01Config_TCPServerMissingPort 는 tcp-server 모드에서 tcp_port 누락 시
// ErrTCPPortRequired 가 발생함을 검증한다.
func TestParseHvacr01Config_TCPServerMissingPort(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
		"tcp_host":       "0.0.0.0",
	}
	_, err := parseHvacr01Config(opts)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTCPPortRequired)
}

// TestParseHvacr01Config_TCPClientRequiresHost 는 tcp-client 모드에서 tcp_host 누락 시
// parse 단계에서 ErrTCPHostRequired 가 발생함을 검증한다.
func TestParseHvacr01Config_TCPClientRequiresHost(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_port":       4196,
	}
	_, err := parseHvacr01Config(opts)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTCPHostRequired)
}

// ---------------------------------------------------------------------------
// SPEC-HVACR-CONNSTATE-001: connection-state 설정 파싱 테스트
// ---------------------------------------------------------------------------

// TestParseHvacr01Config_ConnectionDefaults (AC-1/AC-2 기본값): 미설정 시 60s.
func TestParseHvacr01Config_ConnectionDefaults(t *testing.T) {
	opts := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0"}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, cfg.ConnectionReportInterval, "기본 60s (AC-1)")
	// poll_interval 기본 30s → 파생 timeout min(60s,30s)=30s (AC-12).
	assert.Equal(t, 30*time.Second, cfg.StartupProbeTimeout)
}

// TestParseHvacr01Config_ConnectionReportInterval (AC-1): 명시값 반영.
func TestParseHvacr01Config_ConnectionReportInterval(t *testing.T) {
	opts := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0", "connection_report_interval": "30s"}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.ConnectionReportInterval)
}

// TestParseHvacr01Config_ConnectionReportZero (AC-2): 0 허용.
func TestParseHvacr01Config_ConnectionReportZero(t *testing.T) {
	opts := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0", "connection_report_interval": "0s"}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), cfg.ConnectionReportInterval)
}

// TestParseHvacr01Config_StartupProbeDerivedCap (AC-12): 파생 30s 캡.
func TestParseHvacr01Config_StartupProbeDerivedCap(t *testing.T) {
	opts := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0", "poll_interval": "20s"}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.StartupProbeTimeout, "min(2*20s,30s)=30s")

	opts["poll_interval"] = "5s"
	cfg, err = parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.StartupProbeTimeout, "min(2*5s,30s)=10s")
}

// TestParseHvacr01Config_StartupProbeExplicitNoCap (AC-12): 명시 override 는 캡 없음.
func TestParseHvacr01Config_StartupProbeExplicitNoCap(t *testing.T) {
	opts := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0", "poll_interval": "20s", "startup_probe_timeout": "60s"}
	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, cfg.StartupProbeTimeout, "명시 60s 는 30s 캡 미적용")
}

// TestDeriveStartupProbeTimeout_PollZeroFallback (AC-12): poll=0 → 10s.
func TestDeriveStartupProbeTimeout_PollZeroFallback(t *testing.T) {
	assert.Equal(t, 10*time.Second, deriveStartupProbeTimeout(0))
	assert.Equal(t, 10*time.Second, deriveStartupProbeTimeout(5*time.Second))
	assert.Equal(t, 30*time.Second, deriveStartupProbeTimeout(20*time.Second))
}

// TestParseHvacr01Config_ConnectionHardErrors (AC-18): 잘못된 값/legacy alias 는 hard error.
func TestParseHvacr01Config_ConnectionHardErrors(t *testing.T) {
	base := map[string]any{"transport_type": "serial", "serial_port": "/dev/ttyUSB0"}

	bad := map[string]any{}
	for k, v := range base {
		bad[k] = v
	}
	bad["connection_report_interval"] = "notaduration"
	_, err := parseHvacr01Config(bad)
	require.Error(t, err, "잘못된 duration 값은 hard error")

	legacy := map[string]any{}
	for k, v := range base {
		legacy[k] = v
	}
	legacy["connection_notify_interval"] = "30s"
	_, err = parseHvacr01Config(legacy)
	require.Error(t, err, "legacy alias 는 hard error")

	badProbe := map[string]any{}
	for k, v := range base {
		badProbe[k] = v
	}
	badProbe["startup_probe_timeout"] = "xyz"
	_, err = parseHvacr01Config(badProbe)
	require.Error(t, err, "잘못된 startup_probe_timeout 은 hard error")
}

// TestParseHvacr01Config_OfflineTimeout3Way 는 offline_timeout 의 3-way 파싱을 검증한다:
// 미설정 → -1 sentinel, 명시적 양수 → verbatim, 명시적 0 → 0(비활성), 음수 → hard error.
func TestParseHvacr01Config_OfflineTimeout3Way(t *testing.T) {
	// 미설정 → -1 sentinel (명시 0 과 구분됨).
	cfg, err := parseHvacr01Config(map[string]any{"transport_type": "serial"})
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), cfg.OfflineTimeout, "미설정 → -1 sentinel")

	// 명시적 양수 → verbatim.
	cfg, err = parseHvacr01Config(map[string]any{"transport_type": "serial", "offline_timeout": "5s"})
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, cfg.OfflineTimeout, "양수 → verbatim")

	// 명시적 0 → 0 (비활성). 미설정 sentinel(-1)과 반드시 달라야 한다.
	cfg, err = parseHvacr01Config(map[string]any{"transport_type": "serial", "offline_timeout": "0s"})
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), cfg.OfflineTimeout, "명시적 0 → 0 (비활성)")

	// 음수 → hard error (기존 검증 유지).
	_, err = parseHvacr01Config(map[string]any{"transport_type": "serial", "offline_timeout": "-1s"})
	require.Error(t, err, "음수 offline_timeout 은 hard error")
}
