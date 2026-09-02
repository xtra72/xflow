package system

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
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
	doneOnce     sync.Once
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
//
// SPEC-DEVICE-IDENTITY-001 Phase D § D-T17: NewInfluxDBAgentWithOptions /
// WithDeviceResolver / InfluxDBAgentOption 가 제거되어 단일 팩토리로 단순화됨.
// dual-tag 부착 기능이 사라졌으므로 옵션 주입 경로가 더 이상 필요하지 않다.
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
		logger:        agent.ResolveLogger(config),
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

// --- 구조화 시리즈 질의 (@spec SPEC-TSDB-002 §2.6 (U6) · §2.8 (U8)) ---

// SeriesBucket 는 구조화 시리즈 질의가 돌려주는 정규화된 버킷 하나이다.
//
// StartMs 는 버킷 **시작** 시각(epoch ms)이며 끝이 아니다(§2.8). 정렬 계약의
// 정본은 Store 의 epoch-zero 식이고 두 소스가 같은 식을 쓴다.
type SeriesBucket struct {
	StartMs int64
	Value   any
	// Tags 는 이 버킷이 속한 **그룹의 실제 태그 값**이다
	// (SPEC-TSDB-004 §2.5 U5).
	//
	// SeriesQuerySpec.GroupBy 가 비어 있으면 nil 이다 — 정확 일치 모드에서
	// 태그는 요청이 이미 알고 있으므로 응답에 실을 이유가 없고, nil 이어야
	// 라벨 구성이 본 축 도입 이전과 같아진다(§2.9 U9).
	Tags map[string]string
}

// InfluxSeriesQueryer 는 구조화 시리즈 질의 계약이다.
// HTTP 핸들러가 타입 단언으로 에이전트를 검증한다.
type InfluxSeriesQueryer interface {
	QuerySeriesBuckets(ctx context.Context, spec SeriesQuerySpec) ([]SeriesBucket, error)
}

// 컴파일 타임 인터페이스 준수 체크.
var _ InfluxSeriesQueryer = (*InfluxDBAgent)(nil)

// QuerySeriesBuckets 는 구조화 시리즈 질의를 실행하고 버킷 배열을 반환한다.
//
// 흐름은 셋이다 — (1) 에이전트 버전으로 방언을 고르고, (2) 순수 함수로 쿼리를
// 생성하고, (3) 결과 행을 버킷으로 정규화한다. 생성이 순수 함수로 분리되어
// 있으므로 (v2/v3) × (집계 5) × (fill 5) × (태그 0/1/N) 전수는 네트워크 없이
// 검증된다.
func (a *InfluxDBAgent) QuerySeriesBuckets(ctx context.Context, spec SeriesQuerySpec) ([]SeriesBucket, error) {
	a.mu.RLock()
	version := a.influxConfig.Version
	defaultBucket := a.influxConfig.Bucket
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, errClientNotInitialized
	}
	if spec.Bucket == "" {
		spec.Bucket = defaultBucket
	}

	query, lang, err := buildSeriesQueryForVersion(version, spec)
	if err != nil {
		return nil, err
	}

	rows, err := client.Query(ctx, query, lang)
	if err != nil {
		return nil, fmt.Errorf("influxdb %s series query: %w", lang, err)
	}
	buckets, err := normalizeSeriesBuckets(rows, spec)
	if err != nil {
		return nil, err
	}
	// 직전값 채우기는 응답을 받은 뒤 여기서 한다(사용 기간 제한을 걸기 위해).
	// 그 밖의 전략은 DB 가 이미 처리했으므로 손대지 않는다.
	if spec.Fill == SeriesFillPrevious {
		buckets = applyPreviousFill(
			buckets, sortedGroupKeys(spec.GroupBy), spec.IntervalMs, spec.FillPreviousLimit)
	}
	return buckets, nil
}

// buildSeriesQueryForVersion 는 에이전트의 InfluxDB 버전에 따라 방언을 고른다.
//
// v3 가 InfluxQL 인 것은 선택이 아니다 — v3 는 Flux 를 지원하지 않고 SQL 은
// HTTP 계층에서 도달 불가다(§1.2.9).
func buildSeriesQueryForVersion(version string, spec SeriesQuerySpec) (string, string, error) {
	switch version {
	case "2":
		q, err := BuildFluxSeriesQuery(spec)
		return q, "flux", err
	case "3":
		q, err := BuildInfluxQLSeriesQuery(spec)
		return q, "influxql", err
	default:
		return "", "", fmt.Errorf("influxdb: 지원하지 않는 버전 %q ('2' 또는 '3'을 지정하세요)", version)
	}
}

// influxQLTimeColumn 은 InfluxQL 결과의 시간 컬럼 이름이다.
// Flux 결과는 fluxTimeColumn(_time) 을 쓴다.
const influxQLTimeColumn = "time"

// normalizeSeriesBuckets 는 결과 행을 버킷 시작 시각 오름차순 배열로 정규화한다.
//
// 시간 컬럼과 값 컬럼의 이름이 방언마다 다르다. Flux 는 _time/_value 이고,
// InfluxQL 은 time 과 **집계 함수 이름**(MEAN → mean)이다. 두 방언의 값 컬럼
// 이름이 각각 Flux 함수 이름과 일치하므로 같은 매핑 함수를 쓴다.
func normalizeSeriesBuckets(rows []map[string]any, spec SeriesQuerySpec) ([]SeriesBucket, error) {
	valueColumn, err := fluxAggregationFn(spec.Aggregation)
	if err != nil {
		return nil, err
	}

	// 그룹 키는 방언과 무관하게 결과 **행의 컬럼**으로 온다. v2 는 keep 이
	// 남긴 태그 컬럼, v3 는 GROUP BY 가 드러낸 태그 컬럼이며, 이 함수는 둘을
	// 구분하지 않는다(SPEC-TSDB-004 §2.5).
	groupKeys := sortedGroupKeys(spec.GroupBy)

	out := make([]SeriesBucket, 0, len(rows))
	for _, row := range rows {
		tsMs, ok := seriesRowTimeMs(row)
		if !ok {
			// 시간을 읽지 못한 행은 버킷에 배치할 수 없다. 0 으로 두면 1970 년
			// 버킷이 생겨 차트의 시간축이 통째로 늘어난다.
			continue
		}
		out = append(out, SeriesBucket{
			StartMs: SeriesBucketStartMs(tsMs, spec.IntervalMs),
			Value:   seriesRowValue(row, valueColumn),
			Tags:    seriesRowGroupTags(row, groupKeys),
		})
	}
	// 그룹 태그 값 사전순 → 버킷 시작 시각 순으로 정렬한다(§2.8 UB1-8).
	//
	// 그룹 순서가 폴링마다 달라지면 클라이언트의 그룹 등장 순서가 흔들리고
	// 자동 팔레트 색이 라인 사이를 옮겨 다닌다. 사용자는 같은 색을 같은 대상으로
	// 읽으므로 이는 조용한 오답이다. groupKeys 가 비면 서명이 전부 빈 문자열이라
	// 시작 시각 단일 기준으로 되돌아간다 — 본 축 도입 이전과 같다.
	sort.SliceStable(out, func(i, j int) bool {
		si := seriesGroupSignature(out[i].Tags, groupKeys)
		sj := seriesGroupSignature(out[j].Tags, groupKeys)
		if si != sj {
			return si < sj
		}
		return out[i].StartMs < out[j].StartMs
	})
	return out, nil
}

// seriesRowGroupTags 는 결과 행에서 그룹 키에 해당하는 값을 뽑는다.
//
// groupKeys 가 비면 nil 을 돌려준다(§2.5). 행에 그 키가 없거나 값이 문자열이
// 아니면 **빈 문자열**로 둔다 — 태그가 결손된 시리즈를 조용히 버리면 데이터가
// 사라지고, 별도 그룹으로 두면 사용자가 결손 사실을 볼 수 있다.
func seriesRowGroupTags(row map[string]any, groupKeys []string) map[string]string {
	if len(groupKeys) == 0 {
		return nil
	}
	tags := make(map[string]string, len(groupKeys))
	for _, k := range groupKeys {
		tags[k] = seriesTagValueString(row[k])
	}
	return tags
}

// seriesTagValueString 은 결과 행의 태그 값을 문자열로 만든다.
// 태그는 규약상 문자열이지만 클라이언트가 다른 타입으로 넘길 수 있으므로
// 문자열이 아니면 표기로 대체한다(nil 은 빈 문자열).
func seriesTagValueString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// seriesGroupSignature 는 그룹 정렬용 결정적 서명을 만든다.
//
// 구분자는 NUL 이다 — 태그 값에 등장할 수 없으므로 "a|b" 와 "a" + "|b" 가
// 같은 서명이 되는 충돌을 막는다.
func seriesGroupSignature(tags map[string]string, groupKeys []string) string {
	if len(groupKeys) == 0 {
		return ""
	}
	var b strings.Builder
	for i, k := range groupKeys {
		if i > 0 {
			b.WriteByte(0)
		}
		b.WriteString(tags[k])
	}
	return b.String()
}

// seriesRowTimeMs 는 결과 행에서 시각을 epoch ms 로 뽑는다.
//
// 정수형은 InfluxDB 규약대로 나노초로 해석한다. 문자열은 RFC3339Nano 로 읽는다.
func seriesRowTimeMs(row map[string]any) (int64, bool) {
	raw, ok := row[fluxTimeColumn]
	if !ok {
		raw, ok = row[influxQLTimeColumn]
	}
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case time.Time:
		if v.IsZero() {
			return 0, false
		}
		return v.UnixMilli(), true
	case int64:
		return v / nsPerMs, true
	case int:
		return int64(v) / nsPerMs, true
	case uint64:
		return int64(v) / nsPerMs, true
	case float64:
		return int64(v) / nsPerMs, true
	case string:
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return 0, false
		}
		return t.UnixMilli(), true
	default:
		return 0, false
	}
}

// seriesRowValue 는 결과 행에서 집계값을 뽑는다.
//
// 키가 있으나 값이 nil 인 경우(fill 로 만들어진 빈 버킷)를 키 부재와 구분해야
// 하므로 comma-ok 로 조회한다. nil 값은 그대로 통과시켜 클라이언트가 "빈 결과"로
// 표시하게 한다.
func seriesRowValue(row map[string]any, aggColumn string) any {
	if v, ok := row[fluxValueColumn]; ok {
		return v
	}
	if v, ok := row[aggColumn]; ok {
		return v
	}
	return nil
}
