package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// NASAAdapter 는 Samsung NASA HVAC 에이전트용 브릿지 어댑터이다.
// BridgeAdapter(이벤트 변환)와 CommandPollAdapter(주기적 상태 폴링)를 구현한다.
type NASAAdapter struct{}

// NewNASAAdapter 는 새 NASAAdapter 인스턴스를 생성한다.
func NewNASAAdapter() *NASAAdapter {
	return &NASAAdapter{}
}

// compile-time 인터페이스 구현 확인
var (
	_ node.BridgeAdapter      = (*NASAAdapter)(nil)
	_ node.CommandPollAdapter = (*NASAAdapter)(nil)
)

// --- BridgeAdapter ---

// Validate 는 브릿지 설정이 Samsung NASA 에이전트에 유효한지 검증한다.
func (a *NASAAdapter) Validate(config node.BridgeConfig) error {
	return nil
}

// DefaultConfig 는 Samsung NASA 기본 브릿지 설정을 반환한다.
func (a *NASAAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{}
}

// TransformToFlow 는 에이전트 이벤트 데이터를 플로우 메시지로 변환한다.
func (a *NASAAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		// JSON 파싱 실패 시 원시 데이터를 raw 필드에 저장
		msg.Payload().Set("raw", string(data))
		msg.Metadata().Set("nasa.source", "event")
		msg.Metadata().Set("nasa.format", "raw")
		return msg, nil
	}

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("nasa.source", "event")

	return msg, nil
}

// TransformToAgent 는 플로우 메시지를 에이전트 명령으로 변환한다.
func (a *NASAAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("nasa adapter: marshal: %w", err)
	}
	return data, node.AgentMeta{AgentType: "samsung-nasa"}, nil
}

// HandleControl 은 제어 메시지를 처리한다.
func (a *NASAAdapter) HandleControl(msg message.Message) error {
	return nil
}

// --- CommandPollAdapter ---

// PollCommand 는 전체 디바이스 상태 조회 명령을 반환한다.
func (a *NASAAdapter) PollCommand() []byte {
	cmd, _ := json.Marshal(map[string]any{"command": "get_all_states"})
	return cmd
}

// AssemblePollMessage 는 get_all_states 응답을 플로우 메시지로 변환한다.
func (a *NASAAdapter) AssemblePollMessage(response []byte) (message.Message, error) {
	msg := message.New()

	var payload map[string]any
	if err := json.Unmarshal(response, &payload); err != nil {
		return msg, fmt.Errorf("nasa adapter: unmarshal poll response: %w", err)
	}

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("nasa.source", "poll")

	return msg, nil
}
