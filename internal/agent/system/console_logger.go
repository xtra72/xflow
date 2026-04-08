package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	xflowio "github.com/xtra/xflow/internal/io"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ConsoleLoggerConfig 는 ConsoleLoggerAgent의 설정을 담는 구조체이다.
type ConsoleLoggerConfig struct {
	// Prefix 는 로그 출력 시 앞에 붙는 접두어이다.
	Prefix string
	// Level 은 최소 로그 레벨이다 (기본: INFO).
	Level slog.Level
	// Output 은 출력 대상이다. "stdout"(기본), "stderr", 또는 파일 경로.
	Output string
	// Format 은 로그 출력 형식이다. "text"(기본) 또는 "json".
	Format string
	// MaxSize 는 롤링 파일의 최대 크기(바이트). 기본값: 10MB.
	MaxSize int64
	// MaxAge 는 백업 파일 최대 보관 일수. 0이면 무제한.
	MaxAge int
	// MaxBackups 는 백업 파일 최대 개수. 0이면 무제한.
	MaxBackups int
	// Compress 는 백업 파일을 gzip 압축할지 여부이다.
	Compress bool
	// ContentMode 는 메시지에서 출력할 부분을 선택한다. "full"(기본) 또는 "payload".
	ContentMode string
}

// parseConsoleLoggerConfig 는 AgentConfig에서 ConsoleLoggerConfig를 추출한다.
func parseConsoleLoggerConfig(cfg agent.AgentConfig) ConsoleLoggerConfig {
	cc := ConsoleLoggerConfig{
		Prefix:      "[logger]",
		Level:       slog.LevelInfo,
		ContentMode: "full",
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return cc
	}

	if v, ok := opts["prefix"].(string); ok && v != "" {
		cc.Prefix = v
	}

	if v, ok := opts["output"].(string); ok && v != "" {
		cc.Output = v
	}

	if v, ok := opts["format"].(string); ok && v != "" {
		cc.Format = v
	}

	// max_size: 사용자가 MB 단위로 지정, 내부에서는 바이트로 변환
	if v, ok := opts["max_size"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxSize = int64(n) * 1024 * 1024
		case int64:
			cc.MaxSize = n * 1024 * 1024
		case float64:
			cc.MaxSize = int64(n) * 1024 * 1024
		}
	}

	// output 이 파일 경로이고 max_size 가 미지정이면 기본 10MB 설정
	if cc.Output != "" && cc.Output != "stdout" && cc.Output != "stderr" && cc.MaxSize == 0 {
		cc.MaxSize = 10 * 1024 * 1024
	}

	if v, ok := opts["max_age"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxAge = n
		case float64:
			cc.MaxAge = int(n)
		}
	}

	if v, ok := opts["max_backups"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxBackups = n
		case float64:
			cc.MaxBackups = int(n)
		}
	}

	if v, ok := opts["compress"].(bool); ok {
		cc.Compress = v
	}

	if v, ok := opts["content_mode"].(string); ok && v != "" {
		cc.ContentMode = v
	}

	return cc
}

// resolveWriter 는 설정에 따라 적절한 writer 를 생성한다.
func resolveWriter(cc ConsoleLoggerConfig) (io.Writer, io.Closer, error) {
	switch cc.Output {
	case "", "stdout":
		return os.Stdout, nil, nil
	case "stderr":
		return os.Stderr, nil, nil
	default:
		// 파일 경로 — MaxSize > 0 이면 RollingWriter 사용
		if cc.MaxSize > 0 {
			rw, err := xflowio.NewRollingWriter(xflowio.RollingWriterConfig{
				FilePath:   cc.Output,
				MaxSize:    cc.MaxSize,
				MaxAge:     cc.MaxAge,
				MaxBackups: cc.MaxBackups,
				Compress:   cc.Compress,
			})
			return rw, rw, err
		}
		// 단순 파일 추가 모드
		dir := filepath.Dir(cc.Output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, nil, fmt.Errorf("logger: 디렉터리 생성 실패: %w", err)
		}
		f, err := os.OpenFile(cc.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("logger: 파일 열기 실패: %w", err)
		}
		return f, f, nil
	}
}

// createLogger 는 지정된 형식과 writer 로 slog.Logger 를 생성한다.
func createLogger(w io.Writer, cc ConsoleLoggerConfig) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cc.Level}
	var handler slog.Handler
	if cc.Format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
}

// managedFileWriter 는 PublishMessage 를 통해 동적으로 생성되는 파일 writer 이다.
// topic(파일 경로)별로 하나씩 생성되며, 롤링 설정은 에이전트 config 에서 가져온다.
type managedFileWriter struct {
	writer io.Writer
	closer io.Closer
}

// ConsoleLoggerAgent 는 수신한 메시지를 표준 출력에 기록하는 싱크 에이전트이다.
// BridgeOut 방향의 플로우 종단에서 사용된다.
// MessagePublisher 를 구현하여 Bridge 노드의 publish_topic 을 파일 경로로 해석,
// 토픽(파일 경로)별 writer 를 자동 생성·관리한다.
type ConsoleLoggerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	logConfig   ConsoleLoggerConfig
	logger      *slog.Logger
	stats       *agent.AgentStats
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
	writer      io.Writer // Process() 기본 출력 대상
	closer      io.Closer // Process() 파일 정리용
	fileWriters map[string]*managedFileWriter // PublishMessage 용 파일 writer 맵
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*ConsoleLoggerAgent)(nil)
var _ agent.MessagePublisher = (*ConsoleLoggerAgent)(nil)

// NewConsoleLoggerAgent 는 ConsoleLoggerAgent 팩토리 함수이다.
func NewConsoleLoggerAgent(config agent.AgentConfig) (agent.Agent, error) {
	cc := parseConsoleLoggerConfig(config)

	// writer/closer 설정
	w, c, err := resolveWriter(cc)
	if err != nil {
		return nil, fmt.Errorf("logger: writer 설정 실패: %w", err)
	}

	// 로거 생성: config.Logger 가 명시적으로 설정된 경우 해당 로거를 사용,
	// 그렇지 않으면 writer 기반 로거 생성
	var logger *slog.Logger
	if config.Logger != nil {
		logger = agent.ResolveLogger(config)
	} else {
		logger = createLogger(w, cc)
	}

	a := &ConsoleLoggerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("logger")),
		logConfig:     cc,
		logger:        logger,
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
		writer:        w,
		closer:        c,
		fileWriters:   make(map[string]*managedFileWriter),
	}

	if err := a.Init(config); err != nil {
		// 초기화 실패 시 파일 정리
		if c != nil {
			c.Close()
		}
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *ConsoleLoggerAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("logger init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("logger init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("logger init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 에이전트를 시작한다. 이미 Running이면 no-op.
// Stopped 상태이면 Created로 리셋 후 Init()을 재호출하여 재시작한다.
func (a *ConsoleLoggerAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("logger start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("logger: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 에이전트를 정지한다. closer 가 있으면 닫는다.
func (a *ConsoleLoggerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("logger stop: %w", err)
	}

	// 파일 closer 정리
	a.mu.Lock()
	if a.closer != nil {
		a.closer.Close()
		a.closer = nil
	}
	// PublishMessage 용 파일 writer 모두 정리
	for path, fw := range a.fileWriters {
		if fw.closer != nil {
			fw.closer.Close()
		}
		delete(a.fileWriters, path)
	}
	a.mu.Unlock()

	return a.TransitionTo(lifecycle.StateStopped)
}

// Pause 는 에이전트를 일시정지한다.
func (a *ConsoleLoggerAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *ConsoleLoggerAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *ConsoleLoggerAgent) Health() agent.HealthStatus {
	state := a.CurrentState()
	if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
		return agent.HealthStatus{Status: agent.HealthHealthy}
	}
	return agent.HealthStatus{Status: agent.HealthUnhealthy}
}

// extractContent 는 content_mode 에 따라 출력할 데이터를 결정한다.
// "full": 데이터를 그대로 반환한다.
// "payload": JSON 에서 "payload" 키의 값을 추출한다. 실패 시 원본 데이터를 반환한다.
func (a *ConsoleLoggerAgent) extractContent(data []byte) []byte {
	a.mu.RLock()
	mode := a.logConfig.ContentMode
	a.mu.RUnlock()

	if mode != "payload" {
		return data
	}

	var msg map[string]json.RawMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return data // JSON 이 아닌 경우 원본 반환
	}

	payload, ok := msg["payload"]
	if !ok {
		return data // payload 키가 없는 경우 원본 반환
	}

	// payload 가 JSON 문자열이면 언퀴트한다
	var s string
	if err := json.Unmarshal(payload, &s); err == nil {
		return []byte(s)
	}

	// 그 외(오브젝트, 배열, 숫자 등)는 Raw JSON 반환
	return payload
}

// formatHexDump 은 hexdump -C 스타일의 출력을 생성한다.
// 각 줄: [prefix] OFFSET  HH HH ... HH  HH HH ... HH  |ASCII...|
func (a *ConsoleLoggerAgent) formatHexDump(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var sb strings.Builder
	prefix := a.logConfig.Prefix

	for offset := 0; offset < len(data); offset += 16 {
		// prefix + offset
		fmt.Fprintf(&sb, "%s %08x  ", prefix, offset)

		// hex bytes — 8 + 8 그룹
		end := offset + 16
		if end > len(data) {
			end = len(data)
		}

		for i := offset; i < offset+16; i++ {
			if i < end {
				fmt.Fprintf(&sb, "%02x ", data[i])
			} else {
				sb.WriteString("   ")
			}
			if i == offset+7 {
				sb.WriteByte(' ')
			}
		}

		// ASCII 표현
		sb.WriteByte(' ')
		sb.WriteByte('|')
		for i := offset; i < end; i++ {
			if data[i] >= 0x20 && data[i] <= 0x7e {
				sb.WriteByte(data[i])
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteByte('|')
		sb.WriteByte('\n')
	}

	return sb.String()
}

// Process 는 수신한 데이터를 콘솔에 출력한다.
func (a *ConsoleLoggerAgent) Process(data []byte) ([]byte, error) {
	a.stats.IncrExternalMessagesReceived()

	content := a.extractContent(data)

	if a.logConfig.Format == "binary" {
		dump := a.formatHexDump(content)
		if dump != "" {
			a.mu.RLock()
			_, _ = a.writer.Write([]byte(dump))
			a.mu.RUnlock()
		}
	} else {
		a.logger.Info("message received",
			"prefix", a.logConfig.Prefix,
			"payload", string(content),
		)
	}

	a.stats.IncrExternalMessagesSent()
	return nil, nil
}

// Configure 는 에이전트 설정을 변경한다.
// 기존 closer 가 있으면 닫고, 새 writer/logger 를 설정한다.
func (a *ConsoleLoggerAgent) Configure(config agent.AgentConfig) error {
	newCC := parseConsoleLoggerConfig(config)

	// 새 writer/closer 생성
	w, c, err := resolveWriter(newCC)
	if err != nil {
		return fmt.Errorf("logger configure: %w", err)
	}

	// 새 로거 생성
	var logger *slog.Logger
	if config.Logger != nil {
		logger = agent.ResolveLogger(config)
	} else {
		logger = createLogger(w, newCC)
	}

	a.mu.Lock()
	// 기존 closer 정리
	if a.closer != nil {
		a.closer.Close()
	}
	// PublishMessage 용 파일 writer 모두 정리 (설정 변경 시 재생성)
	for path, fw := range a.fileWriters {
		if fw.closer != nil {
			fw.closer.Close()
		}
		delete(a.fileWriters, path)
	}
	a.agentConfig = config
	a.logConfig = newCC
	a.writer = w
	a.closer = c
	a.logger = logger
	a.mu.Unlock()

	return nil
}

// ID 는 에이전트의 고유 식별자를 반환한다.
func (a *ConsoleLoggerAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트의 이름을 반환한다.
func (a *ConsoleLoggerAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트의 타입을 반환한다.
func (a *ConsoleLoggerAgent) Type() string {
	return "logger"
}

// Info 는 에이전트의 상세 정보를 반환한다.
func (a *ConsoleLoggerAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var uptime time.Duration
	if !a.startedAt.IsZero() {
		uptime = time.Since(a.startedAt)
	}

	return agent.AgentInfo{
		ID:     a.agentConfig.ID,
		Name:   a.agentConfig.Name,
		Type:   "logger",
		State:  a.CurrentState(),
		Config: a.agentConfig,
		Stats:  a.stats.Snapshot(),
		Uptime: uptime,
	}
}

// Stats 는 에이전트의 처리 통계를 반환한다.
func (a *ConsoleLoggerAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// PublishMessage 는 topic 을 파일 경로로 해석하여 데이터를 기록한다.
// Bridge 노드의 publish_topic 값이 파일 경로로 전달된다.
// topic 이 비어있으면 기본 Process() 로 폴백한다.
func (a *ConsoleLoggerAgent) PublishMessage(topic string, _ byte, _ bool, payload []byte) error {
	a.stats.IncrExternalMessagesReceived()

	content := a.extractContent(payload)

	if topic == "" {
		if a.logConfig.Format == "binary" {
			dump := a.formatHexDump(content)
			if dump != "" {
				a.mu.RLock()
				_, _ = a.writer.Write([]byte(dump))
				a.mu.RUnlock()
			}
		} else {
			a.logger.Info("message received",
				"prefix", a.logConfig.Prefix,
				"payload", string(content),
			)
		}
		a.stats.IncrExternalMessagesSent()
		return nil
	}

	fw, err := a.getOrCreateFileWriter(topic)
	if err != nil {
		return fmt.Errorf("logger: publish to %s: %w", topic, err)
	}

	a.mu.RLock()
	if a.logConfig.Format == "binary" {
		dump := a.formatHexDump(content)
		if dump != "" {
			_, err = fw.writer.Write([]byte(dump))
		}
	} else {
		_, err = fw.writer.Write(append(content, '\n'))
	}
	a.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("logger: write to %s: %w", topic, err)
	}

	a.stats.IncrExternalMessagesSent()
	return nil
}

// getOrCreateFileWriter 는 filePath 에 해당하는 writer 를 반환한다.
// 없으면 에이전트의 롤링 설정을 사용하여 새로 생성한다.
func (a *ConsoleLoggerAgent) getOrCreateFileWriter(filePath string) (*managedFileWriter, error) {
	// fast path: 읽기 잠금으로 먼저 확인
	a.mu.RLock()
	if fw, ok := a.fileWriters[filePath]; ok {
		a.mu.RUnlock()
		return fw, nil
	}
	a.mu.RUnlock()

	// slow path: 쓰기 잠금으로 생성
	a.mu.Lock()
	defer a.mu.Unlock()

	// 이중 체크
	if fw, ok := a.fileWriters[filePath]; ok {
		return fw, nil
	}

	// 에이전트의 롤링 설정을 사용하여 파일 writer 생성
	cc := a.logConfig
	cc.Output = filePath

	w, c, err := resolveWriter(cc)
	if err != nil {
		return nil, err
	}

	fw := &managedFileWriter{writer: w, closer: c}
	a.fileWriters[filePath] = fw
	return fw, nil
}
