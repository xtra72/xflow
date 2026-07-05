package node

import (
	"fmt"
	"sync"
)

// expr_funcs_lookup.go (message-slim-metadata / enrich) 는 expression 빌트인
// agentInfo(id) / deviceInfo(id) 를 제공한다. 두 함수는 C 레이어의 룩업
// (AgentInfoLookup / DeviceInfoLookup) 을 통해 id 로 {type,id,name} 맵을 반환한다.
//
// 스레딩 방식: expression 컴파일 경로(compileExpressionV2 → defaultBuiltinFuncs)는
// 인자 없는 package-level 함수 체인이다. 룩업은 프로세스 전역 싱글턴(레지스트리
// 1세트)이므로, 여기서는 package-level holder 에 룩업을 1회 설정(SetExprLookups)하고
// defaultBuiltinFuncs 가 이를 참조하여 빌트인을 등록한다. holder 미설정 시 두 빌트인은
// 등록되지 않으며(호출 시 "unknown function" 에러) 기존 동작에 영향이 없다.
//
// backward-compat: 룩업을 설정하지 않은 기존 배포/테스트는 agentInfo/deviceInfo 를
// 노출하지 않으므로 표현식 평가 동작이 전혀 바뀌지 않는다.

// exprLookups 는 expression 빌트인이 사용하는 프로세스 전역 룩업 holder 이다.
var exprLookups struct {
	mu     sync.RWMutex
	agent  AgentInfoLookup
	device DeviceInfoLookup
}

// SetExprLookups 는 expression 빌트인(agentInfo/deviceInfo)이 사용할 룩업을 설정한다.
// 엔진 구성 시 1회 호출한다(멱등). nil 인자는 해당 빌트인을 비활성 상태로 둔다.
func SetExprLookups(agent AgentInfoLookup, device DeviceInfoLookup) {
	exprLookups.mu.Lock()
	defer exprLookups.mu.Unlock()
	exprLookups.agent = agent
	exprLookups.device = device
}

// currentExprLookups 는 현재 설정된 룩업을 반환한다(읽기 전용 스냅샷).
func currentExprLookups() (AgentInfoLookup, DeviceInfoLookup) {
	exprLookups.mu.RLock()
	defer exprLookups.mu.RUnlock()
	return exprLookups.agent, exprLookups.device
}

// registerLookupBuiltins 는 룩업이 설정되어 있으면 agentInfo/deviceInfo 빌트인을
// funcs 맵에 추가한다. defaultBuiltinFuncs 가 마지막에 호출한다.
func registerLookupBuiltins(funcs map[string]BuiltinFunc) {
	agentLookup, deviceLookup := currentExprLookups()
	if agentLookup != nil {
		funcs["agentInfo"] = func(args []any) (any, error) {
			return lookupBuiltin("agentInfo", args, func(id string) (RegistryMeta, bool) {
				return agentLookup.LookupAgent(id)
			})
		}
	}
	if deviceLookup != nil {
		funcs["deviceInfo"] = func(args []any) (any, error) {
			return lookupBuiltin("deviceInfo", args, func(id string) (RegistryMeta, bool) {
				return deviceLookup.LookupDevice(id)
			})
		}
	}
}

// lookupBuiltin 은 agentInfo/deviceInfo 의 공통 본체이다.
//
// 인자: 정확히 1개의 문자열 id.
// 반환: 조회 성공 시 {type,id,name} map[string]any (비어있지 않은 필드만).
//
//	조회 실패(not-found) 또는 빈 id 는 nil 을 반환한다(에러 아님) — 표현식에서
//	nil 병합/분기가 자연스럽도록.
func lookupBuiltin(name string, args []any, lookup func(string) (RegistryMeta, bool)) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: %s expects 1 arg, got %d", ErrArgumentCount, name, len(args))
	}
	id, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: %s expects string id, got %T", ErrTypeMismatch, name, args[0])
	}
	if id == "" {
		return nil, nil
	}
	meta, found := lookup(id)
	if !found {
		return nil, nil
	}
	// id 는 항상 소스 인자 우선.
	meta.ID = id
	out := make(map[string]any, 3)
	if meta.Type != "" {
		out["type"] = meta.Type
	}
	out["id"] = meta.ID
	if meta.Name != "" {
		out["name"] = meta.Name
	}
	return out, nil
}
