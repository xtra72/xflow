package engine

// testsupport.go 는 다른 패키지의 테스트가 결정적인 런타임 통계를 구성하도록 돕는
// 테스트 지원 메서드를 제공한다. 이 메서드들은 카운터를 직접 설정할 뿐이며 프로덕션
// 코드 경로에서는 호출되지 않는다(이름의 ForTest 접미사로 의도를 명시한다).
//
// 외부 패키지(internal/api/service) 테스트에서 호출되어야 하므로 export_test.go 가 아닌
// 일반 파일에 둔다(Go 의 _test.go 심볼은 같은 패키지 테스트 바이너리에만 보인다).

// SetNodeProcessedForTest 는 배포된 flowID 내 nodeID 의 processed 카운터를 지정 값으로
// 설정한다. 실제 메시지 흐름 없이도 결정적인 "활성 노드" 통계를 구성할 수 있게 한다.
// 노드 카운터는 DeployFlow 시점에 생성되므로 배포 직후 호출할 수 있다(시작 불필요).
// flowID/nodeID 가 없으면 false 를 반환한다.
func (e *Engine) SetNodeProcessedForTest(flowID, nodeID string, processed int64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, ok := e.flows[flowID]
	if !ok {
		return false
	}
	nc := rt.nodeCounters[nodeID]
	if nc == nil {
		return false
	}
	nc.processed.Store(processed)
	return true
}

// SetPortMessagesForTest 는 배포된 flowID 내 nodeID 의 portName 포트가 생산한 messages
// 카운터를 지정 값으로 설정한다(결정적 활성 포트 통계 구성용). 포트 카운터는 DeployFlow
// 시점에 노드가 선언한 포트별로 생성되므로 배포 직후 호출할 수 있다. 대상이 없으면 false.
func (e *Engine) SetPortMessagesForTest(flowID, nodeID, portName string, messages int64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rt, ok := e.flows[flowID]
	if !ok {
		return false
	}
	nc := rt.nodeCounters[nodeID]
	if nc == nil {
		return false
	}
	pc := nc.portCounters[portName]
	if pc == nil {
		return false
	}
	pc.messages.Store(messages)
	return true
}
