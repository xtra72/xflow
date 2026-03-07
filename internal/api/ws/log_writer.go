package ws

import (
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"
)

// rateLimiter 는 외부 의존성 없이 초당 허용 이벤트 수를 제한하는 간단한 토큰 버킷이다.
type rateLimiter struct {
	maxPerSecond int64
	count        atomic.Int64
	lastReset    atomic.Int64 // unix seconds
}

// Allow 는 이벤트를 허용할지 여부를 반환한다.
// 매 초마다 카운터를 리셋하고, maxPerSecond 이하이면 허용한다.
func (r *rateLimiter) Allow() bool {
	now := time.Now().Unix()
	if r.lastReset.Load() != now {
		r.count.Store(0)
		r.lastReset.Store(now)
	}
	return r.count.Add(1) <= r.maxPerSecond
}

// wsLogWriter 는 StreamRouter 에 등록되어 로그를 수신하고
// WebSocket Hub 를 통해 log.entry 메시지로 브로드캐스트하는 io.Writer 이다.
//
// slog 파이프라인의 안정성을 위해 Write 는 항상 len(p), nil 을 반환한다.
// 순환 로깅을 방지하기 위해 내부 에러는 무시한다.
type wsLogWriter struct {
	hub      *Hub
	limiter  *rateLimiter
	minLevel slog.Level
	dropped  atomic.Int64
}

// newWsLogWriter 는 지정된 Hub 와 최소 로그 레벨로 wsLogWriter 를 생성한다.
// 초당 최대 100건의 로그 이벤트를 허용한다.
func newWsLogWriter(hub *Hub, minLevel slog.Level) *wsLogWriter {
	return &wsLogWriter{
		hub: hub,
		limiter: &rateLimiter{
			maxPerSecond: 100,
		},
		minLevel: minLevel,
	}
}

// Write 는 io.Writer 인터페이스를 구현한다.
// JSON 로그 라인을 파싱하여 level, msg, time 필드를 추출하고
// log.entry 메시지로 브로드캐스트한다.
//
// slog 파이프라인을 깨뜨리지 않기 위해 항상 len(p), nil 을 반환한다.
func (w *wsLogWriter) Write(p []byte) (int, error) {
	n := len(p)

	// 연결된 클라이언트가 없으면 불필요한 작업을 건너뛴다
	if w.hub.ClientCount() == 0 {
		return n, nil
	}

	// JSON 로그 라인을 파싱한다
	var logLine map[string]any
	if err := json.Unmarshal(p, &logLine); err != nil {
		// 파싱 실패 시 무시한다 (비-JSON 로그 라인)
		return n, nil
	}

	// 레벨 필터링: 최소 레벨 미만이면 건너뛴다
	levelStr, _ := logLine["level"].(string)
	if !w.isLevelEnabled(levelStr) {
		return n, nil
	}

	// 레이트 리미터 확인: 초과 시 dropped 카운터를 증가시키고 건너뛴다
	if !w.limiter.Allow() {
		w.dropped.Add(1)
		return n, nil
	}

	// 페이로드 구성
	msg, _ := logLine["msg"].(string)
	ts, _ := logLine["time"].(string)

	payload := map[string]string{
		"level":     levelStr,
		"message":   msg,
		"timestamp": ts,
	}

	// 브로드캐스트 (에러 무시 - 순환 로깅 방지)
	_ = w.hub.BroadcastMessage(TypeLogEntry, payload)

	return n, nil
}

// isLevelEnabled 는 문자열 레벨이 최소 레벨 이상인지 확인한다.
func (w *wsLogWriter) isLevelEnabled(levelStr string) bool {
	var level slog.Level
	switch levelStr {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		// 알 수 없는 레벨은 허용한다
		return true
	}
	return level >= w.minLevel
}

// Dropped 은 레이트 리미터에 의해 버려진 로그 항목 수를 반환한다.
func (w *wsLogWriter) Dropped() int64 {
	return w.dropped.Load()
}
