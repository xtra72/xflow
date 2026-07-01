package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/script"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// newTestStoreAgent 는 Init 된 실 StoreAgent 를 생성한다.
func newTestStoreAgent(t *testing.T) *system.StoreAgent {
	t.Helper()
	ag := system.NewStoreAgent()
	require.NoError(t, ag.Init(context.Background()))
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag
}

// newStoreScriptEngine 은 stdlib(store 포함)가 활성화된 실 스크립트 엔진을 만든다.
func newStoreScriptEngine(t *testing.T) *script.DefaultScriptEngine {
	t.Helper()
	eng := script.NewScriptEngine(script.WithStdlib(
		script.StdlibOptions{EnableStore: true},
		script.StdlibDeps{},
	))
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	return eng
}

// newScriptNodeWithStore 는 프로덕션 형태로 ScriptNode 를 만든다:
//   - 엔진: WithScriptEngine(NodeEngineAdapter) 옵션
//   - 스토어: WithAgentResolver 옵션 + def.AgentRef + namespace(Configure)
//
// resolver/ref/namespace 로 네임스페이스 스토어를 해석한다.
func newScriptNodeWithStore(t *testing.T, eng *script.DefaultScriptEngine, nodeID, src, namespace string, resolver AgentResolver) *ScriptNode {
	t.Helper()
	def := flow.NodeDef{
		ID: nodeID, Type: "script", Name: nodeID,
		AgentRef: &flow.AgentRef{AgentName: "store-1"},
	}
	adapter := script.NewNodeEngineAdapter(eng, nodeID)
	n, err := NewScriptNode(def, WithScriptEngine(adapter), WithAgentResolver(resolver))
	require.NoError(t, err)
	sn := n.(*ScriptNode)
	require.NoError(t, sn.Configure(map[string]any{"script": src, "namespace": namespace}))
	require.NoError(t, sn.Init(context.Background()))
	return sn
}

// runProcess 는 빈 payload 메시지로 노드를 1회 실행한다.
func runProcess(t *testing.T, n *ScriptNode) message.Message {
	t.Helper()
	out, err := n.Process(context.Background(), message.New(message.WithID("m1")))
	require.NoError(t, err)
	require.Len(t, out, 1)
	return out[0]
}

// newStoreResolver 는 주어진 StoreAgent 를 감싸는 AgentResolver 를 만든다(프로덕션
// 해석 경로: AgentResolver → AgentAccessor → storeProvider → NodeStoreForNamespace).
func newStoreResolver(ag *system.StoreAgent) AgentResolver {
	return &fakeStoreResolver{agent: &fakeStoreProviderAgent{BaseAgent: agent.NewBaseAgent(), store: ag}}
}

// TestScriptStore_SetGet_EndToEnd 는 스크립트 노드가 xflow.store.set 후 get 으로
// 올바른 네임스페이스 스토어에서 값을 읽어옴을 프로덕션 경로(ScriptNode →
// AgentResolver 해석 → 어댑터 → 엔진 → per-exec 바인딩)로 검증한다.
func TestScriptStore_SetGet_EndToEnd(t *testing.T) {
	ag := newTestStoreAgent(t)
	eng := newStoreScriptEngine(t)
	resolver := newStoreResolver(ag)

	// set 스크립트를 1회 실행(namespace flow-a).
	setNode := newScriptNodeWithStore(t, eng, "set-node",
		`xflow.store.set("k", "v-a"); return { payload = { done = true } }`, "flow-a", resolver)
	runProcess(t, setNode)

	// get 스크립트로 값을 읽어 payload 로 반환.
	getNode := newScriptNodeWithStore(t, eng, "get-node",
		`return { payload = { got = xflow.store.get("k") } }`, "flow-a", resolver)
	out := runProcess(t, getNode)

	got, ok := out.Payload().Get("got")
	require.True(t, ok, "payload.got 이 있어야 함")
	assert.Equal(t, "v-a", got)

	// 하위 스토어에도 실제로 기록되었는지 직접 확인(네임스페이스 접두사 경유).
	entry, err := ag.ForNamespace("flow-a").Get(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, "v-a", entry.Value)
}

// TestScriptStore_NamespaceIsolation 는 서로 다른 네임스페이스를 쓰는 두 스크립트
// 노드가 같은 키에 대해 서로의 값을 읽지 못함을(격리) 검증한다. 두 노드는 동일한
// 풀링 VM(poolSize=1)을 공유하지만 per-execution 바인딩으로 각자의 네임스페이스만 본다.
func TestScriptStore_NamespaceIsolation(t *testing.T) {
	ag := newTestStoreAgent(t)
	resolver := newStoreResolver(ag)
	// poolSize=1 로 강제하여 두 노드가 반드시 같은 VM 을 공유하게 만든다
	// (바인딩 격리가 VM 공유에도 유지됨을 증명).
	eng := script.NewScriptEngine(
		script.WithStdlib(script.StdlibOptions{EnableStore: true}, script.StdlibDeps{}),
		script.WithPoolSize(1),
	)
	require.NoError(t, eng.Init(context.Background()))
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	// 노드 A: 네임스페이스 flow-a 에 shared_key="from-a" 기록.
	nodeA := newScriptNodeWithStore(t, eng, "node-a",
		`xflow.store.set("shared_key", "from-a"); return { payload = { ok = true } }`, "flow-a", resolver)
	runProcess(t, nodeA)

	// 노드 B: 같은 VM 을 공유하지만 flow-b 네임스페이스이므로 shared_key 는 없어야 한다.
	nodeBRead := newScriptNodeWithStore(t, eng, "node-b-read",
		`local v = xflow.store.get("shared_key"); if v == nil then return { payload = { got = "nil" } } end; return { payload = { got = v } }`, "flow-b", resolver)
	outB := runProcess(t, nodeBRead)
	gotB, _ := outB.Payload().Get("got")
	assert.Equal(t, "nil", gotB, "flow-b 는 flow-a 의 shared_key 를 읽으면 안 된다(격리)")

	// 노드 A 는 자기 네임스페이스에서 여전히 값을 읽을 수 있어야 한다.
	nodeARead := newScriptNodeWithStore(t, eng, "node-a-read",
		`return { payload = { got = xflow.store.get("shared_key") } }`, "flow-a", resolver)
	outA := runProcess(t, nodeARead)
	gotA, _ := outA.Payload().Get("got")
	assert.Equal(t, "from-a", gotA, "flow-a 는 자기 값을 읽을 수 있어야 한다")

	// 하위 스토어 직접 확인: flow-b 네임스페이스엔 shared_key 가 없다.
	_, err := ag.ForNamespace("flow-b").Get(context.Background(), "shared_key")
	assert.Error(t, err, "flow-b 네임스페이스엔 shared_key 가 없어야 한다")
}

// TestScriptStore_NoStoreConfigured_Graceful 는 스토어 미구성 노드(agent_ref 없음)에서
// xflow.store 가 nil-safe(get→nil, has→false, set/delete→no-op) 로 동작하고 스크립트가
// 정상 완료됨을 검증한다.
func TestScriptStore_NoStoreConfigured_Graceful(t *testing.T) {
	eng := newStoreScriptEngine(t)
	// agent_ref/resolver 없음 → 바인딩 없음.
	def := flow.NodeDef{ID: "no-store", Type: "script", Name: "no-store"}
	adapter := script.NewNodeEngineAdapter(eng, "no-store")
	n, err := NewScriptNode(def, WithScriptEngine(adapter))
	require.NoError(t, err)
	sn := n.(*ScriptNode)
	require.NoError(t, sn.Configure(map[string]any{"script": `
		local before = xflow.store.get("k")
		xflow.store.set("k", "v")   -- no-op
		xflow.store.delete("k")     -- no-op
		local has = xflow.store.has("k")
		return { payload = { before = (before == nil) and "nil" or "not-nil", has = has } }
	`}))
	require.NoError(t, sn.Init(context.Background()))

	out := runProcess(t, sn)
	before, _ := out.Payload().Get("before")
	has, _ := out.Payload().Get("has")
	assert.Equal(t, "nil", before, "미바인딩 시 get 은 nil")
	assert.Equal(t, false, has, "미바인딩 시 has 는 false")
}

// ---------------------------------------------------------------------------
// 테스트용 fake AgentResolver / storeProvider 에이전트
// ---------------------------------------------------------------------------

// fakeStoreProviderAgent 는 agent.Agent(BaseAgent 임베딩) 와 storeProvider
// (NodeStoreForNamespace)를 함께 구현하는 최소 에이전트이다.
type fakeStoreProviderAgent struct {
	*agent.BaseAgent
	store *system.StoreAgent
}

// NodeStoreForNamespace 는 네임스페이스별 NodeStoreAdapter 를 반환한다(storeProvider 구현).
func (a *fakeStoreProviderAgent) NodeStoreForNamespace(namespace string) any {
	return system.NewNodeStoreAdapter(a.store.ForNamespace(namespace))
}

// fakeStoreTransport 는 AgentTransport + AgentAccessor 를 구현하여 원본 에이전트에 접근하게 한다.
type fakeStoreTransport struct {
	agent *fakeStoreProviderAgent
}

func (t *fakeStoreTransport) Send(_ context.Context, _ message.Message) error { return nil }
func (t *fakeStoreTransport) Receive(_ context.Context) (message.Message, error) {
	return nil, context.Canceled
}

// UnderlyingAgent 는 AgentAccessor 를 구현한다. storeProvider 로 단언될 값을 반환한다.
func (t *fakeStoreTransport) UnderlyingAgent() agent.Agent { return t.agent }

// fakeStoreResolver 는 AgentResolver 를 구현하여 fakeStoreTransport 를 반환한다.
type fakeStoreResolver struct {
	agent *fakeStoreProviderAgent
}

func (r *fakeStoreResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	return &fakeStoreTransport{agent: r.agent}, nil
}
