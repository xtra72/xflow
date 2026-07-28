package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// ThingplusAdapter 는 thingplus-gateway 에이전트 전용 브릿지 어댑터이다.
//
// 이 어댑터는 무상태(stateless) 변환 계층이다. 디바이스 상태 머신, NAME↔device_id 매핑,
// 무손실 버퍼링, RPC 상관 등 상태 로직은 모두 ThingplusGatewayAgent(internal/agent/system)에
// 존재하며, 어댑터는 순수 변환만 담당한다 (관심사 분리, SPEC §1.4 어댑터 추가 스코프).
//
// DESIGN DECISION (bridge.go 근거): Bridge In 기본 경로(engine/resolver.convertPayload)는
// 에이전트 recvCh 바이트를 payload 맵으로만 파싱하고 message Type 을 보존하지 않으며,
// bridge.startReceiveLoop 는 Type 이 비어 있으면 "event" 로 강제한다. 따라서 에이전트는
// 이미 message.Message JSON(Type 포함)을 recvCh 로 방출한다. 이 어댑터의 TransformToFlow 는
// 그 JSON 을 FromJSON 으로 복원하여 Type 을 보존하며, 향후 In 경로가 어댑터를 경유하도록
// 코어가 변경되면 자연히 Type 이 유지된다. (현재 코어 변경은 스코프 밖 — A: 어댑터 추가만.)
type ThingplusAdapter struct{}

// NewThingplusAdapter 는 새로운 ThingplusAdapter 인스턴스를 반환한다.
func NewThingplusAdapter() *ThingplusAdapter {
	return &ThingplusAdapter{}
}

// Validate 는 항상 nil 을 반환한다 (thingplus 어댑터는 방향/토픽 제약을 두지 않는다).
func (a *ThingplusAdapter) Validate(_ node.BridgeConfig) error {
	return nil
}

// DefaultConfig 는 thingplus 어댑터의 기본 BridgeConfig 를 반환한다.
func (a *ThingplusAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{
		Transform:  node.TransformConfig{Mode: "auto", PayloadFormat: node.PayloadFormatAuto},
		BufferSize: 256,
	}
}

// TransformToFlow 는 에이전트가 방출한 바이트를 플로우 Message 로 변환한다.
//
// 에이전트는 message.Message JSON(Type 포함)을 방출하므로 우선 FromJSON 으로 복원하여
// Type("thingplus.rpc.request"/"thingplus.attr.update")을 보존한다. 복원에 실패하면
// 원시 JSON 을 payload 로 파싱하고 payload 의 "type" 필드가 있으면 Type 으로 승격한다.
func (a *ThingplusAdapter) TransformToFlow(data []byte, _ node.AgentMeta) (message.Message, error) {
	// 1) message.Message JSON 복원 시도 (Type 보존).
	if m, err := message.FromJSON(data); err == nil {
		return m, nil
	}

	// 2) 원시 JSON payload 파싱 폴백.
	msg := message.New()
	if data != nil {
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err == nil {
			for k, v := range parsed {
				msg.Payload().Set(k, v)
			}
			// payload 의 "type" 필드를 message Type 으로 승격한다.
			if t, ok := parsed["type"].(string); ok && t != "" {
				msg.SetType(t)
			}
		} else {
			msg.Payload().Set("_raw", data)
		}
	}
	return msg, nil
}

// TransformToAgent 는 플로우 Message 를 에이전트로 전송할 바이트로 변환한다.
//
// 전체 message.Message 를 JSON 으로 직렬화하여 에이전트가 Type("thingplus.rpc.response" 등)과
// payload 를 모두 활용해 업링크/RPC 응답을 분기할 수 있도록 한다. 토픽은 에이전트가
// 내부적으로 결정하므로 AgentMeta.Topic 은 비워 둔다.
func (a *ThingplusAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.MarshalJSON()
	if err != nil {
		return nil, node.AgentMeta{}, fmt.Errorf("thingplus adapter: message 직렬화 실패: %w", err)
	}
	return data, node.AgentMeta{AgentType: "thingplus-gateway"}, nil
}

// HandleControl 은 nil 을 반환한다 (thingplus 어댑터는 제어 메시지 처리가 없다).
func (a *ThingplusAdapter) HandleControl(_ message.Message) error {
	return nil
}
