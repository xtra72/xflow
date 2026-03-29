package lg

import (
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// TestParseLGAPConfig_FullValid verifies all fields are correctly parsed.
func TestParseLGAPConfig_FullValid(t *testing.T) {
	opts := map[string]any{
		"serial_port":         "/dev/ttyUSB0",
		"baud_rate":           9600,
		"data_bits":           7,
		"stop_bits":           2,
		"parity":              "even",
		"read_timeout":        "1s",
		"poll_interval":       "1m",
		"inter_command_delay": "100ms",
		"devices": []any{
			map[string]any{"address": "0x10", "name": "living-room"},
			map[string]any{"address": "0x11", "name": "bedroom"},
			map[string]any{"address": "0x20"},
		},
		"msg_channel_size":      512,
		"offline_threshold":     5,
		"reconnect_interval":    "10s",
		"max_reconnect_backoff": "2m",
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
	}

	if cfg.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("SerialPort = %q, want %q", cfg.SerialPort, "/dev/ttyUSB0")
	}
	if cfg.BaudRate != 9600 {
		t.Errorf("BaudRate = %d, want %d", cfg.BaudRate, 9600)
	}
	if cfg.DataBits != 7 {
		t.Errorf("DataBits = %d, want %d", cfg.DataBits, 7)
	}
	if cfg.StopBits != 2 {
		t.Errorf("StopBits = %d, want %d", cfg.StopBits, 2)
	}
	if cfg.Parity != "even" {
		t.Errorf("Parity = %q, want %q", cfg.Parity, "even")
	}
	if cfg.ReadTimeout != 1*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 1*time.Second)
	}
	if cfg.PollInterval != 1*time.Minute {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 1*time.Minute)
	}
	if cfg.InterCommandDelay != 100*time.Millisecond {
		t.Errorf("InterCommandDelay = %v, want %v", cfg.InterCommandDelay, 100*time.Millisecond)
	}
	if len(cfg.Devices) != 3 {
		t.Fatalf("len(Devices) = %d, want %d", len(cfg.Devices), 3)
	}
	expectedDevices := []agent.DeviceEntry{
		{Address: "0x10", Name: "living-room"},
		{Address: "0x11", Name: "bedroom"},
		{Address: "0x20", Name: ""},
	}
	for i, want := range expectedDevices {
		if cfg.Devices[i].Address != want.Address {
			t.Errorf("Devices[%d].Address = %q, want %q", i, cfg.Devices[i].Address, want.Address)
		}
		if cfg.Devices[i].Name != want.Name {
			t.Errorf("Devices[%d].Name = %q, want %q", i, cfg.Devices[i].Name, want.Name)
		}
	}
	if cfg.MsgChannelSize != 512 {
		t.Errorf("MsgChannelSize = %d, want %d", cfg.MsgChannelSize, 512)
	}
	if cfg.OfflineThreshold != 5 {
		t.Errorf("OfflineThreshold = %d, want %d", cfg.OfflineThreshold, 5)
	}
	if cfg.ReconnectInterval != 10*time.Second {
		t.Errorf("ReconnectInterval = %v, want %v", cfg.ReconnectInterval, 10*time.Second)
	}
	if cfg.MaxReconnectBackoff != 2*time.Minute {
		t.Errorf("MaxReconnectBackoff = %v, want %v", cfg.MaxReconnectBackoff, 2*time.Minute)
	}
}

// TestParseLGAPConfig_MinimalValid verifies default values with minimal config.
func TestParseLGAPConfig_MinimalValid(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
	}

	if cfg.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("SerialPort = %q, want %q", cfg.SerialPort, "/dev/ttyUSB0")
	}
}

// TestParseLGAPConfig_Defaults verifies all default values are correctly applied.
func TestParseLGAPConfig_Defaults(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "BaudRate", got: cfg.BaudRate, want: 4800},
		{name: "DataBits", got: cfg.DataBits, want: 8},
		{name: "StopBits", got: cfg.StopBits, want: 1},
		{name: "Parity", got: cfg.Parity, want: "none"},
		{name: "ReadTimeout", got: cfg.ReadTimeout, want: 500 * time.Millisecond},
		{name: "PollInterval", got: cfg.PollInterval, want: 30 * time.Second},
		{name: "InterCommandDelay", got: cfg.InterCommandDelay, want: 50 * time.Millisecond},
		{name: "MsgChannelSize", got: cfg.MsgChannelSize, want: 256},
		{name: "OfflineThreshold", got: cfg.OfflineThreshold, want: 3},
		{name: "ReconnectInterval", got: cfg.ReconnectInterval, want: 5 * time.Second},
		{name: "MaxReconnectBackoff", got: cfg.MaxReconnectBackoff, want: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestParseLGAPConfig_MissingSerialPort verifies error on missing serial_port.
func TestParseLGAPConfig_MissingSerialPort(t *testing.T) {
	opts := map[string]any{}

	_, err := parseLGAPConfig(opts)
	if err == nil {
		t.Fatal("parseLGAPConfig() expected error for missing serial_port, got nil")
	}
}

// TestParseLGAPConfig_EmptySerialPort verifies error on empty serial_port.
func TestParseLGAPConfig_EmptySerialPort(t *testing.T) {
	opts := map[string]any{
		"serial_port": "",
	}

	_, err := parseLGAPConfig(opts)
	if err == nil {
		t.Fatal("parseLGAPConfig() expected error for empty serial_port, got nil")
	}
}

// TestParseLGAPConfig_DurationParsing verifies various duration formats.
func TestParseLGAPConfig_DurationParsing(t *testing.T) {
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
		{name: "inter_command_delay ms", key: "inter_command_delay", value: "100ms", field: "InterCommandDelay", expected: 100 * time.Millisecond},
		{name: "reconnect_interval", key: "reconnect_interval", value: "10s", field: "ReconnectInterval", expected: 10 * time.Second},
		{name: "max_reconnect_backoff", key: "max_reconnect_backoff", value: "2m", field: "MaxReconnectBackoff", expected: 2 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"serial_port": "/dev/ttyUSB0",
				tt.key:        tt.value,
			}
			cfg, err := parseLGAPConfig(opts)
			if err != nil {
				t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
			}

			var got time.Duration
			switch tt.field {
			case "PollInterval":
				got = cfg.PollInterval
			case "ReadTimeout":
				got = cfg.ReadTimeout
			case "InterCommandDelay":
				got = cfg.InterCommandDelay
			case "ReconnectInterval":
				got = cfg.ReconnectInterval
			case "MaxReconnectBackoff":
				got = cfg.MaxReconnectBackoff
			}

			if got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, got, tt.expected)
			}
		})
	}
}

// TestParseLGAPConfig_InvalidDuration verifies error on invalid duration strings.
func TestParseLGAPConfig_InvalidDuration(t *testing.T) {
	durationKeys := []string{
		"read_timeout",
		"poll_interval",
		"inter_command_delay",
		"reconnect_interval",
		"max_reconnect_backoff",
	}

	for _, key := range durationKeys {
		t.Run(key, func(t *testing.T) {
			opts := map[string]any{
				"serial_port": "/dev/ttyUSB0",
				key:           "not-a-duration",
			}
			_, err := parseLGAPConfig(opts)
			if err == nil {
				t.Errorf("parseLGAPConfig() expected error for invalid %s, got nil", key)
			}
		})
	}
}

// TestParseLGAPConfig_DevicesAsSliceAny verifies YAML-compatible devices parsing.
func TestParseLGAPConfig_DevicesAsSliceAny(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"devices": []any{
			map[string]any{"address": "0x10", "name": "living-room"},
			map[string]any{"address": "0x11", "name": "bedroom"},
			map[string]any{"address": "0x20", "name": "kitchen"},
		},
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
	}

	if len(cfg.Devices) != 3 {
		t.Fatalf("len(Devices) = %d, want %d", len(cfg.Devices), 3)
	}
	expected := []agent.DeviceEntry{
		{Address: "0x10", Name: "living-room"},
		{Address: "0x11", Name: "bedroom"},
		{Address: "0x20", Name: "kitchen"},
	}
	for i, want := range expected {
		if cfg.Devices[i].Address != want.Address {
			t.Errorf("Devices[%d].Address = %q, want %q", i, cfg.Devices[i].Address, want.Address)
		}
		if cfg.Devices[i].Name != want.Name {
			t.Errorf("Devices[%d].Name = %q, want %q", i, cfg.Devices[i].Name, want.Name)
		}
	}
}

// TestParseLGAPConfig_DevicesAddressOnly verifies devices with address only (no name).
func TestParseLGAPConfig_DevicesAddressOnly(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"devices": []any{
			map[string]any{"address": "0x10"},
			map[string]any{"address": "0x20"},
		},
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
	}

	if len(cfg.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want %d", len(cfg.Devices), 2)
	}
}

// TestParseLGAPConfig_NumericAsFloat64 verifies YAML compatibility for numeric fields.
func TestParseLGAPConfig_NumericAsFloat64(t *testing.T) {
	opts := map[string]any{
		"serial_port":       "/dev/ttyUSB0",
		"baud_rate":         float64(9600),
		"data_bits":         float64(8),
		"stop_bits":         float64(1),
		"offline_threshold": float64(5),
		"msg_channel_size":  float64(1024),
	}

	cfg, err := parseLGAPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGAPConfig() unexpected error: %v", err)
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

// TestParseZoneKey verifies hex and decimal zone key parsing.
func TestParseZoneKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want int
	}{
		{name: "hex lowercase 0x10", key: "0x10", want: 16},
		{name: "hex uppercase 0X10", key: "0X10", want: 16},
		{name: "hex 0xFF", key: "0xFF", want: 255},
		{name: "decimal 16", key: "16", want: 16},
		{name: "decimal 0", key: "0", want: 0},
		{name: "decimal 255", key: "255", want: 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt(parseZoneKey(tt.key))
			if got != tt.want {
				t.Errorf("toInt(parseZoneKey(%q)) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

// TestToInt verifies int/float64 conversion helper.
func TestToInt(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{name: "int value", val: 42, want: 42},
		{name: "float64 value", val: float64(42), want: 42},
		{name: "float64 with decimal", val: float64(42.9), want: 42},
		{name: "zero int", val: 0, want: 0},
		{name: "zero float64", val: float64(0), want: 0},
		{name: "unsupported type returns 0", val: "42", want: 0},
		{name: "nil returns 0", val: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt(tt.val)
			if got != tt.want {
				t.Errorf("toInt(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}
