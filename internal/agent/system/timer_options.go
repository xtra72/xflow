package system

import (
	"time"

	"github.com/robfig/cron/v3"
)

// TimerOption 은 TimerAgent 생성 시 적용할 수 있는 옵션 함수 타입이다.
type TimerOption func(*timerConfig)

// defaultTimerConfig 는 기본 타이머 설정 값을 반환한다.
func defaultTimerConfig() timerConfig {
	return timerConfig{
		minInterval: 100 * time.Millisecond,
		maxTimers:   1000,
		cronParser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// WithMinInterval 은 최소 인터벌을 설정하는 옵션을 반환한다.
func WithMinInterval(d time.Duration) TimerOption {
	return func(c *timerConfig) {
		c.minInterval = d
	}
}

// WithMaxTimers 는 최대 타이머 수를 설정하는 옵션을 반환한다.
func WithMaxTimers(n int) TimerOption {
	return func(c *timerConfig) {
		c.maxTimers = n
	}
}

// WithCronParser 는 cron 파서를 설정하는 옵션을 반환한다.
func WithCronParser(parser cron.Parser) TimerOption {
	return func(c *timerConfig) {
		c.cronParser = parser
	}
}
