package ws

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/observe"
)

// MetricsSource 는 메트릭 수집에 필요한 엔진 인터페이스이다.
// 테스트 시 모킹이 가능하도록 인터페이스로 분리한다.
type MetricsSource interface {
	ListFlows() []engine.FlowStatus
}

// MonitoringBroadcaster 는 시스템 메트릭을 주기적으로 수집하여
// WebSocket Hub 를 통해 연결된 모든 클라이언트에게 브로드캐스트한다.
type MonitoringBroadcaster struct {
	hub      *Hub
	engine   MetricsSource
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	logger   *slog.Logger

	// 이전 틱의 상태 (처리량·에러율 델타 계산용)
	prevMsgCount int64
	prevErrCount int64
	prevTickTime time.Time

	// 로그 스트리밍 (nil 이면 비활성화)
	streams   observe.StreamRouter
	logWriter *wsLogWriter
}

// BroadcasterOption 은 MonitoringBroadcaster 생성 시 설정을 변경하는 함수 옵션이다.
type BroadcasterOption func(*MonitoringBroadcaster)

// WithBroadcastInterval 은 메트릭 브로드캐스트 주기를 설정한다.
// 기본값은 1초이다.
func WithBroadcastInterval(d time.Duration) BroadcasterOption {
	return func(b *MonitoringBroadcaster) {
		if d > 0 {
			b.interval = d
		}
	}
}

// WithStreamRouter 는 로그 스트리밍을 위한 StreamRouter 를 설정한다.
// 설정 시 Start() 에서 wsLogWriter 를 생성하여 전역 라우트에 등록하고,
// Stop() 에서 자동으로 해제한다.
func WithStreamRouter(sr observe.StreamRouter) BroadcasterOption {
	return func(b *MonitoringBroadcaster) {
		b.streams = sr
	}
}

// NewMonitoringBroadcaster 는 새 MonitoringBroadcaster 를 생성한다.
// hub 를 통해 메시지를 브로드캐스트하고, engine 에서 플로우 메트릭을 수집한다.
func NewMonitoringBroadcaster(hub *Hub, engine MetricsSource, logger *slog.Logger, opts ...BroadcasterOption) *MonitoringBroadcaster {
	if logger == nil {
		logger = slog.Default()
	}

	b := &MonitoringBroadcaster{
		hub:      hub,
		engine:   engine,
		interval: 1 * time.Second,
		logger:   logger,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Start 는 백그라운드 고루틴을 시작하여 주기적으로 메트릭을 수집·브로드캐스트한다.
// 전달된 parent context 가 취소되거나 Stop() 이 호출되면 종료된다.
func (b *MonitoringBroadcaster) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	b.cancel = cancel
	b.prevTickTime = time.Now()

	// 초기 카운터 값 설정 (첫 번째 틱의 델타 계산을 위해)
	b.initCounters()

	// 로그 스트리밍 활성화: wsLogWriter 를 생성하여 전역 라우트에 등록한다
	if b.streams != nil {
		b.logWriter = newWsLogWriter(b.hub, slog.LevelDebug)
		b.streams.AddRoute("", b.logWriter)
		b.logger.Info("로그 스트리밍 활성화")
	}

	b.wg.Add(1)
	go b.run(ctx)

	b.logger.Info("모니터링 브로드캐스터 시작", "interval", b.interval)
}

// Stop 은 브로드캐스터를 정상 종료한다.
// 백그라운드 고루틴이 완전히 종료될 때까지 블로킹한다.
func (b *MonitoringBroadcaster) Stop() {
	// 로그 스트리밍 해제
	if b.streams != nil && b.logWriter != nil {
		b.streams.RemoveRoute("", b.logWriter)
		if dropped := b.logWriter.Dropped(); dropped > 0 {
			b.logger.Info("로그 스트리밍 종료", "dropped", dropped)
		}
		b.logWriter = nil
	}

	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.logger.Info("모니터링 브로드캐스터 종료")
}

// initCounters 는 첫 번째 틱의 델타 계산을 위해 초기 카운터 값을 설정한다.
func (b *MonitoringBroadcaster) initCounters() {
	var totalMsg, totalErr int64
	for _, fs := range b.engine.ListFlows() {
		totalMsg += fs.MessageCount
		totalErr += fs.ErrorCount
	}
	b.prevMsgCount = totalMsg
	b.prevErrCount = totalErr
}

// run 은 메트릭 수집·브로드캐스트 루프를 실행한다.
func (b *MonitoringBroadcaster) run(ctx context.Context) {
	defer b.wg.Done()

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.tick()
		}
	}
}

// tick 은 단일 메트릭 수집·브로드캐스트 사이클을 수행한다.
func (b *MonitoringBroadcaster) tick() {
	// 연결된 클라이언트가 없으면 불필요한 작업을 건너뛴다
	if b.hub.ClientCount() == 0 {
		return
	}

	now := time.Now()
	elapsed := now.Sub(b.prevTickTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1.0 // 0으로 나누기 방지
	}

	// CPU 지표: 고루틴 수 기반 부하 추정
	cpu := float64(runtime.NumGoroutine())

	// 메모리 지표: 할당 메모리 / 시스템 메모리 비율 (%)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	var memory float64
	if memStats.Sys > 0 {
		memory = float64(memStats.Alloc) / float64(memStats.Sys) * 100
	}

	// 플로우 메트릭 집계
	var totalMsg, totalErr int64
	for _, fs := range b.engine.ListFlows() {
		totalMsg += fs.MessageCount
		totalErr += fs.ErrorCount
	}

	// 처리량: 초당 메시지 델타
	deltaMsg := totalMsg - b.prevMsgCount
	throughput := float64(deltaMsg) / elapsed

	// 에러율: 메시지 델타 대비 에러 델타 비율 (%)
	deltaErr := totalErr - b.prevErrCount
	var errorRate float64
	if deltaMsg > 0 {
		errorRate = float64(deltaErr) / float64(deltaMsg) * 100
	}

	// 다음 틱을 위한 상태 갱신
	b.prevMsgCount = totalMsg
	b.prevErrCount = totalErr
	b.prevTickTime = now

	// flow.metrics 메시지 브로드캐스트
	payload := map[string]float64{
		"cpu":        cpu,
		"memory":     memory,
		"throughput":  throughput,
		"error_rate": errorRate,
	}

	if err := b.hub.BroadcastMessage(TypeFlowMetrics, payload); err != nil {
		b.logger.Warn("메트릭 브로드캐스트 실패", "error", err)
	}
}
