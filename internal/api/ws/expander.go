package ws

import "github.com/xtra/xflow/pkg/message"

// expander.go (message-slim-metadata) 는 레지스트리 기반 GroupExpander 구성 헬퍼를
// 제공한다. egress expand opt-in 을 활성화할 때 사용한다.
//
// 레이어링: ws 패키지는 agent / device 레지스트리 구체 타입에 직접 결합하지 않도록,
// id → fields 조회 함수만 주입받는다. 호출 측(cmd/xflowd 의 wiring)이 레지스트리의
// Get 메서드를 감싼 콜백을 전달한다.

// AgentFieldLookup 은 agentID 로 agent 부가 필드(type/name)를 조회한다.
// 매칭 없으면 ok=false.
type AgentFieldLookup func(id string) (fields map[string]string, ok bool)

// DeviceFieldLookup 은 deviceID 로 device 부가 필드(type/name)를 조회한다.
// 매칭 없으면 ok=false.
type DeviceFieldLookup func(id string) (fields map[string]string, ok bool)

// NewGroupExpander 는 주어진 조회 콜백으로 message.GroupExpander 를 구성한다.
// 어느 콜백이든 nil 이면 해당 그룹은 expand 되지 않고 슬림 상태로 유지된다.
//
// 사용 예(cmd/xflowd):
//
//	exp := ws.NewGroupExpander(
//	    func(id string) (map[string]string, bool) {
//	        a, err := agentMgr.Get(id)
//	        if err != nil { return nil, false }
//	        return map[string]string{"type": a.Type(), "name": a.Name()}, true
//	    },
//	    func(id string) (map[string]string, bool) {
//	        d, err := deviceRegistry.Get(id)
//	        if err != nil { return nil, false }
//	        return map[string]string{"type": string(d.Type()), "name": d.Name()}, true
//	    },
//	)
//	eng.SetOutputObserver(ws.NewTapObserver(tapRegistry, wsHub).WithExpander(exp))
func NewGroupExpander(agentLookup AgentFieldLookup, deviceLookup DeviceFieldLookup) *message.GroupExpander {
	exp := &message.GroupExpander{}
	if agentLookup != nil {
		exp.Agent = func(id string) (map[string]string, bool) { return agentLookup(id) }
	}
	if deviceLookup != nil {
		exp.Device = func(id string) (map[string]string, bool) { return deviceLookup(id) }
	}
	return exp
}
