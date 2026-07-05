// integration_m6_test.go 는 M6 end-to-end 통합 + 다중 노드 확장성 + 회귀 테스트를
// 담는다(@SPEC:SPEC-REMOTE-001 M6, REQ-N01/N03, F05/F06/F07).
//
// 메모리 Conn pipe 한 쌍으로 server.HandleConnection ↔ client 세션을 연결하여
// 등록→승인(토큰+jti)→명령 디스패치→적용→결과→인벤토리 스냅샷→서버 미러 캐시
// 반영, 그리고 폐기→재인증 거부의 전 흐름을 실제 메시지 왕복으로 검증한다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipeShared 는 양쪽 pipeConn 이 공유하는 종료 신호이다. 어느 한 쪽이라도 Close
// 하면 양쪽 읽기/쓰기를 해제한다(멱등 — close of closed channel 방지).
type pipeShared struct {
	once   sync.Once
	closed chan struct{}
}

func (s *pipeShared) close() { s.once.Do(func() { close(s.closed) }) }

// pipeConn 은 메모리 양방향 Conn 한 쪽이다. WriteMessage 는 상대의 ReadMessage 로
// 전달된다. Close 는 양쪽 읽기를 해제한다(공유 pipeShared 로 멱등).
type pipeConn struct {
	in     chan []byte // 이 쪽이 읽는 채널
	out    chan []byte // 이 쪽이 쓰는 채널(=상대의 in)
	shared *pipeShared
}

// newPipe 는 연결된 두 pipeConn 을 반환한다(a↔b).
func newPipe() (*pipeConn, *pipeConn) {
	ab := make(chan []byte, 64)
	ba := make(chan []byte, 64)
	shared := &pipeShared{closed: make(chan struct{})}
	a := &pipeConn{in: ba, out: ab, shared: shared}
	b := &pipeConn{in: ab, out: ba, shared: shared}
	return a, b
}

func (p *pipeConn) ReadMessage() ([]byte, error) {
	select {
	case data, ok := <-p.in:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-p.shared.closed:
		return nil, io.EOF
	}
}

func (p *pipeConn) WriteMessage(data []byte) error {
	// 복사하여 송신(호출자 버퍼 재사용 안전).
	cp := append([]byte(nil), data...)
	select {
	case p.out <- cp:
		return nil
	case <-p.shared.closed:
		return errors.New("pipe closed")
	}
}

func (p *pipeConn) Close() error {
	p.shared.close()
	return nil
}

// fakeApplier 는 CommandApplier 의 테스트 구현이다. 적용된 명령을 기록한다.
type fakeApplier struct {
	mu      sync.Mutex
	applied []string
	failOn  string // domain/action == failOn 이면 실패 반환
}

func (f *fakeApplier) Apply(_ context.Context, domain, action string, _ json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := domain + "/" + action
	if key == f.failOn {
		return nil, errors.New("apply failed: validation")
	}
	f.applied = append(f.applied, key)
	return json.RawMessage(`{"applied":"` + key + `"}`), nil
}

func (f *fakeApplier) appliedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.applied...)
}

// staticInventory 는 InventorySource 의 고정 테스트 구현이다(노출 범위 무시 — 이미
// redacted 라고 가정). 시크릿이 없는 정의만 반환한다(F06 준수 소스 모사).
type staticInventory struct {
	flows []InventoryItem
}

func (s staticInventory) ListFlows(_ context.Context) ([]InventoryItem, error) {
	return s.flows, nil
}
func (s staticInventory) ListAgents(_ context.Context) ([]InventoryItem, error) {
	return nil, nil
}
func (s staticInventory) ListDevices(_ context.Context) ([]InventoryItem, error) {
	return nil, nil
}

// TestM6_EndToEnd 는 등록→승인→명령→적용→결과→인벤토리→미러 캐시 반영 +
// 폐기→재인증 거부의 전 흐름을 검증한다(REQ-C01~C07/D01~D05/E01/E03/F07).
func TestM6_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := newMemManagedNodeRepo()
	mirror := newServerMirror(t)
	audit := newMemAuditRepo()
	issuer := newFakeTokenIssuer()
	srv := NewServer(ServerConfig{
		Repo: repo, Mirror: mirror, TokenIssuer: issuer, Audit: audit,
		HeartbeatTimeout: 2 * time.Second,
	}, nil)

	applier := &fakeApplier{}
	inv := staticInventory{flows: []InventoryItem{
		{ID: "f1", Name: "flow-1", Kind: KindFlow, Definition: json.RawMessage(`{"steps":1}`), UpdatedAt: 100},
	}}

	// 각 dial 마다 새 pipe 를 만들고 서버 측 HandleConnection 을 띄운다(실제 재연결을
	// 모사 — 승인 후 재접속 시 hello/토큰 경로로 세션이 복원되고 미러 루프가 시작된다).
	var (
		dialWG      sync.WaitGroup
		latestSrvMu sync.Mutex
		latestSrv   Conn
	)
	defer dialWG.Wait()
	client := NewClient(ClientConfig{
		ServerURL:             "wss://test/ignored", // dialer 주입으로 실제 dial 안 함.
		InstanceID:            "node-e2e",
		Hostname:              "host-e2e",
		Version:               "v1",
		HeartbeatInterval:     200 * time.Millisecond,
		ReconnectInitial:      20 * time.Millisecond,
		ReconnectMax:          100 * time.Millisecond,
		InventoryPollInterval: 100 * time.Millisecond,
		Exposure:              ExposureSummary{Flows: ExposeAll},
		Applier:               applier,
		Inventory:             inv,
		DataDir:               t.TempDir(), // 노드 토큰 영속(재접속 인증 — REQ-C05).
	}, DialerFunc(func(dctx context.Context, _ string) (Conn, error) {
		srvConn, cliConn := newPipe()
		latestSrvMu.Lock()
		latestSrv = srvConn
		latestSrvMu.Unlock()
		dialWG.Add(1)
		go func() {
			defer dialWG.Done()
			_ = srv.HandleConnection(dctx, srvConn)
		}()
		return cliConn, nil
	}))
	client.Start(ctx)
	defer client.Stop()

	// dropLink 는 현재 서버 측 연결을 닫아 클라이언트 재연결(토큰 경로)을 유도한다.
	dropLink := func() {
		latestSrvMu.Lock()
		c := latestSrv
		latestSrvMu.Unlock()
		if c != nil {
			_ = c.Close()
		}
	}

	// 1) 등록 → pending.
	require.Eventually(t, func() bool {
		n, err := repo.Get(ctx, "node-e2e")
		return err == nil && n.Status == RegStatusPending
	}, 2*time.Second, 10*time.Millisecond, "등록 요청 → pending 이어야 함")

	// 2) 관리자 승인 → 토큰(+jti) 발급, register_ack push.
	require.NoError(t, srv.Approve(ctx, "node-e2e"))
	require.Eventually(t, func() bool {
		return client.hasToken()
	}, 2*time.Second, 10*time.Millisecond, "승인 후 클라이언트가 노드 토큰을 보유해야 함")

	// token_id 에는 jti 가 저장되어야 한다(원본 토큰 미저장 — M6 하드닝).
	n, _ := repo.Get(ctx, "node-e2e")
	assert.NotEmpty(t, n.TokenID)
	assert.False(t, issuer.IsIDRevoked(n.TokenID))

	// 3) 승인+온라인 확인 후 명령 디스패치.
	require.Eventually(t, func() bool { return srv.IsManaged("node-e2e") },
		2*time.Second, 10*time.Millisecond)
	dctx := ContextWithActor(ctx, "admin-e2e")
	result, err := srv.Dispatch(dctx, "node-e2e", KindFlow, "deploy", json.RawMessage(`{"password":"x"}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "flow/deploy")
	assert.Contains(t, applier.appliedKeys(), "flow/deploy")

	// 명령 감사가 기록되되 시크릿이 새지 않아야 한다(F05/F06).
	require.Eventually(t, func() bool { return len(audit.all()) >= 1 }, time.Second, 10*time.Millisecond)
	for _, r := range audit.all() {
		assertNoSecret(t, r)
	}

	// 4) 인벤토리 미러: 승인된 노드가 (재)접속하면 redacted 스냅샷을 push 하고 서버
	// 미러 캐시에 반영된다(E01/E03). 승인은 register 세션 중에 이뤄졌으므로, 토큰을
	// 보유한 채 재접속하는 경로에서 미러 루프가 시작된다(M4 계약). 현재 링크를 끊어
	// 토큰 경로 재접속을 유도한다.
	dropLink()
	require.Eventually(t, func() bool {
		flows, _ := mirror.ListFlows(ctx, "node-e2e")
		return len(flows) == 1 && flows[0].ID == "f1"
	}, 3*time.Second, 20*time.Millisecond, "재접속 후 인벤토리 스냅샷이 서버 미러에 반영되어야 함")

	// 5) 폐기 → 연결 종료 + jti blacklist + 재인증 거부(C07/F07).
	// 폐기 전: 노드가 보유한 토큰(jti=n.TokenID)은 검증을 통과한다.
	revokedToken := validateTokenFor(issuer, n.TokenID)
	_, _, preErr := issuer.Validate(revokedToken)
	require.NoError(t, preErr, "폐기 전 노드 토큰은 유효해야 함")

	require.NoError(t, srv.Revoke(ctx, "node-e2e"))
	assert.True(t, issuer.IsIDRevoked(n.TokenID), "폐기 시 jti 가 blacklist 되어야 함")
	gotN, _ := repo.Get(ctx, "node-e2e")
	assert.Equal(t, RegStatusRevoked, gotN.Status)

	// 폐기 후: 동일 토큰 재인증이 거부되어야 한다(REQ-C07/F07 — DB-안전 jti 폐기).
	_, _, postErr := issuer.Validate(revokedToken)
	assert.Error(t, postErr, "폐기된 jti 토큰의 재인증은 거부되어야 함")
	assert.False(t, srv.IsManaged("node-e2e"), "폐기 후 노드는 managed 가 아니어야 함")
}

// validateTokenFor 는 fakeTokenIssuer 에서 jti 에 대응하는 토큰을 역조회한다(테스트 편의).
func validateTokenFor(f *fakeTokenIssuer, jti string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for tok, j := range f.tokenJTI {
		if j == jti {
			return tok
		}
	}
	return "no-such-token"
}

// count 는 등록된 노드 수를 반환한다(다중 노드 테스트 편의).
func (m *memManagedNodeRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes)
}

// TestM6_MultiNodeScalability 는 N 개 노드 동시 연결 + 인벤토리 미러를 -race 로
// 검증한다(REQ-N01): 노드 간 격리(미러 cross-leak 없음), disconnect-all 후 자원
// 정리(online=false), goroutine leak 없음.
func TestM6_MultiNodeScalability(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := newMemManagedNodeRepo()
	mirror := newServerMirror(t)
	issuer := newFakeTokenIssuer()
	srv := NewServer(ServerConfig{
		Repo: repo, Mirror: mirror, TokenIssuer: issuer,
		HeartbeatTimeout: time.Second,
	}, nil)

	const n = 12
	baseGoroutines := runtime.NumGoroutine()

	clients := make([]*Client, n)
	cancels := make([]context.CancelFunc, n)
	dropLinks := make([]func(), n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		id := nodeID(i)
		nodeCtx, nodeCancel := context.WithCancel(ctx)
		cancels[i] = nodeCancel

		var (
			linkMu   sync.Mutex
			lastConn Conn
		)
		dropLinks[i] = func() {
			linkMu.Lock()
			c := lastConn
			linkMu.Unlock()
			if c != nil {
				_ = c.Close()
			}
		}

		inv := staticInventory{flows: []InventoryItem{
			{ID: "flow-" + id, Name: id, Kind: KindFlow, Definition: json.RawMessage(`{}`), UpdatedAt: 1},
		}}
		c := NewClient(ClientConfig{
			ServerURL:             "wss://test",
			InstanceID:            id,
			HeartbeatInterval:     100 * time.Millisecond,
			ReconnectInitial:      20 * time.Millisecond,
			ReconnectMax:          100 * time.Millisecond,
			InventoryPollInterval: 100 * time.Millisecond,
			Exposure:              ExposureSummary{Flows: ExposeAll},
			Inventory:             inv,
			Applier:               &fakeApplier{},
			DataDir:               t.TempDir(), // 토큰 영속(재접속 인증 — REQ-C05).
		}, DialerFunc(func(dctx context.Context, _ string) (Conn, error) {
			srvConn, cliConn := newPipe()
			linkMu.Lock()
			lastConn = srvConn
			linkMu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = srv.HandleConnection(dctx, srvConn)
			}()
			return cliConn, nil
		}))
		c.Start(nodeCtx)
		clients[i] = c
	}

	// 모든 노드를 승인하여 인벤토리 미러링을 활성화한다.
	require.Eventually(t, func() bool {
		return repo.count() == n
	}, 3*time.Second, 20*time.Millisecond, "모든 노드가 등록되어야 함")
	for i := 0; i < n; i++ {
		require.NoError(t, srv.Approve(ctx, nodeID(i)))
	}
	// 토큰 보유 재접속 경로에서 미러 루프가 시작되도록 각 노드 링크를 한 번 끊는다.
	require.Eventually(t, func() bool {
		for i := 0; i < n; i++ {
			if !clients[i].hasToken() {
				return false
			}
		}
		return true
	}, 3*time.Second, 20*time.Millisecond, "모든 노드가 토큰을 수신해야 함")
	for i := 0; i < n; i++ {
		dropLinks[i]()
	}

	// 각 노드의 미러가 자기 자원만 보유해야 한다(cross-node leak 없음 — 격리).
	require.Eventually(t, func() bool {
		for i := 0; i < n; i++ {
			flows, _ := mirror.ListFlows(ctx, nodeID(i))
			if len(flows) != 1 || flows[0].ID != "flow-"+nodeID(i) {
				return false
			}
		}
		return true
	}, 4*time.Second, 25*time.Millisecond, "각 노드 미러는 자기 자원만 보유해야 함(격리)")

	// 통합 목록은 정확히 n 개(노드당 1) — 자원 총량 예측 가능(N01).
	all, err := mirror.ListAllFlows(ctx)
	require.NoError(t, err)
	assert.Len(t, all, n)

	// disconnect-all: 모든 노드를 종료한다.
	for i := 0; i < n; i++ {
		clients[i].Stop()
		cancels[i]()
	}
	wg.Wait()

	// 모든 노드가 offline 으로 표시되어야 한다(last-known 보존 — B06).
	require.Eventually(t, func() bool {
		for i := 0; i < n; i++ {
			if srv.IsManaged(nodeID(i)) {
				return false
			}
		}
		return true
	}, 3*time.Second, 25*time.Millisecond, "disconnect 후 어떤 노드도 managed 가 아니어야 함")

	// goroutine leak 검사: 정리 후 베이스라인 근처로 수렴해야 한다(엄격 동치 대신 여유).
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseGoroutines+n // 여유 폭(스케줄러 지연).
	}, 3*time.Second, 50*time.Millisecond, "disconnect-all 후 goroutine 이 정리되어야 함")
}

func nodeID(i int) string {
	return "node-" + string(rune('A'+i))
}
