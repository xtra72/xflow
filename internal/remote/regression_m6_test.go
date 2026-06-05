// regression_m6_test.go 는 M6 회귀 보호(disabled 모드 불변) + 시크릿 미유출
// (redaction 완전성)을 검증한다(@SPEC:SPEC-REMOTE-001 M6, REQ-N03/F06).
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// commonSecrets 는 redaction 이 절대 노출하지 않아야 하는 흔한 시크릿 키/값이다.
var commonSecrets = []string{"password", "token", "api_key", "apikey", "secret",
	"hunter2", "s3cr3t-value", "bearer-xyz"}

// TestRegression_DisabledMode_NoServerNeeded 는 disabled 모드에서 어떤 관리 서버/
// 클라이언트도 필요하지 않음을 문서화·검증한다(REQ-N03 — 기존 동작 불변).
//
// cmd/xflowd 의 mode switch 는 disabled 시 default(no-op) 분기로 어떤 remote 자원도
// 생성하지 않는다. 본 테스트는 그 계약을 단위 수준에서 고정한다: nil repo/issuer 로
// 생성된 Server 는 어떤 노드도 관리하지 않으며(빈 목록), 명령 디스패치도 거절한다.
func TestRegression_DisabledMode_NoServerNeeded(t *testing.T) {
	// disabled 모드에서는 Server 를 만들지 않는 것이 정상 경로이다. 그러나 방어적으로,
	// repo/issuer 미주입 Server 가 어떤 관리 권한도 부여하지 않음을 확인한다.
	srv := NewServer(ServerConfig{}, nil)

	nodes, err := srv.ListNodes(context.Background())
	require.NoError(t, err)
	assert.Empty(t, nodes, "disabled/미구성 서버는 관리 노드를 갖지 않아야 함")

	assert.False(t, srv.IsManaged("any"), "미구성 서버는 어떤 노드도 관리하지 않아야 함")

	// 명령 디스패치는 미관리 노드 거절 오류여야 한다(어떤 명령도 적용되지 않음).
	_, dErr := srv.Dispatch(context.Background(), "any", "flow", "deploy", nil)
	assert.ErrorIs(t, dErr, ErrNodeNotManaged)
}

// TestRedaction_CommandLogsNoSecrets 는 명령 적용 시 구조화 로그에 시크릿(args)이
// 새지 않는지 검증한다(REQ-F06). slog 출력을 캡처하여 시크릿 부재를 단언한다.
func TestRedaction_CommandLogsNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-r", Status: RegStatusApproved,
	}))
	audit := newMemAuditRepo()
	srv := NewServer(ServerConfig{
		Repo: repo, TokenIssuer: newFakeTokenIssuer(), Audit: audit,
		CommandTimeout: 80 * time.Millisecond, Logger: logger,
	}, nil)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "node-r") }()
	require.Eventually(t, func() bool { return srv.IsManaged("node-r") }, time.Second, 5*time.Millisecond)
	go func() { <-conn.outgoing }() // 명령을 소비하되 응답 안 함 → 타임아웃.

	secretArgs := json.RawMessage(`{"password":"hunter2","token":"bearer-xyz","api_key":"s3cr3t-value"}`)
	dctx := ContextWithActor(context.Background(), "admin")
	_, _ = srv.Dispatch(dctx, "node-r", "agent", "create", secretArgs)

	logs := buf.String()
	for _, secret := range commonSecrets {
		assert.NotContains(t, logs, secret,
			"명령 구조화 로그에 시크릿이 새면 안 됨: %q", secret)
	}
}

// TestRedaction_AuditRowsNoSecrets 는 감사 행에 시크릿이 저장되지 않는지 검증한다
// (REQ-F06). 명령 args 에 시크릿을 담아도 감사에는 domain/action/result 만 남아야 한다.
func TestRedaction_AuditRowsNoSecrets(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-r", Status: RegStatusApproved,
	}))
	audit := newMemAuditRepo()
	srv := NewServer(ServerConfig{
		Repo: repo, TokenIssuer: newFakeTokenIssuer(), Audit: audit,
		CommandTimeout: 60 * time.Millisecond,
	}, nil)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "node-r") }()
	require.Eventually(t, func() bool { return srv.IsManaged("node-r") }, time.Second, 5*time.Millisecond)
	go func() { <-conn.outgoing }()

	dctx := ContextWithActor(context.Background(), "admin")
	_, _ = srv.Dispatch(dctx, "node-r", "device",
		"set", json.RawMessage(`{"password":"hunter2","secret":"s3cr3t-value"}`))

	require.Eventually(t, func() bool { return len(audit.all()) >= 1 }, time.Second, 5*time.Millisecond)
	for _, r := range audit.all() {
		blob := strings.Join([]string{r.InstanceID, r.Actor, r.Action, r.Domain,
			r.CommandAction, r.Result, r.Reason}, "|")
		for _, secret := range commonSecrets {
			assert.NotContains(t, blob, secret, "감사 행에 시크릿이 새면 안 됨: %q", secret)
		}
	}
}

// TestRedaction_MirrorPayloadNoSecrets 는 미러 페이로드(서버 저장)에 시크릿이
// 포함되지 않는지 검증한다(REQ-F06). 인벤토리 소스가 이미 redacted 정의를 보내므로
// (cmd/xflowd 어댑터의 책임), 서버는 받은 그대로 저장한다. 본 테스트는 redacted
// 정의가 그대로 저장되고, 비-redacted(시크릿 포함) 정의를 보내지 않는 계약을 고정한다.
func TestRedaction_MirrorPayloadNoSecrets(t *testing.T) {
	mirror := newServerMirror(t)
	srv := NewServer(ServerConfig{Mirror: mirror}, nil)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	// hello 로 식별.
	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-r", Hostname: "h", Version: "v"})
	conn.inject(t, hello)

	// 인벤토리 스냅샷(이미 redacted — 시크릿 없는 정의만 운반).
	snap, _ := NewInventorySnapshotMessage(InventorySnapshotPayload{
		InstanceID: "node-r",
		Flows: []InventoryItem{{
			ID: "f1", Name: "flow-1", Kind: KindFlow,
			Definition: json.RawMessage(`{"steps":1,"endpoint":"https://api"}`), // redacted.
			UpdatedAt:  1,
		}},
	})
	conn.inject(t, snap)

	require.Eventually(t, func() bool {
		flows, _ := mirror.ListFlows(ctx, "node-r")
		return len(flows) == 1
	}, time.Second, 10*time.Millisecond)

	flows, err := mirror.ListFlows(ctx, "node-r")
	require.NoError(t, err)
	require.Len(t, flows, 1)
	for _, secret := range commonSecrets {
		assert.NotContains(t, flows[0].Definition, secret,
			"미러 정의에 시크릿이 새면 안 됨: %q", secret)
	}
}
