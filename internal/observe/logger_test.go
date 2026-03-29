package observe_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/xtra/xflow/internal/observe"
)

// syncWriter 는 동시성 안전한 io.Writer 래퍼이다.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sw *syncWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.buf.Write(p)
}

// io.Writer 인터페이스 컴파일 타임 확인
var _ io.Writer = (*syncWriter)(nil)

// TestComponentLogger_Interface 는 componentLogger 가 ComponentLogger 인터페이스를 만족하는지 검증한다.
func TestComponentLogger_Interface(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))
	logger := factory.NewLogger("test.component")

	// ComponentLogger 인터페이스 컴파일 타임 확인
	var _ observe.ComponentLogger = logger
}

// TestComponentLogger_LogMethods 는 Debug, Info, Warn, Error 메서드가 올바르게 출력하는지 검증한다.
func TestComponentLogger_LogMethods(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(observe.ComponentLogger, string, ...any)
		level    string
		message  string
	}{
		{
			name:    "Debug 메서드",
			logFunc: func(l observe.ComponentLogger, msg string, args ...any) { l.Debug(msg, args...) },
			level:   "DEBUG",
			message: "디버그 메시지",
		},
		{
			name:    "Info 메서드",
			logFunc: func(l observe.ComponentLogger, msg string, args ...any) { l.Info(msg, args...) },
			level:   "INFO",
			message: "정보 메시지",
		},
		{
			name:    "Warn 메서드",
			logFunc: func(l observe.ComponentLogger, msg string, args ...any) { l.Warn(msg, args...) },
			level:   "WARN",
			message: "경고 메시지",
		},
		{
			name:    "Error 메서드",
			logFunc: func(l observe.ComponentLogger, msg string, args ...any) { l.Error(msg, args...) },
			level:   "ERROR",
			message: "에러 메시지",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// Debug 레벨까지 출력하려면 LevelManager 로 레벨을 설정한다
			lm := observe.NewLevelManager(slog.LevelDebug)
			router := observe.NewStreamRouter(
				observe.WithDefaultWriter(&buf),
				observe.WithStreamLevelManager(lm),
			)
			factory := observe.NewLoggerFactory(
				observe.WithStreamRouter(router),
				observe.WithLevelManager(lm),
			)
			logger := factory.NewLogger("test.component")

			tt.logFunc(logger, tt.message, "key", "value")

			output := buf.String()
			if output == "" {
				t.Errorf("%s: 로그 출력이 비어있다", tt.name)
				return
			}

			// JSON 파싱하여 레벨과 메시지를 확인한다
			var logEntry map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
				t.Errorf("%s: JSON 파싱 실패: %v, 출력: %s", tt.name, err, output)
				return
			}

			if msg, ok := logEntry["msg"].(string); !ok || msg != tt.message {
				t.Errorf("%s: msg = %v, 기대값 %q", tt.name, logEntry["msg"], tt.message)
			}

			if logEntry["key"] != "value" {
				t.Errorf("%s: key = %v, 기대값 \"value\"", tt.name, logEntry["key"])
			}
		})
	}
}

// TestComponentLogger_ComponentAttribute 는 모든 로그 출력에 "type", "name" 키가 포함되고
// "component" 키가 필터링되는지 검증한다.
func TestComponentLogger_ComponentAttribute(t *testing.T) {
	var buf bytes.Buffer
	lm := observe.NewLevelManager(slog.LevelDebug)
	router := observe.NewStreamRouter(
		observe.WithDefaultWriter(&buf),
		observe.WithStreamLevelManager(lm),
	)
	factory := observe.NewLoggerFactory(
		observe.WithStreamRouter(router),
		observe.WithLevelManager(lm),
	)
	logger := factory.NewLogger("agent.mqtt-client")

	// 여러 레벨로 로그를 기록한다
	logger.Debug("디버그")
	logger.Info("정보")
	logger.Warn("경고")
	logger.Error("에러")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("로그 라인 수 = %d, 기대값 4", len(lines))
	}

	for i, line := range lines {
		var logEntry map[string]any
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			t.Errorf("라인 %d JSON 파싱 실패: %v", i, err)
			continue
		}

		// component 는 내부 라우팅용이므로 JSON 출력에서 필터링된다
		if _, ok := logEntry["component"]; ok {
			t.Errorf("라인 %d: component 키가 출력에 포함되어 있다 (필터링되어야 함)", i)
		}

		// type, name 이 올바르게 출력되어야 한다
		typ, ok := logEntry["type"].(string)
		if !ok || typ != "mqtt-client" {
			t.Errorf("라인 %d: type = %v, 기대값 \"mqtt-client\"", i, logEntry["type"])
		}

		name, ok := logEntry["name"].(string)
		if !ok || name != "mqtt-client" {
			t.Errorf("라인 %d: name = %v, 기대값 \"mqtt-client\"", i, logEntry["name"])
		}
	}
}

// TestComponentLogger_Component 는 Component() 메서드가 올바른 이름을 반환하는지 검증한다.
func TestComponentLogger_Component(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))
	logger := factory.NewLogger("engine.scheduler")

	got := logger.Component()
	if got != "engine.scheduler" {
		t.Errorf("Component() = %q, 기대값 \"engine.scheduler\"", got)
	}
}

// TestComponentLogger_Logger 는 Logger() 메서드가 유효한 *slog.Logger 를 반환하는지 검증한다.
func TestComponentLogger_Logger(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))
	logger := factory.NewLogger("test.component")

	slogLogger := logger.Logger()
	if slogLogger == nil {
		t.Fatal("Logger() 가 nil 을 반환했다")
	}

	// 반환된 slog.Logger 로 로그를 기록할 수 있어야 한다
	slogLogger.Info("slog 직접 호출")

	if buf.Len() == 0 {
		t.Error("Logger() 로 반환된 slog.Logger 로 로그를 기록할 수 없다")
	}
}

// TestComponentLogger_With 는 With() 로 속성을 추가한 새 로거가
// component 를 유지하면서 추가 속성을 포함하는지 검증한다.
func TestComponentLogger_With(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))
	logger := factory.NewLogger("agent.mqtt-client")

	// 새 속성을 추가한 로거를 생성한다
	derived := logger.With("request_id", "abc-123")

	// 원본 로거와 다른 인스턴스여야 한다
	if derived == logger {
		t.Error("With() 가 같은 인스턴스를 반환했다")
	}

	// component 이름이 유지되어야 한다
	if derived.Component() != "agent.mqtt-client" {
		t.Errorf("With() 후 Component() = %q, 기대값 \"agent.mqtt\"", derived.Component())
	}

	// 로그 출력에 추가 속성이 포함되어야 한다
	derived.Info("속성 추가 테스트")

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &logEntry); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}

	// component 는 필터링되어야 한다
	if _, ok := logEntry["component"]; ok {
		t.Errorf("component 키가 출력에 포함되어 있다 (필터링되어야 함)")
	}

	// type, name 이 유지되어야 한다
	if logEntry["type"] != "mqtt-client" {
		t.Errorf("type = %v, 기대값 \"mqtt-client\"", logEntry["type"])
	}
	if logEntry["name"] != "mqtt-client" {
		t.Errorf("name = %v, 기대값 \"mqtt-client\"", logEntry["name"])
	}

	if logEntry["request_id"] != "abc-123" {
		t.Errorf("request_id = %v, 기대값 \"abc-123\"", logEntry["request_id"])
	}
}

// TestComponentLogger_WithGroup 은 WithGroup() 으로 그룹이 설정된
// 새 로거가 올바르게 동작하는지 검증한다.
func TestComponentLogger_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))
	logger := factory.NewLogger("agent.mqtt-client")

	// 그룹을 추가한 로거를 생성한다
	grouped := logger.WithGroup("request")

	// 원본 로거와 다른 인스턴스여야 한다
	if grouped == logger {
		t.Error("WithGroup() 이 같은 인스턴스를 반환했다")
	}

	// component 이름이 유지되어야 한다
	if grouped.Component() != "agent.mqtt-client" {
		t.Errorf("WithGroup() 후 Component() = %q, 기대값 \"agent.mqtt\"", grouped.Component())
	}

	// 그룹 하위에 속성이 기록되어야 한다
	grouped.Info("그룹 테스트", "method", "GET")

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &logEntry); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}

	// "request" 그룹 아래에 "method" 가 있어야 한다
	reqGroup, ok := logEntry["request"].(map[string]any)
	if !ok {
		t.Fatalf("request 그룹이 없다, 출력: %v", logEntry)
	}

	if reqGroup["method"] != "GET" {
		t.Errorf("request.method = %v, 기대값 \"GET\"", reqGroup["method"])
	}
}

// TestLoggerFactory_NewLogger 는 다양한 컴포넌트 이름으로 로거를 생성하는지 검증한다.
func TestLoggerFactory_NewLogger(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	components := []string{"engine", "agent.mqtt-client", "node.filter", "db.postgres"}

	for _, comp := range components {
		logger := factory.NewLogger(comp)
		if logger == nil {
			t.Errorf("NewLogger(%q) 가 nil 을 반환했다", comp)
			continue
		}
		if logger.Component() != comp {
			t.Errorf("NewLogger(%q).Component() = %q", comp, logger.Component())
		}
	}
}

// TestLoggerFactory_DuplicateComponent 는 같은 컴포넌트 이름으로 요청하면
// 동일한 인스턴스를 반환하는지 검증한다 (포인터 동일성).
func TestLoggerFactory_DuplicateComponent(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	first := factory.NewLogger("agent.mqtt-client")
	second := factory.NewLogger("agent.mqtt-client")

	if first != second {
		t.Error("같은 컴포넌트 이름으로 NewLogger 를 호출했는데 다른 인스턴스를 반환했다")
	}
}

// TestLoggerFactory_GetLogger 는 등록된 로거 조회와 미등록 조회를 검증한다.
func TestLoggerFactory_GetLogger(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	// 로거를 생성한다
	created := factory.NewLogger("agent.mqtt-client")

	// 등록된 로거를 조회한다
	found, ok := factory.GetLogger("agent.mqtt-client")
	if !ok {
		t.Error("등록된 컴포넌트 GetLogger 가 false 를 반환했다")
	}
	if found != created {
		t.Error("GetLogger 가 NewLogger 로 생성한 것과 다른 인스턴스를 반환했다")
	}

	// 미등록 컴포넌트를 조회한다
	notFound, ok := factory.GetLogger("nonexistent")
	if ok {
		t.Error("미등록 컴포넌트 GetLogger 가 true 를 반환했다")
	}
	if notFound != nil {
		t.Error("미등록 컴포넌트 GetLogger 가 nil 이 아닌 값을 반환했다")
	}
}

// TestLoggerFactory_Components 는 등록된 컴포넌트 이름의 정렬된 목록을 검증한다.
func TestLoggerFactory_Components(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	// 순서 무관하게 여러 로거를 생성한다
	factory.NewLogger("router.main")
	factory.NewLogger("agent.mqtt-client")
	factory.NewLogger("engine")
	factory.NewLogger("db.postgres")

	components := factory.Components()

	expected := []string{"agent.mqtt-client", "db.postgres", "engine", "router.main"}
	if len(components) != len(expected) {
		t.Fatalf("Components() 길이 = %d, 기대값 %d", len(components), len(expected))
	}

	for i, comp := range components {
		if comp != expected[i] {
			t.Errorf("Components()[%d] = %q, 기대값 %q", i, comp, expected[i])
		}
	}
}

// TestLoggerFactory_EmptyComponent 는 빈 문자열 컴포넌트 이름이 패닉을 발생시키는지 검증한다.
func TestLoggerFactory_EmptyComponent(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	defer func() {
		r := recover()
		if r == nil {
			t.Error("빈 컴포넌트 이름으로 NewLogger 호출 시 패닉이 발생하지 않았다")
		}
	}()

	factory.NewLogger("")
}

// TestLoggerFactory_HierarchicalNames 는 계층적 컴포넌트 이름이 올바르게 동작하는지 검증한다.
func TestLoggerFactory_HierarchicalNames(t *testing.T) {
	var buf bytes.Buffer
	router := observe.NewStreamRouter(observe.WithDefaultWriter(&buf))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	// component → classifyComponent 결과의 type, name 매핑
	type expected struct {
		kind string
		name string
	}
	hierarchicalCases := []struct {
		component string
		want      expected
	}{
		{"engine", expected{"engine", "engine"}},
		{"engine.scheduler", expected{"engine", "scheduler"}},
		{"agent.mqtt-client.client1", expected{"mqtt-client", "client1"}},
		{"node.filter.node-3", expected{"node", "filter.node-3"}},
	}

	for _, tc := range hierarchicalCases {
		logger := factory.NewLogger(tc.component)
		if logger == nil {
			t.Errorf("NewLogger(%q) 가 nil 을 반환했다", tc.component)
			continue
		}
		if logger.Component() != tc.component {
			t.Errorf("NewLogger(%q).Component() = %q", tc.component, logger.Component())
		}

		// 로그 출력에 type, name 이 올바르게 포함되어야 한다
		buf.Reset()
		logger.Info("계층 테스트")

		var logEntry map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &logEntry); err != nil {
			t.Errorf("컴포넌트 %q JSON 파싱 실패: %v", tc.component, err)
			continue
		}

		// component 는 필터링되어야 한다
		if _, ok := logEntry["component"]; ok {
			t.Errorf("컴포넌트 %q: component 키가 출력에 포함되어 있다", tc.component)
		}

		if logEntry["type"] != tc.want.kind {
			t.Errorf("컴포넌트 %q: type = %v, 기대값 %q", tc.component, logEntry["type"], tc.want.kind)
		}
		if logEntry["name"] != tc.want.name {
			t.Errorf("컴포넌트 %q: name = %v, 기대값 %q", tc.component, logEntry["name"], tc.want.name)
		}
	}
}

// TestLoggerFactory_WithLevelManager 는 LevelManager 연동 시
// 레벨 변경이 로그 출력에 반영되는지 검증한다.
func TestLoggerFactory_WithLevelManager(t *testing.T) {
	var buf bytes.Buffer
	lm := observe.NewLevelManager(slog.LevelInfo)
	router := observe.NewStreamRouter(
		observe.WithDefaultWriter(&buf),
		observe.WithStreamLevelManager(lm),
	)
	factory := observe.NewLoggerFactory(
		observe.WithStreamRouter(router),
		observe.WithLevelManager(lm),
	)

	logger := factory.NewLogger("agent.mqtt-client")

	// 기본 레벨(Info)에서 Debug 로그는 필터링되어야 한다
	logger.Debug("이 메시지는 보이면 안 된다")
	if buf.Len() != 0 {
		t.Errorf("Info 레벨에서 Debug 로그가 기록되었다: %s", buf.String())
	}

	// Info 로그는 출력되어야 한다
	logger.Info("정보 메시지")
	if buf.Len() == 0 {
		t.Error("Info 레벨에서 Info 로그가 기록되지 않았다")
	}
	buf.Reset()

	// 컴포넌트의 레벨을 Debug 로 변경한다
	lm.SetLevel("agent.mqtt-client", slog.LevelDebug)

	// 이제 Debug 로그도 출력되어야 한다
	logger.Debug("이제 보이는 디버그 메시지")
	if buf.Len() == 0 {
		t.Error("레벨을 Debug 로 변경했는데 Debug 로그가 기록되지 않았다")
	}
}

// TestLoggerFactory_WithStreamRouter 는 StreamRouter 연동 시
// 로그가 올바른 Writer 로 라우팅되는지 검증한다.
func TestLoggerFactory_WithStreamRouter(t *testing.T) {
	mqttBuf := &bytes.Buffer{}
	httpBuf := &bytes.Buffer{}
	defaultBuf := &bytes.Buffer{}

	router := observe.NewStreamRouter(observe.WithDefaultWriter(defaultBuf))
	router.AddRoute("agent.mqtt-client", mqttBuf)
	router.AddRoute("agent.http", httpBuf)

	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	mqttLogger := factory.NewLogger("agent.mqtt-client")
	httpLogger := factory.NewLogger("agent.http")
	otherLogger := factory.NewLogger("engine")

	mqttLogger.Info("MQTT 메시지")
	httpLogger.Info("HTTP 메시지")
	otherLogger.Info("기타 메시지")

	// MQTT 로그는 mqttBuf 에 기록되어야 한다
	if mqttBuf.Len() == 0 {
		t.Error("MQTT 로거의 로그가 MQTT Writer 에 기록되지 않았다")
	}

	// HTTP 로그는 httpBuf 에 기록되어야 한다
	if httpBuf.Len() == 0 {
		t.Error("HTTP 로거의 로그가 HTTP Writer 에 기록되지 않았다")
	}

	// 라우트 없는 컴포넌트 로그는 defaultBuf 에 기록되어야 한다
	if defaultBuf.Len() == 0 {
		t.Error("라우트 없는 로거의 로그가 기본 Writer 에 기록되지 않았다")
	}

	// 교차 오염이 없어야 한다
	if strings.Contains(httpBuf.String(), "MQTT") {
		t.Error("MQTT 로그가 HTTP Writer 에 잘못 라우팅되었다")
	}
	if strings.Contains(mqttBuf.String(), "HTTP") {
		t.Error("HTTP 로그가 MQTT Writer 에 잘못 라우팅되었다")
	}
}

// TestLoggerFactory_Concurrent 는 동시성 안전성을 검증한다.
// 여러 고루틴에서 동시에 NewLogger, GetLogger, Components 를 호출한다.
func TestLoggerFactory_Concurrent(t *testing.T) {
	sw := &syncWriter{}
	router := observe.NewStreamRouter(observe.WithDefaultWriter(sw))
	factory := observe.NewLoggerFactory(observe.WithStreamRouter(router))

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				comp := "component." + string(rune('a'+id))

				switch j % 4 {
				case 0:
					logger := factory.NewLogger(comp)
					logger.Info("동시성 테스트")
				case 1:
					factory.GetLogger(comp)
				case 2:
					factory.Components()
				case 3:
					// 같은 컴포넌트를 다시 요청한다 (멱등성)
					factory.NewLogger(comp)
				}
			}
		}(i)
	}

	wg.Wait()

	// 각 고루틴의 컴포넌트가 모두 등록되어야 한다
	components := factory.Components()
	if len(components) != goroutines {
		t.Errorf("동시성 실행 후 Components() 길이 = %d, 기대값 %d", len(components), goroutines)
	}

	// 정렬되어야 한다
	if !sort.StringsAreSorted(components) {
		t.Error("Components() 가 정렬되지 않았다")
	}
}

// TestLoggerFactory_NoRouter 는 StreamRouter 없이 팩토리를 생성했을 때
// 기본 핸들러로 정상 동작하는지 검증한다.
func TestLoggerFactory_NoRouter(t *testing.T) {
	factory := observe.NewLoggerFactory()

	logger := factory.NewLogger("test.component")
	if logger == nil {
		t.Fatal("StreamRouter 없이 생성된 로거가 nil 이다")
	}

	if logger.Component() != "test.component" {
		t.Errorf("Component() = %q, 기대값 \"test.component\"", logger.Component())
	}

	// 패닉 없이 로그를 기록할 수 있어야 한다
	logger.Info("기본 핸들러 테스트", "key", "value")
}
