// Package chirpstack 는 ChirpStack LoRaWAN Network Server 가 발행하는 MQTT 업링크를
// 수신하여 measurement 당 1개 메시지로 fan-out 하는 에이전트를 제공한다
// (SPEC-CHIRPSTACK-001), 그리고 LoRaWAN 다운링크(제어) 발행 경로를 제공한다
// (SPEC-CHIRPSTACK-002).
//
// SPEC-CHIRPSTACK-001 은 본 에이전트를 "수신 전용"으로 정의하며 발행 경로를 의도적으로
// 배제했다. SPEC-CHIRPSTACK-002 REQ-M1-02 가 그 배제를 명시적으로 역전한다 — 수신 계약
// (REQ-FROZEN-A)은 그대로 보존되며, 발행은 추가된 능력이다(회귀 아님).
//
// SPEC-CHIRPSTACK-003 REQ-M3-01 은 같은 규율로 **Process 표면의 배제를 역전**한다:
// SPEC-001/002 가 "Process 는 사용하지 않는다" 로 비워 두었던 자리에 조회 전용
// exec 커맨드 디스패처(list_gateways)를 둔다. 다운링크 발행은 여전히 PublishMessage
// 이며 Process 는 발행하지 않는다 — 회귀가 아니라 추가된 조회 능력이다(Process 주석 참조).
//
// 2계층 설계:
//   - 에이전트(본 패키지): MQTT 연결/구독/수신, 업링크 디코드, per-measurement
//     레코드 생성, 디바이스 자동 생성/메타데이터 노출, (device, gateway) 링크 캐시 +
//     파생 게이트웨이 로스터(gateways.go), 다운링크 발행 프리미티브
//     (publish.go) + deviceProfile 별 다운링크 코덱(codec.go).
//   - 노드(internal/node/chirpstack.go): 수신 전용 SourceNode. 에이전트가 emit 한
//     per-measurement 레코드를 소비해 flow message 로 빌드하고 device 그룹을 승격.
//   - 노드(internal/node/chirpstack_control.go): typed command 를 코덱으로 인코딩해
//     다운링크 토픽으로 발행하는 제어 노드.
package chirpstack

import (
	"context"
	"encoding/json"
	"errors"
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

	// reports 는 devEui 키 **주기 집계 리포트** 누적 맵이다 (report.go).
	//
	// comm 맵과도, 로스터(devices)와도 분리한다 — comm 맵은 "최신 스냅샷 1건" 이라
	// 집계를 담을 수 없고, 로스터에 얹으면 emit_report 가 꺼진 배포에서도 devicesMu
	// 보유 구간이 길어진다. 별도 맵 + 별도 mutex 가 두 기존 표면을 전혀 건드리지 않는
	// 유일한 배치이다.
	//
	// 락 규율(REQ-FROZEN-B): reportsMu 는 devicesMu / commMu / a.mu 와 **절대
	// 중첩하지 않는다**. 이 맵을 만지는 두 경로(onUplinkReport /
	// emitMeasurementReports)는 나머지 락을 하나도 잡지 않으므로 락 순서 엣지 자체가
	// 생기지 않는다.
	reports   map[string]*deviceWindow
	reportsMu sync.Mutex
	// reportWindowStartMs 는 현재 집계 윈도의 시작 시각이다 (reportsMu 보호).
	reportWindowStartMs int64

	// 집계 리포트 goroutine 수명 제어. comm watchdog(wd*)과 별도인 이유는 두 기능의
	// 수명이 독립이기 때문이다 — 한쪽만 켠 배포에서 다른 쪽 goroutine 이 생기면 안 된다.
	mrCancel  context.CancelFunc
	mrWg      sync.WaitGroup
	mrStarted bool // a.mu 보호

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
//
// BufferInfoProvider 는 선택 인터페이스이므로 구현이 사라져도 컴파일은 통과한다 —
// 대신 통계에서 버퍼 사용량이 조용히 0 으로 사라진다. 그 침묵은 실제로 비용을 치른
// 적이 있다(recvCh 를 아무도 드레인하지 않아 적체되던 결함이, 버퍼 사용량이 보고되지
// 않았기에 오래 보이지 않았다). 아래 단언이 그 침묵을 컴파일 에러로 바꾼다.
var (
	_ agent.Agent              = (*ChirpStackAgent)(nil)
	_ agent.MessageReceiver    = (*ChirpStackAgent)(nil)
	_ agent.TransportChecker   = (*ChirpStackAgent)(nil)
	_ agent.StatefulAgent      = (*ChirpStackAgent)(nil)
	_ agent.MessagePublisher   = (*ChirpStackAgent)(nil)
	_ agent.BufferInfoProvider = (*ChirpStackAgent)(nil)
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
	// timestamp_source 도 같은 이유로 생성 경로에서 무효값을 거부한다 — 오타가 조용히
	// uplink 로 폴백하면 "server 를 켰는데 왜 여전히 장비 시계 시각이지" 를 진단할
	// 단서가 사라진다.
	if err := validateTimestampSource(config.Transport.Options); err != nil {
		return nil, err
	}
	// report_emit_mode 도 동일한 규율로 생성 경로에서 무효값을 거부한다 — 오타가
	// 조용히 per_measurement 로 폴백하면 "combined 를 켰는데 왜 measurement 마다 오지"
	// 를 진단할 단서가 사라진다.
	if err := validateReportEmitMode(config.Transport.Options); err != nil {
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
		reports:       make(map[string]*deviceWindow),
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

	// 집계 리포트 루프 시작 (활성화 + emit_report + report_interval>0 시에만).
	// comm watchdog 과 동일한 기동 지점/규약이며 수명만 독립이다 (report.go).
	a.startMeasurementReporter()

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

	// 타임스탬프 확정 지점 — 업링크 1건당 정확히 한 번.
	//
	// receivedAt(수신 시각)은 여기서 1회만 캡처한다. server 모드에서 각 레코드가 각자
	// time.Now() 를 부르면 같은 업링크에서 나온 측정치들이 마이크로초씩 다른 시각을
	// 갖게 되고, flow message 와 로스터 캐시의 시각도 어긋난다.
	//
	// timestamp_source 스냅샷을 매 업링크마다 읽으므로 Configure 의 모드 전환이 즉시
	// 반영된다 (emit_comm_state / measurement_emit_mode 게이트와 동일한 규약).
	receivedAt := time.Now()
	timeMs := resolveUplinkTimeMs(up, a.cs().TimestampSource, receivedAt)

	// 디바이스 자동 생성/갱신 + UID 발급 + 런타임 info 등록 (M4, REQ-M4-01/02/04).
	a.upsertDevice(up, timeMs)

	// comm-state fold: 활성화 시 last-seen 갱신 + online 전이 change emit (M5, REQ-M5-02).
	// 매 업링크마다 현재 스냅샷을 읽으므로 Configure 의 emit_comm_state 토글이 이 게이트에는
	// 즉시 반영된다 (watchdog 기동/정지는 재시작 필요 — Configure 주석 참조).
	if a.cs().EmitCommState {
		a.onUplinkCommState(up)
	}

	// 집계 리포트 누적 (emit_report opt-in). comm-state fold 와 동일한 게이트 규약이며,
	// 방출은 여기가 아니라 report_interval tick(measurementReportLoop)에서 일어난다.
	if a.cs().EmitReport {
		a.onUplinkReport(up, timeMs)
	}

	// 무선 품질 그룹 (emit_radio opt-in). 업링크 1건당 정확히 한 번만 산출해 그 업링크가
	// 만드는 모든 레코드가 **동일한 인스턴스**를 공유한다 — per_measurement 모드에서
	// measurement 마다 rxInfo 를 다시 순회하면 같은 값을 N 번 재계산할 뿐이다.
	//
	// 꺼져 있으면 nil 이고, nil 은 레코드의 omitempty 포인터 필드에 그대로 들어가
	// radio 키 자체가 사라진다 — 기본 설정의 출력이 오늘과 바이트 동일해지는 지점이다.
	var radio *radioGroup
	if a.cs().EmitRadio {
		radio = buildRadioGroup(up, timeMs)
	}

	// measurement_emit_mode 스냅샷을 매 업링크마다 읽으므로 Configure 의 모드 전환이
	// 즉시 반영된다 (emit_comm_state 게이트와 동일한 규약).
	if a.cs().MeasurementEmitMode == measurementEmitModeCombined {
		a.emitCombinedRecord(up, timeMs, topic, radio)
		return
	}

	records := buildMeasurementRecords(up, timeMs, a.logger)
	for i := range records {
		// radio 부착은 빌더 시그니처를 넓히지 않고 여기서 한다 — buildMeasurementRecords
		// 는 동결 경로의 핵심 빌더이고 기존 호출부/테스트가 그 시그니처에 고정되어 있다.
		// 순수 추가 필드이므로 빌드 후 대입으로 충분하다.
		records[i].Radio = radio
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
//
// radio 는 호출자가 업링크 1건당 1회 산출한 무선 품질 그룹이며(emit_radio 꺼짐 시
// nil), Values 와 분리된 필드로 실린다 — 노드가 Values 를 payload 최상위로 펼 때
// 섞이지 않으므로 "rssi" 라는 이름의 센서가 있어도 충돌하지 않는다.
func (a *ChirpStackAgent) emitCombinedRecord(up *uplink, timeMs int64, topic string, radio *radioGroup) {
	rec, ok := buildCombinedMeasurementRecord(up, timeMs, a.logger)
	if !ok {
		return
	}
	rec.Radio = radio
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
		// 내부 발신 경계: 여기가 메시지를 노드에 실제로 넘기는 유일한 지점이다.
		// 이 카운트가 없으면 "노드로 전달된 건수" 가 구조적으로 항상 0 이 되어,
		// 브로커에서 받기만 하고 노드로는 못 넘기는 상태(적체/미드레인)를 통계로
		// 구분할 수 없다. 실패 분기(done / ctx)는 전달이 아니므로 계상하지 않는다.
		a.stats.IncrInternalMessagesSent()
		return data, nil
	case <-done:
		return nil, fmt.Errorf("chirpstack: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BufferInfo 는 수신 채널(recvCh)의 적체량과 용량을 반환한다
// (agent.BufferInfoProvider, system/mqtt_agent.go 패턴).
//
// WHY: 버퍼 사용량이 보고되지 않으면 "브로커에서는 들어오는데 노드가 드레인하지 않아
// 계속 쌓이는" 상태가 관측되지 않는다. 이 프로젝트에서 실제로 그 침묵 때문에 결함이
// 오래 보이지 않은 적이 있다 — pending/capacity 는 진단의 1차 신호이다.
//
// 락 없음: recvCh 는 생성자에서 1회 할당된 뒤 어디서도 재할당되지 않으므로(Configure
// 도 교체하지 않는다) 필드 읽기에 경합이 없고, 채널의 len/cap 은 런타임이 안전하게
// 처리한다. 수신 핫패스에 락/할당을 추가하지 않기 위한 의도적 선택이다.
func (a *ChirpStackAgent) BufferInfo() (int, int) {
	return len(a.recvCh), cap(a.recvCh)
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

	// 집계 리포트 goroutine 종료 (동일 규약 — 누수 금지).
	a.stopMeasurementReporter()

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

// ErrInvalidCommand 는 Process 디스패처가 알 수 없는 커맨드를 거부할 때 반환하는
// sentinel 이다 (SPEC-CHIRPSTACK-003 REQ-M3-01, xsfm 디스패처 관용구 미러).
//
// 종전 no-op 의 `return nil, nil` 은 오타 커맨드를 성공으로 보이게 만들었다 — 조용한
// nil 대신 명시적 에러로 거부해 호출자가 실패를 인지하게 한다.
var ErrInvalidCommand = errors.New("chirpstack: invalid command")

// Process 는 조회 전용 exec 커맨드를 디스패치한다 (SPEC-CHIRPSTACK-003 REQ-M3-01).
//
// # 의도적 배제의 역전 기록 (REQ-M3-01)
//
// 종전 이 함수는 **의도적 no-op** 이었다 — 주석 원문: "Process 는 사용하지 않는다.
// 다운링크 발행은 PublishMessage 인터페이스를 사용한다 (SPEC-CHIRPSTACK-002
// REQ-M1-02 — SPEC-CHIRPSTACK-001 의 '발행 경로 없음' 배제 역전)."
// SPEC-CHIRPSTACK-003 REQ-M3-01 이 그 배제를 **명시적으로 역전**한다.
//
// WHY: 게이트웨이 로스터는 관리 UI 관심사이지 플로우 런타임 관심사가 아니므로, 신규
// Flow 노드 타입이 아니라 기존 exec 표면(POST /agents/{id}/exec → ag.Process)으로
// 노출한다. State() 확장은 거부했다 — 에이전트 LIST 페이지가 모든 에이전트를
// detail=summary 로 질의하므로 게이트웨이 목록이 모든 LIST 응답을 비대하게 만든다
// (REQ-M3-05).
//
// IMPACT: 이 기록이 없으면 후속 리뷰어가 no-op 의 소멸을 SPEC-001/002 대비 **회귀**로
// 오독한다. SPEC-002 REQ-M1-02 가 발행 경로 역전을 기록한 것과 동일한 규율이다.
//
// # 보존되는 것 (회귀 아님)
//
//   - 다운링크 발행 경로는 **변경되지 않는다**: 여전히 PublishMessage(publish.go)를
//     사용하며 본 디스패처는 발행을 수행하지 않는다.
//   - 수신 계약(REQ-FROZEN-A)과 SPEC-002 status/control 노출면(REQ-FROZEN-C)도 무변경.
//
// # 부작용 없음 (REQ-M3-04)
//
// list_gateways 는 인메모리 캐시의 순수 조회이다 — MQTT publish 0건, on-demand poll
// 0건, 저장소 write 0건. 저장소 영속화 분기(agent_adapter.go)는
// add_device/remove_device/set_device 에만 걸리므로 조회 커맨드는 저장 경로에 닿지 않는다.
func (a *ChirpStackAgent) Process(data []byte) ([]byte, error) {
	var req struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("chirpstack process: invalid JSON: %w", err)
	}

	switch req.Command {
	case "list_gateways":
		return json.Marshal(listGatewaysResponse{Gateways: a.listGateways()})
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidCommand, req.Command)
	}
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
//   - 집계 리포트 루프(measurementReportLoop)도 **정확히 같은 한계**를 갖는다:
//     emit_radio 와 emit_report 의 per-uplink 게이트는 즉시 반영되지만
//     (handleUplink 가 매 업링크마다 현재 스냅샷을 읽는다), report_interval tick
//     goroutine 의 기동/정지는 재시작이 필요하다. 이는 comm watchdog 이 이미 갖고
//     있던 선재 제약을 그대로 따른 것이며 본 변경이 새로 도입하거나 악화시킨 것이
//     아니다(해소도 하지 않는다 — 본 범위 밖).
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
	if prev.EmitReport != next.EmitReport || prev.ReportInterval != next.ReportInterval {
		a.logger.Warn("chirpstack: 집계 리포트 설정 변경 저장됨 — 업링크 누적 게이트는 즉시 반영되나 "+
			"report_interval tick 루프는 에이전트 재시작 후 반영됩니다",
			"emit_report", next.EmitReport, "report_interval", next.ReportInterval)
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
//
// 버퍼 사용량은 AgentStats 가 아니라 채널에서 직접 읽으므로(순간값) 스냅샷 시점에
// 덧붙인다 — system/mqtt_agent.go 와 동일한 관례이며, REST DTO(AgentStatsInfo.Buffer)
// 가 이 두 필드를 그대로 읽는다.
func (a *ChirpStackAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	return s
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
