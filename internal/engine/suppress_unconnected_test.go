package engine

import (
	"testing"
)

// TestWarnUnconnectedPort_Suppressed 는 suppressUnconnected 플래그가 설정된
// portCounter 에 대해 미연결 경고가 로깅되지 않는지 검증한다.
func TestWarnUnconnectedPort_Suppressed(t *testing.T) {
	h := &countingHandler{}
	e := &Engine{logger: newTestLogger(h)}

	pc := &portCounter{suppressUnconnected: true}

	// 여러 번 호출해도 경고가 한 번도 로깅되지 않아야 한다.
	for i := 0; i < 5; i++ {
		e.warnUnconnectedPort("n1", "노드1", "out", pc)
	}

	if got := h.warnCount(); got != 0 {
		t.Errorf("suppress 설정된 포트 경고 횟수 = %d, want 0", got)
	}
}

// TestWarnUnconnectedPort_NotSuppressed_WarnsOnce 는 suppress 플래그가 없는
// portCounter 는 기존과 동일하게 포트당 1회 경고하는지 검증한다 (회귀 방지).
func TestWarnUnconnectedPort_NotSuppressed_WarnsOnce(t *testing.T) {
	h := &countingHandler{}
	e := &Engine{logger: newTestLogger(h)}

	pc := &portCounter{} // suppressUnconnected 기본 false

	for i := 0; i < 5; i++ {
		e.warnUnconnectedPort("n1", "노드1", "out", pc)
	}

	if got := h.warnCount(); got != 1 {
		t.Errorf("suppress 미설정 포트 경고 횟수 = %d, want 1", got)
	}
}

// TestWarnUnconnectedPort_SuppressedDoesNotConsumeGuard 는 suppress 가 once-guard
// CAS 를 소비하지 않음을 검증한다. (suppress 호출 후 flag 를 끄면 다시 1회 경고 가능)
func TestWarnUnconnectedPort_SuppressedDoesNotConsumeGuard(t *testing.T) {
	h := &countingHandler{}
	e := &Engine{logger: newTestLogger(h)}

	pc := &portCounter{suppressUnconnected: true}
	// suppress 상태에서 여러 번 호출
	for i := 0; i < 3; i++ {
		e.warnUnconnectedPort("n1", "노드1", "out", pc)
	}
	if got := h.warnCount(); got != 0 {
		t.Fatalf("suppress 중 경고 횟수 = %d, want 0", got)
	}

	// suppress 해제 후에는 once-guard 가 미소비 상태이므로 1회 경고되어야 한다.
	pc.suppressUnconnected = false
	e.warnUnconnectedPort("n1", "노드1", "out", pc)
	if got := h.warnCount(); got != 1 {
		t.Errorf("suppress 해제 후 경고 횟수 = %d, want 1", got)
	}
}

// TestParseSuppressUnconnected 는 config 값의 관대한(tolerant) bool 파싱을 검증한다.
func TestParseSuppressUnconnected(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]any
		want bool
	}{
		{name: "bool true", cfg: map[string]any{"suppress_unconnected_warning": true}, want: true},
		{name: "bool false", cfg: map[string]any{"suppress_unconnected_warning": false}, want: false},
		{name: "string true", cfg: map[string]any{"suppress_unconnected_warning": "true"}, want: true},
		{name: "string false", cfg: map[string]any{"suppress_unconnected_warning": "false"}, want: false},
		{name: "string True (mixed case)", cfg: map[string]any{"suppress_unconnected_warning": "True"}, want: true},
		{name: "string 1", cfg: map[string]any{"suppress_unconnected_warning": "1"}, want: true},
		{name: "absent (default false)", cfg: map[string]any{}, want: false},
		{name: "nil config (default false)", cfg: nil, want: false},
		{name: "invalid string (default false)", cfg: map[string]any{"suppress_unconnected_warning": "yep"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSuppressUnconnected(tt.cfg); got != tt.want {
				t.Errorf("parseSuppressUnconnected(%v) = %v, want %v", tt.cfg, got, tt.want)
			}
		})
	}
}
