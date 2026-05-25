// provider_uid_test.go (SPEC-DEVICE-IDENTITY-001 Phase A — A-AC1)
//
// 본 테스트는 centuryDeviceAdapter.UID() 가 다음을 만족함을 검증한다:
//
//   - 저장소 설정 시 UUID v4 반환 (idempotent, unique per sub_dev_id).
//   - 저장소 미설정 시 빈 문자열 (graceful degradation).
//   - localID 형식이 "0x%02X" — CenturyAgent.emit 경로의 ResolveDeviceID
//     호출과 정확히 일치하여 emit payload 의 device_id 와 UID() 가 같은
//     UUID 를 반환함을 보장.

package century

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/storage"
)

func TestCenturyAdapter_UID_WithRepository(t *testing.T) {
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(storage.NewDeviceIDMemoryRepository())
	t.Cleanup(func() { agent.SetDeviceIDRepository(original) })

	snap := CenturyDeviceSnapshot{
		SubDevID: 0x81,
		Label:    "indoor-1",
		Source:   "auto",
		Online:   true,
		LastSeen: time.Now(),
		State:    &CenturyDeviceState{},
	}
	a := newCenturyDeviceAdapter("century", snap)

	uid1 := a.UID()
	require.NotEmpty(t, uid1, "UID() must not be empty when repository is configured")
	assert.Len(t, uid1, 36, "UID must be 36-char UUID v4 string")

	// idempotent
	assert.Equal(t, uid1, a.UID(), "UID() must be idempotent")

	// 다른 sub_dev_id 는 다른 UUID
	snap2 := snap
	snap2.SubDevID = 0x82
	a2 := newCenturyDeviceAdapter("century", snap2)
	uid2 := a2.UID()
	require.NotEmpty(t, uid2)
	assert.NotEqual(t, uid1, uid2, "different sub_dev_id must yield different UUIDs")
}

func TestCenturyAdapter_UID_WithoutRepository(t *testing.T) {
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(nil)
	t.Cleanup(func() { agent.SetDeviceIDRepository(original) })

	snap := CenturyDeviceSnapshot{
		SubDevID: 0x81,
		Label:    "indoor-1",
		Source:   "auto",
		Online:   true,
		LastSeen: time.Now(),
		State:    &CenturyDeviceState{},
	}
	a := newCenturyDeviceAdapter("century", snap)

	assert.Empty(t, a.UID(), "UID() must be empty when repository is unconfigured (graceful degradation)")
}

// TestCenturyAdapter_UID_MatchesEmitPath — Century 의 emit 경로
// (agent.go:738, 771, 1451) 가 "0x%02X" 형식의 unitID 로 ResolveDeviceID
// 를 호출하므로, UID() 도 같은 형식을 사용해야 같은 UUID 가 반환된다.
// 이 invariant 가 깨지면 emit payload 의 device_id 와 API 응답의 uid 가
// 서로 다른 값이 되어 downstream 의 키 매핑이 단절된다.
func TestCenturyAdapter_UID_MatchesEmitPath(t *testing.T) {
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(storage.NewDeviceIDMemoryRepository())
	t.Cleanup(func() { agent.SetDeviceIDRepository(original) })

	snap := CenturyDeviceSnapshot{
		SubDevID: 0x3B,
		Label:    "indoor-3b",
		Source:   "auto",
	}
	a := newCenturyDeviceAdapter("century", snap)

	uidFromAdapter := a.UID()
	require.NotEmpty(t, uidFromAdapter)

	// emit 경로의 ResolveDeviceID 호출 시뮬레이션 (agent.go:738 패턴).
	uidFromEmit := agent.ResolveDeviceID(context.Background(), "century", "0x3B")
	require.NotEmpty(t, uidFromEmit)

	assert.Equal(t, uidFromAdapter, uidFromEmit,
		"centuryDeviceAdapter.UID() must yield the same UUID as the emit path's "+
			"ResolveDeviceID(ctx, agentName, \"0x%%02X\") call")
}
