// display_override_test.go 는 v1.6(M11 확장)의 서버-측 노드 해상도 오버라이드
// (SetNodeDisplayOverride/ClearNodeDisplayOverride)와 NodeDetail 의 effective 해상도
// 파생(오버라이드>보고값)을 검증한다(@SPEC:SPEC-REMOTE-001 M11, OQ-M1 보조 override).
//
// M9 그룹 배정 패턴과 동일하게 서버 메서드는 repo 갱신만 수행하고 노드로 명령을 전파하지
// 않는다(A13 — 서버 운영 메타데이터). effective 는 NodeDetail 이 파생한다(저장 안 함).
package remote

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// TestServer_SetAndClearDisplayOverride 는 오버라이드 설정/해제가 repo 에 반영되는지
// 검증한다(서버 메서드 직접 호출 — 노드 비전파, A13).
func TestServer_SetAndClearDisplayOverride(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))

	require.NoError(t, srv.SetNodeDisplayOverride(ctx, "n", 1920, 1080))
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, 1920, got.DisplayOverrideWidth)
	assert.Equal(t, 1080, got.DisplayOverrideHeight)

	require.NoError(t, srv.ClearNodeDisplayOverride(ctx, "n"))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, 0, got.DisplayOverrideWidth, "해제 시 0 으로 환원")
	assert.Equal(t, 0, got.DisplayOverrideHeight)
}

// TestServer_SetDisplayOverrideNotFound 는 미존재 노드 오버라이드 설정 시
// ErrManagedNodeNotFound 를 반환하는지 검증한다.
func TestServer_SetDisplayOverrideNotFound(t *testing.T) {
	srv, _, _ := newGroupingServer(t)
	err := srv.SetNodeDisplayOverride(context.Background(), "missing", 1920, 1080)
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestServer_SetDisplayOverrideNoRepo 는 repo 미구성(M1 모드)에서 ErrNoRepo 를
// 반환하는지 검증한다(모드 안전).
func TestServer_SetDisplayOverrideNoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	err := srv.SetNodeDisplayOverride(context.Background(), "n", 1920, 1080)
	assert.ErrorIs(t, err, ErrNoRepo)
}

// TestServer_NodeDetailEffectiveOverride 는 오버라이드가 설정된 노드의 NodeDetail
// effective 해상도가 오버라이드를 반영하고, Node 필드로 보고 원본이 보존되는지 검증한다.
func TestServer_NodeDetailEffectiveOverride(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{
		InstanceID: "n", Status: RegStatusApproved,
		DisplayWidth: 1366, DisplayHeight: 768, // 노드 보고
	}))
	require.NoError(t, srv.SetNodeDisplayOverride(ctx, "n", 1920, 1080)) // 관리자 오버라이드

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1920, detail.EffectiveWidth, "effective 는 오버라이드 우선")
	assert.Equal(t, 1080, detail.EffectiveHeight)
	// Node 필드는 보고 원본과 오버라이드를 각각 보존한다(핸들러가 reported/override 노출에 사용).
	assert.Equal(t, 1366, detail.Node.DisplayWidth, "노드 보고값 보존")
	assert.Equal(t, 768, detail.Node.DisplayHeight)
	assert.Equal(t, 1920, detail.Node.DisplayOverrideWidth)
	assert.Equal(t, 1080, detail.Node.DisplayOverrideHeight)
}

// TestServer_NodeDetailEffectiveReportedWhenNoOverride 는 오버라이드 미설정 시
// effective 가 노드 보고값이 되는지 검증한다(하위 호환 폴백 경로).
func TestServer_NodeDetailEffectiveReportedWhenNoOverride(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{
		InstanceID: "n", Status: RegStatusApproved,
		DisplayWidth: 1366, DisplayHeight: 768,
	}))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1366, detail.EffectiveWidth, "오버라이드 없으면 effective=보고값")
	assert.Equal(t, 768, detail.EffectiveHeight)
}

// TestServer_NodeDetailEffectiveZeroWhenNeither 는 보고값·오버라이드 둘 다 없으면
// effective 가 0 인지 검증한다(프론트엔드 폴백 트리거 — REQ-M03).
func TestServer_NodeDetailEffectiveZeroWhenNeither(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 0, detail.EffectiveWidth, "둘 다 없으면 0(프론트 폴백)")
	assert.Equal(t, 0, detail.EffectiveHeight)
}

// TestServer_DisplayOverridePreservedAcrossSystemInfo 는 노드 보고(SetSystemInfo via
// storeSystemInfo)가 관리자 오버라이드를 건드리지 않고, effective 가 여전히 오버라이드를
// 따르는지 검증한다(admin-owned 보존 — M9 group_name 패턴 일관).
func TestServer_DisplayOverridePreservedAcrossSystemInfo(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved}))
	require.NoError(t, srv.SetNodeDisplayOverride(ctx, "n", 1920, 1080))

	// 노드가 다른 해상도를 보고해도 오버라이드는 유지된다.
	require.NoError(t, repo.SetSystemInfo(ctx, "n", "linux", "arm64", 1000, 800, 600))

	detail, err := srv.NodeDetail(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, 1920, detail.EffectiveWidth, "오버라이드는 노드 보고에 의해 덮이지 않아야 함")
	assert.Equal(t, 1080, detail.EffectiveHeight)
	assert.Equal(t, 800, detail.Node.DisplayWidth, "노드 보고값은 별도 갱신")
	assert.Equal(t, 600, detail.Node.DisplayHeight)
}
