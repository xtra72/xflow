package node

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	// tcpDefaultBufferSize 는 수신 메시지 버퍼의 기본 크기이다.
	tcpDefaultBufferSize = 256
)

// ---------------------------------------------------------------------------
// tcpNodeConfig
// ---------------------------------------------------------------------------

// tcpNodeConfig 는 TCP 노드 공용 설정 구조체이다.
type tcpNodeConfig struct {
	AgentRef string `json:"agent_ref"` // 대상 TCP Agent 이름/ID (필수)
}

// ---------------------------------------------------------------------------
// tcpNodeBase
// ---------------------------------------------------------------------------

// tcpNodeBase 는 TCP 노드 공통 기반 구조체이다.
// TCPInNode, TCPOutNode가 이를 임베딩한다.
type tcpNodeBase struct {
	*BaseNode
	tcpCfg    tcpNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent  // 원본 Agent 객체
	mu        sync.RWMutex // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (tb *tcpNodeBase) configure(config map[string]any) error {
	if err := tb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg tcpNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrTCPMissingAgentRef
	}

	tb.mu.Lock()
	tb.tcpCfg = cfg
	tb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve한다.
func (tb *tcpNodeBase) initAgent(ctx context.Context) error {
	if tb.resolver == nil {
		return ErrTCPNoResolver
	}

	tb.mu.RLock()
	agentRef := tb.tcpCfg.AgentRef
	tb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := tb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("tcp init: agent resolve failed: %w", err)
	}
	tb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득
	if accessor, ok := transport.(AgentAccessor); ok {
		tb.agent = accessor.UnderlyingAgent()
	}

	return nil
}

// shutdown 은 공통 종료 로직을 수행한다.
func (tb *tcpNodeBase) shutdown() error {
	return tb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (tb *tcpNodeBase) AgentRef() flow.AgentRef {
	tb.mu.RLock()
	ref := tb.tcpCfg.AgentRef
	tb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// TCPInNode — TCP 수신 SourceNode
// ===========================================================================

// TCPInNode 는 TCP 에이전트로부터 메시지를 수신하는 SourceNode이다.
// ConnAwareReceiver 인터페이스가 있으면 연결 정보(remote_addr)를 포함하여 수신하고,
// 없으면 MessageReceiver 인터페이스로 폴백한다.
type TCPInNode struct {
	tcpNodeBase
	connReceiver agent.ConnAwareReceiver // 연결 정보 포함 수신 (TCP 서버)
	receiver     agent.MessageReceiver   // 일반 수신 (TCP 클라이언트 등)
	sourceCh     chan message.Message    // SourceNode 메시지 채널
	stopCh       chan struct{}           // 수신 루프 종료 시그널
	stopOnce     sync.Once               // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*TCPInNode)(nil)
	_ SourceNode = (*TCPInNode)(nil)
)

// NewTCPInNode 는 새로운 TCPInNode를 생성하는 팩토리 함수이다.
func NewTCPInNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TCPInNode{
		tcpNodeBase: tcpNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, tcpDefaultBufferSize),
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

// Configure 는 TCPInNode의 설정을 적용한다.
func (n *TCPInNode) Configure(config map[string]any) error {
	return n.tcpNodeBase.configure(config)
}

// Init 은 TCPInNode를 초기화한다.
// 에이전트를 resolve하고, ConnAwareReceiver 또는 MessageReceiver 인터페이스를 확인한 후,
// 수신 루프를 시작한다.
func (n *TCPInNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.tcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// ConnAwareReceiver 인터페이스 확인 (TCP 서버 — 우선)
	if cr, ok := n.agent.(agent.ConnAwareReceiver); ok {
		n.connReceiver = cr
	}

	// MessageReceiver 인터페이스 확인 (폴백)
	if recv, ok := n.agent.(agent.MessageReceiver); ok {
		n.receiver = recv
	}

	// 둘 다 미구현이면 에러
	if n.connReceiver == nil && n.receiver == nil {
		return ErrTCPAgentNotReceiver
	}

	// 수신 루프 시작
	go n.receiveLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 Agent로부터 메시지를 수신하여 sourceCh에 전달하는 고루틴이다.
// ConnAwareReceiver가 있으면 연결 정보를 포함하여 수신하고,
// 없으면 MessageReceiver로 폴백한다.
func (n *TCPInNode) receiveLoop() {
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		var data []byte
		var remoteAddr string
		var err error

		// ConnAwareReceiver가 있으면 연결 정보 포함 수신
		if n.connReceiver != nil {
			data, remoteAddr, err = n.connReceiver.ReceiveMessageFrom(ctx)
		} else {
			data, err = n.receiver.ReceiveMessage(ctx)
		}
		cancel()

		if err != nil {
			// 타임아웃이나 컨텍스트 취소는 정상 — 다시 시도
			continue
		}
		if data == nil {
			continue
		}

		// 플로우 메시지 생성
		// data: 바이너리를 hex 문자열로 변환 (가독성 + JSON 직렬화 안전)
		msg := message.New()
		msg.Payload().Set("raw", data)
		msg.Payload().Set("data", hex.EncodeToString(data))
		msg.Metadata().Set("tcp.node_id", n.ID())

		// 연결 정보를 메타데이터에 저장 (응답 라우팅에 사용)
		if remoteAddr != "" {
			msg.Metadata().Set("tcp.remote_addr", remoteAddr)
			msg.Metadata().Set("connection_id", remoteAddr)
		} else {
			msg.Metadata().Set("connection_id", n.ID())
		}

		if n.agent != nil {
			msg.Metadata().Set("tcp.agent_type", n.agent.Type())
		}
		msg.Metadata().Set("message_type", "event")

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}
}

// Process 는 TCPInNode에서는 사용되지 않는다 (SourceNode이므로).
// 입력 메시지를 그대로 통과시킨다.
func (n *TCPInNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// Shutdown 은 TCPInNode를 종료한다. 수신 루프를 중지한다.
func (n *TCPInNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	return n.tcpNodeBase.shutdown()
}

// SourceCh 는 수신된 TCP 메시지를 전달하는 채널을 반환한다.
func (n *TCPInNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Reinit 은 에이전트 재시작 후 agent / transport / receiver 참조를 재해석하고
// 수신 루프를 재시작한다.
func (n *TCPInNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.tcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// ConnAwareReceiver / MessageReceiver 인터페이스 재확인
	var connRecv agent.ConnAwareReceiver
	var recv agent.MessageReceiver
	if cr, ok := n.agent.(agent.ConnAwareReceiver); ok {
		connRecv = cr
	}
	if mr, ok := n.agent.(agent.MessageReceiver); ok {
		recv = mr
	}
	if connRecv == nil && recv == nil {
		return ErrTCPAgentNotReceiver
	}

	n.mu.Lock()
	n.connReceiver = connRecv
	n.receiver = recv
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}

// ===========================================================================
// TCPOutNode — TCP 송신 Process 노드
// ===========================================================================

// TCPOutNode 는 TCP 에이전트를 통해 메시지를 전송하는 Process 기반 노드이다.
// 메타데이터의 tcp.remote_addr 를 읽어 특정 클라이언트에게 응답을 라우팅하고,
// 비어 있으면 전체 브로드캐스트로 전송한다.
type TCPOutNode struct {
	tcpNodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*TCPOutNode)(nil)

// NewTCPOutNode 는 새로운 TCPOutNode를 생성하는 팩토리 함수이다.
func NewTCPOutNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TCPOutNode{
		tcpNodeBase: tcpNodeBase{
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

// Configure 는 TCPOutNode의 설정을 적용한다.
func (n *TCPOutNode) Configure(config map[string]any) error {
	return n.tcpNodeBase.configure(config)
}

// Init 은 TCPOutNode를 초기화한다.
// 에이전트를 resolve한다 (Process 메서드로 JSON 명령을 전송하므로 별도 인터페이스 확인 불필요).
func (n *TCPOutNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.tcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지를 TCP 에이전트를 통해 전송한다.
// 메타데이터에서 tcp.remote_addr를 추출하여 특정 클라이언트에게 라우팅하고,
// 비어 있으면 전체 브로드캐스트한다.
// 페이로드에서 raw > data > JSON 직렬화 순서로 전송 데이터를 결정한다.
func (n *TCPOutNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	// 전송할 데이터 추출 (raw > data > JSON fallback)
	var sendData []byte

	if raw, ok := msg.Payload().Get("raw"); ok {
		if b, ok := raw.([]byte); ok {
			sendData = b
		}
	}
	if sendData == nil {
		if s, ok := msg.Payload().Get("data"); ok {
			if str, ok := s.(string); ok {
				sendData = []byte(str)
			}
		}
	}
	if sendData == nil {
		jsonData, err := msg.Payload().ToJSON()
		if err != nil {
			return nil, fmt.Errorf("tcp-out: payload serialization failed: %w", err)
		}
		sendData = jsonData
	}

	// tcp.remote_addr 메타데이터에서 대상 클라이언트 주소 추출
	target, _ := msg.Metadata().Get("tcp.remote_addr")

	// JSON 명령으로 에이전트에 전송
	type sendCommand struct {
		Command string `json:"command"`
		Target  string `json:"target,omitempty"`
		Data    string `json:"data"`
	}

	cmd := sendCommand{
		Command: "send",
		Target:  target, // 빈 문자열이면 전체 브로드캐스트
		Data:    base64.StdEncoding.EncodeToString(sendData),
	}

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("tcp-out: command marshal failed: %w", err)
	}

	if n.agent == nil {
		return nil, fmt.Errorf("tcp-out: agent not initialized")
	}

	if _, err := n.agent.Process(cmdJSON); err != nil {
		return nil, fmt.Errorf("tcp-out: send failed: %w", err)
	}

	// 패스스루 — 원본 메시지를 복제하여 출력
	out := msg.Clone()
	out.Metadata().Set("tcp.node_id", n.ID())
	out.Metadata().Set("message_type", "response")

	return []message.Message{out}, nil
}

// Shutdown 은 TCPOutNode를 종료한다.
func (n *TCPOutNode) Shutdown(_ context.Context) error {
	return n.tcpNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *TCPOutNode) Reinit(ctx context.Context) error {
	return n.tcpNodeBase.initAgent(ctx)
}
