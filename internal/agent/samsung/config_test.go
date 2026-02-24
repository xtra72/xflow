package samsung

import (
	"testing"
	"time"
)

// TestParseNASAConfig_FullValid 는 모든 필드가 지정된 설정을 올바르게 파싱하는지 검증한다.
func TestParseNASAConfig_FullValid(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "serial",
		"serial_port":     "/dev/ttyUSB0",
		"baud_rate":       19200,
		"data_bits":       7,
		"stop_bits":       2,
		"parity":          "none",
		"tcp_address":     "192.168.1.100:4196",
		"connect_timeout": "10s",
		"read_timeout":    "5s",
		"poll_interval":   "1m",
		"notify_interval": "500ms",
		"device_addresses": []any{"20 00 01", "200002"},
		"device_ids":       map[string]any{"living-room": "200001", "bedroom": "20 00 02"},
		"protocol_file":   "/etc/xflow/nasa.json",
		"auto_discovery":  true,
		"registry_path":   "/var/lib/xflow/registry.json",
		"offline_threshold": 5,
		"msg_channel_size": 512,
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
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
	if cfg.TCPAddr != "192.168.1.100:4196" {
		t.Errorf("TCPAddr = %q, want %q", cfg.TCPAddr, "192.168.1.100:4196")
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
		t.Errorf("NotifyInterval = %v, want %v", cfg.NotifyInterval, 500*time.Millisecond)
	}
	if len(cfg.DeviceAddresses) != 2 {
		t.Fatalf("len(DeviceAddresses) = %d, want %d", len(cfg.DeviceAddresses), 2)
	}
	if cfg.DeviceAddresses[0] != "20 00 01" {
		t.Errorf("DeviceAddresses[0] = %q, want %q", cfg.DeviceAddresses[0], "20 00 01")
	}
	if cfg.DeviceAddresses[1] != "200002" {
		t.Errorf("DeviceAddresses[1] = %q, want %q", cfg.DeviceAddresses[1], "200002")
	}
	if len(cfg.DeviceIDs) != 2 {
		t.Fatalf("len(DeviceIDs) = %d, want %d", len(cfg.DeviceIDs), 2)
	}
	if cfg.DeviceIDs["living-room"] != "200001" {
		t.Errorf("DeviceIDs[living-room] = %q, want %q", cfg.DeviceIDs["living-room"], "200001")
	}
	if cfg.DeviceIDs["bedroom"] != "20 00 02" {
		t.Errorf("DeviceIDs[bedroom] = %q, want %q", cfg.DeviceIDs["bedroom"], "20 00 02")
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

// TestParseNASAConfig_MinimalValid 는 필수 필드만으로 기본값이 올바르게 적용되는지 검증한다.
func TestParseNASAConfig_MinimalValid(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "serial",
		"device_addresses": []any{"200001"},
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
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
	if cfg.NotifyInterval != 0 {
		t.Errorf("NotifyInterval default = %v, want %v", cfg.NotifyInterval, time.Duration(0))
	}
	if cfg.OfflineThreshold != 3 {
		t.Errorf("OfflineThreshold default = %d, want %d", cfg.OfflineThreshold, 3)
	}
	if cfg.MsgChannelSize != 256 {
		t.Errorf("MsgChannelSize default = %d, want %d", cfg.MsgChannelSize, 256)
	}
	if cfg.AutoDiscovery != false {
		t.Errorf("AutoDiscovery default = %v, want %v", cfg.AutoDiscovery, false)
	}
	if cfg.ProtocolFile != "" {
		t.Errorf("ProtocolFile default = %q, want %q", cfg.ProtocolFile, "")
	}
	if cfg.RegistryPath != "" {
		t.Errorf("RegistryPath default = %q, want %q", cfg.RegistryPath, "")
	}
	if cfg.DeviceIDs != nil {
		t.Errorf("DeviceIDs default = %v, want nil", cfg.DeviceIDs)
	}
}

// TestParseNASAConfig_MissingTransportType 는 transport_type 누락 시 에러를 반환하는지 검증한다.
func TestParseNASAConfig_MissingTransportType(t *testing.T) {
	opts := map[string]any{
		"device_addresses": []any{"200001"},
	}

	_, err := parseNASAConfig(opts)
	if err == nil {
		t.Fatal("parseNASAConfig() expected error for missing transport_type, got nil")
	}
}

// TestParseNASAConfig_MissingDeviceAddresses 는 device_addresses 누락 시 에러를 반환하는지 검증한다.
func TestParseNASAConfig_MissingDeviceAddresses(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
	}

	_, err := parseNASAConfig(opts)
	if err == nil {
		t.Fatal("parseNASAConfig() expected error for missing device_addresses, got nil")
	}
}

// TestParseNASAConfig_EmptyDeviceAddresses 는 빈 device_addresses 시 에러를 반환하는지 검증한다.
func TestParseNASAConfig_EmptyDeviceAddresses(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "serial",
		"device_addresses": []any{},
	}

	_, err := parseNASAConfig(opts)
	if err == nil {
		t.Fatal("parseNASAConfig() expected error for empty device_addresses, got nil")
	}
}

// TestParseNASAConfig_DurationParsing 은 다양한 duration 형식이 올바르게 파싱되는지 검증한다.
func TestParseNASAConfig_DurationParsing(t *testing.T) {
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
				"transport_type":   "serial",
				"device_addresses": []any{"200001"},
				tt.key:             tt.value,
			}
			cfg, err := parseNASAConfig(opts)
			if err != nil {
				t.Fatalf("parseNASAConfig() unexpected error: %v", err)
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

// TestParseNASAConfig_NumericAsFloat64 는 YAML 파싱 호환을 위해
// 숫자 필드가 float64 로 전달될 때 올바르게 처리되는지 검증한다.
func TestParseNASAConfig_NumericAsFloat64(t *testing.T) {
	opts := map[string]any{
		"transport_type":    "serial",
		"device_addresses":  []any{"200001"},
		"baud_rate":         float64(9600),
		"data_bits":         float64(8),
		"stop_bits":         float64(1),
		"offline_threshold": float64(5),
		"msg_channel_size":  float64(1024),
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
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

// TestParseNASAConfig_DeviceAddressesAsSliceAny 는 YAML 파싱에서
// device_addresses 가 []any 로 전달될 때 처리하는지 검증한다.
func TestParseNASAConfig_DeviceAddressesAsSliceAny(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "tcp",
		"tcp_address":      "192.168.1.100:4196",
		"device_addresses": []any{"20 00 01", "200002", "10 00 00"},
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
	}

	if len(cfg.DeviceAddresses) != 3 {
		t.Fatalf("len(DeviceAddresses) = %d, want %d", len(cfg.DeviceAddresses), 3)
	}
	expected := []string{"20 00 01", "200002", "10 00 00"}
	for i, want := range expected {
		if cfg.DeviceAddresses[i] != want {
			t.Errorf("DeviceAddresses[%d] = %q, want %q", i, cfg.DeviceAddresses[i], want)
		}
	}
}

// TestParseNASAConfig_DeviceIDsAsMapStringAny 는 YAML 파싱에서
// device_ids 가 map[string]any 로 전달될 때 처리하는지 검증한다.
func TestParseNASAConfig_DeviceIDsAsMapStringAny(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "serial",
		"device_addresses": []any{"200001"},
		"device_ids":       map[string]any{"living-room": "200001"},
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
	}

	if len(cfg.DeviceIDs) != 1 {
		t.Fatalf("len(DeviceIDs) = %d, want %d", len(cfg.DeviceIDs), 1)
	}
	if cfg.DeviceIDs["living-room"] != "200001" {
		t.Errorf("DeviceIDs[living-room] = %q, want %q", cfg.DeviceIDs["living-room"], "200001")
	}
}

// TestParseNASAConfig_Defaults 는 모든 기본값이 올바르게 적용되는지 테이블 기반으로 검증한다.
func TestParseNASAConfig_Defaults(t *testing.T) {
	opts := map[string]any{
		"transport_type":   "serial",
		"device_addresses": []any{"200001"},
	}

	cfg, err := parseNASAConfig(opts)
	if err != nil {
		t.Fatalf("parseNASAConfig() unexpected error: %v", err)
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
		{name: "NotifyInterval", got: cfg.NotifyInterval, want: time.Duration(0)},
		{name: "OfflineThreshold", got: cfg.OfflineThreshold, want: 3},
		{name: "MsgChannelSize", got: cfg.MsgChannelSize, want: 256},
		{name: "AutoDiscovery", got: cfg.AutoDiscovery, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}
