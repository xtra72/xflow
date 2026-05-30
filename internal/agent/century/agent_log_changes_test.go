package century

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// TestDiffPayloadBytePositions 는 byte diff 위치 계산 함수의 케이스를 검증한다.
func TestDiffPayloadBytePositions(t *testing.T) {
	tests := []struct {
		name string
		prev []byte
		cur  []byte
		want string
	}{
		{
			name: "single data byte differs",
			prev: []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x05},
			cur:  []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x11},
			want: "data[2]",
		},
		{
			name: "multiple data bytes differ",
			prev: []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00, 0xFA},
			cur:  []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x11, 0x00, 0xC0, 0x00, 0x00, 0xFA},
			want: "data[2],data[4]",
		},
		{
			name: "prefix byte differs",
			prev: []byte{0x3B, 0x00, 0x02, 0x00},
			cur:  []byte{0x3C, 0x00, 0x02, 0x00},
			want: "p[0]",
		},
		{
			name: "register byte (p[2]) differs",
			prev: []byte{0x3B, 0x00, 0x02, 0x00},
			cur:  []byte{0x3B, 0x00, 0x03, 0x00},
			want: "p[2]",
		},
		{
			name: "length differs (cur longer)",
			prev: []byte{0x3B, 0x00, 0x02},
			cur:  []byte{0x3B, 0x00, 0x02, 0x99},
			want: "len(3→4)",
		},
		{
			name: "length differs (prev longer)",
			prev: []byte{0x3B, 0x00, 0x02, 0xAA},
			cur:  []byte{0x3B, 0x00, 0x02},
			want: "len(4→3)",
		},
		{
			name: "data + length",
			prev: []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x05},
			cur:  []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x11, 0x99},
			want: "data[2],len(6→7)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := diffPayloadBytePositions(tc.prev, tc.cur)
			if got != tc.want {
				t.Errorf("diffPayloadBytePositions = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLogStateChangesOnly_DedupsAndAnnotates 는 LogStateChangesOnly=true 환경에서
// (1) 동일 payload 반복은 로그가 1회만 출력되고
// (2) 다른 payload 가 오면 changed_bytes 필드가 함께 노출되는지 검증한다.
func TestLogStateChangesOnly_DedupsAndAnnotates(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.LogStateUpdates = true
	cfg.LogStateChangesOnly = true

	a := newHvacr01AgentForTest(
		agent.AgentConfig{
			ID:     "test",
			Name:   "test",
			Logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		},
		cfg,
		nil,
	)

	// 동일 raw payload 의 Reg02 두 번 → 두 번째는 dedup 되어야 함.
	reg02Payload1 := []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x05, 0x00, 0x01, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0x00, 0x00}
	frame1 := &Frame{Payload: reg02Payload1}
	decoded1 := &Reg02Decoded{SubDevID: 0x3B}

	a.logDecodedState(decoded1, frame1, 0x3B)
	if !strings.Contains(buf.String(), "(initial)") {
		t.Fatalf("expected first log to be marked (initial), got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "state update (reg02)") {
		t.Fatalf("expected reg02 state update log, got: %s", buf.String())
	}

	buf.Reset()
	a.logDecodedState(decoded1, frame1, 0x3B) // 동일 payload 재호출
	if buf.String() != "" {
		t.Errorf("expected no log for identical payload, got: %s", buf.String())
	}

	// data[2] 와 data[7] 이 바뀐 새 payload → 한 번 로그, changed_bytes=data[2],data[7]
	reg02Payload2 := []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x11, 0x00, 0x01, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0x00, 0x00}
	// Wait — data[2]=payload[5], 첫 byte that differs is payload[5]=0x05→0x11.
	// So changed_bytes = data[2] (payload offset 5 = data[2]).
	// data[7]=payload[10] doesn't differ here. Let me fix the fixture.
	reg02Payload2[10] = 0x0F // data[7] 변경 (FA → 0F)
	frame2 := &Frame{Payload: reg02Payload2}

	buf.Reset()
	a.logDecodedState(decoded1, frame2, 0x3B)
	out := buf.String()
	if !strings.Contains(out, "changed_bytes=") {
		t.Errorf("expected changed_bytes field in log, got: %s", out)
	}
	if !strings.Contains(out, "data[2]") {
		t.Errorf("expected data[2] in changed_bytes, got: %s", out)
	}
	if !strings.Contains(out, "data[7]") {
		t.Errorf("expected data[7] in changed_bytes, got: %s", out)
	}
}

// TestLogStateChangesOnly_SeparatesReadAndWrite 는 같은 register 0x04 라도
// read response (Reg04ReadDecoded) 와 write request (Reg04WriteDecoded) 가
// 별도 cache key 로 dedup 되는지 검증한다.
func TestLogStateChangesOnly_SeparatesReadAndWrite(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.LogStateUpdates = true
	cfg.LogStateChangesOnly = true

	a := newHvacr01AgentForTest(
		agent.AgentConfig{
			ID:     "test",
			Name:   "test",
			Logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		},
		cfg,
		nil,
	)

	readPayload := []byte{0x3B, 0x00, 0x04, 0x4F, 0xF6, 0x09, 0x00, 0x00, 0x00, 0x00, 0x2E, 0x00, 0x00, 0x1E, 0x00, 0x1E, 0x00}
	writePayload := []byte{0x3B, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC4, 0x00}

	readDecoded := &Reg04ReadDecoded{SubDevID: 0x3B, Register: 0x04}
	writeDecoded := &Reg04WriteDecoded{SubDevID: 0x3B, Register: 0x04}

	a.logDecodedState(readDecoded, &Frame{Payload: readPayload}, 0x3B)
	a.logDecodedState(writeDecoded, &Frame{Payload: writePayload}, 0x3B)

	out := buf.String()
	// 두 메시지 모두 (initial) 로 표기되어야 한다 (각각 별개 cache 키).
	if strings.Count(out, "(initial)") != 2 {
		t.Errorf("expected 2 (initial) markers (read + write), got: %s", out)
	}
	if !strings.Contains(out, "state update (reg04 read)") {
		t.Errorf("expected reg04 read log, got: %s", out)
	}
	if !strings.Contains(out, "state update (reg04 write observed)") {
		t.Errorf("expected reg04 write log, got: %s", out)
	}

	// 동일 payload 재전송 → dedup.
	buf.Reset()
	a.logDecodedState(readDecoded, &Frame{Payload: readPayload}, 0x3B)
	a.logDecodedState(writeDecoded, &Frame{Payload: writePayload}, 0x3B)
	if buf.String() != "" {
		t.Errorf("expected dedup of both read and write, got: %s", buf.String())
	}
}

// TestLogStateChangesOnly_DisabledIsBackwardCompat 는 옵션이 false 일 때
// 기존 v0.4.x 동작 (매 frame 마다 로그) 이 유지됨을 검증한다.
func TestLogStateChangesOnly_DisabledIsBackwardCompat(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.LogStateUpdates = true
	cfg.LogStateChangesOnly = false // 분석 모드 OFF

	a := newHvacr01AgentForTest(
		agent.AgentConfig{
			ID:     "test",
			Name:   "test",
			Logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		},
		cfg,
		nil,
	)

	payload := []byte{0x3B, 0x00, 0x02, 0x00, 0x01, 0x05, 0x00, 0x01, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0xFA, 0x00, 0x00, 0x00, 0x00, 0x00}
	decoded := &Reg02Decoded{SubDevID: 0x3B}

	a.logDecodedState(decoded, &Frame{Payload: payload}, 0x3B)
	a.logDecodedState(decoded, &Frame{Payload: payload}, 0x3B)
	a.logDecodedState(decoded, &Frame{Payload: payload}, 0x3B)

	out := buf.String()
	if strings.Count(out, "state update (reg02)") != 3 {
		t.Errorf("expected 3 reg02 logs (no dedup), got %d in: %s",
			strings.Count(out, "state update (reg02)"), out)
	}
	if strings.Contains(out, "changed_bytes") {
		t.Errorf("expected no changed_bytes field when option disabled, got: %s", out)
	}
}

// defaultTestConfig 는 본 테스트용 최소 config 를 반환한다.
func defaultTestConfig() Hvacr01Config {
	return Hvacr01Config{
		RingBufferSize:  16,
		EmitDeviceState: false, // device_state emit path 비활성 (logDecodedState 만 테스트)
	}
}
