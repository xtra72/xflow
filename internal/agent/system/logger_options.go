package system

import (
	"io"
	"log/slog"

	"github.com/xtra/xflow/internal/observe"
)

// LoggerOption 은 LoggerAgent 생성 시 적용할 수 있는 옵션 함수 타입이다.
type LoggerOption func(*loggerConfig)

// loggerConfig 는 LoggerAgent의 내부 설정을 담는 구조체이다.
type loggerConfig struct {
	defaultLevel  slog.Level       // 기본 로그 레벨 (기본: slog.LevelInfo)
	format        string           // 출력 포맷 ("json" 또는 "text")
	defaultWriter io.Writer        // 기본 출력 Writer (nil이면 Observer 기본값 사용)
	observer      *observe.Observer // 외부 주입 Observer (nil이면 Init에서 생성)
}

// defaultLoggerConfig 는 기본 설정 값을 반환한다.
func defaultLoggerConfig() loggerConfig {
	return loggerConfig{
		defaultLevel: slog.LevelInfo,
		format:       "json",
	}
}

// WithLogDefaultLevel 은 기본 로그 레벨을 설정하는 옵션을 반환한다.
func WithLogDefaultLevel(level slog.Level) LoggerOption {
	return func(c *loggerConfig) {
		c.defaultLevel = level
	}
}

// WithLogFormat 은 로그 출력 포맷을 설정하는 옵션을 반환한다.
// "json" 또는 "text" 를 지정할 수 있다.
func WithLogFormat(format string) LoggerOption {
	return func(c *loggerConfig) {
		c.format = format
	}
}

// WithLogWriter 는 기본 출력 Writer를 설정하는 옵션을 반환한다.
func WithLogWriter(writer io.Writer) LoggerOption {
	return func(c *loggerConfig) {
		c.defaultWriter = writer
	}
}

// WithLogObserver 는 외부에서 생성된 Observer를 주입하는 옵션을 반환한다.
// 이 옵션이 설정되면 Init에서 새로운 Observer를 생성하지 않고 주입된 것을 사용한다.
func WithLogObserver(observer *observe.Observer) LoggerOption {
	return func(c *loggerConfig) {
		c.observer = observer
	}
}
