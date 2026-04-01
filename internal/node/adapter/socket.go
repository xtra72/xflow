package adapter

import (
	"fmt"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// SocketAdapter 는 소켓(TCP/UDP) 에이전트용 브릿지 어댑터이다.
// 바이트 스트림을 플로우 메시지로 변환하고, 플로우 메시지를 바이트 스트림으로 변환한다.
type SocketAdapter struct{}

// NewSocketAdapter 는 새로운 SocketAdapter 를 생성한다.
func NewSocketAdapter() *SocketAdapter {
	return &SocketAdapter{}
}

// Validate 는 주어진 BridgeConfig가 소켓 어댑터에서 유효한지 검증한다.
func (a *SocketAdapter) Validate(config node.BridgeConfig) error {
	switch config.Direction {
	case flow.BridgeIn, flow.BridgeOut, flow.BridgeInOut, flow.BridgeRequestReply:
		return nil
	default:
		return fmt.Errorf("socket adapter: unsupported direction %q", config.Direction)
	}
}

// DefaultConfig 는 소켓 어댑터의 기본 BridgeConfig를 반환한다.
func (a *SocketAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{
		Direction:  flow.BridgeIn,
		BufferSize: 1024,
	}
}

// TransformToFlow 는 소켓에서 수신한 바이트 데이터를 플로우 Message로 변환한다.
func (a *SocketAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	// raw 바이트와 문자열 데이터를 페이로드에 저장한다.
	msg.Payload().Set("raw", data)
	msg.Payload().Set("data", string(data))

	// 에이전트 타입을 메타데이터에 설정한다.
	if meta.AgentType != "" {
		msg.Metadata().Set("agent.type", meta.AgentType)
	}

	return msg, nil
}

// TransformToAgent 는 플로우 Message를 소켓으로 전송할 바이트 데이터로 변환한다.
func (a *SocketAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	meta := node.AgentMeta{}

	// raw 바이트가 있으면 직접 사용한다.
	if raw, ok := msg.Payload().Get("raw"); ok {
		if data, ok := raw.([]byte); ok {
			return data, meta, nil
		}
	}

	// 폴백: 페이로드를 JSON으로 직렬화한다.
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, meta, fmt.Errorf("socket adapter: %w", err)
	}
	return data, meta, nil
}

// HandleControl 은 제어 메시지를 처리한다.
func (a *SocketAdapter) HandleControl(_ message.Message) error {
	return nil
}
