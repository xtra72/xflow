package config

// ServerConfig - 서버 설정
type ServerConfig struct {
	Port      int
	Host      string
	Mode      string // "development" 또는 "production"
	TLS       TLSConfig
	CORS      CORSConfig
	RateLimit RateLimitConfig
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
	Type        string // "sqlite" 또는 "postgres"
	SQLitePath  string
	PostgresDSN string
	PoolSize    int
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
