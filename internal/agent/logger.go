package agent

import "log/slog"

// ResolveLogger 는 AgentConfig 에서 로거를 추출한다.
// config.Logger 가 nil 이 아니면 해당 로거를 반환하고,
// nil 이면 slog.Default() 를 반환하여 하위 호환성을 유지한다.
func ResolveLogger(config AgentConfig) *slog.Logger {
	if config.Logger != nil {
		return config.Logger
	}
	return slog.Default()
}
