package xsfm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
)

// ---------------------------------------------------------------------------
// B7 — 제어 감사 (acceptance.md Module 6: Scenario 6.2 / 6.3)
// ---------------------------------------------------------------------------

// mockAuditRepo 는 storage.RemoteAuditRepository 의 테스트용 목이다. Append 호출을 기록하여
// 감사 레코드 기록을 관측한다. failAppend 가 true 면 Append 가 에러를 반환한다(best-effort 검증).
type mockAuditRepo struct {
	mu         sync.Mutex
	appended   []storage.RemoteAuditRecord
	failAppend bool
}

// 컴파일 타임 인터페이스 체크.
var _ storage.RemoteAuditRepository = (*mockAuditRepo)(nil)

func newMockAuditRepo() *mockAuditRepo { return &mockAuditRepo{} }

func (m *mockAuditRepo) Append(_ context.Context, rec storage.RemoteAuditRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAppend {
		return fmt.Errorf("mock audit append failed")
	}
	m.appended = append(m.appended, rec)
	return nil
}

func (m *mockAuditRepo) List(_ context.Context, _ string, _, _ int) ([]storage.RemoteAuditRecord, error) {
	return m.records(), nil
}

func (m *mockAuditRepo) Close() error { return nil }

// records 는 기록된 감사 레코드의 복사본을 락 하에 반환한다.
func (m *mockAuditRepo) records() []storage.RemoteAuditRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.RemoteAuditRecord, len(m.appended))
	copy(out, m.appended)
	return out
}

// Scenario 6.2: 개별 제어 감사 기록 (who/when/what/device_id 를 포함한 레코드).
func TestControlAudit_IndividualRecorded(t *testing.T) {
	repo := newMockAuditRepo()
	SetAuditRepository(repo)
	defer SetAuditRepository(nil)

	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil)) // ap-101 config 시드, timeout 0(fire-and-forget)

	before := time.Now().UnixMilli()
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)

	recs := repo.records()
	require.Len(t, recs, 1, "개별 제어는 감사 레코드 1건")
	r := recs[0]
	assert.Equal(t, "ap-101", r.InstanceID, "대상 device_id")
	assert.Equal(t, "set_power", r.CommandAction, "제어 명령(what)")
	assert.Equal(t, storage.AuditActionCommand, r.Action)
	assert.Equal(t, "device", r.Domain)
	assert.Equal(t, storage.AuditResultOK, r.Result)
	assert.NotEmpty(t, r.Actor, "who(에이전트 식별자)")
	assert.GreaterOrEqual(t, r.Timestamp, before, "when(epoch ms)")
}

// Scenario 6.3: 그룹 제어 감사 기록 (대상 group_id + fan-out 멤버 목록 포함).
func TestGroupAudit_SelectorAndMembersRecorded(t *testing.T) {
	repo := newMockAuditRepo()
	SetAuditRepository(repo)
	defer SetAuditRepository(nil)

	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-103", "group_id": "concourse-b1"},
	)
	ap, _ := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"set_power","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err)

	recs := repo.records()

	// 그룹 요약 레코드: InstanceID=셀렉터 값, Reason 에 group_id + 멤버 목록.
	var group *storage.RemoteAuditRecord
	for i := range recs {
		if recs[i].InstanceID == "concourse-b1" {
			r := recs[i]
			group = &r
			break
		}
	}
	require.NotNil(t, group, "그룹 요약 감사 레코드가 있어야 한다")
	assert.Equal(t, "group_id", group.CommandAction, "셀렉터 종류")
	assert.Equal(t, storage.AuditResultOK, group.Result)
	assert.Contains(t, group.Reason, "concourse-b1", "대상 group_id")
	for _, id := range []string{"ap-101", "ap-102", "ap-103"} {
		assert.Contains(t, group.Reason, id, "fan-out 멤버 목록")
	}

	// 멤버별 개별 감사도 각각 남는다(3건, device_id 단위 감사).
	perMember := 0
	for _, r := range recs {
		switch r.InstanceID {
		case "ap-101", "ap-102", "ap-103":
			perMember++
		}
	}
	assert.Equal(t, 3, perMember, "멤버별 device_id 감사 3건")
}

// 감사 저장소 미설정 시 제어가 crash 없이 정상 동작한다(nil-safe graceful no-op).
func TestControlAudit_NilRepoGracefulNoOp(t *testing.T) {
	SetAuditRepository(nil) // 명시적으로 미설정
	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":false}}`))
	require.NoError(t, err, "감사 저장소 미설정에서도 제어는 성공")
}

// 감사 기록 실패는 제어 명령을 실패시키지 않는다(best-effort logging).
func TestControlAudit_AppendErrorBestEffort(t *testing.T) {
	repo := newMockAuditRepo()
	repo.failAppend = true
	SetAuditRepository(repo)
	defer SetAuditRepository(nil)

	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err, "감사 Append 실패해도 제어는 성공(best-effort)")
}
