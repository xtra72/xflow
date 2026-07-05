// enrollment_test.go 는 수동 enrollment(그룹 H)의 등록 자동 승인 경로를 검증한다
// (@SPEC:SPEC-REMOTE-001 v1.1, REQ-REMOTE-H02/H05/H08).
//
// 경로 A: 사전 등록(pre-registration) — approved + token 미발급 노드가 접속하면
//
//	관리자 개입 없이 즉시 토큰을 발급받는다(자동 승인).
//
// 경로 B: enrollment 토큰 — register 가 유효한 enrollment 토큰을 운반하면 신규 노드를
//
//	즉시 승인하고 토큰을 발급하며 uses 를 증가시킨다. 무효/만료/소진/폐기 토큰은
//	pending 으로 폴백한다.
//
// fakeConn / readAck / injectRegister 는 server_test.go / registration_test.go 의
// 헬퍼를 재사용한다. memEnrollmentRepo / memAuditRepo 는 본 파일에서 정의한다.
package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// memEnrollmentRepo 는 EnrollmentTokenRepository 의 인메모리 구현(테스트용)이다.
type memEnrollmentRepo struct {
	mu     sync.Mutex
	tokens map[string]storage.EnrollmentToken // id -> token
}

func newMemEnrollmentRepo() *memEnrollmentRepo {
	return &memEnrollmentRepo{tokens: make(map[string]storage.EnrollmentToken)}
}

func (m *memEnrollmentRepo) Create(_ context.Context, tok storage.EnrollmentToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[tok.ID] = tok
	return nil
}

func (m *memEnrollmentRepo) GetByHash(_ context.Context, hash string) (storage.EnrollmentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return storage.EnrollmentToken{}, storage.ErrEnrollmentTokenNotFound
}

func (m *memEnrollmentRepo) List(_ context.Context) ([]storage.EnrollmentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.EnrollmentToken, 0, len(m.tokens))
	for _, t := range m.tokens {
		t.TokenHash = "" // List 는 hash 미노출(REQ-H06).
		out = append(out, t)
	}
	return out, nil
}

func (m *memEnrollmentRepo) Revoke(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	if !ok {
		return storage.ErrEnrollmentTokenNotFound
	}
	t.Revoked = true
	m.tokens[id] = t
	return nil
}

func (m *memEnrollmentRepo) IncrementUses(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[id]
	if !ok {
		return storage.ErrEnrollmentTokenNotFound
	}
	if t.MaxUses != nil && t.Uses >= *t.MaxUses {
		return storage.ErrEnrollmentTokenExhausted
	}
	t.Uses++
	m.tokens[id] = t
	return nil
}

func (m *memEnrollmentRepo) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[id]; !ok {
		return storage.ErrEnrollmentTokenNotFound
	}
	delete(m.tokens, id)
	return nil
}

func (m *memEnrollmentRepo) Close() error { return nil }

// memAuditRepo / newMemAuditRepo 는 audit_test.go 에 정의된 것을 재사용한다.

// hashToken 은 테스트에서 원본 토큰의 SHA-256 16진 해시를 계산한다(구현과 동일 규약).
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newEnrollServer 는 repo + tokenIssuer + enrollment + audit 가 주입된 서버를 만든다.
func newEnrollServer(repo storage.ManagedNodeRepository, issuer TokenIssuer, enroll storage.EnrollmentTokenRepository, audit storage.RemoteAuditRepository) *Server {
	return NewServer(ServerConfig{
		Repo:        repo,
		TokenIssuer: issuer,
		Enrollment:  enroll,
		Audit:       audit,
	}, nil)
}

// --- 경로 A: 사전 등록(pre-registration) 자동 승인 -------------------------------

// TestPreRegistered_AutoApproveOnConnect 는 approved + token 미발급(사전 등록) 노드가
// register 하면 관리자 개입 없이 즉시 토큰을 발급받는지 검증한다(REQ-H01/H02).
func TestPreRegistered_AutoApproveOnConnect(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	audit := newMemAuditRepo()
	srv := newEnrollServer(repo, issuer, newMemEnrollmentRepo(), audit)

	// 사전 등록: approved 이지만 토큰 미발급(관리자가 미리 생성).
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "pre-1", Status: RegStatusApproved, // TokenID 비어 있음
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "pre-1", Hostname: "h", Version: "v"})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusApproved, ack.Status)
	assert.NotEmpty(t, ack.NodeToken, "사전 등록 노드는 즉시 토큰을 받아야 한다")

	// 토큰이 영속되었다.
	n, _ := repo.Get(ctx, "pre-1")
	assert.NotEmpty(t, n.TokenID)
	assert.True(t, srv.IsManaged("pre-1"))

	// 감사: 자동 승인 1건 기록(actor=admin pre-registration).
	recs := audit.all()
	require.NotEmpty(t, recs)
	assert.Equal(t, storage.AuditActionApprove, recs[0].Action)
	assert.Equal(t, "pre-1", recs[0].InstanceID)
}

// TestApprovedWithToken_ReconnectUnchanged 는 approved + token 보유 노드의 재등록이
// 기존 동작(현재 상태 ack)을 유지하는지 검증한다(정상 재접속 — 변경 없음).
func TestApprovedWithToken_ReconnectUnchanged(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	audit := newMemAuditRepo()
	srv := newEnrollServer(repo, issuer, newMemEnrollmentRepo(), audit)

	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "rc-1", Status: RegStatusApproved, TokenID: "existing-jti",
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "rc-1"})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusApproved, ack.Status)

	// 이미 토큰을 가진 노드는 사전 등록 자동 승인 감사가 기록되지 않는다.
	for _, r := range audit.all() {
		assert.NotEqual(t, storage.AuditActionApprove, r.Action,
			"기 토큰 보유 재접속은 자동 승인 감사를 남기지 않아야 한다")
	}
}

// --- 경로 B: enrollment 토큰 자동 승인 ------------------------------------------

// TestEnrollmentToken_AutoApprove 는 유효한 enrollment 토큰을 운반한 register 가
// 신규 노드를 즉시 승인하고 토큰을 발급하며 uses 를 증가시키는지 검증한다(REQ-H05/H08).
func TestEnrollmentToken_AutoApprove(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	enroll := newMemEnrollmentRepo()
	audit := newMemAuditRepo()
	srv := newEnrollServer(repo, issuer, enroll, audit)

	raw := "super-secret-enrollment-token"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{
		ID: "et-1", TokenHash: hashToken(raw), Label: "fleet", CreatedAt: time.Now().UnixMilli(),
		MaxUses: storage.IntPtr(5),
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{
		InstanceID: "enr-1", Hostname: "host-a", Version: "1.0", EnrollmentToken: raw,
	})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusApproved, ack.Status)
	assert.NotEmpty(t, ack.NodeToken)

	// 노드는 approved + 토큰 보유로 영속.
	n, err := repo.Get(ctx, "enr-1")
	require.NoError(t, err)
	assert.Equal(t, RegStatusApproved, n.Status)
	assert.Equal(t, "host-a", n.Hostname)
	assert.NotEmpty(t, n.TokenID)

	// uses 증가.
	used, _ := enroll.GetByHash(ctx, hashToken(raw))
	assert.Equal(t, 1, used.Uses)

	// 감사: enrollment 토큰 id 기록(원본 토큰 아님 — REQ-H08).
	recs := audit.all()
	require.NotEmpty(t, recs)
	assert.Equal(t, storage.AuditActionApprove, recs[0].Action)
	assert.Contains(t, recs[0].Reason+recs[0].Actor, "et-1", "감사에 enrollment 토큰 id 가 기록되어야 한다")
	// 원본 토큰은 감사에 절대 남지 않는다.
	for _, r := range recs {
		assert.NotContains(t, r.Reason, raw)
		assert.NotContains(t, r.Actor, raw)
	}
}

// TestEnrollmentToken_InvalidFallsThroughToPending 는 무효(미존재) 토큰이 pending 으로
// 폴백하는지 검증한다(REQ-H05 invalid-token 정책).
func TestEnrollmentToken_InvalidFallsThroughToPending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "bad-1", EnrollmentToken: "does-not-exist"})

	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
	n, _ := repo.Get(ctx, "bad-1")
	assert.Equal(t, RegStatusPending, n.Status)
}

// TestEnrollmentToken_RevokedFallsThroughToPending 는 폐기된 토큰이 pending 으로
// 폴백하는지 검증한다(REQ-H04).
func TestEnrollmentToken_RevokedFallsThroughToPending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	enroll := newMemEnrollmentRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), enroll, newMemAuditRepo())

	raw := "revoked-token"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{
		ID: "et-r", TokenHash: hashToken(raw), Revoked: true, CreatedAt: 1,
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "rev-1", EnrollmentToken: raw})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
}

// TestEnrollmentToken_ExpiredFallsThroughToPending 는 만료 토큰이 pending 으로
// 폴백하는지 검증한다(REQ-H05).
func TestEnrollmentToken_ExpiredFallsThroughToPending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	enroll := newMemEnrollmentRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), enroll, newMemAuditRepo())

	raw := "expired-token"
	past := time.Now().Add(-time.Hour).UnixMilli()
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{
		ID: "et-e", TokenHash: hashToken(raw), ExpiresAt: storage.Int64Ptr(past), CreatedAt: 1,
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "exp-1", EnrollmentToken: raw})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
}

// TestEnrollmentToken_ExhaustedFallsThroughToPending 는 max_uses 소진 토큰이 pending
// 으로 폴백하는지 검증한다(REQ-H07).
func TestEnrollmentToken_ExhaustedFallsThroughToPending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	enroll := newMemEnrollmentRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), enroll, newMemAuditRepo())

	raw := "exhausted-token"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{
		ID: "et-x", TokenHash: hashToken(raw), MaxUses: storage.IntPtr(1), Uses: 1, CreatedAt: 1,
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "exh-1", EnrollmentToken: raw})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
}

// TestEnrollmentToken_BootstrapGateComposes 는 bootstrap_secret 게이트가 enrollment
// 토큰보다 먼저 적용되어, 시크릿 불일치 시 토큰이 유효해도 거부되는지 검증한다(REQ-C08/H05).
func TestEnrollmentToken_BootstrapGateComposes(t *testing.T) {
	repo := newMemManagedNodeRepo()
	enroll := newMemEnrollmentRepo()
	srv := NewServer(ServerConfig{
		Repo:            repo,
		TokenIssuer:     newFakeTokenIssuer(),
		Enrollment:      enroll,
		Audit:           newMemAuditRepo(),
		BootstrapSecret: "the-secret",
	}, nil)

	raw := "valid-but-no-bootstrap"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{
		ID: "et-b", TokenHash: hashToken(raw), CreatedAt: 1,
	}))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	// bootstrap_secret 불일치 → enrollment 토큰이 유효해도 거부(rejected).
	injectRegister(t, conn, RegisterPayload{InstanceID: "bs-1", EnrollmentToken: raw, BootstrapSecret: "wrong"})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusRejected, ack.Status)
}

// TestUnknownNode_NoEnrollment_StillPending 는 토큰 없는 미등록 노드가 여전히 pending
// 으로 가는지(회귀 없음) 검증한다(REQ-C02 보존).
func TestUnknownNode_NoEnrollment_StillPending(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "plain-1"})
	ack := readAck(t, conn)
	assert.Equal(t, RegStatusPending, ack.Status)
}

// --- 사전 등록/삭제 서비스 메서드 ----------------------------------------------

// TestPreRegister_CreatesApprovedNode 는 PreRegister 가 approved+offline 노드를
// 생성하는지, 중복은 ErrManagedNodeExists 를 반환하는지 검증한다(REQ-H01).
func TestPreRegister_CreatesApprovedNode(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())
	ctx := context.Background()

	require.NoError(t, srv.PreRegister(ctx, "node-pre", "Edge A"))
	n, err := repo.Get(ctx, "node-pre")
	require.NoError(t, err)
	assert.Equal(t, RegStatusApproved, n.Status)
	assert.False(t, n.Online)
	assert.Empty(t, n.TokenID, "사전 등록은 접속 전까지 토큰을 발급하지 않는다")

	// 중복 instance_id → ErrManagedNodeExists.
	assert.ErrorIs(t, srv.PreRegister(ctx, "node-pre", "dup"), ErrManagedNodeExists)
}

// TestRemoveNode_DeletesAndRevokes 는 RemoveNode 가 노드를 삭제하고 토큰을 폐기하는지
// 검증한다(REQ-H01).
func TestRemoveNode_DeletesAndRevokes(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := newEnrollServer(repo, issuer, newMemEnrollmentRepo(), newMemAuditRepo())
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{
		InstanceID: "rm-1", Status: RegStatusApproved, TokenID: "rm-jti",
	}))

	require.NoError(t, srv.RemoveNode(ctx, "rm-1"))
	_, err := repo.Get(ctx, "rm-1")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
	assert.True(t, issuer.IsIDRevoked("rm-jti"), "삭제 시 노드 토큰이 폐기되어야 한다")

	// 미존재 삭제 → ErrManagedNodeNotFound.
	assert.ErrorIs(t, srv.RemoveNode(ctx, "rm-1"), storage.ErrManagedNodeNotFound)
}
