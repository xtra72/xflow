// agent_id_resolver_test.go (SPEC-DEVICE-IDENTITY-001 — device_id 정규화)
//
// 재현 우선(reproduction-first) 테스트: agentRef 가 에이전트 "이름" 으로 호출되든
// "ID"(UUID) 로 호출되든 동일한 device_id / device_info 키로 정규화되어야 한다.
//
// 정규화 도입 전: 이름 호출과 ID 호출이 서로 다른 device_id 를 반환하여
// 같은 물리 디바이스에 두 개의 UUID 가 발급되는 버그를 재현한다.

package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeAgentIDResolver 는 이름 → ID 매핑을 가진 테스트용 resolver 를 만든다.
// 등록된 이름이면 해당 ID 와 true 를, 아니면 빈 문자열과 false 를 반환한다.
func fakeAgentIDResolver(nameToID map[string]string) AgentIDResolver {
	return func(ref string) (string, bool) {
		id, ok := nameToID[ref]
		return id, ok
	}
}

// TestResolveDeviceID_NameAndIDProduceSameID 는 핵심 재현 테스트이다.
// "LG HVACR2"(이름) 와 "b4195fa0-..."(그 에이전트의 ID) 가 동일 unitID 에 대해
// 같은 device_id 를 반환해야 한다.
func TestResolveDeviceID_NameAndIDProduceSameID(t *testing.T) {
	const (
		agentName = "LG HVACR2"
		agentID   = "b4195fa0-1111-2222-3333-444455556666"
		unitID    = "44550065"
	)

	originalRepo := GetDeviceIDRepository()
	originalResolver := GetAgentIDResolver()
	SetDeviceIDRepository(newFakeDeviceIDRepo())
	SetAgentIDResolver(fakeAgentIDResolver(map[string]string{agentName: agentID}))
	t.Cleanup(func() {
		SetDeviceIDRepository(originalRepo)
		SetAgentIDResolver(originalResolver)
	})

	// 이름으로 호출 (discovery / registry 경로 시뮬레이션).
	idByName := ResolveDeviceID(context.Background(), agentName, unitID)
	// ID 로 호출 (engine.go 가 agent_ref 에 AgentID 를 주입한 노드 경로 시뮬레이션).
	idByID := ResolveDeviceID(context.Background(), agentID, unitID)

	assert.NotEmpty(t, idByName)
	assert.NotEmpty(t, idByID)
	assert.Equal(t, idByName, idByID,
		"이름 호출과 ID 호출은 동일한 device_id 로 정규화되어야 한다")

	// 정규화 키가 ID 기준임을 확인: ID 호출이 이름 키를 만들지 않아야 한다.
	assert.Equal(t, "uuid-"+agentID+":"+unitID, idByName,
		"정규화 결과 키는 에이전트 ID 기준이어야 한다")
}

// TestResolveDeviceID_ResolverNilFallback 는 resolver 미주입(nil) 시
// 기존처럼 ref 를 그대로 키로 사용하는 graceful 폴백을 검증한다.
func TestResolveDeviceID_ResolverNilFallback(t *testing.T) {
	originalRepo := GetDeviceIDRepository()
	originalResolver := GetAgentIDResolver()
	SetDeviceIDRepository(newFakeDeviceIDRepo())
	SetAgentIDResolver(nil)
	t.Cleanup(func() {
		SetDeviceIDRepository(originalRepo)
		SetAgentIDResolver(originalResolver)
	})

	id := ResolveDeviceID(context.Background(), "some-ref", "unit-1")
	assert.Equal(t, "uuid-some-ref:unit-1", id,
		"resolver 미주입 시 ref 를 그대로 키로 사용해야 한다 (graceful 폴백)")
}

// TestResolveDeviceID_UnknownRefFallback 는 resolver 가 주입되었으나 ref 를
// 못 찾는 경우(이미 ID 이거나 미등록) ref 를 그대로 사용함을 검증한다.
func TestResolveDeviceID_UnknownRefFallback(t *testing.T) {
	const knownName = "Known Agent"
	const knownID = "known-id-aaaa"

	originalRepo := GetDeviceIDRepository()
	originalResolver := GetAgentIDResolver()
	SetDeviceIDRepository(newFakeDeviceIDRepo())
	SetAgentIDResolver(fakeAgentIDResolver(map[string]string{knownName: knownID}))
	t.Cleanup(func() {
		SetDeviceIDRepository(originalRepo)
		SetAgentIDResolver(originalResolver)
	})

	// 이미 ID 인 ref 는 resolver 에서 못 찾으므로 그대로 사용.
	id := ResolveDeviceID(context.Background(), knownID, "unit-9")
	assert.Equal(t, "uuid-"+knownID+":unit-9", id,
		"이미 ID 인 ref 는 그대로 키로 사용해야 한다")
}

// TestNormalizeAgentRef 는 정규화 함수 자체의 규칙을 직접 검증한다.
func TestNormalizeAgentRef(t *testing.T) {
	originalResolver := GetAgentIDResolver()
	SetAgentIDResolver(fakeAgentIDResolver(map[string]string{"My Agent": "agent-uuid-1"}))
	t.Cleanup(func() { SetAgentIDResolver(originalResolver) })

	assert.Equal(t, "agent-uuid-1", normalizeAgentRef("My Agent"),
		"등록된 이름은 ID 로 정규화")
	assert.Equal(t, "agent-uuid-1", normalizeAgentRef("agent-uuid-1"),
		"이미 ID 인 ref 는 그대로")
	assert.Equal(t, "unknown-ref", normalizeAgentRef("unknown-ref"),
		"미등록 ref 는 그대로 (안전 폴백)")
	assert.Equal(t, "", normalizeAgentRef(""),
		"빈 ref 는 빈 문자열")
}
