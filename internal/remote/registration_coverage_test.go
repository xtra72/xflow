// registration_coverage_test.go 는 등록/승인 상태 머신의 경계/에러 경로를 추가로
// 검증하여 신규 M2 코드 커버리지를 보강한다(@SPEC:SPEC-REMOTE-001 M2).
package remote

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// failingTokenIssuer 는 Issue 가 항상 실패하는 TokenIssuer 이다.
type failingTokenIssuer struct{}

func (failingTokenIssuer) Issue(string, string) (string, error) {
	return "", errors.New("issue failed")
}
func (failingTokenIssuer) IssueWithID(string, string) (string, string, error) {
	return "", "", errors.New("issue failed")
}
func (failingTokenIssuer) Validate(string) (string, string, error) {
	return "", "", errors.New("invalid")
}
func (failingTokenIssuer) Revoke(string)           {}
func (failingTokenIssuer) IsRevoked(string) bool   { return false }
func (failingTokenIssuer) RevokeID(string)         {}
func (failingTokenIssuer) IsIDRevoked(string) bool { return false }

// TestServer_ListNodes_NoRepo 는 repo 미구성 시 빈 목록을 반환하는지 검증한다.
func TestServer_ListNodes_NoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	nodes, err := srv.ListNodes(context.Background())
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

// TestServer_ListNodes_WithRepo 는 repo 의 노드를 반환하고 라이브 online 상태를
// 반영하는지 검증한다(REQ-E05/G01 토대).
func TestServer_ListNodes_WithRepo(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "n1", Status: RegStatusApproved, Online: false,
	}))
	srv := newM2Server(repo, newFakeTokenIssuer())

	// 라이브 online 상태를 in-memory 에 설정.
	srv.setNodeState("n1", RegStatusApproved, true, time.Now())

	nodes, err := srv.ListNodes(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.True(t, nodes[0].Online, "라이브 추적이 repo online 보다 우선해야 함")
}

// TestServer_ManagedNodes 는 approved+online 노드만 ManagedNodes 에 포함되는지
// 검증한다(REQ-C06).
func TestServer_ManagedNodes(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())

	srv.setNodeState("approved-online", RegStatusApproved, true, time.Now())
	srv.setNodeState("approved-offline", RegStatusApproved, false, time.Now())
	srv.setNodeState("pending-online", RegStatusPending, true, time.Now())

	managed := srv.ManagedNodes()
	assert.Equal(t, []string{"approved-online"}, managed)
}

// TestServer_Approve_NoRepo 는 repo 없이 Approve 호출 시 ErrNoRepo 를 반환하는지
// 검증한다.
func TestServer_Approve_NoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	assert.ErrorIs(t, srv.Approve(context.Background(), "x"), ErrNoRepo)
	assert.ErrorIs(t, srv.Reject(context.Background(), "x", ""), ErrNoRepo)
	assert.ErrorIs(t, srv.Revoke(context.Background(), "x"), ErrNoRepo)
}

// TestServer_Reject_UnknownNode 는 미존재 노드 거부 시 에러를 반환하는지 검증한다.
func TestServer_Reject_UnknownNode(t *testing.T) {
	srv := newM2Server(newMemManagedNodeRepo(), newFakeTokenIssuer())
	assert.ErrorIs(t, srv.Reject(context.Background(), "missing", "r"), storage.ErrManagedNodeNotFound)
}

// TestServer_Revoke_UnknownNode 는 미존재 노드 폐기 시 에러를 반환하는지 검증한다.
func TestServer_Revoke_UnknownNode(t *testing.T) {
	srv := newM2Server(newMemManagedNodeRepo(), newFakeTokenIssuer())
	assert.ErrorIs(t, srv.Revoke(context.Background(), "missing"), storage.ErrManagedNodeNotFound)
}

// TestServer_Approve_TokenIssueFails 는 토큰 발급 실패 시 Approve 가 에러를
// 반환하는지 검증한다.
func TestServer_Approve_TokenIssueFails(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "n", Status: RegStatusPending}))
	srv := newM2Server(repo, failingTokenIssuer{})

	err := srv.Approve(context.Background(), "n")
	assert.Error(t, err, "토큰 발급 실패 시 Approve 는 에러를 반환해야 함")
}

// TestServer_Revoke_DisconnectedNode 는 연결 없는 approved 노드의 폐기도 상태/토큰
// 처리가 동작하는지 검증한다(approve-while-disconnected 의 반대 — revoke-while-offline).
func TestServer_Revoke_DisconnectedNode(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	// M6 하드닝: token_id 에는 jti 가 저장된다(원본 토큰 미저장). 폐기는 jti 로 수행.
	_, jti, _ := issuer.IssueWithID("n", "node")
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "n", Status: RegStatusApproved, TokenID: jti,
	}))
	srv := newM2Server(repo, issuer)

	// 연결 없이 폐기.
	require.NoError(t, srv.Revoke(context.Background(), "n"))
	assert.True(t, issuer.IsIDRevoked(jti))

	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, RegStatusRevoked, got.Status)
}

// TestServer_Approve_WhileDisconnected 는 연결이 끊긴 pending 노드를 승인하면
// 토큰이 발급·저장되지만 ack 는 전송되지 않는지 검증한다(approve-while-disconnected).
func TestServer_Approve_WhileDisconnected(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "n", Status: RegStatusPending}))
	srv := newM2Server(repo, issuer)

	// 연결 추적 없음 → ack push 없이도 승인은 성공해야 한다.
	require.NoError(t, srv.Approve(context.Background(), "n"))

	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, RegStatusApproved, got.Status)
	assert.NotEmpty(t, got.TokenID, "오프라인 승인도 토큰을 저장해야 함")
}

// TestServer_RestoreSession_NonApproved 는 비승인 상태 노드의 재접속 세션 복원이
// 거부되는지 검증한다(REQ-C05/C06).
func TestServer_RestoreSession_NonApproved(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "n", Status: RegStatusRejected}))
	srv := newM2Server(repo, newFakeTokenIssuer())

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnectionAuth(ctx, conn, "n")
	}()

	// 비승인 → 세션 복원 거부 → 연결 종료.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("비승인 재접속은 즉시 종료되어야 함")
	}
	assert.False(t, srv.IsManaged("n"))
}

// TestServer_RestoreSession_UnknownNode 는 repo 에 없는 노드의 재접속이 거부되는지
// 검증한다.
func TestServer_RestoreSession_UnknownNode(t *testing.T) {
	srv := newM2Server(newMemManagedNodeRepo(), newFakeTokenIssuer())
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnectionAuth(ctx, conn, "ghost")
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("미등록 재접속은 즉시 종료되어야 함")
	}
}

// TestClearNodeToken 는 노드 토큰 파일 정리를 검증한다.
func TestClearNodeToken(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "tok"))

	require.NoError(t, ClearNodeToken(dir))
	_, ok, err := LoadNodeToken(dir)
	require.NoError(t, err)
	assert.False(t, ok)

	// 미존재 파일 정리는 에러가 아니어야 한다(멱등).
	require.NoError(t, ClearNodeToken(dir))
}

// TestServer_Register_NoInstanceID 는 instance_id 누락 register 가 무시되는지
// 검증한다.
func TestServer_Register_NoInstanceID(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: ""})

	// 잠시 후에도 노드가 등록되지 않아야 한다.
	time.Sleep(50 * time.Millisecond)
	list, _ := repo.List(ctx)
	assert.Empty(t, list)
}

// TestServer_HelloSyncsStatusFromRepo 는 hello 경로에서 repo 의 등록 상태가
// in-memory 상태에 반영되는지 검증한다(syncStatusFromRepo).
func TestServer_HelloSyncsStatusFromRepo(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "n-sync", Status: RegStatusApproved,
	}))
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "n-sync"})
	conn.inject(t, hello)

	// hello 후 repo 의 approved 상태가 in-memory 에 반영되어 managed 가 되어야 한다.
	require.Eventually(t, func() bool {
		return srv.IsManaged("n-sync")
	}, time.Second, 5*time.Millisecond, "hello 는 repo 등록 상태를 동기화해야 함")
}

// TestSaveNodeToken_InvalidDir 는 잘못된 디렉토리에 토큰 저장 시 에러를 반환하는지
// 검증한다(에러 경로).
func TestSaveNodeToken_InvalidDir(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/afile"
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o600))
	// filePath 는 일반 파일이므로 그 하위를 dataDir 로 쓰면 MkdirAll 이 실패한다.
	err := SaveNodeToken(filePath, "token")
	assert.Error(t, err)
}

// TestServer_Register_RepoWriteFailure 는 repo upsert 실패 시 등록이 실패 처리되는지
// 검증한다(handleRegister 에러 경로).
func TestServer_Register_RepoWriteFailure(t *testing.T) {
	repo := &writeFailRepo{memManagedNodeRepo: newMemManagedNodeRepo()}
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "n-fail"})

	// upsert 실패 → ack 없음(등록 실패). 잠시 후 노드 미등록 확인.
	time.Sleep(50 * time.Millisecond)
	assert.False(t, srv.IsManaged("n-fail"))
}

// writeFailRepo 는 Upsert 가 항상 실패하는 repo 이다(에러 경로 테스트용).
type writeFailRepo struct {
	*memManagedNodeRepo
}

func (w *writeFailRepo) Upsert(context.Context, storage.ManagedNode) error {
	return errors.New("write failed")
}

// setTokenFailRepo 는 SetToken 이 실패하는 repo 이다(issueAndStoreToken 에러 경로).
type setTokenFailRepo struct {
	*memManagedNodeRepo
}

func (r *setTokenFailRepo) SetToken(context.Context, string, string) error {
	return errors.New("set token failed")
}

// TestServer_Approve_SetTokenFails 는 token_id 저장 실패 시 Approve 가 에러를
// 반환하는지 검증한다(issueAndStoreToken 에러 경로).
func TestServer_Approve_SetTokenFails(t *testing.T) {
	base := newMemManagedNodeRepo()
	require.NoError(t, base.Upsert(context.Background(), storage.ManagedNode{InstanceID: "n", Status: RegStatusPending}))
	repo := &setTokenFailRepo{memManagedNodeRepo: base}
	srv := newM2Server(repo, newFakeTokenIssuer())

	assert.Error(t, srv.Approve(context.Background(), "n"))
}

// TestServer_SendRegisterAckWriteFailure 는 닫힌 연결로 ack 전송 실패가 패닉 없이
// 처리되는지 검증한다(sendRegisterAck 에러 경로).
func TestServer_SendRegisterAckWriteFailure(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "n", Status: RegStatusPending}))
	srv := newM2Server(repo, newFakeTokenIssuer())

	// 닫힌 연결을 연결 추적에 직접 등록.
	conn := newFakeConn()
	_ = conn.Close()
	srv.registerConn("n", conn, func() {})

	// approve 시 ack 전송이 실패하지만 승인 자체는 성공해야 한다.
	require.NoError(t, srv.Approve(context.Background(), "n"))
	got, _ := repo.Get(context.Background(), "n")
	assert.Equal(t, RegStatusApproved, got.Status)
}

// TestServer_Register_NoRepoModeIgnored 는 repo 미구성(M1 모드) 서버가 register 를
// 무시하는지 검증한다(하위 호환).
func TestServer_Register_NoRepoModeIgnored(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil) // repo 없음.
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "n"})

	// register 는 무시되고 노드는 식별되지 않는다(M1 은 hello 만 처리).
	time.Sleep(50 * time.Millisecond)
	_, ok := srv.NodeState("n")
	assert.False(t, ok)
}
