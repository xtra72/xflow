package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	Auth() AuthConfig
	Observe() ObserveConfig
	Script() ScriptConfig
	Plugin() PluginConfig

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
	configFile  string               // 사용자 지정 설정 파일 경로
	configName  string               // 확장자 없는 설정 파일명 (예: "xflowd")
	configPaths []string             // 기본 설정 파일 검색 경로
	envPrefix   string               // 환경 변수 접두사 (기본: "XFLOW")
	flags       *pflag.FlagSet       // CLI 플래그 셋
	defaultsFn  func(*viper.Viper)   // 커스텀 기본값 함수
	logger      *slog.Logger         // 선택적 로거
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
		Type:        c.v.GetString("storage.type"),
		SQLitePath:  c.v.GetString("storage.sqlite.path"),
		PostgresDSN: c.v.GetString("storage.postgres.dsn"),
		PoolSize:    c.v.GetInt("storage.pool_size"),
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
