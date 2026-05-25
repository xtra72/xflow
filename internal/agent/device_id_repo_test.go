// device_id_repo_test.go (SPEC-DEVICE-IDENTITY-001 Phase A — M1)
//
// ResolveDeviceID 의 동작과 graceful degradation 경고 로그를 검증한다.

package agent

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDeviceIDRepo 는 in-memory DeviceIDRepository 구현이다.
// (storage 패키지에 의존하지 않도록 self-contained 한 테스트 더블.)
type fakeDeviceIDRepo struct {
	mu    sync.Mutex
	store map[string]string
}

func newFakeDeviceIDRepo() *fakeDeviceIDRepo {
	return &fakeDeviceIDRepo{store: make(map[string]string)}
}

func (r *fakeDeviceIDRepo) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := agentName + ":" + unitID
	if v, ok := r.store[key]; ok {
		return v, nil
	}
	v := "uuid-" + key
	r.store[key] = v
	return v, nil
}

func (r *fakeDeviceIDRepo) Get(_ context.Context, agentName, unitID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store[agentName+":"+unitID], nil
}

// withTestSlogHandler 는 테스트 동안 slog 의 기본 핸들러를 buf 에 캡처하는
// 핸들러로 바꾼다. 테스트 종료 시 원복.
func withTestSlogHandler(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return buf
}

func TestResolveDeviceID_WithRepository(t *testing.T) {
	original := GetDeviceIDRepository()
	SetDeviceIDRepository(newFakeDeviceIDRepo())
	t.Cleanup(func() { SetDeviceIDRepository(original) })

	id1 := ResolveDeviceID(context.Background(), "lgcnp", "81")
	assert.Equal(t, "uuid-lgcnp:81", id1)

	// idempotent
	id1Again := ResolveDeviceID(context.Background(), "lgcnp", "81")
	assert.Equal(t, id1, id1Again)

	// uniqueness
	id2 := ResolveDeviceID(context.Background(), "lgcnp", "82")
	assert.NotEqual(t, id1, id2)
}

func TestResolveDeviceID_WithoutRepository_EmptyReturn(t *testing.T) {
	original := GetDeviceIDRepository()
	SetDeviceIDRepository(nil)
	t.Cleanup(func() { SetDeviceIDRepository(original) })

	resetDeviceIDRepoMissingWarnForTest()

	uid := ResolveDeviceID(context.Background(), "anyagent", "anyunit")
	assert.Empty(t, uid, "ResolveDeviceID must return empty when repo is nil (graceful degradation)")
}

// TestResolveDeviceID_WithoutRepository_WarnOnce — 저장소 미설정 시 첫 호출에
// 한 번만 경고 로그가 남아야 한다 (sync.Once 동작 검증).
func TestResolveDeviceID_WithoutRepository_WarnOnce(t *testing.T) {
	original := GetDeviceIDRepository()
	SetDeviceIDRepository(nil)
	t.Cleanup(func() { SetDeviceIDRepository(original) })

	resetDeviceIDRepoMissingWarnForTest()
	buf := withTestSlogHandler(t)

	// 첫 호출: 경고 로그 발생.
	_ = ResolveDeviceID(context.Background(), "agent-a", "unit-1")

	logsAfterFirst := buf.String()
	assert.Contains(t, logsAfterFirst, "DeviceIDRepository not configured",
		"first ResolveDeviceID call with nil repo must emit a warning")
	require.Equal(t, 1, strings.Count(logsAfterFirst, "DeviceIDRepository not configured"))

	// 추가 100 회 호출: 추가 경고 없음 (sync.Once 보호).
	for i := 0; i < 100; i++ {
		_ = ResolveDeviceID(context.Background(), "agent-a", "unit-2")
	}
	logsAfterRepeat := buf.String()
	assert.Equal(t, 1, strings.Count(logsAfterRepeat, "DeviceIDRepository not configured"),
		"warning must be emitted at most once (sync.Once spam protection)")
}
