package config

import "time"

// ServerConfig - 서버 설정
type ServerConfig struct {
	Port      int
	Host      string
	Mode      string // "development" 또는 "production"
	TLS       TLSConfig
	CORS      CORSConfig
	RateLimit RateLimitConfig
	WebUI     WebUIConfig
	BasicAuth BasicAuthConfig
}

// BasicAuthConfig - 기본 인증 설정
type BasicAuthConfig struct {
	Enabled         bool
	CredentialsFile string
	JWTSecret       string
	TokenExpiry     string // 예: "24h"
	RefreshExpiry   string // 예: "168h"
}

// WebUIConfig - Web UI 정적 파일 서빙 설정
type WebUIConfig struct {
	Enabled bool
	Dir     string // 정적 파일 디렉토리 (예: "./web/dist")
}

// TLSConfig - TLS 설정
type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

// CORSConfig - CORS 설정
type CORSConfig struct {
	Enabled        bool
	AllowedOrigins []string
}

// RateLimitConfig - 속도 제한 설정
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerSecond int
}

// EngineConfig - 플로우 엔진 설정
type EngineConfig struct {
	BackpressureThreshold int
	MaxConcurrentFlows    int
	ExecutionPolicy       string // "parallel" 또는 "sequential"
	WireDefaultBuffer     int
}

// StorageConfig - 스토리지 설정
type StorageConfig struct {
	Type          string // "file", "sqlite", "postgres"
	FileDirectory string // 파일 저장 디렉토리 (Type="file" 시)
	SQLitePath    string
	PostgresDSN   string
	PoolSize      int
}

// AuthConfig - 인증 설정
type AuthConfig struct {
	JWT    JWTConfig
	APIKey APIKeyConfig
	OAuth2 OAuth2Config
}

// JWTConfig - JWT 설정
type JWTConfig struct {
	Secret     string
	AccessTTL  string // 예: "15m"
	RefreshTTL string // 예: "168h"
}

// APIKeyConfig - API 키 설정
type APIKeyConfig struct {
	Enabled bool
}

// OAuth2Config - OAuth2 설정
type OAuth2Config struct {
	Providers []string
}

// ObserveConfig - 관측 설정
type ObserveConfig struct {
	DefaultLevel   string // "debug", "info", "warn", "error"
	MetricsEnabled bool
	TraceEnabled   bool
	Format         string // "json" 또는 "text"
	Output         string // "stdout", 파일 경로, 또는 "stdout+파일경로"
}

// ScriptConfig - 스크립트 설정
type ScriptConfig struct {
	Timeout    string // 예: "5s"
	VMPoolSize int
	Sandbox    SandboxConfig
}

// SandboxConfig - 샌드박스 설정
type SandboxConfig struct {
	Enabled        bool
	MaxMemoryMB    int
	MaxExecutionMS int
}

// PluginConfig - 플러그인 설정
type PluginConfig struct {
	Directory   string
	WASMEnabled bool
	GoEnabled   bool
}

// RemoteManagementConfig - 원격 관리 서버/클라이언트 설정 (@SPEC:SPEC-REMOTE-001 M1)
//
// remote_management 섹션을 표현한다(spec §5.3). Mode 로 역할(server|client|
// disabled)을 결정하며, 기본 disabled 는 기존 동작을 회귀 없이 유지한다
// (REQ-REMOTE-N03). TLS 는 기존 TLSConfig 를 재사용한다(wss, REQ-F01).
type RemoteManagementConfig struct {
	// Mode 는 "server" | "client" | "disabled" (기본 disabled, REQ-A01).
	Mode string

	// ServerURL 은 client 모드의 접속 wss URL (REQ-A02).
	ServerURL string

	// InstanceID 는 노드 식별 UUID override (빈 값이면 자동 생성·영속, REQ-A03).
	InstanceID string

	// AutoRegister 는 미등록 시 자동 등록 요청 여부 (기본 true, REQ-C01).
	AutoRegister bool

	// HeartbeatInterval 은 heartbeat 주기 (안전 기본값, REQ-A05).
	HeartbeatInterval time.Duration

	// BootstrapSecret 은 (선택) enrollment 사전 공유 시크릿 (REQ-C08).
	// 시크릿이므로 redaction·비커밋 대상 (REQ-F06).
	BootstrapSecret string

	// Exposure 는 서버에 노출할 자원 범위 (opt-in, REQ-A04).
	Exposure ExposureConfig

	// TLS 는 wss 용 TLS 설정 (기존 TLSConfig 재사용, REQ-F01).
	TLS TLSConfig

	// RequireSecure 는 보안 전송(wss/TLS)을 강제할지 결정한다 (M6, REQ-F01).
	// true 이고 non-dev(server.mode != "development")이면, client 모드의 평문 ws://
	// server_url 과 server 모드의 TLS 미설정을 거부한다. 기본 false(기존 동작 보존,
	// REQ-N03). development 모드에서는 강제하지 않는다(로컬 개발 편의).
	RequireSecure bool
}

// ExposureConfig - 노출(exposure) 범위 설정 (REQ-REMOTE-A04, spec §5.3)
//
// 각 필드는 노출 범위를 표현하는 정책 문자열이다(기본: "all" | "none" | 명시
// 목록, OPEN Q5). M1 은 문자열로만 보관하고, 실제 미러링 평가는 M4 에서 수행한다.
type ExposureConfig struct {
	Flows   string
	Agents  string
	Devices string
}
