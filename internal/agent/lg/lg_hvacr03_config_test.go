package lg

import (
	"errors"
	"testing"
	"time"
)

// baseRTUOpts 는 유효한 최소 RTU 설정을 반환한다.
func baseRTUOpts() map[string]any {
	return map[string]any{
		"transport_type": "rtu",
		"serial_port":    "/dev/ttyUSB0",
	}
}

// ---------------------------------------------------------------------------
// 기본값 (AC-025)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_Defaults(t *testing.T) {
	cfg, err := parseHvacr03Config(baseRTUOpts())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"transport_type", cfg.TransportType, "rtu"},
		{"baud_rate", cfg.BaudRate, 9600},
		{"data_bits", cfg.DataBits, 8},
		{"stop_bits", cfg.StopBits, 1},
		{"parity", cfg.Parity, "none"},
		{"slave_id", cfg.SlaveID, byte(1)},
		{"poll_interval", cfg.PollInterval, 10 * time.Second},
		{"scan_interval", cfg.ScanInterval, 30 * time.Second},
		{"request_timeout", cfg.RequestTimeout, 1 * time.Second},
		{"offline_timeout", cfg.OfflineTimeout, 30 * time.Second},
		{"reconnect_interval", cfg.ReconnectInterval, 5 * time.Second},
		{"max_reconnect_backoff", cfg.MaxReconnectBackoff, 5 * time.Minute},
		{"temp_scale", cfg.TempScale, 10},
		{"address_base", cfg.AddressBase, 0},
		{"fan_auto_code", cfg.FanAutoCode, 4},
		{"auto_discovery", cfg.AutoDiscovery, true},
		{"report_interval", cfg.ReportInterval, 60 * time.Second},
		{"event_temp_threshold", cfg.EventTempThreshold, 1.0},
		{"msg_channel_size", cfg.MsgChannelSize, 256},
		{"control_enabled", cfg.ControlEnabled, false},
		{"control_verify_delay", cfg.ControlVerifyDelay, 3 * time.Second},
		{"log_messages", cfg.LogMessages, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 트랜스포트 검증 (AC-020 ~ AC-022)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_RTURequiresSerialPort(t *testing.T) {
	opts := map[string]any{"transport_type": "rtu"}
	if _, err := parseHvacr03Config(opts); !errors.Is(err, ErrHvacr03SerialPortRequired) {
		t.Fatalf("expected ErrHvacr03SerialPortRequired, got %v", err)
	}
}

func TestParseHvacr03Config_TCPRequiresHost(t *testing.T) {
	opts := map[string]any{"transport_type": "tcp-client"}
	if _, err := parseHvacr03Config(opts); !errors.Is(err, ErrHvacr03TCPHostRequired) {
		t.Fatalf("expected ErrHvacr03TCPHostRequired, got %v", err)
	}
}

// TestParseHvacr03Config_RejectsTCPServer 는 tcp-server 거부를 확인한다.
// lg_hvacr02 는 지원하지만 본 에이전트는 능동 마스터라 서버 모드가 성립하지 않는다.
func TestParseHvacr03Config_RejectsTCPServer(t *testing.T) {
	opts := map[string]any{"transport_type": "tcp-server", "tcp_host": "0.0.0.0", "tcp_port": 502}
	if _, err := parseHvacr03Config(opts); !errors.Is(err, ErrHvacr03UnknownTransportType) {
		t.Fatalf("expected ErrHvacr03UnknownTransportType, got %v", err)
	}
}

func TestParseHvacr03Config_TCPClientValid(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.0.50",
		"tcp_port":       5020,
	}
	cfg, err := parseHvacr03Config(opts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.TCPHost != "192.168.0.50" || cfg.TCPPort != 5020 {
		t.Errorf("tcp config = %s:%d, want 192.168.0.50:5020", cfg.TCPHost, cfg.TCPPort)
	}
}

// ---------------------------------------------------------------------------
// slave_id (AC-024)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_SlaveIDRange(t *testing.T) {
	for _, id := range []int{0, 17, -1, 255} {
		opts := baseRTUOpts()
		opts["slave_id"] = id
		if _, err := parseHvacr03Config(opts); !errors.Is(err, ErrHvacr03InvalidSlaveID) {
			t.Errorf("slave_id=%d should be rejected, got %v", id, err)
		}
	}
	for _, id := range []int{1, 8, 16} {
		opts := baseRTUOpts()
		opts["slave_id"] = id
		cfg, err := parseHvacr03Config(opts)
		if err != nil {
			t.Errorf("slave_id=%d should be accepted: %v", id, err)
			continue
		}
		if cfg.SlaveID != byte(id) {
			t.Errorf("slave_id = %d, want %d", cfg.SlaveID, id)
		}
	}
}

// ---------------------------------------------------------------------------
// poll_interval 하한 (AC-023)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_PollIntervalLowerBound(t *testing.T) {
	for _, s := range []string{"1s", "3s", "4999ms"} {
		opts := baseRTUOpts()
		opts["poll_interval"] = s
		if _, err := parseHvacr03Config(opts); !errors.Is(err, ErrHvacr03PollIntervalTooShort) {
			t.Errorf("poll_interval=%s should be rejected, got %v", s, err)
		}
	}
	// 하한 자체는 허용된다.
	opts := baseRTUOpts()
	opts["poll_interval"] = "5s"
	if _, err := parseHvacr03Config(opts); err != nil {
		t.Errorf("poll_interval=5s should be accepted: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 미검증 가정 설정 (AC-008, AC-011, AC-012)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_UnverifiedAssumptions(t *testing.T) {
	opts := baseRTUOpts()
	opts["temp_scale"] = 1
	opts["address_base"] = 1
	opts["fan_auto_code"] = 5

	cfg, err := parseHvacr03Config(opts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.TempScale != 1 || cfg.AddressBase != 1 || cfg.FanAutoCode != 5 {
		t.Errorf("assumptions = scale %d, base %d, autoCode %d; want 1, 1, 5",
			cfg.TempScale, cfg.AddressBase, cfg.FanAutoCode)
	}
}

func TestParseHvacr03Config_AssumptionRanges(t *testing.T) {
	bad := []struct {
		key string
		val any
	}{
		{"temp_scale", 0},
		{"temp_scale", -10},
		{"address_base", 2},
		{"address_base", -1},
		{"fan_auto_code", 0},
		{"fan_auto_code", 6},
	}
	for _, c := range bad {
		opts := baseRTUOpts()
		opts[c.key] = c.val
		if _, err := parseHvacr03Config(opts); err == nil {
			t.Errorf("%s=%v should be rejected", c.key, c.val)
		}
	}
}

// ---------------------------------------------------------------------------
// 타입 안전성 (AC-026)
// ---------------------------------------------------------------------------

// TestParseHvacr03Config_WrongTypesDoNotPanic 은 잘못된 타입의 YAML 값이
// panic 이 아니라 설정 오류를 내는지 확인한다.
func TestParseHvacr03Config_WrongTypesDoNotPanic(t *testing.T) {
	cases := []struct {
		key string
		val any
	}{
		{"slave_id", "abc"},
		{"slave_id", 1.5},
		{"transport_type", 42},
		{"serial_port", 123},
		{"poll_interval", 10},           // duration 은 문자열이어야 한다
		{"poll_interval", "ten"},        // 파싱 불가
		{"auto_discovery", "yes"},       // bool 이어야 한다
		{"event_temp_threshold", "1.0"}, // 숫자여야 한다
		{"msg_channel_size", "big"},
		{"control_enabled", 1},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on %s=%v: %v", c.key, c.val, r)
				}
			}()
			opts := baseRTUOpts()
			opts[c.key] = c.val
			if _, err := parseHvacr03Config(opts); err == nil {
				t.Errorf("%s=%v should be rejected", c.key, c.val)
			}
		})
	}
}

// TestParseHvacr03Config_IntFlexibility 는 YAML 로더가 돌려주는 여러 정수 표현을
// 모두 받아들이는지 확인한다.
func TestParseHvacr03Config_IntFlexibility(t *testing.T) {
	for _, v := range []any{int(8), int64(8), float64(8)} {
		opts := baseRTUOpts()
		opts["slave_id"] = v
		cfg, err := parseHvacr03Config(opts)
		if err != nil {
			t.Errorf("slave_id=%v (%T) should be accepted: %v", v, v, err)
			continue
		}
		if cfg.SlaveID != 8 {
			t.Errorf("slave_id = %d, want 8", cfg.SlaveID)
		}
	}
}

// ---------------------------------------------------------------------------
// 디바이스 주소 검증 (AC-027)
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_DeviceAddresses(t *testing.T) {
	opts := baseRTUOpts()
	opts["devices"] = []any{
		map[string]any{"address": "0", "name": "거실"},
		map[string]any{"address": "3", "name": "침실"},
	}
	cfg, err := parseHvacr03Config(opts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(cfg.Devices))
	}
	if cfg.Devices[0].Address != "0" || cfg.Devices[1].Address != "3" {
		t.Errorf("addresses = %q, %q", cfg.Devices[0].Address, cfg.Devices[1].Address)
	}
}

// TestParseHvacr03Config_RejectsLGCPHexAddress 는 LGCP 물리 주소 표기를 거부하는지
// 확인한다. Modbus 의 N 은 실외기에 설정된 중앙 주소로 LGCP 물리 주소와 별개 값이다.
func TestParseHvacr03Config_RejectsLGCPHexAddress(t *testing.T) {
	bad := []string{"44550065", "16", "0x03"}
	for _, addr := range bad {
		opts := baseRTUOpts()
		opts["devices"] = []any{map[string]any{"address": addr}}
		if _, err := parseHvacr03Config(opts); err == nil {
			t.Errorf("device address %q should be rejected", addr)
		}
	}
}

// TestParseHvacr03Config_EmptyDeviceAddressDropped 은 주소가 빈 디바이스 항목이
// agent.ParseDevices 단계에서 조용히 버려지는 기존 계약을 고정한다. 본 에이전트의
// 주소 검증에는 도달하지 않으므로 오류가 아니라 무시가 올바른 동작이다.
func TestParseHvacr03Config_EmptyDeviceAddressDropped(t *testing.T) {
	opts := baseRTUOpts()
	opts["devices"] = []any{
		map[string]any{"address": ""},
		map[string]any{"address": "2"},
	}
	cfg, err := parseHvacr03Config(opts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Devices) != 1 || cfg.Devices[0].Address != "2" {
		t.Errorf("devices = %+v, want only address \"2\"", cfg.Devices)
	}
}

// TestParseHvacr03Config_DeviceAddressWithBase1 은 address_base 교정 시 디바이스
// 주소 검증도 함께 이동하는지 확인한다.
func TestParseHvacr03Config_DeviceAddressWithBase1(t *testing.T) {
	opts := baseRTUOpts()
	opts["address_base"] = 1
	opts["devices"] = []any{map[string]any{"address": "16"}}
	if _, err := parseHvacr03Config(opts); err != nil {
		t.Errorf("address 16 with base=1 should be valid (N=15): %v", err)
	}

	opts2 := baseRTUOpts()
	opts2["address_base"] = 1
	opts2["devices"] = []any{map[string]any{"address": "0"}}
	if _, err := parseHvacr03Config(opts2); err == nil {
		t.Error("address 0 with base=1 should be rejected")
	}
}

// ---------------------------------------------------------------------------
// 전체 필드 왕복
// ---------------------------------------------------------------------------

func TestParseHvacr03Config_AllFields(t *testing.T) {
	opts := map[string]any{
		"transport_type":        "rtu",
		"serial_port":           "/dev/ttyS1",
		"baud_rate":             19200,
		"data_bits":             8,
		"stop_bits":             1,
		"parity":                "even",
		"slave_id":              2,
		"poll_interval":         "15s",
		"scan_interval":         "60s",
		"request_timeout":       "2s",
		"offline_timeout":       "90s",
		"reconnect_interval":    "10s",
		"max_reconnect_backoff": "10m",
		"temp_scale":            10,
		"address_base":          0,
		"fan_auto_code":         4,
		"auto_discovery":        false,
		"report_interval":       "120s",
		"event_temp_threshold":  0.5,
		"msg_channel_size":      512,
		"control_enabled":       true,
		"control_verify_delay":  "5s",
		"log_messages":          true,
		"log_state_updates":     true,
		"log_decode_errors":     true,
		"log_drops":             true,
	}
	cfg, err := parseHvacr03Config(opts)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"serial_port", cfg.SerialPort, "/dev/ttyS1"},
		{"baud_rate", cfg.BaudRate, 19200},
		{"parity", cfg.Parity, "even"},
		{"slave_id", cfg.SlaveID, byte(2)},
		{"poll_interval", cfg.PollInterval, 15 * time.Second},
		{"scan_interval", cfg.ScanInterval, 60 * time.Second},
		{"request_timeout", cfg.RequestTimeout, 2 * time.Second},
		{"offline_timeout", cfg.OfflineTimeout, 90 * time.Second},
		{"max_reconnect_backoff", cfg.MaxReconnectBackoff, 10 * time.Minute},
		{"auto_discovery", cfg.AutoDiscovery, false},
		{"report_interval", cfg.ReportInterval, 120 * time.Second},
		{"event_temp_threshold", cfg.EventTempThreshold, 0.5},
		{"msg_channel_size", cfg.MsgChannelSize, 512},
		{"control_enabled", cfg.ControlEnabled, true},
		{"control_verify_delay", cfg.ControlVerifyDelay, 5 * time.Second},
		{"log_messages", cfg.LogMessages, true},
		{"log_drops", cfg.LogDrops, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}
