package observe

import (
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
)

// logTimeFormat 은 로그 타임스탬프를 소수점 6자리(마이크로초) 고정으로 출력하는
// 포맷이다. slog 기본 RFC3339Nano 는 뒤따르는 0 을 제거해 자릿수가 들쭉날쭉하므로
// (예: .974887 vs .97492), 항상 6자리로 고정해 정렬·파싱을 일관되게 한다.
const logTimeFormat = "2006-01-02T15:04:05.000000Z07:00"

// logHandlerOpts 는 모든 slog 핸들러에 공통 적용하는 옵션이다(읽기 전용 공유).
// time 속성을 logTimeFormat(소수점 6자리 고정)으로 재포맷한다.
var logHandlerOpts = &slog.HandlerOptions{
	ReplaceAttr: replaceLogTime,
}

// replaceLogTime 은 최상위 time 속성을 소수점 6자리 고정 문자열로 바꾼다.
func replaceLogTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		a.Value = slog.StringValue(a.Value.Time().Format(logTimeFormat))
	}
	return a
}

// ComponentLogger 는 컴포넌트별 구조화된 로거 인터페이스이다.
// 모든 로그 출력에 "component" 속성이 자동으로 포함된다.
type ComponentLogger interface {
	// Debug 는 DEBUG 레벨 로그를 기록한다.
	Debug(msg string, args ...any)

	// Info 는 INFO 레벨 로그를 기록한다.
	Info(msg string, args ...any)

	// Warn 은 WARN 레벨 로그를 기록한다.
	Warn(msg string, args ...any)

	// Error 는 ERROR 레벨 로그를 기록한다.
	Error(msg string, args ...any)

	// With 는 추가 속성이 포함된 새 ComponentLogger 를 반환한다.
	// 기존 "component" 속성은 유지된다.
	With(args ...any) ComponentLogger

	// WithGroup 은 그룹이 설정된 새 ComponentLogger 를 반환한다.
	WithGroup(name string) ComponentLogger

	// Component 는 이 로거의 컴포넌트 이름을 반환한다.
	Component() string

	// Logger 는 호환성을 위해 내부 *slog.Logger 를 반환한다.
	Logger() *slog.Logger
}

// LoggerFactory 는 컴포넌트별 로거를 생성하고 관리하는 팩토리 인터페이스이다.
type LoggerFactory interface {
	// NewLogger 는 지정된 컴포넌트에 대한 ComponentLogger 를 생성하거나 반환한다.
	// 빈 문자열 컴포넌트 이름은 패닉을 발생시킨다 (프로그래밍 오류).
	// 동일한 컴포넌트 이름으로 재호출하면 같은 인스턴스를 반환한다 (멱등성).
	NewLogger(component string) ComponentLogger

	// GetLogger 는 등록된 ComponentLogger 를 조회한다.
	// 등록되지 않은 컴포넌트는 (nil, false) 를 반환한다.
	GetLogger(component string) (ComponentLogger, bool)

	// Components 는 등록된 모든 컴포넌트 이름을 정렬된 목록으로 반환한다.
	Components() []string
}

// componentLogger 는 ComponentLogger 의 구현체이다.
// *slog.Logger 를 래핑하며 컴포넌트 이름을 보존한다.
type componentLogger struct {
	logger    *slog.Logger
	component string
}

// Debug 는 DEBUG 레벨 로그를 기록한다.
func (cl *componentLogger) Debug(msg string, args ...any) {
	cl.logger.Debug(msg, args...)
}

// Info 는 INFO 레벨 로그를 기록한다.
func (cl *componentLogger) Info(msg string, args ...any) {
	cl.logger.Info(msg, args...)
}

// Warn 은 WARN 레벨 로그를 기록한다.
func (cl *componentLogger) Warn(msg string, args ...any) {
	cl.logger.Warn(msg, args...)
}

// Error 는 ERROR 레벨 로그를 기록한다.
func (cl *componentLogger) Error(msg string, args ...any) {
	cl.logger.Error(msg, args...)
}

// With 는 추가 속성이 포함된 새 ComponentLogger 를 반환한다.
// "component" 속성은 내부 slog.Logger 에 이미 포함되어 있으므로 자동 유지된다.
func (cl *componentLogger) With(args ...any) ComponentLogger {
	return &componentLogger{
		logger:    cl.logger.With(args...),
		component: cl.component,
	}
}

// WithGroup 은 그룹이 설정된 새 ComponentLogger 를 반환한다.
func (cl *componentLogger) WithGroup(name string) ComponentLogger {
	return &componentLogger{
		logger:    cl.logger.WithGroup(name),
		component: cl.component,
	}
}

// Component 는 이 로거의 컴포넌트 이름을 반환한다.
func (cl *componentLogger) Component() string {
	return cl.component
}

// Logger 는 내부 *slog.Logger 를 반환한다.
func (cl *componentLogger) Logger() *slog.Logger {
	return cl.logger
}

// loggerFactory 는 LoggerFactory 의 구현체이다.
// sync.Map 을 사용하여 동시성 안전한 컴포넌트 레지스트리를 관리한다.
type loggerFactory struct {
	// registry 는 컴포넌트 이름 -> *componentLogger 매핑을 저장한다.
	registry sync.Map

	// levelManager 는 컴포넌트별 레벨 관리자이다. (선택적)
	levelManager LevelManager

	// streamRouter 는 컴포넌트별 스트림 라우터이다. (선택적)
	streamRouter StreamRouter
}

// NewLoggerFactory 는 지정된 옵션으로 새 LoggerFactory 를 생성한다.
func NewLoggerFactory(opts ...FactoryOption) LoggerFactory {
	cfg := &factoryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &loggerFactory{
		levelManager: cfg.levelManager,
		streamRouter: cfg.streamRouter,
	}
}

// NewLogger 는 지정된 컴포넌트에 대한 ComponentLogger 를 생성하거나 반환한다.
// 빈 문자열 컴포넌트 이름은 패닉을 발생시킨다.
// 동일한 컴포넌트 이름으로 재호출하면 같은 인스턴스를 반환한다.
func (lf *loggerFactory) NewLogger(component string) ComponentLogger {
	if component == "" {
		panic("observe: 컴포넌트 이름이 비어있다")
	}

	// 이미 등록된 로거가 있으면 반환한다 (멱등성)
	if v, ok := lf.registry.Load(component); ok {
		return v.(*componentLogger)
	}

	// 핸들러를 결정한다
	var handler slog.Handler
	if lf.streamRouter != nil {
		handler = lf.streamRouter.Handler()
	} else {
		handler = slog.NewJSONHandler(os.Stdout, logHandlerOpts)
	}

	// component 는 내부 라우팅용, type/name 은 출력용 속성이다
	_, kind, cname := classifyComponent(component)
	logger := slog.New(handler).With(
		componentAttrKey, component,
		"type", kind,
		"name", cname,
	)

	// LevelManager 가 있으면 컴포넌트를 등록한다
	if lf.levelManager != nil {
		lf.levelManager.SetLevel(component, lf.levelManager.GetLevel(component))
	}

	cl := &componentLogger{
		logger:    logger,
		component: component,
	}

	// 동시성 안전: LoadOrStore 로 경합 조건을 방지한다
	actual, _ := lf.registry.LoadOrStore(component, cl)
	return actual.(*componentLogger)
}

// GetLogger 는 등록된 ComponentLogger 를 조회한다.
func (lf *loggerFactory) GetLogger(component string) (ComponentLogger, bool) {
	v, ok := lf.registry.Load(component)
	if !ok {
		return nil, false
	}
	return v.(*componentLogger), true
}

// Components 는 등록된 모든 컴포넌트 이름을 정렬된 목록으로 반환한다.
func (lf *loggerFactory) Components() []string {
	var names []string
	lf.registry.Range(func(key, value any) bool {
		names = append(names, key.(string))
		return true
	})
	sort.Strings(names)
	return names
}

// classifyComponent 는 component 문자열에서 source, kind, name 을 분리한다.
// 예: "agent.modbus.reader-01" → source="agent", kind="modbus", name="reader-01"
//
//	"flow.pipeline.node.transform" → source="node", kind="node", name="transform"
//	"api.server" → source="api", kind="api", name="server"
func classifyComponent(component string) (source, kind, name string) {
	parts := strings.Split(component, ".")

	switch {
	case strings.HasPrefix(component, "agent."):
		source = "agent"
		if len(parts) >= 3 {
			return source, parts[1], strings.Join(parts[2:], ".")
		}
		if len(parts) == 2 {
			return source, parts[1], parts[1]
		}
	case strings.HasPrefix(component, "flow.") && strings.Contains(component, ".node."):
		source = "node"
		if idx := strings.Index(component, ".node."); idx >= 0 {
			return source, "node", component[idx+6:]
		}
	case strings.HasPrefix(component, "node."):
		source = "node"
		if len(parts) >= 2 {
			return source, "node", strings.Join(parts[1:], ".")
		}
	case strings.HasPrefix(component, "flow."):
		source = "flow"
		if len(parts) >= 2 {
			return source, "flow", strings.Join(parts[1:], ".")
		}
	case strings.HasPrefix(component, "api."):
		source = "api"
		if len(parts) >= 2 {
			return source, "api", strings.Join(parts[1:], ".")
		}
	case component == "xflowd" || component == "engine" || strings.HasPrefix(component, "engine."):
		source = "engine"
		if len(parts) >= 2 {
			return source, "engine", strings.Join(parts[1:], ".")
		}
		return source, "engine", component
	default:
		source = "system"
	}

	return source, source, component
}
