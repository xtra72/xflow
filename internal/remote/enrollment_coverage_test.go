// enrollment_coverage_test.go 는 수동 enrollment(그룹 H) 서버 로직의 에러/경계 경로를
// 보강 검증한다(@SPEC:SPEC-REMOTE-001 v1.1). 정상 경로는 enrollment_test.go 가 다룬다.
package remote

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// TestPreRegister_NoRepo 는 repo 미구성 시 ErrNoRepo 를 반환하는지 검증한다.
func TestPreRegister_NoRepo(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil) // repo 미주입
	assert.ErrorIs(t, srv.PreRegister(context.Background(), "x", ""), ErrNoRepo)
	assert.ErrorIs(t, srv.RemoveNode(context.Background(), "x"), ErrNoRepo)
}

// TestPreRegister_EmptyInstanceID 는 빈 instance_id 가 거부되는지 검증한다.
func TestPreRegister_EmptyInstanceID(t *testing.T) {
	srv := newEnrollServer(newMemManagedNodeRepo(), newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())
	assert.Error(t, srv.PreRegister(context.Background(), "", "name"))
}

// TestRemoveNode_NoTokenNoConn 는 토큰/연결이 없는 노드도 정상 삭제되는지 검증한다.
func TestRemoveNode_NoTokenNoConn(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, storage.ManagedNode{InstanceID: "no-tok", Status: RegStatusApproved}))
	require.NoError(t, srv.RemoveNode(ctx, "no-tok"))
	_, err := repo.Get(ctx, "no-tok")
	assert.ErrorIs(t, err, storage.ErrManagedNodeNotFound)
}

// TestEnrollment_NilRepoOrToken 는 enroll 저장소 미구성/토큰 미운반 시 자동 승인을
// 시도하지 않는지(handled=false) 검증한다.
func TestEnrollment_NilRepoOrToken(t *testing.T) {
	// enroll 저장소 미구성.
	srv := NewServer(ServerConfig{Repo: newMemManagedNodeRepo(), TokenIssuer: newFakeTokenIssuer()}, nil)
	assert.False(t, srv.tryEnrollmentAutoApprove(context.Background(), newFakeConn(), RegisterPayload{InstanceID: "a", EnrollmentToken: "t"}))

	// 토큰 미운반.
	srv2 := newEnrollServer(newMemManagedNodeRepo(), newFakeTokenIssuer(), newMemEnrollmentRepo(), newMemAuditRepo())
	assert.False(t, srv2.tryEnrollmentAutoApprove(context.Background(), newFakeConn(), RegisterPayload{InstanceID: "a"}))
}

// TestEnrollmentTokenUsable 는 사용 가능 판정의 분기를 모두 검증한다.
func TestEnrollmentTokenUsable(t *testing.T) {
	now := int64(1000)
	assert.True(t, enrollmentTokenUsable(storage.EnrollmentToken{}, now))
	assert.False(t, enrollmentTokenUsable(storage.EnrollmentToken{Revoked: true}, now))
	assert.False(t, enrollmentTokenUsable(storage.EnrollmentToken{ExpiresAt: storage.Int64Ptr(500)}, now))
	assert.True(t, enrollmentTokenUsable(storage.EnrollmentToken{ExpiresAt: storage.Int64Ptr(2000)}, now))
	assert.False(t, enrollmentTokenUsable(storage.EnrollmentToken{MaxUses: storage.IntPtr(1), Uses: 1}, now))
	assert.True(t, enrollmentTokenUsable(storage.EnrollmentToken{MaxUses: storage.IntPtr(2), Uses: 1}, now))
}

// TestAuditRecorders_NilAudit 는 audit 미구성 시 기록 함수가 패닉 없이 no-op 인지 검증한다.
func TestAuditRecorders_NilAudit(t *testing.T) {
	srv := NewServer(ServerConfig{Repo: newMemManagedNodeRepo()}, nil) // audit 미주입
	assert.NotPanics(t, func() {
		srv.recordEnrollmentAudit(context.Background(), "n", "et")
		srv.recordPreApprovedAudit(context.Background(), "n")
	})
}

// upsertFailRepo 는 Upsert 가 실패하는 ManagedNodeRepository 데코레이터이다.
type upsertFailRepo struct {
	storage.ManagedNodeRepository
	failUpsert error
}

func (r *upsertFailRepo) Upsert(ctx context.Context, n storage.ManagedNode) error {
	if r.failUpsert != nil {
		return r.failUpsert
	}
	return r.ManagedNodeRepository.Upsert(ctx, n)
}

// failIssuer 는 IssueWithID 가 실패하는 TokenIssuer 데코레이터이다.
type failIssuer struct {
	TokenIssuer
	err error
}

func (f *failIssuer) IssueWithID(subject, role string) (string, string, error) {
	return "", "", f.err
}

// TestEnrollment_AutoApproveUpsertFails 는 자동 승인 중 Upsert 실패 시 폴백(handled=false)
// 하는지 검증한다(REQ-H05 보수적 폴백).
func TestEnrollment_AutoApproveUpsertFails(t *testing.T) {
	base := newMemManagedNodeRepo()
	repo := &upsertFailRepo{ManagedNodeRepository: base, failUpsert: assertError("upsert boom")}
	enroll := newMemEnrollmentRepo()
	srv := newEnrollServer(repo, newFakeTokenIssuer(), enroll, newMemAuditRepo())

	raw := "tok"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{ID: "e", TokenHash: hashToken(raw), CreatedAt: 1}))
	handled := srv.tryEnrollmentAutoApprove(context.Background(), newFakeConn(),
		RegisterPayload{InstanceID: "n", EnrollmentToken: raw})
	assert.False(t, handled)
}

// TestEnrollment_AutoApproveIssueFails 는 자동 승인 중 토큰 발급 실패 시 폴백하는지 검증한다.
func TestEnrollment_AutoApproveIssueFails(t *testing.T) {
	repo := newMemManagedNodeRepo()
	enroll := newMemEnrollmentRepo()
	issuer := &failIssuer{TokenIssuer: newFakeTokenIssuer(), err: assertError("issue boom")}
	srv := newEnrollServer(repo, issuer, enroll, newMemAuditRepo())

	raw := "tok2"
	require.NoError(t, enroll.Create(context.Background(), storage.EnrollmentToken{ID: "e2", TokenHash: hashToken(raw), CreatedAt: 1}))
	handled := srv.tryEnrollmentAutoApprove(context.Background(), newFakeConn(),
		RegisterPayload{InstanceID: "n", EnrollmentToken: raw})
	assert.False(t, handled)
}
