package node

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	// serialDefaultBufferSize 는 수신 메시지 버퍼의 기본 크기이다.
	serialDefaultBufferSize = 256
)

// ---------------------------------------------------------------------------
// serialNodeConfig
// ---------------------------------------------------------------------------

// serialNodeConfig 는 시리얼 I/O 노드의 공통 설정이다.
type serialNodeConfig struct {
	AgentRef string `json:"agent_ref"` // 대상 시리얼 Agent 이름/ID (필수)

	// InputEncoding 은 문자열 페이로드(data 필드, 문자열로 도착한 raw 필드)를
	// 바이트로 변환하는 방식을 지정한다. SerialOutNode 에서만 사용한다.
	// 허용 값: "auto"(기본, hex 추론 후 평문 폴백), "hex", "text", "base64".
	InputEncoding string `json:"input_encoding"`
}

// 허용되는 input_encoding 값.
const (
	serialEncodingAuto   = "auto"
	serialEncodingHex    = "hex"
	serialEncodingText   = "text"
	serialEncodingBase64 = "base64"
)

// decodeSerialPayloadString 는 input_encoding 설정에 따라 문자열 페이로드를 바이트로 변환한다.
// raw 가 실제 []byte 로 도착한 경우에는 호출하지 않는다 (바이트는 디코딩이 필요 없다).
func decodeSerialPayloadString(s, encoding string) ([]byte, error) {
	switch encoding {
	case serialEncodingHex:
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("serial-out: input_encoding=hex 인데 유효한 hex 문자열이 아님: %w", err)
		}
		return b, nil
	case serialEncodingText:
		return []byte(s), nil
	case serialEncodingBase64:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("serial-out: input_encoding=base64 인데 유효한 base64 문자열이 아님: %w", err)
		}
		return b, nil
	case serialEncodingAuto, "":
		// 기존 휴리스틱: hex 디코딩을 우선 시도하고, 실패 시에만 평문 바이트로 폴백한다.
		if decoded, err := hex.DecodeString(s); err == nil {
			return decoded, nil
		}
		return []byte(s), nil
	default:
		return nil, fmt.Errorf("serial-out: 알 수 없는 input_encoding %q", encoding)
	}
}

// ---------------------------------------------------------------------------
// serialNodeBase
// ---------------------------------------------------------------------------

// serialNodeBase 는 시리얼 I/O 노드 공통 기반 구조체이다.
// SerialInNode, SerialOutNode가 이를 임베딩한다.
type serialNodeBase struct {
	*BaseNode
	serialCfg serialNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent  // 원본 Agent 객체
	mu        sync.RWMutex // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (sb *serialNodeBase) configure(config map[string]any) error {
	if err := sb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg serialNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrSerialMissingAgentRef
	}

	// input_encoding (선택, 기본 "auto"). SerialOutNode 에서만 사용하지만
	// 공통 설정에서 파싱·검증하여 알 수 없는 값을 Configure 단계에서 거부한다.
	cfg.InputEncoding = serialEncodingAuto
	if v, ok := config["input_encoding"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.InputEncoding = s
		}
	}
	switch cfg.InputEncoding {
	case serialEncodingAuto, serialEncodingHex, serialEncodingText, serialEncodingBase64:
		// 허용 값
	default:
		return fmt.Errorf("serial: 알 수 없는 input_encoding %q (허용: auto, hex, text, base64)", cfg.InputEncoding)
	}

	sb.mu.Lock()
	sb.serialCfg = cfg
	sb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve한다.
func (sb *serialNodeBase) initAgent(ctx context.Context) error {
	if sb.resolver == nil {
		return ErrSerialNoResolver
	}

	sb.mu.RLock()
	agentRef := sb.serialCfg.AgentRef
	sb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := sb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("serial init: agent resolve failed: %w", err)
	}
	sb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득
	if accessor, ok := transport.(AgentAccessor); ok {
		sb.agent = accessor.UnderlyingAgent()
	}

	return nil
}

// shutdown 은 공통 종료 로직을 수행한다.
func (sb *serialNodeBase) shutdown() error {
	return sb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (sb *serialNodeBase) AgentRef() flow.AgentRef {
	sb.mu.RLock()
	ref := sb.serialCfg.AgentRef
	sb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// SerialInNode (시리얼 수신 노드)
// ===========================================================================

// SerialInNode 는 시리얼 포트로부터 데이터를 수신하는 SourceNode이다.
// Agent의 MessageReceiver 인터페이스를 사용하여 데이터를 수신한다.
// Agent가 RawMessageReceiver를 구현하면 raw_out 포트로 프레이밍 이전 원시 바이트도 출력한다.
type SerialInNode struct {
	serialNodeBase
	receiver    agent.MessageReceiver // 메시지 수신 인터페이스
	sourceCh    chan message.Message  // SourceNode 메시지 채널 (out 포트)
	rawSourceCh chan message.Message  // raw_out 포트 메시지 채널 (nil이면 비활성)
	stopCh      chan struct{}         // 수신 루프 종료 시그널
	stopOnce    sync.Once             // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node            = (*SerialInNode)(nil)
	_ SourceNode      = (*SerialInNode)(nil)
	_ MultiSourceNode = (*SerialInNode)(nil)
)

// NewSerialInNode 는 새로운 SerialInNode를 생성하는 팩토리 함수이다.
func NewSerialInNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SerialInNode{
		serialNodeBase: serialNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, serialDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}

	// 옵션에서 AgentResolver 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// Configure 는 SerialInNode의 설정을 적용한다.
func (n *SerialInNode) Configure(config map[string]any) error {
	return n.serialNodeBase.configure(config)
}

// Init 은 SerialInNode를 초기화한다.
// 에이전트를 resolve하고, MessageReceiver 인터페이스를 확인한 후,
// 수신 루프를 시작한다.
// Agent가 RawMessageReceiver를 구현하면 raw_out 포트용 수신 루프도 시작한다.
//
// 에이전트가 아직 활성화되지 않은 경우:
// - 경고를 로깅하고, 에러를 반환하지 않음 (플로우 시작을 막지 않음)
// - 이후 ReinitNodesForAgent 호출 시 에이전트에 연결됨
func (n *SerialInNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.serialNodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrSerialNoResolver: 구성 오류, 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrSerialNoResolver) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("serial init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.serialCfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행하지만, 아직 수신 루프를 시작하지 않음
		// (receiver, agent, transport는 nil 상태)
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// MessageReceiver 인터페이스 확인
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrSerialAgentNotReceiver
	}
	n.receiver = recv

	// RawMessageReceiver 인터페이스 확인 → raw_out 포트 활성화
	if rawRecv, ok := n.agent.(agent.RawMessageReceiver); ok {
		n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)
		go n.rawReceiveLoop(rawRecv.ReceiveRawMessage())
	}

	// 수신 루프 시작
	go n.receiveLoop()

	if err := n.BaseNode.TransitionTo(lifecycle.StateRunning); err != nil {
		return err
	}

	return nil
}

// receiveLoop 는 Agent로부터 시리얼 데이터를 수신하여 sourceCh에 전달하는 고루틴이다.
// receiver가 nil이면 (에이전트가 아직 활성화되지 않음), stopCh를 기다리며 아무것도 하지 않는다.
// Reinit 호출 시 receiver가 설정되고 루프가 재시작된다.
func (n *SerialInNode) receiveLoop() {
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		// receiver가 설정되지 않으면 (에이전트 미사용 가능), 대기
		n.mu.RLock()
		receiver := n.receiver
		n.mu.RUnlock()

		if receiver == nil {
			// 에이전트 연결 대기: stopCh가 닫힐 때까지 또는 일정 시간마다 체크
			select {
			case <-n.stopCh:
				return
			case <-time.After(500 * time.Millisecond):
				// 주기적으로 receiver 상태 재확인
				continue
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := receiver.ReceiveMessage(ctx)
		cancel()

		if err != nil {
			// 타임아웃이나 컨텍스트 취소는 정상 -- 다시 시도
			continue
		}

		if data == nil {
			continue
		}

		// 시리얼 데이터를 플로우 메시지로 변환
		// data: 바이너리를 hex 문자열로 변환 (가독성 + JSON 직렬화 안전)
		msg := message.New()
		// 2026-05-14 hotfix: receiver/framer 가 재사용 buffer 를 반환할 수 있으므로
		// raw 필드는 방어적으로 복사하여 저장한다. 복사하지 않으면 다음 read 가
		// 같은 buffer 를 덮어쓰면서 이미 전달된 메시지의 raw 가 변조된다.
		rawCopy := make([]byte, len(data))
		copy(rawCopy, data)
		msg.Payload().Set("raw", rawCopy)
		msg.Payload().Set("data", hex.EncodeToString(data))
		msg.Metadata().Set("serial.node_id", n.ID())
		if n.agent != nil {
			msg.Metadata().Set("serial.agent_type", n.agent.Type())
		}
		msg.Metadata().Set("message_type", "event")

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}
}

// Process 는 SerialInNode에서는 사용되지 않는다 (SourceNode이므로).
// 입력 메시지를 그대로 통과시킨다.
func (n *SerialInNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// Shutdown 은 SerialInNode를 종료한다.
// 수신 루프를 종료한다.
func (n *SerialInNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	return n.serialNodeBase.shutdown()
}

// SourceCh 는 수신된 시리얼 메시지를 전달하는 채널을 반환한다 (out 포트).
func (n *SerialInNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Reinit 은 에이전트 재시작 후 agent / transport / receiver 참조를 재해석하고
// 수신 루프를 재시작한다. rawReceiveLoop 은 agent.RawMessageReceiver 의 채널을
// 캡처하므로 같이 재시작한다.
func (n *SerialInNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.serialNodeBase.initAgent(ctx); err != nil {
		return err
	}

	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrSerialAgentNotReceiver
	}

	n.mu.Lock()
	n.receiver = recv
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.mu.Unlock()

	// RawMessageReceiver 가 있으면 raw_out 루프도 재시작
	if rawRecv, ok := n.agent.(agent.RawMessageReceiver); ok {
		if n.rawSourceCh == nil {
			n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)
		}
		go n.rawReceiveLoop(rawRecv.ReceiveRawMessage())
	}

	go n.receiveLoop()
	return nil
}

// ExtraSourceChannels 는 추가 출력 포트 채널을 반환한다.
// Agent가 RawMessageReceiver를 구현하면 "raw_out" 포트 채널을 포함한다.
func (n *SerialInNode) ExtraSourceChannels() map[string]<-chan message.Message {
	if n.rawSourceCh == nil {
		return nil
	}
	return map[string]<-chan message.Message{
		"raw_out": n.rawSourceCh,
	}
}

// rawReceiveLoop 는 Agent의 RawMessageReceiver 채널에서 프레이밍 이전 원시 바이트를 수신하여
// rawSourceCh에 메시지로 전달하는 고루틴이다.
func (n *SerialInNode) rawReceiveLoop(rawCh <-chan []byte) {
	for {
		select {
		case <-n.stopCh:
			return
		case data, ok := <-rawCh:
			if !ok {
				return
			}
			msg := message.New()
			// 2026-05-14 hotfix: receiver/framer 가 재사용 buffer 를 반환할 수 있으므로
			// raw 필드는 방어적으로 복사하여 저장한다. 복사하지 않으면 다음 read 가
			// 같은 buffer 를 덮어쓰면서 이미 전달된 메시지의 raw 가 변조된다.
			rawCopy := make([]byte, len(data))
			copy(rawCopy, data)
			msg.Payload().Set("raw", rawCopy)
			msg.Metadata().Set("serial.node_id", n.ID())
			msg.Metadata().Set("serial.port", "raw_out")
			msg.Metadata().Set("message_type", "event")

			select {
			case n.rawSourceCh <- msg:
			case <-n.stopCh:
				return
			}
		}
	}
}

// ===========================================================================
// SerialOutNode (시리얼 송신 노드)
// ===========================================================================

// SerialOutNode 는 시리얼 포트로 데이터를 전송하는 Process 기반 노드이다.
// Agent의 Process 메서드를 사용하여 데이터를 전송한다.
type SerialOutNode struct {
	serialNodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*SerialOutNode)(nil)

// NewSerialOutNode 는 새로운 SerialOutNode를 생성하는 팩토리 함수이다.
func NewSerialOutNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SerialOutNode{
		serialNodeBase: serialNodeBase{
			BaseNode: base,
		},
	}

	// 옵션에서 AgentResolver 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// Configure 는 SerialOutNode의 설정을 적용한다.
func (n *SerialOutNode) Configure(config map[string]any) error {
	return n.serialNodeBase.configure(config)
}

// Init 은 SerialOutNode를 초기화한다.
// 에이전트를 resolve한다.
func (n *SerialOutNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.serialNodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrSerialNoResolver: 구성 오류, 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrSerialNoResolver) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("serial init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.serialCfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행하지만, 아직 agent/transport는 nil 상태
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지의 페이로드를 시리얼 포트로 전송한다.
// raw 바이트 우선, 없으면 data 문자열, 없으면 JSON 직렬화하여 전송한다.
// 전송 후 원본 메시지를 복제하여 출력한다.
func (n *SerialOutNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	var data []byte

	// input_encoding 설정 읽기 (문자열 페이로드 디코딩 방식 결정).
	n.mu.RLock()
	encoding := n.serialCfg.InputEncoding
	n.mu.RUnlock()

	// raw 바이트 우선. []byte 면 그대로(바이트는 디코딩 불필요),
	// 문자열이면 input_encoding 설정에 따라 디코딩한다.
	// 2026-05-14 hotfix: SerialInNode 는 raw 를 []byte 로 set 하지만 JSON round-trip
	// 또는 다른 노드 경유 시 문자열로 전달될 수 있어 양쪽을 모두 처리한다.
	if raw, ok := msg.Payload().Get("raw"); ok {
		switch v := raw.(type) {
		case []byte:
			data = v
		case string:
			decoded, err := decodeSerialPayloadString(v, encoding)
			if err != nil {
				return nil, err
			}
			data = decoded
		}
	}

	// 없으면 data 문자열. input_encoding 설정에 따라 바이트로 변환한다.
	// 2026-05-14: 이전 구현은 hex 추론 휴리스틱만 사용했다. input_encoding 으로
	// hex/text/base64 를 명시할 수 있으며, 미설정 시 auto(휴리스틱)로 하위호환된다.
	if data == nil {
		if s, ok := msg.Payload().Get("data"); ok {
			if str, ok := s.(string); ok {
				decoded, err := decodeSerialPayloadString(str, encoding)
				if err != nil {
					return nil, err
				}
				data = decoded
			}
		}
	}

	// 없으면 JSON 직렬화
	if data == nil {
		jsonData, err := msg.Payload().ToJSON()
		if err != nil {
			return nil, fmt.Errorf("serial-out: payload serialization failed: %w", err)
		}
		data = jsonData
	}

	// 에이전트의 Process 메서드로 데이터 전송
	n.mu.RLock()
	agent := n.agent
	n.mu.RUnlock()

	if agent == nil {
		// 에이전트가 아직 활성화되지 않음: 메시지를 패스스루만 함 (데이터 손실 방지)
		out := msg.Clone()
		out.Metadata().Set("serial.node_id", n.ID())
		out.Metadata().Set("message_type", "response")
		out.Metadata().Set("serial.warning", "agent not available, message not sent to serial port")
		return []message.Message{out}, nil
	}

	if _, err := agent.Process(data); err != nil {
		return nil, fmt.Errorf("serial-out: send failed: %w", err)
	}

	// 패스스루: 입력 메시지를 출력으로 전달
	out := msg.Clone()
	out.Metadata().Set("serial.node_id", n.ID())
	out.Metadata().Set("message_type", "response")

	return []message.Message{out}, nil
}

// Shutdown 은 SerialOutNode를 종료한다.
func (n *SerialOutNode) Shutdown(_ context.Context) error {
	return n.serialNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *SerialOutNode) Reinit(ctx context.Context) error {
	return n.serialNodeBase.initAgent(ctx)
}
