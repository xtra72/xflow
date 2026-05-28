package century

import (
	"errors"
	"testing"
	"time"
)

func TestParseHvacr01Config_Defaults(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.TransportType != "serial" {
		t.Errorf("TransportType = %q, want serial", cfg.TransportType)
	}
	if cfg.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("SerialPort = %q, want /dev/ttyUSB0", cfg.SerialPort)
	}
	if cfg.BaudRate != 9600 {
		t.Errorf("BaudRate = %d, want 9600", cfg.BaudRate)
	}
	if cfg.DataBits != 8 {
		t.Errorf("DataBits = %d, want 8", cfg.DataBits)
	}
	if cfg.StopBits != 1 {
		t.Errorf("StopBits = %d, want 1", cfg.StopBits)
	}
	if cfg.Parity != "none" {
		t.Errorf("Parity = %q, want none", cfg.Parity)
	}
	if cfg.MasterAddress != 0x0030 {
		t.Errorf("MasterAddress = 0x%04X, want 0x0030", cfg.MasterAddress)
	}
	if cfg.SlaveAddress != 0x0001 {
		t.Errorf("SlaveAddress = 0x%04X, want 0x0001", cfg.SlaveAddress)
	}
	if cfg.SubDevID != 0x3B {
		t.Errorf("SubDevID = 0x%02X, want 0x3B", cfg.SubDevID)
	}
	if cfg.RingBufferSize != 128 {
		t.Errorf("RingBufferSize = %d, want 128", cfg.RingBufferSize)
	}
	if cfg.OfflineTimeout != 5*time.Second {
		t.Errorf("OfflineTimeout = %s, want 5s", cfg.OfflineTimeout)
	}
	if !cfg.AutoDiscovery {
		t.Errorf("AutoDiscovery = false, want true (default)")
	}
	if !cfg.DedupeWrites {
		t.Errorf("DedupeWrites = false, want true (default)")
	}
	if cfg.CycleIdleTimeout != 100*time.Millisecond {
		t.Errorf("CycleIdleTimeout = %s, want 100ms", cfg.CycleIdleTimeout)
	}
	if cfg.LogDecodeErrors {
		t.Errorf("LogDecodeErrors = true, want false (default)")
	}
	if cfg.LogDrops {
		t.Errorf("LogDrops = true, want false (default)")
	}
	if cfg.LogUnconfirmedFields {
		t.Errorf("LogUnconfirmedFields = true, want false (default)")
	}
	// v0.5.1: device-centric emit defaults (register-decoded stream 제거).
	if !cfg.EmitDeviceState {
		t.Errorf("EmitDeviceState = false, want true (default)")
	}
	if cfg.ReportInterval != DefaultReportInterval {
		t.Errorf("ReportInterval = %s, want %s (default)", cfg.ReportInterval, DefaultReportInterval)
	}
	if DefaultReportInterval != 60*time.Second {
		t.Errorf("DefaultReportInterval = %s, want 60s", DefaultReportInterval)
	}
}

// ---------------------------------------------------------------------------
// v0.3.0 (M7) — Group H device-centric output config tests (REQ-CENTURY-034).
// ---------------------------------------------------------------------------

// TestParseHvacr01Config_DeviceStateEmitOverrides covers v0.5.1 emit options.
// v0.5.1: emit_device_state=false → ErrHvacr01NoOutputEnabled (단일 stream).
//
// 본 테스트는 keepalive_interval 의 override 만 검증한다. emit_device_state=false
// 단독 케이스는 별도 AC-H9 회귀 테스트에서 다룬다.
func TestParseHvacr01Config_DeviceStateEmitOverrides(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"keepalive_interval": "30s",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if !cfg.EmitDeviceState {
		t.Errorf("EmitDeviceState = false, want true (default)")
	}
	if cfg.ReportInterval != 30*time.Second {
		t.Errorf("ReportInterval = %s, want 30s", cfg.ReportInterval)
	}
}

// AC-H9: emit_device_state=false + emit_register_decoded=false → ErrHvacr01NoOutputEnabled.
func TestParseHvacr01Config_BothEmitOptionsOff_ReturnsErrHvacr01NoOutputEnabled(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":           "/dev/ttyUSB0",
		"emit_device_state":     false,
		"emit_register_decoded": false,
	})
	if !errors.Is(err, ErrHvacr01NoOutputEnabled) {
		t.Fatalf("err = %v, want ErrHvacr01NoOutputEnabled", err)
	}
}

// keepalive_interval=0 must be accepted (disables keepalive fallback).
func TestParseHvacr01Config_ReportIntervalZero_AcceptedDisablesKeepalive(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"keepalive_interval": "0s",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.ReportInterval != 0 {
		t.Errorf("ReportInterval = %s, want 0s", cfg.ReportInterval)
	}
}

// keepalive_interval=2s test-friendly override is accepted.
func TestParseHvacr01Config_ReportIntervalShortDuration(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"keepalive_interval": "2s",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.ReportInterval != 2*time.Second {
		t.Errorf("ReportInterval = %s, want 2s", cfg.ReportInterval)
	}
}

// Negative keepalive_interval is rejected.
func TestParseHvacr01Config_ReportIntervalNegative_Rejected(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"keepalive_interval": "-1s",
	})
	if err == nil {
		t.Fatalf("err = nil, want error for negative keepalive_interval")
	}
}

func TestParseHvacr01Config_MissingSerialPort(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{})
	if !errors.Is(err, ErrSerialPortRequired) {
		t.Fatalf("err = %v, want ErrSerialPortRequired", err)
	}
}

func TestParseHvacr01Config_UnknownTransportType(t *testing.T) {
	t.Parallel()
	// v0.2.0: unknown transport rejected; tcp-client/server are now valid (REQ-CENTURY-028).
	_, err := parseHvacr01Config(map[string]any{
		"transport_type": "websocket",
		"serial_port":    "/dev/ttyUSB0",
	})
	if !errors.Is(err, ErrUnknownTransportType) {
		t.Fatalf("err = %v, want ErrUnknownTransportType", err)
	}
}

// ---------------------------------------------------------------------------
// v0.2.0 (M6) — Group G TCP transport config tests (REQ-CENTURY-028, AC-G7).
// ---------------------------------------------------------------------------

func TestParseHvacr01Config_TCPClient_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.TransportType != "tcp-client" {
		t.Errorf("TransportType = %q, want tcp-client", cfg.TransportType)
	}
	if cfg.TCPHost != "192.168.1.100" {
		t.Errorf("TCPHost = %q, want 192.168.1.100", cfg.TCPHost)
	}
	if cfg.TCPPort != 4196 {
		t.Errorf("TCPPort = %d, want 4196", cfg.TCPPort)
	}
	// Defaults are populated.
	if cfg.TCPConnectTimeout != DefaultTCPConnectTimeout {
		t.Errorf("TCPConnectTimeout = %s, want %s", cfg.TCPConnectTimeout, DefaultTCPConnectTimeout)
	}
	if cfg.TCPReadTimeout != DefaultTCPReadTimeout {
		t.Errorf("TCPReadTimeout = %s, want %s", cfg.TCPReadTimeout, DefaultTCPReadTimeout)
	}
	if cfg.ReconnectInitial != DefaultReconnectInitial {
		t.Errorf("ReconnectInitial = %s, want %s", cfg.ReconnectInitial, DefaultReconnectInitial)
	}
	if cfg.MaxReconnectBackoff != DefaultMaxReconnectBackoff {
		t.Errorf("MaxReconnectBackoff = %s, want %s", cfg.MaxReconnectBackoff, DefaultMaxReconnectBackoff)
	}
}

func TestParseHvacr01Config_TCPServer_DefaultsHost(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-server",
		"tcp_port":       4197,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.TransportType != "tcp-server" {
		t.Errorf("TransportType = %q, want tcp-server", cfg.TransportType)
	}
	if cfg.TCPHost != "0.0.0.0" {
		t.Errorf("TCPHost = %q, want 0.0.0.0 (default for tcp-server)", cfg.TCPHost)
	}
	if cfg.TCPPort != 4197 {
		t.Errorf("TCPPort = %d, want 4197", cfg.TCPPort)
	}
}

func TestParseHvacr01Config_TCPClient_MissingHost(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-client",
		"tcp_port":       4196,
	})
	if !errors.Is(err, ErrHvacr01TCPHostRequired) {
		t.Fatalf("err = %v, want ErrHvacr01TCPHostRequired", err)
	}
}

func TestParseHvacr01Config_TCPClient_MissingPort(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
	})
	if !errors.Is(err, ErrHvacr01TCPPortRequired) {
		t.Fatalf("err = %v, want ErrHvacr01TCPPortRequired", err)
	}
}

func TestParseHvacr01Config_TCPServer_MissingPort(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-server",
	})
	if !errors.Is(err, ErrHvacr01TCPPortRequired) {
		t.Fatalf("err = %v, want ErrHvacr01TCPPortRequired", err)
	}
}

func TestParseHvacr01Config_TCPPort_OutOfRange(t *testing.T) {
	t.Parallel()
	for _, port := range []int{0, -1, 65536, 100000} {
		_, err := parseHvacr01Config(map[string]any{
			"transport_type": "tcp-client",
			"tcp_host":       "127.0.0.1",
			"tcp_port":       port,
		})
		if !errors.Is(err, ErrHvacr01TCPPortRequired) {
			t.Errorf("port=%d: err = %v, want ErrHvacr01TCPPortRequired", port, err)
		}
	}
}

func TestParseHvacr01Config_TCPMode_SkipsSerialPortRequirement(t *testing.T) {
	t.Parallel()
	// tcp-client/tcp-server modes do not require serial_port (REQ-CENTURY-028).
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
		"tcp_port":       4196,
	})
	if err != nil {
		t.Fatalf("tcp-client without serial_port: err = %v, want nil", err)
	}
	if cfg.SerialPort != "" {
		t.Errorf("SerialPort = %q, want empty when tcp-client", cfg.SerialPort)
	}

	cfg, err = parseHvacr01Config(map[string]any{
		"transport_type": "tcp-server",
		"tcp_port":       4197,
	})
	if err != nil {
		t.Fatalf("tcp-server without serial_port: err = %v, want nil", err)
	}
	if cfg.SerialPort != "" {
		t.Errorf("SerialPort = %q, want empty when tcp-server", cfg.SerialPort)
	}
}

func TestParseHvacr01Config_TCPTimeouts_Override(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type":        "tcp-client",
		"tcp_host":              "192.168.1.100",
		"tcp_port":              4196,
		"tcp_connect_timeout":   "10s",
		"tcp_read_timeout":      "7s",
		"reconnect_initial":     "1s",
		"max_reconnect_backoff": "30s",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.TCPConnectTimeout != 10*time.Second {
		t.Errorf("TCPConnectTimeout = %s, want 10s", cfg.TCPConnectTimeout)
	}
	if cfg.TCPReadTimeout != 7*time.Second {
		t.Errorf("TCPReadTimeout = %s, want 7s", cfg.TCPReadTimeout)
	}
	if cfg.ReconnectInitial != 1*time.Second {
		t.Errorf("ReconnectInitial = %s, want 1s", cfg.ReconnectInitial)
	}
	if cfg.MaxReconnectBackoff != 30*time.Second {
		t.Errorf("MaxReconnectBackoff = %s, want 30s", cfg.MaxReconnectBackoff)
	}
}

// AC-G7: Transport-aware cycle_idle_timeout default.
func TestParseHvacr01Config_CycleIdleTimeoutDefault_TransportAware(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		opts          map[string]any
		wantTransport string
		wantTimeout   time.Duration
	}{
		{
			name: "serial default = 100ms",
			opts: map[string]any{
				"serial_port": "/dev/ttyUSB0",
			},
			wantTransport: "serial",
			wantTimeout:   DefaultCycleIdleTimeoutSerial,
		},
		{
			name: "tcp-client default = 200ms",
			opts: map[string]any{
				"transport_type": "tcp-client",
				"tcp_host":       "127.0.0.1",
				"tcp_port":       4196,
			},
			wantTransport: "tcp-client",
			wantTimeout:   DefaultCycleIdleTimeoutTCP,
		},
		{
			name: "tcp-server default = 200ms",
			opts: map[string]any{
				"transport_type": "tcp-server",
				"tcp_port":       4197,
			},
			wantTransport: "tcp-server",
			wantTimeout:   DefaultCycleIdleTimeoutTCP,
		},
		{
			name: "explicit serial override wins",
			opts: map[string]any{
				"serial_port":        "/dev/ttyUSB0",
				"cycle_idle_timeout": "150ms",
			},
			wantTransport: "serial",
			wantTimeout:   150 * time.Millisecond,
		},
		{
			name: "explicit tcp override wins regardless of transport",
			opts: map[string]any{
				"transport_type":     "tcp-client",
				"tcp_host":           "127.0.0.1",
				"tcp_port":           4196,
				"cycle_idle_timeout": "150ms",
			},
			wantTransport: "tcp-client",
			wantTimeout:   150 * time.Millisecond,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseHvacr01Config(tc.opts)
			if err != nil {
				t.Fatalf("parseHvacr01Config: %v", err)
			}
			if cfg.TransportType != tc.wantTransport {
				t.Errorf("TransportType = %q, want %q", cfg.TransportType, tc.wantTransport)
			}
			if cfg.CycleIdleTimeout != tc.wantTimeout {
				t.Errorf("CycleIdleTimeout = %s, want %s", cfg.CycleIdleTimeout, tc.wantTimeout)
			}
		})
	}
}

func TestParseHvacr01Config_DefaultConstants(t *testing.T) {
	t.Parallel()
	// Pin the public default constants so any silent change is caught.
	if DefaultCycleIdleTimeoutSerial != 100*time.Millisecond {
		t.Errorf("DefaultCycleIdleTimeoutSerial = %s, want 100ms", DefaultCycleIdleTimeoutSerial)
	}
	if DefaultCycleIdleTimeoutTCP != 200*time.Millisecond {
		t.Errorf("DefaultCycleIdleTimeoutTCP = %s, want 200ms", DefaultCycleIdleTimeoutTCP)
	}
	if DefaultTCPConnectTimeout != 5*time.Second {
		t.Errorf("DefaultTCPConnectTimeout = %s, want 5s", DefaultTCPConnectTimeout)
	}
	if DefaultTCPReadTimeout != 3*time.Second {
		t.Errorf("DefaultTCPReadTimeout = %s, want 3s", DefaultTCPReadTimeout)
	}
	if DefaultReconnectInitial != 5*time.Second {
		t.Errorf("DefaultReconnectInitial = %s, want 5s", DefaultReconnectInitial)
	}
	if DefaultMaxReconnectBackoff != 5*time.Minute {
		t.Errorf("DefaultMaxReconnectBackoff = %s, want 5m", DefaultMaxReconnectBackoff)
	}
}

func TestParseHvacr01Config_HexAddresses(t *testing.T) {
	t.Parallel()
	// AC-D3: hex string input is accepted.
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"master_address": "0x0030",
		"slave_address":  "0x0001",
		"sub_dev_id":     "0x3B",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.MasterAddress != 0x0030 {
		t.Errorf("MasterAddress = 0x%04X, want 0x0030", cfg.MasterAddress)
	}
	if cfg.SlaveAddress != 0x0001 {
		t.Errorf("SlaveAddress = 0x%04X, want 0x0001", cfg.SlaveAddress)
	}
	if cfg.SubDevID != 0x3B {
		t.Errorf("SubDevID = 0x%02X, want 0x3B", cfg.SubDevID)
	}
}

func TestParseHvacr01Config_IntegerAddresses(t *testing.T) {
	t.Parallel()
	// AC-D3: integer input is also accepted.
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"master_address": 48,
		"slave_address":  1,
		"sub_dev_id":     59,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.MasterAddress != 0x0030 {
		t.Errorf("MasterAddress = 0x%04X, want 0x0030", cfg.MasterAddress)
	}
	if cfg.SlaveAddress != 0x0001 {
		t.Errorf("SlaveAddress = 0x%04X, want 0x0001", cfg.SlaveAddress)
	}
	if cfg.SubDevID != 0x3B {
		t.Errorf("SubDevID = 0x%02X, want 0x3B", cfg.SubDevID)
	}
}

func TestParseHvacr01Config_InvalidBaudRate(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   0,
	})
	if !errors.Is(err, ErrInvalidBaudRate) {
		t.Fatalf("err = %v, want ErrInvalidBaudRate", err)
	}
}

func TestParseHvacr01Config_InvalidRingBufferSize(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":      "/dev/ttyUSB0",
		"ring_buffer_size": 8,
	})
	if !errors.Is(err, ErrInvalidRingBufferSize) {
		t.Fatalf("err = %v, want ErrInvalidRingBufferSize", err)
	}
}

func TestParseHvacr01Config_InvalidOfflineTimeout(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":     "/dev/ttyUSB0",
		"offline_timeout": "0s",
	})
	if !errors.Is(err, ErrInvalidOfflineTimeout) {
		t.Fatalf("err = %v, want ErrInvalidOfflineTimeout", err)
	}
}

func TestParseHvacr01Config_InvalidCycleIdleTimeout(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"cycle_idle_timeout": "0s",
	})
	if !errors.Is(err, ErrInvalidCycleIdleTimeout) {
		t.Fatalf("err = %v, want ErrInvalidCycleIdleTimeout", err)
	}
}

func TestParseHvacr01Config_DurationsAndBooleans(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port":            "/dev/ttyUSB0",
		"offline_timeout":        "200ms",
		"cycle_idle_timeout":     "50ms",
		"auto_discovery":         false,
		"dedupe_writes":          false,
		"log_decode_errors":      true,
		"log_drops":              true,
		"log_unconfirmed_fields": true,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if cfg.OfflineTimeout != 200*time.Millisecond {
		t.Errorf("OfflineTimeout = %s, want 200ms", cfg.OfflineTimeout)
	}
	if cfg.CycleIdleTimeout != 50*time.Millisecond {
		t.Errorf("CycleIdleTimeout = %s, want 50ms", cfg.CycleIdleTimeout)
	}
	if cfg.AutoDiscovery {
		t.Errorf("AutoDiscovery = true, want false (override)")
	}
	if cfg.DedupeWrites {
		t.Errorf("DedupeWrites = true, want false (override)")
	}
	if !cfg.LogDecodeErrors || !cfg.LogDrops || !cfg.LogUnconfirmedFields {
		t.Errorf("log flags not all true: %+v", cfg)
	}
}

func TestParseHvacr01Config_InvalidDurationString(t *testing.T) {
	t.Parallel()
	_, err := parseHvacr01Config(map[string]any{
		"serial_port":     "/dev/ttyUSB0",
		"offline_timeout": "not-a-duration",
	})
	if err == nil {
		t.Fatalf("err = nil, want a parse error")
	}
}

func TestParseHvacr01Config_DevicesPassthrough(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"devices": []any{
			map[string]any{"address": "0x3B", "name": "living-room"},
			map[string]any{"address": "0x3C", "name": "bedroom"},
		},
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config returned error: %v", err)
	}
	if got := len(cfg.Devices); got != 2 {
		t.Fatalf("len(Devices) = %d, want 2", got)
	}
	if cfg.Devices[0].Address != "0x3B" {
		t.Errorf("Devices[0].Address = %q, want 0x3B", cfg.Devices[0].Address)
	}
}
