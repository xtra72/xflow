package samsung

// SPEC-HVACR-SYNC-001: 게이트웨이↔서버 미러링/동기화 런타임.
//
// 동일 Hvacr01Agent 코드가 설정으로 3역할을 분기한다:
//   - standalone: a.mirror == nil (기존 동작, 행위 보존)
//   - gateway   : 로컬 트랜스포트 + mirror_uplink_enabled → 디코드 메시지를 up/nasa 로
//     발행(tap, M3), down/control 을 구독해 Process 로 실행(M4)
//   - server    : transport_type:"mirror" → up/nasa 를 구독해 mirrorTransport 에 급전
//     (M2), 제어는 down/control 로 위임(M4)
//
// MQTT 발행/구독은 MirrorBroker 인터페이스로 추상화된다. 프로덕션은 paho 어댑터를,
// 테스트는 in-memory fake 를 주입한다(mirrorBrokerFactory). thingplus/ThingsBoard 전용
// 로직에 의존하지 않는 범용 MQTT 경로이다(REQ-SYNC-001-08-03).

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MirrorBroker 는 미러 동기화 경로의 MQTT 발행/구독을 추상화한다.
type MirrorBroker interface {
	// Publish 는 topic 에 payload 를 발행한다.
	Publish(topic string, qos byte, retained bool, payload []byte) error
	// Subscribe 는 topics 를 구독하고 수신 메시지를 handler 로 전달한다.
	Subscribe(topics []string, handler func(topic string, payload []byte)) error
	// Close 는 브로커 연결을 닫는다.
	Close() error
}

// mirrorBrokerFactory 는 MirrorBroker 를 생성한다. 테스트에서 in-memory fake 로 대체한다.
var mirrorBrokerFactory = newPahoMirrorBroker

// mirrorRole 은 미러 에이전트의 역할이다.
type mirrorRole int

const (
	roleNone mirrorRole = iota
	roleGateway
	roleServer
)

// mirrorRuntime 은 미러 동기화 런타임 상태이다.
type mirrorRuntime struct {
	role       mirrorRole
	topics     mirrorTopics
	qos        byte
	brokerAddr string
	clientID   string

	// M9: 미러 브로커 보안 설정 (인증 + TLS). config 로부터 setupMirror 에서 채운다.
	username string
	password string
	tls      bool
	caCert   string

	controlEnabled  bool
	ackEnabled      bool
	snapshotEnabled bool

	agent     *Hvacr01Agent    // back-ref (Process/logger/protocol 재사용)
	transport *mirrorTransport // server 역할에서만 non-nil
	logger    *slog.Logger

	brokerMu sync.Mutex
	broker   MirrorBroker
	started  atomic.Bool
	stopCh   chan struct{}
	stopOnce sync.Once

	publishErrs atomic.Int64 // 발행 실패 카운터 (REQ-SYNC-001-03-04)
}

// setupMirror 는 config 로부터 미러 런타임을 구성한다(New 단계). broker/goroutine 은
// 아직 기동하지 않는다. 미러/업링크 비활성이면 a.mirror 를 nil 로 남겨 단독 동작한다.
func (a *Hvacr01Agent) setupMirror(cfg Hvacr01Config) {
	role := roleNone
	if cfg.MirrorMode {
		role = roleServer
	} else if cfg.MirrorUplinkEnabled {
		role = roleGateway
	}
	if role == roleNone {
		return
	}

	m := &mirrorRuntime{
		role:            role,
		topics:          newMirrorTopics(cfg.MirrorTopicPrefix, cfg.MirrorGatewayID),
		qos:             cfg.MirrorQoS,
		brokerAddr:      cfg.MirrorBrokerAddr,
		clientID:        fmt.Sprintf("xflow-hvacr-%d-%s", role, cfg.MirrorGatewayID),
		username:        cfg.MirrorUsername,
		password:        cfg.MirrorPassword,
		tls:             cfg.MirrorTLS,
		caCert:          cfg.MirrorCACert,
		controlEnabled:  cfg.MirrorControlEnabled,
		ackEnabled:      cfg.MirrorAckEnabled,
		snapshotEnabled: cfg.MirrorSnapshotEnabled,
		agent:           a,
		logger:          a.logger,
		stopCh:          make(chan struct{}),
	}

	// server 역할: 로컬 transport 는 mirrorTransport 이다.
	if role == roleServer {
		if mt, ok := a.transport.(*mirrorTransport); ok {
			m.transport = mt
		}
	}

	a.mirror = m
}

// startMirror 는 브로커를 생성하고 역할별 구독을 시작한다(Start 단계).
func (a *Hvacr01Agent) startMirror() {
	m := a.mirror
	if m == nil || m.started.Swap(true) {
		return
	}

	broker, err := mirrorBrokerFactory(mirrorBrokerConn{
		addr:     m.brokerAddr,
		clientID: m.clientID,
		username: m.username,
		password: m.password,
		tls:      m.tls,
		caCert:   m.caCert,
	}, m.logger)
	if err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: broker 생성 실패", "error", err)
		m.started.Store(false)
		return
	}
	m.brokerMu.Lock()
	m.broker = broker
	m.brokerMu.Unlock()

	switch m.role {
	case roleServer:
		// up/nasa + (7a) 스냅샷 토픽 구독 → mirrorTransport 급전.
		topics := []string{m.topics.uplinkNasa()}
		if m.snapshotEnabled {
			topics = append(topics, m.topics.snapshotPrefix())
		}
		if err := broker.Subscribe(topics, m.onUplinkMessage); err != nil {
			m.logger.Warn("samsung_hvacr01 mirror: 업링크 구독 실패", "error", err)
		}
		if m.transport != nil {
			m.transport.setAvailable(true)
		}
	case roleGateway:
		// down/control 구독 → Process 로 실행(M4).
		if m.controlEnabled {
			if err := broker.Subscribe([]string{m.topics.downlinkControl()}, m.onDownlinkControl); err != nil {
				m.logger.Warn("samsung_hvacr01 mirror: 다운링크 구독 실패", "error", err)
			}
		}
	}
	m.logger.Info("samsung_hvacr01 mirror: 시작", "role", m.role, "gateway_topics", m.topics.uplinkNasa())
}

// stopMirror 는 미러 런타임을 정지하고 브로커를 닫는다(Stop 단계).
func (a *Hvacr01Agent) stopMirror() {
	m := a.mirror
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stopCh) })
	// server 역할: mirrorTransport.Receive 는 채널 블로킹이므로, wg.Wait 이전에
	// transport 를 닫아 receiveLoop 를 깨워야 교착을 피한다(closeOnce 로 idempotent).
	if m.transport != nil {
		_ = m.transport.Close()
	}
	m.brokerMu.Lock()
	b := m.broker
	m.brokerMu.Unlock()
	if b != nil {
		_ = b.Close()
	}
}

// getBroker 는 현재 브로커를 반환한다(nil 가능).
func (m *mirrorRuntime) getBroker() MirrorBroker {
	m.brokerMu.Lock()
	defer m.brokerMu.Unlock()
	return m.broker
}

// ---------------------------------------------------------------------------
// M3: 게이트웨이 업링크 tap
// ---------------------------------------------------------------------------

// tapUplink 은 성공적으로 디코드된 메시지를 게이트웨이 역할에서 업링크로 발행한다
// (REQ-SYNC-001-03-01). 게이트웨이가 아니거나 broker 미기동이면 no-op 이다(행위 보존).
// handleMessage 종료 후(락 해제 상태) 호출되므로 로컬 상태 반영을 차단하지 않으며,
// 발행 실패는 카운터+로그로 격리된다(REQ-SYNC-001-03-04).
func (a *Hvacr01Agent) tapUplink(msg *NasaMessage) {
	m := a.mirror
	if m == nil || m.role != roleGateway {
		return
	}
	broker := m.getBroker()
	if broker == nil {
		return
	}

	payload, err := json.Marshal(toWireUplink(msg))
	if err != nil {
		m.publishErrs.Add(1)
		m.logger.Warn("samsung_hvacr01 mirror: 업링크 직렬화 실패", "error", err)
		return
	}

	// 업링크 디코드 메시지: QoS≥1, retain=false (스트림 성격, REQ-SYNC-001-07-03).
	if err := broker.Publish(m.topics.uplinkNasa(), m.qos, false, payload); err != nil {
		m.publishErrs.Add(1)
		m.logger.Warn("samsung_hvacr01 mirror: 업링크 발행 실패", "error", err)
	}

	// 7a: 디바이스별 최신 상태를 retained 스냅샷으로 유지 → 재시작 서버 즉시 복원.
	if m.snapshotEnabled {
		if err := broker.Publish(m.topics.snapshot(msg.SourceAddr.Hex()), m.qos, true, payload); err != nil {
			m.publishErrs.Add(1)
			m.logger.Warn("samsung_hvacr01 mirror: 스냅샷 발행 실패", "error", err)
		}
	}
}

// ---------------------------------------------------------------------------
// M2: 서버 업링크 수신 → mirrorTransport 급전
// ---------------------------------------------------------------------------

// onUplinkMessage 는 up/nasa(및 스냅샷) 구독 콜백이다. 와이어 JSON 을 NasaMessage 로
// 역직렬화 → protocol.Encode 로 프레임 재구성 → mirrorTransport.Feed 한다.
// receiveLoop 가 프레임을 소비해 기존 scanner→Decode→handleMessage 경로를 재사용한다.
func (m *mirrorRuntime) onUplinkMessage(_ string, payload []byte) {
	var w wireUplink
	if err := json.Unmarshal(payload, &w); err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: 업링크 역직렬화 실패", "error", err)
		return
	}
	msg, err := w.toNasaMessage()
	if err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: 업링크 메시지 복원 실패", "error", err)
		return
	}
	frame, err := m.agent.protocol.Encode(msg)
	if err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: 업링크 프레임 재구성 실패", "error", err)
		return
	}
	if m.transport != nil {
		m.transport.Feed(frame)
	}
}

// ---------------------------------------------------------------------------
// M4: 제어 역경로
// ---------------------------------------------------------------------------

// publishControl 은 서버 역할에서 제어 명령을 down/control 로 발행한다
// (REQ-SYNC-001-04-01). QoS 1, retain=false(재적용 부작용 방지, REQ-SYNC-001-07-03).
func (m *mirrorRuntime) publishControl(req *processRequest) error {
	broker := m.getBroker()
	if broker == nil {
		return fmt.Errorf("samsung_hvacr01 mirror: broker not started")
	}
	payload, err := json.Marshal(toWireControl(req))
	if err != nil {
		return fmt.Errorf("samsung_hvacr01 mirror: control 직렬화 실패: %w", err)
	}
	return broker.Publish(m.topics.downlinkControl(), m.qos, false, payload)
}

// onDownlinkControl 은 게이트웨이의 down/control 구독 콜백이다. 제어 와이어를
// Process 입력 JSON 으로 매핑해 실제 RS-485 제어를 수행한다(REQ-SYNC-001-04-02).
// ack 활성 시 실행 결과를 up/ack 로 발행한다(REQ-SYNC-001-04-03).
func (m *mirrorRuntime) onDownlinkControl(_ string, payload []byte) {
	var w wireControl
	if err := json.Unmarshal(payload, &w); err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: 다운링크 역직렬화 실패", "error", err)
		return
	}
	req := w.toProcessRequest()
	reqJSON, err := json.Marshal(req)
	if err != nil {
		m.logger.Warn("samsung_hvacr01 mirror: 다운링크 매핑 실패", "error", err)
		return
	}

	resp, procErr := m.agent.Process(reqJSON)

	if m.ackEnabled {
		m.publishAck(w.ReqID, resp, procErr)
	}
}

// publishAck 은 제어 실행 결과를 up/ack 로 발행한다(선택, req_id 상관).
func (m *mirrorRuntime) publishAck(reqID string, resp []byte, procErr error) {
	broker := m.getBroker()
	if broker == nil {
		return
	}
	ack := map[string]any{
		"ts":     time.Now().UnixMilli(),
		"req_id": reqID,
		"ok":     procErr == nil,
	}
	if procErr != nil {
		ack["error"] = procErr.Error()
	} else if len(resp) > 0 {
		ack["result"] = json.RawMessage(resp)
	}
	payload, err := json.Marshal(ack)
	if err != nil {
		return
	}
	if err := broker.Publish(m.topics.uplinkAck(), m.qos, false, payload); err != nil {
		m.publishErrs.Add(1)
	}
}

// ---------------------------------------------------------------------------
// paho MQTT 어댑터 (프로덕션 배선)
// ---------------------------------------------------------------------------

// pahoMirrorBroker 는 MirrorBroker 의 paho 기반 구현체이다.
type pahoMirrorBroker struct {
	client mqtt.Client
	logger *slog.Logger
}

// mirrorBrokerConn 은 미러 브로커 연결에 필요한 주소·식별자·보안 설정을 묶는다(M9).
// 팩토리 시그니처를 단일 파라미터로 유지해 향후 필드 추가 시 호출부 변경을 최소화한다.
type mirrorBrokerConn struct {
	addr     string
	clientID string
	username string // 빈 값이면 SetUsername 미적용
	password string // 빈 값이면 SetPassword 미적용
	tls      bool   // true 면 SetTLSConfig 적용
	caCert   string // TLS 활성 시 서버 검증용 CA (PEM 또는 경로; 빈 값이면 시스템 루트)
}

// newPahoMirrorBroker 는 conn 에 연결된 paho MirrorBroker 를 생성한다.
// AutoReconnect/ConnectRetry 로 백그라운드 재연결을 담당한다(REQ-SYNC-001-07-04).
// 인증(username/password) 및 TLS 는 conn 설정에 따라 적용된다(M9, thingplus 패턴).
func newPahoMirrorBroker(conn mirrorBrokerConn, logger *slog.Logger) (MirrorBroker, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(conn.addr).
		SetClientID(conn.clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetCleanSession(true)

	// 인증: 비어 있지 않을 때만 적용한다(mqtt_agent 패턴 — 무인증 브로커 호환 보존).
	if conn.username != "" {
		opts.SetUsername(conn.username)
	}
	if conn.password != "" {
		opts.SetPassword(conn.password)
	}

	// TLS: 활성 시 CA 로부터 tls.Config 를 구성해 적용한다(thingplus buildTLSConfig 패턴).
	if conn.tls {
		tlsConfig, err := buildMirrorTLSConfig(conn.caCert)
		if err != nil {
			return nil, fmt.Errorf("samsung_hvacr01 mirror: tls 구성 실패: %w", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	// 최초 연결은 대기하되, 타임아웃/에러여도 hard-fail 하지 않고 백그라운드 재연결에
	// 위임한다. 다만 즉시 에러(설정 오류 등)는 반환한다.
	token.WaitTimeout(5 * time.Second)
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 mirror: mqtt connect: %w", err)
	}
	return &pahoMirrorBroker{client: client, logger: logger}, nil
}

// Publish 는 fire-and-forget 로 발행한다(토큰 대기 없음 → 수신 루프 비차단).
func (b *pahoMirrorBroker) Publish(topic string, qos byte, retained bool, payload []byte) error {
	token := b.client.Publish(topic, qos, retained, payload)
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

// Subscribe 는 topics 를 구독하고 수신 메시지를 handler 로 전달한다.
func (b *pahoMirrorBroker) Subscribe(topics []string, handler func(topic string, payload []byte)) error {
	cb := func(_ mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	}
	for _, t := range topics {
		token := b.client.Subscribe(t, 1, cb)
		token.Wait()
		if err := token.Error(); err != nil {
			return err
		}
	}
	return nil
}

// Close 는 브로커 연결을 닫는다.
func (b *pahoMirrorBroker) Close() error {
	b.client.Disconnect(250)
	return nil
}

// buildMirrorTLSConfig 는 caCert 로부터 *tls.Config 를 구성한다(M9, thingplus buildTLSConfig 미러링).
// TLS 활성(conn.tls==true) 경로에서만 호출되므로 항상 유효한 config 를 반환한다.
//   - caCert 가 빈 값이면 시스템 루트 CA 를 사용한다(RootCAs 미설정).
//   - caCert 는 PEM 문자열 또는 파일 경로를 모두 허용한다.
//   - PEM 파싱 실패 시 에러를 반환한다.
func buildMirrorTLSConfig(caCert string) (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCert == "" {
		return tlsConfig, nil
	}

	pemData := []byte(caCert)
	// PEM 헤더가 없으면 파일 경로로 간주하여 로드한다.
	if !strings.Contains(caCert, "-----BEGIN") {
		data, err := os.ReadFile(caCert)
		if err != nil {
			return nil, fmt.Errorf("mirror_ca_cert 읽기 실패: %w", err)
		}
		pemData = data
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("mirror_ca_cert PEM 파싱 실패")
	}
	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}
