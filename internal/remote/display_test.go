// display_test.go 는 v1.6(M11, 그룹 M) 서버 측 노드 해상도(display_width/
// display_height) 보고 수신을 검증한다(@SPEC:SPEC-REMOTE-001 M11, REQ-M01~M03).
//
// 검증 범위(M9 시스템 정보 패턴 일관):
//   - register 가 해상도를 저장(REQ-M01/M02).
//   - heartbeat 가 해상도를 갱신하되, 미보고(0) 시 기존값 보존(REQ-M03 하위 호환).
//   - 미보고 노드는 0(폴백 — REQ-M03).
package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// TestServer_RegisterStoresDisplayResolution 는 register 페이로드의 해상도가
// managed_nodes 에 저장되는지 검증한다(REQ-M01/M02).
func TestServer_RegisterStoresDisplayResolution(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{
		InstanceID: "n", Hostname: "h", Version: "1.2.3",
		OS: "linux", Arch: "arm64", StartedAt: 5000,
		DisplayWidth: 1920, DisplayHeight: 1080,
	})
	_ = readAck(t, conn)

	require.Eventually(t, func() bool {
		got, err := repo.Get(context.Background(), "n")
		return err == nil && got.DisplayWidth == 1920
	}, time.Second, 10*time.Millisecond)

	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, 1920, got.DisplayWidth)
	assert.Equal(t, 1080, got.DisplayHeight)
}

// TestServer_HeartbeatUpdatesDisplayPreservesOmitted 는 heartbeat 가 제공된 해상도를
// 갱신하되, 후속 heartbeat 가 생략하면 기존값을 보존하는지 검증한다(REQ-M03 하위 호환).
func TestServer_HeartbeatUpdatesDisplayPreservesOmitted(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "n", Status: RegStatusApproved, TokenID: "jti"}))

	conn := newFakeConn()
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = srv.HandleConnectionAuth(connCtx, conn, "n") }()

	// 첫 heartbeat: 해상도 포함.
	hb1, _ := NewHeartbeatMessageWithInfo("n", "linux", "amd64", "1.0.0", 7000, 1920, 1080)
	conn.inject(t, hb1)
	require.Eventually(t, func() bool {
		got, err := repo.Get(ctx, "n")
		return err == nil && got.DisplayWidth == 1920
	}, time.Second, 10*time.Millisecond)

	// 둘째 heartbeat: 해상도 생략(구버전 노드 모사) → 기존값 보존(REQ-M03).
	hb2, _ := NewHeartbeatMessage("n")
	conn.inject(t, hb2)
	time.Sleep(50 * time.Millisecond)
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, 1920, got.DisplayWidth, "생략된 display_width 는 기존값 보존")
	assert.Equal(t, 1080, got.DisplayHeight, "생략된 display_height 는 기존값 보존")
}

// TestServer_RegisterWithoutDisplayBackwardCompat 는 해상도를 보고하지 않는 구버전
// 노드도 등록이 정상 동작하고 해상도가 0(미보고)인지 검증한다(REQ-M03 회귀 0).
func TestServer_RegisterWithoutDisplayBackwardCompat(t *testing.T) {
	srv, repo, _ := newGroupingServer(t)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{
		InstanceID: "n", Hostname: "h", Version: "1.2.3",
	})
	_ = readAck(t, conn)

	require.Eventually(t, func() bool {
		_, err := repo.Get(context.Background(), "n")
		return err == nil
	}, time.Second, 10*time.Millisecond)

	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, 0, got.DisplayWidth, "미보고 노드는 display_width 0")
	assert.Equal(t, 0, got.DisplayHeight, "미보고 노드는 display_height 0")
}
