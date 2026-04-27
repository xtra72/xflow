package observe_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xtra/xflow/internal/observe"
)

// TestObserver_New 는 New() 가 nil 이 아닌 Observer 를 반환하고
// 모든 필드가 초기화되었는지 검증한다.
func TestObserver_New(t *testing.T) {
	obs := observe.New()

	if obs == nil {
		t.Fatal("New() 가 nil 을 반환했다")
	}

	if obs.Loggers == nil {
		t.Error("Observer.Loggers 가 nil 이다")
	}
	if obs.Levels == nil {
		t.Error("Observer.Levels 가 nil 이다")
	}
	if obs.Metrics == nil {
		t.Error("Observer.Metrics 가 nil 이다")
	}
	if obs.Tracer == nil {
		t.Error("Observer.Tracer 가 nil 이다")
	}
	if obs.Streams == nil {
		t.Error("Observer.Streams 가 nil 이다")
	}
}

// TestObserver_LoggersNotNil 은 Observer.Loggers 가 정상적으로 동작하는지 검증한다.
func TestObserver_LoggersNotNil(t *testing.T) {
	var buf bytes.Buffer
	obs := observe.New(
		observe.WithObserverWriter(&buf),
	)

	logger := obs.Loggers.NewLogger("test.component")
	if logger == nil {
		t.Fatal("Loggers.NewLogger() 가 nil 을 반환했다")
	}

	if logger.Component() != "test.component" {
		t.Errorf("Component() = %q, 기대값 \"test.component\"", logger.Component())
	}

	logger.Info("로거 테스트")
	if buf.Len() == 0 {
		t.Error("Observer.Loggers 로 생성한 로거가 로그를 기록하지 않았다")
	}
}

// TestObserver_LevelsNotNil 은 Observer.Levels 가 정상적으로 동작하는지 검증한다.
func TestObserver_LevelsNotNil(t *testing.T) {
	obs := observe.New()

	// 기본 레벨을 조회할 수 있어야 한다
	defaultLevel := obs.Levels.DefaultLevel()
	if defaultLevel != slog.LevelInfo {
		t.Errorf("DefaultLevel() = %v, 기대값 %v", defaultLevel, slog.LevelInfo)
	}

	// 컴포넌트 레벨을 설정/조회할 수 있어야 한다
	obs.Levels.SetLevel("test.comp", slog.LevelDebug)
	got := obs.Levels.GetLevel("test.comp")
	if got != slog.LevelDebug {
		t.Errorf("GetLevel() = %v, 기대값 %v", got, slog.LevelDebug)
	}
}

// TestObserver_MetricsNotNil 은 Observer.Metrics 가 정상적으로 동작하는지 검증한다.
func TestObserver_MetricsNotNil(t *testing.T) {
	obs := observe.New()

	counter := obs.Metrics.Counter("test_counter", "test.comp")
	if counter == nil {
		t.Fatal("Metrics.Counter() 가 nil 을 반환했다")
	}

	// 패닉 없이 Inc 를 호출할 수 있어야 한다
	counter.Inc()

	// Registry 가 nil 이 아니어야 한다
	if obs.Metrics.Registry() == nil {
		t.Error("Metrics.Registry() 가 nil 이다")
	}
}

// TestObserver_TracerNotNil 은 Observer.Tracer 가 nil 이 아니고
// 기본적으로 비활성화되어 있는지 검증한다.
func TestObserver_TracerNotNil(t *testing.T) {
	obs := observe.New()

	if obs.Tracer.Enabled() {
		t.Error("기본 Tracer 가 활성화되어 있다, 기대값: 비활성화")
	}

	// 비활성화 상태에서도 StartSpan 이 패닉 없이 동작해야 한다
	span := obs.Tracer.StartSpan("trace-1", "comp", "op")
	if span == nil {
		t.Fatal("비활성화 Tracer.StartSpan() 이 nil 을 반환했다")
	}
}

// TestObserver_StreamsNotNil 은 Observer.Streams 가 정상적으로 동작하는지 검증한다.
func TestObserver_StreamsNotNil(t *testing.T) {
	var buf bytes.Buffer
	obs := observe.New(
		observe.WithObserverWriter(&buf),
	)

	// Handler 가 nil 이 아니어야 한다
	handler := obs.Streams.Handler()
	if handler == nil {
		t.Error("Streams.Handler() 가 nil 이다")
	}

	// AddRoute 가 패닉 없이 동작해야 한다
	var routeBuf bytes.Buffer
	obs.Streams.AddRoute("routed.comp", &routeBuf)

	routes := obs.Streams.Routes("routed.comp")
	if len(routes) != 1 {
		t.Errorf("Routes() 길이 = %d, 기대값 1", len(routes))
	}
}

// TestObserver_WithOptions 는 옵션이 올바르게 적용되는지 검증한다.
func TestObserver_WithOptions(t *testing.T) {
	var buf bytes.Buffer
	reg := prometheus.NewRegistry()

	obs := observe.New(
		observe.WithObserverWriter(&buf),
		observe.WithObserverFormat("text"),
		observe.WithObserverDefaultLevel(slog.LevelDebug),
		observe.WithObserverRegistry(reg),
		observe.WithObserverNamespace("myapp"),
		observe.WithObserverTracerEnabled(true),
		observe.WithObserverSamplingRate(0.5),
	)

	// 레벨이 Debug 로 설정되어야 한다
	if obs.Levels.DefaultLevel() != slog.LevelDebug {
		t.Errorf("DefaultLevel() = %v, 기대값 %v", obs.Levels.DefaultLevel(), slog.LevelDebug)
	}

	// Tracer 가 활성화되어야 한다
	if !obs.Tracer.Enabled() {
		t.Error("WithObserverTracerEnabled(true) 설정 후 Tracer 가 비활성화되어 있다")
	}

	// 샘플링 비율이 0.5 이어야 한다
	if obs.Tracer.SamplingRate() != 0.5 {
		t.Errorf("SamplingRate() = %f, 기대값 0.5", obs.Tracer.SamplingRate())
	}

	// Registry 가 전달한 레지스트리와 동일해야 한다
	if obs.Metrics.Registry() != reg {
		t.Error("Metrics.Registry() 가 전달된 레지스트리와 다르다")
	}

	// Debug 로그가 출력되어야 한다
	logger := obs.Loggers.NewLogger("test.debug")
	logger.Debug("디버그 메시지")
	if buf.Len() == 0 {
		t.Error("Debug 레벨 설정 후 Debug 로그가 기록되지 않았다")
	}

	// text 포맷이어야 한다 (JSON 이 아닌)
	output := buf.String()
	if strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Error("포맷이 text 로 설정되었는데 JSON 형태로 출력되었다")
	}
}

// TestObserver_Integration 은 모든 서브시스템이 통합적으로 동작하는지 검증한다.
func TestObserver_Integration(t *testing.T) {
	var buf bytes.Buffer
	obs := observe.New(
		observe.WithObserverWriter(&buf),
		observe.WithObserverFormat("json"),
	)

	// 1. 로거를 생성한다
	logger := obs.Loggers.NewLogger("agent.mqtt-client")

	// 2. Info 메시지를 기록한다
	logger.Info("connected", "broker", "localhost")

	// 3. 출력에 type, name 이 포함되어야 한다 (component 는 필터링)
	output := buf.String()
	if output == "" {
		t.Fatal("로그 출력이 비어있다")
	}

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Fatalf("JSON 파싱 실패: %v, 출력: %s", err, output)
	}

	// component 는 내부 라우팅용이므로 JSON 출력에서 필터링된다
	if _, ok := logEntry["component"]; ok {
		t.Errorf("component 키가 출력에 포함되어 있다 (필터링되어야 함)")
	}
	if logEntry["type"] != "mqtt-client" {
		t.Errorf("type = %v, 기대값 \"mqtt-client\"", logEntry["type"])
	}
	if logEntry["name"] != "mqtt-client" {
		t.Errorf("name = %v, 기대값 \"mqtt-client\"", logEntry["name"])
	}
	if logEntry["broker"] != "localhost" {
		t.Errorf("broker = %v, 기대값 \"localhost\"", logEntry["broker"])
	}

	// 4. 레벨을 DEBUG 로 변경한다
	obs.Levels.SetLevel("agent.mqtt-client", slog.LevelDebug)

	buf.Reset()
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("레벨 변경 후 Debug 로그가 출력되지 않았다")
	}

	// 5. Metrics 를 사용한다
	obs.Metrics.Counter("messages_total", "agent.mqtt-client").Inc()

	// 6. Tracer 를 사용한다
	obs.Tracer.SetEnabled(true)
	span := obs.Tracer.StartSpan("trace-1", "agent.mqtt-client", "connect")
	span.SetAttribute("broker", "localhost")
	span.End()

	if span.Duration() <= 0 {
		t.Error("span Duration() 이 0 이하이다, End() 후에는 양수여야 한다")
	}
}

// TestObserver_LevelManagerStreamRouterIntegration 은 LevelManager 레벨 변경이
// StreamRouter 를 통해 로그 출력에 반영되는지 검증한다.
func TestObserver_LevelManagerStreamRouterIntegration(t *testing.T) {
	var buf bytes.Buffer
	obs := observe.New(
		observe.WithObserverWriter(&buf),
		observe.WithObserverFormat("json"),
		observe.WithObserverDefaultLevel(slog.LevelWarn),
	)

	logger := obs.Loggers.NewLogger("agent.mqtt-client")

	// 기본 레벨이 Warn 이므로 Info 로그는 필터링되어야 한다
	logger.Info("이 메시지는 보이면 안 된다")
	if buf.Len() != 0 {
		t.Errorf("Warn 레벨에서 Info 로그가 기록되었다: %s", buf.String())
	}

	// Warn 로그는 출력되어야 한다
	logger.Warn("경고 메시지")
	if buf.Len() == 0 {
		t.Error("Warn 레벨에서 Warn 로그가 기록되지 않았다")
	}
	buf.Reset()

	// 컴포넌트 레벨을 Info 로 변경한다
	obs.Levels.SetLevel("agent.mqtt-client", slog.LevelInfo)

	// 이제 Info 로그도 출력되어야 한다
	logger.Info("이제 보이는 Info 메시지")
	if buf.Len() == 0 {
		t.Error("레벨을 Info 로 변경했는데 Info 로그가 기록되지 않았다")
	}
}

// TestObserver_DefaultValues 는 기본값이 올바르게 설정되는지 검증한다.
func TestObserver_DefaultValues(t *testing.T) {
	obs := observe.New()

	// 기본 레벨은 Info 이어야 한다
	if obs.Levels.DefaultLevel() != slog.LevelInfo {
		t.Errorf("기본 DefaultLevel() = %v, 기대값 %v", obs.Levels.DefaultLevel(), slog.LevelInfo)
	}

	// Tracer 는 기본적으로 비활성화되어야 한다
	if obs.Tracer.Enabled() {
		t.Error("기본 Tracer 가 활성화되어 있다")
	}

	// 기본 샘플링 비율은 1.0 이어야 한다
	if obs.Tracer.SamplingRate() != 1.0 {
		t.Errorf("기본 SamplingRate() = %f, 기대값 1.0", obs.Tracer.SamplingRate())
	}

	// Metrics Registry 가 nil 이 아니어야 한다
	if obs.Metrics.Registry() == nil {
		t.Error("기본 Metrics.Registry() 가 nil 이다")
	}
}
