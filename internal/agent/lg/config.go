package lg

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// LGAPConfig 는 LG LGAP HVAC 에이전트의 설정을 나타낸다.
type LGAPConfig struct {
	SerialPort          string
	BaudRate            int
	DataBits            int
	StopBits            int
	Parity              string
	ReadTimeout         time.Duration
	PollInterval        time.Duration
	InterCommandDelay   time.Duration
	Devices             []agent.DeviceEntry
	MsgChannelSize      int
	OfflineThreshold    int
	ReconnectInterval   time.Duration
	MaxReconnectBackoff time.Duration

	// v0.6.0 통합 옵션 (Century/NASA/LG ICP-01/LGCP 와 명칭 통일):
	NotifyInterval time.Duration // report_interval 의 backing field — 주기적 상태보고 간격
	ReportMode     string        // "relative" (default) 또는 "absolute"
	IncludeRawHex  bool          // raw_hex 출력 옵션 (기본 false)

	// EventTempThreshold 는 change 트리거 event 보고의 실내온도 변화 임계값이다 (단위: ℃, v0.6.6).
	// 온도(RoomTemp)만 변경되고 |Δ| < EventTempThreshold 면 emit suppress.
	// 기본 1.0℃. 0 이하면 게이트 비활성.
	EventTempThreshold float64

	// ConnectionReportInterval 은 device_connection.report 주기 방출 간격이다
	// (SPEC-HVACR-CONNSTATE-001 §4.2). 옵션 키: connection_report_interval.
	// 기본 60s. 0 이면 주기 연결 리포트 비활성(단 initial/change 이벤트는 항상 방출).
	// report_interval(동작 상태) 과 완전히 독립적이다.
	ConnectionReportInterval time.Duration

	// StartupProbeTimeout 은 startup probe(첫 poll 라운드 await)의 bounded timeout 이다
	// (SPEC-HVACR-CONNSTATE-001 §4.2, §4.6). 옵션 키: startup_probe_timeout.
	// 명시값은 캡 없이 그대로 존중하고, 미설정이면 poll_interval 에서 파생한다:
	// min(2×poll_interval, 30s), poll_interval 이 0 이면 fallback 10s.
	StartupProbeTimeout time.Duration
}

// parseLGAPConfig 는 Transport.Options 맵에서 LGAPConfig 를 파싱한다.
func parseLGAPConfig(opts map[string]any) (LGAPConfig, error) {
	cfg := LGAPConfig{
		BaudRate:            4800,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		ReadTimeout:         500 * time.Millisecond,
		PollInterval:        30 * time.Second,
		InterCommandDelay:   50 * time.Millisecond,
		MsgChannelSize:      256,
		OfflineThreshold:    3,
		ReconnectInterval:   5 * time.Second,
		MaxReconnectBackoff: 5 * time.Minute,
		EventTempThreshold:  1.0,
	}

	// serial_port (필수)
	if v, ok := opts["serial_port"]; ok {
		cfg.SerialPort = v.(string)
	}
	if cfg.SerialPort == "" {
		return LGAPConfig{}, fmt.Errorf("lgap: serial_port is required")
	}

	// baud_rate
	if v, ok := opts["baud_rate"]; ok {
		cfg.BaudRate = toInt(v)
	}

	// data_bits
	if v, ok := opts["data_bits"]; ok {
		cfg.DataBits = toInt(v)
	}

	// stop_bits
	if v, ok := opts["stop_bits"]; ok {
		cfg.StopBits = toInt(v)
	}

	// parity
	if v, ok := opts["parity"]; ok {
		cfg.Parity = v.(string)
	}

	// read_timeout
	if v, ok := opts["read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// poll_interval
	if v, ok := opts["poll_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid poll_interval: %w", err)
		}
		cfg.PollInterval = d
	}

	// inter_command_delay
	if v, ok := opts["inter_command_delay"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid inter_command_delay: %w", err)
		}
		cfg.InterCommandDelay = d
	}

	// devices (선택)
	cfg.Devices = agent.ParseDevices(opts)

	// msg_channel_size
	if v, ok := opts["msg_channel_size"]; ok {
		cfg.MsgChannelSize = toInt(v)
	}

	// offline_threshold
	if v, ok := opts["offline_threshold"]; ok {
		cfg.OfflineThreshold = toInt(v)
	}

	// reconnect_interval
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid reconnect_interval: %w", err)
		}
		cfg.ReconnectInterval = d
	}

	// max_reconnect_backoff
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid max_reconnect_backoff: %w", err)
		}
		cfg.MaxReconnectBackoff = d
	}

	// 2026-05-29 breaking: notify_interval alias 제거. report_interval 만 허용.
	// notify_interval 키가 입력에 포함되면 명시적 에러를 반환한다 (silent ignore X).
	if _, ok := opts["notify_interval"]; ok {
		return LGAPConfig{}, fmt.Errorf("lgap: deprecated option 'notify_interval' is removed; use 'report_interval' instead")
	}
	if v, ok := opts["report_interval"]; ok {
		if s, sok := v.(string); sok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return LGAPConfig{}, fmt.Errorf("lgap: invalid report_interval: %w", err)
			}
			cfg.NotifyInterval = d
		}
	}

	// report_mode — "relative" (default) 또는 "absolute".
	if v, ok := opts["report_mode"]; ok {
		if s, sok := v.(string); sok {
			switch s {
			case "relative", "absolute", "":
				cfg.ReportMode = s
			default:
				return LGAPConfig{}, fmt.Errorf("lgap: invalid report_mode %q (must be 'relative' or 'absolute')", s)
			}
		}
	}
	if cfg.ReportMode == "" {
		cfg.ReportMode = "relative"
	}

	// include_raw_hex — raw_hex 출력 옵션 (기본 false).
	if v, ok := opts["include_raw_hex"]; ok {
		if b, isBool := v.(bool); isBool {
			cfg.IncludeRawHex = b
		}
	}

	// event_temp_threshold (v0.6.6) — 실내온도 변화 임계값 (단위 ℃, 기본 1.0).
	if v, ok := opts["event_temp_threshold"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid event_temp_threshold: %w", err)
		}
		cfg.EventTempThreshold = f
	}

	// connection_report_interval (SPEC-HVACR-CONNSTATE-001 §4.2) — device_connection.report
	// 주기 간격. 기본 60s. report_interval 과 독립. 잘못된 legacy alias 는 hard error
	// (notify_interval → report_interval 선례를 따른다, N4/AC-18).
	cfg.ConnectionReportInterval = 60 * time.Second
	if _, ok := opts["connection_notify_interval"]; ok {
		return LGAPConfig{}, fmt.Errorf("lgap: deprecated option 'connection_notify_interval' is not supported; use 'connection_report_interval' instead")
	}
	if v, ok := opts["connection_report_interval"]; ok {
		s, sok := v.(string)
		if !sok {
			return LGAPConfig{}, fmt.Errorf("lgap: connection_report_interval must be a duration string")
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid connection_report_interval: %w", err)
		}
		cfg.ConnectionReportInterval = d
	}

	// startup_probe_timeout (SPEC-HVACR-CONNSTATE-001 §4.2, OQ-A) — startup probe 의
	// bounded timeout. 명시값은 캡 없이 존중, 미설정이면 poll_interval 에서 파생한다.
	if v, ok := opts["startup_probe_timeout"]; ok {
		s, sok := v.(string)
		if !sok {
			return LGAPConfig{}, fmt.Errorf("lgap: startup_probe_timeout must be a duration string")
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return LGAPConfig{}, fmt.Errorf("lgap: invalid startup_probe_timeout: %w", err)
		}
		// 명시 override — 30s 캡 미적용(운영자 의도 존중).
		cfg.StartupProbeTimeout = d
	} else {
		cfg.StartupProbeTimeout = deriveStartupProbeTimeout(cfg.PollInterval)
	}

	return cfg, nil
}

// deriveStartupProbeTimeout 은 startup_probe_timeout 미설정 시 파생 기본값을 계산한다
// (SPEC-HVACR-CONNSTATE-001 §4.2, OQ-A). poll_interval 이 0/미설정이면 절대 fallback
// 10s 를, 아니면 min(2×poll_interval, 30s) 를 반환한다.
func deriveStartupProbeTimeout(pollInterval time.Duration) time.Duration {
	if pollInterval <= 0 {
		return 10 * time.Second
	}
	d := 2 * pollInterval
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// toFloat64 는 수치 후보를 float64 로 변환한다 (v0.6.6, lg 패키지 공통 헬퍼).
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

// parseZoneKey 는 존 키 문자열을 정수값으로 변환한다.
// "0x10" (16진수) 또는 "16" (10진수) 형식을 모두 지원한다.
func parseZoneKey(key string) any {
	// 16진수 접두사 처리
	if len(key) > 2 && (key[:2] == "0x" || key[:2] == "0X") {
		var val int
		_, err := fmt.Sscanf(key, "0x%x", &val)
		if err != nil {
			_, _ = fmt.Sscanf(key, "0X%x", &val)
		}
		return val
	}
	// 10진수 처리
	var val int
	_, _ = fmt.Sscanf(key, "%d", &val)
	return val
}

// toInt 는 int 또는 float64 값을 int 로 변환한다.
// YAML/JSON 파싱에서 숫자가 float64 로 전달될 수 있으므로 두 타입 모두 처리한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
