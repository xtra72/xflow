package system

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// InfluxDBAgent 는 InfluxDB 연동 에이전트이다.
// agent.Agent, agent.MessageReceiver 인터페이스를 구현한다.
type InfluxDBAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig  agent.AgentConfig
	influxConfig InfluxDBConfig
	client       InfluxClient
	recvCh       chan []byte
	done         chan struct{}
	stats        *agent.AgentStats
	logger       *slog.Logger
	mu           sync.RWMutex
	startedAt    time.Time
	createdAt    time.Time
	paused       bool
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*InfluxDBAgent)(nil)
var _ agent.MessageReceiver = (*InfluxDBAgent)(nil)
var _ agent.BufferInfoProvider = (*InfluxDBAgent)(nil)

// NewInfluxDBAgent 는 InfluxDBAgent 팩토리 함수이다.
func NewInfluxDBAgent(config agent.AgentConfig) (agent.Agent, error) {
	ic, err := parseInfluxDBConfig(config)
	if err != nil {
		return nil, fmt.Errorf("influxdb agent: %w", err)
	}

	a := &InfluxDBAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("influxdb")),
		influxConfig:  ic,
		recvCh:        make(chan []byte, ic.BufferSize),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화하고 InfluxDB 클라이언트를 생성한다.
func (a *InfluxDBAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("influxdb init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("influxdb init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// InfluxDB 클라이언트 생성
	client, err := NewInfluxClient(a.influxConfig)
	if err != nil {
		_ = a.TransitionTo(lifecycle.StateError)
		return fmt.Errorf("influxdb init: %w", err)
	}
	a.client = client

	// 헬스체크
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()
	if err := client.Health(ctx); err != nil {
		a.logger.Warn("influxdb: 헬스체크 실패 (계속 진행)", "error", err)
		// init 은 실패시키지 않음 - 서버가 나중에 사용 가능해질 수 있음
	}

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("influxdb init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("influxdb: 에이전트 초기화 완료",
		"url", a.influxConfig.URL,
		"version", a.influxConfig.Version,
		"bucket", a.influxConfig.Bucket,
	)

	return nil
}

// Start 는 이미 Running 상태이면 no-op 이다.
func (a *InfluxDBAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("influxdb start: not in running state (current: %s)", a.CurrentState())
}

// Stop 은 클라이언트를 닫고 에이전트를 정지한다.
func (a *InfluxDBAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("influxdb stop: %w", err)
	}

	// 클라이언트 닫기
	if a.client != nil {
		if err := a.client.Close(); err != nil {
			a.logger.Warn("influxdb: client close error", "error", err)
		}
	}

	// ReceiveMessage 대기자에게 종료 시그널
	close(a.done)

	// recvCh 드레인
	for {
		select {
		case <-a.recvCh:
			// 드레인
		default:
			if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
				return fmt.Errorf("influxdb stop: %w", err)
			}
			return nil
		}
	}
}

// Pause 는 Running -> Paused 로 전환한다.
func (a *InfluxDBAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("influxdb pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume 은 Paused -> Running 으로 전환한다.
func (a *InfluxDBAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("influxdb resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *InfluxDBAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   "influxdb agent is running",
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "influxdb agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("influxdb agent is in %s state", state),
		}
	}
}

// Process 는 입력 데이터의 타입(쓰기/쿼리)을 자동 판별하여 처리한다.
func (a *InfluxDBAgent) Process(data []byte) ([]byte, error) {
	a.mu.RLock()
	paused := a.paused
	a.mu.RUnlock()

	if paused {
		return nil, fmt.Errorf("influxdb: agent is paused")
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("influxdb: empty data")
	}

	// 요청 타입을 판별한다
	// 먼저 JSON 배열(배치 쓰기)을 시도
	var rawArray []json.RawMessage
	if err := json.Unmarshal(data, &rawArray); err == nil {
		return a.processWriteBatch(data)
	}

	// JSON 객체를 시도
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("influxdb: 유효하지 않은 JSON 형식: %w", err)
	}

	// 쿼리 요청 ("query" 키를 가짐)
	if _, hasQuery := obj["query"]; hasQuery {
		return a.processQuery(data)
	}

	// 쓰기 요청 ("measurement" 키를 가짐)
	if _, hasMeasurement := obj["measurement"]; hasMeasurement {
		return a.processWriteSingle(data)
	}

	return nil, fmt.Errorf("influxdb: 지원하지 않는 요청 형식 ('query' 또는 'measurement' 필드, 혹은 JSON 배열이 필요합니다)")
}

// processWriteSingle 은 단일 쓰기 요청을 처리한다.
func (a *InfluxDBAgent) processWriteSingle(data []byte) ([]byte, error) {
	var wd WriteData
	if err := json.Unmarshal(data, &wd); err != nil {
		return nil, fmt.Errorf("influxdb write: %w", err)
	}

	if err := validateWriteData(&wd); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	if err := a.client.Write(ctx, []WriteData{wd}); err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("influxdb: 쓰기 실패", "error", err)
		return nil, fmt.Errorf("influxdb write: %w", err)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(data)))
	a.stats.UpdateLastActivity()

	return nil, nil
}

// processWriteBatch 는 배치 쓰기 요청을 처리한다.
func (a *InfluxDBAgent) processWriteBatch(data []byte) ([]byte, error) {
	var wds []WriteData
	if err := json.Unmarshal(data, &wds); err != nil {
		return nil, fmt.Errorf("influxdb batch write: %w", err)
	}

	for i := range wds {
		if err := validateWriteData(&wds[i]); err != nil {
			return nil, fmt.Errorf("influxdb batch write[%d]: %w", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	if err := a.client.Write(ctx, wds); err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("influxdb: 배치 쓰기 실패", "error", err)
		return nil, fmt.Errorf("influxdb batch write: %w", err)
	}

	a.stats.IncrMessagesSent()
	a.stats.AddBytesWritten(int64(len(data)))
	a.stats.UpdateLastActivity()

	return nil, nil
}

// processQuery 는 쿼리 요청을 처리한다.
func (a *InfluxDBAgent) processQuery(data []byte) ([]byte, error) {
	var qr QueryRequest
	if err := json.Unmarshal(data, &qr); err != nil {
		return nil, fmt.Errorf("influxdb query: %w", err)
	}

	if qr.Query == "" {
		return nil, fmt.Errorf("influxdb query: query 는 필수입니다")
	}

	// 언어가 지정되지 않은 경우 기본값 사용
	lang := qr.Language
	if lang == "" {
		lang = a.influxConfig.QueryLanguage
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	rows, err := a.client.Query(ctx, qr.Query, lang)
	if err != nil {
		a.stats.IncrMessagesErrored()
		a.logger.Error("influxdb: 쿼리 실패", "error", err, "language", lang)
		return nil, fmt.Errorf("influxdb query: %w", err)
	}

	// 결과를 JSON 으로 직렬화
	result, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("influxdb query result marshal: %w", err)
	}

	// ReceiveMessage 용으로 recvCh 에 결과를 보낸다
	select {
	case a.recvCh <- result:
		a.stats.IncrMessagesReceived()
		a.stats.AddBytesRead(int64(len(result)))
	default:
		a.logger.Warn("influxdb: 결과 버퍼가 가득 찼습니다, 결과를 드롭합니다")
		a.stats.IncrMessagesErrored()
	}

	a.stats.UpdateLastActivity()

	return nil, nil
}

// validateWriteData 는 쓰기 데이터를 검증한다.
func validateWriteData(wd *WriteData) error {
	if wd.Measurement == "" {
		return fmt.Errorf("influxdb write: measurement 는 필수입니다")
	}
	if len(wd.Fields) == 0 {
		return fmt.Errorf("influxdb write: 최소 1개의 필드가 필요합니다")
	}
	return nil
}

// ReceiveMessage 는 쿼리 결과 채널에서 결과를 읽어 반환한다.
// agent.MessageReceiver 인터페이스 구현.
func (a *InfluxDBAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data := <-a.recvCh:
		return data, nil
	case <-a.done:
		return nil, fmt.Errorf("influxdb: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Configure 는 에이전트 설정을 업데이트한다.
func (a *InfluxDBAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("influxdb configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID 는 에이전트 ID 를 반환한다.
func (a *InfluxDBAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *InfluxDBAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *InfluxDBAgent) Type() string {
	return "influxdb"
}

// Info 는 에이전트 정보의 스냅샷을 반환한다.
func (a *InfluxDBAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      "influxdb",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// BufferInfo returns the pending and capacity of the receive buffer.
func (a *InfluxDBAgent) BufferInfo() (int, int) {
	return len(a.recvCh), cap(a.recvCh)
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *InfluxDBAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
}
