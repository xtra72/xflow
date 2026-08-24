package system

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xtra/xflow/internal/observe"
)

func TestLoggerOptions_DefaultConfig(t *testing.T) {
	cfg := defaultLoggerConfig()

	assert.Equal(t, slog.LevelInfo, cfg.defaultLevel)
	assert.Equal(t, "json", cfg.format)
	assert.Nil(t, cfg.defaultWriter)
	assert.Nil(t, cfg.observer)
}

func TestLoggerOptions_WithLogDefaultLevel(t *testing.T) {
	cfg := defaultLoggerConfig()
	WithLogDefaultLevel(slog.LevelDebug)(&cfg)

	assert.Equal(t, slog.LevelDebug, cfg.defaultLevel)
}

func TestLoggerOptions_WithLogFormat(t *testing.T) {
	cfg := defaultLoggerConfig()
	WithLogFormat("text")(&cfg)

	assert.Equal(t, "text", cfg.format)
}

func TestLoggerOptions_WithLogWriter(t *testing.T) {
	cfg := defaultLoggerConfig()
	buf := &bytes.Buffer{}
	WithLogWriter(buf)(&cfg)

	assert.Equal(t, buf, cfg.defaultWriter)
}

func TestLoggerOptions_WithLogObserver(t *testing.T) {
	cfg := defaultLoggerConfig()
	obs := observe.New()
	WithLogObserver(obs)(&cfg)

	assert.Same(t, obs, cfg.observer)
}

func TestLoggerOptions_MultipleOptions(t *testing.T) {
	cfg := defaultLoggerConfig()

	buf := &bytes.Buffer{}
	opts := []LoggerOption{
		WithLogDefaultLevel(slog.LevelWarn),
		WithLogFormat("text"),
		WithLogWriter(buf),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	assert.Equal(t, slog.LevelWarn, cfg.defaultLevel)
	assert.Equal(t, "text", cfg.format)
	assert.Same(t, buf, cfg.defaultWriter)
}

func TestLoggerOptions_ObserverOverridesOtherOptions(t *testing.T) {
	// WithLogObserver가 설정되면 Init에서 해당 Observer를 사용한다
	obs := observe.New(
		observe.WithObserverDefaultLevel(slog.LevelError),
	)
	cfg := defaultLoggerConfig()
	WithLogDefaultLevel(slog.LevelDebug)(&cfg)
	WithLogObserver(obs)(&cfg)

	assert.Equal(t, slog.LevelDebug, cfg.defaultLevel) // 설정은 유지됨
	assert.Same(t, obs, cfg.observer)                  // 하지만 Observer가 주입됨
}
