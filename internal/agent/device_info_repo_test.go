// device_info_repo_test.go (SPEC-DEVICE-IDENTITY-001 — device_id 정규화)
//
// device_info 의 읽기(Get)·쓰기(Set)·삭제(Delete) 키가 agentRef 정규화를 거쳐
// 이름 호출과 ID 호출 간에 일치함을 검증한다.

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeviceInfo_NameAndIDShareKey 는 이름으로 Set 한 device_info 를 ID 로 Get
// 할 수 있어야 함을 검증한다 (읽기/쓰기 키 일치).
func TestDeviceInfo_NameAndIDShareKey(t *testing.T) {
	const (
		agentName = "Samsung NASA"
		agentID   = "c1d2e3f4-aaaa-bbbb-cccc-ddddeeeeffff"
		unitID    = "0x20"
	)

	originalResolver := GetAgentIDResolver()
	SetAgentIDResolver(fakeAgentIDResolver(map[string]string{agentName: agentID}))
	t.Cleanup(func() {
		SetAgentIDResolver(originalResolver)
		DeleteDeviceInfo(agentID, unitID)
	})

	want := DeviceInfo{DeviceType: "HVACR.IDU", Label: "거실"}

	// 이름으로 등록 (에이전트 discovery 경로).
	SetDeviceInfo(agentName, unitID, want)

	// ID 로 조회 (노드 build 경로, engine.go 가 ID 를 주입).
	got, ok := GetDeviceInfo(agentID, unitID)
	assert.True(t, ok, "ID 로 조회 시 이름으로 등록한 device_info 를 찾아야 한다")
	assert.Equal(t, want, got)

	// 이름으로 조회해도 동일.
	gotByName, okByName := GetDeviceInfo(agentName, unitID)
	assert.True(t, okByName)
	assert.Equal(t, want, gotByName)

	// ID 로 삭제하면 이름으로도 사라져야 한다 (동일 키).
	DeleteDeviceInfo(agentID, unitID)
	_, okAfterDelete := GetDeviceInfo(agentName, unitID)
	assert.False(t, okAfterDelete, "ID 로 삭제하면 이름으로도 조회되지 않아야 한다")
}

// TestDeviceInfo_ResolverNilFallback 는 resolver 미주입 시 ref 를 그대로
// 키로 사용하여 기존 동작이 유지됨을 검증한다.
func TestDeviceInfo_ResolverNilFallback(t *testing.T) {
	originalResolver := GetAgentIDResolver()
	SetAgentIDResolver(nil)
	t.Cleanup(func() {
		SetAgentIDResolver(originalResolver)
		DeleteDeviceInfo("ref-x", "u1")
	})

	want := DeviceInfo{DeviceType: "HVACR.ODU"}
	SetDeviceInfo("ref-x", "u1", want)

	got, ok := GetDeviceInfo("ref-x", "u1")
	assert.True(t, ok)
	assert.Equal(t, want, got)
}
