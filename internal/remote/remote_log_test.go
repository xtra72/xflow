// 원격 관리 로그와 두 시각의 계약 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 고치기 전: 화면의 "마지막 수신" 은 keep-alive 시각이 아니라 마지막 연결 시각이었다.
// `touch` 가 메모리만 고쳤기 때문에, 30초마다 하트비트를 보내는 멀쩡한 노드가
// "3일째 무소식" 으로 보였다. 아래 시험이 그 자리를 고정한다.
package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

/** 승인된 노드 하나와 감사 저장소를 갖춘 서버를 만든다. */
func newLogServer(t *testing.T, instanceID string) (*Server, *memManagedNodeRepo, *memAuditRepo) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: instanceID, Status: RegStatusApproved, Online: true,
	}))
	audit := newMemAuditRepo()
	srv := NewServer(ServerConfig{Repo: repo, TokenIssuer: newFakeTokenIssuer(), Audit: audit}, nil)
	return srv, repo, audit
}

/** 감사 저장소에서 해당 액션의 레코드만 고른다. */
func auditOf(t *testing.T, audit *memAuditRepo, action string) []storage.RemoteAuditRecord {
	t.Helper()
	all, _, err := audit.List(context.Background(), storage.RemoteAuditQuery{Limit: 1000})
	require.NoError(t, err)
	out := make([]storage.RemoteAuditRecord, 0)
	for _, r := range all {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

// 하트비트 **메시지**를 실제로 흘려보내 DB 까지 닿는지 본다.
//
// 처음에는 persistKeepAlive 를 직접 불러 검증했는데, 그러면 읽기 루프에서 그 호출을
// 통째로 지워도 시험이 통과한다(돌연변이로 확인함). 고치려는 결함이 바로 그 이음매 —
// "수신 시각이 DB 에 닿지 않는다" — 이므로 메시지부터 흘려보내야 한다.
func TestKeepAlive_HeartbeatPersistsLastSeen(t *testing.T) {
	srv, repo, _ := newLogServer(t, "n1")
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "n1", Hostname: "h", Version: "v"})
	require.NoError(t, err)
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("n1")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)

	// 연결이 남긴 last_seen 을 기준선으로 잡는다.
	base, err := repo.Get(context.Background(), "n1")
	require.NoError(t, err)

	// 영속 간격을 지난 것으로 만들어 하트비트가 쓰기를 하게 한다.
	srv.mu.Lock()
	srv.lastKeepAlivePersist["n1"] = time.Now().Add(-2 * keepAlivePersistInterval)
	srv.mu.Unlock()

	hb, err := NewHeartbeatMessage("n1")
	require.NoError(t, err)
	conn.inject(t, hb)

	require.Eventually(t, func() bool {
		after, gErr := repo.Get(context.Background(), "n1")
		return gErr == nil && after.LastSeen > base.LastSeen
	}, time.Second, 5*time.Millisecond,
		"하트비트 수신은 DB 의 last_seen 을 갱신해야 한다 — 메모리만 고치면 화면이 연결 시각에 굳는다")
}

func TestKeepAlive_ThrottlesWrites(t *testing.T) {
	srv, repo, _ := newLogServer(t, "n1")

	srv.persistKeepAlive("n1")
	first, err := repo.Get(context.Background(), "n1")
	require.NoError(t, err)

	// 간격 안의 두 번째 호출은 DB 를 다시 때리지 않는다.
	time.Sleep(5 * time.Millisecond)
	srv.persistKeepAlive("n1")
	second, err := repo.Get(context.Background(), "n1")
	require.NoError(t, err)
	require.Equal(t, first.LastSeen, second.LastSeen,
		"간격 안의 하트비트는 쓰기를 건너뛰어야 한다")
}

func TestTouchAccess_RecordsTimeAndActor(t *testing.T) {
	srv, repo, audit := newLogServer(t, "n1")

	srv.TouchAccess(context.Background(), "n1", "admin")

	node, err := repo.Get(context.Background(), "n1")
	require.NoError(t, err)
	require.Greater(t, node.LastAccessAt, int64(0))
	require.Equal(t, "admin", node.LastAccessBy)

	rows := auditOf(t, audit, storage.AuditActionAccess)
	require.Len(t, rows, 1, "접속 한 번은 로그 한 줄이다")
	require.Equal(t, "admin", rows[0].Actor)
}

func TestTouchAccess_SameSessionDoesNotFloodLog(t *testing.T) {
	srv, _, audit := newLogServer(t, "n1")

	// 화면이 5초마다 폴링하는 상황 — 창(window) 안이므로 로그는 한 줄이어야 한다.
	for i := 0; i < 20; i++ {
		srv.TouchAccess(context.Background(), "n1", "admin")
	}
	require.Len(t, auditOf(t, audit, storage.AuditActionAccess), 1,
		"열어 둔 창의 폴링이 로그를 채우면 안 된다")
}

func TestTouchAccess_IgnoresSystemCalls(t *testing.T) {
	srv, repo, audit := newLogServer(t, "n1")

	// actor 없는 내부 호출(스윕·자동 작업)은 "사람이 들여다봤다" 가 아니다.
	srv.TouchAccess(context.Background(), "n1", "")

	node, err := repo.Get(context.Background(), "n1")
	require.NoError(t, err)
	require.Equal(t, int64(0), node.LastAccessAt)
	require.Empty(t, auditOf(t, audit, storage.AuditActionAccess))
}

func TestLifecycle_ConnectAndDisconnectAreLogged(t *testing.T) {
	srv, _, audit := newLogServer(t, "n1")
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "n1", Hostname: "h", Version: "v"})
	require.NoError(t, err)
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		return len(auditOf(t, audit, storage.AuditActionConnect)) == 1
	}, time.Second, 5*time.Millisecond, "연결은 로그에 남아야 한다")

	// 연결을 끊으면 끊김이 남는다.
	_ = conn.Close()
	require.Eventually(t, func() bool {
		return len(auditOf(t, audit, storage.AuditActionDisconnect)) == 1
	}, time.Second, 5*time.Millisecond, "끊김도 로그에 남아야 한다")
}

func TestDelete_IsLoggedWithActor(t *testing.T) {
	srv, _, audit := newLogServer(t, "n1")

	ctx := ContextWithActor(context.Background(), "admin")
	require.NoError(t, srv.RemoveNode(ctx, "n1"))

	rows := auditOf(t, audit, storage.AuditActionDelete)
	require.Len(t, rows, 1, "삭제는 행이 사라지는 유일한 사건이라 기록이 없으면 흔적도 없다")
	require.Equal(t, "admin", rows[0].Actor)
}
