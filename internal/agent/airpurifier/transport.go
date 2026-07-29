package airpurifier

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MessageHandler 는 구독한 토픽으로 메시지가 도착했을 때 호출되는 콜백이다.
type MessageHandler func(topic string, payload []byte)

// MQTTClient 는 브로커 I/O 경계 추상화이다 (REQ-AIRPUR-001-01-03).
//
// 이 인터페이스로 실제 Paho 클라이언트(direct 모드)와 목(mock) 브로커(테스트)를 교체
// 주입할 수 있다. 공유 로직 레이어는 이 인터페이스만 알고 broker-vs-port 를 알지 못한다.
type MQTTClient interface {
	Connect() error
	Disconnect()
	Publish(topic string, qos byte, payload []byte) error
	Subscribe(topic string, qos byte, cb MessageHandler) error
	// Unsubscribe 는 토픽 구독을 해제한다 (remove_device 배선, REQ-AIRPUR-001-02-04).
	Unsubscribe(topic string) error
	IsConnected() bool
}

// outboundCommand 는 cmdSink 로 방출되는 단일 명령의 완전한 주소 컨텍스트이다 (M14).
//
// Fields 는 명령 토픽의 비-attribute placeholder 값(주소)이며, Attribute 는 attribute-per-topic
// 모드에서 이 발행이 지칭하는 상태 축 토큰이다(blob 모드에서는 ""). Payload 는 attribute
// 모드에서 축 스칼라, blob 모드에서 JSON blob 이다. direct·port 두 구현이 이 값을 공유한다.
type outboundCommand struct {
	DeviceID  string            // 합성 device_id / 로스터 키 (port ControlMessage · 로깅용)
	Fields    map[string]string // 비-attribute placeholder 값 (명령 토픽 렌더용)
	Attribute string            // {attribute} 토큰 (attribute 모드), blob 모드는 ""
	Payload   []byte            // 인코딩된 페이로드 (attribute: 스칼라, blob: JSON)
}

// ControlMessage 는 제어 출력 포트로 방출되는 단일 명령이다 (port 모드).
//
// port 모드에서 에이전트는 실제 토픽 스킴을 소유하지 않으므로(하류 mqtt-out 노드가 소유),
// 하류가 명령 토픽을 재구성할 수 있도록 device_id · 주소 필드(Fields) · attribute · 인코딩된
// 페이로드를 담는다 (REQ-AIRPUR-001-01-11/12, M14 다중 필드). Fields/Attribute 는 blob 모드에서
// 비어 있으며, DeviceID·Payload 는 하위호환을 위해 항상 채워진다.
type ControlMessage struct {
	DeviceID  string            `json:"device_id"`
	Fields    map[string]string `json:"fields,omitempty"`
	Attribute string            `json:"attribute,omitempty"`
	Payload   []byte            `json:"payload"`
}

// CommandSink 는 명령 출력(egress) 경계 추상화이다 (REQ-AIRPUR-001-01-11).
//
// direct 구현은 MQTTClient 로 브로커에 발행하고, port 구현은 제어 출력 포트(채널)로
// 방출한다. 공유 로직 레이어(제어 명령 구성/인코딩)는 이 인터페이스만 사용한다.
type CommandSink interface {
	// SendCommand 는 인코딩된 명령을 대상 디바이스로 방출한다.
	SendCommand(cmd outboundCommand) error
}

// brokerCommandSink 는 명령을 브로커로 발행하는 direct 모드 구현이다.
type brokerCommandSink struct {
	client    MQTTClient
	topicTmpl string
	qos       byte
}

// SendCommand 는 command_topic_template 을 주소 필드(+attribute)로 렌더링한 토픽으로 발행한다.
// attribute-per-topic 모드에서는 {attribute} 를 cmd.Attribute 로 채워 축별 토픽을 산출한다.
func (s *brokerCommandSink) SendCommand(cmd outboundCommand) error {
	if s.client == nil || !s.client.IsConnected() {
		return ErrNotConnected
	}
	fields := cmd.Fields
	if cmd.Attribute != "" {
		fields = make(map[string]string, len(cmd.Fields)+1)
		for k, v := range cmd.Fields {
			fields[k] = v
		}
		fields[placeholderAttribute] = cmd.Attribute
	}
	return s.client.Publish(renderTopic(s.topicTmpl, fields), s.qos, cmd.Payload)
}

// portCommandSink 는 명령을 제어 출력 포트(버퍼드 채널)로 방출하는 port 모드 구현이다.
// 하류 노드가 ControlPort 채널을 drain 하여 mqtt-out 으로 발행한다.
type portCommandSink struct {
	ch      chan ControlMessage
	logger  *slog.Logger
	logDrop bool
}

// SendCommand 는 제어 출력 포트로 ControlMessage 를 non-blocking 방출한다.
// 주소 필드/attribute 를 함께 실어 하류 mqtt-out 이 토픽을 재구성할 수 있게 한다 (M14).
// 포트가 가득 차면(하류 미소비) 드롭하고 로그만 남긴다 (전체 백프레셔는 후속 배치).
func (s *portCommandSink) SendCommand(cmd outboundCommand) error {
	msg := ControlMessage{DeviceID: cmd.DeviceID, Fields: cmd.Fields, Attribute: cmd.Attribute, Payload: cmd.Payload}
	select {
	case s.ch <- msg:
		return nil
	default:
		if s.logger != nil && s.logDrop {
			s.logger.Warn("airpurifier: control port full, dropping command", "device_id", cmd.DeviceID)
		}
		return nil
	}
}

// subEntry 는 재연결 시 복원할 구독 항목이다 (topic → qos + 콜백).
type subEntry struct {
	qos byte
	cb  MessageHandler
}

// pahoMQTTClient 는 Paho MQTT 클라이언트를 MQTTClient 인터페이스로 감싸는 thin wrapper 이다
// (thingplus_agent.go 패턴). 재연결 시 OnConnect 에서 구독을 복원한다. 무거운 오프라인/
// 모니터링 로직은 B5 에서 추가된다.
type pahoMQTTClient struct {
	cfg    AirPurifierConfig
	logger *slog.Logger

	client mqtt.Client
	mu     sync.Mutex
	subs   map[string]subEntry // 재연결 복원용 구독 레지스트리
}

// logLifecycle 은 log_mqtt 옵션이 켜져 있을 때만 MQTT 생명주기 이벤트를 INFO 로 로그한다
// (opt-in 진단용). 토글이 꺼져 있거나 logger 가 nil 이면 no-op → 운영 로그 볼륨 0.
// 연결 실패/유실 같은 문제 신호는 별도 Warn 으로 항상 남기며(이 게이트와 무관), 여기서는
// 정상 생명주기 트레이스(연결 성공·해제·구독·발행·구독 복원)만 게이트한다.
func (c *pahoMQTTClient) logLifecycle(msg string, args ...any) {
	if !c.cfg.LogMQTT || c.logger == nil {
		return
	}
	c.logger.Info("airpurifier mqtt: "+msg, args...)
}

// newPahoMQTTClient 는 설정으로 pahoMQTTClient 를 생성한다 (아직 연결하지 않는다).
func newPahoMQTTClient(cfg AirPurifierConfig, logger *slog.Logger) *pahoMQTTClient {
	return &pahoMQTTClient{
		cfg:    cfg,
		logger: logger,
		subs:   make(map[string]subEntry),
	}
}

// Connect 는 브로커에 연결한다. AutoReconnect 시 초기 연결 실패를 치명적으로 보지 않고
// 백그라운드 재연결에 맡긴다 (thingplus 패턴). OnConnect 에서 구독을 복원한다.
func (c *pahoMQTTClient) Connect() error {
	tlsConfig, err := c.buildTLSConfig()
	if err != nil {
		return fmt.Errorf("airpurifier: tls config: %w", err)
	}

	opts := mqtt.NewClientOptions().
		AddBroker(c.cfg.Broker).
		SetClientID(c.cfg.ClientID).
		SetUsername(c.cfg.Username).
		SetPassword(c.cfg.Password).
		SetKeepAlive(c.cfg.KeepAlive).
		SetAutoReconnect(c.cfg.AutoReconnect).
		SetCleanSession(true).
		SetConnectTimeout(c.cfg.ConnectTimeout).
		SetOrderMatters(false)

	if tlsConfig != nil {
		opts.SetTLSConfig(tlsConfig)
	}
	if c.cfg.AutoReconnect {
		opts.SetConnectRetry(true)
		opts.SetConnectRetryInterval(c.cfg.ConnectTimeout)
	}
	opts.SetOnConnectHandler(c.onConnect)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.logger.Warn("airpurifier: mqtt connection lost", "broker", c.cfg.Broker, "error", err)
	})

	c.client = mqtt.NewClient(opts)
	token := c.client.Connect()
	connected := token.WaitTimeout(c.cfg.ConnectTimeout) && token.Error() == nil
	if !connected {
		if c.cfg.AutoReconnect {
			c.logger.Warn("airpurifier: initial mqtt connect failed, retrying in background",
				"broker", c.cfg.Broker, "error", token.Error())
			return nil
		}
		if token.Error() != nil {
			return fmt.Errorf("airpurifier: mqtt connect: %w", token.Error())
		}
		return fmt.Errorf("airpurifier: mqtt connect timeout (%s)", c.cfg.Broker)
	}
	c.logLifecycle("connect succeeded", "broker", c.cfg.Broker, "auto_reconnect", c.cfg.AutoReconnect)
	return nil
}

// Disconnect 는 브로커 연결을 끊는다.
func (c *pahoMQTTClient) Disconnect() {
	if c.client != nil {
		c.client.Disconnect(250)
		c.logLifecycle("disconnect", "broker", c.cfg.Broker)
	}
}

// Publish 는 토픽으로 페이로드를 발행한다.
func (c *pahoMQTTClient) Publish(topic string, qos byte, payload []byte) error {
	if c.client == nil || !c.client.IsConnected() {
		return ErrNotConnected
	}
	token := c.client.Publish(topic, qos, false, payload)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("airpurifier: mqtt publish %q: %w", topic, err)
	}
	c.logLifecycle("published", "topic", topic, "qos", qos, "len", len(payload))
	return nil
}

// Subscribe 는 토픽을 구독하고 재연결 복원을 위해 구독을 레지스트리에 기록한다.
func (c *pahoMQTTClient) Subscribe(topic string, qos byte, cb MessageHandler) error {
	c.mu.Lock()
	c.subs[topic] = subEntry{qos: qos, cb: cb}
	c.mu.Unlock()

	if c.client == nil || !c.client.IsConnected() {
		// 미연결 상태: 구독을 기록만 하고 OnConnect 에서 복원한다 (thingplus 패턴).
		return nil
	}
	return c.doSubscribe(topic, qos, cb)
}

// Unsubscribe 는 토픽 구독을 해제하고 재연결 복원 레지스트리에서도 제거한다.
// 미연결 상태면 레지스트리에서만 제거한다(OnConnect 복원 대상에서 빠진다).
func (c *pahoMQTTClient) Unsubscribe(topic string) error {
	c.mu.Lock()
	delete(c.subs, topic)
	c.mu.Unlock()

	if c.client == nil || !c.client.IsConnected() {
		return nil
	}
	token := c.client.Unsubscribe(topic)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("airpurifier: mqtt unsubscribe %q: %w", topic, err)
	}
	return nil
}

// IsConnected 는 브로커 연결 상태를 반환한다.
func (c *pahoMQTTClient) IsConnected() bool {
	return c.client != nil && c.client.IsConnected()
}

// onConnect 는 (재)연결 성공 시 기록된 구독을 모두 복원한다 (REQ-AIRPUR-001-01 재연결 복원).
func (c *pahoMQTTClient) onConnect(_ mqtt.Client) {
	c.mu.Lock()
	restore := make(map[string]subEntry, len(c.subs))
	for topic, e := range c.subs {
		restore[topic] = e
	}
	c.mu.Unlock()

	for topic, e := range restore {
		if err := c.doSubscribe(topic, e.qos, e.cb); err != nil {
			c.logger.Error("airpurifier: subscription restore failed", "topic", topic, "error", err)
			continue
		}
	}
	c.logLifecycle("connected, subscriptions restored",
		"broker", c.cfg.Broker, "subscriptions", len(restore))
}

// doSubscribe 는 실제 Paho 구독을 수행하며 MessageHandler 로 어댑트한다.
func (c *pahoMQTTClient) doSubscribe(topic string, qos byte, cb MessageHandler) error {
	token := c.client.Subscribe(topic, qos, func(_ mqtt.Client, m mqtt.Message) {
		cb(m.Topic(), m.Payload())
	})
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("airpurifier: mqtt subscribe %q: %w", topic, err)
	}
	c.logLifecycle("subscribed", "topic", topic, "qos", qos)
	return nil
}

// buildTLSConfig 는 CACert 로부터 *tls.Config 를 구성한다 (thingplus 패턴).
// TLS 미사용 시 nil. CACert 는 인라인 PEM 문자열 또는 파일 경로를 모두 허용한다.
func (c *pahoMQTTClient) buildTLSConfig() (*tls.Config, error) {
	if !c.cfg.TLS {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.cfg.CACert == "" {
		return tlsConfig, nil
	}
	pemData := []byte(c.cfg.CACert)
	if !strings.Contains(c.cfg.CACert, "-----BEGIN") {
		data, err := os.ReadFile(c.cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("airpurifier: read ca_cert: %w", err)
		}
		pemData = data
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("airpurifier: parse ca_cert PEM failed")
	}
	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}

// 컴파일 타임 인터페이스 체크.
var (
	_ MQTTClient  = (*pahoMQTTClient)(nil)
	_ CommandSink = (*brokerCommandSink)(nil)
	_ CommandSink = (*portCommandSink)(nil)
)

// controlPortBuffer 는 제어 출력 포트 채널의 기본 버퍼 크기이다.
const controlPortBuffer = 256
