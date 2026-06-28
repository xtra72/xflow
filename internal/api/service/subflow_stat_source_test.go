// subflow_stat_source_test.go 는 SPEC-SUBFLOW-002 M4(통계 정합)의 출처 분류·표식과 이중계상
// 방지 필터를 검증한다(@SPEC:SPEC-SUBFLOW-002 그룹 S, REQ-SUBFLOW2-S01/S02/S03, AC-16).
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 순수 함수 — classifyStatSource (출처 분류)
// ---------------------------------------------------------------------------

// 출처 조합별 표식이 결정적이어야 한다(S03).
func TestClassifyStatSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		inDirect, inEmbed bool
		want              string
	}{
		{"direct 전용 → direct(S01)", true, false, statSourceDirect},
		{"embedded 전용 → embedded(S02)", false, true, statSourceEmbedded},
		{"양쪽 → 혼합(요구3)", true, true, statSourceMixed},
		{"둘 다 아님 → 빈 문자열(표식 생략)", false, false, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, classifyStatSource(tc.inDirect, tc.inEmbed))
		})
	}
}

// ---------------------------------------------------------------------------
// 순수 함수 — tagStatSources (Extra 표식 부여)
// ---------------------------------------------------------------------------

// direct 전용 노드는 stat_source=direct 표식을 받아야 한다(shared/단독 직접 — S01).
func TestTagStatSources_direct전용(t *testing.T) {
	t.Parallel()
	direct := []engine.NodeInstanceInfo{nsNode("A", 10, 0)}
	merged := []engine.NodeInstanceInfo{nsNode("A", 10, 0)}

	got := tagStatSources(merged, direct, nil)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Extra)
	assert.Equal(t, statSourceDirect, got[0].Extra[statSourceKey])
}

// embedded 전용 노드는 stat_source=embedded 표식을 받아야 한다(instance 병합 — S02).
func TestTagStatSources_embedded전용(t *testing.T) {
	t.Parallel()
	embedded := []engine.NodeInstanceInfo{nsNode("A", 5, 1)}
	merged := []engine.NodeInstanceInfo{nsNode("A", 5, 1)}

	got := tagStatSources(merged, nil, embedded)
	require.Len(t, got, 1)
	assert.Equal(t, statSourceEmbedded, got[0].Extra[statSourceKey])
}

// 같은 원본 노드가 direct·embedded 양쪽에 있으면 혼합 표식을 받아야 한다(요구3 — 한 참조
// 플로우를 어떤 부모는 shared, 어떤 부모는 instance 로 참조).
func TestTagStatSources_혼합(t *testing.T) {
	t.Parallel()
	direct := []engine.NodeInstanceInfo{nsNode("A", 10, 0)}
	embedded := []engine.NodeInstanceInfo{nsNode("A", 5, 0)}
	// 병합 결과는 direct+embedded 합산(15)이며 동일 노드 ID 를 가진다.
	merged := mergeNodeInstanceStats(direct, embedded)

	got := tagStatSources(merged, direct, embedded)
	require.Len(t, got, 1)
	assert.Equal(t, int64(15), got[0].Processed, "혼합은 direct+embedded 합산(이중계상 아님)")
	assert.Equal(t, statSourceMixed, got[0].Extra[statSourceKey])
}

// 출처가 서로 다른 노드들이 섞여 있으면 각 노드가 정확한 표식을 받아야 한다(부분 혼합).
func TestTagStatSources_노드별_상이출처(t *testing.T) {
	t.Parallel()
	direct := []engine.NodeInstanceInfo{nsNode("A", 1, 0), nsNode("B", 2, 0)}
	embedded := []engine.NodeInstanceInfo{nsNode("B", 3, 0), nsNode("C", 4, 0)}
	merged := mergeNodeInstanceStats(direct, embedded)

	got := tagStatSources(merged, direct, embedded)
	byID := map[string]engine.NodeInstanceInfo{}
	for _, n := range got {
		byID[n.NodeID] = n
	}
	assert.Equal(t, statSourceDirect, byID["A"].Extra[statSourceKey], "A 는 direct 전용")
	assert.Equal(t, statSourceMixed, byID["B"].Extra[statSourceKey], "B 는 direct+embedded 혼합")
	assert.Equal(t, statSourceEmbedded, byID["C"].Extra[statSourceKey], "C 는 embedded 전용")
}

// 기존 Extra 키는 보존하고 stat_source 만 추가해야 한다(비파괴).
func TestTagStatSources_기존Extra보존(t *testing.T) {
	t.Parallel()
	n := nsNode("A", 1, 0)
	n.Extra = map[string]any{"keep": "value"}
	merged := []engine.NodeInstanceInfo{n}

	got := tagStatSources(merged, []engine.NodeInstanceInfo{nsNode("A", 1, 0)}, nil)
	require.Len(t, got, 1)
	assert.Equal(t, "value", got[0].Extra["keep"], "기존 Extra 키는 보존")
	assert.Equal(t, statSourceDirect, got[0].Extra[statSourceKey])
}

// ---------------------------------------------------------------------------
// 이중계상 방지 필터 — localFlowNodesReferencing (mode-aware)
// ---------------------------------------------------------------------------

// localFlowNodesReferencing 은 instance 참조만 임베디드 병합 대상으로 채택하고, shared(미지정
// 포함)·remote 참조는 제외해야 한다(S02 / 이중계상 방지 — 요구3).
func TestLocalFlowNodesReferencing_모드필터(t *testing.T) {
	t.Parallel()

	const subID = "S"
	def := flow.NewFlow("parent",
		flow.WithNodes(
			// instance 참조 → 채택.
			flowNode("inst1", "인스턴스1", subID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}),
			// shared 명시 참조 → 제외(direct 경로로만 노출).
			flowNodeShared("sh1", "공유1", subID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, true),
			// 미지정(=shared) 참조 → 제외.
			flowNodeShared("sh2", "공유2", subID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}, false),
			// 다른 서브플로우 instance 참조 → 제외(대상 불일치).
			flowNode("other", "기타", "OTHER", []flow.Port{inPort("X")}, []flow.Port{outPort("Y")}),
		),
	)

	got := localFlowNodesReferencing(def, subID)
	assert.Equal(t, []string{"inst1"}, got,
		"instance 참조만 임베디드 병합 대상이어야 한다(shared/미지정/타플로우 제외)")
}

// remote:// 참조 flow-node 는 임베디드 병합 대상에서 제외되어야 한다(라이브 브리지 — 네임스페이스
// 노드 없음).
func TestLocalFlowNodesReferencing_remote제외(t *testing.T) {
	t.Parallel()

	const subID = "S"
	remoteFN := flow.NodeDef{
		ID:      "rfn",
		Name:    "원격",
		Type:    flowNodeType,
		Config:  map[string]any{flowNodeFlowIDKey: "remote://inst/" + subID, flowNodeModeKey: flowModeInstance},
		Inputs:  []flow.Port{inPort("X")},
		Outputs: []flow.Port{outPort("Y")},
	}
	def := flow.NewFlow("parent", flow.WithNodes(remoteFN))

	got := localFlowNodesReferencing(def, subID)
	assert.Empty(t, got, "remote:// 참조는 LOCAL 임베디드 병합 대상이 아니다")
}

// ---------------------------------------------------------------------------
// 통합 — ListFlowNodes 출처 표식 (AC-16)
// ---------------------------------------------------------------------------

// instance 참조만 있는 서브플로우 단독 뷰는 임베디드 병합 결과를 노출하고 embedded 표식을
// 부여해야 한다(S02/S03 — 기존 동작 보존 + 표식 추가).
func TestListFlowNodes_instance참조_embedded표식(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const subflowID = "S"
	const parentID = "P"
	// buildParentReferencingSubflow 의 flowNode 헬퍼는 mode=instance 를 명시한다.
	buildParentReferencingSubflow(t, ctx, repo, subflowID, parentID)
	require.NoError(t, adapter.DeployFlow(ctx, parentID))

	nodes, err := adapter.ListFlowNodes(ctx, subflowID)
	require.NoError(t, err)

	a := nodeInfoByID(t, nodes, "A")
	require.NotNil(t, a.Extra, "임베디드 노드는 stat_source 표식을 가져야 한다")
	assert.Equal(t, statSourceEmbedded, a.Extra[statSourceKey],
		"instance 참조 임베디드 노드는 embedded 출처여야 한다(S02/S03)")
}

// 단독(또는 shared 직접) 배포 노드는 direct 표식을 부여받아야 한다(S01/S03).
func TestListFlowNodes_단독배포_direct표식(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const flowID = "standalone"
	f := flow.NewFlowWithID(flowID, "단독",
		flow.WithNodes(passthroughNode("only", "단독노드")),
	)
	require.NoError(t, repo.Save(ctx, f))
	require.NoError(t, adapter.DeployFlow(ctx, flowID))

	nodes, err := adapter.ListFlowNodes(ctx, flowID)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.NotNil(t, nodes[0].Extra)
	assert.Equal(t, statSourceDirect, nodes[0].Extra[statSourceKey],
		"단독/직접 실행 노드는 direct 출처여야 한다(S01/S03)")
}

// nodeInfoByID 는 ListFlowNodes 결과에서 노드 ID 로 항목을 찾는다(테스트 헬퍼).
func nodeInfoByID(t *testing.T, nodes []handler.FlowNodeInfo, id string) handler.FlowNodeInfo {
	t.Helper()
	for _, n := range nodes {
		if n.NodeID == id {
			return n
		}
	}
	t.Fatalf("노드 %q 가 결과에 없다, got=%v", id, nodes)
	return handler.FlowNodeInfo{}
}
