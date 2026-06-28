// shared_bridge_e2e_test.go 는 로컬 shared 모드의 생명주기/자가치유/다중참조를 실제 엔진 +
// FlowServiceAdapter + 로컬 in-process opener 로 종단 검증한다(@SPEC:SPEC-SUBFLOW-002 그룹
// SH/MR/L, AC-2/AC-5/AC-6/AC-7/AC-8/AC-4).
//
// 구성:
//
//	참조 플로우 R = [input "X" → relay(transform passthrough) → output "Y"], 경계 포트 X/Y.
//	  R 배포 시 경계가 공유 경계 탭(flow_id=R)으로 노출된다.
//	부모 P = [inFwd(input tap 역할 trigger 미사용 — 주입은 브리지 입력으로) ...]
//	  본 e2e 는 엔진 라우팅 자체보다 "브리지 연결/자가치유/fan-in·fan-out"을 검증하므로,
//	  부모 배포로 생성된 매니저 브리지 컨트롤러의 연결 상태와 브리지 입출력을 직접 관찰한다.
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// newSharedTestEnv 는 shared 모드 e2e 용 엔진/어댑터/opener 를 구성한다(공유 경계 탭 + 브리지
// fwd/emit 노드 타입 등록 + 로컬 opener 주입).
func newSharedTestEnv(t *testing.T) (*FlowServiceAdapter, *engine.Engine) {
	t.Helper()
	reg := node.NewRegistry()
	require.NoError(t, RegisterSharedBoundaryNodes(reg))
	require.NoError(t, RegisterRemoteBridgeNodes(reg))
	eng := engine.NewEngine(engine.WithNodeRegistry(reg))
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	adapter.SetLocalBridgeOpener(NewLocalBridgeOpener(eng, nil))
	return adapter, eng
}

// makeRefFlow 는 입력 X / 출력 Y 경계 포트를 가진 참조 플로우(passthrough)를 생성·저장한다.
func makeRefFlow(t *testing.T, adapter *FlowServiceAdapter, id string) flow.Flow {
	t.Helper()
	ref := flow.NewFlowWithID(id, id,
		flow.WithNodes(passthroughNode("relay", "릴레이")),
		flow.WithWires(
			wire("bi", flow.FlowInputBoundaryID, "X", "relay", "in"),
			wire("bo", "relay", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	require.NoError(t, adapter.repo.Save(context.Background(), ref))
	return ref
}

// deployAndStartParentWithShared 는 sharedFN 1개(또는 다수)를 가진 부모 플로우를 생성·저장·
// 배포·시작한다. 본 e2e 는 브리지 컨트롤러 관찰이 목적이므로 부모는 flow-node 만 둔다(상류/하류
// 노드 없이도 fwd/emit 가 생성되어 컨트롤러가 등록된다).
//
// 매니저 브리지 컨트롤러의 start(OpenBridge)는 fwd 노드 Init 에서 구동되며, 노드 Init 은
// 엔진 StartFlow 시점에 실행된다(engine.go). 따라서 컨트롤러 연결을 관찰하려면 부모를
// 배포 후 시작해야 한다.
func deployAndStartParentWithShared(t *testing.T, adapter *FlowServiceAdapter, eng *engine.Engine, parentID string, refIDs ...string) {
	t.Helper()
	nodes := make([]flow.NodeDef, 0, len(refIDs))
	for i, ref := range refIDs {
		fnID := "fn" + itoa(i)
		nodes = append(nodes, flowNodeShared(fnID, "공유"+fnID, ref,
			[]flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, true))
	}
	parent := flow.NewFlowWithID(parentID, parentID, flow.WithNodes(nodes...))
	require.NoError(t, adapter.repo.Save(context.Background(), parent))
	require.NoError(t, adapter.DeployFlow(context.Background(), parentID))
	require.NoError(t, eng.StartFlow(context.Background(), parentID))
}

// deployAndStartParentWithSharedPorts 는 단일 shared flow-node 를 가지되, 입출력 핸들 포트를
// 명시 지정한 부모 플로우를 생성·저장·배포·시작한다(다중 출력/입력 경계 포트 검증용).
func deployAndStartParentWithSharedPorts(t *testing.T, adapter *FlowServiceAdapter, eng *engine.Engine, parentID, refID string, inputs, outputs []flow.Port) {
	t.Helper()
	fn := flowNodeShared("fn0", "공유fn0", refID, inputs, outputs, true)
	parent := flow.NewFlowWithID(parentID, parentID, flow.WithNodes(fn))
	require.NoError(t, adapter.repo.Save(context.Background(), parent))
	require.NoError(t, adapter.DeployFlow(context.Background(), parentID))
	require.NoError(t, eng.StartFlow(context.Background(), parentID))
}

// firstController 는 어댑터가 parentID 배포에서 등록한 첫 매니저 브리지 컨트롤러를 반환한다.
func firstController(t *testing.T, adapter *FlowServiceAdapter, parentID string) *managerBridgeController {
	t.Helper()
	adapter.bridgeMu.Lock()
	defer adapter.bridgeMu.Unlock()
	ctrls := adapter.flowBridges[parentID]
	require.NotEmpty(t, ctrls, "부모 배포에서 매니저 브리지 컨트롤러가 등록되어야 한다")
	return ctrls[0]
}

// TestSharedE2E_연결_양방향 은 참조 플로우 실행 중 부모 배포 시 브리지가 연결되고, 입력 주입이
// 참조 플로우 경계로 들어가는지 검증한다(AC-2 SH02/SH03).
func TestSharedE2E_연결_양방향(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	// 참조 플로우 R 배포 + 시작(공유 경계 탭 설치).
	makeRefFlow(t, adapter, "R")
	require.NoError(t, adapter.DeployFlow(ctx, "R"))
	require.NoError(t, eng.StartFlow(ctx, "R"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "R"); _ = adapter.UndeployFlow(ctx, "R") })

	// 공유 경계 컨트롤러가 등록되어야 한다.
	require.Eventually(t, func() bool {
		_, ok := globalSharedBoundaryTable.lookup("R")
		return ok
	}, 2*time.Second, 20*time.Millisecond, "참조 플로우 배포 시 공유 경계 컨트롤러 등록")

	// 부모 P 배포·시작(shared flow-node → R).
	deployAndStartParentWithShared(t, adapter, eng, "P", "R")
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "P"); _ = adapter.UndeployFlow(ctx, "P") })

	ctrl := firstController(t, adapter, "P")

	// 브리지가 연결될 때까지 대기(running status — 연결 성공 신호).
	requireBridgeConnected(t, ctrl)

	// 라운드트립(SH03): forwardInput(X) → R 입력 경계 → relay passthrough → R 출력 경계(Y) →
	// fan-out → 부모 컨트롤러 onOutput. marker 가 출력으로 돌아오면 양방향 연결 성공.
	got := make(chan BridgeOutput, 4)
	ctrl.setOnOutput(func(p string, d json.RawMessage) { trySend(got, BridgeOutput{Port: p, Data: d}) })

	require.Eventually(t, func() bool {
		_ = ctrl.forwardInput("X", json.RawMessage(`{"marker":"rt-1"}`))
		select {
		case o := <-got:
			return o.Port == "Y" && string(o.Data) != ""
		default:
			return false
		}
	}, 5*time.Second, 50*time.Millisecond, "입력 주입이 R 을 거쳐 출력으로 돌아와야 함(SH03 양방향)")
}

// requireBridgeConnected 는 컨트롤러의 라이브 브리지가 연결될 때까지 대기한다(running status).
func requireBridgeConnected(t *testing.T, ctrl *managerBridgeController) {
	t.Helper()
	require.Eventually(t, func() bool {
		ctrl.mu.Lock()
		defer ctrl.mu.Unlock()
		return ctrl.bridge != nil
	}, 5*time.Second, 50*time.Millisecond, "라이브 브리지 연결 대기")
}

// TestSharedE2E_오프라인_자가치유 는 참조 플로우 미실행 시 부모 배포가 오프라인 대기하고(L02),
// 참조 플로우 시작 시 자가치유 연결되는지 검증한다(AC-5/AC-6 L01~L03).
func TestSharedE2E_오프라인_자가치유(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	// 참조 플로우 R 은 저장만 하고 배포/시작하지 않는다(미실행).
	makeRefFlow(t, adapter, "R")

	// 부모 P 배포·시작 — R 미실행이므로 브리지는 오프라인 대기(배포/시작은 실패하지 않아야 함 — L02).
	deployAndStartParentWithShared(t, adapter, eng, "P", "R")
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "P"); _ = adapter.UndeployFlow(ctx, "P") })

	ctrl := firstController(t, adapter, "P")

	// 오프라인 상태: forwardInput 은 조용히 드롭(브리지 nil)되어야 한다(무출력 — L02).
	require.NoError(t, ctrl.forwardInput("X", json.RawMessage(`{"v":0}`)), "오프라인 주입은 조용히 드롭")

	// 이제 참조 플로우를 (독립적으로) 시작한다 → 자가치유 연결(L03).
	require.NoError(t, adapter.DeployFlow(ctx, "R"))
	require.NoError(t, eng.StartFlow(ctx, "R"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "R"); _ = adapter.UndeployFlow(ctx, "R") })

	// 자가치유 연결 후 라운드트립이 성립해야 한다(L03).
	requireRoundTrip(t, ctrl, "참조 플로우 시작 후 자가치유 연결되어 라운드트립이 성립해야 함(L03)")
}

// requireRoundTrip 은 forwardInput(X) → R passthrough → 출력(Y) 라운드트립이 성립할 때까지
// 대기한다(브리지 연결 + 양방향 라우팅 검증). 입력 채널은 엔진이 드레인하므로 길이가 아니라
// 출력 도달로 연결을 확인한다.
func requireRoundTrip(t *testing.T, ctrl *managerBridgeController, msg string) {
	t.Helper()
	got := make(chan BridgeOutput, 4)
	ctrl.setOnOutput(func(p string, d json.RawMessage) { trySend(got, BridgeOutput{Port: p, Data: d}) })
	require.Eventually(t, func() bool {
		_ = ctrl.forwardInput("X", json.RawMessage(`{"marker":"rt"}`))
		select {
		case o := <-got:
			return o.Port == "Y"
		default:
			return false
		}
	}, 6*time.Second, 50*time.Millisecond, msg)
}

// TestSharedE2E_정지_끊김_재시작_자가치유 는 연결 후 참조 플로우 정지 시 끊김(L04), 재시작 시
// 자가치유(L05)를 검증한다(AC-7).
func TestSharedE2E_정지_끊김_재시작_자가치유(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	makeRefFlow(t, adapter, "R")
	require.NoError(t, adapter.DeployFlow(ctx, "R"))
	require.NoError(t, eng.StartFlow(ctx, "R"))

	deployAndStartParentWithShared(t, adapter, eng, "P", "R")
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "P"); _ = adapter.UndeployFlow(ctx, "P") })

	ctrl := firstController(t, adapter, "P")

	// 최초 연결 확인(라운드트립).
	requireRoundTrip(t, ctrl, "최초 연결 라운드트립")

	// 참조 플로우 정지 → 공유 경계 컨트롤러 등록 해제(L04).
	require.NoError(t, eng.StopFlow(ctx, "R"))
	require.NoError(t, adapter.UndeployFlow(ctx, "R"))
	require.Eventually(t, func() bool {
		_, ok := globalSharedBoundaryTable.lookup("R")
		return !ok
	}, 3*time.Second, 50*time.Millisecond, "정지 시 공유 경계 컨트롤러가 사라져야 함(L04)")

	// 참조 플로우 재시작 → 자가치유 재연결(L05). 부모는 재배포하지 않는다.
	require.NoError(t, adapter.DeployFlow(ctx, "R"))
	require.NoError(t, eng.StartFlow(ctx, "R"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "R"); _ = adapter.UndeployFlow(ctx, "R") })

	requireRoundTrip(t, ctrl, "재시작 후 부모 재배포 없이 자가치유 재연결되어야 함(L05)")
}

// TestSharedE2E_다중참조_fanout_teardown격리 는 두 부모 B/C 가 같은 R 에 연결될 때 단일
// 인스턴스 fan-out(MR03)과, 한 부모 undeploy 시 teardown 격리(L07)를 검증한다(AC-8/AC-3).
func TestSharedE2E_다중참조_fanout_teardown격리(t *testing.T) {
	adapter, eng := newSharedTestEnv(t)
	ctx := context.Background()

	makeRefFlow(t, adapter, "R")
	require.NoError(t, adapter.DeployFlow(ctx, "R"))
	require.NoError(t, eng.StartFlow(ctx, "R"))
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "R"); _ = adapter.UndeployFlow(ctx, "R") })

	deployAndStartParentWithShared(t, adapter, eng, "B", "R")
	deployAndStartParentWithShared(t, adapter, eng, "C", "R")
	t.Cleanup(func() { _ = eng.StopFlow(ctx, "C"); _ = adapter.UndeployFlow(ctx, "C") })

	ctrlB := firstController(t, adapter, "B")
	ctrlC := firstController(t, adapter, "C")

	// 두 부모가 같은 단일 인스턴스(공유 경계 컨트롤러)에 연결되어야 한다(MR01).
	sbCtrl := requireSharedCtrl(t, "R")
	require.Eventually(t, func() bool {
		sbCtrl.mu.Lock()
		n := len(sbCtrl.subscribers)
		sbCtrl.mu.Unlock()
		return n >= 2
	}, 5*time.Second, 50*time.Millisecond, "두 부모가 단일 인스턴스에 fan-out subscriber 로 attach(MR01/MR03)")

	// 출력 fan-out: 컨트롤러 emitOutput → 두 부모 컨트롤러 onOutput.
	gotB := make(chan BridgeOutput, 1)
	gotC := make(chan BridgeOutput, 1)
	ctrlB.setOnOutput(func(p string, d json.RawMessage) { trySend(gotB, BridgeOutput{Port: p, Data: d}) })
	ctrlC.setOnOutput(func(p string, d json.RawMessage) { trySend(gotC, BridgeOutput{Port: p, Data: d}) })
	sbCtrl.emitOutput("Y", json.RawMessage(`{"fan":"out"}`))

	for name, ch := range map[string]chan BridgeOutput{"B": gotB, "C": gotC} {
		select {
		case o := <-ch:
			assert.JSONEq(t, `{"fan":"out"}`, string(o.Data), "부모 %s fan-out", name)
		case <-time.After(2 * time.Second):
			t.Fatalf("부모 %s 가 fan-out 출력을 받지 못함", name)
		}
	}

	// teardown 격리(L07): 부모 B 정지·undeploy → R 인스턴스·C 브리지 불변.
	require.NoError(t, eng.StopFlow(ctx, "B"))
	require.NoError(t, adapter.UndeployFlow(ctx, "B"))
	// R 은 여전히 running, 공유 경계 컨트롤러도 살아 있어야 한다.
	st, err := eng.GetFlowStatus("R")
	require.NoError(t, err)
	assert.Equal(t, flow.FlowRunning, st.State, "B undeploy 후에도 R 인스턴스는 계속 실행(L06/L07)")

	// C 는 여전히 출력을 받아야 한다.
	gotC2 := make(chan BridgeOutput, 1)
	ctrlC.setOnOutput(func(p string, d json.RawMessage) { trySend(gotC2, BridgeOutput{Port: p, Data: d}) })
	sbCtrl.emitOutput("Y", json.RawMessage(`{"still":"alive"}`))
	select {
	case o := <-gotC2:
		assert.JSONEq(t, `{"still":"alive"}`, string(o.Data))
	case <-time.After(2 * time.Second):
		t.Fatal("teardown 격리 실패: 남은 부모 C 가 출력을 못 받음(L07)")
	}
}

// requireSharedCtrl 은 flow_id 의 공유 경계 컨트롤러를 대기·반환한다(테스트 헬퍼).
func requireSharedCtrl(t *testing.T, flowID string) *sharedBoundaryController {
	t.Helper()
	var ctrl *sharedBoundaryController
	require.Eventually(t, func() bool {
		c, ok := globalSharedBoundaryTable.lookup(flowID)
		if ok {
			ctrl = c
		}
		return ok
	}, 3*time.Second, 20*time.Millisecond, "공유 경계 컨트롤러 등록 대기(flow_id=%s)", flowID)
	return ctrl
}

// trySend 는 비블로킹 송신 헬퍼이다(버퍼 1 채널로 마지막 값 보관).
func trySend(ch chan BridgeOutput, v BridgeOutput) {
	select {
	case ch <- v:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}
