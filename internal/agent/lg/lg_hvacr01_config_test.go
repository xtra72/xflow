package lg

import "testing"

// TestParseHvacr01Config_LogMessages 는 log_messages 옵션의 기본값(false)과 파싱을 검증한다
// (LG HVACR-01, Samsung/LGAP 와 통일된 송/수신 프레임 로그 옵션).
func TestParseHvacr01Config_LogMessages(t *testing.T) {
	t.Parallel()

	def, err := parseHvacr01Config(map[string]any{"serial_port": "/dev/ttyUSB0"})
	if err != nil {
		t.Fatalf("parse(default) error: %v", err)
	}
	if def.LogMessages {
		t.Errorf("LogMessages = true, want false (default)")
	}

	on, err := parseHvacr01Config(map[string]any{"serial_port": "/dev/ttyUSB0", "log_messages": true})
	if err != nil {
		t.Fatalf("parse(log_messages=true) error: %v", err)
	}
	if !on.LogMessages {
		t.Errorf("LogMessages = false, want true")
	}
}
