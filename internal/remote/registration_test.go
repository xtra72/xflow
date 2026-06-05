// registration_test.go 는 M2 등록/승인 상태 머신을 검증한다
// (@SPEC:SPEC-REMOTE-001 M2, spec §5.6, REQ-C01~C07, F02/F03/F07).
//
// fakeConn / clientFakeConn 은 server_test.go / client_test.go 에 정의된 것을
// 재사용한다. fakeTokenIssuer / memManagedNodeRepo 는 본 파일에서 정의한다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// memManagedNodeRepo 는 ManagedNodeRepository 의 인메모리 구현(테스트용)이다.
type memManagedNodeRepo struct {
	mu    sync.Mutex
	nodes map[string]storage.ManagedNode
}

func newMemManagedNodeRepo() *memManagedNodeRepo {
	return &memManagedNodeRepo{nodes: make(map[string]storage.ManagedNode)}
}

func (m *memManagedNodeRepo) Upsert(_ context.Context, node storage.ManagedNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.nodes[node.InstanceID]; ok {
		node.CreatedAt = existing.CreatedAt
	} else {
		node.CreatedAt = time.Now().UnixMilli()
	}
	node.UpdatedAt = time.Now().UnixMilli()
	m.nodes[node.InstanceID] = node
	return nil
}

func (m *memManagedNodeRepo) Get(_ context.Context, instanceID string) (storage.ManagedNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[instanceID]
	if !ok {
		return storage.ManagedNode{}, storage.ErrManagedNodeNotFound
	}
	return n, nil
}

func (m *memManagedNodeRepo) List(_ context.Context) ([]storage.ManagedNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.ManagedNode, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (m *memManagedNodeRepo) UpdateStatus(_ context.Context, instanceID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[instanceID]
	if !ok {
		return storage.ErrManagedNodeNotFound
	}
	n.Status = status
	m.nodes[instanceID] = n
	return nil
}

func (m *memManagedNodeRepo) SetToken(_ context.Context, instanceID, tokenID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[instanceID]
	if !ok {
		return storage.ErrManagedNodeNotFound
	}
	n.TokenID = tokenID
	m.nodes[instanceID] = n
	return nil
}

func (m *memManagedNodeRepo) SetOnline(_ context.Context, instanceID string, online bool, lastSeenMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[instanceID]
	if !ok {
		return storage.ErrManagedNodeNotFound
	}
	n.Online = online
	n.LastSeen = lastSeenMs
	m.nodes[instanceID] = n
	return nil
}

func (m *memManagedNodeRepo) Delete(_ context.Context, instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.nodes[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	delete(m.nodes, instanceID)
	return nil
}

func (m *memManagedNodeRepo) Close() error { return nil }

// fakeTokenIssuer 는 TokenIssuer 의 테스트 구현이다. subject→token 매핑을 단순화하고,
// M6 jti 하드닝을 모사한다(token→jti 매핑, jti 기반 폐기).
type fakeTokenIssuer struct {
	mu         sync.Mutex
	issued     map[string]string // token -> subject
	tokenJTI   map[string]string // token -> jti
	revoked    map[string]bool   // 원본 토큰 기반 폐기
	revokedJTI map[string]bool   // jti 기반 폐기(M6)
	nextToken  int
}

func newFakeTokenIssuer() *fakeTokenIssuer {
	return &fakeTokenIssuer{
		issued:     make(map[string]string),
		tokenJTI:   make(map[string]string),
		revoked:    make(map[string]bool),
		revokedJTI: make(map[string]bool),
	}
}

func (f *fakeTokenIssuer) Issue(subject, role string) (string, error) {
	token, _, err := f.IssueWithID(subject, role)
	return token, err
}

func (f *fakeTokenIssuer) IssueWithID(subject, _ string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextToken++
	token := subject + "-token-" + string(rune('a'+f.nextToken))
	jti := subject + "-jti-" + string(rune('a'+f.nextToken))
	f.issued[token] = subject
	f.tokenJTI[token] = jti
	return token, jti, nil
}

func (f *fakeTokenIssuer) Validate(token string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.revoked[token] || f.revokedJTI[f.tokenJTI[token]] {
		return "", "", errors.New("revoked")
	}
	subject, ok := f.issued[token]
	if !ok {
		return "", "", errors.New("invalid")
	}
	return subject, "node", nil
}

func (f *fakeTokenIssuer) Revoke(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[token] = true
}

func (f *fakeTokenIssuer) IsRevoked(token string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revoked[token]
}

func (f *fakeTokenIssuer) RevokeID(jti string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokedJTI[jti] = true
}

func (f *fakeTokenIssuer) IsIDRevoked(jti string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revokedJTI[jti]
}

// newM2Server 는 repo + tokenIssuer 가 주입된 서버를 생성한다.
func newM2Server(repo storage.ManagedNodeRepository, issuer TokenIssuer) *Server {
	return NewServer(ServerConfig{
		Repo:        repo,
		TokenIssuer: issuer,
	}, nil)
}

// injectRegister 는 register 메시지를 주입한다.
func injectRegister(t *testing.T, conn *fakeConn, p RegisterPayload) {
	t.Helper()
	msg, err := NewRegisterMessage(p)
	require.NoError(t, err)
	conn.inject(t, msg)
}

// readAck 는 서버가 보낸 다음 register_ack 를 읽어 디코드한다.
func readAck(t *testing.T, conn *fakeConn) RegisterAckPayload {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeRegisterAck, msg.Type)
		var ack RegisterAckPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &ack))
		return ack
	case <-time.After(time.Second):
		t.Fatal("register_ack 수신 타임아웃")
		return RegisterAckPayload{}
	}
}

// TestRegister_NewNodePending 는 미등록 노드의 register 가 pending 으로 보류되고
// pending ack 를 받으며 관리 권한이 부여되지 않는지 검증한다(REQ-C01/C02/F03).
func TestRegister_NewNodePending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-1", Hostname: "h", Version: "v"})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
	assert.Empty(t, ack.NodeToken)

	// 영속: pending 으로 저장.
	n, err := repo.Get(ctx, "node-1")
	require.NoError(t, err)
	assert.Equal(t, RegStatusPending, n.Status)

	// 관리 대상으로 노출되지 않아야 한다(REQ-C06).
	assert.False(t, srv.IsManaged("node-1"), "pending 노드는 managed 가 아니어야 함")
}

// TestApprove_IssuesTokenAndAck 는 승인 시 approved 전이 + 토큰 발급 + approved ack
// 전송을 검증한다(REQ-C03/C04).
func TestApprove_IssuesTokenAndAck(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := newM2Server(repo, issuer)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-2"})
	pending := readAck(t, conn)
	require.Equal(t, RegStatusPending, pending.Status)

	// 관리자 승인.
	require.NoError(t, srv.Approve(ctx, "node-2"))

	// 연결된 노드는 approved ack(토큰 포함)를 받는다.
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusApproved, ack.Status)
	assert.NotEmpty(t, ack.NodeToken, "approved ack 는 node_token 을 포함해야 함")

	// 토큰은 유효하고 subject=instance_id 이다(REQ-F02).
	subject, role, err := issuer.Validate(ack.NodeToken)
	require.NoError(t, err)
	assert.Equal(t, "node-2", subject)
	assert.Equal(t, "node", role)

	// 영속: approved + token_id 저장.
	n, _ := repo.Get(ctx, "node-2")
	assert.Equal(t, RegStatusApproved, n.Status)
	assert.NotEmpty(t, n.TokenID)

	// 이제 managed 이다(REQ-C06 반대편).
	assert.True(t, srv.IsManaged("node-2"))
}

// TestReject_SetsRejected 는 거부 시 rejected 전이 + rejected ack 를 검증한다(REQ-C03).
func TestReject_SetsRejected(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-3"})
	_ = readAck(t, conn)

	require.NoError(t, srv.Reject(ctx, "node-3", "정책 위반"))

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusRejected, ack.Status)
	assert.Equal(t, "정책 위반", ack.Reason)

	n, _ := repo.Get(ctx, "node-3")
	assert.Equal(t, RegStatusRejected, n.Status)
	assert.False(t, srv.IsManaged("node-3"))
}

// TestReconnectWithValidToken_RestoresSession 는 승인된 노드가 유효 토큰으로
// 재접속하면 재등록 없이 관리 세션이 복원되는지 검증한다(REQ-C05/F02).
func TestReconnectWithValidToken_RestoresSession(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := newM2Server(repo, issuer)

	// 사전 조건: 승인된 노드 + 발급 토큰.
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "node-4", Status: RegStatusApproved}))
	token, _ := issuer.Issue("node-4", "node")
	require.NoError(t, repo.SetToken(context.Background(), "node-4", token))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 핸들러가 토큰을 검증하고 authedInstanceID 를 넘기는 재접속 경로.
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "node-4") }()

	// 재접속 시 register 없이 곧바로 managed online 이어야 한다.
	require.Eventually(t, func() bool {
		return srv.IsManaged("node-4")
	}, time.Second, 5*time.Millisecond, "유효 토큰 재접속은 세션을 복원해야 함")

	st, ok := srv.NodeState("node-4")
	require.True(t, ok)
	assert.True(t, st.Online)
	assert.Equal(t, RegStatusApproved, st.Status)
}

// TestRevoke_BlacklistsAndDisconnects 는 폐기 시 토큰 blacklist + revoked 전이 +
// 연결 종료 + 이후 재인증 거부를 검증한다(REQ-C07/F07).
func TestRevoke_BlacklistsAndDisconnects(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := newM2Server(repo, issuer)

	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "node-5", Status: RegStatusApproved}))
	// M6 하드닝: 서버는 원본 토큰이 아닌 jti 만 저장한다(DB-안전). 폐기는 jti 로 수행된다.
	token, jti, _ := issuer.IssueWithID("node-5", "node")
	require.NoError(t, repo.SetToken(context.Background(), "node-5", jti))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnectionAuth(ctx, conn, "node-5")
	}()

	require.Eventually(t, func() bool {
		return srv.IsManaged("node-5")
	}, time.Second, 5*time.Millisecond)

	// 관리자 폐기.
	require.NoError(t, srv.Revoke(ctx, "node-5"))

	// jti 가 blacklist 되어야 한다(REQ-F07 — M6 jti 기반 폐기).
	assert.True(t, issuer.IsIDRevoked(jti))

	// 연결이 종료되어야 한다.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("폐기 후 연결이 종료되어야 함")
	}

	// 상태 revoked + managed 아님.
	n, _ := repo.Get(ctx, "node-5")
	assert.Equal(t, RegStatusRevoked, n.Status)
	assert.False(t, srv.IsManaged("node-5"))

	// 이후 동일 토큰 재인증 거부(REQ-C07).
	_, _, err := issuer.Validate(token)
	assert.Error(t, err)
}

// TestPendingNodeNotManaged 는 pending 상태 노드가 연결되어 있어도 managed 가
// 아니어야 함을 검증한다(REQ-C06).
func TestPendingNodeNotManaged(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-6"})
	_ = readAck(t, conn)

	// 연결되어 있으나 managed 아님.
	st, ok := srv.NodeState("node-6")
	require.True(t, ok)
	assert.True(t, st.Online, "pending 노드도 연결은 online")
	assert.False(t, srv.IsManaged("node-6"), "pending 노드는 managed 가 아님")
}

// TestApprove_UnknownNode 는 미존재 노드 승인 시 에러를 반환하는지 검증한다.
func TestApprove_UnknownNode(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	err := srv.Approve(context.Background(), "missing")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestRegister_RejectedNodeStaysRejected 는 rejected 노드가 다시 register 해도
// 자동으로 pending 으로 돌아가지 않아야 함을 검증한다(REQ-C06 — 거부 유지).
func TestRegister_RejectedNodeStaysRejected(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "node-7", Status: RegStatusRejected}))

	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-7"})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusRejected, ack.Status, "rejected 노드는 register 해도 rejected ack 를 받아야 함")

	n, _ := repo.Get(ctx, "node-7")
	assert.Equal(t, RegStatusRejected, n.Status)
}

// TestBootstrapSecret_Mismatch 는 bootstrap_secret 이 구성된 서버에서 잘못된
// 시크릿의 register 가 거부되는지 검증한다(REQ-C08).
func TestBootstrapSecret_Mismatch(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{
		Repo:            repo,
		TokenIssuer:     newFakeTokenIssuer(),
		BootstrapSecret: "correct-secret",
	}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-8", BootstrapSecret: "wrong"})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusRejected, ack.Status, "잘못된 부트스트랩 시크릿은 거부되어야 함")

	// pending 으로 큐잉되지 않아야 한다.
	_, err := repo.Get(ctx, "node-8")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestBootstrapSecret_Match 는 올바른 시크릿이면 pending 큐잉됨을 검증한다(REQ-C08).
func TestBootstrapSecret_Match(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := NewServer(ServerConfig{
		Repo:            repo,
		TokenIssuer:     newFakeTokenIssuer(),
		BootstrapSecret: "correct-secret",
	}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-9", BootstrapSecret: "correct-secret"})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)

	n, err := repo.Get(ctx, "node-9")
	require.NoError(t, err)
	assert.Equal(t, RegStatusPending, n.Status)
}
