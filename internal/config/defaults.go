package config

import "github.com/spf13/viper"

// SetDefaults - 주어진 Viper 인스턴스에 모든 기본값을 설정
func SetDefaults(v *viper.Viper) {
	// 서버 기본값
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.mode", "development")
	v.SetDefault("server.tls.enabled", false)
	v.SetDefault("server.cors.enabled", true)
	v.SetDefault("server.cors.allowed_origins", []string{"*"})
	v.SetDefault("server.rate_limit.enabled", true)
	v.SetDefault("server.rate_limit.requests_per_second", 100)
	v.SetDefault("server.web_ui.enabled", false)
	v.SetDefault("server.web_ui.dir", "./web/dist")

	// 기본 인증 기본값
	v.SetDefault("server.basic_auth.enabled", false)
	v.SetDefault("server.basic_auth.credentials_file", "")
	v.SetDefault("server.basic_auth.jwt_secret", "")
	v.SetDefault("server.basic_auth.token_expiry", "24h")
	v.SetDefault("server.basic_auth.refresh_expiry", "168h")

	// 엔진 기본값
	v.SetDefault("engine.backpressure_threshold", 1000)
	v.SetDefault("engine.max_concurrent_flows", 100)
	v.SetDefault("engine.execution_policy", "parallel")
	v.SetDefault("engine.wire_default_buffer", 0)

	// 스토리지 기본값
	v.SetDefault("storage.type", "sqlite")
	v.SetDefault("storage.sqlite.path", "./data/xflow.db")
	v.SetDefault("storage.file.directory", "./data/flows")
	v.SetDefault("storage.pool_size", 10)

	// 인증 기본값
	v.SetDefault("auth.jwt.secret", "")
	v.SetDefault("auth.jwt.access_ttl", "15m")
	v.SetDefault("auth.jwt.refresh_ttl", "168h")
	v.SetDefault("auth.api_key.enabled", true)

	// 관측 기본값
	v.SetDefault("observe.default_level", "info")
	v.SetDefault("observe.metrics.enabled", true)
	v.SetDefault("observe.trace.enabled", false)
	v.SetDefault("observe.format", "json")
	v.SetDefault("observe.output", "stdout")

	// 스크립트 기본값
	v.SetDefault("script.timeout", "5s")
	v.SetDefault("script.vm_pool_size", 10)
	v.SetDefault("script.sandbox.enabled", true)
	v.SetDefault("script.sandbox.max_memory_mb", 64)
	v.SetDefault("script.sandbox.max_execution_ms", 5000)

	// 플러그인 기본값
	v.SetDefault("plugin.directory", "./plugins")
	v.SetDefault("plugin.wasm.enabled", true)
	v.SetDefault("plugin.go.enabled", true)

	// 자동 업데이트 기본값 (@SPEC:SPEC-UPDATE-001 v0.1.0)
	// 보안 기본값: enabled=false (운영자 명시적 opt-in)
	v.SetDefault("update.enabled", false)
	v.SetDefault("update.channel", "stable")
	v.SetDefault("update.check_interval", "24h")
	v.SetDefault("update.auto_apply", false)
	v.SetDefault("update.notify_only", false)
	v.SetDefault("update.update_url", "https://api.github.com/repos/xtra72/xflow")
	v.SetDefault("update.public_key_path", "")
	v.SetDefault("update.drain_timeout", "30s")
	v.SetDefault("update.health_check_timeout", "5s")
	v.SetDefault("update.insecure_skip_verify", false)
}
