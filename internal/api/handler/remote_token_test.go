// remote_token_test.go 는 관리 WS 핸들러의 노드 토큰 핸드셰이크 검증 경로를
// 검증한다(@SPEC:SPEC-REMOTE-001 M2, REQ-B02/C05/F02).
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// fakeTokenValidator 는 RemoteTokenValidator 의 테스트 구현이다.
type fakeTokenValidator struct {
	subject string
	role    string
	err     error
}

func (f fakeTokenValidator) Validate(_ string) (string, string, error) {
	return f.subject, f.role, f.err
}

// newM2RemoteServer 는 repo + 토큰 발급기가 주입된 remote.Server 를 생성한다.
func newM2RemoteServer(t *testing.T) (*remote.Server, storage.ManagedNodeRepository) {
	t.Helper()
	repo, err := storage.NewManagedNodeSQLiteRepository(context.Background(),
		t.TempDir()+"/managed.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	srv := remote.NewServer(remote.ServerConfig{
		Repo:        repo,
		TokenIssuer: newStubIssuer(),
	}, nil)
	return srv, repo
}

// stubIssuer 는 핸들러 테스트용 최소 TokenIssuer 이다.
type stubIssuer struct{}

func newStubIssuer() remote.TokenIssuer { return stubIssuer{} }

func (stubIssuer) Issue(subject, _ string) (string, error) { return subject + "-tok", nil }
func (stubIssuer) IssueWithID(subject, _ string) (string, string, error) {
	return subject + "-tok", subject + "-jti", nil
}
func (stubIssuer) Validate(string) (string, string, error) { return "", "", errors.New("n/a") }
func (stubIssuer) Revoke(string)                           {}
func (stubIssuer) IsRevoked(string) bool                   { return false }
func (stubIssuer) RevokeID(string)                         {}
func (stubIssuer) IsIDRevoked(string) bool                 { return false }

// TestRemoteHandler_ValidTokenRestoresSession 는 유효 노드 토큰으로 접속한 승인
// 노드의 세션이 복원되는지(managed online) 검증한다(REQ-C05/F02).
func TestRemoteHandler_ValidTokenRestoresSession(t *testing.T) {
	srv, repo := newM2RemoteServer(t)
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-tok", Status: "approved",
	}))

	h := NewRemoteHandler(srv, nil).
		WithTokenValidator(fakeTokenValidator{subject: "node-tok", role: "node"})

	ts := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "?token=valid"
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// 토큰이 검증되어 register 없이 세션이 복원되어야 한다(managed online).
	require.Eventually(t, func() bool {
		return srv.IsManaged("node-tok")
	}, 2*time.Second, 10*time.Millisecond, "유효 토큰은 세션을 복원해야 함")
}

// TestRemoteHandler_InvalidTokenFallsBackToRegister 는 무효 토큰이면 등록 경로로
// 진행(연결은 수락)하는지 검증한다(REQ-B02 — 등록은 허용).
func TestRemoteHandler_InvalidTokenFallsBackToRegister(t *testing.T) {
	srv, _ := newM2RemoteServer(t)

	h := NewRemoteHandler(srv, nil).
		WithTokenValidator(fakeTokenValidator{err: errors.New("invalid")})

	ts := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "?token=bad"
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// 무효 토큰 → 등록 경로. register 를 보내면 pending 으로 큐잉되어야 한다.
	reg, _ := remote.NewRegisterMessage(remote.RegisterPayload{InstanceID: "node-fb"})
	data, _ := reg.Encode()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-fb")
		return ok && st.Status == "pending"
	}, 2*time.Second, 10*time.Millisecond, "무효 토큰은 등록 경로로 진행해야 함")
}

// TestRemoteHandler_AdminRoleTokenNotRestored 는 admin-role 토큰으로는 노드 세션이
// 복원되지 않는지 검증한다(REQ-F03/F04 — 노드 세션은 role=node 만).
func TestRemoteHandler_AdminRoleTokenNotRestored(t *testing.T) {
	srv, repo := newM2RemoteServer(t)
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-admin", Status: "approved",
	}))

	h := NewRemoteHandler(srv, nil).
		WithTokenValidator(fakeTokenValidator{subject: "node-admin", role: "admin"})

	ts := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "?token=admintok"
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// admin role 토큰은 노드 세션 복원에 사용되지 않아야 한다.
	time.Sleep(100 * time.Millisecond)
	assert.False(t, srv.IsManaged("node-admin"), "admin-role 토큰은 노드 세션을 복원하지 않아야 함")
}
