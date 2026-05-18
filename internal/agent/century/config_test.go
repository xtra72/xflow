package century

import (
	"errors"
	"testing"
	"time"
)

func TestParseCenturyConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg, err := parseCenturyConfig(map[string]any{
		"serial_port": "/dev/ttyUSB0",
	})
	if err != nil {
		t.Fatalf("parseCenturyConfig returned error: %v", err)
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
}

func TestParseCenturyConfig_MissingSerialPort(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{})
	if !errors.Is(err, ErrSerialPortRequired) {
		t.Fatalf("err = %v, want ErrSerialPortRequired", err)
	}
}

func TestParseCenturyConfig_UnknownTransportType(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"transport_type": "tcp-client",
		"serial_port":    "/dev/ttyUSB0",
	})
	if !errors.Is(err, ErrUnknownTransportType) {
		t.Fatalf("err = %v, want ErrUnknownTransportType", err)
	}
}

func TestParseCenturyConfig_HexAddresses(t *testing.T) {
	t.Parallel()
	// AC-D3: hex string input is accepted.
	cfg, err := parseCenturyConfig(map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"master_address": "0x0030",
		"slave_address":  "0x0001",
		"sub_dev_id":     "0x3B",
	})
	if err != nil {
		t.Fatalf("parseCenturyConfig returned error: %v", err)
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

func TestParseCenturyConfig_IntegerAddresses(t *testing.T) {
	t.Parallel()
	// AC-D3: integer input is also accepted.
	cfg, err := parseCenturyConfig(map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"master_address": 48,
		"slave_address":  1,
		"sub_dev_id":     59,
	})
	if err != nil {
		t.Fatalf("parseCenturyConfig returned error: %v", err)
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

func TestParseCenturyConfig_InvalidBaudRate(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   0,
	})
	if !errors.Is(err, ErrInvalidBaudRate) {
		t.Fatalf("err = %v, want ErrInvalidBaudRate", err)
	}
}

func TestParseCenturyConfig_InvalidRingBufferSize(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"serial_port":      "/dev/ttyUSB0",
		"ring_buffer_size": 8,
	})
	if !errors.Is(err, ErrInvalidRingBufferSize) {
		t.Fatalf("err = %v, want ErrInvalidRingBufferSize", err)
	}
}

func TestParseCenturyConfig_InvalidOfflineTimeout(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"serial_port":     "/dev/ttyUSB0",
		"offline_timeout": "0s",
	})
	if !errors.Is(err, ErrInvalidOfflineTimeout) {
		t.Fatalf("err = %v, want ErrInvalidOfflineTimeout", err)
	}
}

func TestParseCenturyConfig_InvalidCycleIdleTimeout(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"serial_port":        "/dev/ttyUSB0",
		"cycle_idle_timeout": "0s",
	})
	if !errors.Is(err, ErrInvalidCycleIdleTimeout) {
		t.Fatalf("err = %v, want ErrInvalidCycleIdleTimeout", err)
	}
}

func TestParseCenturyConfig_DurationsAndBooleans(t *testing.T) {
	t.Parallel()
	cfg, err := parseCenturyConfig(map[string]any{
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
		t.Fatalf("parseCenturyConfig returned error: %v", err)
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

func TestParseCenturyConfig_InvalidDurationString(t *testing.T) {
	t.Parallel()
	_, err := parseCenturyConfig(map[string]any{
		"serial_port":     "/dev/ttyUSB0",
		"offline_timeout": "not-a-duration",
	})
	if err == nil {
		t.Fatalf("err = nil, want a parse error")
	}
}

func TestParseCenturyConfig_DevicesPassthrough(t *testing.T) {
	t.Parallel()
	cfg, err := parseCenturyConfig(map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"devices": []any{
			map[string]any{"address": "0x3B", "name": "living-room"},
			map[string]any{"address": "0x3C", "name": "bedroom"},
		},
	})
	if err != nil {
		t.Fatalf("parseCenturyConfig returned error: %v", err)
	}
	if got := len(cfg.Devices); got != 2 {
		t.Fatalf("len(Devices) = %d, want 2", got)
	}
	if cfg.Devices[0].Address != "0x3B" {
		t.Errorf("Devices[0].Address = %q, want 0x3B", cfg.Devices[0].Address)
	}
}
