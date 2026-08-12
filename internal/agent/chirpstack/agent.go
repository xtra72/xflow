// Package chirpstack 는 ChirpStack LoRaWAN Network Server 가 발행하는 MQTT 업링크를
// 수신하여 measurement 당 1개 메시지로 fan-out 하는 에이전트를 제공한다
// (SPEC-CHIRPSTACK-001), 그리고 LoRaWAN 다운링크(제어) 발행 경로를 제공한다
// (SPEC-CHIRPSTACK-002).
//
// SPEC-CHIRPSTACK-001 은 본 에이전트를 "수신 전용"으로 정의하며 발행 경로를 의도적으로
// 배제했다. SPEC-CHIRPSTACK-002 REQ-M1-02 가 그 배제를 명시적으로 역전한다 — 수신 계약
// (REQ-FROZEN-A)은 그대로 보존되며, 발행은 추가된 능력이다(회귀 아님).
//
// 2계층 설계:
//   - 에이전트(본 패키지): MQTT 연결/구독/수신, 업링크 디코드, per-measurement
//     레코드 생성, 디바이스 자동 생성/메타데이터 노출, 다운링크 발행 프리미티브
//     (publish.go) + deviceProfile 별 다운링크 코덱(codec.go).
//   - 노드(internal/node/chirpstack.go): 수신 전용 SourceNode. 에이전트가 emit 한
//     per-measurement 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격.
//   - 노드(internal/node/chirpstack_control.go): typed command 를 코덱으로 인코딩해
//     다운링크 토픽으로 발행하는 제어 노드.
package chirpstack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ChirpStackAgent 는 ChirpStack LoRaWAN 업링크를 수신하고, 다운링크(제어) 명령을
// 발행하는 에이전트이다.
//
// SPEC-CHIRPSTACK-001 은 "발행 경로(MessagePublisher)는 구현하지 않는다" 로 발행을
// 의도적으로 배제했으나, SPEC-CHIRPSTACK-002 REQ-M1-02 가 이 배제를 명시적으로
// 역전하여 PublishMessage(publish.go)를 구현한다. 수신 계약은 변경되지 않는다.
type ChirpStackAgent struct {
	*lifecycle.BaseLifecycle

	agentConfig agent.AgentConfig

	// csConfig 는 파싱된 ChirpStack 설정 스냅샷이다. Configure 가 런타임에 통째로
	// 교체하고 수신 콜백 / watchdog goroutine 이 락 없이 읽으므로 atomic.Pointer 로
	// 보관한다. a.mu 와 무관한 경로이므로 REQ-FROZEN-B(락 보유 중 재-lock 금지)에
	// 영향을 주지 않는다. 읽기는 반드시 cs() 접근자를 사용한다.
	csConfig atomic.Pointer[ChirpStackConfig]

	client   mqtt.Client
	recvCh   chan []byte
	done     chan struct{}
	doneOnce sync.Once

	// stopped 는 Stop() 이 호출되었음을 나타내는 가드 플래그이다 (mqtt_agent.go 이식).
	// Paho 의 백그라운드 재연결 goroutine 이 Stop 이후 재연결에 성공해도
	// subscribe() 가 stopped 상태이면 즉시 Disconnect 하여 세션 부활을 막는다.
	stopped atomic.Bool

	subscribedTopics []string
	topicsMu         sync.RWMutex

	// devices 는 devEui 키 자동 생성 디바이스 로스터이다 (M4, REQ-M4-01).
	devices   map[string]*deviceState
	devicesMu sync.RWMutex

	// comm 은 devEui 키 comm-state 추적 맵이다 (M5, REQ-M5-02/03). 로스터(devices)와
	// 분리해 comm-state watchdog 이 online/last-seen 을 독립적으로 관리한다.
	comm   map[string]*commEntry
	commMu sync.Mutex

	// watchdog goroutine 수명 제어 (REQ-M6-03: context 취소로 종료 보장).
	wdCancel  context.CancelFunc
	wdWg      sync.WaitGroup
	wdStarted bool // a.mu 보호

	stats  *agent.AgentStats
	logger *slog.Logger

	mu        sync.RWMutex
	startedAt time.Time
	createdAt time.Time
}

// 컴파일 타임 인터페이스 체크.
//
// MessagePublisher 는 SPEC-CHIRPSTACK-002 REQ-M1-02 로 추가되었다 — SPEC-CHIRPSTACK-001
// 의 발행 경로 배제를 역전한 기계적 신호이며, 회귀가 아니다(publish.go 구현).
var (
	_ agent.Agent            = (*ChirpStackAgent)(nil)
	_ agent.MessageReceiver  = (*ChirpStackAgent)(nil)
	_ agent.TransportChecker = (*ChirpStackAgent)(nil)
	_ agent.StatefulAgent    = (*ChirpStackAgent)(nil)
	_ agent.MessagePublisher = (*ChirpStackAgent)(nil)
)

// NewChirpStackAgent 는 ChirpStackAgent 팩토리 함수이다.
//
// 에이전트 이름은 배포 내에서 고유해야 한다(REQ-M1-04): device_id 가
// (agentName, devEui) 로 키잉되므로, 이름 충돌은 조용히 덮어쓰지 않고
// ErrNameCollision 으로 거부한다.
func NewChirpStackAgent(config agent.AgentConfig) (agent.Agent, error) {
	// measurement_emit_mode 는 생성 경로에서도 무효값을 거부한다 — 오타가 조용히
	// per_measurement 로 폴백하면 "combined 를 켰는데 왜 그대로지" 를 진단할 단서가
	// 사라진다. 기존(동결) 노브의 관용적 파싱 동작은 그대로 보존한다.
	if err := validateMeasurementEmitMode(config.Transport.Options); err != nil {
		return nil, err
	}

	if err := claimAgentName(config.Name, config.ID); err != nil {
		return nil, err
	}

	cc := parseChirpStackConfig(config)

	a := &ChirpStackAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("chirpstack")),
		recvCh:        make(chan []byte, cc.BufferSize),
		done:          make(chan struct{}),
		devices:       make(map[string]*deviceState),
		comm:          make(map[string]*commEntry),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(config),
		createdAt:     time.Now(),
	}
	a.csConfig.Store(&cc)

	if err := a.Init(config); err != nil {
		releaseAgentName(config.Name, config.ID)
		return nil, err
	}
	return a, nil
}

// cs 는 현재 ChirpStack 설정 스냅샷을 반환한다.
//
// 반환된 포인터가 가리키는 값은 절대 변경하지 않는다 — Configure 는 새 값을 통째로
// Store 하여 교체한다(copy-on-write). 락을 잡지 않으므로 어떤 락 보유 구간에서도
// 안전하게 호출할 수 있다.
func (a *ChirpStackAgent) cs() *ChirpStackConfig {
	return a.csConfig.Load()
}

// Init 은 에이전트를 초기화하고, 활성화 상태이면 MQTT 브로커에 연결/구독한다.
func (a *ChirpStackAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}

	// 재시작 경로에서 stopped 가드를 해제한다 (mqtt_agent.go 관례).
	a.stopped.Store(false)
	// 재시작 경로에서 Stop 이 close 한 done 채널을 새로 만든다 (Stop→Start hot-spin 방지).
	a.resetDoneIfClosed()

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	// 비활성화 게이트: disabled 에이전트도 List API 노출을 위해 생성되나 연결하지
	// 않는다. lifecycle 은 StateCreated 에 머문다.
	if !config.IsEnabled() {
		cc := a.cs()
		a.logger.Info("chirpstack: 비활성화 상태로 생성됨 — 연결 건너뜀",
			"broker", cc.Broker,
			"client_id", cc.ClientID,
		)
		return nil
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}
	if err := a.connect(); err != nil {
		return err
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("chirpstack init: %w", err)
	}

	// comm-state watchdog 시작 (활성화 + emit_comm_state 시에만) (M5, REQ-M5-03/04).
	a.startCommWatchdog()

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()
	return nil
}

// connect 는 MQTT 클라이언트를 구성하고 브로커에 연결한다 (mqtt_agent.go Init 이식).
// AutoReconnect 시 초기 연결 실패를 치명적으로 보지 않고 degraded 로 진행한다
// (죽은 브로커가 플로우 시작을 막지 않게 함).
func (a *ChirpStackAgent) connect() error {
	// 클라이언트 구성 전체가 하나의 일관된 설정 세대를 보도록 1회 스냅샷한다.
	cc := a.cs()

	opts := mqtt.NewClientOptions().
		AddBroker(cc.Broker).
		SetClientID(cc.ClientID).
		SetKeepAlive(time.Duration(cc.KeepAliveSec) * time.Second).
		SetAutoReconnect(cc.AutoReconnect).
		SetCleanSession(cc.CleanSession).
		SetConnectTimeout(time.Duration(cc.ConnectTimeoutSec) * time.Second).
		SetOrderMatters(false)

	if cc.AutoReconnect {
		opts.SetConnectRetry(true)
		opts.SetConnectRetryInterval(time.Duration(cc.ConnectTimeoutSec) * time.Second)
	}
	if cc.Username != "" {
		opts.SetUsername(cc.Username)
	}
	if cc.Password != "" {
		opts.SetPassword(cc.Password)
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		a.logger.Info("chirpstack: 브로커에 연결됨",
			"broker", cc.Broker,
			"client_id", cc.ClientID,
		)
		a.subscribe(c)
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		a.logger.Warn("chirpstack: 연결 끊김", "broker", cc.Broker, "error", err)
	})
	opts.SetDefaultPublishHandler(a.messageHandler)

	// a.client 는 노드 goroutine 의 발행 경로(PublishMessage)와 TransportConnected 가
	// a.mu.RLock 으로 읽으므로, 대입도 a.mu 하에서 수행한다 (SPEC-CHIRPSTACK-002:
	// 발행 경로 추가로 넓어진 데이터 레이스 차단 — 동작 보존 최소 수정).
	client := mqtt.NewClient(opts)
	a.mu.Lock()
	a.client = client
	a.mu.Unlock()

	token := client.Connect()
	connected := token.WaitTimeout(time.Duration(cc.ConnectTimeoutSec)*time.Second) && token.Error() == nil
	if !connected {
		if cc.AutoReconnect {
			a.logger.Warn("chirpstack: 초기 연결 실패 — 백그라운드 재연결로 진행(degraded)",
				"broker", cc.Broker, "error", token.Error(),
			)
			return nil
		}
		_ = a.TransitionTo(lifecycle.StateError)
		if token.Error() != nil {
			return fmt.Errorf("chirpstack init: 연결 실패: %w", token.Error())
		}
		return fmt.Errorf("chirpstack init: 연결 타임아웃 (%s)", cc.Broker)
	}
	return nil
}

// subscribe 는 설정 토픽을 구독한다. stopped 상태이면 즉시 Disconnect 하여
// Stop 이후 재연결에 의한 세션 부활을 막는다 (mqtt_agent.go stopped-guard 이식).
func (a *ChirpStackAgent) subscribe(c mqtt.Client) {
	if a.stopped.Load() {
		a.logger.Info("chirpstack: Stop 이후 재연결 감지 — 재구독 없이 즉시 종료(세션 부활 방지)")
		c.Disconnect(0)
		return
	}

	cc := a.cs()

	a.topicsMu.Lock()
	if len(a.subscribedTopics) == 0 && len(cc.Topics) > 0 {
		a.subscribedTopics = make([]string, len(cc.Topics))
		copy(a.subscribedTopics, cc.Topics)
	}
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.Unlock()

	for _, topic := range topics {
		token := c.Subscribe(topic, cc.QoS, nil)
		token.Wait()
		if token.Error() != nil {
			a.logger.Error("chirpstack: 토픽 구독 실패", "topic", topic, "error", token.Error())
		} else {
			a.logger.Info("chirpstack: 토픽 구독 완료", "topic", topic, "qos", cc.QoS)
		}
	}
}

// messageHandler 는 MQTT 업링크 수신 콜백이다. 업링크를 디코드/fan-out 한다.
func (a *ChirpStackAgent) messageHandler(_ mqtt.Client, msg mqtt.Message) {
	a.handleUplink(msg.Payload(), msg.Topic())
}

// handleUplink 는 원시 업링크를 디코드하여 measurement 당 1개 레코드로 fan-out 하고
// 각 레코드(JSON)를 수신 채널에 넣는다 (REQ-M3-01/02/05).
//
// 노드는 이 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격한다.
func (a *ChirpStackAgent) handleUplink(raw []byte, topic string) {
	up, err := decodeUplink(raw)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: 업링크 디코드 실패", "topic", topic, "error", err)
		return
	}

	// 디바이스 자동 생성/갱신 + UID 발급 + 런타임 info 등록 (M4, REQ-M4-01/02/04).
	a.upsertDevice(up)

	// comm-state fold: 활성화 시 last-seen 갱신 + online 전이 change emit (M5, REQ-M5-02).
	// 매 업링크마다 현재 스냅샷을 읽으므로 Configure 의 emit_comm_state 토글이 이 게이트에는
	// 즉시 반영된다 (watchdog 기동/정지는 재시작 필요 — Configure 주석 참조).
	if a.cs().EmitCommState {
		a.onUplinkCommState(up)
	}

	// measurement_emit_mode 스냅샷을 매 업링크마다 읽으므로 Configure 의 모드 전환이
	// 즉시 반영된다 (emit_comm_state 게이트와 동일한 규약).
	if a.cs().MeasurementEmitMode == measurementEmitModeCombined {
		a.emitCombinedRecord(up, topic)
		return
	}

	records := buildMeasurementRecords(up, a.logger)
	for i := range records {
		b, err := json.Marshal(records[i])
		if err != nil {
			a.stats.IncrExternalMessagesErrored()
			a.logger.Warn("chirpstack: 레코드 직렬화 실패", "measurement", records[i].Measurement, "error", err)
			continue
		}
		a.enqueue(b, topic)
	}
}

// emitCombinedRecord 는 업링크 1건을 combined 레코드 1개로 접어 수신 채널에 넣는다
// (measurement_emit_mode="combined").
//
// 스칼라 measurement 가 없으면 아무것도 방출하지 않는다(빈 payload 메시지 금지).
func (a *ChirpStackAgent) emitCombinedRecord(up *uplink, topic string) {
	rec, ok := buildCombinedMeasurementRecord(up, a.logger)
	if !ok {
		return
	}
	b, err := json.Marshal(rec)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: combined 레코드 직렬화 실패", "devEui", rec.UnitID, "error", err)
		return
	}
	a.enqueue(b, topic)
}

// enqueue 는 바이트를 수신 채널에 넣고 통계를 갱신한다. 버퍼가 가득 차면 드롭한다.
func (a *ChirpStackAgent) enqueue(data []byte, topic string) {
	select {
	case a.recvCh <- data:
		a.stats.IncrExternalMessagesReceived()
		a.stats.AddBytesRead(int64(len(data)))
		a.stats.UpdateLastActivity()
	default:
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: 버퍼 가득 참, 메시지 드롭", "topic", topic)
	}
}

// ReceiveMessage 는 수신 채널에서 메시지를 가져온다 (agent.MessageReceiver).
//
// done 은 재시작 경로(Init)에서 교체되므로 select 전에 a.mu 하에서 1회 캡처한다.
// 진행 중인 수신은 자신이 캡처한 세대의 done 만 관찰한다.
func (a *ChirpStackAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.mu.RLock()
	done := a.done
	a.mu.RUnlock()

	select {
	case data := <-a.recvCh:
		return data, nil
	case <-done:
		return nil, fmt.Errorf("chirpstack: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// resetDoneIfClosed 는 이미 close 된 done 채널을 새 채널로 교체한다 (Init 재시작 경로).
//
// Stop 이 close 한 done 을 그대로 두고 Start 하면 ReceiveMessage 의 `case <-done`
// 이 영구히 ready 상태가 되어, recvCh 가 잠시라도 비어 있을 때마다 즉시 에러를
// 반환한다. 노드 수신 루프는 그 에러로 블로킹 없이 재시도하며 CPU 코어 하나를
// 태운다(hot-spin).
//
// "이미 close 된 경우에만" 교체하는 것이 핵심이다 — 열려 있는 done 을 교체하면
// 그 채널에서 대기 중인 in-flight ReceiveMessage 가 깨어날 수단을 잃는다. 또한
// Stop 이 아닌 경로에서는 no-op 이므로, 의도적으로 정지된 에이전트를 되살리지도
// 않는다(부활은 오직 Start→Init 경로에서만 일어난다).
func (a *ChirpStackAgent) resetDoneIfClosed() {
	a.mu.Lock()
	defer a.mu.Unlock()

	select {
	case <-a.done:
		a.done = make(chan struct{})
		a.doneOnce = sync.Once{}
	default:
		// 아직 열려 있음 — 진행 중인 수신을 보존한다.
	}
}

// Start 는 이미 Running 이면 no-op, Stopped/Created 이면 재-Init 하여 재연결한다.
func (a *ChirpStackAgent) Start(_ context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		return nil
	case lifecycle.StateCreated:
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("chirpstack start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("chirpstack start: not in running state (current: %s)", a.CurrentState())
	}
}

// Stop 은 MQTT 구독을 해제하고 연결을 종료한다. idempotent 하며, stopped 가드로
// Stop 이후 Paho 재연결에 의한 세션 부활을 막는다.
func (a *ChirpStackAgent) Stop(_ context.Context) error {
	a.stopped.Store(true)

	// comm-state watchdog goroutine 종료 (context 취소 + 대기) (M5, REQ-M6-03).
	a.stopCommWatchdog()

	// Created(비활성화 생성) / Stopped 는 Stopping 전이가 invalid 하므로 건너뛴다.
	// 그래도 done close / 버퍼 드레인 / 이름 해제는 항상 수행한다.
	state := a.CurrentState()
	transition := state != lifecycle.StateStopped && state != lifecycle.StateCreated
	if transition {
		if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
			return fmt.Errorf("chirpstack stop: %w", err)
		}
	}

	if a.client != nil {
		if a.client.IsConnected() {
			for _, topic := range a.cs().Topics {
				token := a.client.Unsubscribe(topic)
				token.Wait()
			}
		}
		a.client.Disconnect(250)
	}

	// done / doneOnce 는 재시작 경로(resetDoneIfClosed)가 교체하므로 a.mu 하에서 다룬다.
	a.mu.Lock()
	a.doneOnce.Do(func() { close(a.done) })
	a.mu.Unlock()

	// 버퍼 드레인.
	for {
		select {
		case <-a.recvCh:
		default:
			if transition {
				if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
					return fmt.Errorf("chirpstack stop: %w", err)
				}
			}
			a.mu.RLock()
			name, id := a.agentConfig.Name, a.agentConfig.ID
			a.mu.RUnlock()
			releaseAgentName(name, id)
			return nil
		}
	}
}

// Pause 는 Running -> Paused 전이한다.
func (a *ChirpStackAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 Paused -> Running 전이한다.
func (a *ChirpStackAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Process 는 사용하지 않는다. 다운링크 발행은 PublishMessage 인터페이스를 사용한다
// (SPEC-CHIRPSTACK-002 REQ-M1-02 — SPEC-CHIRPSTACK-001 의 "발행 경로 없음" 배제 역전).
func (a *ChirpStackAgent) Process(_ []byte) ([]byte, error) {
	return nil, nil
}

// Configure 는 에이전트 설정을 업데이트한다.
//
// agentConfig 뿐 아니라 파싱된 csConfig 도 함께 교체한다 — 이전에는 csConfig 가 생성
// 시점에 1회만 파싱되어, UI 에서 저장한 ChirpStack 노브 변경이 조용히 무시되었다.
//
// 런타임 반영 범위(의도적 한계, 재시작 필요):
//   - emit_comm_state 의 per-uplink fold 게이트는 즉시 반영된다(handleUplink 가 매
//     업링크마다 현재 스냅샷을 읽는다).
//   - comm-state watchdog(watchLoop / reportLoop)은 Init 에서만 기동되므로 런타임
//     토글로 기동/정지되지 않는다. 따라서 false→true 토글 시 staleness 기반 offline
//     판정과 주기 report 는 에이전트를 재시작해야 동작한다.
//   - broker / topics / qos 등 트랜스포트 노브는 이미 열린 MQTT 연결에 반영되지
//     않는다. 저장만 되고 다음 연결에서 적용된다.
//
// 위 한계에 해당하는 변경은 경고 로그로 사용자에게 알린다(조용한 무시 금지).
func (a *ChirpStackAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("chirpstack configure: %w", err)
	}

	// 파싱 실패 시 이전 csConfig 를 그대로 유지하고 에러를 반환한다 — 절반만 갱신된
	// 상태(agentConfig 는 새 값 / csConfig 는 옛 값)를 만들지 않는다.
	cc, err := parseChirpStackConfigStrict(config)
	if err != nil {
		return fmt.Errorf("chirpstack configure: %w", err)
	}

	prev := a.cs()

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	a.csConfig.Store(&cc)

	// REQ-FROZEN-B: a.mu 를 보유하지 않은 상태에서 로깅한다.
	a.logRestartRequiredChanges(prev, &cc)
	return nil
}

// logRestartRequiredChanges 는 저장은 되었으나 런타임에 즉시 반영되지 않는 설정
// 변경을 경고로 알린다 (Configure 주석의 "런타임 반영 범위" 참조).
func (a *ChirpStackAgent) logRestartRequiredChanges(prev, next *ChirpStackConfig) {
	if prev == nil || a.logger == nil {
		return
	}

	if prev.EmitCommState != next.EmitCommState {
		a.logger.Warn("chirpstack: emit_comm_state 변경 저장됨 — 업링크 fold 는 즉시 반영되나 "+
			"staleness watchdog / 주기 report 는 에이전트 재시작 후 반영됩니다",
			"from", prev.EmitCommState, "to", next.EmitCommState)
	}
	if prev.Broker != next.Broker || prev.ClientID != next.ClientID || !equalStringSlice(prev.Topics, next.Topics) {
		a.logger.Warn("chirpstack: 트랜스포트 설정 변경 저장됨 — 기존 MQTT 연결에는 반영되지 않으며 "+
			"에이전트 재시작이 필요합니다",
			"broker", next.Broker, "client_id", next.ClientID, "topics", next.Topics)
	}
}

// equalStringSlice 는 두 문자열 슬라이스가 순서까지 동일한지 비교한다.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TransportConnected 는 실제 Paho 클라이언트의 연결 여부를 반환한다
// (agent.TransportChecker).
func (a *ChirpStackAgent) TransportConnected() bool {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	return client != nil && client.IsConnected()
}

// ID 는 에이전트 ID 를 반환한다.
func (a *ChirpStackAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트 이름을 반환한다.
func (a *ChirpStackAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트 타입을 반환한다.
func (a *ChirpStackAgent) Type() string {
	return "chirpstack"
}

// Health 는 에이전트의 건강 상태를 반환한다.
func (a *ChirpStackAgent) Health() agent.HealthStatus {
	now := time.Now()
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		if a.TransportConnected() {
			return agent.HealthStatus{Status: agent.HealthHealthy, LastCheck: now, Message: "chirpstack is running and connected"}
		}
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "chirpstack is running but disconnected (reconnecting)"}
	case lifecycle.StatePaused:
		return agent.HealthStatus{Status: agent.HealthDegraded, LastCheck: now, Message: "chirpstack is paused"}
	default:
		return agent.HealthStatus{Status: agent.HealthUnhealthy, LastCheck: now, Message: fmt.Sprintf("chirpstack is in %s state", a.CurrentState())}
	}
}

// Info 는 에이전트 정보 스냅샷을 반환한다.
func (a *ChirpStackAgent) Info() agent.AgentInfo {
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
		Type:      "chirpstack",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// Stats 는 통계 스냅샷을 반환한다.
func (a *ChirpStackAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// State 는 타입별 런타임 상태를 반환한다 (agent.StatefulAgent).
func (a *ChirpStackAgent) State() map[string]any {
	cc := a.cs()

	a.topicsMu.RLock()
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.RUnlock()
	if len(topics) == 0 {
		topics = append(topics, cc.Topics...)
	}

	return map[string]any{
		"broker":    cc.Broker,
		"client_id": cc.ClientID,
		"connected": a.TransportConnected(),
		"qos":       cc.QoS,
		"topics":    topics,
	}
}
