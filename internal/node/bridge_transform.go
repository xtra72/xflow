package node

import (
	"github.com/xtra/xflow/pkg/message"
)

// PayloadFormatSetter 는 수신 데이터의 페이로드 변환 형식을 설정할 수 있는 인터페이스이다.
// AgentTransport 구현체가 이 인터페이스를 구현하면 BridgeNode가 Init 시점에
// BridgeConfig의 PayloadFormat을 전달한다.
type PayloadFormatSetter interface {
	SetPayloadFormat(format string)
}

// BridgeTransformer 는 에이전트 데이터와 플로우 메시지 간의 변환을 담당하는 인터페이스이다.
type BridgeTransformer interface {
	// AgentToFlow 는 에이전트로부터 수신한 바이트 데이터를 플로우 Message로 변환한다.
	AgentToFlow(data []byte) (message.Message, error)

	// FlowToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
	FlowToAgent(msg message.Message) ([]byte, error)
}

// DefaultTransformer 는 BridgeTransformer의 기본 구현체이다.
// 에이전트 데이터를 "raw" 키로 페이로드에 저장하고, 역변환 시 "raw" 키에서 추출한다.
type DefaultTransformer struct{}

// NewDefaultTransformer 는 새로운 DefaultTransformer 인스턴스를 반환한다.
func NewDefaultTransformer() *DefaultTransformer {
	return &DefaultTransformer{}
}

// AgentToFlow 는 에이전트로부터 수신한 바이트 데이터를 새 Message로 변환한다.
// 데이터는 페이로드의 "raw" 키에 []byte 타입으로 저장된다.
func (t *DefaultTransformer) AgentToFlow(data []byte) (message.Message, error) {
	msg := message.New()

	// nil 데이터도 그대로 저장 (nil과 빈 슬라이스를 구분하기 위해)
	if data != nil {
		msg.Payload().Set("raw", data)
	} else {
		msg.Payload().Set("raw", nil)
	}

	return msg, nil
}

// FlowToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
// 변환 우선순위:
// 1. "raw" 키가 []byte이면 직접 반환
// 2. "raw" 키가 string이면 []byte로 변환하여 반환
// 3. "_raw" 키도 동일하게 시도 (하위 호환)
// 4. 어디에도 없으면 전체 Payload를 JSON으로 직렬화하여 반환
func (t *DefaultTransformer) FlowToAgent(msg message.Message) ([]byte, error) {
	// "raw" 키 우선
	for _, key := range []string{"raw", "_raw"} {
		raw, ok := msg.Payload().Get(key)
		if ok {
			switch v := raw.(type) {
			case []byte:
				return v, nil
			case string:
				return []byte(v), nil
			}
		}
	}

	// raw가 없거나 지원하지 않는 타입이면 JSON 폴백
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, err
	}
	return data, nil
}
