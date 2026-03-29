package lg

import (
	"testing"
	"time"
)

// TestParseLGCPConfig_FullValid 는 모든 필드가 올바르게 파싱되는지 검증한다.
func TestParseLGCPConfig_FullValid(t *testing.T) {
	opts := map[string]any{
		"serial_port":           "/dev/ttyUSB0",
		"baud_rate":             19200,
		"data_bits":             7,
		"stop_bits":             2,
		"parity":                "even",
		"read_timeout":          "1s",
		"msg_channel_size":      512,
		"reconnect_interval":    "10s",
		"max_reconnect_backoff": "2m",
		"verify_crc":            false,
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
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
	if cfg.Parity != "even" {
		t.Errorf("Parity = %q, want %q", cfg.Parity, "even")
	}
	if cfg.ReadTimeout != 1*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 1*time.Second)
	}
	if cfg.MsgChannelSize != 512 {
		t.Errorf("MsgChannelSize = %d, want %d", cfg.MsgChannelSize, 512)
	}
	if cfg.ReconnectInterval != 10*time.Second {
		t.Errorf("ReconnectInterval = %v, want %v", cfg.ReconnectInterval, 10*time.Second)
	}
	if cfg.MaxReconnectBackoff != 2*time.Minute {
		t.Errorf("MaxReconnectBackoff = %v, want %v", cfg.MaxReconnectBackoff, 2*time.Minute)
	}
	if cfg.VerifyCRC != false {
		t.Errorf("VerifyCRC = %v, want %v", cfg.VerifyCRC, false)
	}
}

// TestParseLGCPConfig_Defaults 는 기본값이 올바르게 설정되는지 검증한다.
func TestParseLGCPConfig_Defaults(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}

	if cfg.BaudRate != 9600 {
		t.Errorf("BaudRate default = %d, want %d", cfg.BaudRate, 9600)
	}
	if cfg.DataBits != 8 {
		t.Errorf("DataBits default = %d, want %d", cfg.DataBits, 8)
	}
	if cfg.StopBits != 1 {
		t.Errorf("StopBits default = %d, want %d", cfg.StopBits, 1)
	}
	if cfg.Parity != "none" {
		t.Errorf("Parity default = %q, want %q", cfg.Parity, "none")
	}
	if cfg.ReadTimeout != 500*time.Millisecond {
		t.Errorf("ReadTimeout default = %v, want %v", cfg.ReadTimeout, 500*time.Millisecond)
	}
	if cfg.MsgChannelSize != 256 {
		t.Errorf("MsgChannelSize default = %d, want %d", cfg.MsgChannelSize, 256)
	}
	if cfg.ReconnectInterval != 5*time.Second {
		t.Errorf("ReconnectInterval default = %v, want %v", cfg.ReconnectInterval, 5*time.Second)
	}
	if cfg.MaxReconnectBackoff != 5*time.Minute {
		t.Errorf("MaxReconnectBackoff default = %v, want %v", cfg.MaxReconnectBackoff, 5*time.Minute)
	}
	if cfg.VerifyCRC != true {
		t.Errorf("VerifyCRC default = %v, want %v", cfg.VerifyCRC, true)
	}
}

// TestParseLGCPConfig_MissingSerialPort 는 필수 필드 누락 시 에러를 반환하는지 검증한다.
func TestParseLGCPConfig_MissingSerialPort(t *testing.T) {
	opts := map[string]any{
		"baud_rate": 9600,
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for missing serial_port, got nil")
	}
	if err != ErrLGCPSerialPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPSerialPortRequired)
	}
}

// TestParseLGCPConfig_EmptySerialPort 는 빈 시리얼 포트 시 에러를 반환하는지 검증한다.
func TestParseLGCPConfig_EmptySerialPort(t *testing.T) {
	opts := map[string]any{
		"serial_port": "",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for empty serial_port, got nil")
	}
	if err != ErrLGCPSerialPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPSerialPortRequired)
	}
}

// TestParseLGCPConfig_InvalidBaudRate 는 잘못된 보레이트 값에 대한 에러를 검증한다.
func TestParseLGCPConfig_InvalidBaudRate(t *testing.T) {
	tests := []struct {
		name     string
		baudRate any
	}{
		{"zero", 0},
		{"negative", -1},
		{"float_zero", float64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"serial_port": "/dev/ttyUSB0",
				"baud_rate":   tt.baudRate,
			}

			_, err := parseLGCPConfig(opts)
			if err == nil {
				t.Fatal("parseLGCPConfig() expected error for invalid baud_rate, got nil")
			}
			if err != ErrLGCPInvalidBaudRate {
				t.Errorf("error = %v, want %v", err, ErrLGCPInvalidBaudRate)
			}
		})
	}
}

// TestParseLGCPConfig_InvalidReadTimeout 는 잘못된 read_timeout 문자열 에러를 검증한다.
func TestParseLGCPConfig_InvalidReadTimeout(t *testing.T) {
	opts := map[string]any{
		"serial_port":  "/dev/ttyUSB0",
		"read_timeout": "invalid",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for invalid read_timeout, got nil")
	}
}

// TestParseLGCPConfig_InvalidReconnectInterval 는 잘못된 reconnect_interval 에러를 검증한다.
func TestParseLGCPConfig_InvalidReconnectInterval(t *testing.T) {
	opts := map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"reconnect_interval": "not-a-duration",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for invalid reconnect_interval, got nil")
	}
}

// TestParseLGCPConfig_InvalidMaxReconnectBackoff 는 잘못된 max_reconnect_backoff 에러를 검증한다.
func TestParseLGCPConfig_InvalidMaxReconnectBackoff(t *testing.T) {
	opts := map[string]any{
		"serial_port":           "/dev/ttyUSB0",
		"max_reconnect_backoff": "bad",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for invalid max_reconnect_backoff, got nil")
	}
}

// TestParseLGCPConfig_FloatBaudRate 는 float64 로 전달된 baud_rate 가 올바르게 파싱되는지 검증한다.
// JSON/YAML 파싱 시 숫자가 float64 로 전달되는 경우를 처리한다.
func TestParseLGCPConfig_FloatBaudRate(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   float64(115200),
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.BaudRate != 115200 {
		t.Errorf("BaudRate = %d, want %d", cfg.BaudRate, 115200)
	}
}

// TestParseLGCPConfig_EmptyOptions 는 빈 옵션 맵에서 에러를 반환하는지 검증한다.
func TestParseLGCPConfig_EmptyOptions(t *testing.T) {
	opts := map[string]any{}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for empty options, got nil")
	}
	if err != ErrLGCPSerialPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPSerialPortRequired)
	}
}
