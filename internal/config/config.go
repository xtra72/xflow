package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// --- 타입 정의 ---

// ChangeCallback - 설정 변경 시 호출되는 콜백 함수 타입
type ChangeCallback func(event ChangeEvent)

// UnsubscribeFunc - 콜백 구독 해제 함수 타입
type UnsubscribeFunc func()

// ChangeEvent - 설정 변경 이벤트 정보
type ChangeEvent struct {
	Key       string    // 변경된 키
	OldValue  any       // 이전 값
	NewValue  any       // 새로운 값
	Source    string    // 변경 소스: "file", "api", "env"
	Timestamp time.Time // 변경 시각
}

// Configurable - 플러그인/컴포넌트가 설정을 수신하는 인터페이스
type Configurable interface {
	Configure(cfg map[string]any) error
	GetConfig() map[string]any
}

// --- Config 인터페이스 ---

// Config - 설정 접근 인터페이스
type Config interface {
	// 카테고리별 설정 접근
	Server() ServerConfig
	Engine() EngineConfig
	Storage() StorageConfig
	DeviceHistory() DeviceHistoryConfig // 디바이스 수신 데이터 이력(주기 스냅샷)
	Auth() AuthConfig
	Observe() ObserveConfig
	Script() ScriptConfig
	Plugin() PluginConfig
	Update() UpdateSettings                   // @SPEC:SPEC-UPDATE-001 v0.1.0
	RemoteManagement() RemoteManagementConfig // @SPEC:SPEC-REMOTE-001 M1

	// 범용 접근
	Get(key string) any
	IsSet(key string) bool

	// 런타임 변경
	Set(key string, value any) error
	OnChange(key string, fn ChangeCallback) UnsubscribeFunc
	ChangeHistory() []ChangeEvent

	// 파일 감시
	WatchConfig() error
	StopWatch()
}

// --- viperConfig 구현 ---

// 컴파일 타임 인터페이스 충족 검증
var _ Config = (*viperConfig)(nil)

// callbackEntry - 콜백 등록 정보
type callbackEntry struct {
	id int
	fn ChangeCallback
}

// viperConfig - Config 인터페이스의 viper 기반 구현 (비공개)
type viperConfig struct {
	mu          sync.RWMutex
	v           *viper.Viper
	logger      *slog.Logger
	callbacks   map[string][]callbackEntry
	callbacksMu sync.RWMutex
	history     []ChangeEvent
	historyMu   sync.RWMutex
	maxHistory  int
	nextID      atomic.Int64
	watching    bool
	watchCtx    context.Context
	watchCancel context.CancelFunc
	lastChange  time.Time  // 디바운스를 위한 마지막 변경 시각
	debounceMu  sync.Mutex // 디바운스 시간 보호
}

// --- LoadOption 타입 및 옵션 함수 ---

// loadConfig - Load 함수의 설정 옵션 집합
type loadConfig struct {
	configFile  string             // 사용자 지정 설정 파일 경로
	configName  string             // 확장자 없는 설정 파일명 (예: "xflowd")
	configPaths []string           // 기본 설정 파일 검색 경로
	envPrefix   string             // 환경 변수 접두사 (기본: "XFLOW")
	flags       *pflag.FlagSet     // CLI 플래그 셋
	defaultsFn  func(*viper.Viper) // 커스텀 기본값 함수
	logger      *slog.Logger       // 선택적 로거
}

// LoadOption - Load 함수에 전달하는 옵션 함수 타입
type LoadOption func(*loadConfig)

// WithConfigFile - 사용자 지정 설정 파일 경로 설정
func WithConfigFile(path string) LoadOption {
	return func(lc *loadConfig) {
		lc.configFile = path
	}
}

// WithConfigName - 설정 파일명 설정 (확장자 없이)
func WithConfigName(name string) LoadOption {
	return func(lc *loadConfig) {
		lc.configName = name
	}
}

// WithConfigPaths - 설정 파일 검색 경로 설정
func WithConfigPaths(paths ...string) LoadOption {
	return func(lc *loadConfig) {
		lc.configPaths = paths
	}
}

// WithEnvPrefix - 환경 변수 접두사 설정
func WithEnvPrefix(prefix string) LoadOption {
	return func(lc *loadConfig) {
		lc.envPrefix = prefix
	}
}

// WithFlags - CLI 플래그 셋 바인딩
func WithFlags(flags *pflag.FlagSet) LoadOption {
	return func(lc *loadConfig) {
		lc.flags = flags
	}
}

// WithDefaults - 커스텀 기본값 설정 함수
func WithDefaults(fn func(*viper.Viper)) LoadOption {
	return func(lc *loadConfig) {
		lc.defaultsFn = fn
	}
}

// WithLogger - 선택적 로거 설정
func WithLogger(logger *slog.Logger) LoadOption {
	return func(lc *loadConfig) {
		lc.logger = logger
	}
}

// --- Load 함수 ---

// Load - 설정을 로드하고 Config 인터페이스를 반환
func Load(opts ...LoadOption) (Config, error) {
	// 1. 옵션 적용
	lc := &loadConfig{
		configName:  "xflow",
		configPaths: []string{".", "$HOME/.xflow", "/etc/xflow"},
		envPrefix:   "XFLOW",
	}
	for _, opt := range opts {
		opt(lc)
	}

	// 2. Viper 인스턴스 생성
	v := viper.New()

	// 3. 기본값 설정
	SetDefaults(v)
	if lc.defaultsFn != nil {
		lc.defaultsFn(v)
	}

	// 4. 기본 경로 설정 파일 검색 경로 구성
	v.SetConfigName(lc.configName)
	// NOTE: SetConfigType 를 호출하지 않는다.
	// 호출 시 Viper 가 확장자 없는 동명 파일(예: xflow 바이너리)도 매칭하여
	// 바이너리를 YAML 로 파싱하려는 문제가 발생한다.
	// 미호출 시 Viper 가 확장자(.yaml, .yml, .json 등) 기반으로만 검색한다.
	for _, path := range lc.configPaths {
		expanded := os.ExpandEnv(path)
		v.AddConfigPath(expanded)
	}

	// 5. 기본 경로 설정 파일 읽기 (발견 시 기본값 오버라이드)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("config: 설정 파일 읽기 에러: %w", err)
		}
		// 설정 파일 미발견은 허용 - 기본값만 사용
	}

	// 6. 사용자 지정 설정 파일 읽기 및 병합 (기본 경로 파일 오버라이드)
	if lc.configFile != "" {
		userViper := viper.New()
		userViper.SetConfigFile(lc.configFile)
		if err := userViper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: 사용자 설정 파일 %q 읽기 에러: %w", lc.configFile, err)
		}
		if err := v.MergeConfigMap(userViper.AllSettings()); err != nil {
			return nil, fmt.Errorf("config: 사용자 설정 병합 에러: %w", err)
		}
	}

	// 7. 환경 변수 (파일보다 우선)
	v.SetEnvPrefix(lc.envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 8. CLI 플래그 (최우선)
	if lc.flags != nil {
		if err := v.BindPFlags(lc.flags); err != nil {
			return nil, fmt.Errorf("config: 플래그 바인딩 에러: %w", err)
		}
	}

	// 9. 유효성 검증
	if err := Validate(v); err != nil {
		return nil, err
	}

	// 10. Config 객체 생성
	vc := &viperConfig{
		v:          v,
		logger:     lc.logger,
		callbacks:  make(map[string][]callbackEntry),
		maxHistory: 100,
	}

	return vc, nil
}

// --- 카테고리 접근자 (항상 Viper에서 최신 값을 RLock으로 읽기) ---

// Server - 서버 설정 반환
func (c *viperConfig) Server() ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ServerConfig{
		Port: c.v.GetInt("server.port"),
		Host: c.v.GetString("server.host"),
		Mode: c.v.GetString("server.mode"),
		TLS: TLSConfig{
			Enabled:  c.v.GetBool("server.tls.enabled"),
			CertFile: c.v.GetString("server.tls.cert_file"),
			KeyFile:  c.v.GetString("server.tls.key_file"),
		},
		CORS: CORSConfig{
			Enabled:        c.v.GetBool("server.cors.enabled"),
			AllowedOrigins: c.v.GetStringSlice("server.cors.allowed_origins"),
		},
		RateLimit: RateLimitConfig{
			Enabled:           c.v.GetBool("server.rate_limit.enabled"),
			RequestsPerSecond: c.v.GetInt("server.rate_limit.requests_per_second"),
		},
		WebUI: WebUIConfig{
			Enabled: c.v.GetBool("server.web_ui.enabled"),
			Dir:     c.v.GetString("server.web_ui.dir"),
		},
		BasicAuth: BasicAuthConfig{
			Enabled:         c.v.GetBool("server.basic_auth.enabled"),
			CredentialsFile: c.v.GetString("server.basic_auth.credentials_file"),
			JWTSecret:       c.v.GetString("server.basic_auth.jwt_secret"),
			TokenExpiry:     c.v.GetString("server.basic_auth.token_expiry"),
			RefreshExpiry:   c.v.GetString("server.basic_auth.refresh_expiry"),
		},
	}
}

// Engine - 엔진 설정 반환
func (c *viperConfig) Engine() EngineConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return EngineConfig{
		BackpressureThreshold: c.v.GetInt("engine.backpressure_threshold"),
		MaxConcurrentFlows:    c.v.GetInt("engine.max_concurrent_flows"),
		ExecutionPolicy:       c.v.GetString("engine.execution_policy"),
		WireDefaultBuffer:     c.v.GetInt("engine.wire_default_buffer"),
	}
}

// Storage - 스토리지 설정 반환
func (c *viperConfig) Storage() StorageConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return StorageConfig{
		Type:          c.v.GetString("storage.type"),
		FileDirectory: c.v.GetString("storage.file.directory"),
		SQLitePath:    c.v.GetString("storage.sqlite.path"),
		PostgresDSN:   c.v.GetString("storage.postgres.dsn"),
		PoolSize:      c.v.GetInt("storage.pool_size"),
	}
}

// DeviceHistory - 디바이스 수신 데이터 이력(주기 스냅샷) 설정 반환.
//
// Interval 은 viper.GetDuration 으로 "10s" 형식과 정수형(ns)을 모두 수용하며,
// 0/미설정 시 안전 기본값(10s)으로 대체한다. MaxEntries 가 0 이하이면 100 으로
// 보정한다(레코더도 자체 보정하지만 설정 계층에서 명시값을 노출).
func (c *viperConfig) DeviceHistory() DeviceHistoryConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	interval := c.v.GetDuration("device_history.interval")
	if interval <= 0 {
		interval = 10 * time.Second
	}
	maxEntries := c.v.GetInt("device_history.max_entries")
	if maxEntries <= 0 {
		maxEntries = 100
	}

	return DeviceHistoryConfig{
		Enabled:    c.v.GetBool("device_history.enabled"),
		Interval:   interval,
		MaxEntries: maxEntries,
	}
}

// Auth - 인증 설정 반환
func (c *viperConfig) Auth() AuthConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return AuthConfig{
		JWT: JWTConfig{
			Secret:     c.v.GetString("auth.jwt.secret"),
			AccessTTL:  c.v.GetString("auth.jwt.access_ttl"),
			RefreshTTL: c.v.GetString("auth.jwt.refresh_ttl"),
		},
		APIKey: APIKeyConfig{
			Enabled: c.v.GetBool("auth.api_key.enabled"),
		},
		OAuth2: OAuth2Config{
			Providers: c.v.GetStringSlice("auth.oauth2.providers"),
		},
	}
}

// Observe - 관측 설정 반환
func (c *viperConfig) Observe() ObserveConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ObserveConfig{
		DefaultLevel:   c.v.GetString("observe.default_level"),
		MetricsEnabled: c.v.GetBool("observe.metrics.enabled"),
		TraceEnabled:   c.v.GetBool("observe.trace.enabled"),
		Format:         c.v.GetString("observe.format"),
		Output:         c.v.GetString("observe.output"),
	}
}

// Script - 스크립트 설정 반환
func (c *viperConfig) Script() ScriptConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ScriptConfig{
		Timeout:    c.v.GetString("script.timeout"),
		VMPoolSize: c.v.GetInt("script.vm_pool_size"),
		Sandbox: SandboxConfig{
			Enabled:        c.v.GetBool("script.sandbox.enabled"),
			MaxMemoryMB:    c.v.GetInt("script.sandbox.max_memory_mb"),
			MaxExecutionMS: c.v.GetInt("script.sandbox.max_execution_ms"),
		},
	}
}

// Plugin - 플러그인 설정 반환
func (c *viperConfig) Plugin() PluginConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return PluginConfig{
		Directory:   c.v.GetString("plugin.directory"),
		WASMEnabled: c.v.GetBool("plugin.wasm.enabled"),
		GoEnabled:   c.v.GetBool("plugin.go.enabled"),
	}
}

// Update - 자동 업데이트 설정 반환 (@SPEC:SPEC-UPDATE-001 v0.1.0).
//
// time.Duration 필드는 viper.GetDuration 를 사용해 yaml 의 "30s", "24h" 형식과
// 정수형 (단위 ns) 입력을 모두 수용한다.
func (c *viperConfig) Update() UpdateSettings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return UpdateSettings{
		Enabled:            c.v.GetBool("update.enabled"),
		Channel:            c.v.GetString("update.channel"),
		CheckInterval:      c.v.GetDuration("update.check_interval"),
		AutoApply:          c.v.GetBool("update.auto_apply"),
		NotifyOnly:         c.v.GetBool("update.notify_only"),
		UpdateURL:          c.v.GetString("update.update_url"),
		PublicKeyPath:      c.v.GetString("update.public_key_path"),
		DrainTimeout:       c.v.GetDuration("update.drain_timeout"),
		HealthCheckTimeout: c.v.GetDuration("update.health_check_timeout"),
		InsecureSkipVerify: c.v.GetBool("update.insecure_skip_verify"),
	}
}

// ParseResolution 은 "WIDTHxHEIGHT" 형식 문자열을 (width, height) 정수로 파싱한다
// (@SPEC:SPEC-REMOTE-001 M11, REQ-M01). 구분자는 대소문자 'x'/'X' 이며 주변 공백을
// 허용한다(예: "1920x1080", " 1280 X 720 ").
//
// 무효/빈값/음수/0/비정수/부동소수/추가 토큰은 모두 (0, 0)을 반환한다(미보고 폴백 —
// REQ-M03 안전 처리). 양수 width·height 둘 다 유효할 때만 (w, h)를 반환한다.
func ParseResolution(s string) (int, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0
	}
	// 'x' 또는 'X' 로 분할. 정확히 두 토큰이어야 한다(추가 토큰은 무효).
	var parts []string
	if strings.ContainsAny(s, "xX") {
		parts = strings.FieldsFunc(s, func(r rune) bool { return r == 'x' || r == 'X' })
	}
	if len(parts) != 2 {
		return 0, 0
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// resolveDisplay 는 config 의 display.resolution / display.width / display.height 를
// 해석해 효과적인 (width, height)를 반환한다(REQ-M01). 유효한 resolution("WxH")이
// width/height 보다 우선하며, resolution 이 무효/미설정이면 명시적 width/height 로
// 폴백한다. 음수/0 width·height 는 0(미보고)으로 안전 처리된다(REQ-M03).
func resolveDisplay(resolution string, width, height int) (int, int) {
	if w, h := ParseResolution(resolution); w > 0 && h > 0 {
		return w, h
	}
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

// RemoteManagement - 원격 관리 설정 반환 (@SPEC:SPEC-REMOTE-001 M1).
//
// HeartbeatInterval 은 viper.GetDuration 으로 yaml 의 "30s" 형식과 정수형(ns)
// 입력을 모두 수용하며, 0/미설정 시 안전 기본값(30s)으로 대체한다(REQ-A05).
func (c *viperConfig) RemoteManagement() RemoteManagementConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	heartbeat := c.v.GetDuration("remote_management.heartbeat_interval")
	if heartbeat <= 0 {
		heartbeat = 30 * time.Second
	}

	// 노드 해상도(v1.6 M11): resolution("WxH") 우선, 미설정/무효 시 width/height 폴백.
	dispW, dispH := resolveDisplay(
		c.v.GetString("remote_management.display.resolution"),
		c.v.GetInt("remote_management.display.width"),
		c.v.GetInt("remote_management.display.height"),
	)

	return RemoteManagementConfig{
		Mode:              c.v.GetString("remote_management.mode"),
		ServerURL:         c.v.GetString("remote_management.server_url"),
		InstanceID:        c.v.GetString("remote_management.instance_id"),
		AutoRegister:      c.v.GetBool("remote_management.auto_register"),
		HeartbeatInterval: heartbeat,
		BootstrapSecret:   c.v.GetString("remote_management.bootstrap_secret"),
		EnrollmentToken:   c.v.GetString("remote_management.enrollment_token"),
		Exposure: ExposureConfig{
			Flows:   c.v.GetString("remote_management.exposure.flows"),
			Agents:  c.v.GetString("remote_management.exposure.agents"),
			Devices: c.v.GetString("remote_management.exposure.devices"),
		},
		Display: DisplayConfig{
			Width:  dispW,
			Height: dispH,
		},
		TLS: TLSConfig{
			Enabled:  c.v.GetBool("remote_management.tls.enabled"),
			CertFile: c.v.GetString("remote_management.tls.cert_file"),
			KeyFile:  c.v.GetString("remote_management.tls.key_file"),
		},
		RequireSecure:      c.v.GetBool("remote_management.require_secure"),
		InsecureSkipVerify: c.v.GetBool("remote_management.insecure_skip_verify"),
		PublicBaseURL:      c.v.GetString("remote_management.public_base_url"),
		ReleasesDir:        c.v.GetString("remote_management.releases_dir"),
	}
}

// --- 범용 접근 메서드 ---

// Get - 키에 대응하는 값 반환
func (c *viperConfig) Get(key string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.v.Get(key)
}

// IsSet - 키가 설정되어 있는지 확인
func (c *viperConfig) IsSet(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.v.IsSet(key)
}

// --- 런타임 변경 메서드 ---

// Set - 런타임에 설정 값 변경 (mutable 키만 허용)
func (c *viperConfig) Set(key string, value any) error {
	if !IsMutable(key) {
		return fmt.Errorf("%w: %s", ErrImmutableKey, key)
	}

	// 런타임 유효성 검증 (검증 실패 시 값 변경/콜백/이력 없음)
	if err := validateKeyValue(key, value); err != nil {
		return err
	}

	c.mu.Lock()
	oldValue := c.v.Get(key)
	c.v.Set(key, value)
	c.mu.Unlock()

	// 변경 이벤트 생성
	event := ChangeEvent{
		Key:       key,
		OldValue:  oldValue,
		NewValue:  value,
		Source:    "api",
		Timestamp: time.Now(),
	}

	// 이력 기록
	c.historyMu.Lock()
	if len(c.history) >= c.maxHistory {
		c.history = c.history[1:]
	}
	c.history = append(c.history, event)
	c.historyMu.Unlock()

	// 콜백 호출 (panic 복구 포함)
	c.callbacksMu.RLock()
	entries := c.callbacks[key]
	// 슬라이스 복사하여 락 해제 후 안전하게 호출
	copied := make([]callbackEntry, len(entries))
	copy(copied, entries)
	c.callbacksMu.RUnlock()

	for _, entry := range copied {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if c.logger != nil {
						c.logger.Error("config: 콜백 panic 복구됨",
							"key", key,
							"panic", r,
						)
					}
				}
			}()
			entry.fn(event)
		}()
	}

	return nil
}

// OnChange - 특정 키의 변경 콜백 등록, 구독 해제 함수 반환
func (c *viperConfig) OnChange(key string, fn ChangeCallback) UnsubscribeFunc {
	id := int(c.nextID.Add(1))

	c.callbacksMu.Lock()
	c.callbacks[key] = append(c.callbacks[key], callbackEntry{
		id: id,
		fn: fn,
	})
	c.callbacksMu.Unlock()

	return func() {
		c.callbacksMu.Lock()
		defer c.callbacksMu.Unlock()

		entries := c.callbacks[key]
		for i, entry := range entries {
			if entry.id == id {
				c.callbacks[key] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// ChangeHistory - 변경 이력 반환
func (c *viperConfig) ChangeHistory() []ChangeEvent {
	c.historyMu.RLock()
	defer c.historyMu.RUnlock()

	result := make([]ChangeEvent, len(c.history))
	copy(result, c.history)
	return result
}

// --- 파일 감시 메서드 ---

// WatchConfig - 설정 파일 변경 감시 시작 (fsnotify 기반)
func (c *viperConfig) WatchConfig() error {
	c.mu.Lock()
	if c.watching {
		c.mu.Unlock()
		return nil
	}
	c.watchCtx, c.watchCancel = context.WithCancel(context.Background())
	c.watching = true
	configFile := c.v.ConfigFileUsed()
	c.mu.Unlock()

	if configFile == "" {
		return nil // 감시할 설정 파일 없음
	}

	c.v.OnConfigChange(func(e fsnotify.Event) {
		// 디바운스: 100ms 이내 이벤트 무시
		c.debounceMu.Lock()
		now := time.Now()
		if now.Sub(c.lastChange) < 100*time.Millisecond {
			c.debounceMu.Unlock()
			return
		}
		c.lastChange = now
		c.debounceMu.Unlock()

		// 등록된 콜백에 source="file" 이벤트 전달
		c.callbacksMu.RLock()
		allCallbacks := make(map[string][]callbackEntry)
		for k, entries := range c.callbacks {
			copied := make([]callbackEntry, len(entries))
			copy(copied, entries)
			allCallbacks[k] = copied
		}
		c.callbacksMu.RUnlock()

		for key, entries := range allCallbacks {
			c.mu.RLock()
			currentVal := c.v.Get(key)
			c.mu.RUnlock()

			event := ChangeEvent{
				Key:       key,
				OldValue:  nil, // 파일 변경 시 이전 값 추적 불가
				NewValue:  currentVal,
				Source:    "file",
				Timestamp: now,
			}
			for _, entry := range entries {
				func(fn ChangeCallback, evt ChangeEvent) {
					defer func() { recover() }()
					fn(evt)
				}(entry.fn, event)
			}
		}
	})
	c.v.WatchConfig()
	return nil
}

// StopWatch - 설정 파일 변경 감시 중지
func (c *viperConfig) StopWatch() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.watching {
		return
	}

	if c.watchCancel != nil {
		c.watchCancel()
	}
	c.watching = false
}

// --- 런타임 유효성 검증 헬퍼 ---

// validateKeyValue - 단일 키의 값 유효성 검증 (Set() 내부에서 사용)
func validateKeyValue(key string, value any) error {
	switch key {
	case "observe.default_level":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: observe.default_level은 문자열이어야 합니다", ErrInvalidLogLevel)
		}
		switch s {
		case "debug", "info", "warn", "error":
			return nil
		default:
			return fmt.Errorf("%w: observe.default_level=%q", ErrInvalidLogLevel, s)
		}

	case "engine.backpressure_threshold", "engine.max_concurrent_flows":
		n, ok := toInt(value)
		if !ok || n <= 0 {
			return fmt.Errorf("%w: %s는 양수 정수여야 합니다", ErrInvalidPositiveValue, key)
		}

	case "server.rate_limit.requests_per_second":
		n, ok := toInt(value)
		if !ok || n <= 0 {
			return fmt.Errorf("%w: %s는 양수 정수여야 합니다", ErrInvalidPositiveValue, key)
		}

	case "auth.jwt.access_ttl", "auth.jwt.refresh_ttl":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: %s는 문자열이어야 합니다", ErrInvalidDuration, key)
		}
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf("%w: %s=%q", ErrInvalidDuration, key, s)
		}

	case "remote_management.mode":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: remote_management.mode는 문자열이어야 합니다", ErrInvalidRemoteMode)
		}
		switch s {
		case "disabled", "server", "client":
			return nil
		default:
			return fmt.Errorf("%w: remote_management.mode=%q", ErrInvalidRemoteMode, s)
		}
	}
	return nil
}

// toInt - 값을 int로 변환 (int, int64, float64 지원)
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
