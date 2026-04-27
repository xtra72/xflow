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

// ---------------------------------------------------------------------------
// TCP 트랜스포트 설정 테스트
// ---------------------------------------------------------------------------

// TestParseLGCPConfig_TransportTypeDefaults 는 transport_type 미지정 시 기본값 "serial" 을 검증한다.
func TestParseLGCPConfig_TransportTypeDefaults(t *testing.T) {
	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.TransportType != "serial" {
		t.Errorf("TransportType default = %q, want %q", cfg.TransportType, "serial")
	}
	if cfg.TCPHost != "0.0.0.0" {
		t.Errorf("TCPHost default = %q, want %q", cfg.TCPHost, "0.0.0.0")
	}
	if cfg.TCPReadTimeout != 500*time.Millisecond {
		t.Errorf("TCPReadTimeout default = %v, want %v", cfg.TCPReadTimeout, 500*time.Millisecond)
	}
	if cfg.TCPWriteTimeout != 1*time.Second {
		t.Errorf("TCPWriteTimeout default = %v, want %v", cfg.TCPWriteTimeout, 1*time.Second)
	}
	if cfg.TCPConnectTimeout != 5*time.Second {
		t.Errorf("TCPConnectTimeout default = %v, want %v", cfg.TCPConnectTimeout, 5*time.Second)
	}
}

// TestParseLGCPConfig_TCPClientValid 는 tcp-client 모드의 올바른 설정을 검증한다.
func TestParseLGCPConfig_TCPClientValid(t *testing.T) {
	opts := map[string]any{
		"transport_type":      "tcp-client",
		"tcp_host":            "192.168.1.100",
		"tcp_port":            9600,
		"tcp_read_timeout":    "2s",
		"tcp_write_timeout":   "3s",
		"tcp_connect_timeout": "10s",
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.TransportType != "tcp-client" {
		t.Errorf("TransportType = %q, want %q", cfg.TransportType, "tcp-client")
	}
	if cfg.TCPHost != "192.168.1.100" {
		t.Errorf("TCPHost = %q, want %q", cfg.TCPHost, "192.168.1.100")
	}
	if cfg.TCPPort != 9600 {
		t.Errorf("TCPPort = %d, want %d", cfg.TCPPort, 9600)
	}
	if cfg.TCPReadTimeout != 2*time.Second {
		t.Errorf("TCPReadTimeout = %v, want %v", cfg.TCPReadTimeout, 2*time.Second)
	}
	if cfg.TCPWriteTimeout != 3*time.Second {
		t.Errorf("TCPWriteTimeout = %v, want %v", cfg.TCPWriteTimeout, 3*time.Second)
	}
	if cfg.TCPConnectTimeout != 10*time.Second {
		t.Errorf("TCPConnectTimeout = %v, want %v", cfg.TCPConnectTimeout, 10*time.Second)
	}
	// tcp-client 모드에서 serial_port 는 필수가 아니다.
	if cfg.SerialPort != "" {
		t.Errorf("SerialPort = %q, want empty for tcp-client", cfg.SerialPort)
	}
}

// TestParseLGCPConfig_TCPServerValid 는 tcp-server 모드의 올바른 설정을 검증한다.
func TestParseLGCPConfig_TCPServerValid(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
		"tcp_port":       8080,
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.TransportType != "tcp-server" {
		t.Errorf("TransportType = %q, want %q", cfg.TransportType, "tcp-server")
	}
	if cfg.TCPPort != 8080 {
		t.Errorf("TCPPort = %d, want %d", cfg.TCPPort, 8080)
	}
	// tcp-server 는 tcp_host 미지정 시 기본값 "0.0.0.0" 을 사용한다.
	if cfg.TCPHost != "0.0.0.0" {
		t.Errorf("TCPHost = %q, want %q", cfg.TCPHost, "0.0.0.0")
	}
}

// TestParseLGCPConfig_TCPServerWithCustomHost 는 tcp-server 모드에서 커스텀 호스트를 검증한다.
func TestParseLGCPConfig_TCPServerWithCustomHost(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
		"tcp_host":       "127.0.0.1",
		"tcp_port":       8080,
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.TCPHost != "127.0.0.1" {
		t.Errorf("TCPHost = %q, want %q", cfg.TCPHost, "127.0.0.1")
	}
}

// TestParseLGCPConfig_UnknownTransportType 은 알 수 없는 트랜스포트 타입에 대한 에러를 검증한다.
func TestParseLGCPConfig_UnknownTransportType(t *testing.T) {
	tests := []struct {
		name          string
		transportType string
	}{
		{"websocket", "websocket"},
		{"udp", "udp"},
		{"empty_string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"transport_type": tt.transportType,
				"serial_port":   "/dev/ttyUSB0",
			}

			_, err := parseLGCPConfig(opts)
			if err == nil {
				t.Fatal("parseLGCPConfig() expected error for unknown transport_type, got nil")
			}
			if err != ErrLGCPUnknownTransportType {
				t.Errorf("error = %v, want %v", err, ErrLGCPUnknownTransportType)
			}
		})
	}
}

// TestParseLGCPConfig_TCPClientMissingPort 는 tcp-client 모드에서 tcp_port 누락 시 에러를 검증한다.
func TestParseLGCPConfig_TCPClientMissingPort(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for missing tcp_port, got nil")
	}
	if err != ErrLGCPTCPPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPTCPPortRequired)
	}
}

// TestParseLGCPConfig_TCPServerMissingPort 는 tcp-server 모드에서 tcp_port 누락 시 에러를 검증한다.
func TestParseLGCPConfig_TCPServerMissingPort(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-server",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for missing tcp_port, got nil")
	}
	if err != ErrLGCPTCPPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPTCPPortRequired)
	}
}

// TestParseLGCPConfig_TCPClientMissingHost 는 tcp-client 모드에서 빈 tcp_host 시 에러를 검증한다.
func TestParseLGCPConfig_TCPClientMissingHost(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "",
		"tcp_port":       9600,
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for empty tcp_host in tcp-client, got nil")
	}
	if err != ErrLGCPTCPHostRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPTCPHostRequired)
	}
}

// TestParseLGCPConfig_TCPPortFloat64 는 float64 로 전달된 tcp_port 가 올바르게 파싱되는지 검증한다.
func TestParseLGCPConfig_TCPPortFloat64(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "10.0.0.1",
		"tcp_port":       float64(5000),
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.TCPPort != 5000 {
		t.Errorf("TCPPort = %d, want %d", cfg.TCPPort, 5000)
	}
}

// TestParseLGCPConfig_InvalidTCPTimeouts 는 잘못된 TCP 타임아웃 문자열 에러를 검증한다.
func TestParseLGCPConfig_InvalidTCPTimeouts(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"tcp_read_timeout", "tcp_read_timeout"},
		{"tcp_write_timeout", "tcp_write_timeout"},
		{"tcp_connect_timeout", "tcp_connect_timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"transport_type": "tcp-client",
				"tcp_host":       "10.0.0.1",
				"tcp_port":       9600,
				tt.key:           "not-a-duration",
			}

			_, err := parseLGCPConfig(opts)
			if err == nil {
				t.Fatalf("parseLGCPConfig() expected error for invalid %s, got nil", tt.key)
			}
		})
	}
}

// TestParseLGCPConfig_SerialModeNoSerialPort 는 serial 모드에서 serial_port 누락 시 에러를 검증한다.
func TestParseLGCPConfig_SerialModeNoSerialPort(t *testing.T) {
	opts := map[string]any{
		"transport_type": "serial",
	}

	_, err := parseLGCPConfig(opts)
	if err == nil {
		t.Fatal("parseLGCPConfig() expected error for missing serial_port in serial mode, got nil")
	}
	if err != ErrLGCPSerialPortRequired {
		t.Errorf("error = %v, want %v", err, ErrLGCPSerialPortRequired)
	}
}

// TestParseLGCPConfig_TCPClientNoSerialPortRequired 는 tcp-client 모드에서 serial_port 없이 성공하는지 검증한다.
func TestParseLGCPConfig_TCPClientNoSerialPortRequired(t *testing.T) {
	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "10.0.0.1",
		"tcp_port":       9600,
	}

	cfg, err := parseLGCPConfig(opts)
	if err != nil {
		t.Fatalf("parseLGCPConfig() unexpected error: %v", err)
	}
	if cfg.SerialPort != "" {
		t.Errorf("SerialPort = %q, want empty", cfg.SerialPort)
	}
}
