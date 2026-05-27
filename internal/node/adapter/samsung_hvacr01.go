package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// SamsungHvacr01Adapter 는 Samsung NASA HVAC 에이전트용 브릿지 어댑터이다.
// BridgeAdapter(이벤트 변환)와 CommandPollAdapter(주기적 상태 폴링)를 구현한다.
type SamsungHvacr01Adapter struct{}

// NewSamsungHvacr01Adapter 는 새 SamsungHvacr01Adapter 인스턴스를 생성한다.
func NewSamsungHvacr01Adapter() *SamsungHvacr01Adapter {
	return &SamsungHvacr01Adapter{}
}

// compile-time 인터페이스 구현 확인
var (
	_ node.BridgeAdapter          = (*SamsungHvacr01Adapter)(nil)
	_ node.MultiMessagePollAdapter = (*SamsungHvacr01Adapter)(nil)
)

// --- BridgeAdapter ---

// Validate 는 브릿지 설정이 Samsung NASA 에이전트에 유효한지 검증한다.
func (a *SamsungHvacr01Adapter) Validate(config node.BridgeConfig) error {
	return nil
}

// DefaultConfig 는 Samsung NASA 기본 브릿지 설정을 반환한다.
func (a *SamsungHvacr01Adapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{}
}

// TransformToFlow 는 에이전트 이벤트 데이터를 플로우 메시지로 변환한다.
func (a *SamsungHvacr01Adapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
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
func (a *SamsungHvacr01Adapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("nasa adapter: marshal: %w", err)
	}
	return data, node.AgentMeta{AgentType: "samsung_hvacr01"}, nil
}

// HandleControl 은 제어 메시지를 처리한다.
func (a *SamsungHvacr01Adapter) HandleControl(msg message.Message) error {
	return nil
}

// --- CommandPollAdapter ---

// PollCommand 는 전체 디바이스 상태 조회 명령을 반환한다.
func (a *SamsungHvacr01Adapter) PollCommand() []byte {
	cmd, _ := json.Marshal(map[string]any{"command": "get_all_states"})
	return cmd
}

// AssemblePollMessage 는 get_all_states 응답을 단일 플로우 메시지로 변환한다.
// 하위 호환성을 위해 유지하지만, bridge 노드는 AssemblePollMessages를 우선 사용한다.
func (a *SamsungHvacr01Adapter) AssemblePollMessage(response []byte) (message.Message, error) {
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

// AssemblePollMessages 는 get_all_states 응답을 디바이스별 개별 메시지로 분리한다.
// 각 메시지는 단일 디바이스의 상태를 포함하며, device_state_changed 이벤트와
// 동일한 페이로드 구조(device_id, state, address 등)를 갖는다.
func (a *SamsungHvacr01Adapter) AssemblePollMessages(response []byte) ([]message.Message, error) {
	var resp struct {
		Devices []map[string]any `json:"devices"`
		Status  string           `json:"status"`
	}
	if err := json.Unmarshal(response, &resp); err != nil {
		return nil, fmt.Errorf("nasa adapter: unmarshal poll response: %w", err)
	}

	// devices 배열이 없으면 단일 메시지로 폴백
	if len(resp.Devices) == 0 {
		msg, err := a.AssemblePollMessage(response)
		if err != nil {
			return nil, err
		}
		return []message.Message{msg}, nil
	}

	msgs := make([]message.Message, 0, len(resp.Devices))
	for _, dev := range resp.Devices {
		msg := message.New()
		for k, v := range dev {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("nasa.source", "poll")
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
