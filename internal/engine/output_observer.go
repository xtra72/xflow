package engine

import (
	"github.com/xtra/xflow/pkg/message"
)

// OutputObserver 는 노드가 출력 포트로 메시지를 방출할 때마다 호출되는 옵저버이다.
//
// 엔진의 핫 패스에서 호출되므로 구현체는 다음을 반드시 지켜야 한다:
//   - 빠를 것: 관측 대상이 아니면 즉시 반환(직렬화/할당 금지).
//   - 논블로킹: 느린 소비자가 엔진을 막지 않도록 드롭 전략을 사용할 것.
//   - 패닉 격리: 내부 패닉이 엔진 고루틴을 죽이지 않도록 복구할 것.
//
// 엔진은 in-memory message.Message 를 그대로 전달한다. 직렬화 여부와 시점은
// 옵저버가 결정한다(엔진은 직렬화하지 않는다). msg 는 호출 이후에도 다른
// 고루틴(와이어 전송)에서 사용될 수 있으므로 옵저버는 msg 를 변경해서는 안 된다.
type OutputObserver interface {
	OnNodeOutput(flowID, nodeID, port string, msg message.Message)
}

// outputObserverHolder 는 atomic.Value 에 저장하기 위한 래퍼이다.
// 인터페이스 nil 값을 atomic.Value 에 직접 저장할 수 없으므로 구조체로 감싼다.
type outputObserverHolder struct {
	obs OutputObserver
}

// notifyOutputObserver 는 옵저버가 설정되어 있으면 OnNodeOutput 을 호출한다.
// 옵저버가 없으면 atomic load + nil 체크만 수행하여 핫 패스 오버헤드를 최소화한다.
// 직렬화/할당은 발생하지 않으며, 와이어 연결 여부와 무관하게 모든 방출 메시지에 대해 호출된다.
func (e *Engine) notifyOutputObserver(flowID, nodeID, port string, msg message.Message) {
	v := e.outputObserver.Load()
	if v == nil {
		return
	}
	holder, ok := v.(outputObserverHolder)
	if !ok || holder.obs == nil {
		return
	}
	holder.obs.OnNodeOutput(flowID, nodeID, port, msg)
}

// OutputObserver 는 현재 설정된 옵저버를 반환한다. 미설정 시 nil 을 반환한다.
func (e *Engine) OutputObserver() OutputObserver {
	v := e.outputObserver.Load()
	if v == nil {
		return nil
	}
	holder, ok := v.(outputObserverHolder)
	if !ok {
		return nil
	}
	return holder.obs
}

// SetOutputObserver 는 엔진에 OutputObserver 를 사후 주입한다.
// WebSocket Hub 초기화 이후, 플로우 시작 이전에 호출하는 것을 권장한다.
// atomic 저장이므로 동시 접근에 안전하다.
func (e *Engine) SetOutputObserver(obs OutputObserver) {
	e.outputObserver.Store(outputObserverHolder{obs: obs})
}
