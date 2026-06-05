// audit_test.go 는 M6 원격 변경 감사(audit) 영속화를 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F05/F06).
//
// 서버 Dispatch 가 명령 결과(성공/실패/타임아웃)를 감사 저장소에 기록하되, 시크릿
// (명령 인자/토큰)은 절대 기록하지 않음을 확인한다.
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

// memAuditRepo 는 RemoteAuditRepository 의 인메모리 테스트 구현이다.
type memAuditRepo struct {
	mu      sync.Mutex
	records []storage.RemoteAuditRecord
}

func newMemAuditRepo() *memAuditRepo {
	return &memAuditRepo{}
}

func (m *memAuditRepo) Append(_ context.Context, rec storage.RemoteAuditRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.ID = int64(len(m.records) + 1)
	m.records = append(m.records, rec)
	return nil
}

func (m *memAuditRepo) List(_ context.Context, instanceID string, limit, _ int) ([]storage.RemoteAuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.RemoteAuditRecord, 0)
	for _, r := range m.records {
		if instanceID == "" || r.InstanceID == instanceID {
			out = append(out, r)
		}
		if len(out) >= limit && limit > 0 {
			break
		}
	}
	return out, nil
}

func (m *memAuditRepo) Close() error { return nil }

func (m *memAuditRepo) all() []storage.RemoteAuditRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]storage.RemoteAuditRecord(nil), m.records...)
}

// dispatchTestServer 는 audit 저장소가 주입된 서버 + 승인+온라인 노드 연결을 구성한다.
func newAuditDispatchServer(t *testing.T, audit storage.RemoteAuditRepository) (*Server, *fakeConn) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-a", Status: RegStatusApproved,
	}))
	srv := NewServer(ServerConfig{
		Repo:        repo,
		TokenIssuer: newFakeTokenIssuer(),
		Audit:       audit,
	}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "node-a") }()
	require.Eventually(t, func() bool { return srv.IsManaged("node-a") },
		time.Second, 5*time.Millisecond)
	return srv, conn
}

// TestDispatch_AuditSuccess 는 명령 성공 시 감사 레코드(actor/instance/domain/action/
// result=ok)가 기록되는지 검증한다(REQ-F05).
func TestDispatch_AuditSuccess(t *testing.T) {
	audit := newMemAuditRepo()
	srv, conn := newAuditDispatchServer(t, audit)

	// 클라이언트 결과 응답을 시뮬레이션: 서버가 보낸 command 를 읽고 ok 결과를 주입.
	go func() {
		raw := <-conn.outgoing
		msg, _ := DecodeMessage(raw)
		var cmd CommandPayload
		_ = json.Unmarshal(msg.Payload, &cmd)
		res, _ := NewCommandResultMessage(CommandResultPayload{CommandID: cmd.CommandID, OK: true, Result: json.RawMessage(`{"done":true}`)})
		conn.incoming <- mustEncode(t, res)
	}()

	ctx := ContextWithActor(context.Background(), "admin-bob")
	args := json.RawMessage(`{"password":"hunter2","token":"abc123"}`)
	_, err := srv.Dispatch(ctx, "node-a", "flow", "deploy", args)
	require.NoError(t, err)

	recs := audit.all()
	require.Len(t, recs, 1)
	r := recs[0]
	assert.Equal(t, "node-a", r.InstanceID)
	assert.Equal(t, "admin-bob", r.Actor)
	assert.Equal(t, storage.AuditActionCommand, r.Action)
	assert.Equal(t, "flow", r.Domain)
	assert.Equal(t, "deploy", r.CommandAction)
	assert.Equal(t, storage.AuditResultOK, r.Result)

	// 시크릿이 어떤 필드에도 새지 않아야 한다(REQ-F06).
	assertNoSecret(t, r)
}

// TestDispatch_AuditFailure 는 명령 실패 시 result=error 가 기록되고 시크릿이 새지
// 않는지 검증한다(REQ-F05/F09/F06).
func TestDispatch_AuditFailure(t *testing.T) {
	audit := newMemAuditRepo()
	srv, conn := newAuditDispatchServer(t, audit)

	go func() {
		raw := <-conn.outgoing
		msg, _ := DecodeMessage(raw)
		var cmd CommandPayload
		_ = json.Unmarshal(msg.Payload, &cmd)
		res, _ := NewCommandResultMessage(CommandResultPayload{CommandID: cmd.CommandID, OK: false, Error: "validation failed"})
		conn.incoming <- mustEncode(t, res)
	}()

	ctx := ContextWithActor(context.Background(), "admin-bob")
	_, err := srv.Dispatch(ctx, "node-a", "agent", "start", json.RawMessage(`{"api_key":"secret"}`))
	require.Error(t, err)

	recs := audit.all()
	require.Len(t, recs, 1)
	assert.Equal(t, storage.AuditResultError, recs[0].Result)
	assert.Equal(t, "agent", recs[0].Domain)
	assertNoSecret(t, recs[0])
}

// TestDispatch_AuditTimeout 는 타임아웃 시 result=error 가 기록되는지 검증한다(REQ-D06/F05).
func TestDispatch_AuditTimeout(t *testing.T) {
	audit := newMemAuditRepo()
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: "node-t", Status: RegStatusApproved}))
	srv := NewServer(ServerConfig{
		Repo: repo, TokenIssuer: newFakeTokenIssuer(), Audit: audit,
		CommandTimeout: 50 * time.Millisecond,
	}, nil)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "node-t") }()
	require.Eventually(t, func() bool { return srv.IsManaged("node-t") }, time.Second, 5*time.Millisecond)
	// drain outgoing command but never respond.
	go func() { <-conn.outgoing }()

	dctx := ContextWithActor(context.Background(), "admin-x")
	_, err := srv.Dispatch(dctx, "node-t", "device", "set", json.RawMessage(`{"secret":"x"}`))
	require.ErrorIs(t, err, ErrCommandTimeout)

	recs := audit.all()
	require.Len(t, recs, 1)
	assert.Equal(t, storage.AuditResultError, recs[0].Result)
	assertNoSecret(t, recs[0])
}

// TestActorContext_RoundTrip 는 ContextWithActor/ActorFromContext 의 왕복을 검증한다.
func TestActorContext_RoundTrip(t *testing.T) {
	assert.Equal(t, "", ActorFromContext(context.Background()),
		"actor 미설정 ctx 는 빈 문자열")
	ctx := ContextWithActor(context.Background(), "admin-z")
	assert.Equal(t, "admin-z", ActorFromContext(ctx))
}

// failingAudit 는 Append 가 항상 실패하는 감사 저장소이다(에러 경로 커버용).
type failingAudit struct{}

func (failingAudit) Append(context.Context, storage.RemoteAuditRecord) error {
	return errors.New("audit append failed")
}
func (failingAudit) List(context.Context, string, int, int) ([]storage.RemoteAuditRecord, error) {
	return nil, nil
}
func (failingAudit) Close() error { return nil }

// TestRecordCommandAudit_AppendErrorTolerated 는 감사 기록 실패가 명령 경로를 막지
// 않음을 검증한다(감사 실패는 로깅만).
func TestRecordCommandAudit_AppendErrorTolerated(t *testing.T) {
	srv := NewServer(ServerConfig{Audit: failingAudit{}}, nil)
	// recordCommandAudit 는 패닉 없이 반환해야 한다(실패는 로깅으로 흡수).
	srv.recordCommandAudit(ContextWithActor(context.Background(), "a"),
		"node", "flow", "deploy", storage.AuditResultOK, "")

	// audit nil 인 경우도 no-op 이어야 한다.
	srv2 := NewServer(ServerConfig{}, nil)
	srv2.recordCommandAudit(context.Background(), "n", "flow", "deploy", storage.AuditResultOK, "")
}

func mustEncode(t *testing.T, msg interface{ Encode() ([]byte, error) }) []byte {
	t.Helper()
	data, err := msg.Encode()
	require.NoError(t, err)
	return data
}

// assertNoSecret 는 감사 레코드의 모든 텍스트 필드에 공통 시크릿 키/값이 없음을 단언한다.
func assertNoSecret(t *testing.T, r storage.RemoteAuditRecord) {
	t.Helper()
	blob := r.InstanceID + "|" + r.Actor + "|" + r.Action + "|" + r.Domain + "|" +
		r.CommandAction + "|" + r.Result + "|" + r.Reason
	for _, secret := range []string{"hunter2", "abc123", "password", "api_key", "secret\":", "token\":"} {
		assert.NotContains(t, blob, secret, "감사 레코드에 시크릿이 새면 안 됨")
	}
}
