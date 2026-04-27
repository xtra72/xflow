package adapter

import (
	"fmt"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/message"
)

// validSubtypes 는 유효한 시스템 에이전트 서브타입 목록이다.
var validSubtypes = map[string]bool{
	"store":  true,
	"timer":  true,
	"logger": true,
	"event":  true,
	"file":   true,
}

// SystemAdapter 는 시스템 에이전트 연산을 BridgeAdapter로 래핑한다.
// 기존 메타데이터 기반 디스패치 패턴(store_bridge, timer_bridge, logger_bridge)을
// 어댑터 인터페이스로 브릿징한다.
type SystemAdapter struct {
	subtype string // store, timer, logger, event, file
}

// NewSystemAdapter 는 지정된 서브타입의 SystemAdapter를 생성한다.
func NewSystemAdapter(subtype string) *SystemAdapter {
	return &SystemAdapter{subtype: subtype}
}

// Validate 는 주어진 BridgeConfig가 이 어댑터에서 유효한지 검증한다.
// 서브타입이 유효하지 않으면 에러를 반환한다.
func (a *SystemAdapter) Validate(_ node.BridgeConfig) error {
	if !validSubtypes[a.subtype] {
		return fmt.Errorf(
			"invalid system agent subtype %q: valid subtypes are store, timer, logger, event, file",
			a.subtype,
		)
	}
	return nil
}

// DefaultConfig 는 이 어댑터의 기본 BridgeConfig를 반환한다.
// 시스템 에이전트는 별도의 브릿지 설정이 필요 없으므로 빈 설정을 반환한다.
func (a *SystemAdapter) DefaultConfig() node.BridgeConfig {
	return node.BridgeConfig{}
}

// TransformToFlow 는 에이전트로부터 수신한 바이트 데이터를 플로우 Message로 변환한다.
// 시스템 에이전트의 데이터는 주로 JSON 형식이다. JSON 파싱을 시도하고,
// 실패 시 원본 데이터를 _raw 키로 저장한다.
// AgentMeta를 통해 전달된 메타데이터도 함께 설정한다.
func (a *SystemAdapter) TransformToFlow(data []byte, meta node.AgentMeta) (message.Message, error) {
	msg := message.New()

	if data != nil {
		// JSON 파싱 시도, 실패 시 원본 데이터를 _raw로 저장
		if err := trySetJSONPayload(msg, data); err != nil {
			msg.Payload().Set("_raw", data)
		}
	}

	// 에이전트 메타데이터를 메시지 메타데이터로 변환
	node.MetaToMetadata(meta, msg.Metadata())

	return msg, nil
}

// TransformToAgent 는 플로우 Message를 에이전트로 전송할 바이트 데이터로 변환한다.
// 기존 메타데이터 디스패치 패턴을 보존한다.
func (a *SystemAdapter) TransformToAgent(msg message.Message) ([]byte, node.AgentMeta, error) {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, node.AgentMeta{}, err
	}

	meta := node.MetadataToMeta(msg.Metadata())
	meta.AgentType = "system"

	return data, meta, nil
}

// HandleControl 은 제어 메시지를 처리한다.
// 시스템 에이전트는 현재 별도의 제어 메시지 처리가 필요 없으므로 nil을 반환한다.
func (a *SystemAdapter) HandleControl(_ message.Message) error {
	return nil
}
