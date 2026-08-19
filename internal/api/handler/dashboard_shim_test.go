// @SPEC:SPEC-DASHBOARD-004 (M4, acceptance.md AC-12)
// dashboard_shim_test.go — 원격 노드 프록시가 소비하는 레거시 형상 합성 검증.
//
// 이 경로가 깨지면 원격 대시보드 조회가 조용히 죽는다. 원격 노드는 본 SPEC 미적용
// 버전이 혼재할 수 있어 서버가 형상을 바꿀 수 없으므로, 합성 결과가 레거시 DTO 로
// 그대로 역직렬화되는지 고정한다.

package handler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
)

// newShimRepo 는 신규 스키마만 가진 저장소를 만든다.
func newShimRepo(t *testing.T) storage.DashboardRepository {
	t.Helper()
	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "shim.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo, err := storage.NewDashboardEntitySQLiteRepository(ctx, db)
	require.NoError(t, err)
	return repo
}

// mustCreate 는 대시보드 1장을 만든다.
func mustCreate(t *testing.T, repo storage.DashboardRepository, uid, name, owner, visibility string, payload string) {
	t.Helper()
	_, err := repo.Create(context.Background(), storage.Dashboard{
		UID: uid, Name: name, Owner: owner, Visibility: visibility,
		Payload: []byte(payload),
	})
	require.NoError(t, err)
}

// TestDashboardSnapshotShim_Get 은 (scope, owner) 별 합성 범위를 고정한다.
func TestDashboardSnapshotShim_Get(t *testing.T) {
	repo := newShimRepo(t)
	mustCreate(t, repo, "s1", "공유", "root", "shared",
		`{"panels":[{"id":"p1"}],"layout":[],"gridCols":12,"showGridLines":true,"refreshInterval":5000}`)
	mustCreate(t, repo, "a1", "acl 대상", "root", "acl", `{"panels":[],"layout":[]}`)
	mustCreate(t, repo, "p1", "root 개인", "root", "private", `{"panels":[],"layout":[]}`)
	mustCreate(t, repo, "p2", "alice 개인", "alice", "private", `{"panels":[],"layout":[]}`)

	shim := NewDashboardSnapshotShim(repo)

	t.Run("global 은 private 을 제외한 전량", func(t *testing.T) {
		snap, err := shim.Get(context.Background(), "global", "")
		require.NoError(t, err)
		require.NotNil(t, snap)
		assert.Equal(t, "global", snap.Scope)

		ids := snapshotPageIDs(t, snap)
		assert.ElementsMatch(t, []string{"s1", "a1"}, ids)
	})

	t.Run("user 는 해당 소유자의 private 만", func(t *testing.T) {
		snap, err := shim.Get(context.Background(), "user", "alice")
		require.NoError(t, err)
		assert.Equal(t, "user", snap.Scope)
		assert.Equal(t, "alice", snap.Owner)
		assert.Equal(t, []string{"p2"}, snapshotPageIDs(t, snap))
	})

	t.Run("대시보드가 0장이어도 not-found 가 아니라 빈 묶음", func(t *testing.T) {
		snap, err := shim.Get(context.Background(), "user", "nobody")
		require.NoError(t, err, "빈 상태를 오류로 만들면 원격 조회가 502 가 된다")
		require.NotNil(t, snap)
		assert.Empty(t, snapshotPageIDs(t, snap))
		assert.NotZero(t, snap.UpdatedAt)
	})
}

// TestDashboardSnapshotShim_LegacyDTORoundTrip 은 합성 결과가 DashboardSnapshotToDTO
// (시그니처 고정)를 통과해 레거시 응답 키를 유지하는지 고정한다.
func TestDashboardSnapshotShim_LegacyDTORoundTrip(t *testing.T) {
	repo := newShimRepo(t)
	mustCreate(t, repo, "s1", "공유", "root", "shared",
		`{"panels":[{"id":"p1"}],"layout":[{"i":"p1"}],"gridCols":12,"showGridLines":false,"refreshInterval":3000}`)

	snap, err := NewDashboardSnapshotShim(repo).Get(context.Background(), "global", "")
	require.NoError(t, err)

	// 원격 프록시(cmd/xflowd/remote_query.go)가 하는 것과 동일한 변환.
	raw, err := json.Marshal(DashboardSnapshotToDTO(snap))
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))
	for _, key := range []string{"scope", "owner", "version", "updatedAt", "payload"} {
		assert.Contains(t, got, key, "레거시 키 %s 가 사라지면 원격 프록시가 깨진다", key)
	}
	assert.JSONEq(t, `null`, string(got["owner"]), "global 은 owner=null")

	var payload struct {
		DashboardPages []struct {
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			IsDefault bool            `json:"isDefault"`
			Panels    json.RawMessage `json:"panels"`
			Layout    json.RawMessage `json:"layout"`
		} `json:"dashboardPages"`
		ActiveDashboardID string          `json:"activeDashboardId"`
		GridCols          json.RawMessage `json:"dashboardGridCols"`
		ShowGridLines     json.RawMessage `json:"dashboardShowGridLines"`
		RefreshInterval   json.RawMessage `json:"dashboardRefreshInterval"`
		DeviceGridLayout  json.RawMessage `json:"deviceGridLayout"`
	}
	require.NoError(t, json.Unmarshal(got["payload"], &payload))
	require.Len(t, payload.DashboardPages, 1)
	assert.Equal(t, "s1", payload.DashboardPages[0].ID)
	assert.Equal(t, "공유", payload.DashboardPages[0].Name)
	assert.JSONEq(t, `[{"id":"p1"}]`, string(payload.DashboardPages[0].Panels))
	assert.JSONEq(t, `[{"i":"p1"}]`, string(payload.DashboardPages[0].Layout))

	// 그리드 설정 3종은 접두사 있는 레거시 키로 되돌아온다.
	assert.JSONEq(t, `12`, string(payload.GridCols))
	assert.JSONEq(t, `false`, string(payload.ShowGridLines))
	assert.JSONEq(t, `3000`, string(payload.RefreshInterval))
	assert.JSONEq(t, `{}`, string(payload.DeviceGridLayout))
}

// TestSynthesizeDashboardSnapshot_BrokenPayloadDoesNotFail 은 payload 가 깨져 있어도
// 읽기 전용 경로가 죽지 않음을 고정한다.
func TestSynthesizeDashboardSnapshot_BrokenPayloadDoesNotFail(t *testing.T) {
	snap := SynthesizeDashboardSnapshot("global", "", []storage.Dashboard{
		{UID: "broken", Name: "깨진 payload", Version: 3, UpdatedAt: 100, Payload: []byte(`not json`)},
	})
	require.NotNil(t, snap)
	assert.EqualValues(t, 3, snap.Version)

	pages := snapshotPageIDs(t, snap)
	assert.Equal(t, []string{"broken"}, pages)
}

// snapshotPageIDs 는 합성된 스냅샷의 dashboardPages id 목록을 뽑는다.
func snapshotPageIDs(t *testing.T, snap *storage.DashboardSnapshot) []string {
	t.Helper()
	var payload struct {
		DashboardPages []struct {
			ID string `json:"id"`
		} `json:"dashboardPages"`
	}
	require.NoError(t, json.Unmarshal(snap.Payload, &payload))
	out := make([]string, 0, len(payload.DashboardPages))
	for _, p := range payload.DashboardPages {
		out = append(out, p.ID)
	}
	return out
}
