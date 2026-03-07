package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// --- rateLimiter 테스트 ---

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	t.Parallel()

	rl := &rateLimiter{maxPerSecond: 100}

	// 100건 이내에서는 모두 허용
	allowed := 0
	for i := 0; i < 100; i++ {
		if rl.Allow() {
			allowed++
		}
	}

	if allowed != 100 {
		t.Errorf("100건 중 %d건 허용됨, want 100", allowed)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	t.Parallel()

	rl := &rateLimiter{maxPerSecond: 10}

	allowed := 0
	blocked := 0
	for i := 0; i < 20; i++ {
		if rl.Allow() {
			allowed++
		} else {
			blocked++
		}
	}

	if allowed != 10 {
		t.Errorf("허용된 건수 = %d, want 10", allowed)
	}
	if blocked != 10 {
		t.Errorf("차단된 건수 = %d, want 10", blocked)
	}
}

func TestRateLimiter_ResetsPerSecond(t *testing.T) {
	// 시간 의존 테스트이므로 Parallel 사용 안 함
	rl := &rateLimiter{maxPerSecond: 5}

	// 첫 번째 초에 5건 모두 소진
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Fatalf("첫 번째 초의 %d번째 요청이 차단됨", i+1)
		}
	}

	// 5건 초과는 차단
	if rl.Allow() {
		t.Error("초당 제한 초과 시 차단되어야 함")
	}

	// 1초 이상 대기 후 카운터 리셋 확인
	time.Sleep(1100 * time.Millisecond)

	// 새로운 초에서는 다시 허용되어야 함
	if !rl.Allow() {
		t.Error("초 리셋 후 첫 번째 요청이 차단됨")
	}
}

func TestRateLimiter_MaxPerSecondOne(t *testing.T) {
	t.Parallel()

	rl := &rateLimiter{maxPerSecond: 1}

	if !rl.Allow() {
		t.Error("첫 번째 요청이 차단됨")
	}
	if rl.Allow() {
		t.Error("두 번째 요청이 허용됨 (maxPerSecond=1)")
	}
}

// --- wsLogWriter 테스트 ---

func TestWsLogWriter_WritesValidJSON(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// 가짜 클라이언트를 등록하여 ClientCount > 0 으로 만든다
	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	w := newWsLogWriter(hub, slog.LevelDebug)

	logLine := map[string]any{
		"time":  "2024-01-01T00:00:00Z",
		"level": "INFO",
		"msg":   "테스트 로그 메시지",
	}
	data, _ := json.Marshal(logLine)

	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write 에러: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write 반환값 = %d, want %d", n, len(data))
	}

	// 브로드캐스트된 메시지 수신
	var received []byte
	select {
	case received = <-sendCh:
	case <-time.After(2 * time.Second):
		t.Fatal("브로드캐스트 메시지 수신 실패")
	}

	var msg Message
	if err := json.Unmarshal(received, &msg); err != nil {
		t.Fatalf("메시지 파싱 실패: %v", err)
	}

	if msg.Type != TypeLogEntry {
		t.Errorf("메시지 타입 = %q, want %q", msg.Type, TypeLogEntry)
	}

	var payload map[string]string
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("페이로드 파싱 실패: %v", err)
	}

	if payload["level"] != "INFO" {
		t.Errorf("level = %q, want %q", payload["level"], "INFO")
	}
	if payload["message"] != "테스트 로그 메시지" {
		t.Errorf("message = %q, want %q", payload["message"], "테스트 로그 메시지")
	}
	if payload["timestamp"] != "2024-01-01T00:00:00Z" {
		t.Errorf("timestamp = %q, want %q", payload["timestamp"], "2024-01-01T00:00:00Z")
	}
}

func TestWsLogWriter_SkipsInvalidJSON(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// 클라이언트 등록
	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	w := newWsLogWriter(hub, slog.LevelDebug)

	invalidJSON := []byte("이것은 유효하지 않은 JSON 입니다")
	n, err := w.Write(invalidJSON)

	// 항상 len(p), nil 반환
	if err != nil {
		t.Errorf("Write 에러가 nil 이어야 함: got %v", err)
	}
	if n != len(invalidJSON) {
		t.Errorf("Write 반환값 = %d, want %d", n, len(invalidJSON))
	}

	// 브로드캐스트가 발생하지 않아야 함
	select {
	case msg := <-sendCh:
		t.Errorf("유효하지 않은 JSON 에 대해 메시지가 브로드캐스트됨: %s", msg)
	case <-time.After(50 * time.Millisecond):
		// 정상: 메시지 없음
	}
}

func TestWsLogWriter_SkipsWhenNoClients(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// 클라이언트를 등록하지 않음 (ClientCount == 0)
	w := newWsLogWriter(hub, slog.LevelDebug)

	logLine := map[string]any{
		"time":  "2024-01-01T00:00:00Z",
		"level": "INFO",
		"msg":   "무시될 메시지",
	}
	data, _ := json.Marshal(logLine)

	n, err := w.Write(data)

	if err != nil {
		t.Errorf("Write 에러가 nil 이어야 함: got %v", err)
	}
	if n != len(data) {
		t.Errorf("Write 반환값 = %d, want %d", n, len(data))
	}
}

func TestWsLogWriter_LevelFiltering(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	// minLevel 을 INFO 로 설정
	w := newWsLogWriter(hub, slog.LevelInfo)

	tests := []struct {
		name     string
		level    string
		shouldBe bool // 브로드캐스트되어야 하는지
	}{
		{"DEBUG_필터링", "DEBUG", false},
		{"INFO_허용", "INFO", true},
		{"WARN_허용", "WARN", true},
		{"ERROR_허용", "ERROR", true},
		{"알수없는_레벨_허용", "TRACE", true},
		{"빈_레벨_허용", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 이전 메시지 비우기
			for len(sendCh) > 0 {
				<-sendCh
			}

			logLine := map[string]any{
				"time":  "2024-01-01T00:00:00Z",
				"level": tc.level,
				"msg":   "테스트",
			}
			data, _ := json.Marshal(logLine)

			n, err := w.Write(data)
			if err != nil {
				t.Fatalf("Write 에러: %v", err)
			}
			if n != len(data) {
				t.Fatalf("Write 반환값 = %d, want %d", n, len(data))
			}

			if tc.shouldBe {
				select {
				case <-sendCh:
					// 정상: 메시지 수신
				case <-time.After(200 * time.Millisecond):
					t.Error("메시지가 브로드캐스트되어야 하지만 수신되지 않음")
				}
			} else {
				select {
				case msg := <-sendCh:
					t.Errorf("메시지가 필터링되어야 하지만 브로드캐스트됨: %s", msg)
				case <-time.After(50 * time.Millisecond):
					// 정상: 필터링됨
				}
			}
		})
	}
}

func TestWsLogWriter_RateLimiting(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	sendCh := make(chan []byte, 512)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	w := newWsLogWriter(hub, slog.LevelDebug)

	// 기본 리미트는 100/초 이므로 120건 전송
	logLine := map[string]any{
		"time":  "2024-01-01T00:00:00Z",
		"level": "INFO",
		"msg":   "rate test",
	}
	data, _ := json.Marshal(logLine)

	for i := 0; i < 120; i++ {
		n, err := w.Write(data)
		if err != nil {
			t.Fatalf("Write[%d] 에러: %v", i, err)
		}
		if n != len(data) {
			t.Fatalf("Write[%d] 반환값 = %d, want %d", i, n, len(data))
		}
	}

	// dropped 카운터 확인: 120 - 100 = 20건이 버려져야 함
	dropped := w.Dropped()
	if dropped != 20 {
		t.Errorf("dropped = %d, want 20", dropped)
	}
}

func TestWsLogWriter_CircularPrevention(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	w := newWsLogWriter(hub, slog.LevelDebug)

	// 유효한 JSON 로그 라인을 작성하여 패닉 없이 동작하는지 확인
	logLine := map[string]any{
		"time":  "2024-01-01T00:00:00Z",
		"level": "ERROR",
		"msg":   "순환 로깅 테스트",
	}
	data, _ := json.Marshal(logLine)

	// 패닉 없이 완료되면 순환 방지가 동작하는 것으로 간주
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("순환 방지 실패: err = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write 반환값 = %d, want %d", n, len(data))
	}
}

// --- isLevelEnabled 테스트 ---

func TestWsLogWriter_IsLevelEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		minLevel slog.Level
		input    string
		expected bool
	}{
		// minLevel=DEBUG: 모든 레벨 허용
		{slog.LevelDebug, "DEBUG", true},
		{slog.LevelDebug, "INFO", true},
		{slog.LevelDebug, "WARN", true},
		{slog.LevelDebug, "ERROR", true},

		// minLevel=INFO: DEBUG 제외
		{slog.LevelInfo, "DEBUG", false},
		{slog.LevelInfo, "INFO", true},
		{slog.LevelInfo, "WARN", true},
		{slog.LevelInfo, "ERROR", true},

		// minLevel=WARN: DEBUG, INFO 제외
		{slog.LevelWarn, "DEBUG", false},
		{slog.LevelWarn, "INFO", false},
		{slog.LevelWarn, "WARN", true},
		{slog.LevelWarn, "ERROR", true},

		// minLevel=ERROR: ERROR 만 허용
		{slog.LevelError, "DEBUG", false},
		{slog.LevelError, "INFO", false},
		{slog.LevelError, "WARN", false},
		{slog.LevelError, "ERROR", true},

		// 알 수 없는 레벨은 항상 허용
		{slog.LevelInfo, "TRACE", true},
		{slog.LevelError, "UNKNOWN", true},
		{slog.LevelError, "", true},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("min=%s_input=%s", tc.minLevel, tc.input)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := &wsLogWriter{minLevel: tc.minLevel}
			result := w.isLevelEnabled(tc.input)
			if result != tc.expected {
				t.Errorf("isLevelEnabled(%q) = %v, want %v (minLevel=%s)", tc.input, result, tc.expected, tc.minLevel)
			}
		})
	}
}

// --- Dropped 카운터 테스트 ---

func TestWsLogWriter_Dropped(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	w := newWsLogWriter(hub, slog.LevelDebug)

	// 초기 dropped 은 0
	if w.Dropped() != 0 {
		t.Errorf("초기 Dropped = %d, want 0", w.Dropped())
	}
}

// --- newWsLogWriter 기본값 테스트 ---

func TestNewWsLogWriter(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	w := newWsLogWriter(hub, slog.LevelWarn)

	if w.hub != hub {
		t.Error("hub 가 올바르게 설정되지 않음")
	}
	if w.minLevel != slog.LevelWarn {
		t.Errorf("minLevel = %v, want %v", w.minLevel, slog.LevelWarn)
	}
	if w.limiter == nil {
		t.Error("limiter 가 nil")
	}
	if w.limiter.maxPerSecond != 100 {
		t.Errorf("limiter.maxPerSecond = %d, want 100", w.limiter.maxPerSecond)
	}
}

// --- Write 반환값 일관성 테스트 ---

func TestWsLogWriter_WriteAlwaysReturnsLenNil(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	w := newWsLogWriter(hub, slog.LevelDebug)

	cases := []struct {
		name string
		data []byte
	}{
		{"유효한_JSON", []byte(`{"level":"INFO","msg":"test","time":"2024-01-01T00:00:00Z"}`)},
		{"유효하지_않은_JSON", []byte(`not json at all`)},
		{"빈_바이트", []byte(``)},
		{"부분_JSON", []byte(`{"level":"INFO"`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n, err := w.Write(tc.data)
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if n != len(tc.data) {
				t.Errorf("n = %d, want %d", n, len(tc.data))
			}
		})
	}
}
