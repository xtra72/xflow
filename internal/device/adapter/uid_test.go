// uid_test.go (SPEC-DEVICE-IDENTITY-001 Phase A — A-AC1, A-AC2)
//
// 본 테스트는 ResolveAdapterUID 헬퍼와 각 device.Device 어댑터의 UID() 메서드가
// 다음을 만족함을 검증한다:
//
//   - A-AC1: DeviceIDRepository 설정 시 UID() 는 빈 문자열이 아닌 UUID v4 를
//     반환하며, 같은 (agentName, localID) 의 반복 호출은 idempotent
//     (항상 동일한 UUID 반환). 다른 (agentName, localID) 는 서로 다른 UUID.
//   - A-AC2: DeviceIDRepository 미설정 시 UID() 는 빈 문자열을 반환 (graceful
//     degradation). xflowd_device_uid_missing_total 카운터가 호출당 1 증가.
//
// 본 테스트는 in-package 이므로 ResolveAdapterUID 의 직접 호출과 각 어댑터의
// UID() 호출을 모두 검증한다.

package adapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/internal/storage"
)

// withRepository 는 테스트 동안 패키지-레벨 DeviceIDRepository 를 r 로 설정하고,
// 테스트 종료 시 원복하는 헬퍼이다. 패키지-레벨 전역 상태를 가진 도메인
// (agent.SetDeviceIDRepository) 의 테스트에 사용된다.
func withRepository(t *testing.T, r agent.DeviceIDRepository) {
	t.Helper()
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(r)
	t.Cleanup(func() {
		agent.SetDeviceIDRepository(original)
	})
}

// uuidV4Re 는 standard UUID v4 텍스트 형식의 길이 검증용 (간소).
// 정확한 패턴은 storage.DeviceIDMemoryRepository 가 google/uuid 로 생성하므로
// 길이만 확인한다.
func isUUIDv4Shape(s string) bool {
	// 8-4-4-4-12 = 36 chars total
	if len(s) != 36 {
		return false
	}
	// dashes at positions 8, 13, 18, 23
	return s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

// TestResolveAdapterUID_WithRepository — A-AC1 의 핵심: 저장소 설정 시 안정적
// UUID 발급과 idempotency, uniqueness 검증.
func TestResolveAdapterUID_WithRepository(t *testing.T) {
	withRepository(t, storage.NewDeviceIDMemoryRepository())

	// 첫 호출 — UUID 발급.
	uid1 := ResolveAdapterUID("lg_hvacr01", "81")
	require.NotEmpty(t, uid1, "ResolveAdapterUID should not return empty when repository is configured")
	assert.True(t, isUUIDv4Shape(uid1), "uid should be UUID v4 shape: %q", uid1)

	// 같은 (agent, localID) — idempotent.
	uid1Again := ResolveAdapterUID("lg_hvacr01", "81")
	assert.Equal(t, uid1, uid1Again, "ResolveAdapterUID must be idempotent for same (agent, localID)")

	// 같은 agent, 다른 localID — uniqueness.
	uid2 := ResolveAdapterUID("lg_hvacr01", "82")
	require.NotEmpty(t, uid2)
	assert.NotEqual(t, uid1, uid2, "different localIDs in same agent must yield different UUIDs")

	// 다른 agent, 같은 localID — uniqueness (composite namespace).
	uid3 := ResolveAdapterUID("samsung", "81")
	require.NotEmpty(t, uid3)
	assert.NotEqual(t, uid1, uid3, "same localID in different agents must yield different UUIDs")
}

// TestResolveAdapterUID_WithoutRepository — A-AC2: 저장소 미설정 시 graceful
// degradation. 빈 문자열 반환 + xflowd_device_uid_missing_total 증가.
func TestResolveAdapterUID_WithoutRepository(t *testing.T) {
	withRepository(t, nil)

	before := observe.CollectDeviceUIDMissing()
	beforeCount := before["test-agent"]

	uid := ResolveAdapterUID("test-agent", "unit-1")
	assert.Empty(t, uid, "UID must be empty when repository is unconfigured")

	after := observe.CollectDeviceUIDMissing()
	afterCount := after["test-agent"]
	assert.Equal(t, beforeCount+1, afterCount,
		"xflowd_device_uid_missing_total{agent_name=test-agent} must increase by 1 on missing UID")
}

// TestResolveAdapterUID_EmptyInputs — 빈 agentName / localID 는 메트릭에
// 기록하지 않고 즉시 "" 반환 (호출자 보호).
func TestResolveAdapterUID_EmptyInputs(t *testing.T) {
	withRepository(t, storage.NewDeviceIDMemoryRepository())

	assert.Empty(t, ResolveAdapterUID("", "unit-1"), "empty agentName must return empty UID")
	assert.Empty(t, ResolveAdapterUID("agent", ""), "empty localID must return empty UID")
	assert.Empty(t, ResolveAdapterUID("", ""), "both empty must return empty UID")
}

// TestAdapters_UID_AllTypes — 모든 HVAC 어댑터의 UID() 메서드가 일관되게
// 동작함을 검증한다. NASA (Samsung + LGAP), LG ICP-01, LGCP, Modbus 4 어댑터.
// Century 어댑터는 별도 패키지에 위치하므로 century 패키지의 테스트에서 검증.
func TestAdapters_UID_AllTypes(t *testing.T) {
	repo := storage.NewDeviceIDMemoryRepository()
	withRepository(t, repo)

	now := time.Now()

	t.Run("NASADeviceAdapter", func(t *testing.T) {
		d := NewNASADevice("samsung", NASADeviceInfo{
			Address:    "20.01.00",
			DeviceType: "HVACR.IDU",
			Online:     true,
			LastSeen:   now,
		})
		var _ device.Device = d
		uid := d.UID()
		require.NotEmpty(t, uid)
		assert.True(t, isUUIDv4Shape(uid))
		// idempotent
		assert.Equal(t, uid, d.UID())
	})

	t.Run("Icp01DeviceAdapter", func(t *testing.T) {
		d := NewIcp01Device("lg_hvacr01", Icp01DeviceInfo{
			Address:    "81",
			DeviceType: "HVACR.IDU",
			Online:     true,
			LastSeen:   now,
		})
		var _ device.Device = d
		uid := d.UID()
		require.NotEmpty(t, uid)
		assert.True(t, isUUIDv4Shape(uid))
		assert.Equal(t, uid, d.UID())
	})

	t.Run("LGCPDeviceAdapter", func(t *testing.T) {
		d := NewLGCPDevice("lgcp", LGCPDeviceInfo{
			Address:    "44550067",
			DeviceType: "HVACR.IDU",
			Online:     true,
			LastSeen:   now,
		})
		var _ device.Device = d
		uid := d.UID()
		require.NotEmpty(t, uid)
		assert.True(t, isUUIDv4Shape(uid))
		assert.Equal(t, uid, d.UID())
	})

	t.Run("ModbusDeviceAdapter", func(t *testing.T) {
		d := NewModbusDevice("modbus", ModbusDeviceInfo{
			DeviceID: "device-1",
			Host:     "192.168.1.10",
			Port:     502,
			UnitID:   1,
			Online:   true,
			LastSeen: now,
		})
		var _ device.Device = d
		uid := d.UID()
		require.NotEmpty(t, uid)
		assert.True(t, isUUIDv4Shape(uid))
		assert.Equal(t, uid, d.UID())
	})
}

// TestAdapter_UID_MatchesEmitPath — emit 경로의 ResolveDeviceID 호출과 UID()
// 가 같은 UUID 를 반환함을 검증. 이는 SPEC-DEVICE-IDENTITY-001 의 핵심 보장
// (emit payload 의 device_id 와 API 응답의 uid 가 동일) 의 회귀 방지.
func TestAdapter_UID_MatchesEmitPath(t *testing.T) {
	repo := storage.NewDeviceIDMemoryRepository()
	withRepository(t, repo)

	// 어댑터의 UID() 호출.
	d := NewNASADevice("samsung", NASADeviceInfo{
		Address:    "20.01.00",
		DeviceType: "HVACR.IDU",
	})
	uidFromAdapter := d.UID()
	require.NotEmpty(t, uidFromAdapter)

	// emit 경로와 동일한 직접 호출.
	uidFromEmitPath := agent.ResolveDeviceID(context.Background(), "samsung", "20.01.00")
	require.NotEmpty(t, uidFromEmitPath)

	assert.Equal(t, uidFromAdapter, uidFromEmitPath,
		"Adapter.UID() and emit-path ResolveDeviceID must return the same UUID for "+
			"the same (agentName, localID); this is the SPEC-DEVICE-IDENTITY-001 invariant")
}

// TestAdapter_UID_EmptyOnMissingRepo — 모든 어댑터가 저장소 미설정 시
// 일관되게 빈 문자열을 반환함을 검증 (graceful degradation).
func TestAdapter_UID_EmptyOnMissingRepo(t *testing.T) {
	withRepository(t, nil)

	adapters := []struct {
		name string
		d    device.Device
	}{
		{"NASA", NewNASADevice("samsung", NASADeviceInfo{Address: "20.01.00"})},
		{"lg_icp01", NewIcp01Device("lg_hvacr01", Icp01DeviceInfo{Address: "81"})},
		{"LGCP", NewLGCPDevice("lgcp", LGCPDeviceInfo{Address: "44550067"})},
		{"Modbus", NewModbusDevice("modbus", ModbusDeviceInfo{DeviceID: "device-1"})},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			uid := tc.d.UID()
			assert.Empty(t, uid, "%s adapter UID() must be empty when repository is unconfigured", tc.name)
		})
	}
}

// sanity: 패키지 이름 충돌 방지용 — strings import 유지 확인.
var _ = strings.HasPrefix
