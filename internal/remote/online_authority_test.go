// online 판정의 권위는 keep-alive 다 — 항목의 유무가 아니다
// (@SPEC:SPEC-REMOTE-ONLINE-001).
//
// 신고: "xagent04 는 연결도 안되어 있는데 online 으로 표시됨." 재시작 이후 한 번도 붙지
// 않은 노드는 메모리 항목이 없고, 종전 오버레이는 항목이 없으면 영속값을 그대로 냈다.

package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
)

const testHeartbeatTimeout = 90 * time.Second

// newOnlineServer 는 repo 가 주입된 서버를 낸다. HeartbeatTimeout 을 명시해 최근성
// 판정의 기준을 시험이 들고 있게 한다.
func newOnlineServer(t *testing.T) (*Server, *memManagedNodeRepo) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{
		Repo:             repo,
		TokenIssuer:      newFakeTokenIssuer(),
		HeartbeatTimeout: testHeartbeatTimeout,
	}, nil)
	return srv, repo
}

// seedNode 는 영속 행 하나를 심는다. `online`/`lastSeen` 이 이 시험들의 입력이다.
func seedNode(t *testing.T, repo *memManagedNodeRepo, id string, online bool, lastSeen time.Time) {
	t.Helper()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: id,
		Status:     RegStatusApproved,
		Online:     online,
		LastSeen:   lastSeen.UnixMilli(),
	}))
}

func onlineOfList(t *testing.T, srv *Server, id string) bool {
	t.Helper()
	nodes, err := srv.ListNodes(context.Background())
	require.NoError(t, err)
	for _, n := range nodes {
		if n.InstanceID == id {
			return n.Online
		}
	}
	t.Fatalf("노드 %s 가 목록에 없다", id)
	return false
}

// --- ① 신고된 그 장면 (REQ-02) ---------------------------------------------

// TestListNodes_StalePersistedOnline_ReportsOffline 은 재시작 이후 붙지 않은 노드가
// 영속 online=1 을 들고 있어도 offline 으로 보고되는지 검증한다(REQ-02).
func TestListNodes_StalePersistedOnline_ReportsOffline(t *testing.T) {
	srv, repo := newOnlineServer(t)
	// 하트비트 타임아웃보다 오래된 last_seen — 그 사이 keep-alive 가 없었다.
	seedNode(t, repo, "xagent04", true, time.Now().Add(-10*testHeartbeatTimeout))

	// 메모리 항목이 없다 — 이번 부팅 이후 한 번도 붙지 않았다는 뜻이다.
	_, tracked := srv.NodeState("xagent04")
	require.False(t, tracked, "이 시험의 전제는 '항목이 없다' 이다")

	assert.False(t, onlineOfList(t, srv, "xagent04"),
		"keep-alive 가 끊긴 지 오래면 영속값이 무엇이든 offline 이다")
}

// TestListNodes_RecentPersistedOnline_ReportsOnline 은 최근 keep-alive 가 있었던 행은
// 항목이 없어도 online 으로 남는지 검증한다(REQ-02).
//
// 재시작 직후의 짧은 창이다 — 아직 모르는 것을 offline 으로 단정하지 않고, 노드가 다시
// 붙거나 타임아웃이 지나면 스스로 낫는다.
func TestListNodes_RecentPersistedOnline_ReportsOnline(t *testing.T) {
	srv, repo := newOnlineServer(t)
	seedNode(t, repo, "n", true, time.Now().Add(-testHeartbeatTimeout/2))
	assert.True(t, onlineOfList(t, srv, "n"))
}

// TestListNodes_PersistedOffline_StaysOffline 은 영속값이 offline 이면 최근성을 묻지
// 않는지 검증한다(REQ-02).
func TestListNodes_PersistedOffline_StaysOffline(t *testing.T) {
	srv, repo := newOnlineServer(t)
	// last_seen 이 방금이어도 online=false 면 offline 이다.
	seedNode(t, repo, "n", false, time.Now())
	assert.False(t, onlineOfList(t, srv, "n"))
}

// TestListNodes_TrackedNode_LiveWins 는 항목이 있으면 라이브 추적이 답하는지 검증한다
// (REQ-02 · K3) — 주석이 세운 그 불변식이다.
func TestListNodes_TrackedNode_LiveWins(t *testing.T) {
	srv, repo := newOnlineServer(t)
	// 영속값은 오래된 online=1 이지만, 라이브 추적은 offline 이다.
	seedNode(t, repo, "n", true, time.Now().Add(-10*testHeartbeatTimeout))
	srv.setNodeState("n", RegStatusApproved, false, time.Now())
	assert.False(t, onlineOfList(t, srv, "n"), "항목이 있으면 라이브가 이긴다")

	// 반대 방향도 라이브가 이긴다 — 영속값이 offline 인데 라이브는 online 이다.
	seedNode(t, repo, "m", false, time.Now().Add(-10*testHeartbeatTimeout))
	srv.setNodeState("m", RegStatusApproved, true, time.Now())
	assert.True(t, onlineOfList(t, srv, "m"))
}

// --- ② 견고성 (REQ-04) -----------------------------------------------------

// TestOnlineOf_Robustness 는 손상된 last_seen 에도 던지지 않는지 검증한다(REQ-04).
func TestOnlineOf_Robustness(t *testing.T) {
	srv, _ := newOnlineServer(t)
	now := time.Now()

	// `0` 은 "본 적 없음" 이다 — 오래된 것으로 읽어 offline.
	assert.False(t, srv.OnlineOf(storage.ManagedNode{InstanceID: "z", Online: true, LastSeen: 0}, now))

	// 미래 값(시계 왜곡)은 `Sub` 이 음수를 내므로 최근으로 읽힌다 — 패닉이 아니다.
	future := now.Add(365 * 24 * time.Hour).UnixMilli()
	assert.True(t, srv.OnlineOf(storage.ManagedNode{InstanceID: "z", Online: true, LastSeen: future}, now))
}

// --- ③ 청소기가 영속한다 (REQ-01 · K4) --------------------------------------

// TestSweepOnce_PersistsOffline 은 하트비트 타임아웃이 **DB 에도** 남는지 검증한다
// (REQ-01). 종전에는 메모리만 뒤집어, 다음 재시작에 online=1 이 되살아났다.
func TestSweepOnce_PersistsOffline(t *testing.T) {
	srv, repo := newOnlineServer(t)
	ctx := context.Background()
	seedNode(t, repo, "n", true, time.Now())
	// 타임아웃보다 오래된 라이브 last_seen — 청소기가 뒤집을 대상이다.
	srv.setNodeState("n", RegStatusApproved, true, time.Now().Add(-2*testHeartbeatTimeout))

	srv.sweepOnce(time.Now())

	// 메모리가 뒤집혔다.
	st, ok := srv.NodeState("n")
	require.True(t, ok)
	assert.False(t, st.Online)

	// **그리고 DB 에도 남았다** — 이 줄이 이 SPEC 이 더한 것이다.
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.False(t, got.Online, "청소기가 뒤집은 것은 영속되어야 한다")
}

// TestSweepOnce_NoTimeout_DoesNotPersist 는 뒤집을 것이 없으면 쓰지 않는지 검증한다.
func TestSweepOnce_NoTimeout_DoesNotPersist(t *testing.T) {
	srv, repo := newOnlineServer(t)
	seedNode(t, repo, "n", true, time.Now())
	srv.setNodeState("n", RegStatusApproved, true, time.Now())

	srv.sweepOnce(time.Now())

	got, err := repo.Get(context.Background(), "n")
	require.NoError(t, err)
	assert.True(t, got.Online, "타임아웃이 아니면 건드리지 않는다")
}

// TestSweepThenRestart_StaysOffline 은 ① 과 ② 가 함께 고쳐졌음을 **재시작을 흉내 내어**
// 검증한다 — 신고된 형상의 끝에서 끝이다.
func TestSweepThenRestart_StaysOffline(t *testing.T) {
	srv, repo := newOnlineServer(t)
	seedNode(t, repo, "xagent04", true, time.Now())
	srv.setNodeState("xagent04", RegStatusApproved, true, time.Now().Add(-2*testHeartbeatTimeout))
	srv.sweepOnce(time.Now())

	// **재시작**: 같은 repo 를 물린 새 서버 — 메모리는 비어 있다.
	restarted := NewServer(ServerConfig{
		Repo:             repo,
		TokenIssuer:      newFakeTokenIssuer(),
		HeartbeatTimeout: testHeartbeatTimeout,
	}, nil)
	_, tracked := restarted.NodeState("xagent04")
	require.False(t, tracked)

	assert.False(t, onlineOfList(t, restarted, "xagent04"),
		"청소기가 영속했으므로 재시작 뒤에도 offline 이다")
}

// --- ④ 판정이 한 자리다 (REQ-03 · K1) ---------------------------------------

// TestNodeDetail_SameAnswerAsList 는 상세와 목록이 **같은 답**을 내는지 검증한다
// (REQ-03). 종전에는 `NodeDetail` 이 조건부 오버레이의 사본을 들고 있었다.
func TestNodeDetail_SameAnswerAsList(t *testing.T) {
	cases := []struct {
		name     string
		online   bool
		lastSeen time.Duration
		tracked  *bool
	}{
		{name: "낡은 online — 항목 없음", online: true, lastSeen: -10 * testHeartbeatTimeout},
		{name: "최근 online — 항목 없음", online: true, lastSeen: -testHeartbeatTimeout / 2},
		{name: "영속 offline", online: false, lastSeen: 0},
		{name: "항목이 offline", online: true, lastSeen: 0, tracked: boolPtr(false)},
		{name: "항목이 online", online: false, lastSeen: -10 * testHeartbeatTimeout, tracked: boolPtr(true)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, repo := newOnlineServer(t)
			seedNode(t, repo, "n", tc.online, time.Now().Add(tc.lastSeen))
			if tc.tracked != nil {
				srv.setNodeState("n", RegStatusApproved, *tc.tracked, time.Now())
			}
			detail, err := srv.NodeDetail(context.Background(), "n")
			require.NoError(t, err)
			assert.Equal(t, onlineOfList(t, srv, "n"), detail.Online,
				"상세와 목록이 갈리면 사용자는 같은 노드를 두 상태로 본다")
			// 상세의 두 자리(`Node.Online` 과 `Online`)도 서로 같아야 한다.
			assert.Equal(t, detail.Online, detail.Node.Online)
		})
	}
}

func boolPtr(b bool) *bool { return &b }
