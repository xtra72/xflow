package observe_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/xtra/xflow/internal/observe"
)

// errorWriter 는 항상 에러를 반환하는 테스트용 Writer 이다.
type errorWriter struct{}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("의도적 쓰기 에러")
}

// TestStreamRouter_AddRemoveRoute 는 라우트 추가와 제거를 검증한다.
func TestStreamRouter_AddRemoveRoute(t *testing.T) {
	sr := observe.NewStreamRouter()
	buf := &bytes.Buffer{}

	sr.AddRoute("agent.mqtt", buf)
	routes := sr.Routes("agent.mqtt")
	if len(routes) != 1 {
		t.Errorf("AddRoute 후 Routes 길이 = %d, 기대값 1", len(routes))
	}

	sr.RemoveRoute("agent.mqtt", buf)
	routes = sr.Routes("agent.mqtt")
	if len(routes) != 0 {
		t.Errorf("RemoveRoute 후 Routes 길이 = %d, 기대값 0", len(routes))
	}
}

// TestStreamRouter_Routes 는 Writer 목록 조회를 검증한다.
func TestStreamRouter_Routes(t *testing.T) {
	sr := observe.NewStreamRouter()
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	sr.AddRoute("agent.mqtt", buf1)
	sr.AddRoute("agent.mqtt", buf2)

	routes := sr.Routes("agent.mqtt")
	if len(routes) != 2 {
		t.Errorf("Routes 길이 = %d, 기대값 2", len(routes))
	}

	// 등록되지 않은 컴포넌트의 라우트는 비어있어야 한다
	routes = sr.Routes("unknown.component")
	if len(routes) != 0 {
		t.Errorf("미등록 컴포넌트 Routes 길이 = %d, 기대값 0", len(routes))
	}
}

// TestStreamRouter_DefaultWriter 는 기본 Writer 폴백을 검증한다.
func TestStreamRouter_DefaultWriter(t *testing.T) {
	defaultBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))

	handler := sr.Handler()
	logger := slog.New(handler)

	// 라우트가 없는 컴포넌트의 로그는 기본 Writer 로 전달되어야 한다
	logger.Info("테스트 메시지", "component", "unrouted.component")

	if defaultBuf.Len() == 0 {
		t.Error("기본 Writer 에 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_FanOut 은 동일 컴포넌트에 여러 Writer 가 있을 때
// 모든 Writer 에 로그가 전달되는지 검증한다.
func TestStreamRouter_FanOut(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	defaultBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))

	sr.AddRoute("agent.mqtt", buf1)
	sr.AddRoute("agent.mqtt", buf2)

	handler := sr.Handler()
	logger := slog.New(handler)

	logger.Info("팬아웃 메시지", "component", "agent.mqtt")

	if buf1.Len() == 0 {
		t.Error("첫 번째 Writer 에 로그가 기록되지 않았다")
	}
	if buf2.Len() == 0 {
		t.Error("두 번째 Writer 에 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_WriterErrorTolerance 는 하나의 Writer 가 실패해도
// 나머지 Writer 에 정상 기록되는지 검증한다.
func TestStreamRouter_WriterErrorTolerance(t *testing.T) {
	goodBuf := &bytes.Buffer{}
	badWriter := &errorWriter{}
	defaultBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))

	sr.AddRoute("agent.mqtt", badWriter)
	sr.AddRoute("agent.mqtt", goodBuf)

	handler := sr.Handler()
	logger := slog.New(handler)

	logger.Info("에러 허용 테스트", "component", "agent.mqtt")

	if goodBuf.Len() == 0 {
		t.Error("실패한 Writer 가 있어도 정상 Writer 에 로그가 기록되어야 한다")
	}
}

// TestStreamRouter_Handler 는 유효한 slog.Handler 를 반환하는지 검증한다.
func TestStreamRouter_Handler(t *testing.T) {
	sr := observe.NewStreamRouter()
	handler := sr.Handler()

	if handler == nil {
		t.Fatal("Handler() 가 nil 을 반환했다")
	}

	// slog.Handler 인터페이스 구현 확인
	var _ slog.Handler = handler
}

// TestStreamRouter_HandlerRouting 은 컴포넌트에 따라 올바른 Writer 로
// 로그가 라우팅되는지 검증한다.
func TestStreamRouter_HandlerRouting(t *testing.T) {
	mqttBuf := &bytes.Buffer{}
	httpBuf := &bytes.Buffer{}
	defaultBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))

	sr.AddRoute("agent.mqtt", mqttBuf)
	sr.AddRoute("agent.http", httpBuf)

	handler := sr.Handler()
	logger := slog.New(handler)

	logger.Info("MQTT 메시지", "component", "agent.mqtt")
	logger.Info("HTTP 메시지", "component", "agent.http")

	if mqttBuf.Len() == 0 {
		t.Error("MQTT Writer 에 로그가 기록되지 않았다")
	}
	if httpBuf.Len() == 0 {
		t.Error("HTTP Writer 에 로그가 기록되지 않았다")
	}

	// MQTT 로그가 HTTP Writer 에 기록되면 안 된다
	if strings.Contains(httpBuf.String(), "MQTT 메시지") {
		t.Error("MQTT 로그가 HTTP Writer 에 잘못 라우팅되었다")
	}
	// HTTP 로그가 MQTT Writer 에 기록되면 안 된다
	if strings.Contains(mqttBuf.String(), "HTTP 메시지") {
		t.Error("HTTP 로그가 MQTT Writer 에 잘못 라우팅되었다")
	}
}

// TestStreamRouter_HandlerDefaultFallback 은 라우트가 없는 컴포넌트의 로그가
// 기본 Writer 로 전달되는지 검증한다.
func TestStreamRouter_HandlerDefaultFallback(t *testing.T) {
	mqttBuf := &bytes.Buffer{}
	defaultBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))

	sr.AddRoute("agent.mqtt", mqttBuf)

	handler := sr.Handler()
	logger := slog.New(handler)

	// 라우트가 없는 컴포넌트
	logger.Info("미등록 컴포넌트 메시지", "component", "router.main")

	if defaultBuf.Len() == 0 {
		t.Error("미등록 컴포넌트의 로그가 기본 Writer 에 기록되지 않았다")
	}
	if mqttBuf.Len() != 0 {
		t.Error("미등록 컴포넌트의 로그가 MQTT Writer 에 잘못 기록되었다")
	}
}

// TestStreamRouter_SetDefaultWriter 는 기본 Writer 교체를 검증한다.
func TestStreamRouter_SetDefaultWriter(t *testing.T) {
	oldBuf := &bytes.Buffer{}
	newBuf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(oldBuf))

	sr.SetDefaultWriter(newBuf)

	handler := sr.Handler()
	logger := slog.New(handler)

	logger.Info("교체 후 메시지", "component", "unrouted")

	if newBuf.Len() == 0 {
		t.Error("교체된 기본 Writer 에 로그가 기록되지 않았다")
	}
	if oldBuf.Len() != 0 {
		t.Error("교체 전 기본 Writer 에 로그가 기록되었다")
	}
}

// TestStreamRouter_HandlerWithAttrs 는 WithAttrs 로 속성을 추가한 핸들러가
// 올바르게 동작하는지 검증한다.
func TestStreamRouter_HandlerWithAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(buf))

	handler := sr.Handler()

	// component 속성을 미리 설정한 핸들러 생성
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("component", "agent.mqtt"),
		slog.String("version", "1.0"),
	})

	if handlerWithAttrs == nil {
		t.Fatal("WithAttrs 가 nil 을 반환했다")
	}

	logger := slog.New(handlerWithAttrs)
	logger.Info("속성 추가 테스트")

	if buf.Len() == 0 {
		t.Error("WithAttrs 핸들러로 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_HandlerWithGroup 은 WithGroup 으로 그룹을 추가한 핸들러가
// 올바르게 동작하는지 검증한다.
func TestStreamRouter_HandlerWithGroup(t *testing.T) {
	buf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(observe.WithDefaultWriter(buf))

	handler := sr.Handler()

	handlerWithGroup := handler.WithGroup("request")
	if handlerWithGroup == nil {
		t.Fatal("WithGroup 이 nil 을 반환했다")
	}

	logger := slog.New(handlerWithGroup)
	logger.Info("그룹 테스트", "method", "GET")

	if buf.Len() == 0 {
		t.Error("WithGroup 핸들러로 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_JSONFormat 은 JSON 포맷 출력을 검증한다.
func TestStreamRouter_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(
		observe.WithDefaultWriter(buf),
		observe.WithFormat("json"),
	)

	handler := sr.Handler()
	logger := slog.New(handler)
	logger.Info("JSON 포맷 테스트", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "{") {
		t.Errorf("JSON 출력이 아니다: %s", output)
	}
}

// TestStreamRouter_TextFormat 은 텍스트 포맷 출력을 검증한다.
func TestStreamRouter_TextFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	sr := observe.NewStreamRouter(
		observe.WithDefaultWriter(buf),
		observe.WithFormat("text"),
	)

	handler := sr.Handler()
	logger := slog.New(handler)
	logger.Info("텍스트 포맷 테스트", "key", "value")

	output := buf.String()
	// 텍스트 포맷에서는 key=value 형식으로 출력된다
	if !strings.Contains(output, "key=") {
		t.Errorf("텍스트 출력이 아니다: %s", output)
	}
}

// TestStreamRouter_LevelFiltering 은 LevelManager 연동 시
// 레벨 필터링이 동작하는지 검증한다.
func TestStreamRouter_LevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	lm := observe.NewLevelManager(slog.LevelWarn)
	sr := observe.NewStreamRouter(
		observe.WithDefaultWriter(buf),
		observe.WithStreamLevelManager(lm),
	)

	handler := sr.Handler()
	logger := slog.New(handler)

	// Info 레벨 로그는 필터링되어야 한다 (기본 레벨: Warn)
	logger.Info("이 메시지는 보이면 안 된다")

	if buf.Len() != 0 {
		t.Errorf("Warn 레벨 필터에서 Info 로그가 기록되었다: %s", buf.String())
	}

	// Warn 레벨 로그는 통과해야 한다
	logger.Warn("이 메시지는 보여야 한다")

	if buf.Len() == 0 {
		t.Error("Warn 레벨 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_ComponentLevelFiltering 은 컴포넌트별 레벨 설정이
// routingHandler.Enabled() 를 통해 올바르게 필터링되는지 검증한다.
// 버그 재현: 플로우 log_level=info 설정인데 DEBUG 메시지가 출력되는 문제.
func TestStreamRouter_ComponentLevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}

	// 데몬 기본 레벨을 DEBUG 로 설정 (실제 운영 시나리오)
	lm := observe.NewLevelManager(slog.LevelDebug)
	sr := observe.NewStreamRouter(
		observe.WithDefaultWriter(buf),
		observe.WithStreamLevelManager(lm),
	)

	// 컴포넌트가 포함된 로거 생성 (실제 NewLogger 경로와 동일)
	handler := sr.Handler()
	logger := slog.New(handler).With("component", "node.test-node")

	// 컴포넌트를 LevelManager 에 등록 (NewLogger 가 하는 작업)
	lm.SetLevel("node.test-node", lm.GetLevel("node.test-node"))

	// 기본 레벨이 DEBUG 이므로 DEBUG 가 활성화되어야 한다
	if !logger.Enabled(nil, slog.LevelDebug) {
		t.Error("기본 DEBUG 레벨에서 DEBUG 가 비활성화되었다")
	}

	// 플로우의 log_level=info 적용 시뮬레이션
	lm.SetLevel("node.test-node", slog.LevelInfo)

	// 이제 DEBUG 가 비활성화되어야 한다
	if logger.Enabled(nil, slog.LevelDebug) {
		t.Error("INFO 레벨 설정 후에도 DEBUG 가 여전히 활성화되어 있다 (버그!)")
	}

	// INFO 는 활성화되어야 한다
	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("INFO 레벨이 비활성화되었다")
	}

	// DEBUG 로그 시도 - 출력되면 안 된다
	buf.Reset()
	logger.Debug("이 메시지는 보이면 안 된다")
	if buf.Len() != 0 {
		t.Errorf("INFO 레벨에서 DEBUG 로그가 기록되었다: %s", buf.String())
	}

	// INFO 로그 시도 - 출력되어야 한다
	logger.Info("이 메시지는 보여야 한다")
	if buf.Len() == 0 {
		t.Error("INFO 로그가 기록되지 않았다")
	}
}

// TestStreamRouter_ComponentLevelFiltering_WithObserver 는 Observer 를 통한
// 전체 통합 경로에서 컴포넌트별 레벨 필터링이 동작하는지 검증한다.
func TestStreamRouter_ComponentLevelFiltering_WithObserver(t *testing.T) {
	// 데몬 기본 레벨 = DEBUG
	obs := observe.New(observe.WithObserverDefaultLevel(slog.LevelDebug))

	// 컴포넌트 로거 생성 (실제 engine 이 하는 작업)
	nodeLogger := obs.Loggers.NewLogger("node.test-node")

	// 기본 레벨이 DEBUG 이므로 DEBUG 가 활성화되어야 한다
	if !nodeLogger.Logger().Enabled(nil, slog.LevelDebug) {
		t.Error("기본 DEBUG 레벨에서 DEBUG 가 비활성화되었다")
	}

	// 플로우의 log_level=info 적용 (resolveNodeLogLevel 결과)
	obs.Levels.SetLevel("node.test-node", slog.LevelInfo)

	// 이제 DEBUG 가 비활성화되어야 한다
	if nodeLogger.Logger().Enabled(nil, slog.LevelDebug) {
		t.Error("INFO 레벨 설정 후에도 DEBUG 가 여전히 활성화되어 있다 (버그!)")
	}

	// INFO 는 활성화되어야 한다
	if !nodeLogger.Logger().Enabled(nil, slog.LevelInfo) {
		t.Error("INFO 레벨이 비활성화되었다")
	}
}

// Compile-time check: errorWriter 가 io.Writer 를 구현하는지 확인
var _ io.Writer = (*errorWriter)(nil)
