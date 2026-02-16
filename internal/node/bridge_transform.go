package node

import (
	"github.com/xtra/xflow/pkg/message"
)

// BridgeTransformer 는 에이전트 데이터와 플로우 메시지 간의 변환을 담당하는 인터페이스이다.
type BridgeTransformer interface {
	// AgentToFlow 는 에이전트로부터 수신한 바이트 데이터를 플로우 Message로 변환한다.
	AgentToFlow(data []byte) (message.Message, error)

	// FlowToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
	FlowToAgent(msg message.Message) ([]byte, error)
}

// DefaultTransformer 는 BridgeTransformer의 기본 구현체이다.
// 에이전트 데이터를 "_raw" 키로 페이로드에 저장하고, 역변환 시 "_raw" 키에서 추출한다.
type DefaultTransformer struct{}

// NewDefaultTransformer 는 새로운 DefaultTransformer 인스턴스를 반환한다.
func NewDefaultTransformer() *DefaultTransformer {
	return &DefaultTransformer{}
}

// AgentToFlow 는 에이전트로부터 수신한 바이트 데이터를 새 Message로 변환한다.
// 데이터는 페이로드의 "_raw" 키에 []byte 타입으로 저장된다.
func (t *DefaultTransformer) AgentToFlow(data []byte) (message.Message, error) {
	msg := message.New()

	// nil 데이터도 그대로 저장 (nil과 빈 슬라이스를 구분하기 위해)
	if data != nil {
		msg.Payload().Set("_raw", data)
	} else {
		msg.Payload().Set("_raw", nil)
	}

	return msg, nil
}

// FlowToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
// 변환 우선순위:
// 1. "_raw" 키가 []byte이면 직접 반환
// 2. "_raw" 키가 string이면 []byte로 변환하여 반환
// 3. "_raw" 키가 없거나 다른 타입이면 전체 Payload를 JSON으로 직렬화하여 반환
func (t *DefaultTransformer) FlowToAgent(msg message.Message) ([]byte, error) {
	raw, ok := msg.Payload().Get("_raw")
	if ok {
		switch v := raw.(type) {
		case []byte:
			return v, nil
		case string:
			return []byte(v), nil
		}
	}

	// _raw가 없거나 지원하지 않는 타입이면 JSON 폴백
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, err
	}
	return data, nil
}
