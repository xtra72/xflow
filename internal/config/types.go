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

// DeviceHistoryConfig - 디바이스 수신 데이터 이력(주기 스냅샷) 설정
//
// 수신마다의 중앙 이벤트가 없으므로 주기 스냅샷 방식으로 동작한다: Interval 마다 전체
// 디바이스의 현재 상태를 디바이스별 링버퍼(최대 MaxEntries)에 저장한다. 설정 변경
// 적용은 재시작 기준으로 충분하다(런타임 핫리로드 비대상).
type DeviceHistoryConfig struct {
	// Enabled 는 이력 레코더 활성 여부이다(기본 true).
	Enabled bool

	// Interval 은 스냅샷 수집 주기이다(기본 10s).
	Interval time.Duration

	// MaxEntries 는 디바이스별 링버퍼 최대 보관 개수이다(기본 100).
	MaxEntries int
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

	// EnrollmentToken 은 (선택) 가입 토큰이다 (v1.1 그룹 H, REQ-REMOTE-H05).
	// client 모드에서 설정되면 register 에 실어 보내, 서버가 유효성을 검증해 관리자
	// 수동 승인 없이 노드를 자동 승인하도록 한다. 시크릿이므로 redaction·비커밋·비로깅
	// 대상이다 (REQ-F06/H06).
	EnrollmentToken string

	// Exposure 는 서버에 노출할 자원 범위 (opt-in, REQ-A04).
	Exposure ExposureConfig

	// Display 는 노드 장비 모니터 해상도이다 (v1.6 M11, 그룹 M, REQ-M01).
	// xflowd 는 헤드리스 데몬이므로 런타임 자동 감지 대상이 없어, 운영자가 config
	// 로 선언한 값(resolution "WxH" 또는 width+height)을 1차 출처로 한다(A20). 파싱된
	// Width/Height 는 register/heartbeat 시스템 정보 페이로드로 운반되어 서버가
	// managed_nodes 에 저장한다(REQ-M02). 미설정/무효 → 0(미보고, REQ-M03 하위 호환).
	Display DisplayConfig

	// TLS 는 wss 용 TLS 설정 (기존 TLSConfig 재사용, REQ-F01).
	TLS TLSConfig

	// RequireSecure 는 보안 전송(wss/TLS)을 강제할지 결정한다 (M6, REQ-F01).
	// true 이고 non-dev(server.mode != "development")이면, client 모드의 평문 ws://
	// server_url 과 server 모드의 TLS 미설정을 거부한다. 기본 false(기존 동작 보존,
	// REQ-N03). development 모드에서는 강제하지 않는다(로컬 개발 편의).
	RequireSecure bool
}

// DisplayConfig - 노드 장비 모니터 해상도 설정 (v1.6 M11, 그룹 M, REQ-M01/M03, spec §5.13.1)
//
// Width/Height 는 px 정수이며, 0 은 미보고(폴백 — REQ-M03)를 의미한다. accessor
// (RemoteManagement)가 config 의 resolution("WxH") 또는 width+height 를 파싱·해석해
// 채운다: 유효한 resolution 이 width/height 보다 우선하며(REQ-M01), 무효/음수/0 은 0
// 으로 안전 처리된다(REQ-M03). 노드는 이 값을 시스템 정보 페이로드로 운반한다(REQ-M01).
type DisplayConfig struct {
	Width  int // 장비 화면 가로 px (0=미보고)
	Height int // 장비 화면 세로 px (0=미보고)
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
