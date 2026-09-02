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
	// 스케줄(예약) 실행 로그 저장소 백엔드(시작 설정, 재시작 시 적용).
	// "sqlite"(기본, 영속) | "memory"(비영속) | "file"/"jsonl"(append-only JSONL).
	v.SetDefault("storage.schedule_log.type", "sqlite")

	// 디바이스 수신 데이터 이력(주기 스냅샷) 기본값.
	// 주기 스냅샷 방식: interval 마다 전체 디바이스의 현재 상태를 디바이스별
	// 링버퍼(최대 max_entries)에 저장한다. 기본 활성, 10초 주기, 100개 보관.
	v.SetDefault("device_history.enabled", true)
	v.SetDefault("device_history.interval", "10s")
	v.SetDefault("device_history.max_entries", 100)

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
	v.SetDefault("observe.id_style", "both")

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

	// 원격 관리 기본값 (@SPEC:SPEC-REMOTE-001 M1)
	// 보안/회귀 기본값: mode=disabled (기존 동작 불변 — REQ-N03).
	v.SetDefault("remote_management.mode", "disabled")
	v.SetDefault("remote_management.server_url", "")
	v.SetDefault("remote_management.instance_id", "")
	v.SetDefault("remote_management.auto_register", true)
	v.SetDefault("remote_management.heartbeat_interval", "30s")
	v.SetDefault("remote_management.bootstrap_secret", "")
	// enrollment_token 기본 빈 값 — 설정 시 client register 가 가입 토큰을 운반한다(v1.1 H).
	v.SetDefault("remote_management.enrollment_token", "")
	v.SetDefault("remote_management.exposure.flows", "none")
	v.SetDefault("remote_management.exposure.agents", "none")
	v.SetDefault("remote_management.exposure.devices", "none")
	// 노드 장비 모니터 해상도 (v1.6 M11, 그룹 M, REQ-M01/M03). xflowd 는 헤드리스
	// 데몬이라 런타임 감지 대상이 없어 운영자가 선언한다(A20). resolution("WIDTHxHEIGHT",
	// 예: "1920x1080")이 유효하면 width/height 보다 우선하고, 미설정/무효 시 width+height
	// 로 폴백한다. 기본 빈값/0 → 미보고(폴백 — 관리자 뷰가 기본 해상도/컨테이너 크기 사용).
	v.SetDefault("remote_management.display.resolution", "")
	v.SetDefault("remote_management.display.width", 0)
	v.SetDefault("remote_management.display.height", 0)
	v.SetDefault("remote_management.tls.enabled", false)
	v.SetDefault("remote_management.tls.cert_file", "")
	v.SetDefault("remote_management.tls.key_file", "")
	// require_secure 기본 false — 기존 동작 보존(REQ-N03). M6 보안 전송 강제 옵션.
	v.SetDefault("remote_management.require_secure", false)

	// insecure_skip_verify 기본 false — client 모드 wss 핸드셰이크의 TLS 인증서 검증을
	// 건너뛸지(자체 서명 인증서/사설망 전용 옵트인). 기본 보안(검증 수행) 유지.
	v.SetDefault("remote_management.insecure_skip_verify", false)
}
