package system

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/observe"
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
	doneOnce     sync.Once
	stats        *agent.AgentStats
	logger       *slog.Logger
	mu           sync.RWMutex
	startedAt    time.Time
	createdAt    time.Time
	paused       bool

	// deviceResolver 는 SPEC-DEVICE-IDENTITY-001 Phase C § C3 의 dual-tag 부착
	// 시 composite tag value 를 UUID 로 변환하기 위한 lookup 인터페이스이다.
	//
	// nil 이면 dual-tag 부착이 자동 비활성화 (graceful degradation) — agent 가
	// 단독 테스트 환경 또는 device.Registry 미주입 시 안전하게 동작한다.
	//
	// WithDeviceResolver 옵션으로 주입되며, cmd/xflowd 의 startup 코드가
	// device.Registry 를 주입한다 (DeviceRegistry 가 본 인터페이스 자연 만족).
	deviceResolver DeviceResolver
}

// InfluxDBAgentOption 은 NewInfluxDBAgentWithOptions 의 가변 인자 구성이다.
//
// 옵션 함수형 패턴 — 추후 새 의존성 추가 시 시그니처 변경 없이 확장 가능.
//
// SPEC-DEVICE-IDENTITY-001 Phase C § C3.
type InfluxDBAgentOption func(*InfluxDBAgent)

// WithDeviceResolver 는 dual-tag 부착에 사용할 DeviceResolver 를 주입한다.
//
// resolver 가 nil 이면 옵션은 no-op (기본 동작 유지 — dual-tag 부착 없음).
// device.DeviceRegistry 가 본 인터페이스를 자연 만족하므로 cmd/xflowd 에서
// deviceRegistry 를 직접 전달 가능.
//
// SPEC-DEVICE-IDENTITY-001 Phase C § C3.
func WithDeviceResolver(resolver DeviceResolver) InfluxDBAgentOption {
	return func(a *InfluxDBAgent) {
		a.deviceResolver = resolver
	}
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*InfluxDBAgent)(nil)
var _ agent.MessageReceiver = (*InfluxDBAgent)(nil)
var _ agent.BufferInfoProvider = (*InfluxDBAgent)(nil)

// NewInfluxDBAgent 는 InfluxDBAgent 팩토리 함수이다 (옵션 없는 표준 경로).
//
// 본 함수는 NewInfluxDBAgentWithOptions 에 빈 옵션 슬라이스를 전달하는 wrapper
// 로 동작하여 하위 호환을 유지한다. 기존 호출자 코드 무수정.
func NewInfluxDBAgent(config agent.AgentConfig) (agent.Agent, error) {
	return NewInfluxDBAgentWithOptions(config)
}

// NewInfluxDBAgentWithOptions 는 옵션을 지원하는 InfluxDBAgent 팩토리이다.
//
// 옵션 함수형 패턴 — 현재는 WithDeviceResolver (Phase C § C3) 만 정의되어
// 있다. cmd/xflowd 의 startup 코드는 RegisterInfluxDBTypesWithResolver 를
// 거쳐 본 팩토리를 호출한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase C § C3.
func NewInfluxDBAgentWithOptions(config agent.AgentConfig, opts ...InfluxDBAgentOption) (agent.Agent, error) {
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
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}

	// 옵션 적용 — Init 전에 deviceResolver 등 의존성을 주입한다.
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
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
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("influxdb start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("influxdb start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 클라이언트를 닫고 에이전트를 정지한다.
func (a *InfluxDBAgent) Stop(_ context.Context) error {
	// 상태 전환 시도 — 실패해도 리소스 정리는 수행
	transErr := a.TransitionTo(lifecycle.StateStopping)

	// ReceiveMessage 대기자에게 종료 시그널 (이중 close 방지)
	a.doneOnce.Do(func() { close(a.done) })

	// 클라이언트 닫기 (타임아웃 5초 — Close가 블로킹될 수 있음)
	if a.client != nil {
		closeDone := make(chan error, 1)
		go func() { closeDone <- a.client.Close() }()
		select {
		case err := <-closeDone:
			if err != nil {
				a.logger.Warn("influxdb: client close error", "error", err)
			}
		case <-time.After(5 * time.Second):
			a.logger.Warn("influxdb: client close timed out (5s)")
		}
	}

	// recvCh 드레인
	for {
		select {
		case <-a.recvCh:
		default:
			goto drained
		}
	}
drained:

	if transErr != nil {
		// 상태 전환 실패 시에도 Stopped로 강제 전환 시도
		_ = a.TransitionTo(lifecycle.StateStopped)
		return nil
	}

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("influxdb stop: %w", err)
	}
	return nil
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

	// SPEC-DEVICE-IDENTITY-001 Phase C § C3: dual-tag 부착.
	// validateWriteData 직후, client.Write 직전에 적용. 기본 ON 이지만 resolver
	// 가 nil 이면 안전하게 skip (단독 테스트 환경 graceful degradation).
	if a.influxConfig.DualTagEmit && a.deviceResolver != nil {
		state := augmentWriteDataWithUID(&wd, a.deviceResolver, a.influxConfig.DualTagEmitSourceKeys)
		observe.IncTSDBDualTag(state)
	}

	// v0.16.4: debug 활성화 시 전송할 WriteData 를 로그.
	if a.influxConfig.Debug {
		a.logger.Debug("influxdb: write 전송",
			"measurement", wd.Measurement,
			"tags", wd.Tags,
			"fields", wd.Fields,
			"timestamp", wd.Timestamp,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	if err := a.client.Write(ctx, []WriteData{wd}); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("influxdb: 쓰기 실패", "error", err)
		return nil, fmt.Errorf("influxdb write: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
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

	// SPEC-DEVICE-IDENTITY-001 Phase C § C3: 배치의 각 WriteData 에 dual-tag 부착.
	// 각 항목별로 분류되어 메트릭에 기록된다. 한 배치에 mapped + unmapped 등
	// 다양한 state 가 혼재해도 정확히 카운팅된다.
	if a.influxConfig.DualTagEmit && a.deviceResolver != nil {
		for i := range wds {
			state := augmentWriteDataWithUID(&wds[i], a.deviceResolver, a.influxConfig.DualTagEmitSourceKeys)
			observe.IncTSDBDualTag(state)
		}
	}

	// v0.16.4: debug 활성화 시 배치의 각 WriteData 를 로그.
	if a.influxConfig.Debug {
		for i := range wds {
			a.logger.Debug("influxdb: batch write 전송",
				"index", i,
				"total", len(wds),
				"measurement", wds[i].Measurement,
				"tags", wds[i].Tags,
				"fields", wds[i].Fields,
				"timestamp", wds[i].Timestamp,
			)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	if err := a.client.Write(ctx, wds); err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Error("influxdb: 배치 쓰기 실패", "error", err)
		return nil, fmt.Errorf("influxdb batch write: %w", err)
	}

	a.stats.IncrExternalMessagesSent()
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

	// v0.16.4: debug 활성화 시 query 요청 로그.
	if a.influxConfig.Debug {
		a.logger.Debug("influxdb: query 전송",
			"language", lang,
			"query", qr.Query,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(a.influxConfig.TimeoutSec)*time.Second)
	defer cancel()

	rows, err := a.client.Query(ctx, qr.Query, lang)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
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
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(result)))
	default:
		a.logger.Warn("influxdb: 결과 버퍼가 가득 찼습니다, 결과를 드롭합니다")
		a.stats.IncrExternalMessagesErrored()
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
// 설정 변경 시 influxConfig를 재파싱하고, 클라이언트를 재생성한다.
func (a *InfluxDBAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("influxdb configure: %w", err)
	}

	ic, err := parseInfluxDBConfig(config)
	if err != nil {
		return fmt.Errorf("influxdb configure: %w", err)
	}

	// 새 클라이언트 생성
	newClient, err := NewInfluxClient(ic)
	if err != nil {
		return fmt.Errorf("influxdb configure: client create: %w", err)
	}

	a.mu.Lock()
	oldClient := a.client
	a.agentConfig = config
	a.influxConfig = ic
	a.client = newClient
	a.mu.Unlock()

	// 이전 클라이언트 닫기 (비동기 — Close 블로킹 방지)
	if oldClient != nil {
		go func() {
			closeDone := make(chan error, 1)
			go func() { closeDone <- oldClient.Close() }()
			select {
			case err := <-closeDone:
				if err != nil {
					a.logger.Warn("influxdb: 이전 클라이언트 close 실패", "error", err)
				}
			case <-time.After(5 * time.Second):
				a.logger.Warn("influxdb: 이전 클라이언트 close 타임아웃 (5s)")
			}
		}()
	}

	a.logger.Info("influxdb: 설정 업데이트 완료",
		"url", ic.URL,
		"org", ic.Org,
		"bucket", ic.Bucket,
	)
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
